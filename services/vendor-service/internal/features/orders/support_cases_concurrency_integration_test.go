package orders

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/supportcase"
)

// Uses one canonical order fixture already present in the explicitly named
// Vendor test database. It never guesses a development database.
func TestSupportCaseLinkRaceReplayAndOrderingPostgres(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set VENDOR_SERVICE_TEST_DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	parent, vendorOrder, customer, business := seedAdministrativeOrder(t, pool)
	store := NewStore(pool)
	op := "support-race-" + uuid.NewString()
	req := supportcase.CreateRequest{SchemaVersion: 1, OperationRef: op, Category: supportcase.DeliveryDelay, Priority: "normal", Subject: "Delivery delay", Description: "Delivery timing dispute", CustomerRef: customer, Context: supportcase.Context{CommerceOrderRef: parent, VendorOrderRef: vendorOrder, VendorRef: business}}
	fp, _ := supportcase.Fingerprint(req)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM commerce_support_resolution_inbox WHERE support_case_ref=$1`, "case-"+op)
		_, _ = pool.Exec(context.Background(), `DELETE FROM commerce_support_case_links WHERE customer_id=$1 AND operation_ref=$2`, customer, op)
	}()
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, e := store.EnsureSupportCaseLink(ctx, req, fp, time.Now())
			if e != nil {
				errs <- e
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	link, err := store.GetSupportCase(ctx, `customer_id=$1 AND operation_ref=$2`, customer, op)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.BindSupportCase(ctx, link.ID, supportcase.Case{CaseRef: "case-" + op, Status: "open"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	event := supportcase.ResolutionEvent{SchemaVersion: 1, EventID: "event-" + op, CaseRef: "case-" + op, OperationRef: op, Category: supportcase.DeliveryDelay, Resolution: "NO_ACTION", ResolutionVersion: 2, CustomerRef: customer, Context: req.Context, ResolvedAt: time.Now()}
	applied, err := store.ApplySupportResolution(ctx, event, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", time.Now())
	if err != nil || !applied {
		t.Fatalf("first apply=%t err=%v", applied, err)
	}
	applied, err = store.ApplySupportResolution(ctx, event, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", time.Now())
	if err != nil || applied {
		t.Fatalf("replay apply=%t err=%v", applied, err)
	}
	stale := event
	stale.EventID = "stale-" + op
	stale.ResolutionVersion = 1
	applied, err = store.ApplySupportResolution(ctx, stale, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", time.Now())
	if err != nil || applied {
		t.Fatalf("stale apply=%t err=%v", applied, err)
	}
}
