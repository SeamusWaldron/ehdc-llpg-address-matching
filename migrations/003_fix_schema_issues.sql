-- Fix migration issues from 002_normalized_schema.sql
-- 1. Fix ambiguous column reference in UPDATE statement
-- 2. Handle duplicate UPRNs in dim_address table

-- First, fix the ambiguous column reference issue
UPDATE dim_location 
SET 
    easting = COALESCE(dim_location.easting, NULLIF(s.easting, '')::NUMERIC),
    northing = COALESCE(dim_location.northing, NULLIF(s.northing, '')::NUMERIC),
    geom_27700 = COALESCE(dim_location.geom_27700, 
        CASE 
            WHEN s.easting IS NOT NULL AND s.northing IS NOT NULL AND s.easting != '' AND s.northing != ''
            THEN ST_SetSRID(ST_MakePoint(s.easting::NUMERIC, s.northing::NUMERIC), 27700)
            ELSE NULL
        END
    )
FROM stg_ehdc_llpg s
WHERE dim_location.uprn = s.bs7666uprn
  AND dim_location.source_dataset = 'os_uprn'
  AND (dim_location.easting IS NULL OR dim_location.northing IS NULL);

-- Handle duplicate UPRNs in address table by adding a unique constraint that allows NULLs
-- First, drop the existing unique constraint if it exists
ALTER TABLE dim_address DROP CONSTRAINT IF EXISTS dim_address_uprn_key;

-- Create a partial unique index that allows multiple NULLs
CREATE UNIQUE INDEX idx_dim_address_uprn_unique ON dim_address(uprn) WHERE uprn IS NOT NULL;

-- Now insert EHDC LLPG addresses, handling duplicates with ON CONFLICT
INSERT INTO dim_address (
    location_id,
    uprn,
    full_address,
    address_canonical,
    usrn,
    blpu_class,
    postal_flag,
    status_code
)
SELECT 
    l.location_id,
    s.bs7666uprn,
    s.locaddress,
    -- Basic address canonicalization (uppercase, remove extra spaces)
    REGEXP_REPLACE(
        REGEXP_REPLACE(UPPER(TRIM(s.locaddress)), '\s+', ' ', 'g'),
        '[^A-Z0-9 ]', '', 'g'
    ),
    s.bs7666usrn,
    s.blpuclass,
    CASE WHEN UPPER(TRIM(s.postal)) IN ('Y', 'YES', 'TRUE', '1') THEN TRUE ELSE FALSE END,
    s.lgcstatusc
FROM stg_ehdc_llpg s
INNER JOIN dim_location l ON l.uprn = s.bs7666uprn
WHERE s.bs7666uprn IS NOT NULL 
  AND s.bs7666uprn != ''
  AND s.locaddress IS NOT NULL 
  AND s.locaddress != ''
ON CONFLICT (uprn) DO UPDATE SET
    full_address = EXCLUDED.full_address,
    address_canonical = EXCLUDED.address_canonical,
    usrn = EXCLUDED.usrn,
    blpu_class = EXCLUDED.blpu_class,
    postal_flag = EXCLUDED.postal_flag,
    status_code = EXCLUDED.status_code;