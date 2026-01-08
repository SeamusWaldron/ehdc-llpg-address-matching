# Chapter 7: Scoring and Decision Logic

## 7.1 Scoring Overview

The scoring system transforms raw matching features into a single confidence score that drives automated decision-making. This chapter documents the complete scoring pipeline from feature extraction through final decision, including the mathematical foundations, weight rationale, and calibration methodology.

### 7.1.1 Design Principles

The scoring system prioritises:

1. **Explainability**: Every score can be decomposed into constituent features with their individual contributions
2. **Tuneability**: All weights and thresholds are configurable without code changes
3. **Conservatism**: High thresholds prevent false auto-accepts, prioritising precision over recall
4. **Auditability**: Complete feature storage enables post-hoc analysis and threshold refinement

### 7.1.2 Scoring Architecture

```
                                    +------------------+
Source Document -----> Feature ---->| Score Calculator |----> Decision
                       Extractor    +------------------+        |
                          |                  ^                  v
                          v                  |            +----------+
                    +----------+      +-----------+      | Database |
                    | Features |----->| Weights   |      | Storage  |
                    | Map      |      | Config    |      +----------+
                    +----------+      +-----------+
```

### 7.1.3 Score Range and Interpretation

| Score Range | Interpretation | Typical Decision |
|-------------|----------------|------------------|
| 0.95 - 1.00 | Near-certain match | Auto-accept |
| 0.92 - 0.94 | High confidence | Auto-accept |
| 0.88 - 0.91 | Good confidence | Auto-accept (with conditions) |
| 0.80 - 0.87 | Moderate confidence | Needs review |
| 0.70 - 0.79 | Low confidence | Needs review |
| Below 0.70 | Insufficient evidence | Reject |

## 7.2 Feature Extraction

For each candidate address, the system extracts a comprehensive feature set organised into categories.

### 7.2.1 String Similarity Features

| Feature | Range | Description | Typical Weight |
|---------|-------|-------------|----------------|
| trigram_similarity | 0.0-1.0 | PostgreSQL pg_trgm similarity | 0.45 |
| jaro_similarity | 0.0-1.0 | Jaro string similarity | 0.40 |
| levenshtein_norm | 0.0-1.0 | Normalised Levenshtein distance | Reference |
| cosine_bow | 0.0-1.0 | Bag-of-words cosine similarity | Reference |

### 7.2.2 Token Overlap Features

| Feature | Range | Description | Typical Weight |
|---------|-------|-------------|----------------|
| locality_overlap | 0.0-1.0 | Ratio of matching locality tokens | 0.05 |
| street_overlap | 0.0-1.0 | Ratio of matching street tokens | 0.05 |
| token_overlap | 0.0-1.0 | Overall token Jaccard similarity | Reference |

### 7.2.3 House Number Features

| Feature | Type | Description | Typical Boost |
|---------|------|-------------|---------------|
| same_house_number | Boolean | House numbers match exactly | +0.08 |
| same_house_alpha | Boolean | Alpha suffixes match (12A = 12A) | +0.02 |
| house_number_close | Boolean | Numbers within tolerance (12 vs 14) | +0.04 |
| house_number_conflict | Boolean | Numbers present but different | Penalty |

### 7.2.4 Phonetic Features

| Feature | Range | Description | Effect |
|---------|-------|-------------|--------|
| phonetic_hits | 0-N | Count of phonetic token matches | Bonus |
| phonetic_miss | Boolean | No phonetic overlap detected | -0.03 penalty |

### 7.2.5 Semantic Features

| Feature | Range | Description | Typical Weight |
|---------|-------|-------------|----------------|
| embedding_cosine | 0.0-1.0 | Cosine similarity of embeddings | 0.45 |

### 7.2.6 Spatial Features

| Feature | Range | Description | Effect |
|---------|-------|-------------|--------|
| distance_metres | 0-Inf | Distance in metres | Input to boost |
| spatial_boost | 0.0-0.10 | Exponential distance decay boost | Additive |
| distance_bucket | String | Category (0-100m, 100-250m, etc.) | Reference |

### 7.2.7 Metadata Features

| Feature | Type | Description | Typical Boost |
|---------|------|-------------|---------------|
| usrn_match | Boolean | Unique Street Reference matches | +0.04 |
| llpg_live | Boolean | LLPG status is live (not historic) | +0.03 |
| blpu_class_compat | Boolean | Property class compatibility | Reference |
| legacy_uprn_valid | Boolean | Source UPRN validated in LLPG | +0.20 |

## 7.3 Feature Computation Implementation

The feature computation function calculates all features for a candidate:

### 7.3.1 Core Feature Computer

```go
type FeatureComputer struct {
    weights   *FeatureWeights
    embedder  Embedder
    phonetics PhoneticsMatcher
}

func (fc *FeatureComputer) ComputeFeatures(input Input, candidate Candidate) map[string]interface{} {
    features := make(map[string]interface{})

    // Get canonical forms
    srcCanonical := input.AddrCan
    candCanonical := candidate.AddrCan

    // 1. String similarity features
    features["trigram_similarity"] = fc.trigramSimilarity(srcCanonical, candCanonical)
    features["jaro_similarity"] = JaroSimilarity(srcCanonical, candCanonical)
    features["levenshtein_similarity"] = fc.levenshteinSimilarity(srcCanonical, candCanonical)
    features["cosine_bow"] = fc.cosineBagOfWords(srcCanonical, candCanonical)

    // 2. Tokenise addresses
    srcTokens := tokenize(srcCanonical)
    candTokens := tokenize(candCanonical)

    // 3. Extract specific token types
    srcHouseNums := extractHouseNumbers(srcCanonical)
    candHouseNums := extractHouseNumbers(candCanonical)
    srcLocalities := extractLocalityTokens(srcCanonical)
    candLocalities := extractLocalityTokens(candCanonical)
    srcStreets := extractStreetTokens(srcCanonical)
    candStreets := extractStreetTokens(candCanonical)

    // 4. House number features
    features["has_same_house_num"] = hasOverlap(srcHouseNums, candHouseNums)
    features["has_same_house_alpha"] = hasAlphaOverlap(srcHouseNums, candHouseNums)
    features["house_number_match"] = fc.computeHouseNumberMatch(srcHouseNums, candHouseNums)

    // 5. Token overlap features
    features["locality_overlap_ratio"] = overlapRatio(srcLocalities, candLocalities)
    features["street_overlap_ratio"] = overlapRatio(srcStreets, candStreets)
    features["token_overlap"] = calculateTokenOverlap(srcTokens, candTokens)

    // 6. Phonetic features
    srcPhonetics := extractPhonetics(srcTokens)
    candPhonetics := extractPhonetics(candTokens)
    features["phonetic_hits"] = countPhoneticMatches(srcPhonetics, candPhonetics)

    // 7. Descriptor analysis
    features["descriptor_penalty"] = fc.hasDescriptorMismatch(input.RawAddress, candidate.LocAddress)

    // 8. Spatial features (if coordinates available)
    if input.Easting != nil && input.Northing != nil {
        distance := calculateDistance(
            *input.Easting, *input.Northing,
            candidate.Easting, candidate.Northing,
        )
        features["distance_metres"] = distance
        features["spatial_boost"] = calculateSpatialBoost(distance)
        features["distance_bucket"] = categoriseDistance(distance)
    } else {
        features["distance_metres"] = nil
        features["spatial_boost"] = 0.0
        features["distance_bucket"] = "unknown"
    }

    // 9. Metadata features
    features["llpg_live"] = candidate.Status == 1
    features["blpu_class_compat"] = fc.checkBLPUCompatibility(input, candidate)
    features["legacy_uprn_valid"] = input.LegacyUPRN != "" && input.LegacyUPRN == candidate.UPRN

    // 10. Embedding similarity (if available)
    if fc.embedder != nil {
        features["embedding_cosine"] = fc.embeddingCosine(srcCanonical, candCanonical)
    } else {
        features["embedding_cosine"] = 0.0
    }

    return features
}
```

### 7.3.2 Trigram Similarity Calculation

```go
func (fc *FeatureComputer) trigramSimilarity(s1, s2 string) float64 {
    // Uses PostgreSQL pg_trgm similarity function via database query
    // Fallback: compute locally using trigram set intersection/union

    if fc.db != nil {
        var similarity float64
        err := fc.db.QueryRow(`SELECT similarity($1, $2)`, s1, s2).Scan(&similarity)
        if err == nil {
            return similarity
        }
    }

    // Local computation fallback
    return fc.localTrigramSimilarity(s1, s2)
}

func (fc *FeatureComputer) localTrigramSimilarity(s1, s2 string) float64 {
    // Generate trigrams
    t1 := generateTrigrams(s1)
    t2 := generateTrigrams(s2)

    // Calculate Jaccard similarity of trigram sets
    intersection := setIntersection(t1, t2)
    union := setUnion(t1, t2)

    if len(union) == 0 {
        return 0.0
    }

    return float64(len(intersection)) / float64(len(union))
}

func generateTrigrams(s string) map[string]bool {
    s = "  " + strings.ToUpper(s) + " "  // Pad for edge trigrams
    trigrams := make(map[string]bool)

    for i := 0; i <= len(s)-3; i++ {
        trigram := s[i : i+3]
        trigrams[trigram] = true
    }

    return trigrams
}
```

### 7.3.3 Levenshtein Similarity

```go
func (fc *FeatureComputer) levenshteinSimilarity(s1, s2 string) float64 {
    dist := LevenshteinDistance(s1, s2)
    maxLen := max(len(s1), len(s2))

    if maxLen == 0 {
        return 1.0  // Both empty strings are identical
    }

    // Convert distance to similarity (1.0 = identical)
    return 1.0 - float64(dist)/float64(maxLen)
}
```

### 7.3.4 House Number Match Computation

```go
// computeHouseNumberMatch returns a score indicating house number agreement.
// Returns: 1.0 (exact), 0.5 (close), 0.0 (unknown), -1.0 (conflict)
func (fc *FeatureComputer) computeHouseNumberMatch(srcNums, candNums []string) float64 {
    // Case 1: Either side has no house numbers
    if len(srcNums) == 0 || len(candNums) == 0 {
        return 0.0  // Cannot determine - neutral
    }

    // Case 2: Check for exact match
    for _, sn := range srcNums {
        for _, cn := range candNums {
            if sn == cn {
                return 1.0  // Exact match
            }
        }
    }

    // Case 3: Check for close match (renumbering tolerance)
    for _, sn := range srcNums {
        srcNum := extractNumericPart(sn)
        if srcNum == -1 {
            continue
        }

        for _, cn := range candNums {
            candNum := extractNumericPart(cn)
            if candNum == -1 {
                continue
            }

            // Tolerance of 2 for historic renumbering
            if abs(srcNum-candNum) <= 2 {
                return 0.5  // Close match
            }
        }
    }

    // Case 4: Numbers present but no match - conflict
    return -1.0
}
```

### 7.3.5 Descriptor Mismatch Detection

```go
// hasDescriptorMismatch detects when source has property descriptors
// that the candidate lacks, indicating potential structural mismatch.
func (fc *FeatureComputer) hasDescriptorMismatch(srcAddr, candAddr string) bool {
    descriptors := []string{
        "LAND AT",
        "LAND ADJACENT",
        "REAR OF",
        "ADJACENT TO",
        "PLOT",
        "SITE OF",
        "PART OF",
        "GARAGE AT",
        "PARKING SPACE",
    }

    srcUpper := strings.ToUpper(srcAddr)
    candUpper := strings.ToUpper(candAddr)

    // Check if source contains descriptors
    hasSourceDescriptor := false
    for _, desc := range descriptors {
        if strings.Contains(srcUpper, desc) {
            hasSourceDescriptor = true
            break
        }
    }

    if !hasSourceDescriptor {
        return false  // No mismatch possible
    }

    // Check if candidate also has the descriptor
    hasCandidateDescriptor := false
    for _, desc := range descriptors {
        if strings.Contains(candUpper, desc) {
            hasCandidateDescriptor = true
            break
        }
    }

    // Mismatch: source has descriptor but candidate does not
    return hasSourceDescriptor && !hasCandidateDescriptor
}
```

### 7.3.6 Distance Categorisation

```go
func categoriseDistance(distanceMetres float64) string {
    switch {
    case distanceMetres <= 0:
        return "exact"
    case distanceMetres <= 25:
        return "0-25m"
    case distanceMetres <= 50:
        return "25-50m"
    case distanceMetres <= 100:
        return "50-100m"
    case distanceMetres <= 250:
        return "100-250m"
    case distanceMetres <= 500:
        return "250-500m"
    case distanceMetres <= 1000:
        return "500-1000m"
    case distanceMetres <= 2000:
        return "1000-2000m"
    default:
        return "2000m+"
    }
}
```

### 7.3.7 BLPU Class Compatibility

```go
// checkBLPUCompatibility determines if property classes are compatible.
// BLPU classes: R* = Residential, C* = Commercial, L* = Land, etc.
func (fc *FeatureComputer) checkBLPUCompatibility(input Input, candidate Candidate) bool {
    // If source has no class indication, always compatible
    if input.PropertyType == "" {
        return true
    }

    candClass := candidate.BLPUClass
    if candClass == "" {
        return true  // Unknown class is compatible
    }

    // Check class family compatibility
    srcFamily := getClassFamily(input.PropertyType)
    candFamily := getClassFamily(candClass)

    // Same family is always compatible
    if srcFamily == candFamily {
        return true
    }

    // Some cross-family combinations are acceptable
    compatiblePairs := map[string][]string{
        "R": {"X"},      // Residential compatible with mixed-use
        "C": {"X"},      // Commercial compatible with mixed-use
        "L": {"R", "C"}, // Land compatible with built properties
    }

    if allowed, exists := compatiblePairs[srcFamily]; exists {
        for _, a := range allowed {
            if candFamily == a {
                return true
            }
        }
    }

    return false
}

func getClassFamily(blpuClass string) string {
    if len(blpuClass) == 0 {
        return ""
    }
    return string(blpuClass[0])  // First character is the family
}
```

## 7.4 Feature Weights

The feature weights define how individual features contribute to the final score.

### 7.4.1 Weight Structure

```go
type FeatureWeights struct {
    // Core similarity weights (should sum to ~0.90)
    TrigramSimilarity float64 // Default: 0.45
    EmbeddingCosine   float64 // Default: 0.45

    // Token overlap weights (should sum to ~0.10)
    LocalityOverlap float64 // Default: 0.05
    StreetOverlap   float64 // Default: 0.05

    // Discrete boosts (additive when conditions met)
    SameHouseNumber float64 // Default: 0.08
    SameHouseAlpha  float64 // Default: 0.02
    USRNMatch       float64 // Default: 0.04
    LLPGLive        float64 // Default: 0.03
    LegacyUPRNValid float64 // Default: 0.20

    // Spatial (maximum additive boost)
    SpatialBoostMax float64 // Default: 0.10

    // Penalties (subtractive when conditions met)
    DescriptorPenalty   float64 // Default: -0.05
    PhoneticMissPenalty float64 // Default: -0.03
    HouseNumberConflict float64 // Default: -0.15
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
        HouseNumberConflict: -0.15,
    }
}
```

### 7.4.2 Weight Rationale

**Primary Similarity (90% combined)**:
- **Trigram similarity (0.45)**: PostgreSQL pg_trgm provides robust fuzzy matching that handles minor spelling variations and transpositions
- **Embedding cosine (0.45)**: Vector embeddings capture semantic similarity beyond lexical matching, useful for address variations with different word order

**Token Overlap (10% combined)**:
- **Locality overlap (0.05)**: Validates geographic component agreement (town, village names)
- **Street overlap (0.05)**: Validates street name component agreement

**Discrete Boosts**:
- **House number match (0.08)**: Critical for residential addresses - wrong house number is a strong negative signal
- **Alpha suffix match (0.02)**: Fine-grained differentiation (Flat 12A vs 12B)
- **USRN match (0.04)**: Same Unique Street Reference Number indicates same street
- **LLPG live status (0.03)**: Prefers current over historic addresses
- **Legacy UPRN valid (0.20)**: Strongest signal - validates existing metadata

**Spatial Boost (up to 0.10)**:
- Distance-based boost provides corroborating evidence
- Maximum 0.10 prevents over-reliance on coordinates alone

**Penalties**:
- **Descriptor mismatch (-0.05)**: Source describes partial property (land, plot) but candidate is full building
- **Phonetic miss (-0.03)**: No phonetic overlap suggests fundamentally different addresses
- **House number conflict (-0.15)**: Both have house numbers that do not match

### 7.4.3 Weight Allocation Visualisation

```
Score Composition (typical high-confidence match):
+--------------------------------------------------+
| Trigram (0.45 × 0.92)           = 0.414          |
+--------------------------------------------------+
| Embedding (0.45 × 0.88)         = 0.396          |
+--------------------------------------------------+
| Locality (0.05 × 1.0)           = 0.050          |
+--------------------------------------------------+
| Street (0.05 × 0.80)            = 0.040          |
+--------------------------------------------------+
| House Number Match              = 0.080          |
+--------------------------------------------------+
| LLPG Live                       = 0.030          |
+--------------------------------------------------+
| Spatial Boost                   = 0.060          |
+--------------------------------------------------+
                          Total   = 1.070 (clamped to 1.0)
```

## 7.5 Score Calculation

The final score is computed as a weighted sum of features with bonuses and penalties.

### 7.5.1 Main Scorer Implementation

```go
type Scorer struct {
    weights *FeatureWeights
    tiers   *MatchTiers
}

func NewScorer(weights *FeatureWeights, tiers *MatchTiers) *Scorer {
    if weights == nil {
        weights = DefaultWeights()
    }
    if tiers == nil {
        tiers = DefaultTiers()
    }
    return &Scorer{weights: weights, tiers: tiers}
}

func (s *Scorer) ScoreCandidate(features map[string]interface{}, legacyUPRNValid bool) float64 {
    score := 0.0

    // 1. Core similarity scores (90% weight)
    trigramSim := s.getFloatFeature(features, "trigram_similarity", 0.0)
    embeddingCos := s.getFloatFeature(features, "embedding_cosine", 0.0)

    score += trigramSim * s.weights.TrigramSimilarity
    score += embeddingCos * s.weights.EmbeddingCosine

    // 2. Token overlap scores (10% weight)
    localityOverlap := s.getFloatFeature(features, "locality_overlap_ratio", 0.0)
    streetOverlap := s.getFloatFeature(features, "street_overlap_ratio", 0.0)

    score += localityOverlap * s.weights.LocalityOverlap
    score += streetOverlap * s.weights.StreetOverlap

    // 3. Discrete boosts
    boosts := 0.0

    if s.getBoolFeature(features, "has_same_house_num") {
        boosts += s.weights.SameHouseNumber
    }
    if s.getBoolFeature(features, "has_same_house_alpha") {
        boosts += s.weights.SameHouseAlpha
    }
    if s.getBoolFeature(features, "usrn_match") {
        boosts += s.weights.USRNMatch
    }
    if s.getBoolFeature(features, "llpg_live") {
        boosts += s.weights.LLPGLive
    }
    if legacyUPRNValid {
        boosts += s.weights.LegacyUPRNValid
    }

    score += boosts

    // 4. Spatial boost
    spatialBoost := s.getFloatFeature(features, "spatial_boost", 0.0)
    score += spatialBoost  // Already clamped to [0, SpatialBoostMax]

    // 5. Penalties
    penalties := 0.0

    if s.getBoolFeature(features, "descriptor_penalty") {
        penalties += s.weights.DescriptorPenalty
    }

    phoneticHits := s.getIntFeature(features, "phonetic_hits", 0)
    if phoneticHits == 0 && trigramSim < 0.85 {
        penalties += s.weights.PhoneticMissPenalty
    }

    houseNumberMatch := s.getFloatFeature(features, "house_number_match", 0.0)
    if houseNumberMatch < 0 {
        penalties += s.weights.HouseNumberConflict
    }

    score += penalties

    // 6. Clamp to valid range
    if score < 0 {
        score = 0
    }
    if score > 1 {
        score = 1
    }

    return score
}
```

### 7.5.2 Feature Accessors

```go
func (s *Scorer) getFloatFeature(features map[string]interface{}, key string, defaultVal float64) float64 {
    if val, exists := features[key]; exists {
        switch v := val.(type) {
        case float64:
            return v
        case float32:
            return float64(v)
        case int:
            return float64(v)
        }
    }
    return defaultVal
}

func (s *Scorer) getBoolFeature(features map[string]interface{}, key string) bool {
    if val, exists := features[key]; exists {
        if b, ok := val.(bool); ok {
            return b
        }
    }
    return false
}

func (s *Scorer) getIntFeature(features map[string]interface{}, key string, defaultVal int) int {
    if val, exists := features[key]; exists {
        switch v := val.(type) {
        case int:
            return v
        case int64:
            return int(v)
        case float64:
            return int(v)
        }
    }
    return defaultVal
}
```

### 7.5.3 Alternative Scoring Formula (Fuzzy Matcher)

The database-backed fuzzy matcher uses a slightly different scoring formula:

```go
func (fm *FuzzyMatcher) computeFinalScore(candidate *FuzzyCandidate) float64 {
    score := 0.0

    // Primary similarity scores (90% weight)
    // Note: Uses Jaro instead of embedding cosine
    score += 0.50 * candidate.TrigramScore
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

    // Spatial boost (weighted lower)
    score += candidate.SpatialBoost * 0.05

    // Status bonus for live addresses
    if candidate.Status != nil && *candidate.Status == 1 {
        score += 0.02
    }

    // Penalties
    if candidate.PhoneticHits == 0 && candidate.TrigramScore < 0.85 {
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

**Key Differences from Main Scorer**:
- Uses Jaro similarity (40%) instead of embedding cosine (45%)
- Different trigram weight (50% vs 45%)
- Slightly lower spatial weight (0.05 vs 0.10 max)
- Simpler phonetic bonus instead of penalty-only

## 7.6 Spatial Boost Calculation

The spatial boost rewards candidates that are geographically close to the source document's coordinates.

### 7.6.1 Linear Decay Model

```go
// calculateSpatialBoost returns a boost based on distance using linear decay.
// Maximum boost 0.10 at exact location, zero at 2000m.
func calculateSpatialBoost(distanceMetres float64) float64 {
    const maxBoost = 0.10
    const maxDistance = 2000.0  // 2km

    if distanceMetres <= 0 {
        return maxBoost  // Perfect location match
    }

    if distanceMetres >= maxDistance {
        return 0.0  // Too far for any boost
    }

    // Linear decay: boost = maxBoost * (1 - distance/maxDistance)
    boost := maxBoost * (1.0 - distanceMetres/maxDistance)

    return boost
}
```

**Linear Decay Characteristics**:

| Distance | Spatial Boost | Contribution |
|----------|---------------|--------------|
| 0m | 0.100 | +10.0% |
| 250m | 0.0875 | +8.75% |
| 500m | 0.075 | +7.5% |
| 1000m | 0.050 | +5.0% |
| 1500m | 0.025 | +2.5% |
| 2000m | 0.000 | +0.0% |

### 7.6.2 Exponential Decay Model (Alternative)

```go
// calculateSpatialBoostExponential uses exponential decay for smoother falloff.
// Scale parameter determines decay rate (300m = ~37% at one scale distance).
func calculateSpatialBoostExponential(distanceMetres float64) float64 {
    const maxBoost = 0.10
    const scaleParameter = 300.0  // Distance at which boost is ~37% of max

    if distanceMetres <= 0 {
        return maxBoost
    }

    // Exponential decay: boost = maxBoost * e^(-distance/scale)
    boost := maxBoost * math.Exp(-distanceMetres/scaleParameter)

    // Floor at very small values
    if boost < 0.001 {
        return 0.0
    }

    return boost
}
```

**Exponential Decay Characteristics**:

| Distance | e^(-d/300) | Boost (x 0.10) |
|----------|------------|----------------|
| 0m | 1.000 | 0.100 |
| 150m | 0.606 | 0.061 |
| 300m | 0.368 | 0.037 |
| 600m | 0.135 | 0.014 |
| 900m | 0.050 | 0.005 |
| 1200m | 0.018 | 0.002 |

### 7.6.3 Distance Calculation

```go
// calculateDistance computes Euclidean distance in British National Grid coordinates.
func calculateDistance(e1, n1, e2, n2 float64) float64 {
    de := e1 - e2
    dn := n1 - n2
    return math.Sqrt(de*de + dn*dn)
}
```

## 7.7 Decision Tiers

The system uses tiered thresholds for decision-making, balancing precision and automation rate.

### 7.7.1 Tier Structure

```go
type MatchTiers struct {
    AutoAcceptHigh   float64 // High confidence auto-accept threshold
    AutoAcceptMedium float64 // Medium confidence with conditions
    ReviewThreshold  float64 // Minimum score for manual review
    MinThreshold     float64 // Minimum score to consider at all
    WinnerMargin     float64 // Required gap to second-best candidate
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

### 7.7.2 Tier Definitions and Conditions

| Tier | Score Threshold | Margin Required | Additional Conditions | Decision |
|------|-----------------|-----------------|----------------------|----------|
| High Confidence | >= 0.92 | >= 0.03 | None | Auto-Accept |
| Medium Confidence | >= 0.88 | >= 0.05 | House number AND locality overlap >= 0.5 | Auto-Accept |
| Review | >= 0.80 | N/A | Score qualifies but conditions not met | Needs Review |
| Low Confidence | >= 0.70 | N/A | Below auto-accept thresholds | Needs Review |
| Reject | < 0.70 | N/A | Insufficient evidence | Reject |

### 7.7.3 Fuzzy Matcher Tiers (Alternative Configuration)

```go
type FuzzyMatchingTiers struct {
    HighConfidence   float64 // Default: 0.85
    MediumConfidence float64 // Default: 0.78
    LowConfidence    float64 // Default: 0.70
    MinThreshold     float64 // Default: 0.60
    WinnerMargin     float64 // Default: 0.05
}

func DefaultFuzzyTiers() *FuzzyMatchingTiers {
    return &FuzzyMatchingTiers{
        HighConfidence:   0.85,
        MediumConfidence: 0.78,
        LowConfidence:    0.70,
        MinThreshold:     0.60,
        WinnerMargin:     0.05,
    }
}
```

## 7.8 Decision Logic

The decision function evaluates candidates against tiers and determines the match outcome.

### 7.8.1 Main Decision Algorithm

```go
func (s *Scorer) MakeDecision(candidates []Candidate) (decision string, acceptedUPRN string) {
    // Step 1: Check if any candidates exist
    if len(candidates) == 0 {
        return "reject", ""
    }

    // Candidates should already be sorted by score descending
    topCandidate := candidates[0]
    topScore := topCandidate.Score

    // Step 2: Check minimum threshold
    if topScore < s.tiers.MinThreshold {
        return "reject", ""
    }

    // Step 3: Calculate margin to next candidate
    var margin float64 = 1.0  // Perfect margin if only one candidate
    if len(candidates) > 1 {
        margin = topScore - candidates[1].Score
    }

    // Step 4a: High confidence auto-accept
    if topScore >= s.tiers.AutoAcceptHigh && margin >= s.tiers.WinnerMargin {
        return "auto_accept", topCandidate.UPRN
    }

    // Step 4b: Medium confidence auto-accept (with additional validation)
    if topScore >= s.tiers.AutoAcceptMedium {
        // Require larger margin for medium confidence
        if margin >= s.tiers.WinnerMargin+0.02 {
            // Check additional conditions
            hasHouseNumber := s.getBoolFeature(topCandidate.Features, "has_same_house_num")
            localityOverlap := s.getFloatFeature(topCandidate.Features, "locality_overlap_ratio", 0.0)

            if hasHouseNumber && localityOverlap >= 0.5 {
                return "auto_accept", topCandidate.UPRN
            }
        }
    }

    // Step 5: Review threshold
    if topScore >= s.tiers.ReviewThreshold {
        return "review", ""
    }

    // Step 6: Default reject
    return "reject", ""
}
```

### 7.8.2 Decision Flow Diagram

```
                    Start
                      |
                      v
              +----------------+
              | Has Candidates?|
              +----------------+
                |           |
               No          Yes
                |           |
                v           v
            REJECT    +----------------+
                      | Score >= 0.70? |
                      +----------------+
                        |           |
                       No          Yes
                        |           |
                        v           v
                    REJECT    +------------------+
                              | Score >= 0.92?   |
                              +------------------+
                                |            |
                               No           Yes
                                |            |
                                v            v
                        +------------+   +--------------+
                        | Score >=   |   | Margin >=    |
                        | 0.88?      |   | 0.03?        |
                        +------------+   +--------------+
                         |        |       |          |
                        No       Yes     No         Yes
                         |        |       |          |
                         v        v       v          v
                    +---------+  Check   REVIEW  AUTO_ACCEPT
                    | Score   |  House              (High)
                    | >= 0.80?|  Number
                    +---------+  + Locality
                     |      |      |      |
                    No     Yes    Fail   Pass
                     |      |      |      |
                     v      v      v      v
                  REJECT  REVIEW REVIEW AUTO_ACCEPT
                                         (Medium)
```

### 7.8.3 Winner Margin Requirement

The winner margin prevents auto-accepting when multiple candidates have similar scores:

```go
// Example 1: Clear winner
Candidate A: 0.94
Candidate B: 0.88
Margin: 0.06 (>= 0.03)
Decision: AUTO_ACCEPT (clear winner)

// Example 2: Too close
Candidate A: 0.94
Candidate B: 0.92
Margin: 0.02 (< 0.03)
Decision: REVIEW (too close to call)

// Example 3: Single candidate
Candidate A: 0.91
Margin: 1.0 (no competition)
Decision: REVIEW (score < 0.92 threshold)
```

### 7.8.4 House Number Gate

For medium confidence matches, house number agreement is required:

```go
// Example 1: Passes gate
Score: 0.89
Same House Number: true
Locality Overlap: 0.75
Margin: 0.08 (>= 0.05)
Decision: AUTO_ACCEPT (passes all conditions)

// Example 2: Fails house number check
Score: 0.89
Same House Number: false
Locality Overlap: 0.75
Margin: 0.08
Decision: REVIEW (fails house number condition)

// Example 3: Fails locality check
Score: 0.89
Same House Number: true
Locality Overlap: 0.30 (< 0.50)
Margin: 0.08
Decision: REVIEW (fails locality condition)

// Example 4: Fails margin check
Score: 0.89
Same House Number: true
Locality Overlap: 0.75
Margin: 0.03 (< 0.05 for medium)
Decision: REVIEW (insufficient margin)
```

### 7.8.5 Fuzzy Matcher Decision Logic

```go
func (fm *FuzzyMatcher) makeDecision(candidates []*FuzzyCandidate, tiers *FuzzyMatchingTiers) (string, string) {
    if len(candidates) == 0 {
        return "rejected", ""
    }

    best := candidates[0]

    // High confidence auto-accept
    if best.FinalScore >= tiers.HighConfidence {
        if len(candidates) == 1 {
            return "auto_accepted", best.UPRN
        }
        if candidates[1].FinalScore <= best.FinalScore-tiers.WinnerMargin {
            return "auto_accepted", best.UPRN
        }
    }

    // Medium confidence with validation gates
    if best.FinalScore >= tiers.MediumConfidence {
        if best.SameHouseNumber && best.LocalityOverlap >= 0.5 {
            // Require larger margin for medium confidence
            if len(candidates) == 1 {
                return "auto_accepted", best.UPRN
            }
            if candidates[1].FinalScore <= best.FinalScore-0.05 {
                return "auto_accepted", best.UPRN
            }
        }
    }

    // Review threshold
    if best.FinalScore >= tiers.LowConfidence {
        return "needs_review", ""
    }

    return "rejected", ""
}
```

## 7.9 Tie-Breaking and Candidate Ranking

When multiple candidates have similar scores, the system employs sophisticated tie-breaking logic.

### 7.9.1 Primary Ranking by Score

```go
// sortCandidatesByScore sorts candidates in descending order by final score.
func sortCandidatesByScore(candidates []*Candidate) {
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].Score > candidates[j].Score
    })
}
```

### 7.9.2 Secondary Ranking Criteria

When scores are identical (within epsilon), secondary criteria apply:

```go
func sortCandidatesWithTieBreaking(candidates []*Candidate) {
    const epsilon = 0.001  // Scores within this are considered tied

    sort.Slice(candidates, func(i, j int) bool {
        // Primary: Score (descending)
        if math.Abs(candidates[i].Score-candidates[j].Score) > epsilon {
            return candidates[i].Score > candidates[j].Score
        }

        // Tie-breaker 1: House number match (exact > close > none)
        hnI := candidates[i].Features["house_number_match"].(float64)
        hnJ := candidates[j].Features["house_number_match"].(float64)
        if hnI != hnJ {
            return hnI > hnJ
        }

        // Tie-breaker 2: LLPG status (live > provisional > historic)
        statusPriority := map[int]int{1: 3, 8: 2, 6: 1}
        pI := statusPriority[candidates[i].Status]
        pJ := statusPriority[candidates[j].Status]
        if pI != pJ {
            return pI > pJ
        }

        // Tie-breaker 3: Spatial proximity (closer is better)
        distI := candidates[i].Features["distance_metres"]
        distJ := candidates[j].Features["distance_metres"]
        if distI != nil && distJ != nil {
            return distI.(float64) < distJ.(float64)
        }

        // Tie-breaker 4: Trigram score (prefer higher primary similarity)
        return candidates[i].TrigramScore > candidates[j].TrigramScore
    })
}
```

### 7.9.3 Deduplication by UPRN

When the same UPRN appears from multiple matching methods:

```go
func dedupeByUPRN(candidates []*Candidate) []*Candidate {
    // Sort by score first
    sortCandidatesByScore(candidates)

    uprnMap := make(map[string]*Candidate)
    var result []*Candidate

    for _, cand := range candidates {
        existing, exists := uprnMap[cand.UPRN]

        if !exists {
            // First occurrence - keep it
            uprnMap[cand.UPRN] = cand
            result = append(result, cand)
        } else {
            // Duplicate UPRN - merge method information
            existing.Methods = append(existing.Methods, cand.Method)

            // Keep higher score (should already be the case from sorting)
            if cand.Score > existing.Score {
                existing.Score = cand.Score
                existing.Features = cand.Features
            }
        }
    }

    return result
}
```

### 7.9.4 Tie Rank Assignment

For candidates sent to review, tie ranks indicate their relative ordering:

```go
func assignTieRanks(candidates []*Candidate) {
    for i := range candidates {
        candidates[i].TieRank = i + 1

        // Mark candidates with close scores as ties
        if i > 0 {
            scoreDiff := candidates[i-1].Score - candidates[i].Score
            if scoreDiff < 0.02 {
                candidates[i].IsTie = true
                candidates[i-1].IsTie = true
            }
        }
    }
}
```

## 7.10 Explainability

Every match result includes a full feature breakdown enabling audit and analysis.

### 7.10.1 Explanation Generation

```go
func (s *Scorer) GetExplanation(candidate Candidate, legacyUPRNValid bool) map[string]interface{} {
    explanation := make(map[string]interface{})
    features := candidate.Features

    // 1. Component contributions
    trigramSim := s.getFloatFeature(features, "trigram_similarity", 0.0)
    embeddingCos := s.getFloatFeature(features, "embedding_cosine", 0.0)
    localityOverlap := s.getFloatFeature(features, "locality_overlap_ratio", 0.0)
    streetOverlap := s.getFloatFeature(features, "street_overlap_ratio", 0.0)

    explanation["trigram_contribution"] = trigramSim * s.weights.TrigramSimilarity
    explanation["embedding_contribution"] = embeddingCos * s.weights.EmbeddingCosine
    explanation["locality_contribution"] = localityOverlap * s.weights.LocalityOverlap
    explanation["street_contribution"] = streetOverlap * s.weights.StreetOverlap

    // 2. Boosts applied
    boosts := make(map[string]float64)

    if s.getBoolFeature(features, "has_same_house_num") {
        boosts["house_number"] = s.weights.SameHouseNumber
    }
    if s.getBoolFeature(features, "has_same_house_alpha") {
        boosts["house_alpha"] = s.weights.SameHouseAlpha
    }
    if s.getBoolFeature(features, "llpg_live") {
        boosts["llpg_live"] = s.weights.LLPGLive
    }
    if legacyUPRNValid {
        boosts["legacy_uprn"] = s.weights.LegacyUPRNValid
    }

    explanation["boosts"] = boosts
    explanation["boosts_total"] = sumValues(boosts)

    // 3. Spatial contribution
    spatialBoost := s.getFloatFeature(features, "spatial_boost", 0.0)
    explanation["spatial_contribution"] = spatialBoost
    explanation["distance_metres"] = s.getFloatFeature(features, "distance_metres", -1)
    explanation["distance_bucket"] = features["distance_bucket"]

    // 4. Penalties applied
    penalties := make(map[string]float64)

    if s.getBoolFeature(features, "descriptor_penalty") {
        penalties["descriptor_mismatch"] = s.weights.DescriptorPenalty
    }

    phoneticHits := s.getIntFeature(features, "phonetic_hits", 0)
    if phoneticHits == 0 && trigramSim < 0.85 {
        penalties["phonetic_miss"] = s.weights.PhoneticMissPenalty
    }

    houseNumberMatch := s.getFloatFeature(features, "house_number_match", 0.0)
    if houseNumberMatch < 0 {
        penalties["house_number_conflict"] = s.weights.HouseNumberConflict
    }

    explanation["penalties"] = penalties
    explanation["penalties_total"] = sumValues(penalties)

    // 5. Summary
    explanation["final_score"] = candidate.Score
    explanation["methods"] = candidate.Methods

    return explanation
}
```

### 7.10.2 Example Explanation Output

```json
{
    "trigram_contribution": 0.405,
    "embedding_contribution": 0.369,
    "locality_contribution": 0.0375,
    "street_contribution": 0.035,
    "boosts": {
        "house_number": 0.08,
        "house_alpha": 0.02,
        "llpg_live": 0.03
    },
    "boosts_total": 0.13,
    "spatial_contribution": 0.08,
    "distance_metres": 45.2,
    "distance_bucket": "25-50m",
    "penalties": {
        "descriptor_mismatch": -0.05
    },
    "penalties_total": -0.05,
    "final_score": 0.9065,
    "methods": ["trigram", "embedding", "spatial"]
}
```

### 7.10.3 Human-Readable Explanation

```go
func (s *Scorer) GetHumanReadableExplanation(candidate Candidate) string {
    exp := s.GetExplanation(candidate, false)

    var parts []string

    // Score summary
    parts = append(parts, fmt.Sprintf("Final Score: %.3f", candidate.Score))

    // Primary contributors
    parts = append(parts, fmt.Sprintf("  Trigram: %.3f (%.1f%%)",
        exp["trigram_contribution"].(float64),
        exp["trigram_contribution"].(float64)*100))

    parts = append(parts, fmt.Sprintf("  Embedding: %.3f (%.1f%%)",
        exp["embedding_contribution"].(float64),
        exp["embedding_contribution"].(float64)*100))

    // Boosts
    if boosts, ok := exp["boosts"].(map[string]float64); ok && len(boosts) > 0 {
        parts = append(parts, "  Boosts:")
        for name, value := range boosts {
            parts = append(parts, fmt.Sprintf("    +%.3f %s", value, name))
        }
    }

    // Penalties
    if penalties, ok := exp["penalties"].(map[string]float64); ok && len(penalties) > 0 {
        parts = append(parts, "  Penalties:")
        for name, value := range penalties {
            parts = append(parts, fmt.Sprintf("    %.3f %s", value, name))
        }
    }

    // Spatial
    if dist, ok := exp["distance_metres"].(float64); ok && dist >= 0 {
        parts = append(parts, fmt.Sprintf("  Spatial: %.1fm (%s)",
            dist, exp["distance_bucket"]))
    }

    return strings.Join(parts, "\n")
}
```

### 7.10.4 Debug Tracing

```go
func (s *Scorer) ScoreCandidateWithDebug(localDebug bool, features map[string]interface{}, legacyUPRNValid bool) float64 {
    if localDebug {
        log.Printf("=== SCORING DEBUG ===")
    }

    score := 0.0

    // Core similarities
    trigramSim := s.getFloatFeature(features, "trigram_similarity", 0.0)
    embeddingCos := s.getFloatFeature(features, "embedding_cosine", 0.0)

    trigramContrib := trigramSim * s.weights.TrigramSimilarity
    embeddingContrib := embeddingCos * s.weights.EmbeddingCosine

    score += trigramContrib + embeddingContrib

    if localDebug {
        log.Printf("  Core: trgm=%.3f*%.2f + emb=%.3f*%.2f = %.4f",
            trigramSim, s.weights.TrigramSimilarity,
            embeddingCos, s.weights.EmbeddingCosine,
            trigramContrib+embeddingContrib)
    }

    // Token overlaps
    localityOverlap := s.getFloatFeature(features, "locality_overlap_ratio", 0.0)
    streetOverlap := s.getFloatFeature(features, "street_overlap_ratio", 0.0)

    score += localityOverlap*s.weights.LocalityOverlap + streetOverlap*s.weights.StreetOverlap

    if localDebug {
        log.Printf("  Tokens: locality=%.3f*%.2f + street=%.3f*%.2f = %.4f",
            localityOverlap, s.weights.LocalityOverlap,
            streetOverlap, s.weights.StreetOverlap,
            localityOverlap*s.weights.LocalityOverlap+streetOverlap*s.weights.StreetOverlap)
    }

    // ... continue for all components

    if localDebug {
        log.Printf("  Final (clamped): %.4f", math.Max(0, math.Min(1, score)))
        log.Printf("=== END SCORING ===")
    }

    return math.Max(0, math.Min(1, score))
}
```

## 7.11 Penalty System

The penalty system reduces scores for specific warning conditions.

### 7.11.1 House Number Conflict Penalty

When house numbers are present in both addresses but do not match:

```go
func applyHouseNumberPenalty(score float64, srcNums, candNums []string) float64 {
    // Only apply if both have house numbers
    if len(srcNums) == 0 || len(candNums) == 0 {
        return score
    }

    // Check for any match
    for _, sn := range srcNums {
        for _, cn := range candNums {
            if sn == cn {
                return score  // Match found - no penalty
            }
            // Check close match (renumbering tolerance)
            if isCloseNumber(sn, cn, 2) {
                return score  // Close enough - no penalty
            }
        }
    }

    // No match found - apply severe penalty
    // This prevents "4 MONKS ORCHARD" matching "16 MONKS ORCHARD"
    return score * 0.1  // 90% reduction
}
```

### 7.11.2 Descriptor Incompatibility Penalty

```go
func applyDescriptorPenalty(score float64, srcAddr, candAddr string, weight float64) float64 {
    if hasDescriptorMismatch(srcAddr, candAddr) {
        return score + weight  // weight is negative, e.g., -0.05
    }
    return score
}
```

### 7.11.3 Phonetic Miss Penalty

```go
func applyPhoneticPenalty(score float64, phoneticHits int, trigramScore float64, weight float64) float64 {
    // Only apply penalty for lower similarity matches with no phonetic overlap
    if phoneticHits == 0 && trigramScore < 0.85 {
        return score + weight  // weight is negative, e.g., -0.03
    }
    return score
}
```

### 7.11.4 Status Penalty

```go
func applyStatusPenalty(score float64, status int) float64 {
    switch status {
    case 1:  // Live
        return score  // No penalty
    case 8:  // Provisional
        return score - 0.01  // Small penalty
    case 6:  // Historic
        return score - 0.03  // Larger penalty for historic addresses
    default:
        return score - 0.02  // Unknown status
    }
}
```

### 7.11.5 Penalty Summary

| Penalty | Condition | Effect | Rationale |
|---------|-----------|--------|-----------|
| House Number Conflict | Both have numbers, no match | Score * 0.1 | Wrong house is a critical error |
| Descriptor Mismatch | Source has descriptor, candidate lacks | -0.05 | Structural incompatibility |
| Phonetic Miss | No phonetic overlap, trgm < 0.85 | -0.03 | Fundamentally different addresses |
| Historic Status | LLPG status is historic (6) | -0.03 | Prefer current addresses |
| Provisional Status | LLPG status is provisional (8) | -0.01 | Slight preference for confirmed |

## 7.12 Score Calibration

The scoring system was calibrated using empirical data to achieve target precision levels.

### 7.12.1 Calibration Methodology

```go
type ThresholdTuner struct {
    db        *sql.DB
    goldSet   []GoldStandardMatch  // Verified matches
    testSet   []TestDocument       // Documents with unknown matches
}

func (tt *ThresholdTuner) TestThresholds(sampleSize int) {
    thresholds := []float64{0.50, 0.55, 0.60, 0.65, 0.70, 0.75, 0.80, 0.85, 0.90, 0.95}

    for _, threshold := range thresholds {
        // Create test tiers
        tiers := &MatchTiers{
            AutoAcceptHigh:   math.Min(threshold+0.07, 0.97),
            AutoAcceptMedium: math.Min(threshold+0.03, 0.93),
            ReviewThreshold:  threshold,
            MinThreshold:     threshold - 0.10,
            WinnerMargin:     0.03,
        }

        // Run matching against gold standard
        result := tt.evaluateThreshold(tiers)

        log.Printf("Threshold %.2f: Precision=%.3f Recall=%.3f F1=%.3f",
            threshold, result.Precision, result.Recall, result.F1Score)
    }
}
```

### 7.12.2 Evaluation Metrics

```go
type ThresholdResult struct {
    Threshold  float64
    TruePos    int     // Correct auto-accepts
    FalsePos   int     // Incorrect auto-accepts
    TrueNeg    int     // Correct rejections
    FalseNeg   int     // Missed matches
    Precision  float64 // TP / (TP + FP)
    Recall     float64 // TP / (TP + FN)
    F1Score    float64 // 2 * (Precision * Recall) / (Precision + Recall)
}

func (tt *ThresholdTuner) evaluateThreshold(tiers *MatchTiers) *ThresholdResult {
    result := &ThresholdResult{Threshold: tiers.ReviewThreshold}

    for _, gold := range tt.goldSet {
        // Run matching
        candidates := tt.generateCandidates(gold.SourceDocument)
        decision, acceptedUPRN := tt.makeDecision(candidates, tiers)

        // Compare to gold standard
        if decision == "auto_accept" {
            if acceptedUPRN == gold.CorrectUPRN {
                result.TruePos++
            } else {
                result.FalsePos++
            }
        } else {
            if gold.CorrectUPRN != "" {
                result.FalseNeg++
            } else {
                result.TrueNeg++
            }
        }
    }

    // Calculate metrics
    if result.TruePos+result.FalsePos > 0 {
        result.Precision = float64(result.TruePos) / float64(result.TruePos+result.FalsePos)
    }
    if result.TruePos+result.FalseNeg > 0 {
        result.Recall = float64(result.TruePos) / float64(result.TruePos+result.FalseNeg)
    }
    if result.Precision+result.Recall > 0 {
        result.F1Score = 2 * (result.Precision * result.Recall) / (result.Precision + result.Recall)
    }

    return result
}
```

### 7.12.3 Calibration Results

| Threshold | Precision | Recall | F1 Score | Notes |
|-----------|-----------|--------|----------|-------|
| 0.95 | 99.6% | 42% | 0.59 | Very conservative |
| 0.92 | 99.1% | 48% | 0.65 | **Selected: High tier** |
| 0.88 | 98.2% | 55% | 0.70 | **Selected: Medium tier** |
| 0.85 | 97.0% | 62% | 0.76 | Below target precision |
| 0.80 | 94.5% | 71% | 0.81 | **Selected: Review tier** |
| 0.75 | 91.2% | 78% | 0.84 | Too many false positives |
| 0.70 | 86.5% | 84% | 0.85 | **Selected: Minimum tier** |

### 7.12.4 Optimal Threshold Selection

```go
func (tt *ThresholdTuner) findOptimalThreshold(minPrecision float64) float64 {
    bestThreshold := 0.0
    bestF1 := 0.0

    for threshold := 0.50; threshold <= 0.95; threshold += 0.01 {
        result := tt.evaluateThreshold(&MatchTiers{
            AutoAcceptHigh: threshold,
            MinThreshold:   threshold - 0.20,
            WinnerMargin:   0.03,
        })

        // Must meet minimum precision requirement
        if result.Precision < minPrecision {
            continue
        }

        // Optimise for F1 score
        if result.F1Score > bestF1 {
            bestF1 = result.F1Score
            bestThreshold = threshold
        }
    }

    return bestThreshold
}
```

## 7.13 Alternative Scoring Models

The system architecture supports alternative scoring approaches for future enhancement.

### 7.13.1 Logistic Regression Model

```go
type LogisticScorer struct {
    coefficients map[string]float64
    intercept    float64
}

func (ls *LogisticScorer) Score(features map[string]interface{}) float64 {
    // Linear combination
    z := ls.intercept

    for feature, coef := range ls.coefficients {
        if val, ok := features[feature]; ok {
            switch v := val.(type) {
            case float64:
                z += coef * v
            case bool:
                if v {
                    z += coef
                }
            }
        }
    }

    // Sigmoid activation
    return 1.0 / (1.0 + math.Exp(-z))
}

// Example trained coefficients
func NewTrainedLogisticScorer() *LogisticScorer {
    return &LogisticScorer{
        intercept: -2.5,
        coefficients: map[string]float64{
            "trigram_similarity":     3.2,
            "jaro_similarity":        2.8,
            "locality_overlap_ratio": 1.5,
            "street_overlap_ratio":   1.2,
            "has_same_house_num":     2.1,
            "phonetic_hits":          0.3,
            "spatial_boost":          1.8,
            "llpg_live":              0.5,
        },
    }
}
```

### 7.13.2 Gradient Boosting Model

```go
type GBMScorer struct {
    model   *xgb.Booster  // XGBoost or LightGBM model
    columns []string      // Feature column order
}

func (gs *GBMScorer) Score(features map[string]interface{}) float64 {
    // Convert features to matrix row
    row := make([]float32, len(gs.columns))
    for i, col := range gs.columns {
        if val, ok := features[col]; ok {
            row[i] = toFloat32(val)
        }
    }

    // Run inference
    mat := xgb.DMatrixCreateFromMat([][]float32{row}, -1)
    predictions := gs.model.Predict(mat)

    return float64(predictions[0])
}
```

### 7.13.3 Ensemble Scoring

```go
type EnsembleScorer struct {
    scorers []ScorerInterface
    weights []float64
}

func (es *EnsembleScorer) Score(features map[string]interface{}) float64 {
    totalWeight := 0.0
    weightedSum := 0.0

    for i, scorer := range es.scorers {
        score := scorer.Score(features)
        weightedSum += score * es.weights[i]
        totalWeight += es.weights[i]
    }

    if totalWeight == 0 {
        return 0.0
    }

    return weightedSum / totalWeight
}

// Example: 60% weighted scorer, 40% ML model
func NewHybridEnsemble() *EnsembleScorer {
    return &EnsembleScorer{
        scorers: []ScorerInterface{
            NewScorer(DefaultWeights(), DefaultTiers()),
            NewTrainedLogisticScorer(),
        },
        weights: []float64{0.6, 0.4},
    }
}
```

## 7.14 Configuration Reference

### 7.14.1 Complete Weight Configuration

| Weight | Default | Min | Max | Description |
|--------|---------|-----|-----|-------------|
| TrigramSimilarity | 0.45 | 0.30 | 0.60 | PostgreSQL pg_trgm weight |
| EmbeddingCosine | 0.45 | 0.00 | 0.60 | Vector embedding weight |
| LocalityOverlap | 0.05 | 0.02 | 0.10 | Locality token overlap |
| StreetOverlap | 0.05 | 0.02 | 0.10 | Street token overlap |
| SameHouseNumber | 0.08 | 0.05 | 0.15 | House number match boost |
| SameHouseAlpha | 0.02 | 0.01 | 0.05 | Alpha suffix match boost |
| USRNMatch | 0.04 | 0.02 | 0.08 | USRN match boost |
| LLPGLive | 0.03 | 0.01 | 0.05 | Live status boost |
| LegacyUPRNValid | 0.20 | 0.15 | 0.30 | Legacy UPRN validation boost |
| SpatialBoostMax | 0.10 | 0.05 | 0.15 | Maximum spatial boost |
| DescriptorPenalty | -0.05 | -0.10 | -0.02 | Descriptor mismatch penalty |
| PhoneticMissPenalty | -0.03 | -0.05 | -0.01 | No phonetic match penalty |
| HouseNumberConflict | -0.15 | -0.25 | -0.10 | House number conflict penalty |

### 7.14.2 Complete Threshold Configuration

| Threshold | Default | Purpose |
|-----------|---------|---------|
| AutoAcceptHigh | 0.92 | High confidence auto-accept |
| AutoAcceptMedium | 0.88 | Medium confidence with conditions |
| ReviewThreshold | 0.80 | Minimum for manual review queue |
| MinThreshold | 0.70 | Minimum to consider candidate |
| WinnerMargin | 0.03 | Required gap to runner-up |

## 7.15 Chapter Summary

This chapter has documented the complete scoring and decision system:

- **Feature extraction**: 16+ features across string similarity, token overlap, phonetic, spatial, and metadata categories
- **Feature computation**: Trigram, Jaro, Levenshtein, cosine similarity calculations with full implementations
- **Weight system**: Configurable weights with 90% core similarity, 10% token overlap, plus discrete boosts and penalties
- **Score calculation**: Weighted sum with bonuses, penalties, and clamping to [0,1] range
- **Spatial boost**: Linear and exponential decay models for distance-based boosting
- **Decision tiers**: Four-tier system (High/Medium/Review/Reject) with margin requirements
- **Decision logic**: Winner margin, house number gates, locality validation
- **Tie-breaking**: Multi-criteria ranking when scores are close
- **Explainability**: Complete feature breakdown for every match decision
- **Penalty system**: House number conflict, descriptor mismatch, phonetic miss, status penalties
- **Calibration**: Empirical threshold tuning achieving 99.1% precision at auto-accept tier
- **Alternative models**: Architecture for logistic regression, gradient boosting, ensemble scoring

The scoring system ensures high precision on automated decisions whilst providing clear explanations for manual review cases.

---

*This chapter details scoring and decision logic. Chapter 8 describes the data pipeline and ETL processes.*
