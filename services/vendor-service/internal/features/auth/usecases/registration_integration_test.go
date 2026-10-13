package vendorauthusecases_test

import (
	"context"
	repo "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
	auth "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/vendor-service/internal/features/registration"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"testing"
	"time"
)

type registrationBridge struct {
	pool    *pgxpool.Pool
	service *registration.Service
	staff   *fakeStaffResolver
	calls   int
}

func (b *registrationBridge) RegisterOwner(ctx context.Context, userID string, in auth.RegistrationInput) error {
	b.calls++
	if err := b.service.RegisterOwner(ctx, userID, in); err != nil {
		return err
	}
	var id, vendor, status string
	if err := b.pool.QueryRow(ctx, `SELECT id,vendor_id,status FROM vendor_staff WHERE user_id=$1 LIMIT 1`, userID).Scan(&id, &vendor, &status); err != nil {
		return err
	}
	b.staff.mu.Lock()
	_, found := b.staff.byStaffID[id]
	b.staff.mu.Unlock()
	if !found {
		b.staff.addStaff(userID, id, vendor, status == "active")
	}
	return nil
}

func TestSelfRegistrationRequiresOTPAndCreatesOnlyDraftOwner(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	staff := newFakeStaffResolver()
	bridge := &registrationBridge{pool: pool, service: registration.NewService(pool, true), staff: staff}
	service := auth.NewAuthService(auth.Options{
		Users: repo.NewUserRepository(pool), Challenges: repo.NewOTPChallengeRepository(pool), Sessions: repo.NewSessionRepository(pool), StepUps: repo.NewStepUpRepository(pool), Staff: staff, Registration: bridge,
		AccessTokenSecret: []byte("test-access-secret"), RefreshTokenSecret: []byte("test-refresh-secret"), OTPSecret: []byte("test-otp-secret"), AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 24 * time.Hour, OTPTTL: 5 * time.Minute, OTPMaxAttempts: 5, OTPMaxResends: 3, OTPDebug: true,
	})
	email := uuid.NewString() + "@example.com"
	start, err := service.Start(ctx, auth.StartInput{Email: email})
	if err != nil {
		t.Fatal(err)
	}
	input := auth.RegistrationInput{LegalName: "Registration test business", Currency: "NGN", Timezone: "UTC"}
	wrong := "000000"
	if start.DebugOTP == wrong {
		wrong = "111111"
	}
	if _, err = service.Register(ctx, auth.VerifyInput{Email: email, OTP: wrong, ChallengeID: start.ChallengeID}, input); err == nil {
		t.Fatal("invalid OTP registered an owner")
	}
	if bridge.calls != 0 {
		t.Fatal("registration ran before OTP verification")
	}
	result, err := service.Register(ctx, auth.VerifyInput{Email: email, OTP: start.DebugOTP, ChallengeID: start.ChallengeID}, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tokens == nil {
		t.Fatal("missing authenticated session")
	}
	var status string
	var owners int
	if err = pool.QueryRow(ctx, `SELECT status FROM vendor_businesses WHERE id=$1`, result.Tokens.VendorID).Scan(&status); err != nil || status != "DRAFT" {
		t.Fatalf("business status=%s err=%v", status, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_staff_role_assignments WHERE staff_id=$1 AND role_id='role_owner' AND branch_id IS NULL AND revoked_at IS NULL`, result.Tokens.StaffID).Scan(&owners); err != nil || owners != 1 {
		t.Fatalf("owner grants=%d err=%v", owners, err)
	}
	if _, err = service.Register(ctx, auth.VerifyInput{Email: email, OTP: start.DebugOTP, ChallengeID: start.ChallengeID}, input); err == nil {
		t.Fatal("consumed OTP reused")
	}
	start, err = service.Start(ctx, auth.StartInput{Email: email})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := service.Register(ctx, auth.VerifyInput{Email: email, OTP: start.DebugOTP, ChallengeID: start.ChallengeID}, input)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Tokens.VendorID != result.Tokens.VendorID {
		t.Fatal("retry created another business")
	}
}

func TestSelfRegistrationConcurrentRetriesAndSuspension(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	user, err := repo.NewUserRepository(pool).UpsertByEmail(ctx, uuid.NewString()+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	service := registration.NewService(pool, true)
	input := auth.RegistrationInput{LegalName: "Concurrent registration", Currency: "NGN", Timezone: "UTC"}
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- service.RegisterOwner(ctx, user.ID, input) }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_staff WHERE user_id=$1`, user.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal(fmt.Sprintf("staff count=%d err=%v", count, err))
	}
	if _, err = pool.Exec(ctx, `UPDATE vendor_staff SET status='suspended' WHERE user_id=$1`, user.ID); err != nil {
		t.Fatal(err)
	}
	if err = service.RegisterOwner(ctx, user.ID, input); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM vendor_staff WHERE user_id=$1`, user.ID).Scan(&status); err != nil || status != "suspended" {
		t.Fatal("registration bypassed suspension")
	}
}
