package internal_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isScannerFile reports whether a file is one of the architecture scanners.
// They necessarily contain the forbidden token list, so scanning them would
// always self-trip.
func isScannerFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, "architecture") && strings.HasSuffix(base, "_test.go")
}

// sqlWithoutComments strips `--` line comments so prose explaining a
// prohibition ("no password column exists", "never stores a balance") is not
// mistaken for the prohibited declaration itself.
func sqlWithoutComments(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// goWithoutComments strips // line comments for the same reason.
func goWithoutComments(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// TestNoPasswordAuthentication fails the build if any password-based
// authentication concept is introduced into corporate auth.
//
// Infrastructure credentials (database URLs, SMTP logins, service secrets)
// are legitimate and are deliberately not matched: the forbidden tokens below
// are user-credential concepts only.
func TestNoPasswordAuthentication(t *testing.T) {
	forbidden := []string{
		"password_hash", "passwordHash", "PasswordHash",
		"temporary_password", "temporaryPassword", "TemporaryPassword",
		"new_password", "newPassword", "NewPassword",
		"old_password", "oldPassword", "OldPassword",
		"passwordConfirmation", "password_confirmation",
		"ChangePasswordOnFirstLogin", "security_question", "securityQuestion",
	}

	for _, file := range collectGoFiles(t, serviceRoot, true) {
		if isScannerFile(file) {
			continue
		}
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := goWithoutComments(string(content))
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Errorf("%s contains password-authentication token %q", file, token)
			}
		}
	}

	// Migrations must not create any password column either.
	migrations, _ := filepath.Glob(filepath.Join(serviceRoot, "migrations", "*.sql"))
	for _, file := range migrations {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		lowered := strings.ToLower(sqlWithoutComments(string(content)))
		for _, token := range []string{"password_hash", "password_digest", "password text", "password varchar"} {
			if strings.Contains(lowered, token) {
				t.Errorf("%s declares a password column (%q)", file, token)
			}
		}
	}
}

// TestNoPasswordLoginRoute asserts no password-based login endpoint exists.
func TestNoPasswordLoginRoute(t *testing.T) {
	for _, file := range collectGoFiles(t, serviceRoot, false) {
		if isScannerFile(file) {
			continue
		}
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		// The corporate API exposes /auth/start and /auth/verify only; a
		// bare /auth/login would imply a credential exchange.
		if strings.Contains(text, `"/auth/login"`) || strings.Contains(text, `"/login"`) {
			t.Errorf("%s mounts a password-style login route", file)
		}
	}
}

// TestNoRawSecretStorage asserts OTP codes, refresh tokens and invitation
// tokens are only ever persisted as digests.
func TestNoRawSecretStorage(t *testing.T) {
	migrations, _ := filepath.Glob(filepath.Join(serviceRoot, "migrations", "*.up.sql"))
	for _, file := range migrations {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		lowered := strings.ToLower(sqlWithoutComments(string(content)))
		// Forbidden raw columns.
		for _, token := range []string{"otp_code", "raw_code", "code_plain",
			"refresh_token text", "invitation_token text", "access_token text"} {
			if strings.Contains(lowered, token) {
				t.Errorf("%s stores a raw secret column (%q)", file, token)
			}
		}
	}

	// The challenge and session tables must use digest/hash columns.
	authMigration := filepath.Join(serviceRoot, "migrations", "000003_corporate_authentication.up.sql")
	content, err := os.ReadFile(authMigration)
	if err != nil {
		t.Fatalf("read %s: %v", authMigration, err)
	}
	text := string(content)
	for _, required := range []string{"code_digest", "refresh_token_hash", "email_hash", "phone_hash"} {
		if !strings.Contains(text, required) {
			t.Errorf("auth migration is missing hashed column %q", required)
		}
	}
}

// TestNoSMSOTPDelivery asserts the corporate OTP is emailed, never texted.
func TestNoSMSOTPDelivery(t *testing.T) {
	authDirs := []string{
		filepath.Join(serviceRoot, "internal", "features", "auth"),
		filepath.Join(serviceRoot, "internal", "features", "staff"),
		filepath.Join(serviceRoot, "internal", "features", "providers"),
	}
	for _, dir := range authDirs {
		for _, file := range collectGoFiles(t, dir, false) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			lowered := strings.ToLower(goWithoutComments(string(content)))
			for _, token := range []string{"channelsms", "\"sms\"", "send_sms", "sendsms", "twilio"} {
				if strings.Contains(lowered, token) {
					t.Errorf("%s requests SMS delivery; corporate OTP is email-only", file)
				}
			}
		}
	}
}

// TestNoLocalProviderIdentitySourceOfTruth asserts the provider feature never
// stores provider credentials or claims provider identity ownership.
func TestNoLocalProviderIdentitySourceOfTruth(t *testing.T) {
	dir := filepath.Join(serviceRoot, "internal", "features", "providers")
	for _, file := range collectGoFiles(t, dir, false) {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		lowered := strings.ToLower(goWithoutComments(string(content)))
		for _, token := range []string{"provider_password", "providersecret", "issueprovidercredential"} {
			if strings.Contains(lowered, token) {
				t.Errorf("%s implies local provider credential ownership (%q)", file, token)
			}
		}
	}

	// The provider migration must keep provider_id as an opaque reference
	// with no foreign key into another service.
	migration := filepath.Join(serviceRoot, "migrations", "000007_managed_providers.up.sql")
	content, err := os.ReadFile(migration)
	if err != nil {
		t.Fatalf("read %s: %v", migration, err)
	}
	text := string(content)
	if strings.Contains(text, "provider_id text NOT NULL REFERENCES") ||
		strings.Contains(text, "provider_id text REFERENCES") {
		t.Error("provider_id must stay an opaque external reference, never a foreign key")
	}
}

// TestNoLocalFinancialLedger asserts no balance or ledger column was added.
func TestNoLocalFinancialLedger(t *testing.T) {
	migrations, _ := filepath.Glob(filepath.Join(serviceRoot, "migrations", "*.up.sql"))
	for _, file := range migrations {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		lowered := strings.ToLower(sqlWithoutComments(string(content)))
		// Column-shaped tokens only: a comment explaining that balances are
		// NOT stored here must not trip the check.
		for _, token := range []string{
			"balance bigint", "balance numeric", "balance_minor", "available_balance",
			"ledger_entry", "ledger_entries", "debit_minor", "credit_minor",
		} {
			if strings.Contains(lowered, token) {
				t.Errorf("%s introduces a local financial ledger concept (%q)", file, token)
			}
		}
	}
}
