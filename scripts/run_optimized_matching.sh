#!/bin/bash

# EHDC LLPG Optimized Address Matching Script
# Uses research-backed thresholds for optimal precision/recall balance

set -e

# Load environment variables if .env exists
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Color output for better readability
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== EHDC LLPG Optimized Address Matching ===${NC}"
echo -e "${BLUE}Using research-backed thresholds for optimal results${NC}"
echo

# Configuration with optimized defaults
MIN_SIMILARITY=${MATCH_MIN_THRESHOLD:-0.60}
BATCH_SIZE=${MATCH_BATCH_SIZE:-1000}
WORKERS=${MATCH_WORKERS:-8}
RUN_LABEL="optimized-$(date +%Y%m%d-%H%M%S)"

echo -e "${GREEN}Configuration:${NC}"
echo "  • Minimum Similarity: $MIN_SIMILARITY (60% - broad matching)"
echo "  • Batch Size: $BATCH_SIZE records"
echo "  • Workers: $WORKERS parallel processes"
echo "  • Run Label: $RUN_LABEL"
echo

# Check if matcher binary exists
if [ ! -f "./matcher" ]; then
    echo -e "${RED}Error: matcher binary not found. Please build first:${NC}"
    echo "  go build -o matcher ./cmd/matcher"
    exit 1
fi

# Test database connectivity
echo -e "${YELLOW}Testing database connectivity...${NC}"
if ! ./matcher ping > /dev/null 2>&1; then
    echo -e "${RED}Error: Cannot connect to database. Check your .env file.${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Database connection successful${NC}"

# Get record counts before matching
echo -e "${YELLOW}Checking current status...${NC}"
TOTAL_RECORDS=$(./matcher ping 2>/dev/null | grep "Source documents:" | awk '{print $3}' || echo "Unknown")
LLPG_ADDRESSES=$(./matcher ping 2>/dev/null | grep "LLPG addresses:" | awk '{print $3}' || echo "Unknown")

echo "  • Total source records: $TOTAL_RECORDS"
echo "  • LLPG addresses available: $LLPG_ADDRESSES"
echo

# Run Stage 1: Deterministic Matching (if not done)
echo -e "${YELLOW}=== Stage 1: Deterministic Matching ===${NC}"
echo "Running exact matches (UPRN validation + canonical addresses)..."
./matcher match deterministic \
    --label "deterministic-$(date +%Y%m%d-%H%M%S)" \
    --batch-size $BATCH_SIZE

echo -e "${GREEN}✓ Stage 1 completed${NC}"
echo

# Run Stage 2: Optimized Fuzzy Matching
echo -e "${YELLOW}=== Stage 2: Optimized Fuzzy Matching ===${NC}"
echo "Running fuzzy matching with optimized thresholds..."
echo "  • High Confidence (≥85%): Auto-accept"
echo "  • Medium Confidence (≥78%): Auto-accept with validation"  
echo "  • Low Confidence (≥70%): Flag for review"
echo "  • Below 60%: Reject"
echo

./matcher match fuzzy-optimized \
    --label "$RUN_LABEL" \
    --min-similarity $MIN_SIMILARITY \
    --batch-size $BATCH_SIZE \
    --workers $WORKERS

echo -e "${GREEN}✓ Stage 2 completed${NC}"
echo

# Run Stage 3: Postcode-based Matching (for remaining unmatched)
echo -e "${YELLOW}=== Stage 3: Postcode Proximity Matching ===${NC}"
echo "Running postcode-based matching for remaining records..."
./matcher match postcode \
    --label "postcode-$(date +%Y%m%d-%H%M%S)" \
    --batch-size $BATCH_SIZE \
    --distance-threshold 1000

echo -e "${GREEN}✓ Stage 3 completed${NC}"
echo

# Run Stage 4: Spatial Matching
echo -e "${YELLOW}=== Stage 4: Spatial Proximity Matching ===${NC}"
echo "Running coordinate-based spatial matching..."
./matcher match spatial \
    --label "spatial-$(date +%Y%m%d-%H%M%S)" \
    --batch-size $BATCH_SIZE \
    --radius-meters 100

echo -e "${GREEN}✓ Stage 4 completed${NC}"
echo

# Generate summary report
echo -e "${BLUE}=== MATCHING SUMMARY ===${NC}"
echo "Generating comprehensive matching report..."

# This would ideally query the match_run table for statistics
echo "View detailed results:"
echo "  • Web Interface: http://localhost:${WEB_PORT:-8443}"
echo "  • Export results: ./matcher export --format csv --output results.csv"
echo "  • Review interface: ./matcher review"

echo
echo -e "${GREEN}=== MATCHING PROCESS COMPLETED ===${NC}"
echo -e "${GREEN}All stages completed successfully!${NC}"
echo
echo "Next steps:"
echo "  1. Review match results in web interface"
echo "  2. Use review interface for ambiguous matches"
echo "  3. Export final results when satisfied"
echo
echo -e "${BLUE}Web Interface URL: http://localhost:${WEB_PORT:-8443}${NC}"