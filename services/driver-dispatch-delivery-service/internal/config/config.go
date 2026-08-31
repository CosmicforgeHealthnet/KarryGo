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
	CustomerTokenSecret        []byte
	// HandoverSecret signs/verifies pickup & delivery handover codes (QR/OTP).
	// Service-local — no other service needs to match this value.
	HandoverSecret                []byte
	AvailabilityRadiusKM          float64
	NotificationURL               string
	NotificationSecret            []byte
	AnalyticsURL                  string
	AnalyticsSecret               []byte
	AnalyticsEventDeliveryEnabled bool
	ServiceSecrets                string
	// media-file-service integration, used by *_STORAGE_MODE=media (trip
	// proof, verification documents, vehicle documents, avatars) instead of
	// the local_private mode that only works on a single host. Bare origin —
	// mediaclient appends the full API path itself.
	MediaFileBaseURL      string
	MediaFileServiceToken string

	// Fleet internal API. Used by the marketplace relay to deliver provider
	// decisions and to verify provider acceptance. When the secret is empty
	// the marketplace clients fall back to their fail-closed variants rather
	// than half-working: an unconfigured deployment must refuse, not guess.
	FleetServiceURL    string
	FleetServiceSecret []byte

	// VendorServiceSecret is the shared HMAC secret Vendor Service signs its
	// commerce quote requests with. Empty means this deployment does not serve
	// commerce quotes at all, and the route is not registered.
	VendorServiceSecret          []byte
	VendorServiceURL             string
	RouteServiceURL              string
	RouteServiceTimeout          time.Duration
	CommerceEventDeliveryEnabled bool
	CommerceEventDeliveryPath    string

	// CommerceQuoteTTL is how long an issued commerce delivery quote stands.
	CommerceQuoteTTL    time.Duration
	FleetServiceCaller  string
	FleetServiceTimeout time.Duration

	// Customer serviceability/package-admission is additive and disabled by
	// default. It never changes the legacy booking route unless both controls
	// are explicitly enabled by deployment configuration.
	RouteServiceabilityEnabled bool
	PackagePolicyEnabled       bool
	ServiceabilityDegradedMode bool
	ServiceabilityTTL          time.Duration
	ServiceabilityMaxRouteKM   float64
	ServiceZoneCenterLat       float64
	ServiceZoneCenterLng       float64
	ServiceZoneRadiusKM        float64
	ServiceRestrictedZones     string

	// Additive Sections 7-8 capabilities. Every switch defaults false so the
	// established broadcast Request -> Trip flow remains the rollout default.
	SequentialAssignmentEnabled    bool
	DispatchQueueEnabled           bool
	OrganizationAssignmentEnabled  bool
	AdminAssistedAssignmentEnabled bool
	MultiVendorPlanningEnabled     bool
	AssignmentKillSwitch           bool
	AssignmentOfferWindow          time.Duration
	AssignmentWorkerInterval       time.Duration
	MetricsInterval                time.Duration
	AssignmentQueueRetryInterval   time.Duration
	PlanningMaxWeightGrams         int64
	PlanningMaxVolumeML            int64
	PlanningCapacityPolicyVersion  string

	// Additive secure handover/evidence controls. Safe defaults preserve the
	// legacy Trip OTP/proof workflow until an environment opts in.
	HandoverChallengeEnabled    bool
	PickupEvidenceGateEnabled   bool
	DeliveryEvidenceGateEnabled bool
	HandoverQREnabled           bool
	HandoverChallengeTTL        time.Duration
	HandoverWorkerInterval      time.Duration
	HandoverCaptureSessionTTL   time.Duration
	HandoverResendCooldown      time.Duration
	HandoverMaxAttempts         int
	HandoverMaxResends          int
	HandoverPolicyVersion       string
	HandoverEmailAllowed        bool
	HandoverInAppAllowed        bool
	HandoverPushAllowed         bool
	HandoverSMSAllowed          bool

	// Additive delivery-recovery and route-integrity controls. All default off
	// so existing Trip/failure/location behavior remains the rollout baseline.
	DeliveryRecoveryEnabled       bool
	ReturnWorkflowEnabled         bool
	HubHoldEnabled                bool
	ReplacementHandoverEnabled    bool
	RouteAlertsEnabled            bool
	OfflineReplayEnabled          bool
	ReceiverGracePeriod           time.Duration
	RecoverySameDayWindow         time.Duration
	RecoveryWorkerInterval        time.Duration
	ExternalOperationRetryLimit   int
	ExternalOperationBackoffBase  time.Duration
	ExternalOperationBackoffMax   time.Duration
	ExternalOperationLease        time.Duration
	ExternalServiceTimeout        time.Duration
	ApprovedHoldHubs              string
	RouteDeviationMeters          int
	RouteWrongWayToleranceDegrees int
	RouteStationaryThreshold      time.Duration
	RouteStaleThreshold           time.Duration
	RouteHighRiskDeviationMeters  int
	RouteHighRiskStaleThreshold   time.Duration
	RouteMaximumAccuracyMeters    float64
	OfflineMaximumClientClockSkew time.Duration

	// Organization marketplace feature flags. All default false: a write path
	// that is not explicitly enabled must not run.
	MarketplaceInternalAPIEnabled         bool
	MarketplaceReservationEnabled         bool
	MarketplaceExecutionEnabled           bool
	MarketplaceOutboxEnabled              bool
	MarketplaceProviderEligibilityEnabled bool
	MarketplaceVehicleEligibilityEnabled  bool
	PrivateExecutionEnabled               bool
	PrivateExecutionKillSwitch            bool
	ProviderOrganizationOffersEnabled     bool
	ProviderCompanyAssignmentsEnabled     bool
	ProviderOrganizationModeEnabled       bool

	// Dispatch-to-fleet provider decision relay.
	FleetRelayEnabled       bool
	FleetRelayBatchSize     int
	FleetRelayLeaseDuration time.Duration
	FleetRelayRetryLimit    int
	FleetRelayBackoffBase   time.Duration
	FleetRelayInterval      time.Duration

	// Dispatch-to-fleet canonical event delivery. Separate from the relay
	// above: the relay carries provider decisions to fleet, this drains the
	// marketplace outbox of request, trip and proof events. They are gated
	// independently so event delivery can be enabled without also enabling
	// the decision relay, or held back while the relay runs.
	EventDeliveryEnabled     bool
	EventDeliveryPath        string
	EventDeliveryBatchSize   int
	EventDeliveryLease       time.Duration
	EventDeliveryRetryLimit  int
	EventDeliveryBackoffBase time.Duration
	EventDeliveryBackoffMax  time.Duration
	EventDeliveryInterval    time.Duration

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

	cfg := Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		ServiceName: getEnv("SERVICE_NAME", "driver-dispatch-delivery-service"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8103"),
		DatabaseURL: getEnv("DISPATCH_DELIVERY_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5435/dispatch_delivery_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("DISPATCH_DELIVERY_REDIS_ADDR", "localhost:6382"),
			Password: os.Getenv("DISPATCH_DELIVERY_REDIS_PASSWORD"),
			DB:       getEnvInt("DISPATCH_DELIVERY_REDIS_DB", 0),
		},
		InternalServiceKey:             getEnv("DISPATCH_DELIVERY_INTERNAL_SERVICE_KEY", "development-internal-service-key"),
		AvailabilityServiceURL:         getEnv("AVAILABILITY_SERVICE_URL", "http://localhost:8103"),
		BroadcastInitialRadiusKM:       getEnvFloat("BROADCAST_INITIAL_RADIUS_KM", 5),
		BroadcastRadiusIncrementKM:     getEnvFloat("BROADCAST_RADIUS_INCREMENT_KM", 3),
		BroadcastMaxAttempts:           getEnvInt("BROADCAST_MAX_ATTEMPTS", 3),
		BroadcastWindow:                time.Duration(getEnvInt("BROADCAST_WINDOW_SECONDS", 30)) * time.Second,
		WalletServiceURL:               getEnv("WALLET_SERVICE_URL", "http://localhost:8105/api/v1/payment-wallet"),
		WalletServiceSecret:            []byte(walletServiceSecret),
		WalletServiceSource:            getEnv("WALLET_SERVICE_SOURCE", "dispatch-delivery-service"),
		CustomerTokenSecret:            []byte(getEnv("DISPATCH_CUSTOMER_TOKEN_SECRET", "development-customer-access-token-secret")),
		HandoverSecret:                 []byte(getEnv("HANDOVER_CODE_SECRET", "development-handover-code-secret")),
		AvailabilityRadiusKM:           getEnvFloat("DISPATCH_AVAILABILITY_RADIUS_KM", 10),
		FleetServiceURL:                getEnv("FLEET_PARTNER_SERVICE_URL", ""),
		FleetServiceSecret:             []byte(getEnv("FLEET_PARTNER_SERVICE_SECRET", "")),
		VendorServiceSecret:            []byte(getEnv("VENDOR_SERVICE_SECRET", "")),
		VendorServiceURL:               getEnv("VENDOR_SERVICE_URL", ""),
		RouteServiceURL:                getEnv("DISPATCH_ROUTE_SERVICE_URL", ""),
		RouteServiceTimeout:            time.Duration(getEnvInt("DISPATCH_ROUTE_SERVICE_TIMEOUT_SECONDS", 5)) * time.Second,
		CommerceEventDeliveryEnabled:   getEnvBool("DISPATCH_COMMERCE_EVENT_DELIVERY_ENABLED", false),
		CommerceEventDeliveryPath:      getEnv("DISPATCH_COMMERCE_EVENT_DELIVERY_PATH", "/api/v1/internal/dispatch/v1/commerce/events"),
		CommerceQuoteTTL:               time.Duration(getEnvInt("COMMERCE_QUOTE_TTL_SECONDS", 900)) * time.Second,
		FleetServiceCaller:             getEnv("FLEET_PARTNER_SERVICE_CALLER", "driver-dispatch-delivery-service"),
		FleetServiceTimeout:            time.Duration(getEnvInt("FLEET_PARTNER_SERVICE_TIMEOUT_SECONDS", 5)) * time.Second,
		RouteServiceabilityEnabled:     getEnvBool("DISPATCH_ROUTE_SERVICEABILITY_ENABLED", false),
		PackagePolicyEnabled:           getEnvBool("DISPATCH_PACKAGE_POLICY_ENABLED", false),
		ServiceabilityDegradedMode:     getEnvBool("DISPATCH_SERVICEABILITY_DEGRADED_MODE_ENABLED", false),
		ServiceabilityTTL:              time.Duration(getEnvInt("DISPATCH_SERVICEABILITY_TTL_SECONDS", 900)) * time.Second,
		ServiceabilityMaxRouteKM:       getEnvFloat("DISPATCH_SERVICEABILITY_MAX_ROUTE_KM", 60),
		ServiceZoneCenterLat:           getEnvFloat("DISPATCH_SERVICE_ZONE_CENTER_LAT", 0),
		ServiceZoneCenterLng:           getEnvFloat("DISPATCH_SERVICE_ZONE_CENTER_LNG", 0),
		ServiceZoneRadiusKM:            getEnvFloat("DISPATCH_SERVICE_ZONE_RADIUS_KM", 0),
		ServiceRestrictedZones:         getEnv("DISPATCH_SERVICE_RESTRICTED_ZONES", ""),
		SequentialAssignmentEnabled:    getEnvBool("DISPATCH_SEQUENTIAL_MATCHING_ENABLED", getEnvBool("DISPATCH_SEQUENTIAL_ASSIGNMENT_ENABLED", false)),
		DispatchQueueEnabled:           getEnvBool("DISPATCH_QUEUE_ENABLED", false),
		OrganizationAssignmentEnabled:  getEnvBool("DISPATCH_ORGANIZATION_ASSIGNMENT_ENABLED", false),
		AdminAssistedAssignmentEnabled: getEnvBool("DISPATCH_ADMIN_ASSISTED_ASSIGNMENT_ENABLED", false),
		MultiVendorPlanningEnabled:     getEnvBool("DISPATCH_MULTI_STOP_MISSIONS_ENABLED", getEnvBool("DISPATCH_MULTI_VENDOR_PLANNING_ENABLED", false)),
		AssignmentKillSwitch:           getEnvBool("DISPATCH_ASSIGNMENT_KILL_SWITCH", false),
		AssignmentOfferWindow:          time.Duration(getEnvInt("DISPATCH_ASSIGNMENT_OFFER_WINDOW_SECONDS", 75)) * time.Second,
		AssignmentWorkerInterval:       time.Duration(getEnvInt("DISPATCH_ASSIGNMENT_WORKER_INTERVAL_SECONDS", 5)) * time.Second,
		MetricsInterval:                time.Duration(getEnvInt("DISPATCH_METRICS_INTERVAL_SECONDS", 30)) * time.Second,
		AssignmentQueueRetryInterval:   time.Duration(getEnvInt("DISPATCH_QUEUE_RETRY_INTERVAL_SECONDS", 90)) * time.Second,
		PlanningMaxWeightGrams:         int64(getEnvInt("DISPATCH_MOTORCYCLE_MAX_WEIGHT_GRAMS", 25000)),
		PlanningMaxVolumeML:            int64(getEnvInt("DISPATCH_MOTORCYCLE_MAX_VOLUME_ML", 120000)),
		PlanningCapacityPolicyVersion:  getEnv("DISPATCH_CAPACITY_POLICY_VERSION", "dispatch_motorcycle_capacity_v1"),
		HandoverChallengeEnabled:       getEnvBool("DISPATCH_HANDOVER_CHALLENGE_ENABLED", false),
		PickupEvidenceGateEnabled:      getEnvBool("DISPATCH_PICKUP_EVIDENCE_GATE_ENABLED", false),
		DeliveryEvidenceGateEnabled:    getEnvBool("DISPATCH_DELIVERY_EVIDENCE_GATE_ENABLED", false),
		HandoverQREnabled:              getEnvBool("DISPATCH_HANDOVER_QR_ENABLED", false),
		HandoverChallengeTTL:           time.Duration(getEnvInt("DISPATCH_HANDOVER_CHALLENGE_TTL_SECONDS", 900)) * time.Second,
		HandoverWorkerInterval:         time.Duration(getEnvInt("DISPATCH_HANDOVER_WORKER_INTERVAL_SECONDS", 30)) * time.Second,
		HandoverCaptureSessionTTL:      time.Duration(getEnvInt("DISPATCH_EVIDENCE_CAPTURE_SESSION_TTL_SECONDS", 600)) * time.Second,
		HandoverResendCooldown:         time.Duration(getEnvInt("DISPATCH_HANDOVER_RESEND_COOLDOWN_SECONDS", 45)) * time.Second,
		HandoverMaxAttempts:            getEnvInt("DISPATCH_HANDOVER_MAX_ATTEMPTS", 5),
		HandoverMaxResends:             getEnvInt("DISPATCH_HANDOVER_MAX_RESENDS", 5),
		HandoverPolicyVersion:          getEnv("DISPATCH_HANDOVER_POLICY_VERSION", "dispatch_secure_handover_v1"),
		HandoverEmailAllowed:           getEnvBool("DELIVERY_HANDOVER_EMAIL_ALLOWED", true),
		HandoverInAppAllowed:           getEnvBool("DELIVERY_HANDOVER_IN_APP_ALLOWED", true),
		HandoverPushAllowed:            getEnvBool("DELIVERY_HANDOVER_PUSH_ALLOWED", true),
		HandoverSMSAllowed:             getEnvBool("DELIVERY_HANDOVER_SMS_ALLOWED", false),
		DeliveryRecoveryEnabled:        getEnvBool("DISPATCH_DELIVERY_RECOVERY_ENABLED", false),
		ReturnWorkflowEnabled:          getEnvBool("DISPATCH_RETURN_WORKFLOW_ENABLED", false),
		HubHoldEnabled:                 getEnvBool("DISPATCH_HUB_HOLD_ENABLED", false),
		ReplacementHandoverEnabled:     getEnvBool("DISPATCH_REPLACEMENT_HANDOVER_ENABLED", false),
		RouteAlertsEnabled:             getEnvBool("DISPATCH_ROUTE_ALERTS_ENABLED", false),
		OfflineReplayEnabled:           getEnvBool("DISPATCH_OFFLINE_REPLAY_ENABLED", false),
		ReceiverGracePeriod:            time.Duration(getEnvInt("DISPATCH_RECEIVER_GRACE_PERIOD_SECONDS", 600)) * time.Second,
		RecoverySameDayWindow:          time.Duration(getEnvInt("DISPATCH_RECOVERY_SAME_DAY_WINDOW_SECONDS", 28800)) * time.Second,
		RecoveryWorkerInterval:         time.Duration(getEnvInt("DISPATCH_RECOVERY_WORKER_INTERVAL_SECONDS", 10)) * time.Second,
		ExternalOperationRetryLimit:    getEnvInt("DISPATCH_EXTERNAL_OPERATION_RETRY_LIMIT", 10),
		ExternalOperationBackoffBase:   time.Duration(getEnvInt("DISPATCH_EXTERNAL_OPERATION_BACKOFF_BASE_SECONDS", 5)) * time.Second,
		ExternalOperationBackoffMax:    time.Duration(getEnvInt("DISPATCH_EXTERNAL_OPERATION_BACKOFF_MAX_SECONDS", 900)) * time.Second,
		ExternalOperationLease:         time.Duration(getEnvInt("DISPATCH_EXTERNAL_OPERATION_LEASE_SECONDS", 60)) * time.Second,
		ExternalServiceTimeout:         time.Duration(getEnvInt("DISPATCH_EXTERNAL_SERVICE_TIMEOUT_SECONDS", 10)) * time.Second,
		ApprovedHoldHubs:               getEnv("DISPATCH_APPROVED_HOLD_HUBS", ""),
		RouteDeviationMeters:           getEnvInt("DISPATCH_ROUTE_DEVIATION_METERS", 250),
		RouteWrongWayToleranceDegrees:  getEnvInt("DISPATCH_ROUTE_WRONG_WAY_TOLERANCE_DEGREES", 110),
		RouteStationaryThreshold:       time.Duration(getEnvInt("DISPATCH_ROUTE_STATIONARY_SECONDS", 600)) * time.Second,
		RouteStaleThreshold:            time.Duration(getEnvInt("DISPATCH_ROUTE_STALE_SECONDS", 120)) * time.Second,
		RouteHighRiskDeviationMeters:   getEnvInt("DISPATCH_ROUTE_HIGH_RISK_DEVIATION_METERS", 120),
		RouteHighRiskStaleThreshold:    time.Duration(getEnvInt("DISPATCH_ROUTE_HIGH_RISK_STALE_SECONDS", 60)) * time.Second,
		RouteMaximumAccuracyMeters:     getEnvFloat("DISPATCH_ROUTE_MAX_ACCURACY_METERS", 150),
		OfflineMaximumClientClockSkew:  time.Duration(getEnvInt("DISPATCH_OFFLINE_MAX_CLOCK_SKEW_SECONDS", 600)) * time.Second,

		MarketplaceInternalAPIEnabled:         getEnvBool("DISPATCH_MARKETPLACE_INTERNAL_API_ENABLED", false),
		MarketplaceReservationEnabled:         getEnvBool("DISPATCH_ORGANIZATION_RESERVATION_ENABLED", false),
		MarketplaceExecutionEnabled:           getEnvBool("DISPATCH_ORGANIZATION_EXECUTION_ENABLED", false),
		MarketplaceOutboxEnabled:              getEnvBool("DISPATCH_MARKETPLACE_OUTBOX_ENABLED", false),
		MarketplaceProviderEligibilityEnabled: getEnvBool("DISPATCH_PROVIDER_ELIGIBILITY_ENABLED", false),
		MarketplaceVehicleEligibilityEnabled:  getEnvBool("DISPATCH_VEHICLE_ELIGIBILITY_ENABLED", false),
		PrivateExecutionEnabled:               getEnvBool("DISPATCH_PRIVATE_EXECUTION_ENABLED", false),
		PrivateExecutionKillSwitch:            getEnvBool("DISPATCH_PRIVATE_EXECUTION_KILL_SWITCH", false),
		ProviderOrganizationOffersEnabled:     getEnvBool("DISPATCH_PROVIDER_ORGANIZATION_OFFERS_ENABLED", false),
		ProviderCompanyAssignmentsEnabled:     getEnvBool("DISPATCH_PROVIDER_COMPANY_ASSIGNMENTS_ENABLED", false),
		ProviderOrganizationModeEnabled:       getEnvBool("PROVIDER_ORGANIZATION_MODE_ENABLED", false),

		FleetRelayEnabled:       getEnvBool("DISPATCH_FLEET_RELAY_ENABLED", false),
		FleetRelayBatchSize:     getEnvInt("DISPATCH_FLEET_RELAY_BATCH_SIZE", 20),
		FleetRelayLeaseDuration: time.Duration(getEnvInt("DISPATCH_FLEET_RELAY_LEASE_DURATION", 120)) * time.Second,
		FleetRelayRetryLimit:    getEnvInt("DISPATCH_FLEET_RELAY_RETRY_LIMIT", 10),
		FleetRelayBackoffBase:   time.Duration(getEnvInt("DISPATCH_FLEET_RELAY_BACKOFF_BASE", 5)) * time.Second,
		FleetRelayInterval:      time.Duration(getEnvInt("DISPATCH_FLEET_RELAY_INTERVAL_SECONDS", 5)) * time.Second,

		EventDeliveryEnabled:     getEnvBool("DISPATCH_EVENT_DELIVERY_ENABLED", false),
		EventDeliveryPath:        getEnv("DISPATCH_EVENT_DELIVERY_PATH", "/internal/fleet-partner/v1/events"),
		EventDeliveryBatchSize:   getEnvInt("DISPATCH_EVENT_DELIVERY_BATCH_SIZE", 25),
		EventDeliveryLease:       time.Duration(getEnvInt("DISPATCH_EVENT_DELIVERY_LEASE_SECONDS", 120)) * time.Second,
		EventDeliveryRetryLimit:  getEnvInt("DISPATCH_EVENT_DELIVERY_RETRY_LIMIT", 10),
		EventDeliveryBackoffBase: time.Duration(getEnvInt("DISPATCH_EVENT_DELIVERY_BACKOFF_BASE_SECONDS", 5)) * time.Second,
		EventDeliveryBackoffMax:  time.Duration(getEnvInt("DISPATCH_EVENT_DELIVERY_BACKOFF_MAX_SECONDS", 900)) * time.Second,
		EventDeliveryInterval:    time.Duration(getEnvInt("DISPATCH_EVENT_DELIVERY_INTERVAL_SECONDS", 5)) * time.Second,
		AccessTokenSecret:        []byte(accessTokenSecret),
		RefreshTokenSecret:       []byte(refreshTokenSecret),
		OTPSecret:                []byte(otpSecret),
		AccessTokenTTL:           time.Duration(getEnvInt("DISPATCH_RIDER_JWT_ACCESS_TTL_MINUTES", 15)) * time.Minute,
		RefreshTokenTTL:          time.Duration(getEnvInt("DISPATCH_RIDER_JWT_REFRESH_TTL_DAYS", 30)) * 24 * time.Hour,
		OTPTTL:                   time.Duration(getEnvInt("DISPATCH_RIDER_OTP_TTL_MINUTES", 10)) * time.Minute,
		OTPRateWindow:            time.Duration(getEnvInt("DISPATCH_RIDER_OTP_RATE_LIMIT_WINDOW_MINUTES", 10)) * time.Minute,
		OTPMaxRequests:           getEnvInt("DISPATCH_RIDER_OTP_RATE_LIMIT_MAX", 3),
		OTPMaxAttempts:           getEnvInt("DISPATCH_RIDER_OTP_MAX_ATTEMPTS", 3),
		OTPLockoutTTL:            time.Duration(getEnvInt("DISPATCH_RIDER_OTP_LOCKOUT_MINUTES", 30)) * time.Minute,
		OTPDebug:                 getEnvBool("DISPATCH_RIDER_DEBUG_OTP", false),
		SMTPHost:                 getEnv("SMTP_HOST", ""),
		SMTPPort:                 getEnvInt("SMTP_PORT", 465),
		SMTPTLSMode:              getEnv("SMTP_TLS_MODE", ""),
		SMTPUser:                 getEnv("SMTP_USER", ""),
		SMTPPassword:             os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:                 getEnv("SMTP_FROM", ""),
		// Notification-service integration is optional: when unset, lifecycle
		// notifications no-op and the app notification proxy routes are skipped.
		NotificationURL:               getEnv("DISPATCH_NOTIFICATION_URL", ""),
		AnalyticsURL:                  getEnv("DISPATCH_ANALYTICS_URL", ""),
		AnalyticsSecret:               []byte(getEnv("DISPATCH_ANALYTICS_SECRET", "")),
		AnalyticsEventDeliveryEnabled: getEnvBool("DISPATCH_ANALYTICS_EVENT_DELIVERY_ENABLED", false),
		NotificationSecret:            []byte(getEnv("DISPATCH_NOTIFICATION_SECRET", "")),
		// HMAC secrets trusted for internal service-to-service calls
		// (name=secret pairs). Used by the internal identity endpoint that
		// support-dispute-service calls to resolve rider identities.
		ServiceSecrets:        getEnv("DISPATCH_SERVICE_SECRETS", "support-dispute-service=development-support-dispute-service-secret"),
		MediaFileBaseURL:      getEnv("MEDIA_FILE_BASE_URL", "http://localhost:8109"),
		MediaFileServiceToken: getEnv("MEDIA_FILE_SERVICE_TOKEN", "development-media-token"),
	}
	if cfg.ProviderOrganizationModeEnabled &&
		(strings.TrimSpace(cfg.FleetServiceURL) == "" || strings.TrimSpace(cfg.FleetServiceCaller) == "" || len(cfg.FleetServiceSecret) == 0) {
		return Config{}, fmt.Errorf("FLEET_PARTNER_SERVICE_URL, FLEET_PARTNER_SERVICE_CALLER and FLEET_PARTNER_SERVICE_SECRET are required when PROVIDER_ORGANIZATION_MODE_ENABLED is true")
	}
	if cfg.OTPDebug && cfg.AppEnv != "development" && cfg.AppEnv != "test" {
		return Config{}, fmt.Errorf("DISPATCH_RIDER_DEBUG_OTP is prohibited outside development/test")
	}
	if len(cfg.HandoverSecret) < 16 {
		return Config{}, fmt.Errorf("HANDOVER_CODE_SECRET must contain at least 16 bytes")
	}
	if string(cfg.HandoverSecret) == string(cfg.OTPSecret) {
		return Config{}, fmt.Errorf("HANDOVER_CODE_SECRET must be separate from the authentication OTP key")
	}
	if cfg.AppEnv != "development" && cfg.AppEnv != "test" && strings.HasPrefix(string(cfg.HandoverSecret), "development-") {
		return Config{}, fmt.Errorf("a deployment handover key is required outside development")
	}
	if (cfg.ReturnWorkflowEnabled || cfg.HubHoldEnabled || cfg.ReplacementHandoverEnabled) && !cfg.DeliveryRecoveryEnabled {
		return Config{}, fmt.Errorf("DISPATCH_DELIVERY_RECOVERY_ENABLED is required for return, hub-hold or replacement recovery")
	}
	if cfg.ReturnWorkflowEnabled && strings.TrimSpace(cfg.RouteServiceURL) == "" {
		return Config{}, fmt.Errorf("DISPATCH_ROUTE_SERVICE_URL is required when DISPATCH_RETURN_WORKFLOW_ENABLED is true")
	}
	if cfg.ReturnWorkflowEnabled && !cfg.SequentialAssignmentEnabled {
		return Config{}, fmt.Errorf("DISPATCH_SEQUENTIAL_MATCHING_ENABLED is required when DISPATCH_RETURN_WORKFLOW_ENABLED is true")
	}
	if cfg.HubHoldEnabled && strings.TrimSpace(cfg.ApprovedHoldHubs) == "" {
		return Config{}, fmt.Errorf("DISPATCH_APPROVED_HOLD_HUBS is required when DISPATCH_HUB_HOLD_ENABLED is true")
	}
	if cfg.ReplacementHandoverEnabled && !cfg.SequentialAssignmentEnabled {
		return Config{}, fmt.Errorf("DISPATCH_SEQUENTIAL_MATCHING_ENABLED is required when DISPATCH_REPLACEMENT_HANDOVER_ENABLED is true")
	}
	if cfg.OfflineReplayEnabled && !cfg.RouteAlertsEnabled {
		return Config{}, fmt.Errorf("DISPATCH_ROUTE_ALERTS_ENABLED is required when DISPATCH_OFFLINE_REPLAY_ENABLED is true")
	}
	if cfg.RouteAlertsEnabled && strings.TrimSpace(cfg.RouteServiceURL) == "" {
		return Config{}, fmt.Errorf("DISPATCH_ROUTE_SERVICE_URL is required when DISPATCH_ROUTE_ALERTS_ENABLED is true")
	}
	return cfg, nil
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
