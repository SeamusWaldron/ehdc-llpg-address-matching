package symspell

import (
	"database/sql"
	"regexp"
	"strings"
	"sync"
)

// Corrector provides high-level address spelling correction.
type Corrector struct {
	symspell *SymSpell
	config   *Config
	mu       sync.RWMutex
}

// Global corrector singleton for use across the application.
var (
	globalCorrector     *Corrector
	globalCorrectorOnce sync.Once
	globalCorrectorErr  error
)

// GetCorrector returns the global corrector instance.
// Returns nil if SymSpell is not enabled or initialization failed.
func GetCorrector() *Corrector {
	return globalCorrector
}

// IsEnabled returns true if SymSpell correction is enabled and initialized.
func IsEnabled() bool {
	config := LoadConfigFromEnv()
	return config.Enabled && globalCorrector != nil
}

// InitGlobalCorrector initializes the global corrector from database.
// Should be called once at application startup if SymSpell is enabled.
func InitGlobalCorrector(db *sql.DB) error {
	config := LoadConfigFromEnv()
	if !config.Enabled {
		return nil
	}

	globalCorrectorOnce.Do(func() {
		builder := NewDictionaryBuilder(db, config)
		symspell, err := builder.BuildFromLLPG()
		if err != nil {
			globalCorrectorErr = err
			return
		}

		globalCorrector = &Corrector{
			symspell: symspell,
			config:   config,
		}
	})

	return globalCorrectorErr
}

// InitWithEntries initializes a corrector with pre-built entries (for testing).
func InitWithEntries(entries []DictionaryEntry, config *Config) *Corrector {
	if config == nil {
		config = DefaultConfig()
	}
	return &Corrector{
		symspell: BuildFromEntries(entries, config),
		config:   config,
	}
}

// CorrectAddress corrects spelling in an address string.
// Returns the corrected address and a list of corrections made.
func (c *Corrector) CorrectAddress(address string) (string, []CorrectionResult) {
	if c == nil || c.symspell == nil {
		return address, nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Split into tokens
	tokens := strings.Fields(address)
	var corrections []CorrectionResult
	modified := false

	for i, token := range tokens {
		result := c.correctToken(token)
		if result.WasCorrected {
			tokens[i] = result.Corrected
			corrections = append(corrections, result)
			modified = true
		}
	}

	if !modified {
		return address, nil
	}

	return strings.Join(tokens, " "), corrections
}

// correctToken attempts to correct a single token.
func (c *Corrector) correctToken(token string) CorrectionResult {
	token = strings.ToUpper(strings.TrimSpace(token))

	// Skip tokens that are too short
	if len(token) < c.config.MinTermLength {
		return CorrectionResult{Original: token, Corrected: token, WasCorrected: false}
	}

	// Skip numbers and alphanumeric tokens (house numbers like "12A")
	if isNumericOrHouseNumber(token) {
		return CorrectionResult{Original: token, Corrected: token, WasCorrected: false}
	}

	// Skip known street suffixes (they should already be correct)
	if isStreetSuffix(token) {
		return CorrectionResult{Original: token, Corrected: token, WasCorrected: false}
	}

	// Look up in SymSpell
	suggestion := c.symspell.LookupBest(token, c.config.MaxEditDistance)
	if suggestion == nil {
		return CorrectionResult{Original: token, Corrected: token, WasCorrected: false}
	}

	// Only apply correction if distance > 0
	if suggestion.Distance == 0 {
		return CorrectionResult{Original: token, Corrected: token, WasCorrected: false}
	}

	// Calculate confidence: 1 - (distance / maxEditDistance)
	confidence := 1.0 - float64(suggestion.Distance)/float64(c.config.MaxEditDistance)

	return CorrectionResult{
		Original:     token,
		Corrected:    suggestion.Term,
		Distance:     suggestion.Distance,
		WasCorrected: true,
		Confidence:   confidence,
	}
}

// CorrectToken corrects a single token and returns the correction result.
// Exported for use in matching where per-token correction is needed.
func (c *Corrector) CorrectToken(token string) CorrectionResult {
	if c == nil || c.symspell == nil {
		return CorrectionResult{Original: token, Corrected: token, WasCorrected: false}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.correctToken(token)
}

// LookupSuggestions returns all suggestions for a token (for debugging/UI).
func (c *Corrector) LookupSuggestions(token string, maxResults int) []Suggestion {
	if c == nil || c.symspell == nil {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	suggestions := c.symspell.Lookup(token, c.config.MaxEditDistance)
	if maxResults > 0 && len(suggestions) > maxResults {
		return suggestions[:maxResults]
	}
	return suggestions
}

// Stats returns dictionary statistics.
func (c *Corrector) Stats() DictionaryStats {
	if c == nil || c.symspell == nil {
		return DictionaryStats{}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.symspell.Stats()
}

// Helper functions

var houseNumberPattern = regexp.MustCompile(`^\d+[A-Z]?$`)

func isNumericOrHouseNumber(token string) bool {
	return houseNumberPattern.MatchString(token)
}

func isStreetSuffix(token string) bool {
	suffixes := map[string]bool{
		"ROAD": true, "STREET": true, "LANE": true, "CLOSE": true, "DRIVE": true,
		"AVENUE": true, "GARDENS": true, "COURT": true, "TERRACE": true, "WAY": true,
		"GROVE": true, "PLACE": true, "CRESCENT": true, "HILL": true, "RISE": true,
		"GREEN": true, "PARK": true, "SQUARE": true, "WALK": true, "MEWS": true,
		"PASSAGE": true, "YARD": true, "ROW": true, "PARADE": true,
	}
	return suffixes[token]
}
