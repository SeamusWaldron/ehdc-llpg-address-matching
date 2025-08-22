# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the **EHDC LLPG (East Hampshire District Council Local Land and Property Gazetteer)** address matching system - a sophisticated Go-based application for matching historic document addresses to modern UPRNs using multiple matching strategies including deterministic matching, fuzzy string similarity, vector embeddings, phonetics, and spatial analysis.

The project processes four historic document datasets (decision notices, land charges cards, enforcement notices, agreements) that often lack UPRNs or coordinates, and matches them against an authoritative LLPG loaded in PostGIS to back-fill missing location data.

## Database Connection

The system uses PostgreSQL with PostGIS running in Docker:
- **Connection**: `postgres://postgres:kljh234hjkl2h@localhost:15435/ehdc_llpg?sslmode=disable`
- **Host**: localhost
- **Port**: 15435 (external), 5432 (container internal)
- **Database**: ehdc_llpg
- **User**: postgres
- **Password**: kljh234hjkl2h

## Development Commands

### Database Operations
```bash
# Start PostgreSQL with PostGIS
docker compose up -d

# Connect to database
PGPASSWORD=kljh234hjkl2h psql -h localhost -p 15435 -U postgres -d ehdc_llpg

# Check database connection in Go app
go run ./app/main.go
```

### Go Development
```bash
# Build the optimized matcher
go build -o bin/matcher-v2 cmd/matcher-v2/main.go

# Run complete comprehensive matching pipeline
./bin/matcher-v2 -cmd=comprehensive-match

# Run individual pipeline stages
./bin/matcher-v2 -cmd=validate-uprns        # Stage 1: Source UPRN validation
./bin/matcher-v2 -cmd=fuzzy-match-groups    # Stage 2 & 3: Deterministic + Fuzzy
./bin/matcher-v2 -cmd=conservative-match    # Stage 4: Conservative validation

# Install dependencies
go mod tidy
```

### Web Interface
The application starts a Gin web server on port 8080 that serves:
- `/` - HTML interface (index.html)
- `/data` - JSON API returning address data with coordinates

## Core Architecture

### Data Flow
1. **LLPG Import**: Loads authoritative address data from `source_docs/ehdc_llpg_20250710.csv`
2. **OS Data Import**: Loads coordinates from OS Open UPRN data
3. **Address Matching**: Multi-stage matching process using various algorithms
4. **Web API**: Serves matched address data via REST API

### Database Schema (Current Implementation)
- `ehdc_addresses`: Main LLPG table with coordinates
- `os_addresses`: OS Open UPRN coordinate data
- Extensions: PostGIS for spatial operations

### Planned Advanced Architecture
The project specifications describe a comprehensive matching system with:

**Core Tables**:
- `dim_address`: Authoritative LLPG dimension table
- `src_document`: Unified historic document table
- `match_result`, `match_accepted`, `match_override`: Audit trail tables
- `match_run`: Versioned matching attempts

**Matching Strategies**:
1. **Deterministic**: Legacy UPRN validation, exact canonical matches
2. **Fuzzy**: PostgreSQL pg_trgm similarity matching
3. **Semantic**: Vector embeddings via Qdrant + local models (Ollama)
4. **Phonetic**: Double Metaphone for street/locality matching
5. **Spatial**: Distance-based filtering when coordinates available

## Technology Stack

- **Language**: Go 1.18+
- **Database**: PostgreSQL 12+ with PostGIS 2.5+
- **Web Framework**: Gin
- **Database Driver**: lib/pq
- **Extensions**: PostGIS, pg_trgm, unaccent
- **Planned**: Qdrant (vector DB), Ollama (embeddings), libpostal (normalization)

## Data Sources

Located in `source_docs/`:
- `ehdc_llpg_20250710.csv` - Current LLPG (71,904 rows)
- `decision_notices.csv` - Planning decisions (76,167 rows, ~90% missing UPRN)
- `land_charges_cards.csv` - Land charges (49,760 rows, ~60% missing UPRN)  
- `enforcement_notices.csv` - Enforcement actions (1,172 rows, ~92% missing UPRN)
- `agreements.csv` - Various agreements (2,602 rows, ~78% missing UPRN)
- `osopenuprn_202507.csv` - OS coordinate data

## Key Implementation Notes

### Address Normalization
- Canonical form: uppercase, punctuation removed, postcodes stripped
- Abbreviation expansion: RD→ROAD, ST→STREET, AVE→AVENUE, etc.
- Preserve house/flat numbers and alpha suffixes (e.g., "12A")
- Store original raw addresses for audit trail

### Coordinate Systems  
- **BNG (EPSG:27700)**: British National Grid for easting/northing
- **WGS84 (EPSG:4326)**: Lat/lon for web display
- PostGIS handles coordinate transformations

### Performance Considerations
- Use GIN indexes for pg_trgm on canonical addresses
- GIST indexes for PostGIS geometry columns
- Batch processing for large datasets
- Connection pooling for concurrent operations

## Quality Targets

- **Auto-accept precision**: ≥98% accuracy on automated matches
- **Coverage**: Maximize % of records with valid UPRNs
- **Explainability**: All matching decisions must be auditable
- **Performance**: <600ms end-to-end matching per query

## File Structure

- `/app/` - Current Go application
- `/source_docs/` - Input CSV data files  
- `/ai_docs/` - AI assistant documentation
- `PROJECT_OVERVIEW.md` - High-level project description
- `PROJECT_SPECIFICATION.md` - Detailed technical requirements
- `ADDRESS_MATCHING_ALGORITHM.md` - Advanced matching algorithm specification
- `docker-compose.yml` - PostgreSQL + PostGIS container setup

## Testing Approach

The project currently lacks formal tests. When adding tests:
- Unit tests for address normalization functions
- Integration tests for database operations
- End-to-end tests for matching accuracy
- Performance benchmarks for large datasets

## Debugging

The planned architecture includes a debug package with Header/Output/Footer pattern for tracing operations through the matching pipeline. This should be implemented consistently across all matching components.

# Development Partnership

We build production code together. I handle implementation details while you guide architecture and catch complexity early.

## Core Workflow: Research → Plan → Implement → Validate

**Start every feature with:** "Let me research the codebase and create a plan before implementing."

1. **Research** - Understand existing patterns and architecture
2. **Plan** - Propose approach and verify with you
3. **Implement** - Build with tests and error handling
4. **Validate** - ALWAYS run formatters, linters, and tests after implementation

## Code Organization

**Keep functions small and focused:**
- If you need comments to explain sections, split into functions
- Group related functionality into clear packages
- Prefer many small files over few large ones

## Architecture Principles

**This is always a feature branch:**
- Delete old code completely - no deprecation needed
- No versioned names (processV2, handleNew, ClientOld)
- No migration code unless explicitly requested
- No "removed code" comments - just delete it

**Prefer explicit over implicit:**
- Clear function names over clever abstractions
- Obvious data flow over hidden magic
- Direct dependencies over service locators

## Maximize Efficiency

**Parallel operations:** Run multiple searches, reads, and greps in single messages
**Multiple agents:** Split complex tasks - one for tests, one for implementation
**Batch similar work:** Group related file edits together

## Go Development Standards

### Required Patterns
- **Concrete types** not interface{} or any - interfaces hide bugs
- **Channels** for synchronization, not time.Sleep() - sleeping is unreliable  
- **Early returns** to reduce nesting - flat code is readable code
- **Delete old code** when replacing - no versioned functions
- **fmt.Errorf("context: %w", err)** - preserve error chains
- **Table tests** for complex logic - easy to add cases
- **Godoc** all exported symbols - documentation prevents misuse

## Problem Solving

**When stuck:** Stop. The simple solution is usually correct.

**When uncertain:** "Let me ultrathink about this architecture."

**When choosing:** "I see approach A (simple) vs B (flexible). Which do you prefer?"

Your redirects prevent over-engineering. When uncertain about implementation, stop and ask for guidance.

## Testing Strategy

**Match testing approach to code complexity:**
- Complex business logic: Write tests first (TDD)
- Simple CRUD operations: Write code first, then tests
- Hot paths: Add benchmarks after implementation

**Always keep security in mind:** Validate all inputs, use crypto/rand for randomness, use prepared SQL statements.

**Performance rule:** Measure before optimizing. No guessing.

## Progress Tracking

- **TodoWrite** for task management
- **Clear naming** in all code

Focus on maintainable solutions over clever abstractions.