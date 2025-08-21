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

	// Find a document with UPRN that doesn't exist in LLPG
	var docID int64
	var address, uprn string
	
	err = db.QueryRow(`
		SELECT sd.document_id, sd.raw_address, sd.raw_uprn
		FROM src_document sd
		WHERE sd.raw_uprn IS NOT NULL
		  AND sd.raw_uprn != ''
		  AND NOT EXISTS(SELECT 1 FROM dim_address WHERE uprn = sd.raw_uprn)
		LIMIT 1
	`).Scan(&docID, &address, &uprn)
	
	if err != nil {
		log.Fatalf("Failed to find test document: %v", err)
	}

	fmt.Println("🧪 TESTING HISTORIC UPRN CREATION")
	fmt.Println("==================================")
	fmt.Printf("Test Document ID: %d\n", docID)
	fmt.Printf("Address: %s\n", address)
	fmt.Printf("UPRN (not in LLPG): %s\n", uprn)
	fmt.Println()

	// Create fixed component engine
	engine := matcher.NewFixedComponentEngine(db)

	// Process with the engine
	input := matcher.MatchInput{
		DocumentID: docID,
		RawAddress: address,
		RawUPRN:    &uprn,
	}

	fmt.Println("🔄 Processing with Fixed Component Engine...")
	result, err := engine.ProcessDocument(true, input) // Enable debug
	if err != nil {
		log.Fatalf("Processing failed: %v", err)
	}

	// Check result
	if result.BestCandidate != nil {
		fmt.Printf("✅ HISTORIC UPRN CREATED:\n")
		fmt.Printf("   Address ID: %d\n", result.BestCandidate.AddressID)
		fmt.Printf("   Method: %s\n", result.BestCandidate.MethodCode)
		fmt.Printf("   UPRN: %s\n", result.BestCandidate.UPRN)
		fmt.Printf("   Score: %.4f\n", result.BestCandidate.Score)
		fmt.Printf("   Decision: %s\n", result.Decision)
		
		// Verify the historic record exists
		var isHistoric bool
		var createdFromSource bool
		var sourceDocID sql.NullInt32
		
		err = db.QueryRow(`
			SELECT is_historic, created_from_source, source_document_id
			FROM dim_address 
			WHERE address_id = $1
		`, result.BestCandidate.AddressID).Scan(&isHistoric, &createdFromSource, &sourceDocID)
		
		if err == nil {
			fmt.Printf("\n📊 HISTORIC RECORD VERIFICATION:\n")
			fmt.Printf("   is_historic: %v\n", isHistoric)
			fmt.Printf("   created_from_source: %v\n", createdFromSource)
			fmt.Printf("   source_document_id: %v\n", sourceDocID.Int32)
			
			if isHistoric && createdFromSource && sourceDocID.Valid && sourceDocID.Int32 == int32(docID) {
				fmt.Println("   ✅ All flags set correctly!")
			} else {
				fmt.Println("   ❌ Historic flags not set properly")
			}
		}
		
	} else {
		fmt.Println("❌ No match result returned")
	}

	// Check total historic records created
	var historicCount int
	db.QueryRow("SELECT COUNT(*) FROM dim_address WHERE is_historic = TRUE").Scan(&historicCount)
	fmt.Printf("\n📈 Total historic records in system: %d\n", historicCount)
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