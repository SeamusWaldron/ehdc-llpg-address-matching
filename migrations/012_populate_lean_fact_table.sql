-- Migration 012: Populate Lean Fact Table
-- Purpose: Populate fact_documents_lean with foreign key references to dimension tables
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- Helper function to convert date to date_id format (YYYYMMDD)
CREATE OR REPLACE FUNCTION date_to_date_id(input_date DATE)
RETURNS INTEGER AS $$
BEGIN
    IF input_date IS NULL THEN
        RETURN NULL;
    END IF;
    RETURN TO_CHAR(input_date, 'YYYYMMDD')::INTEGER;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Helper function to get or create unknown dimension IDs
CREATE OR REPLACE FUNCTION get_unknown_document_type_id()
RETURNS INTEGER AS $$
DECLARE
    type_id INTEGER;
BEGIN
    SELECT document_type_id INTO type_id 
    FROM dim_document_type 
    WHERE document_type_code = 'UNKNOWN';
    
    IF type_id IS NULL THEN
        INSERT INTO dim_document_type (document_type_code, document_type_name, document_category)
        VALUES ('UNKNOWN', 'Unknown Type', 'Other')
        RETURNING document_type_id INTO type_id;
    END IF;
    
    RETURN type_id;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION get_unknown_document_status_id()
RETURNS INTEGER AS $$
DECLARE
    status_id INTEGER;
BEGIN
    SELECT document_status_id INTO status_id 
    FROM dim_document_status 
    WHERE status_code = 'UNKNOWN';
    
    IF status_id IS NULL THEN
        INSERT INTO dim_document_status (status_code, status_name, status_category)
        VALUES ('UNKNOWN', 'Unknown Status', 'Other')
        RETURNING document_status_id INTO status_id;
    END IF;
    
    RETURN status_id;
END;
$$ LANGUAGE plpgsql;

-- Create temporary lookup functions for match methods
CREATE OR REPLACE FUNCTION get_match_method_id(method_code TEXT)
RETURNS INTEGER AS $$
DECLARE
    method_id INTEGER;
BEGIN
    SELECT match_method_id INTO method_id 
    FROM dim_match_method 
    WHERE method_code = COALESCE(method_code, 'NO_MATCH');
    
    IF method_id IS NULL THEN
        SELECT match_method_id INTO method_id 
        FROM dim_match_method 
        WHERE method_code = 'NO_MATCH';
    END IF;
    
    RETURN method_id;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION get_match_decision_id(decision_code TEXT)
RETURNS INTEGER AS $$
DECLARE
    decision_id INTEGER;
BEGIN
    SELECT match_decision_id INTO decision_id 
    FROM dim_match_decision 
    WHERE decision_code = COALESCE(decision_code, 'NO_MATCH');
    
    IF decision_id IS NULL THEN
        SELECT match_decision_id INTO decision_id 
        FROM dim_match_decision 
        WHERE decision_code = 'NO_MATCH';
    END IF;
    
    RETURN decision_id;
END;
$$ LANGUAGE plpgsql;

-- Main population query for the lean fact table
INSERT INTO fact_documents_lean (
    document_id,
    document_type_id,
    document_status_id,
    original_address_id,
    matched_address_id,
    matched_location_id,
    match_method_id,
    match_decision_id,
    property_type_id,
    application_status_id,
    development_type_id,
    application_date_id,
    decision_date_id,
    import_date_id,
    match_confidence_score,
    address_quality_score,
    data_completeness_score,
    processing_time_ms,
    application_fee,
    estimated_value,
    floor_area_sqm,
    import_batch_id,
    planning_reference,
    is_auto_processed,
    has_validation_issues,
    additional_measures,
    created_at,
    processing_version
)
SELECT 
    sd.document_id,
    
    -- Document type dimension lookup
    COALESCE(
        dt.document_type_id,
        get_unknown_document_type_id()
    ) as document_type_id,
    
    -- Document status dimension lookup
    COALESCE(
        ds.document_status_id,
        get_unknown_document_status_id()
    ) as document_status_id,
    
    -- Original address dimension lookup (required)
    oa.original_address_id,
    
    -- Matched address and location (may be null)
    am.address_id as matched_address_id,
    am.location_id as matched_location_id,
    
    -- Match method dimension lookup
    get_match_method_id(
        CASE 
            WHEN am.document_id IS NOT NULL THEN 
                CASE am.match_method_id
                    WHEN 13 THEN 'EXACT_COMPONENTS'
                    WHEN 8 THEN 'POSTCODE_HOUSE'
                    WHEN 9 THEN 'ROAD_CITY_EXACT'
                    WHEN 10 THEN 'FUZZY_ROAD'
                    ELSE 'EXACT_COMPONENTS'
                END
            ELSE 'NO_MATCH'
        END
    ) as match_method_id,
    
    -- Match decision dimension lookup
    get_match_decision_id(
        CASE 
            WHEN am.confidence_score >= 0.85 THEN 'AUTO_ACCEPT'
            WHEN am.confidence_score >= 0.50 THEN 'NEEDS_REVIEW'
            WHEN am.confidence_score >= 0.20 THEN 'LOW_CONFIDENCE'
            ELSE 'NO_MATCH'
        END
    ) as match_decision_id,
    
    -- Property type dimension lookup (optional)
    pt.property_type_id,
    
    -- Application status dimension lookup (optional)
    ast.application_status_id,
    
    -- Development type dimension lookup (optional)
    devt.development_type_id,
    
    -- Date dimensions
    date_to_date_id(sd.application_date::date) as application_date_id,
    date_to_date_id(sd.decision_date::date) as decision_date_id,
    date_to_date_id(COALESCE(sd.imported_at, NOW())::date) as import_date_id,
    
    -- Measures (numerical facts)
    am.confidence_score as match_confidence_score,
    
    -- Address quality score calculation
    CASE 
        WHEN am.confidence_score IS NOT NULL THEN am.confidence_score
        WHEN sd.gopostal_processed = TRUE THEN 0.3
        ELSE 0.1
    END as address_quality_score,
    
    -- Data completeness score
    (
        CASE WHEN sd.raw_address IS NOT NULL AND sd.raw_address != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.postcode IS NOT NULL AND sd.postcode != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.planning_reference IS NOT NULL AND sd.planning_reference != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.application_date IS NOT NULL THEN 0.2 ELSE 0 END +
        CASE WHEN sd.property_type IS NOT NULL AND sd.property_type != '' THEN 0.2 ELSE 0 END
    ) as data_completeness_score,
    
    -- Processing time (estimated based on match complexity)
    CASE 
        WHEN am.document_id IS NOT NULL THEN 
            CASE am.match_method_id
                WHEN 13 THEN 150  -- exact_components
                WHEN 8 THEN 100   -- postcode_house  
                WHEN 9 THEN 200   -- road_city_exact
                WHEN 10 THEN 300  -- fuzzy_road
                ELSE 250
            END
        ELSE 50 -- no match processing
    END as processing_time_ms,
    
    -- Business measures (extract from additional data if available)
    CASE 
        WHEN sd.additional_data IS NOT NULL AND sd.additional_data ? 'application_fee' 
        THEN (sd.additional_data->>'application_fee')::decimal(10,2)
        ELSE NULL
    END as application_fee,
    
    CASE 
        WHEN sd.additional_data IS NOT NULL AND sd.additional_data ? 'estimated_value'
        THEN (sd.additional_data->>'estimated_value')::decimal(12,2)
        ELSE NULL
    END as estimated_value,
    
    CASE 
        WHEN sd.additional_data IS NOT NULL AND sd.additional_data ? 'floor_area_sqm'
        THEN (sd.additional_data->>'floor_area_sqm')::decimal(10,2)
        ELSE NULL
    END as floor_area_sqm,
    
    -- Technical fields
    sd.batch_id as import_batch_id,
    sd.planning_reference,
    
    -- Boolean flags
    CASE 
        WHEN am.confidence_score >= 0.85 THEN TRUE
        ELSE FALSE
    END as is_auto_processed,
    
    CASE 
        WHEN sd.raw_address IS NULL OR sd.raw_address = '' THEN TRUE
        WHEN sd.postcode IS NULL OR sd.postcode = '' THEN TRUE
        WHEN sd.gopostal_processed = FALSE THEN TRUE
        WHEN am.document_id IS NULL AND sd.gopostal_processed = TRUE THEN TRUE
        WHEN am.confidence_score IS NOT NULL AND am.confidence_score < 0.7 THEN TRUE
        ELSE FALSE
    END as has_validation_issues,
    
    -- Additional measures as JSONB
    jsonb_build_object(
        'original_source', COALESCE(sd.source_system, 'unknown'),
        'import_notes', sd.import_notes,
        'validation_status', sd.validation_status,
        'original_record_id', sd.original_record_id,
        'gopostal_processed', sd.gopostal_processed,
        'has_coordinates', CASE WHEN am.location_id IS NOT NULL THEN TRUE ELSE FALSE END
    ) as additional_measures,
    
    -- Audit fields
    NOW() as created_at,
    '1.0' as processing_version

FROM src_document sd

-- Join to get original address dimension ID (REQUIRED)
INNER JOIN dim_original_address oa ON oa.address_hash = MD5(LOWER(TRIM(COALESCE(sd.raw_address, ''))))

-- Optional joins to existing match data
LEFT JOIN address_match am ON sd.document_id = am.document_id

-- Dimension lookups (optional - will use defaults if not found)
LEFT JOIN dim_document_type dt ON dt.document_type_code = UPPER(COALESCE(sd.document_type, 'UNKNOWN'))
LEFT JOIN dim_document_status ds ON ds.status_code = UPPER(COALESCE('ACTIVE', 'UNKNOWN'))
LEFT JOIN dim_property_type pt ON pt.property_code = UPPER(COALESCE(sd.property_type, 'UNKNOWN'))
LEFT JOIN dim_application_status ast ON ast.status_code = UPPER(COALESCE(sd.application_status, 'UNKNOWN'))
LEFT JOIN dim_development_type devt ON devt.development_code = UPPER(COALESCE(sd.development_type, 'UNKNOWN'))

ORDER BY sd.document_id;

-- Update statistics after population
ANALYZE fact_documents_lean;

-- Get population results
SELECT 
    'Fact Table Population Results' as info,
    COUNT(*) as total_records,
    COUNT(CASE WHEN matched_address_id IS NOT NULL THEN 1 END) as with_address_match,
    COUNT(CASE WHEN is_matched = TRUE THEN 1 END) as is_matched_flag,
    COUNT(CASE WHEN is_high_confidence = TRUE THEN 1 END) as high_confidence,
    COUNT(CASE WHEN has_validation_issues = TRUE THEN 1 END) as with_validation_issues,
    ROUND(100.0 * COUNT(CASE WHEN matched_address_id IS NOT NULL THEN 1 END) / COUNT(*), 2) as match_rate_pct,
    ROUND(AVG(match_confidence_score), 4) as avg_confidence,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_data_completeness
FROM fact_documents_lean;

-- Validate referential integrity
SELECT 
    'Referential Integrity Check' as info,
    COUNT(*) as total_records,
    COUNT(CASE WHEN document_type_id IS NULL THEN 1 END) as missing_doc_type,
    COUNT(CASE WHEN document_status_id IS NULL THEN 1 END) as missing_doc_status,
    COUNT(CASE WHEN original_address_id IS NULL THEN 1 END) as missing_orig_address,
    COUNT(CASE WHEN match_method_id IS NULL THEN 1 END) as missing_match_method,
    COUNT(CASE WHEN match_decision_id IS NULL THEN 1 END) as missing_match_decision,
    COUNT(CASE WHEN import_date_id IS NULL THEN 1 END) as missing_import_date
FROM fact_documents_lean;

-- Clean up helper functions
DROP FUNCTION IF EXISTS get_unknown_document_type_id();
DROP FUNCTION IF EXISTS get_unknown_document_status_id();
DROP FUNCTION IF EXISTS get_match_method_id(TEXT);
DROP FUNCTION IF EXISTS get_match_decision_id(TEXT);

COMMIT;