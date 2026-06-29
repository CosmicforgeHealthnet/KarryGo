package trip

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

func TestValidateTransitionCoversPhase7CMap(t *testing.T) {
	valid := [][2]TripStatus{
		{StatusAssigned, StatusEnRoutePickup},
		{StatusAssigned, StatusArrivedPickup},
		{StatusAssigned, StatusCancelled},
		{StatusAssigned, StatusFailed},
		{StatusEnRoutePickup, StatusArrivedPickup},
		{StatusArrivedPickup, StatusInProgress},
		{StatusInProgress, StatusProofSubmitted},
		{StatusProofSubmitted, StatusCompleted},
	}
	for _, transition := range valid {
		if err := ValidateTransition(transition[0], transition[1]); err != nil {
			t.Fatalf("%s -> %s rejected: %v", transition[0], transition[1], err)
		}
	}
	for _, transition := range [][2]TripStatus{
		{StatusAssigned, StatusCompleted},
		{StatusCompleted, StatusInProgress},
		{StatusCancelled, StatusAssigned},
		{StatusFailed, StatusAssigned},
		{TripStatus("invalid"), StatusAssigned},
	} {
		err := ValidateTransition(transition[0], transition[1])
		if err == nil {
			t.Fatalf("%s -> %s accepted", transition[0], transition[1])
		}
		appErr := apperrors.From(err)
		if appErr.Status != http.StatusConflict || appErr.Code != "invalid_trip_transition" {
			t.Fatalf("error=%+v", appErr)
		}
		if appErr.Details["from_status"] != transition[0] || appErr.Details["to_status"] != transition[1] {
			t.Fatalf("transition details=%+v", appErr.Details)
		}
	}
}

func TestLocationUpdatedTransitionsAssignedOnce(t *testing.T) {
	repo := newFakeRepository()
	providerID := uuid.NewString()
	trip := Trip{ID: uuid.NewString(), ProviderID: providerID, Status: StatusAssigned}
	repo.trips = append(repo.trips, trip)
	service := NewService(repo, nil)
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	payload, _ := json.Marshal(ProviderLocationUpdatedEvent{
		Event: TopicProviderLocationUpdated, ProviderID: providerID, UpdatedAt: now,
	})

	if err := HandleProviderLocationUpdatedPayload(context.Background(), service, payload); err != nil {
		t.Fatal(err)
	}
	if err := HandleProviderLocationUpdatedPayload(context.Background(), service, payload); err != nil {
		t.Fatal(err)
	}
	if repo.trips[0].Status != StatusEnRoutePickup {
		t.Fatalf("status=%s", repo.trips[0].Status)
	}
	if len(repo.stateLogs) != 1 {
		t.Fatalf("state logs=%d want 1", len(repo.stateLogs))
	}
	log := repo.stateLogs[0]
	if log.FromStatus != string(StatusAssigned) || log.ToStatus != StatusEnRoutePickup ||
		log.ChangedBy != CancelledBySystem || log.Notes == nil || *log.Notes != "auto_started_from_location_update" {
		t.Fatalf("log=%+v", log)
	}
}

func TestLocationUpdatedNoAssignedTripAndBadPayloadDropSafely(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo, nil)
	for _, payload := range [][]byte{
		[]byte("bad-json"),
		[]byte(`{"event":"provider.location_updated"}`),
		[]byte(`{"event":"provider.location_updated","provider_id":"` + uuid.NewString() + `"}`),
	} {
		if err := HandleProviderLocationUpdatedPayload(context.Background(), service, payload); err != nil {
			t.Fatalf("payload returned error: %v", err)
		}
	}
	if len(repo.stateLogs) != 0 {
		t.Fatalf("state logs=%d want 0", len(repo.stateLogs))
	}
}

func TestTripListEndpointFilteringPaginationOrderingAndScope(t *testing.T) {
	env := newTripHTTPTestEnv(t)
	owner, other := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	env.repo.trips = append(env.repo.trips,
		Trip{ID: uuid.NewString(), BookingID: uuid.NewString(), ProviderID: owner, Status: StatusAssigned, CreatedAt: now.Add(-2 * time.Hour), FareAmount: 1000},
		Trip{ID: uuid.NewString(), BookingID: uuid.NewString(), ProviderID: owner, Status: StatusCompleted, CreatedAt: now, FareAmount: 2000},
		Trip{ID: uuid.NewString(), BookingID: uuid.NewString(), ProviderID: other, Status: StatusCompleted, CreatedAt: now.Add(time.Hour), FareAmount: 3000},
	)

	recorder := env.request(t, owner, http.MethodGet, "/api/v1/provider/trips?limit=1&page=1&provider_id="+other)
	assertTripStatus(t, recorder, http.StatusOK)
	data := tripDataMap(t, recorder)
	if int(data["total"].(float64)) != 2 {
		t.Fatalf("total=%v want 2", data["total"])
	}
	trips := data["trips"].([]interface{})
	if len(trips) != 1 || trips[0].(map[string]interface{})["fare_amount"].(float64) != 2000 {
		t.Fatalf("trips=%+v", trips)
	}

	recorder = env.request(t, owner, http.MethodGet, "/api/v1/provider/trips?status=assigned")
	assertTripStatus(t, recorder, http.StatusOK)
	data = tripDataMap(t, recorder)
	if int(data["total"].(float64)) != 1 {
		t.Fatalf("filtered total=%v", data["total"])
	}

	for _, path := range []string{
		"/api/v1/provider/trips?status=unknown",
		"/api/v1/provider/trips?page=zero",
		"/api/v1/provider/trips?limit=51",
	} {
		assertTripStatus(t, env.request(t, owner, http.MethodGet, path), http.StatusBadRequest)
	}

	empty := env.request(t, uuid.NewString(), http.MethodGet, "/api/v1/provider/trips")
	assertTripStatus(t, empty, http.StatusOK)
	if len(tripDataMap(t, empty)["trips"].([]interface{})) != 0 {
		t.Fatal("empty provider list was not empty")
	}
}

func TestActiveTripEndpointStatusAndScope(t *testing.T) {
	for _, status := range []TripStatus{
		StatusAssigned, StatusEnRoutePickup, StatusArrivedPickup, StatusInProgress, StatusProofSubmitted,
	} {
		t.Run(string(status), func(t *testing.T) {
			env := newTripHTTPTestEnv(t)
			owner := uuid.NewString()
			env.repo.trips = append(env.repo.trips, Trip{ID: uuid.NewString(), ProviderID: owner, Status: status})
			assertTripStatus(t, env.request(t, owner, http.MethodGet, "/api/v1/provider/trips/active"), http.StatusOK)
			assertTripStatus(t, env.request(t, uuid.NewString(), http.MethodGet, "/api/v1/provider/trips/active"), http.StatusNotFound)
		})
	}
	for _, status := range []TripStatus{StatusCompleted, StatusCancelled, StatusFailed} {
		t.Run("terminal_"+string(status), func(t *testing.T) {
			env := newTripHTTPTestEnv(t)
			owner := uuid.NewString()
			env.repo.trips = append(env.repo.trips, Trip{ID: uuid.NewString(), ProviderID: owner, Status: status})
			assertTripStatus(t, env.request(t, owner, http.MethodGet, "/api/v1/provider/trips/active"), http.StatusNotFound)
		})
	}
}

func TestTripDetailIncludesOrderedStateLogAndNullableProof(t *testing.T) {
	env := newTripHTTPTestEnv(t)
	owner := uuid.NewString()
	trip := Trip{
		ID: uuid.NewString(), BookingID: uuid.NewString(), ProviderID: owner, Status: StatusCompleted,
		ReceiverPhone: "+2348011223344", CreatedAt: time.Now().UTC(),
	}
	env.repo.trips = append(env.repo.trips, trip)
	early, late := time.Now().UTC().Add(-time.Minute), time.Now().UTC()
	env.repo.stateLogs = append(env.repo.stateLogs,
		TripStateLog{ID: uuid.NewString(), TripID: trip.ID, FromStatus: string(StatusAssigned), ToStatus: StatusEnRoutePickup, ChangedAt: late},
		TripStateLog{ID: uuid.NewString(), TripID: trip.ID, FromStatus: "none", ToStatus: StatusAssigned, ChangedAt: early},
	)

	recorder := env.request(t, owner, http.MethodGet, "/api/v1/provider/trips/"+trip.ID)
	assertTripStatus(t, recorder, http.StatusOK)
	data := tripDataMap(t, recorder)
	logs := data["state_log"].([]interface{})
	if logs[0].(map[string]interface{})["from_status"] != "none" || data["proof"] != nil ||
		data["receiver_phone"] != "+2348011223344" {
		t.Fatalf("detail=%+v", data)
	}

	env.repo.proofs = append(env.repo.proofs, DeliveryProof{
		ID: uuid.NewString(), TripID: trip.ID,
		PhotoRef: "local-private://trips/photo", SignatureRef: "local-private://trips/signature",
	})
	recorder = env.request(t, owner, http.MethodGet, "/api/v1/provider/trips/"+trip.ID)
	assertTripStatus(t, recorder, http.StatusOK)
	proof := tripDataMap(t, recorder)["proof"].(map[string]interface{})
	if proof["photo_ref"] != "local-private://trips/photo" || proof["signature_ref"] != "local-private://trips/signature" {
		t.Fatalf("proof=%+v", proof)
	}

	assertTripStatus(t, env.request(t, uuid.NewString(), http.MethodGet, "/api/v1/provider/trips/"+trip.ID), http.StatusNotFound)
	assertTripStatus(t, env.request(t, owner, http.MethodGet, "/api/v1/provider/trips/not-a-uuid"), http.StatusBadRequest)
}

type tripHTTPTestEnv struct {
	engine *gin.Engine
	tokens *authusecases.TokenUsecase
	repo   *fakeRepository
}

func newTripHTTPTestEnv(t *testing.T) *tripHTTPTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newFakeRepository()
	tokens := authusecases.NewTokenUsecase([]byte("phase7cde-secret"), time.Hour, time.Hour)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	RegisterRoutes(engine, tokens, NewHandler(NewService(repo, nil)))
	return &tripHTTPTestEnv{engine: engine, tokens: tokens, repo: repo}
}

func (e *tripHTTPTestEnv) request(t *testing.T, providerID, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	token, _, err := e.tokens.GenerateAccessToken(providerID, "+2348011223344", uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	e.engine.ServeHTTP(recorder, req)
	return recorder
}

func assertTripStatus(t *testing.T, recorder *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if recorder.Code != expected {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, expected, recorder.Body.String())
	}
}

func tripDataMap(t *testing.T, recorder *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body: %v body=%s", err, recorder.Body.String())
	}
	return envelope.Data
}
