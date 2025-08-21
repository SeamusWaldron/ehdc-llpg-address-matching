-- Migration 025: Fixed Address Migration  
-- Purpose: Correctly migrate all unique addresses grouped by address hash only
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- Create temp table with properly deduplicated addresses
CREATE TEMP TABLE temp_unique_addresses AS
SELECT 
    -- Use the first occurrence of each field for the unique address
    (array_agg(raw_address ORDER BY document_id))[1] as raw_address,
    address_hash,
    (array_agg(SUBSTRING(gopostal_house_number, 1, 50) ORDER BY CASE WHEN gopostal_house_number IS NOT NULL THEN 0 ELSE 1 END, document_id))[1] as std_house_number,
    (array_agg(SUBSTRING(gopostal_house, 1, 150) ORDER BY CASE WHEN gopostal_house IS NOT NULL THEN 0 ELSE 1 END, document_id))[1] as std_house_name,
    (array_agg(SUBSTRING(gopostal_road, 1, 300) ORDER BY CASE WHEN gopostal_road IS NOT NULL THEN 0 ELSE 1 END, document_id))[1] as std_road,
    (array_agg(SUBSTRING(gopostal_suburb, 1, 150) ORDER BY CASE WHEN gopostal_suburb IS NOT NULL THEN 0 ELSE 1 END, document_id))[1] as std_suburb,
    (array_agg(SUBSTRING(gopostal_city, 1, 150) ORDER BY CASE WHEN gopostal_city IS NOT NULL THEN 0 ELSE 1 END, document_id))[1] as std_city,
    (array_agg(SUBSTRING(gopostal_state_district, 1, 150) ORDER BY CASE WHEN gopostal_state_district IS NOT NULL THEN 0 ELSE 1 END, document_id))[1] as std_state_district,
    (array_agg(SUBSTRING(gopostal_state, 1, 100) ORDER BY CASE WHEN gopostal_state IS NOT NULL THEN 0 ELSE 1 END, document_id))[1] as std_state,
    (array_agg(SUBSTRING(gopostal_postcode, 1, 20) ORDER BY CASE WHEN gopostal_postcode IS NOT NULL THEN 0 ELSE 1 END, document_id))[1] as std_postcode,
    (array_agg(SUBSTRING(gopostal_country, 1, 50) ORDER BY CASE WHEN gopostal_country IS NOT NULL THEN 0 ELSE 1 END, document_id))[1] as std_country,
    (array_agg(SUBSTRING(gopostal_unit, 1, 100) ORDER BY CASE WHEN gopostal_unit IS NOT NULL THEN 0 ELSE 1 END, document_id))[1] as std_unit,
    -- Take the best quality score from all instances
    MAX(CASE 
        WHEN gopostal_processed = TRUE AND gopostal_postcode IS NOT NULL THEN 0.9
        WHEN gopostal_postcode IS NOT NULL THEN 0.6
        ELSE 0.3
    END) as address_quality_score,
    -- Take the best component completeness
    MAX(
        CASE WHEN raw_address IS NOT NULL AND raw_address != '' THEN 0.2 ELSE 0 END +
        CASE WHEN gopostal_postcode IS NOT NULL AND gopostal_postcode != '' THEN 0.2 ELSE 0 END +
        CASE WHEN gopostal_road IS NOT NULL AND gopostal_road != '' THEN 0.2 ELSE 0 END +
        CASE WHEN gopostal_city IS NOT NULL AND gopostal_city != '' THEN 0.2 ELSE 0 END +
        CASE WHEN gopostal_house_number IS NOT NULL AND gopostal_house_number != '' THEN 0.2 ELSE 0 END
    ) as component_completeness,
    BOOL_OR(COALESCE(gopostal_processed, FALSE)) as gopostal_processed,
    COUNT(*) as usage_count
FROM (
    SELECT 
        document_id,
        raw_address,
        MD5(LOWER(TRIM(raw_address))) as address_hash,
        gopostal_house_number,
        gopostal_house,
        gopostal_road,
        gopostal_suburb,
        gopostal_city,
        gopostal_state_district,
        gopostal_state,
        gopostal_postcode,
        gopostal_country,
        gopostal_unit,
        gopostal_processed
    FROM src_document
    WHERE raw_address IS NOT NULL 
      AND raw_address != ''
) subq
GROUP BY address_hash;

-- Get stats
SELECT 
    'Address Deduplication Results' as status,
    COUNT(*) as unique_addresses,
    SUM(usage_count) as total_references,
    MAX(usage_count) as max_usage,
    MIN(usage_count) as min_usage,
    COUNT(CASE WHEN gopostal_processed = TRUE THEN 1 END) as with_gopostal
FROM temp_unique_addresses;

-- Insert into dimension table
INSERT INTO dim_original_address (
    raw_address,
    address_hash,
    std_house_number,
    std_house_name,
    std_road,
    std_suburb,
    std_city,
    std_state_district,
    std_state,
    std_postcode,
    std_country,
    std_unit,
    address_quality_score,
    component_completeness,
    gopostal_processed,
    usage_count,
    created_at
)
SELECT 
    raw_address,
    address_hash,
    std_house_number,
    std_house_name,
    std_road,
    std_suburb,
    std_city,
    std_state_district,
    std_state,
    std_postcode,
    std_country,
    std_unit,
    address_quality_score,
    component_completeness,
    gopostal_processed,
    usage_count,
    NOW() as created_at
FROM temp_unique_addresses;

-- Final results
SELECT 
    'Address Migration Completed' as status,
    COUNT(*) as unique_addresses_created,
    COUNT(CASE WHEN gopostal_processed = TRUE THEN 1 END) as gopostal_processed_addresses,
    SUM(usage_count) as total_address_references,
    MAX(usage_count) as max_address_usage,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(component_completeness), 3) as avg_component_completeness
FROM dim_original_address;

-- Show most frequently used addresses
SELECT 
    'Top 10 Most Used Addresses' as info,
    raw_address,
    usage_count,
    std_city,
    std_postcode
FROM dim_original_address 
ORDER BY usage_count DESC 
LIMIT 10;

COMMIT;