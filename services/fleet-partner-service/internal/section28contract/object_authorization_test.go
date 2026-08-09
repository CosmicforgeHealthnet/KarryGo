// Package section28contract's object-authorization coverage test reuses
// the SAME live-route enumeration route_coverage_test.go's own riskregistry
// coverage test already established (enumerateManifestBackedRoutes,
// manifestlessRoutes, staticPlatformRoutes) -- there is no second,
// hand-maintained route list anywhere in this file, by construction: the
// exact same three functions are the only source of "what routes actually
// exist" for both this file's tests and route_coverage_test.go's.
package section28contract

import (
	"sort"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/objectauth"
)

// isObjectRoute reports whether path carries at least one gin path
// parameter (":segment") -- Section 28's "every mounted route that
// contains or selects an object reference" via path parameters, the
// dominant and mechanically-detectable case. Query-selected IDs,
// download/export IDs embedded in a path parameter, and retry/replay
// targets are already :param segments in this codebase (confirmed by
// reading the routes below -- e.g. "/exports/:exportId/download",
// "/worker-operations/jobs/:jobId/retry") and so are already covered by
// this same check; there is no separate class of "query-string-only
// object ID" route in this codebase's mounted surface today.
func isObjectRoute(path string) bool {
	return strings.Contains(path, "/:")
}

func allLiveRoutes(t *testing.T) []liveRoute {
	t.Helper()
	var live []liveRoute
	live = append(live, enumerateManifestBackedRoutes(t)...)
	live = append(live, manifestlessRoutes()...)
	live = append(live, staticPlatformRoutes()...)
	return live
}

// TestObjectAuthorizationRegistry_CoversEveryMountedObjectRoute is Section
// 28's mandated contract test: it fails the moment a live, mounted object
// route cannot be resolved to a real RequiredPermission (or one of the two
// explicit non-permission sentinels -- self-ownership, HMAC caller
// allowlist -- for the two real routes that are correctly gated by neither
// a permission nor a registry gap). A newly added manifest route already
// carries its own permission and needs no registry change at all; a newly
// added manifestless-package object route with a permission this registry
// does not yet know about is exactly what this test is designed to catch.
func TestObjectAuthorizationRegistry_CoversEveryMountedObjectRoute(t *testing.T) {
	live := allLiveRoutes(t)

	var needsReview []string
	seen := map[string]bool{}
	for _, r := range live {
		if !isObjectRoute(r.path) {
			continue
		}
		key := r.method + " " + r.path
		if seen[key] {
			continue
		}
		seen[key] = true

		decl := objectauth.Classify(r.method, r.path, r.permission)
		if decl.NeedsManualReview {
			needsReview = append(needsReview, key)
		}
	}
	sort.Strings(needsReview)
	if len(needsReview) > 0 {
		t.Fatalf("%d live object route(s) have no resolvable authorization declaration:\n%s",
			len(needsReview), joinLines(needsReview))
	}
}

// TestObjectAuthorizationRegistry_EveryDeclarationIsInternallyConsistent
// pins the structural invariants Classify itself must never violate,
// independent of whatever the schema currently mounts: every declaration
// has at least one allowed actor class, every ORGANIZATION-exposure route
// has OrganizationPredicate true, every route answers SafeNotFound true
// (this codebase has no route, tenant or internal, that is allowed to
// confirm a resource's existence outside the caller's own scope), and a
// PUBLIC_CUSTOMER declaration (were one ever classified -- none are today,
// see the next test) would be the only case exempt from SafeNotFound.
func TestObjectAuthorizationRegistry_EveryDeclarationIsInternallyConsistent(t *testing.T) {
	live := allLiveRoutes(t)
	seen := map[string]bool{}
	for _, r := range live {
		if !isObjectRoute(r.path) {
			continue
		}
		key := r.method + " " + r.path
		if seen[key] {
			continue
		}
		seen[key] = true

		decl := objectauth.Classify(r.method, r.path, r.permission)
		if len(decl.AllowedActorClasses) == 0 {
			t.Errorf("%s: no allowed actor classes", key)
		}
		if decl.Exposure == objectauth.ExposureOrganization && !decl.OrganizationPredicate {
			t.Errorf("%s: ORGANIZATION exposure without OrganizationPredicate", key)
		}
		if decl.Exposure == objectauth.ExposureInternal && decl.OrganizationPredicate {
			t.Errorf("%s: INTERNAL exposure must not carry an organization predicate (it is platform-scoped)", key)
		}
		if !decl.SafeNotFound {
			t.Errorf("%s: every route this codebase mounts must answer safe-404, got false", key)
		}
	}
}

// TestObjectAuthorizationRegistry_ProviderRoutesRequireProviderRelationship
// proves every route this pass classified as provider-relationship-scoped
// (path contains a /providers/:providerId or /providers/:membershipId
// segment) actually carries the ActorProvider class alongside whichever
// base actor class its exposure implies -- a provider-shaped path that
// silently classified as organization-user-only (or internal-only) would
// be exactly the kind of gap this registry exists to catch.
func TestObjectAuthorizationRegistry_ProviderRoutesRequireProviderRelationship(t *testing.T) {
	live := allLiveRoutes(t)
	seen := map[string]bool{}
	checked := 0
	for _, r := range live {
		if !isObjectRoute(r.path) {
			continue
		}
		key := r.method + " " + r.path
		if seen[key] {
			continue
		}
		seen[key] = true
		if !strings.Contains(r.path, "/providers/:providerId") && !strings.Contains(r.path, "/providers/:membershipId") {
			continue
		}
		checked++
		decl := objectauth.Classify(r.method, r.path, r.permission)
		if !decl.ProviderRelationship {
			t.Errorf("%s: path names a provider object but ProviderRelationship is false", key)
		}
		found := false
		for _, actor := range decl.AllowedActorClasses {
			if actor == objectauth.ActorProvider {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: ProviderRelationship true but ActorProvider missing from AllowedActorClasses: %v", key, decl.AllowedActorClasses)
		}
	}
	if checked == 0 {
		t.Fatal("expected at least one live provider-object route to check -- test fixture assumption broke")
	}
}
