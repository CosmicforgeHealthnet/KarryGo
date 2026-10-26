// Package hqevents is the Bridge side of the Section 22 event pipeline:
// signed owner ingress -> deduplicated inbox -> version-fenced minimized
// projection -> append-only HQ outbox -> acknowledged cursor, with backfill,
// reconciliation, gap detection and quarantine. It never becomes domain truth.
package hqevents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"cosmicforge/logistics/shared/go/adminevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Outcome string

const (
	Applied     Outcome = "APPLIED"
	GapApplied  Outcome = "GAP_APPLIED"
	Stale       Outcome = "STALE"
	Duplicate   Outcome = "DUPLICATE"
	Quarantined Outcome = "QUARANTINED"
)

const (
	DeliveryIngress        = "INGRESS"
	DeliveryBackfill       = "BACKFILL"
	DeliveryReconciliation = "RECONCILIATION"
)

var (
	ErrQuarantined = errors.New("event quarantined")
	ErrMismatch    = errors.New("event source does not match authenticated caller")
	ErrInvalid     = errors.New("invalid request")
)

type Store struct {
	DB  *pgxpool.Pool
	Now func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func sum(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func (s *Store) quarantine(ctx context.Context, caller string, raw []byte, reason string) error {
	var meta struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
	}
	_ = json.Unmarshal(raw, &meta)
	if !adminevents.Opaque(meta.EventID) {
		meta.EventID = "unparseable"
	}
	if !adminevents.Opaque(meta.EventType) {
		meta.EventType = "unparseable"
	}
	_, err := s.DB.Exec(ctx, `INSERT INTO admin_event_quarantine(source_service,event_id,event_type,reason,raw_sha256) VALUES($1,$2,$3,$4,$5) ON CONFLICT (source_service,raw_sha256) DO NOTHING`, caller, meta.EventID, meta.EventType, reason, sum(raw))
	if err != nil {
		return err
	}
	return ErrQuarantined
}

// Ingest applies one owner event. caller is the HMAC-authenticated service.
func (s *Store) Ingest(ctx context.Context, caller string, raw []byte, delivery string) (Outcome, error) {
	ev, err := adminevents.Decode(raw)
	if err != nil {
		if len(raw) > 0 && len(raw) <= adminevents.MaxEventBytes {
			var probe struct {
				Product string `json:"product"`
			}
			if json.Unmarshal(raw, &probe) == nil && probe.Product != "" && probe.Product != adminevents.Product {
				return Quarantined, s.quarantine(ctx, caller, raw, "PRODUCT_MISMATCH")
			}
		}
		return Quarantined, s.quarantine(ctx, caller, raw, "INVALID_PAYLOAD")
	}
	if ev.SourceService != caller {
		_ = s.quarantine(ctx, caller, raw, "SOURCE_MISMATCH")
		return Quarantined, ErrMismatch
	}
	adapter, ok := adminevents.Lookup(ev.SourceService, ev.EventType)
	if !ok {
		return Quarantined, s.quarantine(ctx, caller, raw, "UNKNOWN_SCHEMA")
	}
	payload, err := adapter.Minimize(ev)
	if err != nil {
		reason := "INVALID_PAYLOAD"
		if errors.Is(err, adminevents.ErrUnknown) {
			reason = "UNKNOWN_SCHEMA"
		}
		return Quarantined, s.quarantine(ctx, caller, raw, reason)
	}
	if ev.Deleted {
		payload = map[string]any{}
	}
	now := s.now()
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	// Serialize all writers for this entity (ingress, backfill, reconciliation).
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, ev.SourceService+"|"+ev.EntityType+"|"+ev.EntityRef); err != nil {
		return "", err
	}
	var seen bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_event_inbox WHERE source_service=$1 AND event_id=$2)`, ev.SourceService, ev.EventID).Scan(&seen); err != nil {
		return "", err
	}
	if seen {
		return Duplicate, nil
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT entity_version FROM admin_projections WHERE product='karryam' AND source_service=$1 AND entity_type=$2 AND entity_ref=$3`, ev.SourceService, ev.EntityType, ev.EntityRef).Scan(&current)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	outcome := Applied
	switch {
	case exists && ev.EntityVersion <= current:
		outcome = Stale
	case exists && ev.EntityVersion > current+1 && delivery == DeliveryIngress:
		outcome = GapApplied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO admin_event_inbox(source_service,event_id,event_type,entity_type,entity_ref,entity_version,payload_sha256,delivery,outcome,received_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ev.SourceService, ev.EventID, ev.EventType, ev.EntityType, ev.EntityRef, ev.EntityVersion, sum(raw), delivery, string(outcome), now); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return Duplicate, nil
		}
		return "", err
	}
	if outcome == Stale {
		return outcome, tx.Commit(ctx)
	}
	if outcome == GapApplied {
		if _, err = tx.Exec(ctx, `INSERT INTO admin_projection_gaps(source_service,entity_type,entity_ref,previous_version,received_version) VALUES($1,$2,$3,$4,$5)`, ev.SourceService, ev.EntityType, ev.EntityRef, current, ev.EntityVersion); err != nil {
			return "", err
		}
	}
	coverage := "INCREMENTAL"
	if delivery == DeliveryBackfill {
		coverage = "BACKFILLED"
	} else if delivery == DeliveryReconciliation {
		coverage = "RECONCILED"
	}
	body, _ := json.Marshal(payload)
	if _, err = tx.Exec(ctx, `INSERT INTO admin_projections(product,source_service,entity_type,entity_ref,entity_version,export_type,market,zone,organization_ref,tenant_ref,classification,payload,tombstone,occurred_at,received_at,coverage)
VALUES('karryam',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT (product,source_service,entity_type,entity_ref) DO UPDATE SET entity_version=EXCLUDED.entity_version,export_type=EXCLUDED.export_type,market=EXCLUDED.market,zone=EXCLUDED.zone,organization_ref=EXCLUDED.organization_ref,tenant_ref=EXCLUDED.tenant_ref,classification=EXCLUDED.classification,payload=EXCLUDED.payload,tombstone=EXCLUDED.tombstone,occurred_at=EXCLUDED.occurred_at,received_at=EXCLUDED.received_at,coverage=EXCLUDED.coverage
WHERE admin_projections.entity_version < EXCLUDED.entity_version`, ev.SourceService, ev.EntityType, ev.EntityRef, ev.EntityVersion, adapter.ExportType, ev.Scope.Market, ev.Scope.Zone, ev.Scope.Organization, ev.Scope.Tenant, adapter.Classification, body, ev.Deleted, ev.OccurredAt, now, coverage); err != nil {
		return "", err
	}
	if delivery != DeliveryIngress {
		if _, err = tx.Exec(ctx, `UPDATE admin_projection_gaps SET repaired_at=$4 WHERE source_service=$1 AND entity_type=$2 AND entity_ref=$3 AND repaired_at IS NULL AND received_version<=$5`, ev.SourceService, ev.EntityType, ev.EntityRef, now, ev.EntityVersion); err != nil {
			return "", err
		}
	}
	export := adminevents.ExportEvent{Schema: adminevents.ExportSchema, EventID: ev.EventID, EventType: adapter.ExportType, SourceService: ev.SourceService, Product: adminevents.Product, EntityType: ev.EntityType, EntityRef: ev.EntityRef, EntityVersion: ev.EntityVersion, Tombstone: ev.Deleted, OccurredAt: ev.OccurredAt, EmittedAt: now, Market: ev.Scope.Market, Scope: adminevents.ExportScope{Zone: ev.Scope.Zone, Organization: ev.Scope.Organization, Tenant: ev.Scope.Tenant}, Classification: adapter.Classification, CorrelationID: ev.CorrelationID, Payload: payload}
	exportRaw, _ := json.Marshal(export)
	if _, err = tx.Exec(ctx, `INSERT INTO admin_hq_outbox(event_id,export_type,market,zone,organization_ref,tenant_ref,export) VALUES($1,$2,$3,$4,$5,$6,$7)`, ev.EventID, adapter.ExportType, ev.Scope.Market, ev.Scope.Zone, ev.Scope.Organization, ev.Scope.Tenant, exportRaw); err != nil {
		return "", err
	}
	return outcome, tx.Commit(ctx)
}

// Scope filters HQ reads. An empty event dimension means the owner does not
// record it canonically; such minimized events match any requested value of
// that dimension (detail reads remain owner-context scoped).
type Scope struct{ Market, Zone, Organization, Tenant string }

type FeedPage struct {
	Events       []json.RawMessage `json:"events"`
	NextSequence int64             `json:"next_sequence"`
	HeadSequence int64             `json:"head_sequence"`
	AsOf         time.Time         `json:"as_of"`
}

func (s *Store) Feed(ctx context.Context, after int64, limit int, scope Scope) (FeedPage, error) {
	if after < 0 || limit < 1 || limit > 500 {
		return FeedPage{}, ErrInvalid
	}
	page := FeedPage{Events: []json.RawMessage{}, NextSequence: after, AsOf: s.now()}
	if err := s.DB.QueryRow(ctx, `SELECT COALESCE(max(sequence),0) FROM admin_hq_outbox`).Scan(&page.HeadSequence); err != nil {
		return page, err
	}
	rows, err := s.DB.Query(ctx, `SELECT sequence, export || jsonb_build_object('sequence',sequence) FROM admin_hq_outbox
WHERE sequence>$1 AND (market='' OR market=$2) AND (zone='' OR zone=$3) AND (organization_ref='' OR organization_ref=$4) AND (tenant_ref='' OR tenant_ref=$5)
ORDER BY sequence LIMIT $6`, after, scope.Market, scope.Zone, scope.Organization, scope.Tenant, limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var seq int64
		var raw []byte
		if err := rows.Scan(&seq, &raw); err != nil {
			return page, err
		}
		page.Events = append(page.Events, raw)
		page.NextSequence = seq
	}
	return page, rows.Err()
}

var ErrCursor = errors.New("acknowledgement beyond head or regressing")

func (s *Store) Ack(ctx context.Context, consumer string, sequence int64, staff string) error {
	if !adminevents.Opaque(consumer) || sequence < 0 {
		return ErrInvalid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var head int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(sequence),0) FROM admin_hq_outbox`).Scan(&head); err != nil {
		return err
	}
	if sequence > head {
		return ErrCursor
	}
	var prior int64
	err = tx.QueryRow(ctx, `SELECT acknowledged_sequence FROM admin_hq_consumer_cursors WHERE consumer=$1 FOR UPDATE`, consumer).Scan(&prior)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		_, err = tx.Exec(ctx, `INSERT INTO admin_hq_consumer_cursors(consumer,acknowledged_sequence,acknowledged_by) VALUES($1,$2,$3)`, consumer, sequence, staff)
	case err != nil:
		return err
	case sequence < prior:
		return ErrCursor
	default:
		_, err = tx.Exec(ctx, `UPDATE admin_hq_consumer_cursors SET acknowledged_sequence=$2,acknowledged_by=$3,updated_at=now() WHERE consumer=$1`, consumer, sequence, staff)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SupportCase is the unified HQ queue row (Section 24). Identity is the
// composite product+source_service+source_case_id; never a bare number.
type SupportCase struct {
	Product        string         `json:"product"`
	SourceService  string         `json:"source_service"`
	SourceCaseID   string         `json:"source_case_id"`
	CompositeID    string         `json:"composite_id"`
	SourceVersion  int64          `json:"source_version"`
	Classification string         `json:"classification"`
	Organization   string         `json:"organization"`
	Fields         map[string]any `json:"fields"`
	ReceivedAt     time.Time      `json:"received_at"`
	Coverage       string         `json:"coverage"`
}
type SupportQueue struct {
	Items      []SupportCase `json:"items"`
	NextCursor string        `json:"next_cursor"`
	AsOf       time.Time     `json:"as_of"`
	Freshness  string        `json:"freshness"`
	Coverage   string        `json:"coverage"`
}

func (s *Store) SupportQueue(ctx context.Context, scope Scope, priority, status string, afterRef string, limit int) (SupportQueue, error) {
	if limit < 1 || limit > 100 || (afterRef != "" && !adminevents.Opaque(afterRef)) {
		return SupportQueue{}, ErrInvalid
	}
	q := SupportQueue{Items: []SupportCase{}, AsOf: s.now()}
	rows, err := s.DB.Query(ctx, `SELECT source_service,entity_ref,entity_version,classification,organization_ref,payload,received_at,coverage FROM admin_projections
WHERE product='karryam' AND entity_type='support_case' AND NOT tombstone
 AND (market='' OR market=$1) AND (zone='' OR zone=$2) AND (organization_ref='' OR organization_ref=$3) AND (tenant_ref='' OR tenant_ref=$4)
 AND ($5='' OR payload->>'priority'=$5) AND ($6='' OR payload->>'status'=$6) AND ($7='' OR entity_ref>$7)
ORDER BY CASE payload->>'priority' WHEN 'EMERGENCY' THEN 0 WHEN 'HIGH' THEN 1 ELSE 2 END, entity_ref LIMIT $8`, scope.Market, scope.Zone, scope.Organization, scope.Tenant, priority, status, afterRef, limit+1)
	if err != nil {
		return q, err
	}
	defer rows.Close()
	for rows.Next() {
		var c SupportCase
		var raw []byte
		if err := rows.Scan(&c.SourceService, &c.SourceCaseID, &c.SourceVersion, &c.Classification, &c.Organization, &raw, &c.ReceivedAt, &c.Coverage); err != nil {
			return q, err
		}
		_ = json.Unmarshal(raw, &c.Fields)
		c.Product = "karryam"
		c.CompositeID = "karryam:" + c.SourceService + ":" + c.SourceCaseID
		q.Items = append(q.Items, c)
	}
	if err := rows.Err(); err != nil {
		return q, err
	}
	if len(q.Items) > limit {
		q.Items = q.Items[:limit]
		q.NextCursor = q.Items[limit-1].SourceCaseID
	}
	st, err := s.Status(ctx, 15*time.Minute)
	if err != nil {
		return q, err
	}
	q.Freshness, q.Coverage = "UNKNOWN", "PARTIAL_COVERAGE"
	for _, src := range st.Sources {
		if src.SourceService == "support-dispute-service" && src.EntityType == "support_case" {
			q.Freshness, q.Coverage = src.Freshness, src.Coverage
		}
	}
	return q, nil
}

type SourceStatus struct {
	SourceService      string     `json:"source_service"`
	EntityType         string     `json:"entity_type"`
	Projections        int64      `json:"projections"`
	LastReceivedAt     *time.Time `json:"last_received_at"`
	LagSeconds         *int64     `json:"lag_seconds"`
	OpenGaps           int64      `json:"open_gaps"`
	Freshness          string     `json:"freshness"`
	Coverage           string     `json:"coverage"`
	LastBackfillStatus string     `json:"last_backfill_status"`
}
type Status struct {
	AsOf             time.Time        `json:"as_of"`
	FreshnessSeconds int64            `json:"freshness_threshold_seconds"`
	Sources          []SourceStatus   `json:"sources"`
	Quarantined      int64            `json:"quarantined"`
	HeadSequence     int64            `json:"head_sequence"`
	Consumers        map[string]int64 `json:"consumer_lag"`
}

// Status reports truthful freshness: no data -> UNKNOWN, never "fresh zero".
func (s *Store) Status(ctx context.Context, threshold time.Duration) (Status, error) {
	now := s.now()
	st := Status{AsOf: now, FreshnessSeconds: int64(threshold.Seconds()), Sources: []SourceStatus{}, Consumers: map[string]int64{}}
	known := map[string]bool{}
	for _, a := range adminevents.Adapters() {
		key := a.SourceService + "|" + a.EntityType
		if known[key] {
			continue
		}
		known[key] = true
		src := SourceStatus{SourceService: a.SourceService, EntityType: a.EntityType, Freshness: "UNKNOWN", Coverage: "PARTIAL_COVERAGE", LastBackfillStatus: "NONE"}
		var last *time.Time
		if err := s.DB.QueryRow(ctx, `SELECT count(*), max(received_at) FROM admin_projections WHERE source_service=$1 AND entity_type=$2`, a.SourceService, a.EntityType).Scan(&src.Projections, &last); err != nil {
			return st, err
		}
		src.LastReceivedAt = last
		if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_projection_gaps WHERE source_service=$1 AND entity_type=$2 AND repaired_at IS NULL`, a.SourceService, a.EntityType).Scan(&src.OpenGaps); err != nil {
			return st, err
		}
		var status string
		err := s.DB.QueryRow(ctx, `SELECT status FROM admin_backfill_runs WHERE source_service=$1 AND entity_type=$2 ORDER BY started_at DESC LIMIT 1`, a.SourceService, a.EntityType).Scan(&status)
		if err == nil {
			src.LastBackfillStatus = status
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return st, err
		}
		if status == "COMPLETE" && src.OpenGaps == 0 {
			src.Coverage = "COMPLETE"
		}
		if last != nil {
			lag := int64(now.Sub(*last).Seconds())
			src.LagSeconds = &lag
			src.Freshness = "LIVE"
			if now.Sub(*last) > threshold {
				src.Freshness = "STALE"
			}
		}
		st.Sources = append(st.Sources, src)
	}
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_event_quarantine`).Scan(&st.Quarantined); err != nil {
		return st, err
	}
	if err := s.DB.QueryRow(ctx, `SELECT COALESCE(max(sequence),0) FROM admin_hq_outbox`).Scan(&st.HeadSequence); err != nil {
		return st, err
	}
	rows, err := s.DB.Query(ctx, `SELECT consumer, acknowledged_sequence FROM admin_hq_consumer_cursors ORDER BY consumer`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		var seq int64
		if err := rows.Scan(&c, &seq); err != nil {
			return st, err
		}
		st.Consumers[c] = st.HeadSequence - seq
	}
	return st, rows.Err()
}

// ClaimNonce durably records an ingress nonce; a repeat is a replay.
func (s *Store) ClaimNonce(ctx context.Context, source, nonce string, expires time.Time) error {
	tag, err := s.DB.Exec(ctx, `INSERT INTO admin_event_ingress_nonces(source_service,nonce_hash,expires_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, source, sum([]byte(source+"\x00"+nonce)), expires)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrReplay
	}
	_, _ = s.DB.Exec(ctx, `DELETE FROM admin_event_ingress_nonces WHERE expires_at < now() - interval '1 minute'`)
	return nil
}

var ErrReplay = errors.New("replayed ingress nonce")

func newID() string { return uuid.NewString() }
