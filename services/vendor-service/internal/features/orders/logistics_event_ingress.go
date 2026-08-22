package orders

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	DispatchServiceCaller = "driver-dispatch-delivery-service"
	HaulingServiceCaller  = "driver-hauling-service"
)

type logisticsEventIngress struct{ service *Service }

type logisticsEventRequest struct {
	EventID       string          `json:"event_id"`
	MissionRef    string          `json:"mission_ref"`
	VendorOrderID string          `json:"vendor_order_id"`
	ParentOrderID string          `json:"parent_order_id"`
	EventType     string          `json:"event_type"`
	SourceVersion int64           `json:"source_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Payload       json.RawMessage `json:"payload"`
}

func (h *logisticsEventIngress) allowCaller(c *gin.Context) {
	caller, _ := c.Get("service_name")
	name, _ := caller.(string)
	if name != DispatchServiceCaller && name != HaulingServiceCaller {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Next()
}

func (h *logisticsEventIngress) consumeNonce(c *gin.Context) {
	caller, _ := c.Get("service_name")
	nonce := strings.TrimSpace(c.GetHeader(serviceauth.HeaderNonce))
	if nonce == "" || h.service == nil || h.service.store == nil || h.service.store.db == nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	tag, err := h.service.store.db.Exec(c, `
		INSERT INTO vendor_service_nonces(caller,nonce,expires_at)
		VALUES($1,$2,now()+interval '10 minutes') ON CONFLICT DO NOTHING`, caller, nonce)
	if err != nil || tag.RowsAffected() != 1 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

func (h *logisticsEventIngress) post(c *gin.Context) {
	var request logisticsEventRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if request.Payload == nil {
		request.Payload = json.RawMessage(`{}`)
	}
	if request.EventID == "" || request.MissionRef == "" || request.VendorOrderID == "" ||
		request.ParentOrderID == "" || request.SourceVersion < 1 || request.OccurredAt.IsZero() {
		httpx.RespondError(c, errors.New("invalid logistics event"))
		return
	}
	caller, _ := c.Get("service_name")
	name, _ := caller.(string)
	source := LogisticsDispatch
	if name == HaulingServiceCaller {
		source = LogisticsHauling
	}
	updated, parent, err := h.service.ApplyLogisticsEvent(c, LogisticsOrderEvent{
		EventID: request.EventID, MissionRef: request.MissionRef,
		VendorOrderID: request.VendorOrderID, ParentOrderID: request.ParentOrderID,
		Source: source, EventType: request.EventType, SourceVersion: request.SourceVersion,
		OccurredAt: request.OccurredAt, Payload: request.Payload,
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"vendor_order": updated, "parent_order": parent,
	}})
}

// RegisterInternalLogisticsRoutes mounts the only service boundary that can
// advance platform-delivered orders. It is root-mounted and HMAC protected.
func RegisterInternalLogisticsRoutes(router *gin.Engine, service *Service, verifier *serviceauth.Verifier) {
	if router == nil || service == nil || verifier == nil {
		return
	}
	handler := &logisticsEventIngress{service: service}
	group := router.Group("/internal/vendor/v1")
	group.Use(serviceauth.Middleware(verifier), handler.allowCaller, handler.consumeNonce)
	group.POST("/commerce/logistics-events", handler.post)
}
