package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	shared "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// customerOrderMovementHTTPPublisher unwraps the Vendor outbox envelope and
// sends Customer the shared movement envelope it carries, with that
// envelope's own event ID, to the existing movement ingress.
type customerOrderMovementHTTPPublisher struct{ inner events.Publisher }

func (p customerOrderMovementHTTPPublisher) Publish(ctx context.Context, record events.OutboxRecord) error {
	var outer struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(record.Payload, &outer); err != nil {
		return err
	}
	var envelope shared.Envelope
	if err := json.Unmarshal(outer.Payload, &envelope); err != nil || envelope.Validate() != nil {
		return fmt.Errorf("invalid customer order movement envelope")
	}
	var summary shared.MovementSummary
	if err := json.Unmarshal(envelope.Payload, &summary); err != nil || summary.Validate() != nil ||
		summary.Authority != shared.MovementAuthorityVendorCommerce {
		return fmt.Errorf("invalid customer order movement payload")
	}
	record.Payload = outer.Payload
	return p.inner.Publish(ctx, record)
}

// customerOrderMovementReconciler lets Vendor, the ParentOrder authority,
// compare Customer's projected purchase-row version with the aggregate's own
// version and queue the current row when Customer is missing it or behind.
// It reads and republishes; it never changes an order.
type customerOrderMovementReconciler struct {
	db      *pgxpool.Pool
	outbox  *events.OutboxRepository
	baseURL string
	secret  []byte
	client  *http.Client
	after   string
	batch   int
}

func (r *customerOrderMovementReconciler) projected(ctx context.Context, refs []string) (map[string]int64, error) {
	endpoint := strings.TrimRight(r.baseURL, "/") + shared.CustomerMovementStatePath +
		"?authority=" + shared.MovementAuthorityVendorCommerce + "&source_refs=" + url.QueryEscape(strings.Join(refs, ","))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if err = serviceauth.SignRequest(request, orders.CustomerOrderMovementSourceService, r.secret, nil, time.Now().UTC()); err != nil {
		return nil, err
	}
	client := r.client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("customer order movement projection status %d", response.StatusCode)
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Versions map[string]int64 `json:"versions"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result) != nil || !result.Success {
		return nil, fmt.Errorf("invalid customer order movement projection response")
	}
	return result.Data.Versions, nil
}

// Sweep checks one bounded keyset batch and returns how many rows it queued.
func (r *customerOrderMovementReconciler) Sweep(ctx context.Context) (int, error) {
	batch := r.batch
	if batch <= 0 || batch > 200 {
		batch = 200
	}
	rows, err := r.db.Query(ctx, `SELECT id,version FROM commerce_parent_orders
		WHERE id>$1 AND domain IN ('FOOD','SHOP') ORDER BY id LIMIT $2`, r.after, batch)
	if err != nil {
		return 0, err
	}
	versions := map[string]int64{}
	var refs []string
	for rows.Next() {
		var ref string
		var version int64
		if err = rows.Scan(&ref, &version); err != nil {
			rows.Close()
			return 0, err
		}
		versions[ref] = version
		refs = append(refs, ref)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	if len(refs) == 0 {
		r.after = ""
		return 0, nil
	}
	projected, err := r.projected(ctx, refs)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, ref := range refs {
		if projected[ref] >= versions[ref] {
			continue
		}
		ok, err := r.requeue(ctx, ref)
		if err != nil {
			return queued, err
		}
		if ok {
			queued++
		}
	}
	if len(refs) < batch {
		r.after = ""
	} else {
		r.after = refs[len(refs)-1]
	}
	return queued, nil
}

// requeue queues the current row unless one for this version is already
// waiting, so a slow worker is not flooded with duplicates. Customer's inbox
// and version guard make any duplicate that does slip through harmless.
func (r *customerOrderMovementReconciler) requeue(ctx context.Context, ref string) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT id FROM commerce_parent_orders WHERE id=$1 FOR SHARE`, ref); err != nil {
		return false, err
	}
	envelope, ok, err := orders.CustomerOrderMovementEnvelope(ctx, tx, ref, "")
	if err != nil || !ok {
		return false, err
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_outbox WHERE event_type=$1 AND aggregate_id=$2
		AND aggregate_version=$3 AND status='PENDING')`, orders.CustomerOrderMovementEvent, ref, envelope.SourceVersion).Scan(&pending); err != nil {
		return false, err
	}
	if pending {
		return false, nil
	}
	outer, err := events.NewEnvelope(orders.CustomerOrderMovementEvent, 1, "ParentOrder", ref, envelope.SourceVersion, "", ref, "", envelope, true)
	if err != nil {
		return false, err
	}
	if err = r.outbox.Append(ctx, tx, outer); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (r *customerOrderMovementReconciler) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		count, err := r.Sweep(ctx)
		if ctx.Err() == nil {
			outcome := "success"
			if err != nil {
				outcome = "dependency_error"
			}
			logging.Structured("info", "commerce_order_movement_reconciliation", map[string]any{"outcome": outcome, "repaired": count})
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
