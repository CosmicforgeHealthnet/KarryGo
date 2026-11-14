package trip

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/providercompat"
)

type capturedCompletedWork struct {
	requests []providercompat.CompletedWorkRequest
}

func (c *capturedCompletedWork) EnqueueCompletedWork(_ context.Context, _ pgx.Tx, request providercompat.CompletedWorkRequest) error {
	c.requests = append(c.requests, request)
	return nil
}

// FLEET-13.11. Dispatch takes the customer's payment per BOOKING
// (booking/payments.go: source "dispatch-delivery-service", reference = the
// booking id) but completes and reports work per TRIP. A managed trip's
// completed-work record must therefore carry the booking as its payment
// reference, or Wallet cannot find the money Fleet's settlement releases.
func TestManagedTripCompletionReportsTheBookingAsItsPaymentReference(t *testing.T) {
	databaseURL := os.Getenv("DISPATCH_TRIP_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_TRIP_TEST_DATABASE_URL or TEST_DATABASE_URL to run trip repository integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "trip_payment_ref_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE") //nolint:errcheck
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	entries, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	for _, path := range entries {
		sql, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(path), err)
		}
	}

	captured := &capturedCompletedWork{}
	repo := NewPostgresRepository(pool).WithCompletedWorkEnqueuer(captured)
	complete := func(managed bool) (tripID, bookingID string) {
		t.Helper()
		providerID, customerID := uuid.NewString(), uuid.NewString()
		bookingID = uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO providers (id,phone) VALUES ($1,$2)`,
			providerID, "+23480"+strings.ReplaceAll(uuid.NewString(), "-", "")[:8]); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO request_broadcasts (booking_id,broadcast_radius_km,expires_at,booking_payload)
			VALUES ($1,5,now()+interval '30 seconds',jsonb_build_object('customer_id',$2::text))`,
			bookingID, customerID); err != nil {
			t.Fatal(err)
		}
		created, err := repo.CreateTripFromAcceptedRequest(ctx, CreateTripInput{
			BookingID: bookingID, ProviderID: providerID,
			PickupAddress: "Pickup", PickupLat: 6.5, PickupLng: 3.4,
			DropoffAddress: "Dropoff", DropoffLat: 6.6, DropoffLng: 3.5,
			FareAmount: 150000, Currency: "NGN", ReceiverName: "Receiver",
			ReceiverPhone: "+2348011223344", PackageDesc: "Parcel", PackageWeight: 2.5,
		})
		if err != nil {
			t.Fatal(err)
		}
		if managed {
			if _, err := pool.Exec(ctx, `
				UPDATE trips SET execution_mode='ORGANIZATION_MANAGED', organization_id='org_1',
				       settlement_organization_id='org_1', organization_membership_id='mem_1',
				       fleet_assignment_id=$2, provider_response_id='resp_1',
				       acceptance_source='ORGANIZATION_DISPATCHER'
				WHERE id=$1`, created.ID, "asg_"+created.ID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pool.Exec(ctx, `UPDATE trips SET status='proof_submitted' WHERE id=$1`, created.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.CreateDeliveryProof(ctx, CreateProofInput{
			TripID: created.ID, PhotoRef: "local-private://trips/photo", SignatureRef: "local-private://trips/signature",
			ReceiverName: "Receiver", ReceiverPhone: "+2348011223344",
		}); err != nil {
			t.Fatal(err)
		}
		if trip, _, err := repo.CompleteTripTx(ctx, CompleteTripInput{TripID: created.ID, ProviderID: providerID, Now: time.Now().UTC()}); err != nil || trip == nil {
			t.Fatalf("complete trip=%v err=%v", trip, err)
		}
		return created.ID, bookingID
	}

	tripID, bookingID := complete(true)
	if len(captured.requests) != 1 {
		t.Fatalf("managed completion enqueued %d completed-work records, want 1", len(captured.requests))
	}
	request := captured.requests[0]
	if request.SourceService != providercompat.SourceServiceDispatch || request.SourceJobID != tripID {
		t.Fatalf("settlement identity changed: %+v", request)
	}
	if request.PaymentSourceReference != bookingID || request.PaymentSourceReference == request.SourceJobID {
		t.Fatalf("payment reference=%q, want the booking %q (trip %q)", request.PaymentSourceReference, bookingID, tripID)
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("completed-work record does not validate: %v", err)
	}

	// An independent trip reports nothing, so it carries no reference anywhere.
	complete(false)
	if len(captured.requests) != 1 {
		t.Fatalf("independent completion enqueued completed work: %d records", len(captured.requests))
	}
}
