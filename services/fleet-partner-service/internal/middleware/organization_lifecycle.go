package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// OrganizationStatusFunc reads an organization's authoritative lifecycle status.
type OrganizationStatusFunc func(ctx context.Context, organizationID string) (string, error)

// OperationalOrganizationStatus is the only lifecycle state in which an
// organization may change operational data. It matches
// organizations/usecases.IsOperational.
const OperationalOrganizationStatus = "ACTIVE"

// PreActivationWriteRoutes are the state-changing corporate routes an
// organization may use before it is ACTIVE: completing and correcting its
// own onboarding, submitting it, closing it (the closure workflow enforces
// its own lifecycle rules), managing its own staff, security settings and
// sessions, and contacting support. Every other state-changing route is
// refused until the organization is approved and activated.
var PreActivationWriteRoutes = map[string]bool{
	"PUT /fleet-partner/v1/organizations/:organizationId":                                   true,
	"PUT /fleet-partner/v1/organizations/:organizationId/stages/:stage":                     true,
	"POST /fleet-partner/v1/organizations/:organizationId/representatives":                  true,
	"PUT /fleet-partner/v1/organizations/:organizationId/representatives/:representativeId": true,
	"PUT /fleet-partner/v1/organizations/:organizationId/capabilities":                      true,
	"PUT /fleet-partner/v1/organizations/:organizationId/profile":                           true,
	"POST /fleet-partner/v1/organizations/:organizationId/documents/upload-authorization":   true,
	"POST /fleet-partner/v1/organizations/:organizationId/documents/upload-content":         true,
	"POST /fleet-partner/v1/organizations/:organizationId/documents":                        true,
	"DELETE /fleet-partner/v1/organizations/:organizationId/documents/:documentId":          true,
	"POST /fleet-partner/v1/organizations/:organizationId/submit":                           true,
	"POST /fleet-partner/v1/organizations/:organizationId/wallet-provisioning/retry":        true,
	"POST /fleet-partner/v1/organizations/:organizationId/close-request":                    true,
	"POST /fleet-partner/v1/organizations/:organizationId/closure/assess":                   true,
	"POST /fleet-partner/v1/organizations/:organizationId/closure/approve":                  true,
	"POST /fleet-partner/v1/organizations/:organizationId/closure/reject":                   true,
	"POST /fleet-partner/v1/organizations/:organizationId/closure/execute":                  true,
	"POST /fleet-partner/v1/staff/invitations/:invitationId/resend":                         true,
	"DELETE /fleet-partner/v1/staff/invitations/:invitationId":                              true,
	"POST /fleet-partner/v1/branches":                                                       true,
	"PATCH /fleet-partner/v1/branches/:branchId":                                            true,
	"POST /fleet-partner/v1/branches/:branchId/archive":                                     true,
	"POST /fleet-partner/v1/departments":                                                    true,
	"PATCH /fleet-partner/v1/departments/:departmentId":                                     true,
	"POST /fleet-partner/v1/departments/:departmentId/archive":                              true,
	"PATCH /fleet-partner/v1/organization-settings":                                         true,
	"PATCH /fleet-partner/v1/staff/:staffId":                                                true,
	"PUT /fleet-partner/v1/staff/:staffId/roles":                                            true,
	"POST /fleet-partner/v1/roles":                                                          true,
	"PATCH /fleet-partner/v1/roles/:roleId":                                                 true,
	"POST /fleet-partner/v1/roles/:roleId/archive":                                          true,
	"POST /fleet-partner/v1/staff/invitations":                                              true,
	"POST /fleet-partner/v1/staff/:staffId/suspend":                                         true,
	"POST /fleet-partner/v1/staff/:staffId/reactivate":                                      true,
	"PUT /fleet-partner/v1/security-settings":                                               true,
	"DELETE /fleet-partner/v1/sessions/:sessionId":                                          true,
	"POST /fleet-partner/v1/support/cases":                                                  true,
}

// RequireOperationalOrganization refuses state-changing tenant requests from
// an organization that is not ACTIVE, except PreActivationWriteRoutes.
//
// A company that has submitted its onboarding may sign in and explore the
// dashboard while it is verified, but it must not create orders, dispatch,
// onboard providers, move money or otherwise operate until approval. The
// frontend hides and explains those actions; this is the server-side
// guarantee, so calling the API directly does not bypass it. Reads are
// unaffected: they are already confined to the caller's own organization.
//
// The status is read from the database on every state-changing request, so
// approval takes effect immediately and a stale client claim cannot unlock
// anything. A failed read refuses the request.
func RequireOperationalOrganization(status OrganizationStatusFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		if PreActivationWriteRoutes[c.Request.Method+" "+c.FullPath()] {
			c.Next()
			return
		}
		tenantCtx, ok := TenantFromGin(c)
		if !ok {
			// ResolveTenant already refused unscoped requests; nothing to gate.
			c.Next()
			return
		}
		current, err := status(c.Request.Context(), tenantCtx.OrganizationID)
		if err != nil {
			httpx.Abort(c, errorcodes.Unavailable(errorcodes.FleetServiceUnavailable, "Company status could not be confirmed. Please try again shortly."))
			return
		}
		if current == OperationalOrganizationStatus {
			c.Next()
			return
		}
		httpx.Abort(c, organizationNotOperational(current))
	}
}

func organizationNotOperational(status string) *apperrors.Error {
	switch status {
	case "BUSINESS_SUBMITTED", "UNDER_REVIEW":
		return errorcodes.Forbidden(errorcodes.OrganizationVerificationPending,
			"Company verification required. This feature will be available after your company is approved.")
	case "CHANGES_REQUIRED":
		return errorcodes.Forbidden(errorcodes.OrganizationChangesRequired,
			"Update your company details and resubmit them for review before using this feature.")
	case "DRAFT", "PENDING_REVIEW", "CONTACT_VERIFIED":
		return errorcodes.Forbidden(errorcodes.OrganizationOnboardingIncomplete,
			"Finish setting up your company and submit it for review before using this feature.")
	default:
		return errorcodes.Forbidden(errorcodes.OrganizationNotEligible,
			"This feature is not available while your company is not active.")
	}
}
