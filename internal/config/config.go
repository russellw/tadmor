// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
)

// Config holds the settings the server needs to start.
type Config struct {
	DatabaseURL string // Postgres connection string (DATABASE_URL)
	HTTPAddr    string // listen address for the HTTP server (HTTP_ADDR, or PORT)
}

// Load reads configuration from the environment, applying defaults and
// validating that required values are present.
func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
	}
	// Container platforms such as Cloud Run inject the listen port via PORT;
	// honour it when set so the server binds where the platform expects.
	if p := os.Getenv("PORT"); p != "" {
		c.HTTPAddr = ":" + p
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
