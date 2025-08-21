# EHDC LLPG Address Matching - Comprehensive Fix Plan

## Executive Summary

This document outlines a comprehensive plan to fix critical issues in the EHDC LLPG address matching system. The primary focus is eliminating false positive matches while implementing conservative, high-precision matching logic.

## Current Critical Issues

### **Issue 1: False Positive House Number Matches**
- **Problem**: "168 Station Road, Liss" matches to "147 Station Road, Liss" (76% similarity)
- **Impact**: Wrong house numbers create serious data quality issues
- **Root Cause**: Fuzzy matching prioritizes street similarity without validating house numbers

### **Issue 2: False Positive Unit Number Matches**  
- **Problem**: "Unit 10, Mill Lane, Alton" matches to "Unit 7, 4 Mill Lane, Alton" (81% similarity)
- **Impact**: Wrong unit numbers in commercial/industrial addresses
- **Root Cause**: No validation that unit numbers must match exactly

### **Issue 3: Low Quality Control Standards**
- **Problem**: System accepts matches with different critical identifiers based purely on string similarity
- **Impact**: High confidence in wrong matches corrupts data integrity
- **Root Cause**: Insufficient component-level validation

### **Issue 4: Planning App 20026 Still Failing**
- **Problem**: Documents like "UNIT 2, AMEY INDUSTRIAL EST" still show 0.0000 confidence despite 84% similarity to correct targets
- **Impact**: Missing obvious matches due to rigid thresholds combined with poor validation
- **Root Cause**: System rejects good matches while accepting bad ones

## Root Cause Analysis

### **Architectural Problems**
1. **String-based matching**: Treats addresses as opaque text rather than structured components
2. **No component validation**: House/unit numbers not validated separately from street names  
3. **Inappropriate thresholds**: Too lenient for different identifiers, too strict for good matches
4. **Poor gopostal integration**: UK industrial estate formats not handled properly

### **Quality Control Gaps**
1. **No pre-match validation**: Components not extracted and validated before matching
2. **No post-match verification**: No checks that critical identifiers match
3. **Inadequate confidence scoring**: High similarity scores for wrong matches
4. **Missing rejection criteria**: No automatic rejection for mismatched house numbers

## Comprehensive Fix Plan

### **Phase 1: Conservative Matching Framework**

#### **1.1: Address Component Extraction**
Implement structured address parsing:

```go
type AddressComponents struct {
    HouseNumber  string  // "168", "Unit 2", "5A", "Flat B"
    Street       string  // "Station Road", "Mill Lane", "Amey Industrial Estate"  
    Locality     string  // "Alton", "Petersfield", "Liss"
    Postcode     string  // "GU34 2QG", "GU32 3AN"
    Raw          string  // Original unparsed address
}

func extractAddressComponents(address string) AddressComponents {
    // Enhanced gopostal parsing with UK-specific rules
    // Handle "Unit X", "Flat Y", industrial estates
    // Extract postcodes with validation
    // Normalize case and punctuation
}
```

#### **1.2: Strict Component Validation Rules**

**House/Unit Number Validation**:
- House numbers must match exactly: "168" ≠ "147" 
- Unit numbers must match exactly: "Unit 10" ≠ "Unit 7"
- Allow case variations: "Unit 2" = "UNIT 2"
- Allow punctuation variations: "5A" = "5a"
- **Zero tolerance for different numbers**

**Street Name Validation**:
- Allow fuzzy matching only after house number validation passes
- Minimum 90% similarity for street names (increased from 70%)
- Handle common abbreviations: "EST" = "ESTATE", "RD" = "ROAD"
- Special handling for industrial estates

**Postcode Validation**:
- Exact match preferred
- Same postcode district acceptable (GU34 2QG ≈ GU34 2QF)
- Different districts require manual review

#### **1.3: Multi-Level Matching Strategy**

```
Level 1 - EXACT MATCH (Auto-Accept, Confidence: 1.0)
├── House number + street + postcode exactly match
└── Example: "168 Station Road, GU34 2QG" → "168 Station Road, Liss, GU34 2QG"

Level 2 - VALIDATED FUZZY (Auto-Accept, Confidence: 0.9-0.99)  
├── House number exact + street fuzzy (>90%) + postcode district match
└── Example: "Unit 2, Amey Industrial Est, GU32" → "Unit, 2 Amey Industrial Estate, GU32 3AN"

Level 3 - PARTIAL MATCH (Manual Review, Confidence: 0.7-0.89)
├── House number exact + street exact + different postcode district  
└── Example: "168 Station Road, GU34" → "168 Station Road, GU33"

Level 4 - NO MATCH (Reject, Confidence: 0.0)
├── Any house/unit number mismatch
├── Street similarity < 90%
└── Example: "168 Station Road" → "147 Station Road" (REJECT)
```

### **Phase 2: Enhanced Validation Logic**

#### **2.1: House Number Validation Function**

```go
func validateHouseNumbers(source, target AddressComponents) ValidationResult {
    sourceNum := normalizeHouseNumber(source.HouseNumber)
    targetNum := normalizeHouseNumber(target.HouseNumber)
    
    // Exact match (case-insensitive)
    if strings.EqualFold(sourceNum, targetNum) {
        return ValidationResult{Valid: true, Confidence: 1.0}
    }
    
    // Handle common variations
    variations := []string{
        // "Unit 2" vs "Unit, 2"
        strings.ReplaceAll(sourceNum, ",", ""),
        strings.ReplaceAll(sourceNum, " ", ""),
    }
    
    for _, variation := range variations {
        if strings.EqualFold(variation, targetNum) {
            return ValidationResult{Valid: true, Confidence: 0.95}
        }
    }
    
    // Different numbers = automatic rejection
    return ValidationResult{
        Valid: false, 
        Confidence: 0.0,
        Reason: fmt.Sprintf("House number mismatch: '%s' vs '%s'", sourceNum, targetNum),
    }
}
```

#### **2.2: Conservative Similarity Thresholds**

**New Thresholds** (much stricter):
- **House number match**: Required (was: ignored)
- **Street similarity**: >0.90 (was: 0.70)  
- **Overall similarity**: >0.95 for auto-accept (was: 0.70)
- **Edit distance**: <5 characters (was: <20)
- **Component validation**: All must pass

**Automatic Rejection Criteria**:
```go
func shouldRejectMatch(source, target AddressComponents, similarity float64) (bool, string) {
    // House/unit number mismatch = immediate rejection
    if !validateHouseNumbers(source, target).Valid {
        return true, "House number mismatch"
    }
    
    // High edit distance = likely different addresses
    if levenshtein(source.Raw, target.Raw) > 5 {
        return true, "Too many character differences"
    }
    
    // Low similarity = insufficient match quality
    if similarity < 0.90 {
        return true, "Insufficient overall similarity"
    }
    
    return false, ""
}
```

#### **2.3: Enhanced Confidence Scoring**

```go
func calculateConservativeConfidence(match AddressMatch) float64 {
    components := extractComponents(match.Source, match.Target)
    
    // Start with base similarity
    confidence := match.SimilarityScore
    
    // House number validation (mandatory)
    houseValidation := validateHouseNumbers(components.Source, components.Target)
    if !houseValidation.Valid {
        return 0.0  // Automatic rejection
    }
    confidence *= houseValidation.Confidence
    
    // Street name validation
    streetSimilarity := calculateStreetSimilarity(components.Source.Street, components.Target.Street)
    if streetSimilarity < 0.90 {
        return 0.0  // Reject poor street matches
    }
    confidence *= streetSimilarity
    
    // Postcode validation
    postcodeMatch := validatePostcodes(components.Source.Postcode, components.Target.Postcode)
    confidence *= postcodeMatch.Confidence
    
    // Conservative threshold - only accept very high quality matches
    if confidence < 0.95 {
        return 0.0  // Send to manual review instead
    }
    
    return confidence
}
```

### **Phase 3: Quality Assurance Framework**

#### **3.1: Pre-Match Validation**

```go
func validateAddressForMatching(address string) AddressValidation {
    components := extractAddressComponents(address)
    
    validation := AddressValidation{
        Address: address,
        Components: components,
        Issues: []string{},
        Suitable: true,
    }
    
    // Check for missing house number
    if components.HouseNumber == "" {
        validation.Issues = append(validation.Issues, "No house number identified")
        validation.Suitable = false
    }
    
    // Check for valid street name
    if len(components.Street) < 3 {
        validation.Issues = append(validation.Issues, "Street name too short or missing")
        validation.Suitable = false
    }
    
    // Check postcode format (UK-specific)
    if !isValidUKPostcode(components.Postcode) {
        validation.Issues = append(validation.Issues, "Invalid UK postcode format")
    }
    
    return validation
}
```

#### **3.2: Post-Match Quality Checks**

```go
func auditMatchQuality(match AddressMatch) MatchAudit {
    audit := MatchAudit{
        MatchID: match.ID,
        Timestamp: time.Now(),
        Checks: []QualityCheck{},
    }
    
    // House number consistency check
    sourceHouse := extractHouseNumber(match.Source)
    targetHouse := extractHouseNumber(match.Target)
    
    audit.Checks = append(audit.Checks, QualityCheck{
        Name: "House Number Match",
        Passed: sourceHouse == targetHouse,
        Details: fmt.Sprintf("Source: '%s', Target: '%s'", sourceHouse, targetHouse),
    })
    
    // Geographic consistency
    sourcePostcode := extractPostcode(match.Source)
    targetPostcode := extractPostcode(match.Target)
    
    audit.Checks = append(audit.Checks, QualityCheck{
        Name: "Postcode District Match", 
        Passed: samePostcodeDistrict(sourcePostcode, targetPostcode),
        Details: fmt.Sprintf("Source: '%s', Target: '%s'", sourcePostcode, targetPostcode),
    })
    
    // Calculate overall audit score
    passed := 0
    for _, check := range audit.Checks {
        if check.Passed {
            passed++
        }
    }
    audit.Score = float64(passed) / float64(len(audit.Checks))
    
    return audit
}
```

#### **3.3: Match Decision Framework**

```go
type MatchDecision struct {
    Accept      bool
    Confidence  float64
    Method      string
    Reason      string
    RequiresReview bool
}

func makeMatchDecision(source, target string, similarity float64) MatchDecision {
    sourceComp := extractAddressComponents(source)
    targetComp := extractAddressComponents(target)
    
    // Pre-validation
    sourceValid := validateAddressForMatching(source)
    targetValid := validateAddressForMatching(target)
    
    if !sourceValid.Suitable || !targetValid.Suitable {
        return MatchDecision{
            Accept: false,
            Confidence: 0.0,
            Method: "Pre-validation failed",
            Reason: "Address components could not be reliably extracted",
            RequiresReview: true,
        }
    }
    
    // Conservative matching logic
    confidence := calculateConservativeConfidence(source, target, similarity)
    
    if confidence >= 0.95 {
        return MatchDecision{
            Accept: true,
            Confidence: confidence,
            Method: "Conservative Auto-Match",
            Reason: "All components validated with high confidence",
            RequiresReview: false,
        }
    } else if confidence >= 0.70 {
        return MatchDecision{
            Accept: false,
            Confidence: confidence,
            Method: "Manual Review Required", 
            Reason: "Good match but below auto-accept threshold",
            RequiresReview: true,
        }
    } else {
        return MatchDecision{
            Accept: false,
            Confidence: confidence,
            Method: "Rejected",
            Reason: "Insufficient match quality or component validation failed",
            RequiresReview: false,
        }
    }
}
```

### **Phase 4: Implementation Strategy**

#### **4.1: Database Schema Updates**

```sql
-- Enhanced match tracking
ALTER TABLE address_match_corrected ADD COLUMN IF NOT EXISTS (
    source_house_number TEXT,
    target_house_number TEXT,
    house_number_match BOOLEAN,
    source_street TEXT,
    target_street TEXT, 
    street_similarity FLOAT,
    source_postcode TEXT,
    target_postcode TEXT,
    postcode_match BOOLEAN,
    validation_flags TEXT[],
    component_confidence FLOAT,
    audit_score FLOAT,
    requires_review BOOLEAN DEFAULT FALSE
);

-- Match audit log
CREATE TABLE IF NOT EXISTS match_audit_log (
    audit_id SERIAL PRIMARY KEY,
    match_id INTEGER REFERENCES address_match_corrected(document_id),
    audit_timestamp TIMESTAMP DEFAULT NOW(),
    quality_checks JSONB,
    audit_score FLOAT,
    issues_found TEXT[]
);

-- Address validation cache
CREATE TABLE IF NOT EXISTS address_component_cache (
    address_hash TEXT PRIMARY KEY,
    raw_address TEXT,
    house_number TEXT,
    street TEXT,
    locality TEXT,
    postcode TEXT,
    validation_issues TEXT[],
    extraction_confidence FLOAT,
    created_at TIMESTAMP DEFAULT NOW()
);
```

#### **4.2: New Matching Commands**

```bash
# Address component validation
./matcher-v4 -cmd=validate-address-parsing
./matcher-v4 -cmd=audit-existing-matches  
./matcher-v4 -cmd=identify-false-positives

# Conservative matching
./matcher-v4 -cmd=conservative-fuzzy-match
./matcher-v4 -cmd=component-based-match
./matcher-v4 -cmd=manual-review-queue

# Quality assurance
./matcher-v4 -cmd=match-quality-report
./matcher-v4 -cmd=precision-analysis
./matcher-v4 -cmd=validation-statistics
```

#### **4.3: Enhanced Address Parser**

Focus on improving gopostal/libpostal integration:

```go
// UK-specific address parsing improvements
func parseUKAddress(address string) AddressComponents {
    // Pre-processing for UK formats
    cleaned := preprocessUKAddress(address)
    
    // Enhanced gopostal parsing
    parsed := gopostal.ParseAddress(cleaned)
    
    // Post-processing for UK-specific patterns
    components := postprocessUKComponents(parsed)
    
    // Validation and confidence scoring
    components.Confidence = validateComponentExtraction(address, components)
    
    return components
}

func preprocessUKAddress(address string) string {
    // Handle UK-specific patterns before gopostal
    address = regexp.MustCompile(`\bEST\b`).ReplaceAllString(address, "ESTATE")
    address = regexp.MustCompile(`\bFLAT (\d+[A-Z]?)\b`).ReplaceAllString(address, "FLAT $1")
    address = regexp.MustCompile(`\bUNIT (\d+[A-Z]?)\b`).ReplaceAllString(address, "UNIT $1")
    
    // Normalize spacing and punctuation
    address = regexp.MustCompile(`\s+`).ReplaceAllString(address, " ")
    address = strings.TrimSpace(address)
    
    return address
}
```

### **Phase 5: Testing & Validation Plan**

#### **5.1: Critical Test Cases**

**False Positive Prevention**:
```
✗ "168 Station Road, Liss" → "147 Station Road, Liss" (house number mismatch)
✗ "Unit 10, Mill Lane" → "Unit 7, Mill Lane" (unit number mismatch)  
✗ "5A Thorpe Gardens" → "5B Thorpe Gardens" (flat letter mismatch)
```

**True Positive Validation**:
```
✓ "Unit 2, Amey Industrial Est" → "Unit, 2 Amey Industrial Estate" (punctuation variation)
✓ "168 Station Rd" → "168 Station Road" (abbreviation expansion)
✓ "Flat A, 12 High St" → "12A High Street" (format variation)
```

**Edge Cases**:
```
? "The Old Brewery, Frenchmans Road" → ? (no house number - manual review)
? "Land at Amey Industrial Estate" → ? (vague address - manual review)
```

#### **5.2: Quality Metrics & Targets**

**Precision Targets**:
- **House Number Accuracy**: 100% (zero tolerance for wrong numbers)
- **Auto-Accept Precision**: >98% (less than 2% false positives)
- **Component Extraction**: >95% accuracy for house numbers and street names

**Coverage Expectations**:
- **Auto-Accept Rate**: 30-50% (conservative, high-precision)
- **Manual Review Queue**: 20-30% (medium confidence matches)
- **Rejection Rate**: 20-50% (prefer no match over wrong match)

#### **5.3: Implementation Phases**

**Phase A: Component Extraction (Week 1)**
- Implement enhanced address parsing
- Create component validation functions
- Build test suite for parsing accuracy

**Phase B: Conservative Matching (Week 2)**  
- Implement strict validation rules
- Create new matching algorithms
- Test against known false positives

**Phase C: Quality Assurance (Week 3)**
- Add audit logging and quality checks
- Implement manual review queue
- Create quality reporting dashboard

**Phase D: Full Integration (Week 4)**
- Integrate all components
- Run comprehensive testing
- Deploy with monitoring

### **Phase 6: Monitoring & Quality Control**

#### **6.1: Continuous Quality Monitoring**

```sql
-- Daily quality metrics
SELECT 
    DATE(created_at) as match_date,
    COUNT(*) as total_matches,
    COUNT(*) FILTER (WHERE confidence >= 0.95) as high_confidence,
    COUNT(*) FILTER (WHERE house_number_match = false) as house_mismatch,
    COUNT(*) FILTER (WHERE requires_review = true) as manual_review,
    AVG(audit_score) as avg_audit_score
FROM address_match_corrected 
WHERE created_at >= CURRENT_DATE - INTERVAL '7 days'
GROUP BY DATE(created_at)
ORDER BY match_date DESC;
```

#### **6.2: Alert System**

Monitor for quality degradation:
- House number mismatch rate > 1%
- Average audit score < 0.90  
- False positive reports from users
- Confidence score distribution anomalies

#### **6.3: Continuous Improvement Process**

1. **Weekly Quality Reviews**: Analyze manual review queue patterns
2. **Monthly Precision Audits**: Sample validation of auto-accepted matches  
3. **Quarterly Algorithm Updates**: Refine based on quality patterns
4. **User Feedback Integration**: Address reported false positives immediately

## Expected Outcomes

### **Immediate (Post-Implementation)**
- **Zero house number false positives**: Eliminate "168→147" type errors
- **Higher precision**: >98% accuracy on auto-accepted matches
- **Clear audit trails**: Full visibility into matching decisions

### **Short Term (1-3 months)**
- **User confidence**: Trust in automated matching results
- **Reduced corrections**: Fewer manual fixes needed
- **Quality stability**: Consistent precision metrics

### **Long Term (3-12 months)**  
- **Scalable quality**: Foundation for expanding automated matching
- **Data integrity**: Reliable address matching across all datasets
- **Process optimization**: Refined algorithms based on real-world performance

## Risk Mitigation

### **Coverage Reduction Risk**
- **Risk**: Conservative approach may reduce match coverage
- **Mitigation**: Better no match than wrong match; build manual review process
- **Monitoring**: Track coverage trends, optimize thresholds carefully

### **Performance Impact Risk**
- **Risk**: Component validation may slow matching process
- **Mitigation**: Implement caching, optimize parsing algorithms
- **Monitoring**: Track processing times, optimize bottlenecks

### **Implementation Complexity Risk**
- **Risk**: Enhanced validation logic increases system complexity
- **Mitigation**: Comprehensive testing, phased rollout, clear documentation
- **Monitoring**: Code quality metrics, bug tracking, performance monitoring

## Success Criteria

### **Must Achieve**
1. **Zero house number false positives** in production
2. **>98% precision** on auto-accepted matches
3. **Complete audit trail** for all matching decisions
4. **Reliable rejection** of known false positive patterns

### **Should Achieve**
1. **30-50% auto-accept rate** with high precision
2. **Efficient manual review queue** for medium-confidence matches
3. **Improved industrial estate matching** (planning app 20026 type cases)
4. **User confidence** in automated results

### **Could Achieve**
1. **Enhanced coverage** through better component parsing
2. **Predictive quality scoring** for match reliability
3. **Automated quality monitoring** with alerting
4. **Integration with external validation sources**

---

## Conclusion

This comprehensive fix plan addresses the root causes of false positive matching while implementing a conservative, high-precision approach to address matching. The key principle is **"better no match than wrong match"** - prioritizing data quality and user trust over coverage statistics.

The phased implementation allows for careful testing and validation at each stage, ensuring the system maintains reliability while gaining enhanced capabilities. The enhanced monitoring and quality assurance framework provides ongoing confidence in system performance and early detection of any quality degradation.

Success will be measured not by the number of matches made, but by the precision and reliability of those matches, building a foundation for long-term trust and expanded automation in the EHDC LLPG address matching system.