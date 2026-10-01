package internal

// Section 26 static audit: media-file-service stores and binds evidence; it
// never decides what the evidence proves. Certification, eligibility,
// matching and money belong to verification-compliance-service, the execution
// services and payment-wallet-service. Media answers "is this object bound to
// that owner and purpose", nothing more.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaDecidesNoCertificationEligibilityOrMatch(t *testing.T) {
	forbidden := []string{
		"capabilityfit", "logisticscapability", "providercompat", "walletclient",
		"commercequote", "dispatchpricing", "EligibilityEligible", "\"CERTIFIED\"",
		"verification_status", "SelectReplacement", "/certification-decisions",
	}
	count := 0
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		count++
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, marker := range forbidden {
			if strings.Contains(string(raw), marker) {
				t.Errorf("%s contains %q: media binds evidence, it does not decide what it proves", path, marker)
			}
		}
		return nil
	})
	if err != nil || count < 5 {
		t.Fatalf("walk: %d files, %v", count, err)
	}
}
