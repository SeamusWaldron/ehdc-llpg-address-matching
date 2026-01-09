# EHDC LLPG Address Matching System

## Technical Thesis

### Table of Contents

---

## Part I: Introduction and Overview

### [Chapter 1: Executive Summary](01_executive_summary.md)
- 1.1 Project Overview
- 1.2 Key Achievements
- 1.3 Technology Stack
- 1.4 Document Structure

### [Chapter 2: Introduction and Background](02_introduction_and_background.md)
- 2.1 The Address Matching Challenge
- 2.2 East Hampshire District Council Context
- 2.3 The UPRN Standard
- 2.4 Source Document Analysis
- 2.5 Project Objectives
- 2.6 Success Criteria

---

## Part II: System Design

### [Chapter 3: System Architecture](03_system_architecture.md)
- 3.1 Architectural Overview
- 3.2 Technology Stack
- 3.3 Docker Service Architecture
- 3.4 Go Application Structure
- 3.5 Data Flow Architecture
- 3.6 Integration Points
- 3.7 Scalability Considerations

### [Chapter 4: Database Schema Design](04_database_schema_design.md)
- 4.1 Schema Philosophy
- 4.2 Staging Layer
- 4.3 Dimension Tables
- 4.4 Fact Tables
- 4.5 Supporting Tables
- 4.6 Index Strategy
- 4.7 Spatial Data Handling
- 4.8 Data Type Decisions

---

## Part III: Core Algorithms

### [Chapter 5: Address Normalisation](05_address_normalisation.md)
- 5.1 The Normalisation Challenge
- 5.2 Canonical Form Specification
- 5.3 Core Normalisation Function
- 5.4 Postcode Extraction
- 5.5 Abbreviation Expansion
- 5.6 Descriptor Handling
- 5.7 House Number Extraction
- 5.8 Locality Token Recognition
- 5.9 Street Token Extraction
- 5.10 Token Overlap Calculation
- 5.11 libpostal Integration
- 5.12 Database Normalisation
- 5.13 Normalisation Quality Metrics

### [Chapter 6: Matching Algorithms](06_matching_algorithms.md)
- 6.1 Multi-Layer Matching Philosophy
- 6.2 Layer 2: Deterministic Matching
- 6.3 Layer 3: Fuzzy Group Matching
- 6.4 Layer 4: Individual Document Matching
- 6.5 Layer 5: Conservative Validation
- 6.6 Phonetic Matching
- 6.7 Semantic Vector Matching
- 6.8 Spatial Matching
- 6.9 Candidate Generation
- 6.10 Algorithm Selection

### [Chapter 7: Scoring and Decision Logic](07_scoring_and_decision_logic.md)
- 7.1 Scoring Overview
- 7.2 Feature Extraction
- 7.3 Feature Computation
- 7.4 Feature Weights
- 7.5 Score Calculation
- 7.6 Decision Tiers
- 7.7 Decision Logic
- 7.8 Explainability
- 7.9 Penalty System
- 7.10 Score Calibration
- 7.11 Alternative Scoring Models

---

## Part IV: Implementation

### [Chapter 8: Data Pipeline and ETL](08_data_pipeline_and_etl.md)
- 8.1 Pipeline Overview
- 8.2 LLPG Data Loading
- 8.3 OS Open UPRN Loading
- 8.4 Source Document Loading
- 8.5 Address Standardisation
- 8.6 Group Consensus Corrections
- 8.7 LLM Address Correction
- 8.8 Fact Table Rebuilding
- 8.9 Data Validation
- 8.10 Pipeline Orchestration

### [Chapter 9: Web Interface](09_web_interface.md)
- 9.1 Interface Overview
- 9.2 Server Architecture
- 9.3 API Endpoints
- 9.4 Search Functionality
- 9.5 Record Management
- 9.6 Export Functionality
- 9.7 Dashboard Statistics
- 9.8 Frontend Implementation
- 9.9 Error Handling
- 9.10 Performance Optimisation

### [Chapter 10: Configuration and Deployment](10_configuration_and_deployment.md)
- 10.1 Deployment Overview
- 10.2 Docker Compose Architecture
- 10.3 Environment Configuration
- 10.4 Go Application Configuration
- 10.5 PostgreSQL Tuning
- 10.6 Volume Management
- 10.7 Network Configuration
- 10.8 Deployment Procedures
- 10.9 Migration Management
- 10.10 Monitoring and Health Checks
- 10.11 Backup and Recovery
- 10.12 Security Considerations

---

## Part V: Results and Reference

### [Chapter 11: Results and Statistics](11_results_and_statistics.md)
- 11.1 Data Volume Summary
- 11.2 Matching Pipeline Results
- 11.3 Matching Accuracy
- 11.4 Performance Metrics
- 11.5 Coverage by Document Type
- 11.6 Quality Metrics
- 11.7 Temporal Analysis
- 11.8 Algorithm Comparison
- 11.9 Unmatched Analysis
- 11.10 Summary Statistics

### [Chapter 12: Appendices](12_appendices.md)
- Appendix A: Complete Database Schema DDL
- Appendix B: Configuration Reference
- Appendix C: CLI Command Reference
- Appendix D: Glossary of Terms
- Appendix E: File Structure Reference
- Appendix F: Abbreviation Expansion Rules
- Appendix G: Hampshire Locality Tokens
- Appendix H: Regular Expression Patterns
- Appendix I: Spelling Correction Techniques for UK Addresses
- Appendix J: UK Address Standards Reference
- Appendix K: External Data Sources for Address Enrichment
- Appendix L: Active Learning for Address Matching
- Appendix M: Multi-Source Address Resolution Best Practices

---

## Part VI: Future Development

### [Chapter 13: Data Quality Improvement Analysis](13_data_quality_improvement_analysis.md)
- 13.1 Executive Summary
- 13.2 Current Cleansing Methods Inventory
- 13.3 Identified Gaps and Limitations
- 13.4 IDOX Planning Data Opportunity
- 13.5 Proposed Enhancements
- 13.6 Implementation Priority Matrix
- 13.7 Expected Outcomes
- 13.8 Chapter Summary

---

## Quick Reference

| Topic | Chapter |
|-------|---------|
| Project overview | [Chapter 1](01_executive_summary.md) |
| Source data formats | [Chapter 2](02_introduction_and_background.md) |
| Docker services | [Chapter 3](03_system_architecture.md) |
| Database tables | [Chapter 4](04_database_schema_design.md) |
| Address parsing | [Chapter 5](05_address_normalisation.md) |
| Matching pipeline | [Chapter 6](06_matching_algorithms.md) |
| Confidence scoring | [Chapter 7](07_scoring_and_decision_logic.md) |
| Data loading | [Chapter 8](08_data_pipeline_and_etl.md) |
| REST API | [Chapter 9](09_web_interface.md) |
| Environment setup | [Chapter 10](10_configuration_and_deployment.md) |
| Match statistics | [Chapter 11](11_results_and_statistics.md) |
| CLI commands | [Appendix C](12_appendices.md#appendix-c-cli-command-reference) |
| Glossary | [Appendix D](12_appendices.md#appendix-d-glossary-of-terms) |
| Spelling correction | [Appendix I](12_appendices.md#appendix-i-spelling-correction-techniques-for-uk-addresses) |
| UK address standards | [Appendix J](12_appendices.md#appendix-j-uk-address-standards-reference) |
| External data sources | [Appendix K](12_appendices.md#appendix-k-external-data-sources-for-address-enrichment) |
| Active learning | [Appendix L](12_appendices.md#appendix-l-active-learning-for-address-matching) |
| Multi-source resolution | [Appendix M](12_appendices.md#appendix-m-multi-source-address-resolution-best-practices) |

---

## Document Information

| Attribute | Value |
|-----------|-------|
| System | EHDC LLPG Address Matching System |
| Version | 2.1 |
| Language | British English |
| Total Chapters | 13 |
| Total Appendices | 13 (A-M) |
| Total Source Records | 129,701 |
| Overall Match Rate | 57.22% |
| Auto-Accept Precision | 99.1% |

---

*EHDC LLPG Address Matching System - Technical Thesis*
