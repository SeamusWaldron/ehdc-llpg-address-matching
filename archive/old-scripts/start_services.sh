#!/bin/bash

# EHDC LLPG Quick Start Script
# Builds and starts the web interface with optimized configuration

set -e

# Load environment variables if .env exists
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Color output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}=== EHDC LLPG System Startup ===${NC}"
echo

# Check for .env file
if [ ! -f .env ]; then
    echo -e "${YELLOW}Warning: No .env file found. Using defaults.${NC}"
    echo "Consider creating a .env file for custom configuration."
    echo
fi

# Build binaries
echo -e "${YELLOW}Building applications...${NC}"

# Build matcher
if ! go build -o matcher ./cmd/matcher; then
    echo -e "${RED}Failed to build matcher${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Matcher built successfully${NC}"

# Build web server  
if ! go build -o web ./cmd/web; then
    echo -e "${RED}Failed to build web server${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Web server built successfully${NC}"

# Test database connection
echo -e "${YELLOW}Testing database connection...${NC}"
if ! ./matcher ping > /dev/null 2>&1; then
    echo -e "${RED}Error: Cannot connect to database${NC}"
    echo "Please check your database configuration in .env file:"
    echo "  DB_HOST=${DB_HOST:-localhost}"
    echo "  DB_PORT=${DB_PORT:-5432}" 
    echo "  DB_NAME=${DB_NAME:-ehdc_llpg}"
    echo "  DB_USER=${DB_USER:-postgres}"
    exit 1
fi
echo -e "${GREEN}✓ Database connection successful${NC}"

# Get configuration
WEB_PORT=${WEB_PORT:-8443}
WEB_HOST=${WEB_HOST:-localhost}

echo
echo -e "${BLUE}Configuration:${NC}"
echo "  • Web Interface: http://${WEB_HOST}:${WEB_PORT}"
echo "  • Database: ${DB_NAME:-ehdc_llpg}@${DB_HOST:-localhost}:${DB_PORT:-5432}"
echo "  • Features Enabled:"
echo "    - Export: ${ENABLE_EXPORT:-true}"
echo "    - Manual Override: ${ENABLE_MANUAL_OVERRIDE:-true}" 
echo "    - Real-time Updates: ${ENABLE_REALTIME_UPDATES:-true}"
echo "    - Audit Logging: ${ENABLE_AUDIT_LOGGING:-true}"
echo

# Run database migrations
echo -e "${YELLOW}Running database migrations...${NC}"
if ls db/migrations/*.sql 1> /dev/null 2>&1; then
    echo "Found migration files. Please run manually if needed:"
    echo "  psql -d ${DB_NAME:-ehdc_llpg} -f db/migrations/008_create_audit_tables.sql"
fi

# Check if optimized matching script exists
if [ -f "scripts/run_optimized_matching.sh" ]; then
    echo
    echo -e "${YELLOW}Matching script available:${NC}"
    echo "  To run optimized matching: ./scripts/run_optimized_matching.sh"
    echo
fi

# Start web server
echo -e "${BLUE}Starting web server...${NC}"
echo "Press Ctrl+C to stop the server"
echo
./web