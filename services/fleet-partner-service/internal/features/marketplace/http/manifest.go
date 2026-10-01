package http

import "github.com/gin-gonic/gin"

// Permission codes enforced by the corporate marketplace handler. They are
// seeded by migration 000076_marketplace_permissions. A permission that is
// not seeded cannot be granted, so every route here is deny-by-default until
// that migration runs.
const (
	PermissionMarketplaceRead     = "marketplace.read"
	PermissionMarketplaceAccept   = "marketplace.accept"
	PermissionMarketplaceDecline  = "marketplace.decline"
	PermissionMarketplaceAssign   = "marketplace.assign"
	PermissionMarketplaceCancel   = "marketplace.cancel"
	PermissionMarketplaceReassign = "marketplace.reassign"
)

// Route describes one corporate marketplace route.
type Route struct {
	Method                  string
	Path                    string
	OperationID             string
	Permission              string
	AuthClass               string
	IdempotencyRequired     bool
	ExpectedVersionRequired bool
}

// TenantRoutes is the authoritative corporate marketplace route table.
func TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/marketplace/offers", OperationID: "listCorporateOffers", Permission: PermissionMarketplaceRead, AuthClass: "corporate"},
		{Method: "GET", Path: "/marketplace/offers/:offerId", OperationID: "getCorporateOffer", Permission: PermissionMarketplaceRead, AuthClass: "corporate"},
		{Method: "GET", Path: "/marketplace/metrics", OperationID: "getCorporateOfferMetrics", Permission: PermissionMarketplaceRead, AuthClass: "corporate"},
		{Method: "GET", Path: "/marketplace/requests/:projectionId/candidates", OperationID: "listCorporateCandidates", Permission: PermissionMarketplaceRead, AuthClass: "corporate"},
		{Method: "GET", Path: "/marketplace/assignments", OperationID: "listCorporateAssignments", Permission: PermissionMarketplaceRead, AuthClass: "corporate"},
		{Method: "GET", Path: "/marketplace/assignments/:assignmentId", OperationID: "getCorporateAssignment", Permission: PermissionMarketplaceRead, AuthClass: "corporate"},
		{Method: "GET", Path: "/marketplace/assignments/:assignmentId/history", OperationID: "listCorporateAssignmentHistory", Permission: PermissionMarketplaceRead, AuthClass: "corporate"},

		{Method: "POST", Path: "/marketplace/offers/:offerId/accept", OperationID: "acceptCorporateOffer", Permission: PermissionMarketplaceAccept, AuthClass: "corporate", IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/marketplace/offers/:offerId/decline", OperationID: "declineCorporateOffer", Permission: PermissionMarketplaceDecline, AuthClass: "corporate", IdempotencyRequired: true},
		{Method: "POST", Path: "/marketplace/requests/:projectionId/assign", OperationID: "assignCorporateRequest", Permission: PermissionMarketplaceAssign, AuthClass: "corporate", IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/marketplace/assignments/:assignmentId/cancel", OperationID: "cancelCorporateAssignment", Permission: PermissionMarketplaceCancel, AuthClass: "corporate", IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/marketplace/assignments/:assignmentId/reassign", OperationID: "reassignCorporateAssignment", Permission: PermissionMarketplaceReassign, AuthClass: "corporate", IdempotencyRequired: true},
	}
}

// RegisterTenantRoutes mounts the corporate marketplace routes.
//
// The operation map is exhaustive and static: adding a route to TenantRoutes
// without adding its handler fails at this site rather than silently
// returning a nil handler at runtime.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	handlers := map[string]gin.HandlerFunc{
		"listCorporateOffers":            h.ListOffers,
		"getCorporateOffer":              h.GetOffer,
		"getCorporateOfferMetrics":       h.GetOfferMetrics,
		"listCorporateCandidates":        h.ListCandidates,
		"listCorporateAssignments":       h.ListAssignments,
		"getCorporateAssignment":         h.GetAssignment,
		"listCorporateAssignmentHistory": h.ListAssignmentHistory,
		"acceptCorporateOffer":           h.AcceptOffer,
		"declineCorporateOffer":          h.DeclineOffer,
		"assignCorporateRequest":         h.Assign,
		"cancelCorporateAssignment":      h.CancelAssignment,
		"reassignCorporateAssignment":    h.ReassignAssignment,
	}
	for _, r := range TenantRoutes() {
		handler, ok := handlers[r.OperationID]
		if !ok || handler == nil {
			panic("marketplace: route " + r.OperationID + " has no registered handler")
		}
		group.Handle(r.Method, r.Path, handler)
	}
}
