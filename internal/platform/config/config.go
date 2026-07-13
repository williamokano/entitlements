// Package config loads application configuration from the environment.
//
// It provides fail-fast parsing of required variables with sane development
// defaults. The generic Parse helper loads an optional .env file (useful in
// development) and then binds environment variables onto a struct via
// `env` struct tags.
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Environment identifies the deployment environment.
type Environment string

const (
	// EnvDevelopment is the default local environment.
	EnvDevelopment Environment = "development"
	// EnvProduction is the hardened, deployed environment.
	EnvProduction Environment = "production"

	// defaultDatabaseURL matches docker-compose.yml and is intended for local
	// development only. Production must override it (see Validate).
	defaultDatabaseURL = "postgres://entitlements:entitlements@localhost:5432/entitlements?sslmode=disable"
)

// Config is the application configuration. Fields carry development defaults
// so the binary runs out of the box; production deployments override them via
// the environment.
type Config struct {
	Environment Environment `env:"APP_ENV" envDefault:"development"`
	HTTPPort    string      `env:"PORT" envDefault:"8080"`
	DatabaseURL string      `env:"DATABASE_URL" envDefault:"postgres://entitlements:entitlements@localhost:5432/entitlements?sslmode=disable"`
	// MigrationDatabaseURL, when set, is the DSN used to apply migrations (DDL).
	// It lets migrations run as a privileged owner role while the app serves
	// requests as a least-privilege runtime role via DatabaseURL. Empty ⇒
	// migrations run with DatabaseURL (single-role setup).
	MigrationDatabaseURL string `env:"MIGRATION_DATABASE_URL"`
	LogLevel             string `env:"LOG_LEVEL" envDefault:"info"`

	// CORSAllowedOrigins lists the browser origins allowed to call the API
	// cross-origin (the admin SPA runs on a different origin than the API). Use
	// "*" to allow any origin. Comma-separated in the environment.
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`

	// Observability. OTLPEndpoint is the OTLP/HTTP collector URL; when empty,
	// tracing/metrics are created but not exported (no-op).
	ServiceName  string `env:"OTEL_SERVICE_NAME" envDefault:"entitlements"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`

	// BillingDisabled short-circuits the money side: renewals auto-advance the
	// subscription period instead of waiting for an InvoicePaid event. Defaults
	// to true until the billing module (T-025/026) lands.
	BillingDisabled bool `env:"BILLING_DISABLED" envDefault:"true"`
	// TrialEndingDays is how many days before a trial ends the TrialEnding event
	// fires.
	TrialEndingDays int `env:"SUBSCRIPTION_TRIAL_ENDING_DAYS" envDefault:"3"`

	// EntitlementsUnknownFeaturePolicy decides how resolution treats a feature key
	// referenced by a plan/addon/override but absent from the registry: "deny"
	// (default) drops it, "allow" keeps it.
	EntitlementsUnknownFeaturePolicy string `env:"ENTITLEMENTS_UNKNOWN_FEATURE_POLICY" envDefault:"deny"`
}

// IsProduction reports whether the configuration targets production.
func (c Config) IsProduction() bool { return c.Environment == EnvProduction }

// MigrationDSN returns the DSN to apply migrations with: MigrationDatabaseURL
// when set (privileged owner role), otherwise DatabaseURL (single-role setup).
func (c Config) MigrationDSN() string {
	if strings.TrimSpace(c.MigrationDatabaseURL) != "" {
		return c.MigrationDatabaseURL
	}
	return c.DatabaseURL
}

// Validate enforces cross-field invariants that a plain parse cannot.
func (c Config) Validate() error {
	if c.IsProduction() && c.DatabaseURL == defaultDatabaseURL {
		return errors.New("config: DATABASE_URL must be set to a non-default value in production")
	}
	if strings.TrimSpace(c.HTTPPort) == "" {
		return errors.New("config: PORT must not be empty")
	}
	return nil
}

// Load reads the application Config from the environment (loading an optional
// .env file first) and validates it.
func Load() (Config, error) {
	c, err := Parse[Config]()
	if err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Parse loads an optional .env file (ignored if absent) and binds environment
// variables onto a fresh value of T using `env` struct tags. It is generic so
// each module can define and load its own typed configuration.
func Parse[T any]() (T, error) {
	loadDotEnv()

	var out T
	if err := env.Parse(&out); err != nil {
		return out, fmt.Errorf("config: parse environment: %w", err)
	}
	return out, nil
}

// loadDotEnv loads a .env file if one exists in the working directory. A
// missing file is not an error; any other failure is ignored here so that
// real configuration errors surface during Parse.
func loadDotEnv() {
	_ = godotenv.Load()
}
