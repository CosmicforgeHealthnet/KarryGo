package config

import (
	"reflect"
	"strings"
	"testing"
)

// AppRequiredFlags is the complete flag set the Vendor Flutter app needs so
// that no screen calls a route that is unregistered or permanently refusing.
// deploy/vendor-service/enable-staging-app-features.sh writes exactly this set
// (plus COMMERCE_GLOBAL_KILL_SWITCH=false); keep the two in sync.
//
// "external" flags need another service's URL and secret; the rest are
// vendor-service-internal.
var appRequiredFlags = map[string]bool{
	// vendor-service-internal
	"VENDOR_SERVICE_ENABLED":                     false,
	"VENDOR_ONBOARDING_WRITES_ENABLED":           false,
	"VENDOR_CRM_ENABLED":                         false,
	"VENDOR_CRM_WRITES_ENABLED":                  false,
	"VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED":     false,
	"VENDOR_CRM_CONSENT_INTEGRATION_ENABLED":     false,
	"VENDOR_CRM_PRIVACY_INTEGRATION_ENABLED":     false,
	"VENDOR_CRM_SEGMENTATION_ENABLED":            false,
	"VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED": false,
	"VENDOR_CRM_INTERACTIONS_ENABLED":            false,
	"VENDOR_CRM_TASKS_ENABLED":                   false,
	"VENDOR_CRM_LOYALTY_ENABLED":                 false,
	"VENDOR_CRM_CAMPAIGNS_ENABLED":               false,
	"VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED":    false,
	"VENDOR_CRM_CORPORATE_ENABLED":               false,
	"VENDOR_CRM_INTELLIGENCE_ENABLED":            false,
	"VENDOR_CRM_INTELLIGENCE_WORKER_ENABLED":     false,
	"VENDOR_PRIVACY_RETENTION_ENABLED":           false,
	"VENDOR_INVENTORY_ENABLED":                   false,
	"VENDOR_INVENTORY_WRITES_ENABLED":            false,
	"VENDOR_PURCHASING_ENABLED":                  false,
	"VENDOR_RECIPES_ENABLED":                     false,
	"VENDOR_COST_ENABLED":                        false,
	"COMMERCE_FOOD_ENABLED":                      false,
	"VENDOR_FOOD_CATALOGUE_WRITES_ENABLED":       false,
	"COMMERCE_SHOP_ENABLED":                      false,
	"VENDOR_SHOP_CATALOGUE_WRITES_ENABLED":       false,
	"CUSTOMER_FOOD_ENABLED":                      false,
	"CUSTOMER_SHOP_ENABLED":                      false,
	"COMMERCE_MULTI_VENDOR_ENABLED":              false,
	"COMMERCE_VARIABLE_WEIGHT_ENABLED":           false,
	"COMMERCE_BULK_RFQ_ENABLED":                  false,
	"VENDOR_OWN_DELIVERY_ENABLED":                false,
	"COMMERCE_CHECKOUT_ENABLED":                  false,
	"COMMERCE_CHECKOUT_RECOVERY_ENABLED":         false,
	"COMMERCE_FULFILMENT_PLANNING_ENABLED":       false,
	"COMMERCE_FULFILMENT_WORKERS_ENABLED":        false,
	"COMMERCE_QUOTE_LOCK_ENABLED":                false,
	"COMMERCE_ORDERS_ENABLED":                    false,
	"VENDOR_READINESS_ORCHESTRATION_ENABLED":     false,
	"VENDOR_TAXONOMY_ENABLED":                    false,
	"VENDOR_TAXONOMY_WRITES_ENABLED":             false,
	"VENDOR_SEARCH_ENABLED":                      false,
	"VENDOR_OWNED_HANDOVER_RELAY_ENABLED":        false,
	// external credentials required
	"VENDOR_CRM_CAMPAIGN_EXECUTION_ENABLED":   true, // Notification
	"VENDOR_CRM_EXPORTS_ENABLED":              true, // Media
	"PAYMENT_WALLET_COMMERCE_ENABLED":         true, // Wallet (finance screens)
	"VENDOR_VERIFICATION_INTEGRATION_ENABLED": true, // Verification (onboarding submit)
	"DISPATCH_COMMERCE_INTEGRATION_ENABLED":   true, // Dispatch
	"HAULING_COMMERCE_INTEGRATION_ENABLED":    true, // Hauling
	"TAXI_GOODS_DELIVERY_ENABLED":             true, // Taxi
	"VENDOR_JIT_PICKUP_ENABLED":               true, // a logistics authority
	"VENDOR_AUTO_AUTHORITY_SELECTION_ENABLED": true, // a logistics authority
	"VENDOR_DISPATCH_HANDOVER_RELAY_ENABLED":  true, // Dispatch
	"VENDOR_HAULING_HANDOVER_RELAY_ENABLED":   true, // Hauling
	"VENDOR_TAXI_HANDOVER_RELAY_ENABLED":      true, // Taxi
}

func appDeployEnv(t *testing.T) {
	t.Helper()
	for key, value := range map[string]string{
		"APP_ENV":                          "production",
		"COMMERCE_GLOBAL_KILL_SWITCH":      "false",
		"VENDOR_AUTH_ACCESS_TOKEN_SECRET":  "a",
		"VENDOR_AUTH_REFRESH_TOKEN_SECRET": "r",
		"VENDOR_AUTH_OTP_SECRET":           "o",
		"VENDOR_SERVICE_SECRETS":           "verification-compliance-service=s",
		"VENDOR_TAXONOMY_INTERNAL_CALLERS": "admin-backoffice-service",
		"VENDOR_SEARCH_INTERNAL_CALLERS":   "customer-service",
		"VENDOR_NOTIFICATION_URL":          "http://notification:8106",
		"VENDOR_NOTIFICATION_SECRET":       "s",
		"VENDOR_MEDIA_URL":                 "http://media:8109",
		"VENDOR_MEDIA_SECRET":              "s",
		"VENDOR_MEDIA_PUBLIC_URL":          "https://api.example.com",
		"VENDOR_WALLET_URL":                "http://wallet:8105",
		"VENDOR_WALLET_SECRET":             "s",
		"VENDOR_VERIFICATION_URL":          "http://verification:8108",
		"VENDOR_VERIFICATION_SECRET":       "s",
		"VENDOR_DISPATCH_URL":              "http://dispatch:8103",
		"VENDOR_DISPATCH_SECRET":           "s",
		"VENDOR_HAULING_URL":               "http://hauling:8104",
		"VENDOR_HAULING_SECRET":            "s",
		"VENDOR_TAXI_URL":                  "http://taxi:8102",
		"VENDOR_TAXI_SECRET":               "s",
	} {
		t.Setenv(key, value)
	}
	for key := range appRequiredFlags {
		t.Setenv(key, "true")
	}
}

func TestAppRequiredFlagSetValidates(t *testing.T) {
	appDeployEnv(t)
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("app-required flag set must validate: %v", err)
	}
	if cfg.Flags.CommerceGlobalKillSwitch {
		t.Fatal("the app set requires the kill switch off")
	}
	if !cfg.Flags.CommerceBulkRFQEnabled || !cfg.Flags.CommerceOrdersEnabled ||
		!cfg.Flags.VendorJITPickupEnabled || !cfg.Flags.PaymentWalletCommerceEnabled {
		t.Fatal("app-required flags were not loaded")
	}
	if !Section32DispatchHandoverEnabled() || !Section32VendorOwnedHandoverEnabled() {
		t.Fatal("handover relay flags were not loaded")
	}
}

// Every app flag must be a key this package actually reads; a typo would be
// silently ignored at deploy time.
func TestAppRequiredFlagsAreKnownKeys(t *testing.T) {
	appDeployEnv(t)
	on := Load()
	for key := range appRequiredFlags {
		if strings.Contains(key, "HANDOVER_RELAY") {
			continue // read directly by section32.go, asserted above
		}
		t.Setenv(key, "false")
		if reflect.DeepEqual(Load().Flags, on.Flags) {
			t.Fatalf("%s is not read by config.Load", key)
		}
		t.Setenv(key, "true")
	}
}

// The deploy helper enables only the logistics authorities whose credentials
// exist; the set must still validate with a single authority.
func TestAppRequiredFlagSetValidatesWithOneAuthority(t *testing.T) {
	appDeployEnv(t)
	for _, key := range []string{"HAULING_COMMERCE_INTEGRATION_ENABLED", "TAXI_GOODS_DELIVERY_ENABLED",
		"VENDOR_HAULING_HANDOVER_RELAY_ENABLED", "VENDOR_TAXI_HANDOVER_RELAY_ENABLED",
		"VENDOR_HAULING_URL", "VENDOR_HAULING_SECRET", "VENDOR_TAXI_URL", "VENDOR_TAXI_SECRET"} {
		t.Setenv(key, "")
	}
	if err := Load().Validate(); err != nil {
		t.Fatalf("dispatch-only app set must validate: %v", err)
	}
}
