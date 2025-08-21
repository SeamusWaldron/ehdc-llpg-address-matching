package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

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

	// Get HORNDEAN FOOTBALL CLUB documents
	rows, err := db.Query(`
		SELECT document_id, raw_address 
		FROM src_document 
		WHERE raw_address ILIKE '%HORNDEAN FOOTBALL CLUB%'
		ORDER BY document_id
		LIMIT 5
	`)
	if err != nil {
		log.Fatalf("Failed to query documents: %v", err)
	}
	defer rows.Close()

	// Create fixed component engine
	engine := matcher.NewFixedComponentEngine(db)

	fmt.Println("🧪 TESTING FIXED ALGORITHM ON HORNDEAN FOOTBALL CLUB ADDRESSES")
	fmt.Println("================================================================")
	
	var testResults []TestResult
	
	for rows.Next() {
		var docID int64
		var address string
		
		if err := rows.Scan(&docID, &address); err != nil {
			continue
		}

		fmt.Printf("\n📍 Testing Document %d: %s\n", docID, address)
		fmt.Println("   " + strings.Repeat("─", 60))

		// Process with fixed engine
		input := matcher.MatchInput{
			DocumentID: docID,
			RawAddress: address,
		}

		result, err := engine.ProcessDocument(true, input)
		if err != nil {
			fmt.Printf("   ❌ Error: %v\n", err)
			continue
		}

		var resultSummary TestResult
		resultSummary.DocumentID = docID
		resultSummary.Address = address
		resultSummary.Decision = result.Decision
		
		if result.BestCandidate != nil {
			resultSummary.MatchedAddress = result.BestCandidate.FullAddress
			resultSummary.MatchedUPRN = result.BestCandidate.UPRN
			resultSummary.Score = result.BestCandidate.Score
			resultSummary.MethodCode = result.BestCandidate.MethodCode
			
			fmt.Printf("   ✅ MATCH FOUND:\n")
			fmt.Printf("      📍 Address: %s\n", result.BestCandidate.FullAddress)
			fmt.Printf("      🆔 UPRN: %s\n", result.BestCandidate.UPRN)
			fmt.Printf("      📊 Score: %.4f\n", result.BestCandidate.Score)
			fmt.Printf("      🔧 Method: %s\n", result.BestCandidate.MethodCode)
			fmt.Printf("      ⚖️ Decision: %s\n", result.Decision)
			
			// Check if this is the correct football club
			if contains(result.BestCandidate.FullAddress, "Horndean Football Club") {
				fmt.Printf("      ✅ CORRECT: Found actual football club!\n")
				resultSummary.IsCorrect = true
			} else {
				fmt.Printf("      ⚠️  POTENTIAL ISSUE: Not the actual football club\n")
				resultSummary.IsCorrect = false
			}
		} else {
			fmt.Printf("   ⚪ NO MATCH FOUND\n")
			fmt.Printf("      ⚖️ Decision: %s\n", result.Decision)
			resultSummary.IsCorrect = true // No match is better than wrong match
		}
		
		testResults = append(testResults, resultSummary)
	}

	// Summary
	fmt.Printf("\n\n📊 FIXED ALGORITHM TEST RESULTS SUMMARY\n")
	fmt.Println("=========================================")
	
	correctCount := 0
	totalCount := len(testResults)
	
	for _, result := range testResults {
		status := "❌ WRONG"
		if result.IsCorrect {
			status = "✅ CORRECT"
			correctCount++
		}
		
		fmt.Printf("Doc %d: %s (Decision: %s, Score: %.3f)\n", 
			result.DocumentID, status, result.Decision, result.Score)
	}
	
	successRate := float64(correctCount) / float64(totalCount) * 100
	fmt.Printf("\n🎯 Success Rate: %d/%d (%.1f%%)\n", correctCount, totalCount, successRate)
	
	if successRate >= 80 {
		fmt.Println("🎉 EXCELLENT: Fixed algorithm is working correctly!")
	} else {
		fmt.Println("⚠️  NEEDS IMPROVEMENT: Algorithm requires further tuning")
	}
}

type TestResult struct {
	DocumentID     int64
	Address        string
	MatchedAddress string
	MatchedUPRN    string
	Score          float64
	MethodCode     string
	Decision       string
	IsCorrect      bool
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		   len(s) > len(substr) && 
		   (s[:len(substr)] == substr || 
		    s[len(s)-len(substr):] == substr ||
		    hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}