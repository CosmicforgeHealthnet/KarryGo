package config

import (
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/redisx"
)

func validConfig() Config {
	return Config{
		AppEnv:                     "development",
		HTTPAddr:                   ":8113",
		LogLevel:                   "info",
		DatabaseURL:                "postgres://user:pass@localhost:5499/fleet_partner_service",
		DatabaseMaxOpenConnections: 10,
		DatabaseMaxIdleConnections: 2,
		DatabaseConnectionMaxLife:  time.Hour,
		Redis:                      redisx.Config{},
		AccessTokenSecret:          []byte("test-secret"),
		MetricsEnabled:             true,
		MetricsAuthToken:           "metrics-token",
		HTTPClientTimeout:          10 * time.Second,
		ShutdownTimeout:            15 * time.Second,
		OutboxPollInterval:         2 * time.Second,
		JobPollInterval:            2 * time.Second,
		JobLeaseDuration:           60 * time.Second,
		JobHeartbeatInterval:       15 * time.Second,
		MaxRequestBodyBytes:        1 << 20,
		Clients:                    map[string]ClientConfig{},

		// Section 8–10 authentication settings. These secrets are mandatory
		// in every environment — there is deliberately no default.
		OTPSecret:             []byte("otp-pepper"),
		SessionSecret:         []byte("session-secret"),
		ContactSecret:         []byte("contact-secret"),
		OTPLength:             6,
		OTPExpiry:             10 * time.Minute,
		OTPMaxAttempts:        5,
		OTPResendCooldown:     60 * time.Second,
		OTPMaxPerHour:         10,
		AccessTokenTTL:        15 * time.Minute,
		RefreshTokenTTL:       720 * time.Hour,
		StepUpTTL:             5 * time.Minute,
		StaffInvitationTTL:    168 * time.Hour,
		ProviderInvitationTTL: 336 * time.Hour,
		DefaultCountryCode:    "234",
		AuthorizationCacheTTL: 30 * time.Second,
	}
}

func TestAuthenticationSecretsAreMandatory(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"FLEET_PARTNER_OTP_HMAC_SECRET":     func(c *Config) { c.OTPSecret = nil },
		"FLEET_PARTNER_SESSION_HASH_SECRET": func(c *Config) { c.SessionSecret = nil },
		"FLEET_PARTNER_CONTACT_HASH_SECRET": func(c *Config) { c.ContactSecret = nil },
	} {
		cfg := validConfig()
		mutate(&cfg)
		assertInvalid(t, cfg, name)
	}
}

func TestOTPBoundsAreEnforced(t *testing.T) {
	cfg := validConfig()
	cfg.OTPLength = 4
	assertInvalid(t, cfg, "FLEET_PARTNER_OTP_LENGTH")

	cfg = validConfig()
	cfg.OTPExpiry = 24 * time.Hour
	assertInvalid(t, cfg, "FLEET_PARTNER_OTP_EXPIRY")

	cfg = validConfig()
	cfg.OTPMaxAttempts = 50
	assertInvalid(t, cfg, "FLEET_PARTNER_OTP_MAX_ATTEMPTS")
}

func TestRefreshTokenMustOutliveAccessToken(t *testing.T) {
	cfg := validConfig()
	cfg.AccessTokenTTL = time.Hour
	cfg.RefreshTokenTTL = time.Minute
	assertInvalid(t, cfg, "FLEET_PARTNER_REFRESH_TOKEN_TTL")
}

func TestValidationErrorNeverLeaksAuthSecrets(t *testing.T) {
	cfg := validConfig()
	cfg.DatabaseURL = ""
	cfg.OTPSecret = []byte("super-secret-pepper")
	cfg.SessionSecret = []byte("super-secret-session")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, secret := range []string{"super-secret-pepper", "super-secret-session"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("validation error leaks an authentication secret: %v", err)
		}
	}
}

func TestValidConfigPasses(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestMissingDatabaseURLFails(t *testing.T) {
	cfg := validConfig()
	cfg.DatabaseURL = ""
	assertInvalid(t, cfg, "FLEET_PARTNER_DATABASE_URL")
}

func TestMissingAccessTokenSecretFails(t *testing.T) {
	cfg := validConfig()
	cfg.AccessTokenSecret = nil
	assertInvalid(t, cfg, "ACCESS_TOKEN_SECRET")
}

func TestMetricsRequireToken(t *testing.T) {
	cfg := validConfig()
	cfg.MetricsAuthToken = ""
	assertInvalid(t, cfg, "METRICS_AUTH_TOKEN")
}

func TestProductionRequiresServiceSecrets(t *testing.T) {
	cfg := validConfig()
	cfg.AppEnv = "production"
	cfg.ServiceSecrets = ""
	assertInvalid(t, cfg, "SERVICE_SECRETS")
}

func TestLeaseMustExceedHeartbeat(t *testing.T) {
	cfg := validConfig()
	cfg.JobLeaseDuration = 10 * time.Second
	cfg.JobHeartbeatInterval = 15 * time.Second
	assertInvalid(t, cfg, "JOB_LEASE_DURATION")
}

func TestInvalidPortFails(t *testing.T) {
	cfg := validConfig()
	cfg.HTTPAddr = ":99999"
	assertInvalid(t, cfg, "HTTP_ADDR")
}

func TestInvalidClientBaseURLFails(t *testing.T) {
	cfg := validConfig()
	cfg.Clients = map[string]ClientConfig{
		"customer-service": {BaseURL: "not a url", Secret: []byte("s")},
	}
	assertInvalid(t, cfg, "customer-service")
}

func TestRedisRequiredNeedsAddr(t *testing.T) {
	cfg := validConfig()
	cfg.RedisRequired = true
	assertInvalid(t, cfg, "REDIS_ADDR")
}

func TestValidationErrorNeverContainsSecrets(t *testing.T) {
	cfg := validConfig()
	cfg.DatabaseURL = "" // trigger a failure while secrets are set
	cfg.AccessTokenSecret = []byte("super-secret-value")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "super-secret-value") || strings.Contains(err.Error(), "pass@") {
		t.Fatalf("validation error leaks secrets: %v", err)
	}
}

func assertInvalid(t *testing.T, cfg Config, wantFragment string) {
	t.Helper()
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("config should be invalid (%s)", wantFragment)
	}
	if !strings.Contains(err.Error(), wantFragment) {
		t.Fatalf("error %q should mention %s", err, wantFragment)
	}
}
