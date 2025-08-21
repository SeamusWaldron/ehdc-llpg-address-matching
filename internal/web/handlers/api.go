package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// Config represents the web server configuration (simplified)
type Config struct {
	Features struct {
		ExportEnabled         bool `json:"export_enabled"`
		ManualOverrideEnabled bool `json:"manual_override_enabled"`
	} `json:"features"`
}

// APIHandler handles general API endpoints
type APIHandler struct {
	DB     *sql.DB
	Config *Config
}

// StatsResponse represents overall statistics
type StatsResponse struct {
	TotalRecords     int                `json:"total_records"`
	MatchedRecords   int                `json:"matched_records"`
	UnmatchedRecords int                `json:"unmatched_records"`
	NeedsReview      int                `json:"needs_review"`
	MatchRate        float64            `json:"match_rate"`
	BySourceType     map[string]Stats   `json:"by_source_type"`
	ByMatchMethod    map[string]Stats   `json:"by_match_method"`
	ByAddressQuality map[string]Stats   `json:"by_address_quality"`
}

// Stats represents statistics for a category
type Stats struct {
	Count     int     `json:"count"`
	MatchRate float64 `json:"match_rate"`
}

// ViewportStatsResponse represents statistics for a map viewport
type ViewportStatsResponse struct {
	TotalRecords   int            `json:"total_records"`
	BySourceType   map[string]int `json:"by_source_type"`
	ByMatchStatus  map[string]int `json:"by_match_status"`
	ByQuality      map[string]int `json:"by_quality"`
}

// GetStats returns overall system statistics
func (h *APIHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	// Query overall statistics from database views
	var stats StatsResponse
	
	// Get total counts by match status
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN match_status = 'MATCHED' THEN 1 END) as matched,
			COUNT(CASE WHEN match_status = 'UNMATCHED' THEN 1 END) as unmatched,
			COUNT(CASE WHEN match_status = 'NEEDS_REVIEW' THEN 1 END) as needs_review
		FROM v_enhanced_source_documents
	`
	
	err := h.DB.QueryRow(query).Scan(
		&stats.TotalRecords,
		&stats.MatchedRecords,
		&stats.UnmatchedRecords,
		&stats.NeedsReview,
	)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Calculate match rate
	if stats.TotalRecords > 0 {
		stats.MatchRate = float64(stats.MatchedRecords) / float64(stats.TotalRecords) * 100
	}

	// Get statistics by source type
	stats.BySourceType = make(map[string]Stats)
	sourceQuery := `
		SELECT 
			source_type,
			COUNT(*) as total,
			COUNT(CASE WHEN match_status = 'MATCHED' THEN 1 END) as matched
		FROM v_enhanced_source_documents
		GROUP BY source_type
	`
	
	rows, err := h.DB.Query(sourceQuery)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var sourceType string
		var total, matched int
		if err := rows.Scan(&sourceType, &total, &matched); err != nil {
			continue
		}
		
		matchRate := float64(0)
		if total > 0 {
			matchRate = float64(matched) / float64(total) * 100
		}
		
		stats.BySourceType[sourceType] = Stats{
			Count:     total,
			MatchRate: matchRate,
		}
	}

	// Get statistics by match method (for matched records only)
	stats.ByMatchMethod = make(map[string]Stats)
	methodQuery := `
		SELECT 
			COALESCE(match_method, 'unknown') as method,
			COUNT(*) as count
		FROM v_enhanced_source_documents
		WHERE match_status = 'MATCHED'
		GROUP BY match_method
	`
	
	rows, err = h.DB.Query(methodQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var method string
			var count int
			if err := rows.Scan(&method, &count); err != nil {
				continue
			}
			stats.ByMatchMethod[method] = Stats{Count: count, MatchRate: 100}
		}
	}

	// Get statistics by address quality
	stats.ByAddressQuality = make(map[string]Stats)
	qualityQuery := `
		SELECT 
			address_quality,
			COUNT(*) as total,
			COUNT(CASE WHEN match_status = 'MATCHED' THEN 1 END) as matched
		FROM v_enhanced_source_documents
		GROUP BY address_quality
	`
	
	rows, err = h.DB.Query(qualityQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var quality string
			var total, matched int
			if err := rows.Scan(&quality, &total, &matched); err != nil {
				continue
			}
			
			matchRate := float64(0)
			if total > 0 {
				matchRate = float64(matched) / float64(total) * 100
			}
			
			stats.ByAddressQuality[quality] = Stats{
				Count:     total,
				MatchRate: matchRate,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetViewportStats returns statistics for records within a map viewport
func (h *APIHandler) GetViewportStats(w http.ResponseWriter, r *http.Request) {
	// Parse viewport bounds from query parameters
	minLat := parseFloat(r.URL.Query().Get("min_lat"), -90)
	maxLat := parseFloat(r.URL.Query().Get("max_lat"), 90)
	minLng := parseFloat(r.URL.Query().Get("min_lng"), -180)
	maxLng := parseFloat(r.URL.Query().Get("max_lng"), 180)

	var stats ViewportStatsResponse
	stats.BySourceType = make(map[string]int)
	stats.ByMatchStatus = make(map[string]int)
	stats.ByQuality = make(map[string]int)

	// Query records within viewport bounds
	// Convert WGS84 bounds to BNG coordinates for spatial query
	query := `
		SELECT 
			source_type,
			match_status,
			address_quality,
			COUNT(*) as count
		FROM v_map_all_records
		WHERE easting IS NOT NULL AND northing IS NOT NULL
			AND ST_Within(
				ST_SetSRID(ST_MakePoint(easting, northing), 27700),
				ST_Transform(ST_MakeEnvelope($1, $2, $3, $4, 4326), 27700)
			)
		GROUP BY source_type, match_status, address_quality
	`

	rows, err := h.DB.Query(query, minLng, minLat, maxLng, maxLat)
	if err != nil {
		// Fallback to simpler query if spatial query fails
		simpleQuery := `
			SELECT 
				source_type,
				match_status,
				address_quality,
				COUNT(*) as count
			FROM v_map_all_records
			WHERE easting IS NOT NULL AND northing IS NOT NULL
			GROUP BY source_type, match_status, address_quality
		`
		rows, err = h.DB.Query(simpleQuery)
	}
	
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var sourceType, matchStatus, quality string
		var count int
		if err := rows.Scan(&sourceType, &matchStatus, &quality, &count); err != nil {
			continue
		}

		stats.TotalRecords += count
		stats.BySourceType[sourceType] += count
		stats.ByMatchStatus[matchStatus] += count
		stats.ByQuality[quality] += count
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// parseFloat parses a string as float64 with a default value
func parseFloat(s string, defaultVal float64) float64 {
	if s == "" {
		return defaultVal
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return defaultVal
}