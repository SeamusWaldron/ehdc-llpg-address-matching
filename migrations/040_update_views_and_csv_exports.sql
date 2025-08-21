-- Migration: Update views for new fact table structure and add CSV export views
-- Description: Updates existing views and creates new views to reconstruct original CSV formats

BEGIN;

-- Drop existing views that depend on the old fact table structure
DROP VIEW IF EXISTS vw_high_quality_matches_lean CASCADE;
DROP VIEW IF EXISTS vw_needs_review_lean CASCADE;
DROP VIEW IF EXISTS vw_match_method_performance_lean CASCADE;
DROP VIEW IF EXISTS vw_data_quality_dashboard_lean CASCADE;
DROP VIEW IF EXISTS vw_documents_complete CASCADE;

-- 1. High Quality Matches View (Updated)
CREATE OR REPLACE VIEW vw_high_quality_matches_lean AS
SELECT 
    f.document_id,
    dt.type_name as document_type,
    oa.raw_address as original_address,
    da.full_address as matched_address,
    da.uprn,
    f.match_confidence_score,
    mm.method_name as match_method,
    md.decision_name as match_decision,
    dl.easting,
    dl.northing,
    dl.latitude,
    dl.longitude,
    f.is_auto_processed,
    f.created_at as matched_at
FROM fact_documents_lean f
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
INNER JOIN dim_original_address oa ON f.original_address_id = oa.original_address_id
INNER JOIN dim_address da ON f.matched_address_id = da.address_id
INNER JOIN dim_location dl ON f.matched_location_id = dl.location_id
LEFT JOIN dim_match_method mm ON f.match_method_id = mm.method_id
LEFT JOIN dim_match_decision md ON f.match_decision_id = md.match_decision_id
WHERE f.match_confidence_score >= 0.85
   OR f.is_high_confidence = true;

-- 2. Needs Review View (Updated)
CREATE OR REPLACE VIEW vw_needs_review_lean AS
SELECT 
    f.document_id,
    dt.type_name as document_type,
    oa.raw_address as original_address,
    da.full_address as suggested_match,
    da.uprn as suggested_uprn,
    f.match_confidence_score,
    mm.method_name as match_method,
    md.decision_name as current_decision,
    CASE 
        WHEN f.match_confidence_score < 0.5 THEN 'Low confidence'
        WHEN f.match_confidence_score < 0.7 THEN 'Medium confidence'
        WHEN md.decision_code = 'needs_review' THEN 'Flagged for review'
        ELSE 'Other'
    END as review_reason,
    f.created_at as processed_at
FROM fact_documents_lean f
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
INNER JOIN dim_original_address oa ON f.original_address_id = oa.original_address_id
LEFT JOIN dim_address da ON f.matched_address_id = da.address_id
LEFT JOIN dim_match_method mm ON f.match_method_id = mm.method_id
LEFT JOIN dim_match_decision md ON f.match_decision_id = md.match_decision_id
WHERE (f.match_confidence_score BETWEEN 0.3 AND 0.85 AND f.matched_address_id IS NOT NULL)
   OR md.decision_code = 'needs_review'
   OR f.has_validation_issues = true;

-- 3. Match Method Performance View (Updated)
CREATE OR REPLACE VIEW vw_match_method_performance_lean AS
SELECT 
    mm.method_name,
    mm.method_code,
    COUNT(*) as total_matches,
    ROUND(AVG(f.match_confidence_score), 3) as avg_confidence,
    ROUND(MIN(f.match_confidence_score), 3) as min_confidence,
    ROUND(MAX(f.match_confidence_score), 3) as max_confidence,
    COUNT(*) FILTER (WHERE f.match_confidence_score >= 0.85) as high_confidence_count,
    COUNT(*) FILTER (WHERE f.match_confidence_score < 0.5) as low_confidence_count,
    COUNT(*) FILTER (WHERE f.is_auto_processed = true) as auto_processed_count,
    ROUND(100.0 * COUNT(*) FILTER (WHERE f.is_auto_processed = true) / COUNT(*), 1) as auto_process_rate
FROM fact_documents_lean f
INNER JOIN dim_match_method mm ON f.match_method_id = mm.method_id
WHERE f.matched_address_id IS NOT NULL
GROUP BY mm.method_name, mm.method_code
ORDER BY total_matches DESC;

-- 4. Data Quality Dashboard View (Updated)
CREATE OR REPLACE VIEW vw_data_quality_dashboard_lean AS
SELECT 
    dt.type_name as document_type,
    COUNT(*) as total_documents,
    COUNT(f.matched_address_id) as matched_documents,
    ROUND(100.0 * COUNT(f.matched_address_id) / NULLIF(COUNT(*), 0), 1) as match_rate,
    ROUND(AVG(f.match_confidence_score), 3) as avg_confidence,
    COUNT(*) FILTER (WHERE f.match_confidence_score >= 0.85) as high_confidence_matches,
    COUNT(*) FILTER (WHERE f.match_confidence_score < 0.5 AND f.matched_address_id IS NOT NULL) as low_confidence_matches,
    COUNT(*) FILTER (WHERE f.has_validation_issues = true) as validation_issues,
    COUNT(*) FILTER (WHERE f.is_auto_processed = true) as auto_processed,
    COUNT(*) FILTER (WHERE f.matched_address_id IS NULL) as unmatched
FROM fact_documents_lean f
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
GROUP BY dt.type_name
ORDER BY total_documents DESC;

-- 5. Complete Documents View (Updated)
CREATE OR REPLACE VIEW vw_documents_complete AS
SELECT 
    f.document_id,
    f.fact_id,
    dt.type_name as document_type,
    ds.status_name as document_status,
    oa.raw_address as original_address,
    da.full_address as matched_address,
    da.uprn,
    dl.easting,
    dl.northing,
    dl.latitude,
    dl.longitude,
    f.match_confidence_score,
    mm.method_name as match_method,
    md.decision_name as match_decision,
    f.is_matched,
    f.is_auto_processed,
    f.is_high_confidence,
    f.has_validation_issues,
    f.created_at,
    f.updated_at
FROM fact_documents_lean f
LEFT JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
LEFT JOIN dim_document_status ds ON f.document_status_id = ds.document_status_id
LEFT JOIN dim_original_address oa ON f.original_address_id = oa.original_address_id
LEFT JOIN dim_address da ON f.matched_address_id = da.address_id
LEFT JOIN dim_location dl ON f.matched_location_id = dl.location_id
LEFT JOIN dim_match_method mm ON f.match_method_id = mm.method_id
LEFT JOIN dim_match_decision md ON f.match_decision_id = md.match_decision_id;

-- ============================================================================
-- NEW: CSV RECONSTRUCTION VIEWS
-- ============================================================================

-- 6. CSV Export: Decision Notices
CREATE OR REPLACE VIEW vw_csv_export_decision_notices AS
SELECT 
    s.document_id,
    s.external_reference as planning_app_no,
    s.document_date,
    s.raw_address as address,
    s.raw_uprn as uprn,
    da.full_address as matched_llpg_address,
    da.uprn as matched_llpg_uprn,
    dl.easting,
    dl.northing,
    dl.latitude,
    dl.longitude,
    f.match_confidence_score,
    CASE 
        WHEN f.matched_address_id IS NOT NULL THEN 'Matched'
        ELSE 'Unmatched'
    END as match_status
FROM src_document s
INNER JOIN fact_documents_lean f ON s.document_id = f.document_id
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
LEFT JOIN dim_address da ON f.matched_address_id = da.address_id
LEFT JOIN dim_location dl ON f.matched_location_id = dl.location_id
WHERE dt.type_name = 'Decision Notice'
ORDER BY s.document_date DESC, s.external_reference;

-- 7. CSV Export: Land Charges
CREATE OR REPLACE VIEW vw_csv_export_land_charges AS
SELECT 
    s.document_id,
    s.external_reference as planning_app_no,
    s.document_date,
    s.raw_address as address,
    s.raw_uprn as uprn,
    da.full_address as matched_llpg_address,
    da.uprn as matched_llpg_uprn,
    dl.easting,
    dl.northing,
    dl.latitude,
    dl.longitude,
    f.match_confidence_score,
    CASE 
        WHEN f.matched_address_id IS NOT NULL THEN 'Matched'
        ELSE 'Unmatched'
    END as match_status
FROM src_document s
INNER JOIN fact_documents_lean f ON s.document_id = f.document_id
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
LEFT JOIN dim_address da ON f.matched_address_id = da.address_id
LEFT JOIN dim_location dl ON f.matched_location_id = dl.location_id
WHERE dt.type_name = 'Land Charge'
ORDER BY s.document_date DESC, s.external_reference;

-- 8. CSV Export: Agreements
CREATE OR REPLACE VIEW vw_csv_export_agreements AS
SELECT 
    s.document_id,
    s.external_reference as planning_app_no,
    s.document_date,
    s.raw_address as address,
    s.raw_uprn as uprn,
    da.full_address as matched_llpg_address,
    da.uprn as matched_llpg_uprn,
    dl.easting,
    dl.northing,
    dl.latitude,
    dl.longitude,
    f.match_confidence_score,
    CASE 
        WHEN f.matched_address_id IS NOT NULL THEN 'Matched'
        ELSE 'Unmatched'
    END as match_status
FROM src_document s
INNER JOIN fact_documents_lean f ON s.document_id = f.document_id
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
LEFT JOIN dim_address da ON f.matched_address_id = da.address_id
LEFT JOIN dim_location dl ON f.matched_location_id = dl.location_id
WHERE dt.type_name = 'Agreement'
ORDER BY s.document_date DESC, s.external_reference;

-- 9. CSV Export: Enforcement Notices
CREATE OR REPLACE VIEW vw_csv_export_enforcement_notices AS
SELECT 
    s.document_id,
    s.external_reference as planning_app_no,
    s.document_date,
    s.raw_address as address,
    s.raw_uprn as uprn,
    da.full_address as matched_llpg_address,
    da.uprn as matched_llpg_uprn,
    dl.easting,
    dl.northing,
    dl.latitude,
    dl.longitude,
    f.match_confidence_score,
    CASE 
        WHEN f.matched_address_id IS NOT NULL THEN 'Matched'
        ELSE 'Unmatched'
    END as match_status
FROM src_document s
INNER JOIN fact_documents_lean f ON s.document_id = f.document_id
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
LEFT JOIN dim_address da ON f.matched_address_id = da.address_id
LEFT JOIN dim_location dl ON f.matched_location_id = dl.location_id
WHERE dt.type_name = 'Enforcement Notice'
ORDER BY s.document_date DESC, s.external_reference;

-- 10. CSV Export: Street Name and Numbering
CREATE OR REPLACE VIEW vw_csv_export_street_naming AS
SELECT 
    s.document_id,
    s.external_reference as planning_app_no,
    s.document_date,
    s.raw_address as address,
    s.raw_uprn as uprn,
    da.full_address as matched_llpg_address,
    da.uprn as matched_llpg_uprn,
    dl.easting,
    dl.northing,
    dl.latitude,
    dl.longitude,
    f.match_confidence_score,
    CASE 
        WHEN f.matched_address_id IS NOT NULL THEN 'Matched'
        ELSE 'Unmatched'
    END as match_status
FROM src_document s
INNER JOIN fact_documents_lean f ON s.document_id = f.document_id
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
LEFT JOIN dim_address da ON f.matched_address_id = da.address_id
LEFT JOIN dim_location dl ON f.matched_location_id = dl.location_id
WHERE dt.type_name = 'Street Name and Numbering'
ORDER BY s.document_date DESC, s.external_reference;

-- 11. CSV Export: Combined All Documents
CREATE OR REPLACE VIEW vw_csv_export_all_documents AS
SELECT 
    s.document_id,
    dt.type_name as document_type,
    s.external_reference as planning_app_no,
    s.document_date,
    s.raw_address as original_address,
    s.raw_uprn as original_uprn,
    da.full_address as matched_llpg_address,
    da.uprn as matched_llpg_uprn,
    dl.easting,
    dl.northing,
    dl.latitude,
    dl.longitude,
    f.match_confidence_score,
    mm.method_name as match_method,
    CASE 
        WHEN f.matched_address_id IS NOT NULL THEN 'Matched'
        ELSE 'Unmatched'
    END as match_status,
    CASE 
        WHEN f.match_confidence_score >= 0.85 THEN 'High'
        WHEN f.match_confidence_score >= 0.5 THEN 'Medium'
        WHEN f.match_confidence_score > 0 THEN 'Low'
        ELSE NULL
    END as confidence_level
FROM src_document s
INNER JOIN fact_documents_lean f ON s.document_id = f.document_id
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
LEFT JOIN dim_address da ON f.matched_address_id = da.address_id
LEFT JOIN dim_location dl ON f.matched_location_id = dl.location_id
LEFT JOIN dim_match_method mm ON f.match_method_id = mm.method_id
ORDER BY dt.type_name, s.document_date DESC, s.external_reference;

-- 12. CSV Export: Unmatched Documents Only
CREATE OR REPLACE VIEW vw_csv_export_unmatched AS
SELECT 
    s.document_id,
    dt.type_name as document_type,
    s.external_reference as planning_app_no,
    s.document_date,
    s.raw_address as address,
    s.raw_uprn as uprn,
    s.gopostal_house_number,
    s.gopostal_road,
    s.gopostal_city,
    s.gopostal_postcode,
    'Requires manual matching' as action_required
FROM src_document s
INNER JOIN fact_documents_lean f ON s.document_id = f.document_id
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
WHERE f.matched_address_id IS NULL
  AND s.raw_address IS NOT NULL
  AND s.raw_address <> 'N/A'
  AND LENGTH(s.raw_address) > 5
ORDER BY dt.type_name, s.document_date DESC;

-- 13. CSV Export: High Confidence Matches for Validation
CREATE OR REPLACE VIEW vw_csv_export_high_confidence AS
SELECT 
    s.document_id,
    dt.type_name as document_type,
    s.external_reference as planning_app_no,
    s.raw_address as original_address,
    da.full_address as matched_address,
    da.uprn as matched_uprn,
    f.match_confidence_score,
    mm.method_name as match_method,
    dl.easting,
    dl.northing
FROM src_document s
INNER JOIN fact_documents_lean f ON s.document_id = f.document_id
INNER JOIN dim_document_type dt ON f.doc_type_id = dt.doc_type_id
INNER JOIN dim_address da ON f.matched_address_id = da.address_id
INNER JOIN dim_location dl ON f.matched_location_id = dl.location_id
LEFT JOIN dim_match_method mm ON f.match_method_id = mm.method_id
WHERE f.match_confidence_score >= 0.85
  OR f.is_high_confidence = true
ORDER BY f.match_confidence_score DESC;

-- Create indexes for better view performance
CREATE INDEX IF NOT EXISTS idx_fact_doc_type_match ON fact_documents_lean(doc_type_id, matched_address_id);
CREATE INDEX IF NOT EXISTS idx_src_doc_external_ref ON src_document(external_reference);
CREATE INDEX IF NOT EXISTS idx_src_doc_date ON src_document(document_date);

-- Add comments to document the views
COMMENT ON VIEW vw_csv_export_decision_notices IS 'Reconstructs Decision Notices CSV format with match results';
COMMENT ON VIEW vw_csv_export_land_charges IS 'Reconstructs Land Charges CSV format with match results';
COMMENT ON VIEW vw_csv_export_agreements IS 'Reconstructs Agreements CSV format with match results';
COMMENT ON VIEW vw_csv_export_enforcement_notices IS 'Reconstructs Enforcement Notices CSV format with match results';
COMMENT ON VIEW vw_csv_export_street_naming IS 'Reconstructs Street Name and Numbering CSV format with match results';
COMMENT ON VIEW vw_csv_export_all_documents IS 'Combined export of all document types with match results';
COMMENT ON VIEW vw_csv_export_unmatched IS 'Export of unmatched documents requiring manual intervention';
COMMENT ON VIEW vw_csv_export_high_confidence IS 'High confidence matches for quality validation';

COMMIT;