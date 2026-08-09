package section28contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sensitiveColumnInventoryPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "docs", "section28-sensitive-column-inventory.json")
}

func sensitiveColumnMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}

// TestSensitiveColumnInventory_MatchesLiveSchema is the drift detector: it
// re-parses every migration's CREATE TABLE / ALTER TABLE ADD COLUMN
// statements on every test run and fails if the result differs from the
// committed docs/section28-sensitive-column-inventory.json -- so the
// inventory cannot silently go stale the way a hand-written document would
// the moment a migration adds, renames or removes a column without anyone
// remembering to update a separate doc.
//
// Regenerate the committed file after an intentional schema change with:
//
//	UPDATE_SENSITIVE_COLUMN_INVENTORY=1 go test ./internal/section28contract/... -run TestSensitiveColumnInventory_MatchesLiveSchema
func TestSensitiveColumnInventory_MatchesLiveSchema(t *testing.T) {
	entries, err := BuildSensitiveColumnInventory(sensitiveColumnMigrationsDir(t))
	if err != nil {
		t.Fatalf("build inventory: %v", err)
	}
	got, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	got = append(got, '\n')

	path := sensitiveColumnInventoryPath(t)
	if os.Getenv("UPDATE_SENSITIVE_COLUMN_INVENTORY") != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write inventory: %v", err)
		}
		t.Logf("regenerated %s with %d entries", path, len(entries))
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed inventory (run with UPDATE_SENSITIVE_COLUMN_INVENTORY=1 to generate it first): %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("docs/section28-sensitive-column-inventory.json is stale (%d entries computed from migrations). "+
			"Re-run with UPDATE_SENSITIVE_COLUMN_INVENTORY=1 and review the diff before committing.", len(entries))
	}
}

// TestClassifyColumn_KeywordCategoriesAreDistinctAndPrioritized pins the
// priority ordering directly (independent of whatever the schema currently
// contains), so a future reordering of categoryOrder is caught even if no
// migration happens to exercise the ambiguous case today.
func TestClassifyColumn_KeywordCategoriesAreDistinctAndPrioritized(t *testing.T) {
	cases := map[string]string{
		// Not "password_hash": internal/architecture_auth_test.go's
		// TestNoPasswordAuthentication scans every non-scanner .go file for
		// password-authentication tokens (this codebase's corporate auth is
		// OTP-only, never password-based) and would fail the build on that
		// literal string appearing here, even only as a test fixture value.
		"session_secret_hash":     CategoryCredential,
		"refresh_token":           CategoryCredential,
		"national_id_number":      CategoryGovernmentID,
		"bank_account_number":     CategoryFinancial,
		"fare_estimate_kobo":      CategoryFinancial,
		"face_scan_url":           CategoryBiometric,
		"current_latitude":        CategoryLocation,
		"contact_email":           CategoryContactPII,
		"emergency_contact_phone": CategoryContactPII,
		"status":                  "",
		"organization_id":         "",
		"created_at":              "",
	}
	for column, want := range cases {
		if got := ClassifyColumn(column); got != want {
			t.Errorf("ClassifyColumn(%q) = %q, want %q", column, got, want)
		}
	}
}

// TestClassifyDisposition_EveryEntryHasAJustification is a structural
// backstop: BuildSensitiveColumnInventory already asserts this implicitly
// (the committed JSON has one), but this pins ClassifyDisposition itself,
// independent of the current schema, so a future category added to
// categoryOrder without a matching ClassifyDisposition case cannot silently
// ship with an empty justification.
func TestClassifyDisposition_EveryEntryHasAJustification(t *testing.T) {
	categories := []string{
		CategoryCredential, CategoryGovernmentID, CategoryFinancial,
		CategoryBiometric, CategoryLocation, CategoryContactPII, "",
	}
	for _, category := range categories {
		disposition, justification := ClassifyDisposition("some_table", "some_column", category)
		if disposition == "" {
			t.Errorf("category %q: empty disposition", category)
		}
		if justification == "" {
			t.Errorf("category %q: empty justification", category)
		}
		if disposition == DispositionApprovedPlaintext && justification == "" {
			t.Errorf("category %q: APPROVED_PLAINTEXT requires a written justification (Section 28)", category)
		}
	}
}

// TestClassifyDisposition_HashSuffixAlwaysHashedRegardlessOfCategory pins
// the one rule that overrides category entirely: a "*_hash" column is
// HASHED no matter what category its name would otherwise classify as.
func TestClassifyDisposition_HashSuffixAlwaysHashedRegardlessOfCategory(t *testing.T) {
	disposition, _ := ClassifyDisposition("t", "refresh_token_hash", CategoryCredential)
	if disposition != DispositionHashed {
		t.Errorf("disposition = %q, want HASHED", disposition)
	}
}

// TestClassifyDisposition_FinancialReferenceVsAmount proves the two
// FINANCIAL sub-cases land on different, individually-correct dispositions:
// a "*reference*" column (points at payment-wallet-service) is a safe
// reference, while a bare amount column is approved plaintext.
func TestClassifyDisposition_FinancialReferenceVsAmount(t *testing.T) {
	refDisposition, _ := ClassifyDisposition("t", "payment_reference", CategoryFinancial)
	if refDisposition != DispositionSafeReference {
		t.Errorf("payment_reference disposition = %q, want SAFE_REFERENCE", refDisposition)
	}
	amountDisposition, _ := ClassifyDisposition("t", "amount_minor", CategoryFinancial)
	if amountDisposition != DispositionApprovedPlaintext {
		t.Errorf("amount_minor disposition = %q, want APPROVED_PLAINTEXT", amountDisposition)
	}
}

// TestClassifyDisposition_OrgContactHasRealRemediationCitedInJustification
// proves the one CONTACT_PII table that actually has an implemented
// anonymization path (internal/platform/retentionexec's OrgContactExecutor)
// is distinguished, in its justification text, from every other CONTACT_PII
// table that does not yet have one -- a reviewer reading the inventory can
// tell the two apart without cross-referencing other code.
func TestClassifyDisposition_OrgContactHasRealRemediationCitedInJustification(t *testing.T) {
	_, withRemediation := ClassifyDisposition(
		"fleet_partner_organization_authorized_representatives", "email", CategoryContactPII)
	if !strings.Contains(withRemediation, "retentionexec") {
		t.Errorf("expected the organization-representative justification to cite retentionexec, got: %s", withRemediation)
	}
	_, withoutRemediation := ClassifyDisposition("fleet_partner_customer_contacts", "email", CategoryContactPII)
	if strings.Contains(withoutRemediation, "retentionexec") {
		t.Errorf("expected a table with no implemented remediation to say so, got: %s", withoutRemediation)
	}
}

// TestBuildSensitiveColumnInventory_SkipsTableLevelClauses proves the
// parser does not mistake CONSTRAINT/CHECK/PRIMARY KEY/UNIQUE/FOREIGN KEY
// table-level clauses for column definitions -- a naive
// "every comma-separated line is a column" parser would misparse a table
// like this into phantom columns named "constraint", "check", etc.
func TestBuildSensitiveColumnInventory_SkipsTableLevelClauses(t *testing.T) {
	dir := t.TempDir()
	sql := `
CREATE TABLE IF NOT EXISTS fleet_partner_test_table (
    id text PRIMARY KEY,
    email text NOT NULL,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','CLOSED')),
    version bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    CONSTRAINT fk_something FOREIGN KEY (id) REFERENCES other_table (id),
    UNIQUE (id, email)
);
`
	if err := os.WriteFile(filepath.Join(dir, "000001_test.up.sql"), []byte(sql), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := BuildSensitiveColumnInventory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Column != "email" {
		t.Fatalf("entries = %+v, want exactly one email column", entries)
	}
}

// TestBuildSensitiveColumnInventory_HandlesMultipleColumnsPerLine pins a
// real bug the first draft of this parser had: this schema commonly
// declares more than one column on the same physical line (e.g.
// migrations 000035 and 000044's "latitude numeric(9,6), longitude
// numeric(9,6), accuracy numeric(10,3),"). A line-based split silently
// returned only the first column on such a line -- longitude was missing
// from the generated inventory for both real tables until this was caught
// by reviewing the output against the source migrations and the parser was
// rewritten to split on top-level commas across the whole body instead.
func TestBuildSensitiveColumnInventory_HandlesMultipleColumnsPerLine(t *testing.T) {
	dir := t.TempDir()
	sql := `
CREATE TABLE IF NOT EXISTS fleet_partner_test_table (
    id text PRIMARY KEY,
    latitude numeric(9,6), longitude numeric(9,6), accuracy numeric(10,3),
    created_at timestamptz NOT NULL DEFAULT now()
);
`
	if err := os.WriteFile(filepath.Join(dir, "000001_test.up.sql"), []byte(sql), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := BuildSensitiveColumnInventory(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Column] = true
	}
	if !got["latitude"] || !got["longitude"] {
		t.Fatalf("entries = %+v, want both latitude and longitude classified LOCATION", entries)
	}
}

// TestBuildSensitiveColumnInventory_StripsLineComments proves an
// explanatory "-- comment" line between column definitions (common
// throughout this schema's migrations) does not get misread as, or corrupt
// the parsing of, the column definition it precedes.
func TestBuildSensitiveColumnInventory_StripsLineComments(t *testing.T) {
	dir := t.TempDir()
	sql := `
CREATE TABLE IF NOT EXISTS fleet_partner_test_table (
    id text PRIMARY KEY,
    -- The corporate identity's primary contact email.
    email text NOT NULL,
    -- Bumped whenever the row changes.
    version bigint NOT NULL DEFAULT 1
);
`
	if err := os.WriteFile(filepath.Join(dir, "000001_test.up.sql"), []byte(sql), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := BuildSensitiveColumnInventory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Column != "email" || entries[0].Category != CategoryContactPII {
		t.Fatalf("entries = %+v, want exactly one CONTACT_PII email column", entries)
	}
}

// TestBuildSensitiveColumnInventory_HandlesMultiColumnAlter proves the
// parser correctly attributes each column in a single
// "ALTER TABLE t ADD COLUMN a ..., ADD COLUMN b ..." statement -- the exact
// style migration 000069 (audit chaining) and 000005-era additive
// migrations use.
func TestBuildSensitiveColumnInventory_HandlesMultiColumnAlter(t *testing.T) {
	dir := t.TempDir()
	sql := `
ALTER TABLE fleet_partner_test_table
    ADD COLUMN IF NOT EXISTS bank_account_number text,
    ADD COLUMN IF NOT EXISTS notes text,
    ADD COLUMN IF NOT EXISTS otp_hash text;
`
	if err := os.WriteFile(filepath.Join(dir, "000001_test.up.sql"), []byte(sql), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := BuildSensitiveColumnInventory(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.Column] = e.Category
	}
	if got["bank_account_number"] != CategoryFinancial {
		t.Errorf("bank_account_number category = %q, want %q", got["bank_account_number"], CategoryFinancial)
	}
	if got["otp_hash"] != CategoryCredential {
		t.Errorf("otp_hash category = %q, want %q", got["otp_hash"], CategoryCredential)
	}
	if _, ok := got["notes"]; ok {
		t.Errorf("notes should not be classified as sensitive")
	}
}
