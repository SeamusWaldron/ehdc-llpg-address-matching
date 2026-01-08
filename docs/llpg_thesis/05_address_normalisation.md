# Chapter 5: Address Normalisation and Cleansing

## 5.1 The Normalisation Challenge

Address normalisation - converting variable address formats into a consistent canonical form - is the foundation of effective address matching. UK addresses present particular challenges that require systematic, multi-layered cleansing:

- **Inconsistent Capitalisation**: "High Street", "HIGH STREET", "high street"
- **Variable Punctuation**: "12, High St.", "12 High St", "12 High Street"
- **Abbreviations**: "Rd" vs "Road", "St" vs "Street", "Ave" vs "Avenue"
- **Embedded Postcodes**: Sometimes present, sometimes absent, various formats
- **Descriptive Prefixes**: "Land at", "Rear of", "Adjacent to", "Site of"
- **Flat and Unit Numbers**: "Flat 2", "Unit 3A", "Apartment 12", "Suite B"
- **Business Name Variations**: "Co-op", "CO-OP", "Cooperative"
- **Historic Spelling**: Addresses from decades-old documents with obsolete formats
- **Data Entry Errors**: Typos, transpositions, missing components

The EHDC LLPG system implements a comprehensive multi-stage cleansing pipeline across three packages:

1. **`internal/normalize`** - Core canonicalisation functions
2. **`internal/validation`** - UK-specific parsing and validation
3. **SQL Migrations** - Database-level normalisation

## 5.2 Canonical Form Specification

The canonical form follows these transformation rules:

1. **Uppercase**: All text converted to uppercase
2. **Postcode Extraction**: Postcodes removed and stored separately
3. **Punctuation Removal**: Non-alphanumeric characters replaced with spaces
4. **Abbreviation Expansion**: Standard UK abbreviations expanded to full form
5. **Whitespace Collapse**: Multiple spaces reduced to single space
6. **Descriptor Handling**: Standardise or remove descriptive phrases
7. **Noise Word Removal**: Remove words that do not aid matching

### 5.2.1 Example Transformations

| Raw Address | Canonical Form | Extracted Postcode |
|-------------|----------------|-------------------|
| 12a High St., Alton GU34 1AB | 12A HIGH STREET ALTON | GU341AB |
| Flat 2, 15 Station Rd | FLAT 2 15 STATION ROAD | |
| Land at Rear of The Old Mill | LAND AT REAR OF OLD MILL | |
| 3B PETERSFIELD AVE, FOUR MARKS | 3B PETERSFIELD AVENUE FOUR MARKS | |
| UNIT 9C, AMEY INDL EST | UNIT 9C AMEY INDUSTRIAL ESTATE | |
| CO-OP, HIGH STREET | COOPERATIVE HIGH STREET | |

## 5.3 Core Normalisation Pipeline

The primary normalisation is implemented in `internal/normalize/address.go`:

### 5.3.1 Main Canonicalisation Function

```go
func CanonicalAddressDebug(localDebug bool, raw string) (addrCan, postcode string, tokens []string) {
    if raw == "" {
        return "", "", []string{}
    }

    s := strings.ToUpper(strings.TrimSpace(raw))

    // Step 1: Extract postcode first
    if m := rePostcode.FindString(s); m != "" {
        postcode = strings.ReplaceAll(m, " ", "")
        s = rePostcode.ReplaceAllString(s, " ")
    }

    // Step 2: Remove punctuation but preserve spaces
    b := strings.Builder{}
    for _, r := range s {
        if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
            b.WriteRune(r)
        } else {
            b.WriteRune(' ')
        }
    }
    s = strings.Join(strings.Fields(b.String()), " ")

    // Step 3: Expand abbreviations using rules
    rules := NewAbbrevRules()
    s = rules.Expand(s)

    // Step 4: Handle special UK descriptors
    s = handleDescriptors(s)

    // Step 5: Collapse spaces again
    s = strings.Join(strings.Fields(s), " ")

    tokens = strings.Fields(s)
    return s, postcode, tokens
}
```

### 5.3.2 Processing Steps Explained

| Step | Operation | Example Input | Example Output |
|------|-----------|---------------|----------------|
| 1 | Uppercase + Trim | "  12a high st  " | "12A HIGH ST" |
| 2 | Extract Postcode | "12A HIGH ST GU34 1AB" | "12A HIGH ST " (postcode: GU341AB) |
| 3 | Remove Punctuation | "12A, HIGH ST." | "12A HIGH ST" |
| 4 | Expand Abbreviations | "12A HIGH ST" | "12A HIGH STREET" |
| 5 | Handle Descriptors | "FORMER 12A HIGH STREET" | "12A HIGH STREET" |
| 6 | Collapse Whitespace | "12A  HIGH   STREET" | "12A HIGH STREET" |

## 5.4 Postcode Extraction

UK postcodes follow specific patterns. The extraction regex handles all standard formats:

```go
var rePostcode = regexp.MustCompile(
    `\b([A-Za-z]{1,2}\d[\dA-Za-z]?\s*\d[ABD-HJLNP-UW-Zabd-hjlnp-uw-z]{2})\b`)
```

### 5.4.1 Postcode Pattern Breakdown

| Component | Pattern | Description |
|-----------|---------|-------------|
| Area | `[A-Za-z]{1,2}` | 1-2 letters (e.g., GU, SO, PO) |
| District | `\d[\dA-Za-z]?` | Digit optionally followed by digit or letter |
| Space | `\s*` | Optional whitespace |
| Sector | `\d` | Single digit |
| Unit | `[ABD-HJLNP-UW-Z]{2}` | Two letters (excluding C, I, K, M, O, V) |

### 5.4.2 Valid Postcode Examples

| Postcode | Area | District | Sector | Unit | Format |
|----------|------|----------|--------|------|--------|
| GU34 1AB | GU | 34 | 1 | AB | AN NN NAA |
| PO8 9HG | PO | 8 | 9 | HG | AN N NAA |
| SO24 9NP | SO | 24 | 9 | NP | AAN NN NAA |
| RG27 8PL | RG | 27 | 8 | PL | AAN NN NAA |
| W1A 0AX | W | 1A | 0 | AX | ANA N NAA |
| EC1A 1BB | EC | 1A | 1 | BB | AANA NAA |

### 5.4.3 Postcode Handling Strategy

Postcodes are:
1. **Extracted** from the raw address using regex
2. **Normalised** by removing internal spaces
3. **Stored separately** in `postcode_text` column
4. **Removed** from the canonical form
5. **Available** for candidate filtering during matching

This approach allows matching addresses with missing or incorrect postcodes whilst preserving postcode information when present.

## 5.5 Abbreviation Expansion

### 5.5.1 Basic Abbreviation Rules (37 Rules)

The `AbbrevRules` type manages abbreviation expansion using word-boundary regex:

```go
func NewAbbrevRules() *AbbrevRules {
    rules := map[string]string{
        // Street Types
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

        // Building Types
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

        // Directions
        `\bNTH\b`:    "NORTH",
        `\bSTH\b`:    "SOUTH",
        `\bE\b`:      "EAST",
        `\bWST\b`:    "WEST",
    }
    return &AbbrevRules{rules: rules}
}
```

### 5.5.2 Complete Abbreviation Reference

| Category | Abbreviation | Expansion |
|----------|--------------|-----------|
| **Street Types** | RD | ROAD |
| | ST | STREET |
| | AVE | AVENUE |
| | GDNS | GARDENS |
| | CT | COURT |
| | DR | DRIVE |
| | LN | LANE |
| | PL | PLACE |
| | SQ | SQUARE |
| | CRES | CRESCENT |
| | TER | TERRACE |
| | CL | CLOSE |
| | PK | PARK |
| | GRN | GREEN |
| | WY | WAY |
| **Building Types** | APT | APARTMENT |
| | FLT | FLAT |
| | BLDG | BUILDING |
| | HSE | HOUSE |
| | CTG | COTTAGE |
| | FM | FARM |
| | MNR | MANOR |
| | VIL | VILLA |
| **Area Types** | EST | ESTATE |
| | INDL | INDUSTRIAL |
| | CTR | CENTRE |
| **Directions** | NTH | NORTH |
| | STH | SOUTH |
| | E | EAST |
| | WST | WEST |

## 5.6 Enhanced Normalisation

The enhanced normalisation in `internal/normalize/enhanced.go` provides additional cleansing:

### 5.6.1 Extended Abbreviation Rules (60+ Rules)

```go
func expandAbbreviations(address string) string {
    abbreviations := map[string]string{
        // Additional Street Types
        " WLK ": " WALK ",
        " GRV ": " GROVE ",
        " VW ":  " VIEW ",
        " HTS ": " HEIGHTS ",
        " HL ":  " HILL ",
        " PSGE ": " PASSAGE ",
        " YD ":  " YARD ",
        " MS ":  " MEWS ",
        " RIS ": " RISE ",
        " PTH ": " PATH ",

        // Compass Directions
        " N ":  " NORTH ",
        " S ":  " SOUTH ",
        " E ":  " EAST ",
        " W ":  " WEST ",
        " NE ": " NORTH EAST ",
        " NW ": " NORTH WEST ",
        " SE ": " SOUTH EAST ",
        " SW ": " SOUTH WEST ",

        // Prefixes
        " ST. ":  " SAINT ",
        " MT ":   " MOUNT ",
        " FT ":   " FORT ",

        // Building Types
        " BLDGS ": " BUILDINGS ",
        " BLK ":   " BLOCK ",
        " FLR ":   " FLOOR ",
        " STE ":   " SUITE ",
        " RM ":    " ROOM ",
        " HO ":    " HOUSE ",
        " COTT ":  " COTTAGE ",

        // Business/Landmarks
        " PO ":   " POST OFFICE ",
        " IND ":  " INDUSTRIAL ",
        " PH ":   " PUBLIC HOUSE ",
        " CH ":   " CHURCH ",
        " SCH ":  " SCHOOL ",
        " HOSP ": " HOSPITAL ",
        " UNI ":  " UNIVERSITY ",
        " STN ":  " STATION ",

        // Regional
        " HANTS ": " HAMPSHIRE ",
    }
    // ... expansion logic
}
```

### 5.6.2 Noise Word Removal

Words that do not contribute to matching accuracy are removed:

```go
func removeNoiseWords(address string) string {
    noiseWords := []string{
        " THE ",
        "^THE ",      // At start
        " OF ",
        " NEAR ",
        " OPPOSITE ",
        " OPP ",
        " ADJ ",
        " ADJACENT ",
        " BEHIND ",
        " FRONT ",
        " REAR ",
        " SIDE ",
    }
    // ... removal logic
}
```

### 5.6.3 Business Name Normalisation

Common business name variations are standardised:

```go
func normalizeBusinessNames(address string) string {
    businessNames := map[string]string{
        "CO-OP":           "COOPERATIVE",
        "COOP":            "COOPERATIVE",
        "CO OP":           "COOPERATIVE",
        "TESCO'S":         "TESCO",
        "SAINSBURY'S":     "SAINSBURYS",
        "SAINSBURY":       "SAINSBURYS",
        "MCDONALD'S":      "MCDONALDS",
        "MARKS & SPENCER": "MARKS AND SPENCER",
        "M&S":             "MARKS AND SPENCER",
        "B&Q":             "B AND Q",
        "BARCLAYS BANK":   "BARCLAYS",
        "LLOYDS BANK":     "LLOYDS",
        "HSBC BANK":       "HSBC",
        "NATWEST BANK":    "NATWEST",
    }
    // ... normalisation logic
}
```

### 5.6.4 Punctuation Cleansing

```go
func cleanPunctuation(address string) string {
    // Remove apostrophes and quotes
    address = strings.ReplaceAll(address, "'", "")
    address = strings.ReplaceAll(address, "\"", "")
    address = strings.ReplaceAll(address, "`", "")

    // Replace hyphens and underscores with spaces
    address = strings.ReplaceAll(address, "-", " ")
    address = strings.ReplaceAll(address, "_", " ")

    // Remove other punctuation
    punctuation := []string{",", ".", ";", ":", "!", "?",
                           "(", ")", "[", "]", "{", "}", "/", "\\"}
    for _, p := range punctuation {
        address = strings.ReplaceAll(address, p, " ")
    }

    // Handle ampersands
    address = strings.ReplaceAll(address, "&", " AND ")

    return address
}
```

## 5.7 Descriptor Handling

UK addresses often contain descriptive phrases indicating property relationships:

### 5.7.1 Descriptor Processing

```go
func handleDescriptors(text string) string {
    descriptorMap := map[string]string{
        // Preserved (indicate property relationships)
        "LAND AT":        "LAND AT",
        "LAND ADJ TO":    "LAND ADJACENT TO",
        "LAND ADJACENT":  "LAND ADJACENT TO",
        "REAR OF":        "REAR OF",
        "PLOT":           "PLOT",
        "PARCEL":         "PARCEL",
        "SITE":           "SITE",
        "DEVELOPMENT":    "DEVELOPMENT",

        // Removed (temporal or speculative)
        "PROPOSED":       "",
        "FORMER":         "",
    }
    // ... processing logic
}
```

### 5.7.2 Descriptor Categories

| Category | Descriptors | Action |
|----------|-------------|--------|
| **Preserved** | LAND AT, LAND ADJACENT TO, REAR OF, PLOT, PARCEL, SITE | Standardise format |
| **Removed** | PROPOSED, FORMER | Delete from address |
| **Vague** | NORTH OF, SOUTH OF, EAST OF, WEST OF, ADJOINING | Flag for review |

## 5.8 House Number Extraction

House numbers require special handling due to various UK formats:

### 5.8.1 Extraction Patterns

```go
// Simple house number: "12", "45A"
var reHouseNumber = regexp.MustCompile(`\b(\d+[A-Za-z]?)\b`)

// Flat/unit patterns: "Flat 2", "Unit 3A"
var reFlatUnit = regexp.MustCompile(
    `\b(FLAT|APT|APARTMENT|UNIT|STUDIO)\s+(\d+[A-Za-z]?)\b`)

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

### 5.8.2 House Number Patterns

| Pattern | Example | Extracted |
|---------|---------|-----------|
| Simple number | "12 High Street" | 12 |
| Alpha suffix | "12A High Street" | 12A |
| Flat number | "Flat 2, 15 High Street" | 2, 15 |
| Unit number | "Unit 3B Industrial Estate" | 3B |
| Range | "12-14 High Street" | 12, 14 |
| Suite | "Suite 100, Business Park" | 100 |

### 5.8.3 Validation Patterns

```go
func isValidHouseNumber(houseNum string) bool {
    patterns := []string{
        `^\d+[A-Z]?$`,                    // "123", "45A"
        `^(?i)UNIT\s+\d+[A-Z]?$`,         // "Unit 2", "UNIT 5A"
        `^(?i)FLAT\s+[A-Z0-9]+$`,         // "Flat A", "FLAT 12"
        `^(?i)SUITE\s+\d+[A-Z]?$`,        // "Suite 1", "SUITE 10B"
        `^\d+[A-Z]?[-/]\d+[A-Z]?$`,       // "12-14", "5A/B"
    }
    // ... validation logic
}
```

## 5.9 Locality Token Recognition

The system recognises Hampshire locality names to improve matching accuracy:

### 5.9.1 Hampshire Localities (32 Towns)

```go
var localityTokens = map[string]bool{
    // Major Towns
    "ALTON":         true,
    "PETERSFIELD":   true,
    "LIPHOOK":       true,
    "WATERLOOVILLE": true,
    "HORNDEAN":      true,
    "BORDON":        true,
    "WHITEHILL":     true,

    // Villages
    "GRAYSHOTT":     true,
    "HEADLEY":       true,
    "BRAMSHOTT":     true,
    "LINDFORD":      true,
    "HOLLYWATER":    true,
    "PASSFIELD":     true,
    "CONFORD":       true,
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
    "FROXFIELD":     true,
    "PRIVETT":       true,
    "ROPLEY":        true,
    "BINSTED":       true,
    "BENTLEY":       true,

    // Multi-word
    "FOUR MARKS":    true,
    "EAST MEON":     true,
    "WEST MEON":     true,
    "WEST TISTED":   true,
    "EAST TISTED":   true,
    "HOLT POUND":    true,

    // Border Towns
    "FARNHAM":       true,
    "HASLEMERE":     true,
}
```

### 5.9.2 Locality Extraction

```go
func ExtractLocalityTokens(text string) []string {
    var localities []string
    tokens := strings.Fields(strings.ToUpper(text))

    // Single-word localities
    for _, token := range tokens {
        if localityTokens[token] {
            localities = append(localities, token)
        }
    }

    // Multi-word localities
    upperText := strings.ToUpper(text)
    for locality := range localityTokens {
        if strings.Contains(locality, " ") &&
           strings.Contains(upperText, locality) {
            localities = append(localities, locality)
        }
    }

    return localities
}
```

## 5.10 Street Token Extraction

Street names are extracted by excluding other token types:

```go
func TokenizeStreet(text string) []string {
    tokens := strings.Fields(strings.ToUpper(text))
    var streetTokens []string

    skipWords := map[string]bool{
        // Unit identifiers
        "FLAT": true, "APT": true, "APARTMENT": true,
        "UNIT": true, "STUDIO": true,
        // Common words
        "THE": true, "AND": true, "OF": true,
        "AT": true, "IN": true, "ON": true,
        // Descriptors
        "LAND": true, "REAR": true, "ADJACENT": true,
        "TO": true, "PLOT": true, "SITE": true,
        "DEVELOPMENT": true, "PARCEL": true,
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

## 5.11 Phonetic Normalisation

The system uses Double Metaphone for phonetic matching of address components:

### 5.11.1 Phonetic Substitution Rules

```go
type DoubleMetaphone struct {
    substitutions map[string]string
}

func NewDoubleMetaphone() *DoubleMetaphone {
    return &DoubleMetaphone{
        substitutions: map[string]string{
            // Common UK place name phonetic issues
            "PH": "F",
            "GH": "F",
            "CK": "K",
            "QU": "KW",
            "C":  "K",   // Before E, I, Y
            "G":  "J",   // Before E, I, Y
            "Y":  "I",
            "Z":  "S",

            // Double letters
            "LL": "L",
            "SS": "S",
            "NN": "N",
            "MM": "M",
            "RR": "R",

            // Silent letters
            "KN": "N",
            "WR": "R",
            "PS": "S",
        },
    }
}
```

### 5.11.2 Phonetic Encoding

```go
func (dm *DoubleMetaphone) Encode(s string) string {
    s = strings.ToUpper(strings.TrimSpace(s))
    result := strings.Builder{}

    for i := 0; i < len(s); i++ {
        // Handle two-character combinations first
        if i < len(s)-1 {
            twoChar := string(s[i]) + string(s[i+1])
            if replacement, exists := dm.substitutions[twoChar]; exists {
                result.WriteString(replacement)
                i++
                continue
            }
        }

        // Handle single characters
        char := string(s[i])
        if replacement, exists := dm.substitutions[char]; exists {
            result.WriteString(replacement)
        } else if isConsonant(char) {
            result.WriteString(char)
        }

        // Keep initial vowels
        if i == 0 && isVowel(char) {
            result.WriteString(char)
        }
    }

    // Limit length and remove duplicates
    code := result.String()
    if len(code) > 6 {
        code = code[:6]
    }

    return removeDuplicateChars(code)
}
```

### 5.11.3 Phonetic Examples

| Word | Phonetic Code | Notes |
|------|---------------|-------|
| SMITH | SMT | TH dropped |
| SMYTHE | SMT | Y→I, TH dropped |
| WRIGHT | RT | WR→R, GH→silent |
| KNIGHT | NT | KN→N, GH→silent |
| PHONE | FN | PH→F |
| CROFT | KRFT | C→K |
| CHURCH | KRK | CH→K |

## 5.12 Address Component Extraction

### 5.12.1 Component Structure

```go
type AddressComponents struct {
    HouseNumber  string
    HouseName    string
    StreetName   string
    Locality     string
    Town         string
    County       string
    Postcode     string
}
```

### 5.12.2 Component Extraction Function

```go
func ExtractAddressComponents(address string) *AddressComponents {
    components := &AddressComponents{}

    // Extract postcode first
    components.Postcode = extractPostcode(address)
    addressWithoutPostcode := removePostcode(address, components.Postcode)

    // Extract house number
    houseNumPattern := regexp.MustCompile(`^(\d+[A-Z]?)\s+`)
    if matches := houseNumPattern.FindStringSubmatch(addressWithoutPostcode);
       len(matches) > 1 {
        components.HouseNumber = matches[1]
    }

    // Identify town from Hampshire towns list
    hampshireTowns := []string{
        "ALTON", "PETERSFIELD", "LIPHOOK", "WATERLOOVILLE",
        "HORNDEAN", "FOUR MARKS", "BEECH", "ROPLEY",
        "ALRESFORD", "BORDON", "WHITEHILL", "GRAYSHOTT",
        // ... 30 towns total
    }

    for _, town := range hampshireTowns {
        if strings.Contains(strings.ToUpper(addressWithoutPostcode), town) {
            components.Town = town
            break
        }
    }

    // Extract street name using street type indicators
    streetTypes := []string{
        "ROAD", "STREET", "AVENUE", "LANE", "DRIVE", "CLOSE",
        "COURT", "PLACE", "GARDENS", "GREEN", "CRESCENT",
        "TERRACE", "SQUARE", "HILL", "WAY", "PARK", "GROVE",
        "RISE", "WALK", "PATH", "MEWS", "YARD",
    }

    for _, streetType := range streetTypes {
        pattern := regexp.MustCompile(`(\w+(?:\s+\w+)*)\s+` + streetType)
        if matches := pattern.FindStringSubmatch(
           strings.ToUpper(addressWithoutPostcode)); len(matches) > 1 {
            components.StreetName = matches[1] + " " + streetType
            break
        }
    }

    // County detection
    if strings.Contains(strings.ToUpper(address), "HAMPSHIRE") ||
       strings.Contains(strings.ToUpper(address), "HANTS") {
        components.County = "HAMPSHIRE"
    }

    return components
}
```

## 5.13 UK Address Parsing

The `internal/validation/parser.go` module provides comprehensive UK-specific parsing:

### 5.13.1 Parser Configuration

```go
type AddressParser struct {
    config ParsingConfig

    // Compiled regex patterns
    unitPattern     *regexp.Regexp  // UNIT, 2 or UNIT 2
    flatPattern     *regexp.Regexp  // FLAT, A or FLAT A
    estatePattern   *regexp.Regexp  // INDUSTRIAL ESTATE
    postcodePattern *regexp.Regexp  // UK postcodes
    houseNumPattern *regexp.Regexp  // House numbers
}

func NewAddressParser() *AddressParser {
    return &AddressParser{
        unitPattern:     regexp.MustCompile(`(?i)\b(UNIT[,\s]+\d+[A-Z]?)\b`),
        flatPattern:     regexp.MustCompile(`(?i)\b(FLAT[,\s]+[A-Z0-9]+)\b`),
        estatePattern:   regexp.MustCompile(`(?i)\b(INDUSTRIAL\s+ESTATE?|IND\s+EST)\b`),
        postcodePattern: regexp.MustCompile(`(?i)\b([A-Z]{1,2}\d{1,2}[A-Z]?\s*\d[A-Z]{2})\b`),
        houseNumPattern: regexp.MustCompile(`(?i)^\s*(\d+[A-Z]?)\b`),
    }
}
```

### 5.13.2 Parsing Pipeline

```go
func (p *AddressParser) ParseAddress(address string) AddressComponents {
    // Step 1: Pre-process for UK-specific patterns
    cleaned := p.preprocessAddress(address)

    // Step 2: Parse with regex (gopostal placeholder)
    components := p.parseWithGopostal(cleaned)

    // Step 3: Post-process with UK enhancements
    components = p.postprocessComponents(components, address)

    // Step 4: Validate extraction quality
    components = p.validateExtraction(components)

    return components
}
```

### 5.13.3 Extraction Confidence Scoring

```go
func (p *AddressParser) validateExtraction(components AddressComponents) AddressComponents {
    var confidenceFactors []float64

    // House number: 0.0 (missing), 0.5 (questionable), 1.0 (valid)
    if components.HouseNumber == "" {
        confidenceFactors = append(confidenceFactors, 0.0)
    } else if p.isValidHouseNumber(components.HouseNumber) {
        confidenceFactors = append(confidenceFactors, 1.0)
    } else {
        confidenceFactors = append(confidenceFactors, 0.5)
    }

    // Street: 0.0 (missing), 0.3 (too short), 1.0 (valid)
    // Postcode: 0.0 (missing), 0.2 (invalid format), 1.0 (valid)
    // Locality: 0.5 (missing - not critical), 1.0 (present)

    // Calculate overall confidence
    components.ExtractionConfidence = average(confidenceFactors)
    components.IsValidForMatching =
        components.ExtractionConfidence >= 0.6 &&
        components.HasHouseNumber() &&
        components.HasStreet()

    return components
}
```

## 5.14 Database-Level Normalisation

### 5.14.1 SQL Canonicalisation

```sql
-- Basic address canonicalization in SQL
SELECT REGEXP_REPLACE(
    REGEXP_REPLACE(UPPER(TRIM(address)), '\s+', ' ', 'g'),
    '[^A-Z0-9 ]', '', 'g'
) AS address_canonical
FROM source_table;
```

### 5.14.2 Normalisation Rules Table

```sql
CREATE TABLE address_normalization_rules (
    rule_id SERIAL PRIMARY KEY,
    pattern TEXT,
    replacement TEXT,
    rule_type TEXT,
    priority INTEGER DEFAULT 0
);

INSERT INTO address_normalization_rules (pattern, replacement, rule_type, priority) VALUES
('\bRD\b', 'ROAD', 'abbreviation', 100),
('\bST\b', 'STREET', 'abbreviation', 100),
('\bAVE\b', 'AVENUE', 'abbreviation', 100),
('\bGDNS\b', 'GARDENS', 'abbreviation', 100),
('\bCT\b', 'COURT', 'abbreviation', 100),
('\bDR\b', 'DRIVE', 'abbreviation', 100),
('\bLN\b', 'LANE', 'abbreviation', 100),
('\bPL\b', 'PLACE', 'abbreviation', 100),
('\bSQ\b', 'SQUARE', 'abbreviation', 100),
('\bCRES\b', 'CRESCENT', 'abbreviation', 100),
('\bTER\b', 'TERRACE', 'abbreviation', 100),
('\bCL\b', 'CLOSE', 'abbreviation', 100),
('\bPK\b', 'PARK', 'abbreviation', 90),
('\bGRN\b', 'GREEN', 'abbreviation', 90),
('\bWY\b', 'WAY', 'abbreviation', 90);
```

## 5.15 LLM-Based Address Correction (Experimental)

The system includes experimental LLM-based address correction using Ollama:

### 5.15.1 LLM Correction Approach

```go
func callLLMForAddressCorrection(rawAddress string, localDebug bool) (string, error) {
    prompt := `You are an address formatting expert. Your task is to correct
obvious formatting issues in UK addresses.

Rules:
1. Fix capitalization: "HIGH STREET" → "High Street"
2. Expand abbreviations: "RD" → "Road", "ST" → "Street"
3. Keep house numbers exactly as they are
4. For incomplete estate names, add "Estate" if clearly missing
5. DO NOT add "Unit" prefix unless it already exists
6. Only return the corrected address, no explanation

Address to correct: ` + rawAddress

    // Call Ollama with llama3.2:1b model
    // Temperature: 0.1 (deterministic)
}
```

### 5.15.2 LLM Correction Status

**Currently Disabled** - Testing revealed that LLM corrections degraded data quality:

| Issue | Example |
|-------|---------|
| Wrong direction expansion | AVENUE → AVE (should expand, not abbreviate) |
| Case inconsistency | BUNTINGS → BUNtings |
| Spurious additions | Adding words not in original |

The core matching engine provides better results without LLM intervention.

## 5.16 Token Overlap Calculation

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
    minLen := min(len(tokens1), len(tokens2))

    return float64(overlap) / float64(minLen)
}
```

## 5.17 Normalisation Quality Metrics

The normalisation process is evaluated on:

| Metric | Description | Target |
|--------|-------------|--------|
| **Consistency** | Same input produces same output | 100% |
| **Reversibility** | Original preserved alongside canonical | Yes |
| **Token Preservation** | Important components retained | >95% |
| **Matching Effectiveness** | Normalised forms improve match rates | +15% |
| **Processing Speed** | Addresses normalised per second | >10,000/s |

## 5.18 Complete Data Flow

```
Raw Address Input
         │
         ▼
┌─────────────────────────────────────┐
│  Stage 1: Basic Normalisation       │
│  - Uppercase + Trim                 │
│  - Extract postcode                 │
│  - Remove punctuation               │
│  - Expand abbreviations (37 rules)  │
│  - Handle descriptors               │
└─────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────┐
│  Stage 2: Enhanced Normalisation    │
│  - Extended abbreviations (60+)     │
│  - Remove noise words               │
│  - Normalise business names         │
│  - Clean punctuation                │
└─────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────┐
│  Stage 3: Component Extraction      │
│  - House numbers                    │
│  - Flat/unit identifiers            │
│  - Street name tokens               │
│  - Locality tokens                  │
└─────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────┐
│  Stage 4: Phonetic Encoding         │
│  - Double Metaphone codes           │
│  - Token phonetic overlap           │
└─────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────┐
│  Stage 5: Validation                │
│  - Component confidence scoring     │
│  - Vague address detection          │
│  - Matching suitability flag        │
└─────────────────────────────────────┘
         │
         ▼
    Canonical Address
    + Postcode
    + Tokens
    + Components
    + Phonetic Codes
    + Confidence Score
```

## 5.19 Chapter Summary

This chapter has documented the comprehensive address normalisation subsystem:

- **Core canonicalisation** with uppercase, postcode extraction, punctuation removal
- **97+ abbreviation rules** across basic and enhanced normalisation
- **Descriptor handling** for UK property relationships
- **House number extraction** supporting various UK formats
- **32 Hampshire locality tokens** for geographic recognition
- **Street token extraction** excluding noise words
- **Phonetic encoding** using Double Metaphone with 17 substitution rules
- **Component extraction** into structured address parts
- **UK address parsing** with confidence scoring
- **Database-level normalisation** via SQL functions
- **Experimental LLM correction** (currently disabled)

The multi-stage normalisation pipeline ensures consistent, high-quality canonical addresses that maximise matching effectiveness.

---

*This chapter establishes normalisation as the foundation for matching. Chapter 6 describes the matching algorithms in detail.*
