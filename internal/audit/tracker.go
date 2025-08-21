package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ehdc-llpg/internal/debug"
	"github.com/ehdc-llpg/internal/match"
)

// Tracker manages audit trails and decision tracking for address matching
type Tracker struct {
	db *sql.DB
}

// NewTracker creates a new audit tracker
func NewTracker(db *sql.DB) *Tracker {
	return &Tracker{db: db}
}

// AuditDecision records a matching decision in the audit trail
type AuditDecision struct {
	SrcID           int64
	UPRN            string
	Decision        string // "accepted", "rejected", "needs_review"
	Method          string
	Score           float64
	Features        map[string]interface{}
	DecidedBy       string
	DecidedAt       time.Time
	RunID           int64
	Candidates      []match.Candidate
	ProcessingTime  time.Duration
	Explanation     map[string]interface{}
}

// RecordDecision saves a matching decision to the audit trail
func (t *Tracker) RecordDecision(localDebug bool, decision AuditDecision) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	debug.DebugOutput(localDebug, "Recording decision for src_id %d: %s -> %s", 
		decision.SrcID, decision.Decision, decision.UPRN)

	// Start transaction
	tx, err := t.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Record in match_result table
	var matchID int64
	err = tx.QueryRow(`
		INSERT INTO match_result (
			run_id, src_id, candidate_uprn, method, score, tie_rank, 
			decided, decision, decided_by, decided_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING match_id
	`, decision.RunID, decision.SrcID, decision.UPRN, decision.Method, decision.Score, 1,
		true, decision.Decision, decision.DecidedBy, decision.DecidedAt).Scan(&matchID)
	
	if err != nil {
		return fmt.Errorf("failed to insert match result: %w", err)
	}

	debug.DebugOutput(localDebug, "Created match_result record %d", matchID)

	// Record all candidates (top 5) as alternative options
	for rank, candidate := range decision.Candidates {
		if rank >= 5 { // Limit to top 5 for audit
			break
		}
		if rank == 0 && candidate.UPRN == decision.UPRN {
			continue // Skip the accepted candidate (already recorded above)
		}

		methods := "unknown"
		if len(candidate.Methods) > 0 {
			methods = candidate.Methods[0]
		}

		_, err = tx.Exec(`
			INSERT INTO match_result (
				run_id, src_id, candidate_uprn, method, score, tie_rank,
				decided, decision, decided_by, decided_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, decision.RunID, decision.SrcID, candidate.UPRN, methods, candidate.Score, rank+1,
			false, "not_selected", decision.DecidedBy, decision.DecidedAt)
		
		if err != nil {
			debug.DebugOutput(localDebug, "Warning: failed to record candidate %d: %v", rank, err)
		}
	}

	// If decision is "accepted", also record in match_accepted table
	if decision.Decision == "accepted" {
		_, err = tx.Exec(`
			INSERT INTO match_accepted (src_id, uprn, method, score, run_id, accepted_by, accepted_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (src_id) DO UPDATE SET
				uprn = EXCLUDED.uprn,
				method = EXCLUDED.method,
				score = EXCLUDED.score,
				run_id = EXCLUDED.run_id,
				accepted_by = EXCLUDED.accepted_by,
				accepted_at = EXCLUDED.accepted_at
		`, decision.SrcID, decision.UPRN, decision.Method, decision.Score, 
			decision.RunID, decision.DecidedBy, decision.DecidedAt)
		
		if err != nil {
			return fmt.Errorf("failed to insert match accepted: %w", err)
		}

		debug.DebugOutput(localDebug, "Recorded accepted match for src_id %d", decision.SrcID)
	}

	// Record detailed audit information in separate audit table
	err = t.recordDetailedAudit(tx, localDebug, matchID, decision)
	if err != nil {
		return fmt.Errorf("failed to record detailed audit: %w", err)
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	debug.DebugOutput(localDebug, "Successfully recorded decision audit for src_id %d", decision.SrcID)
	return nil
}

// recordDetailedAudit stores detailed audit information
func (t *Tracker) recordDetailedAudit(tx *sql.Tx, localDebug bool, matchID int64, decision AuditDecision) error {
	// Create detailed audit table if it doesn't exist
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS match_audit (
			audit_id        bigserial PRIMARY KEY,
			match_id        bigint REFERENCES match_result(match_id),
			src_id          bigint NOT NULL,
			decision_type   text NOT NULL,
			features_json   jsonb,
			candidates_json jsonb,
			explanation_json jsonb,
			processing_time_ms bigint,
			created_at      timestamptz DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create audit table: %w", err)
	}

	// Serialize complex data to JSON
	featuresJSON, _ := json.Marshal(decision.Features)
	candidatesJSON, _ := json.Marshal(decision.Candidates)
	explanationJSON, _ := json.Marshal(decision.Explanation)

	_, err = tx.Exec(`
		INSERT INTO match_audit (
			match_id, src_id, decision_type, features_json, candidates_json, 
			explanation_json, processing_time_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, matchID, decision.SrcID, decision.Decision, featuresJSON, candidatesJSON, 
		explanationJSON, decision.ProcessingTime.Milliseconds())

	if err != nil {
		return fmt.Errorf("failed to insert detailed audit: %w", err)
	}

	debug.DebugOutput(localDebug, "Recorded detailed audit for match_id %d", matchID)
	return nil
}

// RecordManualOverride records a manual decision override by a reviewer
func (t *Tracker) RecordManualOverride(localDebug bool, srcID int64, uprn string, reason string, reviewerID string) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	debug.DebugOutput(localDebug, "Recording manual override for src_id %d: %s", srcID, reason)

	// Insert override record
	_, err := t.db.Exec(`
		INSERT INTO match_override (src_id, uprn, reason, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, srcID, uprn, reason, reviewerID, time.Now())

	if err != nil {
		return fmt.Errorf("failed to record manual override: %w", err)
	}

	// Update match_accepted if UPRN provided
	if uprn != "" {
		_, err = t.db.Exec(`
			INSERT INTO match_accepted (src_id, uprn, method, score, run_id, accepted_by, accepted_at)
			VALUES ($1, $2, 'manual_override', 1.0, 0, $3, $4)
			ON CONFLICT (src_id) DO UPDATE SET
				uprn = EXCLUDED.uprn,
				method = EXCLUDED.method,
				accepted_by = EXCLUDED.accepted_by,
				accepted_at = EXCLUDED.accepted_at
		`, srcID, uprn, reviewerID, time.Now())

		if err != nil {
			return fmt.Errorf("failed to update match_accepted: %w", err)
		}

		debug.DebugOutput(localDebug, "Updated accepted match with manual override")
	}

	debug.DebugOutput(localDebug, "Successfully recorded manual override for src_id %d", srcID)
	return nil
}

// GetDecisionHistory retrieves the decision history for a source document
func (t *Tracker) GetDecisionHistory(localDebug bool, srcID int64) ([]DecisionHistoryEntry, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	rows, err := t.db.Query(`
		SELECT 
			mr.match_id,
			mr.run_id,
			mr.candidate_uprn,
			mr.method,
			mr.score,
			mr.tie_rank,
			mr.decided,
			mr.decision,
			mr.decided_by,
			mr.decided_at,
			ma.features_json,
			ma.explanation_json,
			ma.processing_time_ms,
			r.run_label,
			da.locaddress as candidate_address
		FROM match_result mr
		LEFT JOIN match_audit ma ON ma.match_id = mr.match_id
		LEFT JOIN match_run r ON r.run_id = mr.run_id
		LEFT JOIN dim_address da ON da.uprn = mr.candidate_uprn
		WHERE mr.src_id = $1
		ORDER BY mr.decided_at DESC, mr.tie_rank ASC
	`, srcID)

	if err != nil {
		return nil, fmt.Errorf("failed to query decision history: %w", err)
	}
	defer rows.Close()

	var history []DecisionHistoryEntry
	for rows.Next() {
		var entry DecisionHistoryEntry
		var featuresJSON, explanationJSON sql.NullString
		var processingTimeMS sql.NullInt64

		err := rows.Scan(
			&entry.MatchID,
			&entry.RunID,
			&entry.CandidateUPRN,
			&entry.Method,
			&entry.Score,
			&entry.TieRank,
			&entry.Decided,
			&entry.Decision,
			&entry.DecidedBy,
			&entry.DecidedAt,
			&featuresJSON,
			&explanationJSON,
			&processingTimeMS,
			&entry.RunLabel,
			&entry.CandidateAddress,
		)
		if err != nil {
			debug.DebugOutput(localDebug, "Error scanning history row: %v", err)
			continue
		}

		// Parse JSON fields
		if featuresJSON.Valid {
			json.Unmarshal([]byte(featuresJSON.String), &entry.Features)
		}
		if explanationJSON.Valid {
			json.Unmarshal([]byte(explanationJSON.String), &entry.Explanation)
		}
		if processingTimeMS.Valid {
			entry.ProcessingTime = time.Duration(processingTimeMS.Int64) * time.Millisecond
		}

		history = append(history, entry)
	}

	debug.DebugOutput(localDebug, "Retrieved %d decision history entries for src_id %d", len(history), srcID)
	return history, nil
}

// GetOverrideHistory retrieves manual override history for a source document
func (t *Tracker) GetOverrideHistory(localDebug bool, srcID int64) ([]OverrideHistoryEntry, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	rows, err := t.db.Query(`
		SELECT 
			mo.override_id,
			mo.uprn,
			mo.reason,
			mo.created_by,
			mo.created_at,
			da.locaddress as uprn_address
		FROM match_override mo
		LEFT JOIN dim_address da ON da.uprn = mo.uprn
		WHERE mo.src_id = $1
		ORDER BY mo.created_at DESC
	`, srcID)

	if err != nil {
		return nil, fmt.Errorf("failed to query override history: %w", err)
	}
	defer rows.Close()

	var history []OverrideHistoryEntry
	for rows.Next() {
		var entry OverrideHistoryEntry
		err := rows.Scan(
			&entry.OverrideID,
			&entry.UPRN,
			&entry.Reason,
			&entry.CreatedBy,
			&entry.CreatedAt,
			&entry.UPRNAddress,
		)
		if err != nil {
			debug.DebugOutput(localDebug, "Error scanning override row: %v", err)
			continue
		}
		history = append(history, entry)
	}

	debug.DebugOutput(localDebug, "Retrieved %d override history entries for src_id %d", len(history), srcID)
	return history, nil
}

// GetMatchingStatistics retrieves statistics for matching runs
func (t *Tracker) GetMatchingStatistics(localDebug bool, runID int64) (*MatchingStats, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	stats := &MatchingStats{RunID: runID}

	// Get run information
	err := t.db.QueryRow(`
		SELECT run_label, run_started_at, notes
		FROM match_run
		WHERE run_id = $1
	`, runID).Scan(&stats.RunLabel, &stats.RunStartedAt, &stats.Notes)

	if err != nil {
		return nil, fmt.Errorf("failed to get run info: %w", err)
	}

	// Get decision statistics
	rows, err := t.db.Query(`
		SELECT 
			decision,
			method,
			COUNT(*) as count,
			AVG(score) as avg_score,
			MIN(score) as min_score,
			MAX(score) as max_score
		FROM match_result
		WHERE run_id = $1 AND tie_rank = 1
		GROUP BY decision, method
		ORDER BY decision, method
	`, runID)

	if err != nil {
		return nil, fmt.Errorf("failed to query decision stats: %w", err)
	}
	defer rows.Close()

	stats.DecisionBreakdown = make(map[string]DecisionStats)
	stats.MethodBreakdown = make(map[string]MethodStats)

	for rows.Next() {
		var decision, method string
		var count int64
		var avgScore, minScore, maxScore float64

		err := rows.Scan(&decision, &method, &count, &avgScore, &minScore, &maxScore)
		if err != nil {
			continue
		}

		// Update decision breakdown
		if decisionStats, exists := stats.DecisionBreakdown[decision]; exists {
			decisionStats.Count += count
		} else {
			stats.DecisionBreakdown[decision] = DecisionStats{
				Decision: decision,
				Count:    count,
			}
		}

		// Update method breakdown
		if methodStats, exists := stats.MethodBreakdown[method]; exists {
			methodStats.Count += count
			methodStats.AvgScore = (methodStats.AvgScore + avgScore) / 2
		} else {
			stats.MethodBreakdown[method] = MethodStats{
				Method:   method,
				Count:    count,
				AvgScore: avgScore,
				MinScore: minScore,
				MaxScore: maxScore,
			}
		}

		stats.TotalProcessed += count
	}

	debug.DebugOutput(localDebug, "Retrieved statistics for run %d: %d total processed", runID, stats.TotalProcessed)
	return stats, nil
}

// Data structures for audit trail

type DecisionHistoryEntry struct {
	MatchID          int64                  `json:"match_id"`
	RunID            int64                  `json:"run_id"`
	RunLabel         string                 `json:"run_label"`
	CandidateUPRN    string                 `json:"candidate_uprn"`
	CandidateAddress string                 `json:"candidate_address"`
	Method           string                 `json:"method"`
	Score            float64                `json:"score"`
	TieRank          int                    `json:"tie_rank"`
	Decided          bool                   `json:"decided"`
	Decision         string                 `json:"decision"`
	DecidedBy        string                 `json:"decided_by"`
	DecidedAt        time.Time              `json:"decided_at"`
	Features         map[string]interface{} `json:"features"`
	Explanation      map[string]interface{} `json:"explanation"`
	ProcessingTime   time.Duration          `json:"processing_time"`
}

type OverrideHistoryEntry struct {
	OverrideID  int64     `json:"override_id"`
	UPRN        string    `json:"uprn"`
	UPRNAddress string    `json:"uprn_address"`
	Reason      string    `json:"reason"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

type MatchingStats struct {
	RunID              int64                    `json:"run_id"`
	RunLabel           string                   `json:"run_label"`
	RunStartedAt       time.Time                `json:"run_started_at"`
	Notes              string                   `json:"notes"`
	TotalProcessed     int64                    `json:"total_processed"`
	DecisionBreakdown  map[string]DecisionStats `json:"decision_breakdown"`
	MethodBreakdown    map[string]MethodStats   `json:"method_breakdown"`
}

type DecisionStats struct {
	Decision string `json:"decision"`
	Count    int64  `json:"count"`
}

type MethodStats struct {
	Method   string  `json:"method"`
	Count    int64   `json:"count"`
	AvgScore float64 `json:"avg_score"`
	MinScore float64 `json:"min_score"`
	MaxScore float64 `json:"max_score"`
}