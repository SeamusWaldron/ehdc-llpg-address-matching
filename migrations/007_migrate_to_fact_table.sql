-- Migration 007: Migrate Data to Fact Documents Table
-- Purpose: Populate fact_documents with consolidated data from staging tables
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- First, let's check if we need to create a match_method lookup table
-- if it doesn't exist (based on the component matching we've been doing)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'match_method') THEN
        CREATE TABLE match_method (
            method_id SERIAL PRIMARY KEY,
            method_name VARCHAR(100) NOT NULL,
            method_code VARCHAR(50) NOT NULL UNIQUE,
            description TEXT,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        );
        
        INSERT INTO match_method (method_name, method_code, description) VALUES
        ('Exact Components', 'exact_components', 'Perfect match on multiple address components'),
        ('Postcode + House Number', 'postcode_house', 'Match on postcode and house number'),
        ('Road + City Exact', 'road_city_exact', 'Exact match on road and city'),
        ('Road + City Fuzzy', 'road_city_fuzzy', 'Fuzzy match on road and city'),
        ('Fuzzy Road', 'fuzzy_road', 'Fuzzy matching on road name only'),
        ('No Match', 'no_match', 'No suitable match found'),
        ('Manual Review', 'manual_review', 'Requires manual verification');
    END IF;
END $$;

-- Create a temporary view to check our data before migration
CREATE OR REPLACE VIEW vw_migration_preview AS
SELECT 
    sd.document_id,
    sd.raw_address,
    sd.gopostal_processed,
    am.confidence_score,
    am.match_status,
    da.uprn,
    da.full_address,
    dl.easting,
    dl.northing,
    ST_Y(dl.geom) as latitude,
    ST_X(dl.geom) as longitude
FROM src_document sd
LEFT JOIN address_match am ON sd.document_id = am.document_id
LEFT JOIN dim_address da ON am.address_id = da.address_id
LEFT JOIN dim_location dl ON da.location_id = dl.location_id
ORDER BY sd.document_id
LIMIT 10;

-- Display preview
SELECT 'Migration Preview - First 10 Records:' as info;
SELECT * FROM vw_migration_preview;

-- Get counts for validation
SELECT 
    'Pre-Migration Counts' as info,
    (SELECT COUNT(*) FROM src_document) as src_documents,
    (SELECT COUNT(*) FROM address_match) as address_matches,
    (SELECT COUNT(*) FROM src_document WHERE gopostal_processed = TRUE) as gopostal_processed;

-- Main migration query
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
    
    CASE 
        WHEN am.document_id IS NOT NULL THEN 
            CASE am.match_method_id
                WHEN 13 THEN 'exact_components'
                WHEN 8 THEN 'postcode_house'
                WHEN 9 THEN 'road_city_exact'
                WHEN 10 THEN 'fuzzy_road'
                ELSE 'component_match'
            END
        ELSE 'no_match'
    END as match_method,
    
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
        'original_source', COALESCE(sd.source_system, 'unknown'),
        'import_notes', sd.import_notes,
        'validation_status', sd.validation_status,
        'original_record_id', sd.original_record_id
    ) as additional_data,
    
    -- Data quality scores
    CASE 
        WHEN am.confidence_score IS NOT NULL THEN am.confidence_score
        WHEN sd.gopostal_processed = TRUE THEN 0.3 -- Processed but no match
        ELSE 0.1 -- Not processed
    END as address_quality_score,
    
    -- Completeness score based on available fields
    (
        CASE WHEN sd.raw_address IS NOT NULL AND sd.raw_address != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.postcode IS NOT NULL AND sd.postcode != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.planning_reference IS NOT NULL AND sd.planning_reference != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.application_date IS NOT NULL THEN 0.2 ELSE 0 END +
        CASE WHEN sd.property_type IS NOT NULL AND sd.property_type != '' THEN 0.2 ELSE 0 END
    ) as data_completeness_score,
    
    -- Validation flags
    ARRAY(
        SELECT flag FROM (
            SELECT CASE WHEN sd.raw_address IS NULL OR sd.raw_address = '' THEN 'missing_address' END
            UNION SELECT CASE WHEN sd.postcode IS NULL OR sd.postcode = '' THEN 'missing_postcode' END
            UNION SELECT CASE WHEN sd.gopostal_processed = FALSE THEN 'not_processed' END
            UNION SELECT CASE WHEN am.document_id IS NULL AND sd.gopostal_processed = TRUE THEN 'no_match_found' END
            UNION SELECT CASE WHEN am.confidence_score IS NOT NULL AND am.confidence_score < 0.7 THEN 'low_confidence' END
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
ORDER BY sd.document_id;

-- Get the number of records migrated
SELECT 
    'Migration Results' as info,
    COUNT(*) as total_migrated,
    COUNT(CASE WHEN match_status = 'matched' THEN 1 END) as matched_records,
    COUNT(CASE WHEN match_status = 'no_match' THEN 1 END) as no_match_records,
    COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) as with_uprn,
    ROUND(100.0 * COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) / COUNT(*), 2) as uprn_coverage_pct
FROM fact_documents;

-- Update table statistics
ANALYZE fact_documents;

-- Drop the temporary view
DROP VIEW vw_migration_preview;

COMMIT;