-- Migration 016: Populate Dimension Tables (Compatible)
-- Purpose: Populate dimension tables using actual src_document column names
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- Function to create MD5 hash for address deduplication
CREATE OR REPLACE FUNCTION create_address_hash(address_text TEXT)
RETURNS VARCHAR(64) AS $$
BEGIN
    RETURN MD5(LOWER(TRIM(COALESCE(address_text, ''))));
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- 1. Populate dim_original_address from src_document
-- This will deduplicate addresses that appear multiple times
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
SELECT DISTINCT ON (create_address_hash(sd.raw_address))
    sd.raw_address,
    create_address_hash(sd.raw_address) as address_hash,
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
    -- Count how many times this address appears
    (SELECT COUNT(*) FROM src_document sd2 WHERE create_address_hash(sd2.raw_address) = create_address_hash(sd.raw_address)) as usage_count,
    NOW() as created_at
FROM src_document sd
WHERE sd.raw_address IS NOT NULL 
  AND sd.raw_address != ''
ON CONFLICT (address_hash) DO UPDATE SET
    usage_count = EXCLUDED.usage_count,
    last_used = NOW(),
    updated_at = NOW();

-- Update usage statistics for addresses
UPDATE dim_original_address 
SET usage_count = subq.cnt,
    last_used = NOW(),
    updated_at = NOW()
FROM (
    SELECT 
        create_address_hash(raw_address) as hash,
        COUNT(*) as cnt
    FROM src_document 
    WHERE raw_address IS NOT NULL 
    GROUP BY create_address_hash(raw_address)
) subq
WHERE dim_original_address.address_hash = subq.hash;

-- Display population statistics
SELECT 
    'Dimension Population Results' as info,
    (SELECT COUNT(*) FROM dim_original_address) as original_addresses,
    (SELECT COUNT(*) FROM dim_document_type) as document_types,
    (SELECT COUNT(*) FROM dim_document_status) as document_statuses,
    (SELECT COUNT(*) FROM dim_application_status) as application_statuses,
    (SELECT COUNT(*) FROM dim_property_type) as property_types,
    (SELECT COUNT(*) FROM dim_development_type) as development_types,
    (SELECT COUNT(*) FROM dim_match_method) as match_methods,
    (SELECT COUNT(*) FROM dim_match_decision) as match_decisions;

-- Show address deduplication results
SELECT 
    'Address Deduplication Summary' as info,
    (SELECT COUNT(*) FROM src_document WHERE raw_address IS NOT NULL) as source_addresses,
    (SELECT COUNT(*) FROM dim_original_address) as unique_addresses,
    (SELECT COUNT(*) FROM dim_original_address WHERE usage_count > 1) as addresses_used_multiple_times,
    (SELECT MAX(usage_count) FROM dim_original_address) as max_usage_count;

-- Show some sample deduplication results
SELECT 
    'Sample Address Usage' as info,
    raw_address,
    usage_count,
    component_completeness,
    address_quality_score
FROM dim_original_address 
WHERE usage_count > 1
ORDER BY usage_count DESC 
LIMIT 5;

COMMIT;