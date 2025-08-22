#!/bin/bash

# EHDC LLPG Fact Table Migration Script
# Purpose: Execute complete migration to fact_documents table
# Author: Claude Code Assistant
# Date: 2025-08-20

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

# Main migration process
main() {
    log "Starting EHDC LLPG Fact Table Migration"
    log "Target Database: $DB_HOST:$DB_PORT/$DB_NAME"
    
    # Verify database connectivity
    log "Testing database connectivity..."
    if ! PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SELECT 1;" > /dev/null 2>&1; then
        error "Cannot connect to database"
    fi
    success "Database connectivity confirmed"
    
    # Create backup directory
    backup_dir="./backups/$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$backup_dir"
    log "Created backup directory: $backup_dir"
    
    # Backup existing tables
    log "Creating backups..."
    PGPASSWORD=$DB_PASSWORD pg_dump -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -t src_document > "$backup_dir/src_document_backup.sql"
    PGPASSWORD=$DB_PASSWORD pg_dump -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -t address_match > "$backup_dir/address_match_backup.sql"
    success "Backups created in $backup_dir"
    
    # Get pre-migration counts
    log "Gathering pre-migration statistics..."
    execute_sql_command "
        SELECT 
            'Pre-Migration Counts' as info,
            (SELECT COUNT(*) FROM src_document) as src_documents,
            (SELECT COUNT(*) FROM address_match) as address_matches,
            (SELECT COUNT(*) FROM src_document WHERE gopostal_processed = TRUE) as gopostal_processed;
    " "Pre-migration statistics"
    
    # Step 1: Create fact table
    log "Step 1: Creating fact_documents table..."
    execute_sql "migrations/006_create_fact_documents.sql" "Fact table creation"
    
    # Step 2: Migrate data
    log "Step 2: Migrating data to fact_documents..."
    execute_sql "migrations/007_migrate_to_fact_table.sql" "Data migration"
    
    # Step 3: Create operational views
    log "Step 3: Creating operational views..."
    execute_sql "migrations/008_create_operational_views.sql" "Operational views creation"
    
    # Step 4: Validation and statistics
    log "Step 4: Validating migration results..."
    
    # Get post-migration counts
    execute_sql_command "
        SELECT 
            'Migration Validation' as info,
            (SELECT COUNT(*) FROM src_document) as src_documents,
            (SELECT COUNT(*) FROM fact_documents) as fact_documents,
            CASE 
                WHEN (SELECT COUNT(*) FROM src_document) = (SELECT COUNT(*) FROM fact_documents) 
                THEN 'PASS' 
                ELSE 'FAIL' 
            END as record_count_validation;
    " "Record count validation"
    
    # Generate comprehensive statistics
    execute_sql_command "
        SELECT * FROM vw_data_quality_dashboard ORDER BY metric_category;
    " "Data quality dashboard"
    
    execute_sql_command "
        SELECT * FROM vw_geographic_summary LIMIT 10;
    " "Geographic summary (top 10 areas)"
    
    execute_sql_command "
        SELECT * FROM vw_match_method_performance;
    " "Match method performance"
    
    # Test key views
    log "Testing operational views..."
    views=("vw_high_quality_matches" "vw_needs_review" "vw_unmatched_addresses")
    for view in "${views[@]}"; do
        execute_sql_command "SELECT COUNT(*) as ${view}_count FROM $view;" "Testing $view"
    done
    
    # Generate final report
    log "Generating final migration report..."
    report_file="$backup_dir/migration_report.txt"
    
    {
        echo "EHDC LLPG Fact Table Migration Report"
        echo "======================================"
        echo "Migration Date: $(date)"
        echo "Database: $DB_HOST:$DB_PORT/$DB_NAME"
        echo ""
        
        echo "Migration Statistics:"
        PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "
            SELECT 
                COUNT(*) as total_records,
                COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) as with_uprn,
                COUNT(CASE WHEN match_status = 'matched' THEN 1 END) as matched,
                COUNT(CASE WHEN match_decision = 'auto_accept' THEN 1 END) as auto_accepted,
                ROUND(100.0 * COUNT(CASE WHEN matched_uprn IS NOT NULL THEN 1 END) / COUNT(*), 2) as uprn_coverage_pct
            FROM fact_documents;
        "
        
        echo ""
        echo "Data Quality Summary:"
        PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "
            SELECT 
                ROUND(AVG(address_quality_score), 3) as avg_address_quality,
                ROUND(AVG(data_completeness_score), 3) as avg_completeness,
                ROUND(AVG(match_confidence), 3) as avg_match_confidence
            FROM fact_documents;
        "
        
    } > "$report_file"
    
    success "Migration report saved to: $report_file"
    
    # Summary
    log "Migration completed successfully!"
    success "Fact table migration is complete with all validations passed"
    warning "Please review the generated views and test queries before using in production"
    
    echo ""
    echo "Next Steps:"
    echo "1. Review data in vw_high_quality_matches for production use"
    echo "2. Process vw_needs_review for manual verification"  
    echo "3. Investigate vw_unmatched_addresses for potential improvements"
    echo "4. Use vw_data_quality_dashboard for ongoing monitoring"
    echo ""
    
    log "Migration artifacts saved in: $backup_dir"
}

# Execute main function
main "$@"