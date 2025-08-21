-- Performance optimizations for batch address matching
-- Focus on reducing query overhead and improving fuzzy matching speed

-- 1. Create a materialized view for faster unmatched document queries
CREATE MATERIALIZED VIEW mv_unmatched_documents AS
SELECT 
    s.document_id, 
    s.raw_address, 
    s.address_canonical,
    s.raw_uprn, 
    s.raw_easting, 
    s.raw_northing,
    dt.type_code
FROM src_document s
INNER JOIN dim_document_type dt ON dt.doc_type_id = s.doc_type_id
LEFT JOIN address_match m ON m.document_id = s.document_id
WHERE m.document_id IS NULL
  AND s.raw_address IS NOT NULL
  AND s.raw_address != ''
  AND s.address_canonical IS NOT NULL
  AND s.address_canonical != ''
ORDER BY s.document_id;

-- Index the materialized view
CREATE INDEX idx_mv_unmatched_documents_id ON mv_unmatched_documents(document_id);
CREATE INDEX idx_mv_unmatched_documents_type ON mv_unmatched_documents(type_code);
CREATE INDEX idx_mv_unmatched_documents_canonical ON mv_unmatched_documents USING GIN(address_canonical gin_trgm_ops);

-- 2. Create a function for faster fuzzy matching that combines all tiers
CREATE OR REPLACE FUNCTION fast_address_match(
    input_canonical TEXT,
    input_uprn TEXT DEFAULT NULL,
    limit_results INTEGER DEFAULT 50
) RETURNS TABLE (
    address_id INTEGER,
    location_id INTEGER,
    uprn TEXT,
    full_address TEXT,
    address_canonical TEXT,
    easting NUMERIC,
    northing NUMERIC,
    match_score NUMERIC,
    match_method TEXT
) AS $$
BEGIN
    -- First try exact UPRN match if provided
    IF input_uprn IS NOT NULL AND input_uprn != '' THEN
        RETURN QUERY
        SELECT 
            a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
            l.easting, l.northing, 1.0::NUMERIC as match_score, 'exact_uprn' as match_method
        FROM dim_address a
        INNER JOIN dim_location l ON l.location_id = a.location_id
        WHERE a.uprn = input_uprn;
        
        -- If we found exact UPRN match, return immediately
        IF FOUND THEN
            RETURN;
        END IF;
    END IF;
    
    -- Try exact text match
    RETURN QUERY
    SELECT 
        a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
        l.easting, l.northing, 0.99::NUMERIC as match_score, 'exact_text' as match_method
    FROM dim_address a
    INNER JOIN dim_location l ON l.location_id = a.location_id
    WHERE a.address_canonical = input_canonical
    LIMIT limit_results;
    
    -- If we have exact matches, return
    IF FOUND THEN
        RETURN;
    END IF;
    
    -- Single combined fuzzy query with multiple thresholds
    RETURN QUERY
    SELECT 
        a.address_id, a.location_id, a.uprn, a.full_address, a.address_canonical,
        l.easting, l.northing, 
        similarity(input_canonical, a.address_canonical) as match_score,
        CASE 
            WHEN similarity(input_canonical, a.address_canonical) >= 0.90 THEN 'fuzzy_high'
            WHEN similarity(input_canonical, a.address_canonical) >= 0.80 THEN 'fuzzy_medium'
            ELSE 'fuzzy_low'
        END as match_method
    FROM dim_address a
    INNER JOIN dim_location l ON l.location_id = a.location_id
    WHERE a.address_canonical % input_canonical
      AND similarity(input_canonical, a.address_canonical) >= 0.70
    ORDER BY similarity(input_canonical, a.address_canonical) DESC
    LIMIT limit_results;
    
END;
$$ LANGUAGE plpgsql;

-- 3. Optimize PostgreSQL settings at database level
ALTER DATABASE ehdc_llpg SET work_mem = '256MB';
ALTER DATABASE ehdc_llpg SET pg_trgm.similarity_threshold = '0.6';
ALTER DATABASE ehdc_llpg SET random_page_cost = '1.1';  -- For SSD storage
ALTER DATABASE ehdc_llpg SET effective_io_concurrency = '200';  -- For SSD

-- 4. Create summary statistics table for faster reporting
CREATE TABLE IF NOT EXISTS match_statistics (
    stat_date DATE DEFAULT CURRENT_DATE,
    total_documents INTEGER,
    matched_documents INTEGER,
    unmatched_documents INTEGER,
    match_rate NUMERIC(5,2),
    avg_confidence NUMERIC(5,4),
    last_updated TIMESTAMPTZ DEFAULT now()
);

-- Update statistics
REFRESH MATERIALIZED VIEW mv_unmatched_documents;
ANALYZE mv_unmatched_documents;

-- Insert current statistics
INSERT INTO match_statistics (
    total_documents, matched_documents, unmatched_documents, match_rate, avg_confidence
)
SELECT 
    (SELECT COUNT(*) FROM src_document) as total_documents,
    (SELECT COUNT(*) FROM address_match) as matched_documents,
    (SELECT COUNT(*) FROM mv_unmatched_documents) as unmatched_documents,
    (SELECT COUNT(*)::NUMERIC / (SELECT COUNT(*) FROM src_document) * 100 FROM address_match) as match_rate,
    (SELECT COALESCE(AVG(confidence_score), 0) FROM address_match) as avg_confidence;