package matcher

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ehdc-llpg/internal/debug"
	"github.com/ehdc-llpg/internal/normalize"
)

// FixedComponentEngine performs component-based address matching with proper validation
// This engine fixes the critical house number validation issues
type FixedComponentEngine struct {
	db *sql.DB
}

// NewFixedComponentEngine creates a fixed component-based matching engine
func NewFixedComponentEngine(db *sql.DB) *FixedComponentEngine {
	return &FixedComponentEngine{db: db}
}

// ValidationResult tracks validation failures for debugging
type ValidationResult struct {
	IsValid           bool
	FailureReasons    []string
	HouseNumberMatch  bool
	PostcodeMatch     bool
	BusinessNameMatch bool
	GeographicCheck   bool
}

// ProcessDocument performs component-based address matching with proper validation
func (e *FixedComponentEngine) ProcessDocument(localDebug bool, input MatchInput) (*MatchResult, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	
	debug.DebugOutput(localDebug, "Processing document %d (FIXED COMPONENT): %s", input.DocumentID, input.RawAddress)
	
	// PRIORITY 1: If source document has UPRN, use it directly
	if input.RawUPRN != nil && *input.RawUPRN != "" {
		debug.DebugOutput(localDebug, "Source has UPRN: %s - attempting exact UPRN match", *input.RawUPRN)
		
		candidates, err := e.exactUPRNDirectMatch(localDebug, *input.RawUPRN)
		if err == nil && len(candidates) > 0 {
			debug.DebugOutput(localDebug, "Found %d exact UPRN matches", len(candidates))
			
			// Convert to standard candidates
			var standardCandidates []MatchCandidate
			for _, cm := range candidates {
				standardCandidates = append(standardCandidates, cm.MatchCandidate)
			}
			
			// UPRN match is always high confidence
			result := &MatchResult{
				DocumentID:     input.DocumentID,
				BestCandidate:  &standardCandidates[0],
				AllCandidates:  standardCandidates,
				Decision:       "auto_accept",
				MatchStatus:    "auto",
				ProcessingTime: time.Since(startTime),
			}
			
			debug.DebugOutput(localDebug, "UPRN match successful: address_id=%d, score=%.4f", 
				result.BestCandidate.AddressID, result.BestCandidate.Score)
			
			return result, nil
		}
		
		debug.DebugOutput(localDebug, "UPRN %s not found in LLPG, creating historic record", *input.RawUPRN)
		
		// Create historic UPRN record
		historicCandidate, err := e.createHistoricUPRNRecord(localDebug, *input.RawUPRN, input.RawAddress, input.DocumentID)
		if err == nil && historicCandidate != nil {
			debug.DebugOutput(localDebug, "Created historic UPRN record: address_id=%d", historicCandidate.AddressID)
			
			result := &MatchResult{
				DocumentID:     input.DocumentID,
				BestCandidate:  historicCandidate,
				AllCandidates:  []MatchCandidate{*historicCandidate},
				Decision:       "auto_accept",
				MatchStatus:    "auto",
				ProcessingTime: time.Since(startTime),
			}
			
			return result, nil
		}
		
		debug.DebugOutput(localDebug, "Failed to create historic record for UPRN %s: %v, falling back to component matching", *input.RawUPRN, err)
	}
	
	// Get input components for fallback matching
	inputComponents, err := e.getInputComponents(localDebug, input)
	if err != nil {
		return nil, fmt.Errorf("failed to extract input components: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Input components: house_number='%s', road='%s', city='%s', postcode='%s'",
		inputComponents["house_number"], inputComponents["road"], inputComponents["city"], inputComponents["postcode"])
	
	// Perform component-based matching with validation
	candidates, err := e.performValidatedMatching(localDebug, inputComponents)
	if err != nil {
		return nil, fmt.Errorf("component matching failed: %w", err)
	}
	
	// Convert to standard candidates
	var standardCandidates []MatchCandidate
	for _, cm := range candidates {
		standardCandidates = append(standardCandidates, cm.MatchCandidate)
	}
	
	// Make decision with strict validation
	decision, matchStatus := e.makeValidatedDecision(localDebug, candidates, inputComponents)
	
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
	
	debug.DebugOutput(localDebug, "Fixed component decision: %s with %d candidates in %v", 
		decision, len(standardCandidates), result.ProcessingTime)
	
	return result, nil
}

// getInputComponents gets the components for the input address
func (e *FixedComponentEngine) getInputComponents(localDebug bool, input MatchInput) (map[string]string, error) {
	components := make(map[string]string)
	
	// Try to get from database first
	var houseNumber, road, city, postcode, unit sql.NullString
	err := e.db.QueryRow(`
		SELECT gopostal_house_number, gopostal_road, gopostal_city, gopostal_postcode, gopostal_unit
		FROM src_document 
		WHERE document_id = $1
	`, input.DocumentID).Scan(&houseNumber, &road, &city, &postcode, &unit)
	
	if err == nil {
		// Use database components
		if houseNumber.Valid {
			components["house_number"] = strings.TrimSpace(houseNumber.String)
		}
		if road.Valid {
			components["road"] = strings.TrimSpace(road.String)
		}
		if city.Valid {
			components["city"] = strings.TrimSpace(city.String)
		}
		if postcode.Valid {
			components["postcode"] = strings.TrimSpace(postcode.String)
		}
		if unit.Valid {
			components["unit"] = strings.TrimSpace(unit.String)
		}
	}
	
	// Extract business name if present
	components["business_name"] = e.extractBusinessName(input.RawAddress)
	
	return components, nil
}

// extractBusinessName extracts business/organization names from addresses
func (e *FixedComponentEngine) extractBusinessName(address string) string {
	address = strings.ToUpper(address)
	
	// Business indicators
	businessKeywords := []string{
		"LTD", "LIMITED", "CLUB", "CENTRE", "CENTER", "SCHOOL", "HOTEL", 
		"RESTAURANT", "CAFE", "SHOP", "STORE", "GARAGE", "SURGERY",
		"CHURCH", "HALL", "FARM", "WORKS", "FACTORY", "OFFICE",
	}
	
	for _, keyword := range businessKeywords {
		if strings.Contains(address, keyword) {
			// Extract the business name part (before the first comma typically)
			parts := strings.Split(address, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
	}
	
	return ""
}

// performValidatedMatching performs component-based matching with strict validation
func (e *FixedComponentEngine) performValidatedMatching(localDebug bool, inputComponents map[string]string) ([]ComponentMatch, error) {
	var allCandidates []ComponentMatch
	
	// Strategy 1: Exact UPRN/reference match (highest priority)
	if inputComponents["postcode"] != "" && inputComponents["house_number"] != "" {
		candidates := e.exactUPRNMatch(localDebug, inputComponents)
		allCandidates = append(allCandidates, candidates...)
	}
	
	// Strategy 2: Postcode + House Number match (high confidence)
	if len(allCandidates) < 5 && inputComponents["postcode"] != "" && inputComponents["house_number"] != "" {
		candidates := e.postcodeHouseValidatedMatch(localDebug, inputComponents)
		allCandidates = append(allCandidates, candidates...)
	}
	
	// Strategy 3: Business name match (for organizations)
	if len(allCandidates) < 10 && inputComponents["business_name"] != "" {
		candidates := e.businessNameMatch(localDebug, inputComponents)
		allCandidates = append(allCandidates, candidates...)
	}
	
	// Strategy 4: Road + City with STRICT house number validation
	if len(allCandidates) < 15 && inputComponents["road"] != "" && inputComponents["city"] != "" {
		candidates := e.roadCityValidatedMatch(localDebug, inputComponents)
		allCandidates = append(allCandidates, candidates...)
	}
	
	// Strategy 5: Fuzzy road ONLY if no house number conflict
	if len(allCandidates) < 20 && inputComponents["road"] != "" {
		candidates := e.fuzzyRoadValidatedMatch(localDebug, inputComponents)
		allCandidates = append(allCandidates, candidates...)
	}
	
	// Validate and filter all candidates
	validCandidates := e.validateCandidates(localDebug, inputComponents, allCandidates)
	
	debug.DebugOutput(localDebug, "Validated matching found %d candidates total, %d after validation", 
		len(allCandidates), len(validCandidates))
	
	return validCandidates, nil
}

// exactUPRNDirectMatch finds exact UPRN matches from source document UPRN
func (e *FixedComponentEngine) exactUPRNDirectMatch(localDebug bool, uprn string) ([]ComponentMatch, error) {
	var candidates []ComponentMatch
	
	// Clean the UPRN - remove decimal suffixes like .00
	cleanUPRN := strings.TrimSpace(uprn)
	if cleanUPRN == "" {
		return candidates, nil
	}
	
	// Remove .00 decimal suffix if present
	if strings.HasSuffix(cleanUPRN, ".00") {
		cleanUPRN = strings.TrimSuffix(cleanUPRN, ".00")
		debug.DebugOutput(localDebug, "Normalized UPRN from %s to %s", uprn, cleanUPRN)
	}
	
	// Query LLPG for exact UPRN match
	rows, err := e.db.Query(`
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing,
			a.gopostal_house_number, a.gopostal_road, a.gopostal_city, a.gopostal_postcode
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.uprn = $1
	`, cleanUPRN)
	
	if err != nil {
		debug.DebugOutput(localDebug, "UPRN query error: %v", err)
		return candidates, err
	}
	defer rows.Close()
	
	for rows.Next() {
		var cm ComponentMatch
		var dbHouseNum, dbRoad, dbCity, dbPostcode sql.NullString
		
		err := rows.Scan(
			&cm.MatchCandidate.AddressID, &cm.MatchCandidate.LocationID, &cm.MatchCandidate.UPRN,
			&cm.MatchCandidate.FullAddress, &cm.MatchCandidate.AddressCanonical,
			&cm.MatchCandidate.Easting, &cm.MatchCandidate.Northing,
			&dbHouseNum, &dbRoad, &dbCity, &dbPostcode,
		)
		if err != nil {
			continue
		}
		
		// UPRN match gets perfect score
		cm.MatchCandidate.Score = 1.0
		cm.MatchCandidate.MethodCode = "exact_uprn"
		cm.MatchCandidate.MethodID = 1 // exact_uprn method ID
		
		// Set component score for perfect match
		cm.ComponentScore = ComponentScore{
			OverallScore: 1.0,
			MatchedCount: 1,
			TotalCount:   1,
		}
		
		candidates = append(candidates, cm)
		
		debug.DebugOutput(localDebug, "UPRN %s matched to address_id=%d: %s", 
			cleanUPRN, cm.MatchCandidate.AddressID, cm.MatchCandidate.FullAddress)
	}
	
	return candidates, nil
}

// exactUPRNMatch finds exact UPRN matches (legacy - kept for compatibility)
func (e *FixedComponentEngine) exactUPRNMatch(localDebug bool, input map[string]string) []ComponentMatch {
	var candidates []ComponentMatch
	
	// This would need UPRN data in source - placeholder for now
	debug.DebugOutput(localDebug, "Exact UPRN match not implemented yet")
	
	return candidates
}

// postcodeHouseValidatedMatch matches on postcode + house number with validation
func (e *FixedComponentEngine) postcodeHouseValidatedMatch(localDebug bool, input map[string]string) []ComponentMatch {
	var candidates []ComponentMatch
	
	if input["postcode"] == "" || input["house_number"] == "" {
		return candidates
	}
	
	rows, err := e.db.Query(`
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing,
			a.gopostal_house_number, a.gopostal_road, a.gopostal_city, a.gopostal_postcode
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.gopostal_postcode = $1 
		  AND a.gopostal_house_number = $2
		ORDER BY a.address_id
		LIMIT 10
	`, input["postcode"], input["house_number"])
	
	if err != nil {
		debug.DebugOutput(localDebug, "Postcode+house query error: %v", err)
		return candidates
	}
	defer rows.Close()
	
	for rows.Next() {
		var cm ComponentMatch
		var dbHouseNum, dbRoad, dbCity, dbPostcode sql.NullString
		
		err := rows.Scan(
			&cm.MatchCandidate.AddressID, &cm.MatchCandidate.LocationID, &cm.MatchCandidate.UPRN,
			&cm.MatchCandidate.FullAddress, &cm.MatchCandidate.AddressCanonical,
			&cm.MatchCandidate.Easting, &cm.MatchCandidate.Northing,
			&dbHouseNum, &dbRoad, &dbCity, &dbPostcode,
		)
		if err != nil {
			continue
		}
		
		cm.ComponentScore = e.calculateValidatedScore(input, map[string]string{
			"house_number": dbHouseNum.String,
			"road":         dbRoad.String,
			"city":         dbCity.String,
			"postcode":     dbPostcode.String,
		})
		
		cm.MatchCandidate.Score = cm.ComponentScore.OverallScore
		cm.MatchCandidate.MethodCode = "postcode_house_validated"
		cm.MatchCandidate.MethodID = 20
		
		candidates = append(candidates, cm)
	}
	
	debug.DebugOutput(localDebug, "Validated postcode+house match found %d candidates", len(candidates))
	return candidates
}

// businessNameMatch matches on business/organization names
func (e *FixedComponentEngine) businessNameMatch(localDebug bool, input map[string]string) []ComponentMatch {
	var candidates []ComponentMatch
	
	if input["business_name"] == "" {
		return candidates
	}
	
	// Extract key words from business name for fuzzy matching
	businessWords := strings.Fields(input["business_name"])
	if len(businessWords) < 2 {
		return candidates
	}
	
	// Build query for business name similarity
	query := `
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing,
			a.gopostal_house_number, a.gopostal_road, a.gopostal_city, a.gopostal_postcode,
			similarity(a.full_address, $1) as name_sim
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.full_address % $1 
		  AND similarity(a.full_address, $1) >= 0.8
		ORDER BY name_sim DESC
		LIMIT 5
	`
	
	rows, err := e.db.Query(query, input["business_name"])
	if err != nil {
		debug.DebugOutput(localDebug, "Business name query error: %v", err)
		return candidates
	}
	defer rows.Close()
	
	for rows.Next() {
		var cm ComponentMatch
		var dbHouseNum, dbRoad, dbCity, dbPostcode sql.NullString
		var nameSim float64
		
		err := rows.Scan(
			&cm.MatchCandidate.AddressID, &cm.MatchCandidate.LocationID, &cm.MatchCandidate.UPRN,
			&cm.MatchCandidate.FullAddress, &cm.MatchCandidate.AddressCanonical,
			&cm.MatchCandidate.Easting, &cm.MatchCandidate.Northing,
			&dbHouseNum, &dbRoad, &dbCity, &dbPostcode, &nameSim,
		)
		if err != nil {
			continue
		}
		
		cm.ComponentScore = e.calculateValidatedScore(input, map[string]string{
			"house_number": dbHouseNum.String,
			"road":         dbRoad.String,
			"city":         dbCity.String,
			"postcode":     dbPostcode.String,
		})
		
		// Boost for business name match
		cm.ComponentScore.OverallScore = nameSim
		cm.MatchCandidate.Score = cm.ComponentScore.OverallScore
		cm.MatchCandidate.MethodCode = "business_name_match"
		cm.MatchCandidate.MethodID = 21
		
		candidates = append(candidates, cm)
	}
	
	debug.DebugOutput(localDebug, "Business name match found %d candidates", len(candidates))
	return candidates
}

// roadCityValidatedMatch matches on road + city with STRICT house number validation
func (e *FixedComponentEngine) roadCityValidatedMatch(localDebug bool, input map[string]string) []ComponentMatch {
	var candidates []ComponentMatch
	
	if input["road"] == "" || input["city"] == "" {
		return candidates
	}
	
	// CRITICAL FIX: If input has house number, candidate MUST have matching house number
	var houseNumberClause string
	var args []interface{}
	
	if input["house_number"] != "" {
		houseNumberClause = "AND a.gopostal_house_number = $3"
		args = []interface{}{input["road"], input["city"], input["house_number"]}
	} else {
		args = []interface{}{input["road"], input["city"]}
	}
	
	query := fmt.Sprintf(`
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing,
			a.gopostal_house_number, a.gopostal_road, a.gopostal_city, a.gopostal_postcode,
			similarity(a.gopostal_road, $1) as road_sim
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.gopostal_road = $1 
		  AND a.gopostal_city = $2
		  %s
		ORDER BY road_sim DESC
		LIMIT 10
	`, houseNumberClause)
	
	rows, err := e.db.Query(query, args...)
	if err != nil {
		debug.DebugOutput(localDebug, "Validated road+city query error: %v", err)
		return candidates
	}
	defer rows.Close()
	
	for rows.Next() {
		var cm ComponentMatch
		var dbHouseNum, dbRoad, dbCity, dbPostcode sql.NullString
		var roadSim float64
		
		err := rows.Scan(
			&cm.MatchCandidate.AddressID, &cm.MatchCandidate.LocationID, &cm.MatchCandidate.UPRN,
			&cm.MatchCandidate.FullAddress, &cm.MatchCandidate.AddressCanonical,
			&cm.MatchCandidate.Easting, &cm.MatchCandidate.Northing,
			&dbHouseNum, &dbRoad, &dbCity, &dbPostcode, &roadSim,
		)
		if err != nil {
			continue
		}
		
		cm.ComponentScore = e.calculateValidatedScore(input, map[string]string{
			"house_number": dbHouseNum.String,
			"road":         dbRoad.String,
			"city":         dbCity.String,
			"postcode":     dbPostcode.String,
		})
		
		cm.MatchCandidate.Score = cm.ComponentScore.OverallScore
		cm.MatchCandidate.MethodCode = "road_city_validated"
		cm.MatchCandidate.MethodID = 22
		
		candidates = append(candidates, cm)
	}
	
	debug.DebugOutput(localDebug, "Validated road+city match found %d candidates", len(candidates))
	return candidates
}

// fuzzyRoadValidatedMatch performs fuzzy road matching with house number validation
func (e *FixedComponentEngine) fuzzyRoadValidatedMatch(localDebug bool, input map[string]string) []ComponentMatch {
	var candidates []ComponentMatch
	
	if input["road"] == "" {
		return candidates
	}
	
	// CRITICAL FIX: If input has house number, candidate MUST have matching house number
	var houseNumberClause string
	var args []interface{}
	
	if input["house_number"] != "" {
		houseNumberClause = "AND a.gopostal_house_number = $2"
		args = []interface{}{input["road"], input["house_number"]}
	} else {
		args = []interface{}{input["road"]}
	}
	
	query := fmt.Sprintf(`
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing,
			a.gopostal_house_number, a.gopostal_road, a.gopostal_city, a.gopostal_postcode,
			similarity(a.gopostal_road, $1) as road_sim
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.gopostal_road %% $1 
		  AND similarity(a.gopostal_road, $1) >= 0.8
		  %s
		ORDER BY road_sim DESC
		LIMIT 20
	`, houseNumberClause)
	
	rows, err := e.db.Query(query, args...)
	if err != nil {
		debug.DebugOutput(localDebug, "Validated fuzzy road query error: %v", err)
		return candidates
	}
	defer rows.Close()
	
	for rows.Next() {
		var cm ComponentMatch
		var dbHouseNum, dbRoad, dbCity, dbPostcode sql.NullString
		var roadSim float64
		
		err := rows.Scan(
			&cm.MatchCandidate.AddressID, &cm.MatchCandidate.LocationID, &cm.MatchCandidate.UPRN,
			&cm.MatchCandidate.FullAddress, &cm.MatchCandidate.AddressCanonical,
			&cm.MatchCandidate.Easting, &cm.MatchCandidate.Northing,
			&dbHouseNum, &dbRoad, &dbCity, &dbPostcode, &roadSim,
		)
		if err != nil {
			continue
		}
		
		cm.ComponentScore = e.calculateValidatedScore(input, map[string]string{
			"house_number": dbHouseNum.String,
			"road":         dbRoad.String,
			"city":         dbCity.String,
			"postcode":     dbPostcode.String,
		})
		
		// Penalize for fuzzy match
		cm.ComponentScore.OverallScore *= roadSim * 0.9
		cm.MatchCandidate.Score = cm.ComponentScore.OverallScore
		cm.MatchCandidate.MethodCode = "fuzzy_road_validated"
		cm.MatchCandidate.MethodID = 23
		
		candidates = append(candidates, cm)
	}
	
	debug.DebugOutput(localDebug, "Validated fuzzy road match found %d candidates", len(candidates))
	return candidates
}

// calculateValidatedScore calculates component scores with PROPER penalty for mismatches
func (e *FixedComponentEngine) calculateValidatedScore(input, candidate map[string]string) ComponentScore {
	score := ComponentScore{}
	
	// House number matching - CRITICAL validation
	if input["house_number"] != "" {
		score.TotalCount++
		if candidate["house_number"] == input["house_number"] {
			score.HouseNumberMatch = true
			score.HouseNumberScore = 1.0
			score.MatchedCount++
		} else if candidate["house_number"] != "" {
			// CRITICAL FIX: Major penalty for house number mismatch
			score.HouseNumberMatch = false
			score.HouseNumberScore = 0.0
			// Don't increment MatchedCount for mismatch
		}
	}
	
	// Road matching
	if input["road"] != "" {
		score.TotalCount++
		if candidate["road"] == input["road"] {
			score.RoadMatch = true
			score.RoadScore = 1.0
			score.MatchedCount++
		} else if candidate["road"] != "" {
			// Calculate similarity
			score.RoadScore = normalize.TokenOverlap(
				strings.Fields(input["road"]),
				strings.Fields(candidate["road"]),
			)
			if score.RoadScore >= 0.8 { // Stricter threshold
				score.RoadMatch = true
				score.MatchedCount++
			}
		}
	}
	
	// City matching
	if input["city"] != "" {
		score.TotalCount++
		if candidate["city"] == input["city"] {
			score.CityMatch = true
			score.CityScore = 1.0
			score.MatchedCount++
		}
	}
	
	// Postcode matching
	if input["postcode"] != "" {
		score.TotalCount++
		if candidate["postcode"] == input["postcode"] {
			score.PostcodeMatch = true
			score.PostcodeScore = 1.0
			score.MatchedCount++
		}
	}
	
	// Calculate overall score with PROPER penalties
	if score.TotalCount > 0 {
		baseScore := float64(score.MatchedCount) / float64(score.TotalCount)
		
		// CRITICAL FIX: Apply severe penalties for mismatches
		if input["house_number"] != "" && candidate["house_number"] != "" && !score.HouseNumberMatch {
			// House number mismatch = major penalty
			baseScore *= 0.1 // 90% penalty
		}
		
		score.OverallScore = baseScore
		
		// Boost for perfect matches only
		if score.PostcodeMatch && score.HouseNumberMatch {
			score.OverallScore = 1.0 // Perfect match
		} else if score.PostcodeMatch && input["house_number"] == "" {
			score.OverallScore += 0.1 // Boost for postcode when no house number
		}
		
		// Cap at 1.0
		if score.OverallScore > 1.0 {
			score.OverallScore = 1.0
		}
	}
	
	return score
}

// validateCandidates applies additional validation rules
func (e *FixedComponentEngine) validateCandidates(localDebug bool, input map[string]string, candidates []ComponentMatch) []ComponentMatch {
	var validCandidates []ComponentMatch
	
	for _, candidate := range candidates {
		validation := e.validateCandidate(localDebug, input, candidate)
		
		if validation.IsValid {
			validCandidates = append(validCandidates, candidate)
		} else {
			debug.DebugOutput(localDebug, "Candidate %d rejected: %v", 
				candidate.MatchCandidate.AddressID, validation.FailureReasons)
		}
	}
	
	return validCandidates
}

// validateCandidate performs comprehensive validation of a match candidate
func (e *FixedComponentEngine) validateCandidate(localDebug bool, input map[string]string, candidate ComponentMatch) ValidationResult {
	validation := ValidationResult{IsValid: true}
	
	// Rule 1: House number must match if both present
	if input["house_number"] != "" && candidate.ComponentScore.HouseNumberScore == 0.0 {
		validation.IsValid = false
		validation.FailureReasons = append(validation.FailureReasons, "house_number_mismatch")
	}
	
	// Rule 2: Minimum overall score threshold
	if candidate.ComponentScore.OverallScore < 0.6 {
		validation.IsValid = false
		validation.FailureReasons = append(validation.FailureReasons, "low_overall_score")
	}
	
	// Rule 3: Must have at least one strong match
	if !candidate.ComponentScore.PostcodeMatch && !candidate.ComponentScore.HouseNumberMatch && !candidate.ComponentScore.RoadMatch {
		validation.IsValid = false
		validation.FailureReasons = append(validation.FailureReasons, "no_strong_component_match")
	}
	
	return validation
}

// makeValidatedDecision makes matching decisions with strict validation
func (e *FixedComponentEngine) makeValidatedDecision(localDebug bool, candidates []ComponentMatch, input map[string]string) (string, string) {
	if len(candidates) == 0 {
		return "no_match", "auto"
	}
	
	best := candidates[0]
	
	debug.DebugOutput(localDebug, "Best validated match: score=%.4f, house_match=%t, postcode_match=%t", 
		best.ComponentScore.OverallScore, best.ComponentScore.HouseNumberMatch, best.ComponentScore.PostcodeMatch)
	
	// Perfect matches (postcode + house number)
	if best.ComponentScore.PostcodeMatch && best.ComponentScore.HouseNumberMatch {
		return "auto_accept", "auto"
	}
	
	// High confidence with validation
	if best.ComponentScore.OverallScore >= 0.95 {
		return "auto_accept", "auto"
	}
	
	// Medium confidence - require manual review
	if best.ComponentScore.OverallScore >= 0.8 {
		return "needs_review", "manual"
	}
	
	// Low confidence 
	if best.ComponentScore.OverallScore >= 0.6 {
		return "low_confidence", "manual"
	}
	
	return "no_match", "auto"
}

// createHistoricUPRNRecord creates a historic address record for UPRNs not found in LLPG
func (e *FixedComponentEngine) createHistoricUPRNRecord(localDebug bool, uprn, fullAddress string, documentID int64) (*MatchCandidate, error) {
	// Normalize UPRN - remove .00 decimal suffix if present
	cleanUPRN := strings.TrimSpace(uprn)
	if strings.HasSuffix(cleanUPRN, ".00") {
		cleanUPRN = strings.TrimSuffix(cleanUPRN, ".00")
		debug.DebugOutput(localDebug, "Normalizing historic UPRN from %s to %s", uprn, cleanUPRN)
	}
	
	debug.DebugOutput(localDebug, "Creating historic UPRN record: uprn=%s, address=%s, doc_id=%d", cleanUPRN, fullAddress, documentID)
	
	// Call the database function to create the historic record
	var addressID int
	err := e.db.QueryRow(`
		SELECT create_historic_uprn_record($1, $2, $3)
	`, cleanUPRN, fullAddress, int(documentID)).Scan(&addressID)
	
	if err != nil {
		debug.DebugOutput(localDebug, "Failed to create historic UPRN record: %v", err)
		return nil, err
	}
	
	debug.DebugOutput(localDebug, "Created historic UPRN record with address_id=%d", addressID)
	
	// Fetch the created record to return as a match candidate
	var candidate MatchCandidate
	var locationID int
	var fullAddr, canonical string
	var easting, northing float64
	
	err = e.db.QueryRow(`
		SELECT a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
		       l.easting, l.northing
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.address_id = $1
	`, addressID).Scan(
		&candidate.AddressID, &locationID, &candidate.UPRN,
		&fullAddr, &canonical, &easting, &northing,
	)
	
	if err != nil {
		debug.DebugOutput(localDebug, "Failed to fetch created historic record: %v", err)
		return nil, err
	}
	
	// Populate the match candidate
	candidate.LocationID = locationID
	candidate.FullAddress = fullAddr
	candidate.AddressCanonical = canonical
	candidate.Easting = &easting
	candidate.Northing = &northing
	candidate.Score = 1.0 // Perfect score for UPRN match
	candidate.MethodCode = "historic_uprn"
	candidate.MethodID = 30 // New method ID for historic UPRN creation
	
	debug.DebugOutput(localDebug, "Historic UPRN candidate created: address_id=%d, uprn=%s", 
		candidate.AddressID, candidate.UPRN)
	
	return &candidate, nil
}