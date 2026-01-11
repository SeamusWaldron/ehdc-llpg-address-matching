package symspell

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DictionaryBuilder builds a SymSpell dictionary from LLPG data.
type DictionaryBuilder struct {
	db     *sql.DB
	config *Config
}

// NewDictionaryBuilder creates a new dictionary builder.
func NewDictionaryBuilder(db *sql.DB, config *Config) *DictionaryBuilder {
	if config == nil {
		config = DefaultConfig()
	}
	return &DictionaryBuilder{
		db:     db,
		config: config,
	}
}

// BuildFromLLPG builds a complete dictionary from the dim_address table.
// It extracts towns, street names, and other address tokens.
func (b *DictionaryBuilder) BuildFromLLPG() (*SymSpell, error) {
	startTime := time.Now()

	symspell := New(b.config)

	// Load towns/localities
	towns, err := b.extractTowns()
	if err != nil {
		return nil, fmt.Errorf("extracting towns: %w", err)
	}
	symspell.AddTerms(towns)

	// Load address tokens (streets, etc.)
	tokens, err := b.extractAddressTokens()
	if err != nil {
		return nil, fmt.Errorf("extracting address tokens: %w", err)
	}
	symspell.AddTerms(tokens)

	// Add Hampshire localities from the known list
	symspell.AddTerms(getHampshireLocalities())

	// Add common street suffixes
	symspell.AddTerms(getStreetSuffixes())

	stats := symspell.Stats()
	stats.BuildTimeMs = time.Since(startTime).Milliseconds()

	return symspell, nil
}

// extractTowns extracts unique town names from dim_address with frequencies.
func (b *DictionaryBuilder) extractTowns() ([]DictionaryEntry, error) {
	// Extract the last comma-separated component as town
	query := `
		SELECT
			UPPER(TRIM(SPLIT_PART(address_canonical, ',', ARRAY_LENGTH(STRING_TO_ARRAY(address_canonical, ','), 1)))) as term,
			COUNT(*) as freq
		FROM dim_address
		WHERE address_canonical IS NOT NULL
			AND address_canonical != ''
			AND LENGTH(TRIM(SPLIT_PART(address_canonical, ',', ARRAY_LENGTH(STRING_TO_ARRAY(address_canonical, ','), 1)))) >= 3
		GROUP BY 1
		HAVING COUNT(*) >= $1
		ORDER BY freq DESC
	`

	rows, err := b.db.Query(query, b.config.MinFrequency)
	if err != nil {
		return nil, fmt.Errorf("querying towns: %w", err)
	}
	defer rows.Close()

	var entries []DictionaryEntry
	for rows.Next() {
		var entry DictionaryEntry
		if err := rows.Scan(&entry.Term, &entry.Frequency); err != nil {
			return nil, fmt.Errorf("scanning town row: %w", err)
		}
		// Clean up the term
		entry.Term = strings.TrimSpace(entry.Term)
		if len(entry.Term) >= b.config.MinTermLength {
			entries = append(entries, entry)
		}
	}

	return entries, rows.Err()
}

// extractAddressTokens extracts individual word tokens from canonical addresses.
func (b *DictionaryBuilder) extractAddressTokens() ([]DictionaryEntry, error) {
	// Get all canonical addresses
	query := `
		SELECT address_canonical
		FROM dim_address
		WHERE address_canonical IS NOT NULL AND address_canonical != ''
	`

	rows, err := b.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("querying addresses: %w", err)
	}
	defer rows.Close()

	// Count token frequencies
	tokenFreq := make(map[string]int64)
	wordPattern := regexp.MustCompile(`[A-Z]+`)

	for rows.Next() {
		var addrCan string
		if err := rows.Scan(&addrCan); err != nil {
			return nil, fmt.Errorf("scanning address row: %w", err)
		}

		// Extract words
		words := wordPattern.FindAllString(strings.ToUpper(addrCan), -1)
		for _, word := range words {
			if len(word) >= b.config.MinTermLength && !isSkipWord(word) {
				tokenFreq[word]++
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Convert to entries
	entries := make([]DictionaryEntry, 0, len(tokenFreq))
	for term, freq := range tokenFreq {
		if freq >= b.config.MinFrequency {
			entries = append(entries, DictionaryEntry{
				Term:      term,
				Frequency: freq,
			})
		}
	}

	return entries, nil
}

// isSkipWord returns true for words that shouldn't be in the dictionary.
func isSkipWord(word string) bool {
	skipWords := map[string]bool{
		// Common noise words
		"THE": true, "AND": true, "OF": true, "AT": true, "IN": true, "ON": true,
		"TO": true, "FOR": true, "WITH": true, "BY": true,
		// Property descriptors
		"LAND": true, "REAR": true, "PLOT": true, "SITE": true, "ADJ": true,
		"ADJACENT": true, "PROPOSED": true, "FORMER": true,
		// Numbers would be filtered by length, but be explicit
		"A": true, "B": true, "C": true, "D": true,
	}
	return skipWords[word]
}

// getHampshireLocalities returns known Hampshire town/locality names.
// These are seeded with high frequency to ensure they're preferred.
func getHampshireLocalities() []DictionaryEntry {
	localities := []string{
		"ALTON", "PETERSFIELD", "LIPHOOK", "WATERLOOVILLE", "HORNDEAN",
		"BORDON", "WHITEHILL", "GRAYSHOTT", "HEADLEY", "BRAMSHOTT",
		"LINDFORD", "HOLLYWATER", "PASSFIELD", "CONFORD", "FOUR MARKS",
		"MEDSTEAD", "CHAWTON", "SELBORNE", "EMPSHOTT", "HAWKLEY",
		"LISS", "STEEP", "STROUD", "BURITON", "LANGRISH",
		"EAST MEON", "WEST MEON", "FROXFIELD", "PRIVETT", "ROPLEY",
		"WEST TISTED", "EAST TISTED", "BINSTED", "HOLT POUND", "BENTLEY",
		"FARNHAM", "HASLEMERE", "ALRESFORD", "WINCHESTER", "SOUTHAMPTON",
		"PORTSMOUTH", "GOSPORT", "FAREHAM", "HAVANT", "EMSWORTH",
		"ROWLANDS CASTLE", "CLANFIELD", "CATHERINGTON", "LOVEDEAN",
		"DENMEAD", "HAMBLEDON", "DROXFORD", "BISHOPS WALTHAM",
	}

	entries := make([]DictionaryEntry, len(localities))
	for i, loc := range localities {
		entries[i] = DictionaryEntry{
			Term:      loc,
			Frequency: 10000, // High frequency to prefer these
		}
	}
	return entries
}

// getStreetSuffixes returns common UK street suffixes.
func getStreetSuffixes() []DictionaryEntry {
	suffixes := []string{
		"ROAD", "STREET", "LANE", "CLOSE", "DRIVE", "AVENUE", "GARDENS",
		"COURT", "TERRACE", "WAY", "GROVE", "PLACE", "CRESCENT", "HILL",
		"RISE", "GREEN", "PARK", "SQUARE", "WALK", "MEWS", "PASSAGE",
		"YARD", "ROW", "PARADE", "CIRCUS", "BROADWAY", "HIGHWAY",
	}

	entries := make([]DictionaryEntry, len(suffixes))
	for i, suffix := range suffixes {
		entries[i] = DictionaryEntry{
			Term:      suffix,
			Frequency: 50000, // Very high frequency as these are common
		}
	}
	return entries
}

// BuildFromEntries builds a dictionary from pre-provided entries.
// Useful for testing or when database is not available.
func BuildFromEntries(entries []DictionaryEntry, config *Config) *SymSpell {
	if config == nil {
		config = DefaultConfig()
	}
	symspell := New(config)
	symspell.AddTerms(entries)
	return symspell
}
