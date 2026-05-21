package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration read from HERMES_* environment variables.
type Config struct {
	// Source database
	SourceType     string
	SourceHost     string
	SourcePort     int32
	SourceDatabase string
	SourceUsername string
	SourcePassword string
	SourceSSLMode  string

	// Destination database
	DestType     string
	DestHost     string
	DestPort     int32
	DestDatabase string
	DestUsername string
	DestPassword string
	DestSSLMode  string

	// Migration options
	MigrateSchema bool
	MigrateData   bool
	Tables        []string
	BatchSize     int32
}

// Load reads configuration from environment variables and returns a validated Config.
func Load() (*Config, error) {
	cfg := &Config{
		SourceType:     getEnv("HERMES_SOURCE_TYPE", ""),
		SourceHost:     getEnv("HERMES_SOURCE_HOST", ""),
		SourceDatabase: getEnv("HERMES_SOURCE_DATABASE", ""),
		SourceUsername: getEnv("HERMES_SOURCE_USERNAME", ""),
		SourcePassword: getEnv("HERMES_SOURCE_PASSWORD", ""),
		SourceSSLMode:  getEnv("HERMES_SOURCE_SSL_MODE", ""),

		DestType:     getEnv("HERMES_DEST_TYPE", ""),
		DestHost:     getEnv("HERMES_DEST_HOST", ""),
		DestDatabase: getEnv("HERMES_DEST_DATABASE", ""),
		DestUsername: getEnv("HERMES_DEST_USERNAME", ""),
		DestPassword: getEnv("HERMES_DEST_PASSWORD", ""),
		DestSSLMode:  getEnv("HERMES_DEST_SSL_MODE", ""),

		MigrateSchema: getBoolEnv("HERMES_MIGRATE_SCHEMA", true),
		MigrateData:   getBoolEnv("HERMES_MIGRATE_DATA", true),
		BatchSize:     int32(getIntEnv("HERMES_BATCH_SIZE", 1000)),
	}

	// Parse ports with type-specific defaults
	cfg.SourcePort = int32(getPortEnv("HERMES_SOURCE_PORT", defaultPort(cfg.SourceType)))
	cfg.DestPort = int32(getPortEnv("HERMES_DEST_PORT", defaultPort(cfg.DestType)))

	// Parse tables list
	tablesStr := getEnv("HERMES_TABLES", "")
	if tablesStr != "" {
		for _, t := range strings.Split(tablesStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				cfg.Tables = append(cfg.Tables, t)
			}
		}
	}

	// Validate required fields
	if cfg.SourceType == "" {
		return nil, fmt.Errorf("HERMES_SOURCE_TYPE is required")
	}
	if cfg.SourceHost == "" {
		return nil, fmt.Errorf("HERMES_SOURCE_HOST is required")
	}
	if cfg.SourceDatabase == "" {
		return nil, fmt.Errorf("HERMES_SOURCE_DATABASE is required")
	}
	if cfg.DestType == "" {
		return nil, fmt.Errorf("HERMES_DEST_TYPE is required")
	}
	if cfg.DestHost == "" {
		return nil, fmt.Errorf("HERMES_DEST_HOST is required")
	}
	if cfg.DestDatabase == "" {
		return nil, fmt.Errorf("HERMES_DEST_DATABASE is required")
	}

	return cfg, nil
}

func defaultPort(dbType string) int {
	switch strings.ToLower(dbType) {
	case "mysql":
		return 3306
	case "postgresql", "postgres":
		return 5432
	default:
		return 0
	}
}

func getEnv(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}

func getBoolEnv(key string, defaultVal bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}

func getIntEnv(key string, defaultVal int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func getPortEnv(key string, defaultVal int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}
