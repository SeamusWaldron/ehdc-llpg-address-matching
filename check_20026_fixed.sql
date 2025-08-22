-- Check planning reference 20026 addresses and their current match status
SELECT 
    f.planning_reference,
    o.raw_address,
    o.address_canonical,
    f.match_method_id,
    f.match_confidence_score,
    f.matched_address_id,
    d.type_name as source_type
FROM fact_documents_lean f
JOIN dim_original_address o ON f.original_address_id = o.original_address_id
LEFT JOIN dim_document_type d ON f.doc_type_id = d.doc_type_id
WHERE f.planning_reference = '20026' 
ORDER BY o.raw_address;