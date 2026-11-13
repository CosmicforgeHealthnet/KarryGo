package integration_test

import (
	"context"
	"testing"
)

// TestPendingVerificationAccessAndLifecycleGate drives the production router
// and PostgreSQL: a company that has submitted its onboarding can sign in and
// read its own data, cannot operate until it is ACTIVE, and is unlocked the
// moment its authoritative status becomes ACTIVE — with no new session.
func TestPendingVerificationAccessAndLifecycleGate(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	ctx := context.Background()
	owner := onboardingLogin(t, router, h, "pending-owner@example.com", "+2348012345621")
	company := map[string]any{"legalName": "Pending Logistics", "registrationNumber": "RC-PENDING-GATE", "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Pending Owner"}
	code, body := onboardCall(t, router, owner, "POST", "/organizations", company, 0)
	if code != 201 {
		t.Fatal(body)
	}
	orgID := documentData(t, body)["organizationId"].(string)
	setStatus := func(status string) {
		t.Helper()
		if _, err := h.pool.Exec(ctx, `UPDATE fleet_partner_organizations SET status=$2 WHERE id=$1`, orgID, status); err != nil {
			t.Fatal(err)
		}
	}
	errorCode := func(body map[string]any) string {
		if e, ok := body["error"].(map[string]any); ok {
			code, _ := e["code"].(string)
			return code
		}
		return ""
	}
	operate := func(token string) (int, string) {
		code, body := onboardCall(t, router, token, "POST", "/providers", map[string]any{}, 0)
		return code, errorCode(body)
	}

	// Before submission the company is still onboarding.
	if code, errCode := operate(owner); code != 403 || errCode != "organization_onboarding_incomplete" {
		t.Fatalf("draft company operated: %d %s", code, errCode)
	}

	// Submitted and under review: a fresh sign-in reaches the company...
	for _, status := range []string{"BUSINESS_SUBMITTED", "UNDER_REVIEW"} {
		setStatus(status)
		token := signInExisting(t, router, h, "pending-owner@example.com")
		code, body := onboardCall(t, router, token, "GET", "/organizations/current", nil, 0)
		if code != 200 || documentData(t, body)["status"] != status {
			t.Fatalf("%s: signed-in company read %d %v", status, code, body)
		}
		// ...can read its own records...
		for _, path := range []string{"/organizations/" + orgID + "/onboarding-status", "/organizations/" + orgID + "/documents", "/organizations/" + orgID + "/profile", "/identity/profile"} {
			if code, body := onboardCall(t, router, token, "GET", path, nil, 0); code != 200 {
				t.Fatalf("%s: read %s refused %d %v", status, path, code, body)
			}
		}
		// ...cannot operate, even calling the API directly...
		if code, errCode := operate(token); code != 403 || errCode != "organization_verification_pending" {
			t.Fatalf("%s: operational write not refused: %d %s", status, code, errCode)
		}
		// ...and the gate does not block permitted account management (its
		// own rules still apply, so the outcome is not the lifecycle refusal).
		code, body = onboardCall(t, router, token, "POST", "/staff/invitations", map[string]any{}, 0)
		if errorCode(body) == "organization_verification_pending" {
			t.Fatalf("%s: account management blocked by the lifecycle gate: %d", status, code)
		}
	}

	// Changes requested: corrections are allowed, operations are not.
	setStatus("CHANGES_REQUIRED")
	if code, errCode := operate(owner); code != 403 || errCode != "organization_changes_required" {
		t.Fatalf("changes-required company operated: %d %s", code, errCode)
	}
	profile := map[string]any{"registeredAddress": map[string]any{"street": "1 Road", "city": "Lagos", "state": "Lagos", "countryCode": "NG"}, "operatingAddress": map[string]any{"street": "1 Road", "city": "Lagos", "state": "Lagos", "countryCode": "NG"}}
	if code, body := onboardCall(t, router, owner, "PUT", "/organizations/"+orgID+"/profile", profile, 0); errorCode(body) == "organization_changes_required" {
		t.Fatalf("correction blocked by the lifecycle gate: %d %v", code, body)
	}

	// Approval: the same session is unlocked immediately.
	setStatus("UNDER_REVIEW")
	if code, _ := operate(owner); code != 403 {
		t.Fatalf("expected refusal before approval, got %d", code)
	}
	setStatus("ACTIVE")
	// The request now reaches the providers use case, which rejects the empty
	// contact details itself: proof the lifecycle gate let it through.
	if code, errCode := operate(owner); code != 422 {
		t.Fatalf("active company not unlocked: %d %s", code, errCode)
	}

	// Another company can neither read nor write this one.
	other := onboardingLogin(t, router, h, "pending-other@example.com", "+2348012345622")
	company["legalName"], company["registrationNumber"] = "Other Pending Logistics", "RC-PENDING-OTHER"
	if code, body := onboardCall(t, router, other, "POST", "/organizations", company, 0); code != 201 {
		t.Fatal(body)
	}
	if code, _ := onboardCall(t, router, other, "GET", "/organizations/"+orgID+"/documents", nil, 0); code != 404 {
		t.Fatalf("cross-organization read: %d", code)
	}
	if code, _ := onboardCall(t, router, other, "PUT", "/organizations/"+orgID+"/profile", profile, 0); code != 404 {
		t.Fatalf("cross-organization write: %d", code)
	}
}
