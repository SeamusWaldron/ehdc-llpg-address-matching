package normalize

import (
	"regexp"
	"strings"
)

// removePostcode removes postcode from address string
func removePostcode(address, postcode string) string {
	if postcode == "" {
		return address
	}
	
	// Remove the postcode (with and without spaces)
	result := strings.ReplaceAll(address, postcode, "")
	result = strings.ReplaceAll(result, strings.ReplaceAll(postcode, " ", ""), "")
	
	// Clean up any extra whitespace
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

// EnhancedCanonicalAddress provides improved address canonicalization
func EnhancedCanonicalAddress(rawAddress string) (canonical, postcode string, tokens []string) {
	// First extract postcode
	postcode = extractPostcode(rawAddress)
	addressWithoutPostcode := removePostcode(rawAddress, postcode)
	
	// Convert to uppercase
	canonical = strings.ToUpper(strings.TrimSpace(addressWithoutPostcode))
	
	// Apply comprehensive abbreviation expansions
	canonical = expandAbbreviations(canonical)
	
	// Remove noise words
	canonical = removeNoiseWords(canonical)
	
	// Normalize business names
	canonical = normalizeBusinessNames(canonical)
	
	// Handle special characters and punctuation
	canonical = cleanPunctuation(canonical)
	
	// Normalize multiple spaces
	canonical = regexp.MustCompile(`\s+`).ReplaceAllString(canonical, " ")
	canonical = strings.TrimSpace(canonical)
	
	// Generate tokens for matching
	tokens = strings.Fields(canonical)
	
	return canonical, postcode, tokens
}

// expandAbbreviations expands common UK address abbreviations
func expandAbbreviations(address string) string {
	// Comprehensive abbreviation map
	abbreviations := map[string]string{
		// Street types
		" RD ": " ROAD ",
		" RD$": " ROAD",
		"^RD ": "ROAD ",
		" ST ": " STREET ",
		" ST$": " STREET",
		"^ST ": "STREET ",
		" AVE ": " AVENUE ",
		" AVE$": " AVENUE",
		" CT ": " COURT ",
		" CT$": " COURT",
		" PL ": " PLACE ",
		" PL$": " PLACE",
		" DR ": " DRIVE ",
		" DR$": " DRIVE",
		" LN ": " LANE ",
		" LN$": " LANE",
		" GDNS ": " GARDENS ",
		" GDNS$": " GARDENS",
		" GRNS ": " GARDENS ",
		" GRN ": " GREEN ",
		" GRN$": " GREEN",
		" CLS ": " CLOSE ",
		" CL ": " CLOSE ",
		" CL$": " CLOSE",
		" CRES ": " CRESCENT ",
		" CRES$": " CRESCENT",
		" SQ ": " SQUARE ",
		" SQ$": " SQUARE",
		" TER ": " TERRACE ",
		" TER$": " TERRACE",
		" WLK ": " WALK ",
		" WK ": " WALK ",
		" WY ": " WAY ",
		" WY$": " WAY",
		" GRV ": " GROVE ",
		" GRV$": " GROVE",
		" PK ": " PARK ",
		" PK$": " PARK",
		" VW ": " VIEW ",
		" VW$": " VIEW",
		" HTS ": " HEIGHTS ",
		" HTS$": " HEIGHTS",
		" HL ": " HILL ",
		" HL$": " HILL",
		" PSGE ": " PASSAGE ",
		" YD ": " YARD ",
		" YD$": " YARD",
		" MS ": " MEWS ",
		" MS$": " MEWS",
		" EST ": " ESTATE ",
		" EST$": " ESTATE",
		" RIS ": " RISE ",
		" RIS$": " RISE",
		" PTH ": " PATH ",
		" PTH$": " PATH",
		
		// Compass directions
		" N ": " NORTH ",
		"^N ": "NORTH ",
		" S ": " SOUTH ",
		"^S ": "SOUTH ",
		" E ": " EAST ",
		"^E ": "EAST ",
		" W ": " WEST ",
		"^W ": "WEST ",
		" NE ": " NORTH EAST ",
		" NW ": " NORTH WEST ",
		" SE ": " SOUTH EAST ",
		" SW ": " SOUTH WEST ",
		
		// Common prefixes/suffixes
		" ST. ": " SAINT ",
		"^ST. ": "SAINT ",
		" MT ": " MOUNT ",
		"^MT ": "MOUNT ",
		" MT. ": " MOUNT ",
		"^MT. ": "MOUNT ",
		" FT ": " FORT ",
		"^FT ": "FORT ",
		" FT. ": " FORT ",
		"^FT. ": "FORT ",
		
		// Building types
		" BLDG ": " BUILDING ",
		" BLDGS ": " BUILDINGS ",
		" BLK ": " BLOCK ",
		" FLR ": " FLOOR ",
		" FL ": " FLAT ",
		" APT ": " APARTMENT ",
		" STE ": " SUITE ",
		" RM ": " ROOM ",
		" HSE ": " HOUSE ",
		" HO ": " HOUSE ",
		" COTT ": " COTTAGE ",
		" CTG ": " COTTAGE ",
		
		// Business/Landmark
		" CTR ": " CENTRE ",
		" CNTR ": " CENTRE ",
		" PO ": " POST OFFICE ",
		" P.O ": " POST OFFICE ",
		" IND ": " INDUSTRIAL ",
		" INDL ": " INDUSTRIAL ",
		" PH ": " PUBLIC HOUSE ",
		" P.H ": " PUBLIC HOUSE ",
		" CH ": " CHURCH ",
		" SCH ": " SCHOOL ",
		" HOSP ": " HOSPITAL ",
		" UNI ": " UNIVERSITY ",
		" STN ": " STATION ",
		" STA ": " STATION ",
		
		// Hampshire specific
		" HANTS ": " HAMPSHIRE ",
		" HANTS$": " HAMPSHIRE",
	}
	
	result := address
	for abbr, expansion := range abbreviations {
		if strings.HasPrefix(abbr, "^") {
			// Handle start of string
			pattern := regexp.MustCompile("^" + strings.TrimPrefix(abbr, "^"))
			result = pattern.ReplaceAllString(result, expansion)
		} else if strings.HasSuffix(abbr, "$") {
			// Handle end of string
			pattern := regexp.MustCompile(strings.TrimSuffix(abbr, "$") + "$")
			result = pattern.ReplaceAllString(result, expansion)
		} else {
			// Normal replacement
			result = strings.ReplaceAll(result, abbr, expansion)
		}
	}
	
	return result
}

// removeNoiseWords removes common words that don't help with matching
func removeNoiseWords(address string) string {
	noiseWords := []string{
		" THE ",
		"^THE ",
		" OF ",
		" NEAR ",
		" OPPOSITE ",
		" OPP ",
		" ADJ ",
		" ADJACENT ",
		" BEHIND ",
		" FRONT ",
		" REAR ",
		" SIDE ",
	}
	
	result := address
	for _, noise := range noiseWords {
		if strings.HasPrefix(noise, "^") {
			pattern := regexp.MustCompile("^" + strings.TrimPrefix(noise, "^"))
			result = pattern.ReplaceAllString(result, "")
		} else {
			result = strings.ReplaceAll(result, noise, " ")
		}
	}
	
	return result
}

// normalizeBusinessNames standardizes common business name variations
func normalizeBusinessNames(address string) string {
	businessNames := map[string]string{
		"CO-OP": "COOPERATIVE",
		"COOP": "COOPERATIVE",
		"CO OP": "COOPERATIVE",
		"TESCO'S": "TESCO",
		"SAINSBURY'S": "SAINSBURYS",
		"SAINSBURY": "SAINSBURYS",
		"MCDONALD'S": "MCDONALDS",
		"MARKS & SPENCER": "MARKS AND SPENCER",
		"M&S": "MARKS AND SPENCER",
		"B&Q": "B AND Q",
		"BARCLAYS BANK": "BARCLAYS",
		"LLOYDS BANK": "LLOYDS",
		"HSBC BANK": "HSBC",
		"NATWEST BANK": "NATWEST",
	}
	
	result := address
	for variant, standard := range businessNames {
		result = strings.ReplaceAll(result, variant, standard)
	}
	
	return result
}

// cleanPunctuation removes or normalizes punctuation
func cleanPunctuation(address string) string {
	// Remove apostrophes and quotes
	address = strings.ReplaceAll(address, "'", "")
	address = strings.ReplaceAll(address, "\"", "")
	address = strings.ReplaceAll(address, "`", "")
	
	// Replace hyphens and underscores with spaces
	address = strings.ReplaceAll(address, "-", " ")
	address = strings.ReplaceAll(address, "_", " ")
	
	// Remove other punctuation
	punctuation := []string{",", ".", ";", ":", "!", "?", "(", ")", "[", "]", "{", "}", "/", "\\"}
	for _, p := range punctuation {
		address = strings.ReplaceAll(address, p, " ")
	}
	
	// Handle ampersands
	address = strings.ReplaceAll(address, "&", " AND ")
	
	return address
}

// ExtractComponents extracts structured components from an address
type AddressComponents struct {
	HouseNumber  string
	HouseName    string
	StreetName   string
	Locality     string
	Town         string
	County       string
	Postcode     string
}

// ExtractAddressComponents attempts to parse address into components
func ExtractAddressComponents(address string) *AddressComponents {
	components := &AddressComponents{}
	
	// Extract postcode first
	components.Postcode = extractPostcode(address)
	addressWithoutPostcode := removePostcode(address, components.Postcode)
	
	// Extract house number
	houseNumPattern := regexp.MustCompile(`^(\d+[A-Z]?)\s+`)
	if matches := houseNumPattern.FindStringSubmatch(addressWithoutPostcode); len(matches) > 1 {
		components.HouseNumber = matches[1]
		addressWithoutPostcode = houseNumPattern.ReplaceAllString(addressWithoutPostcode, "")
	}
	
	// Known Hampshire towns
	hampshireTowns := []string{
		"ALTON", "PETERSFIELD", "LIPHOOK", "WATERLOOVILLE", "HORNDEAN",
		"FOUR MARKS", "BEECH", "ROPLEY", "ALRESFORD", "BORDON",
		"WHITEHILL", "GRAYSHOTT", "HINDHEAD", "HASLEMERE", "SHEET",
		"STEEP", "STROUD", "HAWKLEY", "SELBORNE", "EAST TISTED",
		"WEST TISTED", "CHAWTON", "HOLYBOURNE", "MEDSTEAD", "BENTLEY",
		"CATHERINGTON", "CLANFIELD", "DENMEAD", "HAMBLEDON", "ROWLANDS CASTLE",
		"SOBERTON", "WICKHAM", "DROXFORD", "EXTON", "MEONSTOKE",
	}
	
	// Try to identify town
	upperAddr := strings.ToUpper(addressWithoutPostcode)
	for _, town := range hampshireTowns {
		if strings.Contains(upperAddr, town) {
			components.Town = town
			break
		}
	}
	
	// Extract street type and name
	streetTypes := []string{
		"ROAD", "STREET", "AVENUE", "LANE", "DRIVE", "CLOSE",
		"COURT", "PLACE", "GARDENS", "GREEN", "CRESCENT", "TERRACE",
		"SQUARE", "HILL", "WAY", "PARK", "GROVE", "RISE", "WALK",
		"PATH", "MEWS", "YARD",
	}
	
	for _, streetType := range streetTypes {
		pattern := regexp.MustCompile(`(\w+(?:\s+\w+)*)\s+` + streetType)
		if matches := pattern.FindStringSubmatch(upperAddr); len(matches) > 1 {
			components.StreetName = matches[1] + " " + streetType
			break
		}
	}
	
	// County detection
	if strings.Contains(upperAddr, "HAMPSHIRE") || strings.Contains(upperAddr, "HANTS") {
		components.County = "HAMPSHIRE"
	}
	
	return components
}

// MatchByComponents provides component-based matching score
func MatchByComponents(source, target *AddressComponents) float64 {
	score := 0.0
	weights := 0.0
	
	// Postcode match (highest weight)
	if source.Postcode != "" && target.Postcode != "" {
		if source.Postcode == target.Postcode {
			score += 0.35
		} else if source.Postcode[:4] == target.Postcode[:4] { // Same sector
			score += 0.20
		} else if source.Postcode[:2] == target.Postcode[:2] { // Same area
			score += 0.10
		}
		weights += 0.35
	}
	
	// House number match
	if source.HouseNumber != "" && target.HouseNumber != "" {
		if source.HouseNumber == target.HouseNumber {
			score += 0.25
		}
		weights += 0.25
	}
	
	// Street name match
	if source.StreetName != "" && target.StreetName != "" {
		similarity := jaroWinklerSimilarity(source.StreetName, target.StreetName)
		score += 0.20 * similarity
		weights += 0.20
	}
	
	// Town match
	if source.Town != "" && target.Town != "" {
		if source.Town == target.Town {
			score += 0.15
		}
		weights += 0.15
	}
	
	// House name match
	if source.HouseName != "" && target.HouseName != "" {
		similarity := jaroWinklerSimilarity(source.HouseName, target.HouseName)
		score += 0.05 * similarity
		weights += 0.05
	}
	
	// Normalize by weights
	if weights > 0 {
		return score / weights
	}
	
	return 0.0
}

// jaroWinklerSimilarity calculates Jaro-Winkler similarity
func jaroWinklerSimilarity(s1, s2 string) float64 {
	// Simplified implementation
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}
	
	// Calculate basic overlap
	matches := 0
	for i := range s1 {
		if strings.Contains(s2, string(s1[i])) {
			matches++
		}
	}
	
	return float64(matches) / float64(max(len(s1), len(s2)))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// PartialStringMatch calculates similarity between two strings using token overlap
func PartialStringMatch(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	tokens1 := strings.Fields(strings.ToUpper(s1))
	tokens2 := strings.Fields(strings.ToUpper(s2))

	if len(tokens1) == 0 || len(tokens2) == 0 {
		return 0.0
	}

	// Count overlapping tokens
	matches := 0
	for _, t1 := range tokens1 {
		for _, t2 := range tokens2 {
			if t1 == t2 {
				matches++
				break
			}
		}
	}

	// Return Jaccard similarity
	totalTokens := len(tokens1) + len(tokens2) - matches
	if totalTokens == 0 {
		return 0.0
	}
	
	return float64(matches) / float64(totalTokens)
}