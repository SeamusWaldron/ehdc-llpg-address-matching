package engine

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ReviewInterface provides manual review capabilities
type ReviewInterface struct {
	db *sql.DB
}

// NewReviewInterface creates a new review interface
func NewReviewInterface(db *sql.DB) *ReviewInterface {
	return &ReviewInterface{db: db}
}

// ReviewItem represents an item requiring manual review
type ReviewItem struct {
	SrcID         int64
	SourceAddr    string
	SourceType    string
	Candidates    []*ReviewCandidate
	ReviewedBy    string
	ReviewedAt    *time.Time
	Decision      string
	Notes         string
}

// ReviewCandidate represents a candidate address for review
type ReviewCandidate struct {
	UPRN       string
	Address    string
	Method     string
	Score      float64
	Confidence float64
	Features   map[string]interface{}
	TieRank    int
}

// RunInteractiveReview starts an interactive review session
func (ri *ReviewInterface) RunInteractiveReview(batchSize int, reviewer string) error {
	fmt.Println("=== EHDC LLPG Interactive Review Interface ===\n")
	
	if reviewer == "" {
		reviewer = "system_user"
	}

	totalReviewed := 0
	
	for {
		// Get next batch of items needing review
		items, err := ri.getItemsNeedingReview(batchSize)
		if err != nil {
			return fmt.Errorf("failed to get review items: %w", err)
		}

		if len(items) == 0 {
			fmt.Println("No more items requiring review!")
			break
		}

		fmt.Printf("Found %d items requiring review. Starting batch...\n\n", len(items))

		for i, item := range items {
			fmt.Printf("=== Review Item %d of %d ===\n", i+1, len(items))
			
			decision, err := ri.reviewItem(item, reviewer)
			if err != nil {
				fmt.Printf("Error reviewing item: %v\n", err)
				continue
			}

			if decision == "quit" {
				fmt.Printf("\nReview session ended. Total reviewed: %d\n", totalReviewed)
				return nil
			}

			totalReviewed++
			fmt.Printf("Decision recorded: %s\n\n", decision)
		}

		// Ask if user wants to continue
		fmt.Printf("Batch complete. Continue with next batch? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		
		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			break
		}
	}

	fmt.Printf("\nReview session complete. Total items reviewed: %d\n", totalReviewed)
	return nil
}

// getItemsNeedingReview gets items that need manual review
func (ri *ReviewInterface) getItemsNeedingReview(limit int) ([]*ReviewItem, error) {
	// Get distinct source documents that have candidates needing review
	rows, err := ri.db.Query(`
		SELECT DISTINCT s.src_id, s.source_type, s.raw_address, s.addr_can
		FROM src_document s
		JOIN match_result mr ON mr.src_id = s.src_id
		WHERE mr.decision = 'needs_review'
		  AND mr.src_id NOT IN (
			  SELECT src_id FROM match_accepted
		  )
		ORDER BY s.src_id
		LIMIT $1
	`, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*ReviewItem

	for rows.Next() {
		item := &ReviewItem{}
		var addrCan *string
		
		err := rows.Scan(&item.SrcID, &item.SourceType, &item.SourceAddr, &addrCan)
		if err != nil {
			continue
		}

		if addrCan != nil {
			item.SourceAddr = *addrCan // Use canonical address if available
		}

		// Get candidates for this item
		candidates, err := ri.getCandidatesForReview(item.SrcID)
		if err != nil {
			continue
		}

		item.Candidates = candidates
		items = append(items, item)
	}

	return items, nil
}

// getCandidatesForReview gets all candidates for a source document
func (ri *ReviewInterface) getCandidatesForReview(srcID int64) ([]*ReviewCandidate, error) {
	rows, err := ri.db.Query(`
		SELECT mr.candidate_uprn, mr.method, mr.score, mr.confidence, 
			   mr.tie_rank, mr.features, d.locaddress
		FROM match_result mr
		LEFT JOIN dim_address d ON d.uprn = mr.candidate_uprn
		WHERE mr.src_id = $1 AND mr.decision = 'needs_review'
		ORDER BY mr.score DESC, mr.tie_rank ASC
		LIMIT 5
	`, srcID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []*ReviewCandidate

	for rows.Next() {
		candidate := &ReviewCandidate{}
		var featuresJSON *string
		
		err := rows.Scan(
			&candidate.UPRN, &candidate.Method, &candidate.Score,
			&candidate.Confidence, &candidate.TieRank, &featuresJSON,
			&candidate.Address,
		)
		if err != nil {
			continue
		}

		// Parse features JSON
		if featuresJSON != nil {
			json.Unmarshal([]byte(*featuresJSON), &candidate.Features)
		}
		if candidate.Features == nil {
			candidate.Features = make(map[string]interface{})
		}

		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

// reviewItem presents an item for manual review
func (ri *ReviewInterface) reviewItem(item *ReviewItem, reviewer string) (string, error) {
	// Display source information
	fmt.Printf("Source ID: %d\n", item.SrcID)
	fmt.Printf("Source Type: %s\n", item.SourceType)
	fmt.Printf("Source Address: %s\n", item.SourceAddr)
	fmt.Println()

	if len(item.Candidates) == 0 {
		fmt.Println("No candidates available for review")
		return "no_candidates", nil
	}

	// Display candidates
	fmt.Printf("Found %d candidate matches:\n\n", len(item.Candidates))
	
	for i, candidate := range item.Candidates {
		fmt.Printf("%d. UPRN: %s\n", i+1, candidate.UPRN)
		fmt.Printf("   Address: %s\n", candidate.Address)
		fmt.Printf("   Method: %s (Score: %.3f, Confidence: %.3f)\n", 
			candidate.Method, candidate.Score, candidate.Confidence)
		
		// Show key features
		ri.displayKeyFeatures(candidate.Features)
		fmt.Println()
	}

	// Get user decision
	return ri.getUserDecision(item, reviewer)
}

// displayKeyFeatures shows important matching features
func (ri *ReviewInterface) displayKeyFeatures(features map[string]interface{}) {
	if features == nil {
		return
	}

	// Show most relevant features
	keyFeatures := []string{
		"distance_meters", "similarity", "trgm_score", "semantic_score",
		"component_score", "same_house_number", "locality_overlap",
	}

	var displayFeatures []string
	for _, key := range keyFeatures {
		if value, exists := features[key]; exists {
			switch v := value.(type) {
			case float64:
				if key == "distance_meters" {
					displayFeatures = append(displayFeatures, fmt.Sprintf("%s: %.1fm", key, v))
				} else {
					displayFeatures = append(displayFeatures, fmt.Sprintf("%s: %.3f", key, v))
				}
			case bool:
				displayFeatures = append(displayFeatures, fmt.Sprintf("%s: %v", key, v))
			}
		}
	}

	if len(displayFeatures) > 0 {
		fmt.Printf("   Features: %s\n", strings.Join(displayFeatures, ", "))
	}
}

// getUserDecision prompts user for decision
func (ri *ReviewInterface) getUserDecision(item *ReviewItem, reviewer string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Options:")
	for i := range item.Candidates {
		fmt.Printf("  %d - Accept candidate %d\n", i+1, i+1)
	}
	fmt.Println("  r - Reject all candidates")
	fmt.Println("  s - Skip this item (review later)")
	fmt.Println("  q - Quit review session")
	fmt.Println()
	fmt.Print("Your decision: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	choice := strings.TrimSpace(strings.ToLower(input))

	switch choice {
	case "r":
		return ri.recordDecision(item, "rejected", "", reviewer)
	case "s":
		return "skipped", nil
	case "q":
		return "quit", nil
	default:
		// Try to parse as number
		num, err := strconv.Atoi(choice)
		if err != nil || num < 1 || num > len(item.Candidates) {
			fmt.Printf("Invalid choice '%s'. Please try again.\n", choice)
			return ri.getUserDecision(item, reviewer)
		}
		
		selectedCandidate := item.Candidates[num-1]
		return ri.recordDecision(item, "accepted", selectedCandidate.UPRN, reviewer)
	}
}

// recordDecision records the user's decision
func (ri *ReviewInterface) recordDecision(item *ReviewItem, decision, uprn, reviewer string) (string, error) {
	// Get notes if rejecting
	var notes string
	if decision == "rejected" {
		fmt.Print("Optional notes for rejection: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		notes = strings.TrimSpace(input)
	}

	// Record the decision
	if decision == "accepted" && uprn != "" {
		// Accept the match
		engine := &MatchEngine{db: ri.db}
		err := engine.AcceptMatch(item.SrcID, uprn, "manual_review", 1.0, 1.0, 0, reviewer)
		if err != nil {
			return "", fmt.Errorf("failed to accept match: %w", err)
		}

		// Update all match_result records for this src_id
		_, err = ri.db.Exec(`
			UPDATE match_result 
			SET decided = true, decision = $1, decided_by = $2, reviewed_at = NOW(),
				notes = $3
			WHERE src_id = $4
		`, "manual_accepted", reviewer, notes, item.SrcID)
		
		if err != nil {
			return "", fmt.Errorf("failed to update match results: %w", err)
		}

		fmt.Printf("✓ Accepted match: UPRN %s\n", uprn)
		return "accepted", nil

	} else if decision == "rejected" {
		// Reject all candidates
		_, err := ri.db.Exec(`
			UPDATE match_result 
			SET decided = true, decision = $1, decided_by = $2, reviewed_at = NOW(),
				notes = $3
			WHERE src_id = $4 AND decision = 'needs_review'
		`, "manual_rejected", reviewer, notes, item.SrcID)
		
		if err != nil {
			return "", fmt.Errorf("failed to update match results: %w", err)
		}

		fmt.Printf("✗ Rejected all candidates%s\n", 
			func() string {
				if notes != "" {
					return " (Notes: " + notes + ")"
				}
				return ""
			}())
		return "rejected", nil
	}

	return decision, nil
}

// GetReviewStats shows statistics about items needing review
func (ri *ReviewInterface) GetReviewStats() error {
	fmt.Println("\n=== Review Queue Statistics ===\n")

	// Total items needing review
	var totalNeedsReview int
	err := ri.db.QueryRow(`
		SELECT COUNT(DISTINCT src_id)
		FROM match_result 
		WHERE decision = 'needs_review'
		  AND src_id NOT IN (SELECT src_id FROM match_accepted)
	`).Scan(&totalNeedsReview)

	if err == nil {
		fmt.Printf("Total items needing review: %d\n", totalNeedsReview)
	}

	// Items by matching method
	fmt.Println("\n=== Items Needing Review by Method ===")
	rows, err := ri.db.Query(`
		SELECT 
			CASE 
				WHEN method LIKE 'fuzzy%' THEN 'Fuzzy'
				WHEN method LIKE 'spatial%' THEN 'Spatial'
				WHEN method LIKE 'postcode%' THEN 'Postcode'
				WHEN method LIKE 'hierarchical%' THEN 'Hierarchical'
				WHEN method LIKE 'rule%' THEN 'Rule-based'
				WHEN method LIKE 'vector%' THEN 'Vector/Semantic'
				ELSE 'Other'
			END as method_type,
			COUNT(DISTINCT src_id) as count
		FROM match_result
		WHERE decision = 'needs_review'
		  AND src_id NOT IN (SELECT src_id FROM match_accepted)
		GROUP BY method_type
		ORDER BY count DESC
	`)

	if err == nil {
		defer rows.Close()
		fmt.Println("Method        | Count")
		fmt.Println("--------------|-------")
		
		for rows.Next() {
			var methodType string
			var count int
			if err := rows.Scan(&methodType, &count); err == nil {
				fmt.Printf("%-13s | %6d\n", methodType, count)
			}
		}
	}

	// Score distribution
	fmt.Println("\n=== Score Distribution for Review Items ===")
	rows2, err := ri.db.Query(`
		SELECT 
			CASE 
				WHEN score >= 0.90 THEN '0.90-1.00'
				WHEN score >= 0.80 THEN '0.80-0.90'
				WHEN score >= 0.70 THEN '0.70-0.80'
				WHEN score >= 0.60 THEN '0.60-0.70'
				ELSE '< 0.60'
			END as score_range,
			COUNT(DISTINCT src_id) as count
		FROM match_result
		WHERE decision = 'needs_review'
		  AND src_id NOT IN (SELECT src_id FROM match_accepted)
		GROUP BY score_range
		ORDER BY score_range DESC
	`)

	if err == nil {
		defer rows2.Close()
		fmt.Println("Score Range | Count")
		fmt.Println("------------|-------")
		
		for rows2.Next() {
			var scoreRange string
			var count int
			if err := rows2.Scan(&scoreRange, &count); err == nil {
				fmt.Printf("%-11s | %6d\n", scoreRange, count)
			}
		}
	}

	return nil
}