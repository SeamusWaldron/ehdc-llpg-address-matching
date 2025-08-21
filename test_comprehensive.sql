-- Test comprehensive matching on a small sample
-- First, let's see what we have before
SELECT 
    'BEFORE' as phase,
    COUNT(*) as total_docs,
    COUNT(CASE WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) >= 0.8 THEN 1 END) as high_confidence,
    COUNT(CASE WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) BETWEEN 0.5 AND 0.8 THEN 1 END) as medium_confidence,
    COUNT(CASE WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score) BETWEEN 0.2 AND 0.5 THEN 1 END) as low_confidence,
    COUNT(CASE WHEN COALESCE(amc.corrected_confidence_score, am.confidence_score, 0) < 0.2 THEN 1 END) as no_match,
    ROUND(AVG(COALESCE(amc.corrected_confidence_score, am.confidence_score, 0)), 3) as avg_confidence
FROM src_document s
LEFT JOIN address_match am ON s.document_id = am.document_id
LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
WHERE s.planning_app_base IN ('20026', '20049', '20021', '20216', '20200')  -- Test groups we know

;

-- Show specific test cases separately
SELECT 
    s.document_id,
    s.raw_address,
    COALESCE(amc.corrected_confidence_score, am.confidence_score, 0) as confidence,
    CASE WHEN amc.document_id IS NOT NULL THEN 'CORRECTED' ELSE 'ORIGINAL' END as match_type,
    COALESCE(da2.full_address, da.full_address, 'NO MATCH') as matched_address
FROM src_document s
LEFT JOIN address_match am ON s.document_id = am.document_id
LEFT JOIN dim_address da ON am.address_id = da.address_id
LEFT JOIN address_match_corrected amc ON s.document_id = amc.document_id
LEFT JOIN dim_address da2 ON amc.corrected_address_id = da2.address_id
WHERE s.planning_app_base IN ('20026', '20049', '20021', '20216', '20200')
  AND s.document_id IN (117, 247, 174, 1644, 1522)  -- Specific test documents
ORDER BY s.document_id;