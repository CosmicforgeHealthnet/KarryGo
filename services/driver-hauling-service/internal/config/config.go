package config

import (
	"cosmicforge/logistics/shared/go/redisx"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv   string
	HTTPAddr string

	Migration bool

	DatabaseURL string
	Redis       redisx.Config

	// NotificationRedis carries the booking-notification queue. It must point at
	// the Redis that notification-service consumes, which is not necessarily
	// this service's own Redis.
	NotificationRedis redisx.Config

	// Provider token signing
	ProviderTokenSecret   []byte
	ProviderRefreshSecret []byte
	ProviderOTPSecret     []byte

	// Customer token verification (mirrors customer-service secret)
	CustomerTokenSecret []byte

	// HandoverSecret signs/verifies pickup & delivery handover codes (QR/OTP).
	HandoverSecret []byte

	// Token TTLs
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	OTPTTL          time.Duration
	OTPRateWindow   time.Duration
	OTPMaxRequests  int
	OTPMaxAttempts  int
	OTPDebug        bool

	// Internal service clients
	NotificationURL    string
	NotificationSecret []byte

	// Customer movement events. Disabled by default; enabling it requires the
	// customer-service origin and the shared HMAC secret.
	CustomerMovementEventsEnabled bool
	CustomerServiceURL            string
	CustomerServiceSecret         []byte
	PaymentURL                    string
	PaymentSecret                 []byte
	MediaURL                      string
	MediaToken                    string
	CashPaymentsEnabled           bool

	// RoutesAPIKey enables road-route lookups for the live trip map and the
	// progress bar. It is a Google Cloud key with the Routes API enabled, and
	// is separate from the app-side Maps SDK key. Unset means routes are
	// never fetched and every consumer falls back to straight-line geometry.
	RoutesAPIKey string

	// ServiceSecrets are HMAC secrets for inbound internal callers (e.g.
	// support-dispute-service resolving a provider identity).
	ServiceSecrets                string
	FleetIntegrationEnabled       bool
	FleetIntegrationKillSwitch    bool
	CommerceIntegrationEnabled    bool
	CommerceIntegrationKillSwitch bool
	FleetAllowedCallers           string
	OutboxWorkerEnabled           bool

	// MetricsAuthToken gates GET <api-base>/metrics. Empty means the route
	// refuses every request: outbox backlog and dead-letter counts describe
	// where the system is weak, and the safe default for that is closed.
	MetricsAuthToken        string
	FleetEventURL           string
	FleetEventSecret        []byte
	PrivateExecutionEnabled bool
	// HaulingFleetProviderWorkEnabled mounts the provider-facing corporate
	// surface (company assignments + organization offers). Default false:
	// Section 10 gates activation behind certification, exactly as Taxi does.
	HaulingFleetProviderWorkEnabled bool
	// HaulingFleetCompletedWorkEnabled turns on canonical Fleet
	// completed-work delivery for organization-managed jobs.
	HaulingFleetCompletedWorkEnabled bool
	// HaulingTypedFinancialVisibilityEnabled enforces Fleet's typed financial
	// visibility policy on provider-facing responses, server-side.
	HaulingTypedFinancialVisibilityEnabled bool
	// HaulingManagedSettlementEnabled isolates corporate settlement from the
	// legacy direct-to-provider payout. Default false until Section 12 exists.
	HaulingManagedSettlementEnabled bool

	FleetServiceURL                 string
	FleetServiceCaller              string
	FleetServiceSecret              []byte
	FleetServiceTimeout             time.Duration
	ProviderOrganizationModeEnabled bool

	// ── Section 9 capability matching ─────────────────────────────────────
	//
	// The pair is a two-stage rollout, not two independent switches:
	//
	//	MATCHING=false                 legacy weight-and-type matching only
	//	MATCHING=true, FAIL_CLOSED=false  SHADOW: decide and record, do not enforce
	//	MATCHING=true, FAIL_CLOSED=true   ENFORCED: hard fit selects
	//
	// Shadow exists because enabling hard fit on an unsurveyed fleet would
	// refuse nearly every candidate as CAPABILITY_UNKNOWN and be
	// indistinguishable from a total outage. Run shadow, watch
	// hauling_fleet_capability_completeness_percent, then enforce.
	CapabilityMatchingEnabled bool
	// FailClosedTruckLookup makes an unresolved truck ineligible instead of
	// falling through to proximity-only matching. It has no effect unless
	// CapabilityMatchingEnabled is also true.
	FailClosedTruckLookup bool
	// SupplyPreviewEnabled mounts the read-only planning surface Vendor's
	// just-in-time orchestration reads before it commits.
	SupplyPreviewEnabled bool
	// CertificationRequiredProducts lists products that may only be served by
	// a formally certified truck, comma-separated. A generic truck type never
	// substitutes for a certificate for a product named here.
	CertificationRequiredProducts string
	// CapabilityMaintenanceInterval and MatchDecisionRetention drive the
	// eligibility-expiry and evidence-retention sweeper.
	CapabilityMaintenanceInterval time.Duration
	MatchDecisionRetention        time.Duration

	// Matching config
	BookingMatchTimeout int     // seconds a matched provider has to accept
	BookingSearchWindow int     // seconds to keep searching (rescanning) before unmatched
	ProviderOnlineTTL   int     // seconds provider stays online without heartbeat
	MatchMaxRadiusKm    float64 // providers farther than this from pickup are not dispatched

	// CommerceQuoteTTL is how long an issued internal commerce delivery quote
	// stands before a caller must re-ask.
	CommerceQuoteTTL time.Duration
}

func Load() Config {
	return Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPAddr: getEnv("HTTP_ADDR", ":8104"),

		Migration: getEnvBool("HAULING_MIGRATION", false),

		DatabaseURL: getEnv("HAULING_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5436/hauling_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("HAULING_REDIS_ADDR", "localhost:6383"),
			Password: os.Getenv("HAULING_REDIS_PASSWORD"),
			DB:       getEnvInt("HAULING_REDIS_DB", 0),
		},

		// Booking-lifecycle notifications are published onto a Redis stream that
		// notification-service consumes, so both services must point at the SAME
		// Redis instance. That is a different instance from this service's own
		// Redis (OTP challenges, availability, match locks), so it is configured
		// separately here. Defaults to this service's Redis, which is correct for
		// a single-Redis deployment but silently strands notifications when each
		// service runs its own — set HAULING_NOTIFICATION_REDIS_ADDR to
		// notification-service's Redis in that case.
		NotificationRedis: redisx.Config{
			Addr:     getEnv("HAULING_NOTIFICATION_REDIS_ADDR", getEnv("HAULING_REDIS_ADDR", "localhost:6383")),
			Password: getEnv("HAULING_NOTIFICATION_REDIS_PASSWORD", os.Getenv("HAULING_REDIS_PASSWORD")),
			DB:       getEnvInt("HAULING_NOTIFICATION_REDIS_DB", getEnvInt("HAULING_REDIS_DB", 0)),
		},

		ProviderTokenSecret:   []byte(os.Getenv("HAULING_PROVIDER_TOKEN_SECRET")),
		ProviderRefreshSecret: []byte(os.Getenv("HAULING_PROVIDER_REFRESH_SECRET")),
		ProviderOTPSecret:     []byte(os.Getenv("HAULING_PROVIDER_OTP_SECRET")),
		CustomerTokenSecret:   []byte(os.Getenv("HAULING_CUSTOMER_TOKEN_SECRET")),
		HandoverSecret:        []byte(getEnv("HANDOVER_CODE_SECRET", "development-handover-code-secret")),

		AccessTokenTTL:  time.Duration(getEnvInt("HAULING_ACCESS_TOKEN_TTL_SECONDS", 3600)) * time.Second,
		RefreshTokenTTL: time.Duration(getEnvInt("HAULING_REFRESH_TOKEN_TTL_DAYS", 30)) * 24 * time.Hour,
		OTPTTL:          time.Duration(getEnvInt("HAULING_OTP_TTL_SECONDS", 600)) * time.Second,
		OTPRateWindow:   time.Duration(getEnvInt("HAULING_OTP_RATE_WINDOW_SECONDS", 60)) * time.Second,
		OTPMaxRequests:  getEnvInt("HAULING_OTP_MAX_REQUESTS", 5),
		OTPMaxAttempts:  getEnvInt("HAULING_OTP_MAX_ATTEMPTS", 5),
		OTPDebug:        getEnvBool("HAULING_OTP_DEBUG", false),

		NotificationURL:    os.Getenv("HAULING_NOTIFICATION_URL"),
		NotificationSecret: []byte(os.Getenv("HAULING_NOTIFICATION_SECRET")),

		CustomerMovementEventsEnabled: getEnvBool("HAULING_CUSTOMER_MOVEMENT_EVENTS_ENABLED", false),
		CustomerServiceURL:            os.Getenv("HAULING_CUSTOMER_SERVICE_URL"),
		CustomerServiceSecret:         []byte(os.Getenv("HAULING_CUSTOMER_SERVICE_SECRET")),
		PaymentURL:                    os.Getenv("HAULING_PAYMENT_URL"),
		PaymentSecret:                 []byte(os.Getenv("HAULING_PAYMENT_SECRET")),
		MediaURL:                      os.Getenv("HAULING_MEDIA_URL"),
		MediaToken:                    os.Getenv("HAULING_MEDIA_TOKEN"),
		CashPaymentsEnabled:           getEnvBool("HAULING_CASH_PAYMENTS_ENABLED", false),
		RoutesAPIKey:                  os.Getenv("HAULING_ROUTES_API_KEY"),

		ServiceSecrets:                         os.Getenv("HAULING_SERVICE_SECRETS"),
		FleetIntegrationEnabled:                getEnvBool("HAULING_FLEET_INTEGRATION_ENABLED", false),
		FleetIntegrationKillSwitch:             getEnvBool("HAULING_FLEET_INTEGRATION_KILL_SWITCH", false),
		CommerceIntegrationEnabled:             getEnvBool("HAULING_COMMERCE_INTEGRATION_ENABLED", false),
		CommerceIntegrationKillSwitch:          getEnvBool("HAULING_COMMERCE_INTEGRATION_KILL_SWITCH", false),
		FleetAllowedCallers:                    getEnv("HAULING_FLEET_ALLOWED_CALLERS", "fleet-partner-service"),
		OutboxWorkerEnabled:                    getEnvBool("HAULING_OUTBOX_WORKER_ENABLED", false),
		MetricsAuthToken:                       getEnv("HAULING_METRICS_AUTH_TOKEN", ""),
		FleetEventURL:                          os.Getenv("HAULING_FLEET_EVENT_URL"),
		FleetEventSecret:                       []byte(os.Getenv("HAULING_FLEET_EVENT_SECRET")),
		PrivateExecutionEnabled:                getEnvBool("HAULING_PRIVATE_EXECUTION_ENABLED", false),
		HaulingFleetProviderWorkEnabled:        getEnvBool("HAULING_FLEET_PROVIDER_WORK_ENABLED", false),
		HaulingFleetCompletedWorkEnabled:       getEnvBool("HAULING_FLEET_COMPLETED_WORK_ENABLED", false),
		HaulingTypedFinancialVisibilityEnabled: getEnvBool("HAULING_TYPED_FINANCIAL_VISIBILITY_ENABLED", true),
		HaulingManagedSettlementEnabled:        getEnvBool("HAULING_MANAGED_SETTLEMENT_ENABLED", false),

		FleetServiceURL:                 os.Getenv("FLEET_PARTNER_SERVICE_URL"),
		FleetServiceCaller:              getEnv("FLEET_PARTNER_SERVICE_CALLER", "driver-hauling-service"),
		FleetServiceSecret:              []byte(os.Getenv("FLEET_PARTNER_SERVICE_SECRET")),
		FleetServiceTimeout:             time.Duration(getEnvInt("FLEET_PARTNER_SERVICE_TIMEOUT_SECONDS", 5)) * time.Second,
		ProviderOrganizationModeEnabled: getEnvBool("PROVIDER_ORGANIZATION_MODE_ENABLED", false),

		CapabilityMatchingEnabled:     getEnvBool("HAULING_CAPABILITY_MATCHING_ENABLED", false),
		FailClosedTruckLookup:         getEnvBool("HAULING_FAIL_CLOSED_TRUCK_LOOKUP", false),
		SupplyPreviewEnabled:          getEnvBool("HAULING_SUPPLY_PREVIEW_ENABLED", false),
		CertificationRequiredProducts: os.Getenv("HAULING_CERTIFICATION_REQUIRED_PRODUCTS"),
		CapabilityMaintenanceInterval: time.Duration(getEnvInt("HAULING_CAPABILITY_MAINTENANCE_INTERVAL_SECONDS", 900)) * time.Second,
		MatchDecisionRetention:        time.Duration(getEnvInt("HAULING_MATCH_DECISION_RETENTION_DAYS", 30)) * 24 * time.Hour,

		BookingMatchTimeout: getEnvInt("HAULING_BOOKING_MATCH_TIMEOUT", 30),
		BookingSearchWindow: getEnvInt("HAULING_BOOKING_SEARCH_WINDOW", 60),
		ProviderOnlineTTL:   getEnvInt("HAULING_PROVIDER_ONLINE_TTL", 90),
		MatchMaxRadiusKm:    getEnvFloat("HAULING_MATCH_MAX_RADIUS_KM", 25),
		CommerceQuoteTTL:    time.Duration(getEnvInt("HAULING_COMMERCE_QUOTE_TTL_SECONDS", 1200)) * time.Second,
	}
}

// Validate rejects unsafe partial enablement. Disabled integrations require
// no downstream credentials, while enabled ones require explicit secrets and
// identities; there are no development secret fallbacks.
func (c Config) Validate() error {
	var problems []string
	for name, secret := range map[string][]byte{
		"HAULING_PROVIDER_TOKEN_SECRET":   c.ProviderTokenSecret,
		"HAULING_PROVIDER_REFRESH_SECRET": c.ProviderRefreshSecret,
		"HAULING_PROVIDER_OTP_SECRET":     c.ProviderOTPSecret,
		"HAULING_CUSTOMER_TOKEN_SECRET":   c.CustomerTokenSecret,
	} {
		if len(secret) == 0 {
			problems = append(problems, name+" is required")
		}
	}
	if c.FleetIntegrationEnabled && !c.FleetIntegrationKillSwitch {
		if len(serviceSecrets(c.ServiceSecrets)) == 0 {
			problems = append(problems, "HAULING_SERVICE_SECRETS is required when Fleet integration is enabled")
		}
		if strings.TrimSpace(c.FleetAllowedCallers) == "" {
			problems = append(problems, "HAULING_FLEET_ALLOWED_CALLERS is required when Fleet integration is enabled")
		}
		if !c.OutboxWorkerEnabled {
			problems = append(problems, "HAULING_OUTBOX_WORKER_ENABLED must be true when Fleet integration is enabled")
		}
		if strings.TrimSpace(c.FleetEventURL) == "" {
			problems = append(problems, "HAULING_FLEET_EVENT_URL is required when Fleet integration is enabled")
		}
		if len(c.FleetEventSecret) == 0 {
			problems = append(problems, "HAULING_FLEET_EVENT_SECRET is required when Fleet integration is enabled")
		}
		if strings.TrimSpace(c.PaymentURL) == "" || len(c.PaymentSecret) == 0 {
			problems = append(problems, "HAULING_PAYMENT_URL and HAULING_PAYMENT_SECRET are required when Fleet integration is enabled")
		}
		if strings.TrimSpace(c.NotificationURL) == "" || len(c.NotificationSecret) == 0 {
			problems = append(problems, "HAULING_NOTIFICATION_URL and HAULING_NOTIFICATION_SECRET are required when Fleet integration is enabled")
		}
	}
	if c.PrivateExecutionEnabled && !c.FleetIntegrationKillSwitch {
		if !c.FleetIntegrationEnabled {
			problems = append(problems, "HAULING_FLEET_INTEGRATION_ENABLED must be true when private execution is enabled")
		}
		if strings.TrimSpace(c.FleetServiceURL) == "" || strings.TrimSpace(c.FleetServiceCaller) == "" || len(c.FleetServiceSecret) == 0 {
			problems = append(problems, "FLEET_PARTNER_SERVICE_URL, FLEET_PARTNER_SERVICE_CALLER and FLEET_PARTNER_SERVICE_SECRET are required when private execution is enabled")
		}
	}
	if c.CommerceIntegrationEnabled && !c.CommerceIntegrationKillSwitch {
		if strings.TrimSpace(c.MediaURL) == "" || strings.TrimSpace(c.MediaToken) == "" {
			problems = append(problems, "HAULING_MEDIA_URL and HAULING_MEDIA_TOKEN are required when Commerce integration is enabled")
		}
	}
	// Fail-closed without capability matching would be a flag that silently
	// does nothing, which is worse than a refusal: an operator would believe
	// unresolved trucks were being rejected while they were still matching.
	if c.FailClosedTruckLookup && !c.CapabilityMatchingEnabled {
		problems = append(problems, "HAULING_FAIL_CLOSED_TRUCK_LOOKUP requires HAULING_CAPABILITY_MATCHING_ENABLED")
	}
	// A preview answers from the capability store. Without it the endpoint
	// could only ever return UNKNOWN, which would teach Vendor's planner to
	// stop asking.
	if c.SupplyPreviewEnabled && !c.CapabilityMatchingEnabled {
		problems = append(problems, "HAULING_SUPPLY_PREVIEW_ENABLED requires HAULING_CAPABILITY_MATCHING_ENABLED")
	}
	if c.ProviderOrganizationModeEnabled {
		if strings.TrimSpace(c.FleetServiceURL) == "" || strings.TrimSpace(c.FleetServiceCaller) == "" || len(c.FleetServiceSecret) == 0 {
			problems = append(problems, "FLEET_PARTNER_SERVICE_URL, FLEET_PARTNER_SERVICE_CALLER and FLEET_PARTNER_SERVICE_SECRET are required when PROVIDER_ORGANIZATION_MODE_ENABLED is true")
		}
	}
	// Public booking routes are mounted in production as well as the Fleet
	// authority. Their enabled payment and notification paths must therefore
	// never inherit local no-op behavior from an incomplete deployment.
	if strings.EqualFold(strings.TrimSpace(c.AppEnv), "production") && !c.FleetIntegrationKillSwitch {
		if strings.TrimSpace(c.PaymentURL) == "" || len(c.PaymentSecret) == 0 {
			problems = append(problems, "HAULING_PAYMENT_URL and HAULING_PAYMENT_SECRET are required in production")
		}
		if strings.TrimSpace(c.NotificationURL) == "" || len(c.NotificationSecret) == 0 {
			problems = append(problems, "HAULING_NOTIFICATION_URL and HAULING_NOTIFICATION_SECRET are required in production")
		}
	}
	if len(problems) > 0 {
		return errors.New("configuration invalid: " + strings.Join(problems, "; "))
	}
	return nil
}

func serviceSecrets(value string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		name, secret, ok := strings.Cut(part, "=")
		if ok && strings.TrimSpace(name) != "" && strings.TrimSpace(secret) != "" {
			out[strings.TrimSpace(name)] = true
		}
	}
	return out
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func getEnv(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
