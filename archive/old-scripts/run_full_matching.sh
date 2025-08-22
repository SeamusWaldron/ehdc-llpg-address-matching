#!/bin/bash

# EHDC LLPG Full Matching Process
# This script runs all matching algorithms in sequence

echo "=== EHDC LLPG Full Address Matching Process ==="
echo "Started at: $(date)"
echo ""

# Ensure we're in the correct directory
cd "$(dirname "$0")"

# Build the application (in case there were updates)
echo "Building matcher..."
go build -o bin/matcher cmd/matcher/main.go
if [ $? -ne 0 ]; then
    echo "Build failed!"
    exit 1
fi

# Test database connection
echo "Testing database connection..."
./bin/matcher ping
if [ $? -ne 0 ]; then
    echo "Database connection failed!"
    exit 1
fi

echo ""
echo "=== Starting Matching Process ==="
echo ""

# Stage 1: Deterministic Matching
echo "Stage 1: Deterministic Matching"
echo "--------------------------------"
./bin/matcher match deterministic --batch-size 1000 --label "full-run-deterministic"
if [ $? -ne 0 ]; then
    echo "Deterministic matching failed!"
    exit 1
fi

# Stage 2: Fuzzy Matching
echo ""
echo "Stage 2: Fuzzy Matching"
echo "-----------------------"
./bin/matcher match fuzzy --batch-size 1000 --min-similarity 0.75 --label "full-run-fuzzy"
if [ $? -ne 0 ]; then
    echo "Fuzzy matching failed!"
    exit 1
fi

# Stage 3: Spatial Matching
echo ""
echo "Stage 3: Spatial Proximity Matching"
echo "------------------------------------"
./bin/matcher match spatial --batch-size 500 --max-distance 100 --label "full-run-spatial"
if [ $? -ne 0 ]; then
    echo "Spatial matching failed!"
    exit 1
fi

# Stage 4: Hierarchical Matching
echo ""
echo "Stage 4: Hierarchical Component Matching"
echo "----------------------------------------"
./bin/matcher match hierarchical --batch-size 500 --label "full-run-hierarchical"
if [ $? -ne 0 ]; then
    echo "Hierarchical matching failed!"
    exit 1
fi

# Stage 5: Rule-Based Matching
echo ""
echo "Stage 5: Rule-Based Pattern Matching"
echo "------------------------------------"
./bin/matcher match rule-based --batch-size 1000 --label "full-run-rules"
if [ $? -ne 0 ]; then
    echo "Rule-based matching failed!"
    exit 1
fi

# Stage 6: Vector/Semantic Matching
echo ""
echo "Stage 6: Vector/Semantic Matching"
echo "---------------------------------"
./bin/matcher match vector --batch-size 100 --min-similarity 0.70 --label "full-run-vector"
if [ $? -ne 0 ]; then
    echo "Vector matching failed!"
    exit 1
fi

# Export Results
echo ""
echo "=== Exporting Results ==="
echo "========================="
./bin/matcher match export --output export
if [ $? -ne 0 ]; then
    echo "Export failed!"
    exit 1
fi

# Show final statistics
echo ""
echo "=== Final Statistics ==="
echo "========================"
./bin/matcher match export --stats

echo ""
echo "=== Matching Process Complete ==="
echo "Completed at: $(date)"
echo ""
echo "Results available in:"
echo "  - CSV files: export/ directory"
echo "  - Database views: v_enhanced_source_documents"
echo ""
echo "Next steps:"
echo "  1. Review results in Excel: export/*.csv"
echo "  2. Query database: SELECT * FROM v_match_summary_by_type;"
echo "  3. Manual review: ./bin/matcher match review"