package masterreleasecontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSection55MigrationIsPairedAdditiveAndHasNoPlaintextCredentialColumn(t *testing.T) {
	upPath := filepath.Join(moduleRoot, "migrations/000074_section55_company_integrations.up.sql")
	downPath := filepath.Join(moduleRoot, "migrations/000074_section55_company_integrations.down.sql")
	raw, err := os.ReadFile(upPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(downPath); err != nil {
		t.Fatalf("paired down migration is absent: %v", err)
	}
	text := strings.ToLower(string(raw))
	for _, table := range []string{"fleet_partner_organization_api_clients", "fleet_partner_webhook_subscriptions", "fleet_partner_webhook_deliveries", "fleet_partner_integration_commands", "fleet_partner_webhook_fanout_events"} {
		if !strings.Contains(text, "create table if not exists "+table) {
			t.Errorf("migration lacks %s", table)
		}
	}
	for _, forbidden := range []string{"secret text", "secret varchar", "previous_secret text", "authorization text"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("migration contains plaintext credential column pattern %q", forbidden)
		}
	}
	for _, required := range []string{"secret_hash", "secret_ciphertext", "unique", "organization_id", "status"} {
		if !strings.Contains(text, required) {
			t.Errorf("migration lacks %q", required)
		}
	}
}

func TestSection55OpenAPIRouteParityAndSecurityMetadata(t *testing.T) {
	var document struct {
		FeatureFlags []string `json:"x-feature-flags"`
		Paths        map[string]map[string]struct {
			Permission      string `json:"x-permission"`
			RequiredScope   string `json:"x-required-scope"`
			Idempotency     bool   `json:"x-idempotency-key"`
			ExpectedVersion bool   `json:"x-expected-version"`
			StepUp          bool   `json:"x-step-up"`
			Security        []map[string][]string
		} `json:"paths"`
	}
	raw, err := os.ReadFile(filepath.Join(moduleRoot, "docs/openapi/section55-integrations.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	required := map[string][]string{
		"/fleet-partner/v1/integrations/api-clients":                    {"get", "post"},
		"/fleet-partner/v1/integrations/api-clients/{id}/rotate":        {"post"},
		"/fleet-partner/v1/integrations/api-clients/{id}/revoke":        {"post"},
		"/fleet-partner/v1/integrations/webhooks":                       {"get", "post"},
		"/fleet-partner/v1/integrations/webhooks/{id}":                  {"patch"},
		"/fleet-partner/v1/integrations/webhooks/{id}/rotate-secret":    {"post"},
		"/fleet-partner/v1/integrations/webhooks/{id}/deliveries":       {"get"},
		"/fleet-partner/v1/integrations/webhook-deliveries/{id}/replay": {"post"},
		"/fleet-partner/integrations/v1/context":                        {"get"},
	}
	for path, methods := range required {
		operations, exists := document.Paths[path]
		if !exists {
			t.Errorf("OpenAPI path %s is absent", path)
			continue
		}
		for _, method := range methods {
			op, exists := operations[method]
			if !exists {
				t.Errorf("OpenAPI operation %s %s is absent", method, path)
				continue
			}
			if len(op.Security) == 0 {
				t.Errorf("%s %s has no auth class", method, path)
			}
			if path == "/fleet-partner/integrations/v1/context" {
				if op.RequiredScope != "integration.context.read" {
					t.Errorf("organization client scope = %q", op.RequiredScope)
				}
			} else if op.Permission == "" {
				t.Errorf("%s %s has no corporate permission", method, path)
			}
			if method != "get" && (!op.Idempotency || !op.StepUp) {
				t.Errorf("%s %s lacks idempotency or step-up metadata", method, path)
			}
		}
	}
	if len(document.FeatureFlags) != 3 {
		t.Errorf("Section 55 feature flags = %v", document.FeatureFlags)
	}
}

func TestSection55AsyncAPIContainsEveryLifecycleEventAndSignatureContract(t *testing.T) {
	var document map[string]any
	raw, err := os.ReadFile(filepath.Join(moduleRoot, "docs/asyncapi/section55-integrations.asyncapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, eventType := range []string{
		"api_client.created.v1", "api_client.rotated.v1", "api_client.revoked.v1",
		"webhook.subscription_created.v1", "webhook.subscription_updated.v1", "webhook.secret_rotated.v1",
		"webhook.disabled.v1", "webhook.delivery_succeeded.v1", "webhook.delivery_failed.v1", "webhook.delivery_replayed.v1",
		"HMAC-SHA-256", "X-KarryGo-Webhook-Signature", "constant time", "at-least-once",
	} {
		if !strings.Contains(text, eventType) {
			t.Errorf("AsyncAPI contract lacks %q", eventType)
		}
	}
}
