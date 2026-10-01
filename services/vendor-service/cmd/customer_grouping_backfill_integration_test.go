package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	fg "cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	shared "cosmicforge/logistics/shared/go/events"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHistoricalGroupingBackfillCanonicalSequenceDryRunAndResume(t *testing.T) {
	f := newSection17EFixture(t, true)
	ctx := context.Background()
	parent, _, err := f.service.CreateFromQuote(ctx, f.customerID, f.quoteID, f.finger)
	if err != nil {
		t.Fatal(err)
	}
	// A second immutable purchase has no operational plan: classify it rather
	// than infer delivery order from its UUID or commercial allocations.
	missing := newSection17EFixture(t, false)
	unrecoverable, _, err := missing.service.CreateFromQuote(ctx, missing.customerID, missing.quoteID, missing.finger)
	if err != nil {
		t.Fatal(err)
	}
	refs := seedCanonicalPlan(t, f, parent)
	p := &customerGroupingPublisher{db: f.env.pool, outbox: events.NewOutboxRepository(f.env.pool)}
	cursor := ""
	eligible := 0
	for {
		c, next, done, e := p.BackfillBatch(ctx, "dry-"+parent.ID, cursor, true)
		if e != nil {
			t.Fatal(e)
		}
		eligible += c.Eligible
		cursor = next
		if done {
			break
		}
	}
	if eligible < 1 {
		t.Fatal("canonical evidence not eligible")
	}
	var present bool
	if err = f.env.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_customer_grouping_emissions WHERE parent_order_id=$1)`, parent.ID).Scan(&present); err != nil || present {
		t.Fatal("dry run mutated checkpoint", present, err)
	}
	run := uuid.NewString()
	for {
		_, _, done, e := p.BackfillBatch(ctx, run, "", false)
		if e != nil {
			t.Fatal(e)
		}
		if done {
			break
		}
	}
	c, _, done, err := p.BackfillBatch(ctx, run, "", false)
	if err != nil || !done || c.Repaired != 0 {
		t.Fatal("resume not idempotent", c, done, err)
	}
	var raw []byte
	if err = f.env.pool.QueryRow(ctx, `SELECT payload->'payload'->'payload' FROM vendor_outbox WHERE event_type=$1 AND aggregate_id=$2 ORDER BY aggregate_version DESC LIMIT 1`, shared.CommerceGroupingEventName, parent.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var reconstructed shared.MovementGrouping
	if err = json.Unmarshal(raw, &reconstructed); err != nil {
		t.Fatal(err)
	}
	if len(refs) != len(reconstructed.Missions) {
		t.Fatal(reconstructed)
	}
	for i, ref := range refs {
		if reconstructed.Missions[i].FulfilmentGroupRef != ref || reconstructed.Missions[i].MissionSequence != i+1 {
			t.Fatal("canonical sequence changed", reconstructed)
		}
	}
	var outcome string
	if err = f.env.pool.QueryRow(ctx, `SELECT outcome FROM commerce_customer_grouping_backfill_results WHERE run_id=$1 AND parent_order_id=$2`, run, parent.ID).Scan(&outcome); err != nil || outcome != "REPAIRED" {
		t.Fatal(outcome, err)
	}
	var insufficient int
	if err = f.env.pool.QueryRow(ctx, `SELECT count(*) FROM commerce_customer_grouping_backfill_results WHERE run_id=$1 AND outcome='UNRECOVERABLE'`, run).Scan(&insufficient); err != nil {
		t.Fatal(err)
	}
	if insufficient < 1 {
		t.Fatal("missing evidence not classified")
	}
	if err = f.env.pool.QueryRow(ctx, `SELECT outcome FROM commerce_customer_grouping_backfill_results WHERE run_id=$1 AND parent_order_id=$2`, run, unrecoverable.ID).Scan(&outcome); err != nil || outcome != "UNRECOVERABLE" {
		t.Fatal(outcome, err)
	}
	t.Logf("backfill repaired canonical parent; insufficient evidence records=%d", insufficient)
}

// seedCanonicalPlan gives a parent an ACTIVE plan with one bound mission per
// vendor order, returning the mission refs in persisted slot order.
func seedCanonicalPlan(t *testing.T, f section17EFixture, parent orders.ParentOrder) []string {
	t.Helper()
	ctx := context.Background()
	store := fg.NewStore(f.env.pool, nil)
	now := time.Now().UTC()
	group, err := store.EnsureGroup(ctx, parent.ID, parent.CustomerID, now)
	if err != nil {
		t.Fatal(err)
	}
	var slots []fg.PlanMission
	for i, v := range parent.VendorOrders {
		unit, e := store.EnsureUnit(ctx, group, fg.UnitInput{VendorOrderID: v.ID}, now)
		if e != nil {
			t.Fatal(e)
		}
		parcel, e := store.CreateParcel(ctx, v.BusinessID, unit.ID, fg.ParcelInput{IdempotencyKey: uuid.NewString(), WeightGrams: 100}, now)
		if e != nil {
			t.Fatal(e)
		}
		slots = append(slots, fg.PlanMission{SlotRef: uuid.NewString(), Sequence: i + 1, Owner: fg.OwnerVendor, Authority: fg.AuthorityVendor, Currency: "NGN", ParcelIDs: []string{parcel.ID}, Stops: []fg.Stop{{Sequence: 1, Type: fg.StopPickup, UnitID: unit.ID}, {Sequence: 2, Type: fg.StopDropoff, Address: "Receiver"}}})
	}
	plan, err := store.ActivatePlan(ctx, group.ID, fg.PlanInput{ExpectedVersion: 0, Reason: fg.ReasonInitialPlan, DeliveryMode: fg.ModeVendor, Currency: "NGN", IdempotencyKey: uuid.NewString(), Missions: slots}, now)
	if err != nil {
		t.Fatal(err)
	}
	var refs []string
	for _, slot := range plan.Missions {
		m, e := store.EnsureVendorMission(ctx, group.ID, plan.ID, slot.ID, now)
		if e != nil {
			t.Fatal(e)
		}
		refs = append(refs, m.ID)
	}
	_ = plan
	return refs
}

// Concurrent runs, an already-correct checkpoint and the absence of any
// operational side effect: backfill publishes grouping only.
func TestHistoricalGroupingBackfillConcurrentAndSideEffectFree(t *testing.T) {
	f := newSection17EFixture(t, true)
	ctx := context.Background()
	repairable, _, err := f.service.CreateFromQuote(ctx, f.customerID, f.quoteID, f.finger)
	if err != nil {
		t.Fatal(err)
	}
	seedCanonicalPlan(t, f, repairable)
	correct := newSection17EFixture(t, true)
	already, _, err := correct.service.CreateFromQuote(ctx, correct.customerID, correct.quoteID, correct.finger)
	if err != nil {
		t.Fatal(err)
	}
	seedCanonicalPlan(t, correct, already)
	pool := f.env.pool
	// A live publication already produced a checkpoint for this parent.
	if _, err = pool.Exec(ctx, `INSERT INTO commerce_customer_grouping_emissions(parent_order_id,source_version,fingerprint,updated_at) VALUES($1,7,$2,'2020-01-01T00:00:00Z')`, already.ID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	type snapshot struct {
		orderMissions, vendorMissions, platformRefs, nonGroupingOutbox int
		states                                                         string
	}
	snap := func() snapshot {
		var s snapshot
		if e := pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM commerce_order_missions),
			(SELECT count(*) FROM commerce_vendor_missions),
			(SELECT count(*) FROM commerce_platform_mission_refs),
			(SELECT count(*) FROM vendor_outbox WHERE event_type<>$1),
			(SELECT string_agg(id||':'||state,',' ORDER BY id) FROM commerce_parent_orders WHERE id IN ($2,$3))`, shared.CommerceGroupingEventName, repairable.ID, already.ID).Scan(&s.orderMissions, &s.vendorMissions, &s.platformRefs, &s.nonGroupingOutbox, &s.states); e != nil {
			t.Fatal(e)
		}
		return s
	}
	before := snap()
	var vendorOrderStates string
	orderStates := func() string {
		var out string
		if e := pool.QueryRow(ctx, `SELECT string_agg(id||':'||status,',' ORDER BY id) FROM commerce_vendor_orders WHERE parent_order_id IN ($1,$2)`, repairable.ID, already.ID).Scan(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	vendorOrderStates = orderStates()

	p := &customerGroupingPublisher{db: pool, outbox: events.NewOutboxRepository(pool)}
	runs := []string{"concurrent-a-" + uuid.NewString(), "concurrent-b-" + uuid.NewString(), "concurrent-c-" + uuid.NewString()}
	var wg sync.WaitGroup
	errs := make(chan error, len(runs))
	for _, run := range runs {
		wg.Add(1)
		go func(run string) {
			defer wg.Done()
			for {
				_, _, done, e := p.BackfillBatch(ctx, run, "", false)
				if e != nil {
					errs <- e
					return
				}
				if done {
					return
				}
			}
		}(run)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal("concurrent backfill:", e)
	}
	// Exactly one run repaired the parent; the others found its checkpoint.
	var repaired, alreadyCorrect int
	if err = pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE outcome='REPAIRED'),count(*) FILTER (WHERE outcome='ALREADY_CORRECT') FROM commerce_customer_grouping_backfill_results WHERE parent_order_id=$1 AND run_id=ANY($2)`, repairable.ID, runs).Scan(&repaired, &alreadyCorrect); err != nil {
		t.Fatal(err)
	}
	if repaired != 1 || alreadyCorrect != len(runs)-1 {
		t.Fatalf("repaired=%d already_correct=%d across %d runs", repaired, alreadyCorrect, len(runs))
	}
	var groupingEvents int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE event_type=$1 AND aggregate_id=$2`, shared.CommerceGroupingEventName, repairable.ID).Scan(&groupingEvents); err != nil || groupingEvents != 1 {
		t.Fatal("grouping published more than once", groupingEvents, err)
	}
	// The pre-existing checkpoint is untouched and produced no event.
	var version int64
	var updated time.Time
	if err = pool.QueryRow(ctx, `SELECT source_version,updated_at FROM commerce_customer_grouping_emissions WHERE parent_order_id=$1`, already.ID).Scan(&version, &updated); err != nil || version != 7 || updated.Year() != 2020 {
		t.Fatal("already-correct checkpoint modified", version, updated, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE event_type=$1 AND aggregate_id=$2`, shared.CommerceGroupingEventName, already.ID).Scan(&groupingEvents); err != nil || groupingEvents != 0 {
		t.Fatal("already-correct parent republished", groupingEvents, err)
	}
	// Grouping only: no mission created or recreated, no lifecycle advanced.
	if after := snap(); after != before {
		t.Fatalf("operational side effect: before=%+v after=%+v", before, after)
	}
	if got := orderStates(); got != vendorOrderStates {
		t.Fatalf("vendor order lifecycle changed: %s -> %s", vendorOrderStates, got)
	}
	var eligible, rep, correctCount, unrecoverable, failed int
	var completed bool
	for _, run := range runs {
		var e, r, c, u, fl int
		var done bool
		if err = pool.QueryRow(ctx, `SELECT eligible,repaired,already_correct,insufficient_evidence,failed,completed FROM commerce_customer_grouping_backfill_runs WHERE id=$1`, run).Scan(&e, &r, &c, &u, &fl, &done); err != nil {
			t.Fatal(err)
		}
		eligible, rep, correctCount, unrecoverable, failed, completed = eligible+e, rep+r, correctCount+c, unrecoverable+u, failed+fl, done
		if !done {
			t.Fatal("run not completed", run)
		}
	}
	_ = completed
	t.Logf("BACKFILL runs=%d eligible=%d repaired=%d already_correct=%d unrecoverable=%d failed=%d", len(runs), eligible, rep, correctCount, unrecoverable, failed)
}

// Bounded batches with a durable cursor: a fresh publisher (a restarted
// process) resumes exactly where the committed checkpoint stopped, and every
// parent is classified exactly once per run.
func TestHistoricalGroupingBackfillBoundedAndResumable(t *testing.T) {
	f := newSection17EFixture(t, false)
	ctx := context.Background()
	pool := f.env.pool
	for i := 0; i < 120; i++ {
		seedBarePurchase(t, f, "po_bounded_"+uuid.NewString())
	}
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM commerce_parent_orders`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	run := "bounded-" + uuid.NewString()
	first := &customerGroupingPublisher{db: pool, outbox: events.NewOutboxRepository(pool)}
	c, cursor, done, err := first.BackfillBatch(ctx, run, "", false)
	if err != nil || done || c.Eligible+c.AlreadyCorrect+c.InsufficientEvidence != 50 {
		t.Fatalf("first batch not bounded: %+v done=%v err=%v", c, done, err)
	}
	var persisted string
	if err = pool.QueryRow(ctx, `SELECT cursor_ref FROM commerce_customer_grouping_backfill_runs WHERE id=$1`, run).Scan(&persisted); err != nil || persisted != cursor {
		t.Fatal("cursor not checkpointed", persisted, cursor, err)
	}
	// Restart: a new publisher ignores any in-memory cursor and resumes from
	// the durable checkpoint.
	restarted := &customerGroupingPublisher{db: pool, outbox: events.NewOutboxRepository(pool)}
	batches := 1
	for !done {
		if _, _, done, err = restarted.BackfillBatch(ctx, run, "", false); err != nil {
			t.Fatal(err)
		}
		batches++
	}
	var results, distinct int
	if err = pool.QueryRow(ctx, `SELECT count(*),count(DISTINCT parent_order_id) FROM commerce_customer_grouping_backfill_results WHERE run_id=$1`, run).Scan(&results, &distinct); err != nil {
		t.Fatal(err)
	}
	if results != total || distinct != total {
		t.Fatalf("classified %d (%d distinct) of %d parents", results, distinct, total)
	}
	var sum int
	if err = pool.QueryRow(ctx, `SELECT eligible+already_correct+insufficient_evidence FROM commerce_customer_grouping_backfill_runs WHERE id=$1 AND completed`, run).Scan(&sum); err != nil || sum != total {
		t.Fatal("run counts inconsistent", sum, total, err)
	}
	t.Logf("BACKFILL bounded batches=%d parents=%d", batches, total)
}

// seedBarePurchase records an immutable purchase with no operational plan.
func seedBarePurchase(t *testing.T, f section17EFixture, id string) {
	t.Helper()
	ctx := context.Background()
	customer := "customer_" + uuid.NewString()
	suffix := uuid.NewString()
	cart, checkout, plan, quote := "cart_b5_"+suffix, "checkout_b5_"+suffix, "plan_b5_"+suffix, "quote_b5_"+suffix
	fp := strings.Repeat(strings.ReplaceAll(uuid.NewString(), "-", ""), 2)
	now := time.Now().UTC()
	exp := now.Add(time.Hour)
	for _, st := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO commerce_carts(id,customer_id,domain,status,version,expires_at,created_at,updated_at) VALUES($1,$2,'FOOD','CONVERTED',1,$3,$4,$4)`, []any{cart, customer, exp, now}},
		{`INSERT INTO commerce_checkout_sessions(id,customer_id,cart_id,cart_version,domain,status,validation_revision,address_ref,address_snapshotted_at,currency,quoted_items_total_minor,max_authorized_items_total_minor,reservation_ttl_seconds,expires_at,validated_at,created_at,updated_at) VALUES($1,$2,$3,1,'FOOD','VALIDATED',1,'b5-address',$4,'NGN',0,0,900,$5,$4,$4,$4)`, []any{checkout, customer, cart, now, exp}},
		{`INSERT INTO commerce_fulfilment_plans(id,checkout_session_id,customer_id,checkout_validation_revision,status,mode,plan_revision,currency,delivery_gross_minor,delivery_customer_minor,delivery_vendor_funded_minor,delivery_platform_funded_minor,expires_at,version,planned_at,created_at,updated_at) VALUES($1,$2,$3,1,'PLANNED','SINGLE',1,'NGN',0,0,0,0,$4,1,$5,$5,$5)`, []any{plan, checkout, customer, exp, now}},
		{`INSERT INTO commerce_quotes(id,customer_id,checkout_session_id,checkout_validation_revision,cart_id,cart_version,fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,quote_version,material_fingerprint,quote_fingerprint,currency,selected_fulfilment_mode,address_snapshot,expires_at,product_subtotal_minor,max_product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,subsidy_total_minor,final_payable_minor,quoted_total_minor,max_authorized_total_minor,funding_breakdown,price_disclosure_snapshot,created_at) VALUES($1,$2,$3,1,$4,1,$5,1,1,1,$6,$6,'NGN','SINGLE','{}'::jsonb,$7,0,0,0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,$8)`, []any{quote, customer, checkout, cart, plan, fp, exp, now}},
		{`INSERT INTO commerce_parent_orders(id,customer_id,quote_id,quote_version,quote_fingerprint,checkout_session_id,checkout_validation_revision,cart_id,cart_version,fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,domain,currency,selected_fulfilment_mode,address_snapshot,customer_approved_total_minor,max_authorized_total_minor,product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,subsidy_total_minor,funding_breakdown,price_disclosure_snapshot,state,vendor_order_count,created_at,updated_at) VALUES($1,$2,$3,1,$4,$5,1,$6,1,$7,1,1,'FOOD','NGN','SINGLE','{}'::jsonb,0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,'CREATED',1,$8,$8)`, []any{id, customer, quote, fp, checkout, cart, plan, now}},
	} {
		if _, err := f.env.pool.Exec(ctx, st.q, st.args...); err != nil {
			t.Fatalf("seed bare purchase: %v", err)
		}
	}
}
