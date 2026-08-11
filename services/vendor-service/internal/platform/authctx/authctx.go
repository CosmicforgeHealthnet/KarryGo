// Package authctx is the shared (not feature-owned) request-context carrier
// for the fully-resolved Vendor Staff identity: session, staff, vendor
// scope. internal/features/auth's HTTP middleware resolves and sets it;
// every other feature's HTTP handlers (staff, roles, and later Section 6+
// business routes) read it via FromContext — never trusting a vendor_id or
// branch_id supplied by the request body/path for authorization purposes
// (Section 5 rule 9/17).
//
// Living outside any one feature's http package is deliberate: both
// internal/features/auth/http and internal/features/staff/http need this
// type, and a feature importing another feature's http package is exactly
// what internal/architecture_test.go forbids.
package authctx

import "github.com/gin-gonic/gin"

const contextKey = "vendor_staff_context"

// StaffContext is the authenticated actor for the current request, resolved
// server-side from the session referenced by the access token — never from
// client-supplied claims beyond the opaque session id.
type StaffContext struct {
	StaffID   string
	VendorID  string
	SessionID string
}

func Set(c *gin.Context, ctx StaffContext) {
	c.Set(contextKey, ctx)
}

func FromContext(c *gin.Context) (StaffContext, bool) {
	value, ok := c.Get(contextKey)
	if !ok {
		return StaffContext{}, false
	}
	ctx, ok := value.(StaffContext)
	return ctx, ok
}
