package matcher

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/ehdc-llpg/internal/debug"
	"github.com/ehdc-llpg/internal/normalize"
)

// HybridEngine combines fast database pre-filtering with advanced Go algorithms
type HybridEngine struct {
	db       *sql.DB
	embedder Embedder
	vectorDB VectorDB
}

// HybridCandidate extends MatchCandidate with additional analysis data
type HybridCandidate struct {
	MatchCandidate
	TokenAnalysis    TokenAnalysis
	SpatialAnalysis  SpatialAnalysis
	SemanticAnalysis SemanticAnalysis
	FinalScore       float64
	Confidence       string
}

// TokenAnalysis holds detailed token-based matching analysis
type TokenAnalysis struct {
	HouseNumberMatch bool
	StreetTokenMatch float64
	LocalityMatch    bool
	PostcodeMatch    bool
	TokenOverlap     float64
}

// SpatialAnalysis holds spatial matching analysis
type SpatialAnalysis struct {
	HasCoordinates bool
	Distance       *float64
	SpatialBoost   float64
	WithinRadius   bool
}

// SemanticAnalysis holds semantic/vector matching analysis
type SemanticAnalysis struct {
	VectorSimilarity *float64
	SemanticBoost    float64
}

// NewHybridEngine creates a new hybrid matching engine
func NewHybridEngine(db *sql.DB, embedder Embedder, vectorDB VectorDB) *HybridEngine {
	return &HybridEngine{
		db:       db,
		embedder: embedder,
		vectorDB: vectorDB,
	}
}

// ProcessDocument performs hybrid address matching
func (e *HybridEngine) ProcessDocument(localDebug bool, input MatchInput) (*MatchResult, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	
	debug.DebugOutput(localDebug, "Processing document %d (HYBRID): %s", input.DocumentID, input.RawAddress)
	
	// Stage 1: Fast Database Pre-filtering
	debug.DebugOutput(localDebug, "=== STAGE 1: Fast Database Pre-filtering ===")
	rawCandidates, err := e.fastDatabasePrefilter(localDebug, input)
	if err != nil {
		return nil, fmt.Errorf("database pre-filtering failed: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Pre-filter found %d raw candidates", len(rawCandidates))
	
	if len(rawCandidates) == 0 {
		return &MatchResult{
			DocumentID:     input.DocumentID,
			Decision:       "no_match",
			MatchStatus:    "auto",
			ProcessingTime: time.Since(startTime),
		}, nil
	}
	
	// Stage 2: Advanced Go Analysis
	debug.DebugOutput(localDebug, "=== STAGE 2: Advanced Go Analysis ===")
	hybridCandidates, err := e.advancedGoAnalysis(localDebug, input, rawCandidates)
	if err != nil {
		return nil, fmt.Errorf("advanced analysis failed: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Advanced analysis processed %d candidates", len(hybridCandidates))
	
	// Stage 3: Intelligent Decision Making
	debug.DebugOutput(localDebug, "=== STAGE 3: Intelligent Decision Making ===")
	finalCandidates, decision, matchStatus := e.intelligentDecisionMaking(localDebug, hybridCandidates)
	
	// Convert back to standard MatchCandidate format
	var standardCandidates []MatchCandidate
	for _, hc := range finalCandidates {
		standardCandidates = append(standardCandidates, hc.MatchCandidate)
	}
	
	var bestCandidate *MatchCandidate
	if len(standardCandidates) > 0 {
		bestCandidate = &standardCandidates[0]
	}
	
	result := &MatchResult{
		DocumentID:     input.DocumentID,
		BestCandidate:  bestCandidate,
		AllCandidates:  standardCandidates,
		Decision:       decision,
		MatchStatus:    matchStatus,
		ProcessingTime: time.Since(startTime),
	}
	
	debug.DebugOutput(localDebug, "HYBRID decision: %s with %d candidates in %v", 
		decision, len(standardCandidates), result.ProcessingTime)
	
	return result, nil
}

// Stage 1: Fast Database Pre-filtering
func (e *HybridEngine) fastDatabasePrefilter(localDebug bool, input MatchInput) ([]MatchCandidate, error) {
	var rawUPRN *string
	if input.RawUPRN != nil && *input.RawUPRN != "" {
		rawUPRN = input.RawUPRN
	}
	
	// Use optimized database function but request more candidates for analysis
	rows, err := e.db.Query(`
		SELECT 
			address_id, location_id, uprn, full_address, address_canonical,
			easting, northing, match_score, match_method
		FROM fast_address_match($1, $2, 100)  -- Get top 100 for analysis
		ORDER BY match_score DESC
	`, input.AddressCanonical, rawUPRN)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var candidates []MatchCandidate
	methodMap := map[string]int{
		"exact_uprn":   1,
		"exact_text":   2, 
		"fuzzy_high":   3,
		"fuzzy_medium": 4,
		"fuzzy_low":    5,
	}
	
	for rows.Next() {
		var candidate MatchCandidate
		var methodCode string
		var score32 float32
		
		err := rows.Scan(
			&candidate.AddressID, &candidate.LocationID, &candidate.UPRN,
			&candidate.FullAddress, &candidate.AddressCanonical,
			&candidate.Easting, &candidate.Northing, &score32, &methodCode,
		)
		if err != nil {
			debug.DebugOutput(localDebug, "Error scanning pre-filter candidate: %v", err)
			continue
		}
		
		candidate.Score = float64(score32)
		candidate.MethodCode = methodCode
		if methodID, exists := methodMap[methodCode]; exists {
			candidate.MethodID = methodID
		} else {
			candidate.MethodID = 5
		}
		
		candidate.Features = map[string]interface{}{
			"prefilter_score": float64(score32),
			"prefilter_method": methodCode,
		}
		
		candidates = append(candidates, candidate)
	}
	
	debug.DebugOutput(localDebug, "Database pre-filter returned %d candidates (scores: %.4f to %.4f)", 
		len(candidates), 
		func() float64 { if len(candidates) > 0 { return candidates[0].Score } else { return 0 } }(),
		func() float64 { if len(candidates) > 0 { return candidates[len(candidates)-1].Score } else { return 0 } }())
	
	return candidates, nil
}

// Stage 2: Advanced Go Analysis
func (e *HybridEngine) advancedGoAnalysis(localDebug bool, input MatchInput, rawCandidates []MatchCandidate) ([]HybridCandidate, error) {
	debug.DebugOutput(localDebug, "Starting advanced Go analysis on %d candidates", len(rawCandidates))
	
	// Parse input address components using advanced normalization
	inputCanonical, inputPostcode, _ := normalize.CanonicalAddressDebug(localDebug, input.RawAddress)
	inputHouseNumbers := normalize.ExtractHouseNumbers(inputCanonical)
	inputLocalities := normalize.ExtractLocalityTokens(inputCanonical)
	inputStreetTokens := normalize.TokenizeStreet(inputCanonical)
	
	debug.DebugOutput(localDebug, "Input analysis: canonical='%s', postcode='%s'", inputCanonical, inputPostcode)
	debug.DebugOutput(localDebug, "Input tokens: house=%v, localities=%v, streets=%v", inputHouseNumbers, inputLocalities, inputStreetTokens)
	
	var hybridCandidates []HybridCandidate
	
	for i, rawCandidate := range rawCandidates {
		debug.DebugOutput(localDebug, "Analyzing candidate %d/%d: %s", i+1, len(rawCandidates), rawCandidate.FullAddress)
		
		hc := HybridCandidate{
			MatchCandidate: rawCandidate,
		}
		
		// Token Analysis
		hc.TokenAnalysis = e.performTokenAnalysis(localDebug, inputHouseNumbers, inputLocalities, inputStreetTokens, inputPostcode, rawCandidate)
		
		// Spatial Analysis
		hc.SpatialAnalysis = e.performSpatialAnalysis(localDebug, input, rawCandidate)
		
		// Semantic Analysis (if available)
		hc.SemanticAnalysis = e.performSemanticAnalysis(localDebug, inputCanonical, rawCandidate)
		
		// Calculate enhanced final score
		hc.FinalScore = e.calculateHybridScore(localDebug, hc)
		hc.Confidence = e.determineConfidenceLevel(hc)
		
		// Update the match candidate with hybrid features
		if hc.MatchCandidate.Features == nil {
			hc.MatchCandidate.Features = make(map[string]interface{})
		}
		hc.MatchCandidate.Features["hybrid_score"] = hc.FinalScore
		hc.MatchCandidate.Features["confidence"] = hc.Confidence
		hc.MatchCandidate.Features["token_analysis"] = hc.TokenAnalysis
		hc.MatchCandidate.Features["spatial_analysis"] = hc.SpatialAnalysis
		
		// Update the score to the hybrid score
		hc.MatchCandidate.Score = hc.FinalScore
		
		hybridCandidates = append(hybridCandidates, hc)
		
		debug.DebugOutput(localDebug, "  -> Hybrid score: %.4f, Confidence: %s", hc.FinalScore, hc.Confidence)
	}
	
	return hybridCandidates, nil
}

// Token Analysis using advanced Go algorithms
func (e *HybridEngine) performTokenAnalysis(localDebug bool, inputHouseNumbers, inputLocalities, inputStreetTokens []string, inputPostcode string, candidate MatchCandidate) TokenAnalysis {
	analysis := TokenAnalysis{}
	
	// Parse candidate address
	candCanonical, candPostcode, _ := normalize.CanonicalAddress(candidate.FullAddress)
	candHouseNumbers := normalize.ExtractHouseNumbers(candCanonical)
	candLocalities := normalize.ExtractLocalityTokens(candCanonical)
	candStreetTokens := normalize.TokenizeStreet(candCanonical)
	
	// House number matching
	analysis.HouseNumberMatch = false
	for _, inputHN := range inputHouseNumbers {
		for _, candHN := range candHouseNumbers {
			if inputHN == candHN {
				analysis.HouseNumberMatch = true
				break
			}
		}
	}
	
	// Street token matching
	analysis.StreetTokenMatch = normalize.TokenOverlap(inputStreetTokens, candStreetTokens)
	
	// Locality matching
	analysis.LocalityMatch = false
	for _, inputLoc := range inputLocalities {
		for _, candLoc := range candLocalities {
			if inputLoc == candLoc {
				analysis.LocalityMatch = true
				break
			}
		}
	}
	
	// Postcode matching
	analysis.PostcodeMatch = (inputPostcode != "" && candPostcode != "" && inputPostcode == candPostcode)
	
	// Overall token overlap
	inputAllTokens := append(append(inputHouseNumbers, inputLocalities...), inputStreetTokens...)
	candAllTokens := append(append(candHouseNumbers, candLocalities...), candStreetTokens...)
	analysis.TokenOverlap = normalize.TokenOverlap(inputAllTokens, candAllTokens)
	
	debug.DebugOutput(localDebug, "    Token analysis: house=%t, street=%.3f, locality=%t, postcode=%t, overlap=%.3f", 
		analysis.HouseNumberMatch, analysis.StreetTokenMatch, analysis.LocalityMatch, analysis.PostcodeMatch, analysis.TokenOverlap)
	
	return analysis
}

// Spatial Analysis using coordinate-based logic
func (e *HybridEngine) performSpatialAnalysis(localDebug bool, input MatchInput, candidate MatchCandidate) SpatialAnalysis {
	analysis := SpatialAnalysis{}
	
	if input.RawEasting == nil || input.RawNorthing == nil || candidate.Easting == nil || candidate.Northing == nil {
		analysis.HasCoordinates = false
		return analysis
	}
	
	if *input.RawEasting == "" || *input.RawNorthing == "" {
		analysis.HasCoordinates = false
		return analysis
	}
	
	analysis.HasCoordinates = true
	
	// Parse input coordinates
	inputEasting, err1 := strconv.ParseFloat(*input.RawEasting, 64)
	inputNorthing, err2 := strconv.ParseFloat(*input.RawNorthing, 64)
	
	if err1 != nil || err2 != nil {
		debug.DebugOutput(localDebug, "    Failed to parse input coordinates: %v, %v", err1, err2)
		return analysis
	}
	
	// Calculate distance (simplified Euclidean in BNG coordinates)
	de := inputEasting - *candidate.Easting
	dn := inputNorthing - *candidate.Northing
	distance := (de*de + dn*dn) * 0.5
	analysis.Distance = &distance
	
	// Spatial boost calculation
	analysis.SpatialBoost = calculateSpatialBoost(distance)
	analysis.WithinRadius = distance <= 2000.0 // 2km radius
	
	debug.DebugOutput(localDebug, "    Spatial analysis: distance=%.1fm, boost=%.4f, within_radius=%t", 
		distance, analysis.SpatialBoost, analysis.WithinRadius)
	
	return analysis
}

// Semantic Analysis using vector similarity (if available)
func (e *HybridEngine) performSemanticAnalysis(localDebug bool, inputCanonical string, candidate MatchCandidate) SemanticAnalysis {
	analysis := SemanticAnalysis{}
	
	if e.embedder == nil || e.vectorDB == nil {
		debug.DebugOutput(localDebug, "    Semantic analysis: embedder/vectorDB not available")
		return analysis
	}
	
	// This would be implemented when embeddings are available
	// For now, return empty analysis
	debug.DebugOutput(localDebug, "    Semantic analysis: not yet implemented")
	
	return analysis
}

// Calculate hybrid score combining all analysis factors
func (e *HybridEngine) calculateHybridScore(localDebug bool, hc HybridCandidate) float64 {
	baseScore := hc.MatchCandidate.Score
	
	// Token-based boosts
	tokenBoost := 0.0
	if hc.TokenAnalysis.HouseNumberMatch {
		tokenBoost += 0.15 // Significant boost for house number match
	}
	if hc.TokenAnalysis.LocalityMatch {
		tokenBoost += 0.10 // Good boost for locality match
	}
	if hc.TokenAnalysis.PostcodeMatch {
		tokenBoost += 0.20 // Very strong boost for postcode match
	}
	tokenBoost += hc.TokenAnalysis.StreetTokenMatch * 0.10 // Variable boost for street similarity
	tokenBoost += hc.TokenAnalysis.TokenOverlap * 0.05     // Small boost for overall token overlap
	
	// Spatial boost
	spatialBoost := 0.0
	if hc.SpatialAnalysis.HasCoordinates {
		spatialBoost = hc.SpatialAnalysis.SpatialBoost
	}
	
	// Semantic boost (when available)
	semanticBoost := hc.SemanticAnalysis.SemanticBoost
	
	// Calculate final score (cap at 1.0)
	finalScore := baseScore + tokenBoost + spatialBoost + semanticBoost
	if finalScore > 1.0 {
		finalScore = 1.0
	}
	
	debug.DebugOutput(localDebug, "    Score calculation: base=%.4f + token=%.4f + spatial=%.4f + semantic=%.4f = %.4f", 
		baseScore, tokenBoost, spatialBoost, semanticBoost, finalScore)
	
	return finalScore
}

// Determine confidence level based on analysis
func (e *HybridEngine) determineConfidenceLevel(hc HybridCandidate) string {
	score := hc.FinalScore
	ta := hc.TokenAnalysis
	sa := hc.SpatialAnalysis
	
	// Very High Confidence
	if score >= 0.95 || (score >= 0.90 && ta.HouseNumberMatch && ta.LocalityMatch) {
		return "very_high"
	}
	
	// High Confidence
	if score >= 0.85 || (score >= 0.80 && ta.PostcodeMatch) {
		return "high"
	}
	
	// Medium Confidence
	if score >= 0.75 || (score >= 0.70 && (ta.HouseNumberMatch || ta.LocalityMatch)) {
		return "medium"
	}
	
	// Low Confidence
	if score >= 0.60 || (sa.HasCoordinates && sa.WithinRadius) {
		return "low"
	}
	
	return "very_low"
}

// Stage 3: Intelligent Decision Making
func (e *HybridEngine) intelligentDecisionMaking(localDebug bool, candidates []HybridCandidate) ([]HybridCandidate, string, string) {
	if len(candidates) == 0 {
		return candidates, "no_match", "auto"
	}
	
	// Sort by final hybrid score
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].FinalScore > candidates[i].FinalScore {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	
	bestCandidate := candidates[0]
	
	debug.DebugOutput(localDebug, "Best candidate: score=%.4f, confidence=%s, method=%s", 
		bestCandidate.FinalScore, bestCandidate.Confidence, bestCandidate.MethodCode)
	
	// Intelligent decision making based on confidence and score
	switch bestCandidate.Confidence {
	case "very_high":
		return candidates, "auto_accept", "auto"
	case "high":
		if bestCandidate.FinalScore >= 0.85 {
			return candidates, "auto_accept", "auto"
		}
		return candidates, "needs_review", "manual"
	case "medium":
		if bestCandidate.FinalScore >= 0.80 && bestCandidate.TokenAnalysis.HouseNumberMatch {
			return candidates, "auto_accept", "auto"
		}
		return candidates, "needs_review", "manual"
	case "low":
		return candidates, "needs_review", "manual"
	default: // very_low
		return candidates, "low_confidence", "manual"
	}
}

// SaveMatchResult saves the matching result to the database
func (e *HybridEngine) SaveMatchResult(localDebug bool, result *MatchResult) error {
	if result.BestCandidate == nil {
		debug.DebugOutput(localDebug, "No match to save for document %d", result.DocumentID)
		return nil
	}
	
	_, err := e.db.Exec(`
		INSERT INTO address_match (
			document_id, address_id, location_id, match_method_id,
			confidence_score, match_status, matched_by, matched_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
	`,
		result.DocumentID,
		result.BestCandidate.AddressID,
		result.BestCandidate.LocationID,
		result.BestCandidate.MethodID,
		result.BestCandidate.Score,
		result.MatchStatus,
		"system_hybrid",
	)
	
	if err != nil {
		return fmt.Errorf("failed to save hybrid match result: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Saved hybrid match result for document %d -> address %d (%.4f)", 
		result.DocumentID, result.BestCandidate.AddressID, result.BestCandidate.Score)
	
	return nil
}