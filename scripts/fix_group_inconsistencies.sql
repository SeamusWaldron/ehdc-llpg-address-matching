-- Script to identify and fix planning application group inconsistencies
-- Run this to correct addresses within planning application groups

-- Create temporary view to find all problematic groups
CREATE TEMPORARY VIEW problematic_groups AS
WITH group_analysis AS (
    SELECT 
        s.planning_app_base,
        COUNT(*) as total_docs,
        COUNT(am.document_id) as matched_docs,
        COUNT(DISTINCT am.address_id) as unique_matches,
        COUNT(DISTINCT da.uprn) as unique_uprns,
        
        -- Check if group has source UPRN
        COUNT(DISTINCT s.raw_uprn) FILTER (WHERE s.raw_uprn IS NOT NULL AND s.raw_uprn <> '') as source_uprns,
        (SELECT s2.raw_uprn 
         FROM src_document s2 
         WHERE s2.planning_app_base = s.planning_app_base 
           AND s2.raw_uprn IS NOT NULL 
           AND s2.raw_uprn <> ''
         LIMIT 1) as group_source_uprn
         
    FROM src_document s
    LEFT JOIN address_match am ON s.document_id = am.document_id
    LEFT JOIN dim_address da ON am.address_id = da.address_id
    WHERE s.planning_app_base IS NOT NULL
    GROUP BY s.planning_app_base
)
SELECT *
FROM group_analysis
WHERE unique_uprns > 1  -- Multiple different UPRNs matched in same group
   OR (matched_docs > 0 AND matched_docs < total_docs AND total_docs > 1)  -- Partial matches
ORDER BY total_docs DESC, unique_uprns DESC;

-- Show summary of problematic groups
SELECT 
    'PROBLEMATIC GROUPS FOUND' as status,
    COUNT(*) as total_problematic_groups,
    SUM(total_docs) as total_affected_documents,
    SUM(CASE WHEN unique_uprns > 1 THEN 1 ELSE 0 END) as groups_with_multiple_uprns,
    SUM(CASE WHEN matched_docs < total_docs THEN 1 ELSE 0 END) as groups_with_partial_matches
FROM problematic_groups;

-- Show top 10 most problematic groups
SELECT 
    planning_app_base,
    total_docs as documents,
    matched_docs as matched,
    unique_matches as different_addresses,
    unique_uprns as different_uprns,
    source_uprns,
    group_source_uprn,
    CASE 
        WHEN unique_uprns > 1 THEN 'Multiple UPRNs'
        WHEN matched_docs < total_docs THEN 'Partial matches'
        ELSE 'Other'
    END as issue_type
FROM problematic_groups
ORDER BY total_docs DESC, unique_uprns DESC
LIMIT 10;

-- Create corrections for groups with multiple UPRNs (use group consensus)
BEGIN;

-- First, handle groups with source UPRNs - they get highest priority
WITH source_uprn_corrections AS (
    SELECT 
        s.document_id,
        am.address_id as original_address_id,
        am.confidence_score as original_confidence,
        am.match_method_id as original_method_id,
        
        -- Match to the source UPRN address
        da.address_id as corrected_address_id,
        da.location_id as corrected_location_id,
        1.0 as corrected_confidence,
        1 as corrected_method_id, -- exact_uprn method
        
        'Source UPRN priority correction' as correction_reason,
        s.planning_app_base
        
    FROM problematic_groups pg
    JOIN src_document s ON s.planning_app_base = pg.planning_app_base
    JOIN address_match am ON s.document_id = am.document_id
    JOIN dim_address da ON da.uprn = pg.group_source_uprn
    WHERE pg.group_source_uprn IS NOT NULL
      AND am.address_id <> da.address_id  -- Only correct mismatched ones
)
INSERT INTO address_match_corrected (
    document_id, original_address_id, original_confidence_score, original_method_id,
    corrected_address_id, corrected_location_id, corrected_confidence_score, corrected_method_id,
    correction_reason, planning_app_base
)
SELECT * FROM source_uprn_corrections
ON CONFLICT (document_id) DO NOTHING;  -- Don't overwrite existing corrections

-- Second, handle groups without source UPRNs - use group consensus
WITH consensus_corrections AS (
    SELECT 
        s.document_id,
        am.address_id as original_address_id,
        am.confidence_score as original_confidence,
        am.match_method_id as original_method_id,
        
        -- Get the best match for the group
        gbm.address_id as corrected_address_id,
        da.location_id as corrected_location_id,
        GREATEST(0.85, gbm.votes::numeric / pg.matched_docs) as corrected_confidence,
        am.match_method_id as corrected_method_id,
        
        'Group consensus correction (' || gbm.votes || '/' || pg.matched_docs || ' votes)' as correction_reason,
        s.planning_app_base
        
    FROM problematic_groups pg
    JOIN src_document s ON s.planning_app_base = pg.planning_app_base
    JOIN address_match am ON s.document_id = am.document_id
    CROSS JOIN get_group_best_match_simple(s.planning_app_base) gbm
    JOIN dim_address da ON gbm.address_id = da.address_id
    WHERE pg.group_source_uprn IS NULL  -- Only groups without source UPRN
      AND pg.unique_uprns > 1  -- Only groups with multiple UPRNs
      AND am.address_id <> gbm.address_id  -- Only correct mismatched ones
      AND gbm.votes >= 2  -- Require at least 2 votes for consensus
)
INSERT INTO address_match_corrected (
    document_id, original_address_id, original_confidence_score, original_method_id,
    corrected_address_id, corrected_location_id, corrected_confidence_score, corrected_method_id,
    correction_reason, planning_app_base
)
SELECT * FROM consensus_corrections
ON CONFLICT (document_id) DO NOTHING;

COMMIT;

-- Show results
SELECT 
    'CORRECTIONS APPLIED' as status,
    COUNT(*) as total_corrections,
    COUNT(DISTINCT planning_app_base) as groups_corrected,
    COUNT(*) FILTER (WHERE correction_reason LIKE 'Source UPRN%') as source_uprn_corrections,
    COUNT(*) FILTER (WHERE correction_reason LIKE 'Group consensus%') as consensus_corrections
FROM address_match_corrected;

-- Show sample corrections
SELECT 
    planning_app_base,
    COUNT(*) as corrections_applied,
    STRING_AGG(DISTINCT correction_reason, '; ') as correction_types
FROM address_match_corrected
GROUP BY planning_app_base
ORDER BY COUNT(*) DESC
LIMIT 10;