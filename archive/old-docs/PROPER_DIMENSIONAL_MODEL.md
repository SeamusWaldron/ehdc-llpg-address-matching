# EHDC LLPG Proper Dimensional Model Design

## Overview

You're absolutely right - the previous design was storing dimensional data directly in the fact table. This redesign follows proper dimensional modeling principles with a lean fact table and proper dimension tables.

## Dimensional Model Architecture

```
Fact Table (fact_documents)
├── dim_document_type
├── dim_document_status  
├── dim_original_address
├── dim_matched_address (existing: dim_address)
├── dim_location (existing)
├── dim_match_method
├── dim_match_decision
├── dim_property_type
├── dim_application_status
├── dim_development_type
└── dim_date (for application_date, decision_date, etc.)
```

## Dimension Tables

### 1. Document Type Dimension
```sql
CREATE TABLE dim_document_type (
    document_type_id        SERIAL PRIMARY KEY,
    document_type_code      VARCHAR(20) NOT NULL UNIQUE,
    document_type_name      VARCHAR(100) NOT NULL,
    document_category       VARCHAR(50),
    description             TEXT,
    is_active              BOOLEAN DEFAULT TRUE,
    created_at             TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Example data
INSERT INTO dim_document_type (document_type_code, document_type_name, document_category) VALUES
('PLAN_APP', 'Planning Application', 'Planning'),
('BUILD_REG', 'Building Regulations', 'Building Control'),
('APPEAL', 'Planning Appeal', 'Planning'),
('ENFORCE', 'Enforcement Notice', 'Enforcement'),
('PREAPP', 'Pre-Application', 'Planning');
```

### 2. Document Status Dimension
```sql
CREATE TABLE dim_document_status (
    document_status_id      SERIAL PRIMARY KEY,
    status_code            VARCHAR(20) NOT NULL UNIQUE,
    status_name            VARCHAR(50) NOT NULL,
    status_category        VARCHAR(30),
    is_active_status       BOOLEAN DEFAULT TRUE,
    sort_order             INTEGER,
    created_at             TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Example data
INSERT INTO dim_document_status (status_code, status_name, status_category, sort_order) VALUES
('ACTIVE', 'Active', 'Current', 1),
('PENDING', 'Pending Review', 'Processing', 2),
('APPROVED', 'Approved', 'Completed', 3),
('REJECTED', 'Rejected', 'Completed', 4),
('WITHDRAWN', 'Withdrawn', 'Completed', 5),
('ARCHIVED', 'Archived', 'Historical', 6);
```

### 3. Original Address Dimension
```sql
CREATE TABLE dim_original_address (
    original_address_id     SERIAL PRIMARY KEY,
    raw_address            TEXT NOT NULL,
    address_hash           VARCHAR(64) NOT NULL UNIQUE, -- MD5/SHA hash for deduplication
    
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
CREATE INDEX idx_dim_original_address_postcode ON dim_original_address(std_postcode);
CREATE INDEX idx_dim_original_address_road_city ON dim_original_address(std_road, std_city);
CREATE UNIQUE INDEX idx_dim_original_address_raw ON dim_original_address(raw_address);
```

### 4. Match Method Dimension
```sql
CREATE TABLE dim_match_method (
    match_method_id        SERIAL PRIMARY KEY,
    method_code           VARCHAR(50) NOT NULL UNIQUE,
    method_name           VARCHAR(100) NOT NULL,
    method_category       VARCHAR(50),
    confidence_threshold  DECIMAL(3,2),
    description           TEXT,
    algorithm_version     VARCHAR(20),
    is_active            BOOLEAN DEFAULT TRUE,
    created_at           TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Example data
INSERT INTO dim_match_method (method_code, method_name, method_category, confidence_threshold) VALUES
('EXACT_COMPONENTS', 'Exact Component Match', 'High Precision', 0.95),
('POSTCODE_HOUSE', 'Postcode + House Number', 'High Precision', 0.90),
('ROAD_CITY_EXACT', 'Exact Road + City', 'Medium Precision', 0.85),
('ROAD_CITY_FUZZY', 'Fuzzy Road + City', 'Medium Precision', 0.75),
('FUZZY_ROAD', 'Fuzzy Road Only', 'Low Precision', 0.60),
('MANUAL_MATCH', 'Manual Override', 'Manual', 1.00),
('NO_MATCH', 'No Match Found', 'None', 0.00);
```

### 5. Match Decision Dimension
```sql
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

-- Example data
INSERT INTO dim_match_decision (decision_code, decision_name, auto_process, requires_review, confidence_min, confidence_max) VALUES
('AUTO_ACCEPT', 'Auto Accept', TRUE, FALSE, 0.85, 1.00),
('NEEDS_REVIEW', 'Needs Review', FALSE, TRUE, 0.50, 0.84),
('LOW_CONFIDENCE', 'Low Confidence', FALSE, TRUE, 0.20, 0.49),
('NO_MATCH', 'No Match', FALSE, FALSE, 0.00, 0.19),
('MANUAL_OVERRIDE', 'Manual Override', TRUE, FALSE, 0.00, 1.00);
```

### 6. Property Type Dimension
```sql
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

-- Example data
INSERT INTO dim_property_type (property_code, property_name, property_category, is_residential) VALUES
('HOUSE', 'House', 'Residential', TRUE),
('FLAT', 'Flat/Apartment', 'Residential', TRUE),
('BUNGALOW', 'Bungalow', 'Residential', TRUE),
('OFFICE', 'Office Building', 'Commercial', FALSE),
('RETAIL', 'Retail Unit', 'Commercial', FALSE),
('INDUSTRIAL', 'Industrial Unit', 'Commercial', FALSE),
('MIXED_USE', 'Mixed Use', 'Mixed', FALSE);
```

### 7. Application Status Dimension
```sql
CREATE TABLE dim_application_status (
    application_status_id  SERIAL PRIMARY KEY,
    status_code           VARCHAR(20) NOT NULL UNIQUE,
    status_name           VARCHAR(50) NOT NULL,
    status_category       VARCHAR(30),
    is_final_status       BOOLEAN DEFAULT FALSE,
    days_typical_duration INTEGER,
    next_possible_statuses TEXT[], -- Array of possible next statuses
    sort_order            INTEGER,
    created_at            TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### 8. Development Type Dimension
```sql
CREATE TABLE dim_development_type (
    development_type_id   SERIAL PRIMARY KEY,
    development_code     VARCHAR(20) NOT NULL UNIQUE,
    development_name     VARCHAR(100) NOT NULL,
    development_category VARCHAR(50),
    impact_level        VARCHAR(20), -- 'Minor', 'Major', 'Significant'
    requires_eia        BOOLEAN DEFAULT FALSE, -- Environmental Impact Assessment
    description         TEXT,
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### 9. Date Dimension (Optional but recommended)
```sql
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
    is_holiday          BOOLEAN,
    financial_year      INTEGER, -- UK financial year
    financial_quarter   INTEGER
);
```

## Lean Fact Table

```sql
CREATE TABLE fact_documents (
    -- Fact table surrogate key
    fact_id                    BIGSERIAL PRIMARY KEY,
    
    -- Business key
    document_id               BIGINT NOT NULL UNIQUE,
    
    -- Dimension foreign keys
    document_type_id          INTEGER REFERENCES dim_document_type(document_type_id),
    document_status_id        INTEGER REFERENCES dim_document_status(document_status_id),
    original_address_id       INTEGER REFERENCES dim_original_address(original_address_id),
    matched_address_id        INTEGER REFERENCES dim_address(address_id), -- Existing LLPG
    matched_location_id       INTEGER REFERENCES dim_location(location_id), -- Existing
    match_method_id           INTEGER REFERENCES dim_match_method(match_method_id),
    match_decision_id         INTEGER REFERENCES dim_match_decision(match_decision_id),
    property_type_id          INTEGER REFERENCES dim_property_type(property_type_id),
    application_status_id     INTEGER REFERENCES dim_application_status(application_status_id),
    development_type_id       INTEGER REFERENCES dim_development_type(development_type_id),
    
    -- Date dimensions
    application_date_id       INTEGER REFERENCES dim_date(date_id),
    decision_date_id          INTEGER REFERENCES dim_date(date_id),
    import_date_id            INTEGER REFERENCES dim_date(date_id),
    
    -- Measures (facts/metrics)
    match_confidence_score    DECIMAL(5,4), -- 0.0000 to 1.0000
    address_quality_score     DECIMAL(3,2), -- 0.00 to 1.00
    data_completeness_score   DECIMAL(3,2), -- 0.00 to 1.00
    processing_time_ms        INTEGER,      -- Time taken to process
    
    -- Business measures
    application_fee           DECIMAL(10,2),
    estimated_value          DECIMAL(12,2),
    floor_area_sqm           DECIMAL(10,2),
    
    -- Technical fields
    import_batch_id          INTEGER,
    original_filename        VARCHAR(255),
    planning_reference       VARCHAR(50), -- Keep as it's a true business identifier
    
    -- Flags (boolean measures)
    is_matched               BOOLEAN GENERATED ALWAYS AS (matched_address_id IS NOT NULL) STORED,
    is_auto_processed        BOOLEAN DEFAULT FALSE,
    has_validation_issues    BOOLEAN DEFAULT FALSE,
    
    -- Additional flexible data (use sparingly)
    additional_measures      JSONB,
    
    -- Audit measures
    created_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processing_version      VARCHAR(20) DEFAULT '1.0'
);
```

## Key Improvements

### 1. Significant Size Reduction
- **Before**: ~50+ columns with repeated text
- **After**: ~25 columns with integer foreign keys
- **Storage Savings**: 60-70% reduction in fact table size

### 2. Referential Integrity
- All lookups enforced by foreign key constraints
- Consistent values across all records
- Easy to add new types without schema changes

### 3. Performance Benefits
- Integer joins much faster than text comparisons  
- Smaller fact table = better cache utilization
- Dimension tables can be heavily cached

### 4. Maintenance Advantages
- Change property type name once in dimension
- Easy to add new document types
- Historical tracking in dimensions
- Analytics-friendly star schema

## Migration Strategy

### Phase 1: Create Dimensions
```sql
-- Create all dimension tables
-- Populate with existing distinct values from staging tables
INSERT INTO dim_original_address (raw_address, address_hash, ...)
SELECT DISTINCT raw_address, MD5(raw_address), ...
FROM src_document;
```

### Phase 2: Create Lean Fact Table
```sql
-- Create fact table with FK references
-- Populate through joins to get dimension IDs
INSERT INTO fact_documents (document_id, document_type_id, original_address_id, ...)
SELECT 
    sd.document_id,
    dt.document_type_id,
    oa.original_address_id,
    ...
FROM src_document sd
JOIN dim_document_type dt ON sd.document_type = dt.document_type_code
JOIN dim_original_address oa ON sd.raw_address = oa.raw_address
...
```

### Phase 3: Create Optimized Views
```sql
-- Business-friendly views that join dimensions back
CREATE VIEW vw_fact_documents_detailed AS
SELECT 
    f.fact_id,
    f.document_id,
    dt.document_type_name,
    ds.status_name,
    oa.raw_address,
    da.full_address as matched_address,
    mm.method_name as match_method,
    f.match_confidence_score,
    ...
FROM fact_documents f
JOIN dim_document_type dt ON f.document_type_id = dt.document_type_id
JOIN dim_document_status ds ON f.document_status_id = ds.document_status_id
JOIN dim_original_address oa ON f.original_address_id = oa.original_address_id
LEFT JOIN dim_address da ON f.matched_address_id = da.address_id
...
```

## Benefits of This Approach

✅ **60-70% smaller fact table** (integers vs repeated text)  
✅ **Referential integrity** enforced  
✅ **Much faster queries** (integer joins)  
✅ **Easy maintenance** (change dimension once)  
✅ **Analytics-ready** star schema  
✅ **Deduplication** of original addresses  
✅ **Historical tracking** in dimensions  
✅ **Extensible** (add new types easily)  

This is now a proper dimensional model that will scale much better and follow data warehousing best practices!