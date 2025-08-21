package main

import (
	"database/sql"
	"fmt"
	"log"

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

	fmt.Println("=== CURRENT FUZZY MATCHING BEHAVIOR ===")
	
	// Test 1: Current "completely unmatched" query
	fmt.Println("\n1. Groups that would be processed by CURRENT fuzzy matching:")
	currentSQL := `
WITH unmatched_groups AS (
    SELECT 
        s.planning_app_base,
        COUNT(*) as total_docs,
        COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0) as matched_docs,
        COUNT(*) FILTER (WHERE is_real_address(s.raw_address)) as real_addresses,
        (SELECT s2.raw_address 
         FROM src_document s2 
         WHERE s2.planning_app_base = s.planning_app_base 
           AND is_real_address(s2.raw_address)
         ORDER BY LENGTH(s2.raw_address) DESC 
         LIMIT 1) as best_address_in_group
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    WHERE s.planning_app_base IS NOT NULL
    GROUP BY s.planning_app_base
    HAVING COUNT(*) BETWEEN 2 AND 30
      AND COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0) = 0  -- Current condition: NO matches
      AND COUNT(*) FILTER (WHERE is_real_address(s.raw_address)) > 0
)
SELECT 
    planning_app_base,
    total_docs,
    real_addresses,
    best_address_in_group
FROM unmatched_groups 
WHERE best_address_in_group IS NOT NULL
ORDER BY planning_app_base
LIMIT 10`
	
	rows, err := db.Query(currentSQL)
	if err != nil {
		log.Printf("Current query failed: %v", err)
		return
	}
	defer rows.Close()
	
	currentCount := 0
	for rows.Next() {
		var planningApp, bestAddr string
		var totalDocs, realAddrs int
		err := rows.Scan(&planningApp, &totalDocs, &realAddrs, &bestAddr)
		if err != nil {
			continue
		}
		currentCount++
		if currentCount <= 5 {
			fmt.Printf("   %s: %d docs, best: %.50s...\n", planningApp, totalDocs, bestAddr)
		}
	}
	fmt.Printf("   Total groups for current fuzzy matching: %d\n", currentCount)
	
	// Test 2: Improved "low confidence" query  
	fmt.Println("\n2. Groups that would be processed by IMPROVED fuzzy matching:")
	improvedSQL := `
WITH low_confidence_groups AS (
    SELECT 
        s.planning_app_base,
        COUNT(*) as total_docs,
        COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0.5) as good_matches,
        COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0) as any_matches,
        MAX(COALESCE(am.confidence_score, 0)) as best_confidence,
        COUNT(*) FILTER (WHERE is_real_address(s.raw_address)) as real_addresses,
        (SELECT s2.raw_address 
         FROM src_document s2 
         WHERE s2.planning_app_base = s.planning_app_base 
           AND is_real_address(s2.raw_address)
         ORDER BY LENGTH(s2.raw_address) DESC 
         LIMIT 1) as best_address_in_group
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    WHERE s.planning_app_base IS NOT NULL
    GROUP BY s.planning_app_base
    HAVING COUNT(*) BETWEEN 2 AND 30
      AND COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0.5) = 0        -- No good matches (>0.5)
      AND MAX(COALESCE(am.confidence_score, 0)) < 0.5   -- Best match is poor (<0.5)
      AND COUNT(*) FILTER (WHERE is_real_address(s.raw_address)) > 0
)
SELECT 
    planning_app_base,
    total_docs,
    any_matches,
    best_confidence,
    real_addresses,
    best_address_in_group
FROM low_confidence_groups 
WHERE best_address_in_group IS NOT NULL
ORDER BY planning_app_base
LIMIT 20`
	
	rows2, err := db.Query(improvedSQL)
	if err != nil {
		log.Printf("Improved query failed: %v", err)
		return
	}
	defer rows2.Close()
	
	improvedCount := 0
	found20026 := false
	for rows2.Next() {
		var planningApp, bestAddr string
		var totalDocs, anyMatches, realAddrs int
		var bestConf float64
		err := rows2.Scan(&planningApp, &totalDocs, &anyMatches, &bestConf, &realAddrs, &bestAddr)
		if err != nil {
			continue
		}
		improvedCount++
		if planningApp == "20026" {
			found20026 = true
			fmt.Printf("   ★ %s: %d docs, %d matches, best conf: %.3f, addr: %.50s...\n", 
				planningApp, totalDocs, anyMatches, bestConf, bestAddr)
		} else if improvedCount <= 7 {
			fmt.Printf("   %s: %d docs, %d matches, best conf: %.3f, addr: %.50s...\n", 
				planningApp, totalDocs, anyMatches, bestConf, bestAddr)
		}
	}
	fmt.Printf("   Total groups for improved fuzzy matching: %d\n", improvedCount)
	
	if found20026 {
		fmt.Println("\n✓ SUCCESS: Planning app 20026 would be processed by improved fuzzy matching!")
	} else {
		fmt.Println("\n✗ ISSUE: Planning app 20026 still not found by improved fuzzy matching")
	}
	
	// Test 3: Check planning app 20026 specifically
	fmt.Println("\n3. Planning app 20026 detailed analysis:")
	detailSQL := `
SELECT 
    s.planning_app_base,
    COUNT(*) as total_docs,
    COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0.5) as good_matches,
    COUNT(am.document_id) FILTER (WHERE am.confidence_score > 0) as any_matches,
    MAX(COALESCE(am.confidence_score, 0)) as best_confidence,
    COUNT(*) FILTER (WHERE is_real_address(s.raw_address)) as real_addresses,
    COUNT(*) FILTER (WHERE NOT is_real_address(s.raw_address)) as planning_refs
FROM src_document s
LEFT JOIN address_match am ON s.document_id = am.document_id
WHERE s.planning_app_base = '20026'
GROUP BY s.planning_app_base`
	
	var planningApp string
	var totalDocs, goodMatches, anyMatches, realAddrs, planningRefs int
	var bestConf float64
	
	err = db.QueryRow(detailSQL).Scan(&planningApp, &totalDocs, &goodMatches, &anyMatches, &bestConf, &realAddrs, &planningRefs)
	if err != nil {
		fmt.Printf("   Error getting 20026 details: %v\n", err)
	} else {
		fmt.Printf("   Planning App: %s\n", planningApp)
		fmt.Printf("   Total docs: %d\n", totalDocs)
		fmt.Printf("   Good matches (>0.5): %d\n", goodMatches)
		fmt.Printf("   Any matches (>0): %d\n", anyMatches)
		fmt.Printf("   Best confidence: %.3f\n", bestConf)
		fmt.Printf("   Real addresses: %d\n", realAddrs)
		fmt.Printf("   Planning refs: %d\n", planningRefs)
		
		if goodMatches == 0 && bestConf < 0.5 && realAddrs > 0 {
			fmt.Println("   ✓ Would qualify for improved fuzzy matching")
		} else {
			fmt.Println("   ✗ Would NOT qualify for improved fuzzy matching")
		}
	}
}