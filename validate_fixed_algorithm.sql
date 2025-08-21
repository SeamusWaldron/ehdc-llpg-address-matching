-- Validation Script: Compare Old vs Fixed Algorithm Performance
-- Purpose: Comprehensive analysis of the fixed algorithm's improvements
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- 1. Overall Performance Comparison
SELECT 
    '=== ALGORITHM PERFORMANCE COMPARISON ===' as section;

SELECT 
    'Performance Metrics' as category,
    'Total Documents' as metric,
    COUNT(*) as value,
    '100.0%' as percentage
FROM src_document WHERE raw_address IS NOT NULL
UNION ALL
SELECT 
    'Performance Metrics',
    'Old Algorithm Matches',
    COUNT(*),
    ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM src_document WHERE raw_address IS NOT NULL), 1) || '%'
FROM address_match 
WHERE matched_by != 'system_fixed_component' AND address_id IS NOT NULL
UNION ALL
SELECT 
    'Performance Metrics',
    'Fixed Algorithm Matches',
    COUNT(*),
    ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM src_document WHERE raw_address IS NOT NULL), 1) || '%'
FROM address_match 
WHERE matched_by = 'system_fixed_component' AND address_id IS NOT NULL
UNION ALL
SELECT 
    'Performance Metrics',
    'Fixed Algorithm Processed',
    COUNT(*),
    ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM src_document WHERE raw_address IS NOT NULL), 1) || '%'
FROM address_match 
WHERE matched_by = 'system_fixed_component';

-- 2. Quality Metrics - House Number Validation
SELECT 
    '=== HOUSE NUMBER VALIDATION RESULTS ===' as section;

-- Old algorithm house number mismatches
SELECT 
    'House Number Validation' as category,
    'Old Algorithm Wrong House Numbers' as issue,
    COUNT(*) as violations,
    'HIGH PRIORITY' as severity
FROM address_match am
JOIN src_document sd ON am.document_id = sd.document_id
JOIN dim_address da ON am.address_id = da.address_id
WHERE am.matched_by != 'system_fixed_component'
  AND sd.gopostal_house_number IS NOT NULL
  AND da.gopostal_house_number IS NOT NULL
  AND sd.gopostal_house_number != da.gopostal_house_number
  AND am.confidence_score >= 0.8
UNION ALL
-- Fixed algorithm house number mismatches (should be 0)
SELECT 
    'House Number Validation',
    'Fixed Algorithm Wrong House Numbers',
    COUNT(*),
    CASE WHEN COUNT(*) = 0 THEN 'EXCELLENT ✅' ELSE 'NEEDS ATTENTION ⚠️' END
FROM address_match am
JOIN src_document sd ON am.document_id = sd.document_id
JOIN dim_address da ON am.address_id = da.address_id
WHERE am.matched_by = 'system_fixed_component'
  AND sd.gopostal_house_number IS NOT NULL
  AND da.gopostal_house_number IS NOT NULL
  AND sd.gopostal_house_number != da.gopostal_house_number
  AND am.confidence_score >= 0.8;

-- 3. Business Address Handling
SELECT 
    '=== BUSINESS ADDRESS VALIDATION ===' as section;

-- Check HORNDEAN FOOTBALL CLUB matches specifically
SELECT 
    'Business Address Quality' as category,
    'HORNDEAN FOOTBALL CLUB - Old Algorithm' as test_case,
    COUNT(DISTINCT am.address_id) as unique_matches,
    CASE 
        WHEN COUNT(DISTINCT am.address_id) > 1 THEN 'INCONSISTENT ❌'
        WHEN COUNT(DISTINCT am.address_id) = 1 AND MAX(da.full_address) ILIKE '%football club%' THEN 'CORRECT ✅'
        WHEN COUNT(DISTINCT am.address_id) = 1 THEN 'WRONG MATCH ❌'
        ELSE 'NO MATCHES'
    END as quality_assessment
FROM address_match am
JOIN src_document sd ON am.document_id = sd.document_id
JOIN dim_address da ON am.address_id = da.address_id
WHERE am.matched_by != 'system_fixed_component'
  AND sd.raw_address ILIKE '%HORNDEAN FOOTBALL CLUB%'
  AND am.address_id IS NOT NULL
UNION ALL
SELECT 
    'Business Address Quality',
    'HORNDEAN FOOTBALL CLUB - Fixed Algorithm',
    COUNT(DISTINCT am.address_id),
    CASE 
        WHEN COUNT(DISTINCT am.address_id) > 1 THEN 'INCONSISTENT ❌'
        WHEN COUNT(DISTINCT am.address_id) = 1 AND MAX(da.full_address) ILIKE '%football club%' THEN 'CORRECT ✅'
        WHEN COUNT(DISTINCT am.address_id) = 1 THEN 'WRONG MATCH ❌'
        ELSE 'NO MATCHES'
    END
FROM address_match am
JOIN src_document sd ON am.document_id = sd.document_id
JOIN dim_address da ON am.address_id = da.address_id
WHERE am.matched_by = 'system_fixed_component'
  AND sd.raw_address ILIKE '%HORNDEAN FOOTBALL CLUB%'
  AND am.address_id IS NOT NULL;

-- 4. Confidence Score Distribution
SELECT 
    '=== CONFIDENCE SCORE ANALYSIS ===' as section;

SELECT 
    'Confidence Distribution' as category,
    CASE 
        WHEN am.matched_by = 'system_fixed_component' THEN 'Fixed Algorithm'
        ELSE 'Old Algorithm'
    END as algorithm,
    CASE 
        WHEN confidence_score >= 0.95 THEN 'Very High (≥0.95)'
        WHEN confidence_score >= 0.85 THEN 'High (0.85-0.94)'
        WHEN confidence_score >= 0.70 THEN 'Medium (0.70-0.84)'
        WHEN confidence_score >= 0.50 THEN 'Low (0.50-0.69)'
        ELSE 'Very Low (<0.50)'
    END as confidence_range,
    COUNT(*) as match_count,
    ROUND(100.0 * COUNT(*) / SUM(COUNT(*)) OVER (PARTITION BY 
        CASE WHEN am.matched_by = 'system_fixed_component' THEN 'Fixed' ELSE 'Old' END), 1) || '%' as percentage
FROM address_match am
WHERE address_id IS NOT NULL
GROUP BY 
    CASE WHEN am.matched_by = 'system_fixed_component' THEN 'Fixed Algorithm' ELSE 'Old Algorithm' END,
    CASE 
        WHEN confidence_score >= 0.95 THEN 'Very High (≥0.95)'
        WHEN confidence_score >= 0.85 THEN 'High (0.85-0.94)'
        WHEN confidence_score >= 0.70 THEN 'Medium (0.70-0.84)'
        WHEN confidence_score >= 0.50 THEN 'Low (0.50-0.69)'
        ELSE 'Very Low (<0.50)'
    END
ORDER BY algorithm, 
    CASE 
        WHEN confidence_score >= 0.95 THEN 1
        WHEN confidence_score >= 0.85 THEN 2
        WHEN confidence_score >= 0.70 THEN 3
        WHEN confidence_score >= 0.50 THEN 4
        ELSE 5
    END;

-- 5. Processing Progress
SELECT 
    '=== PROCESSING PROGRESS ===' as section;

SELECT 
    'Processing Status' as category,
    'Total Documents to Process' as status,
    COUNT(*) as count
FROM src_document WHERE raw_address IS NOT NULL
UNION ALL
SELECT 
    'Processing Status',
    'Fixed Algorithm Processed',
    COUNT(*)
FROM address_match WHERE matched_by = 'system_fixed_component'
UNION ALL
SELECT 
    'Processing Status',
    'Remaining Documents',
    COUNT(*)
FROM src_document sd
WHERE sd.raw_address IS NOT NULL
  AND sd.document_id NOT IN (
      SELECT document_id FROM address_match 
      WHERE matched_by = 'system_fixed_component'
  );

-- 6. Sample Improvements
SELECT 
    '=== SAMPLE BEFORE/AFTER COMPARISON ===' as section;

-- Show some specific improvements for documents that had problematic matches
SELECT 
    'Improvement Examples' as category,
    sd.document_id,
    LEFT(sd.raw_address, 50) as source_address,
    CASE 
        WHEN old_match.address_id IS NOT NULL THEN LEFT(old_addr.full_address, 40)
        ELSE 'NO MATCH'
    END as old_match,
    CASE 
        WHEN new_match.address_id IS NOT NULL THEN LEFT(new_addr.full_address, 40)
        ELSE 'NO MATCH'
    END as fixed_match,
    CASE 
        WHEN old_match.address_id IS NULL AND new_match.address_id IS NOT NULL THEN 'NEW MATCH ✅'
        WHEN old_match.address_id IS NOT NULL AND new_match.address_id IS NULL THEN 'REMOVED BAD MATCH ✅'
        WHEN old_match.address_id != new_match.address_id THEN 'IMPROVED MATCH ✅'
        WHEN old_match.address_id = new_match.address_id THEN 'SAME MATCH'
        ELSE 'DIFFERENT'
    END as improvement_type
FROM src_document sd
LEFT JOIN address_match old_match ON sd.document_id = old_match.document_id 
    AND old_match.matched_by != 'system_fixed_component'
LEFT JOIN dim_address old_addr ON old_match.address_id = old_addr.address_id
LEFT JOIN address_match new_match ON sd.document_id = new_match.document_id 
    AND new_match.matched_by = 'system_fixed_component'
LEFT JOIN dim_address new_addr ON new_match.address_id = new_addr.address_id
WHERE sd.raw_address ILIKE '%HORNDEAN FOOTBALL CLUB%'
  AND (old_match.document_id IS NOT NULL OR new_match.document_id IS NOT NULL)
ORDER BY sd.document_id
LIMIT 10;

COMMIT;