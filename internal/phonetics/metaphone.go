package phonetics

import (
	"strings"
)

// SimplePhonetics implements basic phonetic matching
type SimplePhonetics struct{}

// NewSimplePhonetics creates a simple phonetics matcher
func NewSimplePhonetics() *SimplePhonetics {
	return &SimplePhonetics{}
}

// GetMetaphone returns simplified phonetic codes
func (sp *SimplePhonetics) GetMetaphone(text string) (primary, secondary string) {
	// Simplified Double Metaphone implementation
	text = strings.ToUpper(strings.TrimSpace(text))
	if text == "" {
		return "", ""
	}
	
	// Basic phonetic transformations for common UK address terms
	replacements := map[string]string{
		"PH": "F",
		"GH": "F", 
		"CK": "K",
		"QU": "KW",
		"TH": "0", // Use 0 as theta sound
		"SH": "X",
		"CH": "X",
		"WH": "W",
		"KN": "N",
		"WR": "R",
	}
	
	result := text
	for pattern, replacement := range replacements {
		result = strings.ReplaceAll(result, pattern, replacement)
	}
	
	// Remove vowels except at start
	if len(result) > 1 {
		first := string(result[0])
		rest := result[1:]
		rest = strings.Map(func(r rune) rune {
			switch r {
			case 'A', 'E', 'I', 'O', 'U', 'Y':
				return -1 // Remove
			default:
				return r
			}
		}, rest)
		result = first + rest
	}
	
	// Remove duplicate consecutive letters
	var cleaned strings.Builder
	var lastChar rune
	for _, char := range result {
		if char != lastChar {
			cleaned.WriteRune(char)
			lastChar = char
		}
	}
	
	metaphone := cleaned.String()
	if len(metaphone) > 4 {
		metaphone = metaphone[:4] // Truncate to 4 chars
	}
	
	return metaphone, metaphone
}

// Match checks if two strings have similar phonetic representation
func (sp *SimplePhonetics) Match(text1, text2 string) bool {
	p1, _ := sp.GetMetaphone(text1)
	p2, _ := sp.GetMetaphone(text2)
	return p1 != "" && p2 != "" && p1 == p2
}