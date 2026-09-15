package serviceapp

import (
	"os"
	"runtime/debug"
	"strings"
)

// Build metadata, stamped by the deployment pipeline.
//
// These are deliberately package-level vars rather than constants so the
// linker can set them:
//
//	go build -ldflags "-X cosmicforge/logistics/shared/go/serviceapp.commitSHA=$GITHUB_SHA \
//	                   -X cosmicforge/logistics/shared/go/serviceapp.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
//	                   -X cosmicforge/logistics/shared/go/serviceapp.version=$VERSION"
//
// They are NEVER hardcoded to a source SHA. A commit written into the tree
// would describe the commit someone typed, not the commit that was built, and
// the whole point of OPS-002 is being able to ask a running service which
// commit it actually is.
var (
	commitSHA string
	buildTime string
	version   string
)

// BuildInfo is what a deployment verification asks a running service.
type BuildInfo struct {
	Service     string `json:"service"`
	Version     string `json:"version"`
	CommitSHA   string `json:"commit_sha"`
	BuildTime   string `json:"build_time"`
	Environment string `json:"environment"`
}

// unknown is what an unstamped field reports.
//
// The empty string is not used: a missing key and a key whose value is empty
// are indistinguishable to a script, and "did the pipeline stamp this?" is
// exactly the question that must not be ambiguous. An explicit "unknown" says
// the binary was built outside the pipeline, which is itself a finding.
const unknown = "unknown"

// vcsCommit reads the commit Go itself recorded at build time.
//
// It is the fallback, not the primary source: this repository builds with
// GOFLAGS=-buildvcs=false in CI and copies only a subdirectory into the
// Docker build context, so .git is usually absent and this returns nothing.
// When it does resolve it is still real evidence, so it beats reporting
// unknown.
func vcsCommit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return ""
}

// CurrentBuildInfo resolves the build metadata for a service.
func CurrentBuildInfo(service string) BuildInfo {
	commit := strings.TrimSpace(commitSHA)
	if commit == "" {
		commit = strings.TrimSpace(os.Getenv("BUILD_COMMIT_SHA"))
	}
	if commit == "" {
		commit = vcsCommit()
	}
	if commit == "" {
		commit = unknown
	}

	built := strings.TrimSpace(buildTime)
	if built == "" {
		built = strings.TrimSpace(os.Getenv("BUILD_TIME"))
	}
	if built == "" {
		built = unknown
	}

	released := strings.TrimSpace(version)
	if released == "" {
		released = strings.TrimSpace(os.Getenv("BUILD_VERSION"))
	}
	if released == "" {
		// A build with a commit but no release tag is normal on a staging
		// branch; the commit is the identity that matters there.
		released = unknown
	}

	environment := strings.TrimSpace(os.Getenv("APP_ENV"))
	if environment == "" {
		environment = unknown
	}

	return BuildInfo{
		Service:     service,
		Version:     released,
		CommitSHA:   commit,
		BuildTime:   built,
		Environment: environment,
	}
}

// Stamped reports whether the pipeline supplied a commit.
//
// A deployment verification uses this to tell "this service is running the
// wrong commit" apart from "this service cannot tell me which commit it is",
// which are different problems with different fixes.
func (b BuildInfo) Stamped() bool { return b.CommitSHA != unknown && b.CommitSHA != "" }
