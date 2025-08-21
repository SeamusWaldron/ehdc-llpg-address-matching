-- Migration 006: Create Fact Documents Table
-- Purpose: Unified fact table consolidating source documents with address matching results
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- Create the main fact table
CREATE TABLE fact_documents (
    -- Primary Keys
    document_id             BIGINT PRIMARY KEY,
    fact_id                 BIGSERIAL UNIQUE NOT NULL,
    
    -- Source Document Information
    original_filename       VARCHAR(255),
    import_batch_id         INTEGER,
    import_timestamp        TIMESTAMP WITH TIME ZONE,
    document_type           VARCHAR(50),
    document_status         VARCHAR(20) DEFAULT 'active',
    
    -- Original Address Data
    raw_address             TEXT,
    parsed_address_line_1   VARCHAR(255),
    parsed_address_line_2   VARCHAR(255),
    parsed_town             VARCHAR(100),
    parsed_county           VARCHAR(100),
    parsed_postcode         VARCHAR(10),
    parsed_country          VARCHAR(50),
    
    -- Standardized Address Components (from gopostal)
    std_house_number        VARCHAR(20),
    std_house_name          VARCHAR(100),
    std_road                VARCHAR(200),
    std_suburb              VARCHAR(100),
    std_city                VARCHAR(100),
    std_state_district      VARCHAR(100),
    std_state               VARCHAR(100),
    std_postcode            VARCHAR(10),
    std_country             VARCHAR(50),
    std_unit                VARCHAR(50),
    
    -- Address Matching Results
    match_status            VARCHAR(20), -- 'matched', 'no_match', 'needs_review'
    match_method            VARCHAR(50), -- 'exact_components', 'postcode_house', etc.
    match_confidence        DECIMAL(5,4), -- 0.0000 to 1.0000
    match_decision          VARCHAR(20), -- 'auto_accept', 'needs_review', 'low_confidence'
    matched_uprn            VARCHAR(20), -- The golden UPRN if matched
    matched_address_id      INTEGER, -- Reference to dim_address
    matched_location_id     INTEGER, -- Reference to dim_location
    
    -- Matched Address Information (denormalized for performance)
    matched_full_address    TEXT,
    matched_address_canonical TEXT,
    matched_easting         DECIMAL(10,2),
    matched_northing        DECIMAL(10,2),
    matched_latitude        DECIMAL(10,8),
    matched_longitude       DECIMAL(11,8),
    
    -- Business Data Fields
    property_type           VARCHAR(100),
    property_description    TEXT,
    planning_reference      VARCHAR(50),
    application_date        DATE,
    decision_date           DATE,
    application_status      VARCHAR(50),
    development_type        VARCHAR(100),
    
    -- Additional Source Fields (flexible JSON for varying schemas)
    additional_data         JSONB,
    
    -- Data Quality Flags
    address_quality_score   DECIMAL(3,2), -- 0.00 to 1.00
    data_completeness_score DECIMAL(3,2), -- 0.00 to 1.00
    validation_flags        TEXT[], -- Array of validation warnings/notes
    
    -- Audit Information
    created_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processed_by            VARCHAR(50) DEFAULT 'system',
    processing_version      VARCHAR(20) DEFAULT '1.0',
    
    -- Foreign Key Constraints
    CONSTRAINT fk_matched_address FOREIGN KEY (matched_address_id) 
        REFERENCES dim_address(address_id),
    CONSTRAINT fk_matched_location FOREIGN KEY (matched_location_id) 
        REFERENCES dim_location(location_id),
        
    -- Check Constraints
    CONSTRAINT chk_match_confidence CHECK (match_confidence >= 0.0 AND match_confidence <= 1.0),
    CONSTRAINT chk_address_quality CHECK (address_quality_score >= 0.0 AND address_quality_score <= 1.0),
    CONSTRAINT chk_data_completeness CHECK (data_completeness_score >= 0.0 AND data_completeness_score <= 1.0),
    CONSTRAINT chk_match_status CHECK (match_status IN ('matched', 'no_match', 'needs_review', 'pending')),
    CONSTRAINT chk_match_decision CHECK (match_decision IN ('auto_accept', 'needs_review', 'low_confidence', 'no_match'))
);

-- Create performance indexes
CREATE INDEX idx_fact_documents_uprn ON fact_documents(matched_uprn) WHERE matched_uprn IS NOT NULL;
CREATE INDEX idx_fact_documents_match_status ON fact_documents(match_status);
CREATE INDEX idx_fact_documents_postcode ON fact_documents(std_postcode) WHERE std_postcode IS NOT NULL;
CREATE INDEX idx_fact_documents_location ON fact_documents(matched_location_id) WHERE matched_location_id IS NOT NULL;

-- Spatial index for geographic queries
CREATE INDEX idx_fact_documents_spatial ON fact_documents USING GIST(
    ST_Point(matched_longitude, matched_latitude)
) WHERE matched_longitude IS NOT NULL AND matched_latitude IS NOT NULL;

-- Business query patterns
CREATE INDEX idx_fact_documents_planning_ref ON fact_documents(planning_reference) WHERE planning_reference IS NOT NULL;
CREATE INDEX idx_fact_documents_app_date ON fact_documents(application_date) WHERE application_date IS NOT NULL;
CREATE INDEX idx_fact_documents_property_type ON fact_documents(property_type) WHERE property_type IS NOT NULL;

-- Data quality indexes
CREATE INDEX idx_fact_documents_quality ON fact_documents(address_quality_score DESC, data_completeness_score DESC);
CREATE INDEX idx_fact_documents_confidence ON fact_documents(match_confidence DESC) WHERE match_confidence IS NOT NULL;

-- Composite indexes for common queries
CREATE INDEX idx_fact_documents_status_confidence ON fact_documents(match_status, match_confidence DESC);
CREATE INDEX idx_fact_documents_type_status ON fact_documents(document_type, match_status);

-- Create trigger for updating updated_at timestamp
CREATE OR REPLACE FUNCTION update_fact_documents_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tr_fact_documents_updated_at
    BEFORE UPDATE ON fact_documents
    FOR EACH ROW
    EXECUTE FUNCTION update_fact_documents_updated_at();

-- Add comments for documentation
COMMENT ON TABLE fact_documents IS 'Unified fact table containing all source documents with address matching results and UPRN associations';
COMMENT ON COLUMN fact_documents.document_id IS 'Primary key linking to original src_document';
COMMENT ON COLUMN fact_documents.matched_uprn IS 'UPRN from dim_address if successfully matched';
COMMENT ON COLUMN fact_documents.match_confidence IS 'Confidence score from address matching (0.0-1.0)';
COMMENT ON COLUMN fact_documents.address_quality_score IS 'Overall address data quality assessment';
COMMENT ON COLUMN fact_documents.validation_flags IS 'Array of data quality issues or validation warnings';

COMMIT;