package llpg

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RangeExpander handles the expansion of LLPG range addresses into individual addresses
type RangeExpander struct {
	db *sql.DB
}

// NewRangeExpander creates a new LLPG range expander
func NewRangeExpander(db *sql.DB) *RangeExpander {
	return &RangeExpander{db: db}
}

// ExpandedAddress represents an expanded LLPG address
type ExpandedAddress struct {
	OriginalAddressID int
	UPRN             string
	FullAddress      string
	AddressCanonical string
	ExpansionType    string
	UnitNumber       string
}

// InitializeExpandedTable creates the expanded address table if it doesn't exist
func (re *RangeExpander) InitializeExpandedTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS dim_address_expanded (
		expanded_id SERIAL PRIMARY KEY,
		original_address_id INTEGER REFERENCES dim_address(address_id),
		uprn TEXT,
		full_address TEXT,
		address_canonical TEXT,
		expansion_type TEXT,
		unit_number TEXT,
		created_at TIMESTAMP DEFAULT NOW()
	);
	
	CREATE INDEX IF NOT EXISTS idx_address_expanded_uprn ON dim_address_expanded(uprn);
	CREATE INDEX IF NOT EXISTS idx_address_expanded_canonical ON dim_address_expanded(address_canonical);
	CREATE INDEX IF NOT EXISTS idx_address_expanded_unit ON dim_address_expanded(unit_number);
	`
	
	_, err := re.db.Exec(query)
	return err
}

// ExpandAllRanges processes all LLPG addresses and expands ranges (Option A: only expanded addresses)
func (re *RangeExpander) ExpandAllRanges() (int, error) {
	// Clear existing expansions
	if err := re.clearExpansions(); err != nil {
		return 0, fmt.Errorf("failed to clear existing expansions: %v", err)
	}
	
	// Process property ranges with proper validation
	count, err := re.expandPropertyRanges()
	if err != nil {
		return 0, fmt.Errorf("failed to expand property ranges: %v", err)
	}
	
	return count, nil
}

// clearExpansions removes all previous expansions
func (re *RangeExpander) clearExpansions() error {
	_, err := re.db.Exec("TRUNCATE dim_address_expanded")
	return err
}

// expandPropertyRanges expands all types of property number ranges with proper validation
func (re *RangeExpander) expandPropertyRanges() (int, error) {
	// Pattern matches property numbers with optional letters and spaces: 9-11, 9A-9C, 9 - 11
	propertyRangePattern := regexp.MustCompile(`\b(\d+[A-Z]?)\s*-\s*(\d+[A-Z]?)\b`)
	
	// Query addresses with property ranges 
	query := `
	SELECT address_id, uprn, full_address, address_canonical
	FROM dim_address 
	WHERE full_address ~ '\m\d+[A-Z]?\s*-\s*\d+[A-Z]?\M'
	`
	
	rows, err := re.db.Query(query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	
	expandedCount := 0
	
	for rows.Next() {
		var addressID int
		var uprn, fullAddress, canonical string
		
		if err := rows.Scan(&addressID, &uprn, &fullAddress, &canonical); err != nil {
			continue
		}
		
		// Find all property ranges in the address
		matches := propertyRangePattern.FindAllStringSubmatch(fullAddress, -1)
		
		for _, match := range matches {
			startProp := strings.TrimSpace(match[1])
			endProp := strings.TrimSpace(match[2])
			
			// Validate this is a property number range
			if !re.isValidPropertyRange(startProp, endProp, fullAddress) {
				continue
			}
			
			// Generate individual addresses for property range
			expanded := re.generatePropertyRange(startProp, endProp)
			for _, propNum := range expanded {
				// Replace the range with the individual property number
				newAddress := strings.Replace(fullAddress, match[0], propNum, 1)
				newCanonical := strings.Replace(canonical, match[0], propNum, 1)
				
				// Handle concatenated canonical addresses ("10-11" -> "1011")
				concatenated := regexp.MustCompile(`\s`).ReplaceAllString(match[1], "") + regexp.MustCompile(`\s`).ReplaceAllString(match[2], "")
				newCanonical = strings.Replace(newCanonical, concatenated, propNum, 1)
				
				if err := re.insertExpanded(addressID, uprn, newAddress, newCanonical, "range_expansion", propNum); err != nil {
					continue
				}
				expandedCount++
			}
		}
	}
	
	return expandedCount, nil
}

// isValidPropertyRange validates if a range represents property numbers using your rules
func (re *RangeExpander) isValidPropertyRange(start, end, address string) bool {
	// Extract numeric parts for validation
	startNum := regexp.MustCompile(`^(\d+)`).FindString(start)
	endNum := regexp.MustCompile(`^(\d+)`).FindString(end)
	
	if startNum == "" || endNum == "" {
		return false
	}
	
	startInt, err1 := strconv.Atoi(startNum)
	endInt, err2 := strconv.Atoi(endNum)
	
	if err1 != nil || err2 != nil {
		return false
	}
	
	// Extract letter suffixes for additional validation
	startSuffix := strings.TrimPrefix(start, startNum)
	endSuffix := strings.TrimPrefix(end, endNum)
	
	// Property number validation rules:
	// 1. For pure numeric ranges: start must be less than end
	// 2. For letter ranges (9A-9C): same number allowed if different letters
	// 3. Range should not be too large (max 50 units for aggressive expansion)
	// 4. Numbers should be reasonable property numbers (1-9999)
	
	// Check if this is a letter range (same number, different letters)
	isLetterRange := startInt == endInt && len(startSuffix) == 1 && len(endSuffix) == 1 && startSuffix != endSuffix
	
	if !isLetterRange && startInt >= endInt {
		return false // Pure numeric ranges need start < end
	}
	
	// More aggressive: allow up to 50 units (e.g., 47-93)
	if (endInt-startInt) > 50 || startInt < 1 || endInt > 9999 {
		return false
	}
	
	// More aggressive: Always expand ranges in addresses (remove context restrictions)
	// This will over-expand but that's what we want for comprehensive matching
	return true
}

// generatePropertyRange generates individual property numbers from a range  
func (re *RangeExpander) generatePropertyRange(start, end string) []string {
	var result []string
	
	// Handle pure numeric ranges (9-11) and letter ranges (9A-9C)
	startNum := regexp.MustCompile(`^(\d+)`).FindString(start)
	endNum := regexp.MustCompile(`^(\d+)`).FindString(end)
	startSuffix := strings.TrimPrefix(start, startNum)
	endSuffix := strings.TrimPrefix(end, endNum)
	
	startInt, _ := strconv.Atoi(startNum)
	endInt, _ := strconv.Atoi(endNum)
	
	// If both have same number with letter suffix (9A-9C), generate letter range
	if len(startSuffix) == 1 && len(endSuffix) == 1 && startNum == endNum && startSuffix[0] <= endSuffix[0] {
		// Letter range: 9A-9C -> 9A, 9B, 9C
		for c := startSuffix[0]; c <= endSuffix[0]; c++ {
			result = append(result, startNum+string(c))
		}
	} else {
		// Numeric range: 9-11 -> 9, 10, 11
		for i := startInt; i <= endInt; i++ {
			result = append(result, strconv.Itoa(i)+startSuffix)
		}
	}
	
	return result
}

// insertExpanded inserts an expanded address into the database
func (re *RangeExpander) insertExpanded(originalID int, uprn, fullAddress, canonical, expansionType, unitNumber string) error {
	query := `
	INSERT INTO dim_address_expanded (
		original_address_id, uprn, full_address, address_canonical, 
		expansion_type, unit_number, created_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	
	_, err := re.db.Exec(query, originalID, uprn, fullAddress, canonical, expansionType, unitNumber, time.Now())
	return err
}

// GetExpandedAddressStats returns statistics about the expansion
func (re *RangeExpander) GetExpandedAddressStats() (map[string]int, error) {
	query := `
	SELECT expansion_type, COUNT(*) 
	FROM dim_address_expanded
	GROUP BY expansion_type
	`
	
	rows, err := re.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	stats := make(map[string]int)
	for rows.Next() {
		var expansionType string
		var count int
		if err := rows.Scan(&expansionType, &count); err != nil {
			continue
		}
		stats[expansionType] = count
	}
	
	return stats, nil
}