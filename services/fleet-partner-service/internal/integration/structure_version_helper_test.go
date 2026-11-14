package integration_test

import (
	"net/http"
	"strings"
	"testing"
)

// currentVersionFor returns the version a screen would hold for the resource
// a versioned write targets, by reading it first, as the web app does. It is
// used when a structure test passes 0 to mean "the version I just read":
// these writes require If-Match, so a missing header is never what the test
// means. A write that is not versioned (POST create) returns 0.
func currentVersionFor(t *testing.T, router http.Handler, token, grant, method, path string) int {
	t.Helper()
	target := path
	switch {
	case method == http.MethodPatch:
	case method == http.MethodPost && strings.HasSuffix(path, "/archive"):
		target = strings.TrimSuffix(path, "/archive")
	default:
		return 0
	}
	code, body := onboardCallStepUp(t, router, token, grant, http.MethodGet, target, nil, 0)
	if code != http.StatusOK {
		return 1 // the write is expected to be refused for another reason (404, 409)
	}
	data, _ := body["data"].(map[string]any)
	if v, ok := data["version"].(float64); ok && v >= 1 {
		return int(v)
	}
	return 1
}
