package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/redisx"
)

// defaultProvisionalRatesJSON is a LOCAL-DEV-ONLY fallback so a fresh
// checkout can price a ride without any extra setup. Every amount is kobo
// integers, per CLAUDE.md's money convention. This is not a commercial rate
// decision -- see internal/pricing's package doc comment -- and any real
// deployment must set TAXI_PROVISIONAL_RATES explicitly rather than inherit
// this value.
const defaultProvisionalRatesJSON = `{
	"economy": {"base_kobo": 50000, "per_km_kobo": 10000, "per_min_kobo": 2000, "min_kobo": 70000},
	"comfort": {"base_kobo": 70000, "per_km_kobo": 13000, "per_min_kobo": 2500, "min_kobo": 90000},
	"xl":      {"base_kobo": 90000, "per_km_kobo": 16000, "per_min_kobo": 3000, "min_kobo": 120000}
}`

type Config struct {
	CustomerLiveLocationEnabled bool
	CustomerLocationFreshFor    time.Duration
	AppEnv                      string
	HTTPAddr                    string

	Migration bool

	DatabaseURL string
	Redis       redisx.Config

	// NotificationRedis carries the ride-notification queue. It must point at
	// the Redis that notification-service consumes, which is not necessarily
	// this service's own Redis.
	NotificationRedis redisx.Config

	// Provider token signing.
	//
	// ProviderTokenSecret must equal the `taxi=` entry in payment-wallet's
	// PAYMENT_WALLET_PROVIDER_ACCESS_TOKEN_SECRETS and support-dispute's
	// SUPPORT_DISPUTE_TAXI_PROVIDER_TOKEN_SECRET. One driver token is presented
	// to all three services; changing this value here alone breaks driver
	// earnings, withdrawals and complaints.
	ProviderTokenSecret   []byte
	ProviderRefreshSecret []byte
	ProviderOTPSecret     []byte

	// CustomerTokenSecret verifies passenger bearer tokens and must equal
	// CUSTOMER_ACCESS_TOKEN_SECRET in customer-service.
	CustomerTokenSecret []byte

	// HandoverSecret signs/verifies the pickup ride PIN. Service-local: no
	// other service reads these codes, so it needs no cross-service match.
	HandoverSecret []byte

	// Taxi Goods is a distinct Vendor Commerce product. It is disabled by
	// default and uses its own pricing and handover namespaces so passenger
	// fare/PIN configuration cannot accidentally activate it.
	TaxiGoodsDeliveryEnabled bool
	TaxiGoodsPricingJSON     string
	TaxiGoodsHandoverSecret  []byte
	TaxiGoodsQuoteTTL        time.Duration
	MediaURL                 string
	MediaToken               string
	VendorURL                string
	VendorSecret             []byte

	// Token TTLs
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	OTPTTL          time.Duration
	OTPRateWindow   time.Duration
	OTPMaxRequests  int
	OTPMaxAttempts  int
	OTPDebug        bool

	// Internal service clients.
	//
	// NotificationURL and PaymentURL must be BARE ORIGINS ("http://host:port").
	// shared/go/walletclient and the notification client append their own
	// "/api/v1/..." paths; a prefixed URL 404s every internal call silently.
	NotificationURL    string
	NotificationSecret []byte
	PaymentURL         string
	PaymentSecret      []byte

	// Referral feed: announces new drivers (and their completed onboarding)
	// to referral-service. Off by default; when on, the URL (bare origin),
	// secret and pepper are all required. The pepper must be identical in
	// every service that feeds referral-service.
	ReferralFeedEnabled    bool
	ReferralServiceURL     string
	ReferralServiceSecret  []byte
	ReferralIdentityPepper []byte

	// RoutesAPIKey enables road-route lookups for the live trip map and the
	// progress bar. It is a Google Cloud key with the Routes API enabled, and
	// is separate from the app-side Maps SDK key. Unset means routes are never
	// fetched and every consumer falls back to straight-line geometry.
	RoutesAPIKey string

	// PricingRatesJSON/PricingRateVersion feed the PROVISIONAL config-driven
	// PricingClient (internal/pricing) -- see that package's own doc comment.
	// No number here is a real commercial rate; the defaults exist only so a
	// fresh checkout can price a ride locally without extra setup, and are
	// expected to be overridden by real ops-owned values before any real
	// deployment.
	PricingRatesJSON   string
	PricingRateVersion string

	// ServiceSecrets are HMAC secrets for inbound internal callers (e.g.
	// support-dispute-service resolving a driver identity).
	ServiceSecrets      string
	AdminReadEnabled    bool
	AdminReadSecretFile string

	// MetricsAuthToken gates GET <api-base>/outbox-metrics. Empty means the
	// route refuses every request: backlog and dead-letter counts describe
	// where the system is weak, and the safe default for that is closed.
	MetricsAuthToken string

	// Fleet Partner integration is optional while rolled out, but once
	// configured all corporate provider checks use HMAC v2 and Fleet's
	// canonical provider context.
	FleetServiceURL              string
	FleetServiceCaller           string
	FleetServiceSecret           []byte
	FleetServiceTimeout          time.Duration
	TaxiFleetProviderWorkEnabled bool

	// Section 7 flags. All default false; multi-pickup is only advertised
	// when execution, secure handover and cumulative fit are all enabled.
	ServiceProductsEnabled        bool
	CapabilityMatchingEnabled     bool
	GoodsCapabilityEnabled        bool
	GoodsMultiPickupEnabled       bool
	GoodsSecureHandoverEnabled    bool
	GoodsCumulativeFitEnabled     bool
	RecoveryEnabled               bool
	ScheduledRiskMonitorEnabled   bool
	CustomerMovementEventsEnabled bool
	CustomerContextEnabled        bool
	CustomerServiceURL            string
	CustomerServiceSecret         []byte
	// Section 13. Analytics is advisory: off by default, and a Taxi journey
	// never waits on it.
	AnalyticsEventsEnabled bool
	AnalyticsURL           string
	AnalyticsSecret        []byte
	// RecoveryNotificationsEnabled tells customers, drivers, fleets and
	// operations about recovery (Section 14). Off by default.
	RecoveryNotificationsEnabled bool

	CapabilityInternalCallers           string
	TaxiFleetCompletedWorkEnabled       bool
	TaxiTypedFinancialVisibilityEnabled bool
	// TaxiFleetManagedExecutionEnabled mounts the Fleet-facing private
	// execution surface (reservation, provider confirmation, execution start,
	// reconciliation reads). Default false: Section 8 TAX-028 gates the whole
	// corporate Taxi chain behind certification.
	TaxiFleetManagedExecutionEnabled bool
	// TaxiFleetPrivateExecutionEnabled mounts the Fleet-facing PRIVATE
	// execution surface: corporate work that never reached a marketplace at
	// all, where Fleet already holds the order, the assignment and the
	// provider's acceptance and asks Taxi to create the trip.
	//
	// It is separate from the managed-execution flag because the two answer
	// different questions. Managed execution claims an EXISTING passenger
	// booking for an organization; private execution CREATES a booking that no
	// passenger requested. A deployment may legitimately want one without the
	// other, and sharing a flag would make that impossible to express.
	TaxiFleetPrivateExecutionEnabled bool
	// TaxiFleetKillSwitch stops every Fleet-facing surface immediately,
	// independently of the rollout flags. A rollout flag is a planning
	// decision; a kill switch has to work during an incident without anyone
	// reasoning about which of several flags also has to move.
	TaxiFleetKillSwitch bool
	// TaxiManagedSettlementEnabled isolates corporate settlement from the
	// legacy direct-to-provider wallet path (Section 8 TAX-020A). Default
	// false until Section 12 exists; turning it on stops corporate work
	// being paid directly to the provider.
	TaxiManagedSettlementEnabled bool
	// TaxiFleetExecutionCallers lists the service callers allowed to drive
	// taxi execution over the internal marketplace surface.
	TaxiFleetExecutionCallers []string
	// TaxiFleetReservationTTLSeconds bounds how long a Fleet reservation
	// holds a booking before it can be reclaimed.
	TaxiFleetReservationTTL time.Duration

	// Matching config
	BookingMatchTimeout int     // seconds a matched driver has to accept
	BookingSearchWindow int     // seconds to keep searching (rescanning) before unmatched
	ProviderOnlineTTL   int     // seconds a driver stays online without a heartbeat
	MatchMaxRadiusKm    float64 // drivers farther than this from pickup are not dispatched
	// MatchExpandedRadiusKm is used once a scan at MatchMaxRadiusKm finds no
	// candidates -- the matcher's one radius-expansion step (addendum's
	// "wave 2"; see MatchingConfig's own doc comment for why there is no
	// wave 3 in this build).
	MatchExpandedRadiusKm float64
	// MatchPollIntervalSeconds is how often the matcher rescans for
	// candidates or checks an outstanding offer's status.
	MatchPollIntervalSeconds int

	// ScheduleLeadMinutes is how far ahead of a scheduled pickup the scheduler
	// worker promotes the ride to matching, so a driver is found before the
	// pickup time rather than at it.
	ScheduleLeadMinutes int

	// Pickup verification (phase B5, addendum §9/§32). PickupVerifyRateWindow/
	// MaxAttempts is a short-burst Redis rate limit, layered on top of --
	// never a substitute for -- handover.MaxVerifyAttempts' durable
	// per-code cap in Postgres: the Postgres counter caps LIFETIME attempts
	// against one generated code, this caps the RATE of attempts within a
	// short window regardless of which code they are against.
	PickupVerifyRateWindow  time.Duration
	PickupVerifyMaxAttempts int

	// Active-trip telemetry (phase B6, addendum §8). TelemetryMaxSpeedKph
	// gates FlagImpossibleJump; TelemetryStaleSeconds gates FlagStale. Both
	// are generous on purpose -- these classify a point as untrustworthy for
	// live tracking / distance reconciliation, they do not reject it (see
	// internal/features/booking/telemetry's own package doc comment), so a
	// false positive here only means one GPS blip is not treated as the
	// driver's live position, not a lost or dropped point.
	TelemetryMaxSpeedKph  float64
	TelemetryStaleSeconds int

	// ReviewRevealWindow tunes phase B8's two-way review withholding rule
	// (addendum §13): a party's free-text review/tags stay hidden from the
	// other side until BOTH have submitted, or this many hours have passed
	// since completion, whichever comes first. Ratings themselves are never
	// withheld -- see bookingusecases' reviews.go.
	ReviewRevealWindow time.Duration
}

func Load() Config {
	return Config{
		CustomerLiveLocationEnabled: getEnvBool("TAXI_CUSTOMER_LIVE_LOCATION_ENABLED", false),
		CustomerLocationFreshFor:    time.Duration(getEnvInt("TAXI_CUSTOMER_LOCATION_FRESH_SECONDS", 30)) * time.Second,
		AppEnv:                      getEnv("APP_ENV", "development"),
		HTTPAddr:                    getEnv("HTTP_ADDR", ":8102"),

		Migration: getEnvBool("TAXI_MIGRATION", false),

		DatabaseURL: getEnv("TAXI_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5434/taxi_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("TAXI_REDIS_ADDR", "localhost:6381"),
			Password: os.Getenv("TAXI_REDIS_PASSWORD"),
			DB:       getEnvInt("TAXI_REDIS_DB", 0),
		},

		// Ride-lifecycle notifications are published onto a Redis stream that
		// notification-service consumes, so both services must point at the SAME
		// Redis instance. That is a different instance from this service's own
		// Redis (OTP challenges, availability, match locks), so it is configured
		// separately here. Defaults to this service's Redis, which is correct for
		// a single-Redis deployment but silently strands notifications when each
		// service runs its own — set TAXI_NOTIFICATION_REDIS_ADDR to
		// notification-service's Redis in that case.
		NotificationRedis: redisx.Config{
			Addr:     getEnv("TAXI_NOTIFICATION_REDIS_ADDR", getEnv("TAXI_REDIS_ADDR", "localhost:6381")),
			Password: getEnv("TAXI_NOTIFICATION_REDIS_PASSWORD", os.Getenv("TAXI_REDIS_PASSWORD")),
			DB:       getEnvInt("TAXI_NOTIFICATION_REDIS_DB", getEnvInt("TAXI_REDIS_DB", 0)),
		},

		ProviderTokenSecret:      []byte(os.Getenv("TAXI_PROVIDER_TOKEN_SECRET")),
		ProviderRefreshSecret:    []byte(os.Getenv("TAXI_PROVIDER_REFRESH_SECRET")),
		ProviderOTPSecret:        []byte(os.Getenv("TAXI_PROVIDER_OTP_SECRET")),
		CustomerTokenSecret:      []byte(os.Getenv("TAXI_CUSTOMER_TOKEN_SECRET")),
		HandoverSecret:           []byte(getEnv("HANDOVER_CODE_SECRET", "development-handover-code-secret")),
		TaxiGoodsDeliveryEnabled: getEnvBool("TAXI_GOODS_DELIVERY_ENABLED", false),
		TaxiGoodsPricingJSON:     os.Getenv("TAXI_GOODS_PRICING"),
		TaxiGoodsHandoverSecret:  []byte(os.Getenv("TAXI_GOODS_HANDOVER_SECRET")),
		TaxiGoodsQuoteTTL:        time.Duration(getEnvInt("TAXI_GOODS_QUOTE_TTL_SECONDS", 900)) * time.Second,
		MediaURL:                 os.Getenv("TAXI_MEDIA_URL"),
		MediaToken:               os.Getenv("TAXI_MEDIA_TOKEN"),
		VendorURL:                os.Getenv("TAXI_VENDOR_URL"),
		VendorSecret:             []byte(os.Getenv("TAXI_VENDOR_SECRET")),

		AccessTokenTTL:  time.Duration(getEnvInt("TAXI_ACCESS_TOKEN_TTL_SECONDS", 3600)) * time.Second,
		RefreshTokenTTL: time.Duration(getEnvInt("TAXI_REFRESH_TOKEN_TTL_DAYS", 30)) * 24 * time.Hour,
		OTPTTL:          time.Duration(getEnvInt("TAXI_OTP_TTL_SECONDS", 600)) * time.Second,
		OTPRateWindow:   time.Duration(getEnvInt("TAXI_OTP_RATE_WINDOW_SECONDS", 60)) * time.Second,
		OTPMaxRequests:  getEnvInt("TAXI_OTP_MAX_REQUESTS", 5),
		OTPMaxAttempts:  getEnvInt("TAXI_OTP_MAX_ATTEMPTS", 5),
		OTPDebug:        getEnvBool("TAXI_OTP_DEBUG", false),

		NotificationURL:    os.Getenv("TAXI_NOTIFICATION_URL"),
		NotificationSecret: []byte(os.Getenv("TAXI_NOTIFICATION_SECRET")),
		PaymentURL:         os.Getenv("TAXI_PAYMENT_URL"),
		PaymentSecret:      []byte(os.Getenv("TAXI_PAYMENT_SECRET")),
		RoutesAPIKey:       os.Getenv("TAXI_ROUTES_API_KEY"),

		ReferralFeedEnabled:    getEnvBool("TAXI_REFERRAL_FEED_ENABLED", false),
		ReferralServiceURL:     os.Getenv("TAXI_REFERRAL_SERVICE_URL"),
		ReferralServiceSecret:  []byte(os.Getenv("TAXI_REFERRAL_SERVICE_SECRET")),
		ReferralIdentityPepper: []byte(os.Getenv("TAXI_REFERRAL_IDENTITY_PEPPER")),

		PricingRatesJSON:   getEnv("TAXI_PROVISIONAL_RATES", defaultProvisionalRatesJSON),
		PricingRateVersion: getEnv("TAXI_PRICING_RATE_VERSION", "provisional-dev-v1"),

		ServiceSecrets:      os.Getenv("TAXI_SERVICE_SECRETS"),
		AdminReadEnabled:    os.Getenv("TAXI_ADMIN_READ_ENABLED") == "true",
		AdminReadSecretFile: os.Getenv("TAXI_ADMIN_READ_SECRET_FILE"),
		MetricsAuthToken:    os.Getenv("TAXI_METRICS_AUTH_TOKEN"),

		FleetServiceURL:              os.Getenv("FLEET_PARTNER_SERVICE_URL"),
		FleetServiceCaller:           getEnv("TAXI_FLEET_SERVICE_CALLER", "driver-taxi-service"),
		FleetServiceSecret:           []byte(os.Getenv("FLEET_PARTNER_SERVICE_SECRET")),
		FleetServiceTimeout:          time.Duration(getEnvInt("FLEET_PARTNER_SERVICE_TIMEOUT_SECONDS", 5)) * time.Second,
		TaxiFleetProviderWorkEnabled: getEnvBool("TAXI_FLEET_PROVIDER_WORK_ENABLED", false),

		ServiceProductsEnabled:        getEnvBool("TAXI_SERVICE_PRODUCTS_ENABLED", false),
		CapabilityMatchingEnabled:     getEnvBool("TAXI_CAPABILITY_MATCHING_ENABLED", false),
		GoodsCapabilityEnabled:        getEnvBool("TAXI_GOODS_CAPABILITY_ENABLED", false),
		GoodsMultiPickupEnabled:       getEnvBool("TAXI_GOODS_MULTI_PICKUP_ENABLED", false),
		GoodsSecureHandoverEnabled:    getEnvBool("TAXI_GOODS_SECURE_HANDOVER_ENABLED", false),
		GoodsCumulativeFitEnabled:     getEnvBool("TAXI_GOODS_CUMULATIVE_FIT_ENABLED", false),
		RecoveryEnabled:               getEnvBool("TAXI_RECOVERY_ENABLED", false),
		ScheduledRiskMonitorEnabled:   getEnvBool("TAXI_SCHEDULED_RISK_MONITOR_ENABLED", false),
		CustomerMovementEventsEnabled: getEnvBool("TAXI_CUSTOMER_MOVEMENT_EVENTS_ENABLED", false),
		CustomerContextEnabled:        getEnvBool("TAXI_CUSTOMER_CONTEXT_ENABLED", false),
		CustomerServiceURL:            os.Getenv("TAXI_CUSTOMER_SERVICE_URL"),
		CustomerServiceSecret:         []byte(os.Getenv("TAXI_CUSTOMER_SERVICE_SECRET")),
		AnalyticsEventsEnabled:        getEnvBool("TAXI_ANALYTICS_EVENTS_ENABLED", false),
		AnalyticsURL:                  os.Getenv("TAXI_ANALYTICS_URL"),
		AnalyticsSecret:               []byte(os.Getenv("TAXI_ANALYTICS_SECRET")),
		RecoveryNotificationsEnabled:  getEnvBool("TAXI_RECOVERY_NOTIFICATIONS_ENABLED", false),

		CapabilityInternalCallers:           getEnv("TAXI_CAPABILITY_INTERNAL_CALLERS", "vendor-service,operations-service,verification-compliance-service"),
		TaxiFleetCompletedWorkEnabled:       getEnvBool("TAXI_FLEET_COMPLETED_WORK_ENABLED", false),
		TaxiTypedFinancialVisibilityEnabled: getEnvBool("TAXI_TYPED_FINANCIAL_VISIBILITY_ENABLED", true),
		TaxiFleetManagedExecutionEnabled:    getEnvBool("TAXI_FLEET_MANAGED_EXECUTION_ENABLED", false),
		TaxiFleetPrivateExecutionEnabled:    getEnvBool("TAXI_FLEET_PRIVATE_EXECUTION_ENABLED", false),
		TaxiFleetKillSwitch:                 getEnvBool("TAXI_FLEET_KILL_SWITCH", false),
		TaxiManagedSettlementEnabled:        getEnvBool("TAXI_MANAGED_SETTLEMENT_ENABLED", false),
		TaxiFleetExecutionCallers:           splitCSV(getEnv("TAXI_FLEET_EXECUTION_CALLERS", "fleet-partner-service")),
		TaxiFleetReservationTTL:             time.Duration(getEnvInt("TAXI_FLEET_RESERVATION_TTL_SECONDS", 300)) * time.Second,

		BookingMatchTimeout: getEnvInt("TAXI_BOOKING_MATCH_TIMEOUT", 30),
		BookingSearchWindow: getEnvInt("TAXI_BOOKING_SEARCH_WINDOW", 60),
		// 90s and no higher. A longer TTL keeps force-quit drivers in the online
		// set, where they are still dispatchable and can consume the entire
		// search window before a reachable driver is ever offered the ride.
		ProviderOnlineTTL:        getEnvInt("TAXI_PROVIDER_ONLINE_TTL", 90),
		MatchMaxRadiusKm:         getEnvFloat("TAXI_MATCH_MAX_RADIUS_KM", 10),
		MatchExpandedRadiusKm:    getEnvFloat("TAXI_MATCH_EXPANDED_RADIUS_KM", 20),
		MatchPollIntervalSeconds: getEnvInt("TAXI_MATCH_POLL_INTERVAL_SECONDS", 2),

		ScheduleLeadMinutes: getEnvInt("TAXI_SCHEDULE_LEAD_MINUTES", 10),

		PickupVerifyRateWindow:  time.Duration(getEnvInt("TAXI_PICKUP_VERIFY_RATE_WINDOW_SECONDS", 60)) * time.Second,
		PickupVerifyMaxAttempts: getEnvInt("TAXI_PICKUP_VERIFY_MAX_ATTEMPTS", 5),

		// 180kph comfortably covers highway speeds plus GPS jitter margin;
		// 120s tolerates a normal offline-queue batch flush without flagging
		// it as stale.
		TelemetryMaxSpeedKph:  getEnvFloat("TAXI_TELEMETRY_MAX_SPEED_KPH", 180),
		TelemetryStaleSeconds: getEnvInt("TAXI_TELEMETRY_STALE_SECONDS", 120),

		// 48h: long enough that most riders/drivers have submitted a review
		// within it, short enough that neither side is locked out of ever
		// reading the other's remarks for an unreasonable time.
		ReviewRevealWindow: time.Duration(getEnvInt("TAXI_REVIEW_REVEAL_WINDOW_HOURS", 48)) * time.Hour,
	}
}

// Validate rejects unsafe partial enablement. Disabled integrations require no
// downstream credentials, while enabled ones require explicit secrets and
// identities; there are no development secret fallbacks.
func (c Config) Validate() error {
	var problems []string
	if c.ReferralFeedEnabled && (c.ReferralServiceURL == "" || len(c.ReferralServiceSecret) == 0 || len(c.ReferralIdentityPepper) == 0) {
		problems = append(problems, "TAXI_REFERRAL_FEED_ENABLED requires TAXI_REFERRAL_SERVICE_URL, TAXI_REFERRAL_SERVICE_SECRET and TAXI_REFERRAL_IDENTITY_PEPPER")
	}
	for name, secret := range map[string][]byte{
		"TAXI_PROVIDER_TOKEN_SECRET":   c.ProviderTokenSecret,
		"TAXI_PROVIDER_REFRESH_SECRET": c.ProviderRefreshSecret,
		"TAXI_PROVIDER_OTP_SECRET":     c.ProviderOTPSecret,
		"TAXI_CUSTOMER_TOKEN_SECRET":   c.CustomerTokenSecret,
	} {
		if len(secret) == 0 {
			problems = append(problems, name+" is required")
		}
	}

	// A prefixed internal URL is the single most common wiring defect on this
	// platform: every call 404s and the failure is swallowed by fire-and-forget
	// callers. Reject it at boot instead.
	if path := urlPath(c.PaymentURL); path != "" {
		problems = append(problems, "TAXI_PAYMENT_URL must be a bare origin, got path "+path)
	}
	if path := urlPath(c.NotificationURL); path != "" {
		problems = append(problems, "TAXI_NOTIFICATION_URL must be a bare origin, got path "+path)
	}

	// Payment and notification are optional locally (nil clients no-op), but a
	// production deployment must never silently inherit that behaviour.
	if strings.EqualFold(strings.TrimSpace(c.AppEnv), "production") {
		if strings.TrimSpace(c.PaymentURL) == "" || len(c.PaymentSecret) == 0 {
			problems = append(problems, "TAXI_PAYMENT_URL and TAXI_PAYMENT_SECRET are required in production")
		}
		if strings.TrimSpace(c.NotificationURL) == "" || len(c.NotificationSecret) == 0 {
			problems = append(problems, "TAXI_NOTIFICATION_URL and TAXI_NOTIFICATION_SECRET are required in production")
		}
	}

	if c.BookingSearchWindow < c.BookingMatchTimeout {
		problems = append(problems, "TAXI_BOOKING_SEARCH_WINDOW must be at least TAXI_BOOKING_MATCH_TIMEOUT")
	}
	if c.MatchMaxRadiusKm <= 0 {
		problems = append(problems, "TAXI_MATCH_MAX_RADIUS_KM must be greater than zero")
	}
	if c.MatchExpandedRadiusKm < c.MatchMaxRadiusKm {
		problems = append(problems, "TAXI_MATCH_EXPANDED_RADIUS_KM must be at least TAXI_MATCH_MAX_RADIUS_KM")
	}
	if c.MatchPollIntervalSeconds <= 0 {
		problems = append(problems, "TAXI_MATCH_POLL_INTERVAL_SECONDS must be greater than zero")
	}
	if c.PickupVerifyRateWindow <= 0 {
		problems = append(problems, "TAXI_PICKUP_VERIFY_RATE_WINDOW_SECONDS must be greater than zero")
	}
	if c.PickupVerifyMaxAttempts <= 0 {
		problems = append(problems, "TAXI_PICKUP_VERIFY_MAX_ATTEMPTS must be greater than zero")
	}
	if c.ScheduleLeadMinutes < 0 {
		problems = append(problems, "TAXI_SCHEDULE_LEAD_MINUTES must not be negative")
	}
	if c.TelemetryMaxSpeedKph <= 0 {
		problems = append(problems, "TAXI_TELEMETRY_MAX_SPEED_KPH must be greater than zero")
	}
	if c.TelemetryStaleSeconds <= 0 {
		problems = append(problems, "TAXI_TELEMETRY_STALE_SECONDS must be greater than zero")
	}
	if c.CapabilityMatchingEnabled && !c.ServiceProductsEnabled {
		problems = append(problems, "TAXI_CAPABILITY_MATCHING_ENABLED requires TAXI_SERVICE_PRODUCTS_ENABLED (run shadow evaluation first)")
	}
	if c.GoodsMultiPickupEnabled && (!c.TaxiGoodsDeliveryEnabled || !c.GoodsCapabilityEnabled || !c.GoodsSecureHandoverEnabled || !c.GoodsCumulativeFitEnabled) {
		problems = append(problems, "TAXI_GOODS_MULTI_PICKUP_ENABLED requires TAXI_GOODS_DELIVERY_ENABLED, TAXI_GOODS_CAPABILITY_ENABLED, TAXI_GOODS_SECURE_HANDOVER_ENABLED and TAXI_GOODS_CUMULATIVE_FIT_ENABLED")
	}
	if c.CustomerContextEnabled && !c.ServiceProductsEnabled {
		problems = append(problems, "TAXI_CUSTOMER_CONTEXT_ENABLED requires TAXI_SERVICE_PRODUCTS_ENABLED (saved needs are frozen into the product snapshot)")
	}
	if c.CustomerLiveLocationEnabled && len(serviceSecretsFor(c.ServiceSecrets)["customer-service"]) == 0 {
		problems = append(problems, "TAXI_CUSTOMER_LIVE_LOCATION_ENABLED requires customer-service in TAXI_SERVICE_SECRETS")
	}
	if c.CustomerMovementEventsEnabled || c.CustomerContextEnabled {
		if strings.TrimSpace(c.CustomerServiceURL) == "" || len(c.CustomerServiceSecret) == 0 {
			problems = append(problems, "TAXI_CUSTOMER_MOVEMENT_EVENTS_ENABLED and TAXI_CUSTOMER_CONTEXT_ENABLED require TAXI_CUSTOMER_SERVICE_URL and TAXI_CUSTOMER_SERVICE_SECRET")
		} else if path := urlPath(c.CustomerServiceURL); path != "" {
			problems = append(problems, "TAXI_CUSTOMER_SERVICE_URL must be a bare origin, got path "+path)
		}
	}
	if c.TaxiGoodsDeliveryEnabled {
		if strings.TrimSpace(c.TaxiGoodsPricingJSON) == "" {
			problems = append(problems, "TAXI_GOODS_PRICING is required when TAXI_GOODS_DELIVERY_ENABLED=true")
		}
		if len(c.TaxiGoodsHandoverSecret) < 32 {
			problems = append(problems, "TAXI_GOODS_HANDOVER_SECRET must be at least 32 bytes when Taxi Goods is enabled")
		}
		if c.TaxiGoodsQuoteTTL <= 0 {
			problems = append(problems, "TAXI_GOODS_QUOTE_TTL_SECONDS must be greater than zero")
		}
		if strings.TrimSpace(c.MediaURL) == "" || strings.TrimSpace(c.MediaToken) == "" {
			problems = append(problems, "TAXI_MEDIA_URL and TAXI_MEDIA_TOKEN are required when Taxi Goods is enabled")
		}
		if strings.TrimSpace(c.PaymentURL) == "" || len(c.PaymentSecret) == 0 {
			problems = append(problems, "TAXI_PAYMENT_URL and TAXI_PAYMENT_SECRET are required when Taxi Goods is enabled")
		}
		if len(serviceSecretsFor(c.ServiceSecrets)["vendor-service"]) == 0 {
			problems = append(problems, "TAXI_SERVICE_SECRETS must allow vendor-service when Taxi Goods is enabled")
		}
		if strings.TrimSpace(c.VendorURL) == "" || len(c.VendorSecret) == 0 {
			problems = append(problems, "TAXI_VENDOR_URL and TAXI_VENDOR_SECRET are required when Taxi Goods is enabled")
		}
	}
	if (c.TaxiFleetProviderWorkEnabled || c.TaxiFleetCompletedWorkEnabled || c.TaxiTypedFinancialVisibilityEnabled) &&
		(strings.TrimSpace(c.FleetServiceURL) == "" || strings.TrimSpace(c.FleetServiceCaller) == "" || len(c.FleetServiceSecret) == 0) &&
		strings.EqualFold(strings.TrimSpace(c.AppEnv), "production") {
		problems = append(problems, "FLEET_PARTNER_SERVICE_URL, TAXI_FLEET_SERVICE_CALLER and FLEET_PARTNER_SERVICE_SECRET are required for Taxi Fleet integration in production")
	}
	if path := urlPath(c.FleetServiceURL); path != "" {
		problems = append(problems, "FLEET_PARTNER_SERVICE_URL must be a bare origin, got path "+path)
	}
	// Private execution depends on Fleet unconditionally, in every
	// environment. Its whole safety rests on reading Fleet's own acceptance
	// record before creating a trip, so an unreachable Fleet is not a degraded
	// mode here -- it is a surface that would accept commands it cannot verify.
	if c.TaxiFleetPrivateExecutionEnabled && !c.TaxiFleetKillSwitch {
		if strings.TrimSpace(c.FleetServiceURL) == "" || strings.TrimSpace(c.FleetServiceCaller) == "" ||
			len(c.FleetServiceSecret) == 0 {
			problems = append(problems,
				"FLEET_PARTNER_SERVICE_URL, TAXI_FLEET_SERVICE_CALLER and FLEET_PARTNER_SERVICE_SECRET are required when TAXI_FLEET_PRIVATE_EXECUTION_ENABLED is true: acceptance cannot be verified without them")
		}
		if len(serviceSecretsFor(c.ServiceSecrets)) == 0 {
			problems = append(problems,
				"TAXI_SERVICE_SECRETS must carry a secret for fleet-partner-service when TAXI_FLEET_PRIVATE_EXECUTION_ENABLED is true")
		}
	}

	if len(problems) > 0 {
		return errors.New("configuration invalid: " + strings.Join(problems, "; "))
	}
	return nil
}

// splitCSV parses a comma-separated allowlist, dropping empty entries.
func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if name := strings.TrimSpace(part); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func serviceSecretsFor(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		name, secret, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			name, secret, ok = strings.Cut(strings.TrimSpace(part), ":")
		}
		if ok {
			out[strings.TrimSpace(name)] = strings.TrimSpace(secret)
		}
	}
	return out
}

// urlPath returns the path portion of a configured internal base URL, or "" if
// the value is empty or is already a bare origin.
func urlPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	rest := trimmed
	if _, after, ok := strings.Cut(trimmed, "://"); ok {
		rest = after
	}
	_, path, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return ""
	}
	return "/" + path
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
