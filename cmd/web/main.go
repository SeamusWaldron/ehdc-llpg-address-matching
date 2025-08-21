package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/ehdc-llpg/internal/config"
	"github.com/ehdc-llpg/internal/db"
	"github.com/ehdc-llpg/internal/web"
)

func main() {
	// Load environment configuration
	config.LoadEnv()

	fmt.Println("=== EHDC LLPG Web Interface ===")

	// Get configuration from environment
	portStr := config.GetEnv("WEB_PORT", "8443")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("Invalid port number: %s", portStr)
	}
	
	host := config.GetEnv("WEB_HOST", "localhost")
	dbName := config.GetEnv("DB_NAME", "ehdc_llpg")

	fmt.Printf("Server: http://%s:%d\n", host, port)
	fmt.Printf("Database: %s\n", dbName)

	// Initialize database connection
	dbConn, err := db.NewConnection()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbConn.Close()

	// Test database connectivity
	var dbVersion string
	err = dbConn.DB.QueryRow("SELECT version()").Scan(&dbVersion)
	if err != nil {
		log.Fatalf("Database connection test failed: %v", err)
	}
	fmt.Printf("Database connected successfully\n")

	// Create web server configuration using the existing structure
	webConfig := &web.Config{
		Server: web.ServerConfig{
			Port: port,
			Host: host,
		},
		Database: web.DatabaseConfig{
			URL:            fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
				config.GetEnv("DB_USER", "postgres"),
				config.GetEnv("DB_PASSWORD", "postgres"),
				config.GetEnv("DB_HOST", "localhost"),
				config.GetEnv("DB_PORT", "5432"),
				dbName),
			MaxConnections: config.GetEnvInt("DB_MAX_CONNECTIONS", 10),
		},
		Auth: web.AuthConfig{
			Enabled:    false,
			SessionKey: config.GetEnv("SESSION_KEY", "changeme"),
		},
		Features: web.FeatureConfig{
			ExportEnabled:         config.GetEnvBool("ENABLE_EXPORT", true),
			ManualOverrideEnabled: config.GetEnvBool("ENABLE_MANUAL_OVERRIDE", true),
		},
	}

	// Create web server (it already has the DB connection from internal/web/server.go)
	server, err := web.NewServer(webConfig)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	fmt.Printf("\nStarting web server on http://%s:%d\n", host, port)
	fmt.Println("\nFeatures enabled:")
	fmt.Printf("  • Export: %v\n", webConfig.Features.ExportEnabled)
	fmt.Printf("  • Manual Override: %v\n", webConfig.Features.ManualOverrideEnabled) 
	fmt.Println()

	// Start server
	if err := server.Start(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}