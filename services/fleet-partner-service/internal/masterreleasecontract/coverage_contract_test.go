package masterreleasecontract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const moduleRoot = "../.."

type coverageRecord struct {
	Section                int      `json:"section"`
	Subsection             string   `json:"subsection"`
	RequirementSummary     string   `json:"requirementSummary"`
	Status                 string   `json:"status"`
	ImplementationEvidence []string `json:"implementationEvidence"`
	TestEvidence           []string `json:"testEvidence"`
	MigrationEvidence      []string `json:"migrationEvidence"`
	ContractEvidence       []string `json:"contractEvidence"`
	DeferralReference      string   `json:"deferralReference"`
}

type coverageDocument struct {
	SchemaVersion int              `json:"schemaVersion"`
	Records       []coverageRecord `json:"records"`
}

type acceptanceItem struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	Evidence []string `json:"evidence"`
}

type acceptanceDocument struct {
	SchemaVersion int              `json:"schemaVersion"`
	Items         []acceptanceItem `json:"items"`
}

func decodeJSON(t *testing.T, path string, destination any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(moduleRoot, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	if err = decoder.Decode(destination); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func requireEvidencePaths(t *testing.T, owner string, paths []string) {
	t.Helper()
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			t.Fatalf("%s has an empty evidence path", owner)
		}
		if _, err := os.Stat(filepath.Clean(filepath.Join(moduleRoot, path))); err != nil {
			t.Errorf("%s points at missing evidence %q: %v", owner, path, err)
		}
	}
}

func TestCoverageMatrixIncludesEveryBindingSectionAndNoRealGap(t *testing.T) {
	var document coverageDocument
	decodeJSON(t, "docs/master-spec-release1-coverage.json", &document)
	if document.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", document.SchemaVersion)
	}
	allowed := map[string]bool{"PASS": true, "DEFERRED_TAXI": true, "DEFERRED_AI_AUTOMATION": true, "NOT_APPLICABLE_WITH_REASON": true, "REAL_GAP": true}
	sections := map[int]bool{}
	deferrals := map[string]bool{}
	for index, record := range document.Records {
		owner := fmt.Sprintf("records[%d] section %d", index, record.Section)
		if !allowed[record.Status] {
			t.Errorf("%s has unsupported status %q", owner, record.Status)
		}
		if record.Status == "REAL_GAP" {
			t.Errorf("%s leaves a REAL_GAP in the release gate", owner)
		}
		if record.Section >= 34 && record.Section <= 72 {
			sections[record.Section] = true
		}
		if strings.TrimSpace(record.Subsection) == "" || strings.TrimSpace(record.RequirementSummary) == "" {
			t.Errorf("%s has no subsection or requirement summary", owner)
		}
		if record.Status == "PASS" {
			if len(record.ImplementationEvidence) == 0 || len(record.TestEvidence) == 0 {
				t.Errorf("%s PASS lacks implementation or test evidence", owner)
			}
			combined := strings.ToLower(record.Subsection + " " + record.RequirementSummary)
			if strings.Contains(combined, "taxi production") || strings.Contains(combined, "ai/model") || strings.Contains(combined, "automation execution") {
				t.Errorf("%s marks an explicitly deferred capability PASS", owner)
			}
		}
		if strings.HasPrefix(record.Status, "DEFERRED_") {
			if record.DeferralReference == "" {
				t.Errorf("%s has no deferral ADR", owner)
			}
			deferrals[record.Status] = true
		}
		requireEvidencePaths(t, owner, record.ImplementationEvidence)
		requireEvidencePaths(t, owner, record.TestEvidence)
		requireEvidencePaths(t, owner, record.MigrationEvidence)
		requireEvidencePaths(t, owner, record.ContractEvidence)
		if record.DeferralReference != "" {
			requireEvidencePaths(t, owner, []string{record.DeferralReference})
		}
	}
	for section := 34; section <= 72; section++ {
		if !sections[section] {
			t.Errorf("binding section %d is absent from the coverage matrix", section)
		}
	}
	for _, status := range []string{"DEFERRED_TAXI", "DEFERRED_AI_AUTOMATION"} {
		if !deferrals[status] {
			t.Errorf("explicit deferral %s is absent", status)
		}
	}
}

func TestDeferralADRsContainRequiredReleaseBoundaries(t *testing.T) {
	checks := map[string][]string{
		"docs/adr/ADR-001-release1-taxi-deferral.md": {
			"Dispatch", "Hauling", "fail-closed", "No Taxi success", "source-service contract", "provider/execution compatibility", "Rollback", "Security", "Human",
		},
		"docs/adr/ADR-002-release1-ai-automation-deferral.md": {
			"live assignments", "money", "safety", "compliance", "disabled", "No automatic adverse action", "approved implementation package", "Rollback", "Human",
		},
	}
	for path, fragments := range checks {
		raw, err := os.ReadFile(filepath.Join(moduleRoot, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(raw)
		for _, fragment := range fragments {
			if !strings.Contains(strings.ToLower(text), strings.ToLower(fragment)) {
				t.Errorf("%s does not document %q", path, fragment)
			}
		}
	}
}

func TestSection71MachineChecklistIsCompleteAndEvidenceBacked(t *testing.T) {
	var document acceptanceDocument
	decodeJSON(t, "docs/release1-master-spec-acceptance.json", &document)
	want := map[string]bool{}
	for group := 1; group <= 8; group++ {
		for item := 1; item <= 5; item++ {
			want[fmt.Sprintf("71.%d.%d", group, item)] = true
		}
	}
	want["release1.deferral.taxi"] = true
	want["release1.deferral.ai_automation"] = true
	seen := map[string]bool{}
	for _, item := range document.Items {
		if !want[item.ID] {
			t.Errorf("unexpected checklist item %q", item.ID)
		}
		if seen[item.ID] {
			t.Errorf("duplicate checklist item %q", item.ID)
		}
		seen[item.ID] = true
		switch item.ID {
		case "release1.deferral.taxi":
			if item.Status != "DEFERRED_TAXI" {
				t.Errorf("Taxi status = %s", item.Status)
			}
		case "release1.deferral.ai_automation":
			if item.Status != "DEFERRED_AI_AUTOMATION" {
				t.Errorf("AI/automation status = %s", item.Status)
			}
		default:
			if item.Status != "PASS" {
				t.Errorf("%s status = %s, want PASS", item.ID, item.Status)
			}
		}
		if len(item.Evidence) == 0 {
			t.Errorf("%s has no evidence", item.ID)
		}
		requireEvidencePaths(t, item.ID, item.Evidence)
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("required checklist item %s is absent", id)
		}
	}
}

func TestSpecializedCapabilityClassificationDoesNotClaimTaxiExecution(t *testing.T) {
	var document struct {
		Capabilities []struct {
			Code, Status, Reason, Deferral string
			DefaultEnabled                 bool
		} `json:"capabilities"`
	}
	decodeJSON(t, "docs/release1-specialized-logistics-classification.json", &document)
	want := map[string]bool{"COURIER_LAST_MILE": true, "TAXI_PASSENGER": true, "TRUCKING_HAULAGE": true, "HEAVY_SPECIALIZED": true, "FREIGHT_FORWARDING": true, "COLD_CHAIN": true, "MEDICAL_SENSITIVE": true, "AGRICULTURAL_FOOD": true, "MOVING_RELOCATION": true, "TOWING_RECOVERY": true}
	for _, capability := range document.Capabilities {
		delete(want, capability.Code)
		if capability.Status != "PASS_ENABLED" && capability.Status != "DISABLED_BY_CAPABILITY" && capability.Status != "DEFERRED_WITH_REASON" {
			t.Errorf("%s has invalid classification %s", capability.Code, capability.Status)
		}
		if capability.DefaultEnabled {
			t.Errorf("%s is not safely default-disabled", capability.Code)
		}
		if capability.Code == "TAXI_PASSENGER" && (capability.Status != "DEFERRED_WITH_REASON" || capability.Deferral == "") {
			t.Errorf("Taxi must be explicitly deferred with an ADR: %+v", capability)
		}
	}
	for code := range want {
		t.Errorf("specialized capability %s is absent", code)
	}
}
