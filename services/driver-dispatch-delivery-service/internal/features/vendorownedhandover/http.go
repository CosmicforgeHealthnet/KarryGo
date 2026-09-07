package vendorownedhandover

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }

func Register(group *gin.RouterGroup, h *Handler, verifier *serviceauth.Verifier, db *pgxpool.Pool) {
	if h == nil || h.service == nil || !h.service.Enabled() || verifier == nil {
		return
	}
	g := group.Group("/commerce/vendor-owned/missions")
	g.Use(authority.ServiceBoundary(verifier, authority.NewPostgresNonceClaimer(db), []string{"vendor-service"})...)
	g.POST("", h.bind)
	g.GET("/:missionRef/handover/:stage", h.read)
	g.POST("/:missionRef/handover/:stage/confirm", h.confirm)
	g.POST("/:missionRef/handover/:stage/resend", h.resend)
	g.POST("/:missionRef/reassignments", h.reassign)
	g.POST("/:missionRef/receiver-changes", h.changeReceiver)
	g.POST("/:missionRef/cancellations", h.cancel)
}

func (h *Handler) bind(c *gin.Context) {
	var b MissionBinding
	if c.ShouldBindJSON(&b) != nil {
		fail(c, ErrInvalid)
		return
	}
	b.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	b.CorrelationID = c.GetHeader("X-Correlation-ID")
	o, err := h.service.Bind(c, b)
	respond(c, http.StatusCreated, o, err)
}
func (h *Handler) read(c *gin.Context) {
	o, err := h.service.Read(c, c.Param("missionRef"), Stage(strings.ToUpper(c.Param("stage"))))
	respond(c, http.StatusOK, o, err)
}
func (h *Handler) confirm(c *gin.Context) {
	var in ConfirmCommand
	if c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	in.MissionRef = c.Param("missionRef")
	in.Stage = Stage(strings.ToUpper(c.Param("stage")))
	in.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	caller, _ := c.Get("service_name")
	o, err := h.service.Confirm(c, in, stringValue(caller), c.GetHeader("X-Correlation-ID"))
	respond(c, http.StatusOK, o, err)
}
func (h *Handler) resend(c *gin.Context) {
	var in struct {
		ProviderRef string `json:"provider_ref"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	caller, _ := c.Get("service_name")
	o, err := h.service.Resend(c, c.Param("missionRef"), Stage(strings.ToUpper(c.Param("stage"))), in.ProviderRef, c.GetHeader("Idempotency-Key"), stringValue(caller), c.GetHeader("X-Correlation-ID"))
	respond(c, http.StatusOK, o, err)
}
func (h *Handler) reassign(c *gin.Context) {
	var in ChangeCommand
	if c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	in.MissionRef = c.Param("missionRef")
	in.OperationRef = c.GetHeader("Idempotency-Key")
	o, err := h.service.Reassign(c, in, c.GetHeader("X-Correlation-ID"))
	respond(c, http.StatusOK, o, err)
}
func (h *Handler) changeReceiver(c *gin.Context) {
	var in ChangeCommand
	if c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	in.MissionRef = c.Param("missionRef")
	in.OperationRef = c.GetHeader("Idempotency-Key")
	o, err := h.service.ChangeReceiver(c, in, c.GetHeader("X-Correlation-ID"))
	respond(c, http.StatusOK, o, err)
}
func (h *Handler) cancel(c *gin.Context) {
	err := h.service.Cancel(c, c.Param("missionRef"), c.GetHeader("Idempotency-Key"), c.GetHeader("X-Correlation-ID"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "CANCELLED"}})
}
func respond(c *gin.Context, status int, o Observation, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(status, gin.H{"success": true, "data": o})
}
func fail(c *gin.Context, err error) {
	status := http.StatusConflict
	code := "HANDOVER_REJECTED"
	switch {
	case errors.Is(err, ErrInvalid):
		status = http.StatusBadRequest
		code = "HANDOVER_SCOPE_INVALID"
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
		code = "HANDOVER_NOT_FOUND"
	case errors.Is(err, ErrDisabled):
		status = http.StatusServiceUnavailable
		code = "HANDOVER_NOT_AVAILABLE"
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"code": code}})
}
func stringValue(v any) string { s, _ := v.(string); return strings.TrimSpace(s) }
