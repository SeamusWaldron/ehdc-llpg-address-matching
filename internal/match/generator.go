package match

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/ehdc-llpg/internal/debug"
	"github.com/ehdc-llpg/internal/normalize"
)

// Generators handles multi-tier candidate generation
type Generators struct {
	DB        *sql.DB // PostgreSQL with pg_trgm + PostGIS
	VDB       VectorDB // Vector database interface  
	Embedder  Embedder // Embedding service interface
	Parser    Parser   // Address parser interface
}

// VectorDB interface for vector similarity search
type VectorDB interface {
	Query(vector []float32, limit int) ([]VectorResult, error)
	GetVector(uprn string) ([]float32, error)
}

// VectorResult represents a vector search result
type VectorResult struct {
	UPRN  string
	Score float64
}

// Embedder interface for generating embeddings
type Embedder interface {
	Embed(text string) ([]float32, error)
}

// Parser interface for address parsing (libpostal or similar)
type Parser interface {
	Parse(text string) (*ParsedAddress, error)
}

// ParsedAddress represents parsed address components
type ParsedAddress struct {
	HouseNumber string
	Street      string
	City        string
	Postcode    string
	Components  map[string]string
}

// NewGenerators creates a new candidate generator
func NewGenerators(db *sql.DB, vdb VectorDB, embedder Embedder, parser Parser) *Generators {
	return &Generators{
		DB:       db,
		VDB:      vdb,
		Embedder: embedder,
		Parser:   parser,
	}
}

// Generate produces candidate UPRNs using multi-tier approach from ADDRESS_MATCHING_ALGORITHM.md
func (g *Generators) Generate(localDebug bool, input Input, canonical string, tokens []string) ([]Candidate, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	var candidates []Candidate

	debug.DebugOutput(localDebug, "Starting candidate generation for: %s", canonical)

	// Tier A - Deterministic matches
	debug.DebugOutput(localDebug, "=== Tier A: Deterministic Matching ===")
	
	// A1: Legacy UPRN validation
	if input.LegacyUPRN != "" {
		if cand, found := g.lookupUPRN(localDebug, input.LegacyUPRN); found {
			cand.Methods = append(cand.Methods, "legacy_uprn_valid")
			cand.Features["legacy_uprn_hit"] = true
			candidates = append(candidates, cand)
			debug.DebugOutput(localDebug, "Found legacy UPRN match: %s", input.LegacyUPRN)
		}
	}

	// A2: Exact canonical match
	exactCands := g.exactCanonicalMatch(localDebug, canonical)
	for i := range exactCands {
		exactCands[i].Methods = append(exactCands[i].Methods, "addr_exact")
	}
	candidates = append(candidates, exactCands...)
	debug.DebugOutput(localDebug, "Found %d exact canonical matches", len(exactCands))

	// Tier B - Database fuzzy matching
	debug.DebugOutput(localDebug, "=== Tier B: Database Fuzzy Matching ===")

	// B1: Trigram similarity with filtering
	trigramCands := g.trigramMatch(localDebug, canonical, tokens, 0.50, 50)
	for i := range trigramCands {
		trigramCands[i].Methods = append(trigramCands[i].Methods, "trigram")
	}
	candidates = append(candidates, trigramCands...)
	debug.DebugOutput(localDebug, "Found %d trigram matches", len(trigramCands))

	// B2: Phonetic filtering (enhance trigram results)
	candidates = g.applyPhoneticFilter(localDebug, candidates, tokens)

	// B3: Locality filtering
	localities := normalize.ExtractLocalityTokens(input.RawAddress)
	if len(localities) > 0 {
		candidates = g.applyLocalityFilter(localDebug, candidates, localities)
		debug.DebugOutput(localDebug, "Applied locality filter for: %v", localities)
	}

	// B4: House number filtering
	houseNumbers := normalize.ExtractHouseNumbers(input.RawAddress)
	if len(houseNumbers) > 0 {
		candidates = g.applyHouseNumberFilter(localDebug, candidates, houseNumbers)
		debug.DebugOutput(localDebug, "Applied house number filter for: %v", houseNumbers)
	}

	// Tier C - Vector semantic matching
	debug.DebugOutput(localDebug, "=== Tier C: Vector Semantic Matching ===")
	if g.VDB != nil && g.Embedder != nil {
		vectorCands, err := g.vectorMatch(localDebug, canonical, 50)
		if err == nil {
			for i := range vectorCands {
				vectorCands[i].Methods = append(vectorCands[i].Methods, "vector_ann")
			}
			candidates = append(candidates, vectorCands...)
			debug.DebugOutput(localDebug, "Found %d vector matches", len(vectorCands))
		} else {
			debug.DebugOutput(localDebug, "Vector matching failed: %v", err)
		}
	}

	// Tier D - Spatial filtering (if coordinates available)
	if input.Easting != nil && input.Northing != nil {
		debug.DebugOutput(localDebug, "=== Tier D: Spatial Filtering ===")
		candidates = g.applySpatialFilter(localDebug, candidates, *input.Easting, *input.Northing, 2000.0)
		debug.DebugOutput(localDebug, "Applied spatial filter within 2km of (%f, %f)", *input.Easting, *input.Northing)
	}

	// Deduplicate by UPRN (keep highest scoring candidate for each UPRN)
	candidates = g.dedupeByUPRN(candidates)
	debug.DebugOutput(localDebug, "Final candidate count after deduplication: %d", len(candidates))

	return candidates, nil
}

// lookupUPRN validates a legacy UPRN against the LLPG
func (g *Generators) lookupUPRN(localDebug bool, uprn string) (Candidate, bool) {
	trimmedUPRN := strings.TrimSpace(uprn)
	if trimmedUPRN == "" {
		return Candidate{}, false
	}

	var cand Candidate
	err := g.DB.QueryRow(`
		SELECT uprn, locaddress, easting, northing
		FROM dim_address
		WHERE uprn = $1
	`, trimmedUPRN).Scan(&cand.UPRN, &cand.LocAddress, &cand.Easting, &cand.Northing)

	if err != nil {
		debug.DebugOutput(localDebug, "UPRN lookup failed for %s: %v", trimmedUPRN, err)
		return Candidate{}, false
	}

	cand.Score = 1.0 // Perfect score for valid legacy UPRN
	cand.Features = make(map[string]interface{})
	return cand, true
}

// exactCanonicalMatch finds exact canonical address matches
func (g *Generators) exactCanonicalMatch(localDebug bool, canonical string) []Candidate {
	if canonical == "" {
		return []Candidate{}
	}

	rows, err := g.DB.Query(`
		SELECT uprn, locaddress, easting, northing
		FROM dim_address
		WHERE addr_can = $1
	`, canonical)
	
	if err != nil {
		debug.DebugOutput(localDebug, "Exact canonical match failed: %v", err)
		return []Candidate{}
	}
	defer rows.Close()

	var candidates []Candidate
	for rows.Next() {
		var cand Candidate
		err := rows.Scan(&cand.UPRN, &cand.LocAddress, &cand.Easting, &cand.Northing)
		if err != nil {
			debug.DebugOutput(localDebug, "Error scanning exact match: %v", err)
			continue
		}
		cand.Score = 0.99 // Very high score for exact match
		cand.Features = make(map[string]interface{})
		candidates = append(candidates, cand)
	}

	return candidates
}

// trigramMatch uses PostgreSQL pg_trgm for fuzzy matching
func (g *Generators) trigramMatch(localDebug bool, canonical string, tokens []string, threshold float64, limit int) []Candidate {
	if canonical == "" {
		return []Candidate{}
	}

	rows, err := g.DB.Query(`
		SELECT uprn, locaddress, easting, northing, similarity($1, addr_can) AS trgm_score
		FROM dim_address
		WHERE addr_can % $1
		  AND similarity($1, addr_can) >= $2
		ORDER BY trgm_score DESC
		LIMIT $3
	`, canonical, threshold, limit)

	if err != nil {
		debug.DebugOutput(localDebug, "Trigram match failed: %v", err)
		return []Candidate{}
	}
	defer rows.Close()

	var candidates []Candidate
	for rows.Next() {
		var cand Candidate
		var trigramScore float64
		err := rows.Scan(&cand.UPRN, &cand.LocAddress, &cand.Easting, &cand.Northing, &trigramScore)
		if err != nil {
			debug.DebugOutput(localDebug, "Error scanning trigram match: %v", err)
			continue
		}
		cand.Score = trigramScore
		cand.Features = map[string]interface{}{
			"trigram_similarity": trigramScore,
		}
		candidates = append(candidates, cand)
	}

	debug.DebugOutput(localDebug, "Trigram matching found %d candidates", len(candidates))
	return candidates
}

// vectorMatch uses vector similarity search
func (g *Generators) vectorMatch(localDebug bool, canonical string, limit int) ([]Candidate, error) {
	if g.VDB == nil || g.Embedder == nil {
		return []Candidate{}, fmt.Errorf("vector DB or embedder not available")
	}

	// Generate embedding for canonical address
	embedding, err := g.Embedder.Embed(canonical)
	if err != nil {
		return []Candidate{}, fmt.Errorf("failed to embed canonical address: %w", err)
	}

	// Query vector database
	vectorResults, err := g.VDB.Query(embedding, limit)
	if err != nil {
		return []Candidate{}, fmt.Errorf("vector search failed: %w", err)
	}

	var candidates []Candidate
	for _, vr := range vectorResults {
		// Look up full address details from PostgreSQL
		var cand Candidate
		err := g.DB.QueryRow(`
			SELECT uprn, locaddress, easting, northing
			FROM dim_address
			WHERE uprn = $1
		`, vr.UPRN).Scan(&cand.UPRN, &cand.LocAddress, &cand.Easting, &cand.Northing)

		if err != nil {
			debug.DebugOutput(localDebug, "Failed to lookup vector result UPRN %s: %v", vr.UPRN, err)
			continue
		}

		cand.Score = vr.Score
		cand.Features = map[string]interface{}{
			"embedding_cosine": vr.Score,
		}
		candidates = append(candidates, cand)
	}

	return candidates, nil
}

// applyPhoneticFilter enhances candidates with phonetic matching
func (g *Generators) applyPhoneticFilter(localDebug bool, candidates []Candidate, tokens []string) []Candidate {
	// For now, just add phonetic features to candidates
	// Could be enhanced to filter candidates based on phonetic similarity
	for i := range candidates {
		if candidates[i].Features == nil {
			candidates[i].Features = make(map[string]interface{})
		}
		candidates[i].Features["phonetic_processing"] = true
	}
	return candidates
}

// applyLocalityFilter keeps only candidates that match locality tokens
func (g *Generators) applyLocalityFilter(localDebug bool, candidates []Candidate, localities []string) []Candidate {
	if len(localities) == 0 {
		return candidates
	}

	var filtered []Candidate
	localitySet := make(map[string]bool)
	for _, locality := range localities {
		localitySet[strings.ToUpper(locality)] = true
	}

	for _, cand := range candidates {
		candLocalities := normalize.ExtractLocalityTokens(cand.LocAddress)
		hasMatch := false
		for _, candLoc := range candLocalities {
			if localitySet[strings.ToUpper(candLoc)] {
				hasMatch = true
				break
			}
		}
		if hasMatch || len(candLocalities) == 0 { // Keep if match or no locality data
			filtered = append(filtered, cand)
		}
	}

	debug.DebugOutput(localDebug, "Locality filter: %d -> %d candidates", len(candidates), len(filtered))
	return filtered
}

// applyHouseNumberFilter prefers candidates with matching house numbers
func (g *Generators) applyHouseNumberFilter(localDebug bool, candidates []Candidate, houseNumbers []string) []Candidate {
	if len(houseNumbers) == 0 {
		return candidates
	}

	numberSet := make(map[string]bool)
	for _, num := range houseNumbers {
		numberSet[strings.ToUpper(num)] = true
	}

	// Boost scores for candidates with matching house numbers
	for i := range candidates {
		candNumbers := normalize.ExtractHouseNumbers(candidates[i].LocAddress)
		hasMatch := false
		for _, candNum := range candNumbers {
			if numberSet[strings.ToUpper(candNum)] {
				hasMatch = true
				break
			}
		}
		if hasMatch {
			candidates[i].Features["same_house_number"] = true
			if candidates[i].Score < 0.95 { // Avoid inflating perfect scores
				candidates[i].Score += 0.05 // Small boost for house number match
			}
		}
	}

	return candidates
}

// applySpatialFilter keeps candidates within distance threshold
func (g *Generators) applySpatialFilter(localDebug bool, candidates []Candidate, easting, northing, radiusMeters float64) []Candidate {
	var filtered []Candidate

	for _, cand := range candidates {
		distance := calculateDistance(easting, northing, cand.Easting, cand.Northing)
		if distance <= radiusMeters {
			cand.Features["distance_meters"] = distance
			cand.Features["spatial_boost"] = calculateSpatialBoost(distance)
			filtered = append(filtered, cand)
		}
	}

	debug.DebugOutput(localDebug, "Spatial filter: %d -> %d candidates within %.0fm", len(candidates), len(filtered), radiusMeters)
	return filtered
}

// dedupeByUPRN removes duplicate UPRNs, keeping the highest scoring candidate
func (g *Generators) dedupeByUPRN(candidates []Candidate) []Candidate {
	uprnMap := make(map[string]Candidate)

	for _, cand := range candidates {
		existing, exists := uprnMap[cand.UPRN]
		if !exists || cand.Score > existing.Score {
			// Merge methods from previous candidate if applicable
			if exists {
				methodSet := make(map[string]bool)
				for _, method := range existing.Methods {
					methodSet[method] = true
				}
				for _, method := range cand.Methods {
					methodSet[method] = true
				}
				var allMethods []string
				for method := range methodSet {
					allMethods = append(allMethods, method)
				}
				cand.Methods = allMethods
			}
			uprnMap[cand.UPRN] = cand
		}
	}

	var deduped []Candidate
	for _, cand := range uprnMap {
		deduped = append(deduped, cand)
	}

	return deduped
}

// calculateDistance computes Euclidean distance between two points (simplified)
func calculateDistance(e1, n1, e2, n2 float64) float64 {
	de := e1 - e2
	dn := n1 - n2
	return (de*de + dn*dn) * 0.5 // Simplified distance calculation
}

// calculateSpatialBoost returns boost factor based on distance
func calculateSpatialBoost(distance float64) float64 {
	// Exponential decay: closer = higher boost
	// boost = e^(-distance/300) where 300m is the scale parameter
	if distance <= 0 {
		return 0.10 // Maximum boost for exact location
	}
	boost := 0.10 * (1.0 - distance/2000.0) // Linear decay over 2km, max 0.10
	if boost < 0 {
		boost = 0
	}
	return boost
}