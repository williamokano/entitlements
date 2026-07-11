package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Ensure a clean environment so defaults apply.
	for _, k := range []string{"APP_ENV", "PORT", "DATABASE_URL", "LOG_LEVEL"} {
		t.Setenv(k, "")
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if c.Environment != EnvDevelopment {
		t.Errorf("Environment = %q, want %q", c.Environment, EnvDevelopment)
	}
	if c.HTTPPort != "8080" {
		t.Errorf("HTTPPort = %q, want 8080", c.HTTPPort)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", c.LogLevel)
	}
	if c.DatabaseURL != defaultDatabaseURL {
		t.Errorf("DatabaseURL = %q, want default", c.DatabaseURL)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/app?sslmode=require")
	t.Setenv("LOG_LEVEL", "debug")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !c.IsProduction() {
		t.Error("IsProduction() = false, want true")
	}
	if c.HTTPPort != "9090" {
		t.Errorf("HTTPPort = %q, want 9090", c.HTTPPort)
	}
	if c.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", c.LogLevel)
	}
}

func TestValidateRejectsProductionWithDefaultDSN(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("LOG_LEVEL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for production with default DSN")
	}
}

func TestParseRequiredFieldFailsFast(t *testing.T) {
	type requiredConfig struct {
		Secret string `env:"CONFIG_TEST_REQUIRED,required"`
	}
	// Ensure the variable is genuinely absent (not merely empty), then restore
	// whatever the environment had afterwards.
	if orig, ok := os.LookupEnv("CONFIG_TEST_REQUIRED"); ok {
		t.Cleanup(func() { _ = os.Setenv("CONFIG_TEST_REQUIRED", orig) })
	}
	_ = os.Unsetenv("CONFIG_TEST_REQUIRED")

	if _, err := Parse[requiredConfig](); err == nil {
		t.Fatal("Parse() error = nil, want error for missing required var")
	}

	t.Setenv("CONFIG_TEST_REQUIRED", "present")
	got, err := Parse[requiredConfig]()
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if got.Secret != "present" {
		t.Errorf("Secret = %q, want present", got.Secret)
	}
}
