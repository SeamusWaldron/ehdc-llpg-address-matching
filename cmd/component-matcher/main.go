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

const version = "1.0.0-component-matcher"

func main() {
	var (
		limit      = flag.Int("limit", 0, "Number of documents to process (0 = all)")
		batchSize  = flag.Int("batch-size", 1000, "Batch size for processing")
		debug      = flag.Bool("debug", false, "Enable debug output")
		configFile = flag.String("config", ".env", "Path to configuration file")
		workers    = flag.Int("workers", 4, "Number of parallel workers")
	)
	flag.Parse()

	fmt.Printf("EHDC Component-Based Matcher v%s\n", version)
	fmt.Println("Processing addresses with component-based matching for maximum accuracy")
	fmt.Println()

	// Load configuration
	err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Connect to database
	db := connectDB()
	defer db.Close()

	// Get total document count
	var totalDocs int
	query := `SELECT COUNT(*) FROM src_document WHERE gopostal_processed = TRUE AND raw_address IS NOT NULL`
	err = db.QueryRow(query).Scan(&totalDocs)
	if err != nil {
		log.Fatalf("Failed to count documents: %v", err)
	}

	if *limit > 0 && *limit < totalDocs {
		totalDocs = *limit
	}

	fmt.Printf("📊 Processing %d source documents with component-based matching\n", totalDocs)
	fmt.Printf("⚙️  Configuration: batch-size=%d, workers=%d, debug=%t\n", *batchSize, *workers, *debug)
	fmt.Println()

	// Create component engine
	engine := matcher.NewComponentEngine(db)

	// Process documents
	startTime := time.Now()
	processed, matched, errors := processDocuments(db, engine, totalDocs, *batchSize, *debug)
	duration := time.Since(startTime)

	// Report results
	fmt.Printf("\n✅ COMPONENT MATCHING COMPLETE\n")
	fmt.Printf("===============================\n")
	fmt.Printf("📊 Documents processed: %d\n", processed)
	fmt.Printf("🎯 Matches found: %d (%.1f%%)\n", matched, float64(matched)/float64(processed)*100)
	fmt.Printf("❌ Errors: %d\n", errors)
	fmt.Printf("⏱️  Total time: %v\n", duration)
	fmt.Printf("🚀 Processing rate: %.1f docs/sec\n", float64(processed)/duration.Seconds())
	fmt.Println()

	// Generate summary statistics
	generateSummaryStats(db)
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

func processDocuments(db *sql.DB, engine *matcher.ComponentEngine, totalDocs, batchSize int, debug bool) (int, int, int) {
	var processed, matched, errors int

	query := `
		SELECT document_id, raw_address 
		FROM src_document 
		WHERE gopostal_processed = TRUE 
		  AND raw_address IS NOT NULL 
		  AND raw_address != ''
		ORDER BY document_id
	`
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
		
		if err := rows.Scan(&docID, &address); err != nil {
			errors++
			continue
		}

		// Process document
		input := matcher.MatchInput{
			DocumentID: docID,
			RawAddress: address,
		}

		result, err := engine.ProcessDocument(false, input) // Set debug=false for performance
		if err != nil {
			if debug {
				fmt.Printf("❌ Error processing doc %d: %v\n", docID, err)
			}
			errors++
			continue
		}

		// Save result if we found a match
		if result.BestCandidate != nil {
			err = engine.SaveMatchResult(false, result)
			if err != nil {
				if debug {
					fmt.Printf("❌ Error saving result for doc %d: %v\n", docID, err)
				}
				errors++
				continue
			}
			matched++
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

func generateSummaryStats(db *sql.DB) {
	fmt.Println("📈 MATCHING RESULTS SUMMARY")
	fmt.Println("===========================")

	// Decision breakdown
	rows, err := db.Query(`
		SELECT decision, COUNT(*) as count, 
		       ROUND(AVG(confidence_score), 4) as avg_confidence
		FROM address_match 
		GROUP BY decision 
		ORDER BY count DESC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var decision string
			var count int
			var avgConf float64
			rows.Scan(&decision, &count, &avgConf)
			fmt.Printf("🎯 %-15s: %6d matches (avg confidence: %.4f)\n", decision, count, avgConf)
		}
	}

	fmt.Println()

	// Method breakdown
	rows, err = db.Query(`
		SELECT mm.method_code, COUNT(am.*) as count,
		       ROUND(AVG(am.confidence_score), 4) as avg_confidence
		FROM match_method mm
		LEFT JOIN address_match am ON mm.method_id = am.match_method_id
		WHERE mm.method_code IN ('exact_components', 'postcode_house', 'road_city_exact', 'road_city_fuzzy', 'fuzzy_road')
		GROUP BY mm.method_code, mm.method_id
		HAVING COUNT(am.*) > 0
		ORDER BY count DESC
	`)
	if err == nil {
		defer rows.Close()
		fmt.Println("🔧 MATCHING METHODS PERFORMANCE")
		fmt.Println("==============================")
		for rows.Next() {
			var method string
			var count int
			var avgConf float64
			rows.Scan(&method, &count, &avgConf)
			fmt.Printf("⚙️  %-20s: %6d matches (avg confidence: %.4f)\n", method, count, avgConf)
		}
	}

	fmt.Println()
	fmt.Println("💡 Next Steps:")
	fmt.Println("   • Review 'needs_review' matches manually")
	fmt.Println("   • Generate full accuracy report: psql -f generate_accuracy_report.sql")
	fmt.Println("   • Export results for stakeholder review")
}