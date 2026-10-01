package serviceapp

import (
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/logging"
)

// An unstamped binary must say so rather than report an empty string. A
// missing key and an empty value look identical to a deployment script, and
// "did the pipeline stamp this?" is the one question that must not be
// ambiguous.
func TestUnstampedBuildReportsUnknownRatherThanEmpty(t *testing.T) {
	t.Setenv("BUILD_COMMIT_SHA", "")
	t.Setenv("BUILD_TIME", "")
	t.Setenv("BUILD_VERSION", "")
	t.Setenv("APP_ENV", "")

	info := CurrentBuildInfo("driver-taxi-service")
	if info.Service != "driver-taxi-service" {
		t.Fatalf("service = %q", info.Service)
	}
	for name, value := range map[string]string{
		"version":     info.Version,
		"build time":  info.BuildTime,
		"environment": info.Environment,
	} {
		if value == "" {
			t.Fatalf("%s is empty; it must be explicit", name)
		}
	}
	// CommitSHA may legitimately resolve from Go's own VCS stamp when the
	// build has a .git directory, so only its emptiness is asserted.
	if info.CommitSHA == "" {
		t.Fatal("commit sha is empty; it must be explicit")
	}
}

// The environment is the pipeline's to state, not the binary's to guess.
func TestBuildInfoReadsTheEnvironment(t *testing.T) {
	t.Setenv("BUILD_COMMIT_SHA", "9ebad225ea9ec25397778a4dc8d3ff842f2938a3")
	t.Setenv("BUILD_TIME", "2026-09-15T05:00:00Z")
	t.Setenv("BUILD_VERSION", "staging-19")
	t.Setenv("APP_ENV", "staging")

	info := CurrentBuildInfo("payment-wallet-service")
	if info.CommitSHA != "9ebad225ea9ec25397778a4dc8d3ff842f2938a3" {
		t.Fatalf("commit = %q", info.CommitSHA)
	}
	if info.BuildTime != "2026-09-15T05:00:00Z" || info.Version != "staging-19" {
		t.Fatalf("build metadata not read: %+v", info)
	}
	if info.Environment != "staging" {
		t.Fatalf("environment = %q", info.Environment)
	}
	if !info.Stamped() {
		t.Fatal("a build carrying a commit must report as stamped")
	}
}

// Stamped is what separates "running the wrong commit" from "cannot tell me
// which commit it is". Those are different problems with different fixes, so
// a deployment check must be able to distinguish them.
func TestStampedDistinguishesUnknownFromWrong(t *testing.T) {
	if (BuildInfo{CommitSHA: unknown}).Stamped() {
		t.Fatal("an unknown commit must not report as stamped")
	}
	if (BuildInfo{CommitSHA: ""}).Stamped() {
		t.Fatal("an empty commit must not report as stamped")
	}
	if !(BuildInfo{CommitSHA: "deadbeef"}).Stamped() {
		t.Fatal("a real commit must report as stamped")
	}
}

// The startup log line must actually carry the commit. The platform logger
// redacts by keyword, and a field name that happened to trip one of those
// keywords would silently turn the deployed-version log into [REDACTED] —
// which is exactly the evidence OPS-002 depends on.
func TestBuildFieldsSurviveLogRedaction(t *testing.T) {
	fields := logging.SafeFields(map[string]any{
		"service":     "driver-hauling-service",
		"version":     "staging-19",
		"commit_sha":  "9ebad225ea9ec25397778a4dc8d3ff842f2938a3",
		"build_time":  "2026-09-15T05:00:00Z",
		"environment": "staging",
		"stamped":     true,
	})
	for key, want := range map[string]string{
		"commit_sha":  "9ebad225ea9ec25397778a4dc8d3ff842f2938a3",
		"version":     "staging-19",
		"build_time":  "2026-09-15T05:00:00Z",
		"environment": "staging",
	} {
		got, _ := fields[key].(string)
		if got != want {
			t.Fatalf("%s was not logged verbatim: got %q want %q", key, got, want)
		}
		if strings.Contains(got, "REDACTED") {
			t.Fatalf("%s was redacted out of the deployed-version log line", key)
		}
	}
}
