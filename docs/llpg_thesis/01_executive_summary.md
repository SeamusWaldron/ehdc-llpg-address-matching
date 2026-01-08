# Chapter 1: Executive Summary

## 1.1 Project Overview

The East Hampshire District Council Local Land and Property Gazetteer (EHDC LLPG) Address Matching System is a sophisticated Go-based application designed to solve a critical challenge in local government data management: matching historic document addresses to modern Unique Property Reference Numbers (UPRNs).

East Hampshire District Council maintains four categories of historic documents - decision notices, land charges cards, enforcement notices, and agreements - totalling approximately 130,000 records. A significant proportion of these records lack the UPRN and coordinate data required for modern spatial analysis and cross-referencing. This system was developed to systematically backfill this missing information by matching historic addresses against an authoritative LLPG containing 71,904 properties.

## 1.2 Business Drivers

The project addresses several operational requirements:

1. **Regulatory Compliance**: Local authorities are required to maintain accurate property records linked to the national UPRN standard
2. **Spatial Analysis**: Planning and enforcement decisions require accurate geographic coordinates for mapping and analysis
3. **Data Integration**: Cross-referencing between historic and modern records necessitates a common identifier (UPRN)
4. **Operational Efficiency**: Manual address matching is prohibitively time-consuming for the volume of records involved

## 1.3 Key Achievements

The system has achieved the following results:

- **Total Documents Processed**: 130,540 records across nine document types
- **Match Rate**: 57.22% (74,696 documents successfully matched)
- **High Confidence Matches**: 54,202 auto-accepted matches (41.5% of total)
- **Corrections Applied**: 10,015 address corrections through various methods
- **Processing Performance**: Sub-second matching for individual addresses; batch processing at approximately 5,000 records per minute

### Match Quality Distribution

| Confidence Level | Document Count | Percentage |
|-----------------|----------------|------------|
| High (0.85 or above) | 54,202 | 41.5% |
| Medium (0.50-0.84) | 16,357 | 12.5% |
| Low (0.20-0.49) | 4,130 | 3.2% |
| No Match (below 0.20) | 55,844 | 42.8% |

### Correction Method Breakdown

| Method | Corrections Applied |
|--------|-------------------|
| Historic UPRN Creation | 5,119 |
| Exact UPRN Match | 3,450 |
| Road and City Validation | 939 |
| Fuzzy Road Matching | 305 |
| Postcode and House Number | 169 |
| LLM-Powered Correction | 32 |
| Business Name Matching | 1 |

## 1.4 Technology Stack

The system employs a modern, local-first architecture:

**Core Technologies**
- **Programming Language**: Go 1.18+
- **Database**: PostgreSQL 15+ with PostGIS 3.4 and pg_trgm extensions
- **Vector Database**: Qdrant v1.7.4 for semantic address embeddings
- **Embedding Generation**: Ollama with nomic-embed-text model
- **Address Parsing**: libpostal HTTP service

**Infrastructure**
- Docker Compose orchestration
- Connection pooling (20 max open, 10 idle connections)
- Parallel processing with automatic CPU detection

**Key Libraries**
- lib/pq for PostgreSQL connectivity
- Gorilla mux for HTTP routing
- Internal packages for normalisation, matching, and scoring

## 1.5 Matching Pipeline Summary

The system implements a multi-layer matching pipeline:

1. **Layer 1 - Setup**: Database schema creation, data loading, and index building
2. **Layer 2 - Deterministic**: Legacy UPRN validation and exact canonical address matching
3. **Layer 3 - Fuzzy**: PostgreSQL trigram similarity with phonetic and locality filters
4. **Layer 4 - Semantic**: Vector embedding search using Qdrant
5. **Layer 5 - Spatial**: Distance-based matching using PostGIS geometries

Each layer progressively addresses harder matching cases, with earlier layers handling straightforward matches and later layers employing more sophisticated techniques for ambiguous addresses.

## 1.6 Document Structure

This thesis is organised into twelve chapters:

- **Chapters 1-2**: Introduction, context, and background
- **Chapters 3-4**: System architecture and database design
- **Chapters 5-7**: Core algorithms (normalisation, matching, scoring)
- **Chapters 8-10**: Implementation details (ETL, web interface, deployment)
- **Chapter 11**: Results and performance analysis
- **Chapter 12**: Appendices and reference material

Each chapter provides detailed technical documentation suitable for system maintainers, developers, and technical stakeholders seeking to understand, modify, or extend the system.

## 1.7 Target Audience

This documentation is intended for:

- **System Administrators**: Responsible for deployment and maintenance
- **Software Developers**: Tasked with extending or modifying the system
- **Data Analysts**: Seeking to understand match quality and confidence metrics
- **Technical Managers**: Requiring architectural understanding for planning purposes

## 1.8 Conventions Used

Throughout this thesis:

- Code examples are provided in Go or SQL as appropriate
- File paths are given relative to the project root
- Configuration values show defaults where applicable
- British English spelling is used throughout

---

*This chapter provides a high-level overview of the EHDC LLPG Address Matching System. Subsequent chapters explore each component in detail.*
