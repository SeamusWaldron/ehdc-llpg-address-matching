-- Migration: Add planning application grouping fields and address similarity matching
-- Description: Split planning app numbers and implement group-based address matching

BEGIN;

-- Add planning app grouping columns to src_document
ALTER TABLE src_document 
ADD COLUMN IF NOT EXISTS planning_app_base TEXT,
ADD COLUMN IF NOT EXISTS planning_app_sequence TEXT,
ADD COLUMN IF NOT EXISTS planning_app_group_id INTEGER;

-- Function to split planning application numbers
CREATE OR REPLACE FUNCTION split_planning_app_number(app_no TEXT)
RETURNS TABLE(base_app TEXT, sequence TEXT) AS $$
BEGIN
    -- Handle null/empty cases
    IF app_no IS NULL OR LENGTH(TRIM(app_no)) = 0 THEN
        RETURN QUERY SELECT app_no, NULL::TEXT;
        RETURN;
    END IF;
    
    -- Check if it contains a slash (like 20003/001)
    IF position('/' IN app_no) > 0 THEN
        RETURN QUERY SELECT 
            TRIM(split_part(app_no, '/', 1)) as base_app,
            TRIM(split_part(app_no, '/', 2)) as sequence;
    ELSE
        -- No slash, it's a base application
        RETURN QUERY SELECT TRIM(app_no), NULL::TEXT;
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Update the planning app splits
UPDATE src_document 
SET (planning_app_base, planning_app_sequence) = (
    SELECT base_app, sequence 
    FROM split_planning_app_number(external_reference)
);

-- Create planning app group IDs (sequential numbering for each base app)
WITH base_apps AS (
    SELECT DISTINCT planning_app_base,
           ROW_NUMBER() OVER (ORDER BY planning_app_base) as group_id
    FROM src_document
    WHERE planning_app_base IS NOT NULL
)
UPDATE src_document 
SET planning_app_group_id = ba.group_id
FROM base_apps ba
WHERE src_document.planning_app_base = ba.planning_app_base;

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_src_document_planning_base ON src_document(planning_app_base);
CREATE INDEX IF NOT EXISTS idx_src_document_planning_group ON src_document(planning_app_group_id);
CREATE INDEX IF NOT EXISTS idx_src_document_planning_base_seq ON src_document(planning_app_base, planning_app_sequence);

-- Create view for planning application groups with address analysis
CREATE OR REPLACE VIEW vw_planning_app_groups AS
SELECT 
    planning_app_base,
    planning_app_group_id,
    COUNT(*) as total_documents,
    COUNT(DISTINCT raw_address) as unique_addresses,
    COUNT(DISTINCT CASE WHEN raw_address <> 'N/A' AND LENGTH(raw_address) > 5 THEN raw_address END) as valid_addresses,
    
    -- Address matching statistics
    COUNT(am.document_id) as matched_documents,
    COUNT(DISTINCT am.address_id) as unique_matched_addresses,
    COUNT(DISTINCT s.raw_uprn) FILTER (WHERE s.raw_uprn IS NOT NULL AND s.raw_uprn <> '') as source_uprns,
    
    -- Most common address in the group
    (SELECT raw_address 
     FROM src_document s2 
     WHERE s2.planning_app_base = s.planning_app_base 
       AND s2.raw_address <> 'N/A' 
       AND LENGTH(s2.raw_address) > 5
     GROUP BY raw_address 
     ORDER BY COUNT(*) DESC 
     LIMIT 1) as most_common_address,
     
    -- Check if group has source UPRN
    (SELECT s3.raw_uprn 
     FROM src_document s3 
     WHERE s3.planning_app_base = s.planning_app_base 
       AND s3.raw_uprn IS NOT NULL 
       AND s3.raw_uprn <> ''
     LIMIT 1) as group_source_uprn,
     
    -- Check if group has successful match
    (SELECT da.uprn 
     FROM src_document s4
     JOIN address_match am2 ON s4.document_id = am2.document_id
     JOIN dim_address da ON am2.address_id = da.address_id
     WHERE s4.planning_app_base = s.planning_app_base 
       AND am2.confidence_score >= 0.8
     ORDER BY am2.confidence_score DESC
     LIMIT 1) as best_matched_uprn

FROM src_document s
LEFT JOIN address_match am ON s.document_id = am.document_id
WHERE planning_app_base IS NOT NULL
GROUP BY planning_app_base, planning_app_group_id
ORDER BY planning_app_base;

-- Create view for problematic planning groups (same group, different matches)
CREATE OR REPLACE VIEW vw_planning_groups_inconsistent_matches AS
SELECT 
    pag.planning_app_base,
    pag.total_documents,
    pag.unique_addresses,
    pag.matched_documents,
    pag.unique_matched_addresses,
    pag.most_common_address,
    pag.group_source_uprn,
    pag.best_matched_uprn,
    
    -- List all different matched UPRNs in this group
    array_agg(DISTINCT da.uprn ORDER BY da.uprn) FILTER (WHERE da.uprn IS NOT NULL) as all_matched_uprns,
    
    -- Check if there are inconsistencies
    CASE 
        WHEN pag.unique_matched_addresses > 1 THEN 'Multiple different matches'
        WHEN pag.matched_documents > 0 AND pag.matched_documents < pag.total_documents THEN 'Partial matches'
        WHEN pag.unique_addresses > 1 AND pag.matched_documents = 0 THEN 'No matches despite address variations'
        ELSE 'Consistent'
    END as issue_type

FROM vw_planning_app_groups pag
LEFT JOIN src_document s ON s.planning_app_base = pag.planning_app_base
LEFT JOIN address_match am ON s.document_id = am.document_id
LEFT JOIN dim_address da ON am.address_id = da.address_id
WHERE pag.planning_app_base IS NOT NULL
GROUP BY pag.planning_app_base, pag.total_documents, pag.unique_addresses, pag.matched_documents, 
         pag.unique_matched_addresses, pag.most_common_address, pag.group_source_uprn, pag.best_matched_uprn
HAVING COUNT(DISTINCT da.uprn) > 1  -- Only show groups with inconsistent matches
   OR (COUNT(DISTINCT s.raw_address) > 1 AND COUNT(am.document_id) = 0)  -- Multiple addresses but no matches
   OR (COUNT(am.document_id) > 0 AND COUNT(am.document_id) < COUNT(DISTINCT s.document_id))  -- Partial matches
ORDER BY pag.total_documents DESC;

COMMIT;