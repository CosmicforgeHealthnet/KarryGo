package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/browserauth"
	"cosmicforge/logistics/shared/go/redisx"
)

type Config struct {
	AppEnv        string
	HTTPAddr      string
	DatabaseURL   string
	Redis         redisx.Config
	Wallet        WalletConfig
	WebhookSecret string // WEBHOOK_SIGNING_SECRET: key material for encrypting webhook secrets at rest

	// Webhook delivery worker settings.
	WebhookMaxRetries      int           // WEBHOOK_MAX_RETRIES (default 5)
	WebhookDeliveryTimeout time.Duration // WEBHOOK_DELIVERY_TIMEOUT_SECONDS (default 30s)

	// PartnerAuth holds partner dashboard session settings (separate from any
	// dispatch/customer/provider JWT secret).
	PartnerAuth PartnerAuthConfig
	// PartnerOTP holds web signup OTP settings.
	PartnerOTP PartnerOTPConfig
	// PartnerSMTP holds cPanel SMTP settings for partner OTP email.
	PartnerSMTP PartnerSMTPConfig
	// Pricing holds the fare engine parameters.
	Pricing PricingConfig
	// Billing holds partner Billing & Usage and charge worker parameters.
	Billing BillingConfig
	// Edge holds the Section 18 pass-through routing onto canonical services.
	Edge EdgeConfig
}

// EdgeConfig configures the Section 18 edge. Off by default: until it is
// enabled the gateway serves only what it always has.
type EdgeConfig struct {
	Enabled bool
	// Origins maps upstream name to bare origin, from GATEWAY_UPSTREAM_<NAME>_URL.
	Origins          map[string]string
	Timeout          time.Duration
	BreakerThreshold int
	BreakerCooldown  time.Duration
}

// edgeUpstreams are the upstream names the edge can route to.
var edgeUpstreams = []string{"customer", "taxi", "dispatch", "hauling", "payment_wallet", "notification",
	"support", "vendor", "fleet", "verification"}

func loadEdgeConfig() EdgeConfig {
	origins := map[string]string{}
	for _, name := range edgeUpstreams {
		if origin := strings.TrimSpace(os.Getenv("GATEWAY_UPSTREAM_" + strings.ToUpper(name) + "_URL")); origin != "" {
			origins[name] = origin
		}
	}
	return EdgeConfig{
		Enabled:          strings.EqualFold(os.Getenv("GATEWAY_EDGE_ROUTING_ENABLED"), "true"),
		Origins:          origins,
		Timeout:          time.Duration(getEnvInt("GATEWAY_UPSTREAM_TIMEOUT_SECONDS", 15)) * time.Second,
		BreakerThreshold: getEnvInt("GATEWAY_BREAKER_THRESHOLD", 5),
		BreakerCooldown:  time.Duration(getEnvInt("GATEWAY_BREAKER_COOLDOWN_SECONDS", 30)) * time.Second,
	}
}

// WalletConfig holds the payment-wallet client settings. The URL is a bare
// origin (e.g. http://localhost:8105); the shared walletclient appends the path.
type WalletConfig struct {
	URL         string
	ServiceName string
	Secret      string
}

// PartnerAuthConfig holds partner dashboard auth + internal service settings.
type PartnerAuthConfig struct {
	BrowserPolicy      browserauth.Policy
	RefreshTTL         time.Duration
	JWTSecret          string        // PARTNER_JWT_SECRET (dashboard sessions only)
	JWTTTL             time.Duration // PARTNER_ACCESS_TTL_MINUTES, capped at 15 minutes
	InternalServiceKey string        // PARTNER_INTERNAL_SERVICE_KEY (internal routes only)
}

// PartnerOTPConfig holds partner web app OTP signup settings.
type PartnerOTPConfig struct {
	TTL            time.Duration
	MaxAttempts    int
	ResendCooldown time.Duration
}

// PartnerSMTPConfig holds direct SMTP settings for partner OTP email.
type PartnerSMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	TLSMode  string
}

// PricingConfig holds the fare engine parameters (all kobo except the multiplier).
type PricingConfig struct {
	BaseKobo          int64
	StandardPerKmKobo int64
	ExpressPerKmKobo  int64
	LargeMultiplier   float64
}

// BillingConfig holds monthly invoice generation and invoice charge settings.
// RatePerDeliveryKobo is a placeholder default and must be set to the real
// agreed price via BILLING_RATE_PER_DELIVERY_KOBO before real invoicing.
// The charge worker only starts when ChargeInterval > 0.
type BillingConfig struct {
	RatePerDeliveryKobo int64
	ChargeInterval      time.Duration
}

func Load() Config {
	// Disabled Partner deployments must not change unrelated Gateway startup.
	var policy browserauth.Policy
	if os.Getenv("API_GATEWAY_DATABASE_URL") != "" || os.Getenv("PARTNER_AUTH_WEB_ORIGINS") != "" || getEnv("APP_ENV", "development") == "development" {
		var err error
		policy, err = browserauth.Load("PARTNER_AUTH", getEnv("APP_ENV", "development"), "karrygo_partner_refresh", "/api/v1/partner/v1/auth")
		if err != nil {
			panic(err)
		}
		if getEnv("APP_ENV", "development") != "development" && getEnv("APP_ENV", "development") != "test" && len(os.Getenv("PARTNER_JWT_SECRET")) < 32 {
			panic("PARTNER_JWT_SECRET must contain at least 32 bytes")
		}
	}
	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("API_GATEWAY_DATABASE_URL"),
		Redis: redisx.Config{
			Addr:     getEnv("API_GATEWAY_REDIS_ADDR", "localhost:6379"),
			Password: os.Getenv("API_GATEWAY_REDIS_PASSWORD"),
			DB:       getEnvInt("API_GATEWAY_REDIS_DB", 0),
		},
		Wallet: WalletConfig{
			URL:         getEnv("API_GATEWAY_WALLET_SERVICE_URL", "http://localhost:8105"),
			ServiceName: getEnv("API_GATEWAY_WALLET_SERVICE_NAME", "api-gateway"),
			Secret:      os.Getenv("API_GATEWAY_WALLET_SERVICE_SECRET"),
		},
		WebhookSecret:          os.Getenv("WEBHOOK_SIGNING_SECRET"),
		WebhookMaxRetries:      getEnvInt("WEBHOOK_MAX_RETRIES", 5),
		WebhookDeliveryTimeout: time.Duration(getEnvInt("WEBHOOK_DELIVERY_TIMEOUT_SECONDS", 30)) * time.Second,
		PartnerAuth: PartnerAuthConfig{
			BrowserPolicy:      policy,
			RefreshTTL:         time.Duration(getEnvInt("PARTNER_REFRESH_TTL_HOURS", 720)) * time.Hour,
			JWTSecret:          os.Getenv("PARTNER_JWT_SECRET"),
			JWTTTL:             time.Duration(getEnvInt("PARTNER_ACCESS_TTL_MINUTES", 15)) * time.Minute,
			InternalServiceKey: os.Getenv("PARTNER_INTERNAL_SERVICE_KEY"),
		},
		PartnerOTP: PartnerOTPConfig{
			TTL:            time.Duration(getEnvInt("PARTNER_OTP_TTL_MINUTES", 10)) * time.Minute,
			MaxAttempts:    getEnvInt("PARTNER_OTP_MAX_ATTEMPTS", 5),
			ResendCooldown: time.Duration(getEnvInt("PARTNER_OTP_RESEND_COOLDOWN_SECONDS", 60)) * time.Second,
		},
		PartnerSMTP: loadPartnerSMTPConfig(),
		Pricing: PricingConfig{
			BaseKobo:          getEnvInt64("PRICING_BASE_KOBO", 50000),
			StandardPerKmKobo: getEnvInt64("PRICING_STANDARD_PER_KM_KOBO", 15000),
			ExpressPerKmKobo:  getEnvInt64("PRICING_EXPRESS_PER_KM_KOBO", 25000),
			LargeMultiplier:   getEnvFloat("PRICING_LARGE_MULTIPLIER", 1.5),
		},
		Billing: BillingConfig{
			RatePerDeliveryKobo: getEnvInt64("BILLING_RATE_PER_DELIVERY_KOBO", 10000),
			ChargeInterval:      time.Duration(getEnvInt("BILLING_CHARGE_INTERVAL_SECONDS", 0)) * time.Second,
		},
		Edge: loadEdgeConfig(),
	}
}

func loadPartnerSMTPConfig() PartnerSMTPConfig {
	username := os.Getenv("PARTNER_SMTP_USERNAME")
	port := getEnvInt("PARTNER_SMTP_PORT", 465)
	tlsMode := strings.ToLower(strings.TrimSpace(os.Getenv("PARTNER_SMTP_TLS_MODE")))
	if tlsMode == "" {
		if port == 587 {
			tlsMode = "starttls"
		} else {
			tlsMode = "tls"
		}
	}

	return PartnerSMTPConfig{
		Host:     os.Getenv("PARTNER_SMTP_HOST"),
		Port:     port,
		Username: username,
		Password: os.Getenv("PARTNER_SMTP_PASSWORD"),
		From:     getEnv("PARTNER_SMTP_FROM", username),
		FromName: getEnv("PARTNER_SMTP_FROM_NAME", "KarryGo"),
		TLSMode:  tlsMode,
	}
}

func getEnvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
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
