package integration_test

// Requirement-row test for spec Section 6 (API and data conventions), row
// 6c: "UTC ISO 8601 timestamps. Store original local timezone separately
// where operationally relevant."

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// rowsTimestamps walks a decoded JSON value and returns every string that is
// an RFC 3339 timestamp, keyed by its JSON path.
func rowsTimestamps(prefix string, value any, out map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			rowsTimestamps(prefix+"."+key, child, out)
		}
	case []any:
		for _, child := range typed {
			rowsTimestamps(prefix+"[]", child, out)
		}
	case string:
		if _, err := time.Parse(time.RFC3339Nano, typed); err == nil {
			out[prefix] = typed
		}
	}
}

// FLEET-6c: every timestamp the API returns is UTC ISO 8601, whatever the
// server process's local zone, and the operational local timezone of an
// organization and of each branch is stored and returned as its own field.
//
// The process zone is moved to West Africa Time (UTC+1) for the duration of
// the test: a server that let PostgreSQL timestamps or computed instants
// leak in the process zone would answer "+01:00" here.
func TestRow6c_UTCTimestampsAndSeparateLocalTimezone(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("WAT", 3600)
	t.Cleanup(func() { time.Local = previous })

	router, h := buildOnboardingRouter(t)
	owner := onboardingLogin(t, router, h, "r6c-owner@example.com", "+2348012347401")
	orgID := rowsActiveCompany(t, router, h, owner, "Rows6c Time Co", "RC-R6C-1")

	call := func(method, path string, payload any, version int) map[string]any {
		t.Helper()
		code, body := onboardCall(t, router, owner, method, path, payload, version)
		if code >= 300 {
			t.Fatalf("%s %s = %d %v", method, path, code, body)
		}
		return body
	}
	// A branch in a different operational zone from its organization.
	branch := documentData(t, call("POST", "/branches", map[string]any{"code": "ACC", "name": "Accra Depot", "type": "DEPOT", "timezone": "Africa/Accra"}, 0))
	if branch["timezone"] != "Africa/Accra" {
		t.Fatalf("branch timezone = %v, want Africa/Accra stored separately", branch["timezone"])
	}
	current := documentData(t, call("GET", "/organizations/current", nil, 0))
	if current["timezone"] != "Africa/Lagos" {
		t.Fatalf("organization timezone = %v, want Africa/Lagos", current["timezone"])
	}
	var stored string
	if err := h.pool.QueryRow(context.Background(), `SELECT timezone FROM fleet_partner_branches WHERE organization_id = $1 AND code = 'ACC'`, orgID).Scan(&stored); err != nil || stored != "Africa/Accra" {
		t.Fatalf("stored branch timezone = %q (%v)", stored, err)
	}

	// A client instant with an offset is normalized to UTC.
	member := seedMember(t, h.pool, orgID, seedIdentity(t, h.pool, "r6c-staff@example.com", "+2348012347402"), false, nil)
	offsetExpiry := time.Now().Add(72 * time.Hour).In(time.FixedZone("UTC+5", 5*3600)).Truncate(time.Second)
	bound := documentData(t, call("PUT", "/staff/"+member.ID+"/roles", map[string]any{"bindings": []map[string]any{
		{"roleId": "rol_system_view_only", "scopeType": "BRANCH", "scopeId": branch["id"], "expiresAt": offsetExpiry.Format(time.RFC3339)},
	}}, 0))
	binding := bound["bindings"].([]any)[0].(map[string]any)
	got, err := time.Parse(time.RFC3339Nano, binding["expiresAt"].(string))
	if err != nil || !got.Equal(offsetExpiry) {
		t.Fatalf("binding expiresAt = %v (%v), want the same instant as %s", binding["expiresAt"], err, offsetExpiry.Format(time.RFC3339))
	}

	// Every timestamp on these representative reads is UTC.
	timestamps := map[string]string{}
	for _, path := range []string{"/organizations/current", "/branches", "/branches/" + branch["id"].(string), "/staff/" + member.ID + "/role-bindings", "/sessions", "/identity/profile", "/organization-settings"} {
		rowsTimestamps(path, call("GET", path, nil, 0), timestamps)
	}
	rowsTimestamps("PUT /staff/roles", bound, timestamps)
	if len(timestamps) < 5 {
		t.Fatalf("too few timestamps inspected (%d): %v", len(timestamps), timestamps)
	}
	for path, value := range timestamps {
		if !strings.HasSuffix(value, "Z") {
			t.Errorf("%s = %q is not UTC", path, value)
		}
	}

	// The step-up deadline is a computed instant, not a stored one.
	code, body := onboardCall(t, router, owner, "POST", "/auth/step-up/start", map[string]any{"actionClass": "phone_change:+2348012347499"}, 0)
	if code != http.StatusAccepted {
		t.Fatalf("step-up start = %d %v", code, body)
	}
	otp, _ := h.sender.LastOTP()
	code, body = onboardCall(t, router, owner, "POST", "/auth/step-up/verify", map[string]any{"challengeId": body["data"].(map[string]any)["challengeId"], "code": otp.Code}, 0)
	if code != http.StatusOK {
		t.Fatalf("step-up verify = %d %v", code, body)
	}
	if expires, _ := body["data"].(map[string]any)["expiresAt"].(string); !strings.HasSuffix(expires, "Z") {
		t.Fatalf("step-up expiresAt = %q is not UTC", expires)
	}
}
