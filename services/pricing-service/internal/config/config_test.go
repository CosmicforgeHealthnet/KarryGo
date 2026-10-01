package config

import (
	"strings"
	"testing"
)

func TestDefaultsAreSafeAndValid(t *testing.T) {
	cfg := Load()

	if cfg.HTTPAddr != ":8115" {
		t.Errorf("HTTPAddr = %q, want :8115", cfg.HTTPAddr)
	}
	if !strings.Contains(cfg.DatabaseURL, "localhost:5451/pricing_service") {
		t.Errorf("DatabaseURL = %q, want the local pricing_service database", cfg.DatabaseURL)
	}
	if cfg.Migration {
		t.Error("migrations must not run by default")
	}
	if !cfg.FourEyeRequired {
		t.Error("four-eye approval must default to ON; one person must not be able to change prices alone")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the development defaults must validate: %v", err)
	}
}

func TestNoCommercialValuesLiveInConfiguration(t *testing.T) {
	// Rates, minimum fares, commission and caps are versioned policy DATA with an
	// approver's name against them. If any of them could be set by an environment
	// variable, a price change could bypass approval entirely, which is the
	// central rule this service exists to enforce.
	cfg := Load()
	forbidden := []string{"Rate", "Fare", "Commission", "Multiplier", "Surcharge", "BPS", "Kobo"}

	// Reflectively scanning the struct would be brittle; naming the fields is
	// clearer and fails loudly when someone adds one.
	fieldNames := []string{
		"AppEnv", "HTTPAddr", "Migration", "DatabaseURL", "ServiceSecrets",
		"QuoteCallers", "AdminCallers", "FourEyeRequired", "MaxRouteAge",
		"PolicyCacheRefresh", "QuoteRetention", "OutboxRelayURL",
		"OutboxRelaySecret", "Workers",
	}
	for _, field := range fieldNames {
		for _, word := range forbidden {
			if strings.Contains(field, word) {
				t.Errorf("config field %q looks like a commercial value (%q); rates belong in versioned policy data, not the environment", field, word)
			}
		}
	}
	_ = cfg
}

func TestProductionRefusesToRunWithoutItsControls(t *testing.T) {
	base := Load()
	base.AppEnv = "production"

	t.Run("four-eye approval cannot be disabled", func(t *testing.T) {
		cfg := base
		cfg.FourEyeRequired = false
		cfg.ServiceSecrets = map[string][]byte{"taxi-service": []byte("s")}
		cfg.QuoteCallers = []string{"taxi-service"}
		cfg.AdminCallers = nil

		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "PRICING_FOUR_EYE_REQUIRED") {
			t.Fatalf("error = %v, want a complaint about four-eye approval", err)
		}
	})

	t.Run("service secrets are required", func(t *testing.T) {
		cfg := base
		cfg.ServiceSecrets = nil
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "PRICING_SERVICE_SECRETS") {
			t.Fatalf("error = %v, want a complaint about missing secrets", err)
		}
	})
}

func TestAnAllowedCallerWithoutASecretIsAMisconfiguration(t *testing.T) {
	cfg := Load()
	cfg.ServiceSecrets = map[string][]byte{"taxi-service": []byte("secret")}
	cfg.QuoteCallers = []string{"taxi-service", "dispatch-delivery-service"}
	cfg.AdminCallers = nil

	err := cfg.Validate()
	if err == nil {
		t.Fatal("a caller that is allowed but has no secret can never succeed; it must be reported")
	}
	if !strings.Contains(err.Error(), "dispatch-delivery-service") {
		t.Fatalf("error should name the caller, got: %v", err)
	}
}

func TestOutboxRelayCredentialsMustBePaired(t *testing.T) {
	cfg := Load()
	cfg.OutboxRelayURL = "http://localhost:8111"
	cfg.OutboxRelaySecret = nil

	if err := cfg.Validate(); err == nil {
		t.Fatal("a relay URL without a secret must be refused")
	}

	cfg.OutboxRelaySecret = []byte("secret")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a paired URL and secret must validate: %v", err)
	}

	cfg.OutboxRelayURL = "not-a-url"
	if err := cfg.Validate(); err == nil {
		t.Fatal("a relative relay URL must be refused; the client appends its own path")
	}
}

func TestValidateReportsEveryProblemAtOnce(t *testing.T) {
	cfg := Load()
	cfg.DatabaseURL = ""
	cfg.MaxRouteAge = 0
	cfg.QuoteRetention = 0
	cfg.Workers.BatchSize = 0

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{
		"PRICING_DATABASE_URL", "PRICING_MAX_ROUTE_AGE_SECONDS",
		"PRICING_QUOTE_RETENTION_DAYS", "PRICING_WORKER_BATCH_SIZE",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s so one restart shows the whole list; got: %v", want, err)
		}
	}
}
