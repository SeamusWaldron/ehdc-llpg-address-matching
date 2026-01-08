# Chapter 5: Address Normalisation

## 5.1 The Normalisation Challenge

Address normalisation - converting variable address formats into a consistent canonical form - is a critical prerequisite for effective matching. UK addresses present particular challenges:

- **Inconsistent Capitalisation**: "High Street", "HIGH STREET", "high street"
- **Variable Punctuation**: "12, High St.", "12 High St", "12 High Street"
- **Abbreviations**: "Rd" vs "Road", "St" vs "Street", "Ave" vs "Avenue"
- **Embedded Postcodes**: Sometimes present, sometimes absent
- **Descriptive Prefixes**: "Land at", "Rear of", "Adjacent to"
- **Flat and Unit Numbers**: "Flat 2", "Unit 3A", "Apartment 12"

The normalisation package (`internal/normalize`) addresses these challenges through systematic transformation rules.

## 5.2 Canonical Form Specification

The canonical form follows these rules:

1. **Uppercase**: All text converted to uppercase
2. **Postcode Extraction**: Postcodes removed and stored separately
3. **Punctuation Removal**: Non-alphanumeric characters replaced with spaces
4. **Abbreviation Expansion**: Standard UK abbreviations expanded
5. **Whitespace Collapse**: Multiple spaces reduced to single space
6. **Descriptor Handling**: Standardise or remove descriptive phrases

### 5.2.1 Example Transformations

| Raw Address | Canonical Form |
|-------------|----------------|
| 12a High St., Alton GU34 1AB | 12A HIGH STREET ALTON |
| Flat 2, 15 Station Rd | FLAT 2 15 STATION ROAD |
| Land at Rear of The Old Mill | LAND AT REAR OF THE OLD MILL |
| 3B PETERSFIELD AVENUE, FOUR MARKS | 3B PETERSFIELD AVENUE FOUR MARKS |

## 5.3 Core Normalisation Function

The primary normalisation function is implemented in `internal/normalize/address.go`:

```go
func CanonicalAddress(raw string) (addrCan, postcode string, tokens []string) {
    return CanonicalAddressDebug(false, raw)
}

func CanonicalAddressDebug(localDebug bool, raw string) (addrCan, postcode string, tokens []string) {
    debug.DebugHeader(localDebug)
    defer debug.DebugFooter(localDebug)

    if raw == "" {
        return "", "", []string{}
    }

    s := strings.ToUpper(strings.TrimSpace(raw))

    // Extract postcode first
    if m := rePostcode.FindString(s); m != "" {
        postcode = strings.ReplaceAll(m, " ", "")
        s = rePostcode.ReplaceAllString(s, " ")
    }

    // Remove punctuation but preserve spaces
    b := strings.Builder{}
    for _, r := range s {
        if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
            b.WriteRune(r)
        } else {
            b.WriteRune(' ')
        }
    }
    s = strings.Join(strings.Fields(b.String()), " ")

    // Expand abbreviations using rules
    rules := NewAbbrevRules()
    s = rules.Expand(s)

    // Handle special UK descriptors
    s = handleDescriptors(s)

    // Collapse spaces again
    s = strings.Join(strings.Fields(s), " ")

    tokens = strings.Fields(s)
    return s, postcode, tokens
}
```

## 5.4 Postcode Extraction

UK postcodes follow specific patterns. The extraction regex handles standard formats:

```go
var rePostcode = regexp.MustCompile(
    `\b([A-Za-z]{1,2}\d[\dA-Za-z]?\s*\d[ABD-HJLNP-UW-Zabd-hjlnp-uw-z]{2})\b`)
```

This pattern matches:

- **Outward Code**: 2-4 characters (area + district)
- **Inward Code**: 3 characters (sector + unit)
- **Optional Space**: Between outward and inward codes

### 5.4.1 Valid Postcode Examples

| Postcode | Area | District | Sector | Unit |
|----------|------|----------|--------|------|
| GU34 1AB | GU | 34 | 1 | AB |
| PO8 9HG | PO | 8 | 9 | HG |
| SO24 9NP | SO | 24 | 9 | NP |
| RG27 8PL | RG | 27 | 8 | PL |

### 5.4.2 Postcode Handling

Postcodes are:
1. Extracted from the raw address
2. Stored separately in `postcode_text` column
3. Removed from the canonical form
4. Available for candidate filtering

This approach allows matching addresses with missing or incorrect postcodes whilst preserving postcode information when present.

## 5.5 Abbreviation Expansion

The `AbbrevRules` type manages abbreviation expansion:

```go
type AbbrevRules struct {
    rules map[string]string
}

func NewAbbrevRules() *AbbrevRules {
    rules := map[string]string{
        `\bRD\b`:     "ROAD",
        `\bST\b`:     "STREET",
        `\bAVE\b`:    "AVENUE",
        `\bGDNS\b`:   "GARDENS",
        `\bCT\b`:     "COURT",
        `\bDR\b`:     "DRIVE",
        `\bLN\b`:     "LANE",
        `\bPL\b`:     "PLACE",
        `\bSQ\b`:     "SQUARE",
        `\bCRES\b`:   "CRESCENT",
        `\bTER\b`:    "TERRACE",
        `\bCL\b`:     "CLOSE",
        `\bPK\b`:     "PARK",
        `\bGRN\b`:    "GREEN",
        `\bWY\b`:     "WAY",
        `\bAPT\b`:    "APARTMENT",
        `\bFLT\b`:    "FLAT",
        `\bBLDG\b`:   "BUILDING",
        `\bHSE\b`:    "HOUSE",
        `\bCTG\b`:    "COTTAGE",
        `\bFM\b`:     "FARM",
        `\bMNR\b`:    "MANOR",
        `\bVIL\b`:    "VILLA",
        `\bEST\b`:    "ESTATE",
        `\bINDL\b`:   "INDUSTRIAL",
        `\bCTR\b`:    "CENTRE",
        `\bNTH\b`:    "NORTH",
        `\bSTH\b`:    "SOUTH",
        `\bWST\b`:    "WEST",
    }
    return &AbbrevRules{rules: rules}
}

func (ar *AbbrevRules) Expand(text string) string {
    result := text
    for pattern, replacement := range ar.rules {
        re := regexp.MustCompile(pattern)
        result = re.ReplaceAllString(result, replacement)
    }
    return result
}
```

### 5.5.1 Complete Abbreviation List

| Abbreviation | Expansion | Category |
|--------------|-----------|----------|
| RD | ROAD | Street Type |
| ST | STREET | Street Type |
| AVE | AVENUE | Street Type |
| GDNS | GARDENS | Street Type |
| CT | COURT | Street Type |
| DR | DRIVE | Street Type |
| LN | LANE | Street Type |
| PL | PLACE | Street Type |
| SQ | SQUARE | Street Type |
| CRES | CRESCENT | Street Type |
| TER | TERRACE | Street Type |
| CL | CLOSE | Street Type |
| PK | PARK | Street Type |
| GRN | GREEN | Street Type |
| WY | WAY | Street Type |
| APT | APARTMENT | Building Type |
| FLT | FLAT | Building Type |
| BLDG | BUILDING | Building Type |
| HSE | HOUSE | Building Type |
| CTG | COTTAGE | Building Type |
| FM | FARM | Building Type |
| MNR | MANOR | Building Type |
| VIL | VILLA | Building Type |
| EST | ESTATE | Area Type |
| INDL | INDUSTRIAL | Area Type |
| CTR | CENTRE | Area Type |
| NTH | NORTH | Direction |
| STH | SOUTH | Direction |
| WST | WEST | Direction |

## 5.6 Descriptor Handling

UK addresses often contain descriptive phrases that indicate property relationships:

```go
func handleDescriptors(text string) string {
    descriptorMap := map[string]string{
        "LAND AT":        "LAND AT",
        "LAND ADJ TO":    "LAND ADJACENT TO",
        "LAND ADJACENT":  "LAND ADJACENT TO",
        "REAR OF":        "REAR OF",
        "PLOT":           "PLOT",
        "PARCEL":         "PARCEL",
        "SITE":           "SITE",
        "DEVELOPMENT":    "DEVELOPMENT",
        "PROPOSED":       "",  // Remove
        "FORMER":         "",  // Remove
    }

    result := text
    for pattern, replacement := range descriptorMap {
        re := regexp.MustCompile(`\b` + pattern + `\b`)
        result = re.ReplaceAllString(result, replacement)
    }
    return strings.TrimSpace(result)
}
```

### 5.6.1 Descriptor Categories

**Preserved Descriptors** (indicate property relationships):
- LAND AT
- LAND ADJACENT TO
- REAR OF
- PLOT
- PARCEL
- SITE

**Removed Descriptors** (temporal or speculative):
- PROPOSED
- FORMER

## 5.7 House Number Extraction

House numbers require special handling due to various formats:

```go
var reHouseNumber = regexp.MustCompile(`\b(\d+[A-Za-z]?)\b`)
var reFlatUnit = regexp.MustCompile(`\b(FLAT|APT|APARTMENT|UNIT|STUDIO)\s+(\d+[A-Za-z]?)\b`)

func ExtractHouseNumbers(text string) []string {
    var numbers []string

    // Find house numbers
    matches := reHouseNumber.FindAllString(text, -1)
    numbers = append(numbers, matches...)

    // Find flat/unit numbers
    flatMatches := reFlatUnit.FindAllStringSubmatch(text, -1)
    for _, match := range flatMatches {
        if len(match) > 2 {
            numbers = append(numbers, match[2])
        }
    }

    return numbers
}
```

### 5.7.1 House Number Patterns

| Pattern | Example | Extracted |
|---------|---------|-----------|
| Simple number | "12 High Street" | 12 |
| Alpha suffix | "12A High Street" | 12A |
| Flat number | "Flat 2, 15 High Street" | 2, 15 |
| Unit number | "Unit 3B Industrial Estate" | 3B |

## 5.8 Locality Token Recognition

The system recognises Hampshire locality names to improve matching accuracy:

```go
var localityTokens = map[string]bool{
    "ALTON":         true,
    "PETERSFIELD":   true,
    "LIPHOOK":       true,
    "WATERLOOVILLE": true,
    "HORNDEAN":      true,
    "BORDON":        true,
    "WHITEHILL":     true,
    "GRAYSHOTT":     true,
    "HEADLEY":       true,
    "BRAMSHOTT":     true,
    "LINDFORD":      true,
    "HOLLYWATER":    true,
    "PASSFIELD":     true,
    "CONFORD":       true,
    "FOUR MARKS":    true,
    "MEDSTEAD":      true,
    "CHAWTON":       true,
    "SELBORNE":      true,
    "EMPSHOTT":      true,
    "HAWKLEY":       true,
    "LISS":          true,
    "STEEP":         true,
    "STROUD":        true,
    "BURITON":       true,
    "LANGRISH":      true,
    "EAST MEON":     true,
    "WEST MEON":     true,
    "FROXFIELD":     true,
    "PRIVETT":       true,
    "ROPLEY":        true,
    "WEST TISTED":   true,
    "EAST TISTED":   true,
    "BINSTED":       true,
    "HOLT POUND":    true,
    "BENTLEY":       true,
    "FARNHAM":       true,
    "HASLEMERE":     true,
}

func ExtractLocalityTokens(text string) []string {
    var localities []string
    tokens := strings.Fields(strings.ToUpper(text))

    for _, token := range tokens {
        if localityTokens[token] {
            localities = append(localities, token)
        }
    }

    // Handle multi-word localities
    upperText := strings.ToUpper(text)
    for locality := range localityTokens {
        if strings.Contains(locality, " ") && strings.Contains(upperText, locality) {
            localities = append(localities, locality)
        }
    }

    return localities
}
```

## 5.9 Street Token Extraction

Street names are extracted by excluding other token types:

```go
func TokenizeStreet(text string) []string {
    tokens := strings.Fields(strings.ToUpper(text))
    var streetTokens []string

    skipWords := map[string]bool{
        "FLAT": true, "APT": true, "APARTMENT": true,
        "UNIT": true, "STUDIO": true,
        "THE": true, "AND": true, "OF": true,
        "AT": true, "IN": true, "ON": true,
        "LAND": true, "REAR": true, "ADJACENT": true,
        "TO": true, "PLOT": true,
        "SITE": true, "DEVELOPMENT": true, "PARCEL": true,
    }

    for _, token := range tokens {
        // Skip numbers
        if reHouseNumber.MatchString(token) {
            continue
        }
        // Skip localities
        if localityTokens[token] {
            continue
        }
        // Skip common words
        if skipWords[token] {
            continue
        }
        // Skip very short tokens
        if len(token) < 2 {
            continue
        }

        streetTokens = append(streetTokens, token)
    }

    return streetTokens
}
```

## 5.10 Token Overlap Calculation

Token overlap measures similarity between address components:

```go
func TokenOverlap(tokens1, tokens2 []string) float64 {
    if len(tokens1) == 0 && len(tokens2) == 0 {
        return 1.0
    }
    if len(tokens1) == 0 || len(tokens2) == 0 {
        return 0.0
    }

    set1 := make(map[string]bool)
    for _, token := range tokens1 {
        set1[token] = true
    }

    overlap := 0
    for _, token := range tokens2 {
        if set1[token] {
            overlap++
        }
    }

    // Return overlap as ratio of smaller set
    minLen := len(tokens1)
    if len(tokens2) < minLen {
        minLen = len(tokens2)
    }

    return float64(overlap) / float64(minLen)
}
```

## 5.11 libpostal Integration

For advanced parsing, the system integrates with libpostal via HTTP:

```go
type PostalComponents struct {
    HouseNumber string `json:"house_number"`
    Road        string `json:"road"`
    City        string `json:"city"`
    Postcode    string `json:"postcode"`
    Country     string `json:"country"`
}

func ParseWithLibpostal(address string) (*PostalComponents, error) {
    resp, err := http.Post(
        "http://localhost:8080/parse",
        "application/json",
        strings.NewReader(`{"address": "`+address+`"}`),
    )
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var components PostalComponents
    if err := json.NewDecoder(resp.Body).Decode(&components); err != nil {
        return nil, err
    }

    return &components, nil
}
```

libpostal provides:
- Statistical address parsing trained on global data
- Component extraction (house number, road, city, etc.)
- Normalisation of international address formats

## 5.12 Database Normalisation

Normalisation also occurs at the database level:

```sql
-- Basic SQL normalisation
SELECT REGEXP_REPLACE(
    REGEXP_REPLACE(UPPER(TRIM(address)), '\\s+', ' ', 'g'),
    '[^A-Z0-9 ]', '', 'g'
) AS address_canonical
FROM source_table;
```

This provides consistent normalisation for addresses already in the database.

## 5.13 Normalisation Quality Metrics

The normalisation process is evaluated on:

1. **Consistency**: Same input always produces same output
2. **Reversibility**: Original address preserved alongside canonical form
3. **Token Preservation**: Important address components retained
4. **Matching Effectiveness**: Normalised forms improve match rates

## 5.14 Chapter Summary

This chapter has documented the address normalisation subsystem:

- Canonical form specification and transformation rules
- Postcode extraction and handling
- Abbreviation expansion with configurable rules
- Descriptor handling for property relationships
- House number and flat number extraction
- Locality token recognition for Hampshire
- Street token extraction
- Token overlap calculation for similarity scoring
- libpostal integration for advanced parsing

Effective normalisation is essential for the matching algorithms described in the following chapter.

---

*This chapter establishes normalisation as a foundation for matching. Chapter 6 describes the matching algorithms in detail.*
