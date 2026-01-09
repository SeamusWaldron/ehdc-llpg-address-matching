# Chapter 13: Data Quality Improvement Analysis

## 13.1 Executive Summary

This chapter analyses the current address cleansing methods in the EHDC LLPG system, identifies gaps and limitations, and proposes advanced techniques to improve data quality. A key finding is the opportunity to leverage the IDOX Planning Data Warehouse as both training data and cross-validation reference.

**Current State:**
- 18 distinct cleansing methods implemented across normalisation, validation, and matching
- 57.22% overall match rate achieved
- Key gaps: libpostal not integrated, LLM corrections disabled, limited spelling correction

**Recommended Enhancements:**
- SymSpell + phonetic hybrid pipeline for spelling correction
- Fine-tuned sentence-transformer embeddings for semantic matching
- Active learning from manual review feedback
- Cross-validation with IDOX planning data (626 applications with 32% UPRN coverage)

## 13.2 Current Cleansing Methods Inventory

### 13.2.1 Normalisation Layer

| Method | Location | Description | Effectiveness |
|--------|----------|-------------|---------------|
| CanonicalAddress() | normalize/address.go | Uppercase, trim, punctuation removal | High |
| AbbrevRules.Expand() | normalize/address.go | 30 abbreviation expansions | Medium |
| expandAbbreviations() | normalize/enhanced.go | 70+ abbreviations including Hampshire-specific | High |
| cleanPunctuation() | normalize/enhanced.go | Remove quotes, replace hyphens/ampersands | High |
| extractPostcode() | normalize/address.go | UK postcode regex extraction | High |

### 13.2.2 Token Extraction Layer

| Method | Location | Description | Effectiveness |
|--------|----------|-------------|---------------|
| ExtractHouseNumbers() | normalize/address.go | Numeric + alpha suffix extraction | High |
| ExtractLocalityTokens() | normalize/address.go | 37 Hampshire locality tokens | Medium |
| TokenizeStreet() | normalize/address.go | Street name with type suffix | Medium |
| handleDescriptors() | normalize/address.go | LAND AT, REAR OF preservation | Medium |

### 13.2.3 Phonetic Layer

| Method | Location | Description | Effectiveness |
|--------|----------|-------------|---------------|
| DoubleMetaphone.Encode() | phonetics/metaphone.go | Phonetic code generation | Medium |
| PhoneticTokenOverlap() | normalize/phonetics.go | Match counting between addresses | Medium |

### 13.2.4 Validation Layer

| Method | Location | Description | Effectiveness |
|--------|----------|-------------|---------------|
| ValidateHouseNumbers() | validation/validator.go | Exact/close match detection | High |
| ValidateStreetNames() | validation/validator.go | Levenshtein-based similarity | Medium |
| ValidatePostcode() | validation/parser.go | UK format validation | High |
| ValidateAddressForMatching() | validation/parser.go | Suitability assessment | Medium |

### 13.2.5 Correction Layer

| Method | Location | Description | Effectiveness |
|--------|----------|-------------|---------------|
| cleanSourceAddressData() | cmd/matcher-v2/main.go | Hard-coded error corrections | Low (limited scope) |
| applyGroupConsensusCorrections() | cmd/matcher-v2/main.go | Vote-based UPRN propagation | High |
| llmFixLowConfidenceAddresses() | cmd/matcher-v2/main.go | **DISABLED** - quality issues | N/A |

## 13.3 Identified Gaps and Limitations

### 13.3.1 Critical Gaps

1. **libpostal Not Integrated**
   - Parser.go line 90: "TODO: integrate libpostal for better parsing"
   - Currently uses regex fallback only
   - Impact: Poor handling of complex address structures

2. **LLM Corrections Disabled**
   - Explicitly disabled due to quality degradation
   - Issues observed: "AVENUE to AVE" (wrong direction), "BUNTINGS to BUNtings"
   - Root cause: Generic model not trained on UK address patterns

3. **No Spelling Correction Pipeline**
   - Phonetic matching catches sound-alikes but not typos
   - "PFTERSFTELD" requires hard-coded correction
   - No dictionary-based suggestion system

4. **Limited Historical Address Resolution**
   - No temporal matching (address valid at document date)
   - Street name changes not tracked
   - Property demolitions/mergers not handled

### 13.3.2 Moderate Gaps

5. **No Unicode Normalisation**
   - Accented characters not handled (cafe vs cafe)
   - No NFD/NFC normalisation

6. **Range Address Not Expanded**
   - "5-7 High Street" not split into discrete properties
   - Affects ~2% of addresses

7. **Business Name Standardisation Limited**
   - Only 30 entries in enhanced.go
   - Missing: pub names, church names, school names

8. **No Grammar-Aware Parsing**
   - Cannot distinguish "The Green" (place) from "Green Lane" (street)
   - Relies on suffix patterns only

### 13.3.3 Minor Gaps

9. **Coordinate Validation Missing**
   - No bounds checking for Hampshire (Easting 440000-490000, Northing 90000-150000)
   - Invalid coordinates could pollute spatial matching

10. **No Confidence Decay**
    - Old documents scored same as recent ones
    - Address changes more likely in older records

## 13.4 IDOX Planning Data Opportunity

### 13.4.1 Data Available

The IDOX Planning Data Warehouse contains 626 planning applications with:

| Field | Completeness | Quality |
|-------|--------------|---------|
| site_address | 95% | Free-text, multi-line |
| site_postcode | 96% | UK format, validated |
| site_uprn | 32% | 11-digit, verified |
| site_easting | 32% | BNG coordinates |
| site_northing | 32% | BNG coordinates |
| site_latitude | 32% | WGS84 coordinates |
| site_longitude | 32% | WGS84 coordinates |

### 13.4.2 Strategic Value

**Training Data:**
- ~200 applications with verified UPRNs = positive training examples
- Known-good matches from authoritative IDOX system
- Diverse address formats from planning submissions

**Cross-Validation:**
- Where IDOX has UPRN, validate EHDC matching accuracy
- Identify systematic errors in matching algorithm
- Ground truth for precision measurement

**Enrichment Targets:**
- 426 applications (68%) missing UPRN = enrichment opportunity
- Run EHDC matcher to backfill UPRNs
- Mutual benefit: both systems improved

### 13.4.3 Integration Architecture

```
IDOX Planning DB                    EHDC LLPG Matcher
      |                                    |
      v                                    v
gold.dim_location  -----> Address ---> Candidate
(site_address)            Matcher      Generation
      |                      |              |
      |                      v              v
      |                   Scoring <---- LLPG Lookup
      |                      |
      v                      v
Enriched Record <----- Match Result
(UPRN, coords)         (confidence)
```

## 13.5 Proposed Enhancements

### 13.5.1 Spelling Correction Pipeline

**SymSpell + Phonetic Hybrid:**

```go
type SpellingCorrector struct {
    symSpell    *symspell.SymSpell
    phonetics   *metaphone.DoubleMetaphone
    llpgTokens  map[string]bool
}

func (sc *SpellingCorrector) Correct(token string) (string, float64) {
    // Stage 1: Fast SymSpell lookup (edit distance 1-2)
    suggestions := sc.symSpell.Lookup(token, symspell.Top, 2)
    if len(suggestions) > 0 && suggestions[0].Distance <= 1 {
        return suggestions[0].Term, 0.95
    }

    // Stage 2: Phonetic fallback for larger errors
    tokenPhonetic := sc.phonetics.Encode(token)
    for llpgToken := range sc.llpgTokens {
        if sc.phonetics.Encode(llpgToken) == tokenPhonetic {
            // Verify with Jaro-Winkler
            if jaroWinkler(token, llpgToken) > 0.85 {
                return llpgToken, 0.85
            }
        }
    }

    return token, 0.0 // No correction found
}
```

**Dictionary Building:**
```sql
-- Extract unique tokens from LLPG for spelling dictionary
SELECT DISTINCT unnest(string_to_array(addr_can, ' ')) as token
FROM dim_address
WHERE LENGTH(token) >= 3
  AND token !~ '^\d+$'  -- Exclude pure numbers
ORDER BY token;
```

**Expected Impact:** +15-25% improvement in fuzzy match accuracy for misspelled addresses.

### 13.5.2 Fine-Tuned Address Embeddings

**Training Data Generation:**

```go
type TrainingPair struct {
    Address1   string
    Address2   string
    IsMatch    bool
    Confidence float64
}

func GenerateSyntheticPairs(llpgAddresses []string) []TrainingPair {
    var pairs []TrainingPair

    for _, addr := range llpgAddresses {
        // Positive pair: original + variant
        pairs = append(pairs, TrainingPair{
            Address1:   addr,
            Address2:   generateVariant(addr),
            IsMatch:    true,
            Confidence: 1.0,
        })

        // Hard negative: similar but different property
        similar := findSimilarDifferentProperty(addr)
        pairs = append(pairs, TrainingPair{
            Address1:   addr,
            Address2:   similar,
            IsMatch:    false,
            Confidence: 0.0,
        })
    }

    return pairs
}

func generateVariant(addr string) string {
    // Randomly apply transformations:
    // - Abbreviate: STREET -> ST
    // - Remove postcode
    // - Swap components
    // - Introduce typos
    // - Change case
}
```

**Model Architecture:**
- Base: sentence-transformers/all-MiniLM-L6-v2 (384 dimensions)
- Fine-tune on 10,000+ UK address pairs
- Contrastive loss with hard negative mining
- Output: 384-dimensional embedding per address

**Deployment:**
```go
type EmbeddingMatcher struct {
    model       *onnx.Session
    llpgIndex   *faiss.IndexFlatIP  // Pre-computed LLPG embeddings
    dimension   int
}

func (em *EmbeddingMatcher) FindCandidates(addr string, topK int) []Candidate {
    // Generate embedding for query address
    embedding := em.model.Encode(addr)

    // FAISS approximate nearest neighbor search
    distances, indices := em.llpgIndex.Search(embedding, topK)

    // Return candidates with cosine similarity scores
    var candidates []Candidate
    for i, idx := range indices {
        candidates = append(candidates, Candidate{
            UPRN:             em.llpgUPRNs[idx],
            SemanticScore:    distances[i],
        })
    }
    return candidates
}
```

**Expected Impact:** +10-15% improvement in matching addresses with word order variations and missing components.

### 13.5.3 Active Learning Pipeline

**Review Queue Design:**

```go
type ReviewQueue struct {
    db *sql.DB
}

type ReviewCandidate struct {
    SourceID        int64
    SourceAddress   string
    CandidateUPRN   string
    CandidateAddress string
    AutoScore       float64
    Features        map[string]interface{}
}

func (rq *ReviewQueue) GetNextBatch(batchSize int) []ReviewCandidate {
    // Priority: uncertain matches (0.40-0.70 score range)
    rows, _ := rq.db.Query(`
        SELECT src_id, src_address, candidate_uprn, candidate_address,
               score, features
        FROM match_result
        WHERE decision = 'needs_review'
          AND manual_reviewed = false
          AND score BETWEEN 0.40 AND 0.70
        ORDER BY ABS(score - 0.55)  -- Most uncertain first
        LIMIT $1
    `, batchSize)

    // ... scan rows into ReviewCandidate slice
}

func (rq *ReviewQueue) RecordDecision(srcID int64, uprn string, isMatch bool, reviewer string) {
    rq.db.Exec(`
        INSERT INTO match_manual_review
            (src_id, candidate_uprn, is_match, reviewer, reviewed_at)
        VALUES ($1, $2, $3, $4, NOW())
    `, srcID, uprn, isMatch, reviewer)

    // Update match_result with human decision
    if isMatch {
        rq.db.Exec(`
            UPDATE match_result
            SET decision = 'manual_accepted',
                manual_reviewed = true,
                confidence = 1.0
            WHERE src_id = $1 AND candidate_uprn = $2
        `, srcID, uprn)
    }
}
```

**Feedback Loop:**

```
Week 1: Review 500 uncertain matches
        |
        v
Week 2: Analyse patterns in manual decisions
        - Common correction types
        - Systematic algorithm failures
        |
        v
Week 3: Update rules/thresholds based on patterns
        |
        v
Week 4: Retrain ML model with new labeled data
        |
        v
Week 5: A/B test updated model vs baseline
        |
        v
Repeat cycle
```

**Expected Impact:** Continuous improvement of 2-5% per review cycle, with diminishing returns over time.

### 13.5.4 libpostal Integration

**HTTP Service Wrapper:**

```go
type LibpostalClient struct {
    baseURL string
    client  *http.Client
}

type ParsedAddress struct {
    HouseNumber  string `json:"house_number"`
    Road         string `json:"road"`
    Suburb       string `json:"suburb"`
    City         string `json:"city"`
    StateDistrict string `json:"state_district"`
    Postcode     string `json:"postcode"`
    Country      string `json:"country"`
}

func (lp *LibpostalClient) Parse(address string) (*ParsedAddress, error) {
    resp, err := lp.client.Post(
        lp.baseURL+"/parse",
        "application/json",
        bytes.NewBuffer([]byte(`{"address": "`+address+`"}`)),
    )
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result ParsedAddress
    json.NewDecoder(resp.Body).Decode(&result)
    return &result, nil
}

func (lp *LibpostalClient) Expand(address string) ([]string, error) {
    resp, err := lp.client.Post(
        lp.baseURL+"/expand",
        "application/json",
        bytes.NewBuffer([]byte(`{"address": "`+address+`"}`)),
    )
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result struct {
        Expansions []string `json:"expansions"`
    }
    json.NewDecoder(resp.Body).Decode(&result)
    return result.Expansions, nil
}
```

**Component-Based Matching:**

```go
func (fm *FuzzyMatcher) MatchWithLibpostal(srcAddr string, candidates []*Candidate) {
    srcParsed, _ := fm.libpostal.Parse(srcAddr)

    for _, cand := range candidates {
        candParsed, _ := fm.libpostal.Parse(cand.LocAddress)

        // Component-level matching
        cand.HouseNumberMatch = matchHouseNumbers(srcParsed.HouseNumber, candParsed.HouseNumber)
        cand.RoadMatch = jaroWinkler(srcParsed.Road, candParsed.Road)
        cand.CityMatch = jaroWinkler(srcParsed.City, candParsed.City)
        cand.PostcodeMatch = matchPostcodes(srcParsed.Postcode, candParsed.Postcode)

        // Weighted component score
        cand.LibpostalScore =
            cand.HouseNumberMatch * 0.30 +
            cand.RoadMatch * 0.35 +
            cand.CityMatch * 0.20 +
            cand.PostcodeMatch * 0.15
    }
}
```

**Expected Impact:** +20-30% improvement for complex addresses with building names, sub-buildings, and non-standard formats.

### 13.5.5 Cross-Validation with IDOX Data

**Validation Query:**

```sql
-- Find IDOX applications where our matching disagrees with IDOX UPRN
WITH idox_matches AS (
    SELECT
        ia.site_address,
        ia.site_uprn AS idox_uprn,
        mr.candidate_uprn AS ehdc_uprn,
        mr.score AS ehdc_score,
        mr.decision AS ehdc_decision
    FROM idox.gold.fact_application ia
    JOIN ehdc.match_result mr ON normalise(ia.site_address) = mr.src_addr_can
    WHERE ia.site_uprn IS NOT NULL
)
SELECT
    site_address,
    idox_uprn,
    ehdc_uprn,
    ehdc_score,
    CASE
        WHEN idox_uprn = ehdc_uprn THEN 'AGREE'
        WHEN ehdc_uprn IS NULL THEN 'EHDC_MISSED'
        ELSE 'DISAGREE'
    END AS validation_status
FROM idox_matches
ORDER BY validation_status, ehdc_score DESC;
```

**Precision Measurement:**

```go
func CalculatePrecisionFromIDOX(db *sql.DB) PrecisionMetrics {
    rows, _ := db.Query(`
        SELECT
            COUNT(*) FILTER (WHERE idox_uprn = ehdc_uprn) AS true_positives,
            COUNT(*) FILTER (WHERE idox_uprn != ehdc_uprn AND ehdc_uprn IS NOT NULL) AS false_positives,
            COUNT(*) FILTER (WHERE ehdc_uprn IS NULL AND idox_uprn IS NOT NULL) AS false_negatives
        FROM idox_validation_view
        WHERE ehdc_decision = 'auto_accepted'
    `)

    var tp, fp, fn int
    rows.Scan(&tp, &fp, &fn)

    return PrecisionMetrics{
        TruePositives:  tp,
        FalsePositives: fp,
        FalseNegatives: fn,
        Precision:      float64(tp) / float64(tp+fp),
        Recall:         float64(tp) / float64(tp+fn),
    }
}
```

**Expected Impact:** Provides ground truth for algorithm tuning, expected to identify 5-10% of false positives currently auto-accepted.

## 13.6 Implementation Priority Matrix

| Enhancement | Effort | Impact | Priority |
|-------------|--------|--------|----------|
| SymSpell spelling correction | Medium | High | **P1** |
| IDOX cross-validation | Low | High | **P1** |
| libpostal integration | Medium | High | **P2** |
| Active learning pipeline | High | Medium | **P2** |
| Fine-tuned embeddings | High | Medium | **P3** |
| Unicode normalisation | Low | Low | **P3** |
| Range address expansion | Medium | Low | **P4** |
| Coordinate bounds validation | Low | Low | **P4** |

## 13.7 Expected Outcomes

### 13.7.1 Match Rate Improvement

| Metric | Current | With Enhancements | Improvement |
|--------|---------|-------------------|-------------|
| Overall match rate | 57.22% | 68-72% | +11-15 pp |
| Auto-accept precision | 99.1% | 99.3-99.5% | +0.2-0.4 pp |
| Review queue size | 14.3% | 8-10% | -4-6 pp |
| Rejection rate | 28.5% | 20-22% | -6-8 pp |

### 13.7.2 Processing Efficiency

| Metric | Current | With Enhancements | Improvement |
|--------|---------|-------------------|-------------|
| Spelling correction | Manual only | Automated | N/A |
| FAISS candidate retrieval | N/A | <100ms | New capability |
| Review throughput | N/A | 50-100/hour | New capability |

## 13.8 Chapter Summary

This analysis has identified:

1. **18 current cleansing methods** across normalisation, validation, and correction layers
2. **10 gaps and limitations** including disabled LLM corrections and missing libpostal integration
3. **IDOX planning data opportunity** with 626 applications for training and validation
4. **5 major enhancements** prioritised by effort/impact:
   - SymSpell spelling correction (P1)
   - IDOX cross-validation (P1)
   - libpostal integration (P2)
   - Active learning pipeline (P2)
   - Fine-tuned embeddings (P3)

Expected overall improvement: **+11-15 percentage points** in match rate whilst maintaining >99% auto-accept precision.

---

*This chapter presents data quality improvement analysis. It extends the core thesis with forward-looking recommendations.*
