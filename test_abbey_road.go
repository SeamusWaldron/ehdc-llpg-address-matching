package main

import (
	"database/sql"
	"fmt"
	"log"
	
	_ "github.com/lib/pq"
	"github.com/ehdc-llpg/internal/matcher"
)

func main() {
	// Connect to database
	connStr := "host=localhost port=15435 user=postgres password=kljh234hjkl2h dbname=ehdc_llpg sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("🏘️  Testing Abbey Road Component Matching")
	fmt.Println("=========================================")

	// Create component engine
	engine := matcher.NewComponentEngine(db)

	// Test Abbey Road match
	input := matcher.MatchInput{
		DocumentID: 100,
		RawAddress: "ABBEY ROAD, MEDSTEAD, ALTON, HANTS",
	}

	fmt.Printf("📍 Testing: %s\n", input.RawAddress)
	
	// Check what we have in LLPG for Abbey Road
	fmt.Println("\n📋 LLPG Abbey Road addresses:")
	rows, _ := db.Query(`
		SELECT full_address, gopostal_house_number, gopostal_road, gopostal_city, gopostal_postcode
		FROM dim_address 
		WHERE gopostal_road ILIKE '%abbey road%' 
		AND gopostal_processed = TRUE
		LIMIT 5
	`)
	defer rows.Close()
	
	for rows.Next() {
		var addr, houseNum, road, city, postcode sql.NullString
		rows.Scan(&addr, &houseNum, &road, &city, &postcode)
		fmt.Printf("   %s → [%s][%s][%s][%s]\n", 
			addr.String, houseNum.String, road.String, city.String, postcode.String)
	}

	fmt.Println("\n🔍 Component Matching Test:")
	result, err := engine.ProcessDocument(true, input)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}

	fmt.Printf("🎯 Result: %s (%s)\n", result.Decision, result.MatchStatus)
	if result.BestCandidate != nil {
		fmt.Printf("📍 Best Match: %s (Score: %.4f)\n", 
			result.BestCandidate.FullAddress, result.BestCandidate.Score)
		fmt.Printf("🏷️  UPRN: %s, Method: %s\n", 
			result.BestCandidate.UPRN, result.BestCandidate.MethodCode)
	}
	fmt.Printf("🔢 Candidates: %d\n", len(result.AllCandidates))
	fmt.Printf("⏱️  Time: %v\n", result.ProcessingTime)
	
	// Show all candidates
	if len(result.AllCandidates) > 0 {
		fmt.Printf("\n📋 All candidates:\n")
		for i, cand := range result.AllCandidates {
			if i >= 10 { break }
			fmt.Printf("   %d. %s (%.4f) - %s\n", 
				i+1, cand.FullAddress, cand.Score, cand.MethodCode)
		}
	}

	fmt.Println("\n✅ Abbey Road test complete!")
}