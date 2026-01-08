# Chapter 6: Matching Algorithms

## 6.1 Multi-Layer Pipeline Overview

The matching system employs a multi-layer pipeline where each layer addresses progressively more difficult matching cases. Earlier layers handle straightforward matches efficiently, whilst later layers employ more sophisticated techniques for ambiguous addresses.

```
Input Address
     |
     v
+--------------------+
| Layer 2:           |    Matched (5-15%)
| Deterministic      |-------------------------> ACCEPT
| - Legacy UPRN      |
| - Exact Canonical  |
+--------------------+
     |
     | Unmatched
     v
+--------------------+
| Layer 3:           |    High Confidence (40-60%)
| Fuzzy              |-------------------------> ACCEPT
| - pg_trgm          |
| - Phonetic Filter  |    Medium Confidence
| - Locality Filter  |-------------------------> REVIEW
+--------------------+
     |
     | Unmatched
     v
+--------------------+
| Layer 4:           |    Semantic Matches (10-20%)
| Vector/Semantic    |-------------------------> ACCEPT/REVIEW
| - Embeddings       |
| - Qdrant ANN       |
+--------------------+
     |
     | Unmatched
     v
+--------------------+
| Layer 5:           |    Spatial Matches (5-15%)
| Spatial            |-------------------------> ACCEPT/REVIEW
| - PostGIS Distance |
| - Area Caching     |
+--------------------+
     |
     | Unmatched
     v
NO MATCH
```

## 6.2 Layer 2: Deterministic Matching

Deterministic matching handles cases where exact information is available.

### 6.2.1 Legacy UPRN Validation

When a source document contains a UPRN, the system validates it against the LLPG:

```go
func (dm *DeterministicMatcher) ValidateLegacyUPRN(srcID int64, rawUPRN string) (*MatchResult, error) {
    // Normalise UPRN (remove decimal suffixes)
    cleanUPRN := strings.TrimSpace(rawUPRN)
    if strings.HasSuffix(cleanUPRN, ".00") {
        cleanUPRN = strings.TrimSuffix(cleanUPRN, ".00")
    }

    // Check if UPRN exists in LLPG
    var exists bool
    err := dm.db.QueryRow(`
        SELECT EXISTS(SELECT 1 FROM dim_address WHERE uprn = $1)
    `, cleanUPRN).Scan(&exists)

    if err != nil {
        return nil, err
    }

    if exists {
        return &MatchResult{
            SrcID:         srcID,
            CandidateUPRN: cleanUPRN,
            Method:        "valid_uprn",
            Score:         1.0,
            Confidence:    1.0,
            Decision:      "auto_accepted",
        }, nil
    }

    return nil, nil  // UPRN not found
}
```

**UPRN Normalisation**: Source UPRNs sometimes contain decimal suffixes (for example, "123456789.00") that must be removed before validation.

### 6.2.2 Exact Canonical Address Matching

For documents without valid UPRNs, the system attempts exact canonical matching:

```go
func (dm *DeterministicMatcher) FindExactCanonicalMatch(srcID int64, addrCan string) ([]*MatchResult, error) {
    rows, err := dm.db.Query(`
        SELECT uprn, locaddress, easting, northing
        FROM dim_address
        WHERE addr_can = $1
    `, addrCan)

    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []*MatchResult
    for rows.Next() {
        var uprn, locaddr string
        var easting, northing float64

        if err := rows.Scan(&uprn, &locaddr, &easting, &northing); err != nil {
            continue
        }

        results = append(results, &MatchResult{
            SrcID:         srcID,
            CandidateUPRN: uprn,
            Method:        "exact_canonical",
            Score:         0.99,
            Confidence:    0.99,
        })
    }

    // Single match: auto-accept
    // Multiple matches: needs review
    if len(results) == 1 {
        results[0].Decision = "auto_accepted"
    } else if len(results) > 1 {
        for i := range results {
            results[i].Decision = "needs_review"
            results[i].TieRank = i + 1
        }
    }

    return results, nil
}
```

## 6.3 Layer 3: Fuzzy Matching

Fuzzy matching handles addresses that differ in spelling, abbreviation, or formatting.

### 6.3.1 PostgreSQL pg_trgm Similarity

The pg_trgm extension provides trigram-based similarity matching:

```sql
SELECT uprn, locaddress, addr_can, easting, northing,
       similarity($1, addr_can) as trgm_score
FROM dim_address
WHERE addr_can % $1
  AND similarity($1, addr_can) >= $2
ORDER BY trgm_score DESC
LIMIT 50
```

**Trigram Operation**: The `%` operator returns true if the similarity exceeds the configured threshold (default 0.3). The `similarity()` function returns the actual similarity score between 0 and 1.

### 6.3.2 Fuzzy Candidate Generation

```go
func (fm *FuzzyMatcher) FindFuzzyCandidates(doc SourceDocument, minSimilarity float64) ([]*FuzzyCandidate, error) {
    addrCan := strings.TrimSpace(*doc.AddrCan)

    rows, err := fm.db.Query(`
        SELECT d.uprn, d.locaddress, d.addr_can, d.easting, d.northing,
               d.usrn, d.blpu_class, d.status,
               similarity($1, d.addr_can) as trgm_score
        FROM dim_address d
        WHERE d.addr_can % $1
          AND similarity($1, d.addr_can) >= $2
        ORDER BY trgm_score DESC
        LIMIT 50
    `, addrCan, minSimilarity)

    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var candidates []*FuzzyCandidate
    for rows.Next() {
        candidate := &FuzzyCandidate{
            AddressCandidate: &AddressCandidate{},
            Features:         make(map[string]interface{}),
        }

        err := rows.Scan(
            &candidate.UPRN, &candidate.LocAddress, &candidate.AddrCan,
            &candidate.Easting, &candidate.Northing, &candidate.USRN,
            &candidate.BLPUClass, &candidate.Status, &candidate.TrgramScore,
        )
        if err != nil {
            continue
        }

        // Compute additional features
        fm.computeFeatures(doc, candidate)

        // Apply filtering
        if fm.passesFilters(doc, candidate) {
            candidates = append(candidates, candidate)
        }
    }

    return candidates, nil
}
```

### 6.3.3 Phonetic Filtering

The phonetic filter uses Double Metaphone to catch spelling variations:

```go
type SimplePhonetics struct{}

func (sp *SimplePhonetics) GetMetaphone(text string) (primary, secondary string) {
    text = strings.ToUpper(strings.TrimSpace(text))
    if text == "" {
        return "", ""
    }

    // Basic phonetic transformations
    replacements := map[string]string{
        "PH": "F",
        "GH": "F",
        "CK": "K",
        "QU": "KW",
        "TH": "0",
        "SH": "X",
        "CH": "X",
        "WH": "W",
        "KN": "N",
        "WR": "R",
    }

    result := text
    for pattern, replacement := range replacements {
        result = strings.ReplaceAll(result, pattern, replacement)
    }

    // Remove vowels except at start
    if len(result) > 1 {
        first := string(result[0])
        rest := strings.Map(func(r rune) rune {
            switch r {
            case 'A', 'E', 'I', 'O', 'U', 'Y':
                return -1
            default:
                return r
            }
        }, result[1:])
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
    if len(metaphone) > 4 {
        metaphone = metaphone[:4]
    }

    return metaphone, metaphone
}
```

**Use Case**: Catches variations like HORNDENE vs HORNDEAN, PETERSFEILD vs PETERSFIELD.

### 6.3.4 Locality and House Number Filters

```go
func (fm *FuzzyMatcher) passesFilters(doc SourceDocument, candidate *FuzzyCandidate) bool {
    // Require phonetic overlap for lower similarities
    if candidate.TrgramScore < 0.85 && candidate.PhoneticHits == 0 {
        return false
    }

    // House number validation
    srcHouseNums := extractHouseNumbers(*doc.AddrCan)
    candHouseNums := extractHouseNumbers(candidate.AddrCan)

    if len(srcHouseNums) > 0 && len(candHouseNums) > 0 {
        if !hasOverlap(srcHouseNums, candHouseNums) {
            if !hasCloseNumbers(srcHouseNums, candHouseNums) {
                return false
            }
        }
    }

    return true
}

func hasCloseNumbers(slice1, slice2 []string) bool {
    for _, item1 := range slice1 {
        num1, err1 := strconv.Atoi(strings.TrimRight(item1, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
        if err1 != nil {
            continue
        }

        for _, item2 := range slice2 {
            num2, err2 := strconv.Atoi(strings.TrimRight(item2, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
            if err2 != nil {
                continue
            }

            // Allow renumbering within small range
            if abs(num1-num2) <= 2 {
                return true
            }
        }
    }
    return false
}
```

## 6.4 Layer 4: Vector/Semantic Matching

Semantic matching uses embedding vectors to find addresses with similar meaning but different text.

### 6.4.1 Embedding Generation

Embeddings are generated using Ollama with the nomic-embed-text model:

```go
type SimpleEmbedder struct {
    host  string
    model string
}

func (e *SimpleEmbedder) Embed(text string) ([]float32, error) {
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
        return nil, err
    }
    defer resp.Body.Close()

    var result struct {
        Embedding []float32 `json:"embedding"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    return result.Embedding, nil
}
```

### 6.4.2 Qdrant Vector Storage

The Qdrant vector database stores LLPG address embeddings:

```go
type AddressVectorDB struct {
    client     *QdrantClient
    collection string
    dimension  int
}

func (vdb *AddressVectorDB) Initialize(localDebug bool) error {
    // Create collection with HNSW indexing
    return vdb.client.CreateCollection(vdb.collection, vdb.dimension, "Cosine")
}

func (vdb *AddressVectorDB) IndexAddresses(addresses []AddressWithEmbedding) error {
    points := make([]Point, len(addresses))

    for i, addr := range addresses {
        points[i] = Point{
            ID:      addr.UPRN,
            Vector:  addr.Embedding,
            Payload: map[string]interface{}{
                "locaddress": addr.LocAddress,
                "addr_can":   addr.AddrCan,
                "easting":    addr.Easting,
                "northing":   addr.Northing,
            },
        }
    }

    return vdb.client.Upsert(vdb.collection, points)
}

func (vdb *AddressVectorDB) Search(embedding []float32, limit int) ([]SearchResult, error) {
    return vdb.client.Search(vdb.collection, embedding, limit)
}
```

### 6.4.3 Vector Candidate Generation

```go
func (vm *VectorMatcher) FindVectorCandidates(addrCan string, limit int) ([]*VectorCandidate, error) {
    // Generate embedding for query address
    embedding, err := vm.embedder.Embed(addrCan)
    if err != nil {
        return nil, err
    }

    // Search Qdrant for similar addresses
    results, err := vm.vectorDB.Search(embedding, limit)
    if err != nil {
        return nil, err
    }

    var candidates []*VectorCandidate
    for _, result := range results {
        candidates = append(candidates, &VectorCandidate{
            UPRN:          result.ID,
            LocAddress:    result.Payload["locaddress"].(string),
            AddrCan:       result.Payload["addr_can"].(string),
            Easting:       result.Payload["easting"].(float64),
            Northing:      result.Payload["northing"].(float64),
            CosineSimilarity: result.Score,
        })
    }

    return candidates, nil
}
```

## 6.5 Layer 5: Spatial Matching

Spatial matching uses coordinate data when available.

### 6.5.1 PostGIS Distance Calculation

```go
func (sm *SpatialMatcher) FindSpatialCandidates(easting, northing float64, radiusMetres float64) ([]*SpatialCandidate, error) {
    rows, err := sm.db.Query(`
        SELECT d.uprn, d.locaddress, d.addr_can,
               l.easting, l.northing,
               ST_Distance(
                   l.geom_27700,
                   ST_SetSRID(ST_MakePoint($1, $2), 27700)
               ) as distance_m
        FROM dim_address d
        JOIN dim_location l ON d.location_id = l.location_id
        WHERE ST_DWithin(
            l.geom_27700,
            ST_SetSRID(ST_MakePoint($1, $2), 27700),
            $3
        )
        ORDER BY distance_m
        LIMIT 100
    `, easting, northing, radiusMetres)

    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var candidates []*SpatialCandidate
    for rows.Next() {
        var c SpatialCandidate
        err := rows.Scan(&c.UPRN, &c.LocAddress, &c.AddrCan,
                        &c.Easting, &c.Northing, &c.DistanceMetres)
        if err != nil {
            continue
        }

        // Calculate spatial boost
        c.SpatialBoost = math.Exp(-c.DistanceMetres / 300.0)

        candidates = append(candidates, &c)
    }

    return candidates, nil
}
```

### 6.5.2 Spatial Area Preprocessing

For efficiency, the system precomputes spatial areas:

```go
func (sm *SpatialMatcher) BuildRoadPostcodeAreas() error {
    _, err := sm.db.Exec(`
        INSERT INTO spatial_road_postcode_area (road, postcode, centroid_easting, centroid_northing, address_count)
        SELECT
            COALESCE(gopostal_road, 'UNKNOWN') as road,
            COALESCE(LEFT(gopostal_postcode, 4), 'UNKN') as postcode,
            AVG(easting) as centroid_easting,
            AVG(northing) as centroid_northing,
            COUNT(*) as address_count
        FROM dim_address
        WHERE easting IS NOT NULL AND northing IS NOT NULL
        GROUP BY
            COALESCE(gopostal_road, 'UNKNOWN'),
            COALESCE(LEFT(gopostal_postcode, 4), 'UNKN')
    `)

    return err
}
```

## 6.6 Candidate Deduplication

When generating candidates from multiple sources, deduplication ensures each UPRN appears once:

```go
func dedupeByUPRN(candidates []*Candidate) []*Candidate {
    seen := make(map[string]bool)
    var unique []*Candidate

    for _, c := range candidates {
        if !seen[c.UPRN] {
            seen[c.UPRN] = true
            unique = append(unique, c)
        }
    }

    return unique
}
```

## 6.7 Parallel Processing

Layer 3 fuzzy matching supports parallel execution:

```go
func (fm *FuzzyMatcher) RunParallelFuzzyMatching(numWorkers int) error {
    // Get unique canonical addresses
    addresses, err := fm.getUniqueUnmatchedAddresses()
    if err != nil {
        return err
    }

    // Create work channel
    work := make(chan string, len(addresses))
    results := make(chan *FuzzyResult, len(addresses))

    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for addrCan := range work {
                result := fm.processAddress(addrCan)
                results <- result
            }
        }()
    }

    // Send work
    for _, addr := range addresses {
        work <- addr
    }
    close(work)

    // Wait for completion
    go func() {
        wg.Wait()
        close(results)
    }()

    // Collect results
    for result := range results {
        fm.applyResult(result)
    }

    return nil
}
```

## 6.8 Matching Tiers Configuration

The system uses configurable matching tiers:

```go
type FuzzyMatchingTiers struct {
    HighConfidence   float64  // Default: 0.85
    MediumConfidence float64  // Default: 0.78
    LowConfidence    float64  // Default: 0.70
    MinThreshold     float64  // Default: 0.60
    WinnerMargin     float64  // Default: 0.05
}

func DefaultTiers() *FuzzyMatchingTiers {
    return &FuzzyMatchingTiers{
        HighConfidence:   0.85,
        MediumConfidence: 0.78,
        LowConfidence:    0.70,
        MinThreshold:     0.60,
        WinnerMargin:     0.05,
    }
}
```

## 6.9 Algorithm Performance Characteristics

| Algorithm | Throughput | Use Case |
|-----------|------------|----------|
| Legacy UPRN Validation | 50,000/min | Documents with existing UPRNs |
| Exact Canonical | 30,000/min | Clean, standardised addresses |
| Trigram Fuzzy | 5,000/min (single) | Variable formatting |
| Trigram Fuzzy | 5,000 x N/min (parallel) | High volume processing |
| Vector Semantic | 2,000/min | Misspellings, word order |
| Spatial | 20,000/min | Coordinate-based refinement |

## 6.10 Chapter Summary

This chapter has documented the matching algorithms:

- Multi-layer pipeline architecture
- Layer 2 deterministic matching (UPRN validation, exact canonical)
- Layer 3 fuzzy matching (pg_trgm, phonetic filters, locality/house number)
- Layer 4 semantic matching (embeddings, Qdrant vector search)
- Layer 5 spatial matching (PostGIS distance)
- Candidate deduplication
- Parallel processing support
- Configurable matching tiers

The following chapter describes how candidates are scored and decisions are made.

---

*This chapter details the matching algorithms. Chapter 7 covers scoring and decision logic.*
