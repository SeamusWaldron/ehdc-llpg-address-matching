package match

import (
	"math"
	"strings"

	"github.com/ehdc-llpg/internal/debug"
	"github.com/ehdc-llpg/internal/normalize"
)

// FeatureComputer calculates rich features for address matching
type FeatureComputer struct {
	weights   *FeatureWeights
	embedder  Embedder
	phonetics PhoneticsMatcher
}

// PhoneticsMatcher interface for phonetic matching (Double Metaphone)
type PhoneticsMatcher interface {
	GetMetaphone(text string) (primary, secondary string)
	Match(text1, text2 string) bool
}

// NewFeatureComputer creates a new feature computer
func NewFeatureComputer(weights *FeatureWeights, embedder Embedder, phonetics PhoneticsMatcher) *FeatureComputer {
	return &FeatureComputer{
		weights:   weights,
		embedder:  embedder,
		phonetics: phonetics,
	}
}

// ComputeFeatures calculates comprehensive features for a source-candidate pair
func (fc *FeatureComputer) ComputeFeatures(localDebug bool, input Input, canonical string, tokens []string, candidate Candidate) map[string]interface{} {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	features := make(map[string]interface{})

	// Start with any existing features from candidate generation
	for k, v := range candidate.Features {
		features[k] = v
	}

	// Canonical forms for comparison
	candCanonical, _, candTokens := normalize.CanonicalAddress(candidate.LocAddress)
	debug.DebugOutput(localDebug, "Source canonical: %s", canonical)
	debug.DebugOutput(localDebug, "Candidate canonical: %s", candCanonical)

	// === String Similarities ===
	
	// Trigram similarity (may already exist from generator)
	if _, exists := features["trigram_similarity"]; !exists {
		features["trigram_similarity"] = fc.trigramSimilarity(canonical, candCanonical)
	}

	// Jaro similarity  
	features["jaro_similarity"] = JaroSimilarity(canonical, candCanonical)
	
	// Normalized Levenshtein distance (as similarity)
	features["levenshtein_similarity"] = 1.0 - fc.normalizedLevenshtein(canonical, candCanonical)

	// Cosine similarity on token bags
	features["cosine_bow"] = fc.cosineBagOfWords(tokens, candTokens)

	debug.DebugOutput(localDebug, "String similarities - Jaro: %.3f, Levenshtein: %.3f, Cosine BOW: %.3f", 
		features["jaro_similarity"], features["levenshtein_similarity"], features["cosine_bow"])

	// === Embedding Cosine Similarity ===
	if fc.embedder != nil {
		if embSim, err := fc.embeddingCosine(canonical, candCanonical); err == nil {
			features["embedding_cosine"] = embSim
			debug.DebugOutput(localDebug, "Embedding cosine: %.3f", embSim)
		} else {
			debug.DebugOutput(localDebug, "Embedding cosine failed: %v", err)
			features["embedding_cosine"] = 0.0
		}
	} else {
		features["embedding_cosine"] = 0.0
	}

	// === Token/Structure Features ===
	
	// House number matching
	srcHouseNums := normalize.ExtractHouseNumbers(input.RawAddress)
	candHouseNums := normalize.ExtractHouseNumbers(candidate.LocAddress)
	features["has_same_house_num"] = fc.hasCommonElement(srcHouseNums, candHouseNums)
	features["has_same_house_alpha"] = fc.hasCommonAlphaElement(srcHouseNums, candHouseNums)
	
	// Locality overlap
	srcLocalities := normalize.ExtractLocalityTokens(input.RawAddress)
	candLocalities := normalize.ExtractLocalityTokens(candidate.LocAddress)
	features["locality_overlap_ratio"] = fc.overlapRatio(srcLocalities, candLocalities)
	
	// Street token overlap
	srcStreetTokens := normalize.TokenizeStreet(input.RawAddress)
	candStreetTokens := normalize.TokenizeStreet(candidate.LocAddress)
	features["street_overlap_ratio"] = fc.overlapRatio(srcStreetTokens, candStreetTokens)

	// Descriptor penalty (if source has descriptors but candidate doesn't)
	features["descriptor_penalty"] = fc.hasDescriptorMismatch(input.RawAddress, candidate.LocAddress)

	debug.DebugOutput(localDebug, "Structural features - House: %v, Alpha: %v, Locality: %.3f, Street: %.3f", 
		features["has_same_house_num"], features["has_same_house_alpha"], 
		features["locality_overlap_ratio"], features["street_overlap_ratio"])

	// === Phonetic Features ===
	if fc.phonetics != nil {
		features["phonetic_hits"] = fc.countPhoneticMatches(tokens, candTokens)
	} else {
		features["phonetic_hits"] = 0
	}

	// === Spatial Features ===
	if input.Easting != nil && input.Northing != nil {
		distance := calculateDistance(*input.Easting, *input.Northing, candidate.Easting, candidate.Northing)
		features["distance_meters"] = distance
		features["spatial_boost"] = calculateSpatialBoost(distance)
		
		// Distance buckets for categorization
		features["distance_bucket"] = fc.distanceBucket(distance)
	} else {
		features["distance_meters"] = nil
		features["spatial_boost"] = 0.0
		features["distance_bucket"] = "unknown"
	}

	// === Meta Features ===
	
	// LLPG status (assume live for now - could be enhanced from lgcstatusc)
	features["llpg_live"] = true // TODO: Read from dim_address.lgcstatusc when available

	// BLPU class compatibility (basic heuristic)
	features["blpu_class_compat"] = fc.blpuClassCompatible(input.RawAddress, candidate.LocAddress)

	// USRN match (if USRN data available)
	features["usrn_match"] = false // TODO: Implement USRN matching when available

	// Legacy UPRN validation
	features["legacy_uprn_valid"] = (input.LegacyUPRN != "" && input.LegacyUPRN == candidate.UPRN)

	debug.DebugOutput(localDebug, "Meta features - LLPG Live: %v, Legacy UPRN: %v, BLPU Compat: %v", 
		features["llpg_live"], features["legacy_uprn_valid"], features["blpu_class_compat"])

	return features
}

// trigramSimilarity calculates trigram similarity (if not already computed)
func (fc *FeatureComputer) trigramSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if s1 == "" || s2 == "" {
		return 0.0
	}
	
	// Simple trigram similarity approximation
	// In production, this would use the same algorithm as PostgreSQL pg_trgm
	return JaroSimilarity(s1, s2) * 0.9 // Approximate conversion
}

// normalizedLevenshtein computes normalized Levenshtein distance
func (fc *FeatureComputer) normalizedLevenshtein(s1, s2 string) float64 {
	if s1 == s2 {
		return 0.0
	}
	if s1 == "" {
		return float64(len(s2))
	}
	if s2 == "" {
		return float64(len(s1))
	}
	
	distance := LevenshteinDistance(s1, s2)
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}
	
	return float64(distance) / float64(maxLen)
}

// cosineBagOfWords computes cosine similarity on token sets
func (fc *FeatureComputer) cosineBagOfWords(tokens1, tokens2 []string) float64 {
	if len(tokens1) == 0 && len(tokens2) == 0 {
		return 1.0
	}
	if len(tokens1) == 0 || len(tokens2) == 0 {
		return 0.0
	}
	
	// Build token frequency maps
	freq1 := make(map[string]int)
	freq2 := make(map[string]int)
	
	for _, token := range tokens1 {
		freq1[token]++
	}
	for _, token := range tokens2 {
		freq2[token]++
	}
	
	// Calculate cosine similarity
	var dotProduct, norm1, norm2 float64
	
	allTokens := make(map[string]bool)
	for token := range freq1 {
		allTokens[token] = true
	}
	for token := range freq2 {
		allTokens[token] = true
	}
	
	for token := range allTokens {
		f1 := float64(freq1[token])
		f2 := float64(freq2[token])
		
		dotProduct += f1 * f2
		norm1 += f1 * f1
		norm2 += f2 * f2
	}
	
	if norm1 == 0 || norm2 == 0 {
		return 0.0
	}
	
	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))
}

// embeddingCosine computes cosine similarity using embeddings
func (fc *FeatureComputer) embeddingCosine(s1, s2 string) (float64, error) {
	vec1, err := fc.embedder.Embed(s1)
	if err != nil {
		return 0.0, err
	}
	
	vec2, err := fc.embedder.Embed(s2)
	if err != nil {
		return 0.0, err
	}
	
	return CosineSimilarity(vec1, vec2), nil
}

// hasCommonElement checks if two slices share any common elements
func (fc *FeatureComputer) hasCommonElement(slice1, slice2 []string) bool {
	set1 := make(map[string]bool)
	for _, item := range slice1 {
		set1[strings.ToUpper(item)] = true
	}
	
	for _, item := range slice2 {
		if set1[strings.ToUpper(item)] {
			return true
		}
	}
	return false
}

// hasCommonAlphaElement checks for alpha suffixes in house numbers (12A, 12B)
func (fc *FeatureComputer) hasCommonAlphaElement(slice1, slice2 []string) bool {
	// Extract alpha suffixes from house numbers
	getAlphaSuffix := func(houseNum string) string {
		if len(houseNum) > 1 && houseNum[len(houseNum)-1] >= 'A' && houseNum[len(houseNum)-1] <= 'Z' {
			return string(houseNum[len(houseNum)-1])
		}
		return ""
	}
	
	for _, num1 := range slice1 {
		suffix1 := getAlphaSuffix(strings.ToUpper(num1))
		if suffix1 != "" {
			for _, num2 := range slice2 {
				suffix2 := getAlphaSuffix(strings.ToUpper(num2))
				if suffix1 == suffix2 {
					return true
				}
			}
		}
	}
	return false
}

// overlapRatio computes overlap ratio between two string slices
func (fc *FeatureComputer) overlapRatio(slice1, slice2 []string) float64 {
	return normalize.TokenOverlap(slice1, slice2)
}

// hasDescriptorMismatch checks if source has descriptors but candidate doesn't
func (fc *FeatureComputer) hasDescriptorMismatch(srcAddr, candAddr string) bool {
	descriptors := []string{"LAND AT", "REAR OF", "ADJACENT TO", "PLOT", "SITE"}
	
	srcUpper := strings.ToUpper(srcAddr)
	candUpper := strings.ToUpper(candAddr)
	
	hasSourceDescriptor := false
	for _, desc := range descriptors {
		if strings.Contains(srcUpper, desc) {
			hasSourceDescriptor = true
			break
		}
	}
	
	if !hasSourceDescriptor {
		return false // No penalty if source has no descriptors
	}
	
	// Source has descriptors - check if candidate has them too
	for _, desc := range descriptors {
		if strings.Contains(candUpper, desc) {
			return false // No penalty if candidate also has descriptors
		}
	}
	
	return true // Penalty: source has descriptors but candidate doesn't
}

// countPhoneticMatches counts phonetic matches between token sets
func (fc *FeatureComputer) countPhoneticMatches(tokens1, tokens2 []string) int {
	if fc.phonetics == nil {
		return 0
	}
	
	matches := 0
	for _, token1 := range tokens1 {
		for _, token2 := range tokens2 {
			if fc.phonetics.Match(token1, token2) {
				matches++
				break // Count each token1 match only once
			}
		}
	}
	return matches
}

// distanceBucket categorizes distance into buckets
func (fc *FeatureComputer) distanceBucket(distance float64) string {
	switch {
	case distance <= 100:
		return "0-100m"
	case distance <= 250:
		return "100-250m"
	case distance <= 500:
		return "250-500m"
	case distance <= 1000:
		return "500m-1km"
	case distance <= 2000:
		return "1-2km"
	default:
		return "2km+"
	}
}

// blpuClassCompatible checks basic BLPU class compatibility
func (fc *FeatureComputer) blpuClassCompatible(srcAddr, candAddr string) bool {
	// Basic heuristics for BLPU class compatibility
	// If source mentions specific property types, could check compatibility
	// For now, return true (compatible) - this could be enhanced with BLPU class data
	return true
}

// === Utility Functions ===

// JaroSimilarity computes Jaro similarity between two strings
func JaroSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	
	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}
	
	matchWindow := max(len1, len2)/2 - 1
	if matchWindow < 0 {
		matchWindow = 0
	}
	
	s1Matches := make([]bool, len1)
	s2Matches := make([]bool, len2)
	
	matches := 0
	transpositions := 0
	
	// Find matches
	for i := 0; i < len1; i++ {
		start := max(0, i-matchWindow)
		end := min(i+matchWindow+1, len2)
		
		for j := start; j < end; j++ {
			if s2Matches[j] || s1[i] != s2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}
	
	if matches == 0 {
		return 0.0
	}
	
	// Count transpositions
	k := 0
	for i := 0; i < len1; i++ {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}
	
	jaro := (float64(matches)/float64(len1) + 
			 float64(matches)/float64(len2) + 
			 float64(matches-transpositions/2)/float64(matches)) / 3.0
			 
	return jaro
}

// LevenshteinDistance computes Levenshtein distance between two strings
func LevenshteinDistance(s1, s2 string) int {
	if s1 == s2 {
		return 0
	}
	
	len1, len2 := len(s1), len(s2)
	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}
	
	// Create matrix
	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}
	
	// Fill matrix
	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}
			
			matrix[i][j] = min(
				min(matrix[i-1][j]+1, matrix[i][j-1]+1), // min of deletion and insertion
				matrix[i-1][j-1]+cost,                    // substitution
			)
		}
	}
	
	return matrix[len1][len2]
}

// CosineSimilarity computes cosine similarity between two vectors
func CosineSimilarity(vec1, vec2 []float32) float64 {
	if len(vec1) != len(vec2) {
		return 0.0
	}
	
	var dotProduct, norm1, norm2 float64
	
	for i := range vec1 {
		dotProduct += float64(vec1[i] * vec2[i])
		norm1 += float64(vec1[i] * vec1[i])
		norm2 += float64(vec2[i] * vec2[i])
	}
	
	if norm1 == 0 || norm2 == 0 {
		return 0.0
	}
	
	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}