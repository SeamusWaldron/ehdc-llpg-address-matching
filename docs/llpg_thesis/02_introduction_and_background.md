# Chapter 2: Introduction and Background

## 2.1 The Address Matching Problem

Address matching - the process of linking textual address descriptions to standardised property identifiers - represents one of the most challenging problems in geographic information systems. Unlike structured data fields such as dates or numerical values, addresses exhibit significant variability in format, abbreviation, spelling, and completeness.

In the United Kingdom, this challenge is compounded by:

- **Historical Naming**: Properties may have been known by different names over time
- **Renumbering**: Street numbering schemes change as areas develop
- **Boundary Changes**: Parish, district, and postal boundaries shift
- **Informal Descriptions**: Historic documents often contain colloquial or abbreviated addresses
- **Data Entry Variation**: Different clerks recorded addresses using different conventions

Local authorities face particular difficulties because they must reconcile decades of historic records with modern addressing standards, often without the benefit of consistent identifiers across time periods.

## 2.2 The Unique Property Reference Number (UPRN)

The Unique Property Reference Number (UPRN) is a 12-digit identifier assigned to every addressable location in Great Britain. Introduced as part of the National Land and Property Gazetteer (NLPG) initiative, UPRNs provide:

- **Permanence**: Once assigned, a UPRN remains constant even if the property address changes
- **Uniqueness**: Each addressable location receives exactly one UPRN
- **Interoperability**: UPRNs enable data sharing across public sector organisations
- **Spatial Linking**: UPRNs connect to authoritative coordinates via the AddressBase products

For local authorities, UPRNs are essential for:

1. Coordinating planning and enforcement activities
2. Linking historic records to modern property databases
3. Enabling spatial analysis and mapping
4. Supporting central government reporting requirements

The absence of UPRNs in historic records creates a significant data quality issue that this system addresses.

## 2.3 East Hampshire District Council Context

East Hampshire District Council (EHDC) is a local authority in Hampshire, England, covering an area of approximately 514 square kilometres. The district includes market towns such as Alton, Petersfield, and Bordon, alongside numerous villages and rural areas.

The Council maintains a Local Land and Property Gazetteer (LLPG) containing 71,904 addressable properties. This gazetteer serves as the authoritative source for property addresses within the district and includes:

- Full postal addresses
- British National Grid coordinates (Easting and Northing)
- Unique Street Reference Numbers (USRNs)
- Basic Land and Property Unit (BLPU) classifications
- Property status indicators

The LLPG is updated regularly as new properties are built, existing properties are modified, and addressing schemes change.

## 2.4 Historic Document Datasets

The system processes four primary categories of historic documents:

### 2.4.1 Decision Notices

Decision notices document the outcomes of planning applications. The dataset contains 76,167 records spanning several decades of planning history.

**Characteristics**:
- Planning application reference numbers
- Full addresses (often with inconsistent formatting)
- Decision dates and types
- Some records include legacy UPRNs and coordinates
- Approximately 90% missing UPRN data

### 2.4.2 Land Charges Cards

Land charges records document legal encumbrances on properties. The dataset contains 49,760 records.

**Characteristics**:
- Card code identifiers
- Property addresses
- Associated charge information
- Approximately 60% missing UPRN data
- Many records contain coordinates without corresponding UPRNs

### 2.4.3 Enforcement Notices

Enforcement notices document planning enforcement actions. The dataset contains 1,172 records.

**Characteristics**:
- Enforcement reference numbers
- Property addresses under investigation
- Date and type information
- Approximately 92% missing UPRN data

### 2.4.4 Agreements

Agreements document planning agreements, Section 106 agreements, and similar legal instruments. The dataset contains 2,602 records.

**Characteristics**:
- Legal agreement identifiers
- Property addresses
- Date information
- Approximately 78% missing UPRN data

### 2.4.5 Additional Document Types

The system also processes several supplementary datasets:

- **Microfiche Post-1974**: 108,164 records (planning references, no addresses)
- **Microfiche Pre-1974**: 43,977 records (historic planning references)
- **Street Name and Numbering**: 7,385 records (address changes)
- **Enlargement Maps**: 1,514 records (map references)
- **ENL Folders**: 17 records (development documentation)

## 2.5 Data Quality Challenges

Analysis of the source datasets revealed several data quality challenges:

### 2.5.1 Address Formatting Issues

- Inconsistent capitalisation (mixed case, all uppercase, all lowercase)
- Variable punctuation (commas, full stops, hyphens)
- Abbreviated street types (RD for Road, ST for Street)
- Missing or incomplete postcodes
- Embedded special characters

### 2.5.2 UPRN Data Issues

- Decimal suffixes on UPRNs (for example, 123456789.00)
- Invalid or expired UPRNs not present in current LLPG
- UPRNs from neighbouring authority areas
- Incorrectly transcribed UPRN values

### 2.5.3 Coordinate Data Issues

- Coordinates present without corresponding UPRNs
- Coordinates at incorrect precision
- Coordinates outside district boundaries
- Missing coordinate reference system information

### 2.5.4 Temporal Issues

- Properties renamed or renumbered since document creation
- Properties demolished and rebuilt with new identifiers
- Street names changed over time
- Postal district boundary changes affecting postcodes

## 2.6 Project Objectives

The primary objectives of this system are:

1. **UPRN Recovery**: Match historic addresses to modern UPRNs with high accuracy
2. **Coordinate Backfill**: Populate missing Easting and Northing values from LLPG
3. **Auditability**: Record all matching decisions with full explainability
4. **Repeatability**: Support re-running matching as algorithms improve
5. **Manual Review**: Surface ambiguous cases for human verification

### 2.6.1 Quality Targets

The system targets:

- **Auto-accept Precision**: 98% or greater accuracy on automated matches
- **Coverage**: Maximise percentage of records with valid UPRNs
- **Performance**: Sub-second matching for individual queries; batch processing at scale
- **Explainability**: Every decision traceable with feature breakdown

## 2.7 Constraints and Scope

### 2.7.1 In Scope

- Matching against EHDC LLPG only (not neighbouring authorities)
- Processing of four primary historic document types
- Coordinate validation using OS Open UPRN data
- Support for manual review workflow
- Export of matched data for downstream systems

### 2.7.2 Out of Scope

- Direct integration with operational planning systems
- Real-time matching of incoming applications
- Cross-authority address resolution
- Historic address research beyond LLPG records

### 2.7.3 Technical Constraints

- Local-first architecture (no external API dependencies for matching)
- Free and open-source tooling preferred
- PostgreSQL as the primary data store
- Docker-based deployment for portability

## 2.8 Related Work

Address matching is a well-studied problem in geographic information science. Common approaches include:

- **Rule-based Matching**: Pattern matching with handcrafted rules
- **Statistical Matching**: Probabilistic record linkage using string similarity metrics
- **Machine Learning**: Supervised classification of match candidates
- **Semantic Matching**: Vector embeddings for meaning-based similarity

This system employs a hybrid approach, combining deterministic rules with statistical similarity measures and semantic embeddings to achieve robust matching across diverse address formats.

## 2.9 Chapter Summary

This chapter has established the context for the EHDC LLPG Address Matching System:

- The fundamental challenge of address matching in local government
- The importance of UPRNs as property identifiers
- The specific datasets and quality issues at East Hampshire District Council
- The project objectives and constraints

The following chapters detail the technical architecture and implementation of the system designed to address these challenges.

---

*This chapter provides essential background for understanding the system requirements. Chapter 3 examines the technical architecture in detail.*
