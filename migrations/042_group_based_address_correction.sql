-- Migration: Group-based address matching correction
-- Description: Implement strategy to use best match within planning groups for consistency

BEGIN;

-- Create function to find the best match within a planning group
CREATE OR REPLACE FUNCTION get_best_group_match(group_base TEXT)
RETURNS TABLE(
    best_uprn TEXT,
    best_address_id INTEGER,
    confidence_score NUMERIC,
    match_method TEXT,
    total_votes INTEGER
) AS $$
BEGIN
    RETURN QUERY
    WITH group_matches AS (
        -- Get all matches for documents in this planning group
        SELECT 
            da.uprn,
            am.address_id,
            da.full_address,
            am.confidence_score,
            mm.method_name,
            COUNT(*) as vote_count,
            AVG(am.confidence_score) as avg_confidence,
            MAX(am.confidence_score) as max_confidence
        FROM src_document s
        JOIN address_match am ON s.document_id = am.document_id
        JOIN dim_address da ON am.address_id = da.address_id
        LEFT JOIN dim_match_method mm ON am.match_method_id = mm.method_id
        WHERE s.planning_app_base = group_base
        GROUP BY da.uprn, am.address_id, da.full_address, am.confidence_score, mm.method_name
    ),
    source_uprn_match AS (
        -- Check if any document in the group has a source UPRN that exists in LLPG
        SELECT 
            s.raw_uprn as uprn,
            da.address_id,
            1.0 as confidence_score,
            'source_uprn' as method_name,
            COUNT(*) as vote_count
        FROM src_document s
        JOIN dim_address da ON s.raw_uprn = da.uprn
        WHERE s.planning_app_base = group_base
          AND s.raw_uprn IS NOT NULL 
          AND s.raw_uprn <> ''
        GROUP BY s.raw_uprn, da.address_id
    ),
    all_candidates AS (
        SELECT uprn, address_id, avg_confidence as confidence_score, method_name, vote_count
        FROM group_matches
        UNION ALL
        SELECT uprn, address_id, confidence_score, method_name, vote_count
        FROM source_uprn_match
    ),
    ranked_candidates AS (
        SELECT 
            uprn,
            address_id,
            confidence_score,
            method_name,
            vote_count,
            -- Priority: 1) Source UPRN, 2) Most votes, 3) Highest confidence
            ROW_NUMBER() OVER (
                ORDER BY 
                    CASE WHEN method_name = 'source_uprn' THEN 1 ELSE 2 END,
                    vote_count DESC,
                    confidence_score DESC
            ) as rank
        FROM all_candidates
    )
    SELECT 
        rc.uprn,
        rc.address_id,
        rc.confidence_score,
        rc.method_name,
        rc.vote_count
    FROM ranked_candidates rc
    WHERE rank = 1;
END;
$$ LANGUAGE plpgsql;

-- Create view to show planning groups with inconsistent matches
CREATE OR REPLACE VIEW vw_inconsistent_planning_groups AS
WITH group_stats AS (
    SELECT 
        s.planning_app_base,
        s.planning_app_group_id,
        COUNT(*) as total_documents,
        COUNT(DISTINCT s.raw_address) FILTER (WHERE s.raw_address <> 'N/A' AND LENGTH(s.raw_address) > 5) as unique_addresses,
        COUNT(am.document_id) as matched_documents,
        COUNT(DISTINCT am.address_id) as unique_matched_addresses,
        COUNT(DISTINCT da.uprn) as unique_matched_uprns,
        
        -- Check for source UPRNs in the group
        COUNT(DISTINCT s.raw_uprn) FILTER (WHERE s.raw_uprn IS NOT NULL AND s.raw_uprn <> '') as source_uprns,
        
        -- Get the most common address (by count)
        (SELECT s2.raw_address
         FROM src_document s2 
         WHERE s2.planning_app_base = s.planning_app_base 
           AND s2.raw_address <> 'N/A' 
           AND LENGTH(s2.raw_address) > 5
         GROUP BY s2.raw_address 
         ORDER BY COUNT(*) DESC, LENGTH(s2.raw_address) DESC
         LIMIT 1) as representative_address
         
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    LEFT JOIN dim_address da ON am.address_id = da.address_id
    WHERE s.planning_app_base IS NOT NULL
    GROUP BY s.planning_app_base, s.planning_app_group_id
),
best_matches AS (
    SELECT 
        gs.planning_app_base,
        bgm.best_uprn,
        bgm.best_address_id,
        bgm.confidence_score as best_confidence,
        bgm.match_method as best_method,
        bgm.total_votes
    FROM group_stats gs
    CROSS JOIN LATERAL get_best_group_match(gs.planning_app_base) bgm
)
SELECT 
    gs.planning_app_base,
    gs.total_documents,
    gs.unique_addresses,
    gs.matched_documents,
    gs.unique_matched_addresses,
    gs.unique_matched_uprns,
    gs.source_uprns,
    gs.representative_address,
    
    bm.best_uprn as recommended_uprn,
    bm.best_address_id as recommended_address_id,
    bm.best_confidence as recommended_confidence,
    bm.best_method as recommended_method,
    bm.total_votes as recommendation_strength,
    
    -- Flag different types of issues
    CASE 
        WHEN gs.unique_matched_uprns > 1 THEN 'Multiple different UPRNs matched'
        WHEN gs.matched_documents > 0 AND gs.matched_documents < gs.total_documents THEN 'Partial matches only'
        WHEN gs.unique_addresses > 1 AND gs.matched_documents = 0 THEN 'Multiple addresses, no matches'
        WHEN gs.matched_documents = 0 THEN 'No matches'
        ELSE 'Consistent'
    END as issue_type

FROM group_stats gs
LEFT JOIN best_matches bm ON gs.planning_app_base = bm.planning_app_base
WHERE gs.unique_matched_uprns > 1  -- Focus on groups with inconsistent matches
   OR (gs.matched_documents > 0 AND gs.matched_documents < gs.total_documents)
   OR (gs.unique_addresses > 1 AND gs.matched_documents = 0)
ORDER BY gs.total_documents DESC, gs.unique_matched_uprns DESC;

-- Create corrected matches table
CREATE TABLE IF NOT EXISTS address_match_corrected (
    document_id INTEGER PRIMARY KEY REFERENCES src_document(document_id),
    original_address_id INTEGER REFERENCES dim_address(address_id),
    original_confidence_score NUMERIC(5,4),
    original_method_id INTEGER REFERENCES dim_match_method(method_id),
    
    corrected_address_id INTEGER REFERENCES dim_address(address_id),
    corrected_location_id INTEGER REFERENCES dim_location(location_id),
    corrected_confidence_score NUMERIC(5,4),
    corrected_method_id INTEGER REFERENCES dim_match_method(method_id),
    
    correction_reason TEXT,
    planning_app_base TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    CONSTRAINT fk_corrected_address_location 
        FOREIGN KEY (corrected_address_id) REFERENCES dim_address(address_id),
    CONSTRAINT fk_corrected_location 
        FOREIGN KEY (corrected_location_id) REFERENCES dim_location(location_id)
);

-- Create index for performance
CREATE INDEX IF NOT EXISTS idx_address_match_corrected_planning ON address_match_corrected(planning_app_base);
CREATE INDEX IF NOT EXISTS idx_address_match_corrected_document ON address_match_corrected(document_id);

COMMIT;