-- Check if expanded addresses exist
SELECT 'Expanded addresses count' as description, COUNT(*) as count FROM dim_address_expanded WHERE expansion_type = 'range_expansion';

-- Check for Unit 2 Amey examples
SELECT 'Unit 2 Amey matches' as description, COUNT(*) as count FROM dim_address_expanded WHERE UPPER(full_address) LIKE '%UNIT%2%AMEY%';

-- Show some Unit 2 Amey examples
SELECT 'Examples:' as description, full_address FROM dim_address_expanded WHERE UPPER(full_address) LIKE '%UNIT%2%AMEY%' LIMIT 5;

-- Check total addresses in expanded table
SELECT 'Total expanded table' as description, COUNT(*) as count FROM dim_address_expanded;