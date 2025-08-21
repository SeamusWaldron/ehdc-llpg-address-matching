package engine

import (
	"database/sql"
	"fmt"
	"io"
	"os"
)

// DBUtil provides database utility functions
type DBUtil struct {
	db *sql.DB
}

// NewDBUtil creates a new database utility
func NewDBUtil(db *sql.DB) *DBUtil {
	return &DBUtil{db: db}
}

// ExecuteSQLFiles executes multiple SQL files
func (util *DBUtil) ExecuteSQLFiles(filenames ...string) error {
	for _, filename := range filenames {
		if err := util.ExecuteSQLFile(filename); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteSQLFile executes SQL commands from a file
func (util *DBUtil) ExecuteSQLFile(filename string) error {
	fmt.Printf("Executing SQL file: %s\n", filename)
	
	// Read the SQL file
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open SQL file: %w", err)
	}
	defer file.Close()
	
	content, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read SQL file: %w", err)
	}
	
	// Execute the SQL
	_, err = util.db.Exec(string(content))
	if err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}
	
	fmt.Printf("✓ Successfully executed SQL file: %s\n", filename)
	return nil
}

// TestViews tests that all enhanced views are working correctly
func (util *DBUtil) TestViews() error {
	fmt.Println("=== Testing Enhanced Views ===\n")
	
	views := map[string]string{
		"v_enhanced_source_documents": "Main enhanced source documents view",
		"v_enhanced_decisions":        "Enhanced decisions view", 
		"v_enhanced_land_charges":     "Enhanced land charges view",
		"v_enhanced_enforcement":      "Enhanced enforcement view",
		"v_enhanced_agreements":       "Enhanced agreements view",
		"v_match_summary_by_type":     "Match summary by type",
		"v_address_quality_summary":   "Address quality summary",
		"v_method_effectiveness":      "Method effectiveness summary",
		"v_unmatched_analysis":        "Unmatched analysis view",
	}
	
	for viewName, description := range views {
		var count int
		err := util.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", viewName)).Scan(&count)
		if err != nil {
			fmt.Printf("✗ %s (%s): ERROR - %v\n", viewName, description, err)
			continue
		}
		fmt.Printf("✓ %s (%s): %d rows\n", viewName, description, count)
	}
	
	return nil
}

// ShowViewSamples shows sample data from key views
func (util *DBUtil) ShowViewSamples() error {
	fmt.Println("\n=== Sample Data from Enhanced Views ===\n")
	
	// Show main enhanced view sample
	fmt.Println("Sample from v_enhanced_source_documents:")
	fmt.Println("ID | Type    | Address Quality | Match Status | Method | Score")
	fmt.Println("---|---------|-----------------|--------------|--------|------")
	
	rows, err := util.db.Query(`
		SELECT src_id, source_type, address_quality, match_status, 
			   COALESCE(match_method, 'N/A'), COALESCE(match_score::text, 'N/A')
		FROM v_enhanced_source_documents 
		ORDER BY src_id 
		LIMIT 5
	`)
	if err != nil {
		return fmt.Errorf("failed to query sample data: %w", err)
	}
	defer rows.Close()
	
	for rows.Next() {
		var srcID int64
		var sourceType, addressQuality, matchStatus, method, score string
		err := rows.Scan(&srcID, &sourceType, &addressQuality, &matchStatus, &method, &score)
		if err != nil {
			continue
		}
		fmt.Printf("%3d| %-7s | %-15s | %-12s | %-6s | %s\n", 
			srcID, sourceType, addressQuality, matchStatus, method, score)
	}
	
	// Show summary statistics
	fmt.Println("\nMatch Summary by Source Type:")
	rows2, err := util.db.Query("SELECT * FROM v_match_summary_by_type ORDER BY source_type")
	if err == nil {
		defer rows2.Close()
		fmt.Println("Type        | Total  | Matched | Unmatched | Rate")
		fmt.Println("------------|--------|---------|-----------|------")
		
		for rows2.Next() {
			var sourceType string
			var total, matched, unmatched, needsReview int64
			var rate float64
			err := rows2.Scan(&sourceType, &total, &matched, &unmatched, &needsReview, &rate)
			if err != nil {
				continue
			}
			fmt.Printf("%-11s | %6d | %7d | %9d | %5.1f%%\n", 
				sourceType, total, matched, unmatched, rate)
		}
	}
	
	return nil
}