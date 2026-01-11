package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq"

	"github.com/ehdc-llpg/internal/symspell"
)

func main() {
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

	fmt.Println("Building SymSpell dictionary...")
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
	fmt.Printf("Dictionary ready: %d terms\n\n", ss.Stats().TermCount)

	// Sample addresses from source documents
	query := `
		SELECT DISTINCT raw_address
		FROM src_document
		WHERE raw_address IS NOT NULL AND raw_address != ''
		LIMIT 5000
	`

	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("Failed to query: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	var correctedCount, totalCount int
	var examples []string

	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			continue
		}
		totalCount++

		// Normalize: uppercase, strip punctuation
		normalized := strings.ToUpper(addr)
		var cleanTokens []string
		for _, r := range normalized {
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' {
				cleanTokens = append(cleanTokens, string(r))
			} else {
				cleanTokens = append(cleanTokens, " ")
			}
		}
		normalized = strings.Join(strings.Fields(string(strings.Join(cleanTokens, ""))), " ")

		// Tokenize and check each token
		tokens := strings.Fields(normalized)
		var corrections []string
		for _, token := range tokens {
			if len(token) < 3 {
				continue
			}
			// Skip numbers and house numbers
			if isNumber(token) {
				continue
			}
			// Skip postcodes (pattern like GU34, PO8, SO24)
			if isPostcode(token) {
				continue
			}
			// Skip common words that shouldn't be corrected
			if isSkipWord(token) {
				continue
			}
			// Check if token can be corrected
			suggestion := ss.LookupBest(token, 2)
			if suggestion != nil && suggestion.Distance > 0 {
				corrections = append(corrections, fmt.Sprintf("%s→%s", token, suggestion.Term))
			}
		}

		if len(corrections) > 0 {
			correctedCount++
			if len(examples) < 30 {
				examples = append(examples, fmt.Sprintf("  %s\n    Corrections: %s", addr, strings.Join(corrections, ", ")))
			}
		}
	}

	fmt.Printf("=== SymSpell Impact Analysis ===\n")
	fmt.Printf("Total addresses sampled: %d\n", totalCount)
	fmt.Printf("Addresses with potential corrections: %d (%.1f%%)\n", correctedCount, float64(correctedCount)*100/float64(totalCount))
	fmt.Printf("\nExample corrections:\n")
	for _, ex := range examples {
		fmt.Println(ex)
	}
}

func isNumber(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			if r < 'A' || r > 'Z' {
				return false
			}
		}
	}
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		return true
	}
	return false
}

func isPostcode(s string) bool {
	// UK postcode patterns: starts with 1-2 letters followed by digit
	if len(s) < 2 || len(s) > 8 {
		return false
	}
	// Check first char is letter
	if s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	// Check has digit somewhere after first char
	hasDigit := false
	for i := 1; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return false
	}
	// Common postcode patterns
	prefixes := []string{"GU", "PO", "SO", "RG", "SP", "BH", "SN", "BN", "HP"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) && len(s) >= 3 {
			return true
		}
	}
	return false
}

func isSkipWord(s string) bool {
	skip := map[string]bool{
		"THE": true, "AND": true, "OF": true, "AT": true, "IN": true, "ON": true,
		"TO": true, "FOR": true, "WITH": true, "BY": true, "OR": true,
		"LAND": true, "ADJ": true, "ADJACENT": true, "OPP": true, "SITE": true,
		"PLOT": true, "FLAT": true, "UNIT": true, "FLOOR": true, "FIRST": true,
		"GROUND": true, "REAR": true, "FORMER": true, "PROPOSED": true,
	}
	return skip[s]
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
