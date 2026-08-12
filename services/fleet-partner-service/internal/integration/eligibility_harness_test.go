package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	fleetevents "cosmicforge/logistics/services/fleet-partner-service/internal/events"
	eligibilityrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// The eligibility test harness.
//
// Two things are needed to exercise a gated operation honestly: a real
// eligibility service (never a stub of the gate itself — that would test
// nothing) and a stub of the AUTHORITIES the gate consults, because those live
// in another service. The stub is deliberately explicit about every state it
// can report, so a test can make a provider unverified or a vehicle restricted
// without reaching into fleet's own tables to fake something fleet does not own.

// stubAuthority stands in for the vehicle and provider authority.
type stubAuthority struct {
	configured bool

	vehicle    platformclients.VehicleAuthorityState
	vehicleErr error

	provider    platformclients.ProviderAuthorityState
	providerErr error

	vehicleCalls  int
	providerCalls int
}

func (a *stubAuthority) LookupVehicle(_ context.Context, _ platformclients.CallMeta,
	lookup platformclients.VehicleLookup) (platformclients.VehicleAuthorityState, error) {
	a.vehicleCalls++
	if a.vehicleErr != nil {
		return platformclients.VehicleAuthorityState{}, a.vehicleErr
	}
	// Echo the identifiers asked about unless the test pinned them. A stub
	// that answered about a different vehicle than the one requested would
	// make the ownership check pass for the wrong reason.
	answer := a.vehicle
	if answer.VehicleID == "" {
		answer.VehicleID = lookup.VehicleID
	}
	if answer.ProviderID == "" {
		answer.ProviderID = lookup.ExpectedProviderID
	}
	return answer, nil
}

func (a *stubAuthority) LookupProvider(_ context.Context, _ platformclients.CallMeta,
	lookup platformclients.ProviderLookup) (platformclients.ProviderAuthorityState, error) {
	a.providerCalls++
	if a.providerErr != nil {
		return platformclients.ProviderAuthorityState{}, a.providerErr
	}
	answer := a.provider
	if answer.ProviderID == "" {
		answer.ProviderID = lookup.ProviderID
	}
	return answer, nil
}

func (a *stubAuthority) AuthorityService() string {
	return "driver-dispatch-delivery-service"
}

func (a *stubAuthority) Configured() bool { return a.configured }

// healthyAuthority reports a provider and vehicle that pass every check.
//
// ProviderMatch and ServiceTypeMatch are pointers to true rather than absent,
// because absent means "the authority did not answer that question" and the
// ownership check treats an unanswered question as a failure.
func healthyAuthority(providerID string, vehicleID string) *stubAuthority {
	yes := true
	now := time.Now().UTC()
	return &stubAuthority{
		configured: true,
		vehicle: platformclients.VehicleAuthorityState{
			VehicleID: vehicleID, ProviderID: providerID,
			ControlState: "PROVIDER_CONTROLLED", ProviderMatch: &yes,
			ServiceType: "dispatch_bike", ServiceTypeMatch: &yes,
			VerificationState: platformclients.VehicleVerificationVerified,
			Available:         true, AvailabilityState: "ONLINE",
			Restricted: false, RestrictionState: "NONE",
			DocumentsSatisfied: true,
			RequiredDocuments: []platformclients.VehicleDocumentStatus{
				{Type: "registration", Present: true},
				{Type: "insurance", Present: true},
			},
			ActiveAssignmentConflict: false,
			AssignmentConflictBasis:  "PROVIDER_LIVE_TRIP",
			SourceVersion:            now.UnixMilli(), ObservedAt: now,
		},
		provider: platformclients.ProviderAuthorityState{
			ProviderID:        providerID,
			AccountState:      platformclients.ProviderAccountActive,
			VerificationState: platformclients.VehicleVerificationVerified,
			ServiceType:       "individual", ServiceTypeMatch: &yes,
			Available: true, AvailabilityState: "ONLINE", ShiftOpen: true,
			Restricted: false, RestrictionState: "NONE",
			DocumentsSatisfied: true,
			RequiredDocuments: []platformclients.VehicleDocumentStatus{
				{Type: "identity", Present: true},
				{Type: "licence", Present: true},
			},
			ActiveAssignmentConflict: false,
			AssignmentConflictBasis:  "PROVIDER_LIVE_TRIP",
			SourceVersion:            now.UnixMilli(), ObservedAt: now,
		},
	}
}

// newEligibilityService builds the real eligibility service over a stub
// authority.
func newEligibilityService(pool *pgxpool.Pool, authority *stubAuthority) *eligibilityusecases.Service {
	var vehicles platformclients.VehicleAuthority
	var providers platformclients.ProviderAuthority
	if authority != nil {
		vehicles, providers = authority, authority
	}
	return eligibilityusecases.NewService(eligibilityusecases.Options{
		Pool:      pool,
		Decisions: eligibilityrepos.NewRepository(pool),
		Vehicles:  vehicles,
		Providers: providers,
		Events:    fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(pool)),
	})
}

// seedEligibleOrganization gives an organization everything the platform
// default policies require of a company: an ACTIVE lifecycle, an approved and
// freshly synced verification, completed onboarding stages, an approved
// service capability and a current active subscription projection.
//
// It exists so a test about assignment is a test about assignment, rather than
// a test about whether the fixture remembered nine unrelated rows. Tests that
// want a company to FAIL a check undo one of these deliberately.
func seedEligibleOrganization(t *testing.T, pool *pgxpool.Pool, organizationID string) {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		UPDATE fleet_partner_organizations SET status = 'ACTIVE' WHERE id = $1`,
		organizationID); err != nil {
		t.Fatalf("activate organization: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_organization_verification_requests
			(id, organization_id, submission_version, submission_snapshot, status,
			 source_version, synced_at, submitted_by)
		VALUES ($1, $2, 1, '{}'::jsonb, 'APPROVED', 1, now(), 'test')
		ON CONFLICT (organization_id, submission_version) DO UPDATE
			SET status = 'APPROVED', synced_at = now()`,
		ids.New("ver"), organizationID); err != nil {
		t.Fatalf("seed verification: %v", err)
	}

	for _, stage := range []string{
		"OWNER_CONTACT", "BUSINESS_PROFILE", "AUTHORIZED_REPRESENTATIVE",
		"OPERATING_CAPABILITIES", "HEADQUARTERS", "BUSINESS_DOCUMENTS",
		"VERIFICATION", "ORGANIZATION_SETTINGS", "OPERATING_TERRITORIES",
		"WALLET_PROVISIONING", "ACTIVATION",
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO fleet_partner_organization_onboarding_stages
				(organization_id, stage, status, blocking_issues, completed_at)
			VALUES ($1, $2, 'COMPLETE', '[]'::jsonb, now())
			ON CONFLICT (organization_id, stage) DO UPDATE
				SET status = 'COMPLETE', blocking_issues = '[]'::jsonb`,
			organizationID, stage); err != nil {
			t.Fatalf("seed onboarding stage: %v", err)
		}
	}

	for _, capability := range []string{"DISPATCH", "TAXI", "HAULING"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO fleet_partner_organization_capabilities
				(organization_id, capability, status, requires_review)
			VALUES ($1, $2, 'APPROVED', false)
			ON CONFLICT (organization_id, capability) DO UPDATE SET status = 'APPROVED'`,
			organizationID, capability); err != nil {
			t.Fatalf("seed capability: %v", err)
		}
	}

	// The documents the platform default activation policy names. READY rather
	// than ATTACHED: a file that exists is not a check that passed.
	for _, documentType := range []string{"REGISTRATION_CERTIFICATE", "TAX_IDENTIFICATION"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO fleet_partner_organization_documents
				(id, organization_id, document_type, file_id, status, expires_at)
			VALUES ($1, $2, $3, $4, 'READY', now() + interval '1 year')
			ON CONFLICT (organization_id, file_id) WHERE status <> 'REMOVED'
			DO UPDATE SET status = 'READY', expires_at = now() + interval '1 year'`,
			ids.New("doc"), organizationID, documentType,
			"file-"+organizationID+"-"+documentType); err != nil {
			t.Fatalf("seed organization document: %v", err)
		}
	}

	seedActiveSubscription(t, pool, organizationID)
}

// seedActiveSubscription writes a current, reconciled, ACTIVE subscription
// projection.
func seedActiveSubscription(t *testing.T, pool *pgxpool.Pool, organizationID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_finance_projections
			(id, organization_id, projection_type, subject_type, subject_id,
			 wallet_reference, safe_status, source_version, freshness_state,
			 last_synced_at, reconciliation_status)
		VALUES ($1, $2, 'SUBSCRIPTION', 'ORGANIZATION', $2, $3, 'ACTIVE', 1,
		        'CURRENT', now(), 'IDLE')
		ON CONFLICT (organization_id, projection_type, wallet_reference) DO UPDATE
			SET safe_status = 'ACTIVE', freshness_state = 'CURRENT',
			    reconciliation_status = 'IDLE', last_synced_at = now()`,
		ids.New("fpj"), organizationID, "sub-"+organizationID); err != nil {
		t.Fatalf("seed subscription projection: %v", err)
	}
}

// markMembershipSynced makes a provider membership look freshly projected, so
// the projection-freshness check has something current to read.
func markMembershipSynced(t *testing.T, pool *pgxpool.Pool, membershipID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		UPDATE fleet_partner_organization_provider_memberships
		SET synced_at = now(), source_version = greatest(source_version, 1),
		    association_accepted_at = coalesce(association_accepted_at, now())
		WHERE id = $1`, membershipID); err != nil {
		t.Fatalf("mark membership synced: %v", err)
	}
}

// providerIdentity reads the authoritative provider identifier a membership
// carries, if any. A membership without one has no authority to ask about, and
// the gate blocks on provider_not_linked rather than on a stubbed answer.
func providerIdentity(membership *providermodels.Membership) string {
	if membership == nil || membership.ProviderID == nil {
		return ""
	}
	return *membership.ProviderID
}
