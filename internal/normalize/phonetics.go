package normalize

import (
	"strings"
)

// DoubleMetaphone provides simplified phonetic matching for UK place names
// This is a basic implementation focusing on common UK address issues
type DoubleMetaphone struct {
	// Common phonetic substitutions for UK addresses
	substitutions map[string]string
}

// NewDoubleMetaphone creates a new phonetic matcher
func NewDoubleMetaphone() *DoubleMetaphone {
	return &DoubleMetaphone{
		substitutions: map[string]string{
			// Common UK place name phonetic issues
			"PH": "F",
			"GH": "F",
			"CK": "K",
			"QU": "KW", 
			"C":  "K", // Before E, I, Y
			"G":  "J", // Before E, I, Y
			"Y":  "I",
			"Z":  "S",
			// Double letters
			"LL": "L",
			"SS": "S",
			"NN": "N",
			"MM": "M",
			"RR": "R",
			// Silent letters
			"KN": "N",
			"WR": "R",
			"PS": "S",
		},
	}
}

// Encode returns a phonetic code for the given string
func (dm *DoubleMetaphone) Encode(s string) string {
	if s == "" {
		return ""
	}
	
	s = strings.ToUpper(strings.TrimSpace(s))
	result := strings.Builder{}
	
	// Simple phonetic encoding focusing on consonants
	for i := 0; i < len(s); i++ {
		char := string(s[i])
		
		// Handle two-character combinations first
		if i < len(s)-1 {
			twoChar := string(s[i]) + string(s[i+1])
			if replacement, exists := dm.substitutions[twoChar]; exists {
				result.WriteString(replacement)
				i++ // Skip next character as it's part of this combination
				continue
			}
		}
		
		// Handle single characters
		if replacement, exists := dm.substitutions[char]; exists {
			result.WriteString(replacement)
		} else if isConsonant(char) {
			result.WriteString(char)
		}
		// Skip vowels except at the beginning
		if i == 0 && isVowel(char) {
			result.WriteString(char)
		}
	}
	
	// Limit to reasonable length and remove duplicates
	code := result.String()
	if len(code) > 6 {
		code = code[:6]
	}
	
	return removeDuplicateChars(code)
}

// PhoneticMatch checks if two strings are phonetically similar
func (dm *DoubleMetaphone) PhoneticMatch(s1, s2 string) bool {
	if s1 == "" || s2 == "" {
		return false
	}
	
	code1 := dm.Encode(s1)
	code2 := dm.Encode(s2)
	
	return code1 == code2
}

// GetPhoneticTokens returns phonetic codes for all significant tokens in an address
func GetPhoneticTokens(address string) []string {
	dm := NewDoubleMetaphone()
	tokens := strings.Fields(strings.ToUpper(address))
	
	var phoneticTokens []string
	for _, token := range tokens {
		// Skip very short tokens and numbers
		if len(token) <= 2 || isNumeric(token) {
			continue
		}
		
		// Skip common words that aren't useful for phonetic matching
		if isCommonWord(token) {
			continue
		}
		
		phoneticCode := dm.Encode(token)
		if phoneticCode != "" {
			phoneticTokens = append(phoneticTokens, phoneticCode)
		}
	}
	
	return phoneticTokens
}

// PhoneticTokenOverlap counts phonetic matches between two address strings
func PhoneticTokenOverlap(addr1, addr2 string) int {
	tokens1 := GetPhoneticTokens(addr1)
	tokens2 := GetPhoneticTokens(addr2)
	
	matches := 0
	for _, t1 := range tokens1 {
		for _, t2 := range tokens2 {
			if t1 == t2 {
				matches++
				break // Count each token only once
			}
		}
	}
	
	return matches
}

// Helper functions

func isConsonant(s string) bool {
	if len(s) != 1 {
		return false
	}
	consonants := "BCDFGHJKLMNPQRSTVWXYZ"
	return strings.Contains(consonants, s)
}

func isVowel(s string) bool {
	if len(s) != 1 {
		return false
	}
	vowels := "AEIOU"
	return strings.Contains(vowels, s)
}

func isNumeric(s string) bool {
	for _, char := range s {
		if char < '0' || char > '9' {
			return false
		}
	}
	return len(s) > 0
}

func isCommonWord(token string) bool {
	commonWords := []string{
		"THE", "AND", "OR", "OF", "AT", "IN", "ON", "TO", "FOR", "WITH",
		"FLAT", "APARTMENT", "UNIT", "SUITE", "FLOOR", "GROUND", "FIRST",
		"SECOND", "THIRD", "UPPER", "LOWER", "REAR", "FRONT", "SIDE",
		"NORTH", "SOUTH", "EAST", "WEST", "OLD", "NEW", "LITTLE", "GREAT",
	}
	
	for _, word := range commonWords {
		if token == word {
			return true
		}
	}
	return false
}

func removeDuplicateChars(s string) string {
	if len(s) <= 1 {
		return s
	}
	
	result := strings.Builder{}
	result.WriteByte(s[0])
	
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1] {
			result.WriteByte(s[i])
		}
	}
	
	return result.String()
}