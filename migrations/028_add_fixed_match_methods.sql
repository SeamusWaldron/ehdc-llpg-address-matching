-- Migration 028: Add Fixed Match Methods
-- Purpose: Add match method entries for the fixed component-based algorithm
-- Author: Claude Code Assistant
-- Date: 2025-08-20

BEGIN;

-- Add new match methods for the fixed algorithm
INSERT INTO dim_match_method (method_id, method_code, method_name, confidence_threshold) VALUES
(20, 'postcode_house_validated', 'Postcode + House Number (Validated)', 0.95),
(21, 'business_name_match', 'Business Name Matching', 0.80),
(22, 'road_city_validated', 'Road + City (Validated)', 0.85),
(23, 'fuzzy_road_validated', 'Fuzzy Road (Validated)', 0.70),
(24, 'exact_components_validated', 'Exact Components (Validated)', 0.98)
ON CONFLICT (method_id) DO UPDATE SET
    method_code = EXCLUDED.method_code,
    method_name = EXCLUDED.method_name,
    confidence_threshold = EXCLUDED.confidence_threshold;

-- Update sequence to avoid conflicts
SELECT setval('dim_match_method_method_id_seq', 25, false);

COMMIT;