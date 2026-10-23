package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/admincommands"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestVendorSecureAdminCapacityRescheduleUsesNativeCapacityFence(t *testing.T) {
	e := newVendorAdminFixture(t)
	ctx := context.Background()
	service := production.NewService(production.Options{DB: e.f.env.pool, Flags: production.Flags{ProductionEnabled: true, WritesEnabled: true}, Events: events.NewDomainPublisher(events.NewOutboxRepository(e.f.env.pool))})
	e.svc.Executor = admincommands.Executor{Production: service}
	pool := "section11-capacity-" + e.f.env.suffix
	e.f.env.exec(ctx, `INSERT INTO production_capacity_pools(id,business_id,branch_id,capability,name,code,unit_type,default_capacity) VALUES($1,$2,$3,'CLOUD_KITCHEN','Test only kitchen','SECTION11','MEAL',10)`, pool, e.f.env.businessA, e.f.env.branchA)
	actor := production.Actor{StaffID: e.actor.Staff, VendorID: e.f.env.businessA, CommandRef: "section11-capacity-test"}
	scope := production.Scope{BranchIDs: []string{e.f.env.branchA}}
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	reserve := func(at time.Time, ref string) production.Reservation {
		t.Helper()
		r, _, err := service.Reserve(ctx, actor, scope, production.ReserveInput{BranchID: e.f.env.branchA, Capability: "CLOUD_KITCHEN", WindowStart: at, WindowEnd: at.Add(time.Hour), SourceType: "EXTERNAL_ORDER", SourceRef: ref, Allocations: []production.AllocationRequest{{CapacityPoolID: pool, Quantity: 6}}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := reserve(start, "first")
	reserve(start.Add(2*time.Hour), "second")
	e.enroll(t, "capacity-reservations", first.ID)
	params := func(at time.Time) map[string]string {
		return map[string]string{"window_start_unix": strconv.FormatInt(at.Unix(), 10), "window_end_unix": strconv.FormatInt(at.Add(time.Hour).Unix(), 10)}
	}
	full := e.cmd("RESCHEDULE_CAPACITY", "capacity-reservations", first.ID, "", 1, params(start.Add(2*time.Hour)))
	if _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, full); err == nil {
		t.Fatal("admin bypassed kitchen capacity")
	}
	var version int64
	var window time.Time
	if err := e.f.env.pool.QueryRow(ctx, `SELECT version,window_start FROM capacity_reservations WHERE id=$1`, first.ID).Scan(&version, &window); err != nil || version != 1 || !window.Equal(start) {
		t.Fatal("failed reschedule changed reservation", version, window, err)
	}
	good := e.cmd("RESCHEDULE_CAPACITY", "capacity-reservations", first.ID, "", 1, params(start.Add(4*time.Hour)))
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, good); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := e.f.env.pool.QueryRow(ctx, `SELECT version,window_start FROM capacity_reservations WHERE id=$1`, first.ID).Scan(&version, &window); err != nil || version != 2 || !window.Equal(start.Add(4*time.Hour)) {
		t.Fatal("reschedule did not apply exactly once", version, window, err)
	}
	var total int64
	if err := e.f.env.pool.QueryRow(ctx, `SELECT sum(quantity) FROM capacity_reservation_allocations WHERE reservation_id=$1`, first.ID).Scan(&total); err != nil || total != 6 {
		t.Fatal("admin invented capacity quantities", total, err)
	}
}
