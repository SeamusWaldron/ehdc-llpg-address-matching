package main

import (
	"database/sql"
	"fmt"
)

// createLayerSnapshot creates a complete snapshot of the fact table after each layer
func createLayerSnapshot(localDebug bool, db *sql.DB, layerName string) error {
	snapshotTable := fmt.Sprintf("snapshot_fact_documents_lean_%s", layerName)
	
	fmt.Printf("Creating snapshot: %s\n", snapshotTable)
	
	// Drop existing snapshot table if it exists
	dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", snapshotTable)
	_, err := db.Exec(dropSQL)
	if err != nil {
		return fmt.Errorf("failed to drop existing snapshot table %s: %v", snapshotTable, err)
	}
	
	// Create new snapshot table with complete copy
	createSQL := fmt.Sprintf(`
		CREATE TABLE %s AS 
		SELECT * FROM fact_documents_lean
	`, snapshotTable)
	
	_, err = db.Exec(createSQL)
	if err != nil {
		return fmt.Errorf("failed to create snapshot table %s: %v", snapshotTable, err)
	}
	
	// Get row count
	var count int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s", snapshotTable)
	err = db.QueryRow(countSQL).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count snapshot rows: %v", err)
	}
	
	fmt.Printf("✓ Created snapshot %s with %d records\n", snapshotTable, count)
	return nil
}

// rebuildFactTableIntelligent implements the intelligent fact table population strategy
// Phase 1: Insert records with UPRN matches
// Phase 2: Insert records with canonical address matches  
// Phase 3: Insert remaining unmatched records
func rebuildFactTableIntelligent(localDebug bool, db *sql.DB) error {
	fmt.Println("Rebuilding fact table with intelligent UPRN-first approach...")
	fmt.Println("===========================================================")
	
	// First, clear the fact table
	fmt.Println("Truncating fact table...")
	_, err := db.Exec("TRUNCATE TABLE fact_documents_lean")
	if err != nil {
		return fmt.Errorf("failed to truncate fact table: %v", err)
	}
	
	// Ensure dim_original_address is up to date
	fmt.Println("Updating dim_original_address...")
	updateDimOriginalQuery := `
	INSERT INTO dim_original_address (raw_address, address_hash)
	SELECT DISTINCT 
	    raw_address,
	    MD5(UPPER(TRIM(raw_address))) as address_hash
	FROM src_document s
	WHERE raw_address IS NOT NULL 
	  AND raw_address != ''
	  AND NOT EXISTS (
	    SELECT 1 FROM dim_original_address oa 
	    WHERE oa.raw_address = s.raw_address
	  )
	`
	result, err := db.Exec(updateDimOriginalQuery)
	if err != nil {
		return fmt.Errorf("failed to update dim_original_address: %v", err)
	}
	newOriginalAddresses, _ := result.RowsAffected()
	if newOriginalAddresses > 0 {
		fmt.Printf("  ✓ Added %d new original addresses\n", newOriginalAddresses)
	}
	
	// PHASE 1: Insert records with UPRN matches
	fmt.Println("\nPhase 1: Inserting records with UPRN matches...")
	uprnQuery := `
	INSERT INTO fact_documents_lean (
		document_id, doc_type_id, document_status_id, original_address_id,
		matched_address_id, match_method_id, match_confidence_score, 
		property_type_id, application_status_id, development_type_id,
		application_date_id, decision_date_id, import_date_id, 
		match_decision_id, matched_location_id, planning_reference
	)
	SELECT DISTINCT ON (s.document_id)
		s.document_id,
		COALESCE(s.doc_type_id, 1) as doc_type_id,
		1 as document_status_id,  -- Active
		oa.original_address_id,
		da.address_id as matched_address_id,  -- UPRN MATCH!
		1 as match_method_id,  -- "Exact UPRN Match"
		1.0 as match_confidence_score,
		NULL as property_type_id,  -- Unknown
		NULL as application_status_id,
		NULL as development_type_id,
		CASE 
			WHEN s.document_date IS NOT NULL THEN TO_CHAR(s.document_date, 'YYYYMMDD')::INTEGER
			ELSE NULL
		END as application_date_id,
		NULL::INTEGER as decision_date_id,
		TO_CHAR(CURRENT_DATE, 'YYYYMMDD')::INTEGER as import_date_id,
		2 as match_decision_id,  -- "Accepted"
		da.location_id as matched_location_id,
		CASE 
		WHEN s.planning_app_sequence IS NOT NULL AND s.planning_app_sequence != '' 
		THEN s.planning_app_base || '/' || s.planning_app_sequence
		ELSE s.planning_app_base
	END as planning_reference
	FROM src_document s
	INNER JOIN dim_original_address oa ON oa.raw_address = s.raw_address
	INNER JOIN dim_address da ON da.uprn = s.raw_uprn  -- UPRN MATCH
	WHERE s.raw_uprn IS NOT NULL 
	  AND s.raw_uprn != ''
	  AND da.uprn IS NOT NULL
	ORDER BY s.document_id, da.address_id
	`
	
	result, err = db.Exec(uprnQuery)
	if err != nil {
		return fmt.Errorf("failed to insert UPRN matches: %v", err)
	}
	uprnMatches, _ := result.RowsAffected()
	fmt.Printf("  ✓ Inserted %d records with UPRN matches\n", uprnMatches)
	
	// PHASE 2: Insert records with exact canonical address matches
	fmt.Println("\nPhase 2: Inserting records with exact canonical address matches...")
	
	// First ensure we have an index on canonical addresses for speed
	fmt.Println("  Creating index on canonical addresses...")
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS idx_dim_address_canonical ON dim_address(address_canonical)")
	if err != nil {
		fmt.Printf("  Warning: failed to create canonical index: %v\n", err)
	}
	
	canonicalQuery := `
	WITH unmatched_canonical AS (
		-- Get unique canonical addresses from unmatched source documents
		SELECT DISTINCT 
			UPPER(REGEXP_REPLACE(COALESCE(s.standardized_address, s.raw_address), '[^\w\s]', '', 'g')) as canonical_address
		FROM src_document s
		WHERE NOT EXISTS (SELECT 1 FROM fact_documents_lean f WHERE f.document_id = s.document_id)
		  AND s.raw_uprn IS NULL OR s.raw_uprn = ''  -- No UPRN in source
	)
	INSERT INTO fact_documents_lean (
		document_id, doc_type_id, document_status_id, original_address_id,
		matched_address_id, match_method_id, match_confidence_score,
		property_type_id, application_status_id, development_type_id,
		application_date_id, decision_date_id, import_date_id,
		match_decision_id, matched_location_id, planning_reference
	)
	SELECT DISTINCT ON (s.document_id)
		s.document_id,
		COALESCE(s.doc_type_id, 1) as doc_type_id,
		1 as document_status_id,
		oa.original_address_id,
		da.address_id as matched_address_id,  -- EXACT CANONICAL MATCH!
		2 as match_method_id,  -- "Exact Canonical Match"
		1.0 as match_confidence_score,  -- Exact match
		NULL::INTEGER as property_type_id,
		NULL::INTEGER as application_status_id,
		NULL::INTEGER as development_type_id,
		CASE 
			WHEN s.document_date IS NOT NULL THEN TO_CHAR(s.document_date, 'YYYYMMDD')::INTEGER
			ELSE NULL
		END as application_date_id,
		NULL::INTEGER as decision_date_id,
		TO_CHAR(CURRENT_DATE, 'YYYYMMDD')::INTEGER as import_date_id,
		2 as match_decision_id,  -- "Accepted"
		da.location_id as matched_location_id,
		CASE 
		WHEN s.planning_app_sequence IS NOT NULL AND s.planning_app_sequence != '' 
		THEN s.planning_app_base || '/' || s.planning_app_sequence
		ELSE s.planning_app_base
	END as planning_reference
	FROM src_document s
	INNER JOIN dim_original_address oa ON oa.raw_address = s.raw_address
	INNER JOIN dim_address da ON 
		da.address_canonical = UPPER(REGEXP_REPLACE(COALESCE(s.standardized_address, s.raw_address), '[^\w\s]', '', 'g'))
	WHERE NOT EXISTS (SELECT 1 FROM fact_documents_lean f WHERE f.document_id = s.document_id)  -- Not already inserted
	  AND (s.raw_uprn IS NULL OR s.raw_uprn = '')  -- No UPRN in source
	ORDER BY s.document_id, da.address_id
	`
	
	result, err = db.Exec(canonicalQuery)
	if err != nil {
		return fmt.Errorf("failed to insert canonical matches: %v", err)
	}
	canonicalMatches, _ := result.RowsAffected()
	fmt.Printf("  ✓ Inserted %d records with canonical address matches\n", canonicalMatches)
	
	// PHASE 2b: Also check exact canonical matches in dim_address_expanded
	fmt.Println("\nPhase 2b: Checking expanded addresses for exact canonical matches...")
	
	// Create index on expanded canonical addresses for speed
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS idx_dim_address_expanded_canonical ON dim_address_expanded(address_canonical)")
	if err != nil {
		fmt.Printf("  Warning: failed to create expanded canonical index: %v\n", err)
	}
	
	expandedCanonicalQuery := `
	INSERT INTO fact_documents_lean (
		document_id, doc_type_id, document_status_id, original_address_id,
		matched_address_id, match_method_id, match_confidence_score,
		property_type_id, application_status_id, development_type_id,
		application_date_id, decision_date_id, import_date_id,
		match_decision_id, matched_location_id, planning_reference
	)
	SELECT DISTINCT ON (s.document_id)
		s.document_id,
		COALESCE(s.doc_type_id, 1) as doc_type_id,
		1 as document_status_id,
		oa.original_address_id,
		dae.original_address_id as matched_address_id,  -- EXPANDED CANONICAL MATCH!
		3 as match_method_id,  -- "Expanded Canonical Match"
		1.0 as match_confidence_score,  -- Exact match
		NULL::INTEGER as property_type_id,
		NULL::INTEGER as application_status_id,
		NULL::INTEGER as development_type_id,
		CASE 
			WHEN s.document_date IS NOT NULL THEN TO_CHAR(s.document_date, 'YYYYMMDD')::INTEGER
			ELSE NULL
		END as application_date_id,
		NULL::INTEGER as decision_date_id,
		TO_CHAR(CURRENT_DATE, 'YYYYMMDD')::INTEGER as import_date_id,
		2 as match_decision_id,  -- "Accepted"
		da.location_id as matched_location_id,
		CASE 
		WHEN s.planning_app_sequence IS NOT NULL AND s.planning_app_sequence != '' 
		THEN s.planning_app_base || '/' || s.planning_app_sequence
		ELSE s.planning_app_base
	END as planning_reference
	FROM src_document s
	INNER JOIN dim_original_address oa ON oa.raw_address = s.raw_address
	INNER JOIN dim_address_expanded dae ON 
		dae.address_canonical = UPPER(REGEXP_REPLACE(COALESCE(s.standardized_address, s.raw_address), '[^\w\s]', '', 'g'))
	INNER JOIN dim_address da ON da.address_id = dae.original_address_id
	WHERE NOT EXISTS (SELECT 1 FROM fact_documents_lean f WHERE f.document_id = s.document_id)  -- Not already inserted
	  AND (s.raw_uprn IS NULL OR s.raw_uprn = '')  -- No UPRN in source
	ORDER BY s.document_id, dae.original_address_id
	`
	
	result, err = db.Exec(expandedCanonicalQuery)
	if err != nil {
		// Ignore errors for expanded addresses as table might not exist
		if localDebug {
			fmt.Printf("  Note: Expanded canonical matching skipped: %v\n", err)
		}
	} else {
		expandedCanonicalMatches, _ := result.RowsAffected()
		fmt.Printf("  ✓ Inserted %d records with expanded canonical matches\n", expandedCanonicalMatches)
		canonicalMatches += expandedCanonicalMatches
	}
	
	// PHASE 3: Insert remaining unmatched records
	fmt.Println("\nPhase 3: Inserting remaining unmatched records...")
	
	// Check how many records are already in fact table
	var currentFactCount int
	err = db.QueryRow("SELECT COUNT(*) FROM fact_documents_lean").Scan(&currentFactCount)
	if err == nil {
		fmt.Printf("  Current fact table has %d records before Phase 3\n", currentFactCount)
	}
	
	// Check how many source documents should be unmatched
	var sourceCount, remainingCount int
	err = db.QueryRow("SELECT COUNT(*) FROM src_document").Scan(&sourceCount)
	if err == nil {
		remainingCount = sourceCount - currentFactCount
		fmt.Printf("  Source documents: %d, Already processed: %d, Remaining: %d\n", 
			sourceCount, currentFactCount, remainingCount)
	}
	
	unmatchedQuery := `
	INSERT INTO fact_documents_lean (
		document_id, doc_type_id, document_status_id, original_address_id,
		matched_address_id, match_method_id, match_confidence_score,
		property_type_id, application_status_id, development_type_id,
		application_date_id, decision_date_id, import_date_id,
		match_decision_id, matched_location_id, planning_reference
	)
	SELECT DISTINCT ON (s.document_id)
		s.document_id,
		COALESCE(s.doc_type_id, 1) as doc_type_id,
		1 as document_status_id,
		oa.original_address_id,
		NULL::INTEGER as matched_address_id,  -- NO MATCH YET
		NULL::INTEGER as match_method_id,
		NULL::NUMERIC as match_confidence_score,
		NULL::INTEGER as property_type_id,
		NULL::INTEGER as application_status_id,
		NULL::INTEGER as development_type_id,
		CASE 
			WHEN s.document_date IS NOT NULL THEN TO_CHAR(s.document_date, 'YYYYMMDD')::INTEGER
			ELSE NULL
		END as application_date_id,
		NULL::INTEGER as decision_date_id,
		TO_CHAR(CURRENT_DATE, 'YYYYMMDD')::INTEGER as import_date_id,
		1 as match_decision_id,  -- "Pending"
		NULL::INTEGER as matched_location_id,
		CASE 
		WHEN s.planning_app_sequence IS NOT NULL AND s.planning_app_sequence != '' 
		THEN s.planning_app_base || '/' || s.planning_app_sequence
		ELSE s.planning_app_base
	END as planning_reference
	FROM src_document s
	INNER JOIN dim_original_address oa ON oa.raw_address = s.raw_address
	WHERE NOT EXISTS (SELECT 1 FROM fact_documents_lean f WHERE f.document_id = s.document_id)  -- Not already inserted
	ORDER BY s.document_id, oa.original_address_id
	`
	
	result, err = db.Exec(unmatchedQuery)
	if err != nil {
		return fmt.Errorf("failed to insert unmatched records: %v", err)
	}
	unmatchedRecords, _ := result.RowsAffected()
	fmt.Printf("  ✓ Inserted %d unmatched records\n", unmatchedRecords)
	
	// Summary
	totalRecords := uprnMatches + canonicalMatches + unmatchedRecords
	fmt.Printf("\n=== INTELLIGENT FACT TABLE SUMMARY ===\n")
	fmt.Printf("Total records: %d\n", totalRecords)
	fmt.Printf("  UPRN matches: %d (%.1f%%)\n", uprnMatches, float64(uprnMatches)/float64(totalRecords)*100)
	fmt.Printf("  Canonical matches: %d (%.1f%%)\n", canonicalMatches, float64(canonicalMatches)/float64(totalRecords)*100)
	fmt.Printf("  Unmatched (for further processing): %d (%.1f%%)\n", unmatchedRecords, float64(unmatchedRecords)/float64(totalRecords)*100)
	
	// Verify all source records are accounted for
	var totalSourceCount int
	err = db.QueryRow("SELECT COUNT(*) FROM src_document").Scan(&totalSourceCount)
	if err == nil {
		if int64(totalSourceCount) == totalRecords {
			fmt.Printf("\n✓ All %d source records accounted for\n", totalSourceCount)
		} else {
			fmt.Printf("\n⚠ Warning: %d source records but %d in fact table (difference: %d)\n", 
				totalSourceCount, totalRecords, int64(totalSourceCount)-totalRecords)
		}
	}
	
	// Show sample matches from each phase
	if localDebug {
		fmt.Println("\n=== SAMPLE MATCHES ===")
		
		// Sample UPRN matches
		fmt.Println("\nSample UPRN matches:")
		sampleQuery := `
		SELECT s.raw_address, da.full_address, da.uprn
		FROM fact_documents_lean f
		JOIN src_document s ON f.document_id = s.document_id
		JOIN dim_address da ON f.matched_address_id = da.address_id
		WHERE f.match_method_id = 1
		LIMIT 3
		`
		rows, err := db.Query(sampleQuery)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var source, matched, uprn string
				if rows.Scan(&source, &matched, &uprn) == nil {
					fmt.Printf("  %s\n    → %s (UPRN: %s)\n", source, matched, uprn)
				}
			}
		}
		
		// Sample canonical matches
		fmt.Println("\nSample canonical matches:")
		sampleQuery = `
		SELECT s.raw_address, da.full_address, f.match_confidence_score
		FROM fact_documents_lean f
		JOIN src_document s ON f.document_id = s.document_id
		JOIN dim_address da ON f.matched_address_id = da.address_id
		WHERE f.match_method_id = 32
		ORDER BY f.match_confidence_score DESC
		LIMIT 3
		`
		rows, err = db.Query(sampleQuery)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var source, matched string
				var score float64
				if rows.Scan(&source, &matched, &score) == nil {
					fmt.Printf("  %s\n    → %s (similarity: %.3f)\n", source, matched, score)
				}
			}
		}
	}
	
	// Create Layer 1 snapshot after intelligent fact table population
	fmt.Println("\n=== CREATING LAYER 1 SNAPSHOT ===")
	err = createLayerSnapshot(localDebug, db, "layer_1")
	if err != nil {
		return fmt.Errorf("failed to create Layer 1 snapshot: %v", err)
	}
	
	return nil
}