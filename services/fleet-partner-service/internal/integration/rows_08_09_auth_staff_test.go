package integration_test

// Requirement-row tests for spec Sections 8 and 9 (and detail Section 38)
// that previously had use-case evidence only: session revocation and step-up
// over HTTP (8.16b, 8.16c), staff suspension and reactivation (9.5) and the
// staff invitation expiry sweep (9.2b). Everything runs through the fully
// wired production router (real OTP identity, sessions, tenant resolution and
// authorizer) against PostgreSQL.

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// rowsSession is one authenticated session.
type rowsSession struct {
	token     string
	sessionID string
}

// rowsSignIn signs an existing identity in through /auth/sign-in/start and
// /auth/verify and returns the issued session.
func rowsSignIn(t *testing.T, router *gin.Engine, h *authHarness, identifier string) rowsSession {
	t.Helper()
	if _, err := h.pool.Exec(context.Background(), `UPDATE fleet_partner_auth_otp_challenges SET resend_available_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	start := postJSON(router, "/fleet-partner/v1/auth/sign-in/start", map[string]string{"identifier": identifier}, nil)
	if start.Code != http.StatusAccepted {
		t.Fatalf("sign-in start = %d %s", start.Code, start.Body.String())
	}
	otp, _ := h.sender.LastOTP()
	verified := postJSON(router, "/fleet-partner/v1/auth/verify", map[string]string{
		"challengeId": decodeEnvelope(t, start)["data"].(map[string]any)["challengeId"].(string), "code": otp.Code}, nil)
	if verified.Code != http.StatusOK {
		t.Fatalf("sign-in verify = %d %s", verified.Code, verified.Body.String())
	}
	tokens, ok := decodeEnvelope(t, verified)["data"].(map[string]any)["tokens"].(map[string]any)
	if !ok {
		t.Fatalf("sign-in issued no session: %s", verified.Body.String())
	}
	return rowsSession{token: tokens["accessToken"].(string), sessionID: tokens["sessionId"].(string)}
}

func rowsSessionStatus(t *testing.T, h *authHarness, sessionID string) string {
	t.Helper()
	var status string
	if err := h.pool.QueryRow(context.Background(), `SELECT status FROM fleet_partner_corporate_sessions WHERE id = $1`, sessionID).Scan(&status); err != nil {
		t.Fatalf("session %s: %v", sessionID, err)
	}
	return status
}

// FLEET-8.16b DELETE /sessions/{sessionId}: a user revokes one of their own
// sessions; the revoked token stops working at once, other sessions keep
// working, and nobody can revoke someone else's session.
func TestRow8_16b_RevokeSessionOverHTTP(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	first := onboardingLogin(t, router, h, "r816-sessions@example.com", "+2348012347101")
	second := rowsSignIn(t, router, h, "r816-sessions@example.com")

	code, body := onboardCall(t, router, first, "GET", "/sessions", nil, 0)
	if code != http.StatusOK {
		t.Fatalf("GET /sessions = %d %v", code, body)
	}
	sessions := body["data"].([]any)
	var firstID string
	seenSecond := false
	for _, item := range sessions {
		session := item.(map[string]any)
		if session["current"] == true {
			firstID = session["sessionId"].(string)
		}
		if session["sessionId"] == second.sessionID {
			seenSecond = true
		}
		if _, leaked := session["refreshTokenHash"]; leaked {
			t.Fatal("refresh hash serialized")
		}
	}
	if firstID == "" || !seenSecond || firstID == second.sessionID {
		t.Fatalf("session list = %v", sessions)
	}

	// Revoke the second session from the first.
	code, body = onboardCall(t, router, first, "DELETE", "/sessions/"+second.sessionID, nil, 0)
	if code != http.StatusOK || body["data"].(map[string]any)["revoked"] != true {
		t.Fatalf("DELETE /sessions = %d %v", code, body)
	}
	if status := rowsSessionStatus(t, h, second.sessionID); status != "REVOKED" {
		t.Fatalf("revoked session status = %s", status)
	}
	if code, _ := onboardCall(t, router, second.token, "GET", "/identity/profile", nil, 0); code != http.StatusUnauthorized {
		t.Fatalf("revoked session still authenticates: %d", code)
	}
	if code, _ := onboardCall(t, router, first, "GET", "/identity/profile", nil, 0); code != http.StatusOK {
		t.Fatalf("the revoking session stopped working: %d", code)
	}
	if n := rowsCount(t, h.pool, `SELECT count(*) FROM fleet_partner_corporate_session_history WHERE session_id = $1 AND reason = 'user_revoked'`, second.sessionID); n != 1 {
		t.Fatalf("revocation history rows = %d, want 1", n)
	}
	// Revoking it again is a no-op, not an error.
	if code, _ := onboardCall(t, router, first, "DELETE", "/sessions/"+second.sessionID, nil, 0); code != http.StatusOK {
		t.Fatalf("repeat revoke = %d, want 200", code)
	}

	// Unknown session, and another identity's session: both 404, nothing changes.
	if code, _ := onboardCall(t, router, first, "DELETE", "/sessions/ses_does_not_exist", nil, 0); code != http.StatusNotFound {
		t.Fatalf("unknown session = %d, want 404", code)
	}
	stranger := onboardingLogin(t, router, h, "r816-stranger@example.com", "+2348012347102")
	if code, _ := onboardCall(t, router, stranger, "DELETE", "/sessions/"+firstID, nil, 0); code != http.StatusNotFound {
		t.Fatalf("revoking another identity's session = %d, want 404", code)
	}
	if status := rowsSessionStatus(t, h, firstID); status != "ACTIVE" {
		t.Fatalf("another identity changed this session to %s", status)
	}
	// Without a bearer token the route is unreachable.
	if code, _ := documentCall(t, router, "DELETE", "/fleet-partner/v1/sessions/"+firstID, nil, map[string]string{"Origin": "https://fleet.example.test"}); code != http.StatusUnauthorized {
		t.Fatalf("anonymous revoke = %d, want 401", code)
	}
}

// FLEET-8.16c POST /auth/step-up/start and /auth/step-up/verify: a fresh
// email challenge yields a reference bound to the session and the exact
// action; the protected action accepts it once, and refuses a reused, foreign
// or mismatched one.
func TestRow8_16c_StepUpOverHTTP(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	token := onboardingLogin(t, router, h, "r816-stepup@example.com", "+2348012347201")
	newPhone := "+2348012347299"

	stepUp := func(token, actionClass string) string {
		t.Helper()
		code, body := onboardCall(t, router, token, "POST", "/auth/step-up/start", map[string]any{"actionClass": actionClass}, 0)
		if code != http.StatusAccepted {
			t.Fatalf("step-up start = %d %v", code, body)
		}
		challenge := body["data"].(map[string]any)
		if challenge["challengeId"] == "" || challenge["maskedDestination"] == "" {
			t.Fatalf("step-up challenge = %v", challenge)
		}
		otp, _ := h.sender.LastOTP()
		if otp.Purpose != "STEP_UP" {
			t.Fatalf("step-up OTP purpose = %q", otp.Purpose)
		}
		code, body = onboardCall(t, router, token, "POST", "/auth/step-up/verify", map[string]any{"challengeId": challenge["challengeId"], "code": otp.Code}, 0)
		if code != http.StatusOK {
			t.Fatalf("step-up verify = %d %v", code, body)
		}
		reference, _ := body["data"].(map[string]any)["stepUpReference"].(string)
		if reference == "" || body["data"].(map[string]any)["expiresAt"] == nil {
			t.Fatalf("step-up verify response = %v", body)
		}
		if _, leaked := body["data"].(map[string]any)["accessToken"]; leaked {
			t.Fatal("step-up verify must not issue a session")
		}
		return reference
	}
	changePhone := func(token, phone, reference string) (int, map[string]any) {
		return onboardCall(t, router, token, "POST", "/auth/phone/change", map[string]any{"phone": phone, "stepUpReference": reference}, 0)
	}
	cooldown := func() {
		if _, err := h.pool.Exec(context.Background(), `UPDATE fleet_partner_auth_otp_challenges SET resend_available_at = now() - interval '1 second'`); err != nil {
			t.Fatal(err)
		}
	}

	// No reference, or a reference for a different action: refused.
	if code, body := changePhone(token, newPhone, ""); code != http.StatusForbidden || rowsErrorCode(body) == "" {
		t.Fatalf("phone change without step-up = %d %v", code, body)
	}
	wrongAction := stepUp(token, "phone_change:+2348012340000")
	if code, _ := changePhone(token, newPhone, wrongAction); code != http.StatusForbidden {
		t.Fatalf("step-up for another phone accepted: %d", code)
	}

	// A wrong code does not verify.
	cooldown()
	code, body := onboardCall(t, router, token, "POST", "/auth/step-up/start", map[string]any{"actionClass": "phone_change:" + newPhone}, 0)
	if code != http.StatusAccepted {
		t.Fatalf("step-up start = %d %v", code, body)
	}
	challengeID := body["data"].(map[string]any)["challengeId"]
	if code, _ := onboardCall(t, router, token, "POST", "/auth/step-up/verify", map[string]any{"challengeId": challengeID, "code": "000000"}, 0); code < 400 || code >= 500 {
		t.Fatalf("wrong step-up code = %d, want a 4xx refusal", code)
	}

	// The reference belongs to the session that earned it.
	cooldown()
	reference := stepUp(token, "phone_change:"+newPhone)
	other := rowsSignIn(t, router, h, "r816-stepup@example.com")
	if code, _ := changePhone(other.token, newPhone, reference); code != http.StatusForbidden {
		t.Fatalf("step-up reference used from another session: %d", code)
	}

	// The right session, action and reference: accepted once.
	if code, body := changePhone(token, newPhone, reference); code != http.StatusOK {
		t.Fatalf("phone change with step-up = %d %v", code, body)
	}
	var phone string
	if err := h.pool.QueryRow(context.Background(), `SELECT phone_e164 FROM fleet_partner_corporate_identities WHERE email_normalized = 'r816-stepup@example.com'`).Scan(&phone); err != nil || phone != newPhone {
		t.Fatalf("stored phone = %q (%v), want %s", phone, err, newPhone)
	}
	if code, _ := changePhone(token, newPhone, reference); code != http.StatusForbidden {
		t.Fatalf("reused step-up reference = %d, want 403", code)
	}

	// Anonymous callers cannot start step-up.
	if code, _ := documentCall(t, router, "POST", "/fleet-partner/v1/auth/step-up/start", map[string]any{"actionClass": "x"}, map[string]string{"Origin": "https://fleet.example.test"}); code != http.StatusUnauthorized {
		t.Fatalf("anonymous step-up start = %d, want 401", code)
	}
}

// rowsActiveCompany has an owner create a company and marks it ACTIVE so
// operational writes pass the lifecycle gate.
func rowsActiveCompany(t *testing.T, router *gin.Engine, h *authHarness, owner, legalName, registration string) string {
	t.Helper()
	company := map[string]any{"legalName": legalName, "registrationNumber": registration, "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Rows Owner"}
	if code, body := onboardCall(t, router, owner, "POST", "/organizations", company, 0); code != http.StatusCreated {
		t.Fatalf("create company = %d %v", code, body)
	}
	var orgID string
	if err := h.pool.QueryRow(context.Background(), `SELECT id FROM fleet_partner_organizations WHERE registration_number = $1`, registration).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(context.Background(), `UPDATE fleet_partner_organizations SET status = 'ACTIVE' WHERE id = $1`, orgID); err != nil {
		t.Fatal(err)
	}
	return orgID
}

func rowsStaffVersion(t *testing.T, h *authHarness, memberID string) int {
	t.Helper()
	var version int
	if err := h.pool.QueryRow(context.Background(), `SELECT version FROM fleet_partner_organization_users WHERE id = $1`, memberID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

// FLEET-9.5 / 38.3 POST /staff/{id}/suspend and /reactivate: suspension needs
// the dedicated permission and the expected version, revokes the member's
// sessions immediately, bumps the role version, is audited and announced, and
// protects the last owner; reactivation restores access.
func TestRow9_5_SuspendAndReactivateStaff(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	ctx := context.Background()
	owner := onboardingLogin(t, router, h, "r95-owner@example.com", "+2348012347301")
	orgID := rowsActiveCompany(t, router, h, owner, "Rows95 Staff Co", "RC-R95-1")

	member := seedMember(t, h.pool, orgID, seedIdentity(t, h.pool, "r95-staff@example.com", "+2348012347302"), false, nil)
	bindRole(t, h.pool, orgID, member.ID, "rol_system_dispatcher", staffmodels.ScopeOrganization, "")
	staff := rowsSignIn(t, router, h, "r95-staff@example.com")
	if code, body := onboardCall(t, router, staff.token, "GET", "/branches", nil, 0); code != http.StatusOK {
		t.Fatalf("staff member before suspension = %d %v", code, body)
	}
	var roleVersion int
	if err := h.pool.QueryRow(ctx, `SELECT role_version FROM fleet_partner_organization_users WHERE id = $1`, member.ID).Scan(&roleVersion); err != nil {
		t.Fatal(err)
	}

	path := "/staff/" + member.ID
	// Suspension and reactivation require step-up (spec 64.1); the owner
	// confirms once and the grant covers this test's access changes.
	ownerGrant := obtainStepUp(t, router, h, owner, stepup.ActionAccessManage)
	// The member cannot suspend the owner: no staff.suspend permission.
	var ownerID string
	if err := h.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_organization_users WHERE organization_id = $1 AND is_owner`, orgID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	// The member holds a valid step-up grant, so the refusal below is the
	// missing permission, not the missing confirmation.
	staffGrant := obtainStepUp(t, router, h, staff.token, stepup.ActionAccessManage)
	if code, body := onboardCallStepUp(t, router, staff.token, staffGrant, "POST", "/staff/"+ownerID+"/suspend", map[string]any{}, rowsStaffVersion(t, h, ownerID)); code != http.StatusForbidden || body["error"].(map[string]any)["code"] == "step_up_required" {
		t.Fatalf("suspend without staff.suspend = %d %v, want 403 permission refusal", code, body)
	}
	// A stale version is refused and changes nothing.
	if code, _ := onboardCallStepUp(t, router, owner, ownerGrant, "POST", path+"/suspend", map[string]any{}, 99); code != http.StatusConflict {
		t.Fatalf("stale suspend = %d, want 409", code)
	}
	code, body := onboardCallStepUp(t, router, owner, ownerGrant, "POST", path+"/suspend", map[string]any{}, rowsStaffVersion(t, h, member.ID))
	if code != http.StatusOK || body["data"].(map[string]any)["status"] != staffmodels.UserStatusSuspended {
		t.Fatalf("suspend = %d %v", code, body)
	}

	// Access stops at once: the member's live session is revoked.
	if code, _ := onboardCall(t, router, staff.token, "GET", "/branches", nil, 0); code != http.StatusUnauthorized && code != http.StatusForbidden {
		t.Fatalf("suspended member's session still works: %d", code)
	}
	if status := rowsSessionStatus(t, h, staff.sessionID); status != "REVOKED" {
		t.Fatalf("suspended member's session = %s, want REVOKED", status)
	}
	var newRoleVersion int
	if err := h.pool.QueryRow(ctx, `SELECT role_version FROM fleet_partner_organization_users WHERE id = $1`, member.ID).Scan(&newRoleVersion); err != nil {
		t.Fatal(err)
	}
	if newRoleVersion <= roleVersion {
		t.Fatalf("role version %d -> %d: cached authorization was not invalidated", roleVersion, newRoleVersion)
	}
	if n := rowsCount(t, h.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id = $1 AND action = 'staff.status_changed' AND resource_id = $2`, orgID, member.ID); n != 1 {
		t.Fatalf("suspension audit rows = %d", n)
	}
	if n := rowsCount(t, h.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id = $1 AND event_type = 'organization_user.suspended.v1' AND aggregate_id = $2`, orgID, member.ID); n != 1 {
		t.Fatalf("suspension events = %d", n)
	}

	// Suspending an already suspended member is a state conflict, not a
	// second suspension.
	if code, body := onboardCallStepUp(t, router, owner, ownerGrant, "POST", path+"/suspend", map[string]any{}, rowsStaffVersion(t, h, member.ID)); code != http.StatusConflict {
		t.Fatalf("repeat suspend = %d %v, want 409", code, body)
	}

	// Another organization cannot touch this member.
	other := onboardingLogin(t, router, h, "r95-other@example.com", "+2348012347303")
	rowsActiveCompany(t, router, h, other, "Rows95 Other Co", "RC-R95-2")
	otherGrant := obtainStepUp(t, router, h, other, stepup.ActionAccessManage)
	if code, _ := onboardCallStepUp(t, router, other, otherGrant, "POST", path+"/reactivate", map[string]any{}, rowsStaffVersion(t, h, member.ID)); code != http.StatusNotFound {
		t.Fatalf("cross-tenant reactivate = %d, want 404", code)
	}

	// Reactivation restores access through a new sign-in.
	code, body = onboardCallStepUp(t, router, owner, ownerGrant, "POST", path+"/reactivate", map[string]any{}, rowsStaffVersion(t, h, member.ID))
	if code != http.StatusOK || body["data"].(map[string]any)["status"] != staffmodels.UserStatusActive {
		t.Fatalf("reactivate = %d %v", code, body)
	}
	if n := rowsCount(t, h.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id = $1 AND event_type = 'organization_user.reactivated.v1' AND aggregate_id = $2`, orgID, member.ID); n != 1 {
		t.Fatalf("reactivation events = %d", n)
	}
	again := rowsSignIn(t, router, h, "r95-staff@example.com")
	if code, _ := onboardCall(t, router, again.token, "GET", "/branches", nil, 0); code != http.StatusOK {
		t.Fatalf("reactivated member = %d, want 200", code)
	}
	// Reactivating an active member is a state conflict.
	if code, body := onboardCallStepUp(t, router, owner, ownerGrant, "POST", path+"/reactivate", map[string]any{}, rowsStaffVersion(t, h, member.ID)); code != http.StatusConflict {
		t.Fatalf("reactivating an active member = %d %v, want 409", code, body)
	}

	// The last active owner cannot be suspended.
	if code, body := onboardCallStepUp(t, router, owner, ownerGrant, "POST", "/staff/"+ownerID+"/suspend", map[string]any{}, rowsStaffVersion(t, h, ownerID)); code != http.StatusConflict || rowsErrorCode(body) != "last_owner_required" {
		t.Fatalf("suspending the last owner = %d %v", code, body)
	}
}

// FLEET-9.2b / 38.2 staff invitation expiry: invitations are single-use,
// expiring and stored as hashes. The maintenance sweep expires overdue
// pending invitations only; an expired token can no longer be accepted,
// swept or not.
func TestRow9_2b_StaffInvitationExpirySweep(t *testing.T) {
	ctx := context.Background()
	var invitations *staffrepos.InvitationRepository
	h := newAuthHarness(t, nil, func(opts *authusecases.Options) {
		invitations = staffrepos.NewInvitationRepository(opts.Pool)
		staff := rowsStaffService(opts, invitations)
		opts.Invitations = staff
		opts.Memberships = staff
	})
	org := createOrganization(t, h.pool, "Rows92 Invitations Co", "RC-R92-1")
	other := createOrganization(t, h.pool, "Rows92 Other Co", "RC-R92-2")

	seed := func(organizationID, token, email string, expiresAt time.Time) *staffmodels.StaffInvitation {
		invitation := &staffmodels.StaffInvitation{FullName: "Invited", EmailNormalized: email, PhoneE164: "+23480123474" + fmt.Sprint(len(token)%10) + fmt.Sprint(len(email)%10),
			InvitationTokenHash: auth.HashRefreshToken([]byte("rows92-invite-secret"), token), ExpiresAt: expiresAt, CreatedBy: "test-owner"}
		if err := invitations.Create(ctx, h.pool, organizationID, invitation); err != nil {
			t.Fatal(err)
		}
		return invitation
	}
	overdue := seed(org.ID, "rows92-overdue-token", "r92-overdue@example.com", time.Now().Add(-time.Hour))
	unswept := seed(org.ID, "rows92-unswept-token-x", "r92-unswept@example.com", time.Now().Add(-time.Minute))
	live := seed(org.ID, "rows92-live-token-xyz", "r92-live@example.com", time.Now().Add(time.Hour))
	foreign := seed(other.ID, "rows92-foreign-token-ab", "r92-foreign@example.com", time.Now().Add(-time.Hour))
	accepted := seed(org.ID, "rows92-accepted-token-abc", "r92-accepted@example.com", time.Now().Add(-time.Hour))
	if _, err := h.pool.Exec(ctx, `UPDATE fleet_partner_organization_staff_invitations SET status = 'ACCEPTED' WHERE id = $1`, accepted.ID); err != nil {
		t.Fatal(err)
	}

	// An expired token is refused even before the sweep has run.
	if _, err := h.service.Start(ctx, authusecases.StartInput{Email: unswept.EmailNormalized, Phone: unswept.PhoneE164, InvitationToken: "rows92-unswept-token-x"}); err == nil {
		t.Fatal("an expired (not yet swept) invitation was accepted")
	}

	// The production maintenance worker, with the staff invitation sweep.
	worker := &jobs.MaintenanceWorker{Metrics: observability.NewMetrics(), Interval: 20 * time.Millisecond,
		Sweeps: map[string]jobs.Expirer{"staff_invitations": invitations}}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { worker.Run(runCtx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for rowsCount(t, h.pool, `SELECT count(*) FROM fleet_partner_organization_staff_invitations WHERE status = 'EXPIRED'`) < 3 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done

	status := func(id string) string {
		var value string
		if err := h.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_organization_staff_invitations WHERE id = $1`, id).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	for name, want := range map[*staffmodels.StaffInvitation]string{overdue: "EXPIRED", unswept: "EXPIRED", foreign: "EXPIRED", live: "PENDING", accepted: "ACCEPTED"} {
		if got := status(name.ID); got != want {
			t.Fatalf("invitation %s = %s, want %s", name.EmailNormalized, got, want)
		}
	}

	// A swept invitation stays unusable, the live one still works.
	if _, err := h.service.Start(ctx, authusecases.StartInput{Email: overdue.EmailNormalized, Phone: overdue.PhoneE164, InvitationToken: "rows92-overdue-token"}); err == nil {
		t.Fatal("a swept invitation was accepted")
	}
	if _, err := h.service.Start(ctx, authusecases.StartInput{Email: live.EmailNormalized, Phone: live.PhoneE164, InvitationToken: "rows92-live-token-xyz"}); err != nil {
		t.Fatalf("a live invitation was refused: %v", err)
	}
	// Only the hash is stored: the raw token appears nowhere.
	if n := rowsCount(t, h.pool, `SELECT count(*) FROM fleet_partner_organization_staff_invitations WHERE invitation_token_hash LIKE 'rows92-%'`); n != 0 {
		t.Fatalf("raw invitation tokens stored: %d", n)
	}
}

// rowsStaffService builds the real staff service the invitation flow uses.
func rowsStaffService(opts *authusecases.Options, invitations *staffrepos.InvitationRepository) *staffusecases.Service {
	return staffusecases.NewService(staffusecases.Options{Pool: opts.Pool,
		Members: staffrepos.NewMemberRepository(opts.Pool), Invitations: invitations,
		Roles: staffrepos.NewRoleRepository(opts.Pool), Organizations: authTestOrganizations{opts.Pool},
		Events:  events.NewDomainPublisher(events.NewOutboxRepository(opts.Pool)),
		Metrics: observability.NewMetrics(), InvitationSecret: []byte("rows92-invite-secret")})
}
