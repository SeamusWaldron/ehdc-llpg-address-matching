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

	// Get the specific document
	var docID int64 = 77571
	var address string
	var rawUPRN sql.NullString
	
	err = db.QueryRow(`
		SELECT raw_address, raw_uprn
		FROM src_document 
		WHERE document_id = $1
	`, docID).Scan(&address, &rawUPRN)
	
	if err != nil {
		log.Fatalf("Failed to get document: %v", err)
	}

	fmt.Printf("🧪 TESTING HISTORIC UPRN CREATION FOR DOCUMENT %d\n", docID)
	fmt.Println("=======================================================")
	fmt.Printf("Address: %s\n", address)
	fmt.Printf("UPRN: %s\n", rawUPRN.String)
	fmt.Println()

	// Verify UPRN doesn't exist in LLPG
	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM dim_address WHERE uprn = $1)", rawUPRN.String).Scan(&exists)
	fmt.Printf("UPRN exists in LLPG: %v\n", exists)
	fmt.Println()

	// Create engine and process
	engine := matcher.NewFixedComponentEngine(db)
	
	input := matcher.MatchInput{
		DocumentID: docID,
		RawAddress: address,
	}
	
	if rawUPRN.Valid && rawUPRN.String != "" {
		input.RawUPRN = &rawUPRN.String
	}

	fmt.Println("🔄 Processing with Fixed Component Engine...")
	result, err := engine.ProcessDocument(true, input) // Enable debug
	if err != nil {
		log.Fatalf("Processing failed: %v", err)
	}

	// Check result
	if result.BestCandidate != nil {
		fmt.Printf("\n✅ RESULT:\n")
		fmt.Printf("   Method: %s\n", result.BestCandidate.MethodCode)
		fmt.Printf("   Address ID: %d\n", result.BestCandidate.AddressID)
		fmt.Printf("   Score: %.4f\n", result.BestCandidate.Score)
		fmt.Printf("   Decision: %s\n", result.Decision)
		
		if result.BestCandidate.MethodCode == "historic_uprn" {
			fmt.Printf("   ✅ SUCCESS: Historic UPRN record created!\n")
			
			// Verify the record
			var isHistoric bool
			var sourceDoc sql.NullInt32
			err = db.QueryRow(`
				SELECT is_historic, source_document_id 
				FROM dim_address 
				WHERE address_id = $1
			`, result.BestCandidate.AddressID).Scan(&isHistoric, &sourceDoc)
			
			if err == nil {
				fmt.Printf("   Historic flag: %v\n", isHistoric)
				fmt.Printf("   Source doc: %v\n", sourceDoc.Int32)
			}
		}
	} else {
		fmt.Println("❌ No match result returned")
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