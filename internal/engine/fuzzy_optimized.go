package engine

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// OptimizedFuzzyMatcher provides optimized fuzzy matching with parallel processing
type OptimizedFuzzyMatcher struct {
	db           *sql.DB
	workerCount  int
	cacheEnabled bool
	cache        map[string][]*FuzzyCandidate
	cacheMutex   sync.RWMutex
}

// NewOptimizedFuzzyMatcher creates an optimized fuzzy matcher
func NewOptimizedFuzzyMatcher(db *sql.DB, workerCount int) *OptimizedFuzzyMatcher {
	if workerCount <= 0 {
		workerCount = 4 // Default to 4 workers
	}
	return &OptimizedFuzzyMatcher{
		db:           db,
		workerCount:  workerCount,
		cacheEnabled: true,
		cache:        make(map[string][]*FuzzyCandidate),
	}
}

// RunOptimizedFuzzyMatching performs optimized fuzzy matching with parallel processing
func (ofm *OptimizedFuzzyMatcher) RunOptimizedFuzzyMatching(runID int64, batchSize int, tiers *FuzzyMatchingTiers) (int, int, int, error) {
	if tiers == nil {
		tiers = DefaultTiers()
	}

	startTime := time.Now()
	totalProcessed := 0
	totalAccepted := 0
	totalNeedsReview := 0

	fmt.Printf("Starting OPTIMIZED fuzzy matching with %d workers...\n", ofm.workerCount)
	fmt.Printf("Similarity thresholds: High=%.2f, Medium=%.2f, Low=%.2f, Min=%.2f\n",
		tiers.HighConfidence, tiers.MediumConfidence, tiers.LowConfidence, tiers.MinThreshold)

	// Create worker pool
	docChan := make(chan *SourceDocument, batchSize)
	resultChan := make(chan *matchResult, batchSize)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < ofm.workerCount; i++ {
		wg.Add(1)
		go ofm.worker(i, runID, tiers, docChan, resultChan, &wg)
	}

	// Result collector goroutine
	doneChan := make(chan bool)
	go func() {
		for result := range resultChan {
			totalProcessed++
			if result.accepted {
				totalAccepted++
			} else if result.needsReview {
				totalNeedsReview++
			}

			if totalProcessed%1000 == 0 {
				elapsed := time.Since(startTime)
				rate := float64(totalProcessed) / elapsed.Seconds()
				fmt.Printf("Processed %d documents (%.1f/sec), accepted %d, needs review %d...\n",
					totalProcessed, rate, totalAccepted, totalNeedsReview)
			}
		}
		doneChan <- true
	}()

	// Feed documents to workers
	engine := &MatchEngine{db: ofm.db}
	for {
		docs, err := engine.GetUnmatchedDocuments(batchSize, "")
		if err != nil {
			close(docChan)
			wg.Wait()
			close(resultChan)
			<-doneChan
			return totalProcessed, totalAccepted, totalNeedsReview, fmt.Errorf("failed to get unmatched documents: %w", err)
		}

		if len(docs) == 0 {
			break
		}

		for _, doc := range docs {
			docChan <- &doc
		}
	}

	// Close channels and wait for completion
	close(docChan)
	wg.Wait()
	close(resultChan)
	<-doneChan

	elapsed := time.Since(startTime)
	fmt.Printf("\nOptimized fuzzy matching complete in %.2f seconds\n", elapsed.Seconds())
	fmt.Printf("Final: processed %d (%.1f/sec), accepted %d, needs review %d\n",
		totalProcessed, float64(totalProcessed)/elapsed.Seconds(), totalAccepted, totalNeedsReview)

	return totalProcessed, totalAccepted, totalNeedsReview, nil
}

// worker processes documents in parallel
func (ofm *OptimizedFuzzyMatcher) worker(id int, runID int64, tiers *FuzzyMatchingTiers,
	docChan <-chan *SourceDocument, resultChan chan<- *matchResult, wg *sync.WaitGroup) {
	
	defer wg.Done()
	fm := &FuzzyMatcher{db: ofm.db}
	engine := &MatchEngine{db: ofm.db}

	for doc := range docChan {
		result := &matchResult{srcID: doc.SrcID}

		if doc.AddrCan == nil || *doc.AddrCan == "" || *doc.AddrCan == "N A" {
			resultChan <- result
			continue
		}

		// Check cache first
		var candidates []*FuzzyCandidate
		var err error

		if ofm.cacheEnabled {
			candidates = ofm.getFromCache(*doc.AddrCan)
		}

		if candidates == nil {
			// Find fuzzy candidates with optimized query
			candidates, err = ofm.findOptimizedCandidates(*doc, tiers.MinThreshold)
			if err != nil {
				fmt.Printf("Worker %d: Error finding candidates for doc %d: %v\n", id, doc.SrcID, err)
				resultChan <- result
				continue
			}

			if ofm.cacheEnabled && len(candidates) > 0 {
				ofm.addToCache(*doc.AddrCan, candidates)
			}
		}

		if len(candidates) == 0 {
			resultChan <- result
			continue
		}

		// Make decision
		decision, _ := fm.makeDecision(candidates, tiers)

		switch decision {
		case "auto_accepted":
			best := candidates[0]
			err := fm.acceptFuzzyMatch(engine, runID, doc.SrcID, best, "fuzzy_auto_optimized", best.FinalScore, best.TrgramScore)
			if err == nil {
				result.accepted = true
			}

		case "needs_review":
			// Save top candidates for review
			for i, candidate := range candidates {
				if i >= 3 {
					break
				}

				matchResult := &MatchResult{
					RunID:         runID,
					SrcID:         doc.SrcID,
					CandidateUPRN: candidate.UPRN,
					Method:        fmt.Sprintf("fuzzy_opt_%.2f", candidate.TrgramScore),
					Score:         candidate.FinalScore,
					Confidence:    candidate.TrgramScore,
					TieRank:       i + 1,
					Features:      candidate.Features,
					Decided:       true,
					Decision:      "needs_review",
					DecidedBy:     "system",
					Notes:         fmt.Sprintf("Optimized fuzzy match requiring review (similarity=%.3f)", candidate.TrgramScore),
				}
				engine.SaveMatchResult(matchResult)
			}
			result.needsReview = true
		}

		resultChan <- result
	}
}

// findOptimizedCandidates uses an optimized query for finding candidates
func (ofm *OptimizedFuzzyMatcher) findOptimizedCandidates(doc SourceDocument, minSimilarity float64) ([]*FuzzyCandidate, error) {
	if doc.AddrCan == nil || *doc.AddrCan == "" {
		return nil, nil
	}

	addrCan := *doc.AddrCan

	// Use prepared statement for better performance
	rows, err := ofm.db.Query(`
		WITH candidates AS (
			SELECT d.uprn, d.locaddress, d.addr_can, d.easting, d.northing, 
				   d.usrn, d.blpu_class, d.status,
				   similarity($1, d.addr_can) as trgm_score
			FROM dim_address d
			WHERE d.addr_can % $1
			  AND similarity($1, d.addr_can) >= $2
			ORDER BY trgm_score DESC
			LIMIT 20
		)
		SELECT * FROM candidates WHERE trgm_score >= $2
	`, addrCan, minSimilarity)

	if err != nil {
		return nil, fmt.Errorf("optimized query failed: %w", err)
	}
	defer rows.Close()

	var candidates []*FuzzyCandidate
	fm := &FuzzyMatcher{db: ofm.db}

	for rows.Next() {
		candidate := &FuzzyCandidate{
			AddressCandidate: &AddressCandidate{},
			Features:         make(map[string]interface{}),
		}

		err := rows.Scan(
			&candidate.UPRN, &candidate.LocAddress, &candidate.AddrCan,
			&candidate.Easting, &candidate.Northing, &candidate.USRN,
			&candidate.BLPUClass, &candidate.Status, &candidate.TrgramScore,
		)
		if err != nil {
			continue
		}

		// Compute features only for high-scoring candidates
		if candidate.TrgramScore >= minSimilarity {
			fm.computeFeatures(doc, candidate)
			if fm.passesFilters(doc, candidate) {
				candidates = append(candidates, candidate)
			}
		}
	}

	return candidates, nil
}

// Cache management functions
func (ofm *OptimizedFuzzyMatcher) getFromCache(address string) []*FuzzyCandidate {
	ofm.cacheMutex.RLock()
	defer ofm.cacheMutex.RUnlock()
	return ofm.cache[address]
}

func (ofm *OptimizedFuzzyMatcher) addToCache(address string, candidates []*FuzzyCandidate) {
	ofm.cacheMutex.Lock()
	defer ofm.cacheMutex.Unlock()
	
	// Limit cache size to prevent memory issues
	if len(ofm.cache) > 10000 {
		// Clear oldest entries (simple strategy)
		for k := range ofm.cache {
			delete(ofm.cache, k)
			if len(ofm.cache) <= 5000 {
				break
			}
		}
	}
	
	ofm.cache[address] = candidates
}

// matchResult is used for communicating results from workers
type matchResult struct {
	srcID       int64
	accepted    bool
	needsReview bool
}