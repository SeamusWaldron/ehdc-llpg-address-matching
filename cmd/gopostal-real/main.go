package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
	postal "github.com/openvenues/gopostal/parser"
	"github.com/ehdc-llpg/internal/config"
)

const version = "1.0.0-real-gopostal"

func main() {
	var (
		command    = flag.String("cmd", "", "Command: test-parse, preprocess-llpg, preprocess-source, preprocess-all, stats")
		address    = flag.String("address", "", "Single address to test parsing")
		limit      = flag.Int("limit", 1000, "Number of records to process (0 = all)")
		configFile = flag.String("config", ".env", "Path to configuration file")
	)
	flag.Parse()

	if *command == "" {
		printUsage()
		return
	}

	fmt.Printf("EHDC Real gopostal Preprocessor v%s\n", version)
	fmt.Println("Using libpostal for maximum UK address parsing accuracy")
	fmt.Println()

	// Load configuration
	err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	switch *command {
	case "test-parse":
		if *address == "" {
			fmt.Println("Error: -address required for test-parse")
			return
		}
		testRealParse(*address)
	case "preprocess-llpg":
		db := connectDB()
		defer db.Close()
		err = preprocessLLPGReal(db, *limit)
	case "preprocess-source":
		db := connectDB()
		defer db.Close()
		err = preprocessSourceReal(db, *limit)
	case "preprocess-all":
		db := connectDB()
		defer db.Close()
		err = preprocessAll(db, *limit)
	case "stats":
		db := connectDB()
		defer db.Close()
		showStatsReal(db)
	default:
		fmt.Printf("Unknown command: %s\n", *command)
		printUsage()
	}

	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  Test real gopostal parsing:")
	fmt.Println("    ./gopostal-real -cmd=test-parse -address=\"Flat 3, 123 High St, Nr Alton, Hants, GU34 1AB\"")
	fmt.Println()
	fmt.Println("  Pre-process LLPG addresses:")
	fmt.Println("    ./gopostal-real -cmd=preprocess-llpg -limit=1000")
	fmt.Println()
	fmt.Println("  Pre-process source documents:")
	fmt.Println("    ./gopostal-real -cmd=preprocess-source -limit=1000")
	fmt.Println()
	fmt.Println("  Pre-process everything:")
	fmt.Println("    ./gopostal-real -cmd=preprocess-all -limit=0")
	fmt.Println()
	fmt.Println("  Show statistics:")
	fmt.Println("    ./gopostal-real -cmd=stats")
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
	return db
}

// testRealParse tests real gopostal parsing
func testRealParse(address string) {
	fmt.Printf("🔍 Testing REAL gopostal parsing:\n")
	fmt.Printf("   Input: %s\n\n", address)

	// Parse with real gopostal
	components := postal.ParseAddress(address)

	fmt.Println("📋 Real gopostal Components:")
	for _, component := range components {
		fmt.Printf("   %-15s: %s\n", component.Label, component.Value)
	}

	fmt.Println("\n🎯 Extracted for matching:")
	extracted := extractComponents(components)
	if extracted["house_number"] != "" {
		fmt.Printf("   House Number: %s\n", extracted["house_number"])
	}
	if extracted["house"] != "" {
		fmt.Printf("   House Name:   %s\n", extracted["house"])
	}
	if extracted["road"] != "" {
		fmt.Printf("   Road:         %s\n", extracted["road"])
	}
	if extracted["suburb"] != "" {
		fmt.Printf("   Suburb:       %s\n", extracted["suburb"])
	}
	if extracted["city"] != "" {
		fmt.Printf("   City:         %s\n", extracted["city"])
	}
	if extracted["state"] != "" {
		fmt.Printf("   State:        %s\n", extracted["state"])
	}
	if extracted["postcode"] != "" {
		fmt.Printf("   Postcode:     %s\n", extracted["postcode"])
	}
	if extracted["unit"] != "" {
		fmt.Printf("   Unit:         %s\n", extracted["unit"])
	}
}

// extractComponents converts gopostal output to our component structure
func extractComponents(components []postal.ParsedComponent) map[string]string {
	extracted := make(map[string]string)
	
	for _, comp := range components {
		switch comp.Label {
		case "house_number":
			extracted["house_number"] = comp.Value
		case "house":
			extracted["house"] = comp.Value
		case "road":
			extracted["road"] = comp.Value
		case "suburb":
			extracted["suburb"] = comp.Value
		case "city":
			extracted["city"] = comp.Value
		case "state_district":
			extracted["state_district"] = comp.Value
		case "state":
			extracted["state"] = comp.Value
		case "postcode":
			extracted["postcode"] = comp.Value
		case "country":
			extracted["country"] = comp.Value
		case "unit":
			extracted["unit"] = comp.Value
		case "level":
			extracted["level"] = comp.Value
		case "staircase":
			extracted["staircase"] = comp.Value
		case "entrance":
			extracted["entrance"] = comp.Value
		case "po_box":
			extracted["po_box"] = comp.Value
		}
	}
	
	return extracted
}

// preprocessLLPGReal processes LLPG addresses with real gopostal
func preprocessLLPGReal(db *sql.DB, limit int) error {
	fmt.Println("📍 Pre-processing LLPG addresses with REAL gopostal...")
	startTime := time.Now()

	// Get unprocessed addresses
	query := `
		SELECT address_id, full_address 
		FROM dim_address 
		WHERE gopostal_processed = FALSE
		ORDER BY address_id
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	processed := 0
	errors := 0
	
	for rows.Next() {
		var id int
		var address string
		if err := rows.Scan(&id, &address); err != nil {
			errors++
			continue
		}

		// Parse with REAL gopostal
		components := postal.ParseAddress(address)
		extracted := extractComponents(components)

		// Update database with all components
		_, err = db.Exec(`
			UPDATE dim_address SET
				gopostal_house = $2,
				gopostal_house_number = $3,
				gopostal_road = $4,
				gopostal_suburb = $5,
				gopostal_city = $6,
				gopostal_state_district = $7,
				gopostal_state = $8,
				gopostal_postcode = $9,
				gopostal_country = $10,
				gopostal_unit = $11,
				gopostal_level = $12,
				gopostal_staircase = $13,
				gopostal_entrance = $14,
				gopostal_po_box = $15,
				gopostal_processed = TRUE
			WHERE address_id = $1
		`, id, 
			nullIfEmpty(extracted["house"]),
			nullIfEmpty(extracted["house_number"]),
			nullIfEmpty(extracted["road"]),
			nullIfEmpty(extracted["suburb"]),
			nullIfEmpty(extracted["city"]),
			nullIfEmpty(extracted["state_district"]),
			nullIfEmpty(extracted["state"]),
			nullIfEmpty(extracted["postcode"]),
			nullIfEmpty(extracted["country"]),
			nullIfEmpty(extracted["unit"]),
			nullIfEmpty(extracted["level"]),
			nullIfEmpty(extracted["staircase"]),
			nullIfEmpty(extracted["entrance"]),
			nullIfEmpty(extracted["po_box"]))

		if err != nil {
			fmt.Printf("  ❌ Error updating address %d: %v\n", id, err)
			errors++
			continue
		}

		processed++
		if processed%100 == 0 {
			fmt.Printf("  ✅ Processed %d/%d LLPG addresses...\n", processed, processed+errors)
		}
	}

	// Record statistics
	duration := time.Since(startTime)
	_, err = db.Exec(`
		INSERT INTO gopostal_processing_stats 
		(table_name, total_records, processed_records, processing_time, notes)
		VALUES ('dim_address', $1, $2, $3, $4)
	`, processed+errors, processed, duration, fmt.Sprintf("Real gopostal batch of %d", limit))

	fmt.Printf("\n✅ LLPG Processing Complete:\n")
	fmt.Printf("   Processed: %d addresses\n", processed)
	fmt.Printf("   Errors:    %d\n", errors)
	fmt.Printf("   Duration:  %v\n", duration)
	fmt.Printf("   Rate:      %.1f addresses/sec\n", float64(processed)/duration.Seconds())

	return nil
}

// preprocessSourceReal processes source documents with real gopostal
func preprocessSourceReal(db *sql.DB, limit int) error {
	fmt.Println("📄 Pre-processing source documents with REAL gopostal...")
	startTime := time.Now()

	// Get unprocessed addresses
	query := `
		SELECT document_id, raw_address 
		FROM src_document 
		WHERE gopostal_processed = FALSE
			AND raw_address IS NOT NULL
			AND raw_address != ''
		ORDER BY document_id
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	processed := 0
	errors := 0
	
	for rows.Next() {
		var id int
		var address string
		if err := rows.Scan(&id, &address); err != nil {
			errors++
			continue
		}

		// Parse with REAL gopostal
		components := postal.ParseAddress(address)
		extracted := extractComponents(components)

		// Update database
		_, err = db.Exec(`
			UPDATE src_document SET
				gopostal_house = $2,
				gopostal_house_number = $3,
				gopostal_road = $4,
				gopostal_suburb = $5,
				gopostal_city = $6,
				gopostal_state_district = $7,
				gopostal_state = $8,
				gopostal_postcode = $9,
				gopostal_country = $10,
				gopostal_unit = $11,
				gopostal_level = $12,
				gopostal_staircase = $13,
				gopostal_entrance = $14,
				gopostal_po_box = $15,
				gopostal_processed = TRUE
			WHERE document_id = $1
		`, id,
			nullIfEmpty(extracted["house"]),
			nullIfEmpty(extracted["house_number"]),
			nullIfEmpty(extracted["road"]),
			nullIfEmpty(extracted["suburb"]),
			nullIfEmpty(extracted["city"]),
			nullIfEmpty(extracted["state_district"]),
			nullIfEmpty(extracted["state"]),
			nullIfEmpty(extracted["postcode"]),
			nullIfEmpty(extracted["country"]),
			nullIfEmpty(extracted["unit"]),
			nullIfEmpty(extracted["level"]),
			nullIfEmpty(extracted["staircase"]),
			nullIfEmpty(extracted["entrance"]),
			nullIfEmpty(extracted["po_box"]))

		if err != nil {
			fmt.Printf("  ❌ Error updating document %d: %v\n", id, err)
			errors++
			continue
		}

		processed++
		if processed%100 == 0 {
			fmt.Printf("  ✅ Processed %d/%d source documents...\n", processed, processed+errors)
		}
	}

	// Record statistics
	duration := time.Since(startTime)
	_, err = db.Exec(`
		INSERT INTO gopostal_processing_stats 
		(table_name, total_records, processed_records, processing_time, notes)
		VALUES ('src_document', $1, $2, $3, $4)
	`, processed+errors, processed, duration, fmt.Sprintf("Real gopostal batch of %d", limit))

	fmt.Printf("\n✅ Source Document Processing Complete:\n")
	fmt.Printf("   Processed: %d documents\n", processed)
	fmt.Printf("   Errors:    %d\n", errors)
	fmt.Printf("   Duration:  %v\n", duration)
	fmt.Printf("   Rate:      %.1f documents/sec\n", float64(processed)/duration.Seconds())

	return nil
}

// preprocessAll processes both LLPG and source documents
func preprocessAll(db *sql.DB, limit int) error {
	fmt.Println("🚀 Pre-processing ALL addresses with REAL gopostal...")
	fmt.Println("   This will take some time but dramatically improve matching accuracy!")
	fmt.Println()

	// Process LLPG first
	fmt.Println("STEP 1: Processing LLPG addresses...")
	if err := preprocessLLPGReal(db, limit); err != nil {
		return fmt.Errorf("LLPG processing failed: %w", err)
	}

	fmt.Println("\nSTEP 2: Processing source documents...")
	if err := preprocessSourceReal(db, limit); err != nil {
		return fmt.Errorf("Source processing failed: %w", err)
	}

	fmt.Println("\n🎉 ALL PREPROCESSING COMPLETE!")
	fmt.Println("   Ready for component-based matching with maximum accuracy")

	return nil
}

// showStatsReal shows real gopostal processing statistics
func showStatsReal(db *sql.DB) {
	fmt.Println("📊 Real gopostal Processing Statistics")
	fmt.Println("=====================================")

	// LLPG statistics
	var llpgTotal, llpgProcessed int
	db.QueryRow("SELECT COUNT(*) FROM dim_address").Scan(&llpgTotal)
	db.QueryRow("SELECT COUNT(*) FROM dim_address WHERE gopostal_processed = TRUE").Scan(&llpgProcessed)

	fmt.Printf("\n📍 LLPG Addresses:\n")
	fmt.Printf("   Total:     %d\n", llpgTotal)
	fmt.Printf("   Processed: %d (%.1f%%)\n", llpgProcessed, float64(llpgProcessed)/float64(llpgTotal)*100)
	fmt.Printf("   Remaining: %d\n", llpgTotal-llpgProcessed)

	// Source document statistics
	var srcTotal, srcProcessed int
	db.QueryRow("SELECT COUNT(*) FROM src_document WHERE raw_address IS NOT NULL").Scan(&srcTotal)
	db.QueryRow("SELECT COUNT(*) FROM src_document WHERE gopostal_processed = TRUE").Scan(&srcProcessed)

	fmt.Printf("\n📄 Source Documents:\n")
	fmt.Printf("   Total:     %d\n", srcTotal)
	fmt.Printf("   Processed: %d (%.1f%%)\n", srcProcessed, float64(srcProcessed)/float64(srcTotal)*100)
	fmt.Printf("   Remaining: %d\n", srcTotal-srcProcessed)

	// Processing performance
	fmt.Println("\n⚡ Processing Performance:")
	rows, _ := db.Query(`
		SELECT table_name, 
		       AVG(processed_records::NUMERIC / EXTRACT(EPOCH FROM processing_time)) as avg_rate,
		       COUNT(*) as batches
		FROM gopostal_processing_stats
		WHERE notes LIKE '%Real gopostal%'
		GROUP BY table_name
	`)
	defer rows.Close()

	for rows.Next() {
		var tableName string
		var avgRate float64
		var batches int
		rows.Scan(&tableName, &avgRate, &batches)
		fmt.Printf("   %s: %.1f records/sec (%d batches)\n", tableName, avgRate, batches)
	}

	// Sample parsed data
	fmt.Println("\n🔍 Sample REAL gopostal Components:")
	rows, _ = db.Query(`
		SELECT full_address, gopostal_house_number, gopostal_road, gopostal_city, 
		       gopostal_postcode, gopostal_unit
		FROM dim_address
		WHERE gopostal_processed = TRUE
		  AND (gopostal_house_number IS NOT NULL OR gopostal_road IS NOT NULL)
		LIMIT 5
	`)
	defer rows.Close()

	for rows.Next() {
		var addr string
		var houseNum, road, city, postcode, unit sql.NullString
		rows.Scan(&addr, &houseNum, &road, &city, &postcode, &unit)
		
		fmt.Printf("\n   Original: %s\n", addr)
		fmt.Printf("   → Components: ")
		parts := []string{}
		if unit.Valid && unit.String != "" {
			parts = append(parts, fmt.Sprintf("unit=%s", unit.String))
		}
		if houseNum.Valid && houseNum.String != "" {
			parts = append(parts, fmt.Sprintf("house#=%s", houseNum.String))
		}
		if road.Valid && road.String != "" {
			parts = append(parts, fmt.Sprintf("road=%s", road.String))
		}
		if city.Valid && city.String != "" {
			parts = append(parts, fmt.Sprintf("city=%s", city.String))
		}
		if postcode.Valid && postcode.String != "" {
			parts = append(parts, fmt.Sprintf("postcode=%s", postcode.String))
		}
		fmt.Printf("%s\n", strings.Join(parts, ", "))
	}

	totalProcessed := llpgProcessed + srcProcessed
	totalRecords := llpgTotal + srcTotal
	
	fmt.Printf("\n🎯 Overall Progress:\n")
	fmt.Printf("   Total processed: %d/%d (%.1f%%)\n", 
		totalProcessed, totalRecords, float64(totalProcessed)/float64(totalRecords)*100)
	
	if totalProcessed == totalRecords {
		fmt.Println("\n🎉 ALL ADDRESSES PROCESSED!")
		fmt.Println("   Ready for maximum accuracy component-based matching!")
	} else {
		fmt.Printf("\n📋 To process remaining %d addresses:\n", totalRecords-totalProcessed)
		fmt.Println("   ./gopostal-real -cmd=preprocess-all -limit=0")
	}
}

// nullIfEmpty returns nil if string is empty, otherwise the string
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}