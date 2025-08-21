package engine

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VectorMatcher handles semantic/vector-based matching using embeddings
type VectorMatcher struct {
	db             *sql.DB
	embeddingAPI   string
	vectorDB       VectorDatabase
	embeddingModel string
}

// VectorDatabase interface for vector operations
type VectorDatabase interface {
	Store(id string, vector []float32, metadata map[string]interface{}) error
	Search(vector []float32, limit int, minScore float64) ([]*VectorMatch, error)
	Initialize() error
}

// VectorMatch represents a vector-based match
type VectorMatch struct {
	ID       string
	Score    float64
	Metadata map[string]interface{}
}

// VectorCandidate represents a semantic match candidate
type VectorCandidate struct {
	UPRN            string
	Address         string
	CanonicalAddr   string
	SemanticScore   float64
	CombinedScore   float64
	Features        map[string]interface{}
}

// EmbeddingResponse represents response from embedding API
type EmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

// NewVectorMatcher creates a new vector matcher
func NewVectorMatcher(db *sql.DB, embeddingAPI string) *VectorMatcher {
	return &VectorMatcher{
		db:             db,
		embeddingAPI:   embeddingAPI,
		embeddingModel: "all-MiniLM-L6-v2", // Default lightweight model
		vectorDB:       NewInMemoryVectorDB(),
	}
}

// RunVectorMatching performs semantic/vector-based matching
func (vm *VectorMatcher) RunVectorMatching(runID int64, batchSize int, minSimilarity float64) (int, int, int, error) {
	if minSimilarity <= 0 {
		minSimilarity = 0.70 // Default semantic similarity threshold
	}

	startTime := time.Now()
	totalProcessed := 0
	totalAccepted := 0
	totalNeedsReview := 0

	fmt.Printf("Starting vector/semantic matching (min similarity: %.2f)...\n", minSimilarity)
	
	// Initialize vector database
	if err := vm.vectorDB.Initialize(); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to initialize vector database: %w", err)
	}

	// Pre-populate vector database with LLPG addresses if empty
	if err := vm.indexLLPGAddresses(); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to index LLPG addresses: %w", err)
	}

	engine := &MatchEngine{db: vm.db}

	for {
		// Get unmatched documents
		docs, err := vm.getUnmatchedForVector(batchSize)
		if err != nil {
			return totalProcessed, totalAccepted, totalNeedsReview, fmt.Errorf("failed to get documents: %w", err)
		}

		if len(docs) == 0 {
			break
		}

		for _, doc := range docs {
			totalProcessed++

			if doc.AddrCan == nil || *doc.AddrCan == "" || *doc.AddrCan == "N A" {
				continue
			}

			// Find semantic candidates
			candidates, err := vm.findSemanticCandidates(doc, minSimilarity)
			if err != nil {
				fmt.Printf("Error finding semantic candidates for doc %d: %v\n", doc.SrcID, err)
				continue
			}

			if len(candidates) == 0 {
				continue
			}

			// Make decision based on semantic similarity
			bestCandidate := candidates[0]

			if bestCandidate.CombinedScore >= 0.85 {
				// High semantic similarity - auto accept
				err = vm.acceptMatch(engine, runID, doc.SrcID, bestCandidate)
				if err == nil {
					totalAccepted++
				}
			} else if bestCandidate.CombinedScore >= minSimilarity {
				// Medium similarity - needs review
				for i, candidate := range candidates {
					if i >= 3 {
						break
					}
					vm.saveForReview(engine, runID, doc.SrcID, candidate, i+1)
				}
				totalNeedsReview++
			}
		}

		if totalProcessed%100 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(totalProcessed) / elapsed.Seconds()
			fmt.Printf("Processed %d documents (%.1f/sec), accepted %d, needs review %d...\n",
				totalProcessed, rate, totalAccepted, totalNeedsReview)
		}
	}

	fmt.Printf("Vector matching complete: processed %d, accepted %d, needs review %d\n",
		totalProcessed, totalAccepted, totalNeedsReview)
	return totalProcessed, totalAccepted, totalNeedsReview, nil
}

// getUnmatchedForVector gets unmatched documents suitable for vector matching
func (vm *VectorMatcher) getUnmatchedForVector(limit int) ([]SourceDocument, error) {
	rows, err := vm.db.Query(`
		SELECT s.src_id, s.source_type, s.raw_address, s.addr_can, s.postcode_text,
			   s.easting_raw, s.northing_raw, s.uprn_raw
		FROM src_document s
		LEFT JOIN match_accepted m ON m.src_id = s.src_id
		WHERE m.src_id IS NULL
		  AND s.addr_can IS NOT NULL
		  AND s.addr_can != 'N A'
		  AND s.addr_can != ''
		  AND length(s.addr_can) > 15  -- Reasonable address length for semantic matching
		ORDER BY s.src_id
		LIMIT $1
	`, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []SourceDocument
	for rows.Next() {
		var doc SourceDocument
		err := rows.Scan(
			&doc.SrcID, &doc.SourceType, &doc.RawAddress, &doc.AddrCan,
			&doc.PostcodeText, &doc.EastingRaw, &doc.NorthingRaw, &doc.UPRNRaw,
		)
		if err != nil {
			continue
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// indexLLPGAddresses pre-indexes all LLPG addresses for vector search
func (vm *VectorMatcher) indexLLPGAddresses() error {
	fmt.Println("Indexing LLPG addresses for vector search (this may take a few minutes)...")

	rows, err := vm.db.Query(`
		SELECT uprn, locaddress, addr_can
		FROM dim_address
		WHERE addr_can IS NOT NULL AND addr_can != ''
		ORDER BY uprn
	`)
	if err != nil {
		return fmt.Errorf("failed to query LLPG addresses: %w", err)
	}
	defer rows.Close()

	count := 0
	batchSize := 50 // Process in batches to avoid overwhelming the embedding API
	var batch []struct {
		UPRN    string
		Address string
		CanAddr string
	}

	for rows.Next() {
		var uprn, address, canAddr string
		if err := rows.Scan(&uprn, &address, &canAddr); err != nil {
			continue
		}

		batch = append(batch, struct {
			UPRN    string
			Address string
			CanAddr string
		}{uprn, address, canAddr})

		if len(batch) >= batchSize {
			if err := vm.processBatch(batch); err != nil {
				fmt.Printf("Warning: failed to process batch: %v\n", err)
			}
			batch = nil
			count += batchSize
			if count%500 == 0 {
				fmt.Printf("Indexed %d LLPG addresses...\n", count)
			}
		}
	}

	// Process remaining addresses
	if len(batch) > 0 {
		if err := vm.processBatch(batch); err != nil {
			fmt.Printf("Warning: failed to process final batch: %v\n", err)
		}
		count += len(batch)
	}

	fmt.Printf("Completed indexing %d LLPG addresses\n", count)
	return nil
}

// processBatch processes a batch of addresses for vector indexing
func (vm *VectorMatcher) processBatch(batch []struct {
	UPRN    string
	Address string
	CanAddr string
}) error {
	for _, item := range batch {
		// Get embedding for canonical address
		embedding, err := vm.getEmbedding(item.CanAddr)
		if err != nil {
			continue // Skip this address if embedding fails
		}

		// Store in vector database
		metadata := map[string]interface{}{
			"uprn":            item.UPRN,
			"address":         item.Address,
			"canonical_addr":  item.CanAddr,
		}

		if err := vm.vectorDB.Store(item.UPRN, embedding, metadata); err != nil {
			continue // Skip if storage fails
		}
	}

	// Small delay to be respectful to embedding API
	time.Sleep(100 * time.Millisecond)
	return nil
}

// findSemanticCandidates finds candidates using semantic similarity
func (vm *VectorMatcher) findSemanticCandidates(doc SourceDocument, minSimilarity float64) ([]*VectorCandidate, error) {
	sourceAddr := *doc.AddrCan

	// Get embedding for source address
	embedding, err := vm.getEmbedding(sourceAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding: %w", err)
	}

	// Search vector database
	matches, err := vm.vectorDB.Search(embedding, 10, minSimilarity)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	var candidates []*VectorCandidate
	for _, match := range matches {
		candidate := &VectorCandidate{
			UPRN:          match.Metadata["uprn"].(string),
			Address:       match.Metadata["address"].(string),
			CanonicalAddr: match.Metadata["canonical_addr"].(string),
			SemanticScore: match.Score,
			Features:      make(map[string]interface{}),
		}

		// Calculate combined score (semantic + text similarity)
		candidate.CombinedScore = vm.calculateCombinedScore(sourceAddr, candidate)

		// Store features for explainability
		candidate.Features = map[string]interface{}{
			"semantic_score":     candidate.SemanticScore,
			"combined_score":     candidate.CombinedScore,
			"source_address":     sourceAddr,
			"target_address":     candidate.Address,
			"matching_method":    "vector_semantic",
			"embedding_model":    vm.embeddingModel,
		}

		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

// calculateCombinedScore combines semantic and text similarity
func (vm *VectorMatcher) calculateCombinedScore(sourceAddr string, candidate *VectorCandidate) float64 {
	// Weight semantic similarity at 70%, text similarity at 30%
	semanticWeight := 0.70
	textWeight := 0.30

	// Calculate text similarity using trigram (if available)
	var textSim float64
	err := vm.db.QueryRow(`SELECT similarity($1, $2)`, sourceAddr, candidate.CanonicalAddr).Scan(&textSim)
	if err != nil {
		textSim = 0.5 // Default if similarity function not available
	}

	combinedScore := semanticWeight*candidate.SemanticScore + textWeight*textSim

	// Bonus for exact token matches
	sourceTokens := strings.Fields(strings.ToUpper(sourceAddr))
	targetTokens := strings.Fields(strings.ToUpper(candidate.CanonicalAddr))
	
	exactMatches := 0
	for _, st := range sourceTokens {
		for _, tt := range targetTokens {
			if st == tt {
				exactMatches++
				break
			}
		}
	}

	if exactMatches > 0 {
		tokenBonus := float64(exactMatches) / float64(len(sourceTokens)) * 0.10
		combinedScore += tokenBonus
	}

	// Clamp to [0, 1]
	if combinedScore > 1.0 {
		combinedScore = 1.0
	}

	return combinedScore
}

// getEmbedding gets embedding vector for a text string
func (vm *VectorMatcher) getEmbedding(text string) ([]float32, error) {
	if vm.embeddingAPI == "" {
		// Fallback: return mock embedding for testing
		return vm.getMockEmbedding(text), nil
	}

	// Prepare request
	reqBody := map[string]interface{}{
		"text":  text,
		"model": vm.embeddingModel,
	}
	jsonData, _ := json.Marshal(reqBody)

	// Call embedding API
	resp, err := http.Post(vm.embeddingAPI, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("embedding API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedding response: %w", err)
	}

	var embeddingResp EmbeddingResponse
	if err := json.Unmarshal(body, &embeddingResp); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %w", err)
	}

	if embeddingResp.Error != "" {
		return nil, fmt.Errorf("embedding API error: %s", embeddingResp.Error)
	}

	return embeddingResp.Embedding, nil
}

// getMockEmbedding creates a simple mock embedding for testing
func (vm *VectorMatcher) getMockEmbedding(text string) []float32 {
	// Create a simple hash-based mock embedding
	embedding := make([]float32, 384) // MiniLM dimension
	hash := 0
	for _, char := range text {
		hash = hash*31 + int(char)
	}

	// Fill embedding with pseudo-random values based on text hash
	for i := range embedding {
		hash = hash*1103515245 + 12345 // Simple LCG
		embedding[i] = float32(hash%1000-500) / 1000.0 // Normalize to [-0.5, 0.5]
	}

	return embedding
}

// acceptMatch accepts a vector-based match
func (vm *VectorMatcher) acceptMatch(engine *MatchEngine, runID, srcID int64, candidate *VectorCandidate) error {
	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        "vector_semantic",
		Score:         candidate.CombinedScore,
		Confidence:    candidate.SemanticScore,
		TieRank:       1,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "auto_accepted",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Vector match auto-accepted (semantic=%.3f, combined=%.3f)",
			candidate.SemanticScore, candidate.CombinedScore),
	}

	if err := engine.SaveMatchResult(result); err != nil {
		return fmt.Errorf("failed to save match result: %w", err)
	}

	return engine.AcceptMatch(srcID, candidate.UPRN, "vector_semantic",
		candidate.CombinedScore, candidate.SemanticScore, runID, "system")
}

// saveForReview saves a vector candidate for manual review
func (vm *VectorMatcher) saveForReview(engine *MatchEngine, runID, srcID int64,
	candidate *VectorCandidate, rank int) error {

	result := &MatchResult{
		RunID:         runID,
		SrcID:         srcID,
		CandidateUPRN: candidate.UPRN,
		Method:        "vector_semantic",
		Score:         candidate.CombinedScore,
		Confidence:    candidate.SemanticScore,
		TieRank:       rank,
		Features:      candidate.Features,
		Decided:       true,
		Decision:      "needs_review",
		DecidedBy:     "system",
		Notes:         fmt.Sprintf("Vector match requiring review (semantic=%.3f, combined=%.3f)",
			candidate.SemanticScore, candidate.CombinedScore),
	}

	return engine.SaveMatchResult(result)
}