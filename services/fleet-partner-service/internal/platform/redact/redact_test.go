package redact

import "testing"

func TestIsSensitiveKey_MatchesCommonVariantsCaseInsensitively(t *testing.T) {
	sensitive := []string{
		"password", "Password", "passcode", "token", "access_token", "refreshToken",
		"Authorization", "cookie", "secret", "api_key", "apiKey", "otp", "mfa_code",
		"nin", "bvn", "bank_account", "bankReference", "account_number", "card_number",
		"cardHolderName", "cvv", "signed_url", "signedUrl", "passport", "ssn", "document",
	}
	for _, key := range sensitive {
		if !IsSensitiveKey(key) {
			t.Errorf("expected %q to be classified sensitive", key)
		}
	}
}

func TestIsSensitiveKey_DoesNotFlagOrdinaryFields(t *testing.T) {
	safe := []string{"route", "status", "organization_id", "resource_type", "occurred_at", "action"}
	for _, key := range safe {
		if IsSensitiveKey(key) {
			t.Errorf("expected %q to NOT be classified sensitive", key)
		}
	}
}
