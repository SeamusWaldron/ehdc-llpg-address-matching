-- EHDC LLPG Range Expansion Script
-- Expands range addresses (e.g., "10-11") into individual addresses for better matching

-- Create a table for expanded/implied addresses
CREATE TABLE IF NOT EXISTS dim_address_expanded (
    expanded_id SERIAL PRIMARY KEY,
    original_address_id INTEGER REFERENCES dim_address(address_id),
    uprn TEXT,  -- Changed to TEXT to match dim_address
    full_address TEXT,
    address_canonical TEXT,
    expansion_type TEXT, -- 'range_expansion', 'original'
    unit_number TEXT,    -- The specific unit number extracted
    created_at TIMESTAMP DEFAULT NOW()
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_address_expanded_uprn ON dim_address_expanded(uprn);
CREATE INDEX IF NOT EXISTS idx_address_expanded_canonical ON dim_address_expanded(address_canonical);
CREATE INDEX IF NOT EXISTS idx_address_expanded_unit ON dim_address_expanded(unit_number);

-- Function to expand numeric ranges
CREATE OR REPLACE FUNCTION expand_address_ranges() RETURNS INTEGER AS $$
DECLARE
    rec RECORD;
    range_match TEXT[];
    start_num INTEGER;
    end_num INTEGER;
    i INTEGER;
    new_address TEXT;
    new_canonical TEXT;
    expanded_count INTEGER := 0;
BEGIN
    -- Clear previous expansions
    DELETE FROM dim_address_expanded WHERE expansion_type = 'range_expansion';
    
    -- First, copy all original addresses
    INSERT INTO dim_address_expanded (original_address_id, uprn, full_address, address_canonical, expansion_type, unit_number)
    SELECT address_id, uprn, full_address, address_canonical, 'original', NULL
    FROM dim_address;
    
    -- Process addresses with numeric ranges
    FOR rec IN 
        SELECT address_id, uprn, full_address, address_canonical
        FROM dim_address 
        WHERE full_address ~ '\m\d+-\d+\M'  -- Matches patterns like "10-11", "3-4"
    LOOP
        -- Extract the range pattern (e.g., "10-11")
        range_match := regexp_match(rec.full_address, '(\d+)-(\d+)');
        
        IF range_match IS NOT NULL THEN
            start_num := range_match[1]::INTEGER;
            end_num := range_match[2]::INTEGER;
            
            -- Generate individual addresses for each number in the range
            FOR i IN start_num..end_num LOOP
                -- Replace the range with the individual number
                new_address := regexp_replace(rec.full_address, '\m' || start_num || '-' || end_num || '\M', i::TEXT);
                new_canonical := regexp_replace(rec.address_canonical, '\m' || start_num || '-' || end_num || '\M', i::TEXT);
                
                -- Insert the expanded address
                INSERT INTO dim_address_expanded (
                    original_address_id, uprn, full_address, address_canonical, 
                    expansion_type, unit_number
                ) VALUES (
                    rec.address_id, rec.uprn, new_address, new_canonical,
                    'range_expansion', i::TEXT
                );
                
                expanded_count := expanded_count + 1;
            END LOOP;
        END IF;
    END LOOP;
    
    -- Handle Unit ranges specifically (e.g., "Unit, 10-11")
    FOR rec IN 
        SELECT address_id, uprn, full_address, address_canonical
        FROM dim_address 
        WHERE full_address ~ 'Unit[,\s]+\d+-\d+'
    LOOP
        -- Extract the unit range
        range_match := regexp_match(rec.full_address, 'Unit[,\s]+(\d+)-(\d+)');
        
        IF range_match IS NOT NULL THEN
            start_num := range_match[1]::INTEGER;
            end_num := range_match[2]::INTEGER;
            
            FOR i IN start_num..end_num LOOP
                -- Replace "Unit, 10-11" with "Unit, 10" etc.
                new_address := regexp_replace(rec.full_address, 'Unit[,\s]+\d+-\d+', 'Unit, ' || i);
                new_canonical := regexp_replace(rec.address_canonical, 'UNIT\s*\d+\s*\d+', 'UNIT ' || i);
                
                INSERT INTO dim_address_expanded (
                    original_address_id, uprn, full_address, address_canonical, 
                    expansion_type, unit_number
                ) VALUES (
                    rec.address_id, rec.uprn, new_address, new_canonical,
                    'range_expansion', i::TEXT
                );
                
                expanded_count := expanded_count + 1;
            END LOOP;
        END IF;
    END LOOP;
    
    -- Handle alpha ranges (e.g., "9A-9C")
    FOR rec IN 
        SELECT address_id, uprn, full_address, address_canonical
        FROM dim_address 
        WHERE full_address ~ '\m\d+[A-Z]-\d+[A-Z]\M'
    LOOP
        -- Extract the alpha range pattern
        range_match := regexp_match(rec.full_address, '(\d+)([A-Z])-(\d+)([A-Z])');
        
        IF range_match IS NOT NULL AND range_match[1] = range_match[3] THEN
            -- Same number, different letters (e.g., 9A-9C)
            FOR i IN ASCII(range_match[2])..ASCII(range_match[4]) LOOP
                new_address := regexp_replace(
                    rec.full_address, 
                    '\m' || range_match[1] || range_match[2] || '-' || range_match[3] || range_match[4] || '\M',
                    range_match[1] || CHR(i)
                );
                new_canonical := regexp_replace(
                    rec.address_canonical,
                    '\m' || range_match[1] || range_match[2] || '-' || range_match[3] || range_match[4] || '\M',
                    range_match[1] || CHR(i)
                );
                
                INSERT INTO dim_address_expanded (
                    original_address_id, uprn, full_address, address_canonical, 
                    expansion_type, unit_number
                ) VALUES (
                    rec.address_id, rec.uprn, new_address, new_canonical,
                    'range_expansion', range_match[1] || CHR(i)
                );
                
                expanded_count := expanded_count + 1;
            END LOOP;
        END IF;
    END LOOP;
    
    RETURN expanded_count;
END;
$$ LANGUAGE plpgsql;

-- Execute the expansion
SELECT expand_address_ranges() as addresses_expanded;

-- Show summary
SELECT 
    expansion_type,
    COUNT(*) as count
FROM dim_address_expanded
GROUP BY expansion_type
ORDER BY expansion_type;

-- Show some examples of expanded addresses
SELECT 
    'Original' as type,
    full_address
FROM dim_address
WHERE full_address LIKE '%Unit%10-11%Amey%'
UNION ALL
SELECT 
    'Expanded' as type,
    full_address
FROM dim_address_expanded
WHERE original_address_id IN (
    SELECT address_id FROM dim_address WHERE full_address LIKE '%Unit%10-11%Amey%'
)
AND expansion_type = 'range_expansion'
ORDER BY type, full_address;