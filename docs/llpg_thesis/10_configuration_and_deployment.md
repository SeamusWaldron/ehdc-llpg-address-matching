# Chapter 10: Configuration and Deployment

## 10.1 Deployment Overview

The EHDC LLPG Address Matching System is deployed as a containerised application stack using Docker Compose. This architecture provides:

1. **Portability**: Consistent deployment across development, staging, and production
2. **Isolation**: Each service runs in its own container with defined resources
3. **Scalability**: Services can be scaled independently as needed
4. **Reproducibility**: Infrastructure as code ensures identical environments

## 10.2 Docker Compose Architecture

The system is orchestrated via `docker-compose.yml`:

```yaml
# EHDC LLPG Address Matching System - Complete Stack
services:
  # PostgreSQL with PostGIS for address data and fuzzy matching
  postgres:
    image: postgis/postgis:15-3.4
    container_name: ehdc_postgres
    environment:
      POSTGRES_DB: ${DB_NAME:-ehdc_llpg}
      POSTGRES_USER: ${DB_USER:-postgres}
      POSTGRES_PASSWORD: ${DB_PASSWORD:-kljh234hjkl2h}
    ports:
      - "${DB_PORT:-15435}:5432"
    volumes:
      - ${POSTGRES_DATA_PATH:-postgres_data}:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d
    command: |
      postgres
      -c shared_preload_libraries=pg_stat_statements
      -c max_connections=200
      -c shared_buffers=256MB
      -c effective_cache_size=1GB
      -c work_mem=4MB
    restart: unless-stopped

  # Qdrant vector database for semantic address matching
  qdrant:
    image: qdrant/qdrant:v1.7.4
    container_name: ehdc_qdrant
    ports:
      - "${QDRANT_PORT:-6333}:6333"
      - "${QDRANT_GRPC_PORT:-6334}:6334"
    volumes:
      - qdrant_storage:/qdrant/storage
    environment:
      QDRANT__SERVICE__HTTP_PORT: 6333
      QDRANT__SERVICE__GRPC_PORT: 6334
    restart: unless-stopped

  # Ollama for local embeddings and LLM address matching
  ollama:
    image: ollama/ollama:latest
    container_name: ehdc_ollama
    ports:
      - "${OLLAMA_PORT:-11434}:11434"
    volumes:
      - ollama_data:/root/.ollama
    environment:
      OLLAMA_KEEP_ALIVE: "24h"
      OLLAMA_HOST: "0.0.0.0"
    command: |
      sh -c "ollama serve & sleep 10 && ollama pull nomic-embed-text && ollama pull llama3.2:1b && wait"
    restart: unless-stopped

  # libpostal HTTP service for address parsing
  libpostal:
    build:
      context: .
      dockerfile: Dockerfile.libpostal
    container_name: ehdc_libpostal
    ports:
      - "${LIBPOSTAL_PORT:-8080}:8080"
    environment:
      PORT: 8080
    restart: unless-stopped

volumes:
  postgres_data:
    driver: local
  qdrant_storage:
    driver: local
  ollama_data:
    driver: local
```

### 10.2.1 Service Descriptions

| Service | Image | Purpose | Default Port |
|---------|-------|---------|--------------|
| postgres | postgis/postgis:15-3.4 | Primary database with spatial extensions | 15435 |
| qdrant | qdrant/qdrant:v1.7.4 | Vector similarity search | 6333 (HTTP), 6334 (gRPC) |
| ollama | ollama/ollama:latest | Local LLM and embedding generation | 11434 |
| libpostal | Custom build | Statistical address parsing | 8080 |

## 10.3 Environment Configuration

The system uses environment variables for configuration, managed via a `.env` file:

### 10.3.1 Database Configuration

```bash
# PostgreSQL Data Location
POSTGRES_DATA_PATH=/path/to/database/storage

# Database Connection
DB_HOST=localhost
DB_PORT=15435
DB_USER=postgres
DB_PASSWORD=kljh234hjkl2h
DB_NAME=ehdc_llpg
DB_SSLMODE=disable
```

### 10.3.2 Web Interface Configuration

```bash
# Web Server Settings
WEB_PORT=8443
WEB_HOST=localhost
WEB_BASE_URL=http://localhost:8443
```

### 10.3.3 Matching Engine Configuration

```bash
# Confidence Thresholds
MATCH_MIN_THRESHOLD=0.60
MATCH_LOW_CONFIDENCE=0.70
MATCH_MEDIUM_CONFIDENCE=0.78
MATCH_HIGH_CONFIDENCE=0.85
MATCH_WINNER_MARGIN=0.05

# Performance Settings
MATCH_BATCH_SIZE=1000
MATCH_WORKERS=8
MATCH_CACHE_SIZE=10000
```

### 10.3.4 Feature Flags

```bash
# Feature Toggles
ENABLE_MANUAL_OVERRIDE=true
ENABLE_EXPORT=true
ENABLE_REALTIME_UPDATES=true
ENABLE_AUDIT_LOGGING=true
```

### 10.3.5 API Configuration

```bash
# API Settings
API_RATE_LIMIT=1000
API_TIMEOUT=30s
```

### 10.3.6 Logging Configuration

```bash
# Logging
LOG_LEVEL=info
LOG_FORMAT=json
```

### 10.3.7 LLM Services Configuration

```bash
# Ollama Settings
OLLAMA_PORT=11434
OLLAMA_URL=http://localhost:11434

# Qdrant Settings
QDRANT_PORT=6333
QDRANT_GRPC_PORT=6334
QDRANT_URL=http://localhost:6333
```

### 10.3.8 Development Settings

```bash
# Development Options
DEBUG=false
PROFILING_ENABLED=false
```

## 10.4 Go Application Configuration

The web server configuration is managed via the `internal/web/config.go` module:

```go
type Config struct {
    Server   ServerConfig   `json:"server"`
    Database DatabaseConfig `json:"database"`
    Auth     AuthConfig     `json:"auth"`
    Features FeatureConfig  `json:"features"`
}

type ServerConfig struct {
    Port int    `json:"port"`
    Host string `json:"host"`
}

type DatabaseConfig struct {
    URL            string `json:"url"`
    MaxConnections int    `json:"max_connections"`
}

type AuthConfig struct {
    Enabled    bool   `json:"enabled"`
    SessionKey string `json:"session_key"`
}

type FeatureConfig struct {
    ExportEnabled         bool `json:"export_enabled"`
    ManualOverrideEnabled bool `json:"manual_override_enabled"`
}
```

### 10.4.1 Default Configuration

```go
func DefaultConfig() *Config {
    return &Config{
        Server: ServerConfig{
            Port: 8080,
            Host: "0.0.0.0",
        },
        Database: DatabaseConfig{
            URL:            "", // Set via environment
            MaxConnections: 25,
        },
        Auth: AuthConfig{
            Enabled:    false,
            SessionKey: "development-session-key",
        },
        Features: FeatureConfig{
            ExportEnabled:         true,
            ManualOverrideEnabled: true,
        },
    }
}
```

## 10.5 PostgreSQL Tuning

The PostgreSQL container is configured with performance optimisations:

### 10.5.1 Connection Settings

```
max_connections=200
```

This allows up to 200 concurrent database connections, supporting parallel matching workers and web interface queries.

### 10.5.2 Memory Configuration

```
shared_buffers=256MB
effective_cache_size=1GB
work_mem=4MB
```

| Parameter | Value | Purpose |
|-----------|-------|---------|
| shared_buffers | 256MB | Shared memory for caching data pages |
| effective_cache_size | 1GB | Planner estimate of available memory |
| work_mem | 4MB | Memory per sort/hash operation |

### 10.5.3 Extensions

The following PostgreSQL extensions are loaded:

```
shared_preload_libraries=pg_stat_statements
```

Additional extensions created via migrations:

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
```

## 10.6 Volume Management

### 10.6.1 Persistent Volumes

| Volume | Mount Point | Purpose |
|--------|-------------|---------|
| postgres_data | /var/lib/postgresql/data | Database storage |
| qdrant_storage | /qdrant/storage | Vector index storage |
| ollama_data | /root/.ollama | Model weights and cache |

### 10.6.2 External Data Path

For production deployments, the PostgreSQL data directory can be mounted from an external path:

```bash
POSTGRES_DATA_PATH=/external/storage/ehdc-llpg
```

This allows database persistence across container rebuilds.

## 10.7 Network Configuration

### 10.7.1 Port Mappings

| Service | Internal Port | External Port | Protocol |
|---------|---------------|---------------|----------|
| PostgreSQL | 5432 | 15435 | TCP |
| Qdrant HTTP | 6333 | 6333 | HTTP |
| Qdrant gRPC | 6334 | 6334 | gRPC |
| Ollama | 11434 | 11434 | HTTP |
| libpostal | 8080 | 8080 | HTTP |

### 10.7.2 Inter-Service Communication

Services communicate via Docker's internal network. The Go application connects to services using the configured hostnames:

- Database: `localhost:15435` (external) or `postgres:5432` (internal)
- Qdrant: `localhost:6333` (external) or `qdrant:6333` (internal)
- Ollama: `localhost:11434` (external) or `ollama:11434` (internal)

## 10.8 Deployment Procedures

### 10.8.1 Initial Setup

```bash
# 1. Clone repository
git clone <repository-url>
cd ehdc-llpg

# 2. Create environment file
cp env_example .env
# Edit .env with appropriate values

# 3. Start infrastructure services
docker compose up -d

# 4. Build Go application
go build -o bin/matcher-v2 cmd/matcher-v2/main.go

# 5. Initialise database schema
./bin/matcher-v2 -cmd=setup-db

# 6. Load reference data
./bin/matcher-v2 -cmd=load-llpg -llpg-file=source_docs/ehdc_llpg_20250710.csv
./bin/matcher-v2 -cmd=load-os-uprn -os-uprn-file=source_docs/osopenuprn_202507.csv

# 7. Load source documents
./bin/matcher-v2 -cmd=load-sources -source-files=source_docs/decision_notices.csv,source_docs/land_charges_cards.csv,source_docs/enforcement_notices.csv,source_docs/agreements.csv
```

### 10.8.2 Running the Matching Pipeline

```bash
# Complete comprehensive matching
./bin/matcher-v2 -cmd=comprehensive-match

# Or run individual stages
./bin/matcher-v2 -cmd=validate-uprns
./bin/matcher-v2 -cmd=fuzzy-match-groups
./bin/matcher-v2 -cmd=conservative-match
```

### 10.8.3 Starting the Web Interface

```bash
# Start the web server
./bin/matcher-v2 -cmd=serve
# Access at http://localhost:8443
```

## 10.9 Migration Management

Database migrations are stored in the `migrations/` directory and executed automatically on container startup:

### 10.9.1 Migration Files

| File | Purpose |
|------|---------|
| 001_staging_tables.sql | Create staging tables for raw data |
| 002_normalized_schema.sql | Create normalised dimension tables |

### 10.9.2 Manual Migration

Migrations can be applied manually:

```bash
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg -f migrations/001_staging_tables.sql
```

## 10.10 Monitoring and Health Checks

### 10.10.1 Service Health

```bash
# Check container status
docker compose ps

# View service logs
docker compose logs -f postgres
docker compose logs -f qdrant
docker compose logs -f ollama
```

### 10.10.2 Database Statistics

```bash
# Connect to database
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg

# View table sizes
SELECT
    relname AS table_name,
    pg_size_pretty(pg_total_relation_size(relid)) AS total_size
FROM pg_stat_user_tables
ORDER BY pg_total_relation_size(relid) DESC;
```

### 10.10.3 Application Statistics

```bash
# View matching statistics
./bin/matcher-v2 -cmd=stats
```

## 10.11 Backup and Recovery

### 10.11.1 Database Backup

```bash
# Full database dump
PGPASSWORD=kljh234hjkl2h pg_dump -h localhost -p 15435 -U postgres -d ehdc_llpg > backup.sql

# Compressed backup
PGPASSWORD=kljh234hjkl2h pg_dump -h localhost -p 15435 -U postgres -d ehdc_llpg | gzip > backup.sql.gz
```

### 10.11.2 Database Restore

```bash
# Restore from backup
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg < backup.sql

# Restore from compressed backup
gunzip -c backup.sql.gz | PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg
```

### 10.11.3 Volume Backup

```bash
# Stop services
docker compose stop

# Backup volumes
tar -czvf postgres_backup.tar.gz /path/to/postgres_data
tar -czvf qdrant_backup.tar.gz /path/to/qdrant_storage

# Restart services
docker compose start
```

## 10.12 Security Considerations

### 10.12.1 Database Security

- Change default database password in production
- Use SSL connections for remote database access
- Restrict network access to database port

### 10.12.2 API Security

- Enable authentication for web interface in production
- Configure API rate limiting
- Use HTTPS for all external connections

### 10.12.3 Container Security

- Run containers as non-root users where possible
- Keep container images updated
- Scan images for vulnerabilities

## 10.13 Chapter Summary

This chapter has documented the deployment configuration:

- Docker Compose orchestration of four services
- Environment variable configuration
- PostgreSQL performance tuning
- Volume management for persistence
- Network and port configuration
- Deployment procedures
- Migration management
- Monitoring and health checks
- Backup and recovery procedures
- Security considerations

The containerised architecture provides a consistent, reproducible deployment that can be adapted for different environments.

---

*This chapter details deployment configuration. Chapter 11 presents results and statistics.*
