-- Migration 022: Migrate All Addresses to Dimensional Model
-- Purpose: Populate dim_original_address with all unique addresses from the full dataset
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- Clear existing sample data
TRUNCATE TABLE fact_documents_lean;
TRUNCATE TABLE dim_original_address RESTART IDENTITY CASCADE;

-- Populate all unique addresses with efficient batch processing
-- This approach uses INSERT with ON CONFLICT to handle duplicates efficiently
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
    MD5(LOWER(TRIM(raw_address))) as address_hash,
    SUBSTRING(gopostal_house_number, 1, 50),
    SUBSTRING(gopostal_house, 1, 150),
    SUBSTRING(gopostal_road, 1, 300),
    SUBSTRING(gopostal_suburb, 1, 150),
    SUBSTRING(gopostal_city, 1, 150),
    SUBSTRING(gopostal_state_district, 1, 150),
    SUBSTRING(gopostal_state, 1, 100),
    SUBSTRING(gopostal_postcode, 1, 20),
    SUBSTRING(gopostal_country, 1, 50),
    SUBSTRING(gopostal_unit, 1, 100),
    -- Calculate address quality score
    CASE 
        WHEN gopostal_processed = TRUE AND gopostal_postcode IS NOT NULL THEN 0.9
        WHEN gopostal_postcode IS NOT NULL THEN 0.6
        ELSE 0.3
    END as address_quality_score,
    -- Calculate component completeness
    (
        CASE WHEN raw_address IS NOT NULL AND raw_address != '' THEN 0.2 ELSE 0 END +
        CASE WHEN gopostal_postcode IS NOT NULL AND gopostal_postcode != '' THEN 0.2 ELSE 0 END +
        CASE WHEN gopostal_road IS NOT NULL AND gopostal_road != '' THEN 0.2 ELSE 0 END +
        CASE WHEN gopostal_city IS NOT NULL AND gopostal_city != '' THEN 0.2 ELSE 0 END +
        CASE WHEN gopostal_house_number IS NOT NULL AND gopostal_house_number != '' THEN 0.2 ELSE 0 END
    ) as component_completeness,
    COALESCE(gopostal_processed, FALSE),
    COUNT(*) as usage_count, -- Count duplicates in the same query
    NOW() as created_at
FROM src_document
WHERE raw_address IS NOT NULL 
  AND raw_address != ''
GROUP BY 
    raw_address,
    MD5(LOWER(TRIM(raw_address))),
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
ON CONFLICT (address_hash) DO UPDATE SET
    usage_count = EXCLUDED.usage_count,
    last_used = NOW(),
    updated_at = NOW();

-- Get results of address population
SELECT 
    'Address Migration Results' as info,
    COUNT(*) as unique_addresses_created,
    COUNT(CASE WHEN gopostal_processed = TRUE THEN 1 END) as gopostal_processed_addresses,
    SUM(usage_count) as total_address_references,
    MAX(usage_count) as max_address_usage,
    ROUND(AVG(address_quality_score), 3) as avg_address_quality,
    ROUND(AVG(component_completeness), 3) as avg_component_completeness
FROM dim_original_address;

-- Show top 10 most frequently used addresses
SELECT 
    'Most Frequently Used Addresses' as info,
    raw_address,
    usage_count,
    std_city,
    std_postcode,
    address_quality_score
FROM dim_original_address 
ORDER BY usage_count DESC 
LIMIT 10;

COMMIT;