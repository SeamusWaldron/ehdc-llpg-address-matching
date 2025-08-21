package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

// RecordsHandler handles record-related endpoints
type RecordsHandler struct {
	DB     *sql.DB
	Config *Config
}

// Record represents a source document record
type Record struct {
	SrcID               int       `json:"src_id"`
	SourceType          string    `json:"source_type"`
	Filepath            string    `json:"filepath"`
	ExternalRef         *string   `json:"external_ref"`
	DocType             *string   `json:"doc_type"`
	DocDate             *string   `json:"doc_date"`
	OriginalAddress     string    `json:"original_address"`
	CanonicalAddress    *string   `json:"canonical_address"`
	ExtractedPostcode   *string   `json:"extracted_postcode"`
	SourceUPRN          *string   `json:"source_uprn"`
	SourceEasting       *float64  `json:"source_easting"`
	SourceNorthing      *float64  `json:"source_northing"`
	AddressQuality      string    `json:"address_quality"`
	MatchStatus         string    `json:"match_status"`
	MatchMethod         *string   `json:"match_method"`
	MatchScore          *float64  `json:"match_score"`
	CoordinateDistance  *float64  `json:"coordinate_distance"`
	AddressSimilarity   *float64  `json:"address_similarity"`
	MatchedUPRN         *string   `json:"matched_uprn"`
	LLPGAddress         *string   `json:"llpg_address"`
	LLPGEasting         *float64  `json:"llpg_easting"`
	LLPGNorthing        *float64  `json:"llpg_northing"`
	USRN                *string   `json:"usrn"`
	ImportDate          time.Time `json:"import_date"`
}

// MatchCandidate represents a potential match for a record
type MatchCandidate struct {
	UPRN             string   `json:"uprn"`
	Address          string   `json:"address"`
	CanonicalAddress string   `json:"canonical_address"`
	Easting          float64  `json:"easting"`
	Northing         float64  `json:"northing"`
	USRN             *string  `json:"usrn"`
	BLPUClass        *string  `json:"blpu_class"`
	PostalFlag       *string  `json:"postal_flag"`
	Status           string   `json:"status"`
	Score            float64  `json:"score"`
	Method           string   `json:"method"`
	Features         []string `json:"features"`
}

// RecordsListResponse represents a paginated list of records
type RecordsListResponse struct {
	Records []Record `json:"records"`
	Total   int      `json:"total"`
	Page    int      `json:"page"`
	PerPage int      `json:"per_page"`
}

// ListRecords returns a filtered and paginated list of records
func (h *RecordsHandler) ListRecords(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse pagination parameters
	page := parseIntParam(query.Get("page"), 1)
	perPage := parseIntParam(query.Get("per_page"), 50)
	if perPage > 1000 {
		perPage = 1000 // Limit maximum page size
	}
	offset := (page - 1) * perPage

	// Parse filter parameters
	sourceType := query.Get("source_type")
	matchStatus := query.Get("match_status")
	addressQuality := query.Get("address_quality")
	addressSearch := query.Get("address_search")
	
	// Build dynamic query
	baseQuery := `
		SELECT 
			src_id, source_type, filepath, external_ref, doc_type, doc_date,
			original_address, canonical_address, extracted_postcode,
			source_uprn, source_easting, source_northing,
			address_quality, match_status, match_method, match_score,
			coordinate_distance, address_similarity,
			matched_uprn, llpg_address, llpg_easting, llpg_northing, usrn,
			import_date
		FROM v_enhanced_source_documents
		WHERE 1=1
	`
	
	countQuery := `
		SELECT COUNT(*) 
		FROM v_enhanced_source_documents
		WHERE 1=1
	`
	
	var conditions []string
	var args []interface{}
	argIndex := 1

	// Add filter conditions
	if sourceType != "" {
		conditions = append(conditions, " AND source_type = $"+strconv.Itoa(argIndex))
		args = append(args, sourceType)
		argIndex++
	}
	if matchStatus != "" {
		conditions = append(conditions, " AND match_status = $"+strconv.Itoa(argIndex))
		args = append(args, matchStatus)
		argIndex++
	}
	if addressQuality != "" {
		conditions = append(conditions, " AND address_quality = $"+strconv.Itoa(argIndex))
		args = append(args, addressQuality)
		argIndex++
	}
	if addressSearch != "" {
		conditions = append(conditions, " AND (original_address ILIKE $"+strconv.Itoa(argIndex)+" OR canonical_address ILIKE $"+strconv.Itoa(argIndex)+")")
		args = append(args, "%"+addressSearch+"%")
		argIndex++
	}

	// Apply conditions to both queries
	for _, condition := range conditions {
		baseQuery += condition
		countQuery += condition
	}

	// Get total count
	var total int
	err := h.DB.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Add ordering and pagination to main query
	baseQuery += " ORDER BY src_id LIMIT $" + strconv.Itoa(argIndex) + " OFFSET $" + strconv.Itoa(argIndex+1)
	args = append(args, perPage, offset)

	// Execute main query
	rows, err := h.DB.Query(baseQuery, args...)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var record Record
		var docDate sql.NullString
		
		err := rows.Scan(
			&record.SrcID, &record.SourceType, &record.Filepath, &record.ExternalRef,
			&record.DocType, &docDate, &record.OriginalAddress, &record.CanonicalAddress,
			&record.ExtractedPostcode, &record.SourceUPRN, &record.SourceEasting, &record.SourceNorthing,
			&record.AddressQuality, &record.MatchStatus, &record.MatchMethod, &record.MatchScore,
			&record.CoordinateDistance, &record.AddressSimilarity,
			&record.MatchedUPRN, &record.LLPGAddress, &record.LLPGEasting, &record.LLPGNorthing,
			&record.USRN, &record.ImportDate,
		)
		if err != nil {
			continue
		}

		if docDate.Valid {
			record.DocDate = &docDate.String
		}

		records = append(records, record)
	}

	response := RecordsListResponse{
		Records: records,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetRecord returns details for a specific record
func (h *RecordsHandler) GetRecord(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	srcID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid record ID", http.StatusBadRequest)
		return
	}

	query := `
		SELECT 
			src_id, source_type, filepath, external_ref, doc_type, doc_date,
			original_address, canonical_address, extracted_postcode,
			source_uprn, source_easting, source_northing,
			address_quality, match_status, match_method, match_score,
			coordinate_distance, address_similarity,
			matched_uprn, llpg_address, llpg_easting, llpg_northing, usrn,
			import_date
		FROM v_enhanced_source_documents
		WHERE src_id = $1
	`

	var record Record
	var docDate sql.NullString
	
	err = h.DB.QueryRow(query, srcID).Scan(
		&record.SrcID, &record.SourceType, &record.Filepath, &record.ExternalRef,
		&record.DocType, &docDate, &record.OriginalAddress, &record.CanonicalAddress,
		&record.ExtractedPostcode, &record.SourceUPRN, &record.SourceEasting, &record.SourceNorthing,
		&record.AddressQuality, &record.MatchStatus, &record.MatchMethod, &record.MatchScore,
		&record.CoordinateDistance, &record.AddressSimilarity,
		&record.MatchedUPRN, &record.LLPGAddress, &record.LLPGEasting, &record.LLPGNorthing,
		&record.USRN, &record.ImportDate,
	)
	
	if err == sql.ErrNoRows {
		http.Error(w, "Record not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if docDate.Valid {
		record.DocDate = &docDate.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(record)
}

// GetCandidates returns potential matches for a record
func (h *RecordsHandler) GetCandidates(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	srcID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid record ID", http.StatusBadRequest)
		return
	}

	// For now, return a simple implementation
	// In a full implementation, this would run the matching algorithms
	// to find potential candidates from the LLPG
	
	query := `
		SELECT 
			uprn, locaddress, addr_can, easting, northing, usrn, 
			blpu_class, postal_flag, status
		FROM dim_address
		WHERE addr_can ILIKE (
			SELECT '%' || COALESCE(canonical_address, original_address) || '%'
			FROM v_enhanced_source_documents 
			WHERE src_id = $1
		)
		LIMIT 10
	`

	rows, err := h.DB.Query(query, srcID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var candidates []MatchCandidate
	for rows.Next() {
		var candidate MatchCandidate
		err := rows.Scan(
			&candidate.UPRN, &candidate.Address, &candidate.CanonicalAddress,
			&candidate.Easting, &candidate.Northing, &candidate.USRN,
			&candidate.BLPUClass, &candidate.PostalFlag, &candidate.Status,
		)
		if err != nil {
			continue
		}

		// Mock score and method for demonstration
		candidate.Score = 0.85
		candidate.Method = "fuzzy_similarity"
		candidate.Features = []string{"address_similarity", "postcode_match"}

		candidates = append(candidates, candidate)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(candidates)
}

// AcceptMatch accepts a match candidate for a record
func (h *RecordsHandler) AcceptMatch(w http.ResponseWriter, r *http.Request) {
	if !h.Config.Features.ManualOverrideEnabled {
		http.Error(w, "Feature disabled", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	srcID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid record ID", http.StatusBadRequest)
		return
	}

	// Parse JSON body for match acceptance
	var acceptRequest struct {
		UPRN   string  `json:"uprn"`
		Method string  `json:"method"`
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&acceptRequest); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate UPRN exists in LLPG
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM dim_address WHERE uprn = $1)`
	err = h.DB.QueryRow(checkQuery, acceptRequest.UPRN).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Invalid UPRN", http.StatusBadRequest)
		return
	}

	// Get current record state for audit
	var currentRecord Record
	getCurrentQuery := `
		SELECT match_status, matched_uprn
		FROM v_enhanced_source_documents
		WHERE src_id = $1
	`
	h.DB.QueryRow(getCurrentQuery, srcID).Scan(&currentRecord.MatchStatus, &currentRecord.MatchedUPRN)

	// Begin transaction for atomic operations
	tx, err := h.DB.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Insert audit record
	auditQuery := `
		INSERT INTO audit_match_decisions (
			src_id, old_match_status, new_match_status, old_uprn, new_uprn,
			decision_type, decision_reason, match_method, match_score, confidence,
			decided_by, client_info
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	clientInfo := h.getClientInfo(r)
	_, err = tx.Exec(auditQuery, srcID, currentRecord.MatchStatus, "MATCHED",
		currentRecord.MatchedUPRN, &acceptRequest.UPRN, "ACCEPT_MATCH", 
		acceptRequest.Reason, acceptRequest.Method, acceptRequest.Score, 
		acceptRequest.Score, "web_user", clientInfo)

	if err != nil {
		http.Error(w, "Audit logging failed", http.StatusInternalServerError)
		return
	}

	// Insert/update match_accepted table (handled by trigger)
	matchQuery := `
		INSERT INTO match_accepted (src_id, uprn, method, score, confidence, accepted_by, accepted_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (src_id) DO UPDATE SET
			uprn = EXCLUDED.uprn,
			method = EXCLUDED.method,
			score = EXCLUDED.score,
			confidence = EXCLUDED.confidence,
			accepted_by = EXCLUDED.accepted_by,
			accepted_at = NOW()
	`

	_, err = tx.Exec(matchQuery, srcID, acceptRequest.UPRN, acceptRequest.Method, 
		acceptRequest.Score, acceptRequest.Score, "web_user")

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, "Transaction commit failed", http.StatusInternalServerError)
		return
	}

	// Return success response
	response := map[string]interface{}{
		"status":    "accepted",
		"src_id":    srcID,
		"uprn":      acceptRequest.UPRN,
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SetCoordinates sets manual coordinates for a record
func (h *RecordsHandler) SetCoordinates(w http.ResponseWriter, r *http.Request) {
	if !h.Config.Features.ManualOverrideEnabled {
		http.Error(w, "Feature disabled", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	srcID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid record ID", http.StatusBadRequest)
		return
	}

	// Parse JSON body for coordinate override
	var coordRequest struct {
		Easting  float64 `json:"easting"`
		Northing float64 `json:"northing"`
		Source   string  `json:"source"`
		Reason   string  `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&coordRequest); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Get current coordinates for audit
	var currentEasting, currentNorthing sql.NullFloat64
	getCurrentQuery := `
		SELECT source_easting, source_northing
		FROM source_documents
		WHERE src_id = $1
	`
	h.DB.QueryRow(getCurrentQuery, srcID).Scan(&currentEasting, &currentNorthing)

	// Begin transaction
	tx, err := h.DB.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Insert audit record for coordinate change
	auditQuery := `
		INSERT INTO audit_coordinate_changes (
			src_id, old_easting, old_northing, new_easting, new_northing,
			coordinate_source, change_reason, changed_by, client_info
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	clientInfo := h.getClientInfo(r)
	var oldE, oldN interface{}
	if currentEasting.Valid {
		oldE = currentEasting.Float64
	}
	if currentNorthing.Valid {
		oldN = currentNorthing.Float64
	}

	_, err = tx.Exec(auditQuery, srcID, oldE, oldN, coordRequest.Easting, 
		coordRequest.Northing, coordRequest.Source, coordRequest.Reason, 
		"web_user", clientInfo)

	if err != nil {
		http.Error(w, "Audit logging failed", http.StatusInternalServerError)
		return
	}

	// Update source coordinates
	updateQuery := `
		UPDATE source_documents 
		SET source_easting = $1, source_northing = $2
		WHERE src_id = $3
	`
	_, err = tx.Exec(updateQuery, coordRequest.Easting, coordRequest.Northing, srcID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, "Transaction commit failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "coordinates_set",
		"src_id":    srcID,
		"easting":   coordRequest.Easting,
		"northing":  coordRequest.Northing,
		"timestamp": time.Now(),
	})
}

// RejectMatch rejects all candidates for a record
func (h *RecordsHandler) RejectMatch(w http.ResponseWriter, r *http.Request) {
	if !h.Config.Features.ManualOverrideEnabled {
		http.Error(w, "Feature disabled", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	srcID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid record ID", http.StatusBadRequest)
		return
	}

	// Parse JSON body for rejection reason
	var rejectRequest struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&rejectRequest)

	// Get current record state for audit
	var currentRecord Record
	getCurrentQuery := `
		SELECT match_status, matched_uprn
		FROM v_enhanced_source_documents
		WHERE src_id = $1
	`
	h.DB.QueryRow(getCurrentQuery, srcID).Scan(&currentRecord.MatchStatus, &currentRecord.MatchedUPRN)

	// Begin transaction
	tx, err := h.DB.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Insert audit record
	auditQuery := `
		INSERT INTO audit_match_decisions (
			src_id, old_match_status, new_match_status, old_uprn, new_uprn,
			decision_type, decision_reason, decided_by, client_info
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	clientInfo := h.getClientInfo(r)
	_, err = tx.Exec(auditQuery, srcID, currentRecord.MatchStatus, "UNMATCHED",
		currentRecord.MatchedUPRN, nil, "REJECT_MATCH", 
		rejectRequest.Reason, "web_user", clientInfo)

	if err != nil {
		http.Error(w, "Audit logging failed", http.StatusInternalServerError)
		return
	}

	// Remove from match_accepted table
	_, err = tx.Exec("DELETE FROM match_accepted WHERE src_id = $1", srcID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, "Transaction commit failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "rejected",
		"src_id":    srcID,
		"timestamp": time.Now(),
	})
}

// GetHistory returns audit history for a specific record
func (h *RecordsHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	srcID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid record ID", http.StatusBadRequest)
		return
	}

	query := r.URL.Query()
	eventType := query.Get("type") // Optional filter by event type
	limit := parseIntParam(query.Get("limit"), 50)

	baseQuery := `
		SELECT event_type, action, details, user_name, event_timestamp, notes
		FROM v_record_audit_history
		WHERE src_id = $1
	`
	
	var queryArgs []interface{}
	queryArgs = append(queryArgs, srcID)
	argIndex := 2

	if eventType != "" {
		baseQuery += " AND event_type = $" + strconv.Itoa(argIndex)
		queryArgs = append(queryArgs, eventType)
		argIndex++
	}

	baseQuery += " ORDER BY event_timestamp DESC LIMIT $" + strconv.Itoa(argIndex)
	queryArgs = append(queryArgs, limit)

	rows, err := h.DB.Query(baseQuery, queryArgs...)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type HistoryEntry struct {
		EventType      string    `json:"event_type"`
		Action         string    `json:"action"`
		Details        *string   `json:"details"`
		UserName       string    `json:"user_name"`
		EventTimestamp time.Time `json:"event_timestamp"`
		Notes          *string   `json:"notes"`
	}

	var history []HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		err := rows.Scan(&entry.EventType, &entry.Action, &entry.Details, 
			&entry.UserName, &entry.EventTimestamp, &entry.Notes)
		if err != nil {
			continue
		}
		history = append(history, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// AddNote adds a manual note to a record
func (h *RecordsHandler) AddNote(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	srcID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid record ID", http.StatusBadRequest)
		return
	}

	var noteRequest struct {
		Text string `json:"text"`
		Type string `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&noteRequest); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if noteRequest.Text == "" {
		http.Error(w, "Note text required", http.StatusBadRequest)
		return
	}

	if noteRequest.Type == "" {
		noteRequest.Type = "GENERAL"
	}

	query := `
		INSERT INTO record_notes (src_id, note_type, note_text, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`

	var noteID int
	var createdAt time.Time
	err = h.DB.QueryRow(query, srcID, noteRequest.Type, noteRequest.Text, "web_user").
		Scan(&noteID, &createdAt)

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"id":         noteID,
		"src_id":     srcID,
		"text":       noteRequest.Text,
		"type":       noteRequest.Type,
		"created_at": createdAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper method to extract client information
func (h *RecordsHandler) getClientInfo(r *http.Request) string {
	clientInfo := map[string]string{
		"user_agent":    r.Header.Get("User-Agent"),
		"remote_addr":   r.RemoteAddr,
		"x_forwarded":   r.Header.Get("X-Forwarded-For"),
		"accept_lang":   r.Header.Get("Accept-Language"),
	}
	
	jsonBytes, _ := json.Marshal(clientInfo)
	return string(jsonBytes)
}