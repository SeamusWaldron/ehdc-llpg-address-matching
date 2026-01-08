# Chapter 8: Data Pipeline and ETL

## 8.1 Pipeline Overview

The Extract-Transform-Load (ETL) pipeline ingests data from CSV files, transforms addresses into canonical form, and loads them into the dimensional schema. The pipeline handles both authoritative reference data (LLPG, OS UPRN) and historic source documents.

```
CSV Files           Staging            Transform           Dimension
+------------+     +------------+     +------------+     +------------+
| LLPG       | --> | stg_llpg   | --> | Normalise  | --> | dim_address|
| OS UPRN    | --> | stg_os_uprn| --> | Validate   | --> | dim_location|
| Sources    | --> | stg_*      | --> | Parse      | --> | src_document|
+------------+     +------------+     +------------+     +------------+
```

## 8.2 LLPG Loading

### 8.2.1 Pipeline Implementation

The LLPG loader in `internal/etl/pipeline.go`:

```go
type Pipeline struct {
    db *sql.DB
}

func NewPipeline(db *sql.DB) *Pipeline {
    return &Pipeline{db: db}
}

func (p *Pipeline) LoadLLPG(localDebug bool, csvPath string) error {
    debug.DebugHeader(localDebug)
    defer debug.DebugFooter(localDebug)

    file, err := os.Open(csvPath)
    if err != nil {
        return fmt.Errorf("failed to open LLPG file: %w", err)
    }
    defer file.Close()

    reader := csv.NewReader(file)
    reader.FieldsPerRecord = -1  // Variable field count

    // Read header
    header, err := reader.Read()
    if err != nil {
        return fmt.Errorf("failed to read header: %w", err)
    }

    // Map column positions
    colMap := makeColumnMap(header)

    // Begin transaction
    tx, err := p.db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    // Prepare insert statement
    stmt, err := tx.Prepare(`
        INSERT INTO dim_address (
            uprn, full_address, address_canonical, usrn,
            blpu_class, postal_flag, status_code,
            easting, northing
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (uprn) DO UPDATE SET
            full_address = EXCLUDED.full_address,
            address_canonical = EXCLUDED.address_canonical
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare statement: %w", err)
    }
    defer stmt.Close()

    count := 0
    for {
        record, err := reader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            continue  // Skip malformed rows
        }

        // Extract fields
        uprn := getField(record, colMap, "bs7666uprn")
        address := getField(record, colMap, "locaddress")
        usrn := getField(record, colMap, "bs7666usrn")
        blpuClass := getField(record, colMap, "blpuclass")
        postal := getField(record, colMap, "postal")
        status := getField(record, colMap, "lgcstatusc")
        eastingStr := getField(record, colMap, "easting")
        northingStr := getField(record, colMap, "northing")

        // Normalise address
        canonical, _, _ := normalize.CanonicalAddress(address)

        // Parse coordinates
        easting, _ := strconv.ParseFloat(eastingStr, 64)
        northing, _ := strconv.ParseFloat(northingStr, 64)

        // Insert
        _, err = stmt.Exec(
            uprn, address, canonical, usrn,
            blpuClass, postal == "Y", status,
            easting, northing,
        )
        if err != nil {
            debug.DebugOutput(localDebug, "Insert error for UPRN %s: %v", uprn, err)
            continue
        }

        count++
        if count%10000 == 0 {
            fmt.Printf("Loaded %d LLPG records...\n", count)
        }
    }

    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }

    fmt.Printf("Successfully loaded %d LLPG records\n", count)
    return nil
}
```

### 8.2.2 LLPG CSV Format

The LLPG CSV contains these columns:

| Column | Description | Example |
|--------|-------------|---------|
| ogc_fid | Feature ID | 12345 |
| locaddress | Full address | 12 HIGH STREET, ALTON, GU34 1AB |
| easting | BNG X coordinate | 471234 |
| northing | BNG Y coordinate | 139567 |
| lgcstatusc | Status code | 1 |
| bs7666uprn | UPRN | 100012345678 |
| bs7666usrn | USRN | 12345678 |
| landparcel | Land parcel ref | LP123 |
| blpuclass | Property class | RD |
| postal | Postal flag | Y |

## 8.3 OS Open UPRN Loading

The OS Open UPRN dataset contains 41+ million records requiring batch processing:

```go
type OSDataLoader struct {
    db *sql.DB
}

func NewOSDataLoader(db *sql.DB) *OSDataLoader {
    return &OSDataLoader{db: db}
}

func (l *OSDataLoader) LoadOSOpenUPRN(localDebug bool, csvPath string, batchSize int) error {
    debug.DebugHeader(localDebug)
    defer debug.DebugFooter(localDebug)

    file, err := os.Open(csvPath)
    if err != nil {
        return fmt.Errorf("failed to open OS UPRN file: %w", err)
    }
    defer file.Close()

    reader := csv.NewReader(file)

    // Skip header
    _, err = reader.Read()
    if err != nil {
        return fmt.Errorf("failed to read header: %w", err)
    }

    batch := make([][]string, 0, batchSize)
    totalCount := 0
    batchCount := 0

    for {
        record, err := reader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            continue
        }

        batch = append(batch, record)

        if len(batch) >= batchSize {
            if err := l.insertBatch(batch); err != nil {
                fmt.Printf("Batch %d failed: %v\n", batchCount, err)
            }
            totalCount += len(batch)
            batchCount++

            fmt.Printf("Processed %d records (%d batches)...\n",
                totalCount, batchCount)

            batch = batch[:0]  // Reset batch
        }
    }

    // Insert remaining records
    if len(batch) > 0 {
        if err := l.insertBatch(batch); err != nil {
            fmt.Printf("Final batch failed: %v\n", err)
        }
        totalCount += len(batch)
    }

    fmt.Printf("Successfully loaded %d OS UPRN records\n", totalCount)
    return nil
}

func (l *OSDataLoader) insertBatch(records [][]string) error {
    tx, err := l.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    stmt, err := tx.Prepare(`
        INSERT INTO dim_location (uprn, easting, northing, latitude, longitude, source_dataset)
        VALUES ($1, $2, $3, $4, $5, 'os_uprn')
        ON CONFLICT (uprn) DO UPDATE SET
            easting = COALESCE(dim_location.easting, EXCLUDED.easting),
            northing = COALESCE(dim_location.northing, EXCLUDED.northing),
            latitude = COALESCE(dim_location.latitude, EXCLUDED.latitude),
            longitude = COALESCE(dim_location.longitude, EXCLUDED.longitude)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, record := range records {
        if len(record) < 5 {
            continue
        }

        uprn := record[0]
        easting, _ := strconv.ParseFloat(record[1], 64)
        northing, _ := strconv.ParseFloat(record[2], 64)
        latitude, _ := strconv.ParseFloat(record[3], 64)
        longitude, _ := strconv.ParseFloat(record[4], 64)

        _, err := stmt.Exec(uprn, easting, northing, latitude, longitude)
        if err != nil {
            continue  // Skip individual failures
        }
    }

    return tx.Commit()
}
```

### 8.3.1 Batch Processing Performance

| Batch Size | Memory Usage | Throughput | Total Time (41M) |
|------------|--------------|------------|------------------|
| 10,000 | ~50MB | 80,000/min | ~8.5 hours |
| 50,000 | ~200MB | 120,000/min | ~5.7 hours |
| 100,000 | ~400MB | 150,000/min | ~4.5 hours |

The default batch size of 50,000 balances memory usage and throughput.

## 8.4 Source Document Loading

### 8.4.1 Unified Loading Function

```go
func (p *Pipeline) LoadSourceDocuments(localDebug bool, sourceType, csvPath string) error {
    debug.DebugHeader(localDebug)
    defer debug.DebugFooter(localDebug)

    file, err := os.Open(csvPath)
    if err != nil {
        return fmt.Errorf("failed to open source file: %w", err)
    }
    defer file.Close()

    reader := csv.NewReader(file)
    reader.FieldsPerRecord = -1

    header, err := reader.Read()
    if err != nil {
        return fmt.Errorf("failed to read header: %w", err)
    }

    colMap := makeColumnMap(header)

    // Get document type ID
    var docTypeID int
    err = p.db.QueryRow(`
        SELECT doc_type_id FROM dim_document_type WHERE type_code = $1
    `, sourceType).Scan(&docTypeID)
    if err != nil {
        return fmt.Errorf("unknown source type: %s", sourceType)
    }

    tx, err := p.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    stmt, err := tx.Prepare(`
        INSERT INTO src_document (
            doc_type_id, job_number, filepath, external_reference,
            document_date, raw_address, address_canonical,
            raw_uprn, raw_easting, raw_northing
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    count := 0
    for {
        record, err := reader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            continue
        }

        // Extract common fields
        jobNumber := getField(record, colMap, "job_number")
        filepath := getField(record, colMap, "filepath")
        rawUPRN := getField(record, colMap, "bs7666uprn")
        easting := getField(record, colMap, "easting")
        northing := getField(record, colMap, "northing")

        // Extract type-specific fields
        var rawAddress, externalRef, dateStr string
        switch sourceType {
        case "decision":
            rawAddress = getField(record, colMap, "adress")  // Note typo
            externalRef = getField(record, colMap, "planning_application_number")
            dateStr = getField(record, colMap, "decision_date")
        case "land_charge":
            rawAddress = getField(record, colMap, "address")
            externalRef = getField(record, colMap, "card_code")
        case "enforcement":
            rawAddress = getField(record, colMap, "address")
            externalRef = getField(record, colMap, "planning_enforcement_reference_number")
            dateStr = getField(record, colMap, "date")
        case "agreement":
            rawAddress = getField(record, colMap, "address")
            dateStr = getField(record, colMap, "date")
        }

        if rawAddress == "" {
            continue  // Skip records without addresses
        }

        // Normalise address
        canonical, _, _ := normalize.CanonicalAddress(rawAddress)

        // Parse date
        var docDate *time.Time
        if dateStr != "" {
            if t, err := parseUKDate(dateStr); err == nil {
                docDate = &t
            }
        }

        _, err = stmt.Exec(
            docTypeID, jobNumber, filepath, externalRef,
            docDate, rawAddress, canonical,
            rawUPRN, easting, northing,
        )
        if err != nil {
            continue
        }

        count++
        if count%10000 == 0 {
            fmt.Printf("Loaded %d %s records...\n", count, sourceType)
        }
    }

    if err := tx.Commit(); err != nil {
        return err
    }

    fmt.Printf("Successfully loaded %d %s records\n", count, sourceType)
    return nil
}
```

### 8.4.2 Date Parsing

UK dates require special handling:

```go
func parseUKDate(dateStr string) (time.Time, error) {
    formats := []string{
        "02/01/2006",  // DD/MM/YYYY
        "2/1/2006",    // D/M/YYYY
        "02/01/06",    // DD/MM/YY
        "2/1/06",      // D/M/YY
        "2006-01-02",  // ISO format
    }

    for _, format := range formats {
        if t, err := time.Parse(format, dateStr); err == nil {
            return t, nil
        }
    }

    return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}
```

## 8.5 UPRN Validation and Enrichment

### 8.5.1 Legacy UPRN Validation

```go
func (l *OSDataLoader) ValidateLegacyUPRNs(localDebug bool) (*ValidationReport, error) {
    report := &ValidationReport{}

    // Count total documents with UPRNs
    err := l.db.QueryRow(`
        SELECT COUNT(*) FROM src_document WHERE raw_uprn IS NOT NULL AND raw_uprn != ''
    `).Scan(&report.TotalWithUPRN)
    if err != nil {
        return nil, err
    }

    // Count valid in EHDC LLPG
    err = l.db.QueryRow(`
        SELECT COUNT(DISTINCT s.document_id)
        FROM src_document s
        JOIN dim_address d ON REPLACE(s.raw_uprn, '.00', '') = d.uprn
        WHERE s.raw_uprn IS NOT NULL AND s.raw_uprn != ''
    `).Scan(&report.ValidInEHDCLLPG)
    if err != nil {
        return nil, err
    }

    // Count valid in OS data only
    err = l.db.QueryRow(`
        SELECT COUNT(DISTINCT s.document_id)
        FROM src_document s
        JOIN dim_location l ON REPLACE(s.raw_uprn, '.00', '') = l.uprn
        LEFT JOIN dim_address d ON REPLACE(s.raw_uprn, '.00', '') = d.uprn
        WHERE s.raw_uprn IS NOT NULL
          AND s.raw_uprn != ''
          AND d.uprn IS NULL
    `).Scan(&report.ValidInOSOnly)
    if err != nil {
        return nil, err
    }

    report.Invalid = report.TotalWithUPRN - report.ValidInEHDCLLPG - report.ValidInOSOnly

    return report, nil
}
```

### 8.5.2 Coordinate Enrichment

```go
func (l *OSDataLoader) EnrichCoordinates(localDebug bool) error {
    result, err := l.db.Exec(`
        UPDATE dim_address d
        SET
            easting = COALESCE(d.easting, l.easting),
            northing = COALESCE(d.northing, l.northing)
        FROM dim_location l
        WHERE d.uprn = l.uprn
          AND (d.easting IS NULL OR d.northing IS NULL)
          AND l.easting IS NOT NULL
          AND l.northing IS NOT NULL
    `)
    if err != nil {
        return err
    }

    affected, _ := result.RowsAffected()
    fmt.Printf("Enriched coordinates for %d addresses\n", affected)

    return nil
}
```

## 8.6 Address Standardisation

### 8.6.1 libpostal Integration

```go
func (p *Pipeline) StandardiseAddresses(localDebug bool) error {
    rows, err := p.db.Query(`
        SELECT document_id, raw_address
        FROM src_document
        WHERE gopostal_road IS NULL
          AND raw_address IS NOT NULL
          AND raw_address != ''
        LIMIT 10000
    `)
    if err != nil {
        return err
    }
    defer rows.Close()

    client := &http.Client{Timeout: 5 * time.Second}

    for rows.Next() {
        var docID int
        var rawAddress string
        if err := rows.Scan(&docID, &rawAddress); err != nil {
            continue
        }

        // Call libpostal service
        components, err := parseWithLibpostal(client, rawAddress)
        if err != nil {
            continue
        }

        // Update document
        _, err = p.db.Exec(`
            UPDATE src_document SET
                gopostal_house_number = $1,
                gopostal_road = $2,
                gopostal_city = $3,
                gopostal_postcode = $4
            WHERE document_id = $5
        `, components.HouseNumber, components.Road,
           components.City, components.Postcode, docID)

        if err != nil {
            debug.DebugOutput(localDebug, "Update failed for doc %d: %v", docID, err)
        }
    }

    return nil
}
```

## 8.7 Group Consensus Corrections

### 8.7.1 Planning Application Grouping

Documents with the same planning application number should have the same address:

```go
func (p *Pipeline) ApplyGroupConsensusCorrections(localDebug bool) error {
    // Find groups with mixed match status
    rows, err := p.db.Query(`
        SELECT external_reference, address_canonical
        FROM src_document
        WHERE doc_type_id = 1  -- Decision notices
          AND external_reference IS NOT NULL
        GROUP BY external_reference, address_canonical
        HAVING COUNT(*) > 1
    `)
    if err != nil {
        return err
    }
    defer rows.Close()

    correctionCount := 0

    for rows.Next() {
        var extRef, addrCan string
        if err := rows.Scan(&extRef, &addrCan); err != nil {
            continue
        }

        // Find consensus UPRN for this group
        var consensusUPRN string
        var consensusScore float64

        err := p.db.QueryRow(`
            SELECT ma.uprn, ma.confidence
            FROM src_document s
            JOIN match_accepted ma ON s.document_id = ma.src_id
            WHERE s.external_reference = $1
              AND ma.confidence >= 0.85
            ORDER BY ma.confidence DESC
            LIMIT 1
        `, extRef).Scan(&consensusUPRN, &consensusScore)

        if err != nil {
            continue  // No high-confidence match in group
        }

        // Apply consensus to unmatched documents in group
        result, err := p.db.Exec(`
            INSERT INTO address_match_corrected (
                document_id, matched_uprn, correction_method, confidence_score
            )
            SELECT s.document_id, $1, 'group_consensus', $2
            FROM src_document s
            LEFT JOIN match_accepted ma ON s.document_id = ma.src_id
            WHERE s.external_reference = $3
              AND ma.src_id IS NULL
        `, consensusUPRN, consensusScore, extRef)

        if err != nil {
            continue
        }

        affected, _ := result.RowsAffected()
        correctionCount += int(affected)
    }

    fmt.Printf("Applied %d group consensus corrections\n", correctionCount)
    return nil
}
```

## 8.8 LLM-Powered Address Correction

### 8.8.1 Low Confidence Address Fixing

```go
func (p *Pipeline) LLMFixLowConfidenceAddresses(localDebug bool) error {
    // Find addresses with formatting issues
    rows, err := p.db.Query(`
        SELECT s.document_id, s.raw_address
        FROM src_document s
        JOIN match_accepted ma ON s.document_id = ma.src_id
        WHERE ma.confidence <= 0.4
          AND s.raw_address ~ '^[0-9]+,'  -- Starts with number,comma
        LIMIT 100
    `)
    if err != nil {
        return err
    }
    defer rows.Close()

    ollamaURL := config.GetEnv("OLLAMA_HOST", "http://localhost:11434")

    for rows.Next() {
        var docID int
        var rawAddress string
        if err := rows.Scan(&docID, &rawAddress); err != nil {
            continue
        }

        // Call Ollama for address correction
        corrected, err := correctAddressWithLLM(ollamaURL, rawAddress)
        if err != nil {
            debug.DebugOutput(localDebug, "LLM correction failed: %v", err)
            continue
        }

        // Validate corrected address against LLPG
        var matchedUPRN string
        var matchScore float64

        err = p.db.QueryRow(`
            SELECT uprn, similarity($1, address_canonical) as score
            FROM dim_address
            WHERE address_canonical % $1
            ORDER BY score DESC
            LIMIT 1
        `, corrected).Scan(&matchedUPRN, &matchScore)

        if err != nil || matchScore < 0.75 {
            continue  // Correction didn't improve match
        }

        // Save correction
        _, err = p.db.Exec(`
            INSERT INTO address_match_corrected (
                document_id, original_address, corrected_address,
                matched_uprn, correction_method, confidence_score
            ) VALUES ($1, $2, $3, $4, 'llm_correction', $5)
        `, docID, rawAddress, corrected, matchedUPRN, matchScore)

        if err != nil {
            debug.DebugOutput(localDebug, "Failed to save correction: %v", err)
        }
    }

    return nil
}

func correctAddressWithLLM(ollamaURL, address string) (string, error) {
    prompt := fmt.Sprintf(`Fix this UK address formatting. Only output the corrected address, nothing else.

Input: %s
Output:`, address)

    payload := map[string]interface{}{
        "model":  "llama3.2:1b",
        "prompt": prompt,
        "stream": false,
    }

    jsonData, _ := json.Marshal(payload)

    resp, err := http.Post(
        ollamaURL+"/api/generate",
        "application/json",
        bytes.NewBuffer(jsonData),
    )
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        Response string `json:"response"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }

    return strings.TrimSpace(result.Response), nil
}
```

## 8.9 Fact Table Rebuild

### 8.9.1 Incorporating Corrections

```go
func (p *Pipeline) RebuildFactTable(localDebug bool) error {
    // Clear existing fact table
    _, err := p.db.Exec("TRUNCATE TABLE fact_documents_lean")
    if err != nil {
        return err
    }

    // Rebuild with all corrections applied
    _, err = p.db.Exec(`
        INSERT INTO fact_documents_lean (
            document_id, doc_type_id, matched_uprn, match_method,
            confidence_score, easting, northing, processing_version
        )
        SELECT
            s.document_id,
            s.doc_type_id,
            COALESCE(c.matched_uprn, ma.uprn) as matched_uprn,
            COALESCE(c.correction_method, ma.method) as match_method,
            COALESCE(c.confidence_score, ma.confidence) as confidence_score,
            COALESCE(d.easting, s.raw_easting::numeric) as easting,
            COALESCE(d.northing, s.raw_northing::numeric) as northing,
            '1.1-with-corrections' as processing_version
        FROM src_document s
        LEFT JOIN match_accepted ma ON s.document_id = ma.src_id
        LEFT JOIN address_match_corrected c ON s.document_id = c.document_id
        LEFT JOIN dim_address d ON COALESCE(c.matched_uprn, ma.uprn) = d.uprn
    `)

    if err != nil {
        return fmt.Errorf("failed to rebuild fact table: %w", err)
    }

    // Get statistics
    var total, matched int
    p.db.QueryRow("SELECT COUNT(*) FROM fact_documents_lean").Scan(&total)
    p.db.QueryRow("SELECT COUNT(*) FROM fact_documents_lean WHERE matched_uprn IS NOT NULL").Scan(&matched)

    fmt.Printf("Fact table rebuilt: %d total, %d matched (%.1f%%)\n",
        total, matched, float64(matched)/float64(total)*100)

    return nil
}
```

## 8.10 Chapter Summary

This chapter has documented the ETL pipeline:

- LLPG loading with normalisation
- OS Open UPRN batch loading for 41+ million records
- Source document loading for all nine document types
- UPRN validation and coordinate enrichment
- Address standardisation with libpostal
- Group consensus corrections
- LLM-powered address correction
- Fact table rebuild with all corrections

The pipeline ensures data quality whilst maintaining full traceability of all transformations.

---

*This chapter covers data loading and transformation. Chapter 9 describes the web interface.*
