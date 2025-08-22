-- Test both original and expanded Unit 2 Amey addresses
SELECT 
    'All Unit 2 Amey' as test,
    expansion_type,
    original_address_id, uprn, full_address 
FROM dim_address_expanded 
WHERE UPPER(full_address) LIKE '%UNIT%2%AMEY%'
ORDER BY expansion_type, full_address;