# Address Matching Process - Complete Workflow

This document outlines the complete step-by-step process for identifying addresses and matching them with UPRNs in the EHDC LLPG system.

## Overview

The address matching system uses a multi-phase approach that prioritizes official UPRNs and falls back to component-based matching when UPRNs are not available or found in the Local Land and Property Gazetteer (LLPG).

---

## Phase 1: Data Preparation

### 1. Import Source Documents → `src_document` table
**Complete Dataset: 290,758 documents across 9 document types**

| Document Type | Records | Source File | Contains |
|---------------|---------|-------------|----------|
| **Microfiche Post-1974** | 108,164 | `microfiche_post_1974.csv` | Planning refs, no addresses |
| **Decision Notice** | 76,167 | `decision_notices.csv` | Full addresses, some UPRNs |
| **Land Charge** | 49,760 | `land_charges_cards.csv` | Addresses, coordinates |
| **Microfiche Pre-1974** | 43,977 | `microfiche_pre_1974.csv` | Planning refs, no addresses |
| **Street Name & Numbering** | 7,385 | `street_name_and_numbering.csv` | Full addresses, UPRNs, coordinates |
| **Agreement** | 2,602 | `agreements.csv` | Legal agreements, addresses |
| **Enlargement Maps** | 1,514 | `enlargement_maps.csv` | Map references only |
| **Enforcement Notice** | 1,172 | `enforcement_notices.csv` | Addresses, enforcement actions |
| **ENL Folders** | 17 | `enl_folders.csv` | Development addresses, UPRNs |

**UPRN Coverage**: 31,203 documents have UPRNs (10.7% of total dataset)

### 2. Import LLPG Data → `dim_address` table  
- East Hampshire LLPG golden records (71,880 addresses)
- Official UPRNs, addresses, coordinates
- Mark all as `is_historic = false`

### 3. Bulk Historic UPRN Creation (NEW - Phase 1.5)
**Efficient bulk process to identify and create all missing UPRNs upfront**

```sql
-- Find all normalized UPRNs not in LLPG
SELECT CASE WHEN raw_uprn LIKE '%.00' 
            THEN REPLACE(raw_uprn, '.00', '') 
            ELSE raw_uprn END as normalized_uprn
FROM src_document 
WHERE NOT EXISTS(SELECT 1 FROM dim_address WHERE uprn = normalized_uprn)
```

- **UPRN Normalization**: Remove `.00` decimal suffixes (critical fix)
- **Bulk Insert**: Create all missing historic records in single transaction
- **Performance**: ~700 UPRNs/sec vs. individual checking during matching

### 4. Address Normalization (gopostal/libpostal)
- **Normalize LLPG addresses**: Extract components (house_number, road, city, postcode)
- **Normalize source addresses**: Extract same components from raw text
- Store normalized components in both tables

---

## Phase 2: UPRN Priority Matching (Highest Confidence)

### 5. Check Source Document UPRN
```
IF source document has UPRN:
```

### 6. UPRN Normalization & Lookup
```go
// Critical Fix: Normalize UPRN by removing decimal suffixes
cleanUPRN := strings.TrimSpace(uprn)
if strings.HasSuffix(cleanUPRN, ".00") {
    cleanUPRN = strings.TrimSuffix(cleanUPRN, ".00")
}
```

```sql
-- Query with normalized UPRN
SELECT * FROM dim_address WHERE uprn = cleanUPRN
```

### 7. UPRN Match Result
- **Found in LLPG** → Perfect match (score 1.0, auto-accept)
- **Not found in LLPG** → Match to pre-created historic record

**Key Improvement**: Historic UPRNs are now **bulk-created upfront** rather than individually during matching, improving performance from ~10 docs/sec to ~700 UPRNs/sec for missing UPRN processing.

---

## Phase 3: Component-Based Matching (Fallback)

### 8. Component Matching Trigger
**IF no UPRN or UPRN processing complete, use components:**

### 9. Strategy 1: Postcode + House Number (High Confidence)
```sql
WHERE gopostal_postcode = input_postcode 
  AND gopostal_house_number = input_house_number
```

### 10. Strategy 2: Business Name Matching (Organizations)
```sql
WHERE similarity(full_address, business_name) >= 0.8
```

### 11. Strategy 3: Road + City + House Number (Strict Validation)
```sql
WHERE gopostal_road = input_road 
  AND gopostal_city = input_city
  AND gopostal_house_number = input_house_number  -- CRITICAL FIX
```

### 12. Strategy 4: Fuzzy Road Matching (With House Number Validation)
```sql
WHERE gopostal_road % input_road  -- fuzzy match
  AND similarity(gopostal_road, input_road) >= 0.8
  AND gopostal_house_number = input_house_number  -- MUST MATCH
```

---

## Phase 4: Validation & Scoring

### 13. Component Score Calculation
- **House number match**: 1.0 or 0.0 (no partial credit)
- **Road match**: Exact (1.0) or token overlap similarity
- **City match**: Exact (1.0) or 0.0
- **Postcode match**: Exact (1.0) or 0.0

### 14. Critical Validation Rules
- **House number mismatch** → 90% penalty (score × 0.1)
- **Minimum score threshold** → 0.6 to be considered
- **Must have strong component match** → At least one perfect component

### 15. Decision Matrix
- **Perfect score (1.0)** → auto_accept
- **Score ≥ 0.95** → auto_accept  
- **Score ≥ 0.8** → needs_review (manual)
- **Score ≥ 0.6** → low_confidence (manual)
- **Score < 0.6** → no_match

---

## Phase 5: Result Storage

### 16. Save Match Results → `address_match` table
- `document_id` → source document
- `address_id` → matched LLPG address (or historic record)
- `match_method_id` → how it was matched
- `confidence_score` → calculated score
- `match_status` → auto/manual
- `decision` → auto_accept/needs_review/low_confidence/no_match

---

## Phase 6: LLM-Powered Address Correction (NEW)

### 17. Intelligent Address Format Correction
**Uses Large Language Model to fix obvious formatting mistakes in low-confidence addresses**

```bash
./matcher-v2 -cmd=llm-fix-addresses
```

**Target Patterns**:
- `"5, AMEY INDUSTRIAL ESTATE"` → `"Unit 5, Amey Industrial Estate"`
- `"INDUSTRIAL ESTATE"` → `"Industrial Estate"` (capitalization)
- Adding missing "Unit" prefixes where clearly needed
- Preserving core address components while improving searchability

**LLM Integration**:
- **Model**: llama3.2:1b via Ollama container
- **Selection Criteria**: Low confidence (≤0.4) addresses with formatting patterns
- **Smart Prompting**: Distinguishes industrial estates from residential addresses
- **Postcode-Flexible Validation**: Uses component-based matching with ordering insights
- **House Number Protection**: Prevents false matches through number validation

**Process Flow**:
1. **Identify candidates**: Regex patterns for unit numbers, industrial estates, capitalization issues
2. **Intelligent LLM correction**: 
   - Industrial estates: Add "Unit" prefix (e.g., "5, AMEY INDUSTRIAL" → "Unit 5, Amey Industrial Estate")
   - Residential: Capitalization only (e.g., "14 THORPE GARDENS" → "14 Thorpe Gardens")
3. **Advanced LLPG validation**: 
   - Trigram similarity with postcode boundary handling
   - House number validation prevents dangerous matches
   - Uses ordering: `postcode, house, house_number, address_canonical`
4. **Conservative application**: Only corrections with strong validation applied

**Production Results** (Final Implementation):
- **32 LLM corrections applied** with enhanced validation
- **Hybrid approach**: Combines trigram matching with component validation
- **Zero false house number matches**: House number validation prevents errors
- **Postcode boundary handling**: Flexible matching across postcode variations

---

## Phase 7: Dimensional Model

### 18. Populate Fact Table → `fact_documents_lean`
- Links to dimension tables for normalized reporting
- References original addresses, match decisions, document types
- Incorporates corrections from all phases (group consensus, fuzzy matching, LLM)
- Enables analytics on match quality and patterns

---

## Key Improvements Made

### UPRN Priority Implementation
- Source UPRNs now get **absolute priority**
- Missing UPRNs create **historic records** instead of falling back
- **9,297 UPRNs** will be added as historic when processed

### Fixed Component Engine
- **Strict house number validation** prevents wrong matches
- **Proper penalty scoring** for mismatches
- **Business name matching** for organizations
- **Geographic validation** ensures quality

---

## Current System Status (Complete Dataset - UPDATED)

### Dataset Overview
- **Total Documents**: 130,540 (processed dataset after address quality filtering)
- **Document Types**: 9 types from planning, legal, enforcement, and archival sources  
- **Date Range**: Pre-1974 to present day
- **LLPG Records**: 71,880 official addresses

### Matching Results (Final Production System)
- **Total Processed**: 130,540 documents (complete dataset after quality filtering)
- **Total Matches**: 74,696 documents (**57.22% match rate**)
- **Total Corrections Applied**: 10,015 corrections across 7 methods
- **Processing Version**: 1.1-with-corrections (production ready)

#### Quality Distribution
- **High Confidence (≥0.85)**: 54,202 documents (auto-accepted)
- **Medium Confidence (0.50-0.84)**: 16,357 documents (manual review)
- **Low Confidence (0.20-0.49)**: 4,130 documents (flagged for review)
- **No Match (<0.20)**: 55,844 documents (unmatchable)

### Correction Method Breakdown
```
Historic UPRN Creation              | 5,119 corrections (Legacy UPRN validation)
Exact UPRN Match                    | 3,450 corrections (Direct UPRN confirmation)
Road + City (Validated)             |   939 corrections (Geographic validation)
Fuzzy Road (Validated)              |   305 corrections (Fuzzy matching with validation)
Postcode + House Number (Validated) |   169 corrections (Postal code matching)
LLM-Powered Address Correction      |    32 corrections (AI-powered format improvement)
Business Name Matching              |     1 correction (Organization matching)
Total Corrections                   | 10,015 corrections
```

### Key Data Quality Improvements
1. **Dangerous Match Correction**: Fixed 5,836 false high-confidence matches
2. **Group Consensus Safety**: Removed 7,920 unsafe consensus corrections
3. **LLM-Powered Formatting**: Intelligent address format correction with postcode-flexible validation
4. **House Number Protection**: Prevents false matches through strict number validation
5. **Postcode Boundary Handling**: Flexible matching across postcode variations using ordering insights
6. **Conservative Confidence Scoring**: Downgraded risky matches to prevent false positives

### Match Quality Distribution
- **High Confidence (≥0.85)**: Auto-accepted matches with validation
- **Medium Confidence (0.50-0.84)**: Manually reviewable matches
- **Low Confidence (0.20-0.49)**: Flagged for expert review
- **Corrected Addresses**: 10,009 addresses improved through various correction methods

---

## Quality Assurance

### Before Fixed Engine
- **Critical Issue**: "4 MONKS ORCHARD" matched to "16 MONKS ORCHARD"
- **House number mismatches**: 7,077+ wrong matches identified
- **No validation**: Addresses with different house numbers accepted

### After Fixed Engine
- **✅ Zero high-confidence house number mismatches**
- **Strict validation**: House numbers must match exactly
- **Proper penalties**: 90% score reduction for mismatches
- **Business matching**: Improved organization address handling

---

## Technical Implementation

### Database Schema
1. **Source Documents**: Complete staging and import pipeline
   - 5 new staging tables: `stg_street_name_numbering`, `stg_microfiche_post_1974`, `stg_microfiche_pre_1974`, `stg_enlargement_maps`, `stg_enl_folders`
   - Migration 030: Create staging tables with proper indexing
   - Migration 031: Bulk migration to `src_document` with 161,057 new records

2. **Historic UPRN Support**: Enhanced schema for historic records
   - Migration 029: `is_historic`, `created_from_source`, `source_document_id` columns
   - Historic UPRN creation function with audit trail

### Key Applications
1. **`/cmd/matcher-v2/main.go`** - Complete orchestrated matching system (CURRENT)
   - All-in-one application with multiple commands
   - Integrated LLM-powered address correction
   - Group consensus corrections and fuzzy matching
   - Comprehensive fact table management

2. **`/cmd/bulk-historic-uprns/main.go`** - Bulk historic UPRN creation
   - Identifies missing UPRNs with normalization
   - Bulk creates historic records (700+ UPRNs/sec)
   - Comprehensive reporting and validation

3. **`/cmd/component-matcher-fixed/main.go`** - Legacy enhanced matching engine
   - UPRN normalization (removes `.00` suffixes)
   - Integration with pre-created historic records
   - Superseded by matcher-v2

### Key Functions
- `findMissingUPRNs()` - Bulk identification of missing UPRNs with normalization
- `bulkCreateHistoricUPRNs()` - High-performance bulk insertion
- `ProcessDocument()` - Main entry point with UPRN priority and normalization
- `exactUPRNDirectMatch()` - Handles existing UPRN matches with decimal fix
- `performValidatedMatching()` - Component-based fallback
- `validateCandidate()` - Quality validation rules

### Complete Orchestrated Workflow (matcher-v2)

The current production system uses a single orchestrated application with multiple phases:

```bash
# 1. Database setup and schema creation
./matcher-v2 -cmd=setup-db

# 2. Load authoritative address data
./matcher-v2 -cmd=load-llpg -llpg=llpg_docs/ehdc_llpg_20250710.csv
./matcher-v2 -cmd=load-os-uprn -os-uprn=llpg_docs/osopenuprn_202507.csv

# 3. Load all source documents (9 types)
./matcher-v2 -cmd=load-sources

# 4. Validate legacy UPRNs and enrich coordinates
./matcher-v2 -cmd=validate-uprns

# 5. Run initial batch matching (algorithmic)
./matcher-v2 -cmd=match-batch -run-label="production-v2"

# 6. Apply group consensus corrections (safe patterns)
./matcher-v2 -cmd=apply-corrections

# 7. Find fuzzy matches for unmatched groups
./matcher-v2 -cmd=fuzzy-match-groups

# 8. LLM-powered address format correction (NEW)
./matcher-v2 -cmd=llm-fix-addresses

# 9. Rebuild dimensional fact table with all corrections
./matcher-v2 -cmd=rebuild-fact

# 10. Validate data integrity
./matcher-v2 -cmd=validate-integrity
```

### Key Integration Points

1. **Corrections Table**: `address_match_corrected` centralizes all correction methods
2. **Fact Table Rebuild**: Incorporates corrections from ALL phases automatically  
3. **LLM Integration**: Seamlessly integrated with existing Ollama container
4. **Quality Assurance**: Built-in validation prevents dangerous matches
5. **Audit Trail**: Full traceability of all correction methods and confidence scores

This orchestrated process ensures **maximum accuracy and safety** by:
- Prioritizing official UPRNs and validated matches
- Applying intelligent corrections through multiple methods (consensus, fuzzy, LLM)
- Maintaining conservative confidence scoring to prevent false positives
- Providing complete audit trail and validation capabilities