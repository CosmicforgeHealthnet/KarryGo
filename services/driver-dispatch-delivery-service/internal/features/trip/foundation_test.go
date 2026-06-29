package trip

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/httpx"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

func TestTripFeatureFilesExist(t *testing.T) {
	for _, name := range []string{"handler.go", "service.go", "repository.go", "model.go", "subscribers.go"} {
		if _, err := os.Stat(filepath.Join(".", name)); err != nil {
			t.Fatalf("trip feature file %s is missing: %v", name, err)
		}
	}
}

func TestTripStateMachine(t *testing.T) {
	valid := [][2]TripStatus{
		{StatusAssigned, StatusEnRoutePickup},
		{StatusEnRoutePickup, StatusArrivedPickup},
		{StatusArrivedPickup, StatusInProgress},
		{StatusInProgress, StatusProofSubmitted},
		{StatusProofSubmitted, StatusCompleted},
	}
	for _, transition := range valid {
		if !CanTransition(transition[0], transition[1]) {
			t.Fatalf("expected transition %s -> %s", transition[0], transition[1])
		}
	}
	if CanTransition(StatusCompleted, StatusAssigned) || CanTransition(StatusAssigned, StatusCompleted) {
		t.Fatal("invalid transition accepted")
	}
	for _, status := range []TripStatus{StatusAssigned, StatusEnRoutePickup, StatusArrivedPickup, StatusInProgress} {
		if !CanProviderCancel(status) {
			t.Fatalf("provider should be able to cancel %s", status)
		}
	}
	for _, status := range []TripStatus{StatusProofSubmitted, StatusCompleted, StatusCancelled, StatusFailed} {
		if CanProviderCancel(status) {
			t.Fatalf("provider should not be able to cancel %s", status)
		}
	}
}

func TestAllProviderTripRoutesRequireJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	tokens := authusecases.NewTokenUsecase([]byte("trip-test-secret"), time.Hour, time.Hour)
	RegisterRoutes(engine, tokens, NewHandler(NewService(newFakeRepository(), nil)))

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/provider/trips"},
		{http.MethodGet, "/api/v1/provider/trips/active"},
		{http.MethodGet, "/api/v1/provider/trips/" + uuid.NewString()},
		{http.MethodPost, "/api/v1/provider/trips/" + uuid.NewString() + "/arrived"},
		{http.MethodPost, "/api/v1/provider/trips/" + uuid.NewString() + "/start"},
		{http.MethodPost, "/api/v1/provider/trips/" + uuid.NewString() + "/proof"},
		{http.MethodGet, "/api/v1/provider/trips/" + uuid.NewString() + "/proof"},
		{http.MethodPost, "/api/v1/provider/trips/" + uuid.NewString() + "/complete"},
		{http.MethodPost, "/api/v1/provider/trips/" + uuid.NewString() + "/cancel"},
	} {
		req := httptest.NewRequest(route.method, route.path, nil)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status=%d want 401", route.method, route.path, recorder.Code)
		}
	}
}

func TestRequestAcceptedSubscriberCreatesTripIdempotently(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo, nil)
	event := validAcceptedEvent()
	payload, _ := json.Marshal(event)

	if err := HandleRequestAcceptedPayload(context.Background(), service, payload); err != nil {
		t.Fatal(err)
	}
	if err := HandleRequestAcceptedPayload(context.Background(), service, payload); err != nil {
		t.Fatal(err)
	}
	if len(repo.trips) != 1 {
		t.Fatalf("trips=%d want 1", len(repo.trips))
	}
	if repo.initialLogs != 1 {
		t.Fatalf("initial state logs=%d want 1", repo.initialLogs)
	}
	if repo.trips[0].Status != StatusAssigned {
		t.Fatalf("status=%s want assigned", repo.trips[0].Status)
	}
}

func TestRequestAcceptedSubscriberDropsBadPayloads(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo, nil)
	for _, payload := range [][]byte{
		[]byte("not-json"),
		[]byte(`{"event":"request.accepted"}`),
		[]byte(`{"event":"different.event"}`),
	} {
		if err := HandleRequestAcceptedPayload(context.Background(), service, payload); err != nil {
			t.Fatalf("payload should be dropped safely: %v", err)
		}
	}
	if len(repo.trips) != 0 {
		t.Fatalf("bad payloads created %d trips", len(repo.trips))
	}
}

func TestProviderScopedTripLookup(t *testing.T) {
	repo := newFakeRepository()
	owner := uuid.NewString()
	trip := Trip{ID: uuid.NewString(), BookingID: uuid.NewString(), ProviderID: owner, Status: StatusAssigned}
	repo.trips = append(repo.trips, trip)
	service := NewService(repo, nil)

	if _, err := service.GetProviderTrip(context.Background(), trip.ID, uuid.NewString()); err == nil {
		t.Fatal("cross-provider lookup should return not found")
	}
	result, err := service.GetProviderTrip(context.Background(), trip.ID, owner)
	if err != nil || result.ID != trip.ID {
		t.Fatalf("owner lookup result=%+v err=%v", result, err)
	}
}

func TestLocalProofStorageReturnsPrivateReference(t *testing.T) {
	storage := NewLocalProofStorage(t.TempDir(), "local-private://")
	file, err := os.CreateTemp(t.TempDir(), "proof-*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content := []byte("image-content")
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	header := &multipart.FileHeader{
		Filename: "delivered package.png",
		Size:     int64(len(content)),
		Header:   textproto.MIMEHeader{"Content-Type": []string{"image/png"}},
	}
	ref, err := storage.SaveProofFile(context.Background(), uuid.NewString(), uuid.NewString(), file, header, "photo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref, "local-private://trips/") {
		t.Fatalf("reference=%q", ref)
	}
	if strings.Contains(ref, storage.rootDir) || filepath.IsAbs(ref) {
		t.Fatalf("raw filesystem path exposed: %q", ref)
	}
}

func TestTripProductionFilesDoNotContainForbiddenInfrastructureTerms(t *testing.T) {
	forbidden := []string{"aws-sdk-go", "github.com/aws/", "s3.amazonaws", "sns.amazonaws", "sqs.amazonaws"}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(data))
		for _, term := range forbidden {
			if strings.Contains(lower, term) {
				t.Fatalf("forbidden infrastructure term %q found in %s", term, entry.Name())
			}
		}
	}
}

func validAcceptedEvent() RequestAcceptedEvent {
	return RequestAcceptedEvent{
		Event: TopicRequestAccepted, BookingID: uuid.NewString(), ProviderID: uuid.NewString(),
		FareAmount: 150000, Currency: "NGN",
		PickupLat: 6.52, PickupLng: 3.37, PickupAddress: "Pickup",
		DropoffLat: 6.45, DropoffLng: 3.40, DropoffAddress: "Dropoff",
		ReceiverName: "Receiver", ReceiverPhone: "+2348011223344",
		PackageDesc: "Parcel", PackageWeight: 2.5, OccurredAt: time.Now().UTC(),
	}
}

type fakeRepository struct {
	trips           []Trip
	stateLogs       []TripStateLog
	proofs          []DeliveryProof
	cancels         []Cancellation
	customerRatings map[string]CustomerRating
	initialLogs     int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{customerRatings: make(map[string]CustomerRating)}
}

func (r *fakeRepository) CreateTripFromAcceptedRequest(_ context.Context, input CreateTripInput) (*Trip, error) {
	for i := range r.trips {
		if r.trips[i].BookingID == input.BookingID {
			return &r.trips[i], nil
		}
	}
	trip := Trip{
		ID: uuid.NewString(), BookingID: input.BookingID, ProviderID: input.ProviderID, Status: StatusAssigned,
		PickupAddress: input.PickupAddress, DropoffAddress: input.DropoffAddress,
		ReceiverName: input.ReceiverName, ReceiverPhone: input.ReceiverPhone,
	}
	r.trips = append(r.trips, trip)
	r.initialLogs++
	notes := "created_from_request_accepted"
	r.stateLogs = append(r.stateLogs, TripStateLog{
		ID: uuid.NewString(), TripID: trip.ID, FromStatus: "none", ToStatus: StatusAssigned,
		ChangedAt: trip.CreatedAt, ChangedBy: CancelledBySystem, Notes: &notes,
	})
	return &r.trips[len(r.trips)-1], nil
}

func (r *fakeRepository) GetTripByID(_ context.Context, tripID string) (*Trip, error) {
	for i := range r.trips {
		if r.trips[i].ID == tripID {
			return &r.trips[i], nil
		}
	}
	return nil, nil
}

func (r *fakeRepository) GetTripByBookingID(_ context.Context, bookingID string) (*Trip, error) {
	for i := range r.trips {
		if r.trips[i].BookingID == bookingID {
			return &r.trips[i], nil
		}
	}
	return nil, nil
}

func (r *fakeRepository) ListProviderTrips(_ context.Context, providerID string, options ListTripsOptions) ([]Trip, int, error) {
	result := []Trip{}
	for _, trip := range r.trips {
		if trip.ProviderID == providerID && (options.Status == "" || trip.Status == options.Status) {
			result = append(result, trip)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	total := len(result)
	start := options.Offset
	if start > total {
		start = total
	}
	end := start + options.Limit
	if end > total {
		end = total
	}
	return result[start:end], total, nil
}

func (r *fakeRepository) GetProviderActiveTrip(_ context.Context, providerID string) (*Trip, error) {
	for i := range r.trips {
		if r.trips[i].ProviderID == providerID && r.trips[i].Status != StatusCompleted &&
			r.trips[i].Status != StatusCancelled && r.trips[i].Status != StatusFailed {
			return &r.trips[i], nil
		}
	}
	return nil, nil
}

func (r *fakeRepository) GetProviderTripByID(_ context.Context, tripID, providerID string) (*Trip, error) {
	for i := range r.trips {
		if r.trips[i].ID == tripID && r.trips[i].ProviderID == providerID {
			return &r.trips[i], nil
		}
	}
	return nil, nil
}

func (r *fakeRepository) GetAssignedTripForProvider(_ context.Context, providerID string) (*Trip, error) {
	for i := range r.trips {
		if r.trips[i].ProviderID == providerID && r.trips[i].Status == StatusAssigned {
			return &r.trips[i], nil
		}
	}
	return nil, nil
}

func (r *fakeRepository) TransitionTripStatus(_ context.Context, input TransitionTripInput) (bool, error) {
	for i := range r.trips {
		if r.trips[i].ID == input.TripID && r.trips[i].Status == input.FromStatus {
			r.trips[i].Status = input.ToStatus
			r.trips[i].UpdatedAt = input.ChangedAt
			notes := input.Notes
			r.stateLogs = append(r.stateLogs, TripStateLog{
				ID: uuid.NewString(), TripID: input.TripID, FromStatus: string(input.FromStatus),
				ToStatus: input.ToStatus, ChangedAt: input.ChangedAt, ChangedBy: input.ChangedBy, Notes: &notes,
			})
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeRepository) InsertStateLog(_ context.Context, input StateLogInput) error {
	notes := input.Notes
	r.stateLogs = append(r.stateLogs, TripStateLog{
		ID: uuid.NewString(), TripID: input.TripID, FromStatus: input.FromStatus,
		ToStatus: input.ToStatus, ChangedBy: input.ChangedBy, Notes: &notes,
	})
	return nil
}

func (r *fakeRepository) ListTripStateLog(_ context.Context, tripID string) ([]TripStateLog, error) {
	result := []TripStateLog{}
	for _, item := range r.stateLogs {
		if item.TripID == tripID {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ChangedAt.Before(result[j].ChangedAt) })
	return result, nil
}

func (r *fakeRepository) CreateDeliveryProof(_ context.Context, input CreateProofInput) (*DeliveryProof, error) {
	proof := DeliveryProof{
		ID: uuid.NewString(), TripID: input.TripID, PhotoRef: input.PhotoRef,
		SignatureRef: input.SignatureRef, ReceiverName: input.ReceiverName, ReceiverPhone: input.ReceiverPhone,
	}
	r.proofs = append(r.proofs, proof)
	return &r.proofs[len(r.proofs)-1], nil
}
func (r *fakeRepository) GetDeliveryProofByTripID(_ context.Context, tripID string) (*DeliveryProof, error) {
	for i := range r.proofs {
		if r.proofs[i].TripID == tripID {
			return &r.proofs[i], nil
		}
	}
	return nil, nil
}
func (r *fakeRepository) CreateCancellation(_ context.Context, input CreateCancellationInput) (*Cancellation, error) {
	cancellation := Cancellation{ID: uuid.NewString(), TripID: input.TripID}
	r.cancels = append(r.cancels, cancellation)
	return &r.cancels[len(r.cancels)-1], nil
}
