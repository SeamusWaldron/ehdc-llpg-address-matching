package engine

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ehdc-llpg/internal/normalize"
)

// PostcodeMatcher handles postcode-centric matching
type PostcodeMatcher struct {
	db *sql.DB
}

// NewPostcodeMatcher creates a new postcode matcher
func NewPostcodeMatcher(db *sql.DB) *PostcodeMatcher {
	return &PostcodeMatcher{db: db}
}

// RunPostcodeMatching performs matching based on postcodes
func (pm *PostcodeMatcher) RunPostcodeMatching(runID int64, batchSize int) (int, int, int, error) {
	startTime := time.Now()
	totalProcessed := 0
	totalAccepted := 0
	totalNeedsReview := 0

	fmt.Println("Starting postcode-centric matching...")
	
	engine := &MatchEngine{db: pm.db}

	for {
		// Get unmatched documents with postcodes
		docs, err := pm.getUnmatchedWithPostcodes(batchSize)
		if err != nil {
			return totalProcessed, totalAccepted, totalNeedsReview, fmt.Errorf("failed to get documents: %w", err)
		}

		if len(docs) == 0 {
			break
		}

		for _, doc := range docs {
			totalProcessed++

			// Find candidates in same postcode
			candidates, err := pm.findPostcodeMatches(doc)
			if err != nil {
				fmt.Printf("Error finding postcode matches for doc %d: %v\n", doc.SrcID, err)
				continue
			}

			if len(candidates) == 0 {
				continue
			}

			// Make decision based on similarity
			bestCandidate := candidates[0]
			
			if bestCandidate.Score >= 0.85 && len(candidates) == 1 {
				// High confidence single match
				err = pm.acceptMatch(engine, runID, doc.SrcID, bestCandidate)
				if err == nil {
					totalAccepted++
				}
			} else if bestCandidate.Score >= 0.75 {
				// Medium confidence - needs review
				for i, candidate := range candidates {
					if i >= 3 {
						break
					}
					pm.saveForReview(engine, runID, doc.SrcID, candidate, i+1)
				}
				totalNeedsReview++
			}
			// Below 0.75 is rejected
		}

		if totalProcessed%1000 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(totalProcessed) / elapsed.Seconds()
			fmt.Printf("Processed %d documents (%.1f/sec), accepted %d, needs review %d...\n",
				totalProcessed, rate, totalAccepted, totalNeedsReview)
		}
	}

	fmt.Printf("Postcode matching complete: processed %d, accepted %d, needs review %d\n",
		totalProcessed, totalAccepted, totalNeedsReview)
	return totalProcessed, totalAccepted, totalNeedsReview, nil
}

// getUnmatchedWithPostcodes gets documents that have postcodes but no matches
func (pm *PostcodeMatcher) getUnmatchedWithPostcodes(limit int) ([]SourceDocument, error) {
	rows, err := pm.db.Query(`
		SELECT s.src_id, s.source_type, s.raw_address, s.addr_can, s.postcode_text,
			   s.easting_raw, s.northing_raw, s.uprn_raw
		FROM src_document s
		LEFT JOIN match_accepted m ON m.src_id = s.src_id
		WHERE m.src_id IS NULL
		  AND s.postcode_text IS NOT NULL
		  AND s.postcode_text != ''
		  AND s.addr_can IS NOT NULL
		  AND s.addr_can != 'N A'
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

// PostcodeCandidate represents a candidate found by postcode matching
type PostcodeCandidate struct {
	UPRN           string
	Address        string
	CanonicalAddr  string
	Score          float64
	ComponentScore float64
	HouseNumMatch  bool
	StreetMatch    bool
	Features       map[string]interface{}
}

// findPostcodeMatches finds LLPG addresses in the same postcode
func (pm *PostcodeMatcher) findPostcodeMatches(doc SourceDocument) ([]*PostcodeCandidate, error) {
	if doc.PostcodeText == nil || *doc.PostcodeText == "" {
		return nil, nil
	}

	postcode := *doc.PostcodeText
	sourceAddr := ""
	if doc.AddrCan != nil {
		sourceAddr = *doc.AddrCan
	}

	// Extract components from source address
	sourceComponents := normalize.ExtractAddressComponents(sourceAddr + " " + postcode)

	// Query for addresses with same postcode
	rows, err := pm.db.Query(`
		SELECT d.uprn, d.locaddress, d.addr_can
		FROM dim_address d
		WHERE d.locaddress LIKE $1
		   OR d.locaddress LIKE $2
		ORDER BY d.uprn
	`, "%"+postcode, "%"+strings.ReplaceAll(postcode, " ", ""))

	if err != nil {
		return nil, fmt.Errorf("postcode query failed: %w", err)
	}
	defer rows.Close()

	var candidates []*PostcodeCandidate
	
	for rows.Next() {
		candidate := &PostcodeCandidate{
			Features: make(map[string]interface{}),
		}

		err := rows.Scan(&candidate.UPRN, &candidate.Address, &candidate.CanonicalAddr)
		if err != nil {
			continue
		}

		// Extract components from candidate
		targetComponents := normalize.ExtractAddressComponents(candidate.CanonicalAddr + " " + postcode)

		// Calculate component-based score
		candidate.ComponentScore = normalize.MatchByComponents(sourceComponents, targetComponents)

		// Check specific matches
		candidate.HouseNumMatch = sourceComponents.HouseNumber != "" && 
			sourceComponents.HouseNumber == targetComponents.HouseNumber
		
		candidate.StreetMatch = sourceComponents.StreetName != "" &&
			strings.Contains(targetComponents.StreetName, sourceComponents.StreetName)

		// Calculate overall similarity (without postcode since they match)
		sourceWithoutPostcode := strings.ReplaceAll(sourceAddr, postcode, "")
		targetWithoutPostcode := strings.ReplaceAll(candidate.CanonicalAddr, postcode, "")
		
		// Use trigram similarity if available
		var trigramScore float64
		err = pm.db.QueryRow(`
			SELECT similarity($1, $2)
		`, sourceWithoutPostcode, targetWithoutPostcode).Scan(&trigramScore)
		
		if err == nil {
			candidate.Score = trigramScore*0.6 + candidate.ComponentScore*0.4
		} else {
			candidate.Score = candidate.ComponentScore
		}

		// Boost score for exact house number match
		if candidate.HouseNumMatch {
			candidate.Score += 0.15
			if candidate.Score > 1.0 {
				candidate.Score = 1.0
			}
		}

		// Store features for explainability
		candidate.Features = map[string]interface{}{
			"postcode":         postcode,
			"component_score":  candidate.ComponentScore,
			"house_num_match":  candidate.HouseNumMatch,
			"street_match":     candidate.StreetMatch,
			"trigram_score":    trigramScore,
			"final_score":      candidate.Score,
			"source_addr":      sourceAddr,
			"target_addr":      candidate.Address,
			"source_house_num": sourceComponents.HouseNumber,
			"target_house_num": targetComponents.HouseNumber,
			"source_street":    sourceComponents.StreetName,
			"target_street":    targetComponents.StreetName,
		}

		candidates = append(candidates, candidate)
	}

	// Sort by score
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Score > candidates[i].Score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	return candidates, nil
}

// acceptMatch accepts a postcode-based match
func (pm *PostcodeMatcher) acceptMatch(engine *MatchEngine, runID, srcID int64, candidate *PostcodeCandidate) error {
	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        "postcode_match",
		Score:         candidate.Score,
		Confidence:    candidate.Score,
		TieRank:       1,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "auto_accepted",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Postcode match auto-accepted (score=%.3f, house_num=%v)",
			candidate.Score, candidate.HouseNumMatch),
	}

	if err := engine.SaveMatchResult(result); err != nil {
		return fmt.Errorf("failed to save match result: %w", err)
	}

	return engine.AcceptMatch(srcID, candidate.UPRN, "postcode_match",
		candidate.Score, candidate.Score, runID, "system")
}

// saveForReview saves a candidate for manual review
func (pm *PostcodeMatcher) saveForReview(engine *MatchEngine, runID, srcID int64, 
	candidate *PostcodeCandidate, rank int) error {
	
	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        "postcode_match",
		Score:         candidate.Score,
		Confidence:    candidate.Score,
		TieRank:       rank,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "needs_review",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Postcode match requiring review (score=%.3f)", candidate.Score),
	}

	return engine.SaveMatchResult(result)
}

// AnalyzePostcodeQuality analyzes the quality of postcode data
func (pm *PostcodeMatcher) AnalyzePostcodeQuality() error {
	fmt.Println("\n=== Postcode Data Quality Analysis ===\n")

	// Documents with postcodes
	var withPostcode, withoutPostcode, invalidPostcode int
	err := pm.db.QueryRow(`
		SELECT 
			COUNT(CASE WHEN postcode_text IS NOT NULL AND postcode_text != '' THEN 1 END) as with_postcode,
			COUNT(CASE WHEN postcode_text IS NULL OR postcode_text = '' THEN 1 END) as without_postcode,
			COUNT(CASE WHEN postcode_text IS NOT NULL AND postcode_text != '' 
				AND postcode_text !~ '^[A-Z]{1,2}[0-9]{1,2}[A-Z]?\s*[0-9][A-Z]{2}$' THEN 1 END) as invalid
		FROM src_document
	`).Scan(&withPostcode, &withoutPostcode, &invalidPostcode)

	if err != nil {
		return err
	}

	fmt.Printf("Documents with postcodes: %d\n", withPostcode)
	fmt.Printf("Documents without postcodes: %d\n", withoutPostcode)
	fmt.Printf("Documents with invalid postcodes: %d\n", invalidPostcode)
	fmt.Printf("Postcode coverage: %.2f%%\n", 
		float64(withPostcode)/float64(withPostcode+withoutPostcode)*100)

	// Postcode distribution
	fmt.Println("\n=== Top 10 Postcodes in Source Data ===")
	rows, err := pm.db.Query(`
		SELECT postcode_text, COUNT(*) as count
		FROM src_document
		WHERE postcode_text IS NOT NULL AND postcode_text != ''
		GROUP BY postcode_text
		ORDER BY count DESC
		LIMIT 10
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var postcode string
		var count int
		if err := rows.Scan(&postcode, &count); err == nil {
			fmt.Printf("  %s: %d documents\n", postcode, count)
		}
	}

	// Postcode matching potential
	fmt.Println("\n=== Postcode Matching Potential ===")
	
	var matchPotential int
	err = pm.db.QueryRow(`
		WITH postcode_pairs AS (
			SELECT DISTINCT s.src_id, s.postcode_text
			FROM src_document s
			LEFT JOIN match_accepted m ON m.src_id = s.src_id
			WHERE m.src_id IS NULL
			  AND s.postcode_text IS NOT NULL
			  AND s.postcode_text != ''
		)
		SELECT COUNT(DISTINCT pp.src_id)
		FROM postcode_pairs pp
		WHERE EXISTS (
			SELECT 1 FROM dim_address d
			WHERE d.locaddress LIKE '%' || pp.postcode_text || '%'
		)
	`).Scan(&matchPotential)

	if err == nil {
		fmt.Printf("Documents with matching postcodes in LLPG: %d\n", matchPotential)
		fmt.Printf("Potential coverage improvement: %.2f%%\n",
			float64(matchPotential)/float64(withPostcode)*100)
	}

	return nil
}