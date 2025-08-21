-- Test Script: Test Fixed Algorithm on HORNDEAN FOOTBALL CLUB addresses
-- Purpose: Verify the fixed algorithm correctly handles the problematic business addresses
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- Clear any existing matches for these documents
DELETE FROM address_match WHERE document_id IN (
    SELECT document_id FROM src_document 
    WHERE raw_address ILIKE '%HORNDEAN FOOTBALL CLUB%'
);

-- Show the source addresses we're testing
SELECT 
    'TESTING ADDRESSES' as section,
    document_id,
    raw_address,
    gopostal_house_number,
    gopostal_road,
    gopostal_city,
    gopostal_postcode
FROM src_document 
WHERE raw_address ILIKE '%HORNDEAN FOOTBALL CLUB%'
ORDER BY document_id
LIMIT 10;

ROLLBACK;