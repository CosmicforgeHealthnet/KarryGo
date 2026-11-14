package providercompat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// recordedFleetMembershipSummary is the body Fleet's membership-summary route
// answers with after FLEET-6h: the standard envelope plus the deprecated
// top-level aliases kept for clients that predate envelope decoding.
const recordedFleetMembershipSummary = `{
  "data": {
    "providerMode": "ORGANIZATION_MANAGED",
    "organization": {"id": "org_fixture", "displayName": "Fixture Logistics", "status": "ACTIVE",
      "branch": {"id": "br_1", "name": "Ikeja", "type": "DEPOT"}},
    "membership": {"status": "ACTIVE", "role": "EMPLOYEE", "serviceCategory": "HAULING"},
    "assignmentPolicy": {"assignmentMode": "OFFER", "providerDeclineRequired": true},
    "fieldVisibilityPolicy": "ORGANIZATION"
  },
  "meta": {"requestId": "req-1"},
  "providerMode": "ORGANIZATION_MANAGED",
  "organization": {"id": "org_fixture", "displayName": "Fixture Logistics", "status": "ACTIVE",
    "branch": {"id": "br_1", "name": "Ikeja", "type": "DEPOT"}},
  "membership": {"status": "ACTIVE", "role": "EMPLOYEE", "serviceCategory": "HAULING"},
  "assignmentPolicy": {"assignmentMode": "OFFER", "providerDeclineRequired": true},
  "fieldVisibilityPolicy": "ORGANIZATION"
}`

func readSummaryFrom(t *testing.T, body string) (Summary, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	client := NewClient(server.URL, "driver-hauling-service", []byte("secret"), time.Second)
	return client.ReadMembershipSummary(context.Background(), "driver-hauling-service", "provider_fixture")
}

func TestMembershipSummaryDecodesRecordedFleetEnvelope(t *testing.T) {
	got, err := readSummaryFrom(t, recordedFleetMembershipSummary)
	if err != nil {
		t.Fatalf("ReadMembershipSummary: %v", err)
	}
	if got.ProviderMode != ModeOrganizationManaged || got.Organization == nil ||
		got.Organization.Branch == nil || got.Organization.Branch.ID != "br_1" ||
		got.Membership == nil || got.AssignmentPolicy == nil || !got.AssignmentPolicy.ProviderDeclineRequired {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestMembershipSummaryPrefersEnvelopeOverTopLevelAliases(t *testing.T) {
	got, err := readSummaryFrom(t, `{"data":{"providerMode":"INDEPENDENT"},"meta":{"requestId":"r"},
		"providerMode":"ORGANIZATION_MANAGED","organization":{"id":"must_not_be_read"}}`)
	if err != nil {
		t.Fatalf("ReadMembershipSummary: %v", err)
	}
	if got.ProviderMode != ModeIndependent || got.Organization != nil {
		t.Fatalf("envelope must win over aliases, got %+v", got)
	}
}

func TestMembershipSummaryStillDecodesLegacyBareBody(t *testing.T) {
	got, err := readSummaryFrom(t, `{"providerMode":"INDEPENDENT"}`)
	if err != nil {
		t.Fatalf("ReadMembershipSummary: %v", err)
	}
	if got.ProviderMode != ModeIndependent {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestMembershipSummaryRejectsEnvelopeWithUnsupportedMode(t *testing.T) {
	if _, err := readSummaryFrom(t, `{"data":{"providerMode":"BOGUS"},"meta":{}}`); err == nil {
		t.Fatal("expected an unsupported providerMode inside the envelope to be refused")
	}
}
