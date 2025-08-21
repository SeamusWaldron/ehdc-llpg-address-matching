-- Migration 018: Create Address Dimension with Proper Column Sizes
-- Purpose: Create dim_original_address with column sizes that match the data
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- Create the dim_original_address table with proper column sizes
CREATE TABLE dim_original_address (
    original_address_id     SERIAL PRIMARY KEY,
    raw_address            TEXT NOT NULL,
    address_hash           VARCHAR(64) NOT NULL UNIQUE,
    
    -- Parsed components (not used in this case)
    address_line_1         VARCHAR(255),
    address_line_2         VARCHAR(255),
    town                   VARCHAR(100),
    county                 VARCHAR(100),
    postcode               VARCHAR(20),
    country                VARCHAR(50),
    
    -- Standardized components (from gopostal) - increased sizes based on actual data
    std_house_number       VARCHAR(50),  -- Was 20, now 50
    std_house_name         VARCHAR(150), -- Was 100, now 150
    std_road               VARCHAR(300), -- Was 200, now 300
    std_suburb             VARCHAR(150), -- Was 100, now 150
    std_city               VARCHAR(150), -- Was 100, now 150
    std_state_district     VARCHAR(150), -- Was 100, now 150
    std_state              VARCHAR(100),
    std_postcode           VARCHAR(20),  -- Was 10, now 20
    std_country            VARCHAR(50),
    std_unit               VARCHAR(100), -- Was 50, now 100
    
    -- Quality metrics
    address_quality_score  DECIMAL(3,2),
    component_completeness DECIMAL(3,2),
    gopostal_processed    BOOLEAN DEFAULT FALSE,
    
    -- Usage tracking
    usage_count           INTEGER DEFAULT 0,
    first_seen            TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_used             TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Audit
    created_at            TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at            TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Add the foreign key constraint back to fact table
ALTER TABLE fact_documents_lean 
ADD CONSTRAINT fact_documents_lean_original_address_id_fkey 
FOREIGN KEY (original_address_id) REFERENCES dim_original_address(original_address_id);

-- Create indexes
CREATE INDEX idx_dim_original_address_hash ON dim_original_address(address_hash);
CREATE INDEX idx_dim_original_address_postcode ON dim_original_address(std_postcode) WHERE std_postcode IS NOT NULL;
CREATE INDEX idx_dim_original_address_road_city ON dim_original_address(std_road, std_city) WHERE std_road IS NOT NULL;
CREATE INDEX idx_dim_original_address_usage ON dim_original_address(usage_count DESC);

-- Simple insert approach - just populate with sample data for now
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
    usage_count
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
    CASE 
        WHEN sd.gopostal_processed = TRUE AND sd.gopostal_postcode IS NOT NULL THEN 0.9
        WHEN sd.gopostal_postcode IS NOT NULL THEN 0.6
        ELSE 0.3
    END as address_quality_score,
    (
        CASE WHEN sd.raw_address IS NOT NULL AND sd.raw_address != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_postcode IS NOT NULL AND sd.gopostal_postcode != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_road IS NOT NULL AND sd.gopostal_road != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_city IS NOT NULL AND sd.gopostal_city != '' THEN 0.2 ELSE 0 END +
        CASE WHEN sd.gopostal_house_number IS NOT NULL AND sd.gopostal_house_number != '' THEN 0.2 ELSE 0 END
    ) as component_completeness,
    COALESCE(sd.gopostal_processed, FALSE),
    1 as usage_count
FROM src_document sd
WHERE sd.raw_address IS NOT NULL 
  AND sd.raw_address != ''
LIMIT 1000; -- Start with a sample

-- Update usage counts for the sample
UPDATE dim_original_address 
SET usage_count = subq.cnt
FROM (
    SELECT 
        MD5(LOWER(TRIM(raw_address))) as hash,
        COUNT(*) as cnt
    FROM src_document 
    WHERE raw_address IS NOT NULL 
      AND MD5(LOWER(TRIM(raw_address))) IN (SELECT address_hash FROM dim_original_address)
    GROUP BY MD5(LOWER(TRIM(raw_address)))
) subq
WHERE dim_original_address.address_hash = subq.hash;

-- Show results
SELECT 
    'Sample Address Population' as info,
    COUNT(*) as addresses_created,
    COUNT(CASE WHEN gopostal_processed = TRUE THEN 1 END) as gopostal_processed,
    ROUND(AVG(address_quality_score), 3) as avg_quality,
    MAX(usage_count) as max_usage
FROM dim_original_address;

COMMIT;