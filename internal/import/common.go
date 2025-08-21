package import_pkg

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/ehdc-llpg/internal/normalize"
)

// SourceDocument represents a document record for import
type SourceDocument struct {
	SourceType   string
	JobNumber    string
	Filepath     string
	ExternalRef  string
	DocType      string
	DocDate      *time.Time
	RawAddress   string
	AddrCan      string
	PostcodeText string
	UPRNRaw      string
	EastingRaw   *float64
	NorthingRaw  *float64
}

// CSVImporter handles importing CSV files into src_document table
type CSVImporter struct {
	db *sql.DB
}

// NewCSVImporter creates a new CSV importer
func NewCSVImporter(db *sql.DB) *CSVImporter {
	return &CSVImporter{db: db}
}

// ImportCSV imports CSV file with given mapping function
func (ci *CSVImporter) ImportCSV(filename string, sourceType string, mapFunc func([]string) (*SourceDocument, error)) error {
	fmt.Printf("Importing %s from %s...\n", sourceType, filename)
	
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	
	// Skip header
	_, err = reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	// Prepare insert statement
	stmt, err := ci.db.Prepare(`
		INSERT INTO src_document (
			source_type, job_number, filepath, external_ref, doc_type, doc_date,
			raw_address, addr_can, postcode_text, uprn_raw, easting_raw, northing_raw
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	imported := 0
	errors := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error reading CSV record: %v\n", err)
			errors++
			continue
		}

		doc, err := mapFunc(record)
		if err != nil {
			fmt.Printf("Error mapping record: %v\n", err)
			errors++
			continue
		}

		// Generate canonical address and extract postcode
		doc.AddrCan, doc.PostcodeText, _ = normalize.CanonicalAddress(doc.RawAddress)

		// Insert record
		_, err = stmt.Exec(
			doc.SourceType, doc.JobNumber, doc.Filepath, doc.ExternalRef,
			doc.DocType, doc.DocDate, doc.RawAddress, doc.AddrCan,
			doc.PostcodeText, doc.UPRNRaw, doc.EastingRaw, doc.NorthingRaw,
		)
		if err != nil {
			fmt.Printf("Error inserting record: %v\n", err)
			errors++
			continue
		}

		imported++
		if imported%1000 == 0 {
			fmt.Printf("Imported %d records...\n", imported)
		}
	}

	fmt.Printf("Import complete: %d records imported, %d errors\n", imported, errors)
	return nil
}

// parseFloat safely converts string to float64 pointer
func parseFloat(s string) *float64 {
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// parseDate safely converts string to time.Time pointer
func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	
	// Try different date formats
	formats := []string{
		"02/01/2006",
		"2/1/2006", 
		"02/01/06",
		"2/1/06",
		"2006-01-02",
	}
	
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return &t
		}
	}
	
	return nil
}