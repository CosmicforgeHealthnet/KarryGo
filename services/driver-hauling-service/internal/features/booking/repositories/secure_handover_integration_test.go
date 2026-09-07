package bookingrepo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/database"
	"cosmicforge/logistics/shared/go/handover"
)

func secureHandoverTestRepository(t *testing.T) (*PostgresBookingRepository, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required for secure-handover PostgreSQL race tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, sourceFile, _, _ := runtime.Caller(0)
	serviceRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", ".."))
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(serviceRoot); err != nil {
		t.Fatal(err)
	}
	if err = database.ApplyMigrations(context.Background(), pool); err != nil {
		_ = os.Chdir(previous)
		t.Fatalf("apply migrations: %v", err)
	}
	if err = os.Chdir(previous); err != nil {
		t.Fatal(err)
	}
	return NewPostgresBookingRepository(pool), pool
}

func seedSecureHandoverBooking(t *testing.T, pool *pgxpool.Pool) (bookingID, customerID, providerID string) {
	t.Helper()
	ctx := context.Background()
	bookingID, customerID, providerID = uuid.NewString(), uuid.NewString(), uuid.NewString()
	_, err := pool.Exec(ctx, `INSERT INTO truck_providers(id,phone) VALUES($1,$2)`, providerID, "+390"+providerID[:12])
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO haulage_bookings(id,customer_id,provider_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg,status)
		VALUES($1,$2,$3,'pickup',1,1,'dropoff',2,2,'general',10,'en_route_pickup')`, bookingID, customerID, providerID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM haulage_bookings WHERE id=$1`, bookingID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM truck_providers WHERE id=$1`, providerID)
	})
	return bookingID, customerID, providerID
}

func TestSecureHandoverPostgresOTPVsQROneConsumeAndNoPlaintext(t *testing.T) {
	repo, pool := secureHandoverTestRepository(t)
	bookingID, customerID, providerID := seedSecureHandoverBooking(t, pool)
	secret := []byte("postgres-secure-handover-secret")
	rec, err := handover.NewSecureCodeRecord(secret, "hauling", handover.StepPickup, uuid.NewString(), 1, bookingID, providerID, customerID, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.MarkArrivedAtPickup(context.Background(), bookingID, rec); err != nil {
		t.Fatal(err)
	}
	var pickupPlaintext, deliveryPlaintext string
	if err = pool.QueryRow(context.Background(), `SELECT pickup_code,delivery_code FROM haulage_bookings WHERE id=$1`, bookingID).Scan(&pickupPlaintext, &deliveryPlaintext); err != nil {
		t.Fatal(err)
	}
	if pickupPlaintext != "" || deliveryPlaintext != "" {
		t.Fatal("plaintext handover credential was persisted")
	}
	var storedOTP, storedQR string
	if err = pool.QueryRow(context.Background(), `SELECT otp_hash,qr_token_hash FROM hauling_handover_challenges WHERE challenge_ref=$1`, rec.ChallengeRef).Scan(&storedOTP, &storedQR); err != nil {
		t.Fatal(err)
	}
	if storedOTP == rec.Code || storedQR == rec.QRToken || storedOTP == "" || storedQR == "" {
		t.Fatal("database did not retain hash-only credentials")
	}

	var successes atomic.Int32
	var wg sync.WaitGroup
	for _, credential := range []string{rec.Code, rec.QRToken} {
		credential := credential
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, consumed, consumeErr := repo.ConsumeHandoverChallenge(context.Background(), bookingID, handover.StepPickup, customerID, credential, secret, "", ""); consumeErr == nil && consumed {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("OTP vs QR successful consumers = %d, want 1", successes.Load())
	}
	if _, _, err = repo.ConsumeHandoverChallenge(context.Background(), bookingID, handover.StepPickup, customerID, rec.QRToken, secret, "", ""); err == nil {
		t.Fatal("QR replay unexpectedly succeeded")
	}
}

func TestSecureHandoverPostgresConcurrentOTPHasOneConsumer(t *testing.T) {
	repo, pool := secureHandoverTestRepository(t)
	bookingID, customerID, providerID := seedSecureHandoverBooking(t, pool)
	secret := []byte("postgres-secure-handover-secret")
	rec, err := handover.NewSecureCodeRecord(secret, "hauling", handover.StepPickup, uuid.NewString(), 1, bookingID, providerID, customerID, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.MarkArrivedAtPickup(context.Background(), bookingID, rec); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, consumed, consumeErr := repo.ConsumeHandoverChallenge(context.Background(), bookingID, handover.StepPickup, customerID, rec.Code, secret, "", ""); consumeErr == nil && consumed {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("concurrent OTP successful consumers = %d, want 1", successes.Load())
	}
}

func TestSecureHandoverPostgresResendVsVerifyAndReassignment(t *testing.T) {
	repo, pool := secureHandoverTestRepository(t)
	bookingID, customerID, providerID := seedSecureHandoverBooking(t, pool)
	secret := []byte("postgres-secure-handover-secret")
	first, err := handover.NewSecureCodeRecord(secret, "hauling", handover.StepPickup, uuid.NewString(), 1, bookingID, providerID, customerID, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.MarkArrivedAtPickup(context.Background(), bookingID, first); err != nil {
		t.Fatal(err)
	}
	second, err := handover.NewSecureCodeRecord(secret, "hauling", handover.StepPickup, uuid.NewString(), 2, bookingID, providerID, customerID, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	var winners atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := repo.ResendPickupCode(context.Background(), bookingID, second); err == nil {
			winners.Add(1)
		}
	}()
	go func() {
		defer wg.Done()
		if _, consumed, err := repo.ConsumeHandoverChallenge(context.Background(), bookingID, handover.StepPickup, customerID, first.Code, secret, "", ""); err == nil && consumed {
			winners.Add(1)
		}
	}()
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("resend vs verification winners = %d, want 1", winners.Load())
	}

	// Reassignment commits before verification and invalidates the credential's
	// authoritative provider binding.
	reassignedBooking, reassignedCustomer, oldProvider := seedSecureHandoverBooking(t, pool)
	reassigned, err := handover.NewSecureCodeRecord(secret, "hauling", handover.StepPickup, uuid.NewString(), 1, reassignedBooking, oldProvider, reassignedCustomer, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.MarkArrivedAtPickup(context.Background(), reassignedBooking, reassigned); err != nil {
		t.Fatal(err)
	}
	newProviderID := uuid.NewString()
	_, err = pool.Exec(context.Background(), `INSERT INTO truck_providers(id,phone) VALUES($1,$2)`, newProviderID, "+391"+newProviderID[:12])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM haulage_bookings WHERE id=$1`, reassignedBooking)
		_, _ = pool.Exec(context.Background(), `DELETE FROM truck_providers WHERE id=$1`, newProviderID)
	})
	if _, err = pool.Exec(context.Background(), `UPDATE haulage_bookings SET provider_id=$2 WHERE id=$1`, reassignedBooking, newProviderID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = repo.ConsumeHandoverChallenge(context.Background(), reassignedBooking, handover.StepPickup, reassignedCustomer, reassigned.QRToken, secret, "", ""); err == nil {
		t.Fatal("credential survived provider reassignment")
	}
}
