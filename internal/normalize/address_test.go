package normalize

import (
	"testing"
)

func TestCanonicalAddress(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantCanonical string
		wantPostcode  string
	}{
		{
			name:          "simple address with postcode",
			input:         "12 High Street, Alton, GU34 1AA",
			wantCanonical: "12 HIGH STREET ALTON",
			wantPostcode:  "GU34 1AA",
		},
		{
			name:          "address with abbreviations",
			input:         "Flat 3, 45 Church Rd, Petersfield, GU31 4HX",
			wantCanonical: "FLAT 3 45 CHURCH ROAD PETERSFIELD",
			wantPostcode:  "GU31 4HX",
		},
		{
			name:          "complex address from LLPG sample",
			input:         "Oakleigh, West Tisted Road, West Tisted, Alresford, SO24 0HJ",
			wantCanonical: "OAKLEIGH WEST TISTED ROAD WEST TISTED ALRESFORD",
			wantPostcode:  "SO24 0HJ",
		},
		{
			name:          "address without postcode",
			input:         "The Old Rectory, Church Lane, Selborne",
			wantCanonical: "THE OLD RECTORY CHURCH LANE SELBORNE",
			wantPostcode:  "",
		},
		{
			name:          "address with multiple abbreviations",
			input:         "2A St. James Gdns, Four Marks, Alton, GU34 5EZ",
			wantCanonical: "2A SAINT JAMES GARDENS FOUR MARKS ALTON",
			wantPostcode:  "GU34 5EZ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canonical, postcode, _ := CanonicalAddress(tt.input)
			
			if canonical != tt.wantCanonical {
				t.Errorf("CanonicalAddress() canonical = %v, want %v", canonical, tt.wantCanonical)
			}
			
			if postcode != tt.wantPostcode {
				t.Errorf("CanonicalAddress() postcode = %v, want %v", postcode, tt.wantPostcode)
			}
		})
	}
}

func TestExtractPostcode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"12 High Street, Alton, GU34 1AA", "GU34 1AA"},
		{"Flat 2, London, W1A 0AX", "W1A 0AX"},
		{"No postcode here", ""},
		{"Mixed GU341AA format", "GU34 1AA"}, // Should normalize
		{"Multiple postcodes GU34 1AA and SO24 0HJ", "GU34 1AA"}, // Returns first
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractPostcode(tt.input)
			if got != tt.want {
				t.Errorf("extractPostcode(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}