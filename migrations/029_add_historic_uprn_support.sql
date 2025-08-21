-- Migration 029: Add Historic UPRN Support
-- Purpose: Add is_historic flag and support for historic UPRNs from source documents
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- 1. Add is_historic flag to dim_address
ALTER TABLE dim_address 
ADD COLUMN IF NOT EXISTS is_historic BOOLEAN DEFAULT FALSE;

-- Create index for historic records
CREATE INDEX IF NOT EXISTS idx_dim_address_historic 
ON dim_address(is_historic);

-- Create index for UPRN lookups
CREATE INDEX IF NOT EXISTS idx_dim_address_uprn 
ON dim_address(uprn);

-- 2. Add columns for tracking historic record source
ALTER TABLE dim_address
ADD COLUMN IF NOT EXISTS created_from_source BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS source_document_id INTEGER,
ADD COLUMN IF NOT EXISTS historic_created_at TIMESTAMP WITH TIME ZONE;

-- 3. Create a function to create historic UPRN records
CREATE OR REPLACE FUNCTION create_historic_uprn_record(
    p_uprn TEXT,
    p_full_address TEXT,
    p_document_id INTEGER
) RETURNS INTEGER AS $$
DECLARE
    v_address_id INTEGER;
    v_location_id INTEGER;
BEGIN
    -- Check if UPRN already exists
    SELECT address_id INTO v_address_id
    FROM dim_address
    WHERE uprn = p_uprn;
    
    IF v_address_id IS NOT NULL THEN
        -- UPRN already exists, return existing ID
        RETURN v_address_id;
    END IF;
    
    -- Create a default location for historic records (0,0 coordinates)
    INSERT INTO dim_location (easting, northing, latitude, longitude)
    VALUES (0, 0, 0, 0)
    RETURNING location_id INTO v_location_id;
    
    -- Create the historic address record
    INSERT INTO dim_address (
        location_id,
        uprn,
        full_address,
        address_canonical,
        is_historic,
        created_from_source,
        source_document_id,
        historic_created_at,
        created_at
    ) VALUES (
        v_location_id,
        p_uprn,
        p_full_address,
        LOWER(REGEXP_REPLACE(p_full_address, '[^a-zA-Z0-9 ]', '', 'g')), -- Simple canonicalization
        TRUE,
        TRUE,
        p_document_id,
        NOW(),
        NOW()
    )
    RETURNING address_id INTO v_address_id;
    
    RETURN v_address_id;
END;
$$ LANGUAGE plpgsql;

-- 4. Create view for historic UPRNs that need to be created
CREATE OR REPLACE VIEW vw_missing_uprns AS
SELECT DISTINCT 
    sd.raw_uprn,
    sd.document_id,
    sd.raw_address,
    NOT EXISTS(SELECT 1 FROM dim_address WHERE uprn = sd.raw_uprn) as needs_creation
FROM src_document sd
WHERE sd.raw_uprn IS NOT NULL
  AND sd.raw_uprn != ''
  AND NOT EXISTS(SELECT 1 FROM dim_address WHERE uprn = sd.raw_uprn)
ORDER BY sd.document_id;

-- 5. Add comment explaining historic records
COMMENT ON COLUMN dim_address.is_historic IS 
'TRUE if this address was created from a source document UPRN that did not exist in the original LLPG data';

COMMENT ON COLUMN dim_address.created_from_source IS 
'TRUE if this record was created from a source document rather than imported from LLPG';

COMMENT ON COLUMN dim_address.source_document_id IS 
'The document_id from src_document that caused this historic record to be created';

-- 6. Report on missing UPRNs
SELECT 
    'Missing UPRNs Summary' as report;

SELECT 
    COUNT(DISTINCT sd.raw_uprn) as unique_missing_uprns,
    COUNT(*) as total_documents_affected
FROM src_document sd
WHERE sd.raw_uprn IS NOT NULL
  AND sd.raw_uprn != ''
  AND NOT EXISTS(SELECT 1 FROM dim_address WHERE uprn = sd.raw_uprn);

-- Show sample of missing UPRNs
SELECT 
    'Sample Missing UPRNs (first 10)' as report;

SELECT 
    raw_uprn,
    COUNT(*) as document_count,
    STRING_AGG(DISTINCT LEFT(raw_address, 50), '; ') as sample_addresses
FROM src_document sd
WHERE sd.raw_uprn IS NOT NULL
  AND sd.raw_uprn != ''
  AND NOT EXISTS(SELECT 1 FROM dim_address WHERE uprn = sd.raw_uprn)
GROUP BY raw_uprn
ORDER BY COUNT(*) DESC
LIMIT 10;

COMMIT;