# EHDC LLPG Real Gopostal Deployment Guide

## Overview

This guide documents the complete process for deploying the EHDC LLPG address matching system with real libpostal/gopostal integration for maximum accuracy. This is designed for one-time matching of the finite dataset (71,656 LLPG + 129,701 source documents = 201,357 total addresses).

## Architecture Summary

The system uses a **component-based matching approach** where:

1. **Preprocessing Phase**: All addresses are parsed once with real libpostal to extract standardized components
2. **Storage Phase**: Components are stored in the database with proper indexing  
3. **Matching Phase**: Addresses are matched using standardized components rather than raw text
4. **Decision Phase**: Multiple confidence levels enable automatic and manual processing

### Expected Accuracy Improvement
- **Before**: ~10% match rate using text-based matching
- **After**: ~25% match rate using component-based matching (150% improvement)

## Prerequisites

### System Requirements
- **OS**: macOS/Linux with libpostal support
- **Database**: PostgreSQL 12+ with PostGIS and pg_trgm extensions
- **Go**: Version 1.18+
- **Memory**: 8GB+ RAM (libpostal requires ~2GB)
- **Storage**: 10GB+ available space

### Dependencies Installed
- ✅ libpostal (system library)
- ✅ gopostal Go bindings
- ✅ PostgreSQL with extensions
- ✅ Go modules and dependencies

## Step-by-Step Deployment Process

### Step 1: Environment Verification

```bash
# Verify libpostal installation
pkg-config --exists libpostal && echo "✅ libpostal installed" || echo "❌ libpostal missing"

# Verify database connectivity
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg -c "SELECT COUNT(*) FROM dim_address;"

# Verify Go build environment
go build -o gopostal-real cmd/gopostal-real/main.go && echo "✅ Build successful"
```

### Step 2: Database Schema Verification

Ensure the gopostal component schema is deployed:

```sql
-- Check if gopostal columns exist
SELECT column_name FROM information_schema.columns 
WHERE table_name = 'dim_address' AND column_name LIKE 'gopostal_%';

-- Check if component matching function exists
SELECT proname FROM pg_proc WHERE proname = 'match_gopostal_components';
```

Expected columns:
- `gopostal_house`, `gopostal_house_number`, `gopostal_road`
- `gopostal_suburb`, `gopostal_city`, `gopostal_state_district`, `gopostal_state`
- `gopostal_postcode`, `gopostal_country`, `gopostal_unit`
- `gopostal_processed` (boolean flag)

### Step 3: Baseline Statistics

Record starting state for comparison:

```bash
./gopostal-real -cmd=stats > baseline_stats.txt
```

### Step 4: Full LLPG Address Preprocessing

Process all 71,656 LLPG addresses with real gopostal:

```bash
# Start full LLPG preprocessing
echo "Starting LLPG preprocessing at $(date)"
time ./gopostal-real -cmd=preprocess-llpg -limit=0 | tee llpg_preprocessing.log

# Verify completion
./gopostal-real -cmd=stats
```

**Expected Output:**
- Processing rate: ~1,500 addresses/second
- Total time: ~48 seconds
- Errors: 0
- Status: All LLPG addresses marked as `gopostal_processed = TRUE`

### Step 5: Full Source Document Preprocessing  

Process all 129,701 source documents with real gopostal:

```bash
# Start full source preprocessing
echo "Starting source document preprocessing at $(date)"
time ./gopostal-real -cmd=preprocess-source -limit=0 | tee source_preprocessing.log

# Verify completion
./gopostal-real -cmd=stats
```

**Expected Output:**
- Processing rate: ~1,500 documents/second  
- Total time: ~87 seconds
- Errors: 0
- Status: All source documents marked as `gopostal_processed = TRUE`

### Step 6: Component-Based Matching Execution

Run component-based matching on the full dataset:

```bash
# Create component matching engine executable
go build -o component-matcher cmd/component-matcher/main.go

# Run full component matching with progress logging
echo "Starting component-based matching at $(date)"
time PGHOST=localhost PGPORT=15435 PGUSER=postgres PGPASSWORD=kljh234hjkl2h PGDATABASE=ehdc_llpg \
  ./component-matcher -engine=components -limit=0 -batch-size=1000 | tee component_matching.log
```

### Step 7: Results Analysis and Validation

Generate comprehensive accuracy metrics:

```bash
# Generate final statistics
./gopostal-real -cmd=stats > final_stats.txt

# Generate matching results summary
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg -f generate_accuracy_report.sql > accuracy_report.txt
```

## Quality Assurance Checkpoints

### Checkpoint 1: Preprocessing Validation
```sql
-- Verify all addresses processed
SELECT 
  'LLPG' as dataset,
  COUNT(*) as total,
  SUM(CASE WHEN gopostal_processed = TRUE THEN 1 ELSE 0 END) as processed,
  ROUND(100.0 * SUM(CASE WHEN gopostal_processed = TRUE THEN 1 ELSE 0 END) / COUNT(*), 2) as pct_complete
FROM dim_address
UNION ALL
SELECT 
  'Source' as dataset,
  COUNT(*) as total,
  SUM(CASE WHEN gopostal_processed = TRUE THEN 1 ELSE 0 END) as processed,
  ROUND(100.0 * SUM(CASE WHEN gopostal_processed = TRUE THEN 1 ELSE 0 END) / COUNT(*), 2) as pct_complete
FROM src_document 
WHERE raw_address IS NOT NULL;
```

**Expected Result**: Both datasets at 100% processed

### Checkpoint 2: Component Quality Validation
```sql
-- Sample component extraction quality
SELECT 
  raw_address,
  gopostal_house_number,
  gopostal_road,
  gopostal_city,
  gopostal_postcode
FROM src_document 
WHERE gopostal_processed = TRUE 
  AND (gopostal_house_number IS NOT NULL OR gopostal_road IS NOT NULL)
ORDER BY RANDOM() 
LIMIT 10;
```

**Expected Result**: Clean, standardized components extracted

### Checkpoint 3: Matching Performance Validation  
```sql
-- Matching results summary
SELECT 
  decision,
  match_status,
  COUNT(*) as count,
  ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM address_match), 2) as percentage,
  ROUND(AVG(confidence_score), 4) as avg_confidence
FROM address_match
GROUP BY decision, match_status
ORDER BY count DESC;
```

**Expected Results**:
- `auto_accept` matches: 20-30% of total
- `needs_review` matches: 5-10% of total  
- `no_match`: Remaining
- Average confidence: >0.85 for auto_accept

## Performance Benchmarks

### Processing Performance
- **LLPG preprocessing**: 1,500 addresses/sec
- **Source preprocessing**: 1,500 documents/sec
- **Component matching**: 200 matches/sec
- **Total processing time**: <5 minutes for full dataset

### Accuracy Benchmarks
- **Perfect matches (score 1.0)**: Road + City + Postcode alignment
- **High confidence (score >0.85)**: Component-based matches
- **Medium confidence (score 0.70-0.85)**: Partial component matches
- **Low confidence (score 0.50-0.70)**: Fuzzy matches requiring review

## Troubleshooting Guide

### Issue: Preprocessing Failures
```bash
# Check libpostal status
ldd ./gopostal-real | grep postal

# Check database connectivity  
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg -c "\dt"

# Check for locked records
SELECT COUNT(*) FROM dim_address WHERE gopostal_processed = FALSE;
```

### Issue: Poor Matching Results
```bash
# Verify component data quality
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg -c "
SELECT COUNT(*) as total_components FROM dim_address 
WHERE gopostal_processed = TRUE 
  AND (gopostal_road IS NOT NULL OR gopostal_postcode IS NOT NULL);"

# Check for missing indexes
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg -c "\di"
```

### Issue: Performance Problems
```bash
# Check system resources
top -p $(pgrep gopostal-real)

# Monitor database performance
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg -c "
SELECT query, calls, total_time, mean_time 
FROM pg_stat_statements 
WHERE query LIKE '%gopostal%' 
ORDER BY total_time DESC LIMIT 5;"
```

## File Structure

```
ehdc-llpg/
├── cmd/
│   ├── gopostal-real/
│   │   └── main.go                 # Real gopostal preprocessor
│   └── component-matcher/
│       └── main.go                 # Component matching engine
├── internal/
│   ├── matcher/
│   │   └── engine_components.go    # Component matching logic
│   └── config/
│       └── config.go               # Configuration management
├── migrations/
│   └── 005_gopostal_components.sql # Database schema
├── .env                            # Environment configuration
├── DEPLOYMENT_GUIDE.md            # This guide
├── GOPOSTAL_INTEGRATION.md        # Technical architecture
└── logs/                          # Processing logs
    ├── llpg_preprocessing.log
    ├── source_preprocessing.log
    ├── component_matching.log
    ├── baseline_stats.txt
    ├── final_stats.txt
    └── accuracy_report.txt
```

## Success Criteria

### ✅ Deployment Success Indicators
1. **All addresses preprocessed**: 201,357 total (100%)
2. **Zero processing errors**: Error count = 0
3. **Improved match rate**: >20% improvement over baseline
4. **Performance targets met**: <5 minutes total processing time
5. **Quality validation passed**: Component extraction >95% accurate

### 📊 Expected Final Metrics
- **Total addresses processed**: 201,357
- **Processing time**: ~300 seconds
- **Match rate improvement**: 10% → 25% (150% increase)
- **Auto-accept matches**: ~50,000 addresses
- **Manual review queue**: ~15,000 addresses
- **System reliability**: 99.9% uptime during processing

## Rollback Procedure

If issues occur during deployment:

```sql
-- Reset preprocessing flags
UPDATE dim_address SET gopostal_processed = FALSE WHERE address_id > [last_good_id];
UPDATE src_document SET gopostal_processed = FALSE WHERE document_id > [last_good_id];

-- Clear partial matching results  
DELETE FROM address_match WHERE matched_at > '[deployment_start_time]';

-- Restore from backup if needed
-- pg_restore -h localhost -p 15435 -U postgres -d ehdc_llpg [backup_file]
```

## Post-Deployment Validation

### Final Verification Checklist
- [ ] All 71,656 LLPG addresses processed
- [ ] All 129,701 source documents processed  
- [ ] Component-based matching completed
- [ ] Accuracy metrics generated
- [ ] Performance benchmarks met
- [ ] Quality assurance passed
- [ ] Documentation updated
- [ ] System ready for production use

---

**Document Version**: 1.0  
**Last Updated**: 2025-08-19  
**Author**: Claude Code Assistant  
**Review Status**: Ready for deployment