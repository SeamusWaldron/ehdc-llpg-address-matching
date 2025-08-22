package config

import (
	"os"
	"strconv"
	"strings"
)

// LoadConfig loads configuration from file - alias for LoadEnv for compatibility
func LoadConfig(configFile string) error {
	return LoadEnv()
}

// LoadEnv loads environment variables from .env file
func LoadEnv() error {
	// Try to load from .env file in current directory first, then parent directories
	envPaths := []string{".env", "../.env", "../../.env"}
	
	for _, envPath := range envPaths {
		if data, err := os.ReadFile(envPath); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					
					// Only set if not already set
					if os.Getenv(key) == "" {
						os.Setenv(key, value)
					}
				}
			}
			break // Successfully loaded, don't try other paths
		}
	}
	return nil
}

// GetEnv gets environment variable with default
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvInt gets integer environment variable with default
func GetEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// GetEnvFloat gets float environment variable with default
func GetEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

// GetEnvBool gets boolean environment variable with default
func GetEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		switch strings.ToLower(value) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return defaultValue
}