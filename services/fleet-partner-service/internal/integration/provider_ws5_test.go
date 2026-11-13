package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	orghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/http"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Shared fixtures
// ---------------------------------------------------------------------------

type ws5Fixture struct {
	pool     *pgxpool.Pool
	repo     *providerrepos.Repository
	service  *providerusecases.Service
	sender   *authclients.FakeSender
	org      string
	branch   string
	scope    tenant.Context
	workflow *workforce.Service
}

func newWS5Fixture(t *testing.T, name string) *ws5Fixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, name, "RC-"+ids.New("reg")[4:14])
	branch := seedBranch(t, pool, org.ID, "Home "+randomHex()[:6])
	repo := providerrepos.NewRepository(pool)
	sender := authclients.NewFakeSender()
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	service := providerusecases.NewService(providerusecases.Options{
		Pool: pool, Providers: repo, Organizations: activeOrgReader{}, Events: publisher, Sender: sender,
		InvitationSecret: []byte("ws5-invitation-secret"), AcceptURLTemplate: "https://fleet.example.test/provider/accept",
	})
	w := workforce.NewService(workforce.Options{Pool: pool, Events: publisher, Flags: workforce.Flags{
		Workforce: true, Shifts: true, Leave: true, Qualifications: true, Training: true, Performance: true}})
	return &ws5Fixture{pool: pool, repo: repo, service: service, sender: sender, org: org.ID, branch: branch,
		workflow: w, scope: tenant.Context{OrganizationID: org.ID, OrganizationUserID: "oru_ws5",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "oru_ws5", RequestID: "req-ws5", CorrelationID: "corr-ws5"}}
}

func (f *ws5Fixture) membership(t *testing.T, status string) *providermodels.Membership {
	t.Helper()
	providerID := "prv_ws5_" + randomHex()
	m := &providermodels.Membership{
		ProviderService: "driver-dispatch-delivery-service", ServiceCategory: providermodels.CategoryDispatch,
		OnboardingSource: providermodels.SourceSponsored, RelationshipType: providermodels.RelationshipEmployee,
		Status: status, ProviderID: &providerID, HomeBranchID: &f.branch,
	}
	if err := f.repo.CreateMembership(context.Background(), f.pool, f.org, m); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return m
}

// invitation seeds an invitation as the create flow leaves it: just handed
// to notification-service, so still inside the resend cooldown.
func (f *ws5Fixture) invitation(t *testing.T, m *providermodels.Membership, status string) *providermodels.Invitation {
	t.Helper()
	ctx := context.Background()
	inv := &providermodels.Invitation{MembershipID: &m.ID, ProviderService: m.ProviderService, LegalName: "Ada Rider",
		EmailNormalized: "ada.rider." + randomHex()[:6] + "@example.com", PhoneE164: "+2348031234567",
		InvitationTokenHash: "seed-hash-" + randomHex(), ExpiresAt: time.Now().Add(7 * 24 * time.Hour), CreatedBy: "oru_ws5"}
	if err := f.repo.CreateInvitation(ctx, f.pool, f.org, inv); err != nil {
		t.Fatalf("seed invitation: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_provider_invitations SET status = $2,
		delivery_status = 'ACCEPTED', last_sent_at = now() WHERE id = $1`, inv.ID, status); err != nil {
		t.Fatalf("seed invitation state: %v", err)
	}
	return inv
}

func (f *ws5Fixture) agePastCooldown(t *testing.T, invitationID string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE fleet_partner_provider_invitations
		SET last_sent_at = now() - interval '2 minutes' WHERE id = $1`, invitationID); err != nil {
		t.Fatal(err)
	}
}

func (f *ws5Fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// providerAdminRouter mounts the real organization provider routes behind an
// injected tenant context and the real, database-backed authorizer.
func providerAdminRouter(t *testing.T, pool *pgxpool.Pool, service *providerusecases.Service, orgID, userID string, branches []string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := orghttp.NewHandler(nil, nil, service, accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0), nil)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: orgID, OrganizationUserID: userID,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: userID, PermittedBranchIDs: branches})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	handler.RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	return engine
}

func ws5Call(t *testing.T, router http.Handler, method, path string, headers map[string]string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	var body map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder.Code, body
}

func ws5ReasonCodes(a *providerusecases.ProviderAvailability) []string {
	out := []string{}
	for _, r := range a.Reasons {
		out = append(out, r.Code+":"+r.Effect)
	}
	return out
}

func ws5HasReason(a *providerusecases.ProviderAvailability, code string) bool {
	for _, r := range a.Reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// FLEET-10.8 resend-login
// ---------------------------------------------------------------------------

func TestProviderResendLogin_InvitationRotationRateLimitIdempotencyAndAudit(t *testing.T) {
	f := newWS5Fixture(t, "Resend Login Co")
	ctx := context.Background()
	m := f.membership(t, providermodels.StatusPendingProviderAcceptance)
	inv := f.invitation(t, m, providermodels.InvitationPending)
	in := providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: m.ID, IdempotencyKey: "resend-1"}

	// Idempotency-Key is required.
	if _, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: m.ID}); appStatus(err) != 400 {
		t.Fatalf("missing key = %v", err)
	}

	// Inside the cooldown of the original send: refused.
	if _, err := f.service.ResendLogin(ctx, in); appStatus(err) != 429 {
		t.Fatalf("resend inside the original cooldown = %v", err)
	}
	f.agePastCooldown(t, inv.ID)
	var oldHash string
	_ = f.pool.QueryRow(ctx, `SELECT invitation_token_hash FROM fleet_partner_provider_invitations WHERE id=$1`, inv.ID).Scan(&oldHash)

	result, err := f.service.ResendLogin(ctx, in)
	if err != nil {
		t.Fatalf("resend: %v", err)
	}
	if result.Notice != providerusecases.NoticeAssociationInvitation || result.InvitationID != inv.ID ||
		result.ResendCount != 1 || result.DeliveryStatus != "ACCEPTED" || result.Channel != "email" ||
		result.SignInMethod != "email_and_phone_otp" || result.Replayed || result.InvitationExpiresAt == nil {
		t.Fatalf("result = %+v", result)
	}
	var newHash string
	var resendCount int
	_ = f.pool.QueryRow(ctx, `SELECT invitation_token_hash, resend_count FROM fleet_partner_provider_invitations WHERE id=$1`, inv.ID).Scan(&newHash, &resendCount)
	if newHash == oldHash || resendCount != 1 {
		t.Fatalf("token not rotated or count wrong: %v %d", newHash == oldHash, resendCount)
	}
	sent, _ := f.sender.LastInvitation()
	if sent.EventType != authclients.EventCorporateProviderInvitation || !strings.Contains(sent.AcceptURL, "token=") ||
		sent.Email == "" || len(f.sender.SentOTPs) != 0 {
		t.Fatalf("notice = %+v", sent)
	}
	token := strings.SplitN(sent.AcceptURL, "token=", 2)[1]
	if auth.HashRefreshToken([]byte("ws5-invitation-secret"), token) != newHash {
		t.Fatal("emailed link does not carry the rotated token")
	}
	// The raw token is never persisted or published.
	if f.count(t, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND payload::text LIKE '%'||$2||'%'`, f.org, token) != 0 {
		t.Fatal("token leaked into the outbox")
	}

	// Replaying the same key returns the stored result without a second email.
	before := len(f.sender.SentInvitations)
	replay, err := f.service.ResendLogin(ctx, in)
	if err != nil || !replay.Replayed || replay.InvitationID != inv.ID || replay.ResendCount != 1 || len(f.sender.SentInvitations) != before {
		t.Fatalf("replay = %+v %v", replay, err)
	}
	// A new key inside the cooldown is rate limited.
	if _, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: m.ID, IdempotencyKey: "resend-2"}); appStatus(err) != 429 {
		t.Fatalf("cooldown = %v", err)
	}

	// Concurrency: two different keys at once -> exactly one is sent.
	f.agePastCooldown(t, inv.ID)
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: m.ID,
				IdempotencyKey: "race-" + string(rune('a'+i))})
		}(i)
	}
	wg.Wait()
	ok, limited := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case appStatus(err) == 429:
			limited++
		default:
			t.Fatalf("concurrent resend error: %v", err)
		}
	}
	if ok != 1 || limited != 1 {
		t.Fatalf("concurrent resends ok=%d limited=%d", ok, limited)
	}

	// Delivery failure: the claim is released and the failure audited.
	f.agePastCooldown(t, inv.ID)
	f.sender.FailWith = errors.New("notification-service down")
	var countBefore int
	_ = f.pool.QueryRow(ctx, `SELECT resend_count FROM fleet_partner_provider_invitations WHERE id=$1`, inv.ID).Scan(&countBefore)
	if _, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: m.ID, IdempotencyKey: "fail-1"}); appStatus(err) != 503 {
		t.Fatalf("delivery failure = %v", err)
	}
	var countAfter int
	var delivery string
	_ = f.pool.QueryRow(ctx, `SELECT resend_count, delivery_status FROM fleet_partner_provider_invitations WHERE id=$1`, inv.ID).Scan(&countAfter, &delivery)
	if countAfter != countBefore || delivery != "UNAVAILABLE" {
		t.Fatalf("failed delivery left count=%d (before %d) delivery=%s", countAfter, countBefore, delivery)
	}
	f.sender.FailWith = nil
	// The failed key was released: the same key can be retried immediately.
	if _, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: m.ID, IdempotencyKey: "fail-1"}); err != nil {
		t.Fatalf("retry after failed delivery: %v", err)
	}

	// Cap.
	f.agePastCooldown(t, inv.ID)
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_provider_invitations SET resend_count=$2 WHERE id=$1`, inv.ID, providerusecases.MaxLoginResends); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: m.ID, IdempotencyKey: "cap-1"}); appStatus(err) != 429 {
		t.Fatalf("cap = %v", err)
	}

	// Audit and outbox.
	if n := f.count(t, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND resource_id=$2 AND action='provider.login_resent'`, f.org, m.ID); n != 3 {
		t.Fatalf("login_resent audit rows = %d, want 3", n)
	}
	if n := f.count(t, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND resource_id=$2 AND action='provider.login_resend_failed'`, f.org, m.ID); n != 1 {
		t.Fatalf("login_resend_failed audit rows = %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND aggregate_id=$2 AND event_type='organization.provider.login_resent.v1'`, f.org, m.ID); n != 3 {
		t.Fatalf("login_resent events = %d, want 3", n)
	}
}

func TestProviderResendLogin_StatesExpiryScopeAndFirstLoginNotice(t *testing.T) {
	f := newWS5Fixture(t, "Resend Login States Co")
	ctx := context.Background()

	// An invitation whose link lapsed is replaced, not resurrected.
	lapsed := f.membership(t, providermodels.StatusPendingProviderAcceptance)
	old := f.invitation(t, lapsed, providermodels.InvitationExpired)
	result, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: lapsed.ID, IdempotencyKey: "lapsed-1"})
	if err != nil || result.InvitationID == old.ID || result.Notice != providerusecases.NoticeAssociationInvitation {
		t.Fatalf("lapsed resend = %+v %v", result, err)
	}
	var oldStatus, newStatus string
	_ = f.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_provider_invitations WHERE id=$1`, old.ID).Scan(&oldStatus)
	_ = f.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_provider_invitations WHERE id=$1`, result.InvitationID).Scan(&newStatus)
	if oldStatus != "EXPIRED" || newStatus != "PENDING" {
		t.Fatalf("lapsed=%s replacement=%s", oldStatus, newStatus)
	}

	// PENDING_SETUP: first-login notice, no token, no link, invitation untouched.
	setup := f.membership(t, providermodels.StatusPendingSetup)
	accepted := f.invitation(t, setup, providermodels.InvitationAccepted)
	if _, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: setup.ID, IdempotencyKey: "setup-early"}); appStatus(err) != 429 {
		t.Fatalf("first-login notice inside the cooldown = %v", err)
	}
	f.agePastCooldown(t, accepted.ID)
	var hashBefore string
	_ = f.pool.QueryRow(ctx, `SELECT invitation_token_hash FROM fleet_partner_provider_invitations WHERE id=$1`, accepted.ID).Scan(&hashBefore)
	notice, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: setup.ID, IdempotencyKey: "setup-1"})
	if err != nil || notice.Notice != providerusecases.NoticeFirstLogin || notice.InvitationExpiresAt != nil {
		t.Fatalf("first-login notice = %+v %v", notice, err)
	}
	sent, _ := f.sender.LastInvitation()
	if sent.EventType != authclients.EventCorporateProviderLoginNotice || sent.AcceptURL != "" || sent.InvitationToken != "" {
		t.Fatalf("first-login message = %+v", sent)
	}
	var hashAfter, status string
	_ = f.pool.QueryRow(ctx, `SELECT invitation_token_hash, status FROM fleet_partner_provider_invitations WHERE id=$1`, accepted.ID).Scan(&hashAfter, &status)
	if hashAfter != hashBefore || status != "ACCEPTED" {
		t.Fatal("first-login notice must not touch the consumed invitation token or state")
	}

	// Nothing is pending for an active provider.
	active := f.membership(t, providermodels.StatusActive)
	f.invitation(t, active, providermodels.InvitationAccepted)
	if _, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: f.scope, MembershipID: active.ID, IdempotencyKey: "active-1"}); appStatus(err) != 409 {
		t.Fatalf("active provider = %v", err)
	}

	// Cross-tenant and out-of-branch are not found.
	other := createOrganization(t, f.pool, "Resend Other Co", "RC-"+ids.New("reg")[4:14])
	intruder := f.scope
	intruder.OrganizationID = other.ID
	if _, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: intruder, MembershipID: setup.ID, IdempotencyKey: "x-1"}); appStatus(err) != 404 {
		t.Fatalf("cross-tenant = %v", err)
	}
	scoped := f.scope
	scoped.PermittedBranchIDs = []string{"brn_elsewhere"}
	if _, err := f.service.ResendLogin(ctx, providerusecases.ResendLoginInput{Tenant: scoped, MembershipID: setup.ID, IdempotencyKey: "x-2"}); appStatus(err) != 404 {
		t.Fatalf("out-of-branch = %v", err)
	}

	// Through the real route: providers.manage is required.
	owner := seedAuditRouteUser(t, f.pool, f.org, "rol_system_owner")
	dispatcher := seedAuditRouteUser(t, f.pool, f.org, "rol_system_dispatcher")
	f.agePastCooldown(t, accepted.ID)
	path := "/fleet-partner/v1/providers/" + setup.ID + "/resend-login"
	if code, body := ws5Call(t, providerAdminRouter(t, f.pool, f.service, f.org, dispatcher, nil), "POST", path, map[string]string{"Idempotency-Key": "route-1"}); code != 403 {
		t.Fatalf("dispatcher resend = %d %v", code, body)
	}
	code, body := ws5Call(t, providerAdminRouter(t, f.pool, f.service, f.org, owner, nil), "POST", path, map[string]string{"Idempotency-Key": "route-1"})
	if code != 200 || body["data"].(map[string]any)["notice"] != providerusecases.NoticeFirstLogin {
		t.Fatalf("owner resend = %d %v", code, body)
	}
}

// ---------------------------------------------------------------------------
// FLEET-10.11 availability, FLEET-10.12 branches / vehicles / service roles
// ---------------------------------------------------------------------------

func TestProviderAvailabilityIsDerivedAndExplainable(t *testing.T) {
	f := newWS5Fixture(t, "Availability Co")
	ctx := context.Background()
	m := f.membership(t, providermodels.StatusActive)
	scope := f.scope

	check := func(want string, codes ...string) *providerusecases.ProviderAvailability {
		t.Helper()
		a, err := f.service.Availability(ctx, scope, m.ID)
		if err != nil {
			t.Fatalf("availability: %v", err)
		}
		if a.Status != want {
			t.Fatalf("status = %s want %s; reasons %v", a.Status, want, ws5ReasonCodes(a))
		}
		for _, code := range codes {
			if !ws5HasReason(a, code) {
				t.Fatalf("missing reason %s in %v", code, ws5ReasonCodes(a))
			}
		}
		return a
	}

	base := check(providerusecases.AvailabilityAvailable, "NO_SHIFT_SCHEDULE_IN_FORCE")
	if base.Inputs.MembershipStatus != "ACTIVE" || base.Inputs.Tracking.FreshnessStatus != "UNKNOWN" || base.Derivation == "" {
		t.Fatalf("inputs = %+v", base.Inputs)
	}

	// Mandatory qualification: missing, then expired, then expiring soon.
	q, err := f.workflow.CreateQualification(ctx, scope, workforce.Qualification{Code: "RIDER-" + randomHex()[:4], Name: "Rider licence", ApplicableServices: []string{"DISPATCH"}})
	if err != nil {
		t.Fatal(err)
	}
	check(providerusecases.AvailabilityUnavailable, "QUALIFICATION_MISSING")
	past := time.Now().Add(-time.Hour)
	pq, err := f.workflow.AddProviderQualification(ctx, scope, m.ID, workforce.ProviderQualification{QualificationID: q.ID, ExpiresAt: &past})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.workflow.QualificationAction(ctx, scope, pq.ID, "VERIFIED", "ver-1", pq.Version); err != nil {
		t.Fatal(err)
	}
	expired := check(providerusecases.AvailabilityUnavailable, "QUALIFICATION_EXPIRED")
	if len(expired.Inputs.Qualifications.Expired) != 1 || expired.Inputs.Qualifications.MandatoryCount != 1 {
		t.Fatalf("qualification inputs = %+v", expired.Inputs.Qualifications)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_provider_qualifications SET expires_at = now() + interval '10 days' WHERE id=$1`, pq.ID); err != nil {
		t.Fatal(err)
	}
	check(providerusecases.AvailabilityAvailable, "QUALIFICATION_EXPIRING_SOON")

	// Approved leave covering now.
	now := time.Now().UTC()
	leave, err := f.workflow.CreateLeave(ctx, scope, workforce.CreateLeaveInput{MembershipID: m.ID, LeaveType: "SICK", Reason: "flu", StartAt: now.Add(-time.Minute), EndAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	leave, err = f.workflow.LeaveAction(ctx, scope, leave.ID, "APPROVED", leave.Version)
	if err != nil {
		t.Fatal(err)
	}
	onLeave := check(providerusecases.AvailabilityUnavailable, "APPROVED_LEAVE")
	if onLeave.Inputs.ApprovedLeave == nil || onLeave.Inputs.ApprovedLeave.LeaveID != leave.ID {
		t.Fatalf("leave input = %+v", onLeave.Inputs.ApprovedLeave)
	}
	if _, err = f.workflow.LeaveAction(ctx, scope, leave.ID, "CANCELLED", leave.Version); err != nil {
		t.Fatal(err)
	}

	// A published shift covering now: off shift until assigned.
	shift, err := f.workflow.CreateShift(ctx, scope, workforce.CreateShiftInput{BranchID: f.branch, Name: "Day", Role: "RIDER", Timezone: "Africa/Lagos",
		StartAt: now.Add(-time.Hour), EndAt: now.Add(6 * time.Hour), RequiredCapacity: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.workflow.UpdateShift(ctx, scope, shift.ID, shift.Version, "", nil, nil, "PUBLISHED"); err != nil {
		t.Fatal(err)
	}
	check(providerusecases.AvailabilityUnavailable, "OFF_SHIFT")
	if _, err = f.workflow.AssignShift(ctx, scope, shift.ID, workforce.AssignShiftInput{MembershipID: m.ID, AssignmentType: "PRIMARY"}); err != nil {
		t.Fatal(err)
	}
	onShift := check(providerusecases.AvailabilityAvailable)
	if !onShift.Inputs.Shift.OnShift || !onShift.Inputs.Shift.ScheduleInForce || onShift.Inputs.Shift.CurrentShiftAssignmentID == nil {
		t.Fatalf("shift inputs = %+v", onShift.Inputs.Shift)
	}

	// Live company work makes the provider busy; stale tracking is explained.
	seedEligibleOrganization(t, f.pool, f.org)
	projection := seedProjection(t, f.pool, f.org, "req-ws5-"+randomHex(), "ORGANIZATION_ACCEPTED")
	seedAssignment(t, f.pool, f.org, projection.ID, m, now.Add(time.Minute))
	if _, err := f.pool.Exec(ctx, `INSERT INTO fleet_partner_tracking_projections(organization_id,source_service,canonical_trip_reference,provider_id,
		source_timestamp,received_at,source_version,payload_hash,freshness_status,gap_status,retention_expires_at)
		VALUES($1,'driver-dispatch-delivery-service',$2,$3,now()-interval '20 minutes',now(),1,'h','STALE','CURRENT',now()+interval '1 day')`,
		f.org, "trip-ws5-"+randomHex(), *m.ProviderID); err != nil {
		t.Fatal(err)
	}
	auditBefore := f.count(t, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1`, f.org)
	outboxBefore := f.count(t, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1`, f.org)
	busy := check(providerusecases.AvailabilityBusy, "ACTIVE_COMPANY_WORK", "TRACKING_NOT_FRESH")
	if busy.Inputs.ActiveWork.Marketplace != 1 || busy.Inputs.Tracking.FreshnessStatus != "STALE" || busy.Inputs.Tracking.LastObservedAt == nil {
		t.Fatalf("work/tracking inputs = %+v %+v", busy.Inputs.ActiveWork, busy.Inputs.Tracking)
	}

	// Read-only: evaluating availability writes nothing.
	if f.count(t, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1`, f.org) != auditBefore ||
		f.count(t, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1`, f.org) != outboxBefore {
		t.Fatal("availability wrote audit or outbox rows")
	}

	// A suspended membership is unavailable whatever else holds.
	if err := f.service.ChangeStatus(ctx, providerusecases.ChangeStatusInput{Tenant: scope, MembershipID: m.ID, TargetStatus: providermodels.StatusSuspended}); err != nil {
		t.Fatal(err)
	}
	check(providerusecases.AvailabilityUnavailable, "MEMBERSHIP_NOT_ACTIVE")

	// Tenant isolation.
	other := createOrganization(t, f.pool, "Availability Other", "RC-"+ids.New("reg")[4:14])
	intruder := scope
	intruder.OrganizationID = other.ID
	if _, err := f.service.Availability(ctx, intruder, m.ID); appStatus(err) != 404 {
		t.Fatalf("cross-tenant availability = %v", err)
	}
}

func TestProviderBranchesVehiclesAndServiceRoles(t *testing.T) {
	f := newWS5Fixture(t, "Provider Views Co")
	ctx := context.Background()
	m := f.membership(t, providermodels.StatusActive)
	second := seedBranch(t, f.pool, f.org, "Second "+randomHex()[:6])
	if _, err := f.workflow.CreateProfile(ctx, f.scope, workforce.CreateProfileInput{BranchID: second, MembershipID: m.ID,
		EmployeeNumber: "EMP-" + randomHex()[:6], RelationshipType: "MANAGED_PROVIDER", Role: "RIDER", StartDate: time.Now()}); err != nil {
		t.Fatal(err)
	}

	branches, err := f.service.ProviderBranches(ctx, f.scope, m.ID)
	if err != nil || len(branches) != 2 || branches[0].BranchID != f.branch || branches[0].Sources[0] != providerusecases.BranchSourceHome ||
		branches[1].BranchID != second || branches[1].Sources[0] != providerusecases.BranchSourceWorkforce {
		t.Fatalf("branches = %+v %v", branches, err)
	}
	homeOnly := f.scope
	homeOnly.PermittedBranchIDs = []string{f.branch}
	if scoped, err := f.service.ProviderBranches(ctx, homeOnly, m.ID); err != nil || len(scoped) != 1 {
		t.Fatalf("branch-scoped branches = %+v %v", scoped, err)
	}
	secondOnly := f.scope
	secondOnly.PermittedBranchIDs = []string{second}
	if _, err := f.service.ProviderBranches(ctx, secondOnly, m.ID); appStatus(err) != 404 {
		t.Fatalf("provider outside the actor's branch scope = %v", err)
	}

	// Vehicles come from the asset allocation, current by default.
	a := assets.NewService(assets.Options{Pool: f.pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(f.pool)),
		Flags: assets.Flags{Assets: true, Assignments: true}})
	asset, err := a.CreateAsset(ctx, f.scope, assets.Asset{BranchID: f.branch, AssetCode: "BIKE-" + randomHex()[:6], AssetType: "MOTORCYCLE", OwnershipType: "OWNED"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Assign(ctx, f.scope, asset.ID, assets.Assignment{ProviderMembershipID: m.ID, AssignmentType: "PRIMARY"}); err != nil {
		t.Fatal(err)
	}
	vehicles, err := f.service.ProviderVehicles(ctx, f.scope, m.ID, false)
	if err != nil || len(vehicles) != 1 || vehicles[0].AssetID != asset.ID || vehicles[0].AssignmentStatus != "ACTIVE" {
		t.Fatalf("vehicles = %+v %v", vehicles, err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_asset_assignments SET status='COMPLETED', end_at=now() WHERE asset_id=$1`, asset.ID); err != nil {
		t.Fatal(err)
	}
	if current, _ := f.service.ProviderVehicles(ctx, f.scope, m.ID, false); len(current) != 0 {
		t.Fatalf("completed allocation listed as current: %+v", current)
	}
	if history, _ := f.service.ProviderVehicles(ctx, f.scope, m.ID, true); len(history) != 1 {
		t.Fatalf("history = %+v", history)
	}

	// Service roles: category, workforce role, effective capabilities.
	future := time.Now().Add(30 * 24 * time.Hour)
	if _, err := f.service.ReplaceCapabilities(ctx, f.scope, m.ID, []providermodels.Capability{{VehicleClasses: []string{"MOTORCYCLE"}, ValidUntil: &future}}); err != nil {
		t.Fatal(err)
	}
	roles, err := f.service.ProviderServiceRoles(ctx, f.scope, m.ID)
	if err != nil || roles.ServiceCategory != "DISPATCH" || roles.WorkforceRole == nil || *roles.WorkforceRole != "RIDER" ||
		len(roles.Capabilities) != 1 || !roles.Capabilities[0].Effective {
		t.Fatalf("service roles = %+v %v", roles, err)
	}

	// Real routes: providers.read is required, cross-tenant is not found.
	owner := seedAuditRouteUser(t, f.pool, f.org, "rol_system_owner")
	finance := seedAuditRouteUser(t, f.pool, f.org, "rol_system_finance")
	ownerRouter := providerAdminRouter(t, f.pool, f.service, f.org, owner, nil)
	for _, suffix := range []string{"availability", "branches", "vehicles?includeHistory=true", "service-roles"} {
		path := "/fleet-partner/v1/providers/" + m.ID + "/" + suffix
		if code, body := ws5Call(t, ownerRouter, "GET", path, nil); code != 200 {
			t.Fatalf("GET %s = %d %v", suffix, code, body)
		}
		if code, _ := ws5Call(t, providerAdminRouter(t, f.pool, f.service, f.org, finance, nil), "GET", path, nil); code != 403 {
			t.Fatalf("GET %s without providers.read = %d", suffix, code)
		}
	}
	other := createOrganization(t, f.pool, "Provider Views Other", "RC-"+ids.New("reg")[4:14])
	otherOwner := seedAuditRouteUser(t, f.pool, other.ID, "rol_system_owner")
	if code, _ := ws5Call(t, providerAdminRouter(t, f.pool, f.service, other.ID, otherOwner, nil), "GET", "/fleet-partner/v1/providers/"+m.ID+"/branches", nil); code != 404 {
		t.Fatalf("cross-tenant branches = %d", code)
	}
}

// ---------------------------------------------------------------------------
// FLEET-17.10 performance includes marketplace responses
// ---------------------------------------------------------------------------

func TestProviderPerformanceCountsMarketplaceAndPrivateWorkDistinctly(t *testing.T) {
	f := newWS5Fixture(t, "Performance Co")
	ctx := context.Background()
	m := f.membership(t, providermodels.StatusActive)
	seedEligibleOrganization(t, f.pool, f.org)
	now := time.Now().UTC()

	marketplace := func(status string, extra string, args ...any) string {
		t.Helper()
		projection := seedProjection(t, f.pool, f.org, "req-perf-"+randomHex(), "ORGANIZATION_ACCEPTED")
		a := seedAssignment(t, f.pool, f.org, projection.ID, m, now.Add(time.Hour))
		if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_provider_assignments SET status=$2`+extra+` WHERE id=$1`, append([]any{a.ID, status}, args...)...); err != nil {
			t.Fatalf("set status %s: %v", status, err)
		}
		// A resolved assignment frees its provider hold, as production does.
		if err := mktrepos.NewAssignmentRepository(f.pool).ReleaseReservations(ctx, f.pool, a.ID); err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	declined := marketplace("DECLINED", `, declined_at=now()`)
	if _, err := f.pool.Exec(ctx, `INSERT INTO fleet_partner_assignment_responses(id,organization_id,assignment_id,membership_id,response,reason_code,safety_related,client_response_id)
		VALUES($1,$2,$3,$4,'DECLINED','LOAD_NOT_SECURED',true,$5)`, ids.New("rsp"), f.org, declined, m.ID, randomHex()); err != nil {
		t.Fatal(err)
	}
	marketplace("EXPIRED", `, expired_at=now()`)
	marketplace("CANCELLED", `, cancelled_at=now()`)
	marketplace("COMPLETED", `, accepted_at=now(), completed_at=now()`)
	marketplace("CANCELLED", `, accepted_at=now(), cancelled_at=now()`)
	if _, err := f.pool.Exec(ctx, `INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,route_stop_id,provider_membership_id,provider_id,execution_service,status,response_deadline,decline_reason,declined_at,created_by)
		VALUES($1,$2,$3,'ROUTE_STOP','stop-ws5',$4,$5,'DISPATCH','DECLINED',now()+interval '1 hour','PERSONAL_EMERGENCY',now(),'oru_ws5')`,
		ids.New("pas"), f.org, f.branch, m.ID, *m.ProviderID); err != nil {
		t.Fatal(err)
	}

	scope := tenant.Context{OrganizationID: f.org, ActorType: tenant.ActorTypeSystem, ActorID: "test", CorrelationID: "perf"}
	snap, err := f.workflow.GeneratePerformance(ctx, scope, m.ID, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if snap.FormulaVersion != workforce.PerformanceFormulaVersion {
		t.Fatalf("formula = %s", snap.FormulaVersion)
	}
	var metrics struct {
		CompanyAssignmentQuality map[string]int            `json:"companyAssignmentQuality"`
		ProviderResponse         map[string]int            `json:"providerResponse"`
		DeclineReasons           map[string]int            `json:"declineReasons"`
		Safety                   map[string]int            `json:"safety"`
		ServiceQuality           map[string]int            `json:"serviceQuality"`
		Productivity             map[string]int            `json:"productivity"`
		ByOrigin                 map[string]map[string]int `json:"byOrigin"`
	}
	if err := json.Unmarshal(snap.CalculatedMetrics, &metrics); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{
		"offered": 6, "marketplace": 5, "private": 1, "cancelledBefore": 1,
		"accepted": 2, "declined": 2, "expired": 1, "safety": 1, "completed": 1, "cancelledAfter": 1,
	}
	got := map[string]int{
		"offered": metrics.CompanyAssignmentQuality["assignmentsOffered"], "marketplace": metrics.CompanyAssignmentQuality["marketplaceAssignmentsOffered"],
		"private": metrics.CompanyAssignmentQuality["privateAssignmentsOffered"], "cancelledBefore": metrics.CompanyAssignmentQuality["cancelledByCompanyBeforeResponse"],
		"accepted": metrics.ProviderResponse["accepted"], "declined": metrics.ProviderResponse["declined"], "expired": metrics.ProviderResponse["expired"],
		"safety": metrics.Safety["safetyRelatedDeclines"], "completed": metrics.Productivity["jobsCompleted"], "cancelledAfter": metrics.ServiceQuality["cancelledAfterAcceptance"],
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %d want %d (metrics %s)", k, got[k], v, snap.CalculatedMetrics)
		}
	}
	// The marketplace decline's reason code is uncategorised ("other") but the
	// provider flagged it safety-related; the private one is personal.
	if metrics.DeclineReasons["other"] != 1 || metrics.DeclineReasons["personal"] != 1 || metrics.ByOrigin["MARKETPLACE"]["DECLINED"] != 1 {
		t.Fatalf("decline breakdown = %v %v", metrics.DeclineReasons, metrics.ByOrigin)
	}

	// Explainable: every contributing assignment is listed with its outcome.
	var inputs struct {
		Assignments []workforce.ExplainedAssignment `json:"assignments"`
		Complete    bool                            `json:"assignmentListComplete"`
	}
	_ = json.Unmarshal(snap.MetricInputs, &inputs)
	if len(inputs.Assignments) != 6 || !inputs.Complete {
		t.Fatalf("explained assignments = %d complete=%v", len(inputs.Assignments), inputs.Complete)
	}
	for _, a := range inputs.Assignments {
		if a.AssignmentID == declined && (a.Outcome != workforce.OutcomeDeclined || !a.SafetyRelated || a.DeclineCategory != "other") {
			t.Fatalf("declined marketplace assignment explained as %+v", a)
		}
	}
	var explanation map[string]any
	_ = json.Unmarshal(snap.Explanation, &explanation)
	if explanation["automaticPunishment"] != false || explanation["disciplinaryAction"] != false ||
		explanation["declinesExpiriesAndCancellationsAreDistinct"] != true || explanation["moneyFieldsRead"] != false {
		t.Fatalf("explanation = %v", explanation)
	}
	for _, forbidden := range []string{"amount", "fare", "price", "margin", "commission"} {
		if strings.Contains(strings.ToLower(string(snap.CalculatedMetrics)+string(snap.MetricInputs)), forbidden) {
			t.Fatalf("performance snapshot mentions %q", forbidden)
		}
	}

	// Replay returns the stored snapshot; no automatic consequence.
	again, err := f.workflow.GeneratePerformance(ctx, scope, m.ID, now.Add(-time.Hour), now.Add(time.Hour))
	var first, replayed map[string]any
	_ = json.Unmarshal(snap.CalculatedMetrics, &first)
	_ = json.Unmarshal(again.CalculatedMetrics, &replayed)
	if err != nil || again.ID != snap.ID || !reflect.DeepEqual(first, replayed) || again.GeneratedAt.IsZero() {
		t.Fatalf("replay = %v %v", again.ID, err)
	}
	reloaded, _ := f.repo.GetMembership(ctx, f.org, m.ID)
	if reloaded.Status != providermodels.StatusActive {
		t.Fatalf("performance changed membership status to %s", reloaded.Status)
	}
}

// ---------------------------------------------------------------------------
// FLEET-13.7 + FLEET-17.15 provider self-service (internal routes)
// ---------------------------------------------------------------------------

const ws5CallerSecret = "ws5-provider-self-service-secret-value"

func selfServiceRouter(t *testing.T, components *app.Components) http.Handler {
	t.Helper()
	registrar := components.InternalProviderSelfServiceRoutes(config.Config{
		ProviderMembershipAllowedCallers: "driver-dispatch-delivery-service,driver-hauling-service,support-dispute-service"})
	if registrar == nil {
		t.Fatal("self-service routes not mounted")
	}
	secrets := serviceauth.Secrets{}
	for _, caller := range []string{"driver-dispatch-delivery-service", "driver-hauling-service", "support-dispute-service", "driver-taxi-service"} {
		secrets[caller] = []byte(ws5CallerSecret)
	}
	return app.BuildRouter(app.Options{
		Config:         config.Config{AppEnv: "test", MaxRequestBodyBytes: 1 << 20},
		Metrics:        observability.NewMetrics(),
		TokenSigner:    auth.NewTokenSigner([]byte("unused-ws5-user-token-secret")),
		Verifier:       serviceauth.NewVerifier(secrets, time.Minute),
		InternalRoutes: func(group *gin.RouterGroup) { registrar(group) },
	})
}

func selfCall(t *testing.T, router http.Handler, caller, method, path string, body any, headers map[string]string) (int, map[string]any, string) {
	t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, "/internal/fleet-partner/v1"+path, bytes.NewReader(raw))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if caller != "" {
		if err := serviceauth.SignRequest(req, caller, []byte(ws5CallerSecret), raw, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded, recorder.Body.String()
}

func TestProviderSelfServiceInternalRoutes(t *testing.T) {
	wallet := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	providerID := "prv_self_" + randomHex()
	if _, err := wallet.pool.Exec(ctx, `UPDATE fleet_partner_organization_provider_memberships SET provider_id=$2 WHERE id=$1`, wallet.membership, providerID); err != nil {
		t.Fatal(err)
	}
	branch := seedBranch(t, wallet.pool, wallet.organization, "Self Hub "+randomHex()[:6])
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(wallet.pool))
	w := workforce.NewService(workforce.Options{Pool: wallet.pool, Events: publisher, Flags: workforce.Flags{
		Workforce: true, Shifts: true, Leave: true, Qualifications: true, Training: true, Performance: true}})
	scope := tenant.Context{OrganizationID: wallet.organization, OrganizationUserID: "oru_planner", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "oru_planner", CorrelationID: "self"}

	// Seed the provider's own records through the real workforce usecases.
	now := time.Now().UTC()
	shift, err := w.CreateShift(ctx, scope, workforce.CreateShiftInput{BranchID: branch, Name: "Morning", Role: "RIDER", Timezone: "Africa/Lagos",
		StartAt: now.Add(24 * time.Hour), EndAt: now.Add(30 * time.Hour), RequiredCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.UpdateShift(ctx, scope, shift.ID, shift.Version, "", nil, nil, "PUBLISHED"); err != nil {
		t.Fatal(err)
	}
	if _, err = w.AssignShift(ctx, scope, shift.ID, workforce.AssignShiftInput{MembershipID: wallet.membership, AssignmentType: "PRIMARY"}); err != nil {
		t.Fatal(err)
	}
	draft, err := w.CreateShift(ctx, scope, workforce.CreateShiftInput{BranchID: branch, Name: "Unpublished", Role: "RIDER", Timezone: "Africa/Lagos",
		StartAt: now.Add(48 * time.Hour), EndAt: now.Add(50 * time.Hour), RequiredCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.AssignShift(ctx, scope, draft.ID, workforce.AssignShiftInput{MembershipID: wallet.membership, AssignmentType: "PRIMARY"}); err != nil {
		t.Fatal(err)
	}
	q, err := w.CreateQualification(ctx, scope, workforce.Qualification{Code: "SELF-" + randomHex()[:4], Name: "Safety induction", ApplicableServices: []string{"DISPATCH"}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := w.CreateCourse(ctx, scope, workforce.Course{Code: "C-" + randomHex()[:4], Title: "Defensive riding", PassingScore: 70})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Enroll(ctx, scope, workforce.EnrollInput{CourseID: course.ID, MembershipID: wallet.membership}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.GeneratePerformance(ctx, tenant.Context{OrganizationID: wallet.organization, ActorType: tenant.ActorTypeSystem, ActorID: "test"},
		wallet.membership, now.Add(-24*time.Hour), now); err != nil {
		t.Fatal(err)
	}

	providerSvc := providerusecases.NewService(providerusecases.Options{Pool: wallet.pool, Providers: providerrepos.NewRepository(wallet.pool), Organizations: activeOrgReader{}})
	router := selfServiceRouter(t, &app.Components{Providers: providerSvc, CompanyWallet: wallet.service, Workforce: w})
	base := "/providers/" + providerID
	dispatch := "driver-dispatch-delivery-service"

	// FLEET-13.7: own sub-wallet, statement and withdrawal request.
	code, body, raw := selfCall(t, router, dispatch, "GET", base+"/sub-wallet", nil, nil)
	if code != 200 || body["data"].(map[string]any)["membershipId"] != wallet.membership {
		t.Fatalf("sub-wallet = %d %s", code, raw)
	}
	if strings.Contains(raw, "fwal_org_1") {
		t.Fatal("the organization wallet account leaked into a provider view")
	}
	if code, _, raw = selfCall(t, router, dispatch, "GET", base+"/sub-wallet/statement", nil, nil); code != 200 {
		t.Fatalf("statement = %d %s", code, raw)
	}
	code, body, raw = selfCall(t, router, dispatch, "POST", base+"/sub-wallet/withdrawals",
		map[string]any{"amountMinor": 50000, "currency": "NGN", "destinationBankAccountReference": "bank_ref_opaque"},
		map[string]string{"Idempotency-Key": "self-wd-" + randomHex()})
	if code != 201 || body["data"].(map[string]any)["requestedByActorType"] != "PROVIDER" || body["data"].(map[string]any)["membershipId"] != wallet.membership {
		t.Fatalf("provider withdrawal = %d %s", code, raw)
	}

	// FLEET-17.15: own schedule, qualifications, training, performance.
	code, body, raw = selfCall(t, router, dispatch, "GET", base+"/schedule", nil, nil)
	shifts := body["data"].(map[string]any)["shifts"].([]any)
	if code != 200 || len(shifts) != 1 || shifts[0].(map[string]any)["shiftId"] != shift.ID {
		t.Fatalf("schedule (draft shifts excluded) = %d %s", code, raw)
	}
	if strings.Contains(raw, "plannerUserId") || strings.Contains(raw, "oru_planner") {
		t.Fatal("schedule exposes staff identifiers")
	}
	if code, _, _ = selfCall(t, router, dispatch, "GET", base+"/schedule?from=not-a-time", nil, nil); code != 422 {
		t.Fatalf("invalid schedule window = %d", code)
	}
	code, body, raw = selfCall(t, router, dispatch, "GET", base+"/qualifications", nil, nil)
	quals := body["data"].([]any)
	if code != 200 || len(quals) != 1 || quals[0].(map[string]any)["qualificationId"] != q.ID || quals[0].(map[string]any)["status"] != "MISSING" {
		t.Fatalf("qualifications = %d %s", code, raw)
	}
	code, body, raw = selfCall(t, router, dispatch, "GET", base+"/training", nil, nil)
	if code != 200 || len(body["data"].(map[string]any)["enrollments"].([]any)) != 1 || strings.Contains(raw, "assignedBy") {
		t.Fatalf("training = %d %s", code, raw)
	}
	code, body, raw = selfCall(t, router, dispatch, "GET", base+"/performance", nil, nil)
	snaps := body["data"].([]any)
	if code != 200 || len(snaps) != 1 || snaps[0].(map[string]any)["formulaVersion"] != workforce.PerformanceFormulaVersion {
		t.Fatalf("performance = %d %s", code, raw)
	}

	// Authorization boundary.
	if code, _, _ = selfCall(t, router, "driver-hauling-service", "GET", base+"/sub-wallet", nil, nil); code != 404 {
		t.Fatalf("another owning service reached a dispatch provider: %d", code)
	}
	if code, _, _ = selfCall(t, router, "support-dispute-service", "GET", base+"/schedule", nil, nil); code != 403 {
		t.Fatalf("allowlisted non-owning service = %d", code)
	}
	if code, _, _ = selfCall(t, router, "driver-taxi-service", "GET", base+"/schedule", nil, nil); code != 403 {
		t.Fatalf("non-allowlisted caller = %d", code)
	}
	if code, _, _ = selfCall(t, router, "", "GET", base+"/schedule", nil, nil); code != 401 {
		t.Fatalf("unsigned call = %d", code)
	}
	if code, _, _ = selfCall(t, router, dispatch, "GET", "/providers/prv_unknown_"+randomHex()+"/training", nil, nil); code != 404 {
		t.Fatalf("unknown provider = %d", code)
	}
	// An ended membership is no longer reachable.
	if _, err := wallet.pool.Exec(ctx, `UPDATE fleet_partner_organization_provider_memberships SET status='REMOVED' WHERE id=$1`, wallet.membership); err != nil {
		t.Fatal(err)
	}
	if code, _, _ = selfCall(t, router, dispatch, "GET", base+"/sub-wallet", nil, nil); code != 404 {
		t.Fatalf("removed provider sub-wallet = %d", code)
	}
}
