package supportrepositories

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

func TestCommerceCaseCreateRaceAndResolutionOrderingPostgres(t *testing.T) {
	dsn := os.Getenv("SUPPORT_DISPUTE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SUPPORT_DISPUTE_TEST_DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := NewPostgresSupportRepository(pool)
	suffix := uuid.NewString()
	request := supportcase.CreateRequest{SchemaVersion: 1, OperationRef: "case-race-" + suffix, Category: supportcase.QualityIssue, Priority: "normal", Subject: "Quality issue", Description: "Concurrent quality report", CustomerRef: "customer-" + suffix, Context: supportcase.Context{CommerceOrderRef: "order-" + suffix, VendorOrderRef: "vendor-order-" + suffix, VendorRef: "vendor-" + suffix}}
	fingerprint, _ := supportcase.Fingerprint(request)
	const workers = 8
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, _, e := repo.CreateCommerceCase(ctx, request, fingerprint)
			if e != nil {
				errs <- e
				return
			}
			ids <- value.CaseRef
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	caseRef := ""
	for id := range ids {
		if caseRef == "" {
			caseRef = id
		}
		if id != caseRef {
			t.Fatalf("race created different cases %q and %q", caseRef, id)
		}
	}
	if caseRef == "" {
		t.Fatal("no case created")
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM support_case_outbox WHERE aggregate_ref=$1`, caseRef)
		_, _ = pool.Exec(context.Background(), `DELETE FROM complaint_events WHERE complaint_id=$1`, caseRef)
		_, _ = pool.Exec(context.Background(), `DELETE FROM complaints WHERE id=$1`, caseRef)
	}()
	changed := request
	changed.Category = supportcase.MissingItem
	other, _ := supportcase.Fingerprint(changed)
	if _, _, err = repo.CreateCommerceCase(ctx, changed, other); err == nil {
		t.Fatal("fingerprint conflict accepted")
	}
	first, err := repo.ResolveCommerceCase(ctx, caseRef, "NO_ACTION", "", "tester", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.ResolveCommerceCase(ctx, caseRef, "CUSTOMER_UPHELD", "", "tester", time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if first.ResolutionVersion != 1 || second.ResolutionVersion != 2 {
		t.Fatalf("resolution versions=%d,%d", first.ResolutionVersion, second.ResolutionVersion)
	}
}
