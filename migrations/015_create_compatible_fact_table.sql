-- Migration 015: Create Compatible Lean Fact Table
-- Purpose: Create fact table compatible with existing dimension table structures
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- Create the lean fact table using existing dimension column names
CREATE TABLE fact_documents_lean (
    -- Fact table surrogate key
    fact_id                    BIGSERIAL PRIMARY KEY,
    
    -- Business key
    document_id               BIGINT NOT NULL UNIQUE,
    
    -- Dimension foreign keys (using existing column names)
    doc_type_id               INTEGER REFERENCES dim_document_type(doc_type_id),
    document_status_id        INTEGER REFERENCES dim_document_status(document_status_id),
    original_address_id       INTEGER REFERENCES dim_original_address(original_address_id),
    matched_address_id        INTEGER REFERENCES dim_address(address_id), -- Existing LLPG
    matched_location_id       INTEGER REFERENCES dim_location(location_id), -- Existing
    match_method_id           INTEGER REFERENCES dim_match_method(method_id),
    match_decision_id         INTEGER REFERENCES dim_match_decision(match_decision_id),
    property_type_id          INTEGER REFERENCES dim_property_type(property_type_id),
    application_status_id     INTEGER REFERENCES dim_application_status(application_status_id),
    development_type_id       INTEGER REFERENCES dim_development_type(development_type_id),
    
    -- Date dimensions (using date_id from dim_date)
    application_date_id       INTEGER REFERENCES dim_date(date_id),
    decision_date_id          INTEGER REFERENCES dim_date(date_id),
    import_date_id            INTEGER REFERENCES dim_date(date_id),
    
    -- Measures (facts/metrics) - the actual numerical data
    match_confidence_score    DECIMAL(5,4), -- 0.0000 to 1.0000
    address_quality_score     DECIMAL(3,2), -- 0.00 to 1.00
    data_completeness_score   DECIMAL(3,2), -- 0.00 to 1.00
    processing_time_ms        INTEGER,      -- Time taken to process this record
    
    -- Business measures
    application_fee           DECIMAL(10,2),
    estimated_value          DECIMAL(12,2),
    floor_area_sqm           DECIMAL(10,2),
    
    -- Technical identifiers (keep minimal business keys)
    import_batch_id          INTEGER,
    planning_reference       VARCHAR(50), -- Keep as it's a true business identifier
    
    -- Boolean measures (computed flags)
    is_matched               BOOLEAN GENERATED ALWAYS AS (matched_address_id IS NOT NULL) STORED,
    is_auto_processed        BOOLEAN DEFAULT FALSE,
    has_validation_issues    BOOLEAN DEFAULT FALSE,
    is_high_confidence       BOOLEAN GENERATED ALWAYS AS (match_confidence_score >= 0.85) STORED,
    
    -- Minimal flexible data (use very sparingly)
    additional_measures      JSONB,
    
    -- Audit measures
    created_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processing_version      VARCHAR(20) DEFAULT '1.0'
);

-- Create indexes on the fact table for performance
CREATE INDEX idx_fact_documents_lean_document_id ON fact_documents_lean(document_id);
CREATE INDEX idx_fact_documents_lean_original_address ON fact_documents_lean(original_address_id) WHERE original_address_id IS NOT NULL;
CREATE INDEX idx_fact_documents_lean_matched_address ON fact_documents_lean(matched_address_id) WHERE matched_address_id IS NOT NULL;
CREATE INDEX idx_fact_documents_lean_application_date ON fact_documents_lean(application_date_id) WHERE application_date_id IS NOT NULL;
CREATE INDEX idx_fact_documents_lean_match_confidence ON fact_documents_lean(match_confidence_score) WHERE match_confidence_score IS NOT NULL;
CREATE INDEX idx_fact_documents_lean_import_batch ON fact_documents_lean(import_batch_id) WHERE import_batch_id IS NOT NULL;

-- Composite indexes for common query patterns
CREATE INDEX idx_fact_documents_lean_type_status ON fact_documents_lean(doc_type_id, document_status_id) WHERE doc_type_id IS NOT NULL;
CREATE INDEX idx_fact_documents_lean_match_decision ON fact_documents_lean(match_method_id, match_decision_id) WHERE match_method_id IS NOT NULL;
CREATE INDEX idx_fact_documents_lean_property_development ON fact_documents_lean(property_type_id, development_type_id) WHERE property_type_id IS NOT NULL;

-- Index on boolean flags for filtering
CREATE INDEX idx_fact_documents_lean_matched ON fact_documents_lean(is_matched);
CREATE INDEX idx_fact_documents_lean_high_confidence ON fact_documents_lean(is_high_confidence);

-- Update trigger to maintain updated_at timestamp
CREATE OR REPLACE FUNCTION update_fact_documents_lean_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tr_fact_documents_lean_updated_at
    BEFORE UPDATE ON fact_documents_lean
    FOR EACH ROW
    EXECUTE FUNCTION update_fact_documents_lean_updated_at();

-- Add table comments
COMMENT ON TABLE fact_documents_lean IS 'Lean fact table for documents with foreign key references to dimension tables';
COMMENT ON COLUMN fact_documents_lean.fact_id IS 'Surrogate key for the fact table';
COMMENT ON COLUMN fact_documents_lean.document_id IS 'Business key from source system';
COMMENT ON COLUMN fact_documents_lean.match_confidence_score IS 'Address matching confidence (0.0000-1.0000)';
COMMENT ON COLUMN fact_documents_lean.address_quality_score IS 'Overall address data quality (0.00-1.00)';
COMMENT ON COLUMN fact_documents_lean.data_completeness_score IS 'Completeness of record data (0.00-1.00)';
COMMENT ON COLUMN fact_documents_lean.is_matched IS 'Computed: TRUE if matched_address_id is not null';
COMMENT ON COLUMN fact_documents_lean.is_high_confidence IS 'Computed: TRUE if match_confidence_score >= 0.85';

SELECT 
    'Fact Table Created' as status,
    'fact_documents_lean' as table_name;

COMMIT;