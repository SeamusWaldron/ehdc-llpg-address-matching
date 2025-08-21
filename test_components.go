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

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal("Cannot ping database:", err)
	}

	fmt.Println("🔧 Testing Component-Based Matching Engine")
	fmt.Println("==========================================")

	// Create component engine
	engine := matcher.NewComponentEngine(db)

	// Test with some documents that should have gopostal components
	var docs []struct {
		ID      int
		Address string
	}

	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM src_document 
		WHERE gopostal_processed = TRUE AND raw_address IS NOT NULL
	`).Scan(&count)
	if err == nil && count > 0 {
		rows, err := db.Query(`
			SELECT document_id, raw_address 
			FROM src_document 
			WHERE gopostal_processed = TRUE AND raw_address IS NOT NULL
			LIMIT 3
		`)
		if err == nil {
			defer rows.Close()
			
			for rows.Next() {
				var doc struct {
					ID      int
					Address string
				}
				rows.Scan(&doc.ID, &doc.Address)
				docs = append(docs, doc)
			}
		}
	}

	if len(docs) == 0 {
		fmt.Println("❌ No preprocessed documents found. Run gopostal preprocessing first.")
		return
	}

	// Test each document
	for i, doc := range docs {
		fmt.Printf("\n📍 Test %d: Document %d\n", i+1, doc.ID)
		fmt.Printf("   Address: %s\n", doc.Address)

		input := matcher.MatchInput{
			DocumentID: int64(doc.ID),
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
		}
		fmt.Printf("   🔢 Candidates: %d\n", len(result.AllCandidates))
		fmt.Printf("   ⏱️  Time: %v\n", result.ProcessingTime)
	}

	fmt.Println("\n✅ Component engine testing complete!")
}