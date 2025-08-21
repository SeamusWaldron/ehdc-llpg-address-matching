package engine

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Exporter handles exporting matched data back to CSV format
type Exporter struct {
	db *sql.DB
}

// NewExporter creates a new exporter
func NewExporter(db *sql.DB) *Exporter {
	return &Exporter{db: db}
}

// EnhancedSourceDocument represents a source document with additional matching columns
type EnhancedSourceDocument struct {
	// Original source document fields
	SrcID        int64      `json:"src_id"`
	SourceType   string     `json:"source_type"`
	JobNumber    *string    `json:"job_number,omitempty"`
	Filepath     *string    `json:"filepath,omitempty"`
	ExternalRef  *string    `json:"external_ref,omitempty"`
	DocType      *string    `json:"doc_type,omitempty"`
	DocDate      *time.Time `json:"doc_date,omitempty"`
	RawAddress   string     `json:"raw_address"`
	AddrCan      *string    `json:"addr_can,omitempty"`
	PostcodeText *string    `json:"postcode_text,omitempty"`
	UPRNRaw      *string    `json:"uprn_raw,omitempty"`
	EastingRaw   *float64   `json:"easting_raw,omitempty"`
	NorthingRaw  *float64   `json:"northing_raw,omitempty"`
	
	// Enhanced matching fields (your requested additions)
	AddressQuality     string   `json:"address_quality"`      // GOOD/FAIR/POOR
	MatchStatus        string   `json:"match_status"`         // MATCHED/UNMATCHED/NEEDS_REVIEW
	MatchMethod        *string  `json:"match_method,omitempty"` // deterministic/fuzzy/spatial/etc
	MatchScore         *float64 `json:"match_score,omitempty"`  // 0.00-1.00
	CoordinateDistance *float64 `json:"coordinate_distance,omitempty"` // meters between source/LLPG coords
	AddressSimilarity  *float64 `json:"address_similarity,omitempty"`  // text similarity score
	
	// Matched LLPG data (for context)
	MatchedUPRN     *string  `json:"matched_uprn,omitempty"`
	LLPGAddress     *string  `json:"llpg_address,omitempty"`
	LLPGEasting     *float64 `json:"llpg_easting,omitempty"`
	LLPGNorthing    *float64 `json:"llpg_northing,omitempty"`
	MatchedBy       *string  `json:"matched_by,omitempty"`
	MatchedAt       *time.Time `json:"matched_at,omitempty"`
}

// ExportEnhancedCSVs exports all source types as enhanced CSVs with matching results
func (e *Exporter) ExportEnhancedCSVs(outputDir string) error {
	fmt.Println("=== Exporting Enhanced Source Document CSVs ===\n")
	
	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	
	// Get all source types
	sourceTypes, err := e.getSourceTypes()
	if err != nil {
		return fmt.Errorf("failed to get source types: %w", err)
	}
	
	totalExported := 0
	for _, sourceType := range sourceTypes {
		count, err := e.exportSourceType(sourceType, outputDir)
		if err != nil {
			fmt.Printf("Error exporting %s: %v\n", sourceType, err)
			continue
		}
		totalExported += count
		fmt.Printf("✓ Exported %d %s records\n", count, sourceType)
	}
	
	fmt.Printf("\n=== Export Complete ===\n")
	fmt.Printf("Total records exported: %d\n", totalExported)
	fmt.Printf("Output directory: %s\n", outputDir)
	
	return nil
}

// getSourceTypes returns all unique source types in the database
func (e *Exporter) getSourceTypes() ([]string, error) {
	rows, err := e.db.Query(`
		SELECT DISTINCT source_type 
		FROM src_document 
		ORDER BY source_type
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var types []string
	for rows.Next() {
		var sourceType string
		if err := rows.Scan(&sourceType); err != nil {
			continue
		}
		types = append(types, sourceType)
	}
	
	return types, nil
}

// exportSourceType exports a specific source type to CSV
func (e *Exporter) exportSourceType(sourceType string, outputDir string) (int, error) {
	// Get enhanced documents for this source type
	docs, err := e.getEnhancedDocuments(sourceType)
	if err != nil {
		return 0, fmt.Errorf("failed to get documents: %w", err)
	}
	
	if len(docs) == 0 {
		fmt.Printf("No documents found for source type: %s\n", sourceType)
		return 0, nil
	}
	
	// Create output file
	filename := fmt.Sprintf("enhanced_%s_results.csv", sourceType)
	outputPath := filepath.Join(outputDir, filename)
	
	file, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	
	writer := csv.NewWriter(file)
	defer writer.Flush()
	
	// Write header based on source type
	header := e.getCSVHeader(sourceType)
	if err := writer.Write(header); err != nil {
		return 0, fmt.Errorf("failed to write header: %w", err)
	}
	
	// Write data rows
	for _, doc := range docs {
		row := e.documentToCSVRow(doc, sourceType)
		if err := writer.Write(row); err != nil {
			fmt.Printf("Error writing row for doc %d: %v\n", doc.SrcID, err)
			continue
		}
	}
	
	return len(docs), nil
}

// getEnhancedDocuments retrieves all documents with matching results for a source type
func (e *Exporter) getEnhancedDocuments(sourceType string) ([]*EnhancedSourceDocument, error) {
	query := `
		SELECT 
			s.src_id, s.source_type, s.job_number, s.filepath, s.external_ref,
			s.doc_type, s.doc_date, s.raw_address, s.addr_can, s.postcode_text,
			s.uprn_raw, s.easting_raw, s.northing_raw,
			
			-- Match data
			m.uprn, m.method, m.score, m.confidence, m.accepted_by, m.accepted_at,
			
			-- LLPG data
			d.locaddress, d.easting as llpg_easting, d.northing as llpg_northing
			
		FROM src_document s
		LEFT JOIN match_accepted m ON m.src_id = s.src_id
		LEFT JOIN dim_address d ON d.uprn = m.uprn
		WHERE s.source_type = $1
		ORDER BY s.src_id
	`
	
	rows, err := e.db.Query(query, sourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var docs []*EnhancedSourceDocument
	
	for rows.Next() {
		doc := &EnhancedSourceDocument{}
		
		err := rows.Scan(
			&doc.SrcID, &doc.SourceType, &doc.JobNumber, &doc.Filepath, &doc.ExternalRef,
			&doc.DocType, &doc.DocDate, &doc.RawAddress, &doc.AddrCan, &doc.PostcodeText,
			&doc.UPRNRaw, &doc.EastingRaw, &doc.NorthingRaw,
			
			// Match data
			&doc.MatchedUPRN, &doc.MatchMethod, &doc.MatchScore, &doc.MatchScore, // using score for both score and confidence
			&doc.MatchedBy, &doc.MatchedAt,
			
			// LLPG data  
			&doc.LLPGAddress, &doc.LLPGEasting, &doc.LLPGNorthing,
		)
		if err != nil {
			continue
		}
		
		// Calculate derived fields
		doc.AddressQuality = e.calculateAddressQuality(doc)
		doc.MatchStatus = e.calculateMatchStatus(doc)
		doc.CoordinateDistance = e.calculateCoordinateDistance(doc)
		doc.AddressSimilarity = e.calculateAddressSimilarity(doc)
		
		docs = append(docs, doc)
	}
	
	return docs, nil
}

// calculateAddressQuality assesses the quality of the original address data
func (e *Exporter) calculateAddressQuality(doc *EnhancedSourceDocument) string {
	score := 0
	
	// Address length and content
	if len(doc.RawAddress) >= 15 {
		score += 2
	} else if len(doc.RawAddress) >= 8 {
		score += 1
	}
	
	// Has postcode
	if doc.PostcodeText != nil && *doc.PostcodeText != "" {
		score += 2
	}
	
	// Has coordinates
	if doc.EastingRaw != nil && doc.NorthingRaw != nil {
		score += 2
	}
	
	// Has house number or building name
	addr := strings.ToUpper(doc.RawAddress)
	hasNumber := false
	for _, char := range addr {
		if char >= '0' && char <= '9' {
			hasNumber = true
			break
		}
	}
	if hasNumber {
		score += 1
	}
	
	// Not just "N A" or very short
	if len(strings.TrimSpace(doc.RawAddress)) <= 3 || 
		strings.ToUpper(strings.TrimSpace(doc.RawAddress)) == "N A" {
		return "POOR"
	}
	
	// Score interpretation
	if score >= 6 {
		return "GOOD"
	} else if score >= 3 {
		return "FAIR" 
	} else {
		return "POOR"
	}
}

// calculateMatchStatus determines the overall match status
func (e *Exporter) calculateMatchStatus(doc *EnhancedSourceDocument) string {
	if doc.MatchedUPRN != nil && *doc.MatchedUPRN != "" {
		// Check if this might need review (low confidence)
		if doc.MatchScore != nil && *doc.MatchScore < 0.70 {
			return "NEEDS_REVIEW"
		}
		return "MATCHED"
	}
	return "UNMATCHED"
}

// calculateCoordinateDistance calculates distance between source and LLPG coordinates
func (e *Exporter) calculateCoordinateDistance(doc *EnhancedSourceDocument) *float64 {
	if doc.EastingRaw == nil || doc.NorthingRaw == nil || 
		doc.LLPGEasting == nil || doc.LLPGNorthing == nil {
		return nil
	}
	
	// Calculate Euclidean distance in meters (BNG coordinates are in meters)
	dx := *doc.LLPGEasting - *doc.EastingRaw
	dy := *doc.LLPGNorthing - *doc.NorthingRaw
	distance := math.Sqrt(dx*dx + dy*dy)
	
	return &distance
}

// calculateAddressSimilarity calculates text similarity between addresses
func (e *Exporter) calculateAddressSimilarity(doc *EnhancedSourceDocument) *float64 {
	if doc.AddrCan == nil || doc.LLPGAddress == nil {
		return nil
	}
	
	// Use PostgreSQL similarity function if available, otherwise simple comparison
	var similarity float64
	err := e.db.QueryRow(`SELECT similarity($1, $2)`, *doc.AddrCan, *doc.LLPGAddress).Scan(&similarity)
	if err != nil {
		// Fallback: simple normalized length comparison
		source := strings.ToUpper(strings.TrimSpace(*doc.AddrCan))
		target := strings.ToUpper(strings.TrimSpace(*doc.LLPGAddress))
		
		if source == target {
			similarity = 1.0
		} else {
			// Simple Jaccard-like similarity
			sourceWords := strings.Fields(source)
			targetWords := strings.Fields(target)
			
			matches := 0
			for _, sw := range sourceWords {
				for _, tw := range targetWords {
					if sw == tw {
						matches++
						break
					}
				}
			}
			
			totalWords := len(sourceWords) + len(targetWords) - matches
			if totalWords > 0 {
				similarity = float64(matches) / float64(totalWords)
			}
		}
	}
	
	return &similarity
}

// getCSVHeader returns the appropriate header for each source type
func (e *Exporter) getCSVHeader(sourceType string) []string {
	// Common enhanced fields for all types
	baseHeader := []string{
		"Source_ID", "Job_Number", "Filepath", "External_Reference", 
		"Document_Type", "Document_Date", "Original_Address", "Canonical_Address",
		"Extracted_Postcode", "Source_UPRN", "Source_Easting", "Source_Northing",
	}
	
	// Enhanced matching columns (your requested additions)
	enhancedFields := []string{
		"Address_Quality", "Match_Status", "Match_Method", "Match_Score",
		"Coordinate_Distance", "Address_Similarity",
		"Matched_UPRN", "LLPG_Address", "LLPG_Easting", "LLPG_Northing",
		"Matched_By", "Matched_At",
	}
	
	// Combine base + enhanced
	header := append(baseHeader, enhancedFields...)
	
	// Add source-type specific fields based on original CSV structures
	switch sourceType {
	case "decision":
		// Original: Job Number,Filepath,Planning Application Number,Adress,Decision Date,Decision Type,Document Type,BS7666UPRN,Easting,Northing
		header = append([]string{"Decision_Date", "Decision_Type"}, header[1:]...)
		
	case "land_charge":
		// Original: Job Number,Filepath,Card Code,Address,BS7666UPRN,Easting,Northing
		// No additional fields needed
		
	case "enforcement":
		// Original: Job Number,Filepath,Planning Enforcement Reference Number,Address,Date,Document Type,BS7666UPRN,Easting,Northing
		// Date and Document_Type already in base
		
	case "agreement":
		// Original: Job Number,Filepath,Address,Date,BS7666UPRN,Easting,Northing
		// No additional fields needed
	}
	
	return header
}

// documentToCSVRow converts an enhanced document to CSV row
func (e *Exporter) documentToCSVRow(doc *EnhancedSourceDocument, sourceType string) []string {
	// Helper function to safely convert pointers to strings
	safeString := func(s *string) string {
		if s == nil { return "" }
		return *s
	}
	
	safeFloat := func(f *float64) string {
		if f == nil { return "" }
		return fmt.Sprintf("%.3f", *f)
	}
	
	
	safeDate := func(t *time.Time) string {
		if t == nil { return "" }
		return t.Format("2006-01-02")
	}
	
	safeDateTime := func(t *time.Time) string {
		if t == nil { return "" }
		return t.Format("2006-01-02 15:04:05")
	}
	
	// Base fields (common to all types)
	row := []string{
		strconv.FormatInt(doc.SrcID, 10),           // Source_ID
		safeString(doc.JobNumber),                  // Job_Number
		safeString(doc.Filepath),                   // Filepath
		safeString(doc.ExternalRef),                // External_Reference
		safeString(doc.DocType),                    // Document_Type
		safeDate(doc.DocDate),                      // Document_Date
		doc.RawAddress,                             // Original_Address
		safeString(doc.AddrCan),                    // Canonical_Address
		safeString(doc.PostcodeText),               // Extracted_Postcode
		safeString(doc.UPRNRaw),                    // Source_UPRN
		safeFloat(doc.EastingRaw),                  // Source_Easting
		safeFloat(doc.NorthingRaw),                 // Source_Northing
	}
	
	// Enhanced matching fields
	enhancedFields := []string{
		doc.AddressQuality,                         // Address_Quality
		doc.MatchStatus,                            // Match_Status
		safeString(doc.MatchMethod),                // Match_Method
		safeFloat(doc.MatchScore),                  // Match_Score
		safeFloat(doc.CoordinateDistance),          // Coordinate_Distance
		safeFloat(doc.AddressSimilarity),           // Address_Similarity
		safeString(doc.MatchedUPRN),                // Matched_UPRN
		safeString(doc.LLPGAddress),                // LLPG_Address
		safeFloat(doc.LLPGEasting),                 // LLPG_Easting
		safeFloat(doc.LLPGNorthing),                // LLPG_Northing
		safeString(doc.MatchedBy),                  // Matched_By
		safeDateTime(doc.MatchedAt),                // Matched_At
	}
	
	return append(row, enhancedFields...)
}

// GetExportStats returns statistics about the export
func (e *Exporter) GetExportStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Total documents by source type
	rows, err := e.db.Query(`
		SELECT 
			source_type,
			COUNT(*) as total,
			COUNT(CASE WHEN m.uprn IS NOT NULL THEN 1 END) as matched,
			COUNT(CASE WHEN m.uprn IS NULL THEN 1 END) as unmatched
		FROM src_document s
		LEFT JOIN match_accepted m ON m.src_id = s.src_id
		GROUP BY source_type
		ORDER BY source_type
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	sourceStats := make(map[string]map[string]int)
	totalDocs := 0
	totalMatched := 0
	
	for rows.Next() {
		var sourceType string
		var total, matched, unmatched int
		
		if err := rows.Scan(&sourceType, &total, &matched, &unmatched); err != nil {
			continue
		}
		
		sourceStats[sourceType] = map[string]int{
			"total":     total,
			"matched":   matched,
			"unmatched": unmatched,
		}
		
		totalDocs += total
		totalMatched += matched
	}
	
	stats["by_source_type"] = sourceStats
	stats["total_documents"] = totalDocs
	stats["total_matched"] = totalMatched
	stats["total_unmatched"] = totalDocs - totalMatched
	stats["match_rate"] = float64(totalMatched) / float64(totalDocs) * 100
	
	return stats, nil
}