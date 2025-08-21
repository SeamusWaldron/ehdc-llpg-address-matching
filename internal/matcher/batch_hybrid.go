package matcher

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ehdc-llpg/internal/debug"
)

// HybridBatchProcessor handles high-performance batch processing with advanced analysis
type HybridBatchProcessor struct {
	engine *HybridEngine
	db     *sql.DB
}

// HybridBatchStats extends BatchStats with additional hybrid metrics
type HybridBatchStats struct {
	BatchStats
	VeryHighConfidenceCount int
	HighConfidenceCount     int
	MediumConfidenceCount   int
	LowConfidenceCount      int
	VeryLowConfidenceCount  int
	TokenMatchCount         int
	SpatialMatchCount       int
	SemanticMatchCount      int
	HybridBoostAverage      float64
}

// NewHybridBatchProcessor creates a new hybrid batch processor
func NewHybridBatchProcessor(engine *HybridEngine, db *sql.DB) *HybridBatchProcessor {
	return &HybridBatchProcessor{
		engine: engine,
		db:     db,
	}
}

// ProcessAllDocuments processes all unmatched documents with hybrid analysis
func (bp *HybridBatchProcessor) ProcessAllDocuments(localDebug bool, batchSize int) (*HybridBatchStats, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	stats := &HybridBatchStats{}
	
	debug.DebugOutput(localDebug, "Starting HYBRID batch processing with batch size: %d", batchSize)
	
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
	
	debug.DebugOutput(localDebug, "Found %d unmatched documents for hybrid processing", stats.TotalDocuments)
	
	if stats.TotalDocuments == 0 {
		debug.DebugOutput(localDebug, "No documents to process")
		return stats, nil
	}
	
	// Process documents in batches
	offset := 0
	var totalScore float64
	var totalHybridBoost float64
	hybridBoostCount := 0
	
	for offset < stats.TotalDocuments {
		debug.DebugOutput(localDebug, "Processing HYBRID batch: %d-%d of %d", 
			offset+1, minInt(offset+batchSize, stats.TotalDocuments), stats.TotalDocuments)
		
		// Get batch from materialized view
		inputs, err := bp.getHybridUnmatchedDocuments(offset, batchSize)
		if err != nil {
			debug.DebugOutput(localDebug, "Error getting hybrid batch: %v", err)
			stats.ErrorCount++
			offset += batchSize
			continue
		}
		
		// Process each document with hybrid analysis
		for _, input := range inputs {
			result, err := bp.engine.ProcessDocument(false, input) // Disable debug for batch
			if err != nil {
				debug.DebugOutput(localDebug, "Error processing document %d: %v", input.DocumentID, err)
				stats.ErrorCount++
				continue
			}
			
			// Save result using hybrid engine
			err = bp.engine.SaveMatchResult(false, result)
			if err != nil {
				debug.DebugOutput(localDebug, "Error saving result for document %d: %v", input.DocumentID, err)
				stats.ErrorCount++
				continue
			}
			
			// Update standard statistics
			stats.ProcessedCount++
			if result.BestCandidate != nil {
				totalScore += result.BestCandidate.Score
				
				// Extract hybrid-specific metrics
				bp.updateHybridStats(result, stats, &totalHybridBoost, &hybridBoostCount)
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
			if stats.ProcessedCount%1000 == 0 {
				debug.DebugOutput(localDebug, "HYBRID progress: %d/%d documents (%.1f%%) - %.1f docs/sec", 
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
	if hybridBoostCount > 0 {
		stats.HybridBoostAverage = totalHybridBoost / float64(hybridBoostCount)
	}
	
	debug.DebugOutput(localDebug, "HYBRID batch processing complete:")
	debug.DebugOutput(localDebug, "  Total documents: %d", stats.TotalDocuments)
	debug.DebugOutput(localDebug, "  Processed: %d", stats.ProcessedCount)
	debug.DebugOutput(localDebug, "  Processing rate: %.1f docs/sec", 
		float64(stats.ProcessedCount)/stats.ProcessingTime.Seconds())
	debug.DebugOutput(localDebug, "  Confidence breakdown: VH=%d, H=%d, M=%d, L=%d, VL=%d",
		stats.VeryHighConfidenceCount, stats.HighConfidenceCount, stats.MediumConfidenceCount,
		stats.LowConfidenceCount, stats.VeryLowConfidenceCount)
	debug.DebugOutput(localDebug, "  Advanced matching: Token=%d, Spatial=%d, Semantic=%d",
		stats.TokenMatchCount, stats.SpatialMatchCount, stats.SemanticMatchCount)
	debug.DebugOutput(localDebug, "  Average hybrid boost: %.4f", stats.HybridBoostAverage)
	
	return stats, nil
}

// ProcessDocumentsByType processes documents of a specific type with hybrid analysis
func (bp *HybridBatchProcessor) ProcessDocumentsByType(localDebug bool, docType string, batchSize int) (*HybridBatchStats, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	stats := &HybridBatchStats{}
	
	debug.DebugOutput(localDebug, "Processing HYBRID documents of type: %s", docType)
	
	// Refresh materialized view first
	_, err := bp.db.Exec("REFRESH MATERIALIZED VIEW mv_unmatched_documents")
	if err != nil {
		return nil, fmt.Errorf("failed to refresh unmatched documents view: %w", err)
	}
	
	// Get total count of unmatched documents of this type
	err = bp.db.QueryRow(`
		SELECT COUNT(*) FROM mv_unmatched_documents WHERE type_code = $1
	`, docType).Scan(&stats.TotalDocuments)
	
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Found %d unmatched %s documents for hybrid processing", stats.TotalDocuments, docType)
	
	if stats.TotalDocuments == 0 {
		return stats, nil
	}
	
	// Get all documents of this type from materialized view
	inputs, err := bp.getHybridUnmatchedDocumentsByType(docType)
	if err != nil {
		return nil, fmt.Errorf("failed to get documents: %w", err)
	}
	
	// Process documents with hybrid analysis
	var totalScore float64
	var totalHybridBoost float64
	hybridBoostCount := 0
	
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
			
			// Extract hybrid-specific metrics
			bp.updateHybridStats(result, stats, &totalHybridBoost, &hybridBoostCount)
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
		if (i+1)%200 == 0 {
			debug.DebugOutput(localDebug, "HYBRID processed %d/%d %s documents (%.1f docs/sec)", 
				i+1, len(inputs), docType, float64(i+1)/time.Since(startTime).Seconds())
		}
	}
	
	// Calculate final statistics
	stats.ProcessingTime = time.Since(startTime)
	if stats.ProcessedCount > 0 {
		stats.AverageScore = totalScore / float64(stats.ProcessedCount)
	}
	if hybridBoostCount > 0 {
		stats.HybridBoostAverage = totalHybridBoost / float64(hybridBoostCount)
	}
	
	return stats, nil
}

// updateHybridStats extracts and updates hybrid-specific statistics
func (bp *HybridBatchProcessor) updateHybridStats(result *MatchResult, stats *HybridBatchStats, totalHybridBoost *float64, hybridBoostCount *int) {
	if result.BestCandidate == nil || result.BestCandidate.Features == nil {
		return
	}
	
	// Extract confidence level
	if confidence, ok := result.BestCandidate.Features["confidence"].(string); ok {
		switch confidence {
		case "very_high":
			stats.VeryHighConfidenceCount++
		case "high":
			stats.HighConfidenceCount++
		case "medium":
			stats.MediumConfidenceCount++
		case "low":
			stats.LowConfidenceCount++
		case "very_low":
			stats.VeryLowConfidenceCount++
		}
	}
	
	// Extract token analysis
	if tokenAnalysis, ok := result.BestCandidate.Features["token_analysis"].(TokenAnalysis); ok {
		if tokenAnalysis.HouseNumberMatch || tokenAnalysis.LocalityMatch || tokenAnalysis.PostcodeMatch {
			stats.TokenMatchCount++
		}
	}
	
	// Extract spatial analysis
	if spatialAnalysis, ok := result.BestCandidate.Features["spatial_analysis"].(SpatialAnalysis); ok {
		if spatialAnalysis.HasCoordinates && spatialAnalysis.WithinRadius {
			stats.SpatialMatchCount++
		}
	}
	
	// Calculate hybrid boost (difference between hybrid and prefilter scores)
	if hybridScore, ok := result.BestCandidate.Features["hybrid_score"].(float64); ok {
		if prefilterScore, ok := result.BestCandidate.Features["prefilter_score"].(float64); ok {
			boost := hybridScore - prefilterScore
			if boost > 0 {
				*totalHybridBoost += boost
				*hybridBoostCount++
			}
		}
	}
}

// getHybridUnmatchedDocuments retrieves a batch from the materialized view
func (bp *HybridBatchProcessor) getHybridUnmatchedDocuments(offset, limit int) ([]MatchInput, error) {
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

// getHybridUnmatchedDocumentsByType retrieves all unmatched documents of a specific type
func (bp *HybridBatchProcessor) getHybridUnmatchedDocumentsByType(docType string) ([]MatchInput, error) {
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