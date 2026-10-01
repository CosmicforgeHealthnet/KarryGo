package main

// Customer V4 §22 purchase Activity row through the real order lifecycle and
// the real outbox, plus authority-driven reconciliation against a Customer
// movement-state endpoint double.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	sharedevents "cosmicforge/logistics/shared/go/events"
)

type queuedPurchaseRow struct {
	version int64
	summary sharedevents.MovementSummary
	raw     string
}

func purchaseRows(t *testing.T, env *s17bEnv, parentID string) []queuedPurchaseRow {
	t.Helper()
	rows, err := env.pool.Query(context.Background(), `SELECT aggregate_version,payload FROM vendor_outbox
		WHERE event_type=$1 AND aggregate_id=$2 ORDER BY aggregate_version,created_at`, orders.CustomerOrderMovementEvent, parentID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []queuedPurchaseRow
	for rows.Next() {
		var row queuedPurchaseRow
		var raw []byte
		if err = rows.Scan(&row.version, &raw); err != nil {
			t.Fatal(err)
		}
		var outer struct {
			Payload sharedevents.Envelope `json:"payload"`
		}
		if json.Unmarshal(raw, &outer) != nil || outer.Payload.Validate() != nil || json.Unmarshal(outer.Payload.Payload, &row.summary) != nil {
			t.Fatalf("queued purchase row is not a valid shared envelope: %s", raw)
		}
		if outer.Payload.SourceVersion != row.version {
			t.Fatalf("envelope version %d differs from outbox version %d", outer.Payload.SourceVersion, row.version)
		}
		row.raw = string(raw)
		out = append(out, row)
	}
	return out
}

func TestCustomerOrderMovementCancelledBeforeDispatchAndReconciled(t *testing.T) {
	env := newSection17BEnv(t, 15*time.Minute)
	ctx := context.Background()
	item := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Goods", PriceMinor: 1000, Quantity: 100})
	lifecycleExec(t, env, `UPDATE shop_variants SET net_weight_grams=1000,stackable=true WHERE id=$1`, item.VariantID)
	customer := uuid.NewString()
	run := runToPaidOrder(t, env, customer, env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1}), env.branchA1)
	vendorOrder := run.order.VendorOrders[0]

	current, err := orders.NewStore(env.pool).GetVendorOrderForCustomer(ctx, customer, vendorOrder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = run.service.Advance(ctx, run.actor(), vendorOrder.ID, orders.TransitionCancel, "OUT_OF_STOCK", current.Version); err != nil {
		t.Fatalf("vendor cancel: %v", err)
	}

	rows := purchaseRows(t, env, run.order.ID)
	if len(rows) < 2 {
		t.Fatalf("purchase rows=%d, want creation and the cancellation", len(rows))
	}
	first, last := rows[0], rows[len(rows)-1]
	if first.summary.State != sharedevents.MovementStateRequested || first.summary.ProductCode != "SHOP" ||
		first.summary.Authority != sharedevents.MovementAuthorityVendorCommerce || first.summary.ParentContextRef != run.order.ID {
		t.Fatalf("creation row = %+v", first.summary)
	}
	if last.summary.State != sharedevents.MovementStateCancelled {
		t.Fatalf("cancelled purchase row = %+v", last.summary)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].version <= rows[i-1].version {
			t.Fatalf("purchase row versions are not strictly increasing: %d then %d", rows[i-1].version, rows[i].version)
		}
	}
	for _, row := range rows {
		for _, private := range []string{"amount", "minor", "payment", "wallet", "otp", "qr_", "provider_id", "provider_ref", env.businessA, env.staffA} {
			if strings.Contains(row.raw, private) {
				t.Fatalf("purchase row leaks %q: %s", private, row.raw)
			}
		}
	}

	// Reconciliation: every queued row is delivered; Customer then reports
	// it holds an older version. Vendor republishes current once, and never
	// again while that row waits; nothing about the order changes.
	if _, err = env.pool.Exec(ctx, `UPDATE vendor_outbox SET status='PUBLISHED',published_at=now() WHERE event_type=$1 AND aggregate_id=$2`,
		orders.CustomerOrderMovementEvent, run.order.ID); err != nil {
		t.Fatal(err)
	}
	var beforeState string
	var beforeVersion int64
	if err = env.pool.QueryRow(ctx, `SELECT state,version FROM commerce_parent_orders WHERE id=$1`, run.order.ID).Scan(&beforeState, &beforeVersion); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	projected := int64(1)
	customerState := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != sharedevents.CustomerMovementStatePath || r.URL.Query().Get("authority") != sharedevents.MovementAuthorityVendorCommerce ||
			r.Header.Get("X-Service-Name") == "" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"versions": map[string]int64{run.order.ID: projected}}})
	}))
	defer customerState.Close()
	reconciler := &customerOrderMovementReconciler{db: env.pool, outbox: events.NewOutboxRepository(env.pool),
		baseURL: customerState.URL, secret: []byte("reconcile-secret")}
	sweepAll := func() int {
		t.Helper()
		total := 0
		for i := 0; i < 100; i++ {
			n, err := reconciler.Sweep(ctx)
			if err != nil {
				t.Fatal(err)
			}
			total += n
			if reconciler.after == "" {
				break
			}
		}
		return total
	}
	sweepAll()
	rows = purchaseRows(t, env, run.order.ID)
	if republished := rows[len(rows)-1]; republished.version != beforeVersion || republished.summary.State != sharedevents.MovementStateCancelled {
		t.Fatalf("republished row = v%d %+v, want the current v%d", republished.version, republished.summary, beforeVersion)
	}
	count := len(rows)
	sweepAll()
	if len(purchaseRows(t, env, run.order.ID)) != count {
		t.Fatal("a waiting republication was queued again")
	}
	mu.Lock()
	projected = beforeVersion
	mu.Unlock()
	if _, err = env.pool.Exec(ctx, `UPDATE vendor_outbox SET status='PUBLISHED',published_at=now() WHERE event_type=$1 AND aggregate_id=$2`,
		orders.CustomerOrderMovementEvent, run.order.ID); err != nil {
		t.Fatal(err)
	}
	sweepAll()
	if len(purchaseRows(t, env, run.order.ID)) != count {
		t.Fatal("a converged projection was republished")
	}
	var afterState string
	var afterVersion int64
	if err = env.pool.QueryRow(ctx, `SELECT state,version FROM commerce_parent_orders WHERE id=$1`, run.order.ID).Scan(&afterState, &afterVersion); err != nil {
		t.Fatal(err)
	}
	if afterState != beforeState || afterVersion != beforeVersion {
		t.Fatalf("reconciliation changed the authoritative order: %s/v%d -> %s/v%d", beforeState, beforeVersion, afterState, afterVersion)
	}
}
