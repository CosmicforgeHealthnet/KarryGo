// Package section28contract's route-coverage test enumerates every route
// each feature's own manifest ACTUALLY reports (calling the real
// TenantRoutes()/InternalRoutes()/etc. methods with nil-safe constructors,
// not a one-time regex extraction over the source text) and cross-checks it
// against internal/platform/riskregistry, so a route added to a manifest
// without a matching registry entry fails the build immediately.
//
// The first version of riskregistry.go was built by a one-time Python
// regex pass over every manifest.go file. That pass had two real, silently
// missed gaps this test exists specifically to catch a repeat of:
//  1. bulkimports/manifests/packages/privateoperations/recurringorders use
//     positional struct literals ({"GET", "/x", ...}) instead of keyed
//     ({Method: "GET", Path: "/x", ...}) -- a regex tuned for the keyed form
//     silently matched zero entries in five packages.
//  2. backgroundops's InternalRoutes() lives in internal.go, not
//     manifest.go -- a glob for manifest.go alone silently skipped it.
//
// Calling the real Go methods (which the Go compiler itself parses,
// regardless of literal style or which file a function lives in) is immune
// to both classes of gap.
package section28contract

import (
	"sort"
	"testing"

	assetsfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	audithttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/http"
	auditrepositories "cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/repositories"
	backgroundopshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/backgroundops/http"
	bulkimportshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/http"
	compliancefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance"
	crmhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/http"
	dispatchfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch"
	documenthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	exportshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	legalholdhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/http"
	manifestshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/http"
	ordershttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/http"
	packageshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/http"
	privatehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/http"
	prooffeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/proof"
	recurringhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/http"
	retentionexecutionhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionexecution/http"
	retentionpolicyhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/http"
	safetyfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety"
	searchhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/http"
	securitysettingshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/http"
	cataloghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/http"
	shipmentshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/http"
	specializedfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/synchronization"
	warehousefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse"
	workforcefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/riskregistry"
)

const (
	tenantPrefix          = "/fleet-partner/v1"
	internalPrefix        = "/internal/fleet-partner/v1"
	organizationAPIPrefix = "/fleet-partner/integrations/v1"
)

type liveRoute struct {
	method, path string
	// permission is the manifest's own Route.Permission field, threaded
	// through unchanged -- empty for the handful of manifest routes that
	// carry no permission field at all (customer-token routes) and for
	// every route returned by manifestlessRoutes/staticPlatformRoutes
	// (those two return the zero value; internal/platform/objectauth
	// resolves the object-route subset of those from its own
	// manifestlessPermissions map, populated by reading each package's
	// real h.authorize(...) call sites directly, not guessed).
	permission string
}

// enumerateManifestBackedRoutes calls every feature's real route-manifest
// methods (nil-safe constructors: these handlers are never actually invoked,
// only asked to describe their own routes) and returns the complete,
// prefixed, live set.
func enumerateManifestBackedRoutes(t *testing.T) []liveRoute {
	t.Helper()
	var routes []liveRoute
	add := func(prefix string, path string, method string, permission string) {
		routes = append(routes, liveRoute{method: method, path: prefix + path, permission: permission})
	}

	auditH := audithttp.NewHandler((*auditrepositories.Repository)(nil), nil)
	for _, r := range auditH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	bgH := backgroundopshttp.NewHandler(nil, nil, nil)
	for _, r := range bgH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}
	for _, r := range backgroundopshttp.InternalRoutes() {
		add(internalPrefix, r.Path, r.Method, r.Permission)
	}

	bulkH := bulkimportshttp.NewHandler(nil, nil, nil)
	for _, r := range bulkH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}
	for _, r := range bulkH.GenericTenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	// crmhttp.AllRoutes exists but flattens TenantRoutes/CustomerTokenRoutes/
	// InternalRoutes into one slice with no way to tell which prefix each
	// entry needs afterward, so the three lists are called directly here
	// instead, each tagged with its own correct prefix.
	crmCorporate := crmhttp.NewHandler(nil, nil)
	crmPublic := crmhttp.NewPublicHandler(nil, nil)
	crmInternal := crmhttp.NewInternalHandler(nil)
	for _, r := range crmCorporate.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}
	for _, r := range crmPublic.CustomerTokenRoutes() {
		// Unauthenticated customer-token surface: not organization-scoped,
		// not internal-HMAC either. It is mounted under /fleet-partner/v1
		// alongside the corporate surface (see internal/app.Components.
		// CustomerRoutes) so its risk lookup key uses the same prefix.
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}
	for _, r := range crmInternal.InternalRoutes() {
		add(internalPrefix, r.Path, r.Method, r.Permission)
	}

	docH := documenthttp.NewHandler(nil, nil)
	for _, r := range docH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}
	for _, r := range documenthttp.InternalRoutes() {
		add(internalPrefix, r.Path, r.Method, r.Permission)
	}

	exportsH := exportshttp.NewHandler(nil, nil, nil)
	for _, r := range exportsH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	legalH := legalholdhttp.NewHandler(nil, nil)
	for _, r := range legalH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	manifestsH := manifestshttp.NewHandler(nil, nil, nil)
	for _, r := range manifestsH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	ordersH := ordershttp.NewHandler(nil, nil)
	for _, r := range ordersH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	packagesH := packageshttp.NewHandler(nil, nil, nil)
	for _, r := range packagesH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	privateH := privatehttp.NewHandler(nil, nil, nil)
	for _, r := range privateH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}
	for _, r := range privateH.InternalRoutes() {
		add(internalPrefix, r.Path, r.Method, r.Permission)
	}

	recurringH := recurringhttp.NewHandler(nil, nil, nil)
	for _, r := range recurringH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	retentionH := retentionpolicyhttp.NewHandler(nil, nil)
	for _, r := range retentionH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	retentionExecH := retentionexecutionhttp.NewHandler(nil, nil)
	for _, r := range retentionExecH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	searchH := searchhttp.NewHandler(nil, nil)
	for _, r := range searchH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	securityH := securitysettingshttp.NewHandler(nil, nil)
	for _, r := range securityH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	catalogH := cataloghttp.NewHandler(nil, nil)
	for _, r := range catalogH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	shipmentsH := shipmentshttp.NewHandler(nil, nil, nil)
	for _, r := range shipmentsH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	// The 11 packages below were found, while building Section 28's
	// object-authorization registry, to be mounted for real in
	// internal/app/wiring.go's tenantScoped registrar (RegisterTenantRoutes
	// called directly on each) but ENTIRELY ABSENT from this enumeration --
	// meaning every one of their live routes had NO riskregistry rate-limit
	// classification and, before this fix, could have had no
	// object-authorization declaration either, invisible to both coverage
	// tests. Fixed here as the direct root cause, not a separate cleanup.
	assetsH := assetsfeature.NewHandler(nil, nil, nil)
	for _, r := range assetsH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	complianceH := compliancefeature.NewHandler(nil, nil, nil)
	for _, r := range complianceH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	dispatchH := dispatchfeature.NewHandler(nil, nil, nil)
	for _, r := range dispatchH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}
	for _, r := range dispatchH.InternalRoutes() {
		add(internalPrefix, r.Path, r.Method, r.Permission)
	}

	// finance.OperatorHandler has no Route-listing manifest at all (routes
	// are registered directly by group.METHOD calls, the same shape as the
	// 8 manifestless packages) -- its 13 tenant routes are declared in
	// manifestlessRoutes() below instead, permissions read directly from
	// each handler function's own h.gate(c, "...") call.

	proofH := prooffeature.NewHandler(nil, nil, nil)
	for _, r := range proofH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	specializedH := specializedfeature.NewHandler(nil, nil, nil)
	for _, r := range specializedH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	warehouseH := warehousefeature.NewHandler(nil, nil, nil)
	for _, r := range warehouseH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	workforceH := workforcefeature.NewHandler(nil, nil, nil)
	for _, r := range workforceH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	financeOpsH := financeops.NewHandler(nil, nil, nil)
	for _, r := range financeOpsH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	engagementH := engagement.NewHandler(nil, nil, nil)
	for _, r := range engagementH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	for _, r := range synchronization.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	// safety's OperationsHandler is a SECOND, separate handler from the
	// already-covered references Handler (manifestlessRoutes' "safety"
	// entries) -- both are real, both are mounted
	// (safetyHandler.RegisterTenantRoutes AND
	// safetyOperationsHandler.RegisterTenantRoutes in wiring.go), and this
	// one was the other half of the same blind spot.
	safetyOpsH := safetyfeature.NewOperationsHandler(nil, nil, nil)
	for _, r := range safetyOpsH.TenantRoutes() {
		add(tenantPrefix, r.Path, r.Method, r.Permission)
	}

	return routes
}

// manifestlessRoutes are routes registered directly (group.METHOD calls,
// no manifest.go/Route-slice method to call) by the eight packages that
// predate the manifest convention: approvals, auth, companywallet,
// eligibility, finance, marketplace, organizations, safety. This list is
// hand-maintained -- it CAN drift if one of these packages adds a route
// without updating it here, unlike the manifest-backed list above which is
// immune to drift by construction. Migrating these eight onto the manifest
// convention (adding a manifest.go each) would close this one remaining gap;
// tracked as a named follow-up rather than done silently.
//
// finance was found missing entirely (not here, not in any manifest, so its
// 3 real internal routes had NO risk classification and were invisible to
// TestRouteRiskRegistry_CoversEveryMountedRoute) while building the Section
// 28 service-auth route-scoping registry, which needed accurate ground
// truth for every mounted internal route -- fixed here as a direct
// prerequisite, not a separate unrelated cleanup.
func manifestlessRoutes() []liveRoute {
	tenant := func(method, path string) liveRoute { return liveRoute{method: method, path: tenantPrefix + path} }
	internalRoute := func(method, path string) liveRoute { return liveRoute{method: method, path: internalPrefix + path} }
	var routes []liveRoute
	for _, r := range []liveRoute{
		tenant("GET", "/approvals/requests"), tenant("GET", "/approvals/requests/:approvalId"),
		tenant("GET", "/approvals/requests/:approvalId/decisions"), tenant("GET", "/approvals/requests/:approvalId/history"),
		tenant("POST", "/approvals/requests"), tenant("POST", "/approvals/requests/:approvalId/approve"),
		tenant("POST", "/approvals/requests/:approvalId/cancel"), tenant("POST", "/approvals/requests/:approvalId/execute"),
		tenant("POST", "/approvals/requests/:approvalId/reject"),

		tenant("DELETE", "/sessions/:sessionId"), tenant("GET", "/sessions"),
		tenant("POST", "/auth/logout"), tenant("POST", "/auth/refresh"), tenant("POST", "/auth/resend"),
		tenant("POST", "/auth/select-organization"), tenant("POST", "/auth/start"),
		tenant("POST", "/auth/step-up/start"), tenant("POST", "/auth/step-up/verify"), tenant("POST", "/auth/verify"),

		tenant("GET", "/provider/sub-wallet"), tenant("GET", "/provider/sub-wallet/statement"),
		tenant("GET", "/providers/:membershipId/sub-wallet"), tenant("GET", "/providers/:membershipId/sub-wallet/allocations"),
		tenant("GET", "/providers/:membershipId/sub-wallet/statement"), tenant("GET", "/providers/:membershipId/sub-wallet/withdrawals"),
		tenant("GET", "/wallet"), tenant("GET", "/wallet/policy"), tenant("GET", "/wallet/transactions"),
		tenant("GET", "/wallet/transactions/:transactionId"),
		tenant("POST", "/provider/sub-wallet/withdrawals"), tenant("POST", "/providers/:membershipId/sub-wallet/allocations"),
		tenant("POST", "/providers/:membershipId/sub-wallet/allocations/:allocationId/approve"),
		tenant("POST", "/providers/:membershipId/sub-wallet/allocations/:allocationId/reject"),
		tenant("POST", "/providers/:membershipId/sub-wallet/allocations/:allocationId/replay"),
		tenant("POST", "/providers/:membershipId/sub-wallet/withdrawals"),
		tenant("POST", "/providers/:membershipId/sub-wallet/withdrawals/:withdrawalId/approve"),
		tenant("POST", "/providers/:membershipId/sub-wallet/withdrawals/:withdrawalId/reject"),
		tenant("POST", "/providers/:membershipId/sub-wallet/withdrawals/:withdrawalId/replay"),
		tenant("PUT", "/wallet/policy"),

		tenant("GET", "/eligibility/decisions"), tenant("GET", "/eligibility/decisions/:decisionId"),
		tenant("GET", "/eligibility/decisions/:decisionId/history"), tenant("GET", "/eligibility/policies"),
		tenant("POST", "/eligibility/evaluations"), tenant("POST", "/eligibility/policies"),

		internalRoute("GET", "/assignments/:assignmentId/provider-acceptance"),
		internalRoute("GET", "/assignments/:assignmentId/provider-decision-receipt"),
		internalRoute("GET", "/admin/resources/:kind"),
		internalRoute("GET", "/admin/authorization-contexts/bindings/:kind/:id"),
		internalRoute("GET", "/admin/resources/:kind/:id"),
		internalRoute("POST", "/admin/authorization-contexts"),
		internalRoute("GET", "/providers/:providerId/assignments"), internalRoute("GET", "/providers/:providerId/assignments/:assignmentId"),
		internalRoute("GET", "/providers/:providerId/memberships"),
		internalRoute("GET", "/providers/:providerId/offers"), internalRoute("GET", "/providers/:providerId/offers/:offerId"),
		internalRoute("POST", "/assignments/:assignmentId/provider-accept"), internalRoute("POST", "/assignments/:assignmentId/provider-acknowledge"),
		internalRoute("POST", "/assignments/:assignmentId/provider-decline"),
		internalRoute("POST", "/events"),
		internalRoute("POST", "/finance/events"),
		internalRoute("POST", "/finance/operations/:id/replay"), internalRoute("POST", "/finance/projections/:id/replay"),
		internalRoute("POST", "/marketplace-offers/:offerId/provider-accept"), internalRoute("POST", "/marketplace-offers/:offerId/provider-decline"),

		tenant("GET", "/organizations/:organizationId/closure-status"), tenant("GET", "/organizations/:organizationId/documents"),
		tenant("GET", "/organizations/:organizationId/onboarding-status"), tenant("GET", "/organizations/:organizationId/verification"),
		tenant("GET", "/organizations/:organizationId/wallet-provisioning"), tenant("GET", "/organizations/current"),
		tenant("GET", "/permissions"), tenant("GET", "/providers"), tenant("GET", "/providers/:membershipId"),
		tenant("GET", "/providers/:membershipId/history"), tenant("GET", "/providers/:membershipId/provisioning-status"),
		tenant("GET", "/roles"), tenant("GET", "/staff"),
		tenant("POST", "/organizations"), tenant("POST", "/organizations/:organizationId/close-request"),
		tenant("POST", "/organizations/:organizationId/closure/assess"), tenant("POST", "/organizations/:organizationId/closure/approve"),
		tenant("POST", "/organizations/:organizationId/closure/reject"), tenant("POST", "/organizations/:organizationId/closure/execute"),
		tenant("POST", "/organizations/:organizationId/documents"), tenant("POST", "/organizations/:organizationId/submit"),
		tenant("POST", "/organizations/:organizationId/wallet-provisioning/retry"),
		tenant("POST", "/providers"), tenant("POST", "/providers/:membershipId/suspend"),
		tenant("POST", "/staff/:staffId/reactivate"), tenant("POST", "/staff/:staffId/suspend"), tenant("POST", "/staff/invitations"),

		tenant("GET", "/safety/references"), tenant("GET", "/safety/references/:referenceId"),
		tenant("GET", "/safety/references/:referenceId/history"), tenant("POST", "/safety/references"),

		// finance.OperatorHandler (found missing entirely, alongside 10
		// other packages, while building Section 28's object-authorization
		// registry -- see the comment in enumerateManifestBackedRoutes).
		// Registers its routes directly with group.METHOD calls, same
		// shape as every other package in this function; permissions in
		// internal/platform/objectauth.manifestlessPermissions were read
		// directly from each handler's own h.gate(c, "...") call.
		tenant("GET", "/finance/operations"), tenant("GET", "/finance/operations/:operationId"),
		tenant("GET", "/finance/operations/:operationId/history"), tenant("GET", "/finance/operations/:operationId/audit"),
		tenant("POST", "/finance/operations/:operationId/replay"),
		tenant("GET", "/finance/projections"), tenant("GET", "/finance/projections/:projectionId"),
		tenant("GET", "/finance/reconciliation-cases"), tenant("GET", "/finance/reconciliation-cases/:projectionId"),
		tenant("POST", "/finance/reconciliation-cases/:projectionId/replay"),
		tenant("POST", "/finance/reconciliation-cases/:projectionId/read-back"),
		tenant("POST", "/finance/settlements"), tenant("POST", "/finance/refunds"), tenant("POST", "/finance/reversals"),

		// Section 55 integrations register directly in http.go. Management
		// routes are corporate tenant routes; /context is mounted on the
		// separately authenticated organization API-client route class.
		tenant("GET", "/integrations/api-clients"), tenant("POST", "/integrations/api-clients"),
		tenant("POST", "/integrations/api-clients/:id/rotate"), tenant("POST", "/integrations/api-clients/:id/revoke"),
		tenant("GET", "/integrations/webhooks"), tenant("POST", "/integrations/webhooks"),
		tenant("PATCH", "/integrations/webhooks/:id"), tenant("POST", "/integrations/webhooks/:id/rotate-secret"),
		tenant("GET", "/integrations/webhooks/:id/deliveries"), tenant("POST", "/integrations/webhook-deliveries/:id/replay"),
	} {
		routes = append(routes, r)
	}
	return routes
}

// staticPlatformRoutes are mounted directly in internal/app/app.go, outside
// any feature's manifest.
func staticPlatformRoutes() []liveRoute {
	return []liveRoute{
		{method: "GET", path: "/health"}, {method: "GET", path: "/ready"}, {method: "GET", path: "/metrics"},
		{method: "GET", path: tenantPrefix + "/meta"}, {method: "GET", path: internalPrefix + "/meta"},
		{method: "GET", path: organizationAPIPrefix + "/context"},
	}
}

// TestRouteRiskRegistry_CoversEveryMountedRoute fails the moment a real,
// live route (enumerated from the feature packages' own manifest methods,
// not a stale copy) has no entry in riskregistry -- Section 28's "every
// mounted route must be classified... contract tests must fail when a route
// is unclassified" requirement.
func TestRouteRiskRegistry_CoversEveryMountedRoute(t *testing.T) {
	var live []liveRoute
	live = append(live, enumerateManifestBackedRoutes(t)...)
	live = append(live, manifestlessRoutes()...)
	live = append(live, staticPlatformRoutes()...)

	var missing []string
	seen := map[string]bool{}
	for _, r := range live {
		key := r.method + " " + r.path
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, ok := riskregistry.Lookup(r.method, r.path); !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("%d live route(s) have no riskregistry classification:\n%s",
			len(missing), joinLines(missing))
	}
}

// TestRouteRiskRegistry_HasNoStaleEntries flags a registry entry that no
// longer corresponds to any live route (renamed/removed route) -- not a
// security gap the way a missing entry is, but a signal the registry needs
// regenerating.
func TestRouteRiskRegistry_HasNoStaleEntries(t *testing.T) {
	var live []liveRoute
	live = append(live, enumerateManifestBackedRoutes(t)...)
	live = append(live, manifestlessRoutes()...)
	live = append(live, staticPlatformRoutes()...)

	liveSet := map[string]bool{}
	for _, r := range live {
		liveSet[r.method+" "+r.path] = true
	}

	var stale []string
	for _, e := range riskregistry.All() {
		key := e.Method + " " + e.Path
		if !liveSet[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Logf("%d riskregistry entries no longer correspond to a live route (informational, not a failure):\n%s",
			len(stale), joinLines(stale))
	}
}

// TestRiskRegistry_EveryEntryIsAValidClass proves the registry itself never
// contains a typo'd or otherwise unrecognized risk class string.
func TestRiskRegistry_EveryEntryIsAValidClass(t *testing.T) {
	for _, e := range riskregistry.All() {
		validClasses := map[string]bool{
			"LOW": true, "STANDARD": true, "SENSITIVE": true, "HIGH_RISK": true, "INTERNAL_SERVICE": true,
		}
		if !validClasses[e.RiskClass] {
			t.Errorf("%s %s has unrecognized risk class %q", e.Method, e.Path, e.RiskClass)
		}
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += "  " + l + "\n"
	}
	return out
}
