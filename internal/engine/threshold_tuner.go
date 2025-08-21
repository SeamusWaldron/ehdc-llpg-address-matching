package engine

import (
	"database/sql"
	"fmt"
	"math"
	"time"
)

// ThresholdTuner helps find optimal similarity thresholds
type ThresholdTuner struct {
	db *sql.DB
}

// NewThresholdTuner creates a new threshold tuner
func NewThresholdTuner(db *sql.DB) *ThresholdTuner {
	return &ThresholdTuner{db: db}
}

// TuningResult holds the results of threshold tuning
type TuningResult struct {
	Threshold        float64
	TruePositives    int
	FalsePositives   int
	TrueNegatives    int
	FalseNegatives   int
	Precision        float64
	Recall           float64
	F1Score          float64
	ProcessingTime   time.Duration
	CandidatesFound  int
	AutoAcceptCount  int
	ReviewCount      int
}

// TestThresholds tests different similarity thresholds to find optimal settings
func (tt *ThresholdTuner) TestThresholds(sampleSize int) ([]*TuningResult, error) {
	// Test different threshold values
	thresholds := []float64{0.50, 0.55, 0.60, 0.65, 0.70, 0.75, 0.80, 0.85, 0.90}
	results := make([]*TuningResult, 0, len(thresholds))

	fmt.Println("\n=== Starting Threshold Tuning Analysis ===")
	fmt.Printf("Testing %d thresholds with sample size %d\n\n", len(thresholds), sampleSize)

	// Get sample of documents with known good matches (using existing accepted matches as ground truth)
	knownGood, err := tt.getKnownGoodMatches(sampleSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get known good matches: %w", err)
	}

	fmt.Printf("Found %d documents with known good matches for validation\n\n", len(knownGood))

	for _, threshold := range thresholds {
		fmt.Printf("Testing threshold %.2f...\n", threshold)
		result, err := tt.testThreshold(threshold, knownGood)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			continue
		}

		results = append(results, result)
		
		fmt.Printf("  Precision: %.2f%%, Recall: %.2f%%, F1: %.3f\n",
			result.Precision*100, result.Recall*100, result.F1Score)
		fmt.Printf("  Auto-accept: %d, Review: %d, Time: %.2fs\n\n",
			result.AutoAcceptCount, result.ReviewCount, result.ProcessingTime.Seconds())
	}

	// Find and recommend optimal threshold
	optimal := tt.findOptimalThreshold(results)
	if optimal != nil {
		fmt.Printf("\n=== RECOMMENDATION ===\n")
		fmt.Printf("Optimal threshold: %.2f\n", optimal.Threshold)
		fmt.Printf("Expected precision: %.2f%%\n", optimal.Precision*100)
		fmt.Printf("Expected recall: %.2f%%\n", optimal.Recall*100)
		fmt.Printf("F1 Score: %.3f\n", optimal.F1Score)
	}

	return results, nil
}

// testThreshold tests a specific threshold value
func (tt *ThresholdTuner) testThreshold(threshold float64, knownGood map[int64]string) (*TuningResult, error) {
	startTime := time.Now()
	result := &TuningResult{
		Threshold: threshold,
	}

	// Create test tiers with this threshold
	tiers := &FuzzyMatchingTiers{
		HighConfidence:   math.Min(threshold+0.15, 0.95),
		MediumConfidence: math.Min(threshold+0.10, 0.90),
		LowConfidence:    math.Min(threshold+0.05, 0.85),
		MinThreshold:     threshold,
		WinnerMargin:     0.03,
	}

	fm := NewFuzzyMatcher(tt.db)
	
	// Test each known good document
	for srcID, correctUPRN := range knownGood {
		// Get the document
		var doc SourceDocument
		err := tt.db.QueryRow(`
			SELECT src_id, addr_can, easting_raw, northing_raw
			FROM src_document
			WHERE src_id = $1
		`, srcID).Scan(&doc.SrcID, &doc.AddrCan, &doc.EastingRaw, &doc.NorthingRaw)
		
		if err != nil {
			continue
		}

		if doc.AddrCan == nil || *doc.AddrCan == "" {
			continue
		}

		// Find candidates at this threshold
		candidates, err := fm.FindFuzzyCandidates(doc, threshold)
		if err != nil {
			continue
		}

		result.CandidatesFound += len(candidates)

		// Check if correct UPRN was found
		correctFound := false
		correctRank := -1
		for i, candidate := range candidates {
			if candidate.UPRN == correctUPRN {
				correctFound = true
				correctRank = i
				break
			}
		}

		// Make decision
		decision, selectedUPRN := fm.makeDecision(candidates, tiers)

		// Update counts based on decision and correctness
		switch decision {
		case "auto_accepted":
			result.AutoAcceptCount++
			if selectedUPRN == correctUPRN {
				result.TruePositives++
			} else {
				result.FalsePositives++
			}
		case "needs_review":
			result.ReviewCount++
			if correctFound && correctRank < 3 {
				// Would be found in review (top 3)
				result.TruePositives++
			} else if !correctFound {
				result.FalseNegatives++
			}
		case "rejected":
			if !correctFound {
				result.TrueNegatives++
			} else {
				result.FalseNegatives++
			}
		}
	}

	// Calculate metrics
	result.ProcessingTime = time.Since(startTime)
	
	if result.TruePositives+result.FalsePositives > 0 {
		result.Precision = float64(result.TruePositives) / float64(result.TruePositives+result.FalsePositives)
	}
	
	if result.TruePositives+result.FalseNegatives > 0 {
		result.Recall = float64(result.TruePositives) / float64(result.TruePositives+result.FalseNegatives)
	}
	
	if result.Precision+result.Recall > 0 {
		result.F1Score = 2 * (result.Precision * result.Recall) / (result.Precision + result.Recall)
	}

	return result, nil
}

// getKnownGoodMatches gets documents with existing accepted matches for validation
func (tt *ThresholdTuner) getKnownGoodMatches(limit int) (map[int64]string, error) {
	knownGood := make(map[int64]string)

	rows, err := tt.db.Query(`
		SELECT s.src_id, m.uprn
		FROM src_document s
		JOIN match_accepted m ON m.src_id = s.src_id
		WHERE s.addr_can IS NOT NULL 
		  AND s.addr_can != ''
		  AND s.addr_can != 'N A'
		  AND m.method LIKE 'deterministic%'
		GROUP BY s.src_id, m.uprn
		ORDER BY RANDOM()
		LIMIT $1
	`, limit)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var srcID int64
		var uprn string
		if err := rows.Scan(&srcID, &uprn); err == nil {
			knownGood[srcID] = uprn
		}
	}

	// If we don't have enough deterministic matches, add some high-confidence fuzzy matches
	if len(knownGood) < limit/2 {
		rows2, err := tt.db.Query(`
			SELECT s.src_id, m.uprn
			FROM src_document s
			JOIN match_accepted m ON m.src_id = s.src_id
			WHERE s.addr_can IS NOT NULL 
			  AND s.addr_can != ''
			  AND s.addr_can != 'N A'
			  AND m.confidence >= 0.90
			  AND s.src_id NOT IN (SELECT src_id FROM match_accepted WHERE method LIKE 'deterministic%')
			GROUP BY s.src_id, m.uprn, m.confidence
			ORDER BY m.confidence DESC
			LIMIT $1
		`, limit-len(knownGood))
		
		if err == nil {
			defer rows2.Close()
			for rows2.Next() {
				var srcID int64
				var uprn string
				if err := rows2.Scan(&srcID, &uprn); err == nil {
					knownGood[srcID] = uprn
				}
			}
		}
	}

	return knownGood, nil
}

// findOptimalThreshold finds the threshold with best F1 score
func (tt *ThresholdTuner) findOptimalThreshold(results []*TuningResult) *TuningResult {
	if len(results) == 0 {
		return nil
	}

	var best *TuningResult
	for _, result := range results {
		// Prefer thresholds with precision >= 0.95 for auto-accept
		if result.Precision >= 0.95 {
			if best == nil || result.F1Score > best.F1Score {
				best = result
			}
		}
	}

	// If no threshold has precision >= 0.95, find best F1 score
	if best == nil {
		for _, result := range results {
			if best == nil || result.F1Score > best.F1Score {
				best = result
			}
		}
	}

	return best
}

// AnalyzeCurrentMatches analyzes the quality of existing matches
func (tt *ThresholdTuner) AnalyzeCurrentMatches() error {
	fmt.Println("\n=== Current Match Quality Analysis ===\n")

	// Analyze by similarity score bands
	rows, err := tt.db.Query(`
		SELECT 
			CASE 
				WHEN confidence >= 0.90 THEN '0.90-1.00'
				WHEN confidence >= 0.85 THEN '0.85-0.90'
				WHEN confidence >= 0.80 THEN '0.80-0.85'
				WHEN confidence >= 0.75 THEN '0.75-0.80'
				WHEN confidence >= 0.70 THEN '0.70-0.75'
				WHEN confidence >= 0.65 THEN '0.65-0.70'
				WHEN confidence >= 0.60 THEN '0.60-0.65'
				ELSE '< 0.60'
			END as score_band,
			COUNT(*) as count,
			SUM(CASE WHEN decision = 'auto_accepted' THEN 1 ELSE 0 END) as auto_accepted,
			SUM(CASE WHEN decision = 'needs_review' THEN 1 ELSE 0 END) as needs_review,
			AVG(score) as avg_score
		FROM match_result
		WHERE method LIKE 'fuzzy%'
		GROUP BY score_band
		ORDER BY score_band DESC
	`)
	
	if err != nil {
		return fmt.Errorf("failed to analyze matches: %w", err)
	}
	defer rows.Close()

	fmt.Println("Score Band  | Count | Auto-Accept | Review | Avg Score")
	fmt.Println("------------|-------|-------------|--------|----------")
	
	for rows.Next() {
		var band string
		var count, autoAccepted, needsReview int
		var avgScore float64
		
		err := rows.Scan(&band, &count, &autoAccepted, &needsReview, &avgScore)
		if err != nil {
			continue
		}
		
		fmt.Printf("%-11s | %5d | %11d | %6d | %.3f\n",
			band, count, autoAccepted, needsReview, avgScore)
	}

	// Show distribution of similarity scores
	fmt.Println("\n=== Address Quality Distribution ===\n")
	
	err = tt.db.QueryRow(`
		SELECT 
			COUNT(DISTINCT src_id) as total_docs,
			SUM(CASE WHEN addr_can IS NOT NULL AND addr_can != '' AND addr_can != 'N A' THEN 1 ELSE 0 END) as with_address,
			SUM(CASE WHEN addr_can = 'N A' THEN 1 ELSE 0 END) as no_address
		FROM src_document
	`).Scan(&struct{ total, withAddr, noAddr int }{})

	return nil
}