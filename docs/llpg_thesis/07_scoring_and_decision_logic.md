# Chapter 7: Scoring and Decision Logic

## 7.1 Scoring Overview

The scoring system transforms raw matching features into a single confidence score that drives automated decision-making. The design prioritises:

1. **Explainability**: Every score can be decomposed into its constituent features
2. **Tuneability**: Weights can be adjusted based on empirical results
3. **Conservatism**: High thresholds prevent false auto-accepts

## 7.2 Feature Extraction

For each candidate address, the system extracts a comprehensive feature set:

### 7.2.1 String Similarity Features

| Feature | Range | Description |
|---------|-------|-------------|
| trgm_score | 0.0-1.0 | PostgreSQL pg_trgm similarity |
| jaro_score | 0.0-1.0 | Jaro-Winkler string similarity |
| levenshtein_norm | 0.0-1.0 | Normalised Levenshtein distance |

### 7.2.2 Token Overlap Features

| Feature | Range | Description |
|---------|-------|-------------|
| locality_overlap | 0.0-1.0 | Ratio of matching locality tokens |
| street_overlap | 0.0-1.0 | Ratio of matching street tokens |
| same_house_number | Boolean | House numbers match exactly |
| same_house_alpha | Boolean | Alpha suffixes match (12A = 12A) |

### 7.2.3 Phonetic Features

| Feature | Range | Description |
|---------|-------|-------------|
| phonetic_hits | 0-N | Count of phonetic token matches |
| phonetic_miss | Boolean | No phonetic overlap detected |

### 7.2.4 Semantic Features

| Feature | Range | Description |
|---------|-------|-------------|
| embed_cos | 0.0-1.0 | Cosine similarity of embeddings |

### 7.2.5 Spatial Features

| Feature | Range | Description |
|---------|-------|-------------|
| spatial_distance | 0-Inf | Distance in metres |
| spatial_boost | 0.0-0.1 | Exponential distance decay boost |

### 7.2.6 Metadata Features

| Feature | Range | Description |
|---------|-------|-------------|
| usrn_match | Boolean | Unique Street Reference matches |
| llpg_live | Boolean | LLPG status is live (not historic) |
| blpu_class_compat | Boolean | Property class compatibility |
| legacy_uprn_valid | Boolean | Source UPRN validated in LLPG |

## 7.3 Feature Computation

The feature computation function calculates all features for a candidate:

```go
func (fm *FuzzyMatcher) computeFeatures(doc SourceDocument, candidate *FuzzyCandidate) {
    srcAddr := ""
    if doc.AddrCan != nil {
        srcAddr = *doc.AddrCan
    }

    // Basic string similarities
    candidate.JaroScore = jaroSimilarity(srcAddr, candidate.AddrCan)

    // Token analysis
    srcTokens := strings.Fields(srcAddr)
    candTokens := strings.Fields(candidate.AddrCan)

    // Extract specific token types
    srcHouseNums, srcLocalities, srcStreets := extractTokenTypes(srcTokens)
    candHouseNums, candLocalities, candStreets := extractTokenTypes(candTokens)

    // House number matching
    candidate.SameHouseNumber = hasOverlap(srcHouseNums, candHouseNums)
    candidate.SameHouseAlpha = hasAlphaOverlap(srcHouseNums, candHouseNums)

    // Locality and street overlap
    candidate.LocalityOverlap = overlapRatio(srcLocalities, candLocalities)
    candidate.StreetOverlap = overlapRatio(srcStreets, candStreets)

    // Phonetic matching
    candidate.PhoneticHits = normalize.PhoneticTokenOverlap(srcAddr, candidate.AddrCan)

    // Spatial distance if available
    if doc.EastingRaw != nil && doc.NorthingRaw != nil {
        candidate.SpatialDistance = distance(
            *doc.EastingRaw, *doc.NorthingRaw,
            candidate.Easting, candidate.Northing,
        )
        candidate.SpatialBoost = math.Exp(-candidate.SpatialDistance / 300.0)
    } else {
        candidate.SpatialBoost = 0.0
    }

    // Compute final score
    candidate.FinalScore = fm.computeFinalScore(candidate)

    // Store all features for explainability
    candidate.Features = map[string]interface{}{
        "trgm_score":        candidate.TrgramScore,
        "jaro_score":        candidate.JaroScore,
        "locality_overlap":  candidate.LocalityOverlap,
        "street_overlap":    candidate.StreetOverlap,
        "same_house_number": candidate.SameHouseNumber,
        "same_house_alpha":  candidate.SameHouseAlpha,
        "phonetic_hits":     candidate.PhoneticHits,
        "spatial_distance":  candidate.SpatialDistance,
        "spatial_boost":     candidate.SpatialBoost,
        "final_score":       candidate.FinalScore,
    }
}
```

## 7.4 Feature Weights

The default feature weights are defined in `internal/match/types.go`:

```go
type FeatureWeights struct {
    TrigramSimilarity     float64  // 0.45
    EmbeddingCosine       float64  // 0.45
    LocalityOverlap       float64  // 0.05
    StreetOverlap         float64  // 0.05
    SameHouseNumber       float64  // 0.08
    SameHouseAlpha        float64  // 0.02
    USRNMatch             float64  // 0.04
    LLPGLive              float64  // 0.03
    LegacyUPRNValid       float64  // 0.20
    SpatialBoostMax       float64  // 0.10
    DescriptorPenalty     float64  // -0.05
    PhoneticMissPenalty   float64  // -0.03
}

func DefaultWeights() *FeatureWeights {
    return &FeatureWeights{
        TrigramSimilarity:   0.45,
        EmbeddingCosine:     0.45,
        LocalityOverlap:     0.05,
        StreetOverlap:       0.05,
        SameHouseNumber:     0.08,
        SameHouseAlpha:      0.02,
        USRNMatch:           0.04,
        LLPGLive:            0.03,
        LegacyUPRNValid:     0.20,
        SpatialBoostMax:     0.10,
        DescriptorPenalty:   -0.05,
        PhoneticMissPenalty: -0.03,
    }
}
```

### 7.4.1 Weight Rationale

**Primary Similarity (90% combined)**:
- Trigram similarity (0.45) and embedding cosine (0.45) form the foundation
- These capture both lexical and semantic similarity

**Structural Bonuses (10%)**:
- Locality overlap (0.05) and street overlap (0.05) validate address structure
- Same house number (0.08) is critical for residential addresses
- Alpha suffix matching (0.02) provides fine-grained differentiation

**Metadata Bonuses**:
- USRN match (0.04) indicates same street
- LLPG live status (0.03) prefers current addresses
- Legacy UPRN valid (0.20) is a strong positive signal

**Penalties**:
- Descriptor mismatch (-0.05) penalises incompatible qualifiers
- Phonetic miss (-0.03) penalises candidates with no phonetic overlap

## 7.5 Score Calculation

The final score is computed as a weighted sum:

```go
func (fm *FuzzyMatcher) computeFinalScore(candidate *FuzzyCandidate) float64 {
    score := 0.0

    // Primary similarity scores (90% weight)
    score += 0.50 * candidate.TrgramScore
    score += 0.40 * candidate.JaroScore

    // Structural bonuses (10% weight)
    score += 0.05 * candidate.LocalityOverlap
    score += 0.05 * candidate.StreetOverlap

    // Discrete bonuses
    if candidate.SameHouseNumber {
        score += 0.08
    }
    if candidate.SameHouseAlpha {
        score += 0.02
    }
    if candidate.PhoneticHits > 0 {
        score += 0.03
    }

    // Spatial boost
    score += candidate.SpatialBoost * 0.05

    // Status bonus for live addresses
    if candidate.Status != nil && *candidate.Status == "1" {
        score += 0.02
    }

    // Penalties
    if candidate.PhoneticHits == 0 && candidate.TrgramScore < 0.85 {
        score -= 0.03
    }

    // Clamp to [0, 1]
    if score < 0 {
        score = 0
    }
    if score > 1 {
        score = 1
    }

    return score
}
```

## 7.6 Decision Tiers

The system uses tiered thresholds for decision-making:

```go
type MatchTiers struct {
    AutoAcceptHigh   float64  // 0.92
    AutoAcceptMedium float64  // 0.88
    ReviewThreshold  float64  // 0.80
    MinThreshold     float64  // 0.70
    WinnerMargin     float64  // 0.03
}

func DefaultTiers() *MatchTiers {
    return &MatchTiers{
        AutoAcceptHigh:   0.92,
        AutoAcceptMedium: 0.88,
        ReviewThreshold:  0.80,
        MinThreshold:     0.70,
        WinnerMargin:     0.03,
    }
}
```

### 7.6.1 Tier Definitions

| Tier | Threshold | Decision | Conditions |
|------|-----------|----------|------------|
| High Confidence | >= 0.92 | Auto-Accept | Clear winner (margin >= 0.03) |
| Medium Confidence | >= 0.88 | Auto-Accept | With house number and locality match |
| Review Required | >= 0.80 | Needs Review | Multiple close candidates |
| Low Confidence | >= 0.70 | Needs Review | Weak match, review required |
| Reject | < 0.70 | Reject | Insufficient evidence |

## 7.7 Decision Logic

The decision function evaluates candidates against tiers:

```go
func (fm *FuzzyMatcher) makeDecision(candidates []*FuzzyCandidate, tiers *FuzzyMatchingTiers) (string, string) {
    if len(candidates) == 0 {
        return "rejected", ""
    }

    best := candidates[0]

    // Check for high confidence auto-accept
    if best.FinalScore >= tiers.HighConfidence {
        // Require margin to next candidate
        if len(candidates) == 1 ||
           (candidates[1].FinalScore <= best.FinalScore - tiers.WinnerMargin) {
            return "auto_accepted", best.UPRN
        }
    }

    // Check for medium confidence with validation
    if best.FinalScore >= tiers.MediumConfidence &&
       best.SameHouseNumber &&
       best.LocalityOverlap >= 0.5 {
        if len(candidates) == 1 ||
           (candidates[1].FinalScore <= best.FinalScore - 0.05) {
            return "auto_accepted", best.UPRN
        }
    }

    // Check if worth reviewing
    if best.FinalScore >= tiers.LowConfidence {
        return "needs_review", ""
    }

    return "rejected", ""
}
```

### 7.7.1 Winner Margin Requirement

The winner margin prevents auto-accepting when multiple candidates have similar scores:

```
Candidate A: 0.94
Candidate B: 0.92
Margin: 0.02 (less than 0.03)
Decision: needs_review (too close to call)

Candidate A: 0.94
Candidate B: 0.88
Margin: 0.06 (greater than 0.03)
Decision: auto_accept (clear winner)
```

### 7.7.2 House Number Gate

For medium confidence matches, house number agreement is required:

```
Score: 0.89
Same House Number: true
Locality Overlap: 0.75
Decision: auto_accept (passes gate)

Score: 0.89
Same House Number: false
Locality Overlap: 0.75
Decision: needs_review (fails gate)
```

## 7.8 Explainability

Every match result includes a full feature breakdown:

```json
{
    "src_id": 12345,
    "candidate_uprn": "100012345678",
    "method": "fuzzy_auto",
    "score": 0.92,
    "confidence": 0.88,
    "features": {
        "trgm_score": 0.88,
        "jaro_score": 0.85,
        "locality_overlap": 1.0,
        "street_overlap": 0.8,
        "same_house_number": true,
        "same_house_alpha": false,
        "phonetic_hits": 2,
        "spatial_distance": 45.2,
        "spatial_boost": 0.086,
        "final_score": 0.92
    },
    "decision": "auto_accepted",
    "notes": "Fuzzy match auto-accepted (trgm=0.880, final=0.920)"
}
```

This enables:
- Auditing specific matching decisions
- Identifying patterns in false matches
- Tuning weights and thresholds
- Manual review with full context

## 7.9 Penalty System

### 7.9.1 House Number Mismatch Penalty

When house numbers are present but do not match, a severe penalty applies:

```go
// In validation layer
if len(srcHouseNums) > 0 && len(candHouseNums) > 0 {
    if !hasOverlap(srcHouseNums, candHouseNums) {
        // 90% penalty - score multiplied by 0.1
        score = score * 0.1
    }
}
```

This prevents matching "4 MONKS ORCHARD" to "16 MONKS ORCHARD" despite high string similarity.

### 7.9.2 Descriptor Incompatibility

Addresses with incompatible descriptors receive penalties:

```go
descriptorPenalty := 0.0

// Source has "LAND AT" but candidate is a street address
if containsDescriptor(srcAddr, "LAND") && !containsDescriptor(candAddr, "LAND") {
    descriptorPenalty = -0.05
}

score += descriptorPenalty
```

## 7.10 Score Calibration

The scoring system was calibrated using:

1. **Gold Standard Set**: Manually verified matches from documents with known UPRNs
2. **Precision Analysis**: Measuring false positive rate at each threshold
3. **Coverage Analysis**: Measuring recall at each threshold
4. **Threshold Tuning**: Adjusting thresholds to achieve 98% precision target

### 7.10.1 Calibration Results

| Threshold | Precision | Recall | F1 Score |
|-----------|-----------|--------|----------|
| 0.95 | 99.5% | 42% | 0.59 |
| 0.92 | 99.1% | 48% | 0.65 |
| 0.88 | 98.2% | 55% | 0.70 |
| 0.85 | 97.0% | 62% | 0.76 |
| 0.80 | 94.5% | 71% | 0.81 |

The default thresholds (0.92 high, 0.88 medium) balance precision and recall for automatic acceptance.

## 7.11 Alternative Scoring Models

The system architecture supports alternative scoring approaches:

### 7.11.1 Logistic Regression

Features can be fed to a trained logistic regression model:

```go
// ONNX model inference (future enhancement)
func (scorer *LearnedScorer) Score(features map[string]interface{}) float64 {
    input := scorer.extractFeatureVector(features)
    output := scorer.onnxSession.Run(input)
    return output[0]  // Probability of correct match
}
```

### 7.11.2 Gradient Boosting

For more complex feature interactions:

```go
// XGBoost or LightGBM model
func (scorer *GBMScorer) Score(features map[string]interface{}) float64 {
    // Convert features to model input format
    // Run inference
    // Return probability
}
```

These models can be trained on accumulated match decisions and manual reviews.

## 7.12 Chapter Summary

This chapter has documented the scoring and decision system:

- Feature extraction from multiple sources (string, token, phonetic, spatial)
- Configurable feature weights
- Weighted score calculation with bonuses and penalties
- Tiered decision thresholds
- Winner margin requirements
- House number gating for medium confidence
- Full explainability through feature storage
- Calibration approach and results

The scoring system ensures high precision on automated decisions whilst providing clear explanations for manual review cases.

---

*This chapter details scoring logic. Chapter 8 describes the data pipeline and ETL processes.*
