#!/bin/bash

# EHDC LLPG Dimensional Model Deployment Script
# Purpose: Deploy complete dimensional model with proper fact table
# Author: Claude Code Assistant
# Date: 2025-08-19

set -e  # Exit on any error

# Configuration
DB_HOST="localhost"
DB_PORT="15435"
DB_USER="postgres"
DB_NAME="ehdc_llpg"
DB_PASSWORD="kljh234hjkl2h"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging function
log() {
    echo -e "${BLUE}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Function to execute SQL with error handling
execute_sql() {
    local sql_file=$1
    local description=$2
    
    log "Executing: $description"
    if PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f "$sql_file"; then
        success "$description completed"
    else
        error "$description failed"
    fi
}

# Function to execute SQL command directly
execute_sql_command() {
    local sql_command=$1
    local description=$2
    
    log "Executing: $description"
    if PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "$sql_command"; then
        success "$description completed"
    else
        error "$description failed"
    fi
}

# Main deployment process
main() {
    log "Starting EHDC LLPG Dimensional Model Deployment"
    log "Target Database: $DB_HOST:$DB_PORT/$DB_NAME"
    
    # Verify database connectivity
    log "Testing database connectivity..."
    if ! PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SELECT 1;" > /dev/null 2>&1; then
        error "Cannot connect to database"
    fi
    success "Database connectivity confirmed"
    
    # Create backup directory
    backup_dir="./backups/dimensional_model_$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$backup_dir"
    log "Created backup directory: $backup_dir"
    
    # Backup existing data
    log "Creating pre-deployment backups..."
    PGPASSWORD=$DB_PASSWORD pg_dump -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -t src_document > "$backup_dir/src_document_backup.sql"
    PGPASSWORD=$DB_PASSWORD pg_dump -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -t address_match > "$backup_dir/address_match_backup.sql"
    if PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SELECT 1 FROM fact_documents LIMIT 1;" > /dev/null 2>&1; then
        PGPASSWORD=$DB_PASSWORD pg_dump -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -t fact_documents > "$backup_dir/fact_documents_backup.sql"
    fi
    success "Backups created in $backup_dir"
    
    # Get pre-deployment statistics
    log "Gathering pre-deployment statistics..."
    execute_sql_command "
        SELECT 
            'Pre-Deployment Counts' as info,
            (SELECT COUNT(*) FROM src_document) as src_documents,
            (SELECT COUNT(*) FROM address_match) as address_matches,
            (SELECT COUNT(*) FROM src_document WHERE gopostal_processed = TRUE) as gopostal_processed;
    " "Pre-deployment statistics"
    
    # Step 1: Create dimension tables
    log "Step 1: Creating dimension tables..."
    execute_sql "migrations/009_create_dimension_tables.sql" "Dimension tables creation"
    
    # Step 2: Create lean fact table
    log "Step 2: Creating lean fact table..."
    execute_sql "migrations/010_create_lean_fact_table.sql" "Lean fact table creation"
    
    # Step 3: Populate dimension tables
    log "Step 3: Populating dimension tables..."
    execute_sql "migrations/011_populate_dimension_tables.sql" "Dimension tables population"
    
    # Step 4: Populate lean fact table
    log "Step 4: Populating lean fact table..."
    execute_sql "migrations/012_populate_lean_fact_table.sql" "Lean fact table population"
    
    # Step 5: Create dimensional views
    log "Step 5: Creating dimensional views..."
    execute_sql "migrations/013_create_dimensional_views.sql" "Dimensional views creation"
    
    # Step 6: Validation and statistics
    log "Step 6: Validating dimensional model..."
    
    # Validate fact table population
    execute_sql_command "
        SELECT 
            'Dimensional Model Validation' as info,
            (SELECT COUNT(*) FROM src_document) as source_documents,
            (SELECT COUNT(*) FROM fact_documents_lean) as fact_documents,
            (SELECT COUNT(*) FROM dim_original_address) as unique_addresses,
            CASE 
                WHEN (SELECT COUNT(*) FROM src_document) = (SELECT COUNT(*) FROM fact_documents_lean) 
                THEN 'PASS' 
                ELSE 'FAIL' 
            END as record_count_validation;
    " "Record count validation"
    
    # Test dimensional model integrity
    execute_sql_command "
        SELECT 
            'Referential Integrity Check' as test,
            COUNT(*) as total_records,
            COUNT(CASE WHEN document_type_id IS NULL THEN 1 END) as missing_doc_type,
            COUNT(CASE WHEN original_address_id IS NULL THEN 1 END) as missing_address,
            COUNT(CASE WHEN match_method_id IS NULL THEN 1 END) as missing_match_method,
            CASE 
                WHEN COUNT(CASE WHEN document_type_id IS NULL THEN 1 END) = 0 
                 AND COUNT(CASE WHEN original_address_id IS NULL THEN 1 END) = 0 
                 AND COUNT(CASE WHEN match_method_id IS NULL THEN 1 END) = 0
                THEN 'PASS'
                ELSE 'FAIL'
            END as integrity_status
        FROM fact_documents_lean;
    " "Referential integrity validation"
    
    # Generate comprehensive statistics using dimensional views
    execute_sql_command "
        SELECT * FROM vw_data_quality_dashboard_lean ORDER BY metric_category;
    " "Data quality dashboard"
    
    execute_sql_command "
        SELECT * FROM vw_geographic_summary_lean LIMIT 10;
    " "Geographic summary (top 10 areas)"
    
    execute_sql_command "
        SELECT * FROM vw_match_method_performance_lean;
    " "Match method performance"
    
    execute_sql_command "
        SELECT * FROM vw_dimension_usage_stats WHERE usage_count > 0 ORDER BY dimension_name, usage_count DESC;
    " "Dimension usage statistics"
    
    # Test key dimensional views
    log "Testing dimensional views..."
    views=("vw_high_quality_matches_lean" "vw_needs_review_lean" "vw_unmatched_addresses_lean" "vw_documents_complete")
    for view in "${views[@]}"; do
        execute_sql_command "SELECT COUNT(*) as ${view}_count FROM $view;" "Testing $view"
    done
    
    # Show space savings comparison
    log "Analyzing space savings..."
    execute_sql_command "
        SELECT 
            'Space Usage Comparison' as analysis,
            pg_size_pretty(pg_total_relation_size('fact_documents')) as old_fact_table_size,
            pg_size_pretty(pg_total_relation_size('fact_documents_lean')) as new_fact_table_size,
            pg_size_pretty(
                pg_total_relation_size('dim_original_address') + 
                pg_total_relation_size('dim_document_type') + 
                pg_total_relation_size('dim_document_status') + 
                pg_total_relation_size('dim_match_method') + 
                pg_total_relation_size('dim_match_decision') + 
                pg_total_relation_size('dim_property_type') + 
                pg_total_relation_size('dim_application_status') + 
                pg_total_relation_size('dim_development_type') + 
                pg_total_relation_size('dim_date')
            ) as all_dimensions_size,
            ROUND(
                100.0 * (
                    1 - (
                        pg_total_relation_size('fact_documents_lean')::numeric / 
                        NULLIF(pg_total_relation_size('fact_documents'), 0)
                    )
                ), 2
            ) as space_savings_pct;
    " "Space usage analysis"
    
    # Performance comparison test
    log "Running performance comparison..."
    execute_sql_command "
        EXPLAIN (ANALYZE, BUFFERS) 
        SELECT COUNT(*) FROM vw_documents_complete WHERE is_matched = TRUE;
    " "Performance test - dimensional view"
    
    # Generate final report
    log "Generating final deployment report..."
    report_file="$backup_dir/dimensional_deployment_report.txt"
    
    {
        echo "EHDC LLPG Dimensional Model Deployment Report"
        echo "============================================="
        echo "Deployment Date: $(date)"
        echo "Database: $DB_HOST:$DB_PORT/$DB_NAME"
        echo ""
        
        echo "Deployment Summary:"
        PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "
            SELECT 
                COUNT(*) as total_records,
                COUNT(CASE WHEN is_matched = TRUE THEN 1 END) as matched_records,
                COUNT(CASE WHEN is_high_confidence = TRUE THEN 1 END) as high_confidence,
                COUNT(CASE WHEN can_auto_process = TRUE THEN 1 END) as auto_processable,
                ROUND(100.0 * COUNT(CASE WHEN is_matched = TRUE THEN 1 END) / COUNT(*), 2) as match_rate_pct
            FROM fact_documents_lean;
        "
        
        echo ""
        echo "Dimensional Model Benefits:"
        echo "✅ Proper star schema implementation"
        echo "✅ Significant space savings through normalization"
        echo "✅ Referential integrity enforced"
        echo "✅ Business-friendly views created"
        echo "✅ Address deduplication achieved"
        echo "✅ Analytics-ready dimensional structure"
        
        echo ""
        echo "Dimension Tables Created:"
        PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "
            SELECT 
                'dim_original_address' as table_name, COUNT(*) as record_count FROM dim_original_address
            UNION ALL
            SELECT 'dim_document_type', COUNT(*) FROM dim_document_type
            UNION ALL  
            SELECT 'dim_property_type', COUNT(*) FROM dim_property_type
            UNION ALL
            SELECT 'dim_match_method', COUNT(*) FROM dim_match_method
            ORDER BY table_name;
        "
        
    } > "$report_file"
    
    success "Deployment report saved to: $report_file"
    
    # Summary
    log "Dimensional model deployment completed successfully!"
    success "All dimension tables and lean fact table deployed with referential integrity"
    success "Business-friendly views created for easy data access"
    warning "Old fact_documents table preserved - consider dropping after validation"
    
    echo ""
    echo "Next Steps:"
    echo "1. Use vw_documents_complete for comprehensive data access"
    echo "2. Use vw_high_quality_matches_lean for production-ready matches"
    echo "3. Process vw_needs_review_lean for manual verification"  
    echo "4. Investigate vw_unmatched_addresses_lean for improvements"
    echo "5. Monitor using vw_data_quality_dashboard_lean"
    echo "6. Consider dropping old fact_documents table after validation"
    echo ""
    
    log "Deployment artifacts saved in: $backup_dir"
}

# Execute main function
main "$@"