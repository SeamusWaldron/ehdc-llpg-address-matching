// Test script for SymSpell spelling correction with real LLPG data
package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"

	"github.com/ehdc-llpg/internal/symspell"
)

func main() {
	// Connect to database
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("PGHOST", "localhost"),
		getEnv("PGPORT", "15435"),
		getEnv("PGUSER", "postgres"),
		getEnv("PGPASSWORD", "kljh234hjkl2h"),
		getEnv("PGDATABASE", "ehdc_llpg"),
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Printf("Failed to ping database: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Connected to database")
	fmt.Println()

	// Build dictionary from LLPG
	fmt.Println("Building SymSpell dictionary from LLPG...")
	startTime := time.Now()

	config := &symspell.Config{
		MaxEditDistance: 2,
		PrefixLength:    7,
		MinTermLength:   3,
		MinFrequency:    1,
		Enabled:         true,
	}

	builder := symspell.NewDictionaryBuilder(db, config)
	ss, err := builder.BuildFromLLPG()
	if err != nil {
		fmt.Printf("Failed to build dictionary: %v\n", err)
		os.Exit(1)
	}

	buildTime := time.Since(startTime)
	stats := ss.Stats()

	fmt.Printf("Dictionary built in %v\n", buildTime)
	fmt.Printf("  Terms: %d\n", stats.TermCount)
	fmt.Printf("  Delete entries: %d\n", stats.DeleteCount)
	fmt.Printf("  Total frequency: %d\n", stats.TotalFrequency)
	fmt.Println()

	// Use the built symspell directly for testing
	_ = symspell.InitWithEntries(nil, config) // Just to verify it compiles

	// Test addresses with intentional typos
	testAddresses := []string{
		// Town name typos
		"12 HIGH STREET PTTERSFIELD",      // PETERSFIELD
		"5 CHURCH LANE ALTTON",            // ALTON
		"23 MILL ROAD LIPHOK",             // LIPHOOK
		"7 STATION ROAD HORNDEA",          // HORNDEAN
		"15 LONDON ROAD WATERLOOVILEL",    // WATERLOOVILLE
		// Street name typos
		"8 HIHG STREET ALTON",             // HIGH
		"3 CHRUCH LANE PETERSFIELD",       // CHURCH
		"10 STATOIN ROAD LIPHOOK",         // STATION
		"2 LONDN ROAD BORDON",             // LONDON
		// Multiple typos
		"6 HIHG STRET PTTERSFIELD",        // HIGH STREET PETERSFIELD
		// Real addresses that should not change
		"12 HIGH STREET PETERSFIELD",
		"5 CHURCH LANE ALTON",
		"23 MILL ROAD LIPHOOK",
	}

	fmt.Println("Testing spelling correction:")
	fmt.Println("=" + string(make([]byte, 79)))

	// We need to use the symspell instance directly since corrector wrapper needs initialization
	for _, addr := range testAddresses {
		corrected, corrections := correctAddressWithSymSpell(ss, addr, config)

		if len(corrections) > 0 {
			fmt.Printf("\nInput:     %s\n", addr)
			fmt.Printf("Corrected: %s\n", corrected)
			fmt.Println("Changes:")
			for _, c := range corrections {
				fmt.Printf("  - %s -> %s (distance=%d, confidence=%.2f)\n",
					c.Original, c.Corrected, c.Distance, c.Confidence)
			}
		} else {
			fmt.Printf("\nInput:     %s\n", addr)
			fmt.Printf("Corrected: (no changes needed)\n")
		}
	}

	fmt.Println()
	fmt.Println("=" + string(make([]byte, 79)))

	// Interactive lookup test
	fmt.Println("\nSample lookups:")
	testTerms := []string{
		"PTTERSFIELD", "PETERSFIELD",
		"ALTTON", "ALTON",
		"HIHG", "HIGH",
		"CHRUCH", "CHURCH",
		"STRET", "STREET",
		"HORNDEA", "HORNDEAN",
	}

	for _, term := range testTerms {
		suggestions := ss.Lookup(term, 2)
		if len(suggestions) > 0 {
			best := suggestions[0]
			if best.Distance == 0 {
				fmt.Printf("  %s -> (exact match, freq=%d)\n", term, best.Frequency)
			} else {
				fmt.Printf("  %s -> %s (distance=%d, freq=%d)\n",
					term, best.Term, best.Distance, best.Frequency)
			}
		} else {
			fmt.Printf("  %s -> (no suggestions)\n", term)
		}
	}
}

// correctAddressWithSymSpell corrects an address using the symspell instance directly
func correctAddressWithSymSpell(ss *symspell.SymSpell, address string, config *symspell.Config) (string, []symspell.CorrectionResult) {
	tokens := splitWords(address)
	var corrections []symspell.CorrectionResult
	modified := false

	for i, token := range tokens {
		if len(token) < config.MinTermLength {
			continue
		}
		if isNumber(token) || isStreetSuffix(token) {
			continue
		}

		suggestion := ss.LookupBest(token, config.MaxEditDistance)
		if suggestion != nil && suggestion.Distance > 0 {
			corrections = append(corrections, symspell.CorrectionResult{
				Original:     token,
				Corrected:    suggestion.Term,
				Distance:     suggestion.Distance,
				WasCorrected: true,
				Confidence:   1.0 - float64(suggestion.Distance)/float64(config.MaxEditDistance),
			})
			tokens[i] = suggestion.Term
			modified = true
		}
	}

	if !modified {
		return address, nil
	}

	return joinWords(tokens), corrections
}

func splitWords(s string) []string {
	var words []string
	word := ""
	for _, r := range s {
		if r == ' ' {
			if word != "" {
				words = append(words, word)
				word = ""
			}
		} else {
			word += string(r)
		}
	}
	if word != "" {
		words = append(words, word)
	}
	return words
}

func joinWords(words []string) string {
	result := ""
	for i, w := range words {
		if i > 0 {
			result += " "
		}
		result += w
	}
	return result
}

func isNumber(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			if r < 'A' || r > 'Z' {
				return false
			}
		}
	}
	// Check if it starts with a digit
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		return true
	}
	return false
}

func isStreetSuffix(s string) bool {
	suffixes := map[string]bool{
		"ROAD": true, "STREET": true, "LANE": true, "CLOSE": true,
		"DRIVE": true, "AVENUE": true, "GARDENS": true, "COURT": true,
		"TERRACE": true, "WAY": true, "GROVE": true, "PLACE": true,
		"CRESCENT": true, "HILL": true, "RISE": true, "GREEN": true,
		"PARK": true, "SQUARE": true, "WALK": true, "MEWS": true,
	}
	return suffixes[s]
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
