package recovery

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Handler exposes the recovery surface.
type Handler struct{ service *Service }

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterInternal mounts recovery on a group the caller has ALREADY protected
// with HMAC service auth.
//
// Recovery is an internal/operator surface, never a customer or provider one.
// A provider reporting their own breakdown does so through their existing
// cancel path, which is what opens the case; nothing here lets a caller choose
// who replaces them. Exposing replacement selection to a provider would be
// handing one provider the ability to name their successor.
func RegisterInternal(group *gin.RouterGroup, handler *Handler) {
	if group == nil || handler == nil || handler.service == nil {
		return
	}
	group.POST("/recovery/bookings/:id/start", handler.start)
	group.POST("/recovery/bookings/:id/select-replacement", handler.selectReplacement)
	group.POST("/recovery/bookings/:id/complete-transfer", handler.completeTransfer)
	group.POST("/recovery/bookings/:id/resolve", handler.resolve)
	group.GET("/recovery/bookings/:id", handler.get)
}

type startRequest struct {
	OperationRef  string `json:"operation_ref"`
	TriggerCode   string `json:"trigger_code"`
	CorrelationID string `json:"correlation_id"`
	Actor         string `json:"actor"`
}

func (h *Handler) start(c *gin.Context) {
	var request startRequest
	if err := decodeOne(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	result, err := h.service.Start(c.Request.Context(), StartCommand{
		BookingID:     c.Param("id"),
		OperationRef:  strings.TrimSpace(request.OperationRef),
		TriggerCode:   strings.ToUpper(strings.TrimSpace(request.TriggerCode)),
		CorrelationID: request.CorrelationID,
		Actor:         actorOf(c, request.Actor),
	})
	respond(c, result, err)
}

func (h *Handler) selectReplacement(c *gin.Context) {
	var request struct {
		Actor string `json:"actor"`
	}
	// A body is optional here: the command carries no parameters a caller
	// could legitimately vary. Selection is the service's decision, not the
	// caller's, which is precisely why there is no provider_id field.
	_ = decodeOptional(c, &request)
	result, err := h.service.SelectReplacement(c.Request.Context(),
		c.Param("id"), actorOf(c, request.Actor))
	respond(c, result, err)
}

type transferRequest struct {
	OperationRef         string `json:"operation_ref"`
	TransferRef          string `json:"transfer_ref"`
	EvidenceRef          string `json:"evidence_ref"`
	OldProviderConfirmed bool   `json:"old_provider_confirmed"`
	NewProviderConfirmed bool   `json:"new_provider_confirmed"`
	ExpectedVersion      int64  `json:"expected_version"`
	Actor                string `json:"actor"`
}

func (h *Handler) completeTransfer(c *gin.Context) {
	var request transferRequest
	if err := decodeOne(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	result, err := h.service.CompleteTransfer(c.Request.Context(), TransferCommand{
		BookingID:            c.Param("id"),
		OperationRef:         strings.TrimSpace(request.OperationRef),
		TransferRef:          strings.TrimSpace(request.TransferRef),
		EvidenceRef:          strings.TrimSpace(request.EvidenceRef),
		OldProviderConfirmed: request.OldProviderConfirmed,
		NewProviderConfirmed: request.NewProviderConfirmed,
		ExpectedVersion:      request.ExpectedVersion,
		Actor:                actorOf(c, request.Actor),
	})
	respond(c, result, err)
}

func (h *Handler) resolve(c *gin.Context) {
	var request struct {
		OperationRef    string `json:"operation_ref"`
		ExpectedVersion int64  `json:"expected_version"`
		Actor           string `json:"actor"`
	}
	_ = decodeOptional(c, &request)
	result, err := h.service.Resolve(c.Request.Context(), ResolveCommand{
		BookingID:       c.Param("id"),
		OperationRef:    strings.TrimSpace(request.OperationRef),
		ExpectedVersion: request.ExpectedVersion,
		Actor:           actorOf(c, request.Actor),
	})
	respond(c, result, err)
}

func (h *Handler) get(c *gin.Context) {
	result, err := h.service.Get(c.Request.Context(), c.Param("id"))
	respond(c, result, err)
}

func respond(c *gin.Context, result Public, err error) {
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// actorOf prefers the authenticated caller over a body field.
//
// The body value is a convenience for an operator console that wants to record
// a human name; it must never be able to overwrite who the HMAC boundary says
// is calling, or the audit trail becomes self-reported.
func actorOf(c *gin.Context, fallback string) string {
	if value, ok := c.Get("service_name"); ok {
		if caller, _ := value.(string); strings.TrimSpace(caller) != "" {
			if trimmed := strings.TrimSpace(fallback); trimmed != "" {
				return caller + ":" + trimmed
			}
			return caller
		}
	}
	return strings.TrimSpace(fallback)
}

func decodeOne(c *gin.Context, target any) error {
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(target); err != nil {
		return apperrors.BadRequest("Request body must be a single JSON object.", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return apperrors.BadRequest("Request body must contain exactly one JSON object.", err)
	}
	return nil
}

func decodeOptional(c *gin.Context, target any) error {
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return nil
	}
	return decodeOne(c, target)
}
