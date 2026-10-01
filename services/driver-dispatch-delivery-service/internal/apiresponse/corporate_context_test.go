package apiresponse

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/providercompat"
)

// canonicalReader implements BOTH surfaces, with the legacy one deliberately
// returning a contradictory answer, so a test can prove which one wins.
type canonicalReader struct {
	corporate providercompat.CorporateContext
	err       error
}

func (r canonicalReader) ReadCorporateContext(context.Context, string, string) (providercompat.CorporateContext, error) {
	return r.corporate, r.err
}

func (r canonicalReader) ReadMembershipSummary(context.Context, string, string) (providercompat.Summary, error) {
	// Contradicts the canonical answer on purpose.
	return providercompat.Summary{ProviderMode: providercompat.ModeIndependent}, nil
}

// legacyOnlyReader has no canonical surface.
type legacyOnlyReader struct {
	summary providercompat.Summary
	err     error
}

func (r legacyOnlyReader) ReadMembershipSummary(context.Context, string, string) (providercompat.Summary, error) {
	return r.summary, r.err
}

func newContext(riderID string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set("dispatch_rider_id", riderID)
	return c
}

// DSP-002: the canonical Section 7 corporate context is authoritative. Two
// conflicting interpretations must not coexist.
func TestResolveCorporateContext_CanonicalWins(t *testing.T) {
	reader := canonicalReader{corporate: providercompat.CorporateContext{
		ProviderID: "rider-1", IsManagedProvider: true, OrganizationID: "org-1",
		MembershipID: "mem-1", MembershipStatus: "ACTIVE", OperatingMode: "HYBRID",
		FinancialVisibilityPolicy: providercompat.FinancialVisibilityPolicy{ShowOwnEarning: true},
		ServiceEligibility:        map[string]bool{"DISPATCH": true},
	}}
	got, err := resolveCorporateContext(newContext("rider-1"), reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsManagedProvider {
		t.Fatal("the canonical answer (managed) must win over the legacy one (independent)")
	}
	if got.OperatingMode != "HYBRID" {
		t.Fatalf("operatingMode = %q; Fleet's persisted mode must be carried through", got.OperatingMode)
	}
	if !got.FinancialVisibilityPolicy.ShowOwnEarning {
		t.Fatal("the typed policy must be carried through")
	}
}

// DSP-003: whatever operating mode Fleet persists is the one Dispatch uses.
// Nothing here defaults or hardcodes a mode.
func TestResolveCorporateContext_UsesPersistedOperatingMode(t *testing.T) {
	for _, mode := range []string{"CENTRALIZED", "DELEGATED", "HYBRID"} {
		reader := canonicalReader{corporate: providercompat.CorporateContext{
			ProviderID: "rider-1", IsManagedProvider: true, OrganizationID: "org-1",
			MembershipID: "mem-1", MembershipStatus: "ACTIVE", OperatingMode: mode,
			ServiceEligibility: map[string]bool{"DISPATCH": true},
		}}
		got, err := resolveCorporateContext(newContext("rider-1"), reader)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", mode, err)
		}
		if got.OperatingMode != mode {
			t.Fatalf("operatingMode = %q, want %q", got.OperatingMode, mode)
		}
	}
}

// The legacy membership-summary path is NORMALIZED onto the canonical model
// rather than interpreted separately.
func TestResolveCorporateContext_LegacyNormalizesOntoCanonicalModel(t *testing.T) {
	reader := legacyOnlyReader{summary: providercompat.Summary{
		ProviderMode:     providercompat.ModeOrganizationManaged,
		Organization:     &providercompat.Organization{ID: "org-7", Status: "ACTIVE"},
		Membership:       &providercompat.Membership{Status: "ACTIVE", Role: "RIDER", ServiceCategory: "DISPATCH"},
		AssignmentPolicy: &providercompat.AssignmentPolicy{AssignmentMode: "CENTRALIZED"},
	}}
	got, err := resolveCorporateContext(newContext("rider-1"), reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsManagedProvider || got.OrganizationID != "org-7" || got.OperatingMode != "CENTRALIZED" {
		t.Fatalf("legacy summary not normalized onto the canonical model: %+v", got)
	}
	// The compatibility path must not be leakier than the canonical one: with
	// no typed policy to carry, it falls back to conservative.
	if got.FinancialVisibilityPolicy.ShowCustomerFare {
		t.Fatal("the legacy path must not invent a permissive visibility policy")
	}
}

func TestResolveCorporateContext_IndependentLegacySummary(t *testing.T) {
	reader := legacyOnlyReader{summary: providercompat.Summary{ProviderMode: providercompat.ModeIndependent}}
	got, err := resolveCorporateContext(newContext("rider-1"), reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsManagedProvider {
		t.Fatal("an independent summary must not resolve to managed")
	}
}

// A Fleet outage must refuse the request, never serve an unmasked body.
func TestResolveCorporateContext_OutageFailsClosed(t *testing.T) {
	if _, err := resolveCorporateContext(newContext("rider-1"),
		canonicalReader{err: errors.New("connection refused")}); err == nil {
		t.Fatal("a canonical read failure must fail closed")
	}
	if _, err := resolveCorporateContext(newContext("rider-1"),
		legacyOnlyReader{err: errors.New("connection refused")}); err == nil {
		t.Fatal("a legacy read failure must fail closed")
	}
	// An unrecognised provider mode is not a licence to proceed.
	if _, err := resolveCorporateContext(newContext("rider-1"),
		legacyOnlyReader{summary: providercompat.Summary{ProviderMode: "SOMETHING_NEW"}}); err == nil {
		t.Fatal("an unknown provider mode must fail closed")
	}
}
