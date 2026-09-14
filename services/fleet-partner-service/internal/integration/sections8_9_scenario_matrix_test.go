package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/providercompat"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fleetpartner"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Sections 8/9 cross-service certification.
//
// These tests drive Fleet's REAL completed-work ingestion against a real
// database, using the exact canonical DTO the Taxi and Dispatch outboxes
// submit. They certify the part of scenarios A-J that is new in Sections 8/9:
// that an organization-managed completion from either execution service
// reaches Fleet exactly once, that an independent provider is unaffected, and
// that a dependency failure cannot produce a duplicate compensation input.

// sectionsFlags is the configuration Sections 8/9 actually ship in:
// completed-work projection ON, compensation resolver and settlement
// instruction OFF (they belong to Sections 12/13).
func sectionsFlags() fleetpartner.Flags {
	return fleetpartner.Flags{
		OperatingModeEnforcement: true,
		CompletedWorkProjection:  true,
		CompensationResolver:     false,
		SettlementInstruction:    false,
		TypedFinancialVisibility: true,
	}
}

type membershipStub struct{}

func (membershipStub) FindLiveByProvider(context.Context, string, string) (fleetpartner.ProviderMembership, error) {
	return fleetpartner.ProviderMembership{}, fleetpartner.ErrNotFound
}

func newCompletedWorkService(t *testing.T, pool *pgxpool.Pool, flags fleetpartner.Flags) *fleetpartner.Service {
	t.Helper()
	return fleetpartner.NewService(pool, fleetpartner.NewRepository(pool), membershipStub{}, nil, nil, flags)
}

// seedManagedMembership creates an organization with one ACTIVE managed
// provider membership for the given service category, and returns the
// organization, membership and provider identifiers.
func seedManagedMembership(t *testing.T, pool *pgxpool.Pool, category string) (orgID, membershipID, providerID string) {
	t.Helper()
	ctx := context.Background()

	org := createOrganization(t, pool, "Sections89 "+category+" Ltd", ids.New("rc"))
	orgID = org.ID
	membershipID = ids.New("mem")
	providerID = ids.New("prov")

	providerService := map[string]string{
		"TAXI":     "driver-taxi-service",
		"DISPATCH": "driver-dispatch-delivery-service",
		"HAULING":  "driver-hauling-service",
	}[category]

	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_organization_provider_memberships
			(id, organization_id, provider_service, service_category, provider_id,
			 onboarding_source, relationship_type, status, joined_at)
		VALUES ($1,$2,$3,$4,$5,'EXISTING_PROVIDER_LINKED','CONTRACTOR','ACTIVE',now())`,
		membershipID, orgID, providerService, category, providerID); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return orgID, membershipID, providerID
}

func canonicalRequest(orgID, membershipID, providerID, source, jobID string) providercompat.CompletedWorkRequest {
	return providercompat.CompletedWorkRequest{
		SourceService: source, SourceJobID: jobID, SourceAssignmentID: "asg-" + jobID,
		ProviderID: providerID, MembershipID: membershipID, OrganizationID: orgID,
		CompletedAt: time.Now().UTC().Truncate(time.Second), Currency: "NGN",
		EligibleBasisMinor: 450000, SourceVersion: 3,
		IdempotencyKey: source + "-completed-work:" + jobID + ":3",
		CorrelationID:  "corr-" + jobID,
	}
}

func asInput(request providercompat.CompletedWorkRequest) fleetpartner.CompletedWorkInput {
	return fleetpartner.CompletedWorkInput{
		SourceService: request.SourceService, SourceJobID: request.SourceJobID,
		SourceAssignmentID: request.SourceAssignmentID, ProviderID: request.ProviderID,
		MembershipID: request.MembershipID, OrganizationID: request.OrganizationID,
		CompletedAt: request.CompletedAt, Currency: request.Currency,
		EligibleBasisMinor: request.EligibleBasisMinor, SourceVersion: request.SourceVersion,
		IdempotencyKey: request.IdempotencyKey, CorrelationID: request.CorrelationID,
	}
}

func countCompletedWork(t *testing.T, pool *pgxpool.Pool, jobID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM fleet_partner_completed_work_records WHERE source_job_id=$1`, jobID).
		Scan(&count); err != nil {
		t.Fatalf("count completed work: %v", err)
	}
	return count
}

// Scenarios B/C/D (corporate Taxi) and G/H (corporate Dispatch): a managed
// completion from either execution service is recorded by Fleet's canonical
// ingestion, exactly once, using the shared DTO.
func TestSections89_ManagedCompletionIsRecordedFromBothSources(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())

	for _, source := range []string{providercompat.SourceServiceTaxi, providercompat.SourceServiceDispatch} {
		t.Run(source, func(t *testing.T) {
			orgID, membershipID, providerID := seedManagedMembership(t, pool, source)
			caller := map[string]string{
				providercompat.SourceServiceTaxi:     "driver-taxi-service",
				providercompat.SourceServiceDispatch: "driver-dispatch-delivery-service",
			}[source]

			request := canonicalRequest(orgID, membershipID, providerID, source, ids.New("job"))
			if err := request.Validate(); err != nil {
				t.Fatalf("the submitted DTO is not canonical: %v", err)
			}

			record, _, _, err := service.RecordCompletedWork(context.Background(), caller, asInput(request))
			if err != nil {
				t.Fatalf("ingestion failed: %v", err)
			}
			if record.ID == "" {
				t.Fatal("no completed-work record was returned")
			}
			if record.SourceService != source || record.OrganizationID != orgID ||
				record.MembershipID != membershipID || record.ProviderID != providerID {
				t.Fatalf("record does not carry the canonical binding: %+v", record)
			}
			if record.EligibleBasisMinor != 450000 {
				t.Fatalf("eligibleBasisMinor = %d", record.EligibleBasisMinor)
			}
			if got := countCompletedWork(t, pool, request.SourceJobID); got != 1 {
				t.Fatalf("completed-work rows = %d, want exactly 1", got)
			}
		})
	}
}

// Scenario J (dependency failure): Fleet may commit and then lose the
// response, so the execution service redelivers under the SAME idempotency
// key. Fleet must return the existing record, never create a second one --
// otherwise Section 12 would see two compensation inputs for one job.
func TestSections89_RedeliveryProducesExactlyOneRecord(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())
	orgID, membershipID, providerID := seedManagedMembership(t, pool, providercompat.SourceServiceDispatch)

	request := canonicalRequest(orgID, membershipID, providerID, providercompat.SourceServiceDispatch, ids.New("job"))
	ctx := context.Background()

	first, _, _, err := service.RecordCompletedWork(ctx, "driver-dispatch-delivery-service", asInput(request))
	if err != nil {
		t.Fatalf("first submission failed: %v", err)
	}
	// Three more redeliveries, exactly as the outbox would perform them after
	// an ambiguous response.
	for i := 0; i < 3; i++ {
		replay, _, _, err := service.RecordCompletedWork(ctx, "driver-dispatch-delivery-service", asInput(request))
		if err != nil {
			t.Fatalf("redelivery %d failed: %v", i, err)
		}
		if replay.ID != first.ID {
			t.Fatalf("redelivery %d created a different record: %s vs %s", i, replay.ID, first.ID)
		}
	}
	if got := countCompletedWork(t, pool, request.SourceJobID); got != 1 {
		t.Fatalf("completed-work rows after redelivery = %d, want exactly 1", got)
	}
}

// A retry that reuses the key with DIFFERENT content is a conflict, not a
// silent overwrite: the first authoritative completion stands.
func TestSections89_SameKeyDifferentContentIsRejected(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())
	orgID, membershipID, providerID := seedManagedMembership(t, pool, providercompat.SourceServiceTaxi)
	ctx := context.Background()

	request := canonicalRequest(orgID, membershipID, providerID, providercompat.SourceServiceTaxi, ids.New("job"))
	if _, _, _, err := service.RecordCompletedWork(ctx, "driver-taxi-service", asInput(request)); err != nil {
		t.Fatalf("first submission failed: %v", err)
	}

	tampered := request
	tampered.EligibleBasisMinor = 999999
	_, _, _, err := service.RecordCompletedWork(ctx, "driver-taxi-service", asInput(tampered))
	if err == nil {
		t.Fatal("a reused idempotency key with different content must be rejected")
	}
	if got := countCompletedWork(t, pool, request.SourceJobID); got != 1 {
		t.Fatalf("completed-work rows = %d, want exactly 1", got)
	}
}

// Section 8/9 ships with the compensation resolver and settlement instruction
// OFF. Ingestion must still succeed, and must produce NO compensation or
// settlement side effect -- that is the whole point of decoupling the flags.
func TestSections89_IngestionWorksWithResolverAndSettlementOff(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())
	orgID, membershipID, providerID := seedManagedMembership(t, pool, providercompat.SourceServiceTaxi)
	ctx := context.Background()

	request := canonicalRequest(orgID, membershipID, providerID, providercompat.SourceServiceTaxi, ids.New("job"))
	record, resolution, instruction, err := service.RecordCompletedWork(ctx, "driver-taxi-service", asInput(request))
	if err != nil {
		t.Fatalf("ingestion must succeed with the Sections 8/9 flag set: %v", err)
	}
	if record.ID == "" {
		t.Fatal("no record was created")
	}
	if resolution.ID != "" {
		t.Fatal("a compensation resolution was produced while the resolver is disabled")
	}
	if instruction.InstructionID != "" {
		t.Fatal("a settlement instruction was produced while it is disabled")
	}
	var resolutions, instructions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_compensation_resolutions`).Scan(&resolutions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_managed_work_settlement_instructions`).Scan(&instructions); err != nil {
		t.Fatal(err)
	}
	if resolutions != 0 || instructions != 0 {
		t.Fatalf("resolver/settlement side effects leaked: resolutions=%d instructions=%d", resolutions, instructions)
	}
}

// Projection off means ingestion is refused outright -- the rollout gate is
// real, not decorative.
func TestSections89_ProjectionDisabledRefusesIngestion(t *testing.T) {
	pool := testPool(t)
	flags := sectionsFlags()
	flags.CompletedWorkProjection = false
	service := newCompletedWorkService(t, pool, flags)
	orgID, membershipID, providerID := seedManagedMembership(t, pool, providercompat.SourceServiceTaxi)

	request := canonicalRequest(orgID, membershipID, providerID, providercompat.SourceServiceTaxi, ids.New("job"))
	_, _, _, err := service.RecordCompletedWork(context.Background(), "driver-taxi-service", asInput(request))
	if err == nil {
		t.Fatal("ingestion must be refused while the projection flag is off")
	}
	if got := countCompletedWork(t, pool, request.SourceJobID); got != 0 {
		t.Fatalf("completed-work rows = %d, want 0", got)
	}
}

// Authorization: an execution service may only report work for the source it
// owns, for a provider that actually holds the membership, in the
// organization that owns it. Each of these is a TERMINAL rejection in the
// outbox classification, not a retry.
func TestSections89_IngestionAuthorization(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())
	orgID, membershipID, providerID := seedManagedMembership(t, pool, providercompat.SourceServiceTaxi)
	ctx := context.Background()

	t.Run("caller does not own the source service", func(t *testing.T) {
		request := canonicalRequest(orgID, membershipID, providerID, providercompat.SourceServiceTaxi, ids.New("job"))
		_, _, _, err := service.RecordCompletedWork(ctx, "driver-dispatch-delivery-service", asInput(request))
		if err == nil {
			t.Fatal("dispatch must not be able to report taxi work")
		}
		if apperrors.From(err).Status != 403 {
			t.Fatalf("status = %d, want 403", apperrors.From(err).Status)
		}
	})

	t.Run("wrong provider for the membership", func(t *testing.T) {
		request := canonicalRequest(orgID, membershipID, "some-other-provider",
			providercompat.SourceServiceTaxi, ids.New("job"))
		if _, _, _, err := service.RecordCompletedWork(ctx, "driver-taxi-service", asInput(request)); err == nil {
			t.Fatal("a provider that does not hold the membership must be rejected")
		}
	})

	t.Run("wrong organization", func(t *testing.T) {
		other := createOrganization(t, pool, "Other Org Ltd", ids.New("rc"))
		request := canonicalRequest(other.ID, membershipID, providerID,
			providercompat.SourceServiceTaxi, ids.New("job"))
		if _, _, _, err := service.RecordCompletedWork(ctx, "driver-taxi-service", asInput(request)); err == nil {
			t.Fatal("a membership in a different organization must be rejected")
		}
	})

	t.Run("FLEET and VENDOR are not execution sources", func(t *testing.T) {
		for _, source := range []string{"FLEET", "VENDOR"} {
			request := canonicalRequest(orgID, membershipID, providerID, source, ids.New("job"))
			if _, _, _, err := service.RecordCompletedWork(ctx, "driver-taxi-service", asInput(request)); err == nil {
				t.Fatalf("%s must not be accepted as an execution source", source)
			}
		}
	})
}

// Scenarios A/F (independent Taxi and Dispatch): an independent provider has
// no Fleet membership, so the canonical corporate context reports them
// unmanaged with a conservative policy -- and no completed work is ever
// reported for them. Nothing about the independent flow depends on Fleet.
func TestSections89_IndependentProviderIsUnaffected(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())

	for _, category := range []string{providercompat.SourceServiceTaxi, providercompat.SourceServiceDispatch} {
		t.Run(category, func(t *testing.T) {
			corporate, err := service.CorporateContext(context.Background(), ids.New("independent"), category)
			if err != nil {
				t.Fatalf("corporate context for an independent provider must not error: %v", err)
			}
			if corporate.IsManagedProvider {
				t.Fatal("an unknown provider must be reported as independent")
			}
			if corporate.OrganizationID != "" || corporate.MembershipID != "" {
				t.Fatalf("independent context leaked organization data: %+v", corporate)
			}
			if corporate.ServiceEligibility[category] {
				t.Fatal("an independent provider must not be reported as eligible for managed work")
			}
		})
	}
}
