package internal

// Section 14 guard: notification-service delivers what others decided. It
// holds no readiness, recovery, matching or capability logic of its own.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotificationHoldsNoDecisionEngine(t *testing.T) {
	forbidden := []string{"capabilityfit", "providercompat.Select", "recovery_cases", "readiness_rules",
		"SelectReplacement", "PredictReady", "commercequote", "dispatchpricing", "walletclient"}
	count := 0
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		count++
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, marker := range forbidden {
			if strings.Contains(string(content), marker) {
				t.Errorf("%s contains %q: notification delivers decisions, it does not make them", path, marker)
			}
		}
		return nil
	})
	if err != nil || count < 5 {
		t.Fatalf("walk: %d files, %v", count, err)
	}
}
