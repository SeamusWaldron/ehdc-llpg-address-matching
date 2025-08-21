package validation

import (
	"fmt"
	"time"
)

// AddressComponents represents the structured components of a UK address
type AddressComponents struct {
	// Core components
	HouseNumber string `json:"house_number"` // "168", "Unit 2", "5A", "Flat B"
	Street      string `json:"street"`       // "Station Road", "Mill Lane", "Amey Industrial Estate"
	Locality    string `json:"locality"`     // "Alton", "Petersfield", "Liss"
	Postcode    string `json:"postcode"`     // "GU34 2QG", "GU32 3AN"
	
	// Additional components
	SubBuilding string `json:"sub_building"` // "Unit 2", "Flat A", "Suite 1"
	Building    string `json:"building"`     // "Industrial Estate", "Shopping Centre"
	County      string `json:"county"`       // "Hampshire", "Hants"
	
	// Metadata
	Raw                string  `json:"raw"`                 // Original unparsed address
	ExtractionMethod   string  `json:"extraction_method"`   // "gopostal", "regex", "manual"
	ExtractionConfidence float64 `json:"extraction_confidence"` // 0.0-1.0
	ParsedAt           time.Time `json:"parsed_at"`
	
	// Validation flags
	ValidationIssues []string `json:"validation_issues"`
	IsValidForMatching bool   `json:"is_valid_for_matching"`
}

// ValidationResult represents the result of component validation
type ValidationResult struct {
	Valid      bool    `json:"valid"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// AddressValidation holds the complete validation state of an address
type AddressValidation struct {
	Address    string            `json:"address"`
	Components AddressComponents `json:"components"`
	Issues     []string          `json:"issues"`
	Suitable   bool              `json:"suitable_for_matching"`
	Score      float64           `json:"validation_score"`
}

// MatchDecision represents a comprehensive matching decision
type MatchDecision struct {
	Accept         bool    `json:"accept"`
	Confidence     float64 `json:"confidence"`
	Method         string  `json:"method"`
	Reason         string  `json:"reason"`
	RequiresReview bool    `json:"requires_review"`
	
	// Component-level validation
	ComponentValidation ComponentValidation `json:"component_validation"`
	
	// Audit information
	DecisionTime time.Time `json:"decision_time"`
	Version      string    `json:"version"` // Algorithm version for tracking
}

// ComponentValidation holds detailed component-level validation results
type ComponentValidation struct {
	HouseNumberMatch ValidationResult `json:"house_number_match"`
	StreetMatch      ValidationResult `json:"street_match"`
	PostcodeMatch    ValidationResult `json:"postcode_match"`
	LocalityMatch    ValidationResult `json:"locality_match"`
	OverallScore     float64          `json:"overall_score"`
}

// MatchAudit represents a quality audit of a match
type MatchAudit struct {
	MatchID    int64         `json:"match_id"`
	Timestamp  time.Time     `json:"timestamp"`
	Checks     []QualityCheck `json:"checks"`
	Score      float64       `json:"score"`
	Passed     bool          `json:"passed"`
	Issues     []string      `json:"issues"`
}

// QualityCheck represents an individual quality validation check
type QualityCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details"`
	Impact  string `json:"impact"` // "critical", "major", "minor"
}

// ParsingConfig holds configuration for address parsing
type ParsingConfig struct {
	// UK-specific patterns
	UKIndustrialEstatePatterns []string `json:"uk_industrial_estate_patterns"`
	UKUnitPatterns            []string `json:"uk_unit_patterns"`
	UKFlatPatterns            []string `json:"uk_flat_patterns"`
	
	// Common abbreviations
	StreetTypeAbbreviations map[string]string `json:"street_type_abbreviations"`
	CountyAbbreviations     map[string]string `json:"county_abbreviations"`
	
	// Validation thresholds
	MinHouseNumberConfidence float64 `json:"min_house_number_confidence"`
	MinStreetConfidence      float64 `json:"min_street_confidence"`
	MinOverallConfidence     float64 `json:"min_overall_confidence"`
}

// MatchingThresholds defines conservative thresholds for address matching
type MatchingThresholds struct {
	// Component-level thresholds
	HouseNumberSimilarity float64 `json:"house_number_similarity"` // Must be 1.0 (exact)
	StreetSimilarity      float64 `json:"street_similarity"`       // ≥0.90
	PostcodeSimilarity    float64 `json:"postcode_similarity"`     // ≥0.95
	
	// Overall thresholds
	MinAutoAcceptConfidence float64 `json:"min_auto_accept_confidence"` // ≥0.95
	MinReviewConfidence     float64 `json:"min_review_confidence"`      // ≥0.70
	MaxEditDistance         int     `json:"max_edit_distance"`           // ≤5
	
	// Quality gates
	RequireHouseNumberMatch bool `json:"require_house_number_match"` // true
	RequireStreetValidation bool `json:"require_street_validation"`  // true
	AllowPostcodeDistrict   bool `json:"allow_postcode_district"`    // true (GU34 ≈ GU32)
}

// String methods for debugging and logging
func (ac AddressComponents) String() string {
	return fmt.Sprintf("House: %s, Street: %s, Locality: %s, Postcode: %s", 
		ac.HouseNumber, ac.Street, ac.Locality, ac.Postcode)
}

func (vr ValidationResult) String() string {
	if vr.Valid {
		return fmt.Sprintf("VALID (%.2f): %s", vr.Confidence, vr.Reason)
	}
	return fmt.Sprintf("INVALID: %s", vr.Reason)
}

func (md MatchDecision) String() string {
	if md.Accept {
		return fmt.Sprintf("ACCEPT (%.2f) via %s: %s", md.Confidence, md.Method, md.Reason)
	}
	reviewFlag := ""
	if md.RequiresReview {
		reviewFlag = " [REVIEW]"
	}
	return fmt.Sprintf("REJECT%s via %s: %s", reviewFlag, md.Method, md.Reason)
}

// Helper methods for common validations
func (ac AddressComponents) HasHouseNumber() bool {
	return ac.HouseNumber != ""
}

func (ac AddressComponents) HasStreet() bool {
	return len(ac.Street) >= 3
}

func (ac AddressComponents) HasValidPostcode() bool {
	return len(ac.Postcode) >= 6 && len(ac.Postcode) <= 8
}

func (ac AddressComponents) IsComplete() bool {
	return ac.HasHouseNumber() && ac.HasStreet() && ac.HasValidPostcode()
}

// DefaultParsingConfig returns sensible defaults for UK address parsing
func DefaultParsingConfig() ParsingConfig {
	return ParsingConfig{
		UKIndustrialEstatePatterns: []string{
			`(?i)\b(INDUSTRIAL\s+ESTATE?)\b`,
			`(?i)\b(IND\s+EST)\b`,
			`(?i)\b(TRADING\s+ESTATE?)\b`,
		},
		UKUnitPatterns: []string{
			`(?i)\b(UNIT\s+\d+[A-Z]?)\b`,
			`(?i)\b(UNITS?\s+\d+[A-Z]?[-/]\d+[A-Z]?)\b`,
		},
		UKFlatPatterns: []string{
			`(?i)\b(FLAT\s+[A-Z0-9]+)\b`,
			`(?i)\b(APT\s+[A-Z0-9]+)\b`,
			`(?i)\b(APARTMENT\s+[A-Z0-9]+)\b`,
		},
		StreetTypeAbbreviations: map[string]string{
			"RD":      "ROAD",
			"ST":      "STREET",
			"AVE":     "AVENUE",
			"CRESC":   "CRESCENT",
			"CRES":    "CRESCENT",
			"CL":      "CLOSE",
			"CLS":     "CLOSE",
			"CT":      "COURT",
			"DR":      "DRIVE",
			"GDNS":    "GARDENS",
			"GDN":     "GARDEN",
			"LN":      "LANE",
			"PK":      "PARK",
			"PL":      "PLACE",
			"SQ":      "SQUARE",
			"TER":     "TERRACE",
			"WY":      "WAY",
			"WLK":     "WALK",
			"EST":     "ESTATE",
			"IND":     "INDUSTRIAL",
			"INDUSTR": "INDUSTRIAL",
		},
		CountyAbbreviations: map[string]string{
			"HANTS": "HAMPSHIRE",
		},
		MinHouseNumberConfidence: 0.8,
		MinStreetConfidence:      0.7,
		MinOverallConfidence:     0.6,
	}
}

// DefaultMatchingThresholds returns conservative thresholds for high-precision matching
func DefaultMatchingThresholds() MatchingThresholds {
	return MatchingThresholds{
		HouseNumberSimilarity:   1.0,   // Exact match required
		StreetSimilarity:        0.90,  // Very high similarity
		PostcodeSimilarity:      0.95,  // Near-exact match
		MinAutoAcceptConfidence: 0.95,  // Very conservative
		MinReviewConfidence:     0.70,  // Reasonable review threshold
		MaxEditDistance:         5,     // Small differences only
		RequireHouseNumberMatch: true,  // Mandatory house number validation
		RequireStreetValidation: true,  // Mandatory street validation
		AllowPostcodeDistrict:   true,  // Allow same district (GU34 ≈ GU32)
	}
}