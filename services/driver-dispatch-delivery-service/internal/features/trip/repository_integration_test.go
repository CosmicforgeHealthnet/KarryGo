package trip

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRepositoryTripFoundation(t *testing.T) {
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

	schema := "trip_repository_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	for _, name := range []string{
		"000004_create_providers.up.sql", "000019_create_request_broadcasts.up.sql",
		"000021_create_trips.up.sql", "000022_create_trip_state_log.up.sql",
		"000023_create_delivery_proofs.up.sql", "000024_create_cancellations.up.sql",
	} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}

	providerA, providerB, customerID, bookingID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO providers (id,phone) VALUES ($1,$2),($3,$4)`,
		providerA, "+2348000071001", providerB, "+2348000071002"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO request_broadcasts (booking_id,broadcast_radius_km,expires_at,booking_payload)
		VALUES ($1,5,now()+interval '30 seconds',jsonb_build_object('customer_id',$2::text))
	`, bookingID, customerID); err != nil {
		t.Fatal(err)
	}

	repo := NewPostgresRepository(pool)
	input := CreateTripInput{
		BookingID: bookingID, ProviderID: providerA,
		PickupAddress: "Pickup", PickupLat: 6.5, PickupLng: 3.4,
		DropoffAddress: "Dropoff", DropoffLat: 6.6, DropoffLng: 3.5,
		FareAmount: 150000, Currency: "NGN", ReceiverName: "Receiver",
		ReceiverPhone: "+2348011223344", PackageDesc: "Parcel", PackageWeight: 2.5,
	}
	created, err := repo.CreateTripFromAcceptedRequest(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := repo.CreateTripFromAcceptedRequest(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != duplicate.ID || created.CustomerID != customerID || created.Status != StatusAssigned {
		t.Fatalf("created=%+v duplicate=%+v", created, duplicate)
	}
	if created.PickupAddress != input.PickupAddress || !closeFloat(created.PickupLat, input.PickupLat) ||
		!closeFloat(created.PickupLng, input.PickupLng) || created.DropoffAddress != input.DropoffAddress ||
		!closeFloat(created.DropoffLat, input.DropoffLat) || !closeFloat(created.DropoffLng, input.DropoffLng) ||
		created.FareAmount != input.FareAmount || created.Currency != input.Currency ||
		created.ReceiverName != input.ReceiverName || created.ReceiverPhone != input.ReceiverPhone ||
		created.PackageDesc == nil || *created.PackageDesc != input.PackageDesc ||
		created.PackageWeight == nil || !closeFloat(*created.PackageWeight, input.PackageWeight) {
		var desc string
		if created.PackageDesc != nil {
			desc = *created.PackageDesc
		}
		var weight float64
		if created.PackageWeight != nil {
			weight = *created.PackageWeight
		}
		t.Fatalf("accepted payload mapping incomplete: %+v desc=%q weight=%f", created, desc, weight)
	}
	var trips, logs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM trips WHERE booking_id=$1`, bookingID).Scan(&trips); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM trip_state_log WHERE trip_id=$1`, created.ID).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if trips != 1 || logs != 1 {
		t.Fatalf("trips=%d logs=%d want 1 each", trips, logs)
	}
	if other, err := repo.GetProviderTripByID(ctx, created.ID, providerB); err != nil || other != nil {
		t.Fatalf("cross-provider lookup=%+v err=%v", other, err)
	}
	if active, err := repo.GetProviderActiveTrip(ctx, providerA); err != nil || active == nil || active.ID != created.ID {
		t.Fatalf("active=%+v err=%v", active, err)
	}
	changedAt := time.Now().UTC()
	transition := TransitionTripInput{
		TripID: created.ID, FromStatus: StatusAssigned, ToStatus: StatusEnRoutePickup,
		ChangedBy: CancelledBySystem, Notes: "auto_started_from_location_update", ChangedAt: changedAt,
	}
	if updated, err := repo.TransitionTripStatus(ctx, transition); err != nil || !updated {
		t.Fatalf("first transition updated=%v err=%v", updated, err)
	}
	if updated, err := repo.TransitionTripStatus(ctx, transition); err != nil || updated {
		t.Fatalf("duplicate transition updated=%v err=%v", updated, err)
	}
	stateLogs, err := repo.ListTripStateLog(ctx, created.ID)
	if err != nil || len(stateLogs) != 2 {
		t.Fatalf("state logs=%+v err=%v", stateLogs, err)
	}
	if stateLogs[0].FromStatus != "none" || stateLogs[1].FromStatus != string(StatusAssigned) ||
		stateLogs[1].ToStatus != StatusEnRoutePickup {
		t.Fatalf("state logs out of order or incorrect: %+v", stateLogs)
	}
	if assigned, err := repo.GetAssignedTripForProvider(ctx, providerA); err != nil || assigned != nil {
		t.Fatalf("transitioned trip still assigned=%+v err=%v", assigned, err)
	}
	list, total, err := repo.ListProviderTrips(ctx, providerA, ListTripsOptions{Status: StatusEnRoutePickup, Limit: 20})
	if err != nil || total != 1 || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("filtered list=%+v total=%d err=%v", list, total, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE trips SET status='completed' WHERE id=$1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if active, err := repo.GetProviderActiveTrip(ctx, providerA); err != nil || active != nil {
		t.Fatalf("completed trip returned active=%+v err=%v", active, err)
	}
	if err := repo.InsertStateLog(ctx, StateLogInput{
		TripID: created.ID, FromStatus: string(StatusAssigned), ToStatus: StatusCompleted,
		ChangedBy: CancelledBySystem, Notes: "test",
	}); err != nil {
		t.Fatal(err)
	}
	proofInput := CreateProofInput{
		TripID: created.ID, PhotoRef: "local-private://trips/photo", SignatureRef: "local-private://trips/signature",
		ReceiverName: "Receiver", ReceiverPhone: "+2348011223344",
	}
	if _, err := repo.CreateDeliveryProof(ctx, proofInput); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDeliveryProof(ctx, proofInput); err == nil {
		t.Fatal("duplicate proof accepted")
	}
	cancelInput := CreateCancellationInput{TripID: created.ID, CancelledBy: CancelledBySystem, ReasonCode: "other"}
	if _, err := repo.CreateCancellation(ctx, cancelInput); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateCancellation(ctx, cancelInput); err == nil {
		t.Fatal("duplicate cancellation accepted")
	}
}

func closeFloat(left, right float64) bool {
	return math.Abs(left-right) < 0.000001
}
