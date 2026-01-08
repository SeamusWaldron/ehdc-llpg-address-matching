# Chapter 4: Database Schema Design

## 4.1 Schema Overview

The database schema follows a dimensional modelling approach, separating raw staging data from normalised dimensions and fact tables. This design supports both operational matching and analytical reporting whilst maintaining a complete audit trail.

```
+------------------+     +------------------+     +------------------+
|  STAGING TABLES  | --> | DIMENSION TABLES | --> |   FACT TABLES    |
|  (Raw Imports)   |     |   (Normalised)   |     |   (Results)      |
+------------------+     +------------------+     +------------------+
| stg_ehdc_llpg    |     | dim_address      |     | match_result     |
| stg_os_uprn      |     | dim_location     |     | match_accepted   |
| stg_decision_*   |     | dim_document_type|     | match_override   |
| stg_land_charges |     | dim_match_method |     | match_run        |
| stg_enforcement  |     | src_document     |     | fact_documents   |
| stg_agreements   |     |                  |     |                  |
+------------------+     +------------------+     +------------------+
```

## 4.2 Required Extensions

The schema requires three PostgreSQL extensions:

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
```

- **PostGIS**: Provides spatial data types and functions for coordinate handling
- **pg_trgm**: Enables trigram-based fuzzy string matching
- **unaccent**: Removes diacritical marks for normalisation

## 4.3 Staging Tables

Staging tables preserve raw data exactly as received from CSV imports, enabling re-processing without re-import.

### 4.3.1 LLPG Staging

```sql
CREATE TABLE stg_ehdc_llpg (
    ogc_fid text,
    locaddress text,
    easting text,
    northing text,
    lgcstatusc text,
    bs7666uprn text,
    bs7666usrn text,
    landparcel text,
    blpuclass text,
    postal text,
    loaded_at timestamptz DEFAULT now()
);
```

**Column Descriptions**:
- `ogc_fid`: Original feature identifier
- `locaddress`: Full address string
- `easting`, `northing`: British National Grid coordinates
- `lgcstatusc`: Status code (1 = live, other = historic)
- `bs7666uprn`: Unique Property Reference Number
- `bs7666usrn`: Unique Street Reference Number
- `landparcel`: Land parcel reference
- `blpuclass`: Basic Land and Property Unit classification
- `postal`: Postal address flag (Y/N)

### 4.3.2 OS UPRN Staging

```sql
CREATE TABLE stg_os_uprn (
    uprn text,
    x_coordinate text,
    y_coordinate text,
    latitude text,
    longitude text,
    loaded_at timestamptz DEFAULT now()
);
```

This table holds OS Open UPRN data (41+ million records) used to validate and enrich LLPG coordinates.

### 4.3.3 Source Document Staging

Each source type has a dedicated staging table:

**Decision Notices**:
```sql
CREATE TABLE stg_decision_notices (
    job_number text,
    filepath text,
    planning_application_number text,
    adress text,  -- Note: typo preserved from source
    decision_date text,
    decision_type text,
    document_type text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);
```

**Land Charges**:
```sql
CREATE TABLE stg_land_charges (
    job_number text,
    filepath text,
    card_code text,
    address text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);
```

**Enforcement Notices**:
```sql
CREATE TABLE stg_enforcement_notices (
    job_number text,
    filepath text,
    planning_enforcement_reference_number text,
    address text,
    date text,
    document_type text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);
```

**Agreements**:
```sql
CREATE TABLE stg_agreements (
    job_number text,
    filepath text,
    address text,
    date text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);
```

## 4.4 Dimension Tables

### 4.4.1 Location Dimension

Stores coordinate data from both LLPG and OS Open UPRN sources:

```sql
CREATE TABLE dim_location (
    location_id SERIAL PRIMARY KEY,
    uprn TEXT UNIQUE,
    easting NUMERIC,
    northing NUMERIC,
    latitude NUMERIC,
    longitude NUMERIC,
    geom_27700 GEOMETRY(POINT, 27700),
    geom_4326 GEOMETRY(POINT, 4326),
    source_dataset TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);
```

**Key Features**:
- Dual geometry columns for British National Grid (EPSG:27700) and WGS84 (EPSG:4326)
- Source tracking to identify coordinate origin
- Unique constraint on UPRN prevents duplicates

### 4.4.2 Address Dimension

The authoritative address table derived from LLPG:

```sql
CREATE TABLE dim_address (
    address_id SERIAL PRIMARY KEY,
    location_id INTEGER REFERENCES dim_location(location_id),
    uprn TEXT UNIQUE,
    full_address TEXT NOT NULL,
    address_canonical TEXT,
    usrn TEXT,
    blpu_class TEXT,
    postal_flag BOOLEAN,
    status_code TEXT,
    is_historic BOOLEAN DEFAULT false,
    created_from_source TEXT,
    source_document_id INTEGER,
    created_at TIMESTAMPTZ DEFAULT now()
);
```

**Column Descriptions**:
- `address_canonical`: Normalised address for matching (uppercase, punctuation removed)
- `usrn`: Links to street-level data
- `blpu_class`: Property classification (residential, commercial, etc.)
- `is_historic`: Flag for addresses created from historic document UPRNs
- `created_from_source`: Source type if created as historic record

### 4.4.3 Document Type Dimension

Lookup table for source document types:

```sql
CREATE TABLE dim_document_type (
    doc_type_id SERIAL PRIMARY KEY,
    type_code TEXT UNIQUE,
    type_name TEXT,
    description TEXT
);

INSERT INTO dim_document_type (type_code, type_name, description) VALUES
('decision', 'Decision Notice', 'Planning application decision notices'),
('land_charge', 'Land Charge', 'Land charges cards'),
('enforcement', 'Enforcement Notice', 'Planning enforcement notices'),
('agreement', 'Agreement', 'Planning agreements and obligations'),
('microfiche_post_1974', 'Microfiche Post-1974', 'Post-1974 microfiche records'),
('microfiche_pre_1974', 'Microfiche Pre-1974', 'Pre-1974 microfiche records'),
('street_numbering', 'Street Name and Numbering', 'Street addressing records'),
('enlargement_map', 'Enlargement Map', 'Map reference records'),
('enl_folder', 'ENL Folder', 'Development documentation');
```

### 4.4.4 Match Method Dimension

Lookup table for matching algorithms:

```sql
CREATE TABLE dim_match_method (
    method_id SERIAL PRIMARY KEY,
    method_code TEXT UNIQUE,
    method_name TEXT,
    description TEXT,
    confidence_threshold NUMERIC(5,4)
);

INSERT INTO dim_match_method (method_code, method_name, description, confidence_threshold) VALUES
('exact_uprn', 'Exact UPRN Match', 'Direct UPRN match against LLPG', 1.0000),
('exact_text', 'Exact Text Match', 'Exact canonical address match', 0.9500),
('fuzzy_high', 'High Confidence Fuzzy', 'High confidence fuzzy matching', 0.9000),
('fuzzy_medium', 'Medium Confidence Fuzzy', 'Medium confidence fuzzy matching', 0.8000),
('fuzzy_low', 'Low Confidence Fuzzy', 'Low confidence fuzzy matching', 0.7000),
('spatial', 'Spatial Match', 'Coordinate-based spatial matching', 0.8500),
('vector', 'Vector Embedding Match', 'Semantic similarity matching', 0.8000),
('manual', 'Manual Match', 'Manually verified match', 1.0000),
('llm_corrected', 'LLM Corrected', 'LLM-powered address correction', 0.8500);
```

## 4.5 Source Document Table

Unified storage for all source documents:

```sql
CREATE TABLE src_document (
    document_id SERIAL PRIMARY KEY,
    doc_type_id INTEGER REFERENCES dim_document_type(doc_type_id),
    job_number TEXT,
    filepath TEXT,
    external_reference TEXT,
    document_date DATE,
    raw_address TEXT NOT NULL,
    address_canonical TEXT,
    raw_uprn TEXT,
    raw_easting TEXT,
    raw_northing TEXT,
    gopostal_house_number TEXT,
    gopostal_road TEXT,
    gopostal_city TEXT,
    gopostal_postcode TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);
```

**Key Features**:
- Single table for all nine document types
- Preserves both raw and canonical addresses
- Stores libpostal-parsed components
- Links to document type dimension

## 4.6 Match Result Tables

### 4.6.1 Match Run

Tracks individual matching executions:

```sql
CREATE TABLE match_run (
    run_id BIGSERIAL PRIMARY KEY,
    run_started_at TIMESTAMPTZ DEFAULT now(),
    run_completed_at TIMESTAMPTZ,
    run_label TEXT,
    notes TEXT,
    total_processed INTEGER,
    total_accepted INTEGER,
    total_review INTEGER,
    total_rejected INTEGER
);
```

### 4.6.2 Match Result

Stores all match attempts with full feature detail:

```sql
CREATE TABLE match_result (
    match_id BIGSERIAL PRIMARY KEY,
    run_id BIGINT REFERENCES match_run(run_id),
    src_id BIGINT,
    candidate_uprn TEXT,
    method TEXT NOT NULL,
    score NUMERIC,
    confidence NUMERIC,
    tie_rank INTEGER,
    features JSONB,
    decided BOOLEAN DEFAULT false,
    decision TEXT,
    decided_by TEXT,
    decided_at TIMESTAMPTZ,
    notes TEXT
);
```

**Features Column**: Stores complete feature breakdown as JSON:
```json
{
    "trgm_score": 0.85,
    "jaro_score": 0.82,
    "locality_overlap": 1.0,
    "street_overlap": 0.8,
    "same_house_number": true,
    "phonetic_hits": 2,
    "spatial_distance": 45.2,
    "spatial_boost": 0.086
}
```

### 4.6.3 Match Accepted

Stores only accepted matches for efficient querying:

```sql
CREATE TABLE match_accepted (
    src_id BIGINT PRIMARY KEY,
    uprn TEXT,
    method TEXT NOT NULL,
    score NUMERIC,
    confidence NUMERIC,
    run_id BIGINT REFERENCES match_run(run_id),
    accepted_by TEXT DEFAULT 'system',
    accepted_at TIMESTAMPTZ DEFAULT now()
);
```

### 4.6.4 Match Override

Stores manual corrections and overrides:

```sql
CREATE TABLE match_override (
    override_id BIGSERIAL PRIMARY KEY,
    src_id BIGINT,
    uprn TEXT,
    reason TEXT,
    created_by TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);
```

## 4.7 Address Match Table

Direct link between documents and addresses:

```sql
CREATE TABLE address_match (
    match_id SERIAL PRIMARY KEY,
    document_id INTEGER REFERENCES src_document(document_id),
    address_id INTEGER REFERENCES dim_address(address_id),
    location_id INTEGER REFERENCES dim_location(location_id),
    match_method_id INTEGER REFERENCES dim_match_method(method_id),
    confidence_score NUMERIC(5,4),
    match_status TEXT,
    matched_by TEXT,
    matched_at TIMESTAMPTZ DEFAULT now()
);
```

## 4.8 Supporting Tables

### 4.8.1 Address Normalisation Rules

Configurable abbreviation expansion:

```sql
CREATE TABLE address_normalization_rules (
    rule_id SERIAL PRIMARY KEY,
    pattern TEXT,
    replacement TEXT,
    rule_type TEXT,
    priority INTEGER DEFAULT 0
);

INSERT INTO address_normalization_rules (pattern, replacement, rule_type, priority) VALUES
('\\bRD\\b', 'ROAD', 'abbreviation', 100),
('\\bST\\b', 'STREET', 'abbreviation', 100),
('\\bAVE\\b', 'AVENUE', 'abbreviation', 100),
('\\bGDNS\\b', 'GARDENS', 'abbreviation', 100),
('\\bCT\\b', 'COURT', 'abbreviation', 100),
('\\bDR\\b', 'DRIVE', 'abbreviation', 100),
('\\bLN\\b', 'LANE', 'abbreviation', 100),
('\\bCL\\b', 'CLOSE', 'abbreviation', 100),
('\\bCRES\\b', 'CRESCENT', 'abbreviation', 100),
('\\bTER\\b', 'TERRACE', 'abbreviation', 100);
```

### 4.8.2 Address Match Corrected

Stores corrections from all correction methods:

```sql
CREATE TABLE address_match_corrected (
    correction_id SERIAL PRIMARY KEY,
    document_id INTEGER,
    original_address TEXT,
    corrected_address TEXT,
    matched_uprn TEXT,
    correction_method TEXT,
    confidence_score NUMERIC(5,4),
    created_at TIMESTAMPTZ DEFAULT now()
);
```

## 4.9 Index Strategy

### 4.9.1 GIN Indexes for Trigram Matching

```sql
CREATE INDEX idx_dim_address_canonical_trgm
    ON dim_address USING GIN(address_canonical gin_trgm_ops);

CREATE INDEX idx_src_document_canonical_trgm
    ON src_document USING GIN(address_canonical gin_trgm_ops);
```

These indexes enable efficient trigram similarity queries using the `%` operator.

### 4.9.2 GIST Indexes for Spatial Queries

```sql
CREATE INDEX idx_dim_location_geom_27700
    ON dim_location USING GIST(geom_27700);

CREATE INDEX idx_dim_location_geom_4326
    ON dim_location USING GIST(geom_4326);
```

These indexes support PostGIS spatial functions such as ST_Distance and ST_DWithin.

### 4.9.3 Standard B-tree Indexes

```sql
-- Primary lookup indexes
CREATE INDEX idx_dim_address_uprn ON dim_address(uprn);
CREATE INDEX idx_dim_location_uprn ON dim_location(uprn);
CREATE INDEX idx_src_document_raw_uprn ON src_document(raw_uprn);

-- Foreign key indexes
CREATE INDEX idx_dim_address_location_id ON dim_address(location_id);
CREATE INDEX idx_src_document_doc_type ON src_document(doc_type_id);
CREATE INDEX idx_address_match_document ON address_match(document_id);
CREATE INDEX idx_address_match_address ON address_match(address_id);

-- Query optimisation indexes
CREATE INDEX idx_match_result_run_id ON match_result(run_id);
CREATE INDEX idx_match_result_src_id ON match_result(src_id);
CREATE INDEX idx_match_accepted_uprn ON match_accepted(uprn);
```

## 4.10 Coordinate Systems

The schema handles two coordinate reference systems:

### 4.10.1 British National Grid (EPSG:27700)

- Used for Easting and Northing values
- Native format for UK Ordnance Survey data
- Stored in `geom_27700` geometry columns
- Units: metres

### 4.10.2 WGS84 (EPSG:4326)

- Used for Latitude and Longitude values
- Required for web mapping (Leaflet, Google Maps)
- Stored in `geom_4326` geometry columns
- Units: degrees

### 4.10.3 Coordinate Transformation

PostGIS handles transformations:

```sql
-- Create BNG geometry from coordinates
ST_SetSRID(ST_MakePoint(easting, northing), 27700)

-- Transform to WGS84
ST_Transform(geom_27700, 4326)
```

## 4.11 Data Integrity Constraints

### 4.11.1 Unique Constraints

```sql
ALTER TABLE dim_address ADD CONSTRAINT uq_dim_address_uprn UNIQUE (uprn);
ALTER TABLE dim_location ADD CONSTRAINT uq_dim_location_uprn UNIQUE (uprn);
ALTER TABLE match_accepted ADD CONSTRAINT pk_match_accepted PRIMARY KEY (src_id);
```

### 4.11.2 Foreign Key Constraints

All foreign key relationships are enforced to maintain referential integrity:

```sql
ALTER TABLE dim_address
    ADD CONSTRAINT fk_address_location
    FOREIGN KEY (location_id) REFERENCES dim_location(location_id);

ALTER TABLE src_document
    ADD CONSTRAINT fk_document_type
    FOREIGN KEY (doc_type_id) REFERENCES dim_document_type(doc_type_id);
```

## 4.12 Chapter Summary

This chapter has documented the database schema:

- Staging tables for raw data preservation
- Dimension tables for normalised reference data
- Source document table for unified historic records
- Match result tables for complete audit trails
- Index strategy for query performance
- Coordinate system handling

The schema design supports both operational matching workflows and analytical reporting whilst maintaining full traceability of all decisions.

---

*This chapter provides the data foundation. Chapter 5 examines address normalisation algorithms.*
