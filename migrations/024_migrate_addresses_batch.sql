-- Migration 024: Batch Address Migration
-- Purpose: Efficiently migrate all unique addresses using temporary table approach
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- Create temporary table with all unique addresses and their usage counts
CREATE TEMP TABLE temp_unique_addresses AS
SELECT 
    raw_address,
    MD5(LOWER(TRIM(raw_address))) as address_hash,
    SUBSTRING(gopostal_house_number, 1, 50) as std_house_number,
    SUBSTRING(gopostal_house, 1, 150) as std_house_name,
    SUBSTRING(gopostal_road, 1, 300) as std_road,
    SUBSTRING(gopostal_suburb, 1, 150) as std_suburb,
    SUBSTRING(gopostal_city, 1, 150) as std_city,
    SUBSTRING(gopostal_state_district, 1, 150) as std_state_district,
    SUBSTRING(gopostal_state, 1, 100) as std_state,
    SUBSTRING(gopostal_postcode, 1, 20) as std_postcode,
    SUBSTRING(gopostal_country, 1, 50) as std_country,
    SUBSTRING(gopostal_unit, 1, 100) as std_unit,
    -- Take the best quality values for each unique address
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
FROM src_document
WHERE raw_address IS NOT NULL 
  AND raw_address != ''
GROUP BY 
    raw_address,
    MD5(LOWER(TRIM(raw_address))),
    SUBSTRING(gopostal_house_number, 1, 50),
    SUBSTRING(gopostal_house, 1, 150),
    SUBSTRING(gopostal_road, 1, 300),
    SUBSTRING(gopostal_suburb, 1, 150),
    SUBSTRING(gopostal_city, 1, 150),
    SUBSTRING(gopostal_state_district, 1, 150),
    SUBSTRING(gopostal_state, 1, 100),
    SUBSTRING(gopostal_postcode, 1, 20),
    SUBSTRING(gopostal_country, 1, 50),
    SUBSTRING(gopostal_unit, 1, 100);

-- Create index on temp table for performance
CREATE INDEX ON temp_unique_addresses(address_hash);

-- Get stats on temp table
SELECT 
    'Temp Table Created' as status,
    COUNT(*) as unique_addresses,
    SUM(usage_count) as total_references,
    MAX(usage_count) as max_usage,
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

-- Show validation that we have all addresses
SELECT 
    'Validation Check' as info,
    (SELECT COUNT(*) FROM src_document WHERE raw_address IS NOT NULL) as source_addresses,
    (SELECT SUM(usage_count) FROM dim_original_address) as dimension_address_refs,
    CASE 
        WHEN (SELECT COUNT(*) FROM src_document WHERE raw_address IS NOT NULL) = 
             (SELECT SUM(usage_count) FROM dim_original_address)
        THEN 'PASS'
        ELSE 'FAIL'
    END as validation_status;

COMMIT;