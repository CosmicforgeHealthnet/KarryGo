package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/redisx"
)

type Config struct {
	ShareJourneyEnabled                                                         bool
	ShareJourneyTTL                                                             time.Duration
	ShareCompletedRetention                                                     time.Duration
	ShareViewerOrigin                                                           string
	ShareTrustedGatewayCIDRs                                                    []string
	ShareTaxiURL, ShareDispatchURL, ShareHaulingURL, ShareVendorURL             string
	ShareTaxiSecret, ShareDispatchSecret, ShareHaulingSecret, ShareVendorSecret []byte
	AppEnv                                                                      string
	HTTPAddr                                                                    string
	Migration                                                                   bool

	DatabaseURL string
	Redis       redisx.Config

	AccessTokenSecret  []byte
	RefreshTokenSecret []byte
	OTPSecret          []byte

	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	OTPTTL          time.Duration
	OTPRateWindow   time.Duration

	OTPMaxRequests int
	OTPMaxAttempts int
	OTPDebug       bool

	NotificationBaseURL string
	NotificationSecret  []byte

	MediaBaseURL       string
	MediaPublicBaseURL string
	MediaServiceToken  string
	MediaPrivateSecret []byte
	// MediaUploadSessionEnabled is the Customer V4 §24 gate for the N12
	// customer upload capability. Off until secure-upload certification.
	MediaUploadSessionEnabled bool
	SupportBaseURL            string

	// ServiceSecrets are the HMAC secrets for internal service-to-service callers
	// (e.g. support-dispute-service resolving a customer identity).
	ServiceSecrets string

	// Section 6 capability flags. All default to false so each capability is
	// enabled deliberately per environment.
	MobilityPreferencesEnabled    bool
	GuestTravellersEnabled        bool
	TrustedContactsEnabled        bool
	UnifiedMovementHistoryEnabled bool

	// ContextCallerScopes allowlists which HMAC-authenticated service may read
	// which customer context scopes: "svc=scope|scope;svc2=scope". A caller
	// must also have an HMAC secret in ServiceSecrets.
	ContextCallerScopes string

	// MovementEventsConsumerEnabled gates the movement event ingress
	// independently of the customer read endpoints. MovementEventCallers binds
	// each publishing service to the one authority it owns.
	MovementEventsConsumerEnabled bool
	MovementEventCallers          string
	// MovementGroupingCallers lists services allowed to publish commerce
	// grouping. They are never movement authorities.
	MovementGroupingCallers string
}

func Load() Config {
	return Config{
		ShareJourneyEnabled:      getEnvBool("CUSTOMER_SHARE_JOURNEY_ENABLED", false),
		ShareJourneyTTL:          time.Duration(getEnvInt("CUSTOMER_SHARE_JOURNEY_TTL_SECONDS", 86400)) * time.Second,
		ShareCompletedRetention:  time.Duration(getEnvInt("CUSTOMER_SHARE_COMPLETED_RETENTION_SECONDS", 900)) * time.Second,
		ShareViewerOrigin:        getEnv("CUSTOMER_SHARE_VIEWER_ORIGIN", ""),
		ShareTrustedGatewayCIDRs: strings.FieldsFunc(getEnv("CUSTOMER_SHARE_TRUSTED_GATEWAY_CIDRS", ""), func(r rune) bool { return r == ',' || r == ' ' }),
		ShareTaxiURL:             getEnv("CUSTOMER_SHARE_TAXI_URL", ""), ShareDispatchURL: getEnv("CUSTOMER_SHARE_DISPATCH_URL", ""), ShareHaulingURL: getEnv("CUSTOMER_SHARE_HAULING_URL", ""), ShareVendorURL: getEnv("CUSTOMER_SHARE_VENDOR_URL", ""),
		ShareTaxiSecret: []byte(getEnv("CUSTOMER_SHARE_TAXI_SECRET", "")), ShareDispatchSecret: []byte(getEnv("CUSTOMER_SHARE_DISPATCH_SECRET", "")), ShareHaulingSecret: []byte(getEnv("CUSTOMER_SHARE_HAULING_SECRET", "")), ShareVendorSecret: []byte(getEnv("CUSTOMER_SHARE_VENDOR_SECRET", "")),
		AppEnv:    getEnv("APP_ENV", "development"),
		HTTPAddr:  getEnv("HTTP_ADDR", ":8101"),
		Migration: getEnvBool("MIGRATION", false),

		DatabaseURL: getEnv("CUSTOMER_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5433/customer_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("CUSTOMER_REDIS_ADDR", "localhost:6380"),
			Password: os.Getenv("CUSTOMER_REDIS_PASSWORD"),
			DB:       getEnvInt("CUSTOMER_REDIS_DB", 0),
		},

		AccessTokenSecret:  []byte(getEnv("CUSTOMER_ACCESS_TOKEN_SECRET", "development-customer-access-token-secret")),
		RefreshTokenSecret: []byte(getEnv("CUSTOMER_REFRESH_TOKEN_SECRET", "development-customer-refresh-token-secret")),
		OTPSecret:          []byte(getEnv("CUSTOMER_OTP_SECRET", "development-customer-otp-secret")),

		AccessTokenTTL:  time.Duration(getEnvInt("CUSTOMER_ACCESS_TOKEN_TTL_SECONDS", 900)) * time.Second,
		RefreshTokenTTL: time.Duration(getEnvInt("CUSTOMER_REFRESH_TOKEN_TTL_HOURS", 720)) * time.Hour,
		OTPTTL:          time.Duration(getEnvInt("CUSTOMER_OTP_TTL_SECONDS", 300)) * time.Second,
		OTPRateWindow:   time.Duration(getEnvInt("CUSTOMER_OTP_RATE_LIMIT_WINDOW_SECONDS", 600)) * time.Second,

		OTPMaxRequests: getEnvInt("CUSTOMER_OTP_MAX_REQUESTS", 5),
		OTPMaxAttempts: getEnvInt("CUSTOMER_OTP_MAX_ATTEMPTS", 5),
		OTPDebug:       getEnvBool("CUSTOMER_DEBUG_OTP", false),

		NotificationBaseURL: getEnv("CUSTOMER_NOTIFICATION_BASE_URL", ""),
		NotificationSecret:  []byte(getEnv("CUSTOMER_NOTIFICATION_SECRET", "")),

		MediaBaseURL:              getEnv("CUSTOMER_MEDIA_BASE_URL", ""),
		MediaPublicBaseURL:        getEnv("CUSTOMER_MEDIA_PUBLIC_BASE_URL", ""),
		MediaServiceToken:         getEnv("CUSTOMER_MEDIA_SERVICE_TOKEN", ""),
		MediaPrivateSecret:        []byte(getEnv("CUSTOMER_MEDIA_PRIVATE_SECRET", "")),
		MediaUploadSessionEnabled: getEnvBool("CUSTOMER_MEDIA_UPLOAD_SESSION_ENABLED", false),
		SupportBaseURL:            getEnv("CUSTOMER_SUPPORT_BASE_URL", ""),

		ServiceSecrets: getEnv("CUSTOMER_SERVICE_SECRETS", "support-dispute-service=development-support-dispute-service-secret"),

		MobilityPreferencesEnabled:    getEnvBool("CUSTOMER_MOBILITY_PREFERENCES_ENABLED", false),
		GuestTravellersEnabled:        getEnvBool("CUSTOMER_GUEST_TRAVELLERS_ENABLED", false),
		TrustedContactsEnabled:        getEnvBool("CUSTOMER_TRUSTED_CONTACTS_ENABLED", false),
		UnifiedMovementHistoryEnabled: getEnvBool("CUSTOMER_UNIFIED_MOVEMENT_HISTORY_ENABLED", false),

		MovementEventsConsumerEnabled: getEnvBool("CUSTOMER_MOVEMENT_EVENTS_CONSUMER_ENABLED", false),
		MovementEventCallers:          getEnv("CUSTOMER_MOVEMENT_EVENT_CALLERS", "driver-taxi-service=TAXI;driver-dispatch-delivery-service=DISPATCH;driver-hauling-service=HAULING;vendor-service=VENDOR_COMMERCE"),
		MovementGroupingCallers:       getEnv("CUSTOMER_MOVEMENT_GROUPING_CALLERS", "vendor-service"),

		ContextCallerScopes: getEnv("CUSTOMER_CONTEXT_CALLER_SCOPES", "driver-taxi-service=mobility_preferences|saved_location|guest_traveller;vendor-service=saved_location"),
	}
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
