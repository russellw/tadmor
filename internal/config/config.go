// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
)

// Config holds the settings the server needs to start.
type Config struct {
	DatabaseURL string // Postgres connection string (DATABASE_URL)
	HTTPAddr    string // listen address for the HTTP server (HTTP_ADDR)
}

// Load reads configuration from the environment, applying defaults and
// validating that required values are present.
func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("config: DATABASE_URL is required")
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
