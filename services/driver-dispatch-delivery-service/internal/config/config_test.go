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

func TestLoadServiceabilityDefaultsAreSafe(t *testing.T) {
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
	t.Setenv("WALLET_SERVICE_SECRET", "env-wallet-secret")
	t.Setenv("DISPATCH_ROUTE_SERVICEABILITY_ENABLED", "")
	t.Setenv("DISPATCH_PACKAGE_POLICY_ENABLED", "")
	t.Setenv("DISPATCH_SERVICEABILITY_DEGRADED_MODE_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RouteServiceabilityEnabled || cfg.PackagePolicyEnabled || cfg.ServiceabilityDegradedMode {
		t.Fatalf("new flow must default disabled: %+v", cfg)
	}
	if cfg.ServiceabilityTTL != 15*time.Minute || cfg.ServiceabilityMaxRouteKM != 60 || cfg.ServiceZoneRadiusKM != 0 {
		t.Fatalf("unexpected serviceability defaults: %+v", cfg)
	}
}

func TestLoadServiceabilityFlags(t *testing.T) {
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
	t.Setenv("WALLET_SERVICE_SECRET", "env-wallet-secret")
	t.Setenv("DISPATCH_ROUTE_SERVICEABILITY_ENABLED", "true")
	t.Setenv("DISPATCH_PACKAGE_POLICY_ENABLED", "true")
	t.Setenv("DISPATCH_SERVICEABILITY_DEGRADED_MODE_ENABLED", "true")
	t.Setenv("DISPATCH_SERVICE_ZONE_RADIUS_KM", "25")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RouteServiceabilityEnabled || !cfg.PackagePolicyEnabled || !cfg.ServiceabilityDegradedMode || cfg.ServiceZoneRadiusKM != 25 {
		t.Fatalf("serviceability flags not loaded: %+v", cfg)
	}
}

func TestLoadAssignmentAndPlanningDefaultsAreSafe(t *testing.T) {
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
	t.Setenv("WALLET_SERVICE_SECRET", "env-wallet-secret")
	for _, key := range []string{"DISPATCH_SEQUENTIAL_MATCHING_ENABLED", "DISPATCH_SEQUENTIAL_ASSIGNMENT_ENABLED", "DISPATCH_QUEUE_ENABLED", "DISPATCH_ORGANIZATION_ASSIGNMENT_ENABLED", "DISPATCH_ADMIN_ASSISTED_ASSIGNMENT_ENABLED", "DISPATCH_MULTI_STOP_MISSIONS_ENABLED", "DISPATCH_MULTI_VENDOR_PLANNING_ENABLED"} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SequentialAssignmentEnabled || cfg.DispatchQueueEnabled || cfg.OrganizationAssignmentEnabled || cfg.AdminAssistedAssignmentEnabled || cfg.MultiVendorPlanningEnabled {
		t.Fatalf("new assignment/planning flows must default disabled: %+v", cfg)
	}
	if cfg.AssignmentOfferWindow != 75*time.Second || cfg.AssignmentWorkerInterval != 5*time.Second {
		t.Fatalf("unexpected timing defaults: %+v", cfg)
	}
	if cfg.AssignmentQueueRetryInterval != 90*time.Second || cfg.PlanningMaxWeightGrams != 25000 || cfg.PlanningMaxVolumeML != 120000 || cfg.PlanningCapacityPolicyVersion == "" {
		t.Fatalf("unexpected queue/capacity defaults: %+v", cfg)
	}
	if cfg.RecoveryWorkerInterval != 10*time.Second || cfg.ExternalOperationRetryLimit != 10 || cfg.ExternalOperationBackoffBase != 5*time.Second || cfg.ExternalOperationBackoffMax != 15*time.Minute || cfg.ExternalOperationLease != time.Minute || cfg.ExternalServiceTimeout != 10*time.Second {
		t.Fatalf("unexpected reliability defaults: %+v", cfg)
	}
}

func TestLoadCanonicalAssignmentFlags(t *testing.T) {
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
	t.Setenv("WALLET_SERVICE_SECRET", "env-wallet-secret")
	t.Setenv("DISPATCH_SEQUENTIAL_MATCHING_ENABLED", "true")
	t.Setenv("DISPATCH_ASSIGNMENT_ROLLOUT_SERVICE_TIERS", "express")
	t.Setenv("DISPATCH_QUEUE_ENABLED", "true")
	t.Setenv("DISPATCH_MULTI_STOP_MISSIONS_ENABLED", "true")
	t.Setenv("DISPATCH_HANDOVER_CHALLENGE_ENABLED", "true")
	t.Setenv("DISPATCH_PICKUP_EVIDENCE_GATE_ENABLED", "true")
	t.Setenv("DISPATCH_DELIVERY_EVIDENCE_GATE_ENABLED", "true")
	t.Setenv("DISPATCH_HANDOVER_ROLLOUT_SOURCE_TYPES", "VENDOR_COMMERCE")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SequentialAssignmentEnabled || !cfg.DispatchQueueEnabled || !cfg.MultiVendorPlanningEnabled {
		t.Fatalf("canonical flags not loaded: %+v", cfg)
	}
}

func TestLoadRejectsUnsafeRolloutDependencies(t *testing.T) {
	base := func(t *testing.T) {
		t.Helper()
		t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
		t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
		t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
		t.Setenv("WALLET_SERVICE_SECRET", "env-wallet-secret")
	}
	t.Run("evidence requires handover", func(t *testing.T) {
		base(t)
		t.Setenv("DISPATCH_PICKUP_EVIDENCE_GATE_ENABLED", "true")
		if _, err := Load(); err == nil {
			t.Fatal("evidence gate accepted without secure handover")
		}
	})
	t.Run("handover requires server cohort", func(t *testing.T) {
		base(t)
		t.Setenv("DISPATCH_HANDOVER_CHALLENGE_ENABLED", "true")
		if _, err := Load(); err == nil {
			t.Fatal("handover accepted without a server-owned cohort")
		}
	})
	t.Run("multi stop requires protected vendor cohort", func(t *testing.T) {
		base(t)
		t.Setenv("DISPATCH_MULTI_STOP_MISSIONS_ENABLED", "true")
		t.Setenv("DISPATCH_HANDOVER_CHALLENGE_ENABLED", "true")
		t.Setenv("DISPATCH_PICKUP_EVIDENCE_GATE_ENABLED", "true")
		t.Setenv("DISPATCH_DELIVERY_EVIDENCE_GATE_ENABLED", "true")
		t.Setenv("DISPATCH_HANDOVER_ROLLOUT_SOURCE_TYPES", "CUSTOMER")
		if _, err := Load(); err == nil {
			t.Fatal("multi-stop accepted without the protected Vendor Commerce cohort")
		}
	})
	t.Run("sequential requires controlled tier cohort", func(t *testing.T) {
		base(t)
		t.Setenv("DISPATCH_SEQUENTIAL_MATCHING_ENABLED", "true")
		if _, err := Load(); err == nil {
			t.Fatal("sequential matching accepted without a controlled service-tier cohort")
		}
	})
}

func TestLoadSecureHandoverDefaultsAreSafe(t *testing.T) {
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
	t.Setenv("WALLET_SERVICE_SECRET", "env-wallet-secret")
	for _, key := range []string{"DISPATCH_HANDOVER_CHALLENGE_ENABLED", "DISPATCH_PICKUP_EVIDENCE_GATE_ENABLED", "DISPATCH_DELIVERY_EVIDENCE_GATE_ENABLED", "DISPATCH_HANDOVER_QR_ENABLED", "DISPATCH_VENDOR_OWNED_HANDOVER_ENABLED", "DELIVERY_HANDOVER_SMS_ALLOWED"} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HandoverChallengeEnabled || cfg.PickupEvidenceGateEnabled || cfg.DeliveryEvidenceGateEnabled || cfg.HandoverQREnabled || cfg.HandoverSMSAllowed || cfg.VendorOwnedHandoverEnabled {
		t.Fatalf("secure handover must default disabled: %+v", cfg)
	}
	if cfg.HandoverChallengeTTL != 15*time.Minute || cfg.HandoverCaptureSessionTTL != 10*time.Minute || cfg.HandoverResendCooldown != 45*time.Second || cfg.HandoverWorkerInterval != 30*time.Second {
		t.Fatalf("secure handover timing defaults: %+v", cfg)
	}
	if cfg.HandoverMaxAttempts != 5 || cfg.HandoverMaxResends != 5 || cfg.HandoverPolicyVersion == "" {
		t.Fatalf("secure handover policy defaults: %+v", cfg)
	}
}

func TestLoadRejectsWeakSecureHandoverSecretWhenEnabled(t *testing.T) {
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
	t.Setenv("WALLET_SERVICE_SECRET", "env-wallet-secret")
	t.Setenv("DISPATCH_HANDOVER_CHALLENGE_ENABLED", "true")
	t.Setenv("HANDOVER_CODE_SECRET", "short")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "HANDOVER_CODE_SECRET") {
		t.Fatalf("weak handover secret error = %v", err)
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
