package section30contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/migrationlint"
)

const providerFixtureDir = "testdata/provider-v1"

func TestInventory_MachineReadableJSONHasRequiredFields(t *testing.T) {
	migrations, err := migrationlint.LoadMigrations(migrationsDir)
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}
	raw, err := json.Marshal(migrationlint.BuildInventory(migrations))
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decode inventory JSON: %v", err)
	}
	if len(entries) != len(migrations) {
		t.Fatalf("machine-readable inventory has %d entries, want %d", len(entries), len(migrations))
	}
	for _, key := range []string{
		"migrationNumber", "name", "upFile", "downFile", "category", "transactional",
		"tablesAffected", "indexesAffected", "constraintsAffected", "rollbackSafe",
		"replayVerified", "dataLossPossible", "cleanupReleaseRequired", "historicalBaseline",
	} {
		if _, ok := entries[0][key]; !ok {
			t.Errorf("inventory JSON entry missing required field %q", key)
		}
	}
}

func TestCompatibilityFixtures_CoverRequiredProviderContracts(t *testing.T) {
	metadataRaw, err := os.ReadFile(filepath.Join(providerFixtureDir, "contract-metadata.json"))
	if err != nil {
		t.Fatalf("read contract metadata: %v", err)
	}
	var metadata struct {
		ContractVersion        string `json:"contractVersion"`
		OldestSupportedVersion string `json:"oldestSupportedVersion"`
		Contracts              map[string]struct {
			Status   int      `json:"status"`
			Required []string `json:"required"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		t.Fatalf("parse contract metadata: %v", err)
	}
	if metadata.ContractVersion != "1" || metadata.OldestSupportedVersion != "1" {
		t.Fatalf("provider contract versions = current %q oldest %q, want 1/1", metadata.ContractVersion, metadata.OldestSupportedVersion)
	}
	required := []string{
		"independent_login_dispatch", "independent_login_hauling", "provider_profile",
		"availability", "offer_request", "assignment", "accept", "decline",
		"trip_execution", "wallet_earnings", "error_envelope",
	}
	for _, name := range required {
		contract, ok := metadata.Contracts[name]
		if !ok {
			t.Errorf("contract metadata missing %q", name)
			continue
		}
		if contract.Status < 200 || contract.Status > 599 || len(contract.Required) == 0 {
			t.Errorf("contract %q has invalid status/required metadata: %+v", name, contract)
		}
		raw, err := os.ReadFile(filepath.Join(providerFixtureDir, name+".json"))
		if err != nil {
			t.Errorf("read %s fixture: %v", name, err)
			continue
		}
		if containsCredentialLikeValue(string(raw)) {
			t.Errorf("fixture %q contains a credential-like or private-key value", name)
		}
		var fixture map[string]any
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Errorf("parse %s fixture: %v", name, err)
			continue
		}
		for _, path := range contract.Required {
			if _, ok := dottedValue(fixture, path); !ok {
				t.Errorf("fixture %q missing required path %q", name, path)
			}
		}
	}
}

func containsCredentialLikeValue(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "-----begin private key-----") ||
		strings.Contains(lower, "sk-proj-") || strings.Contains(lower, "eyjhb")
}

func dottedValue(root map[string]any, path string) (any, bool) {
	var current any = root
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok || current == nil {
			return nil, false
		}
	}
	return current, true
}

func TestOriginalAPIMap_CoversEveryAuthoritativeArea(t *testing.T) {
	raw, err := os.ReadFile("../../docs/section30-original-api-mapping.md")
	if err != nil {
		t.Fatalf("read original API map: %v", err)
	}
	text := strings.ToLower(string(raw))
	for _, area := range []string{
		"auth and owner onboarding", "dashboard", "requests and trips", "drivers", "vehicles",
		"verification", "wallet and transactions", "disputes and support", "account delete",
	} {
		if !strings.Contains(text, area) {
			t.Errorf("original API map missing authoritative area %q", area)
		}
	}
	for _, column := range []string{
		"current replacement", "canonical owning service/package", "compatibility behavior",
		"deprecated behavior", "response compatibility", "migration status", "removal policy",
	} {
		if !strings.Contains(text, column) {
			t.Errorf("original API map missing required column %q", column)
		}
	}
}

func TestOpenAPI_MembershipSummaryRouteMatchesManifestAndModels(t *testing.T) {
	raw, err := os.ReadFile("../../docs/openapi/section30-backward-compatibility.openapi.json")
	if err != nil {
		t.Fatalf("read Section 30 OpenAPI: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse Section 30 OpenAPI: %v", err)
	}
	const path = "/internal/fleet-partner/v1/providers/{providerId}/services/{providerService}/membership-summary"
	operation, ok := spec.Paths[path]["get"]
	if !ok || operation.OperationID != "getProviderMembershipSummary" {
		t.Fatalf("OpenAPI route/operation mismatch: %+v", spec.Paths[path])
	}
	source, err := os.ReadFile("../features/providers/http/internal.go")
	if err != nil {
		t.Fatalf("read provider route manifest: %v", err)
	}
	for _, token := range []string{"/providers/:providerId/services/:providerService/membership-summary", "getProviderMembershipSummary"} {
		if !strings.Contains(string(source), token) {
			t.Errorf("provider route manifest missing %q", token)
		}
	}
}

func TestProviderApps_ParseAdditiveProviderModeContract(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", "..", ".."))
	files := []string{
		filepath.Join(repoRoot, "apps", "truck_provider", "lib", "features", "auth", "models", "provider_auth_models.dart"),
		filepath.Join(repoRoot, "apps", "dispatch_provider", "lib", "features", "auth", "models", "dispatch_auth_models.dart"),
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read provider app model %s: %v", file, err)
		}
		for _, field := range []string{"providerMode", "organization", "membership", "assignmentPolicy", "fieldVisibilityPolicy"} {
			if !strings.Contains(string(raw), "['"+field+"']") {
				t.Errorf("provider app model %s does not parse additive field %q", file, field)
			}
		}
	}
}

func TestRolloutFlag_IsWiredInBothProviderServices(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", "..", ".."))
	for _, file := range []string{
		filepath.Join(repoRoot, "services", "driver-hauling-service", "internal", "config", "config.go"),
		filepath.Join(repoRoot, "services", "driver-dispatch-delivery-service", "internal", "config", "config.go"),
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read provider service config %s: %v", file, err)
		}
		if !strings.Contains(string(raw), "PROVIDER_ORGANIZATION_MODE_ENABLED") {
			t.Errorf("provider service config %s is missing the organization-mode rollout flag", file)
		}
	}
}
