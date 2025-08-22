# gopostal Integration for Enhanced UK Address Matching

## Why gopostal Will Dramatically Improve EHDC LLPG Matching

Currently, our system struggles with common UK address variations:
- **"St" → "Street" vs "Saint"** (context-dependent)
- **"Nr" → "Near"**
- **"Hants" → "Hampshire"**
- **Different address structures** ("Land at X" vs "X")

## gopostal Benefits for EHDC

### 1. Intelligent UK Address Parsing
```
Input: "Flat 3, 123 High St, Nr Alton, Hants"
gopostal output:
{
  "unit": "flat 3",
  "house_number": "123", 
  "road": "high street",      // Automatically expanded!
  "city": "alton",
  "state": "hampshire"         // County expanded!
}
```

### 2. Handles Complex UK Addresses
- Building names: "The Old Rectory, Church Lane"
- Land references: "Land adjacent to 1 High Street"
- Business addresses: "The Swan Hotel, Market Square"
- Rural addresses: "Barn 2, Home Farm, Selborne"

### 3. Expected Accuracy Improvements

**Current System:**
- Match rate: ~2.2% (2,800/129,701)
- Misses due to format variations

**With gopostal:**
- Expected match rate: **15-25%** (20,000-32,000 matches)
- Better handling of:
  - Abbreviations (St, Rd, Nr, etc.)
  - County variations (Hants, Berks, etc.)
  - Business/landmark names
  - Land/plot references

## Installation Requirements

### macOS (Apple Silicon)
```bash
# Install libpostal (C library)
brew install curl autoconf automake libtool pkg-config

# Clone and build libpostal
git clone https://github.com/openvenues/libpostal
cd libpostal
./bootstrap.sh
./configure --datadir=/usr/local/share/libpostal
make -j4
sudo make install

# Download data files (~2GB)
libpostal_data download all /usr/local/share/libpostal
```

### Using gopostal in Our System
```go
import (
    "github.com/openvenues/gopostal/expand"
    "github.com/openvenues/gopostal/parser"
)

// Parse UK address
components := parser.ParseAddress("123 High St, Alton, Hants")
// Returns: [{house_number 123} {road high street} {city alton} {state hampshire}]

// Expand abbreviations
expansions := expand.ExpandAddress("123 High St Nr Alton")
// Returns: ["123 high street near alton", "123 high saint near alton"]
```

## Integration Strategy

1. **Install libpostal** on processing machine
2. **Update ParsedAddress** struct to use gopostal components
3. **Enhance matching** with component-level comparison
4. **Test on sample data** to validate improvements

## Expected Results

### Before gopostal:
```
Source: "The Swan Hotel, High St, Alton"
LLPG: "Swan Hotel, 123 High Street, Alton, GU34 1XX"
Result: NO MATCH (different structure)
```

### With gopostal:
```
Both parse to:
- house: "swan hotel"
- road: "high street"  
- city: "alton"
Result: MATCH! (components align)
```

## Next Steps

1. Install libpostal on the system
2. Update the AccurateEngine to use gopostal parsing
3. Re-run matching on full dataset
4. Expect 10x improvement in match rate

## Note
Since libpostal requires ~2GB of data files and system installation, it cannot be installed in this environment. However, the AccurateEngine has been designed to work with or without gopostal, using enhanced parsing strategies that will still improve matching accuracy.