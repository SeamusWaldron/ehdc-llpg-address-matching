#!/bin/bash

# Export CSV files from PostgreSQL views
# Usage: ./export_csv.sh [output_directory]

# Configuration
DB_HOST="localhost"
DB_PORT="15435"
DB_NAME="ehdc_llpg"
DB_USER="postgres"
DB_PASSWORD="kljh234hjkl2h"

# Output directory (default to current directory)
OUTPUT_DIR="${1:-.}"

# Create output directory if it doesn't exist
mkdir -p "$OUTPUT_DIR"

# Export function
export_view_to_csv() {
    local view_name=$1
    local output_file=$2
    local description=$3
    
    echo "Exporting $description..."
    PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME \
        -c "\COPY (SELECT * FROM $view_name) TO '$OUTPUT_DIR/$output_file' WITH CSV HEADER"
    
    if [ $? -eq 0 ]; then
        echo "  ✓ Exported to $output_file"
    else
        echo "  ✗ Failed to export $output_file"
    fi
}

echo "=========================================="
echo "EHDC LLPG CSV Export Tool"
echo "=========================================="
echo "Output directory: $OUTPUT_DIR"
echo ""

# Export individual document type CSVs
export_view_to_csv "vw_csv_export_decision_notices" "decision_notices.csv" "Decision Notices"
export_view_to_csv "vw_csv_export_land_charges" "land_charges.csv" "Land Charges"
export_view_to_csv "vw_csv_export_agreements" "agreements.csv" "Agreements"
export_view_to_csv "vw_csv_export_enforcement_notices" "enforcement_notices.csv" "Enforcement Notices"
export_view_to_csv "vw_csv_export_street_naming" "street_naming.csv" "Street Name and Numbering"

# Export combined and analysis CSVs
export_view_to_csv "vw_csv_export_all_documents" "all_documents.csv" "All Documents Combined"
export_view_to_csv "vw_csv_export_unmatched" "unmatched_documents.csv" "Unmatched Documents"
export_view_to_csv "vw_csv_export_high_confidence" "high_confidence_matches.csv" "High Confidence Matches"

# Export analysis views
export_view_to_csv "vw_data_quality_dashboard_lean" "data_quality_summary.csv" "Data Quality Summary"
export_view_to_csv "vw_match_method_performance_lean" "match_method_performance.csv" "Match Method Performance"
export_view_to_csv "vw_needs_review_lean" "needs_review.csv" "Matches Needing Review"

echo ""
echo "=========================================="
echo "Export complete!"
echo "=========================================="

# Show summary statistics
echo ""
echo "Summary Statistics:"
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -t << EOF
SELECT 
    'Total Documents: ' || COUNT(*) || 
    ' | Matched: ' || COUNT(matched_address_id) ||
    ' | Match Rate: ' || ROUND(COUNT(matched_address_id)::numeric / COUNT(*)::numeric * 100, 1) || '%'
FROM fact_documents_lean;
EOF

# Count lines in exported files
echo ""
echo "Exported file sizes:"
for file in "$OUTPUT_DIR"/*.csv; do
    if [ -f "$file" ]; then
        lines=$(wc -l < "$file")
        filename=$(basename "$file")
        echo "  $filename: $lines lines"
    fi
done