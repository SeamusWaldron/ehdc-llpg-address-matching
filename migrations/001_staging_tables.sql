-- EHDC LLPG Address Matching System - Staging Tables
-- Load raw data first, then design proper schema

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;

-- EHDC LLPG staging table (from ehdc_llpg_20250710.csv)
-- ogc_fid,locaddress,easting,northing,lgcstatusc,bs7666uprn,bs7666usrn,landparcel,blpuclass,postal
CREATE TABLE IF NOT EXISTS stg_ehdc_llpg (
    ogc_fid text,
    locaddress text,
    easting text,
    northing text,
    lgcstatusc text,
    bs7666uprn text,
    bs7666usrn text,
    landparcel text,
    blpuclass text,
    postal text,
    loaded_at timestamptz DEFAULT now()
);

-- OS UPRN staging table (from osopenuprn_202507.csv)
-- UPRN,X_COORDINATE,Y_COORDINATE,LATITUDE,LONGITUDE
CREATE TABLE IF NOT EXISTS stg_os_uprn (
    uprn text,
    x_coordinate text,
    y_coordinate text,
    latitude text,
    longitude text,
    loaded_at timestamptz DEFAULT now()
);

-- Decision notices staging (from decision_notices.csv)
-- Job Number,Filepath,Planning Application Number,Adress,Decision Date,Decision Type,Document Type,BS7666UPRN,Easting,Northing
CREATE TABLE IF NOT EXISTS stg_decision_notices (
    job_number text,
    filepath text,
    planning_application_number text,
    adress text,  -- Note: typo preserved from source
    decision_date text,
    decision_type text,
    document_type text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);

-- Land charges staging (from land_charges_cards.csv)
-- Job Number,Filepath,Card Code,Address,BS7666UPRN,Easting,Northing
CREATE TABLE IF NOT EXISTS stg_land_charges (
    job_number text,
    filepath text,
    card_code text,
    address text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);

-- Enforcement notices staging (from enforcement_notices.csv)  
-- Job Number,Filepath,Planning Enforcement Reference Number,Address,Date,Document Type,BS7666UPRN,Easting,Northing
CREATE TABLE IF NOT EXISTS stg_enforcement_notices (
    job_number text,
    filepath text,
    planning_enforcement_reference_number text,
    address text,
    date text,
    document_type text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);

-- Agreements staging (from agreements.csv)
-- Job Number,Filepath,Address,Date,BS7666UPRN,Easting,Northing
CREATE TABLE IF NOT EXISTS stg_agreements (
    job_number text,
    filepath text,
    address text,
    date text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);