-- Migration 030: Create Missing Staging Tables
-- Purpose: Create staging tables for the missing source document types
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- 1. Street Name and Numbering staging table
-- Has: Job Number, Filepath, Address, BS7666UPRN, Easting, Northing
DROP TABLE IF EXISTS stg_street_name_numbering CASCADE;
CREATE TABLE stg_street_name_numbering (
    job_number TEXT,
    filepath TEXT,
    address TEXT,
    bs7666uprn TEXT,
    easting TEXT,
    northing TEXT
);

-- 2. Microfiche Post-1974 staging table  
-- Has: Job Number, Filepath, Planning Application Reference Number, Fiche Number
DROP TABLE IF EXISTS stg_microfiche_post_1974 CASCADE;
CREATE TABLE stg_microfiche_post_1974 (
    job_number TEXT,
    filepath TEXT,
    planning_application_reference TEXT,
    fiche_number TEXT
);

-- 3. Microfiche Pre-1974 staging table
-- Has: Job Number, Filepath, Planning Application Reference Number, Fiche Number  
DROP TABLE IF EXISTS stg_microfiche_pre_1974 CASCADE;
CREATE TABLE stg_microfiche_pre_1974 (
    job_number TEXT,
    filepath TEXT,
    planning_application_reference TEXT,
    fiche_number TEXT
);

-- 4. Enlargement Maps staging table
-- Has: Job Number, Filepath, Enlargement Map Number
DROP TABLE IF EXISTS stg_enlargement_maps CASCADE;
CREATE TABLE stg_enlargement_maps (
    job_number TEXT,
    filepath TEXT,
    enlargement_map_number TEXT
);

-- 5. ENL Folders staging table
-- Has: Job Number, Filepath, Address, BS7666UPRN, Easting, Northing
DROP TABLE IF EXISTS stg_enl_folders CASCADE;
CREATE TABLE stg_enl_folders (
    job_number TEXT,
    filepath TEXT,
    address TEXT,
    bs7666uprn TEXT,
    easting TEXT,
    northing TEXT
);

-- Add indexes for performance
CREATE INDEX IF NOT EXISTS idx_stg_street_name_numbering_job ON stg_street_name_numbering(job_number);
CREATE INDEX IF NOT EXISTS idx_stg_street_name_numbering_uprn ON stg_street_name_numbering(bs7666uprn);

CREATE INDEX IF NOT EXISTS idx_stg_microfiche_post_1974_job ON stg_microfiche_post_1974(job_number);
CREATE INDEX IF NOT EXISTS idx_stg_microfiche_post_1974_ref ON stg_microfiche_post_1974(planning_application_reference);

CREATE INDEX IF NOT EXISTS idx_stg_microfiche_pre_1974_job ON stg_microfiche_pre_1974(job_number);
CREATE INDEX IF NOT EXISTS idx_stg_microfiche_pre_1974_ref ON stg_microfiche_pre_1974(planning_application_reference);

CREATE INDEX IF NOT EXISTS idx_stg_enlargement_maps_job ON stg_enlargement_maps(job_number);
CREATE INDEX IF NOT EXISTS idx_stg_enlargement_maps_num ON stg_enlargement_maps(enlargement_map_number);

CREATE INDEX IF NOT EXISTS idx_stg_enl_folders_job ON stg_enl_folders(job_number);
CREATE INDEX IF NOT EXISTS idx_stg_enl_folders_uprn ON stg_enl_folders(bs7666uprn);

-- Add comments
COMMENT ON TABLE stg_street_name_numbering IS 'Staging table for street name and numbering records';
COMMENT ON TABLE stg_microfiche_post_1974 IS 'Staging table for microfiche records post-1974';
COMMENT ON TABLE stg_microfiche_pre_1974 IS 'Staging table for microfiche records pre-1974';
COMMENT ON TABLE stg_enlargement_maps IS 'Staging table for enlargement map records';
COMMENT ON TABLE stg_enl_folders IS 'Staging table for ENL folder records';

SELECT 'Created 5 staging tables for missing document types' as result;

COMMIT;