package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/ehdc-llpg/internal/config"
)

const version = "1.0.0-bulk-historic-uprns"

func main() {
	fmt.Printf("EHDC Bulk Historic UPRN Creator v%s\n", version)
	fmt.Println("Creating historic address records for all missing UPRNs in bulk")
	fmt.Println()

	// Load configuration
	err := config.LoadConfig(".env")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Connect to database
	db := connectDB()
	defer db.Close()

	startTime := time.Now()

	// Step 1: Find all missing UPRNs (normalized)
	fmt.Println("🔍 Step 1: Identifying missing UPRNs...")
	missingUPRNs, err := findMissingUPRNs(db)
	if err != nil {
		log.Fatalf("Failed to find missing UPRNs: %v", err)
	}

	fmt.Printf("   Found %d unique missing UPRNs\n", len(missingUPRNs))
	if len(missingUPRNs) == 0 {
		fmt.Println("✅ No missing UPRNs to process!")
		return
	}

	// Step 2: Bulk insert historic records
	fmt.Println("\n🏗️  Step 2: Creating historic address records...")
	created, err := bulkCreateHistoricUPRNs(db, missingUPRNs)
	if err != nil {
		log.Fatalf("Failed to create historic UPRNs: %v", err)
	}

	duration := time.Since(startTime)

	// Step 3: Report results
	fmt.Printf("\n✅ BULK HISTORIC UPRN CREATION COMPLETE\n")
	fmt.Printf("======================================\n")
	fmt.Printf("📊 Missing UPRNs identified: %d\n", len(missingUPRNs))
	fmt.Printf("🏗️  Historic records created: %d\n", created)
	fmt.Printf("⏱️  Total time: %v\n", duration)
	fmt.Printf("🚀 Processing rate: %.1f UPRNs/sec\n", float64(created)/duration.Seconds())

	// Verify results
	var totalHistoric int
	db.QueryRow("SELECT COUNT(*) FROM dim_address WHERE is_historic = TRUE").Scan(&totalHistoric)
	fmt.Printf("📈 Total historic records in system: %d\n", totalHistoric)
	
	fmt.Println("\n💡 Next Step: Run the matching process - all missing UPRNs are now pre-populated!")
}

type MissingUPRN struct {
	UPRN          string
	Address       string
	DocumentID    int64
	DocumentCount int
}

func findMissingUPRNs(db *sql.DB) ([]MissingUPRN, error) {
	// Find all UPRNs in source documents that don't exist in LLPG (with normalization)
	query := `
		SELECT 
			-- Normalize UPRN by removing .00 suffix
			CASE 
				WHEN sd.raw_uprn LIKE '%.00' THEN REPLACE(sd.raw_uprn, '.00', '')
				ELSE sd.raw_uprn 
			END as normalized_uprn,
			sd.raw_address,
			MIN(sd.document_id) as sample_document_id,
			COUNT(*) as document_count
		FROM src_document sd
		WHERE sd.raw_uprn IS NOT NULL 
		  AND sd.raw_uprn <> ''
		  AND NOT EXISTS(
			  SELECT 1 FROM dim_address da 
			  WHERE da.uprn = CASE 
				  WHEN sd.raw_uprn LIKE '%.00' THEN REPLACE(sd.raw_uprn, '.00', '')
				  ELSE sd.raw_uprn 
			  END
			  AND da.is_historic = FALSE
		  )
		GROUP BY normalized_uprn, sd.raw_address
		ORDER BY document_count DESC, normalized_uprn
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var missing []MissingUPRN
	for rows.Next() {
		var m MissingUPRN
		err := rows.Scan(&m.UPRN, &m.Address, &m.DocumentID, &m.DocumentCount)
		if err != nil {
			continue
		}
		missing = append(missing, m)
	}

	return missing, nil
}

func bulkCreateHistoricUPRNs(db *sql.DB, missingUPRNs []MissingUPRN) (int, error) {
	if len(missingUPRNs) == 0 {
		return 0, nil
	}

	// Use transaction for bulk operation
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	created := 0
	batchSize := 100
	
	for i := 0; i < len(missingUPRNs); i += batchSize {
		end := i + batchSize
		if end > len(missingUPRNs) {
			end = len(missingUPRNs)
		}
		
		batch := missingUPRNs[i:end]
		batchCreated, err := createHistoricBatch(tx, batch)
		if err != nil {
			return created, fmt.Errorf("batch %d-%d failed: %w", i, end-1, err)
		}
		
		created += batchCreated
		
		if (i+batchSize)%1000 == 0 {
			fmt.Printf("   Processed %d/%d UPRNs...\n", i+batchSize, len(missingUPRNs))
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return created, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return created, nil
}

func createHistoricBatch(tx *sql.Tx, batch []MissingUPRN) (int, error) {
	if len(batch) == 0 {
		return 0, nil
	}

	// Build bulk insert for locations (all historic records get 0,0 coordinates)
	var locationValues []string
	var locationArgs []interface{}
	for i := range batch {
		locationValues = append(locationValues, fmt.Sprintf("($%d, $%d, $%d, $%d)", 
			i*4+1, i*4+2, i*4+3, i*4+4))
		locationArgs = append(locationArgs, 0, 0, 0, 0) // easting, northing, lat, lng
	}

	locationQuery := fmt.Sprintf(`
		INSERT INTO dim_location (easting, northing, latitude, longitude)
		VALUES %s
		RETURNING location_id
	`, strings.Join(locationValues, ", "))

	locationRows, err := tx.Query(locationQuery, locationArgs...)
	if err != nil {
		return 0, fmt.Errorf("failed to create locations: %w", err)
	}
	defer locationRows.Close()

	// Collect location IDs
	var locationIDs []int
	for locationRows.Next() {
		var locationID int
		if err := locationRows.Scan(&locationID); err != nil {
			return 0, fmt.Errorf("failed to scan location ID: %w", err)
		}
		locationIDs = append(locationIDs, locationID)
	}

	if len(locationIDs) != len(batch) {
		return 0, fmt.Errorf("location count mismatch: expected %d, got %d", len(batch), len(locationIDs))
	}

	// Build bulk insert for addresses
	var addressValues []string
	var addressArgs []interface{}
	argIndex := 1

	for i, missing := range batch {
		addressValues = append(addressValues, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, NOW())",
			argIndex, argIndex+1, argIndex+2, argIndex+3, argIndex+4, argIndex+5, argIndex+6, argIndex+7))
		
		// Simple canonicalization - lowercase and remove special chars
		canonical := strings.ToLower(missing.Address)
		canonical = strings.ReplaceAll(canonical, ",", " ")
		canonical = strings.ReplaceAll(canonical, ".", "")
		canonical = strings.Join(strings.Fields(canonical), " ") // normalize whitespace
		
		addressArgs = append(addressArgs,
			locationIDs[i],    // location_id
			missing.UPRN,      // uprn
			missing.Address,   // full_address
			canonical,         // address_canonical
			true,              // is_historic
			true,              // created_from_source
			missing.DocumentID, // source_document_id
			time.Now(),        // historic_created_at
		)
		argIndex += 8
	}

	addressQuery := fmt.Sprintf(`
		INSERT INTO dim_address (
			location_id, uprn, full_address, address_canonical,
			is_historic, created_from_source, source_document_id, historic_created_at, created_at
		) VALUES %s
	`, strings.Join(addressValues, ", "))

	_, err = tx.Exec(addressQuery, addressArgs...)
	if err != nil {
		return 0, fmt.Errorf("failed to create addresses: %w", err)
	}

	return len(batch), nil
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