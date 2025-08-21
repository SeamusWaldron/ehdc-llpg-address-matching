# EHDC LLPG Quick Start Guide

## 🚀 Immediate Actions Required

### 1. Stop Current Matcher
If you have a matcher running with high thresholds (80%+), stop it immediately:
```bash
# Press Ctrl+C to stop the current matcher
```

### 2. Quick Start
```bash
# Start the system with optimized settings
./start_services.sh
```

**Web Interface URL: http://localhost:8443**

### 3. Run Optimized Matching
```bash
# Run all matching stages with research-backed thresholds
./scripts/run_optimized_matching.sh
```

## 📊 What Changed

### Optimized Thresholds
- **OLD (problematic):** 80% minimum threshold → Zero matches
- **NEW (optimized):** 60% minimum threshold → Broader matching

| Confidence Level | Threshold | Action |
|------------------|-----------|--------|
| High Confidence | ≥85% | Auto-accept |
| Medium Confidence | ≥78% | Auto-accept with validation |
| Low Confidence | ≥70% | Flag for manual review |
| **Minimum** | **≥60%** | **Consider for matching** |
| Below 60% | <60% | Reject |

### Port Configuration
- **Web Interface:** Port 8443 (instead of 8080)
- **Database:** Standard PostgreSQL ports
- All configurable via `.env` file

## 🔧 Configuration

### Environment File (.env)
The system now uses a `.env` file for configuration:

```env
# Web Interface
WEB_PORT=8443
WEB_HOST=localhost

# Database  
DB_HOST=localhost
DB_PORT=5432
DB_NAME=ehdc_llpg
DB_USER=postgres
DB_PASSWORD=postgres

# Matching Thresholds (research-optimized)
MATCH_MIN_THRESHOLD=0.60
MATCH_HIGH_CONFIDENCE=0.85
MATCH_MEDIUM_CONFIDENCE=0.78
MATCH_LOW_CONFIDENCE=0.70

# Performance
MATCH_WORKERS=8
MATCH_BATCH_SIZE=1000
```

## 📈 Expected Results

With the new optimized thresholds:
- **Immediate matches** instead of zero results
- **Balanced precision/recall** based on research analysis
- **Multi-stage matching** for comprehensive coverage

### Matching Stages
1. **Deterministic** - Exact matches (UPRN validation, canonical addresses)
2. **Fuzzy Optimized** - Similarity-based with parallel processing
3. **Postcode Proximity** - Geographic proximity within postcodes  
4. **Spatial Matching** - Coordinate-based proximity

## 🖥️ Web Interface Features

Access at **http://localhost:8443**:
- Interactive map with address data
- Advanced filtering and selection tools
- Real-time statistics and updates
- Record detail drawer with full audit history
- Match candidate review and acceptance
- Manual coordinate override capabilities
- Comprehensive export functionality

## 🚨 Why This Fixes the Zero Matches Issue

The previous configuration used an **80% similarity threshold**, which is:
- Too restrictive for real-world address variations
- Designed for high-precision scenarios
- Inappropriate for initial broad matching

The new **60% threshold**:
- Captures address variations (abbreviations, formatting differences)
- Allows fuzzy matching to work as intended
- Provides candidates for manual review
- Based on empirical threshold analysis

## 🔍 Monitoring Progress

After starting the optimized matching:
```bash
# View progress in real-time
tail -f /var/log/matcher.log

# Check web interface for live statistics
open http://localhost:8443
```

You should see matches appearing within the first few thousand records processed.

## 📞 Next Steps

1. **Start the services:** `./start_services.sh`
2. **Run matching:** `./scripts/run_optimized_matching.sh` 
3. **Monitor via web interface:** http://localhost:8443
4. **Review results and adjust as needed**

The system is now configured for optimal performance with research-backed thresholds that should produce matches immediately.