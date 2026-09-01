// Package http implements the Section 30 provider membership-summary
// internal surface: a single service-authenticated read that lets an
// owning provider service (driver-hauling-service,
// driver-dispatch-delivery-service, driver-taxi-service) discover a
// provider's providerMode and, when organization-managed, a tenant-safe
// organization/assignment/field-visibility summary -- to splice into ITS
// OWN provider login/session response. fleet-partner-service issues no
// provider session itself; see usecases.MembershipSummary's doc comment.
package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
)

// Route is one mounted internal operation, following the established
// manifest convention used by every other feature's HTTP surface (see
// backgroundops/http/manifest.go).
type Route struct {
	Method      string
	Path        string
	OperationID string
}

// InternalHandler serves the provider membership-summary internal surface.
type InternalHandler struct {
	service *usecases.Service
	allowed map[string]bool
}

// NewInternalHandler builds the handler with a caller allowlist, mirroring
// backgroundops.NewInternalHandler.
func NewInternalHandler(service *usecases.Service, allowedCallers []string) *InternalHandler {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		if name := strings.TrimSpace(caller); name != "" {
			allowed[name] = true
		}
	}
	return &InternalHandler{service: service, allowed: allowed}
}

// InternalRoutes is the HMAC-authenticated provider membership-summary
// surface, mounted under /internal/fleet-partner/v1 by the composition
// root.
func InternalRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/providers/:providerId/services/:providerService/membership-summary", OperationID: "getProviderMembershipSummary"},
	}
}

// RegisterInternalRoutes mounts the provider membership-summary route.
func RegisterInternalRoutes(group *gin.RouterGroup, handler *InternalHandler) {
	for _, route := range InternalRoutes() {
		group.Handle(route.Method, route.Path, handler.dispatch(route.OperationID))
	}
}

func (h *InternalHandler) dispatch(operationID string) gin.HandlerFunc {
	if operationID != "getProviderMembershipSummary" {
		panic("providers/http: no internal handler registered for operation " + operationID)
	}
	return h.getMembershipSummary
}

func (h *InternalHandler) requireAllowedCaller(c *gin.Context) bool {
	caller := c.GetString(middleware.ServiceCallerKey)
	if !h.allowed[caller] {
		c.JSON(http.StatusForbidden, gin.H{"status": "REJECTED", "code": "caller_not_allowed"})
		return false
	}
	return true
}

// branchView is omitted from the response entirely when the membership has
// no branch, or the branch lookup failed -- never emitted as a
// present-but-empty object.
type branchView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type organizationSummaryView struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"displayName"`
	Status      string      `json:"status"`
	Branch      *branchView `json:"branch,omitempty"`
}

type membershipView struct {
	Status          string `json:"status"`
	Role            string `json:"role"`
	ServiceCategory string `json:"serviceCategory"`
}

type assignmentPolicyView struct {
	AssignmentMode          string `json:"assignmentMode"`
	ProviderDeclineRequired bool   `json:"providerDeclineRequired"`
}

// membershipSummaryView is the wire response. Every field beyond
// providerMode is additive and OMITTED (not null, not empty-string) for an
// INDEPENDENT provider -- see Section 30 WP16: an old or new client that
// only reads providerMode is unaffected either way, and a client that
// checks "is organization present" gets a clean, unambiguous signal.
type membershipSummaryView struct {
	ProviderMode          string                   `json:"providerMode"`
	Organization          *organizationSummaryView `json:"organization,omitempty"`
	Membership            *membershipView          `json:"membership,omitempty"`
	AssignmentPolicy      *assignmentPolicyView    `json:"assignmentPolicy,omitempty"`
	FieldVisibilityPolicy string                   `json:"fieldVisibilityPolicy,omitempty"`
}

func newMembershipSummaryView(summary usecases.MembershipSummary) membershipSummaryView {
	view := membershipSummaryView{ProviderMode: summary.ProviderMode}
	if summary.ProviderMode != models.ProviderModeOrganizationManaged {
		return view
	}
	if summary.Organization != nil {
		orgView := &organizationSummaryView{
			ID:          summary.Organization.ID,
			DisplayName: summary.Organization.DisplayName,
			Status:      summary.Organization.Status,
		}
		if summary.Organization.BranchID != "" {
			orgView.Branch = &branchView{
				ID:   summary.Organization.BranchID,
				Name: summary.Organization.BranchName,
				Type: summary.Organization.BranchType,
			}
		}
		view.Organization = orgView
	}
	view.Membership = &membershipView{
		Status:          summary.MembershipStatus,
		Role:            summary.MembershipRole,
		ServiceCategory: summary.ServiceCategory,
	}
	view.AssignmentPolicy = &assignmentPolicyView{
		AssignmentMode:          summary.AssignmentMode,
		ProviderDeclineRequired: summary.ProviderDeclineRequired,
	}
	view.FieldVisibilityPolicy = summary.FieldVisibilityPolicy
	return view
}

// getMembershipSummary never trusts a caller-supplied provider mode -- the
// path only ever names WHICH provider to look up; the mode itself is
// always derived fresh from the authoritative membership row (Section 30
// WP19).
func (h *InternalHandler) getMembershipSummary(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	providerService := strings.TrimSpace(c.Param("providerService"))
	providerID := strings.TrimSpace(c.Param("providerId"))
	if providerService == "" || providerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "REJECTED", "code": "provider_identity_required"})
		return
	}

	summary, err := h.service.GetMembershipSummaryForProvider(c.Request.Context(), providerService, providerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
		return
	}
	c.JSON(http.StatusOK, newMembershipSummaryView(summary))
}
