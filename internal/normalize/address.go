package normalize

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/ehdc-llpg/internal/debug"
)

// ParseFloat converts string to float64, handling UK number formats
func ParseFloat(s string) (float64, error) {
	trimmed := strings.TrimSpace(s)
	return strconv.ParseFloat(trimmed, 64)
}

// AbbrevRules handles address abbreviation expansion
type AbbrevRules struct {
	rules map[string]string
}

// NewAbbrevRules creates abbreviation rules from database or defaults
func NewAbbrevRules() *AbbrevRules {
	// Default UK abbreviation rules - these should be loaded from address_normalise_rule table
	rules := map[string]string{
		`\bRD\b`:     "ROAD",
		`\bST\b`:     "STREET", // but keep SAINT separate
		`\bAVE\b`:    "AVENUE",
		`\bGDNS\b`:   "GARDENS",
		`\bCT\b`:     "COURT",
		`\bDR\b`:     "DRIVE",
		`\bLN\b`:     "LANE",
		`\bPL\b`:     "PLACE",
		`\bSQ\b`:     "SQUARE",
		`\bCRES\b`:   "CRESCENT",
		`\bTER\b`:    "TERRACE",
		`\bCL\b`:     "CLOSE",
		`\bPK\b`:     "PARK",
		`\bGRN\b`:    "GREEN",
		`\bWY\b`:     "WAY",
		`\bAPT\b`:    "APARTMENT",
		`\bFLT\b`:    "FLAT",
		`\bBLDG\b`:   "BUILDING",
		`\bHSE\b`:    "HOUSE",
		`\bCTG\b`:    "COTTAGE",
		`\bFM\b`:     "FARM",
		`\bMNR\b`:    "MANOR",
		`\bVIL\b`:    "VILLA",
		`\bEST\b`:    "ESTATE",
		`\bINDL\b`:   "INDUSTRIAL",
		`\bCTR\b`:    "CENTRE",
		`\bCENTRE\b`: "CENTRE", // normalize spelling
		`\bNTH\b`:    "NORTH",
		`\bSTH\b`:    "SOUTH",
		`\bE\b`:     "EAST",
		`\bWST\b`:    "WEST",
	}
	
	return &AbbrevRules{rules: rules}
}

// Expand applies abbreviation rules to text
func (ar *AbbrevRules) Expand(text string) string {
	result := text
	for pattern, replacement := range ar.rules {
		re := regexp.MustCompile(pattern)
		result = re.ReplaceAllString(result, replacement)
	}
	return result
}

// UK postcode regex - more comprehensive
var rePostcode = regexp.MustCompile(`\b([A-Za-z]{1,2}\d[\dA-Za-z]?\s*\d[ABD-HJLNP-UW-Zabd-hjlnp-uw-z]{2})\b`)

// House number patterns
var reHouseNumber = regexp.MustCompile(`\b(\d+[A-Za-z]?)\b`)

// Flat/unit patterns  
var reFlatUnit = regexp.MustCompile(`\b(FLAT|APT|APARTMENT|UNIT|STUDIO)\s+(\d+[A-Za-z]?)\b`)

// Locality tokens that should be preserved (common Hampshire towns)
var localityTokens = map[string]bool{
	"ALTON":         true,
	"PETERSFIELD":   true,
	"LIPHOOK":      true,
	"WATERLOOVILLE": true,
	"HORNDEAN":     true,
	"BORDON":       true,
	"WHITEHILL":    true,
	"GRAYSHOTT":    true,
	"HEADLEY":      true,
	"BRAMSHOTT":    true,
	"LINDFORD":     true,
	"HOLLYWATER":   true,
	"PASSFIELD":    true,
	"CONFORD":      true,
	"FOUR MARKS":   true,
	"MEDSTEAD":     true,
	"CHAWTON":      true,
	"SELBORNE":     true,
	"EMPSHOTT":     true,
	"HAWKLEY":      true,
	"LISS":         true,
	"STEEP":        true,
	"STROUD":       true,
	"BURITON":      true,
	"LANGRISH":     true,
	"EAST MEON":    true,
	"WEST MEON":    true,
	"FROXFIELD":    true,
	"PRIVETT":      true,
	"ROPLEY":       true,
	"WEST TISTED":  true,
	"EAST TISTED":  true,
	"BINSTED":      true,
	"HOLT POUND":   true,
	"BENTLEY":      true,
	"FARNHAM":      true, // Surrey border
	"HASLEMERE":    true, // Surrey border
}

// CanonicalAddress normalizes an address following UK-specific rules
// For backwards compatibility, this is the simple version
func CanonicalAddress(raw string) (addrCan, postcode string, tokens []string) {
	return CanonicalAddressDebug(false, raw)
}

// CanonicalAddressDebug normalizes an address with optional debug output
func CanonicalAddressDebug(localDebug bool, raw string) (addrCan, postcode string, tokens []string) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	if raw == "" {
		return "", "", []string{}
	}

	s := strings.ToUpper(strings.TrimSpace(raw))
	debug.DebugOutput(localDebug, "Input: %s", s)

	// Extract postcode first
	if m := rePostcode.FindString(s); m != "" {
		postcode = strings.ReplaceAll(m, " ", "")
		s = rePostcode.ReplaceAllString(s, " ")
		debug.DebugOutput(localDebug, "Extracted postcode: %s", postcode)
	}

	// Remove punctuation but preserve spaces
	b := strings.Builder{}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	s = strings.Join(strings.Fields(b.String()), " ")
	debug.DebugOutput(localDebug, "After punctuation removal: %s", s)

	// Expand abbreviations using rules
	rules := NewAbbrevRules()
	s = rules.Expand(s)
	debug.DebugOutput(localDebug, "After abbreviation expansion: %s", s)

	// Handle special UK descriptors
	s = handleDescriptors(s)
	debug.DebugOutput(localDebug, "After descriptor handling: %s", s)

	// Collapse spaces again
	s = strings.Join(strings.Fields(s), " ")

	tokens = strings.Fields(s)
	debug.DebugOutput(localDebug, "Final canonical: %s", s)
	debug.DebugOutput(localDebug, "Tokens: %v", tokens)
	
	return s, postcode, tokens
}

// handleDescriptors processes UK-specific address descriptors
func handleDescriptors(text string) string {
	// Common UK descriptors that should be normalized
	descriptorMap := map[string]string{
		"LAND AT":        "LAND AT",
		"LAND ADJ TO":    "LAND ADJACENT TO", 
		"LAND ADJACENT":  "LAND ADJACENT TO",
		"REAR OF":        "REAR OF",
		"PLOT":           "PLOT",
		"PARCEL":         "PARCEL",
		"SITE":           "SITE",
		"DEVELOPMENT":    "DEVELOPMENT",
		"PROPOSED":       "", // Remove "proposed" 
		"FORMER":         "", // Remove "former"
	}

	result := text
	for pattern, replacement := range descriptorMap {
		re := regexp.MustCompile(`\b` + pattern + `\b`)
		result = re.ReplaceAllString(result, replacement)
	}

	return strings.TrimSpace(result)
}

// ExtractHouseNumbers extracts house numbers and flat numbers from address
func ExtractHouseNumbers(text string) []string {
	var numbers []string
	
	// Find house numbers
	matches := reHouseNumber.FindAllString(text, -1)
	numbers = append(numbers, matches...)
	
	// Find flat/unit numbers
	flatMatches := reFlatUnit.FindAllStringSubmatch(text, -1)
	for _, match := range flatMatches {
		if len(match) > 2 {
			numbers = append(numbers, match[2]) // The number part
		}
	}
	
	return numbers
}

// ExtractLocalityTokens extracts known locality/town tokens from address
func ExtractLocalityTokens(text string) []string {
	var localities []string
	tokens := strings.Fields(strings.ToUpper(text))
	
	for _, token := range tokens {
		if localityTokens[token] {
			localities = append(localities, token)
		}
	}
	
	// Handle multi-word localities
	upperText := strings.ToUpper(text)
	for locality := range localityTokens {
		if strings.Contains(locality, " ") && strings.Contains(upperText, locality) {
			localities = append(localities, locality)
		}
	}
	
	return localities
}

// TokenizeStreet extracts street name tokens (excluding numbers, flats, localities)
func TokenizeStreet(text string) []string {
	tokens := strings.Fields(strings.ToUpper(text))
	var streetTokens []string
	
	skipWords := map[string]bool{
		"FLAT": true, "APT": true, "APARTMENT": true, "UNIT": true, "STUDIO": true,
		"THE": true, "AND": true, "OF": true, "AT": true, "IN": true, "ON": true,
		"LAND": true, "REAR": true, "ADJACENT": true, "TO": true, "PLOT": true,
		"SITE": true, "DEVELOPMENT": true, "PARCEL": true,
	}
	
	for _, token := range tokens {
		// Skip numbers
		if reHouseNumber.MatchString(token) {
			continue
		}
		// Skip localities
		if localityTokens[token] {
			continue
		}
		// Skip common words
		if skipWords[token] {
			continue
		}
		// Skip very short tokens
		if len(token) < 2 {
			continue
		}
		
		streetTokens = append(streetTokens, token)
	}
	
	return streetTokens
}

// IsBlank checks if an address is effectively blank after normalization
func IsBlank(addr string) bool {
	canonical, _, _ := CanonicalAddress(addr)
	return strings.TrimSpace(canonical) == ""
}

// TokenOverlap calculates overlap ratio between two token sets
func TokenOverlap(tokens1, tokens2 []string) float64 {
	if len(tokens1) == 0 && len(tokens2) == 0 {
		return 1.0
	}
	if len(tokens1) == 0 || len(tokens2) == 0 {
		return 0.0
	}
	
	set1 := make(map[string]bool)
	for _, token := range tokens1 {
		set1[token] = true
	}
	
	overlap := 0
	for _, token := range tokens2 {
		if set1[token] {
			overlap++
		}
	}
	
	// Return overlap as ratio of smaller set
	minLen := len(tokens1)
	if len(tokens2) < minLen {
		minLen = len(tokens2)
	}
	
	return float64(overlap) / float64(minLen)
}

// Legacy functions for compatibility with existing code
func ExtractTokens(canonical string) (houseNumbers, localities, streets []string) {
	return ExtractHouseNumbers(canonical), ExtractLocalityTokens(canonical), TokenizeStreet(canonical)
}

func extractPostcode(address string) string {
	_, postcode, _ := CanonicalAddress(address)
	return postcode
}

func isLikelyLocality(token string) bool {
	return localityTokens[token]
}

func isLikelyStreet(token string, allTokens []string) bool {
	streetIndicators := []string{
		"ROAD", "STREET", "AVENUE", "GARDENS", "COURT", "DRIVE",
		"LANE", "PLACE", "SQUARE", "CRESCENT", "TERRACE", "CLOSE",
		"PARK", "WAY", "GREEN", "HEIGHTS", "HILL", "VIEW", "GROVE",
	}
	
	for _, indicator := range streetIndicators {
		if token == indicator {
			return true
		}
	}
	return false
}