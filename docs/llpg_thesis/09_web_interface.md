# Chapter 9: Web Interface

## 9.1 Interface Overview

The web interface provides HTTP endpoints for address search, record browsing, map visualisation, and data export. The interface is implemented using the Gorilla mux router in `internal/web/`.

```
+------------------+     +------------------+     +------------------+
|   Web Browser    | --> |    Gin Server    | --> |   PostgreSQL     |
|   or API Client  |     |   (port 8443)    |     |   (PostGIS)      |
+------------------+     +------------------+     +------------------+
```

## 9.2 Server Configuration

### 9.2.1 Server Setup

The server is configured in `internal/web/server.go`:

```go
type Server struct {
    db     *sql.DB
    config *Config
    router *mux.Router
}

type Config struct {
    Host            string
    Port            int
    EnableExport    bool
    EnableOverride  bool
    StaticPath      string
}

func NewServer(db *sql.DB, config *Config) *Server {
    s := &Server{
        db:     db,
        config: config,
        router: mux.NewRouter(),
    }

    s.setupRoutes()
    return s
}

func (s *Server) setupRoutes() {
    // Static files
    s.router.PathPrefix("/static/").Handler(
        http.StripPrefix("/static/",
            http.FileServer(http.Dir(s.config.StaticPath))))

    // API routes
    api := s.router.PathPrefix("/api").Subrouter()

    // Address search
    api.HandleFunc("/search", s.handleSearch).Methods("GET")
    api.HandleFunc("/uprn/{uprn}", s.handleUPRNLookup).Methods("GET")

    // Records
    api.HandleFunc("/records", s.handleRecords).Methods("GET")
    api.HandleFunc("/records/{id}", s.handleRecordDetail).Methods("GET")

    // Matching
    api.HandleFunc("/match", s.handleBatchMatch).Methods("POST")
    api.HandleFunc("/match/single", s.handleSingleMatch).Methods("POST")

    // Maps
    api.HandleFunc("/maps/bounds", s.handleMapBounds).Methods("GET")
    api.HandleFunc("/maps/points", s.handleMapPoints).Methods("GET")

    // Export
    if s.config.EnableExport {
        api.HandleFunc("/export", s.handleExport).Methods("GET")
    }

    // Override
    if s.config.EnableOverride {
        api.HandleFunc("/override", s.handleOverride).Methods("POST")
    }

    // Dashboard
    s.router.HandleFunc("/", s.handleDashboard).Methods("GET")
    s.router.HandleFunc("/data", s.handleData).Methods("GET")
}

func (s *Server) Start() error {
    addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
    fmt.Printf("Starting server on %s\n", addr)

    server := &http.Server{
        Addr:         addr,
        Handler:      s.router,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 60 * time.Second,
    }

    return server.ListenAndServe()
}
```

## 9.3 API Endpoints

### 9.3.1 Address Search

**Endpoint**: `GET /api/search`

**Parameters**:
- `q`: Search query (address text)
- `limit`: Maximum results (default 50)

**Implementation**:

```go
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
    query := r.URL.Query().Get("q")
    limit := getIntParam(r, "limit", 50)

    if query == "" {
        http.Error(w, "Query parameter 'q' required", http.StatusBadRequest)
        return
    }

    // Normalise query
    canonical, _, _ := normalize.CanonicalAddress(query)

    rows, err := s.db.Query(`
        SELECT uprn, full_address, address_canonical, easting, northing,
               similarity($1, address_canonical) as score
        FROM dim_address
        WHERE address_canonical % $1
        ORDER BY score DESC
        LIMIT $2
    `, canonical, limit)

    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    var results []SearchResult
    for rows.Next() {
        var r SearchResult
        if err := rows.Scan(&r.UPRN, &r.FullAddress, &r.Canonical,
                           &r.Easting, &r.Northing, &r.Score); err != nil {
            continue
        }
        results = append(results, r)
    }

    json.NewEncoder(w).Encode(results)
}
```

**Response Example**:
```json
[
    {
        "uprn": "100012345678",
        "full_address": "12 HIGH STREET, ALTON, GU34 1AB",
        "canonical": "12 HIGH STREET ALTON",
        "easting": 471234.5,
        "northing": 139567.2,
        "score": 0.92
    }
]
```

### 9.3.2 UPRN Lookup

**Endpoint**: `GET /api/uprn/{uprn}`

**Implementation**:

```go
func (s *Server) handleUPRNLookup(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    uprn := vars["uprn"]

    var result AddressDetail
    err := s.db.QueryRow(`
        SELECT d.uprn, d.full_address, d.address_canonical,
               d.usrn, d.blpu_class, d.postal_flag, d.status_code,
               l.easting, l.northing, l.latitude, l.longitude
        FROM dim_address d
        LEFT JOIN dim_location l ON d.location_id = l.location_id
        WHERE d.uprn = $1
    `, uprn).Scan(
        &result.UPRN, &result.FullAddress, &result.Canonical,
        &result.USRN, &result.BLPUClass, &result.PostalFlag, &result.Status,
        &result.Easting, &result.Northing, &result.Latitude, &result.Longitude,
    )

    if err == sql.ErrNoRows {
        http.Error(w, "UPRN not found", http.StatusNotFound)
        return
    }
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(result)
}
```

### 9.3.3 Record Browsing

**Endpoint**: `GET /api/records`

**Parameters**:
- `type`: Document type filter
- `status`: Match status filter (matched, unmatched, review)
- `page`: Page number
- `limit`: Records per page

**Implementation**:

```go
func (s *Server) handleRecords(w http.ResponseWriter, r *http.Request) {
    docType := r.URL.Query().Get("type")
    status := r.URL.Query().Get("status")
    page := getIntParam(r, "page", 1)
    limit := getIntParam(r, "limit", 50)
    offset := (page - 1) * limit

    query := `
        SELECT s.document_id, s.raw_address, s.address_canonical,
               s.external_reference, s.document_date,
               dt.type_name,
               ma.uprn as matched_uprn, ma.confidence, ma.method,
               d.full_address as matched_address
        FROM src_document s
        JOIN dim_document_type dt ON s.doc_type_id = dt.doc_type_id
        LEFT JOIN match_accepted ma ON s.document_id = ma.src_id
        LEFT JOIN dim_address d ON ma.uprn = d.uprn
        WHERE 1=1
    `

    var args []interface{}
    argNum := 1

    if docType != "" {
        query += fmt.Sprintf(" AND dt.type_code = $%d", argNum)
        args = append(args, docType)
        argNum++
    }

    if status == "matched" {
        query += " AND ma.uprn IS NOT NULL"
    } else if status == "unmatched" {
        query += " AND ma.uprn IS NULL"
    }

    query += fmt.Sprintf(" ORDER BY s.document_id LIMIT $%d OFFSET $%d", argNum, argNum+1)
    args = append(args, limit, offset)

    rows, err := s.db.Query(query, args...)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    var records []RecordSummary
    for rows.Next() {
        var r RecordSummary
        rows.Scan(&r.DocumentID, &r.RawAddress, &r.Canonical,
                  &r.ExternalRef, &r.DocumentDate, &r.TypeName,
                  &r.MatchedUPRN, &r.Confidence, &r.Method,
                  &r.MatchedAddress)
        records = append(records, r)
    }

    // Get total count
    var total int
    countQuery := strings.Replace(query, "SELECT s.document_id, s.raw_address", "SELECT COUNT(*)", 1)
    countQuery = strings.Split(countQuery, "ORDER BY")[0]
    s.db.QueryRow(countQuery, args[:len(args)-2]...).Scan(&total)

    response := RecordResponse{
        Records:    records,
        Total:      total,
        Page:       page,
        PageSize:   limit,
        TotalPages: (total + limit - 1) / limit,
    }

    json.NewEncoder(w).Encode(response)
}
```

### 9.3.4 Map Points

**Endpoint**: `GET /api/maps/points`

**Parameters**:
- `minX`, `minY`, `maxX`, `maxY`: Bounding box coordinates
- `limit`: Maximum points

**Implementation**:

```go
func (s *Server) handleMapPoints(w http.ResponseWriter, r *http.Request) {
    minX := getFloatParam(r, "minX", 0)
    minY := getFloatParam(r, "minY", 0)
    maxX := getFloatParam(r, "maxX", 0)
    maxY := getFloatParam(r, "maxY", 0)
    limit := getIntParam(r, "limit", 1000)

    rows, err := s.db.Query(`
        SELECT d.uprn, d.full_address, l.easting, l.northing,
               l.latitude, l.longitude
        FROM dim_address d
        JOIN dim_location l ON d.location_id = l.location_id
        WHERE l.easting BETWEEN $1 AND $3
          AND l.northing BETWEEN $2 AND $4
        LIMIT $5
    `, minX, minY, maxX, maxY, limit)

    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    var points []MapPoint
    for rows.Next() {
        var p MapPoint
        rows.Scan(&p.UPRN, &p.Address, &p.Easting, &p.Northing,
                  &p.Latitude, &p.Longitude)
        points = append(points, p)
    }

    json.NewEncoder(w).Encode(points)
}
```

### 9.3.5 Data Export

**Endpoint**: `GET /api/export`

**Parameters**:
- `type`: Document type (or "all")
- `format`: Output format (csv, json)

**Implementation**:

```go
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
    docType := r.URL.Query().Get("type")
    format := r.URL.Query().Get("format")

    if format == "" {
        format = "csv"
    }

    query := `
        SELECT s.document_id, dt.type_code, s.job_number, s.filepath,
               s.external_reference, s.document_date, s.raw_address,
               ma.uprn, ma.method, ma.confidence,
               d.full_address, d.easting, d.northing
        FROM src_document s
        JOIN dim_document_type dt ON s.doc_type_id = dt.doc_type_id
        LEFT JOIN match_accepted ma ON s.document_id = ma.src_id
        LEFT JOIN dim_address d ON ma.uprn = d.uprn
    `

    if docType != "" && docType != "all" {
        query += " WHERE dt.type_code = '" + docType + "'"
    }

    query += " ORDER BY s.document_id"

    rows, err := s.db.Query(query)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    if format == "csv" {
        w.Header().Set("Content-Type", "text/csv")
        w.Header().Set("Content-Disposition", "attachment; filename=export.csv")

        writer := csv.NewWriter(w)
        writer.Write([]string{
            "document_id", "type", "job_number", "filepath",
            "external_reference", "document_date", "raw_address",
            "matched_uprn", "match_method", "confidence",
            "matched_address", "easting", "northing",
        })

        for rows.Next() {
            var record [13]sql.NullString
            rows.Scan(&record[0], &record[1], &record[2], &record[3],
                     &record[4], &record[5], &record[6], &record[7],
                     &record[8], &record[9], &record[10], &record[11],
                     &record[12])

            row := make([]string, 13)
            for i, v := range record {
                if v.Valid {
                    row[i] = v.String
                }
            }
            writer.Write(row)
        }
        writer.Flush()

    } else {
        w.Header().Set("Content-Type", "application/json")

        var records []map[string]interface{}
        cols, _ := rows.Columns()

        for rows.Next() {
            values := make([]interface{}, len(cols))
            valuePtrs := make([]interface{}, len(cols))
            for i := range values {
                valuePtrs[i] = &values[i]
            }
            rows.Scan(valuePtrs...)

            record := make(map[string]interface{})
            for i, col := range cols {
                record[col] = values[i]
            }
            records = append(records, record)
        }

        json.NewEncoder(w).Encode(records)
    }
}
```

## 9.4 Middleware

### 9.4.1 CORS Middleware

```go
func CORSMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

### 9.4.2 Logging Middleware

```go
func LoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()

        // Create response wrapper to capture status
        wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

        next.ServeHTTP(wrapped, r)

        duration := time.Since(start)
        fmt.Printf("%s %s %d %v\n",
            r.Method, r.URL.Path, wrapped.statusCode, duration)
    })
}

type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (w *responseWriter) WriteHeader(code int) {
    w.statusCode = code
    w.ResponseWriter.WriteHeader(code)
}
```

## 9.5 Static Web Interface

### 9.5.1 Dashboard HTML

The main dashboard is served from `internal/web/static/index.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>EHDC LLPG Address Matcher</title>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <header>
        <h1>EHDC Address Matching System</h1>
        <nav>
            <a href="#search">Search</a>
            <a href="#records">Records</a>
            <a href="#map">Map</a>
            <a href="#export">Export</a>
        </nav>
    </header>

    <main>
        <section id="search">
            <h2>Address Search</h2>
            <form id="search-form">
                <input type="text" id="search-query"
                       placeholder="Enter address to search...">
                <button type="submit">Search</button>
            </form>
            <div id="search-results"></div>
        </section>

        <section id="statistics">
            <h2>Matching Statistics</h2>
            <div id="stats-container">
                <div class="stat-box">
                    <span class="stat-value" id="total-docs">-</span>
                    <span class="stat-label">Total Documents</span>
                </div>
                <div class="stat-box">
                    <span class="stat-value" id="matched-docs">-</span>
                    <span class="stat-label">Matched</span>
                </div>
                <div class="stat-box">
                    <span class="stat-value" id="match-rate">-</span>
                    <span class="stat-label">Match Rate</span>
                </div>
            </div>
        </section>

        <section id="records">
            <h2>Document Records</h2>
            <div id="filters">
                <select id="type-filter">
                    <option value="">All Types</option>
                    <option value="decision">Decision Notices</option>
                    <option value="land_charge">Land Charges</option>
                    <option value="enforcement">Enforcement</option>
                    <option value="agreement">Agreements</option>
                </select>
                <select id="status-filter">
                    <option value="">All Status</option>
                    <option value="matched">Matched</option>
                    <option value="unmatched">Unmatched</option>
                </select>
            </div>
            <table id="records-table">
                <thead>
                    <tr>
                        <th>ID</th>
                        <th>Type</th>
                        <th>Address</th>
                        <th>Matched UPRN</th>
                        <th>Confidence</th>
                    </tr>
                </thead>
                <tbody></tbody>
            </table>
            <div id="pagination"></div>
        </section>
    </main>

    <script src="/static/js/app.js"></script>
</body>
</html>
```

### 9.5.2 JavaScript Application

```javascript
// app.js
document.addEventListener('DOMContentLoaded', function() {
    loadStatistics();
    loadRecords();

    document.getElementById('search-form').addEventListener('submit', handleSearch);
    document.getElementById('type-filter').addEventListener('change', loadRecords);
    document.getElementById('status-filter').addEventListener('change', loadRecords);
});

async function loadStatistics() {
    const response = await fetch('/data');
    const data = await response.json();

    document.getElementById('total-docs').textContent = data.total.toLocaleString();
    document.getElementById('matched-docs').textContent = data.matched.toLocaleString();
    document.getElementById('match-rate').textContent =
        ((data.matched / data.total) * 100).toFixed(1) + '%';
}

async function handleSearch(e) {
    e.preventDefault();
    const query = document.getElementById('search-query').value;

    const response = await fetch(`/api/search?q=${encodeURIComponent(query)}`);
    const results = await response.json();

    const container = document.getElementById('search-results');
    container.innerHTML = results.map(r => `
        <div class="result">
            <strong>${r.full_address}</strong>
            <span class="uprn">UPRN: ${r.uprn}</span>
            <span class="score">Score: ${r.score.toFixed(3)}</span>
        </div>
    `).join('');
}

async function loadRecords(page = 1) {
    const type = document.getElementById('type-filter').value;
    const status = document.getElementById('status-filter').value;

    const params = new URLSearchParams({page, limit: 50});
    if (type) params.append('type', type);
    if (status) params.append('status', status);

    const response = await fetch(`/api/records?${params}`);
    const data = await response.json();

    const tbody = document.querySelector('#records-table tbody');
    tbody.innerHTML = data.records.map(r => `
        <tr>
            <td>${r.document_id}</td>
            <td>${r.type_name}</td>
            <td>${r.raw_address || '-'}</td>
            <td>${r.matched_uprn || '-'}</td>
            <td>${r.confidence ? r.confidence.toFixed(3) : '-'}</td>
        </tr>
    `).join('');

    renderPagination(data.page, data.total_pages);
}

function renderPagination(current, total) {
    const container = document.getElementById('pagination');
    let html = '';

    if (current > 1) {
        html += `<button onclick="loadRecords(${current - 1})">Previous</button>`;
    }

    html += `<span>Page ${current} of ${total}</span>`;

    if (current < total) {
        html += `<button onclick="loadRecords(${current + 1})">Next</button>`;
    }

    container.innerHTML = html;
}
```

## 9.6 Real-Time Updates

### 9.6.1 WebSocket Support

```go
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        return true
    },
}

func (s *Server) handleRealtime(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    // Subscribe to updates
    updates := make(chan MatchUpdate)
    s.subscribeUpdates(updates)
    defer s.unsubscribeUpdates(updates)

    for update := range updates {
        if err := conn.WriteJSON(update); err != nil {
            break
        }
    }
}
```

## 9.7 Chapter Summary

This chapter has documented the web interface:

- HTTP server configuration with Gorilla mux
- REST API endpoints for search, records, maps, and export
- Middleware for CORS and logging
- Static HTML dashboard
- JavaScript application for interactive use
- WebSocket support for real-time updates

The web interface provides operational visibility into the matching system and supports manual review workflows.

---

*This chapter covers the web interface. Chapter 10 addresses configuration and deployment.*
