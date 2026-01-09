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

## Appendix I: Spelling Correction Techniques for UK Addresses

This appendix details spelling correction techniques tailored for UK address components, addressing the mix of standard English, historical names, and regional variations.

### I.1 SymSpell Algorithm

SymSpell is based on the Symmetric Delete spelling correction algorithm. Its primary advantage is speed - it pre-calculates a dictionary of "deletes" from original dictionary terms, transforming complex search into simple dictionary lookup.

**Performance Characteristics:**

| Aspect | Characteristic |
|--------|----------------|
| Lookup | O(1) complexity for given max edit distance |
| Dictionary Generation | One-time preprocessing (minutes to hours for large dictionaries) |
| Memory | Large footprint (stores terms plus pre-calculated deletes) |

**Dictionary Management for Addresses:**

- Populate from authoritative sources (AddressBase, PAF)
- Create separate token lists for:
  - Towns (`BIRMINGHAM`, `PETERSFIELD`)
  - Street Names (`HIGH STREET`, `ABBEY ROAD`)
  - Dependent Thoroughfares (`THE MEWS`)
  - Street Suffixes (`ROAD`, `STREET`, `LANE`)
- Process multi-word names as single dictionary entries

**Implementation Example (Python):**

```python
from symspellpy import SymSpell, Verbosity

sym_spell = SymSpell(max_dictionary_edit_distance=2, prefix_length=7)
sym_spell.load_dictionary("address_tokens.txt", term_index=0, count_index=1)

# Correct a typo
input_term = "Brimingham"
suggestions = sym_spell.lookup(input_term, Verbosity.TOP, max_edit_distance=2)

if suggestions:
    best = suggestions[0]
    print(f"Correction: '{best.term}' with distance {best.distance}")

# Multi-word correction
input_phrase = "high stret"
suggestions_compound = sym_spell.lookup_compound(input_phrase, max_edit_distance=2)
```

**Recommendation:** Excellent for first-pass correction due to speed. Ideal for correcting common, low-edit-distance typos in single address tokens.

### I.2 Edit Distance Algorithms

**Damerau-Levenshtein:**
- Extension of Levenshtein distance
- Calculates insertions, deletions, substitutions, and transpositions
- Transpositions (`naem` to `name`) are common typing errors
- Complexity: O(m*n)

**Jaro-Winkler:**
- Designed for comparing short strings like names
- Based on matching characters and transpositions
- Bonus for strings sharing common prefix
- Complexity: O(m+n)

**Implementation Example (Python):**

```python
import jellyfish

term_a = "LEICESTER"
term_b = "LECESTER"  # Transposition typo
term_c = "LEISTER"   # Deletion typo

# Damerau-Levenshtein (lower is better)
dl_distance = jellyfish.damerau_levenshtein_distance(term_a, term_b)
print(f"D-L distance: {dl_distance}")  # Expected: 1

# Jaro-Winkler (higher is better, 0-1 scale)
jw_score = jellyfish.jaro_winkler_similarity(term_a, term_c)
print(f"J-W similarity: {jw_score:.4f}")
```

**Recommendation:** Jaro-Winkler highly recommended for scoring candidate matches. Its speed and prefix-weighting suit address data well.

### I.3 Phonetic Pre-filtering

Phonetic algorithms create a "key" for how a word sounds, filtering the dictionary to a manageable candidate set before applying expensive edit distance calculations.

**Algorithm Comparison:**

| Algorithm | Description | Use Case |
|-----------|-------------|----------|
| Soundex | Original, simple, Anglo-centric | Basic matching |
| Metaphone | Improved Soundex | Better accuracy |
| Double Metaphone | Produces primary and secondary keys | Recommended for addresses |

**Implementation Example:**

```python
import jellyfish

term1 = "Leicester"
term2 = "Lecester"
term3 = "Worcester"

dm1 = jellyfish.double_metaphone(term1)  # ('LSTR', '')
dm2 = jellyfish.double_metaphone(term2)  # ('LSTR', '')
dm3 = jellyfish.double_metaphone(term3)  # ('RSSTR', '')

if dm1 == dm2:
    print("Phonetically identical - good candidates for edit distance check")
```

**Recommendation:** Essential. Pre-calculate phonetic keys for entire address dictionary and store them. Reduces search space by orders of magnitude.

### I.4 Contextual Spelling Correction

Uses hierarchical address structure to validate corrections:

1. **Anchor on Postcode:** If valid, confidently identify Post Town and area
2. **Correct Town Name:** If postcode missing/invalid, correct using techniques above
3. **Restrict Street Search:** Once town identified, search only streets in that town
4. **Prevent Cross-Locality Errors:** Avoids correcting to wrong "CHURCH ROAD"

**Recommendation:** Critical for high-accuracy systems. Requires relational address data structure.

### I.5 Hybrid Pipeline (Recommended)

```
Input Address
    │
    ▼
┌─────────────────────────────────────────┐
│ 1. Preprocessing                        │
│    - Uppercase, remove punctuation      │
│    - Standardise suffixes (ST→STREET)   │
└─────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────┐
│ 2. Tokenisation & Component ID          │
│    - Split into tokens                  │
│    - Identify postcode, building number │
└─────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────┐
│ 3. Town Correction                      │
│    a. SymSpell fast lookup              │
│    b. If fails: Double Metaphone key    │
│    c. Score candidates with Jaro-Winkler│
└─────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────┐
│ 4. Contextual Street Correction         │
│    - Retrieve streets for known town    │
│    - Repeat Step 3 on smaller dictionary│
└─────────────────────────────────────────┘
    │
    ▼
Corrected Address
```

### I.6 Performance Analysis

For 50,000+ address comparisons against 1.5M token dictionary:

| Approach | Comparisons | Expected Time |
|----------|-------------|---------------|
| Brute-force (naive) | 75 billion | Hours/days |
| SymSpell only | 50,000 lookups | ~5 seconds |
| Hybrid (phonetic + J-W) | ~5 million | 10-30 seconds |

---

## Appendix J: UK Address Standards Reference

### J.1 BS7666 Standard

The British Standard BS7666 provides specification for spatial datasets and is the standard for UK geographic referencing.

**Key Components:**

| Component | Description |
|-----------|-------------|
| **BLPU** | Basic Land and Property Unit - contiguous area in single occupation |
| **UPRN** | Unique Property Reference Number - 12-digit identifier for every BLPU |
| **LPI** | Land and Property Identifier - links BLPU to specific address |
| **Street Record** | Defines streets, identified by USRN |
| **ESU** | Elementary Street Unit - street section between junctions |

**Compliance:**
- Local authorities maintain LLPGs in BS7666 compliance
- NLPG aggregates all LLPGs nationally
- NLPG now part of Ordnance Survey AddressBase products

### J.2 Royal Mail PAF Format

The Postcode Address File contains all known delivery addresses and postcodes in the UK.

**Field Definitions:**

| Field | Description | Required |
|-------|-------------|----------|
| Organisation Name | Company or organisation name | No |
| Department Name | Department within organisation | No |
| Sub Building Name | e.g., "Flat 3", "Apartment A" | No |
| Building Name | e.g., "The Old Rectory" | No |
| Building Number | Property number on street | No |
| Thoroughfare | Street name | No |
| Dependent Thoroughfare | Secondary street name | No |
| Dependent Locality | Smaller settlement within larger | No |
| Double Dependent Locality | Further locality detail | No |
| Post Town | Main town for postal delivery | **Yes** |
| Postcode | Address postcode | **Yes** |
| PO Box | PO Box number | No |
| UDPRN | Unique Delivery Point Reference Number | Auto |

**Canonical Representation:**
- All data in UPPERCASE
- Post Town and Postcode only mandatory elements
- County no longer required
- Property with name and number: prefer number
- Postcode always last line

### J.3 Ordnance Survey AddressBase Products

| Product | Description | Use Case |
|---------|-------------|----------|
| AddressBase Core | Simple flat file with UPRN and coordinates | Basic address lists |
| AddressBase Plus | Core plus multi-occupancy and non-postal objects | Detailed views |
| AddressBase Premium | Most detailed: current, historic, alternative addresses | Analysis, emergency services |
| AddressBase Islands | Isle of Man and Channel Islands | Crown Dependencies |

### J.4 GeoPlace LLPG Structure

**Mandatory Fields:**

| Field | Description |
|-------|-------------|
| UPRN | Unique Property Reference Number |
| USRN | Unique Street Reference Number |
| Logical Status | Lifecycle stage (Approved, Under Construction, In Use, Demolished) |
| BLPU State | Physical state of property |
| Custodian Code | Local authority identifier |
| Start Date / End Date | Record validity period |
| Street Descriptor | Street name |
| Town Name / Locality Name | Location identifiers |
| PAO / SAO | Primary/Secondary Addressable Object (building/sub-building) |

### J.5 UPRN Validation

- **Structure:** Integer up to 12 digits
- **Persistence:** Unique and persistent for every addressable location
- **Assignment:** By local authorities, validated by GeoPlace
- **Validation:** No mathematical checksum; validate against authoritative source

### J.6 Postcode Format Rules

**Structure:**
```
[Outward Code] [Inward Code]
   2-4 chars     3 chars
```

| Part | Characters | Purpose |
|------|------------|---------|
| Outward Code | 2-4 | Postal area and district |
| Inward Code | 3 | Postal sector and unit |

**Formatting Rules:**
- Always UPPERCASE
- Single space between outward and inward codes
- No other punctuation

**Validation Regex:**
```regex
^[A-Z]{1,2}[0-9][A-Z0-9]? [0-9][A-Z]{2}$
```

True validation requires checking against PAF or ONS Postcode Directory.

### J.7 Address Example Formats

**Standard Address:**
```
Miss S Pollard
7 Gipsy Hill
LONDON
SE19 1QG
```

**With Building Name:**
```
Mr A Jones
The Old School House
Main Street
Anytown
AN1 1AA
```

**With Flat:**
```
Mr B Smith
Flat 2
34 The High Street
Anytown
AN1 1AB
```

**PAF to BS7666 Mapping Example:**

PAF: `10, DOWNING STREET, LONDON, SW1A 2AA`

BS7666:
| Field | Value |
|-------|-------|
| SAO_TEXT | (empty) |
| PAO_START_NUMBER | 10 |
| STREET_DESCRIPTOR | DOWNING STREET |
| TOWN_NAME | LONDON |
| ADMINISTRATIVE_AREA | CITY OF WESTMINSTER |
| POSTCODE_LOCATOR | SW1A 2AA |
| UPRN | 100023336956 |

---

## Appendix K: External Data Sources for Address Enrichment

### K.1 Ordnance Survey Products

**Products:**

| Product | Data | Cost | Updates |
|---------|------|------|---------|
| AddressBase Premium | 40M+ addresses, UPRNs, precise coordinates | Paid (PSGA for public sector) | 6 weeks |
| AddressBase Core | UPRNs linked to single address/coordinate | Paid | Weekly |
| AddressBase Plus | Mid-tier with PAF linkage | Paid | 6 weeks |
| OS Open UPRN | All UPRNs with coordinates (no address text) | Free (OGL) | Quarterly |
| OS OpenMap Local | Street-level vector map | Free (OGL) | Varies |

**Format:** CSV or GML with structured columns for address components, UPRNs, coordinates (eastings/northings).

**API:** OS Places API (paid, RESTful JSON) - address/postcode lookups, reverse geocoding, UPRN queries.

**Use Cases:**
- Authoritative verification (gold standard)
- UPRN enrichment
- Geocoding
- De-duplication

### K.2 Royal Mail PAF

**Format:** Flat-file CSV or fixed-width text, structured address components, all uppercase.

**Updates:** Master updated daily; licensees access daily/weekly/monthly/quarterly.

**Cost:** Commercial license required (hundreds to thousands of pounds depending on scale). Public sector via PSGA. Free/discounted for charities/microbusinesses.

**API:** No direct Royal Mail API. Third-party providers offer:
- Postcode lookup
- Address autocomplete
- Address validation

**Use Cases:**
- Postal address validation
- Data cleansing
- Web form entry
- UDPRN linkage

### K.3 GeoPlace / LLPG / NLPG

**Structure:**
- LLPG: Local authority definitive address database (BS7666 compliant)
- NLPG: National aggregation of all LLPGs
- NAG: Combined with PAF to create AddressBase

**Updates:** Daily (local authority custodians update as part of street naming duties).

**Access:** Not directly available to public/commercial. Access via AddressBase products.

**Use Cases:**
- Source of truth for government
- Lifecycle information
- Authoritative UPRN creation

### K.4 Historical Gazetteers

**Sources:**

| Source | Content | Format | Cost |
|--------|---------|--------|------|
| NHLE (Historic England) | Listed buildings database | CSV, Shapefile, GML, GeoJSON | Free (OGL) |
| British Listed Buildings | Comprehensive online database | Web browsing | Free |
| A Vision of Britain | Historical maps, census data, gazetteers | KML, Shapefile, web | Non-commercial use |

**API:** NHLE provides FeatureServer endpoint.

**Use Cases:**
- Resolving ambiguous historical addresses
- Property name verification
- Enrichment with listed status
- Identifying historic vs. "vanity" names

### K.5 OpenStreetMap (OSM)

**Format:** XML-based (`.osm.pbf` compressed), available as GeoJSON, XML. Tagged with key-value pairs (`addr:housenumber=10`).

**Access Methods:**
- Bulk download from Geofabrik
- Overpass API (query specific features)
- Nominatim API (geocoding)

**Updates:** Continuous (edits go live within minutes).

**Cost:** Free under ODbL (attribution required).

**API Usage Policy:** Public Nominatim max 1 req/sec; heavy use requires own instance.

**Use Cases:**
- Non-authoritative geocoding
- Address discovery
- Cost-effective solution
- Reverse geocoding

**Caveat:** Completeness and accuracy significantly lower than OS/PAF. Not suitable for authoritative validation.

### K.6 Companies House

**Format:** RESTful API returning JSON. Well-structured `registered_office_address` object.

**Updates:** Near real-time.

**Cost:** Free. Requires API key registration. Rate limit: 600 requests per 5 minutes.

**Use Cases:**
- Business address verification (KYB)
- Distinguishing commercial vs. residential
- Finding trading vs. legal addresses
- Data enrichment with company details

### K.7 Land Registry INSPIRE Index Polygons

**Format:** GML (for GIS software). Each polygon has unique `Land_Registry_INSPIRE_ID`.

**Updates:** Monthly.

**Cost:** Free (OGL).

**Access:** Bulk downloads by local authority area (no API).

**Use Cases:**
- Linking property titles to UPRNs (via UPRN-INSPIRE ID lookup)
- Resolving address ambiguity via parcel visualisation
- Identifying unaddressed land
- Boundary confirmation

### K.8 Census Geography Lookups (OA, LSOA, MSOA)

**Hierarchy:**

| Level | Typical Size |
|-------|--------------|
| OA (Output Area) | 40-250 households |
| LSOA (Lower Layer Super Output Area) | 400-1,200 households |
| MSOA (Middle Layer Super Output Area) | 2,000-6,000 households |

**Format:** CSV lookup tables (Postcode to OA). Boundary data in Shapefile, GeoJSON.

**Updates:** Per census (every 10 years).

**Cost:** Free (OGL) from ONS Open Geography portal.

**Use Cases:**
- Data enrichment with statistical area codes
- Geographic/demographic analysis
- Service area definition
- Not for finding addresses - for adding context

---

## Appendix L: Active Learning for Address Matching

### L.1 Fundamentals

Active Learning lets the model identify the most informative data points from an unlabeled pool and request human annotation. For address matching, this means intelligently selecting tricky address pairs for human review.

**Core Strategies:**

| Strategy | Description | Best For |
|----------|-------------|----------|
| Uncertainty Sampling | Select examples with least confident predictions | Most common, simplest |
| Query-by-Committee | Multiple models vote; select where they disagree | Diverse model ensemble |
| Expected Model Change | Select examples causing greatest model parameter change | Advanced, computationally expensive |

### L.2 Address Matching Specific Tactics

**Score-Based Routing:**

| Score Range | Confidence | Action |
|-------------|------------|--------|
| >= 0.95 | High (Match) | Auto-confirm |
| 0.20 - 0.95 | Uncertain | **Queue for review** |
| < 0.20 | High (Non-match) | Auto-reject |

Most valuable pairs: scores between 0.4 - 0.6 (abbreviations, missing directionals, typos).

**Confidence Threshold Tuning:** Use labeled review data to precisely simulate threshold adjustments: "If I lower auto-match from 0.95 to 0.92, how many additional correct matches vs. false positives?"

### L.3 Feedback Loop Architecture

```
                                ┌─────────────────┐    ┌──────────────────┐
[New Address Data] → [Matcher] →│ Score > 0.95    │───→│ Auto-confirm     │
                                └─────────────────┘    └──────────────────┘
                                        │
                                        │ (Score 0.2-0.95)
                                        ▼
                                ┌─────────────────┐    ┌──────────────────┐
                                │ Uncertain       │───→│ Review Queue     │
                                └─────────────────┘    └──────────────────┘
                                        │                       │
                                        │ (Score < 0.2)         │ (Human Decision)
                                        ▼                       ▼
                                ┌─────────────────┐    ┌──────────────────┐
                                │ Auto-reject     │    │ Manual Review UI │
                                └─────────────────┘    └──────────────────┘
                                        │                       │
                                        │ (Periodically)        ▼
┌─────────────────┐    ┌────────────────────┐    ┌─────────────────────┐
│ Updated Model   │←───│ Retraining Pipeline │←───│ Labeled Pairs (DB)  │
└─────────────────┘    └────────────────────┘    └─────────────────────┘
        │
        ▼
[A/B Test Module]
```

**Code Pattern:**

```python
UNCERTAINTY_LOWER = 0.20
UNCERTAINTY_UPPER = 0.95

def process_address_pair(addr1, addr2):
    score = model.predict(addr1, addr2)

    if score >= UNCERTAINTY_UPPER:
        db.confirm_match(addr1, addr2)
    elif score < UNCERTAINTY_LOWER:
        db.reject_match(addr1, addr2)
    else:
        # Active learning: queue for human review
        db.add_to_review_queue(
            address1=addr1,
            address2=addr2,
            model_score=score
        )

def retrain_from_feedback():
    labeled_data = db.get_reviewed_pairs(since_last_training=True)
    new_model = model.train(existing_data + labeled_data)
    model.save(new_model, version="v2.1.0")
```

### L.4 UI Design for Efficient Manual Review

**Design Principles:**

| Element | Recommendation |
|---------|----------------|
| Batch Review | Present 10-20 pairs per screen |
| Diff Highlighting | Colour-code differing tokens |
| Context Display | Show customer name, order date, geocodes |
| Keyboard Shortcuts | `M`=Match, `N`=Not Match, `S`=Skip, Arrows=Navigate |
| Clear Actions | Large, unambiguous [Match] and [Not a Match] buttons |

### L.5 Metrics to Track

| Metric | Purpose |
|--------|---------|
| Precision/Recall Curves | Track model improvement per retraining cycle |
| Pairs Reviewed per Hour | Measure UI efficiency |
| Model Improvement per 1000 Reviews | Calculate ROI of manual review |
| Confidence Distribution | Track shift toward bimodal distribution |

### L.6 Transfer Learning from Reviews

**Benefits:**
- Fine-tune `UNCERTAINTY_LOWER` and `UNCERTAINTY_UPPER` thresholds
- Discover new matching patterns (regional PO Box conventions)
- Identify missing abbreviations (`Blvd` vs. `Bvd`)

### L.7 Handling Reviewer Disagreement

| Approach | When to Use |
|----------|-------------|
| Consensus (3+ reviewers, majority vote) | Critical decisions |
| Adjudication Queue | Pairs where reviewers disagree; escalate to senior |
| Exclude from Training | If disagreements rare, treat as noise |

### L.8 Production Examples

| Domain | Application |
|--------|-------------|
| E-commerce | Customer deduplication from sign-ups/checkouts |
| Finance (KYC/AML) | Sanctions list matching with high-precision model |
| Geospatial | POI consolidation from multiple listing sources |

---

## Appendix M: Multi-Source Address Resolution Best Practices

### M.1 Master Data Management Patterns

**Golden Record Creation:**

1. **Data Ingestion:** Collect from all source systems
2. **Standardisation:** Parse, standardise, correct
3. **Matching:** Identify records referring to same address
4. **Survivorship:** Select best-of-breed attribute values

**Source-of-Truth Hierarchy:**

Define relative trustworthiness of sources. Example:
```
LLPG (highest) > AddressBase > PAF > Planning System > Legacy Records (lowest)
```

**Conflict Resolution Strategies:**

| Strategy | Description |
|----------|-------------|
| Voting/Consensus | Most frequent value across sources wins |
| Weighted Scoring | Score sources by historical accuracy |
| Manual Stewardship | Data steward resolves unresolvable conflicts |

### M.2 Entity Resolution Algorithms

**Probabilistic Matching:**
- String similarity metrics (Jaro-Winkler, Levenshtein, n-gram)
- Machine learning models classifying pairs as match/non-match

**Deterministic Rules:**
- Predefined rules (e.g., same UPRN = match)
- Fast but brittle if data not perfectly clean

**Hybrid Approaches:**
- Deterministic for obvious matches/non-matches
- Probabilistic for ambiguous cases

### M.3 Handling Conflicting Data

**Version Control:**
- Track every change to golden record
- Enable auditing and rollback

**Temporal Validity:**
- Store validity period for each address/attribute
- Accurately represent state at any point in time

**Provenance Tracking:**
- Record origin of each piece of data
- Store source system and record reference
- Essential for understanding data lineage

### M.4 Data Quality Scoring

| Dimension | Assessment |
|-----------|------------|
| Completeness | Are all required components present? |
| Accuracy | Does address correspond to real location? (validate against LLPG) |
| Timeliness | How recently updated/verified? |
| Consistency | Represented consistently across systems? |

### M.5 Deduplication Strategies

**Blocking:**
- Divide dataset into smaller blocks (e.g., by postcode prefix)
- Only compare records within same block

**Clustering:**
- Group similar records together
- Detailed pairwise comparison within clusters

**Optimisation Techniques:**

| Technique | Description |
|-----------|-------------|
| Sorted Neighbourhood | Sort by key, compare within sliding window |
| Canopy Clustering | Fast approximate clustering for preprocessing |

### M.6 Incremental vs. Full Reprocessing

| Approach | Description | Trade-offs |
|----------|-------------|------------|
| CDC (Change Data Capture) | Capture and propagate changes in near-real-time | Complex but current |
| Delta Processing | Process only new/changed data in batches | Efficient |
| Full Reprocessing | Re-ingest and reprocess all data | Simple, corrects accumulated errors |

**Recommendation:** Hybrid approach - incremental for daily changes, periodic full reprocessing for quality assurance.

### M.7 Audit Trails and Lineage

Store metadata for every golden record:
- Source system and source record ID for every attribute
- Date and time of every update
- Rules/algorithms used for matching decisions
- Data steward who resolved manual conflicts

### M.8 Production Architectures

**Batch Pipeline:**

```
┌─────────────┐
│ Source 1    │──┐
├─────────────┤  │    ┌─────────────┐    ┌──────────────────┐    ┌─────────────────┐
│ Source 2    │──┼───→│ ETL Process │───→│ Address Pipeline │───→│ Master Address  │
├─────────────┤  │    └─────────────┘    └──────────────────┘    └─────────────────┘
│ Source 3    │──┘
└─────────────┘
```

**Streaming Architecture:**

```
┌─────────────┐
│ Source 1    │──CDC──┐
├─────────────┤       │    ┌───────────────┐    ┌──────────────────┐    ┌─────────────────┐
│ Source 2    │──CDC──┼───→│ Message Queue │───→│ Stream Processor │───→│ Master Address  │
├─────────────┤       │    └───────────────┘    └──────────────────┘    └─────────────────┘
│ Source 3    │──CDC──┘
└─────────────┘
```

**Data Lake Pattern:**

```
┌─────────────┐
│ Source 1    │──┐
├─────────────┤  │    ┌─────────────┐    ┌──────────────────┐
│ Source 2    │──┼───→│ Data Lake   │───→│ Resolution Engine│──┐
├─────────────┤  │    │ (Raw Data)  │    └──────────────────┘  │
│ Source 3    │──┘    └─────────────┘                          │
                            ▲                                   │
                            │ (Golden Records)                  │
                            └───────────────────────────────────┘
```

---

*End of Appendices*

---

*This thesis documents the EHDC LLPG Address Matching System - a comprehensive solution for matching historic document addresses to modern UPRNs using multi-layered matching algorithms.*
