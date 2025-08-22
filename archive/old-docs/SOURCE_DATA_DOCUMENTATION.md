# EHDC Source Data Documentation

This document provides comprehensive documentation of the complete source dataset used in the EHDC LLPG address matching system.

## Dataset Overview

- **Total Documents**: 290,758 records (imported) → 130,540 records (processed after filtering)
- **Source Files**: 9 CSV files
- **Document Types**: Planning, legal, enforcement, and archival records
- **Date Range**: Pre-1974 to present day
- **UPRN Coverage**: 31,203 documents (10.7%) have UPRNs
- **Import Date**: August 2025
- **Processing Status**: Production ready with LLM-powered corrections

---

## Source File Breakdown

### 1. Microfiche Post-1974 (`microfiche_post_1974.csv`)
- **Records**: 108,164 (37.2% of total dataset)
- **Content**: Planning application records from post-1974 period stored on microfiche
- **Data Fields**:
  - `Job Number`: Archive job reference
  - `Filepath`: Location of scanned microfiche file
  - `Planning Application Reference Number`: Planning reference (e.g., F20001, AB-JG-20437)
  - `Fiche Number`: Microfiche storage reference
- **Address Information**: **No direct addresses** - uses planning references
- **UPRN Information**: None
- **Typical Record**: `JN24908, Box 01_F20001-F20437\F20001.pdf, F20001, [blank]`

### 2. Decision Notice (`decision_notices.csv`)
- **Records**: 76,167 (26.2% of total dataset)
- **Content**: Planning decision notices with full address details
- **Data Fields**: Full address information, some UPRNs, decision details
- **Address Information**: **Full addresses available** - primary source for matching
- **UPRN Information**: **Partial UPRN coverage**
- **Import Date**: Previously imported (part of original 129,701 records)

### 3. Land Charge (`land_charges_cards.csv`)
- **Records**: 49,760 (17.1% of total dataset)
- **Content**: Land charge registration records
- **Data Fields**: Property addresses, charge details, coordinates
- **Address Information**: **Full addresses available**
- **UPRN Information**: **Partial UPRN coverage**
- **Import Date**: Previously imported (part of original 129,701 records)

### 4. Microfiche Pre-1974 (`microfiche_pre_1974.csv`)
- **Records**: 43,977 (15.1% of total dataset)
- **Content**: Planning application records from pre-1974 period stored on microfiche
- **Data Fields**:
  - `Job Number`: Archive job reference
  - `Filepath`: Location of scanned microfiche file
  - `Planning Application Reference Number`: Historical planning reference
  - `Fiche Number`: Microfiche storage reference
- **Address Information**: **No direct addresses** - uses planning references
- **UPRN Information**: None (predates UPRN system)
- **Historical Significance**: Covers pre-UPRN era planning applications

### 5. Street Name and Numbering (`street_name_and_numbering.csv`)
- **Records**: 7,385 (2.5% of total dataset)
- **Content**: Official street naming and property numbering records
- **Data Fields**:
  - `Job Number`: Processing job reference
  - `Filepath`: Document location
  - `Address`: **Full address with postal formatting**
  - `BS7666UPRN`: **Official UPRN** (BS7666 standard)
  - `Easting`: **Coordinate data** (British National Grid)
  - `Northing`: **Coordinate data** (British National Grid)
- **Address Information**: **High quality addresses** - authoritative source
- **UPRN Information**: **100% UPRN coverage** - official UPRNs
- **Coordinate Information**: **Full coordinate coverage**
- **Typical Record**: `JN24963, "Box 05\Ropley\Plum Tree House...", "Plum Tree House, Gascoigne Lane, Ropley", 100060259666, 464217, 132298`

### 6. Agreement (`agreements.csv`)
- **Records**: 2,602 (0.9% of total dataset)
- **Content**: Legal agreements related to planning and development
- **Data Fields**: Agreement addresses, legal details
- **Address Information**: **Full addresses available**
- **UPRN Information**: **Partial UPRN coverage**
- **Import Date**: Previously imported (part of original 129,701 records)

### 7. Enlargement Maps (`enlargement_maps.csv`)
- **Records**: 1,514 (0.5% of total dataset)
- **Content**: References to enlargement maps for planning applications
- **Data Fields**:
  - `Job Number`: Archive processing reference
  - `Filepath`: Map file location
  - `Enlargement Map Number`: Map reference (e.g., 1, 10B, 45A)
- **Address Information**: **No addresses** - map references only
- **UPRN Information**: None
- **Typical Record**: `JN24960, Box 14_Client Box 14\ENL Maps 001-099\1.pdf, 1`

### 8. Enforcement Notice (`enforcement_notices.csv`)
- **Records**: 1,172 (0.4% of total dataset)
- **Content**: Planning enforcement notices
- **Data Fields**: Enforcement addresses, violation details
- **Address Information**: **Full addresses available**
- **UPRN Information**: **Limited UPRN coverage**
- **Import Date**: Previously imported (part of original 129,701 records)

### 9. ENL Folders (`enl_folders.csv`)
- **Records**: 17 (<0.1% of total dataset)
- **Content**: Development-related folder records
- **Data Fields**:
  - `Job Number`: Processing reference
  - `Filepath`: Folder location
  - `Address`: **Development site address**
  - `BS7666UPRN`: **Official UPRN**
  - `Easting`: **Coordinate data**
  - `Northing`: **Coordinate data**
- **Address Information**: **High quality addresses** - development sites
- **UPRN Information**: **100% UPRN coverage**
- **Coordinate Information**: **Full coordinate coverage**
- **Typical Record**: `JN24960, "Box 12_Client Box 12\01_Dev at Green Lane\Land at, Green Lane...", "Land at, Green Lane, Clanfield", 10032900051, 470854.738, 116133.415`

---

## Data Quality Analysis

### Address Coverage by Type
| Category | Document Types | Records | Address Quality |
|----------|---------------|---------|-----------------|
| **Full Addresses** | Decision Notice, Land Charge, Street Name, Agreement, Enforcement, ENL | 137,698 (47.4%) | High - suitable for matching |
| **Planning References** | Microfiche Post/Pre-1974 | 152,141 (52.3%) | Low - reference codes only |
| **Map References** | Enlargement Maps | 1,514 (0.5%) | None - map numbers only |

### UPRN Coverage by Type
| Document Type | UPRN Coverage | Quality |
|---------------|---------------|---------|
| **Street Name & Numbering** | 100% (7,385) | High - official BS7666 |
| **ENL Folders** | 100% (17) | High - official BS7666 |
| **Decision Notice** | Partial | Mixed - some with decimals |
| **Land Charge** | Partial | Mixed - some with decimals |
| **Agreement** | Limited | Mixed |
| **Enforcement Notice** | Limited | Mixed |
| **Microfiche Records** | 0% | None - predates/lacks UPRNs |
| **Enlargement Maps** | 0% | None - map references only |

### Coordinate Data Coverage
| Document Type | Coordinate Coverage | Format |
|---------------|-------------------|--------|
| **Street Name & Numbering** | 100% | British National Grid (Easting/Northing) |
| **ENL Folders** | 100% | British National Grid (Easting/Northing) |
| **Land Charge** | Partial | Mixed formats |
| **Other Types** | Limited/None | Various |

---

## Data Import Pipeline

### Stage 1: CSV Import to Staging Tables
```sql
-- 5 new staging tables created
stg_street_name_numbering    -- 7,385 records
stg_microfiche_post_1974     -- 108,164 records  
stg_microfiche_pre_1974      -- 43,977 records
stg_enlargement_maps         -- 1,514 records
stg_enl_folders              -- 17 records
```

### Stage 2: Data Transformation & Migration
- **Address Canonicalization**: Standardize format, remove special characters
- **UPRN Normalization**: Handle decimal suffixes (`.00` removal)
- **External Reference Mapping**: Planning refs → external_reference field
- **Coordinate Preservation**: Maintain spatial accuracy

### Stage 3: Integration with Existing Data
- **Document Type Dimensions**: 5 new types added to `dim_document_type`
- **Final Integration**: 161,057 new records added to `src_document`
- **Index Creation**: Performance optimization for matching queries

---

## Critical Data Issues Identified & Resolved

### 1. UPRN Decimal Format Issue
**Problem**: UPRNs stored with `.00` decimal suffixes (e.g., `1710022145.00`)
**Impact**: 9,206+ UPRNs couldn't match LLPG records stored without decimals
**Solution**: UPRN normalization in matching engine removes decimal suffixes
**Result**: ~9,000 false historic records prevented

### 2. Missing Source Documents
**Problem**: Only 4 of 9 source files were initially imported (129,701 of 290,758 records)
**Impact**: 55% of available data was missing from analysis
**Solution**: Complete staging table creation and data import pipeline
**Result**: Full dataset now available for comprehensive matching

### 3. Address Quality Variation
**Problem**: Different document types contain varying address quality
**Solution**: Tailored matching strategies per document type
- Full addresses: Component-based matching
- Planning references: Reference-based lookup
- Map references: Reference-only storage

---

## Matching Strategy by Document Type

### High Confidence Matching (Full Addresses + UPRNs)
- **Street Name & Numbering**: UPRN priority → Component matching
- **ENL Folders**: UPRN priority → Component matching  
- **Decision Notice**: UPRN priority → Component matching
- **Land Charge**: UPRN priority → Component matching

### Medium Confidence Matching (Full Addresses Only)
- **Agreement**: Component matching only
- **Enforcement Notice**: Component matching only

### Low Confidence Matching (References Only)
- **Microfiche Post-1974**: Reference matching → No address matching
- **Microfiche Pre-1974**: Reference matching → No address matching
- **Enlargement Maps**: Reference storage only → No address matching

---

## Data Lineage & Audit Trail

### Source Tracking
- Every record maintains `job_number` and `filepath` for traceability
- Document type classification for processing differentiation  
- Creation timestamps for import audit trail

### Historic Record Tracking
- Historic UPRNs link back to source document via `source_document_id`
- `created_from_source` flag identifies system-generated records
- `is_historic` flag distinguishes from official LLPG records

### Processing Statistics
- Import success rates tracked per document type
- UPRN normalization statistics maintained
- Address matching results auditable by source type

---

## Final Processing Results (Production System)

### Address Matching Achievement (Final Production System)
- **Total Processed Records**: 130,540 documents (complete dataset after quality filtering)
- **Successfully Matched**: 74,696 addresses (**57.22% match rate**)
- **Corrections Applied**: 10,015 intelligent corrections across 7 methods
- **Processing Version**: 1.1-with-corrections (production ready)

### Final Correction Technology Breakdown
```
Historic UPRN Creation              | 5,119 corrections (51.11% - Legacy UPRN validation)
Exact UPRN Match                    | 3,450 corrections (34.45% - Direct UPRN confirmation)
Road + City (Validated)             |   939 corrections  (9.38% - Geographic validation)
Fuzzy Road (Validated)              |   305 corrections  (3.05% - Fuzzy matching with validation)
Postcode + House Number (Validated) |   169 corrections  (1.69% - Postal code matching)
LLM-Powered Address Correction      |    32 corrections  (0.32% - AI-powered format improvement)
Business Name Matching              |     1 correction   (0.01% - Organization matching)
Total Applied                       | 10,015 corrections
```

### Quality Assurance Metrics (Production)
- **Records with Complete Address Data**: 74,696 (57.22% of total)
- **Records without Validation Issues**: 60,102 (46.05% of total)
- **Auto-Processed (High Confidence)**: 54,202 (41.53% of total)
- **Average Address Quality Score**: 0.500 (0.0-1.0 scale)
- **Average Data Completeness Score**: 0.700 (0.0-1.0 scale)

### Key Quality Improvements Achieved
1. **Eliminated Dangerous Matches**: Fixed 5,836 false high-confidence matches
2. **Conservative Confidence Scoring**: Prevented false positives through rigorous validation
3. **Advanced LLM Format Correction**: 
   - Industrial estates: "5, AMEY INDUSTRIAL" → "Unit 5, Amey Industrial Estate"
   - Residential addresses: "14 THORPE GARDENS" → "14 Thorpe Gardens" (capitalization only)
   - Postcode-flexible validation with house number protection
4. **Building Number Validation**: Eliminated conflicting building assignments through strict validation
5. **Postcode Boundary Handling**: Flexible matching across postcode variations using ordering insights
6. **Comprehensive Audit Trail**: Full traceability of all 7 correction methods with confidence scores

### Data Quality Assurance
- **Zero high-confidence house number mismatches** after validation fixes
- **Conservative approach**: Prioritizes accuracy over match rate
- **Multi-layered validation**: Algorithmic + AI + manual review thresholds
- **Complete audit trail**: Every correction method tracked and explained

This comprehensive dataset and processing pipeline delivers accurate, validated address matching across East Hampshire District Council's complete historical and current planning, legal, and administrative records with industry-leading quality assurance.