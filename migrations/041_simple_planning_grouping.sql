-- Simple planning app grouping without complex views
BEGIN;

-- Add planning app grouping columns to src_document (if not already added)
DO $$ 
BEGIN
    -- Add columns if they don't exist
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'src_document' AND column_name = 'planning_app_base') THEN
        ALTER TABLE src_document ADD COLUMN planning_app_base TEXT;
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'src_document' AND column_name = 'planning_app_sequence') THEN
        ALTER TABLE src_document ADD COLUMN planning_app_sequence TEXT;
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'src_document' AND column_name = 'planning_app_group_id') THEN
        ALTER TABLE src_document ADD COLUMN planning_app_group_id INTEGER;
    END IF;
END $$;

-- Simple update using string functions
UPDATE src_document 
SET 
    planning_app_base = CASE 
        WHEN external_reference LIKE '%/%' THEN split_part(external_reference, '/', 1)
        ELSE external_reference
    END,
    planning_app_sequence = CASE 
        WHEN external_reference LIKE '%/%' THEN split_part(external_reference, '/', 2)
        ELSE NULL
    END
WHERE external_reference IS NOT NULL;

-- Create group IDs
WITH base_groups AS (
    SELECT DISTINCT planning_app_base,
           DENSE_RANK() OVER (ORDER BY planning_app_base) as group_id
    FROM src_document 
    WHERE planning_app_base IS NOT NULL
)
UPDATE src_document s
SET planning_app_group_id = bg.group_id
FROM base_groups bg
WHERE s.planning_app_base = bg.planning_app_base;

-- Create basic indexes
CREATE INDEX IF NOT EXISTS idx_src_document_planning_base ON src_document(planning_app_base);
CREATE INDEX IF NOT EXISTS idx_src_document_planning_group ON src_document(planning_app_group_id);

COMMIT;