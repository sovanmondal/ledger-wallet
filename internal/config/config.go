// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime configuration for the service.
type Config struct {
	Port          string
	DatabaseURL   string
	LogLevel      string
	RunMigrations bool
	AllowedOrigin string
}

// Load reads configuration from environment variables, applying sane defaults.
// DATABASE_URL is the only strictly required value.
func Load() (Config, error) {
	c := Config{
		Port:          getenv("PORT", "8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		LogLevel:      getenv("LOG_LEVEL", "info"),
		RunMigrations: getbool("RUN_MIGRATIONS", true),
		AllowedOrigin: getenv("ALLOWED_ORIGIN", "*"),
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getbool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
