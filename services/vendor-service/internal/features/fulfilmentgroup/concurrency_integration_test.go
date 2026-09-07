package fulfilmentgroup

// Real-Postgres concurrency guards.
//
// These are the invariants that cannot be proven anywhere but against a real
// database, because what enforces them is a constraint and a row lock, not Go
// code. They skip cleanly when no test database is configured, so a run without
// one reports honestly rather than passing on a guarantee it never checked.
//
//	VENDOR_SERVICE_TEST_DATABASE_URL='postgres://...' \
//	  go test ./internal/features/fulfilmentgroup/ -run Integration -v

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
)

// seedCommerceParentOrderEvidence creates the real immutable agreement chain
// required by commerce_parent_orders. Tests must not bypass the quote contract
// with a partial ParentOrder row.
func seedCommerceParentOrderEvidence(t *testing.T, pool *pgxpool.Pool, orderID, customerID string, now time.Time) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin commerce evidence seed: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	suffix := uuid.NewString()
	cartID := "cart_fixture_" + suffix
	checkoutID := "checkout_fixture_" + suffix
	planID := "plan_fixture_" + suffix
	quoteID := "quote_fixture_" + suffix
	fingerprint := strings.Repeat(strings.ReplaceAll(uuid.NewString(), "-", ""), 2)
	expiresAt := now.Add(time.Hour)

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO commerce_carts(id,customer_id,domain,status,version,expires_at,created_at,updated_at)
		  VALUES($1,$2,'FOOD','CONVERTED',1,$3,$4,$4)`, []any{cartID, customerID, expiresAt, now}},
		{`INSERT INTO commerce_checkout_sessions(
			id,customer_id,cart_id,cart_version,domain,status,validation_revision,address_ref,address_snapshotted_at,
			currency,quoted_items_total_minor,max_authorized_items_total_minor,reservation_ttl_seconds,expires_at,validated_at,created_at,updated_at)
		  VALUES($1,$2,$3,1,'FOOD','VALIDATED',1,'fixture-address',$4,'NGN',0,0,900,$5,$4,$4,$4)`, []any{checkoutID, customerID, cartID, now, expiresAt}},
		{`INSERT INTO commerce_fulfilment_plans(
			id,checkout_session_id,customer_id,checkout_validation_revision,status,mode,plan_revision,currency,
			delivery_gross_minor,delivery_customer_minor,delivery_vendor_funded_minor,delivery_platform_funded_minor,
			expires_at,version,planned_at,created_at,updated_at)
		  VALUES($1,$2,$3,1,'PLANNED','SINGLE',1,'NGN',0,0,0,0,$4,1,$5,$5,$5)`, []any{planID, checkoutID, customerID, expiresAt, now}},
		{`INSERT INTO commerce_quotes(
			id,customer_id,checkout_session_id,checkout_validation_revision,cart_id,cart_version,
			fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,quote_version,
			material_fingerprint,quote_fingerprint,currency,selected_fulfilment_mode,address_snapshot,expires_at,
			product_subtotal_minor,max_product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,
			discount_total_minor,subsidy_total_minor,final_payable_minor,quoted_total_minor,max_authorized_total_minor,
			funding_breakdown,price_disclosure_snapshot,created_at)
		  VALUES($1,$2,$3,1,$4,1,$5,1,1,1,$6,$6,'NGN','SINGLE','{}'::jsonb,$7,
			0,0,0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,$8)`, []any{quoteID, customerID, checkoutID, cartID, planID, fingerprint, expiresAt, now}},
		{`INSERT INTO commerce_parent_orders(
			id,customer_id,quote_id,quote_version,quote_fingerprint,checkout_session_id,checkout_validation_revision,
			cart_id,cart_version,fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,domain,currency,
			selected_fulfilment_mode,address_snapshot,customer_approved_total_minor,max_authorized_total_minor,
			product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,
			subsidy_total_minor,funding_breakdown,price_disclosure_snapshot,state,vendor_order_count,created_at,updated_at)
		  VALUES($1,$2,$3,1,$4,$5,1,$6,1,$7,1,1,'FOOD','NGN','SINGLE','{}'::jsonb,
			0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,'CREATED',1,$8,$8)`, []any{orderID, customerID, quoteID, fingerprint, checkoutID, cartID, planID, now}},
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed commerce evidence: %v", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit commerce evidence seed: %v", err)
	}
}

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres fulfilment concurrency tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

// seedGroup creates the minimum order rows the aggregate's foreign keys
// require, then opens a group on them.
func seedGroup(t *testing.T, store *Store) (Group, Unit) {
	t.Helper()

	ctx := context.Background()
	now := time.Now().UTC()

	customerID := "cust_" + uuid.NewString()
	orderID := "po_" + uuid.NewString()
	vendorOrderID := "vo_" + uuid.NewString()
	businessID := "biz_" + uuid.NewString()
	branchID := "br_" + uuid.NewString()

	seedCommerceParentOrderEvidence(t, store.db, orderID, customerID, now)
	if _, err := store.db.Exec(ctx, `
		INSERT INTO commerce_vendor_orders (
			id, parent_order_id, customer_id, business_id, branch_id,
			domain, currency, quote_group_ref, product_subtotal_minor, max_product_subtotal_minor,
			delivery_responsibility, delivery_fee_beneficiary, customer_payable_minor, vendor_payable_minor,
			fulfilment_mode, status, inventory_state, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,'FOOD','NGN',$6,0,0,'NONE','NONE',0,0,'SINGLE','PAID','NOT_REQUIRED',$7,$7)`,
		vendorOrderID, orderID, customerID, businessID, branchID, "quote_group_"+uuid.NewString(), now); err != nil {
		t.Fatalf("seed vendor order: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = store.db.Exec(cleanupCtx,
			`DELETE FROM commerce_vendor_orders WHERE id=$1`, vendorOrderID)
		_, _ = store.db.Exec(cleanupCtx,
			`DELETE FROM commerce_parent_orders WHERE id=$1`, orderID)
	})

	group, err := store.EnsureGroup(ctx, orderID, customerID, now)
	if err != nil {
		t.Fatalf("open group: %v", err)
	}
	unit, err := store.EnsureUnit(ctx, group, UnitInput{VendorOrderID: vendorOrderID}, now)
	if err != nil {
		t.Fatalf("open unit: %v", err)
	}
	return group, unit
}

func slotFor(unitID, slotRef string, sequence int) PlanMission {
	return PlanMission{
		SlotRef:   slotRef,
		Sequence:  sequence,
		Owner:     OwnerPlatform,
		Authority: AuthorityDispatch,
		Currency:  "NGN",
		Stops: []Stop{
			{Sequence: 1, Type: StopPickup, UnitID: unitID},
			{Sequence: 2, Type: StopDropoff, Address: "12 Customer Road"},
		},
	}
}

// Two concurrent replans against the same version must produce exactly one next
// version and one stale answer — never two rows claiming the same version.
func TestIntegration_ConcurrentReplansProduceOneVersion(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	group, unit := seedGroup(t, store)

	ctx := context.Background()
	now := time.Now().UTC()

	initial, err := store.ActivatePlan(ctx, group.ID, PlanInput{
		ExpectedVersion: 0,
		Reason:          ReasonInitialPlan,
		DeliveryMode:    ModePlatform,
		IdempotencyKey:  "initial",
		Missions:        []PlanMission{slotFor(unit.ID, "slot-1", 1)},
	}, now)
	if err != nil {
		t.Fatalf("initial plan: %v", err)
	}
	persistedGroup, err := store.GetGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("read group after initial plan: %v", err)
	}
	if initial.Version != 1 || persistedGroup.CurrentPlanVersion != initial.Version {
		t.Fatalf("initial plan/group versions=%d/%d; want 1/1", initial.Version, persistedGroup.CurrentPlanVersion)
	}

	var wait sync.WaitGroup
	results := make([]error, 2)
	versions := make([]int64, 2)

	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			plan, err := store.ActivatePlan(ctx, group.ID, PlanInput{
				ExpectedVersion: 1,
				Reason:          ReasonCapacityOverflow,
				DeliveryMode:    ModePlatform,
				IdempotencyKey:  "replan-" + uuid.NewString(),
				Missions:        []PlanMission{slotFor(unit.ID, "slot-1", 1)},
			}, time.Now().UTC())
			results[index], versions[index] = err, plan.Version
		}(index)
	}
	wait.Wait()

	winners, stale := 0, 0
	for index, err := range results {
		switch {
		case err == nil:
			winners++
			if versions[index] != 2 {
				t.Errorf("winner produced version %d; want 2", versions[index])
			}
		case errors.Is(err, ErrStalePlan) || errors.Is(err, ErrVersionConflict):
			stale++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if winners != 1 || stale != 1 {
		latestGroup, readErr := store.GetGroup(ctx, group.ID)
		plans, plansErr := store.ListPlans(ctx, group.ID)
		t.Fatalf("winners=%d stale=%d errors=%v current=%d plans=%d read_error=%v plans_error=%v; want exactly one of each",
			winners, stale, results, latestGroup.CurrentPlanVersion, len(plans), readErr, plansErr)
	}

	plans, err := store.ListPlans(ctx, group.ID)
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("plan versions = %d; want 2 (never two rows at the same version)", len(plans))
	}

	// Version 1 must still be readable and still say what it said.
	var active, superseded int
	for _, plan := range plans {
		switch plan.Status {
		case PlanActive:
			active++
		case PlanSuperseded:
			superseded++
			if plan.Version != 1 || plan.Reason != ReasonInitialPlan {
				t.Errorf("history was rewritten: v%d reason=%s", plan.Version, plan.Reason)
			}
		}
	}
	if active != 1 || superseded != 1 {
		t.Fatalf("active=%d superseded=%d; want exactly one in force", active, superseded)
	}
}

// A retried replan reads back its own version rather than appending another.
func TestIntegration_ReplanIsIdempotentOnItsKey(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	group, unit := seedGroup(t, store)

	ctx := context.Background()
	now := time.Now().UTC()

	first, err := store.ActivatePlan(ctx, group.ID, PlanInput{
		ExpectedVersion: 0, Reason: ReasonInitialPlan, DeliveryMode: ModePlatform,
		IdempotencyKey: "activation-1",
		Missions:       []PlanMission{slotFor(unit.ID, "slot-1", 1)},
	}, now)
	if err != nil {
		t.Fatalf("first activation: %v", err)
	}

	second, err := store.ActivatePlan(ctx, group.ID, PlanInput{
		ExpectedVersion: 0, Reason: ReasonInitialPlan, DeliveryMode: ModePlatform,
		IdempotencyKey: "activation-1",
		Missions:       []PlanMission{slotFor(unit.ID, "slot-1", 1)},
	}, now)
	if err != nil {
		t.Fatalf("retried activation: %v", err)
	}

	if first.ID != second.ID || first.Version != second.Version {
		t.Fatalf("retry created a second version: %s v%d then %s v%d",
			first.ID, first.Version, second.ID, second.Version)
	}
}

// Repeated packing with the same operation key must not create a second
// physical parcel.
func TestIntegration_ConcurrentPackingCreatesOneParcel(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	_, unit := seedGroup(t, store)

	ctx := context.Background()

	var wait sync.WaitGroup
	ids := make([]string, 4)
	errs := make([]error, 4)

	for index := range ids {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			parcel, err := store.CreateParcel(ctx, unit.BusinessID, unit.ID, ParcelInput{
				IdempotencyKey: "pack-once",
				WeightGrams:    1_200,
			}, time.Now().UTC())
			ids[index], errs[index] = parcel.ID, err
		}(index)
	}
	wait.Wait()

	distinct := map[string]bool{}
	for index, err := range errs {
		if err != nil {
			// A lock contention loser is acceptable; a second parcel is not.
			continue
		}
		distinct[ids[index]] = true
	}
	if len(distinct) != 1 {
		t.Fatalf("distinct parcels = %d; a retried packing worker must not create a second box", len(distinct))
	}

	parcels, err := store.ListParcelsByUnit(ctx, unit.ID)
	if err != nil {
		t.Fatalf("list parcels: %v", err)
	}
	if len(parcels) != 1 {
		t.Fatalf("parcels in unit = %d; want 1", len(parcels))
	}
}

// A unit may legitimately hold several parcels — one unit is not one parcel.
func TestIntegration_UnitHoldsSeveralParcels(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	_, unit := seedGroup(t, store)

	ctx := context.Background()

	for _, key := range []string{"pack-a", "pack-b", "pack-c"} {
		if _, err := store.CreateParcel(ctx, unit.BusinessID, unit.ID, ParcelInput{
			IdempotencyKey: key,
		}, time.Now().UTC()); err != nil {
			t.Fatalf("pack %s: %v", key, err)
		}
	}

	parcels, err := store.ListParcelsByUnit(ctx, unit.ID)
	if err != nil {
		t.Fatalf("list parcels: %v", err)
	}
	if len(parcels) != 3 {
		t.Fatalf("parcels = %d; want 3 distinct boxes", len(parcels))
	}
	for index, parcel := range parcels {
		if parcel.Sequence != index+1 {
			t.Errorf("parcel %s has sequence %d; want %d", parcel.ID, parcel.Sequence, index+1)
		}
	}
}

// Two workers racing to reserve the same plan slot must produce one mission
// reference, or two vehicles are dispatched for the same goods.
func TestIntegration_ConcurrentMissionReservationYieldsOneRef(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	group, unit := seedGroup(t, store)

	ctx := context.Background()
	plan, err := store.ActivatePlan(ctx, group.ID, PlanInput{
		ExpectedVersion: 0, Reason: ReasonInitialPlan, DeliveryMode: ModePlatform,
		IdempotencyKey: "activation-1",
		Missions:       []PlanMission{slotFor(unit.ID, "slot-1", 1)},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("activate plan: %v", err)
	}
	slotID := plan.Missions[0].ID

	var wait sync.WaitGroup
	refs := make([]string, 3)
	errs := make([]error, 3)

	for index := range refs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			ref, _, err := store.EnsurePlatformMissionRef(
				ctx, group.ID, plan.ID, slotID, AuthorityDispatch, time.Now().UTC())
			refs[index], errs[index] = ref.MissionRef, err
		}(index)
	}
	wait.Wait()

	distinct := map[string]bool{}
	for index, err := range errs {
		if err != nil {
			continue
		}
		distinct[refs[index]] = true
	}
	if len(distinct) != 1 {
		t.Fatalf("distinct mission references = %d; a slot must map to one canonical mission", len(distinct))
	}

	live, err := store.ListMissionRefs(ctx, group.ID)
	if err != nil {
		t.Fatalf("list mission refs: %v", err)
	}
	occupying := 0
	for _, ref := range live {
		if ref.Live() {
			occupying++
		}
	}
	if occupying != 1 {
		t.Fatalf("live mission references = %d; want exactly one per slot", occupying)
	}
}

// Recovery from a lost response: the reservation already exists, so reading the
// canonical mission back and linking it must not create a second mission.
func TestIntegration_UnknownOutcomeRecoveryLinksRatherThanRecreates(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	group, unit := seedGroup(t, store)

	ctx := context.Background()
	plan, err := store.ActivatePlan(ctx, group.ID, PlanInput{
		ExpectedVersion: 0, Reason: ReasonInitialPlan, DeliveryMode: ModePlatform,
		IdempotencyKey: "activation-1",
		Missions:       []PlanMission{slotFor(unit.ID, "slot-1", 1)},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("activate plan: %v", err)
	}

	// The reservation is durable before the authority is called. This is the
	// state a crash mid-call leaves behind.
	reserved, created, err := store.EnsurePlatformMissionRef(
		ctx, group.ID, plan.ID, plan.Missions[0].ID, AuthorityDispatch, time.Now().UTC())
	if err != nil || !created {
		t.Fatalf("reserve: err=%v created=%v", err, created)
	}

	// Recovery re-runs and finds the same reservation rather than making one.
	again, created, err := store.EnsurePlatformMissionRef(
		ctx, group.ID, plan.ID, plan.Missions[0].ID, AuthorityDispatch, time.Now().UTC())
	if err != nil {
		t.Fatalf("recovery reserve: %v", err)
	}
	if created {
		t.Fatal("recovery created a second reservation for the same slot")
	}
	if again.MissionRef != reserved.MissionRef {
		t.Fatal("recovery must resolve the same mission reference")
	}

	// It reads the canonical mission back from the authority and links it.
	linked, err := store.LinkPlatformMission(ctx, reserved.MissionRef, MissionLink{
		ExternalRef: "bk_recovered", Accepted: true,
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if linked.LinkState != LinkLinked || linked.ExternalRef != "bk_recovered" {
		t.Fatalf("link state=%s external=%q", linked.LinkState, linked.ExternalRef)
	}

	// Linking the same answer again is a no-op, not a second mission.
	relinked, err := store.LinkPlatformMission(ctx, reserved.MissionRef, MissionLink{
		ExternalRef: "bk_recovered", Accepted: true,
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("relink: %v", err)
	}
	if relinked.ID != linked.ID {
		t.Fatal("a duplicate answer must resolve to the same projection")
	}

	// A DIFFERENT canonical mission on an already-linked slot is refused: two
	// missions believing they execute the same work is the failure this guard
	// exists for.
	if _, err = store.LinkPlatformMission(ctx, reserved.MissionRef, MissionLink{
		ExternalRef: "bk_other", Accepted: true,
	}, time.Now().UTC()); err != ErrConflict {
		t.Fatalf("err = %v; want ErrConflict for a conflicting canonical mission", err)
	}
}

// A late event must not walk the projection backwards.
func TestIntegration_StaleStatusProjectionIsAbsorbed(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	group, unit := seedGroup(t, store)

	ctx := context.Background()
	plan, err := store.ActivatePlan(ctx, group.ID, PlanInput{
		ExpectedVersion: 0, Reason: ReasonInitialPlan, DeliveryMode: ModePlatform,
		IdempotencyKey: "activation-1",
		Missions:       []PlanMission{slotFor(unit.ID, "slot-1", 1)},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("activate plan: %v", err)
	}
	ref, _, err := store.EnsurePlatformMissionRef(
		ctx, group.ID, plan.ID, plan.Missions[0].ID, AuthorityDispatch, time.Now().UTC())
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}

	if _, applied, err := store.ProjectMissionStatus(
		ctx, ref.MissionRef, "DELIVERED", "evt-9", 9, time.Now().UTC()); err != nil || !applied {
		t.Fatalf("newer status: err=%v applied=%v", err, applied)
	}

	after, applied, err := store.ProjectMissionStatus(
		ctx, ref.MissionRef, "IN_TRANSIT", "evt-4", 4, time.Now().UTC())
	if err != nil {
		t.Fatalf("stale status: %v", err)
	}
	if applied {
		t.Fatal("a delayed older event must not be applied")
	}
	if after.StatusProjection != "DELIVERED" || after.StatusSourceVersion != 9 {
		t.Fatalf("projection regressed to %s/v%d", after.StatusProjection, after.StatusSourceVersion)
	}

	// A redelivery of the same version changes nothing either.
	if _, applied, err = store.ProjectMissionStatus(
		ctx, ref.MissionRef, "DELIVERED", "evt-9", 9, time.Now().UTC()); err != nil || applied {
		t.Fatalf("redelivery: err=%v applied=%v (must be absorbed)", err, applied)
	}
}

// Vendor A must not read, repack or act on Vendor B's parcel, even inside one
// shared commerce order.
func TestIntegration_CrossVendorParcelAccessIsRefused(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	_, unit := seedGroup(t, store)

	ctx := context.Background()
	parcel, err := store.CreateParcel(ctx, unit.BusinessID, unit.ID, ParcelInput{
		IdempotencyKey: "pack-1",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	other := "biz_" + uuid.NewString()

	if _, err = store.GetParcel(ctx, other, parcel.ID); err != ErrNotFound {
		t.Fatalf("read: err = %v; a foreign parcel must be indistinguishable from a missing one", err)
	}
	if _, err = store.UpdateParcelManifest(
		ctx, other, parcel.ID, parcel.Version, Manifest{PackageCount: 1}, time.Now().UTC(),
	); err != ErrNotFound {
		t.Fatalf("amend: err = %v; want ErrNotFound", err)
	}
	if _, err = store.CreateParcel(ctx, other, unit.ID, ParcelInput{
		IdempotencyKey: "pack-foreign",
	}, time.Now().UTC()); err != ErrNotFound {
		t.Fatalf("pack into a foreign unit: err = %v; want ErrNotFound", err)
	}
}

// Custody seals the manifest. After handover, what the parcel contained is
// history and is no longer editable.
func TestIntegration_CustodySealsTheManifest(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	group, unit := seedGroup(t, store)

	ctx := context.Background()
	plan, err := store.ActivatePlan(ctx, group.ID, PlanInput{
		ExpectedVersion: 0, Reason: ReasonInitialPlan, DeliveryMode: ModePlatform,
		IdempotencyKey: "activation-1",
		Missions:       []PlanMission{slotFor(unit.ID, "slot-1", 1)},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("activate plan: %v", err)
	}
	ref, _, err := store.EnsurePlatformMissionRef(
		ctx, group.ID, plan.ID, plan.Missions[0].ID, AuthorityDispatch, time.Now().UTC())
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}

	parcel, err := store.CreateParcel(ctx, unit.BusinessID, unit.ID, ParcelInput{
		IdempotencyKey: "pack-1",
		Manifest:       Manifest{PackageCount: 2},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	// Amending before handover is fine.
	amended, err := store.UpdateParcelManifest(ctx, unit.BusinessID, parcel.ID,
		parcel.Version, Manifest{PackageCount: 3}, time.Now().UTC())
	if err != nil {
		t.Fatalf("pre-custody amend: %v", err)
	}

	handoverEvent := CustodyEvent{
		ParcelID:             parcel.ID,
		GroupID:              parcel.GroupID,
		EventType:            CustodyHandedOver,
		ToCustodianType:      CustodianPlatform,
		Source:               SourceDispatch,
		SourceEventID:        "evt-pickup-1",
		SourceVersion:        1,
		PlatformMissionRefID: ref.ID,
		OccurredAt:           time.Now().UTC(),
	}
	if _, err = store.RecordCustody(ctx, handoverEvent, time.Now().UTC()); err != nil {
		t.Fatalf("record custody: %v", err)
	}

	sealed, err := store.GetParcel(ctx, unit.BusinessID, parcel.ID)
	if err != nil {
		t.Fatalf("read after handover: %v", err)
	}
	if sealed.Status != ParcelInTransit {
		t.Fatalf("status = %s; want IN_TRANSIT", sealed.Status)
	}
	if !sealed.Sealed() {
		t.Fatal("a parcel in a provider's custody must be sealed")
	}
	if sealed.ID != parcel.ID {
		t.Fatal("handover must not change the parcel identity")
	}

	if _, err = store.UpdateParcelManifest(ctx, unit.BusinessID, parcel.ID,
		sealed.Version, Manifest{PackageCount: 99}, time.Now().UTC()); err != ErrCustodySealed {
		t.Fatalf("post-custody amend: err = %v; want ErrCustodySealed", err)
	}

	// The same event again is absorbed, not applied twice.
	if _, err = store.RecordCustody(ctx, handoverEvent, time.Now().UTC()); err != nil {
		t.Fatalf("replayed custody event: %v", err)
	}
	events, err := store.ListCustodyEvents(ctx, parcel.ID)
	if err != nil {
		t.Fatalf("list custody: %v", err)
	}
	handovers := 0
	for _, event := range events {
		if event.EventType == CustodyHandedOver {
			handovers++
		}
	}
	if handovers != 1 {
		t.Fatalf("handover links = %d; an at-least-once replay must not duplicate the chain", handovers)
	}

	_ = amended
}

// A return keeps the parcel and links a new mission to carry it back.
func TestIntegration_ReturnKeepsTheSameParcel(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	group, unit := seedGroup(t, store)

	ctx := context.Background()
	plan, err := store.ActivatePlan(ctx, group.ID, PlanInput{
		ExpectedVersion: 0, Reason: ReasonInitialPlan, DeliveryMode: ModePlatform,
		IdempotencyKey: "activation-1",
		Missions:       []PlanMission{slotFor(unit.ID, "slot-1", 1)},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("activate plan: %v", err)
	}
	ref, _, err := store.EnsurePlatformMissionRef(
		ctx, group.ID, plan.ID, plan.Missions[0].ID, AuthorityDispatch, time.Now().UTC())
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}

	parcel, err := store.CreateParcel(ctx, unit.BusinessID, unit.ID, ParcelInput{
		IdempotencyKey: "pack-1",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	if _, err = store.RecordCustody(ctx, CustodyEvent{
		ParcelID: parcel.ID, GroupID: parcel.GroupID,
		EventType: CustodyHandedOver, ToCustodianType: CustodianPlatform,
		Source: SourceDispatch, SourceEventID: "evt-1", SourceVersion: 1,
		PlatformMissionRefID: ref.ID, OccurredAt: time.Now().UTC(),
	}, time.Now().UTC()); err != nil {
		t.Fatalf("handover: %v", err)
	}

	parcelReturn, err := store.InitiateReturn(
		ctx, parcel.ID, "RECIPIENT_UNAVAILABLE", "return-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("initiate return: %v", err)
	}
	if parcelReturn.ParcelID != parcel.ID {
		t.Fatal("a return must name the parcel that went out")
	}

	// Retrying the same return is the same return.
	again, err := store.InitiateReturn(
		ctx, parcel.ID, "RECIPIENT_UNAVAILABLE", "return-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("retried return: %v", err)
	}
	if again.ID != parcelReturn.ID {
		t.Fatal("a retried return must not open a second one")
	}

	// The return leg may be a brand-new mission. The parcel is not new.
	replacement, err := store.ReplacePlatformMission(
		ctx, ref.MissionRef, "return leg", time.Now().UTC())
	if err != nil {
		t.Fatalf("replace mission: %v", err)
	}
	linked, err := store.LinkReturnMission(
		ctx, parcelReturn.ID, replacement.ID, "", time.Now().UTC())
	if err != nil {
		t.Fatalf("link return mission: %v", err)
	}
	if linked.ReturnPlatformMissionRefID != replacement.ID {
		t.Fatal("the return leg must be recorded")
	}
	if linked.ParcelID != parcel.ID {
		t.Fatal("the return still names the same parcel")
	}

	// The replaced mission stays readable: it really did happen.
	refs, err := store.ListMissionRefs(ctx, group.ID)
	if err != nil {
		t.Fatalf("list mission refs: %v", err)
	}
	var foundReplaced bool
	for _, item := range refs {
		if item.ID == ref.ID && item.LinkState == LinkReplaced {
			foundReplaced = true
		}
	}
	if !foundReplaced {
		t.Fatal("a replaced mission must remain in the history")
	}
}
