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

	// Test the problematic UPRN: 1710022145.00 should match 1710022145
	docID := int64(78053)  // From our earlier query
	address := ""
	uprn := "1710022145.00"
	
	// Get the actual address
	err = db.QueryRow(`
		SELECT raw_address FROM src_document WHERE document_id = $1
	`, docID).Scan(&address)
	if err != nil {
		log.Fatalf("Failed to get document: %v", err)
	}

	fmt.Printf("🧪 TESTING UPRN NORMALIZATION FIX\n")
	fmt.Printf("==================================\n")
	fmt.Printf("Document ID: %d\n", docID)
	fmt.Printf("Address: %s\n", address)
	fmt.Printf("Source UPRN: %s (with decimal)\n", uprn)
	fmt.Println()

	// Check if normalized version exists in LLPG
	var existsInLLPG bool
	var llpgAddress string
	err = db.QueryRow(`
		SELECT TRUE, full_address FROM dim_address 
		WHERE uprn = $1 AND is_historic = FALSE
	`, "1710022145").Scan(&existsInLLPG, &llpgAddress)
	
	if err == nil {
		fmt.Printf("✅ LLPG Record Found: %s (UPRN: 1710022145)\n", llpgAddress)
	} else {
		fmt.Printf("❌ No LLPG record found for normalized UPRN: 1710022145\n")
		return
	}
	fmt.Println()

	// Test with fixed engine
	engine := matcher.NewFixedComponentEngine(db)
	
	input := matcher.MatchInput{
		DocumentID: docID,
		RawAddress: address,
		RawUPRN:    &uprn,
	}

	fmt.Printf("🔄 Processing with FIXED engine (should normalize %s to 1710022145)...\n", uprn)
	result, err := engine.ProcessDocument(true, input) // Enable debug
	if err != nil {
		log.Fatalf("Processing failed: %v", err)
	}

	if result.BestCandidate != nil {
		fmt.Printf("\n✅ MATCH RESULT:\n")
		fmt.Printf("   Method: %s\n", result.BestCandidate.MethodCode)
		fmt.Printf("   Score: %.4f\n", result.BestCandidate.Score)
		fmt.Printf("   Decision: %s\n", result.Decision)
		fmt.Printf("   Matched UPRN: %s\n", result.BestCandidate.UPRN)
		
		if result.BestCandidate.MethodCode == "exact_uprn" {
			fmt.Printf("   ✅ SUCCESS: Found exact UPRN match (normalization worked!)\n")
		} else if result.BestCandidate.MethodCode == "historic_uprn" {
			fmt.Printf("   ❌ FAILED: Still creating historic record (normalization didn't work)\n")
		}
	} else {
		fmt.Printf("❌ No match found\n")
	}

	// Check if any historic records were created
	var historicCount int
	db.QueryRow("SELECT COUNT(*) FROM dim_address WHERE is_historic = TRUE").Scan(&historicCount)
	
	fmt.Printf("\n📊 Historic records after test: %d\n", historicCount)
	if historicCount == 0 {
		fmt.Printf("✅ PERFECT: No historic records created - normalization worked!\n")
	} else {
		fmt.Printf("❌ ISSUE: Historic record was created despite existing LLPG match\n")
	}
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