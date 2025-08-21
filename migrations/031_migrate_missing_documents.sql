-- Migration 031: Migrate Missing Documents to src_document
-- Purpose: Migrate all missing document types from staging to src_document table
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- Get current max document_id to continue sequence
SELECT 'Starting migration of missing documents to src_document' as status;

-- 1. Migrate Street Name and Numbering records (type_id = 5)
INSERT INTO src_document (
    doc_type_id,
    job_number,
    filepath,
    raw_address,
    address_canonical,
    raw_uprn,
    raw_easting,
    raw_northing,
    created_at
)
SELECT 
    5 as doc_type_id,
    job_number,
    filepath,
    address as raw_address,
    LOWER(REGEXP_REPLACE(address, '[^a-zA-Z0-9 ]', '', 'g')) as address_canonical,
    bs7666uprn as raw_uprn,
    easting as raw_easting,
    northing as raw_northing,
    NOW() as created_at
FROM stg_street_name_numbering
WHERE address IS NOT NULL AND TRIM(address) <> '';

-- Street Name and Numbering migration completed

-- 2. Migrate Microfiche Post-1974 records (type_id = 6)
-- Note: These have planning references but no addresses, so we'll use the planning reference as the address
INSERT INTO src_document (
    doc_type_id,
    job_number,
    filepath,
    external_reference,
    raw_address,
    address_canonical,
    created_at
)
SELECT 
    6 as doc_type_id,
    job_number,
    filepath,
    planning_application_reference as external_reference,
    COALESCE(planning_application_reference, 'No Address Available') as raw_address,
    LOWER(REGEXP_REPLACE(COALESCE(planning_application_reference, 'no address available'), '[^a-zA-Z0-9 ]', '', 'g')) as address_canonical,
    NOW() as created_at
FROM stg_microfiche_post_1974;

-- Microfiche Post-1974 migration completed

-- 3. Migrate Microfiche Pre-1974 records (type_id = 7)
-- Note: These have planning references but no addresses, so we'll use the planning reference as the address
INSERT INTO src_document (
    doc_type_id,
    job_number,
    filepath,
    external_reference,
    raw_address,
    address_canonical,
    created_at
)
SELECT 
    7 as doc_type_id,
    job_number,
    filepath,
    planning_application_reference as external_reference,
    COALESCE(planning_application_reference, 'No Address Available') as raw_address,
    LOWER(REGEXP_REPLACE(COALESCE(planning_application_reference, 'no address available'), '[^a-zA-Z0-9 ]', '', 'g')) as address_canonical,
    NOW() as created_at
FROM stg_microfiche_pre_1974;

-- Microfiche Pre-1974 migration completed

-- 4. Migrate Enlargement Maps records (type_id = 8)
-- Note: These have map numbers but no addresses, so we'll use the map number as the address
INSERT INTO src_document (
    doc_type_id,
    job_number,
    filepath,
    external_reference,
    raw_address,
    address_canonical,
    created_at
)
SELECT 
    8 as doc_type_id,
    job_number,
    filepath,
    enlargement_map_number as external_reference,
    COALESCE('Enlargement Map ' || enlargement_map_number, 'No Address Available') as raw_address,
    LOWER(REGEXP_REPLACE(COALESCE('enlargement map ' || enlargement_map_number, 'no address available'), '[^a-zA-Z0-9 ]', '', 'g')) as address_canonical,
    NOW() as created_at
FROM stg_enlargement_maps;

-- Enlargement Maps migration completed

-- 5. Migrate ENL Folders records (type_id = 9)
INSERT INTO src_document (
    doc_type_id,
    job_number,
    filepath,
    raw_address,
    address_canonical,
    raw_uprn,
    raw_easting,
    raw_northing,
    created_at
)
SELECT 
    9 as doc_type_id,
    job_number,
    filepath,
    address as raw_address,
    LOWER(REGEXP_REPLACE(address, '[^a-zA-Z0-9 ]', '', 'g')) as address_canonical,
    bs7666uprn as raw_uprn,
    easting as raw_easting,
    northing as raw_northing,
    NOW() as created_at
FROM stg_enl_folders
WHERE address IS NOT NULL AND TRIM(address) <> '';

-- ENL Folders migration completed

-- Summary statistics
SELECT 'Migration Summary:' as status;

SELECT 
    ddt.type_name,
    COUNT(*) as record_count
FROM src_document sd
JOIN dim_document_type ddt ON sd.doc_type_id = ddt.doc_type_id
WHERE sd.doc_type_id IN (5, 6, 7, 8, 9)
GROUP BY ddt.doc_type_id, ddt.type_name
ORDER BY record_count DESC;

-- Total count
SELECT 
    'Total documents in system: ' || COUNT(*) || ' records' as final_status
FROM src_document;

SELECT 'Migration completed successfully' as status;

COMMIT;