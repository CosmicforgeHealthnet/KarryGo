package integration_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	eligibilityrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// WP9 eligibility decisions, against real PostgreSQL.
//
// The tests below are organized around one question: can a gate be made to
// open when it should not? Each case removes exactly one fact — a suspended
// company, an unreachable authority, an expired document, a stale projection —
// and asserts both that the answer blocks AND that the recorded decision says
// why in a controlled code somebody can query for later.

// eligibilityFixture is an organization with a managed provider.
type eligibilityFixture struct {
	pool       *pgxpool.Pool
	authority  *stubAuthority
	service    *eligibilityusecases.Service
	repo       *eligibilityrepos.Repository
	orgID      string
	membership *providermodels.Membership
}

func newEligibilityFixture(t *testing.T, name string, registration string) *eligibilityFixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, name, registration)
	membership := seedProviderMembership(t, pool, org.ID, "drv-"+registration)
	authority := healthyAuthority(*membership.ProviderID, "veh-"+registration)
	return &eligibilityFixture{
		pool:       pool,
		authority:  authority,
		service:    newEligibilityService(pool, authority),
		repo:       eligibilityrepos.NewRepository(pool),
		orgID:      org.ID,
		membership: membership,
	}
}

func (f *eligibilityFixture) actor() eligibilityusecases.Actor {
	return eligibilityusecases.Actor{
		OrganizationID: f.orgID,
		ActorType:      eligibilitymodels.ActorOrganizationUser,
		ActorID:        "usr-eligibility",
	}
}

func (f *eligibilityFixture) evaluateCompany(t *testing.T,
	operation string) *eligibilitymodels.Decision {
	t.Helper()
	decision, err := f.service.Evaluate(context.Background(), eligibilityusecases.EvaluateInput{
		Actor:         f.actor(),
		SubjectType:   eligibilitymodels.SubjectCompany,
		SubjectID:     f.orgID,
		OperationType: operation,
		CorrelationID: "corr-company",
	})
	if err != nil {
		t.Fatalf("evaluate company: %v", err)
	}
	return decision
}

func (f *eligibilityFixture) evaluateProvider(t *testing.T,
	operation string) *eligibilitymodels.Decision {
	t.Helper()
	decision, err := f.service.Evaluate(context.Background(), eligibilityusecases.EvaluateInput{
		Actor:           f.actor(),
		SubjectType:     eligibilitymodels.SubjectProvider,
		SubjectID:       f.membership.ID,
		OperationType:   operation,
		ServiceCategory: providermodels.CategoryDispatch,
		CorrelationID:   "corr-provider",
	})
	if err != nil {
		t.Fatalf("evaluate provider: %v", err)
	}
	return decision
}

func (f *eligibilityFixture) evaluateVehicle(t *testing.T, vehicleID string,
	operation string) *eligibilitymodels.Decision {
	t.Helper()
	decision, err := f.service.Evaluate(context.Background(), eligibilityusecases.EvaluateInput{
		Actor:         f.actor(),
		SubjectType:   eligibilitymodels.SubjectVehicle,
		SubjectID:     vehicleID,
		MembershipID:  f.membership.ID,
		OperationType: operation,
		CorrelationID: "corr-vehicle",
	})
	if err != nil {
		t.Fatalf("evaluate vehicle: %v", err)
	}
	return decision
}

// hasReason reports whether a decision carries a controlled reason code.
func hasReason(decision *eligibilitymodels.Decision, reason string) bool {
	for _, candidate := range decision.Reasons {
		if candidate == reason {
			return true
		}
	}
	return false
}

func requireReason(t *testing.T, decision *eligibilitymodels.Decision, reason string) {
	t.Helper()
	if !hasReason(decision, reason) {
		t.Fatalf("status=%s reasons=%v; want reason %q",
			decision.Status, decision.Reasons, reason)
	}
}

func requireStatus(t *testing.T, decision *eligibilitymodels.Decision, status string) {
	t.Helper()
	if decision.Status != status {
		t.Fatalf("status = %s (reasons %v), want %s",
			decision.Status, decision.Reasons, status)
	}
}

// ── Happy paths ───────────────────────────────────────────────────────────────

func TestEligibleCompanyProviderAndVehicle(t *testing.T) {
	f := newEligibilityFixture(t, "Eligible Co", "RC-ELG-1")

	company := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)
	requireStatus(t, company, eligibilitymodels.StatusEligible)
	if !company.Authorizes(time.Now().UTC()) {
		t.Fatal("a current unexpired ELIGIBLE decision must authorize")
	}

	provider := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, provider, eligibilitymodels.StatusEligible)

	vehicle := f.evaluateVehicle(t, "veh-RC-ELG-1", eligibilitymodels.OperationVehicleLinking)
	requireStatus(t, vehicle, eligibilitymodels.StatusEligible)

	// Every decision names the authorities it consulted and the version each
	// answered at. Without that a decision is an unfalsifiable claim.
	if len(vehicle.Sources) == 0 {
		t.Fatal("a vehicle decision must record the authority it consulted")
	}
	var sawVehicleAuthority bool
	for _, source := range vehicle.Sources {
		if source.SourceKind == "VEHICLE" {
			sawVehicleAuthority = true
			if source.AuthorityService != "driver-dispatch-delivery-service" {
				t.Fatalf("authority = %s", source.AuthorityService)
			}
			if source.SourceVersion <= 0 {
				t.Fatal("the vehicle authority's source version must be recorded")
			}
			if source.FreshnessState != eligibilitymodels.FreshnessCurrent {
				t.Fatalf("freshness = %s", source.FreshnessState)
			}
		}
	}
	if !sawVehicleAuthority {
		t.Fatal("the vehicle authority must appear in the decision's sources")
	}
}

func TestEligibilityDecisionWritesHistoryAndOutboxAtomically(t *testing.T) {
	f := newEligibilityFixture(t, "Eligible Audit Co", "RC-ELG-2")
	ctx := context.Background()

	decision := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)

	entries, err := f.repo.ListHistory(ctx, f.orgID, decision.ID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("history = %d entries, %v; want 1", len(entries), err)
	}
	if entries[0].Action != eligibilitymodels.ActionEvaluated ||
		entries[0].ToStatus != eligibilitymodels.StatusEligible {
		t.Fatalf("history entry = %+v", entries[0])
	}

	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records
		WHERE organization_id = $1 AND resource_id = $2`, f.orgID, decision.ID).
		Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit records = %d, %v; want 1", audits, err)
	}

	var events int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox
		WHERE aggregate_id = $1 AND event_type = $2`, decision.ID,
		eligibilityusecases.EventEligibilityDecided).Scan(&events); err != nil || events != 1 {
		t.Fatalf("outbox events = %d, %v; want 1", events, err)
	}
}

// ── Company failures ──────────────────────────────────────────────────────────

func TestSuspendedCompanyIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Suspended Co", "RC-ELG-3")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_organizations SET status = 'SUSPENDED' WHERE id = $1`,
		f.orgID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonCompanySuspended)
	if decision.Authorizes(time.Now().UTC()) {
		t.Fatal("an INELIGIBLE decision must never authorize")
	}
}

func TestRestrictedCompanyIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Restricted Co", "RC-ELG-4")
	if _, err := f.pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_organization_closure_requests
			(id, organization_id, status, reason_code, requested_by)
		VALUES ('clr_restricted_1', $1, 'ASSESSING', 'owner_request', 'usr-test')`,
		f.orgID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonCompanyRestricted)
}

func TestUnverifiedCompanyIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Unverified Co", "RC-ELG-5")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_organization_verification_requests
		SET status = 'UNDER_REVIEW' WHERE organization_id = $1`, f.orgID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonCompanyVerificationPending)
}

// TestUnknownVerificationOutcomeFailsClosed proves the distinction that makes
// the status set worth having: an outcome nobody could establish is UNKNOWN,
// not INELIGIBLE and certainly not eligible.
func TestUnknownVerificationOutcomeFailsClosed(t *testing.T) {
	f := newEligibilityFixture(t, "Unknown Verification Co", "RC-ELG-6")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_organization_verification_requests
		SET status = 'UNKNOWN_OUTCOME' WHERE organization_id = $1`, f.orgID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)
	requireStatus(t, decision, eligibilitymodels.StatusUnknown)
	requireReason(t, decision, eligibilitymodels.ReasonCompanyVerificationUnknown)
}

// TestStaleVerificationProjectionIsStale proves an approval nobody has
// confirmed recently stops being usable rather than lasting forever.
func TestStaleVerificationProjectionIsStale(t *testing.T) {
	f := newEligibilityFixture(t, "Stale Verification Co", "RC-ELG-7")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_organization_verification_requests
		SET synced_at = now() - interval '30 days' WHERE organization_id = $1`,
		f.orgID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)
	requireStatus(t, decision, eligibilitymodels.StatusStale)
	requireReason(t, decision, eligibilitymodels.ReasonSourceVersionStale)
}

func TestMissingRequiredDocumentIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "No Documents Co", "RC-ELG-8")

	// COMPANY_ACTIVATION requires a registration certificate and a tax id.
	// Removing them is what this test is about, so it undoes the fixture's
	// baseline explicitly rather than relying on it being absent.
	if _, err := f.pool.Exec(context.Background(), `
		DELETE FROM fleet_partner_organization_documents WHERE organization_id = $1`,
		f.orgID); err != nil {
		t.Fatal(err)
	}
	decision := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonCompanyDocumentMissing)
}

func TestExpiredRequiredDocumentIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Expired Documents Co", "RC-ELG-9")
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `
		DELETE FROM fleet_partner_organization_documents WHERE organization_id = $1`,
		f.orgID); err != nil {
		t.Fatal(err)
	}
	for index, documentType := range []string{"REGISTRATION_CERTIFICATE", "TAX_IDENTIFICATION"} {
		expiry := "now() + interval '1 year'"
		if index == 0 {
			expiry = "now() - interval '1 day'"
		}
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO fleet_partner_organization_documents
				(id, organization_id, document_type, file_id, status, expires_at)
			VALUES ($1, $2, $3, $4, 'READY', `+expiry+`)`,
			"doc_expired_"+documentType, f.orgID, documentType, "file-"+documentType); err != nil {
			t.Fatal(err)
		}
	}

	decision := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonCompanyDocumentExpired)
	// The satisfied document must not also be reported missing: a reason list
	// that over-reports is one operators learn to ignore.
	if hasReason(decision, eligibilitymodels.ReasonCompanyDocumentMissing) {
		t.Fatalf("reasons over-report: %v", decision.Reasons)
	}
}

func TestInactiveSubscriptionBlocksAssignment(t *testing.T) {
	f := newEligibilityFixture(t, "Lapsed Sub Co", "RC-ELG-10")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_finance_projections SET safe_status = 'PAST_DUE'
		WHERE organization_id = $1 AND projection_type = 'SUBSCRIPTION'`,
		f.orgID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonCompanySubscriptionInactive)
}

func TestStaleSubscriptionProjectionIsStale(t *testing.T) {
	f := newEligibilityFixture(t, "Stale Sub Co", "RC-ELG-11")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_finance_projections SET freshness_state = 'GAP_DETECTED'
		WHERE organization_id = $1 AND projection_type = 'SUBSCRIPTION'`,
		f.orgID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, decision, eligibilitymodels.StatusStale)
	requireReason(t, decision, eligibilitymodels.ReasonCompanySubscriptionStale)
}

// TestReconcilingSubscriptionIsReconciling proves an open reconciliation case
// blocks even when the projection claims to be current: the two systems
// disagreed, and acting during the disagreement is acting on a value nobody
// vouches for.
func TestReconcilingSubscriptionIsReconciling(t *testing.T) {
	f := newEligibilityFixture(t, "Reconciling Sub Co", "RC-ELG-12")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_finance_projections
		SET freshness_state = 'CURRENT', reconciliation_status = 'PROCESSING'
		WHERE organization_id = $1 AND projection_type = 'SUBSCRIPTION'`,
		f.orgID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, decision, eligibilitymodels.StatusReconciling)
}

// ── Provider failures ─────────────────────────────────────────────────────────

func TestUnverifiedProviderIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Unverified Provider Co", "RC-ELG-13")
	f.authority.provider.VerificationState = platformclients.VehicleVerificationPending

	decision := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonProviderUnverified)
}

func TestSuspendedProviderMembershipIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Suspended Provider Co", "RC-ELG-14")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_organization_provider_memberships
		SET status = 'SUSPENDED' WHERE id = $1`, f.membership.ID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonProviderSuspended)
}

func TestWrongServiceCategoryIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Wrong Service Co", "RC-ELG-15")

	decision, err := f.service.Evaluate(context.Background(), eligibilityusecases.EvaluateInput{
		Actor:           f.actor(),
		SubjectType:     eligibilitymodels.SubjectProvider,
		SubjectID:       f.membership.ID,
		OperationType:   eligibilitymodels.OperationAssignment,
		ServiceCategory: providermodels.CategoryTaxi,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonProviderServiceMismatch)
}

func TestProviderWithoutConsentIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "No Consent Co", "RC-ELG-16")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_organization_provider_memberships
		SET association_accepted_at = NULL WHERE id = $1`, f.membership.ID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonProviderConsentMissing)
}

func TestProviderWithExpiredDocumentIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Expired Licence Co", "RC-ELG-17")
	expired := time.Now().UTC().Add(-24 * time.Hour)
	f.authority.provider.DocumentsSatisfied = false
	f.authority.provider.RequiredDocuments = []platformclients.VehicleDocumentStatus{
		{Type: "identity", Present: true},
		{Type: "licence", Present: true, Expired: true, ExpiresAt: &expired},
	}

	decision := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonProviderDocumentExpired)
}

func TestProviderAssignmentConflictIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Busy Provider Co", "RC-ELG-18")
	projection := seedProjection(t, f.pool, f.orgID, "req-RC-ELG-18",
		"ORGANIZATION_ACCEPTED")
	seedAssignment(t, f.pool, f.orgID, projection.ID, f.membership,
		time.Now().Add(time.Hour))

	decision := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonProviderAssignmentConflict)
}

func TestStaleProviderProjectionIsStale(t *testing.T) {
	f := newEligibilityFixture(t, "Stale Provider Co", "RC-ELG-19")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_organization_provider_memberships
		SET synced_at = now() - interval '30 days' WHERE id = $1`,
		f.membership.ID); err != nil {
		t.Fatal(err)
	}

	decision := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, decision, eligibilitymodels.StatusStale)
	requireReason(t, decision, eligibilitymodels.ReasonProviderProjectionStale)
}

// ── Authority availability ────────────────────────────────────────────────────

// TestUnavailableAuthorityFailsClosed is the case the whole status set exists
// for: an authority that cannot be reached must not be indistinguishable from
// one that answered "fine".
func TestUnavailableAuthorityFailsClosed(t *testing.T) {
	f := newEligibilityFixture(t, "Authority Down Co", "RC-ELG-20")
	f.authority.providerErr = &platformclients.Error{
		Service: "driver-dispatch-delivery-service",
		Outcome: platformclients.OutcomeTransient, Message: "unreachable"}

	decision := f.evaluateProvider(t, eligibilitymodels.OperationAssignment)
	requireStatus(t, decision, eligibilitymodels.StatusUnknown)
	requireReason(t, decision, eligibilitymodels.ReasonAuthorityUnavailable)

	// The authority is still named on the decision, with UNKNOWN freshness, so
	// an operator can see which integration was down when work was blocked.
	var named bool
	for _, source := range decision.Sources {
		if source.SourceKind == "PROVIDER" &&
			source.FreshnessState == eligibilitymodels.FreshnessUnknown {
			named = true
		}
	}
	if !named {
		t.Fatalf("the unreachable authority must be recorded: %+v", decision.Sources)
	}
}

func TestUnconfiguredAuthorityFailsClosed(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "No Authority Co", "RC-ELG-21")
	membership := seedProviderMembership(t, pool, org.ID, "drv-RC-ELG-21")

	// No authority client at all — the deployment has no dispatch credentials.
	service := newEligibilityService(pool, nil)
	decision, err := service.Evaluate(context.Background(), eligibilityusecases.EvaluateInput{
		Actor: eligibilityusecases.Actor{OrganizationID: org.ID,
			ActorType: eligibilitymodels.ActorSystem, ActorID: "sys"},
		SubjectType:     eligibilitymodels.SubjectProvider,
		SubjectID:       membership.ID,
		OperationType:   eligibilitymodels.OperationAssignment,
		ServiceCategory: providermodels.CategoryDispatch,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, decision, eligibilitymodels.StatusUnknown)
	requireReason(t, decision, eligibilitymodels.ReasonAuthorityNotConfigured)
}

// ── Vehicle failures ──────────────────────────────────────────────────────────

func TestUnknownVehicleIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Unknown Vehicle Co", "RC-ELG-22")
	f.authority.vehicleErr = platformclients.ErrVehicleNotFound

	decision := f.evaluateVehicle(t, "veh-missing", eligibilitymodels.OperationVehicleLinking)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonVehicleNotFound)
}

// TestVehicleOwnershipFailureIsIneligible proves the ownership question is
// answered by the authority against the provider FLEET believes owns it, not
// against whatever the caller nominated.
func TestVehicleOwnershipFailureIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Wrong Owner Co", "RC-ELG-23")
	no := false
	f.authority.vehicle.ProviderMatch = &no
	f.authority.vehicle.ProviderID = "drv-somebody-else"

	decision := f.evaluateVehicle(t, "veh-RC-ELG-23", eligibilitymodels.OperationVehicleLinking)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonVehicleOwnershipMismatch)
	requireReason(t, decision, eligibilitymodels.ReasonVehicleProviderMismatch)
}

func TestUnverifiedVehicleIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Unverified Vehicle Co", "RC-ELG-24")
	f.authority.vehicle.VerificationState = platformclients.VehicleVerificationSuspended

	decision := f.evaluateVehicle(t, "veh-RC-ELG-24", eligibilitymodels.OperationVehicleLinking)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonVehicleUnverified)
}

func TestRestrictedVehicleIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "Restricted Vehicle Co", "RC-ELG-25")
	f.authority.vehicle.Restricted = true
	f.authority.vehicle.RestrictionState = "SUSPENDED"

	decision := f.evaluateVehicle(t, "veh-RC-ELG-25", eligibilitymodels.OperationVehicleLinking)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonVehicleRestricted)
}

func TestVehicleWithMissingDocumentIsIneligible(t *testing.T) {
	f := newEligibilityFixture(t, "No Insurance Co", "RC-ELG-26")
	f.authority.vehicle.DocumentsSatisfied = false
	f.authority.vehicle.RequiredDocuments = []platformclients.VehicleDocumentStatus{
		{Type: "registration", Present: true},
		{Type: "insurance", Present: false},
	}

	decision := f.evaluateVehicle(t, "veh-RC-ELG-26", eligibilitymodels.OperationVehicleLinking)
	requireStatus(t, decision, eligibilitymodels.StatusIneligible)
	requireReason(t, decision, eligibilitymodels.ReasonVehicleDocumentMissing)
}

// TestVehicleWithoutSourceVersionIsStale proves an authority that cannot state
// a version for its own answer cannot be checked for staleness later, so the
// answer is treated as already stale rather than trusted indefinitely.
func TestVehicleWithoutSourceVersionIsStale(t *testing.T) {
	f := newEligibilityFixture(t, "Versionless Vehicle Co", "RC-ELG-27")
	f.authority.vehicle.SourceVersion = 0

	decision := f.evaluateVehicle(t, "veh-RC-ELG-27", eligibilitymodels.OperationVehicleLinking)
	requireStatus(t, decision, eligibilitymodels.StatusStale)
	requireReason(t, decision, eligibilitymodels.ReasonSourceVersionStale)
}

// ── Decision lifecycle ────────────────────────────────────────────────────────

// TestExpiredDecisionDoesNotAuthorize proves a decision stops being usable on
// its own terms, without anything having to notice.
func TestExpiredDecisionDoesNotAuthorize(t *testing.T) {
	f := newEligibilityFixture(t, "Expiring Co", "RC-ELG-28")
	ctx := context.Background()

	// The expiry is produced by a real one-second policy rather than by
	// editing the row: the row cannot be edited, which is itself the point,
	// and a test that reached around the trigger would be testing nothing.
	if _, err := f.service.PublishPolicy(ctx, f.actor(), &eligibilitymodels.Policy{
		PolicyKey:             eligibilitymodels.OperationPaidFeature,
		RequiredCompanyChecks: []string{eligibilitymodels.CheckLifecycle},
		DecisionTTLSeconds:    1,
		SourceMaxAgeSeconds:   600,
	}); err != nil {
		t.Fatalf("publish short-lived policy: %v", err)
	}

	decision := f.evaluateCompany(t, eligibilitymodels.OperationPaidFeature)
	requireStatus(t, decision, eligibilitymodels.StatusEligible)
	if !decision.Authorizes(time.Now().UTC()) {
		t.Fatal("a fresh ELIGIBLE decision must authorize")
	}

	time.Sleep(1100 * time.Millisecond)

	reloaded, err := f.service.Get(ctx, f.orgID, decision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Authorizes(time.Now().UTC()) {
		t.Fatal("an expired decision must not authorize")
	}

	// Require therefore takes a NEW decision rather than reusing the old one.
	fresh, err := f.service.Require(ctx, eligibilityusecases.EvaluateInput{
		Actor:         f.actor(),
		SubjectType:   eligibilitymodels.SubjectCompany,
		SubjectID:     f.orgID,
		OperationType: eligibilitymodels.OperationPaidFeature,
	})
	if err != nil {
		t.Fatalf("re-evaluation after expiry: %v", err)
	}
	if fresh.ID == decision.ID {
		t.Fatal("an expired decision must not be reused")
	}
}

// TestNewDecisionSupersedesTheOldOne proves history is preserved rather than
// overwritten: the superseded row stays exactly as it was.
func TestNewDecisionSupersedesTheOldOne(t *testing.T) {
	f := newEligibilityFixture(t, "Superseding Co", "RC-ELG-29")
	ctx := context.Background()

	first := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)
	second := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)
	if first.ID == second.ID {
		t.Fatal("a second evaluation must produce a new decision")
	}

	reloaded, err := f.service.Get(ctx, f.orgID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Current() {
		t.Fatal("the first decision must be superseded")
	}
	if reloaded.SupersededBy == nil || *reloaded.SupersededBy != second.ID {
		t.Fatalf("supersededBy = %v, want %s", reloaded.SupersededBy, second.ID)
	}
	if reloaded.Status != first.Status || reloaded.CheckedAt.Unix() != first.CheckedAt.Unix() {
		t.Fatal("superseding must not rewrite the earlier decision")
	}

	current, err := f.service.Current(ctx, f.orgID, eligibilitymodels.SubjectCompany,
		f.orgID, eligibilitymodels.OperationCompanyActivation)
	if err != nil || current.ID != second.ID {
		t.Fatalf("current = %v, %v; want %s", current, err, second.ID)
	}
}

// TestConcurrentEvaluationsProduceOneLiveDecision exercises the partial unique
// index under real contention. Two dispatchers evaluating the same subject at
// once must not leave two live decisions a caller could choose between.
func TestConcurrentEvaluationsProduceOneLiveDecision(t *testing.T) {
	f := newEligibilityFixture(t, "Concurrent Co", "RC-ELG-30")
	ctx := context.Background()

	const attempts = 6
	var wait sync.WaitGroup
	errs := make([]error, attempts)
	wait.Add(attempts)
	for index := 0; index < attempts; index++ {
		go func(slot int) {
			defer wait.Done()
			_, errs[slot] = f.service.Evaluate(ctx, eligibilityusecases.EvaluateInput{
				Actor:         f.actor(),
				SubjectType:   eligibilitymodels.SubjectCompany,
				SubjectID:     f.orgID,
				OperationType: eligibilitymodels.OperationCompanyActivation,
			})
		}(index)
	}
	wait.Wait()

	var succeeded int
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		var appErr *apperrors.Error
		if !errors.As(err, &appErr) ||
			appErr.Code != errorcodes.EligibilityDecisionConflict {
			t.Fatalf("unexpected concurrent failure: %v", err)
		}
	}
	if succeeded == 0 {
		t.Fatal("at least one concurrent evaluation must succeed")
	}

	var live int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_eligibility_decisions
		WHERE organization_id = $1 AND subject_type = 'COMPANY' AND superseded_at IS NULL`,
		f.orgID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 1 {
		t.Fatalf("live decisions = %d, want exactly 1", live)
	}
}

// TestDecisionsAreImmutable proves the schema itself refuses a rewrite. An
// eligibility record that can be edited is not evidence of anything.
func TestDecisionsAreImmutable(t *testing.T) {
	f := newEligibilityFixture(t, "Immutable Co", "RC-ELG-31")
	decision := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)

	// Every field that carries the meaning of the decision is protected, not
	// just the status: an expiry that could be pushed out would make the
	// status irrelevant.
	for _, statement := range []string{
		`UPDATE fleet_partner_eligibility_decisions SET status = 'INELIGIBLE' WHERE id = $1`,
		`UPDATE fleet_partner_eligibility_decisions SET expires_at = now() + interval '10 years' WHERE id = $1`,
		`UPDATE fleet_partner_eligibility_decisions SET input_fingerprint = 'rewritten' WHERE id = $1`,
		`UPDATE fleet_partner_eligibility_decisions SET policy_version = 99 WHERE id = $1`,
	} {
		if _, err := f.pool.Exec(context.Background(), statement, decision.ID); err == nil {
			t.Fatalf("editing a recorded decision must be refused: %s", statement)
		}
	}
	if _, err := f.pool.Exec(context.Background(), `
		DELETE FROM fleet_partner_eligibility_decisions WHERE id = $1`,
		decision.ID); err == nil {
		t.Fatal("deleting a recorded decision must be refused by the database")
	}
}

// ── Policy versioning ─────────────────────────────────────────────────────────

// TestPolicyPublicationDoesNotRewriteHistory proves the central promise of
// versioned policy: a stricter rule published today does not change what
// yesterday's decision meant, and does not silently revalidate it either.
func TestPolicyPublicationDoesNotRewriteHistory(t *testing.T) {
	f := newEligibilityFixture(t, "Policy Co", "RC-ELG-32")
	ctx := context.Background()

	before := f.evaluateCompany(t, eligibilitymodels.OperationPaidFeature)
	requireStatus(t, before, eligibilitymodels.StatusEligible)
	if before.PolicyVersion != 1 {
		t.Fatalf("policyVersion = %d, want the platform default 1", before.PolicyVersion)
	}

	published, err := f.service.PublishPolicy(ctx, f.actor(), &eligibilitymodels.Policy{
		PolicyKey:             eligibilitymodels.OperationPaidFeature,
		RequiredCompanyChecks: []string{eligibilitymodels.CheckLifecycle, eligibilitymodels.CheckDocuments},
		RequiredDocuments:     []string{"OPERATING_LICENCE"},
		DecisionTTLSeconds:    600,
		SourceMaxAgeSeconds:   600,
	})
	if err != nil {
		t.Fatalf("publish policy: %v", err)
	}
	if published.Version != 1 {
		t.Fatalf("first tenant version = %d, want 1", published.Version)
	}

	// The historical decision still cites what it was decided under.
	historical, err := f.service.Get(ctx, f.orgID, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if historical.PolicyID != before.PolicyID || historical.PolicyVersion != before.PolicyVersion {
		t.Fatal("publishing a policy must not rewrite the policy a past decision cited")
	}
	if historical.Status != eligibilitymodels.StatusEligible {
		t.Fatal("publishing a policy must not change a past decision's outcome")
	}

	// A new evaluation is governed by the new version and now fails the
	// document requirement the tenant just introduced.
	after := f.evaluateCompany(t, eligibilitymodels.OperationPaidFeature)
	if after.PolicyID != published.ID {
		t.Fatalf("new decision policyId = %s, want the published tenant version", after.PolicyID)
	}
	requireStatus(t, after, eligibilitymodels.StatusIneligible)
	requireReason(t, after, eligibilitymodels.ReasonCompanyDocumentMissing)
}

// ── Tenant isolation ──────────────────────────────────────────────────────────

// TestEligibilityDecisionsAreTenantIsolated proves an identifier from another
// organization is not a way in, and that the answer is indistinguishable from
// a nonexistent one.
func TestEligibilityDecisionsAreTenantIsolated(t *testing.T) {
	f := newEligibilityFixture(t, "Tenant A Co", "RC-ELG-33")
	ctx := context.Background()
	other := createOrganization(t, f.pool, "Tenant B Co", "RC-ELG-34")
	decision := f.evaluateCompany(t, eligibilitymodels.OperationCompanyActivation)

	if _, err := f.service.Get(ctx, other.ID, decision.ID); err == nil {
		t.Fatal("another tenant must not read this decision")
	} else {
		var appErr *apperrors.Error
		if !errors.As(err, &appErr) || appErr.Code != errorcodes.EligibilityDecisionNotFound {
			t.Fatalf("cross-tenant read error = %v; want a not-found code", err)
		}
	}

	decisions, err := f.service.List(ctx, tenant.Query{OrganizationID: other.ID},
		eligibilityrepos.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range decisions {
		if candidate.ID == decision.ID {
			t.Fatal("another tenant's decision leaked into a listing")
		}
	}
}

// TestCompanyDecisionCannotNameAnotherTenant proves the endpoint cannot be
// turned into an oracle for another organization's standing.
func TestCompanyDecisionCannotNameAnotherTenant(t *testing.T) {
	f := newEligibilityFixture(t, "Oracle A Co", "RC-ELG-35")
	other := createOrganization(t, f.pool, "Oracle B Co", "RC-ELG-36")

	_, err := f.service.Evaluate(context.Background(), eligibilityusecases.EvaluateInput{
		Actor:         f.actor(),
		SubjectType:   eligibilitymodels.SubjectCompany,
		SubjectID:     other.ID,
		OperationType: eligibilitymodels.OperationCompanyActivation,
	})
	if err == nil {
		t.Fatal("evaluating another organization must be refused")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("error = %v; the refusal must not confirm the other tenant exists", err)
	}
}
