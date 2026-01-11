package engine

import (
	"database/sql"
	"fmt"
	"strings"
)

// DeterministicMatcher handles Stage 1 deterministic matching
type DeterministicMatcher struct {
	db *sql.DB
}

// NewDeterministicMatcher creates a new deterministic matcher
func NewDeterministicMatcher(db *sql.DB) *DeterministicMatcher {
	return &DeterministicMatcher{db: db}
}

// AddressCandidate represents a potential address match
type AddressCandidate struct {
	UPRN       string  `json:"uprn"`
	LocAddress string  `json:"loc_address"`
	AddrCan    string  `json:"addr_can"`
	Easting    float64 `json:"easting"`
	Northing   float64 `json:"northing"`
	USRN       *string `json:"usrn,omitempty"`
	BLPUClass  *string `json:"blpu_class,omitempty"`
	Status     *string `json:"status,omitempty"`
}

// RunDeterministicMatching performs Stage 1 matching for all unmatched documents
func (dm *DeterministicMatcher) RunDeterministicMatching(runID int64, batchSize int) (int, int, error) {
	totalProcessed := 0
	totalAccepted := 0
	
	fmt.Println("Starting deterministic matching (Stage 1)...")
	
	for {
		// Get batch of unmatched documents
		engine := &MatchEngine{db: dm.db}
		docs, err := engine.GetUnmatchedDocuments(batchSize, "")
		if err != nil {
			return totalProcessed, totalAccepted, fmt.Errorf("failed to get unmatched documents: %w", err)
		}
		
		if len(docs) == 0 {
			break
		}
		
		batchAccepted := 0
		
		for _, doc := range docs {
			accepted := false
			
			// Try legacy UPRN validation first
			if doc.UPRNRaw != nil && strings.TrimSpace(*doc.UPRNRaw) != "" {
				if candidate, found := dm.ValidateLegacyUPRN(strings.TrimSpace(*doc.UPRNRaw)); found {
					err := dm.acceptMatch(engine, runID, doc.SrcID, candidate, "valid_uprn", 1.0, 1.0, "Legacy UPRN validated against LLPG")
					if err == nil {
						accepted = true
						batchAccepted++
					}
				}
			}
			
			// If not matched by legacy UPRN, try exact canonical match
			if !accepted && doc.AddrCan != nil && strings.TrimSpace(*doc.AddrCan) != "" {
				candidates := dm.FindExactCanonicalMatches(strings.TrimSpace(*doc.AddrCan))
				
				if len(candidates) == 1 {
					// Single exact match - auto accept
					err := dm.acceptMatch(engine, runID, doc.SrcID, candidates[0], "addr_exact", 0.99, 0.99, "Exact canonical address match")
					if err == nil {
						accepted = true
						batchAccepted++
					}
				} else if len(candidates) > 1 {
					// Multiple matches - save all for review
					for i, candidate := range candidates {
						result := &MatchResult{
							RunID:         runID,
							SrcID:         doc.SrcID,
							CandidateUPRN: candidate.UPRN,
							Method:        "addr_exact_multiple",
							Score:         0.99,
							Confidence:    0.85, // Lower confidence due to ambiguity
							TieRank:       i + 1,
							Features: map[string]interface{}{
								"exact_canonical_match": true,
								"multiple_candidates":   len(candidates),
								"candidate_address":     candidate.LocAddress,
							},
							Decided:   true,
							Decision:  "needs_review",
							DecidedBy: "system",
							Notes:     fmt.Sprintf("Multiple exact matches found (%d candidates)", len(candidates)),
						}
						
						engine.SaveMatchResult(result)
					}
				}
			}
			
			totalProcessed++
		}
		
		totalAccepted += batchAccepted
		
		if totalProcessed%1000 == 0 {
			fmt.Printf("Processed %d documents, accepted %d matches...\n", totalProcessed, totalAccepted)
		}
	}
	
	fmt.Printf("Deterministic matching complete: processed %d, accepted %d\n", totalProcessed, totalAccepted)
	return totalProcessed, totalAccepted, nil
}

// ValidateLegacyUPRN checks if a legacy UPRN exists in the LLPG
func (dm *DeterministicMatcher) ValidateLegacyUPRN(uprn string) (*AddressCandidate, bool) {
	var candidate AddressCandidate

	err := dm.db.QueryRow(`
		SELECT a.uprn, a.full_address, a.address_canonical,
		       COALESCE(l.easting, 0), COALESCE(l.northing, 0),
		       a.usrn, a.blpu_class, a.status_code
		FROM dim_address a
		LEFT JOIN dim_location l ON a.location_id = l.location_id
		WHERE a.uprn = $1
	`, uprn).Scan(&candidate.UPRN, &candidate.LocAddress, &candidate.AddrCan,
		&candidate.Easting, &candidate.Northing, &candidate.USRN,
		&candidate.BLPUClass, &candidate.Status)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false
		}
		// Log error but continue
		fmt.Printf("Error validating UPRN %s: %v\n", uprn, err)
		return nil, false
	}

	return &candidate, true
}

// FindExactCanonicalMatches finds addresses with exactly matching canonical form
func (dm *DeterministicMatcher) FindExactCanonicalMatches(addrCan string) []*AddressCandidate {
	rows, err := dm.db.Query(`
		SELECT a.uprn, a.full_address, a.address_canonical,
		       COALESCE(l.easting, 0), COALESCE(l.northing, 0),
		       a.usrn, a.blpu_class, a.status_code
		FROM dim_address a
		LEFT JOIN dim_location l ON a.location_id = l.location_id
		WHERE a.address_canonical = $1
		ORDER BY a.uprn
	`, addrCan)

	if err != nil {
		fmt.Printf("Error finding exact canonical matches for '%s': %v\n", addrCan, err)
		return nil
	}
	defer rows.Close()

	var candidates []*AddressCandidate

	for rows.Next() {
		var candidate AddressCandidate
		err := rows.Scan(&candidate.UPRN, &candidate.LocAddress, &candidate.AddrCan,
			&candidate.Easting, &candidate.Northing, &candidate.USRN,
			&candidate.BLPUClass, &candidate.Status)
		if err != nil {
			fmt.Printf("Error scanning candidate: %v\n", err)
			continue
		}
		candidates = append(candidates, &candidate)
	}

	return candidates
}

// acceptMatch is a helper to accept a match and save the result
func (dm *DeterministicMatcher) acceptMatch(engine *MatchEngine, runID, srcID int64, candidate *AddressCandidate, method string, score, confidence float64, notes string) error {
	// Save match result
	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        method,
		Score:         score,
		Confidence:    confidence,
		TieRank:       1,
		Features: map[string]interface{}{
			"uprn":            candidate.UPRN,
			"candidate_address": candidate.LocAddress,
			"easting":         candidate.Easting,
			"northing":        candidate.Northing,
			"usrn":           candidate.USRN,
			"blpu_class":     candidate.BLPUClass,
			"status":         candidate.Status,
		},
		Decided:   true,
		Decision:  "auto_accepted",
		DecidedBy: "system",
		Notes:     notes,
	}
	
	if err := engine.SaveMatchResult(result); err != nil {
		return fmt.Errorf("failed to save match result: %w", err)
	}
	
	// Accept the match
	if err := engine.AcceptMatch(srcID, candidate.UPRN, method, score, confidence, runID, "system"); err != nil {
		return fmt.Errorf("failed to accept match: %w", err)
	}
	
	return nil
}