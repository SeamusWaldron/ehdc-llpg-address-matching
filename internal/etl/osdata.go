package etl

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ehdc-llpg/internal/debug"
)

// OSDataLoader handles loading OS Open UPRN data for validation and enrichment
type OSDataLoader struct {
	db *sql.DB
}

// NewOSDataLoader creates a new OS data loader
func NewOSDataLoader(db *sql.DB) *OSDataLoader {
	return &OSDataLoader{db: db}
}

// LoadOSOpenUPRN loads OS Open UPRN data for validation purposes
func (osl *OSDataLoader) LoadOSOpenUPRN(localDebug bool, csvPath string, batchSize int) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	debug.DebugOutput(localDebug, "Loading OS Open UPRN data from: %s", csvPath)

	// Create OS UPRN reference table if it doesn't exist
	err := osl.createOSUPRNTable(localDebug)
	if err != nil {
		return fmt.Errorf("failed to create OS UPRN table: %w", err)
	}

	// Clear existing data
	_, err = osl.db.Exec("TRUNCATE TABLE os_uprn_reference")
	if err != nil {
		return fmt.Errorf("failed to truncate OS UPRN table: %w", err)
	}

	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("failed to open OS UPRN CSV: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	
	// Read header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	debug.DebugOutput(localDebug, "CSV columns: %v", header)

	// Create column mapping
	columnMap := make(map[string]int)
	for i, col := range header {
		columnMap[strings.ToLower(strings.TrimSpace(col))] = i
	}

	// Prepare batch insert statement
	stmt, err := osl.db.Prepare(`
		INSERT INTO os_uprn_reference (
			uprn, x_coordinate, y_coordinate, latitude, longitude
		) VALUES ($1, $2, $3, $4, $5)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()

	// Process records in batches
	recordCount := 0
	batchCount := 0
	tx, err := osl.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			debug.DebugOutput(localDebug, "Error reading CSV record %d: %v", recordCount, err)
			continue
		}

		// Extract values using column mapping
		uprn := osl.getColumnValue(record, columnMap, "uprn")
		xCoord := osl.getColumnValue(record, columnMap, "x_coordinate")
		yCoord := osl.getColumnValue(record, columnMap, "y_coordinate")
		latitude := osl.getColumnValue(record, columnMap, "latitude")
		longitude := osl.getColumnValue(record, columnMap, "longitude")

		// Skip records with missing essential data
		if uprn == "" || xCoord == "" || yCoord == "" {
			continue
		}

		// Insert record
		_, err = tx.Stmt(stmt).Exec(
			uprn,
			osl.parseNullableFloat(xCoord),
			osl.parseNullableFloat(yCoord),
			osl.parseNullableFloat(latitude),
			osl.parseNullableFloat(longitude),
		)
		if err != nil {
			debug.DebugOutput(localDebug, "Error inserting record %d: %v", recordCount, err)
			continue
		}

		recordCount++
		batchCount++

		// Commit batch and start new transaction
		if batchCount >= batchSize {
			err = tx.Commit()
			if err != nil {
				return fmt.Errorf("failed to commit batch at record %d: %w", recordCount, err)
			}

			debug.DebugOutput(localDebug, "Committed batch: %d records processed", recordCount)

			tx, err = osl.db.Begin()
			if err != nil {
				return fmt.Errorf("failed to begin new transaction: %w", err)
			}
			batchCount = 0
		}

		if recordCount%100000 == 0 {
			debug.DebugOutput(localDebug, "Processed %d OS UPRN records", recordCount)
		}
	}

	// Commit final batch
	if batchCount > 0 {
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit final batch: %w", err)
		}
	}

	debug.DebugOutput(localDebug, "OS UPRN loading complete: %d records", recordCount)

	// Create indexes for performance
	err = osl.createOSUPRNIndexes(localDebug)
	if err != nil {
		debug.DebugOutput(localDebug, "Warning: failed to create OS UPRN indexes: %v", err)
	}

	return nil
}

// createOSUPRNTable creates the OS UPRN reference table
func (osl *OSDataLoader) createOSUPRNTable(localDebug bool) error {
	debug.DebugOutput(localDebug, "Creating OS UPRN reference table")

	_, err := osl.db.Exec(`
		CREATE TABLE IF NOT EXISTS os_uprn_reference (
			uprn           text PRIMARY KEY,
			x_coordinate   numeric,
			y_coordinate   numeric,  
			latitude       numeric,
			longitude      numeric,
			geom27700      geometry(Point, 27700), -- British National Grid
			geom4326       geometry(Point, 4326),  -- WGS84
			created_at     timestamptz DEFAULT now()
		)
	`)
	if err != nil {
		return err
	}

	// Create geometry columns from coordinates
	_, err = osl.db.Exec(`
		UPDATE os_uprn_reference 
		SET geom27700 = ST_SetSRID(ST_MakePoint(x_coordinate::float8, y_coordinate::float8), 27700),
		    geom4326 = ST_SetSRID(ST_MakePoint(longitude::float8, latitude::float8), 4326)
		WHERE geom27700 IS NULL AND x_coordinate IS NOT NULL AND y_coordinate IS NOT NULL
	`)
	
	return err
}

// createOSUPRNIndexes creates performance indexes
func (osl *OSDataLoader) createOSUPRNIndexes(localDebug bool) error {
	debug.DebugOutput(localDebug, "Creating OS UPRN indexes")

	indexes := []string{
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS os_uprn_reference_uprn_idx ON os_uprn_reference (uprn)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS os_uprn_reference_geom27700_idx ON os_uprn_reference USING gist (geom27700)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS os_uprn_reference_geom4326_idx ON os_uprn_reference USING gist (geom4326)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS os_uprn_reference_coords_idx ON os_uprn_reference (x_coordinate, y_coordinate)",
	}

	for _, indexSQL := range indexes {
		_, err := osl.db.Exec(indexSQL)
		if err != nil {
			debug.DebugOutput(localDebug, "Warning: failed to create index: %v", err)
		}
	}

	return nil
}

// ValidateLegacyUPRNs validates source document UPRNs against OS Open UPRN dataset
func (osl *OSDataLoader) ValidateLegacyUPRNs(localDebug bool) (*ValidationReport, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	debug.DebugOutput(localDebug, "Validating legacy UPRNs against OS Open UPRN dataset")

	// Query for validation statistics
	var report ValidationReport

	// Total source documents with UPRNs
	err := osl.db.QueryRow(`
		SELECT COUNT(*) 
		FROM src_document 
		WHERE uprn_raw IS NOT NULL AND TRIM(uprn_raw) != ''
	`).Scan(&report.TotalWithUPRN)
	if err != nil {
		return nil, err
	}

	// UPRNs found in EHDC LLPG
	err = osl.db.QueryRow(`
		SELECT COUNT(*) 
		FROM src_document s
		INNER JOIN dim_address d ON d.uprn = TRIM(s.uprn_raw)
		WHERE s.uprn_raw IS NOT NULL AND TRIM(s.uprn_raw) != ''
	`).Scan(&report.ValidInEHDCLLPG)
	if err != nil {
		return nil, err
	}

	// UPRNs found in OS Open UPRN (but not in EHDC LLPG)
	err = osl.db.QueryRow(`
		SELECT COUNT(*) 
		FROM src_document s
		LEFT JOIN dim_address d ON d.uprn = TRIM(s.uprn_raw)
		INNER JOIN os_uprn_reference o ON o.uprn = TRIM(s.uprn_raw)
		WHERE s.uprn_raw IS NOT NULL 
		  AND TRIM(s.uprn_raw) != ''
		  AND d.uprn IS NULL
	`).Scan(&report.ValidInOSOnly)
	if err != nil {
		return nil, err
	}

	// Invalid UPRNs (not found in either dataset)
	report.Invalid = report.TotalWithUPRN - report.ValidInEHDCLLPG - report.ValidInOSOnly

	debug.DebugOutput(localDebug, "UPRN Validation Report:")
	debug.DebugOutput(localDebug, "  Total with UPRN: %d", report.TotalWithUPRN)
	debug.DebugOutput(localDebug, "  Valid in EHDC LLPG: %d", report.ValidInEHDCLLPG) 
	debug.DebugOutput(localDebug, "  Valid in OS only: %d", report.ValidInOSOnly)
	debug.DebugOutput(localDebug, "  Invalid: %d", report.Invalid)

	return &report, nil
}

// EnrichCoordinates enriches EHDC addresses with coordinates from OS data where missing
func (osl *OSDataLoader) EnrichCoordinates(localDebug bool) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	debug.DebugOutput(localDebug, "Enriching EHDC addresses with OS coordinates")

	// Update dim_address with coordinates from OS data where EHDC data is missing or different
	result, err := osl.db.Exec(`
		UPDATE dim_address 
		SET 
			easting = o.x_coordinate,
			northing = o.y_coordinate,
			geom27700 = o.geom27700,
			geom4326 = o.geom4326
		FROM os_uprn_reference o
		WHERE dim_address.uprn = o.uprn
		  AND (dim_address.easting IS NULL 
		       OR dim_address.northing IS NULL
		       OR dim_address.geom27700 IS NULL)
	`)
	if err != nil {
		return fmt.Errorf("failed to enrich coordinates: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	debug.DebugOutput(localDebug, "Enriched %d addresses with OS coordinates", rowsAffected)

	return nil
}

// Helper methods
func (osl *OSDataLoader) getColumnValue(record []string, columnMap map[string]int, columnName string) string {
	if idx, exists := columnMap[strings.ToLower(columnName)]; exists && idx < len(record) {
		return strings.TrimSpace(record[idx])
	}
	return ""
}

func (osl *OSDataLoader) parseNullableFloat(s string) interface{} {
	if s == "" {
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return nil
}

// ValidationReport holds UPRN validation statistics
type ValidationReport struct {
	TotalWithUPRN    int `json:"total_with_uprn"`
	ValidInEHDCLLPG  int `json:"valid_in_ehdc_llpg"`
	ValidInOSOnly    int `json:"valid_in_os_only"`
	Invalid          int `json:"invalid"`
}