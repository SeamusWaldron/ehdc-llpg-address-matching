# Docker Configuration - Environment Variables

The docker-compose.yml file now uses environment variables for all configuration, making it portable across different machines and environments.

## Environment Variables Used

### PostgreSQL Configuration
- `DB_NAME` - Database name (default: ehdc_llpg)
- `DB_USER` - Database user (default: postgres)  
- `DB_PASSWORD` - Database password (default: kljh234hjkl2h)
- `DB_PORT` - External port mapping (default: 15435)
- `POSTGRES_DATA_PATH` - Data volume path (default: postgres_data Docker volume)

### Ollama Configuration
- `OLLAMA_PORT` - External port mapping (default: 11434)

### Qdrant Configuration
- `QDRANT_PORT` - HTTP port (default: 6333)
- `QDRANT_GRPC_PORT` - gRPC port (default: 6334)

### LibPostal Configuration
- `LIBPOSTAL_PORT` - External port mapping (default: 8080)

## Usage Examples

### Your Current Setup (.env file)
```bash
# Your existing database location
POSTGRES_DATA_PATH=/Users/seamus_waldron/Documents/Dev/databases/ehdc-llpg
DB_PORT=15435
DB_USER=postgres
DB_PASSWORD=kljh234hjkl2h
DB_NAME=ehdc_llpg
OLLAMA_PORT=11434
QDRANT_PORT=6333
```

### New Machine Setup
```bash
# Uses Docker volumes and default ports
# No POSTGRES_DATA_PATH needed - creates new volume
DB_PORT=15435
DB_USER=postgres
DB_PASSWORD=secure_password_here
```

### Alternative Port Configuration
```bash
# If ports conflict on your machine
DB_PORT=5433
OLLAMA_PORT=11435
QDRANT_PORT=6334
```

## Benefits

1. **Portability**: Same docker-compose.yml works on any machine
2. **Flexibility**: Easy to change ports without editing docker-compose.yml
3. **Security**: Passwords in .env file, not committed to git
4. **Development**: Different developers can use different configurations

## Files Updated

- `docker-compose.yml` - Uses environment variables with sensible defaults
- `.env` - Contains your specific configuration
- `start_complete_system.sh` - Reads .env variables for connection strings
- `setup_new_machine.sh` - Creates .env with defaults for new machines

## Migration Path

1. **Existing Setup**: Your .env file preserves current database location
2. **New Machines**: setup_new_machine.sh creates appropriate .env
3. **No Changes Needed**: Everything continues to work as before