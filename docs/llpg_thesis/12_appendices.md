# Chapter 12: Appendices

## Appendix A: Complete Database Schema DDL

### A.1 Extensions

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
```

### A.2 Staging Tables

```sql
-- EHDC LLPG staging table
CREATE TABLE IF NOT EXISTS stg_ehdc_llpg (
    ogc_fid text,
    locaddress text,
    easting text,
    northing text,
    lgcstatusc text,
    bs7666uprn text,
    bs7666usrn text,
    landparcel text,
    blpuclass text,
    postal text,
    loaded_at timestamptz DEFAULT now()
);

-- OS UPRN staging table
CREATE TABLE IF NOT EXISTS stg_os_uprn (
    uprn text,
    x_coordinate text,
    y_coordinate text,
    latitude text,
    longitude text,
    loaded_at timestamptz DEFAULT now()
);

-- Decision notices staging
CREATE TABLE IF NOT EXISTS stg_decision_notices (
    job_number text,
    filepath text,
    planning_application_number text,
    adress text,
    decision_date text,
    decision_type text,
    document_type text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);

-- Land charges staging
CREATE TABLE IF NOT EXISTS stg_land_charges (
    job_number text,
    filepath text,
    card_code text,
    address text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);

-- Enforcement notices staging
CREATE TABLE IF NOT EXISTS stg_enforcement_notices (
    job_number text,
    filepath text,
    planning_enforcement_reference_number text,
    address text,
    date text,
    document_type text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);

-- Agreements staging
CREATE TABLE IF NOT EXISTS stg_agreements (
    job_number text,
    filepath text,
    address text,
    date text,
    bs7666uprn text,
    easting text,
    northing text,
    loaded_at timestamptz DEFAULT now()
);
```

### A.3 Dimension Tables

```sql
-- Location dimension
CREATE TABLE dim_location (
    location_id SERIAL PRIMARY KEY,
    uprn TEXT UNIQUE,
    easting NUMERIC,
    northing NUMERIC,
    latitude NUMERIC,
    longitude NUMERIC,
    geom_27700 GEOMETRY(POINT, 27700),
    geom_4326 GEOMETRY(POINT, 4326),
    source_dataset TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Address dimension
CREATE TABLE dim_address (
    address_id SERIAL PRIMARY KEY,
    location_id INTEGER REFERENCES dim_location(location_id),
    uprn TEXT UNIQUE,
    full_address TEXT NOT NULL,
    address_canonical TEXT,
    usrn TEXT,
    blpu_class TEXT,
    postal_flag BOOLEAN,
    status_code TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Document type dimension
CREATE TABLE dim_document_type (
    doc_type_id SERIAL PRIMARY KEY,
    type_code TEXT UNIQUE,
    type_name TEXT,
    description TEXT
);

-- Match method dimension
CREATE TABLE dim_match_method (
    method_id SERIAL PRIMARY KEY,
    method_code TEXT UNIQUE,
    method_name TEXT,
    description TEXT,
    confidence_threshold NUMERIC(5,4)
);
```

### A.4 Fact Tables

```sql
-- Source document table
CREATE TABLE src_document (
    document_id SERIAL PRIMARY KEY,
    doc_type_id INTEGER REFERENCES dim_document_type(doc_type_id),
    job_number TEXT,
    filepath TEXT,
    external_reference TEXT,
    document_date DATE,
    raw_address TEXT NOT NULL,
    address_canonical TEXT,
    raw_uprn TEXT,
    raw_easting TEXT,
    raw_northing TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Address matching table
CREATE TABLE address_match (
    match_id SERIAL PRIMARY KEY,
    document_id INTEGER REFERENCES src_document(document_id),
    address_id INTEGER REFERENCES dim_address(address_id),
    location_id INTEGER REFERENCES dim_location(location_id),
    match_method_id INTEGER REFERENCES dim_match_method(method_id),
    confidence_score NUMERIC(5,4),
    match_status TEXT,
    matched_by TEXT,
    matched_at TIMESTAMPTZ DEFAULT now()
);

-- Normalisation rules
CREATE TABLE address_normalization_rules (
    rule_id SERIAL PRIMARY KEY,
    pattern TEXT,
    replacement TEXT,
    rule_type TEXT,
    priority INTEGER DEFAULT 0
);
```

### A.5 Indexes

```sql
-- Location indexes
CREATE INDEX idx_dim_location_uprn ON dim_location(uprn);
CREATE INDEX idx_dim_location_coords ON dim_location(easting, northing);
CREATE INDEX idx_dim_location_geom_27700 ON dim_location USING GIST(geom_27700);
CREATE INDEX idx_dim_location_geom_4326 ON dim_location USING GIST(geom_4326);
CREATE INDEX idx_dim_location_source ON dim_location(source_dataset);

-- Address indexes
CREATE INDEX idx_dim_address_uprn ON dim_address(uprn);
CREATE INDEX idx_dim_address_location_id ON dim_address(location_id);
CREATE INDEX idx_dim_address_canonical_trgm ON dim_address USING GIN(address_canonical gin_trgm_ops);
CREATE INDEX idx_dim_address_full_text ON dim_address USING GIN(to_tsvector('english', full_address));

-- Source document indexes
CREATE INDEX idx_src_document_doc_type ON src_document(doc_type_id);
CREATE INDEX idx_src_document_canonical_trgm ON src_document USING GIN(address_canonical gin_trgm_ops);
CREATE INDEX idx_src_document_raw_uprn ON src_document(raw_uprn);
CREATE INDEX idx_src_document_date ON src_document(document_date);

-- Match indexes
CREATE INDEX idx_address_match_document ON address_match(document_id);
CREATE INDEX idx_address_match_address ON address_match(address_id);
CREATE INDEX idx_address_match_location ON address_match(location_id);
CREATE INDEX idx_address_match_method ON address_match(match_method_id);
CREATE INDEX idx_address_match_status ON address_match(match_status);
CREATE INDEX idx_address_match_confidence ON address_match(confidence_score);
```

---

## Appendix B: Configuration Reference

### B.1 Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_DATA_PATH` | `postgres_data` | PostgreSQL data directory |
| `DB_HOST` | `localhost` | Database host |
| `DB_PORT` | `15435` | Database port |
| `DB_USER` | `postgres` | Database user |
| `DB_PASSWORD` | - | Database password |
| `DB_NAME` | `ehdc_llpg` | Database name |
| `DB_SSLMODE` | `disable` | SSL mode |
| `WEB_PORT` | `8443` | Web server port |
| `WEB_HOST` | `localhost` | Web server host |
| `WEB_BASE_URL` | `http://localhost:8443` | Base URL |
| `LIBPOSTAL_PORT` | `8080` | libpostal service port |
| `MATCH_MIN_THRESHOLD` | `0.60` | Minimum match threshold |
| `MATCH_LOW_CONFIDENCE` | `0.70` | Low confidence threshold |
| `MATCH_MEDIUM_CONFIDENCE` | `0.78` | Medium confidence threshold |
| `MATCH_HIGH_CONFIDENCE` | `0.85` | High confidence threshold |
| `MATCH_WINNER_MARGIN` | `0.05` | Winner margin requirement |
| `MATCH_BATCH_SIZE` | `1000` | Batch processing size |
| `MATCH_WORKERS` | `8` | Parallel workers |
| `MATCH_CACHE_SIZE` | `10000` | Cache entries |
| `ENABLE_MANUAL_OVERRIDE` | `true` | Enable manual overrides |
| `ENABLE_EXPORT` | `true` | Enable data export |
| `ENABLE_REALTIME_UPDATES` | `true` | Enable real-time updates |
| `ENABLE_AUDIT_LOGGING` | `true` | Enable audit logging |
| `API_RATE_LIMIT` | `1000` | API requests per minute |
| `API_TIMEOUT` | `30s` | API request timeout |
| `LOG_LEVEL` | `info` | Log level |
| `LOG_FORMAT` | `json` | Log format |
| `OLLAMA_PORT` | `11434` | Ollama port |
| `OLLAMA_URL` | `http://localhost:11434` | Ollama URL |
| `QDRANT_PORT` | `6333` | Qdrant HTTP port |
| `QDRANT_GRPC_PORT` | `6334` | Qdrant gRPC port |
| `QDRANT_URL` | `http://localhost:6333` | Qdrant URL |
| `DEBUG` | `false` | Debug mode |
| `PROFILING_ENABLED` | `false` | Enable profiling |

### B.2 Feature Weights

| Feature | Weight | Description |
|---------|--------|-------------|
| TrigramSimilarity | 0.45 | pg_trgm similarity score |
| EmbeddingCosine | 0.45 | Vector embedding similarity |
| LocalityOverlap | 0.05 | Locality token overlap |
| StreetOverlap | 0.05 | Street token overlap |
| SameHouseNumber | 0.08 | House number match bonus |
| SameHouseAlpha | 0.02 | Alpha suffix match bonus |
| USRNMatch | 0.04 | USRN match bonus |
| LLPGLive | 0.03 | Live status bonus |
| LegacyUPRNValid | 0.20 | Validated UPRN bonus |
| SpatialBoostMax | 0.10 | Maximum spatial boost |
| DescriptorPenalty | -0.05 | Descriptor mismatch penalty |
| PhoneticMissPenalty | -0.03 | Phonetic mismatch penalty |

### B.3 Match Tiers

| Tier | Threshold | Decision |
|------|-----------|----------|
| High Confidence | >= 0.92 | Auto-Accept |
| Medium Confidence | >= 0.88 | Auto-Accept (with validation) |
| Review Required | >= 0.80 | Needs Review |
| Low Confidence | >= 0.70 | Needs Review |
| Reject | < 0.70 | Rejected |

---

## Appendix C: CLI Command Reference

### C.1 Database Setup Commands

| Command | Description |
|---------|-------------|
| `setup-db` | Initialise database schema and extensions |
| `setup-vector` | Configure Qdrant vector database |
| `setup-spatial-tables` | Create spatial index tables |

### C.2 Data Loading Commands

| Command | Options | Description |
|---------|---------|-------------|
| `load-llpg` | `-llpg-file=<path>` | Load EHDC LLPG data |
| `load-os-uprn` | `-os-uprn-file=<path>`, `-batch-size=<n>` | Load OS UPRN coordinates |
| `load-sources` | `-source-files=<paths>` | Load source documents |

### C.3 Matching Commands

| Command | Options | Description |
|---------|---------|-------------|
| `validate-uprns` | | Validate source UPRNs against LLPG |
| `fuzzy-match-groups` | | Match by address group |
| `fuzzy-match-individual` | | Match individual documents |
| `conservative-match` | `-run-label=<label>` | Conservative validation |
| `comprehensive-match` | | Full matching pipeline |
| `match-single` | `-address=<address>` | Match single address |
| `match-batch` | `-run-label=<label>` | Batch matching |

### C.4 Data Processing Commands

| Command | Description |
|---------|-------------|
| `apply-corrections` | Apply group consensus corrections |
| `standardize-addresses` | Standardise source addresses |
| `clean-source-data` | Clean source address data |
| `expand-llpg-ranges` | Expand address ranges |
| `llm-fix-addresses` | LLM-based address correction |

### C.5 Spatial Commands

| Command | Description |
|---------|-------------|
| `build-spatial-parallel` | Build spatial indexes in parallel |
| `build-road-postcode-parallel` | Build road/postcode areas |
| `build-road-parallel` | Build road areas |

### C.6 Maintenance Commands

| Command | Description |
|---------|-------------|
| `rebuild-fact` | Rebuild fact table |
| `rebuild-fact-intelligent` | Intelligent fact table rebuild |
| `validate-integrity` | Validate data integrity |
| `stats` | Display matching statistics |

### C.7 Pipeline Commands

| Command | Description |
|---------|-------------|
| `end-to-end-with-snapshots` | Full pipeline with snapshots |
| `layer3-enhanced` | Enhanced Layer 3 matching |

### C.8 Command Examples

```bash
# Initial setup
./bin/matcher-v2 -cmd=setup-db
./bin/matcher-v2 -cmd=load-llpg -llpg-file=source_docs/ehdc_llpg_20250710.csv
./bin/matcher-v2 -cmd=load-os-uprn -os-uprn-file=source_docs/osopenuprn_202507.csv -batch-size=50000

# Load source documents
./bin/matcher-v2 -cmd=load-sources -source-files=source_docs/decision_notices.csv,source_docs/land_charges_cards.csv

# Run matching
./bin/matcher-v2 -cmd=comprehensive-match

# View statistics
./bin/matcher-v2 -cmd=stats

# Single address match (debug)
./bin/matcher-v2 -cmd=match-single -address="12 HIGH STREET, ALTON" -debug
```

---

## Appendix D: Glossary of Terms

| Term | Definition |
|------|------------|
| **BLPU** | Basic Land and Property Unit - the fundamental addressable unit |
| **BNG** | British National Grid - UK coordinate system (EPSG:27700) |
| **Canonical Address** | Normalised address format for matching |
| **Double Metaphone** | Phonetic algorithm producing two encodings |
| **ETL** | Extract, Transform, Load - data pipeline process |
| **Fuzzy Matching** | Approximate string matching using similarity algorithms |
| **GIN Index** | Generalised Inverted Index for full-text search |
| **GIST Index** | Generalised Search Tree for spatial data |
| **Jaro-Winkler** | String similarity algorithm favouring prefix matches |
| **LLPG** | Local Land and Property Gazetteer - local authority address register |
| **LPI** | Land and Property Identifier - address component |
| **NLPG** | National Land and Property Gazetteer - national address register |
| **Postcode** | UK postal code for geographic areas |
| **PostGIS** | PostgreSQL extension for geographic objects |
| **pg_trgm** | PostgreSQL extension for trigram-based similarity |
| **Qdrant** | Vector database for semantic search |
| **Trigram** | Three-character substring for similarity matching |
| **UPRN** | Unique Property Reference Number - unique address identifier |
| **USRN** | Unique Street Reference Number - street identifier |
| **Vector Embedding** | Numerical representation of text for semantic comparison |
| **WGS84** | World Geodetic System 1984 - GPS coordinate system (EPSG:4326) |

---

## Appendix E: File Structure Reference

### E.1 Project Structure

```
ehdc-llpg/
├── app/                         # Legacy application
├── bin/                         # Compiled binaries
│   └── matcher-v2               # Main executable
├── cmd/
│   └── matcher-v2/
│       └── main.go              # CLI entry point
├── docs/                        # Documentation
│   ├── LLPG_THESIS_PLAN.md
│   └── llpg_thesis/             # This thesis
├── internal/                    # Internal packages
│   ├── db/                      # Database operations
│   │   ├── connection.go
│   │   └── queries.go
│   ├── engine/                  # Matching engines
│   │   ├── fuzzy.go
│   │   └── semantic.go
│   ├── match/                   # Matching types
│   │   └── types.go
│   ├── normalize/               # Address normalisation
│   │   └── address.go
│   ├── phonetics/               # Phonetic algorithms
│   │   └── metaphone.go
│   ├── spatial/                 # Spatial operations
│   │   └── index.go
│   └── web/                     # Web interface
│       ├── config.go
│       ├── handlers.go
│       └── server.go
├── migrations/                  # Database migrations
│   ├── 001_staging_tables.sql
│   └── 002_normalized_schema.sql
├── source_docs/                 # Input data files
│   ├── ehdc_llpg_20250710.csv
│   ├── osopenuprn_202507.csv
│   ├── decision_notices.csv
│   ├── land_charges_cards.csv
│   ├── enforcement_notices.csv
│   └── agreements.csv
├── static/                      # Web static files
│   └── index.html
├── .env                         # Environment configuration
├── docker-compose.yml           # Container orchestration
├── go.mod                       # Go module definition
├── go.sum                       # Go module checksums
└── README.md                    # Project documentation
```

### E.2 Internal Package Dependencies

```
cmd/matcher-v2/main.go
    ├── internal/db
    ├── internal/engine
    ├── internal/match
    ├── internal/normalize
    ├── internal/phonetics
    ├── internal/spatial
    └── internal/web

internal/engine
    ├── internal/db
    ├── internal/match
    ├── internal/normalize
    └── internal/phonetics

internal/web
    ├── internal/db
    └── internal/match
```

### E.3 Data File Formats

#### EHDC LLPG (CSV)
```
ogc_fid,locaddress,easting,northing,lgcstatusc,bs7666uprn,bs7666usrn,landparcel,blpuclass,postal
```

#### OS Open UPRN (CSV)
```
UPRN,X_COORDINATE,Y_COORDINATE,LATITUDE,LONGITUDE
```

#### Decision Notices (CSV)
```
Job Number,Filepath,Planning Application Number,Adress,Decision Date,Decision Type,Document Type,BS7666UPRN,Easting,Northing
```

#### Land Charges (CSV)
```
Job Number,Filepath,Card Code,Address,BS7666UPRN,Easting,Northing
```

#### Enforcement Notices (CSV)
```
Job Number,Filepath,Planning Enforcement Reference Number,Address,Date,Document Type,BS7666UPRN,Easting,Northing
```

#### Agreements (CSV)
```
Job Number,Filepath,Address,Date,BS7666UPRN,Easting,Northing
```

---

## Appendix F: Abbreviation Expansion Rules

### F.1 Street Type Abbreviations

| Abbreviation | Expansion |
|--------------|-----------|
| RD | ROAD |
| ST | STREET |
| AVE | AVENUE |
| GDNS | GARDENS |
| CT | COURT |
| DR | DRIVE |
| LN | LANE |
| PL | PLACE |
| SQ | SQUARE |
| CRES | CRESCENT |
| TER | TERRACE |
| CL | CLOSE |
| PK | PARK |
| GRN | GREEN |
| WY | WAY |

### F.2 Building Type Abbreviations

| Abbreviation | Expansion |
|--------------|-----------|
| APT | APARTMENT |
| FLT | FLAT |
| BLDG | BUILDING |
| HSE | HOUSE |
| CTG | COTTAGE |
| FM | FARM |
| MNR | MANOR |
| VIL | VILLA |

### F.3 Area Type Abbreviations

| Abbreviation | Expansion |
|--------------|-----------|
| EST | ESTATE |
| INDL | INDUSTRIAL |
| CTR | CENTRE |

### F.4 Direction Abbreviations

| Abbreviation | Expansion |
|--------------|-----------|
| NTH | NORTH |
| STH | SOUTH |
| WST | WEST |

---

## Appendix G: Hampshire Locality Tokens

The following locality names are recognised for Hampshire:

```
ALTON, PETERSFIELD, LIPHOOK, WATERLOOVILLE, HORNDEAN,
BORDON, WHITEHILL, GRAYSHOTT, HEADLEY, BRAMSHOTT,
LINDFORD, HOLLYWATER, PASSFIELD, CONFORD, FOUR MARKS,
MEDSTEAD, CHAWTON, SELBORNE, EMPSHOTT, HAWKLEY,
LISS, STEEP, STROUD, BURITON, LANGRISH,
EAST MEON, WEST MEON, FROXFIELD, PRIVETT, ROPLEY,
WEST TISTED, EAST TISTED, BINSTED, HOLT POUND, BENTLEY,
FARNHAM, HASLEMERE
```

---

## Appendix H: Regular Expression Patterns

### H.1 Postcode Pattern

```regex
\b([A-Za-z]{1,2}\d[\dA-Za-z]?\s*\d[ABD-HJLNP-UW-Zabd-hjlnp-uw-z]{2})\b
```

### H.2 House Number Pattern

```regex
\b(\d+[A-Za-z]?)\b
```

### H.3 Flat/Unit Pattern

```regex
\b(FLAT|APT|APARTMENT|UNIT|STUDIO)\s+(\d+[A-Za-z]?)\b
```

### H.4 Abbreviation Word Boundary

```regex
\bABBREV\b
```

Example: `\bRD\b` matches "RD" as a complete word.

---

*End of Appendices*

---

*This thesis documents the EHDC LLPG Address Matching System - a comprehensive solution for matching historic document addresses to modern UPRNs using multi-layered matching algorithms.*
