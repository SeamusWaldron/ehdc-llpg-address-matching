-- Test if conservative matching would find Unit 2 Amey
SELECT 
    'Direct match test' as test,
    original_address_id, uprn, full_address 
FROM dim_address_expanded 
WHERE UPPER(full_address) LIKE '%UNIT%2%AMEY%' 
    AND expansion_type = 'range_expansion'
LIMIT 5;