package validation

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AddressValidator handles component-level validation and matching decisions
type AddressValidator struct {
	parser     *AddressParser
	thresholds MatchingThresholds
}

// NewAddressValidator creates a validator with conservative thresholds
func NewAddressValidator() *AddressValidator {
	return &AddressValidator{
		parser:     NewAddressParser(),
		thresholds: DefaultMatchingThresholds(),
	}
}

// ValidateHouseNumbers performs strict house number validation between two addresses
func (v *AddressValidator) ValidateHouseNumbers(source, target AddressComponents) ValidationResult {
	sourceNum := v.normalizeHouseNumber(source.HouseNumber)
	targetNum := v.normalizeHouseNumber(target.HouseNumber)
	
	if sourceNum == "" || targetNum == "" {
		return ValidationResult{
			Valid:      false,
			Confidence: 0.0,
			Reason:     fmt.Sprintf("Missing house number: source='%s', target='%s'", sourceNum, targetNum),
		}
	}
	
	// Exact match (case-insensitive)
	if strings.EqualFold(sourceNum, targetNum) {
		return ValidationResult{
			Valid:      true,
			Confidence: 1.0,
			Reason:     fmt.Sprintf("Exact house number match: '%s'", sourceNum),
		}
	}
	
	// Handle common UK variations
	variations := v.generateHouseNumberVariations(sourceNum)
	for _, variation := range variations {
		if strings.EqualFold(variation, targetNum) {
			return ValidationResult{
				Valid:      true,
				Confidence: 0.95,
				Reason:     fmt.Sprintf("House number variation match: '%s' ≈ '%s'", sourceNum, targetNum),
				Details: map[string]interface{}{
					"variation_type": "punctuation_spacing",
				},
			}
		}
	}
	
	// Check for numeric proximity (might indicate data entry error)
	sourceDigits := v.extractDigits(sourceNum)
	targetDigits := v.extractDigits(targetNum)
	
	if sourceDigits != "" && targetDigits != "" {
		sourceInt, sourceErr := strconv.Atoi(sourceDigits)
		targetInt, targetErr := strconv.Atoi(targetDigits)
		
		if sourceErr == nil && targetErr == nil {
			diff := abs(sourceInt - targetInt)
			if diff <= 2 && diff > 0 {
				// Close numbers - likely data quality issue, flag for review
				return ValidationResult{
					Valid:      false,
					Confidence: 0.0,
					Reason:     fmt.Sprintf("House number mismatch with proximity concern: %s vs %s (diff: %d)", sourceNum, targetNum, diff),
					Details: map[string]interface{}{
						"numeric_difference": diff,
						"requires_review":    true,
						"issue_type":        "potential_data_error",
					},
				}
			}
		}
	}
	
	// Different house numbers - automatic rejection
	return ValidationResult{
		Valid:      false,
		Confidence: 0.0,
		Reason:     fmt.Sprintf("House number mismatch: '%s' ≠ '%s'", sourceNum, targetNum),
		Details: map[string]interface{}{
			"mismatch_type": "different_identifiers",
		},
	}
}

// ValidateStreetNames performs fuzzy validation of street names
func (v *AddressValidator) ValidateStreetNames(source, target AddressComponents) ValidationResult {
	sourceStreet := v.parser.normalizeStreetName(source.Street)
	targetStreet := v.parser.normalizeStreetName(target.Street)
	
	if sourceStreet == "" || targetStreet == "" {
		return ValidationResult{
			Valid:      false,
			Confidence: 0.0,
			Reason:     "Missing street name",
		}
	}
	
	// Exact match
	if sourceStreet == targetStreet {
		return ValidationResult{
			Valid:      true,
			Confidence: 1.0,
			Reason:     fmt.Sprintf("Exact street match: '%s'", sourceStreet),
		}
	}
	
	// Calculate string similarity
	similarity := v.calculateStringSimilarity(sourceStreet, targetStreet)
	
	if similarity >= v.thresholds.StreetSimilarity {
		return ValidationResult{
			Valid:      true,
			Confidence: similarity,
			Reason:     fmt.Sprintf("High street similarity: '%s' ≈ '%s' (%.2f)", sourceStreet, targetStreet, similarity),
			Details: map[string]interface{}{
				"similarity_score": similarity,
			},
		}
	}
	
	// Check for common abbreviation issues
	if v.isAbbreviationMatch(sourceStreet, targetStreet) {
		return ValidationResult{
			Valid:      true,
			Confidence: 0.9,
			Reason:     fmt.Sprintf("Street abbreviation match: '%s' ≈ '%s'", sourceStreet, targetStreet),
			Details: map[string]interface{}{
				"match_type": "abbreviation",
			},
		}
	}
	
	return ValidationResult{
		Valid:      false,
		Confidence: similarity,
		Reason:     fmt.Sprintf("Insufficient street similarity: '%s' vs '%s' (%.2f < %.2f)", sourceStreet, targetStreet, similarity, v.thresholds.StreetSimilarity),
	}
}

// ValidatePostcodes performs postcode validation with district-level flexibility
func (v *AddressValidator) ValidatePostcodes(source, target AddressComponents) ValidationResult {
	sourcePostcode := v.parser.normalizePostcode(source.Postcode)
	targetPostcode := v.parser.normalizePostcode(target.Postcode)
	
	if sourcePostcode == "" || targetPostcode == "" {
		return ValidationResult{
			Valid:      true, // Postcodes are helpful but not mandatory
			Confidence: 0.8,
			Reason:     "Missing postcode - acceptable for matching",
		}
	}
	
	// Exact match
	if sourcePostcode == targetPostcode {
		return ValidationResult{
			Valid:      true,
			Confidence: 1.0,
			Reason:     fmt.Sprintf("Exact postcode match: '%s'", sourcePostcode),
		}
	}
	
	// Same postcode district (e.g., GU34 2QG ≈ GU34 2QF)
	sourceDistrict := v.extractPostcodeDistrict(sourcePostcode)
	targetDistrict := v.extractPostcodeDistrict(targetPostcode)
	
	if sourceDistrict == targetDistrict && v.thresholds.AllowPostcodeDistrict {
		return ValidationResult{
			Valid:      true,
			Confidence: 0.9,
			Reason:     fmt.Sprintf("Same postcode district: '%s' ≈ '%s'", sourcePostcode, targetPostcode),
			Details: map[string]interface{}{
				"district":   sourceDistrict,
				"match_type": "district_level",
			},
		}
	}
	
	// Calculate similarity for different districts
	similarity := v.calculateStringSimilarity(sourcePostcode, targetPostcode)
	
	return ValidationResult{
		Valid:      false,
		Confidence: similarity,
		Reason:     fmt.Sprintf("Postcode mismatch: '%s' vs '%s' (similarity: %.2f)", sourcePostcode, targetPostcode, similarity),
		Details: map[string]interface{}{
			"source_district": sourceDistrict,
			"target_district": targetDistrict,
			"similarity":      similarity,
		},
	}
}

// MakeMatchDecision performs comprehensive address matching with conservative thresholds
func (v *AddressValidator) MakeMatchDecision(sourceAddr, targetAddr string) MatchDecision {
	decision := MatchDecision{
		DecisionTime: time.Now(),
		Version:      "conservative-v1.0",
	}
	
	// Parse both addresses
	sourceComponents := v.parser.ParseAddress(sourceAddr)
	targetComponents := v.parser.ParseAddress(targetAddr)
	
	// Validate both addresses are suitable for matching
	sourceValidation := v.parser.ValidateAddressForMatching(sourceAddr)
	targetValidation := v.parser.ValidateAddressForMatching(targetAddr)
	
	if !sourceValidation.Suitable || !targetValidation.Suitable {
		decision.Accept = false
		decision.Confidence = 0.0
		decision.Method = "Pre-validation Failed"
		decision.Reason = "Address components could not be reliably extracted"
		decision.RequiresReview = true
		return decision
	}
	
	// Perform component-level validation
	houseValidation := v.ValidateHouseNumbers(sourceComponents, targetComponents)
	streetValidation := v.ValidateStreetNames(sourceComponents, targetComponents)
	postcodeValidation := v.ValidatePostcodes(sourceComponents, targetComponents)
	
	decision.ComponentValidation = ComponentValidation{
		HouseNumberMatch: houseValidation,
		StreetMatch:      streetValidation,
		PostcodeMatch:    postcodeValidation,
	}
	
	// House number validation is mandatory
	if !houseValidation.Valid {
		decision.Accept = false
		decision.Confidence = 0.0
		decision.Method = "House Number Mismatch"
		decision.Reason = houseValidation.Reason
		decision.RequiresReview = shouldReview(houseValidation)
		return decision
	}
	
	// Street validation is mandatory
	if !streetValidation.Valid {
		decision.Accept = false
		decision.Confidence = streetValidation.Confidence
		decision.Method = "Street Validation Failed"
		decision.Reason = streetValidation.Reason
		decision.RequiresReview = streetValidation.Confidence >= v.thresholds.MinReviewConfidence
		return decision
	}
	
	// Calculate overall confidence
	overallConfidence := v.calculateOverallConfidence(houseValidation, streetValidation, postcodeValidation)
	decision.ComponentValidation.OverallScore = overallConfidence
	decision.Confidence = overallConfidence
	
	// Make final decision based on confidence
	if overallConfidence >= v.thresholds.MinAutoAcceptConfidence {
		decision.Accept = true
		decision.Method = "Conservative Auto-Match"
		decision.Reason = fmt.Sprintf("All component validations passed with high confidence (%.2f)", overallConfidence)
		decision.RequiresReview = false
	} else if overallConfidence >= v.thresholds.MinReviewConfidence {
		decision.Accept = false
		decision.Method = "Manual Review Required"
		decision.Reason = fmt.Sprintf("Good match quality but below auto-accept threshold (%.2f < %.2f)", overallConfidence, v.thresholds.MinAutoAcceptConfidence)
		decision.RequiresReview = true
	} else {
		decision.Accept = false
		decision.Method = "Rejected - Insufficient Quality"
		decision.Reason = fmt.Sprintf("Match quality below acceptable thresholds (%.2f < %.2f)", overallConfidence, v.thresholds.MinReviewConfidence)
		decision.RequiresReview = false
	}
	
	return decision
}

// Helper functions

// normalizeHouseNumber standardizes house number format for comparison
func (v *AddressValidator) normalizeHouseNumber(houseNum string) string {
	if houseNum == "" {
		return ""
	}
	
	normalized := strings.ToUpper(strings.TrimSpace(houseNum))
	
	// Normalize spacing in unit numbers
	normalized = strings.ReplaceAll(normalized, ", ", " ")
	normalized = strings.ReplaceAll(normalized, ",", " ")
	
	// Normalize multiple spaces
	normalized = strings.Join(strings.Fields(normalized), " ")
	
	return normalized
}

// generateHouseNumberVariations creates common variations of a house number
func (v *AddressValidator) generateHouseNumberVariations(houseNum string) []string {
	variations := []string{}
	
	// Punctuation variations
	variations = append(variations, strings.ReplaceAll(houseNum, ",", ""))
	variations = append(variations, strings.ReplaceAll(houseNum, " ", ""))
	variations = append(variations, strings.ReplaceAll(houseNum, ", ", " "))
	
	// Unit formatting variations
	if strings.Contains(strings.ToUpper(houseNum), "UNIT") {
		withComma := strings.ReplaceAll(strings.ToUpper(houseNum), "UNIT ", "UNIT, ")
		withoutComma := strings.ReplaceAll(strings.ToUpper(houseNum), "UNIT, ", "UNIT ")
		variations = append(variations, withComma, withoutComma)
	}
	
	return variations
}

// extractDigits extracts numeric digits from a house number string
func (v *AddressValidator) extractDigits(str string) string {
	digits := ""
	for _, char := range str {
		if char >= '0' && char <= '9' {
			digits += string(char)
		}
	}
	return digits
}

// calculateStringSimilarity computes similarity between two strings
func (v *AddressValidator) calculateStringSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	
	// Simple Levenshtein-based similarity (placeholder)
	// In production, this would use a proper similarity algorithm
	maxLen := max(len(s1), len(s2))
	if maxLen == 0 {
		return 1.0
	}
	
	editDistance := levenshteinDistance(s1, s2)
	return 1.0 - (float64(editDistance) / float64(maxLen))
}

// isAbbreviationMatch checks if two strings are abbreviation variants
func (v *AddressValidator) isAbbreviationMatch(s1, s2 string) bool {
	// Check if one is an abbreviated form of the other
	// This is a simplified implementation
	shorter, longer := s1, s2
	if len(s1) > len(s2) {
		shorter, longer = s2, s1
	}
	
	// If one is significantly shorter and contained in the other
	if float64(len(shorter)) < float64(len(longer))*0.7 && strings.Contains(longer, shorter) {
		return true
	}
	
	return false
}

// extractPostcodeDistrict extracts the district part of a UK postcode
func (v *AddressValidator) extractPostcodeDistrict(postcode string) string {
	parts := strings.Fields(postcode)
	if len(parts) > 0 {
		return parts[0] // Return the first part (e.g., "GU34" from "GU34 2QG")
	}
	return postcode
}

// calculateOverallConfidence combines component confidences into overall score
func (v *AddressValidator) calculateOverallConfidence(house, street, postcode ValidationResult) float64 {
	// House number is critical (weight: 0.5)
	// Street is critical (weight: 0.4)
	// Postcode is helpful (weight: 0.1)
	
	weights := map[string]float64{
		"house":    0.5,
		"street":   0.4,
		"postcode": 0.1,
	}
	
	score := house.Confidence*weights["house"] +
		street.Confidence*weights["street"] +
		postcode.Confidence*weights["postcode"]
	
	return score
}

// shouldReview determines if a validation result should trigger manual review
func shouldReview(validation ValidationResult) bool {
	if details, ok := validation.Details["requires_review"]; ok {
		if requiresReview, ok := details.(bool); ok {
			return requiresReview
		}
	}
	return false
}

// Utility functions
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// levenshteinDistance calculates edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}
	
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}
	
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}
	
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			if s1[i-1] == s2[j-1] {
				matrix[i][j] = matrix[i-1][j-1]
			} else {
				matrix[i][j] = min3(
					matrix[i-1][j]+1,   // deletion
					matrix[i][j-1]+1,   // insertion
					matrix[i-1][j-1]+1, // substitution
				)
			}
		}
	}
	
	return matrix[len(s1)][len(s2)]
}

func min3(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}