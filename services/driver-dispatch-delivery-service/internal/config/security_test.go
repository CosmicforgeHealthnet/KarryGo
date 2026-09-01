package config

import "testing"

func TestHandoverKeyMustBeSeparateAndProductionSafe(t *testing.T) {
	t.Setenv("WALLET_SERVICE_SECRET", "test-wallet-key")
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "test-access")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "test-refresh")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "test-auth-otp-key-long-enough")
	t.Setenv("DISPATCH_HANDOVER_CHALLENGE_ENABLED", "true")
	for _, key := range []string{"test-auth-otp-key-long-enough", "development-handover-code-secret"} {
		t.Setenv("APP_ENV", "production")
		t.Setenv("HANDOVER_CODE_SECRET", key)
		if _, err := Load(); err == nil {
			t.Fatal("unsafe production handover key accepted")
		}
	}
	t.Setenv("HANDOVER_CODE_SECRET", "separate-test-handover-key")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
