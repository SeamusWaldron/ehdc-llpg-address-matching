package match

import (
	"math"
	"sort"

	"github.com/ehdc-llpg/internal/debug"
)

// Scorer implements the scoring algorithm from ADDRESS_MATCHING_ALGORITHM.md
type Scorer struct {
	weights *FeatureWeights
	tiers   *MatchTiers
}

// NewScorer creates a new scorer with default weights and tiers
func NewScorer() *Scorer {
	return &Scorer{
		weights: DefaultWeights(),
		tiers:   DefaultTiers(),
	}
}

// NewScorerWithConfig creates a scorer with custom weights and tiers
func NewScorerWithConfig(weights *FeatureWeights, tiers *MatchTiers) *Scorer {
	return &Scorer{
		weights: weights,
		tiers:   tiers,
	}
}

// ScoreCandidates scores all candidates and updates their Score field
func (s *Scorer) ScoreCandidates(localDebug bool, candidates []Candidate, legacyUPRNValid bool) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	for i := range candidates {
		candidates[i].Score = s.ScoreCandidate(localDebug, candidates[i].Features, legacyUPRNValid)
		debug.DebugOutput(localDebug, "Candidate %s scored: %.4f", candidates[i].UPRN, candidates[i].Score)
	}

	// Sort candidates by score (highest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	debug.DebugOutput(localDebug, "Scored %d candidates, top score: %.4f", len(candidates), 
		func() float64 { if len(candidates) > 0 { return candidates[0].Score }; return 0.0 }())
}

// ScoreCandidate computes the final score for a single candidate using the feature weights
func (s *Scorer) ScoreCandidate(localDebug bool, features map[string]interface{}, legacyUPRNValid bool) float64 {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	var score float64

	// Core similarities (weighted 0.45 + 0.45 = 0.90 total)
	trigramSim := s.getFloatFeature(features, "trigram_similarity", 0.0)
	embeddingCos := s.getFloatFeature(features, "embedding_cosine", 0.0)
	
	score += s.weights.TrigramSimilarity * trigramSim
	score += s.weights.EmbeddingCosine * embeddingCos
	
	debug.DebugOutput(localDebug, "Core similarities: trgm=%.3f*%.2f + emb=%.3f*%.2f = %.4f", 
		trigramSim, s.weights.TrigramSimilarity, embeddingCos, s.weights.EmbeddingCosine, 
		s.weights.TrigramSimilarity*trigramSim + s.weights.EmbeddingCosine*embeddingCos)

	// Token overlaps
	localityOverlap := s.getFloatFeature(features, "locality_overlap_ratio", 0.0)
	streetOverlap := s.getFloatFeature(features, "street_overlap_ratio", 0.0)
	
	score += s.weights.LocalityOverlap * localityOverlap
	score += s.weights.StreetOverlap * streetOverlap
	
	debug.DebugOutput(localDebug, "Token overlaps: locality=%.3f*%.2f + street=%.3f*%.2f = %.4f", 
		localityOverlap, s.weights.LocalityOverlap, streetOverlap, s.weights.StreetOverlap,
		s.weights.LocalityOverlap*localityOverlap + s.weights.StreetOverlap*streetOverlap)

	// Boolean features (positive boosts)
	var boosts float64
	
	if s.getBoolFeature(features, "has_same_house_num") {
		boosts += s.weights.SameHouseNumber
		debug.DebugOutput(localDebug, "House number match: +%.3f", s.weights.SameHouseNumber)
	}
	
	if s.getBoolFeature(features, "has_same_house_alpha") {
		boosts += s.weights.SameHouseAlpha
		debug.DebugOutput(localDebug, "House alpha match: +%.3f", s.weights.SameHouseAlpha)
	}
	
	if s.getBoolFeature(features, "usrn_match") {
		boosts += s.weights.USRNMatch
		debug.DebugOutput(localDebug, "USRN match: +%.3f", s.weights.USRNMatch)
	}
	
	if s.getBoolFeature(features, "llpg_live") {
		boosts += s.weights.LLPGLive
		debug.DebugOutput(localDebug, "LLPG live status: +%.3f", s.weights.LLPGLive)
	}
	
	if legacyUPRNValid {
		boosts += s.weights.LegacyUPRNValid
		debug.DebugOutput(localDebug, "Legacy UPRN valid: +%.3f", s.weights.LegacyUPRNValid)
	}
	
	score += boosts

	// Spatial boost
	spatialBoost := s.getFloatFeature(features, "spatial_boost", 0.0)
	score += spatialBoost
	if spatialBoost > 0 {
		debug.DebugOutput(localDebug, "Spatial boost: +%.4f", spatialBoost)
	}

	// Penalties (negative impacts)
	var penalties float64
	
	if s.getBoolFeature(features, "descriptor_penalty") {
		penalties += s.weights.DescriptorPenalty // This is negative
		debug.DebugOutput(localDebug, "Descriptor penalty: %.3f", s.weights.DescriptorPenalty)
	}
	
	phoneticHits := s.getIntFeature(features, "phonetic_hits", 0)
	if phoneticHits == 0 {
		penalties += s.weights.PhoneticMissPenalty // This is negative
		debug.DebugOutput(localDebug, "No phonetic matches penalty: %.3f", s.weights.PhoneticMissPenalty)
	}
	
	score += penalties

	// Clamp score to [0, 1] range
	score = math.Max(0.0, math.Min(1.0, score))
	
	debug.DebugOutput(localDebug, "Final score (clamped): %.4f", score)
	
	return score
}

// MakeDecision determines the match decision based on scores and thresholds
func (s *Scorer) MakeDecision(localDebug bool, candidates []Candidate) (decision string, acceptedUPRN string) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	if len(candidates) == 0 {
		debug.DebugOutput(localDebug, "No candidates - reject")
		return "reject", ""
	}

	topCandidate := candidates[0]
	topScore := topCandidate.Score

	debug.DebugOutput(localDebug, "Top candidate: %s (score=%.4f)", topCandidate.UPRN, topScore)

	// Check if score is below minimum threshold
	if topScore < s.tiers.MinThreshold {
		debug.DebugOutput(localDebug, "Score %.4f below min threshold %.4f - reject", topScore, s.tiers.MinThreshold)
		return "reject", ""
	}

	// Calculate margin to next best candidate
	var margin float64 = 1.0 // Default to maximum margin if only one candidate
	if len(candidates) > 1 {
		margin = topScore - candidates[1].Score
		debug.DebugOutput(localDebug, "Margin to next candidate: %.4f (next score: %.4f)", margin, candidates[1].Score)
	}

	// Auto-accept high confidence with sufficient margin
	if topScore >= s.tiers.AutoAcceptHigh && margin >= s.tiers.WinnerMargin {
		debug.DebugOutput(localDebug, "Auto-accept: high confidence %.4f >= %.4f with margin %.4f >= %.4f", 
			topScore, s.tiers.AutoAcceptHigh, margin, s.tiers.WinnerMargin)
		return "auto_accept", topCandidate.UPRN
	}

	// Auto-accept medium confidence with additional conditions and larger margin
	if topScore >= s.tiers.AutoAcceptMedium && margin >= s.tiers.WinnerMargin+0.02 {
		// Additional conditions for medium confidence auto-accept
		hasHouseNumber := s.getBoolFeature(topCandidate.Features, "has_same_house_num")
		localityOverlap := s.getFloatFeature(topCandidate.Features, "locality_overlap_ratio", 0.0)
		
		if hasHouseNumber && localityOverlap >= 0.5 {
			debug.DebugOutput(localDebug, "Auto-accept: medium confidence %.4f >= %.4f with house number and locality overlap %.3f", 
				topScore, s.tiers.AutoAcceptMedium, localityOverlap)
			return "auto_accept", topCandidate.UPRN
		}
	}

	// Review if score is above review threshold
	if topScore >= s.tiers.ReviewThreshold {
		debug.DebugOutput(localDebug, "Manual review: score %.4f >= %.4f but not auto-accept", topScore, s.tiers.ReviewThreshold)
		return "review", ""
	}

	// Otherwise reject
	debug.DebugOutput(localDebug, "Reject: score %.4f < review threshold %.4f", topScore, s.tiers.ReviewThreshold)
	return "reject", ""
}

// GetExplanation returns a human-readable explanation of the score
func (s *Scorer) GetExplanation(candidate Candidate, legacyUPRNValid bool) map[string]interface{} {
	explanation := make(map[string]interface{})
	
	// Core components
	trigramSim := s.getFloatFeature(candidate.Features, "trigram_similarity", 0.0)
	embeddingCos := s.getFloatFeature(candidate.Features, "embedding_cosine", 0.0)
	
	explanation["trigram_contribution"] = trigramSim * s.weights.TrigramSimilarity
	explanation["embedding_contribution"] = embeddingCos * s.weights.EmbeddingCosine
	
	// Token overlaps
	localityOverlap := s.getFloatFeature(candidate.Features, "locality_overlap_ratio", 0.0)
	streetOverlap := s.getFloatFeature(candidate.Features, "street_overlap_ratio", 0.0)
	
	explanation["locality_contribution"] = localityOverlap * s.weights.LocalityOverlap
	explanation["street_contribution"] = streetOverlap * s.weights.StreetOverlap
	
	// Boolean boosts
	var totalBoosts float64
	if s.getBoolFeature(candidate.Features, "has_same_house_num") {
		totalBoosts += s.weights.SameHouseNumber
	}
	if s.getBoolFeature(candidate.Features, "has_same_house_alpha") {
		totalBoosts += s.weights.SameHouseAlpha
	}
	if s.getBoolFeature(candidate.Features, "usrn_match") {
		totalBoosts += s.weights.USRNMatch
	}
	if s.getBoolFeature(candidate.Features, "llpg_live") {
		totalBoosts += s.weights.LLPGLive
	}
	if legacyUPRNValid {
		totalBoosts += s.weights.LegacyUPRNValid
	}
	explanation["boosts_total"] = totalBoosts
	
	// Spatial boost
	spatialBoost := s.getFloatFeature(candidate.Features, "spatial_boost", 0.0)
	explanation["spatial_contribution"] = spatialBoost
	
	// Penalties
	var totalPenalties float64
	if s.getBoolFeature(candidate.Features, "descriptor_penalty") {
		totalPenalties += s.weights.DescriptorPenalty
	}
	phoneticHits := s.getIntFeature(candidate.Features, "phonetic_hits", 0)
	if phoneticHits == 0 {
		totalPenalties += s.weights.PhoneticMissPenalty
	}
	explanation["penalties_total"] = totalPenalties
	
	explanation["final_score"] = candidate.Score
	explanation["methods"] = candidate.Methods
	
	return explanation
}

// Helper functions to safely extract typed values from features map
func (s *Scorer) getFloatFeature(features map[string]interface{}, key string, defaultVal float64) float64 {
	if val, exists := features[key]; exists {
		if f, ok := val.(float64); ok {
			return f
		}
		if i, ok := val.(int); ok {
			return float64(i)
		}
		if f, ok := val.(float32); ok {
			return float64(f)
		}
	}
	return defaultVal
}

func (s *Scorer) getBoolFeature(features map[string]interface{}, key string) bool {
	if val, exists := features[key]; exists {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

func (s *Scorer) getIntFeature(features map[string]interface{}, key string, defaultVal int) int {
	if val, exists := features[key]; exists {
		if i, ok := val.(int); ok {
			return i
		}
		if f, ok := val.(float64); ok {
			return int(f)
		}
	}
	return defaultVal
}