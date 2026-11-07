// Package adminsnapshot serves the Secure Admin event backfill for vendor
// verification cases: a stable, id-ordered page of current case state in the
// exact adminevents SourceEvent shape the trigger emits incrementally. The
// Bridge buffers live events, applies the snapshot, then the buffer; version
// fencing makes the overlap harmless.
package adminsnapshot

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	Source     = "verification-compliance-service"
	EntityType = "vendor_verification_case"
	Path       = "/internal/verification/v1/admin-events/snapshot"
	// Caller is the only service that may backfill.
	Caller   = "admin-backoffice-service"
	MaxLimit = 200
)

var ErrConfiguration = errors.New("invalid verification admin-event snapshot configuration")

// eventNamespace makes snapshot event ids deterministic per case state, so a
// repeated backfill of an unchanged case is the same event, not a new one.
var eventNamespace = uuid.MustParse("6d3b9f2e-4f1a-5c1e-9a57-18b2f0c6a4d1")

type Handler struct {
	DB          *pgxpool.Pool
	Verifier    *serviceauth.Verifier
	Enabled     bool
	CheckEvent  func(context.Context, pgx.Tx, adminevents.SourceEvent) error
	ExportReady func() bool
}

// FromEnvironment is default off. Enabling requires a dedicated secret file
// (base64url, >=32 bytes) distinct from every other credential.
func FromEnvironment(db *pgxpool.Pool, guards ...ScopeGuard) (*Handler, error) {
	h := &Handler{DB: db, Enabled: os.Getenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_ENABLED") == "true"}
	if !h.Enabled {
		return h, nil
	}
	path := os.Getenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE")
	raw, err := os.ReadFile(path)
	if db == nil || path == "" || err != nil || len(raw) > 4096 {
		return nil, ErrConfiguration
	}
	secret, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(secret) < 32 {
		return nil, ErrConfiguration
	}
	for _, suffix := range []string{"_ADMIN_RESOURCE_SECRET_FILE", "_ADMIN_CONTEXT_PUBLISH_SECRET_FILE", "_ADMIN_CASE_SOURCE_SECRET_FILE", "_ADMIN_COMMAND_WRITE_SECRET_FILE", "_ADMIN_COMMAND_READBACK_SECRET_FILE", "_ADMIN_EVENTS_SECRET_FILE"} {
		otherPath := os.Getenv("VERIFICATION_COMPLIANCE" + suffix)
		if otherPath == "" {
			continue
		}
		otherRaw, err := os.ReadFile(otherPath)
		if err != nil || len(otherRaw) > 4096 {
			return nil, ErrConfiguration
		}
		other, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(otherRaw)))
		if err != nil || len(other) < 32 || bytes.Equal(secret, other) {
			return nil, ErrConfiguration
		}
	}
	if len(guards) != 1 || !guards[0].Configured() {
		return nil, ErrConfiguration
	}
	h.CheckEvent = guards[0].Check
	h.ExportReady = guards[0].Configured
	h.Verifier = serviceauth.NewVerifier(serviceauth.Secrets{Caller: secret}, 5*time.Minute)
	return h, nil
}

func (h *Handler) Register(r *gin.Engine) { r.GET(Path, h.serve) }

func (h *Handler) serve(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || !h.Enabled || h.DB == nil || h.Verifier == nil || h.CheckEvent == nil || h.ExportReady == nil || !h.ExportReady() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "snapshot_disabled"})
		return
	}
	caller, err := h.Verifier.VerifyRequest(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	if caller != Caller {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	nonce := strings.TrimSpace(c.GetHeader(serviceauth.HeaderNonce))
	tag, err := h.DB.Exec(c, `INSERT INTO verification_service_nonces(caller,nonce,expires_at) VALUES($1,$2,now()+interval '10 minutes') ON CONFLICT DO NOTHING`, "admin-events-snapshot:"+caller, nonce)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "nonce_unavailable"})
		return
	}
	if tag.RowsAffected() != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "replayed"})
		return
	}
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > MaxLimit {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_limit"})
			return
		}
		limit = n
	}
	cursor := c.Query("cursor")
	if cursor != "" {
		if id, err := uuid.Parse(cursor); err != nil || id.String() != cursor {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_cursor"})
			return
		}
	}
	page, err := h.Page(c, cursor, limit)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "snapshot_unavailable"})
		return
	}
	c.JSON(http.StatusOK, page)
}

// Querier is satisfied by a pool or a transaction.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const opaquePattern = `^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$`

// BuildPage returns one validated snapshot page after cursor (a case id).
// Cases whose business identifier is not an opaque token are not exportable
// (the trigger skips them too) and are omitted rather than quarantined.
func BuildPage(ctx context.Context, q Querier, cursor string, limit int) (adminevents.SnapshotPage, error) {
	if limit < 1 || limit > MaxLimit {
		limit = MaxLimit
	}
	page := adminevents.SnapshotPage{Contract: adminevents.SnapshotContract, Source: Source, EntityType: EntityType, Events: []adminevents.SourceEvent{}}
	if err := q.QueryRow(ctx, `SELECT now()`).Scan(&page.Watermark); err != nil {
		return page, err
	}
	page.Watermark = page.Watermark.UTC()
	var cursorArg any
	if cursor != "" {
		cursorArg = cursor
	}
	rows, err := q.Query(ctx, `SELECT id::text,source_version,status,dimension,subject_type,business_id,automation_recommendation,
		CASE WHEN reason_code ~ $3 THEN reason_code END,expires_at,updated_at,verification_admin_event_scope(id::text)
		FROM vendor_verification_cases WHERE ($1::uuid IS NULL OR id>$1::uuid) AND business_id ~ $3 AND subject_type ~ $3 ORDER BY id LIMIT $2`, cursorArg, limit+1, opaquePattern)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	emitted := time.Now().UTC()
	for rows.Next() {
		var id, status, dimension, subject, business, recommendation string
		var reason *string
		var version int64
		var expires *time.Time
		var updated time.Time
		var scope adminevents.Scope
		if err := rows.Scan(&id, &version, &status, &dimension, &subject, &business, &recommendation, &reason, &expires, &updated, &scope); err != nil {
			return page, err
		}
		if len(page.Events) == limit {
			page.NextCursor = page.Events[len(page.Events)-1].EntityRef
			break
		}
		var expiresValue any
		if expires != nil {
			expiresValue = expires.UTC()
		}
		var reasonValue any
		if reason != nil {
			reasonValue = *reason
		}
		ev, err := adminevents.NewSourceEvent(Source, adminevents.VerificationCaseChanged, EntityType, id, version, updated, scope, "", map[string]any{
			"status": status, "dimension": dimension, "subject_type": subject, "business_ref": business,
			"automation_recommendation": recommendation, "reason_code": reasonValue, "expires_at": expiresValue, "updated_at": updated.UTC(),
		}, false)
		if err != nil {
			return page, err
		}
		ev.EmittedAt = emitted
		ev.EventID = uuid.NewSHA1(eventNamespace, []byte("snapshot|"+id+"|"+strconv.FormatInt(version, 10)+"|"+recommendation)).String()
		page.Events = append(page.Events, ev)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	page.Complete = page.NextCursor == ""
	return page, page.Validate(Source)
}

// Page binds the snapshot and every disclosure check to one repeatable-read
// source transaction. Missing or revoked allocations fail the complete page.
func (h *Handler) Page(ctx context.Context, cursor string, limit int) (adminevents.SnapshotPage, error) {
	var empty adminevents.SnapshotPage
	if h == nil || h.DB == nil || h.CheckEvent == nil || h.ExportReady == nil || !h.ExportReady() {
		return empty, ErrConfiguration
	}
	tx, err := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	page, err := BuildPage(ctx, tx, cursor, limit)
	if err != nil {
		return empty, err
	}
	for _, ev := range page.Events {
		if err := h.CheckEvent(ctx, tx, ev); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return page, nil
}
