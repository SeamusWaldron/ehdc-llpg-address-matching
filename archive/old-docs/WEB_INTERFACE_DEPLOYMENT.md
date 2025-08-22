# EHDC LLPG Web Interface Deployment Guide

## Overview

The EHDC LLPG web interface provides an interactive mapping application for visualizing and managing address matching results. This document covers deployment and usage instructions.

## Prerequisites

1. **Go 1.18+** - Required to build the application
2. **PostgreSQL with PostGIS** - Database with the EHDC LLPG data
3. **Docker** (optional) - For containerized deployment

## Quick Start

### 1. Build the Application

```bash
# Build the web server
go build -o bin/ehdc-web cmd/web/main.go

# Build the CLI matcher (if not already built)
go build -o bin/matcher cmd/matcher/main.go
```

### 2. Database Setup

Ensure your PostgreSQL database is running and contains:
- All LLPG data (from Phase 1-2 implementation)
- Enhanced views (from `sql/09_create_enhanced_views.sql`)
- Map views (from `sql/10_create_map_views_fixed.sql`)

```bash
# Apply database views if not already done
./bin/matcher db apply-sql sql/09_create_enhanced_views.sql
./bin/matcher db apply-sql sql/10_create_map_views_fixed.sql
```

### 3. Configuration

Create/modify `config.json` with your database connection:

```json
{
    "server": {
        "port": 8080,
        "host": "0.0.0.0"
    },
    "database": {
        "url": "postgres://username:password@localhost:5432/ehdc_gis?sslmode=disable",
        "max_connections": 25
    },
    "auth": {
        "enabled": false,
        "session_key": "change-me-in-production"
    },
    "features": {
        "export_enabled": true,
        "manual_override_enabled": true
    }
}
```

### 4. Start the Server

```bash
# Start with custom config
./bin/ehdc-web --config config.json

# Or with default config.json
./bin/ehdc-web

# Custom port
./bin/ehdc-web --port 9090
```

The web interface will be available at `http://localhost:8080`

## API Endpoints

### Core Data Endpoints
- `GET /api/records` - Filtered records list with pagination
- `GET /api/records/geojson` - GeoJSON data for map display
- `GET /api/records/{id}` - Individual record details
- `GET /api/records/{id}/candidates` - Match candidates for record

### Search Endpoints
- `GET /api/search/llpg?q={query}` - Search LLPG addresses
- `GET /api/search/records?q={query}` - Search source records

### Statistics Endpoints
- `GET /api/stats` - Overall system statistics
- `GET /api/stats/viewport` - Statistics for map viewport

### Management Endpoints (if enabled)
- `POST /api/records/{id}/accept` - Accept a match candidate
- `PUT /api/records/{id}/coordinates` - Set manual coordinates
- `POST /api/records/{id}/reject` - Reject all candidates
- `POST /api/export` - Generate data exports

## API Usage Examples

### Get GeoJSON Data with Filters

```bash
# All matched decision notices
curl "http://localhost:8080/api/records/geojson?source_type=decision&match_status=MATCHED"

# Records within viewport bounds
curl "http://localhost:8080/api/records/geojson?min_lat=50.8&max_lat=51.0&min_lng=-1.2&max_lng=-0.8"

# Search by address
curl "http://localhost:8080/api/records/geojson?address_search=high%20street"
```

### Get System Statistics

```bash
curl "http://localhost:8080/api/stats"
```

### Search LLPG Addresses

```bash
curl "http://localhost:8080/api/search/llpg?q=mill%20lane&limit=10"
```

## Web Interface Features

### Interactive Map
- **Clustered markers** by confidence level:
  - 🟢 Green: High confidence matches (≥ 0.92)
  - 🔵 Blue: Review needed (0.80-0.92)
  - 🟠 Orange: Low confidence (< 0.80)
  - 🔴 Red: No location/coordinates

### Filter Panel
- **Source Type**: Filter by decision notices, land charges, enforcement, agreements
- **Match Status**: Filter by matched, unmatched, needs review
- **Address Quality**: Filter by good, fair, poor quality ratings
- **Address Search**: Text search across addresses and references

### Statistics Panel
- Real-time statistics for current map view
- Total records, match counts, and match rates
- Updates automatically as filters change

## Production Deployment

### Security Considerations

1. **Enable Authentication**:
```json
{
    "auth": {
        "enabled": true,
        "session_key": "secure-random-key-256-bits"
    }
}
```

2. **Database Security**:
   - Use connection pooling
   - Enable SSL connections
   - Restrict database user permissions

3. **Network Security**:
   - Use HTTPS with TLS certificates
   - Configure firewall rules
   - Consider reverse proxy (nginx/Apache)

### Environment Variables

Override config values with environment variables:

```bash
export EHDC_DB_URL="postgres://user:pass@db:5432/ehdc_gis?sslmode=require"
export EHDC_SERVER_PORT=8080
export EHDC_AUTH_ENABLED=true
```

### Docker Deployment

```dockerfile
# Dockerfile
FROM golang:1.18 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o ehdc-web cmd/web/main.go

FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates
COPY --from=builder /app/ehdc-web /usr/local/bin/
COPY --from=builder /app/internal/web/static /app/static
COPY config.json /app/
WORKDIR /app
CMD ["ehdc-web"]
```

```bash
# Build and run
docker build -t ehdc-web .
docker run -p 8080:8080 -v $(pwd)/config.json:/app/config.json ehdc-web
```

### Systemd Service

```ini
# /etc/systemd/system/ehdc-web.service
[Unit]
Description=EHDC LLPG Web Interface
After=network.target postgresql.service

[Service]
Type=simple
User=ehdc
WorkingDirectory=/opt/ehdc-llpg
ExecStart=/opt/ehdc-llpg/bin/ehdc-web --config /opt/ehdc-llpg/config.json
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
# Enable and start
sudo systemctl enable ehdc-web
sudo systemctl start ehdc-web
```

## Monitoring and Maintenance

### Health Checks

```bash
# Basic health check
curl -f http://localhost:8080/api/stats

# Database connectivity check
curl -f http://localhost:8080/api/records?per_page=1
```

### Log Monitoring

The application logs HTTP requests and database operations:

```bash
# Follow logs in systemd
journalctl -u ehdc-web -f

# Log levels
# - Request logging: Method, URL, status code, duration
# - Database errors: Connection issues, query failures
# - Server events: Startup, shutdown, configuration
```

### Performance Monitoring

Monitor key metrics:
- **Response time**: API endpoint performance
- **Database connections**: Pool usage and connection health  
- **Memory usage**: Application memory consumption
- **Disk usage**: Database size and growth

### Database Maintenance

```bash
# Update statistics (run periodically)
psql -c "ANALYZE v_enhanced_source_documents, v_map_all_records;"

# Vacuum database (weekly)
psql -c "VACUUM ANALYZE;"

# Check database size
psql -c "SELECT pg_size_pretty(pg_database_size('ehdc_gis'));"
```

## Troubleshooting

### Common Issues

1. **Database Connection Errors**
   - Check PostgreSQL is running
   - Verify connection string in config
   - Test database connectivity: `psql "postgres://..."`

2. **Missing Map Data**
   - Ensure views are created: `sql/09_*.sql` and `sql/10_*.sql`
   - Check PostGIS extension: `SELECT PostGIS_Version();`
   - Verify coordinate data exists

3. **Slow Performance**
   - Check database indexes are created
   - Monitor query performance with `EXPLAIN ANALYZE`
   - Consider adjusting connection pool size

4. **Frontend Issues**
   - Check browser console for JavaScript errors
   - Verify API endpoints return valid JSON
   - Test with curl/Postman for API debugging

### Debug Mode

Add debugging to configuration:

```json
{
    "server": {
        "debug": true,
        "log_level": "debug"
    }
}
```

This enables:
- Detailed request/response logging
- SQL query logging
- Performance metrics
- Error stack traces

## Next Steps

1. **Implement authentication** for production use
2. **Add export functionality** for generating reports
3. **Create user management** for multi-user scenarios
4. **Add audit logging** for tracking user actions
5. **Implement real-time updates** with WebSockets
6. **Add mobile responsiveness** improvements

## Support

For technical support:
1. Check application logs for error details
2. Verify database connectivity and view existence  
3. Test API endpoints independently
4. Review configuration settings
5. Monitor system resources (CPU, memory, disk)

The web interface provides a powerful tool for visualizing and managing the EHDC LLPG address matching results, with comprehensive filtering, statistics, and export capabilities.