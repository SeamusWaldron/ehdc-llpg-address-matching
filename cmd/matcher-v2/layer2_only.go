package main

import (
	"database/sql"
	"fmt"
)

// runLayer2Only runs only Layer 2 Conservative Matching on existing fact table
func runLayer2Only(localDebug bool, db *sql.DB) error {
	fmt.Println("Running Layer 2: Conservative Validation Matching...")
	fmt.Println("==================================================")
	
	// Check current fact table status
	var totalRecords, matchedRecords, unmatchedRecords int
	err := db.QueryRow(`
		SELECT 
			COUNT(*) as total,
			COUNT(matched_address_id) as matched,
			COUNT(*) - COUNT(matched_address_id) as unmatched
		FROM fact_documents_lean
	`).Scan(&totalRecords, &matchedRecords, &unmatchedRecords)
	
	if err != nil {
		return fmt.Errorf("failed to get fact table status: %v", err)
	}
	
	fmt.Printf("Fact table status:\n")
	fmt.Printf("  Total records: %d\n", totalRecords)
	fmt.Printf("  Already matched: %d (%.1f%%)\n", matchedRecords, float64(matchedRecords)/float64(totalRecords)*100)
	fmt.Printf("  Unmatched for Layer 2: %d (%.1f%%)\n", unmatchedRecords, float64(unmatchedRecords)/float64(totalRecords)*100)
	
	// Run Conservative Matching on unmatched records
	fmt.Println("\nStarting Conservative Matching on unmatched records...")
	err = runConservativeMatching(localDebug, db, "layer2-conservative")
	if err != nil {
		return fmt.Errorf("Layer 2 conservative matching failed: %v", err)
	}
	
	// Show updated statistics
	fmt.Println("\n--- LAYER 2 RESULTS ---")
	err = db.QueryRow(`
		SELECT 
			COUNT(*) as total,
			COUNT(matched_address_id) as matched,
			COUNT(*) - COUNT(matched_address_id) as unmatched
		FROM fact_documents_lean
	`).Scan(&totalRecords, &matchedRecords, &unmatchedRecords)
	
	if err == nil {
		fmt.Printf("Updated fact table status:\n")
		fmt.Printf("  Total records: %d\n", totalRecords)
		fmt.Printf("  Matched after Layer 2: %d (%.1f%%)\n", matchedRecords, float64(matchedRecords)/float64(totalRecords)*100)
		fmt.Printf("  Still unmatched: %d (%.1f%%)\n", unmatchedRecords, float64(unmatchedRecords)/float64(totalRecords)*100)
	}
	
	return nil
}