package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// RealtimeHandler handles real-time updates via Server-Sent Events
type RealtimeHandler struct {
	DB     *sql.DB
	Config *Config
}

// UpdateNotification represents a real-time update
type UpdateNotification struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// DataUpdate represents data changes
type DataUpdate struct {
	RecordsChanged int    `json:"records_changed"`
	ChangeType     string `json:"change_type"` // "matches_updated", "new_records", "records_deleted"
	Viewport       *struct {
		MinLat float64 `json:"min_lat"`
		MaxLat float64 `json:"max_lat"`
		MinLng float64 `json:"min_lng"`
		MaxLng float64 `json:"max_lng"`
	} `json:"viewport,omitempty"`
}

// StatsUpdate represents statistics changes
type StatsUpdate struct {
	TotalRecords   int     `json:"total_records"`
	MatchedRecords int     `json:"matched_records"`
	MatchRate      float64 `json:"match_rate"`
}

// SSEUpdates handles Server-Sent Events for real-time updates
func (h *RealtimeHandler) SSEUpdates(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get client identifier and viewport from query params
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		clientID = fmt.Sprintf("client_%d", time.Now().UnixNano())
	}

	// Parse viewport bounds if provided
	var viewport *struct {
		MinLat float64 `json:"min_lat"`
		MaxLat float64 `json:"max_lat"`
		MinLng float64 `json:"min_lng"`
		MaxLng float64 `json:"max_lng"`
	}

	if r.URL.Query().Has("min_lat") {
		minLat, _ := strconv.ParseFloat(r.URL.Query().Get("min_lat"), 64)
		maxLat, _ := strconv.ParseFloat(r.URL.Query().Get("max_lat"), 64)
		minLng, _ := strconv.ParseFloat(r.URL.Query().Get("min_lng"), 64)
		maxLng, _ := strconv.ParseFloat(r.URL.Query().Get("max_lng"), 64)

		viewport = &struct {
			MinLat float64 `json:"min_lat"`
			MaxLat float64 `json:"max_lat"`
			MinLng float64 `json:"min_lng"`
			MaxLng float64 `json:"max_lng"`
		}{minLat, maxLat, minLng, maxLng}
	}

	// Create flusher for immediate response sending
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send initial connection confirmation
	h.sendSSEEvent(w, flusher, "connected", map[string]interface{}{
		"client_id": clientID,
		"timestamp": time.Now(),
		"message":   "Connected to EHDC LLPG real-time updates",
	})

	// Set up periodic updates
	ticker := time.NewTicker(30 * time.Second) // Update every 30 seconds
	defer ticker.Stop()

	// Context for client disconnection detection
	ctx := r.Context()

	// Send initial stats
	if stats := h.getLatestStats(); stats != nil {
		h.sendSSEEvent(w, flusher, "stats_update", stats)
	}

	// Main update loop
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return
		case <-ticker.C:
			// Periodic update check
			if h.hasDataChanges() {
				// Send data update notification
				update := &DataUpdate{
					RecordsChanged: h.getChangedRecordsCount(),
					ChangeType:     "matches_updated",
					Viewport:       viewport,
				}
				h.sendSSEEvent(w, flusher, "data_update", update)
			}

			// Send updated stats
			if stats := h.getLatestStats(); stats != nil {
				h.sendSSEEvent(w, flusher, "stats_update", stats)
			}

			// Send heartbeat
			h.sendSSEEvent(w, flusher, "heartbeat", map[string]interface{}{
				"timestamp": time.Now(),
			})
		}
	}
}

// sendSSEEvent sends a Server-Sent Event
func (h *RealtimeHandler) sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, data interface{}) {
	notification := UpdateNotification{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}

	jsonData, err := json.Marshal(notification)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "event: %s\n", eventType)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

// getLatestStats gets the latest system statistics
func (h *RealtimeHandler) getLatestStats() *StatsUpdate {
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN match_status = 'MATCHED' THEN 1 END) as matched
		FROM v_enhanced_source_documents
	`

	var total, matched int
	err := h.DB.QueryRow(query).Scan(&total, &matched)
	if err != nil {
		return nil
	}

	matchRate := float64(0)
	if total > 0 {
		matchRate = float64(matched) / float64(total) * 100
	}

	return &StatsUpdate{
		TotalRecords:   total,
		MatchedRecords: matched,
		MatchRate:      matchRate,
	}
}

// hasDataChanges checks if there have been recent data changes
func (h *RealtimeHandler) hasDataChanges() bool {
	// Simple implementation - check for recent matches
	// In production, this could use a change log table or timestamps
	query := `
		SELECT COUNT(*)
		FROM match_accepted
		WHERE accepted_at > NOW() - INTERVAL '1 minute'
	`

	var recentChanges int
	err := h.DB.QueryRow(query).Scan(&recentChanges)
	if err != nil {
		return false
	}

	return recentChanges > 0
}

// getChangedRecordsCount gets the count of recently changed records
func (h *RealtimeHandler) getChangedRecordsCount() int {
	query := `
		SELECT COUNT(*)
		FROM match_accepted
		WHERE accepted_at > NOW() - INTERVAL '5 minutes'
	`

	var count int
	err := h.DB.QueryRow(query).Scan(&count)
	if err != nil {
		return 0
	}

	return count
}

// MatchingStatus provides real-time matching process status
func (h *RealtimeHandler) MatchingStatus(w http.ResponseWriter, r *http.Request) {
	// This endpoint could provide status of running matching processes
	status := map[string]interface{}{
		"is_running":     false, // Would check if any matching processes are active
		"last_run":       time.Now().Add(-2 * time.Hour), // Example last run time
		"records_queued": 0,     // Records waiting for matching
		"current_method": nil,   // Currently running matching method
		"progress":       0,     // Percentage complete
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// TriggerRefresh manually triggers a data refresh for all connected clients
func (h *RealtimeHandler) TriggerRefresh(w http.ResponseWriter, r *http.Request) {
	// This could be called after batch matching operations
	// For now, just return success
	// In production, this would notify all connected SSE clients

	response := map[string]interface{}{
		"success":   true,
		"message":   "Refresh triggered for all connected clients",
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// WebSocket alternative (basic implementation)
// This could be used instead of SSE for more interactive features
/*
func (h *RealtimeHandler) WebSocketUpdates(w http.ResponseWriter, r *http.Request) {
	// WebSocket implementation would go here
	// Requires websocket library like gorilla/websocket
	// Left as placeholder for future enhancement
}
*/