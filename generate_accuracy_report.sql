-- EHDC LLPG Component-Based Matching Accuracy Report
-- Generated: Current timestamp
-- Purpose: Comprehensive analysis of matching results and accuracy improvement

\echo '============================================================'
\echo 'EHDC LLPG REAL GOPOSTAL ACCURACY REPORT'
\echo '============================================================'
\echo ''

-- SECTION 1: PREPROCESSING COMPLETION STATUS
\echo '1. PREPROCESSING COMPLETION STATUS'
\echo '=================================='

SELECT 
  'LLPG Addresses' as dataset,
  COUNT(*) as total_records,
  SUM(CASE WHEN gopostal_processed = TRUE THEN 1 ELSE 0 END) as processed_records,
  ROUND(100.0 * SUM(CASE WHEN gopostal_processed = TRUE THEN 1 ELSE 0 END) / COUNT(*), 2) as completion_percentage,
  SUM(CASE WHEN gopostal_road IS NOT NULL OR gopostal_postcode IS NOT NULL THEN 1 ELSE 0 END) as addresses_with_components
FROM dim_address

UNION ALL

SELECT 
  'Source Documents' as dataset,
  COUNT(*) as total_records,
  SUM(CASE WHEN gopostal_processed = TRUE THEN 1 ELSE 0 END) as processed_records,
  ROUND(100.0 * SUM(CASE WHEN gopostal_processed = TRUE THEN 1 ELSE 0 END) / COUNT(*), 2) as completion_percentage,
  SUM(CASE WHEN gopostal_road IS NOT NULL OR gopostal_postcode IS NOT NULL THEN 1 ELSE 0 END) as addresses_with_components
FROM src_document 
WHERE raw_address IS NOT NULL AND raw_address != '';

\echo ''
\echo '2. COMPONENT EXTRACTION QUALITY'
\echo '==============================='

-- Component extraction breakdown
SELECT 
  'LLPG' as dataset,
  COUNT(*) as total_processed,
  SUM(CASE WHEN gopostal_house_number IS NOT NULL THEN 1 ELSE 0 END) as has_house_number,
  SUM(CASE WHEN gopostal_road IS NOT NULL THEN 1 ELSE 0 END) as has_road,
  SUM(CASE WHEN gopostal_city IS NOT NULL THEN 1 ELSE 0 END) as has_city,
  SUM(CASE WHEN gopostal_postcode IS NOT NULL THEN 1 ELSE 0 END) as has_postcode,
  SUM(CASE WHEN gopostal_unit IS NOT NULL THEN 1 ELSE 0 END) as has_unit
FROM dim_address 
WHERE gopostal_processed = TRUE

UNION ALL

SELECT 
  'Source' as dataset,
  COUNT(*) as total_processed,
  SUM(CASE WHEN gopostal_house_number IS NOT NULL THEN 1 ELSE 0 END) as has_house_number,
  SUM(CASE WHEN gopostal_road IS NOT NULL THEN 1 ELSE 0 END) as has_road,
  SUM(CASE WHEN gopostal_city IS NOT NULL THEN 1 ELSE 0 END) as has_city,
  SUM(CASE WHEN gopostal_postcode IS NOT NULL THEN 1 ELSE 0 END) as has_postcode,
  SUM(CASE WHEN gopostal_unit IS NOT NULL THEN 1 ELSE 0 END) as has_unit
FROM src_document 
WHERE gopostal_processed = TRUE;

\echo ''
\echo '3. MATCHING RESULTS SUMMARY'
\echo '=========================='

-- Overall matching performance
SELECT 
  decision,
  match_status,
  COUNT(*) as match_count,
  ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM address_match), 2) as percentage,
  ROUND(AVG(confidence_score), 4) as avg_confidence_score,
  ROUND(MIN(confidence_score), 4) as min_confidence_score,
  ROUND(MAX(confidence_score), 4) as max_confidence_score
FROM address_match
GROUP BY decision, match_status
ORDER BY match_count DESC;

\echo ''
\echo '4. MATCHING METHOD BREAKDOWN'
\echo '============================'

-- Performance by matching method
SELECT 
  mm.method_name,
  mm.method_code,
  COUNT(am.*) as matches_found,
  ROUND(100.0 * COUNT(am.*) / (SELECT COUNT(*) FROM address_match), 2) as percentage_of_total,
  ROUND(AVG(am.confidence_score), 4) as avg_confidence,
  COUNT(CASE WHEN am.decision = 'auto_accept' THEN 1 END) as auto_accepted,
  COUNT(CASE WHEN am.decision = 'needs_review' THEN 1 END) as needs_review
FROM match_method mm
LEFT JOIN address_match am ON mm.method_id = am.match_method_id
WHERE mm.method_code IN ('exact_components', 'postcode_house', 'road_city_exact', 'road_city_fuzzy', 'fuzzy_road')
GROUP BY mm.method_id, mm.method_name, mm.method_code
ORDER BY matches_found DESC;

\echo ''
\echo '5. CONFIDENCE SCORE DISTRIBUTION'
\echo '==============================='

-- Distribution of confidence scores
SELECT 
  CASE 
    WHEN confidence_score >= 0.95 THEN '0.95-1.00 (Excellent)'
    WHEN confidence_score >= 0.85 THEN '0.85-0.94 (High)'
    WHEN confidence_score >= 0.70 THEN '0.70-0.84 (Medium)'
    WHEN confidence_score >= 0.50 THEN '0.50-0.69 (Low)'
    ELSE '0.00-0.49 (Very Low)'
  END as confidence_range,
  COUNT(*) as match_count,
  ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM address_match), 2) as percentage,
  COUNT(CASE WHEN decision = 'auto_accept' THEN 1 END) as auto_accepted
FROM address_match
GROUP BY 
  CASE 
    WHEN confidence_score >= 0.95 THEN '0.95-1.00 (Excellent)'
    WHEN confidence_score >= 0.85 THEN '0.85-0.94 (High)'
    WHEN confidence_score >= 0.70 THEN '0.70-0.84 (Medium)'
    WHEN confidence_score >= 0.50 THEN '0.50-0.69 (Low)'
    ELSE '0.00-0.49 (Very Low)'
  END
ORDER BY MIN(confidence_score) DESC;

\echo ''
\echo '6. GEOGRAPHIC DISTRIBUTION'
\echo '========================='

-- Matching success by geographic area
SELECT 
  COALESCE(sd.gopostal_city, 'Unknown') as city,
  COUNT(sd.*) as total_documents,
  COUNT(am.*) as matched_documents,
  ROUND(100.0 * COUNT(am.*) / COUNT(sd.*), 2) as match_rate_percentage,
  ROUND(AVG(am.confidence_score), 4) as avg_confidence
FROM src_document sd
LEFT JOIN address_match am ON sd.document_id = am.document_id
WHERE sd.gopostal_processed = TRUE 
  AND sd.raw_address IS NOT NULL
GROUP BY sd.gopostal_city
HAVING COUNT(sd.*) >= 5  -- Only show areas with 5+ documents
ORDER BY match_rate_percentage DESC, total_documents DESC
LIMIT 20;

\echo ''
\echo '7. PROCESSING PERFORMANCE METRICS'
\echo '================================'

-- Processing performance statistics
SELECT 
  table_name,
  SUM(processed_records) as total_processed,
  ROUND(AVG(processed_records::NUMERIC / EXTRACT(EPOCH FROM processing_time)), 2) as avg_records_per_second,
  SUM(processing_time) as total_processing_time,
  COUNT(*) as batch_count
FROM gopostal_processing_stats
WHERE notes LIKE '%Real gopostal%'
GROUP BY table_name;

\echo ''
\echo '8. SAMPLE HIGH-QUALITY MATCHES'
\echo '============================='

-- Examples of excellent matches
SELECT 
  sd.raw_address as source_address,
  da.full_address as llpg_match,
  da.uprn,
  am.confidence_score,
  mm.method_code as match_method
FROM src_document sd
JOIN address_match am ON sd.document_id = am.document_id
JOIN dim_address da ON am.address_id = da.address_id  
JOIN match_method mm ON am.match_method_id = mm.method_id
WHERE am.confidence_score >= 0.95
  AND am.decision = 'auto_accept'
ORDER BY am.confidence_score DESC, sd.document_id
LIMIT 10;

\echo ''
\echo '9. ADDRESSES REQUIRING MANUAL REVIEW'
\echo '==================================='

-- High-value addresses that need manual review
SELECT 
  sd.raw_address as source_address,
  da.full_address as potential_match,
  am.confidence_score,
  mm.method_code as match_method,
  'Confidence: ' || ROUND(am.confidence_score, 3) || 
  ' | Method: ' || mm.method_code as review_notes
FROM src_document sd
JOIN address_match am ON sd.document_id = am.document_id
JOIN dim_address da ON am.address_id = da.address_id
JOIN match_method mm ON am.match_method_id = mm.method_id
WHERE am.decision = 'needs_review'
  AND am.confidence_score >= 0.70
ORDER BY am.confidence_score DESC
LIMIT 15;

\echo ''
\echo '10. OVERALL ACCURACY IMPROVEMENT'
\echo '==============================='

-- Final accuracy metrics
WITH baseline_estimates AS (
  SELECT 
    (SELECT COUNT(*) FROM src_document WHERE raw_address IS NOT NULL) as total_source_documents,
    -- Estimate baseline match rate at 10%
    ROUND((SELECT COUNT(*) FROM src_document WHERE raw_address IS NOT NULL) * 0.10) as estimated_baseline_matches
),
current_results AS (
  SELECT 
    COUNT(*) as total_matches,
    COUNT(CASE WHEN decision = 'auto_accept' THEN 1 END) as high_confidence_matches,
    COUNT(CASE WHEN confidence_score >= 0.70 THEN 1 END) as good_quality_matches
  FROM address_match
)
SELECT 
  be.total_source_documents,
  be.estimated_baseline_matches as estimated_baseline_matches_10pct,
  cr.total_matches as actual_matches_found,
  cr.high_confidence_matches,
  cr.good_quality_matches,
  ROUND(100.0 * cr.total_matches / be.total_source_documents, 2) as actual_match_rate_pct,
  ROUND(100.0 * cr.high_confidence_matches / be.total_source_documents, 2) as high_confidence_rate_pct,
  ROUND(((cr.total_matches::NUMERIC / be.total_source_documents) / 0.10 - 1) * 100, 1) as improvement_percentage,
  CASE 
    WHEN cr.total_matches > be.estimated_baseline_matches THEN '✅ SUCCESS'
    ELSE '❌ BELOW TARGET'
  END as accuracy_target_status
FROM baseline_estimates be, current_results cr;

\echo ''
\echo '============================================================'
\echo 'REPORT COMPLETE'
\echo '============================================================'
\echo ''
\echo 'KEY RECOMMENDATIONS:'
\echo '• Auto-accept all matches with confidence ≥ 0.95'
\echo '• Manual review required for matches 0.70-0.94'  
\echo '• Investigate no-match cases for data quality issues'
\echo '• Consider fuzzy matching improvements for remaining cases'
\echo ''