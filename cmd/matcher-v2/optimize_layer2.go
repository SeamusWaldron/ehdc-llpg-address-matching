package main

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// optimizeLayer2Performance creates optimized structures for faster Layer 2 matching
func optimizeLayer2Performance(localDebug bool, db *sql.DB) error {
	fmt.Println("Optimizing Layer 2 Performance...")
	fmt.Println("================================")
	
	// 1. Create materialized combined address table
	fmt.Println("Creating materialized combined address table...")
	
	_, err := db.Exec(`DROP TABLE IF EXISTS dim_address_combined`)
	if err != nil {
		fmt.Printf("Warning: failed to drop existing table: %v\n", err)
	}
	
	combineQuery := `
	CREATE TABLE dim_address_combined AS
	SELECT 
		address_id,
		uprn,
		full_address,
		address_canonical,
		'original' as source_type,
		location_id,
		-- Add parsed components for faster matching
		gopostal_house_number,
		gopostal_road,
		gopostal_city,
		gopostal_postcode
	FROM dim_address
	WHERE uprn IS NOT NULL AND full_address IS NOT NULL
	
	UNION ALL
	
	SELECT 
		original_address_id as address_id,
		uprn,
		full_address,
		address_canonical,
		'expanded' as source_type,
		-- Get location_id from original address
		(SELECT location_id FROM dim_address da WHERE da.address_id = dae.original_address_id) as location_id,
		-- Parse components from expanded addresses (simplified)
		REGEXP_REPLACE(unit_number, '[^0-9A-Z]', '', 'g') as gopostal_house_number,
		REGEXP_REPLACE(UPPER(full_address), '^[^,]+,\\s*', '') as gopostal_road,
		'ALTON' as gopostal_city, -- Most addresses are in Alton area
		REGEXP_REPLACE(full_address, '.*(GU[0-9]{2}\\s*[0-9][A-Z]{2}).*', '\\1') as gopostal_postcode
	FROM dim_address_expanded dae
	WHERE uprn IS NOT NULL AND full_address IS NOT NULL
	`
	
	_, err = db.Exec(combineQuery)
	if err != nil {
		return fmt.Errorf("failed to create combined address table: %v", err)
	}
	
	// 2. Create optimized indexes
	fmt.Println("Creating optimized indexes...")
	
	indexes := []string{
		"CREATE INDEX idx_combined_uprn ON dim_address_combined(uprn)",
		"CREATE INDEX idx_combined_canonical ON dim_address_combined(address_canonical)",
		"CREATE INDEX idx_combined_house_road ON dim_address_combined(gopostal_house_number, gopostal_road)",
		"CREATE INDEX idx_combined_postcode_house ON dim_address_combined(gopostal_postcode, gopostal_house_number)",
		"CREATE INDEX idx_combined_full_text ON dim_address_combined USING gin(to_tsvector('english', full_address))",
		"CREATE INDEX idx_combined_canonical_trgm ON dim_address_combined USING gin(address_canonical gin_trgm_ops)",
	}
	
	for _, indexSQL := range indexes {
		_, err = db.Exec(indexSQL)
		if err != nil {
			fmt.Printf("Warning: failed to create index: %v\n", err)
		}
	}
	
	// 3. Get statistics
	var originalCount, expandedCount, totalCount int
	err = db.QueryRow(`
		SELECT 
			SUM(CASE WHEN source_type = 'original' THEN 1 ELSE 0 END) as original_count,
			SUM(CASE WHEN source_type = 'expanded' THEN 1 ELSE 0 END) as expanded_count,
			COUNT(*) as total_count
		FROM dim_address_combined
	`).Scan(&originalCount, &expandedCount, &totalCount)
	
	if err == nil {
		fmt.Printf("✓ Combined address table created:\n")
		fmt.Printf("  Original addresses: %d\n", originalCount)
		fmt.Printf("  Expanded addresses: %d\n", expandedCount)
		fmt.Printf("  Total searchable addresses: %d\n", totalCount)
	}
	
	return nil
}

// runOptimizedLayer2 runs Layer 2 with distinct address processing and batch updates
func runOptimizedLayer2(localDebug bool, db *sql.DB) error {
	fmt.Println("Running Optimized Layer 2: Address-Based Conservative Matching...")
	fmt.Println("================================================================")
	
	// First optimize the data structures
	err := optimizeLayer2Performance(localDebug, db)
	if err != nil {
		return fmt.Errorf("failed to optimize structures: %v", err)
	}
	
	// Get distinct unmatched addresses
	fmt.Println("\nFinding distinct unmatched addresses...")
	
	distinctQuery := `
	SELECT 
		o.raw_address,
		COUNT(*) as document_count,
		MIN(f.fact_id) as sample_fact_id,
		o.original_address_id
	FROM fact_documents_lean f
	JOIN dim_original_address o ON f.original_address_id = o.original_address_id
	WHERE f.matched_address_id IS NULL
	GROUP BY o.raw_address, o.original_address_id
	HAVING COUNT(*) BETWEEN 1 AND 50  -- Focus on reasonable frequency addresses
	  AND LENGTH(o.raw_address) > 10     -- Filter out short/incomplete addresses
	  AND o.raw_address NOT LIKE 'F%'    -- Exclude F-codes
	  AND o.raw_address NOT LIKE 'PRD%'  -- Exclude planning reference codes
	  AND o.raw_address NOT LIKE 'N/A%'  -- Exclude N/A entries
	  AND o.raw_address NOT LIKE 'ALR%'  -- Exclude ALR codes
	  AND o.raw_address NOT LIKE 'AUD%'  -- Exclude AUD codes
	  AND o.raw_address NOT LIKE 'UNKNOWN%'  -- Exclude unknown entries
	  AND o.raw_address ~ '[0-9]'         -- Must contain at least one number
	ORDER BY 
	  CASE WHEN o.raw_address ~ '^[0-9]+[A-Z]?\s' THEN 0 ELSE 1 END, -- Prefer addresses starting with house numbers
	  LENGTH(o.raw_address) DESC,  -- Prefer longer, more complete addresses
	  document_count DESC
	LIMIT 5000  -- Process top 5000 quality addresses
	`
	
	rows, err := db.Query(distinctQuery)
	if err != nil {
		return fmt.Errorf("failed to get distinct addresses: %v", err)
	}
	defer rows.Close()
	
	var addresses []struct {
		rawAddress        string
		documentCount     int
		sampleFactID      int
		originalAddressID int
	}
	
	for rows.Next() {
		var addr struct {
			rawAddress        string
			documentCount     int
			sampleFactID      int
			originalAddressID int
		}
		if err := rows.Scan(&addr.rawAddress, &addr.documentCount, &addr.sampleFactID, &addr.originalAddressID); err != nil {
			continue
		}
		addresses = append(addresses, addr)
	}
	
	fmt.Printf("Found %d distinct unmatched addresses to process\n", len(addresses))
	
	var totalMatches, totalDocuments int
	
	// Process each distinct address
	for i, addr := range addresses {
		if i%100 == 0 && i > 0 {
			fmt.Printf("Processed %d addresses, found %d matches affecting %d documents\n", 
				i, totalMatches, totalDocuments)
		}
		
		// Try to match this address using optimized queries
		matchedAddressID, confidence, err := tryOptimizedMatch(db, addr.rawAddress, localDebug)
		if err != nil {
			if localDebug {
				fmt.Printf("Error matching address '%s': %v\n", addr.rawAddress, err)
			}
			continue
		}
		
		if matchedAddressID > 0 {
			// Update ALL documents with this address
			updateQuery := `
			UPDATE fact_documents_lean 
			SET matched_address_id = $1, 
			    match_method_id = 4,  -- "Optimized Conservative Match"
			    match_confidence_score = $2,
			    match_decision_id = 2  -- "Accepted"
			WHERE original_address_id = $3 
			  AND matched_address_id IS NULL
			`
			
			result, err := db.Exec(updateQuery, matchedAddressID, confidence, addr.originalAddressID)
			if err != nil {
				if localDebug {
					fmt.Printf("Error updating documents for address '%s': %v\n", addr.rawAddress, err)
				}
				continue
			}
			
			rowsAffected, _ := result.RowsAffected()
			totalMatches++
			totalDocuments += int(rowsAffected)
			
			if localDebug && i < 10 {
				fmt.Printf("✓ Matched address '%s' -> UPRN found, updated %d documents\n", 
					addr.rawAddress, rowsAffected)
			}
		}
	}
	
	fmt.Printf("\n=== OPTIMIZED LAYER 2 RESULTS ===\n")
	fmt.Printf("Distinct addresses processed: %d\n", len(addresses))
	fmt.Printf("Successful matches: %d\n", totalMatches)
	fmt.Printf("Documents updated: %d\n", totalDocuments)
	fmt.Printf("Match rate: %.1f%%\n", float64(totalMatches)/float64(len(addresses))*100)
	
	return nil
}

// tryOptimizedMatch attempts to match an address using the optimized combined table
func tryOptimizedMatch(db *sql.DB, rawAddress string, debug bool) (int, float64, error) {
	// Normalize the address for canonical matching
	canonicalAddress := normalizeAddress(rawAddress)
	
	// Try exact canonical match first (fastest)
	exactQuery := `
	SELECT address_id, uprn, full_address, 1.0 as confidence
	FROM dim_address_combined
	WHERE address_canonical = $1
	ORDER BY source_type = 'expanded', address_id
	LIMIT 1
	`
	
	var addressID int
	var uprn, fullAddress string
	var confidence float64
	
	err := db.QueryRow(exactQuery, canonicalAddress).Scan(&addressID, &uprn, &fullAddress, &confidence)
	if err == nil {
		if debug {
			fmt.Printf("  Exact canonical match: '%s' -> '%s' (UPRN: %s)\n", 
				rawAddress, fullAddress, uprn)
		}
		return addressID, confidence, nil
	}
	
	// Try component-based matching with optimized query
	components := parseAddressComponents(rawAddress)
	if components.houseNumber != "" && components.street != "" {
		componentQuery := `
		SELECT address_id, uprn, full_address, 
		       similarity(address_canonical, $1) as confidence
		FROM dim_address_combined
		WHERE gopostal_house_number = $2
		  AND gopostal_road ILIKE '%' || $3 || '%'
		  AND similarity(address_canonical, $1) >= 0.8
		ORDER BY confidence DESC, source_type = 'expanded', address_id
		LIMIT 1
		`
		
		err = db.QueryRow(componentQuery, canonicalAddress, components.houseNumber, components.street).
			Scan(&addressID, &uprn, &fullAddress, &confidence)
		if err == nil {
			if debug {
				fmt.Printf("  Component match: '%s' -> '%s' (UPRN: %s, conf: %.3f)\n", 
					rawAddress, fullAddress, uprn, confidence)
			}
			return addressID, confidence, nil
		}
	}
	
	return 0, 0, fmt.Errorf("no match found")
}

type addressComponents struct {
	houseNumber string
	street      string
	locality    string
	postcode    string
}

func parseAddressComponents(address string) addressComponents {
	// Simple component extraction (could be enhanced)
	parts := strings.Split(strings.ToUpper(address), ",")
	
	var components addressComponents
	if len(parts) > 0 {
		// Extract house number from first part
		firstPart := strings.TrimSpace(parts[0])
		houseMatch := regexp.MustCompile(`^(\d+[A-Z]?)`).FindString(firstPart)
		components.houseNumber = houseMatch
		
		// Extract street (rest of first part)
		if houseMatch != "" {
			components.street = strings.TrimSpace(strings.TrimPrefix(firstPart, houseMatch))
		}
	}
	
	return components
}

func normalizeAddress(address string) string {
	// Normalize to canonical form
	normalized := strings.ToUpper(strings.TrimSpace(address))
	normalized = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(normalized, "")
	normalized = regexp.MustCompile(`\s+`).ReplaceAllString(normalized, " ")
	return normalized
}