-- Schema for storing gopostal-parsed address components
-- This allows us to pre-process all addresses ONCE and match on standardized components

-- ============================================================================
-- 1. ADD GOPOSTAL COMPONENTS TO ADDRESS TABLES
-- ============================================================================

-- Add gopostal component columns to dim_address (LLPG addresses)
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_processed BOOLEAN DEFAULT FALSE;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_house TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_house_number TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_road TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_suburb TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_city TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_state_district TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_state TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_postcode TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_country TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_unit TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_level TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_staircase TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_entrance TEXT;
ALTER TABLE dim_address ADD COLUMN IF NOT EXISTS gopostal_po_box TEXT;

-- Add gopostal component columns to src_document (source addresses)
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_processed BOOLEAN DEFAULT FALSE;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_house TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_house_number TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_road TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_suburb TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_city TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_state_district TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_state TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_postcode TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_country TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_unit TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_level TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_staircase TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_entrance TEXT;
ALTER TABLE src_document ADD COLUMN IF NOT EXISTS gopostal_po_box TEXT;

-- ============================================================================
-- 2. CREATE INDEXES FOR COMPONENT MATCHING
-- ============================================================================

-- Indexes for LLPG address components
CREATE INDEX IF NOT EXISTS idx_dim_address_gopostal_house_number ON dim_address(gopostal_house_number);
CREATE INDEX IF NOT EXISTS idx_dim_address_gopostal_road ON dim_address(gopostal_road);
CREATE INDEX IF NOT EXISTS idx_dim_address_gopostal_city ON dim_address(gopostal_city);
CREATE INDEX IF NOT EXISTS idx_dim_address_gopostal_postcode ON dim_address(gopostal_postcode);
CREATE INDEX IF NOT EXISTS idx_dim_address_gopostal_processed ON dim_address(gopostal_processed);

-- Trigram indexes for fuzzy component matching
CREATE INDEX IF NOT EXISTS idx_dim_address_gopostal_road_trgm ON dim_address USING GIN(gopostal_road gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_dim_address_gopostal_house_trgm ON dim_address USING GIN(gopostal_house gin_trgm_ops);

-- Indexes for source document components
CREATE INDEX IF NOT EXISTS idx_src_document_gopostal_house_number ON src_document(gopostal_house_number);
CREATE INDEX IF NOT EXISTS idx_src_document_gopostal_road ON src_document(gopostal_road);
CREATE INDEX IF NOT EXISTS idx_src_document_gopostal_city ON src_document(gopostal_city);
CREATE INDEX IF NOT EXISTS idx_src_document_gopostal_postcode ON src_document(gopostal_postcode);
CREATE INDEX IF NOT EXISTS idx_src_document_gopostal_processed ON src_document(gopostal_processed);

-- ============================================================================
-- 3. CREATE OPTIMIZED MATCHING FUNCTION USING COMPONENTS
-- ============================================================================

CREATE OR REPLACE FUNCTION match_gopostal_components(
    src_house_number TEXT,
    src_road TEXT,
    src_city TEXT,
    src_postcode TEXT,
    src_house TEXT,
    src_unit TEXT,
    limit_results INTEGER DEFAULT 50
) RETURNS TABLE (
    address_id INTEGER,
    location_id INTEGER,
    uprn TEXT,
    full_address TEXT,
    match_score REAL,
    match_reason TEXT
) AS $$
DECLARE
    has_postcode BOOLEAN := src_postcode IS NOT NULL AND src_postcode != '';
    has_house_number BOOLEAN := src_house_number IS NOT NULL AND src_house_number != '';
    has_road BOOLEAN := src_road IS NOT NULL AND src_road != '';
    has_city BOOLEAN := src_city IS NOT NULL AND src_city != '';
BEGIN
    -- Strategy 1: Exact component match (highest confidence)
    IF has_postcode AND has_house_number THEN
        RETURN QUERY
        SELECT 
            a.address_id, a.location_id, a.uprn, a.full_address,
            1.0::REAL as match_score,
            'Exact: postcode + house number' as match_reason
        FROM dim_address a
        WHERE a.gopostal_postcode = src_postcode
          AND a.gopostal_house_number = src_house_number
        LIMIT limit_results;
        
        IF FOUND THEN RETURN; END IF;
    END IF;
    
    -- Strategy 2: Road + House Number + City (very high confidence)
    IF has_road AND has_house_number AND has_city THEN
        RETURN QUERY
        SELECT 
            a.address_id, a.location_id, a.uprn, a.full_address,
            0.95::REAL as match_score,
            'Road + house number + city' as match_reason
        FROM dim_address a
        WHERE a.gopostal_road = src_road
          AND a.gopostal_house_number = src_house_number
          AND a.gopostal_city = src_city
        LIMIT limit_results;
        
        IF FOUND THEN RETURN; END IF;
    END IF;
    
    -- Strategy 3: Fuzzy road match with city (high confidence)
    IF has_road AND has_city THEN
        RETURN QUERY
        SELECT 
            a.address_id, a.location_id, a.uprn, a.full_address,
            similarity(src_road, a.gopostal_road) * 0.9 as match_score,
            'Fuzzy road + city' as match_reason
        FROM dim_address a
        WHERE a.gopostal_city = src_city
          AND a.gopostal_road % src_road
          AND similarity(src_road, a.gopostal_road) >= 0.7
        ORDER BY similarity(src_road, a.gopostal_road) DESC
        LIMIT limit_results;
        
        IF FOUND THEN RETURN; END IF;
    END IF;
    
    -- Strategy 4: House/Business name match (medium confidence)
    IF src_house IS NOT NULL AND src_house != '' THEN
        RETURN QUERY
        SELECT 
            a.address_id, a.location_id, a.uprn, a.full_address,
            similarity(src_house, a.gopostal_house) * 0.85 as match_score,
            'House/business name' as match_reason
        FROM dim_address a
        WHERE a.gopostal_house % src_house
          AND similarity(src_house, a.gopostal_house) >= 0.6
        ORDER BY similarity(src_house, a.gopostal_house) DESC
        LIMIT limit_results;
    END IF;
    
    -- Strategy 5: Postcode-only match (low confidence, many results)
    IF has_postcode THEN
        RETURN QUERY
        SELECT 
            a.address_id, a.location_id, a.uprn, a.full_address,
            0.5::REAL as match_score,
            'Postcode only' as match_reason
        FROM dim_address a
        WHERE a.gopostal_postcode = src_postcode
        LIMIT limit_results;
    END IF;
    
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- 4. CREATE BATCH UPDATE FUNCTIONS
-- ============================================================================

-- Function to simulate gopostal parsing (placeholder until real gopostal is used)
-- This will be replaced by actual gopostal processing
CREATE OR REPLACE FUNCTION simulate_gopostal_parse(input_address TEXT)
RETURNS TABLE (
    house TEXT,
    house_number TEXT,
    road TEXT,
    city TEXT,
    postcode TEXT
) AS $$
BEGIN
    -- This is a simplified parser - will be replaced by gopostal
    -- Extract postcode
    postcode := substring(input_address from '[A-Z]{1,2}[0-9]{1,2}[A-Z]?\s*[0-9][A-Z]{2}');
    
    -- Extract house number
    house_number := substring(input_address from '^\d+[A-Za-z]?');
    
    -- Extract common city names (Hampshire specific)
    IF input_address ~* 'ALTON' THEN city := 'ALTON';
    ELSIF input_address ~* 'PETERSFIELD' THEN city := 'PETERSFIELD';
    ELSIF input_address ~* 'BORDON' THEN city := 'BORDON';
    ELSIF input_address ~* 'LIPHOOK' THEN city := 'LIPHOOK';
    ELSIF input_address ~* 'LISS' THEN city := 'LISS';
    END IF;
    
    -- Extract road (simplified)
    road := regexp_replace(input_address, '^\d+[A-Za-z]?\s+', ''); -- Remove house number
    road := regexp_replace(road, '\s+(ALTON|PETERSFIELD|BORDON|LIPHOOK|LISS).*$', '', 'i'); -- Remove city onwards
    road := trim(road);
    
    RETURN QUERY SELECT simulate_gopostal_parse.house, 
                        simulate_gopostal_parse.house_number, 
                        simulate_gopostal_parse.road, 
                        simulate_gopostal_parse.city, 
                        simulate_gopostal_parse.postcode;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- 5. STATISTICS TRACKING
-- ============================================================================

CREATE TABLE IF NOT EXISTS gopostal_processing_stats (
    id SERIAL PRIMARY KEY,
    table_name TEXT,
    total_records INTEGER,
    processed_records INTEGER,
    processing_date TIMESTAMPTZ DEFAULT now(),
    processing_time INTERVAL,
    notes TEXT
);

-- ============================================================================
-- 6. SAMPLE UPDATE QUERIES (commented out - run manually)
-- ============================================================================

-- Update a sample of LLPG addresses with simulated parsing
-- UPDATE dim_address a
-- SET (gopostal_house_number, gopostal_road, gopostal_city, gopostal_postcode, gopostal_processed) = 
--     (p.house_number, p.road, p.city, p.postcode, TRUE)
-- FROM simulate_gopostal_parse(a.full_address) p
-- WHERE a.address_id <= 100;

-- Update a sample of source documents with simulated parsing  
-- UPDATE src_document s
-- SET (gopostal_house_number, gopostal_road, gopostal_city, gopostal_postcode, gopostal_processed) = 
--     (p.house_number, p.road, p.city, p.postcode, TRUE)
-- FROM simulate_gopostal_parse(s.raw_address) p
-- WHERE s.document_id <= 100;