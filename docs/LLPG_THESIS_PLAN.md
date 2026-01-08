# EHDC LLPG Address Matching System - Thesis Plan

## Overview

This plan outlines the creation of a comprehensive technical thesis documenting the East Hampshire District Council Local Land and Property Gazetteer (LLPG) address matching system. The thesis will be stored in `docs/llpg_thesis/` with chapters numbered sequentially.

## Thesis Structure

### Part I: Introduction and Context

**Chapter 01 - Executive Summary**
- Project objectives and business drivers
- Key achievements and metrics
- Technology stack summary
- Document structure overview

**Chapter 02 - Introduction and Background**
- The address matching problem in local government
- UPRN (Unique Property Reference Number) significance
- Historic document challenges
- Project scope and constraints
- Data sources overview (LLPG, OS Open UPRN, four historic document types)

### Part II: Technical Architecture

**Chapter 03 - System Architecture**
- High-level architecture diagram
- Component interactions
- Docker services (PostgreSQL/PostGIS, Qdrant, Ollama, libpostal)
- Go application structure
- Package layout and responsibilities

**Chapter 04 - Database Schema Design**
- Staging tables (raw data import)
- Dimension tables (dim_address, dim_location, dim_document_type, dim_match_method)
- Source document table (src_document)
- Match result tables (match_result, match_accepted, match_override, match_run)
- Fact tables for reporting
- Index strategy (GIN for pg_trgm, GIST for PostGIS)
- PostGIS geometry handling (BNG EPSG:27700, WGS84 EPSG:4326)

### Part III: Core Algorithms

**Chapter 05 - Address Normalisation**
- UK address challenges and variations
- Canonical form specification
- Postcode extraction and handling
- Abbreviation expansion rules
- Descriptor handling (LAND AT, REAR OF, etc.)
- House number and flat number parsing
- Locality token recognition
- libpostal integration for component parsing

**Chapter 06 - Matching Algorithms**
- Multi-layer matching pipeline overview
- Layer 1: Setup and data loading
- Layer 2: Deterministic matching
  - Legacy UPRN validation
  - Exact canonical address matching
- Layer 3: Fuzzy matching
  - PostgreSQL pg_trgm trigram similarity
  - Phonetic matching (Double Metaphone)
  - Locality and house number filters
- Layer 4: Semantic vector matching
  - Embedding generation (Ollama/nomic-embed-text)
  - Qdrant HNSW approximate nearest neighbour search
  - Cosine similarity scoring
- Layer 5: Spatial proximity matching
  - PostGIS distance calculations
  - Spatial area caching
  - Distance-based boosting

**Chapter 07 - Scoring and Decision Logic**
- Feature extraction and computation
- Feature weight configuration
- Meta-score calculation formula
- Decision tiers (auto-accept, review, reject)
- Winner margin requirements
- Confidence thresholds
- Explainability and audit trail

### Part IV: Implementation

**Chapter 08 - Data Pipeline and ETL**
- CSV import process
- LLPG loading (71,904 records)
- OS Open UPRN loading (41+ million records, batch processing)
- Source document loading (four historic datasets)
- Address standardisation pipeline
- Group consensus corrections
- LLM-powered address correction

**Chapter 09 - Web Interface**
- Gin HTTP server
- REST API endpoints
- Record browsing and filtering
- Map visualisation
- Export functionality
- Real-time updates

**Chapter 10 - Configuration and Deployment**
- Docker Compose services
- Environment variables
- Database connection pooling
- Parallel processing configuration
- Performance tuning parameters

### Part V: Results and Evaluation

**Chapter 11 - Results and Statistics**
- Data volume summary
- Match rate by document type
- Confidence distribution
- Correction method breakdown
- Quality assurance measures
- Performance metrics

### Part VI: Appendices

**Chapter 12 - Appendices**
- A: Complete database schema DDL
- B: Configuration reference
- C: CLI command reference
- D: Glossary of terms
- E: File structure reference

## Document Conventions

- British English spelling and grammar throughout
- No em dashes (use hyphens or separate sentences)
- No emojis
- Technical accuracy prioritised
- Code examples where appropriate
- Diagrams in ASCII art format

## File Naming Convention

Each chapter file follows the pattern:
```
docs/llpg_thesis/01_executive_summary.md
docs/llpg_thesis/02_introduction_and_background.md
docs/llpg_thesis/03_system_architecture.md
...
```

## Estimated Content

- Total chapters: 12
- Estimated total length: 15,000-20,000 words
- Target audience: Technical staff, system administrators, future maintainers

## Execution Order

1. Create directory structure
2. Write chapters sequentially (01 through 12)
3. Review for consistency
4. Cross-reference between chapters

---

*Plan created: January 2026*
