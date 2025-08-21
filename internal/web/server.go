package web

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"

	"github.com/ehdc-llpg/internal/web/handlers"
	"github.com/ehdc-llpg/internal/web/middleware"
)

// Server represents the web server
type Server struct {
	config     *Config
	db         *sql.DB
	httpServer *http.Server
	router     *mux.Router
}

// NewServer creates a new web server instance
func NewServer(config *Config) (*Server, error) {
	// Initialize database connection
	db, err := sql.Open("postgres", config.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure database connection pool
	db.SetMaxOpenConns(config.Database.MaxConnections)
	db.SetMaxIdleConns(config.Database.MaxConnections / 2)
	db.SetConnMaxLifetime(time.Hour)

	// Test database connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Create server instance
	server := &Server{
		config: config,
		db:     db,
	}

	// Setup routes
	server.setupRoutes()

	// Create HTTP server
	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port),
		Handler:      server.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server, nil
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	s.router = mux.NewRouter()

	// Convert config for handlers (to avoid import cycle)
	handlerConfig := &handlers.Config{}
	handlerConfig.Features.ExportEnabled = s.config.Features.ExportEnabled
	handlerConfig.Features.ManualOverrideEnabled = s.config.Features.ManualOverrideEnabled

	// Create handlers with database access
	apiHandler := &handlers.APIHandler{DB: s.db, Config: handlerConfig}
	recordsHandler := &handlers.RecordsHandler{DB: s.db, Config: handlerConfig}
	mapsHandler := &handlers.MapsHandler{DB: s.db, Config: handlerConfig}
	searchHandler := &handlers.SearchHandler{DB: s.db, Config: handlerConfig}
	exportHandler := &handlers.ExportHandler{DB: s.db, Config: handlerConfig}
	realtimeHandler := &handlers.RealtimeHandler{DB: s.db, Config: handlerConfig}

	// API routes
	api := s.router.PathPrefix("/api").Subrouter()

	// Core data endpoints
	api.HandleFunc("/records", recordsHandler.ListRecords).Methods("GET")
	api.HandleFunc("/records/geojson", mapsHandler.GetGeoJSON).Methods("GET")
	api.HandleFunc("/records/{id:[0-9]+}", recordsHandler.GetRecord).Methods("GET")
	api.HandleFunc("/records/{id:[0-9]+}/candidates", recordsHandler.GetCandidates).Methods("GET")

	// Modification endpoints (if features enabled)
	if s.config.Features.ManualOverrideEnabled {
		api.HandleFunc("/records/{id:[0-9]+}/accept", recordsHandler.AcceptMatch).Methods("POST")
		api.HandleFunc("/records/{id:[0-9]+}/coordinates", recordsHandler.SetCoordinates).Methods("PUT")
		api.HandleFunc("/records/{id:[0-9]+}/reject", recordsHandler.RejectMatch).Methods("POST")
	}

	// Audit and history endpoints
	api.HandleFunc("/records/{id:[0-9]+}/history", recordsHandler.GetHistory).Methods("GET")
	api.HandleFunc("/records/{id:[0-9]+}/notes", recordsHandler.AddNote).Methods("POST")

	// Search endpoints
	api.HandleFunc("/search/llpg", searchHandler.SearchLLPG).Methods("GET")
	api.HandleFunc("/search/records", searchHandler.SearchRecords).Methods("GET")

	// Export endpoint (if enabled)
	if s.config.Features.ExportEnabled {
		api.HandleFunc("/export", exportHandler.ExportData).Methods("POST")
	}

	// Statistics endpoints
	api.HandleFunc("/stats", apiHandler.GetStats).Methods("GET")
	api.HandleFunc("/stats/viewport", apiHandler.GetViewportStats).Methods("GET")

	// Real-time update endpoints
	api.HandleFunc("/updates/stream", realtimeHandler.SSEUpdates).Methods("GET")
	api.HandleFunc("/updates/status", realtimeHandler.MatchingStatus).Methods("GET")
	api.HandleFunc("/updates/refresh", realtimeHandler.TriggerRefresh).Methods("POST")

	// Static file serving
	staticDir := "internal/web/static"
	if _, err := os.Stat(staticDir); err == nil {
		s.router.PathPrefix("/").Handler(http.FileServer(http.Dir(staticDir + "/")))
	}

	// Apply middleware
	s.router.Use(middleware.CORS())
	s.router.Use(middleware.RequestLogging())
	
	if s.config.Auth.Enabled {
		// Apply authentication middleware to API routes only
		api.Use(middleware.Authentication(s.config.Auth.SessionKey))
	}
}

// Start starts the web server
func (s *Server) Start() error {
	// Setup graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Start server in background
	go func() {
		fmt.Printf("Starting server on http://%s\n", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	// Wait for shutdown signal
	<-stop
	fmt.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		fmt.Printf("Server shutdown error: %v\n", err)
	}

	// Close database connection
	if err := s.db.Close(); err != nil {
		fmt.Printf("Database close error: %v\n", err)
	}

	fmt.Println("Server stopped")
	return nil
}