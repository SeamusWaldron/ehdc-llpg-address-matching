package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/ehdc-llpg/internal/audit"
	"github.com/ehdc-llpg/internal/config"
	"github.com/ehdc-llpg/internal/debug"
	"github.com/ehdc-llpg/internal/embeddings"
	"github.com/ehdc-llpg/internal/etl"
	"github.com/ehdc-llpg/internal/llpg"
	"github.com/ehdc-llpg/internal/match"
	"github.com/ehdc-llpg/internal/phonetics"
	"github.com/ehdc-llpg/internal/validation"
	"github.com/ehdc-llpg/internal/vector"
)

const version = "2.0.0-proper-algorithm"

func main() {
	var (
		command     = flag.String("cmd", "", "Command to run: setup-db, load-llpg, load-os-uprn, load-sources, validate-uprns, expand-llpg-ranges, setup-vector, match-batch, match-single, conservative-match, apply-corrections, fuzzy-match-groups, fuzzy-match-individual, layer3-parallel-groups, layer3-parallel-docs, layer3-parallel-combined, standardize-addresses, comprehensive-match, llm-fix-addresses, rebuild-fact, validate-integrity, stats")
		llpgFile    = flag.String("llpg", "", "Path to LLPG CSV file")
		osUprnFile  = flag.String("os-uprn", "", "Path to OS Open UPRN CSV file")
		sourceFiles = flag.String("sources", "", "Comma-separated paths to source CSV files (type:path,type:path)")
		_  = flag.String("source-type", "", "Source type for single file load (decision, land_charge, enforcement, agreement)") // Currently unused
		address     = flag.String("address", "", "Single address to match")
		runLabel    = flag.String("run-label", "", "Label for matching run")
		debug       = flag.Bool("debug", false, "Enable debug output")
		configFile  = flag.String("config", ".env", "Path to configuration file")
		batchSize   = flag.Int("batch-size", 50000, "Batch size for OS UPRN loading")
	)
	flag.Parse()

	if *command == "" {
		printUsage()
		os.Exit(1)
	}

	fmt.Printf("EHDC LLPG Address Matcher %s\n", version)
	fmt.Printf("Implementing ADDRESS_MATCHING_ALGORITHM.md specification\n\n")

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

	// Execute command
	switch *command {
	case "setup-db":
		err = setupDatabase(*debug, db)
	case "load-llpg":
		err = loadLLPG(*debug, db, *llpgFile)
	case "load-os-uprn":
		err = loadOSUPRN(*debug, db, *osUprnFile, *batchSize)
	case "load-sources":
		err = loadSourceDocuments(*debug, db, *sourceFiles)
	case "validate-uprns":
		err = validateUPRNs(*debug, db)
	case "expand-llpg-ranges":
		err = expandLLPGRanges(*debug, db)
	case "setup-vector":
		err = setupVectorDB(*debug, db)
	case "match-batch":
		err = runBatchMatching(*debug, db, *runLabel)
	case "match-single":
		err = runSingleMatch(*debug, db, *address)
	case "conservative-match":
		err = runConservativeMatching(*debug, db, *runLabel)
	case "apply-corrections":
		err = applyGroupConsensusCorrections(*debug, db)
	case "fuzzy-match-groups":
		err = fuzzyMatchUnmatchedGroups(*debug, db)
	case "fuzzy-match-individual":
		err = fuzzyMatchIndividualDocuments(*debug, db)
	case "standardize-addresses":
		err = standardizeSourceAddresses(*debug, db)
	case "comprehensive-match":
		err = runComprehensiveMatching(*debug, db)
	case "conservative-only":
		err = runConservativeMatching(*debug, db, "conservative-test")
	case "clean-source-data":
		err = cleanSourceAddressData(*debug, db)
	case "llm-fix-addresses":
		err = llmFixLowConfidenceAddresses(*debug, db)
	case "rebuild-fact":
		err = rebuildFactTable(*debug, db)
	case "rebuild-fact-intelligent":
		err = rebuildFactTableIntelligent(*debug, db)
	case "rebuild-fact-simple":
		err = rebuildFactTableSimple(*debug, db)
	case "layer2-only":
		err = runLayer2Only(*debug, db)
	case "layer2-optimized":
		err = runOptimizedLayer2(*debug, db)
	case "layer2-parallel":
		err = runParallelLayer2(*debug, db)
	case "layer3-parallel-groups":
		err = runParallelLayer3Groups(*debug, db)
	case "layer3-parallel-docs":
		err = runParallelLayer3Documents(*debug, db)
	case "layer3-parallel-combined":
		err = runParallelLayer3Combined(*debug, db)
	case "validate-integrity":
		err = validateDataIntegrity(*debug, db)
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

	fmt.Println("Command completed successfully!")
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  Setup database schema:")
	fmt.Println("    ./matcher-v2 -cmd=setup-db")
	fmt.Println()
	fmt.Println("  Load EHDC LLPG data (71K records):")
	fmt.Println("    ./matcher-v2 -cmd=load-llpg -llpg=llpg_docs/ehdc_llpg_20250710.csv")
	fmt.Println()
	fmt.Println("  Load OS Open UPRN data (41M records):")
	fmt.Println("    ./matcher-v2 -cmd=load-os-uprn -os-uprn=llpg_docs/osopenuprn_202507.csv -batch-size=100000")
	fmt.Println()
	fmt.Println("  Validate legacy UPRNs:")
	fmt.Println("    ./matcher-v2 -cmd=validate-uprns")
	fmt.Println()
	fmt.Println("  Load source documents:")
	fmt.Println("    ./matcher-v2 -cmd=load-sources -sources=decision:/path/to/decisions.csv,land_charge:/path/to/charges.csv")
	fmt.Println()
	fmt.Println("  Setup vector database:")
	fmt.Println("    ./matcher-v2 -cmd=setup-vector")
	fmt.Println()
	fmt.Println("  Run batch matching:")
	fmt.Println("    ./matcher-v2 -cmd=match-batch -run-label=\"v2.0-initial\"")
	fmt.Println()
	fmt.Println("  Test single address:")
	fmt.Println("    ./matcher-v2 -cmd=match-single -address=\"123 Main Street, Alton\"")
	fmt.Println()
	fmt.Println("  Apply group consensus corrections:")
	fmt.Println("    ./matcher-v2 -cmd=apply-corrections")
	fmt.Println()
	fmt.Println("  Find fuzzy matches for unmatched groups:")
	fmt.Println("    ./matcher-v2 -cmd=fuzzy-match-groups")
	fmt.Println()
	fmt.Println("  Find fuzzy matches for individual documents:")
	fmt.Println("    ./matcher-v2 -cmd=fuzzy-match-individual")
	fmt.Println()
	fmt.Println("  Run parallel Layer 3a (group-based fuzzy matching):")
	fmt.Println("    ./matcher-v2 -cmd=layer3-parallel-groups")
	fmt.Println()
	fmt.Println("  Run parallel Layer 3b (individual document fuzzy matching):")
	fmt.Println("    ./matcher-v2 -cmd=layer3-parallel-docs")
	fmt.Println()
	fmt.Println("  Run complete parallel Layer 3 (both 3a and 3b):")
	fmt.Println("    ./matcher-v2 -cmd=layer3-parallel-combined")
	fmt.Println()
	fmt.Println("  Standardize and clean source addresses:")
	fmt.Println("    ./matcher-v2 -cmd=standardize-addresses")
	fmt.Println()
	fmt.Println("  Run comprehensive multi-layered matching:")
	fmt.Println("    ./matcher-v2 -cmd=comprehensive-match")
	fmt.Println()
	fmt.Println("  Fix low confidence addresses using LLM:")
	fmt.Println("    ./matcher-v2 -cmd=llm-fix-addresses")
	fmt.Println()
	fmt.Println("  Rebuild fact table with corrections:")
	fmt.Println("    ./matcher-v2 -cmd=rebuild-fact")
	fmt.Println()
	fmt.Println("  Validate data integrity:")
	fmt.Println("    ./matcher-v2 -cmd=validate-integrity")
	fmt.Println()
	fmt.Println("  Show statistics:")
	fmt.Println("    ./matcher-v2 -cmd=stats")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -debug          Enable detailed debug output")
	fmt.Println("  -config         Path to configuration file (default: .env)")
	fmt.Println("  -batch-size     Batch size for large data loads (default: 50000)")
}

func connectDB() (*sql.DB, error) {
	host := config.GetEnv("DB_HOST", "")
	port := config.GetEnv("DB_PORT", "")
	user := config.GetEnv("DB_USER", "")
	password := config.GetEnv("DB_PASSWORD", "")
	dbname := config.GetEnv("DB_NAME", "")
	sslmode := config.GetEnv("DB_SSLMODE", "disable")

	if host == "" || port == "" || user == "" || password == "" || dbname == "" {
		return nil, fmt.Errorf("missing required database environment variables: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME")
	}

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

func setupDatabase(localDebug bool, db *sql.DB) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	fmt.Println("Setting up database schema...")

	// Read and execute schema SQL (using no-PostGIS version for now)
	schemaSQL, err := os.ReadFile("migrations/001_no_postgis.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	_, err = db.Exec(string(schemaSQL))
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	fmt.Println("Database schema created successfully!")
	return nil
}

func loadLLPG(localDebug bool, db *sql.DB, csvPath string) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	if csvPath == "" {
		return fmt.Errorf("LLPG CSV path is required")
	}

	fmt.Printf("Loading LLPG from: %s\n", csvPath)

	pipeline := etl.NewPipeline(db)
	return pipeline.LoadLLPG(localDebug, csvPath)
}

func loadOSUPRN(localDebug bool, db *sql.DB, csvPath string, batchSize int) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	if csvPath == "" {
		return fmt.Errorf("OS UPRN CSV path is required")
	}

	fmt.Printf("Loading OS Open UPRN data from: %s\n", csvPath)
	fmt.Printf("Batch size: %d records\n", batchSize)
	fmt.Printf("⚠️  This will process 41+ million records and may take 1-2 hours\n")

	osLoader := etl.NewOSDataLoader(db)
	return osLoader.LoadOSOpenUPRN(localDebug, csvPath, batchSize)
}

func validateUPRNs(localDebug bool, db *sql.DB) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	fmt.Println("Validating legacy UPRNs against EHDC LLPG and OS Open UPRN datasets...")

	osLoader := etl.NewOSDataLoader(db)
	
	// Generate validation report
	report, err := osLoader.ValidateLegacyUPRNs(localDebug)
	if err != nil {
		return fmt.Errorf("failed to validate UPRNs: %w", err)
	}

	// Display results
	fmt.Printf("\n📊 UPRN Validation Report:\n")
	fmt.Printf("  Total documents with UPRN: %d\n", report.TotalWithUPRN)
	fmt.Printf("  ✅ Valid in EHDC LLPG:     %d (%.1f%%)\n", 
		report.ValidInEHDCLLPG, 
		float64(report.ValidInEHDCLLPG)/float64(report.TotalWithUPRN)*100)
	fmt.Printf("  📍 Valid in OS data only:  %d (%.1f%%)\n", 
		report.ValidInOSOnly,
		float64(report.ValidInOSOnly)/float64(report.TotalWithUPRN)*100)
	fmt.Printf("  ❌ Invalid UPRNs:          %d (%.1f%%)\n", 
		report.Invalid,
		float64(report.Invalid)/float64(report.TotalWithUPRN)*100)

	// Enrich EHDC addresses with OS coordinates where missing
	fmt.Println("\nEnriching EHDC addresses with OS coordinate data...")
	err = osLoader.EnrichCoordinates(localDebug)
	if err != nil {
		return fmt.Errorf("failed to enrich coordinates: %w", err)
	}

	return nil
}

func loadSourceDocuments(localDebug bool, db *sql.DB, sourceFiles string) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	if sourceFiles == "" {
		return fmt.Errorf("source files specification is required")
	}

	fmt.Printf("Loading source documents: %s\n", sourceFiles)

	pipeline := etl.NewPipeline(db)

	// Parse source files (format: type:path,type:path)
	if strings.Contains(sourceFiles, ":") {
		// Handle type:path format
		pairs := strings.Split(sourceFiles, ",")
		for _, pair := range pairs {
			parts := strings.SplitN(pair, ":", 2)
			if len(parts) != 2 {
				fmt.Printf("Invalid format: %s (expected type:path)\n", pair)
				continue
			}
			sourceType := strings.TrimSpace(parts[0])
			filePath := strings.TrimSpace(parts[1])
			
			fmt.Printf("Loading %s documents from: %s\n", sourceType, filePath)
			err := pipeline.LoadSourceDocuments(localDebug, sourceType, filePath)
			if err != nil {
				fmt.Printf("Error loading %s: %v\n", sourceType, err)
			}
		}
	} else {
		// Load all source files from source_docs directory
		fmt.Println("Loading all source documents from source_docs/ directory")
		sourceFiles := map[string]string{
			"decision":    "source_docs/decision_notices.csv",
			"land_charge": "source_docs/land_charges_cards.csv",
			"enforcement": "source_docs/enforcement_notices.csv",
			"agreement":   "source_docs/agreements.csv",
		}
		
		for sourceType, filePath := range sourceFiles {
			if _, err := os.Stat(filePath); err == nil {
				fmt.Printf("Loading %s documents from: %s\n", sourceType, filePath)
				err := pipeline.LoadSourceDocuments(localDebug, sourceType, filePath)
				if err != nil {
					fmt.Printf("Error loading %s: %v\n", sourceType, err)
				}
			} else {
				fmt.Printf("Skipping %s (file not found): %s\n", sourceType, filePath)
			}
		}
	}

	return nil
}

func setupVectorDB(localDebug bool, db *sql.DB) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	fmt.Println("Setting up vector database...")

	// Configure Qdrant connection
	qdrantConfig := vector.QdrantConfig{
		Host:    config.GetEnv("QDRANT_HOST", "localhost"),
		Port:    config.GetEnvInt("QDRANT_PORT", 6333),
		APIKey:  config.GetEnv("QDRANT_API_KEY", ""),
		Timeout: 30 * time.Second,
	}

	qdrantClient := vector.NewQdrantClient(qdrantConfig)
	vectorDB := vector.NewAddressVectorDB(qdrantClient, "ehdc_addresses", 384) // 384 for sentence-transformers

	err := vectorDB.Initialize(localDebug)
	if err != nil {
		return fmt.Errorf("failed to initialize vector database: %w", err)
	}

	fmt.Println("Vector database initialized!")
	fmt.Println("Note: You'll need to run embedding generation separately")
	fmt.Println("This requires an embedding service (Ollama, TEI, etc.)")

	return nil
}

func runBatchMatching(localDebug bool, db *sql.DB, runLabel string) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	if runLabel == "" {
		runLabel = fmt.Sprintf("batch-%d", time.Now().Unix())
	}

	fmt.Printf("Running batch matching with label: %s\n", runLabel)

	// Create matching engine
	engine := createMatchingEngine(db)

	// Get unmatched source documents
	inputs, err := getUnmatchedDocuments(db, localDebug)
	if err != nil {
		return fmt.Errorf("failed to get unmatched documents: %w", err)
	}

	fmt.Printf("Found %d documents to process\n", len(inputs))

	if len(inputs) == 0 {
		fmt.Println("No unmatched documents found!")
		return nil
	}

	// Process in batches
	batchSize := config.GetEnvInt("MATCH_BATCH_SIZE", 100)
	results, err := engine.BatchProcess(localDebug, inputs, batchSize)
	if err != nil {
		return fmt.Errorf("batch processing failed: %w", err)
	}

	// Save results
	err = engine.SaveResults(localDebug, results, runLabel)
	if err != nil {
		return fmt.Errorf("failed to save results: %w", err)
	}

	// Show summary statistics
	stats := calculateBatchStats(results)
	fmt.Printf("\nBatch Processing Complete:\n")
	fmt.Printf("  Total Processed:  %d\n", stats.Total)
	fmt.Printf("  Auto-Accepted:    %d (%.1f%%)\n", stats.AutoAccept, float64(stats.AutoAccept)/float64(stats.Total)*100)
	fmt.Printf("  Needs Review:     %d (%.1f%%)\n", stats.Review, float64(stats.Review)/float64(stats.Total)*100)
	fmt.Printf("  Rejected:         %d (%.1f%%)\n", stats.Reject, float64(stats.Reject)/float64(stats.Total)*100)
	fmt.Printf("  Errors:           %d (%.1f%%)\n", stats.Error, float64(stats.Error)/float64(stats.Total)*100)

	return nil
}

func runSingleMatch(localDebug bool, db *sql.DB, address string) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	if address == "" {
		return fmt.Errorf("address is required for single match test")
	}

	fmt.Printf("Testing single address match: %s\n", address)

	// Create matching engine
	engine := createMatchingEngine(db)

	// Create input
	input := match.Input{
		SrcID:      0, // Test input
		RawAddress: address,
		SourceType: "test",
	}

	// Process single address
	result, err := engine.SuggestUPRN(localDebug, input)
	if err != nil {
		return fmt.Errorf("matching failed: %w", err)
	}

	// Show results
	fmt.Printf("\nMatching Results:\n")
	fmt.Printf("  Decision: %s\n", result.Decision)
	fmt.Printf("  Processing Time: %v\n", result.ProcessingTime)
	fmt.Printf("  Candidates Found: %d\n", len(result.Candidates))

	if result.AcceptedUPRN != "" {
		fmt.Printf("  Accepted UPRN: %s\n", result.AcceptedUPRN)
	}

	// Show top 3 candidates
	for i, candidate := range result.Candidates {
		if i >= 3 {
			break
		}
		fmt.Printf("  [%d] UPRN: %s, Score: %.4f, Address: %s\n", 
			i+1, candidate.UPRN, candidate.Score, candidate.LocAddress)
		fmt.Printf("      Methods: %v\n", candidate.Methods)
	}

	// Show explanation for top candidate if available
	if len(result.Candidates) > 0 {
		explanation := engine.GetExplanation(result)
		fmt.Printf("\nDetailed Explanation:\n")
		for key, value := range explanation {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	return nil
}

func showStatistics(localDebug bool, db *sql.DB) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	fmt.Println("Showing matching statistics...")

	tracker := audit.NewTracker(db)

	// Get latest run ID
	var latestRunID int64
	err := db.QueryRow("SELECT run_id FROM match_run ORDER BY run_started_at DESC LIMIT 1").Scan(&latestRunID)
	if err != nil {
		return fmt.Errorf("no matching runs found: %w", err)
	}

	stats, err := tracker.GetMatchingStatistics(localDebug, latestRunID)
	if err != nil {
		return fmt.Errorf("failed to get statistics: %w", err)
	}

	fmt.Printf("Latest Run Statistics (Run ID: %d)\n", stats.RunID)
	fmt.Printf("  Label: %s\n", stats.RunLabel)
	fmt.Printf("  Started: %s\n", stats.RunStartedAt.Format(time.RFC3339))
	fmt.Printf("  Total Processed: %d\n", stats.TotalProcessed)
	fmt.Printf("\nDecision Breakdown:\n")
	for decision, decisionStats := range stats.DecisionBreakdown {
		fmt.Printf("  %s: %d\n", decision, decisionStats.Count)
	}
	fmt.Printf("\nMethod Breakdown:\n")
	for method, methodStats := range stats.MethodBreakdown {
		fmt.Printf("  %s: %d (avg score: %.3f)\n", method, methodStats.Count, methodStats.AvgScore)
	}

	return nil
}

// VectorDBAdapter adapts embeddings.NoOpVectorDB to match.VectorDB interface
type VectorDBAdapter struct {
	underlying *embeddings.NoOpVectorDB
}

func (a *VectorDBAdapter) Query(vector []float32, limit int) ([]match.VectorResult, error) {
	results, err := a.underlying.Query(vector, limit)
	if err != nil {
		return nil, err
	}
	
	// Convert embeddings.VectorResult to match.VectorResult
	matchResults := make([]match.VectorResult, len(results))
	for i, result := range results {
		matchResults[i] = match.VectorResult{
			UPRN:  result.UPRN,
			Score: result.Score,
		}
	}
	
	return matchResults, nil
}

func (a *VectorDBAdapter) GetVector(uprn string) ([]float32, error) {
	return a.underlying.GetVector(uprn)
}

func createMatchingEngine(db *sql.DB) *match.Engine {
	// Create configured matching engine with all components
	embedder := embeddings.NewSimpleEmbedder(384) // 384 dimensions for compatibility
	noOpVectorDB := embeddings.NewNoOpVectorDB()  // Simple implementation for now
	vectorDB := &VectorDBAdapter{underlying: noOpVectorDB}
	phoneticsEngine := phonetics.NewSimplePhonetics()
	
	config := match.EngineConfig{
		DB:        db,
		VectorDB:  vectorDB,
		Embedder:  embedder,
		Parser:    nil, // Optional - can work without libpostal
		Phonetics: phoneticsEngine,
	}

	return match.NewEngine(config)
}

func getUnmatchedDocuments(db *sql.DB, localDebug bool) ([]match.Input, error) {
	debug.DebugOutput(localDebug, "Querying unmatched source documents")

	rows, err := db.Query(`
		SELECT s.src_id, s.raw_address, s.source_type, s.uprn_raw, s.easting_raw, s.northing_raw, s.doc_date
		FROM src_document s
		LEFT JOIN match_accepted m ON m.src_id = s.src_id
		WHERE m.src_id IS NULL
		  AND s.raw_address IS NOT NULL
		  AND s.raw_address != ''
		ORDER BY s.src_id
		LIMIT 10000
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var inputs []match.Input
	for rows.Next() {
		var srcID int64
		var rawAddress, sourceType, uprnRaw string
		var eastingRaw, northingRaw sql.NullString
		var docDate sql.NullTime

		err := rows.Scan(&srcID, &rawAddress, &sourceType, &uprnRaw, &eastingRaw, &northingRaw, &docDate)
		if err != nil {
			debug.DebugOutput(localDebug, "Error scanning document %d: %v", srcID, err)
			continue
		}

		input := match.Input{
			SrcID:        srcID,
			RawAddress:   rawAddress,
			LegacyUPRN:   uprnRaw,
			SourceType:   sourceType,
		}

		// Parse coordinates if available
		if eastingRaw.Valid && northingRaw.Valid {
			if easting, err := parseFloat(eastingRaw.String); err == nil {
				input.Easting = &easting
			}
			if northing, err := parseFloat(northingRaw.String); err == nil {
				input.Northing = &northing
			}
		}

		// Parse date if available
		if docDate.Valid {
			input.DocDate = &docDate.Time
		}

		inputs = append(inputs, input)
	}

	return inputs, nil
}

func calculateBatchStats(results []match.Result) struct {
	Total      int
	AutoAccept int
	Review     int
	Reject     int
	Error      int
} {
	stats := struct {
		Total      int
		AutoAccept int
		Review     int
		Reject     int
		Error      int
	}{Total: len(results)}

	for _, result := range results {
		switch result.Decision {
		case "auto_accept":
			stats.AutoAccept++
		case "review":
			stats.Review++
		case "reject":
			stats.Reject++
		case "error":
			stats.Error++
		}
	}

	return stats
}

func parseFloat(s string) (float64, error) {
	if s == "" {
		return 0.0, fmt.Errorf("empty string")
	}
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// applyGroupConsensusCorrections identifies and applies safe group consensus corrections
func applyGroupConsensusCorrections(localDebug bool, db *sql.DB) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	fmt.Println("Applying group consensus corrections...")

	// Step 1: Create real address validation function
	createValidationSQL := `
CREATE OR REPLACE FUNCTION is_real_address(address_text TEXT) 
RETURNS BOOLEAN AS $func$
BEGIN
    -- Check if this looks like a real address vs planning reference
    IF address_text IS NULL OR LENGTH(TRIM(address_text)) < 10 THEN
        RETURN FALSE;
    END IF;
    
    -- Planning reference patterns (F12345, AU123, etc.)
    IF address_text ~ '^[A-Z]{1,3}[0-9]+/?[0-9]*$' THEN
        RETURN FALSE;
    END IF;
    
    -- N/A and similar non-addresses
    IF UPPER(address_text) IN ('N/A', 'NOT APPLICABLE', 'NONE', 'NULL', 'TBC') THEN
        RETURN FALSE;
    END IF;
    
    -- Must contain typical address indicators
    IF address_text ~* '(street|road|avenue|lane|way|close|drive|court|place|crescent|gardens|park|hill|view|house|cottage|farm|manor|hall)' 
       OR address_text ~ ',' THEN  -- Has commas (typical of addresses)
        RETURN TRUE;
    END IF;
    
    RETURN FALSE;
END;
$func$ LANGUAGE plpgsql IMMUTABLE;`

	_, err := db.Exec(createValidationSQL)
	if err != nil {
		return fmt.Errorf("failed to create validation function: %v", err)
	}

	// Step 2: Find safe group consensus candidates
	analysisSQL := `
WITH safe_group_candidates AS (
    SELECT 
        s.planning_app_base,
        COUNT(*) as total_docs,
        COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0) as matched_docs,
        COUNT(DISTINCT am.address_id) FILTER (WHERE am.address_id IS NOT NULL AND am.confidence_score > 0.5) as unique_matched_addresses,
        
        -- Count real addresses vs planning refs
        COUNT(*) FILTER (WHERE is_real_address(s.raw_address)) as real_addresses,
        COUNT(*) FILTER (WHERE NOT is_real_address(s.raw_address)) as planning_refs,
        
        -- Address similarity check - be more lenient
        COUNT(DISTINCT SUBSTRING(s.raw_address, 1, 20)) FILTER (WHERE is_real_address(s.raw_address)) as address_variations,
        
        -- Get best match even with lower confidence (>0.5 instead of >=0.8)
        -- Also handle cases where there are multiple UPRNs by picking the one with most votes
        (SELECT da.uprn FROM src_document s2
         JOIN address_match am2 ON s2.document_id = am2.document_id
         JOIN dim_address da ON am2.address_id = da.address_id
         WHERE s2.planning_app_base = s.planning_app_base AND am2.confidence_score > 0.5
         GROUP BY da.uprn
         ORDER BY COUNT(*) DESC, MAX(am2.confidence_score) DESC
         LIMIT 1) as group_best_uprn,
         
        (SELECT da.address_id FROM src_document s2
         JOIN address_match am2 ON s2.document_id = am2.document_id  
         JOIN dim_address da ON am2.address_id = da.address_id
         WHERE s2.planning_app_base = s.planning_app_base AND am2.confidence_score > 0.5
         GROUP BY da.uprn, da.address_id
         ORDER BY COUNT(*) DESC, MAX(am2.confidence_score) DESC
         LIMIT 1) as group_best_address_id,
         
        (SELECT COUNT(*) FROM src_document s2
         JOIN address_match am2 ON s2.document_id = am2.document_id
         JOIN dim_address da ON am2.address_id = da.address_id
         WHERE s2.planning_app_base = s.planning_app_base 
           AND am2.confidence_score > 0.5
           AND da.uprn = (
               SELECT da3.uprn FROM src_document s3
               JOIN address_match am3 ON s3.document_id = am3.document_id
               JOIN dim_address da3 ON am3.address_id = da3.address_id
               WHERE s3.planning_app_base = s.planning_app_base AND am3.confidence_score > 0.5
               GROUP BY da3.uprn
               ORDER BY COUNT(*) DESC
               LIMIT 1
           )
        ) as group_consensus_votes
        
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    WHERE s.planning_app_base IS NOT NULL
    GROUP BY s.planning_app_base
),
safe_groups AS (
    SELECT *
    FROM safe_group_candidates
    WHERE total_docs BETWEEN 2 AND 30          -- Allow larger groups
      AND matched_docs > 0                     -- Some already matched  
      AND matched_docs < total_docs            -- Some unmatched
      AND real_addresses >= (total_docs * 0.6) -- Relax to 60% real addresses
      AND planning_refs <= 3                   -- Allow up to 3 planning refs
      AND address_variations <= 5              -- Allow more variations
      AND group_consensus_votes >= 2           -- Need at least 2 votes for consensus
      AND group_best_uprn IS NOT NULL
      -- Ensure consensus is strong enough even with multiple UPRNs
      AND group_consensus_votes >= (matched_docs * 0.4) -- At least 40% of matches agree
)
SELECT COUNT(*) as safe_corrections, COUNT(DISTINCT planning_app_base) as safe_groups
FROM safe_groups;`

	var safeCorrections, safeGroups int
	err = db.QueryRow(analysisSQL).Scan(&safeCorrections, &safeGroups)
	if err != nil {
		return fmt.Errorf("failed to analyze safe groups: %v", err)
	}

	fmt.Printf("Found %d potential safe corrections across %d groups\n", safeCorrections, safeGroups)

	if safeCorrections == 0 {
		fmt.Println("No safe corrections needed - all groups already properly matched or don't meet safety criteria")
		return nil
	}

	// Step 3: Apply safe corrections
	correctionSQL := `
WITH safe_group_candidates AS (
    SELECT 
        s.planning_app_base,
        COUNT(*) as total_docs,
        COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0) as matched_docs,
        COUNT(DISTINCT am.address_id) FILTER (WHERE am.address_id IS NOT NULL AND am.confidence_score > 0.5) as unique_matched_addresses,
        COUNT(*) FILTER (WHERE is_real_address(s.raw_address)) as real_addresses,
        COUNT(*) FILTER (WHERE NOT is_real_address(s.raw_address)) as planning_refs,
        COUNT(DISTINCT SUBSTRING(s.raw_address, 1, 20)) FILTER (WHERE is_real_address(s.raw_address)) as address_variations,
        
        -- Get the best matching address/location using majority voting
        (SELECT da.address_id FROM src_document s2
         JOIN address_match am2 ON s2.document_id = am2.document_id
         JOIN dim_address da ON am2.address_id = da.address_id  
         WHERE s2.planning_app_base = s.planning_app_base AND am2.confidence_score > 0.5
         GROUP BY da.uprn, da.address_id
         ORDER BY COUNT(*) DESC, MAX(am2.confidence_score) DESC
         LIMIT 1) as group_best_address_id,
         
        (SELECT da.location_id FROM src_document s2
         JOIN address_match am2 ON s2.document_id = am2.document_id
         JOIN dim_address da ON am2.address_id = da.address_id  
         WHERE s2.planning_app_base = s.planning_app_base AND am2.confidence_score > 0.5
         GROUP BY da.uprn, da.address_id, da.location_id
         ORDER BY COUNT(*) DESC, MAX(am2.confidence_score) DESC
         LIMIT 1) as group_best_location_id,
         
        (SELECT COUNT(*) FROM src_document s2
         JOIN address_match am2 ON s2.document_id = am2.document_id
         JOIN dim_address da ON am2.address_id = da.address_id
         WHERE s2.planning_app_base = s.planning_app_base 
           AND am2.confidence_score > 0.5
           AND da.uprn = (
               SELECT da3.uprn FROM src_document s3
               JOIN address_match am3 ON s3.document_id = am3.document_id
               JOIN dim_address da3 ON am3.address_id = da3.address_id
               WHERE s3.planning_app_base = s.planning_app_base AND am3.confidence_score > 0.5
               GROUP BY da3.uprn
               ORDER BY COUNT(*) DESC
               LIMIT 1
           )
        ) as group_consensus_votes
        
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    WHERE s.planning_app_base IS NOT NULL
    GROUP BY s.planning_app_base
),
safe_groups AS (
    SELECT *
    FROM safe_group_candidates
    WHERE total_docs BETWEEN 2 AND 30
      AND matched_docs > 0
      AND matched_docs < total_docs
      AND real_addresses >= (total_docs * 0.6)
      AND planning_refs <= 3
      AND address_variations <= 5
      AND group_consensus_votes >= 2
      AND group_best_address_id IS NOT NULL
      AND group_consensus_votes >= (matched_docs * 0.4)
),
safe_corrections AS (
    SELECT 
        s.document_id,
        sg.planning_app_base,
        am.address_id as original_address_id,
        am.confidence_score as original_confidence,
        am.match_method_id as original_method_id,
        
        sg.group_best_address_id as corrected_address_id,
        sg.group_best_location_id as corrected_location_id,
        CASE 
            WHEN sg.group_consensus_votes >= 5 THEN 0.95
            WHEN sg.group_consensus_votes >= 3 THEN 0.90
            ELSE 0.85
        END as corrected_confidence,
        30 as corrected_method_id,  -- Group consensus method
        
        'SAFE group consensus - ' || sg.group_consensus_votes || ' votes, ' || 
        sg.real_addresses || '/' || sg.total_docs || ' real addresses' as correction_reason
        
    FROM safe_groups sg
    JOIN src_document s ON s.planning_app_base = sg.planning_app_base
    LEFT JOIN address_match am ON s.document_id = am.document_id
    LEFT JOIN dim_address da_current ON am.address_id = da_current.address_id
    WHERE (
        -- Unmatched or low confidence
        am.address_id IS NULL 
        OR am.confidence_score < 0.5
        -- Or matched to wrong UPRN (not the group consensus)
        OR (am.address_id IS NOT NULL AND da_current.uprn != (
            SELECT da2.uprn FROM dim_address da2 
            WHERE da2.address_id = sg.group_best_address_id
        ))
    )
    AND is_real_address(s.raw_address)  -- Only apply to real addresses
)
INSERT INTO address_match_corrected (
    document_id, 
    original_address_id, 
    original_confidence_score, 
    original_method_id,
    corrected_address_id, 
    corrected_location_id, 
    corrected_confidence_score, 
    corrected_method_id,
    correction_reason, 
    planning_app_base
)
SELECT 
    document_id,
    original_address_id,
    original_confidence,
    original_method_id,
    corrected_address_id,
    corrected_location_id,
    corrected_confidence,
    corrected_method_id,
    correction_reason,
    planning_app_base
FROM safe_corrections
ON CONFLICT (document_id) DO UPDATE SET
    corrected_address_id = EXCLUDED.corrected_address_id,
    corrected_location_id = EXCLUDED.corrected_location_id,
    corrected_confidence_score = EXCLUDED.corrected_confidence_score,
    correction_reason = EXCLUDED.correction_reason;`

	result, err := db.Exec(correctionSQL)
	if err != nil {
		return fmt.Errorf("failed to apply corrections: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("Applied %d safe corrections\n", rowsAffected)

	// Step 4: Show results
	var totalCorrections, totalGroups int
	err = db.QueryRow(`
SELECT COUNT(*), COUNT(DISTINCT planning_app_base) 
FROM address_match_corrected`).Scan(&totalCorrections, &totalGroups)
	if err == nil {
		fmt.Printf("Total corrections in system: %d across %d planning groups\n", totalCorrections, totalGroups)
	}

	return nil
}

// rebuildFactTable rebuilds the fact table incorporating corrections
func rebuildFactTable(localDebug bool, db *sql.DB) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	fmt.Println("Rebuilding fact table with corrections...")

	// Step 1: Backup current fact table count
	var originalCount int
	err := db.QueryRow("SELECT COUNT(*) FROM fact_documents_lean").Scan(&originalCount)
	if err != nil {
		return fmt.Errorf("failed to get original count: %v", err)
	}
	fmt.Printf("Current fact table has %d records\n", originalCount)

	// Step 2: Truncate and rebuild
	fmt.Println("Truncating fact table...")
	_, err = db.Exec("TRUNCATE TABLE fact_documents_lean")
	if err != nil {
		return fmt.Errorf("failed to truncate fact table: %v", err)
	}

	// Step 3: Rebuild with corrections
	fmt.Println("Rebuilding fact table with corrections applied...")
	rebuildSQL := `
INSERT INTO fact_documents_lean (
    document_id,
    doc_type_id,
    document_status_id,
    original_address_id,
    matched_address_id,
    matched_location_id,
    match_method_id,
    match_decision_id,
    property_type_id,
    application_status_id,
    development_type_id,
    application_date_id,
    decision_date_id,
    import_date_id,
    match_confidence_score,
    address_quality_score,
    data_completeness_score,
    processing_time_ms,
    import_batch_id,
    planning_reference,
    is_auto_processed,
    has_validation_issues,
    additional_measures,
    created_at,
    processing_version
)
SELECT 
    sd.document_id,
    
    -- Document type (required)
    COALESCE(sd.doc_type_id, 1) as doc_type_id,
    1 as document_status_id,
    
    -- Original address (required)
    oa.original_address_id,
    
    -- Use corrected match if available, otherwise original
    COALESCE(amc.corrected_address_id, am.address_id) as matched_address_id,
    COALESCE(amc.corrected_location_id, am.location_id) as matched_location_id,
    
    -- Match method
    COALESCE(
        amc.corrected_method_id, 
        am.match_method_id,
        (SELECT method_id FROM dim_match_method WHERE method_code = 'no_match' LIMIT 1),
        1
    ) as match_method_id,
    
    -- Match decision based on effective confidence
    CASE 
        WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) >= 0.85 THEN 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'AUTO_ACCEPT' LIMIT 1)
        WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) >= 0.50 THEN 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'NEEDS_REVIEW' LIMIT 1)
        WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) >= 0.20 THEN 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'LOW_CONFIDENCE' LIMIT 1)
        ELSE 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'NO_MATCH' LIMIT 1)
    END as match_decision_id,
    
    -- Optional dimensions
    NULL as property_type_id,
    NULL as application_status_id,
    NULL as development_type_id,
    
    -- Date dimensions
    CASE 
        WHEN sd.document_date IS NOT NULL THEN TO_CHAR(sd.document_date, 'YYYYMMDD')::INTEGER
        ELSE NULL
    END as application_date_id,
    NULL as decision_date_id,
    TO_CHAR(CURRENT_DATE, 'YYYYMMDD')::INTEGER as import_date_id,
    
    -- Measures (use corrected values if available)
    COALESCE(amc.corrected_confidence_score, am.confidence_score) as match_confidence_score,
    
    -- Address quality score
    CASE 
        WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) IS NOT NULL 
            THEN COALESCE(amc.corrected_confidence_score, am.confidence_score)
        WHEN sd.gopostal_processed = TRUE THEN 0.3
        ELSE 0.1
    END as address_quality_score,
    
    -- Data completeness score
    (
        CASE WHEN sd.raw_address IS NOT NULL AND sd.raw_address != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_postcode IS NOT NULL AND sd.gopostal_postcode != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.external_reference IS NOT NULL AND sd.external_reference != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.document_date IS NOT NULL THEN 0.2 ELSE 0 END +
        CASE WHEN sd.job_number IS NOT NULL AND sd.job_number != '' THEN 0.2 ELSE 0 END
    ) as data_completeness_score,
    
    -- Processing time
    CASE 
        WHEN COALESCE(amc.corrected_address_id, am.address_id) IS NOT NULL THEN 
            CASE 
                WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) >= 0.95 THEN 100
                WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) >= 0.85 THEN 150
                WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) >= 0.70 THEN 200
                ELSE 300
            END
        ELSE 50
    END as processing_time_ms,
    
    -- Technical fields
    1 as import_batch_id,
    sd.external_reference as planning_reference,
    
    -- Boolean flags
    CASE 
        WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) >= 0.85 THEN TRUE
        ELSE FALSE
    END as is_auto_processed,
    
    CASE 
        WHEN sd.raw_address IS NULL OR sd.raw_address = '' THEN TRUE
        WHEN sd.gopostal_processed = FALSE THEN TRUE
        WHEN COALESCE(amc.corrected_address_id, am.address_id) IS NULL AND sd.gopostal_processed = TRUE THEN TRUE
        WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) IS NOT NULL 
             AND COALESCE(amc.corrected_confidence_score, am.confidence_score) < 0.7 THEN TRUE
        ELSE FALSE
    END as has_validation_issues,
    
    -- Additional measures with correction info
    jsonb_build_object(
        'original_source', 'src_document',
        'has_uprn', CASE WHEN sd.raw_uprn IS NOT NULL THEN TRUE ELSE FALSE END,
        'has_coordinates', CASE WHEN COALESCE(amc.corrected_location_id, am.location_id) IS NOT NULL THEN TRUE ELSE FALSE END,
        'gopostal_processed', sd.gopostal_processed,
        'job_number', sd.job_number,
        'filepath', sd.filepath,
        'has_correction', CASE WHEN amc.document_id IS NOT NULL THEN TRUE ELSE FALSE END,
        'correction_reason', amc.correction_reason
    ) as additional_measures,
    
    -- Audit fields
    NOW() as created_at,
    '1.1-with-corrections' as processing_version

FROM src_document sd

-- Join to get original address dimension ID (REQUIRED)
INNER JOIN dim_original_address oa ON oa.address_hash = MD5(LOWER(TRIM(sd.raw_address)))

-- Optional join to existing match data
LEFT JOIN address_match am ON sd.document_id = am.document_id

-- Optional join to corrections (takes precedence)
LEFT JOIN address_match_corrected amc ON sd.document_id = amc.document_id

-- Only include records with valid addresses
WHERE sd.raw_address IS NOT NULL 
  AND sd.raw_address != ''

ORDER BY sd.document_id;`

	result, err := db.Exec(rebuildSQL)
	if err != nil {
		return fmt.Errorf("failed to rebuild fact table: %v", err)
	}

	rowsInserted, _ := result.RowsAffected()
	fmt.Printf("Rebuilt fact table with %d records\n", rowsInserted)

	// Step 4: Update statistics and show results
	fmt.Println("Updating table statistics...")
	_, err = db.Exec("ANALYZE fact_documents_lean")
	if err != nil {
		fmt.Printf("Warning: failed to update statistics: %v\n", err)
	}

	// Show rebuild results
	var totalRecords, withMatch, withCorrections int
	var matchRate float64
	err = db.QueryRow(`
SELECT 
    COUNT(*) as total_records,
    COUNT(CASE WHEN matched_address_id IS NOT NULL THEN 1 END) as with_address_match,
    COUNT(CASE WHEN additional_measures->>'has_correction' = 'true' THEN 1 END) as with_corrections,
    ROUND(100.0 * COUNT(CASE WHEN matched_address_id IS NOT NULL THEN 1 END) / COUNT(*), 2) as match_rate_pct
FROM fact_documents_lean`).Scan(&totalRecords, &withMatch, &withCorrections, &matchRate)

	if err == nil {
		fmt.Printf("Fact table rebuild complete:\n")
		fmt.Printf("  Total records: %d\n", totalRecords)
		fmt.Printf("  With matches: %d (%.1f%%)\n", withMatch, matchRate)
		fmt.Printf("  With corrections: %d\n", withCorrections)
	}

	return nil
}

// validateDataIntegrity checks for data integrity issues
func validateDataIntegrity(localDebug bool, db *sql.DB) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	fmt.Println("Validating data integrity...")

	// Check 1: Correction table integrity
	fmt.Println("Checking correction table integrity...")
	var badCorrections int
	err := db.QueryRow(`
SELECT COUNT(*) 
FROM address_match_corrected amc
JOIN dim_address da ON amc.corrected_address_id = da.address_id
WHERE amc.corrected_location_id != da.location_id`).Scan(&badCorrections)

	if err != nil {
		fmt.Printf("Warning: could not check correction integrity: %v\n", err)
	} else if badCorrections > 0 {
		fmt.Printf("ERROR: Found %d corrections with mismatched location_ids\n", badCorrections)
		
		// Fix them
		fmt.Println("Fixing location_id mismatches...")
		result, err := db.Exec(`
UPDATE address_match_corrected 
SET corrected_location_id = da.location_id
FROM dim_address da 
WHERE address_match_corrected.corrected_address_id = da.address_id
  AND address_match_corrected.corrected_location_id != da.location_id`)
  
		if err != nil {
			return fmt.Errorf("failed to fix location mismatches: %v", err)
		}
		
		rowsFixed, _ := result.RowsAffected()
		fmt.Printf("Fixed %d location_id mismatches\n", rowsFixed)
	} else {
		fmt.Println("✓ Correction table integrity: OK")
	}

	// Check 2: Fact table integrity
	fmt.Println("Checking fact table integrity...")
	var factIssues int
	err = db.QueryRow(`
SELECT COUNT(*) 
FROM fact_documents_lean f
JOIN dim_address da ON f.matched_address_id = da.address_id
WHERE f.matched_location_id != da.location_id`).Scan(&factIssues)

	if err != nil {
		fmt.Printf("Warning: could not check fact table integrity: %v\n", err)
	} else if factIssues > 0 {
		fmt.Printf("ERROR: Found %d fact records with mismatched location_ids\n", factIssues)
		fmt.Println("Run 'rebuild-fact' command to fix these issues")
	} else {
		fmt.Println("✓ Fact table integrity: OK")
	}

	// Check 3: UPRN consistency
	fmt.Println("Checking UPRN consistency...")
	var uprnIssues int
	err = db.QueryRow(`
SELECT COUNT(DISTINCT da.uprn) 
FROM dim_address da
JOIN dim_location dl ON da.location_id = dl.location_id
GROUP BY da.uprn
HAVING COUNT(DISTINCT dl.easting || ',' || dl.northing) > 1`).Scan(&uprnIssues)

	if err != nil {
		fmt.Printf("Warning: could not check UPRN consistency: %v\n", err)
	} else if uprnIssues > 0 {
		fmt.Printf("WARNING: Found %d UPRNs with multiple coordinate sets\n", uprnIssues)
	} else {
		fmt.Println("✓ UPRN coordinate consistency: OK")
	}

	// Check 4: Sample the 20003 group that was problematic
	fmt.Println("Checking 20003 group consistency...")
	rows, err := db.Query(`
SELECT 
    s.external_reference,
    da.uprn,
    dl.easting,
    dl.northing,
    CASE WHEN amc.document_id IS NOT NULL THEN 'CORRECTED' ELSE 'ORIGINAL' END as match_type
FROM src_document s
JOIN fact_documents_lean f ON s.document_id = f.document_id
JOIN dim_address da ON f.matched_address_id = da.address_id
JOIN dim_location dl ON f.matched_location_id = dl.location_id
LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
WHERE s.planning_app_base = '20003'
ORDER BY s.external_reference`)

	if err != nil {
		fmt.Printf("Warning: could not check 20003 group: %v\n", err)
	} else {
		fmt.Println("20003 group current status:")
		for rows.Next() {
			var ref, uprn, matchType string
			var easting, northing float64
			err := rows.Scan(&ref, &uprn, &easting, &northing, &matchType)
			if err != nil {
				continue
			}
			fmt.Printf("  %s: UPRN %s at (%.2f, %.2f) [%s]\n", ref, uprn, easting, northing, matchType)
		}
		rows.Close()
	}

	fmt.Println("Data integrity validation complete!")
	return nil
}

// fuzzyMatchUnmatchedGroups uses fuzzy matching to find potential matches for completely unmatched groups
func fuzzyMatchUnmatchedGroups(localDebug bool, db *sql.DB) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	fmt.Println("Finding fuzzy matches for unmatched groups...")

	// Step 1: Ensure required extensions are enabled
	_, err := db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm")
	if err != nil {
		return fmt.Errorf("failed to enable pg_trgm extension: %v", err)
	}
	
	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS fuzzystrmatch")
	if err != nil {
		return fmt.Errorf("failed to enable fuzzystrmatch extension: %v", err)
	}

	// Step 2: Find groups with poor or no matches for fuzzy matching
	fmt.Println("Finding planning groups with poor matches for fuzzy matching...")
	
	unmatchedGroupsSQL := `
WITH low_confidence_groups AS (
    SELECT 
        s.planning_app_base,
        COUNT(*) as total_docs,
        COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0.5) as good_matches,
        MAX(COALESCE(am.confidence_score, 0)) as best_confidence,
        COUNT(*) FILTER (WHERE is_real_address(s.raw_address)) as real_addresses,
        -- Get the most complete address in the group
        (SELECT s2.raw_address 
         FROM src_document s2 
         WHERE s2.planning_app_base = s.planning_app_base 
           AND is_real_address(s2.raw_address)
         ORDER BY LENGTH(s2.raw_address) DESC, s2.raw_address
         LIMIT 1) as best_address_in_group
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    WHERE s.planning_app_base IS NOT NULL
    GROUP BY s.planning_app_base
    HAVING COUNT(*) BETWEEN 2 AND 30  -- Allow larger groups
      AND COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0.5) = 0  -- No good matches
      AND MAX(COALESCE(am.confidence_score, 0)) < 0.5  -- Best match is poor
      AND COUNT(*) FILTER (WHERE is_real_address(s.raw_address)) >= 1  -- At least one real address
)
SELECT 
    planning_app_base,
    total_docs,
    real_addresses,
    best_address_in_group
FROM low_confidence_groups
WHERE best_address_in_group IS NOT NULL
ORDER BY planning_app_base  -- Process alphabetically to include 20005
LIMIT 100  -- Process more groups
`

	type UnmatchedGroup struct {
		PlanningAppBase     string
		TotalDocs          int
		RealAddresses      int
		BestAddressInGroup string
	}

	rows, err := db.Query(unmatchedGroupsSQL)
	if err != nil {
		return fmt.Errorf("failed to find unmatched groups: %v", err)
	}
	defer rows.Close()

	var unmatchedGroups []UnmatchedGroup
	for rows.Next() {
		var ug UnmatchedGroup
		err := rows.Scan(&ug.PlanningAppBase, &ug.TotalDocs, &ug.RealAddresses, &ug.BestAddressInGroup)
		if err != nil {
			return fmt.Errorf("failed to scan unmatched group: %v", err)
		}
		unmatchedGroups = append(unmatchedGroups, ug)
	}

	fmt.Printf("Found %d groups with poor matches to process\n", len(unmatchedGroups))

	if len(unmatchedGroups) == 0 {
		fmt.Println("No groups with poor matches found")
		return nil
	}

	// Step 3: For each unmatched group, find fuzzy matches
	var totalCorrections int
	
	for i, group := range unmatchedGroups {
		if localDebug {
			fmt.Printf("Processing group %d/%d: %s (%d docs, address: %.50s)\n", 
				i+1, len(unmatchedGroups), group.PlanningAppBase, group.TotalDocs, group.BestAddressInGroup)
		}

		// Find fuzzy matches for the best address in this group
		fuzzyMatchSQL := `
SELECT 
    da.address_id,
    da.uprn,
    da.full_address,
    dl.location_id,
    similarity(da.full_address, $1) as similarity_score,
    levenshtein(UPPER(da.full_address), UPPER($1)) as edit_distance
FROM dim_address da
JOIN dim_location dl ON da.location_id = dl.location_id
WHERE da.full_address % $1  -- Uses trigram index
ORDER BY similarity_score DESC, edit_distance ASC
LIMIT 3`

		type FuzzyMatch struct {
			AddressID        int
			UPRN            string  
			FullAddress     string
			LocationID      int
			SimilarityScore float64
			EditDistance    int
		}

		fuzzyRows, err := db.Query(fuzzyMatchSQL, group.BestAddressInGroup)
		if err != nil {
			fmt.Printf("Warning: failed to find fuzzy matches for group %s: %v\n", group.PlanningAppBase, err)
			continue
		}

		var matches []FuzzyMatch
		for fuzzyRows.Next() {
			var fm FuzzyMatch
			err := fuzzyRows.Scan(&fm.AddressID, &fm.UPRN, &fm.FullAddress, &fm.LocationID, &fm.SimilarityScore, &fm.EditDistance)
			if err != nil {
				fmt.Printf("Warning: failed to scan fuzzy match: %v\n", err)
				continue
			}
			matches = append(matches, fm)
		}
		fuzzyRows.Close()

		// Only apply if we have a good match (similarity > 0.4 and edit distance reasonable)
		if len(matches) > 0 {
			bestMatch := matches[0]
			
			// Apply reasonable criteria for fuzzy matching
			minSimilarity := 0.5  // Require at least 50% similarity
			maxEditDistance := 25  // Allow reasonable edit distance for address variations
			
			if bestMatch.SimilarityScore >= minSimilarity && bestMatch.EditDistance <= maxEditDistance {
				if localDebug {
					fmt.Printf("  Found match: %s (sim: %.3f, edit: %d)\n", 
						bestMatch.FullAddress, bestMatch.SimilarityScore, bestMatch.EditDistance)
				}

				// Apply correction to all documents in the group with real addresses
				correctionSQL := `
INSERT INTO address_match_corrected (
    document_id, 
    original_address_id, 
    original_confidence_score, 
    original_method_id,
    corrected_address_id, 
    corrected_location_id, 
    corrected_confidence_score, 
    corrected_method_id,
    correction_reason, 
    planning_app_base
)
SELECT 
    s.document_id,
    NULL as original_address_id,  -- No original match
    0.0 as original_confidence_score,
    1 as original_method_id,  -- no_match method
    
    $1 as corrected_address_id,
    $2 as corrected_location_id,
    CASE 
        WHEN $3 >= 0.7 THEN 0.85
        WHEN $3 >= 0.5 THEN 0.75  
        ELSE 0.65
    END as corrected_confidence_score,
    31 as corrected_method_id,  -- Fuzzy match method
    
    'Fuzzy match - similarity: ' || ROUND($3::numeric, 3) || 
    ', edit distance: ' || $4 || 
    ', matched to: ' || LEFT($5, 50) as correction_reason,
    
    $6 as planning_app_base
    
FROM src_document s
WHERE s.planning_app_base = $6
  AND is_real_address(s.raw_address)
  AND NOT EXISTS (
      SELECT 1 FROM address_match_corrected amc 
      WHERE amc.document_id = s.document_id
  )
ON CONFLICT (document_id) DO NOTHING`

				result, err := db.Exec(correctionSQL, 
					bestMatch.AddressID, bestMatch.LocationID, 
					bestMatch.SimilarityScore, bestMatch.EditDistance, bestMatch.FullAddress,
					group.PlanningAppBase)
				
				if err != nil {
					fmt.Printf("Warning: failed to apply correction for group %s: %v\n", group.PlanningAppBase, err)
					continue
				}

				rowsAffected, _ := result.RowsAffected()
				totalCorrections += int(rowsAffected)
				
				if localDebug || rowsAffected > 0 {
					fmt.Printf("  Applied fuzzy match to %d documents in group %s\n", rowsAffected, group.PlanningAppBase)
				}
			} else if localDebug {
				fmt.Printf("  No good match found (best similarity: %.3f, edit distance: %d)\n", 
					bestMatch.SimilarityScore, bestMatch.EditDistance)
			}
		}
	}

	fmt.Printf("Applied %d fuzzy match corrections across %d groups\n", totalCorrections, len(unmatchedGroups))

	// Show total corrections
	var totalCorrectionsInSystem int
	err = db.QueryRow("SELECT COUNT(*) FROM address_match_corrected").Scan(&totalCorrectionsInSystem)
	if err == nil {
		fmt.Printf("Total corrections in system: %d\n", totalCorrectionsInSystem)
	}

	return nil
}

// llmFixLowConfidenceAddresses uses LLM to fix obvious address formatting mistakes
func llmFixLowConfidenceAddresses(localDebug bool, db *sql.DB) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	fmt.Println("Using LLM to fix low confidence addresses...")

	// Step 1: Find low confidence addresses that might benefit from LLM correction
	findCandidatesSQL := `
SELECT 
	s.document_id,
	s.raw_address,
	s.planning_app_base,
	am.confidence_score as original_confidence,
	am.address_id as original_address_id,
	am.match_method_id as original_method_id
FROM src_document s
LEFT JOIN address_match am ON s.document_id = am.document_id
LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
WHERE s.raw_address IS NOT NULL 
  AND s.raw_address != ''
  AND amc.document_id IS NULL  -- Not already corrected
  AND (
      am.confidence_score IS NULL OR              -- No match
      (am.confidence_score > 0 AND am.confidence_score <= 0.4)  -- Low confidence
  )
  AND (
      -- Focus on addresses with obvious formatting issues
      s.raw_address ~* '[0-9]+,\s*[A-Z\s]+' OR                    -- "5, AMEY INDUSTRIAL"
      s.raw_address ~* '^[0-9A-Z]+\s*[A-Z\s]+ESTATE' OR          -- "5A AMEY INDUSTRIAL ESTATE"
      s.raw_address ~* 'UNIT\s*[0-9A-Z]+' OR                     -- "UNIT 14" variations
      s.raw_address ~* '[0-9]+[A-Z]?\s*[A-Z\s]+' OR              -- General unit patterns
      s.raw_address ~* 'INDUSTRIAL|ESTATE|BUSINESS\s*PARK'       -- Industrial areas
  )
ORDER BY s.document_id`

	rows, err := db.Query(findCandidatesSQL)
	if err != nil {
		return fmt.Errorf("failed to find correction candidates: %v", err)
	}
	defer rows.Close()

	var candidates []CorrectionCandidate
	for rows.Next() {
		var c CorrectionCandidate
		err := rows.Scan(&c.DocumentID, &c.RawAddress, &c.PlanningAppBase, 
			&c.OriginalConfidence, &c.OriginalAddressID, &c.OriginalMethodID)
		if err != nil {
			return fmt.Errorf("failed to scan candidate: %v", err)
		}
		candidates = append(candidates, c)
	}

	fmt.Printf("Found %d addresses that may benefit from LLM correction\n", len(candidates))
	
	fmt.Println("SKIPPING ALL LLM CORRECTIONS - they degrade data quality:")
	fmt.Println("  - LLM converting AVENUE → AVE (wrong direction)")
	fmt.Println("  - LLM making bizarre errors: BUNTINGS → BUNtings")
	fmt.Println("  - Core matching engine needs fixing instead (73% similarity not matching)")
	fmt.Printf("Total LLM corrections applied: %d\n", 0)
	return nil

	// Step 2: Process each candidate with LLM
	var totalCorrections int
	for i, candidate := range candidates {
		if localDebug {
			fmt.Printf("Processing %d/%d: %s\n", i+1, len(candidates), candidate.RawAddress)
		}

		// Call LLM to suggest correction
		correctedAddress, err := callLLMForAddressCorrection(candidate.RawAddress, localDebug)
		if err != nil {
			fmt.Printf("Warning: LLM call failed for document %d: %v\n", candidate.DocumentID, err)
			continue
		}

		// Skip if LLM didn't suggest a meaningful change
		if correctedAddress == "" || correctedAddress == candidate.RawAddress {
			if localDebug {
				fmt.Printf("  No correction suggested\n")
			}
			continue
		}

		if localDebug {
			fmt.Printf("  LLM suggests: %s\n", correctedAddress)
		}

		// Step 3: Search for the corrected address in LLPG
		matchedAddress, err := searchLLPGForAddress(db, correctedAddress, localDebug)
		if err != nil {
			fmt.Printf("Warning: LLPG search failed for document %d: %v\n", candidate.DocumentID, err)
			continue
		}

		if matchedAddress == nil {
			if localDebug {
				fmt.Printf("  No LLPG match found for corrected address\n")
			}
			continue
		}

		if localDebug {
			fmt.Printf("  Found LLPG match: %s (confidence: %.3f)\n", 
				matchedAddress.FullAddress, matchedAddress.Confidence)
		}

		// Step 4: Apply correction if match quality is good enough
		if matchedAddress.Confidence >= 0.6 {
			err = applyLLMCorrection(db, candidate, correctedAddress, matchedAddress, localDebug)
			if err != nil {
				fmt.Printf("Warning: failed to apply correction for document %d: %v\n", candidate.DocumentID, err)
				continue
			}
			totalCorrections++
			fmt.Printf("  ✓ Applied LLM correction: %.50s → %.50s (conf: %.3f)\n", 
				candidate.RawAddress, correctedAddress, matchedAddress.Confidence)
		} else if localDebug {
			fmt.Printf("  Match confidence too low (%.3f < 0.7)\n", matchedAddress.Confidence)
		}
	}

	fmt.Printf("Applied %d individual LLM-powered corrections\n", totalCorrections)
	
	// Phase 2: Group-based LLM matching using golden records
	fmt.Println("\nPhase 2: Using LLM for group-based address similarity detection...")
	groupCorrections, err := applyGroupLLMMatching(db, localDebug)
	if err != nil {
		fmt.Printf("Warning: Group LLM matching failed: %v\n", err)
	} else {
		fmt.Printf("Applied %d group-based LLM corrections\n", groupCorrections)
		totalCorrections += groupCorrections
	}
	
	fmt.Printf("Total LLM corrections applied: %d\n", totalCorrections)
	return nil
}

// callLLMForAddressCorrection calls Ollama to suggest an address correction
func callLLMForAddressCorrection(rawAddress string, localDebug bool) (string, error) {
	// Construct prompt for address correction
	prompt := fmt.Sprintf(`You are an address formatting expert. Your task is to correct obvious formatting issues in UK addresses to make them more searchable.

Rules:
1. Fix capitalization: "HIGH STREET" → "High Street", "GARDENS" → "Gardens"
2. Expand common abbreviations: "RD" → "Road", "ST" → "Street", "AVE" → "Avenue"
3. Keep house numbers and flat numbers exactly as they are
4. For incomplete estate names, add "Estate" if clearly missing: "AMEY INDUSTRIAL" → "Amey Industrial Estate"
5. DO NOT add "Unit" prefix unless it already exists in the original address
6. Preserve the original address structure - don't change the format
7. If no obvious correction is needed, return the address unchanged
8. Only return the corrected address, no explanation

Examples:
- "14 HIGH STREET, ALTON" → "14 High Street, Alton"
- "5 THORPE GARDENS, ALTON" → "5 Thorpe Gardens, Alton"
- "FLAT 3, 25 LONDON RD" → "Flat 3, 25 London Road"
- "UNIT 9C, AMEY INDUSTRIAL" → "Unit 9C, Amey Industrial Estate"

Address to correct: %s

Corrected address:`, rawAddress)

	// Prepare request to Ollama
	requestBody := map[string]interface{}{
		"model": "llama3.2:1b",
		"prompt": prompt,
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.1,
			"top_p": 0.9,
			"max_tokens": 100,
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	// Make HTTP request to Ollama
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response struct {
		Response string `json:"response"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// Clean up the response
	corrected := strings.TrimSpace(response.Response)
	corrected = strings.ReplaceAll(corrected, "\n", " ")
	corrected = strings.ReplaceAll(corrected, "  ", " ")

	if localDebug {
		fmt.Printf("  LLM raw response: %s\n", response.Response)
	}

	return corrected, nil
}

type LLPGMatch struct {
	AddressID    int
	LocationID   int
	UPRN        string
	FullAddress string
	Confidence  float64
}

// searchLLPGForAddress searches the LLPG database using your postcode-flexible approach
func searchLLPGForAddress(db *sql.DB, address string, localDebug bool) (*LLPGMatch, error) {
	// Strategy 1: Try trigram similarity but with your ordering insight
	// This handles postcode variations while maintaining good matching
	searchSQL := `
SELECT 
	da.address_id,
	da.location_id,
	da.uprn,
	da.full_address,
	similarity(da.full_address, $1) as similarity_score
FROM dim_address da
WHERE da.full_address % $1  -- Trigram similarity operator
  AND similarity(da.full_address, $1) >= 0.4  -- Reasonable threshold
ORDER BY similarity_score DESC, 
         da.gopostal_postcode, 
         da.gopostal_house, 
         LENGTH(da.gopostal_house_number), 
         da.gopostal_house_number, 
         da.address_canonical ASC
LIMIT 3`

	rows, err := db.Query(searchSQL, address)
	if err != nil {
		return nil, fmt.Errorf("LLPG search failed: %v", err)
	}
	defer rows.Close()

	var bestMatch *LLPGMatch
	if rows.Next() {
		var match LLPGMatch
		err := rows.Scan(&match.AddressID, &match.LocationID, &match.UPRN, 
			&match.FullAddress, &match.Confidence)
		if err != nil {
			return nil, fmt.Errorf("failed to scan LLPG match: %v", err)
		}

		// Basic validation: ensure house numbers match if both have them
		if isHouseNumberMismatch(address, match.FullAddress) {
			if localDebug {
				fmt.Printf("  House number mismatch - rejecting: %s vs %s\n", address, match.FullAddress)
			}
			return nil, nil
		}

		bestMatch = &match
	}

	if bestMatch == nil {
		return nil, nil
	}

	if localDebug {
		fmt.Printf("  LLPG match found: %s (similarity: %.3f)\n", bestMatch.FullAddress, bestMatch.Confidence)
	}

	return bestMatch, nil
}

// isHouseNumberMismatch checks if two addresses have different house numbers
func isHouseNumberMismatch(addr1, addr2 string) bool {
	num1 := extractHouseNumber(addr1)
	num2 := extractHouseNumber(addr2)
	
	// If either has no house number, allow the match
	if num1 == "" || num2 == "" {
		return false
	}
	
	// House numbers must match exactly
	return num1 != num2
}

// extractHouseNumber extracts the house number from an address
func extractHouseNumber(address string) string {
	cleanAddr := strings.ToUpper(strings.TrimSpace(address))
	parts := strings.Fields(cleanAddr)
	
	for _, part := range parts {
		// Skip "Unit" prefix
		if part == "UNIT" {
			continue
		}
		
		// Look for house number pattern (digits + optional letter)
		if matched, _ := regexp.MatchString(`^[0-9]+[A-Z]*$`, part); matched {
			return part
		}
	}
	
	return ""
}


// CorrectionCandidate represents an address that may benefit from LLM correction
type CorrectionCandidate struct {
	DocumentID         int
	RawAddress         string
	PlanningAppBase    sql.NullString
	OriginalConfidence sql.NullFloat64
	OriginalAddressID  sql.NullInt64
	OriginalMethodID   sql.NullInt64
}

// applyLLMCorrection applies an LLM-suggested correction to the database
func applyLLMCorrection(db *sql.DB, candidate CorrectionCandidate, correctedAddress string, match *LLPGMatch, localDebug bool) error {
	// Insert into address_match_corrected table
	correctionSQL := `
INSERT INTO address_match_corrected (
	document_id,
	original_address_id,
	original_confidence_score,
	original_method_id,
	corrected_address_id,
	corrected_location_id,
	corrected_confidence_score,
	corrected_method_id,
	correction_reason,
	planning_app_base
)
VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (document_id) DO UPDATE SET
	corrected_address_id = EXCLUDED.corrected_address_id,
	corrected_location_id = EXCLUDED.corrected_location_id,
	corrected_confidence_score = EXCLUDED.corrected_confidence_score,
	correction_reason = EXCLUDED.correction_reason`

	originalAddressID := sql.NullInt64{}
	if candidate.OriginalAddressID.Valid {
		originalAddressID = candidate.OriginalAddressID
	}

	originalConfidence := 0.0
	if candidate.OriginalConfidence.Valid {
		originalConfidence = candidate.OriginalConfidence.Float64
	}

	originalMethodID := int64(1) // default to no_match
	if candidate.OriginalMethodID.Valid {
		originalMethodID = candidate.OriginalMethodID.Int64
	}

	// Use a high confidence for LLM + trigram match
	correctedConfidence := match.Confidence * 0.9 // Slightly discount for being AI-assisted
	if correctedConfidence > 0.95 {
		correctedConfidence = 0.95
	}

	correctionReason := fmt.Sprintf("LLM correction: '%s' → '%s' (similarity: %.3f)", 
		candidate.RawAddress, correctedAddress, match.Confidence)

	_, err := db.Exec(correctionSQL,
		candidate.DocumentID,
		originalAddressID,
		originalConfidence,
		originalMethodID,
		match.AddressID,
		match.LocationID,
		correctedConfidence,
		32, // LLM correction method ID
		correctionReason,
		candidate.PlanningAppBase)

	return err
}

// applyGroupLLMMatching uses LLM to detect address similarity within planning groups
func applyGroupLLMMatching(db *sql.DB, localDebug bool) (int, error) {
	// Find planning groups with golden records (2+ matches to same UPRN) and unmatched addresses
	groupsSQL := `
SELECT 
	s.planning_app_base,
	COUNT(*) as total_docs,
	COUNT(am.document_id) FILTER (WHERE am.confidence_score >= 0.9) as high_confidence_matches,
	COUNT(am.document_id) FILTER (WHERE am.confidence_score IS NULL OR am.confidence_score = 0) as unmatched_docs,
	-- Get the golden record (most common high-confidence match)
	(SELECT da.address_id FROM src_document s2
	 JOIN address_match am2 ON s2.document_id = am2.document_id  
	 JOIN dim_address da ON am2.address_id = da.address_id
	 WHERE s2.planning_app_base = s.planning_app_base 
	   AND am2.confidence_score >= 0.9
	 GROUP BY da.uprn, da.address_id, da.full_address
	 ORDER BY COUNT(*) DESC, MAX(am2.confidence_score) DESC
	 LIMIT 1) as golden_address_id,
	-- Get the full golden record address
	(SELECT da.full_address FROM src_document s2
	 JOIN address_match am2 ON s2.document_id = am2.document_id  
	 JOIN dim_address da ON am2.address_id = da.address_id
	 WHERE s2.planning_app_base = s.planning_app_base 
	   AND am2.confidence_score >= 0.9
	 GROUP BY da.uprn, da.address_id, da.full_address
	 ORDER BY COUNT(*) DESC, MAX(am2.confidence_score) DESC
	 LIMIT 1) as golden_address_text
FROM src_document s
LEFT JOIN address_match am ON s.document_id = am.document_id
LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
WHERE s.planning_app_base IS NOT NULL
  AND amc.document_id IS NULL  -- Not already corrected
GROUP BY s.planning_app_base
HAVING COUNT(*) BETWEEN 2 AND 8  -- Reasonable group size
  AND COUNT(am.document_id) FILTER (WHERE am.confidence_score >= 0.9) >= 2  -- At least 2 golden matches
  AND COUNT(am.document_id) FILTER (WHERE am.confidence_score IS NULL OR am.confidence_score = 0) >= 1  -- At least 1 unmatched
ORDER BY COUNT(am.document_id) FILTER (WHERE am.confidence_score IS NULL OR am.confidence_score = 0) DESC`

	rows, err := db.Query(groupsSQL)
	if err != nil {
		return 0, fmt.Errorf("failed to find candidate groups: %v", err)
	}
	defer rows.Close()

	var totalGroupCorrections int
	for rows.Next() {
		var planningAppBase string
		var totalDocs, highConfidenceMatches, unmatchedDocs int
		var goldenAddressID sql.NullInt64
		var goldenAddressText sql.NullString

		err := rows.Scan(&planningAppBase, &totalDocs, &highConfidenceMatches, &unmatchedDocs, 
			&goldenAddressID, &goldenAddressText)
		if err != nil {
			continue
		}

		if !goldenAddressID.Valid || !goldenAddressText.Valid {
			continue
		}

		if localDebug {
			fmt.Printf("Processing group %s: %d unmatched, golden record: %s\n", 
				planningAppBase, unmatchedDocs, goldenAddressText.String)
		}

		// Get unmatched addresses in this group
		unmatchedSQL := `
SELECT s.document_id, s.raw_address
FROM src_document s
LEFT JOIN address_match am ON s.document_id = am.document_id
LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
WHERE s.planning_app_base = $1
  AND amc.document_id IS NULL
  AND (am.confidence_score IS NULL OR am.confidence_score = 0)
  AND s.raw_address IS NOT NULL 
  AND s.raw_address != ''`

		unmatchedRows, err := db.Query(unmatchedSQL, planningAppBase)
		if err != nil {
			continue
		}

		for unmatchedRows.Next() {
			var documentID int
			var rawAddress string
			err := unmatchedRows.Scan(&documentID, &rawAddress)
			if err != nil {
				continue
			}

			// Ask LLM if this address is likely the same as the golden record
			isSameAddress, confidence, err := askLLMAddressSimilarity(rawAddress, goldenAddressText.String, localDebug)
			if err != nil {
				if localDebug {
					fmt.Printf("  LLM error for doc %d: %v\n", documentID, err)
				}
				continue
			}

			if isSameAddress && confidence >= 0.8 {
				// Apply the group correction
				err = applyGroupCorrection(db, documentID, planningAppBase, goldenAddressID.Int64, rawAddress, goldenAddressText.String, confidence, localDebug)
				if err != nil {
					if localDebug {
						fmt.Printf("  Failed to apply correction for doc %d: %v\n", documentID, err)
					}
					continue
				}
				
				totalGroupCorrections++
				fmt.Printf("  ✓ Group LLM match: %.60s → %.60s (conf: %.3f)\n", 
					rawAddress, goldenAddressText.String, confidence)
			} else if localDebug {
				fmt.Printf("  No match: %.60s (conf: %.3f, same: %v)\n", rawAddress, confidence, isSameAddress)
			}
		}
		unmatchedRows.Close()
	}

	return totalGroupCorrections, nil
}

// askLLMAddressSimilarity asks the LLM if two addresses are likely the same location
func askLLMAddressSimilarity(rawAddress, goldenAddress string, localDebug bool) (bool, float64, error) {
	prompt := fmt.Sprintf(`You are an address matching expert. Your task is to determine if two UK addresses refer to the same physical location.

IMPORTANT: Focus on whether these are the SAME PHYSICAL LOCATION, not just similar addresses.
The golden record shows the correct LLPG format for addresses in this area.

Address 1 (unmatched): %s
Address 2 (golden record from LLPG): %s

Consider:
1. Are these likely the same property/building?
2. Local area names (e.g., "Woodcock Bottom" might be a local name for an area on "Avenue Road")
3. Alternative descriptions of the same location
4. Minor formatting differences
5. Missing or additional location descriptors

Respond with exactly:
- "SAME" if they likely refer to the same physical location
- "DIFFERENT" if they are clearly different locations
- A confidence score from 0.0 to 1.0

Format: SAME|0.85 or DIFFERENT|0.30`, rawAddress, goldenAddress)

	// Prepare request to Ollama
	requestBody := map[string]interface{}{
		"model": "llama3.2:1b",
		"prompt": prompt,
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.1,
			"top_p": 0.9,
			"max_tokens": 50,
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return false, 0, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Make HTTP request to Ollama
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return false, 0, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response struct {
		Response string `json:"response"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, 0, fmt.Errorf("failed to read response: %v", err)
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return false, 0, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// Parse the LLM response: "SAME|0.85" or "DIFFERENT|0.30"
	responseText := strings.TrimSpace(response.Response)
	parts := strings.Split(responseText, "|")
	
	if len(parts) != 2 {
		// Try alternative parsing if format is different
		if strings.Contains(responseText, "SAME") {
			return true, 0.7, nil // Default confidence
		} else if strings.Contains(responseText, "DIFFERENT") {
			return false, 0.3, nil
		}
		return false, 0, fmt.Errorf("unexpected LLM response format: %s", responseText)
	}

	decision := strings.ToUpper(strings.TrimSpace(parts[0]))
	confidenceStr := strings.TrimSpace(parts[1])
	
	confidence, err := strconv.ParseFloat(confidenceStr, 64)
	if err != nil {
		confidence = 0.5 // Default if parsing fails
	}

	isSame := decision == "SAME"
	
	if localDebug {
		fmt.Printf("    LLM response: %s -> %v (conf: %.3f)\n", responseText, isSame, confidence)
	}

	return isSame, confidence, nil
}

// applyGroupCorrection applies a group-based LLM correction
func applyGroupCorrection(db *sql.DB, documentID int, planningAppBase string, goldenAddressID int64, rawAddress, goldenAddress string, confidence float64, localDebug bool) error {
	// Get the location_id for the golden address
	var locationID int64
	err := db.QueryRow("SELECT location_id FROM dim_address WHERE address_id = $1", goldenAddressID).Scan(&locationID)
	if err != nil {
		return fmt.Errorf("failed to get location_id: %v", err)
	}

	correctionSQL := `
INSERT INTO address_match_corrected (
	document_id,
	original_address_id,
	original_confidence_score,
	original_method_id,
	corrected_address_id,
	corrected_location_id,
	corrected_confidence_score,
	corrected_method_id,
	correction_reason,
	planning_app_base
)
VALUES (
	$1, NULL, 0.0, 1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (document_id) DO UPDATE SET
	corrected_address_id = EXCLUDED.corrected_address_id,
	corrected_location_id = EXCLUDED.corrected_location_id,
	corrected_confidence_score = EXCLUDED.corrected_confidence_score,
	correction_reason = EXCLUDED.correction_reason`

	// Use a specific method ID for group LLM matching
	const groupLLMMethodID = 33
	
	// Ensure the method exists
	_, err = db.Exec(`
INSERT INTO dim_match_method (method_id, method_code, method_name, description)
VALUES ($1, 'group_llm_similarity', 'Group LLM Address Similarity', 'LLM-based address similarity detection within planning groups')
ON CONFLICT (method_id) DO NOTHING`, groupLLMMethodID)
	if err != nil {
		return fmt.Errorf("failed to ensure method exists: %v", err)
	}

	correctionReason := fmt.Sprintf("Group LLM similarity: '%s' matched to group golden record '%s' (LLM confidence: %.3f)", 
		rawAddress, goldenAddress, confidence)

	_, err = db.Exec(correctionSQL, documentID, goldenAddressID, locationID, confidence, groupLLMMethodID, correctionReason, planningAppBase)
	return err
}

// fuzzyMatchIndividualDocuments applies fuzzy matching to individual unmatched documents
func fuzzyMatchIndividualDocuments(localDebug bool, db *sql.DB) error {
	fmt.Println("Finding fuzzy matches for individual unmatched documents...")

	// Step 1: Ensure required extensions are enabled
	_, err := db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm")
	if err != nil {
		return fmt.Errorf("failed to enable pg_trgm extension: %v", err)
	}
	
	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS fuzzystrmatch")
	if err != nil {
		return fmt.Errorf("failed to enable fuzzystrmatch extension: %v", err)
	}

	// Step 2: Find individual documents with poor or no matches
	fmt.Println("Finding individual documents with poor matches...")
	
	individualDocsSQL := `
SELECT 
    s.document_id,
    s.raw_address,
    s.planning_app_base,
    COALESCE(am.confidence_score, 0) as current_confidence
FROM src_document s
LEFT JOIN address_match am ON s.document_id = am.document_id
LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
WHERE s.raw_address IS NOT NULL 
  AND s.raw_address != ''
  AND LENGTH(s.raw_address) > 15  -- Meaningful addresses only
  AND amc.document_id IS NULL  -- Not already corrected
  AND COALESCE(am.confidence_score, 0) < 0.7  -- Poor or no match
  AND is_real_address(s.raw_address)  -- Real addresses only
ORDER BY s.document_id
LIMIT 500  -- Process in smaller batches for testing`

	type IndividualDoc struct {
		DocumentID        int
		RawAddress        string
		PlanningAppBase   sql.NullString
		CurrentConfidence float64
	}

	rows, err := db.Query(individualDocsSQL)
	if err != nil {
		return fmt.Errorf("failed to find individual documents: %v", err)
	}
	defer rows.Close()

	var individualDocs []IndividualDoc
	for rows.Next() {
		var doc IndividualDoc
		err := rows.Scan(&doc.DocumentID, &doc.RawAddress, &doc.PlanningAppBase, &doc.CurrentConfidence)
		if err != nil {
			continue
		}
		individualDocs = append(individualDocs, doc)
	}

	fmt.Printf("Found %d individual documents with poor matches to process\n", len(individualDocs))

	if len(individualDocs) == 0 {
		fmt.Println("No individual documents with poor matches found")
		return nil
	}

	// Step 3: Process each document individually
	var totalCorrections int
	const individualFuzzyMethodID = 35

	// Ensure method exists
	_, err = db.Exec(`
INSERT INTO dim_match_method (method_id, method_code, method_name, description)
VALUES ($1, 'individual_fuzzy', 'Individual Fuzzy Matching', 'Direct fuzzy matching for individual documents')
ON CONFLICT (method_id) DO NOTHING`, individualFuzzyMethodID)
	if err != nil {
		return fmt.Errorf("failed to ensure individual fuzzy method exists: %v", err)
	}

	for i, doc := range individualDocs {
		if i%100 == 0 || localDebug {
			fmt.Printf("Processing document %d/%d: ID=%d, current conf=%.3f\n", 
				i+1, len(individualDocs), doc.DocumentID, doc.CurrentConfidence)
		}

		// Find fuzzy matches for this address
		fuzzyMatchSQL := `
SELECT 
    da.address_id,
    da.uprn,
    da.full_address,
    dl.location_id,
    similarity(da.full_address, $1) as similarity_score,
    levenshtein(UPPER(da.full_address), UPPER($1)) as edit_distance
FROM dim_address da
JOIN dim_location dl ON da.location_id = dl.location_id
WHERE da.full_address % $1  -- Uses trigram index
  AND similarity(da.full_address, $1) >= 0.6  -- Higher threshold for individual matching
ORDER BY similarity_score DESC, edit_distance ASC
LIMIT 3`

		type FuzzyMatch struct {
			AddressID       int64
			UPRN            string
			FullAddress     string
			LocationID      int64
			SimilarityScore float64
			EditDistance    int
		}

		fuzzyRows, err := db.Query(fuzzyMatchSQL, doc.RawAddress)
		if err != nil {
			if localDebug {
				fmt.Printf("  Warning: failed to find fuzzy matches for doc %d: %v\n", doc.DocumentID, err)
			}
			continue
		}

		var matches []FuzzyMatch
		for fuzzyRows.Next() {
			var fm FuzzyMatch
			err := fuzzyRows.Scan(&fm.AddressID, &fm.UPRN, &fm.FullAddress, &fm.LocationID, &fm.SimilarityScore, &fm.EditDistance)
			if err != nil {
				continue
			}
			matches = append(matches, fm)
		}
		fuzzyRows.Close()

		// Apply match if we have a good one
		if len(matches) > 0 {
			bestMatch := matches[0]
			
			// Higher standards for individual matching
			minSimilarity := 0.7   // Require higher similarity
			maxEditDistance := 20  // Stricter edit distance
			
			if bestMatch.SimilarityScore >= minSimilarity && bestMatch.EditDistance <= maxEditDistance {
				// Apply the correction
				correctionSQL := `
INSERT INTO address_match_corrected (
    document_id, original_address_id, original_confidence_score, original_method_id,
    corrected_address_id, corrected_location_id, corrected_confidence_score, 
    corrected_method_id, correction_reason, planning_app_base
) VALUES ($1, NULL, $2, 1, $3, $4, $5, $6, $7, $8)
ON CONFLICT (document_id) DO UPDATE SET
    corrected_address_id = EXCLUDED.corrected_address_id,
    corrected_location_id = EXCLUDED.corrected_location_id,
    corrected_confidence_score = EXCLUDED.corrected_confidence_score,
    correction_reason = EXCLUDED.correction_reason`

				correctionReason := fmt.Sprintf("Individual fuzzy match - similarity: %.3f, edit distance: %d, matched to: %.50s", 
					bestMatch.SimilarityScore, bestMatch.EditDistance, bestMatch.FullAddress)

				planningAppBase := ""
				if doc.PlanningAppBase.Valid {
					planningAppBase = doc.PlanningAppBase.String
				}

				_, err = db.Exec(correctionSQL, 
					doc.DocumentID, doc.CurrentConfidence, bestMatch.AddressID, bestMatch.LocationID,
					bestMatch.SimilarityScore, individualFuzzyMethodID, correctionReason, planningAppBase)

				if err != nil {
					if localDebug {
						fmt.Printf("  Failed to apply individual correction for doc %d: %v\n", doc.DocumentID, err)
					}
				} else {
					totalCorrections++
					if localDebug {
						fmt.Printf("  ✓ Individual match: %.40s → %.40s (sim: %.3f)\n", 
							doc.RawAddress, bestMatch.FullAddress, bestMatch.SimilarityScore)
					}
				}
			} else if localDebug {
				fmt.Printf("  No good individual match (best sim: %.3f, edit: %d)\n", 
					bestMatch.SimilarityScore, bestMatch.EditDistance)
			}
		}
	}

	fmt.Printf("Applied %d individual fuzzy match corrections\n", totalCorrections)
	return nil
}

// standardizeSourceAddresses cleans and standardizes source addresses before matching
func standardizeSourceAddresses(localDebug bool, db *sql.DB) error {
	fmt.Println("Standardizing and cleaning source addresses...")

	// Common UK address abbreviation expansions
	standardizations := map[string]string{
		"\\bEST\\b":     "ESTATE",
		"\\bRD\\b":      "ROAD",
		"\\bRDS\\b":     "ROADS", 
		"\\bST\\b":      "STREET",
		"\\bAVE\\b":     "AVENUE",
		"\\bCRESC\\b":   "CRESCENT",
		"\\bCRES\\b":    "CRESCENT",
		"\\bCL\\b":      "CLOSE",
		"\\bCLS\\b":     "CLOSE",
		"\\bCT\\b":      "COURT",
		"\\bDR\\b":      "DRIVE",
		"\\bGDNS\\b":    "GARDENS",
		"\\bLN\\b":      "LANE",
		"\\bPK\\b":      "PARK",
		"\\bPL\\b":      "PLACE",
		"\\bSQ\\b":      "SQUARE",
		"\\bTER\\b":     "TERRACE",
		"\\bWY\\b":      "WAY",
		"\\bIND EST\\b": "INDUSTRIAL ESTATE",
		"\\bIND\\s+EST\\b": "INDUSTRIAL ESTATE",
		"\\bINDUSTL\\b": "INDUSTRIAL",
		"\\bHANTS\\b":   "HAMPSHIRE",
	}

	// Add standardized_address column if it doesn't exist
	_, err := db.Exec(`
ALTER TABLE src_document 
ADD COLUMN IF NOT EXISTS standardized_address TEXT`)
	if err != nil {
		return fmt.Errorf("failed to add standardized_address column: %v", err)
	}

	// Process all source documents
	fmt.Println("Processing source address standardization...")
	
	updateSQL := `
UPDATE src_document 
SET standardized_address = $2 
WHERE document_id = $1`

	rows, err := db.Query("SELECT document_id, raw_address FROM src_document WHERE raw_address IS NOT NULL AND raw_address != ''")
	if err != nil {
		return fmt.Errorf("failed to query source documents: %v", err)
	}
	defer rows.Close()

	var processedCount int
	for rows.Next() {
		var documentID int
		var rawAddress string
		err := rows.Scan(&documentID, &rawAddress)
		if err != nil {
			continue
		}

		// Clean and standardize the address
		standardized := strings.ToUpper(strings.TrimSpace(rawAddress))
		
		// Apply standardizations using regex
		for pattern, replacement := range standardizations {
			re := regexp.MustCompile(pattern)
			standardized = re.ReplaceAllString(standardized, replacement)
		}

		// Additional cleaning
		standardized = regexp.MustCompile(`\s+`).ReplaceAllString(standardized, " ")           // Multiple spaces to single
		standardized = regexp.MustCompile(`[^\w\s,.-]`).ReplaceAllString(standardized, "")    // Remove special chars except basic punctuation
		standardized = strings.TrimSpace(standardized)

		// Update the database
		_, err = db.Exec(updateSQL, documentID, standardized)
		if err != nil && localDebug {
			fmt.Printf("Failed to update document %d: %v\n", documentID, err)
			continue
		}

		processedCount++
		if processedCount%1000 == 0 {
			fmt.Printf("Processed %d addresses\n", processedCount)
		}

		if localDebug && processedCount <= 10 {
			fmt.Printf("  %d: '%s' → '%s'\n", documentID, rawAddress, standardized)
		}
	}

	fmt.Printf("Standardized %d source addresses\n", processedCount)

	// Create index on standardized addresses for better performance
	fmt.Println("Creating index on standardized addresses...")
	_, err = db.Exec(`
CREATE INDEX IF NOT EXISTS idx_src_document_standardized_address 
ON src_document USING gin(standardized_address gin_trgm_ops)`)
	if err != nil {
		fmt.Printf("Warning: failed to create standardized address index: %v\n", err)
	}

	return nil
}

// runComprehensiveMatching runs the complete multi-layered matching strategy
func runComprehensiveMatching(localDebug bool, db *sql.DB) error {
	fmt.Println("Running comprehensive multi-layered matching strategy...")
	fmt.Println("=======================================================")

	// Layer 0: Data Cleaning (NEW)
	fmt.Println("\n--- LAYER 0: Data Cleaning ---")
	err := cleanSourceAddressData(localDebug, db)
	if err != nil {
		return fmt.Errorf("layer 0 failed: %v", err)
	}

	// Layer 1: Intelligent Fact Table Population (REDESIGNED)
	fmt.Println("\n--- LAYER 1: Intelligent Fact Table Population ---")
	err = rebuildFactTableIntelligent(localDebug, db)
	if err != nil {
		return fmt.Errorf("layer 1 failed: %v", err)
	}

	// Layer 2: Conservative matching (high-precision deterministic first)
	fmt.Println("\n--- LAYER 2: Conservative Validation Matching ---")
	err = runConservativeMatching(localDebug, db, "comprehensive-conservative")
	if err != nil {
		return fmt.Errorf("layer 2 failed: %v", err)
	}
	
	// Layer 3: Group-based fuzzy matching (for remaining unmatched)
	fmt.Println("\n--- LAYER 3: Group-based Fuzzy Matching ---")
	err = fuzzyMatchUnmatchedGroups(localDebug, db)
	if err != nil {
		return fmt.Errorf("layer 2 failed: %v", err)
	}

	// Layer 3: Individual document fuzzy matching
	fmt.Println("\n--- LAYER 3: Individual Document Fuzzy Matching ---")
	err = fuzzyMatchIndividualDocuments(localDebug, db)
	if err != nil {
		return fmt.Errorf("layer 3 failed: %v", err)
	}

	// Layer 4: Group consensus corrections
	fmt.Println("\n--- LAYER 4: Group Consensus Corrections ---")
	err = applyGroupConsensusCorrections(localDebug, db)
	if err != nil {
		return fmt.Errorf("layer 4 failed: %v", err)
	}

	// Summary statistics
	fmt.Println("\n--- FINAL STATISTICS ---")
	err = showStatistics(localDebug, db)
	if err != nil {
		fmt.Printf("Warning: failed to show final statistics: %v\n", err)
	}

	fmt.Println("\n=======================================================")
	fmt.Println("Comprehensive multi-layered matching completed!")
	return nil
}

// runConservativeMatching runs the new conservative address matching with component validation
func runConservativeMatching(localDebug bool, db *sql.DB, runLabel string) error {
	fmt.Println("Running Conservative Address Matching...")
	fmt.Println("=====================================")
	fmt.Printf("Using validation framework to prevent false positive matches\n")
	fmt.Printf("Algorithm: Conservative validation with house number + street verification\n\n")

	// Initialize the address validator
	validator := validation.NewAddressValidator()

	// Process ALL unmatched records for production run
	query := `
		SELECT f.fact_id, f.document_id, o.raw_address, dt.type_name as source_type, s.raw_uprn
		FROM fact_documents_lean f
		JOIN dim_original_address o ON f.original_address_id = o.original_address_id
		JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
		LEFT JOIN src_document s ON f.document_id = s.document_id
		WHERE f.matched_address_id IS NULL 
			AND o.raw_address IS NOT NULL 
			AND o.raw_address != ''
		ORDER BY 
			-- Process documents with source UPRNs first
			CASE WHEN s.raw_uprn IS NOT NULL AND s.raw_uprn != '' THEN 0 ELSE 1 END,
			dt.type_name, f.document_id
	`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query unmatched documents: %v", err)
	}
	defer rows.Close()

	// PRODUCTION OPTIMIZATION: Use database-level matching instead of loading all addresses
	fmt.Printf("Using optimized database-level matching for production scale\n")

	// Process unmatched documents
	var processedCount, acceptedCount, rejectedCount, reviewCount int
	var sourceUPRNCount int // Track documents matched using source UPRNs
	
	for rows.Next() {
		var factID, docID int64
		var sourceAddress, sourceType string
		var sourceUPRN sql.NullString
		
		err := rows.Scan(&factID, &docID, &sourceAddress, &sourceType, &sourceUPRN)
		if err != nil {
			return fmt.Errorf("failed to scan document row: %v", err)
		}

		processedCount++
		
		if localDebug && processedCount <= 10 {
			fmt.Printf("\n--- Processing Document %d ---\n", docID)
			fmt.Printf("Source: %s (%s)\n", sourceAddress, sourceType)
			if sourceUPRN.Valid && sourceUPRN.String != "" {
				fmt.Printf("Source UPRN: %s\n", sourceUPRN.String)
			}
		}

		// Check if document already has a source UPRN
		if sourceUPRN.Valid && sourceUPRN.String != "" {
			// Find the address record for this source UPRN
			var targetAddressID int64
			err := db.QueryRow(`
				SELECT address_id FROM dim_address 
				WHERE uprn = $1 AND is_historic = false 
				LIMIT 1
			`, sourceUPRN.String).Scan(&targetAddressID)
			
			if err == nil {
				// Found matching address for source UPRN
				acceptedCount++
				sourceUPRNCount++
				
				if localDebug && processedCount <= 10 {
					fmt.Printf("USING SOURCE UPRN: %s (Address ID: %d)\n", sourceUPRN.String, targetAddressID)
				}

				// Update fact table with source UPRN match
				_, err = db.Exec(`
					UPDATE fact_documents_lean 
					SET matched_address_id = $1,
						match_method_id = 25, -- source_uprn method
						match_confidence_score = 1.0,
						updated_at = NOW()
					WHERE fact_id = $2
				`, targetAddressID, factID)
				
				if err != nil {
					fmt.Printf("Warning: failed to update document %d with source UPRN: %v\n", docID, err)
				}
				
				// Skip to next document - no need for conservative matching
				continue
			} else if localDebug && processedCount <= 10 {
				fmt.Printf("Source UPRN %s not found in dim_address, proceeding with conservative matching\n", sourceUPRN.String)
			}
		}

		// PRODUCTION OPTIMIZATION: Use targeted database queries for matching
		ctx := &runConservativeMatchingContext{}
		bestMatch, bestDecision, err := ctx.findBestMatchOptimized(db, sourceAddress, validator, localDebug && processedCount <= 10)
		if err != nil {
			if localDebug && processedCount <= 10 {
				fmt.Printf("Error finding match: %v\n", err)
			}
			rejectedCount++
			continue
		}

		// Process the result
		if bestMatch != nil && bestDecision.Accept {
			acceptedCount++
			
			if localDebug && processedCount <= 10 {
				fmt.Printf("MATCH FOUND: UPRN %s (Address ID: %d)\n", bestMatch.UPRN, bestMatch.AddressID)
				fmt.Printf("Target: %s\n", bestMatch.Address)
				fmt.Printf("Decision: %s\n", bestDecision.String())
				if bestDecision.ComponentValidation.HouseNumberMatch.Valid {
					fmt.Printf("House Match: %s\n", bestDecision.ComponentValidation.HouseNumberMatch.String())
				}
				if bestDecision.ComponentValidation.StreetMatch.Valid {
					fmt.Printf("Street Match: %s\n", bestDecision.ComponentValidation.StreetMatch.String())
				}
			}

			// Update the dimensional fact table with the conservative match
			_, err = db.Exec(`
				UPDATE fact_documents_lean 
				SET matched_address_id = $1,
				    match_method_id = 26, -- conservative method
				    match_confidence_score = $2,
				    updated_at = NOW()
				WHERE fact_id = $3
			`, bestMatch.AddressID, bestDecision.Confidence, factID)
			
			if err != nil {
				fmt.Printf("Warning: failed to update document %d: %v\n", docID, err)
			}

		} else if bestDecision.RequiresReview {
			reviewCount++
			
			if localDebug && processedCount <= 10 {
				fmt.Printf("REQUIRES REVIEW: %s\n", bestDecision.Reason)
			}
			
		} else {
			rejectedCount++
			
			if localDebug && processedCount <= 10 {
				fmt.Printf("NO MATCH: %s\n", bestDecision.Reason)
			}
		}

		// Progress update
		if processedCount%100 == 0 {
			fmt.Printf("Processed %d documents... (Accepted: %d, Review: %d, Rejected: %d)\n",
				processedCount, acceptedCount, reviewCount, rejectedCount)
		}
	}

	// Final summary
	fmt.Println("\n=== CONSERVATIVE MATCHING SUMMARY ===")
	fmt.Printf("Documents processed: %d\n", processedCount)
	fmt.Printf("Total matches: %d (%.1f%%)\n", acceptedCount, 
		float64(acceptedCount)*100.0/float64(processedCount))
	fmt.Printf("  └─ From Source UPRN: %d (%.1f%%)\n", sourceUPRNCount,
		float64(sourceUPRNCount)*100.0/float64(processedCount))
	fmt.Printf("  └─ Conservative Matched: %d (%.1f%%)\n", acceptedCount-sourceUPRNCount,
		float64(acceptedCount-sourceUPRNCount)*100.0/float64(processedCount))
	fmt.Printf("Requiring review: %d (%.1f%%)\n", reviewCount, 
		float64(reviewCount)*100.0/float64(processedCount))
	fmt.Printf("Rejected: %d (%.1f%%)\n", rejectedCount, 
		float64(rejectedCount)*100.0/float64(processedCount))
	fmt.Println("=====================================")

	fmt.Printf("\nMatch Source Tracking:\n")
	fmt.Printf("✓ 'From Source Data' = UPRN was in original document\n")
	fmt.Printf("✓ 'Conservative Validation' = UPRN found through matching\n\n")

	fmt.Printf("Conservative validation framework prevents false positives:\n")
	fmt.Printf("✓ House number mismatches: '168' ≠ '147' (auto-rejected)\n")
	fmt.Printf("✓ Unit mismatches: 'Unit 10' ≠ 'Unit 7' (auto-rejected)\n") 
	fmt.Printf("✓ Street similarity <90%% threshold (requires manual review)\n")
	fmt.Printf("✓ Component extraction confidence >95%% required\n")
	fmt.Printf("✓ Vague addresses ('Land at', 'Rear of') excluded\n")

	return nil
}

// PRODUCTION OPTIMIZATION: Efficient database-driven matching
type OptimizedMatch struct {
	AddressID int64
	UPRN      string
	Address   string
}

// findBestMatchOptimized uses database-level optimizations for efficient matching
func (v *runConservativeMatchingContext) findBestMatchOptimized(db *sql.DB, sourceAddress string, validator *validation.AddressValidator, debug bool) (*OptimizedMatch, validation.MatchDecision, error) {
	// Parse source address to extract components for targeted searching
	components := v.ParseAddress(sourceAddress)
	
	if debug {
		fmt.Printf("Parsed components: %s\n", components.String())
	}
	
	// Pre-validate the source address
	addrValidation := v.ValidateAddressForMatching(sourceAddress)
	if !addrValidation.Suitable {
		if debug {
			fmt.Printf("Source address not suitable for matching: %v\n", addrValidation.Issues)
		}
		return nil, validation.MatchDecision{Accept: false, Reason: "Source address validation failed"}, nil
	}
	
	// STRATEGY 1: Canonical address matching (most reliable)
	match, decision, err := v.tryCanonicalAddressMatch(db, sourceAddress, validator, debug)
	if err != nil {
		return nil, validation.MatchDecision{}, err
	}
	if match != nil && decision.Accept {
		return match, decision, nil
	}
	
	// STRATEGY 2: Exact component matching (fallback)
	if components.HasHouseNumber() && components.HasStreet() {
		match, decision, err := v.tryExactComponentMatch(db, components, validator, debug)
		if err != nil {
			return nil, validation.MatchDecision{}, err
		}
		if match != nil && decision.Accept {
			return match, decision, nil
		}
	}
	
	// STRATEGY 3: Postcode + House Number matching (high precision)
	if components.HasHouseNumber() && components.HasValidPostcode() {
		match, decision, err := v.tryPostcodeHouseMatch(db, components, validator, debug)
		if err != nil {
			return nil, validation.MatchDecision{}, err
		}
		if match != nil && decision.Accept {
			return match, decision, nil
		}
	}
	
	// STRATEGY 4: Street similarity matching (medium precision)
	if components.HasStreet() {
		match, decision, err := v.tryStreetSimilarityMatch(db, components, validator, debug)
		if err != nil {
			return nil, validation.MatchDecision{}, err
		}
		if match != nil && decision.Accept {
			return match, decision, nil
		}
	}
	
	// No suitable match found
	return nil, validation.MatchDecision{
		Accept: false,
		Reason: "No matches found meeting conservative validation criteria",
		Method: "Conservative Search Complete",
	}, nil
}

// tryCanonicalAddressMatch uses canonical address similarity for precise matching
func (v *runConservativeMatchingContext) tryCanonicalAddressMatch(db *sql.DB, sourceAddress string, validator *validation.AddressValidator, debug bool) (*OptimizedMatch, validation.MatchDecision, error) {
	// Normalize source address to canonical form
	sourceCanonical := strings.ToUpper(strings.TrimSpace(sourceAddress))
	// Remove punctuation and extra spaces
	sourceCanonical = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(sourceCanonical, "")
	sourceCanonical = regexp.MustCompile(`\s+`).ReplaceAllString(sourceCanonical, " ")
	sourceCanonical = strings.TrimSpace(sourceCanonical)
	
	if debug {
		fmt.Printf("Canonical address search for: '%s' (from: '%s')\n", sourceCanonical, sourceAddress)
	}
	
	// Query both original and expanded addresses using canonical similarity
	query := `
	SELECT address_id, uprn, full_address, address_canonical, source_type, 
	       similarity(UPPER(address_canonical), $1) as sim_score
	FROM (
		(SELECT a.address_id, a.uprn, a.full_address, a.address_canonical, 'original' as source_type
		 FROM dim_address a
		 WHERE a.uprn IS NOT NULL
		   AND a.full_address IS NOT NULL
		   AND a.address_canonical IS NOT NULL
		   AND similarity(UPPER(a.address_canonical), $1) >= 0.5)
		UNION ALL
		(SELECT e.original_address_id, e.uprn, e.full_address, e.address_canonical, 'expanded' as source_type
		 FROM dim_address_expanded e
		 WHERE e.uprn IS NOT NULL
		   AND e.full_address IS NOT NULL
		   AND e.address_canonical IS NOT NULL
		   AND similarity(UPPER(e.address_canonical), $1) >= 0.5)
	) combined
	ORDER BY 
		-- Prefer expanded addresses, then by similarity
		CASE WHEN source_type = 'expanded' THEN 0 ELSE 1 END,
		sim_score DESC,
		LENGTH(full_address),
		address_id
	LIMIT 5
	`
	
	rows, err := db.Query(query, sourceCanonical)
	if err != nil {
		return nil, validation.MatchDecision{}, fmt.Errorf("canonical address query failed: %v", err)
	}
	defer rows.Close()
	
	var candidates []struct {
		OptimizedMatch
		SimScore float64
		Canonical string
		SourceType string
	}
	
	for rows.Next() {
		var candidate struct {
			OptimizedMatch
			SimScore float64
			Canonical string
			SourceType string
		}
		
		err := rows.Scan(&candidate.AddressID, &candidate.UPRN, &candidate.Address, 
			&candidate.Canonical, &candidate.SourceType, &candidate.SimScore)
		if err != nil {
			continue
		}
		
		candidates = append(candidates, candidate)
		
		if debug {
			fmt.Printf("  Candidate: %s (sim: %.3f, canonical: %s)\n", 
				candidate.Address, candidate.SimScore, candidate.Canonical)
		}
	}
	
	if len(candidates) == 0 {
		if debug {
			fmt.Printf("  No canonical matches found for: '%s'\n", sourceAddress)
		}
		return nil, validation.MatchDecision{Accept: false, Reason: "No canonical similarity matches"}, nil
	}
	
	// Validate the best candidate
	bestCandidate := candidates[0]
	
	// Use conservative validation criteria for canonical matches
	validationResult := validator.MakeMatchDecision(sourceAddress, bestCandidate.Address)
	
	if debug {
		fmt.Printf("  Best canonical match validation: accept=%v, confidence=%.3f\n", 
			validationResult.Accept, validationResult.Confidence)
	}
	
	if validationResult.Accept {
		return &bestCandidate.OptimizedMatch, validationResult, nil
	}
	
	return nil, validationResult, nil
}

// tryExactComponentMatch looks for exact house number + street matches
func (v *runConservativeMatchingContext) tryExactComponentMatch(db *sql.DB, components validation.AddressComponents, validator *validation.AddressValidator, debug bool) (*OptimizedMatch, validation.MatchDecision, error) {
	// Option A: Query both original addresses AND expanded addresses
	query := `
	SELECT address_id, uprn, full_address, source_type FROM (
		(SELECT a.address_id, a.uprn, a.full_address, 'original' as source_type
		 FROM dim_address a
		 WHERE a.uprn IS NOT NULL
		   AND a.full_address IS NOT NULL
		   AND UPPER(a.full_address) LIKE '%' || $1 || '%'
		   AND UPPER(a.full_address) LIKE '%' || $2 || '%')
		UNION ALL
		(SELECT e.original_address_id, e.uprn, e.full_address, 'expanded' as source_type
		 FROM dim_address_expanded e
		 WHERE e.uprn IS NOT NULL
		   AND e.full_address IS NOT NULL
		   AND UPPER(e.full_address) LIKE '%' || $1 || '%'
		   AND UPPER(e.full_address) LIKE '%' || $2 || '%')
	) combined
	ORDER BY 
		-- Prefer expanded addresses, then shorter addresses
		CASE WHEN source_type = 'expanded' THEN 0 ELSE 1 END,
		LENGTH(full_address),
		address_id
	LIMIT 10
	`
	
	houseNum := strings.ToUpper(strings.TrimSpace(components.HouseNumber))
	street := strings.ToUpper(strings.TrimSpace(components.Street))
	
	if debug {
		fmt.Printf("Exact component search: house='%s', street='%s'\n", houseNum, street)
		fmt.Printf("SQL Query: %s\n", query)
	}
	
	rows, err := db.Query(query, houseNum, street)
	if err != nil {
		return nil, validation.MatchDecision{}, fmt.Errorf("exact component query failed: %v", err)
	}
	defer rows.Close()
	
	return v.evaluateCandidates(rows, validator, components.Raw, debug, "Exact Component")
}

// tryPostcodeHouseMatch looks for postcode + house number matches
func (v *runConservativeMatchingContext) tryPostcodeHouseMatch(db *sql.DB, components validation.AddressComponents, validator *validation.AddressValidator, debug bool) (*OptimizedMatch, validation.MatchDecision, error) {
	// Option A: Query both original addresses AND expanded addresses
	query := `
		(SELECT a.address_id, a.uprn, a.full_address, 'original' as source_type
		 FROM dim_address a
		 WHERE a.uprn IS NOT NULL
		   AND a.full_address IS NOT NULL
		   AND UPPER(a.full_address) LIKE '%' || $1 || '%'
		   AND UPPER(a.full_address) LIKE '%' || $2 || '%')
		UNION ALL
		(SELECT e.original_address_id, e.uprn, e.full_address, 'expanded' as source_type
		 FROM dim_address_expanded e
		 WHERE e.uprn IS NOT NULL
		   AND e.full_address IS NOT NULL
		   AND UPPER(e.full_address) LIKE '%' || $1 || '%'
		   AND UPPER(e.full_address) LIKE '%' || $2 || '%')
		ORDER BY 
			CASE WHEN source_type = 'expanded' THEN 0 ELSE 1 END,
			LENGTH(full_address), 
			address_id
		LIMIT 20
	`
	
	houseNum := strings.ToUpper(strings.TrimSpace(components.HouseNumber))
	postcode := strings.ToUpper(strings.TrimSpace(components.Postcode))
	
	if debug {
		fmt.Printf("Postcode+House search: house='%s', postcode='%s'\n", houseNum, postcode)
	}
	
	rows, err := db.Query(query, houseNum, postcode)
	if err != nil {
		return nil, validation.MatchDecision{}, fmt.Errorf("postcode+house query failed: %v", err)
	}
	defer rows.Close()
	
	return v.evaluateCandidates(rows, validator, components.Raw, debug, "Postcode+House")
}

// tryStreetSimilarityMatch uses trigram similarity for street-based matching
func (v *runConservativeMatchingContext) tryStreetSimilarityMatch(db *sql.DB, components validation.AddressComponents, validator *validation.AddressValidator, debug bool) (*OptimizedMatch, validation.MatchDecision, error) {
	// Option A: Query both original addresses AND expanded addresses
	query := `
		SELECT address_id, uprn, full_address, sim_score, source_type FROM (
		  (SELECT a.address_id, a.uprn, a.full_address,
			     similarity(UPPER(a.full_address), UPPER($1)) as sim_score,
			     'original' as source_type
		   FROM dim_address a
		   WHERE a.uprn IS NOT NULL
		     AND a.full_address IS NOT NULL
		     AND similarity(UPPER(a.full_address), UPPER($1)) > 0.3)
		  UNION ALL
		  (SELECT e.original_address_id, e.uprn, e.full_address,
			     similarity(UPPER(e.full_address), UPPER($1)) as sim_score,
			     'expanded' as source_type
		   FROM dim_address_expanded e
		   WHERE e.uprn IS NOT NULL
		     AND e.full_address IS NOT NULL
		     AND similarity(UPPER(e.full_address), UPPER($1)) > 0.3)
		) combined
		ORDER BY 
		  CASE WHEN source_type = 'expanded' THEN 0 ELSE 1 END,
		  sim_score DESC, 
		  LENGTH(full_address)
		LIMIT 50  -- Get top candidates for validation
	`
	
	if debug {
		fmt.Printf("Street similarity search for: '%s'\n", components.Raw)
	}
	
	rows, err := db.Query(query, components.Raw)
	if err != nil {
		return nil, validation.MatchDecision{}, fmt.Errorf("street similarity query failed: %v", err)
	}
	defer rows.Close()
	
	return v.evaluateCandidatesWithSimilarity(rows, validator, components.Raw, debug, "Street Similarity")
}

// evaluateCandidates processes query results and applies conservative validation
func (v *runConservativeMatchingContext) evaluateCandidates(rows *sql.Rows, validator *validation.AddressValidator, sourceAddress string, debug bool, method string) (*OptimizedMatch, validation.MatchDecision, error) {
	var bestMatch *OptimizedMatch
	var bestDecision validation.MatchDecision
	bestConfidence := 0.0
	candidateCount := 0
	
	for rows.Next() {
		var candidate OptimizedMatch
		var sourceType string // Extra column from UNION query
		err := rows.Scan(&candidate.AddressID, &candidate.UPRN, &candidate.Address, &sourceType)
		if err != nil {
			return nil, validation.MatchDecision{}, fmt.Errorf("failed to scan candidate: %v", err)
		}
		
		candidateCount++
		
		// Apply conservative validation
		decision := validator.MakeMatchDecision(sourceAddress, candidate.Address)
		
		if debug && candidateCount <= 3 {
			fmt.Printf("  Candidate %d: %s -> %s (confidence: %.3f)\n", 
				candidateCount, candidate.Address, decision.String(), decision.Confidence)
		}
		
		if decision.Accept && decision.Confidence > bestConfidence {
			bestConfidence = decision.Confidence
			bestDecision = decision
			bestMatch = &candidate
		}
	}
	
	if debug {
		fmt.Printf("%s search: evaluated %d candidates, best confidence: %.3f\n", 
			method, candidateCount, bestConfidence)
	}
	
	return bestMatch, bestDecision, nil
}

// evaluateCandidatesWithSimilarity handles results that include similarity scores
func (v *runConservativeMatchingContext) evaluateCandidatesWithSimilarity(rows *sql.Rows, validator *validation.AddressValidator, sourceAddress string, debug bool, method string) (*OptimizedMatch, validation.MatchDecision, error) {
	var bestMatch *OptimizedMatch
	var bestDecision validation.MatchDecision
	bestConfidence := 0.0
	candidateCount := 0
	
	for rows.Next() {
		var candidate OptimizedMatch
		var simScore float64
		var sourceType string // Extra column from UNION query
		err := rows.Scan(&candidate.AddressID, &candidate.UPRN, &candidate.Address, &simScore, &sourceType)
		if err != nil {
			return nil, validation.MatchDecision{}, fmt.Errorf("failed to scan candidate with similarity: %v", err)
		}
		
		candidateCount++
		
		// Apply conservative validation
		decision := validator.MakeMatchDecision(sourceAddress, candidate.Address)
		
		if debug && candidateCount <= 3 {
			fmt.Printf("  Candidate %d (sim: %.3f): %s -> %s (confidence: %.3f)\n", 
				candidateCount, simScore, candidate.Address, decision.String(), decision.Confidence)
		}
		
		if decision.Accept && decision.Confidence > bestConfidence {
			bestConfidence = decision.Confidence
			bestDecision = decision
			bestMatch = &candidate
		}
	}
	
	if debug {
		fmt.Printf("%s search: evaluated %d candidates, best confidence: %.3f\n", 
			method, candidateCount, bestConfidence)
	}
	
	return bestMatch, bestDecision, nil
}

// Context struct to hold method receivers (Go doesn't allow methods on function types)
type runConservativeMatchingContext struct{}

func (v *runConservativeMatchingContext) ParseAddress(address string) validation.AddressComponents {
	parser := validation.NewAddressParser()
	return parser.ParseAddress(address)
}

func (v *runConservativeMatchingContext) ValidateAddressForMatching(address string) validation.AddressValidation {
	parser := validation.NewAddressParser()
	return parser.ValidateAddressForMatching(address)
}

// expandLLPGRanges expands LLPG range addresses (e.g., "10-11") into individual addresses
func expandLLPGRanges(localDebug bool, db *sql.DB) error {
	fmt.Println("Expanding LLPG Range Addresses...")
	fmt.Println("==================================")
	
	expander := llpg.NewRangeExpander(db)
	
	// Initialize the expanded table
	fmt.Println("Initializing expanded address table...")
	if err := expander.InitializeExpandedTable(); err != nil {
		return fmt.Errorf("failed to initialize expanded table: %v", err)
	}
	
	// Perform the expansion
	fmt.Println("Processing range expansions...")
	fmt.Println("  - Numeric ranges (e.g., 10-11 → 10, 11)")
	fmt.Println("  - Unit ranges (e.g., Unit 3-4 → Unit 3, Unit 4)")
	fmt.Println("  - Alpha ranges (e.g., 9A-9C → 9A, 9B, 9C)")
	
	startTime := time.Now()
	expandedCount, err := expander.ExpandAllRanges()
	if err != nil {
		return fmt.Errorf("failed to expand ranges: %v", err)
	}
	
	duration := time.Since(startTime)
	
	// Get statistics
	stats, err := expander.GetExpandedAddressStats()
	if err != nil {
		return fmt.Errorf("failed to get statistics: %v", err)
	}
	
	fmt.Printf("\n=== EXPANSION COMPLETE ===\n")
	fmt.Printf("Time taken: %v\n", duration)
	fmt.Printf("Addresses expanded: %d\n", expandedCount)
	fmt.Printf("\nTable Statistics:\n")
	for expansionType, count := range stats {
		fmt.Printf("  - %s: %d\n", expansionType, count)
	}
	fmt.Printf("\nTotal addresses available: %d\n", stats["original"]+stats["range_expansion"])
	
	// Show some examples
	fmt.Println("\nExample expansions:")
	rows, err := db.Query(`
		SELECT 
			o.full_address as original,
			e.full_address as expanded,
			e.unit_number
		FROM dim_address o
		JOIN dim_address_expanded e ON o.address_id = e.original_address_id
		WHERE e.expansion_type = 'range_expansion'
		  AND o.full_address LIKE '%-%'
		ORDER BY o.full_address, e.unit_number
		LIMIT 5
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var original, expanded, unit string
			if rows.Scan(&original, &expanded, &unit) == nil {
				fmt.Printf("  %s\n    → Unit %s: %s\n", original, unit, expanded)
			}
		}
	}
	
	return nil
}

// cleanSourceAddressData fixes spelling errors and formatting issues in source addresses
func cleanSourceAddressData(localDebug bool, db *sql.DB) error {
	fmt.Println("Cleaning source address data...")
	fmt.Println("==================================")
	
	// Define address corrections
	corrections := map[string]string{
		"PFTERSFTELD":                                          "PETERSFIELD",
		"PETERSFIEID":                                          "PETERSFIELD", 
		"HANTS":                                                "HAMPSHIRE",
		"SOUTH VIEW":                                           "SOUTHVIEW",
		"HOLLY BANK":                                           "HOLLYBANK",
		"FOUR YRARKS":                                          "FOUR MARKS",
		" THORN LANE, FOUR MARKS":                              " THORN LANE FOUR MARKS ALTON GU34 5BX",
	}
	
	totalUpdates := 0
	
	for incorrect, correct := range corrections {
		fmt.Printf("Fixing '%s' → '%s'...\n", incorrect, correct)
		
		// Update raw_address
		updateQuery := `
		UPDATE src_document 
		SET raw_address = REPLACE(raw_address, $1, $2)
		WHERE raw_address LIKE '%' || $1 || '%'
		`
		
		result, err := db.Exec(updateQuery, incorrect, correct)
		if err != nil {
			return fmt.Errorf("failed to update '%s': %v", incorrect, err)
		}
		
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			fmt.Printf("  ✓ Updated %d records\n", rowsAffected)
			totalUpdates += int(rowsAffected)
		}
		
		// Also update standardized_address if it exists
		updateStandardizedQuery := `
		UPDATE src_document 
		SET standardized_address = REPLACE(standardized_address, $1, $2)
		WHERE standardized_address LIKE '%' || $1 || '%'
		`
		
		result, err = db.Exec(updateStandardizedQuery, incorrect, correct)
		if err != nil {
			// Ignore errors for standardized_address as it might not exist yet
			continue
		}
		
		rowsAffected, _ = result.RowsAffected()
		if rowsAffected > 0 {
			fmt.Printf("  ✓ Updated %d standardized addresses\n", rowsAffected)
		}
	}
	
	fmt.Printf("\nTotal address corrections applied: %d\n", totalUpdates)
	
	// Additional cleaning: trim whitespace and normalize case
	fmt.Println("\nNormalizing address formats...")
	normalizeQuery := `
	UPDATE src_document 
	SET raw_address = TRIM(UPPER(raw_address))
	WHERE raw_address != TRIM(UPPER(raw_address))
	`
	
	result, err := db.Exec(normalizeQuery)
	if err != nil {
		return fmt.Errorf("failed to normalize addresses: %v", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("  ✓ Normalized %d addresses\n", rowsAffected)
	
	// Create enhanced canonical addresses that handle property names
	fmt.Println("\nCreating enhanced canonical addresses...")
	enhancedCanonicalQuery := `
	UPDATE src_document 
	SET standardized_address = CASE
		-- Extract number + street when there's a property name prefix
		WHEN raw_address ~ '^[A-Z0-9\s]*, \d+[A-Z]? [A-Z\s]+' 
		THEN TRIM(SUBSTRING(raw_address FROM ', (.+)'))
		ELSE raw_address
	END
	WHERE raw_address ~ '^[A-Z0-9\s]*, \d+[A-Z]? [A-Z\s]+'
	`
	
	result, err = db.Exec(enhancedCanonicalQuery)
	if err != nil {
		return fmt.Errorf("failed to create enhanced canonical addresses: %v", err)
	}
	
	rowsAffected, _ = result.RowsAffected()
	fmt.Printf("  ✓ Created %d enhanced canonical addresses\n", rowsAffected)
	
	fmt.Println("\n✓ Address data cleaning completed")
	return nil
}