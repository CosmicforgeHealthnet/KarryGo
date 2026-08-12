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
