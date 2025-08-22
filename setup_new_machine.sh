#!/bin/bash

# EHDC LLPG New Machine Setup Script
# Run this on a fresh machine to set up the complete development environment

set -e

echo "🛠️  Setting up EHDC LLPG on new machine..."

# Check prerequisites
echo "🔍 Checking prerequisites..."

# Check Docker
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is required but not installed. Please install Docker Desktop."
    exit 1
fi

# Check Docker Compose
if ! command -v docker-compose &> /dev/null; then
    echo "❌ Docker Compose is required but not installed."
    exit 1
fi

# Check Go
if ! command -v go &> /dev/null; then
    echo "❌ Go is required but not installed. Please install Go 1.19+."
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "✅ Go version: $GO_VERSION"

# Check Git
if ! command -v git &> /dev/null; then
    echo "❌ Git is required but not installed."
    exit 1
fi

echo "✅ All prerequisites found!"

# Create required directories
echo "📁 Creating required directories..."
mkdir -p bin
mkdir -p export
mkdir -p logs

# Initialize Go modules if needed
if [ ! -f go.mod ]; then
    echo "📦 Initializing Go modules..."
    go mod init ehdc-llpg
fi

# Download Go dependencies
echo "⬇️  Downloading Go dependencies..."
go mod tidy

# Build the matcher
echo "🔨 Building matcher-v2..."
go build -o bin/matcher-v2 cmd/matcher-v2/main.go

# Create .env file for local configuration
if [ ! -f .env ]; then
    echo "⚙️  Creating .env file..."
    cat > .env << EOF
# EHDC LLPG Environment Configuration

# PostgreSQL Data Location (uses Docker volume by default)
# POSTGRES_DATA_PATH=/path/to/your/existing/database

# Database Configuration
DB_HOST=localhost
DB_PORT=15435
DB_USER=postgres
DB_PASSWORD=kljh234hjkl2h
DB_NAME=ehdc_llpg
DB_SSLMODE=disable

# Web Interface Configuration  
WEB_PORT=8443
WEB_HOST=localhost
WEB_BASE_URL=http://localhost:8443

# LibPostal Service
LIBPOSTAL_PORT=8080

# LLM Services
OLLAMA_PORT=11434
OLLAMA_URL=http://localhost:11434
QDRANT_PORT=6333
QDRANT_GRPC_PORT=6334
QDRANT_URL=http://localhost:6333

# Development Settings
DEBUG=false
EOF
    echo "✅ Created .env file with default configuration"
    echo "💡 To use existing database, set POSTGRES_DATA_PATH in .env"
else
    echo "✅ .env file already exists"
fi

# Create gitignore additions for local files
if ! grep -q "bin/" .gitignore 2>/dev/null; then
    echo "📝 Updating .gitignore..."
    cat >> .gitignore << EOF

# Local build artifacts
bin/
export/
logs/
*.log

# Local environment
.env.local
EOF
fi

echo ""
echo "🎉 Setup complete!"
echo ""
echo "📋 Next steps:"
echo "  1. Start the system:     ./start_complete_system.sh"
echo "  2. Load LLPG data:       ./bin/matcher-v2 -cmd=load-llpg -llpg=llpg_docs/ehdc_llpg_20250710.csv"
echo "  3. Load source docs:     ./bin/matcher-v2 -cmd=load-sources"
echo "  4. Run matching:         ./bin/matcher-v2 -cmd=comprehensive-match"
echo ""
echo "📚 Documentation:"
echo "  • README.md - Project overview and commands"
echo "  • CLAUDE.md - Development guidelines"
echo "  • PROJECT_SPECIFICATION.md - Technical details"
echo ""