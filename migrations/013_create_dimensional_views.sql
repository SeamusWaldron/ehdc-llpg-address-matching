-- Migration 013: Create Business Views for Dimensional Model
-- Purpose: Create user-friendly views that join dimensions back to fact table
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- 1. Main business view - joins all dimensions for complete data access
CREATE OR REPLACE VIEW vw_documents_complete AS
SELECT 
    -- Fact table keys
    f.fact_id,
    f.document_id,
    
    -- Document information
    dt.document_type_name,
    dt.document_category,
    ds.status_name as document_status,
    ds.status_category as document_status_category,
    
    -- Original address information
    oa.raw_address,
    oa.std_postcode,
    oa.std_road,
    oa.std_city,
    oa.std_house_number,
    oa.std_house_name,
    oa.usage_count as address_usage_count,
    
    -- Matched address information (if available)
    CASE WHEN f.matched_address_id IS NOT NULL THEN da.full_address END as matched_address,
    CASE WHEN f.matched_address_id IS NOT NULL THEN da.uprn END as matched_uprn,
    CASE WHEN f.matched_location_id IS NOT NULL THEN dl.easting END as matched_easting,
    CASE WHEN f.matched_location_id IS NOT NULL THEN dl.northing END as matched_northing,
    CASE WHEN f.matched_location_id IS NOT NULL THEN ST_Y(dl.geom) END as matched_latitude,
    CASE WHEN f.matched_location_id IS NOT NULL THEN ST_X(dl.geom) END as matched_longitude,
    
    -- Match information
    mm.method_name as match_method,
    mm.method_category as match_method_category,
    md.decision_name as match_decision,
    md.auto_process as can_auto_process,
    md.requires_review,
    
    -- Property information (if available)
    pt.property_name as property_type,
    pt.property_category,
    pt.is_residential,
    pt.is_commercial,
    
    -- Application information (if available)
    ast.status_name as application_status,
    ast.status_category as application_status_category,
    ast.is_final_status as application_is_final,
    
    -- Development information (if available)
    devt.development_name as development_type,
    devt.development_category,
    devt.impact_level,
    devt.requires_eia,
    
    -- Date information
    dd_app.date_actual as application_date,
    dd_app.financial_year as application_financial_year,
    dd_dec.date_actual as decision_date,
    dd_imp.date_actual as import_date,
    
    -- Measures
    f.match_confidence_score,
    f.address_quality_score,
    f.data_completeness_score,
    f.processing_time_ms,
    f.application_fee,
    f.estimated_value,
    f.floor_area_sqm,
    
    -- Computed flags
    f.is_matched,
    f.is_auto_processed,
    f.has_validation_issues,
    f.is_high_confidence,
    
    -- Technical fields
    f.import_batch_id,
    f.planning_reference,
    f.additional_measures,
    
    -- Audit
    f.created_at,
    f.processing_version
    
FROM fact_documents_lean f

-- Required dimension joins
INNER JOIN dim_document_type dt ON f.document_type_id = dt.document_type_id
INNER JOIN dim_document_status ds ON f.document_status_id = ds.document_status_id
INNER JOIN dim_original_address oa ON f.original_address_id = oa.original_address_id
INNER JOIN dim_match_method mm ON f.match_method_id = mm.match_method_id
INNER JOIN dim_match_decision md ON f.match_decision_id = md.match_decision_id

-- Optional dimension joins
LEFT JOIN dim_property_type pt ON f.property_type_id = pt.property_type_id
LEFT JOIN dim_application_status ast ON f.application_status_id = ast.application_status_id
LEFT JOIN dim_development_type devt ON f.development_type_id = devt.development_type_id

-- Date dimension joins
LEFT JOIN dim_date dd_app ON f.application_date_id = dd_app.date_id
LEFT JOIN dim_date dd_dec ON f.decision_date_id = dd_dec.date_id
LEFT JOIN dim_date dd_imp ON f.import_date_id = dd_imp.date_id

-- Existing LLPG dimensions (for matched addresses)
LEFT JOIN dim_address da ON f.matched_address_id = da.address_id
LEFT JOIN dim_location dl ON f.matched_location_id = dl.location_id;

COMMENT ON VIEW vw_documents_complete IS 'Complete business view joining all dimensions - use for detailed analysis';

-- 2. High-quality matches ready for production use
CREATE OR REPLACE VIEW vw_high_quality_matches_lean AS
SELECT 
    document_id,
    matched_uprn,
    raw_address,
    matched_address,
    match_confidence_score,
    match_method,
    planning_reference,
    application_date,
    property_type,
    development_type,
    matched_easting,
    matched_northing,
    matched_latitude,
    matched_longitude,
    address_quality_score,
    data_completeness_score
FROM vw_documents_complete
WHERE is_matched = TRUE
  AND is_high_confidence = TRUE
  AND can_auto_process = TRUE
  AND matched_uprn IS NOT NULL;

COMMENT ON VIEW vw_high_quality_matches_lean IS 'High-confidence address matches ready for automated processing';

-- 3. Records needing manual review
CREATE OR REPLACE VIEW vw_needs_review_lean AS
SELECT 
    document_id,
    raw_address,
    matched_address,
    match_confidence_score,
    match_method,
    match_decision,
    requires_review,
    planning_reference,
    property_type,
    application_date,
    address_quality_score,
    data_completeness_score,
    std_postcode,
    std_road,
    std_city,
    has_validation_issues
FROM vw_documents_complete
WHERE requires_review = TRUE
   OR (is_matched = TRUE AND match_confidence_score BETWEEN 0.70 AND 0.94)
ORDER BY match_confidence_score DESC, data_completeness_score DESC;

COMMENT ON VIEW vw_needs_review_lean IS 'Medium-confidence matches requiring manual verification';

-- 4. Unmatched addresses for investigation
CREATE OR REPLACE VIEW vw_unmatched_addresses_lean AS
SELECT 
    document_id,
    raw_address,
    std_postcode,
    std_road,
    std_city,
    std_house_number,
    planning_reference,
    property_type,
    application_date,
    data_completeness_score,
    address_quality_score,
    has_validation_issues,
    additional_measures
FROM vw_documents_complete
WHERE is_matched = FALSE
   OR match_decision = 'No Match'
ORDER BY data_completeness_score DESC, address_quality_score DESC;

COMMENT ON VIEW vw_unmatched_addresses_lean IS 'Addresses that could not be matched - candidates for manual investigation';

-- 5. Data quality dashboard for the dimensional model
CREATE OR REPLACE VIEW vw_data_quality_dashboard_lean AS
SELECT 
    'Overall Statistics' as metric_category,
    COUNT(*) as total_records,
    COUNT(CASE WHEN is_matched = TRUE THEN 1 END) as matched,
    COUNT(CASE WHEN is_high_confidence = TRUE THEN 1 END) as high_confidence,
    COUNT(CASE WHEN can_auto_process = TRUE THEN 1 END) as auto_processable,
    COUNT(CASE WHEN requires_review = TRUE THEN 1 END) as needs_review,
    COUNT(CASE WHEN is_matched = FALSE THEN 1 END) as no_match,
    ROUND(100.0 * COUNT(CASE WHEN is_matched = TRUE THEN 1 END) / COUNT(*), 2) as match_rate_pct,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_data_completeness,
    ROUND(AVG(match_confidence_score), 3) as avg_match_confidence
FROM vw_documents_complete

UNION ALL

SELECT 
    'High Quality (>= 0.85)' as metric_category,
    COUNT(*) as total_records,
    COUNT(CASE WHEN is_matched = TRUE THEN 1 END) as matched,
    COUNT(CASE WHEN is_high_confidence = TRUE THEN 1 END) as high_confidence,
    COUNT(CASE WHEN can_auto_process = TRUE THEN 1 END) as auto_processable,
    COUNT(CASE WHEN requires_review = TRUE THEN 1 END) as needs_review,
    COUNT(CASE WHEN is_matched = FALSE THEN 1 END) as no_match,
    ROUND(100.0 * COUNT(CASE WHEN is_matched = TRUE THEN 1 END) / COUNT(*), 2) as match_rate_pct,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_data_completeness,
    ROUND(AVG(match_confidence_score), 3) as avg_match_confidence
FROM vw_documents_complete
WHERE match_confidence_score >= 0.85

UNION ALL

SELECT 
    'Medium Quality (0.50-0.84)' as metric_category,
    COUNT(*) as total_records,
    COUNT(CASE WHEN is_matched = TRUE THEN 1 END) as matched,
    COUNT(CASE WHEN is_high_confidence = TRUE THEN 1 END) as high_confidence,
    COUNT(CASE WHEN can_auto_process = TRUE THEN 1 END) as auto_processable,
    COUNT(CASE WHEN requires_review = TRUE THEN 1 END) as needs_review,
    COUNT(CASE WHEN is_matched = FALSE THEN 1 END) as no_match,
    ROUND(100.0 * COUNT(CASE WHEN is_matched = TRUE THEN 1 END) / COUNT(*), 2) as match_rate_pct,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_data_completeness,
    ROUND(AVG(match_confidence_score), 3) as avg_match_confidence
FROM vw_documents_complete
WHERE match_confidence_score BETWEEN 0.50 AND 0.84;

COMMENT ON VIEW vw_data_quality_dashboard_lean IS 'Comprehensive data quality metrics for the dimensional model';

-- 6. Geographic summary by area (using dimensional model)
CREATE OR REPLACE VIEW vw_geographic_summary_lean AS
SELECT 
    COALESCE(std_city, 'Unknown Area') as area,
    COUNT(*) as total_documents,
    COUNT(CASE WHEN is_matched = TRUE THEN 1 END) as matched_documents,
    COUNT(CASE WHEN can_auto_process = TRUE THEN 1 END) as auto_processable,
    COUNT(CASE WHEN requires_review = TRUE THEN 1 END) as needs_review,
    COUNT(CASE WHEN is_matched = FALSE THEN 1 END) as no_match,
    ROUND(100.0 * COUNT(CASE WHEN is_matched = TRUE THEN 1 END) / COUNT(*), 2) as match_rate_pct,
    ROUND(AVG(match_confidence_score), 3) as avg_confidence,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_data_completeness
FROM vw_documents_complete
WHERE std_city IS NOT NULL
GROUP BY std_city
HAVING COUNT(*) >= 3
ORDER BY match_rate_pct DESC, total_documents DESC;

COMMENT ON VIEW vw_geographic_summary_lean IS 'Address matching performance summary by geographic area';

-- 7. Business intelligence summary (using dimensional model)
CREATE OR REPLACE VIEW vw_business_intelligence_lean AS
SELECT 
    property_type,
    property_category,
    COUNT(*) as total_applications,
    COUNT(CASE WHEN is_matched = TRUE THEN 1 END) as with_address_match,
    COUNT(CASE WHEN application_date >= CURRENT_DATE - INTERVAL '1 year' THEN 1 END) as recent_applications,
    COUNT(CASE WHEN decision_date IS NOT NULL THEN 1 END) as with_decisions,
    ROUND(100.0 * COUNT(CASE WHEN is_matched = TRUE THEN 1 END) / COUNT(*), 2) as address_match_rate,
    ROUND(AVG(match_confidence_score), 3) as avg_match_confidence,
    ROUND(AVG(COALESCE(application_fee, 0)), 2) as avg_application_fee,
    MIN(application_date) as earliest_application,
    MAX(application_date) as latest_application
FROM vw_documents_complete
WHERE property_type IS NOT NULL
GROUP BY property_type, property_category
HAVING COUNT(*) >= 5
ORDER BY total_applications DESC;

COMMENT ON VIEW vw_business_intelligence_lean IS 'Business metrics by property type using dimensional model';

-- 8. Match method performance analysis
CREATE OR REPLACE VIEW vw_match_method_performance_lean AS
SELECT 
    mm.method_name,
    mm.method_category,
    COUNT(*) as total_matches,
    ROUND(AVG(f.match_confidence_score), 4) as avg_confidence,
    ROUND(MIN(f.match_confidence_score), 4) as min_confidence,
    ROUND(MAX(f.match_confidence_score), 4) as max_confidence,
    COUNT(CASE WHEN md.auto_process = TRUE THEN 1 END) as auto_processable,
    COUNT(CASE WHEN md.requires_review = TRUE THEN 1 END) as needs_review,
    ROUND(100.0 * COUNT(CASE WHEN md.auto_process = TRUE THEN 1 END) / COUNT(*), 2) as auto_process_rate,
    ROUND(AVG(f.processing_time_ms), 1) as avg_processing_time_ms
FROM fact_documents_lean f
INNER JOIN dim_match_method mm ON f.match_method_id = mm.match_method_id
INNER JOIN dim_match_decision md ON f.match_decision_id = md.match_decision_id
WHERE f.is_matched = TRUE
GROUP BY mm.method_name, mm.method_category
ORDER BY total_matches DESC;

COMMENT ON VIEW vw_match_method_performance_lean IS 'Performance analysis of different address matching methods';

-- 9. Dimension usage statistics
CREATE OR REPLACE VIEW vw_dimension_usage_stats AS
SELECT 
    'Document Types' as dimension_name,
    dt.document_type_name as dimension_value,
    COUNT(f.fact_id) as usage_count,
    ROUND(100.0 * COUNT(f.fact_id) / (SELECT COUNT(*) FROM fact_documents_lean), 2) as usage_percentage
FROM dim_document_type dt
LEFT JOIN fact_documents_lean f ON dt.document_type_id = f.document_type_id
GROUP BY dt.document_type_id, dt.document_type_name

UNION ALL

SELECT 
    'Property Types' as dimension_name,
    COALESCE(pt.property_name, 'Not Specified') as dimension_value,
    COUNT(f.fact_id) as usage_count,
    ROUND(100.0 * COUNT(f.fact_id) / (SELECT COUNT(*) FROM fact_documents_lean), 2) as usage_percentage
FROM dim_property_type pt
RIGHT JOIN fact_documents_lean f ON pt.property_type_id = f.property_type_id
GROUP BY pt.property_type_id, pt.property_name

UNION ALL

SELECT 
    'Match Methods' as dimension_name,
    mm.method_name as dimension_value,
    COUNT(f.fact_id) as usage_count,
    ROUND(100.0 * COUNT(f.fact_id) / (SELECT COUNT(*) FROM fact_documents_lean), 2) as usage_percentage
FROM dim_match_method mm
LEFT JOIN fact_documents_lean f ON mm.match_method_id = f.match_method_id
GROUP BY mm.match_method_id, mm.method_name

ORDER BY dimension_name, usage_count DESC;

COMMENT ON VIEW vw_dimension_usage_stats IS 'Usage statistics for all dimension tables';

-- Grant permissions (uncomment and adjust for your environment)
-- GRANT SELECT ON ALL TABLES IN SCHEMA public TO ehdc_read_role;
-- GRANT SELECT ON ALL TABLES IN SCHEMA public TO ehdc_analyst_role;

COMMIT;