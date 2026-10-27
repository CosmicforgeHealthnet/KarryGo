// Package config loads pricing-service settings from the environment.
//
// Pricing decides money, so nothing here defaults to a permissive value: the
// internal API refuses callers it has no secret for, and four-eye approval
// cannot be switched off in production. Commercial values (rates, commission,
// minimum fares, caps) are deliberately ABSENT from this file -- they are
// versioned policy data held in the database and changed through the approval
// process, never through an environment variable or a Go constant.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// Config is the full service configuration.
type Config struct {
	AppEnv      string
	HTTPAddr    string
	Migration   bool
	DatabaseURL string

	// ServiceSecrets are the HMAC secrets of every service allowed to call
	// pricing-service's internal routes ("caller=secret,...").
	ServiceSecrets serviceauth.Secrets
	// QuoteCallers may ask for, bind and reconcile quotes. Holding a secret is
	// not by itself permission to price: a caller must also be listed here.
	QuoteCallers []string
	// AdminCallers may use the policy, rollout and index administration routes.
	AdminCallers []string

	// FourEyeRequired keeps creator and approver separate for material
	// changes. It cannot be false in production.
	FourEyeRequired bool

	// MaxRouteAge rejects a route snapshot that is too old to describe current
	// conditions. Quotes are priced from a caller-supplied route; a stale
	// snapshot is a silent accuracy loss.
	MaxRouteAge time.Duration
	// PolicyCacheRefresh is how often the active-activation list is re-read.
	// Policy bodies are immutable and cached indefinitely; only the question
	// "which version is active now" needs refreshing.
	PolicyCacheRefresh time.Duration
	// QuoteRetention is how long an UNBOUND, expired quote is kept before it
	// is purged. Bound quotes and reconciliations are financial records and
	// are never purged by this setting.
	QuoteRetention time.Duration

	// Outbox relay. Both empty leaves events durable in the outbox table,
	// undelivered, which is the correct default until a consumer exists.
	OutboxRelayURL    string
	OutboxRelaySecret []byte

	Workers Workers
}

// Workers tunes the background workers.
type Workers struct {
	NoncePruneInterval  time.Duration
	OutboxRelayInterval time.Duration
	RetentionInterval   time.Duration
	BatchSize           int
}

// Load reads the environment.
func Load() Config {
	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8115"),
		Migration:   getEnvBool("PRICING_MIGRATION", false),
		DatabaseURL: getEnv("PRICING_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5451/pricing_service?sslmode=disable"),

		ServiceSecrets: serviceauth.ParseSecrets(os.Getenv("PRICING_SERVICE_SECRETS")),
		QuoteCallers: splitCSV(getEnv("PRICING_QUOTE_CALLERS",
			"driver-taxi-service,driver-dispatch-delivery-service,driver-hauling-service,api-gateway")),
		AdminCallers: splitCSV(getEnv("PRICING_ADMIN_CALLERS", "admin-backoffice-service,pricing-ops-cli")),

		FourEyeRequired: getEnvBool("PRICING_FOUR_EYE_REQUIRED", true),

		MaxRouteAge:        time.Duration(getEnvInt("PRICING_MAX_ROUTE_AGE_SECONDS", 900)) * time.Second,
		PolicyCacheRefresh: time.Duration(getEnvInt("PRICING_POLICY_CACHE_REFRESH_SECONDS", 30)) * time.Second,
		QuoteRetention:     time.Duration(getEnvInt("PRICING_QUOTE_RETENTION_DAYS", 30)) * 24 * time.Hour,

		OutboxRelayURL:    os.Getenv("PRICING_OUTBOX_RELAY_URL"),
		OutboxRelaySecret: []byte(os.Getenv("PRICING_OUTBOX_RELAY_SECRET")),

		Workers: Workers{
			NoncePruneInterval:  time.Duration(getEnvInt("PRICING_NONCE_PRUNE_INTERVAL_SECONDS", 600)) * time.Second,
			OutboxRelayInterval: time.Duration(getEnvInt("PRICING_OUTBOX_RELAY_INTERVAL_SECONDS", 10)) * time.Second,
			RetentionInterval:   time.Duration(getEnvInt("PRICING_RETENTION_INTERVAL_SECONDS", 3600)) * time.Second,
			BatchSize:           getEnvInt("PRICING_WORKER_BATCH_SIZE", 100),
		},
	}
}

// Validate reports every configuration problem at once, so one restart shows
// the whole list rather than the first mistake.
func (c Config) Validate() error {
	var problems []string

	if strings.TrimSpace(c.DatabaseURL) == "" {
		problems = append(problems, "PRICING_DATABASE_URL is required")
	}

	// Four-eye approval is the control that stops one person changing prices
	// for a whole market. Production may not run without it.
	if c.AppEnv == "production" && !c.FourEyeRequired {
		problems = append(problems, "PRICING_FOUR_EYE_REQUIRED cannot be false when APP_ENV=production")
	}

	// A caller allowlist without a secret for that caller can never succeed,
	// and is nearly always a deployment mistake rather than an intention.
	if len(c.ServiceSecrets) > 0 {
		for _, caller := range append(append([]string{}, c.QuoteCallers...), c.AdminCallers...) {
			if _, ok := c.ServiceSecrets[caller]; !ok {
				problems = append(problems, fmt.Sprintf(
					"caller %q is allowed but PRICING_SERVICE_SECRETS has no secret for it", caller))
			}
		}
	}
	if c.AppEnv == "production" && len(c.ServiceSecrets) == 0 {
		problems = append(problems, "PRICING_SERVICE_SECRETS is required when APP_ENV=production")
	}

	if (c.OutboxRelayURL == "") != (len(c.OutboxRelaySecret) == 0) {
		problems = append(problems, "PRICING_OUTBOX_RELAY_URL and PRICING_OUTBOX_RELAY_SECRET must be set together")
	}
	if c.OutboxRelayURL != "" {
		if parsed, err := url.ParseRequestURI(c.OutboxRelayURL); err != nil || parsed.Host == "" {
			problems = append(problems, "PRICING_OUTBOX_RELAY_URL must be an absolute URL (bare origin)")
		}
	}

	if c.MaxRouteAge <= 0 {
		problems = append(problems, "PRICING_MAX_ROUTE_AGE_SECONDS must be positive")
	}
	if c.PolicyCacheRefresh <= 0 {
		problems = append(problems, "PRICING_POLICY_CACHE_REFRESH_SECONDS must be positive")
	}
	if c.QuoteRetention <= 0 {
		problems = append(problems, "PRICING_QUOTE_RETENTION_DAYS must be positive")
	}
	if c.Workers.BatchSize < 1 {
		problems = append(problems, "PRICING_WORKER_BATCH_SIZE must be at least 1")
	}
	if c.Workers.NoncePruneInterval <= 0 || c.Workers.OutboxRelayInterval <= 0 || c.Workers.RetentionInterval <= 0 {
		problems = append(problems, "worker intervals must be positive")
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid pricing-service configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

func splitCSV(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
