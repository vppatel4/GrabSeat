package config

import (
	"testing"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is unset")
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("HOLD_TTL_SECONDS", "")
	t.Setenv("RATE_LIMIT_PER_MINUTE", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HoldTTLSeconds != 120 {
		t.Errorf("HoldTTLSeconds default = %d, want 120", cfg.HoldTTLSeconds)
	}
	if cfg.RateLimitPerMin != 60 {
		t.Errorf("RateLimitPerMin default = %d, want 60", cfg.RateLimitPerMin)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port default = %q, want 8080", cfg.Port)
	}
}

func TestLoadRejectsNonPositiveTTL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("HOLD_TTL_SECONDS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-positive HOLD_TTL_SECONDS")
	}
}
