// Package config loads vendor-service configuration from the environment.
//
// This is the architecture-foundation configuration surface only: database,
// Redis, HTTP, internal service auth, and the feature-flag/kill-switch
// families defined by the Vendor Service architecture specification (see
// services/vendor-service/docs/architecture/11_FEATURE_FLAG_AND_ROLLOUT_MODEL.md).
// Individual feature sections add their own config as they are implemented.
package config

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/redisx"
)

// Config is vendor-service's full environment-derived configuration.
type Config struct {
	AppEnv   string
	HTTPAddr string

	Migration bool

	DatabaseURL string
	Redis       redisx.Config

	// ServiceSecrets are HMAC secrets (shared/go/serviceauth) for inbound
	// internal callers, in "caller=secret,caller2=secret2" form.
	ServiceSecrets string
	// ShopInternalCallers is the explicit comma-separated HMAC caller
	// allowlist for the Section 12 internal discovery/reservation contract.
	// Possessing a configured secret alone does not enable Shop authority.
	ShopInternalCallers string

	// Flags is the full feature-flag/kill-switch surface. Every high-risk
	// flag defaults to false/disabled until its implementation section is
	// verified (see 11_FEATURE_FLAG_AND_ROLLOUT_MODEL.md).
	Flags FeatureFlags

	// Downstream integration credentials. Each is empty/unset until the
	// corresponding platform-service change (see
	// docs/PLATFORM_SERVICE_CHANGE_REGISTER.md) has actually been made on the
	// receiving service; an empty URL/secret disables that integration
	// client rather than falling back to an insecure default.
	WalletURL, WalletSecret             string
	VerificationURL, VerificationSecret string
	MediaURL, MediaSecret               string
	NotificationURL, NotificationSecret string
	SupportURL, SupportSecret           string
	AdminURL, AdminSecret               string
	AnalyticsURL, AnalyticsSecret       string
	DispatchURL, DispatchSecret         string
	TaxiURL, TaxiSecret                 string
	HaulingURL, HaulingSecret           string
	InternalServiceCallTimeout          time.Duration

	// Auth is Section 5's (Authentication, Sessions and Staff IAM)
	// configuration surface.
	Auth AuthConfig
}

// AuthConfig is Section 5's configuration: token signing, OTP policy, rate
// limits, and step-up. See docs/architecture/09_SECURITY_AND_AUTHORITY_MODEL.md.
type AuthConfig struct {
	AccessTokenSecret  []byte
	RefreshTokenSecret []byte
	OTPSecret          []byte

	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	OTPTTL         time.Duration
	OTPMaxAttempts int
	OTPMaxResends  int
	// OTPDebug returns the raw code in the API response instead of sending
	// it — a non-production-only shortcut (Config.Validate refuses it when
	// AppEnv=production). Never logged either way (Section 5 rule 11).
	OTPDebug bool

	StepUpTTL time.Duration

	RedisKeyPrefix string

	RateLimits RateLimitConfig
}

// RateLimitConfig is the per-bucket max-per-window applied before each Auth
// flow does any expensive work (Section 5 rule 31).
type RateLimitConfig struct {
	StartMax, StartWindowSeconds               int
	ResendMax, ResendWindowSeconds             int
	VerifyMax, VerifyWindowSeconds             int
	RefreshMax, RefreshWindowSeconds           int
	StepUpStartMax, StepUpStartWindowSeconds   int
	StepUpVerifyMax, StepUpVerifyWindowSeconds int
}

// FeatureFlags is the flag/kill-switch family from
// docs/architecture/11_FEATURE_FLAG_AND_ROLLOUT_MODEL.md, section 17 of the
// implementation specification. All default false.
type FeatureFlags struct {
	VendorServiceEnabled          bool
	VendorOnboardingWritesEnabled bool
	VendorCRMEnabled              bool
	// Section 8A rollout flags. Reads and writes are separable so risky
	// external ingestion can be disabled without blinding operators, and each
	// external integration is gated independently because none of the
	// authorities exists yet. A disabled consent integration can only suppress
	// marketing eligibility, never enable it.
	VendorCRMWritesEnabled             bool
	VendorCRMCustomerProjectionEnabled bool
	VendorCRMConsentIntegrationEnabled bool
	VendorCRMPrivacyIntegrationEnabled bool
	// Section 8B rollout flags. Segmentation gates the whole tags/segments
	// surface; materialization additionally gates the dynamic evaluation
	// worker, so an operator can freeze rebuilds (during a database incident,
	// say) while definitions and previews stay usable. Write commands
	// additionally require VendorCRMWritesEnabled, so the feature can also be
	// exposed read-only.
	VendorCRMSegmentationEnabled           bool
	VendorCRMSegmentMaterializationEnabled bool
	// Section 8C read surfaces. Mutations additionally require the shared CRM
	// writes flag and remain subject to the commerce kill switch.
	VendorCRMInteractionsEnabled bool
	VendorCRMTasksEnabled        bool
	// Section 8D loyalty points. Mutations additionally require the shared CRM
	// writes flag. Turning this off stands the expiry sweeper down without
	// discarding due work: nothing expires late and nothing expired becomes
	// spendable, because every balance command settles expiry itself.
	VendorCRMLoyaltyEnabled bool
	// Section 8E campaign intelligence. Campaigns gate the whole definition and
	// audience surface; preparation additionally gates the audience worker, so
	// an operator can freeze expensive audience freezes while definitions and
	// previews stay usable. Turning preparation off never disturbs an audience
	// that is already prepared.
	VendorCRMCampaignsEnabled           bool
	VendorCRMCampaignPreparationEnabled bool
	VendorCRMCampaignExecutionEnabled   bool
	// Section 8G corporate customer CRM. Mutations additionally require the
	// shared CRM writes flag. Corporate interactions and tasks reuse the Section
	// 8C engine and therefore also observe its own flags.
	VendorCRMCorporateEnabled bool
	// Section 8H CRM intelligence and secure exports. Intelligence requires the
	// CRM foundation because a score is computed from Section 8A projections;
	// the worker flag stands the materializer down without hiding existing
	// scores; exports require the CRM foundation and, at runtime, a configured
	// private artifact store.
	VendorCRMIntelligenceEnabled       bool
	VendorCRMIntelligenceWorkerEnabled bool
	VendorCRMExportsEnabled            bool
	VendorInventoryEnabled             bool
	VendorPurchasingEnabled            bool
	VendorRecipesEnabled               bool

	// CommerceFoodEnabled is Section 9A's master switch: with it false no food
	// catalogue route is registered at all. VendorFoodCatalogueWritesEnabled
	// additionally gates mutations, so a rollout can expose the read surface
	// before it accepts edits. Both are subordinate to
	// CommerceGlobalKillSwitch, which can only ever withhold a write.
	CommerceFoodEnabled              bool
	VendorFoodCatalogueWritesEnabled bool

	CommerceShopEnabled              bool
	VendorShopCatalogueWritesEnabled bool
	CommerceMultiVendorEnabled       bool
	CommerceVariableWeightEnabled    bool
	CommerceBulkRFQEnabled           bool

	VendorOwnDeliveryEnabled bool

	DispatchCommerceIntegrationEnabled bool
	TaxiGoodsDeliveryEnabled           bool
	HaulingCommerceIntegrationEnabled  bool

	PaymentWalletCommerceEnabled         bool
	VendorVerificationIntegrationEnabled bool

	CommerceFulfilmentWorkersEnabled bool
	CommerceGlobalKillSwitch         bool
}

// Load reads configuration from the environment. It never panics; call
// Validate to reject an unsafe configuration explicitly.
func Load() Config {
	return Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPAddr: getEnv("HTTP_ADDR", ":8112"),

		Migration: getEnvBool("VENDOR_MIGRATION", false),

		DatabaseURL: getEnv("VENDOR_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5445/vendor_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("VENDOR_REDIS_ADDR", "localhost:6390"),
			Password: os.Getenv("VENDOR_REDIS_PASSWORD"),
			DB:       getEnvInt("VENDOR_REDIS_DB", 0),
		},

		ServiceSecrets:      os.Getenv("VENDOR_SERVICE_SECRETS"),
		ShopInternalCallers: os.Getenv("VENDOR_SHOP_INTERNAL_CALLERS"),

		Flags: FeatureFlags{
			VendorServiceEnabled:          getEnvBool("VENDOR_SERVICE_ENABLED", false),
			VendorOnboardingWritesEnabled: getEnvBool("VENDOR_ONBOARDING_WRITES_ENABLED", false),
			VendorCRMEnabled:              getEnvBool("VENDOR_CRM_ENABLED", false),

			VendorCRMWritesEnabled:             getEnvBool("VENDOR_CRM_WRITES_ENABLED", false),
			VendorCRMCustomerProjectionEnabled: getEnvBool("VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED", false),
			VendorCRMConsentIntegrationEnabled: getEnvBool("VENDOR_CRM_CONSENT_INTEGRATION_ENABLED", false),
			VendorCRMPrivacyIntegrationEnabled: getEnvBool("VENDOR_CRM_PRIVACY_INTEGRATION_ENABLED", false),

			VendorCRMSegmentationEnabled:           getEnvBool("VENDOR_CRM_SEGMENTATION_ENABLED", false),
			VendorCRMSegmentMaterializationEnabled: getEnvBool("VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED", false),
			VendorCRMInteractionsEnabled:           getEnvBool("VENDOR_CRM_INTERACTIONS_ENABLED", false),
			VendorCRMTasksEnabled:                  getEnvBool("VENDOR_CRM_TASKS_ENABLED", false),
			VendorCRMLoyaltyEnabled:                getEnvBool("VENDOR_CRM_LOYALTY_ENABLED", false),

			VendorCRMCampaignsEnabled:           getEnvBool("VENDOR_CRM_CAMPAIGNS_ENABLED", false),
			VendorCRMCampaignPreparationEnabled: getEnvBool("VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED", false),
			VendorCRMCampaignExecutionEnabled:   getEnvBool("VENDOR_CRM_CAMPAIGN_EXECUTION_ENABLED", false),

			VendorCRMCorporateEnabled:          getEnvBool("VENDOR_CRM_CORPORATE_ENABLED", false),
			VendorCRMIntelligenceEnabled:       getEnvBool("VENDOR_CRM_INTELLIGENCE_ENABLED", false),
			VendorCRMIntelligenceWorkerEnabled: getEnvBool("VENDOR_CRM_INTELLIGENCE_WORKER_ENABLED", false),
			VendorCRMExportsEnabled:            getEnvBool("VENDOR_CRM_EXPORTS_ENABLED", false),

			VendorInventoryEnabled:  getEnvBool("VENDOR_INVENTORY_ENABLED", false),
			VendorPurchasingEnabled: getEnvBool("VENDOR_PURCHASING_ENABLED", false),
			VendorRecipesEnabled:    getEnvBool("VENDOR_RECIPES_ENABLED", false),

			CommerceFoodEnabled:              getEnvBool("COMMERCE_FOOD_ENABLED", false),
			VendorFoodCatalogueWritesEnabled: getEnvBool("VENDOR_FOOD_CATALOGUE_WRITES_ENABLED", false),

			CommerceShopEnabled:              getEnvBool("COMMERCE_SHOP_ENABLED", false),
			VendorShopCatalogueWritesEnabled: getEnvBool("VENDOR_SHOP_CATALOGUE_WRITES_ENABLED", false),
			CommerceMultiVendorEnabled:       getEnvBool("COMMERCE_MULTI_VENDOR_ENABLED", false),
			CommerceVariableWeightEnabled:    getEnvBool("COMMERCE_VARIABLE_WEIGHT_ENABLED", false),
			CommerceBulkRFQEnabled:           getEnvBool("COMMERCE_BULK_RFQ_ENABLED", false),

			VendorOwnDeliveryEnabled: getEnvBool("VENDOR_OWN_DELIVERY_ENABLED", false),

			DispatchCommerceIntegrationEnabled: getEnvBool("DISPATCH_COMMERCE_INTEGRATION_ENABLED", false),
			TaxiGoodsDeliveryEnabled:           getEnvBool("TAXI_GOODS_DELIVERY_ENABLED", false),
			HaulingCommerceIntegrationEnabled:  getEnvBool("HAULING_COMMERCE_INTEGRATION_ENABLED", false),

			PaymentWalletCommerceEnabled:         getEnvBool("PAYMENT_WALLET_COMMERCE_ENABLED", false),
			VendorVerificationIntegrationEnabled: getEnvBool("VENDOR_VERIFICATION_INTEGRATION_ENABLED", false),

			CommerceFulfilmentWorkersEnabled: getEnvBool("COMMERCE_FULFILMENT_WORKERS_ENABLED", false),
			CommerceGlobalKillSwitch:         getEnvBool("COMMERCE_GLOBAL_KILL_SWITCH", false),
		},

		WalletURL: os.Getenv("VENDOR_WALLET_URL"), WalletSecret: os.Getenv("VENDOR_WALLET_SECRET"),
		VerificationURL: os.Getenv("VENDOR_VERIFICATION_URL"), VerificationSecret: os.Getenv("VENDOR_VERIFICATION_SECRET"),
		MediaURL: os.Getenv("VENDOR_MEDIA_URL"), MediaSecret: os.Getenv("VENDOR_MEDIA_SECRET"),
		NotificationURL: os.Getenv("VENDOR_NOTIFICATION_URL"), NotificationSecret: os.Getenv("VENDOR_NOTIFICATION_SECRET"),
		SupportURL: os.Getenv("VENDOR_SUPPORT_URL"), SupportSecret: os.Getenv("VENDOR_SUPPORT_SECRET"),
		AdminURL: os.Getenv("VENDOR_ADMIN_URL"), AdminSecret: os.Getenv("VENDOR_ADMIN_SECRET"),
		AnalyticsURL: os.Getenv("VENDOR_ANALYTICS_URL"), AnalyticsSecret: os.Getenv("VENDOR_ANALYTICS_SECRET"),
		DispatchURL: os.Getenv("VENDOR_DISPATCH_URL"), DispatchSecret: os.Getenv("VENDOR_DISPATCH_SECRET"),
		TaxiURL: os.Getenv("VENDOR_TAXI_URL"), TaxiSecret: os.Getenv("VENDOR_TAXI_SECRET"),
		HaulingURL: os.Getenv("VENDOR_HAULING_URL"), HaulingSecret: os.Getenv("VENDOR_HAULING_SECRET"),
		InternalServiceCallTimeout: time.Duration(getEnvInt("VENDOR_INTERNAL_SERVICE_TIMEOUT_SECONDS", 5)) * time.Second,

		Auth: AuthConfig{
			AccessTokenSecret:  []byte(os.Getenv("VENDOR_AUTH_ACCESS_TOKEN_SECRET")),
			RefreshTokenSecret: []byte(os.Getenv("VENDOR_AUTH_REFRESH_TOKEN_SECRET")),
			OTPSecret:          []byte(os.Getenv("VENDOR_AUTH_OTP_SECRET")),

			AccessTokenTTL:  time.Duration(getEnvInt("VENDOR_AUTH_ACCESS_TOKEN_TTL_SECONDS", 900)) * time.Second,
			RefreshTokenTTL: time.Duration(getEnvInt("VENDOR_AUTH_REFRESH_TOKEN_TTL_DAYS", 30)) * 24 * time.Hour,

			OTPTTL:         time.Duration(getEnvInt("VENDOR_AUTH_OTP_TTL_SECONDS", 300)) * time.Second,
			OTPMaxAttempts: getEnvInt("VENDOR_AUTH_OTP_MAX_ATTEMPTS", 5),
			OTPMaxResends:  getEnvInt("VENDOR_AUTH_OTP_MAX_RESENDS", 5),
			OTPDebug:       getEnvBool("VENDOR_AUTH_OTP_DEBUG", false),

			StepUpTTL: time.Duration(getEnvInt("VENDOR_AUTH_STEP_UP_TTL_SECONDS", 300)) * time.Second,

			RedisKeyPrefix: getEnv("VENDOR_AUTH_RATE_LIMIT_REDIS_PREFIX", "vendor:auth:ratelimit"),

			RateLimits: RateLimitConfig{
				StartMax: getEnvInt("VENDOR_AUTH_START_RATE_MAX", 5), StartWindowSeconds: getEnvInt("VENDOR_AUTH_START_RATE_WINDOW_SECONDS", 60),
				ResendMax: getEnvInt("VENDOR_AUTH_RESEND_RATE_MAX", 5), ResendWindowSeconds: getEnvInt("VENDOR_AUTH_RESEND_RATE_WINDOW_SECONDS", 60),
				VerifyMax: getEnvInt("VENDOR_AUTH_VERIFY_RATE_MAX", 10), VerifyWindowSeconds: getEnvInt("VENDOR_AUTH_VERIFY_RATE_WINDOW_SECONDS", 60),
				RefreshMax: getEnvInt("VENDOR_AUTH_REFRESH_RATE_MAX", 20), RefreshWindowSeconds: getEnvInt("VENDOR_AUTH_REFRESH_RATE_WINDOW_SECONDS", 60),
				StepUpStartMax: getEnvInt("VENDOR_AUTH_STEP_UP_START_RATE_MAX", 5), StepUpStartWindowSeconds: getEnvInt("VENDOR_AUTH_STEP_UP_START_RATE_WINDOW_SECONDS", 60),
				StepUpVerifyMax: getEnvInt("VENDOR_AUTH_STEP_UP_VERIFY_RATE_MAX", 10), StepUpVerifyWindowSeconds: getEnvInt("VENDOR_AUTH_STEP_UP_VERIFY_RATE_WINDOW_SECONDS", 60),
			},
		},
	}
}

// Validate rejects unsafe partial configuration. A feature flag being on
// without its required downstream credentials is refused rather than
// silently degrading to an insecure or no-op integration.
func (c Config) Validate() error {
	var problems []string

	if strings.TrimSpace(c.DatabaseURL) == "" {
		problems = append(problems, "VENDOR_DATABASE_URL is required")
	}
	if strings.TrimSpace(c.Redis.Addr) == "" {
		problems = append(problems, "VENDOR_REDIS_ADDR is required")
	}

	requireCreds := func(enabled bool, flagName, url, secret, urlVar, secretVar string) {
		if !enabled {
			return
		}
		if strings.TrimSpace(url) == "" || strings.TrimSpace(secret) == "" {
			problems = append(problems, flagName+" requires "+urlVar+" and "+secretVar)
		}
	}

	requireCreds(c.Flags.PaymentWalletCommerceEnabled, "PAYMENT_WALLET_COMMERCE_ENABLED", c.WalletURL, c.WalletSecret, "VENDOR_WALLET_URL", "VENDOR_WALLET_SECRET")
	requireCreds(c.Flags.VendorVerificationIntegrationEnabled, "VENDOR_VERIFICATION_INTEGRATION_ENABLED", c.VerificationURL, c.VerificationSecret, "VENDOR_VERIFICATION_URL", "VENDOR_VERIFICATION_SECRET")
	requireCreds(c.Flags.DispatchCommerceIntegrationEnabled, "DISPATCH_COMMERCE_INTEGRATION_ENABLED", c.DispatchURL, c.DispatchSecret, "VENDOR_DISPATCH_URL", "VENDOR_DISPATCH_SECRET")
	requireCreds(c.Flags.TaxiGoodsDeliveryEnabled, "TAXI_GOODS_DELIVERY_ENABLED", c.TaxiURL, c.TaxiSecret, "VENDOR_TAXI_URL", "VENDOR_TAXI_SECRET")
	requireCreds(c.Flags.HaulingCommerceIntegrationEnabled, "HAULING_COMMERCE_INTEGRATION_ENABLED", c.HaulingURL, c.HaulingSecret, "VENDOR_HAULING_URL", "VENDOR_HAULING_SECRET")
	for _, integration := range []struct{ name, urlValue, secret string }{{"Vendor Wallet onboarding", c.WalletURL, c.WalletSecret}, {"Vendor Media validation", c.MediaURL, c.MediaSecret}} {
		if (strings.TrimSpace(integration.urlValue) == "") != (strings.TrimSpace(integration.secret) == "") {
			problems = append(problems, integration.name+" requires both URL and secret or neither")
		}
	}
	for name, raw := range map[string]string{"VENDOR_VERIFICATION_URL": c.VerificationURL, "VENDOR_WALLET_URL": c.WalletURL, "VENDOR_MEDIA_URL": c.MediaURL} {
		if strings.TrimSpace(raw) != "" {
			parsed, err := url.ParseRequestURI(raw)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				problems = append(problems, name+" must be an absolute HTTP(S) URL")
			}
		}
	}
	if c.Flags.VendorVerificationIntegrationEnabled {
		allowed := false
		for _, pair := range strings.Split(c.ServiceSecrets, ",") {
			name, _, ok := strings.Cut(strings.TrimSpace(pair), "=")
			if !ok {
				name, _, ok = strings.Cut(strings.TrimSpace(pair), ":")
			}
			if ok && strings.TrimSpace(name) == "verification-compliance-service" {
				allowed = true
				break
			}
		}
		if !allowed {
			problems = append(problems, "VENDOR_SERVICE_SECRETS must allow verification-compliance-service when VENDOR_VERIFICATION_INTEGRATION_ENABLED=true")
		}
	}

	// The service master switch gates every commerce-writing feature flag; a
	// feature flag enabled while the service itself is off is a
	// configuration mistake, not a valid "partially enabled" state.
	if !c.Flags.VendorServiceEnabled {
		anyFeature := c.Flags.VendorOnboardingWritesEnabled || c.Flags.VendorCRMEnabled ||
			c.Flags.VendorCRMSegmentationEnabled || c.Flags.VendorCRMInteractionsEnabled || c.Flags.VendorCRMTasksEnabled ||
			c.Flags.VendorCRMLoyaltyEnabled || c.Flags.VendorCRMCampaignsEnabled ||
			c.Flags.VendorCRMCorporateEnabled ||
			c.Flags.VendorCRMIntelligenceEnabled ||
			c.Flags.VendorCRMExportsEnabled ||
			c.Flags.VendorInventoryEnabled || c.Flags.VendorPurchasingEnabled || c.Flags.VendorRecipesEnabled ||
			c.Flags.CommerceFoodEnabled || c.Flags.VendorFoodCatalogueWritesEnabled ||
			c.Flags.CommerceShopEnabled || c.Flags.VendorShopCatalogueWritesEnabled || c.Flags.CommerceMultiVendorEnabled ||
			c.Flags.CommerceVariableWeightEnabled || c.Flags.CommerceBulkRFQEnabled || c.Flags.VendorOwnDeliveryEnabled
		if anyFeature {
			problems = append(problems, "VENDOR_SERVICE_ENABLED must be true when any VENDOR_* or COMMERCE_* feature flag is enabled")
		}
	}

	// Section 8B sits on top of Section 8A's projections; enabling it without
	// the CRM foundation would register routes over tables nothing populates.
	// Materialization in turn needs the segmentation surface that owns its
	// definitions. A flag enabled without its prerequisite is a configuration
	// mistake, not a partially-enabled state.
	if c.Flags.VendorCRMSegmentationEnabled && !c.Flags.VendorCRMEnabled {
		problems = append(problems, "VENDOR_CRM_ENABLED must be true when VENDOR_CRM_SEGMENTATION_ENABLED=true")
	}
	if c.Flags.VendorCRMSegmentMaterializationEnabled && !c.Flags.VendorCRMSegmentationEnabled {
		problems = append(problems, "VENDOR_CRM_SEGMENTATION_ENABLED must be true when VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED=true")
	}
	if (c.Flags.VendorCRMInteractionsEnabled || c.Flags.VendorCRMTasksEnabled) && !c.Flags.VendorCRMEnabled {
		problems = append(problems, "VENDOR_CRM_ENABLED must be true when Section 8C interactions or tasks are enabled")
	}

	// Section 9A: catalogue writes without the catalogue itself would be a flag
	// with nothing to act on, which is a configuration mistake rather than a
	// partially-enabled state.
	if c.Flags.VendorFoodCatalogueWritesEnabled && !c.Flags.CommerceFoodEnabled {
		problems = append(problems, "COMMERCE_FOOD_ENABLED must be true when VENDOR_FOOD_CATALOGUE_WRITES_ENABLED=true")
	}
	if c.Flags.VendorShopCatalogueWritesEnabled && !c.Flags.CommerceShopEnabled {
		problems = append(problems, "COMMERCE_SHOP_ENABLED must be true when VENDOR_SHOP_CATALOGUE_WRITES_ENABLED=true")
	}
	if c.Flags.VendorCRMLoyaltyEnabled && !c.Flags.VendorCRMEnabled {
		problems = append(problems, "VENDOR_CRM_ENABLED must be true when VENDOR_CRM_LOYALTY_ENABLED=true")
	}

	// Section 8E draws its whole audience from the Section 8B segment engine
	// and has no second way to select customers, so campaigns without
	// segmentation would be a surface that can define a campaign it can never
	// populate. Preparation in turn needs the campaign surface that owns the
	// definitions it freezes.
	if c.Flags.VendorCRMCampaignsEnabled && !c.Flags.VendorCRMEnabled {
		problems = append(problems, "VENDOR_CRM_ENABLED must be true when VENDOR_CRM_CAMPAIGNS_ENABLED=true")
	}
	if c.Flags.VendorCRMCampaignsEnabled && !c.Flags.VendorCRMSegmentationEnabled {
		problems = append(problems, "VENDOR_CRM_SEGMENTATION_ENABLED must be true when VENDOR_CRM_CAMPAIGNS_ENABLED=true")
	}
	if c.Flags.VendorCRMCampaignPreparationEnabled && !c.Flags.VendorCRMCampaignsEnabled {
		problems = append(problems, "VENDOR_CRM_CAMPAIGNS_ENABLED must be true when VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED=true")
	}
	// Section 8G reuses the Section 8A relationship graph for contact identity
	// and the Section 8C engine for corporate activity, so it is meaningless
	// without the CRM foundation underneath it.
	if c.Flags.VendorCRMCorporateEnabled && !c.Flags.VendorCRMEnabled {
		problems = append(problems, "VENDOR_CRM_ENABLED must be true when VENDOR_CRM_CORPORATE_ENABLED=true")
	}
	// Section 8H computes every score from Section 8A commercial projections and
	// exports every dataset from CRM tables, so both halves are meaningless
	// without the CRM foundation underneath them.
	if c.Flags.VendorCRMIntelligenceEnabled && !c.Flags.VendorCRMEnabled {
		problems = append(problems, "VENDOR_CRM_ENABLED must be true when VENDOR_CRM_INTELLIGENCE_ENABLED=true")
	}
	// A materializer with nothing to materialize would claim jobs no surface
	// can create, so the worker switch requires the feature it serves.
	if c.Flags.VendorCRMIntelligenceWorkerEnabled && !c.Flags.VendorCRMIntelligenceEnabled {
		problems = append(problems, "VENDOR_CRM_INTELLIGENCE_ENABLED must be true when VENDOR_CRM_INTELLIGENCE_WORKER_ENABLED=true")
	}
	if c.Flags.VendorCRMExportsEnabled && !c.Flags.VendorCRMEnabled {
		problems = append(problems, "VENDOR_CRM_ENABLED must be true when VENDOR_CRM_EXPORTS_ENABLED=true")
	}
	// An export with no private store cannot produce an artifact, and the only
	// alternative to a private store is not writing the file at all. Requiring
	// the Media credentials up front turns that into a boot failure rather than
	// a queue of jobs that can never finish.
	if c.Flags.VendorCRMExportsEnabled &&
		(strings.TrimSpace(c.MediaURL) == "" || strings.TrimSpace(c.MediaSecret) == "") {
		problems = append(problems, "VENDOR_MEDIA_URL and VENDOR_MEDIA_SECRET are required when VENDOR_CRM_EXPORTS_ENABLED=true")
	}
	if c.Flags.VendorCRMCampaignExecutionEnabled && !c.Flags.VendorCRMCampaignsEnabled {
		problems = append(problems, "VENDOR_CRM_CAMPAIGNS_ENABLED must be true when VENDOR_CRM_CAMPAIGN_EXECUTION_ENABLED=true")
	}
	if c.Flags.VendorCRMCampaignExecutionEnabled && (strings.TrimSpace(c.NotificationURL) == "" || strings.TrimSpace(c.NotificationSecret) == "") {
		problems = append(problems, "VENDOR_NOTIFICATION_URL and VENDOR_NOTIFICATION_SECRET are required when VENDOR_CRM_CAMPAIGN_EXECUTION_ENABLED=true")
	}

	// Auth secrets are always required — authentication is core infrastructure,
	// not a toggleable feature (unlike VENDOR_SERVICE_ENABLED's commerce
	// features), matching every other service's unconditional token-secret
	// requirement (see e.g. hauling's Validate()).
	for name, secret := range map[string][]byte{
		"VENDOR_AUTH_ACCESS_TOKEN_SECRET":  c.Auth.AccessTokenSecret,
		"VENDOR_AUTH_REFRESH_TOKEN_SECRET": c.Auth.RefreshTokenSecret,
		"VENDOR_AUTH_OTP_SECRET":           c.Auth.OTPSecret,
	} {
		if len(secret) == 0 {
			problems = append(problems, name+" is required")
		}
	}
	// A debug shortcut that returns raw OTPs in the API response must never
	// be live in production (Section 5's own "test/dev-only delivery
	// shortcuts... must never be enabled by production configuration").
	if strings.EqualFold(strings.TrimSpace(c.AppEnv), "production") && c.Auth.OTPDebug {
		problems = append(problems, "VENDOR_AUTH_OTP_DEBUG must be false in production")
	}

	if len(problems) > 0 {
		return errors.New("configuration invalid: " + strings.Join(problems, "; "))
	}
	return nil
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
