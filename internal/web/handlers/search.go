package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// SearchHandler handles search endpoints
type SearchHandler struct {
	DB     *sql.DB
	Config *Config
}

// LLPGSearchResult represents an LLPG search result
type LLPGSearchResult struct {
	UPRN             string   `json:"uprn"`
	Address          string   `json:"address"`
	CanonicalAddress string   `json:"canonical_address"`
	Easting          float64  `json:"easting"`
	Northing         float64  `json:"northing"`
	USRN             *string  `json:"usrn"`
	BLPUClass        *string  `json:"blpu_class"`
	PostalFlag       *string  `json:"postal_flag"`
	Status           string   `json:"status"`
}

// RecordSearchResult represents a record search result
type RecordSearchResult struct {
	SrcID           int     `json:"src_id"`
	SourceType      string  `json:"source_type"`
	Address         string  `json:"address"`
	MatchStatus     string  `json:"match_status"`
	AddressQuality  string  `json:"address_quality"`
	MatchScore      *float64 `json:"match_score"`
	ExternalRef     *string `json:"external_ref"`
}

// SearchLLPG searches LLPG addresses
func (h *SearchHandler) SearchLLPG(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	searchTerm := query.Get("q")
	if searchTerm == "" {
		http.Error(w, "Search term required", http.StatusBadRequest)
		return
	}

	// Parse limit parameter
	limit := parseIntParam(query.Get("limit"), 20)
	if limit > 100 {
		limit = 100 // Maximum limit
	}

	// Search LLPG using trigram similarity and text search
	searchQuery := `
		SELECT 
			uprn, locaddress, addr_can, easting, northing, 
			usrn, blpu_class, postal_flag, status
		FROM dim_address
		WHERE 
			locaddress ILIKE $1 OR 
			addr_can ILIKE $1 OR
			addr_can % $2
		ORDER BY 
			CASE 
				WHEN locaddress ILIKE $1 THEN 1
				WHEN addr_can ILIKE $1 THEN 2
				ELSE 3
			END,
			similarity(addr_can, $2) DESC
		LIMIT $3
	`

	searchPattern := "%" + searchTerm + "%"
	rows, err := h.DB.Query(searchQuery, searchPattern, searchTerm, limit)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []LLPGSearchResult
	for rows.Next() {
		var result LLPGSearchResult
		err := rows.Scan(
			&result.UPRN, &result.Address, &result.CanonicalAddress,
			&result.Easting, &result.Northing, &result.USRN,
			&result.BLPUClass, &result.PostalFlag, &result.Status,
		)
		if err != nil {
			continue
		}
		results = append(results, result)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// SearchRecords searches source records
func (h *SearchHandler) SearchRecords(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	searchTerm := query.Get("q")
	if searchTerm == "" {
		http.Error(w, "Search term required", http.StatusBadRequest)
		return
	}

	// Parse optional filters
	sourceType := query.Get("source_type")
	matchStatus := query.Get("match_status")
	limit := parseIntParam(query.Get("limit"), 50)
	if limit > 200 {
		limit = 200 // Maximum limit
	}

	// Build search query
	searchQuery := `
		SELECT 
			src_id, source_type, 
			COALESCE(canonical_address, original_address) as address,
			match_status, address_quality, match_score, external_ref
		FROM v_enhanced_source_documents
		WHERE (
			original_address ILIKE $1 OR 
			canonical_address ILIKE $1 OR
			external_ref ILIKE $1
		)
	`

	args := []interface{}{"%" + searchTerm + "%"}
	argIndex := 2

	// Add optional filters
	if sourceType != "" {
		searchQuery += " AND source_type = $" + strconv.Itoa(argIndex)
		args = append(args, sourceType)
		argIndex++
	}
	if matchStatus != "" {
		searchQuery += " AND match_status = $" + strconv.Itoa(argIndex)
		args = append(args, matchStatus)
		argIndex++
	}

	searchQuery += " ORDER BY " +
		"CASE " +
		"WHEN external_ref ILIKE $1 THEN 1 " +
		"WHEN original_address ILIKE $1 THEN 2 " +
		"WHEN canonical_address ILIKE $1 THEN 3 " +
		"ELSE 4 END, " +
		"src_id " +
		"LIMIT $" + strconv.Itoa(argIndex)
	args = append(args, limit)

	rows, err := h.DB.Query(searchQuery, args...)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []RecordSearchResult
	for rows.Next() {
		var result RecordSearchResult
		err := rows.Scan(
			&result.SrcID, &result.SourceType, &result.Address,
			&result.MatchStatus, &result.AddressQuality, &result.MatchScore,
			&result.ExternalRef,
		)
		if err != nil {
			continue
		}
		results = append(results, result)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}