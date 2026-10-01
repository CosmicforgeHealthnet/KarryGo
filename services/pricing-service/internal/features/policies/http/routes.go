package http

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/services/pricing-service/internal/platform/servicetrust"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// AdminOptions wires the administrative surface.
type AdminOptions struct {
	Handler    *Handler
	Verifier   *serviceauth.Verifier
	NonceStore database.Querier
	// AdminCallers may use these routes. Holding a secret is not permission: a
	// booking service can ask for a quote but must never change a rate.
	AdminCallers []string
}

// RegisterAdminRoutes mounts the administrative routes at the service root.
//
// They are NOT under the public API base. Nothing here is reachable by a customer
// or a driver: administration arrives as an HMAC-signed request from
// admin-backoffice-service or the ops CLI, and every signed request is single-use.
func RegisterAdminRoutes(router *gin.Engine, opts AdminOptions) {
	group := router.Group("/internal/pricing/v1/admin")
	group.Use(serviceauth.Middleware(opts.Verifier))
	group.Use(servicetrust.RejectReplayedNonce(opts.NonceStore))
	group.Use(servicetrust.RequireCallers(opts.AdminCallers...))

	// Policy lifecycle. Each step is a separate route because each is a separate
	// authority: drafting, submitting, deciding and activating are deliberately
	// not one "save" button.
	group.POST("/policies/drafts", opts.Handler.CreateDraft)
	group.GET("/policies/:id/versions", opts.Handler.ListVersions)
	group.GET("/policy-versions/:id", opts.Handler.GetVersion)
	group.PUT("/policy-versions/:id", opts.Handler.UpdateDraft)
	group.POST("/policy-versions/:id/submit", opts.Handler.Submit)
	group.POST("/policy-versions/:id/approve", opts.Handler.Approve)
	group.POST("/policy-versions/:id/reject", opts.Handler.Reject)
	group.POST("/policy-versions/:id/activate", opts.Handler.Activate)

	// Try a version on real inputs without storing anything.
	group.POST("/simulate", opts.Handler.Simulate)

	// "Which rates would price this place right now?" -- the first question asked
	// when a fare looks wrong.
	group.GET("/resolve", opts.Handler.Resolve)

	group.GET("/approvals", opts.Handler.ListApprovals)

	// The pricing half of the two-key switch.
	group.GET("/rollouts", opts.Handler.ListRollouts)
	group.POST("/rollouts", opts.Handler.SetRollout)

	group.GET("/markets/:code/freeze", opts.Handler.FreezeState)
	group.POST("/markets/:code/freeze", opts.Handler.Freeze)
	group.POST("/markets/:code/unfreeze", opts.Handler.Unfreeze)
}
