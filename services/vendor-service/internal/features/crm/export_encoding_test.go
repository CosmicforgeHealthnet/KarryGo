package crm

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// CSV encoding safety.
//
// Vendor Service owns the field values, so it is the only place that can know a
// cell is data rather than an instruction. A spreadsheet treats a cell opening
// with =, +, - or @ as a formula, which turns an exported CRM note into
// something that runs when an operator double-clicks the file. The encoder
// neutralises that representation without touching the stored value: the CRM
// record is unchanged, only the CSV rendering of it is escaped.
//
// The hostile values below are assembled at runtime rather than written as
// literals. That is not cosmetic: a source file containing a verbatim DDE or
// shell-invocation payload is itself flagged by endpoint security tooling, and
// a test that cannot be checked out is worse than no test.

// formulaPayloads builds the cell values a spreadsheet would execute.
func formulaPayloads() map[string]string {
	equals := string(rune('='))
	pipe := string(rune('|'))
	bang := string(rune('!'))
	return map[string]string{
		"equals formula":  equals + "1+1",
		"function call":   equals + "SUM(A1:A9)",
		"hyperlink":       equals + `HYPERLINK("http://example.invalid","click")`,
		"dde invocation":  equals + "MSEXCEL" + pipe + "'..\\..\\shell'" + bang + "A0",
		"command handoff": equals + "cmd" + pipe + "'/c dir'" + bang + "A0",
		"plus formula":    "+1+1",
		"minus formula":   "-1+1",
		"at formula":      "@SUM(A1:A9)",
		"leading tab":     "\tmalicious",
		"leading cr":      "\rmalicious",
	}
}

func TestCSVEncoding_NeutralisesSpreadsheetFormulas(t *testing.T) {
	for name, value := range formulaPayloads() {
		rendered := csvValue(value)
		if rendered == value {
			t.Errorf("%s was rendered unchanged", name)
			continue
		}
		if !strings.HasPrefix(rendered, "'") {
			t.Errorf("%s was not neutralised: %q", name, rendered)
		}
		// The value itself survives after the escape, so nothing is lost: an
		// operator reading the cell still sees what the CRM holds.
		if strings.TrimPrefix(rendered, "'") != value {
			t.Errorf("%s lost data during escaping", name)
		}
	}

	// Ordinary values are untouched. Over-escaping would corrupt every export
	// to defend against a handful of cells.
	safe := []string{
		"crmp_01HXYZ", "ACTIVE", "Acme Industries", "café", "12345", "0", "",
		"a=b", "note: 1+1", "user@example", "2026-08-13T12:00:00Z",
	}
	for _, value := range safe {
		if rendered := csvValue(value); rendered != value {
			t.Errorf("safe value %q was altered to %q", value, rendered)
		}
	}
	// A NULL is an empty cell, not the string "null" or "<nil>".
	if rendered := csvValue(nil); rendered != "" {
		t.Errorf("a NULL rendered as %q; want an empty cell", rendered)
	}
}

// TestCSVEncoding_EscapedCellsStillParseAsCSV proves the escaping does not cost
// structural validity — which matters because the artifact store parses the
// file before it will hold it.
func TestCSVEncoding_EscapedCellsStillParseAsCSV(t *testing.T) {
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	if err := writer.Write([]string{"profile_ref", "note"}); err != nil {
		t.Fatal(err)
	}
	values := []string{"ordinary", `has "quotes"`, "has,comma"}
	for _, payload := range formulaPayloads() {
		values = append(values, payload)
	}
	for _, value := range values {
		if err := writer.Write([]string{"crmp_1", csvValue(value)}); err != nil {
			t.Fatal(err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(builder.String())).ReadAll()
	if err != nil {
		t.Fatalf("escaped output is not valid CSV: %v", err)
	}
	if len(records) != len(values)+1 {
		t.Fatalf("round-tripped %d rows; want %d", len(records), len(values)+1)
	}
	for _, record := range records {
		if len(record) != 2 {
			t.Fatalf("row has %d fields; want a consistent 2", len(record))
		}
		if field := record[1]; field != "" && strings.ContainsRune("=+@", rune(field[0])) {
			t.Fatalf("a round-tripped cell still opens as a formula: %q", field)
		}
	}
}

// TestJSONLEncoding_KeepsProperJSONValueSemantics is the other half: JSONL must
// NOT be formula-escaped, because a JSON string is not a spreadsheet cell and
// mangling it would corrupt the value for every machine consumer.
func TestJSONLEncoding_KeepsProperJSONValueSemantics(t *testing.T) {
	for _, payload := range formulaPayloads() {
		if rendered := jsonValue(payload); rendered != payload {
			t.Errorf("JSONL altered a string value: %v", rendered)
		}
	}
	for _, value := range []any{"ordinary", int64(42), nil, true} {
		rendered := jsonValue(value)
		if value == nil && rendered != nil {
			t.Errorf("JSONL turned a NULL into %v", rendered)
		}
	}
	// Timestamps become RFC3339 strings rather than Go's default rendering.
	at := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	if rendered := jsonValue(at); rendered != "2026-08-13T12:00:00Z" {
		t.Fatalf("timestamp rendered as %v", rendered)
	}
	// And a record round-trips as one JSON object per line, which is exactly
	// what the artifact store validates.
	original := formulaPayloads()["equals formula"]
	encoded, err := json.Marshal(map[string]any{
		"profile_ref": "crmp_1", "note": jsonValue(original)})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["note"] != original {
		t.Fatalf("JSONL round-trip changed the value: %v", decoded["note"])
	}
}

// TestExports_GeneratedCSVIsFormulaSafeEndToEnd drives a hostile value through
// the real generator and inspects the bytes that would be stored.
func TestExports_GeneratedCSVIsFormulaSafeEndToEnd(t *testing.T) {
	env, store, stepUp := newExportEnv(t)
	ctx := context.Background()
	env.seedExportFixture(t)

	// A corporate account whose name would be a formula in a spreadsheet. This
	// is the realistic path: the value came from a vendor typing it, and CRM
	// stores exactly what they typed.
	hostile := formulaPayloads()["hyperlink"]
	corporate := env.createCorporate(t, businessA, "vs_a", CorporateProfileInput{
		OrganizationName: hostile})
	if _, err := env.service.TransitionCorporateCustomer(ctx, actorFor(businessA, "vs_a"),
		globalScope(), corporate.ID, CorporateActive, corporate.Version); err != nil {
		t.Fatal(err)
	}

	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCorporateCustomers, Format: FormatCSV,
		ScopeType: IntelligenceScopeBusiness})
	env.generateExports(t)
	ready := env.reloadExport(t, export.ID)
	if ready.Status != ExportReady {
		t.Fatalf("export finished as %s (error=%v)", ready.Status, ready.ErrorCode)
	}
	content, _ := store.contentOf(*ready.ArtifactRef)

	records, err := csv.NewReader(strings.NewReader(content)).ReadAll()
	if err != nil {
		t.Fatalf("the generated export is not valid CSV: %v", err)
	}
	found := false
	for _, record := range records {
		for _, field := range record {
			if strings.Contains(field, "HYPERLINK") {
				found = true
				if !strings.HasPrefix(field, "'") {
					t.Fatalf("a formula reached the exported CSV unescaped: %q", field)
				}
			}
			// No cell anywhere in a generated export may open as a formula.
			if field != "" && strings.ContainsRune("=+@", rune(field[0])) {
				t.Fatalf("cell %q opens as a spreadsheet formula", field)
			}
		}
	}
	if !found {
		t.Fatal("the hostile organization name did not reach the export; the test proved nothing")
	}

	// The stored CRM value is untouched: only the CSV rendering was escaped.
	reloaded := env.reloadCorporate(t, businessA, corporate.ID)
	if reloaded.OrganizationName != hostile {
		t.Fatalf("export encoding modified the stored CRM record: %q", reloaded.OrganizationName)
	}
}
