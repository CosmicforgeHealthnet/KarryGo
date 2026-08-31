package vendorauthusecases_test

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	vendorauthrepositories "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
	vendorauthusecases "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"
)

// fakeStaffResolver is an in-memory stand-in for the staff feature, letting
// these tests exercise AuthService in isolation against real Postgres for
// everything auth itself owns (challenges, sessions, step-up), without
// needing the staff feature's own schema populated. This is exactly what the
// StaffResolver port exists for.
type fakeStaffResolver struct {
	mu           sync.Mutex
	affiliations map[string][]vendorauthusecases.StaffAffiliation // userID -> affiliations
	byStaffID    map[string]vendorauthusecases.StaffAffiliation
}

func newFakeStaffResolver() *fakeStaffResolver {
	return &fakeStaffResolver{affiliations: map[string][]vendorauthusecases.StaffAffiliation{}, byStaffID: map[string]vendorauthusecases.StaffAffiliation{}}
}

func (f *fakeStaffResolver) addStaff(userID, staffID, vendorID string, active bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := vendorauthusecases.StaffAffiliation{StaffID: staffID, VendorID: vendorID, UserID: userID, Active: active}
	f.affiliations[userID] = append(f.affiliations[userID], a)
	f.byStaffID[staffID] = a
}

// seedStaff registers a staff affiliation in the fake resolver AND inserts a
// minimal real vendor_staff row, since vendor_sessions.staff_id is a genuine
// foreign key to vendor_staff (id) â€” a real constraint this migration
// deliberately enforces (see migrations/000004), so any test that reaches
// session creation needs a real backing row even while the rest of staff
// status/affiliation logic is exercised through the fake resolver.
func seedStaff(t *testing.T, pool *pgxpool.Pool, staff *fakeStaffResolver, userID, staffID, vendorID string, active bool) {
	t.Helper()
	status := "active"
	if !active {
		status = "suspended"
	}
	_, err := pool.Exec(context.Background(), `
		INSERT INTO vendor_staff (id, vendor_id, user_id, status) VALUES ($1,$2,$3,$4)
		ON CONFLICT (vendor_id, user_id) DO UPDATE SET status = $4`,
		staffID, vendorID, userID, status)
	if err != nil {
		t.Fatalf("seed vendor_staff row: %v", err)
	}
	staff.addStaff(userID, staffID, vendorID, active)
}

func (f *fakeStaffResolver) setActive(staffID string, active bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.byStaffID[staffID]
	a.Active = active
	f.byStaffID[staffID] = a
	for uid, list := range f.affiliations {
		for i, aff := range list {
			if aff.StaffID == staffID {
				list[i].Active = active
			}
		}
		f.affiliations[uid] = list
	}
}

func (f *fakeStaffResolver) ListActiveByUser(ctx context.Context, userID string) ([]vendorauthusecases.StaffAffiliation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []vendorauthusecases.StaffAffiliation
	for _, a := range f.affiliations[userID] {
		if a.Active {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeStaffResolver) GetByVendorAndUser(ctx context.Context, vendorID, userID string) (vendorauthusecases.StaffAffiliation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.affiliations[userID] {
		if a.VendorID == vendorID {
			return a, nil
		}
	}
	return vendorauthusecases.StaffAffiliation{}, vendorauthrepositories.ErrNotFound
}

func (f *fakeStaffResolver) GetByID(ctx context.Context, staffID string) (vendorauthusecases.StaffAffiliation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.byStaffID[staffID]
	if !ok {
		return vendorauthusecases.StaffAffiliation{}, vendorauthrepositories.ErrNotFound
	}
	return a, nil
}

func setupAuthService(t *testing.T, pool *pgxpool.Pool, staff *fakeStaffResolver) *vendorauthusecases.AuthService {
	t.Helper()
	return vendorauthusecases.NewAuthService(vendorauthusecases.Options{
		Users:      vendorauthrepositories.NewUserRepository(pool),
		Challenges: vendorauthrepositories.NewOTPChallengeRepository(pool),
		Sessions:   vendorauthrepositories.NewSessionRepository(pool),
		StepUps:    vendorauthrepositories.NewStepUpRepository(pool),
		Staff:      staff,
		Notifier:   nil, // OTPDebug=true skips real delivery entirely â€” see deliverOTP

		AccessTokenSecret: []byte("test-access-secret"), RefreshTokenSecret: []byte("test-refresh-secret"), OTPSecret: []byte("test-otp-secret"),
		AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 30 * 24 * time.Hour,
		OTPTTL: 5 * time.Minute, OTPMaxAttempts: 3, OTPMaxResends: 3,
		StepUpTTL: 5 * time.Minute, OTPDebug: true,
	})
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres auth integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.ApplyMigrations(ctx, pool, "../../../../migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	t.Cleanup(pool.Close)
	truncateAuthTables(t, pool)
	return pool
}

func truncateAuthTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Auth integration tests own challenges, sessions and step-up grants.
	// They must not TRUNCATE shared Vendor staff/user authority tables.
	// Doing so with CASCADE can remove unrelated domain seed data such as
	// the platform Shop measurement catalogue.
	if _, err := pool.Exec(ctx, `DELETE FROM vendor_step_up_grants`); err != nil {
		t.Fatalf("clean step-up grants: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM vendor_sessions`); err != nil {
		t.Fatalf("clean sessions: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM vendor_otp_challenges`); err != nil {
		t.Fatalf("clean otp challenges: %v", err)
	}
}

func TestAuth_FullLoginLifecycle_SingleVendor(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	start, err := auth.Start(ctx, vendorauthusecases.StartInput{Email: "owner@example.com"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if start.DebugOTP == "" {
		t.Fatal("expected DebugOTP in debug mode")
	}

	// Wrong code fails.
	if _, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "owner@example.com", OTP: "000000", ChallengeID: start.ChallengeID}); err == nil {
		t.Fatal("expected wrong OTP to fail")
	}

	// No staff access yet -> forbidden even with the right code (consumes... but let's check staff-access-cardinality path first, which happens before consume).
	if _, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "owner@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID}); err == nil {
		t.Fatal("expected forbidden: no active staff access to any vendor business")
	}

	// The challenge must still be usable since the previous attempt failed
	// before consuming it (no staff access is checked before Consume) â€” add
	// staff access now and retry with the SAME still-active code.
	// First resolve the user id that Start's upsert created.
	users := vendorauthrepositories.NewUserRepository(pool)
	user, err := users.UpsertByEmail(ctx, "owner@example.com")
	if err != nil {
		t.Fatalf("resolve user: %v", err)
	}
	seedStaff(t, pool, staff, user.ID, "staff-1", "vendor-1", true)

	result, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "owner@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID})
	if err != nil {
		t.Fatalf("expected verify to succeed once staff access exists: %v", err)
	}
	if result.Tokens == nil || result.Tokens.AccessToken == "" || result.Tokens.RefreshToken == "" {
		t.Fatal("expected tokens to be issued")
	}
	if result.Tokens.StaffID != "staff-1" || result.Tokens.VendorID != "vendor-1" {
		t.Fatalf("unexpected staff/vendor in token result: %+v", result.Tokens)
	}

	// The SAME code cannot be verified again (consumed).
	if _, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "owner@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID}); err == nil {
		t.Fatal("expected a consumed OTP to fail on reuse")
	}

	// Refresh works.
	refreshed, err := auth.Refresh(ctx, vendorauthusecases.RefreshInput{RefreshToken: result.Tokens.RefreshToken})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if refreshed.AccessToken == "" || refreshed.RefreshToken == result.Tokens.RefreshToken {
		t.Fatal("expected refresh to rotate to a new refresh token")
	}

	// The OLD refresh token no longer works (rotated away).
	if _, err := auth.Refresh(ctx, vendorauthusecases.RefreshInput{RefreshToken: result.Tokens.RefreshToken}); err == nil {
		t.Fatal("expected the old, rotated-away refresh token to fail")
	}

	// Logout revokes.
	if err := auth.Logout(ctx, refreshed.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := auth.Refresh(ctx, vendorauthusecases.RefreshInput{RefreshToken: refreshed.RefreshToken}); err == nil {
		t.Fatal("expected refresh to fail after logout")
	}
}

func TestAuth_ExpiredOTPFails(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	start, err := auth.Start(ctx, vendorauthusecases.StartInput{Email: "expiry@example.com"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Force expiry directly in the DB (faster than waiting out a real TTL).
	if _, err := pool.Exec(ctx, `UPDATE vendor_otp_challenges SET expires_at = now() - interval '1 minute' WHERE id = $1`, start.ChallengeID); err != nil {
		t.Fatalf("force expiry: %v", err)
	}
	if _, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "expiry@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID}); err == nil {
		t.Fatal("expected expired OTP to fail")
	}
}

func TestAuth_ResendInvalidatesPreviousCode(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	start, err := auth.Start(ctx, vendorauthusecases.StartInput{Email: "resend@example.com"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	oldOTP := start.DebugOTP

	resent, err := auth.Resend(ctx, vendorauthusecases.ResendInput{Email: "resend@example.com", ChallengeID: start.ChallengeID})
	if err != nil {
		t.Fatalf("Resend: %v", err)
	}
	if resent.ChallengeID != start.ChallengeID {
		t.Fatal("expected resend to keep the same challenge id")
	}
	if resent.DebugOTP == oldOTP {
		t.Fatal("expected resend to generate a different code")
	}

	users := vendorauthrepositories.NewUserRepository(pool)
	user, _ := users.UpsertByEmail(ctx, "resend@example.com")
	seedStaff(t, pool, staff, user.ID, "staff-r", "vendor-r", true)

	// The OLD code must be rejected even though the challenge id is still active.
	if _, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "resend@example.com", OTP: oldOTP, ChallengeID: start.ChallengeID}); err == nil {
		t.Fatal("expected the pre-resend code to be rejected")
	}
	// The NEW code succeeds.
	if _, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "resend@example.com", OTP: resent.DebugOTP, ChallengeID: start.ChallengeID}); err != nil {
		t.Fatalf("expected the post-resend code to succeed: %v", err)
	}
}

func TestAuth_MaxAttemptsLocksChallenge(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	start, err := auth.Start(ctx, vendorauthusecases.StartInput{Email: "lockout@example.com"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// OTPMaxAttempts=3: three wrong guesses lock it.
	for i := 0; i < 3; i++ {
		if _, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "lockout@example.com", OTP: "000000", ChallengeID: start.ChallengeID}); err == nil {
			t.Fatal("expected wrong OTP to fail")
		}
	}

	// The CORRECT code must now also fail â€” the challenge is locked, not just attempts exhausted transiently.
	if _, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "lockout@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID}); err == nil {
		t.Fatal("expected the correct code to be rejected once the challenge is locked")
	}
}

func TestAuth_ConcurrentVerifySucceedsOnlyOnce(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	start, err := auth.Start(ctx, vendorauthusecases.StartInput{Email: "concurrent@example.com"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	users := vendorauthrepositories.NewUserRepository(pool)
	user, _ := users.UpsertByEmail(ctx, "concurrent@example.com")
	seedStaff(t, pool, staff, user.ID, "staff-c", "vendor-c", true)

	const workers = 10
	var successes int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			result, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "concurrent@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID})
			if err == nil && result.Tokens != nil {
				atomic.AddInt64(&successes, 1)
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly 1 successful concurrent verify, got %d", successes)
	}
}

func TestAuth_SuspendedStaffCannotLogin(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	users := vendorauthrepositories.NewUserRepository(pool)
	user, _ := users.UpsertByEmail(ctx, "suspended@example.com")
	seedStaff(t, pool, staff, user.ID, "staff-s", "vendor-s", false) // inactive from the start

	start, err := auth.Start(ctx, vendorauthusecases.StartInput{Email: "suspended@example.com"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "suspended@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID}); err == nil {
		t.Fatal("expected suspended staff to be refused login")
	}
}

func TestAuth_SuspendedStaffCannotRefresh(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	users := vendorauthrepositories.NewUserRepository(pool)
	user, _ := users.UpsertByEmail(ctx, "suspend-after@example.com")
	seedStaff(t, pool, staff, user.ID, "staff-sa", "vendor-sa", true)

	start, err := auth.Start(ctx, vendorauthusecases.StartInput{Email: "suspend-after@example.com"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	result, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "suspend-after@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	// Suspend AFTER the session was already issued â€” refresh must re-check
	// status live, not just at login (Section 5 rule 10/13).
	staff.setActive("staff-sa", false)

	if _, err := auth.Refresh(ctx, vendorauthusecases.RefreshInput{RefreshToken: result.Tokens.RefreshToken}); err == nil {
		t.Fatal("expected refresh to fail once the staff member is suspended, even with a still-valid refresh token")
	}
}

func TestAuth_MultiVendorRequiresSelectionWithoutBurningTheOTP(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	users := vendorauthrepositories.NewUserRepository(pool)
	user, _ := users.UpsertByEmail(ctx, "multi@example.com")
	seedStaff(t, pool, staff, user.ID, "staff-m1", "vendor-m1", true)
	seedStaff(t, pool, staff, user.ID, "staff-m2", "vendor-m2", true)

	start, err := auth.Start(ctx, vendorauthusecases.StartInput{Email: "multi@example.com"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	result, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "multi@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID})
	if err != nil {
		t.Fatalf("expected an ambiguous-vendor response, not an error: %v", err)
	}
	if !result.RequiresVendorChoice || len(result.AvailableVendorIDs) != 2 {
		t.Fatalf("expected a vendor-choice response with 2 options, got %+v", result)
	}
	if result.Tokens != nil {
		t.Fatal("expected no tokens to be issued yet")
	}

	// The SAME still-active code, now with vendor_id supplied, must succeed â€”
	// proving the ambiguous response did not consume/burn the OTP.
	result2, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "multi@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID, VendorID: "vendor-m2"})
	if err != nil {
		t.Fatalf("expected the same code to succeed once vendor_id is supplied: %v", err)
	}
	if result2.Tokens == nil || result2.Tokens.VendorID != "vendor-m2" {
		t.Fatalf("expected tokens scoped to vendor-m2, got %+v", result2.Tokens)
	}
}

// sessionIDFromRefreshToken extracts the session id prefix from a refresh
// token ("<sessionID>.<secret>") â€” vendor_step_up_grants.session_id is a real
// foreign key to vendor_sessions (id), so step-up tests need the actual
// session id a real login produced, not a synthetic placeholder.
func sessionIDFromRefreshToken(token string) string {
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			return token[:i]
		}
	}
	return token
}

func TestAuth_StepUp_FullLifecycleAndBindingChecks(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	users := vendorauthrepositories.NewUserRepository(pool)
	user, _ := users.UpsertByEmail(ctx, "stepup@example.com")
	seedStaff(t, pool, staff, user.ID, "staff-su", "vendor-su", true)

	start, _ := auth.Start(ctx, vendorauthusecases.StartInput{Email: "stepup@example.com"})
	login, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "stepup@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	sessionID := sessionIDFromRefreshToken(login.Tokens.RefreshToken)

	stepUpStart, err := auth.StepUpStart(ctx, vendorauthusecases.StepUpStartInput{
		SessionID: sessionID, StaffID: "staff-su", VendorID: "vendor-su", Purpose: "iam.high_risk_permission_change",
	})
	if err != nil {
		t.Fatalf("StepUpStart: %v", err)
	}

	// Wrong purpose fails.
	if _, err := auth.StepUpVerify(ctx, vendorauthusecases.StepUpVerifyInput{
		SessionID: sessionID, StaffID: "staff-su", VendorID: "vendor-su", Purpose: "different.purpose",
		ChallengeID: stepUpStart.ChallengeID, OTP: stepUpStart.DebugOTP,
	}); err == nil {
		t.Fatal("expected step-up verify with the wrong purpose to fail")
	}
	// Wrong session fails.
	if _, err := auth.StepUpVerify(ctx, vendorauthusecases.StepUpVerifyInput{
		SessionID: "some-other-session", StaffID: "staff-su", VendorID: "vendor-su", Purpose: "iam.high_risk_permission_change",
		ChallengeID: stepUpStart.ChallengeID, OTP: stepUpStart.DebugOTP,
	}); err == nil {
		t.Fatal("expected step-up verify bound to a different session to fail")
	}
	// Wrong staff fails.
	if _, err := auth.StepUpVerify(ctx, vendorauthusecases.StepUpVerifyInput{
		SessionID: sessionID, StaffID: "some-other-staff", VendorID: "vendor-su", Purpose: "iam.high_risk_permission_change",
		ChallengeID: stepUpStart.ChallengeID, OTP: stepUpStart.DebugOTP,
	}); err == nil {
		t.Fatal("expected step-up verify bound to a different staff to fail")
	}
	// Wrong vendor fails.
	if _, err := auth.StepUpVerify(ctx, vendorauthusecases.StepUpVerifyInput{
		SessionID: sessionID, StaffID: "staff-su", VendorID: "some-other-vendor", Purpose: "iam.high_risk_permission_change",
		ChallengeID: stepUpStart.ChallengeID, OTP: stepUpStart.DebugOTP,
	}); err == nil {
		t.Fatal("expected step-up verify bound to a different vendor to fail")
	}

	// Correct binding succeeds.
	grant, err := auth.StepUpVerify(ctx, vendorauthusecases.StepUpVerifyInput{
		SessionID: sessionID, StaffID: "staff-su", VendorID: "vendor-su", Purpose: "iam.high_risk_permission_change",
		ChallengeID: stepUpStart.ChallengeID, OTP: stepUpStart.DebugOTP,
	})
	if err != nil {
		t.Fatalf("expected correctly-bound step-up verify to succeed: %v", err)
	}
	if grant.GrantID == "" {
		t.Fatal("expected a grant id")
	}

	// ConsumeGrant succeeds exactly once.
	if err := auth.ConsumeGrant(ctx, grant.GrantID, sessionID, "staff-su", "vendor-su", "iam.high_risk_permission_change"); err != nil {
		t.Fatalf("expected first consume to succeed: %v", err)
	}
	if err := auth.ConsumeGrant(ctx, grant.GrantID, sessionID, "staff-su", "vendor-su", "iam.high_risk_permission_change"); err == nil {
		t.Fatal("expected a second consume of the same grant to fail")
	}
}

func TestAuth_LogoutRevokesStepUpGrantsForSession(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	users := vendorauthrepositories.NewUserRepository(pool)
	user, _ := users.UpsertByEmail(ctx, "logout-stepup@example.com")
	seedStaff(t, pool, staff, user.ID, "staff-ls", "vendor-ls", true)
	start, _ := auth.Start(ctx, vendorauthusecases.StartInput{Email: "logout-stepup@example.com"})
	login, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "logout-stepup@example.com", OTP: start.DebugOTP, ChallengeID: start.ChallengeID})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	sessionID := sessionIDFromRefreshToken(login.Tokens.RefreshToken)
	stepUpStart, err := auth.StepUpStart(ctx, vendorauthusecases.StepUpStartInput{SessionID: sessionID, StaffID: "staff-ls", VendorID: "vendor-ls", Purpose: "finance.bank_change"})
	if err != nil {
		t.Fatalf("StepUpStart: %v", err)
	}
	grant, err := auth.StepUpVerify(ctx, vendorauthusecases.StepUpVerifyInput{
		SessionID: sessionID, StaffID: "staff-ls", VendorID: "vendor-ls", Purpose: "finance.bank_change",
		ChallengeID: stepUpStart.ChallengeID, OTP: stepUpStart.DebugOTP,
	})
	if err != nil {
		t.Fatalf("StepUpVerify: %v", err)
	}

	if err := auth.Logout(ctx, login.Tokens.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// The grant, still unconsumed, must now be unusable â€” logout revokes
	// every active step-up grant tied to the logged-out session (Section 5's
	// own requirement that a step-up grant "cannot be used after
	// staff/session revocation").
	if err := auth.ConsumeGrant(ctx, grant.GrantID, sessionID, "staff-ls", "vendor-ls", "finance.bank_change"); err == nil {
		t.Fatal("expected the step-up grant to be revoked (unusable) after logout")
	}
}

// TestAuth_RawOTPNeverPersisted is Section 5 security scenario 1: the raw
// code must never be written to Postgres, only a safe HMAC verifier.
func TestAuth_RawOTPNeverPersisted(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	start, err := auth.Start(ctx, vendorauthusecases.StartInput{Email: "raw-otp@example.com"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if start.DebugOTP == "" {
		t.Fatal("expected DebugOTP in this test's debug-mode setup")
	}

	var otpHash string
	if err := pool.QueryRow(ctx, `SELECT otp_hash FROM vendor_otp_challenges WHERE id = $1`, start.ChallengeID).Scan(&otpHash); err != nil {
		t.Fatalf("query stored challenge: %v", err)
	}
	if otpHash == start.DebugOTP {
		t.Fatal("otp_hash equals the raw code â€” raw OTP was persisted verbatim")
	}
	if strings.Contains(otpHash, start.DebugOTP) {
		t.Fatal("otp_hash contains the raw code as a substring")
	}
	if len(otpHash) < 32 {
		t.Fatalf("otp_hash %q does not look like a real HMAC digest", otpHash)
	}
}

// TestAuth_RawOTPNeverLogged is Section 5 security scenario 2: the raw code
// must never reach the process log, including via the debug/dev delivery
// shortcut â€” DebugOTP travels only in the direct API response, never a log
// line. Captures the standard `log` package's default output (what
// shared/go/httpx and shared/go/logging both write through) across
// Start/Resend/Verify and asserts the raw code appears nowhere in it.
func TestAuth_RawOTPNeverLogged(t *testing.T) {
	pool := testPool(t)
	staff := newFakeStaffResolver()
	auth := setupAuthService(t, pool, staff)
	ctx := context.Background()

	var captured bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(io.MultiWriter(previousOutput, &captured))
	defer log.SetOutput(previousOutput)

	start, err := auth.Start(ctx, vendorauthusecases.StartInput{Email: "logged-otp@example.com"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	resent, err := auth.Resend(ctx, vendorauthusecases.ResendInput{Email: "logged-otp@example.com", ChallengeID: start.ChallengeID})
	if err != nil {
		t.Fatalf("Resend: %v", err)
	}
	users := vendorauthrepositories.NewUserRepository(pool)
	user, _ := users.UpsertByEmail(ctx, "logged-otp@example.com")
	seedStaff(t, pool, staff, user.ID, "staff-log", "vendor-log", true)
	if _, err := auth.Verify(ctx, vendorauthusecases.VerifyInput{Email: "logged-otp@example.com", OTP: resent.DebugOTP, ChallengeID: start.ChallengeID}); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	logged := captured.String()
	if strings.Contains(logged, start.DebugOTP) {
		t.Fatalf("the original OTP %q appeared in log output", start.DebugOTP)
	}
	if strings.Contains(logged, resent.DebugOTP) {
		t.Fatalf("the resent OTP %q appeared in log output", resent.DebugOTP)
	}
}
