package movementhttp

import (
	"bytes"
	"encoding/json"
	"io"
	nethttp "net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	movementusecases "cosmicforge/logistics/services/customer-service/internal/features/movement_history/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// RegisterCustomerRoutes mounts the read-only customer history endpoints.
func RegisterCustomerRoutes(group *gin.RouterGroup, service *movementusecases.Service, signer *sharedauth.TokenSigner) {
	if group == nil || service == nil || signer == nil {
		return
	}
	g := group.Group("/movements", sharedauth.BearerMiddleware(signer, authusecases.CustomerRole, authusecases.CustomerService))
	g.GET("", func(c *gin.Context) {
		claims, ok := sharedauth.ClaimsFromContext(c)
		if !ok || claims.Subject == "" {
			httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
			return
		}
		page, _ := strconv.Atoi(c.Query("page"))
		limit, _ := strconv.Atoi(c.Query("limit"))
		items, meta, err := service.List(c.Request.Context(), claims.Subject, movementusecases.ListQuery{
			Authority: c.Query("authority"), ProductCode: c.Query("product_code"), MovementType: c.Query("movement_type"),
			State: c.Query("state"), ParentContextRef: c.Query("parent_context_ref"), From: c.Query("from"), To: c.Query("to"),
			Page: page, Limit: limit,
		})
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{"movements": items, "pagination": meta}})
	})
	g.GET("/:authority/:sourceRef", func(c *gin.Context) {
		claims, ok := sharedauth.ClaimsFromContext(c)
		if !ok || claims.Subject == "" {
			httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
			return
		}
		item, err := service.Get(c.Request.Context(), claims.Subject, c.Param("authority"), c.Param("sourceRef"))
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": item})
	})
}

// RegisterIngress mounts POST /internal/customer/v1/movement-events at the
// router root (the /internal/<domain>/v1 convention), behind HMAC v2.
func RegisterIngress(engine *gin.Engine, service *movementusecases.Service, verifier *serviceauth.Verifier) {
	if engine == nil || service == nil || verifier == nil {
		return
	}
	ingest := func(c *gin.Context, handler func(string, events.Envelope) (movementusecases.IngestResult, error)) {
		raw, err := io.ReadAll(nethttp.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
		if err != nil {
			httpx.RespondError(c, apperrors.BadRequest("Event body is too large or unreadable.", err))
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var envelope events.Envelope
		if err := decoder.Decode(&envelope); err != nil {
			httpx.RespondError(c, apperrors.BadRequest("Event body is invalid.", err))
			return
		}
		result, err := handler(strings.TrimSpace(c.GetString("service_name")), envelope)
		if err != nil {
			httpx.RespondError(c, apperrors.From(err))
			return
		}
		c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": result})
	}
	engine.POST(events.CustomerMovementIngressPath, serviceauth.Middleware(verifier), func(c *gin.Context) {
		ingest(c, func(caller string, envelope events.Envelope) (movementusecases.IngestResult, error) {
			return service.Ingest(c.Request.Context(), caller, envelope)
		})
	})
	// Reconciliation: an authority asks what this projection already holds
	// for its own movements, so it republishes only what is missing.
	engine.GET(events.CustomerMovementStatePath, serviceauth.Middleware(verifier), func(c *gin.Context) {
		versions, err := service.ProjectedVersions(c.Request.Context(), strings.TrimSpace(c.GetString("service_name")),
			strings.Split(c.Query("source_refs"), ","))
		if err != nil {
			httpx.RespondError(c, apperrors.From(err))
			return
		}
		c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{"versions": versions}})
	})
	// Commerce grouping has its own ingress and its own caller allowlist, so
	// a grouping publisher can never reach the movement-state path.
	engine.POST(events.CustomerMovementGroupingPath, serviceauth.Middleware(verifier), func(c *gin.Context) {
		ingest(c, func(caller string, envelope events.Envelope) (movementusecases.IngestResult, error) {
			return service.IngestGrouping(c.Request.Context(), caller, envelope)
		})
	})
}
