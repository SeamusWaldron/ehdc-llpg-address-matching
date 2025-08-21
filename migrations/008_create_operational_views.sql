-- Migration 008: Create Operational Views for Fact Documents
-- Purpose: Create business-friendly views for common query patterns
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- View 1: High-quality matched records (ready for production use)
CREATE OR REPLACE VIEW vw_high_quality_matches AS
SELECT 
    document_id,
    matched_uprn,
    raw_address,
    matched_full_address,
    match_confidence,
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
FROM fact_documents
WHERE match_status = 'matched'
  AND match_decision = 'auto_accept'
  AND match_confidence >= 0.85
  AND matched_uprn IS NOT NULL;

COMMENT ON VIEW vw_high_quality_matches IS 'High-confidence address matches ready for automated processing';

-- View 2: Records needing manual review
CREATE OR REPLACE VIEW vw_needs_review AS
SELECT 
    document_id,
    raw_address,
    matched_full_address,
    match_confidence,
    match_method,
    match_decision,
    validation_flags,
    planning_reference,
    property_type,
    application_date,
    address_quality_score,
    data_completeness_score,
    std_postcode,
    std_road,
    std_city
FROM fact_documents
WHERE match_decision = 'needs_review'
   OR (match_status = 'matched' AND match_confidence BETWEEN 0.70 AND 0.94)
ORDER BY match_confidence DESC, data_completeness_score DESC;

COMMENT ON VIEW vw_needs_review IS 'Medium-confidence matches requiring manual verification';

-- View 3: Unmatched addresses for investigation
CREATE OR REPLACE VIEW vw_unmatched_addresses AS
SELECT 
    document_id,
    raw_address,
    std_postcode,
    std_road,
    std_city,
    std_house_number,
    validation_flags,
    planning_reference,
    property_type,
    application_date,
    data_completeness_score,
    address_quality_score,
    additional_data
FROM fact_documents
WHERE match_status = 'no_match'
   OR match_decision = 'no_match'
ORDER BY data_completeness_score DESC, address_quality_score DESC;

COMMENT ON VIEW vw_unmatched_addresses IS 'Addresses that could not be matched - candidates for manual investigation';

-- View 4: Geographic summary by area
CREATE OR REPLACE VIEW vw_geographic_summary AS
SELECT 
    COALESCE(std_city, 'Unknown Area') as area,
    COUNT(*) as total_documents,
    COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) as matched_documents,
    COUNT(CASE WHEN match_decision = 'auto_accept' THEN 1 END) as auto_accepted,
    COUNT(CASE WHEN match_decision = 'needs_review' THEN 1 END) as needs_review,
    COUNT(CASE WHEN match_status = 'no_match' THEN 1 END) as no_match,
    ROUND(100.0 * COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) / COUNT(*), 2) as match_rate_pct,
    ROUND(AVG(match_confidence), 3) as avg_confidence,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_data_completeness
FROM fact_documents
WHERE std_city IS NOT NULL
GROUP BY std_city
HAVING COUNT(*) >= 3
ORDER BY match_rate_pct DESC, total_documents DESC;

COMMENT ON VIEW vw_geographic_summary IS 'Address matching performance summary by geographic area';

-- View 5: Data quality dashboard
CREATE OR REPLACE VIEW vw_data_quality_dashboard AS
SELECT 
    'Overall Statistics' as metric_category,
    COUNT(*) as total_records,
    COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) as with_uprn,
    COUNT(CASE WHEN match_status = 'matched' THEN 1 END) as matched,
    COUNT(CASE WHEN match_decision = 'auto_accept' THEN 1 END) as auto_accepted,
    COUNT(CASE WHEN match_decision = 'needs_review' THEN 1 END) as needs_review,
    COUNT(CASE WHEN match_status = 'no_match' THEN 1 END) as no_match,
    ROUND(100.0 * COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) / COUNT(*), 2) as uprn_coverage_pct,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_data_completeness,
    ROUND(AVG(match_confidence), 3) as avg_match_confidence
FROM fact_documents

UNION ALL

SELECT 
    'High Quality (>= 0.85)' as metric_category,
    COUNT(*) as total_records,
    COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) as with_uprn,
    COUNT(CASE WHEN match_status = 'matched' THEN 1 END) as matched,
    COUNT(CASE WHEN match_decision = 'auto_accept' THEN 1 END) as auto_accepted,
    COUNT(CASE WHEN match_decision = 'needs_review' THEN 1 END) as needs_review,
    COUNT(CASE WHEN match_status = 'no_match' THEN 1 END) as no_match,
    ROUND(100.0 * COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) / COUNT(*), 2) as uprn_coverage_pct,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_data_completeness,
    ROUND(AVG(match_confidence), 3) as avg_match_confidence
FROM fact_documents
WHERE match_confidence >= 0.85

UNION ALL

SELECT 
    'Medium Quality (0.70-0.84)' as metric_category,
    COUNT(*) as total_records,
    COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) as with_uprn,
    COUNT(CASE WHEN match_status = 'matched' THEN 1 END) as matched,
    COUNT(CASE WHEN match_decision = 'auto_accept' THEN 1 END) as auto_accepted,
    COUNT(CASE WHEN match_decision = 'needs_review' THEN 1 END) as needs_review,
    COUNT(CASE WHEN match_status = 'no_match' THEN 1 END) as no_match,
    ROUND(100.0 * COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) / COUNT(*), 2) as uprn_coverage_pct,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_data_completeness,
    ROUND(AVG(match_confidence), 3) as avg_match_confidence
FROM fact_documents
WHERE match_confidence BETWEEN 0.70 AND 0.84;

COMMENT ON VIEW vw_data_quality_dashboard IS 'Comprehensive data quality metrics for monitoring and reporting';

-- View 6: Business intelligence summary
CREATE OR REPLACE VIEW vw_business_intelligence AS
SELECT 
    property_type,
    COUNT(*) as total_applications,
    COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) as with_address_match,
    COUNT(CASE WHEN application_date >= CURRENT_DATE - INTERVAL '1 year' THEN 1 END) as recent_applications,
    COUNT(CASE WHEN decision_date IS NOT NULL THEN 1 END) as with_decisions,
    ROUND(100.0 * COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) / COUNT(*), 2) as address_match_rate,
    ROUND(AVG(match_confidence), 3) as avg_match_confidence,
    MIN(application_date) as earliest_application,
    MAX(application_date) as latest_application
FROM fact_documents
WHERE property_type IS NOT NULL
GROUP BY property_type
HAVING COUNT(*) >= 5
ORDER BY total_applications DESC;

COMMENT ON VIEW vw_business_intelligence IS 'Business metrics by property type for strategic analysis';

-- View 7: Spatial analysis view (for GIS applications)
CREATE OR REPLACE VIEW vw_spatial_analysis AS
SELECT 
    document_id,
    matched_uprn,
    planning_reference,
    property_type,
    application_date,
    match_confidence,
    matched_easting,
    matched_northing,
    matched_latitude,
    matched_longitude,
    ST_Point(matched_longitude, matched_latitude) as geom_wgs84,
    ST_Transform(ST_Point(matched_easting, matched_northing), 27700) as geom_bng
FROM fact_documents
WHERE matched_latitude IS NOT NULL 
  AND matched_longitude IS NOT NULL
  AND matched_easting IS NOT NULL 
  AND matched_northing IS NOT NULL
  AND matched_uprn IS NOT NULL;

COMMENT ON VIEW vw_spatial_analysis IS 'Spatially-enabled view for GIS analysis and mapping applications';

-- View 8: Audit and processing summary
CREATE OR REPLACE VIEW vw_processing_audit AS
SELECT 
    processing_version,
    processed_by,
    DATE(created_at) as processing_date,
    COUNT(*) as records_processed,
    COUNT(CASE WHEN match_status = 'matched' THEN 1 END) as matched_count,
    COUNT(CASE WHEN validation_flags IS NOT NULL AND array_length(validation_flags, 1) > 0 THEN 1 END) as with_validation_issues,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(data_completeness_score), 3) as avg_data_completeness,
    MIN(created_at) as first_processed,
    MAX(created_at) as last_processed
FROM fact_documents
GROUP BY processing_version, processed_by, DATE(created_at)
ORDER BY processing_date DESC, processed_by;

COMMENT ON VIEW vw_processing_audit IS 'Audit trail showing processing batches and quality metrics';

-- View 9: Validation issues summary
CREATE OR REPLACE VIEW vw_validation_issues AS
SELECT 
    unnest(validation_flags) as validation_issue,
    COUNT(*) as occurrence_count,
    ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM fact_documents WHERE validation_flags IS NOT NULL), 2) as percentage_of_flagged,
    COUNT(CASE WHEN match_status = 'matched' THEN 1 END) as matched_despite_issue,
    COUNT(CASE WHEN match_status = 'no_match' THEN 1 END) as no_match_with_issue
FROM fact_documents
WHERE validation_flags IS NOT NULL 
  AND array_length(validation_flags, 1) > 0
GROUP BY unnest(validation_flags)
ORDER BY occurrence_count DESC;

COMMENT ON VIEW vw_validation_issues IS 'Summary of data validation issues and their impact on matching';

-- View 10: Match method performance
CREATE OR REPLACE VIEW vw_match_method_performance AS
SELECT 
    match_method,
    COUNT(*) as total_matches,
    ROUND(AVG(match_confidence), 4) as avg_confidence,
    ROUND(MIN(match_confidence), 4) as min_confidence,
    ROUND(MAX(match_confidence), 4) as max_confidence,
    COUNT(CASE WHEN match_decision = 'auto_accept' THEN 1 END) as auto_accepted,
    COUNT(CASE WHEN match_decision = 'needs_review' THEN 1 END) as needs_review,
    ROUND(100.0 * COUNT(CASE WHEN match_decision = 'auto_accept' THEN 1 END) / COUNT(*), 2) as auto_accept_rate
FROM fact_documents
WHERE match_status = 'matched'
  AND match_method IS NOT NULL
GROUP BY match_method
ORDER BY total_matches DESC;

COMMENT ON VIEW vw_match_method_performance IS 'Performance analysis of different address matching methods';

-- Grant permissions to common roles (adjust as needed for your environment)
-- GRANT SELECT ON ALL TABLES IN SCHEMA public TO ehdc_read_role;
-- GRANT SELECT ON ALL TABLES IN SCHEMA public TO ehdc_analyst_role;

COMMIT;