package config

import (
	"strings"
	"testing"
	"time"
)

// validCfg is a minimal config that passes Validate, so each test can mutate
// exactly the one field it is about.
func validCfg() Config {
	return Config{
		AppEnv:                   "development",
		ProviderTokenSecret:      []byte("a"),
		ProviderRefreshSecret:    []byte("b"),
		ProviderOTPSecret:        []byte("c"),
		CustomerTokenSecret:      []byte("d"),
		BookingMatchTimeout:      30,
		BookingSearchWindow:      60,
		MatchMaxRadiusKm:         10,
		MatchExpandedRadiusKm:    20,
		MatchPollIntervalSeconds: 2,
		PickupVerifyRateWindow:   60 * time.Second,
		PickupVerifyMaxAttempts:  5,
		TelemetryMaxSpeedKph:     180,
		TelemetryStaleSeconds:    120,
		ReviewRevealWindow:       48 * time.Hour,
	}
}

func TestValidateRequiresAllTokenSecrets(t *testing.T) {
	// There are no development fallbacks for these on purpose: a service that
	// boots with an empty signing secret issues tokens nobody else accepts.
	for _, name := range []string{
		"TAXI_PROVIDER_TOKEN_SECRET",
		"TAXI_PROVIDER_REFRESH_SECRET",
		"TAXI_PROVIDER_OTP_SECRET",
		"TAXI_CUSTOMER_TOKEN_SECRET",
	} {
		cfg := validCfg()
		switch name {
		case "TAXI_PROVIDER_TOKEN_SECRET":
			cfg.ProviderTokenSecret = nil
		case "TAXI_PROVIDER_REFRESH_SECRET":
			cfg.ProviderRefreshSecret = nil
		case "TAXI_PROVIDER_OTP_SECRET":
			cfg.ProviderOTPSecret = nil
		case "TAXI_CUSTOMER_TOKEN_SECRET":
			cfg.CustomerTokenSecret = nil
		}
		err := cfg.Validate()
		if err == nil {
			t.Fatalf("%s: expected an error when the secret is empty", name)
		}
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("%s: error should name the missing variable, got %q", name, err)
		}
	}
}

func TestValidateRejectsPrefixedInternalURLs(t *testing.T) {
	// walletclient and the notification client append their own /api/v1 paths.
	// A prefixed base URL 404s every internal call, and because those callers
	// are fire-and-forget the failure is otherwise invisible.
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantVar string
	}{
		{
			name:    "payment with path",
			mutate:  func(c *Config) { c.PaymentURL = "http://localhost:8105/api/v1/payment-wallet" },
			wantVar: "TAXI_PAYMENT_URL",
		},
		{
			name:    "notification with path",
			mutate:  func(c *Config) { c.NotificationURL = "http://localhost:8106/api/v1/notifications" },
			wantVar: "TAXI_NOTIFICATION_URL",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validCfg()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected a prefixed internal URL to be rejected")
			}
			if !strings.Contains(err.Error(), tc.wantVar) {
				t.Fatalf("error should name %s, got %q", tc.wantVar, err)
			}
		})
	}
}

func TestValidateAcceptsBareOrigins(t *testing.T) {
	cfg := validCfg()
	cfg.PaymentURL = "http://localhost:8105"
	cfg.NotificationURL = "http://localhost:8106/" // a lone trailing slash is still bare
	if err := cfg.Validate(); err != nil {
		t.Fatalf("bare origins should be accepted, got %v", err)
	}
}

func TestValidateProductionRequiresPaymentAndNotification(t *testing.T) {
	// Locally a nil payment/notification client makes every such step a no-op.
	// Production must never inherit that silently.
	cfg := validCfg()
	cfg.AppEnv = "production"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected production to require payment and notification config")
	}
	for _, want := range []string{"TAXI_PAYMENT_URL", "TAXI_NOTIFICATION_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should name %s, got %q", want, err)
		}
	}

	cfg.PaymentURL = "http://payments"
	cfg.PaymentSecret = []byte("s")
	cfg.NotificationURL = "http://notifications"
	cfg.NotificationSecret = []byte("s")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("fully configured production should validate, got %v", err)
	}
}

func TestValidateRejectsSearchWindowShorterThanMatchTimeout(t *testing.T) {
	// A window shorter than one offer means the first driver's timer outlives
	// the search itself, and no second driver is ever tried.
	cfg := validCfg()
	cfg.BookingSearchWindow = 10
	cfg.BookingMatchTimeout = 30
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a search window shorter than the match timeout to be rejected")
	}
}

func TestValidateRejectsExpandedRadiusSmallerThanBase(t *testing.T) {
	// Wave 2 is meant to WIDEN the search, never narrow it.
	cfg := validCfg()
	cfg.MatchMaxRadiusKm = 10
	cfg.MatchExpandedRadiusKm = 5
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an expanded radius smaller than the base radius to be rejected")
	}
}

func TestValidateRejectsZeroPollInterval(t *testing.T) {
	cfg := validCfg()
	cfg.MatchPollIntervalSeconds = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a zero poll interval to be rejected -- it would spin the matcher in a tight loop")
	}
}

func TestValidateRejectsZeroPickupVerifyRateWindow(t *testing.T) {
	cfg := validCfg()
	cfg.PickupVerifyRateWindow = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a zero pickup-verify rate window to be rejected")
	}
}

func TestValidateRejectsZeroPickupVerifyMaxAttempts(t *testing.T) {
	cfg := validCfg()
	cfg.PickupVerifyMaxAttempts = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a zero pickup-verify max attempts to be rejected")
	}
}

func TestValidateRejectsNegativeScheduleLeadMinutes(t *testing.T) {
	cfg := validCfg()
	cfg.ScheduleLeadMinutes = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a negative schedule lead time to be rejected")
	}
}

func TestValidateTaxiGoodsEnabledRequiresEveryAuthorityDependency(t *testing.T) {
	cfg := validCfg()
	cfg.TaxiGoodsDeliveryEnabled = true
	err := cfg.Validate()
	if err == nil {
		t.Fatal("enabled Taxi Goods without dependencies validated")
	}
	for _, name := range []string{"TAXI_GOODS_PRICING", "TAXI_GOODS_HANDOVER_SECRET", "TAXI_MEDIA_URL", "TAXI_PAYMENT_URL", "TAXI_SERVICE_SECRETS", "TAXI_VENDOR_URL"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("validation did not name %s: %v", name, err)
		}
	}

	cfg.TaxiGoodsPricingJSON = `{"base_minor":1000,"minimum_minor":1000,"currency":"NGN","version":"goods-v1"}`
	cfg.TaxiGoodsHandoverSecret = []byte("01234567890123456789012345678901")
	cfg.TaxiGoodsQuoteTTL = 15 * time.Minute
	cfg.VendorURL = "http://vendor-service:8080"
	cfg.VendorSecret = []byte("vendor-outbound-secret")
	cfg.MediaURL, cfg.MediaToken = "http://media", "token"
	cfg.PaymentURL, cfg.PaymentSecret = "http://wallet", []byte("secret")
	cfg.ServiceSecrets = "vendor-service=vendor-secret"
	if err = cfg.Validate(); err != nil {
		t.Fatalf("fully configured Taxi Goods rejected: %v", err)
	}
}

func TestValidateAllowsZeroScheduleLeadMinutes(t *testing.T) {
	cfg := validCfg()
	cfg.ScheduleLeadMinutes = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("zero schedule lead time should be valid (dispatch exactly at pickup time), got: %v", err)
	}
}

func TestValidateRejectsZeroTelemetryMaxSpeed(t *testing.T) {
	cfg := validCfg()
	cfg.TelemetryMaxSpeedKph = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a zero telemetry max speed to be rejected")
	}
}

func TestValidateRejectsZeroTelemetryStaleSeconds(t *testing.T) {
	cfg := validCfg()
	cfg.TelemetryStaleSeconds = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a zero telemetry stale threshold to be rejected")
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg := Load()

	if cfg.HTTPAddr != ":8102" {
		t.Fatalf("taxi listens on :8102, got %q", cfg.HTTPAddr)
	}
	// Pinned deliberately: a longer TTL leaves force-quit drivers in the online
	// set where they still consume match offers.
	if cfg.ProviderOnlineTTL != 90 {
		t.Fatalf("ProviderOnlineTTL default must be 90, got %d", cfg.ProviderOnlineTTL)
	}
	if cfg.MatchMaxRadiusKm != 10 {
		t.Fatalf("MatchMaxRadiusKm default must be 10, got %v", cfg.MatchMaxRadiusKm)
	}
	if cfg.MatchExpandedRadiusKm != 20 {
		t.Fatalf("MatchExpandedRadiusKm default must be 20, got %v", cfg.MatchExpandedRadiusKm)
	}
	if cfg.MatchPollIntervalSeconds != 2 {
		t.Fatalf("MatchPollIntervalSeconds default must be 2, got %d", cfg.MatchPollIntervalSeconds)
	}
	if cfg.PickupVerifyRateWindow != 60*time.Second {
		t.Fatalf("PickupVerifyRateWindow default must be 60s, got %v", cfg.PickupVerifyRateWindow)
	}
	if cfg.PickupVerifyMaxAttempts != 5 {
		t.Fatalf("PickupVerifyMaxAttempts default must be 5, got %d", cfg.PickupVerifyMaxAttempts)
	}
	if cfg.ScheduleLeadMinutes != 10 {
		t.Fatalf("ScheduleLeadMinutes default must be 10, got %d", cfg.ScheduleLeadMinutes)
	}
	if cfg.TelemetryMaxSpeedKph != 180 {
		t.Fatalf("TelemetryMaxSpeedKph default must be 180, got %v", cfg.TelemetryMaxSpeedKph)
	}
	if cfg.TelemetryStaleSeconds != 120 {
		t.Fatalf("TelemetryStaleSeconds default must be 120, got %d", cfg.TelemetryStaleSeconds)
	}
	if cfg.ReviewRevealWindow != 48*time.Hour {
		t.Fatalf("ReviewRevealWindow default must be 48h, got %v", cfg.ReviewRevealWindow)
	}
	if cfg.OTPDebug {
		t.Fatal("OTPDebug must default to false so debug_otp never leaks by omission")
	}
	if cfg.TaxiGoodsDeliveryEnabled {
		t.Fatal("TAXI_GOODS_DELIVERY_ENABLED must default to false")
	}
}
