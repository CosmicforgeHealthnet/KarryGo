package pastry

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

type quoteTestScopes struct{}

func (quoteTestScopes) ResolveScope(context.Context, string, string) (production.Scope, error) {
	return production.Scope{Global: true}, nil
}

func TestConcurrentQuoteAcceptanceCreatesOneSnapshotIntentJobAndReservation(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres quote race")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `TRUNCATE vendor_users,vendor_businesses RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}

	const business, branch, staff = "vendor_quote_race", "branch_quote_race", "staff_quote_race"
	if _, err = pool.Exec(ctx, `
		INSERT INTO vendor_users(id,email) VALUES('user_quote_race','quote-race@example.test');
		INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('staff_quote_race','vendor_quote_race','user_quote_race','active');
		INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone) VALUES('vendor_quote_race','Quote Race Ltd','ACTIVE','staff_quote_race','NGN','Africa/Lagos');
		INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status) VALUES('branch_quote_race','vendor_quote_race','Quote Race Branch','1 Test Road','Lagos','Lagos','NG',6.5,3.4,'Africa/Lagos','ACTIVE');
		INSERT INTO vendor_capabilities(id,business_id,capability_type,status) VALUES('cap_quote_race','vendor_quote_race','PASTRY_BAKERY','ACTIVE');
		INSERT INTO vendor_branch_capabilities(id,business_id,branch_id,vendor_capability_id,capability_type,status) VALUES('branch_cap_quote_race','vendor_quote_race','branch_quote_race','cap_quote_race','PASTRY_BAKERY','ACTIVE');
		INSERT INTO production_capacity_pools(id,business_id,branch_id,capability,name,code,scope_type,unit_type,default_capacity,status) VALUES('pool_quote_race','vendor_quote_race','branch_quote_race','PASTRY_BAKERY','Quote capacity','QUOTE_CAP','CAPABILITY','WORKLOAD_UNIT',10,'ACTIVE')`); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	productionService := production.NewService(production.Options{DB: pool, Scopes: quoteTestScopes{}, Flags: production.Flags{ProductionEnabled: true, WritesEnabled: true}, Now: func() time.Time { return now }})
	service := NewService(pool, productionService)
	requestID, err := service.CreateRequest(ctx, business, CustomRequestInput{BranchID: branch, Title: "Race cake", ChangePolicy: map[string]any{}, CancellationPolicy: map[string]any{}, Logistics: map[string]any{}, RiskFlags: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	staleSpecID, _, err := service.AddSpec(ctx, requestID, map[string]any{"occasion": "Test", "servings": float64(20)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	specID, _, err := service.AddSpec(ctx, requestID, map[string]any{"occasion": "Test", "servings": float64(24)}, 2)
	if err != nil {
		t.Fatal(err)
	}
	actor := production.Actor{StaffID: staff, VendorID: business, CorrelationID: "quote-race"}
	scope := production.Scope{Global: true}
	start := now.Add(30 * 24 * time.Hour)
	if _, _, staleErr := service.CreateQuote(ctx, actor, scope, requestID, staleSpecID, 250000, now.Add(2*time.Hour), QuoteCapacityInput{}, 3); staleErr == nil {
		t.Fatal("quote creation accepted a superseded specification")
	}
	quoteID, _, err := service.CreateQuote(ctx, actor, scope, requestID, specID, 250000, now.Add(2*time.Hour), QuoteCapacityInput{WindowStart: start, WindowEnd: start.Add(2 * time.Hour), Allocations: []production.AllocationRequest{{CapacityPoolID: "pool_quote_race", Quantity: 5}}, HoldFor: 30 * time.Minute}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.ApproveQuote(ctx, actor, scope, quoteID, 1); err != nil {
		t.Fatal(err)
	}

	type result struct {
		intent, reservation string
		err                 error
	}
	results := make(chan result, 2)
	startRace := make(chan struct{})
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-startRace
			intent, reservation, acceptErr := service.AcceptQuote(context.Background(), actor, scope, quoteID, 2)
			results <- result{intent: intent, reservation: reservation, err: acceptErr}
		}()
	}
	close(startRace)
	wait.Wait()
	close(results)
	var first result
	for current := range results {
		if current.err != nil {
			t.Fatalf("concurrent acceptance failed: %v", current.err)
		}
		if first.intent == "" {
			first = current
		} else if current.intent != first.intent || current.reservation != first.reservation {
			t.Fatalf("duplicate acceptance returned different truth: first=%+v second=%+v", first, current)
		}
	}

	for table, where := range map[string]string{
		"pastry_accepted_snapshots":       "quote_id=$1",
		"pastry_order_conversion_intents": "quote_id=$1",
		"production_jobs":                 "source_ref=$1 AND source_type='CUSTOM_CAKE_QUOTE'",
	} {
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE `+where, quoteID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s has %d rows after acceptance race; want 1", table, count)
		}
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM capacity_reservations WHERE id=$1`, first.reservation).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "CONFIRMED" {
		t.Fatalf("accepted reservation status=%s want CONFIRMED", status)
	}

	optionalRequestID, err := service.CreateRequest(ctx, business, CustomRequestInput{BranchID: branch, Title: "Direct issue cake", ChangePolicy: map[string]any{}, CancellationPolicy: map[string]any{}, Logistics: map[string]any{}, RiskFlags: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	optionalSpecID, _, err := service.AddSpec(ctx, optionalRequestID, map[string]any{"occasion": "Direct", "servings": float64(8)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	approvalRequired := false
	optionalQuoteID, _, err := service.CreateQuote(ctx, actor, scope, optionalRequestID, optionalSpecID, 75000, now.Add(3*time.Hour), QuoteCapacityInput{ApprovalRequired: &approvalRequired}, 2)
	if err != nil {
		t.Fatal(err)
	}
	var optionalStatus string
	var storedApprovalRequired bool
	if err = pool.QueryRow(ctx, `SELECT status,approval_required FROM pastry_quotes WHERE id=$1`, optionalQuoteID).Scan(&optionalStatus, &storedApprovalRequired); err != nil {
		t.Fatal(err)
	}
	if optionalStatus != "APPROVED" || storedApprovalRequired {
		t.Fatalf("optional approval quote stored status=%s approval_required=%v", optionalStatus, storedApprovalRequired)
	}
	revisedOptionalQuoteID, err := service.ReviseQuote(ctx, actor, scope, optionalQuoteID, optionalSpecID, 80000, now.Add(4*time.Hour), "PRICE_UPDATE", "Updated price", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status,approval_required FROM pastry_quotes WHERE id=$1`, revisedOptionalQuoteID).Scan(&optionalStatus, &storedApprovalRequired); err != nil {
		t.Fatal(err)
	}
	if optionalStatus != "APPROVED" || storedApprovalRequired {
		t.Fatalf("optional approval was not preserved on revision: status=%s approval_required=%v", optionalStatus, storedApprovalRequired)
	}
}
