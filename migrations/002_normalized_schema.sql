-- EHDC LLPG Address Matching System - Normalized Schema
-- Migration from staging tables to normalized dimension tables

-- ============================================================================
-- 1. CREATE NORMALIZED SCHEMA
-- ============================================================================

-- Core dimension tables
CREATE TABLE dim_location (
    location_id SERIAL PRIMARY KEY,
    uprn TEXT UNIQUE,                    -- UPRN when available
    easting NUMERIC,                     -- British National Grid X
    northing NUMERIC,                    -- British National Grid Y  
    latitude NUMERIC,                    -- WGS84 latitude
    longitude NUMERIC,                   -- WGS84 longitude
    geom_27700 GEOMETRY(POINT, 27700),   -- BNG point geometry
    geom_4326 GEOMETRY(POINT, 4326),     -- WGS84 point geometry
    source_dataset TEXT,                 -- 'os_uprn', 'ehdc_llpg', 'derived'
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE dim_address (
    address_id SERIAL PRIMARY KEY,
    location_id INTEGER REFERENCES dim_location(location_id),
    uprn TEXT UNIQUE,
    full_address TEXT NOT NULL,
    address_canonical TEXT,              -- Normalized for matching
    usrn TEXT,                          -- Unique Street Reference Number
    blpu_class TEXT,                    -- Basic Land and Property Unit class
    postal_flag BOOLEAN,                -- TRUE/FALSE postal address
    status_code TEXT,                   -- LGC status code
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE dim_document_type (
    doc_type_id SERIAL PRIMARY KEY,
    type_code TEXT UNIQUE,              -- 'decision', 'land_charge', etc.
    type_name TEXT,
    description TEXT
);

CREATE TABLE dim_match_method (
    method_id SERIAL PRIMARY KEY,
    method_code TEXT UNIQUE,            -- 'exact_uprn', 'fuzzy_text', 'spatial'
    method_name TEXT,
    description TEXT,
    confidence_threshold NUMERIC(5,4)  -- Minimum confidence for auto-accept
);

-- Source document table
CREATE TABLE src_document (
    document_id SERIAL PRIMARY KEY,
    doc_type_id INTEGER REFERENCES dim_document_type(doc_type_id),
    job_number TEXT,
    filepath TEXT,
    external_reference TEXT,            -- Planning app number, card code, etc.
    document_date DATE,
    raw_address TEXT NOT NULL,
    address_canonical TEXT,             -- Normalized for matching
    raw_uprn TEXT,                      -- UPRN as provided in source
    raw_easting TEXT,                   -- Coordinates as provided
    raw_northing TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Address matching table
CREATE TABLE address_match (
    match_id SERIAL PRIMARY KEY,
    document_id INTEGER REFERENCES src_document(document_id),
    address_id INTEGER REFERENCES dim_address(address_id),
    location_id INTEGER REFERENCES dim_location(location_id),
    match_method_id INTEGER REFERENCES dim_match_method(method_id),
    confidence_score NUMERIC(5,4),     -- 0.0000 to 1.0000
    match_status TEXT,                  -- 'auto', 'manual', 'rejected'
    matched_by TEXT,                    -- System or user
    matched_at TIMESTAMPTZ DEFAULT now()
);

-- Supporting tables
CREATE TABLE address_normalization_rules (
    rule_id SERIAL PRIMARY KEY,
    pattern TEXT,
    replacement TEXT,
    rule_type TEXT,                     -- 'abbreviation', 'cleanup', etc.
    priority INTEGER DEFAULT 0
);

-- ============================================================================
-- 2. POPULATE LOOKUP TABLES
-- ============================================================================

-- Document types
INSERT INTO dim_document_type (type_code, type_name, description) VALUES
('decision', 'Decision Notice', 'Planning application decision notices'),
('land_charge', 'Land Charge', 'Land charges cards'),
('enforcement', 'Enforcement Notice', 'Planning enforcement notices'),
('agreement', 'Agreement', 'Planning agreements and obligations');

-- Match methods
INSERT INTO dim_match_method (method_code, method_name, description, confidence_threshold) VALUES
('exact_uprn', 'Exact UPRN Match', 'Direct UPRN match against LLPG', 1.0000),
('exact_text', 'Exact Text Match', 'Exact canonical address match', 0.9500),
('fuzzy_high', 'High Confidence Fuzzy', 'High confidence fuzzy text matching', 0.9000),
('fuzzy_medium', 'Medium Confidence Fuzzy', 'Medium confidence fuzzy text matching', 0.8000),
('fuzzy_low', 'Low Confidence Fuzzy', 'Low confidence fuzzy text matching', 0.7000),
('spatial', 'Spatial Match', 'Coordinate-based spatial matching', 0.8500),
('manual', 'Manual Match', 'Manually verified match', 1.0000);

-- Address normalization rules (UK standards)
INSERT INTO address_normalization_rules (pattern, replacement, rule_type, priority) VALUES
('\\bRD\\b', 'ROAD', 'abbreviation', 100),
('\\bST\\b', 'STREET', 'abbreviation', 100),
('\\bAVE\\b', 'AVENUE', 'abbreviation', 100),
('\\bGDNS\\b', 'GARDENS', 'abbreviation', 100),
('\\bCT\\b', 'COURT', 'abbreviation', 100),
('\\bDR\\b', 'DRIVE', 'abbreviation', 100),
('\\bLN\\b', 'LANE', 'abbreviation', 100),
('\\bPL\\b', 'PLACE', 'abbreviation', 100),
('\\bSQ\\b', 'SQUARE', 'abbreviation', 100),
('\\bCRES\\b', 'CRESCENT', 'abbreviation', 100),
('\\bTER\\b', 'TERRACE', 'abbreviation', 100),
('\\bCL\\b', 'CLOSE', 'abbreviation', 100),
('\\bPK\\b', 'PARK', 'abbreviation', 90),
('\\bGRN\\b', 'GREEN', 'abbreviation', 90),
('\\bWY\\b', 'WAY', 'abbreviation', 90);

-- ============================================================================
-- 3. MIGRATE OS UPRN DATA TO LOCATIONS
-- ============================================================================

INSERT INTO dim_location (
    uprn,
    easting,
    northing,
    latitude,
    longitude,
    geom_27700,
    geom_4326,
    source_dataset
)
SELECT 
    uprn,
    NULLIF(x_coordinate, '')::NUMERIC,
    NULLIF(y_coordinate, '')::NUMERIC,
    NULLIF(latitude, '')::NUMERIC,
    NULLIF(longitude, '')::NUMERIC,
    CASE 
        WHEN x_coordinate IS NOT NULL AND y_coordinate IS NOT NULL AND x_coordinate != '' AND y_coordinate != ''
        THEN ST_SetSRID(ST_MakePoint(x_coordinate::NUMERIC, y_coordinate::NUMERIC), 27700)
        ELSE NULL
    END,
    CASE 
        WHEN latitude IS NOT NULL AND longitude IS NOT NULL AND latitude != '' AND longitude != ''
        THEN ST_SetSRID(ST_MakePoint(longitude::NUMERIC, latitude::NUMERIC), 4326)
        ELSE NULL
    END,
    'os_uprn'
FROM stg_os_uprn
WHERE uprn IS NOT NULL AND uprn != '';

-- ============================================================================
-- 4. MIGRATE EHDC LLPG DATA
-- ============================================================================

-- First, ensure locations exist for EHDC LLPG UPRNs (merge with OS data)
INSERT INTO dim_location (
    uprn,
    easting,
    northing,
    geom_27700,
    source_dataset
)
SELECT 
    s.bs7666uprn,
    NULLIF(s.easting, '')::NUMERIC,
    NULLIF(s.northing, '')::NUMERIC,
    CASE 
        WHEN s.easting IS NOT NULL AND s.northing IS NOT NULL AND s.easting != '' AND s.northing != ''
        THEN ST_SetSRID(ST_MakePoint(s.easting::NUMERIC, s.northing::NUMERIC), 27700)
        ELSE NULL
    END,
    'ehdc_llpg'
FROM stg_ehdc_llpg s
LEFT JOIN dim_location l ON l.uprn = s.bs7666uprn
WHERE s.bs7666uprn IS NOT NULL 
  AND s.bs7666uprn != ''
  AND l.uprn IS NULL;  -- Only insert if not already exists from OS data

-- Update existing OS locations with EHDC coordinate data where missing
UPDATE dim_location 
SET 
    easting = COALESCE(easting, NULLIF(s.easting, '')::NUMERIC),
    northing = COALESCE(northing, NULLIF(s.northing, '')::NUMERIC),
    geom_27700 = COALESCE(geom_27700, 
        CASE 
            WHEN s.easting IS NOT NULL AND s.northing IS NOT NULL AND s.easting != '' AND s.northing != ''
            THEN ST_SetSRID(ST_MakePoint(s.easting::NUMERIC, s.northing::NUMERIC), 27700)
            ELSE NULL
        END
    )
FROM stg_ehdc_llpg s
WHERE dim_location.uprn = s.bs7666uprn
  AND dim_location.source_dataset = 'os_uprn'
  AND (dim_location.easting IS NULL OR dim_location.northing IS NULL);

-- Create address dimension from EHDC LLPG
INSERT INTO dim_address (
    location_id,
    uprn,
    full_address,
    address_canonical,
    usrn,
    blpu_class,
    postal_flag,
    status_code
)
SELECT 
    l.location_id,
    s.bs7666uprn,
    s.locaddress,
    -- Basic address canonicalization (uppercase, remove extra spaces)
    REGEXP_REPLACE(
        REGEXP_REPLACE(UPPER(TRIM(s.locaddress)), '\\s+', ' ', 'g'),
        '[^A-Z0-9 ]', '', 'g'
    ),
    s.bs7666usrn,
    s.blpuclass,
    CASE WHEN UPPER(TRIM(s.postal)) IN ('Y', 'YES', 'TRUE', '1') THEN TRUE ELSE FALSE END,
    s.lgcstatusc
FROM stg_ehdc_llpg s
INNER JOIN dim_location l ON l.uprn = s.bs7666uprn
WHERE s.bs7666uprn IS NOT NULL 
  AND s.bs7666uprn != ''
  AND s.locaddress IS NOT NULL 
  AND s.locaddress != '';

-- ============================================================================
-- 5. MIGRATE SOURCE DOCUMENTS
-- ============================================================================

-- Decision notices
INSERT INTO src_document (
    doc_type_id,
    job_number,
    filepath,
    external_reference,
    document_date,
    raw_address,
    address_canonical,
    raw_uprn,
    raw_easting,
    raw_northing
)
SELECT 
    dt.doc_type_id,
    s.job_number,
    s.filepath,
    s.planning_application_number,
    CASE 
        WHEN s.decision_date ~ '^\\d{1,2}/\\d{1,2}/\\d{4}$' 
        THEN CASE 
            WHEN (split_part(s.decision_date, '/', 2)::int > 12 OR split_part(s.decision_date, '/', 1)::int > 31) 
            THEN NULL
            ELSE to_date(s.decision_date, 'DD/MM/YYYY')
        END
        WHEN s.decision_date ~ '^\\d{1,2}/\\d{1,2}/\\d{2}$' 
        THEN CASE 
            WHEN (split_part(s.decision_date, '/', 2)::int > 12 OR split_part(s.decision_date, '/', 1)::int > 31) 
            THEN NULL
            ELSE to_date(s.decision_date, 'DD/MM/YY')
        END
        ELSE NULL
    END,
    s.adress,
    REGEXP_REPLACE(
        REGEXP_REPLACE(UPPER(TRIM(s.adress)), '\\s+', ' ', 'g'),
        '[^A-Z0-9 ]', '', 'g'
    ),
    NULLIF(TRIM(s.bs7666uprn), ''),
    NULLIF(TRIM(s.easting), ''),
    NULLIF(TRIM(s.northing), '')
FROM stg_decision_notices s
CROSS JOIN dim_document_type dt
WHERE dt.type_code = 'decision'
  AND s.adress IS NOT NULL 
  AND s.adress != '';

-- Land charges
INSERT INTO src_document (
    doc_type_id,
    job_number,
    filepath,
    external_reference,
    raw_address,
    address_canonical,
    raw_uprn,
    raw_easting,
    raw_northing
)
SELECT 
    dt.doc_type_id,
    s.job_number,
    s.filepath,
    s.card_code,
    s.address,
    REGEXP_REPLACE(
        REGEXP_REPLACE(UPPER(TRIM(s.address)), '\\s+', ' ', 'g'),
        '[^A-Z0-9 ]', '', 'g'
    ),
    NULLIF(TRIM(s.bs7666uprn), ''),
    NULLIF(TRIM(s.easting), ''),
    NULLIF(TRIM(s.northing), '')
FROM stg_land_charges s
CROSS JOIN dim_document_type dt
WHERE dt.type_code = 'land_charge'
  AND s.address IS NOT NULL 
  AND s.address != '';

-- Enforcement notices
INSERT INTO src_document (
    doc_type_id,
    job_number,
    filepath,
    external_reference,
    document_date,
    raw_address,
    address_canonical,
    raw_uprn,
    raw_easting,
    raw_northing
)
SELECT 
    dt.doc_type_id,
    s.job_number,
    s.filepath,
    s.planning_enforcement_reference_number,
    CASE 
        WHEN s.date ~ '^\\d{1,2}/\\d{1,2}/\\d{4}$' 
        THEN CASE 
            WHEN (split_part(s.date, '/', 2)::int > 12 OR split_part(s.date, '/', 1)::int > 31) 
            THEN NULL
            ELSE to_date(s.date, 'DD/MM/YYYY')
        END
        WHEN s.date ~ '^\\d{1,2}/\\d{1,2}/\\d{2}$' 
        THEN CASE 
            WHEN (split_part(s.date, '/', 2)::int > 12 OR split_part(s.date, '/', 1)::int > 31) 
            THEN NULL
            ELSE to_date(s.date, 'DD/MM/YY')
        END
        ELSE NULL
    END,
    s.address,
    REGEXP_REPLACE(
        REGEXP_REPLACE(UPPER(TRIM(s.address)), '\\s+', ' ', 'g'),
        '[^A-Z0-9 ]', '', 'g'
    ),
    NULLIF(TRIM(s.bs7666uprn), ''),
    NULLIF(TRIM(s.easting), ''),
    NULLIF(TRIM(s.northing), '')
FROM stg_enforcement_notices s
CROSS JOIN dim_document_type dt
WHERE dt.type_code = 'enforcement'
  AND s.address IS NOT NULL 
  AND s.address != '';

-- Agreements
INSERT INTO src_document (
    doc_type_id,
    job_number,
    filepath,
    document_date,
    raw_address,
    address_canonical,
    raw_uprn,
    raw_easting,
    raw_northing
)
SELECT 
    dt.doc_type_id,
    s.job_number,
    s.filepath,
    CASE 
        WHEN s.date ~ '^\\d{1,2}/\\d{1,2}/\\d{4}$' 
        THEN CASE 
            WHEN (split_part(s.date, '/', 2)::int > 12 OR split_part(s.date, '/', 1)::int > 31) 
            THEN NULL
            ELSE to_date(s.date, 'DD/MM/YYYY')
        END
        WHEN s.date ~ '^\\d{1,2}/\\d{1,2}/\\d{2}$' 
        THEN CASE 
            WHEN (split_part(s.date, '/', 2)::int > 12 OR split_part(s.date, '/', 1)::int > 31) 
            THEN NULL
            ELSE to_date(s.date, 'DD/MM/YY')
        END
        ELSE NULL
    END,
    s.address,
    REGEXP_REPLACE(
        REGEXP_REPLACE(UPPER(TRIM(s.address)), '\\s+', ' ', 'g'),
        '[^A-Z0-9 ]', '', 'g'
    ),
    NULLIF(TRIM(s.bs7666uprn), ''),
    NULLIF(TRIM(s.easting), ''),
    NULLIF(TRIM(s.northing), '')
FROM stg_agreements s
CROSS JOIN dim_document_type dt
WHERE dt.type_code = 'agreement'
  AND s.address IS NOT NULL 
  AND s.address != '';

-- ============================================================================
-- 6. CREATE INDEXES FOR PERFORMANCE
-- ============================================================================

-- Location indexes
CREATE INDEX idx_dim_location_uprn ON dim_location(uprn);
CREATE INDEX idx_dim_location_coords ON dim_location(easting, northing);
CREATE INDEX idx_dim_location_geom_27700 ON dim_location USING GIST(geom_27700);
CREATE INDEX idx_dim_location_geom_4326 ON dim_location USING GIST(geom_4326);
CREATE INDEX idx_dim_location_source ON dim_location(source_dataset);

-- Address indexes
CREATE INDEX idx_dim_address_uprn ON dim_address(uprn);
CREATE INDEX idx_dim_address_location_id ON dim_address(location_id);
CREATE INDEX idx_dim_address_canonical_trgm ON dim_address USING GIN(address_canonical gin_trgm_ops);
CREATE INDEX idx_dim_address_full_text ON dim_address USING GIN(to_tsvector('english', full_address));

-- Source document indexes
CREATE INDEX idx_src_document_doc_type ON src_document(doc_type_id);
CREATE INDEX idx_src_document_canonical_trgm ON src_document USING GIN(address_canonical gin_trgm_ops);
CREATE INDEX idx_src_document_raw_uprn ON src_document(raw_uprn);
CREATE INDEX idx_src_document_date ON src_document(document_date);

-- Match indexes
CREATE INDEX idx_address_match_document ON address_match(document_id);
CREATE INDEX idx_address_match_address ON address_match(address_id);
CREATE INDEX idx_address_match_location ON address_match(location_id);
CREATE INDEX idx_address_match_method ON address_match(match_method_id);
CREATE INDEX idx_address_match_status ON address_match(match_status);
CREATE INDEX idx_address_match_confidence ON address_match(confidence_score);

-- ============================================================================
-- 7. UPDATE STATISTICS
-- ============================================================================

ANALYZE dim_location;
ANALYZE dim_address;
ANALYZE src_document;
ANALYZE address_match;