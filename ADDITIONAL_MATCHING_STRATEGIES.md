# Additional Strategies for Populating EHDC LLPG Data

## Current Situation
- **Deterministic Matching**: ~14.3% coverage potential (18,541 records)
- **Fuzzy Matching at 0.75 threshold**: Limited additional matches due to low similarity
- **Challenge**: Many source addresses have <0.50 similarity to LLPG addresses
- **Example**: "120 WHITE DIRT LANE CATHERINGTON" best match only 0.40 similarity

## Recommended Additional Strategies

### 1. Enhanced Address Normalization & Cleaning
```go
// Implement additional normalization rules
- Expand abbreviations: "RD" → "ROAD", "ST" → "STREET", "AVE" → "AVENUE"
- Handle variations: "SAINT" ↔ "ST", "MOUNT" ↔ "MT"
- Remove noise words: "THE", "OF", "NEAR", "OPPOSITE"
- Normalize business names: "CO-OP" → "COOPERATIVE", "PO" → "POST OFFICE"
- Handle Welsh/Scottish/Irish specific patterns
```

### 2. Hierarchical Matching Strategy
Instead of matching full addresses, break down and match components:

```sql
-- Match by components in order of specificity
1. Postcode + House Number
2. Street Name + House Number + Town
3. Street Name + Locality
4. Partial street name with phonetic matching
5. Locality + nearby streets
```

### 3. Spatial Proximity Matching
For records with coordinates but no UPRN:

```sql
-- Find nearest LLPG addresses within radius
WITH spatial_candidates AS (
  SELECT 
    s.src_id,
    d.uprn,
    ST_Distance(
      ST_SetSRID(ST_MakePoint(s.easting_raw, s.northing_raw), 27700),
      ST_SetSRID(ST_MakePoint(d.easting, d.northing), 27700)
    ) as distance_meters
  FROM src_document s
  CROSS JOIN LATERAL (
    SELECT * FROM dim_address d
    WHERE ST_DWithin(
      ST_SetSRID(ST_MakePoint(s.easting_raw, s.northing_raw), 27700),
      ST_SetSRID(ST_MakePoint(d.easting, d.northing), 27700),
      50  -- 50 meter radius
    )
    ORDER BY ST_Distance(
      ST_SetSRID(ST_MakePoint(s.easting_raw, s.northing_raw), 27700),
      ST_SetSRID(ST_MakePoint(d.easting, d.northing), 27700)
    )
    LIMIT 5
  ) d
  WHERE s.easting_raw IS NOT NULL
)
SELECT * FROM spatial_candidates;
```

### 4. Vector/Semantic Matching with Embeddings
Set up Qdrant or pgvector for semantic similarity:

```python
# Using sentence-transformers for address embeddings
from sentence_transformers import SentenceTransformer
import qdrant_client

model = SentenceTransformer('all-MiniLM-L6-v2')

# Embed all LLPG addresses
llpg_embeddings = model.encode(llpg_addresses)

# For each source address
source_embedding = model.encode(source_address)

# Find semantically similar addresses
similar = qdrant_client.search(
    collection="llpg_addresses",
    query_vector=source_embedding,
    limit=10
)
```

### 5. Machine Learning Classification
Train a classifier on confirmed matches:

```python
# Features for ML model
features = {
    'trigram_similarity': 0.65,
    'jaro_winkler': 0.72,
    'levenshtein_ratio': 0.68,
    'same_postcode': True,
    'same_house_number': True,
    'distance_meters': 25.5,
    'same_street_type': True,
    'phonetic_match': 2,
    'token_overlap': 0.75
}

# Train XGBoost or Random Forest
from xgboost import XGBClassifier
model = XGBClassifier()
model.fit(X_train, y_train)
```

### 6. External Data Enrichment

#### A. Royal Mail PAF (Postcode Address File)
```sql
-- Import PAF data for better postcode matching
CREATE TABLE paf_addresses (
    postcode VARCHAR(10),
    building_number INTEGER,
    building_name VARCHAR(100),
    thoroughfare VARCHAR(100),
    post_town VARCHAR(50),
    uprn VARCHAR(20)
);
```

#### B. OpenStreetMap Data
```python
import osmnx as ox
# Download OSM building data for East Hampshire
buildings = ox.features_from_place(
    "East Hampshire, England", 
    tags={'building': True}
)
```

#### C. Historical Address Databases
- Check historical LLPG snapshots
- Council tax records
- Electoral roll data

### 7. Rule-Based Fallbacks

```sql
-- Specific rules for known problem patterns
CREATE TABLE address_rules (
    pattern VARCHAR(200),
    replacement VARCHAR(200),
    confidence DECIMAL(3,2)
);

INSERT INTO address_rules VALUES
('LUCKY LITE FARM%', 'LUCKYLITE FARM%', 0.95),
('LASHAM AIRFIELD%', 'LASHAM AERODROME%', 0.90),
('FOUR MARKS%', 'FOURMARKS%', 0.85);
```

### 8. Crowd-Sourcing & Manual Review Interface

```javascript
// React component for manual review
const AddressReviewInterface = () => {
  return (
    <div>
      <SourceAddress address={sourceAddr} />
      <CandidateList>
        {candidates.map(c => (
          <Candidate 
            uprn={c.uprn}
            address={c.address}
            similarity={c.score}
            features={c.features}
          />
        ))}
      </CandidateList>
      <Actions>
        <Button onClick={acceptMatch}>Accept</Button>
        <Button onClick={rejectAll}>No Match</Button>
        <Button onClick={needsResearch}>Research Required</Button>
      </Actions>
    </div>
  );
};
```

### 9. Postcode-Centric Matching

```sql
-- For addresses with postcodes, limit search to same postcode
WITH postcode_matches AS (
  SELECT 
    s.src_id,
    s.addr_can,
    s.postcode,
    d.uprn,
    d.addr_can as llpg_addr,
    similarity(
      regexp_replace(s.addr_can, s.postcode, ''), 
      regexp_replace(d.addr_can, s.postcode, '')
    ) as similarity_no_postcode
  FROM src_document s
  JOIN dim_address d ON 
    substring(d.locaddress FROM '[A-Z]{1,2}[0-9]{1,2}[A-Z]?\s*[0-9][A-Z]{2}') = s.postcode
  WHERE s.postcode IS NOT NULL
)
SELECT * FROM postcode_matches 
WHERE similarity_no_postcode > 0.6;
```

### 10. Time-Based Historical Matching

```sql
-- Match based on historical address changes
CREATE TABLE address_history (
    uprn VARCHAR(20),
    old_address TEXT,
    new_address TEXT,
    change_date DATE
);

-- Check if source address matches historical versions
SELECT ah.uprn, ah.old_address, s.addr_raw
FROM src_document s
JOIN address_history ah ON 
  similarity(s.addr_raw, ah.old_address) > 0.8
WHERE s.document_date < ah.change_date;
```

## Implementation Priority

1. **Immediate (Week 1)**
   - Enhanced normalization rules
   - Postcode-centric matching
   - Spatial proximity for geocoded records

2. **Short-term (Week 2-3)**
   - Hierarchical component matching
   - Rule-based fallbacks for known patterns
   - Manual review interface

3. **Medium-term (Week 4-5)**
   - Vector/semantic matching with Qdrant
   - Machine learning classifier
   - PAF data integration

4. **Long-term (Week 6+)**
   - OpenStreetMap integration
   - Historical address database
   - Crowd-sourcing platform

## Expected Coverage Improvements

| Strategy | Expected Additional Coverage | Precision |
|----------|------------------------------|-----------|
| Enhanced Normalization | +5-8% | 95% |
| Postcode-Centric | +10-15% | 90% |
| Spatial Proximity | +3-5% | 85% |
| Hierarchical Matching | +8-12% | 88% |
| Vector/Semantic | +15-20% | 82% |
| ML Classification | +5-10% | 92% |
| External Data | +10-15% | 95% |

**Total Potential Coverage: 75-85%** (from current ~14%)

## Quick Wins to Implement Now

### 1. Improve Canonicalization
```go
func ImprovedCanonicalAddress(raw string) string {
    // Additional abbreviation expansions
    replacements := map[string]string{
        " RD ": " ROAD ",
        " ST ": " STREET ",
        " AVE ": " AVENUE ",
        " CT ": " COURT ",
        " PL ": " PLACE ",
        " DR ": " DRIVE ",
        " LN ": " LANE ",
        " GRNS ": " GARDENS ",
        " GRN ": " GREEN ",
        " CLS ": " CLOSE ",
        " CRES ": " CRESCENT ",
    }
    
    // Apply all replacements
    canonical := strings.ToUpper(raw)
    for old, new := range replacements {
        canonical = strings.ReplaceAll(canonical, old, new)
    }
    
    return canonical
}
```

### 2. Add Partial Matching
```sql
-- Match by house number and street name only
CREATE OR REPLACE FUNCTION match_by_components(
    house_num VARCHAR,
    street_name VARCHAR,
    locality VARCHAR
) RETURNS TABLE(uprn VARCHAR, confidence DECIMAL) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        d.uprn,
        CASE
            WHEN d.addr_can LIKE house_num || '%' || street_name || '%' THEN 0.85
            WHEN d.addr_can LIKE '%' || street_name || '%' || locality || '%' THEN 0.75
            ELSE 0.60
        END as confidence
    FROM dim_address d
    WHERE 
        (house_num IS NULL OR d.addr_can LIKE house_num || '%')
        AND (street_name IS NULL OR d.addr_can LIKE '%' || street_name || '%')
        AND (locality IS NULL OR d.addr_can LIKE '%' || locality || '%')
    ORDER BY confidence DESC
    LIMIT 10;
END;
$$ LANGUAGE plpgsql;
```

### 3. Use Soundex for Phonetic Matching
```sql
-- Add soundex matching for street names
CREATE EXTENSION IF NOT EXISTS fuzzystrmatch;

SELECT 
    s.src_id,
    d.uprn,
    d.addr_can
FROM src_document s
CROSS JOIN LATERAL (
    SELECT * FROM dim_address d
    WHERE soundex(
        substring(s.addr_can from '[A-Z]+ (ROAD|STREET|AVENUE|LANE|DRIVE)')
    ) = soundex(
        substring(d.addr_can from '[A-Z]+ (ROAD|STREET|AVENUE|LANE|DRIVE)')
    )
    LIMIT 10
) d
WHERE s.addr_can IS NOT NULL;
```

## Monitoring & Evaluation

Track these metrics for each strategy:
- Coverage increase (% of records matched)
- Precision (% of correct matches)
- Processing time
- Manual review requirements
- False positive rate
- Confidence distribution

## Next Steps

1. Run the shell script with option 2 (balanced) as default
2. Implement enhanced normalization immediately
3. Set up postcode-centric matching
4. Deploy manual review interface
5. Begin vector database setup for semantic matching