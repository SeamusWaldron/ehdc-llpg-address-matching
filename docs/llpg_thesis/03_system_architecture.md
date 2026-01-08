# Chapter 3: System Architecture

## 3.1 Architectural Overview

The EHDC LLPG Address Matching System follows a local-first, modular architecture designed for high-throughput batch processing with support for individual address queries. The system comprises four Docker services and a Go application that orchestrates data loading, matching, and result management.

```
                                 +------------------+
                                 |   Go Application |
                                 |   (matcher-v2)   |
                                 +--------+---------+
                                          |
          +-------------------------------+-------------------------------+
          |                               |                               |
          v                               v                               v
+------------------+           +------------------+           +------------------+
|   PostgreSQL     |           |     Qdrant       |           |     Ollama       |
|   + PostGIS      |           |   Vector DB      |           |   Embeddings     |
|   + pg_trgm      |           |                  |           |                  |
+------------------+           +------------------+           +------------------+
          |
          v
+------------------+
|   libpostal      |
|   Parser         |
+------------------+
```

## 3.2 Docker Services

### 3.2.1 PostgreSQL with PostGIS

The primary data store uses PostgreSQL 15 with PostGIS 3.4 for spatial operations and pg_trgm for fuzzy string matching.

**Container Configuration**:
- Image: `postgis/postgis:15-3.4`
- External Port: 15435
- Database Name: `ehdc_llpg`
- Extensions: PostGIS, pg_trgm, unaccent

**Performance Tuning**:
```
max_connections=200
shared_buffers=256MB
effective_cache_size=1GB
work_mem=4MB
```

**Role**: Stores all address data, match results, and audit trails. Provides trigram similarity matching via pg_trgm and spatial distance calculations via PostGIS.

### 3.2.2 Qdrant Vector Database

Qdrant provides approximate nearest neighbour search for semantic address matching using vector embeddings.

**Container Configuration**:
- Image: `qdrant/qdrant:v1.7.4`
- HTTP Port: 6333
- gRPC Port: 6334
- Storage: Docker volume `qdrant_storage`

**Role**: Stores address embeddings and performs HNSW-based vector similarity search to find semantically similar addresses that may not match well using string metrics.

### 3.2.3 Ollama Embedding Service

Ollama provides local embedding generation without requiring external API calls.

**Container Configuration**:
- Image: `ollama/ollama:latest`
- Port: 11434
- Models: `nomic-embed-text`, `llama3.2:1b`
- Keep Alive: 24 hours

**Role**: Generates 384-dimensional embeddings for address strings. The `nomic-embed-text` model provides high-quality embeddings suitable for address similarity. The `llama3.2:1b` model supports LLM-powered address correction for formatting issues.

### 3.2.4 libpostal Parser

libpostal provides statistical address parsing and normalisation.

**Container Configuration**:
- Custom Dockerfile build
- Port: 8080
- HTTP API interface

**Role**: Parses raw addresses into structured components (house number, road, city, postcode). Supports the component-based matching strategies.

## 3.3 Go Application Structure

The matcher-v2 application is structured as a single binary with multiple commands:

```
cmd/
  matcher-v2/
    main.go                         # Entry point and command dispatcher
    parallel_layer2.go              # Deterministic matching
    parallel_layer3.go              # Fuzzy matching (parallel)
    enhanced_layer3.go              # Enhanced fuzzy with deduplication
    parallel_spatial_preprocessing.go # Spatial table building
    rebuild_fact_intelligent.go     # Fact table management
    rebuild_fact_simple.go          # Simple fact rebuild

internal/
  match/                            # Modern matching engine
    types.go                        # Core data structures
    engine.go                       # Match orchestration
    generator.go                    # Candidate generation
    scorer.go                       # Feature scoring
    features.go                     # Feature computation

  engine/                           # Legacy matching algorithms
    deterministic.go                # UPRN validation
    fuzzy.go                        # Trigram matching
    fuzzy_optimized.go              # Parallel fuzzy
    vector_matcher.go               # Embedding matching
    spatial_matcher.go              # Distance matching
    matcher.go                      # Result recording

  normalize/                        # Address normalisation
    address.go                      # Canonical form
    enhanced.go                     # Advanced normalisation
    phonetics.go                    # Phonetic features

  phonetics/
    metaphone.go                    # Double Metaphone

  etl/                              # Data loading
    pipeline.go                     # CSV import
    osdata.go                       # OS UPRN loading

  web/                              # Web interface
    server.go                       # HTTP server
    handlers/                       # API endpoints

  config/
    env.go                          # Configuration

  db/
    connection.go                   # Database pooling

  validation/
    validator.go                    # UPRN validation
    parser.go                       # Address parsing

  audit/
    tracker.go                      # Audit trail

  debug/
    debug.go                        # Debug output
```

## 3.4 Data Flow Architecture

### 3.4.1 Ingestion Flow

```
CSV Files                     Staging Tables            Dimension Tables
+------------------+         +------------------+      +------------------+
| ehdc_llpg.csv    | ------> | stg_ehdc_llpg    | ---> | dim_address      |
| osopenuprn.csv   | ------> | stg_os_uprn      | ---> | dim_location     |
| decision_notices | ------> | stg_decision_*   | ---> | src_document     |
| land_charges     | ------> | stg_land_charges | ---> |                  |
| enforcement      | ------> | stg_enforcement  | ---> |                  |
| agreements       | ------> | stg_agreements   | ---> |                  |
+------------------+         +------------------+      +------------------+
```

### 3.4.2 Matching Flow

```
src_document                  Matching Pipeline           Match Results
+------------------+         +------------------+        +------------------+
| Raw Address      |         | 1. Normalise     |        | match_result     |
| Optional UPRN    | ------> | 2. Deterministic | -----> | match_accepted   |
| Optional E/N     |         | 3. Fuzzy         |        | match_override   |
|                  |         | 4. Vector        |        | match_run        |
|                  |         | 5. Spatial       |        |                  |
|                  |         | 6. Score/Decide  |        |                  |
+------------------+         +------------------+        +------------------+
```

## 3.5 Matching Pipeline Architecture

The matching pipeline implements a multi-layer approach where each layer addresses progressively more difficult cases:

### Layer 1: Setup (One-Time)
- Database schema creation
- Extension installation (PostGIS, pg_trgm, unaccent)
- Index building
- Data loading

### Layer 2: Deterministic Matching
- Legacy UPRN validation against LLPG
- Exact canonical address matching
- Expected yield: 5-15% of records
- Confidence: 1.0 (perfect matches)

### Layer 3: Fuzzy Matching
- PostgreSQL pg_trgm trigram similarity
- Phonetic filtering (Double Metaphone)
- Locality and house number overlap
- Expected yield: 40-60% of records
- Confidence: 0.70-0.95

### Layer 4: Semantic Matching
- Address embedding via Ollama
- Qdrant HNSW vector search
- Cosine similarity scoring
- Expected yield: 10-20% of records
- Confidence: 0.70-0.90

### Layer 5: Spatial Matching
- PostGIS distance calculations
- Spatial area caching
- Distance-based boosting
- Expected yield: 5-15% of records with coordinates
- Confidence: Boosts existing candidates

### Layer 6: Scoring and Decision
- Feature aggregation
- Weighted meta-score calculation
- Threshold-based decisions
- Audit trail recording

## 3.6 Parallel Processing Architecture

The system supports parallel processing at multiple levels:

### 3.6.1 Worker Pool Model

```go
func getOptimalWorkerCount() int {
    numWorkers := runtime.NumCPU()
    if numWorkers > 1 {
        numWorkers = numWorkers - 1  // Reserve one core for DB
    }
    if numWorkers > 16 {
        numWorkers = 16  // Upper limit for connections
    }
    if numWorkers < 2 {
        numWorkers = 2  // Minimum for parallelisation
    }
    return numWorkers
}
```

### 3.6.2 Parallel Fuzzy Matching

The enhanced Layer 3 implementation uses:

1. **Address Deduplication**: Groups documents by canonical address to avoid redundant matching
2. **Work Distribution**: Distributes unique addresses across worker goroutines
3. **Result Propagation**: Applies match results to all documents sharing an address
4. **Progress Reporting**: Real-time statistics during processing

### 3.6.3 Spatial Preprocessing

Spatial table building uses parallel processing:

1. **Road-Postcode Areas**: Groups LLPG addresses by road and postcode
2. **Road-Only Areas**: Fallback for addresses without postcodes
3. **Centroid Calculation**: Computes area centroids for distance calculations

## 3.7 Connection Management

### 3.7.1 Database Pooling

```go
db.SetMaxOpenConns(20)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(30 * time.Minute)
```

### 3.7.2 Connection String Format

```
host=localhost port=15435 user=postgres password=*** dbname=ehdc_llpg sslmode=disable
```

## 3.8 Error Handling Strategy

The system employs consistent error handling:

1. **Error Wrapping**: Uses `fmt.Errorf("context: %w", err)` for error chains
2. **Early Returns**: Reduces nesting with immediate error returns
3. **Debug Logging**: Header/Output/Footer pattern for tracing
4. **Graceful Degradation**: Continues processing when individual records fail

## 3.9 Configuration Architecture

Configuration follows a layered approach:

1. **Environment Variables**: Primary configuration source
2. **Configuration File**: `.env` file for local development
3. **Command Line Flags**: Override for specific runs
4. **Defaults**: Sensible defaults in code

Key configuration categories:

- **Database**: Host, port, credentials, pool settings
- **Vector DB**: Qdrant host, port, collection settings
- **Embeddings**: Ollama host, model selection
- **Matching**: Thresholds, batch sizes, worker counts
- **Web**: Server host, port, feature flags

## 3.10 Audit and Traceability

Every matching decision is recorded with:

- **Run Identifier**: Links all results from a single execution
- **Method**: Which algorithm produced the match
- **Score**: Confidence value
- **Features**: Complete feature breakdown
- **Decision**: auto_accept, needs_review, or reject
- **Timestamp**: When the decision was made
- **Actor**: System or user who made the decision

This enables:

- Re-running matching with improved algorithms
- Preserving manual review decisions
- Analysing algorithm performance
- Debugging specific matching failures

## 3.11 Chapter Summary

This chapter has described the system architecture:

- Four Docker services providing database, vector search, embeddings, and parsing
- Go application structure with clear package separation
- Multi-layer matching pipeline from deterministic to semantic
- Parallel processing for performance
- Configuration and error handling strategies
- Comprehensive audit trail

The following chapter examines the database schema design in detail.

---

*This chapter establishes the architectural foundation. Chapter 4 provides detailed database schema documentation.*
