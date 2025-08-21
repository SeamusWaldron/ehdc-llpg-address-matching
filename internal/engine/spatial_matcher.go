package engine

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/ehdc-llpg/internal/normalize"
)

// SpatialMatcher handles spatial proximity matching
type SpatialMatcher struct {
	db *sql.DB
}

// NewSpatialMatcher creates a new spatial matcher
func NewSpatialMatcher(db *sql.DB) *SpatialMatcher {
	return &SpatialMatcher{db: db}
}

// SpatialCandidate represents a spatially matched candidate
type SpatialCandidate struct {
	UPRN              string
	Address           string
	CanonicalAddr     string
	Easting           float64
	Northing          float64
	Distance          float64
	AddressSimilarity float64
	SpatialScore      float64
	FinalScore        float64
	Features          map[string]interface{}
}

// RunSpatialMatching performs spatial proximity matching
func (sm *SpatialMatcher) RunSpatialMatching(runID int64, batchSize int, maxDistance float64) (int, int, int, error) {
	if maxDistance <= 0 {
		maxDistance = 100.0 // Default 100 meters
	}

	startTime := time.Now()
	totalProcessed := 0
	totalAccepted := 0
	totalNeedsReview := 0

	fmt.Printf("Starting spatial proximity matching (max distance: %.0fm)...\n", maxDistance)

	engine := &MatchEngine{db: sm.db}

	for {
		// Get unmatched documents with coordinates
		docs, err := sm.getUnmatchedWithCoordinates(batchSize)
		if err != nil {
			return totalProcessed, totalAccepted, totalNeedsReview, fmt.Errorf("failed to get documents: %w", err)
		}

		if len(docs) == 0 {
			break
		}

		for _, doc := range docs {
			totalProcessed++

			if doc.EastingRaw == nil || doc.NorthingRaw == nil {
				continue
			}

			// Find spatial candidates
			candidates, err := sm.findSpatialCandidates(doc, maxDistance)
			if err != nil {
				fmt.Printf("Error finding spatial candidates for doc %d: %v\n", doc.SrcID, err)
				continue
			}

			if len(candidates) == 0 {
				continue
			}

			// Make decision based on distance and similarity
			bestCandidate := candidates[0]

			if bestCandidate.Distance <= 25.0 && bestCandidate.AddressSimilarity >= 0.80 {
				// Very close and similar - high confidence
				err = sm.acceptMatch(engine, runID, doc.SrcID, bestCandidate, "spatial_high")
				if err == nil {
					totalAccepted++
				}
			} else if bestCandidate.Distance <= 50.0 && bestCandidate.AddressSimilarity >= 0.60 {
				// Close with reasonable similarity - auto accept
				err = sm.acceptMatch(engine, runID, doc.SrcID, bestCandidate, "spatial_medium")
				if err == nil {
					totalAccepted++
				}
			} else if bestCandidate.Distance <= maxDistance && bestCandidate.FinalScore >= 0.50 {
				// Within range but needs review
				for i, candidate := range candidates {
					if i >= 3 {
						break
					}
					sm.saveForReview(engine, runID, doc.SrcID, candidate, i+1)
				}
				totalNeedsReview++
			}
		}

		if totalProcessed%1000 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(totalProcessed) / elapsed.Seconds()
			fmt.Printf("Processed %d documents (%.1f/sec), accepted %d, needs review %d...\n",
				totalProcessed, rate, totalAccepted, totalNeedsReview)
		}
	}

	fmt.Printf("Spatial matching complete: processed %d, accepted %d, needs review %d\n",
		totalProcessed, totalAccepted, totalNeedsReview)
	return totalProcessed, totalAccepted, totalNeedsReview, nil
}

// getUnmatchedWithCoordinates gets documents with coordinates but no matches
func (sm *SpatialMatcher) getUnmatchedWithCoordinates(limit int) ([]SourceDocument, error) {
	rows, err := sm.db.Query(`
		SELECT s.src_id, s.source_type, s.raw_address, s.addr_can, s.postcode_text,
			   s.easting_raw, s.northing_raw, s.uprn_raw
		FROM src_document s
		LEFT JOIN match_accepted m ON m.src_id = s.src_id
		WHERE m.src_id IS NULL
		  AND s.easting_raw IS NOT NULL
		  AND s.northing_raw IS NOT NULL
		  AND s.easting_raw > 0
		  AND s.northing_raw > 0
		ORDER BY s.src_id
		LIMIT $1
	`, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []SourceDocument
	for rows.Next() {
		var doc SourceDocument
		err := rows.Scan(
			&doc.SrcID, &doc.SourceType, &doc.RawAddress, &doc.AddrCan,
			&doc.PostcodeText, &doc.EastingRaw, &doc.NorthingRaw, &doc.UPRNRaw,
		)
		if err != nil {
			continue
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// findSpatialCandidates finds LLPG addresses within spatial proximity
func (sm *SpatialMatcher) findSpatialCandidates(doc SourceDocument, maxDistance float64) ([]*SpatialCandidate, error) {
	if doc.EastingRaw == nil || doc.NorthingRaw == nil {
		return nil, nil
	}

	sourceEasting := *doc.EastingRaw
	sourceNorthing := *doc.NorthingRaw
	sourceAddr := ""
	if doc.AddrCan != nil {
		sourceAddr = *doc.AddrCan
	}

	// Use PostGIS for efficient spatial query
	rows, err := sm.db.Query(`
		SELECT 
			d.uprn, 
			d.locaddress, 
			d.addr_can,
			d.easting,
			d.northing,
			ST_Distance(
				ST_SetSRID(ST_MakePoint($1, $2), 27700),
				ST_SetSRID(ST_MakePoint(d.easting, d.northing), 27700)
			) as distance_meters,
			CASE 
				WHEN $3 != '' THEN similarity($3, d.addr_can)
				ELSE 0.0
			END as address_similarity
		FROM dim_address d
		WHERE ST_DWithin(
			ST_SetSRID(ST_MakePoint($1, $2), 27700),
			ST_SetSRID(ST_MakePoint(d.easting, d.northing), 27700),
			$4
		)
		ORDER BY distance_meters ASC, address_similarity DESC
		LIMIT 10
	`, sourceEasting, sourceNorthing, sourceAddr, maxDistance)

	if err != nil {
		return nil, fmt.Errorf("spatial query failed: %w", err)
	}
	defer rows.Close()

	var candidates []*SpatialCandidate

	for rows.Next() {
		candidate := &SpatialCandidate{
			Features: make(map[string]interface{}),
		}

		err := rows.Scan(
			&candidate.UPRN,
			&candidate.Address,
			&candidate.CanonicalAddr,
			&candidate.Easting,
			&candidate.Northing,
			&candidate.Distance,
			&candidate.AddressSimilarity,
		)
		if err != nil {
			continue
		}

		// Calculate spatial score (closer = higher score)
		candidate.SpatialScore = math.Exp(-candidate.Distance / 50.0)

		// Calculate final score combining spatial and text similarity
		candidate.FinalScore = sm.calculateSpatialScore(candidate, sourceAddr)

		// Store features for explainability
		candidate.Features = map[string]interface{}{
			"distance_meters":      candidate.Distance,
			"address_similarity":   candidate.AddressSimilarity,
			"spatial_score":        candidate.SpatialScore,
			"final_score":          candidate.FinalScore,
			"source_address":       sourceAddr,
			"target_address":       candidate.Address,
			"source_easting":       sourceEasting,
			"source_northing":      sourceNorthing,
			"target_easting":       candidate.Easting,
			"target_northing":      candidate.Northing,
			"matching_method":      "spatial_proximity",
		}

		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

// calculateSpatialScore computes weighted score for spatial matching
func (sm *SpatialMatcher) calculateSpatialScore(candidate *SpatialCandidate, sourceAddr string) float64 {
	score := 0.0

	// Spatial component (60% weight) - exponential decay with distance
	spatialWeight := 0.60
	score += spatialWeight * candidate.SpatialScore

	// Address similarity component (40% weight)
	textWeight := 0.40
	score += textWeight * candidate.AddressSimilarity

	// Distance-based bonuses
	if candidate.Distance <= 10.0 {
		score += 0.10 // Very close bonus
	} else if candidate.Distance <= 25.0 {
		score += 0.05 // Close bonus
	}

	// Address similarity bonuses
	if candidate.AddressSimilarity >= 0.80 {
		score += 0.15 // High similarity bonus
	} else if candidate.AddressSimilarity >= 0.60 {
		score += 0.10 // Medium similarity bonus
	}

	// Component matching bonuses
	if sourceAddr != "" && candidate.CanonicalAddr != "" {
		sourceComponents := normalize.ExtractAddressComponents(sourceAddr)
		targetComponents := normalize.ExtractAddressComponents(candidate.CanonicalAddr)

		// House number match bonus
		if sourceComponents.HouseNumber != "" && 
		   sourceComponents.HouseNumber == targetComponents.HouseNumber {
			score += 0.15
		}

		// Street name partial match bonus
		if sourceComponents.StreetName != "" && targetComponents.StreetName != "" {
			if normalize.PartialStringMatch(sourceComponents.StreetName, targetComponents.StreetName) > 0.7 {
				score += 0.10
			}
		}
	}

	// Clamp to [0, 1]
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return score
}

// acceptMatch accepts a spatial match
func (sm *SpatialMatcher) acceptMatch(engine *MatchEngine, runID, srcID int64, candidate *SpatialCandidate, confidence string) error {
	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        confidence,
		Score:         candidate.FinalScore,
		Confidence:    candidate.FinalScore,
		TieRank:       1,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "auto_accepted",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Spatial match auto-accepted (distance=%.1fm, similarity=%.3f)",
			candidate.Distance, candidate.AddressSimilarity),
	}

	if err := engine.SaveMatchResult(result); err != nil {
		return fmt.Errorf("failed to save match result: %w", err)
	}

	return engine.AcceptMatch(srcID, candidate.UPRN, confidence,
		candidate.FinalScore, candidate.FinalScore, runID, "system")
}

// saveForReview saves a spatial candidate for manual review
func (sm *SpatialMatcher) saveForReview(engine *MatchEngine, runID, srcID int64,
	candidate *SpatialCandidate, rank int) error {

	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        "spatial_proximity",
		Score:         candidate.FinalScore,
		Confidence:    candidate.FinalScore,
		TieRank:       rank,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "needs_review",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Spatial match requiring review (distance=%.1fm, score=%.3f)",
			candidate.Distance, candidate.FinalScore),
	}

	return engine.SaveMatchResult(result)
}

// AnalyzeSpatialQuality analyzes spatial data quality and potential
func (sm *SpatialMatcher) AnalyzeSpatialQuality() error {
	fmt.Println("\n=== Spatial Data Quality Analysis ===\n")

	// Documents with coordinates
	var withCoords, withoutCoords, validCoords int
	err := sm.db.QueryRow(`
		SELECT 
			COUNT(CASE WHEN easting_raw IS NOT NULL AND northing_raw IS NOT NULL THEN 1 END) as with_coords,
			COUNT(CASE WHEN easting_raw IS NULL OR northing_raw IS NULL THEN 1 END) as without_coords,
			COUNT(CASE WHEN easting_raw > 0 AND northing_raw > 0 THEN 1 END) as valid_coords
		FROM src_document
	`).Scan(&withCoords, &withoutCoords, &validCoords)

	if err != nil {
		return err
	}

	fmt.Printf("Documents with coordinates: %d\n", withCoords)
	fmt.Printf("Documents without coordinates: %d\n", withoutCoords)
	fmt.Printf("Documents with valid coordinates: %d\n", validCoords)
	fmt.Printf("Coordinate coverage: %.2f%%\n",
		float64(validCoords)/float64(withCoords+withoutCoords)*100)

	// Spatial matching potential at different distances
	distances := []float64{25, 50, 100, 200}
	
	fmt.Println("\n=== Spatial Matching Potential by Distance ===")
	fmt.Println("Distance | Potential Matches | Coverage Improvement")
	fmt.Println("---------|-------------------|--------------------")

	for _, distance := range distances {
		var potential int
		err = sm.db.QueryRow(`
			WITH unmatched_with_coords AS (
				SELECT s.src_id, s.easting_raw, s.northing_raw
				FROM src_document s
				LEFT JOIN match_accepted m ON m.src_id = s.src_id
				WHERE m.src_id IS NULL
				  AND s.easting_raw IS NOT NULL
				  AND s.northing_raw IS NOT NULL
				  AND s.easting_raw > 0
				  AND s.northing_raw > 0
			)
			SELECT COUNT(DISTINCT uwc.src_id)
			FROM unmatched_with_coords uwc
			WHERE EXISTS (
				SELECT 1 FROM dim_address d
				WHERE ST_DWithin(
					ST_SetSRID(ST_MakePoint(uwc.easting_raw, uwc.northing_raw), 27700),
					ST_SetSRID(ST_MakePoint(d.easting, d.northing), 27700),
					$1
				)
			)
		`, distance).Scan(&potential)

		if err == nil {
			fmt.Printf("%6.0fm | %17d | %17.2f%%\n",
				distance, potential, float64(potential)/float64(validCoords)*100)
		}
	}

	// Coordinate distribution by source type
	fmt.Println("\n=== Coordinate Coverage by Source Type ===")
	rows, err := sm.db.Query(`
		SELECT 
			source_type,
			COUNT(*) as total,
			COUNT(CASE WHEN easting_raw IS NOT NULL AND northing_raw IS NOT NULL THEN 1 END) as with_coords,
			COUNT(CASE WHEN easting_raw > 0 AND northing_raw > 0 THEN 1 END) as valid_coords
		FROM src_document
		GROUP BY source_type
		ORDER BY total DESC
	`)

	if err == nil {
		defer rows.Close()
		fmt.Println("Type       | Total  | With Coords | Valid Coords | Coverage")
		fmt.Println("-----------|--------|-------------|--------------|----------")

		for rows.Next() {
			var sourceType string
			var total, withCoords, validCoords int

			if err := rows.Scan(&sourceType, &total, &withCoords, &validCoords); err == nil {
				fmt.Printf("%-10s | %6d | %11d | %12d | %7.1f%%\n",
					sourceType, total, withCoords, validCoords,
					float64(validCoords)/float64(total)*100)
			}
		}
	}

	return nil
}