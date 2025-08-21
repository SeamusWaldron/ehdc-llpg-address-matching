-- Migration 023: Simple Address Migration
-- Purpose: Migrate all unique addresses using a simpler approach
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- Step 1: Insert all unique addresses (without usage count for now)
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
    SUBSTRING(sd.gopostal_house_number, 1, 50),
    SUBSTRING(sd.gopostal_house, 1, 150),
    SUBSTRING(sd.gopostal_road, 1, 300),
    SUBSTRING(sd.gopostal_suburb, 1, 150),
    SUBSTRING(sd.gopostal_city, 1, 150),
    SUBSTRING(sd.gopostal_state_district, 1, 150),
    SUBSTRING(sd.gopostal_state, 1, 100),
    SUBSTRING(sd.gopostal_postcode, 1, 20),
    SUBSTRING(sd.gopostal_country, 1, 50),
    SUBSTRING(sd.gopostal_unit, 1, 100),
    -- Calculate address quality score
    CASE 
        WHEN sd.gopostal_processed = TRUE AND sd.gopostal_postcode IS NOT NULL THEN 0.9
        WHEN sd.gopostal_postcode IS NOT NULL THEN 0.6
        ELSE 0.3
    END as address_quality_score,
    -- Calculate component completeness
    (
        CASE WHEN sd.raw_address IS NOT NULL AND sd.raw_address != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_postcode IS NOT NULL AND sd.gopostal_postcode != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_road IS NOT NULL AND sd.gopostal_road != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_city IS NOT NULL AND sd.gopostal_city != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_house_number IS NOT NULL AND sd.gopostal_house_number != '' THEN 0.2 ELSE 0 END
    ) as component_completeness,
    COALESCE(sd.gopostal_processed, FALSE),
    1 as usage_count, -- Start with 1, will update later
    NOW() as created_at
FROM src_document sd
WHERE sd.raw_address IS NOT NULL 
  AND sd.raw_address != '';

-- Get initial results
SELECT 
    'Step 1: Unique Addresses Inserted' as status,
    COUNT(*) as unique_addresses
FROM dim_original_address;

COMMIT;