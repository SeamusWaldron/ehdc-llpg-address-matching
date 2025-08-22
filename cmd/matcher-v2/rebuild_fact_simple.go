package main

import (
	"database/sql"
	"fmt"
)

// rebuildFactTableSimple implements a fast intelligent fact table population strategy
// Phase 1: Insert records with UPRN matches  
// Phase 2: Insert remaining unmatched records (skip expensive canonical matching for now)
func rebuildFactTableSimple(localDebug bool, db *sql.DB) error {
	fmt.Println("Rebuilding fact table with simple intelligent UPRN-first approach...")
	fmt.Println("=================================================================")
	
	// First, clear the fact table
	fmt.Println("Truncating fact table...")
	_, err := db.Exec("TRUNCATE TABLE fact_documents_lean")
	if err != nil {
		return fmt.Errorf("failed to truncate fact table: %v", err)
	}
	
	// Ensure dim_original_address is up to date
	fmt.Println("Updating dim_original_address...")
	updateDimOriginalQuery := `
	INSERT INTO dim_original_address (raw_address, address_hash)
	SELECT DISTINCT 
	    raw_address,
	    MD5(UPPER(TRIM(raw_address))) as address_hash
	FROM src_document s
	WHERE raw_address IS NOT NULL 
	  AND raw_address != ''
	  AND NOT EXISTS (
	    SELECT 1 FROM dim_original_address oa 
	    WHERE oa.raw_address = s.raw_address
	  )
	`
	result, err := db.Exec(updateDimOriginalQuery)
	if err != nil {
		return fmt.Errorf("failed to update dim_original_address: %v", err)
	}
	newOriginalAddresses, _ := result.RowsAffected()
	if newOriginalAddresses > 0 {
		fmt.Printf("  ✓ Added %d new original addresses\n", newOriginalAddresses)
	}
	
	// PHASE 1: Insert records with UPRN matches
	fmt.Println("\nPhase 1: Inserting records with UPRN matches...")
	uprnQuery := `
	INSERT INTO fact_documents_lean (
		document_id, doc_type_id, document_status_id, original_address_id,
		matched_address_id, match_method_id, match_confidence_score, 
		property_type_id, application_status_id, development_type_id,
		application_date_id, decision_date_id, import_date_id, 
		match_decision_id, matched_location_id
	)
	SELECT DISTINCT ON (s.document_id)
		s.document_id,
		COALESCE(s.doc_type_id, 1) as doc_type_id,
		1 as document_status_id,  -- Active
		oa.original_address_id,
		da.address_id as matched_address_id,  -- UPRN MATCH!
		1 as match_method_id,  -- "Exact UPRN Match"
		1.0 as match_confidence_score,
		NULL as property_type_id,  -- Unknown
		NULL as application_status_id,
		NULL as development_type_id,
		CASE 
			WHEN s.document_date IS NOT NULL THEN TO_CHAR(s.document_date, 'YYYYMMDD')::INTEGER
			ELSE NULL
		END as application_date_id,
		NULL as decision_date_id,
		TO_CHAR(CURRENT_DATE, 'YYYYMMDD')::INTEGER as import_date_id,
		2 as match_decision_id,  -- "Accepted"
		da.location_id as matched_location_id
	FROM src_document s
	INNER JOIN dim_original_address oa ON oa.raw_address = s.raw_address
	INNER JOIN dim_address da ON da.uprn = s.raw_uprn  -- UPRN MATCH
	WHERE s.raw_uprn IS NOT NULL 
	  AND s.raw_uprn != ''
	  AND da.uprn IS NOT NULL
	ORDER BY s.document_id, da.address_id
	`
	
	result, err = db.Exec(uprnQuery)
	if err != nil {
		return fmt.Errorf("failed to insert UPRN matches: %v", err)
	}
	uprnMatches, _ := result.RowsAffected()
	fmt.Printf("  ✓ Inserted %d records with UPRN matches\n", uprnMatches)
	
	// PHASE 2: Insert remaining unmatched records (skip canonical matching for now)
	fmt.Println("\nPhase 2: Inserting remaining unmatched records...")
	
	// First ensure all raw addresses are in dim_original_address
	fmt.Println("  Ensuring all addresses are in dim_original_address...")
	_, err = db.Exec(`
		INSERT INTO dim_original_address (raw_address, address_hash)
		SELECT DISTINCT 
		    raw_address,
		    MD5(UPPER(TRIM(raw_address))) as address_hash
		FROM src_document s
		WHERE raw_address IS NOT NULL 
		  AND raw_address != ''
		  AND NOT EXISTS (
		    SELECT 1 FROM dim_original_address oa 
		    WHERE oa.raw_address = s.raw_address
		  )
	`)
	if err != nil {
		fmt.Printf("  Warning: failed to update dim_original_address: %v\n", err)
	}
	
	unmatchedQuery := `
	INSERT INTO fact_documents_lean (
		document_id, doc_type_id, document_status_id, original_address_id,
		matched_address_id, match_method_id, match_confidence_score,
		property_type_id, application_status_id, development_type_id,
		application_date_id, decision_date_id, import_date_id,
		match_decision_id, matched_location_id
	)
	SELECT DISTINCT ON (s.document_id)
		s.document_id,
		COALESCE(s.doc_type_id, 1) as doc_type_id,
		1 as document_status_id,
		oa.original_address_id,
		NULL::INTEGER as matched_address_id,  -- NO MATCH YET
		NULL::INTEGER as match_method_id,
		NULL::NUMERIC as match_confidence_score,
		NULL::INTEGER as property_type_id,
		NULL::INTEGER as application_status_id,
		NULL::INTEGER as development_type_id,
		CASE 
			WHEN s.document_date IS NOT NULL THEN TO_CHAR(s.document_date, 'YYYYMMDD')::INTEGER
			ELSE NULL
		END as application_date_id,
		NULL::INTEGER as decision_date_id,
		TO_CHAR(CURRENT_DATE, 'YYYYMMDD')::INTEGER as import_date_id,
		1 as match_decision_id,  -- "Pending"
		NULL::INTEGER as matched_location_id
	FROM src_document s
	INNER JOIN dim_original_address oa ON oa.raw_address = s.raw_address
	WHERE NOT EXISTS (SELECT 1 FROM fact_documents_lean f WHERE f.document_id = s.document_id)  -- Not already inserted
	ORDER BY s.document_id, oa.original_address_id
	`
	
	result, err = db.Exec(unmatchedQuery)
	if err != nil {
		return fmt.Errorf("failed to insert unmatched records: %v", err)
	}
	unmatchedRecords, _ := result.RowsAffected()
	fmt.Printf("  ✓ Inserted %d unmatched records\n", unmatchedRecords)
	
	// Summary
	totalRecords := uprnMatches + unmatchedRecords
	fmt.Printf("\n=== SIMPLE INTELLIGENT FACT TABLE SUMMARY ===\n")
	fmt.Printf("Total records: %d\n", totalRecords)
	fmt.Printf("  UPRN matches: %d (%.1f%%)\n", uprnMatches, float64(uprnMatches)/float64(totalRecords)*100)
	fmt.Printf("  Unmatched (for further processing): %d (%.1f%%)\n", unmatchedRecords, float64(unmatchedRecords)/float64(totalRecords)*100)
	
	// Verify all source records are accounted for
	var sourceCount int
	err = db.QueryRow("SELECT COUNT(*) FROM src_document").Scan(&sourceCount)
	if err == nil {
		if int64(sourceCount) == totalRecords {
			fmt.Printf("\n✓ All %d source records accounted for\n", sourceCount)
		} else {
			fmt.Printf("\n⚠ Warning: %d source records but %d in fact table (difference: %d)\n", 
				sourceCount, totalRecords, int64(sourceCount)-totalRecords)
		}
	}
	
	// Show sample UPRN matches
	if localDebug {
		fmt.Println("\n=== SAMPLE UPRN MATCHES ===")
		sampleQuery := `
		SELECT s.raw_address, da.full_address, da.uprn
		FROM fact_documents_lean f
		JOIN src_document s ON f.document_id = s.document_id
		JOIN dim_address da ON f.matched_address_id = da.address_id
		WHERE f.match_method_id = 1
		LIMIT 5
		`
		rows, err := db.Query(sampleQuery)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var source, matched, uprn string
				if rows.Scan(&source, &matched, &uprn) == nil {
					fmt.Printf("  %s\n    → %s (UPRN: %s)\n", source, matched, uprn)
				}
			}
		}
	}
	
	return nil
}