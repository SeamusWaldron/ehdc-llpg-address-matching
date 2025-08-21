-- Migration 017: Simple Address Population
-- Purpose: Populate dim_original_address with a simpler approach
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- Simple insert with basic deduplication using DISTINCT
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
SELECT DISTINCT
    sd.raw_address,
    MD5(LOWER(TRIM(sd.raw_address))) as address_hash,
    sd.gopostal_house_number,
    sd.gopostal_house,
    sd.gopostal_road,
    sd.gopostal_suburb,
    sd.gopostal_city,
    sd.gopostal_state_district,
    sd.gopostal_state,
    sd.gopostal_postcode,
    sd.gopostal_country,
    sd.gopostal_unit,
    -- Calculate address quality score
    CASE 
        WHEN sd.gopostal_processed = TRUE AND sd.gopostal_postcode IS NOT NULL THEN 0.9
        WHEN sd.gopostal_postcode IS NOT NULL AND sd.raw_address IS NOT NULL THEN 0.6
        WHEN sd.raw_address IS NOT NULL THEN 0.3
        ELSE 0.1
    END as address_quality_score,
    -- Calculate component completeness
    (
        CASE WHEN sd.raw_address IS NOT NULL AND sd.raw_address != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_postcode IS NOT NULL AND sd.gopostal_postcode != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_road IS NOT NULL AND sd.gopostal_road != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_city IS NOT NULL AND sd.gopostal_city != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_house_number IS NOT NULL AND sd.gopostal_house_number != '' THEN 0.2 ELSE 0 END
    ) as component_completeness,
    COALESCE(sd.gopostal_processed, FALSE) as gopostal_processed,
    1 as usage_count, -- Start with 1, will update later
    NOW() as created_at
FROM src_document sd
WHERE sd.raw_address IS NOT NULL 
  AND sd.raw_address != '';

-- Now update usage counts efficiently
UPDATE dim_original_address 
SET usage_count = subq.cnt
FROM (
    SELECT 
        MD5(LOWER(TRIM(raw_address))) as hash,
        COUNT(*) as cnt
    FROM src_document 
    WHERE raw_address IS NOT NULL 
    GROUP BY MD5(LOWER(TRIM(raw_address)))
) subq
WHERE dim_original_address.address_hash = subq.hash;

-- Get results
SELECT 
    'Address Population Results' as info,
    COUNT(*) as unique_addresses,
    COUNT(CASE WHEN gopostal_processed = TRUE THEN 1 END) as gopostal_processed,
    COUNT(CASE WHEN usage_count > 1 THEN 1 END) as addresses_used_multiple_times,
    MAX(usage_count) as max_usage_count,
    ROUND(AVG(address_quality_score), 3) as avg_quality_score
FROM dim_original_address;

COMMIT;