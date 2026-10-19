// Package objectauth implements Section 28's object-level authorization
// registry: one Declaration per mounted route that carries or selects an
// object reference (a gin route template containing at least one
// ":param" segment), covering path parameters, nested resources,
// query-selected IDs, download/export IDs, retry/replay targets and
// internal object references alike -- the registry does not distinguish
// by HOW a route names its object, only that it does.
//
// Same two-layer design internal/platform/riskregistry and
// internal/section28contract's sensitive-column inventory already
// established: Classify mechanically derives every field it can from real,
// already-verified signals (the route's method/path/exposure class, and
// its permission string) rather than a hand-typed answer per route, and a
// small, explicit override map (manifestlessPermissions) supplies the one
// signal that cannot be derived live -- the permission string for the 8
// packages that predate the manifest convention (see
// internal/section28contract/route_coverage_test.go's own doc comment for
// why those 8 exist and are not fixed here). Every value in that map was
// read directly from each package's own h.authorize(...) call sites, not
// guessed from a route name.
//
// "Coverage" is enforced by internal/section28contract's
// TestObjectAuthorizationRegistry_CoversEveryMountedObjectRoute: it
// enumerates the SAME live route manifests riskregistry's own coverage
// test already uses (zero second, hand-maintained route list to drift)
// and fails if any live object route classifies with NeedsManualReview
// true -- which happens only when a route's permission could not be
// resolved from either the live manifest data or manifestlessPermissions,
// so a newly added manifestless-package object route with an
// unregistered permission fails the build exactly as required, rather
// than silently defaulting to "no permission required".
package objectauth

import (
	"fmt"
	"strings"
)

// Exposure classifies which route class an object route is mounted under.
const (
	ExposureInternal        = "INTERNAL"
	ExposureOrganization    = "ORGANIZATION"
	ExposureOrganizationAPI = "ORGANIZATION_API"
	ExposurePublicCustomer  = "PUBLIC_CUSTOMER"
)

// Actor classes this codebase's real authentication/authorization system
// actually distinguishes. Section 28's broader taxonomy (dispatcher, fleet
// manager, finance, support, compliance, organization owner...) are ROLES
// gated by PERMISSION within the single "organization_user" actor type in
// this codebase (accesscontrol.Authorizer.Require checks a permission
// string, never a role name directly) -- not independent authentication
// actor classes the way "internal_service" and "provider" genuinely are.
// AllowedActorClasses therefore names the AUTHENTICATION boundary a route
// enforces; which permission gates it (RequiredPermission) is what
// separates a fleet manager's access from a finance user's within that
// boundary.
const (
	ActorOrganizationUser      = "organization_user"
	ActorOrganizationAPIClient = "organization_api_client"
	ActorInternalService       = "internal_service"
	ActorProvider              = "provider"
	// ActorPublicToken is the one route class in this codebase with no
	// corporate/provider identity at all -- CRM's hashed-token secure-link
	// surface (ExposurePublicCustomer). The token itself is the whole
	// authorization decision.
	ActorPublicToken = "public_token"
)

// Declaration is one route's object-level authorization contract.
type Declaration struct {
	Method                string
	Path                  string
	AllowedActorClasses   []string
	RequiredPermission    string
	OrganizationPredicate bool
	BranchPredicate       bool
	ParentChildValidation bool
	ProviderRelationship  bool
	SafeNotFound          bool
	Exposure              string
	// NeedsManualReview is true only when RequiredPermission could not be
	// resolved at all (neither the live manifest nor
	// manifestlessPermissions had an answer) -- see the package doc
	// comment for why this, not a hand-maintained list, is the mechanism
	// that makes coverage actually fail on a real gap.
	NeedsManualReview bool
}

// manifestlessPermissions supplies RequiredPermission for two distinct
// groups of object routes whose live Permission signal is empty:
//
//  1. Routes in the 8 packages that predate the manifest convention (their
//     Route structs, and therefore the live enumeration, carry no
//     Permission field at all).
//  2. Routes that DO come from a real manifest (documents' and
//     backgroundops' InternalRoutes(), crm's CustomerTokenRoutes()) but
//     whose Permission field is legitimately empty, because the route is
//     gated by a mechanism other than an organization permission string --
//     confirmed by reading each handler, not assumed from the empty field
//     alone.
//
// Every entry was read directly from the owning package's real
// authorization code (h.authorize(...) for group 1;
// h.allowed[caller]/requireAllowedCaller for HMAC-gated internal routes;
// the hashed-token lookup for crm's secure-link surface), not inferred
// from the path. Keyed "METHOD /fleet-partner/v1/path" or
// "METHOD /internal/fleet-partner/v1/path", exactly as
// internal/section28contract's liveRoute keys its own method+path pairs.
var manifestlessPermissions = map[string]string{
	// approvals
	"GET /fleet-partner/v1/approvals/requests/:approvalId":           "approvals.requests.read",
	"GET /fleet-partner/v1/approvals/requests/:approvalId/decisions": "approvals.requests.read",
	"GET /fleet-partner/v1/approvals/requests/:approvalId/history":   "approvals.requests.read",
	"POST /fleet-partner/v1/approvals/requests/:approvalId/approve":  "approvals.requests.decide",
	"POST /fleet-partner/v1/approvals/requests/:approvalId/reject":   "approvals.requests.decide",
	"POST /fleet-partner/v1/approvals/requests/:approvalId/cancel":   "approvals.requests.cancel",
	"POST /fleet-partner/v1/approvals/requests/:approvalId/execute":  "approvals.requests.execute",

	// companywallet
	"GET /fleet-partner/v1/wallet/transactions/:transactionId":                                    "wallet.transactions.read",
	"GET /fleet-partner/v1/providers/:membershipId/sub-wallet":                                    "wallet.provider_subwallet.read",
	"GET /fleet-partner/v1/providers/:membershipId/sub-wallet/statement":                          "wallet.provider_subwallet.read",
	"GET /fleet-partner/v1/providers/:membershipId/sub-wallet/allocations":                        "wallet.provider_subwallet.read",
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/allocations":                       "wallet.provider_allocations.create",
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/allocations/:allocationId/approve": "wallet.provider_allocations.approve",
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/allocations/:allocationId/reject":  "wallet.provider_allocations.approve",
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/allocations/:allocationId/replay":  "wallet.provider_allocations.approve",
	"GET /fleet-partner/v1/providers/:membershipId/sub-wallet/withdrawals":                        "wallet.provider_withdrawals.read",
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/withdrawals":                       "wallet.provider_withdrawals.approve",
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/withdrawals/:withdrawalId/approve": "wallet.provider_withdrawals.approve",
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/withdrawals/:withdrawalId/reject":  "wallet.provider_withdrawals.reject",
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/withdrawals/:withdrawalId/replay":  "wallet.provider_withdrawals.approve",

	// eligibility (evaluate is not a :param route; decisions/:decisionId
	// routes are)
	"GET /fleet-partner/v1/eligibility/decisions/:decisionId":         "eligibility.decisions.read",
	"GET /fleet-partner/v1/eligibility/decisions/:decisionId/history": "eligibility.decisions.read",

	// organizations (exact h.authorize(...) call sites, read directly)
	"GET /fleet-partner/v1/organizations/:organizationId/onboarding-status":          "organization.read",
	"POST /fleet-partner/v1/organizations/:organizationId/documents":                 "organization.update",
	"GET /fleet-partner/v1/organizations/:organizationId/documents":                  "organization.read",
	"POST /fleet-partner/v1/organizations/:organizationId/submit":                    "organization.submit",
	"GET /fleet-partner/v1/organizations/:organizationId/verification":               "organization.read",
	"GET /fleet-partner/v1/organizations/:organizationId/wallet-provisioning":        "organization.read",
	"POST /fleet-partner/v1/organizations/:organizationId/wallet-provisioning/retry": "organization.update",
	"POST /fleet-partner/v1/organizations/:organizationId/close-request":             "organization.close",
	"GET /fleet-partner/v1/organizations/:organizationId/closure-status":             "organization.read",
	"POST /fleet-partner/v1/organizations/:organizationId/closure/assess":            "organization.closure.manage",
	"POST /fleet-partner/v1/organizations/:organizationId/closure/approve":           "organization.closure.manage",
	"POST /fleet-partner/v1/organizations/:organizationId/closure/reject":            "organization.closure.manage",
	"POST /fleet-partner/v1/organizations/:organizationId/closure/execute":           "organization.closure.manage",
	"POST /fleet-partner/v1/staff/:staffId/suspend":                                  "staff.suspend",
	"POST /fleet-partner/v1/staff/:staffId/reactivate":                               "staff.manage",
	"GET /fleet-partner/v1/providers/:membershipId":                                  "providers.read",
	"POST /fleet-partner/v1/providers/:membershipId/suspend":                         "providers.suspend",
	"GET /fleet-partner/v1/providers/:membershipId/provisioning-status":              "providers.read",
	"GET /fleet-partner/v1/providers/:membershipId/history":                          "providers.read",

	// safety
	"GET /fleet-partner/v1/safety/references/:referenceId":         "safety.references.read",
	"GET /fleet-partner/v1/safety/references/:referenceId/history": "safety.references.read",

	// auth: DELETE /sessions/:sessionId is gated by self-ownership (a
	// caller may only revoke their OWN session -- checked against the
	// authenticated identity, not an organization permission string), not
	// a permission -- confirmed by reading internal/features/auth/http;
	// declared with an explicit empty permission plus a dedicated note
	// rather than left to fall through to NeedsManualReview, so the
	// coverage test does not misreport a real, checked, non-permission
	// authorization rule as an unreviewed gap.
	"DELETE /fleet-partner/v1/sessions/:sessionId": selfOwnershipSentinel,

	// marketplace/finance internal object routes: HMAC caller-allowlist
	// (requireAllowedCaller), not an organization permission -- same
	// reasoning as auth's session route above.
	"GET /internal/fleet-partner/v1/assignments/:assignmentId/provider-acceptance":       internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/assignments/:assignmentId/provider-decision-receipt": internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/providers/:providerId/assignments":                   internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/providers/:providerId/assignments/:assignmentId":     internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/providers/:providerId/memberships":                   internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/providers/:providerId/offers":                        internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/providers/:providerId/offers/:offerId":               internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/assignments/:assignmentId/provider-accept":          internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/assignments/:assignmentId/provider-acknowledge":     internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/assignments/:assignmentId/provider-decline":         internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/finance/operations/:id/replay":                      internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/finance/projections/:id/replay":                     internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/marketplace-offers/:offerId/provider-accept":        internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/marketplace-offers/:offerId/provider-decline":       internalCallerAllowlistSentinel,

	// documents' InternalRoutes(): a real manifest, but internal.go gates
	// every one of these with its own h.allowed[caller] HMAC allowlist
	// (internal/features/documents/http/internal.go), not an organization
	// permission -- the manifest's Permission field is legitimately empty.
	"GET /internal/fleet-partner/v1/customers/:customerId/orders/:orderId/documents":                                internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/customers/:customerId/orders/:orderId/documents/:documentId":                    internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/customers/:customerId/orders/:orderId/documents/:documentId/download-requests": internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/providers/:providerId/documents":                                                internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/providers/:providerId/documents/:documentId":                                    internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/providers/:providerId/documents/:documentId/download-requests":                 internalCallerAllowlistSentinel,

	// backgroundops' InternalRoutes(): same pattern, gated by
	// h.requireAllowedCaller (internal/features/backgroundops/http/
	// internal.go), confirmed by reading it directly.
	"GET /internal/fleet-partner/v1/worker-operations/jobs/:jobId":                      internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/worker-operations/jobs/:jobId/attempts":             internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/worker-operations/jobs/:jobId/retry":               internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/worker-operations/jobs/:jobId/cancel":              internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/worker-operations/scheduled-tasks/:taskId":          internalCallerAllowlistSentinel,
	"GET /internal/fleet-partner/v1/worker-operations/scheduled-tasks/:taskId/runs":     internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/worker-operations/scheduled-tasks/:taskId/enable":  internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/worker-operations/scheduled-tasks/:taskId/disable": internalCallerAllowlistSentinel,
	"POST /internal/fleet-partner/v1/worker-operations/scheduled-tasks/:taskId/run-now": internalCallerAllowlistSentinel,

	// crm's CustomerTokenRoutes(): the unauthenticated secure-link surface
	// (internal/features/crm/http/manifest.go) -- gated by a hashed,
	// expiring, single-quotation token presented in the request, checked in
	// the handler itself, never an organization permission (there is no
	// organization session on this route at all).
	"GET /fleet-partner/v1/quotations/:quotationId/view": publicTokenSentinel,

	// finance.OperatorHandler's tenant routes (internal/features/finance/
	// operator_http.go): a real, mounted RegisterTenantRoutes surface, but
	// (like approvals/companywallet/eligibility/organizations/safety above)
	// it registers gin routes directly rather than through a Route{}
	// manifest, so its live Permission signal is empty even though every
	// route IS gated by a real permission -- each string below is copied
	// from the exact h.gate(c, PermissionXxx) call site in that handler.
	// getProjection backs two different routes with a path-conditional
	// permission (FullPath() contains "reconciliation-cases" or not) --
	// both branches are listed explicitly against their own route.
	"GET /fleet-partner/v1/finance/operations/:operationId":                       "finance.operations.read",
	"GET /fleet-partner/v1/finance/operations/:operationId/history":               "finance.operations.read",
	"GET /fleet-partner/v1/finance/operations/:operationId/audit":                 "finance.operations.read",
	"POST /fleet-partner/v1/finance/operations/:operationId/replay":               "finance.operations.replay",
	"GET /fleet-partner/v1/finance/projections/:projectionId":                     "finance.operations.read",
	"GET /fleet-partner/v1/finance/reconciliation-cases/:projectionId":            "finance.reconciliation.read",
	"POST /fleet-partner/v1/finance/reconciliation-cases/:projectionId/replay":    "finance.reconciliation.manage",
	"POST /fleet-partner/v1/finance/reconciliation-cases/:projectionId/read-back": "finance.reconciliation.manage",

	// Section 55 management routes: exact permission constants used by
	// integrations.Handler. The organization-client /context route has no
	// object path parameter and is outside this object-route table.
	"POST /fleet-partner/v1/integrations/api-clients/:id/rotate":        "integrations.manage",
	"POST /fleet-partner/v1/integrations/api-clients/:id/revoke":        "integrations.manage",
	"PATCH /fleet-partner/v1/integrations/webhooks/:id":                 "webhooks.manage",
	"POST /fleet-partner/v1/integrations/webhooks/:id/rotate-secret":    "webhooks.manage",
	"GET /fleet-partner/v1/integrations/webhooks/:id/deliveries":        "integrations.read",
	"POST /fleet-partner/v1/integrations/webhook-deliveries/:id/replay": "webhooks.manage",
}

// selfOwnershipSentinel, internalCallerAllowlistSentinel and
// publicTokenSentinel are not empty strings: an empty RequiredPermission
// would be indistinguishable from "not yet classified" to a reviewer
// scanning the generated inventory. Each names the REAL authorization
// mechanism that route actually uses in place of a permission string, so
// the inventory stays honest about what gates the route rather than
// implying "nothing does".
const (
	selfOwnershipSentinel           = "(self-ownership check, not a permission)"
	internalCallerAllowlistSentinel = "(HMAC caller allowlist, not a permission)"
	publicTokenSentinel             = "(hashed single-resource token, not a permission)"
)

// publicCustomerTokenPaths names the exact routes gated by CRM's
// unauthenticated secure-link token rather than a corporate session at
// all -- Classify uses this to correctly report ExposurePublicCustomer
// (no organization predicate: there is no tenant context on this route to
// predicate against) instead of defaulting to ExposureOrganization purely
// because the path does not start with the internal prefix.
var publicCustomerTokenPaths = map[string]bool{
	"/fleet-partner/v1/quotations/:quotationId/view": true,
}

var providerRelationshipSegments = []string{
	"/providers/:providerId", "/providers/:membershipId",
}

// Classify derives a Declaration for one route. permission is whatever the
// live route manifest reported (empty for a manifestless-package route,
// resolved instead from manifestlessPermissions).
func Classify(method, path, permission string) Declaration {
	key := method + " " + path
	if permission == "" {
		if override, ok := manifestlessPermissions[key]; ok {
			permission = override
		}
	}

	exposure := ExposureOrganization
	switch {
	case strings.HasPrefix(path, "/internal/fleet-partner/v1"):
		exposure = ExposureInternal
	case strings.HasPrefix(path, "/fleet-partner/integrations/v1"):
		exposure = ExposureOrganizationAPI
	case publicCustomerTokenPaths[path]:
		exposure = ExposurePublicCustomer
	}

	actorClasses := []string{ActorOrganizationUser}
	providerRelationship := false
	switch exposure {
	case ExposureInternal:
		actorClasses = []string{ActorInternalService}
	case ExposurePublicCustomer:
		// No corporate/provider identity at all on this route -- the
		// hashed token IS the entire authorization decision, not a
		// stand-in for one of the other actor classes.
		actorClasses = []string{ActorPublicToken}
	case ExposureOrganizationAPI:
		actorClasses = []string{ActorOrganizationAPIClient}
	}
	for _, segment := range providerRelationshipSegments {
		if strings.Contains(path, segment) {
			providerRelationship = true
			if exposure == ExposureInternal {
				actorClasses = []string{ActorInternalService, ActorProvider}
			} else {
				actorClasses = append(actorClasses, ActorProvider)
			}
			break
		}
	}

	paramCount := strings.Count(path, "/:")

	return Declaration{
		Method:              method,
		Path:                path,
		AllowedActorClasses: actorClasses,
		RequiredPermission:  permission,
		// Every tenant-scoped route in this codebase resolves and filters
		// by t.OrganizationID before touching the database -- verified
		// directly, repeatedly, across every feature package read this
		// session; there is no tenant route in this codebase that skips
		// it. Internal routes have no organization predicate of their own
		// (they operate platform-wide, gated by caller identity instead).
		OrganizationPredicate: exposure == ExposureOrganization,
		// Branch-level filtering is a real, but narrower, concept in this
		// codebase (t.PermittedBranchIDs, and eligibility's own
		// evaluate/branchID-aware authorize call, confirmed by reading its
		// handler) -- not universal the way the organization predicate is.
		// Defaulted false; true only for the routes this pass could
		// concretely verify are branch-scoped.
		BranchPredicate:       isBranchScoped(path),
		ParentChildValidation: paramCount >= 2,
		ProviderRelationship:  providerRelationship,
		// This codebase's established, verified convention (read directly
		// in dozens of handlers this session) is to answer NotFound, never
		// Forbidden-with-detail, for a resource outside the caller's own
		// organization -- so as not to confirm another organization's
		// resource exists at all. The one PUBLIC_CUSTOMER route follows the
		// equivalent discipline for its own boundary (an invalid/expired
		// token gets a generic invalid-token response, never a signal that
		// distinguishes "wrong token" from "right token, wrong
		// quotation") -- true unconditionally, not just for tenant-scoped
		// exposure.
		SafeNotFound:      true,
		Exposure:          exposure,
		NeedsManualReview: permission == "",
	}
}

// isBranchScoped names the specific routes this pass could concretely
// verify filter by branch, not just organization -- eligibility's evaluate
// route passes body.BranchID into its own authorize call (read directly),
// and the branches/structure surface is inherently branch-scoped by
// definition. Deliberately a short, explicit list rather than a guess from
// the path containing "branch" as a substring.
func isBranchScoped(path string) bool {
	branchScopedPaths := map[string]bool{
		"/eligibility/evaluations": true,
	}
	return branchScopedPaths[path]
}

// SelfCheck is a production-safe well-formedness check, the objectauth
// counterpart to riskregistry.SelfCheck (see that function's doc comment
// for why this is a self-consistency check rather than the full
// every-mounted-route cross-reference internal/section28contract's
// TestObjectAuthorizationRegistry_CoversEveryMountedObjectRoute performs --
// that remains a CI/build-time concern, not one this package duplicates
// into the production binary). It exercises Classify against a small,
// fixed set of routes spanning every Exposure/sentinel case this registry
// actually classifies, confirming the manifestlessPermissions table and
// Classify's own logic still produce internally consistent answers (every
// invariant TestObjectAuthorizationRegistry_EveryDeclarationIsInternallyConsistent
// pins) rather than NeedsManualReview or a corrupted Declaration -- used by
// cmd/main.go's "object_authorization_registry" readiness check.
func SelfCheck() error {
	canaries := []struct{ method, path, permission string }{
		{"GET", "/fleet-partner/v1/assets/:assetId", "assets.read"},
		{"GET", "/internal/fleet-partner/v1/providers/:providerId/documents", ""},
		{"GET", "/fleet-partner/v1/quotations/:quotationId/view", ""},
		{"DELETE", "/fleet-partner/v1/sessions/:sessionId", ""},
	}
	for _, c := range canaries {
		decl := Classify(c.method, c.path, c.permission)
		if decl.NeedsManualReview {
			return fmt.Errorf("objectauth: canary route %s %s did not resolve a permission", c.method, c.path)
		}
		if len(decl.AllowedActorClasses) == 0 {
			return fmt.Errorf("objectauth: canary route %s %s resolved no allowed actor classes", c.method, c.path)
		}
		if !decl.SafeNotFound {
			return fmt.Errorf("objectauth: canary route %s %s did not resolve SafeNotFound", c.method, c.path)
		}
	}
	return nil
}
