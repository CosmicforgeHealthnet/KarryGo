package http

import (
	"strings"
	"testing"
)

func TestSection17RouteCatalogue(t *testing.T) {
	routes := TenantRoutes()
	if len(routes) != 46 { // 37 + 5 shift-template + 4 enrolment routes (000089)
		t.Fatalf("routes=%d", len(routes))
	}
	seen := map[string]bool{}
	for _, r := range routes {
		k := r.Method + " " + r.Path
		if seen[k] {
			t.Fatalf("duplicate %s", k)
		}
		seen[k] = true
		if r.Permission == "" || r.FeatureFlag == "" || r.AuthClass != "corporate" {
			t.Fatalf("incomplete %+v", r)
		}
		if strings.Contains(strings.ToLower(k), "payroll") {
			t.Fatalf("excluded route %s", k)
		}
	}
}
