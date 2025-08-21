package matcher

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ehdc-llpg/internal/debug"
	"github.com/ehdc-llpg/internal/normalize"
)

// Engine handles address matching using the normalized schema
type Engine struct {
	db       *sql.DB
	embedder Embedder
	vectorDB VectorDB
}

// Embedder interface for generating embeddings
type Embedder interface {
	Embed(text string) ([]float32, error)
}

// VectorDB interface for vector similarity search
type VectorDB interface {
	Query(vector []float32, limit int) ([]VectorResult, error)
}

// VectorResult represents a vector search result
type VectorResult struct {
	AddressID int
	Score     float64
}

// MatchInput represents a source document to be matched
type MatchInput struct {
	DocumentID       int64
	RawAddress       string
	AddressCanonical string
	RawUPRN          *string
	RawEasting       *string
	RawNorthing      *string
}

// MatchCandidate represents a potential address match
type MatchCandidate struct {
	AddressID        int
	LocationID       int
	UPRN             string
	FullAddress      string
	AddressCanonical string
	Easting          *float64
	Northing         *float64
	Score            float64
	MethodID         int
	MethodCode       string
	Features         map[string]interface{}
}

// MatchResult represents the final matching decision
type MatchResult struct {
	DocumentID      int64
	BestCandidate   *MatchCandidate
	AllCandidates   []MatchCandidate
	Decision        string // 'auto_accept', 'needs_review', 'no_match'
	MatchStatus     string // 'auto', 'manual', 'rejected'
	ProcessingTime  time.Duration
}

// MatchMethod represents a matching method from dim_match_method
type MatchMethod struct {
	MethodID             int
	MethodCode           string
	MethodName           string
	ConfidenceThreshold  float64
}

// NewEngine creates a new matching engine
func NewEngine(db *sql.DB, embedder Embedder, vectorDB VectorDB) *Engine {
	return &Engine{
		db:       db,
		embedder: embedder,
		vectorDB: vectorDB,
	}
}

// ProcessDocument performs address matching for a single source document
func (e *Engine) ProcessDocument(localDebug bool, input MatchInput) (*MatchResult, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	
	debug.DebugOutput(localDebug, "Processing document %d: %s", input.DocumentID, input.RawAddress)
	
	// Load match methods
	methods, err := e.loadMatchMethods()
	if err != nil {
		return nil, fmt.Errorf("failed to load match methods: %w", err)
	}
	
	var allCandidates []MatchCandidate
	
	// Tier A: Deterministic Matching
	debug.DebugOutput(localDebug, "=== Tier A: Deterministic Matching ===")
	
	// A1: Legacy UPRN validation
	if input.RawUPRN != nil && *input.RawUPRN != "" {
		candidates, err := e.exactUPRNMatch(localDebug, *input.RawUPRN, methods["exact_uprn"])
		if err == nil {
			allCandidates = append(allCandidates, candidates...)
			debug.DebugOutput(localDebug, "Found %d exact UPRN matches", len(candidates))
		}
	}
	
	// A2: Exact canonical address match
	candidates, err := e.exactTextMatch(localDebug, input.AddressCanonical, methods["exact_text"])
	if err == nil {
		allCandidates = append(allCandidates, candidates...)
		debug.DebugOutput(localDebug, "Found %d exact text matches", len(candidates))
	}
	
	// If we have high-confidence deterministic matches, use them
	if len(allCandidates) > 0 && allCandidates[0].Score >= 0.95 {
		result := &MatchResult{
			DocumentID:     input.DocumentID,
			BestCandidate:  &allCandidates[0],
			AllCandidates:  allCandidates,
			Decision:       "auto_accept",
			MatchStatus:    "auto",
			ProcessingTime: time.Since(startTime),
		}
		debug.DebugOutput(localDebug, "High-confidence deterministic match found: %.4f", allCandidates[0].Score)
		return result, nil
	}
	
	// Tier B: Fuzzy Text Matching
	debug.DebugOutput(localDebug, "=== Tier B: Fuzzy Text Matching ===")
	
	// B1: High confidence fuzzy matching
	candidates, err = e.fuzzyTextMatch(localDebug, input.AddressCanonical, 0.90, 20, methods["fuzzy_high"])
	if err == nil {
		allCandidates = append(allCandidates, candidates...)
		debug.DebugOutput(localDebug, "Found %d high-confidence fuzzy matches", len(candidates))
	}
	
	// B2: Medium confidence fuzzy matching
	candidates, err = e.fuzzyTextMatch(localDebug, input.AddressCanonical, 0.80, 50, methods["fuzzy_medium"])
	if err == nil {
		allCandidates = append(allCandidates, candidates...)
		debug.DebugOutput(localDebug, "Found %d medium-confidence fuzzy matches", len(candidates))
	}
	
	// B3: Low confidence fuzzy matching
	candidates, err = e.fuzzyTextMatch(localDebug, input.AddressCanonical, 0.70, 100, methods["fuzzy_low"])
	if err == nil {
		allCandidates = append(allCandidates, candidates...)
		debug.DebugOutput(localDebug, "Found %d low-confidence fuzzy matches", len(candidates))
	}
	
	// Tier C: Vector Semantic Matching
	debug.DebugOutput(localDebug, "=== Tier C: Vector Semantic Matching ===")
	if e.embedder != nil && e.vectorDB != nil {
		candidates, err = e.vectorSemanticMatch(localDebug, input.AddressCanonical, 50, methods["spatial"])
		if err == nil {
			allCandidates = append(allCandidates, candidates...)
			debug.DebugOutput(localDebug, "Found %d vector semantic matches", len(candidates))
		} else {
			debug.DebugOutput(localDebug, "Vector matching failed: %v", err)
		}
	}
	
	// Tier D: Spatial Filtering
	debug.DebugOutput(localDebug, "=== Tier D: Spatial Filtering ===")
	if input.RawEasting != nil && input.RawNorthing != nil {
		allCandidates = e.applySpatialFilter(localDebug, allCandidates, input.RawEasting, input.RawNorthing, 2000.0)
	}
	
	// Deduplicate and rank candidates
	allCandidates = e.deduplicateAndRank(allCandidates)
	
	// Make final decision
	decision, matchStatus := e.makeMatchingDecision(allCandidates, methods)
	
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
	
	debug.DebugOutput(localDebug, "Final decision: %s with %d candidates", decision, len(allCandidates))
	return result, nil
}

// loadMatchMethods loads match methods from dim_match_method table
func (e *Engine) loadMatchMethods() (map[string]MatchMethod, error) {
	methods := make(map[string]MatchMethod)
	
	rows, err := e.db.Query(`
		SELECT method_id, method_code, method_name, confidence_threshold
		FROM dim_match_method
		ORDER BY method_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	for rows.Next() {
		var method MatchMethod
		err := rows.Scan(&method.MethodID, &method.MethodCode, &method.MethodName, &method.ConfidenceThreshold)
		if err != nil {
			return nil, err
		}
		methods[method.MethodCode] = method
	}
	
	return methods, nil
}

// exactUPRNMatch performs exact UPRN matching
func (e *Engine) exactUPRNMatch(localDebug bool, uprn string, method MatchMethod) ([]MatchCandidate, error) {
	trimmedUPRN := strings.TrimSpace(uprn)
	if trimmedUPRN == "" {
		return []MatchCandidate{}, nil
	}
	
	rows, err := e.db.Query(`
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.uprn = $1
	`, trimmedUPRN)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var candidates []MatchCandidate
	for rows.Next() {
		var candidate MatchCandidate
		err := rows.Scan(
			&candidate.AddressID, &candidate.LocationID, &candidate.UPRN,
			&candidate.FullAddress, &candidate.AddressCanonical,
			&candidate.Easting, &candidate.Northing,
		)
		if err != nil {
			continue
		}
		
		candidate.Score = 1.0 // Perfect score for exact UPRN match
		candidate.MethodID = method.MethodID
		candidate.MethodCode = method.MethodCode
		candidate.Features = map[string]interface{}{
			"exact_uprn_match": true,
		}
		
		candidates = append(candidates, candidate)
	}
	
	return candidates, nil
}

// exactTextMatch performs exact canonical text matching
func (e *Engine) exactTextMatch(localDebug bool, canonical string, method MatchMethod) ([]MatchCandidate, error) {
	if canonical == "" {
		return []MatchCandidate{}, nil
	}
	
	rows, err := e.db.Query(`
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.address_canonical = $1
	`, canonical)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var candidates []MatchCandidate
	for rows.Next() {
		var candidate MatchCandidate
		err := rows.Scan(
			&candidate.AddressID, &candidate.LocationID, &candidate.UPRN,
			&candidate.FullAddress, &candidate.AddressCanonical,
			&candidate.Easting, &candidate.Northing,
		)
		if err != nil {
			continue
		}
		
		candidate.Score = 0.99 // Very high score for exact text match
		candidate.MethodID = method.MethodID
		candidate.MethodCode = method.MethodCode
		candidate.Features = map[string]interface{}{
			"exact_text_match": true,
		}
		
		candidates = append(candidates, candidate)
	}
	
	return candidates, nil
}

// fuzzyTextMatch performs fuzzy text matching using trigrams
func (e *Engine) fuzzyTextMatch(localDebug bool, canonical string, threshold float64, limit int, method MatchMethod) ([]MatchCandidate, error) {
	if canonical == "" {
		return []MatchCandidate{}, nil
	}
	
	rows, err := e.db.Query(`
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing, similarity($1, a.address_canonical) AS fuzzy_score
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.address_canonical % $1
		  AND similarity($1, a.address_canonical) >= $2
		ORDER BY fuzzy_score DESC
		LIMIT $3
	`, canonical, threshold, limit)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var candidates []MatchCandidate
	for rows.Next() {
		var candidate MatchCandidate
		var fuzzyScore float64
		err := rows.Scan(
			&candidate.AddressID, &candidate.LocationID, &candidate.UPRN,
			&candidate.FullAddress, &candidate.AddressCanonical,
			&candidate.Easting, &candidate.Northing, &fuzzyScore,
		)
		if err != nil {
			continue
		}
		
		candidate.Score = fuzzyScore
		candidate.MethodID = method.MethodID
		candidate.MethodCode = method.MethodCode
		candidate.Features = map[string]interface{}{
			"trigram_similarity": fuzzyScore,
			"threshold_used":     threshold,
		}
		
		candidates = append(candidates, candidate)
	}
	
	return candidates, nil
}

// vectorSemanticMatch performs semantic matching using embeddings
func (e *Engine) vectorSemanticMatch(localDebug bool, canonical string, limit int, method MatchMethod) ([]MatchCandidate, error) {
	if e.embedder == nil || e.vectorDB == nil {
		return []MatchCandidate{}, nil
	}
	
	// Generate embedding for input address
	embedding, err := e.embedder.Embed(canonical)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}
	
	// Query vector database
	vectorResults, err := e.vectorDB.Query(embedding, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}
	
	var candidates []MatchCandidate
	for _, vr := range vectorResults {
		// Look up full address details
		var candidate MatchCandidate
		err := e.db.QueryRow(`
			SELECT 
				a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
				l.easting, l.northing
			FROM dim_address a
			INNER JOIN dim_location l ON l.location_id = a.location_id
			WHERE a.address_id = $1
		`, vr.AddressID).Scan(
			&candidate.AddressID, &candidate.LocationID, &candidate.UPRN,
			&candidate.FullAddress, &candidate.AddressCanonical,
			&candidate.Easting, &candidate.Northing,
		)
		
		if err != nil {
			debug.DebugOutput(localDebug, "Failed to lookup vector result address_id %d: %v", vr.AddressID, err)
			continue
		}
		
		candidate.Score = vr.Score
		candidate.MethodID = method.MethodID
		candidate.MethodCode = method.MethodCode
		candidate.Features = map[string]interface{}{
			"embedding_cosine": vr.Score,
		}
		
		candidates = append(candidates, candidate)
	}
	
	return candidates, nil
}

// applySpatialFilter filters candidates by spatial proximity
func (e *Engine) applySpatialFilter(localDebug bool, candidates []MatchCandidate, eastingStr, northingStr *string, radiusMeters float64) []MatchCandidate {
	if eastingStr == nil || northingStr == nil || *eastingStr == "" || *northingStr == "" {
		return candidates
	}
	
	// Parse coordinates
	easting, err := normalize.ParseFloat(*eastingStr)
	if err != nil {
		return candidates
	}
	northing, err2 := normalize.ParseFloat(*northingStr)
	if err2 != nil {
		return candidates
	}
	
	var filtered []MatchCandidate
	for _, cand := range candidates {
		if cand.Easting == nil || cand.Northing == nil {
			continue
		}
		
		// Calculate distance (simplified Euclidean)
		de := easting - *cand.Easting
		dn := northing - *cand.Northing
		distance := (de*de + dn*dn) * 0.5
		
		if distance <= radiusMeters {
			cand.Features["distance_meters"] = distance
			cand.Features["spatial_boost"] = calculateSpatialBoost(distance)
			filtered = append(filtered, cand)
		}
	}
	
	debug.DebugOutput(localDebug, "Spatial filter: %d -> %d candidates within %.0fm", len(candidates), len(filtered), radiusMeters)
	return filtered
}

// deduplicateAndRank removes duplicate addresses and ranks by score
func (e *Engine) deduplicateAndRank(candidates []MatchCandidate) []MatchCandidate {
	addressMap := make(map[int]MatchCandidate)
	
	for _, cand := range candidates {
		existing, exists := addressMap[cand.AddressID]
		if !exists || cand.Score > existing.Score {
			addressMap[cand.AddressID] = cand
		}
	}
	
	var deduped []MatchCandidate
	for _, cand := range addressMap {
		deduped = append(deduped, cand)
	}
	
	// Sort by score descending
	for i := 0; i < len(deduped)-1; i++ {
		for j := i + 1; j < len(deduped); j++ {
			if deduped[j].Score > deduped[i].Score {
				deduped[i], deduped[j] = deduped[j], deduped[i]
			}
		}
	}
	
	return deduped
}

// makeMatchingDecision determines the final matching decision
func (e *Engine) makeMatchingDecision(candidates []MatchCandidate, methods map[string]MatchMethod) (string, string) {
	if len(candidates) == 0 {
		return "no_match", "auto"
	}
	
	bestCandidate := candidates[0]
	
	// Get the confidence threshold for the best method
	var threshold float64 = 0.90 // Default high threshold
	for _, method := range methods {
		if method.MethodID == bestCandidate.MethodID {
			threshold = method.ConfidenceThreshold
			break
		}
	}
	
	if bestCandidate.Score >= threshold {
		return "auto_accept", "auto"
	} else if bestCandidate.Score >= 0.70 {
		return "needs_review", "manual"
	} else {
		return "low_confidence", "manual"
	}
}

// calculateSpatialBoost returns boost factor based on distance
func calculateSpatialBoost(distance float64) float64 {
	if distance <= 0 {
		return 0.10 // Maximum boost for exact location
	}
	boost := 0.10 * (1.0 - distance/2000.0) // Linear decay over 2km
	if boost < 0 {
		boost = 0
	}
	return boost
}

// SaveMatchResult saves the matching result to the database
func (e *Engine) SaveMatchResult(localDebug bool, result *MatchResult) error {
	if result.BestCandidate == nil {
		debug.DebugOutput(localDebug, "No match to save for document %d", result.DocumentID)
		return nil
	}
	
	_, err := e.db.Exec(`
		INSERT INTO address_match (
			document_id, address_id, location_id, match_method_id,
			confidence_score, match_status, matched_by, matched_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		result.DocumentID,
		result.BestCandidate.AddressID,
		result.BestCandidate.LocationID,
		result.BestCandidate.MethodID,
		result.BestCandidate.Score,
		result.MatchStatus,
		"system",
		time.Now(),
	)
	
	if err != nil {
		return fmt.Errorf("failed to save match result: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Saved match result for document %d -> address %d (%.4f)", 
		result.DocumentID, result.BestCandidate.AddressID, result.BestCandidate.Score)
	
	return nil
}