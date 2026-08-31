// Package fleetauthority is the private, authoritative Fleet boundary for
// truck hauling. It stores organization relationships and assignments here;
// Fleet receives projections and never writes Hauling tables directly.
package fleetauthority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"
)

type Flags struct{ Enabled, KillSwitch bool }

type Handler struct {
	db    *pgxpool.Pool
	flags Flags
}

func New(db *pgxpool.Pool, flags Flags) *Handler { return &Handler{db: db, flags: flags} }

// Register mounts only authenticated and allowlisted routes. Nonces are
// persisted after signature verification, closing the replay window left by
// timestamp authentication alone.
func Register(root *gin.RouterGroup, h *Handler, verifier *serviceauth.Verifier, allowed []string) {
	g := root.Group("/internal/marketplace")
	g.Use(Boundary(h, verifier, allowed)...)
	g.POST("/reservations", h.reserve)
	g.GET("/reservations/:requestId", h.readReservation)
	g.POST("/reservations/:requestId/release", h.release)
	g.GET("/eligibility", h.eligibility)
	g.POST("/executions", h.execute)
	g.GET("/executions/by-idempotency/:key", h.readExecution)
	g.GET("/requests/:requestId", h.readRequest)
	g.GET("/trips/:tripId", h.readTrip)
	g.POST("/trips/:tripId/cancel", h.cancel)
	g.POST("/trips/:tripId/transitions", h.transition)
	g.POST("/providers/resolve", h.resolveProvider)
	g.POST("/providers/sponsored", h.provisionProvider)
	g.GET("/providers/:providerId", h.provider)
	g.GET("/vehicles/:vehicleId", h.vehicle)
	g.POST("/provider-links", h.createProviderLink)
	g.GET("/provider-links/by-provider/:providerId", h.readProviderLinkByProvider)
	g.GET("/provider-links/:linkId", h.readProviderLink)
	g.POST("/provider-links/:linkId/transitions", h.transitionProviderLink)
	g.POST("/truck-links", h.createTruckLink)
	g.GET("/truck-links/:linkId", h.readTruckLink)
	g.POST("/truck-links/:linkId/transitions", h.transitionTruckLink)
}

// Boundary returns Hauling's common HMAC-v2, caller allowlist and durable
// nonce replay gates for other private authority route groups.
func Boundary(h *Handler, verifier *serviceauth.Verifier, allowed []string) []gin.HandlerFunc {
	return BoundaryWithDB(h.db, verifier, allowed)
}

// BoundaryWithDB is the same three gates for a private surface that has no
// fleetauthority handler of its own — the internal commerce quote endpoint
// being the case that needs it.
//
// It exists so replay protection has exactly one implementation in this
// service. A second surface hand-rolling its own nonce insert is how one of
// them ends up subtly weaker than the other.
func BoundaryWithDB(db *pgxpool.Pool, verifier *serviceauth.Verifier, allowed []string) []gin.HandlerFunc {
	h := &Handler{db: db}
	return []gin.HandlerFunc{serviceauth.Middleware(verifier), h.allow(allowed), h.rejectReplay()}
}

func (h *Handler) allow(names []string) gin.HandlerFunc {
	set := map[string]bool{}
	for _, n := range names {
		set[strings.TrimSpace(n)] = true
	}
	return func(c *gin.Context) {
		raw, ok := c.Get("service_name")
		name, valid := raw.(string)
		if !ok || !valid || !set[name] {
			c.AbortWithStatusJSON(403, gin.H{"error": gin.H{"code": "CALLER_FORBIDDEN"}})
			return
		}
		c.Next()
	}
}

func (h *Handler) rejectReplay() gin.HandlerFunc {
	return func(c *gin.Context) {
		nonce := strings.TrimSpace(c.GetHeader("X-Service-Nonce"))
		caller, _ := c.Get("service_name")
		if nonce == "" || len(nonce) > 128 {
			c.AbortWithStatusJSON(401, gin.H{"error": gin.H{"code": "NONCE_REQUIRED"}})
			return
		}
		result, err := h.db.Exec(c, `INSERT INTO hauling_service_nonces(caller,nonce,expires_at) VALUES($1,$2,now()+interval '10 minutes') ON CONFLICT DO NOTHING`, caller, nonce)
		if err != nil || result.RowsAffected() != 1 {
			c.AbortWithStatusJSON(401, gin.H{"error": gin.H{"code": "REPLAY_REJECTED"}})
			return
		}
		c.Next()
	}
}

func (h *Handler) writable(c *gin.Context) bool {
	if !h.flags.Enabled || h.flags.KillSwitch {
		c.JSON(503, gin.H{"error": gin.H{"code": "HAULING_INTEGRATION_DISABLED"}})
		return false
	}
	return true
}
func validIDs(ids ...string) bool {
	for _, id := range ids {
		if _, e := uuid.Parse(id); e != nil {
			return false
		}
	}
	return true
}
func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func data(c *gin.Context, status int, v any) { c.JSON(status, gin.H{"success": true, "data": v}) }
func dbError(c *gin.Context, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
	} else {
		c.JSON(500, gin.H{"error": gin.H{"code": "AUTHORITY_UNAVAILABLE"}})
	}
}

type reserveBody struct {
	SourceRequestID          string `json:"source_request_id"`
	ExpectedSourceVersion    int64  `json:"expected_source_version"`
	OrganizationID           string `json:"organization_id"`
	AcceptanceSource         string `json:"acceptance_source"`
	AcceptingProviderID      string `json:"accepting_provider_id"`
	OrganizationMembershipID string `json:"organization_membership_id"`
	IdempotencyKey           string `json:"idempotency_key"`
}
type reservationView struct {
	ReservationID        string `json:"reservation_id"`
	Status               string `json:"status"`
	WinnerOrganizationID string `json:"winner_organization_id"`
	SourceVersion        int64  `json:"source_version"`
}

func (h *Handler) reserve(c *gin.Context) {
	if !h.writable(c) {
		return
	}
	var b reserveBody
	if c.ShouldBindJSON(&b) != nil || !validIDs(b.SourceRequestID, b.OrganizationID) || b.IdempotencyKey == "" {
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
		return
	}
	if prior, ok := h.reservationByKey(c, b.IdempotencyKey, fingerprint(b)); ok {
		data(c, 200, prior)
		return
	}
	tx, err := h.db.Begin(c)
	if err != nil {
		dbError(c, err)
		return
	}
	defer tx.Rollback(c)
	var version int64
	var status string
	err = tx.QueryRow(c, `SELECT source_version,status FROM haulage_bookings WHERE id=$1 FOR UPDATE`, b.SourceRequestID).Scan(&version, &status)
	if err != nil {
		dbError(c, err)
		return
	}
	if version != b.ExpectedSourceVersion {
		c.JSON(409, gin.H{"error": gin.H{"code": "VERSION_MISMATCH"}})
		return
	}
	var v reservationView
	err = tx.QueryRow(c, `INSERT INTO hauling_organization_reservations(booking_id,organization_id,acceptance_source,accepting_provider_id,membership_id,status,idempotency_key,request_fingerprint,expires_at)
	VALUES($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,'RESERVED',$6,$7,now()+interval '2 minutes') RETURNING id::text,status,organization_id::text`, b.SourceRequestID, b.OrganizationID, b.AcceptanceSource, b.AcceptingProviderID, b.OrganizationMembershipID, b.IdempotencyKey, fingerprint(b)).Scan(&v.ReservationID, &v.Status, &v.WinnerOrganizationID)
	if err != nil {
		c.JSON(409, gin.H{"error": gin.H{"code": "ALREADY_CLAIMED"}})
		return
	}
	err = tx.QueryRow(c, `UPDATE haulage_bookings SET winner_organization_id=$2,source_version=source_version+1,updated_at=now() WHERE id=$1 RETURNING source_version`, b.SourceRequestID, b.OrganizationID).Scan(&v.SourceVersion)
	if err == nil {
		err = h.event(c, tx, "REQUEST", b.SourceRequestID, "platform.request.reserved.v1", v.SourceVersion, b, c.GetHeader("X-Correlation-ID"))
	}
	if err != nil {
		dbError(c, err)
		return
	}
	if err = tx.Commit(c); err != nil {
		dbError(c, err)
		return
	}
	data(c, 201, v)
}

func (h *Handler) reservationByKey(ctx context.Context, key, fp string) (reservationView, bool) {
	var v reservationView
	var got string
	err := h.db.QueryRow(ctx, `SELECT id::text,status,organization_id::text,(SELECT source_version FROM haulage_bookings WHERE id=booking_id),request_fingerprint FROM hauling_organization_reservations WHERE idempotency_key=$1`, key).Scan(&v.ReservationID, &v.Status, &v.WinnerOrganizationID, &v.SourceVersion, &got)
	if err != nil {
		return v, false
	}
	return v, got == fp
}
func (h *Handler) readReservation(c *gin.Context) {
	var v reservationView
	err := h.db.QueryRow(c, `SELECT r.id::text,r.status,r.organization_id::text,b.source_version FROM hauling_organization_reservations r JOIN haulage_bookings b ON b.id=r.booking_id WHERE r.booking_id=$1 ORDER BY r.created_at DESC LIMIT 1`, c.Param("requestId")).Scan(&v.ReservationID, &v.Status, &v.WinnerOrganizationID, &v.SourceVersion)
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, v)
}
func (h *Handler) release(c *gin.Context) {
	if !h.writable(c) {
		return
	}
	tag, err := h.db.Exec(c, `UPDATE hauling_organization_reservations SET status='RELEASED',version=version+1,updated_at=now() WHERE booking_id=$1 AND status='RESERVED'`, c.Param("requestId"))
	if err != nil {
		dbError(c, err)
		return
	}
	if tag.RowsAffected() != 1 {
		c.JSON(409, gin.H{"error": gin.H{"code": "NOT_RESERVED"}})
		return
	}
	_, _ = h.db.Exec(c, `UPDATE haulage_bookings SET winner_organization_id=NULL,source_version=source_version+1,updated_at=now() WHERE id=$1`, c.Param("requestId"))
	data(c, 200, gin.H{"status": "RELEASED"})
}

type eligibilityView struct {
	Eligible       bool   `json:"eligible"`
	PlatformStatus string `json:"platform_status"`
	ReasonCode     string `json:"reason_code"`
	VehicleOK      bool   `json:"vehicle_ok"`
	SourceVersion  int64  `json:"source_version"`
}

func (h *Handler) eligibility(c *gin.Context) {
	p, v, r := c.Query("provider_id"), c.Query("vehicle_reference"), c.Query("source_request_id")
	if !validIDs(p, v, r) {
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
		return
	}
	var out eligibilityView
	var ps, os, ts, av string
	var shift bool
	err := h.db.QueryRow(c, `SELECT p.status,p.onboarding_status,t.status,COALESCE(a.state,'UNAVAILABLE'),COALESCE(a.shift_open,false),b.source_version FROM truck_providers p JOIN trucks t ON t.provider_id=p.id JOIN haulage_bookings b ON b.id=$3 LEFT JOIN hauling_provider_availability a ON a.provider_id=p.id WHERE p.id=$1 AND t.id=$2`, p, v, r).Scan(&ps, &os, &ts, &av, &shift, &out.SourceVersion)
	if err != nil {
		dbError(c, err)
		return
	}
	out.PlatformStatus = strings.ToUpper(ps)
	out.VehicleOK = ts == "active"
	out.Eligible = ps == "active" && os == "complete" && out.VehicleOK && av == "AVAILABLE" && shift
	if !out.Eligible {
		out.ReasonCode = "AUTHORITATIVE_ELIGIBILITY_FAILED"
	}
	data(c, 200, out)
}

type executeBody struct {
	RequestID                string `json:"requestId"`
	ExpectedRequestVersion   int64  `json:"expectedRequestVersion"`
	ReservationID            string `json:"reservationId"`
	OrganizationID           string `json:"organizationId"`
	SettlementOrganizationID string `json:"settlementOrganizationId"`
	ExecutingProviderID      string `json:"executingProviderId"`
	OrganizationMembershipID string `json:"organizationMembershipId"`
	VehicleID                string `json:"vehicleId"`
	FleetAssignmentID        string `json:"fleetAssignmentId"`
	ProviderResponseID       string `json:"providerResponseId"`
	AcceptanceSource         string `json:"acceptanceSource"`
}
type executionView struct {
	Status        string `json:"status"`
	TripID        string `json:"tripId"`
	RequestID     string `json:"requestId"`
	SourceVersion int64  `json:"requestVersion"`
	RejectionCode string `json:"rejectionCode,omitempty"`
}

func (h *Handler) execute(c *gin.Context) {
	if !h.writable(c) {
		return
	}
	var b executeBody
	if c.ShouldBindJSON(&b) != nil || !validIDs(b.RequestID, b.ReservationID, b.OrganizationID, b.SettlementOrganizationID, b.ExecutingProviderID, b.OrganizationMembershipID, b.VehicleID, b.FleetAssignmentID, b.ProviderResponseID) || c.GetHeader("Idempotency-Key") == "" {
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
		return
	}
	key := c.GetHeader("Idempotency-Key")
	fp := fingerprint(b)
	var prior executionView
	var oldfp string
	err := h.db.QueryRow(c, `SELECT response_body,request_fingerprint FROM hauling_command_operations WHERE operation_type='CREATE_EXECUTION' AND idempotency_key=$1`, key).Scan(&prior, &oldfp)
	if err == nil {
		if oldfp != fp {
			c.JSON(409, gin.H{"error": gin.H{"code": "IDEMPOTENCY_CONFLICT"}})
			return
		}
		data(c, 200, prior)
		return
	}
	tx, err := h.db.Begin(c)
	if err != nil {
		dbError(c, err)
		return
	}
	defer tx.Rollback(c)
	var version int64
	var rstatus string
	err = tx.QueryRow(c, `SELECT b.source_version,r.status FROM haulage_bookings b JOIN hauling_organization_reservations r ON r.booking_id=b.id WHERE b.id=$1 AND r.id=$2 AND r.organization_id=$3 FOR UPDATE`, b.RequestID, b.ReservationID, b.OrganizationID).Scan(&version, &rstatus)
	if err != nil {
		dbError(c, err)
		return
	}
	if version != b.ExpectedRequestVersion || rstatus != "RESERVED" {
		c.JSON(409, gin.H{"error": gin.H{"code": "VERSION_OR_RESERVATION_MISMATCH"}})
		return
	}
	var eligible bool
	err = tx.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM hauling_organization_provider_links p JOIN hauling_organization_truck_links t ON t.organization_id=p.organization_id JOIN trucks v ON v.id=t.truck_id AND v.provider_id=p.provider_id JOIN truck_providers x ON x.id=p.provider_id LEFT JOIN hauling_provider_availability a ON a.provider_id=p.provider_id WHERE p.organization_id=$1 AND p.provider_id=$2 AND p.membership_id=$3 AND t.truck_id=$4 AND p.state='ACTIVE' AND t.state='ACTIVE' AND x.status='active' AND v.status='active' AND a.state='AVAILABLE' AND a.shift_open)`, b.OrganizationID, b.ExecutingProviderID, b.OrganizationMembershipID, b.VehicleID).Scan(&eligible)
	if err != nil || !eligible {
		c.JSON(422, gin.H{"error": gin.H{"code": "PROVIDER_OR_TRUCK_INELIGIBLE"}})
		return
	}
	prior = executionView{Status: "CREATED", TripID: b.RequestID, RequestID: b.RequestID}
	err = tx.QueryRow(c, `UPDATE haulage_bookings SET provider_id=$2,truck_id=$3,settlement_organization_id=$4,organization_membership_id=$5,fleet_assignment_id=$6,provider_response_id=$7,acceptance_source=$8,status='accepted',accepted_at=now(),source_version=source_version+1,updated_at=now() WHERE id=$1 RETURNING source_version`, b.RequestID, b.ExecutingProviderID, b.VehicleID, b.SettlementOrganizationID, b.OrganizationMembershipID, b.FleetAssignmentID, b.ProviderResponseID, b.AcceptanceSource).Scan(&prior.SourceVersion)
	if err != nil {
		dbError(c, err)
		return
	}
	_, err = tx.Exec(c, `UPDATE hauling_organization_reservations SET status='CONSUMED',version=version+1,updated_at=now() WHERE id=$1`, b.ReservationID)
	if err == nil {
		_, err = tx.Exec(c, `INSERT INTO hauling_command_operations(operation_type,idempotency_key,request_fingerprint,resource_id,response_body) VALUES('CREATE_EXECUTION',$1,$2,$3,$4)`, key, fp, b.RequestID, prior)
	}
	if err == nil {
		err = h.event(c, tx, "TRIP", b.RequestID, "trip.created.v1", prior.SourceVersion, prior, c.GetHeader("X-Correlation-ID"))
	}
	if err != nil {
		dbError(c, err)
		return
	}
	if err = tx.Commit(c); err != nil {
		dbError(c, err)
		return
	}
	data(c, 201, prior)
}

func (h *Handler) readExecution(c *gin.Context) {
	var raw []byte
	err := h.db.QueryRow(c, `SELECT response_body FROM hauling_command_operations WHERE operation_type='CREATE_EXECUTION' AND idempotency_key=$1`, c.Param("key")).Scan(&raw)
	if err != nil {
		dbError(c, err)
		return
	}
	var v executionView
	if json.Unmarshal(raw, &v) != nil {
		c.JSON(500, gin.H{"error": gin.H{"code": "INVALID_STORED_OUTCOME"}})
		return
	}
	data(c, 200, v)
}
func (h *Handler) readRequest(c *gin.Context) {
	var id, status, winner, provider string
	var version int64
	err := h.db.QueryRow(c, `SELECT id::text,status,COALESCE(winner_organization_id::text,''),COALESCE(provider_id::text,''),source_version FROM haulage_bookings WHERE id=$1`, c.Param("requestId")).Scan(&id, &status, &winner, &provider, &version)
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, gin.H{"request_id": id, "booking_id": id, "status": strings.ToUpper(status), "source_version": version, "winner_organization_id": winner, "accepted_provider_id": provider})
}
func (h *Handler) readTrip(c *gin.Context) {
	var id, status, provider string
	var version int64
	var completed, cancelled *time.Time
	err := h.db.QueryRow(c, `SELECT id::text,status,COALESCE(provider_id::text,''),source_version,completed_at,cancelled_at FROM haulage_bookings WHERE id=$1`, c.Param("tripId")).Scan(&id, &status, &provider, &version, &completed, &cancelled)
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, gin.H{"trip_id": id, "request_id": id, "booking_id": id, "status": strings.ToUpper(status), "source_version": version, "executing_provider_id": provider, "completed_at": completed, "cancelled_at": cancelled})
}
func (h *Handler) cancel(c *gin.Context) {
	if !h.writable(c) {
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		var b map[string]any
		_ = c.ShouldBindJSON(&b)
		key, _ = b["idempotency_key"].(string)
	}
	if key == "" {
		c.JSON(400, gin.H{"error": gin.H{"code": "IDEMPOTENCY_KEY_REQUIRED"}})
		return
	}
	var version int64
	err := h.db.QueryRow(c, `UPDATE haulage_bookings SET status='cancelled',cancelled_by='fleet-partner-service',cancel_reason='FLEET_CANCELLED',cancelled_at=now(),source_version=source_version+1,updated_at=now() WHERE id=$1 AND status NOT IN ('completed','cancelled','failed') RETURNING source_version`, c.Param("tripId")).Scan(&version)
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, gin.H{"status": "CANCELLED", "source_version": version})
}

func (h *Handler) provider(c *gin.Context) {
	var id, status, onboarding, service string
	var version int64
	var available, shift bool
	err := h.db.QueryRow(c, `SELECT p.id::text,p.status,p.onboarding_status,COALESCE(p.service_type,'hauling'),EXTRACT(EPOCH FROM p.updated_at)::bigint,COALESCE(a.state='AVAILABLE',false),COALESCE(a.shift_open,false) FROM truck_providers p LEFT JOIN hauling_provider_availability a ON a.provider_id=p.id WHERE p.id=$1`, c.Param("providerId")).Scan(&id, &status, &onboarding, &service, &version, &available, &shift)
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, gin.H{"providerId": id, "accountState": strings.ToUpper(status), "verificationState": strings.ToUpper(onboarding), "serviceType": strings.ToUpper(service), "available": available, "availabilityState": map[bool]string{true: "AVAILABLE", false: "UNAVAILABLE"}[available], "shiftOpen": shift, "restricted": status != "active", "restrictionState": strings.ToUpper(status), "requiredDocuments": []any{}, "documentsSatisfied": onboarding == "complete", "activeAssignmentConflict": false, "sourceVersion": version, "observedAt": time.Now().UTC()})
}

type resolveProviderBody struct {
	EmailNormalized string `json:"emailNormalized"`
	PhoneE164       string `json:"phoneE164"`
}

func (h *Handler) resolveProvider(c *gin.Context) {
	var body resolveProviderBody
	if c.ShouldBindJSON(&body) != nil || strings.TrimSpace(body.EmailNormalized) == "" || strings.TrimSpace(body.PhoneE164) == "" {
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
		return
	}
	type match struct {
		id, status, organization string
		version                  int64
	}
	find := func(column, value string) (match, error) {
		var out match
		err := h.db.QueryRow(c, `SELECT id::text,status,COALESCE(managed_by_organization_id::text,''),source_version FROM truck_providers WHERE `+column+`=$1`, value).Scan(&out.id, &out.status, &out.organization, &out.version)
		return out, err
	}
	email, emailErr := find("lower(email)", strings.ToLower(strings.TrimSpace(body.EmailNormalized)))
	phone, phoneErr := find("phone", strings.TrimSpace(body.PhoneE164))
	if emailErr == nil && phoneErr == nil && email.id != phone.id {
		data(c, 200, gin.H{"outcome": "CONFLICT", "serviceCategory": "HAULING"})
		return
	}
	matched := email
	if emailErr != nil {
		matched = phone
	}
	if emailErr != nil && phoneErr != nil {
		data(c, 200, gin.H{"outcome": "NOT_FOUND", "serviceCategory": "HAULING"})
		return
	}
	data(c, 200, gin.H{"outcome": "MATCHED", "providerId": matched.id, "platformStatus": strings.ToUpper(matched.status), "managedByOrgId": matched.organization, "serviceCategory": "HAULING", "sourceVersion": matched.version})
}

type provisionProviderBody struct {
	OrganizationID  string `json:"organizationId"`
	LegalName       string `json:"legalName"`
	EmailNormalized string `json:"emailNormalized"`
	PhoneE164       string `json:"phoneE164"`
	ServiceCategory string `json:"serviceCategory"`
	OnboardingMode  string `json:"onboardingMode"`
	ProviderMode    string `json:"providerMode"`
}

func (h *Handler) provisionProvider(c *gin.Context) {
	if !h.writable(c) {
		return
	}
	key, correlation, ok := requiredWriteHeaders(c)
	if !ok {
		return
	}
	var body provisionProviderBody
	if c.ShouldBindJSON(&body) != nil || !validIDs(body.OrganizationID) || strings.TrimSpace(body.EmailNormalized) == "" || strings.TrimSpace(body.PhoneE164) == "" || !strings.EqualFold(body.ServiceCategory, "HAULING") {
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
		return
	}
	fp := fingerprint(body)
	var raw []byte
	var priorFP string
	err := h.db.QueryRow(c, `SELECT response_body,request_fingerprint FROM hauling_command_operations WHERE operation_type='PROVISION_PROVIDER' AND idempotency_key=$1`, key).Scan(&raw, &priorFP)
	if err == nil {
		if priorFP != fp {
			c.JSON(409, gin.H{"error": gin.H{"code": "IDEMPOTENCY_CONFLICT"}})
			return
		}
		var prior map[string]any
		if json.Unmarshal(raw, &prior) != nil {
			c.JSON(500, gin.H{"error": gin.H{"code": "INVALID_STORED_OUTCOME"}})
			return
		}
		data(c, 200, prior)
		return
	}
	name := strings.Fields(strings.TrimSpace(body.LegalName))
	first, last := "", ""
	if len(name) > 0 {
		first = name[0]
	}
	if len(name) > 1 {
		last = strings.Join(name[1:], " ")
	}
	tx, err := h.db.Begin(c)
	if err != nil {
		dbError(c, err)
		return
	}
	defer tx.Rollback(c)
	var id string
	var version int64
	err = tx.QueryRow(c, `INSERT INTO truck_providers(phone,email,first_name,last_name,status,onboarding_status,service_type,operation_mode,managed_by_organization_id) VALUES($1,$2,$3,$4,'pending','pending_verification','hauling','organization_managed',$5) RETURNING id::text,source_version`, body.PhoneE164, strings.ToLower(body.EmailNormalized), first, last, body.OrganizationID).Scan(&id, &version)
	if err != nil {
		c.JSON(409, gin.H{"error": gin.H{"code": "PROVIDER_IDENTITY_CONFLICT"}})
		return
	}
	out := gin.H{"providerId": id, "platformStatus": "PENDING", "sourceVersion": version}
	_, err = tx.Exec(c, `INSERT INTO hauling_command_operations(operation_type,idempotency_key,request_fingerprint,resource_id,response_body) VALUES('PROVISION_PROVIDER',$1,$2,$3,$4)`, key, fp, id, out)
	if err == nil {
		err = h.event(c, tx, "PROVIDER", id, "hauling.provider.sponsored.v1", version, out, correlation)
	}
	if err != nil {
		dbError(c, err)
		return
	}
	if err = tx.Commit(c); err != nil {
		dbError(c, err)
		return
	}
	data(c, 201, out)
}
func (h *Handler) vehicle(c *gin.Context) {
	var id, pid, status, service string
	var version int64
	var available bool
	err := h.db.QueryRow(c, `SELECT t.id::text,t.provider_id::text,t.status,COALESCE(p.service_type,'hauling'),EXTRACT(EPOCH FROM t.updated_at)::bigint,COALESCE(a.state='AVAILABLE',false) FROM trucks t JOIN truck_providers p ON p.id=t.provider_id LEFT JOIN hauling_provider_availability a ON a.provider_id=t.provider_id WHERE t.id=$1`, c.Param("vehicleId")).Scan(&id, &pid, &status, &service, &version, &available)
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, gin.H{"vehicleId": id, "providerId": pid, "controlState": "PROVIDER_CONTROLLED", "serviceType": strings.ToUpper(service), "verificationState": map[bool]string{true: "VERIFIED", false: "SUSPENDED"}[status == "active"], "available": available, "availabilityState": map[bool]string{true: "AVAILABLE", false: "UNAVAILABLE"}[available], "restricted": status != "active", "restrictionState": strings.ToUpper(status), "requiredDocuments": []any{}, "documentsSatisfied": status == "active", "activeAssignmentConflict": false, "sourceVersion": version, "observedAt": time.Now().UTC()})
}

type linkBody struct {
	OrganizationID string `json:"organizationId"`
	BranchID       string `json:"branchId"`
	MembershipID   string `json:"membershipId"`
	ProviderID     string `json:"providerId"`
	TruckID        string `json:"truckId"`
}

type linkTransitionBody struct {
	State           string `json:"state"`
	ExpectedVersion int64  `json:"expectedVersion"`
	OrganizationID  string `json:"organizationId"`
	BranchID        string `json:"branchId"`
	ReasonCode      string `json:"reasonCode"`
}

type linkView struct {
	LinkID         string `json:"linkId"`
	OrganizationID string `json:"organizationId"`
	BranchID       string `json:"branchId,omitempty"`
	MembershipID   string `json:"membershipId,omitempty"`
	ProviderID     string `json:"providerId,omitempty"`
	TruckID        string `json:"truckId,omitempty"`
	Status         string `json:"status"`
	SourceVersion  int64  `json:"sourceVersion"`
}

func requiredWriteHeaders(c *gin.Context) (string, string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	correlation := strings.TrimSpace(c.GetHeader("X-Correlation-ID"))
	causation := strings.TrimSpace(c.GetHeader("X-Causation-ID"))
	if key == "" || correlation == "" || causation == "" {
		c.JSON(400, gin.H{"error": gin.H{"code": "WRITE_HEADERS_REQUIRED"}})
		return "", "", false
	}
	return key, correlation, true
}

func (h *Handler) createProviderLink(c *gin.Context) {
	if !h.writable(c) {
		return
	}
	key, correlation, ok := requiredWriteHeaders(c)
	if !ok {
		return
	}
	var body linkBody
	if c.ShouldBindJSON(&body) != nil || strings.TrimSpace(body.OrganizationID) == "" || strings.TrimSpace(body.MembershipID) == "" || !validIDs(body.ProviderID) {
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
		return
	}
	if body.OrganizationID != strings.TrimSpace(c.GetHeader("X-Organization-ID")) {
		c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
		return
	}
	fp := fingerprint(body)
	var prior linkView
	var priorFP string
	err := h.db.QueryRow(c, `SELECT link_id::text,organization_id::text,COALESCE(branch_id::text,''),membership_id::text,provider_id::text,state,version,request_fingerprint FROM hauling_organization_provider_links WHERE idempotency_key=$1`, key).Scan(&prior.LinkID, &prior.OrganizationID, &prior.BranchID, &prior.MembershipID, &prior.ProviderID, &prior.Status, &prior.SourceVersion, &priorFP)
	if err == nil {
		if priorFP != fp {
			c.JSON(409, gin.H{"error": gin.H{"code": "IDEMPOTENCY_CONFLICT"}})
			return
		}
		data(c, 200, prior)
		return
	}
	tx, err := h.db.Begin(c)
	if err != nil {
		dbError(c, err)
		return
	}
	defer tx.Rollback(c)
	var eligible bool
	err = tx.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM truck_providers WHERE id=$1 AND status='active' AND onboarding_status='complete' AND COALESCE(service_type,'hauling')='hauling')`, body.ProviderID).Scan(&eligible)
	if err != nil || !eligible {
		c.JSON(422, gin.H{"error": gin.H{"code": "PROVIDER_INELIGIBLE"}})
		return
	}
	err = tx.QueryRow(c, `INSERT INTO hauling_organization_provider_links(organization_id,provider_id,membership_id,state,branch_id,idempotency_key,request_fingerprint) VALUES($1,$2,$3,'REQUESTED',NULLIF($4,''),$5,$6) RETURNING link_id::text,organization_id,COALESCE(branch_id,''),membership_id,provider_id::text,state,version`, body.OrganizationID, body.ProviderID, body.MembershipID, body.BranchID, key, fp).Scan(&prior.LinkID, &prior.OrganizationID, &prior.BranchID, &prior.MembershipID, &prior.ProviderID, &prior.Status, &prior.SourceVersion)
	if err != nil {
		c.JSON(409, gin.H{"error": gin.H{"code": "LINK_CONFLICT"}})
		return
	}
	if err = h.recordLinkChange(c, tx, "PROVIDER", prior, "", correlation, c.GetHeader("X-Causation-ID"), ""); err == nil {
		err = h.event(c, tx, "PROVIDER_LINK", prior.LinkID, "hauling.provider_link.requested.v1", prior.SourceVersion, prior, correlation)
	}
	if err == nil {
		err = tx.Commit(c)
	}
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 201, prior)
}

func (h *Handler) readProviderLink(c *gin.Context) {
	var out linkView
	err := h.db.QueryRow(c, `SELECT link_id::text,organization_id::text,COALESCE(branch_id::text,''),membership_id::text,provider_id::text,state,version FROM hauling_organization_provider_links WHERE link_id=$1 AND organization_id=$2`, c.Param("linkId"), c.GetHeader("X-Organization-ID")).Scan(&out.LinkID, &out.OrganizationID, &out.BranchID, &out.MembershipID, &out.ProviderID, &out.Status, &out.SourceVersion)
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, out)
}

func (h *Handler) readProviderLinkByProvider(c *gin.Context) {
	var out linkView
	err := h.db.QueryRow(c, `SELECT link_id::text,organization_id::text,COALESCE(branch_id::text,''),membership_id::text,provider_id::text,state,version
		FROM hauling_organization_provider_links
		WHERE provider_id=$1 AND organization_id=$2 AND state IN ('REQUESTED','ACTIVE','SUSPENDED')
		ORDER BY created_at DESC LIMIT 1`, c.Param("providerId"), c.GetHeader("X-Organization-ID")).Scan(&out.LinkID, &out.OrganizationID, &out.BranchID, &out.MembershipID, &out.ProviderID, &out.Status, &out.SourceVersion)
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, out)
}

func (h *Handler) createTruckLink(c *gin.Context) {
	if !h.writable(c) {
		return
	}
	key, correlation, ok := requiredWriteHeaders(c)
	if !ok {
		return
	}
	var body linkBody
	if c.ShouldBindJSON(&body) != nil || strings.TrimSpace(body.OrganizationID) == "" || !validIDs(body.TruckID) {
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
		return
	}
	if body.OrganizationID != strings.TrimSpace(c.GetHeader("X-Organization-ID")) {
		c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
		return
	}
	fp := fingerprint(body)
	var prior linkView
	var priorFP string
	err := h.db.QueryRow(c, `SELECT link_id::text,organization_id::text,COALESCE(branch_id::text,''),COALESCE(membership_id::text,''),truck_id::text,state,version,request_fingerprint FROM hauling_organization_truck_links WHERE idempotency_key=$1`, key).Scan(&prior.LinkID, &prior.OrganizationID, &prior.BranchID, &prior.MembershipID, &prior.TruckID, &prior.Status, &prior.SourceVersion, &priorFP)
	if err == nil {
		if priorFP != fp {
			c.JSON(409, gin.H{"error": gin.H{"code": "IDEMPOTENCY_CONFLICT"}})
			return
		}
		data(c, 200, prior)
		return
	}
	tx, err := h.db.Begin(c)
	if err != nil {
		dbError(c, err)
		return
	}
	defer tx.Rollback(c)
	var eligible bool
	err = tx.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM trucks t JOIN truck_providers p ON p.id=t.provider_id WHERE t.id=$1 AND t.status='active' AND p.status='active' AND p.onboarding_status='complete' AND COALESCE(p.service_type,'hauling')='hauling')`, body.TruckID).Scan(&eligible)
	if err != nil || !eligible {
		c.JSON(422, gin.H{"error": gin.H{"code": "TRUCK_INELIGIBLE"}})
		return
	}
	err = tx.QueryRow(c, `INSERT INTO hauling_organization_truck_links(organization_id,truck_id,state,branch_id,membership_id,idempotency_key,request_fingerprint) VALUES($1,$2,'REQUESTED',NULLIF($3,''),NULLIF($4,''),$5,$6) RETURNING link_id::text,organization_id,COALESCE(branch_id,''),COALESCE(membership_id,''),truck_id::text,state,version`, body.OrganizationID, body.TruckID, body.BranchID, body.MembershipID, key, fp).Scan(&prior.LinkID, &prior.OrganizationID, &prior.BranchID, &prior.MembershipID, &prior.TruckID, &prior.Status, &prior.SourceVersion)
	if err != nil {
		c.JSON(409, gin.H{"error": gin.H{"code": "LINK_CONFLICT"}})
		return
	}
	if err = h.recordLinkChange(c, tx, "TRUCK", prior, "", correlation, c.GetHeader("X-Causation-ID"), ""); err == nil {
		err = h.event(c, tx, "TRUCK_LINK", prior.LinkID, "hauling.truck_link.requested.v1", prior.SourceVersion, prior, correlation)
	}
	if err == nil {
		err = tx.Commit(c)
	}
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 201, prior)
}

func (h *Handler) readTruckLink(c *gin.Context) {
	var out linkView
	err := h.db.QueryRow(c, `SELECT link_id::text,organization_id::text,COALESCE(branch_id::text,''),COALESCE(membership_id::text,''),truck_id::text,state,version FROM hauling_organization_truck_links WHERE link_id=$1 AND organization_id=$2`, c.Param("linkId"), c.GetHeader("X-Organization-ID")).Scan(&out.LinkID, &out.OrganizationID, &out.BranchID, &out.MembershipID, &out.TruckID, &out.Status, &out.SourceVersion)
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, out)
}

var linkTransitions = map[string]map[string]bool{
	"REQUESTED": {"ACTIVE": true, "REJECTED": true, "REVOKED": true},
	"ACTIVE":    {"SUSPENDED": true, "REVOKED": true},
	"SUSPENDED": {"ACTIVE": true, "REVOKED": true},
}

func (h *Handler) transitionProviderLink(c *gin.Context) { h.transitionLink(c, "PROVIDER") }
func (h *Handler) transitionTruckLink(c *gin.Context)    { h.transitionLink(c, "TRUCK") }

func (h *Handler) transitionLink(c *gin.Context, kind string) {
	if !h.writable(c) {
		return
	}
	key, correlation, ok := requiredWriteHeaders(c)
	if !ok {
		return
	}
	var body linkTransitionBody
	if c.ShouldBindJSON(&body) != nil || body.ExpectedVersion < 1 {
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_TRANSITION"}})
		return
	}
	callerOrganization := strings.TrimSpace(c.GetHeader("X-Organization-ID"))
	if callerOrganization == "" {
		c.JSON(400, gin.H{"error": gin.H{"code": "ORGANIZATION_REQUIRED"}})
		return
	}
	body.State = strings.ToUpper(strings.TrimSpace(body.State))
	operationType := kind + "_LINK_TRANSITION"
	fp := fingerprint(map[string]any{"kind": kind, "linkId": c.Param("linkId"), "request": body})
	var replay linkView
	var replayBody []byte
	var replayFP string
	err := h.db.QueryRow(c, `SELECT response_body,request_fingerprint FROM hauling_command_operations WHERE operation_type=$1 AND idempotency_key=$2`, operationType, key).Scan(&replayBody, &replayFP)
	if err == nil {
		if replayFP != fp || json.Unmarshal(replayBody, &replay) != nil {
			c.JSON(409, gin.H{"error": gin.H{"code": "IDEMPOTENCY_CONFLICT"}})
			return
		}
		data(c, 200, replay)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		dbError(c, err)
		return
	}
	table, idColumn, eventPrefix := "hauling_organization_provider_links", "provider_id", "hauling.provider_link"
	if kind == "TRUCK" {
		table, idColumn, eventPrefix = "hauling_organization_truck_links", "truck_id", "hauling.truck_link"
	}
	tx, err := h.db.Begin(c)
	if err != nil {
		dbError(c, err)
		return
	}
	defer tx.Rollback(c)
	var out linkView
	query := `SELECT link_id::text,organization_id::text,COALESCE(branch_id::text,''),COALESCE(membership_id::text,''),` + idColumn + `::text,state,version FROM ` + table + ` WHERE link_id=$1 AND organization_id=$2 FOR UPDATE`
	var resourceID *string
	if kind == "PROVIDER" {
		resourceID = &out.ProviderID
	} else {
		resourceID = &out.TruckID
	}
	err = tx.QueryRow(c, query, c.Param("linkId"), callerOrganization).Scan(&out.LinkID, &out.OrganizationID, &out.BranchID, &out.MembershipID, resourceID, &out.Status, &out.SourceVersion)
	if err != nil {
		dbError(c, err)
		return
	}
	// A concurrent identical request may have been invisible before we waited
	// for the link row lock. Recheck the command record before evaluating the
	// now-advanced source version.
	err = tx.QueryRow(c, `SELECT response_body,request_fingerprint FROM hauling_command_operations WHERE operation_type=$1 AND idempotency_key=$2`, operationType, key).Scan(&replayBody, &replayFP)
	if err == nil {
		if replayFP != fp || json.Unmarshal(replayBody, &replay) != nil {
			c.JSON(409, gin.H{"error": gin.H{"code": "IDEMPOTENCY_CONFLICT"}})
			return
		}
		data(c, 200, replay)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		dbError(c, err)
		return
	}
	if out.SourceVersion != body.ExpectedVersion || !linkTransitions[out.Status][body.State] {
		c.JSON(409, gin.H{"error": gin.H{"code": "INVALID_STATE_OR_VERSION"}})
		return
	}
	prior := out.Status
	newOrg, newBranch := out.OrganizationID, out.BranchID
	if body.OrganizationID != "" {
		newOrg = body.OrganizationID
	}
	if body.BranchID != "" {
		newBranch = body.BranchID
	}
	update := `UPDATE ` + table + ` SET state=$2,organization_id=$3,branch_id=NULLIF($4,''),version=version+1,updated_at=now() WHERE link_id=$1 AND version=$5 RETURNING organization_id,COALESCE(branch_id,''),state,version`
	err = tx.QueryRow(c, update, out.LinkID, body.State, newOrg, newBranch, body.ExpectedVersion).Scan(&out.OrganizationID, &out.BranchID, &out.Status, &out.SourceVersion)
	if err == nil {
		err = h.recordLinkChange(c, tx, kind, out, prior, correlation, c.GetHeader("X-Causation-ID"), body.ReasonCode)
	}
	if err == nil {
		err = h.event(c, tx, kind+"_LINK", out.LinkID, eventPrefix+"."+strings.ToLower(out.Status)+".v1", out.SourceVersion, map[string]any{"link": out, "idempotencyKey": key, "reasonCode": body.ReasonCode}, correlation)
	}
	if err == nil {
		responseBody, marshalErr := json.Marshal(out)
		if marshalErr != nil {
			err = marshalErr
		} else {
			_, err = tx.Exec(c, `INSERT INTO hauling_command_operations(operation_type,idempotency_key,request_fingerprint,resource_id,response_body) VALUES($1,$2,$3,$4,$5)`, operationType, key, fp, out.LinkID, responseBody)
		}
	}
	if err == nil {
		err = tx.Commit(c)
	}
	if err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, out)
}

func (h *Handler) recordLinkChange(ctx context.Context, tx pgx.Tx, kind string, view linkView, prior, correlation, causation, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO hauling_organization_link_history(link_type,link_id,organization_id,branch_id,prior_state,new_state,source_version,actor_service,reason_code,correlation_id,causation_id) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11)`, kind, view.LinkID, view.OrganizationID, view.BranchID, prior, view.Status, view.SourceVersion, "fleet-partner-service", reason, correlation, causation)
	return err
}

type transitionBody struct {
	State               string `json:"state"`
	ExpectedVersion     int64  `json:"expectedVersion"`
	ProofReference      string `json:"proofReference"`
	CheckpointReference string `json:"checkpointReference"`
	ReasonCode          string `json:"reasonCode"`
}

var haulingTransitions = map[string]map[string]bool{
	"accepted":            {"en_route_pickup": true, "cancelled": true, "failed": true},
	"en_route_pickup":     {"arrived_at_pickup": true, "cancelled": true, "failed": true},
	"arrived_at_pickup":   {"loading": true, "cancelled": true, "failed": true},
	"loading":             {"loaded": true, "failed": true},
	"loaded":              {"en_route_delivery": true, "failed": true},
	"en_route_delivery":   {"arrived_at_delivery": true, "failed": true},
	"arrived_at_delivery": {"unloading": true, "failed": true},
	"unloading":           {"delivered": true, "failed": true},
	"delivered":           {"completed": true, "failed": true},
}

func (h *Handler) transition(c *gin.Context) {
	if !h.writable(c) {
		return
	}
	var b transitionBody
	if c.ShouldBindJSON(&b) != nil || b.ExpectedVersion < 1 || strings.TrimSpace(c.GetHeader("Idempotency-Key")) == "" {
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_TRANSITION"}})
		return
	}
	b.State = strings.ToLower(strings.TrimSpace(b.State))
	tx, err := h.db.Begin(c)
	if err != nil {
		dbError(c, err)
		return
	}
	defer tx.Rollback(c)
	var current string
	var version int64
	err = tx.QueryRow(c, `SELECT status,source_version FROM haulage_bookings WHERE id=$1 FOR UPDATE`, c.Param("tripId")).Scan(&current, &version)
	if err != nil {
		dbError(c, err)
		return
	}
	if version != b.ExpectedVersion || !haulingTransitions[current][b.State] {
		c.JSON(409, gin.H{"error": gin.H{"code": "INVALID_STATE_OR_VERSION"}})
		return
	}
	if b.State == "delivered" && strings.TrimSpace(b.ProofReference) == "" {
		c.JSON(422, gin.H{"error": gin.H{"code": "PROOF_REQUIRED"}})
		return
	}
	err = tx.QueryRow(c, `UPDATE haulage_bookings SET status=$2,source_version=source_version+1,updated_at=now(),
loading_started_at=CASE WHEN $2='loading' THEN now() ELSE loading_started_at END,
loading_completed_at=CASE WHEN $2='loaded' THEN now() ELSE loading_completed_at END,
unloading_started_at=CASE WHEN $2='unloading' THEN now() ELSE unloading_started_at END,
unloading_completed_at=CASE WHEN $2='delivered' THEN now() ELSE unloading_completed_at END,
delivered_at=CASE WHEN $2='delivered' THEN now() ELSE delivered_at END,
completed_at=CASE WHEN $2='completed' THEN now() ELSE completed_at END,
		failed_at=CASE WHEN $2='failed' THEN now() ELSE failed_at END,
		failure_reason=CASE WHEN $2='failed' THEN $3 ELSE failure_reason END WHERE id=$1 RETURNING source_version`, c.Param("tripId"), b.State, b.ReasonCode).Scan(&version)
	if err == nil {
		eventType := "trip.status_changed.v1"
		if b.State == "delivered" {
			eventType = "proof.completed.v1"
		} else if b.State == "completed" {
			eventType = "trip.completed.v1"
		}
		err = h.event(c, tx, "TRIP", c.Param("tripId"), eventType, version, map[string]any{"tripId": c.Param("tripId"), "status": strings.ToUpper(b.State), "proofReference": b.ProofReference, "checkpointReference": b.CheckpointReference}, c.GetHeader("X-Correlation-ID"))
	}
	if err != nil {
		dbError(c, err)
		return
	}
	if err = tx.Commit(c); err != nil {
		dbError(c, err)
		return
	}
	data(c, 200, gin.H{"tripId": c.Param("tripId"), "status": strings.ToUpper(b.State), "sourceVersion": version})
}

func (h *Handler) event(ctx context.Context, tx pgx.Tx, aggregate, id, event string, version int64, payload any, correlation string) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if aggregate == "TRIP" {
		var refs map[string]any
		if err = json.Unmarshal(b, &refs); err != nil {
			return err
		}
		var operation, order, shipment, assignment, organization, settlement, provider string
		lookupErr := tx.QueryRow(ctx, `SELECT fleet_operation_id,fleet_private_order_id,
			fleet_shipment_id,fleet_assignment_id,organization_id,
			settlement_organization_id,executing_provider_id::text
			FROM hauling_private_execution_mappings WHERE canonical_booking_id=$1`, id).
			Scan(&operation, &order, &shipment, &assignment, &organization, &settlement, &provider)
		if lookupErr == nil {
			refs["fleetExecutionOperationId"] = operation
			refs["privateOrderId"] = order
			refs["shipmentId"] = shipment
			refs["privateAssignmentId"] = assignment
			refs["organizationId"] = organization
			refs["settlementOrganizationId"] = settlement
			refs["executingProviderId"] = provider
			b, err = json.Marshal(refs)
			if err != nil {
				return err
			}
		} else if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return lookupErr
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO hauling_domain_outbox(aggregate_type,aggregate_id,event_type,event_version,payload,correlation_id) VALUES($1,$2,$3,$4,$5,$6)`, aggregate, id, event, version, b, correlation)
	return err
}
