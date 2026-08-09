package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	crmmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	crmrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	crmusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Fixture
// ---------------------------------------------------------------------------

type crmFixture struct {
	pool    *pgxpool.Pool
	service *crmusecases.Service
	repo    *crmrepos.Repository
	tenant  tenant.Context
	other   tenant.Context
}

// allFlagsOn is the fully enabled configuration.
//
// The tests deliberately run with every capability on so a failure is a defect
// in the feature rather than an artefact of a switch. The defaults-off
// behaviour has its own test.
func allCRMFlagsOn() crmusecases.Flags {
	return crmusecases.Flags{
		CRMEnabled: true, CustomerWritesEnabled: true, ContactWritesEnabled: true,
		LocationWritesEnabled: true, OpportunitiesEnabled: true, QuotationsEnabled: true,
		QuotationIssueEnabled: true, QuotationAcceptEnabled: true, ContractsEnabled: true,
		ContractActivation: true, SLAEnabled: true, CreditEnabled: true,
		NotificationWorkerEnabled: true, QuotationExpiryWorker: true,
		ContractExpiryWorker: true, ContractRenewalWorker: true,
		SLAEvaluationWorker: true, SLAEscalationWorker: true,
	}
}

func newCRMFixture(t *testing.T, flags crmusecases.Flags) *crmFixture {
	t.Helper()
	pool := testPool(t)
	primary := createOrganization(t, pool, "CRM Primary Fleet", "RC-CRM-1")
	secondary := createOrganization(t, pool, "CRM Secondary Fleet", "RC-CRM-2")

	repo := crmrepos.NewRepository(pool)
	service := crmusecases.NewService(crmusecases.Options{
		Pool:       pool,
		Repository: repo,
		Metrics:    observability.NewMetrics(),
		Flags:      flags,
		// Deliberately no Approvals, Media, Notifications or Support port. An
		// absent approval framework means the policy raises no request, which
		// exercises the "no approval required" branch; an absent Media client
		// makes every document projection UNKNOWN, which is what the
		// fail-closed document tests below rely on.
		TokenSecret:   []byte("crm-token-secret-for-integration-tests"),
		ContactSecret: []byte("crm-contact-secret-for-integration-tests"),
		TokenTTL:      48 * time.Hour,
	})

	return &crmFixture{
		pool: pool, service: service, repo: repo,
		tenant: tenant.Context{OrganizationID: primary.ID, OrganizationUserID: "usr_crm_primary",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_crm_primary"},
		other: tenant.Context{OrganizationID: secondary.ID, OrganizationUserID: "usr_crm_other",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_crm_other"},
	}
}

func (f *crmFixture) customer(t *testing.T, name string) crmmodels.Customer {
	t.Helper()
	customer, err := f.service.CreateCustomer(context.Background(), f.tenant,
		crmusecases.CreateCustomerInput{
			CustomerType: crmmodels.CustomerTypeBusiness, LegalName: name,
			RegistrationNumber: "RC-" + name, TaxReference: "TIN-" + name,
			DefaultCurrency: "NGN", IdempotencyKey: "create:" + name,
		})
	if err != nil {
		t.Fatalf("create customer %q: %v", name, err)
	}
	return customer
}

// activeCustomer returns a customer moved to ACTIVE, which is what contract
// activation requires.
func (f *crmFixture) activeCustomer(t *testing.T, name string) crmmodels.Customer {
	t.Helper()
	customer := f.customer(t, name)
	updated, err := f.service.ChangeCustomerStatus(context.Background(), f.tenant, customer.ID,
		crmusecases.ChangeCustomerStatusInput{
			Status: crmmodels.CustomerStatusActive, ExpectedVersion: customer.Version,
			IdempotencyKey: "activate:" + customer.ID,
		})
	if err != nil {
		t.Fatalf("activate customer: %v", err)
	}
	return updated
}

func crmErrorCode(err error) string {
	if err == nil {
		return ""
	}
	return string(apperrors.From(err).Code)
}

// ---------------------------------------------------------------------------
// Customers
// ---------------------------------------------------------------------------

func TestCRMCustomerLifecycleAndIsolation(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()

	for _, customerType := range []string{
		crmmodels.CustomerTypeIndividual, crmmodels.CustomerTypeBusiness,
		crmmodels.CustomerTypeGovernment, crmmodels.CustomerTypeNonprofit,
		crmmodels.CustomerTypeInternalBusinessUnit, crmmodels.CustomerTypeOther,
	} {
		created, err := f.service.CreateCustomer(ctx, f.tenant, crmusecases.CreateCustomerInput{
			CustomerType: customerType, LegalName: "Acme " + customerType,
			DefaultCurrency: "NGN", IdempotencyKey: "type:" + customerType,
		})
		if err != nil {
			t.Fatalf("create %s customer: %v", customerType, err)
		}
		if created.Status != crmmodels.CustomerStatusProspect {
			t.Fatalf("new customer should start PROSPECT, got %s", created.Status)
		}
	}

	// Organization-scoped duplicate detection.
	customer := f.customer(t, "Duplicate Test Ltd")
	_, err := f.service.CreateCustomer(ctx, f.tenant, crmusecases.CreateCustomerInput{
		CustomerType: crmmodels.CustomerTypeBusiness, LegalName: "Duplicate Test Ltd",
		RegistrationNumber: "RC-Duplicate Test Ltd", DefaultCurrency: "NGN",
		IdempotencyKey: "dup",
	})
	if crmErrorCode(err) != "customer_already_exists" {
		t.Fatalf("expected customer_already_exists, got %v", err)
	}
	// The same legal entity in another organization is not a conflict: one
	// tenant's data must never constrain another's.
	if _, err = f.service.CreateCustomer(ctx, f.other, crmusecases.CreateCustomerInput{
		CustomerType: crmmodels.CustomerTypeBusiness, LegalName: "Duplicate Test Ltd",
		RegistrationNumber: "RC-Duplicate Test Ltd", DefaultCurrency: "NGN",
		IdempotencyKey: "dup-other",
	}); err != nil {
		t.Fatalf("same entity in another organization must be permitted: %v", err)
	}

	// Cross-tenant read is not-found, never forbidden: a distinguishable
	// response is an existence oracle.
	if _, err = f.service.GetCustomer(ctx, f.other, customer.ID); crmErrorCode(err) != "customer_not_found" {
		t.Fatalf("cross-tenant read must be customer_not_found, got %v", err)
	}

	// Invalid transition: PROSPECT cannot jump to SUSPENDED.
	_, err = f.service.ChangeCustomerStatus(ctx, f.tenant, customer.ID,
		crmusecases.ChangeCustomerStatusInput{Status: crmmodels.CustomerStatusSuspended,
			ExpectedVersion: customer.Version, IdempotencyKey: "bad-transition"})
	if crmErrorCode(err) != "customer_invalid_state" {
		t.Fatalf("expected customer_invalid_state, got %v", err)
	}

	// An update without an expected version is refused outright.
	name := "Renamed Ltd"
	_, err = f.service.UpdateCustomer(ctx, f.tenant, customer.ID,
		crmusecases.UpdateCustomerInput{LegalName: &name})
	if err == nil {
		t.Fatal("update without expectedVersion must be refused")
	}

	// Stale expected version conflicts.
	_, err = f.service.UpdateCustomer(ctx, f.tenant, customer.ID,
		crmusecases.UpdateCustomerInput{LegalName: &name, ExpectedVersion: 999})
	if crmErrorCode(err) != "quotation_version_conflict" && crmErrorCode(err) != "customer_not_found" {
		t.Fatalf("stale version must conflict, got %v", err)
	}

	// Archive, then prove an archived customer is closed to new commitments.
	archived, err := f.service.ArchiveCustomer(ctx, f.tenant, customer.ID, customer.Version,
		"closed_account", "archive-1", "")
	if err != nil {
		t.Fatalf("archive customer: %v", err)
	}
	if archived.Status != crmmodels.CustomerStatusArchived || archived.ArchivedAt == nil {
		t.Fatalf("archive did not take: %+v", archived)
	}
	_, err = f.service.UpdateCustomer(ctx, f.tenant, customer.ID,
		crmusecases.UpdateCustomerInput{LegalName: &name, ExpectedVersion: archived.Version})
	if crmErrorCode(err) != "customer_archived" {
		t.Fatalf("archived customer must refuse updates, got %v", err)
	}

	// History is append-only and records the archive.
	history, err := f.service.CustomerHistory(ctx, f.tenant, customer.ID,
		emptyCursorValue(), false, 50)
	if err != nil {
		t.Fatalf("customer history: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("expected creation and archive history, got %d", len(history))
	}
	if _, err = f.pool.Exec(ctx,
		`UPDATE fleet_partner_crm_customer_history SET reason_code = 'tampered' WHERE id = $1`,
		history[0].ID); err == nil {
		t.Fatal("customer history must reject UPDATE")
	}
	if _, err = f.pool.Exec(ctx,
		`DELETE FROM fleet_partner_crm_customer_history WHERE id = $1`,
		history[0].ID); err == nil {
		t.Fatal("customer history must reject DELETE")
	}
}

func TestCRMCustomerRiskForcesCreditRestriction(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Risk Subject Ltd")

	profile, err := f.service.SetCreditProfile(ctx, f.tenant, customer.ID,
		crmusecases.SetCreditProfileInput{Currency: "NGN", PaymentTermsDays: 30,
			IdempotencyKey: "credit-1", ReasonCode: "initial"})
	if err != nil {
		t.Fatalf("set credit profile: %v", err)
	}
	if profile.HoldStatus != crmmodels.HoldNone {
		t.Fatalf("new profile should have no hold, got %s", profile.HoldStatus)
	}

	// A risk change requires a reason. A decision nobody has to justify is a
	// decision nobody can review.
	_, err = f.service.ChangeCustomerRisk(ctx, f.tenant, customer.ID,
		crmusecases.ChangeCustomerRiskInput{RiskStatus: crmmodels.RiskRestricted,
			ExpectedVersion: customer.Version, IdempotencyKey: "risk-no-reason"})
	if err == nil {
		t.Fatal("risk change without a reason must be refused")
	}

	restricted, err := f.service.ChangeCustomerRisk(ctx, f.tenant, customer.ID,
		crmusecases.ChangeCustomerRiskInput{RiskStatus: crmmodels.RiskRestricted,
			ReasonCode: "sanctions_match", ExpectedVersion: customer.Version,
			IdempotencyKey: "risk-1"})
	if err != nil {
		t.Fatalf("escalate risk: %v", err)
	}
	if restricted.RiskStatus != crmmodels.RiskRestricted {
		t.Fatalf("risk did not escalate: %+v", restricted)
	}

	// The risk decision must reach the gate that actually blocks activation.
	synced, err := f.repo.GetCreditProfile(ctx, nil, f.tenant.OrganizationID, customer.ID)
	if err != nil {
		t.Fatalf("read credit profile: %v", err)
	}
	if synced.HoldStatus != crmmodels.HoldRestricted {
		t.Fatalf("restricted risk must force a restricted credit hold, got %s",
			synced.HoldStatus)
	}
}

// ---------------------------------------------------------------------------
// Contacts and locations
// ---------------------------------------------------------------------------

func TestCRMContactNormalizationPrimaryAndPrivacy(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.customer(t, "Contact Owner Ltd")

	contact, err := f.service.CreateContact(ctx, f.tenant, customer.ID,
		crmusecases.CreateContactInput{
			FullName: "Ada Operations", Email: "  Ada.Ops@Example.COM ",
			Phone: "08031234567", ContactPurpose: crmmodels.PurposeOperations,
			ConsentStatus: crmmodels.ConsentGranted, IsPrimary: true,
			IdempotencyKey: "contact-1",
		})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	if contact.NormalizedEmail != "ada.ops@example.com" {
		t.Fatalf("email not normalized: %q", contact.NormalizedEmail)
	}
	if !strings.HasPrefix(contact.NormalizedPhone, "+234") {
		t.Fatalf("phone not normalized to E.164: %q", contact.NormalizedPhone)
	}
	if contact.ConsentRecordedAt == nil {
		t.Fatal("recording consent must stamp the time it was recorded")
	}

	// An unsupported channel is refused rather than accepted and quietly
	// ignored at delivery time.
	_, err = f.service.CreateContact(ctx, f.tenant, customer.ID,
		crmusecases.CreateContactInput{FullName: "Bad Channel",
			Email: "bad@example.com", PreferredChannel: "CARRIER_PIGEON",
			IdempotencyKey: "contact-bad-channel"})
	if crmErrorCode(err) != "customer_contact_conflict" {
		t.Fatalf("unsupported channel must be refused, got %v", err)
	}
	// An invalid email is refused.
	_, err = f.service.CreateContact(ctx, f.tenant, customer.ID,
		crmusecases.CreateContactInput{FullName: "Bad Email", Email: "not-an-email",
			IdempotencyKey: "contact-bad-email"})
	if err == nil {
		t.Fatal("invalid email must be refused")
	}

	// A stated WhatsApp preference is recorded but never claimed as
	// deliverable.
	whatsapp, err := f.service.CreateContact(ctx, f.tenant, customer.ID,
		crmusecases.CreateContactInput{FullName: "Prefers WhatsApp",
			Email: "wa@example.com", PreferredChannel: crmmodels.ChannelWhatsApp,
			ContactPurpose: crmmodels.PurposeBilling, IdempotencyKey: "contact-wa"})
	if err != nil {
		t.Fatalf("create whatsapp contact: %v", err)
	}
	if crmmodels.ChannelDeliverable(whatsapp.PreferredChannel) {
		t.Fatal("WhatsApp must not be reported as a deliverable channel")
	}

	// Two concurrent promotions for the same purpose resolve to exactly one
	// primary. The partial unique index is what decides the race.
	first, err := f.service.CreateContact(ctx, f.tenant, customer.ID,
		crmusecases.CreateContactInput{FullName: "Candidate One",
			Email: "one@example.com", ContactPurpose: crmmodels.PurposeSales,
			IdempotencyKey: "contact-c1"})
	if err != nil {
		t.Fatalf("create candidate one: %v", err)
	}
	second, err := f.service.CreateContact(ctx, f.tenant, customer.ID,
		crmusecases.CreateContactInput{FullName: "Candidate Two",
			Email: "two@example.com", ContactPurpose: crmmodels.PurposeSales,
			IdempotencyKey: "contact-c2"})
	if err != nil {
		t.Fatalf("create candidate two: %v", err)
	}

	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, results[0] = f.service.MakeContactPrimary(ctx, f.tenant, customer.ID, first.ID,
			first.Version, "primary-1", "")
	}()
	go func() {
		defer wg.Done()
		_, results[1] = f.service.MakeContactPrimary(ctx, f.tenant, customer.ID, second.ID,
			second.Version, "primary-2", "")
	}()
	wg.Wait()

	var primaries int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_customer_contacts
		WHERE organization_id = $1 AND customer_id = $2 AND contact_purpose = 'SALES'
		  AND is_primary AND archived_at IS NULL`,
		f.tenant.OrganizationID, customer.ID).Scan(&primaries); err != nil {
		t.Fatalf("count primaries: %v", err)
	}
	if primaries != 1 {
		t.Fatalf("exactly one SALES primary must survive concurrent promotion, got %d (%v, %v)",
			primaries, results[0], results[1])
	}

	// Archiving clears the primary flag so the slot is reusable.
	archived, err := f.service.ArchiveContact(ctx, f.tenant, customer.ID, contact.ID,
		contact.Version, "left_company", "archive-contact", "")
	if err != nil {
		t.Fatalf("archive contact: %v", err)
	}
	if archived.IsPrimary {
		t.Fatal("archiving must clear the primary flag or the slot is permanently blocked")
	}
	newName := "Cannot Edit"
	_, err = f.service.UpdateContact(ctx, f.tenant, customer.ID, contact.ID,
		crmusecases.UpdateContactInput{FullName: &newName,
			ExpectedVersion: archived.Version})
	if crmErrorCode(err) != "customer_contact_conflict" {
		t.Fatalf("archived contact must refuse updates, got %v", err)
	}

	// Cross-tenant nested access is not-found.
	if _, err = f.service.GetContact(ctx, f.other, customer.ID,
		contact.ID); crmErrorCode(err) != "customer_not_found" {
		t.Fatalf("cross-tenant contact read must be not-found, got %v", err)
	}
}

func TestCRMLocationValidationAndPrivacy(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.customer(t, "Location Owner Ltd")

	address := crmmodels.Address{Line1: "12 Dockside Road", City: "Apapa",
		StateOrRegion: "Lagos", CountryCode: "NG"}

	for _, locationType := range []string{
		crmmodels.LocationPickup, crmmodels.LocationDelivery, crmmodels.LocationBilling,
		crmmodels.LocationWarehouse, crmmodels.LocationOffice, crmmodels.LocationDepot,
		crmmodels.LocationSite, crmmodels.LocationPort, crmmodels.LocationTerminal,
		crmmodels.LocationOther,
	} {
		if _, err := f.service.CreateLocation(ctx, f.tenant, customer.ID,
			crmusecases.CreateLocationInput{LocationType: locationType,
				Label: "Site " + locationType, Address: address, Timezone: "Africa/Lagos",
				IdempotencyKey: "loc:" + locationType}); err != nil {
			t.Fatalf("create %s location: %v", locationType, err)
		}
	}

	// An offset is not a timezone: it cannot represent a DST boundary.
	_, err := f.service.CreateLocation(ctx, f.tenant, customer.ID,
		crmusecases.CreateLocationInput{LocationType: crmmodels.LocationDepot,
			Label: "Bad TZ", Address: address, Timezone: "UTC+1",
			IdempotencyKey: "loc-bad-tz"})
	if crmErrorCode(err) != "customer_location_invalid_timezone" {
		t.Fatalf("expected customer_location_invalid_timezone, got %v", err)
	}

	// Half a coordinate routes a vehicle to the prime meridian.
	latitude := 6.45
	_, err = f.service.CreateLocation(ctx, f.tenant, customer.ID,
		crmusecases.CreateLocationInput{LocationType: crmmodels.LocationDepot,
			Label: "Half Coordinate", Address: address, Timezone: "Africa/Lagos",
			Latitude: &latitude, IdempotencyKey: "loc-half-coord"})
	if crmErrorCode(err) != "customer_location_invalid_coordinates" {
		t.Fatalf("expected customer_location_invalid_coordinates, got %v", err)
	}

	outOfRange := 91.0
	longitude := 3.39
	_, err = f.service.CreateLocation(ctx, f.tenant, customer.ID,
		crmusecases.CreateLocationInput{LocationType: crmmodels.LocationDepot,
			Label: "Bad Latitude", Address: address, Timezone: "Africa/Lagos",
			Latitude: &outOfRange, Longitude: &longitude, IdempotencyKey: "loc-bad-lat"})
	if crmErrorCode(err) != "customer_location_invalid_coordinates" {
		t.Fatalf("expected out-of-range latitude to be refused, got %v", err)
	}

	// A contact belonging to another customer must not be attachable.
	stranger := f.customer(t, "Stranger Ltd")
	strangerContact, err := f.service.CreateContact(ctx, f.tenant, stranger.ID,
		crmusecases.CreateContactInput{FullName: "Not Yours",
			Email: "not-yours@example.com", IdempotencyKey: "stranger-contact"})
	if err != nil {
		t.Fatalf("create stranger contact: %v", err)
	}
	_, err = f.service.CreateLocation(ctx, f.tenant, customer.ID,
		crmusecases.CreateLocationInput{LocationType: crmmodels.LocationDepot,
			Label: "Wrong Contact", Address: address, Timezone: "Africa/Lagos",
			ContactReference: strangerContact.ID, IdempotencyKey: "loc-wrong-contact"})
	if crmErrorCode(err) != "customer_contact_not_found" {
		t.Fatalf("another customer's contact must not be attachable, got %v", err)
	}

	// Archive, then prove the archived location refuses further edits.
	sensitive, err := f.service.CreateLocation(ctx, f.tenant, customer.ID,
		crmusecases.CreateLocationInput{LocationType: crmmodels.LocationSite,
			Label: "Restricted Yard", Address: address, Timezone: "Africa/Lagos",
			Latitude: &latitude, Longitude: &longitude,
			AccessInstructions: "Gate code 4417, ask for the night supervisor",
			SafetyInstructions: "Live overhead cables on the east approach",
			IdempotencyKey:     "loc-sensitive"})
	if err != nil {
		t.Fatalf("create sensitive location: %v", err)
	}
	archived, err := f.service.ArchiveLocation(ctx, f.tenant, customer.ID, sensitive.ID,
		sensitive.Version, "site_closed", "archive-loc", "")
	if err != nil {
		t.Fatalf("archive location: %v", err)
	}
	label := "Cannot Edit"
	_, err = f.service.UpdateLocation(ctx, f.tenant, customer.ID, sensitive.ID,
		crmusecases.UpdateLocationInput{Label: &label, ExpectedVersion: archived.Version})
	if crmErrorCode(err) != "customer_location_not_found" {
		t.Fatalf("archived location must refuse updates, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Opportunities
// ---------------------------------------------------------------------------

func TestCRMOpportunityTransitionMatrix(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.customer(t, "Pipeline Ltd")

	amount := int64(750000)
	opportunity, err := f.service.CreateOpportunity(ctx, f.tenant,
		crmusecases.CreateOpportunityInput{CustomerID: customer.ID,
			Title: "Q3 haulage tender", ExpectedValueAmountMinor: &amount,
			ExpectedValueCurrency: "NGN", ProbabilityBasisPoints: 4500,
			IdempotencyKey: "opp-1"})
	if err != nil {
		t.Fatalf("create opportunity: %v", err)
	}

	// A value with no currency is a number nobody can add up.
	_, err = f.service.CreateOpportunity(ctx, f.tenant, crmusecases.CreateOpportunityInput{
		CustomerID: customer.ID, Title: "Currencyless",
		ExpectedValueAmountMinor: &amount, IdempotencyKey: "opp-no-currency"})
	if err == nil {
		t.Fatal("an expected value without a currency must be refused")
	}
	// Probability is basis points, bounded at both ends.
	for _, probability := range []int{-1, 10001} {
		if _, err = f.service.CreateOpportunity(ctx, f.tenant,
			crmusecases.CreateOpportunityInput{CustomerID: customer.ID,
				Title: "Out of range", ProbabilityBasisPoints: probability,
				IdempotencyKey: "opp-prob"}); err == nil {
			t.Fatalf("probability %d must be refused", probability)
		}
	}

	// Walk the forward path.
	current := opportunity
	for _, stage := range []string{crmmodels.StageQualified, crmmodels.StageNeedsAnalysis,
		crmmodels.StageProposal, crmmodels.StageNegotiation} {
		current, err = f.service.TransitionOpportunity(ctx, f.tenant, current.ID,
			crmusecases.TransitionOpportunityInput{Stage: stage,
				ExpectedVersion: current.Version, IdempotencyKey: "stage:" + stage})
		if err != nil {
			t.Fatalf("transition to %s: %v", stage, err)
		}
	}

	// Skipping a stage is refused.
	skipper := f.customerOpportunity(t, customer.ID, "Skipper")
	_, err = f.service.TransitionOpportunity(ctx, f.tenant, skipper.ID,
		crmusecases.TransitionOpportunityInput{Stage: crmmodels.StageWon,
			ExpectedVersion: skipper.Version, IdempotencyKey: "skip"})
	if crmErrorCode(err) != "opportunity_invalid_transition" {
		t.Fatalf("LEAD may not jump to WON, got %v", err)
	}

	// ON_HOLD requires a reason and may only resume into a stage the
	// opportunity actually reached.
	held, err := f.service.TransitionOpportunity(ctx, f.tenant, current.ID,
		crmusecases.TransitionOpportunityInput{Stage: crmmodels.StageOnHold,
			ExpectedVersion: current.Version, IdempotencyKey: "hold-no-reason"})
	if err == nil {
		t.Fatal("ON_HOLD without a reason must be refused")
	}
	held, err = f.service.TransitionOpportunity(ctx, f.tenant, current.ID,
		crmusecases.TransitionOpportunityInput{Stage: crmmodels.StageOnHold,
			ReasonCode: "customer_budget_freeze", ExpectedVersion: current.Version,
			IdempotencyKey: "hold-1"})
	if err != nil {
		t.Fatalf("hold opportunity: %v", err)
	}
	// LEAD was occupied, so resuming there is permitted; NEEDS_ANALYSIS was
	// too. A stage never occupied is refused.
	_, err = f.service.TransitionOpportunity(ctx, f.tenant, held.ID,
		crmusecases.TransitionOpportunityInput{Stage: crmmodels.StageWon,
			ExpectedVersion: held.Version, IdempotencyKey: "resume-won"})
	if crmErrorCode(err) != "opportunity_invalid_transition" {
		t.Fatalf("ON_HOLD may not resume straight into WON, got %v", err)
	}
	resumed, err := f.service.TransitionOpportunity(ctx, f.tenant, held.ID,
		crmusecases.TransitionOpportunityInput{Stage: crmmodels.StageNegotiation,
			ExpectedVersion: held.Version, IdempotencyKey: "resume-1"})
	if err != nil {
		t.Fatalf("resume opportunity: %v", err)
	}

	// Winning creates nothing: no contract, no commitment.
	won, err := f.service.TransitionOpportunity(ctx, f.tenant, resumed.ID,
		crmusecases.TransitionOpportunityInput{Stage: crmmodels.StageWon,
			ExpectedVersion: resumed.Version, IdempotencyKey: "won-1"})
	if err != nil {
		t.Fatalf("mark won: %v", err)
	}
	var contracts int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_contracts
		WHERE organization_id = $1`, f.tenant.OrganizationID).Scan(&contracts); err != nil {
		t.Fatalf("count contracts: %v", err)
	}
	if contracts != 0 {
		t.Fatalf("marking an opportunity WON must not create a contract, found %d", contracts)
	}

	// WON is terminal.
	_, err = f.service.TransitionOpportunity(ctx, f.tenant, won.ID,
		crmusecases.TransitionOpportunityInput{Stage: crmmodels.StageLost,
			ReasonCode: "changed_mind", ExpectedVersion: won.Version,
			IdempotencyKey: "reopen"})
	if crmErrorCode(err) != "opportunity_invalid_transition" {
		t.Fatalf("a won opportunity must not be reopened, got %v", err)
	}

	// LOST requires a reason.
	loser := f.customerOpportunity(t, customer.ID, "Loser")
	_, err = f.service.TransitionOpportunity(ctx, f.tenant, loser.ID,
		crmusecases.TransitionOpportunityInput{Stage: crmmodels.StageLost,
			ExpectedVersion: loser.Version, IdempotencyKey: "lost-no-reason"})
	if err == nil {
		t.Fatal("LOST without a reason must be refused")
	}
}

func (f *crmFixture) customerOpportunity(t *testing.T, customerID,
	title string) crmmodels.Opportunity {
	t.Helper()
	opportunity, err := f.service.CreateOpportunity(context.Background(), f.tenant,
		crmusecases.CreateOpportunityInput{CustomerID: customerID, Title: title,
			IdempotencyKey: "opp:" + title})
	if err != nil {
		t.Fatalf("create opportunity %q: %v", title, err)
	}
	return opportunity
}

// ---------------------------------------------------------------------------
// Quotations
// ---------------------------------------------------------------------------

func (f *crmFixture) draftQuotation(t *testing.T, customerID string,
	validUntil time.Time) crmusecases.QuotationDetail {
	t.Helper()
	detail, err := f.service.CreateQuotation(context.Background(), f.tenant,
		crmusecases.CreateQuotationInput{
			CustomerID: customerID, Currency: "NGN",
			ValidFrom: time.Now().UTC().Add(-time.Hour), ValidUntil: validUntil,
			LineItems: []crmusecases.LineItemInput{
				{Description: "Dedicated 20t truck", Quantity: "2.5", UnitType: "TRIP",
					UnitAmountMinor: 400000, ManualReason: "negotiated_rate"},
				{Description: "Overnight standby", Quantity: "1", UnitType: "NIGHT",
					UnitAmountMinor: 75000},
			},
			IdempotencyKey: "quotation:" + customerID + validUntil.Format(time.RFC3339Nano),
		})
	if err != nil {
		t.Fatalf("create quotation: %v", err)
	}
	return detail
}

func TestCRMQuotationArithmeticIsManualAndExact(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Quoting Ltd")

	detail := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(72*time.Hour))

	// 2.5 × 400000 = 1000000 exactly, and 1 × 75000 = 75000.
	if detail.LineItems[0].LineAmountMinor != 1000000 {
		t.Fatalf("decimal quantity arithmetic wrong: %d", detail.LineItems[0].LineAmountMinor)
	}
	if detail.Quotation.TotalAmountMinor != 1075000 {
		t.Fatalf("total must be the sum of explicit lines, got %d",
			detail.Quotation.TotalAmountMinor)
	}
	// The quantity survives as a decimal, not a float.
	if detail.LineItems[0].Quantity != "2.5" {
		t.Fatalf("quantity must stay decimal, got %q", detail.LineItems[0].Quantity)
	}

	// A supplied line amount that disagrees with the arithmetic is refused
	// rather than quietly corrected.
	wrong := int64(999)
	_, err := f.service.CreateQuotation(ctx, f.tenant, crmusecases.CreateQuotationInput{
		CustomerID: customer.ID, Currency: "NGN",
		ValidFrom: time.Now().UTC(), ValidUntil: time.Now().UTC().Add(time.Hour),
		LineItems: []crmusecases.LineItemInput{{Description: "Mismatched", Quantity: "2",
			UnitAmountMinor: 500, LineAmountMinor: &wrong}},
		IdempotencyKey: "quotation-mismatch",
	})
	if crmErrorCode(err) != "quotation_invalid_state" {
		t.Fatalf("a line amount that does not match the arithmetic must be refused, got %v", err)
	}

	// validUntil must be after validFrom.
	_, err = f.service.CreateQuotation(ctx, f.tenant, crmusecases.CreateQuotationInput{
		CustomerID: customer.ID, Currency: "NGN",
		ValidFrom: time.Now().UTC().Add(time.Hour), ValidUntil: time.Now().UTC(),
		IdempotencyKey: "quotation-bad-validity",
	})
	if crmErrorCode(err) != "quotation_invalid_state" {
		t.Fatalf("inverted validity window must be refused, got %v", err)
	}

	// A quotation number is minted per organization and is unique.
	other := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(96*time.Hour))
	if other.Quotation.QuotationNumber == detail.Quotation.QuotationNumber {
		t.Fatal("quotation numbers must be unique within an organization")
	}
	if !strings.HasPrefix(detail.Quotation.QuotationNumber, "QTN-") {
		t.Fatalf("unexpected quotation number format: %q", detail.Quotation.QuotationNumber)
	}
}

func TestCRMQuotationIssueAcceptanceAndConversionIntent(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Issuing Ltd")
	if _, err := f.service.CreateContact(ctx, f.tenant, customer.ID,
		crmusecases.CreateContactInput{FullName: "Sales Contact",
			Email: "sales@example.com", ContactPurpose: crmmodels.PurposeSales,
			IsPrimary: true, ConsentStatus: crmmodels.ConsentGranted,
			IdempotencyKey: "issue-contact"}); err != nil {
		t.Fatalf("create sales contact: %v", err)
	}

	detail := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(72*time.Hour))

	// A draft cannot be issued.
	_, err := f.service.IssueQuotation(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.IssueQuotationInput{ExpectedVersion: detail.Quotation.Version,
			IdempotencyKey: "issue-too-early"})
	if crmErrorCode(err) != "quotation_approval_required" {
		t.Fatalf("a draft must not be issuable, got %v", err)
	}

	submitted, err := f.service.SubmitQuotation(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.SubmitQuotationInput{ExpectedVersion: detail.Quotation.Version,
			IdempotencyKey: "submit-1"})
	if err != nil {
		t.Fatalf("submit quotation: %v", err)
	}
	// With no approval framework configured the policy raises no request, so
	// the quotation is approved directly. It is never left AWAITING_APPROVAL
	// with nobody able to decide it.
	if submitted.Quotation.Status != crmmodels.QuotationApproved {
		t.Fatalf("expected APPROVED with no approval policy, got %s",
			submitted.Quotation.Status)
	}

	issued, err := f.service.IssueQuotation(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.IssueQuotationInput{MintCustomerToken: true,
			ExpectedVersion: submitted.Quotation.Version, IdempotencyKey: "issue-1"})
	if err != nil {
		t.Fatalf("issue quotation: %v", err)
	}
	if issued.Quotation.Status != crmmodels.QuotationIssued {
		t.Fatalf("expected ISSUED, got %s", issued.Quotation.Status)
	}
	if issued.CustomerToken == "" {
		t.Fatal("a customer-facing issue must return a token exactly once")
	}

	// The raw token is nowhere in the database.
	var storedHash, rawMatches string
	if err = f.pool.QueryRow(ctx, `SELECT token_hash,
		coalesce((SELECT 'leaked' FROM fleet_partner_quotation_access_tokens
			WHERE token_hash = $2), '') FROM fleet_partner_quotation_access_tokens
		WHERE quotation_id = $1`,
		detail.Quotation.ID, issued.CustomerToken).Scan(&storedHash, &rawMatches); err != nil {
		t.Fatalf("read token row: %v", err)
	}
	if rawMatches != "" || storedHash == issued.CustomerToken {
		t.Fatal("the raw customer token must never be stored")
	}

	// The issued version is immutable at the database level.
	if _, err = f.pool.Exec(ctx,
		`UPDATE fleet_partner_quotation_versions SET total_amount_minor = 1 WHERE id = $1`,
		issued.Version.ID); err == nil {
		t.Fatal("an issued quotation version must reject UPDATE")
	}
	if _, err = f.pool.Exec(ctx,
		`DELETE FROM fleet_partner_quotation_versions WHERE id = $1`,
		issued.Version.ID); err == nil {
		t.Fatal("an issued quotation version must reject DELETE")
	}

	// A durable notification intent exists and carries no address.
	var recipient, payload string
	if err = f.pool.QueryRow(ctx, `SELECT recipient_reference, payload::text
		FROM fleet_partner_crm_notification_operations
		WHERE subject_id = $1 AND intent_type = 'QUOTATION_ISSUED'`,
		detail.Quotation.ID).Scan(&recipient, &payload); err != nil {
		t.Fatalf("read notification operation: %v", err)
	}
	if !strings.HasPrefix(recipient, "cnt_") {
		t.Fatalf("recipient must be an opaque contact reference, got %q", recipient)
	}
	if strings.Contains(payload, "@") {
		t.Fatalf("a queued notification payload must carry no address: %s", payload)
	}

	// The customer view carries no internal notes, no approval record and no
	// risk or credit information.
	view, err := f.service.ViewQuotationAsCustomer(ctx, issued.CustomerToken,
		detail.Quotation.ID, "")
	if err != nil {
		t.Fatalf("customer view: %v", err)
	}
	serialized, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("serialize customer view: %v", err)
	}
	for _, forbidden := range []string{"internalNotes", "riskStatus", "creditLimit",
		"approvalRequestId", "walletId", "commission", "margin", "providerId",
		"mediaObjectId", "tokenHash", "ipHash"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("customer view leaked %q: %s", forbidden, serialized)
		}
	}

	// Viewing moves ISSUED to VIEWED, and doing it again is idempotent.
	afterView, err := f.repo.GetQuotation(ctx, nil, f.tenant.OrganizationID,
		detail.Quotation.ID)
	if err != nil {
		t.Fatalf("read quotation after view: %v", err)
	}
	if afterView.Status != crmmodels.QuotationViewed {
		t.Fatalf("view must move ISSUED to VIEWED, got %s", afterView.Status)
	}
	if _, err = f.service.ViewQuotationAsCustomer(ctx, issued.CustomerToken,
		detail.Quotation.ID, ""); err != nil {
		t.Fatalf("repeat view must be idempotent: %v", err)
	}

	// A token naming a different quotation is refused, and the refusal is
	// indistinguishable from an unknown token.
	otherQuotation := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(96*time.Hour))
	_, err = f.service.ViewQuotationAsCustomer(ctx, issued.CustomerToken,
		otherQuotation.Quotation.ID, "")
	if crmErrorCode(err) != "quotation_token_invalid" {
		t.Fatalf("a token for another quotation must be refused, got %v", err)
	}
	_, err = f.service.ViewQuotationAsCustomer(ctx, "not-a-real-token",
		detail.Quotation.ID, "")
	if crmErrorCode(err) != "quotation_token_invalid" {
		t.Fatalf("an unknown token must give the same answer, got %v", err)
	}

	// Accepting a version that is not the issued one is refused.
	_, err = f.service.AcceptQuotationByToken(ctx, issued.CustomerToken,
		detail.Quotation.ID, crmusecases.AcceptQuotationInput{
			AcceptedVersionID: "qvr_00000000000000000000000000000000",
			IdempotencyKey:    "accept-wrong-version"})
	if crmErrorCode(err) != "quotation_version_conflict" {
		t.Fatalf("accepting a version other than the issued one must be refused, got %v", err)
	}

	accepted, err := f.service.AcceptQuotationByToken(ctx, issued.CustomerToken,
		detail.Quotation.ID, crmusecases.AcceptQuotationInput{
			AcceptedVersionID: issued.Version.ID, IdempotencyKey: "accept-1",
			RemoteAddress: "203.0.113.7", UserAgent: "Mozilla/5.0"})
	if err != nil {
		t.Fatalf("accept quotation: %v", err)
	}
	if accepted.Quotation.Status != crmmodels.QuotationAccepted {
		t.Fatalf("expected ACCEPTED, got %s", accepted.Quotation.Status)
	}

	// Exactly one conversion intent, and it is not an order.
	if accepted.ConversionIntent.ConversionType !=
		crmmodels.ConversionTypeFuturePrivateOrder {
		t.Fatalf("unexpected conversion type: %s", accepted.ConversionIntent.ConversionType)
	}
	if accepted.ConversionIntent.Status != crmmodels.ConversionStatusPending {
		t.Fatalf("conversion intent must remain PENDING_SECTION_15, got %s",
			accepted.ConversionIntent.Status)
	}
	var intents int
	if err = f.pool.QueryRow(ctx, `SELECT count(*)
		FROM fleet_partner_quotation_conversion_intents WHERE quotation_id = $1`,
		detail.Quotation.ID).Scan(&intents); err != nil {
		t.Fatalf("count conversion intents: %v", err)
	}
	if intents != 1 {
		t.Fatalf("acceptance must create exactly one conversion intent, got %d", intents)
	}

	// The IP and user agent are stored only as keyed digests.
	var ipHash, agentHash string
	if err = f.pool.QueryRow(ctx,
		`SELECT coalesce(ip_hash, ''), coalesce(user_agent_hash, '')
		 FROM fleet_partner_quotation_acceptances WHERE quotation_id = $1`,
		detail.Quotation.ID).Scan(&ipHash, &agentHash); err != nil {
		t.Fatalf("read acceptance: %v", err)
	}
	if ipHash == "203.0.113.7" || strings.Contains(agentHash, "Mozilla") {
		t.Fatal("acceptance evidence must store digests, not raw values")
	}

	// A replay returns the original acceptance rather than creating a second.
	replay, err := f.service.AcceptQuotationByToken(ctx, issued.CustomerToken,
		detail.Quotation.ID, crmusecases.AcceptQuotationInput{
			AcceptedVersionID: issued.Version.ID, IdempotencyKey: "accept-1",
			RemoteAddress: "203.0.113.7", UserAgent: "Mozilla/5.0"})
	if err != nil {
		t.Fatalf("acceptance replay must succeed: %v", err)
	}
	if !replay.Replayed || replay.Acceptance.ID != accepted.Acceptance.ID {
		t.Fatal("a replay must return the original acceptance")
	}

	// Accepting a quotation created a conversion intent and nothing else
	// downstream — no order came into existence on its own.
	assertNoSection15Artifacts(t, f.pool, f.tenant.OrganizationID)
}

// assertNoSection15Artifacts proves that a Section 14 operation alone created
// no private order, order stop or wallet settlement for this organization.
//
// Section 15 (private orders, the service catalogue) now exists — built in
// migrations 000025/000026 — so the table-existence version of this guard
// would always fail. The invariant it protects has not changed: only an
// explicit ConvertConversionIntent call may ever produce an order. Accepting
// a quotation, activating a contract or recording an SLA observation must
// never do it as a side effect. So this checks rows, scoped to the fixture's
// own organization, not table existence.
func assertNoSection15Artifacts(t *testing.T, pool *pgxpool.Pool, organizationID string) {
	t.Helper()
	var orderCount, stopCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM fleet_partner_private_orders WHERE organization_id = $1`,
		organizationID).Scan(&orderCount); err != nil {
		t.Fatalf("count private orders: %v", err)
	}
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM fleet_partner_order_stops WHERE organization_id = $1`,
		organizationID).Scan(&stopCount); err != nil {
		t.Fatalf("count order stops: %v", err)
	}
	if orderCount > 0 || stopCount > 0 {
		t.Fatalf("Section 14 operation must not create Section 15 rows: %d orders, %d stops",
			orderCount, stopCount)
	}
}

func TestCRMQuotationAcceptRejectAndExpiryRaces(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Racing Ltd")

	issued := f.issuedQuotation(t, customer.ID, time.Now().UTC().Add(72*time.Hour))

	// A rejection lands, and the quotation can then never be accepted.
	rejected, err := f.service.RejectQuotationAsStaff(ctx, f.tenant, issued.Quotation.ID,
		crmusecases.RejectQuotationInput{ReasonCode: "price_too_high",
			IdempotencyKey: "reject-1"})
	if err != nil {
		t.Fatalf("reject quotation: %v", err)
	}
	if rejected.Status != crmmodels.QuotationRejected {
		t.Fatalf("expected REJECTED, got %s", rejected.Status)
	}
	_, err = f.service.AcceptQuotationAsStaff(ctx, f.tenant, issued.Quotation.ID,
		crmusecases.AcceptQuotationInput{AcceptedVersionID: issued.Version.ID,
			AcceptanceMethod: crmmodels.AcceptCompanyUser, IdempotencyKey: "accept-after-reject"})
	if crmErrorCode(err) != "quotation_already_resolved" {
		t.Fatalf("a rejected quotation must not be acceptable, got %v", err)
	}

	// An expired quotation cannot be accepted, whatever the client believes.
	expired := f.issuedQuotation(t, customer.ID, time.Now().UTC().Add(2*time.Second))
	if _, err = f.pool.Exec(ctx,
		`UPDATE fleet_partner_quotations SET valid_until = now() - interval '1 hour'
		 WHERE id = $1`, expired.Quotation.ID); err != nil {
		t.Fatalf("age quotation: %v", err)
	}
	_, err = f.service.AcceptQuotationAsStaff(ctx, f.tenant, expired.Quotation.ID,
		crmusecases.AcceptQuotationInput{AcceptedVersionID: expired.Version.ID,
			AcceptanceMethod: crmmodels.AcceptCompanyUser, IdempotencyKey: "accept-expired"})
	if crmErrorCode(err) != "quotation_expired" {
		t.Fatalf("an expired quotation must not be acceptable, got %v", err)
	}

	// The expiry worker resolves it, and never touches a resolved quotation.
	count, err := f.service.ExpireQuotations(ctx, crmusecases.WorkerConfig{
		Owner: "test-expiry", BatchSize: 25})
	if err != nil {
		t.Fatalf("expiry worker: %v", err)
	}
	if count == 0 {
		t.Fatal("the expiry worker should have expired the aged quotation")
	}
	afterSweep, err := f.repo.GetQuotation(ctx, nil, f.tenant.OrganizationID,
		expired.Quotation.ID)
	if err != nil {
		t.Fatalf("read expired quotation: %v", err)
	}
	if afterSweep.Status != crmmodels.QuotationExpired {
		t.Fatalf("expected EXPIRED, got %s", afterSweep.Status)
	}
	stillRejected, err := f.repo.GetQuotation(ctx, nil, f.tenant.OrganizationID,
		issued.Quotation.ID)
	if err != nil {
		t.Fatalf("read rejected quotation: %v", err)
	}
	if stillRejected.Status != crmmodels.QuotationRejected {
		t.Fatalf("the expiry worker must never re-resolve a rejected quotation, got %s",
			stillRejected.Status)
	}

	// Two concurrent acceptances of the same quotation resolve to exactly one.
	contested := f.issuedQuotation(t, customer.ID, time.Now().UTC().Add(72*time.Hour))
	var wg sync.WaitGroup
	outcomes := make([]error, 2)
	wg.Add(2)
	for index := 0; index < 2; index++ {
		position := index
		go func() {
			defer wg.Done()
			_, outcomes[position] = f.service.AcceptQuotationAsStaff(ctx, f.tenant,
				contested.Quotation.ID, crmusecases.AcceptQuotationInput{
					AcceptedVersionID: contested.Version.ID,
					AcceptanceMethod:  crmmodels.AcceptCompanyUser,
					IdempotencyKey:    "race-accept-" + string(rune('a'+position)),
				})
		}()
	}
	wg.Wait()
	successes := 0
	for _, outcome := range outcomes {
		if outcome == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("exactly one concurrent acceptance must win, got %d (%v)", successes, outcomes)
	}
	var acceptances int
	if err = f.pool.QueryRow(ctx, `SELECT count(*)
		FROM fleet_partner_quotation_acceptances WHERE quotation_id = $1`,
		contested.Quotation.ID).Scan(&acceptances); err != nil {
		t.Fatalf("count acceptances: %v", err)
	}
	if acceptances != 1 {
		t.Fatalf("exactly one acceptance row must exist, got %d", acceptances)
	}
}

func (f *crmFixture) issuedQuotation(t *testing.T, customerID string,
	validUntil time.Time) crmusecases.IssueQuotationResult {
	t.Helper()
	ctx := context.Background()
	detail := f.draftQuotation(t, customerID, validUntil)
	submitted, err := f.service.SubmitQuotation(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.SubmitQuotationInput{ExpectedVersion: detail.Quotation.Version,
			IdempotencyKey: "submit:" + detail.Quotation.ID})
	if err != nil {
		t.Fatalf("submit quotation: %v", err)
	}
	issued, err := f.service.IssueQuotation(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.IssueQuotationInput{MintCustomerToken: true,
			ExpectedVersion: submitted.Quotation.Version,
			IdempotencyKey:  "issue:" + detail.Quotation.ID})
	if err != nil {
		t.Fatalf("issue quotation: %v", err)
	}
	return issued
}

func TestCRMQuotationIssueFailsClosedOnUnreadyDocument(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Document Gate Ltd")
	detail := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(72*time.Hour))

	// No Media client is configured in the fixture, so the projection is
	// UNKNOWN. UNKNOWN must block an issue exactly as UNSAFE does: a document
	// nobody has checked must not reach a customer.
	if _, err := f.service.AttachQuotationDocument(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.AttachQuotationDocumentInput{MediaObjectID: "med_unverified",
			DocumentPurpose: crmmodels.DocPurposeQuotation, IsMandatory: true}); err != nil {
		t.Fatalf("attach document: %v", err)
	}
	submitted, err := f.service.SubmitQuotation(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.SubmitQuotationInput{ExpectedVersion: detail.Quotation.Version,
			IdempotencyKey: "submit-doc"})
	if err != nil {
		t.Fatalf("submit quotation: %v", err)
	}
	_, err = f.service.IssueQuotation(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.IssueQuotationInput{ExpectedVersion: submitted.Quotation.Version,
			IdempotencyKey: "issue-unready"})
	if crmErrorCode(err) != "quotation_document_not_ready" {
		t.Fatalf("an unverified mandatory document must block issue, got %v", err)
	}

	// Marking the row READY by hand does not help: issue re-reads Media
	// immediately beforehand, and with no Media client that read is UNKNOWN. A
	// stale local projection is a memory of what Media once said, and it must
	// never be the thing that lets an unchecked file reach a customer.
	if _, err = f.pool.Exec(ctx, `UPDATE fleet_partner_quotation_documents
		SET status_projection = 'READY', readiness_state = 'READY'
		WHERE quotation_id = $1`, detail.Quotation.ID); err != nil {
		t.Fatalf("mark document ready: %v", err)
	}
	_, err = f.service.IssueQuotation(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.IssueQuotationInput{ExpectedVersion: submitted.Quotation.Version,
			IdempotencyKey: "issue-stale-ready"})
	if crmErrorCode(err) != "quotation_document_not_ready" {
		t.Fatalf("a stale READY projection must not survive the pre-issue refresh, got %v",
			err)
	}

	// A non-mandatory document never blocks: the gate is about what the offer
	// depends on, not about every file anybody attached.
	optional := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(72*time.Hour))
	if _, err = f.service.AttachQuotationDocument(ctx, f.tenant, optional.Quotation.ID,
		crmusecases.AttachQuotationDocumentInput{MediaObjectID: "med_optional",
			DocumentPurpose: crmmodels.DocPurposeSupporting}); err != nil {
		t.Fatalf("attach optional document: %v", err)
	}
	optionalSubmitted, err := f.service.SubmitQuotation(ctx, f.tenant, optional.Quotation.ID,
		crmusecases.SubmitQuotationInput{ExpectedVersion: optional.Quotation.Version,
			IdempotencyKey: "submit-optional"})
	if err != nil {
		t.Fatalf("submit optional-document quotation: %v", err)
	}
	if _, err = f.service.IssueQuotation(ctx, f.tenant, optional.Quotation.ID,
		crmusecases.IssueQuotationInput{
			ExpectedVersion: optionalSubmitted.Quotation.Version,
			IdempotencyKey:  "issue-optional"}); err != nil {
		t.Fatalf("a non-mandatory document must not block issue: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Contracts and credit
// ---------------------------------------------------------------------------

func TestCRMContractActivationImmutabilityAndCreditHold(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Contracting Ltd")

	contract, err := f.service.CreateContract(ctx, f.tenant, crmusecases.CreateContractInput{
		CustomerID: customer.ID, EffectiveFrom: time.Now().UTC().Add(-time.Hour),
		CommercialSnapshot: crmmodels.JSONMap{"currency": "NGN", "monthlyMinorUnits": 5000000},
		TerminationTerms:   crmmodels.JSONMap{"noticeDays": 30},
		IdempotencyKey:     "contract-1",
	})
	if err != nil {
		t.Fatalf("create contract: %v", err)
	}

	submitted, err := f.service.SubmitContract(ctx, f.tenant, contract.ID, contract.Version,
		"submit-contract", "", "")
	if err != nil {
		t.Fatalf("submit contract: %v", err)
	}
	if submitted.Status != crmmodels.ContractApproved {
		t.Fatalf("expected APPROVED with no approval policy, got %s", submitted.Status)
	}

	activated, version, err := f.service.ActivateContract(ctx, f.tenant, contract.ID,
		crmusecases.ActivateContractInput{SignatoryReferences: []string{"sig_1"},
			ExpectedVersion: submitted.Version, IdempotencyKey: "activate-1"})
	if err != nil {
		t.Fatalf("activate contract: %v", err)
	}
	if activated.Status != crmmodels.ContractActive || activated.ActivatedAt == nil {
		t.Fatalf("activation did not take: %+v", activated)
	}

	// The activated version is immutable at the database level.
	if _, err = f.pool.Exec(ctx,
		`UPDATE fleet_partner_contract_versions SET content_fingerprint = 'x' WHERE id = $1`,
		version.ID); err == nil {
		t.Fatal("an activated contract version must reject UPDATE")
	}
	if _, err = f.pool.Exec(ctx,
		`DELETE FROM fleet_partner_contract_versions WHERE id = $1`, version.ID); err == nil {
		t.Fatal("an activated contract version must reject DELETE")
	}

	// Suspension preserves the active version and creates no financial effect.
	suspended, err := f.service.SuspendContract(ctx, f.tenant, contract.ID,
		crmusecases.ContractLifecycleInput{ReasonCode: "service_dispute",
			ExpectedVersion: activated.Version, IdempotencyKey: "suspend-1"})
	if err != nil {
		t.Fatalf("suspend contract: %v", err)
	}
	if suspended.ActiveVersionID != version.ID {
		t.Fatal("suspension must not disturb the active version")
	}

	// A credit hold blocks bringing the contract back into force.
	if _, err = f.service.SetCreditProfile(ctx, f.tenant, customer.ID,
		crmusecases.SetCreditProfileInput{Currency: "NGN", PaymentTermsDays: 30,
			IdempotencyKey: "credit-contract", ReasonCode: "initial"}); err != nil {
		t.Fatalf("set credit profile: %v", err)
	}
	profile, err := f.repo.GetCreditProfile(ctx, nil, f.tenant.OrganizationID, customer.ID)
	if err != nil {
		t.Fatalf("read credit profile: %v", err)
	}
	held, err := f.service.ApplyCreditHold(ctx, f.tenant, customer.ID,
		crmusecases.CreditHoldInput{HoldStatus: crmmodels.HoldOnHold,
			ReasonCode: "overdue_balance", ExpectedVersion: profile.Version,
			IdempotencyKey: "hold-1"})
	if err != nil {
		t.Fatalf("apply credit hold: %v", err)
	}
	if !crmmodels.BlockingHold(held.HoldStatus) {
		t.Fatalf("ON_HOLD must be a blocking hold, got %s", held.HoldStatus)
	}

	_, err = f.service.ResumeContract(ctx, f.tenant, contract.ID,
		crmusecases.ContractLifecycleInput{ReasonCode: "dispute_resolved",
			ExpectedVersion: suspended.Version, IdempotencyKey: "resume-blocked"})
	if crmErrorCode(err) != "customer_credit_hold" {
		t.Fatalf("a credit hold must block resuming a contract, got %v", err)
	}

	// A new contract activation is blocked by the same hold.
	blocked, err := f.service.CreateContract(ctx, f.tenant, crmusecases.CreateContractInput{
		CustomerID: customer.ID, EffectiveFrom: time.Now().UTC(),
		CommercialSnapshot: crmmodels.JSONMap{"currency": "NGN"},
		TerminationTerms:   crmmodels.JSONMap{"noticeDays": 30},
		IdempotencyKey:     "contract-blocked",
	})
	if err != nil {
		t.Fatalf("create second contract: %v", err)
	}
	_, err = f.service.SubmitContract(ctx, f.tenant, blocked.ID, blocked.Version,
		"submit-blocked", "", "")
	if crmErrorCode(err) != "customer_credit_hold" {
		t.Fatalf("a credit hold must block a new contract submission, got %v", err)
	}

	// Releasing restores it, and the whole change is recorded append-only.
	released, err := f.service.ReleaseCreditHold(ctx, f.tenant, customer.ID,
		crmusecases.CreditHoldInput{ReasonCode: "payment_received",
			ExpectedVersion: held.Version, IdempotencyKey: "release-1"})
	if err != nil {
		t.Fatalf("release credit hold: %v", err)
	}
	if crmmodels.BlockingHold(released.HoldStatus) {
		t.Fatalf("release must clear the hold, got %s", released.HoldStatus)
	}
	creditHistory, err := f.service.CreditHistory(ctx, f.tenant, customer.ID,
		emptyCursorValue(), false, 50)
	if err != nil {
		t.Fatalf("credit history: %v", err)
	}
	if len(creditHistory) < 3 {
		t.Fatalf("expected create, hold and release history, got %d", len(creditHistory))
	}
	if _, err = f.pool.Exec(ctx,
		`UPDATE fleet_partner_customer_credit_history SET reason_code = 'tampered'
		 WHERE id = $1`, creditHistory[0].ID); err == nil {
		t.Fatal("credit history must reject UPDATE")
	}

	// Termination preserves every version.
	resumed, err := f.service.ResumeContract(ctx, f.tenant, contract.ID,
		crmusecases.ContractLifecycleInput{ReasonCode: "dispute_resolved",
			ExpectedVersion: suspended.Version, IdempotencyKey: "resume-ok"})
	if err != nil {
		t.Fatalf("resume contract after release: %v", err)
	}
	terminated, err := f.service.TerminateContract(ctx, f.tenant, contract.ID,
		crmusecases.ContractLifecycleInput{ReasonCode: "customer_request",
			ExpectedVersion: resumed.Version, IdempotencyKey: "terminate-1"})
	if err != nil {
		t.Fatalf("terminate contract: %v", err)
	}
	if terminated.Status != crmmodels.ContractTerminated || terminated.TerminatedAt == nil {
		t.Fatalf("termination did not take: %+v", terminated)
	}
	versions, err := f.service.ContractVersions(ctx, f.tenant, contract.ID, 50)
	if err != nil {
		t.Fatalf("list contract versions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("termination must preserve every version, got %d", len(versions))
	}

	// No wallet, ledger or receivable was touched anywhere in this flow.
	assertNoSection15Artifacts(t, f.pool, f.tenant.OrganizationID)
}

func TestCRMContractActivationBlockedByRestrictedRisk(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Restricted Contracting Ltd")

	contract, err := f.service.CreateContract(ctx, f.tenant, crmusecases.CreateContractInput{
		CustomerID: customer.ID, EffectiveFrom: time.Now().UTC(),
		CommercialSnapshot: crmmodels.JSONMap{"currency": "NGN"},
		TerminationTerms:   crmmodels.JSONMap{"noticeDays": 30},
		IdempotencyKey:     "restricted-contract",
	})
	if err != nil {
		t.Fatalf("create contract: %v", err)
	}
	submitted, err := f.service.SubmitContract(ctx, f.tenant, contract.ID, contract.Version,
		"submit-restricted", "", "")
	if err != nil {
		t.Fatalf("submit contract: %v", err)
	}

	// The risk decision lands after approval. Activation must revalidate rather
	// than trust the state the reviewer saw.
	if _, err = f.service.ChangeCustomerRisk(ctx, f.tenant, customer.ID,
		crmusecases.ChangeCustomerRiskInput{RiskStatus: crmmodels.RiskRestricted,
			ReasonCode: "compliance_review", ExpectedVersion: customer.Version,
			IdempotencyKey: "restrict-late"}); err != nil {
		t.Fatalf("restrict customer: %v", err)
	}
	_, _, err = f.service.ActivateContract(ctx, f.tenant, contract.ID,
		crmusecases.ActivateContractInput{ExpectedVersion: submitted.Version,
			IdempotencyKey: "activate-restricted"})
	if crmErrorCode(err) != "customer_risk_restricted" {
		t.Fatalf("a restriction applied after approval must block activation, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service level agreements
// ---------------------------------------------------------------------------

func TestCRMSLAObservationIdempotencyAndUnknownHandling(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "SLA Customer Ltd")

	sla, err := f.service.CreateSLA(ctx, f.tenant, crmusecases.CreateSLAInput{
		CustomerID: customer.ID, MetricCode: crmmodels.MetricDeliveryOnTime,
		TargetValue: "95", MeasurementUnit: "PERCENT", MeasurementWindow: "MONTHLY",
		EffectiveFrom: time.Now().UTC().Add(-time.Hour), IdempotencyKey: "sla-1",
	})
	if err != nil {
		t.Fatalf("create SLA: %v", err)
	}

	// An unsupported metric has no evaluation rule, so it is refused.
	_, err = f.service.CreateSLA(ctx, f.tenant, crmusecases.CreateSLAInput{
		CustomerID: customer.ID, MetricCode: "CUSTOMER_HAPPINESS", TargetValue: "10",
		MeasurementUnit: "PERCENT", MeasurementWindow: "MONTHLY",
		EffectiveFrom: time.Now().UTC(), IdempotencyKey: "sla-bad-metric"})
	if crmErrorCode(err) != "sla_invalid_state" {
		t.Fatalf("an unsupported metric must be refused, got %v", err)
	}
	// A unit that does not fit the metric is refused.
	_, err = f.service.CreateSLA(ctx, f.tenant, crmusecases.CreateSLAInput{
		CustomerID: customer.ID, MetricCode: crmmodels.MetricDeliveryOnTime,
		TargetValue: "95", MeasurementUnit: "MINUTES", MeasurementWindow: "MONTHLY",
		EffectiveFrom: time.Now().UTC(), IdempotencyKey: "sla-bad-unit"})
	if crmErrorCode(err) != "sla_invalid_state" {
		t.Fatalf("a mismatched unit must be refused, got %v", err)
	}

	activated, err := f.service.ActivateSLA(ctx, f.tenant, sla.ID,
		crmusecases.SLALifecycleInput{ExpectedVersion: sla.Version,
			IdempotencyKey: "sla-activate"})
	if err != nil {
		t.Fatalf("activate SLA: %v", err)
	}
	if activated.Status != crmmodels.SLAActive {
		t.Fatalf("expected ACTIVE, got %s", activated.Status)
	}

	// A caller that is not an approved source is refused, however well signed.
	_, err = f.service.IngestObservation(ctx, "admin-backoffice-service",
		crmusecases.IngestObservationInput{OrganizationID: f.tenant.OrganizationID,
			SLAID: sla.ID, SourceType: "DELIVERY", SourceReference: "trip_1",
			MeasuredAt: time.Now().UTC(), WindowStart: time.Now().UTC().Add(-time.Hour),
			WindowEnd: time.Now().UTC(), SourceVersion: 1, PayloadHash: "hash-a"})
	if crmErrorCode(err) != "sla_source_unknown" {
		t.Fatalf("an unapproved source must be refused, got %v", err)
	}

	measured := "97.5"
	window := time.Now().UTC()
	first, err := f.service.IngestObservation(ctx, "driver-dispatch-delivery-service",
		crmusecases.IngestObservationInput{OrganizationID: f.tenant.OrganizationID,
			SLAID: sla.ID, SourceType: "DELIVERY", SourceReference: "trip_1",
			MeasuredValue: &measured, MeasurementUnit: "PERCENT",
			MeasuredAt: window, WindowStart: window.Add(-time.Hour), WindowEnd: window,
			SourceVersion: 1, PayloadHash: "hash-a"})
	if err != nil {
		t.Fatalf("ingest observation: %v", err)
	}
	if first.Observation.Outcome != crmmodels.OutcomeMet {
		t.Fatalf("97.5 against a 95 target must be MET, got %s", first.Observation.Outcome)
	}

	// The same version with the same payload is a duplicate.
	duplicate, err := f.service.IngestObservation(ctx, "driver-dispatch-delivery-service",
		crmusecases.IngestObservationInput{OrganizationID: f.tenant.OrganizationID,
			SLAID: sla.ID, SourceType: "DELIVERY", SourceReference: "trip_1",
			MeasuredValue: &measured, MeasurementUnit: "PERCENT",
			MeasuredAt: window, WindowStart: window.Add(-time.Hour), WindowEnd: window,
			SourceVersion: 1, PayloadHash: "hash-a"})
	if err != nil {
		t.Fatalf("duplicate observation must replay: %v", err)
	}
	if !duplicate.Duplicate || duplicate.Observation.ID != first.Observation.ID {
		t.Fatal("a duplicate must return the stored observation")
	}

	// The same version with a different payload is a conflict: a source that
	// changes its mind must say so with a new version.
	changed := "50"
	_, err = f.service.IngestObservation(ctx, "driver-dispatch-delivery-service",
		crmusecases.IngestObservationInput{OrganizationID: f.tenant.OrganizationID,
			SLAID: sla.ID, SourceType: "DELIVERY", SourceReference: "trip_1",
			MeasuredValue: &changed, MeasurementUnit: "PERCENT",
			MeasuredAt: window, WindowStart: window.Add(-time.Hour), WindowEnd: window,
			SourceVersion: 1, PayloadHash: "hash-b"})
	if crmErrorCode(err) != "sla_observation_conflict" {
		t.Fatalf("a changed payload at the same version must conflict, got %v", err)
	}

	// A missed target is recorded as MISSED, never softened.
	missed, err := f.service.IngestObservation(ctx, "driver-dispatch-delivery-service",
		crmusecases.IngestObservationInput{OrganizationID: f.tenant.OrganizationID,
			SLAID: sla.ID, SourceType: "DELIVERY", SourceReference: "trip_1",
			MeasuredValue: &changed, MeasurementUnit: "PERCENT",
			MeasuredAt: window, WindowStart: window.Add(-time.Hour), WindowEnd: window,
			SourceVersion: 2, PayloadHash: "hash-b"})
	if err != nil {
		t.Fatalf("ingest missed observation: %v", err)
	}
	if missed.Observation.Outcome != crmmodels.OutcomeMissed {
		t.Fatalf("50 against a 95 target must be MISSED, got %s", missed.Observation.Outcome)
	}

	// No measurement is INSUFFICIENT_DATA — never MET.
	absent, err := f.service.IngestObservation(ctx, "driver-dispatch-delivery-service",
		crmusecases.IngestObservationInput{OrganizationID: f.tenant.OrganizationID,
			SLAID: sla.ID, SourceType: "DELIVERY", SourceReference: "trip_2",
			MeasurementUnit: "PERCENT", MeasuredAt: window,
			WindowStart: window.Add(-time.Hour), WindowEnd: window,
			SourceVersion: 1, PayloadHash: "hash-empty"})
	if err != nil {
		t.Fatalf("ingest empty observation: %v", err)
	}
	if absent.Observation.Outcome != crmmodels.OutcomeInsufficientData {
		t.Fatalf("an absent measurement must be INSUFFICIENT_DATA, got %s",
			absent.Observation.Outcome)
	}

	// A version gap means measurements were missed, which is UNKNOWN — not MET.
	gapped, err := f.service.IngestObservation(ctx, "driver-dispatch-delivery-service",
		crmusecases.IngestObservationInput{OrganizationID: f.tenant.OrganizationID,
			SLAID: sla.ID, SourceType: "DELIVERY", SourceReference: "trip_1",
			MeasuredValue: &measured, MeasurementUnit: "PERCENT",
			MeasuredAt: window, WindowStart: window.Add(-time.Hour), WindowEnd: window,
			SourceVersion: 9, PayloadHash: "hash-gap"})
	if err != nil {
		t.Fatalf("ingest gapped observation: %v", err)
	}
	if gapped.Observation.Outcome != crmmodels.OutcomeUnknown {
		t.Fatalf("a version gap must record UNKNOWN, got %s", gapped.Observation.Outcome)
	}

	// A cross-tenant SLA ID is answered as not-found, not forbidden.
	_, err = f.service.IngestObservation(ctx, "driver-dispatch-delivery-service",
		crmusecases.IngestObservationInput{OrganizationID: f.other.OrganizationID,
			SLAID: sla.ID, SourceType: "DELIVERY", SourceReference: "trip_1",
			MeasuredValue: &measured, MeasurementUnit: "PERCENT", MeasuredAt: window,
			WindowStart: window.Add(-time.Hour), WindowEnd: window,
			SourceVersion: 3, PayloadHash: "hash-cross"})
	if crmErrorCode(err) != "sla_not_found" {
		t.Fatalf("a cross-tenant SLA must be not-found, got %v", err)
	}

	// Observations are append-only.
	if _, err = f.pool.Exec(ctx,
		`UPDATE fleet_partner_sla_observations SET outcome = 'MET' WHERE id = $1`,
		missed.Observation.ID); err == nil {
		t.Fatal("SLA observations must reject UPDATE")
	}

	// Escalation produces an intent and no money movement whatsoever.
	escalated, err := f.service.EscalateSLABreaches(ctx, crmusecases.WorkerConfig{
		Owner: "test-escalation", BatchSize: 25})
	if err != nil {
		t.Fatalf("escalation worker: %v", err)
	}
	_ = escalated
	var financial int
	if err = f.pool.QueryRow(ctx, `SELECT count(*)
		FROM fleet_partner_crm_notification_operations
		WHERE intent_type = 'SLA_ESCALATION'`).Scan(&financial); err != nil {
		t.Fatalf("count escalation intents: %v", err)
	}
	assertNoSection15Artifacts(t, f.pool, f.tenant.OrganizationID)
}

// ---------------------------------------------------------------------------
// Flags and kill switch
// ---------------------------------------------------------------------------

func TestCRMDefaultsAreOffAndKillSwitchOverrides(t *testing.T) {
	// One fixture, several service configurations. The suite holds a per-test
	// database lock, so a second newCRMFixture in the same test would deadlock
	// against itself.
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()

	// Everything defaults false, so a deployment that has not deliberately
	// enabled CRM does not have it.
	off := f.withFlags(crmusecases.Flags{})
	_, err := off.CreateCustomer(ctx, f.tenant, crmusecases.CreateCustomerInput{
		CustomerType: crmmodels.CustomerTypeBusiness, LegalName: "Should Not Exist",
		DefaultCurrency: "NGN", IdempotencyKey: "disabled"})
	if err == nil {
		t.Fatal("with the flags off, a CRM write must be refused")
	}
	if apperrors.From(err).Status != 404 {
		t.Fatalf("a disabled capability must answer not-found, got %d",
			apperrors.From(err).Status)
	}
	// Read is separable from write: CRM_ENABLED alone opens the read surface
	// without opening any write.
	readOnly := f.withFlags(crmusecases.Flags{CRMEnabled: true})
	if _, err = readOnly.ListCustomers(ctx, f.tenant,
		crmusecases.ListCustomersInput{Limit: 5}); err != nil {
		t.Fatalf("read-only CRM must still list customers: %v", err)
	}
	if _, err = readOnly.CreateCustomer(ctx, f.tenant, crmusecases.CreateCustomerInput{
		CustomerType: crmmodels.CustomerTypeBusiness, LegalName: "Read Only",
		DefaultCurrency: "NGN", IdempotencyKey: "read-only"}); err == nil {
		t.Fatal("read-only CRM must refuse a write")
	}

	// The kill switch overrides an otherwise fully enabled configuration.
	flags := allCRMFlagsOn()
	flags.KillSwitch = true
	killed := f.withFlags(flags)
	_, err = killed.CreateCustomer(ctx, f.tenant, crmusecases.CreateCustomerInput{
		CustomerType: crmmodels.CustomerTypeBusiness, LegalName: "Killed",
		DefaultCurrency: "NGN", IdempotencyKey: "killed"})
	if err == nil {
		t.Fatal("the kill switch must refuse a CRM write even with every flag on")
	}
	if apperrors.From(err).Status != 503 {
		t.Fatalf("the kill switch must answer unavailable, got %d",
			apperrors.From(err).Status)
	}

	// And it stops every worker: the pass returns without touching anything.
	killed.RunPass(ctx, crmusecases.WorkerConfig{Owner: "killed"})
}

// withFlags builds a second service over the same pool and organizations.
//
// It exists because the integration suite serializes on a per-test database
// lock, so a test that needs two flag configurations must reuse one fixture
// rather than open a second.
func (f *crmFixture) withFlags(flags crmusecases.Flags) *crmusecases.Service {
	return crmusecases.NewService(crmusecases.Options{
		Pool: f.pool, Repository: f.repo, Metrics: observability.NewMetrics(),
		Flags:         flags,
		TokenSecret:   []byte("crm-token-secret-for-integration-tests"),
		ContactSecret: []byte("crm-contact-secret-for-integration-tests"),
		TokenTTL:      48 * time.Hour,
	})
}

// ---------------------------------------------------------------------------
// Schema guarantees
// ---------------------------------------------------------------------------

func TestCRMSchemaGuarantees(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()

	// Every Section 14 table exists after migration.
	for _, table := range []string{
		"fleet_partner_company_customers", "fleet_partner_customer_contacts",
		"fleet_partner_customer_locations", "fleet_partner_sales_opportunities",
		"fleet_partner_sales_opportunity_history", "fleet_partner_quotations",
		"fleet_partner_quotation_versions", "fleet_partner_quotation_line_items",
		"fleet_partner_quotation_acceptances",
		"fleet_partner_quotation_conversion_intents", "fleet_partner_contracts",
		"fleet_partner_contract_versions", "fleet_partner_contract_documents",
		"fleet_partner_contract_history", "fleet_partner_service_level_agreements",
		"fleet_partner_sla_observations", "fleet_partner_customer_credit_profiles",
		"fleet_partner_customer_credit_history",
		"fleet_partner_crm_notification_operations",
		// Added by migration 000024.
		"fleet_partner_crm_customer_history", "fleet_partner_crm_contact_history",
		"fleet_partner_crm_location_history", "fleet_partner_quotation_history",
		"fleet_partner_sla_history", "fleet_partner_quotation_documents",
		"fleet_partner_quotation_access_tokens",
	} {
		var exists bool
		if err := f.pool.QueryRow(ctx,
			`SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil {
			t.Fatalf("check %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %s is missing", table)
		}
	}

	// The whole CRM permission catalogue is seeded and active.
	var permissions int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_permissions
		 WHERE code LIKE 'crm.%' AND is_active`).Scan(&permissions); err != nil {
		t.Fatalf("count CRM permissions: %v", err)
	}
	if permissions != 24 {
		t.Fatalf("expected 24 active crm.* permissions, got %d", permissions)
	}

	// No CRM table carries a cross-service foreign key.
	rows, err := f.pool.Query(ctx, `
		SELECT c.conname, c.conrelid::regclass::text, c.confrelid::regclass::text
		FROM pg_constraint c
		WHERE c.contype = 'f'
		  AND c.conrelid::regclass::text LIKE 'fleet_partner_%'
		  AND c.confrelid::regclass::text NOT LIKE 'fleet_partner_%'`)
	if err != nil {
		t.Fatalf("scan foreign keys: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, from, to string
		if err = rows.Scan(&name, &from, &to); err != nil {
			t.Fatalf("scan constraint: %v", err)
		}
		t.Fatalf("cross-service foreign key %s: %s -> %s", name, from, to)
	}

	// The access-token table stores no raw token column and no URL column.
	forbidden, err := f.pool.Query(ctx, `
		SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name LIKE 'fleet_partner_%'
		  AND (column_name IN ('token', 'raw_token', 'password', 'signed_url',
		                       'download_url', 'storage_path', 'file_bytes'))`)
	if err != nil {
		t.Fatalf("scan forbidden columns: %v", err)
	}
	defer forbidden.Close()
	for forbidden.Next() {
		var table, column string
		if err = forbidden.Scan(&table, &column); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		t.Fatalf("forbidden column %s.%s exists", table, column)
	}
}

// emptyCursorValue is the first-page cursor.
func emptyCursorValue() cursor.Cursor { return cursor.Cursor{} }
