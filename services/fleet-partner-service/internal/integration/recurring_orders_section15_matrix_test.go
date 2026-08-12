package integration_test

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"testing"
	"time"

	recurringmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/models"
	recurringrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/repositories"
	recurringusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type recurringOrdersStub struct {
	mu      sync.Mutex
	calls   int
	failFor int
	delay   time.Duration
	byKey   map[string]string
}

func (s *recurringOrdersStub) CreateRecurring(_ context.Context, _ tenant.Context,
	in recurringusecases.OrderInput) (string, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.byKey == nil {
		s.byKey = map[string]string{}
	}
	if id := s.byKey[in.SourceReference]; id != "" {
		return id, nil
	}
	if s.calls <= s.failFor {
		return "", errors.New("authoritative order gate refused or transiently failed")
	}
	id := "ord_generated_" + in.SourceReference
	s.byKey[in.SourceReference] = id
	return id, nil
}

type recurringMatrixFixture struct {
	pool    *pgxpool.Pool
	service *recurringusecases.Service
	repo    *recurringrepos.Repository
	tenant  tenant.Context
	orders  *recurringOrdersStub
}

func newRecurringMatrixFixture(t *testing.T, orders *recurringOrdersStub,
	flags recurringusecases.Flags) (*recurringMatrixFixture, *recurringmodels.Template) {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, "Recurring Matrix Co", "RC-RECUR-MATRIX")
	branch := "brn_recurring_matrix"
	if _, err := pool.Exec(context.Background(), `INSERT INTO fleet_partner_branches(
		id,organization_id,code,name,timezone)VALUES($1,$2,'RECUR-MATRIX','Recurring Matrix','Europe/Rome')`,
		branch, org.ID); err != nil {
		t.Fatal(err)
	}
	tc := tenant.Context{OrganizationID: org.ID, OrganizationUserID: "usr_recurring",
		ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_recurring",
		PermittedBranchIDs: []string{branch}}
	repo := recurringrepos.NewRepository(pool)
	service := recurringusecases.NewService(recurringusecases.Options{Pool: pool, Repository: repo,
		Orders: orders, Audit: audit.NewRecorder(), Flags: flags})
	tpl, err := service.Create(context.Background(), tc, recurringusecases.CreateInput{
		BranchID: branch, CustomerID: "cus_recurring", ServiceCatalogItemID: "sci_recurring",
		RecurrenceType: recurringmodels.TypeCustom, RRule: "FREQ=DAILY;INTERVAL=1",
		Timezone: "Europe/Rome", StartDate: time.Now().Add(-time.Hour),
		Data: recurringmodels.TemplateData{CustomerReference: "snapshot-before-edit"}})
	if err != nil {
		t.Fatal(err)
	}
	tpl, err = service.SetActive(context.Background(), tc, tpl.ID, tpl.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(context.Background(), `UPDATE fleet_partner_recurring_order_templates
		SET next_run_at=now()-interval '1 minute' WHERE id=$1`, tpl.ID); err != nil {
		t.Fatal(err)
	}
	return &recurringMatrixFixture{pool: pool, service: service, repo: repo, tenant: tc, orders: orders}, &tpl
}

func TestRecurringOrders_WorkerIdempotencyHeartbeatAndBoundedExpansion(t *testing.T) {
	orders := &recurringOrdersStub{delay: 700 * time.Millisecond}
	flags := recurringusecases.Flags{Enabled: true, WritesEnabled: true, WorkerEnabled: true}
	f, tpl := newRecurringMatrixFixture(t, orders, flags)
	ctx := context.Background()
	result, err := f.service.RunGenerationPass(ctx, recurringusecases.WorkerConfig{
		Owner: "recurring-worker-1", Lease: 300 * time.Millisecond, BatchSize: 1, MaxAttempts: 3})
	if err != nil || result.Generated != 1 || result.Claimed != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if orders.calls != 1 {
		t.Fatalf("orders calls=%d", orders.calls)
	}
	var occurrences int
	// One pass expands exactly one occurrence, even when the template is overdue.
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_recurring_occurrences
		WHERE template_id=$1`, tpl.ID).Scan(&occurrences); err != nil || occurrences != 1 {
		t.Fatalf("occurrences=%d err=%v", occurrences, err)
	}
}

func TestRecurringOrders_LeaseRecoveryDeadLetterReplayAndExistingOrderReadback(t *testing.T) {
	orders := &recurringOrdersStub{failFor: 1}
	flags := recurringusecases.Flags{Enabled: true, WritesEnabled: true, WorkerEnabled: true}
	f, tpl := newRecurringMatrixFixture(t, orders, flags)
	ctx := context.Background()
	result, err := f.service.RunGenerationPass(ctx, recurringusecases.WorkerConfig{
		Owner: "recurring-fail", Lease: time.Second, BatchSize: 1, MaxAttempts: 1})
	if err != nil || result.Failed != 1 {
		t.Fatalf("deadletter result=%+v err=%v", result, err)
	}
	current, err := f.repo.Get(ctx, nil, f.tenant.OrganizationID, tpl.ID)
	if err != nil || current.Status != recurringmodels.StatusInactive {
		t.Fatalf("template=%+v err=%v", current, err)
	}
	if err = f.service.Replay(ctx, f.tenant, tpl.ID); err != nil {
		t.Fatal(err)
	}
	result, err = f.service.RunGenerationPass(ctx, recurringusecases.WorkerConfig{
		Owner: "recurring-replay", Lease: time.Second, BatchSize: 1, MaxAttempts: 3})
	if err != nil || result.Generated != 1 || orders.calls != 2 {
		t.Fatalf("replay result=%+v calls=%d err=%v", result, orders.calls, err)
	}
}

func TestRecurringOrders_FeatureAndKillSwitchDisableWorker(t *testing.T) {
	for name, flags := range map[string]recurringusecases.Flags{
		"worker-disabled": {Enabled: true, WritesEnabled: true},
		"kill-switch":     {Enabled: true, WritesEnabled: true, WorkerEnabled: true, KillSwitch: true},
	} {
		t.Run(name, func(t *testing.T) {
			orders := &recurringOrdersStub{}
			f, _ := newRecurringMatrixFixture(t, orders, recurringusecases.Flags{
				Enabled: true, WritesEnabled: true, WorkerEnabled: true})
			f.service = recurringusecases.NewService(recurringusecases.Options{Repository: f.repo,
				Orders: orders, Flags: flags})
			result, err := f.service.RunGenerationPass(context.Background(), recurringusecases.WorkerConfig{})
			if err != nil || result.Claimed != 0 || orders.calls != 0 {
				t.Fatalf("result=%+v calls=%d err=%v", result, orders.calls, err)
			}
		})
	}
}
