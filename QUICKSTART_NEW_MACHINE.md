# Quick Start - New Machine Setup

## One-Command Setup

```bash
# Clone repo and run setup
git clone <repo-url> ehdc-llpg
cd ehdc-llpg
./setup_new_machine.sh
./start_complete_system.sh
```

## What This Does

### Prerequisites (you need these installed first)
- **Docker Desktop** - for containers
- **Go 1.19+** - for building the matcher
- **Git** - for cloning the repo

### Services Started
- **PostgreSQL** (port 15435) - Main database with PostGIS
- **Ollama** (port 11434) - LLM for address similarity 
- **Qdrant** (port 6333) - Vector database for semantic matching

### Database Setup Options
1. **New Machine**: Uses Docker volume `postgres_data`
2. **Existing Database**: Set `POSTGRES_DATA_PATH=/path/to/existing/db` in `.env`

### Files Created
- `bin/matcher-v2` - Main application executable
- `.env` - Environment configuration
- Docker volumes for persistent data

## Quick Commands After Setup

```bash
# Build application
go build -o bin/matcher-v2 cmd/matcher-v2/main.go

# Load data and run matching
./bin/matcher-v2 -cmd=load-llpg -llpg=llpg_docs/ehdc_llpg_20250710.csv
./bin/matcher-v2 -cmd=load-sources
./bin/matcher-v2 -cmd=comprehensive-match

# Check database
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg

# Stop everything
docker-compose down
```

## Troubleshooting

### Port Conflicts
If ports 15435, 11434, or 6333 are in use:
1. Stop conflicting services
2. Or edit `.env` to use different ports

### Standalone Containers
If you see containers like `ehdc_ollama_fixed` running outside docker-compose:
```bash
docker stop ehdc_ollama_fixed
docker rm ehdc_ollama_fixed
./start_complete_system.sh
```

### Model Downloads
Ollama models (llama3.2:1b, nomic-embed-text) are downloaded automatically but may take 5-10 minutes on first run.

## Project Structure
- `cmd/matcher-v2/` - Main application
- `docker-compose.yml` - All services defined here
- `source_docs/` - Input CSV files
- `bin/` - Built executables
- `export/` - Output files