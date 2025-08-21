package main

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// OptimizedGroupLLMMatching is a more efficient version of group LLM matching
func OptimizedGroupLLMMatching(db *sql.DB, localDebug bool) (int, error) {
	fmt.Println("\nPhase 2: Optimized Group LLM Matching...")
	start := time.Now()
	
	// Step 1: Fetch ALL qualifying groups and their data in ONE query
	groupDataSQL := `
WITH group_summary AS (
    -- Find groups with golden records and unmatched addresses
    SELECT 
        s.planning_app_base,
        COUNT(*) as total_docs,
        COUNT(am.document_id) FILTER (WHERE am.confidence_score >= 0.9) as golden_count,
        COUNT(*) FILTER (WHERE am.confidence_score IS NULL OR am.confidence_score = 0) as unmatched_count
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
    WHERE s.planning_app_base IS NOT NULL
      AND amc.document_id IS NULL
    GROUP BY s.planning_app_base
    HAVING COUNT(*) BETWEEN 2 AND 8
      AND COUNT(am.document_id) FILTER (WHERE am.confidence_score >= 0.9) >= 2
      AND COUNT(*) FILTER (WHERE am.confidence_score IS NULL OR am.confidence_score = 0) >= 1
),
golden_records AS (
    -- Get the best golden record for each group
    SELECT DISTINCT ON (s.planning_app_base)
        s.planning_app_base,
        da.address_id as golden_address_id,
        da.full_address as golden_address_text,
        dl.location_id as golden_location_id
    FROM src_document s
    JOIN address_match am ON s.document_id = am.document_id
    JOIN dim_address da ON am.address_id = da.address_id
    LEFT JOIN dim_location dl ON da.location_id = dl.location_id
    WHERE am.confidence_score >= 0.9
      AND s.planning_app_base IN (SELECT planning_app_base FROM group_summary)
    ORDER BY s.planning_app_base, am.confidence_score DESC
),
unmatched_docs AS (
    -- Get all unmatched documents in qualifying groups
    SELECT 
        s.document_id,
        s.planning_app_base,
        s.raw_address
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
    WHERE s.planning_app_base IN (SELECT planning_app_base FROM group_summary)
      AND amc.document_id IS NULL
      AND (am.confidence_score IS NULL OR am.confidence_score = 0)
      AND s.raw_address IS NOT NULL 
      AND s.raw_address != ''
      AND LENGTH(s.raw_address) > 10
)
-- Final result combining everything
SELECT 
    u.document_id,
    u.planning_app_base,
    u.raw_address,
    g.golden_address_id,
    g.golden_address_text,
    g.golden_location_id
FROM unmatched_docs u
JOIN golden_records g ON u.planning_app_base = g.planning_app_base
ORDER BY u.planning_app_base, u.document_id`

	rows, err := db.Query(groupDataSQL)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch group data: %v", err)
	}
	defer rows.Close()
	
	// Collect all work items
	type WorkItem struct {
		DocumentID       int
		PlanningAppBase  string
		RawAddress      string
		GoldenAddressID  int64
		GoldenAddress   string
		GoldenLocationID sql.NullInt64
	}
	
	var workItems []WorkItem
	for rows.Next() {
		var w WorkItem
		var goldenLocationID sql.NullInt64
		err := rows.Scan(&w.DocumentID, &w.PlanningAppBase, &w.RawAddress, 
			&w.GoldenAddressID, &w.GoldenAddress, &goldenLocationID)
		if err != nil {
			continue
		}
		w.GoldenLocationID = goldenLocationID
		workItems = append(workItems, w)
	}
	
	fmt.Printf("Found %d unmatched documents in qualifying groups\n", len(workItems))
	if len(workItems) == 0 {
		return 0, nil
	}
	
	// Step 2: Process work items in parallel
	const numWorkers = 8 // Adjust based on system capacity
	workChan := make(chan WorkItem, len(workItems))
	resultChan := make(chan int, len(workItems))
	var wg sync.WaitGroup
	
	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for work := range workChan {
				// Call LLM for this comparison
				isSame, confidence, err := askLLMAddressSimilarity(
					work.RawAddress, 
					work.GoldenAddress, 
					false, // Don't debug individual calls
				)
				
				if err != nil {
					if localDebug {
						fmt.Printf("Worker %d: LLM error for doc %d: %v\n", 
							workerID, work.DocumentID, err)
					}
					resultChan <- 0
					continue
				}
				
				if isSame && confidence >= 0.8 {
					// Apply the correction
					err = applyGroupCorrectionOptimized(db, work, confidence)
					if err != nil {
						if localDebug {
							fmt.Printf("Worker %d: Failed to apply correction for doc %d: %v\n", 
								workerID, work.DocumentID, err)
						}
						resultChan <- 0
					} else {
						fmt.Printf("  ✓ Group LLM match [%s]: %.40s → %.40s (conf: %.3f)\n", 
							work.PlanningAppBase, work.RawAddress, work.GoldenAddress, confidence)
						resultChan <- 1
					}
				} else {
					resultChan <- 0
				}
			}
		}(i)
	}
	
	// Send work to workers
	for _, work := range workItems {
		workChan <- work
	}
	close(workChan)
	
	// Wait for all workers to finish
	wg.Wait()
	close(resultChan)
	
	// Count successful corrections
	totalCorrections := 0
	for result := range resultChan {
		totalCorrections += result
	}
	
	elapsed := time.Since(start)
	fmt.Printf("Optimized group matching completed in %.2f seconds\n", elapsed.Seconds())
	fmt.Printf("Applied %d group-based LLM corrections\n", totalCorrections)
	
	return totalCorrections, nil
}

// applyGroupCorrectionOptimized applies a group correction more efficiently
func applyGroupCorrectionOptimized(db *sql.DB, work WorkItem, confidence float64) error {
	const groupLLMMethodID = 33
	
	// Ensure method exists (could be done once at startup)
	_, err := db.Exec(`
		INSERT INTO dim_match_method (method_id, method_code, method_name, description)
		VALUES ($1, 'group_llm_similarity', 'Group LLM Address Similarity', 
			'LLM-based address similarity detection within planning groups')
		ON CONFLICT (method_id) DO NOTHING`, groupLLMMethodID)
	if err != nil {
		return err
	}
	
	correctionSQL := `
		INSERT INTO address_match_corrected (
			document_id, original_address_id, original_confidence_score, original_method_id,
			corrected_address_id, corrected_location_id, corrected_confidence_score, 
			corrected_method_id, correction_reason, planning_app_base
		) VALUES ($1, NULL, 0, 1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (document_id) DO UPDATE SET
			corrected_address_id = EXCLUDED.corrected_address_id,
			corrected_location_id = EXCLUDED.corrected_location_id,
			corrected_confidence_score = EXCLUDED.corrected_confidence_score,
			correction_reason = EXCLUDED.correction_reason`
	
	correctionReason := fmt.Sprintf("Group LLM similarity: matched to golden record (conf: %.3f)", confidence)
	
	var locationID interface{}
	if work.GoldenLocationID.Valid {
		locationID = work.GoldenLocationID.Int64
	} else {
		locationID = nil
	}
	
	_, err = db.Exec(correctionSQL, 
		work.DocumentID, work.GoldenAddressID, locationID, 
		confidence, groupLLMMethodID, correctionReason, work.PlanningAppBase)
	
	return err
}