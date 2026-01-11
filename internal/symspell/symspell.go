package symspell

import (
	"sort"
	"strings"
)

// SymSpell implements the Symmetric Delete spelling correction algorithm.
// It pre-computes all possible deletions within max edit distance for O(1) lookup.
type SymSpell struct {
	// dictionary maps terms to their frequencies
	dictionary map[string]int64

	// deletes maps delete variants to their original terms
	deletes map[string][]string

	// config holds algorithm parameters
	config *Config
}

// New creates a new SymSpell instance with the given configuration.
func New(config *Config) *SymSpell {
	if config == nil {
		config = DefaultConfig()
	}
	return &SymSpell{
		dictionary: make(map[string]int64),
		deletes:    make(map[string][]string),
		config:     config,
	}
}

// AddTerm adds a term to the dictionary with its frequency.
// It also generates and indexes all delete variants.
func (s *SymSpell) AddTerm(term string, frequency int64) {
	term = strings.ToUpper(strings.TrimSpace(term))
	if len(term) < s.config.MinTermLength {
		return
	}

	// Add to dictionary
	s.dictionary[term] = frequency

	// Generate deletes and add to index
	deletes := s.generateDeletes(term, s.config.MaxEditDistance)
	for _, del := range deletes {
		s.deletes[del] = append(s.deletes[del], term)
	}
}

// AddTerms adds multiple terms to the dictionary.
func (s *SymSpell) AddTerms(entries []DictionaryEntry) {
	for _, entry := range entries {
		s.AddTerm(entry.Term, entry.Frequency)
	}
}

// Contains checks if a term exists exactly in the dictionary.
func (s *SymSpell) Contains(term string) bool {
	term = strings.ToUpper(strings.TrimSpace(term))
	_, ok := s.dictionary[term]
	return ok
}

// Lookup finds spelling suggestions for the input term.
// Returns suggestions sorted by edit distance (ascending), then frequency (descending).
func (s *SymSpell) Lookup(input string, maxDistance int) []Suggestion {
	input = strings.ToUpper(strings.TrimSpace(input))
	if len(input) == 0 {
		return nil
	}

	// Cap at configured max
	if maxDistance > s.config.MaxEditDistance {
		maxDistance = s.config.MaxEditDistance
	}

	// Check exact match first (O(1))
	if freq, ok := s.dictionary[input]; ok {
		return []Suggestion{{Term: input, Distance: 0, Frequency: freq}}
	}

	// Track candidates to avoid duplicates
	seen := make(map[string]bool)
	var candidates []Suggestion

	// Generate deletes of the input term
	inputDeletes := s.generateDeletes(input, maxDistance)

	// Also check the input itself as a potential delete of dictionary terms
	inputDeletes = append(inputDeletes, input)

	for _, del := range inputDeletes {
		// Look up terms that have this delete variant
		if terms, ok := s.deletes[del]; ok {
			for _, term := range terms {
				if seen[term] {
					continue
				}
				seen[term] = true

				// Calculate actual edit distance
				dist := s.editDistance(input, term, maxDistance)
				if dist >= 0 && dist <= maxDistance {
					candidates = append(candidates, Suggestion{
						Term:      term,
						Distance:  dist,
						Frequency: s.dictionary[term],
					})
				}
			}
		}

		// Also check if the delete itself is in the dictionary
		// (handles case where input has extra characters)
		if freq, ok := s.dictionary[del]; ok && !seen[del] {
			seen[del] = true
			dist := s.editDistance(input, del, maxDistance)
			if dist >= 0 && dist <= maxDistance {
				candidates = append(candidates, Suggestion{
					Term:      del,
					Distance:  dist,
					Frequency: freq,
				})
			}
		}
	}

	// Sort by distance (asc), then frequency (desc)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Distance != candidates[j].Distance {
			return candidates[i].Distance < candidates[j].Distance
		}
		return candidates[i].Frequency > candidates[j].Frequency
	})

	return candidates
}

// LookupBest returns the single best suggestion, or nil if none found.
func (s *SymSpell) LookupBest(input string, maxDistance int) *Suggestion {
	suggestions := s.Lookup(input, maxDistance)
	if len(suggestions) == 0 {
		return nil
	}
	return &suggestions[0]
}

// generateDeletes generates all delete variants of a term within maxDistance.
// Uses recursive approach to generate combinations of character deletions.
func (s *SymSpell) generateDeletes(term string, maxDistance int) []string {
	if maxDistance <= 0 || len(term) == 0 {
		return nil
	}

	deletes := make(map[string]bool)
	s.generateDeletesRecursive(term, maxDistance, deletes)

	result := make([]string, 0, len(deletes))
	for del := range deletes {
		result = append(result, del)
	}
	return result
}

func (s *SymSpell) generateDeletesRecursive(term string, distance int, deletes map[string]bool) {
	if distance <= 0 || len(term) <= 1 {
		return
	}

	// Generate all single-character deletions
	for i := 0; i < len(term); i++ {
		del := term[:i] + term[i+1:]
		if !deletes[del] {
			deletes[del] = true
			// Recursively generate further deletions
			s.generateDeletesRecursive(del, distance-1, deletes)
		}
	}
}

// editDistance calculates the Damerau-Levenshtein distance between two strings.
// Returns -1 if distance exceeds maxDistance (early exit optimisation).
func (s *SymSpell) editDistance(a, b string, maxDistance int) int {
	lenA, lenB := len(a), len(b)

	// Quick length check
	if abs(lenA-lenB) > maxDistance {
		return -1
	}

	// Empty string cases
	if lenA == 0 {
		return lenB
	}
	if lenB == 0 {
		return lenA
	}

	// Ensure a is the shorter string for optimisation
	if lenA > lenB {
		a, b = b, a
		lenA, lenB = lenB, lenA
	}

	// Use only two rows of the matrix for memory efficiency
	prev := make([]int, lenA+1)
	curr := make([]int, lenA+1)
	prevPrev := make([]int, lenA+1) // For transposition

	// Initialise first row
	for i := 0; i <= lenA; i++ {
		prev[i] = i
	}

	// Fill the matrix
	for j := 1; j <= lenB; j++ {
		curr[0] = j
		minDist := j

		for i := 1; i <= lenA; i++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}

			// Standard Levenshtein operations
			curr[i] = min3(
				prev[i]+1,      // deletion
				curr[i-1]+1,    // insertion
				prev[i-1]+cost, // substitution
			)

			// Damerau transposition
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				curr[i] = min2(curr[i], prevPrev[i-2]+cost)
			}

			if curr[i] < minDist {
				minDist = curr[i]
			}
		}

		// Early exit if minimum distance in row exceeds maxDistance
		if minDist > maxDistance {
			return -1
		}

		// Rotate rows
		prevPrev, prev, curr = prev, curr, prevPrev
	}

	if prev[lenA] > maxDistance {
		return -1
	}
	return prev[lenA]
}

// Stats returns statistics about the dictionary.
func (s *SymSpell) Stats() DictionaryStats {
	stats := DictionaryStats{
		TermCount:   len(s.dictionary),
		DeleteCount: len(s.deletes),
	}

	for _, freq := range s.dictionary {
		stats.TotalFrequency += freq
		if freq > stats.MaxFrequency {
			stats.MaxFrequency = freq
		}
	}

	return stats
}

// Helper functions

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func min3(a, b, c int) int {
	return min2(min2(a, b), c)
}
