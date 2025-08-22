-- Test conservative matching logic on planning reference 20026 records
WITH test_documents AS (
  SELECT 
    f.document_id,
    f.planning_reference,
    o.raw_address,
    -- Parse house number (simple extraction for testing)
    CASE 
      WHEN o.raw_address ~ '^[0-9]+[A-Z]*[,\s]' THEN regexp_replace(o.raw_address, '^([0-9]+[A-Z]*)[,\s].*', '\1')
      WHEN o.raw_address ~ 'UNIT\s*[0-9]+[A-Z]*[,\s]' THEN regexp_replace(o.raw_address, '.*UNIT\s*([0-9]+[A-Z]*)[,\s].*', '\1', 'i')
      ELSE ''
    END as house_number,
    -- Parse street name  
    CASE
      WHEN o.raw_address ILIKE '%AMEY%' THEN 'AMEY INDUSTRIAL ESTATE'
      WHEN o.raw_address ILIKE '%FRENCHMANS%ROAD%' THEN 'FRENCHMANS ROAD'
      WHEN o.raw_address ILIKE '%BEDFORD%ROAD%' THEN 'BEDFORD ROAD'
      ELSE ''
    END as street_name
  FROM fact_documents_lean f
  JOIN dim_original_address o ON f.original_address_id = o.original_address_id
  WHERE f.planning_reference LIKE '20026%'
)
SELECT 
  td.document_id,
  td.planning_reference,
  td.raw_address,
  td.house_number,
  td.street_name,
  -- Look for potential matches in expanded table
  COUNT(e.expanded_id) as potential_matches,
  STRING_AGG(e.uprn || ': ' || e.full_address, '; ' ORDER BY e.full_address) as candidate_addresses
FROM test_documents td
LEFT JOIN dim_address_expanded e ON (
  (td.house_number != '' AND UPPER(e.full_address) LIKE '%' || td.house_number || '%')
  AND 
  (td.street_name != '' AND UPPER(e.full_address) LIKE '%' || td.street_name || '%')
)
GROUP BY td.document_id, td.planning_reference, td.raw_address, td.house_number, td.street_name
ORDER BY td.planning_reference;