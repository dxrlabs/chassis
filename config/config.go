// Package config loads typed configuration from environment variables.
package config

import (
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Load parses environment variables into T and validates required fields.
// In non-production environments it optionally loads a local .env file first.
func Load[T any]() (T, error) {
	var zero T

	if !isProduction() {
		// Ignore missing .env; production must rely on the real environment.
		_ = godotenv.Load()
	}

	cfg, err := env.ParseAs[T]()
	if err != nil {
		return zero, fmt.Errorf("config: parse environment: %w", err)
	}
	return cfg, nil
}

func isProduction() bool {
	env := os.Getenv("ENV")
	if env == "" {
		env = os.Getenv("APP_ENV")
	}
	return env == "production" || env == "prod"
}
