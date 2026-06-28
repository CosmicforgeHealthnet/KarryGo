package supporthttp

import (
	"time"

	"github.com/gin-gonic/gin"

	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// ProviderGroup describes one provider-app bearer surface (taxi / dispatch /
// hauling). Each provider service signs tokens with its own secret and embeds a
// distinct role+service claim, so each needs its own group + secret.
type ProviderGroup struct {
	Prefix  string // e.g. "/provider", "/taxi-provider"
	Secret  []byte
	Role    string // token claim role, e.g. "truck_provider"
	Service string // token claim service, e.g. "hauling"
}

// RegisterRoutes wires all support & dispute routes.
//
// Customer / provider callers use bearer tokens (per-service secret + exact
// role+service match). Admin / internal callers use HMAC service-auth.
func RegisterRoutes(
	group *gin.RouterGroup,
	service *supportusecases.SupportService,
	customerAccessSecret []byte,
	providerGroups []ProviderGroup,
	serviceSecrets serviceauth.Secrets,
) {
	handler := NewHandler(service)

	// ── Public self-service (no auth) ────────────────────────────────────────
	group.GET("/support/categories", handler.Categories)
	group.GET("/support/faqs", handler.FAQs)

	// ── Customer routes ──────────────────────────────────────────────────────
	customer := group.Group("")
	customer.Use(sharedauth.BearerMiddleware(
		sharedauth.NewTokenSigner(customerAccessSecret),
		"customer",
		"customer",
	))
	registerCallerRoutes(customer, handler, true)

	// ── Provider routes (taxi / dispatch / hauling) ──────────────────────────
	// Same handlers; the complainant identity is derived from the token claims.
	// The role/service claim values must match whatever each driver service
	// adopts when it implements provider auth (hauling uses truck_provider/
	// hauling today; taxi/dispatch assumed taxi/dispatch by that convention).
	for _, pg := range providerGroups {
		if len(pg.Secret) == 0 {
			continue
		}
		grp := group.Group(pg.Prefix)
		grp.Use(sharedauth.BearerMiddleware(sharedauth.NewTokenSigner(pg.Secret), pg.Role, pg.Service))
		registerCallerRoutes(grp, handler, false)
	}

	// ── Admin / internal routes (service-auth) ───────────────────────────────
	admin := group.Group("/admin")
	admin.Use(serviceauth.Middleware(serviceauth.NewVerifier(serviceSecrets, 5*time.Minute)))
	admin.GET("/complaints", handler.AdminListComplaints)
	admin.GET("/complaints/:id", handler.AdminGetComplaint)
	admin.GET("/complaints/:id/evidence", handler.AdminListEvidence)
	admin.GET("/complaints/:id/dispute", handler.AdminGetDispute)
	admin.GET("/complaints/:id/events", handler.AdminListEvents)
	admin.POST("/complaints/:id/refresh-identity", handler.AdminRefreshIdentity)
	admin.PUT("/complaints/:id/status", handler.AdminUpdateStatus)
	admin.GET("/disputes", handler.AdminListDisputes)
	admin.POST("/disputes/:id/resolve", handler.AdminResolveDispute)
	admin.POST("/complaints/:id/messages", handler.AdminSendMessage)
	admin.GET("/complaints/:id/messages", handler.AdminListMessages)
}

// registerCallerRoutes wires the shared complaint/chat surface for a bearer
// caller group. includeDispute adds the customer-only dispute escalation/read.
func registerCallerRoutes(grp *gin.RouterGroup, handler *Handler, includeDispute bool) {
	grp.POST("/complaints", handler.CreateComplaint)
	grp.GET("/complaints", handler.MyComplaints)
	grp.GET("/complaints/:id", handler.GetComplaint)
	grp.POST("/complaints/:id/evidence", handler.AddEvidence)
	grp.GET("/complaints/:id/evidence", handler.ListEvidence)
	grp.POST("/support-chat/start", handler.StartSupportChat)
	grp.POST("/complaints/:id/messages", handler.SendMessage)
	grp.GET("/complaints/:id/messages", handler.ListMessages)
	grp.POST("/complaints/:id/messages/read", handler.MarkMessagesRead)
	grp.POST("/sos", handler.CreateSOS)

	if includeDispute {
		grp.POST("/complaints/:id/dispute", handler.EscalateToDispute)
		grp.GET("/complaints/:id/dispute", handler.GetDispute)
	}
}
