package movement

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/observability"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Backfill and reconciliation.
//
// Taxi publishes a movement event with every transition. A projection can
// still fall behind: events created before the consumer existed, a dead
// letter, or a customer-service outage long enough to exhaust retries.
//
// Reconciliation never invents data and never replays blindly. It reads
// Taxi's OWN committed bookings, asks customer-service what it already holds
// for those exact references, and republishes only the ones that are missing
// or behind. Running it from the beginning of time is the backfill; running
// it over a recent window is the ongoing safety net.

// ProjectionReader answers what the customer projection currently holds.
type ProjectionReader interface {
	ProjectedVersions(ctx context.Context, sourceRefs []string) (map[string]int64, error)
}

// CustomerProjectionClient reads the projection over signed internal HTTP.
type CustomerProjectionClient struct {
	BaseURL     string
	ServiceName string
	Secret      []byte
	HTTPClient  *http.Client
	Now         func() time.Time
}

func (c *CustomerProjectionClient) ProjectedVersions(ctx context.Context, sourceRefs []string) (map[string]int64, error) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || len(c.Secret) == 0 {
		return nil, fmt.Errorf("customer projection client is not configured")
	}
	if len(sourceRefs) == 0 {
		return map[string]int64{}, nil
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	target := strings.TrimRight(c.BaseURL, "/") + events.CustomerMovementStatePath + "?source_refs=" + url.QueryEscape(strings.Join(sourceRefs, ","))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if err = serviceauth.SignRequest(req, c.ServiceName, c.Secret, nil, now()); err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("customer movement state: status %d", resp.StatusCode)
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Versions map[string]int64 `json:"versions"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil || !envelope.Success {
		return nil, fmt.Errorf("customer movement state: unreadable answer")
	}
	return envelope.Data.Versions, nil
}

// Reconciler republishes movements the customer projection is missing.
type Reconciler struct {
	Pool   *pgxpool.Pool
	Writer *Writer
	Reader ProjectionReader
	Batch  int
	// Metrics, when set, receives Customer V4 §27.2 reconciliation evidence.
	Metrics *observability.Metrics
}

// ReconcileResult reports one pass.
type ReconcileResult struct {
	Checked      int
	Republished  int
	OldestPassed time.Time
}

// Reconcile inspects bookings updated since the cursor and republishes any
// whose projected version is missing or behind Taxi's own.
func (r *Reconciler) Reconcile(ctx context.Context, since time.Time) (ReconcileResult, error) {
	var result ReconcileResult
	if r == nil || r.Pool == nil || r.Writer == nil || !r.Writer.Enabled || r.Reader == nil {
		return result, nil
	}
	batch := r.Batch
	if batch <= 0 {
		batch = 200
	}
	rows, err := r.Pool.Query(ctx, `SELECT id::text, movement_version, updated_at FROM taxi_bookings
		WHERE updated_at > $1 ORDER BY updated_at LIMIT $2`, since, batch)
	if err != nil {
		return result, err
	}
	type candidate struct {
		id      string
		version int64
	}
	var candidates []candidate
	newest := since
	for rows.Next() {
		var row candidate
		var updated time.Time
		if err = rows.Scan(&row.id, &row.version, &updated); err != nil {
			rows.Close()
			return result, err
		}
		if updated.After(newest) {
			newest = updated
		}
		candidates = append(candidates, row)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	result.Checked, result.OldestPassed = len(candidates), newest
	if len(candidates) == 0 {
		return result, nil
	}
	refs := make([]string, 0, len(candidates))
	for _, row := range candidates {
		refs = append(refs, row.id)
	}
	projected, err := r.Reader.ProjectedVersions(ctx, refs)
	if err != nil {
		return result, err
	}
	for _, row := range candidates {
		if have, ok := projected[row.id]; ok && have >= row.version {
			continue
		}
		republished, err := r.republish(ctx, row.id)
		if err != nil {
			return result, err
		}
		if republished {
			result.Republished++
		}
	}
	return result, nil
}

// republish re-emits the booking's CURRENT committed state. It reads the
// booking inside the transaction that writes the event, so the published
// summary is never a state Taxi does not hold.
func (r *Reconciler) republish(ctx context.Context, bookingID string) (bool, error) {
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var booking bookingmodels.Booking
	var status, purpose string
	if err = tx.QueryRow(ctx, `SELECT id::text, customer_id::text, status, ride_purpose, scheduled_at
		FROM taxi_bookings WHERE id=$1 FOR UPDATE`, bookingID).
		Scan(&booking.ID, &booking.CustomerID, &status, &purpose, &booking.ScheduledAt); err != nil {
		return false, err
	}
	booking.Status = bookingmodels.Status(status)
	booking.RidePurpose = bookingmodels.RidePurpose(purpose)
	stops, err := tx.Query(ctx, `SELECT type, address, lat, lng FROM taxi_booking_stops WHERE booking_id=$1 ORDER BY sequence`, bookingID)
	if err != nil {
		return false, err
	}
	for stops.Next() {
		var stop bookingmodels.Stop
		var stopType string
		if err = stops.Scan(&stopType, &stop.Address, &stop.Lat, &stop.Lng); err != nil {
			stops.Close()
			return false, err
		}
		stop.Type = bookingmodels.StopType(stopType)
		booking.Stops = append(booking.Stops, stop)
	}
	stops.Close()
	if err = stops.Err(); err != nil {
		return false, err
	}
	if _, ok := BookingState(booking.Status); !ok {
		// A status with no movement meaning publishes nothing rather than a
		// guessed state.
		return false, nil
	}
	if err = r.Writer.WriteBooking(ctx, tx, booking, "reconcile:"+bookingID); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// Run backfills from the beginning of time and then reconciles a recent
// window on every tick.
func (r *Reconciler) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	cursor := time.Time{}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		result, err := r.Reconcile(ctx, cursor)
		if r.Metrics != nil {
			outcome := "success"
			if err != nil {
				outcome = "dependency_error"
			}
			r.Metrics.IncCounter("karry_movement_reconciliation_runs_total", map[string]string{"service": SourceService, "outcome": outcome}, 1)
			r.Metrics.IncCounter("karry_movement_reconciliation_republished_total", map[string]string{"service": SourceService}, float64(result.Republished))
		}
		if err != nil {
			logging.Error("taxi-movement-reconcile", "%v", err)
		} else {
			if result.Republished > 0 {
				logging.Notice("taxi-movement-reconcile", "checked=%d republished=%d", result.Checked, result.Republished)
			}
			cursor = result.OldestPassed
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
