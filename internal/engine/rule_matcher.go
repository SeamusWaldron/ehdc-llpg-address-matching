package engine

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// RuleMatcher handles rule-based matching for known patterns
type RuleMatcher struct {
	db    *sql.DB
	rules []AddressRule
}

// AddressRule represents a transformation rule for addresses
type AddressRule struct {
	ID          int
	Name        string
	Description string
	Pattern     string
	Replacement string
	Confidence  float64
	Active      bool
	CreatedBy   string
	Notes       string
}

// RuleCandidate represents a match found via rule-based matching
type RuleCandidate struct {
	UPRN           string
	Address        string
	CanonicalAddr  string
	RuleName       string
	OriginalAddr   string
	TransformedAddr string
	Confidence     float64
	Features       map[string]interface{}
}

// NewRuleMatcher creates a new rule-based matcher
func NewRuleMatcher(db *sql.DB) *RuleMatcher {
	rm := &RuleMatcher{db: db}
	rm.loadDefaultRules()
	return rm
}

// loadDefaultRules loads the default set of address transformation rules
func (rm *RuleMatcher) loadDefaultRules() {
	rm.rules = []AddressRule{
		// Known problematic patterns from EHDC data
		{
			ID:          1,
			Name:        "lucky_lite_farm",
			Pattern:     `LUCKY LITE FARM.*`,
			Replacement: "LUCKYLITE FARM CATHERINGTON LANE HORNDEAN",
			Confidence:  0.95,
			Active:      true,
			Description: "Fix Lucky Lite Farm address variations",
			Notes:       "Common misspelling in source data",
		},
		{
			ID:          2,
			Name:        "lasham_airfield",
			Pattern:     `LASHAM AIRFIELD.*`,
			Replacement: "LASHAM AERODROME LASHAM",
			Confidence:  0.90,
			Active:      true,
			Description: "Standardize Lasham airfield references",
			Notes:       "Airfield vs Aerodrome terminology",
		},
		{
			ID:          3,
			Name:        "four_marks_spacing",
			Pattern:     `FOUR MARKS`,
			Replacement: "FOURMARKS",
			Confidence:  0.85,
			Active:      true,
			Description: "Fix Four Marks spacing variations",
			Notes:       "Sometimes written as one word",
		},
		{
			ID:          4,
			Name:        "co_op_variations",
			Pattern:     `(?:CO-OP|COOP|CO OP)`,
			Replacement: "COOPERATIVE",
			Confidence:  0.80,
			Active:      true,
			Description: "Standardize cooperative store references",
			Notes:       "Multiple variations of Co-op",
		},
		{
			ID:          5,
			Name:        "former_site_prefix",
			Pattern:     `FORMER SITE OF (.+)`,
			Replacement: "$1",
			Confidence:  0.75,
			Active:      true,
			Description: "Remove 'former site of' prefix",
			Notes:       "Historical references that may still match current addresses",
		},
		{
			ID:          6,
			Name:        "land_at_prefix",
			Pattern:     `LAND AT (.+)`,
			Replacement: "$1",
			Confidence:  0.70,
			Active:      true,
			Description: "Remove 'land at' prefix",
			Notes:       "Development references",
		},
		{
			ID:          7,
			Name:        "rear_of_references",
			Pattern:     `REAR OF (\d+[A-Z]?\s+.+)`,
			Replacement: "$1A",
			Confidence:  0.65,
			Active:      true,
			Description: "Convert 'rear of X' to 'XA'",
			Notes:       "Common pattern for rear properties",
		},
		{
			ID:          8,
			Name:        "adjacent_to",
			Pattern:     `ADJ(?:ACENT)? TO (.+)`,
			Replacement: "$1",
			Confidence:  0.60,
			Active:      true,
			Description: "Remove 'adjacent to' references",
			Notes:       "Relative position descriptions",
		},
		{
			ID:          9,
			Name:        "opposite_references",
			Pattern:     `OPP(?:OSITE)? (.+)`,
			Replacement: "$1",
			Confidence:  0.60,
			Active:      true,
			Description: "Remove 'opposite' references",
			Notes:       "Relative position descriptions",
		},
		{
			ID:          10,
			Name:        "north_south_abbreviations",
			Pattern:     `\b([NS])\b`,
			Replacement: map[string]string{"N": "NORTH", "S": "SOUTH"}["$1"],
			Confidence:  0.75,
			Active:      true,
			Description: "Expand compass abbreviations",
			Notes:       "N/S to North/South",
		},
	}
}

// RunRuleMatching performs rule-based pattern matching
func (rm *RuleMatcher) RunRuleMatching(runID int64, batchSize int) (int, int, int, error) {
	startTime := time.Now()
	totalProcessed := 0
	totalAccepted := 0
	totalNeedsReview := 0

	fmt.Println("Starting rule-based pattern matching...")
	fmt.Printf("Loaded %d active rules\n", len(rm.getActiveRules()))

	engine := &MatchEngine{db: rm.db}

	for {
		// Get unmatched documents
		docs, err := rm.getUnmatchedForRules(batchSize)
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

			sourceAddr := strings.ToUpper(*doc.AddrCan)

			// Try each rule
			var bestCandidate *RuleCandidate
			var matchFound bool

			for _, rule := range rm.getActiveRules() {
				candidate, err := rm.tryRule(doc, sourceAddr, rule)
				if err != nil {
					continue
				}

				if candidate != nil {
					bestCandidate = candidate
					matchFound = true
					break // First matching rule wins
				}
			}

			if !matchFound {
				continue
			}

			// Make decision based on rule confidence
			if bestCandidate.Confidence >= 0.85 {
				// High confidence rule - auto accept
				err = rm.acceptMatch(engine, runID, doc.SrcID, bestCandidate)
				if err == nil {
					totalAccepted++
				}
			} else if bestCandidate.Confidence >= 0.65 {
				// Medium confidence rule - needs review
				rm.saveForReview(engine, runID, doc.SrcID, bestCandidate, 1)
				totalNeedsReview++
			}
			// Below 0.65 is rejected
		}

		if totalProcessed%1000 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(totalProcessed) / elapsed.Seconds()
			fmt.Printf("Processed %d documents (%.1f/sec), accepted %d, needs review %d...\n",
				totalProcessed, rate, totalAccepted, totalNeedsReview)
		}
	}

	fmt.Printf("Rule-based matching complete: processed %d, accepted %d, needs review %d\n",
		totalProcessed, totalAccepted, totalNeedsReview)
	return totalProcessed, totalAccepted, totalNeedsReview, nil
}

// getUnmatchedForRules gets unmatched documents suitable for rule-based matching
func (rm *RuleMatcher) getUnmatchedForRules(limit int) ([]SourceDocument, error) {
	rows, err := rm.db.Query(`
		SELECT s.src_id, s.source_type, s.raw_address, s.addr_can, s.postcode_text,
			   s.easting_raw, s.northing_raw, s.uprn_raw
		FROM src_document s
		LEFT JOIN match_accepted m ON m.src_id = s.src_id
		WHERE m.src_id IS NULL
		  AND s.addr_can IS NOT NULL
		  AND s.addr_can != 'N A'
		  AND s.addr_can != ''
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

// tryRule attempts to apply a rule to an address and find matches
func (rm *RuleMatcher) tryRule(doc SourceDocument, sourceAddr string, rule AddressRule) (*RuleCandidate, error) {
	// Apply the rule transformation
	transformedAddr, matched := rm.applyRule(sourceAddr, rule)
	if !matched {
		return nil, nil // Rule didn't match this address
	}

	// Search for matches using the transformed address
	rows, err := rm.db.Query(`
		SELECT d.uprn, d.full_address, d.address_canonical, similarity($1, d.address_canonical) as sim
		FROM dim_address d
		WHERE d.address_canonical % $1
		  AND similarity($1, d.address_canonical) >= 0.70
		ORDER BY sim DESC
		LIMIT 5
	`, transformedAddr)

	if err != nil {
		return nil, fmt.Errorf("rule query failed: %w", err)
	}
	defer rows.Close()

	// Get the best match
	for rows.Next() {
		candidate := &RuleCandidate{
			RuleName:        rule.Name,
			OriginalAddr:    sourceAddr,
			TransformedAddr: transformedAddr,
			Confidence:      rule.Confidence,
			Features:        make(map[string]interface{}),
		}

		var similarity float64
		err := rows.Scan(
			&candidate.UPRN,
			&candidate.Address,
			&candidate.CanonicalAddr,
			&similarity,
		)
		if err != nil {
			continue
		}

		// Adjust confidence based on similarity
		candidate.Confidence = rm.adjustConfidence(rule.Confidence, similarity)

		// Store features for explainability
		candidate.Features = map[string]interface{}{
			"rule_name":         rule.Name,
			"rule_description":  rule.Description,
			"rule_pattern":      rule.Pattern,
			"rule_replacement":  rule.Replacement,
			"original_address":  sourceAddr,
			"transformed_address": transformedAddr,
			"base_confidence":   rule.Confidence,
			"similarity":        similarity,
			"final_confidence":  candidate.Confidence,
			"rule_notes":        rule.Notes,
			"matching_method":   "rule_based",
		}

		return candidate, nil
	}

	return nil, nil // No matches found for transformed address
}

// applyRule applies a transformation rule to an address
func (rm *RuleMatcher) applyRule(address string, rule AddressRule) (string, bool) {
	regex, err := regexp.Compile("(?i)" + rule.Pattern)
	if err != nil {
		return address, false
	}

	if !regex.MatchString(address) {
		return address, false
	}

	// Handle special replacement cases
	switch rule.Name {
	case "north_south_abbreviations":
		// Special handling for N/S abbreviations
		result := strings.ReplaceAll(address, " N ", " NORTH ")
		result = strings.ReplaceAll(result, " S ", " SOUTH ")
		result = strings.ReplaceAll(result, " E ", " EAST ")
		result = strings.ReplaceAll(result, " W ", " WEST ")
		return result, true

	default:
		// Standard regex replacement
		transformed := regex.ReplaceAllString(address, rule.Replacement)
		return transformed, transformed != address
	}
}

// adjustConfidence adjusts rule confidence based on similarity score
func (rm *RuleMatcher) adjustConfidence(baseConfidence, similarity float64) float64 {
	// Boost confidence if similarity is high
	if similarity >= 0.90 {
		return baseConfidence + 0.10
	} else if similarity >= 0.80 {
		return baseConfidence + 0.05
	} else if similarity < 0.70 {
		// Reduce confidence if similarity is low
		return baseConfidence - 0.10
	}

	return baseConfidence
}

// getActiveRules returns all active rules
func (rm *RuleMatcher) getActiveRules() []AddressRule {
	var activeRules []AddressRule
	for _, rule := range rm.rules {
		if rule.Active {
			activeRules = append(activeRules, rule)
		}
	}
	return activeRules
}

// acceptMatch accepts a rule-based match
func (rm *RuleMatcher) acceptMatch(engine *MatchEngine, runID, srcID int64, candidate *RuleCandidate) error {
	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        fmt.Sprintf("rule_%s", candidate.RuleName),
		Score:         candidate.Confidence,
		Confidence:    candidate.Confidence,
		TieRank:       1,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "auto_accepted",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Rule-based match auto-accepted (rule=%s, confidence=%.3f)",
			candidate.RuleName, candidate.Confidence),
	}

	if err := engine.SaveMatchResult(result); err != nil {
		return fmt.Errorf("failed to save match result: %w", err)
	}

	return engine.AcceptMatch(srcID, candidate.UPRN, fmt.Sprintf("rule_%s", candidate.RuleName),
		candidate.Confidence, candidate.Confidence, runID, "system")
}

// saveForReview saves a rule-based candidate for manual review
func (rm *RuleMatcher) saveForReview(engine *MatchEngine, runID, srcID int64,
	candidate *RuleCandidate, rank int) error {

	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        fmt.Sprintf("rule_%s", candidate.RuleName),
		Score:         candidate.Confidence,
		Confidence:    candidate.Confidence,
		TieRank:       rank,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "needs_review",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Rule-based match requiring review (rule=%s, confidence=%.3f)",
			candidate.RuleName, candidate.Confidence),
	}

	return engine.SaveMatchResult(result)
}

// AddCustomRule adds a custom rule to the matcher
func (rm *RuleMatcher) AddCustomRule(rule AddressRule) {
	rule.ID = len(rm.rules) + 1
	rm.rules = append(rm.rules, rule)
}

// ListRules returns information about all loaded rules
func (rm *RuleMatcher) ListRules() []AddressRule {
	return rm.rules
}

// AnalyzeRuleEffectiveness analyzes how effective the rules are
func (rm *RuleMatcher) AnalyzeRuleEffectiveness() error {
	fmt.Println("\n=== Rule-Based Matching Analysis ===\n")

	// Count potential matches for each rule
	fmt.Println("Rule Effectiveness Analysis:")
	fmt.Println("Rule Name                | Pattern Matches | Description")
	fmt.Println("-------------------------|-----------------|------------------")

	for _, rule := range rm.getActiveRules() {
		// Count how many unmatched documents this rule's pattern matches
		var count int

		err := rm.db.QueryRow(`
			SELECT COUNT(*)
			FROM src_document s
			LEFT JOIN match_accepted m ON m.src_id = s.src_id
			WHERE m.src_id IS NULL
			  AND s.addr_can IS NOT NULL
			  AND s.addr_can != 'N A'
		`).Scan(&count)

		// This is a simplified count - in practice, we'd need to check each address
		// against the regex pattern, which is complex to do in SQL
		if err == nil {
			fmt.Printf("%-24s | %15s | %s\n",
				rule.Name,
				"(SQL limited)", // Would need application-level checking for accurate count
				rule.Description)
		}
	}

	// Show sample addresses that might benefit from rule-based matching
	fmt.Println("\n=== Sample Addresses for Rule Matching ===")
	
	rows, err := rm.db.Query(`
		SELECT addr_can, COUNT(*) as frequency
		FROM src_document s
		LEFT JOIN match_accepted m ON m.src_id = s.src_id
		WHERE m.src_id IS NULL
		  AND s.addr_can IS NOT NULL
		  AND s.addr_can != 'N A'
		  AND (
			addr_can LIKE '%FORMER%' OR
			addr_can LIKE '%LAND AT%' OR
			addr_can LIKE '%REAR OF%' OR
			addr_can LIKE '%ADJACENT%' OR
			addr_can LIKE '%OPPOSITE%' OR
			addr_can LIKE '%CO-OP%' OR
			addr_can LIKE '%LUCKY LITE%' OR
			addr_can LIKE '%LASHAM AIRFIELD%'
		  )
		GROUP BY addr_can
		ORDER BY frequency DESC
		LIMIT 10
	`)

	if err == nil {
		defer rows.Close()
		fmt.Println("Address Pattern                           | Frequency")
		fmt.Println("------------------------------------------|----------")
		
		for rows.Next() {
			var address string
			var frequency int
			if err := rows.Scan(&address, &frequency); err == nil {
				// Truncate long addresses
				displayAddr := address
				if len(displayAddr) > 40 {
					displayAddr = displayAddr[:37] + "..."
				}
				fmt.Printf("%-41s | %9d\n", displayAddr, frequency)
			}
		}
	}

	return nil
}