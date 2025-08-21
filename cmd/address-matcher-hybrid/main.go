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

const version = "3.2.0-hybrid"

func main() {
	var (
		command    = flag.String("cmd", "", "Command: match-all, match-type, match-single, stats")
		docType    = flag.String("type", "", "Document type: decision, land_charge, enforcement, agreement")
		address    = flag.String("address", "", "Single address to match")
		batchSize  = flag.Int("batch-size", 1000, "Batch size for processing (hybrid default)")
		debug      = flag.Bool("debug", false, "Enable debug output")
		configFile = flag.String("config", ".env", "Path to configuration file")
	)
	flag.Parse()

	if *command == "" {
		printUsage()
		os.Exit(1)
	}

	fmt.Printf("EHDC Address Matcher %s (HYBRID)\n", version)
	fmt.Printf("Fast DB pre-filtering + Advanced Go algorithms + Intelligent decisions\n\n")

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

	// Create hybrid matching components (no embeddings for now)
	hybridEngine := matcher.NewHybridEngine(db, nil, nil)
	hybridBatchProcessor := matcher.NewHybridBatchProcessor(hybridEngine, db)

	// Execute command
	switch *command {
	case "match-all":
		err = matchAllDocuments(*debug, hybridBatchProcessor, *batchSize)
	case "match-type":
		if *docType == "" {
			fmt.Println("Error: -type parameter required for match-type command")
			os.Exit(1)
		}
		err = matchDocumentsByType(*debug, hybridBatchProcessor, *docType, *batchSize)
	case "match-single":
		if *address == "" {
			fmt.Println("Error: -address parameter required for match-single command")
			os.Exit(1)
		}
		err = matchSingleAddress(*debug, hybridEngine, *address)
	case "stats":
		err = showStatistics(*debug, db)
	default:
		fmt.Printf("Unknown command: %s\n", *command)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("Command failed: %v", err)
	}

	fmt.Println("\nHybrid command completed successfully!")
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  Match all unmatched documents (hybrid):")
	fmt.Println("    ./address-matcher-hybrid -cmd=match-all -batch-size=1000")
	fmt.Println()
	fmt.Println("  Match documents by type (hybrid):")
	fmt.Println("    ./address-matcher-hybrid -cmd=match-type -type=decision")
	fmt.Println("    ./address-matcher-hybrid -cmd=match-type -type=land_charge")
	fmt.Println("    ./address-matcher-hybrid -cmd=match-type -type=enforcement")
	fmt.Println("    ./address-matcher-hybrid -cmd=match-type -type=agreement")
	fmt.Println()
	fmt.Println("  Test single address (hybrid):")
	fmt.Println("    ./address-matcher-hybrid -cmd=match-single -address=\"123 High Street, Alton\"")
	fmt.Println()
	fmt.Println("  Show matching statistics:")
	fmt.Println("    ./address-matcher-hybrid -cmd=stats")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -debug          Enable detailed debug output (shows all 3 stages)")
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

func matchAllDocuments(localDebug bool, bp *matcher.HybridBatchProcessor, batchSize int) error {
	fmt.Println("Starting HYBRID batch processing of all unmatched documents...")
	fmt.Println("🔄 Stage 1: Fast DB pre-filtering")
	fmt.Println("🧠 Stage 2: Advanced Go analysis (tokens, spatial, semantic)")
	fmt.Println("🎯 Stage 3: Intelligent decision making")
	fmt.Println()
	
	stats, err := bp.ProcessAllDocuments(localDebug, batchSize)
	if err != nil {
		return err
	}
	
	// Print comprehensive summary
	fmt.Printf("\n📊 HYBRID Batch Processing Summary:\n")
	fmt.Printf("  Total Documents:        %d\n", stats.TotalDocuments)
	fmt.Printf("  Successfully Processed: %d\n", stats.ProcessedCount)
	fmt.Printf("  ✅ Auto-Accepted:       %d (%.1f%%)\n", 
		stats.AutoAcceptCount, float64(stats.AutoAcceptCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  🔍 Needs Review:        %d (%.1f%%)\n", 
		stats.NeedsReviewCount, float64(stats.NeedsReviewCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  ❌ No Match:            %d (%.1f%%)\n", 
		stats.NoMatchCount, float64(stats.NoMatchCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  ⚠️  Errors:             %d (%.1f%%)\n", 
		stats.ErrorCount, float64(stats.ErrorCount)/float64(stats.TotalDocuments)*100)
	fmt.Printf("  📈 Average Score:       %.4f\n", stats.AverageScore)
	fmt.Printf("  ⏱️  Processing Time:     %v\n", stats.ProcessingTime)
	fmt.Printf("  🚀 Processing Rate:     %.1f docs/sec\n", 
		float64(stats.ProcessedCount)/stats.ProcessingTime.Seconds())
	
	// Hybrid-specific metrics
	fmt.Printf("\n🧠 Advanced Analysis Results:\n")
	fmt.Printf("  🎯 Very High Confidence: %d\n", stats.VeryHighConfidenceCount)
	fmt.Printf("  🟢 High Confidence:     %d\n", stats.HighConfidenceCount)  
	fmt.Printf("  🟡 Medium Confidence:   %d\n", stats.MediumConfidenceCount)
	fmt.Printf("  🟠 Low Confidence:      %d\n", stats.LowConfidenceCount)
	fmt.Printf("  🔴 Very Low Confidence: %d\n", stats.VeryLowConfidenceCount)
	fmt.Printf("  🏠 Token Enhanced:      %d\n", stats.TokenMatchCount)
	fmt.Printf("  📍 Spatial Enhanced:    %d\n", stats.SpatialMatchCount)
	fmt.Printf("  🧠 Semantic Enhanced:   %d\n", stats.SemanticMatchCount)
	fmt.Printf("  ⬆️  Avg Hybrid Boost:   %.4f\n", stats.HybridBoostAverage)
	
	return nil
}

func matchDocumentsByType(localDebug bool, bp *matcher.HybridBatchProcessor, docType string, batchSize int) error {
	fmt.Printf("Processing documents of type: %s (HYBRID)\n", docType)
	fmt.Println("🔄 Fast DB filtering → 🧠 Advanced Go analysis → 🎯 Intelligent decisions\n")
	
	stats, err := bp.ProcessDocumentsByType(localDebug, docType, batchSize)
	if err != nil {
		return err
	}
	
	// Print comprehensive summary
	fmt.Printf("\n📊 HYBRID Processing Summary for %s:\n", docType)
	fmt.Printf("  Total Documents:        %d\n", stats.TotalDocuments)
	fmt.Printf("  Successfully Processed: %d\n", stats.ProcessedCount)
	fmt.Printf("  ✅ Auto-Accepted:       %d (%.1f%%)\n", 
		stats.AutoAcceptCount, float64(stats.AutoAcceptCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  🔍 Needs Review:        %d (%.1f%%)\n", 
		stats.NeedsReviewCount, float64(stats.NeedsReviewCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  ❌ No Match:            %d (%.1f%%)\n", 
		stats.NoMatchCount, float64(stats.NoMatchCount)/float64(stats.ProcessedCount)*100)
	fmt.Printf("  ⚠️  Errors:             %d\n", stats.ErrorCount)
	fmt.Printf("  📈 Average Score:       %.4f\n", stats.AverageScore)
	fmt.Printf("  ⏱️  Processing Time:     %v\n", stats.ProcessingTime)
	fmt.Printf("  🚀 Processing Rate:     %.1f docs/sec\n", 
		float64(stats.ProcessedCount)/stats.ProcessingTime.Seconds())
	
	// Hybrid-specific metrics
	fmt.Printf("\n🧠 Advanced Analysis Breakdown:\n")
	fmt.Printf("  Confidence: VH=%d, H=%d, M=%d, L=%d, VL=%d\n",
		stats.VeryHighConfidenceCount, stats.HighConfidenceCount, stats.MediumConfidenceCount,
		stats.LowConfidenceCount, stats.VeryLowConfidenceCount)
	fmt.Printf("  Enhanced: Token=%d, Spatial=%d, Semantic=%d\n",
		stats.TokenMatchCount, stats.SpatialMatchCount, stats.SemanticMatchCount)
	fmt.Printf("  Average Hybrid Boost: %.4f\n", stats.HybridBoostAverage)
	
	return nil
}

func matchSingleAddress(localDebug bool, engine *matcher.HybridEngine, address string) error {
	fmt.Printf("Testing single address (HYBRID): %s\n", address)
	fmt.Println("🔄 Stage 1: Fast DB pre-filtering")
	fmt.Println("🧠 Stage 2: Advanced Go analysis")  
	fmt.Println("🎯 Stage 3: Intelligent decision making")
	fmt.Println()
	
	// Create canonical address
	canonical, postcode, _ := normalize.CanonicalAddress(address)
	
	fmt.Printf("Canonical form: %s\n", canonical)
	if postcode != "" {
		fmt.Printf("Extracted postcode: %s\n", postcode)
	}
	
	// Create test input
	input := matcher.MatchInput{
		DocumentID:       0, // Test document
		RawAddress:       address,
		AddressCanonical: canonical,
	}
	
	// Process the address with hybrid analysis
	result, err := engine.ProcessDocument(localDebug, input)
	if err != nil {
		return fmt.Errorf("hybrid matching failed: %w", err)
	}
	
	// Display results
	fmt.Printf("\n🎯 HYBRID Matching Results:\n")
	fmt.Printf("  Decision: %s\n", result.Decision)
	fmt.Printf("  Processing Time: %v\n", result.ProcessingTime)
	fmt.Printf("  Candidates Found: %d\n", len(result.AllCandidates))
	
	if result.BestCandidate != nil {
		fmt.Printf("\n🏆 Best Match (HYBRID Enhanced):\n")
		fmt.Printf("  UPRN: %s\n", result.BestCandidate.UPRN)
		fmt.Printf("  Address: %s\n", result.BestCandidate.FullAddress)
		fmt.Printf("  Hybrid Score: %.4f\n", result.BestCandidate.Score)
		fmt.Printf("  Method: %s\n", result.BestCandidate.MethodCode)
		
		if result.BestCandidate.Easting != nil && result.BestCandidate.Northing != nil {
			fmt.Printf("  Coordinates: %.0f, %.0f\n", *result.BestCandidate.Easting, *result.BestCandidate.Northing)
		}
		
		// Show hybrid analysis details
		if features := result.BestCandidate.Features; features != nil {
			if confidence, ok := features["confidence"].(string); ok {
				fmt.Printf("  Confidence Level: %s\n", confidence)
			}
			
			if prefilterScore, ok := features["prefilter_score"].(float64); ok {
				fmt.Printf("  Pre-filter Score: %.4f\n", prefilterScore)
				hybridBoost := result.BestCandidate.Score - prefilterScore
				fmt.Printf("  Hybrid Boost: +%.4f\n", hybridBoost)
			}
			
			if tokenAnalysis, ok := features["token_analysis"].(matcher.TokenAnalysis); ok {
				fmt.Printf("  🏠 Token Analysis: house=%t, street=%.3f, locality=%t, postcode=%t\n",
					tokenAnalysis.HouseNumberMatch, tokenAnalysis.StreetTokenMatch, 
					tokenAnalysis.LocalityMatch, tokenAnalysis.PostcodeMatch)
			}
			
			if spatialAnalysis, ok := features["spatial_analysis"].(matcher.SpatialAnalysis); ok {
				if spatialAnalysis.HasCoordinates {
					fmt.Printf("  📍 Spatial Analysis: distance=%.1fm, within_radius=%t\n",
						*spatialAnalysis.Distance, spatialAnalysis.WithinRadius)
				}
			}
		}
	}
	
	// Show top candidates with hybrid scores
	fmt.Printf("\n📋 Top Candidates (HYBRID):\n")
	for i, candidate := range result.AllCandidates {
		if i >= 5 { // Show top 5
			break
		}
		confidence := "unknown"
		if features := candidate.Features; features != nil {
			if conf, ok := features["confidence"].(string); ok {
				confidence = conf
			}
		}
		fmt.Printf("  [%d] Score: %.4f (%s) | %s | %s (%s)\n", 
			i+1, candidate.Score, confidence, candidate.UPRN, candidate.FullAddress, candidate.MethodCode)
	}
	
	return nil
}

func showStatistics(localDebug bool, db *sql.DB) error {
	fmt.Println("📊 HYBRID Address Matching Statistics")
	fmt.Println("=====================================")
	
	// Get basic statistics
	var totalDocs, matchedDocs, unmatchedDocs int
	var avgConfidence float64
	
	db.QueryRow("SELECT COUNT(*) FROM src_document").Scan(&totalDocs)
	db.QueryRow("SELECT COUNT(*) FROM address_match").Scan(&matchedDocs)
	unmatchedDocs = totalDocs - matchedDocs
	db.QueryRow("SELECT COALESCE(AVG(confidence_score), 0) FROM address_match").Scan(&avgConfidence)
	
	matchRate := float64(matchedDocs) / float64(totalDocs) * 100
	
	fmt.Printf("\n📈 Overall Statistics:\n")
	fmt.Printf("  Total Documents:      %d\n", totalDocs)
	fmt.Printf("  Matched Documents:    %d (%.1f%%)\n", matchedDocs, matchRate)
	fmt.Printf("  Unmatched Documents:  %d (%.1f%%)\n", unmatchedDocs, 100.0-matchRate)
	fmt.Printf("  Average Confidence:   %.4f\n", avgConfidence)
	
	// Method breakdown with hybrid detection
	fmt.Printf("\n⚙️ Method Breakdown:\n")
	rows, err := db.Query(`
		SELECT dm.method_name, COUNT(*), AVG(am.confidence_score),
		       COUNT(CASE WHEN am.matched_by = 'system_hybrid' THEN 1 END) as hybrid_count
		FROM address_match am
		INNER JOIN dim_match_method dm ON dm.method_id = am.match_method_id
		GROUP BY dm.method_id, dm.method_name
		ORDER BY COUNT(*) DESC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var methodName string
			var count, hybridCount int
			var avgScore float64
			if rows.Scan(&methodName, &count, &avgScore, &hybridCount) == nil {
				fmt.Printf("  %s: %d matches (avg: %.4f) [%d hybrid]\n", 
					methodName, count, avgScore, hybridCount)
			}
		}
	}
	
	// Hybrid-specific stats if available
	var hybridCount int
	db.QueryRow("SELECT COUNT(*) FROM address_match WHERE matched_by = 'system_hybrid'").Scan(&hybridCount)
	
	if hybridCount > 0 {
		fmt.Printf("\n🧠 Hybrid Enhancement Stats:\n")
		fmt.Printf("  Hybrid Processed:     %d\n", hybridCount)
		fmt.Printf("  Hybrid Enhancement:   %.1f%% of all matches\n", 
			float64(hybridCount)/float64(matchedDocs)*100)
	}
	
	return nil
}