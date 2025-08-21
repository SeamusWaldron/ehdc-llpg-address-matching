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

	fmt.Println("🎯 Testing Component-Based Matching with Targeted Addresses")
	fmt.Println("===========================================================")

	// Create component engine
	engine := matcher.NewComponentEngine(db)

	// Test specific documents that should have matches
	testDocs := []struct{
		ID int64
		Address string
	}{
		{8, "8 CHASE ROAD, LINDFORD, BORDON, WHITEHILL, GU35 0RG"},
		{15, "EVELEY CLOSE, WHITEHILL, BORDON, HANTS"},
		{39, "8 THORPE GARDENS, ALTON"},
	}

	for i, doc := range testDocs {
		fmt.Printf("\n📍 Test %d: Document %d\n", i+1, doc.ID)
		fmt.Printf("   Address: %s\n", doc.Address)

		input := matcher.MatchInput{
			DocumentID: doc.ID,
			RawAddress: doc.Address,
		}

		result, err := engine.ProcessDocument(true, input)
		if err != nil {
			fmt.Printf("   ❌ Error: %v\n", err)
			continue
		}

		fmt.Printf("   🎯 Result: %s (%s)\n", result.Decision, result.MatchStatus)
		if result.BestCandidate != nil {
			fmt.Printf("   📍 Best Match: %s (Score: %.4f)\n", 
				result.BestCandidate.FullAddress, result.BestCandidate.Score)
			fmt.Printf("   🏷️  UPRN: %s, Method: %s\n", 
				result.BestCandidate.UPRN, result.BestCandidate.MethodCode)
		}
		fmt.Printf("   🔢 Candidates: %d\n", len(result.AllCandidates))
		fmt.Printf("   ⏱️  Time: %v\n", result.ProcessingTime)
		
		// Show top 3 candidates if available
		if len(result.AllCandidates) > 1 {
			fmt.Printf("   📋 Top candidates:\n")
			for j, cand := range result.AllCandidates {
				if j >= 3 { break }
				fmt.Printf("      %d. %s (%.4f) - %s\n", 
					j+1, cand.FullAddress, cand.Score, cand.MethodCode)
			}
		}
	}

	fmt.Println("\n✅ Targeted component engine testing complete!")
}