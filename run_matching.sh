#!/bin/bash

# EHDC LLPG Address Matching Runner Script
# Based on threshold tuning results:
# Option 1: 0.80 threshold = 100% precision, 85% recall (Conservative)
# Option 2: 0.75 threshold = 90% precision, 100% recall (Balanced) - DEFAULT
# Option 3: 0.60 threshold = 94% precision, 100% recall (Aggressive)

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values based on tuning results
THRESHOLD_OPTION="2"  # Default to Option 2 (Balanced)
BATCH_SIZE=1000
WORKERS=4
RUN_DETERMINISTIC=true
RUN_FUZZY=false
RUN_OPTIMIZED=true
RUN_POSTCODE=true
LABEL_PREFIX="production"

# Function to display help
show_help() {
    echo "EHDC LLPG Address Matching Runner"
    echo ""
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Threshold Options (based on tuning results):"
    echo "  -t, --threshold <1|2|3>    Select threshold option (default: 2)"
    echo "                              1 = Conservative (0.80): 100% precision, 85% recall"
    echo "                              2 = Balanced (0.75): 90% precision, 100% recall [DEFAULT]"
    echo "                              3 = Aggressive (0.60): 94% precision, 100% recall"
    echo ""
    echo "Processing Options:"
    echo "  -b, --batch-size <size>    Batch size for processing (default: 1000)"
    echo "  -w, --workers <count>      Number of parallel workers (default: 4)"
    echo "  -l, --label <prefix>       Label prefix for runs (default: production)"
    echo ""
    echo "Stage Selection:"
    echo "  --deterministic-only       Run only deterministic matching"
    echo "  --fuzzy-only              Run only fuzzy matching"
    echo "  --postcode-only           Run only postcode matching"
    echo "  --skip-deterministic      Skip deterministic matching"
    echo "  --skip-fuzzy              Skip standard fuzzy matching"
    echo "  --skip-optimized          Skip optimized fuzzy matching"
    echo "  --skip-postcode           Skip postcode matching"
    echo ""
    echo "Other Options:"
    echo "  -h, --help                Show this help message"
    echo "  -d, --dry-run             Show what would be run without executing"
    echo "  -s, --stats               Show current matching statistics"
    echo ""
    echo "Examples:"
    echo "  $0                        # Run with default balanced settings"
    echo "  $0 -t 1                   # Run with conservative settings"
    echo "  $0 -t 3 -w 8              # Run aggressive matching with 8 workers"
    echo "  $0 --deterministic-only   # Run only deterministic matching"
    echo ""
}

# Function to show current statistics
show_stats() {
    echo -e "${BLUE}=== Current Matching Statistics ===${NC}"
    
    # Use psql to get stats
    PGPASSWORD=password psql -h localhost -p 15432 -U user -d ehdc_gis -t <<EOF
SELECT 
    'Total Documents: ' || COUNT(*) 
FROM src_document;

SELECT 
    'Documents with Accepted Matches: ' || COUNT(DISTINCT src_id)
FROM match_accepted;

SELECT 
    'Unmatched Documents: ' || COUNT(*)
FROM src_document s
LEFT JOIN match_accepted m ON m.src_id = s.src_id
WHERE m.src_id IS NULL;

SELECT 
    'Documents Needing Review: ' || COUNT(DISTINCT src_id)
FROM match_result
WHERE decision = 'needs_review'
  AND src_id NOT IN (SELECT src_id FROM match_accepted);

SELECT 
    'Coverage by Source Type:' as info
UNION ALL
SELECT 
    '  ' || source_type || ': ' || 
    ROUND(100.0 * SUM(CASE WHEN m.src_id IS NOT NULL THEN 1 ELSE 0 END) / COUNT(*), 2) || '%'
FROM src_document s
LEFT JOIN match_accepted m ON m.src_id = s.src_id
GROUP BY source_type;
EOF
}

# Parse command line arguments
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--threshold)
            THRESHOLD_OPTION="$2"
            shift 2
            ;;
        -b|--batch-size)
            BATCH_SIZE="$2"
            shift 2
            ;;
        -w|--workers)
            WORKERS="$2"
            shift 2
            ;;
        -l|--label)
            LABEL_PREFIX="$2"
            shift 2
            ;;
        --deterministic-only)
            RUN_DETERMINISTIC=true
            RUN_FUZZY=false
            RUN_OPTIMIZED=false
            shift
            ;;
        --fuzzy-only)
            RUN_DETERMINISTIC=false
            RUN_FUZZY=true
            RUN_OPTIMIZED=true
            RUN_POSTCODE=false
            shift
            ;;
        --postcode-only)
            RUN_DETERMINISTIC=false
            RUN_FUZZY=false
            RUN_OPTIMIZED=false
            RUN_POSTCODE=true
            shift
            ;;
        --skip-deterministic)
            RUN_DETERMINISTIC=false
            shift
            ;;
        --skip-fuzzy)
            RUN_FUZZY=false
            shift
            ;;
        --skip-optimized)
            RUN_OPTIMIZED=false
            shift
            ;;
        --skip-postcode)
            RUN_POSTCODE=false
            shift
            ;;
        -d|--dry-run)
            DRY_RUN=true
            shift
            ;;
        -s|--stats)
            show_stats
            exit 0
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            exit 1
            ;;
    esac
done

# Set threshold based on option
case $THRESHOLD_OPTION in
    1)
        THRESHOLD=0.80
        THRESHOLD_NAME="Conservative"
        echo -e "${YELLOW}Using Conservative threshold (0.80): 100% precision, 85% recall${NC}"
        ;;
    2)
        THRESHOLD=0.75
        THRESHOLD_NAME="Balanced"
        echo -e "${GREEN}Using Balanced threshold (0.75): 90% precision, 100% recall [DEFAULT]${NC}"
        ;;
    3)
        THRESHOLD=0.60
        THRESHOLD_NAME="Aggressive"
        echo -e "${YELLOW}Using Aggressive threshold (0.60): 94% precision, 100% recall${NC}"
        ;;
    *)
        echo -e "${RED}Invalid threshold option: $THRESHOLD_OPTION${NC}"
        echo "Please use 1, 2, or 3"
        exit 1
        ;;
esac

# Create timestamp for labels
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Build the matcher if needed
if [ ! -f "./bin/matcher" ]; then
    echo -e "${BLUE}Building matcher...${NC}"
    go build -o bin/matcher cmd/matcher/main.go
fi

# Function to run a command
run_command() {
    local cmd="$1"
    local description="$2"
    
    echo -e "${BLUE}Running: $description${NC}"
    echo -e "${YELLOW}Command: $cmd${NC}"
    
    if [ "$DRY_RUN" = true ]; then
        echo -e "${YELLOW}[DRY RUN] Would execute: $cmd${NC}"
    else
        eval "$cmd"
    fi
    echo ""
}

# Show configuration
echo -e "${BLUE}=== Matching Configuration ===${NC}"
echo "Threshold: $THRESHOLD ($THRESHOLD_NAME)"
echo "Batch Size: $BATCH_SIZE"
echo "Workers: $WORKERS"
echo "Label Prefix: $LABEL_PREFIX"
echo "Timestamp: $TIMESTAMP"
echo ""

# Run deterministic matching
if [ "$RUN_DETERMINISTIC" = true ]; then
    DETERMINISTIC_LABEL="${LABEL_PREFIX}_deterministic_${TIMESTAMP}"
    run_command "./bin/matcher match deterministic --label '$DETERMINISTIC_LABEL' --batch-size $BATCH_SIZE" \
                "Stage 1: Deterministic Matching"
fi

# Run standard fuzzy matching
if [ "$RUN_FUZZY" = true ]; then
    FUZZY_LABEL="${LABEL_PREFIX}_fuzzy_${THRESHOLD_NAME}_${TIMESTAMP}"
    run_command "./bin/matcher match fuzzy --label '$FUZZY_LABEL' --batch-size $BATCH_SIZE --min-similarity $THRESHOLD" \
                "Stage 2: Standard Fuzzy Matching (threshold=$THRESHOLD)"
fi

# Run optimized fuzzy matching
if [ "$RUN_OPTIMIZED" = true ]; then
    OPTIMIZED_LABEL="${LABEL_PREFIX}_optimized_${THRESHOLD_NAME}_${TIMESTAMP}"
    run_command "./bin/matcher match fuzzy-optimized --label '$OPTIMIZED_LABEL' --batch-size $BATCH_SIZE --min-similarity $THRESHOLD --workers $WORKERS" \
                "Stage 2: Optimized Fuzzy Matching (threshold=$THRESHOLD, workers=$WORKERS)"
fi

# Run postcode matching
if [ "$RUN_POSTCODE" = true ]; then
    POSTCODE_LABEL="${LABEL_PREFIX}_postcode_${TIMESTAMP}"
    run_command "./bin/matcher match postcode --label '$POSTCODE_LABEL' --batch-size $BATCH_SIZE" \
                "Stage 3: Postcode-Centric Matching"
fi

# Show final statistics
if [ "$DRY_RUN" = false ]; then
    echo -e "${GREEN}=== Matching Complete ===${NC}"
    show_stats
fi

echo -e "${GREEN}Done!${NC}"