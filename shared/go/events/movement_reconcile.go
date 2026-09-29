package events

import (
	"context"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/observability"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MovementReconciler repairs lost/parked deliveries from the owner's durable
// versioned outbox. It never guesses or increments a business source version.
type MovementReconciler struct {
	Pool                 *pgxpool.Pool
	BaseURL, ServiceName string
	Secret               []byte
	After                string
	// Metrics, when set, receives Customer V4 §27.2 reconciliation evidence:
	// sweep outcomes and projection-behind republications per service.
	Metrics *observability.Metrics
}

func (r *MovementReconciler) Sweep(ctx context.Context) (int, error) {
	rows, err := r.Pool.Query(ctx, `SELECT DISTINCT ON(source_ref) source_ref,source_version,payload FROM customer_movement_outbox WHERE source_ref>$1 ORDER BY source_ref,source_version DESC,id DESC LIMIT 100`, r.After)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		ref     string
		version int64
		raw     []byte
	}
	var items []candidate
	var refs []string
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.ref, &c.version, &c.raw); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, c)
		refs = append(refs, c.ref)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		r.After = ""
		return 0, nil
	}
	request, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(r.BaseURL, "/")+CustomerMovementStatePath+"?source_refs="+url.QueryEscape(strings.Join(refs, ",")), nil)
	if err != nil {
		return 0, err
	}
	if err = serviceauth.SignRequest(request, r.ServiceName, r.Secret, nil, time.Now().UTC()); err != nil {
		return 0, err
	}
	response, err := (&http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return 0, fmt.Errorf("projection read status %d", response.StatusCode)
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Versions map[string]int64 `json:"versions"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&envelope) != nil || !envelope.Success {
		return 0, fmt.Errorf("invalid projection read")
	}
	repaired := 0
	for _, item := range items {
		if envelope.Data.Versions[item.ref] >= item.version {
			continue
		}
		var event Envelope
		if json.Unmarshal(item.raw, &event) != nil || event.Validate() != nil {
			return repaired, ErrInvalidEnvelope
		}
		event.EventID = uuid.NewString()
		raw, _ := json.Marshal(event)
		tag, e := r.Pool.Exec(ctx, `INSERT INTO customer_movement_outbox(event_id,authority,source_ref,source_version,payload) SELECT $1,authority,source_ref,source_version,$2 FROM customer_movement_outbox WHERE source_ref=$3 AND source_version=$4 AND NOT EXISTS(SELECT 1 FROM customer_movement_outbox WHERE source_ref=$3 AND source_version=$4 AND status='PENDING') ORDER BY id DESC LIMIT 1`, event.EventID, raw, item.ref, item.version)
		if e != nil {
			return repaired, e
		}
		repaired += int(tag.RowsAffected())
	}
	r.After = items[len(items)-1].ref
	return repaired, nil
}
func (r *MovementReconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		count, err := r.Sweep(ctx)
		if ctx.Err() == nil {
			outcome := "success"
			if err != nil {
				outcome = "dependency_error"
			}
			logging.Structured("info", "movement_reconciliation", map[string]any{"service": r.ServiceName, "outcome": outcome, "repaired": count})
			if r.Metrics != nil {
				r.Metrics.IncCounter("karry_movement_reconciliation_runs_total", map[string]string{"service": r.ServiceName, "outcome": outcome}, 1)
				r.Metrics.IncCounter("karry_movement_reconciliation_republished_total", map[string]string{"service": r.ServiceName}, float64(count))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
