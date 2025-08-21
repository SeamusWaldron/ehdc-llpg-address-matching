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

const version = "3.0.0-normalized-schema"

func main() {
	var (
		command    = flag.String("cmd", "", "Command: match-all, match-type, match-single, stats")
		docType    = flag.String("type", "", "Document type: decision, land_charge, enforcement, agreement")
		address    = flag.String("address", "", "Single address to match")
		batchSize  = flag.Int("batch-size", 1000, "Batch size for processing")
		debug      = flag.Bool("debug", false, "Enable debug output")
		configFile = flag.String("config", ".env", "Path to configuration file")
	)
	flag.Parse()

	if *command == "" {
		printUsage()
		os.Exit(1)
	}

	fmt.Printf("EHDC Address Matcher %s\\n", version)
	fmt.Printf("Using normalized schema with multi-tier matching\\n\\n")

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

	// Create matching engine (without embeddings for now)
	engine := matcher.NewEngine(db, nil, nil)
	batchProcessor := matcher.NewBatchProcessor(engine, db)

	// Execute command
	switch *command {
	case "match-all":
		err = matchAllDocuments(*debug, batchProcessor, *batchSize)
	case "match-type":
		if *docType == "" {
			fmt.Println("Error: -type parameter required for match-type command")
			os.Exit(1)
		}
		err = matchDocumentsByType(*debug, batchProcessor, *docType, *batchSize)
	case "match-single":
		if *address == "" {
			fmt.Println("Error: -address parameter required for match-single command")
			os.Exit(1)
		}
		err = matchSingleAddress(*debug, engine, *address)
	case "stats":
		err = showStatistics(*debug, batchProcessor)
	default:
		fmt.Printf("Unknown command: %s\\n", *command)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("Command failed: %v", err)
	}

	fmt.Println("\\nCommand completed successfully!")
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  Match all unmatched documents:")
	fmt.Println("    ./address-matcher -cmd=match-all -batch-size=1000")
	fmt.Println()
	fmt.Println("  Match documents by type:")
	fmt.Println("    ./address-matcher -cmd=match-type -type=decision")
	fmt.Println("    ./address-matcher -cmd=match-type -type=land_charge")
	fmt.Println("    ./address-matcher -cmd=match-type -type=enforcement")
	fmt.Println("    ./address-matcher -cmd=match-type -type=agreement")
	fmt.Println()
	fmt.Println("  Test single address:")
	fmt.Println("    ./address-matcher -cmd=match-single -address=\\\"123 High Street, Alton\\\"")
	fmt.Println()
	fmt.Println("  Show matching statistics:")
	fmt.Println("    ./address-matcher -cmd=stats")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -debug          Enable detailed debug output")
	fmt.Println("  -config         Path to configuration file (default: .env)")
	fmt.Println("  -batch-size     Batch size for processing (default: 1000)")
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

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	// Test connection
	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func matchAllDocuments(localDebug bool, bp *matcher.BatchProcessor, batchSize int) error {
	fmt.Println("Starting batch processing of all unmatched documents...")
	
	stats, err := bp.ProcessAllDocuments(localDebug, batchSize)
	if err != nil {
		return err
	}
	
	// Print summary
	fmt.Printf("\\n📊 Batch Processing Summary:\\n")
	fmt.Printf("  Total Documents:     %d\\n", stats.TotalDocuments)
	fmt.Printf("  Successfully Processed: %d\\n", stats.ProcessedCount)
	fmt.Printf("  ✅ Auto-Accepted:    %d (%.1f%%)\\n", 
		stats.AutoAcceptCount, float64(stats.AutoAcceptCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  🔍 Needs Review:     %d (%.1f%%)\\n", 
		stats.NeedsReviewCount, float64(stats.NeedsReviewCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  ❌ No Match:         %d (%.1f%%)\\n", 
		stats.NoMatchCount, float64(stats.NoMatchCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  ⚠️  Errors:          %d (%.1f%%)\\n", 
		stats.ErrorCount, float64(stats.ErrorCount)/float64(stats.TotalDocuments)*100)
	fmt.Printf("  📈 Average Score:    %.4f\\n", stats.AverageScore)
	fmt.Printf("  ⏱️  Processing Time:  %v\\n", stats.ProcessingTime)
	
	return nil
}

func matchDocumentsByType(localDebug bool, bp *matcher.BatchProcessor, docType string, batchSize int) error {
	fmt.Printf("Processing documents of type: %s\\n", docType)
	
	stats, err := bp.ProcessDocumentsByType(localDebug, docType, batchSize)
	if err != nil {
		return err
	}
	
	// Print summary
	fmt.Printf("\\n📊 Processing Summary for %s:\\n", docType)
	fmt.Printf("  Total Documents:     %d\\n", stats.TotalDocuments)
	fmt.Printf("  Successfully Processed: %d\\n", stats.ProcessedCount)
	fmt.Printf("  ✅ Auto-Accepted:    %d (%.1f%%)\\n", 
		stats.AutoAcceptCount, float64(stats.AutoAcceptCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  🔍 Needs Review:     %d (%.1f%%)\\n", 
		stats.NeedsReviewCount, float64(stats.NeedsReviewCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  ❌ No Match:         %d (%.1f%%)\\n", 
		stats.NoMatchCount, float64(stats.NoMatchCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  ⚠️  Errors:          %d\\n", stats.ErrorCount)
	fmt.Printf("  📈 Average Score:    %.4f\\n", stats.AverageScore)
	fmt.Printf("  ⏱️  Processing Time:  %v\\n", stats.ProcessingTime)
	
	return nil
}

func matchSingleAddress(localDebug bool, engine *matcher.Engine, address string) error {
	fmt.Printf("Testing single address: %s\\n", address)
	
	// Create canonical address
	canonical, postcode, _ := normalize.CanonicalAddress(address)
	
	fmt.Printf("Canonical form: %s\\n", canonical)
	if postcode != "" {
		fmt.Printf("Extracted postcode: %s\\n", postcode)
	}
	
	// Create test input
	input := matcher.MatchInput{
		DocumentID:       0, // Test document
		RawAddress:       address,
		AddressCanonical: canonical,
	}
	
	// Process the address
	result, err := engine.ProcessDocument(localDebug, input)
	if err != nil {
		return fmt.Errorf("matching failed: %w", err)
	}
	
	// Display results
	fmt.Printf("\\n🎯 Matching Results:\\n")
	fmt.Printf("  Decision: %s\\n", result.Decision)
	fmt.Printf("  Processing Time: %v\\n", result.ProcessingTime)
	fmt.Printf("  Candidates Found: %d\\n", len(result.AllCandidates))
	
	if result.BestCandidate != nil {
		fmt.Printf("\\n🏆 Best Match:\\n")
		fmt.Printf("  UPRN: %s\\n", result.BestCandidate.UPRN)
		fmt.Printf("  Address: %s\\n", result.BestCandidate.FullAddress)
		fmt.Printf("  Score: %.4f\\n", result.BestCandidate.Score)
		fmt.Printf("  Method: %s\\n", result.BestCandidate.MethodCode)
		
		if result.BestCandidate.Easting != nil && result.BestCandidate.Northing != nil {
			fmt.Printf("  Coordinates: %.0f, %.0f\\n", *result.BestCandidate.Easting, *result.BestCandidate.Northing)
		}
	}
	
	// Show top candidates
	fmt.Printf("\\n📋 Top Candidates:\\n")
	for i, candidate := range result.AllCandidates {
		if i >= 5 { // Show top 5
			break
		}
		fmt.Printf("  [%d] Score: %.4f | %s | %s (%s)\\n", 
			i+1, candidate.Score, candidate.UPRN, candidate.FullAddress, candidate.MethodCode)
	}
	
	return nil
}

func showStatistics(localDebug bool, bp *matcher.BatchProcessor) error {
	fmt.Println("📊 Address Matching Statistics")
	fmt.Println("==============================")
	
	stats, err := bp.GetMatchingStatistics(localDebug)
	if err != nil {
		return fmt.Errorf("failed to get statistics: %w", err)
	}
	
	fmt.Printf("\\n📈 Overall Statistics:\\n")
	fmt.Printf("  Total Documents:      %d\\n", stats.TotalDocuments)
	fmt.Printf("  Matched Documents:    %d (%.1f%%)\\n", 
		stats.MatchedDocuments, float64(stats.MatchedDocuments)/float64(stats.TotalDocuments)*100)
	fmt.Printf("  Unmatched Documents:  %d (%.1f%%)\\n", 
		stats.UnmatchedDocuments, float64(stats.UnmatchedDocuments)/float64(stats.TotalDocuments)*100)
	fmt.Printf("  Average Confidence:   %.4f\\n", stats.AverageConfidence)
	
	fmt.Printf("\\n🔍 Match Status Breakdown:\\n")
	for status, count := range stats.StatusBreakdown {
		fmt.Printf("  %s: %d\\n", status, count)
	}
	
	fmt.Printf("\\n⚙️ Method Breakdown:\\n")
	for method, methodStats := range stats.MethodBreakdown {
		fmt.Printf("  %s: %d matches (avg score: %.4f)\\n", 
			method, methodStats.Count, methodStats.AvgScore)
	}
	
	return nil
}