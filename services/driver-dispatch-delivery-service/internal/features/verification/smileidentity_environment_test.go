package verification

import (
	"context"
	"errors"
	"testing"
)

func TestSmileIdentityFakeModeRestrictedToDevelopmentAndTest(t *testing.T) {
	for _, tc := range []struct {
		name       string
		appEnv     string
		permitFake bool
	}{
		{"development", "development", true},
		{"unit test", "test", true},
		{"staging", "staging", false},
		{"production", "production", false},
		{"case variant", "Production", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SMILE_IDENTITY_FAKE_MODE", "pass")
			t.Setenv("SMILE_IDENTITY_API_KEY", "")
			t.Setenv("SMILE_IDENTITY_PARTNER_ID", "")
			t.Setenv("SMILE_IDENTITY_BASE_URL", "")
			client := NewSmileIdentityClientFromEnv(tc.appEnv)
			result, err := client.MatchFace(context.Background(), "selfie", "id")
			if tc.permitFake {
				if err != nil || !result.Passed {
					t.Fatalf("expected test-only fake match: result=%+v err=%v", result, err)
				}
				return
			}
			if result.Passed || !errors.Is(err, ErrSmileIdentityNotConfigured) {
				t.Fatalf("unsafe fake approval for %q: result=%+v err=%v", tc.appEnv, result, err)
			}
		})
	}
}
