package config

import (
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/redisx"
)

func validConfig() Config {
	return Config{
		AppEnv:                     "development",
		HTTPAddr:                   ":8113",
		LogLevel:                   "info",
		DatabaseURL:                "postgres://user:pass@localhost:5499/fleet_partner_service",
		DatabaseMaxOpenConnections: 10,
		DatabaseMaxIdleConnections: 2,
		DatabaseConnectionMaxLife:  time.Hour,
		Redis:                      redisx.Config{},
		AccessTokenSecret:          []byte("test-secret"),
		MetricsEnabled:             true,
		MetricsAuthToken:           "metrics-token",
		HTTPClientTimeout:          10 * time.Second,
		ShutdownTimeout:            15 * time.Second,
		OutboxPollInterval:         2 * time.Second,
		JobPollInterval:            2 * time.Second,
		JobLeaseDuration:           60 * time.Second,
		JobHeartbeatInterval:       15 * time.Second,
		MaxRequestBodyBytes:        1 << 20,
		Clients:                    map[string]ClientConfig{},

		// Section 8–10 authentication settings. These secrets are mandatory
		// in every environment — there is deliberately no default.
		OTPSecret:             []byte("otp-pepper"),
		SessionSecret:         []byte("session-secret"),
		ContactSecret:         []byte("contact-secret"),
		OTPLength:             6,
		OTPExpiry:             10 * time.Minute,
		OTPMaxAttempts:        5,
		OTPResendCooldown:     60 * time.Second,
		OTPMaxPerHour:         10,
		AccessTokenTTL:        15 * time.Minute,
		RefreshTokenTTL:       720 * time.Hour,
		StepUpTTL:             5 * time.Minute,
		StaffInvitationTTL:    168 * time.Hour,
		ProviderInvitationTTL: 336 * time.Hour,
		DefaultCountryCode:    "234",
		AuthorizationCacheTTL: 30 * time.Second,

		TraceSampleRate:                 1.0,
		ExternalClientMaxRetries:        2,
		ExternalClientRetryBackoffBase:  200 * time.Millisecond,
		ExternalClientRetryBackoffMax:   5 * time.Second,
		CircuitBreakerFailureThreshold:  5,
		CircuitBreakerObservationWindow: 30 * time.Second,
		CircuitBreakerOpenDuration:      30 * time.Second,
		CircuitBreakerHalfOpenProbes:    2,
		CircuitBreakerSuccessThreshold:  2,
		ReadinessOutboxStaleAfter:       10 * time.Minute,
		ReadinessOutboxBacklogCritical:  5000,
		ReadinessJobDeadLetterCritical:  500,
		ReadinessStaleLeaseThreshold:    10 * time.Minute,
	}
}

func TestAuthenticationSecretsAreMandatory(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"FLEET_PARTNER_OTP_HMAC_SECRET":     func(c *Config) { c.OTPSecret = nil },
		"FLEET_PARTNER_SESSION_HASH_SECRET": func(c *Config) { c.SessionSecret = nil },
		"FLEET_PARTNER_CONTACT_HASH_SECRET": func(c *Config) { c.ContactSecret = nil },
	} {
		cfg := validConfig()
		mutate(&cfg)
		assertInvalid(t, cfg, name)
	}
}

func TestOTPBoundsAreEnforced(t *testing.T) {
	cfg := validConfig()
	cfg.OTPLength = 4
	assertInvalid(t, cfg, "FLEET_PARTNER_OTP_LENGTH")

	cfg = validConfig()
	cfg.OTPExpiry = 24 * time.Hour
	assertInvalid(t, cfg, "FLEET_PARTNER_OTP_EXPIRY")

	cfg = validConfig()
	cfg.OTPMaxAttempts = 50
	assertInvalid(t, cfg, "FLEET_PARTNER_OTP_MAX_ATTEMPTS")
}

func TestRefreshTokenMustOutliveAccessToken(t *testing.T) {
	cfg := validConfig()
	cfg.AccessTokenTTL = time.Hour
	cfg.RefreshTokenTTL = time.Minute
	assertInvalid(t, cfg, "FLEET_PARTNER_REFRESH_TOKEN_TTL")
}

func TestValidationErrorNeverLeaksAuthSecrets(t *testing.T) {
	cfg := validConfig()
	cfg.DatabaseURL = ""
	cfg.OTPSecret = []byte("super-secret-pepper")
	cfg.SessionSecret = []byte("super-secret-session")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, secret := range []string{"super-secret-pepper", "super-secret-session"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("validation error leaks an authentication secret: %v", err)
		}
	}
}

func TestValidConfigPasses(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestMissingDatabaseURLFails(t *testing.T) {
	cfg := validConfig()
	cfg.DatabaseURL = ""
	assertInvalid(t, cfg, "FLEET_PARTNER_DATABASE_URL")
}

func TestMissingAccessTokenSecretFails(t *testing.T) {
	cfg := validConfig()
	cfg.AccessTokenSecret = nil
	assertInvalid(t, cfg, "ACCESS_TOKEN_SECRET")
}

func TestMetricsRequireToken(t *testing.T) {
	cfg := validConfig()
	cfg.MetricsAuthToken = ""
	assertInvalid(t, cfg, "METRICS_AUTH_TOKEN")
}

func TestProductionRequiresServiceSecrets(t *testing.T) {
	cfg := validConfig()
	cfg.AppEnv = "production"
	cfg.ServiceSecrets = ""
	assertInvalid(t, cfg, "SERVICE_SECRETS")
}

// TestProductionRequiresAuditChainSecret guards the Section 28 fail-closed
// check: internal/platform/audit/chain.go falls back to a fixed, publicly
// known dev key when FLEET_PARTNER_AUDIT_CHAIN_SECRET is unset (the chain
// stays self-consistent but not confidentially keyed), so production must
// refuse to boot without it. Uses t.Setenv/os.Unsetenv directly because the
// check reads the environment variable itself, not a Config field -- see
// the check's own comment in config.go for why.
func TestProductionRequiresAuditChainSecret(t *testing.T) {
	t.Run("missing in production fails", func(t *testing.T) {
		t.Setenv("FLEET_PARTNER_AUDIT_CHAIN_SECRET", "")
		cfg := validConfig()
		cfg.AppEnv = "production"
		cfg.ServiceSecrets = "driver-taxi-service=secret"
		assertInvalid(t, cfg, "AUDIT_CHAIN_SECRET")
	})

	t.Run("set in production passes", func(t *testing.T) {
		t.Setenv("FLEET_PARTNER_AUDIT_CHAIN_SECRET", "a-real-production-chain-secret")
		cfg := validConfig()
		cfg.AppEnv = "production"
		cfg.ServiceSecrets = "driver-taxi-service=secret"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})

	t.Run("missing outside production is not flagged", func(t *testing.T) {
		t.Setenv("FLEET_PARTNER_AUDIT_CHAIN_SECRET", "")
		cfg := validConfig()
		cfg.AppEnv = "development"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected valid outside production, got %v", err)
		}
	})
}

func TestLeaseMustExceedHeartbeat(t *testing.T) {
	cfg := validConfig()
	cfg.JobLeaseDuration = 10 * time.Second
	cfg.JobHeartbeatInterval = 15 * time.Second
	assertInvalid(t, cfg, "JOB_LEASE_DURATION")
}

func TestInvalidPortFails(t *testing.T) {
	cfg := validConfig()
	cfg.HTTPAddr = ":99999"
	assertInvalid(t, cfg, "HTTP_ADDR")
}

func TestInvalidClientBaseURLFails(t *testing.T) {
	cfg := validConfig()
	cfg.Clients = map[string]ClientConfig{
		"customer-service": {BaseURL: "not a url", Secret: []byte("s")},
	}
	assertInvalid(t, cfg, "customer-service")
}

func TestRedisRequiredNeedsAddr(t *testing.T) {
	cfg := validConfig()
	cfg.RedisRequired = true
	assertInvalid(t, cfg, "REDIS_ADDR")
}

func TestTaxiCannotBeEnabledWithoutProductionContract(t *testing.T) {
	cfg := validConfig()
	cfg.TaxiIntegrationEnabled = true
	assertInvalid(t, cfg, "FLEET_TAXI_INTEGRATION_ENABLED cannot be enabled")
}

func TestPrivateTaxiCannotBeEnabledWithoutProductionContract(t *testing.T) {
	cfg := validConfig()
	cfg.PrivateTaxiExecutionEnabled = true
	assertInvalid(t, cfg, "PRIVATE_TAXI_EXECUTION_ENABLED cannot be enabled")
}

func TestHaulingEnablementRequiresCompleteClient(t *testing.T) {
	cfg := validConfig()
	cfg.HaulingIntegrationEnabled = true
	assertInvalid(t, cfg, "HAULING_SERVICE_BASE_URL")
	cfg.Clients["driver-hauling-service"] = ClientConfig{BaseURL: "https://hauling.internal", Secret: []byte("secret")}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("complete hauling config rejected: %v", err)
	}
}

func TestGlobalKillOverridesIncompleteHaulingConfig(t *testing.T) {
	cfg := validConfig()
	cfg.HaulingIntegrationEnabled = true
	cfg.IntegrationsKillSwitch = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("kill-switched integration should not require live credentials: %v", err)
	}
}

func TestSection16WritesAndWorkersDefaultDisabled(t *testing.T) {
	cfg := validConfig()
	if cfg.DispatchPlanningEnabled || cfg.RoutePlanningEnabled || cfg.RouteOptimizationEnabled || cfg.RoutePublicationEnabled || cfg.ControlTowerEnabled || cfg.TrackingProjectionsEnabled || cfg.GeofencesEnabled || cfg.OperationalAlertsEnabled || cfg.Section16WorkersEnabled {
		t.Fatal("Section 16 capabilities must be opt-in")
	}
}

func TestSection16OptimizationRequiresConfiguredProvider(t *testing.T) {
	cfg := validConfig()
	cfg.RouteOptimizationEnabled = true
	assertInvalid(t, cfg, "ROUTE_OPTIMIZATION_ENABLED")
	cfg.Clients["route-optimization-service"] = ClientConfig{BaseURL: "https://optimizer.internal", Secret: []byte("test-secret")}
	cfg.Section16TrackingFreshAfter = 2 * time.Minute
	cfg.Section16TrackingStaleAfter = 10 * time.Minute
	cfg.Section16TrackingRetention = 24 * time.Hour
	if err := cfg.Validate(); err != nil {
		t.Fatalf("configured provider-neutral optimizer rejected: %v", err)
	}
}

func TestSection16PublicationAndWorkerDependenciesFailClosed(t *testing.T) {
	cfg := validConfig()
	cfg.RoutePublicationEnabled = true
	cfg.Section16TrackingFreshAfter = 2 * time.Minute
	cfg.Section16TrackingStaleAfter = 10 * time.Minute
	cfg.Section16TrackingRetention = 24 * time.Hour
	assertInvalid(t, cfg, "ROUTE_PUBLICATION_ENABLED")

	cfg = validConfig()
	cfg.Section16WorkersEnabled = true
	cfg.Section16TrackingFreshAfter = 2 * time.Minute
	cfg.Section16TrackingStaleAfter = 10 * time.Minute
	cfg.Section16TrackingRetention = 24 * time.Hour
	assertInvalid(t, cfg, "FLEET_SECTION16_WORKERS_ENABLED")
}

func TestSection17And18CapabilitiesDefaultDisabled(t *testing.T) {
	cfg := validConfig()
	if cfg.WorkforceEnabled || cfg.ShiftsEnabled || cfg.LeaveEnabled || cfg.QualificationsEnabled || cfg.TrainingEnabled || cfg.PerformanceSnapshotsEnabled || cfg.AssetsEnabled || cfg.AssetAssignmentsEnabled || cfg.InspectionsEnabled || cfg.MaintenanceEnabled || cfg.BreakdownsEnabled || cfg.PartsInventoryEnabled || cfg.FuelRecordsEnabled || cfg.Section17WorkersEnabled || cfg.Section18WorkersEnabled {
		t.Fatal("Sections 17 and 18 capabilities must be opt-in")
	}
}

func TestSection17And18WorkersRequireSharedWorkerRuntime(t *testing.T) {
	cfg := validConfig()
	cfg.Section17WorkersEnabled = true
	assertInvalid(t, cfg, "SECTION17_WORKERS_ENABLED")

	cfg = validConfig()
	cfg.Section18WorkersEnabled = true
	assertInvalid(t, cfg, "SECTION18_WORKERS_ENABLED")
}

func TestSections19To21CapabilitiesAndWorkersDefaultDisabled(t *testing.T) {
	cfg := validConfig()
	if cfg.ComplianceEnabled || cfg.SafetyReportsEnabled || cfg.IncidentsEnabled || cfg.ProofPoliciesEnabled || cfg.ProofRecordsEnabled || cfg.RestrictionPropagationEnabled ||
		cfg.SpecializedLogisticsEnabled || cfg.CourierLastMileEnabled || cfg.TaxiPassengerEnabled || cfg.TruckingHaulageEnabled || cfg.HeavySpecializedEnabled || cfg.FreightForwardingEnabled || cfg.ColdChainEnabled || cfg.MedicalSensitiveEnabled || cfg.AgriculturalFoodEnabled || cfg.MovingRelocationEnabled || cfg.TowingRecoveryEnabled ||
		cfg.WarehousesEnabled || cfg.WarehouseInventoryEnabled || cfg.FulfilmentEnabled || cfg.DockOperationsEnabled || cfg.YardOperationsEnabled || cfg.TerminalTransfersEnabled || cfg.ReturnsEnabled ||
		cfg.Section19WorkersEnabled || cfg.Section20WorkersEnabled || cfg.Section21WorkersEnabled {
		t.Fatal("Sections 19-21 writes, modules and workers must be opt-in")
	}
}

func TestProofRecordsRequireHashSecret(t *testing.T) {
	cfg := validConfig()
	cfg.ProofRecordsEnabled = true
	assertInvalid(t, cfg, "FLEET_PROOF_HMAC_SECRET")
	cfg.ProofHMACSecret = []byte("proof-test-secret")
	cfg.ProofPoliciesEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("proof records with a hash secret rejected: %v", err)
	}
}

func TestSections19To21WorkersRequireSharedWorkerRuntime(t *testing.T) {
	for _, enable := range []func(*Config){
		func(c *Config) { c.Section19WorkersEnabled = true },
		func(c *Config) { c.Section20WorkersEnabled = true },
		func(c *Config) { c.Section21WorkersEnabled = true },
	} {
		cfg := validConfig()
		enable(&cfg)
		assertInvalid(t, cfg, "FLEET_PARTNER_WORKER_ENABLED")
	}
}

func TestWarehouseCapabilityDependenciesFailClosed(t *testing.T) {
	cfg := validConfig()
	cfg.WarehouseInventoryEnabled = true
	assertInvalid(t, cfg, "WAREHOUSES_ENABLED")
	cfg = validConfig()
	cfg.FulfilmentEnabled = true
	assertInvalid(t, cfg, "WAREHOUSE_INVENTORY_ENABLED")
	cfg = validConfig()
	cfg.DockOperationsEnabled = true
	assertInvalid(t, cfg, "WAREHOUSES_ENABLED")
}

func TestRestrictionPropagationRequiresBothAuthorities(t *testing.T) {
	cfg := validConfig()
	cfg.RestrictionPropagationEnabled = true
	assertInvalid(t, cfg, "driver-dispatch-delivery-service")
	cfg.Clients["driver-dispatch-delivery-service"] = ClientConfig{BaseURL: "https://dispatch.internal", Secret: []byte("dispatch-secret")}
	assertInvalid(t, cfg, "driver-hauling-service")
	cfg.Clients["driver-hauling-service"] = ClientConfig{BaseURL: "https://hauling.internal", Secret: []byte("hauling-secret")}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("complete restriction authorities rejected: %v", err)
	}
}

func TestTaxiDataFoundationDoesNotEnableTaxiExecution(t *testing.T) {
	cfg := validConfig()
	cfg.SpecializedLogisticsEnabled = true
	cfg.TaxiPassengerEnabled = true
	if cfg.TaxiIntegrationEnabled || cfg.PrivateTaxiExecutionEnabled {
		t.Fatal("taxi data capability enabled an execution integration")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("safe taxi data foundations should be configurable: %v", err)
	}
}

func TestSections22And23CapabilitiesDefaultDisabled(t *testing.T) {
	cfg := validConfig()
	if cfg.InvoicesEnabled || cfg.ReceivablesEnabled || cfg.ExpensesEnabled || cfg.BudgetsEnabled || cfg.CompensationEnabled || cfg.PayrollEnabled || cfg.SubcontractorSettlementsEnabled || cfg.ClaimsEnabled || cfg.FinancePostingEnabled || cfg.CommunicationsEnabled || cfg.NotificationRulesEnabled || cfg.SupportProjectionsEnabled || cfg.QualityCasesEnabled || cfg.SLAMeasurementsEnabled || cfg.EscalationsEnabled || cfg.Section22WorkersEnabled || cfg.Section23WorkersEnabled {
		t.Fatal("Sections 22 and 23 capabilities and workers must be opt-in")
	}
}

func TestSection55IntegrationCapabilitiesDefaultDisabled(t *testing.T) {
	cfg := validConfig()
	if cfg.OrganizationAPIClientsEnabled || cfg.WebhooksEnabled || cfg.WebhookDeliveryWorkerEnabled {
		t.Fatal("Section 55 API client and webhook capabilities must be opt-in")
	}
}

func TestSection55IntegrationDependenciesFailClosed(t *testing.T) {
	cfg := validConfig()
	cfg.OrganizationAPIClientsEnabled = true
	assertInvalid(t, cfg, "FLEET_PARTNER_API_CLIENT_HASH_KEY")

	cfg = validConfig()
	cfg.WebhooksEnabled = true
	cfg.WebhookDeliveryTimeout = 10 * time.Second
	cfg.WebhookReplayWindow = 5 * time.Minute
	cfg.WebhookRotationOverlapMax = time.Hour
	cfg.WebhookDisableAfterFailures = 5
	cfg.WebhookDeliveryMaxAttempts = 5
	cfg.WebhookFanoutInterval = time.Second
	cfg.WebhookFanoutBatchSize = 25
	assertInvalid(t, cfg, "FLEET_PARTNER_WEBHOOK_SECRET_ENCRYPTION_KEY")

	cfg = validConfig()
	cfg.WebhookDeliveryWorkerEnabled = true
	assertInvalid(t, cfg, "FLEET_PARTNER_WEBHOOKS_ENABLED")

	cfg = validConfig()
	cfg.AppEnv = "production"
	cfg.WebhookAllowHTTP = true
	assertInvalid(t, cfg, "FLEET_PARTNER_WEBHOOK_ALLOW_HTTP")
}

func TestSections22And23DependenciesFailClosed(t *testing.T) {
	cfg := validConfig()
	cfg.ReceivablesEnabled = true
	assertInvalid(t, cfg, "INVOICES_ENABLED")
	cfg = validConfig()
	cfg.PayrollEnabled = true
	assertInvalid(t, cfg, "COMPENSATION_ENABLED")
	cfg = validConfig()
	cfg.Section22WorkersEnabled = true
	assertInvalid(t, cfg, "FLEET_PARTNER_WORKER_ENABLED")
	cfg = validConfig()
	cfg.Section23WorkersEnabled = true
	assertInvalid(t, cfg, "FLEET_PARTNER_WORKER_ENABLED")
	cfg = validConfig()
	cfg.FinancePostingEnabled = true
	cfg.CompensationEnabled = true
	cfg.PayrollEnabled = true
	assertInvalid(t, cfg, "payment-wallet-service")
	cfg = validConfig()
	cfg.FinancePostingEnabled = true
	cfg.ExpensesEnabled = true
	assertInvalid(t, cfg, "accounting-service")
	cfg = validConfig()
	cfg.CommunicationsEnabled = true
	assertInvalid(t, cfg, "notification-service")
	cfg = validConfig()
	cfg.SupportProjectionsEnabled = true
	assertInvalid(t, cfg, "support-dispute-service")
	cfg = validConfig()
	cfg.SLAMeasurementsEnabled = true
	assertInvalid(t, cfg, "SLA_ENABLED")
}

func TestValidationErrorNeverContainsSecrets(t *testing.T) {
	cfg := validConfig()
	cfg.DatabaseURL = "" // trigger a failure while secrets are set
	cfg.AccessTokenSecret = []byte("super-secret-value")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "super-secret-value") || strings.Contains(err.Error(), "pass@") {
		t.Fatalf("validation error leaks secrets: %v", err)
	}
}

func assertInvalid(t *testing.T, cfg Config, wantFragment string) {
	t.Helper()
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("config should be invalid (%s)", wantFragment)
	}
	if !strings.Contains(err.Error(), wantFragment) {
		t.Fatalf("error %q should mention %s", err, wantFragment)
	}
}
