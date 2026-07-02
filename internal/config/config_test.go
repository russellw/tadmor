package config_test

import (
	"testing"

	"tadmor/internal/config"
)

func TestLoad(t *testing.T) {
	t.Run("defaults HTTP_ADDR when unset", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://localhost/db")
		t.Setenv("HTTP_ADDR", "")
		t.Setenv("PORT", "")

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.DatabaseURL != "postgres://localhost/db" {
			t.Errorf("DatabaseURL = %q, want postgres://localhost/db", cfg.DatabaseURL)
		}
		if cfg.HTTPAddr != ":8080" {
			t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
		}
	})

	t.Run("uses HTTP_ADDR when set", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://localhost/db")
		t.Setenv("HTTP_ADDR", ":9090")
		t.Setenv("PORT", "")

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.HTTPAddr != ":9090" {
			t.Errorf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
		}
	})

	t.Run("PORT overrides HTTP_ADDR", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://localhost/db")
		t.Setenv("HTTP_ADDR", ":9090")
		t.Setenv("PORT", "8080")

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.HTTPAddr != ":8080" {
			t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
		}
	})

	t.Run("errors when DATABASE_URL is missing", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")
		t.Setenv("HTTP_ADDR", "")
		t.Setenv("PORT", "")

		if _, err := config.Load(); err == nil {
			t.Fatal("expected an error when DATABASE_URL is unset, got nil")
		}
	})
}
