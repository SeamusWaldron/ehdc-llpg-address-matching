#!/bin/bash

# EHDC LLPG Complete System Startup Script
# This script ensures clean startup of all services in the correct order

set -e

echo "🚀 Starting EHDC LLPG Address Matching System..."

# Function to check if a container is running
container_running() {
    docker ps --format '{{.Names}}' | grep -q "^$1$"
}

# Function to wait for service to be ready
wait_for_service() {
    local service_name=$1
    local url=$2
    local max_wait=${3:-60}
    
    echo "⏳ Waiting for $service_name to be ready..."
    for i in $(seq 1 $max_wait); do
        if curl -s "$url" > /dev/null 2>&1; then
            echo "✅ $service_name is ready!"
            return 0
        fi
        sleep 2
    done
    echo "❌ $service_name failed to start within ${max_wait} seconds"
    return 1
}

# Stop any conflicting standalone containers
echo "🧹 Cleaning up standalone containers..."
if container_running "ehdc_ollama_fixed"; then
    echo "Stopping standalone ehdc_ollama_fixed..."
    docker stop ehdc_ollama_fixed
    docker rm ehdc_ollama_fixed
fi

# Start services using docker-compose
echo "🐳 Starting Docker Compose services..."
docker-compose down --remove-orphans
docker-compose up -d postgres qdrant

# Load environment variables
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Set defaults if not provided
DB_PORT=${DB_PORT:-15435}
DB_USER=${DB_USER:-postgres}
DB_PASSWORD=${DB_PASSWORD:-kljh234hjkl2h}
DB_NAME=${DB_NAME:-ehdc_llpg}
OLLAMA_PORT=${OLLAMA_PORT:-11434}
QDRANT_PORT=${QDRANT_PORT:-6333}

# Wait for PostgreSQL to be ready
wait_for_service "PostgreSQL" "postgres://${DB_USER}:${DB_PASSWORD}@localhost:${DB_PORT}/${DB_NAME}" 60

# Install PostGIS extension if needed
echo "📊 Setting up PostGIS extensions..."
PGPASSWORD=${DB_PASSWORD} psql -h localhost -p ${DB_PORT} -U ${DB_USER} -d ${DB_NAME} -c "
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
" || echo "Extensions already installed or failed to install"

# Start Ollama (this will take longer due to model downloads)
echo "🤖 Starting Ollama with model downloads..."
docker-compose up -d ollama

# Wait for Ollama to be ready and models to be pulled
wait_for_service "Ollama" "http://localhost:${OLLAMA_PORT}/api/version" 300

# Verify models are available
echo "🔍 Verifying Ollama models..."
if curl -s http://localhost:${OLLAMA_PORT}/api/tags | grep -q "llama3.2:1b"; then
    echo "✅ llama3.2:1b model is available"
else
    echo "⚠️  llama3.2:1b model not found - will be pulled on first use"
fi

if curl -s http://localhost:${OLLAMA_PORT}/api/tags | grep -q "nomic-embed-text"; then
    echo "✅ nomic-embed-text model is available"
else
    echo "⚠️  nomic-embed-text model not found - will be pulled on first use"
fi

# Start Qdrant (should already be running)
wait_for_service "Qdrant" "http://localhost:${QDRANT_PORT}/health" 30

# Show running services
echo ""
echo "📋 System Status:"
echo "=================="
docker-compose ps

echo ""
echo "🎉 EHDC LLPG System is ready!"
echo ""
echo "📝 Quick Commands:"
echo "  • Build matcher:     go build -o bin/matcher-v2 cmd/matcher-v2/main.go"
echo "  • Test connection:   PGPASSWORD=${DB_PASSWORD} psql -h localhost -p ${DB_PORT} -U ${DB_USER} -d ${DB_NAME}"
echo "  • Run matching:      ./bin/matcher-v2 -cmd=comprehensive-match"
echo "  • Stop system:       docker-compose down"
echo ""