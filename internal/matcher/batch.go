package matcher

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ehdc-llpg/internal/debug"
)

// BatchProcessor handles batch address matching operations
type BatchProcessor struct {
	engine *Engine
	db     *sql.DB
}

// BatchStats tracks batch processing statistics
type BatchStats struct {
	TotalDocuments    int
	ProcessedCount    int
	AutoAcceptCount   int
	NeedsReviewCount  int
	NoMatchCount      int
	ErrorCount        int
	AverageScore      float64
	ProcessingTime    time.Duration
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(engine *Engine, db *sql.DB) *BatchProcessor {
	return &BatchProcessor{
		engine: engine,
		db:     db,
	}
}

// ProcessAllDocuments processes all unmatched documents in the database
func (bp *BatchProcessor) ProcessAllDocuments(localDebug bool, batchSize int) (*BatchStats, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	stats := &BatchStats{}
	
	debug.DebugOutput(localDebug, "Starting batch processing with batch size: %d", batchSize)
	
	// Get total count of unmatched documents
	err := bp.db.QueryRow(`
		SELECT COUNT(*)
		FROM src_document s
		LEFT JOIN address_match m ON m.document_id = s.document_id
		WHERE m.document_id IS NULL
	`).Scan(&stats.TotalDocuments)
	
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Found %d unmatched documents to process", stats.TotalDocuments)
	
	if stats.TotalDocuments == 0 {
		debug.DebugOutput(localDebug, "No documents to process")
		return stats, nil
	}
	
	// Process documents in batches
	offset := 0
	var totalScore float64
	
	for offset < stats.TotalDocuments {
		debug.DebugOutput(localDebug, "Processing batch: %d-%d of %d", 
			offset+1, minInt(offset+batchSize, stats.TotalDocuments), stats.TotalDocuments)
		
		// Get batch of unmatched documents
		inputs, err := bp.getUnmatchedDocuments(offset, batchSize)
		if err != nil {
			debug.DebugOutput(localDebug, "Error getting batch: %v", err)
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
			if stats.ProcessedCount%100 == 0 {
				debug.DebugOutput(localDebug, "Processed %d/%d documents (%.1f%%)", 
					stats.ProcessedCount, stats.TotalDocuments, 
					float64(stats.ProcessedCount)/float64(stats.TotalDocuments)*100)
			}
		}
		
		offset += batchSize
	}
	
	// Calculate final statistics
	stats.ProcessingTime = time.Since(startTime)
	if stats.ProcessedCount > 0 {
		stats.AverageScore = totalScore / float64(stats.ProcessedCount)
	}
	
	debug.DebugOutput(localDebug, "Batch processing complete:")
	debug.DebugOutput(localDebug, "  Total documents: %d", stats.TotalDocuments)
	debug.DebugOutput(localDebug, "  Processed: %d", stats.ProcessedCount)
	debug.DebugOutput(localDebug, "  Auto-accepted: %d (%.1f%%)", stats.AutoAcceptCount, 
		float64(stats.AutoAcceptCount)/float64(stats.ProcessedCount)*100)
	debug.DebugOutput(localDebug, "  Needs review: %d (%.1f%%)", stats.NeedsReviewCount,
		float64(stats.NeedsReviewCount)/float64(stats.ProcessedCount)*100)
	debug.DebugOutput(localDebug, "  No match: %d (%.1f%%)", stats.NoMatchCount,
		float64(stats.NoMatchCount)/float64(stats.ProcessedCount)*100)
	debug.DebugOutput(localDebug, "  Errors: %d (%.1f%%)", stats.ErrorCount,
		float64(stats.ErrorCount)/float64(stats.TotalDocuments)*100)
	debug.DebugOutput(localDebug, "  Average score: %.4f", stats.AverageScore)
	debug.DebugOutput(localDebug, "  Processing time: %v", stats.ProcessingTime)
	
	return stats, nil
}

// ProcessDocumentsByType processes documents of a specific type
func (bp *BatchProcessor) ProcessDocumentsByType(localDebug bool, docType string, batchSize int) (*BatchStats, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	stats := &BatchStats{}
	
	debug.DebugOutput(localDebug, "Processing documents of type: %s", docType)
	
	// Get total count of unmatched documents of this type
	err := bp.db.QueryRow(`
		SELECT COUNT(*)
		FROM src_document s
		INNER JOIN dim_document_type dt ON dt.doc_type_id = s.doc_type_id
		LEFT JOIN address_match m ON m.document_id = s.document_id
		WHERE dt.type_code = $1 AND m.document_id IS NULL
	`, docType).Scan(&stats.TotalDocuments)
	
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Found %d unmatched %s documents", stats.TotalDocuments, docType)
	
	if stats.TotalDocuments == 0 {
		return stats, nil
	}
	
	// Get all documents of this type
	inputs, err := bp.getUnmatchedDocumentsByType(docType)
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
			debug.DebugOutput(localDebug, "Processed %d/%d %s documents", i+1, len(inputs), docType)
		}
	}
	
	// Calculate final statistics
	stats.ProcessingTime = time.Since(startTime)
	if stats.ProcessedCount > 0 {
		stats.AverageScore = totalScore / float64(stats.ProcessedCount)
	}
	
	return stats, nil
}

// getUnmatchedDocuments retrieves a batch of unmatched documents
func (bp *BatchProcessor) getUnmatchedDocuments(offset, limit int) ([]MatchInput, error) {
	rows, err := bp.db.Query(`
		SELECT 
			s.document_id, s.raw_address, s.address_canonical,
			s.raw_uprn, s.raw_easting, s.raw_northing
		FROM src_document s
		LEFT JOIN address_match m ON m.document_id = s.document_id
		WHERE m.document_id IS NULL
		  AND s.raw_address IS NOT NULL
		  AND s.raw_address != ''
		ORDER BY s.document_id
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

// getUnmatchedDocumentsByType retrieves all unmatched documents of a specific type
func (bp *BatchProcessor) getUnmatchedDocumentsByType(docType string) ([]MatchInput, error) {
	rows, err := bp.db.Query(`
		SELECT 
			s.document_id, s.raw_address, s.address_canonical,
			s.raw_uprn, s.raw_easting, s.raw_northing
		FROM src_document s
		INNER JOIN dim_document_type dt ON dt.doc_type_id = s.doc_type_id
		LEFT JOIN address_match m ON m.document_id = s.document_id
		WHERE dt.type_code = $1 
		  AND m.document_id IS NULL
		  AND s.raw_address IS NOT NULL
		  AND s.raw_address != ''
		ORDER BY s.document_id
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

// GetMatchingStatistics returns overall matching statistics
func (bp *BatchProcessor) GetMatchingStatistics(localDebug bool) (*MatchingStatistics, error) {
	stats := &MatchingStatistics{}
	
	// Total documents
	err := bp.db.QueryRow("SELECT COUNT(*) FROM src_document").Scan(&stats.TotalDocuments)
	if err != nil {
		return nil, err
	}
	
	// Matched documents
	err = bp.db.QueryRow("SELECT COUNT(*) FROM address_match").Scan(&stats.MatchedDocuments)
	if err != nil {
		return nil, err
	}
	
	// Unmatched documents
	stats.UnmatchedDocuments = stats.TotalDocuments - stats.MatchedDocuments
	
	// Match status breakdown
	rows, err := bp.db.Query(`
		SELECT match_status, COUNT(*) 
		FROM address_match 
		GROUP BY match_status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	stats.StatusBreakdown = make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		err := rows.Scan(&status, &count)
		if err != nil {
			continue
		}
		stats.StatusBreakdown[status] = count
	}
	
	// Method breakdown
	rows, err = bp.db.Query(`
		SELECT dm.method_name, COUNT(*), AVG(am.confidence_score)
		FROM address_match am
		INNER JOIN dim_match_method dm ON dm.method_id = am.match_method_id
		GROUP BY dm.method_id, dm.method_name
		ORDER BY COUNT(*) DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	stats.MethodBreakdown = make(map[string]MethodStats)
	for rows.Next() {
		var methodName string
		var count int
		var avgScore float64
		err := rows.Scan(&methodName, &count, &avgScore)
		if err != nil {
			continue
		}
		stats.MethodBreakdown[methodName] = MethodStats{
			Count:    count,
			AvgScore: avgScore,
		}
	}
	
	// Average confidence score
	err = bp.db.QueryRow("SELECT AVG(confidence_score) FROM address_match").Scan(&stats.AverageConfidence)
	if err != nil {
		stats.AverageConfidence = 0.0
	}
	
	return stats, nil
}

// MatchingStatistics holds overall matching statistics
type MatchingStatistics struct {
	TotalDocuments     int
	MatchedDocuments   int
	UnmatchedDocuments int
	AverageConfidence  float64
	StatusBreakdown    map[string]int
	MethodBreakdown    map[string]MethodStats
}

// MethodStats holds statistics for a specific matching method
type MethodStats struct {
	Count    int
	AvgScore float64
}

// Helper function
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}