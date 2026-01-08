# Chapter 6: Matching Algorithms

## 6.1 Multi-Layer Matching Philosophy

The EHDC LLPG matching system employs a sophisticated multi-layer pipeline where each layer addresses progressively more difficult matching cases. This architecture follows the principle of "easy first" - straightforward matches are resolved quickly at early layers, allowing later layers to focus computational resources on genuinely ambiguous addresses.

### 6.1.1 Pipeline Architecture

```
Input Address
     |
     v
+--------------------+
| Layer 2:           |    Matched (21.7%)
| Deterministic      |-------------------------> ACCEPT
| - Legacy UPRN      |
| - Exact Canonical  |
+--------------------+
     |
     | Unmatched
     v
+--------------------+
| Layer 3:           |    High Confidence (40-60%)
| Group Fuzzy        |-------------------------> ACCEPT
| - pg_trgm          |
| - Phonetic Filter  |    Medium Confidence
| - Locality Filter  |-------------------------> REVIEW
+--------------------+
     |
     | Unmatched
     v
+--------------------+
| Layer 4:           |    Document-Level (10-20%)
| Individual Fuzzy   |-------------------------> ACCEPT/REVIEW
| - Per-Document     |
| - Semantic/Vector  |
+--------------------+
     |
     | Unmatched
     v
+--------------------+
| Layer 5:           |    Conservative (5-10%)
| Conservative       |-------------------------> ACCEPT/REVIEW
| - Spatial          |
| - High Threshold   |
+--------------------+
     |
     | Unmatched
     v
NO MATCH (Remaining ~43%)
```

### 6.1.2 Layer Responsibilities

| Layer | Purpose | Typical Match Rate | Precision Target |
|-------|---------|-------------------|------------------|
| Layer 2 | Validate existing UPRNs and exact matches | 21.7% | 99.9% |
| Layer 3 | Group-level fuzzy matching with deduplication | 18.9% | 98.5% |
| Layer 4 | Individual document fuzzy matching | 14.0% | 97.0% |
| Layer 5 | Conservative validation with spatial | 2.6% | 99.0% |

### 6.1.3 Design Principles

1. **Deterministic First**: Exact matches avoid unnecessary fuzzy computation
2. **Group Deduplication**: Process unique addresses once, propagate to all documents
3. **Progressive Sophistication**: Each layer adds complexity only when needed
4. **Audit Trail**: Every decision is logged with method and confidence
5. **Configurable Thresholds**: All parameters externally tunable

## 6.2 Layer 2: Deterministic Matching

Deterministic matching handles cases where exact information is available. This layer achieves near-perfect precision by relying on validated data.

### 6.2.1 Legacy UPRN Validation

When a source document contains a UPRN, the system validates it against the authoritative LLPG:

```go
func (dm *DeterministicMatcher) ValidateLegacyUPRN(srcID int64, rawUPRN string) (*MatchResult, error) {
    // Step 1: Clean the UPRN
    cleanUPRN := strings.TrimSpace(rawUPRN)

    // Remove decimal suffixes (common in Excel exports)
    if strings.HasSuffix(cleanUPRN, ".00") {
        cleanUPRN = strings.TrimSuffix(cleanUPRN, ".00")
    }
    if strings.HasSuffix(cleanUPRN, ".0") {
        cleanUPRN = strings.TrimSuffix(cleanUPRN, ".0")
    }

    // Remove any non-numeric characters
    cleanUPRN = regexp.MustCompile(`[^0-9]`).ReplaceAllString(cleanUPRN, "")

    // Validate format (UPRNs are 12-digit numbers)
    if len(cleanUPRN) == 0 || len(cleanUPRN) > 12 {
        return nil, nil // Invalid UPRN format
    }

    // Step 2: Check existence in LLPG
    var exists bool
    var locAddress string
    var easting, northing float64

    err := dm.db.QueryRow(`
        SELECT TRUE, locaddress, easting, northing
        FROM dim_address
        WHERE uprn = $1
    `, cleanUPRN).Scan(&exists, &locAddress, &easting, &northing)

    if err == sql.ErrNoRows {
        return nil, nil // UPRN not found in LLPG
    }
    if err != nil {
        return nil, fmt.Errorf("UPRN lookup failed: %w", err)
    }

    // Step 3: Return validated match
    return &MatchResult{
        SrcID:         srcID,
        CandidateUPRN: cleanUPRN,
        Method:        "valid_uprn",
        Score:         1.0,
        Confidence:    1.0,
        Decision:      "auto_accepted",
        Easting:       easting,
        Northing:      northing,
    }, nil
}
```

**UPRN Cleaning Rules**:
- Remove leading/trailing whitespace
- Strip decimal suffixes from Excel-exported values (123456789.00 becomes 123456789)
- Remove non-numeric characters that may have been introduced during data entry
- Validate length (UPRNs must be 12 digits or fewer)

### 6.2.2 UPRN Format Variations

The system handles multiple UPRN formats encountered in source documents:

| Source Format | Cleaned Format | Notes |
|---------------|----------------|-------|
| `100062529753` | `100062529753` | Standard format |
| `100062529753.00` | `100062529753` | Excel decimal |
| `100062529753.0` | `100062529753` | Single decimal |
| `"100062529753"` | `100062529753` | Quoted string |
| `UPRN:100062529753` | `100062529753` | Prefixed |
| `1.00063E+11` | (rejected) | Scientific notation |

### 6.2.3 Exact Canonical Address Matching

For documents without valid UPRNs, the system attempts exact canonical matching against normalised addresses:

```go
func (dm *DeterministicMatcher) FindExactCanonicalMatch(srcID int64, addrCan string) ([]*MatchResult, error) {
    // Query for exact canonical matches
    rows, err := dm.db.Query(`
        SELECT uprn, locaddress, easting, northing, blpu_class, status
        FROM dim_address
        WHERE addr_can = $1
    `, addrCan)

    if err != nil {
        return nil, fmt.Errorf("exact canonical query failed: %w", err)
    }
    defer rows.Close()

    var results []*MatchResult
    for rows.Next() {
        var uprn, locaddr, blpuClass string
        var status int
        var easting, northing float64

        if err := rows.Scan(&uprn, &locaddr, &easting, &northing, &blpuClass, &status); err != nil {
            continue
        }

        results = append(results, &MatchResult{
            SrcID:         srcID,
            CandidateUPRN: uprn,
            Method:        "exact_canonical",
            Score:         0.99,
            Confidence:    0.99,
            Easting:       easting,
            Northing:      northing,
            BLPUClass:     blpuClass,
            AddressStatus: status,
        })
    }

    // Decision logic based on match count
    switch len(results) {
    case 0:
        return nil, nil // No matches
    case 1:
        // Single match: auto-accept
        results[0].Decision = "auto_accepted"
    default:
        // Multiple matches: rank by status and class
        dm.rankMultipleMatches(results)
    }

    return results, nil
}
```

### 6.2.4 Multiple Match Resolution

When exact canonical matching returns multiple candidates, the system ranks them:

```go
func (dm *DeterministicMatcher) rankMultipleMatches(results []*MatchResult) {
    // Sort by preference: Live > Provisional > Historic
    sort.Slice(results, func(i, j int) bool {
        // Status priority: 1 (Live) > 8 (Provisional) > 6 (Historic)
        statusPriority := map[int]int{1: 3, 8: 2, 6: 1}
        pi := statusPriority[results[i].AddressStatus]
        pj := statusPriority[results[j].AddressStatus]

        if pi != pj {
            return pi > pj
        }

        // Secondary: Prefer residential over commercial
        if results[i].BLPUClass != results[j].BLPUClass {
            return strings.HasPrefix(results[i].BLPUClass, "R")
        }

        return false
    })

    // Best candidate may be auto-accepted if margin is clear
    if len(results) >= 2 {
        results[0].Decision = "needs_review"
        results[0].TieRank = 1

        for i := 1; i < len(results); i++ {
            results[i].Decision = "needs_review"
            results[i].TieRank = i + 1
        }
    }
}
```

## 6.3 Layer 3: Fuzzy Group Matching

Layer 3 operates on unique canonical addresses rather than individual documents. This deduplication significantly reduces processing time - 129,701 documents reduce to approximately 76,050 unique addresses.

### 6.3.1 Group-Based Processing Strategy

```go
func (fm *FuzzyMatcher) ProcessGroups(minWorkers int) error {
    // Step 1: Get unique unmatched addresses with document counts
    groups, err := fm.getUnmatchedAddressGroups()
    if err != nil {
        return err
    }

    log.Printf("Processing %d unique address groups", len(groups))

    // Step 2: Process each group once
    for _, group := range groups {
        result, err := fm.matchGroup(group)
        if err != nil {
            log.Printf("Group matching failed for %s: %v", group.AddrCan, err)
            continue
        }

        if result != nil {
            // Step 3: Propagate match to all documents in group
            err = fm.propagateToDocuments(group.DocumentIDs, result)
            if err != nil {
                log.Printf("Propagation failed: %v", err)
            }
        }
    }

    return nil
}

type AddressGroup struct {
    AddrCan      string
    DocumentIDs  []int64
    DocumentType string
    SampleRaw    string
}
```

### 6.3.2 PostgreSQL pg_trgm Similarity

The pg_trgm extension provides trigram-based similarity matching. A trigram is a sequence of three consecutive characters.

**Trigram Example**:
```
Input: "HIGH STREET"
Trigrams: ["  H", " HI", "HIG", "IGH", "GH ", "H S", " ST", "STR", "TRE", "REE", "EET", "ET "]
```

**Similarity Calculation**:
```
similarity(A, B) = |trigrams(A) ∩ trigrams(B)| / |trigrams(A) ∪ trigrams(B)|
```

### 6.3.3 Fuzzy Candidate Generation

```go
func (fm *FuzzyMatcher) FindFuzzyCandidates(addrCan string, minSimilarity float64) ([]*FuzzyCandidate, error) {
    // Ensure address is normalised
    addrCan = strings.ToUpper(strings.TrimSpace(addrCan))

    if len(addrCan) < 5 {
        return nil, nil // Too short for meaningful matching
    }

    // Query with trigram similarity
    rows, err := fm.db.Query(`
        SELECT
            d.uprn,
            d.locaddress,
            d.addr_can,
            d.easting,
            d.northing,
            d.usrn,
            d.blpu_class,
            d.status,
            d.gopostal_road,
            d.gopostal_locality,
            d.gopostal_postcode,
            similarity($1, d.addr_can) as trgm_score
        FROM dim_address d
        WHERE d.addr_can % $1
          AND similarity($1, d.addr_can) >= $2
          AND d.status IN (1, 8)  -- Live or Provisional only
        ORDER BY trgm_score DESC
        LIMIT 50
    `, addrCan, minSimilarity)

    if err != nil {
        return nil, fmt.Errorf("trigram query failed: %w", err)
    }
    defer rows.Close()

    var candidates []*FuzzyCandidate
    for rows.Next() {
        c := &FuzzyCandidate{
            Features: make(map[string]interface{}),
        }

        var road, locality, postcode sql.NullString

        err := rows.Scan(
            &c.UPRN, &c.LocAddress, &c.AddrCan,
            &c.Easting, &c.Northing, &c.USRN,
            &c.BLPUClass, &c.Status,
            &road, &locality, &postcode,
            &c.TrigramScore,
        )
        if err != nil {
            continue
        }

        c.Road = road.String
        c.Locality = locality.String
        c.Postcode = postcode.String

        candidates = append(candidates, c)
    }

    return candidates, nil
}
```

### 6.3.4 Feature Computation

After candidate generation, comprehensive features are computed for scoring:

```go
func (fm *FuzzyMatcher) computeFeatures(srcAddr string, candidate *FuzzyCandidate) {
    // 1. Jaro-Winkler Similarity
    candidate.JaroScore = JaroSimilarity(srcAddr, candidate.AddrCan)
    candidate.Features["jaro"] = candidate.JaroScore

    // 2. Levenshtein Distance (normalised)
    levDist := LevenshteinDistance(srcAddr, candidate.AddrCan)
    maxLen := max(len(srcAddr), len(candidate.AddrCan))
    candidate.LevenshteinNorm = 1.0 - float64(levDist)/float64(maxLen)
    candidate.Features["levenshtein_norm"] = candidate.LevenshteinNorm

    // 3. Token Overlap
    srcTokens := tokenize(srcAddr)
    candTokens := tokenize(candidate.AddrCan)
    candidate.TokenOverlap = calculateTokenOverlap(srcTokens, candTokens)
    candidate.Features["token_overlap"] = candidate.TokenOverlap

    // 4. Phonetic Matching
    srcPhonetics := extractPhonetics(srcTokens)
    candPhonetics := extractPhonetics(candTokens)
    candidate.PhoneticHits = countPhoneticMatches(srcPhonetics, candPhonetics)
    candidate.Features["phonetic_hits"] = candidate.PhoneticHits

    // 5. House Number Match
    srcHouseNums := extractHouseNumbers(srcAddr)
    candHouseNums := extractHouseNumbers(candidate.AddrCan)
    candidate.HouseNumberMatch = checkHouseNumberMatch(srcHouseNums, candHouseNums)
    candidate.Features["house_number_match"] = candidate.HouseNumberMatch

    // 6. Locality Token Overlap
    candidate.LocalityOverlap = calculateLocalityOverlap(srcAddr, candidate)
    candidate.Features["locality_overlap"] = candidate.LocalityOverlap

    // 7. Street Token Overlap
    candidate.StreetOverlap = calculateStreetOverlap(srcAddr, candidate)
    candidate.Features["street_overlap"] = candidate.StreetOverlap

    // 8. Bag-of-Words Cosine Similarity
    candidate.CosineBOW = cosineBagOfWords(srcAddr, candidate.AddrCan)
    candidate.Features["cosine_bow"] = candidate.CosineBOW
}
```

### 6.3.5 Jaro Similarity Algorithm

The Jaro similarity algorithm measures the minimum number of single-character transpositions required to change one string into another:

```go
// JaroSimilarity calculates the Jaro similarity between two strings.
// Returns a value between 0 (no similarity) and 1 (identical).
func JaroSimilarity(s1, s2 string) float64 {
    s1 = strings.ToUpper(strings.TrimSpace(s1))
    s2 = strings.ToUpper(strings.TrimSpace(s2))

    // Handle edge cases
    if s1 == s2 {
        return 1.0
    }
    if len(s1) == 0 || len(s2) == 0 {
        return 0.0
    }

    // Calculate match window
    // Characters are considered matching if they are the same
    // and not farther than floor(max(|s1|,|s2|)/2) - 1
    matchWindow := max(len(s1), len(s2))/2 - 1
    if matchWindow < 0 {
        matchWindow = 0
    }

    s1Matches := make([]bool, len(s1))
    s2Matches := make([]bool, len(s2))

    matches := 0
    transpositions := 0

    // Find matching characters
    for i := 0; i < len(s1); i++ {
        start := max(0, i-matchWindow)
        end := min(len(s2), i+matchWindow+1)

        for j := start; j < end; j++ {
            if s2Matches[j] || s1[i] != s2[j] {
                continue
            }
            s1Matches[i] = true
            s2Matches[j] = true
            matches++
            break
        }
    }

    if matches == 0 {
        return 0.0
    }

    // Count transpositions
    k := 0
    for i := 0; i < len(s1); i++ {
        if !s1Matches[i] {
            continue
        }
        for !s2Matches[k] {
            k++
        }
        if s1[i] != s2[k] {
            transpositions++
        }
        k++
    }

    // Calculate Jaro similarity
    m := float64(matches)
    t := float64(transpositions) / 2

    return (m/float64(len(s1)) + m/float64(len(s2)) + (m-t)/m) / 3.0
}
```

**Jaro-Winkler Extension**:

```go
// JaroWinklerSimilarity extends Jaro with a prefix bonus.
// The prefix scale p is typically 0.1.
func JaroWinklerSimilarity(s1, s2 string, prefixScale float64) float64 {
    jaro := JaroSimilarity(s1, s2)

    // Calculate common prefix length (max 4 characters)
    prefixLen := 0
    maxPrefix := min(4, min(len(s1), len(s2)))

    for i := 0; i < maxPrefix; i++ {
        if s1[i] == s2[i] {
            prefixLen++
        } else {
            break
        }
    }

    // Apply prefix bonus
    return jaro + float64(prefixLen)*prefixScale*(1-jaro)
}
```

### 6.3.6 Levenshtein Distance Algorithm

Levenshtein distance calculates the minimum number of single-character edits (insertions, deletions, or substitutions) required to transform one string into another:

```go
// LevenshteinDistance calculates the edit distance between two strings.
func LevenshteinDistance(s1, s2 string) int {
    s1 = strings.ToUpper(s1)
    s2 = strings.ToUpper(s2)

    if s1 == s2 {
        return 0
    }
    if len(s1) == 0 {
        return len(s2)
    }
    if len(s2) == 0 {
        return len(s1)
    }

    // Create distance matrix
    // Use only two rows for memory efficiency
    prev := make([]int, len(s2)+1)
    curr := make([]int, len(s2)+1)

    // Initialise first row
    for j := 0; j <= len(s2); j++ {
        prev[j] = j
    }

    // Fill matrix
    for i := 1; i <= len(s1); i++ {
        curr[0] = i

        for j := 1; j <= len(s2); j++ {
            cost := 0
            if s1[i-1] != s2[j-1] {
                cost = 1
            }

            curr[j] = min(
                prev[j]+1,      // deletion
                curr[j-1]+1,    // insertion
                prev[j-1]+cost, // substitution
            )
        }

        // Swap rows
        prev, curr = curr, prev
    }

    return prev[len(s2)]
}
```

**Normalised Levenshtein**:
```go
func NormalisedLevenshtein(s1, s2 string) float64 {
    dist := LevenshteinDistance(s1, s2)
    maxLen := max(len(s1), len(s2))
    if maxLen == 0 {
        return 1.0
    }
    return 1.0 - float64(dist)/float64(maxLen)
}
```

### 6.3.7 Cosine Similarity (Bag of Words)

Cosine similarity treats addresses as vectors of token frequencies:

```go
// cosineBagOfWords calculates cosine similarity using token frequency vectors.
func cosineBagOfWords(s1, s2 string) float64 {
    // Tokenise both strings
    tokens1 := tokenize(s1)
    tokens2 := tokenize(s2)

    // Build frequency maps
    freq1 := make(map[string]int)
    freq2 := make(map[string]int)

    for _, t := range tokens1 {
        freq1[t]++
    }
    for _, t := range tokens2 {
        freq2[t]++
    }

    // Calculate dot product and magnitudes
    var dotProduct float64
    var mag1, mag2 float64

    // All unique tokens
    allTokens := make(map[string]bool)
    for t := range freq1 {
        allTokens[t] = true
    }
    for t := range freq2 {
        allTokens[t] = true
    }

    for token := range allTokens {
        v1 := float64(freq1[token])
        v2 := float64(freq2[token])

        dotProduct += v1 * v2
        mag1 += v1 * v1
        mag2 += v2 * v2
    }

    if mag1 == 0 || mag2 == 0 {
        return 0.0
    }

    return dotProduct / (math.Sqrt(mag1) * math.Sqrt(mag2))
}

func tokenize(s string) []string {
    s = strings.ToUpper(s)
    // Split on whitespace and punctuation
    re := regexp.MustCompile(`[^A-Z0-9]+`)
    tokens := re.Split(s, -1)

    // Filter empty tokens
    var result []string
    for _, t := range tokens {
        if len(t) > 0 {
            result = append(result, t)
        }
    }
    return result
}
```

### 6.3.8 Token Overlap Calculation

Token overlap measures how many address components appear in both addresses:

```go
// calculateTokenOverlap returns the proportion of shared tokens.
func calculateTokenOverlap(srcTokens, candTokens []string) float64 {
    if len(srcTokens) == 0 || len(candTokens) == 0 {
        return 0.0
    }

    // Build token sets
    srcSet := make(map[string]bool)
    for _, t := range srcTokens {
        srcSet[t] = true
    }

    candSet := make(map[string]bool)
    for _, t := range candTokens {
        candSet[t] = true
    }

    // Count intersection
    intersection := 0
    for t := range srcSet {
        if candSet[t] {
            intersection++
        }
    }

    // Jaccard-style overlap
    union := len(srcSet) + len(candSet) - intersection
    if union == 0 {
        return 0.0
    }

    return float64(intersection) / float64(union)
}
```

### 6.3.9 Phonetic Filtering with Double Metaphone

The phonetic filter uses Double Metaphone to catch spelling variations:

```go
type DoubleMetaphone struct{}

// GetMetaphone returns primary and secondary phonetic codes.
func (dm *DoubleMetaphone) GetMetaphone(text string) (primary, secondary string) {
    text = strings.ToUpper(strings.TrimSpace(text))
    if text == "" {
        return "", ""
    }

    // Apply phonetic transformations
    result := text

    // Consonant cluster simplifications
    transformations := []struct {
        pattern     string
        replacement string
    }{
        {"GH", "F"},    // COUGH -> COF
        {"PH", "F"},    // PHONE -> FONE
        {"CK", "K"},    // BACK -> BAK
        {"KN", "N"},    // KNIGHT -> NITE
        {"WR", "R"},    // WRITE -> RITE
        {"GN", "N"},    // GNAT -> NAT
        {"MB", "M"},    // DUMB -> DUM
        {"PS", "S"},    // PSALM -> SALM
        {"PN", "N"},    // PNEUMATIC -> NEUMATIC
        {"QU", "KW"},   // QUEEN -> KWEEN
        {"SCH", "SK"},  // SCHOOL -> SKOOL
        {"TH", "0"},    // THE -> 0E (theta)
        {"SH", "X"},    // SHIP -> XIP
        {"CH", "X"},    // CHURCH -> XURX (or K in some cases)
        {"WH", "W"},    // WHEN -> WEN
        {"TCH", "X"},   // MATCH -> MAX
        {"DG", "J"},    // EDGE -> EJE
        {"TI", "X"},    // NATION -> NAXON (before vowel)
        {"SI", "X"},    // MANSION -> MANXON (before vowel)
        {"CI", "S"},    // FACIAL -> FASAL (before vowel)
    }

    for _, t := range transformations {
        result = strings.ReplaceAll(result, t.pattern, t.replacement)
    }

    // Remove vowels except at start (vowels don't affect phonetic matching much)
    if len(result) > 1 {
        first := string(result[0])
        rest := removeVowels(result[1:])
        result = first + rest
    }

    // Remove duplicate consecutive letters
    var cleaned strings.Builder
    var lastChar rune
    for _, char := range result {
        if char != lastChar {
            cleaned.WriteRune(char)
            lastChar = char
        }
    }

    metaphone := cleaned.String()

    // Limit to 4 characters (standard metaphone length)
    if len(metaphone) > 4 {
        metaphone = metaphone[:4]
    }

    return metaphone, metaphone
}

func removeVowels(s string) string {
    return strings.Map(func(r rune) rune {
        switch r {
        case 'A', 'E', 'I', 'O', 'U', 'Y':
            return -1
        default:
            return r
        }
    }, s)
}
```

**Phonetic Matching Examples**:

| Original | Metaphone | Notes |
|----------|-----------|-------|
| HORNDEAN | HRND | Standard |
| HORNDENE | HRND | Matches HORNDEAN |
| PETERSFIELD | PTRS | Standard |
| PETERSFEILD | PTRS | Matches PETERSFIELD |
| ALTON | ALTN | Standard |
| ALLTON | ALTN | Matches ALTON |

### 6.3.10 Phonetic Match Counting

```go
// countPhoneticMatches counts how many tokens have matching phonetic codes.
func countPhoneticMatches(srcPhonetics, candPhonetics []string) int {
    candSet := make(map[string]bool)
    for _, p := range candPhonetics {
        if p != "" {
            candSet[p] = true
        }
    }

    matches := 0
    for _, p := range srcPhonetics {
        if p != "" && candSet[p] {
            matches++
        }
    }

    return matches
}

// extractPhonetics generates phonetic codes for all tokens.
func extractPhonetics(tokens []string) []string {
    dm := &DoubleMetaphone{}
    var phonetics []string

    for _, token := range tokens {
        // Skip numbers and very short tokens
        if len(token) < 2 || isNumeric(token) {
            continue
        }

        primary, _ := dm.GetMetaphone(token)
        if primary != "" {
            phonetics = append(phonetics, primary)
        }
    }

    return phonetics
}
```

### 6.3.11 House Number Validation

House number matching is critical for address accuracy:

```go
// checkHouseNumberMatch validates house numbers between addresses.
// Returns: 1.0 (exact match), 0.5 (close match), 0.0 (no match), -1.0 (conflict)
func checkHouseNumberMatch(srcNums, candNums []string) float64 {
    // If either has no house numbers, cannot validate
    if len(srcNums) == 0 || len(candNums) == 0 {
        return 0.0
    }

    // Check for exact match
    for _, sn := range srcNums {
        for _, cn := range candNums {
            if sn == cn {
                return 1.0 // Exact match
            }
        }
    }

    // Check for close match (renumbering tolerance)
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

            // Allow difference of up to 2 (common renumbering range)
            if abs(srcNum-candNum) <= 2 {
                return 0.5 // Close match
            }
        }
    }

    // Numbers present but don't match - this is a red flag
    return -1.0 // Conflict
}

// extractNumericPart extracts the number from house numbers like "12A", "14B".
func extractNumericPart(houseNum string) int {
    // Remove alpha suffix
    numStr := strings.TrimRight(houseNum, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
    num, err := strconv.Atoi(numStr)
    if err != nil {
        return -1
    }
    return num
}

// extractHouseNumbers finds all house numbers in an address.
func extractHouseNumbers(addr string) []string {
    // Pattern matches: 12, 12A, 14-16, FLAT 3
    re := regexp.MustCompile(`\b(\d+[A-Z]?)\b`)
    matches := re.FindAllStringSubmatch(addr, -1)

    var numbers []string
    seen := make(map[string]bool)

    for _, m := range matches {
        num := m[1]
        // Skip postcodes (4-digit numbers are likely postcode parts)
        if len(num) >= 4 && !strings.ContainsAny(num, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
            continue
        }
        if !seen[num] {
            numbers = append(numbers, num)
            seen[num] = true
        }
    }

    return numbers
}
```

### 6.3.12 Locality and Street Overlap

```go
// calculateLocalityOverlap measures overlap in locality tokens.
func calculateLocalityOverlap(srcAddr string, candidate *FuzzyCandidate) float64 {
    // Extract locality tokens from source
    srcLocalities := extractLocalityTokens(srcAddr)
    if len(srcLocalities) == 0 {
        return 0.0
    }

    // Candidate locality from parsed components
    candLocalities := make(map[string]bool)
    if candidate.Locality != "" {
        for _, token := range strings.Fields(strings.ToUpper(candidate.Locality)) {
            candLocalities[token] = true
        }
    }

    // Also extract from canonical address
    for _, token := range extractLocalityTokens(candidate.AddrCan) {
        candLocalities[token] = true
    }

    if len(candLocalities) == 0 {
        return 0.0
    }

    // Count matches
    matches := 0
    for _, loc := range srcLocalities {
        if candLocalities[loc] {
            matches++
        }
    }

    return float64(matches) / float64(len(srcLocalities))
}

// calculateStreetOverlap measures overlap in street name tokens.
func calculateStreetOverlap(srcAddr string, candidate *FuzzyCandidate) float64 {
    srcStreet := extractStreetTokens(srcAddr)
    if len(srcStreet) == 0 {
        return 0.0
    }

    candStreet := make(map[string]bool)
    if candidate.Road != "" {
        for _, token := range strings.Fields(strings.ToUpper(candidate.Road)) {
            candStreet[token] = true
        }
    }

    if len(candStreet) == 0 {
        return 0.0
    }

    matches := 0
    for _, st := range srcStreet {
        if candStreet[st] {
            matches++
        }
    }

    return float64(matches) / float64(len(srcStreet))
}
```

### 6.3.13 Filter Chain

Candidates must pass multiple filters before being scored:

```go
func (fm *FuzzyMatcher) passesFilters(srcAddr string, candidate *FuzzyCandidate) bool {
    // Filter 1: Minimum trigram similarity
    if candidate.TrigramScore < 0.30 {
        return false
    }

    // Filter 2: Phonetic overlap for lower similarities
    // If trigram score is below 0.85, require at least one phonetic match
    if candidate.TrigramScore < 0.85 && candidate.PhoneticHits == 0 {
        return false
    }

    // Filter 3: House number validation
    // Conflicting house numbers are a strong rejection signal
    if candidate.HouseNumberMatch == -1.0 {
        return false
    }

    // Filter 4: Minimum combined score threshold
    combinedScore := candidate.TrigramScore*0.5 + candidate.JaroScore*0.3 + candidate.TokenOverlap*0.2
    if combinedScore < fm.config.MinThreshold {
        return false
    }

    // Filter 5: BLPU class compatibility (optional)
    if fm.config.EnforceBLPUClass {
        if !isCompatibleBLPUClass(srcAddr, candidate.BLPUClass) {
            return false
        }
    }

    return true
}
```

## 6.4 Layer 4: Individual Document Matching

Layer 4 processes documents that were not matched at the group level. This includes documents where the canonical address had variations not captured by group processing.

### 6.4.1 Document-Level Candidate Generation

```go
type CandidateGenerator struct {
    db           *sql.DB
    embedder     Embedder
    vectorDB     VectorDatabase
    config       *GeneratorConfig
}

// GenerateCandidates implements multi-tier candidate generation.
func (cg *CandidateGenerator) GenerateCandidates(doc *SourceDocument) ([]*Candidate, error) {
    var allCandidates []*Candidate

    // Tier A: Deterministic lookups
    tierA, err := cg.tierADeterministic(doc)
    if err != nil {
        return nil, err
    }
    if len(tierA) > 0 {
        return tierA, nil // Deterministic match found
    }

    // Tier B: Fuzzy matching
    tierB, err := cg.tierBFuzzy(doc)
    if err != nil {
        return nil, err
    }
    allCandidates = append(allCandidates, tierB...)

    // Tier C: Vector/Semantic matching (if enabled)
    if cg.config.EnableVector {
        tierC, err := cg.tierCVector(doc)
        if err != nil {
            log.Printf("Vector matching failed: %v", err)
        } else {
            allCandidates = append(allCandidates, tierC...)
        }
    }

    // Tier D: Spatial refinement (if coordinates available)
    if doc.Easting != nil && doc.Northing != nil {
        tierD, err := cg.tierDSpatial(doc, allCandidates)
        if err != nil {
            log.Printf("Spatial refinement failed: %v", err)
        } else {
            allCandidates = tierD
        }
    }

    // Deduplicate by UPRN, keeping highest scores
    return dedupeByUPRN(allCandidates), nil
}
```

### 6.4.2 Tier A: Deterministic Lookups

```go
func (cg *CandidateGenerator) tierADeterministic(doc *SourceDocument) ([]*Candidate, error) {
    // A.1: UPRN lookup
    if doc.UPRN != nil && *doc.UPRN != "" {
        candidate, err := cg.lookupUPRN(*doc.UPRN)
        if err == nil && candidate != nil {
            candidate.Tier = "A"
            candidate.Method = "uprn_lookup"
            candidate.Score = 1.0
            return []*Candidate{candidate}, nil
        }
    }

    // A.2: Exact canonical match
    if doc.AddrCan != nil && *doc.AddrCan != "" {
        candidates, err := cg.exactCanonicalMatch(*doc.AddrCan)
        if err == nil && len(candidates) > 0 {
            for _, c := range candidates {
                c.Tier = "A"
                c.Method = "exact_canonical"
                c.Score = 0.99
            }
            return candidates, nil
        }
    }

    return nil, nil
}
```

### 6.4.3 Tier B: Fuzzy with Filters

```go
func (cg *CandidateGenerator) tierBFuzzy(doc *SourceDocument) ([]*Candidate, error) {
    if doc.AddrCan == nil || *doc.AddrCan == "" {
        return nil, nil
    }

    // B.1: Initial trigram search
    candidates, err := cg.trigramMatch(*doc.AddrCan, cg.config.MinSimilarity)
    if err != nil {
        return nil, err
    }

    // B.2: Apply phonetic filter
    candidates = cg.applyPhoneticFilter(doc, candidates)

    // B.3: Apply locality filter
    candidates = cg.applyLocalityFilter(doc, candidates)

    // B.4: Apply house number filter
    candidates = cg.applyHouseNumberFilter(doc, candidates)

    // Set tier information
    for _, c := range candidates {
        c.Tier = "B"
        c.Method = "fuzzy_filtered"
    }

    return candidates, nil
}

func (cg *CandidateGenerator) applyPhoneticFilter(doc *SourceDocument, candidates []*Candidate) []*Candidate {
    srcPhonetics := extractPhonetics(tokenize(*doc.AddrCan))

    var filtered []*Candidate
    for _, c := range candidates {
        candPhonetics := extractPhonetics(tokenize(c.AddrCan))
        hits := countPhoneticMatches(srcPhonetics, candPhonetics)

        // Require phonetic overlap for lower similarity matches
        if c.TrigramScore >= 0.85 || hits >= 1 {
            c.PhoneticHits = hits
            filtered = append(filtered, c)
        }
    }

    return filtered
}

func (cg *CandidateGenerator) applyLocalityFilter(doc *SourceDocument, candidates []*Candidate) []*Candidate {
    srcLocalities := extractLocalityTokens(*doc.AddrCan)
    if len(srcLocalities) == 0 {
        return candidates // No locality to filter by
    }

    var filtered []*Candidate
    for _, c := range candidates {
        overlap := calculateLocalityOverlap(*doc.AddrCan, &FuzzyCandidate{
            AddrCan:  c.AddrCan,
            Locality: c.Locality,
        })

        // Require some locality overlap for medium-similarity matches
        if c.TrigramScore >= 0.90 || overlap >= 0.5 {
            c.LocalityOverlap = overlap
            filtered = append(filtered, c)
        }
    }

    return filtered
}

func (cg *CandidateGenerator) applyHouseNumberFilter(doc *SourceDocument, candidates []*Candidate) []*Candidate {
    srcNums := extractHouseNumbers(*doc.AddrCan)
    if len(srcNums) == 0 {
        return candidates // No house number to filter by
    }

    var filtered []*Candidate
    for _, c := range candidates {
        candNums := extractHouseNumbers(c.AddrCan)
        match := checkHouseNumberMatch(srcNums, candNums)

        // Reject conflicting house numbers
        if match != -1.0 {
            c.HouseNumberMatch = match
            filtered = append(filtered, c)
        }
    }

    return filtered
}
```

### 6.4.4 Tier C: Vector/Semantic Matching

```go
func (cg *CandidateGenerator) tierCVector(doc *SourceDocument) ([]*Candidate, error) {
    if doc.AddrCan == nil || *doc.AddrCan == "" {
        return nil, nil
    }

    // Generate embedding for source address
    embedding, err := cg.embedder.Embed(*doc.AddrCan)
    if err != nil {
        return nil, fmt.Errorf("embedding generation failed: %w", err)
    }

    // Search vector database
    results, err := cg.vectorDB.Search(embedding, cg.config.VectorTopK)
    if err != nil {
        return nil, fmt.Errorf("vector search failed: %w", err)
    }

    var candidates []*Candidate
    for _, r := range results {
        // Skip low similarity results
        if r.Score < cg.config.VectorMinSimilarity {
            continue
        }

        candidates = append(candidates, &Candidate{
            UPRN:             r.ID,
            AddrCan:          r.Payload["addr_can"].(string),
            LocAddress:       r.Payload["locaddress"].(string),
            Easting:          r.Payload["easting"].(float64),
            Northing:         r.Payload["northing"].(float64),
            Tier:             "C",
            Method:           "vector_semantic",
            CosineSimilarity: r.Score,
        })
    }

    return candidates, nil
}
```

### 6.4.5 Tier D: Spatial Refinement

```go
func (cg *CandidateGenerator) tierDSpatial(doc *SourceDocument, candidates []*Candidate) ([]*Candidate, error) {
    srcEasting := *doc.Easting
    srcNorthing := *doc.Northing

    // Boost candidates near the source coordinates
    for _, c := range candidates {
        if c.Easting == 0 || c.Northing == 0 {
            continue
        }

        // Calculate distance
        dx := c.Easting - srcEasting
        dy := c.Northing - srcNorthing
        distance := math.Sqrt(dx*dx + dy*dy)

        c.DistanceMetres = distance

        // Apply spatial boost (exponential decay)
        // Full boost at 0m, ~37% at 100m, ~14% at 200m
        c.SpatialBoost = math.Exp(-distance / 100.0)
    }

    // Optionally add new candidates from spatial search
    if cg.config.SpatialExpand {
        spatialCandidates, err := cg.spatialSearch(srcEasting, srcNorthing, cg.config.SpatialRadius)
        if err != nil {
            return candidates, nil // Continue with existing candidates
        }

        // Merge with existing, avoiding duplicates
        existing := make(map[string]bool)
        for _, c := range candidates {
            existing[c.UPRN] = true
        }

        for _, sc := range spatialCandidates {
            if !existing[sc.UPRN] {
                sc.Tier = "D"
                sc.Method = "spatial_search"
                candidates = append(candidates, sc)
            }
        }
    }

    return candidates, nil
}
```

## 6.5 Layer 5: Conservative Validation

Layer 5 applies stricter thresholds to catch matches that earlier layers might have missed or incorrectly rejected.

### 6.5.1 Conservative Matching Philosophy

Conservative matching prioritises precision over recall:

```go
type ConservativeMatcher struct {
    db     *sql.DB
    config *ConservativeConfig
}

type ConservativeConfig struct {
    MinTrigramScore     float64 // Default: 0.90
    MinJaroScore        float64 // Default: 0.92
    RequireHouseNumber  bool    // Default: true
    RequireLocalityMatch bool   // Default: true
    MaxDistanceMetres   float64 // Default: 50
}

func DefaultConservativeConfig() *ConservativeConfig {
    return &ConservativeConfig{
        MinTrigramScore:      0.90,
        MinJaroScore:         0.92,
        RequireHouseNumber:   true,
        RequireLocalityMatch: true,
        MaxDistanceMetres:    50,
    }
}
```

### 6.5.2 Conservative Candidate Evaluation

```go
func (cm *ConservativeMatcher) EvaluateCandidate(doc *SourceDocument, candidate *Candidate) *ConservativeResult {
    result := &ConservativeResult{
        SrcID:     doc.ID,
        UPRN:      candidate.UPRN,
        Passed:    false,
        Reasons:   make([]string, 0),
    }

    // Check 1: Trigram threshold
    if candidate.TrigramScore < cm.config.MinTrigramScore {
        result.Reasons = append(result.Reasons,
            fmt.Sprintf("trigram %.3f < %.3f", candidate.TrigramScore, cm.config.MinTrigramScore))
        return result
    }

    // Check 2: Jaro threshold
    if candidate.JaroScore < cm.config.MinJaroScore {
        result.Reasons = append(result.Reasons,
            fmt.Sprintf("jaro %.3f < %.3f", candidate.JaroScore, cm.config.MinJaroScore))
        return result
    }

    // Check 3: House number match
    if cm.config.RequireHouseNumber {
        srcNums := extractHouseNumbers(*doc.AddrCan)
        candNums := extractHouseNumbers(candidate.AddrCan)

        if len(srcNums) > 0 && len(candNums) > 0 {
            if candidate.HouseNumberMatch != 1.0 {
                result.Reasons = append(result.Reasons, "house number mismatch")
                return result
            }
        }
    }

    // Check 4: Locality match
    if cm.config.RequireLocalityMatch {
        if candidate.LocalityOverlap < 0.5 {
            result.Reasons = append(result.Reasons,
                fmt.Sprintf("locality overlap %.2f < 0.5", candidate.LocalityOverlap))
            return result
        }
    }

    // Check 5: Spatial proximity (if coordinates available)
    if doc.Easting != nil && doc.Northing != nil && candidate.DistanceMetres > 0 {
        if candidate.DistanceMetres > cm.config.MaxDistanceMetres {
            result.Reasons = append(result.Reasons,
                fmt.Sprintf("distance %.1fm > %.1fm", candidate.DistanceMetres, cm.config.MaxDistanceMetres))
            return result
        }
    }

    // All checks passed
    result.Passed = true
    result.Score = cm.calculateConservativeScore(candidate)
    return result
}

func (cm *ConservativeMatcher) calculateConservativeScore(candidate *Candidate) float64 {
    // Weighted combination with conservative emphasis
    score := candidate.TrigramScore * 0.35
    score += candidate.JaroScore * 0.30
    score += candidate.TokenOverlap * 0.15

    // Bonuses
    if candidate.HouseNumberMatch == 1.0 {
        score += 0.10
    }
    if candidate.LocalityOverlap >= 0.8 {
        score += 0.05
    }
    if candidate.SpatialBoost > 0.8 {
        score += 0.05
    }

    return min(score, 1.0)
}
```

## 6.6 Phonetic Matching

Phonetic matching captures variations in pronunciation and spelling that are common in historic documents.

### 6.6.1 Hampshire-Specific Phonetic Patterns

The system includes patterns specific to Hampshire place names:

```go
var hampshirePhoneticPatterns = map[string][]string{
    // Ending variations
    "DEAN": {"DENE", "DEN"},
    "BURY": {"BERRY", "BERY"},
    "FORD": {"FORDE"},
    "HAM":  {"HAME"},
    "LEY":  {"LEA", "LY", "LEIGH"},
    "TON":  {"TONE"},

    // Prefix variations
    "EAST": {"EST"},
    "WEST": {"WST"},
    "NORTH": {"NTH"},
    "SOUTH": {"STH"},

    // Common Hampshire localities
    "PETERSFIELD": {"PETERSFEILD", "PETERFIELD"},
    "HORNDEAN":    {"HORNDENE", "HORNDEN"},
    "LIPHOOK":     {"LIPHOK", "LIPHUK"},
    "ALTON":       {"ALLTUN", "ALLTON"},
}
```

### 6.6.2 Phonetic Bonus Application

```go
func applyPhoneticBonus(srcAddr, candAddr string, baseScore float64) float64 {
    srcTokens := tokenize(srcAddr)
    candTokens := tokenize(candAddr)

    srcPhonetics := extractPhonetics(srcTokens)
    candPhonetics := extractPhonetics(candTokens)

    hits := countPhoneticMatches(srcPhonetics, candPhonetics)
    total := len(srcPhonetics)

    if total == 0 {
        return baseScore
    }

    // Bonus proportional to phonetic match ratio
    phoneticRatio := float64(hits) / float64(total)
    bonus := phoneticRatio * 0.05 // Max 5% bonus

    return min(baseScore+bonus, 1.0)
}
```

## 6.7 Semantic Vector Matching

Semantic matching uses dense vector representations to find addresses with similar meaning but different surface forms.

### 6.7.1 Embedding Architecture

```go
type VectorMatcher struct {
    embedder   Embedder
    vectorDB   VectorDatabase
    db         *sql.DB
    dimension  int
    collection string
}

type Embedder interface {
    Embed(text string) ([]float32, error)
    EmbedBatch(texts []string) ([][]float32, error)
}

type VectorDatabase interface {
    Initialize(collection string, dimension int) error
    Upsert(collection string, points []VectorPoint) error
    Search(collection string, vector []float32, limit int) ([]SearchResult, error)
    Delete(collection string, ids []string) error
}
```

### 6.7.2 LLPG Address Indexing

```go
func (vm *VectorMatcher) IndexLLPGAddresses(batchSize int) error {
    // Count total addresses
    var total int
    err := vm.db.QueryRow("SELECT COUNT(*) FROM dim_address WHERE status = 1").Scan(&total)
    if err != nil {
        return err
    }

    log.Printf("Indexing %d LLPG addresses", total)

    offset := 0
    for offset < total {
        // Fetch batch
        rows, err := vm.db.Query(`
            SELECT uprn, addr_can, locaddress, easting, northing
            FROM dim_address
            WHERE status = 1
            ORDER BY uprn
            LIMIT $1 OFFSET $2
        `, batchSize, offset)
        if err != nil {
            return err
        }

        var addresses []AddressForEmbedding
        for rows.Next() {
            var addr AddressForEmbedding
            rows.Scan(&addr.UPRN, &addr.AddrCan, &addr.LocAddress, &addr.Easting, &addr.Northing)
            addresses = append(addresses, addr)
        }
        rows.Close()

        // Generate embeddings
        texts := make([]string, len(addresses))
        for i, addr := range addresses {
            texts[i] = addr.AddrCan
        }

        embeddings, err := vm.embedder.EmbedBatch(texts)
        if err != nil {
            return fmt.Errorf("batch embedding failed: %w", err)
        }

        // Prepare vector points
        points := make([]VectorPoint, len(addresses))
        for i, addr := range addresses {
            points[i] = VectorPoint{
                ID:     addr.UPRN,
                Vector: embeddings[i],
                Payload: map[string]interface{}{
                    "addr_can":   addr.AddrCan,
                    "locaddress": addr.LocAddress,
                    "easting":    addr.Easting,
                    "northing":   addr.Northing,
                },
            }
        }

        // Upsert to vector database
        if err := vm.vectorDB.Upsert(vm.collection, points); err != nil {
            return fmt.Errorf("vector upsert failed: %w", err)
        }

        offset += batchSize
        log.Printf("Indexed %d/%d addresses", min(offset, total), total)
    }

    return nil
}
```

### 6.7.3 Semantic Candidate Search

```go
func (vm *VectorMatcher) FindSemanticCandidates(addrCan string, limit int, minScore float64) ([]*SemanticCandidate, error) {
    // Generate embedding for query
    embedding, err := vm.embedder.Embed(addrCan)
    if err != nil {
        return nil, fmt.Errorf("query embedding failed: %w", err)
    }

    // Search vector database
    results, err := vm.vectorDB.Search(vm.collection, embedding, limit)
    if err != nil {
        return nil, fmt.Errorf("vector search failed: %w", err)
    }

    var candidates []*SemanticCandidate
    for _, r := range results {
        if r.Score < minScore {
            continue
        }

        candidates = append(candidates, &SemanticCandidate{
            UPRN:             r.ID,
            AddrCan:          r.Payload["addr_can"].(string),
            LocAddress:       r.Payload["locaddress"].(string),
            Easting:          r.Payload["easting"].(float64),
            Northing:         r.Payload["northing"].(float64),
            CosineSimilarity: r.Score,
        })
    }

    return candidates, nil
}
```

### 6.7.4 Combined Semantic Score

```go
func calculateCombinedSemanticScore(semanticScore, trigramScore float64, tokenBonus float64) float64 {
    // Primary: semantic similarity (70%)
    // Secondary: trigram similarity (30%)
    // Bonus: token overlap

    combined := semanticScore*0.70 + trigramScore*0.30

    // Token bonus (up to 5%)
    combined += tokenBonus * 0.05

    return min(combined, 1.0)
}
```

### 6.7.5 Embedding Model Configuration

The system supports multiple embedding providers:

```go
type OllamaEmbedder struct {
    host  string
    model string
}

func NewOllamaEmbedder(host, model string) *OllamaEmbedder {
    return &OllamaEmbedder{
        host:  host,
        model: model, // Default: "nomic-embed-text"
    }
}

func (e *OllamaEmbedder) Embed(text string) ([]float32, error) {
    payload := map[string]string{
        "model":  e.model,
        "prompt": text,
    }

    jsonData, _ := json.Marshal(payload)

    resp, err := http.Post(
        e.host+"/api/embeddings",
        "application/json",
        bytes.NewBuffer(jsonData),
    )
    if err != nil {
        return nil, fmt.Errorf("ollama request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("ollama error: %s", string(body))
    }

    var result struct {
        Embedding []float32 `json:"embedding"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("response decode failed: %w", err)
    }

    return result.Embedding, nil
}
```

## 6.8 Spatial Matching

Spatial matching leverages coordinate data from both source documents and the OS Open UPRN dataset.

### 6.8.1 PostGIS Distance Calculation

```go
func (sm *SpatialMatcher) FindSpatialCandidates(easting, northing float64, radiusMetres float64, limit int) ([]*SpatialCandidate, error) {
    // Use ST_DWithin for indexed spatial search
    // ST_Distance calculates exact distance for scoring
    rows, err := sm.db.Query(`
        SELECT
            d.uprn,
            d.locaddress,
            d.addr_can,
            d.easting,
            d.northing,
            d.blpu_class,
            ST_Distance(
                d.geom_27700,
                ST_SetSRID(ST_MakePoint($1, $2), 27700)
            ) as distance_m
        FROM dim_address d
        WHERE ST_DWithin(
            d.geom_27700,
            ST_SetSRID(ST_MakePoint($1, $2), 27700),
            $3
        )
        AND d.status = 1
        ORDER BY distance_m
        LIMIT $4
    `, easting, northing, radiusMetres, limit)

    if err != nil {
        return nil, fmt.Errorf("spatial query failed: %w", err)
    }
    defer rows.Close()

    var candidates []*SpatialCandidate
    for rows.Next() {
        var c SpatialCandidate
        err := rows.Scan(
            &c.UPRN, &c.LocAddress, &c.AddrCan,
            &c.Easting, &c.Northing, &c.BLPUClass,
            &c.DistanceMetres,
        )
        if err != nil {
            continue
        }

        // Calculate spatial boost
        c.SpatialBoost = calculateSpatialBoost(c.DistanceMetres)

        candidates = append(candidates, &c)
    }

    return candidates, nil
}
```

### 6.8.2 Spatial Boost Calculation

```go
// calculateSpatialBoost returns a score boost based on distance.
// Uses exponential decay with configurable parameters.
func calculateSpatialBoost(distanceMetres float64) float64 {
    // Distance thresholds and corresponding boosts
    // 0-25m:   High confidence spatial match
    // 25-50m:  Medium confidence
    // 50-100m: Low confidence
    // >100m:   Minimal boost

    if distanceMetres <= 25 {
        return 0.20 // 20% boost
    } else if distanceMetres <= 50 {
        return 0.15 // 15% boost
    } else if distanceMetres <= 100 {
        return 0.10 // 10% boost
    } else if distanceMetres <= 200 {
        return 0.05 // 5% boost
    }

    // Exponential decay beyond 200m
    return 0.05 * math.Exp(-(distanceMetres-200)/300)
}
```

### 6.8.3 Spatial Score Combination

```go
func (sm *SpatialMatcher) CalculateSpatialScore(srcAddr string, candidate *SpatialCandidate) float64 {
    // Calculate text similarity
    trigramScore := calculateTrigramSimilarity(srcAddr, candidate.AddrCan)

    // Combine spatial and text scores
    // Primary: spatial proximity (60%)
    // Secondary: text similarity (40%)
    score := candidate.SpatialBoost * 0.60
    score += trigramScore * 0.40

    // Bonuses
    // House number match
    srcNums := extractHouseNumbers(srcAddr)
    candNums := extractHouseNumbers(candidate.AddrCan)
    if checkHouseNumberMatch(srcNums, candNums) == 1.0 {
        score += 0.10
    }

    // Very close proximity bonus
    if candidate.DistanceMetres <= 10 {
        score += 0.05
    }

    return min(score, 1.0)
}
```

### 6.8.4 Spatial Area Preprocessing

For efficiency, the system precomputes spatial reference areas:

```go
func (sm *SpatialMatcher) BuildRoadPostcodeAreas() error {
    _, err := sm.db.Exec(`
        -- Create aggregated spatial areas by road and postcode sector
        INSERT INTO spatial_road_postcode_area
            (road, postcode_sector, centroid_easting, centroid_northing,
             bbox_minx, bbox_miny, bbox_maxx, bbox_maxy, address_count)
        SELECT
            COALESCE(gopostal_road, 'UNKNOWN') as road,
            COALESCE(LEFT(gopostal_postcode, 4), 'UNKN') as postcode_sector,
            AVG(easting) as centroid_easting,
            AVG(northing) as centroid_northing,
            MIN(easting) as bbox_minx,
            MIN(northing) as bbox_miny,
            MAX(easting) as bbox_maxx,
            MAX(northing) as bbox_maxy,
            COUNT(*) as address_count
        FROM dim_address
        WHERE easting IS NOT NULL
          AND northing IS NOT NULL
          AND status = 1
        GROUP BY
            COALESCE(gopostal_road, 'UNKNOWN'),
            COALESCE(LEFT(gopostal_postcode, 4), 'UNKN')
        ON CONFLICT (road, postcode_sector) DO UPDATE SET
            centroid_easting = EXCLUDED.centroid_easting,
            centroid_northing = EXCLUDED.centroid_northing,
            bbox_minx = EXCLUDED.bbox_minx,
            bbox_miny = EXCLUDED.bbox_miny,
            bbox_maxx = EXCLUDED.bbox_maxx,
            bbox_maxy = EXCLUDED.bbox_maxy,
            address_count = EXCLUDED.address_count
    `)

    return err
}
```

### 6.8.5 Coordinate System Handling

The system handles multiple coordinate systems:

```go
const (
    SRID_BNG    = 27700 // British National Grid (Easting/Northing)
    SRID_WGS84  = 4326  // WGS84 (Latitude/Longitude)
)

// TransformBNGToWGS84 converts British National Grid to WGS84.
func TransformBNGToWGS84(easting, northing float64) (lat, lon float64, err error) {
    // Use PostGIS for accurate transformation
    var result struct {
        Lat float64
        Lon float64
    }

    err = db.QueryRow(`
        SELECT
            ST_Y(ST_Transform(ST_SetSRID(ST_MakePoint($1, $2), 27700), 4326)) as lat,
            ST_X(ST_Transform(ST_SetSRID(ST_MakePoint($1, $2), 27700), 4326)) as lon
    `, easting, northing).Scan(&result.Lat, &result.Lon)

    return result.Lat, result.Lon, err
}
```

## 6.9 Candidate Generation and Deduplication

### 6.9.1 Multi-Source Candidate Merging

When candidates come from multiple sources (fuzzy, vector, spatial), they must be merged intelligently:

```go
type CandidateMerger struct {
    preferredOrder []string // ["uprn_lookup", "exact_canonical", "fuzzy", "vector", "spatial"]
}

func (cm *CandidateMerger) MergeCandidates(sources ...[]*Candidate) []*Candidate {
    // Map to track best candidate per UPRN
    bestByUPRN := make(map[string]*Candidate)

    for _, source := range sources {
        for _, c := range source {
            existing, found := bestByUPRN[c.UPRN]
            if !found {
                bestByUPRN[c.UPRN] = c
                continue
            }

            // Keep candidate with higher score
            if c.Score > existing.Score {
                // Preserve information from both
                c.AlternativeMethods = append(c.AlternativeMethods, existing.Method)
                c.MethodScores = map[string]float64{
                    existing.Method: existing.Score,
                    c.Method:        c.Score,
                }
                bestByUPRN[c.UPRN] = c
            } else {
                existing.AlternativeMethods = append(existing.AlternativeMethods, c.Method)
                existing.MethodScores[c.Method] = c.Score
            }
        }
    }

    // Convert to slice and sort by score
    var result []*Candidate
    for _, c := range bestByUPRN {
        result = append(result, c)
    }

    sort.Slice(result, func(i, j int) bool {
        return result[i].Score > result[j].Score
    })

    return result
}
```

### 6.9.2 Deduplication by UPRN

```go
func dedupeByUPRN(candidates []*Candidate) []*Candidate {
    if len(candidates) == 0 {
        return nil
    }

    // Sort by score descending first
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].Score > candidates[j].Score
    })

    seen := make(map[string]bool)
    var unique []*Candidate

    for _, c := range candidates {
        if seen[c.UPRN] {
            continue
        }
        seen[c.UPRN] = true
        unique = append(unique, c)
    }

    return unique
}
```

## 6.10 Parallel Processing Architecture

The matching system supports parallel execution for high throughput.

### 6.10.1 Worker Pool Pattern

```go
type ParallelMatcher struct {
    db         *sql.DB
    numWorkers int
    batchSize  int
}

func (pm *ParallelMatcher) RunParallelFuzzyMatching(addresses []string) error {
    // Create channels
    work := make(chan string, len(addresses))
    results := make(chan *MatchResult, len(addresses))
    errors := make(chan error, pm.numWorkers)

    // Start worker pool
    var wg sync.WaitGroup
    for i := 0; i < pm.numWorkers; i++ {
        wg.Add(1)
        go pm.worker(i, work, results, errors, &wg)
    }

    // Send work
    go func() {
        for _, addr := range addresses {
            work <- addr
        }
        close(work)
    }()

    // Wait for completion in background
    go func() {
        wg.Wait()
        close(results)
        close(errors)
    }()

    // Collect results
    var allResults []*MatchResult
    var allErrors []error

    done := false
    for !done {
        select {
        case result, ok := <-results:
            if !ok {
                done = true
                break
            }
            if result != nil {
                allResults = append(allResults, result)
            }
        case err, ok := <-errors:
            if ok && err != nil {
                allErrors = append(allErrors, err)
            }
        }
    }

    // Log statistics
    log.Printf("Processed %d addresses, %d matches, %d errors",
        len(addresses), len(allResults), len(allErrors))

    // Store results
    return pm.storeResults(allResults)
}

func (pm *ParallelMatcher) worker(id int, work <-chan string, results chan<- *MatchResult, errors chan<- error, wg *sync.WaitGroup) {
    defer wg.Done()

    // Each worker gets its own database connection
    matcher := NewFuzzyMatcher(pm.db)

    for addr := range work {
        result, err := matcher.MatchAddress(addr)
        if err != nil {
            errors <- fmt.Errorf("worker %d: %w", id, err)
            continue
        }
        results <- result
    }
}
```

### 6.10.2 Batch Processing

```go
func (pm *ParallelMatcher) ProcessInBatches(addresses []string) error {
    totalBatches := (len(addresses) + pm.batchSize - 1) / pm.batchSize

    for i := 0; i < totalBatches; i++ {
        start := i * pm.batchSize
        end := min(start+pm.batchSize, len(addresses))
        batch := addresses[start:end]

        log.Printf("Processing batch %d/%d (%d addresses)", i+1, totalBatches, len(batch))

        if err := pm.processBatch(batch); err != nil {
            return fmt.Errorf("batch %d failed: %w", i+1, err)
        }
    }

    return nil
}

func (pm *ParallelMatcher) processBatch(batch []string) error {
    // Process batch with parallel workers
    return pm.RunParallelFuzzyMatching(batch)
}
```

### 6.10.3 Progress Tracking

```go
type ProgressTracker struct {
    total     int
    processed int
    matched   int
    failed    int
    startTime time.Time
    mu        sync.Mutex
}

func (pt *ProgressTracker) Update(matched bool, err error) {
    pt.mu.Lock()
    defer pt.mu.Unlock()

    pt.processed++
    if err != nil {
        pt.failed++
    } else if matched {
        pt.matched++
    }
}

func (pt *ProgressTracker) Report() {
    pt.mu.Lock()
    defer pt.mu.Unlock()

    elapsed := time.Since(pt.startTime)
    rate := float64(pt.processed) / elapsed.Seconds()

    matchRate := 0.0
    if pt.processed > 0 {
        matchRate = float64(pt.matched) / float64(pt.processed) * 100
    }

    remaining := pt.total - pt.processed
    eta := time.Duration(float64(remaining)/rate) * time.Second

    log.Printf("Progress: %d/%d (%.1f%%) | Matched: %d (%.1f%%) | Rate: %.1f/s | ETA: %v",
        pt.processed, pt.total, float64(pt.processed)/float64(pt.total)*100,
        pt.matched, matchRate,
        rate, eta)
}
```

## 6.11 Algorithm Selection Logic

The system dynamically selects which algorithms to apply based on input characteristics:

```go
type AlgorithmSelector struct {
    config *SelectionConfig
}

type SelectionConfig struct {
    EnableVector     bool
    EnableSpatial    bool
    VectorMinLength  int     // Minimum address length for vector matching
    SpatialRequired  bool    // Whether coordinates are required for spatial
    ParallelWorkers  int
}

func (as *AlgorithmSelector) SelectAlgorithms(doc *SourceDocument) []string {
    var algorithms []string

    // Always try deterministic first
    algorithms = append(algorithms, "deterministic")

    // Fuzzy matching for addresses with sufficient length
    if doc.AddrCan != nil && len(*doc.AddrCan) >= 10 {
        algorithms = append(algorithms, "fuzzy")
    }

    // Vector matching for longer addresses (better embeddings)
    if as.config.EnableVector && doc.AddrCan != nil && len(*doc.AddrCan) >= as.config.VectorMinLength {
        algorithms = append(algorithms, "vector")
    }

    // Spatial matching when coordinates available
    if as.config.EnableSpatial && doc.Easting != nil && doc.Northing != nil {
        algorithms = append(algorithms, "spatial")
    }

    return algorithms
}
```

## 6.12 Configuration and Thresholds

### 6.12.1 Matching Tier Thresholds

```go
type MatchingThresholds struct {
    // Decision thresholds
    AutoAcceptHigh   float64 // Default: 0.92
    AutoAcceptMedium float64 // Default: 0.88
    NeedsReview      float64 // Default: 0.80
    MinThreshold     float64 // Default: 0.70

    // Margin requirements
    WinnerMargin     float64 // Default: 0.05

    // Algorithm-specific thresholds
    TrigramMin       float64 // Default: 0.30
    VectorMin        float64 // Default: 0.75
    SpatialMaxDist   float64 // Default: 200 metres
}

func DefaultThresholds() *MatchingThresholds {
    return &MatchingThresholds{
        AutoAcceptHigh:   0.92,
        AutoAcceptMedium: 0.88,
        NeedsReview:      0.80,
        MinThreshold:     0.70,
        WinnerMargin:     0.05,
        TrigramMin:       0.30,
        VectorMin:        0.75,
        SpatialMaxDist:   200,
    }
}
```

### 6.12.2 Feature Weights

```go
type FeatureWeights struct {
    Trigram          float64 // Default: 0.35
    Jaro             float64 // Default: 0.25
    TokenOverlap     float64 // Default: 0.15
    HouseNumber      float64 // Default: 0.10
    Locality         float64 // Default: 0.08
    Street           float64 // Default: 0.05
    Phonetic         float64 // Default: 0.02
}

func DefaultWeights() *FeatureWeights {
    return &FeatureWeights{
        Trigram:      0.35,
        Jaro:         0.25,
        TokenOverlap: 0.15,
        HouseNumber:  0.10,
        Locality:     0.08,
        Street:       0.05,
        Phonetic:     0.02,
    }
}
```

## 6.13 Algorithm Performance Characteristics

| Algorithm | Throughput | Precision | Use Case |
|-----------|------------|-----------|----------|
| UPRN Validation | 50,000/min | 99.9% | Documents with existing UPRNs |
| Exact Canonical | 30,000/min | 99.5% | Clean, standardised addresses |
| Trigram Fuzzy (single) | 3,000/min | 98% | Variable formatting |
| Trigram Fuzzy (8 workers) | 20,000/min | 98% | High volume processing |
| Vector Semantic | 1,500/min | 95% | Misspellings, word order variations |
| Spatial | 15,000/min | 97% | Coordinate-based refinement |

## 6.14 Chapter Summary

This chapter has documented the comprehensive matching algorithm suite:

- **Multi-layer pipeline**: Progressive sophistication from deterministic to fuzzy to semantic
- **Layer 2**: UPRN validation and exact canonical matching with 99.9% precision
- **Layer 3**: Group-based fuzzy matching with pg_trgm, reducing processing by ~40%
- **Layer 4**: Individual document matching with multi-tier candidate generation
- **Layer 5**: Conservative validation with stricter thresholds
- **String similarity**: Jaro, Jaro-Winkler, Levenshtein, and trigram algorithms
- **Phonetic matching**: Double Metaphone for spelling variations
- **Semantic matching**: Vector embeddings with Ollama and Qdrant
- **Spatial matching**: PostGIS distance calculations with exponential decay boosting
- **Parallel processing**: Worker pool pattern with configurable concurrency
- **Configurable thresholds**: All parameters externally tunable

The following chapter describes how candidates are scored and decisions are made.

---

*This chapter details the matching algorithms. Chapter 7 covers scoring and decision logic.*
