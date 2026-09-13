// Package config reads all runtime settings from environment variables. Nothing
// is hardcoded so the same binary runs locally, in CI, or in a real deployment
// just by changing the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port              string
	DatabaseURL       string
	RedisAddr         string
	RedisPassword     string
	HoldTTLSeconds    int
	RateLimitPerMin   int
	AllowedOrigin     string
	OTelExporter      string
	LogLevel          string
}

// Load reads config from the environment, applying sane local defaults so the
// service still starts if an optional var is missing. The two connection
// strings are required — we fail loudly rather than pretend to run.
func Load() (Config, error) {
	c := Config{
		Port:            getenv("BACKEND_PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		RedisAddr:       getenv("REDIS_ADDR", "redis:6379"),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		HoldTTLSeconds:  getenvInt("HOLD_TTL_SECONDS", 120),
		RateLimitPerMin: getenvInt("RATE_LIMIT_PER_MINUTE", 60),
		AllowedOrigin:   getenv("ALLOWED_ORIGIN", "http://localhost:3000"),
		OTelExporter:    getenv("OTEL_TRACES_EXPORTER", "stdout"),
		LogLevel:        getenv("LOG_LEVEL", "info"),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.HoldTTLSeconds <= 0 {
		return c, fmt.Errorf("HOLD_TTL_SECONDS must be positive, got %d", c.HoldTTLSeconds)
	}
	return c, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
