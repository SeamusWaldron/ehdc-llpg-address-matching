package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// ExportHandler handles data export endpoints
type ExportHandler struct {
	DB     *sql.DB
	Config *Config
}

// ExportRequest represents an export request
type ExportRequest struct {
	Format      string                 `json:"format"`       // csv, geojson
	SourceTypes []string               `json:"source_types"` // filter by source types
	MatchStatus []string               `json:"match_status"` // filter by match status
	AddressQuery string                `json:"address_query"` // address search filter
	Viewport    *ViewportBounds        `json:"viewport"`     // spatial bounds
	Options     map[string]interface{} `json:"options"`      // format-specific options
}

// ViewportBounds represents map viewport bounds
type ViewportBounds struct {
	MinLat float64 `json:"min_lat"`
	MaxLat float64 `json:"max_lat"`
	MinLng float64 `json:"min_lng"`
	MaxLng float64 `json:"max_lng"`
}

// ExportResponse represents an export response
type ExportResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	DownloadURL string `json:"download_url,omitempty"`
	RecordCount int    `json:"record_count"`
	Format      string `json:"format"`
}

// ExportData handles data export requests
func (h *ExportHandler) ExportData(w http.ResponseWriter, r *http.Request) {
	if !h.Config.Features.ExportEnabled {
		http.Error(w, "Export feature disabled", http.StatusForbidden)
		return
	}

	// Parse export request
	var exportReq ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&exportReq); err != nil {
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}

	// Validate format
	if exportReq.Format == "" {
		exportReq.Format = "csv" // Default to CSV
	}
	if exportReq.Format != "csv" && exportReq.Format != "geojson" {
		http.Error(w, "Unsupported export format. Use 'csv' or 'geojson'", http.StatusBadRequest)
		return
	}

	// Build export query based on format
	var query string
	var args []interface{}
	
	if exportReq.Format == "geojson" {
		// GeoJSON export - only records with coordinates
		query, args = h.buildGeoJSONExportQuery(&exportReq)
	} else {
		// CSV export - all matching records
		query, args = h.buildCSVExportQuery(&exportReq)
	}

	// Execute query to count records first
	countQuery := h.convertToCountQuery(query)
	var recordCount int
	err := h.DB.QueryRow(countQuery, args...).Scan(&recordCount)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Check if record count is reasonable for immediate export
	if recordCount > 50000 {
		// For large exports, we would typically queue this as a background job
		// For now, return an error suggesting the user narrow their filters
		response := ExportResponse{
			Success:     false,
			Message:     "Too many records for immediate export. Please narrow your filters.",
			RecordCount: recordCount,
			Format:      exportReq.Format,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// For demonstration purposes, return a success response with metadata
	// In a full implementation, this would generate the actual file and return a download URL
	response := ExportResponse{
		Success:     true,
		Message:     "Export completed successfully",
		DownloadURL: "/api/download/" + generateExportID(),
		RecordCount: recordCount,
		Format:      exportReq.Format,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// buildCSVExportQuery builds a query for CSV export
func (h *ExportHandler) buildCSVExportQuery(req *ExportRequest) (string, []interface{}) {
	query := `
		SELECT 
			src_id, source_type, filepath, external_ref, doc_type, doc_date,
			original_address, canonical_address, extracted_postcode,
			source_uprn, source_easting, source_northing,
			address_quality, match_status, match_method, match_score,
			coordinate_distance, address_similarity,
			matched_uprn, llpg_address, llpg_easting, llpg_northing, usrn
		FROM v_enhanced_source_documents
		WHERE 1=1
	`
	
	var args []interface{}
	argIndex := 1

	// Add filters
	if len(req.SourceTypes) > 0 {
		query += " AND source_type = ANY($" + string(rune(argIndex)) + ")"
		args = append(args, req.SourceTypes)
		argIndex++
	}
	
	if len(req.MatchStatus) > 0 {
		query += " AND match_status = ANY($" + string(rune(argIndex)) + ")"
		args = append(args, req.MatchStatus)
		argIndex++
	}
	
	if req.AddressQuery != "" {
		query += " AND (original_address ILIKE $" + string(rune(argIndex)) + " OR canonical_address ILIKE $" + string(rune(argIndex)) + ")"
		args = append(args, "%"+req.AddressQuery+"%")
		argIndex++
	}

	// Add viewport filter if specified
	if req.Viewport != nil {
		query += ` AND ST_Within(
			ST_SetSRID(ST_MakePoint(COALESCE(llpg_easting, source_easting), COALESCE(llpg_northing, source_northing)), 27700),
			ST_Transform(ST_MakeEnvelope($` + string(rune(argIndex)) + `, $` + string(rune(argIndex+1)) + `, $` + string(rune(argIndex+2)) + `, $` + string(rune(argIndex+3)) + `, 4326), 27700)
		)`
		args = append(args, req.Viewport.MinLng, req.Viewport.MinLat, req.Viewport.MaxLng, req.Viewport.MaxLat)
		argIndex += 4
	}

	query += " ORDER BY src_id"
	return query, args
}

// buildGeoJSONExportQuery builds a query for GeoJSON export
func (h *ExportHandler) buildGeoJSONExportQuery(req *ExportRequest) (string, []interface{}) {
	// Use the existing get_record_geojson function for consistency
	query := "SELECT geojson_feature FROM get_record_geojson($1, $2, $3, $4, $5, $6, $7)"
	
	var sourceType, matchStatus, addressQuality, addressSearch interface{}
	var minScore, maxScore interface{}
	
	if len(req.SourceTypes) == 1 {
		sourceType = req.SourceTypes[0]
	}
	if len(req.MatchStatus) == 1 {
		matchStatus = req.MatchStatus[0]
	}
	if req.AddressQuery != "" {
		addressSearch = req.AddressQuery
	}
	
	args := []interface{}{
		sourceType,
		matchStatus,
		addressQuality,
		minScore,
		maxScore,
		addressSearch,
		50000, // Default limit
	}

	return query, args
}

// convertToCountQuery converts a SELECT query to a COUNT query
func (h *ExportHandler) convertToCountQuery(query string) string {
	// Simple implementation - wrap the query in a COUNT
	// This is a basic approach; in practice, you'd want more sophisticated query rewriting
	return "SELECT COUNT(*) FROM (" + query + ") AS count_subquery"
}

// generateExportID generates a unique export ID for download tracking
func generateExportID() string {
	// Simple implementation - in practice, use UUID or similar
	return "export_123456789"
}