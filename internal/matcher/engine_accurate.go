package matcher

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ehdc-llpg/internal/debug"
	"github.com/ehdc-llpg/internal/normalize"
)

// AccurateEngine prioritizes maximum matching accuracy over speed
type AccurateEngine struct {
	db       *sql.DB
	embedder Embedder
	vectorDB VectorDB
}

// ParsedAddress represents components extracted from an address
type ParsedAddress struct {
	Original      string
	HouseNumber   string
	HouseName     string
	FlatNumber    string
	StreetName    string
	Locality      string
	Town          string
	County        string
	Postcode      string
	BusinessName  string
	LandReference string // "Land at", "Rear of", etc.
	Components    map[string]string
}

// AccurateCandidate extends matching with detailed component analysis
type AccurateCandidate struct {
	MatchCandidate
	ComponentMatch ComponentMatchScore
	FinalScore     float64
	MatchReason    string
}

// ComponentMatchScore tracks matching at component level
type ComponentMatchScore struct {
	HouseNumberScore   float64
	HouseNameScore     float64
	StreetNameScore    float64
	LocalityScore      float64
	TownScore          float64
	PostcodeScore      float64
	OverallScore       float64
	ComponentsMatched  int
	TotalComponents    int
}

// NewAccurateEngine creates an accuracy-focused matching engine
func NewAccurateEngine(db *sql.DB, embedder Embedder, vectorDB VectorDB) *AccurateEngine {
	return &AccurateEngine{
		db:       db,
		embedder: embedder,
		vectorDB: vectorDB,
	}
}

// ProcessDocument performs accuracy-optimized address matching
func (e *AccurateEngine) ProcessDocument(localDebug bool, input MatchInput) (*MatchResult, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	
	debug.DebugOutput(localDebug, "Processing document %d (ACCURATE): %s", input.DocumentID, input.RawAddress)
	
	// Parse input address into components
	inputParsed := e.parseAddressEnhanced(localDebug, input.RawAddress)
	debug.DebugOutput(localDebug, "Parsed input: house='%s', street='%s', locality='%s', town='%s', postcode='%s'",
		inputParsed.HouseNumber, inputParsed.StreetName, inputParsed.Locality, inputParsed.Town, inputParsed.Postcode)
	
	// Strategy 1: Exact UPRN matching (if available)
	if input.RawUPRN != nil && *input.RawUPRN != "" {
		candidates := e.exactUPRNMatch(localDebug, *input.RawUPRN)
		if len(candidates) > 0 {
			return e.createResult(input.DocumentID, candidates, "auto_accept", "auto", time.Since(startTime)), nil
		}
	}
	
	// Strategy 2: Component-based matching with multiple passes
	allCandidates := []AccurateCandidate{}
	
	// Pass 1: Try postcode + house number (very reliable)
	if inputParsed.Postcode != "" && inputParsed.HouseNumber != "" {
		candidates := e.postcodeHouseNumberMatch(localDebug, inputParsed)
		allCandidates = append(allCandidates, candidates...)
		debug.DebugOutput(localDebug, "Postcode+HouseNumber match found %d candidates", len(candidates))
	}
	
	// Pass 2: Try street name + house number + locality
	if inputParsed.StreetName != "" {
		candidates := e.streetLocalityMatch(localDebug, inputParsed)
		allCandidates = append(allCandidates, candidates...)
		debug.DebugOutput(localDebug, "Street+Locality match found %d candidates", len(candidates))
	}
	
	// Pass 3: Try fuzzy matching with component weighting
	candidates := e.componentFuzzyMatch(localDebug, inputParsed, input.AddressCanonical)
	allCandidates = append(allCandidates, candidates...)
	debug.DebugOutput(localDebug, "Component fuzzy match found %d candidates", len(candidates))
	
	// Pass 4: Try business/landmark name matching
	if inputParsed.BusinessName != "" || inputParsed.HouseName != "" {
		candidates := e.landmarkMatch(localDebug, inputParsed)
		allCandidates = append(allCandidates, candidates...)
		debug.DebugOutput(localDebug, "Landmark match found %d candidates", len(candidates))
	}
	
	// Pass 5: Handle special cases (Land at, Rear of, etc.)
	if inputParsed.LandReference != "" {
		candidates := e.landReferenceMatch(localDebug, inputParsed)
		allCandidates = append(allCandidates, candidates...)
		debug.DebugOutput(localDebug, "Land reference match found %d candidates", len(candidates))
	}
	
	// Deduplicate and rank candidates
	finalCandidates := e.deduplicateAndRankAccurate(allCandidates)
	
	// Make intelligent decision
	decision, matchStatus := e.makeAccurateDecision(localDebug, finalCandidates)
	
	// Convert to standard format
	var standardCandidates []MatchCandidate
	for _, ac := range finalCandidates {
		standardCandidates = append(standardCandidates, ac.MatchCandidate)
	}
	
	return e.createResult(input.DocumentID, standardCandidates, decision, matchStatus, time.Since(startTime)), nil
}

// Enhanced address parsing using multiple strategies
func (e *AccurateEngine) parseAddressEnhanced(localDebug bool, address string) ParsedAddress {
	parsed := ParsedAddress{
		Original:   address,
		Components: make(map[string]string),
	}
	
	// Normalize and extract postcode first
	canonical, postcode, tokens := normalize.CanonicalAddressDebug(localDebug, address)
	parsed.Postcode = postcode
	
	// Extract house numbers
	houseNumbers := normalize.ExtractHouseNumbers(canonical)
	if len(houseNumbers) > 0 {
		parsed.HouseNumber = houseNumbers[0]
	}
	
	// Extract localities and towns
	localities := normalize.ExtractLocalityTokens(canonical)
	if len(localities) > 0 {
		parsed.Locality = localities[0]
		if len(localities) > 1 {
			parsed.Town = localities[1]
		}
	}
	
	// Extract street tokens
	streetTokens := normalize.TokenizeStreet(canonical)
	if len(streetTokens) > 0 {
		parsed.StreetName = strings.Join(streetTokens, " ")
	}
	
	// Check for business names (common patterns)
	upperAddr := strings.ToUpper(address)
	businessPatterns := []string{"HOTEL", "INN", "ARMS", "SURGERY", "SCHOOL", "CHURCH", "HALL", "FARM", "COTTAGE", "HOUSE"}
	for _, pattern := range businessPatterns {
		if strings.Contains(upperAddr, pattern) {
			// Extract the business name
			for _, token := range tokens {
				if strings.Contains(token, pattern) {
					parsed.BusinessName = token
					break
				}
			}
		}
	}
	
	// Check for land references
	landPatterns := []string{"LAND AT", "LAND ADJACENT", "REAR OF", "PLOT", "SITE AT"}
	for _, pattern := range landPatterns {
		if strings.Contains(upperAddr, pattern) {
			parsed.LandReference = pattern
			break
		}
	}
	
	// Extract flat/apartment numbers
	if strings.Contains(upperAddr, "FLAT") || strings.Contains(upperAddr, "APARTMENT") {
		// Simple extraction - could be enhanced
		for i, token := range tokens {
			if token == "FLAT" || token == "APARTMENT" {
				if i+1 < len(tokens) {
					parsed.FlatNumber = tokens[i+1]
				}
				break
			}
		}
	}
	
	debug.DebugOutput(localDebug, "Enhanced parse: house='%s', flat='%s', street='%s', business='%s', land='%s'",
		parsed.HouseNumber, parsed.FlatNumber, parsed.StreetName, parsed.BusinessName, parsed.LandReference)
	
	return parsed
}

// Postcode + House Number matching (very accurate)
func (e *AccurateEngine) postcodeHouseNumberMatch(localDebug bool, parsed ParsedAddress) []AccurateCandidate {
	if parsed.Postcode == "" || parsed.HouseNumber == "" {
		return []AccurateCandidate{}
	}
	
	query := `
		SELECT DISTINCT
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE 
			REPLACE(a.full_address, ' ', '') LIKE '%' || $1 || '%'
			AND a.full_address ~ '\y' || $2 || '\y'
		LIMIT 20
	`
	
	rows, err := e.db.Query(query, parsed.Postcode, parsed.HouseNumber)
	if err != nil {
		debug.DebugOutput(localDebug, "Postcode+HouseNumber query error: %v", err)
		return []AccurateCandidate{}
	}
	defer rows.Close()
	
	var candidates []AccurateCandidate
	for rows.Next() {
		var ac AccurateCandidate
		err := rows.Scan(
			&ac.MatchCandidate.AddressID, &ac.MatchCandidate.LocationID, &ac.MatchCandidate.UPRN,
			&ac.MatchCandidate.FullAddress, &ac.MatchCandidate.AddressCanonical,
			&ac.MatchCandidate.Easting, &ac.MatchCandidate.Northing,
		)
		if err != nil {
			continue
		}
		
		// High score for postcode + house number match
		ac.MatchCandidate.Score = 0.95
		ac.MatchCandidate.MethodCode = "postcode_house"
		ac.MatchCandidate.MethodID = 8
		ac.MatchReason = "Postcode + House Number match"
		ac.FinalScore = 0.95
		
		candidates = append(candidates, ac)
	}
	
	return candidates
}

// Street + Locality matching
func (e *AccurateEngine) streetLocalityMatch(localDebug bool, parsed ParsedAddress) []AccurateCandidate {
	if parsed.StreetName == "" {
		return []AccurateCandidate{}
	}
	
	query := `
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing,
			similarity($1, a.address_canonical) as sim_score
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE 
			a.address_canonical % $1
			AND similarity($1, a.address_canonical) >= 0.6
	`
	
	// Build search string
	searchStr := parsed.StreetName
	if parsed.HouseNumber != "" {
		searchStr = parsed.HouseNumber + " " + searchStr
	}
	if parsed.Locality != "" {
		searchStr = searchStr + " " + parsed.Locality
	}
	
	query += " ORDER BY sim_score DESC LIMIT 50"
	
	rows, err := e.db.Query(query, searchStr)
	if err != nil {
		debug.DebugOutput(localDebug, "Street+Locality query error: %v", err)
		return []AccurateCandidate{}
	}
	defer rows.Close()
	
	var candidates []AccurateCandidate
	for rows.Next() {
		var ac AccurateCandidate
		var simScore float32
		err := rows.Scan(
			&ac.MatchCandidate.AddressID, &ac.MatchCandidate.LocationID, &ac.MatchCandidate.UPRN,
			&ac.MatchCandidate.FullAddress, &ac.MatchCandidate.AddressCanonical,
			&ac.MatchCandidate.Easting, &ac.MatchCandidate.Northing, &simScore,
		)
		if err != nil {
			continue
		}
		
		// Calculate component match score
		ac.ComponentMatch = e.calculateComponentScore(parsed, ac.MatchCandidate.FullAddress)
		
		// Combine similarity and component scores
		ac.FinalScore = (float64(simScore) * 0.6) + (ac.ComponentMatch.OverallScore * 0.4)
		ac.MatchCandidate.Score = ac.FinalScore
		ac.MatchCandidate.MethodCode = "street_locality"
		ac.MatchCandidate.MethodID = 9
		ac.MatchReason = fmt.Sprintf("Street match (%.0f%% components)", ac.ComponentMatch.OverallScore*100)
		
		candidates = append(candidates, ac)
	}
	
	return candidates
}

// Component-based fuzzy matching with intelligent weighting
func (e *AccurateEngine) componentFuzzyMatch(localDebug bool, parsed ParsedAddress, canonical string) []AccurateCandidate {
	// Use the optimized function but with more candidates
	rows, err := e.db.Query(`
		SELECT 
			address_id, location_id, uprn, full_address, address_canonical,
			easting, northing, match_score, match_method
		FROM fast_address_match($1, NULL, 200)
		WHERE match_score >= 0.5
		ORDER BY match_score DESC
	`, canonical)
	
	if err != nil {
		debug.DebugOutput(localDebug, "Component fuzzy query error: %v", err)
		return []AccurateCandidate{}
	}
	defer rows.Close()
	
	var candidates []AccurateCandidate
	for rows.Next() {
		var ac AccurateCandidate
		var methodCode string
		var score32 float32
		
		err := rows.Scan(
			&ac.MatchCandidate.AddressID, &ac.MatchCandidate.LocationID, &ac.MatchCandidate.UPRN,
			&ac.MatchCandidate.FullAddress, &ac.MatchCandidate.AddressCanonical,
			&ac.MatchCandidate.Easting, &ac.MatchCandidate.Northing, &score32, &methodCode,
		)
		if err != nil {
			continue
		}
		
		// Calculate detailed component matching
		ac.ComponentMatch = e.calculateComponentScore(parsed, ac.MatchCandidate.FullAddress)
		
		// Weight the scores based on component matches
		baseScore := float64(score32)
		componentBoost := 0.0
		
		if ac.ComponentMatch.HouseNumberScore > 0.8 {
			componentBoost += 0.15
		}
		if ac.ComponentMatch.StreetNameScore > 0.8 {
			componentBoost += 0.10
		}
		if ac.ComponentMatch.LocalityScore > 0.8 {
			componentBoost += 0.10
		}
		if ac.ComponentMatch.PostcodeScore > 0.9 {
			componentBoost += 0.20
		}
		
		ac.FinalScore = math.Min(baseScore+componentBoost, 1.0)
		ac.MatchCandidate.Score = ac.FinalScore
		ac.MatchCandidate.MethodCode = "component_fuzzy"
		ac.MatchCandidate.MethodID = 10
		ac.MatchReason = fmt.Sprintf("Fuzzy match with %d/%d components", 
			ac.ComponentMatch.ComponentsMatched, ac.ComponentMatch.TotalComponents)
		
		candidates = append(candidates, ac)
	}
	
	return candidates
}

// Landmark/Business name matching
func (e *AccurateEngine) landmarkMatch(localDebug bool, parsed ParsedAddress) []AccurateCandidate {
	searchTerm := parsed.BusinessName
	if searchTerm == "" {
		searchTerm = parsed.HouseName
	}
	if searchTerm == "" {
		return []AccurateCandidate{}
	}
	
	// Add locality if available
	if parsed.Locality != "" {
		searchTerm = searchTerm + " " + parsed.Locality
	}
	
	query := `
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE 
			a.address_canonical LIKE '%' || $1 || '%'
		LIMIT 20
	`
	
	rows, err := e.db.Query(query, strings.ToUpper(searchTerm))
	if err != nil {
		return []AccurateCandidate{}
	}
	defer rows.Close()
	
	var candidates []AccurateCandidate
	for rows.Next() {
		var ac AccurateCandidate
		err := rows.Scan(
			&ac.MatchCandidate.AddressID, &ac.MatchCandidate.LocationID, &ac.MatchCandidate.UPRN,
			&ac.MatchCandidate.FullAddress, &ac.MatchCandidate.AddressCanonical,
			&ac.MatchCandidate.Easting, &ac.MatchCandidate.Northing,
		)
		if err != nil {
			continue
		}
		
		ac.FinalScore = 0.85
		ac.MatchCandidate.Score = 0.85
		ac.MatchCandidate.MethodCode = "landmark"
		ac.MatchCandidate.MethodID = 11
		ac.MatchReason = "Landmark/Business name match"
		
		candidates = append(candidates, ac)
	}
	
	return candidates
}

// Land reference matching (Land at, Rear of, etc.)
func (e *AccurateEngine) landReferenceMatch(localDebug bool, parsed ParsedAddress) []AccurateCandidate {
	// Extract the location after the land reference
	searchStr := strings.ToUpper(parsed.Original)
	searchStr = strings.Replace(searchStr, parsed.LandReference, "", 1)
	searchStr = strings.TrimSpace(searchStr)
	
	if searchStr == "" {
		return []AccurateCandidate{}
	}
	
	// Try to find nearby addresses
	query := `
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing,
			similarity($1, a.address_canonical) as sim_score
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE 
			similarity($1, a.address_canonical) >= 0.5
		ORDER BY sim_score DESC
		LIMIT 30
	`
	
	rows, err := e.db.Query(query, searchStr)
	if err != nil {
		return []AccurateCandidate{}
	}
	defer rows.Close()
	
	var candidates []AccurateCandidate
	for rows.Next() {
		var ac AccurateCandidate
		var simScore float32
		err := rows.Scan(
			&ac.MatchCandidate.AddressID, &ac.MatchCandidate.LocationID, &ac.MatchCandidate.UPRN,
			&ac.MatchCandidate.FullAddress, &ac.MatchCandidate.AddressCanonical,
			&ac.MatchCandidate.Easting, &ac.MatchCandidate.Northing, &simScore,
		)
		if err != nil {
			continue
		}
		
		ac.FinalScore = float64(simScore) * 0.8 // Reduce confidence for land references
		ac.MatchCandidate.Score = ac.FinalScore
		ac.MatchCandidate.MethodCode = "land_reference"
		ac.MatchCandidate.MethodID = 12
		ac.MatchReason = fmt.Sprintf("%s location match", parsed.LandReference)
		
		candidates = append(candidates, ac)
	}
	
	return candidates
}

// Calculate component-level matching scores
func (e *AccurateEngine) calculateComponentScore(parsed ParsedAddress, candidateAddress string) ComponentMatchScore {
	score := ComponentMatchScore{}
	
	// Parse candidate address
	candCanonical, candPostcode, _ := normalize.CanonicalAddress(candidateAddress)
	candHouseNumbers := normalize.ExtractHouseNumbers(candCanonical)
	candLocalities := normalize.ExtractLocalityTokens(candCanonical)
	candStreetTokens := normalize.TokenizeStreet(candCanonical)
	
	// House number matching
	if parsed.HouseNumber != "" {
		score.TotalComponents++
		for _, candHN := range candHouseNumbers {
			if parsed.HouseNumber == candHN {
				score.HouseNumberScore = 1.0
				score.ComponentsMatched++
				break
			}
		}
	}
	
	// Street name matching
	if parsed.StreetName != "" {
		score.TotalComponents++
		candStreet := strings.Join(candStreetTokens, " ")
		if candStreet != "" {
			score.StreetNameScore = normalize.TokenOverlap(
				strings.Fields(parsed.StreetName),
				candStreetTokens,
			)
			if score.StreetNameScore > 0.5 {
				score.ComponentsMatched++
			}
		}
	}
	
	// Locality matching
	if parsed.Locality != "" {
		score.TotalComponents++
		for _, candLoc := range candLocalities {
			if parsed.Locality == candLoc {
				score.LocalityScore = 1.0
				score.ComponentsMatched++
				break
			}
		}
	}
	
	// Postcode matching
	if parsed.Postcode != "" {
		score.TotalComponents++
		if parsed.Postcode == candPostcode {
			score.PostcodeScore = 1.0
			score.ComponentsMatched++
		}
	}
	
	// Calculate overall score
	if score.TotalComponents > 0 {
		score.OverallScore = float64(score.ComponentsMatched) / float64(score.TotalComponents)
	}
	
	return score
}

// Exact UPRN matching
func (e *AccurateEngine) exactUPRNMatch(localDebug bool, uprn string) []MatchCandidate {
	rows, err := e.db.Query(`
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.uprn = $1
	`, strings.TrimSpace(uprn))
	
	if err != nil {
		return []MatchCandidate{}
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
		
		candidate.Score = 1.0
		candidate.MethodCode = "exact_uprn"
		candidate.MethodID = 1
		
		candidates = append(candidates, candidate)
	}
	
	return candidates
}

// Deduplicate and rank candidates for accuracy
func (e *AccurateEngine) deduplicateAndRankAccurate(candidates []AccurateCandidate) []AccurateCandidate {
	// Deduplicate by address_id, keeping highest score
	addressMap := make(map[int]AccurateCandidate)
	for _, cand := range candidates {
		existing, exists := addressMap[cand.MatchCandidate.AddressID]
		if !exists || cand.FinalScore > existing.FinalScore {
			addressMap[cand.MatchCandidate.AddressID] = cand
		}
	}
	
	// Convert to slice
	var deduped []AccurateCandidate
	for _, cand := range addressMap {
		deduped = append(deduped, cand)
	}
	
	// Sort by final score
	for i := 0; i < len(deduped)-1; i++ {
		for j := i + 1; j < len(deduped); j++ {
			if deduped[j].FinalScore > deduped[i].FinalScore {
				deduped[i], deduped[j] = deduped[j], deduped[i]
			}
		}
	}
	
	return deduped
}

// Make decision based on accuracy criteria
func (e *AccurateEngine) makeAccurateDecision(localDebug bool, candidates []AccurateCandidate) (string, string) {
	if len(candidates) == 0 {
		return "no_match", "auto"
	}
	
	best := candidates[0]
	
	// Very high confidence criteria
	if best.FinalScore >= 0.95 {
		return "auto_accept", "auto"
	}
	
	// High confidence with component validation
	if best.FinalScore >= 0.85 && best.ComponentMatch.ComponentsMatched >= 2 {
		return "auto_accept", "auto"
	}
	
	// Medium confidence
	if best.FinalScore >= 0.75 {
		return "needs_review", "manual"
	}
	
	// Low confidence but has some components
	if best.FinalScore >= 0.60 && best.ComponentMatch.ComponentsMatched >= 1 {
		return "needs_review", "manual"
	}
	
	return "low_confidence", "manual"
}

// Create result from candidates
func (e *AccurateEngine) createResult(documentID int64, candidates []MatchCandidate, decision, matchStatus string, processingTime time.Duration) *MatchResult {
	var bestCandidate *MatchCandidate
	if len(candidates) > 0 {
		bestCandidate = &candidates[0]
	}
	
	return &MatchResult{
		DocumentID:     documentID,
		BestCandidate:  bestCandidate,
		AllCandidates:  candidates,
		Decision:       decision,
		MatchStatus:    matchStatus,
		ProcessingTime: processingTime,
	}
}

// SaveMatchResult saves the matching result to the database
func (e *AccurateEngine) SaveMatchResult(localDebug bool, result *MatchResult) error {
	if result.BestCandidate == nil {
		debug.DebugOutput(localDebug, "No match to save for document %d", result.DocumentID)
		return nil
	}
	
	// Default method ID if not set
	methodID := result.BestCandidate.MethodID
	if methodID == 0 {
		methodID = 10 // component_fuzzy default
	}
	
	_, err := e.db.Exec(`
		INSERT INTO address_match (
			document_id, address_id, location_id, match_method_id,
			confidence_score, match_status, matched_by, matched_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (document_id) DO UPDATE SET
			address_id = EXCLUDED.address_id,
			location_id = EXCLUDED.location_id,
			match_method_id = EXCLUDED.match_method_id,
			confidence_score = EXCLUDED.confidence_score,
			match_status = EXCLUDED.match_status,
			matched_by = EXCLUDED.matched_by,
			matched_at = now()
	`,
		result.DocumentID,
		result.BestCandidate.AddressID,
		result.BestCandidate.LocationID,
		methodID,
		result.BestCandidate.Score,
		result.MatchStatus,
		"system_accurate",
	)
	
	if err != nil {
		return fmt.Errorf("failed to save accurate match result: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Saved accurate match result for document %d -> address %d (%.4f)", 
		result.DocumentID, result.BestCandidate.AddressID, result.BestCandidate.Score)
	
	return nil
}