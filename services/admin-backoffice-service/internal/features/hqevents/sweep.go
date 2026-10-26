package hqevents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/jackc/pgx/v5"
)

// SnapshotSource reads one owner's paginated, watermarked snapshot.
type SnapshotSource interface {
	Source() string
	EntityType() string
	Page(ctx context.Context, cursor string) (adminevents.SnapshotPage, error)
}

// SnapshotClient is a fixed, typed HMAC client; origin and path are compiled
// configuration, never caller input.
type SnapshotClient struct {
	source, entity, endpoint string
	secret                   []byte
	http                     *http.Client
}

// SnapshotPaths is the closed owner -> snapshot route dictionary.
var SnapshotPaths = map[string][2]string{
	"support-dispute-service":         {"/internal/support/v1/admin-events/snapshot", "support_case"},
	"verification-compliance-service": {"/internal/verification/v1/admin-events/snapshot", "vendor_verification_case"},
	"notification-service":            {"/internal/notification/v1/admin-events/snapshot", "campaign_revision"},
}

func NewSnapshotClient(source, origin string, secret []byte) (*SnapshotClient, error) {
	route, ok := SnapshotPaths[source]
	u, err := url.Parse(origin)
	if !ok || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" && u.Path != "/" || u.RawQuery != "" || len(secret) < 32 {
		return nil, errors.New("invalid owner snapshot client configuration")
	}
	return &SnapshotClient{source: source, entity: route[1], endpoint: strings.TrimRight(origin, "/") + route[0], secret: secret, http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *SnapshotClient) Source() string     { return c.source }
func (c *SnapshotClient) EntityType() string { return c.entity }
func (c *SnapshotClient) Page(ctx context.Context, cursor string) (adminevents.SnapshotPage, error) {
	var page adminevents.SnapshotPage
	q := url.Values{"limit": {"200"}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return page, err
	}
	if err = serviceauth.SignRequest(req, "admin-backoffice-service", c.secret, nil, time.Now().UTC()); err != nil {
		return page, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return page, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil || res.StatusCode != 200 {
		return page, errors.New("owner snapshot unavailable")
	}
	// Owners answer either the raw page or the platform success envelope
	// {"success":true,"data":page}; the page itself is always decoded strictly.
	var envelope struct {
		Success *bool           `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Success != nil {
		if !*envelope.Success || len(envelope.Data) == 0 {
			return page, errors.New("owner snapshot contract invalid")
		}
		raw = envelope.Data
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&page); err != nil {
		return page, errors.New("owner snapshot contract invalid")
	}
	if err = page.Validate(c.source); err != nil || page.EntityType != c.entity {
		return page, errors.New("owner snapshot contract invalid")
	}
	return page, nil
}

type SweepResult struct {
	RunID                                   string
	Status                                  string
	Pages, Events, Applied, Stale, Repaired int
	MissingAtSource                         int
}

// Sweep pages through an owner snapshot. Backfill applies everything with
// version fencing; reconciliation additionally counts drift and entities the
// Bridge holds that the owner no longer reports (recorded, never deleted).
// A page or contract failure yields PARTIAL_COVERAGE, never false completeness.
func (s *Store) Sweep(ctx context.Context, src SnapshotSource, delivery string, maxPages int) (SweepResult, error) {
	res := SweepResult{RunID: newID()}
	if delivery != DeliveryBackfill && delivery != DeliveryReconciliation {
		return res, ErrInvalid
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO admin_backfill_runs(id,source_service,entity_type,status,lease_owner,lease_expires_at) VALUES($1,$2,$3,'RUNNING',$4,now()+interval '10 minutes')`, res.RunID, src.Source(), src.EntityType(), delivery); err != nil {
		return res, err // one RUNNING sweep per source/entity (unique index)
	}
	seen := map[string]bool{}
	cursor := ""
	status := "PARTIAL_COVERAGE"
	errorCode := ""
	var watermark time.Time
	for res.Pages < maxPages {
		page, err := src.Page(ctx, cursor)
		if err != nil {
			errorCode = "OWNER_SNAPSHOT_UNAVAILABLE"
			break
		}
		res.Pages++
		if watermark.IsZero() {
			watermark = page.Watermark
		}
		for _, ev := range page.Events {
			raw, _ := json.Marshal(ev)
			seen[ev.EntityRef] = true
			outcome, err := s.Ingest(ctx, src.Source(), raw, delivery)
			if err != nil && !errors.Is(err, ErrQuarantined) {
				errorCode = "INGEST_FAILED"
				break
			}
			res.Events++
			switch outcome {
			case Applied, GapApplied:
				res.Applied++
				if delivery == DeliveryReconciliation {
					res.Repaired++
				}
			case Stale, Duplicate:
				res.Stale++
			}
		}
		if errorCode != "" {
			break
		}
		cursor = page.NextCursor
		if _, err := s.DB.Exec(ctx, `UPDATE admin_backfill_runs SET cursor=$2,pages=$3,events=$4,watermark=$5,lease_expires_at=now()+interval '10 minutes' WHERE id=$1`, res.RunID, cursor, res.Pages, res.Events, watermark); err != nil {
			return res, err
		}
		if page.Complete {
			status = "COMPLETE"
			break
		}
	}
	if status == "COMPLETE" && delivery == DeliveryReconciliation {
		rows, err := s.DB.Query(ctx, `SELECT entity_ref FROM admin_projections WHERE source_service=$1 AND entity_type=$2 AND NOT tombstone`, src.Source(), src.EntityType())
		if err != nil {
			return res, err
		}
		for rows.Next() {
			var ref string
			if rows.Scan(&ref) == nil && !seen[ref] {
				res.MissingAtSource++
			}
		}
		rows.Close()
		if _, err := s.DB.Exec(ctx, `INSERT INTO admin_reconciliation_runs(id,source_service,entity_type,checked,drift,repaired,missing_at_source) VALUES($1,$2,$3,$4,$5,$6,$7)`, res.RunID, src.Source(), src.EntityType(), res.Events, res.Repaired+res.MissingAtSource, res.Repaired, res.MissingAtSource); err != nil {
			return res, err
		}
	}
	if errorCode != "" && res.Pages == 0 {
		status = "FAILED"
	}
	res.Status = status
	var wm any
	if !watermark.IsZero() {
		wm = watermark
	}
	_, err := s.DB.Exec(ctx, `UPDATE admin_backfill_runs SET status=$2,last_error_code=$3,completed_at=now(),lease_owner=NULL,lease_expires_at=NULL,watermark=COALESCE($4,watermark) WHERE id=$1`, res.RunID, status, errorCode, wm)
	return res, err
}

// RecoverExpiredSweeps releases sweeps whose worker died mid-run.
func (s *Store) RecoverExpiredSweeps(ctx context.Context) (int64, error) {
	tag, err := s.DB.Exec(ctx, `UPDATE admin_backfill_runs SET status='PARTIAL_COVERAGE',last_error_code='LEASE_EXPIRED',completed_at=now(),lease_owner=NULL,lease_expires_at=NULL WHERE status='RUNNING' AND lease_expires_at<now()`)
	return tag.RowsAffected(), err
}

// IngressHandler is the private owner-event listener (never mounted on the HQ
// mTLS router or any public gateway). Each owner has its own HMAC secret.
type IngressHandler struct {
	Store    *Store
	Verifier *serviceauth.Verifier
	Enabled  bool
	Metrics  *Metrics
}

func (h *IngressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	reply := func(status int, code string) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"result":"` + code + `"}`))
	}
	if r.Method == http.MethodGet && r.URL.Path == "/ready" {
		reply(200, "ready")
		return
	}
	if !h.Enabled || h.Store == nil || h.Verifier == nil {
		reply(503, "ingress_disabled")
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != adminevents.IngressPath || r.URL.RawQuery != "" || r.URL.RawPath != "" {
		reply(404, "not_found")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, adminevents.MaxEventBytes))
	if err != nil {
		reply(413, "too_large")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	caller, err := h.Verifier.VerifyRequest(r)
	if err != nil {
		h.Metrics.inc(&h.Metrics.IngressBadSignature)
		reply(401, "unauthorized")
		return
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !adminevents.Opaque(nonce) {
		reply(401, "unauthorized")
		return
	}
	if err = h.Store.ClaimNonce(r.Context(), caller, nonce, time.Now().Add(11*time.Minute)); err != nil {
		if errors.Is(err, ErrReplay) {
			h.Metrics.inc(&h.Metrics.IngressReplay)
			reply(409, "replay_rejected")
			return
		}
		reply(503, "unavailable")
		return
	}
	outcome, err := h.Store.Ingest(r.Context(), caller, body, DeliveryIngress)
	switch {
	case errors.Is(err, ErrMismatch):
		h.Metrics.inc(&h.Metrics.IngressQuarantined)
		reply(403, "source_mismatch")
	case errors.Is(err, ErrQuarantined):
		h.Metrics.inc(&h.Metrics.IngressQuarantined)
		reply(422, "quarantined")
	case err != nil:
		reply(503, "unavailable")
	default:
		h.Metrics.inc(&h.Metrics.IngressAccepted)
		reply(202, strings.ToLower(string(outcome)))
	}
}

// SourcesSecrets parses the dedicated ingress secrets file: {"service":"b64"}.
func SourceSecrets(raw []byte) (serviceauth.Secrets, error) {
	var m map[string]string
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if len(raw) > 16<<10 || dec.Decode(&m) != nil || len(m) == 0 {
		return nil, errors.New("invalid event source secrets")
	}
	out := serviceauth.Secrets{}
	for name, encoded := range m {
		if _, ok := SnapshotPaths[name]; !ok {
			return nil, errors.New("unknown event source " + strconv.Quote(name))
		}
		secret, err := decodeSecret(encoded)
		if err != nil {
			return nil, err
		}
		out[name] = secret
	}
	return out, nil
}

var _ = pgx.ErrNoRows

// RunSweeps backfills every configured source once, then reconciles on the
// interval. Failures are recorded as PARTIAL_COVERAGE runs, never hidden.
func RunSweeps(ctx context.Context, s *Store, sources []SnapshotSource, interval time.Duration) {
	run := func(delivery string) {
		_, _ = s.RecoverExpiredSweeps(ctx)
		for _, src := range sources {
			sweepCtx, cancel := context.WithTimeout(ctx, 9*time.Minute)
			_, _ = s.Sweep(sweepCtx, src, delivery, 500)
			cancel()
		}
	}
	run(DeliveryBackfill)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run(DeliveryReconciliation)
		}
	}
}
