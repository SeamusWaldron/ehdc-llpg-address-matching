package validation

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// AddressParser handles UK-specific address parsing with enhanced validation
type AddressParser struct {
	config ParsingConfig
	
	// Compiled regex patterns for efficiency
	unitPattern     *regexp.Regexp
	flatPattern     *regexp.Regexp
	estatePattern   *regexp.Regexp
	postcodePattern *regexp.Regexp
	houseNumPattern *regexp.Regexp
}

// NewAddressParser creates a new parser with UK-specific configuration
func NewAddressParser() *AddressParser {
	config := DefaultParsingConfig()
	
	return &AddressParser{
		config:          config,
		unitPattern:     regexp.MustCompile(`(?i)\b(UNIT[,\s]+\d+[A-Z]?)\b`),
		flatPattern:     regexp.MustCompile(`(?i)\b(FLAT[,\s]+[A-Z0-9]+)\b`),
		estatePattern:   regexp.MustCompile(`(?i)\b(INDUSTRIAL\s+ESTATE?|IND\s+EST)\b`),
		postcodePattern: regexp.MustCompile(`(?i)\b([A-Z]{1,2}\d{1,2}[A-Z]?\s*\d[A-Z]{2})\b`),
		houseNumPattern: regexp.MustCompile(`(?i)^\s*(\d+[A-Z]?)\b`),
	}
}

// ParseAddress extracts structured components from a UK address string
func (p *AddressParser) ParseAddress(address string) AddressComponents {
	if address == "" {
		return AddressComponents{
			Raw:                address,
			ExtractionMethod:   "empty",
			ExtractionConfidence: 0.0,
			ParsedAt:           time.Now(),
			IsValidForMatching: false,
			ValidationIssues:   []string{"Empty address"},
		}
	}
	
	// Pre-process for UK-specific patterns
	cleaned := p.preprocessAddress(address)
	
	// Use gopostal for initial parsing
	components := p.parseWithGopostal(cleaned)
	
	// Post-process for UK-specific enhancements
	components = p.postprocessComponents(components, address)
	
	// Validate extraction quality
	components = p.validateExtraction(components)
	
	return components
}

// preprocessAddress handles UK-specific pre-processing before gopostal
func (p *AddressParser) preprocessAddress(address string) string {
	cleaned := strings.ToUpper(strings.TrimSpace(address))
	
	// Expand common UK abbreviations
	for abbrev, full := range p.config.StreetTypeAbbreviations {
		pattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(abbrev))
		re := regexp.MustCompile(pattern)
		cleaned = re.ReplaceAllString(cleaned, full)
	}
	
	// Expand county abbreviations
	for abbrev, full := range p.config.CountyAbbreviations {
		pattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(abbrev))
		re := regexp.MustCompile(pattern)
		cleaned = re.ReplaceAllString(cleaned, full)
	}
	
	// Normalize spacing
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")
	cleaned = strings.TrimSpace(cleaned)
	
	return cleaned
}

// parseWithGopostal performs the core address parsing using regex patterns
// TODO: Integrate with gopostal/libpostal when available
func (p *AddressParser) parseWithGopostal(address string) AddressComponents {
	components := AddressComponents{
		Raw:              address,
		ExtractionMethod: "regex_fallback",
		ParsedAt:        time.Now(),
	}
	
	// For now, use regex-based parsing as fallback
	// This can be replaced with actual gopostal integration later
	
	// Extract house number or unit/flat identifier
	upperAddr := strings.ToUpper(address)
	
	// Look for unit patterns first
	if unitMatch := p.unitPattern.FindString(upperAddr); unitMatch != "" {
		components.HouseNumber = p.normalizeUnitNumber(strings.TrimSpace(unitMatch))
	} else if flatMatch := p.flatPattern.FindString(upperAddr); flatMatch != "" {
		components.HouseNumber = p.normalizeFlatNumber(strings.TrimSpace(flatMatch))
	} else if houseMatch := p.houseNumPattern.FindStringSubmatch(address); len(houseMatch) > 1 {
		components.HouseNumber = strings.TrimSpace(houseMatch[1])
	}
	
	// Extract postcode
	if postcodeMatch := p.postcodePattern.FindString(address); postcodeMatch != "" {
		components.Postcode = strings.TrimSpace(postcodeMatch)
	}
	
	// Extract street name and locality more intelligently
	streetPart := address
	
	// Remove house number from start (case-insensitive)
	if components.HouseNumber != "" {
		upperHouse := strings.ToUpper(components.HouseNumber)
		upperStreet := strings.ToUpper(streetPart)
		if strings.HasPrefix(upperStreet, upperHouse) {
			streetPart = streetPart[len(components.HouseNumber):]
		}
		streetPart = strings.TrimPrefix(streetPart, ",")
		streetPart = strings.TrimSpace(streetPart)
	}
	
	// Remove postcode from end
	if components.Postcode != "" {
		streetPart = strings.TrimSuffix(streetPart, components.Postcode)
		streetPart = strings.TrimSuffix(streetPart, ",")
		streetPart = strings.TrimSpace(streetPart)
	}
	
	// Split by commas and extract street and locality
	parts := strings.Split(streetPart, ",")
	var streetParts []string
	var localityParts []string
	
	// Improved heuristic: check for street vs locality indicators
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		
		// Check if this part contains street indicators
		upperPart := strings.ToUpper(part)
		streetIndicators := []string{"ROAD", "STREET", "LANE", "AVENUE", "DRIVE", "CLOSE", "COURT", "PLACE", "WAY", "ESTATE", "INDUSTRIAL"}
		hasStreetIndicator := false
		for _, indicator := range streetIndicators {
			if strings.Contains(upperPart, indicator) {
				hasStreetIndicator = true
				break
			}
		}
		
		// If it has street indicators, it's part of the street
		// Otherwise, use original heuristic for locality
		if hasStreetIndicator {
			streetParts = append(streetParts, part)
		} else if i >= len(parts)-2 && (len(part) <= 15 || p.looksLikeLocality(part)) {
			localityParts = append(localityParts, part)
		} else {
			streetParts = append(streetParts, part)
		}
	}
	
	if len(streetParts) > 0 {
		components.Street = strings.Join(streetParts, ", ")
	}
	if len(localityParts) > 0 {
		components.Locality = strings.Join(localityParts, ", ")
	}
	
	return components
}

// postprocessComponents applies UK-specific enhancements to parsed components
func (p *AddressParser) postprocessComponents(components AddressComponents, original string) AddressComponents {
	// Handle UK-specific unit/flat patterns if gopostal missed them
	if components.HouseNumber == "" || components.SubBuilding == "" {
		if unitMatch := p.unitPattern.FindString(original); unitMatch != "" {
			if components.HouseNumber == "" {
				components.HouseNumber = strings.TrimSpace(unitMatch)
			} else if components.SubBuilding == "" {
				components.SubBuilding = strings.TrimSpace(unitMatch)
			}
		}
		
		if flatMatch := p.flatPattern.FindString(original); flatMatch != "" {
			if components.HouseNumber == "" {
				components.HouseNumber = strings.TrimSpace(flatMatch)
			} else if components.SubBuilding == "" {
				components.SubBuilding = strings.TrimSpace(flatMatch)
			}
		}
	}
	
	// Handle industrial estates specially
	if p.estatePattern.MatchString(original) {
		// Industrial estates often get misparsed - try to fix
		if strings.Contains(strings.ToUpper(components.Street), "INDUSTRIAL") {
			// Street is likely correct
		} else if strings.Contains(strings.ToUpper(original), "INDUSTRIAL") {
			// Try to extract the estate name
			parts := strings.Split(original, ",")
			for _, part := range parts {
				if p.estatePattern.MatchString(part) {
					components.Building = strings.TrimSpace(part)
					break
				}
			}
		}
	}
	
	// Extract postcode if not found by gopostal
	if components.Postcode == "" {
		if postcodeMatch := p.postcodePattern.FindString(original); postcodeMatch != "" {
			components.Postcode = strings.TrimSpace(postcodeMatch)
		}
	}
	
	// Try to extract house number from start of address if not found
	if components.HouseNumber == "" {
		if houseMatch := p.houseNumPattern.FindString(original); houseMatch != "" {
			components.HouseNumber = strings.TrimSpace(houseMatch)
		}
	}
	
	return components
}

// validateExtraction calculates confidence and identifies issues with the parsing
func (p *AddressParser) validateExtraction(components AddressComponents) AddressComponents {
	var issues []string
	var confidenceFactors []float64
	
	// House number validation
	if components.HouseNumber == "" {
		issues = append(issues, "No house number identified")
		confidenceFactors = append(confidenceFactors, 0.0)
	} else if p.isValidHouseNumber(components.HouseNumber) {
		confidenceFactors = append(confidenceFactors, 1.0)
	} else {
		issues = append(issues, fmt.Sprintf("Questionable house number: %s", components.HouseNumber))
		confidenceFactors = append(confidenceFactors, 0.5)
	}
	
	// Street validation
	if components.Street == "" {
		issues = append(issues, "No street name identified")
		confidenceFactors = append(confidenceFactors, 0.0)
	} else if len(components.Street) < 3 {
		issues = append(issues, "Street name too short")
		confidenceFactors = append(confidenceFactors, 0.3)
	} else {
		confidenceFactors = append(confidenceFactors, 1.0)
	}
	
	// Postcode validation
	if components.Postcode == "" {
		issues = append(issues, "No postcode identified")
		confidenceFactors = append(confidenceFactors, 0.0)
	} else if p.isValidUKPostcode(components.Postcode) {
		confidenceFactors = append(confidenceFactors, 1.0)
	} else {
		issues = append(issues, fmt.Sprintf("Invalid UK postcode format: %s", components.Postcode))
		confidenceFactors = append(confidenceFactors, 0.2)
	}
	
	// Locality validation
	if components.Locality == "" {
		issues = append(issues, "No locality identified")
		confidenceFactors = append(confidenceFactors, 0.5) // Not critical
	} else {
		confidenceFactors = append(confidenceFactors, 1.0)
	}
	
	// Calculate overall confidence
	if len(confidenceFactors) > 0 {
		sum := 0.0
		for _, factor := range confidenceFactors {
			sum += factor
		}
		components.ExtractionConfidence = sum / float64(len(confidenceFactors))
	}
	
	// Set validation results
	components.ValidationIssues = issues
	components.IsValidForMatching = components.ExtractionConfidence >= p.config.MinOverallConfidence && 
		components.HasHouseNumber() && components.HasStreet()
	
	return components
}

// isValidHouseNumber checks if a house number string looks valid
func (p *AddressParser) isValidHouseNumber(houseNum string) bool {
	if houseNum == "" {
		return false
	}
	
	// Must start with a digit or contain recognizable patterns
	patterns := []string{
		`^\d+[A-Z]?$`,                    // "123", "45A"
		`^(?i)UNIT\s+\d+[A-Z]?$`,        // "Unit 2", "UNIT 5A"
		`^(?i)FLAT\s+[A-Z0-9]+$`,        // "Flat A", "FLAT 12"
		`^(?i)SUITE\s+\d+[A-Z]?$`,       // "Suite 1", "SUITE 10B"
		`^\d+[A-Z]?[-/]\d+[A-Z]?$`,      // "12-14", "5A/B"
	}
	
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, houseNum); matched {
			return true
		}
	}
	
	return false
}

// isValidUKPostcode validates UK postcode format
func (p *AddressParser) isValidUKPostcode(postcode string) bool {
	if postcode == "" {
		return false
	}
	
	// UK postcode patterns: M1 1AA, M60 1NW, CR0 2YR, DN55 1PT, W1A 0AX, EC1A 1BB
	ukPostcodePattern := `^[A-Z]{1,2}\d{1,2}[A-Z]?\s*\d[A-Z]{2}$`
	matched, _ := regexp.MatchString(ukPostcodePattern, strings.ToUpper(strings.TrimSpace(postcode)))
	return matched
}

// ValidateAddressForMatching performs comprehensive validation of address suitability
func (p *AddressParser) ValidateAddressForMatching(address string) AddressValidation {
	components := p.ParseAddress(address)
	
	validation := AddressValidation{
		Address:    address,
		Components: components,
		Issues:     components.ValidationIssues,
		Suitable:   components.IsValidForMatching,
		Score:      components.ExtractionConfidence,
	}
	
	// Additional matching-specific validations
	if !components.HasHouseNumber() {
		validation.Issues = append(validation.Issues, "Missing house number - required for precise matching")
		validation.Suitable = false
	}
	
	if !components.HasStreet() {
		validation.Issues = append(validation.Issues, "Missing or invalid street name")
		validation.Suitable = false
	}
	
	// Check for vague addresses that shouldn't be auto-matched
	vaguePhrases := []string{
		"LAND AT", "SITE OF", "REAR OF", "ADJACENT TO", "ADJOINING",
		"NORTH OF", "SOUTH OF", "EAST OF", "WEST OF",
	}
	
	upperAddr := strings.ToUpper(address)
	for _, phrase := range vaguePhrases {
		if strings.Contains(upperAddr, phrase) {
			validation.Issues = append(validation.Issues, fmt.Sprintf("Vague address contains '%s'", phrase))
			validation.Suitable = false
			validation.Score *= 0.5 // Reduce confidence
			break
		}
	}
	
	return validation
}

// NormalizeAddressComponents applies consistent formatting to components
func (p *AddressParser) NormalizeAddressComponents(components AddressComponents) AddressComponents {
	normalized := components
	
	// Normalize house number format
	if normalized.HouseNumber != "" {
		normalized.HouseNumber = strings.ToUpper(strings.TrimSpace(normalized.HouseNumber))
		normalized.HouseNumber = regexp.MustCompile(`\s+`).ReplaceAllString(normalized.HouseNumber, " ")
	}
	
	// Normalize street name
	if normalized.Street != "" {
		normalized.Street = p.normalizeStreetName(normalized.Street)
	}
	
	// Normalize postcode
	if normalized.Postcode != "" {
		normalized.Postcode = p.normalizePostcode(normalized.Postcode)
	}
	
	// Normalize locality
	if normalized.Locality != "" {
		normalized.Locality = strings.Title(strings.ToLower(strings.TrimSpace(normalized.Locality)))
	}
	
	return normalized
}

// normalizeStreetName applies consistent street name formatting
func (p *AddressParser) normalizeStreetName(street string) string {
	normalized := strings.ToUpper(strings.TrimSpace(street))
	
	// Remove unit/flat identifiers from street names for comparison
	// Patterns like "UNIT, 2", "FLAT A", etc. should be removed from street names
	unitRemovePatterns := []string{
		`(?i)\bUNIT[,\s]+\d+[A-Z]?\b[,\s]*`,
		`(?i)\bFLAT[,\s]+[A-Z0-9]+\b[,\s]*`,
		`(?i)\bSUITE[,\s]+\d+[A-Z]?\b[,\s]*`,
	}
	
	for _, pattern := range unitRemovePatterns {
		re := regexp.MustCompile(pattern)
		normalized = re.ReplaceAllString(normalized, "")
	}
	
	// Apply abbreviation expansions
	for abbrev, full := range p.config.StreetTypeAbbreviations {
		pattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(abbrev))
		re := regexp.MustCompile(pattern)
		normalized = re.ReplaceAllString(normalized, full)
	}
	
	// Normalize spacing and remove extra commas
	normalized = regexp.MustCompile(`\s*,\s*`).ReplaceAllString(normalized, ", ")
	normalized = regexp.MustCompile(`\s+`).ReplaceAllString(normalized, " ")
	normalized = regexp.MustCompile(`^[,\s]+|[,\s]+$`).ReplaceAllString(normalized, "")
	
	return strings.TrimSpace(normalized)
}

// normalizePostcode applies consistent postcode formatting
func (p *AddressParser) normalizePostcode(postcode string) string {
	normalized := strings.ToUpper(strings.TrimSpace(postcode))
	
	// Remove internal spaces and re-add correctly
	normalized = strings.ReplaceAll(normalized, " ", "")
	
	// Add space before final 3 characters (UK standard)
	if len(normalized) >= 5 {
		spacePos := len(normalized) - 3
		normalized = normalized[:spacePos] + " " + normalized[spacePos:]
	}
	
	return normalized
}

// looksLikeLocality uses heuristics to identify locality names
func (p *AddressParser) looksLikeLocality(part string) bool {
	upper := strings.ToUpper(part)
	
	// Common UK place name patterns
	localityIndicators := []string{
		"ALTON", "LISS", "PETERSFIELD", "BORDON", "GRAYSHOTT", "HEADLEY",
		"WATERLOOVILLE", "HORNDEAN", "HAMPSHIRE", "HANTS",
		// Common suffixes
		"FIELD", "FORD", "TON", "HAM", "BURY", "WORTH", "STEAD",
	}
	
	for _, indicator := range localityIndicators {
		if strings.Contains(upper, indicator) {
			return true
		}
	}
	
	// If it's a single word and reasonably short, likely a locality
	if !strings.Contains(part, " ") && len(part) <= 12 {
		return true
	}
	
	return false
}

// normalizeUnitNumber extracts and normalizes unit numbers from patterns like "UNIT, 2" or "UNIT 2"
func (p *AddressParser) normalizeUnitNumber(unitMatch string) string {
	// Extract just the number part from "UNIT, 2" or "UNIT 2"
	numberPattern := regexp.MustCompile(`(\d+[A-Z]?)`)
	if match := numberPattern.FindString(unitMatch); match != "" {
		return match
	}
	return unitMatch // fallback to original if no number found
}

// normalizeFlatNumber extracts and normalizes flat numbers from patterns like "FLAT, A" or "FLAT A"
func (p *AddressParser) normalizeFlatNumber(flatMatch string) string {
	// Extract just the identifier part from "FLAT, A" or "FLAT 12"
	idPattern := regexp.MustCompile(`([A-Z0-9]+)`)
	matches := idPattern.FindAllString(flatMatch, -1)
	if len(matches) > 1 {
		return matches[1] // Return the second match (the identifier, not "FLAT")
	}
	return flatMatch // fallback to original if no identifier found
}