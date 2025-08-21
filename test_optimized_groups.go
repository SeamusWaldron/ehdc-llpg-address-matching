package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	// Connect to database
	connStr := "postgres://postgres:kljh234hjkl2h@localhost:15435/ehdc_llpg?sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Test the data fetching query
	start := time.Now()
	
	groupDataSQL := `
WITH group_summary AS (
    SELECT 
        s.planning_app_base,
        COUNT(*) as total_docs,
        COUNT(am.document_id) FILTER (WHERE am.confidence_score >= 0.9) as golden_count,
        COUNT(*) FILTER (WHERE am.confidence_score IS NULL OR am.confidence_score = 0) as unmatched_count
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
    WHERE s.planning_app_base IS NOT NULL
      AND amc.document_id IS NULL
    GROUP BY s.planning_app_base
    HAVING COUNT(*) BETWEEN 2 AND 8
      AND COUNT(am.document_id) FILTER (WHERE am.confidence_score >= 0.9) >= 2
      AND COUNT(*) FILTER (WHERE am.confidence_score IS NULL OR am.confidence_score = 0) >= 1
),
golden_records AS (
    SELECT DISTINCT ON (s.planning_app_base)
        s.planning_app_base,
        da.address_id as golden_address_id,
        da.full_address as golden_address_text,
        dl.location_id as golden_location_id
    FROM src_document s
    JOIN address_match am ON s.document_id = am.document_id
    JOIN dim_address da ON am.address_id = da.address_id
    LEFT JOIN dim_location dl ON da.location_id = dl.location_id
    WHERE am.confidence_score >= 0.9
      AND s.planning_app_base IN (SELECT planning_app_base FROM group_summary)
    ORDER BY s.planning_app_base, am.confidence_score DESC
),
unmatched_docs AS (
    SELECT 
        s.document_id,
        s.planning_app_base,
        s.raw_address
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
    WHERE s.planning_app_base IN (SELECT planning_app_base FROM group_summary)
      AND amc.document_id IS NULL
      AND (am.confidence_score IS NULL OR am.confidence_score = 0)
      AND s.raw_address IS NOT NULL 
      AND s.raw_address != ''
      AND LENGTH(s.raw_address) > 10
)
SELECT 
    u.document_id,
    u.planning_app_base,
    u.raw_address,
    g.golden_address_id,
    g.golden_address_text,
    g.golden_location_id
FROM unmatched_docs u
JOIN golden_records g ON u.planning_app_base = g.planning_app_base
ORDER BY u.planning_app_base, u.document_id`

	rows, err := db.Query(groupDataSQL)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()
	
	count := 0
	planningApp20003Count := 0
	for rows.Next() {
		var documentID int
		var planningAppBase, rawAddress, goldenAddress string
		var goldenAddressID int64
		var goldenLocationID sql.NullInt64
		
		err := rows.Scan(&documentID, &planningAppBase, &rawAddress, 
			&goldenAddressID, &goldenAddress, &goldenLocationID)
		if err != nil {
			continue
		}
		
		count++
		if planningAppBase == "20003" {
			planningApp20003Count++
			fmt.Printf("20003 item %d: doc=%d, raw=%.50s, golden=%.50s\n", 
				planningApp20003Count, documentID, rawAddress, goldenAddress)
		}
	}
	
	elapsed := time.Since(start)
	fmt.Printf("\nQuery Performance Test:\n")
	fmt.Printf("- Total work items: %d\n", count)
	fmt.Printf("- Planning app 20003 items: %d\n", planningApp20003Count)
	fmt.Printf("- Query execution time: %.2f seconds\n", elapsed.Seconds())
	
	if count > 0 {
		fmt.Printf("- Expected LLM calls: %d\n", count)
		fmt.Printf("- With 8 workers: ~%.1f seconds (est)\n", float64(count)/8.0 * 0.5) // Assume 0.5s per LLM call
	}
}