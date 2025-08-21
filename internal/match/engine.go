package match

import (
	"database/sql"
	"time"

	"github.com/ehdc-llpg/internal/debug"
	"github.com/ehdc-llpg/internal/normalize"
)

// Engine orchestrates the complete address matching process following ADDRESS_MATCHING_ALGORITHM.md
type Engine struct {
	generators       *Generators
	featureComputer  *FeatureComputer
	scorer           *Scorer
	db              *sql.DB
}

// EngineConfig holds configuration for the matching engine
type EngineConfig struct {
	DB        *sql.DB
	VectorDB  VectorDB
	Embedder  Embedder
	Parser    Parser
	Phonetics PhoneticsMatcher
	Weights   *FeatureWeights
	Tiers     *MatchTiers
}

// NewEngine creates a new address matching engine
func NewEngine(config EngineConfig) *Engine {
	weights := config.Weights
	if weights == nil {
		weights = DefaultWeights()
	}

	tiers := config.Tiers
	if tiers == nil {
		tiers = DefaultTiers()
	}

	generators := NewGenerators(config.DB, config.VectorDB, config.Embedder, config.Parser)
	featureComputer := NewFeatureComputer(weights, config.Embedder, config.Phonetics)
	scorer := NewScorerWithConfig(weights, tiers)

	return &Engine{
		generators:      generators,
		featureComputer: featureComputer,
		scorer:          scorer,
		db:              config.DB,
	}
}

// SuggestUPRN performs end-to-end address matching following the sophisticated algorithm
func (e *Engine) SuggestUPRN(localDebug bool, input Input) (Result, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	startTime := time.Now()

	debug.DebugOutput(localDebug, "=== Address Matching Engine Started ===")
	debug.DebugOutput(localDebug, "Raw address: %s", input.RawAddress)
	debug.DebugOutput(localDebug, "Legacy UPRN: %s", input.LegacyUPRN)
	if input.Easting != nil && input.Northing != nil {
		debug.DebugOutput(localDebug, "Coordinates: (%.2f, %.2f)", *input.Easting, *input.Northing)
	}

	// Step 1: Normalize the input address
	debug.DebugOutput(localDebug, "\n=== Step 1: Address Normalization ===")
	canonical, postcode, tokens := normalize.CanonicalAddressDebug(localDebug, input.RawAddress)
	if postcode != "" {
		debug.DebugOutput(localDebug, "Extracted postcode: %s", postcode)
	}

	// Step 2: Generate candidates using multi-tier approach
	debug.DebugOutput(localDebug, "\n=== Step 2: Candidate Generation ===")
	candidates, err := e.generators.Generate(localDebug, input, canonical, tokens)
	if err != nil {
		return Result{}, err
	}

	debug.DebugOutput(localDebug, "Generated %d candidates", len(candidates))

	// Step 3: Compute rich features for each candidate
	debug.DebugOutput(localDebug, "\n=== Step 3: Feature Computation ===")
	for i := range candidates {
		candidates[i].Features = e.featureComputer.ComputeFeatures(
			localDebug, input, canonical, tokens, candidates[i])
	}

	// Step 4: Score all candidates
	debug.DebugOutput(localDebug, "\n=== Step 4: Candidate Scoring ===")
	legacyUPRNValid := e.isLegacyUPRNValid(input.LegacyUPRN, candidates)
	e.scorer.ScoreCandidates(localDebug, candidates, legacyUPRNValid)

	// Step 5: Make decision based on scores and thresholds
	debug.DebugOutput(localDebug, "\n=== Step 5: Decision Making ===")
	decision, acceptedUPRN := e.scorer.MakeDecision(localDebug, candidates)

	processingTime := time.Since(startTime)
	debug.DebugOutput(localDebug, "\n=== Matching Complete ===")
	debug.DebugOutput(localDebug, "Decision: %s", decision)
	debug.DebugOutput(localDebug, "Accepted UPRN: %s", acceptedUPRN)
	debug.DebugOutput(localDebug, "Processing time: %v", processingTime)
	debug.DebugOutput(localDebug, "Final candidate count: %d", len(candidates))

	// Create result
	result := Result{
		Query:          input,
		Candidates:     candidates,
		Decision:       decision,
		AcceptedUPRN:   acceptedUPRN,
		ProcessingTime: processingTime,
		Thresholds: map[string]float64{
			"auto_accept_high":   e.scorer.tiers.AutoAcceptHigh,
			"auto_accept_medium": e.scorer.tiers.AutoAcceptMedium,
			"review":            e.scorer.tiers.ReviewThreshold,
			"min_threshold":     e.scorer.tiers.MinThreshold,
			"winner_margin":     e.scorer.tiers.WinnerMargin,
		},
	}

	return result, nil
}

// BatchProcess processes multiple addresses in batch for efficiency
func (e *Engine) BatchProcess(localDebug bool, inputs []Input, batchSize int) ([]Result, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	debug.DebugOutput(localDebug, "Batch processing %d addresses", len(inputs))

	results := make([]Result, len(inputs))
	
	for i, input := range inputs {
		result, err := e.SuggestUPRN(false, input) // Disable debug for batch to reduce noise
		if err != nil {
			debug.DebugOutput(localDebug, "Error processing address %d: %v", i, err)
			// Continue processing other addresses
			result = Result{
				Query:    input,
				Decision: "error",
			}
		}
		results[i] = result

		if localDebug && (i+1)%batchSize == 0 {
			debug.DebugOutput(localDebug, "Processed %d/%d addresses", i+1, len(inputs))
		}
	}

	// Summary statistics
	stats := e.calculateBatchStats(results)
	debug.DebugOutput(localDebug, "Batch complete - Auto: %d, Review: %d, Reject: %d, Error: %d", 
		stats.AutoAccept, stats.Review, stats.Reject, stats.Error)

	return results, nil
}

// SaveResults persists matching results to the database following PROJECT_SPECIFICATION.md schema
func (e *Engine) SaveResults(localDebug bool, results []Result, runLabel string) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	// Create match run
	var runID int64
	err := e.db.QueryRow(`
		INSERT INTO match_run (run_label, notes)
		VALUES ($1, $2)
		RETURNING run_id
	`, runLabel, "Go-based matching engine with ADDRESS_MATCHING_ALGORITHM.md implementation").Scan(&runID)
	
	if err != nil {
		return err
	}

	debug.DebugOutput(localDebug, "Created match run %d: %s", runID, runLabel)

	// Save results in batches
	tx, err := e.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Prepare statements
	resultStmt, err := tx.Prepare(`
		INSERT INTO match_result (run_id, src_id, candidate_uprn, method, score, tie_rank, decided, decision)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`)
	if err != nil {
		return err
	}
	defer resultStmt.Close()

	acceptedStmt, err := tx.Prepare(`
		INSERT INTO match_accepted (src_id, uprn, method, score, run_id, accepted_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (src_id) DO UPDATE SET
			uprn = EXCLUDED.uprn,
			method = EXCLUDED.method,
			score = EXCLUDED.score,
			run_id = EXCLUDED.run_id,
			accepted_at = now()
	`)
	if err != nil {
		return err
	}
	defer acceptedStmt.Close()

	saved := 0
	accepted := 0

	for _, result := range results {
		if len(result.Candidates) == 0 {
			continue
		}

		// Save top candidates as match_result records
		for rank, candidate := range result.Candidates {
			if rank >= 10 { // Limit to top 10 candidates
				break
			}

			methods := "unknown"
			if len(candidate.Methods) > 0 {
				methods = candidate.Methods[0] // Use primary method
			}

			decided := result.Decision == "auto_accept"
			decision := result.Decision
			if decision == "auto_accept" {
				decision = "accepted"
			}

			// Use src_id from result query
			srcID := result.Query.SrcID
			
			_, err = resultStmt.Exec(runID, srcID, candidate.UPRN, methods, candidate.Score, rank+1, decided, decision)
			if err != nil {
				return err
			}
			saved++
		}

		// Save accepted matches
		if result.Decision == "auto_accept" && result.AcceptedUPRN != "" {
			srcID := result.Query.SrcID
			topScore := float64(0)
			topMethod := "unknown"
			
			if len(result.Candidates) > 0 {
				topScore = result.Candidates[0].Score
				if len(result.Candidates[0].Methods) > 0 {
					topMethod = result.Candidates[0].Methods[0]
				}
			}

			_, err = acceptedStmt.Exec(srcID, result.AcceptedUPRN, topMethod, topScore, runID, "system")
			if err != nil {
				return err
			}
			accepted++
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	debug.DebugOutput(localDebug, "Saved %d match results, %d accepted matches", saved, accepted)
	return nil
}

// isLegacyUPRNValid checks if the legacy UPRN matches any of the candidates
func (e *Engine) isLegacyUPRNValid(legacyUPRN string, candidates []Candidate) bool {
	if legacyUPRN == "" {
		return false
	}

	for _, candidate := range candidates {
		if candidate.UPRN == legacyUPRN {
			return true
		}
	}
	return false
}

// BatchStats holds statistics for batch processing
type BatchStats struct {
	AutoAccept int
	Review     int
	Reject     int
	Error      int
	Total      int
}

// calculateBatchStats computes summary statistics for a batch of results
func (e *Engine) calculateBatchStats(results []Result) BatchStats {
	stats := BatchStats{Total: len(results)}

	for _, result := range results {
		switch result.Decision {
		case "auto_accept":
			stats.AutoAccept++
		case "review":
			stats.Review++
		case "reject":
			stats.Reject++
		case "error":
			stats.Error++
		}
	}

	return stats
}

// GetExplanation provides detailed explanation for a matching result
func (e *Engine) GetExplanation(result Result) map[string]interface{} {
	explanation := map[string]interface{}{
		"query":           result.Query,
		"decision":        result.Decision,
		"accepted_uprn":   result.AcceptedUPRN,
		"processing_time": result.ProcessingTime.String(),
		"candidate_count": len(result.Candidates),
		"thresholds":      result.Thresholds,
	}

	if len(result.Candidates) > 0 {
		topCandidate := result.Candidates[0]
		legacyValid := e.isLegacyUPRNValid(result.Query.LegacyUPRN, result.Candidates)
		explanation["top_candidate"] = e.scorer.GetExplanation(topCandidate, legacyValid)
	}

	return explanation
}