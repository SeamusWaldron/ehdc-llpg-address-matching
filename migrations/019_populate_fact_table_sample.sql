-- Migration 019: Populate Fact Table with Sample Data
-- Purpose: Populate fact_documents_lean with sample data to demonstrate the dimensional model
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

-- Populate fact table with documents that have addresses in our dimension table
INSERT INTO fact_documents_lean (
    document_id,
    doc_type_id,
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
    
    -- Document type (required - use existing or default)
    COALESCE(sd.doc_type_id, 1) as doc_type_id,
    
    -- Document status (default to 'ACTIVE')
    1 as document_status_id, -- Assumes ACTIVE is ID 1
    
    -- Original address (required)
    oa.original_address_id,
    
    -- Matched address and location (may be null)
    am.address_id as matched_address_id,
    am.location_id as matched_location_id,
    
    -- Match method (use existing or default to 'NO_MATCH')
    COALESCE(am.match_method_id, 
        (SELECT method_id FROM dim_match_method WHERE method_code = 'no_match' LIMIT 1),
        1
    ) as match_method_id,
    
    -- Match decision based on confidence
    CASE 
        WHEN am.confidence_score >= 0.85 THEN 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'AUTO_ACCEPT' LIMIT 1)
        WHEN am.confidence_score >= 0.50 THEN 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'NEEDS_REVIEW' LIMIT 1)
        WHEN am.confidence_score >= 0.20 THEN 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'LOW_CONFIDENCE' LIMIT 1)
        ELSE 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'NO_MATCH' LIMIT 1)
    END as match_decision_id,
    
    -- Property type (optional)
    NULL as property_type_id, -- Can be populated later
    
    -- Application status (optional)
    NULL as application_status_id, -- Can be populated later
    
    -- Development type (optional)
    NULL as development_type_id, -- Can be populated later
    
    -- Date dimensions
    date_to_date_id(sd.document_date) as application_date_id,
    NULL as decision_date_id, -- No decision date in source
    date_to_date_id(CURRENT_DATE) as import_date_id,
    
    -- Measures (numerical facts)
    am.confidence_score as match_confidence_score,
    
    -- Address quality score
    CASE 
        WHEN am.confidence_score IS NOT NULL THEN am.confidence_score
        WHEN sd.gopostal_processed = TRUE THEN 0.3
        ELSE 0.1
    END as address_quality_score,
    
    -- Data completeness score
    (
        CASE WHEN sd.raw_address IS NOT NULL AND sd.raw_address != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_postcode IS NOT NULL AND sd.gopostal_postcode != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.external_reference IS NOT NULL AND sd.external_reference != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.document_date IS NOT NULL THEN 0.2 ELSE 0 END +
        CASE WHEN sd.job_number IS NOT NULL AND sd.job_number != '' THEN 0.2 ELSE 0 END
    ) as data_completeness_score,
    
    -- Processing time (estimated)
    CASE 
        WHEN am.document_id IS NOT NULL THEN 
            CASE 
                WHEN am.confidence_score >= 0.95 THEN 100  -- Very fast exact match
                WHEN am.confidence_score >= 0.85 THEN 150  -- Fast high confidence
                WHEN am.confidence_score >= 0.70 THEN 200  -- Medium confidence
                ELSE 300  -- Low confidence, more processing
            END
        ELSE 50 -- No match processing
    END as processing_time_ms,
    
    -- Technical fields
    1 as import_batch_id, -- Default batch
    sd.external_reference as planning_reference,
    
    -- Boolean flags
    CASE 
        WHEN am.confidence_score >= 0.85 THEN TRUE
        ELSE FALSE
    END as is_auto_processed,
    
    CASE 
        WHEN sd.raw_address IS NULL OR sd.raw_address = '' THEN TRUE
        WHEN sd.gopostal_processed = FALSE THEN TRUE
        WHEN am.document_id IS NULL AND sd.gopostal_processed = TRUE THEN TRUE
        WHEN am.confidence_score IS NOT NULL AND am.confidence_score < 0.7 THEN TRUE
        ELSE FALSE
    END as has_validation_issues,
    
    -- Additional measures as JSONB
    jsonb_build_object(
        'original_source', 'src_document',
        'has_uprn', CASE WHEN sd.raw_uprn IS NOT NULL THEN TRUE ELSE FALSE END,
        'has_coordinates', CASE WHEN am.location_id IS NOT NULL THEN TRUE ELSE FALSE END,
        'gopostal_processed', sd.gopostal_processed,
        'job_number', sd.job_number
    ) as additional_measures,
    
    -- Audit fields
    NOW() as created_at,
    '1.0' as processing_version

FROM src_document sd

-- Join to get original address dimension ID (REQUIRED)
INNER JOIN dim_original_address oa ON oa.address_hash = MD5(LOWER(TRIM(sd.raw_address)))

-- Optional join to existing match data
LEFT JOIN address_match am ON sd.document_id = am.document_id

-- Limit to addresses we have in our dimension table (sample data)
WHERE sd.raw_address IS NOT NULL 
  AND EXISTS (SELECT 1 FROM dim_original_address oa2 WHERE oa2.address_hash = MD5(LOWER(TRIM(sd.raw_address))))

ORDER BY sd.document_id
LIMIT 1000; -- Sample of 1000 records

-- Update statistics
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
    COUNT(CASE WHEN doc_type_id IS NULL THEN 1 END) as missing_doc_type,
    COUNT(CASE WHEN document_status_id IS NULL THEN 1 END) as missing_doc_status,
    COUNT(CASE WHEN original_address_id IS NULL THEN 1 END) as missing_orig_address,
    COUNT(CASE WHEN match_method_id IS NULL THEN 1 END) as missing_match_method,
    COUNT(CASE WHEN match_decision_id IS NULL THEN 1 END) as missing_match_decision,
    COUNT(CASE WHEN import_date_id IS NULL THEN 1 END) as missing_import_date
FROM fact_documents_lean;

COMMIT;