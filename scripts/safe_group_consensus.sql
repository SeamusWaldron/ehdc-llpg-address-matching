-- Safe Group Consensus Correction Script
-- Only applies group consensus when addresses are clearly similar and safe

CREATE OR REPLACE FUNCTION is_real_address(address_text TEXT) 
RETURNS BOOLEAN AS $$
BEGIN
    -- Check if this looks like a real address vs planning reference
    IF address_text IS NULL OR LENGTH(TRIM(address_text)) < 10 THEN
        RETURN FALSE;
    END IF;
    
    -- Planning reference patterns (F12345, AU123, etc.)
    IF address_text ~ '^[A-Z]{1,3}[0-9]+/?[0-9]*$' THEN
        RETURN FALSE;
    END IF;
    
    -- N/A and similar non-addresses
    IF UPPER(address_text) IN ('N/A', 'NOT APPLICABLE', 'NONE', 'NULL', 'TBC') THEN
        RETURN FALSE;
    END IF;
    
    -- Must contain typical address indicators
    IF address_text ~* '(street|road|avenue|lane|way|close|drive|court|place|crescent|gardens|park|hill|view|house|cottage|farm|manor|hall)' 
       OR address_text ~ ',' THEN  -- Has commas (typical of addresses)
        RETURN TRUE;
    END IF;
    
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Safe group consensus correction with strict rules
WITH safe_group_candidates AS (
    SELECT 
        s.planning_app_base,
        COUNT(*) as total_docs,
        COUNT(am.document_id) as matched_docs,
        COUNT(DISTINCT am.address_id) FILTER (WHERE am.address_id IS NOT NULL) as unique_matched_addresses,
        
        -- Count real addresses vs planning refs
        COUNT(*) FILTER (WHERE is_real_address(s.raw_address)) as real_addresses,
        COUNT(*) FILTER (WHERE NOT is_real_address(s.raw_address)) as planning_refs,
        
        -- Address similarity check - are the real addresses similar?
        COUNT(DISTINCT SUBSTRING(s.raw_address, 1, 30)) FILTER (WHERE is_real_address(s.raw_address)) as address_variations,
        
        -- Get the best match in the group (if any)
        (SELECT gbm.uprn FROM get_group_best_match_simple(s.planning_app_base) gbm LIMIT 1) as group_best_uprn,
        (SELECT gbm.address_id FROM get_group_best_match_simple(s.planning_app_base) gbm LIMIT 1) as group_best_address_id,
        (SELECT gbm.votes FROM get_group_best_match_simple(s.planning_app_base) gbm LIMIT 1) as group_consensus_votes
        
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    WHERE s.planning_app_base IS NOT NULL
    GROUP BY s.planning_app_base
),
safe_groups AS (
    SELECT *
    FROM safe_group_candidates
    WHERE total_docs BETWEEN 2 AND 10          -- Reasonable group size
      AND matched_docs > 0                     -- Some already matched
      AND matched_docs < total_docs            -- Some unmatched
      AND unique_matched_addresses = 1         -- All matches to same address
      AND real_addresses >= (total_docs * 0.7) -- At least 70% real addresses
      AND planning_refs <= 2                   -- Max 2 planning refs
      AND address_variations <= 2              -- Max 2 address variations
      AND group_consensus_votes >= 2           -- Need at least 2 votes for consensus
      AND group_best_uprn IS NOT NULL
),
safe_corrections AS (
    SELECT 
        s.document_id,
        sg.planning_app_base,
        am.address_id as original_address_id,
        am.confidence_score as original_confidence,
        am.match_method_id as original_method_id,
        
        sg.group_best_address_id as corrected_address_id,
        da.location_id as corrected_location_id,
        CASE 
            WHEN sg.group_consensus_votes >= 5 THEN 0.95
            WHEN sg.group_consensus_votes >= 3 THEN 0.90
            ELSE 0.85
        END as corrected_confidence,
        30 as corrected_method_id,  -- Group consensus method
        
        'SAFE group consensus - ' || sg.group_consensus_votes || ' votes, ' || 
        sg.real_addresses || '/' || sg.total_docs || ' real addresses' as correction_reason
        
    FROM safe_groups sg
    JOIN src_document s ON s.planning_app_base = sg.planning_app_base
    JOIN dim_address da ON sg.group_best_address_id = da.address_id
    LEFT JOIN address_match am ON s.document_id = am.document_id
    WHERE (am.address_id IS NULL OR am.confidence_score = 0.0)  -- Only unmatched/failed
      AND is_real_address(s.raw_address)  -- Only apply to real addresses
)
SELECT 
    'SAFE GROUP CONSENSUS ANALYSIS' as analysis,
    COUNT(*) as potential_safe_corrections,
    COUNT(DISTINCT planning_app_base) as safe_groups
FROM safe_corrections;

-- Show the safe groups that would be corrected
SELECT 
    planning_app_base,
    total_docs,
    matched_docs,
    real_addresses,
    planning_refs,
    group_consensus_votes,
    'Would correct ' || (total_docs - matched_docs) || ' documents' as correction_impact
FROM safe_groups
ORDER BY group_consensus_votes DESC, total_docs DESC;