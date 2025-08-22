package main

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

// GroupBatch represents a batch of groups to process in parallel
type GroupBatch struct {
	ID     int
	Groups []UnmatchedGroup
}

// DocumentBatch represents a batch of documents to process in parallel
type DocumentBatch struct {
	ID        int
	Documents []UnmatchedDocument
}

// UnmatchedGroup represents a planning group with poor matches
type UnmatchedGroup struct {
	PlanningAppBase     string
	TotalDocs          int
	MatchedDocs        int
	BestAddressInGroup string
	AvgConfidence      float64
}

// UnmatchedDocument represents an individual document needing fuzzy matching
type UnmatchedDocument struct {
	DocumentID int
	RawAddress string
}

// FuzzyMatchResult represents the result of a fuzzy match
type FuzzyMatchResult struct {
	AddressID        int
	UPRN            string
	FullAddress     string
	SimilarityScore float64
	EditDistance    int
}

// ParallelLayer3Result holds results from parallel processing
type ParallelLayer3Result struct {
	BatchID       int
	ProcessedCount int
	MatchCount    int
	DocumentCount int
	ProcessTime   time.Duration
	Error         error
	BatchType     string // "groups" or "documents"
}

// runParallelLayer3Groups runs Layer 3a (Group-based Fuzzy Matching) with parallel processing
func runParallelLayer3Groups(localDebug bool, db *sql.DB) error {
	fmt.Println("Running Parallel Layer 3a: Group-based Fuzzy Matching...")
	fmt.Println("===========================================================")

	// Step 1: Ensure required extensions are enabled
	fmt.Println("Enabling fuzzy matching extensions...")
	_, err := db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm")
	if err != nil {
		return fmt.Errorf("failed to enable pg_trgm extension: %v", err)
	}
	
	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS fuzzystrmatch")
	if err != nil {
		return fmt.Errorf("failed to enable fuzzystrmatch extension: %v", err)
	}

	// Step 2: Find groups with poor or no matches for fuzzy matching
	fmt.Println("Finding planning groups with poor matches...")
	
	unmatchedGroupsSQL := `
	WITH low_confidence_groups AS (
		SELECT 
			s.planning_app_base,
			COUNT(*) as total_docs,
			COUNT(f.matched_address_id) as matched_docs,
			(SELECT raw_address FROM src_document s2 WHERE s2.planning_app_base = s.planning_app_base LIMIT 1) as best_address_in_group,
			COALESCE(AVG(f.match_confidence_score), 0) as avg_confidence
		FROM src_document s
		LEFT JOIN fact_documents_lean f ON s.document_id = f.document_id
		WHERE s.planning_app_base IS NOT NULL 
		  AND s.planning_app_base != ''
		  AND s.raw_address IS NOT NULL
		GROUP BY s.planning_app_base
		HAVING COUNT(*) >= 2  -- Groups with at least 2 documents
		  AND (COUNT(f.matched_address_id) = 0 OR COALESCE(AVG(f.match_confidence_score), 0) < 0.7)  -- Poor or no matches
	)
	SELECT planning_app_base, total_docs, matched_docs, best_address_in_group, avg_confidence
	FROM low_confidence_groups
	ORDER BY total_docs DESC, avg_confidence ASC
	-- No limit - process all qualifying groups for production
	`
	
	rows, err := db.Query(unmatchedGroupsSQL)
	if err != nil {
		return fmt.Errorf("failed to get unmatched groups: %v", err)
	}
	defer rows.Close()
	
	var unmatchedGroups []UnmatchedGroup
	for rows.Next() {
		var group UnmatchedGroup
		if err := rows.Scan(&group.PlanningAppBase, &group.TotalDocs, &group.MatchedDocs, 
			&group.BestAddressInGroup, &group.AvgConfidence); err != nil {
			continue
		}
		unmatchedGroups = append(unmatchedGroups, group)
	}
	
	fmt.Printf("Found %d groups with poor matches to process\n", len(unmatchedGroups))
	
	if len(unmatchedGroups) == 0 {
		fmt.Println("No groups with poor matches found")
		return nil
	}

	// Step 3: Create batches for parallel processing
	batchSize := 50  // Process 50 groups per batch for production (optimized for throughput)
	numWorkers := getOptimalWorkerCount()  // Auto-detect optimal worker count
	
	batches := createGroupBatches(unmatchedGroups, batchSize)
	fmt.Printf("Created %d batches of ~%d groups each for %d parallel workers\n", 
		len(batches), batchSize, numWorkers)
	
	// Step 4: Set up parallel processing
	batchChan := make(chan GroupBatch, len(batches))
	resultChan := make(chan ParallelLayer3Result, len(batches))
	
	// Send all batches to the channel
	for _, batch := range batches {
		batchChan <- batch
	}
	close(batchChan)
	
	// Step 5: Start worker goroutines
	var wg sync.WaitGroup
	startTime := time.Now()
	
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processGroupBatchWorker(workerID, db, batchChan, resultChan, localDebug)
		}(i)
	}
	
	// Wait for all workers to complete
	wg.Wait()
	close(resultChan)
	
	// Step 6: Collect and summarize results
	var totalMatches, totalDocuments, totalProcessed int
	var totalBatches int
	
	fmt.Println("\n=== PARALLEL LAYER 3a RESULTS ===")
	for result := range resultChan {
		totalBatches++
		totalProcessed += result.ProcessedCount
		totalMatches += result.MatchCount
		totalDocuments += result.DocumentCount
		
		if result.Error != nil {
			fmt.Printf("Batch %d: ERROR - %v\n", result.BatchID, result.Error)
		} else {
			fmt.Printf("Batch %d: %d groups, %d matches, %d documents updated (%.1fs)\n", 
				result.BatchID, result.ProcessedCount, result.MatchCount, result.DocumentCount, result.ProcessTime.Seconds())
		}
	}
	
	totalTime := time.Since(startTime)
	
	fmt.Printf("\n=== PARALLEL LAYER 3a SUMMARY ===\n")
	fmt.Printf("Total batches processed: %d\n", totalBatches)
	fmt.Printf("Total groups processed: %d\n", totalProcessed)
	fmt.Printf("Successful matches: %d\n", totalMatches)
	fmt.Printf("Documents updated: %d\n", totalDocuments)
	if totalProcessed > 0 {
		fmt.Printf("Overall match rate: %.1f%%\n", float64(totalMatches)/float64(totalProcessed)*100)
	}
	fmt.Printf("Total processing time: %.1f seconds\n", totalTime.Seconds())
	if totalProcessed > 0 {
		fmt.Printf("Average throughput: %.1f groups/second\n", float64(totalProcessed)/totalTime.Seconds())
	}
	
	if totalMatches > 0 {
		fmt.Printf("Average impact: %.1f documents per successful match\n", float64(totalDocuments)/float64(totalMatches))
	}
	
	return nil
}

// runParallelLayer3Documents runs Layer 3b (Individual Document Fuzzy Matching) with parallel processing
func runParallelLayer3Documents(localDebug bool, db *sql.DB) error {
	fmt.Println("Running Parallel Layer 3b: Individual Document Fuzzy Matching...")
	fmt.Println("===============================================================")

	// Step 1: Find individual documents with poor or no matches
	fmt.Println("Finding individual documents with poor matches...")
	
	individualDocsSQL := `
	SELECT 
		s.document_id,
		s.raw_address
	FROM src_document s
	LEFT JOIN fact_documents_lean f ON s.document_id = f.document_id
	WHERE s.raw_address IS NOT NULL 
	  AND s.raw_address != ''
	  AND LENGTH(s.raw_address) > 10  -- Reasonable length addresses
	  AND s.raw_address !~ '^(F|PRD|N/A|ALR|AUD|UNKNOWN)'  -- Exclude codes
	  AND s.raw_address ~ '[0-9]'  -- Must contain a number
	  AND (f.matched_address_id IS NULL OR f.match_confidence_score < 0.6)  -- Poor or no matches
	ORDER BY LENGTH(s.raw_address) DESC  -- Process longer, more complete addresses first
	-- No limit - process all qualifying individual documents for production
	`
	
	rows, err := db.Query(individualDocsSQL)
	if err != nil {
		return fmt.Errorf("failed to get individual documents: %v", err)
	}
	defer rows.Close()
	
	var unmatchedDocs []UnmatchedDocument
	for rows.Next() {
		var doc UnmatchedDocument
		if err := rows.Scan(&doc.DocumentID, &doc.RawAddress); err != nil {
			continue
		}
		unmatchedDocs = append(unmatchedDocs, doc)
	}
	
	fmt.Printf("Found %d individual documents with poor matches to process\n", len(unmatchedDocs))
	
	if len(unmatchedDocs) == 0 {
		fmt.Println("No individual documents with poor matches found")
		return nil
	}

	// Step 2: Create batches for parallel processing
	batchSize := 100  // Process 100 documents per batch for production (optimized for throughput)
	numWorkers := getOptimalWorkerCount()  // Auto-detect optimal worker count
	
	batches := createDocumentBatches(unmatchedDocs, batchSize)
	fmt.Printf("Created %d batches of ~%d documents each for %d parallel workers\n", 
		len(batches), batchSize, numWorkers)
	
	// Step 3: Set up parallel processing
	batchChan := make(chan DocumentBatch, len(batches))
	resultChan := make(chan ParallelLayer3Result, len(batches))
	
	// Send all batches to the channel
	for _, batch := range batches {
		batchChan <- batch
	}
	close(batchChan)
	
	// Step 4: Start worker goroutines
	var wg sync.WaitGroup
	startTime := time.Now()
	
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processDocumentBatchWorker(workerID, db, batchChan, resultChan, localDebug)
		}(i)
	}
	
	// Wait for all workers to complete
	wg.Wait()
	close(resultChan)
	
	// Step 5: Collect and summarize results
	var totalMatches, totalDocuments, totalProcessed int
	var totalBatches int
	
	fmt.Println("\n=== PARALLEL LAYER 3b RESULTS ===")
	for result := range resultChan {
		totalBatches++
		totalProcessed += result.ProcessedCount
		totalMatches += result.MatchCount
		totalDocuments += result.DocumentCount
		
		if result.Error != nil {
			fmt.Printf("Batch %d: ERROR - %v\n", result.BatchID, result.Error)
		} else {
			fmt.Printf("Batch %d: %d documents, %d matches (%.1fs)\n", 
				result.BatchID, result.ProcessedCount, result.MatchCount, result.ProcessTime.Seconds())
		}
	}
	
	totalTime := time.Since(startTime)
	
	fmt.Printf("\n=== PARALLEL LAYER 3b SUMMARY ===\n")
	fmt.Printf("Total batches processed: %d\n", totalBatches)
	fmt.Printf("Total documents processed: %d\n", totalProcessed)
	fmt.Printf("Successful matches: %d\n", totalMatches)
	fmt.Printf("Documents updated: %d\n", totalDocuments)
	if totalProcessed > 0 {
		fmt.Printf("Overall match rate: %.1f%%\n", float64(totalMatches)/float64(totalProcessed)*100)
	}
	fmt.Printf("Total processing time: %.1f seconds\n", totalTime.Seconds())
	if totalProcessed > 0 {
		fmt.Printf("Average throughput: %.1f documents/second\n", float64(totalProcessed)/totalTime.Seconds())
	}
	
	return nil
}

// createGroupBatches splits groups into batches for parallel processing
func createGroupBatches(groups []UnmatchedGroup, batchSize int) []GroupBatch {
	var batches []GroupBatch
	
	for i := 0; i < len(groups); i += batchSize {
		end := i + batchSize
		if end > len(groups) {
			end = len(groups)
		}
		
		batch := GroupBatch{
			ID:     len(batches) + 1,
			Groups: groups[i:end],
		}
		batches = append(batches, batch)
	}
	
	return batches
}

// createDocumentBatches splits documents into batches for parallel processing
func createDocumentBatches(docs []UnmatchedDocument, batchSize int) []DocumentBatch {
	var batches []DocumentBatch
	
	for i := 0; i < len(docs); i += batchSize {
		end := i + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		
		batch := DocumentBatch{
			ID:        len(batches) + 1,
			Documents: docs[i:end],
		}
		batches = append(batches, batch)
	}
	
	return batches
}

// processGroupBatchWorker processes a batch of groups for fuzzy matching
func processGroupBatchWorker(workerID int, db *sql.DB, batchChan <-chan GroupBatch, resultChan chan<- ParallelLayer3Result, debug bool) {
	for batch := range batchChan {
		startTime := time.Now()
		
		if debug {
			fmt.Printf("Worker %d processing group batch %d (%d groups)\n", workerID, batch.ID, len(batch.Groups))
		}
		
		result := ParallelLayer3Result{
			BatchID:       batch.ID,
			ProcessedCount: len(batch.Groups),
			BatchType:     "groups",
		}
		
		// Process each group in the batch
		for _, group := range batch.Groups {
			matches, err := findFuzzyMatchesForGroup(db, group.BestAddressInGroup, false) // Disable debug for parallel processing
			if err != nil {
				continue // Skip failed matches
			}
			
			// Apply reasonable criteria for fuzzy matching
			minSimilarity := 0.5  // Require at least 50% similarity
			maxEditDistance := 25  // Allow reasonable edit distance for address variations
			
			var bestMatch *FuzzyMatchResult
			for _, match := range matches {
				if match.SimilarityScore >= minSimilarity && match.EditDistance <= maxEditDistance {
					if bestMatch == nil || match.SimilarityScore > bestMatch.SimilarityScore {
						bestMatch = &match
					}
				}
			}
			
			if bestMatch != nil {
				// Apply the fuzzy match to all documents in this group
				updateQuery := `
				UPDATE fact_documents_lean 
				SET matched_address_id = $1, 
				    match_method_id = 30,  -- "Group Fuzzy Match"
				    match_confidence_score = $2,
				    match_decision_id = 2  -- "Accepted"
				WHERE document_id IN (
				    SELECT document_id FROM src_document 
				    WHERE planning_app_base = $3
				)
				AND matched_address_id IS NULL  -- Only update unmatched documents
				`
				
				updateResult, err := db.Exec(updateQuery, bestMatch.AddressID, bestMatch.SimilarityScore, group.PlanningAppBase)
				if err != nil {
					if debug {
						fmt.Printf("Worker %d: Error updating group %s: %v\n", workerID, group.PlanningAppBase, err)
					}
					continue
				}
				
				rowsAffected, _ := updateResult.RowsAffected()
				result.MatchCount++
				result.DocumentCount += int(rowsAffected)
				
				if debug {
					fmt.Printf("Worker %d: ✓ Group fuzzy match '%s' -> %d documents updated (sim: %.3f)\n", 
						workerID, group.PlanningAppBase, rowsAffected, bestMatch.SimilarityScore)
				}
			}
		}
		
		result.ProcessTime = time.Since(startTime)
		resultChan <- result
	}
}

// processDocumentBatchWorker processes a batch of documents for fuzzy matching
func processDocumentBatchWorker(workerID int, db *sql.DB, batchChan <-chan DocumentBatch, resultChan chan<- ParallelLayer3Result, debug bool) {
	for batch := range batchChan {
		startTime := time.Now()
		
		if debug {
			fmt.Printf("Worker %d processing document batch %d (%d documents)\n", workerID, batch.ID, len(batch.Documents))
		}
		
		result := ParallelLayer3Result{
			BatchID:       batch.ID,
			ProcessedCount: len(batch.Documents),
			BatchType:     "documents",
		}
		
		// Process each document in the batch
		for _, doc := range batch.Documents {
			matches, err := findFuzzyMatchesForDocument(db, doc.RawAddress, false) // Disable debug for parallel processing
			if err != nil {
				continue // Skip failed matches
			}
			
			// Apply reasonable criteria for fuzzy matching
			minSimilarity := 0.6  // Slightly higher threshold for individual documents
			maxEditDistance := 20  // Tighter edit distance for individual matching
			
			var bestMatch *FuzzyMatchResult
			for _, match := range matches {
				if match.SimilarityScore >= minSimilarity && match.EditDistance <= maxEditDistance {
					if bestMatch == nil || match.SimilarityScore > bestMatch.SimilarityScore {
						bestMatch = &match
					}
				}
			}
			
			if bestMatch != nil {
				// Apply the fuzzy match to this individual document
				updateQuery := `
				UPDATE fact_documents_lean 
				SET matched_address_id = $1, 
				    match_method_id = 31,  -- "Individual Fuzzy Match"
				    match_confidence_score = $2,
				    match_decision_id = 2  -- "Accepted"
				WHERE document_id = $3
				AND matched_address_id IS NULL  -- Only update unmatched documents
				`
				
				updateResult, err := db.Exec(updateQuery, bestMatch.AddressID, bestMatch.SimilarityScore, doc.DocumentID)
				if err != nil {
					if debug {
						fmt.Printf("Worker %d: Error updating document %d: %v\n", workerID, doc.DocumentID, err)
					}
					continue
				}
				
				rowsAffected, _ := updateResult.RowsAffected()
				if rowsAffected > 0 {
					result.MatchCount++
					result.DocumentCount += int(rowsAffected)
					
					if debug {
						fmt.Printf("Worker %d: ✓ Individual fuzzy match doc %d (sim: %.3f)\n", 
							workerID, doc.DocumentID, bestMatch.SimilarityScore)
					}
				}
			}
		}
		
		result.ProcessTime = time.Since(startTime)
		resultChan <- result
	}
}

// findFuzzyMatchesForGroup finds fuzzy matches for a group's best address
func findFuzzyMatchesForGroup(db *sql.DB, bestAddress string, debug bool) ([]FuzzyMatchResult, error) {
	fuzzyMatchSQL := `
	SELECT 
		da.address_id,
		da.uprn,
		da.full_address,
		similarity(da.full_address, $1) as similarity_score,
		levenshtein(UPPER(da.full_address), UPPER($1)) as edit_distance
	FROM dim_address da
	WHERE similarity(da.full_address, $1) >= 0.5  -- Balanced threshold for quality matches
	  AND da.uprn IS NOT NULL
	  AND LENGTH(da.full_address) > 10  -- Filter out incomplete addresses
	ORDER BY similarity_score DESC, edit_distance ASC
	LIMIT 5  -- Top 5 candidates per group (optimized for performance)
	`
	
	rows, err := db.Query(fuzzyMatchSQL, bestAddress)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var matches []FuzzyMatchResult
	for rows.Next() {
		var match FuzzyMatchResult
		if err := rows.Scan(&match.AddressID, &match.UPRN, &match.FullAddress, 
			&match.SimilarityScore, &match.EditDistance); err != nil {
			continue
		}
		matches = append(matches, match)
	}
	
	return matches, nil
}

// findFuzzyMatchesForDocument finds fuzzy matches for an individual document
func findFuzzyMatchesForDocument(db *sql.DB, rawAddress string, debug bool) ([]FuzzyMatchResult, error) {
	fuzzyMatchSQL := `
	SELECT 
		da.address_id,
		da.uprn,
		da.full_address,
		similarity(da.full_address, $1) as similarity_score,
		levenshtein(UPPER(da.full_address), UPPER($1)) as edit_distance
	FROM dim_address da
	WHERE similarity(da.full_address, $1) >= 0.6  -- Higher threshold for individual docs
	  AND da.uprn IS NOT NULL
	  AND LENGTH(da.full_address) > 10  -- Filter out incomplete addresses
	ORDER BY similarity_score DESC, edit_distance ASC
	LIMIT 3  -- Top 3 candidates per document (optimized for performance)
	`
	
	rows, err := db.Query(fuzzyMatchSQL, rawAddress)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var matches []FuzzyMatchResult
	for rows.Next() {
		var match FuzzyMatchResult
		if err := rows.Scan(&match.AddressID, &match.UPRN, &match.FullAddress, 
			&match.SimilarityScore, &match.EditDistance); err != nil {
			continue
		}
		matches = append(matches, match)
	}
	
	return matches, nil
}

// runParallelLayer3Combined runs both Layer 3a and 3b in sequence
func runParallelLayer3Combined(localDebug bool, db *sql.DB) error {
	fmt.Println("Running Complete Parallel Layer 3: Fuzzy Matching Pipeline...")
	fmt.Println("==============================================================")
	
	// Run Layer 3a: Group-based fuzzy matching
	err := runParallelLayer3Groups(localDebug, db)
	if err != nil {
		return fmt.Errorf("Layer 3a failed: %v", err)
	}
	
	fmt.Println("\n" + strings.Repeat("=", 60) + "\n")
	
	// Run Layer 3b: Individual document fuzzy matching
	err = runParallelLayer3Documents(localDebug, db)
	if err != nil {
		return fmt.Errorf("Layer 3b failed: %v", err)
	}
	
	return nil
}