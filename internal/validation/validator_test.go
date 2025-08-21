package validation

import (
	"strings"
	"testing"
)

func TestValidateHouseNumbers(t *testing.T) {
	validator := NewAddressValidator()
	
	tests := []struct {
		name           string
		source         string
		target         string
		expectedValid  bool
		expectedReason string
		description    string
	}{
		{
			name:           "Exact Match",
			source:         "168",
			target:         "168",
			expectedValid:  true,
			expectedReason: "Exact house number match",
			description:    "Identical house numbers should match",
		},
		{
			name:           "Different Numbers - Critical Failure",
			source:         "168",
			target:         "147",
			expectedValid:  false,
			expectedReason: "House number mismatch: '168' ≠ '147'",
			description:    "Different house numbers must be rejected (fixes false positive issue)",
		},
		{
			name:           "Unit Number Mismatch",
			source:         "UNIT 10",
			target:         "UNIT 7",
			expectedValid:  false,
			expectedReason: "House number mismatch: 'UNIT 10' ≠ 'UNIT 7'",
			description:    "Different unit numbers must be rejected (fixes false positive issue)",
		},
		{
			name:           "Unit Punctuation Variations",
			source:         "UNIT 2",
			target:         "UNIT, 2",
			expectedValid:  true,
			expectedReason: "House number variation match",
			description:    "Punctuation variations should be accepted",
		},
		{
			name:           "Case Insensitive Match",
			source:         "unit 2",
			target:         "UNIT 2",
			expectedValid:  true,
			expectedReason: "Exact house number match",
			description:    "Case differences should not matter",
		},
		{
			name:           "Flat Number Match",
			source:         "FLAT A",
			target:         "FLAT A",
			expectedValid:  true,
			expectedReason: "Exact house number match",
			description:    "Flat identifiers should match exactly",
		},
		{
			name:           "Flat Letter Mismatch",
			source:         "FLAT A",
			target:         "FLAT B",
			expectedValid:  false,
			expectedReason: "House number mismatch: 'FLAT A' ≠ 'FLAT B'",
			description:    "Different flat letters must be rejected",
		},
		{
			name:           "Proximity Concern - Should Reject",
			source:         "168",
			target:         "169",
			expectedValid:  false,
			expectedReason: "House number mismatch with proximity concern",
			description:    "Close numbers should be flagged as potential data errors",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceComp := AddressComponents{HouseNumber: tt.source}
			targetComp := AddressComponents{HouseNumber: tt.target}
			
			result := validator.ValidateHouseNumbers(sourceComp, targetComp)
			
			if result.Valid != tt.expectedValid {
				t.Errorf("Expected valid=%v, got=%v for %s vs %s", 
					tt.expectedValid, result.Valid, tt.source, tt.target)
			}
			
			if !strings.Contains(result.Reason, tt.expectedReason) {
				t.Errorf("Expected reason to contain '%s', got '%s'", 
					tt.expectedReason, result.Reason)
			}
			
			t.Logf("%s: %s → %s", tt.description, tt.source, tt.target)
			t.Logf("Result: %s", result.String())
		})
	}
}

func TestValidateStreetNames(t *testing.T) {
	validator := NewAddressValidator()
	
	tests := []struct {
		name          string
		source        string
		target        string
		expectedValid bool
		description   string
	}{
		{
			name:          "Exact Street Match",
			source:        "STATION ROAD",
			target:        "STATION ROAD",
			expectedValid: true,
			description:   "Identical street names should match",
		},
		{
			name:          "Abbreviation Expansion",
			source:        "STATION RD",
			target:        "STATION ROAD",
			expectedValid: true,
			description:   "Abbreviation expansions should be accepted",
		},
		{
			name:          "Industrial Estate Match",
			source:        "AMEY INDUSTRIAL EST",
			target:        "AMEY INDUSTRIAL ESTATE",
			expectedValid: true,
			description:   "Industrial estate abbreviations should work",
		},
		{
			name:          "High Similarity Match",
			source:        "MILL LANE INDUSTRIAL ESTATE",
			target:        "MILL LANE INDUSTRIAL EST",
			expectedValid: true,
			description:   "High similarity streets should match",
		},
		{
			name:          "Different Streets - Should Reject",
			source:        "STATION ROAD",
			target:        "HIGH STREET",
			expectedValid: false,
			description:   "Completely different streets should be rejected",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceComp := AddressComponents{Street: tt.source}
			targetComp := AddressComponents{Street: tt.target}
			
			result := validator.ValidateStreetNames(sourceComp, targetComp)
			
			if result.Valid != tt.expectedValid {
				t.Errorf("Expected valid=%v, got=%v for %s vs %s", 
					tt.expectedValid, result.Valid, tt.source, tt.target)
			}
			
			t.Logf("%s: %s → %s", tt.description, tt.source, tt.target)
			t.Logf("Result: %s", result.String())
		})
	}
}

func TestMakeMatchDecision_CriticalCases(t *testing.T) {
	validator := NewAddressValidator()
	
	// Test cases based on real issues found in the system
	criticalTests := []struct {
		name           string
		sourceAddress  string
		targetAddress  string
		shouldAccept   bool
		shouldReview   bool
		description    string
	}{
		{
			name:          "False Positive Case 1 - Different House Numbers",
			sourceAddress: "168 STATION ROAD, LISS",
			targetAddress: "147 STATION ROAD, LISS",
			shouldAccept:  false,
			shouldReview:  false,
			description:   "Must reject different house numbers (was accepting with 76% similarity)",
		},
		{
			name:          "False Positive Case 2 - Different Unit Numbers",
			sourceAddress: "UNIT 10, MILL LANE, ALTON, GU34 2QG",
			targetAddress: "UNIT 7, 4 MILL LANE, ALTON, GU34 2QG",
			shouldAccept:  false,
			shouldReview:  false,
			description:   "Must reject different unit numbers (was accepting with 81% similarity)",
		},
		{
			name:          "True Positive Case - Unit Punctuation Variation",
			sourceAddress: "UNIT 2, AMEY INDUSTRIAL EST FRENCHMANS ROAD, PETERSFIELD, HANTS",
			targetAddress: "UNIT, 2 AMEY INDUSTRIAL ESTATE, FRENCHMANS ROAD, PETERSFIELD",
			shouldAccept:  true,
			shouldReview:  false,
			description:   "Should accept valid punctuation/abbreviation variations",
		},
		{
			name:          "True Positive Case - Abbreviation Expansion",
			sourceAddress: "168 STATION RD, LISS, GU33 7AA",
			targetAddress: "168 STATION ROAD, LISS, GU33 7AA",
			shouldAccept:  true,
			shouldReview:  false,
			description:   "Should accept abbreviation expansions with same house number",
		},
		{
			name:          "Edge Case - Proximity Numbers",
			sourceAddress: "168 STATION ROAD, LISS",
			targetAddress: "169 STATION ROAD, LISS",  
			shouldAccept:  false,
			shouldReview:  true,
			description:   "Should reject but flag for review (potential data entry error)",
		},
		{
			name:          "Vague Address - Should Reject",
			sourceAddress: "LAND AT AMEY INDUSTRIAL ESTATE",
			targetAddress: "UNIT 5, AMEY INDUSTRIAL ESTATE, FRENCHMANS ROAD",
			shouldAccept:  false,
			shouldReview:  true,
			description:   "Vague addresses should not auto-match",
		},
	}
	
	for _, tt := range criticalTests {
		t.Run(tt.name, func(t *testing.T) {
			decision := validator.MakeMatchDecision(tt.sourceAddress, tt.targetAddress)
			
			if decision.Accept != tt.shouldAccept {
				t.Errorf("Expected accept=%v, got=%v", tt.shouldAccept, decision.Accept)
			}
			
			if decision.RequiresReview != tt.shouldReview {
				t.Errorf("Expected review=%v, got=%v", tt.shouldReview, decision.RequiresReview)
			}
			
			t.Logf("=== %s ===", tt.name)
			t.Logf("Description: %s", tt.description)
			t.Logf("Source: %s", tt.sourceAddress)
			t.Logf("Target: %s", tt.targetAddress)
			t.Logf("Decision: %s", decision.String())
			t.Logf("House Number: %s", decision.ComponentValidation.HouseNumberMatch.String())
			t.Logf("Street: %s", decision.ComponentValidation.StreetMatch.String())
			t.Logf("Postcode: %s", decision.ComponentValidation.PostcodeMatch.String())
			t.Logf("Overall Score: %.3f", decision.ComponentValidation.OverallScore)
			t.Logf("")
		})
	}
}

func TestAddressParser_ComponentExtraction(t *testing.T) {
	parser := NewAddressParser()
	
	tests := []struct {
		name                string
		address            string
		expectedHouseNumber string
		expectedStreet      string
		expectedPostcode    string
		description         string
	}{
		{
			name:                "Simple Residential Address",
			address:            "168 Station Road, Liss, GU33 7AA",
			expectedHouseNumber: "168",
			expectedStreet:      "STATION ROAD",
			expectedPostcode:    "GU33 7AA",
			description:         "Basic residential address parsing",
		},
		{
			name:                "Unit Address",
			address:            "Unit 2, Amey Industrial Estate, Frenchmans Road, Petersfield",
			expectedHouseNumber: "UNIT 2",
			expectedStreet:      "AMEY INDUSTRIAL ESTATE",
			expectedPostcode:    "",
			description:         "Industrial unit address parsing",
		},
		{
			name:                "Abbreviated Address",
			address:            "168 Station Rd, Liss, Hants",
			expectedHouseNumber: "168",
			expectedStreet:      "STATION ROAD",
			expectedPostcode:    "",
			description:         "Abbreviation expansion should work",
		},
		{
			name:                "Flat Address",
			address:            "Flat A, 123 High Street, Alton, GU34 1AA",
			expectedHouseNumber: "FLAT A",
			expectedStreet:      "HIGH STREET",
			expectedPostcode:    "GU34 1AA",
			description:         "Flat identification should work",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			components := parser.ParseAddress(tt.address)
			
			if tt.expectedHouseNumber != "" && components.HouseNumber != tt.expectedHouseNumber {
				t.Errorf("Expected house number '%s', got '%s'", 
					tt.expectedHouseNumber, components.HouseNumber)
			}
			
			if tt.expectedStreet != "" && components.Street != tt.expectedStreet {
				t.Errorf("Expected street '%s', got '%s'", 
					tt.expectedStreet, components.Street)
			}
			
			if tt.expectedPostcode != "" && components.Postcode != tt.expectedPostcode {
				t.Errorf("Expected postcode '%s', got '%s'", 
					tt.expectedPostcode, components.Postcode)
			}
			
			t.Logf("=== %s ===", tt.name)
			t.Logf("Input: %s", tt.address)
			t.Logf("Parsed: %s", components.String())
			t.Logf("Confidence: %.2f", components.ExtractionConfidence)
			t.Logf("Valid for matching: %v", components.IsValidForMatching)
			if len(components.ValidationIssues) > 0 {
				t.Logf("Issues: %v", components.ValidationIssues)
			}
			t.Logf("")
		})
	}
}

func TestValidateAddressForMatching(t *testing.T) {
	parser := NewAddressParser()
	
	tests := []struct {
		name            string
		address         string
		shouldBeValid   bool
		expectedIssues  []string
		description     string
	}{
		{
			name:          "Valid Complete Address",
			address:       "168 Station Road, Liss, GU33 7AA",
			shouldBeValid: true,
			description:   "Complete address should be valid for matching",
		},
		{
			name:           "Missing House Number",
			address:        "Station Road, Liss, GU33 7AA",
			shouldBeValid:  false,
			expectedIssues: []string{"Missing house number"},
			description:    "Address without house number should be invalid",
		},
		{
			name:           "Vague Address",
			address:        "Land at Station Road, Liss",
			shouldBeValid:  false,
			expectedIssues: []string{"Vague address contains 'LAND AT'"},
			description:    "Vague addresses should be flagged as unsuitable",
		},
		{
			name:           "Adjacent Address",
			address:        "Rear of 123 High Street, Alton",
			shouldBeValid:  false,
			expectedIssues: []string{"Vague address contains 'REAR OF'"},
			description:    "Relative location addresses should be flagged",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation := parser.ValidateAddressForMatching(tt.address)
			
			if validation.Suitable != tt.shouldBeValid {
				t.Errorf("Expected suitable=%v, got=%v", tt.shouldBeValid, validation.Suitable)
			}
			
			for _, expectedIssue := range tt.expectedIssues {
				found := false
				for _, actualIssue := range validation.Issues {
					if strings.Contains(actualIssue, expectedIssue) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected issue containing '%s', but not found in: %v", 
						expectedIssue, validation.Issues)
				}
			}
			
			t.Logf("=== %s ===", tt.name)
			t.Logf("Address: %s", tt.address)
			t.Logf("Suitable: %v", validation.Suitable)
			t.Logf("Score: %.2f", validation.Score)
			if len(validation.Issues) > 0 {
				t.Logf("Issues: %v", validation.Issues)
			}
			t.Logf("")
		})
	}
}

// Benchmark tests for performance
func BenchmarkMakeMatchDecision(b *testing.B) {
	validator := NewAddressValidator()
	sourceAddr := "168 Station Road, Liss, GU33 7AA"
	targetAddr := "168 Station Road, Liss, GU33 7AA"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.MakeMatchDecision(sourceAddr, targetAddr)
	}
}

func BenchmarkParseAddress(b *testing.B) {
	parser := NewAddressParser()
	address := "Unit 2, Amey Industrial Estate, Frenchmans Road, Petersfield, GU32 3AN"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.ParseAddress(address)
	}
}