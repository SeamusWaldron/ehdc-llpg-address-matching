package import_pkg

import (
	"fmt"
	"strings"
)

// ImportDecisionNotices imports decision notices CSV
// Columns: Job Number,Filepath,Planning Application Number,Adress,Decision Date,Decision Type,Document Type,BS7666UPRN,Easting,Northing
func (ci *CSVImporter) ImportDecisionNotices(filename string) error {
	return ci.ImportCSV(filename, "decision", func(record []string) (*SourceDocument, error) {
		if len(record) < 10 {
			return nil, fmt.Errorf("insufficient columns: expected 10, got %d", len(record))
		}

		return &SourceDocument{
			SourceType:   "decision",
			JobNumber:    strings.TrimSpace(record[0]),
			Filepath:     strings.TrimSpace(record[1]),
			ExternalRef:  strings.TrimSpace(record[2]), // Planning Application Number
			RawAddress:   strings.TrimSpace(record[3]), // Note: "Adress" - preserving the typo
			DocDate:      parseDate(strings.TrimSpace(record[4])),
			DocType:      strings.TrimSpace(record[6]), // Document Type
			UPRNRaw:      strings.TrimSpace(record[7]),
			EastingRaw:   parseFloat(strings.TrimSpace(record[8])),
			NorthingRaw:  parseFloat(strings.TrimSpace(record[9])),
		}, nil
	})
}

// ImportLandCharges imports land charges cards CSV
// Columns: Job Number,Filepath,Card Code,Address,BS7666UPRN,Easting,Northing
func (ci *CSVImporter) ImportLandCharges(filename string) error {
	return ci.ImportCSV(filename, "land_charge", func(record []string) (*SourceDocument, error) {
		if len(record) < 7 {
			return nil, fmt.Errorf("insufficient columns: expected 7, got %d", len(record))
		}

		return &SourceDocument{
			SourceType:  "land_charge",
			JobNumber:   strings.TrimSpace(record[0]),
			Filepath:    strings.TrimSpace(record[1]),
			ExternalRef: strings.TrimSpace(record[2]), // Card Code
			RawAddress:  strings.TrimSpace(record[3]),
			UPRNRaw:     strings.TrimSpace(record[4]),
			EastingRaw:  parseFloat(strings.TrimSpace(record[5])),
			NorthingRaw: parseFloat(strings.TrimSpace(record[6])),
		}, nil
	})
}

// ImportEnforcementNotices imports enforcement notices CSV
// Columns: Job Number,Filepath,Planning Enforcement Reference Number,Address,Date,Document Type,BS7666UPRN,Easting,Northing
func (ci *CSVImporter) ImportEnforcementNotices(filename string) error {
	return ci.ImportCSV(filename, "enforcement", func(record []string) (*SourceDocument, error) {
		if len(record) < 9 {
			return nil, fmt.Errorf("insufficient columns: expected 9, got %d", len(record))
		}

		return &SourceDocument{
			SourceType:  "enforcement",
			JobNumber:   strings.TrimSpace(record[0]),
			Filepath:    strings.TrimSpace(record[1]),
			ExternalRef: strings.TrimSpace(record[2]), // Planning Enforcement Reference Number
			RawAddress:  strings.TrimSpace(record[3]),
			DocDate:     parseDate(strings.TrimSpace(record[4])),
			DocType:     strings.TrimSpace(record[5]),
			UPRNRaw:     strings.TrimSpace(record[6]),
			EastingRaw:  parseFloat(strings.TrimSpace(record[7])),
			NorthingRaw: parseFloat(strings.TrimSpace(record[8])),
		}, nil
	})
}

// ImportAgreements imports agreements CSV
// Columns: Job Number,Filepath,Address,Date,BS7666UPRN,Easting,Northing
func (ci *CSVImporter) ImportAgreements(filename string) error {
	return ci.ImportCSV(filename, "agreement", func(record []string) (*SourceDocument, error) {
		if len(record) < 7 {
			return nil, fmt.Errorf("insufficient columns: expected 7, got %d", len(record))
		}

		// Generate external reference from filepath since there's no explicit reference field
		externalRef := extractFilenameFromPath(strings.TrimSpace(record[1]))

		return &SourceDocument{
			SourceType:  "agreement",
			JobNumber:   strings.TrimSpace(record[0]),
			Filepath:    strings.TrimSpace(record[1]),
			ExternalRef: externalRef,
			RawAddress:  strings.TrimSpace(record[2]),
			DocDate:     parseDate(strings.TrimSpace(record[3])),
			UPRNRaw:     strings.TrimSpace(record[4]),
			EastingRaw:  parseFloat(strings.TrimSpace(record[5])),
			NorthingRaw: parseFloat(strings.TrimSpace(record[6])),
		}, nil
	})
}

// extractFilenameFromPath extracts filename from full file path
func extractFilenameFromPath(filepath string) string {
	// Handle both Windows and Unix path separators
	lastSlash := -1
	for i, char := range filepath {
		if char == '/' || char == '\\' {
			lastSlash = i
		}
	}
	
	if lastSlash >= 0 && lastSlash < len(filepath)-1 {
		return filepath[lastSlash+1:]
	}
	
	return filepath
}