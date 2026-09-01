package securehandover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecureHandoverRouterOpenAPIAndRegisterParity(t *testing.T) {
	httpSource, err := os.ReadFile("http.go")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..", "..")
	openAPIRaw, err := os.ReadFile(filepath.Join(root, "docs", "openapi", "secure-handover-chain-of-custody.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	registerRaw, err := os.ReadFile(filepath.Join(root, "docs", "API_ROUTE_REGISTER.md"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Paths map[string]any `json:"paths"`
	}
	if err = json.Unmarshal(openAPIRaw, &contract); err != nil {
		t.Fatal(err)
	}
	routes := map[string]string{
		"/:id/stops/:stopId/pickup-credential": "/api/v1/provider/trips/{id}/stops/{stopId}/pickup-credential",
		"/:id/stops/:stopId/parcels/scan":      "/api/v1/provider/trips/{id}/stops/{stopId}/parcels/scan",
		"/:id/stops/:stopId/evidence-session":  "/api/v1/provider/trips/{id}/stops/{stopId}/evidence-session",
		"/:id/stops/:stopId/evidence":          "/api/v1/provider/trips/{id}/stops/{stopId}/evidence",
		"/:id/delivery/verify":                 "/api/v1/provider/trips/{id}/delivery/verify",
		"/:id/return/verify":                   "/api/v1/provider/trips/{id}/return/verify",
		"/:id/pickup/verify":                   "/api/v1/customer/dispatch/bookings/{id}/pickup/verify",
		"/:id/handover/resend":                 "/api/v1/customer/dispatch/bookings/{id}/handover/resend",
		"/:id/return-credential":               "/api/v1/customer/dispatch/bookings/{id}/return-credential",
		"/:id/stops/:stopId/pickup/verify":     "/api/v1/internal/commerce/dispatch/missions/{id}/stops/{stopId}/pickup/verify",
	}
	for ginPath, openAPIPath := range routes {
		if !strings.Contains(string(httpSource), ginPath) {
			t.Errorf("router missing %s", ginPath)
		}
		if _, ok := contract.Paths[openAPIPath]; !ok {
			t.Errorf("OpenAPI missing %s", openAPIPath)
		}
		registerPath := strings.NewReplacer("{id}", ":id", "{stopId}", ":stopId").Replace(openAPIPath)
		if !strings.Contains(string(registerRaw), registerPath) {
			t.Errorf("route register missing %s", registerPath)
		}
	}
	internalResend := "/api/v1/internal/commerce/dispatch/missions/{id}/handover/resend"
	if _, ok := contract.Paths[internalResend]; !ok || !strings.Contains(string(registerRaw), "/api/v1/internal/commerce/dispatch/missions/:id/handover/resend") {
		t.Error("internal source-scoped resend route is not in OpenAPI/register parity")
	}
	internalReturn := "/api/v1/internal/commerce/dispatch/missions/{id}/stops/{stopId}/return-credential"
	if _, ok := contract.Paths[internalReturn]; !ok || !strings.Contains(string(registerRaw), "/api/v1/internal/commerce/dispatch/missions/:id/stops/:stopId/return-credential") {
		t.Error("internal source-scoped return credential route is not in OpenAPI/register parity")
	}
}

func TestChallengeAndOutboxRepresentationsDoNotLeakSecrets(t *testing.T) {
	challenge := Challenge{ChallengeRef: "handover_ref", OTPHash: "otp-secret-hash", QRTokenHash: "qr-secret-hash", IntendedRecipientRef: "private-recipient", AssignedProviderID: "private-provider"}
	raw, err := json.Marshal(challenge)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"otp-secret-hash", "qr-secret-hash", "private-recipient", "private-provider"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("API representation leaked %q", forbidden)
		}
	}
	repositorySource, err := os.ReadFile("repository.go")
	if err != nil {
		t.Fatal(err)
	}
	emitStart := strings.Index(string(repositorySource), "func emitEventTx")
	if emitStart < 0 {
		t.Fatal("outbox emitter not found")
	}
	emitter := string(repositorySource)[emitStart:]
	for _, forbidden := range []string{"OTPHash", "QRTokenHash", "otp_hash", "qr_token_hash"} {
		if strings.Contains(emitter, forbidden) {
			t.Errorf("generic outbox emitter references secret material %q", forbidden)
		}
	}
}
