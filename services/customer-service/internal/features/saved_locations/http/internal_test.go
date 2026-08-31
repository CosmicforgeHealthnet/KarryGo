package savedlocationhttp

import (
	"bytes"
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	savedlocationmodels "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/models"
	savedlocationusecases "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/usecases"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type internalSavedLocationRepository struct {
	locations []savedlocationmodels.SavedLocation
}

func (r *internalSavedLocationRepository) List(
	ctx context.Context,
	customerID string,
) ([]savedlocationmodels.SavedLocation, error) {
	out := make([]savedlocationmodels.SavedLocation, 0)

	for _, location := range r.locations {
		if location.CustomerID == customerID {
			out = append(out, location)
		}
	}

	return out, nil
}

func (r *internalSavedLocationRepository) Create(
	ctx context.Context,
	location savedlocationmodels.SavedLocation,
) (savedlocationmodels.SavedLocation, error) {
	return location, nil
}

func (r *internalSavedLocationRepository) Update(
	ctx context.Context,
	location savedlocationmodels.SavedLocation,
) (savedlocationmodels.SavedLocation, error) {
	return location, nil
}

func (r *internalSavedLocationRepository) Delete(
	ctx context.Context,
	id string,
	customerID string,
) error {
	return nil
}

func buildInternalSavedLocationRouter(
	t *testing.T,
) (*gin.Engine, []byte) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	secret := []byte("vendor-customer-address-test-secret")

	repo := &internalSavedLocationRepository{
		locations: []savedlocationmodels.SavedLocation{
			{
				ID:          "loc-a",
				CustomerID:  "customer-a",
				Title:       "Home",
				Address:     "23 Akeju Street, Lagos",
				Latitude:    6.4301,
				Longitude:   3.4201,
				Description: "Gate beside pharmacy",
			},
			{
				ID:         "loc-b",
				CustomerID: "customer-b",
				Title:      "Office",
				Address:    "10 Marina Road, Lagos",
				Latitude:   6.4510,
				Longitude:  3.3900,
			},
		},
	}

	service := savedlocationusecases.NewSavedLocationService(repo)

	router := gin.New()

	group := router.Group("/api/v1/customer")

	RegisterInternalRoutes(
		group,
		service,
		serviceauth.NewVerifier(
			serviceauth.ParseSecrets(
				"vendor-service=vendor-customer-address-test-secret",
			),
			5*time.Minute,
		),
	)

	return router, secret
}

func signedInternalSavedLocationRequest(
	t *testing.T,
	secret []byte,
	path string,
) *nethttp.Request {
	t.Helper()

	req := httptest.NewRequest(
		nethttp.MethodGet,
		path,
		bytes.NewReader(nil),
	)

	if err := serviceauth.SignRequest(
		req,
		"vendor-service",
		secret,
		nil,
		time.Now(),
	); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	return req
}

func TestInternalSavedLocationRequiresHMAC(t *testing.T) {
	router, _ := buildInternalSavedLocationRouter(t)

	req := httptest.NewRequest(
		nethttp.MethodGet,
		"/api/v1/customer/internal/customers/customer-a/saved-locations/loc-a",
		nil,
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf(
			"status=%d want=%d body=%s",
			rec.Code,
			nethttp.StatusUnauthorized,
			rec.Body.String(),
		)
	}
}

func TestInternalSavedLocationReturnsOwnedAddress(t *testing.T) {
	router, secret := buildInternalSavedLocationRouter(t)

	req := signedInternalSavedLocationRequest(
		t,
		secret,
		"/api/v1/customer/internal/customers/customer-a/saved-locations/loc-a",
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf(
			"status=%d want=%d body=%s",
			rec.Code,
			nethttp.StatusOK,
			rec.Body.String(),
		)
	}

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			ID          string  `json:"id"`
			Title       string  `json:"title"`
			Address     string  `json:"address"`
			Latitude    float64 `json:"latitude"`
			Longitude   float64 `json:"longitude"`
			Description string  `json:"description"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !envelope.Success {
		t.Fatal("expected success=true")
	}

	if envelope.Data.ID != "loc-a" ||
		envelope.Data.Title != "Home" ||
		envelope.Data.Address != "23 Akeju Street, Lagos" {
		t.Fatalf("unexpected address response: %+v", envelope.Data)
	}
}

func TestInternalSavedLocationBlocksCrossCustomerIDOR(t *testing.T) {
	router, secret := buildInternalSavedLocationRouter(t)

	req := signedInternalSavedLocationRequest(
		t,
		secret,
		"/api/v1/customer/internal/customers/customer-a/saved-locations/loc-b",
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusNotFound {
		t.Fatalf(
			"cross-customer lookup status=%d want=%d body=%s",
			rec.Code,
			nethttp.StatusNotFound,
			rec.Body.String(),
		)
	}
}

func TestInternalSavedLocationRejectsUnknownCaller(t *testing.T) {
	router, _ := buildInternalSavedLocationRouter(t)

	req := httptest.NewRequest(
		nethttp.MethodGet,
		"/api/v1/customer/internal/customers/customer-a/saved-locations/loc-a",
		nil,
	)

	if err := serviceauth.SignRequest(
		req,
		"unknown-service",
		[]byte("unknown-secret"),
		nil,
		time.Now(),
	); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf(
			"status=%d want=%d body=%s",
			rec.Code,
			nethttp.StatusForbidden,
			rec.Body.String(),
		)
	}
}
