package integration_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fleetpartner"
	mkthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Section 11 corporate marketplace rows (FLEET-11.1a .. 11.7a).
//
// Every command is driven through the REAL corporate handlers (permission
// gate, idempotent command wrapper, use case, SQL) mounted by rowsRouter. The
// only stand-in is the Dispatch execution service: s11Execution answers the
// canonical reservation and provider-eligibility calls the way Dispatch does
// when it says yes, so the Fleet side of the command runs end to end.

// s11Execution is a configured Dispatch contract that reserves and finds
// providers eligible. Every other call fails closed through the embedded
// UnimplementedExecution.
type s11Execution struct {
	platformclients.UnimplementedExecution
	mu       sync.Mutex
	reserves int
}

func (s *s11Execution) Configured() bool { return true }

func (s *s11Execution) ReserveForOrganization(_ context.Context, _ platformclients.CallMeta,
	request platformclients.ReserveRequest) (platformclients.ReservationOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reserves++
	return platformclients.ReservationOutcome{
		ReservationID: "rsv-" + request.SourceRequestID, Status: platformclients.ReservationReserved,
		WinnerOrganizationID: request.OrganizationID, SourceVersion: request.ExpectedSourceVersion + 1,
	}, nil
}

func (s *s11Execution) ValidateProviderEligibility(context.Context, platformclients.CallMeta,
	string, string, string) (platformclients.ProviderEligibility, error) {
	return platformclients.ProviderEligibility{Eligible: true, VehicleOK: true}, nil
}

func (s *s11Execution) reserveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reserves
}

// s11World is two organizations with the standard actors plus a dispatcher
// and a member holding no role in organization A.
type s11World struct {
	pool       *pgxpool.Pool
	orgs       rowsOrgs
	dispatcher rowsActor
	noRole     rowsActor
	router     http.Handler
	service    *mktusecases.Service
	execution  *s11Execution
}

func newS11World(t *testing.T, label string) *s11World {
	t.Helper()
	// rol_system_analyst holds marketplace.read and nothing else, so it is the
	// member every write must refuse with 403.
	orgs := newRowsOrgs(t, label, "rol_system_analyst")
	pool := orgs.pool
	w := &s11World{pool: pool, orgs: orgs, execution: &s11Execution{
		UnimplementedExecution: platformclients.UnimplementedExecution{Category: platformclients.CategoryDispatch},
	}}
	w.dispatcher = rowsActor{Org: orgs.A.Org, Member: seedApprovalMember(t, pool, orgs.A.Org, "rol_system_dispatcher", "s11-dispatcher")}
	w.noRole = rowsActor{Org: orgs.A.Org, Member: seedApprovalMember(t, pool, orgs.A.Org, "", "s11-norole")}
	executions := platformclients.ExecutionSet{platformclients.CategoryDispatch: w.execution}
	w.service = mktusecases.NewService(mktusecases.Options{
		Pool:            pool,
		Projections:     mktrepos.NewProjectionRepository(pool),
		Offers:          mktrepos.NewOfferRepository(pool),
		Assignments:     mktrepos.NewAssignmentRepository(pool),
		ExecutionStarts: mktrepos.NewExecutionStartRepository(pool),
		Trips:           mktrepos.NewTripRepository(pool),
		Commands:        mktrepos.NewCommandRepository(pool),
		OperatingModes:  orgrepos.NewSettingsRepository(pool),
		Executions:      executions,
		Eligibility: mktusecases.NewEligibilityChecker(
			app.NewMembershipReader(providerrepos.NewRepository(pool)), executions, nil),
		DurableEligibility: newEligibilityService(pool, healthyAuthority("", "")),
		Events:             events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Metrics:            observability.NewMetrics(),
		Opportunities:      mktusecases.OpportunityConfig{Enabled: true},
		Flags: mktusecases.Flags{ProjectionEnabled: true, OrganizationAcceptEnabled: true,
			AssignmentEnabled:     true,
			EnabledSourceServices: map[string]bool{mktmodels.SourceDispatch: true}},
	})
	operatingModes := fleetpartner.NewService(pool, fleetpartner.NewRepository(pool), membershipStub{},
		audit.NewRecorder(), events.NewDomainPublisher(events.NewOutboxRepository(pool)), fleetpartner.Flags{})
	w.router = rowsRouter(t, pool, func(g *gin.RouterGroup, a *accesscontrol.Authorizer, s *idempotency.Store) {
		mkthttp.NewHandler(w.service, a, s).RegisterTenantRoutes(g)
		fleetpartner.NewHandler(operatingModes, a, nil).RegisterTenantRoutes(g)
	})
	return w
}

func (w *s11World) do(t *testing.T, actor rowsActor, method, path string, body any,
	headers map[string]string) rowsResponse {
	t.Helper()
	return rowsDo(t, w.router, actor, method, "/fleet-partner/v1"+path, body, headers)
}

// s11ExpectError fails unless the response carries the wanted status and,
// when given, the wanted error code.
func s11ExpectError(t *testing.T, r rowsResponse, status int, code string, what string) {
	t.Helper()
	rowsExpect(t, r, status, what)
	if code != "" && r.Error.Code != code {
		t.Fatalf("%s: error code %q, want %q; body=%s", what, r.Error.Code, code, r.Raw)
	}
}

// openOffers makes both organizations operating, dispatch-managing
// organizations and ingests one platform offer, which opens a projection and
// an offer in each. It returns organization A's copy.
func (w *s11World) openOffers(t *testing.T) (*mktmodels.RequestProjection, *mktmodels.Offer) {
	t.Helper()
	for _, org := range []string{w.orgs.A.Org, w.orgs.B.Org} {
		seedProviderMembership(t, w.pool, org, ids.New("s11drv"))
		setOperatingMode(t, w.pool, org, mktmodels.ModeCentralized)
	}
	requestID := ids.New("req")
	raw, event := sourceEvent(t, s1112Dispatch, ids.New("evt"), mktusecases.EventRequestOffered,
		"REQUEST", requestID, 2, offeredPayload(requestID, 10*time.Minute))
	if result := ingest(t, w.service, raw, event); result.Status != mktusecases.IngestAccepted {
		t.Fatalf("offered event = %+v", result)
	}
	projection := projectionFor(t, w.pool, w.orgs.A.Org, requestID)
	return projection, offerFor(t, w.pool, w.orgs.A.Org, projection.ID)
}

func s11Audit(t *testing.T, pool *pgxpool.Pool, org, action, resourceID string) int {
	t.Helper()
	return countWhere(t, pool, `SELECT count(*) FROM fleet_partner_audit_records
		WHERE organization_id=$1 AND action=$2 AND resource_id=$3`, org, action, resourceID)
}

func s11OperatingMode(t *testing.T, pool *pgxpool.Pool, org string) (string, int64) {
	t.Helper()
	var mode string
	var version int64
	if err := pool.QueryRow(context.Background(), `SELECT operating_mode, version
		FROM fleet_partner_organization_settings WHERE organization_id=$1`, org).Scan(&mode, &version); err != nil {
		t.Fatalf("read operating mode: %v", err)
	}
	return mode, version
}

func s11Number(t *testing.T, object map[string]any, key string) int64 {
	t.Helper()
	value, ok := object[key].(float64)
	if !ok {
		t.Fatalf("%s is not a number in %v", key, object)
	}
	return int64(value)
}

// TestRow11_1a_OperatingModeReadAndUpdate drives GET/PUT
// /fleet-partner/operating-mode. It also pins the defect it exposed: no role
// held fleet_partner_foundation.read/.manage, so nobody could reach either
// route (fixed by migration 000098).
func TestRow11_1a_OperatingModeReadAndUpdate(t *testing.T) {
	w := newS11World(t, "Row11 1a")
	setOperatingMode(t, w.pool, w.orgs.A.Org, mktmodels.ModeCentralized)
	setOperatingMode(t, w.pool, w.orgs.B.Org, mktmodels.ModeCentralized)
	_, version := s11OperatingMode(t, w.pool, w.orgs.A.Org)
	const path = "/fleet-partner/operating-mode"

	read := rowsExpect(t, w.do(t, w.orgs.A, http.MethodGet, path, nil, nil), http.StatusOK, "owner read").Object(t)
	if read["operatingMode"] != mktmodels.ModeCentralized || read["organizationId"] != w.orgs.A.Org ||
		s11Number(t, read, "version") != version {
		t.Fatalf("owner read = %v", read)
	}
	// Default deny: neither a member without a role nor one whose role lacks
	// the permission may read or change the mode.
	s11ExpectError(t, w.do(t, w.noRole, http.MethodGet, path, nil, nil), http.StatusForbidden, "", "no-role read")
	s11ExpectError(t, w.do(t, w.orgs.NoPermission, http.MethodGet, path, nil, nil), http.StatusForbidden, "", "analyst read")
	s11ExpectError(t, w.do(t, w.orgs.NoPermission, http.MethodPut, path,
		map[string]any{"operatingMode": mktmodels.ModeDelegated, "expectedVersion": version}, nil),
		http.StatusForbidden, "", "analyst update")

	s11ExpectError(t, w.do(t, w.orgs.A, http.MethodPut, path,
		map[string]any{"operatingMode": "ROCKET", "expectedVersion": version}, nil),
		http.StatusUnprocessableEntity, "validation_failed", "invalid mode")
	s11ExpectError(t, w.do(t, w.orgs.A, http.MethodPut, path,
		map[string]any{"operatingMode": mktmodels.ModeDelegated, "expectedVersion": version + 5}, nil),
		http.StatusConflict, "", "stale version")
	if mode, v := s11OperatingMode(t, w.pool, w.orgs.A.Org); mode != mktmodels.ModeCentralized || v != version {
		t.Fatalf("refused updates changed the mode: %s v%d", mode, v)
	}

	updated := rowsExpect(t, w.do(t, w.orgs.A, http.MethodPut, path,
		map[string]any{"operatingMode": mktmodels.ModeDelegated, "expectedVersion": version}, nil),
		http.StatusOK, "owner update").Object(t)
	if updated["operatingMode"] != mktmodels.ModeDelegated || s11Number(t, updated, "version") != version+1 {
		t.Fatalf("update = %v", updated)
	}
	if mode, v := s11OperatingMode(t, w.pool, w.orgs.A.Org); mode != mktmodels.ModeDelegated || v != version+1 {
		t.Fatalf("stored mode = %s v%d", mode, v)
	}
	if got := s11Audit(t, w.pool, w.orgs.A.Org, "fleet.organization.operating_mode.changed", w.orgs.A.Org); got != 1 {
		t.Fatalf("operating-mode audit rows = %d, want 1", got)
	}
	if got := rowsOutboxCount(t, w.pool, w.orgs.A.Org, "fleet.organization.operating_mode.changed.v1"); got != 1 {
		t.Fatalf("operating-mode events = %d, want 1", got)
	}
	// The version it consumed is now stale.
	s11ExpectError(t, w.do(t, w.orgs.A, http.MethodPut, path,
		map[string]any{"operatingMode": mktmodels.ModeHybrid, "expectedVersion": version}, nil),
		http.StatusConflict, "", "replayed version")

	// Tenant isolation: organization B changes only its own mode.
	_, versionB := s11OperatingMode(t, w.pool, w.orgs.B.Org)
	rowsExpect(t, w.do(t, w.orgs.B, http.MethodPut, path,
		map[string]any{"operatingMode": mktmodels.ModeHybrid, "expectedVersion": versionB}, nil),
		http.StatusOK, "org B update")
	if mode, _ := s11OperatingMode(t, w.pool, w.orgs.A.Org); mode != mktmodels.ModeDelegated {
		t.Fatalf("org B's update reached org A: %s", mode)
	}
	readB := rowsExpect(t, w.do(t, w.orgs.B, http.MethodGet, path, nil, nil), http.StatusOK, "org B read").Object(t)
	if readB["operatingMode"] != mktmodels.ModeHybrid || readB["organizationId"] != w.orgs.B.Org {
		t.Fatalf("org B read = %v", readB)
	}
}

// TestRow11_1b_OperatingModeEnforcedOnAcceptance proves a dispatcher cannot
// accept for a DELEGATED organization, and that the refusal happens before
// the source is asked to reserve anything.
func TestRow11_1b_OperatingModeEnforcedOnAcceptance(t *testing.T) {
	w := newS11World(t, "Row11 1b")
	projection, offer := w.openOffers(t)
	_, version := s11OperatingMode(t, w.pool, w.orgs.A.Org)
	const modePath = "/fleet-partner/operating-mode"
	rowsExpect(t, w.do(t, w.orgs.A, http.MethodPut, modePath,
		map[string]any{"operatingMode": mktmodels.ModeDelegated, "expectedVersion": version}, nil),
		http.StatusOK, "switch to DELEGATED")

	acceptPath := "/marketplace/offers/" + offer.ID + "/accept"
	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, acceptPath,
		map[string]any{"expectedVersion": offer.Version}, nil),
		http.StatusConflict, "marketplace_mode_not_allowed", "accept while DELEGATED")
	if got := w.execution.reserveCount(); got != 0 {
		t.Fatalf("source reservations while DELEGATED = %d, want 0", got)
	}
	if current := offerFor(t, w.pool, w.orgs.A.Org, projection.ID); current.Status != mktmodels.OfferOffered {
		t.Fatalf("refused acceptance changed the offer to %s", current.Status)
	}

	// HYBRID allows dispatcher acceptance.
	rowsExpect(t, w.do(t, w.orgs.A, http.MethodPut, modePath,
		map[string]any{"operatingMode": mktmodels.ModeHybrid, "expectedVersion": version + 1}, nil),
		http.StatusOK, "switch to HYBRID")
	rowsExpect(t, w.do(t, w.dispatcher, http.MethodPost, acceptPath,
		map[string]any{"expectedVersion": offer.Version}, nil), http.StatusOK, "accept while HYBRID")
	if got := w.execution.reserveCount(); got != 1 {
		t.Fatalf("source reservations = %d, want 1", got)
	}
}

// TestRow11_2_DispatcherAcceptsOfferForOrganization covers POST
// /marketplace/offers/:offerId/accept. It pins the defect it exposed: the
// expected version was accepted but never compared, so a stale accept went
// through to the source reservation.
func TestRow11_2_DispatcherAcceptsOfferForOrganization(t *testing.T) {
	w := newS11World(t, "Row11 2")
	projection, offer := w.openOffers(t)
	path := "/marketplace/offers/" + offer.ID + "/accept"
	body := map[string]any{"expectedVersion": offer.Version}

	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path, body,
		map[string]string{"Idempotency-Key": ""}), http.StatusBadRequest, "", "missing idempotency key")
	s11ExpectError(t, w.do(t, w.orgs.NoPermission, http.MethodPost, path, body, nil),
		http.StatusForbidden, "", "analyst accept")
	s11ExpectError(t, w.do(t, w.noRole, http.MethodPost, path, body, nil),
		http.StatusForbidden, "", "no-role accept")
	s11ExpectError(t, w.do(t, w.orgs.B, http.MethodPost, path, body, nil),
		http.StatusNotFound, "", "cross-tenant accept")
	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path,
		map[string]any{"expectedVersion": offer.Version + 1}, nil),
		http.StatusConflict, "expected_version_mismatch", "stale version")
	if got := w.execution.reserveCount(); got != 0 {
		t.Fatalf("refused accepts reached the source %d times", got)
	}

	key := map[string]string{"Idempotency-Key": ids.New("idem")}
	accepted := rowsExpect(t, w.do(t, w.dispatcher, http.MethodPost, path, body, key),
		http.StatusOK, "accept").Object(t)
	if accepted["status"] != mktmodels.OfferAccepted {
		t.Fatalf("accepted offer = %v", accepted)
	}
	if current := projectionFor(t, w.pool, w.orgs.A.Org, projection.PlatformRequestID); current.ProjectionStatus != mktmodels.ProjectionOrganizationAccepted {
		t.Fatalf("projection = %s, want ORGANIZATION_ACCEPTED", current.ProjectionStatus)
	}
	acceptances := `SELECT count(*) FROM fleet_partner_organization_acceptances
		WHERE organization_id=$1 AND offer_id=$2 AND accepted_by_user_id=$3 AND source_reservation_id<>''`
	if got := countWhere(t, w.pool, acceptances, w.orgs.A.Org, offer.ID, w.dispatcher.Member); got != 1 {
		t.Fatalf("acceptance records = %d, want 1", got)
	}
	if got := s11Audit(t, w.pool, w.orgs.A.Org, "marketplace.accepted_for_organization", projection.ID); got != 1 {
		t.Fatalf("acceptance audit rows = %d, want 1", got)
	}
	if got := rowsOutboxCount(t, w.pool, projection.ID, "organization.request.accepted.v1"); got != 1 {
		t.Fatalf("acceptance events = %d, want 1", got)
	}

	// The same command replays its stored outcome and commits nothing twice.
	replay := rowsExpect(t, w.do(t, w.dispatcher, http.MethodPost, path, body, key), http.StatusOK, "replay")
	if replay.Object(t)["id"] != accepted["id"] || w.execution.reserveCount() != 1 ||
		countWhere(t, w.pool, acceptances, w.orgs.A.Org, offer.ID, w.dispatcher.Member) != 1 {
		t.Fatalf("replay re-ran the acceptance: %s (reserves %d)", replay.Raw, w.execution.reserveCount())
	}
	// A new command against the won request is refused.
	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path, body, nil),
		http.StatusConflict, "request_not_assignable", "second accept")
	// Organization B's own copy is untouched by A's acceptance.
	projectionB := projectionFor(t, w.pool, w.orgs.B.Org, projection.PlatformRequestID)
	if offerB := offerFor(t, w.pool, w.orgs.B.Org, projectionB.ID); offerB.Status != mktmodels.OfferOffered {
		t.Fatalf("org B offer = %s, want OFFERED", offerB.Status)
	}
}

// TestRow11_3_DispatcherDeclinesOffer covers POST
// /marketplace/offers/:offerId/decline.
func TestRow11_3_DispatcherDeclinesOffer(t *testing.T) {
	w := newS11World(t, "Row11 3")
	projection, offer := w.openOffers(t)
	path := "/marketplace/offers/" + offer.ID + "/decline"
	body := map[string]any{"reasonCode": mktmodels.OrgDeclineOutsideZone, "note": "Lekki only this week"}

	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path,
		map[string]any{"reasonCode": "BORED"}, nil), http.StatusBadRequest, "decline_reason_invalid", "unknown reason")
	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path,
		map[string]any{}, nil), http.StatusBadRequest, "decline_reason_invalid", "missing reason")
	s11ExpectError(t, w.do(t, w.orgs.NoPermission, http.MethodPost, path, body, nil),
		http.StatusForbidden, "", "analyst decline")
	s11ExpectError(t, w.do(t, w.orgs.B, http.MethodPost, path, body, nil),
		http.StatusNotFound, "", "cross-tenant decline")
	if current := offerFor(t, w.pool, w.orgs.A.Org, projection.ID); current.Status != mktmodels.OfferOffered {
		t.Fatalf("refused declines changed the offer to %s", current.Status)
	}

	declined := rowsExpect(t, w.do(t, w.dispatcher, http.MethodPost, path, body, nil),
		http.StatusOK, "decline").Object(t)
	if declined["status"] != "DECLINED" {
		t.Fatalf("decline = %v", declined)
	}
	current := offerFor(t, w.pool, w.orgs.A.Org, projection.ID)
	if current.Status != mktmodels.OfferDeclined || current.ReasonCode == nil ||
		*current.ReasonCode != mktmodels.OrgDeclineOutsideZone {
		t.Fatalf("declined offer = %s/%v", current.Status, current.ReasonCode)
	}
	if p := projectionFor(t, w.pool, w.orgs.A.Org, projection.PlatformRequestID); p.ProjectionStatus != mktmodels.ProjectionOrganizationDeclined {
		t.Fatalf("projection = %s, want ORGANIZATION_DECLINED", p.ProjectionStatus)
	}
	if got := s11Audit(t, w.pool, w.orgs.A.Org, "marketplace.declined_for_organization", projection.ID); got != 1 {
		t.Fatalf("decline audit rows = %d, want 1", got)
	}
	if got := rowsOutboxCount(t, w.pool, projection.ID, "organization.request.declined.v1"); got != 1 {
		t.Fatalf("decline events = %d, want 1", got)
	}

	// A declined offer can be neither declined again nor accepted.
	if r := w.do(t, w.dispatcher, http.MethodPost, path, body, nil); r.Status != http.StatusNotFound && r.Status != http.StatusConflict {
		t.Fatalf("second decline = %d, want 404/409; body=%s", r.Status, r.Raw)
	}
	if r := w.do(t, w.dispatcher, http.MethodPost, "/marketplace/offers/"+offer.ID+"/accept",
		map[string]any{"expectedVersion": current.Version}, nil); r.Status != http.StatusConflict {
		t.Fatalf("accept after decline = %d, want 409; body=%s", r.Status, r.Raw)
	}
	if got := rowsOutboxCount(t, w.pool, projection.ID, "organization.request.declined.v1"); got != 1 {
		t.Fatalf("decline events after retries = %d, want 1", got)
	}
}

// s11Membership creates a managed provider in organization A with the given
// category, association state and home branch.
func s11Membership(t *testing.T, pool *pgxpool.Pool, org, category, branch string, associated bool) *providermodels.Membership {
	t.Helper()
	providerService := "driver-dispatch-delivery-service"
	if category == providermodels.CategoryHauling {
		providerService = "driver-hauling-service"
	}
	providerID := ids.New("s11prv")
	now := time.Now()
	membership := &providermodels.Membership{
		ProviderService: providerService, ServiceCategory: category, ProviderID: &providerID,
		OnboardingSource: providermodels.SourceSponsored, RelationshipType: providermodels.RelationshipEmployee,
		Status: providermodels.StatusActive,
	}
	if associated {
		membership.AssociationAcceptedAt = &now
	}
	if err := providerrepos.NewRepository(pool).CreateMembership(context.Background(), pool, org, membership); err != nil {
		t.Fatalf("seed %s membership: %v", category, err)
	}
	if associated {
		// The repository does not persist the association time on create.
		markMembershipSynced(t, pool, membership.ID)
	}
	if branch != "" {
		if _, err := pool.Exec(context.Background(), `UPDATE fleet_partner_organization_provider_memberships
			SET home_branch_id=$2 WHERE id=$1`, membership.ID, branch); err != nil {
			t.Fatalf("home branch: %v", err)
		}
	}
	return membership
}

// s11BranchDispatcher is a dispatcher whose only grant is at one branch.
func s11BranchDispatcher(t *testing.T, pool *pgxpool.Pool, org, branch string) rowsActor {
	t.Helper()
	member := seedMember(t, pool, org, seedIdentity(t, pool,
		"s11-branch-"+randomHex()[:8]+"@example.com", "+23481"+randomHex()[:8]), false, nil)
	bindRole(t, pool, org, member.ID, "rol_system_dispatcher", "BRANCH", branch)
	return rowsActor{Org: org, Member: member.ID}
}

// TestRow11_4_EligibleCandidates covers GET
// /marketplace/requests/:projectionId/candidates. It pins the defect it
// exposed: the list ignored branch scope, so a branch dispatcher saw (as
// eligible) providers homed in branches they cannot assign.
func TestRow11_4_EligibleCandidates(t *testing.T) {
	w := newS11World(t, "Row11 4")
	org := w.orgs.A.Org
	branchA := seedBranch(t, w.pool, org, "Row11 4 Branch A")
	branchB := seedBranch(t, w.pool, org, "Row11 4 Branch B")
	seedEligibleOrganization(t, w.pool, org)
	inA := s11Membership(t, w.pool, org, providermodels.CategoryDispatch, branchA, true)
	pending := s11Membership(t, w.pool, org, providermodels.CategoryDispatch, "", false)
	inB := s11Membership(t, w.pool, org, providermodels.CategoryDispatch, branchB, true)
	hauler := s11Membership(t, w.pool, org, providermodels.CategoryHauling, "", true)
	projection := seedProjection(t, w.pool, org, ids.New("req"), mktmodels.ProjectionOrganizationAccepted)
	path := "/marketplace/requests/" + projection.ID + "/candidates"

	byID := func(items []map[string]any) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, item := range items {
			out[item["membershipId"].(string)] = item
		}
		return out
	}
	all := byID(rowsExpect(t, w.do(t, w.dispatcher, http.MethodGet, path, nil, nil), http.StatusOK, "candidates").Items(t))
	if len(all) != 3 {
		t.Fatalf("candidates = %v, want the three dispatch providers", all)
	}
	if c := all[inA.ID]; c == nil || c["eligible"] != true || c["providerId"] != *inA.ProviderID {
		t.Fatalf("eligible candidate = %v", c)
	}
	if c := all[pending.ID]; c == nil || c["eligible"] != false || c["reason"] != "provider_association_pending" {
		t.Fatalf("association-pending candidate = %v", c)
	}
	if all[inB.ID] == nil {
		t.Fatal("organization-wide dispatcher must see every branch's providers")
	}
	if all[hauler.ID] != nil {
		t.Fatal("a hauling provider was offered for a dispatch request")
	}

	branchOnly := byID(rowsExpect(t, w.do(t, s11BranchDispatcher(t, w.pool, org, branchA), http.MethodGet,
		path+"?branchId="+branchB, nil, nil), http.StatusOK, "branch candidates").Items(t))
	if branchOnly[inA.ID] == nil || branchOnly[pending.ID] == nil || branchOnly[inB.ID] != nil || len(branchOnly) != 2 {
		t.Fatalf("branch-A dispatcher candidates = %v, want branch A + unbranched only", branchOnly)
	}

	s11ExpectError(t, w.do(t, w.noRole, http.MethodGet, path, nil, nil), http.StatusForbidden, "", "no-role candidates")
	s11ExpectError(t, w.do(t, w.orgs.B, http.MethodGet, path, nil, nil), http.StatusNotFound, "", "cross-tenant candidates")
}

// TestRow11_5_AssignRequestToProvider covers POST
// /marketplace/requests/:projectionId/assign.
func TestRow11_5_AssignRequestToProvider(t *testing.T) {
	w := newS11World(t, "Row11 5")
	org := w.orgs.A.Org
	branchA := seedBranch(t, w.pool, org, "Row11 5 Branch A")
	branchB := seedBranch(t, w.pool, org, "Row11 5 Branch B")
	provider := seedProviderMembership(t, w.pool, org, ids.New("s11drv"))
	providerB := s11Membership(t, w.pool, org, providermodels.CategoryDispatch, branchB, true)
	markMembershipSynced(t, w.pool, providerB.ID)
	projection := seedProjection(t, w.pool, org, ids.New("req"), mktmodels.ProjectionOrganizationAccepted)
	path := "/marketplace/requests/" + projection.ID + "/assign"
	body := map[string]any{"membershipId": provider.ID, "expectedRequestVersion": projection.Version}
	assignments := `SELECT count(*) FROM fleet_partner_provider_assignments WHERE request_projection_id=$1`

	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path, body,
		map[string]string{"Idempotency-Key": ""}), http.StatusBadRequest, "", "missing idempotency key")
	s11ExpectError(t, w.do(t, w.orgs.NoPermission, http.MethodPost, path, body, nil),
		http.StatusForbidden, "", "analyst assign")
	s11ExpectError(t, w.do(t, w.orgs.B, http.MethodPost, path, body, nil),
		http.StatusNotFound, "", "cross-tenant assign")
	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path,
		map[string]any{"membershipId": provider.ID, "responseTimeoutSeconds": 5}, nil),
		http.StatusUnprocessableEntity, "validation_failed", "timeout out of range")
	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path,
		map[string]any{"membershipId": provider.ID, "expectedRequestVersion": projection.Version + 3}, nil),
		http.StatusConflict, "expected_version_mismatch", "stale request version")
	s11ExpectError(t, w.do(t, s11BranchDispatcher(t, w.pool, org, branchA), http.MethodPost, path,
		map[string]any{"membershipId": providerB.ID}, nil), http.StatusNotFound, "", "out-of-branch provider")
	if got := countWhere(t, w.pool, assignments, projection.ID); got != 0 {
		t.Fatalf("refused assigns created %d assignments", got)
	}

	key := map[string]string{"Idempotency-Key": ids.New("idem")}
	created := rowsExpect(t, w.do(t, w.dispatcher, http.MethodPost, path, body, key),
		http.StatusCreated, "assign").Object(t)
	assignmentID, _ := created["id"].(string)
	if assignmentID == "" || created["status"] != mktmodels.AssignmentPendingProviderResponse ||
		created["membershipId"] != provider.ID {
		t.Fatalf("assignment = %v", created)
	}
	if p := projectionFor(t, w.pool, org, projection.PlatformRequestID); p.ProjectionStatus != mktmodels.ProjectionAssignmentPending {
		t.Fatalf("projection = %s, want ASSIGNMENT_PENDING", p.ProjectionStatus)
	}
	if got := countWhere(t, w.pool, `SELECT count(*) FROM fleet_partner_assignment_reservations
		WHERE assignment_id=$1 AND resource_id=$2 AND released_at IS NULL`, assignmentID, provider.ID); got != 1 {
		t.Fatalf("provider holds = %d, want 1", got)
	}
	if got := s11Audit(t, w.pool, org, "assignment.created", assignmentID); got != 1 {
		t.Fatalf("assignment audit rows = %d, want 1", got)
	}
	if got := rowsOutboxCount(t, w.pool, assignmentID, "organization.assignment.created.v1"); got != 1 {
		t.Fatalf("assignment events = %d, want 1", got)
	}

	replay := rowsExpect(t, w.do(t, w.dispatcher, http.MethodPost, path, body, key), http.StatusCreated, "replay").Object(t)
	if replay["id"] != assignmentID || countWhere(t, w.pool, assignments, projection.ID) != 1 {
		t.Fatalf("replay created another assignment: %v", replay)
	}
	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path,
		map[string]any{"membershipId": provider.ID}, nil), http.StatusConflict, "request_not_assignable", "second assign")
}

// s11Assignment seeds a pending assignment for a fresh request and provider,
// optionally homing the provider in a branch.
func s11Assignment(t *testing.T, pool *pgxpool.Pool, org, branch string) *mktmodels.Assignment {
	t.Helper()
	projection := seedProjection(t, pool, org, ids.New("req"), mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org, ids.New("s11drv"))
	if branch != "" {
		if _, err := pool.Exec(context.Background(), `UPDATE fleet_partner_organization_provider_memberships
			SET home_branch_id=$2 WHERE id=$1`, membership.ID, branch); err != nil {
			t.Fatalf("home branch: %v", err)
		}
	}
	return seedAssignment(t, pool, org, projection.ID, membership, time.Now().Add(time.Hour))
}

// TestRow11_6_ListAndReadAssignments covers GET /marketplace/assignments and
// GET /marketplace/assignments/:assignmentId (+ /history).
func TestRow11_6_ListAndReadAssignments(t *testing.T) {
	w := newS11World(t, "Row11 6")
	org := w.orgs.A.Org
	branchA := seedBranch(t, w.pool, org, "Row11 6 Branch A")
	branchB := seedBranch(t, w.pool, org, "Row11 6 Branch B")
	inA := s11Assignment(t, w.pool, org, branchA)
	inB := s11Assignment(t, w.pool, org, branchB)
	cancelled := s11Assignment(t, w.pool, org, "")
	if err := w.service.Cancel(context.Background(), mktusecases.CancelInput{Tenant: w.orgs.tenantA(),
		AssignmentID: cancelled.ID, Reason: "test"}); err != nil {
		t.Fatalf("cancel seed: %v", err)
	}
	other := s11Assignment(t, w.pool, w.orgs.B.Org, "")

	list := rowsIDs(rowsExpect(t, w.do(t, w.dispatcher, http.MethodGet, "/marketplace/assignments", nil, nil),
		http.StatusOK, "list").Items(t), "id")
	if !list[inA.ID] || !list[inB.ID] || !list[cancelled.ID] || list[other.ID] || len(list) != 3 {
		t.Fatalf("org A list = %v", list)
	}
	pending := rowsIDs(rowsExpect(t, w.do(t, w.dispatcher, http.MethodGet,
		"/marketplace/assignments?status=pending_provider_response", nil, nil), http.StatusOK, "status filter").Items(t), "id")
	if !pending[inA.ID] || !pending[inB.ID] || pending[cancelled.ID] || len(pending) != 2 {
		t.Fatalf("pending list = %v", pending)
	}
	limited := rowsExpect(t, w.do(t, w.dispatcher, http.MethodGet, "/marketplace/assignments?limit=1", nil, nil),
		http.StatusOK, "limit").Items(t)
	if len(limited) != 1 {
		t.Fatalf("limit=1 returned %d rows", len(limited))
	}
	if b := rowsIDs(rowsExpect(t, w.do(t, w.orgs.B, http.MethodGet, "/marketplace/assignments", nil, nil),
		http.StatusOK, "org B list").Items(t), "id"); !b[other.ID] || len(b) != 1 {
		t.Fatalf("org B list = %v", b)
	}
	branchDispatcher := s11BranchDispatcher(t, w.pool, org, branchA)
	if scoped := rowsIDs(rowsExpect(t, w.do(t, branchDispatcher, http.MethodGet,
		"/marketplace/assignments?branchId="+branchB, nil, nil), http.StatusOK, "branch list").Items(t), "id"); !scoped[inA.ID] ||
		scoped[inB.ID] || !scoped[cancelled.ID] || len(scoped) != 2 {
		t.Fatalf("branch-A dispatcher list = %v, want branch A + unbranched only", scoped)
	}
	s11ExpectError(t, w.do(t, w.noRole, http.MethodGet, "/marketplace/assignments", nil, nil),
		http.StatusForbidden, "", "no-role list")

	detail := rowsExpect(t, w.do(t, w.dispatcher, http.MethodGet, "/marketplace/assignments/"+inA.ID, nil, nil),
		http.StatusOK, "detail").Object(t)
	if detail["id"] != inA.ID || detail["status"] != mktmodels.AssignmentPendingProviderResponse {
		t.Fatalf("detail = %v", detail)
	}
	history := rowsExpect(t, w.do(t, w.dispatcher, http.MethodGet, "/marketplace/assignments/"+cancelled.ID+"/history", nil, nil),
		http.StatusOK, "history").Items(t)
	if len(history) < 2 || history[0]["toStatus"] != mktmodels.AssignmentCancelled {
		t.Fatalf("history = %v", history)
	}
	s11ExpectError(t, w.do(t, w.orgs.B, http.MethodGet, "/marketplace/assignments/"+inA.ID, nil, nil),
		http.StatusNotFound, "", "cross-tenant detail")
	s11ExpectError(t, w.do(t, w.orgs.B, http.MethodGet, "/marketplace/assignments/"+inA.ID+"/history", nil, nil),
		http.StatusNotFound, "", "cross-tenant history")
	s11ExpectError(t, w.do(t, branchDispatcher, http.MethodGet, "/marketplace/assignments/"+inB.ID, nil, nil),
		http.StatusNotFound, "", "out-of-branch detail")
	s11ExpectError(t, w.do(t, w.noRole, http.MethodGet, "/marketplace/assignments/"+inA.ID, nil, nil),
		http.StatusForbidden, "", "no-role detail")
}

// TestRow11_7a_CancelAssignment covers a successful POST
// /marketplace/assignments/:assignmentId/cancel and its refusals.
func TestRow11_7a_CancelAssignment(t *testing.T) {
	w := newS11World(t, "Row11 7a")
	org := w.orgs.A.Org
	assignment := s11Assignment(t, w.pool, org, "")
	path := "/marketplace/assignments/" + assignment.ID + "/cancel"
	body := map[string]any{"reason": "customer_changed_plan", "expectedVersion": assignment.Version}

	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path,
		map[string]any{"expectedVersion": assignment.Version}, nil),
		http.StatusUnprocessableEntity, "validation_failed", "missing reason")
	s11ExpectError(t, w.do(t, w.orgs.NoPermission, http.MethodPost, path, body, nil),
		http.StatusForbidden, "", "analyst cancel")
	s11ExpectError(t, w.do(t, w.orgs.B, http.MethodPost, path, body, nil),
		http.StatusNotFound, "", "cross-tenant cancel")
	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path,
		map[string]any{"reason": "customer_changed_plan", "expectedVersion": assignment.Version + 5}, nil),
		http.StatusConflict, "expected_version_mismatch", "stale version")
	holds := `SELECT count(*) FROM fleet_partner_assignment_reservations WHERE assignment_id=$1 AND released_at IS NULL`
	if got := countWhere(t, w.pool, holds, assignment.ID); got != 1 {
		t.Fatalf("refused cancels released holds: %d left, want 1", got)
	}

	rowsExpect(t, w.do(t, w.dispatcher, http.MethodPost, path, body, nil), http.StatusOK, "cancel")
	reloaded, err := mktrepos.NewAssignmentRepository(w.pool).GetByID(context.Background(), org, assignment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != mktmodels.AssignmentCancelled || reloaded.CancelledAt == nil {
		t.Fatalf("assignment = %s, want CANCELLED", reloaded.Status)
	}
	if got := countWhere(t, w.pool, holds, assignment.ID); got != 0 {
		t.Fatalf("provider holds after cancel = %d, want 0", got)
	}
	if got := s11Audit(t, w.pool, org, "assignment.cancelled", assignment.ID); got != 1 {
		t.Fatalf("cancel audit rows = %d, want 1", got)
	}
	if got := rowsOutboxCount(t, w.pool, assignment.ID, "organization.assignment.cancelled.v1"); got != 1 {
		t.Fatalf("cancel events = %d, want 1", got)
	}
	s11ExpectError(t, w.do(t, w.dispatcher, http.MethodPost, path,
		map[string]any{"reason": "again"}, nil), http.StatusConflict, "assignment_cannot_cancel", "second cancel")
	if got := rowsOutboxCount(t, w.pool, assignment.ID, "organization.assignment.cancelled.v1"); got != 1 {
		t.Fatalf("cancel events after retry = %d, want 1", got)
	}
}

// s11AlertSink is the Section 16 operational alert service as the expiry
// worker builds it (app.BuildMarketplaceExpiryAlertSink), with the alerts
// flag under test control.
func s11AlertSink(pool *pgxpool.Pool, enabled bool) *dispatch.Service {
	return dispatch.NewService(dispatch.Options{
		Pool:   pool,
		Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Flags:  dispatch.Flags{OperationalAlerts: enabled},
	})
}

// TestRow11_11c_ExpiredAssignmentRaisesOneDispatcherAlert proves the
// assignment expiry worker raises one ASSIGNMENT_EXPIRED operational alert
// per expired marketplace assignment, scoped to the provider's branch, through
// the shared Section 16 alert path (audit + operational_alert.created.v1 with
// its notification intent), and never a second one on re-run or replay.
func TestRow11_11c_ExpiredAssignmentRaisesOneDispatcherAlert(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Row11 11c Co", "RC-"+ids.New("r11c")).ID
	branch := seedBranch(t, pool, org, "Row11 11c Branch")
	overdue := func(branchID string) *mktmodels.Assignment {
		projection := seedProjection(t, pool, org, ids.New("req"), mktmodels.ProjectionOrganizationAccepted)
		membership := seedProviderMembership(t, pool, org, ids.New("s11drv"))
		if branchID != "" {
			if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organization_provider_memberships
				SET home_branch_id=$2 WHERE id=$1`, membership.ID, branchID); err != nil {
				t.Fatal(err)
			}
		}
		return seedAssignment(t, pool, org, projection.ID, membership, time.Now().Add(-time.Minute))
	}
	branched, unbranched := overdue(branch), overdue("")

	sink := s11AlertSink(pool, true)
	service := newExpiryService(pool)
	service.SetExpiryAlertSink(sink)
	alerts := func(assignmentID string) int {
		return countWhere(t, pool, `SELECT count(*) FROM fleet_partner_operational_alerts
			WHERE organization_id=$1 AND alert_type='ASSIGNMENT_EXPIRED' AND source_type='MARKETPLACE_ASSIGNMENT'
			  AND source_id=$2 AND assignment_id=$2`, org, assignmentID)
	}
	created := func() int {
		return countWhere(t, pool, `SELECT count(*) FROM fleet_partner_outbox o
			JOIN fleet_partner_operational_alerts a ON a.id = o.aggregate_id
			WHERE a.organization_id=$1 AND o.event_type='operational_alert.created.v1'
			  AND o.payload->'payload'->>'notificationIntent' = 'true'`, org)
	}

	if _, err := service.ExpireAssignments(ctx, 50); err != nil {
		t.Fatalf("expire: %v", err)
	}
	for _, a := range []*mktmodels.Assignment{branched, unbranched} {
		if got := alerts(a.ID); got != 1 {
			t.Fatalf("alerts for %s = %d, want 1", a.ID, got)
		}
	}
	var alertBranch *string
	var severity, status, alertID string
	if err := pool.QueryRow(ctx, `SELECT id, branch_id, severity, status FROM fleet_partner_operational_alerts
		WHERE organization_id=$1 AND source_id=$2`, org, branched.ID).Scan(&alertID, &alertBranch, &severity, &status); err != nil {
		t.Fatal(err)
	}
	if alertBranch == nil || *alertBranch != branch || severity != "HIGH" || status != "OPEN" {
		t.Fatalf("alert = branch %v severity %s status %s", alertBranch, severity, status)
	}
	if got := s11Audit(t, pool, org, "operational_alert.created", alertID); got != 1 {
		t.Fatalf("alert audit rows = %d, want 1", got)
	}
	if got := created(); got != 2 {
		t.Fatalf("alert-created events = %d, want 2", got)
	}

	// Worker re-run: nothing is pending any more, nothing new is raised.
	if n, err := service.ExpireAssignments(ctx, 50); err != nil || n != 0 {
		t.Fatalf("re-run expired %d (%v), want 0", n, err)
	}
	// Replay after a dispatcher resolved the alert: still no second alert.
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_operational_alerts SET status='RESOLVED', resolved_at=now()
		WHERE id=$1`, alertID); err != nil {
		t.Fatal(err)
	}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return sink.MarketplaceAssignmentExpired(ctx, tx, org, branch, branched.ID)
	}); err != nil {
		t.Fatalf("replayed alert: %v", err)
	}
	if got := alerts(branched.ID); got != 1 {
		t.Fatalf("alerts after replay = %d, want 1", got)
	}
	if got := created(); got != 2 {
		t.Fatalf("alert-created events after re-run and replay = %d, want 2", got)
	}

	// Alerts switched off: expiry still releases the provider, no alert.
	quiet := overdue("")
	off := newExpiryService(pool)
	off.SetExpiryAlertSink(s11AlertSink(pool, false))
	if n, err := off.ExpireAssignments(ctx, 50); err != nil || n < 1 {
		t.Fatalf("expire with alerts off = %d (%v)", n, err)
	}
	reloaded, err := mktrepos.NewAssignmentRepository(pool).GetByID(ctx, org, quiet.ID)
	if err != nil || reloaded.Status != mktmodels.AssignmentExpired {
		t.Fatalf("assignment with alerts off = %v (%v)", reloaded, err)
	}
	if got := alerts(quiet.ID); got != 0 {
		t.Fatalf("alerts while disabled = %d, want 0", got)
	}
}
