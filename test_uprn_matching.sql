-- Test Script: Analyze UPRN Matching Priority
-- Purpose: Verify if source documents with UPRNs are directly matched
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- 1. Check overall UPRN presence and matching
SELECT 
    'UPRN MATCHING ANALYSIS' as section;

SELECT 
    'UPRN Statistics' as category,
    COUNT(*) as total_docs,
    COUNT(raw_uprn) as docs_with_uprn,
    ROUND(100.0 * COUNT(raw_uprn) / COUNT(*), 1) || '%' as uprn_coverage,
    COUNT(CASE WHEN raw_uprn IS NOT NULL AND am.address_id IS NOT NULL THEN 1 END) as uprn_docs_matched,
    ROUND(100.0 * COUNT(CASE WHEN raw_uprn IS NOT NULL AND am.address_id IS NOT NULL THEN 1 END) / 
          NULLIF(COUNT(raw_uprn), 0), 1) || '%' as uprn_match_rate
FROM src_document sd
LEFT JOIN address_match am ON sd.document_id = am.document_id;

-- 2. Check if UPRNs are matching correctly
SELECT 
    'UPRN Match Quality' as section;

SELECT 
    'UPRN Match Accuracy' as category,
    COUNT(*) as total_uprn_docs_matched,
    COUNT(CASE WHEN sd.raw_uprn = da.uprn THEN 1 END) as exact_uprn_matches,
    COUNT(CASE WHEN sd.raw_uprn != da.uprn THEN 1 END) as wrong_uprn_matches,
    ROUND(100.0 * COUNT(CASE WHEN sd.raw_uprn = da.uprn THEN 1 END) / 
          NULLIF(COUNT(*), 0), 1) || '%' as uprn_accuracy
FROM src_document sd
JOIN address_match am ON sd.document_id = am.document_id
JOIN dim_address da ON am.address_id = da.address_id
WHERE sd.raw_uprn IS NOT NULL;

-- 3. Check match methods used for documents with UPRNs
SELECT 
    'Match Methods for UPRN Documents' as section;

SELECT 
    mm.method_code,
    mm.method_name,
    COUNT(*) as matches,
    ROUND(100.0 * COUNT(*) / SUM(COUNT(*)) OVER (), 1) || '%' as percentage
FROM src_document sd
JOIN address_match am ON sd.document_id = am.document_id
JOIN dim_match_method mm ON am.match_method_id = mm.method_id
WHERE sd.raw_uprn IS NOT NULL
GROUP BY mm.method_id, mm.method_code, mm.method_name
ORDER BY COUNT(*) DESC;

-- 4. Check if exact_uprn method is being used
SELECT 
    'Exact UPRN Method Usage' as section;

SELECT 
    'Method ID 1 (exact_uprn)' as check_name,
    COUNT(*) as usage_count,
    CASE 
        WHEN COUNT(*) = 0 THEN '❌ NOT BEING USED - This is the problem!'
        ELSE '✅ Being used correctly'
    END as status
FROM address_match 
WHERE match_method_id = 1;

-- 5. Sample documents with UPRNs that should have exact matches
SELECT 
    'Sample UPRN Mismatches' as section;

SELECT 
    sd.document_id,
    LEFT(sd.raw_address, 40) as source_address,
    sd.raw_uprn as source_uprn,
    da.uprn as matched_uprn,
    CASE 
        WHEN sd.raw_uprn = da.uprn THEN '✅ CORRECT'
        ELSE '❌ WRONG'
    END as match_status,
    mm.method_code
FROM src_document sd
JOIN address_match am ON sd.document_id = am.document_id
JOIN dim_address da ON am.address_id = da.address_id
JOIN dim_match_method mm ON am.match_method_id = mm.method_id
WHERE sd.raw_uprn IS NOT NULL
  AND sd.raw_uprn != da.uprn
LIMIT 10;

-- 6. Check if UPRN exists in LLPG
SELECT 
    'UPRN Coverage in LLPG' as section;

WITH source_uprns AS (
    SELECT DISTINCT raw_uprn 
    FROM src_document 
    WHERE raw_uprn IS NOT NULL
),
matched_uprns AS (
    SELECT DISTINCT su.raw_uprn,
           EXISTS(SELECT 1 FROM dim_address WHERE uprn = su.raw_uprn) as exists_in_llpg
    FROM source_uprns su
)
SELECT 
    'UPRN Coverage' as metric,
    COUNT(*) as total_unique_uprns,
    COUNT(CASE WHEN exists_in_llpg THEN 1 END) as found_in_llpg,
    COUNT(CASE WHEN NOT exists_in_llpg THEN 1 END) as not_in_llpg,
    ROUND(100.0 * COUNT(CASE WHEN exists_in_llpg THEN 1 END) / COUNT(*), 1) || '%' as coverage_rate
FROM matched_uprns;

ROLLBACK;