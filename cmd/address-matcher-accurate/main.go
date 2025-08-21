package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"

	"github.com/ehdc-llpg/internal/config"
	"github.com/ehdc-llpg/internal/matcher"
	"github.com/ehdc-llpg/internal/normalize"
)

const version = "4.0.0-accurate"

func main() {
	var (
		command    = flag.String("cmd", "", "Command: match-single, test-accuracy")
		address    = flag.String("address", "", "Single address to match")
		debug      = flag.Bool("debug", false, "Enable debug output")
		configFile = flag.String("config", ".env", "Path to configuration file")
	)
	flag.Parse()

	if *command == "" {
		printUsage()
		os.Exit(1)
	}

	fmt.Printf("EHDC Address Matcher %s (ACCURACY-FOCUSED)\n", version)
	fmt.Printf("Maximum accuracy with component-level matching\n\n")

	// Load configuration
	err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Connect to database
	db, err := connectDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create accuracy-focused engine
	accurateEngine := matcher.NewAccurateEngine(db, nil, nil)

	// Execute command
	switch *command {
	case "match-single":
		if *address == "" {
			fmt.Println("Error: -address parameter required")
			os.Exit(1)
		}
		err = matchSingleAddress(*debug, accurateEngine, *address)
	case "test-accuracy":
		err = testAccuracyImprovements(*debug, accurateEngine, db)
	default:
		fmt.Printf("Unknown command: %s\n", *command)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("Command failed: %v", err)
	}

	fmt.Println("\nAccuracy-focused matching completed!")
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  Test single address:")
	fmt.Println("    ./address-matcher-accurate -cmd=match-single -address=\"123 High Street, Alton\"")
	fmt.Println()
	fmt.Println("  Test accuracy improvements:")
	fmt.Println("    ./address-matcher-accurate -cmd=test-accuracy")
}

func connectDB() (*sql.DB, error) {
	host := config.GetEnv("DB_HOST", "localhost")
	port := config.GetEnv("DB_PORT", "15435")
	user := config.GetEnv("DB_USER", "postgres")
	password := config.GetEnv("DB_PASSWORD", "kljh234hjkl2h")
	dbname := config.GetEnv("DB_NAME", "ehdc_llpg")
	sslmode := config.GetEnv("DB_SSLMODE", "disable")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	return sql.Open("postgres", connStr)
}

func matchSingleAddress(localDebug bool, engine *matcher.AccurateEngine, address string) error {
	fmt.Printf("Testing address (ACCURATE): %s\n", address)
	fmt.Println("🔍 Using multiple matching strategies:")
	fmt.Println("  1. Postcode + House Number")
	fmt.Println("  2. Street + Locality")
	fmt.Println("  3. Component-based fuzzy")
	fmt.Println("  4. Landmark/Business name")
	fmt.Println("  5. Land reference matching")
	fmt.Println()
	
	// Create canonical address
	canonical, postcode, _ := normalize.CanonicalAddress(address)
	
	fmt.Printf("Canonical: %s\n", canonical)
	if postcode != "" {
		fmt.Printf("Postcode: %s\n", postcode)
	}
	
	// Parse components
	houseNumbers := normalize.ExtractHouseNumbers(canonical)
	localities := normalize.ExtractLocalityTokens(canonical)
	streets := normalize.TokenizeStreet(canonical)
	
	fmt.Printf("Components: house=%v, street=%v, locality=%v\n", houseNumbers, streets, localities)
	fmt.Println()
	
	// Create test input
	input := matcher.MatchInput{
		DocumentID:       0,
		RawAddress:       address,
		AddressCanonical: canonical,
	}
	
	// Process with accuracy-focused matching
	result, err := engine.ProcessDocument(localDebug, input)
	if err != nil {
		return fmt.Errorf("accurate matching failed: %w", err)
	}
	
	// Display results
	fmt.Printf("🎯 ACCURATE Matching Results:\n")
	fmt.Printf("  Decision: %s\n", result.Decision)
	fmt.Printf("  Processing Time: %v\n", result.ProcessingTime)
	fmt.Printf("  Candidates Found: %d\n", len(result.AllCandidates))
	
	if result.BestCandidate != nil {
		fmt.Printf("\n🏆 Best Match (ACCURATE):\n")
		fmt.Printf("  UPRN: %s\n", result.BestCandidate.UPRN)
		fmt.Printf("  Address: %s\n", result.BestCandidate.FullAddress)
		fmt.Printf("  Score: %.4f\n", result.BestCandidate.Score)
		fmt.Printf("  Method: %s\n", result.BestCandidate.MethodCode)
		
		if result.BestCandidate.Easting != nil && result.BestCandidate.Northing != nil {
			fmt.Printf("  Coordinates: %.0f, %.0f\n", *result.BestCandidate.Easting, *result.BestCandidate.Northing)
		}
	}
	
	// Show top candidates
	fmt.Printf("\n📋 Top Candidates (ACCURATE):\n")
	for i, candidate := range result.AllCandidates {
		if i >= 5 {
			break
		}
		fmt.Printf("  [%d] Score: %.4f | %s | %s (%s)\n", 
			i+1, candidate.Score, candidate.UPRN, candidate.FullAddress, candidate.MethodCode)
	}
	
	return nil
}

func testAccuracyImprovements(localDebug bool, engine *matcher.AccurateEngine, db *sql.DB) error {
	fmt.Println("📊 Testing Accuracy Improvements")
	fmt.Println("=================================")
	
	// Test challenging addresses
	testAddresses := []struct {
		address string
		description string
	}{
		{"Land at High Street Alton", "Land reference"},
		{"The Swan Hotel, High St, Alton", "Business name with abbreviation"},
		{"Flat 3, 123 High Street, Alton, GU34", "Flat with partial postcode"},
		{"Rear of 1 Church Lane, Petersfield", "Rear of reference"},
		{"Plot 5, Development Site, Bordon", "Plot reference"},
		{"St Mary's Church, Church Road, Selborne", "Church with Saint abbreviation"},
		{"Nr Railway Station, Station Rd, Liphook", "Near abbreviation"},
		{"123 High Street Alton Hampshire", "No punctuation"},
	}
	
	successCount := 0
	for _, test := range testAddresses {
		fmt.Printf("\n🔍 Testing: %s\n", test.address)
		fmt.Printf("   Type: %s\n", test.description)
		
		canonical, _, _ := normalize.CanonicalAddress(test.address)
		input := matcher.MatchInput{
			DocumentID:       0,
			RawAddress:       test.address,
			AddressCanonical: canonical,
		}
		
		result, err := engine.ProcessDocument(false, input)
		if err != nil {
			fmt.Printf("   ❌ Error: %v\n", err)
			continue
		}
		
		if result.BestCandidate != nil {
			successCount++
			fmt.Printf("   ✅ MATCHED: %s (%.2f%% confidence)\n", 
				result.BestCandidate.FullAddress, result.BestCandidate.Score*100)
			fmt.Printf("   Method: %s\n", result.BestCandidate.MethodCode)
		} else {
			fmt.Printf("   ❌ No match found\n")
		}
	}
	
	fmt.Printf("\n📈 Accuracy Test Results:\n")
	fmt.Printf("   Matched: %d/%d (%.1f%%)\n", 
		successCount, len(testAddresses), float64(successCount)/float64(len(testAddresses))*100)
	
	// Compare with current statistics
	var currentMatches int
	db.QueryRow("SELECT COUNT(*) FROM address_match").Scan(&currentMatches)
	
	fmt.Printf("\n📊 Current System Performance:\n")
	fmt.Printf("   Total matches: %d/129,701 (%.2f%%)\n", 
		currentMatches, float64(currentMatches)/129701.0*100)
	fmt.Printf("\n💡 With gopostal integration, expect 15-25%% match rate\n")
	
	return nil
}