package journeysharetest

import (
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

type Contract struct{ File, Security string }

// AssertContracts compares both sets and probes the mounted auth boundary.
// The supplied verifier must know the deliberately wrong contract-test caller;
// its valid signature must be refused by the exact caller allowlist.
func AssertContracts(t *testing.T, engine *gin.Engine, contracts ...Contract) {
	t.Helper()
	declared := map[string]string{}
	for _, contract := range contracts {
		raw, err := os.ReadFile(contract.File)
		if err != nil {
			t.Fatal(err)
		}
		var spec struct {
			Paths map[string]map[string]json.RawMessage
		}
		if err = json.Unmarshal(raw, &spec); err != nil {
			t.Fatal(err)
		}
		for path, methods := range spec.Paths {
			for method, raw := range methods {
				if method != "get" && method != "post" && method != "put" && method != "delete" && method != "patch" {
					continue
				}
				var op struct{ Security []map[string][]string }
				if err = json.Unmarshal(raw, &op); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, scheme := range op.Security {
					if _, ok := scheme[contract.Security]; ok {
						found = true
					}
				}
				if !found {
					t.Fatalf("wrong OpenAPI auth: %s %s", method, path)
				}
				key := strings.ToUpper(method) + " " + path
				if _, exists := declared[key]; exists {
					t.Fatal("duplicate contract", key)
				}
				declared[key] = contract.Security
			}
		}
	}
	param := regexp.MustCompile(`:([A-Za-z][A-Za-z0-9_]*)`)
	for _, route := range engine.Routes() {
		key := route.Method + " " + param.ReplaceAllString(route.Path, "{$1}")
		security, ok := declared[key]
		if !ok {
			t.Fatal("mounted undocumented/wrong method/path", key)
		}
		delete(declared, key)
		path := param.ReplaceAllString(route.Path, "fixture") + "?customer_id=fixture"
		for _, header := range []string{"", "Bearer invalid"} {
			req := httptest.NewRequest(route.Method, path, nil)
			req.Header.Set("Authorization", header)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("missing/wrong boundary %s: %d", key, w.Code)
			}
		}
		if strings.Contains(route.Path, "/internal/") {
			if security != "HMACv2" {
				t.Fatal("private route has public boundary", key)
			}
			req := httptest.NewRequest(route.Method, path, nil)
			if err := serviceauth.SignRequest(req, "contract-wrong-caller", []byte("contract-test-secret"), nil, time.Now()); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != 403 {
				t.Fatalf("wrong caller accepted %s: %d", key, w.Code)
			}
		}
	}
	if len(declared) > 0 {
		t.Fatal("OpenAPI route unmounted", declared)
	}
}
