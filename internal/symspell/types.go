// Package symspell implements the SymSpell spelling correction algorithm
// for UK address tokens as specified in Appendix I of the EHDC LLPG thesis.
//
// SymSpell uses a pre-computed "delete dictionary" approach for O(1) lookup
// performance, making it suitable for high-volume address correction.
package symspell

import (
	"os"
	"strconv"
)

// Config holds SymSpell configuration parameters.
// Default values are from Appendix I of the thesis.
type Config struct {
	// MaxEditDistance is the maximum Damerau-Levenshtein distance for corrections.
	// Default: 2 (catches most typos while avoiding false corrections)
	MaxEditDistance int

	// PrefixLength is the length of prefix used for indexing.
	// Default: 7 (balances memory usage and lookup speed)
	PrefixLength int

	// Enabled controls whether spelling correction is active.
	// Default: false (must be explicitly enabled)
	Enabled bool

	// MinTermLength is the minimum token length to attempt correction.
	// Default: 3 (avoids correcting abbreviations like "RD", "ST")
	MinTermLength int

	// MinFrequency is the minimum frequency for a term to be included in dictionary.
	// Default: 1 (include all terms)
	MinFrequency int64
}

// DefaultConfig returns the default configuration from Appendix I.
func DefaultConfig() *Config {
	return &Config{
		MaxEditDistance: 2,
		PrefixLength:    7,
		Enabled:         false,
		MinTermLength:   3,
		MinFrequency:    1,
	}
}

// LoadConfigFromEnv loads configuration from environment variables.
func LoadConfigFromEnv() *Config {
	cfg := DefaultConfig()

	if v := os.Getenv("SYMSPELL_ENABLED"); v != "" {
		cfg.Enabled = v == "true" || v == "1"
	}

	if v := os.Getenv("SYMSPELL_MAX_EDIT_DISTANCE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 3 {
			cfg.MaxEditDistance = n
		}
	}

	if v := os.Getenv("SYMSPELL_PREFIX_LENGTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.PrefixLength = n
		}
	}

	if v := os.Getenv("SYMSPELL_MIN_TERM_LENGTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MinTermLength = n
		}
	}

	return cfg
}

// Suggestion represents a spelling correction suggestion.
type Suggestion struct {
	// Term is the suggested correct spelling.
	Term string

	// Distance is the edit distance from the input to this suggestion.
	Distance int

	// Frequency is the occurrence count in the dictionary.
	// Higher frequency terms are preferred when distances are equal.
	Frequency int64
}

// CorrectionResult tracks what was corrected for audit and explainability.
type CorrectionResult struct {
	// Original is the input token before correction.
	Original string

	// Corrected is the token after correction (same as Original if no correction).
	Corrected string

	// Distance is the edit distance (0 if no correction needed).
	Distance int

	// WasCorrected indicates whether a correction was applied.
	WasCorrected bool

	// Confidence is a score from 0-1 indicating correction confidence.
	// Calculated as 1 - (distance / maxEditDistance).
	Confidence float64
}

// DictionaryEntry represents a term with its frequency for dictionary building.
type DictionaryEntry struct {
	Term      string
	Frequency int64
}

// DictionaryStats holds statistics about the built dictionary.
type DictionaryStats struct {
	// TermCount is the number of unique terms in the dictionary.
	TermCount int

	// DeleteCount is the number of entries in the delete dictionary.
	DeleteCount int

	// TotalFrequency is the sum of all term frequencies.
	TotalFrequency int64

	// MaxFrequency is the highest frequency term.
	MaxFrequency int64

	// BuildTimeMs is the time taken to build the dictionary in milliseconds.
	BuildTimeMs int64
}
