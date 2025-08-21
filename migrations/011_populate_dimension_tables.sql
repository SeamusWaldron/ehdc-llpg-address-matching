-- Migration 011: Populate Dimension Tables from Staging Data
-- Purpose: Extract and populate dimension tables from existing staging data
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
    address_line_1,
    address_line_2,
    town,
    county,
    postcode,
    country,
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
    sd.address_line_1,
    sd.address_line_2,
    sd.town,
    sd.county,
    sd.postcode,
    sd.country,
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
        WHEN sd.postcode IS NOT NULL AND sd.raw_address IS NOT NULL THEN 0.6
        WHEN sd.raw_address IS NOT NULL THEN 0.3
        ELSE 0.1
    END as address_quality_score,
    -- Calculate component completeness
    (
        CASE WHEN sd.raw_address IS NOT NULL AND sd.raw_address != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.postcode IS NOT NULL AND sd.postcode != '' THEN 0.2 ELSE 0 END +
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

-- 2. Add any missing document types from staging data
INSERT INTO dim_document_type (document_type_code, document_type_name, document_category, description)
SELECT DISTINCT
    UPPER(COALESCE(document_type, 'UNKNOWN')) as document_type_code,
    CASE 
        WHEN document_type ILIKE '%planning%' THEN 'Planning Application'
        WHEN document_type ILIKE '%building%' THEN 'Building Regulations'
        WHEN document_type ILIKE '%appeal%' THEN 'Planning Appeal'
        WHEN document_type ILIKE '%enforce%' THEN 'Enforcement Notice'
        WHEN document_type ILIKE '%pre%app%' THEN 'Pre-Application'
        WHEN document_type IS NULL THEN 'Unknown Type'
        ELSE INITCAP(document_type)
    END as document_type_name,
    CASE 
        WHEN document_type ILIKE '%planning%' OR document_type ILIKE '%appeal%' OR document_type ILIKE '%pre%app%' THEN 'Planning'
        WHEN document_type ILIKE '%building%' THEN 'Building Control'
        WHEN document_type ILIKE '%enforce%' THEN 'Enforcement'
        ELSE 'Other'
    END as document_category,
    'Extracted from staging data during migration' as description
FROM src_document
WHERE document_type IS NOT NULL
ON CONFLICT (document_type_code) DO NOTHING;

-- 3. Add any missing application statuses from staging data
INSERT INTO dim_application_status (status_code, status_name, status_category, is_final_status, sort_order)
SELECT DISTINCT
    UPPER(COALESCE(application_status, 'UNKNOWN')) as status_code,
    CASE 
        WHEN application_status ILIKE '%submit%' THEN 'Submitted'
        WHEN application_status ILIKE '%valid%' THEN 'Validated'
        WHEN application_status ILIKE '%consult%' THEN 'Out for Consultation'
        WHEN application_status ILIKE '%assess%' THEN 'Under Assessment'
        WHEN application_status ILIKE '%approve%' THEN 'Approved'
        WHEN application_status ILIKE '%refuse%' OR application_status ILIKE '%reject%' THEN 'Refused'
        WHEN application_status ILIKE '%withdraw%' THEN 'Withdrawn'
        WHEN application_status ILIKE '%invalid%' THEN 'Invalid'
        WHEN application_status IS NULL THEN 'Unknown Status'
        ELSE INITCAP(application_status)
    END as status_name,
    CASE 
        WHEN application_status ILIKE '%submit%' OR application_status ILIKE '%valid%' THEN 'Initial'
        WHEN application_status ILIKE '%consult%' OR application_status ILIKE '%assess%' THEN 'Processing'
        WHEN application_status ILIKE '%approve%' OR application_status ILIKE '%refuse%' OR application_status ILIKE '%withdraw%' OR application_status ILIKE '%invalid%' THEN 'Final'
        ELSE 'Other'
    END as status_category,
    CASE 
        WHEN application_status ILIKE '%approve%' OR application_status ILIKE '%refuse%' OR application_status ILIKE '%withdraw%' OR application_status ILIKE '%invalid%' THEN TRUE
        ELSE FALSE
    END as is_final_status,
    CASE 
        WHEN application_status ILIKE '%submit%' THEN 1
        WHEN application_status ILIKE '%valid%' THEN 2
        WHEN application_status ILIKE '%consult%' THEN 3
        WHEN application_status ILIKE '%assess%' THEN 4
        WHEN application_status ILIKE '%approve%' THEN 10
        WHEN application_status ILIKE '%refuse%' THEN 11
        WHEN application_status ILIKE '%withdraw%' THEN 12
        WHEN application_status ILIKE '%invalid%' THEN 13
        ELSE 99
    END as sort_order
FROM src_document
WHERE application_status IS NOT NULL
ON CONFLICT (status_code) DO NOTHING;

-- 4. Add any missing property types from staging data
INSERT INTO dim_property_type (property_code, property_name, property_category, is_residential, is_commercial, sort_order)
SELECT DISTINCT
    UPPER(COALESCE(property_type, 'UNKNOWN')) as property_code,
    CASE 
        WHEN property_type ILIKE '%house%' THEN 'House'
        WHEN property_type ILIKE '%flat%' OR property_type ILIKE '%apartment%' THEN 'Flat/Apartment'
        WHEN property_type ILIKE '%bungalow%' THEN 'Bungalow'
        WHEN property_type ILIKE '%maisonette%' THEN 'Maisonette'
        WHEN property_type ILIKE '%office%' THEN 'Office Building'
        WHEN property_type ILIKE '%retail%' OR property_type ILIKE '%shop%' THEN 'Retail Unit'
        WHEN property_type ILIKE '%restaurant%' OR property_type ILIKE '%cafe%' THEN 'Restaurant/Cafe'
        WHEN property_type ILIKE '%industrial%' THEN 'Industrial Unit'
        WHEN property_type ILIKE '%warehouse%' THEN 'Warehouse'
        WHEN property_type ILIKE '%mixed%' THEN 'Mixed Use'
        WHEN property_type IS NULL THEN 'Unknown Type'
        ELSE INITCAP(property_type)
    END as property_name,
    CASE 
        WHEN property_type ILIKE '%house%' OR property_type ILIKE '%flat%' OR property_type ILIKE '%bungalow%' OR property_type ILIKE '%maisonette%' THEN 'Residential'
        WHEN property_type ILIKE '%office%' OR property_type ILIKE '%retail%' OR property_type ILIKE '%restaurant%' OR property_type ILIKE '%shop%' OR property_type ILIKE '%cafe%' THEN 'Commercial'
        WHEN property_type ILIKE '%industrial%' OR property_type ILIKE '%warehouse%' THEN 'Industrial'
        WHEN property_type ILIKE '%mixed%' THEN 'Mixed'
        ELSE 'Other'
    END as property_category,
    CASE 
        WHEN property_type ILIKE '%house%' OR property_type ILIKE '%flat%' OR property_type ILIKE '%bungalow%' OR property_type ILIKE '%maisonette%' THEN TRUE
        ELSE FALSE
    END as is_residential,
    CASE 
        WHEN property_type ILIKE '%office%' OR property_type ILIKE '%retail%' OR property_type ILIKE '%restaurant%' OR property_type ILIKE '%shop%' OR property_type ILIKE '%cafe%' THEN TRUE
        ELSE FALSE
    END as is_commercial,
    CASE 
        WHEN property_type ILIKE '%house%' THEN 1
        WHEN property_type ILIKE '%flat%' THEN 2
        WHEN property_type ILIKE '%bungalow%' THEN 3
        WHEN property_type ILIKE '%office%' THEN 10
        WHEN property_type ILIKE '%retail%' THEN 11
        WHEN property_type ILIKE '%industrial%' THEN 20
        ELSE 99
    END as sort_order
FROM src_document
WHERE property_type IS NOT NULL
ON CONFLICT (property_code) DO NOTHING;

-- 5. Add any missing development types from staging data
INSERT INTO dim_development_type (development_code, development_name, development_category, impact_level, sort_order)
SELECT DISTINCT
    UPPER(COALESCE(development_type, 'UNKNOWN')) as development_code,
    CASE 
        WHEN development_type ILIKE '%householder%' OR development_type ILIKE '%extension%' THEN 'Householder Extension'
        WHEN development_type ILIKE '%new%dwelling%' OR development_type ILIKE '%new%house%' THEN 'New Dwelling'
        WHEN development_type ILIKE '%subdivision%' OR development_type ILIKE '%plot%' THEN 'Plot Subdivision'
        WHEN development_type ILIKE '%change%use%' THEN 'Change of Use'
        WHEN development_type ILIKE '%commercial%' THEN 'New Commercial Building'
        WHEN development_type ILIKE '%industrial%' THEN 'Industrial Development'
        WHEN development_type ILIKE '%demolition%' THEN 'Demolition'
        WHEN development_type ILIKE '%listed%' THEN 'Listed Building Works'
        WHEN development_type IS NULL THEN 'Unknown Type'
        ELSE INITCAP(development_type)
    END as development_name,
    CASE 
        WHEN development_type ILIKE '%householder%' OR development_type ILIKE '%extension%' OR development_type ILIKE '%dwelling%' OR development_type ILIKE '%subdivision%' THEN 'Residential'
        WHEN development_type ILIKE '%commercial%' OR development_type ILIKE '%change%use%' THEN 'Commercial'
        WHEN development_type ILIKE '%industrial%' THEN 'Industrial'
        WHEN development_type ILIKE '%listed%' THEN 'Heritage'
        ELSE 'Other'
    END as development_category,
    CASE 
        WHEN development_type ILIKE '%householder%' OR development_type ILIKE '%extension%' OR development_type ILIKE '%subdivision%' OR development_type ILIKE '%demolition%' THEN 'Minor'
        WHEN development_type ILIKE '%dwelling%' OR development_type ILIKE '%commercial%' OR development_type ILIKE '%change%use%' OR development_type ILIKE '%listed%' THEN 'Major'
        WHEN development_type ILIKE '%industrial%' THEN 'Significant'
        ELSE 'Minor'
    END as impact_level,
    CASE 
        WHEN development_type ILIKE '%householder%' THEN 1
        WHEN development_type ILIKE '%dwelling%' THEN 2
        WHEN development_type ILIKE '%commercial%' THEN 10
        WHEN development_type ILIKE '%industrial%' THEN 20
        ELSE 90
    END as sort_order
FROM src_document
WHERE development_type IS NOT NULL
ON CONFLICT (development_code) DO NOTHING;

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

COMMIT;