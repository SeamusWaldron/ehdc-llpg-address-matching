-- Check planning reference 20026 addresses and their current match status
SELECT 
    planning_app_base,
    address_1,
    address_canonical,
    match_method_id,
    match_confidence_score,
    matched_address_id,
    source_type
FROM fact_documents_lean 
WHERE planning_app_base = '20026' 
ORDER BY address_1;