# EHDC LLPG Address Matching System

A sophisticated address matching system designed for East Hampshire District Council (EHDC) to match addresses from various source documents against the official Local Land and Property Gazetteer (LLPG).

## Overview

This system processes four types of source documents:
- **Planning Decision Notices** (76,172 records)
- **Land Charge Cards** (49,760 records) 
- **Enforcement Notices** (1,172 records)
- **Planning Agreements** (2,602 records)

**Total: 129,706 documents** to be matched against **71,665 LLPG addresses**

The system implements multiple sophisticated matching strategies to achieve maximum coverage while maintaining high accuracy:

- **Deterministic Matching**: Exact UPRN validation and canonical address matching
- **Fuzzy Matching**: PostgreSQL trigram similarity with phonetic and structural filtering  
- **Spatial Matching**: Geographic proximity using PostGIS
- **Hierarchical Matching**: Multi-level component-based matching with fallbacks
- **Rule-Based Matching**: Pattern transformation for known address variations
- **Vector Matching**: Semantic similarity using text embeddings
- **Manual Review Interface**: Interactive validation for edge cases

## Prerequisites

- **PostgreSQL 12+** with PostGIS and pg_trgm extensions
- **Go 1.19+**
- **Database**: `ehdc_llpg` 

## Quick Start

### 1. Build the System
```bash
go build -o bin/matcher cmd/matcher/main.go
```

### 2. Test Database Connection
```bash
./bin/matcher ping
```

### 3. Run Complete Matching Process

You have several options to apply all matching logic:

#### **Option A: Automated Script (Recommended)**
```bash
# Run complete matching process automatically (2-4 hours)
./run_full_matching.sh

# Or run in background and monitor
nohup ./run_full_matching.sh > matching_log.txt 2>&1 &
tail -f matching_log.txt
```

#### **Option B: Manual Sequential Execution**
Execute the following commands **in sequence** to process all records:

```bash
# Stage 1: Deterministic Matching (validates existing UPRNs)
./bin/matcher match deterministic --batch-size 1000

# Stage 2: Fuzzy Matching (trigram similarity)  
./bin/matcher match fuzzy --batch-size 1000 --min-similarity 0.75

# Stage 3: Advanced Matching Strategies
./bin/matcher match spatial --batch-size 500 --max-distance 100
./bin/matcher match hierarchical --batch-size 500
./bin/matcher match rule-based --batch-size 1000
./bin/matcher match vector --batch-size 100 --min-similarity 0.70
```

#### **Option C: High-Performance Fuzzy Only (Faster)**
For quick results with the most effective algorithm:
```bash
./bin/matcher match fuzzy-optimized --workers 8 --min-similarity 0.75 --batch-size 1000
```

#### **Option D: Test Run First**
Test on a sample before full processing:
```bash
./bin/matcher match tune-thresholds --sample-size 1000
```

**Progress Tracking**: Each command shows real-time progress with processing rates and statistics.

### 4. Export Results to CSV

Export enhanced source documents with matching results:

```bash
# Export all source types to CSV files
./bin/matcher match export --output export

# Show export statistics only
./bin/matcher match export --stats
```

### 5. Access Database Views

The system creates SQL views for direct database access:

```bash
# Apply enhanced database views
./bin/matcher db apply-views

# Test views and show samples
./bin/matcher db test-views
```

## Output Files

### CSV Exports (in `export/` directory)
- `enhanced_decision_results.csv` - Planning decisions with matching results
- `enhanced_land_charge_results.csv` - Land charges with matching results  
- `enhanced_enforcement_results.csv` - Enforcement notices with matching results
- `enhanced_agreement_results.csv` - Planning agreements with matching results

### Enhanced Columns Added

Each CSV includes the original source columns plus these calculated fields:

| Column | Description | Values |
|--------|-------------|---------|
| **Address_Quality** | Source address data quality | GOOD/FAIR/POOR |
| **Match_Status** | Overall matching status | MATCHED/UNMATCHED/NEEDS_REVIEW |
| **Match_Method** | Algorithm that found the match | deterministic/fuzzy/spatial/hierarchical/rule/vector |
| **Match_Score** | Confidence score | 0.000-1.000 |
| **Coordinate_Distance** | Distance between coordinates (meters) | 0.000+ or blank |
| **Address_Similarity** | Text similarity score | 0.000-1.000 |
| **Matched_UPRN** | Official LLPG UPRN | 10-digit identifier |
| **LLPG_Address** | Official LLPG address | Full address text |

## Database Views

### Main Views
- `v_enhanced_source_documents` - All source documents with matching results
- `v_enhanced_decisions` - Decision notices with enhanced fields
- `v_enhanced_land_charges` - Land charges with enhanced fields  
- `v_enhanced_enforcement` - Enforcement notices with enhanced fields
- `v_enhanced_agreements` - Planning agreements with enhanced fields

### Analysis Views  
- `v_match_summary_by_type` - Match rates by document type
- `v_address_quality_summary` - Address quality vs match success
- `v_method_effectiveness` - Algorithm performance metrics
- `v_unmatched_analysis` - Analysis of unmatched documents

### Example Queries

```sql
-- View all matched decisions with high confidence
SELECT src_id, original_address, matched_uprn, llpg_address, match_score
FROM v_enhanced_decisions 
WHERE match_status = 'MATCHED' AND match_score >= 0.90;

-- Analyze unmatched records by address quality
SELECT address_quality, COUNT(*) as unmatched_count
FROM v_enhanced_source_documents 
WHERE match_status = 'UNMATCHED'
GROUP BY address_quality;

-- Method effectiveness summary
SELECT * FROM v_method_effectiveness ORDER BY match_count DESC;
```

## Monitoring Progress

While any matching process runs, monitor progress in another terminal:

```bash
# Check current match statistics
./bin/matcher match export --stats

# View recent matching runs
./bin/matcher db test-views

# Monitor database directly (if you have psql access)
psql -d ehdc_llpg -c "SELECT * FROM v_match_summary_by_type;"

# Check recent activity
psql -d ehdc_llpg -c "SELECT run_label, total_processed, auto_accepted, run_completed_at FROM match_run ORDER BY run_id DESC LIMIT 5;"
```

## Expected Results

After running the complete matching process, you should see:

- **Match rate improvement** from current ~0.17% to target **75-85%**
- **Enhanced CSV files** with calculated quality metrics
- **Full audit trail** of all matching decisions
- **Database views** for ongoing analysis

Processing times (approximate):
- **Deterministic**: ~5-10 minutes (fast)
- **Fuzzy**: ~30-60 minutes (medium)
- **Spatial**: ~45-90 minutes (slower)
- **Hierarchical**: ~20-40 minutes (medium)
- **Rule-based**: ~10-15 minutes (fast)
- **Vector**: ~60-120 minutes (slowest)

**Total time**: 2-4 hours for complete dataset

## Advanced Usage

### Manual Review Interface
```bash
# Start interactive review session
./bin/matcher match review --batch-size 10 --reviewer "John Smith"

# Show review queue statistics
./bin/matcher match review --stats
```

### Analysis and Tuning
```bash
# Analyze data quality and matching potential
./bin/matcher match analyze

# Test different similarity thresholds
./bin/matcher match tune-thresholds --sample-size 1000

# Show postcode quality analysis  
./bin/matcher match postcode --batch-size 1000
```

### Optimized Fuzzy Matching
```bash
# Run fuzzy matching with parallel processing
./bin/matcher match fuzzy-optimized --workers 8 --min-similarity 0.75
```

## Architecture

### Matching Pipeline
1. **Data Import** → Canonical address generation and postcode extraction
2. **Stage 1: Deterministic** → Legacy UPRN validation + exact matches  
3. **Stage 2: Fuzzy** → Trigram similarity with phonetic filtering
4. **Stage 3: Advanced** → Spatial, hierarchical, rule-based, and vector matching
5. **Manual Review** → Human validation of edge cases
6. **Export** → Enhanced CSV generation with all results

### Key Components
- **Normalization Engine**: Address canonicalization and postcode extraction
- **Matching Algorithms**: 6 different matching strategies with configurable parameters
- **Audit System**: Complete match provenance and decision tracking  
- **Quality Assessment**: Automated address quality scoring
- **Export System**: Flexible CSV export with calculated metrics

### Database Schema
- `src_document` - Source documents with canonical addresses
- `dim_address` - LLPG master address dimension
- `match_accepted` - Final accepted matches (one per source)
- `match_result` - Detailed candidate results with full audit trail
- `match_run` - Matching run metadata and statistics

## Performance

**Typical Processing Rates**:
- Deterministic: ~2,000 docs/sec
- Fuzzy: ~100-200 docs/sec  
- Spatial: ~50-100 docs/sec
- Hierarchical: ~80-150 docs/sec
- Rule-based: ~500-800 docs/sec
- Vector: ~20-40 docs/sec

**Full Dataset Processing Time**: ~2-4 hours depending on configuration

## Configuration

Key matching parameters can be tuned:

- `--min-similarity`: Minimum text similarity threshold (0.70-0.90)
- `--batch-size`: Processing batch size for memory management
- `--max-distance`: Maximum coordinate distance for spatial matching (meters)
- `--workers`: Parallel processing workers for optimized matching

## Support

For questions or issues:
1. Check the built-in help: `./bin/matcher --help`
2. Review matching statistics: `./bin/matcher match export --stats`
3. Test database connectivity: `./bin/matcher ping`
4. Analyze data quality: `./bin/matcher match analyze`

## Development

### Project Structure
```
├── cmd/matcher/           # CLI application entry point
├── internal/
│   ├── engine/           # Core matching algorithms
│   ├── import/           # CSV import utilities  
│   ├── normalize/        # Address normalization
│   └── db/              # Database connection handling
├── sql/                 # Database schema and views
└── export/             # CSV output directory
```

### Adding New Matching Strategies
1. Implement matcher interface in `internal/engine/`
2. Add CLI command in `cmd/matcher/main.go`
3. Update export system in `internal/engine/exporter.go`
4. Add tests and documentation

---

**EHDC LLPG Address Matching System** - Sophisticated, scalable, and auditable address matching for local government.