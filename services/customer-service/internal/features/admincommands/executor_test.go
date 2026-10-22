package admincommands

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/customer-service/internal/features/adminresources"
	"cosmicforge/logistics/services/customer-service/internal/features/adminreview"
	authmodels "cosmicforge/logistics/services/customer-service/internal/features/auth/models"
	authrepositories "cosmicforge/logistics/services/customer-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	contexthttp "cosmicforge/logistics/services/customer-service/internal/features/customer_context/http"
	contextusecases "cosmicforge/logistics/services/customer-service/internal/features/customer_context/usecases"
	mobilityrepositories "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/repositories"
	profilerepositories "cosmicforge/logistics/services/customer-service/internal/features/profile/repositories"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// memChallenges is an in-memory OTP challenge store so the real AuthService
// usecase runs end to end without Redis.
type memChallenges struct {
	mu    sync.Mutex
	items map[string]authmodels.OTPChallenge
}

func (m *memChallenges) Save(_ context.Context, c authmodels.OTPChallenge, _, _ time.Duration, _ int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[c.IdentifierKey()] = c
	return nil
}
func (m *memChallenges) Get(_ context.Context, id authmodels.AuthIdentifier) (authmodels.OTPChallenge, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.items[id.Key()]
	return c, ok, nil
}
func (m *memChallenges) RecordFailedAttempt(_ context.Context, c authmodels.OTPChallenge, _ time.Duration) error {
	return nil
}
func (m *memChallenges) Delete(_ context.Context, id authmodels.AuthIdentifier) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, id.Key())
	return nil
}

type harness struct {
	t        *testing.T
	db       *pgxpool.Pool
	svc      *admincommand.Service
	actor    adminpolicy.Actor
	scope    adminpolicy.Scope
	issuer   *contextfixture.Fixture
	store    *admincontext.Store
	auth     *authusecases.AuthService
	sessions *authrepositories.PostgresRefreshSessionRepository
	profiles *profilerepositories.PostgresCustomerRepository
	customer string
	email    string
}

const purpose = "case-1"

func setup(t *testing.T) *harness {
	t.Helper()
	db := fixture.Database(t, "../../../migrations")
	spec := Spec()
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	policy := policyfixture.New(t, scope, Owner, spec.Capability, spec.Contract)
	read := policy.Document.Rules[0]
	read.Capability, read.Permission = spec.ReadCapability, spec.ReadCapability
	policy.Document.Rules = append(policy.Document.Rules, read)
	policy.Sign(t)
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: adminresources.Source(), Authority: issuer.Authority}
	h := &harness{t: t, db: db, scope: scope, issuer: issuer, store: store,
		svc:   &admincommand.Service{Spec: spec, DB: db, Policy: policy.Engine, Contexts: store, Executor: Executor{}, WriteEnabled: true, ReadEnabled: true},
		actor: adminpolicy.Actor{Product: "karryam", Staff: "staff-a", Roles: []string{"staff"}, Permissions: []string{spec.Capability, spec.ReadCapability}, Markets: []string{"M1"}, Zones: []string{"Z1"}, Organizations: []string{"org-1"}, Tenants: []string{"tenant-1"}},
	}
	h.customer, h.email = h.newCustomer(scope)
	h.profiles = profilerepositories.NewPostgresCustomerRepository(db)
	h.sessions = authrepositories.NewPostgresRefreshSessionRepository(db)
	h.auth = authusecases.NewAuthService(authusecases.Options{
		Customers: h.profiles, Sessions: h.sessions, Challenges: &memChallenges{items: map[string]authmodels.OTPChallenge{}},
		AccessTokenSecret: []byte(strings.Repeat("a", 32)), RefreshTokenSecret: []byte(strings.Repeat("r", 32)), OTPSecret: []byte(strings.Repeat("o", 32)),
		AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour, OTPTTL: time.Minute, OTPRateWindow: time.Minute, OTPMaxRequests: 100, OTPMaxAttempts: 5, OTPDebug: true,
		Standing: StandingReader{DB: db},
	})
	return h
}

// newCustomer creates a real customer and imports its signed owner context
// for the given scope through the actual customers resource binding.
func (h *harness) newCustomer(scope adminpolicy.Scope) (string, string) {
	h.t.Helper()
	ctx := context.Background()
	id := uuid.NewString()
	email := "cmd-" + id[:8] + "@example.invalid"
	if _, err := h.db.Exec(ctx, `INSERT INTO customers(id,email) VALUES($1,$2)`, id, email); err != nil {
		h.t.Fatal(err)
	}
	tx, err := h.db.Begin(ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	binding, err := h.store.Reader.Binding(ctx, tx, SubjectKind, id)
	tx.Rollback(ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	spec := Spec()
	approval := h.issuer.Approval("ctx-"+id, SubjectKind, id, spec.Capability, binding, scope)
	approval.Capabilities = []string{spec.Capability, spec.ReadCapability}
	if err := h.store.Import(ctx, h.issuer.Sign(h.t, approval), time.Now()); err != nil {
		h.t.Fatal(err)
	}
	return id, email
}

func (h *harness) cmd(action string, expected int64, params map[string]string, caseRef string) admincommand.Command {
	if params == nil {
		params = map[string]string{}
	}
	return admincommand.Command{ContractVersion: Spec().Contract, OperationID: uuid.NewString(), SubjectKind: SubjectKind, SubjectID: h.customer, CaseRef: caseRef, Action: action, ExpectedVersion: expected, IdempotencyKey: uuid.NewString(), ReasonCode: "OPERATIONS_REVIEW", Params: params, RequestedAt: time.Now().UTC().Add(-time.Second)}
}
func (h *harness) approve(c *admincommand.Command, approver string) {
	c.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: approver, IntentDigest: c.IntentDigest(h.actor.Staff, purpose, h.scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
}
func (h *harness) exec(c admincommand.Command) (admincommand.Receipt, error) {
	return h.svc.Execute(context.Background(), h.actor, purpose, h.scope, c)
}
func (h *harness) mustExec(c admincommand.Command) admincommand.Receipt {
	h.t.Helper()
	r, err := h.exec(c)
	if err != nil {
		h.t.Fatalf("%s v%d: %v", c.Action, c.ExpectedVersion, err)
	}
	return r
}
func (h *harness) current() (int64, string) {
	h.t.Helper()
	tx, err := h.db.Begin(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	v, s, err := Executor{}.Current(context.Background(), tx, SubjectKind, h.customer)
	if err != nil {
		h.t.Fatal(err)
	}
	return v, s
}
func (h *harness) login() (authusecases.TokenResult, error) {
	ctx := context.Background()
	start, err := h.auth.StartAuth(ctx, authusecases.StartAuthInput{Email: h.email})
	if err != nil {
		h.t.Fatal(err)
	}
	return h.auth.VerifyAuth(ctx, authusecases.VerifyAuthInput{Email: h.email, OTP: start.DebugOTP, ChallengeID: start.ChallengeID})
}
func (h *harness) activeSessions() int {
	var n int
	h.db.QueryRow(context.Background(), `SELECT count(*) FROM customer_sessions WHERE customer_id=$1 AND revoked_at IS NULL`, h.customer).Scan(&n)
	return n
}
func restricted(err error) bool {
	app := apperrors.From(err)
	return app != nil && app.Code == "account_restricted" && app.Status == http.StatusForbidden
}

func TestPostgresLoginBlockRevokesSessionsAndRefusesOTPAndRefresh(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	first, err := h.login()
	if err != nil {
		t.Fatal(err)
	}
	if v, s := h.current(); v != 0 || s != "ACTIVE" {
		t.Fatal("fresh customer standing", v, s)
	}
	restrict := h.cmd("RESTRICT_ACCOUNT", 0, map[string]string{"restriction": "LOGIN_BLOCKED"}, "")
	if _, err := h.exec(restrict); !errors.Is(err, admincommand.ErrInvalid) {
		t.Fatal("approval-required restriction accepted without approval", err)
	}
	h.approve(&restrict, h.actor.Staff)
	if _, err := h.exec(restrict); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatal("self-approved restriction accepted", err)
	}
	h.approve(&restrict, "staff-b")
	receipt := h.mustExec(restrict)
	if receipt.PriorState != "ACTIVE" || receipt.ResultingState != "LOGIN_BLOCKED" || receipt.Outcome != "LOGIN_BLOCKED_SESSIONS_REVOKED" || receipt.RecordedVersion != 1 {
		t.Fatal("restriction receipt", receipt)
	}
	if h.activeSessions() != 0 {
		t.Fatal("login block left an active refresh session")
	}
	if _, err := h.auth.Refresh(ctx, authusecases.RefreshInput{RefreshToken: first.RefreshToken}); err == nil {
		t.Fatal("revoked refresh session rotated")
	}
	if _, err := h.login(); !restricted(err) {
		t.Fatal("login-blocked customer completed OTP login", err)
	}
	// Defence in depth: even a direct session insert is refused by the guard.
	if err := h.sessions.Create(ctx, authmodels.RefreshSession{ID: uuid.NewString(), CustomerID: h.customer, RefreshTokenHash: "x", ExpiresAt: time.Now().Add(time.Hour)}); !restricted(err) {
		t.Fatal("database guard admitted a session for a login-blocked customer", err)
	}
	if h.activeSessions() != 0 {
		t.Fatal("blocked customer gained a session")
	}
	// Illegal transitions are rejected with no side effects.
	again := h.cmd("RESTRICT_ACCOUNT", 1, map[string]string{"restriction": "BOOKING_HOLD"}, "")
	h.approve(&again, "staff-b")
	if _, err := h.exec(again); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("restriction stacked on an existing restriction", err)
	}
	stale := h.cmd("LIFT_RESTRICTION", 0, nil, "")
	h.approve(&stale, "staff-b")
	if _, err := h.exec(stale); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("stale lift accepted", err)
	}
	if v, s := h.current(); v != 1 || s != "LOGIN_BLOCKED" {
		t.Fatal("refused commands changed standing", v, s)
	}
	lift := h.cmd("LIFT_RESTRICTION", 1, nil, "")
	h.approve(&lift, "staff-b")
	lifted := h.mustExec(lift)
	if lifted.ResultingState != "ACTIVE" || lifted.Outcome != "RESTRICTION_LIFTED" {
		t.Fatal(lifted)
	}
	replay, err := h.exec(lift)
	if err != nil || replay != lifted {
		t.Fatal("replay did not return the original receipt", err)
	}
	unrestricted := h.cmd("LIFT_RESTRICTION", 2, nil, "")
	h.approve(&unrestricted, "staff-b")
	if _, err := h.exec(unrestricted); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("lifting an unrestricted account accepted", err)
	}
	second, err := h.login()
	if err != nil {
		t.Fatal("lifted customer cannot log in", err)
	}
	if _, err := h.auth.Refresh(ctx, authusecases.RefreshInput{RefreshToken: second.RefreshToken}); err != nil {
		t.Fatal("lifted customer cannot refresh", err)
	}
	history, err := h.svc.History(ctx, h.actor, purpose, h.scope, SubjectKind, h.customer)
	if err != nil || history.CurrentVersion != 2 || history.CurrentState != "ACTIVE" || len(history.Decisions) != 2 || history.Decisions[0].DecisionID != lifted.DecisionID {
		t.Fatal("history", err, history)
	}
	back, err := h.svc.Readback(ctx, h.actor, purpose, h.scope, SubjectKind, h.customer, restrict.IdempotencyKey)
	if err != nil || back != receipt {
		t.Fatal("readback", err)
	}
	var rows int
	h.db.QueryRow(ctx, `SELECT count(*) FROM customer_account_standing_history WHERE customer_id=$1`, h.customer).Scan(&rows)
	if rows != 2 {
		t.Fatal("standing decision history", rows)
	}
	for _, sql := range []string{`UPDATE customer_account_standing_history SET action='X'`, `DELETE FROM customer_account_standing_history`} {
		if _, err := h.db.Exec(ctx, sql); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatal("standing history mutable", sql, err)
		}
	}
}

func TestPostgresBookingHoldIsAuthoritativeOnInternalContextRoute(t *testing.T) {
	h := setup(t)
	gin.SetMode(gin.TestMode)
	scopes, err := contextusecases.ParseCallerScopes("driver-taxi-service=mobility_preferences")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	mobility := mobilityrepositories.NewPostgresRepository(h.db)
	contexthttp.RegisterInternalRoutes(router.Group("/api/v1/customer"), contextusecases.NewService(scopes, mobility, nil).WithStanding(StandingReader{DB: h.db}), serviceauth.NewVerifier(serviceauth.Secrets{"driver-taxi-service": []byte("taxi-secret")}, 5*time.Minute))
	read := func() map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/customer/internal/customers/"+h.customer+"/context?scopes=mobility_preferences", nil)
		if err := serviceauth.SignRequest(req, "driver-taxi-service", []byte("taxi-secret"), nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		var body struct {
			Data struct {
				AccountStanding map[string]any `json:"account_standing"`
				Mobility        map[string]any `json:"mobility_preferences"`
				ContextVersion  string         `json:"context_version"`
			} `json:"data"`
		}
		json.Unmarshal(rec.Body.Bytes(), &body)
		if body.Data.Mobility == nil || body.Data.ContextVersion == "" {
			t.Fatal("existing context contract broken", rec.Body.String())
		}
		return body.Data.AccountStanding
	}
	if s := read(); s["booking_eligible"] != true || s["restriction"] != "NONE" || s["standing_version"] != float64(0) {
		t.Fatal("initial standing", s)
	}
	hold := h.cmd("RESTRICT_ACCOUNT", 0, map[string]string{"restriction": "BOOKING_HOLD"}, "")
	h.approve(&hold, "staff-b")
	if r := h.mustExec(hold); r.Outcome != "BOOKING_HOLD_APPLIED" {
		t.Fatal(r)
	}
	if s := read(); s["booking_eligible"] != false || s["restriction"] != "BOOKING_HOLD" || s["standing_version"] != float64(1) {
		t.Fatal("booking hold not published", s)
	}
	// A booking hold is not a login block.
	if _, err := h.login(); err != nil {
		t.Fatal("booking hold blocked login", err)
	}
	lift := h.cmd("LIFT_RESTRICTION", 1, nil, "")
	h.approve(&lift, "staff-b")
	h.mustExec(lift)
	if s := read(); s["booking_eligible"] != true || s["restriction"] != "NONE" || s["standing_version"] != float64(2) {
		t.Fatal("unrestriction did not restore eligibility", s)
	}
}

func TestPostgresRemediationRevokesSessionsAndRequiresRealCompletion(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := h.login(); err != nil {
			t.Fatal(err)
		}
	}
	if h.activeSessions() != 2 {
		t.Fatal("setup sessions")
	}
	reset := h.mustExec(h.cmd("REQUIRE_REMEDIATION", 0, map[string]string{"remediation": "RESET_SESSIONS"}, ""))
	if reset.ResultingState != "ACTIVE_REMEDIATION_RESET_SESSIONS" || reset.Outcome != "REMEDIATION_REQUIRED_SESSIONS_REVOKED" || h.activeSessions() != 0 {
		t.Fatal("reset sessions", reset, h.activeSessions())
	}
	if _, err := h.exec(h.cmd("REQUIRE_REMEDIATION", 1, map[string]string{"remediation": "REVERIFY_CONTACT"}, "")); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("second remediation stacked", err)
	}
	if r := h.mustExec(h.cmd("COMPLETE_REMEDIATION", 1, map[string]string{"evidence_ref": "ticket-1"}, "")); r.ResultingState != "ACTIVE" {
		t.Fatal(r)
	}
	if _, err := h.exec(h.cmd("COMPLETE_REMEDIATION", 2, nil, "")); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("completed a remediation that was not required", err)
	}
	// Re-verification is complete only after a real OTP login.
	if _, err := h.login(); err != nil {
		t.Fatal(err)
	}
	h.mustExec(h.cmd("REQUIRE_REMEDIATION", 2, map[string]string{"remediation": "REVERIFY_CONTACT"}, ""))
	if h.activeSessions() != 0 {
		t.Fatal("reverify did not revoke sessions")
	}
	if _, err := h.exec(h.cmd("COMPLETE_REMEDIATION", 3, nil, "")); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("reverification completed without a new verified login", err)
	}
	if _, err := h.login(); err != nil {
		t.Fatal(err)
	}
	h.mustExec(h.cmd("COMPLETE_REMEDIATION", 3, nil, ""))
	// Profile correction reopens the existing onboarding gate.
	if _, err := h.profiles.UpdateProfile(ctx, h.customer, "Ada", "Okafor"); err != nil {
		t.Fatal(err)
	}
	h.mustExec(h.cmd("REQUIRE_REMEDIATION", 4, map[string]string{"remediation": "PROFILE_CORRECTION"}, ""))
	customer, err := h.profiles.GetByID(ctx, h.customer)
	if err != nil || customer.OnboardingStatus != "profile_required" {
		t.Fatal("profile correction did not reopen onboarding", customer.OnboardingStatus, err)
	}
	if _, err := h.exec(h.cmd("COMPLETE_REMEDIATION", 5, nil, "")); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("profile correction completed before the customer corrected the profile", err)
	}
	if _, err := h.profiles.UpdateProfile(ctx, h.customer, "Ada", "Okafor-Bello"); err != nil {
		t.Fatal(err)
	}
	if r := h.mustExec(h.cmd("COMPLETE_REMEDIATION", 5, nil, "")); r.RecordedVersion != 6 || r.ResultingState != "ACTIVE" {
		t.Fatal(r)
	}
}

func TestPostgresPrivacyRequestLifecycleAndConsentWithdrawal(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	if _, err := h.db.Exec(ctx, `INSERT INTO emergency_contacts(customer_id,name,phone,relationship,trip_sharing_enabled,emergency_alerts_enabled) VALUES($1,'PRIVATE','+2348000000000','sibling',true,true)`, h.customer); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(ctx, `INSERT INTO customer_mobility_preferences(customer_id,safety_share_default) VALUES($1,true)`, h.customer); err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format("2006-01-02")
	caseRef := uuid.NewString()
	open := h.cmd("OPEN_PRIVACY_REQUEST", 0, map[string]string{"request_type": "CONSENT_WITHDRAWAL", "channel": "IN_APP", "received_on": today}, caseRef)
	if r := h.mustExec(open); r.PriorState != "NONE" || r.ResultingState != "RECEIVED" || r.RecordedVersion != 1 {
		t.Fatal(r)
	}
	if _, err := h.exec(h.cmd("OPEN_PRIVACY_REQUEST", 0, open.Params, caseRef)); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("privacy case opened twice", err)
	}
	if _, err := h.exec(h.cmd("OPEN_PRIVACY_REQUEST", 0, open.Params, "not-a-uuid")); !errors.Is(err, admincommand.ErrInvalid) {
		t.Fatal("non-uuid privacy case accepted", err)
	}
	future := h.cmd("OPEN_PRIVACY_REQUEST", 0, map[string]string{"request_type": "ERASURE", "channel": "EMAIL", "received_on": time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")}, uuid.NewString())
	if _, err := h.exec(future); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("future receipt date accepted", err)
	}
	early := h.cmd("FULFIL_PRIVACY_REQUEST", 1, map[string]string{"evidence_ref": "export-1"}, caseRef)
	h.approve(&early, "staff-b")
	if _, err := h.exec(early); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("fulfilled before acceptance", err)
	}
	if _, err := h.exec(h.cmd("ACCEPT_PRIVACY_REQUEST", 0, map[string]string{"due_on": today}, caseRef)); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("stale accept", err)
	}
	if _, err := h.exec(h.cmd("ACCEPT_PRIVACY_REQUEST", 1, map[string]string{"due_on": "2000-01-01"}, caseRef)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("due date before receipt accepted", err)
	}
	h.mustExec(h.cmd("ACCEPT_PRIVACY_REQUEST", 1, map[string]string{"due_on": time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")}, caseRef))
	fulfil := h.cmd("FULFIL_PRIVACY_REQUEST", 2, map[string]string{"evidence_ref": "consent-log-7"}, caseRef)
	if _, err := h.exec(fulfil); !errors.Is(err, admincommand.ErrInvalid) {
		t.Fatal("fulfilment without approval", err)
	}
	h.approve(&fulfil, "staff-b")
	done := h.mustExec(fulfil)
	if done.ResultingState != "FULFILLED" || done.Outcome != "CONSENTS_WITHDRAWN" {
		t.Fatal(done)
	}
	var trip, alerts, share bool
	var contactVersion int64
	h.db.QueryRow(ctx, `SELECT trip_sharing_enabled,emergency_alerts_enabled,version FROM emergency_contacts WHERE customer_id=$1`, h.customer).Scan(&trip, &alerts, &contactVersion)
	h.db.QueryRow(ctx, `SELECT safety_share_default FROM customer_mobility_preferences WHERE customer_id=$1`, h.customer).Scan(&share)
	if trip || alerts || share || contactVersion != 2 {
		t.Fatal("consents not withdrawn", trip, alerts, share, contactVersion)
	}
	var scope, evidence string
	var withdrawn int64
	h.db.QueryRow(ctx, `SELECT fulfilment_scope,evidence_ref,consents_withdrawn FROM customer_privacy_requests WHERE case_ref=$1`, caseRef).Scan(&scope, &evidence, &withdrawn)
	if scope != FulfilmentScope || evidence != "consent-log-7" || withdrawn != 2 {
		t.Fatal("fulfilment record", scope, evidence, withdrawn)
	}
	late := h.cmd("REJECT_PRIVACY_REQUEST", 3, map[string]string{"rejection": "DUPLICATE"}, caseRef)
	h.approve(&late, "staff-b")
	if _, err := h.exec(late); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("rejected a fulfilled request", err)
	}
	// Erasure is recorded for customer-service records only; nothing is deleted.
	erasure := uuid.NewString()
	h.mustExec(h.cmd("OPEN_PRIVACY_REQUEST", 0, map[string]string{"request_type": "ERASURE", "channel": "SUPPORT", "received_on": today}, erasure))
	h.mustExec(h.cmd("ACCEPT_PRIVACY_REQUEST", 1, map[string]string{"due_on": today}, erasure))
	erase := h.cmd("FULFIL_PRIVACY_REQUEST", 2, map[string]string{"evidence_ref": "erasure-ticket-9"}, erasure)
	h.approve(&erase, "staff-b")
	if r := h.mustExec(erase); r.Outcome != "DISPOSITION_RECORDED_CUSTOMER_SERVICE_ONLY" {
		t.Fatal(r)
	}
	var stillThere bool
	h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customers WHERE id=$1)`, h.customer).Scan(&stillThere)
	if !stillThere {
		t.Fatal("erasure disposition deleted data")
	}
	// REJECTED is reachable straight from RECEIVED.
	identity := uuid.NewString()
	h.mustExec(h.cmd("OPEN_PRIVACY_REQUEST", 0, map[string]string{"request_type": "ACCESS_EXPORT", "channel": "EMAIL", "received_on": today}, identity))
	reject := h.cmd("REJECT_PRIVACY_REQUEST", 1, map[string]string{"rejection": "IDENTITY_UNVERIFIED"}, identity)
	h.approve(&reject, "staff-b")
	if r := h.mustExec(reject); r.PriorState != "RECEIVED" || r.ResultingState != "REJECTED" {
		t.Fatal(r)
	}
	// A case belonging to another customer is not addressable through this one.
	other, _ := h.newCustomer(h.scope)
	foreign := h.cmd("ACCEPT_PRIVACY_REQUEST", 1, map[string]string{"due_on": today}, identity)
	foreign.SubjectID = other
	if _, err := h.exec(foreign); !errors.Is(err, admincommand.ErrNotFound) {
		t.Fatal("foreign case addressed", err)
	}
	var transitions int
	h.db.QueryRow(ctx, `SELECT count(*) FROM customer_privacy_request_transitions WHERE customer_id=$1`, h.customer).Scan(&transitions)
	if transitions != 8 {
		t.Fatal("privacy transition history", transitions)
	}
	if _, err := h.db.Exec(ctx, `DELETE FROM customer_privacy_request_transitions`); err == nil {
		t.Fatal("privacy history deletable")
	}
	if v, s := h.current(); v != 0 || s != "ACTIVE" {
		t.Fatal("privacy cases changed account standing", v, s)
	}
}

func TestPostgresSupportDecisionsAreVersionedCaseRecords(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	ref := "support-case-" + uuid.NewString()[:8]
	first := h.mustExec(h.cmd("RECORD_SUPPORT_DECISION", 0, map[string]string{"decision": "GOODWILL_ACKNOWLEDGED", "note": "Late driver"}, ref))
	if first.PriorState != "NONE" || first.ResultingState != "GOODWILL_ACKNOWLEDGED" || first.RecordedVersion != 1 {
		t.Fatal(first)
	}
	if _, err := h.exec(h.cmd("RECORD_SUPPORT_DECISION", 1, map[string]string{"decision": "GOODWILL_ACKNOWLEDGED"}, ref)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("identical decision re-recorded", err)
	}
	if _, err := h.exec(h.cmd("RECORD_SUPPORT_DECISION", 0, map[string]string{"decision": "NO_ACCOUNT_ACTION"}, ref)); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("stale support decision", err)
	}
	escalated := h.mustExec(h.cmd("RECORD_SUPPORT_DECISION", 1, map[string]string{"decision": "ESCALATED_TO_RESTRICTION_REVIEW"}, ref))
	if escalated.Outcome != "RESTRICTION_REVIEW_ESCALATION_RECORDED" || escalated.PriorState != "GOODWILL_ACKNOWLEDGED" {
		t.Fatal(escalated)
	}
	var reviewStatus string
	h.db.QueryRow(ctx, `SELECT status FROM customer_admin_review_state WHERE customer_id=$1`, h.customer).Scan(&reviewStatus)
	if reviewStatus != "IDLE" {
		t.Fatal("support decision bypassed the review intake", reviewStatus)
	}
	privacy := uuid.NewString()
	h.mustExec(h.cmd("OPEN_PRIVACY_REQUEST", 0, map[string]string{"request_type": "ACCESS_EXPORT", "channel": "EMAIL", "received_on": time.Now().UTC().Format("2006-01-02")}, privacy))
	if _, err := h.exec(h.cmd("RECORD_SUPPORT_DECISION", 0, map[string]string{"decision": "NO_ACCOUNT_ACTION"}, privacy)); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("support decision reused a privacy case reference", err)
	}
	var rows int
	h.db.QueryRow(ctx, `SELECT count(*) FROM customer_support_decisions WHERE case_ref=$1`, ref).Scan(&rows)
	if rows != 2 {
		t.Fatal("support decision history", rows)
	}
	if _, err := h.db.Exec(ctx, `UPDATE customer_support_decisions SET decision='NO_ACCOUNT_ACTION'`); err == nil {
		t.Fatal("support history mutable")
	}
}

func TestPostgresConcurrentRestrictionsCommitExactlyOnce(t *testing.T) {
	h := setup(t)
	if _, err := h.login(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			restriction := "LOGIN_BLOCKED"
			if i%2 == 1 {
				restriction = "BOOKING_HOLD"
			}
			c := h.cmd("RESTRICT_ACCOUNT", 0, map[string]string{"restriction": restriction}, "")
			h.approve(&c, "staff-b")
			_, err := h.exec(c)
			results <- err
		}(i)
	}
	// Concurrent logins race the restriction; none may survive a login block.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.login()
		}()
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		if err == nil {
			ok++
		} else if !errors.Is(err, admincommand.ErrConflict) && !errors.Is(err, admincommand.ErrRejected) {
			t.Fatal("unexpected concurrent error", err)
		}
	}
	v, s := h.current()
	if ok != 1 || v != 1 {
		t.Fatal("concurrent restrictions", ok, v, s)
	}
	if s == "LOGIN_BLOCKED" && h.activeSessions() != 0 {
		t.Fatal("session survived a concurrent login block")
	}
}

func TestPostgresCrossTenantSubjectIsDeniedWithoutEffect(t *testing.T) {
	h := setup(t)
	other := h.scope
	other.Tenant = "tenant-2"
	actor := h.actor
	actor.Tenants = []string{"tenant-2"}
	c := h.cmd("REQUIRE_REMEDIATION", 0, map[string]string{"remediation": "RESET_SESSIONS"}, "")
	if _, err := h.svc.Execute(context.Background(), actor, purpose, other, c); err == nil {
		t.Fatal("foreign-scope command executed")
	}
	// A subject never enrolled in this scope cannot be addressed either.
	stranger := uuid.NewString()
	if _, err := h.db.Exec(context.Background(), `INSERT INTO customers(id,email) VALUES($1,$2)`, stranger, stranger[:8]+"@example.invalid"); err != nil {
		t.Fatal(err)
	}
	c = h.cmd("REQUIRE_REMEDIATION", 0, map[string]string{"remediation": "RESET_SESSIONS"}, "")
	c.SubjectID = stranger
	if _, err := h.exec(c); err == nil {
		t.Fatal("unenrolled customer addressed")
	}
	if v, s := h.current(); v != 0 || s != "ACTIVE" {
		t.Fatal("denied command leaked an effect", v, s)
	}
	var ledger int
	h.db.QueryRow(context.Background(), `SELECT count(*) FROM admin_owner_command_decisions`).Scan(&ledger)
	if ledger != 0 {
		t.Fatal("denied command reached the ledger")
	}
}

func TestPostgresSignedCommandBoundaryOnRealRouteRegistration(t *testing.T) {
	h := setup(t)
	gin.SetMode(gin.TestMode)
	write, read := []byte(strings.Repeat("w", 32)), []byte(strings.Repeat("r", 32))
	engine := gin.New()
	// Same root registrations as cmd/main.go: owner resources, review, commands.
	(&adminowner.Handler{Owner: Owner, Path: "/internal/customer/v1/admin"}).Register(engine)
	(&adminreview.Handler{Service: &adminreview.Service{}}).Register(engine)
	(&admincommand.Handler{Service: h.svc, WriteSecret: write, ReadSecret: read}).Register(engine)
	server := httptest.NewServer(engine)
	defer server.Close()
	client, err := admincommand.NewClient(Owner, server.URL, write, read)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	q := adminread.Query{Scope: adminread.Scope{Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}, Staff: h.actor.Staff, Purpose: purpose, Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1, Role: "staff", PolicyID: h.svc.Policy.ID(), PolicyVersion: h.svc.Policy.Version(), PolicyChecksum: h.svc.Policy.Checksum()}
	if _, err := h.login(); err != nil {
		t.Fatal(err)
	}
	c := h.cmd("RESTRICT_ACCOUNT", 0, map[string]string{"restriction": "LOGIN_BLOCKED"}, "")
	h.approve(&c, "staff-b")
	receipt, err := client.Execute(context.Background(), q, c)
	if err != nil || receipt.ResultingState != "LOGIN_BLOCKED" || h.activeSessions() != 0 {
		t.Fatal("signed restriction", err, receipt)
	}
	if _, err := h.login(); !restricted(err) {
		t.Fatal("signed restriction did not block login", err)
	}
	back, err := client.Readback(context.Background(), q, SubjectKind, h.customer, c.IdempotencyKey)
	if err != nil || back != receipt {
		t.Fatal("signed readback", err)
	}
	history, err := client.History(context.Background(), q, SubjectKind, h.customer)
	if err != nil || history.CurrentVersion != 1 || history.CurrentState != "LOGIN_BLOCKED" || len(history.Decisions) != 1 {
		t.Fatal("signed history", err, history)
	}
	stale := h.cmd("LIFT_RESTRICTION", 0, nil, "")
	h.approve(&stale, "staff-b")
	if _, err := client.Execute(context.Background(), q, stale); !errors.Is(err, admincommand.ErrRefused) {
		t.Fatal("stale version not refused over the wire", err)
	}
	illegal := h.cmd("COMPLETE_REMEDIATION", 1, nil, "")
	if _, err := client.Execute(context.Background(), q, illegal); !errors.Is(err, admincommand.ErrRefused) {
		t.Fatal("illegal transition not refused over the wire", err)
	}
	wrong, _ := admincommand.NewClient(Owner, server.URL, []byte(strings.Repeat("x", 32)), read)
	lift := h.cmd("LIFT_RESTRICTION", 1, nil, "")
	h.approve(&lift, "staff-b")
	if _, err := wrong.Execute(context.Background(), q, lift); err == nil {
		t.Fatal("wrong write key accepted")
	}
	// A captured signed request cannot be replayed.
	req, _ := http.NewRequest(http.MethodGet, server.URL+Spec().Path()+"/customers/"+h.customer+"?"+q.Values().Encode(), nil)
	if err := serviceauth.SignRequest(req, adminread.Caller, read, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{200, 409} {
		replay := req.Clone(context.Background())
		resp, err := http.DefaultClient.Do(replay)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("attempt %d status %d", i, resp.StatusCode)
		}
	}
	if v, s := h.current(); v != 1 || s != "LOGIN_BLOCKED" {
		t.Fatal("refused wire commands changed standing", v, s)
	}
}

func TestRuntimeDefaultsOffAndRefusesUnreadableWrites(t *testing.T) {
	t.Setenv("CUSTOMER_ADMIN_COMMAND_WRITES_ENABLED", "")
	t.Setenv("CUSTOMER_ADMIN_COMMAND_READBACK_ENABLED", "")
	h, err := Runtime(nil)
	if err != nil || h.Service.WriteEnabled || h.Service.ReadEnabled || h.Service.Spec.Owner != Owner {
		t.Fatal("runtime not off by default", err)
	}
	t.Setenv("CUSTOMER_ADMIN_COMMAND_WRITES_ENABLED", "true")
	if _, err := Runtime(nil); !errors.Is(err, admincommand.ErrConfiguration) {
		t.Fatal("write without readback accepted", err)
	}
	t.Setenv("CUSTOMER_ADMIN_COMMAND_READBACK_ENABLED", "true")
	if _, err := Runtime(nil); !errors.Is(err, admincommand.ErrConfiguration) {
		t.Fatal("enabled runtime without an owner handler accepted", err)
	}
}
