-- Script to rebuild fact table with corrections applied
-- This will fix the coordinate inconsistency issue

BEGIN;

-- Truncate and rebuild the fact table to include corrections
TRUNCATE TABLE fact_documents_lean;

-- Helper function to get the effective match (corrected or original)
CREATE OR REPLACE FUNCTION get_effective_match(doc_id INTEGER)
RETURNS TABLE(
    address_id INTEGER,
    location_id INTEGER,
    confidence_score NUMERIC,
    match_method_id INTEGER
) AS $$
BEGIN
    RETURN QUERY
    -- First try corrected match
    SELECT 
        amc.corrected_address_id,
        amc.corrected_location_id,
        amc.corrected_confidence_score,
        amc.corrected_method_id
    FROM address_match_corrected amc
    WHERE amc.document_id = doc_id
    
    UNION ALL
    
    -- If no correction exists, use original match
    SELECT 
        am.address_id,
        am.location_id,
        am.confidence_score,
        am.match_method_id
    FROM address_match am
    WHERE am.document_id = doc_id
      AND NOT EXISTS (
          SELECT 1 FROM address_match_corrected amc2 
          WHERE amc2.document_id = doc_id
      )
    
    LIMIT 1;
END;
$$ LANGUAGE plpgsql;

-- Repopulate fact table with corrections applied
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
    1 as document_status_id,
    
    -- Original address (required)
    oa.original_address_id,
    
    -- Use effective match (corrected if available, otherwise original)
    em.address_id as matched_address_id,
    em.location_id as matched_location_id,
    
    -- Match method
    COALESCE(em.match_method_id, 
        (SELECT method_id FROM dim_match_method WHERE method_code = 'no_match' LIMIT 1),
        1
    ) as match_method_id,
    
    -- Match decision based on confidence
    CASE 
        WHEN em.confidence_score >= 0.85 THEN 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'AUTO_ACCEPT' LIMIT 1)
        WHEN em.confidence_score >= 0.50 THEN 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'NEEDS_REVIEW' LIMIT 1)
        WHEN em.confidence_score >= 0.20 THEN 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'LOW_CONFIDENCE' LIMIT 1)
        ELSE 
            (SELECT match_decision_id FROM dim_match_decision WHERE decision_code = 'NO_MATCH' LIMIT 1)
    END as match_decision_id,
    
    -- Dimensions (optional)
    NULL as property_type_id,
    NULL as application_status_id,
    NULL as development_type_id,
    
    -- Date dimensions
    CASE 
        WHEN sd.document_date IS NOT NULL THEN TO_CHAR(sd.document_date, 'YYYYMMDD')::INTEGER
        ELSE NULL
    END as application_date_id,
    NULL as decision_date_id,
    TO_CHAR(CURRENT_DATE, 'YYYYMMDD')::INTEGER as import_date_id,
    
    -- Measures
    em.confidence_score as match_confidence_score,
    
    -- Address quality score
    CASE 
        WHEN em.confidence_score IS NOT NULL THEN em.confidence_score
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
    
    -- Processing time
    CASE 
        WHEN em.address_id IS NOT NULL THEN 
            CASE 
                WHEN em.confidence_score >= 0.95 THEN 100
                WHEN em.confidence_score >= 0.85 THEN 150
                WHEN em.confidence_score >= 0.70 THEN 200
                ELSE 300
            END
        ELSE 50
    END as processing_time_ms,
    
    -- Technical fields
    1 as import_batch_id,
    sd.external_reference as planning_reference,
    
    -- Boolean flags
    CASE 
        WHEN em.confidence_score >= 0.85 THEN TRUE
        ELSE FALSE
    END as is_auto_processed,
    
    CASE 
        WHEN sd.raw_address IS NULL OR sd.raw_address = '' THEN TRUE
        WHEN sd.gopostal_processed = FALSE THEN TRUE
        WHEN em.address_id IS NULL AND sd.gopostal_processed = TRUE THEN TRUE
        WHEN em.confidence_score IS NOT NULL AND em.confidence_score < 0.7 THEN TRUE
        ELSE FALSE
    END as has_validation_issues,
    
    -- Additional measures
    jsonb_build_object(
        'original_source', 'src_document',
        'has_uprn', CASE WHEN sd.raw_uprn IS NOT NULL THEN TRUE ELSE FALSE END,
        'has_coordinates', CASE WHEN em.location_id IS NOT NULL THEN TRUE ELSE FALSE END,
        'gopostal_processed', sd.gopostal_processed,
        'job_number', sd.job_number,
        'filepath', sd.filepath,
        'has_correction', CASE WHEN EXISTS(SELECT 1 FROM address_match_corrected WHERE document_id = sd.document_id) THEN TRUE ELSE FALSE END
    ) as additional_measures,
    
    -- Audit fields
    NOW() as created_at,
    '1.1' as processing_version -- Updated version to indicate corrections included

FROM src_document sd

-- Join to get original address dimension ID
INNER JOIN dim_original_address oa ON oa.address_hash = MD5(LOWER(TRIM(sd.raw_address)))

-- Get effective match (corrected or original)
LEFT JOIN LATERAL get_effective_match(sd.document_id) em ON TRUE

-- Only include records with valid addresses
WHERE sd.raw_address IS NOT NULL 
  AND sd.raw_address != ''

ORDER BY sd.document_id;

-- Drop the temporary function
DROP FUNCTION get_effective_match(INTEGER);

-- Update statistics
ANALYZE fact_documents_lean;

-- Show results
SELECT 
    'Fact Table Rebuild Results' as info,
    COUNT(*) as total_records,
    COUNT(CASE WHEN matched_address_id IS NOT NULL THEN 1 END) as with_address_match,
    COUNT(CASE WHEN additional_measures->>'has_correction' = 'true' THEN 1 END) as with_corrections,
    ROUND(100.0 * COUNT(CASE WHEN matched_address_id IS NOT NULL THEN 1 END) / COUNT(*), 2) as match_rate_pct
FROM fact_documents_lean;

COMMIT;