package config

import "testing"

func withAuthSecrets(cfg Config) Config {
	cfg.Auth.AccessTokenSecret = []byte("test-access-secret")
	cfg.Auth.RefreshTokenSecret = []byte("test-refresh-secret")
	cfg.Auth.OTPSecret = []byte("test-otp-secret")
	return cfg
}

func TestLoad_Defaults(t *testing.T) {
	cfg := Load()
	if cfg.HTTPAddr == "" {
		t.Fatal("expected a default HTTP address")
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("expected a default database URL")
	}
	if cfg.Redis.Addr == "" {
		t.Fatal("expected a default redis address")
	}
	if cfg.Flags.VendorServiceEnabled {
		t.Fatal("VendorServiceEnabled must default to false")
	}
	if cfg.Flags.CommerceGlobalKillSwitch {
		t.Fatal("CommerceGlobalKillSwitch must default to false")
	}
	if cfg.Auth.OTPDebug {
		t.Fatal("Auth.OTPDebug must default to false")
	}
	if cfg.Auth.OTPTTL <= 0 || cfg.Auth.AccessTokenTTL <= 0 || cfg.Auth.RefreshTokenTTL <= 0 || cfg.Auth.StepUpTTL <= 0 {
		t.Fatal("expected positive default TTLs for Auth")
	}
}

func TestValidate_DefaultConfigIsValid(t *testing.T) {
	cfg := withAuthSecrets(Load())
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default (all-disabled) config with auth secrets must validate cleanly: %v", err)
	}
}

func TestValidate_RejectsMissingDatabaseURL(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.DatabaseURL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error for a missing database URL")
	}
}

func TestValidate_RejectsFeatureFlagWithoutServiceEnabled(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorCRMEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error: a feature flag enabled while VENDOR_SERVICE_ENABLED is false")
	}
}

func TestValidate_RejectsIntegrationFlagWithoutCredentials(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.PaymentWalletCommerceEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error: PAYMENT_WALLET_COMMERCE_ENABLED without wallet URL/secret")
	}
}

func TestValidate_AcceptsIntegrationFlagWithCredentials(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.PaymentWalletCommerceEnabled = true
	cfg.WalletURL = "http://localhost:8105"
	cfg.WalletSecret = "test-secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error once credentials are present: %v", err)
	}
}

func TestValidate_RejectsMissingAuthSecrets(t *testing.T) {
	cfg := Load() // no auth secrets set
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error: auth token/otp secrets are required unconditionally")
	}
}

func TestValidate_RejectsOTPDebugInProduction(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.AppEnv = "production"
	cfg.Auth.OTPDebug = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error: VENDOR_AUTH_OTP_DEBUG must never be true in production")
	}
}

func TestValidate_AllowsOTPDebugOutsideProduction(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.AppEnv = "development"
	cfg.Auth.OTPDebug = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected OTPDebug to be allowed outside production: %v", err)
	}
}

// TestValidate_SegmentationRequiresCRM keeps Section 8B from being switched on
// over a CRM foundation that is switched off. Its routes read Section 8A
// projections, so enabling it alone would register a surface over tables
// nothing populates and report empty audiences as real ones.
func TestValidate_SegmentationRequiresCRM(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.VendorCRMSegmentationEnabled = true
	cfg.Flags.VendorCRMEnabled = false
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error: segmentation enabled without the CRM foundation")
	}

	cfg.Flags.VendorCRMEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("segmentation with the CRM foundation must validate: %v", err)
	}
}

// TestValidate_MaterializationRequiresSegmentation keeps the worker from being
// enabled without the surface that owns the definitions it would evaluate.
func TestValidate_MaterializationRequiresSegmentation(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.VendorCRMEnabled = true
	cfg.Flags.VendorCRMSegmentMaterializationEnabled = true
	cfg.Flags.VendorCRMSegmentationEnabled = false
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error: materialization enabled without segmentation")
	}

	cfg.Flags.VendorCRMSegmentationEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("materialization with segmentation must validate: %v", err)
	}
}

// TestValidate_SegmentationRequiresServiceEnabled covers the master switch.
func TestValidate_SegmentationRequiresServiceEnabled(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorCRMEnabled = true
	cfg.Flags.VendorCRMSegmentationEnabled = true
	cfg.Flags.VendorServiceEnabled = false
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error: segmentation enabled while VENDOR_SERVICE_ENABLED is false")
	}
}

// TestLoad_SegmentationFlagsDefaultOff pins the safe default: a new deployment
// gets no Section 8B surface until it is deliberately turned on.
func TestLoad_SegmentationFlagsDefaultOff(t *testing.T) {
	cfg := Load()
	if cfg.Flags.VendorCRMSegmentationEnabled || cfg.Flags.VendorCRMSegmentMaterializationEnabled {
		t.Fatal("Section 8B flags must default to off")
	}
}

func TestValidate_Section8CRequiresCRM(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.VendorCRMInteractionsEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected interactions without CRM foundation to fail")
	}
	cfg.Flags.VendorCRMEnabled = true
	cfg.Flags.VendorCRMTasksEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Section 8C with CRM foundation must validate: %v", err)
	}
}

func TestLoad_Section8CFlagsDefaultOff(t *testing.T) {
	cfg := Load()
	if cfg.Flags.VendorCRMInteractionsEnabled || cfg.Flags.VendorCRMTasksEnabled {
		t.Fatal("Section 8C flags must default off")
	}
}

// TestValidate_CampaignsRequireCRMAndSegmentation pins the prerequisite that
// matters most: the Section 8B segment engine is the only source a campaign
// audience has, so campaigns without segmentation would be a surface that can
// define a campaign it could never populate.
func TestValidate_CampaignsRequireCRMAndSegmentation(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.VendorCRMCampaignsEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected campaigns without the CRM foundation to fail")
	}

	cfg.Flags.VendorCRMEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected campaigns without segmentation to fail")
	}

	cfg.Flags.VendorCRMSegmentationEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("campaigns with CRM and segmentation must validate: %v", err)
	}
}

// TestValidate_CampaignPreparationRequiresCampaigns keeps the audience worker
// from being enabled without the surface that owns the definitions it freezes.
func TestValidate_CampaignPreparationRequiresCampaigns(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.VendorCRMEnabled = true
	cfg.Flags.VendorCRMSegmentationEnabled = true
	cfg.Flags.VendorCRMCampaignPreparationEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected campaign preparation without campaigns to fail")
	}

	cfg.Flags.VendorCRMCampaignsEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("campaign preparation with campaigns must validate: %v", err)
	}
}

// TestLoad_Section8EFlagsDefaultOff pins the safe default: a new deployment
// gets no campaign surface until it is deliberately turned on.
func TestLoad_Section8EFlagsDefaultOff(t *testing.T) {
	cfg := Load()
	if cfg.Flags.VendorCRMCampaignsEnabled || cfg.Flags.VendorCRMCampaignPreparationEnabled || cfg.Flags.VendorCRMCampaignExecutionEnabled {
		t.Fatal("Section 8E flags must default off")
	}
}

func TestValidate_CampaignExecutionRequiresCampaignsAndNotification(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.VendorCRMEnabled = true
	cfg.Flags.VendorCRMSegmentationEnabled = true
	cfg.Flags.VendorCRMCampaignExecutionEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected execution without campaigns/Notification to fail")
	}
	cfg.Flags.VendorCRMCampaignsEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected execution without Notification to fail")
	}
	cfg.NotificationURL = "http://notification-service"
	cfg.NotificationSecret = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("execution with campaigns and Notification must validate: %v", err)
	}
}

// TestValidate_CorporateRequiresCRM pins the prerequisite: corporate accounts
// hold contacts that are references to Section 8A customer relationships, and
// corporate activity runs on the Section 8C engine. Corporate CRM without the
// CRM foundation would be a surface that could name customers it has no
// definition of.
func TestValidate_CorporateRequiresCRM(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.VendorCRMCorporateEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected corporate CRM without the CRM foundation to fail")
	}
	cfg.Flags.VendorCRMEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("corporate CRM with the CRM foundation must validate: %v", err)
	}
}

// TestValidate_CorporateRequiresServiceEnabled keeps the section behind the
// service switch as well as the CRM one.
func TestValidate_CorporateRequiresServiceEnabled(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorCRMEnabled = true
	cfg.Flags.VendorCRMCorporateEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected corporate CRM without the service flag to fail")
	}
}

// TestLoad_Section8GFlagDefaultsOff pins the safe default: a new deployment gets
// no corporate surface until it is deliberately turned on.
func TestLoad_Section8GFlagDefaultsOff(t *testing.T) {
	if Load().Flags.VendorCRMCorporateEnabled {
		t.Fatal("VENDOR_CRM_CORPORATE_ENABLED must default off")
	}
}

// TestValidate_IntelligenceRequiresCRM pins the prerequisite: every Section 8H
// score is computed from Section 8A commercial projections, so intelligence
// without the CRM foundation would be a surface with nothing to reason over.
func TestValidate_IntelligenceRequiresCRM(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.VendorCRMIntelligenceEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected intelligence without the CRM foundation to fail")
	}
	cfg.Flags.VendorCRMEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("intelligence with the CRM foundation must validate: %v", err)
	}
	// A materializer with nothing to materialize is a boot failure, not a
	// worker that claims jobs no surface can create.
	cfg.Flags.VendorCRMIntelligenceEnabled = false
	cfg.Flags.VendorCRMIntelligenceWorkerEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected the intelligence worker without intelligence to fail")
	}
}

// TestValidate_ExportsRequireCRMAndPrivateStorage is the one that matters most:
// an export with no private store cannot produce an artifact, and the only
// alternative to a private store is not writing the file at all.
func TestValidate_ExportsRequireCRMAndPrivateStorage(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.VendorCRMExportsEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected exports without the CRM foundation to fail")
	}
	cfg.Flags.VendorCRMEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected exports without private artifact storage to fail")
	}
	cfg.MediaURL = "http://media-file-service"
	cfg.MediaSecret = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("exports with CRM and Media configured must validate: %v", err)
	}
}

// TestLoad_Section8HFlagsDefaultOff pins the safe default: a new deployment
// computes no scores and can export nothing until deliberately switched on.
func TestLoad_Section8HFlagsDefaultOff(t *testing.T) {
	flags := Load().Flags
	if flags.VendorCRMIntelligenceEnabled || flags.VendorCRMIntelligenceWorkerEnabled ||
		flags.VendorCRMExportsEnabled {
		t.Fatal("Section 8H flags must default off")
	}
}

func TestValidate_ShopWritesRequireShopSurface(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.VendorShopCatalogueWritesEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Shop writes without COMMERCE_SHOP_ENABLED to fail")
	}
	cfg.Flags.CommerceShopEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Shop writes with the Shop surface must validate: %v", err)
	}
}

func TestLoad_Section12AFlagsDefaultOff(t *testing.T) {
	flags := Load().Flags
	if flags.CommerceShopEnabled || flags.VendorShopCatalogueWritesEnabled {
		t.Fatal("Section 12A flags must default off")
	}
}

// TestValidate_OrdersRequireTheQuoteLock keeps Section 17E from being enabled
// without the approved agreement it is the only legal source of. Without the
// quote lock it would be a surface with nothing it could ever order.
func TestValidate_OrdersRequireTheQuoteLock(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.CommerceOrdersEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error: orders enabled without the Section 17D quote lock")
	}

	cfg.Flags.CommerceShopEnabled = true
	cfg.Flags.VendorInventoryEnabled = true
	cfg.Flags.VendorInventoryWritesEnabled = true
	cfg.Flags.CommerceCheckoutEnabled = true
	cfg.Flags.VendorOwnDeliveryEnabled = true
	cfg.Flags.CommerceFulfilmentPlanningEnabled = true
	cfg.Flags.CommerceQuoteLockEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("orders with the full Section 17 chain must validate: %v", err)
	}
}

// TestLoad_Section17EFlagDefaultsOff pins the safe default: a new deployment
// creates no order until deliberately switched on.
func TestLoad_Section17EFlagDefaultsOff(t *testing.T) {
	if Load().Flags.CommerceOrdersEnabled {
		t.Fatal("COMMERCE_ORDERS_ENABLED must default off")
	}
}
