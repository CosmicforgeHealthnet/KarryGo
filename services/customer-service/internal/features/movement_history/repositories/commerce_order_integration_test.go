package movementrepositories_test

// Customer V4 §22.5 against real PostgreSQL, for the VENDOR_COMMERCE purchase
// row and the execution rows that join it through parent_context_ref.

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	movementmodels "cosmicforge/logistics/services/customer-service/internal/features/movement_history/models"
	movementrepositories "cosmicforge/logistics/services/customer-service/internal/features/movement_history/repositories"
	"cosmicforge/logistics/shared/go/events"
)

func projectAs(t *testing.T, repo *movementrepositories.PostgresRepository, eventID string, version int64, s events.MovementSummary) string {
	t.Helper()
	envelope, err := events.NewMovementEnvelope(eventID, events.EventNameFor(s.Authority, s.State), "vendor-service",
		"commerce_parent_order", version, time.Now(), "", "", s)
	if err != nil {
		t.Fatal(err)
	}
	movement, err := movementmodels.FromEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := repo.Project(context.Background(), envelope, movement)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

func purchase(customer, parent, state, stage string) events.MovementSummary {
	return events.MovementSummary{CustomerID: customer, Authority: events.MovementAuthorityVendorCommerce, SourceRef: parent,
		ProductCode: "FOOD", MovementType: events.MovementTypeCommerceOrder, State: state, Stage: stage,
		ParentContextType: "COMMERCE_ORDER", ParentContextRef: parent}
}

func TestPostgresCommerceOrderReplayOutOfOrderAndReconciliation(t *testing.T) {
	db := pool(t)
	repo := movementrepositories.NewPostgresRepository(db)
	ctx := context.Background()
	customer, parent := uuid.NewString(), "po_"+uuid.NewString()
	prefix := uuid.NewString()

	// v5 applied, then the older v4 arrives: the projection stays at v5.
	if got := projectAs(t, repo, prefix+"-5", 5, purchase(customer, parent, events.MovementStateInProgress, "")); got != movementmodels.OutcomeApplied {
		t.Fatalf("v5 = %s", got)
	}
	if got := projectAs(t, repo, prefix+"-4", 4, purchase(customer, parent, events.MovementStateRequested, "")); got != movementmodels.OutcomeStale {
		t.Fatalf("v4 after v5 = %s", got)
	}
	// v5 delivered twice: one effect.
	if got := projectAs(t, repo, prefix+"-5", 5, purchase(customer, parent, events.MovementStateInProgress, "")); got != movementmodels.OutcomeDuplicate {
		t.Fatalf("v5 replay = %s", got)
	}
	row, err := repo.Get(ctx, customer, events.MovementAuthorityVendorCommerce, parent)
	if err != nil || row.SourceVersion != 5 || row.State != events.MovementStateInProgress {
		t.Fatalf("after replay row=%+v err=%v", row, err)
	}

	// v6 applied, v7 lost in transit. The authority's reconciliation read
	// shows the projection behind, republishes current (v7) and it converges.
	projectAs(t, repo, prefix+"-6", 6, purchase(customer, parent, events.MovementStateInProgress, events.MovementStageRefundPending))
	versions, err := repo.ProjectedVersions(ctx, events.MovementAuthorityVendorCommerce, []string{parent})
	if err != nil || versions[parent] != 6 {
		t.Fatalf("projected=%v err=%v", versions, err)
	}
	authorityVersion := int64(7)
	if versions[parent] < authorityVersion {
		projectAs(t, repo, prefix+"-7r", authorityVersion, purchase(customer, parent, events.MovementStateCancelled, events.MovementStageRefunded))
	}
	row, _ = repo.Get(ctx, customer, events.MovementAuthorityVendorCommerce, parent)
	if row.SourceVersion != 7 || row.State != events.MovementStateCancelled {
		t.Fatalf("reconciled row=%+v", row)
	}

	// The projection row is removed (operator repair / corruption). Only the
	// Customer projection is touched; the next reconciliation sees it missing
	// and the authority's current state rebuilds it.
	if _, err = db.Exec(ctx, `DELETE FROM customer_movement_history_projection WHERE authority=$1 AND source_ref=$2`,
		events.MovementAuthorityVendorCommerce, parent); err != nil {
		t.Fatal(err)
	}
	versions, _ = repo.ProjectedVersions(ctx, events.MovementAuthorityVendorCommerce, []string{parent})
	if versions[parent] != 0 {
		t.Fatalf("deleted row still projected: %v", versions)
	}
	if got := projectAs(t, repo, prefix+"-7b", 7, purchase(customer, parent, events.MovementStateCancelled, events.MovementStageRefunded)); got != movementmodels.OutcomeApplied {
		t.Fatalf("rebuild = %s", got)
	}
	row, _ = repo.Get(ctx, customer, events.MovementAuthorityVendorCommerce, parent)
	if row.SourceVersion != 7 || row.State != events.MovementStateCancelled {
		t.Fatalf("rebuilt row=%+v", row)
	}
}

// Concurrent delivery in random order never loses the newest version.
func TestPostgresCommerceOrderConcurrentDeliveryKeepsNewest(t *testing.T) {
	db := pool(t)
	repo := movementrepositories.NewPostgresRepository(db)
	customer, parent := uuid.NewString(), "po_"+uuid.NewString()
	versions := rand.Perm(20)
	var wg sync.WaitGroup
	errs := make(chan error, len(versions))
	for _, v := range versions {
		v := int64(v + 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := purchase(customer, parent, events.MovementStateInProgress, "")
			envelope, err := events.NewMovementEnvelope(fmt.Sprintf("%s-%d", parent, v), "commerce.customer_order.started",
				"vendor-service", "commerce_parent_order", v, time.Now(), "", "", s)
			if err == nil {
				var m movementmodels.Movement
				if m, err = movementmodels.FromEnvelope(envelope); err == nil {
					_, err = repo.Project(context.Background(), envelope, m)
				}
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	row, err := repo.Get(context.Background(), customer, events.MovementAuthorityVendorCommerce, parent)
	if err != nil || row.SourceVersion != 20 {
		t.Fatalf("row=%+v err=%v, want version 20", row, err)
	}
}

// A purchase row and its execution missions list together for the product
// and group together by parent_context_ref; a replaced mission keeps its own
// row and never replaces the purchase.
func TestPostgresCommerceOrderGroupsItsMissions(t *testing.T) {
	db := pool(t)
	repo := movementrepositories.NewPostgresRepository(db)
	ctx := context.Background()
	customer, parent := uuid.NewString(), "po_"+uuid.NewString()
	projectAs(t, repo, uuid.NewString(), 3, purchase(customer, parent, events.MovementStateInProgress, ""))
	for i, authority := range []string{events.MovementAuthorityDispatch, events.MovementAuthorityTaxi} {
		mission := events.MovementSummary{CustomerID: customer, Authority: authority, SourceRef: fmt.Sprintf("mission-%d-%s", i, parent),
			ProductCode: "FOOD", MovementType: events.MovementTypeGoods, State: events.MovementStateInProgress,
			Stage: events.MovementStageInTransit, ParentContextType: "COMMERCE_ORDER", ParentContextRef: parent}
		if authority == events.MovementAuthorityDispatch {
			mission.MovementType = events.MovementTypeDispatch
			mission.State, mission.Stage = events.MovementStateCancelled, events.MovementStageCancelled
		}
		projectAs(t, repo, uuid.NewString(), 1, mission)
	}
	rows, total, err := repo.List(ctx, customer, movementrepositories.Filter{ParentContextRef: parent, Page: 1, Limit: 10})
	if err != nil || total != 3 || len(rows) != 3 {
		t.Fatalf("grouped rows=%d total=%d err=%v", len(rows), total, err)
	}
	food, _, err := repo.List(ctx, customer, movementrepositories.Filter{ProductCode: "FOOD", Page: 1, Limit: 10})
	if err != nil || len(food) != 3 {
		t.Fatalf("FOOD filter rows=%d err=%v", len(food), err)
	}
	// A purchase row is refused for anything but FOOD/SHOP.
	bad := purchase(customer, "po_"+uuid.NewString(), events.MovementStateInProgress, "")
	bad.ProductCode = "RIDE"
	if _, err := events.NewMovementEnvelope(uuid.NewString(), "commerce.customer_order.started", "vendor-service", "x", 1, time.Now(), "", "", bad); err == nil {
		t.Fatal("a non-commerce product was accepted on a purchase row")
	}
}
