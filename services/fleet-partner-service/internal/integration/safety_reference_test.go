package integration_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	fleetevents "cosmicforge/logistics/services/fleet-partner-service/internal/events"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	safetymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/models"
	safetyrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/repositories"
	safetyusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// stubValidator stands in for support-dispute-service.
//
// It is deliberately explicit about the three distinct answers the real client
// can give — exists, does not exist, and "no such endpoint" — because fleet
// must behave differently for each and a boolean would collapse two of them.
type stubValidator struct {
	incidentExists bool
	incidentErr    error
	disputeErr     error
	configured     bool
	incidentCalls  int
}

func (v *stubValidator) ValidateIncident(context.Context, platformclients.CallMeta,
	string) (platformclients.ReferenceCheck, error) {
	v.incidentCalls++
	if v.incidentErr != nil {
		return platformclients.ReferenceCheck{}, v.incidentErr
	}
	return platformclients.ReferenceCheck{Exists: v.incidentExists}, nil
}

func (v *stubValidator) ValidateDispute(context.Context, platformclients.CallMeta,
	string) (platformclients.ReferenceCheck, error) {
	if v.disputeErr != nil {
		return platformclients.ReferenceCheck{}, v.disputeErr
	}
	return platformclients.ReferenceCheck{Exists: true}, nil
}

func (v *stubValidator) Configured() bool { return v.configured }

// newSafetyService builds the service with a supplied validator.
func newSafetyService(pool *pgxpool.Pool,
	validator platformclients.SafetyReferenceValidator) *safetyusecases.Service {
	return safetyusecases.NewService(safetyusecases.Options{
		Pool:       pool,
		References: safetyrepos.NewReferenceRepository(pool),
		Validator:  validator,
		Events:     fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(pool)),
	})
}

// safetyFixture is an organization with an assignment to attach cases to.
type safetyFixture struct {
	pool       *pgxpool.Pool
	repo       *safetyrepos.ReferenceRepository
	orgID      string
	assignment *mktmodels.Assignment
}

func newSafetyFixture(t *testing.T, name string, registration string) *safetyFixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, name, registration)
	projection := seedProjection(t, pool, org.ID, "req-"+registration, mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-"+registration)
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Hour))
	return &safetyFixture{
		pool: pool, repo: safetyrepos.NewReferenceRepository(pool),
		orgID: org.ID, assignment: assignment,
	}
}

func (f *safetyFixture) actor() safetyusecases.Actor {
	return safetyusecases.Actor{
		OrganizationID: f.orgID,
		ActorType:      safetymodels.ActorOrganizationUser,
		ActorID:        "ousr_safety_tester",
	}
}

func (f *safetyFixture) input(referenceType string, external string,
	key string) safetyusecases.RecordReferenceInput {
	return safetyusecases.RecordReferenceInput{
		Actor:             f.actor(),
		ReferenceType:     referenceType,
		ExternalReference: external,
		AssignmentID:      f.assignment.ID,
		IdempotencyKey:    key,
		CorrelationID:     "corr-safety",
	}
}

// TestIncidentReferenceIsVerifiedThroughAuthoritativeService proves the happy
// path: a confirmed reference is stored VERIFIED and may justify action.
func TestIncidentReferenceIsVerifiedThroughAuthoritativeService(t *testing.T) {
	f := newSafetyFixture(t, "Safety Verified", "RC-SAF-1")
	validator := &stubValidator{incidentExists: true, configured: true}
	service := newSafetyService(f.pool, validator)

	result, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceIncident, "cmp_authoritative_1", "safety-key-1"))
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if validator.incidentCalls != 1 {
		t.Fatalf("validator calls = %d, want 1", validator.incidentCalls)
	}
	if result.Reference.VerificationState != safetymodels.VerificationVerified {
		t.Fatalf("verificationState = %s", result.Reference.VerificationState)
	}
	if !result.Reference.AuthorizesConsequences() {
		t.Fatal("a verified reference must be usable to justify action")
	}
	if result.Reference.VerifiedAt == nil {
		t.Fatal("verifiedAt must be recorded")
	}
}

// TestDisputeReferenceIsStoredUnverifiedAndAuthorizesNothing is the central
// boundary test for WP6.2: support-dispute-service exposes no by-identifier dispute
// lookup, so the reference is stored explicitly unverified and must never
// justify an operational or financial consequence.
func TestDisputeReferenceIsStoredUnverifiedAndAuthorizesNothing(t *testing.T) {
	f := newSafetyFixture(t, "Safety Unverified", "RC-SAF-2")
	validator := &stubValidator{
		configured: true,
		disputeErr: platformclients.ErrValidationEndpointAbsent,
	}
	service := newSafetyService(f.pool, validator)

	result, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceDispute, "dsp_no_endpoint_1", "safety-key-2"))
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if result.Reference.VerificationState != safetymodels.VerificationUnverified {
		t.Fatalf("verificationState = %s, want UNVERIFIED", result.Reference.VerificationState)
	}
	if result.Reference.AuthorizesConsequences() {
		t.Fatal("an unverified reference must never authorize a consequence")
	}
	if result.Reference.VerifiedAt != nil {
		t.Fatal("an unverified reference must not carry a verification timestamp")
	}
}

// TestReferenceFailsClosedWhenValidationIsUnavailable proves an outage cannot
// be used to record whatever you like.
func TestReferenceFailsClosedWhenValidationIsUnavailable(t *testing.T) {
	f := newSafetyFixture(t, "Safety Fail Closed", "RC-SAF-3")
	validator := &stubValidator{
		configured:  true,
		incidentErr: errors.New("connection reset"),
	}
	service := newSafetyService(f.pool, validator)

	_, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceIncident, "cmp_unreachable_1", "safety-key-3"))
	if err == nil {
		t.Fatal("an unreachable authority must refuse the write")
	}
	if code := errorCode(t, err); code != errorcodes.SafetyValidationUnavailable {
		t.Fatalf("code = %s, want %s", code, errorcodes.SafetyValidationUnavailable)
	}

	// Crucially: nothing was stored, and certainly not as UNVERIFIED.
	references, err := f.repo.List(context.Background(),
		tenant.Query{OrganizationID: f.orgID}, safetyrepos.ListFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(references) != 0 {
		t.Fatalf("stored %d references; a failed validation must store nothing", len(references))
	}
}

// TestReferenceFailsClosedWhenNoValidatorIsWired proves a deployment missing
// the support-dispute client cannot silently downgrade verification.
func TestReferenceFailsClosedWhenNoValidatorIsWired(t *testing.T) {
	f := newSafetyFixture(t, "Safety No Client", "RC-SAF-4")
	service := newSafetyService(f.pool, nil)

	_, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceIncident, "cmp_no_client_1", "safety-key-4"))
	if err == nil {
		t.Fatal("a deployment with no validator must refuse a verifiable reference")
	}
	if code := errorCode(t, err); code != errorcodes.SafetyValidationUnavailable {
		t.Fatalf("code = %s, want %s", code, errorcodes.SafetyValidationUnavailable)
	}
}

// TestReferenceRejectsUnknownCase proves a definitive "no" is a rejection.
func TestReferenceRejectsUnknownCase(t *testing.T) {
	f := newSafetyFixture(t, "Safety Unknown", "RC-SAF-5")
	service := newSafetyService(f.pool, &stubValidator{incidentExists: false, configured: true})

	_, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceIncident, "cmp_does_not_exist", "safety-key-5"))
	if err == nil {
		t.Fatal("an unrecognised case must be rejected")
	}
	if code := errorCode(t, err); code != errorcodes.SafetyReferenceInvalid {
		t.Fatalf("code = %s, want %s", code, errorcodes.SafetyReferenceInvalid)
	}
}

// TestReferenceIsIdempotent proves a retry replays.
func TestReferenceIsIdempotent(t *testing.T) {
	f := newSafetyFixture(t, "Safety Idempotent", "RC-SAF-6")
	service := newSafetyService(f.pool, &stubValidator{incidentExists: true, configured: true})
	input := f.input(safetymodels.ReferenceIncident, "cmp_retry_1", "safety-key-6")

	first, err := service.RecordReference(context.Background(), input)
	if err != nil {
		t.Fatalf("first record: %v", err)
	}
	second, err := service.RecordReference(context.Background(), input)
	if err != nil {
		t.Fatalf("retry must replay: %v", err)
	}
	if !second.Replayed {
		t.Fatal("the retry must be reported as a replay")
	}
	if first.Reference.ID != second.Reference.ID {
		t.Fatal("a replay must return the original record")
	}
}

// TestReferenceRejectsReusedKeyWithDifferentInputs proves a used key cannot be
// repointed.
func TestReferenceRejectsReusedKeyWithDifferentInputs(t *testing.T) {
	f := newSafetyFixture(t, "Safety Key Reuse", "RC-SAF-7")
	service := newSafetyService(f.pool, &stubValidator{incidentExists: true, configured: true})

	if _, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceIncident, "cmp_key_a", "safety-key-7")); err != nil {
		t.Fatalf("first record: %v", err)
	}
	_, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceIncident, "cmp_key_b", "safety-key-7"))
	if err == nil {
		t.Fatal("a reused key with different inputs must be rejected")
	}
	if code := errorCode(t, err); code != errorcodes.IdempotencyConflictCode {
		t.Fatalf("code = %s, want %s", code, errorcodes.IdempotencyConflictCode)
	}
}

// TestReferenceRejectsConflictingAttachment proves the same case cannot be
// silently repointed at different work.
func TestReferenceRejectsConflictingAttachment(t *testing.T) {
	f := newSafetyFixture(t, "Safety Conflict", "RC-SAF-8")
	service := newSafetyService(f.pool, &stubValidator{incidentExists: true, configured: true})

	if _, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceIncident, "cmp_conflict_1", "safety-key-8a")); err != nil {
		t.Fatalf("first record: %v", err)
	}

	conflicting := f.input(safetymodels.ReferenceIncident, "cmp_conflict_1", "safety-key-8b")
	conflicting.AssignmentID = ""
	conflicting.PlatformTripID = "trip_elsewhere"
	_, err := service.RecordReference(context.Background(), conflicting)
	if err == nil {
		t.Fatal("the same case must not be attached to different work")
	}
	if code := errorCode(t, err); code != errorcodes.SafetyReferenceConflict {
		t.Fatalf("code = %s, want %s", code, errorcodes.SafetyReferenceConflict)
	}
}

// TestReferenceRefusesAnotherOrganizationsAssignment is the tenant boundary on
// writes.
func TestReferenceRefusesAnotherOrganizationsAssignment(t *testing.T) {
	f := newSafetyFixture(t, "Safety Tenant A", "RC-SAF-9")
	otherOrg := createOrganization(t, f.pool, "Safety Tenant B", "RC-SAF-9B")
	service := newSafetyService(f.pool, &stubValidator{incidentExists: true, configured: true})

	input := f.input(safetymodels.ReferenceIncident, "cmp_cross_tenant", "safety-key-9")
	input.Actor.OrganizationID = otherOrg.ID

	_, err := service.RecordReference(context.Background(), input)
	if err == nil {
		t.Fatal("an organization must not attach a case to another tenant's assignment")
	}
	if code := errorCode(t, err); code != errorcodes.AssignmentNotFound {
		t.Fatalf("code = %s, want %s (and it must not disclose the assignment exists)",
			code, errorcodes.AssignmentNotFound)
	}
}

// TestReferenceReadsAreTenantScoped proves one organization cannot read
// another's references.
func TestReferenceReadsAreTenantScoped(t *testing.T) {
	f := newSafetyFixture(t, "Safety Read A", "RC-SAF-10")
	otherOrg := createOrganization(t, f.pool, "Safety Read B", "RC-SAF-10B")
	service := newSafetyService(f.pool, &stubValidator{incidentExists: true, configured: true})

	result, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceIncident, "cmp_scoped_1", "safety-key-10"))
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	if _, err := service.Get(context.Background(), otherOrg.ID, result.Reference.ID); err == nil {
		t.Fatal("another organization must not read this reference")
	}
	if _, err := service.History(context.Background(), otherOrg.ID, result.Reference.ID); err == nil {
		t.Fatal("another organization must not read this reference's history")
	}

	foreign, err := service.List(context.Background(),
		tenant.Query{OrganizationID: otherOrg.ID}, safetyrepos.ListFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(foreign) != 0 {
		t.Fatalf("cross-tenant listing returned %d rows", len(foreign))
	}

	own, err := service.List(context.Background(),
		tenant.Query{OrganizationID: f.orgID}, safetyrepos.ListFilter{})
	if err != nil || len(own) != 1 {
		t.Fatalf("own listing = %d rows, %v; want 1", len(own), err)
	}
}

// TestReferenceHistoryIsAppendOnly proves the audit trail cannot be rewritten.
func TestReferenceHistoryIsAppendOnly(t *testing.T) {
	f := newSafetyFixture(t, "Safety History", "RC-SAF-11")
	service := newSafetyService(f.pool, &stubValidator{incidentExists: true, configured: true})

	result, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceIncident, "cmp_history_1", "safety-key-11"))
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	entries, err := service.History(context.Background(), f.orgID, result.Reference.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != safetymodels.ActionCreated {
		t.Fatalf("history = %v; want one CREATED entry", entries)
	}

	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_safety_reference_history SET action = 'DETACHED'
		WHERE reference_id = $1`, result.Reference.ID); err == nil {
		t.Fatal("reference history must be append-only")
	}
	if _, err := f.pool.Exec(context.Background(), `
		DELETE FROM fleet_partner_safety_reference_history
		WHERE reference_id = $1`, result.Reference.ID); err == nil {
		t.Fatal("reference history must not be deletable")
	}
}

// TestReferenceEmitsOutboxEventStatingAuthority proves consumers are told, on
// the wire, whether the reference may justify a consequence.
func TestReferenceEmitsOutboxEventStatingAuthority(t *testing.T) {
	f := newSafetyFixture(t, "Safety Event", "RC-SAF-12")
	service := newSafetyService(f.pool, &stubValidator{
		configured: true, disputeErr: platformclients.ErrValidationEndpointAbsent})

	result, err := service.RecordReference(context.Background(),
		f.input(safetymodels.ReferenceDispute, "dsp_event_1", "safety-key-12"))
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	var payload []byte
	if err := f.pool.QueryRow(context.Background(), `
		SELECT payload FROM fleet_partner_outbox
		WHERE aggregate_id = $1 AND event_type = $2`,
		result.Reference.ID, safetyusecases.EventReferenceRecorded).Scan(&payload); err != nil {
		t.Fatalf("outbox: %v", err)
	}
	body := string(payload)
	for _, expected := range []string{
		// Rendered by PostgreSQL's jsonb output, which inserts a space after
		// the colon; matching the stored form is the point of reading it back.
		`"verificationState": "UNVERIFIED"`, `"authorizesConsequences": false`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("event payload missing %s: %s", expected, body)
		}
	}
}

// TestReferenceRejectsUnattachedAndMalformedInput proves the shape guards.
func TestReferenceRejectsUnattachedAndMalformedInput(t *testing.T) {
	f := newSafetyFixture(t, "Safety Shape", "RC-SAF-13")
	service := newSafetyService(f.pool, &stubValidator{incidentExists: true, configured: true})
	ctx := context.Background()

	unattached := f.input(safetymodels.ReferenceIncident, "cmp_unattached", "safety-key-13a")
	unattached.AssignmentID = ""
	if _, err := service.RecordReference(ctx, unattached); err == nil {
		t.Fatal("a reference attached to nothing must be refused")
	}

	badType := f.input("SOMETHING_ELSE", "cmp_bad_type", "safety-key-13b")
	if _, err := service.RecordReference(ctx, badType); err == nil {
		t.Fatal("an unknown reference type must be refused")
	}

	noKey := f.input(safetymodels.ReferenceIncident, "cmp_no_key", "")
	if _, err := service.RecordReference(ctx, noKey); err == nil {
		t.Fatal("a write with no idempotency key must be refused")
	}

	noReference := f.input(safetymodels.ReferenceIncident, "", "safety-key-13c")
	if _, err := service.RecordReference(ctx, noReference); err == nil {
		t.Fatal("a write with no external reference must be refused")
	}
}
