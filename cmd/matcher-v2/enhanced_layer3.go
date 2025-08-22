package main

import (
	"database/sql"
	"fmt"
	"runtime"
	"sync"
)

// UniqueAddress represents a unique address for fuzzy matching with deduplication
type UniqueAddress struct {
	RawAddress       string
	DocumentCount    int
	SampleDocumentID int
}

// FuzzyMatch represents a potential fuzzy match result
type FuzzyMatch struct {
	AddressID       int
	UPRN           string
	FullAddress    string
	LocationID     int
	SimilarityScore float64
}

// parallelFuzzyMatchIndividualDocuments performs enhanced parallel fuzzy matching with address deduplication
func parallelFuzzyMatchIndividualDocuments(localDebug bool, db *sql.DB) error {
	fmt.Println("Enhanced parallel fuzzy matching with address deduplication...")
	fmt.Println("=========================================================")
	
	// Auto-detect optimal worker count
	numWorkers := getOptimalWorkerCount()
	
	fmt.Printf("Using %d parallel workers (detected %d CPU cores)\n", numWorkers, runtime.NumCPU())
	
	// Step 1: Ensure required extensions are enabled
	_, err := db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm")
	if err != nil {
		return fmt.Errorf("failed to enable pg_trgm extension: %v", err)
	}
	
	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS fuzzystrmatch")
	if err != nil {
		return fmt.Errorf("failed to enable fuzzystrmatch extension: %v", err)
	}
	
	// Step 2: Get unique addresses that need fuzzy matching (address deduplication)
	fmt.Println("Finding unique addresses that need fuzzy matching...")
	
	uniqueAddressesSQL := `
	WITH unmatched_addresses AS (
		SELECT DISTINCT
			oa.raw_address,
			COUNT(*) as document_count,
			MIN(s.document_id) as sample_document_id
		FROM fact_documents_lean f
		JOIN dim_original_address oa ON f.original_address_id = oa.original_address_id
		JOIN src_document s ON f.document_id = s.document_id
		WHERE f.matched_address_id IS NULL  -- Still unmatched
		  AND oa.raw_address IS NOT NULL 
		  AND oa.raw_address != ''
		  AND LENGTH(oa.raw_address) > 15  -- Meaningful addresses only
		  AND oa.raw_address ~* '[a-zA-Z]'  -- Contains letters (not just numbers)
		GROUP BY oa.raw_address
		HAVING COUNT(*) >= 1  -- At least one document with this address
	)
	SELECT 
		raw_address,
		document_count,
		sample_document_id
	FROM unmatched_addresses
	ORDER BY document_count DESC, raw_address  -- Process high-impact addresses first
	`
	
	rows, err := db.Query(uniqueAddressesSQL)
	if err != nil {
		return fmt.Errorf("failed to find unique addresses: %v", err)
	}
	defer rows.Close()
	
	var uniqueAddresses []UniqueAddress
	for rows.Next() {
		var addr UniqueAddress
		err := rows.Scan(&addr.RawAddress, &addr.DocumentCount, &addr.SampleDocumentID)
		if err != nil {
			return fmt.Errorf("failed to scan unique address: %v", err)
		}
		uniqueAddresses = append(uniqueAddresses, addr)
	}
	
	totalDocumentsAffected := 0
	for _, addr := range uniqueAddresses {
		totalDocumentsAffected += addr.DocumentCount
	}
	
	fmt.Printf("Found %d unique addresses affecting %d total documents\n", 
		len(uniqueAddresses), totalDocumentsAffected)
	
	if len(uniqueAddresses) == 0 {
		fmt.Println("No addresses need fuzzy matching")
		return nil
	}
	
	// Step 3: Process unique addresses in parallel with workers
	batchSize := 25  // Addresses per batch
	numBatches := (len(uniqueAddresses) + batchSize - 1) / batchSize
	
	fmt.Printf("Processing %d batches of %d addresses each with %d workers\n", 
		numBatches, batchSize, numWorkers)
	
	// Channel for work distribution
	addressBatches := make(chan []UniqueAddress, numBatches)
	results := make(chan int, numBatches)  // Count of successful matches per batch
	
	// Worker pool
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			// Each worker gets its own database connection
			workerDB, err := connectDB()
			if err != nil {
				fmt.Printf("Worker %d failed to connect to database: %v\n", workerID, err)
				return
			}
			defer workerDB.Close()
			
			successCount := 0
			for batch := range addressBatches {
				batchSuccess := processFuzzyMatchBatch(workerID, batch, workerDB, localDebug)
				successCount += batchSuccess
			}
			results <- successCount
		}(i)
	}
	
	// Distribute work to workers
	go func() {
		for i := 0; i < len(uniqueAddresses); i += batchSize {
			end := i + batchSize
			if end > len(uniqueAddresses) {
				end = len(uniqueAddresses)
			}
			batch := uniqueAddresses[i:end]
			addressBatches <- batch
		}
		close(addressBatches)
	}()
	
	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(results)
	}()
	
	// Collect results
	totalMatches := 0
	processedBatches := 0
	for successCount := range results {
		totalMatches += successCount
		processedBatches++
		if processedBatches%5 == 0 || localDebug {
			fmt.Printf("Processed %d/%d batches, %d successful fuzzy matches so far\n", 
				processedBatches, numBatches, totalMatches)
		}
	}
	
	fmt.Printf("\n✓ Enhanced parallel fuzzy matching completed!\n")
	fmt.Printf("  Processed %d unique addresses\n", len(uniqueAddresses))
	fmt.Printf("  Found %d successful fuzzy matches\n", totalMatches)
	fmt.Printf("  Used %d parallel workers for optimal performance\n", numWorkers)
	
	return nil
}

// processFuzzyMatchBatch processes a batch of unique addresses for fuzzy matching
func processFuzzyMatchBatch(workerID int, batch []UniqueAddress, db *sql.DB, localDebug bool) int {
	successCount := 0
	
	for _, addr := range batch {
		matched := processIndividualFuzzyMatch(addr, db, localDebug)
		if matched {
			successCount++
		}
		
		// Progress indicator for debugging
		if localDebug && len(batch) > 10 {
			fmt.Printf("Worker %d: Processed '%s' affecting %d documents\n", 
				workerID, addr.RawAddress, addr.DocumentCount)
		}
	}
	
	return successCount
}

// processIndividualFuzzyMatch processes a single unique address and updates all related documents
func processIndividualFuzzyMatch(addr UniqueAddress, db *sql.DB, localDebug bool) bool {
	// Perform fuzzy matching using trigram similarity
	fuzzyMatchSQL := `
	SELECT 
		da.address_id,
		da.uprn,
		da.full_address,
		da.location_id,
		SIMILARITY(UPPER($1), UPPER(da.full_address)) as similarity_score
	FROM dim_address da
	WHERE SIMILARITY(UPPER($1), UPPER(da.full_address)) > 0.4  -- Minimum threshold
	ORDER BY similarity_score DESC
	LIMIT 3
	`
	
	rows, err := db.Query(fuzzyMatchSQL, addr.RawAddress)
	if err != nil {
		if localDebug {
			fmt.Printf("Error querying fuzzy matches for '%s': %v\n", addr.RawAddress, err)
		}
		return false
	}
	defer rows.Close()
	
	var matches []FuzzyMatch
	for rows.Next() {
		var match FuzzyMatch
		err := rows.Scan(&match.AddressID, &match.UPRN, &match.FullAddress, 
			&match.LocationID, &match.SimilarityScore)
		if err != nil {
			continue
		}
		matches = append(matches, match)
	}
	
	if len(matches) == 0 {
		return false
	}
	
	// Take the best match
	bestMatch := matches[0]
	
	// Apply additional validation - ensure minimum quality
	if bestMatch.SimilarityScore < 0.6 {
		return false
	}
	
	// Update ALL documents with this raw address (key deduplication feature)
	updateSQL := `
	UPDATE fact_documents_lean 
	SET matched_address_id = $1,
		matched_location_id = $2,
		match_method_id = 36,  -- "Enhanced Parallel Fuzzy Match"
		match_confidence_score = $3,
		match_decision_id = 2,  -- "Accepted"
		updated_at = CURRENT_TIMESTAMP
	FROM dim_original_address oa
	WHERE fact_documents_lean.original_address_id = oa.original_address_id
	  AND oa.raw_address = $4
	  AND fact_documents_lean.matched_address_id IS NULL  -- Only update unmatched
	`
	
	result, err := db.Exec(updateSQL, bestMatch.AddressID, bestMatch.LocationID, 
		bestMatch.SimilarityScore, addr.RawAddress)
	if err != nil {
		if localDebug {
			fmt.Printf("Error updating documents for '%s': %v\n", addr.RawAddress, err)
		}
		return false
	}
	
	rowsUpdated, _ := result.RowsAffected()
	
	if localDebug && rowsUpdated > 0 {
		fmt.Printf("✓ Matched '%s' → '%s' (%.3f similarity, %d documents updated)\n",
			addr.RawAddress, bestMatch.FullAddress, bestMatch.SimilarityScore, rowsUpdated)
	}
	
	return rowsUpdated > 0
}