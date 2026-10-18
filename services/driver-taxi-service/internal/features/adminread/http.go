// Package adminread exposes owner-scoped, minimized snapshots. It does not
// authorize provider profiles from a booking association or mutate ride state.
package adminread

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	contract "cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB      *pgxpool.Pool
	Secret  []byte
	Enabled bool
	Policy  *adminpolicy.Engine
}

func Register(root *gin.Engine, db *pgxpool.Pool, secret []byte, enabled bool, policies ...*adminpolicy.Engine) {
	h := &Handler{DB: db, Secret: append([]byte(nil), secret...), Enabled: enabled}
	if len(policies) == 1 {
		h.Policy = policies[0]
	}
	root.GET(contract.TaxiPath, h.Serve)
	root.GET(contract.TaxiPath+"/:id", h.Serve)
}
func fail(c *gin.Context, status int, code string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code}})
}
func (h *Handler) Serve(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if !h.Enabled || len(h.Secret) < 32 || h.DB == nil {
		fail(c, 503, "admin_read_disabled")
		return
	}
	if _, err := h.Policy.Rule(contract.TaxiPermission, time.Now()); err != nil {
		fail(c, 503, "blocked_policy")
		return
	}
	r := c.Request
	if r.URL.RawPath != "" || len(r.URL.RequestURI()) > 8192 || strings.ContainsAny(r.URL.Path, "%\\") || r.ContentLength != 0 || r.Body != nil && r.Body != http.NoBody {
		fail(c, 400, "invalid_request")
		return
	}
	for _, key := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderSignature, serviceauth.HeaderTimestamp, serviceauth.HeaderVersion, serviceauth.HeaderNonce} {
		if len(r.Header.Values(key)) != 1 {
			fail(c, 401, "unauthorized")
			return
		}
	}
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{contract.Caller: h.Secret}, 30*time.Second)
	if _, err := verifier.VerifyRequest(r); err != nil {
		fail(c, 401, "unauthorized")
		return
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !contract.Opaque(nonce) {
		fail(c, 401, "unauthorized")
		return
	}
	hash := sha256.Sum256([]byte(contract.Caller + ":" + nonce))
	tag, err := h.DB.Exec(ctx, `INSERT INTO taxi_admin_read_nonces(nonce_hash,expires_at) VALUES($1,now()+interval '11 minutes') ON CONFLICT DO NOTHING`, hex.EncodeToString(hash[:]))
	if err != nil {
		fail(c, 503, "replay_protection_unavailable")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(c, 409, "replay_rejected")
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		fail(c, 400, "invalid_query")
		return
	}
	q, err := contract.Parse(values, time.Now())
	if err != nil {
		fail(c, 400, "invalid_query")
		return
	}
	if q.PolicyID != h.Policy.ID() || q.PolicyVersion != h.Policy.Version() || q.PolicyChecksum != h.Policy.Checksum() {
		fail(c, 503, "blocked_policy")
		return
	}
	id := c.Param("id")
	if id != "" {
		parsed, e := uuid.Parse(id)
		if e != nil || parsed.String() != id || q.Cursor != "" {
			fail(c, 400, "invalid_reference")
			return
		}
	}
	snapshot, err := h.read(ctx, q, id)
	switch {
	case errors.Is(err, adminpolicy.ErrBlocked):
		fail(c, 503, "blocked_policy")
	case errors.Is(err, adminpolicy.ErrDenied):
		fail(c, 403, "forbidden")
	case errors.Is(err, contract.ErrQuery):
		fail(c, 400, "invalid_cursor")
	case errors.Is(err, pgx.ErrNoRows):
		fail(c, 404, "not_found")
	case err != nil:
		fail(c, 503, "owner_unavailable")
	default:
		c.JSON(200, snapshot)
	}
}

type position struct {
	Query   contract.Query `json:"query"`
	At      time.Time      `json:"at"`
	ID      string         `json:"id"`
	Expires int64          `json:"exp"`
}

func (h *Handler) cursor(p position) string {
	raw, _ := json.Marshal(p)
	data := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, h.Secret)
	mac.Write([]byte("taxi-admin-cursor-v1\n" + data))
	return data + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (h *Handler) position(q contract.Query) (position, error) {
	if q.Cursor == "" {
		return position{At: q.End, ID: "ffffffff-ffff-ffff-ffff-ffffffffffff"}, nil
	}
	parts := strings.Split(q.Cursor, ".")
	if len(parts) != 2 {
		return position{}, contract.ErrQuery
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return position{}, contract.ErrQuery
	}
	var p position
	if json.Unmarshal(data, &p) != nil || !hmac.Equal([]byte(h.cursor(p)), []byte(q.Cursor)) || p.Expires <= time.Now().Unix() {
		return p, contract.ErrQuery
	}
	expected := q
	expected.Cursor = ""
	if p.Query != expected || p.At.Before(q.Start) || p.At.After(q.End) {
		return p, contract.ErrQuery
	}
	if _, err := uuid.Parse(p.ID); err != nil {
		return p, contract.ErrQuery
	}
	return p, nil
}

func (h *Handler) read(ctx context.Context, q contract.Query, id string) (contract.TaxiSnapshot, error) {
	out := contract.TaxiSnapshot{ContractVersion: "1.0", Scope: q.Scope, Freshness: "LIVE", Coverage: "PARTIAL_COVERAGE", Items: []contract.Ride{}, Events: []contract.Event{}, PolicyID: h.Policy.ID(), PolicyVersion: h.Policy.Version(), PolicyChecksum: h.Policy.Checksum()}
	requested := adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
	actor := adminpolicy.Actor{Product: "karryam", Staff: q.Staff, Roles: []string{q.Role}, Permissions: []string{contract.TaxiPermission}, Markets: []string{q.Scope.Market}, Zones: []string{q.Scope.Zone}, Organizations: []string{q.Scope.Organization}, Tenants: []string{q.Scope.Tenant}}
	p, err := h.position(q)
	if err != nil {
		return out, err
	}
	if err := h.Policy.AuthorizeDelegation(actor, contract.TaxiPermission, q.Purpose, requested, time.Now()); err != nil {
		return out, err
	}
	// One repeatable snapshot prevents a scope or lifecycle change between
	// selecting the booking and selecting its history. All scope joins are here
	// in Taxi's database; the bridge never has database access to this owner.
	tx, err := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&out.AsOf); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT b.id::text,b.status,b.booking_mode,b.source_context,b.updated_at,b.created_at,m.code,z.code,b.organization_id
 FROM taxi_bookings b JOIN markets m ON m.id=b.market_id
 JOIN operating_zones z ON z.id=b.operating_zone_id AND z.market_id=b.market_id
 WHERE m.code=$1 AND z.code=$2 AND b.organization_id=$3 AND b.is_corporate_managed
	 AND b.source_context IN ('FLEET_ASSIGNMENT','FLEET_OFFER','FLEET_PRIVATE_EXECUTION')
 AND b.created_at >= $4 AND b.created_at < $5 AND (b.created_at,b.id)<($6,$7::uuid)
 AND ($8='' OR b.id::text=$8) ORDER BY b.created_at DESC,b.id DESC LIMIT $9`, q.Scope.Market, q.Scope.Zone, q.Scope.Organization, q.Start, q.End, p.At, p.ID, id, q.Limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item contract.Ride
		var market, zone, organization string
		if err = rows.Scan(&item.ID, &item.Status, &item.Mode, &item.SourceContext, &item.UpdatedAt, &item.CreatedAt, &market, &zone, &organization); err != nil {
			rows.Close()
			return out, err
		}
		// Fleet's canonical tenant context is OrganizationID. This binding is
		// used only for actual Fleet-origin corporate executions, never for an
		// independent ride or an organization supplied only in the request.
		facts := adminpolicy.Facts{Owner: "driver-taxi-service", Reference: item.ID, SourceContract: "taxi.corporate-rides.v1", Scope: adminpolicy.Scope{Product: "karryam", Market: market, Zone: zone, Organization: organization, Tenant: organization}}
		if err = h.Policy.AuthorizeOwner(actor, contract.TaxiPermission, q.Purpose, requested, facts, time.Now()); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if id != "" && len(out.Items) == 0 {
		return out, pgx.ErrNoRows
	}
	if len(out.Items) > q.Limit {
		out.Items = out.Items[:q.Limit]
		last := out.Items[len(out.Items)-1]
		bound := q
		bound.Cursor = ""
		out.NextCursor = h.cursor(position{Query: bound, At: last.CreatedAt, ID: last.ID, Expires: time.Now().Add(10 * time.Minute).Unix()})
	}
	if id != "" {
		events, e := tx.Query(ctx, `SELECT id::text,event_type,created_at FROM booking_events WHERE booking_id=$1 ORDER BY created_at DESC,id DESC LIMIT 201`, id)
		if e != nil {
			return out, e
		}
		for events.Next() {
			var ev contract.Event
			if e = events.Scan(&ev.ID, &ev.Type, &ev.At); e != nil {
				events.Close()
				return out, e
			}
			out.Events = append(out.Events, ev)
		}
		e = events.Err()
		events.Close()
		if e != nil {
			return out, e
		}
		if len(out.Events) > 200 {
			out.Events = out.Events[:200]
			out.HistoryTruncated = true
		}
	}
	// No raw event metadata, actor contacts, GPS, money, notes, documents or
	// handover credentials leave this owner. Audit persistence precedes release.
	_, err = tx.Exec(ctx, `INSERT INTO taxi_admin_read_audit(staff_ref,market_code,zone_code,organization_ref,purpose_code,target_ref,tenant_ref,approved_role,policy_id,policy_version,policy_checksum) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, q.Staff, q.Scope.Market, q.Scope.Zone, q.Scope.Organization, q.Purpose, id, q.Scope.Tenant, q.Role, h.Policy.ID(), h.Policy.Version(), h.Policy.Checksum())
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
