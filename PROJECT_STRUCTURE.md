# Project Structure

## Root Directory (Clean)

### 📋 Core Documentation
- `README.md` - Project overview and usage
- `CLAUDE.md` - Development guidelines for AI assistants
- `PROJECT_OVERVIEW.md` - High-level project description
- `PROJECT_SPECIFICATION.md` - Detailed technical requirements
- `ADDRESS_MATCHING_ALGORITHM.md` - Algorithm specifications
- `ADDRESS_MATCHING_PROCESS.md` - Process documentation

### 🚀 Quick Start
- `QUICKSTART.md` - Basic setup and usage
- `QUICKSTART_NEW_MACHINE.md` - New machine setup guide
- `DOCKER_CONFIGURATION.md` - Docker environment variables guide

### ⚙️ Configuration & Scripts
- `docker-compose.yml` - Service definitions with environment variables
- `setup_new_machine.sh` - New machine setup script
- `start_complete_system.sh` - System startup orchestration
- `.env` - Environment configuration (local)
- `go.mod` / `go.sum` - Go dependencies

### 📁 Core Directories

#### `/cmd/`
Application entry points:
- `matcher-v2/` - **Main application** (current production version)
- `web/` - Web interface
- Various specialized matchers and utilities

#### `/internal/`
Internal Go packages:
- `engine/` - Matching algorithms
- `db/` - Database utilities
- `normalize/` - Address normalization
- `validation/` - Data validation
- `web/` - Web interface components

#### `/source_docs/`
Input CSV files:
- Decision notices, land charges, enforcement, agreements
- Street naming records, microfiche data

#### `/llpg_docs/`
LLPG baseline data:
- `ehdc_llpg_20250710.csv` - Current LLPG addresses

#### `/migrations/`
Database schema migrations:
- Numbered SQL files for database setup and updates

#### `/scripts/`
Utility scripts:
- Export scripts, SQL utilities

#### `/sql/`
SQL utilities and queries

### 🗃️ Generated/Runtime Directories
- `/bin/` - Compiled executables (gitignored)
- `/export/` - Output CSV files (gitignored)
- `/logs/` - Application logs (gitignored)
- `/backups/` - Database backups (gitignored)

### 📚 Archive
- `/archive/` - Historical files moved for cleanliness
  - `/tests/` - Test files and validation scripts
  - `/old-binaries/` - Previous compiled versions
  - `/old-scripts/` - Legacy deployment scripts  
  - `/old-docs/` - Superseded documentation

## Current Workflow

1. **Setup**: `./setup_new_machine.sh`
2. **Start**: `./start_complete_system.sh`
3. **Build**: `go build -o bin/matcher-v2 cmd/matcher-v2/main.go`
4. **Run**: `./bin/matcher-v2 -cmd=comprehensive-match`

## Key Active Files

- **Main Application**: `cmd/matcher-v2/main.go`
- **Docker Services**: `docker-compose.yml`
- **Environment**: `.env`
- **Startup**: `start_complete_system.sh`
- **Documentation**: Files in root directory