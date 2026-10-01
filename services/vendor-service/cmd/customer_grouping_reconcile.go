package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	shared "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Replays recorded grouping metadata only. It never derives mission ordering,
// lifecycle, assignment or price from a commercial order.
type customerGroupingReconciler struct {
	publisher *customerGroupingPublisher
	baseURL   string
	secret    []byte
	after     string
}

func (r *customerGroupingReconciler) Sweep(ctx context.Context) (int, error) {
	rows, err := r.publisher.db.Query(ctx, `SELECT DISTINCT ON(v.aggregate_id) v.aggregate_id,v.payload->'payload',o.customer_id,o.domain
 FROM vendor_outbox v JOIN commerce_parent_orders o ON o.id=v.aggregate_id WHERE v.event_type=$1 AND v.aggregate_id>$2
 ORDER BY v.aggregate_id,v.aggregate_version DESC,v.created_at DESC LIMIT 50`, shared.CommerceGroupingEventName, r.after)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		ref, owner, product string
		raw                 []byte
		envelope            shared.Envelope
		grouping            shared.MovementGrouping
	}
	var items []candidate
	for rows.Next() {
		var item candidate
		if err = rows.Scan(&item.ref, &item.raw, &item.owner, &item.product); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		r.after = ""
		return 0, nil
	}
	var current []candidate
	var refs []string
	repaired := 0
	for _, item := range items {
		if json.Unmarshal(item.raw, &item.envelope) != nil || item.envelope.Validate() != nil || json.Unmarshal(item.envelope.Payload, &item.grouping) != nil || item.grouping.Validate() != nil {
			continue
		}
		if item.grouping.CustomerID != item.owner || item.grouping.ParentContextRef != item.ref || (item.product != "FOOD" && item.product != "SHOP") {
			continue
		}
		var version int64
		err = r.publisher.db.QueryRow(ctx, `SELECT source_version FROM commerce_customer_grouping_emissions WHERE parent_order_id=$1`, item.ref).Scan(&version)
		if err == pgx.ErrNoRows {
			// Old outbox records preserve canonical mission order. Fill only the
			// missing checkpoint, so a concurrent new publication always wins.
			item.grouping.ProductCode = item.product
			if err = r.publisher.publish(ctx, item.ref, item.grouping, true); err != nil {
				return repaired, err
			}
			repaired++
			continue
		}
		if err != nil {
			return repaired, err
		}
		if version != item.envelope.SourceVersion {
			continue
		}
		current = append(current, item)
		refs = append(refs, item.ref)
	}
	if len(current) > 0 {
		request, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(r.baseURL, "/")+shared.CustomerMovementStatePath+"?source_refs="+url.QueryEscape(strings.Join(refs, ",")), nil)
		if err != nil {
			return repaired, err
		}
		if err = serviceauth.SignRequest(request, "vendor-service", r.secret, nil, time.Now().UTC()); err != nil {
			return repaired, err
		}
		response, err := (&http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
		if err != nil {
			return repaired, err
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return repaired, fmt.Errorf("grouping projection status %d", response.StatusCode)
		}
		var result struct {
			Success bool `json:"success"`
			Data    struct {
				Versions map[string]int64 `json:"versions"`
			} `json:"data"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result) != nil || !result.Success {
			return repaired, fmt.Errorf("invalid grouping projection response")
		}
		for _, item := range current {
			if result.Data.Versions[item.ref] >= item.envelope.SourceVersion {
				continue
			}
			queued, err := r.requeue(ctx, item.ref, item.envelope.SourceVersion, item.grouping)
			if err != nil {
				return repaired, err
			}
			if queued {
				repaired++
			}
		}
	}
	r.after = items[len(items)-1].ref
	return repaired, nil
}

func (r *customerGroupingReconciler) requeue(ctx context.Context, ref string, version int64, grouping shared.MovementGrouping) (bool, error) {
	tx, err := r.publisher.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int64
	err = tx.QueryRow(ctx, `SELECT source_version FROM commerce_customer_grouping_emissions WHERE parent_order_id=$1 FOR UPDATE`, ref).Scan(&current)
	if err != nil {
		return false, err
	}
	if current != version {
		return false, nil
	}
	var pending bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_outbox WHERE event_type=$1 AND aggregate_id=$2 AND aggregate_version=$3 AND status='PENDING')`, shared.CommerceGroupingEventName, ref, version).Scan(&pending)
	if err != nil {
		return false, err
	}
	if pending {
		return false, nil
	}
	inner, err := shared.NewGroupingEnvelope(uuid.NewString(), "vendor-service", ref, version, time.Now().UTC(), ref, grouping)
	if err != nil {
		return false, err
	}
	outer, err := events.NewEnvelope(shared.CommerceGroupingEventName, 1, "commerce_order", ref, version, "", ref, "", inner, true)
	if err != nil {
		return false, err
	}
	if err = r.publisher.outbox.Append(ctx, tx, outer); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (r *customerGroupingReconciler) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		count, err := r.Sweep(ctx)
		if ctx.Err() == nil {
			outcome := "success"
			if err != nil {
				outcome = "dependency_error"
			}
			logging.Structured("info", "commerce_grouping_reconciliation", map[string]any{"outcome": outcome, "repaired": count})
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
