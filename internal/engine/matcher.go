package engine

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// MatchEngine handles address matching operations
type MatchEngine struct {
	db *sql.DB
}

// NewMatchEngine creates a new match engine
func NewMatchEngine(db *sql.DB) *MatchEngine {
	return &MatchEngine{db: db}
}

// MatchRun represents a matching run record
type MatchRun struct {
	RunID           int64     `json:"run_id"`
	RunStartedAt    time.Time `json:"run_started_at"`
	RunCompletedAt  *time.Time `json:"run_completed_at,omitempty"`
	RunLabel        string    `json:"run_label"`
	AlgorithmVersion string   `json:"algorithm_version"`
	Notes           string    `json:"notes"`
	TotalProcessed  int       `json:"total_processed"`
	AutoAccepted    int       `json:"auto_accepted"`
	NeedsReview     int       `json:"needs_review"`
	Rejected        int       `json:"rejected"`
}

// MatchResult represents a candidate match result
type MatchResult struct {
	MatchID      int64                  `json:"match_id"`
	RunID        int64                  `json:"run_id"`
	SrcID        int64                  `json:"src_id"`
	CandidateUPRN string                `json:"candidate_uprn"`
	Method       string                 `json:"method"`
	Score        float64                `json:"score"`
	Confidence   float64                `json:"confidence"`
	TieRank      int                    `json:"tie_rank"`
	Features     map[string]interface{} `json:"features"`
	Decided      bool                   `json:"decided"`
	Decision     string                 `json:"decision"`
	DecidedBy    string                 `json:"decided_by"`
	DecidedAt    *time.Time             `json:"decided_at,omitempty"`
	Notes        string                 `json:"notes"`
}

// CreateMatchRun creates a new matching run
func (me *MatchEngine) CreateMatchRun(label, algorithmVersion, notes string) (*MatchRun, error) {
	run := &MatchRun{
		RunLabel:         label,
		AlgorithmVersion: algorithmVersion,
		Notes:           notes,
		RunStartedAt:    time.Now(),
	}

	err := me.db.QueryRow(`
		INSERT INTO match_run (run_label, algorithm_version, notes, run_started_at)
		VALUES ($1, $2, $3, $4)
		RETURNING run_id
	`, label, algorithmVersion, notes, run.RunStartedAt).Scan(&run.RunID)

	if err != nil {
		return nil, fmt.Errorf("failed to create match run: %w", err)
	}

	fmt.Printf("Created matching run %d: %s\n", run.RunID, label)
	return run, nil
}

// CompleteMatchRun marks a matching run as completed with statistics
func (me *MatchEngine) CompleteMatchRun(runID int64, totalProcessed, autoAccepted, needsReview, rejected int) error {
	now := time.Now()
	
	_, err := me.db.Exec(`
		UPDATE match_run 
		SET run_completed_at = $1, total_processed = $2, auto_accepted = $3, needs_review = $4, rejected = $5
		WHERE run_id = $6
	`, now, totalProcessed, autoAccepted, needsReview, rejected, runID)

	if err != nil {
		return fmt.Errorf("failed to complete match run: %w", err)
	}

	fmt.Printf("Completed matching run %d: processed=%d, accepted=%d, review=%d, rejected=%d\n", 
		runID, totalProcessed, autoAccepted, needsReview, rejected)
	
	return nil
}

// SaveMatchResult saves a match result to the database
func (me *MatchEngine) SaveMatchResult(result *MatchResult) error {
	featuresJSON, err := json.Marshal(result.Features)
	if err != nil {
		return fmt.Errorf("failed to marshal features: %w", err)
	}

	_, err = me.db.Exec(`
		INSERT INTO match_result (
			run_id, src_id, candidate_uprn, method, score, confidence, 
			tie_rank, features, decided, decision, decided_by, decided_at, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, result.RunID, result.SrcID, result.CandidateUPRN, result.Method, 
		result.Score, result.Confidence, result.TieRank, featuresJSON,
		result.Decided, result.Decision, result.DecidedBy, result.DecidedAt, result.Notes)

	return err
}

// AcceptMatch records an accepted match in match_accepted table
func (me *MatchEngine) AcceptMatch(srcID int64, uprn, method string, score, confidence float64, runID int64, acceptedBy string) error {
	_, err := me.db.Exec(`
		INSERT INTO match_accepted (src_id, uprn, method, score, confidence, run_id, accepted_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (src_id) DO UPDATE SET
			uprn = EXCLUDED.uprn,
			method = EXCLUDED.method,
			score = EXCLUDED.score,
			confidence = EXCLUDED.confidence,
			run_id = EXCLUDED.run_id,
			accepted_by = EXCLUDED.accepted_by,
			accepted_at = now()
	`, srcID, uprn, method, score, confidence, runID, acceptedBy)

	return err
}

// GetUnmatchedDocuments returns source documents without accepted matches
func (me *MatchEngine) GetUnmatchedDocuments(limit int, sourceType string) ([]SourceDocument, error) {
	var query string
	var args []interface{}

	if sourceType != "" {
		query = `
			SELECT s.src_id, s.source_type, s.job_number, s.filepath, s.external_ref,
				   s.doc_type, s.doc_date, s.raw_address, s.addr_can, s.postcode_text,
				   s.uprn_raw, s.easting_raw, s.northing_raw
			FROM src_document s
			LEFT JOIN match_accepted m ON m.src_id = s.src_id
			WHERE m.src_id IS NULL AND s.source_type = $1
			ORDER BY s.src_id
			LIMIT $2
		`
		args = []interface{}{sourceType, limit}
	} else {
		query = `
			SELECT s.src_id, s.source_type, s.job_number, s.filepath, s.external_ref,
				   s.doc_type, s.doc_date, s.raw_address, s.addr_can, s.postcode_text,
				   s.uprn_raw, s.easting_raw, s.northing_raw
			FROM src_document s
			LEFT JOIN match_accepted m ON m.src_id = s.src_id
			WHERE m.src_id IS NULL
			ORDER BY s.src_id
			LIMIT $1
		`
		args = []interface{}{limit}
	}

	rows, err := me.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query unmatched documents: %w", err)
	}
	defer rows.Close()

	var docs []SourceDocument
	for rows.Next() {
		var doc SourceDocument
		err := rows.Scan(
			&doc.SrcID, &doc.SourceType, &doc.JobNumber, &doc.Filepath, &doc.ExternalRef,
			&doc.DocType, &doc.DocDate, &doc.RawAddress, &doc.AddrCan, &doc.PostcodeText,
			&doc.UPRNRaw, &doc.EastingRaw, &doc.NorthingRaw,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan document: %w", err)
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// SourceDocument represents a source document for matching
type SourceDocument struct {
	SrcID        int64      `json:"src_id"`
	SourceType   string     `json:"source_type"`
	JobNumber    *string    `json:"job_number,omitempty"`
	Filepath     *string    `json:"filepath,omitempty"`
	ExternalRef  *string    `json:"external_ref,omitempty"`
	DocType      *string    `json:"doc_type,omitempty"`
	DocDate      *time.Time `json:"doc_date,omitempty"`
	RawAddress   string     `json:"raw_address"`
	AddrCan      *string    `json:"addr_can,omitempty"`
	PostcodeText *string    `json:"postcode_text,omitempty"`
	UPRNRaw      *string    `json:"uprn_raw,omitempty"`
	EastingRaw   *float64   `json:"easting_raw,omitempty"`
	NorthingRaw  *float64   `json:"northing_raw,omitempty"`
}