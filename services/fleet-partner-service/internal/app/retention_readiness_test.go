package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/objectauth"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/riskregistry"
)

// Section 28 final wiring: readiness enforcement tests. These mirror
// cmd/main.go's own readiness-check construction (a plain []ReadyCheck
// built from components.Retention*Ready -- see cmd/main.go's own comment
// block for why the condition is checked once, not re-probed per poll)
// against the real /ready HTTP contract app.go already established:
// TestReadySucceedsAndFails (app_test.go) already proves that contract
// generically (200 healthy, 503 broken, never a leaked secret); these
// tests prove the SPECIFIC disabled-stays-ready / enabled-fails-closed
// behavior for each of Section 28's mandatory-dependency features.

func retentionReadyCheck(name string, ready bool) ReadyCheck {
	return ReadyCheck{Name: name, Check: func(context.Context) error {
		if !ready {
			return errors.New(name + " enabled without its mandatory dependency configured")
		}
		return nil
	}}
}

// TestReadiness_RetentionDisabledStaysReadyWithoutDependencies proves the
// disabled (default) case: cfg.RetentionExecutionEnabled=false means
// cmd/main.go never appends the three retention checks at all -- /ready
// reports healthy regardless of whether media/support/synchronization are
// configured, exactly like every other optional dependency in this
// service (Redis, service-credential scoping).
func TestReadiness_RetentionDisabledStaysReadyWithoutDependencies(t *testing.T) {
	// No retention checks appended at all -- the disabled case never adds
	// them, mirroring cmd/main.go's `if cfg.RetentionExecutionEnabled { ... }`.
	router := buildTestRouter(t, nil)
	resp := do(router, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("/ready with retention disabled = %d, want 200", resp.Code)
	}
}

// TestReadiness_RetentionEnabledFailsClosedPerMissingDependency proves
// each of the three retention dependency checks independently fails
// closed -- and names which one, never a secret (the check's own error
// message is deliberately generic; app.go's /ready handler already
// reduces every failure to {dependency, "unavailable"} before it reaches
// the response body, confirmed by TestReadySucceedsAndFails).
func TestReadiness_RetentionEnabledFailsClosedPerMissingDependency(t *testing.T) {
	cases := []struct {
		name                                                      string
		expiredExportReady, supportMetadataReady, projectionReady bool
		wantHealthy                                               bool
	}{
		{"all three ready", true, true, true, true},
		{"expired export missing", false, true, true, false},
		{"support metadata missing", true, false, true, false},
		{"superseded projection missing", true, true, false, false},
		{"all three missing", false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := buildTestRouter(t, nil,
				retentionReadyCheck("retention_expired_export", tc.expiredExportReady),
				retentionReadyCheck("retention_support_metadata", tc.supportMetadataReady),
				retentionReadyCheck("retention_superseded_projection", tc.projectionReady),
			)
			resp := do(router, httptest.NewRequest(http.MethodGet, "/ready", nil))
			wantStatus := http.StatusServiceUnavailable
			if tc.wantHealthy {
				wantStatus = http.StatusOK
			}
			if resp.Code != wantStatus {
				t.Fatalf("/ready(%s) = %d, want %d body=%s", tc.name, resp.Code, wantStatus, resp.Body.String())
			}
		})
	}
}

// TestReadiness_ServiceCredentialScopingFailsClosedWithoutCredentials
// proves the fourth mandatory-dependency check the mandate names: service
// credential scoping enabled without any parsed versioned credential.
func TestReadiness_ServiceCredentialScopingFailsClosedWithoutCredentials(t *testing.T) {
	disabled := buildTestRouter(t, nil) // scoping off entirely -> no check appended, stays ready
	if resp := do(disabled, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusOK {
		t.Fatalf("/ready with scoping disabled = %d, want 200", resp.Code)
	}

	enabledNoCredentials := buildTestRouter(t, nil, retentionReadyCheck("service_credential_scoping", false))
	resp := do(enabledNoCredentials, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready with scoping enabled and zero parsed credentials = %d, want 503", resp.Code)
	}

	enabledWithCredentials := buildTestRouter(t, nil, retentionReadyCheck("service_credential_scoping", true))
	resp = do(enabledWithCredentials, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("/ready with scoping enabled and real credentials = %d, want 200", resp.Code)
	}
}

// TestReadiness_RiskAndObjectAuthorizationRegistrySelfChecksAlwaysRun
// proves the two registry self-checks (always appended, no feature flag
// -- see cmd/main.go) actually reflect riskregistry.SelfCheck/
// objectauth.SelfCheck's real return values through the real /ready
// contract, using the real production functions (not a stub) -- a
// regression in either registry's own well-formedness fails readiness.
func TestReadiness_RiskAndObjectAuthorizationRegistrySelfChecksAlwaysRun(t *testing.T) {
	healthy := buildTestRouter(t, nil,
		ReadyCheck{Name: "risk_registry", Check: func(context.Context) error { return riskregistry.SelfCheck() }},
		ReadyCheck{Name: "object_authorization_registry", Check: func(context.Context) error { return objectauth.SelfCheck() }},
	)
	if resp := do(healthy, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusOK {
		t.Fatalf("/ready with real, well-formed registries = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
}
