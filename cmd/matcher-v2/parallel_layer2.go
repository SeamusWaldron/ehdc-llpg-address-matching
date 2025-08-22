package main

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// ParallelMatchResult holds the result of a batch processing
type ParallelMatchResult struct {
	BatchID       int
	AddressCount  int
	MatchCount    int
	DocumentCount int
	ProcessTime   time.Duration
	Error         error
}

// AddressBatch represents a batch of addresses to process
type AddressBatch struct {
	ID        int
	Addresses []struct {
		rawAddress        string
		documentCount     int
		originalAddressID int
	}
}

// runParallelLayer2 runs Layer 2 with parallel batch processing
func runParallelLayer2(localDebug bool, db *sql.DB) error {
	fmt.Println("Running Parallel Layer 2: Multi-threaded Conservative Matching...")
	fmt.Println("===============================================================")
	
	// First optimize the data structures
	err := optimizeLayer2Performance(localDebug, db)
	if err != nil {
		return fmt.Errorf("failed to optimize structures: %v", err)
	}
	
	// Get distinct unmatched addresses with better filtering
	fmt.Println("\nFinding distinct unmatched addresses...")
	
	distinctQuery := `
	SELECT 
		o.raw_address,
		COUNT(*) as document_count,
		o.original_address_id
	FROM fact_documents_lean f
	JOIN dim_original_address o ON f.original_address_id = o.original_address_id
	WHERE f.matched_address_id IS NULL
	GROUP BY o.raw_address, o.original_address_id
	HAVING COUNT(*) BETWEEN 1 AND 20  -- Focus on manageable frequency addresses
	  AND LENGTH(o.raw_address) BETWEEN 15 AND 100  -- Reasonable length addresses
	  AND o.raw_address NOT LIKE 'F%'    -- Exclude F-codes
	  AND o.raw_address NOT LIKE 'PRD%'  -- Exclude planning reference codes
	  AND o.raw_address NOT LIKE 'N/A%'  -- Exclude N/A entries
	  AND o.raw_address NOT LIKE '%AND %' -- Exclude multi-property entries
	  AND o.raw_address NOT LIKE '%PLOT %' -- Exclude plot references
	  AND o.raw_address NOT LIKE '%DEV%'   -- Exclude development references
	  AND o.raw_address ~ '^[0-9]+[A-Z]?\s'  -- Must start with house number
	  AND o.raw_address ~ '^[^,]+,[^,]+,'    -- Must have at least 2 commas (street, locality)
	ORDER BY 
	  document_count DESC,  -- Process high-impact addresses first
	  LENGTH(o.raw_address) ASC  -- Prefer shorter, simpler addresses
	LIMIT 10000  -- Process top 10,000 quality addresses
	`
	
	rows, err := db.Query(distinctQuery)
	if err != nil {
		return fmt.Errorf("failed to get distinct addresses: %v", err)
	}
	defer rows.Close()
	
	var allAddresses []struct {
		rawAddress        string
		documentCount     int
		originalAddressID int
	}
	
	for rows.Next() {
		var addr struct {
			rawAddress        string
			documentCount     int
			originalAddressID int
		}
		if err := rows.Scan(&addr.rawAddress, &addr.documentCount, &addr.originalAddressID); err != nil {
			continue
		}
		allAddresses = append(allAddresses, addr)
	}
	
	fmt.Printf("Found %d distinct quality addresses to process\n", len(allAddresses))
	
	if len(allAddresses) == 0 {
		fmt.Println("No suitable addresses found for parallel processing")
		return nil
	}
	
	// Create batches for parallel processing
	batchSize := 100  // Process 100 addresses per batch
	numWorkers := getOptimalWorkerCount()  // Auto-detect optimal worker count
	
	batches := createAddressBatches(allAddresses, batchSize)
	fmt.Printf("Created %d batches of ~%d addresses each for %d parallel workers\n", 
		len(batches), batchSize, numWorkers)
	
	// Set up parallel processing
	batchChan := make(chan AddressBatch, len(batches))
	resultChan := make(chan ParallelMatchResult, len(batches))
	
	// Send all batches to the channel
	for _, batch := range batches {
		batchChan <- batch
	}
	close(batchChan)
	
	// Start worker goroutines
	var wg sync.WaitGroup
	startTime := time.Now()
	
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processBatchWorker(workerID, db, batchChan, resultChan, localDebug)
		}(i)
	}
	
	// Wait for all workers to complete
	wg.Wait()
	close(resultChan)
	
	// Collect and summarize results
	var totalMatches, totalDocuments, totalProcessed int
	var totalBatches int
	
	fmt.Println("\n=== PARALLEL PROCESSING RESULTS ===")
	for result := range resultChan {
		totalBatches++
		totalProcessed += result.AddressCount
		totalMatches += result.MatchCount
		totalDocuments += result.DocumentCount
		
		if result.Error != nil {
			fmt.Printf("Batch %d: ERROR - %v\n", result.BatchID, result.Error)
		} else {
			fmt.Printf("Batch %d: %d addresses, %d matches, %d documents updated (%.1fs)\n", 
				result.BatchID, result.AddressCount, result.MatchCount, result.DocumentCount, result.ProcessTime.Seconds())
		}
	}
	
	totalTime := time.Since(startTime)
	
	fmt.Printf("\n=== PARALLEL LAYER 2 SUMMARY ===\n")
	fmt.Printf("Total batches processed: %d\n", totalBatches)
	fmt.Printf("Total addresses processed: %d\n", totalProcessed)
	fmt.Printf("Successful matches: %d\n", totalMatches)
	fmt.Printf("Documents updated: %d\n", totalDocuments)
	fmt.Printf("Overall match rate: %.1f%%\n", float64(totalMatches)/float64(totalProcessed)*100)
	fmt.Printf("Total processing time: %.1f seconds\n", totalTime.Seconds())
	fmt.Printf("Average throughput: %.1f addresses/second\n", float64(totalProcessed)/totalTime.Seconds())
	
	if totalMatches > 0 {
		fmt.Printf("Average impact: %.1f documents per successful match\n", float64(totalDocuments)/float64(totalMatches))
	}
	
	return nil
}

// createAddressBatches splits addresses into batches for parallel processing
func createAddressBatches(addresses []struct {
	rawAddress        string
	documentCount     int
	originalAddressID int
}, batchSize int) []AddressBatch {
	var batches []AddressBatch
	
	for i := 0; i < len(addresses); i += batchSize {
		end := i + batchSize
		if end > len(addresses) {
			end = len(addresses)
		}
		
		batch := AddressBatch{
			ID:        len(batches) + 1,
			Addresses: addresses[i:end],
		}
		batches = append(batches, batch)
	}
	
	return batches
}

// processBatchWorker is a worker goroutine that processes batches of addresses
func processBatchWorker(workerID int, db *sql.DB, batchChan <-chan AddressBatch, resultChan chan<- ParallelMatchResult, debug bool) {
	for batch := range batchChan {
		startTime := time.Now()
		
		if debug {
			fmt.Printf("Worker %d processing batch %d (%d addresses)\n", workerID, batch.ID, len(batch.Addresses))
		}
		
		result := ParallelMatchResult{
			BatchID:      batch.ID,
			AddressCount: len(batch.Addresses),
		}
		
		// Process each address in the batch
		for _, addr := range batch.Addresses {
			matchedAddressID, confidence, err := tryOptimizedMatch(db, addr.rawAddress, false) // Disable debug for parallel processing
			if err != nil {
				continue // Skip failed matches
			}
			
			if matchedAddressID > 0 {
				// Update ALL documents with this address
				updateQuery := `
				UPDATE fact_documents_lean 
				SET matched_address_id = $1, 
				    match_method_id = 5,  -- "Parallel Optimized Match"
				    match_confidence_score = $2,
				    match_decision_id = 2  -- "Accepted"
				WHERE original_address_id = $3 
				  AND matched_address_id IS NULL
				`
				
				updateResult, err := db.Exec(updateQuery, matchedAddressID, confidence, addr.originalAddressID)
				if err != nil {
					if debug {
						fmt.Printf("Worker %d: Error updating documents for address '%s': %v\n", 
							workerID, addr.rawAddress, err)
					}
					continue
				}
				
				rowsAffected, _ := updateResult.RowsAffected()
				result.MatchCount++
				result.DocumentCount += int(rowsAffected)
				
				if debug {
					fmt.Printf("Worker %d: ✓ Matched '%s' -> updated %d documents\n", 
						workerID, addr.rawAddress, rowsAffected)
				}
			}
		}
		
		result.ProcessTime = time.Since(startTime)
		resultChan <- result
	}
}

// Additional optimization: Create dedicated connection pools for parallel processing
func createConnectionPool(baseDB *sql.DB, poolSize int) ([]*sql.DB, error) {
	var connections []*sql.DB
	
	// Get connection string from base connection (simplified approach)
	connStr := "postgres://postgres:kljh234hjkl2h@localhost:15435/ehdc_llpg?sslmode=disable"
	
	for i := 0; i < poolSize; i++ {
		conn, err := sql.Open("postgres", connStr)
		if err != nil {
			// Close any connections we've already opened
			for _, c := range connections {
				c.Close()
			}
			return nil, fmt.Errorf("failed to create connection %d: %v", i, err)
		}
		
		// Test the connection
		if err := conn.Ping(); err != nil {
			conn.Close()
			for _, c := range connections {
				c.Close()
			}
			return nil, fmt.Errorf("failed to ping connection %d: %v", i, err)
		}
		
		// Set connection pool settings for optimal parallel performance
		conn.SetMaxOpenConns(1)  // Each connection in pool handles 1 concurrent connection
		conn.SetMaxIdleConns(1)
		conn.SetConnMaxLifetime(time.Hour)
		
		connections = append(connections, conn)
	}
	
	return connections, nil
}

// Enhanced parallel processing with dedicated connection pools
func runParallelLayer2WithConnectionPool(localDebug bool, db *sql.DB) error {
	fmt.Println("Running Enhanced Parallel Layer 2 with Connection Pooling...")
	fmt.Println("===========================================================")
	
	numWorkers := getOptimalWorkerCount()  // Auto-detect optimal worker count
	
	// Create dedicated connection pool
	connections, err := createConnectionPool(db, numWorkers)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %v", err)
	}
	defer func() {
		for _, conn := range connections {
			conn.Close()
		}
	}()
	
	fmt.Printf("Created connection pool with %d dedicated connections\n", len(connections))
	
	// Rest of the implementation would use the dedicated connections
	// for even better parallel performance...
	
	return runParallelLayer2(localDebug, db) // For now, use the standard parallel implementation
}