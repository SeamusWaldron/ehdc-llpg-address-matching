package web

import (
	"encoding/json"
	"os"
)

// Config represents the web server configuration
type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	Auth     AuthConfig     `json:"auth"`
	Features FeatureConfig  `json:"features"`
}

// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

// DatabaseConfig contains database connection settings
type DatabaseConfig struct {
	URL            string `json:"url"`
	MaxConnections int    `json:"max_connections"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	Enabled    bool   `json:"enabled"`
	SessionKey string `json:"session_key"`
}

// FeatureConfig contains feature toggles
type FeatureConfig struct {
	ExportEnabled         bool `json:"export_enabled"`
	ManualOverrideEnabled bool `json:"manual_override_enabled"`
}

// LoadConfig loads configuration from a JSON file
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
		Database: DatabaseConfig{
			URL:            "postgres://postgres:password@localhost:15432/ehdc_gis?sslmode=disable",
			MaxConnections: 25,
		},
		Auth: AuthConfig{
			Enabled:    false,
			SessionKey: "development-session-key",
		},
		Features: FeatureConfig{
			ExportEnabled:         true,
			ManualOverrideEnabled: true,
		},
	}
}