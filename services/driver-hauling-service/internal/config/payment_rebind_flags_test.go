package config

import (
	"os"
	"strings"
	"testing"
)

// The payment rebind worker is on by default (it only runs where recovery and
// Wallet are configured) and can be switched off explicitly.
func TestPaymentRebindWorkerDefaultsOnAndCanBeSwitchedOff(t *testing.T) {
	os.Unsetenv("HAULING_RECOVERY_PAYMENT_REBIND_WORKER_ENABLED")
	os.Unsetenv("HAULING_RECOVERY_PAYMENT_REBIND_MAX_ATTEMPTS")
	cfg := Load()
	if !cfg.RecoveryPaymentRebindWorkerEnabled {
		t.Fatal("the payment rebind worker defaults off")
	}
	if cfg.RecoveryPaymentRebindMaxAttempts < 1 {
		t.Fatalf("max attempts default %d is not usable", cfg.RecoveryPaymentRebindMaxAttempts)
	}
	withEnv(t, map[string]string{"HAULING_RECOVERY_PAYMENT_REBIND_WORKER_ENABLED": "false"})
	if Load().RecoveryPaymentRebindWorkerEnabled {
		t.Fatal("the payment rebind worker cannot be switched off")
	}
}

// A worker that could never attempt Wallet would park every obligation.
func TestPaymentRebindWorkerRejectsZeroAttempts(t *testing.T) {
	withEnv(t, map[string]string{
		"HAULING_CAPABILITY_MATCHING_ENABLED": "true", "HAULING_RECOVERY_ENABLED": "true",
		"HAULING_PAYMENT_URL": "http://wallet", "HAULING_PAYMENT_SECRET": "s",
		"HAULING_RECOVERY_PAYMENT_REBIND_MAX_ATTEMPTS": "0",
	})
	err := Load().Validate()
	if err == nil || !strings.Contains(err.Error(), "HAULING_RECOVERY_PAYMENT_REBIND_MAX_ATTEMPTS") {
		t.Fatalf("zero attempts accepted or not named: %v", err)
	}
}

// Defaults stay valid: the worker being on does not demand recovery or Wallet.
func TestPaymentRebindWorkerDefaultIsValidWithoutRecovery(t *testing.T) {
	withEnv(t, map[string]string{"HAULING_RECOVERY_ENABLED": "false", "HAULING_PAYMENT_URL": "", "HAULING_PAYMENT_SECRET": ""})
	if err := Load().Validate(); err != nil && strings.Contains(err.Error(), "PAYMENT_REBIND") {
		t.Fatalf("default worker setting rejected: %v", err)
	}
}
