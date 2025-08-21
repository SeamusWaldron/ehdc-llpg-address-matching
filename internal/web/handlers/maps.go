package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// MapsHandler handles map-related endpoints
type MapsHandler struct {
	DB     *sql.DB
	Config *Config
}

// GeoJSONResponse represents a GeoJSON FeatureCollection
type GeoJSONResponse struct {
	Type     string        `json:"type"`
	Features []interface{} `json:"features"`
}

// GetGeoJSON returns filtered GeoJSON data for map display
func (h *MapsHandler) GetGeoJSON(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse filter parameters
	sourceType := query.Get("source_type")
	matchStatus := query.Get("match_status")
	addressQuality := query.Get("address_quality")
	addressSearch := query.Get("address_search")
	
	// Parse numeric filters
	minScore := parseFloatParam(query.Get("min_score"))
	maxScore := parseFloatParam(query.Get("max_score"))
	limit := parseIntParam(query.Get("limit"), 10000)

	// Parse viewport bounds for spatial filtering
	minLat := parseFloatParam(query.Get("min_lat"))
	maxLat := parseFloatParam(query.Get("max_lat"))
	minLng := parseFloatParam(query.Get("min_lng"))
	maxLng := parseFloatParam(query.Get("max_lng"))

	// Use the existing get_record_geojson function with additional spatial filtering
	var geoJSONQuery string
	var args []interface{}

	if minLat != nil && maxLat != nil && minLng != nil && maxLng != nil {
		// Spatial filtering query
		geoJSONQuery = `
			SELECT jsonb_build_object(
				'type', 'Feature',
				'geometry', ST_AsGeoJSON(
					ST_Transform(
						ST_SetSRID(ST_MakePoint(easting, northing), 27700), 
						4326
					)
				)::jsonb,
				'properties', jsonb_build_object(
					'src_id', src_id,
					'source_type', source_type,
					'external_reference', external_reference,
					'address', address,
					'uprn', uprn,
					'match_status', match_status,
					'match_score', match_score,
					'address_quality', address_quality,
					'match_method', match_method,
					'doc_type', doc_type,
					'doc_date', doc_date
				)
			) as geojson_feature
			FROM v_map_all_records
			WHERE easting IS NOT NULL 
			  AND northing IS NOT NULL
			  AND ST_Within(
				ST_SetSRID(ST_MakePoint(easting, northing), 27700),
				ST_Transform(ST_MakeEnvelope($1, $2, $3, $4, 4326), 27700)
			  )
		`
		args = append(args, *minLng, *minLat, *maxLng, *maxLat)
		
		// Add additional filters
		argIndex := 5
		if sourceType != "" {
			geoJSONQuery += " AND source_type = $" + strconv.Itoa(argIndex)
			args = append(args, sourceType)
			argIndex++
		}
		if matchStatus != "" {
			geoJSONQuery += " AND match_status = $" + strconv.Itoa(argIndex)
			args = append(args, matchStatus)
			argIndex++
		}
		if addressQuality != "" {
			geoJSONQuery += " AND address_quality = $" + strconv.Itoa(argIndex)
			args = append(args, addressQuality)
			argIndex++
		}
		if minScore != nil {
			geoJSONQuery += " AND match_score >= $" + strconv.Itoa(argIndex)
			args = append(args, *minScore)
			argIndex++
		}
		if maxScore != nil {
			geoJSONQuery += " AND match_score <= $" + strconv.Itoa(argIndex)
			args = append(args, *maxScore)
			argIndex++
		}
		if addressSearch != "" {
			geoJSONQuery += " AND address ILIKE $" + strconv.Itoa(argIndex)
			args = append(args, "%"+addressSearch+"%")
			argIndex++
		}
		
		geoJSONQuery += " ORDER BY src_id LIMIT $" + strconv.Itoa(argIndex)
		args = append(args, limit)

	} else {
		// Use the existing get_record_geojson function for non-spatial queries
		geoJSONQuery = "SELECT geojson_feature FROM get_record_geojson($1, $2, $3, $4, $5, $6, $7)"
		
		// Prepare parameters for the function (using NULL for unspecified filters)
		var sourceTypeParam, matchStatusParam, addressQualityParam, addressSearchParam interface{}
		var minScoreParam, maxScoreParam interface{}
		
		if sourceType != "" {
			sourceTypeParam = sourceType
		}
		if matchStatus != "" {
			matchStatusParam = matchStatus
		}
		if addressQuality != "" {
			addressQualityParam = addressQuality
		}
		if minScore != nil {
			minScoreParam = *minScore
		}
		if maxScore != nil {
			maxScoreParam = *maxScore
		}
		if addressSearch != "" {
			addressSearchParam = addressSearch
		}
		
		args = []interface{}{
			sourceTypeParam,
			matchStatusParam,
			addressQualityParam,
			minScoreParam,
			maxScoreParam,
			addressSearchParam,
			limit,
		}
	}

	// Execute query
	rows, err := h.DB.Query(geoJSONQuery, args...)
	if err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Collect GeoJSON features
	var features []interface{}
	for rows.Next() {
		var featureJSON []byte
		if err := rows.Scan(&featureJSON); err != nil {
			continue
		}

		var feature interface{}
		if err := json.Unmarshal(featureJSON, &feature); err != nil {
			continue
		}
		
		features = append(features, feature)
	}

	// Create GeoJSON FeatureCollection response
	response := GeoJSONResponse{
		Type:     "FeatureCollection",
		Features: features,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// parseFloatParam parses a string parameter as float64, returns nil if empty or invalid
func parseFloatParam(s string) *float64 {
	if s == "" {
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return &f
	}
	return nil
}

// parseIntParam parses a string parameter as int with default value
func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	return defaultVal
}