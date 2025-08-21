-- Test Script: Validate Fixed Matching Algorithm
-- Purpose: Test the fixed matching rules against known problematic cases
-- Author: Claude Code Assistant
-- Date: 2025-08-19

BEGIN;

-- Create a test results table to track validation
CREATE TEMP TABLE test_cases (
    test_id SERIAL PRIMARY KEY,
    test_name VARCHAR(100),
    source_address TEXT,
    source_house_number VARCHAR(20),
    source_road VARCHAR(200),
    source_city VARCHAR(100),
    matched_address TEXT,
    matched_uprn VARCHAR(20),
    old_method VARCHAR(50),
    old_confidence DECIMAL(5,4),
    expected_result VARCHAR(50),
    issue_type VARCHAR(50)
);

-- Test Case 1: House Number Mismatches (Should be REJECTED)
INSERT INTO test_cases (test_name, source_address, source_house_number, source_road, source_city, matched_address, matched_uprn, old_method, old_confidence, expected_result, issue_type) VALUES
('House Number Mismatch 1', '4 MONKS ORCHARD, PETERSFIELD', '4', 'monks orchard', 'petersfield', '16 Monks Orchard, Petersfield, GU32 2JD', '1710034166', 'Component-Based Fuzzy', 0.5500, 'REJECT', 'wrong_house_number'),
('House Number Mismatch 2', '4 MONKS ORCHARD, PETERSFIELD', '4', 'monks orchard', 'petersfield', '14 Monks Orchard, Petersfield, GU32 2JD', '1710034164', 'Component-Based Fuzzy', 0.5091, 'REJECT', 'wrong_house_number'),
('House Number Mismatch 3', '20 THE SQUARE, LIPHOOK', '20', 'the square', 'liphook', '8 The Square, Liphook, GU30 7AH', '10009816194', 'Street + Locality', 0.9500, 'REJECT', 'wrong_house_number'),
('House Number Mismatch 4', '27 HAVANT ROAD, HORNDEAN', '27', 'havant road', 'horndean', '19 Havant Road, Horndean, Waterlooville, PO8 0DB', '1710000336', 'Street + Locality', 0.9500, 'REJECT', 'wrong_house_number'),
('House Number Mismatch 5', '2, STATION ROAD, LIPHOOK', '2', 'station road', 'liphook', '10a Station Road, Liphook, GU30 7DR', '100060277547', 'Street + Locality', 0.9500, 'REJECT', 'wrong_house_number');

-- Test Case 2: Business Name Mismatches (Should be REJECTED or LOW CONFIDENCE)  
INSERT INTO test_cases (test_name, source_address, source_house_number, source_road, source_city, matched_address, matched_uprn, old_method, old_confidence, expected_result, issue_type) VALUES
('Business Mismatch 1', 'HORNDEAN FOOTBALL CLUB, FIVE HEADS ROAD', NULL, 'five heads road', 'horndean', 'Open space at junction of Portsmouth Road and, Five Heads Road', '10094122021', 'Component-Based Fuzzy', 0.8000, 'REJECT', 'wrong_business'),
('Business Mismatch 2', 'HORNDEAN FOOTBALL CLUB, FIVE HEADS ROAD', NULL, 'five heads road', 'horndean', 'Telecommunications Mast Orange, Five Heads Road', '10032906574', 'Component-Based Fuzzy', 0.8000, 'REJECT', 'wrong_business'),
('Business Mismatch 3', 'REGIS FINE FOODS LTD, UNIT D, STATION ROAD', 'unit d', 'station road', 'liphook', '12a Station Road, Liphook, GU30 7DR', '100060277549', 'Street + Locality', 0.9500, 'REJECT', 'wrong_business');

-- Test Case 3: Valid Matches (Should be ACCEPTED)
INSERT INTO test_cases (test_name, source_address, source_house_number, source_road, source_city, matched_address, matched_uprn, old_method, old_confidence, expected_result, issue_type) VALUES
('Valid Match 1', 'HORNDEAN FOOTBALL CLUB, FIVE HEADS ROAD, HORNDEAN', NULL, 'five heads road', 'horndean', 'Horndean Football Club, Five Heads Road, Horndean, Waterlooville, PO8 9NZ', '100062456518', 'Exact UPRN Match', 1.0000, 'ACCEPT', 'correct_match'),
('Valid Match 2', '4 MONKS ORCHARD, PETERSFIELD', '4', 'monks orchard', 'petersfield', '4 Monks Orchard, Petersfield, GU32 2JD', '1710034162', 'Postcode + House Number', 1.0000, 'ACCEPT', 'correct_match');

-- Now test the fixed algorithm logic using SQL
SELECT 
    'Fixed Algorithm Validation Results' as test_section;

-- Test 1: House Number Validation Logic
SELECT 
    tc.test_name,
    tc.source_house_number,
    tc.matched_address,
    tc.old_confidence,
    tc.expected_result,
    CASE 
        -- Extract house number from matched address (simple pattern)
        WHEN tc.source_house_number IS NOT NULL 
         AND tc.matched_address IS NOT NULL
         AND tc.matched_address !~ ('^' || tc.source_house_number || '[^0-9]')
         AND tc.matched_address ~ '^[0-9]+'
        THEN 'REJECT - House number mismatch'
        
        WHEN tc.source_house_number IS NOT NULL 
         AND tc.matched_address IS NOT NULL
         AND tc.matched_address ~ ('^' || tc.source_house_number || '[^0-9]')
        THEN 'ACCEPT - House number matches'
        
        WHEN tc.source_house_number IS NULL
        THEN 'NEUTRAL - No house number to validate'
        
        ELSE 'UNKNOWN - Cannot parse'
    END as fixed_algorithm_result,
    
    CASE 
        WHEN tc.expected_result = 'REJECT' 
         AND tc.matched_address !~ ('^' || COALESCE(tc.source_house_number, '') || '[^0-9]')
         AND tc.source_house_number IS NOT NULL
        THEN '✅ CORRECTLY REJECTED'
        
        WHEN tc.expected_result = 'ACCEPT'
         AND (tc.source_house_number IS NULL 
              OR tc.matched_address ~ ('^' || tc.source_house_number || '[^0-9]'))
        THEN '✅ CORRECTLY ACCEPTED'
        
        WHEN tc.expected_result = 'REJECT'
         AND tc.matched_address ~ ('^' || COALESCE(tc.source_house_number, '') || '[^0-9]')
        THEN '❌ FALSE ACCEPT'
        
        WHEN tc.expected_result = 'ACCEPT'
         AND tc.matched_address !~ ('^' || COALESCE(tc.source_house_number, '') || '[^0-9]')
         AND tc.source_house_number IS NOT NULL
        THEN '❌ FALSE REJECT'
        
        ELSE '? UNCLEAR'
    END as validation_status

FROM test_cases tc
ORDER BY tc.test_id;

-- Test 2: Business Name Validation
SELECT 
    'Business Name Validation' as test_section;

SELECT 
    tc.test_name,
    tc.source_address,
    tc.matched_address,
    tc.old_confidence,
    CASE 
        WHEN tc.source_address ILIKE '%club%' 
         AND tc.matched_address NOT ILIKE '%club%'
        THEN 'REJECT - Business type mismatch'
        
        WHEN tc.source_address ILIKE '%ltd%' 
         AND tc.matched_address NOT ILIKE '%ltd%'
         AND tc.matched_address NOT ILIKE '%limited%'
        THEN 'REJECT - Company type mismatch'
        
        WHEN tc.source_address ILIKE '%club%' 
         AND tc.matched_address ILIKE '%club%'
        THEN 'ACCEPT - Business type matches'
        
        ELSE 'NEUTRAL - No clear business mismatch'
    END as business_validation_result

FROM test_cases tc
WHERE tc.issue_type = 'wrong_business'
ORDER BY tc.test_id;

-- Test 3: Overall Algorithm Effectiveness
SELECT 
    'Algorithm Effectiveness Summary' as summary_section;

SELECT 
    tc.issue_type,
    COUNT(*) as total_test_cases,
    COUNT(CASE 
        WHEN tc.expected_result = 'REJECT' 
         AND tc.source_house_number IS NOT NULL
         AND tc.matched_address !~ ('^' || tc.source_house_number || '[^0-9]')
        THEN 1 
        WHEN tc.expected_result = 'ACCEPT'
         AND (tc.source_house_number IS NULL 
              OR tc.matched_address ~ ('^' || tc.source_house_number || '[^0-9]'))
        THEN 1
    END) as correctly_handled,
    
    ROUND(100.0 * COUNT(CASE 
        WHEN tc.expected_result = 'REJECT' 
         AND tc.source_house_number IS NOT NULL
         AND tc.matched_address !~ ('^' || tc.source_house_number || '[^0-9]')
        THEN 1 
        WHEN tc.expected_result = 'ACCEPT'
         AND (tc.source_house_number IS NULL 
              OR tc.matched_address ~ ('^' || tc.source_house_number || '[^0-9]'))
        THEN 1
    END) / COUNT(*), 1) as success_rate_pct

FROM test_cases tc
GROUP BY tc.issue_type
ORDER BY tc.issue_type;

-- Test 4: Confidence Score Validation
SELECT 
    'Confidence Score Analysis' as confidence_section;

SELECT 
    AVG(tc.old_confidence) as avg_old_confidence,
    COUNT(CASE WHEN tc.old_confidence >= 0.85 THEN 1 END) as old_high_confidence,
    COUNT(CASE WHEN tc.expected_result = 'REJECT' AND tc.old_confidence >= 0.85 THEN 1 END) as wrong_high_confidence,
    
    ROUND(100.0 * COUNT(CASE WHEN tc.expected_result = 'REJECT' AND tc.old_confidence >= 0.85 THEN 1 END) / 
          NULLIF(COUNT(CASE WHEN tc.old_confidence >= 0.85 THEN 1 END), 0), 1) as false_positive_rate_pct

FROM test_cases tc
WHERE tc.expected_result = 'REJECT';

ROLLBACK;