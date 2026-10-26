// Package adminfeed serves Support's Secure Admin event backfill and wires the
// owner event relay (Section 16 / Section 22).
//
// The snapshot is a stable, id-ordered page of current case state, built by
// the same SQL function that writes incremental events, so a backfilled event
// and a live one can never disagree about which fields exist.
package adminfeed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	SourceService = "support-dispute-service"
	EntityType    = "support_case"
	Prefix        = "SUPPORT_DISPUTE"
	SnapshotPath  = "/internal/support/v1/admin-events/snapshot"
	// Caller is the only workload allowed to backfill.
	Caller       = adminread.Caller
	MaxPageSize  = 200
	defaultLimit = 100
	nonceOwner   = SourceService + "#admin-events-snapshot"
)

var ErrConfiguration = errors.New("invalid Support admin event snapshot configuration")

// SnapshotHandler is default-off. Disabled, it answers 503 without reading
// any credential or row.
type SnapshotHandler struct {
	DB      *pgxpool.Pool
	Secret  []byte
	Enabled bool
	MaxSkew time.Duration
}

func (h *SnapshotHandler) Register(root *gin.Engine) { root.GET(SnapshotPath, h.Serve) }

func fail(c *gin.Context, status int, code string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code}})
}

func (h *SnapshotHandler) Serve(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if !h.Enabled || h.DB == nil || len(h.Secret) < 32 {
		fail(c, 503, "snapshot_disabled")
		return
	}
	r := c.Request
	if r.Method != http.MethodGet || r.URL.RawPath != "" || len(r.URL.RequestURI()) > 2048 || r.ContentLength > 0 || len(r.TransferEncoding) > 0 {
		fail(c, 400, "invalid_request")
		return
	}
	for _, header := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderNonce, serviceauth.HeaderTimestamp, serviceauth.HeaderSignature} {
		if len(r.Header.Values(header)) != 1 {
			fail(c, 401, "unauthorized")
			return
		}
	}
	skew := h.MaxSkew
	if skew <= 0 {
		skew = 30 * time.Second
	}
	caller, err := serviceauth.NewVerifier(serviceauth.Secrets{Caller: h.Secret}, skew).VerifyRequest(r)
	if err != nil || caller != Caller {
		fail(c, 401, "unauthorized")
		return
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !adminread.Opaque(nonce) {
		fail(c, 401, "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	sum := sha256.Sum256([]byte(nonceOwner + ":" + nonce))
	tag, err := h.DB.Exec(ctx, `INSERT INTO admin_owner_read_nonces(owner,nonce_hash,expires_at) VALUES($1,$2,now()+interval '11 minutes') ON CONFLICT DO NOTHING`, nonceOwner, hex.EncodeToString(sum[:]))
	if err != nil {
		fail(c, 503, "replay_protection_unavailable")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(c, 409, "replay_rejected")
		return
	}
	cursor, limit, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		fail(c, 400, "invalid_query")
		return
	}
	page, err := h.Page(ctx, cursor, limit)
	if err != nil {
		fail(c, 503, "snapshot_unavailable")
		return
	}
	c.JSON(http.StatusOK, page)
}

func parseQuery(raw string) (string, int, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "", 0, err
	}
	for key, v := range values {
		if (key != "cursor" && key != "limit") || len(v) != 1 {
			return "", 0, errors.New("invalid query")
		}
	}
	cursor := values.Get("cursor")
	if cursor != "" {
		id, err := uuid.Parse(cursor)
		if err != nil || id.String() != cursor {
			return "", 0, errors.New("invalid cursor")
		}
	}
	limit := defaultLimit
	if value := values.Get("limit"); value != "" || values.Has("limit") {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > MaxPageSize || strconv.Itoa(n) != value {
			return "", 0, errors.New("invalid limit")
		}
		limit = n
	}
	return cursor, limit, nil
}

// Page builds one validated snapshot page after cursor (exclusive).
func (h *SnapshotHandler) Page(ctx context.Context, cursor string, limit int) (adminevents.SnapshotPage, error) {
	var page adminevents.SnapshotPage
	if limit < 1 || limit > MaxPageSize {
		return page, ErrConfiguration
	}
	tx, err := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	var watermark time.Time
	if err := tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&watermark); err != nil {
		return page, err
	}
	var after any
	if cursor != "" {
		after = cursor
	}
	rows, err := tx.Query(ctx, `SELECT support_admin_case_event(id)::text FROM complaints WHERE $1::uuid IS NULL OR id > $1::uuid ORDER BY id LIMIT $2`, after, limit+1)
	if err != nil {
		return page, err
	}
	events := []adminevents.SourceEvent{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return page, err
		}
		ev, err := adminevents.Decode([]byte(raw))
		if err != nil {
			rows.Close()
			return page, err
		}
		events = append(events, ev)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return page, err
	}
	page = adminevents.SnapshotPage{Contract: adminevents.SnapshotContract, Source: SourceService, EntityType: EntityType, Watermark: watermark.UTC(), Events: events, Complete: true}
	if len(events) > limit {
		page.Events = events[:limit]
		page.NextCursor = page.Events[limit-1].EntityRef
		page.Complete = false
	}
	adapter, ok := adminevents.Lookup(SourceService, adminevents.SupportCaseChanged)
	if !ok {
		return adminevents.SnapshotPage{}, adminevents.ErrUnknown
	}
	for _, ev := range page.Events {
		if _, err := adapter.Minimize(ev); err != nil {
			return adminevents.SnapshotPage{}, err
		}
	}
	if err := page.Validate(SourceService); err != nil {
		return adminevents.SnapshotPage{}, err
	}
	return page, tx.Commit(ctx)
}

func readSecret(path string) ([]byte, error) {
	if path == "" {
		return nil, ErrConfiguration
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrConfiguration
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(raw) > 4096 {
		return nil, ErrConfiguration
	}
	secret, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(secret) < 32 {
		return nil, ErrConfiguration
	}
	return secret, nil
}

// otherSecretFiles are every other Secure Admin credential Support may hold.
// The snapshot key must be distinct from each one that is configured.
var otherSecretFiles = []string{
	"_ADMIN_RESOURCE_SECRET_FILE", "_ADMIN_CONTEXT_PUBLISH_SECRET_FILE", "_ADMIN_CASE_SOURCE_SECRET_FILE",
	"_ADMIN_COMMAND_WRITE_SECRET_FILE", "_ADMIN_COMMAND_READBACK_SECRET_FILE", "_ADMIN_EVENTS_SECRET_FILE",
}

// SnapshotFromEnvironment is default-off: with
// SUPPORT_DISPUTE_ADMIN_EVENTS_SNAPSHOT_ENABLED unset no file or table is read.
func SnapshotFromEnvironment(ctx context.Context, db *pgxpool.Pool) (*SnapshotHandler, error) {
	h := &SnapshotHandler{DB: db}
	if os.Getenv(Prefix+"_ADMIN_EVENTS_SNAPSHOT_ENABLED") != "true" {
		return h, nil
	}
	if db == nil {
		return nil, ErrConfiguration
	}
	secret, err := readSecret(os.Getenv(Prefix + "_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE"))
	if err != nil {
		return nil, err
	}
	for _, suffix := range otherSecretFiles {
		path := os.Getenv(Prefix + suffix)
		if path == "" {
			continue
		}
		other, err := readSecret(path)
		if err != nil {
			return nil, ErrConfiguration
		}
		if bytes.Equal(other, secret) {
			return nil, ErrConfiguration
		}
	}
	var ready bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('admin_owner_read_nonces') IS NOT NULL AND to_regclass('admin_event_outbox') IS NOT NULL AND to_regprocedure('support_admin_case_event(uuid)') IS NOT NULL`).Scan(&ready); err != nil || !ready {
		return nil, ErrConfiguration
	}
	h.Secret, h.Enabled = secret, true
	return h, nil
}
