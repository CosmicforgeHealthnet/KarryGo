package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	fleetevents "cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	approvalrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	approvalusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	crmhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/http"
	crmmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	crmrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	crmusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	marketusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	ordershttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/http"
	ordersmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	ordersrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privaterepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/repositories"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	catalogrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/repositories"
	shipmentsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	shipmentsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// FLEET-14.10 / 15.3 / 15.10 / 15.11 against real PostgreSQL.

func codeString(err error) string {
	if err == nil {
		return ""
	}
	return string(apperrors.From(err).Code)
}

func countWhere(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// ---------------------------------------------------------------------------
// FLEET-14.10 Sales leads
// ---------------------------------------------------------------------------

type leadsFixture struct {
	pool   *pgxpool.Pool
	crm    *crmusecases.Service
	tenant tenant.Context
	other  tenant.Context
	member string
	alien  string
}

func newLeadsFixture(t *testing.T) *leadsFixture {
	t.Helper()
	pool := testPool(t)
	primary := createOrganization(t, pool, "Leads Primary Fleet", "RC-LED-1")
	secondary := createOrganization(t, pool, "Leads Secondary Fleet", "RC-LED-2")
	service := crmusecases.NewService(crmusecases.Options{
		Pool: pool, Repository: crmrepos.NewRepository(pool), Metrics: observability.NewMetrics(),
		Flags:       allCRMFlagsOn(),
		Events:      fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(pool)),
		TokenSecret: []byte("leads-token-secret"), ContactSecret: []byte("leads-contact-secret"),
		TokenTTL: 48 * time.Hour,
	})
	member := seedApprovalMember(t, pool, primary.ID, "", "lead-owner")
	alien := seedApprovalMember(t, pool, secondary.ID, "", "alien-owner")
	return &leadsFixture{
		pool: pool, crm: service, member: member, alien: alien,
		tenant: tenant.Context{OrganizationID: primary.ID, OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member, CorrelationID: "corr-leads"},
		other: tenant.Context{OrganizationID: secondary.ID, OrganizationUserID: alien,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: alien},
	}
}

func TestLeads_CaptureQualifyAssignAndCrossTenant(t *testing.T) {
	f := newLeadsFixture(t)
	ctx := context.Background()
	score := 40

	if _, err := f.crm.CreateLead(ctx, f.tenant, crmusecases.CreateLeadInput{
		Source: "WEBSITE", CompanyName: "   ", IdempotencyKey: "lead-blank"}); codeString(err) != "lead_invalid_transition" {
		t.Fatalf("blank company must be refused, got %v", err)
	}
	bad := 101
	if _, err := f.crm.CreateLead(ctx, f.tenant, crmusecases.CreateLeadInput{
		CompanyName: "Bad Score", Score: &bad, IdempotencyKey: "lead-badscore"}); err == nil {
		t.Fatal("score above 100 must be refused")
	}
	if _, err := f.crm.CreateLead(ctx, f.tenant, crmusecases.CreateLeadInput{
		CompanyName: "No Key"}); err == nil {
		t.Fatal("missing Idempotency-Key must be refused")
	}

	input := crmusecases.CreateLeadInput{Source: "referral", CompanyName: "Prospect Haulage Ltd",
		ContactSnapshot: map[string]any{"name": "Ngozi", "email": "ngozi@example.invalid"},
		Score:           &score, IdempotencyKey: "lead-create-1"}
	lead, err := f.crm.CreateLead(ctx, f.tenant, input)
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}
	if lead.Status != crmmodels.LeadNew || lead.Source != crmmodels.LeadSourceReferral {
		t.Fatalf("lead = %+v", lead)
	}
	replay, err := f.crm.CreateLead(ctx, f.tenant, input)
	if err != nil || replay.ID != lead.ID {
		t.Fatalf("identical retry must replay the same lead: %v %s", err, replay.ID)
	}
	changed := input
	changed.CompanyName = "Different Prospect"
	if _, err = f.crm.CreateLead(ctx, f.tenant, changed); codeString(err) != "lead_invalid_transition" {
		t.Fatalf("same key with a different body must conflict, got %v", err)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_sales_leads WHERE organization_id=$1`,
		f.tenant.OrganizationID); n != 1 {
		t.Fatalf("leads=%d want 1", n)
	}

	// Cross-tenant: read, assign and convert are all not-found.
	if _, err = f.crm.GetLead(ctx, f.other, lead.ID); codeString(err) != "lead_not_found" {
		t.Fatalf("cross-tenant read = %v", err)
	}
	if _, err = f.crm.AssignLead(ctx, f.other, lead.ID, crmusecases.AssignLeadInput{
		OwnerUserID: f.alien, ExpectedVersion: lead.Version, IdempotencyKey: "x"}); codeString(err) != "lead_not_found" {
		t.Fatalf("cross-tenant assign = %v", err)
	}

	// Assignment only to staff of the same tenant.
	if _, err = f.crm.AssignLead(ctx, f.tenant, lead.ID, crmusecases.AssignLeadInput{
		OwnerUserID: f.alien, ExpectedVersion: lead.Version, IdempotencyKey: "assign-alien"}); err == nil {
		t.Fatal("assigning a lead to another tenant's user must be refused")
	}
	assigned, err := f.crm.AssignLead(ctx, f.tenant, lead.ID, crmusecases.AssignLeadInput{
		OwnerUserID: f.member, ExpectedVersion: lead.Version, IdempotencyKey: "assign-1"})
	if err != nil || assigned.OwnerUserID != f.member {
		t.Fatalf("assign: %v %+v", err, assigned)
	}

	// Qualification: stale version conflicts; CONVERTED is not a status move.
	if _, err = f.crm.ChangeLeadStatus(ctx, f.tenant, lead.ID, crmusecases.ChangeLeadStatusInput{
		Status: "QUALIFIED", ExpectedVersion: lead.Version, IdempotencyKey: "q-stale"}); err == nil {
		t.Fatal("stale version must conflict")
	}
	if _, err = f.crm.ChangeLeadStatus(ctx, f.tenant, lead.ID, crmusecases.ChangeLeadStatusInput{
		Status: "CONVERTED", ExpectedVersion: assigned.Version, IdempotencyKey: "q-conv"}); err == nil {
		t.Fatal("CONVERTED must only be reachable through convert")
	}
	qualified, err := f.crm.ChangeLeadStatus(ctx, f.tenant, lead.ID, crmusecases.ChangeLeadStatusInput{
		Status: "QUALIFIED", ExpectedVersion: assigned.Version, IdempotencyKey: "q-1"})
	if err != nil || qualified.Status != crmmodels.LeadQualified {
		t.Fatalf("qualify: %v %+v", err, qualified)
	}

	// A disqualified lead is terminal and cannot be converted.
	other, err := f.crm.CreateLead(ctx, f.tenant, crmusecases.CreateLeadInput{
		CompanyName: "Not A Fit Ltd", IdempotencyKey: "lead-create-2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.crm.ChangeLeadStatus(ctx, f.tenant, other.ID, crmusecases.ChangeLeadStatusInput{
		Status: "DISQUALIFIED", ExpectedVersion: other.Version, IdempotencyKey: "dq-noreason"}); err == nil {
		t.Fatal("disqualification without a reason must be refused")
	}
	dq, err := f.crm.ChangeLeadStatus(ctx, f.tenant, other.ID, crmusecases.ChangeLeadStatusInput{
		Status: "DISQUALIFIED", ReasonCode: "NO_BUDGET", ExpectedVersion: other.Version, IdempotencyKey: "dq-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.crm.ConvertLead(ctx, f.tenant, other.ID, crmusecases.ConvertLeadInput{
		NewCustomer:     &crmusecases.CreateCustomerInput{CustomerType: crmmodels.CustomerTypeBusiness, DefaultCurrency: "NGN"},
		ExpectedVersion: dq.Version, IdempotencyKey: "conv-dq"}); codeString(err) != "lead_invalid_transition" {
		t.Fatalf("converting a disqualified lead = %v", err)
	}

	history, err := f.crm.LeadHistory(ctx, f.tenant, lead.ID, cursor.Cursor{}, false, 50)
	if err != nil || len(history) != 3 {
		t.Fatalf("history=%d err=%v", len(history), err)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1
		AND event_type='organization.sales_lead.created.v1'`, lead.ID); n != 1 {
		t.Fatalf("lead.created events=%d", n)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE resource_id=$1
		AND action IN ('crm.lead.created','crm.lead.assigned','crm.lead.status_changed')`, lead.ID); n != 3 {
		t.Fatalf("lead audit rows=%d", n)
	}
	// The contact snapshot never leaves through the outbox.
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1
		AND payload::text LIKE '%ngozi%'`, lead.ID); n != 0 {
		t.Fatal("lead contact snapshot leaked into the outbox")
	}
}

func TestLeads_ConvertExactlyOnceUnderConcurrency(t *testing.T) {
	f := newLeadsFixture(t)
	ctx := context.Background()
	lead, err := f.crm.CreateLead(ctx, f.tenant, crmusecases.CreateLeadInput{
		CompanyName: "Concurrent Prospect Ltd", OwnerUserID: f.member, IdempotencyKey: "lead-conc"})
	if err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		result crmusecases.LeadConversion
		err    error
		key    string
	}
	results := make([]outcome, 4)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "convert-" + string(rune('a'+i))
			res, convErr := f.crm.ConvertLead(ctx, f.tenant, lead.ID, crmusecases.ConvertLeadInput{
				NewCustomer: &crmusecases.CreateCustomerInput{CustomerType: crmmodels.CustomerTypeBusiness,
					DefaultCurrency: "NGN", RegistrationNumber: "RC-CONC", MarketplaceCustomerID: "mkt_should_be_ignored"},
				Opportunity:     &crmusecases.CreateOpportunityInput{Title: "Regional haulage", ProbabilityBasisPoints: 3000},
				ExpectedVersion: lead.Version, IdempotencyKey: key})
			results[i] = outcome{result: res, err: convErr, key: key}
		}(i)
	}
	wg.Wait()

	winners := 0
	var winner outcome
	for _, r := range results {
		if r.err == nil {
			winners++
			winner = r
			continue
		}
		if code := codeString(r.err); code != "lead_already_converted" && code != "lead_invalid_transition" {
			t.Fatalf("loser error = %v", r.err)
		}
	}
	if winners != 1 {
		t.Fatalf("exactly one conversion must win, got %d", winners)
	}
	if !winner.result.CustomerCreated || winner.result.Opportunity == nil ||
		winner.result.Customer.LegalName != "Concurrent Prospect Ltd" {
		t.Fatalf("winner = %+v", winner.result)
	}
	if winner.result.Customer.MarketplaceCustomerID != "" {
		t.Fatal("a converted company customer must never be linked to a marketplace identity")
	}
	if winner.result.Customer.AccountOwnerUserID != f.member || winner.result.Opportunity.CustomerID != winner.result.Customer.ID {
		t.Fatalf("lead ownership/customer link not carried: %+v", winner.result)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_company_customers
		WHERE organization_id=$1 AND legal_name='Concurrent Prospect Ltd'`, f.tenant.OrganizationID); n != 1 {
		t.Fatalf("customers created=%d", n)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_sales_opportunities
		WHERE organization_id=$1`, f.tenant.OrganizationID); n != 1 {
		t.Fatalf("opportunities created=%d", n)
	}

	// The winner's retry replays the same outcome; a fresh key is refused.
	replay, err := f.crm.ConvertLead(ctx, f.tenant, lead.ID, crmusecases.ConvertLeadInput{
		NewCustomer:    &crmusecases.CreateCustomerInput{CustomerType: crmmodels.CustomerTypeBusiness, DefaultCurrency: "NGN"},
		IdempotencyKey: winner.key})
	if err != nil || !replay.Replayed || replay.Customer.ID != winner.result.Customer.ID ||
		replay.Opportunity == nil || replay.Opportunity.ID != winner.result.Opportunity.ID {
		t.Fatalf("replay = %+v err=%v", replay, err)
	}
	if _, err = f.crm.ConvertLead(ctx, f.tenant, lead.ID, crmusecases.ConvertLeadInput{
		ExistingCustomerID: winner.result.Customer.ID, ExpectedVersion: replay.Lead.Version,
		IdempotencyKey: "convert-late"}); codeString(err) != "lead_already_converted" {
		t.Fatalf("second conversion = %v", err)
	}
	stored, err := f.crm.GetLead(ctx, f.tenant, lead.ID)
	if err != nil || stored.Status != crmmodels.LeadConverted || stored.ConvertedCustomerID != winner.result.Customer.ID {
		t.Fatalf("stored lead = %+v err=%v", stored, err)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1
		AND event_type='organization.sales_lead.converted.v1'`, lead.ID); n != 1 {
		t.Fatalf("lead.converted events=%d", n)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE resource_id=$1
		AND action='crm.lead.converted'`, lead.ID); n != 1 {
		t.Fatalf("conversion audit rows=%d", n)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_sales_lead_history WHERE lead_id=$1
		AND change_type='CONVERTED'`, lead.ID); n != 1 {
		t.Fatalf("conversion history rows=%d", n)
	}

	// Mapping onto an existing customer, and never onto another tenant's.
	second, err := f.crm.CreateLead(ctx, f.tenant, crmusecases.CreateLeadInput{
		CompanyName: "Existing Mapping Ltd", IdempotencyKey: "lead-map"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.crm.CreateCustomer(ctx, f.other, crmusecases.CreateCustomerInput{
		CustomerType: crmmodels.CustomerTypeBusiness, LegalName: "Foreign Co", DefaultCurrency: "NGN",
		IdempotencyKey: "foreign"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.crm.ConvertLead(ctx, f.tenant, second.ID, crmusecases.ConvertLeadInput{
		ExistingCustomerID: foreign.ID, ExpectedVersion: second.Version, IdempotencyKey: "map-foreign"}); err == nil ||
		apperrors.From(err).Status != 404 {
		t.Fatalf("mapping onto another tenant's customer must be not-found, got %v", err)
	}
	mapped, err := f.crm.ConvertLead(ctx, f.tenant, second.ID, crmusecases.ConvertLeadInput{
		ExistingCustomerID: winner.result.Customer.ID, ExpectedVersion: second.Version, IdempotencyKey: "map-1"})
	if err != nil || mapped.CustomerCreated || mapped.Customer.ID != winner.result.Customer.ID || mapped.Opportunity != nil {
		t.Fatalf("map existing = %+v err=%v", mapped, err)
	}
}

// ---------------------------------------------------------------------------
// FLEET-15.3 / 15.11 PATCH, submit, approve, reject
// ---------------------------------------------------------------------------

func draftOrderWithStops(t *testing.T, orders *ordersusecases.Service, tc tenant.Context, customerID,
	itemID string) ordersmodels.Order {
	return draftBranchOrderWithStops(t, orders, tc, "", customerID, itemID)
}

func draftBranchOrderWithStops(t *testing.T, orders *ordersusecases.Service, tc tenant.Context, branchID,
	customerID, itemID string) ordersmodels.Order {
	t.Helper()
	ctx := context.Background()
	order, err := orders.Create(ctx, tc, ordersusecases.CreateInput{
		BranchID: branchID, CustomerID: customerID, ServiceCatalogItemID: itemID, Timezone: "Africa/Lagos",
		ServiceFields: ordersmodels.JSONMap{"receiverName": "Ada"},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	lat1, lng1, lat2, lng2 := 6.45, 3.39, 6.60, 3.35
	if _, err = orders.AddStop(ctx, tc, order.ID, ordersusecases.AddStopInput{
		StopType: ordersmodels.StopTypePickup, Timezone: "Africa/Lagos", Latitude: &lat1, Longitude: &lng1,
		AddressSnapshot: ordersmodels.JSONMap{"formattedAddress": "1 Marina, Lagos"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = orders.AddStop(ctx, tc, order.ID, ordersusecases.AddStopInput{
		StopType: ordersmodels.StopTypeDelivery, Timezone: "Africa/Lagos", Latitude: &lat2, Longitude: &lng2,
		AddressSnapshot: ordersmodels.JSONMap{"formattedAddress": "2 Ikeja Way, Lagos"}}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := orders.Get(ctx, tc, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	return reloaded
}

func TestOrders_PatchSubmitApproveRejectLifecycle(t *testing.T) {
	f := newOrdersFixture(t)
	ctx := context.Background()
	publisher := fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(f.pool))
	orders := ordersusecases.NewService(ordersusecases.Options{
		Pool: f.pool, Repository: f.ordersRepo,
		Catalog: app.NewOrdersCatalogAdapter(catalogrepos.NewRepository(f.pool)),
		CRM:     app.NewOrdersCRMAdapter(crmrepos.NewRepository(f.pool)),
		Events:  publisher, Metrics: observability.NewMetrics(), Flags: allOrdersFlagsOn(),
	})
	customer := f.customer(t, "Lifecycle Approval Co")
	item := f.activeCatalogItem(t, "APPROVAL_ITEM")
	order := draftOrderWithStops(t, orders, f.tenant, customer.ID, item.ID)
	approver := f.tenant
	approver.ActorID, approver.OrganizationUserID = "usr_orders_approver", "usr_orders_approver"

	// PATCH in DRAFT, version-checked; identity fields stay put.
	priority, instructions := ordersmodels.PriorityUrgent, "Call before arrival"
	if _, err := orders.Update(ctx, f.tenant, order.ID, ordersusecases.UpdateInput{
		Priority: &priority, ExpectedVersion: order.Version + 5}); err == nil {
		t.Fatal("stale PATCH must conflict")
	}
	badTZ := "Mars/Olympus"
	if _, err := orders.Update(ctx, f.tenant, order.ID, ordersusecases.UpdateInput{
		Timezone: &badTZ, ExpectedVersion: order.Version}); err == nil {
		t.Fatal("invalid timezone must be refused")
	}
	if _, err := orders.Update(ctx, f.tenant, order.ID, ordersusecases.UpdateInput{
		ServiceFields: ordersmodels.JSONMap{"other": "x"}, ExpectedVersion: order.Version}); err == nil {
		t.Fatal("dropping a required service field must be refused")
	}
	if _, err := orders.Update(ctx, f.other, order.ID, ordersusecases.UpdateInput{
		Priority: &priority, ExpectedVersion: order.Version}); codeString(err) != "order_not_found" {
		t.Fatalf("cross-tenant PATCH = %v", err)
	}
	edited, err := orders.Update(ctx, f.tenant, order.ID, ordersusecases.UpdateInput{
		Priority: &priority, SpecialInstructions: &instructions, ExpectedVersion: order.Version})
	if err != nil || edited.Priority != priority || edited.SpecialInstructions != instructions ||
		edited.Status != ordersmodels.StatusDraft || edited.Version != order.Version+1 {
		t.Fatalf("PATCH = %+v err=%v", edited, err)
	}

	// Submit, then the submitter cannot approve their own order.
	if _, err = orders.Submit(ctx, f.other, order.ID, edited.Version, ""); codeString(err) != "order_not_found" {
		t.Fatalf("cross-tenant submit = %v", err)
	}
	submitted, err := orders.Submit(ctx, f.tenant, order.ID, edited.Version, "submit-1")
	if err != nil || submitted.Status != ordersmodels.StatusAwaitingApproval {
		t.Fatalf("submit = %+v err=%v", submitted, err)
	}
	if _, err = orders.Confirm(ctx, f.tenant, order.ID, submitted.Version); codeString(err) != "order_approval_required" {
		t.Fatalf("direct confirm of an order under review = %v", err)
	}
	if _, err = orders.Approve(ctx, f.tenant, order.ID, submitted.Version); codeString(err) != "order_approval_required" {
		t.Fatalf("self-approval = %v", err)
	}

	// Reject requires a reason and returns the order to DRAFT.
	if _, err = orders.Reject(ctx, approver, order.ID, submitted.Version, " "); err == nil {
		t.Fatal("reject without a reason must be refused")
	}
	rejected, err := orders.Reject(ctx, approver, order.ID, submitted.Version, "WINDOW_TOO_TIGHT")
	if err != nil || rejected.Status != ordersmodels.StatusDraft {
		t.Fatalf("reject = %+v err=%v", rejected, err)
	}

	// An edit during review returns the order to DRAFT.
	resubmitted, err := orders.Submit(ctx, f.tenant, order.ID, rejected.Version, "submit-2")
	if err != nil {
		t.Fatal(err)
	}
	reference := "PO-778"
	reverted, err := orders.Update(ctx, f.tenant, order.ID, ordersusecases.UpdateInput{
		CustomerReference: &reference, ExpectedVersion: resubmitted.Version})
	if err != nil || reverted.Status != ordersmodels.StatusDraft || reverted.ApprovalRequestID != "" {
		t.Fatalf("edit during review = %+v err=%v", reverted, err)
	}

	final, err := orders.Submit(ctx, f.tenant, order.ID, reverted.Version, "submit-3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = orders.Approve(ctx, approver, order.ID, final.Version-1); err == nil {
		t.Fatal("stale approve must conflict")
	}
	approved, err := orders.Approve(ctx, approver, order.ID, final.Version)
	if err != nil || approved.Status != ordersmodels.StatusConfirmed || approved.ConfirmedAt == nil {
		t.Fatalf("approve = %+v err=%v", approved, err)
	}
	if _, err = orders.Update(ctx, f.tenant, order.ID, ordersusecases.UpdateInput{
		Priority: &priority, ExpectedVersion: approved.Version}); codeString(err) != "order_invalid_transition" {
		t.Fatalf("PATCH after confirmation = %v", err)
	}

	history, err := orders.History(ctx, f.tenant, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	var reasons []string
	for _, h := range history {
		reasons = append(reasons, h.PreviousStatus+">"+h.NewStatus+":"+h.ReasonCode)
	}
	want := []string{
		">DRAFT:", "DRAFT>AWAITING_APPROVAL:SUBMITTED_FOR_APPROVAL", "AWAITING_APPROVAL>DRAFT:WINDOW_TOO_TIGHT",
		"DRAFT>AWAITING_APPROVAL:SUBMITTED_FOR_APPROVAL", "AWAITING_APPROVAL>DRAFT:EDITED_DURING_APPROVAL",
		"DRAFT>AWAITING_APPROVAL:SUBMITTED_FOR_APPROVAL", "AWAITING_APPROVAL>CONFIRMED:APPROVED",
	}
	// The create path writes no history row; compare from the first transition.
	if len(reasons) != len(want)-1 {
		t.Fatalf("history = %v", reasons)
	}
	for i, r := range reasons {
		if r != want[i+1] {
			t.Fatalf("history[%d] = %s want %s (all %v)", i, r, want[i+1], reasons)
		}
	}
	for event, n := range map[string]int{
		"organization.orders.order.updated.v1": 2, "organization.orders.order.submitted.v1": 3,
		"organization.orders.order.rejected.v1": 1, "organization.orders.order.approved.v1": 1,
		"organization.orders.order.confirmed.v1": 1,
	} {
		if got := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1 AND event_type=$2`,
			order.ID, event); got != n {
			t.Fatalf("%s events=%d want %d", event, got, n)
		}
	}
	if got := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE resource_id=$1
		AND action IN ('orders.order.updated','orders.order.submitted','orders.order.rejected','orders.order.approved')`,
		order.ID); got != 7 {
		t.Fatalf("lifecycle audit rows=%d want 7", got)
	}
}

func TestOrders_SubmitConcurrentlyTransitionsOnce(t *testing.T) {
	f := newOrdersFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Concurrent Submit Co")
	item := f.activeCatalogItem(t, "CONC_SUBMIT")
	order := draftOrderWithStops(t, f.orders, f.tenant, customer.ID, item.ID)
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = f.orders.Submit(ctx, f.tenant, order.ID, order.Version, "")
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("exactly one concurrent submit must win, got %d (%v)", ok, errs)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_private_order_history WHERE order_id=$1
		AND new_status='AWAITING_APPROVAL'`, order.ID); n != 1 {
		t.Fatalf("submission history rows=%d", n)
	}
}

func TestOrders_ApprovalPolicyBindsWP9Framework(t *testing.T) {
	f := newOrdersFixture(t)
	ctx := context.Background()
	requester := seedApprovalMember(t, f.pool, f.tenant.OrganizationID, "rol_system_owner", "order-requester")
	approverID := seedApprovalMember(t, f.pool, f.tenant.OrganizationID, "rol_system_owner", "order-approver")
	eligibility := newEligibilityService(f.pool, nil)
	if _, err := eligibility.PublishPolicy(ctx, eligibilityusecases.Actor{OrganizationID: f.tenant.OrganizationID,
		ActorType: eligibilitymodels.ActorOrganizationUser, ActorID: requester}, &eligibilitymodels.Policy{
		PolicyKey: ordersusecases.ApprovalOperationType, ApprovalRequired: true, MinimumApprovals: 1,
		EligibleRoleIDs: []string{"rol_system_owner"}, SeparationOfDuties: true,
	}); err != nil {
		t.Fatalf("publish policy: %v", err)
	}
	approvals := approvalusecases.NewService(approvalusecases.Options{
		Pool: f.pool, Approvals: approvalrepos.NewRepository(f.pool), Eligibility: eligibility,
		Roles: staffrepos.NewRoleRepository(f.pool), Subjects: approvalrepos.NewSubjectVersions(f.pool),
		Events: fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(f.pool)),
	})
	orders := ordersusecases.NewService(ordersusecases.Options{
		Pool: f.pool, Repository: f.ordersRepo,
		Catalog: app.NewOrdersCatalogAdapter(catalogrepos.NewRepository(f.pool)),
		CRM:     app.NewOrdersCRMAdapter(crmrepos.NewRepository(f.pool)),
		Metrics: observability.NewMetrics(), Flags: allOrdersFlagsOn(),
	}).WithApprovals(app.NewOrdersApprovalAdapter(approvals, eligibility))

	asRequester := tenant.Context{OrganizationID: f.tenant.OrganizationID, OrganizationUserID: requester,
		ActorType: tenant.ActorTypeOrganizationUser, ActorID: requester, CorrelationID: "corr-wp9"}
	asApprover := asRequester
	asApprover.OrganizationUserID, asApprover.ActorID = approverID, approverID

	customer := f.customer(t, "Policy Bound Co")
	item := f.activeCatalogItem(t, "POLICY_ITEM")
	order := draftOrderWithStops(t, orders, asRequester, customer.ID, item.ID)

	if _, err := orders.Confirm(ctx, asRequester, order.ID, order.Version); codeString(err) != "order_approval_required" {
		t.Fatalf("direct confirm under an approval policy = %v", err)
	}
	submitted, err := orders.Submit(ctx, asRequester, order.ID, order.Version, "wp9-submit")
	if err != nil || submitted.ApprovalRequestID == "" {
		t.Fatalf("submit = %+v err=%v", submitted, err)
	}
	if _, err = orders.Approve(ctx, asApprover, order.ID, submitted.Version); err == nil {
		t.Fatal("approve before the WP9 decision must be refused")
	}
	if _, err = approvals.Decide(ctx, approvalusecases.DecideInput{
		Actor: approvalusecases.Actor{OrganizationID: f.tenant.OrganizationID, OrganizationUserID: approverID,
			ActorType: "ORGANIZATION_USER", ActorID: approverID},
		ApprovalRequestID: submitted.ApprovalRequestID, Decision: "APPROVED", ClientDecisionID: "wp9-decision",
		CorrelationID: "corr-wp9"}); err != nil {
		t.Fatalf("decide: %v", err)
	}
	confirmed, err := orders.Approve(ctx, asApprover, order.ID, submitted.Version)
	if err != nil || confirmed.Status != ordersmodels.StatusConfirmed {
		t.Fatalf("approve after WP9 decision = %+v err=%v", confirmed, err)
	}
}

// ---------------------------------------------------------------------------
// FLEET-15.10 / 15.11 real producers: ready -> assignment -> acceptance ->
// execution -> trip events.
// ---------------------------------------------------------------------------

type privateLifecycleFixture struct {
	*shipmentsFixture
	private   *privateusecases.Service
	repo      *privaterepos.Repository
	orders    *ordersusecases.Service
	projector marketusecases.PrivateExecutionEventProjector
	branchID  string
}

func newPrivateLifecycleFixture(t *testing.T) *privateLifecycleFixture {
	t.Helper()
	f := newShipmentsFixture(t)
	publisher := fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(f.pool))
	ordersRepo := ordersrepos.NewRepository(f.pool)
	shipmentsRepo := shipmentsrepos.NewRepository(f.pool)
	repo := privaterepos.NewRepository(f.pool)
	private := privateusecases.NewService(privateusecases.Options{
		Pool: f.pool, Repository: repo, Work: app.NewPrivateWorkAdapter(ordersRepo, shipmentsRepo),
		Eligibility: privateEligibilityAlways{},
		Progress:    app.NewPrivateWorkProgressAdapter(ordersRepo, shipmentsRepo, publisher),
		Events:      publisher,
		Flags: privateusecases.Flags{Enabled: true, WritesEnabled: true, WorkerEnabled: true,
			DispatchEnabled: true},
	})
	orders := ordersusecases.NewService(ordersusecases.Options{
		Pool: f.pool, Repository: ordersRepo,
		Catalog: app.NewOrdersCatalogAdapter(catalogrepos.NewRepository(f.pool)),
		CRM:     app.NewOrdersCRMAdapter(crmrepos.NewRepository(f.pool)),
		Events:  publisher, Metrics: observability.NewMetrics(), Flags: allOrdersFlagsOn(),
	}).WithDependentWork(app.NewOrdersDependentWorkAdapter(ordersRepo, shipmentsRepo, repo, publisher))
	// Private assignments are always branch-owned, so the orders here are too.
	branchID := "brn_ws2_" + f.tenant.OrganizationID[len(f.tenant.OrganizationID)-8:]
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO fleet_partner_branches(
		id,organization_id,code,name,timezone) VALUES($1,$2,$3,'Lifecycle Branch','Africa/Lagos')`,
		branchID, f.tenant.OrganizationID, "WS2-"+branchID); err != nil {
		t.Fatalf("branch: %v", err)
	}
	return &privateLifecycleFixture{shipmentsFixture: f, private: private, repo: repo, orders: orders,
		projector: app.NewPrivateExecutionEventProjector(f.pool, publisher), branchID: branchID}
}

func (f *privateLifecycleFixture) confirmedOrder(t *testing.T, name string) ordersmodels.Order {
	t.Helper()
	customer := f.customer(t, name)
	item := f.activeCatalogItem(t, "ITEM_"+name)
	order := draftBranchOrderWithStops(t, f.orders, f.tenant, f.branchID, customer.ID, item.ID)
	confirmed, err := f.orders.Confirm(context.Background(), f.tenant, order.ID, order.Version)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	return confirmed
}

func (f *privateLifecycleFixture) readyShipment(t *testing.T, order ordersmodels.Order) shipmentsmodels.Shipment {
	t.Helper()
	ctx := context.Background()
	stops, err := f.orders.ListStops(ctx, f.tenant, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	shipment, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: stops[0].ID, DestinationStopID: stops[1].ID,
		CargoProfile: shipmentsmodels.JSONMap{"description": "Pallets", "weightKg": 400}, DeclaredValueCurrency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := f.shipments.MarkReady(ctx, f.tenant, shipment.ID, shipment.Version)
	if err != nil {
		t.Fatalf("mark ready: %v", err)
	}
	return ready
}

func (f *privateLifecycleFixture) assign(t *testing.T, shipmentID, provider string) privatemodels.Assignment {
	t.Helper()
	assignment, err := f.private.Create(context.Background(), f.tenant, privateusecases.CreateInput{
		BranchID: f.branchID, WorkOrigin: "PRIVATE_SHIPMENT", WorkID: shipmentID, ProviderMembershipID: "mem_" + provider,
		ProviderID: provider, VehicleOrTruckReference: "veh_" + provider, ExecutionService: "DISPATCH"})
	if err != nil {
		t.Fatalf("create assignment: %v", err)
	}
	return assignment
}

func (f *privateLifecycleFixture) accept(t *testing.T, a privatemodels.Assignment) {
	t.Helper()
	if _, err := f.private.Respond(context.Background(), f.tenant.OrganizationID, a.ID, privateusecases.RespondInput{
		ProviderID: a.ProviderID, Decision: "ACCEPT", IdempotencyKey: "accept-" + a.ID,
		RequestFingerprint: "fp-" + a.ID}); err != nil {
		t.Fatalf("accept: %v", err)
	}
}

func (f *privateLifecycleFixture) orderStatus(t *testing.T, id string) string {
	t.Helper()
	o, err := f.orders.Get(context.Background(), f.tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	return o.Status
}

func (f *privateLifecycleFixture) shipmentStatus(t *testing.T, id string) string {
	t.Helper()
	s, err := f.shipments.Get(context.Background(), f.tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	return s.Status
}

func TestShipments_ReadyAndCancelCommands(t *testing.T) {
	f := newPrivateLifecycleFixture(t)
	ctx := context.Background()
	order := f.confirmedOrder(t, "ReadyCo")
	stops, _ := f.orders.ListStops(ctx, f.tenant, order.ID)
	empty, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: stops[0].ID, DestinationStopID: stops[1].ID, DeclaredValueCurrency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.shipments.MarkReady(ctx, f.tenant, empty.ID, empty.Version); err == nil {
		t.Fatal("a shipment describing nothing must not become ready")
	}
	if _, err = f.shipments.MarkReady(ctx, f.other, empty.ID, empty.Version); codeString(err) != "shipment_not_found" {
		t.Fatalf("cross-tenant ready = %v", err)
	}
	if _, err = f.shipments.Cancel(ctx, f.tenant, empty.ID, empty.Version, ""); err == nil {
		t.Fatal("cancel without a reason must be refused")
	}
	cancelled, err := f.shipments.Cancel(ctx, f.tenant, empty.ID, empty.Version, "CUSTOMER_REQUEST")
	if err != nil || cancelled.Status != shipmentsmodels.ShipmentCancelled {
		t.Fatalf("cancel = %+v err=%v", cancelled, err)
	}
	ready := f.readyShipment(t, order)
	if _, err = f.shipments.MarkReady(ctx, f.tenant, ready.ID, ready.Version); err == nil {
		t.Fatal("ready is not repeatable")
	}
	assignment := f.assign(t, ready.ID, "prov_ready")
	planned, _ := f.shipments.Get(ctx, f.tenant, ready.ID)
	if _, err = f.shipments.Cancel(ctx, f.tenant, ready.ID, planned.Version, "LATE"); codeString(err) != "shipment_invalid_state" {
		t.Fatalf("cancelling a shipment with a live offer = %v", err)
	}
	if assignment.Status != privatemodels.StatusPending {
		t.Fatalf("assignment status=%s", assignment.Status)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_shipment_history WHERE shipment_id=$1
		AND new_status='READY' AND reason_code='SHIPMENT_READY'`, ready.ID); n != 1 {
		t.Fatalf("ready history rows=%d", n)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE resource_id=$1
		AND action='shipments.shipment.ready'`, ready.ID); n != 1 {
		t.Fatalf("ready audit rows=%d", n)
	}
	// Assignment creation is what planned it, and that is evented.
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1
		AND event_type='organization.shipments.shipment.planned.v1'`, ready.ID); n != 1 {
		t.Fatalf("planned events=%d", n)
	}
}

func TestPrivateOrderLifecycle_RealProducersDriveOrderToCompletion(t *testing.T) {
	f := newPrivateLifecycleFixture(t)
	ctx := context.Background()
	order := f.confirmedOrder(t, "EndToEndCo")
	shipment := f.readyShipment(t, order)

	assignment := f.assign(t, shipment.ID, "prov_e2e")
	if s := f.orderStatus(t, order.ID); s != ordersmodels.StatusPlanned {
		t.Fatalf("assignment creation must plan the order, got %s", s)
	}
	if s := f.shipmentStatus(t, shipment.ID); s != shipmentsmodels.ShipmentPlanned {
		t.Fatalf("assignment creation must plan the shipment, got %s", s)
	}

	f.accept(t, assignment)
	if s := f.orderStatus(t, order.ID); s != ordersmodels.StatusAssigned {
		t.Fatalf("provider acceptance must assign the order, got %s", s)
	}
	if s := f.shipmentStatus(t, shipment.ID); s != shipmentsmodels.ShipmentAssigned {
		t.Fatalf("provider acceptance must assign the shipment, got %s", s)
	}
	op, err := f.repo.GetExecutionByAssignment(ctx, nil, f.tenant.OrganizationID, assignment.ID)
	if err != nil || op.Status != privatemodels.ExecutionPending || op.PrivateOrderID != order.ID {
		t.Fatalf("execution operation = %+v err=%v", op, err)
	}

	// The execution worker submits to the owning execution service.
	port := &executionPortStub{result: privateusecases.ExecutionOutcome{Definite: true, Confirmed: true,
		CanonicalRequestReference: "req_e2e", CanonicalTripReference: "trip_e2e", NormalizedStatus: "CONFIRMED",
		SourceVersion: 1}}
	if _, err = f.private.RunExecutionPass(ctx, port, privateusecases.ExecutionWorkerConfig{Owner: "e2e",
		Lease: 3 * time.Second, BatchSize: 10, MaxAttempts: 3}); err != nil || port.calls != 1 {
		t.Fatalf("execution pass err=%v calls=%d", err, port.calls)
	}

	// Trip events from the execution service drive IN_PROGRESS and COMPLETED.
	ingestion := marketusecases.NewService(marketusecases.Options{Pool: f.pool, PrivateEvents: f.projector})
	if _, err = ingestion.IngestEvent(ctx, nil, privateLifecycleEvent(op.ID, "trip_e2e", "evt_e2e_1",
		marketusecases.EventTripStatusChanged, "IN_TRANSIT", 1)); err != nil {
		t.Fatal(err)
	}
	if s := f.orderStatus(t, order.ID); s != ordersmodels.StatusInProgress {
		t.Fatalf("trip in transit must start the order, got %s", s)
	}
	if _, err = ingestion.IngestEvent(ctx, nil, privateLifecycleEvent(op.ID, "trip_e2e", "evt_e2e_2",
		marketusecases.EventTripCompleted, "COMPLETED", 2)); err != nil {
		t.Fatal(err)
	}
	if s := f.orderStatus(t, order.ID); s != ordersmodels.StatusCompleted {
		t.Fatalf("trip completion must complete the order, got %s", s)
	}

	history, err := f.orders.History(ctx, f.tenant, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	var path []string
	for _, h := range history {
		path = append(path, h.NewStatus+":"+h.ReasonCode)
	}
	want := []string{"CONFIRMED:", "PLANNED:PRIVATE_ASSIGNMENT_CREATED", "ASSIGNED:PROVIDER_ACCEPTED_ASSIGNMENT",
		"IN_PROGRESS:SHIPMENT_OUTCOME_AGGREGATION", "COMPLETED:SHIPMENT_OUTCOME_AGGREGATION"}
	if len(path) != len(want) {
		t.Fatalf("order path = %v", path)
	}
	for i := range want {
		if path[i] != want[i] {
			t.Fatalf("order path = %v want %v", path, want)
		}
	}
	for _, event := range []string{"organization.orders.order.planned.v1", "organization.orders.order.assigned.v1"} {
		if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1 AND event_type=$2`,
			order.ID, event); n != 1 {
			t.Fatalf("%s events=%d", event, n)
		}
	}
}

func TestPrivateOrderLifecycle_PartialAssignmentAndCancellationCompensation(t *testing.T) {
	f := newPrivateLifecycleFixture(t)
	ctx := context.Background()

	// Two shipments: one accepted, one still pending. The order stays PLANNED
	// and cannot be cancelled directly while a provider holds accepted work.
	order := f.confirmedOrder(t, "PartialCo")
	first := f.readyShipment(t, order)
	second := f.readyShipment(t, order)
	accepted := f.assign(t, first.ID, "prov_first")
	pending := f.assign(t, second.ID, "prov_second")
	f.accept(t, accepted)
	if s := f.orderStatus(t, order.ID); s != ordersmodels.StatusPlanned {
		t.Fatalf("order with an unassigned shipment must stay PLANNED, got %s", s)
	}
	current, _ := f.orders.Get(ctx, f.tenant, order.ID)
	if _, err := f.orders.Cancel(ctx, f.tenant, order.ID, current.Version, "CUSTOMER_REQUEST"); codeString(err) != "order_not_cancellable" {
		t.Fatalf("cancelling an order with accepted work = %v", err)
	}
	f.accept(t, pending)
	if s := f.orderStatus(t, order.ID); s != ordersmodels.StatusAssigned {
		t.Fatalf("all shipments accepted must assign the order, got %s", s)
	}

	// A planned order with only a pending offer: cancellation withdraws the
	// offer and the shipment in the same transaction.
	other := f.confirmedOrder(t, "CancelCo")
	shipment := f.readyShipment(t, other)
	offer := f.assign(t, shipment.ID, "prov_cancel")
	planned, _ := f.orders.Get(ctx, f.tenant, other.ID)
	if _, err := f.orders.Cancel(ctx, f.other, other.ID, planned.Version, "X"); codeString(err) != "order_not_found" {
		t.Fatalf("cross-tenant cancel = %v", err)
	}
	cancelled, err := f.orders.Cancel(ctx, f.tenant, other.ID, planned.Version, "CUSTOMER_REQUEST")
	if err != nil || cancelled.Status != ordersmodels.StatusCancelled {
		t.Fatalf("cancel = %+v err=%v", cancelled, err)
	}
	if s := f.shipmentStatus(t, shipment.ID); s != shipmentsmodels.ShipmentCancelled {
		t.Fatalf("shipment after order cancel = %s", s)
	}
	withdrawn, err := f.repo.GetAssignment(ctx, nil, f.tenant.OrganizationID, offer.ID)
	if err != nil || withdrawn.Status != privatemodels.StatusCancelled {
		t.Fatalf("offer after order cancel = %+v err=%v", withdrawn, err)
	}
	if _, err = f.private.Respond(ctx, f.tenant.OrganizationID, offer.ID, privateusecases.RespondInput{
		ProviderID: offer.ProviderID, Decision: "ACCEPT", IdempotencyKey: "late-accept",
		RequestFingerprint: "late"}); err == nil {
		t.Fatal("a provider must not be able to accept a withdrawn offer")
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1
		AND event_type='private_assignment.cancelled.v1'`, offer.ID); n != 1 {
		t.Fatalf("assignment cancelled events=%d", n)
	}
}

// ---------------------------------------------------------------------------
// HTTP: the real orders and CRM handlers with the real authorizer and the
// shared durable idempotency store.
// ---------------------------------------------------------------------------

type fixedRoleBindings map[string][]staffmodels.RoleBinding

func (r fixedRoleBindings) EffectivePermissions(context.Context, string, string) (map[string][]staffmodels.RoleBinding, error) {
	return r, nil
}

func ws2Call(engine *gin.Engine, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	return resp
}

func TestOrdersAndLeadsHTTP_IdempotencyVersionAndPermissions(t *testing.T) {
	f := newOrdersFixture(t)
	owner := seedApprovalMember(t, f.pool, f.tenant.OrganizationID, "rol_system_owner", "http-owner")
	analyst := seedApprovalMember(t, f.pool, f.tenant.OrganizationID, "rol_system_analyst", "http-analyst")
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(f.pool), nil, 0)
	store := idempotency.NewStore(f.pool, 0)
	crm := crmusecases.NewService(crmusecases.Options{
		Pool: f.pool, Repository: crmrepos.NewRepository(f.pool), Metrics: observability.NewMetrics(),
		Flags: allCRMFlagsOn(), TokenSecret: []byte("http-token"), ContactSecret: []byte("http-contact"),
		TokenTTL: time.Hour,
	})

	router := func(tc tenant.Context, authz *accesscontrol.Authorizer) *gin.Engine {
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		group := engine.Group("/fleet-partner/v1", func(c *gin.Context) {
			c.Set(middleware.TenantContextKey, tc)
			c.Next()
		})
		ordershttp.NewHandler(f.orders, authz).WithIdempotency(store).RegisterTenantRoutes(group)
		crmhttp.NewHandler(crm, authz).RegisterTenantRoutes(group)
		return engine
	}
	member := func(id string) tenant.Context {
		return tenant.Context{OrganizationID: f.tenant.OrganizationID, OrganizationUserID: id,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: id}
	}
	asOwner, asAnalyst := router(member(owner), authorizer), router(member(analyst), authorizer)

	customer := f.customer(t, "HTTP Lifecycle Co")
	item := f.activeCatalogItem(t, "HTTP_ITEM")
	order := draftOrderWithStops(t, f.orders, f.tenant, customer.ID, item.ID)
	base := "/fleet-partner/v1/orders/" + order.ID

	// Default deny: the read-only analyst cannot edit.
	if resp := ws2Call(asAnalyst, http.MethodPatch, base, map[string]any{"priority": "HIGH"},
		map[string]string{"If-Match": "1"}); resp.Code != http.StatusForbidden {
		t.Fatalf("analyst PATCH = %d %s", resp.Code, resp.Body.String())
	}
	// PATCH with a stale If-Match is a conflict.
	if resp := ws2Call(asOwner, http.MethodPatch, base, map[string]any{"priority": "HIGH"},
		map[string]string{"If-Match": "99"}); resp.Code != http.StatusConflict {
		t.Fatalf("stale PATCH = %d %s", resp.Code, resp.Body.String())
	}
	if resp := ws2Call(asOwner, http.MethodPatch, base, map[string]any{"priority": "HIGH"},
		map[string]string{"If-Match": fmt.Sprint(order.Version)}); resp.Code != http.StatusOK {
		t.Fatalf("PATCH = %d %s", resp.Code, resp.Body.String())
	}

	// Submit requires an Idempotency-Key, and an identical retry replays.
	submitBody := map[string]any{"expectedVersion": order.Version + 1}
	if resp := ws2Call(asOwner, http.MethodPost, base+"/submit", submitBody, nil); resp.Code != http.StatusBadRequest {
		t.Fatalf("submit without key = %d %s", resp.Code, resp.Body.String())
	}
	first := ws2Call(asOwner, http.MethodPost, base+"/submit", submitBody, map[string]string{"Idempotency-Key": "http-submit"})
	second := ws2Call(asOwner, http.MethodPost, base+"/submit", submitBody, map[string]string{"Idempotency-Key": "http-submit"})
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() {
		t.Fatalf("submit/replay = %d/%d %s | %s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_private_order_history WHERE order_id=$1
		AND new_status='AWAITING_APPROVAL'`, order.ID); n != 1 {
		t.Fatalf("replayed submit wrote %d history rows", n)
	}
	// The submitter cannot approve their own order.
	if resp := ws2Call(asOwner, http.MethodPost, base+"/approve", map[string]any{"expectedVersion": order.Version + 2},
		map[string]string{"Idempotency-Key": "http-approve"}); resp.Code != http.StatusForbidden {
		t.Fatalf("self-approve = %d %s", resp.Code, resp.Body.String())
	}

	// Leads: create, read-only staff cannot convert, owner converts once.
	leadResp := ws2Call(asOwner, http.MethodPost, "/fleet-partner/v1/leads",
		map[string]any{"companyName": "HTTP Prospect", "source": "EVENT",
			"contactSnapshot": map[string]any{"email": "p@example.invalid"}},
		map[string]string{"Idempotency-Key": "http-lead"})
	if leadResp.Code != http.StatusCreated {
		t.Fatalf("create lead = %d %s", leadResp.Code, leadResp.Body.String())
	}
	var created struct {
		Data struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(leadResp.Body.Bytes(), &created); err != nil || created.Data.ID == "" {
		t.Fatalf("lead body %s err=%v", leadResp.Body.String(), err)
	}
	convertPath := "/fleet-partner/v1/leads/" + created.Data.ID + "/convert"
	convertBody := map[string]any{"newCustomer": map[string]any{"customerType": "BUSINESS", "defaultCurrency": "NGN"},
		"opportunity": map[string]any{"title": "HTTP deal"}, "expectedVersion": created.Data.Version}
	if resp := ws2Call(asAnalyst, http.MethodPost, convertPath, convertBody,
		map[string]string{"Idempotency-Key": "http-convert"}); resp.Code != http.StatusForbidden {
		t.Fatalf("analyst convert = %d %s", resp.Code, resp.Body.String())
	}
	converted := ws2Call(asOwner, http.MethodPost, convertPath, convertBody, map[string]string{"Idempotency-Key": "http-convert"})
	if converted.Code != http.StatusCreated {
		t.Fatalf("convert = %d %s", converted.Code, converted.Body.String())
	}
	replayed := ws2Call(asOwner, http.MethodPost, convertPath, convertBody, map[string]string{"Idempotency-Key": "http-convert"})
	if replayed.Code != http.StatusOK || !bytes.Contains(replayed.Body.Bytes(), []byte(`"replayed":true`)) {
		t.Fatalf("convert replay = %d %s", replayed.Code, replayed.Body.String())
	}

	// Cross-tenant lead read through HTTP is not-found even with the grant.
	foreign := router(f.other, accesscontrol.NewAuthorizer(fixedRoleBindings{
		"crm.customers.read": {{OrganizationID: f.other.OrganizationID, OrganizationUserID: f.other.OrganizationUserID,
			ScopeType: staffmodels.ScopeOrganization, EffectiveFrom: time.Now().Add(-time.Minute)}}}, nil, 0))
	if resp := ws2Call(foreign, http.MethodGet, "/fleet-partner/v1/leads/"+created.Data.ID, nil, nil); resp.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant lead read = %d %s", resp.Code, resp.Body.String())
	}
}
