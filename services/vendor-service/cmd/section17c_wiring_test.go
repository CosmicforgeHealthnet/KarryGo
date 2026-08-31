package main

// The vendor-service half of the cross-service quote path.
//
// The full URL is baseURL + apiBase + commercequote.QuotePath. This file pins
// the apiBase this service is configured with; the owning logistics services
// pin the routes they register against the same shared constant. A change to
// either half now fails a test instead of producing a 404 on every call with
// no other symptom - which is the failure this platform already shipped once,
// when a full path was configured as a base URL and the client appended its
// own.

import (
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/commercequote"
)

func TestSection17CQuotePathsMatchTheOwningServices(t *testing.T) {
	cases := map[string]struct {
		apiBase string
		want    string
	}{
		"dispatch": {
			apiBase: dispatchQuoteAPIBase,
			// driver-dispatch-delivery-service mounts every internal surface
			// under /api/v1/internal.
			want: "/api/v1/internal/commerce/quotes",
		},
		"hauling": {
			apiBase: haulingQuoteAPIBase,
			// driver-hauling-service mounts every route under its own API
			// base, internal ones included.
			want: "/api/v1/hauling/internal/commerce/quotes",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			got := testCase.apiBase + commercequote.QuotePath

			if got != testCase.want {
				t.Fatalf("quote path = %q, want %q", got, testCase.want)
			}
		})
	}
}

// An API base carrying a scheme or a host is the doubled-URL bug in the making:
// the base URL is the origin, and this is only the path portion.
func TestSection17CAPIBasesArePathsNotURLs(t *testing.T) {
	for _, base := range []string{dispatchQuoteAPIBase, haulingQuoteAPIBase} {
		if !strings.HasPrefix(base, "/") {
			t.Errorf("api base %q must start with /", base)
		}

		if strings.Contains(base, "://") || strings.Contains(base, "localhost") {
			t.Errorf("api base %q looks like a URL; it must be a path only", base)
		}

		if strings.HasSuffix(base, "/") {
			t.Errorf("api base %q must not end with / - QuotePath supplies it",
				base)
		}
	}
}
