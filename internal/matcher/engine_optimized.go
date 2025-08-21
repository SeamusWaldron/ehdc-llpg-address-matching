package matcher

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ehdc-llpg/internal/debug"
)

// OptimizedEngine uses database functions for faster matching
type OptimizedEngine struct {
	db *sql.DB
}

// NewOptimizedEngine creates a new optimized matching engine
func NewOptimizedEngine(db *sql.DB) *OptimizedEngine {
	return &OptimizedEngine{
		db: db,
	}
}

// ProcessDocument performs optimized address matching using database functions
func (e *OptimizedEngine) ProcessDocument(localDebug bool, input MatchInput) (*MatchResult, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	
	debug.DebugOutput(localDebug, "Processing document %d (optimized): %s", input.DocumentID, input.RawAddress)
	
	var rawUPRN *string
	if input.RawUPRN != nil && *input.RawUPRN != "" {
		rawUPRN = input.RawUPRN
	}
	
	// Use the optimized database function for all matching tiers
	rows, err := e.db.Query(`
		SELECT 
			address_id, location_id, uprn, full_address, address_canonical,
			easting, northing, match_score, match_method
		FROM fast_address_match($1, $2, 50)
		ORDER BY match_score DESC
	`, input.AddressCanonical, rawUPRN)
	
	if err != nil {
		return nil, fmt.Errorf("optimized matching failed: %w", err)
	}
	defer rows.Close()
	
	var allCandidates []MatchCandidate
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
			debug.DebugOutput(localDebug, "Error scanning candidate: %v", err)
			continue
		}
		
		candidate.Score = float64(score32)
		candidate.MethodCode = methodCode
		if methodID, exists := methodMap[methodCode]; exists {
			candidate.MethodID = methodID
		} else {
			candidate.MethodID = 5 // Default to fuzzy_low
		}
		
		candidate.Features = map[string]interface{}{
			"optimized_match": true,
			"method_code":     methodCode,
		}
		
		allCandidates = append(allCandidates, candidate)
	}
	
	debug.DebugOutput(localDebug, "Found %d candidates with optimized matching", len(allCandidates))
	
	// Apply spatial filtering if coordinates available
	if input.RawEasting != nil && input.RawNorthing != nil {
		allCandidates = e.applySpatialFilter(localDebug, allCandidates, input.RawEasting, input.RawNorthing, 2000.0)
	}
	
	// Make final decision
	decision, matchStatus := e.makeOptimizedDecision(allCandidates)
	
	var bestCandidate *MatchCandidate
	if len(allCandidates) > 0 {
		bestCandidate = &allCandidates[0]
	}
	
	result := &MatchResult{
		DocumentID:     input.DocumentID,
		BestCandidate:  bestCandidate,
		AllCandidates:  allCandidates,
		Decision:       decision,
		MatchStatus:    matchStatus,
		ProcessingTime: time.Since(startTime),
	}
	
	debug.DebugOutput(localDebug, "Optimized decision: %s with %d candidates in %v", 
		decision, len(allCandidates), result.ProcessingTime)
	
	return result, nil
}

// applySpatialFilter filters candidates by spatial proximity (simplified version)
func (e *OptimizedEngine) applySpatialFilter(localDebug bool, candidates []MatchCandidate, eastingStr, northingStr *string, radiusMeters float64) []MatchCandidate {
	if eastingStr == nil || northingStr == nil || *eastingStr == "" || *northingStr == "" {
		return candidates
	}
	
	// Use database for coordinate parsing and filtering
	var easting, northing float64
	err := e.db.QueryRow("SELECT $1::NUMERIC, $2::NUMERIC", *eastingStr, *northingStr).Scan(&easting, &northing)
	if err != nil {
		debug.DebugOutput(localDebug, "Failed to parse coordinates: %v", err)
		return candidates
	}
	
	var filtered []MatchCandidate
	for _, cand := range candidates {
		if cand.Easting == nil || cand.Northing == nil {
			continue
		}
		
		// Simplified distance calculation
		de := easting - *cand.Easting
		dn := northing - *cand.Northing
		distance := (de*de + dn*dn) * 0.5
		
		if distance <= radiusMeters {
			cand.Features["distance_meters"] = distance
			cand.Features["spatial_boost"] = calculateSpatialBoost(distance)
			filtered = append(filtered, cand)
		}
	}
	
	debug.DebugOutput(localDebug, "Spatial filter: %d -> %d candidates within %.0fm", 
		len(candidates), len(filtered), radiusMeters)
	return filtered
}

// makeOptimizedDecision determines the final matching decision
func (e *OptimizedEngine) makeOptimizedDecision(candidates []MatchCandidate) (string, string) {
	if len(candidates) == 0 {
		return "no_match", "auto"
	}
	
	bestCandidate := candidates[0]
	
	// Optimized thresholds based on method
	switch bestCandidate.MethodCode {
	case "exact_uprn":
		return "auto_accept", "auto"
	case "exact_text":
		if bestCandidate.Score >= 0.95 {
			return "auto_accept", "auto"
		}
	case "fuzzy_high":
		if bestCandidate.Score >= 0.90 {
			return "auto_accept", "auto"
		}
	case "fuzzy_medium":
		if bestCandidate.Score >= 0.85 {
			return "auto_accept", "auto"
		} else if bestCandidate.Score >= 0.75 {
			return "needs_review", "manual"
		}
	case "fuzzy_low":
		if bestCandidate.Score >= 0.75 {
			return "needs_review", "manual"
		} else {
			return "low_confidence", "manual"
		}
	}
	
	// Default thresholds
	if bestCandidate.Score >= 0.85 {
		return "auto_accept", "auto"
	} else if bestCandidate.Score >= 0.70 {
		return "needs_review", "manual"
	} else {
		return "low_confidence", "manual"
	}
}

// SaveMatchResult saves the matching result to the database
func (e *OptimizedEngine) SaveMatchResult(localDebug bool, result *MatchResult) error {
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
		"system_optimized",
	)
	
	if err != nil {
		return fmt.Errorf("failed to save optimized match result: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Saved optimized match result for document %d -> address %d (%.4f)", 
		result.DocumentID, result.BestCandidate.AddressID, result.BestCandidate.Score)
	
	return nil
}

// calculateSpatialBoost is defined in engine.go