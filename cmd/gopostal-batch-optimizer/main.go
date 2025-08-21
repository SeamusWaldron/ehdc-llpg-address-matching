package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
	postal "github.com/openvenues/gopostal/parser"
	"github.com/ehdc-llpg/internal/config"
)

const version = "1.0.0-gopostal-batch-optimizer"

func main() {
	fmt.Printf("EHDC Optimized Gopostal Batch Processor v%s\n", version)
	fmt.Println("Processing only unique unprocessed addresses for maximum efficiency")
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

	// Step 1: Analyze current state
	fmt.Println("🔍 Step 1: Analyzing current processing state...")
	srcStats, err := analyzeSourceProcessingState(db)
	if err != nil {
		log.Fatalf("Failed to analyze source processing state: %v", err)
	}
	
	llpgStats, err := analyzeLLPGProcessingState(db)
	if err != nil {
		log.Fatalf("Failed to analyze LLPG processing state: %v", err)
	}

	fmt.Printf("   📊 SOURCE DOCUMENTS Processing Statistics:\n")
	fmt.Printf("      Total records: %d\n", srcStats.TotalRecords)
	fmt.Printf("      Already processed: %d (%.1f%%)\n", srcStats.AlreadyProcessed, 
		float64(srcStats.AlreadyProcessed)/float64(srcStats.TotalRecords)*100)
	fmt.Printf("      Unique addresses: %d\n", srcStats.UniqueAddresses)
	fmt.Printf("      Unprocessed unique addresses: %d\n", srcStats.UnprocessedUniqueAddresses)
	
	fmt.Printf("   📊 LLPG ADDRESSES Processing Statistics:\n")
	fmt.Printf("      Total addresses: %d\n", llpgStats.TotalRecords)
	fmt.Printf("      Already processed: %d (%.1f%%)\n", llpgStats.AlreadyProcessed, 
		float64(llpgStats.AlreadyProcessed)/float64(llpgStats.TotalRecords)*100)
	fmt.Printf("      Unprocessed (historic records): %d\n", llpgStats.TotalRecords - llpgStats.AlreadyProcessed)

	totalToProcess := srcStats.UnprocessedUniqueAddresses + (llpgStats.TotalRecords - llpgStats.AlreadyProcessed)
	if totalToProcess == 0 {
		fmt.Println("✅ All addresses already processed!")
		return
	}

	fmt.Printf("   🎯 TOTAL TO PROCESS: %d addresses\n", totalToProcess)
	fmt.Printf("      Source unique addresses: %d\n", srcStats.UnprocessedUniqueAddresses)
	fmt.Printf("      LLPG historic addresses: %d\n", llpgStats.TotalRecords - llpgStats.AlreadyProcessed)

	// Step 2: Get unique unprocessed source addresses
	fmt.Println("\n📝 Step 2: Retrieving unique unprocessed source addresses...")
	uniqueSrcAddresses, err := getUniqueUnprocessedAddresses(db)
	if err != nil {
		log.Fatalf("Failed to get unique source addresses: %v", err)
	}
	fmt.Printf("   Found %d unique source addresses to process\n", len(uniqueSrcAddresses))

	// Step 3: Get unprocessed LLPG addresses (historic records)
	fmt.Println("\n📝 Step 3: Retrieving unprocessed LLPG addresses...")
	llpgAddresses, err := getUnprocessedLLPGAddresses(db)
	if err != nil {
		log.Fatalf("Failed to get LLPG addresses: %v", err)
	}
	fmt.Printf("   Found %d LLPG addresses to process\n", len(llpgAddresses))

	// Step 4: Process source addresses with gopostal
	fmt.Println("\n🔄 Step 4: Processing source addresses with gopostal...")
	srcProcessed, srcSkipped := processUniqueAddresses(uniqueSrcAddresses)

	// Step 5: Process LLPG addresses with gopostal
	fmt.Println("\n🔄 Step 5: Processing LLPG addresses with gopostal...")
	llpgProcessed, llpgSkipped := processLLPGAddresses(llpgAddresses)

	// Step 6: Batch update source document records
	fmt.Println("\n🔄 Step 6: Batch updating source document records...")
	srcUpdated, err := batchUpdateMatchingRecords(db, srcProcessed)
	if err != nil {
		log.Fatalf("Failed to batch update source records: %v", err)
	}

	// Step 7: Batch update LLPG records
	fmt.Println("\n🔄 Step 7: Batch updating LLPG records...")
	llpgUpdated, err := batchUpdateLLPGRecords(db, llpgProcessed)
	if err != nil {
		log.Fatalf("Failed to batch update LLPG records: %v", err)
	}

	duration := time.Since(startTime)

	// Step 8: Report results
	fmt.Printf("\n✅ OPTIMIZED GOPOSTAL PROCESSING COMPLETE\n")
	fmt.Printf("==========================================\n")
	fmt.Printf("📊 SOURCE DOCUMENTS:\n")
	fmt.Printf("    Unique addresses processed: %d\n", len(srcProcessed))
	fmt.Printf("    Special cases skipped: %d\n", srcSkipped)
	fmt.Printf("    Records updated: %d\n", srcUpdated)
	fmt.Printf("📊 LLPG ADDRESSES:\n")
	fmt.Printf("    Historic addresses processed: %d\n", len(llpgProcessed))
	fmt.Printf("    Addresses skipped: %d\n", llpgSkipped)
	fmt.Printf("    Records updated: %d\n", llpgUpdated)
	fmt.Printf("⏱️  Total time: %v\n", duration)
	fmt.Printf("🚀 Processing rate: %.1f addresses/sec\n", float64(len(srcProcessed)+len(llpgProcessed))/duration.Seconds())

	// Final verification
	var finalSrcProcessedCount, finalLLPGProcessedCount int
	db.QueryRow("SELECT COUNT(*) FROM src_document WHERE gopostal_processed = TRUE").Scan(&finalSrcProcessedCount)
	db.QueryRow("SELECT COUNT(*) FROM dim_address WHERE gopostal_processed = TRUE").Scan(&finalLLPGProcessedCount)
	
	fmt.Printf("📋 FINAL RESULTS:\n")
	fmt.Printf("    Source documents processed: %d/%d (%.1f%%)\n", 
		finalSrcProcessedCount, srcStats.TotalRecords, 
		float64(finalSrcProcessedCount)/float64(srcStats.TotalRecords)*100)
	fmt.Printf("    LLPG addresses processed: %d/%d (%.1f%%)\n", 
		finalLLPGProcessedCount, llpgStats.TotalRecords, 
		float64(finalLLPGProcessedCount)/float64(llpgStats.TotalRecords)*100)
}

type ProcessingStats struct {
	TotalRecords                int
	AlreadyProcessed           int
	UniqueAddresses            int
	UnprocessedUniqueAddresses int
}

type UniqueAddress struct {
	Address      string
	RecordCount  int
	DocumentIDs  []int64
}

type LLPGAddress struct {
	AddressID   int64
	Address     string
	UPRN        string
	IsHistoric  bool
}

type ProcessedAddress struct {
	Address              string
	Components          map[string]string
	RecordCount         int
	ProcessingSuccessful bool
}

func analyzeSourceProcessingState(db *sql.DB) (*ProcessingStats, error) {
	stats := &ProcessingStats{}

	// Get total records
	err := db.QueryRow("SELECT COUNT(*) FROM src_document WHERE raw_address IS NOT NULL AND raw_address <> ''").Scan(&stats.TotalRecords)
	if err != nil {
		return nil, err
	}

	// Get already processed
	err = db.QueryRow("SELECT COUNT(*) FROM src_document WHERE gopostal_processed = TRUE").Scan(&stats.AlreadyProcessed)
	if err != nil {
		return nil, err
	}

	// Get unique addresses
	err = db.QueryRow("SELECT COUNT(DISTINCT raw_address) FROM src_document WHERE raw_address IS NOT NULL AND raw_address <> ''").Scan(&stats.UniqueAddresses)
	if err != nil {
		return nil, err
	}

	// Get unprocessed unique addresses (addresses that have at least one unprocessed record)
	err = db.QueryRow(`
		SELECT COUNT(DISTINCT raw_address) 
		FROM src_document 
		WHERE raw_address IS NOT NULL 
		  AND raw_address <> ''
		  AND gopostal_processed = FALSE
	`).Scan(&stats.UnprocessedUniqueAddresses)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func getUniqueUnprocessedAddresses(db *sql.DB) ([]UniqueAddress, error) {
	query := `
		SELECT 
			raw_address,
			COUNT(*) as record_count
		FROM src_document
		WHERE raw_address IS NOT NULL 
		  AND raw_address <> ''
		  AND gopostal_processed = FALSE
		GROUP BY raw_address
		ORDER BY record_count DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var addresses []UniqueAddress
	for rows.Next() {
		var addr UniqueAddress
		err := rows.Scan(&addr.Address, &addr.RecordCount)
		if err != nil {
			continue
		}
		addresses = append(addresses, addr)
	}

	return addresses, nil
}

func processUniqueAddresses(addresses []UniqueAddress) ([]ProcessedAddress, int) {
	var processed []ProcessedAddress
	var skipped int

	for i, addr := range addresses {
		// Progress reporting
		if i%1000 == 0 && i > 0 {
			fmt.Printf("   Processed %d/%d addresses...\n", i, len(addresses))
		}

		// Skip special cases
		if shouldSkipAddress(addr.Address) {
			skipped++
			continue
		}

		// Process with gopostal
		components, success := processAddressWithGopostal(addr.Address)
		
		processed = append(processed, ProcessedAddress{
			Address:              addr.Address,
			Components:          components,
			RecordCount:         addr.RecordCount,
			ProcessingSuccessful: success,
		})
	}

	return processed, skipped
}

func shouldSkipAddress(address string) bool {
	address = strings.TrimSpace(strings.ToUpper(address))
	
	// Skip "N/A" and similar
	skipValues := []string{"N/A", "NOT APPLICABLE", "NONE", "NULL", ""}
	for _, skip := range skipValues {
		if address == skip {
			return true
		}
	}

	// Skip planning references (format like F23650-8, AB-JG-20437)
	if len(address) < 50 && (strings.Contains(address, "-") || strings.HasPrefix(address, "F")) {
		// Simple heuristic: if it's short and has dashes or starts with F, likely a planning ref
		if !strings.Contains(address, " ") || strings.Count(address, " ") <= 1 {
			return true
		}
	}

	// Skip map references (like "1", "10B", "45A")
	if len(address) <= 5 && !strings.Contains(address, " ") {
		return true
	}

	return false
}

func processAddressWithGopostal(address string) (map[string]string, bool) {
	// Use the real gopostal library
	components := postal.ParseAddress(address)
	extracted := extractComponents(components)
	
	// Consider successful if we got any components
	return extracted, len(extracted) > 0
}

func extractComponents(components []postal.ParsedComponent) map[string]string {
	extracted := make(map[string]string)
	
	for _, comp := range components {
		extracted[comp.Label] = comp.Value
	}
	
	return extracted
}

func batchUpdateMatchingRecords(db *sql.DB, processed []ProcessedAddress) (int, error) {
	totalUpdated := 0
	
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	updateStmt, err := tx.Prepare(`
		UPDATE src_document 
		SET gopostal_processed = TRUE,
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
		    gopostal_po_box = $15
		WHERE raw_address = $1 
		  AND gopostal_processed = FALSE
	`)
	if err != nil {
		return 0, err
	}
	defer updateStmt.Close()

	for i, addr := range processed {
		if i%1000 == 0 && i > 0 {
			fmt.Printf("   Updated records for %d/%d addresses...\n", i, len(processed))
		}

		if !addr.ProcessingSuccessful {
			// Still mark as processed even if parsing failed
			result, err := updateStmt.Exec(
				addr.Address,
				"", "", "", "", "", "", "", "", "", "", "", "", "", "",
			)
			if err != nil {
				continue
			}
			
			if rows, _ := result.RowsAffected(); rows > 0 {
				totalUpdated += int(rows)
			}
			continue
		}

		// Update with gopostal components
		result, err := updateStmt.Exec(
			addr.Address,
			getComponent(addr.Components, "house"),
			getComponent(addr.Components, "house_number"),
			getComponent(addr.Components, "road"),
			getComponent(addr.Components, "suburb"),
			getComponent(addr.Components, "city"),
			getComponent(addr.Components, "state_district"),
			getComponent(addr.Components, "state"),
			getComponent(addr.Components, "postcode"),
			getComponent(addr.Components, "country"),
			getComponent(addr.Components, "unit"),
			getComponent(addr.Components, "level"),
			getComponent(addr.Components, "staircase"),
			getComponent(addr.Components, "entrance"),
			getComponent(addr.Components, "po_box"),
		)
		if err != nil {
			continue
		}
		
		if rows, _ := result.RowsAffected(); rows > 0 {
			totalUpdated += int(rows)
		}
	}

	// Handle skipped addresses - mark them as processed but with no components
	_, err = tx.Exec(`
		UPDATE src_document 
		SET gopostal_processed = TRUE
		WHERE gopostal_processed = FALSE
		  AND (raw_address = 'N/A' 
		       OR raw_address IN (
		           SELECT raw_address FROM src_document 
		           WHERE gopostal_processed = FALSE 
		             AND LENGTH(raw_address) <= 5 
		             AND raw_address NOT LIKE '% %'
		       ))
	`)
	if err != nil {
		return totalUpdated, err
	}

	err = tx.Commit()
	return totalUpdated, err
}

func getComponent(components map[string]string, key string) string {
	if val, exists := components[key]; exists {
		return val
	}
	return ""
}

func analyzeLLPGProcessingState(db *sql.DB) (*ProcessingStats, error) {
	stats := &ProcessingStats{}

	// Get total addresses
	err := db.QueryRow("SELECT COUNT(*) FROM dim_address").Scan(&stats.TotalRecords)
	if err != nil {
		return nil, err
	}

	// Get already processed
	err = db.QueryRow("SELECT COUNT(*) FROM dim_address WHERE gopostal_processed = TRUE").Scan(&stats.AlreadyProcessed)
	if err != nil {
		return nil, err
	}

	// For LLPG, unique addresses = total records (no duplicates)
	stats.UniqueAddresses = stats.TotalRecords
	stats.UnprocessedUniqueAddresses = stats.TotalRecords - stats.AlreadyProcessed

	return stats, nil
}

func getUnprocessedLLPGAddresses(db *sql.DB) ([]LLPGAddress, error) {
	query := `
		SELECT address_id, full_address, COALESCE(uprn, ''), COALESCE(is_historic, FALSE)
		FROM dim_address
		WHERE gopostal_processed = FALSE
		ORDER BY address_id
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var addresses []LLPGAddress
	for rows.Next() {
		var addr LLPGAddress
		err := rows.Scan(&addr.AddressID, &addr.Address, &addr.UPRN, &addr.IsHistoric)
		if err != nil {
			continue
		}
		addresses = append(addresses, addr)
	}

	return addresses, nil
}

func processLLPGAddresses(addresses []LLPGAddress) ([]ProcessedAddress, int) {
	var processed []ProcessedAddress
	var skipped int

	for i, addr := range addresses {
		// Progress reporting
		if i%100 == 0 && i > 0 {
			fmt.Printf("   Processed %d/%d LLPG addresses...\n", i, len(addresses))
		}

		// Skip special cases
		if shouldSkipAddress(addr.Address) {
			skipped++
			continue
		}

		// Process with gopostal
		components, success := processAddressWithGopostal(addr.Address)
		
		processed = append(processed, ProcessedAddress{
			Address:              addr.Address,
			Components:          components,
			RecordCount:         1, // Each LLPG address is unique
			ProcessingSuccessful: success,
		})
	}

	return processed, skipped
}

func batchUpdateLLPGRecords(db *sql.DB, processed []ProcessedAddress) (int, error) {
	if len(processed) == 0 {
		return 0, nil
	}
	
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	updateStmt, err := tx.Prepare(`
		UPDATE dim_address 
		SET gopostal_processed = TRUE,
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
		    gopostal_po_box = $15
		WHERE full_address = $1 
		  AND gopostal_processed = FALSE
	`)
	if err != nil {
		return 0, err
	}
	defer updateStmt.Close()

	totalUpdated := 0
	for i, addr := range processed {
		if i%100 == 0 && i > 0 {
			fmt.Printf("   Updated %d/%d LLPG addresses...\n", i, len(processed))
		}

		if !addr.ProcessingSuccessful {
			// Still mark as processed even if parsing failed
			result, err := updateStmt.Exec(
				addr.Address,
				"", "", "", "", "", "", "", "", "", "", "", "", "", "",
			)
			if err != nil {
				continue
			}
			
			if rows, _ := result.RowsAffected(); rows > 0 {
				totalUpdated += int(rows)
			}
			continue
		}

		// Update with gopostal components
		result, err := updateStmt.Exec(
			addr.Address,
			getComponent(addr.Components, "house"),
			getComponent(addr.Components, "house_number"),
			getComponent(addr.Components, "road"),
			getComponent(addr.Components, "suburb"),
			getComponent(addr.Components, "city"),
			getComponent(addr.Components, "state_district"),
			getComponent(addr.Components, "state"),
			getComponent(addr.Components, "postcode"),
			getComponent(addr.Components, "country"),
			getComponent(addr.Components, "unit"),
			getComponent(addr.Components, "level"),
			getComponent(addr.Components, "staircase"),
			getComponent(addr.Components, "entrance"),
			getComponent(addr.Components, "po_box"),
		)
		if err != nil {
			continue
		}
		
		if rows, _ := result.RowsAffected(); rows > 0 {
			totalUpdated += int(rows)
		}
	}

	// Handle any remaining unprocessed LLPG records (mark as processed but with no components)
	_, err = tx.Exec(`
		UPDATE dim_address 
		SET gopostal_processed = TRUE
		WHERE gopostal_processed = FALSE
	`)
	if err != nil {
		return totalUpdated, err
	}

	err = tx.Commit()
	return totalUpdated, err
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