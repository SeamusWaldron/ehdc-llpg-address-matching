package engine

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ehdc-llpg/internal/normalize"
)

// HierarchicalMatcher handles component-based hierarchical matching
type HierarchicalMatcher struct {
	db *sql.DB
}

// NewHierarchicalMatcher creates a new hierarchical matcher
func NewHierarchicalMatcher(db *sql.DB) *HierarchicalMatcher {
	return &HierarchicalMatcher{db: db}
}

// HierarchicalCandidate represents a hierarchically matched candidate
type HierarchicalCandidate struct {
	UPRN          string
	Address       string
	CanonicalAddr string
	MatchLevel    string
	Score         float64
	ComponentMatches map[string]bool
	Features      map[string]interface{}
}

// MatchLevel defines the hierarchy of matching approaches
type MatchLevel struct {
	Name        string
	Description string
	MinScore    float64
	Query       string
}

// RunHierarchicalMatching performs hierarchical component matching
func (hm *HierarchicalMatcher) RunHierarchicalMatching(runID int64, batchSize int) (int, int, int, error) {
	startTime := time.Now()
	totalProcessed := 0
	totalAccepted := 0
	totalNeedsReview := 0

	fmt.Println("Starting hierarchical component matching...")

	// Define matching hierarchy (most specific to least specific)
	matchLevels := []MatchLevel{
		{
			Name:        "postcode_house_number",
			Description: "Postcode + House Number",
			MinScore:    0.90,
			Query: `
				SELECT d.uprn, d.full_address, d.address_canonical, 0.95 as score
				FROM dim_address d
				WHERE d.full_address LIKE '%' || $2 || '%'
				  AND d.address_canonical LIKE $1 || '%'
			`,
		},
		{
			Name:        "street_house_locality",
			Description: "Street + House Number + Locality",
			MinScore:    0.85,
			Query: `
				SELECT d.uprn, d.full_address, d.address_canonical, 0.90 as score
				FROM dim_address d
				WHERE ($1 = '' OR d.address_canonical LIKE $1 || '%')
				  AND ($2 = '' OR d.address_canonical LIKE '%' || $2 || '%')
				  AND ($3 = '' OR d.address_canonical LIKE '%' || $3 || '%')
			`,
		},
		{
			Name:        "street_locality",
			Description: "Street Name + Locality",
			MinScore:    0.75,
			Query: `
				SELECT d.uprn, d.full_address, d.address_canonical, 0.80 as score
				FROM dim_address d
				WHERE ($1 = '' OR d.address_canonical LIKE '%' || $1 || '%')
				  AND ($2 = '' OR d.address_canonical LIKE '%' || $2 || '%')
			`,
		},
		{
			Name:        "partial_street_phonetic",
			Description: "Partial Street with Phonetic",
			MinScore:    0.70,
			Query: `
				SELECT d.uprn, d.full_address, d.address_canonical, 0.75 as score
				FROM dim_address d
				WHERE ($1 = '' OR soundex(d.address_canonical) = soundex($1))
				   OR ($1 = '' OR d.address_canonical LIKE '%' || substring($1 from 1 for 4) || '%')
			`,
		},
		{
			Name:        "locality_nearby",
			Description: "Locality + Nearby Streets",
			MinScore:    0.65,
			Query: `
				SELECT d.uprn, d.full_address, d.address_canonical, 0.70 as score
				FROM dim_address d
				WHERE d.address_canonical LIKE '%' || $1 || '%'
			`,
		},
	}

	engine := &MatchEngine{db: hm.db}

	for {
		// Get unmatched documents
		docs, err := hm.getUnmatchedForHierarchical(batchSize)
		if err != nil {
			return totalProcessed, totalAccepted, totalNeedsReview, fmt.Errorf("failed to get documents: %w", err)
		}

		if len(docs) == 0 {
			break
		}

		for _, doc := range docs {
			totalProcessed++

			if doc.AddrCan == nil || *doc.AddrCan == "" || *doc.AddrCan == "N A" {
				continue
			}

			// Extract components from source address
			sourceAddr := *doc.AddrCan
			postcode := ""
			if doc.PostcodeText != nil {
				postcode = *doc.PostcodeText
			}

			components := normalize.ExtractAddressComponents(sourceAddr + " " + postcode)

			// Try each matching level in hierarchy
			var bestCandidate *HierarchicalCandidate
			var matchFound bool

			for _, level := range matchLevels {
				candidates, err := hm.findCandidatesAtLevel(doc, components, level)
				if err != nil {
					continue
				}

				if len(candidates) > 0 {
					bestCandidate = candidates[0]
					matchFound = true
					break // Found match at this level, don't try lower levels
				}
			}

			if !matchFound {
				continue
			}

			// Make decision based on match level and score
			if bestCandidate.Score >= 0.90 {
				// High confidence - auto accept
				err = hm.acceptMatch(engine, runID, doc.SrcID, bestCandidate)
				if err == nil {
					totalAccepted++
				}
			} else if bestCandidate.Score >= 0.70 {
				// Medium confidence - needs review
				hm.saveForReview(engine, runID, doc.SrcID, bestCandidate, 1)
				totalNeedsReview++
			}
			// Below 0.70 is rejected
		}

		if totalProcessed%1000 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(totalProcessed) / elapsed.Seconds()
			fmt.Printf("Processed %d documents (%.1f/sec), accepted %d, needs review %d...\n",
				totalProcessed, rate, totalAccepted, totalNeedsReview)
		}
	}

	fmt.Printf("Hierarchical matching complete: processed %d, accepted %d, needs review %d\n",
		totalProcessed, totalAccepted, totalNeedsReview)
	return totalProcessed, totalAccepted, totalNeedsReview, nil
}

// getUnmatchedForHierarchical gets unmatched documents suitable for hierarchical matching
func (hm *HierarchicalMatcher) getUnmatchedForHierarchical(limit int) ([]SourceDocument, error) {
	rows, err := hm.db.Query(`
		SELECT s.src_id, s.source_type, s.raw_address, s.addr_can, s.postcode_text,
			   s.easting_raw, s.northing_raw, s.uprn_raw
		FROM src_document s
		LEFT JOIN match_accepted m ON m.src_id = s.src_id
		WHERE m.src_id IS NULL
		  AND s.addr_can IS NOT NULL
		  AND s.addr_can != 'N A'
		  AND s.addr_can != ''
		  AND length(s.addr_can) > 10  -- Reasonable address length
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

// findCandidatesAtLevel finds candidates at a specific hierarchical level
func (hm *HierarchicalMatcher) findCandidatesAtLevel(doc SourceDocument, components *normalize.AddressComponents, level MatchLevel) ([]*HierarchicalCandidate, error) {
	var candidates []*HierarchicalCandidate

	// Prepare parameters based on match level
	var params []interface{}
	switch level.Name {
	case "postcode_house_number":
		if components.HouseNumber == "" || components.Postcode == "" {
			return candidates, nil
		}
		params = []interface{}{components.HouseNumber, components.Postcode}

	case "street_house_locality":
		params = []interface{}{
			components.HouseNumber,
			components.StreetName,
			components.Locality,
		}

	case "street_locality":
		if components.StreetName == "" && components.Locality == "" {
			return candidates, nil
		}
		params = []interface{}{
			components.StreetName,
			components.Locality,
		}

	case "partial_street_phonetic":
		if components.StreetName == "" {
			return candidates, nil
		}
		params = []interface{}{components.StreetName}

	case "locality_nearby":
		if components.Locality == "" && components.Town == "" {
			// Try to extract locality from address
			addr := *doc.AddrCan
			locality := hm.extractBestLocality(addr)
			if locality == "" {
				return candidates, nil
			}
			params = []interface{}{locality}
		} else {
			locality := components.Locality
			if locality == "" {
				locality = components.Town
			}
			params = []interface{}{locality}
		}

	default:
		return candidates, nil
	}

	// Execute query
	rows, err := hm.db.Query(level.Query, params...)
	if err != nil {
		return candidates, fmt.Errorf("hierarchical query failed at level %s: %w", level.Name, err)
	}
	defer rows.Close()

	for rows.Next() {
		candidate := &HierarchicalCandidate{
			MatchLevel:       level.Name,
			ComponentMatches: make(map[string]bool),
			Features:         make(map[string]interface{}),
		}

		err := rows.Scan(
			&candidate.UPRN,
			&candidate.Address,
			&candidate.CanonicalAddr,
			&candidate.Score,
		)
		if err != nil {
			continue
		}

		// Analyze component matches
		targetComponents := normalize.ExtractAddressComponents(candidate.CanonicalAddr)
		hm.analyzeComponentMatches(components, targetComponents, candidate)

		// Adjust score based on component quality
		candidate.Score = hm.adjustScoreByComponents(candidate, level.MinScore)

		// Store features for explainability
		candidate.Features = map[string]interface{}{
			"match_level":        level.Name,
			"level_description":  level.Description,
			"base_score":         level.MinScore,
			"adjusted_score":     candidate.Score,
			"component_matches":  candidate.ComponentMatches,
			"source_components":  hm.componentsToMap(components),
			"target_components":  hm.componentsToMap(targetComponents),
			"matching_method":    "hierarchical_component",
		}

		candidates = append(candidates, candidate)

		// Limit to top candidates at each level
		if len(candidates) >= 5 {
			break
		}
	}

	return candidates, nil
}

// analyzeComponentMatches analyzes which components match between source and target
func (hm *HierarchicalMatcher) analyzeComponentMatches(source, target *normalize.AddressComponents, candidate *HierarchicalCandidate) {
	candidate.ComponentMatches["house_number"] = source.HouseNumber != "" && 
		source.HouseNumber == target.HouseNumber

	candidate.ComponentMatches["house_name"] = source.HouseName != "" && 
		strings.Contains(strings.ToUpper(target.HouseName), strings.ToUpper(source.HouseName))

	candidate.ComponentMatches["street_name"] = source.StreetName != "" &&
		normalize.PartialStringMatch(source.StreetName, target.StreetName) > 0.7

	candidate.ComponentMatches["locality"] = source.Locality != "" &&
		(source.Locality == target.Locality || 
		 strings.Contains(target.Locality, source.Locality))

	candidate.ComponentMatches["town"] = source.Town != "" &&
		(source.Town == target.Town || 
		 strings.Contains(target.Town, source.Town))

	candidate.ComponentMatches["postcode"] = source.Postcode != "" &&
		source.Postcode == target.Postcode
}

// adjustScoreByComponents adjusts score based on component match quality
func (hm *HierarchicalMatcher) adjustScoreByComponents(candidate *HierarchicalCandidate, baseScore float64) float64 {
	score := baseScore

	// Bonuses for exact matches
	if candidate.ComponentMatches["house_number"] {
		score += 0.15
	}
	if candidate.ComponentMatches["postcode"] {
		score += 0.10
	}
	if candidate.ComponentMatches["street_name"] {
		score += 0.10
	}
	if candidate.ComponentMatches["locality"] {
		score += 0.05
	}

	// Penalties for mismatches
	if !candidate.ComponentMatches["house_number"] && baseScore > 0.85 {
		score -= 0.10 // Penalty for missing house number in high-confidence matches
	}

	// Clamp to reasonable range
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return score
}

// extractBestLocality tries to extract a locality from an address string
func (hm *HierarchicalMatcher) extractBestLocality(address string) string {
	// Known Hampshire localities (expand as needed)
	localities := []string{
		"ALTON", "PETERSFIELD", "LIPHOOK", "WATERLOOVILLE", "HORNDEAN",
		"FOUR MARKS", "BEECH", "ROPLEY", "ALRESFORD", "BORDON",
		"WHITEHILL", "GRAYSHOTT", "HINDHEAD", "HASLEMERE", "SHEET",
		"STEEP", "STROUD", "HAWKLEY", "SELBORNE", "EAST TISTED",
		"WEST TISTED", "CHAWTON", "HOLYBOURNE", "MEDSTEAD", "BENTLEY",
		"CATHERINGTON", "CLANFIELD", "DENMEAD", "HAMBLEDON", "ROWLANDS CASTLE",
		"SOBERTON", "WICKHAM", "DROXFORD", "EXTON", "MEONSTOKE",
	}

	upperAddr := strings.ToUpper(address)
	for _, locality := range localities {
		if strings.Contains(upperAddr, locality) {
			return locality
		}
	}

	return ""
}

// componentsToMap converts AddressComponents to map for JSON serialization
func (hm *HierarchicalMatcher) componentsToMap(components *normalize.AddressComponents) map[string]string {
	return map[string]string{
		"house_number": components.HouseNumber,
		"house_name":   components.HouseName,
		"street_name":  components.StreetName,
		"locality":     components.Locality,
		"town":         components.Town,
		"county":       components.County,
		"postcode":     components.Postcode,
	}
}

// acceptMatch accepts a hierarchical match
func (hm *HierarchicalMatcher) acceptMatch(engine *MatchEngine, runID, srcID int64, candidate *HierarchicalCandidate) error {
	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        fmt.Sprintf("hierarchical_%s", candidate.MatchLevel),
		Score:         candidate.Score,
		Confidence:    candidate.Score,
		TieRank:       1,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "auto_accepted",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Hierarchical match auto-accepted (level=%s, score=%.3f)",
			candidate.MatchLevel, candidate.Score),
	}

	if err := engine.SaveMatchResult(result); err != nil {
		return fmt.Errorf("failed to save match result: %w", err)
	}

	return engine.AcceptMatch(srcID, candidate.UPRN, fmt.Sprintf("hierarchical_%s", candidate.MatchLevel),
		candidate.Score, candidate.Score, runID, "system")
}

// saveForReview saves a hierarchical candidate for manual review
func (hm *HierarchicalMatcher) saveForReview(engine *MatchEngine, runID, srcID int64,
	candidate *HierarchicalCandidate, rank int) error {

	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        fmt.Sprintf("hierarchical_%s", candidate.MatchLevel),
		Score:         candidate.Score,
		Confidence:    candidate.Score,
		TieRank:       rank,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "needs_review",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Hierarchical match requiring review (level=%s, score=%.3f)",
			candidate.MatchLevel, candidate.Score),
	}

	return engine.SaveMatchResult(result)
}