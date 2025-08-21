-- Migration 009: Create Proper Dimension Tables
-- Purpose: Create dimension tables for proper star schema design
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- 1. Document Type Dimension
CREATE TABLE dim_document_type (
    document_type_id        SERIAL PRIMARY KEY,
    document_type_code      VARCHAR(20) NOT NULL UNIQUE,
    document_type_name      VARCHAR(100) NOT NULL,
    document_category       VARCHAR(50),
    description             TEXT,
    is_active              BOOLEAN DEFAULT TRUE,
    created_at             TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

INSERT INTO dim_document_type (document_type_code, document_type_name, document_category, description) VALUES
('PLAN_APP', 'Planning Application', 'Planning', 'Standard planning application'),
('BUILD_REG', 'Building Regulations', 'Building Control', 'Building regulations application'),
('APPEAL', 'Planning Appeal', 'Planning', 'Appeal against planning decision'),
('ENFORCE', 'Enforcement Notice', 'Enforcement', 'Planning enforcement action'),
('PREAPP', 'Pre-Application', 'Planning', 'Pre-application advice'),
('DISCHARGE', 'Condition Discharge', 'Planning', 'Discharge of planning conditions'),
('VARIATION', 'Variation Application', 'Planning', 'Variation of existing permission'),
('UNKNOWN', 'Unknown Type', 'Other', 'Document type not specified');

-- 2. Document Status Dimension
CREATE TABLE dim_document_status (
    document_status_id      SERIAL PRIMARY KEY,
    status_code            VARCHAR(20) NOT NULL UNIQUE,
    status_name            VARCHAR(50) NOT NULL,
    status_category        VARCHAR(30),
    is_active_status       BOOLEAN DEFAULT TRUE,
    is_final_status        BOOLEAN DEFAULT FALSE,
    sort_order             INTEGER,
    created_at             TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

INSERT INTO dim_document_status (status_code, status_name, status_category, is_active_status, is_final_status, sort_order) VALUES
('ACTIVE', 'Active', 'Current', TRUE, FALSE, 1),
('PENDING', 'Pending Review', 'Processing', TRUE, FALSE, 2),
('UNDER_REVIEW', 'Under Review', 'Processing', TRUE, FALSE, 3),
('APPROVED', 'Approved', 'Completed', FALSE, TRUE, 4),
('REJECTED', 'Rejected', 'Completed', FALSE, TRUE, 5),
('WITHDRAWN', 'Withdrawn', 'Completed', FALSE, TRUE, 6),
('ARCHIVED', 'Archived', 'Historical', FALSE, TRUE, 7),
('UNKNOWN', 'Unknown Status', 'Other', TRUE, FALSE, 99);

-- 3. Original Address Dimension
CREATE TABLE dim_original_address (
    original_address_id     SERIAL PRIMARY KEY,
    raw_address            TEXT NOT NULL,
    address_hash           VARCHAR(64) NOT NULL UNIQUE, -- MD5 hash for deduplication
    
    -- Parsed components
    address_line_1         VARCHAR(255),
    address_line_2         VARCHAR(255),
    town                   VARCHAR(100),
    county                 VARCHAR(100),
    postcode               VARCHAR(10),
    country                VARCHAR(50),
    
    -- Standardized components (from gopostal)
    std_house_number       VARCHAR(20),
    std_house_name         VARCHAR(100),
    std_road               VARCHAR(200),
    std_suburb             VARCHAR(100),
    std_city               VARCHAR(100),
    std_state_district     VARCHAR(100),
    std_state              VARCHAR(100),
    std_postcode           VARCHAR(10),
    std_country            VARCHAR(50),
    std_unit               VARCHAR(50),
    
    -- Quality metrics
    address_quality_score  DECIMAL(3,2),
    component_completeness DECIMAL(3,2),
    gopostal_processed    BOOLEAN DEFAULT FALSE,
    
    -- Usage tracking
    usage_count           INTEGER DEFAULT 0,
    first_seen            TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_used             TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Audit
    created_at            TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at            TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX idx_dim_original_address_hash ON dim_original_address(address_hash);
CREATE INDEX idx_dim_original_address_postcode ON dim_original_address(std_postcode) WHERE std_postcode IS NOT NULL;
CREATE INDEX idx_dim_original_address_road_city ON dim_original_address(std_road, std_city) WHERE std_road IS NOT NULL;
CREATE INDEX idx_dim_original_address_usage ON dim_original_address(usage_count DESC);

-- 4. Match Method Dimension
CREATE TABLE dim_match_method (
    match_method_id        SERIAL PRIMARY KEY,
    method_code           VARCHAR(50) NOT NULL UNIQUE,
    method_name           VARCHAR(100) NOT NULL,
    method_category       VARCHAR(50),
    confidence_threshold  DECIMAL(3,2),
    description           TEXT,
    algorithm_version     VARCHAR(20) DEFAULT '1.0',
    is_active            BOOLEAN DEFAULT TRUE,
    created_at           TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

INSERT INTO dim_match_method (method_code, method_name, method_category, confidence_threshold, description) VALUES
('EXACT_COMPONENTS', 'Exact Component Match', 'High Precision', 0.95, 'Perfect match on multiple address components'),
('POSTCODE_HOUSE', 'Postcode + House Number', 'High Precision', 0.90, 'Match on postcode and house number'),
('ROAD_CITY_EXACT', 'Exact Road + City', 'Medium Precision', 0.85, 'Exact match on road and city names'),
('ROAD_CITY_FUZZY', 'Fuzzy Road + City', 'Medium Precision', 0.75, 'Fuzzy match on road and city with similarity'),
('FUZZY_ROAD', 'Fuzzy Road Only', 'Low Precision', 0.60, 'Fuzzy matching on road name only'),
('MANUAL_MATCH', 'Manual Override', 'Manual', 1.00, 'Manually verified match'),
('EXACT_UPRN', 'Exact UPRN Match', 'Perfect', 1.00, 'Direct UPRN match'),
('NO_MATCH', 'No Match Found', 'None', 0.00, 'No suitable match identified');

-- 5. Match Decision Dimension  
CREATE TABLE dim_match_decision (
    match_decision_id      SERIAL PRIMARY KEY,
    decision_code         VARCHAR(20) NOT NULL UNIQUE,
    decision_name         VARCHAR(50) NOT NULL,
    auto_process          BOOLEAN DEFAULT FALSE,
    requires_review       BOOLEAN DEFAULT FALSE,
    confidence_min        DECIMAL(3,2),
    confidence_max        DECIMAL(3,2),
    description          TEXT,
    created_at           TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

INSERT INTO dim_match_decision (decision_code, decision_name, auto_process, requires_review, confidence_min, confidence_max, description) VALUES
('AUTO_ACCEPT', 'Auto Accept', TRUE, FALSE, 0.85, 1.00, 'High confidence match - auto process'),
('NEEDS_REVIEW', 'Needs Review', FALSE, TRUE, 0.50, 0.84, 'Medium confidence - requires manual review'),
('LOW_CONFIDENCE', 'Low Confidence', FALSE, TRUE, 0.20, 0.49, 'Low confidence match - manual verification needed'),
('NO_MATCH', 'No Match', FALSE, FALSE, 0.00, 0.19, 'No suitable match found'),
('MANUAL_OVERRIDE', 'Manual Override', TRUE, FALSE, 0.00, 1.00, 'Manually verified and overridden');

-- 6. Property Type Dimension
CREATE TABLE dim_property_type (
    property_type_id      SERIAL PRIMARY KEY,
    property_code        VARCHAR(20) NOT NULL UNIQUE,
    property_name        VARCHAR(100) NOT NULL,
    property_category    VARCHAR(50),
    use_class           VARCHAR(10),
    description         TEXT,
    is_residential      BOOLEAN DEFAULT FALSE,
    is_commercial       BOOLEAN DEFAULT FALSE,
    sort_order          INTEGER,
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

INSERT INTO dim_property_type (property_code, property_name, property_category, use_class, is_residential, is_commercial, sort_order) VALUES
('HOUSE', 'House', 'Residential', 'C3', TRUE, FALSE, 1),
('FLAT', 'Flat/Apartment', 'Residential', 'C3', TRUE, FALSE, 2),
('BUNGALOW', 'Bungalow', 'Residential', 'C3', TRUE, FALSE, 3),
('MAISONETTE', 'Maisonette', 'Residential', 'C3', TRUE, FALSE, 4),
('OFFICE', 'Office Building', 'Commercial', 'B1', FALSE, TRUE, 10),
('RETAIL', 'Retail Unit', 'Commercial', 'A1', FALSE, TRUE, 11),
('RESTAURANT', 'Restaurant/Cafe', 'Commercial', 'A3', FALSE, TRUE, 12),
('INDUSTRIAL', 'Industrial Unit', 'Industrial', 'B2', FALSE, TRUE, 20),
('WAREHOUSE', 'Warehouse', 'Industrial', 'B8', FALSE, TRUE, 21),
('MIXED_USE', 'Mixed Use', 'Mixed', 'MIXED', FALSE, FALSE, 30),
('OTHER', 'Other/Unspecified', 'Other', 'OTHER', FALSE, FALSE, 99),
('UNKNOWN', 'Unknown Type', 'Unknown', NULL, FALSE, FALSE, 100);

-- 7. Application Status Dimension
CREATE TABLE dim_application_status (
    application_status_id  SERIAL PRIMARY KEY,
    status_code           VARCHAR(20) NOT NULL UNIQUE,
    status_name           VARCHAR(50) NOT NULL,
    status_category       VARCHAR(30),
    is_final_status       BOOLEAN DEFAULT FALSE,
    days_typical_duration INTEGER,
    sort_order            INTEGER,
    created_at            TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

INSERT INTO dim_application_status (status_code, status_name, status_category, is_final_status, days_typical_duration, sort_order) VALUES
('SUBMITTED', 'Submitted', 'Initial', FALSE, 0, 1),
('VALIDATED', 'Validated', 'Processing', FALSE, 7, 2),
('CONSULTEE', 'Out for Consultation', 'Processing', FALSE, 21, 3),
('ASSESSMENT', 'Under Assessment', 'Processing', FALSE, 42, 4),
('COMMITTEE', 'Committee Decision', 'Decision', FALSE, 14, 5),
('APPROVED', 'Approved', 'Final', TRUE, NULL, 10),
('REFUSED', 'Refused', 'Final', TRUE, NULL, 11),
('WITHDRAWN', 'Withdrawn', 'Final', TRUE, NULL, 12),
('INVALID', 'Invalid', 'Final', TRUE, NULL, 13),
('UNKNOWN', 'Unknown Status', 'Other', FALSE, NULL, 99);

-- 8. Development Type Dimension
CREATE TABLE dim_development_type (
    development_type_id   SERIAL PRIMARY KEY,
    development_code     VARCHAR(20) NOT NULL UNIQUE,
    development_name     VARCHAR(100) NOT NULL,
    development_category VARCHAR(50),
    impact_level        VARCHAR(20), -- 'Minor', 'Major', 'Significant'
    requires_eia        BOOLEAN DEFAULT FALSE, -- Environmental Impact Assessment
    fee_category        VARCHAR(20),
    description         TEXT,
    sort_order          INTEGER,
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

INSERT INTO dim_development_type (development_code, development_name, development_category, impact_level, requires_eia, sort_order) VALUES
('HOUSEHOLDER', 'Householder Extension', 'Residential', 'Minor', FALSE, 1),
('NEW_DWELLING', 'New Dwelling', 'Residential', 'Major', FALSE, 2),
('SUBDIVISION', 'Plot Subdivision', 'Residential', 'Minor', FALSE, 3),
('CHANGE_USE', 'Change of Use', 'Commercial', 'Major', FALSE, 10),
('NEW_COMMERCIAL', 'New Commercial Building', 'Commercial', 'Major', FALSE, 11),
('INDUSTRIAL', 'Industrial Development', 'Industrial', 'Major', TRUE, 20),
('INFRASTRUCTURE', 'Infrastructure', 'Infrastructure', 'Significant', TRUE, 30),
('DEMOLITION', 'Demolition', 'Other', 'Minor', FALSE, 40),
('LISTED_BUILDING', 'Listed Building Works', 'Heritage', 'Major', FALSE, 50),
('OTHER', 'Other Development', 'Other', 'Minor', FALSE, 90),
('UNKNOWN', 'Unknown Type', 'Unknown', 'Minor', FALSE, 99);

-- 9. Date Dimension (simplified version)
CREATE TABLE dim_date (
    date_id              INTEGER PRIMARY KEY, -- YYYYMMDD format
    date_actual          DATE NOT NULL UNIQUE,
    day_name            VARCHAR(10),
    day_of_week         INTEGER, -- 1=Monday
    day_of_month        INTEGER,
    day_of_year         INTEGER,
    week_of_year        INTEGER,
    month_name          VARCHAR(10),
    month_number        INTEGER,
    quarter             INTEGER,
    year                INTEGER,
    is_weekend          BOOLEAN,
    is_holiday          BOOLEAN DEFAULT FALSE,
    financial_year      INTEGER, -- UK financial year (Apr-Mar)
    financial_quarter   INTEGER
);

-- Populate dim_date with a range of dates (2020-2030)
INSERT INTO dim_date (
    date_id, date_actual, day_name, day_of_week, day_of_month, day_of_year,
    week_of_year, month_name, month_number, quarter, year, is_weekend,
    financial_year, financial_quarter
)
SELECT 
    TO_CHAR(d, 'YYYYMMDD')::INTEGER as date_id,
    d as date_actual,
    TO_CHAR(d, 'Day') as day_name,
    EXTRACT(DOW FROM d) as day_of_week,
    EXTRACT(DAY FROM d) as day_of_month,
    EXTRACT(DOY FROM d) as day_of_year,
    EXTRACT(WEEK FROM d) as week_of_year,
    TO_CHAR(d, 'Month') as month_name,
    EXTRACT(MONTH FROM d) as month_number,
    EXTRACT(QUARTER FROM d) as quarter,
    EXTRACT(YEAR FROM d) as year,
    EXTRACT(DOW FROM d) IN (0,6) as is_weekend,
    -- UK Financial year starts April 1st
    CASE 
        WHEN EXTRACT(MONTH FROM d) >= 4 THEN EXTRACT(YEAR FROM d)
        ELSE EXTRACT(YEAR FROM d) - 1
    END as financial_year,
    CASE 
        WHEN EXTRACT(MONTH FROM d) BETWEEN 4 AND 6 THEN 1
        WHEN EXTRACT(MONTH FROM d) BETWEEN 7 AND 9 THEN 2
        WHEN EXTRACT(MONTH FROM d) BETWEEN 10 AND 12 THEN 3
        ELSE 4
    END as financial_quarter
FROM generate_series('2020-01-01'::DATE, '2030-12-31'::DATE, '1 day') d;

-- Create indexes on dimension tables
CREATE INDEX idx_dim_document_type_code ON dim_document_type(document_type_code);
CREATE INDEX idx_dim_document_status_code ON dim_document_status(status_code);
CREATE INDEX idx_dim_match_method_code ON dim_match_method(method_code);
CREATE INDEX idx_dim_match_decision_code ON dim_match_decision(decision_code);
CREATE INDEX idx_dim_property_type_code ON dim_property_type(property_code);
CREATE INDEX idx_dim_application_status_code ON dim_application_status(status_code);
CREATE INDEX idx_dim_development_type_code ON dim_development_type(development_code);

-- Add comments
COMMENT ON TABLE dim_original_address IS 'Dimension table for original addresses from source documents - deduplicated';
COMMENT ON TABLE dim_document_type IS 'Dimension table for document types (planning applications, building regs, etc.)';
COMMENT ON TABLE dim_match_method IS 'Dimension table for address matching methods and algorithms';
COMMENT ON TABLE dim_match_decision IS 'Dimension table for match confidence decisions';
COMMENT ON TABLE dim_date IS 'Date dimension with financial year calculations for UK public sector';

COMMIT;