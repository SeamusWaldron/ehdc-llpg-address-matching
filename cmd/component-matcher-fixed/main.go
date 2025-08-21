package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
	"github.com/ehdc-llpg/internal/config"
	"github.com/ehdc-llpg/internal/matcher"
)

const version = "1.0.0-component-matcher-fixed"

func main() {
	var (
		limit      = flag.Int("limit", 0, "Number of documents to process (0 = all)")
		batchSize  = flag.Int("batch-size", 1000, "Batch size for processing")
		debug      = flag.Bool("debug", false, "Enable debug output")
		configFile = flag.String("config", ".env", "Path to configuration file")
		workers    = flag.Int("workers", 4, "Number of parallel workers")
		reprocess  = flag.Bool("reprocess", false, "Reprocess existing matches")
	)
	flag.Parse()

	fmt.Printf("EHDC FIXED Component-Based Matcher v%s\n", version)
	fmt.Println("Processing addresses with FIXED component-based matching for maximum accuracy")
	fmt.Println("🔧 CRITICAL FIXES: Strict house number validation, proper business matching")
	fmt.Println()

	// Load configuration
	err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Connect to database
	db := connectDB()
	defer db.Close()

	// Clear existing component-based matches if reprocessing
	if *reprocess {
		fmt.Println("🧹 Clearing existing component-based matches...")
		result, err := db.Exec(`
			DELETE FROM address_match 
			WHERE match_method_id IN (
				SELECT method_id FROM dim_match_method 
				WHERE method_code IN ('exact_components', 'postcode_house', 'road_city_exact', 'road_city_fuzzy', 'fuzzy_road', 'component_fuzzy')
			)
		`)
		if err != nil {
			log.Fatalf("Failed to clear existing matches: %v", err)
		}
		deleted, _ := result.RowsAffected()
		fmt.Printf("🗑️  Deleted %d existing component-based matches\n\n", deleted)
	}

	// Get total document count
	var totalDocs int
	query := `SELECT COUNT(*) FROM src_document WHERE gopostal_processed = TRUE AND raw_address IS NOT NULL`
	if !*reprocess {
		query += ` AND document_id NOT IN (SELECT document_id FROM address_match)`
	}
	err = db.QueryRow(query).Scan(&totalDocs)
	if err != nil {
		log.Fatalf("Failed to count documents: %v", err)
	}

	if *limit > 0 && *limit < totalDocs {
		totalDocs = *limit
	}

	fmt.Printf("📊 Processing %d source documents with FIXED component-based matching\n", totalDocs)
	fmt.Printf("⚙️  Configuration: batch-size=%d, workers=%d, debug=%t, reprocess=%t\n", *batchSize, *workers, *debug, *reprocess)
	fmt.Println()

	// Create FIXED component engine
	engine := matcher.NewFixedComponentEngine(db)

	// Process documents
	startTime := time.Now()
	processed, matched, errors := processDocuments(db, engine, totalDocs, *batchSize, *debug, *reprocess)
	duration := time.Since(startTime)

	// Report results
	fmt.Printf("\n✅ FIXED COMPONENT MATCHING COMPLETE\n")
	fmt.Printf("=====================================\n")
	fmt.Printf("📊 Documents processed: %d\n", processed)
	fmt.Printf("🎯 Matches found: %d (%.1f%%)\n", matched, float64(matched)/float64(processed)*100)
	fmt.Printf("❌ Errors: %d\n", errors)
	fmt.Printf("⏱️  Total time: %v\n", duration)
	fmt.Printf("🚀 Processing rate: %.1f docs/sec\n", float64(processed)/duration.Seconds())
	fmt.Println()

	// Generate comparison with old algorithm
	generateComparisonStats(db)
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

func processDocuments(db *sql.DB, engine *matcher.FixedComponentEngine, totalDocs, batchSize int, debug, reprocess bool) (int, int, int) {
	var processed, matched, errors int

	query := `
		SELECT document_id, raw_address, raw_uprn 
		FROM src_document 
		WHERE gopostal_processed = TRUE 
		  AND raw_address IS NOT NULL 
		  AND raw_address != ''
	`
	if !reprocess {
		query += ` AND document_id NOT IN (SELECT document_id FROM address_match)`
	}
	query += ` ORDER BY document_id`

	if totalDocs > 0 {
		query += fmt.Sprintf(" LIMIT %d", totalDocs)
	}

	rows, err := db.Query(query)
	if err != nil {
		log.Fatalf("Failed to query documents: %v", err)
	}
	defer rows.Close()

	batch := 0
	batchStart := time.Now()

	for rows.Next() {
		var docID int64
		var address string
		var rawUPRN sql.NullString
		
		if err := rows.Scan(&docID, &address, &rawUPRN); err != nil {
			errors++
			continue
		}

		// Process document with FIXED engine
		input := matcher.MatchInput{
			DocumentID: docID,
			RawAddress: address,
		}
		
		// Add UPRN if present
		if rawUPRN.Valid && rawUPRN.String != "" {
			input.RawUPRN = &rawUPRN.String
		}

		result, err := engine.ProcessDocument(debug, input)
		if err != nil {
			if debug {
				fmt.Printf("❌ Error processing doc %d: %v\n", docID, err)
			}
			errors++
			continue
		}

		// Save result - the fixed engine uses different method codes
		if result.BestCandidate != nil {
			err = saveFixedMatchResult(db, result, debug)
			if err != nil {
				if debug {
					fmt.Printf("❌ Error saving result for doc %d: %v\n", docID, err)
				}
				errors++
				continue
			}
			matched++
		} else {
			// Save no-match result
			err = saveNoMatchResult(db, docID, debug)
			if err != nil && debug {
				fmt.Printf("❌ Error saving no-match for doc %d: %v\n", docID, err)
			}
		}

		processed++

		// Progress reporting
		if processed%batchSize == 0 {
			batch++
			batchDuration := time.Since(batchStart)
			fmt.Printf("  ✅ Batch %d: %d docs processed, %d matched (%.1f%%), %d errors, %.1f docs/sec\n",
				batch, processed, matched, float64(matched)/float64(processed)*100, errors,
				float64(batchSize)/batchDuration.Seconds())
			batchStart = time.Now()
		}
	}

	return processed, matched, errors
}

func saveFixedMatchResult(db *sql.DB, result *matcher.MatchResult, debug bool) error {
	_, err := db.Exec(`
		INSERT INTO address_match (
			document_id, address_id, location_id, match_method_id,
			confidence_score, match_status, matched_by, matched_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (document_id) DO UPDATE SET
			address_id = EXCLUDED.address_id,
			location_id = EXCLUDED.location_id,
			match_method_id = EXCLUDED.match_method_id,
			confidence_score = EXCLUDED.confidence_score,
			match_status = EXCLUDED.match_status,
			matched_by = EXCLUDED.matched_by,
			matched_at = now()
	`,
		result.DocumentID,
		result.BestCandidate.AddressID,
		result.BestCandidate.LocationID,
		result.BestCandidate.MethodID,
		result.BestCandidate.Score,
		result.MatchStatus,
		"system_fixed_component",
	)

	if debug && err == nil {
		fmt.Printf("✅ Saved FIXED match for doc %d -> address %d (score: %.4f, decision: %s)\n", 
			result.DocumentID, result.BestCandidate.AddressID, result.BestCandidate.Score, result.Decision)
	}

	return err
}

func saveNoMatchResult(db *sql.DB, documentID int64, debug bool) error {
	_, err := db.Exec(`
		INSERT INTO address_match (
			document_id, match_method_id, confidence_score, 
			match_status, matched_by, matched_at
		) VALUES ($1, 1, 0.0, 'auto', 'system_fixed_component', now())
		ON CONFLICT (document_id) DO UPDATE SET
			address_id = NULL,
			location_id = NULL,
			match_method_id = 1,
			confidence_score = 0.0,
			match_status = 'auto',
			matched_by = 'system_fixed_component',
			matched_at = now()
	`, documentID)

	if debug && err == nil {
		fmt.Printf("⚪ Saved no-match for doc %d\n", documentID)
	}

	return err
}

func generateComparisonStats(db *sql.DB) {
	fmt.Println("📈 FIXED ALGORITHM RESULTS SUMMARY")
	fmt.Println("===================================")

	// Decision breakdown
	rows, err := db.Query(`
		SELECT 
			CASE 
				WHEN matched_by = 'system_fixed_component' THEN decision || ' (FIXED)'
				ELSE decision || ' (OLD)'
			END as decision_type,
			COUNT(*) as count, 
			ROUND(AVG(confidence_score), 4) as avg_confidence
		FROM address_match 
		GROUP BY matched_by, decision
		ORDER BY matched_by DESC, count DESC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var decision string
			var count int
			var avgConf float64
			rows.Scan(&decision, &count, &avgConf)
			fmt.Printf("🎯 %-20s: %6d matches (avg confidence: %.4f)\n", decision, count, avgConf)
		}
	}

	fmt.Println()

	// House number validation check
	fmt.Println("🏠 HOUSE NUMBER VALIDATION CHECK")
	fmt.Println("=================================")
	
	// Check for problematic matches that should have been caught
	problemQuery := `
		SELECT COUNT(*) as problem_count
		FROM address_match am
		JOIN src_document sd ON am.document_id = sd.document_id
		JOIN dim_address da ON am.address_id = da.address_id
		WHERE am.matched_by = 'system_fixed_component'
		  AND sd.gopostal_house_number IS NOT NULL
		  AND da.gopostal_house_number IS NOT NULL
		  AND sd.gopostal_house_number != da.gopostal_house_number
		  AND am.confidence_score >= 0.8
	`
	
	var problemCount int
	err = db.QueryRow(problemQuery).Scan(&problemCount)
	if err == nil {
		if problemCount == 0 {
			fmt.Printf("✅ EXCELLENT: No high-confidence house number mismatches found\n")
		} else {
			fmt.Printf("⚠️  WARNING: %d high-confidence house number mismatches still detected\n", problemCount)
		}
	}

	fmt.Println()
	fmt.Println("💡 Fixed Algorithm Improvements:")
	fmt.Println("   • Strict house number validation prevents major mismatches")
	fmt.Println("   • Business name matching improves organization addresses")
	fmt.Println("   • Proper penalty scoring reduces false positives")
	fmt.Println("   • Enhanced validation rules ensure quality matches")
}