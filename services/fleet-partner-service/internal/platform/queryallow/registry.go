// Package queryallow is the allowlist of query parameters each corporate
// (tenant-scoped) GET collection route accepts.
//
// Spec 6 and 36 require explicit, allowlisted filters and the rejection of
// unknown ones. Before this registry only the CRM handlers enforced that; every
// other list route silently ignored an unrecognised parameter, which is the
// failure mode where a caller believes it narrowed a list and was handed
// everything instead. middleware.QueryAllowlist looks each request up here by
// its matched route pattern (gin FullPath) and rejects, with HTTP 422:
//
//   - any parameter the route does not declare;
//   - a parameter supplied more than once;
//   - a malformed limit, offset, cursor or boolean value (platform/pagination
//     holds the bounds: limit 1-200, offset 0-10000).
//
// Every route here was derived from the handler code that reads the query
// (c.Query and its helpers), not guessed, and cross-checked against what the
// Fleet web app sends (web/fleet/src/lib/services). A route that reads nothing
// is declared with no parameters, so a caller cannot pass one by mistake.
//
// Coverage is enforced by TestTenantListRoutesRejectUnknownQueryAndBadPaging
// (internal/integration): every mounted tenant GET route whose last path
// segment is static (a collection or singleton read) must appear here.
// Routes ending in a path parameter (single-record reads) are not required to.
//
// Adding a filter to a handler means adding its name here in the same change;
// TestRegistryCoversEveryQueryNameReadByHandlers fails if a handler reads a
// parameter name that no route declares.
package queryallow

import (
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/filters"
)

// TenantPrefix is the mount point of the corporate tenant routes.
const TenantPrefix = "/fleet-partner/v1"

// Shorthands for the two pagination styles. A route declares exactly the one
// it implements.
const (
	// offsetPage expands to limit + offset (backward-compatible v1 lists).
	offsetPage = "@offset"
	// cursorPage expands to cursor + limit (opaque cursor lists).
	cursorPage = "@cursor"
	// limitOnly is a capped list with no continuation.
	limitOnly = "limit"
)

// crmList are the parameters every CRM list endpoint already permitted through
// its own allowedQuery helper, kept identical so this registry is no stricter
// and no looser than the CRM contract.
func crmList(filters ...string) []string {
	return append(filters, cursorPage, "sort")
}

// dispatchBoard is the shared control-tower filter (dispatch.parseFilter);
// the board and control tower apply every field of it.
var dispatchBoard = []string{"branchId", "status", "service", "providerId", "vehicleOrTruckReference", "alertType", offsetPage}

// declared maps a GET route pattern below TenantPrefix to its parameters.
// Boolean parameters carry the ":bool" suffix.
var declared = map[string][]string{
	// Organization structure, staff and providers.
	"/branches":                     {},
	"/departments":                  {"branchId", "includeArchived:bool"},
	"/cost-centres":                 {"branchId", "includeArchived:bool"},
	"/teams":                        {"branchId", "departmentId", "includeArchived:bool"},
	"/locations":                    {"branchId", "locationType", "includeArchived:bool"},
	"/organization-settings":        {},
	"/organizations/current":        {},
	"/permissions":                  {},
	"/roles":                        {},
	"/staff":                        {cursorPage},
	"/staff/invitations":            {},
	"/staff/:staffId/role-bindings": {},
	"/organizations/:organizationId/activation-readiness":          {},
	"/organizations/:organizationId/capabilities":                  {},
	"/organizations/:organizationId/closure-status":                {},
	"/organizations/:organizationId/documents":                     {},
	"/organizations/:organizationId/documents/:documentId/preview": {},
	"/organizations/:organizationId/onboarding-status":             {},
	"/organizations/:organizationId/profile":                       {},
	"/organizations/:organizationId/representatives":               {},
	"/organizations/:organizationId/verification":                  {},
	"/organizations/:organizationId/wallet-provisioning":           {},
	"/providers":                                      {cursorPage},
	"/providers/:membershipId/availability":           {},
	"/providers/:membershipId/branches":               {},
	"/providers/:membershipId/capabilities":           {},
	"/providers/:membershipId/compliance":             {},
	"/providers/:membershipId/declines":               {},
	"/providers/:membershipId/history":                {cursorPage},
	"/providers/:membershipId/policies":               {},
	"/providers/:membershipId/provisioning-status":    {},
	"/providers/:membershipId/qualifications":         {},
	"/providers/:membershipId/service-roles":          {},
	"/providers/:membershipId/statements":             {},
	"/providers/:membershipId/vehicles":               {"includeHistory:bool"},
	"/providers/:membershipId/sub-wallet":             {},
	"/providers/:membershipId/sub-wallet/allocations": {"status", "allocationType", cursorPage},
	"/providers/:membershipId/sub-wallet/statement":   {},
	"/providers/:membershipId/sub-wallet/withdrawals": {"status", cursorPage},
	"/managed-members":                                {cursorPage},
	"/managed-assets":                                 {"memberId", cursorPage},
	"/fleet-partner/operating-mode":                   {},
	"/dashboard":                                      {},

	// Approvals.
	"/approvals/requests":                       {limitOnly, "resourceType", "resourceId", "action", "status"},
	"/approvals/requests/:approvalId/decisions": {},
	"/approvals/requests/:approvalId/history":   {},
	"/approval-policies":                        {"resourceType", "action", "status", "origin"},
	"/approval-policies/:policyId/versions":     {},
	"/approval-delegations":                     {limitOnly, "delegatorId", "delegateId", "status"},

	// Audit, security, retention, background operations.
	"/audit-logs": {"branchId", "actorType", "actorId", "action", "resourceType", "resourceId",
		"requestId", "correlationId", "highRiskOnly:bool", "from", "to", cursorPage},
	"/security-settings":                    {},
	"/legal-holds":                          {"scopeType", "scopeReference", "status", limitOnly},
	"/retention-policies":                   {},
	"/retention-runs/:runId/records":        {},
	"/jobs":                                 {limitOnly, "status", "jobType"},
	"/jobs/:jobId/attempts":                 {},
	"/scheduled-tasks":                      {limitOnly},
	"/scheduled-tasks/:taskId/runs":         {limitOnly},
	"/exports":                              {limitOnly},
	"/imports":                              {},
	"/imports/:importId/rows":               {},
	"/orders/imports":                       {},
	"/search":                               {limitOnly, "q", "subjectType", "exactReference", "status"},
	"/integrations/api-clients":             {},
	"/integrations/webhooks":                {},
	"/integrations/webhooks/:id/deliveries": {},
	"/synchronization/dead-letters":         {},
	"/synchronization/projections":          {},
	"/synchronization/reconciliation-cases": {},

	// Documents, eligibility, safety, compliance.
	"/documents": {limitOnly, "ownerType", "ownerId", "branchId", "category", "documentType",
		"lifecycleState", "visibility", "includeRetired:bool", "includeArchived:bool"},
	"/documents/reconciliation-cases":            {"documentId"},
	"/documents/:documentId/deletion":            {},
	"/documents/:documentId/history":             {},
	"/documents/:documentId/verifications":       {},
	"/eligibility/decisions":                     {limitOnly, "subjectType", "subjectId", "operationType", "status", "currentOnly:bool"},
	"/eligibility/decisions/:decisionId/history": {},
	"/eligibility/policies":                      {"policyKey"},
	"/safety/references":                         {limitOnly, "referenceType", "assignmentId", "platformTripId", "executingProviderId"},
	"/safety/references/:referenceId/history":    {},
	"/safety/reports":                            {},
	"/incidents":                                 {},
	"/incidents/:incidentId/investigations":      {},
	"/compliance/checks":                         {"subjectType", "subjectReference"},
	"/compliance/requirements":                   {},
	"/compliance/restrictions":                   {"subjectType", "subjectReference", "state"},
	"/compliance/status":                         {"subjectType", "subjectReference"},
	"/proof-policies":                            {},
	"/proof-records":                             {},

	// Finance and wallet.
	"/finance/operations":                      {"status", "operationType", "subjectId", limitOnly},
	"/finance/operations/:operationId/audit":   {limitOnly},
	"/finance/operations/:operationId/history": {limitOnly},
	"/finance/projections":                     {"projectionType", "freshness", "subjectId", limitOnly},
	"/finance/reconciliation-cases":            {"projectionType", "subjectId", limitOnly},
	"/wallet":                                  {"authoritative:bool"},
	"/wallet/policy":                           {},
	"/wallet/transactions":                     {"walletReference", "category", "direction", cursorPage},
	"/budgets":                                 {},
	"/claims":                                  {},
	"/compensation-rules":                      {},
	"/expenses":                                {},
	"/invoices":                                {},
	"/payroll-periods":                         {},
	"/receivables":                             {},
	"/subcontractor-settlements":               {},

	// CRM (mirrors crm/http allowedQuery exactly).
	"/customers": crmList("status", "riskStatus", "customerType", "branchId", "accountOwnerUserId",
		"createdAfter", "createdBefore", "search"),
	"/customers/:customerId/contacts":               crmList("status", "contactPurpose"),
	"/customers/:customerId/locations":              crmList("locationType", "status"),
	"/customers/:customerId/credit-profile":         {},
	"/customers/:customerId/credit-profile/history": {cursorPage},
	"/customers/:customerId/history":                {cursorPage},
	"/leads":                                        crmList("status", "source", "ownerUserId", "branchId", "createdAfter", "createdBefore"),
	"/leads/:leadId/history":                        {cursorPage},
	"/sales/opportunities":                          crmList("stage", "customerId", "branchId", "accountOwnerUserId", "createdAfter", "createdBefore"),
	"/sales/opportunities/:opportunityId/history":   {cursorPage},
	"/quotations":                                   crmList("status", "customerId", "opportunityId", "branchId", "createdAfter", "createdBefore"),
	"/quotations/:quotationId/documents":            {},
	"/quotations/:quotationId/history":              {cursorPage},
	"/quotations/:quotationId/versions":             {cursorPage},
	"/contracts":                                    crmList("status", "customerId", "branchId", "createdAfter", "createdBefore"),
	"/contracts/:contractId/documents":              {},
	"/contracts/:contractId/history":                {cursorPage},
	"/contracts/:contractId/versions":               {cursorPage},
	"/service-level-agreements":                     crmList("status", "customerId", "contractId", "metricCode"),
	"/service-level-agreements/:slaId/history":      {cursorPage},
	"/service-level-agreements/:slaId/observations": crmList("outcome"),
	"/crm/notification-operations":                  crmList("status", "subjectId"),

	// Orders, shipments, packages, manifests.
	"/service-catalog":                     {limitOnly, "status", "category", "branchId", "cursor"},
	"/orders":                              {limitOnly, "status", "customerId", "branchId", "cursor"},
	"/orders/:orderId/history":             {},
	"/orders/:orderId/specialized-data":    {},
	"/orders/:orderId/stops":               {},
	"/shipments":                           {limitOnly, "status", "orderId", "branchId", "cursor"},
	"/shipments/:shipmentId/cargo-items":   {},
	"/shipments/:shipmentId/packages":      {},
	"/packages/:packageId/custody":         {},
	"/packages/:packageId/history":         {},
	"/custody-events":                      {"subjectType", "subjectReference"},
	"/manifests":                           {"status", "branchId"},
	"/recurring-orders":                    {"status"},
	"/private-assignments":                 {"status"},
	"/specialized/capabilities":            {},
	"/specialized/:capabilityCode/records": {},

	// Dispatch, routes, tracking.
	"/dispatch/board":        dispatchBoard,
	"/control-tower":         dispatchBoard,
	"/dispatch/unassigned":   {"branchId"},
	"/dispatch/candidates":   {"orderId", "shipmentId", "routeId", "routeStopId", "branchId"},
	"/dispatch/plans":        {"branchId", "status", offsetPage},
	"/routes":                {"branchId", "status", "service", "providerId", "vehicleOrTruckReference", offsetPage},
	"/routes/:routeId/stops": {},
	"/tracking":              {"providerId", "vehicleOrTruckReference", offsetPage},
	"/geofences":             {"branchId", offsetPage},
	"/geofence-events":       {"geofenceId", "canonicalTripReference", "eventRule", "branchId", "from", "to", offsetPage},
	"/alerts":                {"branchId", "status", "alertType", offsetPage},
	"/rescue-cases":          {"branchId", "status", "triggerType", offsetPage},

	// Marketplace.
	"/marketplace/offers":                            {limitOnly, "status", "branchId", "cursorSortValue", "cursorLastId"},
	"/marketplace/assignments":                       {limitOnly, "status", "branchId", "cursorSortValue", "cursorLastId"},
	"/marketplace/assignments/:assignmentId/history": {},
	"/marketplace/metrics":                           {},
	"/marketplace/requests/:projectionId/candidates": {},
	"/marketplace/trips":                             {"status", "sourceService", "membershipId", offsetPage},

	// Workforce.
	"/workforce/profiles":   {"branchId", "status", offsetPage},
	"/shifts":               {"branchId", "status", offsetPage},
	"/shift-templates":      {"branchId", "status", offsetPage},
	"/leave":                {"status", offsetPage},
	"/qualifications":       {},
	"/training/courses":     {},
	"/training/enrollments": {"membershipId", "status", offsetPage},

	// Assets and maintenance.
	"/assets":                         {"branchId", "status", offsetPage},
	"/assets/:assetId/compliance":     {},
	"/assets/:assetId/costs":          {},
	"/assets/:assetId/documents":      {},
	"/assets/:assetId/history":        {},
	"/assets/:assetId/meter-readings": {"meterType"},
	"/inspections":                    {"status"},
	"/inspection-templates":           {},
	"/defects":                        {"assetId", "status", "severity"},
	"/tyres":                          {"assetId", "status"},
	"/breakdowns":                     {},
	"/fuel-records":                   {},
	"/maintenance/schedules":          {},
	"/maintenance/work-orders":        {},
	"/inventory/parts":                {},
	"/inventory/stock-movements":      {},

	// Warehouse.
	"/warehouses":                           {},
	"/warehouses/:warehouseId/locations":    {},
	"/inventory":                            {},
	"/inventory/:inventoryItemId/movements": {},
	"/dock-appointments":                    {},
	"/fulfilment-orders":                    {},
	"/returns":                              {},
	"/terminal-transfers":                   {},
	"/yard-visits":                          {},

	// Engagement.
	"/communications/threads":                    {},
	"/communications/threads/:threadId/messages": {},
	"/escalations":                               {},
	"/notification-rules":                        {},
	"/quality/cases":                             {},
	"/sla/measurements":                          {},
	"/support/cases":                             {},
}

var schemas = buildSchemas()

func buildSchemas() map[string]filters.Schema {
	out := make(map[string]filters.Schema, len(declared))
	for path, params := range declared {
		fields := map[string]filters.Field{}
		for _, param := range params {
			for name, field := range expand(param) {
				fields[name] = field
			}
		}
		out["GET "+TenantPrefix+path] = filters.Schema{Fields: fields}
	}
	return out
}

func expand(param string) map[string]filters.Field {
	switch param {
	case offsetPage:
		return map[string]filters.Field{"limit": {Kind: filters.KindLimit}, "offset": {Kind: filters.KindOffset}}
	case cursorPage:
		return map[string]filters.Field{"cursor": {Kind: filters.KindCursor}, "limit": {Kind: filters.KindLimit}}
	case "limit":
		return map[string]filters.Field{"limit": {Kind: filters.KindLimit}}
	case "offset":
		return map[string]filters.Field{"offset": {Kind: filters.KindOffset}}
	case "cursor":
		return map[string]filters.Field{"cursor": {Kind: filters.KindCursor}}
	}
	if name, ok := strings.CutSuffix(param, ":bool"); ok {
		return map[string]filters.Field{name: {Kind: filters.KindBool}}
	}
	return map[string]filters.Field{param: {Kind: filters.KindString}}
}

// Lookup returns the allowlist schema for a route pattern, as gin reports it
// through FullPath(). ok is false for routes that are not declared.
func Lookup(method, fullPath string) (filters.Schema, bool) {
	schema, ok := schemas[method+" "+fullPath]
	return schema, ok
}

// Declared returns every declared route key ("GET <full path>").
func Declared() []string {
	out := make([]string, 0, len(schemas))
	for key := range schemas {
		out = append(out, key)
	}
	return out
}

// Names returns every parameter name declared by any route.
func Names() map[string]bool {
	out := map[string]bool{}
	for _, schema := range schemas {
		for name := range schema.Fields {
			out[name] = true
		}
	}
	return out
}
