# PROJECT IMPLEMENTATION PLAN

## Executive Summary

This plan implements the EHDC LLPG address matching system to populate historic document records with modern UPRNs and coordinates. The system will process 129,701 historic records across 4 datasets with ~78% missing location data, using a sophisticated multi-stage matching algorithm combining deterministic, fuzzy, semantic, phonetic, and spatial techniques.

**Success Metrics:**
- Auto-accept precision ≥98% 
- Overall coverage uplift from ~22% to ~80%+
- All decisions auditable with explainable scoring
- Processing time <600ms per query

---

## Phase 1: Database Schema & Infrastructure (Week 1)

### 1.1 Core Extensions & Setup
```sql
-- Required PostgreSQL extensions
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
```

### 1.2 Source Data Types
```sql
CREATE TYPE source_type AS ENUM ('decision', 'land_charge', 'enforcement', 'agreement');
CREATE TYPE data_origin AS ENUM ('original', 'calculated', 'validated');
```

### 1.3 Master Address Dimension (LLPG)
```sql
CREATE TABLE dim_address (
  uprn            text PRIMARY KEY,
  locaddress      text NOT NULL,
  addr_can        text GENERATED ALWAYS AS (
                    upper(regexp_replace(
                      regexp_replace(locaddress, '\b([A-Z]{1,2}[0-9][0-9A-Z]?\s*[0-9][A-Z]{2})\b', '', 'gi'),
                      '[^A-Z0-9 ]', ' ', 'g'
                    ))
                  ) STORED,
  easting         numeric NOT NULL,
  northing        numeric NOT NULL,
  usrn            text,
  blpu_class      text,
  postal_flag     text,
  status          text,
  geom27700       geometry(Point, 27700),
  geom4326        geometry(Point, 4326),
  created_at      timestamptz DEFAULT now()
);

-- Critical indexes for performance
CREATE INDEX dim_address_addr_can_trgm_idx ON dim_address USING gin (addr_can gin_trgm_ops);
CREATE INDEX dim_address_geom27700_idx ON dim_address USING gist (geom27700);
CREATE INDEX dim_address_uprn_idx ON dim_address (uprn);
```

### 1.4 Unified Source Document Table
```sql
CREATE TABLE src_document (
  src_id          bigserial PRIMARY KEY,
  source_type     source_type NOT NULL,
  job_number      text,
  filepath        text,
  external_ref    text,           -- Planning ref, card code, enforcement ref, etc
  doc_type        text,
  doc_date        date,
  raw_address     text NOT NULL,  -- Original address as-is
  addr_can        text,           -- Canonicalized address
  postcode_text   text,           -- Extracted postcode
  uprn_raw        text,           -- Original UPRN (may be invalid)
  easting_raw     numeric,        -- Original coordinates
  northing_raw    numeric,
  uprn_origin     data_origin DEFAULT 'original',    -- Track data provenance
  easting_origin  data_origin DEFAULT 'original',
  northing_origin data_origin DEFAULT 'original',
  created_at      timestamptz DEFAULT now()
);

-- Performance indexes
CREATE INDEX src_document_addr_can_trgm_idx ON src_document USING gin (addr_can gin_trgm_ops);
CREATE INDEX src_document_source_type_idx ON src_document (source_type);
CREATE INDEX src_document_postcode_idx ON src_document (postcode_text);
CREATE INDEX src_document_uprn_raw_idx ON src_document (uprn_raw);
```

### 1.5 Matching Audit & Results Tables
```sql
CREATE TABLE match_run (
  run_id          bigserial PRIMARY KEY,
  run_started_at  timestamptz DEFAULT now(),
  run_completed_at timestamptz,
  run_label       text NOT NULL,           -- e.g. "v1.0-deterministic"
  algorithm_version text,
  notes           text,
  total_processed int,
  auto_accepted   int,
  needs_review    int,
  rejected        int
);

CREATE TABLE match_result (
  match_id        bigserial PRIMARY KEY,
  run_id          bigint REFERENCES match_run(run_id),
  src_id          bigint REFERENCES src_document(src_id),
  candidate_uprn  text REFERENCES dim_address(uprn),
  method          text NOT NULL,           -- 'valid_uprn', 'addr_exact', 'trgm_0.90', 'vector', etc
  score           numeric(4,3),            -- 0.000 to 1.000
  confidence      numeric(4,3),            -- Algorithm confidence
  tie_rank        int DEFAULT 1,           -- 1=best candidate
  features        jsonb,                   -- All computed features for explainability
  decided         boolean DEFAULT false,
  decision        text,                    -- 'auto_accepted', 'needs_review', 'rejected'
  decided_by      text DEFAULT 'system',
  decided_at      timestamptz,
  notes           text
);

CREATE TABLE match_accepted (
  src_id          bigint PRIMARY KEY REFERENCES src_document(src_id),
  uprn            text REFERENCES dim_address(uprn),
  method          text NOT NULL,
  score           numeric(4,3),
  confidence      numeric(4,3),
  run_id          bigint REFERENCES match_run(run_id),
  accepted_by     text DEFAULT 'system',
  accepted_at     timestamptz DEFAULT now()
);

CREATE TABLE match_override (
  override_id     bigserial PRIMARY KEY,
  src_id          bigint REFERENCES src_document(src_id),
  uprn            text REFERENCES dim_address(uprn),
  reason          text,
  created_by      text,
  created_at      timestamptz DEFAULT now()
);
```

### 1.6 Address Normalization Rules
```sql
CREATE TABLE address_normalise_rule (
  rule_id         bigserial PRIMARY KEY,
  pattern         text NOT NULL,           -- Regex pattern
  replacement     text NOT NULL,           -- Replacement text
  rule_type       text DEFAULT 'abbreviation', -- 'abbreviation', 'cleanup', 'postcode'
  enabled         boolean DEFAULT true,
  weight          int DEFAULT 0,
  created_at      timestamptz DEFAULT now()
);

-- Seed with common UK abbreviations
INSERT INTO address_normalise_rule (pattern, replacement, rule_type) VALUES
  ('\bRD\b', 'ROAD', 'abbreviation'),
  ('\bST\b', 'STREET', 'abbreviation'),
  ('\bAVE\b', 'AVENUE', 'abbreviation'),
  ('\bGDNS\b', 'GARDENS', 'abbreviation'),
  ('\bCT\b', 'COURT', 'abbreviation'),
  ('\bDR\b', 'DRIVE', 'abbreviation'),
  ('\bLN\b', 'LANE', 'abbreviation'),
  ('\bPL\b', 'PLACE', 'abbreviation'),
  ('\bSQ\b', 'SQUARE', 'abbreviation'),
  ('\bCRES\b', 'CRESCENT', 'abbreviation'),
  ('\bTER\b', 'TERRACE', 'abbreviation'),
  ('\bCL\b', 'CLOSE', 'abbreviation'),
  ('\bPK\b', 'PARK', 'abbreviation');
```

---

## Phase 2: Data Import & ETL (Week 1-2)

### 2.1 LLPG Migration from Current Schema
```sql
-- Migrate existing ehdc_addresses to dim_address
INSERT INTO dim_address (uprn, locaddress, easting, northing, usrn, blpu_class, postal_flag, status, geom27700, geom4326)
SELECT 
  bs7666uprn::text,
  locaddress,
  easting,
  northing,
  bs7666usrn::text,
  blpuclass,
  postal,
  lgcstatusc,
  ST_SetSRID(ST_Point(easting::float8, northing::float8), 27700),
  ST_Transform(ST_SetSRID(ST_Point(easting::float8, northing::float8), 27700), 4326)
FROM ehdc_addresses 
WHERE bs7666uprn IS NOT NULL AND easting IS NOT NULL AND northing IS NOT NULL;

-- Update canonical addresses
UPDATE dim_address SET addr_can = generate_canonical_address(locaddress);
```

### 2.2 Source Document Import Strategy
**Go ETL Application Structure:**
```
cmd/
  importer/
    main.go                 -- CLI for data import
internal/
  normalize/
    address.go             -- Address canonicalization
    postcode.go            -- Postcode extraction
    rules.go               -- Normalization rules engine
  import/
    decision_notices.go    -- Import decision notices CSV
    land_charges.go        -- Import land charges CSV  
    enforcement.go         -- Import enforcement notices CSV
    agreements.go          -- Import agreements CSV
    common.go              -- Common CSV parsing utilities
  db/
    connection.go          -- Database connection handling
    migrations/            -- SQL migration files
```

### 2.3 Import Mapping Specifications

#### Decision Notices (76,167 records)
- **Source Type**: 'decision'
- **External Ref**: Planning Application Number
- **Raw Address**: 'Adress' column (preserve typo)
- **Date**: Decision Date -> doc_date
- **Original Data Flags**: Mark UPRN/coordinates as 'original' if present

#### Land Charges Cards (49,760 records)  
- **Source Type**: 'land_charge'
- **External Ref**: Card Code
- **Raw Address**: 'Address' column
- **Note**: ~60% have original UPRNs/coordinates

#### Enforcement Notices (1,172 records)
- **Source Type**: 'enforcement' 
- **External Ref**: Planning Enforcement Reference Number
- **Raw Address**: 'Address' column
- **Date**: Date -> doc_date

#### Agreements (2,602 records)
- **Source Type**: 'agreement'
- **External Ref**: Generate from filepath
- **Raw Address**: 'Address' column  
- **Date**: Date -> doc_date

### 2.4 Address Canonicalization Process
```go
// Canonical address generation rules:
// 1. Extract and store postcode separately
// 2. Convert to uppercase
// 3. Apply abbreviation expansion rules
// 4. Remove punctuation except in flat/unit numbers
// 5. Collapse whitespace
// 6. Preserve house numbers and alpha suffixes

func CanonicalizeAddress(raw string) (canonical, postcode string) {
    // Implementation following PROJECT_SPECIFICATION requirements
}
```

---

## Phase 3: Multi-Stage Matching Algorithm (Week 2-4)

### 3.1 Stage 1: Deterministic Matching

#### 3.1.1 Legacy UPRN Validation
```go
// Validate existing UPRNs against LLPG
// Score: 1.000 (perfect match)
// Auto-accept if UPRN exists in dim_address
```

#### 3.1.2 Canonical Exact Match  
```go
// Match src_document.addr_can = dim_address.addr_can
// Score: 0.990 (high confidence)
// Auto-accept if single result
// Review if multiple candidates
```

### 3.2 Stage 2: Fuzzy String Matching (PostgreSQL pg_trgm)

#### 3.2.1 Trigram Similarity Tiers
```sql
-- High confidence: similarity >= 0.90
-- Auto-accept if unique or margin >= 0.03 to next candidate
SELECT uprn, locaddress, similarity(?, addr_can) AS trgm_score
FROM dim_address  
WHERE addr_can %% ?
AND similarity(?, addr_can) >= 0.90
ORDER BY trgm_score DESC LIMIT 10;

-- Medium confidence: 0.85 <= similarity < 0.90
-- Auto-accept only with house number + locality match + margin >= 0.05

-- Low confidence: 0.80 <= similarity < 0.85  
-- Always requires review
```

#### 3.2.2 Enhanced Filtering
- **Locality Filter**: Require overlap of town/village tokens (ALTON, PETERSFIELD, etc.)
- **House Number Filter**: Match primary address numbers (1, 12A, FLAT 2, etc.)
- **Phonetic Filter**: Double Metaphone on street names to catch misspellings
- **USRN Proximity**: Prefer same street (if available)

### 3.3 Stage 3: Semantic Vector Matching

#### 3.3.1 Infrastructure Setup
- **Vector Database**: Qdrant (Docker container)  
- **Embedding Model**: Ollama with `bge-small-en` or `all-minilm-l6-v2`
- **Vector Dimensions**: 384 (bge-small-en) or 512 (all-minilm-l6-v2)

#### 3.3.2 Vector Pipeline
```go
// 1. Generate embeddings for all dim_address.addr_can
// 2. Index in Qdrant with HNSW
// 3. For each query, embed src_document.addr_can  
// 4. Query top-K similar addresses
// 5. Merge with trigram results (union by UPRN)
```

### 3.4 Stage 4: Spatial Proximity (When Available)

#### 3.4.1 Spatial Boosting
```go
// If source has easting_raw/northing_raw:
// 1. Compute distance to candidate UPRN coordinates
// 2. Apply boost: spatial_boost = exp(-distance_meters / 300.0)
// 3. Filter candidates beyond 2km (configurable)
// 4. Prefer candidates within 100m significantly
```

### 3.5 Meta-Scoring Algorithm

#### 3.5.1 Feature Computation
```go
type MatchFeatures struct {
    // String similarity features
    TrgramScore     float64 `json:"trgm_score"`
    JaroScore       float64 `json:"jaro_score"`  
    LevenshteinNorm float64 `json:"levenshtein_norm"`
    EmbeddingCos    float64 `json:"embedding_cos"`
    
    // Structural features
    SameHouseNumber bool    `json:"same_house_num"`
    SameHouseAlpha  bool    `json:"same_house_alpha"`
    LocalityOverlap float64 `json:"locality_overlap"`
    StreetOverlap   float64 `json:"street_overlap"`
    
    // Meta features  
    USRNMatch       bool    `json:"usrn_match"`
    LLPGStatusLive  bool    `json:"llpg_status_live"`
    SpatialBoost    float64 `json:"spatial_boost"`
    DistanceMeters  float64 `json:"distance_meters,omitempty"`
    
    // Algorithm features
    LegacyUPRNValid bool    `json:"legacy_uprn_valid"`
    PhoneticHits    int     `json:"phonetic_hits"`
    DescriptorPenalty bool  `json:"descriptor_penalty"`
}
```

#### 3.5.2 Scoring Formula
```go
func ComputeScore(features MatchFeatures) float64 {
    score := 0.0
    
    // Primary similarity (90% weight)
    score += 0.45 * features.TrgramScore
    score += 0.45 * features.EmbeddingCos
    
    // Structure bonuses (10% weight)
    score += 0.03 * features.LocalityOverlap
    score += 0.03 * features.StreetOverlap
    
    // Discrete bonuses
    if features.SameHouseNumber { score += 0.08 }
    if features.SameHouseAlpha  { score += 0.02 }
    if features.USRNMatch       { score += 0.04 }
    if features.LLPGStatusLive  { score += 0.03 }
    if features.LegacyUPRNValid { score += 0.20 }
    
    // Spatial boost (distance-dependent)
    score += features.SpatialBoost
    
    // Penalties
    if features.PhoneticHits == 0    { score -= 0.03 }
    if features.DescriptorPenalty    { score -= 0.05 }
    
    // Clamp to [0.0, 1.0]
    return math.Min(1.0, math.Max(0.0, score))
}
```

#### 3.5.3 Decision Thresholds
```go
const (
    AutoAcceptThreshold    = 0.92   // Auto-accept with margin >= 0.03
    AutoAcceptLowThreshold = 0.88   // With house number + locality + margin >= 0.05
    ReviewThreshold        = 0.80   // Manual review required
    RejectThreshold        = 0.80   // Below this = rejected
)

func MakeDecision(candidates []Candidate) (string, string) {
    if len(candidates) == 0 {
        return "rejected", ""
    }
    
    best := candidates[0]
    margin := 0.0
    if len(candidates) > 1 {
        margin = best.Score - candidates[1].Score
    }
    
    // High confidence auto-accept
    if best.Score >= AutoAcceptThreshold && margin >= 0.03 {
        return "auto_accepted", best.UPRN
    }
    
    // Medium confidence with structural matches
    if best.Score >= AutoAcceptLowThreshold && 
       best.Features.SameHouseNumber && 
       best.Features.LocalityOverlap >= 0.5 && 
       margin >= 0.05 {
        return "auto_accepted", best.UPRN
    }
    
    // Review queue
    if best.Score >= ReviewThreshold {
        return "needs_review", ""
    }
    
    return "rejected", ""
}
```

---

## Phase 4: System Integration (Week 4-5)

### 4.1 Go Application Architecture
```
cmd/
  matcher/
    main.go                -- CLI interface
    commands/
      ingest.go           -- Data import commands
      match.go            -- Matching pipeline commands  
      report.go           -- Reporting commands
      export.go           -- Export commands
      
internal/
  engine/
    matcher.go            -- Core matching engine
    pipeline.go           -- Multi-stage pipeline orchestration
    
  generator/
    deterministic.go      -- Stage 1: Legacy UPRN + exact match
    fuzzy.go              -- Stage 2: pg_trgm matching
    vector.go             -- Stage 3: Embedding similarity
    spatial.go            -- Stage 4: Spatial proximity
    
  scorer/
    features.go           -- Feature computation
    scoring.go            -- Meta-scoring algorithm
    decisions.go          -- Decision logic
    
  services/
    qdrant.go            -- Vector database client
    ollama.go            -- Embedding model client
    postgres.go          -- Database operations
    
  normalize/
    address.go           -- Address canonicalization
    postcode.go          -- Postcode extraction
    rules.go             -- Normalization rules
    
  export/
    csv.go               -- CSV export functionality
    reports.go           -- Match quality reports
```

### 4.2 CLI Commands
```bash
# Data import
./matcher ingest --source decision --file source_docs/decision_notices.csv
./matcher ingest --source land_charge --file source_docs/land_charges_cards.csv  
./matcher ingest --source enforcement --file source_docs/enforcement_notices.csv
./matcher ingest --source agreement --file source_docs/agreements.csv

# Vector index building
./matcher index-vectors --model bge-small-en

# Matching pipeline
./matcher run --label v1.0-deterministic --stage deterministic
./matcher run --label v1.1-fuzzy --stage fuzzy --min-score 0.80
./matcher run --label v1.2-vector --stage vector --use-embeddings

# Reporting  
./matcher report summary --run-label v1.2-vector
./matcher report coverage --by-source
./matcher report quality --sample-size 100

# Export results
./matcher export csv --output-dir ./results
./matcher export review-queue --output ./review.csv
```

### 4.3 Docker Compose Stack
```yaml
version: '3.8'
services:
  db:
    image: duvel/postgis:12-2.5-arm64
    # ... existing config
    
  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
    volumes:
      - qdrant_storage:/qdrant/storage
      
  ollama:
    image: ollama/ollama:latest  
    ports:
      - "11434:11434"
    volumes:
      - ollama_data:/root/.ollama
    command: serve
    
  matcher:
    build: .
    depends_on:
      - db
      - qdrant
      - ollama
    environment:
      - DATABASE_URL=postgres://user:password@db:5432/ehdc_gis
      - QDRANT_URL=http://qdrant:6333
      - OLLAMA_URL=http://ollama:11434
      
volumes:
  qdrant_storage:
  ollama_data:
```

---

## Phase 5: Quality Assurance & Optimization (Week 5-6)

### 5.1 Accuracy Validation Framework

#### 5.1.1 Gold Standard Dataset Creation
```go
// Create validation set from high-confidence matches
// 1. All legacy UPRN matches that validate against LLPG
// 2. Sample of exact canonical matches (human verified)
// 3. Manual review of 500 random fuzzy matches
// 4. Geographic distribution across all localities

type ValidationRecord struct {
    SrcID      int64  `json:"src_id"`
    TrueUPRN   string `json:"true_uprn"`
    Confidence string `json:"confidence"` // "certain", "probable", "uncertain"
    Notes      string `json:"notes"`
}
```

#### 5.1.2 Precision/Recall Measurement
```go
func EvaluateMatches(goldSet []ValidationRecord, results []MatchResult) Metrics {
    return Metrics{
        Precision:    correctMatches / totalAutoAccepted,
        Recall:       correctMatches / totalPossibleMatches,
        F1Score:      2 * precision * recall / (precision + recall),
        Coverage:     recordsWithMatch / totalRecords,
        Accuracy:     (truePositives + trueNegatives) / totalPredictions,
    }
}
```

### 5.2 Performance Optimization

#### 5.2.1 Database Tuning
```sql
-- Additional performance indexes
CREATE INDEX CONCURRENTLY src_document_created_at_idx ON src_document (created_at);
CREATE INDEX CONCURRENTLY match_result_run_id_score_idx ON match_result (run_id, score DESC);
CREATE INDEX CONCURRENTLY dim_address_usrn_idx ON dim_address (usrn) WHERE usrn IS NOT NULL;

-- Analyze statistics
ANALYZE dim_address;
ANALYZE src_document;
ANALYZE match_result;

-- Query optimization
SET work_mem = '256MB';
SET maintenance_work_mem = '1GB';
SET effective_cache_size = '4GB';
```

#### 5.2.2 Application Performance
```go
// Connection pooling
db, err := sql.Open("postgres", dsn)
db.SetMaxOpenConns(20)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(time.Hour)

// Batch processing
func ProcessInBatches(records []SourceRecord, batchSize int) {
    for i := 0; i < len(records); i += batchSize {
        end := i + batchSize
        if end > len(records) {
            end = len(records)
        }
        processBatch(records[i:end])
    }
}

// Result caching for repeated queries
type CacheKey struct {
    AddressCanonical string
    Method          string
}

var matchCache = make(map[CacheKey][]Candidate)
```

### 5.3 Monitoring & Alerting

#### 5.3.1 Match Quality Metrics
```go
type QualityMetrics struct {
    Timestamp           time.Time `json:"timestamp"`
    TotalProcessed      int       `json:"total_processed"`
    AutoAcceptedCount   int       `json:"auto_accepted_count"`
    AutoAcceptedRate    float64   `json:"auto_accepted_rate"`
    ReviewQueueSize     int       `json:"review_queue_size"`
    AverageScore        float64   `json:"average_score"`
    AverageConfidence   float64   `json:"average_confidence"`
    ProcessingTimeMs    int64     `json:"processing_time_ms"`
    
    // Quality indicators
    LowConfidenceCount  int     `json:"low_confidence_count"`
    MultiCandidateCount int     `json:"multi_candidate_count"`
    NoMatchCount        int     `json:"no_match_count"`
    
    // Performance metrics  
    DbQueryTimeMs       int64   `json:"db_query_time_ms"`
    VectorQueryTimeMs   int64   `json:"vector_query_time_ms"`
    ScoringTimeMs       int64   `json:"scoring_time_ms"`
}
```

#### 5.3.2 Alert Thresholds
- Auto-accept rate drops below 60%
- Average processing time exceeds 800ms
- Review queue exceeds 1000 items  
- Database connection failures
- Vector service unavailable

---

## Phase 6: Human Review Interface (Week 6)

### 6.1 Review Queue Export
```csv
src_id,source_type,external_ref,raw_address,postcode_text,
candidate_1_uprn,candidate_1_address,candidate_1_score,candidate_1_distance,
candidate_2_uprn,candidate_2_address,candidate_2_score,candidate_2_distance,
candidate_3_uprn,candidate_3_address,candidate_3_score,candidate_3_distance,
recommendation,confidence,match_features
```

### 6.2 Decision Import Format
```csv
src_id,selected_uprn,action,reviewer_notes
123456,1710020110,accept,"House number matches exactly"
123457,,reject,"Address too vague - LAND AT SOMEWHERE"
123458,1710030787,accept_with_note,"Similar property in same street"
```

### 6.3 Review Interface (Optional Web UI)
```go
// Simple Gin web interface for reviewing matches
type ReviewItem struct {
    SrcID       int64       `json:"src_id"`
    RawAddress  string      `json:"raw_address"`
    Candidates  []Candidate `json:"candidates"`
    Features    Features    `json:"features"`
    Confidence  float64     `json:"confidence"`
}

func ReviewHandler(c *gin.Context) {
    items := getReviewQueue(50) // Next 50 items
    c.JSON(200, items)
}

func AcceptHandler(c *gin.Context) {
    var decision ReviewDecision
    c.BindJSON(&decision)
    recordDecision(decision)
    c.JSON(200, gin.H{"status": "accepted"})
}
```

---

## Phase 7: Deployment & Production (Week 7)

### 7.1 Production Configuration
```bash
# Environment variables
export DATABASE_URL="postgres://user:password@localhost:15432/ehdc_gis"
export QDRANT_URL="http://localhost:6333"  
export OLLAMA_URL="http://localhost:11434"
export LOG_LEVEL="info"
export BATCH_SIZE="1000"
export MAX_CANDIDATES="10"
export REVIEW_THRESHOLD="0.80"
export AUTO_ACCEPT_THRESHOLD="0.92"
```

### 7.2 Production Deployment
```bash
# Build production binary
go build -ldflags="-w -s" -o bin/matcher cmd/matcher/main.go

# Database migrations
./matcher migrate up

# Initial data import
./matcher ingest --source decision --file source_docs/decision_notices.csv
./matcher ingest --source land_charge --file source_docs/land_charges_cards.csv
./matcher ingest --source enforcement --file source_docs/enforcement_notices.csv  
./matcher ingest --source agreement --file source_docs/agreements.csv

# Build vector index
./matcher index-vectors --model bge-small-en

# Run matching pipeline
./matcher run --label production-v1.0 --all-stages

# Generate reports
./matcher report summary --output reports/
./matcher export csv --output results/
./matcher export review-queue --output review_queue.csv
```

### 7.3 Backup & Recovery
```bash
# Database backup
pg_dump -h localhost -p 15432 -U user ehdc_gis > backup_$(date +%Y%m%d).sql

# Vector index backup  
tar czf qdrant_backup_$(date +%Y%m%d).tar.gz /path/to/qdrant/storage

# Configuration backup
tar czf config_backup_$(date +%Y%m%d).tar.gz configs/ scripts/ docker-compose.yml
```

---

## Expected Outcomes

### Coverage Improvements
- **Decision Notices**: 18.95% → 75%+ (from 14,436 to ~57,000 records)
- **Land Charges**: 42.29% → 85%+ (from 21,045 to ~42,000 records)  
- **Enforcement Notices**: 12.71% → 70%+ (from 149 to ~820 records)
- **Agreements**: 24.71% → 80%+ (from 643 to ~2,080 records)

### Quality Targets
- **Auto-accept precision**: ≥98% (spot-checked)
- **Overall accuracy**: ≥95% (validated against gold standard)
- **Processing performance**: <600ms per query
- **System uptime**: >99.5%

### Deliverables
1. **Production database** with complete schema and indexes
2. **Go application** with CLI and optional web interface  
3. **Docker Compose stack** for local development and production
4. **Comprehensive documentation** including API docs and troubleshooting
5. **Quality reports** showing coverage and accuracy metrics
6. **Review queue** with prioritized manual decisions needed
7. **Export files** with enhanced UPRN/coordinate data for all source systems

---

## Risk Mitigation

### Technical Risks
- **Vector service failures**: Graceful degradation to fuzzy matching only
- **Database performance**: Comprehensive indexing and query optimization
- **Memory usage**: Batch processing and connection pooling
- **Data quality**: Extensive validation and audit trails

### Operational Risks  
- **Review queue backlogs**: Prioritization by confidence and impact
- **False positives**: Conservative auto-accept thresholds  
- **Data loss**: Regular backups and transaction rollback capabilities
- **Performance degradation**: Monitoring and alerting systems

This implementation plan provides a robust, auditable, and scalable solution for enhancing the EHDC LLPG system with sophisticated address matching capabilities while maintaining high quality standards and operational reliability.