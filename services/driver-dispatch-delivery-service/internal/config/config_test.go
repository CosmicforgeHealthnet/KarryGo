package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadReadsRequiredSecretsFromEnvironment(t *testing.T) {
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
	t.Setenv("WALLET_SERVICE_SECRET", "env-wallet-secret")
	t.Setenv("WALLET_SERVICE_URL", "http://wallet:8105/api/v1/payment-wallet")
	t.Setenv("WALLET_SERVICE_SOURCE", "dispatch-delivery-service")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(cfg.AccessTokenSecret) != "env-access-secret" {
		t.Fatal("AccessTokenSecret was not loaded from env")
	}
	if string(cfg.RefreshTokenSecret) != "env-refresh-secret" {
		t.Fatal("RefreshTokenSecret was not loaded from env")
	}
	if string(cfg.OTPSecret) != "env-otp-secret" {
		t.Fatal("OTPSecret was not loaded from env")
	}
	if cfg.WalletServiceURL != "http://wallet:8105/api/v1/payment-wallet" {
		t.Fatalf("WalletServiceURL = %q", cfg.WalletServiceURL)
	}
	if string(cfg.WalletServiceSecret) != "env-wallet-secret" {
		t.Fatal("WalletServiceSecret was not loaded from env")
	}
	if cfg.WalletServiceSource != "dispatch-delivery-service" {
		t.Fatalf("WalletServiceSource = %q", cfg.WalletServiceSource)
	}
}

func TestLoadRequestFeatureDefaults(t *testing.T) {
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
	t.Setenv("WALLET_SERVICE_SECRET", "env-wallet-secret")
	t.Setenv("AVAILABILITY_SERVICE_URL", "")
	t.Setenv("BROADCAST_INITIAL_RADIUS_KM", "")
	t.Setenv("BROADCAST_RADIUS_INCREMENT_KM", "")
	t.Setenv("BROADCAST_MAX_ATTEMPTS", "")
	t.Setenv("BROADCAST_WINDOW_SECONDS", "")
	t.Setenv("WALLET_SERVICE_URL", "")
	t.Setenv("WALLET_SERVICE_SOURCE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AvailabilityServiceURL != "http://localhost:8103" ||
		cfg.BroadcastInitialRadiusKM != 5 ||
		cfg.BroadcastRadiusIncrementKM != 3 ||
		cfg.BroadcastMaxAttempts != 3 ||
		cfg.BroadcastWindow != 30*time.Second ||
		cfg.WalletServiceURL != "http://localhost:8105/api/v1/payment-wallet" ||
		cfg.WalletServiceSource != "dispatch-delivery-service" {
		t.Fatalf("request defaults=%+v", cfg)
	}
}

func TestLoadFailsClearlyWhenRequiredSecretMissing(t *testing.T) {
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")

	_, err := Load()
	if err == nil {
		t.Fatal("expected missing secret error")
	}
	if !strings.Contains(err.Error(), "DISPATCH_RIDER_ACCESS_TOKEN_SECRET") {
		t.Fatalf("error = %q, want missing env var name", err.Error())
	}
}

func TestLoadFailsClearlyWhenWalletServiceSecretMissing(t *testing.T) {
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
	t.Setenv("WALLET_SERVICE_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected missing wallet service secret error")
	}
	if !strings.Contains(err.Error(), "WALLET_SERVICE_SECRET") {
		t.Fatalf("error = %q, want missing env var name", err.Error())
	}
}
