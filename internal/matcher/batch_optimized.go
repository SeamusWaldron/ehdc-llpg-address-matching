package matcher

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ehdc-llpg/internal/debug"
)

// OptimizedBatchProcessor handles high-performance batch address matching
type OptimizedBatchProcessor struct {
	engine *OptimizedEngine
	db     *sql.DB
}

// NewOptimizedBatchProcessor creates a new optimized batch processor
func NewOptimizedBatchProcessor(engine *OptimizedEngine, db *sql.DB) *OptimizedBatchProcessor {
	return &OptimizedBatchProcessor{
		engine: engine,
		db:     db,
	}
}

// ProcessAllDocuments processes all unmatched documents using optimized queries
func (bp *OptimizedBatchProcessor) ProcessAllDocuments(localDebug bool, batchSize int) (*BatchStats, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	stats := &BatchStats{}
	
	debug.DebugOutput(localDebug, "Starting optimized batch processing with batch size: %d", batchSize)
	
	// Refresh materialized view for latest unmatched documents
	_, err := bp.db.Exec("REFRESH MATERIALIZED VIEW mv_unmatched_documents")
	if err != nil {
		return nil, fmt.Errorf("failed to refresh unmatched documents view: %w", err)
	}
	
	// Get total count from materialized view
	err = bp.db.QueryRow("SELECT COUNT(*) FROM mv_unmatched_documents").Scan(&stats.TotalDocuments)
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Found %d unmatched documents to process", stats.TotalDocuments)
	
	if stats.TotalDocuments == 0 {
		debug.DebugOutput(localDebug, "No documents to process")
		return stats, nil
	}
	
	// Process documents in batches using the materialized view
	offset := 0
	var totalScore float64
	
	for offset < stats.TotalDocuments {
		debug.DebugOutput(localDebug, "Processing optimized batch: %d-%d of %d", 
			offset+1, minInt(offset+batchSize, stats.TotalDocuments), stats.TotalDocuments)
		
		// Get batch from materialized view (much faster)
		inputs, err := bp.getOptimizedUnmatchedDocuments(offset, batchSize)
		if err != nil {
			debug.DebugOutput(localDebug, "Error getting optimized batch: %v", err)
			stats.ErrorCount++
			offset += batchSize
			continue
		}
		
		// Process each document in the batch
		for _, input := range inputs {
			result, err := bp.engine.ProcessDocument(false, input) // Disable debug for batch processing
			if err != nil {
				debug.DebugOutput(localDebug, "Error processing document %d: %v", input.DocumentID, err)
				stats.ErrorCount++
				continue
			}
			
			// Save result using optimized engine
			err = bp.engine.SaveMatchResult(false, result)
			if err != nil {
				debug.DebugOutput(localDebug, "Error saving result for document %d: %v", input.DocumentID, err)
				stats.ErrorCount++
				continue
			}
			
			// Update statistics
			stats.ProcessedCount++
			if result.BestCandidate != nil {
				totalScore += result.BestCandidate.Score
			}
			
			switch result.Decision {
			case "auto_accept":
				stats.AutoAcceptCount++
			case "needs_review", "low_confidence":
				stats.NeedsReviewCount++
			case "no_match":
				stats.NoMatchCount++
			}
			
			// Progress reporting (less frequent for performance)
			if stats.ProcessedCount%500 == 0 {
				debug.DebugOutput(localDebug, "Optimized progress: %d/%d documents (%.1f%%) - %.1f docs/sec", 
					stats.ProcessedCount, stats.TotalDocuments, 
					float64(stats.ProcessedCount)/float64(stats.TotalDocuments)*100,
					float64(stats.ProcessedCount)/time.Since(startTime).Seconds())
			}
		}
		
		offset += batchSize
	}
	
	// Calculate final statistics
	stats.ProcessingTime = time.Since(startTime)
	if stats.ProcessedCount > 0 {
		stats.AverageScore = totalScore / float64(stats.ProcessedCount)
	}
	
	debug.DebugOutput(localDebug, "Optimized batch processing complete:")
	debug.DebugOutput(localDebug, "  Total documents: %d", stats.TotalDocuments)
	debug.DebugOutput(localDebug, "  Processed: %d", stats.ProcessedCount)
	debug.DebugOutput(localDebug, "  Processing rate: %.1f docs/sec", 
		float64(stats.ProcessedCount)/stats.ProcessingTime.Seconds())
	debug.DebugOutput(localDebug, "  Auto-accepted: %d (%.1f%%)", stats.AutoAcceptCount, 
		float64(stats.AutoAcceptCount)/float64(stats.ProcessedCount)*100)
	debug.DebugOutput(localDebug, "  Needs review: %d (%.1f%%)", stats.NeedsReviewCount,
		float64(stats.NeedsReviewCount)/float64(stats.ProcessedCount)*100)
	debug.DebugOutput(localDebug, "  No match: %d (%.1f%%)", stats.NoMatchCount,
		float64(stats.NoMatchCount)/float64(stats.ProcessedCount)*100)
	debug.DebugOutput(localDebug, "  Average score: %.4f", stats.AverageScore)
	debug.DebugOutput(localDebug, "  Processing time: %v", stats.ProcessingTime)
	
	return stats, nil
}

// ProcessDocumentsByType processes documents of a specific type using optimized queries
func (bp *OptimizedBatchProcessor) ProcessDocumentsByType(localDebug bool, docType string, batchSize int) (*BatchStats, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	stats := &BatchStats{}
	
	debug.DebugOutput(localDebug, "Processing optimized documents of type: %s", docType)
	
	// Refresh materialized view first
	_, err := bp.db.Exec("REFRESH MATERIALIZED VIEW mv_unmatched_documents")
	if err != nil {
		return nil, fmt.Errorf("failed to refresh unmatched documents view: %w", err)
	}
	
	// Get total count of unmatched documents of this type from materialized view
	err = bp.db.QueryRow(`
		SELECT COUNT(*) FROM mv_unmatched_documents WHERE type_code = $1
	`, docType).Scan(&stats.TotalDocuments)
	
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Found %d unmatched %s documents", stats.TotalDocuments, docType)
	
	if stats.TotalDocuments == 0 {
		return stats, nil
	}
	
	// Get all documents of this type from materialized view
	inputs, err := bp.getOptimizedUnmatchedDocumentsByType(docType)
	if err != nil {
		return nil, fmt.Errorf("failed to get documents: %w", err)
	}
	
	// Process documents
	var totalScore float64
	for i, input := range inputs {
		result, err := bp.engine.ProcessDocument(false, input)
		if err != nil {
			debug.DebugOutput(localDebug, "Error processing document %d: %v", input.DocumentID, err)
			stats.ErrorCount++
			continue
		}
		
		// Save result
		err = bp.engine.SaveMatchResult(false, result)
		if err != nil {
			debug.DebugOutput(localDebug, "Error saving result for document %d: %v", input.DocumentID, err)
			stats.ErrorCount++
			continue
		}
		
		// Update statistics
		stats.ProcessedCount++
		if result.BestCandidate != nil {
			totalScore += result.BestCandidate.Score
		}
		
		switch result.Decision {
		case "auto_accept":
			stats.AutoAcceptCount++
		case "needs_review", "low_confidence":
			stats.NeedsReviewCount++
		case "no_match":
			stats.NoMatchCount++
		}
		
		// Progress reporting
		if (i+1)%100 == 0 {
			debug.DebugOutput(localDebug, "Processed %d/%d %s documents (%.1f docs/sec)", 
				i+1, len(inputs), docType, float64(i+1)/time.Since(startTime).Seconds())
		}
	}
	
	// Calculate final statistics
	stats.ProcessingTime = time.Since(startTime)
	if stats.ProcessedCount > 0 {
		stats.AverageScore = totalScore / float64(stats.ProcessedCount)
	}
	
	return stats, nil
}

// getOptimizedUnmatchedDocuments retrieves a batch from the materialized view
func (bp *OptimizedBatchProcessor) getOptimizedUnmatchedDocuments(offset, limit int) ([]MatchInput, error) {
	rows, err := bp.db.Query(`
		SELECT 
			document_id, raw_address, address_canonical,
			raw_uprn, raw_easting, raw_northing
		FROM mv_unmatched_documents
		ORDER BY document_id
		LIMIT $1 OFFSET $2
	`, limit, offset)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var inputs []MatchInput
	for rows.Next() {
		var input MatchInput
		var rawUPRN, rawEasting, rawNorthing sql.NullString
		
		err := rows.Scan(
			&input.DocumentID, &input.RawAddress, &input.AddressCanonical,
			&rawUPRN, &rawEasting, &rawNorthing,
		)
		if err != nil {
			continue
		}
		
		if rawUPRN.Valid && rawUPRN.String != "" {
			input.RawUPRN = &rawUPRN.String
		}
		if rawEasting.Valid && rawEasting.String != "" {
			input.RawEasting = &rawEasting.String
		}
		if rawNorthing.Valid && rawNorthing.String != "" {
			input.RawNorthing = &rawNorthing.String
		}
		
		inputs = append(inputs, input)
	}
	
	return inputs, nil
}

// getOptimizedUnmatchedDocumentsByType retrieves all unmatched documents of a specific type
func (bp *OptimizedBatchProcessor) getOptimizedUnmatchedDocumentsByType(docType string) ([]MatchInput, error) {
	rows, err := bp.db.Query(`
		SELECT 
			document_id, raw_address, address_canonical,
			raw_uprn, raw_easting, raw_northing
		FROM mv_unmatched_documents
		WHERE type_code = $1
		ORDER BY document_id
	`, docType)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var inputs []MatchInput
	for rows.Next() {
		var input MatchInput
		var rawUPRN, rawEasting, rawNorthing sql.NullString
		
		err := rows.Scan(
			&input.DocumentID, &input.RawAddress, &input.AddressCanonical,
			&rawUPRN, &rawEasting, &rawNorthing,
		)
		if err != nil {
			continue
		}
		
		if rawUPRN.Valid && rawUPRN.String != "" {
			input.RawUPRN = &rawUPRN.String
		}
		if rawEasting.Valid && rawEasting.String != "" {
			input.RawEasting = &rawEasting.String
		}
		if rawNorthing.Valid && rawNorthing.String != "" {
			input.RawNorthing = &rawNorthing.String
		}
		
		inputs = append(inputs, input)
	}
	
	return inputs, nil
}