// Package http is the Section 13 network analytics surface.
//
// Every route is service-to-service and HMAC-signed; none is reachable by an
// end user. Ingest is open to the producing services, the advisory reads to
// the services that consult them, and the dashboards to the admin backoffice
// alone.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/usecases"
	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// NonceConsumer rejects a replayed signed request.
type NonceConsumer interface {
	Consume(caller, nonce string, now time.Time) bool
}

// Options configures the handler.
type Options struct {
	Service  *usecases.Service
	Verifier *serviceauth.Verifier
	Nonces   NonceConsumer
	// IngestCallers may post events; ReadCallers may read advisory signals.
	IngestCallers []string
	ReadCallers   []string
	AdminCaller   string
}

// Handler serves network analytics.
type Handler struct {
	service     *usecases.Service
	verifier    *serviceauth.Verifier
	nonces      NonceConsumer
	ingest      map[string]bool
	read        map[string]bool
	adminCaller string
}

// NewHandler builds the handler.
func NewHandler(options Options) *Handler {
	set := func(values []string) map[string]bool {
		out := map[string]bool{}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				out[value] = true
			}
		}
		return out
	}
	admin := strings.TrimSpace(options.AdminCaller)
	if admin == "" {
		admin = "admin-backoffice-service"
	}
	return &Handler{service: options.Service, verifier: options.Verifier, nonces: options.Nonces,
		ingest: set(options.IngestCallers), read: set(options.ReadCallers), adminCaller: admin}
}

// RegisterInternalRoutes mounts ingest and the advisory reads at the root.
func RegisterInternalRoutes(router *gin.Engine, h *Handler) {
	router.POST(networkanalytics.IngestPath, h.ingestEvent)
	router.GET(networkanalytics.BranchReadinessPath+":branchId", h.branchReadiness)
	router.GET(networkanalytics.ProviderQualityPath+":authority/:providerId", h.providerQuality)
	router.GET(networkanalytics.SupplySnapshotPath, h.supplySnapshot)
	router.GET(networkanalytics.DemandForecastPath, h.demandForecast)
}

// RegisterAdminRoutes mounts the dashboards under the service's API base.
func RegisterAdminRoutes(group *gin.RouterGroup, h *Handler) {
	network := group.Group("/network")
	network.GET("/mission-success", h.admin(func(ctx context.Context, authority string, days int) (any, error) {
		return h.service.MissionSuccess(ctx, authority, days)
	}))
	network.GET("/recovery", h.admin(func(ctx context.Context, authority string, days int) (any, error) {
		return h.service.Recovery(ctx, authority, days)
	}))
	network.GET("/capability-mismatches", h.admin(func(ctx context.Context, authority string, days int) (any, error) {
		return h.service.Mismatches(ctx, authority, days)
	}))
	network.GET("/declines", h.admin(func(ctx context.Context, authority string, days int) (any, error) {
		return h.service.Declines(ctx, authority, days)
	}))
	network.GET("/demand", h.admin(func(ctx context.Context, authority string, days int) (any, error) {
		return h.service.Demand(ctx, authority, days)
	}))
	network.GET("/provider-quality/:authority/:providerId", h.adminProviderQuality)
	network.GET("/readiness/branches/:branchId", h.adminBranchReadiness)
	network.GET("/supply", h.adminSupply)
	network.GET("/mission-outcomes", h.adminMissionOutcomes)
	network.GET("/demand-forecast", h.adminDemandForecast)
}

func (h *Handler) ingestEvent(c *gin.Context) {
	caller, ok := h.authorize(c, h.ingest)
	if !ok {
		return
	}
	var raw json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		fail(c, http.StatusBadRequest, "validation_failed", "The event body is not valid JSON.")
		return
	}
	envelope, err := normalize(raw)
	if err != nil {
		fail(c, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	status, err := h.service.Ingest(c.Request.Context(), caller, envelope)
	switch {
	case errors.Is(err, usecases.ErrCallerMismatch):
		fail(c, http.StatusForbidden, "forbidden", "The event's source is not the calling service.")
		return
	case errors.Is(err, sharedevents.ErrInvalidEnvelope):
		fail(c, http.StatusBadRequest, "validation_failed", err.Error())
		return
	case err != nil:
		fail(c, http.StatusInternalServerError, "internal_error", "The event could not be stored.")
		return
	}
	// Every stored outcome -- including duplicate and stale -- is a success
	// to the producer. Retrying any of them could only produce the same
	// answer, and an outbox that treated them as failures would retry forever.
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{"status": status}})
}

// normalize accepts the platform's canonical envelope and the camelCase
// envelope Vendor and Dispatch already publish, so neither has to change its
// outbox format to be counted.
func normalize(raw json.RawMessage) (sharedevents.Envelope, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return sharedevents.Envelope{}, errors.New("the event must be a JSON object")
	}
	if _, canonical := probe["event_name"]; canonical {
		var envelope sharedevents.Envelope
		err := json.Unmarshal(raw, &envelope)
		return envelope, err
	}
	var camel struct {
		EventID          string          `json:"eventId"`
		EventType        string          `json:"eventType"`
		SchemaVersion    int             `json:"schemaVersion"`
		SourceService    string          `json:"sourceService"`
		Producer         string          `json:"producer"`
		AggregateType    string          `json:"aggregateType"`
		AggregateID      string          `json:"aggregateId"`
		AggregateVersion int64           `json:"aggregateVersion"`
		OccurredAt       time.Time       `json:"occurredAt"`
		CorrelationID    string          `json:"correlationId"`
		CausationID      string          `json:"causationId"`
		Payload          json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &camel); err != nil {
		return sharedevents.Envelope{}, err
	}
	source := camel.SourceService
	if source == "" {
		source = camel.Producer
	}
	return sharedevents.Envelope{
		EventID: camel.EventID, EventName: camel.EventType, SchemaVersion: camel.SchemaVersion,
		SourceService: source, SourceEntityType: camel.AggregateType, SourceEntityID: camel.AggregateID,
		SourceVersion: camel.AggregateVersion, OccurredAt: camel.OccurredAt,
		CorrelationID: camel.CorrelationID, CausationID: camel.CausationID, Payload: camel.Payload,
	}, nil
}

func (h *Handler) branchReadiness(c *gin.Context) {
	if _, ok := h.authorize(c, h.read); !ok {
		return
	}
	answer, err := h.service.BranchReadiness(c.Request.Context(), c.Param("branchId"), days(c))
	respond(c, answer, err)
}

// callerAuthority is the authority an execution service may read provider
// quality for: its own providers only. Vendor, Fleet and the backoffice are
// not execution services and read across authorities.
var callerAuthority = map[string]string{
	"driver-taxi-service": networkanalytics.AuthorityTaxi, "taxi-service": networkanalytics.AuthorityTaxi,
	"driver-dispatch-delivery-service": networkanalytics.AuthorityDispatch,
	"dispatch-delivery-service":        networkanalytics.AuthorityDispatch,
	"driver-hauling-service":           networkanalytics.AuthorityHauling, "hauling-service": networkanalytics.AuthorityHauling,
}

func (h *Handler) providerQuality(c *gin.Context) {
	caller, ok := h.authorize(c, h.read)
	if !ok {
		return
	}
	authority := strings.ToUpper(c.Param("authority"))
	if !networkanalytics.ValidAuthority(authority) {
		fail(c, http.StatusBadRequest, "validation_failed", "Unknown authority.")
		return
	}
	if own, execution := callerAuthority[caller]; execution && own != authority {
		fail(c, http.StatusForbidden, "forbidden", "A service may read only its own providers.")
		return
	}
	answer, err := h.service.ProviderQuality(c.Request.Context(), authority, c.Param("providerId"), days(c))
	respond(c, answer, err)
}

func (h *Handler) supplySnapshot(c *gin.Context) {
	if _, ok := h.authorize(c, h.read); !ok {
		return
	}
	h.writeSupply(c)
}

func (h *Handler) writeSupply(c *gin.Context) {
	authority := strings.ToUpper(c.Query("authority"))
	zone := strings.TrimSpace(c.Query("zone"))
	if !networkanalytics.ValidAuthority(authority) || zone == "" {
		fail(c, http.StatusBadRequest, "validation_failed", "A logistics authority and a zone are required.")
		return
	}
	answer, err := h.service.SupplySnapshotFor(c.Request.Context(), authority, zone, c.Query("product"))
	respond(c, answer, err)
}

func (h *Handler) admin(read func(ctx context.Context, authority string, days int) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.authorizeAdmin(c) {
			return
		}
		authority := strings.ToUpper(strings.TrimSpace(c.Query("authority")))
		if authority != "" && !networkanalytics.ValidAuthority(authority) {
			fail(c, http.StatusBadRequest, "validation_failed", "Unknown authority.")
			return
		}
		data, err := read(c.Request.Context(), authority, days(c))
		respond(c, data, err)
	}
}

func (h *Handler) adminProviderQuality(c *gin.Context) {
	if !h.authorizeAdmin(c) {
		return
	}
	authority := strings.ToUpper(c.Param("authority"))
	if !networkanalytics.ValidAuthority(authority) {
		fail(c, http.StatusBadRequest, "validation_failed", "Unknown authority.")
		return
	}
	answer, err := h.service.ProviderQuality(c.Request.Context(), authority, c.Param("providerId"), days(c))
	respond(c, answer, err)
}

func (h *Handler) adminBranchReadiness(c *gin.Context) {
	if !h.authorizeAdmin(c) {
		return
	}
	answer, err := h.service.BranchReadiness(c.Request.Context(), c.Param("branchId"), days(c))
	respond(c, answer, err)
}

func (h *Handler) adminSupply(c *gin.Context) {
	if !h.authorizeAdmin(c) {
		return
	}
	h.writeSupply(c)
}

func (h *Handler) authorizeAdmin(c *gin.Context) bool {
	_, ok := h.authorize(c, map[string]bool{h.adminCaller: true})
	return ok
}

// authorize verifies the signature, consumes the nonce, and checks the caller
// against the route's allowlist.
func (h *Handler) authorize(c *gin.Context, allowed map[string]bool) (string, bool) {
	if h.verifier == nil {
		fail(c, http.StatusServiceUnavailable, "service_auth_not_configured", "Service authentication is not configured.")
		return "", false
	}
	caller, err := h.verifier.VerifyRequest(c.Request)
	if err != nil {
		fail(c, http.StatusUnauthorized, "unauthorized", "Invalid service signature.")
		return "", false
	}
	if !allowed[caller] {
		fail(c, http.StatusForbidden, "forbidden", "This service may not call this route.")
		return "", false
	}
	if h.nonces != nil && !h.nonces.Consume(caller, c.GetHeader(serviceauth.HeaderNonce), time.Now().UTC()) {
		fail(c, http.StatusUnauthorized, "replay_rejected", "This signed request was already used.")
		return "", false
	}
	return caller, true
}

func days(c *gin.Context) int {
	value, err := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("days", "30")))
	if err != nil {
		return 30
	}
	return value
}

func respond(c *gin.Context, data any, err error) {
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Analytics could not answer.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"success": false, "error": gin.H{"code": code, "message": message}})
}

func (h *Handler) demandForecast(c *gin.Context) {
	if _, ok := h.authorize(c, h.read); !ok {
		return
	}
	h.writeForecast(c)
}

func (h *Handler) adminDemandForecast(c *gin.Context) {
	if !h.authorizeAdmin(c) {
		return
	}
	h.writeForecast(c)
}

func (h *Handler) writeForecast(c *gin.Context) {
	authority := strings.ToUpper(c.Query("authority"))
	zone := strings.TrimSpace(c.Query("zone"))
	if !networkanalytics.ValidAuthority(authority) || zone == "" {
		fail(c, http.StatusBadRequest, "validation_failed", "A logistics authority and a zone are required.")
		return
	}
	at, _ := time.Parse(time.RFC3339, c.Query("at"))
	answer, err := h.service.DemandForecast(c.Request.Context(), authority, zone, c.Query("product"), at)
	respond(c, answer, err)
}

func (h *Handler) adminMissionOutcomes(c *gin.Context) {
	if !h.authorizeAdmin(c) {
		return
	}
	authority := strings.ToUpper(strings.TrimSpace(c.Query("authority")))
	if authority != "" && !networkanalytics.ValidAuthority(authority) {
		fail(c, http.StatusBadRequest, "validation_failed", "Unknown authority.")
		return
	}
	days, err := h.service.MissionOutcomes(c.Request.Context(), authority, c.Query("product"), days(c))
	respond(c, days, err)
}
