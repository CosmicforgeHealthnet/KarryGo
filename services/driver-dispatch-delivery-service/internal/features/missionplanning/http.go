package missionplanning

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }
func Register(group *gin.RouterGroup, h *Handler, verifier *serviceauth.Verifier, db *pgxpool.Pool, allowed []string) {
	if h == nil || h.service == nil {
		return
	}
	g := group.Group("/dispatch/v1/delivery-plans")
	g.Use(authority.ServiceBoundary(verifier, authority.NewPostgresNonceClaimer(db), allowed)...)
	g.POST("", h.create)
	g.GET("/by-source/:sourceRef", h.readBySource)
	g.GET("/:groupRef", h.read)
	g.POST("/:groupRef/replans", h.replan)
	g.POST("/manifests/:manifestRef/custody/activate", h.activateCustody)
	g.POST("/manifests/:manifestRef/custody/release", h.releaseCustody)
}

func (h *Handler) readBySource(c *gin.Context) {
	version, err := strconv.ParseInt(strings.TrimSpace(c.Query("source_version")), 10, 64)
	if err != nil || version < 1 {
		planningFail(c, errors.New("source_version is required"))
		return
	}
	out, err := h.service.ReadBySource(c.Request.Context(), c.Param("sourceRef"), version)
	if err != nil {
		planningFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) activateCustody(c *gin.Context) { h.custody(c, true) }
func (h *Handler) releaseCustody(c *gin.Context)  { h.custody(c, false) }
func (h *Handler) custody(c *gin.Context, activate bool) {
	var in CustodyCommand
	if c.ShouldBindJSON(&in) != nil {
		planningFail(c, errors.New("invalid request"))
		return
	}
	in.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	var err error
	if activate {
		err = h.service.ActivateCustody(c.Request.Context(), c.Param("manifestRef"), in)
	} else {
		err = h.service.ReleaseCustody(c.Request.Context(), c.Param("manifestRef"), in)
	}
	if err != nil {
		planningFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) create(c *gin.Context) {
	var in PlanRequest
	if c.ShouldBindJSON(&in) != nil {
		planningFail(c, errors.New("invalid request"))
		return
	}
	if k := strings.TrimSpace(c.GetHeader("Idempotency-Key")); k == "" || k != in.OperationRef {
		planningFail(c, errors.New("idempotency key must match operation_ref"))
		return
	}
	out, err := h.service.Create(c.Request.Context(), in)
	if err != nil {
		planningFail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": out})
}
func (h *Handler) replan(c *gin.Context) {
	var in PlanRequest
	if c.ShouldBindJSON(&in) != nil {
		planningFail(c, errors.New("invalid request"))
		return
	}
	if k := strings.TrimSpace(c.GetHeader("Idempotency-Key")); k == "" || k != in.OperationRef {
		planningFail(c, errors.New("idempotency key must match operation_ref"))
		return
	}
	out, err := h.service.Replan(c.Request.Context(), c.Param("groupRef"), in)
	if err != nil {
		planningFail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": out})
}
func (h *Handler) read(c *gin.Context) {
	out, err := h.service.Read(c.Request.Context(), c.Param("groupRef"))
	if err != nil {
		planningFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func planningFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Abort(c, apperrors.NotFound("Delivery plan not found.", nil))
	case errors.Is(err, ErrConflict):
		httpx.Abort(c, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "Operation key was used for a different plan.", nil))
	case errors.Is(err, ErrReplanBlocked):
		httpx.Abort(c, apperrors.New(409, "REPLAN_BLOCKED_BY_EXECUTION", "Mission execution has advanced beyond the safe replanning boundary.", nil))
	default:
		httpx.Abort(c, apperrors.BadRequest("Delivery plan request is invalid.", err))
	}
}
