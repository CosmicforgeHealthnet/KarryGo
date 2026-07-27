package config

import (
	"cosmicforge/logistics/shared/go/redisx"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                     string
	ServiceName                string
	HTTPAddr                   string
	DatabaseURL                string
	Redis                      redisx.Config
	InternalServiceKey         string
	AvailabilityServiceURL     string
	BroadcastInitialRadiusKM   float64
	BroadcastRadiusIncrementKM float64
	BroadcastMaxAttempts       int
	BroadcastWindow            time.Duration
	WalletServiceURL           string
	WalletServiceSecret        []byte
	WalletServiceSource        string

	// Fleet internal API. Used by the marketplace relay to deliver provider
	// decisions and to verify provider acceptance. When the secret is empty
	// the marketplace clients fall back to their fail-closed variants rather
	// than half-working: an unconfigured deployment must refuse, not guess.
	FleetServiceURL     string
	FleetServiceSecret  []byte
	FleetServiceCaller  string
	FleetServiceTimeout time.Duration

	// Organization marketplace feature flags. All default false: a write path
	// that is not explicitly enabled must not run.
	MarketplaceInternalAPIEnabled         bool
	MarketplaceReservationEnabled         bool
	MarketplaceExecutionEnabled           bool
	MarketplaceOutboxEnabled              bool
	MarketplaceProviderEligibilityEnabled bool
	MarketplaceVehicleEligibilityEnabled  bool
	ProviderOrganizationOffersEnabled     bool
	ProviderCompanyAssignmentsEnabled     bool

	// Dispatch-to-fleet provider decision relay.
	FleetRelayEnabled       bool
	FleetRelayBatchSize     int
	FleetRelayLeaseDuration time.Duration
	FleetRelayRetryLimit    int
	FleetRelayBackoffBase   time.Duration
	FleetRelayInterval      time.Duration

	AccessTokenSecret  []byte
	RefreshTokenSecret []byte
	OTPSecret          []byte
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	OTPTTL             time.Duration
	OTPRateWindow      time.Duration
	OTPMaxRequests     int
	OTPMaxAttempts     int
	OTPLockoutTTL      time.Duration
	OTPDebug           bool

	// SMTP fields for OTP email delivery.
	SMTPHost     string
	SMTPPort     int
	SMTPTLSMode  string
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
}

func Load() (Config, error) {
	accessTokenSecret, err := requiredEnv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET")
	if err != nil {
		return Config{}, err
	}
	refreshTokenSecret, err := requiredEnv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET")
	if err != nil {
		return Config{}, err
	}
	otpSecret, err := requiredEnv("DISPATCH_RIDER_OTP_SECRET")
	if err != nil {
		return Config{}, err
	}
	walletServiceSecret, err := requiredEnv("WALLET_SERVICE_SECRET")
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		ServiceName: getEnv("SERVICE_NAME", "driver-dispatch-delivery-service"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8103"),
		DatabaseURL: getEnv("DISPATCH_DELIVERY_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5435/dispatch_delivery_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("DISPATCH_DELIVERY_REDIS_ADDR", "localhost:6382"),
			Password: os.Getenv("DISPATCH_DELIVERY_REDIS_PASSWORD"),
			DB:       getEnvInt("DISPATCH_DELIVERY_REDIS_DB", 0),
		},
		InternalServiceKey:         getEnv("DISPATCH_DELIVERY_INTERNAL_SERVICE_KEY", "development-internal-service-key"),
		AvailabilityServiceURL:     getEnv("AVAILABILITY_SERVICE_URL", "http://localhost:8103"),
		BroadcastInitialRadiusKM:   getEnvFloat("BROADCAST_INITIAL_RADIUS_KM", 5),
		BroadcastRadiusIncrementKM: getEnvFloat("BROADCAST_RADIUS_INCREMENT_KM", 3),
		BroadcastMaxAttempts:       getEnvInt("BROADCAST_MAX_ATTEMPTS", 3),
		BroadcastWindow:            time.Duration(getEnvInt("BROADCAST_WINDOW_SECONDS", 30)) * time.Second,
		WalletServiceURL:           getEnv("WALLET_SERVICE_URL", "http://localhost:8105/api/v1/payment-wallet"),
		WalletServiceSecret:        []byte(walletServiceSecret),
		WalletServiceSource:        getEnv("WALLET_SERVICE_SOURCE", "dispatch-delivery-service"),
		FleetServiceURL:            getEnv("FLEET_PARTNER_SERVICE_URL", ""),
		FleetServiceSecret:         []byte(getEnv("FLEET_PARTNER_SERVICE_SECRET", "")),
		FleetServiceCaller:         getEnv("FLEET_PARTNER_SERVICE_CALLER", "driver-dispatch-delivery-service"),
		FleetServiceTimeout:        time.Duration(getEnvInt("FLEET_PARTNER_SERVICE_TIMEOUT_SECONDS", 5)) * time.Second,

		MarketplaceInternalAPIEnabled:         getEnvBool("DISPATCH_MARKETPLACE_INTERNAL_API_ENABLED", false),
		MarketplaceReservationEnabled:         getEnvBool("DISPATCH_ORGANIZATION_RESERVATION_ENABLED", false),
		MarketplaceExecutionEnabled:           getEnvBool("DISPATCH_ORGANIZATION_EXECUTION_ENABLED", false),
		MarketplaceOutboxEnabled:              getEnvBool("DISPATCH_MARKETPLACE_OUTBOX_ENABLED", false),
		MarketplaceProviderEligibilityEnabled: getEnvBool("DISPATCH_PROVIDER_ELIGIBILITY_ENABLED", false),
		MarketplaceVehicleEligibilityEnabled:  getEnvBool("DISPATCH_VEHICLE_ELIGIBILITY_ENABLED", false),
		ProviderOrganizationOffersEnabled:     getEnvBool("DISPATCH_PROVIDER_ORGANIZATION_OFFERS_ENABLED", false),
		ProviderCompanyAssignmentsEnabled:     getEnvBool("DISPATCH_PROVIDER_COMPANY_ASSIGNMENTS_ENABLED", false),

		FleetRelayEnabled:       getEnvBool("DISPATCH_FLEET_RELAY_ENABLED", false),
		FleetRelayBatchSize:     getEnvInt("DISPATCH_FLEET_RELAY_BATCH_SIZE", 20),
		FleetRelayLeaseDuration: time.Duration(getEnvInt("DISPATCH_FLEET_RELAY_LEASE_DURATION", 120)) * time.Second,
		FleetRelayRetryLimit:    getEnvInt("DISPATCH_FLEET_RELAY_RETRY_LIMIT", 10),
		FleetRelayBackoffBase:   time.Duration(getEnvInt("DISPATCH_FLEET_RELAY_BACKOFF_BASE", 5)) * time.Second,
		FleetRelayInterval:      time.Duration(getEnvInt("DISPATCH_FLEET_RELAY_INTERVAL_SECONDS", 5)) * time.Second,
		AccessTokenSecret:       []byte(accessTokenSecret),
		RefreshTokenSecret:      []byte(refreshTokenSecret),
		OTPSecret:               []byte(otpSecret),
		AccessTokenTTL:          time.Duration(getEnvInt("DISPATCH_RIDER_JWT_ACCESS_TTL_MINUTES", 15)) * time.Minute,
		RefreshTokenTTL:         time.Duration(getEnvInt("DISPATCH_RIDER_JWT_REFRESH_TTL_DAYS", 30)) * 24 * time.Hour,
		OTPTTL:                  time.Duration(getEnvInt("DISPATCH_RIDER_OTP_TTL_MINUTES", 10)) * time.Minute,
		OTPRateWindow:           time.Duration(getEnvInt("DISPATCH_RIDER_OTP_RATE_LIMIT_WINDOW_MINUTES", 10)) * time.Minute,
		OTPMaxRequests:          getEnvInt("DISPATCH_RIDER_OTP_RATE_LIMIT_MAX", 3),
		OTPMaxAttempts:          getEnvInt("DISPATCH_RIDER_OTP_MAX_ATTEMPTS", 3),
		OTPLockoutTTL:           time.Duration(getEnvInt("DISPATCH_RIDER_OTP_LOCKOUT_MINUTES", 30)) * time.Minute,
		OTPDebug:                getEnvBool("DISPATCH_RIDER_DEBUG_OTP", false),
		SMTPHost:                getEnv("SMTP_HOST", ""),
		SMTPPort:                getEnvInt("SMTP_PORT", 465),
		SMTPTLSMode:             getEnv("SMTP_TLS_MODE", ""),
		SMTPUser:                getEnv("SMTP_USER", ""),
		SMTPPassword:            os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:                getEnv("SMTP_FROM", ""),
	}, nil
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

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func requiredEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}

	return value, nil
}
