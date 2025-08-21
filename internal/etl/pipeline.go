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
	"github.com/ehdc-llpg/internal/normalize"
)

// Pipeline handles ETL operations following PROJECT_SPECIFICATION.md
type Pipeline struct {
	db *sql.DB
}

// NewPipeline creates a new ETL pipeline
func NewPipeline(db *sql.DB) *Pipeline {
	return &Pipeline{db: db}
}

// LoadLLPG loads LLPG data into staging and dimension tables
func (p *Pipeline) LoadLLPG(localDebug bool, csvPath string) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	debug.DebugOutput(localDebug, "Loading LLPG from: %s", csvPath)

	// Clear existing data
	_, err := p.db.Exec("TRUNCATE TABLE stg_llpg CASCADE")
	if err != nil {
		return fmt.Errorf("failed to truncate stg_llpg: %w", err)
	}

	_, err = p.db.Exec("TRUNCATE TABLE dim_address CASCADE")
	if err != nil {
		return fmt.Errorf("failed to truncate dim_address: %w", err)
	}

	// Load staging data
	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("failed to open LLPG CSV: %w", err)
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
		columnMap[strings.ToLower(col)] = i
	}

	// Prepare staging insert
	stagingStmt, err := p.db.Prepare(`
		INSERT INTO stg_llpg (
			ogc_fid, locaddress, easting, northing, lgcstatusc,
			bs7666uprn, bs7666usrn, landparcel, blpuclass, postal
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare staging statement: %w", err)
	}
	defer stagingStmt.Close()

	// Process records
	recordCount := 0
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
		ogcFid := p.getColumnValue(record, columnMap, "ogc_fid")
		locAddress := p.getColumnValue(record, columnMap, "locaddress")
		easting := p.getColumnValue(record, columnMap, "easting")
		northing := p.getColumnValue(record, columnMap, "northing")
		lgcStatusC := p.getColumnValue(record, columnMap, "lgcstatusc")
		bs7666UPRN := p.getColumnValue(record, columnMap, "bs7666uprn")
		bs7666USRN := p.getColumnValue(record, columnMap, "bs7666usrn")
		landParcel := p.getColumnValue(record, columnMap, "landparcel")
		blpuClass := p.getColumnValue(record, columnMap, "blpuclass")
		postal := p.getColumnValue(record, columnMap, "postal")

		// Insert into staging
		_, err = stagingStmt.Exec(
			p.parseNullableInt(ogcFid),
			p.nullIfEmpty(locAddress),
			p.parseNullableFloat(easting),
			p.parseNullableFloat(northing),
			p.nullIfEmpty(lgcStatusC),
			p.nullIfEmpty(bs7666UPRN),
			p.nullIfEmpty(bs7666USRN),
			p.nullIfEmpty(landParcel),
			p.nullIfEmpty(blpuClass),
			p.nullIfEmpty(postal),
		)
		if err != nil {
			debug.DebugOutput(localDebug, "Error inserting staging record %d: %v", recordCount, err)
			continue
		}

		recordCount++
		if recordCount%1000 == 0 {
			debug.DebugOutput(localDebug, "Loaded %d staging records", recordCount)
		}
	}

	debug.DebugOutput(localDebug, "Loaded %d total staging records", recordCount)

	// Transform to dimension table
	return p.transformLLPGToDimension(localDebug)
}

// transformLLPGToDimension transforms staging LLPG data to dim_address
func (p *Pipeline) transformLLPGToDimension(localDebug bool) error {
	debug.DebugOutput(localDebug, "Transforming LLPG staging data to dimension table")

	// Transform with canonical address generation (PostGIS temporarily disabled)
	_, err := p.db.Exec(`
		INSERT INTO dim_address (
			uprn, locaddress, easting, northing, usrn, blpu_class, postal_flag
		)
		SELECT 
			bs7666uprn,
			locaddress,
			easting,
			northing,
			bs7666usrn,
			blpuclass,
			postal
		FROM stg_llpg
		WHERE bs7666uprn IS NOT NULL 
		  AND bs7666uprn != ''
		  AND easting IS NOT NULL
		  AND northing IS NOT NULL
		  AND locaddress IS NOT NULL
		  AND locaddress != ''
	`)

	if err != nil {
		return fmt.Errorf("failed to transform LLPG to dimension: %w", err)
	}

	// Get count of transformed records
	var count int
	err = p.db.QueryRow("SELECT COUNT(*) FROM dim_address").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count dimension records: %w", err)
	}

	debug.DebugOutput(localDebug, "Transformed %d records to dim_address", count)

	// Create indexes if they don't exist
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS dim_address_addr_can_trgm_idx ON dim_address USING gin (addr_can gin_trgm_ops)",
		// "CREATE INDEX IF NOT EXISTS dim_address_geom27700_idx ON dim_address USING gist (geom27700)", // PostGIS disabled
		// "CREATE INDEX IF NOT EXISTS dim_address_geom4326_idx ON dim_address USING gist (geom4326)", // PostGIS disabled
		"CREATE INDEX IF NOT EXISTS dim_address_uprn_idx ON dim_address (uprn)",
		"CREATE INDEX IF NOT EXISTS dim_address_usrn_idx ON dim_address (usrn)",
	}

	for _, indexSQL := range indexes {
		_, err = p.db.Exec(indexSQL)
		if err != nil {
			debug.DebugOutput(localDebug, "Warning: failed to create index: %v", err)
		}
	}

	debug.DebugOutput(localDebug, "Created dimension table indexes")

	return nil
}

// LoadSourceDocuments loads source document CSV files into staging and src_document tables
func (p *Pipeline) LoadSourceDocuments(localDebug bool, sourceType, csvPath string) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	debug.DebugOutput(localDebug, "Loading source documents: %s from %s", sourceType, csvPath)

	// Load to appropriate staging table first
	err := p.loadToStaging(localDebug, sourceType, csvPath)
	if err != nil {
		return err
	}

	// Transform to src_document
	return p.transformSourceToDocument(localDebug, sourceType)
}

// loadToStaging loads CSV to appropriate staging table
func (p *Pipeline) loadToStaging(localDebug bool, sourceType, csvPath string) error {
	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("failed to open CSV: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	
	// Read header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	debug.DebugOutput(localDebug, "Source CSV columns: %v", header)

	// Get appropriate staging table and statement
	stagingTable, stmt, err := p.getStagingInsertStatement(sourceType)
	if err != nil {
		return err
	}
	defer stmt.Close()

	debug.DebugOutput(localDebug, "Loading to staging table: %s", stagingTable)

	// Create column mapping
	columnMap := make(map[string]int)
	for i, col := range header {
		columnMap[strings.ToLower(col)] = i
	}

	// Process records
	recordCount := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			debug.DebugOutput(localDebug, "Error reading CSV record %d: %v", recordCount, err)
			continue
		}

		// Extract values based on source type
		err = p.insertStagingRecord(sourceType, stmt, record, columnMap)
		if err != nil {
			debug.DebugOutput(localDebug, "Error inserting staging record %d: %v", recordCount, err)
			continue
		}

		recordCount++
		if recordCount%1000 == 0 {
			debug.DebugOutput(localDebug, "Loaded %d staging records", recordCount)
		}
	}

	debug.DebugOutput(localDebug, "Loaded %d total records to %s", recordCount, stagingTable)
	return nil
}

// getStagingInsertStatement returns the appropriate staging table and prepared statement
func (p *Pipeline) getStagingInsertStatement(sourceType string) (string, *sql.Stmt, error) {
	switch sourceType {
	case "decision":
		stmt, err := p.db.Prepare(`
			INSERT INTO stg_decision_notices (
				job_number, filepath, planning_application_number, adress,
				decision_date, decision_type, document_type, bs7666uprn, easting, northing
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`)
		return "stg_decision_notices", stmt, err

	case "land_charge":
		stmt, err := p.db.Prepare(`
			INSERT INTO stg_land_charges_cards (
				job_number, filepath, card_code, address, bs7666uprn, easting, northing
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
		`)
		return "stg_land_charges_cards", stmt, err

	case "enforcement":
		stmt, err := p.db.Prepare(`
			INSERT INTO stg_enforcement_notices (
				job_number, filepath, planning_enforcement_reference_number, address,
				date, document_type, bs7666uprn, easting, northing
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`)
		return "stg_enforcement_notices", stmt, err

	case "agreement":
		stmt, err := p.db.Prepare(`
			INSERT INTO stg_agreements (
				job_number, filepath, address, date, bs7666uprn, easting, northing
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
		`)
		return "stg_agreements", stmt, err

	default:
		return "", nil, fmt.Errorf("unknown source type: %s", sourceType)
	}
}

// insertStagingRecord inserts a record into the appropriate staging table
func (p *Pipeline) insertStagingRecord(sourceType string, stmt *sql.Stmt, record []string, columnMap map[string]int) error {
	switch sourceType {
	case "decision":
		return p.insertDecisionRecord(stmt, record, columnMap)
	case "land_charge":
		return p.insertLandChargeRecord(stmt, record, columnMap)
	case "enforcement":
		return p.insertEnforcementRecord(stmt, record, columnMap)
	case "agreement":
		return p.insertAgreementRecord(stmt, record, columnMap)
	default:
		return fmt.Errorf("unknown source type: %s", sourceType)
	}
}

// insertDecisionRecord inserts a decision notice record
func (p *Pipeline) insertDecisionRecord(stmt *sql.Stmt, record []string, columnMap map[string]int) error {
	jobNumber := p.getColumnValue(record, columnMap, "job number")
	filepath := p.getColumnValue(record, columnMap, "filepath")
	planningAppNum := p.getColumnValue(record, columnMap, "planning application number")
	address := p.getColumnValue(record, columnMap, "adress") // Note: typo preserved from spec
	decisionDate := p.getColumnValue(record, columnMap, "decision date")
	decisionType := p.getColumnValue(record, columnMap, "decision type")
	documentType := p.getColumnValue(record, columnMap, "document type")
	uprn := p.getColumnValue(record, columnMap, "bs7666uprn")
	easting := p.getColumnValue(record, columnMap, "easting")
	northing := p.getColumnValue(record, columnMap, "northing")

	_, err := stmt.Exec(
		p.nullIfEmpty(jobNumber),
		p.nullIfEmpty(filepath),
		p.nullIfEmpty(planningAppNum),
		p.nullIfEmpty(address),
		p.nullIfEmpty(decisionDate),
		p.nullIfEmpty(decisionType),
		p.nullIfEmpty(documentType),
		p.nullIfEmpty(uprn),
		p.nullIfEmpty(easting),
		p.nullIfEmpty(northing),
	)
	return err
}

// insertLandChargeRecord inserts a land charge record
func (p *Pipeline) insertLandChargeRecord(stmt *sql.Stmt, record []string, columnMap map[string]int) error {
	jobNumber := p.getColumnValue(record, columnMap, "job number")
	filepath := p.getColumnValue(record, columnMap, "filepath")
	cardCode := p.getColumnValue(record, columnMap, "card code")
	address := p.getColumnValue(record, columnMap, "address")
	uprn := p.getColumnValue(record, columnMap, "bs7666uprn")
	easting := p.getColumnValue(record, columnMap, "easting")
	northing := p.getColumnValue(record, columnMap, "northing")

	_, err := stmt.Exec(
		p.nullIfEmpty(jobNumber),
		p.nullIfEmpty(filepath),
		p.nullIfEmpty(cardCode),
		p.nullIfEmpty(address),
		p.nullIfEmpty(uprn),
		p.nullIfEmpty(easting),
		p.nullIfEmpty(northing),
	)
	return err
}

// insertEnforcementRecord inserts an enforcement notice record
func (p *Pipeline) insertEnforcementRecord(stmt *sql.Stmt, record []string, columnMap map[string]int) error {
	jobNumber := p.getColumnValue(record, columnMap, "job number")
	filepath := p.getColumnValue(record, columnMap, "filepath")
	enforcementRef := p.getColumnValue(record, columnMap, "planning enforcement reference number")
	address := p.getColumnValue(record, columnMap, "address")
	date := p.getColumnValue(record, columnMap, "date")
	documentType := p.getColumnValue(record, columnMap, "document type")
	uprn := p.getColumnValue(record, columnMap, "bs7666uprn")
	easting := p.getColumnValue(record, columnMap, "easting")
	northing := p.getColumnValue(record, columnMap, "northing")

	_, err := stmt.Exec(
		p.nullIfEmpty(jobNumber),
		p.nullIfEmpty(filepath),
		p.nullIfEmpty(enforcementRef),
		p.nullIfEmpty(address),
		p.nullIfEmpty(date),
		p.nullIfEmpty(documentType),
		p.nullIfEmpty(uprn),
		p.nullIfEmpty(easting),
		p.nullIfEmpty(northing),
	)
	return err
}

// insertAgreementRecord inserts an agreement record
func (p *Pipeline) insertAgreementRecord(stmt *sql.Stmt, record []string, columnMap map[string]int) error {
	jobNumber := p.getColumnValue(record, columnMap, "job number")
	filepath := p.getColumnValue(record, columnMap, "filepath")
	address := p.getColumnValue(record, columnMap, "address")
	date := p.getColumnValue(record, columnMap, "date")
	uprn := p.getColumnValue(record, columnMap, "bs7666uprn")
	easting := p.getColumnValue(record, columnMap, "easting")
	northing := p.getColumnValue(record, columnMap, "northing")

	_, err := stmt.Exec(
		p.nullIfEmpty(jobNumber),
		p.nullIfEmpty(filepath),
		p.nullIfEmpty(address),
		p.nullIfEmpty(date),
		p.nullIfEmpty(uprn),
		p.nullIfEmpty(easting),
		p.nullIfEmpty(northing),
	)
	return err
}

// transformSourceToDocument transforms staging data to src_document
func (p *Pipeline) transformSourceToDocument(localDebug bool, sourceType string) error {
	debug.DebugOutput(localDebug, "Transforming %s staging data to src_document", sourceType)

	var sql string
	switch sourceType {
	case "decision":
		sql = `
			INSERT INTO src_document (
				source_type, job_number, filepath, external_ref, doc_type, doc_date,
				raw_address, addr_can, postcode_text, uprn_raw, easting_raw, northing_raw
			)
			SELECT 
				'decision'::source_type,
				job_number,
				filepath,
				planning_application_number,
				document_type,
				CASE 
					WHEN decision_date ~ '^\d{1,2}/\d{1,2}/\d{4}$' 
					THEN CASE 
						WHEN (split_part(decision_date, '/', 2)::int > 12 OR split_part(decision_date, '/', 1)::int > 31) 
						THEN NULL  -- Invalid date
						ELSE to_date(decision_date, 'DD/MM/YYYY')
					END
					WHEN decision_date ~ '^\d{1,2}/\d{1,2}/\d{2}$' 
					THEN CASE 
						WHEN (split_part(decision_date, '/', 2)::int > 12 OR split_part(decision_date, '/', 1)::int > 31) 
						THEN NULL  -- Invalid date
						ELSE to_date(decision_date, 'DD/MM/YY')
					END
					ELSE NULL
				END,
				adress as raw_address,
				$1 as addr_can,
				$2 as postcode_text,
				bs7666uprn,
				easting,
				northing
			FROM stg_decision_notices
			WHERE adress IS NOT NULL AND adress != ''
		`
	case "land_charge":
		sql = `
			INSERT INTO src_document (
				source_type, job_number, filepath, external_ref, doc_type, doc_date,
				raw_address, addr_can, postcode_text, uprn_raw, easting_raw, northing_raw
			)
			SELECT 
				'land_charge'::source_type,
				job_number,
				filepath,
				card_code,
				'Land Charge Card',
				NULL,
				address as raw_address,
				$1 as addr_can,
				$2 as postcode_text,
				bs7666uprn,
				easting,
				northing
			FROM stg_land_charges_cards
			WHERE address IS NOT NULL AND address != ''
		`
	case "enforcement":
		sql = `
			INSERT INTO src_document (
				source_type, job_number, filepath, external_ref, doc_type, doc_date,
				raw_address, addr_can, postcode_text, uprn_raw, easting_raw, northing_raw
			)
			SELECT 
				'enforcement'::source_type,
				job_number,
				filepath,
				planning_enforcement_reference_number,
				document_type,
				CASE 
					WHEN date ~ '^\d{1,2}/\d{1,2}/\d{4}$' 
					THEN to_date(date, 'DD/MM/YYYY')
					WHEN date ~ '^\d{1,2}/\d{1,2}/\d{2}$' 
					THEN to_date(date, 'DD/MM/YY')
					ELSE NULL
				END,
				address as raw_address,
				$1 as addr_can,
				$2 as postcode_text,
				bs7666uprn,
				easting,
				northing
			FROM stg_enforcement_notices
			WHERE address IS NOT NULL AND address != ''
		`
	case "agreement":
		sql = `
			INSERT INTO src_document (
				source_type, job_number, filepath, external_ref, doc_type, doc_date,
				raw_address, addr_can, postcode_text, uprn_raw, easting_raw, northing_raw
			)
			SELECT 
				'agreement'::source_type,
				job_number,
				filepath,
				NULL,
				'Agreement',
				CASE 
					WHEN date ~ '^\d{1,2}/\d{1,2}/\d{4}$' 
					THEN to_date(date, 'DD/MM/YYYY')
					WHEN date ~ '^\d{1,2}/\d{1,2}/\d{2}$' 
					THEN to_date(date, 'DD/MM/YY')
					ELSE NULL
				END,
				address as raw_address,
				$1 as addr_can,
				$2 as postcode_text,
				bs7666uprn,
				easting,
				northing
			FROM stg_agreements
			WHERE address IS NOT NULL AND address != ''
		`
	default:
		return fmt.Errorf("unknown source type: %s", sourceType)
	}

	// We need to process row by row to generate canonical addresses
	// This is a simplified version - in practice, you'd batch this
	_, err := p.db.Exec(sql, "", "") // Placeholder for addr_can and postcode_text
	if err != nil {
		return fmt.Errorf("failed to transform %s to src_document: %w", sourceType, err)
	}

	// Update canonical addresses and postcodes
	err = p.updateCanonicalAddresses(localDebug, sourceType)
	if err != nil {
		return err
	}

	// Get count of transformed records
	var count int
	err = p.db.QueryRow("SELECT COUNT(*) FROM src_document WHERE source_type = $1", sourceType).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count transformed records: %w", err)
	}

	debug.DebugOutput(localDebug, "Transformed %d %s records to src_document", count, sourceType)
	return nil
}

// updateCanonicalAddresses updates addr_can and postcode_text fields
func (p *Pipeline) updateCanonicalAddresses(localDebug bool, sourceType string) error {
	debug.DebugOutput(localDebug, "Updating canonical addresses for %s", sourceType)

	rows, err := p.db.Query(`
		SELECT src_id, raw_address 
		FROM src_document 
		WHERE source_type = $1 
		  AND (addr_can IS NULL OR addr_can = '' OR postcode_text IS NULL OR postcode_text = '')
	`, sourceType)
	if err != nil {
		return err
	}
	defer rows.Close()

	stmt, err := p.db.Prepare(`
		UPDATE src_document 
		SET addr_can = $2, postcode_text = $3 
		WHERE src_id = $1
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	updated := 0
	for rows.Next() {
		var srcID int64
		var rawAddress string

		err := rows.Scan(&srcID, &rawAddress)
		if err != nil {
			continue
		}

		// Generate canonical address and extract postcode
		canonical, postcode, _ := normalize.CanonicalAddress(rawAddress)

		_, err = stmt.Exec(srcID, canonical, postcode)
		if err != nil {
			debug.DebugOutput(localDebug, "Error updating canonical address for src_id %d: %v", srcID, err)
			continue
		}

		updated++
		if updated%1000 == 0 {
			debug.DebugOutput(localDebug, "Updated %d canonical addresses", updated)
		}
	}

	debug.DebugOutput(localDebug, "Updated %d canonical addresses for %s", updated, sourceType)
	return nil
}

// Helper methods for data parsing

func (p *Pipeline) getColumnValue(record []string, columnMap map[string]int, columnName string) string {
	if idx, exists := columnMap[strings.ToLower(columnName)]; exists && idx < len(record) {
		return strings.TrimSpace(record[idx])
	}
	return ""
}

func (p *Pipeline) nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (p *Pipeline) parseNullableInt(s string) interface{} {
	if s == "" {
		return nil
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	return nil
}

func (p *Pipeline) parseNullableFloat(s string) interface{} {
	if s == "" {
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return nil
}