package engine

import (
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/ehdc-llpg/internal/normalize"
)

// FuzzyMatcher handles Stage 2 fuzzy matching using PostgreSQL pg_trgm
type FuzzyMatcher struct {
	db *sql.DB
}

// NewFuzzyMatcher creates a new fuzzy matcher
func NewFuzzyMatcher(db *sql.DB) *FuzzyMatcher {
	return &FuzzyMatcher{db: db}
}

// FuzzyCandidate represents a fuzzy match candidate with features
type FuzzyCandidate struct {
	*AddressCandidate
	TrgramScore      float64            `json:"trgm_score"`
	JaroScore        float64            `json:"jaro_score"`
	LocalityOverlap  float64            `json:"locality_overlap"`
	StreetOverlap    float64            `json:"street_overlap"`
	SameHouseNumber  bool               `json:"same_house_number"`
	SameHouseAlpha   bool               `json:"same_house_alpha"`
	PhoneticHits     int                `json:"phonetic_hits"`
	SpatialDistance  float64            `json:"spatial_distance,omitempty"`
	SpatialBoost     float64            `json:"spatial_boost"`
	Features         map[string]interface{} `json:"features"`
	FinalScore       float64            `json:"final_score"`
}

// FuzzyMatchingTiers define the matching thresholds
type FuzzyMatchingTiers struct {
	HighConfidence   float64 // >= 0.90 - auto accept if unique or clear winner
	MediumConfidence float64 // >= 0.85 - auto accept with additional validation
	LowConfidence    float64 // >= 0.80 - always review
	MinThreshold     float64 // >= 0.80 - below this is rejected
	WinnerMargin     float64 // 0.03 - gap needed to next candidate for auto-accept
}

// DefaultTiers returns the default fuzzy matching tier configuration
// Based on threshold tuning analysis for optimal balance of precision/recall
func DefaultTiers() *FuzzyMatchingTiers {
	return &FuzzyMatchingTiers{
		HighConfidence:   0.85,  // Auto-accept: Very high confidence matches
		MediumConfidence: 0.78,  // Auto-accept with validation: Good matches  
		LowConfidence:    0.70,  // Review required: Potential matches
		MinThreshold:     0.60,  // Minimum to consider: Broader net for fuzzy matching
		WinnerMargin:     0.05,  // Gap needed to next candidate for auto-accept
	}
}

// RunFuzzyMatching performs Stage 2 fuzzy matching for unmatched documents
func (fm *FuzzyMatcher) RunFuzzyMatching(runID int64, batchSize int, tiers *FuzzyMatchingTiers) (int, int, int, error) {
	if tiers == nil {
		tiers = DefaultTiers()
	}

	totalProcessed := 0
	totalAccepted := 0
	totalNeedsReview := 0

	fmt.Println("Starting fuzzy matching (Stage 2) with pg_trgm similarity...")

	for {
		// Get batch of unmatched documents  
		engine := &MatchEngine{db: fm.db}
		docs, err := engine.GetUnmatchedDocuments(batchSize, "")
		if err != nil {
			return totalProcessed, totalAccepted, totalNeedsReview, fmt.Errorf("failed to get unmatched documents: %w", err)
		}

		if len(docs) == 0 {
			break
		}

		batchAccepted := 0
		batchReview := 0

		for _, doc := range docs {
			if doc.AddrCan == nil || strings.TrimSpace(*doc.AddrCan) == "" {
				totalProcessed++
				continue // Skip documents without canonical addresses
			}

			// Find fuzzy candidates
			candidates, err := fm.FindFuzzyCandidates(doc, tiers.MinThreshold)
			if err != nil {
				fmt.Printf("Error finding fuzzy candidates for doc %d: %v\n", doc.SrcID, err)
				totalProcessed++
				continue
			}

			if len(candidates) == 0 {
				totalProcessed++
				continue // No candidates found
			}

			// Make decision based on tiers and candidates
			decision, _ := fm.makeDecision(candidates, tiers)

			switch decision {
			case "auto_accepted":
				// Accept the best candidate
				best := candidates[0]
				err := fm.acceptFuzzyMatch(engine, runID, doc.SrcID, best, "fuzzy_auto", best.FinalScore, best.TrgramScore)
				if err == nil {
					batchAccepted++
				}

			case "needs_review":
				// Save top candidates for review
				for i, candidate := range candidates {
					if i >= 3 { // Limit to top 3 for review
						break
					}

					result := &MatchResult{
						RunID:         runID,
						SrcID:         doc.SrcID,
						CandidateUPRN: candidate.UPRN,
						Method:        fmt.Sprintf("fuzzy_%.2f", candidate.TrgramScore),
						Score:         candidate.FinalScore,
						Confidence:    candidate.TrgramScore,
						TieRank:       i + 1,
						Features:      candidate.Features,
						Decided:       true,
						Decision:      "needs_review",
						DecidedBy:     "system",
						Notes:         fmt.Sprintf("Fuzzy match requiring review (similarity=%.3f)", candidate.TrgramScore),
					}

					engine.SaveMatchResult(result)
				}
				batchReview++

			case "rejected":
				// Could save rejection reasons but for now just continue
			}

			totalProcessed++
		}

		totalAccepted += batchAccepted
		totalNeedsReview += batchReview

		if totalProcessed%1000 == 0 {
			fmt.Printf("Processed %d documents, accepted %d, needs review %d...\n", 
				totalProcessed, totalAccepted, totalNeedsReview)
		}
	}

	fmt.Printf("Fuzzy matching complete: processed %d, accepted %d, needs review %d\n", 
		totalProcessed, totalAccepted, totalNeedsReview)
	return totalProcessed, totalAccepted, totalNeedsReview, nil
}

// FindFuzzyCandidates finds fuzzy candidates using pg_trgm similarity
func (fm *FuzzyMatcher) FindFuzzyCandidates(doc SourceDocument, minSimilarity float64) ([]*FuzzyCandidate, error) {
	if doc.AddrCan == nil {
		return nil, nil
	}

	addrCan := strings.TrimSpace(*doc.AddrCan)
	if addrCan == "" {
		return nil, nil
	}

	// Query for trigram similarity matches
	rows, err := fm.db.Query(`
		SELECT d.uprn, d.full_address, d.address_canonical,
		       COALESCE(l.easting, 0), COALESCE(l.northing, 0),
		       d.usrn, d.blpu_class, d.status_code,
		       similarity($1, d.address_canonical) as trgm_score
		FROM dim_address d
		LEFT JOIN dim_location l ON d.location_id = l.location_id
		WHERE d.address_canonical % $1
		  AND similarity($1, d.address_canonical) >= $2
		ORDER BY trgm_score DESC
		LIMIT 50
	`, addrCan, minSimilarity)

	if err != nil {
		return nil, fmt.Errorf("trigram query failed: %w", err)
	}
	defer rows.Close()

	var candidates []*FuzzyCandidate

	for rows.Next() {
		candidate := &FuzzyCandidate{
			AddressCandidate: &AddressCandidate{},
			Features:         make(map[string]interface{}),
		}

		err := rows.Scan(
			&candidate.UPRN, &candidate.LocAddress, &candidate.AddrCan,
			&candidate.Easting, &candidate.Northing, &candidate.USRN,
			&candidate.BLPUClass, &candidate.Status, &candidate.TrgramScore,
		)
		if err != nil {
			continue
		}

		// Compute additional features
		fm.computeFeatures(doc, candidate)

		// Apply filtering
		if fm.passesFilters(doc, candidate) {
			candidates = append(candidates, candidate)
		}
	}

	return candidates, nil
}

// computeFeatures computes all matching features for a candidate
func (fm *FuzzyMatcher) computeFeatures(doc SourceDocument, candidate *FuzzyCandidate) {
	srcAddr := ""
	if doc.AddrCan != nil {
		srcAddr = *doc.AddrCan
	}

	// Basic string similarities
	candidate.JaroScore = jaroSimilarity(srcAddr, candidate.AddrCan)

	// Token analysis
	srcTokens := strings.Fields(srcAddr)
	candTokens := strings.Fields(candidate.AddrCan)

	// Extract specific token types
	srcHouseNums, srcLocalities, srcStreets := extractTokenTypes(srcTokens)
	candHouseNums, candLocalities, candStreets := extractTokenTypes(candTokens)

	// House number matching
	candidate.SameHouseNumber = hasOverlap(srcHouseNums, candHouseNums)
	candidate.SameHouseAlpha = hasAlphaOverlap(srcHouseNums, candHouseNums)

	// Locality and street overlap
	candidate.LocalityOverlap = overlapRatio(srcLocalities, candLocalities)
	candidate.StreetOverlap = overlapRatio(srcStreets, candStreets)

	// Phonetic matching
	candidate.PhoneticHits = normalize.PhoneticTokenOverlap(srcAddr, candidate.AddrCan)

	// Spatial distance if available
	if doc.EastingRaw != nil && doc.NorthingRaw != nil {
		candidate.SpatialDistance = distance(*doc.EastingRaw, *doc.NorthingRaw, 
			candidate.Easting, candidate.Northing)
		candidate.SpatialBoost = math.Exp(-candidate.SpatialDistance / 300.0)
	} else {
		candidate.SpatialBoost = 0.0
	}

	// Compute final score
	candidate.FinalScore = fm.computeFinalScore(candidate)

	// Store all features for explainability
	candidate.Features = map[string]interface{}{
		"trgm_score":        candidate.TrgramScore,
		"jaro_score":        candidate.JaroScore,
		"locality_overlap":  candidate.LocalityOverlap,
		"street_overlap":    candidate.StreetOverlap,
		"same_house_number": candidate.SameHouseNumber,
		"same_house_alpha":  candidate.SameHouseAlpha,
		"phonetic_hits":     candidate.PhoneticHits,
		"spatial_distance":  candidate.SpatialDistance,
		"spatial_boost":     candidate.SpatialBoost,
		"final_score":       candidate.FinalScore,
		"candidate_address": candidate.LocAddress,
		"uprn":             candidate.UPRN,
		"usrn":             candidate.USRN,
		"blpu_class":       candidate.BLPUClass,
		"status":           candidate.Status,
	}
}

// computeFinalScore computes the weighted final score
func (fm *FuzzyMatcher) computeFinalScore(candidate *FuzzyCandidate) float64 {
	score := 0.0

	// Primary similarity scores (90% weight)
	score += 0.50 * candidate.TrgramScore
	score += 0.40 * candidate.JaroScore

	// Structural bonuses (10% weight)  
	score += 0.05 * candidate.LocalityOverlap
	score += 0.05 * candidate.StreetOverlap

	// Discrete bonuses
	if candidate.SameHouseNumber {
		score += 0.08
	}
	if candidate.SameHouseAlpha {
		score += 0.02
	}
	if candidate.PhoneticHits > 0 {
		score += 0.03
	}

	// Spatial boost
	score += candidate.SpatialBoost * 0.05

	// Status bonus for live addresses
	if candidate.Status != nil && *candidate.Status == "1" {
		score += 0.02
	}

	// Penalties
	if candidate.PhoneticHits == 0 && candidate.TrgramScore < 0.85 {
		score -= 0.03 // Penalty for no phonetic hits on lower similarity
	}

	// Clamp to [0, 1]
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

// passesFilters applies filtering logic to reduce false positives
func (fm *FuzzyMatcher) passesFilters(doc SourceDocument, candidate *FuzzyCandidate) bool {
	// If we have phonetic information, require at least some phonetic overlap for lower similarities
	if candidate.TrgramScore < 0.85 && candidate.PhoneticHits == 0 {
		return false
	}

	// If we have house numbers in source, candidate should have compatible numbers
	srcAddr := ""
	if doc.AddrCan != nil {
		srcAddr = *doc.AddrCan
	}
	
	srcHouseNums := extractHouseNumbers(srcAddr)
	candHouseNums := extractHouseNumbers(candidate.AddrCan)
	
	if len(srcHouseNums) > 0 && len(candHouseNums) > 0 && !hasOverlap(srcHouseNums, candHouseNums) {
		// Allow some flexibility for renumbering, but be strict on very different numbers
		if !hasCloseNumbers(srcHouseNums, candHouseNums) {
			return false
		}
	}

	return true
}

// makeDecision decides whether to auto-accept, review, or reject candidates
func (fm *FuzzyMatcher) makeDecision(candidates []*FuzzyCandidate, tiers *FuzzyMatchingTiers) (string, string) {
	if len(candidates) == 0 {
		return "rejected", ""
	}

	// Sort by final score
	best := candidates[0]
	
	// Check for high confidence auto-accept
	if best.FinalScore >= tiers.HighConfidence {
		// Check margin to next candidate
		if len(candidates) == 1 || (candidates[1].FinalScore <= best.FinalScore-tiers.WinnerMargin) {
			return "auto_accepted", best.UPRN
		}
	}

	// Check for medium confidence with validation
	if best.FinalScore >= tiers.MediumConfidence && best.SameHouseNumber && best.LocalityOverlap >= 0.5 {
		if len(candidates) == 1 || (candidates[1].FinalScore <= best.FinalScore-0.05) {
			return "auto_accepted", best.UPRN
		}
	}

	// Check if worth reviewing
	if best.FinalScore >= tiers.LowConfidence {
		return "needs_review", ""
	}

	return "rejected", ""
}

// acceptFuzzyMatch accepts a fuzzy match and records it
func (fm *FuzzyMatcher) acceptFuzzyMatch(engine *MatchEngine, runID, srcID int64, candidate *FuzzyCandidate, method string, score, confidence float64) error {
	// Save match result
	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        method,
		Score:         score,
		Confidence:    confidence,
		TieRank:       1,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "auto_accepted",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Fuzzy match auto-accepted (trgm=%.3f, final=%.3f)", candidate.TrgramScore, candidate.FinalScore),
	}

	if err := engine.SaveMatchResult(result); err != nil {
		return fmt.Errorf("failed to save match result: %w", err)
	}

	// Accept the match
	if err := engine.AcceptMatch(srcID, candidate.UPRN, method, score, confidence, runID, "system"); err != nil {
		return fmt.Errorf("failed to accept match: %w", err)
	}

	return nil
}

// Helper functions for token analysis and scoring

func extractTokenTypes(tokens []string) (houseNumbers, localities, streets []string) {
	for _, token := range tokens {
		if isHouseNumber(token) {
			houseNumbers = append(houseNumbers, token)
		} else if isLikelyLocality(token) {
			localities = append(localities, token)
		} else if isLikelyStreet(token) {
			streets = append(streets, token)
		}
	}
	return
}

func extractHouseNumbers(address string) []string {
	tokens := strings.Fields(address)
	var numbers []string
	
	for _, token := range tokens {
		if isHouseNumber(token) {
			numbers = append(numbers, token)
		}
	}
	return numbers
}

func isHouseNumber(token string) bool {
	// Match numbers with optional alpha suffix: 123, 123A, 123B, etc.
	matched, _ := regexp.MatchString(`^\d+[A-Z]?$`, token)
	return matched
}

func isLikelyLocality(token string) bool {
	// Hampshire localities - extend this list as needed
	localities := []string{
		"ALTON", "PETERSFIELD", "LIPHOOK", "WATERLOOVILLE", "HORNDEAN",
		"FOUR", "MARKS", "BEECH", "ROPLEY", "ALRESFORD", "BORDON",
		"WHITEHILL", "GRAYSHOTT", "HINDHEAD", "HASLEMERE", "SHEET",
		"STEEP", "STROUD", "HAWKLEY", "SELBORNE", "EAST", "WEST",
		"TISTED", "CHAWTON", "HOLYBOURNE", "MEDSTEAD", "BENTLEY",
		"CATHERINGTON", "CLANFIELD", "DENMEAD", "HAMBLEDON", "ROWLANDS",
		"CASTLE", "SOBERTON", "WICKHAM", "DROXFORD", "EXTON", "MEONSTOKE",
	}
	
	return contains(localities, token)
}

func isLikelyStreet(token string) bool {
	streetIndicators := []string{
		"ROAD", "STREET", "AVENUE", "GARDENS", "COURT", "DRIVE",
		"LANE", "PLACE", "SQUARE", "CRESCENT", "TERRACE", "CLOSE",
		"PARK", "WAY", "GREEN", "HEIGHTS", "HILL", "VIEW", "GROVE",
		"RISE", "WALK", "PATH", "MEWS", "YARD", "BROADWAY", "HIGHWAY",
	}
	
	return contains(streetIndicators, token)
}

func hasOverlap(slice1, slice2 []string) bool {
	for _, item1 := range slice1 {
		for _, item2 := range slice2 {
			if item1 == item2 {
				return true
			}
		}
	}
	return false
}

func hasAlphaOverlap(slice1, slice2 []string) bool {
	// Check for alpha suffixes like 12A, 12B
	for _, item1 := range slice1 {
		for _, item2 := range slice2 {
			if len(item1) > 1 && len(item2) > 1 &&
				item1[len(item1)-1:] == item2[len(item2)-1:] && // Same alpha suffix
				item1[:len(item1)-1] == item2[:len(item2)-1] {   // Same number
				return true
			}
		}
	}
	return false
}

func hasCloseNumbers(slice1, slice2 []string) bool {
	// Allow for renumbering within small range (±2)
	for _, item1 := range slice1 {
		num1, err1 := strconv.Atoi(strings.TrimRight(item1, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
		if err1 != nil {
			continue
		}
		
		for _, item2 := range slice2 {
			num2, err2 := strconv.Atoi(strings.TrimRight(item2, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
			if err2 != nil {
				continue
			}
			
			if abs(num1-num2) <= 2 {
				return true
			}
		}
	}
	return false
}

func overlapRatio(slice1, slice2 []string) float64 {
	if len(slice1) == 0 {
		return 0.0
	}
	
	matches := 0
	for _, item1 := range slice1 {
		for _, item2 := range slice2 {
			if item1 == item2 {
				matches++
				break
			}
		}
	}
	
	return float64(matches) / float64(len(slice1))
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func distance(x1, y1, x2, y2 float64) float64 {
	return math.Sqrt((x2-x1)*(x2-x1) + (y2-y1)*(y2-y1))
}

// Simplified Jaro similarity
func jaroSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}
	
	// Simple implementation focusing on character overlap
	// For production, use a proper Jaro-Winkler implementation
	matches := 0
	for i, char1 := range s1 {
		for j, char2 := range s2 {
			if char1 == char2 && abs(i-j) <= max(len(s1), len(s2))/2 {
				matches++
				break
			}
		}
	}
	
	if matches == 0 {
		return 0.0
	}
	
	return float64(matches) / float64(max(len(s1), len(s2)))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}