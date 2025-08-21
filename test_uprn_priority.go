package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
	"github.com/ehdc-llpg/internal/config"
	"github.com/ehdc-llpg/internal/matcher"
)

func main() {
	// Load configuration
	err := config.LoadConfig(".env")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Connect to database
	db := connectDB()
	defer db.Close()

	// Get documents with UPRNs that should match
	rows, err := db.Query(`
		SELECT sd.document_id, sd.raw_address, sd.raw_uprn,
		       EXISTS(SELECT 1 FROM dim_address WHERE uprn = sd.raw_uprn) as uprn_exists_in_llpg
		FROM src_document sd
		WHERE sd.raw_uprn IS NOT NULL
		  AND sd.raw_uprn != ''
		ORDER BY sd.document_id
		LIMIT 10
	`)
	if err != nil {
		log.Fatalf("Failed to query documents: %v", err)
	}
	defer rows.Close()

	// Create fixed component engine
	engine := matcher.NewFixedComponentEngine(db)

	fmt.Println("🧪 TESTING UPRN PRIORITY MATCHING")
	fmt.Println("===================================")
	fmt.Println("Rule: If source document has UPRN, use it directly\n")
	
	var successCount, totalCount int
	
	for rows.Next() {
		var docID int64
		var address, rawUPRN string
		var uprnExistsInLLPG bool
		
		if err := rows.Scan(&docID, &address, &rawUPRN, &uprnExistsInLLPG); err != nil {
			continue
		}

		totalCount++
		fmt.Printf("📍 Test %d - Document %d\n", totalCount, docID)
		fmt.Printf("   Address: %s\n", address)
		fmt.Printf("   Source UPRN: %s\n", rawUPRN)
		fmt.Printf("   UPRN in LLPG: %v\n", uprnExistsInLLPG)

		// Process with fixed engine
		input := matcher.MatchInput{
			DocumentID: docID,
			RawAddress: address,
			RawUPRN:    &rawUPRN,
		}

		result, err := engine.ProcessDocument(false, input)
		if err != nil {
			fmt.Printf("   ❌ Error: %v\n", err)
			continue
		}

		if result.BestCandidate != nil {
			fmt.Printf("   ✅ MATCH FOUND:\n")
			fmt.Printf("      Method: %s\n", result.BestCandidate.MethodCode)
			fmt.Printf("      Matched UPRN: %s\n", result.BestCandidate.UPRN)
			fmt.Printf("      Score: %.4f\n", result.BestCandidate.Score)
			
			// Check if it's using UPRN matching
			if result.BestCandidate.MethodCode == "exact_uprn" {
				fmt.Printf("      ✅ CORRECT: Using exact UPRN match\n")
				
				// Verify UPRN matches
				if result.BestCandidate.UPRN == rawUPRN {
					fmt.Printf("      ✅ PERFECT: UPRNs match exactly\n")
					successCount++
				} else {
					fmt.Printf("      ⚠️  WARNING: UPRN mismatch (source: %s, matched: %s)\n", 
						rawUPRN, result.BestCandidate.UPRN)
				}
			} else {
				fmt.Printf("      ⚠️  WARNING: Not using UPRN match (using %s instead)\n", 
					result.BestCandidate.MethodCode)
			}
		} else {
			if uprnExistsInLLPG {
				fmt.Printf("   ❌ ERROR: No match found but UPRN exists in LLPG\n")
			} else {
				fmt.Printf("   ⚪ Expected: No match (UPRN not in LLPG)\n")
				successCount++ // This is correct behavior
			}
		}
		fmt.Println()
	}

	// Summary
	fmt.Println("📊 TEST RESULTS SUMMARY")
	fmt.Println("========================")
	fmt.Printf("Success Rate: %d/%d (%.1f%%)\n", successCount, totalCount, 
		float64(successCount)/float64(totalCount)*100)
	
	if successCount == totalCount {
		fmt.Println("🎉 PERFECT: All documents with UPRNs handled correctly!")
	} else {
		fmt.Println("⚠️  Some issues detected - review the results above")
	}

	// Check overall statistics
	fmt.Println("\n📈 OVERALL UPRN STATISTICS")
	fmt.Println("==========================")
	
	var totalWithUPRN, matchableUPRNs int
	db.QueryRow(`
		SELECT COUNT(*), 
		       COUNT(CASE WHEN EXISTS(SELECT 1 FROM dim_address WHERE uprn = sd.raw_uprn) THEN 1 END)
		FROM src_document sd
		WHERE raw_uprn IS NOT NULL
	`).Scan(&totalWithUPRN, &matchableUPRNs)
	
	fmt.Printf("Total documents with UPRNs: %d\n", totalWithUPRN)
	fmt.Printf("UPRNs that exist in LLPG: %d (%.1f%%)\n", 
		matchableUPRNs, float64(matchableUPRNs)/float64(totalWithUPRN)*100)
	fmt.Printf("Expected perfect matches: %d\n", matchableUPRNs)
}

func connectDB() *sql.DB {
	host := config.GetEnv("DB_HOST", "localhost")
	port := config.GetEnv("DB_PORT", "15435")
	user := config.GetEnv("DB_USER", "postgres")
	password := config.GetEnv("DB_PASSWORD", "kljh234hjkl2h")
	dbname := config.GetEnv("DB_NAME", "ehdc_llpg")
	sslmode := config.GetEnv("DB_SSLMODE", "disable")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("Cannot connect to database: %v", err)
	}

	return db
}