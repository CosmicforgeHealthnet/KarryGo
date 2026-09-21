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

	handoverintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/handover"
	"cosmicforge/logistics/shared/go/redisx"
)

// Config is vendor-service's full environment-derived configuration.
type Config struct {
	AppEnv   string
	HTTPAddr string
	// ProcessMode controls deployment topology. "api" serves HTTP without
	// claiming jobs, "worker" claims jobs without a public listener, and
	// "all" preserves the legacy combined development mode.
	ProcessMode string
	LoadErrors  []string

	Migration bool

	DatabaseURL string
	Redis       redisx.Config

	// ServiceSecrets are HMAC secrets (shared/go/serviceauth) for inbound
	// internal callers, in "caller=secret,caller2=secret2" form.
	ServiceSecrets string
	// ShopInternalCallers is the explicit comma-separated HMAC caller
	// allowlist for the Section 12 internal discovery/reservation contract.
	// Possessing a configured secret alone does not enable Shop authority.
	ShopInternalCallers      string
	InventoryInternalCallers string

	// TaxonomyInternalCallers and SearchInternalCallers are the Section 13I
	// allowlists for the two root-mounted internal surfaces. They are kept
	// apart from each other on purpose: administering the global taxonomy and
	// reading search results are different authorities, and a service that
	// needs to run a discovery query has no reason to be able to publish a new
	// canonical vocabulary for every vendor on the platform.
	TaxonomyInternalCallers string
	SearchInternalCallers   string

	// Checkout is Section 17B's timing surface. The reservation TTL is
	// required configuration: holding a customer's stock for a duration
	// nobody chose is exactly what the section forbids.
	Checkout CheckoutConfig

	// Fulfilment is Section 17C's planning surface: quote timeout, plan TTL,
	// the consolidation bound and the expiry sweep period.
	Fulfilment FulfilmentConfig

	// Basket is customer shopping optimization: Optimize My Cart, budget
	// shopping and recurring baskets.
	Basket BasketConfig

	// Quote is Section 17D's immutable checkout quote timing surface.
	Quote QuoteConfig

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
	CustomerURL, CustomerSecret         string
	CustomerAccessTokenSecret           []byte
	InternalServiceCallTimeout          time.Duration
	// Auth is Section 5's (Authentication, Sessions and Staff IAM)
	// configuration surface.
	Auth AuthConfig
	// AbuseProtection centralizes non-authentication request limits. These are
	// configurable technical safety defaults, not product entitlements.
	AbuseProtection AbuseProtectionConfig
}

// CheckoutConfig is Section 17B's timing configuration.
type CheckoutConfig struct {
	// ReservationTTL is how long a validated checkout candidate holds real
	// physical stock. Nothing in Section 17B hardcodes it.
	ReservationTTL time.Duration

	// StrandedGrace is how long a checkout may sit mid-reservation before
	// recovery treats it as abandoned by a dead process and releases
	// whatever it might be holding.
	StrandedGrace time.Duration

	// RecoveryInterval is the background sweep period.
	RecoveryInterval time.Duration
}

// FulfilmentConfig is Section 17C's planning configuration.
type FulfilmentConfig struct {
	// QuoteTimeout bounds one call to a logistics authority. It is short: a
	// customer is waiting, and a slow authority must make the group
	// unserviceable rather than hang the checkout.
	QuoteTimeout time.Duration

	// PlanTTL bounds a plan independently of its quotes. The effective expiry
	// is always the earlier of this and the soonest quote inside the plan.
	PlanTTL time.Duration

	// ConsolidationRadiusMeters is how close two vendor pickups must be before
	// one mission may cover both. It is deliberately tight: no logistics
	// authority models a multi-pickup mission today, so consolidating distant
	// pickups would price a route nobody can drive.
	ConsolidationRadiusMeters int

	// ProximityRadiusMeters bounds CANDIDATE CONSTRUCTION rather than
	// execution: two pickups further apart than this are not operationally
	// related enough to evaluate as one mission at all.
	//
	// It is deliberately much wider than the consolidation radius above. A
	// candidate inside it is usually rejected today - no logistics runtime
	// drives a multi-stop collection - but rejecting it WITH a reason is what
	// lets Support answer "why was my order split", and lets operations count
	// how often the missing capability actually costs something.
	ProximityRadiusMeters int

	// DispatchMultiPickupSupported and HaulingMultiPickupSupported declare
	// whether those runtimes can execute a mission with several separate
	// collection points.
	//
	// Both default to FALSE, which is today's truth. They exist as
	// configuration rather than as a constant so the capability can be
	// switched on when the runtime genuinely gains it, without the planner
	// changing - and, just as importantly, so a deployment cannot acquire the
	// capability by accident.
	DispatchMultiPickupSupported bool
	HaulingMultiPickupSupported  bool
	// HaulingSecureHandoverRequired marks deployments/cohorts whose Commerce
	// activation depends on the Section 30 secure OTP+QR contract. It defaults
	// true and fails closed while the audited dependency is PARTIAL.
	HaulingSecureHandoverRequired bool

	// ExpiryInterval is the background sweep period for lapsed plans.
	ExpiryInterval time.Duration
}

// BasketConfig is customer shopping optimization's configuration.
type BasketConfig struct {
	// ProposalTTL is how long an optimization offer stands.
	//
	// Short, because a proposal quotes live prices, live sellable stock and
	// live logistics fees. One that outlived them would be an offer nobody is
	// behind, presented to a customer as a decision they can still make.
	ProposalTTL time.Duration

	// ExpiryInterval is the background sweep period for lapsed proposals.
	ExpiryInterval time.Duration

	// RecurrenceInterval is how often recurring baskets are checked for a due
	// occurrence.
	//
	// The interval affects PROMPTNESS, never correctness. Every occurrence is
	// claimed through a unique index before any work happens, so running this
	// more often produces the same one generation per occurrence - just
	// sooner.
	RecurrenceInterval time.Duration
}

// QuoteConfig bounds how long an immutable commercial agreement may be
// approved. Its effective expiry is also capped by checkout and fulfilment.
type QuoteConfig struct {
	TTL time.Duration
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
	// it Ã¢â‚¬â€ a non-production-only shortcut (Config.Validate refuses it when
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

type AbuseProtectionConfig struct {
	RedisKeyPrefix                   string
	SearchMax, SearchWindowSeconds   int
	QuoteMax, QuoteWindowSeconds     int
	SupportMax, SupportWindowSeconds int
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
	// VendorPrivacyRetentionEnabled is a kill switch for new worker claims.
	// Disabling it leaves every pending job and retained evidence untouched.
	VendorPrivacyRetentionEnabled bool
	VendorInventoryEnabled        bool
	VendorInventoryWritesEnabled  bool
	VendorPurchasingEnabled       bool
	VendorRecipesEnabled          bool
	// VendorCostEnabled is Section 16D's master switch. It is separate from
	// VendorRecipesEnabled on purpose: a vendor can run recipes and production
	// without exposing what its inputs cost, which is the same separation the
	// Section 16D permissions enforce at the IAM layer.
	VendorCostEnabled bool

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
	// CustomerMovementGroupingEnabled publishes the commercial grouping of a
	// purchase to customer-service. Grouping only: never execution state.
	CustomerMovementGroupingEnabled   bool
	HaulingCommerceIntegrationEnabled bool

	PaymentWalletCommerceEnabled         bool
	VendorVerificationIntegrationEnabled bool

	// CommerceCheckoutEnabled is Section 17B's master switch. Checkout is
	// separable from the cart on purpose: Section 17A's cart can be exposed
	// on its own, and standing checkout down must never take the cart with
	// it. CommerceCheckoutRecoveryEnabled additionally gates the background
	// sweep that releases lapsed and orphaned stock holds.
	CommerceCheckoutEnabled         bool
	CommerceCheckoutRecoveryEnabled bool

	// CommerceFulfilmentPlanningEnabled is Section 17C's master switch.
	// Planning depends on checkout, so it may not be enabled without it.
	CommerceFulfilmentPlanningEnabled bool
	CommerceFulfilmentWorkersEnabled  bool

	// Section 11's three-stage rollout. They are separate switches because
	// each stage is independently useful and independently reversible.
	//
	// VendorReadinessOrchestrationEnabled makes readiness authoritative: staff
	// declare state, the predictor produces a window, and history records who
	// said what. On its own it changes nothing about fulfilment -- it is the
	// observe-then-trust stage.
	//
	// VendorJITPickupEnabled defers mission creation until the readiness
	// prediction says a provider should come. It needs readiness, because
	// deferring against no prediction is just delaying.
	//
	// VendorAutoAuthoritySelectionEnabled lets Vendor choose the execution
	// authority from the authorities' own answers instead of a stored plan
	// decision. It needs JIT, because selecting at commit time is the whole
	// point of selecting automatically at all.
	VendorReadinessOrchestrationEnabled bool
	VendorJITPickupEnabled              bool
	VendorAutoAuthoritySelectionEnabled bool

	// CommerceBasketOptimizationEnabled is the master switch for Optimize My
	// Cart and budget shopping.
	CommerceBasketOptimizationEnabled bool

	// CommerceRecurringBasketEnabled additionally enables recurring baskets.
	//
	// A separate flag because recurring generation is the only part of
	// optimization that runs WITHOUT a customer asking. A deployment may
	// reasonably want the on-demand optimizer without a background worker
	// producing suggestions on a schedule, and collapsing the two would take
	// that choice away.
	CommerceRecurringBasketEnabled bool
	// CommerceQuoteLockEnabled exposes Section 17D immutable commercial
	// quote creation and approval. It depends on Sections 17B and 17C.
	CommerceQuoteLockEnabled bool
	// CommerceOrdersEnabled exposes Section 17E ParentOrder and VendorOrder
	// orchestration. An order can only be cut from an approved Section 17D
	// quote, so it may not be enabled without the quote lock.
	CommerceOrdersEnabled    bool
	CommerceGlobalKillSwitch bool

	// Section 13 canonical taxonomy and search. VendorTaxonomyEnabled is the
	// master switch for both the vendor classification surface and the
	// platform administration surface; taxonomy writes are separable so the
	// vocabulary can be exposed read-only during rollout.
	//
	// VendorSearchEnabled gates the internal Food/Shop search contract, and
	// VendorSearchAdminEnabled additionally gates ranking and projection
	// generation administration. Standing administration down leaves search
	// answering normally from the ACTIVE generation: a frozen rebuild pipeline
	// must never take discovery offline.
	VendorTaxonomyEnabled       bool
	VendorTaxonomyWritesEnabled bool
	VendorSearchEnabled         bool
	VendorSearchAdminEnabled    bool
}

// Load reads configuration from the environment. It never panics; call
// Validate to reject an unsafe configuration explicitly.
func Load() Config {
	var loadErrors []string
	boolValue := func(key string, fallback bool) bool {
		raw := os.Getenv(key)
		if raw == "" {
			return fallback
		}
		value, err := strconv.ParseBool(raw)
		if err != nil {
			loadErrors = append(loadErrors, key+" must be a boolean")
			return fallback
		}
		return value
	}
	cfg := Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8112"),
		ProcessMode: strings.ToLower(strings.TrimSpace(getEnv("VENDOR_PROCESS_MODE", "all"))),

		Migration: getEnvBool("VENDOR_MIGRATION", false),

		DatabaseURL: getEnv("VENDOR_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5445/vendor_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("VENDOR_REDIS_ADDR", "localhost:6390"),
			Password: os.Getenv("VENDOR_REDIS_PASSWORD"),
			DB:       getEnvInt("VENDOR_REDIS_DB", 0),
		},

		ServiceSecrets:           os.Getenv("VENDOR_SERVICE_SECRETS"),
		ShopInternalCallers:      os.Getenv("VENDOR_SHOP_INTERNAL_CALLERS"),
		InventoryInternalCallers: os.Getenv("VENDOR_INVENTORY_INTERNAL_CALLERS"),

		TaxonomyInternalCallers: os.Getenv("VENDOR_TAXONOMY_INTERNAL_CALLERS"),
		SearchInternalCallers:   os.Getenv("VENDOR_SEARCH_INTERNAL_CALLERS"),

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
			VendorPrivacyRetentionEnabled:      getEnvBool("VENDOR_PRIVACY_RETENTION_ENABLED", false),

			VendorInventoryEnabled:       getEnvBool("VENDOR_INVENTORY_ENABLED", false),
			VendorInventoryWritesEnabled: getEnvBool("VENDOR_INVENTORY_WRITES_ENABLED", false),
			VendorPurchasingEnabled:      getEnvBool("VENDOR_PURCHASING_ENABLED", false),
			VendorRecipesEnabled:         getEnvBool("VENDOR_RECIPES_ENABLED", false),
			VendorCostEnabled:            getEnvBool("VENDOR_COST_ENABLED", false),

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
			CustomerMovementGroupingEnabled:    getEnvBool("VENDOR_CUSTOMER_MOVEMENT_GROUPING_ENABLED", false),
			HaulingCommerceIntegrationEnabled:  getEnvBool("HAULING_COMMERCE_INTEGRATION_ENABLED", false),

			PaymentWalletCommerceEnabled:         getEnvBool("PAYMENT_WALLET_COMMERCE_ENABLED", false),
			VendorVerificationIntegrationEnabled: getEnvBool("VENDOR_VERIFICATION_INTEGRATION_ENABLED", false),

			CommerceCheckoutEnabled:         getEnvBool("COMMERCE_CHECKOUT_ENABLED", false),
			CommerceCheckoutRecoveryEnabled: getEnvBool("COMMERCE_CHECKOUT_RECOVERY_ENABLED", false),

			CommerceFulfilmentPlanningEnabled: getEnvBool("COMMERCE_FULFILMENT_PLANNING_ENABLED", false),
			CommerceFulfilmentWorkersEnabled:  getEnvBool("COMMERCE_FULFILMENT_WORKERS_ENABLED", false),

			VendorReadinessOrchestrationEnabled: getEnvBool("VENDOR_READINESS_ORCHESTRATION_ENABLED", false),
			VendorJITPickupEnabled:              getEnvBool("VENDOR_JIT_PICKUP_ENABLED", false),
			VendorAutoAuthoritySelectionEnabled: getEnvBool("VENDOR_AUTO_AUTHORITY_SELECTION_ENABLED", false),
			CommerceBasketOptimizationEnabled:   getEnvBool("COMMERCE_BASKET_OPTIMIZATION_ENABLED", false),
			CommerceRecurringBasketEnabled:      getEnvBool("COMMERCE_RECURRING_BASKET_ENABLED", false),
			CommerceQuoteLockEnabled:            getEnvBool("COMMERCE_QUOTE_LOCK_ENABLED", false),
			CommerceOrdersEnabled:               getEnvBool("COMMERCE_ORDERS_ENABLED", false),
			CommerceGlobalKillSwitch:            getEnvBool("COMMERCE_GLOBAL_KILL_SWITCH", false),

			VendorTaxonomyEnabled:       getEnvBool("VENDOR_TAXONOMY_ENABLED", false),
			VendorTaxonomyWritesEnabled: getEnvBool("VENDOR_TAXONOMY_WRITES_ENABLED", false),
			VendorSearchEnabled:         getEnvBool("VENDOR_SEARCH_ENABLED", false),
			VendorSearchAdminEnabled:    getEnvBool("VENDOR_SEARCH_ADMIN_ENABLED", false),
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
		CustomerURL: os.Getenv("VENDOR_CUSTOMER_URL"), CustomerSecret: os.Getenv("VENDOR_CUSTOMER_SECRET"),
		CustomerAccessTokenSecret:  []byte(os.Getenv("VENDOR_CUSTOMER_ACCESS_TOKEN_SECRET")),
		InternalServiceCallTimeout: time.Duration(getEnvInt("VENDOR_INTERNAL_SERVICE_TIMEOUT_SECONDS", 5)) * time.Second,

		Checkout: CheckoutConfig{
			ReservationTTL:   time.Duration(getEnvInt("COMMERCE_CHECKOUT_RESERVATION_TTL_SECONDS", 600)) * time.Second,
			StrandedGrace:    time.Duration(getEnvInt("COMMERCE_CHECKOUT_STRANDED_GRACE_SECONDS", 120)) * time.Second,
			RecoveryInterval: time.Duration(getEnvInt("COMMERCE_CHECKOUT_RECOVERY_INTERVAL_SECONDS", 30)) * time.Second,
		},

		Fulfilment: FulfilmentConfig{
			QuoteTimeout:              time.Duration(getEnvInt("COMMERCE_FULFILMENT_QUOTE_TIMEOUT_SECONDS", 8)) * time.Second,
			PlanTTL:                   time.Duration(getEnvInt("COMMERCE_FULFILMENT_PLAN_TTL_SECONDS", 900)) * time.Second,
			ConsolidationRadiusMeters: getEnvInt("COMMERCE_FULFILMENT_CONSOLIDATION_RADIUS_METERS", 150),
			ProximityRadiusMeters:     getEnvInt("COMMERCE_FULFILMENT_PROXIMITY_RADIUS_METERS", 3000),
			DispatchMultiPickupSupported: getEnvBool(
				"DISPATCH_MULTI_PICKUP_SUPPORTED", false,
			),
			HaulingMultiPickupSupported: getEnvBool(
				"HAULING_MULTI_PICKUP_SUPPORTED", false,
			),
			HaulingSecureHandoverRequired: getEnvBool(
				"HAULING_SECURE_HANDOVER_REQUIRED", true,
			),
			ExpiryInterval: time.Duration(getEnvInt("COMMERCE_FULFILMENT_EXPIRY_INTERVAL_SECONDS", 60)) * time.Second,
		},

		Basket: BasketConfig{
			ProposalTTL:        time.Duration(getEnvInt("COMMERCE_BASKET_PROPOSAL_TTL_SECONDS", 600)) * time.Second,
			ExpiryInterval:     time.Duration(getEnvInt("COMMERCE_BASKET_EXPIRY_INTERVAL_SECONDS", 60)) * time.Second,
			RecurrenceInterval: time.Duration(getEnvInt("COMMERCE_RECURRING_BASKET_INTERVAL_SECONDS", 60)) * time.Second,
		},

		Quote: QuoteConfig{
			TTL: time.Duration(getEnvInt("COMMERCE_QUOTE_TTL_SECONDS", 300)) * time.Second,
		},

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
		AbuseProtection: AbuseProtectionConfig{
			RedisKeyPrefix: getEnv("VENDOR_ABUSE_RATE_LIMIT_REDIS_PREFIX", "vendor:abuse:ratelimit"),
			SearchMax:      getEnvInt("VENDOR_SEARCH_RATE_MAX", 120), SearchWindowSeconds: getEnvInt("VENDOR_SEARCH_RATE_WINDOW_SECONDS", 60),
			QuoteMax: getEnvInt("VENDOR_QUOTE_RATE_MAX", 30), QuoteWindowSeconds: getEnvInt("VENDOR_QUOTE_RATE_WINDOW_SECONDS", 60),
			SupportMax: getEnvInt("VENDOR_SUPPORT_RATE_MAX", 10), SupportWindowSeconds: getEnvInt("VENDOR_SUPPORT_RATE_WINDOW_SECONDS", 3600),
		},
	}
	// Re-parse the operational controls strictly. Existing feature parsing is
	// retained for compatibility; these controls are release gates and a typo
	// must be a boot error rather than silently selecting a fallback.
	cfg.Migration = boolValue("VENDOR_MIGRATION", false)
	cfg.Flags.VendorServiceEnabled = boolValue("VENDOR_SERVICE_ENABLED", false)
	cfg.Flags.CommerceFulfilmentWorkersEnabled = boolValue("COMMERCE_FULFILMENT_WORKERS_ENABLED", false)
	cfg.Flags.CommerceGlobalKillSwitch = boolValue("COMMERCE_GLOBAL_KILL_SWITCH", false)
	cfg.LoadErrors = loadErrors
	return cfg
}

// Validate rejects unsafe partial configuration. A feature flag being on
// without its required downstream credentials is refused rather than
// silently degrading to an insecure or no-op integration.
func (c Config) Validate() error {
	problems := append([]string(nil), c.LoadErrors...)
	if c.ProcessMode != "all" && c.ProcessMode != "api" && c.ProcessMode != "worker" {
		problems = append(problems, "VENDOR_PROCESS_MODE must be one of all, api, worker")
	}

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
	requireCreds(c.Flags.CustomerMovementGroupingEnabled, "VENDOR_CUSTOMER_MOVEMENT_GROUPING_ENABLED", c.CustomerURL, c.CustomerSecret, "VENDOR_CUSTOMER_URL", "VENDOR_CUSTOMER_SECRET")
	requireCreds(c.Flags.HaulingCommerceIntegrationEnabled, "HAULING_COMMERCE_INTEGRATION_ENABLED", c.HaulingURL, c.HaulingSecret, "VENDOR_HAULING_URL", "VENDOR_HAULING_SECRET")
	if c.Flags.HaulingCommerceIntegrationEnabled &&
		c.Fulfilment.HaulingSecureHandoverRequired &&
		!handoverintegration.HaulingSecureHandover().Available() {
		problems = append(problems,
			"HAULING_COMMERCE_INTEGRATION_ENABLED is blocked: HAULING_SECURE_HANDOVER is not AVAILABLE")
	}
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
			c.Flags.VendorPrivacyRetentionEnabled ||
			c.Flags.VendorInventoryEnabled || c.Flags.VendorPurchasingEnabled || c.Flags.VendorRecipesEnabled ||
			c.Flags.VendorCostEnabled ||
			c.Flags.CommerceFoodEnabled || c.Flags.VendorFoodCatalogueWritesEnabled ||
			c.Flags.CommerceShopEnabled || c.Flags.VendorShopCatalogueWritesEnabled || c.Flags.CommerceMultiVendorEnabled ||
			c.Flags.CommerceVariableWeightEnabled || c.Flags.CommerceBulkRFQEnabled || c.Flags.VendorOwnDeliveryEnabled ||
			c.Flags.VendorTaxonomyEnabled || c.Flags.VendorSearchEnabled ||
			c.Flags.CommerceCheckoutEnabled || c.Flags.CommerceCheckoutRecoveryEnabled ||
			c.Flags.CommerceFulfilmentPlanningEnabled || c.Flags.CommerceFulfilmentWorkersEnabled ||
			c.Flags.CommerceQuoteLockEnabled || c.Flags.CommerceOrdersEnabled ||
			c.Flags.VendorReadinessOrchestrationEnabled || c.Flags.VendorJITPickupEnabled ||
			c.Flags.VendorAutoAuthoritySelectionEnabled
		if anyFeature {
			problems = append(problems, "VENDOR_SERVICE_ENABLED must be true when any VENDOR_* or COMMERCE_* feature flag is enabled")
		}
	}

	// Section 8B sits on top of Section 8A's projections; enabling it without
	// the CRM foundation would register routes over tables nothing populates.
	// Materialization in turn needs the segmentation surface that owns its
	// definitions. A flag enabled without its prerequisite is a configuration
	// mistake, not a partially-enabled state.
	// Section 11's stages depend on each other in one direction only, and each
	// refusal below prevents a specific silent failure.
	//
	// JIT without readiness would defer commitment against no prediction,
	// which is not just-in-time -- it is late.
	if c.Flags.VendorJITPickupEnabled && !c.Flags.VendorReadinessOrchestrationEnabled {
		problems = append(problems,
			"VENDOR_READINESS_ORCHESTRATION_ENABLED must be true when VENDOR_JIT_PICKUP_ENABLED=true")
	}
	// Just-in-time commitment replaces the synchronous dispatch that happens
	// during order activation. That makes Section 17E orders a hard
	// prerequisite rather than a nicety: with orders off there is no activation
	// to defer, so every planned orchestration would sit at COMMIT_DUE with
	// nothing behind it.
	if c.Flags.VendorJITPickupEnabled && !c.Flags.CommerceOrdersEnabled {
		problems = append(problems,
			"COMMERCE_ORDERS_ENABLED must be true when VENDOR_JIT_PICKUP_ENABLED=true")
	}
	// And it needs an authority that can actually be asked. A deployment that
	// defers every mission to an authority it has no credentials for would
	// plan every order and dispatch none, which looks healthy from the outside
	// and delivers nothing.
	if c.Flags.VendorJITPickupEnabled && !c.anyLogisticsAuthorityConfigured() {
		problems = append(problems,
			"VENDOR_JIT_PICKUP_ENABLED requires at least one configured logistics authority "+
				"(DISPATCH_COMMERCE_INTEGRATION_ENABLED, TAXI_GOODS_DELIVERY_ENABLED or "+
				"HAULING_COMMERCE_INTEGRATION_ENABLED with its URL and secret)")
	}
	// Automatic selection without JIT would choose an authority at order time
	// from supply that will have changed by the time anyone collects.
	if c.Flags.VendorAutoAuthoritySelectionEnabled && !c.Flags.VendorJITPickupEnabled {
		problems = append(problems,
			"VENDOR_JIT_PICKUP_ENABLED must be true when VENDOR_AUTO_AUTHORITY_SELECTION_ENABLED=true")
	}
	// And automatic selection needs somebody to ask. With no logistics
	// authority integration enabled there is nothing to choose between, and
	// every order would fail selection while looking like a planning bug.
	if c.Flags.VendorAutoAuthoritySelectionEnabled &&
		!c.Flags.DispatchCommerceIntegrationEnabled &&
		!c.Flags.TaxiGoodsDeliveryEnabled &&
		!c.Flags.HaulingCommerceIntegrationEnabled {
		problems = append(problems,
			"VENDOR_AUTO_AUTHORITY_SELECTION_ENABLED requires at least one of "+
				"DISPATCH_COMMERCE_INTEGRATION_ENABLED, TAXI_GOODS_DELIVERY_ENABLED or "+
				"HAULING_COMMERCE_INTEGRATION_ENABLED")
	}

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

	// Section 13: the master switch gates both taxonomy surfaces, and writes
	// need a vocabulary to write into.
	if c.Flags.VendorTaxonomyWritesEnabled && !c.Flags.VendorTaxonomyEnabled {
		problems = append(problems, "VENDOR_TAXONOMY_ENABLED must be true when VENDOR_TAXONOMY_WRITES_ENABLED=true")
	}
	if c.Flags.VendorSearchAdminEnabled && !c.Flags.VendorSearchEnabled {
		problems = append(problems, "VENDOR_SEARCH_ENABLED must be true when VENDOR_SEARCH_ADMIN_ENABLED=true")
	}

	// An internal surface with no caller allowlist is a surface every caller is
	// refused from. That is safe but silent: it looks enabled, answers 403 to
	// everyone, and the misconfiguration only shows up as a mysterious outage in
	// whichever service was meant to consume it. Fail at startup instead.
	if c.Flags.VendorTaxonomyEnabled && strings.TrimSpace(c.TaxonomyInternalCallers) == "" {
		problems = append(problems, "VENDOR_TAXONOMY_INTERNAL_CALLERS is required when VENDOR_TAXONOMY_ENABLED=true")
	}
	if c.Flags.VendorSearchEnabled && strings.TrimSpace(c.SearchInternalCallers) == "" {
		problems = append(problems, "VENDOR_SEARCH_INTERNAL_CALLERS is required when VENDOR_SEARCH_ENABLED=true")
	}
	if c.Flags.VendorShopCatalogueWritesEnabled && !c.Flags.CommerceShopEnabled {
		problems = append(problems, "COMMERCE_SHOP_ENABLED must be true when VENDOR_SHOP_CATALOGUE_WRITES_ENABLED=true")
	}

	// Section 17B. Checkout validates against Food and Shop and holds stock
	// through Section 14, so enabling it without a commerce domain to
	// validate, or without the Inventory writes a hold requires, would be a
	// surface that can only ever refuse.
	if c.Flags.CommerceCheckoutEnabled {
		if !c.Flags.CommerceFoodEnabled && !c.Flags.CommerceShopEnabled {
			problems = append(problems, "COMMERCE_FOOD_ENABLED or COMMERCE_SHOP_ENABLED must be true when COMMERCE_CHECKOUT_ENABLED=true")
		}

		if c.Flags.CommerceShopEnabled &&
			(!c.Flags.VendorInventoryEnabled || !c.Flags.VendorInventoryWritesEnabled) {
			problems = append(problems, "VENDOR_INVENTORY_ENABLED and VENDOR_INVENTORY_WRITES_ENABLED must be true when COMMERCE_CHECKOUT_ENABLED=true with COMMERCE_SHOP_ENABLED=true")
		}

		if c.Checkout.ReservationTTL < 60*time.Second ||
			c.Checkout.ReservationTTL > 24*time.Hour {
			problems = append(problems, "COMMERCE_CHECKOUT_RESERVATION_TTL_SECONDS must be between 60 and 86400")
		}

		if c.Checkout.StrandedGrace <= 0 ||
			c.Checkout.StrandedGrace >= c.Checkout.ReservationTTL {
			problems = append(problems, "COMMERCE_CHECKOUT_STRANDED_GRACE_SECONDS must be positive and shorter than COMMERCE_CHECKOUT_RESERVATION_TTL_SECONDS")
		}

		if c.Checkout.RecoveryInterval < 5*time.Second ||
			c.Checkout.RecoveryInterval > time.Hour {
			problems = append(problems, "COMMERCE_CHECKOUT_RECOVERY_INTERVAL_SECONDS must be between 5 and 3600")
		}
	}

	if c.Flags.CommerceCheckoutRecoveryEnabled && !c.Flags.CommerceCheckoutEnabled {
		problems = append(problems, "COMMERCE_CHECKOUT_ENABLED must be true when COMMERCE_CHECKOUT_RECOVERY_ENABLED=true")
	}

	// Section 17C. Planning consumes validated Section 17B groups, so it
	// cannot be enabled without checkout. It must also have at least one
	// delivery authority available, or it would be a surface whose only
	// possible answer is "no delivery option available".
	if c.Flags.CommerceBasketOptimizationEnabled {
		// Optimization compares baskets by DELIVERED total, so it cannot
		// answer at all without the planner that prices delivery. Enabling it
		// alone would produce comparisons decided on merchandise price with a
		// zero delivery cost - confidently, and wrongly.
		if !c.Flags.CommerceFulfilmentPlanningEnabled {
			problems = append(problems, "COMMERCE_FULFILMENT_PLANNING_ENABLED must be true when COMMERCE_BASKET_OPTIMIZATION_ENABLED=true")
		}

		if !c.Flags.CommerceCheckoutEnabled {
			problems = append(problems, "COMMERCE_CHECKOUT_ENABLED must be true when COMMERCE_BASKET_OPTIMIZATION_ENABLED=true")
		}

		if c.Basket.ProposalTTL < 60*time.Second ||
			c.Basket.ProposalTTL > time.Hour {
			problems = append(problems, "COMMERCE_BASKET_PROPOSAL_TTL_SECONDS must be between 60 and 3600")
		}

		if c.Basket.ExpiryInterval < 5*time.Second ||
			c.Basket.ExpiryInterval > 10*time.Minute {
			problems = append(problems, "COMMERCE_BASKET_EXPIRY_INTERVAL_SECONDS must be between 5 and 600")
		}
	}

	if c.Flags.CommerceRecurringBasketEnabled {
		// Recurring generation is basket optimization on a schedule. Without
		// the optimizer there is nothing for the worker to run, and a flag
		// that silently does nothing is worse than one that refuses at boot.
		if !c.Flags.CommerceBasketOptimizationEnabled {
			problems = append(problems, "COMMERCE_BASKET_OPTIMIZATION_ENABLED must be true when COMMERCE_RECURRING_BASKET_ENABLED=true")
		}

		if c.Basket.RecurrenceInterval < 15*time.Second ||
			c.Basket.RecurrenceInterval > 15*time.Minute {
			problems = append(problems, "COMMERCE_RECURRING_BASKET_INTERVAL_SECONDS must be between 15 and 900")
		}
	}

	if c.Flags.CommerceFulfilmentPlanningEnabled {
		if !c.Flags.CommerceCheckoutEnabled {
			problems = append(problems, "COMMERCE_CHECKOUT_ENABLED must be true when COMMERCE_FULFILMENT_PLANNING_ENABLED=true")
		}

		if !c.Flags.VendorOwnDeliveryEnabled &&
			!c.Flags.DispatchCommerceIntegrationEnabled &&
			!c.Flags.HaulingCommerceIntegrationEnabled {
			problems = append(problems, "at least one of VENDOR_OWN_DELIVERY_ENABLED, DISPATCH_COMMERCE_INTEGRATION_ENABLED or HAULING_COMMERCE_INTEGRATION_ENABLED must be true when COMMERCE_FULFILMENT_PLANNING_ENABLED=true")
		}

		if c.Fulfilment.QuoteTimeout < time.Second ||
			c.Fulfilment.QuoteTimeout > time.Minute {
			problems = append(problems, "COMMERCE_FULFILMENT_QUOTE_TIMEOUT_SECONDS must be between 1 and 60")
		}

		if c.Fulfilment.PlanTTL < 60*time.Second ||
			c.Fulfilment.PlanTTL > 24*time.Hour {
			problems = append(problems, "COMMERCE_FULFILMENT_PLAN_TTL_SECONDS must be between 60 and 86400")
		}

		// A wide consolidation radius would merge pickups no single mission
		// can serve, producing a fee for a route nobody can drive. Two
		// kilometres is already generous for "the same market".
		if c.Fulfilment.ConsolidationRadiusMeters < 1 ||
			c.Fulfilment.ConsolidationRadiusMeters > 2000 {
			problems = append(problems, "COMMERCE_FULFILMENT_CONSOLIDATION_RADIUS_METERS must be between 1 and 2000")
		}

		if c.Fulfilment.ProximityRadiusMeters < 1 ||
			c.Fulfilment.ProximityRadiusMeters > 50000 {
			problems = append(problems, "COMMERCE_FULFILMENT_PROXIMITY_RADIUS_METERS must be between 1 and 50000")
		}

		// A proximity radius narrower than the co-location radius would make
		// every candidate a single stop by arithmetic instead of by
		// geography, which is exactly the misstatement the multi-stop gate
		// exists to prevent. It is refused at boot rather than silently
		// clamped, because a clamped value is a configuration the operator
		// believes is in force and is not.
		if c.Fulfilment.ProximityRadiusMeters <
			c.Fulfilment.ConsolidationRadiusMeters {
			problems = append(problems, "COMMERCE_FULFILMENT_PROXIMITY_RADIUS_METERS must be at least COMMERCE_FULFILMENT_CONSOLIDATION_RADIUS_METERS")
		}

		// A declared multi-pickup capability for an integration this
		// deployment cannot call is a claim about a runtime that will never
		// be asked. It is refused so the flag never sits in a config file
		// looking as though it did something.
		if c.Fulfilment.DispatchMultiPickupSupported &&
			!c.Flags.DispatchCommerceIntegrationEnabled {
			problems = append(problems, "DISPATCH_MULTI_PICKUP_SUPPORTED requires DISPATCH_COMMERCE_INTEGRATION_ENABLED=true")
		}

		if c.Fulfilment.HaulingMultiPickupSupported &&
			!c.Flags.HaulingCommerceIntegrationEnabled {
			problems = append(problems, "HAULING_MULTI_PICKUP_SUPPORTED requires HAULING_COMMERCE_INTEGRATION_ENABLED=true")
		}

		if c.Fulfilment.ExpiryInterval < 5*time.Second ||
			c.Fulfilment.ExpiryInterval > time.Hour {
			problems = append(problems, "COMMERCE_FULFILMENT_EXPIRY_INTERVAL_SECONDS must be between 5 and 3600")
		}
	}

	if c.Flags.CommerceFulfilmentWorkersEnabled &&
		!c.Flags.CommerceFulfilmentPlanningEnabled {
		problems = append(problems, "COMMERCE_FULFILMENT_PLANNING_ENABLED must be true when COMMERCE_FULFILMENT_WORKERS_ENABLED=true")
	}

	// Section 17D freezes the approved output of Sections 17B and 17C. It
	// cannot safely exist without both authorities, and its TTL is bounded so
	// stale product prices and delivery quotes never become long-lived offers.
	if c.Flags.CommerceQuoteLockEnabled {
		if !c.Flags.CommerceCheckoutEnabled {
			problems = append(problems, "COMMERCE_CHECKOUT_ENABLED must be true when COMMERCE_QUOTE_LOCK_ENABLED=true")
		}
		if !c.Flags.CommerceFulfilmentPlanningEnabled {
			problems = append(problems, "COMMERCE_FULFILMENT_PLANNING_ENABLED must be true when COMMERCE_QUOTE_LOCK_ENABLED=true")
		}
		if c.Quote.TTL < time.Minute || c.Quote.TTL > 24*time.Hour {
			problems = append(problems, "COMMERCE_QUOTE_TTL_SECONDS must be between 60 and 86400")
		}
	}

	// Section 17E cuts an order from an approved Section 17D quote. Without
	// the quote lock it would be a surface with nothing it could ever order.
	if c.Flags.CommerceOrdersEnabled && !c.Flags.CommerceQuoteLockEnabled {
		problems = append(problems, "COMMERCE_QUOTE_LOCK_ENABLED must be true when COMMERCE_ORDERS_ENABLED=true")
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

	// Auth secrets are always required Ã¢â‚¬â€ authentication is core infrastructure,
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

// anyLogisticsAuthorityConfigured reports whether at least one execution
// authority is both enabled and credentialled.
//
// It checks the credentials, not just the flag. An authority switched on
// without a URL or a secret cannot be called, and treating it as available is
// how a deployment passes validation and then fails every commit.
func (c Config) anyLogisticsAuthorityConfigured() bool {
	type binding struct {
		enabled bool
		url     string
		secret  string
	}
	for _, item := range []binding{
		{c.Flags.DispatchCommerceIntegrationEnabled, c.DispatchURL, c.DispatchSecret},
		{c.Flags.TaxiGoodsDeliveryEnabled, c.TaxiURL, c.TaxiSecret},
		{c.Flags.HaulingCommerceIntegrationEnabled, c.HaulingURL, c.HaulingSecret},
	} {
		if item.enabled && strings.TrimSpace(item.url) != "" && strings.TrimSpace(item.secret) != "" {
			return true
		}
	}
	return false
}
