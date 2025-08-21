# EHDC LLPG Fact Table Design & Migration Strategy

## Overview

This document outlines the design for the final normalized fact table that consolidates all source document data with matched address information, creating a unified operational dataset with UPRN associations.

## Current State Analysis

### Existing Tables
1. **`src_document`** - Raw imported documents with parsed address components
2. **`address_match`** - Matching results linking documents to LLPG addresses
3. **`dim_address`** - LLPG gold standard addresses with UPRNs
4. **`dim_location`** - Geographic coordinates and spatial data

### Data Flow
```
Source Documents → Component Parsing → Address Matching → Fact Table
     ↓                    ↓                 ↓              ↓
src_document      gopostal_*        address_match    fact_documents
```

## Fact Table Schema Design

### Core Design Principles
1. **Single Source of Truth**: One record per source document
2. **UPRN Association**: Every record linked to matched or null UPRN
3. **Address Standardization**: Both original and matched addresses preserved
4. **Audit Trail**: Complete lineage from import to final state
5. **Performance Optimization**: Indexed for operational queries

### Proposed Schema: `fact_documents`

```sql
CREATE TABLE fact_documents (
    -- Primary Keys
    document_id             BIGINT PRIMARY KEY,
    fact_id                 BIGSERIAL UNIQUE NOT NULL,
    
    -- Source Document Information
    original_filename       VARCHAR(255),
    import_batch_id         INTEGER,
    import_timestamp        TIMESTAMP WITH TIME ZONE,
    document_type           VARCHAR(50),
    document_status         VARCHAR(20) DEFAULT 'active',
    
    -- Original Address Data
    raw_address             TEXT,
    parsed_address_line_1   VARCHAR(255),
    parsed_address_line_2   VARCHAR(255),
    parsed_town             VARCHAR(100),
    parsed_county           VARCHAR(100),
    parsed_postcode         VARCHAR(10),
    parsed_country          VARCHAR(50),
    
    -- Standardized Address Components (from gopostal)
    std_house_number        VARCHAR(20),
    std_house_name          VARCHAR(100),
    std_road                VARCHAR(200),
    std_suburb              VARCHAR(100),
    std_city                VARCHAR(100),
    std_state_district      VARCHAR(100),
    std_state               VARCHAR(100),
    std_postcode            VARCHAR(10),
    std_country             VARCHAR(50),
    std_unit                VARCHAR(50),
    
    -- Address Matching Results
    match_status            VARCHAR(20), -- 'matched', 'no_match', 'needs_review'
    match_method            VARCHAR(50), -- 'exact_components', 'postcode_house', etc.
    match_confidence        DECIMAL(5,4), -- 0.0000 to 1.0000
    match_decision          VARCHAR(20), -- 'auto_accept', 'needs_review', 'low_confidence'
    matched_uprn            VARCHAR(20), -- The golden UPRN if matched
    matched_address_id      INTEGER, -- Reference to dim_address
    matched_location_id     INTEGER, -- Reference to dim_location
    
    -- Matched Address Information (denormalized for performance)
    matched_full_address    TEXT,
    matched_address_canonical TEXT,
    matched_easting         DECIMAL(10,2),
    matched_northing        DECIMAL(10,2),
    matched_latitude        DECIMAL(10,8),
    matched_longitude       DECIMAL(11,8),
    
    -- Business Data Fields
    property_type           VARCHAR(100),
    property_description    TEXT,
    planning_reference      VARCHAR(50),
    application_date        DATE,
    decision_date           DATE,
    application_status      VARCHAR(50),
    development_type        VARCHAR(100),
    
    -- Additional Source Fields (flexible JSON for varying schemas)
    additional_data         JSONB,
    
    -- Data Quality Flags
    address_quality_score   DECIMAL(3,2), -- 0.00 to 1.00
    data_completeness_score DECIMAL(3,2), -- 0.00 to 1.00
    validation_flags        TEXT[], -- Array of validation warnings/notes
    
    -- Audit Information
    created_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processed_by            VARCHAR(50) DEFAULT 'system',
    processing_version      VARCHAR(20) DEFAULT '1.0',
    
    -- Foreign Key Constraints
    CONSTRAINT fk_matched_address FOREIGN KEY (matched_address_id) 
        REFERENCES dim_address(address_id),
    CONSTRAINT fk_matched_location FOREIGN KEY (matched_location_id) 
        REFERENCES dim_location(location_id),
        
    -- Check Constraints
    CONSTRAINT chk_match_confidence CHECK (match_confidence >= 0.0 AND match_confidence <= 1.0),
    CONSTRAINT chk_address_quality CHECK (address_quality_score >= 0.0 AND address_quality_score <= 1.0),
    CONSTRAINT chk_data_completeness CHECK (data_completeness_score >= 0.0 AND data_completeness_score <= 1.0),
    CONSTRAINT chk_match_status CHECK (match_status IN ('matched', 'no_match', 'needs_review', 'pending')),
    CONSTRAINT chk_match_decision CHECK (match_decision IN ('auto_accept', 'needs_review', 'low_confidence', 'no_match'))
);
```

## Indexes for Performance

```sql
-- Primary access patterns
CREATE INDEX idx_fact_documents_uprn ON fact_documents(matched_uprn) WHERE matched_uprn IS NOT NULL;
CREATE INDEX idx_fact_documents_match_status ON fact_documents(match_status);
CREATE INDEX idx_fact_documents_postcode ON fact_documents(std_postcode) WHERE std_postcode IS NOT NULL;
CREATE INDEX idx_fact_documents_location ON fact_documents(matched_location_id) WHERE matched_location_id IS NOT NULL;

-- Spatial index for geographic queries
CREATE INDEX idx_fact_documents_spatial ON fact_documents USING GIST(
    ST_Point(matched_longitude, matched_latitude)
) WHERE matched_longitude IS NOT NULL AND matched_latitude IS NOT NULL;

-- Business query patterns
CREATE INDEX idx_fact_documents_planning_ref ON fact_documents(planning_reference) WHERE planning_reference IS NOT NULL;
CREATE INDEX idx_fact_documents_app_date ON fact_documents(application_date) WHERE application_date IS NOT NULL;
CREATE INDEX idx_fact_documents_property_type ON fact_documents(property_type) WHERE property_type IS NOT NULL;

-- Data quality indexes
CREATE INDEX idx_fact_documents_quality ON fact_documents(address_quality_score DESC, data_completeness_score DESC);
CREATE INDEX idx_fact_documents_confidence ON fact_documents(match_confidence DESC) WHERE match_confidence IS NOT NULL;

-- Composite indexes for common queries
CREATE INDEX idx_fact_documents_status_confidence ON fact_documents(match_status, match_confidence DESC);
CREATE INDEX idx_fact_documents_type_status ON fact_documents(document_type, match_status);
```

## Migration Strategy

### Phase 1: Table Creation and Preparation

```sql
-- Create the fact table
\i create_fact_documents_table.sql

-- Create staging views for data validation
CREATE VIEW vw_migration_preview AS
SELECT 
    sd.document_id,
    sd.raw_address,
    sd.gopostal_processed,
    am.match_method_id,
    am.confidence_score,
    am.match_status,
    da.uprn,
    da.full_address,
    dl.easting,
    dl.northing
FROM src_document sd
LEFT JOIN address_match am ON sd.document_id = am.document_id
LEFT JOIN dim_address da ON am.address_id = da.address_id
LEFT JOIN dim_location dl ON da.location_id = dl.location_id
ORDER BY sd.document_id;
```

### Phase 2: Data Migration Script

```sql
-- Migration script: migrate_to_fact_table.sql
INSERT INTO fact_documents (
    document_id,
    original_filename,
    import_batch_id,
    import_timestamp,
    document_type,
    
    -- Original address data
    raw_address,
    parsed_address_line_1,
    parsed_address_line_2,
    parsed_town,
    parsed_county,
    parsed_postcode,
    parsed_country,
    
    -- Standardized components
    std_house_number,
    std_house_name,
    std_road,
    std_suburb,
    std_city,
    std_state_district,
    std_state,
    std_postcode,
    std_country,
    std_unit,
    
    -- Matching results
    match_status,
    match_method,
    match_confidence,
    match_decision,
    matched_uprn,
    matched_address_id,
    matched_location_id,
    
    -- Matched address details (denormalized)
    matched_full_address,
    matched_address_canonical,
    matched_easting,
    matched_northing,
    matched_latitude,
    matched_longitude,
    
    -- Business data
    property_type,
    property_description,
    planning_reference,
    application_date,
    decision_date,
    application_status,
    development_type,
    additional_data,
    
    -- Data quality metrics
    address_quality_score,
    data_completeness_score,
    validation_flags,
    
    -- Audit fields
    created_at,
    processed_by,
    processing_version
)
SELECT 
    sd.document_id,
    sd.filename,
    sd.batch_id,
    sd.imported_at,
    COALESCE(sd.document_type, 'planning_application'),
    
    -- Original address
    sd.raw_address,
    sd.address_line_1,
    sd.address_line_2,
    sd.town,
    sd.county,
    sd.postcode,
    sd.country,
    
    -- Standardized components from gopostal
    sd.gopostal_house_number,
    sd.gopostal_house,
    sd.gopostal_road,
    sd.gopostal_suburb,
    sd.gopostal_city,
    sd.gopostal_state_district,
    sd.gopostal_state,
    sd.gopostal_postcode,
    sd.gopostal_country,
    sd.gopostal_unit,
    
    -- Matching results
    CASE 
        WHEN am.document_id IS NOT NULL THEN 'matched'
        WHEN sd.gopostal_processed = TRUE THEN 'no_match'
        ELSE 'pending'
    END as match_status,
    
    COALESCE(mm.method_code, 'no_match') as match_method,
    am.confidence_score,
    
    CASE 
        WHEN am.confidence_score >= 0.95 THEN 'auto_accept'
        WHEN am.confidence_score >= 0.70 THEN 'needs_review'
        WHEN am.confidence_score >= 0.50 THEN 'low_confidence'
        ELSE 'no_match'
    END as match_decision,
    
    da.uprn,
    am.address_id,
    am.location_id,
    
    -- Matched address details (denormalized for performance)
    da.full_address,
    da.address_canonical,
    dl.easting,
    dl.northing,
    ST_Y(dl.geom) as matched_latitude,
    ST_X(dl.geom) as matched_longitude,
    
    -- Business data from source documents
    sd.property_type,
    sd.property_description,
    sd.planning_reference,
    sd.application_date,
    sd.decision_date,
    sd.application_status,
    sd.development_type,
    
    -- Additional data as JSONB
    jsonb_build_object(
        'original_source', sd.source_system,
        'import_notes', sd.import_notes,
        'validation_status', sd.validation_status
    ) as additional_data,
    
    -- Data quality scores
    CASE 
        WHEN am.confidence_score IS NOT NULL THEN am.confidence_score
        WHEN sd.gopostal_processed = TRUE THEN 0.3 -- Processed but no match
        ELSE 0.1 -- Not processed
    END as address_quality_score,
    
    -- Completeness score based on available fields
    (
        CASE WHEN sd.raw_address IS NOT NULL THEN 0.2 ELSE 0 END +
        CASE WHEN sd.postcode IS NOT NULL THEN 0.2 ELSE 0 END +
        CASE WHEN sd.planning_reference IS NOT NULL THEN 0.2 ELSE 0 END +
        CASE WHEN sd.application_date IS NOT NULL THEN 0.2 ELSE 0 END +
        CASE WHEN sd.property_type IS NOT NULL THEN 0.2 ELSE 0 END
    ) as data_completeness_score,
    
    -- Validation flags
    ARRAY(
        SELECT flag FROM (
            SELECT CASE WHEN sd.raw_address IS NULL THEN 'missing_address' END
            UNION SELECT CASE WHEN sd.postcode IS NULL THEN 'missing_postcode' END
            UNION SELECT CASE WHEN sd.gopostal_processed = FALSE THEN 'not_processed' END
            UNION SELECT CASE WHEN am.document_id IS NULL AND sd.gopostal_processed = TRUE THEN 'no_match_found' END
        ) flags(flag) 
        WHERE flag IS NOT NULL
    ) as validation_flags,
    
    -- Audit information
    NOW() as created_at,
    'migration_v1.0' as processed_by,
    '1.0' as processing_version

FROM src_document sd
LEFT JOIN address_match am ON sd.document_id = am.document_id
LEFT JOIN dim_address da ON am.address_id = da.address_id
LEFT JOIN dim_location dl ON da.location_id = dl.location_id
LEFT JOIN match_method mm ON am.match_method_id = mm.method_id
ORDER BY sd.document_id;
```

## Data Quality Validation

### Post-Migration Validation Queries

```sql
-- 1. Migration completeness check
SELECT 
    'Source Documents' as table_name,
    COUNT(*) as record_count
FROM src_document
UNION ALL
SELECT 
    'Fact Documents' as table_name,
    COUNT(*) as record_count
FROM fact_documents;

-- 2. Match status distribution
SELECT 
    match_status,
    COUNT(*) as count,
    ROUND(100.0 * COUNT(*) / SUM(COUNT(*)) OVER(), 2) as percentage
FROM fact_documents
GROUP BY match_status
ORDER BY count DESC;

-- 3. Data quality summary
SELECT 
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_completeness,
    COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) as with_uprn,
    COUNT(*) as total_records,
    ROUND(100.0 * COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) / COUNT(*), 2) as uprn_coverage_pct
FROM fact_documents;

-- 4. Geographic coverage
SELECT 
    COUNT(CASE WHEN matched_latitude IS NOT NULL AND matched_longitude IS NOT NULL THEN 1 END) as with_coordinates,
    COUNT(CASE WHEN matched_easting IS NOT NULL AND matched_northing IS NOT NULL THEN 1 END) as with_grid_ref,
    COUNT(*) as total,
    ROUND(100.0 * COUNT(CASE WHEN matched_latitude IS NOT NULL THEN 1 END) / COUNT(*), 2) as coordinate_coverage_pct
FROM fact_documents;

-- 5. Business data completeness
SELECT 
    COUNT(CASE WHEN planning_reference IS NOT NULL THEN 1 END) as with_planning_ref,
    COUNT(CASE WHEN application_date IS NOT NULL THEN 1 END) as with_app_date,
    COUNT(CASE WHEN property_type IS NOT NULL THEN 1 END) as with_property_type,
    COUNT(*) as total
FROM fact_documents;
```

## Operational Views

### Create useful views for business users

```sql
-- High-quality matched records
CREATE VIEW vw_high_quality_matches AS
SELECT 
    document_id,
    matched_uprn,
    raw_address,
    matched_full_address,
    match_confidence,
    planning_reference,
    application_date,
    property_type,
    matched_easting,
    matched_northing
FROM fact_documents
WHERE match_status = 'matched'
  AND match_confidence >= 0.85
  AND matched_uprn IS NOT NULL;

-- Records needing manual review
CREATE VIEW vw_needs_review AS
SELECT 
    document_id,
    raw_address,
    matched_full_address,
    match_confidence,
    match_method,
    validation_flags,
    planning_reference,
    property_type
FROM fact_documents
WHERE match_decision = 'needs_review'
ORDER BY match_confidence DESC;

-- Unmatched addresses for investigation
CREATE VIEW vw_unmatched_addresses AS
SELECT 
    document_id,
    raw_address,
    std_postcode,
    std_road,
    std_city,
    validation_flags,
    planning_reference,
    property_type,
    data_completeness_score
FROM fact_documents
WHERE match_status = 'no_match'
ORDER BY data_completeness_score DESC;

-- Geographic summary by area
CREATE VIEW vw_geographic_summary AS
SELECT 
    COALESCE(std_city, 'Unknown') as area,
    COUNT(*) as total_documents,
    COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) as matched_documents,
    ROUND(100.0 * COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) / COUNT(*), 2) as match_rate_pct,
    ROUND(AVG(match_confidence), 3) as avg_confidence
FROM fact_documents
WHERE std_city IS NOT NULL
GROUP BY std_city
HAVING COUNT(*) >= 5
ORDER BY match_rate_pct DESC, total_documents DESC;
```

## Migration Execution Plan

### 1. Pre-Migration Steps
```bash
# Backup current data
pg_dump -h localhost -p 15435 -U postgres -d ehdc_llpg -t src_document > src_document_backup.sql
pg_dump -h localhost -p 15435 -U postgres -d ehdc_llpg -t address_match > address_match_backup.sql

# Verify data integrity
psql -h localhost -p 15435 -U postgres -d ehdc_llpg -c "SELECT COUNT(*) FROM src_document;"
psql -h localhost -p 15435 -U postgres -d ehdc_llpg -c "SELECT COUNT(*) FROM address_match;"
```

### 2. Execute Migration
```bash
# Create fact table
psql -h localhost -p 15435 -U postgres -d ehdc_llpg -f create_fact_documents_table.sql

# Run migration
psql -h localhost -p 15435 -U postgres -d ehdc_llpg -f migrate_to_fact_table.sql

# Create indexes
psql -h localhost -p 15435 -U postgres -d ehdc_llpg -f create_fact_table_indexes.sql

# Create views
psql -h localhost -p 15435 -U postgres -d ehdc_llpg -f create_operational_views.sql
```

### 3. Post-Migration Validation
```bash
# Run validation queries
psql -h localhost -p 15435 -U postgres -d ehdc_llpg -f validate_migration.sql

# Generate final statistics
psql -h localhost -p 15435 -U postgres -d ehdc_llpg -c "SELECT * FROM vw_geographic_summary;"
```

## Benefits of This Design

1. **Single Source of Truth**: All document data consolidated with address matching
2. **Performance Optimized**: Denormalized key fields for fast queries
3. **Audit Trail**: Complete lineage from import to final state
4. **Quality Metrics**: Built-in data quality scoring and validation
5. **Business Ready**: Structured for operational reporting and analysis
6. **Scalable**: Supports future enhancements and additional data sources

This fact table will serve as the foundation for all operational queries, reporting, and business intelligence needs while maintaining the complete address matching provenance.