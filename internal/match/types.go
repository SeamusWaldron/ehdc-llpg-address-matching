package match

import (
	"time"
)

// Input represents a query for address matching
type Input struct {
	SrcID        int64      // Source document ID for audit trail
	RawAddress   string
	Easting      *float64 // optional
	Northing     *float64 // optional
	LegacyUPRN   string   // optional
	SourceType   string   // e.g. "decision", "enforcement"
	DocDate      *time.Time
}

// Candidate represents a potential UPRN match with scoring details
type Candidate struct {
	UPRN        string
	LocAddress  string
	Easting     float64
	Northing    float64
	Score       float64
	Features    map[string]interface{} // explainability
	Methods     []string              // which generators hit (valid_uprn, trigram, vector, etc.)
}

// Result represents the complete matching result
type Result struct {
	Query         Input
	Candidates    []Candidate // sorted hi→lo
	Decision      string      // "auto_accept" | "review" | "reject"
	AcceptedUPRN  string
	Thresholds    map[string]float64
	ProcessingTime time.Duration
}

// MatchTiers defines the matching confidence tiers
type MatchTiers struct {
	AutoAcceptHigh   float64 // >= 0.92
	AutoAcceptMedium float64 // >= 0.88 with conditions
	ReviewThreshold  float64 // >= 0.80
	MinThreshold     float64 // >= 0.70
	WinnerMargin     float64 // 0.03-0.05 gap to next candidate
}

// DefaultTiers returns the recommended tier thresholds from ADDRESS_MATCHING_ALGORITHM.md
func DefaultTiers() *MatchTiers {
	return &MatchTiers{
		AutoAcceptHigh:   0.92,
		AutoAcceptMedium: 0.88,
		ReviewThreshold:  0.80,
		MinThreshold:     0.70,
		WinnerMargin:     0.03,
	}
}

// FeatureWeights defines the scoring weights for different features
type FeatureWeights struct {
	TrigramSimilarity     float64 // 0.45
	EmbeddingCosine       float64 // 0.45
	LocalityOverlap       float64 // 0.05
	StreetOverlap         float64 // 0.05
	SameHouseNumber       float64 // 0.08
	SameHouseAlpha        float64 // 0.02
	USRNMatch             float64 // 0.04
	LLPGLive              float64 // 0.03
	LegacyUPRNValid       float64 // 0.20
	SpatialBoostMax       float64 // varies with distance
	DescriptorPenalty     float64 // -0.05
	PhoneticMissPenalty   float64 // -0.03
}

// DefaultWeights returns the recommended feature weights from ADDRESS_MATCHING_ALGORITHM.md
func DefaultWeights() *FeatureWeights {
	return &FeatureWeights{
		TrigramSimilarity:   0.45,
		EmbeddingCosine:     0.45,
		LocalityOverlap:     0.05,
		StreetOverlap:       0.05,
		SameHouseNumber:     0.08,
		SameHouseAlpha:      0.02,
		USRNMatch:           0.04,
		LLPGLive:            0.03,
		LegacyUPRNValid:     0.20,
		SpatialBoostMax:     0.10,
		DescriptorPenalty:   -0.05,
		PhoneticMissPenalty: -0.03,
	}
}