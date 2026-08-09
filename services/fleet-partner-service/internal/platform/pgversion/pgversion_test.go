package pgversion

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMajorVersionPattern_ParsesRepresentativeStrings(t *testing.T) {
	cases := map[string]string{
		"16.14":                "16",
		"16.14 (Debian 16.14)": "16",
		"12.22":                "12",
		"9.6.24":               "9",
	}
	for input, want := range cases {
		match := majorVersionPattern.FindStringSubmatch(input)
		if match == nil {
			t.Errorf("no match for %q", input)
			continue
		}
		if match[1] != want {
			t.Errorf("majorVersionPattern(%q) = %q, want %q", input, match[1], want)
		}
	}
}

// TestMinimumSupportedMajor_IsAtLeastGenRandomUUIDRequirement pins the
// documented rationale (gen_random_uuid() built-in requires PostgreSQL 13+)
// to the actual constant, so the constant cannot silently drift below the
// evidence its own doc comment cites.
func TestMinimumSupportedMajor_IsAtLeastGenRandomUUIDRequirement(t *testing.T) {
	const genRandomUUIDBuiltinMinimum = 13
	if MinimumSupportedMajor < genRandomUUIDBuiltinMinimum {
		t.Errorf("MinimumSupportedMajor = %d, but gen_random_uuid() is used directly (no pgcrypto extension) which requires PostgreSQL %d+",
			MinimumSupportedMajor, genRandomUUIDBuiltinMinimum)
	}
}

// TestTestedMajor_MeetsMinimum guards against the two constants drifting
// out of order (a "tested" version below the documented "minimum" would be
// self-contradictory).
func TestTestedMajor_MeetsMinimum(t *testing.T) {
	if TestedMajor < MinimumSupportedMajor {
		t.Errorf("TestedMajor (%d) is below MinimumSupportedMajor (%d)", TestedMajor, MinimumSupportedMajor)
	}
}

func requireTestDatabaseName(t *testing.T, rawURL string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("FLEET_PARTNER_TEST_DATABASE_URL is not a valid URL")
	}
	name := strings.ToLower(strings.TrimPrefix(parsed.Path, "/"))
	if !strings.Contains(name, "test") {
		t.Fatalf("refusing to run: database name %q does not look test-specific (must contain \"test\")", name)
	}
}

// TestRequireSupported_RealTestDatabaseMeetsMinimum is the live half of
// WP11: connects to the actual disposable test database and proves both
// that ServerMajorVersion correctly reports it and that RequireSupported
// passes against it -- this repository's real development/CI PostgreSQL,
// not a simulated version string.
func TestRequireSupported_RealTestDatabaseMeetsMinimum(t *testing.T) {
	rawURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL not set; skipping live PostgreSQL version check")
	}
	requireTestDatabaseName(t, rawURL)

	pool, err := pgxpool.New(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	major, full, err := ServerMajorVersion(context.Background(), pool)
	if err != nil {
		t.Fatalf("ServerMajorVersion: %v", err)
	}
	t.Logf("connected test database reports server_version=%q major=%d", full, major)
	if major < MinimumSupportedMajor {
		t.Errorf("live test database major version %d is below MinimumSupportedMajor %d", major, MinimumSupportedMajor)
	}

	if err := RequireSupported(context.Background(), pool); err != nil {
		t.Errorf("RequireSupported against the real test database: %v", err)
	}
}

// TestCheckMajorSupported_BelowMinimumFails proves the failure path: a
// below-minimum major version produces a non-nil, actionable error naming
// both the offending version and the minimum. This exercises the exact
// comparison RequireSupported performs against a real connection, without
// needing an actual below-minimum PostgreSQL server to connect to.
func TestCheckMajorSupported_BelowMinimumFails(t *testing.T) {
	err := CheckMajorSupported(9, "9.6.24 (fake, for the below-minimum test path)")
	if err == nil {
		t.Fatal("expected a non-nil error for a below-minimum version")
	}
	if !strings.Contains(err.Error(), "9.6.24") || !strings.Contains(err.Error(), "13") {
		t.Errorf("error message should name both the offending version and the minimum, got: %v", err)
	}
}

// TestCheckMajorSupported_AtOrAboveMinimumPasses proves the boundary and
// above-boundary cases both succeed.
func TestCheckMajorSupported_AtOrAboveMinimumPasses(t *testing.T) {
	for _, major := range []int{MinimumSupportedMajor, MinimumSupportedMajor + 1, TestedMajor} {
		if err := CheckMajorSupported(major, fmt.Sprintf("%d.0", major)); err != nil {
			t.Errorf("CheckMajorSupported(%d, ...) = %v, want nil", major, err)
		}
	}
}
