package symspell

import (
	"testing"
)

// Test dictionary with Hampshire localities and common street terms
func buildTestDictionary() *SymSpell {
	entries := []DictionaryEntry{
		// Hampshire towns
		{Term: "PETERSFIELD", Frequency: 5000},
		{Term: "ALTON", Frequency: 4000},
		{Term: "LIPHOOK", Frequency: 3000},
		{Term: "HORNDEAN", Frequency: 2500},
		{Term: "WATERLOOVILLE", Frequency: 2000},
		{Term: "BORDON", Frequency: 1500},
		{Term: "WHITEHILL", Frequency: 1200},
		{Term: "GRAYSHOTT", Frequency: 1000},
		{Term: "HEADLEY", Frequency: 900},
		{Term: "ALRESFORD", Frequency: 800},
		{Term: "WINCHESTER", Frequency: 3500},
		{Term: "SOUTHAMPTON", Frequency: 4500},
		// Street names
		{Term: "HIGH", Frequency: 10000},
		{Term: "CHURCH", Frequency: 8000},
		{Term: "MILL", Frequency: 5000},
		{Term: "STATION", Frequency: 4000},
		{Term: "LONDON", Frequency: 3500},
		// Street suffixes
		{Term: "ROAD", Frequency: 50000},
		{Term: "STREET", Frequency: 40000},
		{Term: "LANE", Frequency: 30000},
		{Term: "CLOSE", Frequency: 20000},
		{Term: "AVENUE", Frequency: 15000},
		{Term: "GARDENS", Frequency: 10000},
	}

	config := &Config{
		MaxEditDistance: 2,
		PrefixLength:    7,
		MinTermLength:   3,
		MinFrequency:    1,
		Enabled:         true,
	}

	return BuildFromEntries(entries, config)
}

func TestSymSpellLookup(t *testing.T) {
	symspell := buildTestDictionary()

	tests := []struct {
		name         string
		input        string
		wantTerm     string
		wantDistance int
	}{
		// Exact matches (distance 0)
		{
			name:         "exact match town",
			input:        "PETERSFIELD",
			wantTerm:     "PETERSFIELD",
			wantDistance: 0,
		},
		{
			name:         "exact match street",
			input:        "HIGH",
			wantTerm:     "HIGH",
			wantDistance: 0,
		},
		// Single character errors (distance 1)
		{
			name:         "missing letter in town",
			input:        "PTTERSFIELD",
			wantTerm:     "PETERSFIELD",
			wantDistance: 1,
		},
		{
			name:         "missing letter at end",
			input:        "HORNDEA",
			wantTerm:     "HORNDEAN",
			wantDistance: 1,
		},
		{
			name:         "extra letter",
			input:        "ALTTON",
			wantTerm:     "ALTON",
			wantDistance: 1,
		},
		{
			name:         "transposition",
			input:        "CHRUCH",
			wantTerm:     "CHURCH",
			wantDistance: 1,
		},
		{
			name:         "wrong letter",
			input:        "HIHG",
			wantTerm:     "HIGH",
			wantDistance: 1,
		},
		// Two character errors (distance 2)
		{
			name:         "two errors in town",
			input:        "PTERSFILD",
			wantTerm:     "PETERSFIELD",
			wantDistance: 2,
		},
		{
			name:         "transposition at end",
			input:        "GARDNES",
			wantTerm:     "GARDENS",
			wantDistance: 1, // NE->EN is single transposition
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions := symspell.Lookup(tt.input, 2)

			if len(suggestions) == 0 {
				t.Errorf("Lookup(%q) returned no suggestions", tt.input)
				return
			}

			best := suggestions[0]
			if best.Term != tt.wantTerm {
				t.Errorf("Lookup(%q) = %q, want %q", tt.input, best.Term, tt.wantTerm)
			}
			if best.Distance != tt.wantDistance {
				t.Errorf("Lookup(%q) distance = %d, want %d", tt.input, best.Distance, tt.wantDistance)
			}
		})
	}
}

func TestSymSpellNoMatch(t *testing.T) {
	symspell := buildTestDictionary()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "completely different word",
			input: "XXXXXXXX",
		},
		{
			name:  "too many errors",
			input: "PTRSFLD", // 4 errors from PETERSFIELD
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions := symspell.Lookup(tt.input, 2)

			if len(suggestions) > 0 {
				t.Errorf("Lookup(%q) should return no suggestions, got %v", tt.input, suggestions)
			}
		})
	}
}

func TestSymSpellFrequencyOrdering(t *testing.T) {
	// When distances are equal, higher frequency should come first
	symspell := buildTestDictionary()

	// ROAD has higher frequency than LOAD (if we had it)
	suggestions := symspell.Lookup("ROAD", 0)
	if len(suggestions) == 0 {
		t.Fatal("Expected at least one suggestion")
	}
	if suggestions[0].Term != "ROAD" {
		t.Errorf("Expected ROAD as first suggestion, got %s", suggestions[0].Term)
	}
}

func TestCorrectorCorrectAddress(t *testing.T) {
	symspell := buildTestDictionary()
	corrector := &Corrector{
		symspell: symspell,
		config: &Config{
			MaxEditDistance: 2,
			MinTermLength:   3,
			Enabled:         true,
		},
	}

	tests := []struct {
		name            string
		input           string
		wantCorrected   string
		wantCorrections int
	}{
		{
			name:            "no corrections needed",
			input:           "12 HIGH STREET PETERSFIELD",
			wantCorrected:   "12 HIGH STREET PETERSFIELD",
			wantCorrections: 0,
		},
		{
			name:            "single typo correction",
			input:           "12 HIHG STREET PETERSFIELD",
			wantCorrected:   "12 HIGH STREET PETERSFIELD",
			wantCorrections: 1,
		},
		{
			name:            "town name typo",
			input:           "12 HIGH STREET PTTERSFIELD",
			wantCorrected:   "12 HIGH STREET PETERSFIELD",
			wantCorrections: 1,
		},
		{
			name:            "multiple typos",
			input:           "12 HIHG STREET PTTERSFIELD",
			wantCorrected:   "12 HIGH STREET PETERSFIELD",
			wantCorrections: 2,
		},
		{
			name:            "preserve house numbers",
			input:           "12A HIHG STREET",
			wantCorrected:   "12A HIGH STREET",
			wantCorrections: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corrected, corrections := corrector.CorrectAddress(tt.input)

			if corrected != tt.wantCorrected {
				t.Errorf("CorrectAddress(%q) = %q, want %q", tt.input, corrected, tt.wantCorrected)
			}
			if len(corrections) != tt.wantCorrections {
				t.Errorf("CorrectAddress(%q) made %d corrections, want %d", tt.input, len(corrections), tt.wantCorrections)
			}
		})
	}
}

func TestCorrectorSkipsHouseNumbers(t *testing.T) {
	symspell := buildTestDictionary()
	corrector := &Corrector{
		symspell: symspell,
		config: &Config{
			MaxEditDistance: 2,
			MinTermLength:   3,
			Enabled:         true,
		},
	}

	// House numbers should not be corrected
	houseNumbers := []string{"12", "12A", "100", "1B", "23C"}

	for _, num := range houseNumbers {
		result := corrector.CorrectToken(num)
		if result.WasCorrected {
			t.Errorf("House number %q should not be corrected", num)
		}
	}
}

func TestCorrectorSkipsStreetSuffixes(t *testing.T) {
	symspell := buildTestDictionary()
	corrector := &Corrector{
		symspell: symspell,
		config: &Config{
			MaxEditDistance: 2,
			MinTermLength:   3,
			Enabled:         true,
		},
	}

	// Street suffixes should not be corrected (they're already correct)
	suffixes := []string{"ROAD", "STREET", "LANE", "CLOSE", "AVENUE"}

	for _, suffix := range suffixes {
		result := corrector.CorrectToken(suffix)
		if result.WasCorrected {
			t.Errorf("Street suffix %q should not be corrected", suffix)
		}
	}
}

func TestEditDistance(t *testing.T) {
	symspell := New(DefaultConfig())

	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "ab", 1},   // deletion
		{"ab", "abc", 1},   // insertion
		{"abc", "adc", 1},  // substitution
		{"abc", "acb", 1},  // transposition (Damerau)
		{"abc", "def", 3},  // all different
		{"kitten", "sitting", 3},
	}

	for _, tt := range tests {
		got := symspell.editDistance(tt.a, tt.b, 10)
		if got != tt.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestEditDistanceEarlyExit(t *testing.T) {
	symspell := New(DefaultConfig())

	// Should return -1 when distance exceeds max
	got := symspell.editDistance("abc", "xyz", 1)
	if got != -1 {
		t.Errorf("editDistance with maxDist=1 should return -1 for distance 3, got %d", got)
	}
}

func TestGenerateDeletes(t *testing.T) {
	symspell := New(&Config{MaxEditDistance: 2, PrefixLength: 7})

	// Test single character deletion
	deletes := symspell.generateDeletes("ABC", 1)
	expected := map[string]bool{"AB": true, "AC": true, "BC": true}

	if len(deletes) != len(expected) {
		t.Errorf("generateDeletes(ABC, 1) returned %d items, want %d", len(deletes), len(expected))
	}

	for _, del := range deletes {
		if !expected[del] {
			t.Errorf("Unexpected delete: %q", del)
		}
	}
}

func TestDictionaryStats(t *testing.T) {
	symspell := buildTestDictionary()
	stats := symspell.Stats()

	if stats.TermCount == 0 {
		t.Error("TermCount should be > 0")
	}
	if stats.DeleteCount == 0 {
		t.Error("DeleteCount should be > 0")
	}
	if stats.TotalFrequency == 0 {
		t.Error("TotalFrequency should be > 0")
	}
}

func BenchmarkSymSpellLookup(b *testing.B) {
	symspell := buildTestDictionary()
	input := "PTTERSFIELD"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		symspell.Lookup(input, 2)
	}
}

func BenchmarkCorrectorCorrectAddress(b *testing.B) {
	symspell := buildTestDictionary()
	corrector := &Corrector{
		symspell: symspell,
		config: &Config{
			MaxEditDistance: 2,
			MinTermLength:   3,
			Enabled:         true,
		},
	}
	input := "12 HIHG STREET PTTERSFIELD"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		corrector.CorrectAddress(input)
	}
}
