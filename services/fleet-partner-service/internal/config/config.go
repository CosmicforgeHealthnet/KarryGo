// Package config loads and validates the service configuration.
//
// Mandatory settings fail startup when missing. Secrets are never printed.
// Env names follow the repository prefix convention (FLEET_PARTNER_*) with
// the common unprefixed vars APP_ENV, HTTP_ADDR and MIGRATION; the mapping
// from the specification's generic names is documented in
// docs/configuration.md.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/redisx"
)

// ServiceName is the canonical service identity used in logs, events,
// service-auth headers and metrics.
const ServiceName = "fleet-partner-service"

// CorporateRole is the access-token role carried by corporate users
// (organization owners and staff).
const CorporateRole = "organization_user"

// ClientConfig configures one downstream platform service client.
type ClientConfig struct {
	BaseURL string
	Secret  []byte
}

// Configured reports whether the client can make live calls.
func (c ClientConfig) Configured() bool {
	return c.BaseURL != "" && len(c.Secret) > 0
}

// Config is the validated service configuration.
type Config struct {
	AppEnv        string
	HTTPAddr      string
	Migration     bool
	WorkerEnabled bool
	LogLevel      string

	DatabaseURL                string
	DatabaseMaxOpenConnections int32
	DatabaseMaxIdleConnections int32
	DatabaseConnectionMaxLife  time.Duration
	RedisRequired              bool
	Redis                      redisx.Config

	AccessTokenSecret []byte
	// ServiceSecrets holds per-caller HMAC secrets for internal routes
	// ("name=secret,..." format, parsed by shared serviceauth).
	ServiceSecrets string

	// Dispatch execution service, used by the execution-start worker to create
	// the canonical trip. Empty secret => the adapter is unconfigured and the
	// worker does not start.
	DispatchServiceURL     string
	DispatchServiceSecret  []byte
	DispatchServiceTimeout time.Duration

	// Marketplace event ingestion and projection reconciliation.
	MarketplaceEventIngestionEnabled bool
	MarketplaceAllowedCallers        string
	MarketplaceReconciliationEnabled bool
	MarketplaceProjectionStaleAfter  time.Duration
	MarketplaceReconcileInterval     time.Duration
	MarketplaceReconcileBatchSize    int

	// Execution-start worker.
	DispatchExecutionStartEnabled bool
	ExecutionStartBatchSize       int
	ExecutionStartLease           time.Duration
	ExecutionStartMaxAttempts     int
	ExecutionStartBackoffBase     time.Duration
	ExecutionStartInterval        time.Duration

	MetricsEnabled   bool
	MetricsAuthToken string

	Clients map[string]ClientConfig

	HTTPClientTimeout    time.Duration
	ShutdownTimeout      time.Duration
	OutboxPollInterval   time.Duration
	JobPollInterval      time.Duration
	JobLeaseDuration     time.Duration
	JobHeartbeatInterval time.Duration

	MaxRequestBodyBytes int64

	// --- Section 8–10 authentication settings ---

	// OTPSecret peppers the keyed OTP digest. Mandatory: without it, a
	// six-digit code would be brute-forceable from a leaked table.
	OTPSecret []byte
	// SessionSecret hashes refresh tokens and invitation tokens.
	SessionSecret []byte
	// ContactSecret hashes email/phone for rate limits and security events.
	ContactSecret []byte

	OTPLength         int
	OTPExpiry         time.Duration
	OTPMaxAttempts    int
	OTPResendCooldown time.Duration
	OTPMaxPerHour     int

	AccessTokenTTL        time.Duration
	RefreshTokenTTL       time.Duration
	StepUpTTL             time.Duration
	StaffInvitationTTL    time.Duration
	ProviderInvitationTTL time.Duration

	DefaultCountryCode    string
	StaffAcceptURL        string
	ProviderAcceptURL     string
	AuthorizationCacheTTL time.Duration
}

// Load reads, validates and returns the configuration. It returns an error
// (never logs secret values) when mandatory settings are missing or invalid.
func Load() (Config, error) {
	cfg := Config{
		AppEnv:        getEnv("APP_ENV", "development"),
		HTTPAddr:      getEnv("HTTP_ADDR", ":8113"),
		Migration:     getEnvBool("MIGRATION", false),
		WorkerEnabled: getEnvBool("FLEET_PARTNER_WORKER_ENABLED", true),
		LogLevel:      getEnv("FLEET_PARTNER_LOG_LEVEL", "info"),

		DatabaseURL:                getEnv("FLEET_PARTNER_DATABASE_URL", ""),
		DatabaseMaxOpenConnections: int32(getEnvInt("FLEET_PARTNER_DATABASE_MAX_OPEN_CONNECTIONS", 10)),
		DatabaseMaxIdleConnections: int32(getEnvInt("FLEET_PARTNER_DATABASE_MAX_IDLE_CONNECTIONS", 2)),
		DatabaseConnectionMaxLife:  getEnvDuration("FLEET_PARTNER_DATABASE_CONNECTION_MAX_LIFETIME", time.Hour),

		RedisRequired: getEnvBool("FLEET_PARTNER_REDIS_REQUIRED", false),
		Redis: redisx.Config{
			Addr:     getEnv("FLEET_PARTNER_REDIS_ADDR", ""),
			Password: os.Getenv("FLEET_PARTNER_REDIS_PASSWORD"),
			DB:       getEnvInt("FLEET_PARTNER_REDIS_DB", 0),
		},

		AccessTokenSecret: []byte(getEnv("FLEET_PARTNER_ACCESS_TOKEN_SECRET", "")),
		ServiceSecrets:    getEnv("FLEET_PARTNER_SERVICE_SECRETS", ""),

		DispatchServiceURL:     getEnv("DISPATCH_SERVICE_URL", ""),
		DispatchServiceSecret:  []byte(getEnv("DISPATCH_SERVICE_SECRET", "")),
		DispatchServiceTimeout: time.Duration(getEnvInt("DISPATCH_SERVICE_TIMEOUT_SECONDS", 10)) * time.Second,

		MarketplaceEventIngestionEnabled: getEnvBool("FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED", false),
		MarketplaceAllowedCallers:        getEnv("FLEET_MARKETPLACE_ALLOWED_CALLERS", ""),
		MarketplaceReconciliationEnabled: getEnvBool("FLEET_MARKETPLACE_RECONCILIATION_ENABLED", false),
		MarketplaceProjectionStaleAfter:  time.Duration(getEnvInt("FLEET_MARKETPLACE_PROJECTION_STALE_AFTER_SECONDS", 900)) * time.Second,
		MarketplaceReconcileInterval:     time.Duration(getEnvInt("FLEET_MARKETPLACE_RECONCILE_INTERVAL_SECONDS", 30)) * time.Second,
		MarketplaceReconcileBatchSize:    getEnvInt("FLEET_MARKETPLACE_RECONCILE_BATCH_SIZE", 25),

		DispatchExecutionStartEnabled: getEnvBool("FLEET_DISPATCH_EXECUTION_START_ENABLED", false),
		ExecutionStartBatchSize:       getEnvInt("FLEET_EXECUTION_START_BATCH_SIZE", 10),
		ExecutionStartLease:           time.Duration(getEnvInt("FLEET_EXECUTION_START_LEASE_SECONDS", 120)) * time.Second,
		ExecutionStartMaxAttempts:     getEnvInt("FLEET_EXECUTION_START_MAX_ATTEMPTS", 8),
		ExecutionStartBackoffBase:     time.Duration(getEnvInt("FLEET_EXECUTION_START_BACKOFF_SECONDS", 5)) * time.Second,
		ExecutionStartInterval:        time.Duration(getEnvInt("FLEET_EXECUTION_START_INTERVAL_SECONDS", 5)) * time.Second,

		MetricsEnabled:   getEnvBool("FLEET_PARTNER_METRICS_ENABLED", true),
		MetricsAuthToken: getEnv("FLEET_PARTNER_METRICS_AUTH_TOKEN", ""),

		HTTPClientTimeout:    getEnvDuration("FLEET_PARTNER_HTTP_CLIENT_TIMEOUT", 10*time.Second),
		ShutdownTimeout:      getEnvDuration("FLEET_PARTNER_SHUTDOWN_TIMEOUT", 15*time.Second),
		OutboxPollInterval:   getEnvDuration("FLEET_PARTNER_OUTBOX_POLL_INTERVAL", 2*time.Second),
		JobPollInterval:      getEnvDuration("FLEET_PARTNER_JOB_POLL_INTERVAL", 2*time.Second),
		JobLeaseDuration:     getEnvDuration("FLEET_PARTNER_JOB_LEASE_DURATION", 60*time.Second),
		JobHeartbeatInterval: getEnvDuration("FLEET_PARTNER_JOB_HEARTBEAT_INTERVAL", 15*time.Second),

		MaxRequestBodyBytes: int64(getEnvInt("FLEET_PARTNER_MAX_REQUEST_BODY_BYTES", 1<<20)),

		OTPSecret:     secret("FLEET_PARTNER_OTP_HMAC_SECRET"),
		SessionSecret: secret("FLEET_PARTNER_SESSION_HASH_SECRET"),
		ContactSecret: secret("FLEET_PARTNER_CONTACT_HASH_SECRET"),

		OTPLength:         getEnvInt("FLEET_PARTNER_OTP_LENGTH", 6),
		OTPExpiry:         getEnvDuration("FLEET_PARTNER_OTP_EXPIRY", 10*time.Minute),
		OTPMaxAttempts:    getEnvInt("FLEET_PARTNER_OTP_MAX_ATTEMPTS", 5),
		OTPResendCooldown: getEnvDuration("FLEET_PARTNER_OTP_RESEND_COOLDOWN", 60*time.Second),
		OTPMaxPerHour:     getEnvInt("FLEET_PARTNER_OTP_MAX_REQUESTS_PER_HOUR", 10),

		AccessTokenTTL:        getEnvDuration("FLEET_PARTNER_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:       getEnvDuration("FLEET_PARTNER_REFRESH_TOKEN_TTL", 720*time.Hour),
		StepUpTTL:             getEnvDuration("FLEET_PARTNER_STEP_UP_TTL", 5*time.Minute),
		StaffInvitationTTL:    getEnvDuration("FLEET_PARTNER_STAFF_INVITATION_TTL", 168*time.Hour),
		ProviderInvitationTTL: getEnvDuration("FLEET_PARTNER_PROVIDER_INVITATION_TTL", 336*time.Hour),

		DefaultCountryCode:    getEnv("FLEET_PARTNER_DEFAULT_COUNTRY_CODE", "234"),
		StaffAcceptURL:        getEnv("FLEET_PARTNER_STAFF_ACCEPT_URL", ""),
		ProviderAcceptURL:     getEnv("FLEET_PARTNER_PROVIDER_ACCEPT_URL", ""),
		AuthorizationCacheTTL: getEnvDuration("FLEET_PARTNER_AUTHORIZATION_CACHE_TTL", 30*time.Second),
	}

	cfg.Clients = map[string]ClientConfig{
		"customer-service":                 {BaseURL: getEnv("FLEET_PARTNER_CUSTOMER_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_CUSTOMER_SERVICE_SECRET")},
		"driver-dispatch-delivery-service": {BaseURL: getEnv("FLEET_PARTNER_DISPATCH_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_DISPATCH_SERVICE_SECRET")},
		"driver-taxi-service":              {BaseURL: getEnv("FLEET_PARTNER_TAXI_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_TAXI_SERVICE_SECRET")},
		"driver-hauling-service":           {BaseURL: getEnv("FLEET_PARTNER_HAULING_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_HAULING_SERVICE_SECRET")},
		"payment-wallet-service":           {BaseURL: getEnv("FLEET_PARTNER_PAYMENT_WALLET_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_PAYMENT_WALLET_SERVICE_SECRET")},
		"notification-service":             {BaseURL: getEnv("FLEET_PARTNER_NOTIFICATION_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_NOTIFICATION_SERVICE_SECRET")},
		"media-file-service":               {BaseURL: getEnv("FLEET_PARTNER_MEDIA_FILE_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_MEDIA_FILE_SERVICE_SECRET")},
		"verification-compliance-service":  {BaseURL: getEnv("FLEET_PARTNER_VERIFICATION_COMPLIANCE_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_VERIFICATION_COMPLIANCE_SERVICE_SECRET")},
		"support-dispute-service":          {BaseURL: getEnv("FLEET_PARTNER_SUPPORT_DISPUTE_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_SUPPORT_DISPUTE_SERVICE_SECRET")},
		"analytics-service":                {BaseURL: getEnv("FLEET_PARTNER_ANALYTICS_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_ANALYTICS_SERVICE_SECRET")},
		"admin-backoffice-service":         {BaseURL: getEnv("FLEET_PARTNER_ADMIN_BACKOFFICE_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_ADMIN_BACKOFFICE_SERVICE_SECRET")},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate enforces mandatory settings and value constraints.
func (c Config) Validate() error {
	var problems []string

	if c.DatabaseURL == "" {
		problems = append(problems, "FLEET_PARTNER_DATABASE_URL is required")
	}
	if len(c.AccessTokenSecret) == 0 {
		problems = append(problems, "FLEET_PARTNER_ACCESS_TOKEN_SECRET is required")
	}
	// Authentication secrets have no defaults in any environment: a
	// predictable OTP pepper would make a six-digit code guessable offline.
	if len(c.OTPSecret) == 0 {
		problems = append(problems, "FLEET_PARTNER_OTP_HMAC_SECRET is required")
	}
	if len(c.SessionSecret) == 0 {
		problems = append(problems, "FLEET_PARTNER_SESSION_HASH_SECRET is required")
	}
	if len(c.ContactSecret) == 0 {
		problems = append(problems, "FLEET_PARTNER_CONTACT_HASH_SECRET is required")
	}
	if c.OTPLength < 6 || c.OTPLength > 10 {
		problems = append(problems, "FLEET_PARTNER_OTP_LENGTH must be between 6 and 10")
	}
	if c.OTPMaxAttempts < 1 || c.OTPMaxAttempts > 10 {
		problems = append(problems, "FLEET_PARTNER_OTP_MAX_ATTEMPTS must be between 1 and 10")
	}
	if c.OTPExpiry < time.Minute || c.OTPExpiry > time.Hour {
		problems = append(problems, "FLEET_PARTNER_OTP_EXPIRY must be between 1m and 1h")
	}
	if c.AccessTokenTTL <= 0 || c.AccessTokenTTL > time.Hour {
		problems = append(problems, "FLEET_PARTNER_ACCESS_TOKEN_TTL must be positive and at most 1h")
	}
	if c.RefreshTokenTTL <= c.AccessTokenTTL {
		problems = append(problems, "FLEET_PARTNER_REFRESH_TOKEN_TTL must exceed FLEET_PARTNER_ACCESS_TOKEN_TTL")
	}
	if c.MetricsEnabled && c.MetricsAuthToken == "" {
		problems = append(problems, "FLEET_PARTNER_METRICS_AUTH_TOKEN is required when metrics are enabled")
	}
	if c.AppEnv == "production" && c.ServiceSecrets == "" {
		problems = append(problems, "FLEET_PARTNER_SERVICE_SECRETS is required in production")
	}
	if err := validateAddr(c.HTTPAddr); err != nil {
		problems = append(problems, "HTTP_ADDR is invalid: "+err.Error())
	}
	if c.RedisRequired && c.Redis.Addr == "" {
		problems = append(problems, "FLEET_PARTNER_REDIS_ADDR is required when FLEET_PARTNER_REDIS_REQUIRED=true")
	}
	if c.JobLeaseDuration <= c.JobHeartbeatInterval {
		problems = append(problems, "FLEET_PARTNER_JOB_LEASE_DURATION must be greater than FLEET_PARTNER_JOB_HEARTBEAT_INTERVAL")
	}
	for _, d := range []struct {
		name  string
		value time.Duration
	}{
		{"FLEET_PARTNER_HTTP_CLIENT_TIMEOUT", c.HTTPClientTimeout},
		{"FLEET_PARTNER_SHUTDOWN_TIMEOUT", c.ShutdownTimeout},
		{"FLEET_PARTNER_OUTBOX_POLL_INTERVAL", c.OutboxPollInterval},
		{"FLEET_PARTNER_JOB_POLL_INTERVAL", c.JobPollInterval},
		{"FLEET_PARTNER_JOB_LEASE_DURATION", c.JobLeaseDuration},
		{"FLEET_PARTNER_JOB_HEARTBEAT_INTERVAL", c.JobHeartbeatInterval},
	} {
		if d.value <= 0 {
			problems = append(problems, d.name+" must be a positive duration")
		}
	}
	if c.DatabaseMaxOpenConnections < 1 {
		problems = append(problems, "FLEET_PARTNER_DATABASE_MAX_OPEN_CONNECTIONS must be at least 1")
	}
	if c.MaxRequestBodyBytes < 1024 {
		problems = append(problems, "FLEET_PARTNER_MAX_REQUEST_BODY_BYTES must be at least 1024")
	}
	for name, client := range c.Clients {
		if client.BaseURL == "" {
			continue
		}
		parsed, err := url.Parse(client.BaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			problems = append(problems, fmt.Sprintf("base URL for %s is invalid", name))
		}
	}

	if len(problems) > 0 {
		return errors.New("configuration invalid: " + strings.Join(problems, "; "))
	}
	return nil
}

func validateAddr(addr string) error {
	host, port, ok := strings.Cut(addr, ":")
	if !ok {
		return fmt.Errorf("expected host:port or :port")
	}
	_ = host
	parsed, err := strconv.Atoi(port)
	if err != nil || parsed < 1 || parsed > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func secret(name string) []byte {
	value := os.Getenv(name)
	if value == "" {
		return nil
	}
	return []byte(value)
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
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

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
