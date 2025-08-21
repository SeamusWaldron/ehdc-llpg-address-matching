package matcher

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ehdc-llpg/internal/debug"
	"github.com/ehdc-llpg/internal/normalize"
)

// ComponentEngine performs component-based address matching
// This engine focuses on matching individual address components
// and can be enhanced with real gopostal parsing later
type ComponentEngine struct {
	db *sql.DB
}

// ComponentMatch represents a match with component-level details
type ComponentMatch struct {
	MatchCandidate
	ComponentScore ComponentScore
	MatchedComponents []string
}

// ComponentScore tracks which components matched and their confidence
type ComponentScore struct {
	HouseNumberMatch bool
	HouseNumberScore float64
	RoadMatch        bool
	RoadScore        float64
	CityMatch        bool
	CityScore        float64
	PostcodeMatch    bool
	PostcodeScore    float64
	UnitMatch        bool
	UnitScore        float64
	OverallScore     float64
	MatchedCount     int
	TotalCount       int
}

// NewComponentEngine creates a component-based matching engine
func NewComponentEngine(db *sql.DB) *ComponentEngine {
	return &ComponentEngine{db: db}
}

// ProcessDocument performs component-based address matching
func (e *ComponentEngine) ProcessDocument(localDebug bool, input MatchInput) (*MatchResult, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)
	
	startTime := time.Now()
	
	debug.DebugOutput(localDebug, "Processing document %d (COMPONENT): %s", input.DocumentID, input.RawAddress)
	
	// First, populate component data if not already done
	err := e.ensureComponentData(localDebug, input)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure component data: %w", err)
	}
	
	// Get or extract components for the input
	inputComponents, err := e.getInputComponents(localDebug, input)
	if err != nil {
		return nil, fmt.Errorf("failed to extract input components: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Input components: house_number='%s', road='%s', city='%s', postcode='%s'",
		inputComponents["house_number"], inputComponents["road"], inputComponents["city"], inputComponents["postcode"])
	
	// Perform component-based matching
	candidates, err := e.performComponentMatching(localDebug, inputComponents)
	if err != nil {
		return nil, fmt.Errorf("component matching failed: %w", err)
	}
	
	// Convert to standard candidates
	var standardCandidates []MatchCandidate
	for _, cm := range candidates {
		standardCandidates = append(standardCandidates, cm.MatchCandidate)
	}
	
	// Make decision
	decision, matchStatus := e.makeComponentDecision(localDebug, candidates)
	
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
	
	debug.DebugOutput(localDebug, "Component decision: %s with %d candidates in %v", 
		decision, len(standardCandidates), result.ProcessingTime)
	
	return result, nil
}

// ensureComponentData ensures the source document has component data
func (e *ComponentEngine) ensureComponentData(localDebug bool, input MatchInput) error {
	// Check if already processed
	var isProcessed bool
	err := e.db.QueryRow("SELECT COALESCE(gopostal_processed, FALSE) FROM src_document WHERE document_id = $1", 
		input.DocumentID).Scan(&isProcessed)
	
	if err == nil && isProcessed {
		return nil // Already processed
	}
	
	// Extract components using our enhanced parser
	components := e.parseAddressComponents(localDebug, input.RawAddress)
	
	// Update the source document with components
	_, err = e.db.Exec(`
		UPDATE src_document SET
			gopostal_house_number = $2,
			gopostal_road = $3,
			gopostal_city = $4,
			gopostal_postcode = $5,
			gopostal_unit = $6,
			gopostal_processed = TRUE
		WHERE document_id = $1
	`, input.DocumentID, 
		nullIfEmpty(components["house_number"]),
		nullIfEmpty(components["road"]),
		nullIfEmpty(components["city"]),
		nullIfEmpty(components["postcode"]),
		nullIfEmpty(components["unit"]))
	
	if err != nil {
		debug.DebugOutput(localDebug, "Warning: failed to update source components: %v", err)
	}
	
	return nil
}

// parseAddressComponents extracts components from an address
// This is a placeholder for real gopostal integration
func (e *ComponentEngine) parseAddressComponents(localDebug bool, address string) map[string]string {
	components := make(map[string]string)
	
	// Use our existing normalization tools enhanced with component extraction
	canonical, postcode, tokens := normalize.CanonicalAddressDebug(localDebug, address)
	
	// Extract postcode
	if postcode != "" {
		components["postcode"] = postcode
	}
	
	// Extract house numbers
	houseNumbers := normalize.ExtractHouseNumbers(canonical)
	if len(houseNumbers) > 0 {
		components["house_number"] = houseNumbers[0]
	}
	
	// Extract localities/cities
	localities := normalize.ExtractLocalityTokens(canonical)
	if len(localities) > 0 {
		components["city"] = localities[0]
	}
	
	// Extract street/road
	streetTokens := normalize.TokenizeStreet(canonical)
	if len(streetTokens) > 0 {
		components["road"] = strings.Join(streetTokens, " ")
	}
	
	// Check for units/flats
	upperAddr := strings.ToUpper(address)
	if strings.Contains(upperAddr, "FLAT") {
		for i, token := range tokens {
			if token == "FLAT" && i+1 < len(tokens) {
				components["unit"] = "FLAT " + tokens[i+1]
				break
			}
		}
	}
	
	// Expand common abbreviations
	if road := components["road"]; road != "" {
		road = strings.ReplaceAll(road, " ST ", " STREET ")
		road = strings.ReplaceAll(road, " RD ", " ROAD ")
		road = strings.ReplaceAll(road, " AVE ", " AVENUE ")
		road = strings.ReplaceAll(road, " LN ", " LANE ")
		components["road"] = road
	}
	
	debug.DebugOutput(localDebug, "Parsed components: %v", components)
	
	return components
}

// getInputComponents gets the components for the input address
func (e *ComponentEngine) getInputComponents(localDebug bool, input MatchInput) (map[string]string, error) {
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
			components["house_number"] = houseNumber.String
		}
		if road.Valid {
			components["road"] = road.String
		}
		if city.Valid {
			components["city"] = city.String
		}
		if postcode.Valid {
			components["postcode"] = postcode.String
		}
		if unit.Valid {
			components["unit"] = unit.String
		}
	} else {
		// Parse on the fly
		components = e.parseAddressComponents(localDebug, input.RawAddress)
	}
	
	return components, nil
}

// performComponentMatching performs the actual component-based matching
func (e *ComponentEngine) performComponentMatching(localDebug bool, inputComponents map[string]string) ([]ComponentMatch, error) {
	var allCandidates []ComponentMatch
	
	// Strategy 1: Exact component matches (highest priority)
	candidates := e.exactComponentMatch(localDebug, inputComponents)
	allCandidates = append(allCandidates, candidates...)
	
	// Strategy 2: Postcode + House Number match
	if len(allCandidates) < 10 && inputComponents["postcode"] != "" && inputComponents["house_number"] != "" {
		candidates = e.postcodeHouseMatch(localDebug, inputComponents)
		allCandidates = append(allCandidates, candidates...)
	}
	
	// Strategy 3: Road + City match with fuzzy
	if len(allCandidates) < 20 && inputComponents["road"] != "" && inputComponents["city"] != "" {
		candidates = e.roadCityMatch(localDebug, inputComponents)
		allCandidates = append(allCandidates, candidates...)
	}
	
	// Strategy 4: Fuzzy road-only match
	if len(allCandidates) < 30 && inputComponents["road"] != "" {
		candidates = e.fuzzyRoadMatch(localDebug, inputComponents)
		allCandidates = append(allCandidates, candidates...)
	}
	
	// Deduplicate and rank
	dedupedCandidates := e.deduplicateComponentMatches(allCandidates)
	
	debug.DebugOutput(localDebug, "Component matching found %d candidates total, %d after deduplication", 
		len(allCandidates), len(dedupedCandidates))
	
	return dedupedCandidates, nil
}

// exactComponentMatch finds exact component matches
func (e *ComponentEngine) exactComponentMatch(localDebug bool, input map[string]string) []ComponentMatch {
	var candidates []ComponentMatch
	
	// Build WHERE clause dynamically based on available components
	var conditions []string
	var args []interface{}
	argIndex := 1
	
	if input["postcode"] != "" {
		conditions = append(conditions, fmt.Sprintf("a.gopostal_postcode = $%d", argIndex))
		args = append(args, input["postcode"])
		argIndex++
	}
	
	if input["house_number"] != "" {
		conditions = append(conditions, fmt.Sprintf("a.gopostal_house_number = $%d", argIndex))
		args = append(args, input["house_number"])
		argIndex++
	}
	
	if input["road"] != "" {
		conditions = append(conditions, fmt.Sprintf("a.gopostal_road = $%d", argIndex))
		args = append(args, input["road"])
		argIndex++
	}
	
	if input["city"] != "" {
		conditions = append(conditions, fmt.Sprintf("a.gopostal_city = $%d", argIndex))
		args = append(args, input["city"])
		argIndex++
	}
	
	if len(conditions) == 0 {
		return candidates
	}
	
	query := fmt.Sprintf(`
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing,
			a.gopostal_house_number, a.gopostal_road, a.gopostal_city, a.gopostal_postcode
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE %s
		LIMIT 50
	`, strings.Join(conditions, " AND "))
	
	rows, err := e.db.Query(query, args...)
	if err != nil {
		debug.DebugOutput(localDebug, "Exact component query error: %v", err)
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
		
		// Calculate component score
		cm.ComponentScore = e.calculateComponentScore(input, map[string]string{
			"house_number": dbHouseNum.String,
			"road":         dbRoad.String,
			"city":         dbCity.String,
			"postcode":     dbPostcode.String,
		})
		
		cm.MatchCandidate.Score = cm.ComponentScore.OverallScore
		cm.MatchCandidate.MethodCode = "exact_components"
		cm.MatchCandidate.MethodID = 13
		
		candidates = append(candidates, cm)
	}
	
	debug.DebugOutput(localDebug, "Exact component match found %d candidates", len(candidates))
	return candidates
}

// postcodeHouseMatch matches on postcode + house number
func (e *ComponentEngine) postcodeHouseMatch(localDebug bool, input map[string]string) []ComponentMatch {
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
		WHERE a.gopostal_postcode = $1 AND a.gopostal_house_number = $2
		LIMIT 20
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
		
		cm.ComponentScore = e.calculateComponentScore(input, map[string]string{
			"house_number": dbHouseNum.String,
			"road":         dbRoad.String,
			"city":         dbCity.String,
			"postcode":     dbPostcode.String,
		})
		
		cm.MatchCandidate.Score = cm.ComponentScore.OverallScore
		cm.MatchCandidate.MethodCode = "postcode_house"
		cm.MatchCandidate.MethodID = 8
		
		candidates = append(candidates, cm)
	}
	
	debug.DebugOutput(localDebug, "Postcode+house match found %d candidates", len(candidates))
	return candidates
}

// roadCityMatch matches on road + city with some fuzzy matching
func (e *ComponentEngine) roadCityMatch(localDebug bool, input map[string]string) []ComponentMatch {
	var candidates []ComponentMatch
	
	if input["road"] == "" || input["city"] == "" {
		return candidates
	}
	
	// Try exact first, then fuzzy
	queries := []string{
		`WHERE a.gopostal_road = $1 AND a.gopostal_city = $2`,
		`WHERE a.gopostal_road % $1 AND a.gopostal_city = $2 AND similarity(a.gopostal_road, $1) >= 0.7`,
	}
	
	for i, whereClause := range queries {
		query := fmt.Sprintf(`
			SELECT 
				a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
				l.easting, l.northing,
				a.gopostal_house_number, a.gopostal_road, a.gopostal_city, a.gopostal_postcode,
				COALESCE(similarity(a.gopostal_road, $1), 1.0) as road_sim
			FROM dim_address a
			INNER JOIN dim_location l ON l.location_id = a.location_id
			%s
			ORDER BY road_sim DESC
			LIMIT 30
		`, whereClause)
		
		rows, err := e.db.Query(query, input["road"], input["city"])
		if err != nil {
			debug.DebugOutput(localDebug, "Road+city query %d error: %v", i, err)
			continue
		}
		
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
			
			cm.ComponentScore = e.calculateComponentScore(input, map[string]string{
				"house_number": dbHouseNum.String,
				"road":         dbRoad.String,
				"city":         dbCity.String,
				"postcode":     dbPostcode.String,
			})
			
			// Adjust score based on road similarity
			cm.ComponentScore.OverallScore *= roadSim
			cm.MatchCandidate.Score = cm.ComponentScore.OverallScore
			
			if i == 0 {
				cm.MatchCandidate.MethodCode = "road_city_exact"
			} else {
				cm.MatchCandidate.MethodCode = "road_city_fuzzy"
			}
			cm.MatchCandidate.MethodID = 9
			
			candidates = append(candidates, cm)
		}
		rows.Close()
		
		// If we got good exact matches, don't try fuzzy
		if i == 0 && len(candidates) > 5 {
			break
		}
	}
	
	debug.DebugOutput(localDebug, "Road+city match found %d candidates", len(candidates))
	return candidates
}

// fuzzyRoadMatch performs fuzzy matching on road names
func (e *ComponentEngine) fuzzyRoadMatch(localDebug bool, input map[string]string) []ComponentMatch {
	var candidates []ComponentMatch
	
	if input["road"] == "" {
		return candidates
	}
	
	rows, err := e.db.Query(`
		SELECT 
			a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
			l.easting, l.northing,
			a.gopostal_house_number, a.gopostal_road, a.gopostal_city, a.gopostal_postcode,
			similarity(a.gopostal_road, $1) as road_sim
		FROM dim_address a
		INNER JOIN dim_location l ON l.location_id = a.location_id
		WHERE a.gopostal_road % $1 
		  AND similarity(a.gopostal_road, $1) >= 0.6
		ORDER BY road_sim DESC
		LIMIT 50
	`, input["road"])
	
	if err != nil {
		debug.DebugOutput(localDebug, "Fuzzy road query error: %v", err)
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
		
		cm.ComponentScore = e.calculateComponentScore(input, map[string]string{
			"house_number": dbHouseNum.String,
			"road":         dbRoad.String,
			"city":         dbCity.String,
			"postcode":     dbPostcode.String,
		})
		
		// Weight by road similarity
		cm.ComponentScore.OverallScore = roadSim * 0.8 // Reduce for fuzzy match
		cm.MatchCandidate.Score = cm.ComponentScore.OverallScore
		cm.MatchCandidate.MethodCode = "fuzzy_road"
		cm.MatchCandidate.MethodID = 10
		
		candidates = append(candidates, cm)
	}
	
	debug.DebugOutput(localDebug, "Fuzzy road match found %d candidates", len(candidates))
	return candidates
}

// calculateComponentScore calculates how well components match
func (e *ComponentEngine) calculateComponentScore(input, candidate map[string]string) ComponentScore {
	score := ComponentScore{}
	
	// House number matching
	if input["house_number"] != "" {
		score.TotalCount++
		if candidate["house_number"] == input["house_number"] {
			score.HouseNumberMatch = true
			score.HouseNumberScore = 1.0
			score.MatchedCount++
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
			if score.RoadScore >= 0.7 {
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
	
	// Calculate overall score
	if score.TotalCount > 0 {
		score.OverallScore = float64(score.MatchedCount) / float64(score.TotalCount)
		
		// Boost for important components
		if score.PostcodeMatch && score.HouseNumberMatch {
			score.OverallScore = 1.0 // Perfect match
		} else if score.PostcodeMatch {
			score.OverallScore += 0.2 // Postcode is very reliable
		} else if score.HouseNumberMatch && score.RoadMatch {
			score.OverallScore += 0.1 // House + road is good
		}
		
		// Cap at 1.0
		if score.OverallScore > 1.0 {
			score.OverallScore = 1.0
		}
	}
	
	return score
}

// deduplicateComponentMatches removes duplicates and sorts by score
func (e *ComponentEngine) deduplicateComponentMatches(candidates []ComponentMatch) []ComponentMatch {
	addressMap := make(map[int]ComponentMatch)
	
	for _, cand := range candidates {
		existing, exists := addressMap[cand.MatchCandidate.AddressID]
		if !exists || cand.ComponentScore.OverallScore > existing.ComponentScore.OverallScore {
			addressMap[cand.MatchCandidate.AddressID] = cand
		}
	}
	
	var deduped []ComponentMatch
	for _, cand := range addressMap {
		deduped = append(deduped, cand)
	}
	
	// Sort by score
	for i := 0; i < len(deduped)-1; i++ {
		for j := i + 1; j < len(deduped); j++ {
			if deduped[j].ComponentScore.OverallScore > deduped[i].ComponentScore.OverallScore {
				deduped[i], deduped[j] = deduped[j], deduped[i]
			}
		}
	}
	
	return deduped
}

// makeComponentDecision makes a decision based on component matching
func (e *ComponentEngine) makeComponentDecision(localDebug bool, candidates []ComponentMatch) (string, string) {
	if len(candidates) == 0 {
		return "no_match", "auto"
	}
	
	best := candidates[0]
	
	debug.DebugOutput(localDebug, "Best match: score=%.4f, matched=%d/%d components", 
		best.ComponentScore.OverallScore, best.ComponentScore.MatchedCount, best.ComponentScore.TotalCount)
	
	// Very high confidence
	if best.ComponentScore.OverallScore >= 0.95 || 
		(best.ComponentScore.PostcodeMatch && best.ComponentScore.HouseNumberMatch) {
		return "auto_accept", "auto"
	}
	
	// High confidence
	if best.ComponentScore.OverallScore >= 0.85 {
		return "auto_accept", "auto"
	}
	
	// Medium confidence
	if best.ComponentScore.OverallScore >= 0.70 {
		return "needs_review", "manual"
	}
	
	// Low confidence
	if best.ComponentScore.OverallScore >= 0.50 {
		return "needs_review", "manual"
	}
	
	return "low_confidence", "manual"
}

// SaveMatchResult saves the matching result to the database
func (e *ComponentEngine) SaveMatchResult(localDebug bool, result *MatchResult) error {
	if result.BestCandidate == nil {
		debug.DebugOutput(localDebug, "No match to save for document %d", result.DocumentID)
		return nil
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
		result.BestCandidate.MethodID,
		result.BestCandidate.Score,
		result.MatchStatus,
		"system_component",
	)
	
	if err != nil {
		return fmt.Errorf("failed to save component match result: %w", err)
	}
	
	debug.DebugOutput(localDebug, "Saved component match result for document %d -> address %d (%.4f)", 
		result.DocumentID, result.BestCandidate.AddressID, result.BestCandidate.Score)
	
	return nil
}

// Helper function
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}