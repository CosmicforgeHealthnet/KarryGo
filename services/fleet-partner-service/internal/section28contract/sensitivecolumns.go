// Package section28contract also hosts a mechanical sensitive-column
// inventory over every migration's actual CREATE TABLE / ALTER TABLE ADD
// COLUMN statements -- Section 28 item asks for a machine-readable
// inventory across the whole schema (Sections 1-28, not only 13-28: the
// classifier has no way to know which section introduced a table, and a
// column's sensitivity does not depend on which section added it).
//
// This is deliberately the same two-layer design internal/platform/
// riskregistry already established for route classification: a mechanical
// first pass (keyword-in-column-name, priority-ordered categories) plus a
// small, individually-reasoned overrides list for the cases the mechanical
// pass gets wrong (a false positive to suppress, or a column that needs a
// category no keyword would ever imply). Both are combined, then diffed
// against the committed JSON inventory (sensitive_columns_test.go) so the
// inventory cannot silently drift from the schema the way a hand-written
// document would.
//
// This is a naming-convention inventory, not a data-flow analysis: it
// cannot know that a column named "ref" holds a bank account number if
// nothing about the name says so. It is a starting point for a real
// encryption/tokenization/access-review pass, not a substitute for one --
// see docs/section28-security-privacy-audit.md's "Sensitive column
// inventory" section for that disclosure in one place a reader will
// actually find it.
package section28contract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Category is a sensitivity classification. Ordered most- to
// least-sensitive; ClassifyColumn returns the FIRST category (in this
// priority order) whose keyword set matches, so a column matching both
// "token" and "reference" (say, "refresh_token_reference") is CREDENTIAL,
// not the less sensitive category a later, looser keyword might also match.
const (
	CategoryCredential   = "CREDENTIAL"    // secrets, tokens, OTPs, password/PIN hashes
	CategoryGovernmentID = "GOVERNMENT_ID" // NIN, BVN, passport, driver's licence, tax ID
	CategoryFinancial    = "FINANCIAL"     // bank/card details and monetary amounts
	CategoryBiometric    = "BIOMETRIC"     // face scans, fingerprints
	CategoryLocation     = "LOCATION"      // GPS coordinates / live tracking
	CategoryContactPII   = "CONTACT_PII"   // name, email, phone, address, date of birth
)

// categoryKeywords is checked in the declared order -- do not reorder
// without re-checking every existing inventory entry's category, since
// reordering can silently reclassify columns that match more than one set.
var categoryOrder = []string{
	CategoryCredential, CategoryGovernmentID, CategoryFinancial,
	CategoryBiometric, CategoryLocation, CategoryContactPII,
}

var categoryKeywords = map[string][]string{
	CategoryCredential: {
		"password", "passcode", "secret", "token", "otp_hash", "otp_code",
		"api_key", "apikey", "private_key", "signing_key", "hmac_key",
		"credential", "pin_hash", "pin_code", "session_hash", "auth_hash",
		"signed_url", "webhook_secret", "chain_secret",
	},
	CategoryGovernmentID: {
		"nin", "bvn", "passport", "drivers_license", "driver_license",
		"driving_license", "tax_id", "tin_number", "national_id",
		"registration_number", "license_number", "licence_number",
	},
	CategoryFinancial: {
		"bank_account", "account_number", "card_number", "card_cvv", "cvv",
		"iban", "swift_code", "routing_number", "sort_code",
		"payout_reference", "settlement_reference", "wallet_reference",
		"_kobo", "amount_", "fare_", "invoice_number", "payment_reference",
	},
	CategoryBiometric: {
		// Deliberately NOT "fingerprint": every "*_fingerprint" column in this
		// schema (request_fingerprint, content_fingerprint,
		// approved_fingerprint, etc. -- ~20 of them) is a change-detection
		// hash of a REQUEST PAYLOAD, confirmed by reading the migrations that
		// introduce them (e.g. 000020_approval_execution_binding.up.sql: "The
		// fingerprint the approver saw."). Including it produced 20 false
		// positives the first time this inventory was generated; this is
		// software-engineering "fingerprint" (a content hash), not biometric
		// data, everywhere in this codebase.
		"biometric", "face_scan", "facial_", "liveness",
	},
	CategoryLocation: {
		"latitude", "longitude", "_lat", "_lng", "gps_", "geo_point",
		"current_location", "live_location",
	},
	CategoryContactPII: {
		"email", "phone", "_address", "full_name", "first_name", "last_name",
		"date_of_birth", "_dob", "next_of_kin", "emergency_contact",
		"guarantor",
	},
}

// Disposition is Section 28's required storage/handling classification for
// every inventoried column. Assigned mechanically from the column's
// Category plus a small set of naming conventions this schema already
// follows consistently (a "*_hash" suffix always means an already-hashed
// value; a "*_reference" FINANCIAL column always means an opaque pointer
// into payment-wallet-service, never the payment data itself -- both
// confirmed by reading the actual columns this inventory found, not
// assumed from the name alone).
const (
	DispositionSafeReference      = "SAFE_REFERENCE"
	DispositionTokenizedReference = "TOKENIZED_REFERENCE"
	DispositionHashed             = "HASHED"
	DispositionEncrypted          = "ENCRYPTED"
	DispositionRedactedOnly       = "REDACTED_ONLY"
	DispositionRemoveAndMigrate   = "REMOVE_AND_MIGRATE"
	DispositionApprovedPlaintext  = "APPROVED_PLAINTEXT"
)

// ColumnEntry is one inventoried column.
type ColumnEntry struct {
	Table          string `json:"table"`
	Column         string `json:"column"`
	Category       string `json:"category"`
	SourceFile     string `json:"sourceFile"`
	AuditRedacted  bool   `json:"auditRedacted"`
	Disposition    string `json:"disposition"`
	Justification  string `json:"justification"`
	OverrideReason string `json:"overrideReason,omitempty"`
}

// ClassifyDisposition assigns a Disposition and a written Justification
// (mandatory for APPROVED_PLAINTEXT per Section 28) to one column, from its
// table/column name and already-assigned Category. Every branch is a real,
// checkable architectural fact about this codebase (confirmed by reading
// the actual migrations and platform doc comments this pass also produced),
// not a generic template answer.
func ClassifyDisposition(table, column, category string) (disposition, justification string) {
	lower := strings.ToLower(column)

	if strings.HasSuffix(lower, "_hash") {
		return DispositionHashed,
			"already stored as a one-way hash (HMAC or equivalent); the plaintext value is never persisted, so there is nothing to encrypt or tokenize -- only equality comparison is ever needed."
	}

	switch category {
	case CategoryCredential:
		// Every CREDENTIAL-category column this inventory actually found is
		// already a "*_hash" column and returned above; this branch exists
		// as a fail-closed backstop, not because a real case reaches it --
		// a NEW credential-shaped column added later that is NOT already
		// hashed must be flagged for real remediation, never defaulted to
		// APPROVED_PLAINTEXT.
		return DispositionRemoveAndMigrate,
			"a CREDENTIAL-category column that is not already a hash -- raw credentials must never be stored; remove and reissue through the owning auth flow instead of storing a value here at all."

	case CategoryGovernmentID:
		if strings.Contains(lower, "registration_number") {
			return DispositionSafeReference,
				"a business/company registration number (e.g. Nigerian CAC number) -- routinely disclosed on invoices, letterheads and public company registries; not a secret, and not an individual's personal government ID (NIN/BVN/passport, none of which this schema currently stores)."
		}
		return DispositionEncrypted,
			"a personal government-identity value -- recoverable encryption is required (see EncryptionKeyProvider), not plaintext, hashing (the business needs to display/verify the actual value, not just compare it) or a bare reference."

	case CategoryFinancial:
		if strings.Contains(lower, "reference") {
			return DispositionSafeReference,
				"an opaque reference string into payment-wallet-service (or another owning service's) records -- confirmed by reading the actual column usage; the sensitive payment/settlement data itself is never stored in fleet-partner-service's own tables, only pointed at, per this platform's service-ownership rules (payments/wallets/ledger belong exclusively to payment-wallet-service)."
		}
		return DispositionApprovedPlaintext,
			"an operational monetary amount (order/invoice/budget/threshold value, stored as a minor-unit integer) the business must read, sum and display directly in its own domain logic -- not a payment credential, bank detail or card number. Those are owned exclusively by payment-wallet-service and are never stored in this table."

	case CategoryLocation:
		return DispositionApprovedPlaintext,
			"GPS coordinates required in plaintext for real-time routing, dispatch and proof-of-delivery -- encrypting a value every routing query must read would make the routing engine itself a decryption client for every request. Transactional/operational data retained under the OPERATIONAL or TRIP_EXECUTION retention class, not a standing personal-location profile."

	case CategoryContactPII:
		if table == "fleet_partner_organization_authorized_representatives" {
			return DispositionApprovedPlaintext,
				"operationally required in plaintext while the organization is open (email/phone used to actually contact the representative; full_name displayed on documents). Unlike every other CONTACT_PII column in this inventory, this one already has a REAL, IMPLEMENTED AND TESTED post-closure remediation path: internal/platform/retentionexec's OrgContactExecutor anonymizes exactly these fields once the organization is CLOSED and its effective OPERATIONAL retention window has elapsed (TestRetentionExecution_ExecuteAnonymizesThroughJobHandler) -- this is not a bare policy statement."
		}
		return DispositionApprovedPlaintext,
			"operationally required in plaintext to actually contact the customer/staff member or display their name on documents/UI -- encrypting it would make every read path (search, display, outbound email/SMS) a decryption client. No executable anonymization path exists yet for this specific table (unlike the organization-representative case above); real, disclosed remaining work, not silently accepted as permanent plaintext with no plan."

	default:
		return DispositionApprovedPlaintext,
			"no naming-convention rule matched a more specific disposition; defaulted to APPROVED_PLAINTEXT pending a manual review that has not yet happened for this specific column. Disclosed as a real gap, not a considered decision."
	}
}

// columnOverride adjusts a mechanically-classified column, or classifies one
// the keyword pass misses entirely, with a human-reasoned justification.
// Applied after the mechanical pass so a later regeneration never silently
// discards a reasoned decision.
type columnOverride struct {
	Table, Column, Category, Reason string
	// Suppress removes the column from the inventory entirely (a keyword
	// false positive -- the column name matches a sensitive keyword but does
	// not hold sensitive data).
	Suppress bool
}

// overrides holds the individually-reasoned adjustments to the mechanical
// pass -- currently empty. The mechanism (Suppress + reclassify) exists and
// is exercised by TestBuildSensitiveColumnInventory tests in the sibling
// _test.go file; no entry is added here speculatively for a column that
// does not exist in this schema today. Add a real entry the next time the
// mechanical pass gets a real column wrong, with the same reasoning
// discipline internal/platform/riskregistry's overrides list uses.
var overrides = []columnOverride{}

// isColumnDefinitionSegment reports whether a top-level, comma-separated
// segment of a CREATE TABLE (...) body defines a column (as opposed to a
// table-level CONSTRAINT/CHECK/PRIMARY KEY/UNIQUE/FOREIGN KEY/EXCLUDE
// clause, which share the same comma-separated shape but are not columns).
var tableLevelClausePrefixes = []string{
	"constraint", "check", "primary key", "unique", "foreign key", "exclude",
}

var identifierPattern = regexp.MustCompile(`^"?([a-zA-Z_][a-zA-Z0-9_]*)"?`)

// stripLineComments removes "-- ..." SQL line comments, which are common
// inside this schema's CREATE TABLE bodies (explanatory comments between or
// above column definitions) and would otherwise corrupt a segment that
// happens to start with one after whitespace trimming.
func stripLineComments(sql string) string {
	lines := strings.Split(sql, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "--"); idx != -1 {
			lines[i] = line[:idx]
		}
	}
	return strings.Join(lines, "\n")
}

// splitTopLevel splits body on commas that are NOT nested inside parens
// (CHECK(...), REFERENCES table(id), NUMERIC(9,6) all contain commas that
// must not split a column definition in two). This -- not a line-by-line
// split -- is what correctly handles this schema's real, common style of
// declaring more than one column on the same physical line (e.g.
// "latitude numeric(9,6), longitude numeric(9,6), accuracy numeric(10,3),"
// in migrations 000035 and 000044): a line-based split would silently
// return only the FIRST column on such a line, a real bug this inventory's
// first draft had until it was caught by manually reviewing the generated
// output against the source migrations.
func splitTopLevel(body string) []string {
	var segments []string
	depth := 0
	start := 0
	for i, r := range body {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				segments = append(segments, body[start:i])
				start = i + 1
			}
		}
	}
	segments = append(segments, body[start:])
	return segments
}

func isColumnDefinitionSegment(segment string) (string, bool) {
	trimmed := strings.TrimSpace(segment)
	if trimmed == "" {
		return "", false
	}
	lower := strings.ToLower(trimmed)
	for _, prefix := range tableLevelClausePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return "", false
		}
	}
	match := identifierPattern.FindStringSubmatch(trimmed)
	if match == nil {
		return "", false
	}
	return match[1], true
}

var createTablePattern = regexp.MustCompile(`(?is)CREATE TABLE(?:\s+IF NOT EXISTS)?\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)

// extractCreateTableColumns finds every CREATE TABLE (...) block in sql and
// returns (table, column) pairs for every column definition inside.
// Matches the closing paren by depth-counting rather than a non-greedy
// regex, since column definitions themselves contain parens (CHECK(...),
// REFERENCES table(id), NUMERIC(10,2)).
func extractCreateTableColumns(sql string) []struct{ Table, Column string } {
	sql = stripLineComments(sql)
	var results []struct{ Table, Column string }
	for _, loc := range createTablePattern.FindAllStringSubmatchIndex(sql, -1) {
		table := sql[loc[2]:loc[3]]
		openParenIdx := loc[1] - 1 // the '(' the pattern matched up to
		depth := 0
		end := -1
		for i := openParenIdx; i < len(sql); i++ {
			switch sql[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = i
				}
			}
			if end != -1 {
				break
			}
		}
		if end == -1 {
			continue // malformed/truncated -- skip rather than misparse
		}
		body := sql[openParenIdx+1 : end]
		for _, segment := range splitTopLevel(body) {
			if column, ok := isColumnDefinitionSegment(segment); ok {
				results = append(results, struct{ Table, Column string }{table, column})
			}
		}
	}
	return results
}

var alterAddColumnPattern = regexp.MustCompile(`(?i)ALTER TABLE\s+(?:IF EXISTS\s+)?([a-zA-Z_][a-zA-Z0-9_]*)\s+(ADD COLUMN[\s\S]*?)(?:;|$)`)
var addColumnClausePattern = regexp.MustCompile(`(?i)ADD COLUMN(?:\s+IF NOT EXISTS)?\s+([a-zA-Z_][a-zA-Z0-9_]*)`)

// extractAlterTableColumns finds every "ALTER TABLE t ADD COLUMN ..."
// statement (including the common multi-column "ADD COLUMN a ..., ADD
// COLUMN b ..." form this schema's later migrations use) and returns
// (table, column) pairs.
func extractAlterTableColumns(sql string) []struct{ Table, Column string } {
	sql = stripLineComments(sql)
	var results []struct{ Table, Column string }
	for _, m := range alterAddColumnPattern.FindAllStringSubmatch(sql, -1) {
		table := m[1]
		for _, colMatch := range addColumnClausePattern.FindAllStringSubmatch(m[2], -1) {
			results = append(results, struct{ Table, Column string }{table, colMatch[1]})
		}
	}
	return results
}

// ClassifyColumn returns the category a column name mechanically matches,
// or "" if none of the keyword sets match.
//
// A keyword containing "_" (e.g. "bank_account", "_kobo", "amount_") is
// matched as an anchored substring of the whole lowercased column name --
// safe, because the underscore already ties it to snake_case token
// boundaries. A bare, underscore-free keyword (e.g. "nin", "bvn", "email",
// "token") is instead matched against WHOLE underscore-split tokens only,
// never as a raw substring: a naive substring match on "nin" matches inside
// "warNINgs", "traINING" and "owNINg_service", none of which have anything
// to do with a National Identification Number. This was found, not
// theorized -- the first generated inventory flagged exactly those three
// real columns before this fix.
func ClassifyColumn(column string) string {
	lower := strings.ToLower(column)
	tokens := strings.Split(lower, "_")
	tokenSet := make(map[string]bool, len(tokens))
	for _, tok := range tokens {
		tokenSet[tok] = true
	}
	for _, category := range categoryOrder {
		for _, keyword := range categoryKeywords[category] {
			if strings.Contains(keyword, "_") {
				if strings.Contains(lower, keyword) {
					return category
				}
				continue
			}
			if tokenSet[keyword] {
				return category
			}
		}
	}
	return ""
}

// isAuditRedacted mirrors internal/platform/redact.IsSensitiveKey's
// substring list, duplicated rather than imported: redact's list is scoped
// to what must never appear unredacted in a LOG LINE or audit summary
// (a narrower, deliberately conservative set), while this inventory's
// categoryKeywords are scoped to "is this column sensitive DATA at all"
// (broader -- e.g. GPS coordinates are inventoried as LOCATION but are not
// on redact's log-redaction list). Importing redact here would wrongly
// suggest the two lists are meant to be the same list.
var auditRedactedSubstrings = []string{
	"password", "passcode", "token", "authorization", "cookie", "secret",
	"api_key", "apikey", "otp", "mfa", "nin", "bvn", "bank",
	"account_number", "card", "cvv", "signed_url", "signedurl",
	"passport", "ssn", "document",
}

func isAuditRedacted(column string) bool {
	lower := strings.ToLower(column)
	for _, needle := range auditRedactedSubstrings {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// BuildSensitiveColumnInventory parses every *.up.sql file in
// migrationsDir, in filename order (so SourceFile reflects the migration
// that first introduced each column, matching git-blame-style provenance),
// classifies every column, applies overrides, and returns the sorted
// inventory of columns that ended up with a non-empty category.
func BuildSensitiveColumnInventory(migrationsDir string) ([]ColumnEntry, error) {
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil {
		return nil, fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(files)

	// first-seen wins: a column added by an early CREATE TABLE and later
	// referenced again (it never is in this schema, but defensively) keeps
	// its original source file.
	seen := map[string]ColumnEntry{}
	key := func(table, column string) string { return table + "." + column }

	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		sql := string(raw)
		base := filepath.Base(file)

		var pairs []struct{ Table, Column string }
		pairs = append(pairs, extractCreateTableColumns(sql)...)
		pairs = append(pairs, extractAlterTableColumns(sql)...)

		for _, pair := range pairs {
			k := key(pair.Table, pair.Column)
			if _, exists := seen[k]; exists {
				continue
			}
			category := ClassifyColumn(pair.Column)
			if category == "" {
				continue
			}
			seen[k] = ColumnEntry{
				Table: pair.Table, Column: pair.Column, Category: category,
				SourceFile: base, AuditRedacted: isAuditRedacted(pair.Column),
			}
		}
	}

	for _, override := range overrides {
		k := key(override.Table, override.Column)
		if override.Suppress {
			// Table "" matches by column name alone, for a false positive
			// that recurs across many tables (e.g. every "token_type" column
			// regardless of which table it is on).
			if override.Table == "" {
				for existingKey, entry := range seen {
					if entry.Column == override.Column {
						delete(seen, existingKey)
					}
				}
				continue
			}
			delete(seen, k)
			continue
		}
		entry, exists := seen[k]
		if !exists {
			entry = ColumnEntry{Table: override.Table, Column: override.Column}
		}
		entry.Category = override.Category
		entry.OverrideReason = override.Reason
		seen[k] = entry
	}

	entries := make([]ColumnEntry, 0, len(seen))
	for _, entry := range seen {
		entry.Disposition, entry.Justification = ClassifyDisposition(entry.Table, entry.Column, entry.Category)
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Table != entries[j].Table {
			return entries[i].Table < entries[j].Table
		}
		return entries[i].Column < entries[j].Column
	})
	return entries, nil
}
