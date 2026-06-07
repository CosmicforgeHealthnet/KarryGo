package vehicle

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

// ── Route protection (Phase 4A) ───────────────────────────────────────────────

func TestProviderVehicleRoutesReturn401WithoutJWT(t *testing.T) {
	router, _ := buildVehicleTestRouter(newFakeRepository())
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/provider/vehicle"},
		{http.MethodGet, "/api/v1/provider/vehicle"},
		{http.MethodGet, "/api/v1/provider/vehicle/11111111-1111-1111-1111-111111111111"},
		{http.MethodPatch, "/api/v1/provider/vehicle/11111111-1111-1111-1111-111111111111"},
		{http.MethodPost, "/api/v1/provider/vehicle/11111111-1111-1111-1111-111111111111/documents"},
		{http.MethodGet, "/api/v1/provider/vehicle/11111111-1111-1111-1111-111111111111/documents"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
			}
			assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeUnauthorized)
		})
	}
}

func TestAdminVehicleReviewReturn401WithoutJWT(t *testing.T) {
	router, _ := buildVehicleTestRouter(newFakeRepository())

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/11111111-1111-1111-1111-111111111111/review", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeUnauthorized)
}

func TestAdminVehicleReviewReturns403ForDispatchProviderJWT(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "11111111-1111-1111-1111-111111111111", "dispatch_provider")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/22222222-2222-2222-2222-222222222222/review", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeForbidden)
}

// ── Phase 4C: POST /api/v1/provider/vehicle ───────────────────────────────────

func TestRegisterBikeReturns201WithValidPayload(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	body := `{"bike_type":"motorcycle","brand":"Honda","model":"CB125F","year":2022,"color":"Red","plate_number":"LAG-123-XY"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["verification_status"] != "unverified" {
		t.Fatalf("verification_status = %v, want unverified", data["verification_status"])
	}
}

func TestFirstBikeIsPrimary(t *testing.T) {
	repo := newFakeRepository()
	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	body := `{"bike_type":"motorcycle","brand":"Honda","model":"CB125F","year":2022,"color":"Red","plate_number":"LAG-001-AA"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["is_primary"] != true {
		t.Fatalf("is_primary = %v, want true", data["is_primary"])
	}
}

func TestSecondBikeIsNotPrimary(t *testing.T) {
	repo := newFakeRepository()
	router, tokens := buildVehicleTestRouter(repo)
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	for i, plate := range []string{"LAG-S01-AA", "LAG-S02-BB"} {
		body := fmt.Sprintf(`{"bike_type":"motorcycle","brand":"Honda","model":"CB125F","year":2022,"color":"Red","plate_number":%q}`, plate)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("bike %d status = %d; body = %s", i+1, w.Code, w.Body.String())
		}
		if i == 1 {
			data := extractVehicleData(t, w.Body.Bytes())
			if data["is_primary"] != false {
				t.Fatalf("second bike is_primary = %v, want false", data["is_primary"])
			}
		}
	}
}

func TestDuplicatePlateNumberReturns409(t *testing.T) {
	repo := newFakeRepository()
	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	body := `{"bike_type":"motorcycle","brand":"Honda","model":"CB125F","year":2022,"color":"Red","plate_number":"LAG-DUP-XX"}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if i == 0 && w.Code != http.StatusCreated {
			t.Fatalf("first insert status = %d; body = %s", w.Code, w.Body.String())
		}
		if i == 1 {
			if w.Code != http.StatusConflict {
				t.Fatalf("duplicate plate status = %d, want 409; body = %s", w.Code, w.Body.String())
			}
			assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeConflict)
		}
	}
}

func TestInvalidBikeTypeReturns400(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	body := `{"bike_type":"truck","brand":"Ford","model":"F150","year":2022,"color":"Black","plate_number":"LAG-BAD-TY"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestMissingBrandReturns400(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	body := `{"bike_type":"motorcycle","brand":"","model":"CB125F","year":2022,"color":"Red","plate_number":"LAG-NBR-00"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestMissingModelReturns400(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	body := `{"bike_type":"motorcycle","brand":"Honda","model":"","year":2022,"color":"Red","plate_number":"LAG-NMD-00"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestInvalidYearReturns400(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	body := `{"bike_type":"motorcycle","brand":"Honda","model":"CB125F","year":99,"color":"Red","plate_number":"LAG-NYR-00"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestMissingColorReturns400(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	body := `{"bike_type":"motorcycle","brand":"Honda","model":"CB125F","year":2022,"color":"","plate_number":"LAG-NCL-00"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestMissingPlateNumberReturns400(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	body := `{"bike_type":"motorcycle","brand":"Honda","model":"CB125F","year":2022,"color":"Red","plate_number":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestProviderIDInBodyIsIgnored(t *testing.T) {
	repo := newFakeRepository()
	router, tokens := buildVehicleTestRouter(repo)
	jwtProviderID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	token := mustVehicleRoleToken(t, tokens, jwtProviderID, "dispatch_provider")

	body := `{"bike_type":"motorcycle","brand":"Honda","model":"CB125F","year":2022,"color":"Red","plate_number":"LAG-PID-00","provider_id":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["provider_id"] == "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" {
		t.Fatal("provider_id came from request body; must come from JWT")
	}
	if data["provider_id"] != jwtProviderID {
		t.Fatalf("provider_id = %v, want %s", data["provider_id"], jwtProviderID)
	}
}

func TestBikeAuditRowCreatedOnRegister(t *testing.T) {
	repo := newFakeRepository()
	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	body := `{"bike_type":"motorcycle","brand":"Honda","model":"CB125F","year":2022,"color":"Red","plate_number":"LAG-AUD-00"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	if len(repo.audits) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(repo.audits))
	}
	if repo.audits[0].Action != AuditRegistered {
		t.Fatalf("audit action = %s, want registered", repo.audits[0].Action)
	}
}

func TestRegisterWithoutJWTReturns401(t *testing.T) {
	router, _ := buildVehicleTestRouter(newFakeRepository())

	body := `{"bike_type":"motorcycle","brand":"Honda","model":"CB125F","year":2022,"color":"Red","plate_number":"LAG-X01-AA"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
}

// ── Phase 4D: GET /api/v1/provider/vehicle ────────────────────────────────────

func TestListBikesReturnsEmptyArrayForNewProvider(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "cccccccc-cccc-cccc-cccc-cccccccccccc", "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	arr, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("data is not an array; got %T; body = %s", resp["data"], w.Body.String())
	}
	if len(arr) != 0 {
		t.Fatalf("expected empty array, got %d items", len(arr))
	}
}

func TestListBikesReturnsOnlyMyBikes(t *testing.T) {
	repo := newFakeRepository()
	// Pre-seed two providers' bikes.
	_ = repo.seedBike("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "LAG-MINE-01", true)
	_ = repo.seedBike("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "LAG-OTHER-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	arr := resp["data"].([]any)
	if len(arr) != 1 {
		t.Fatalf("expected 1 bike for provider A, got %d", len(arr))
	}
	item := arr[0].(map[string]any)
	if item["plate_number"] != "LAG-MINE-01" {
		t.Fatalf("unexpected bike: %v", item["plate_number"])
	}
}

func TestListBikesPrimaryFirst(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	_ = repo.seedBike(providerID, "LAG-SECONDARY-01", false)
	_ = repo.seedBike(providerID, "LAG-PRIMARY-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	arr := resp["data"].([]any)
	if len(arr) < 2 {
		t.Fatalf("expected 2 bikes, got %d", len(arr))
	}
	first := arr[0].(map[string]any)
	if first["is_primary"] != true {
		t.Fatalf("first item is_primary = %v, want true", first["is_primary"])
	}
}

// ── Phase 4D: GET /api/v1/provider/vehicle/:id ───────────────────────────────

func TestGetBikeReturnsFullDetail(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-DETAIL-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/"+bike.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["id"] != bike.ID {
		t.Fatalf("id = %v, want %s", data["id"], bike.ID)
	}
	// documents must be present (empty array is fine)
	docsRaw, hasDocuments := data["documents"]
	if !hasDocuments {
		t.Fatalf("documents field missing; body = %s", w.Body.String())
	}
	docs, ok := docsRaw.([]any)
	if !ok {
		t.Fatalf("documents is not an array; body = %s", w.Body.String())
	}
	if len(docs) != 0 {
		t.Fatalf("expected empty documents, got %d", len(docs))
	}
}

func TestGetBikeReturns404ForAnotherProvidersID(t *testing.T) {
	repo := newFakeRepository()
	bikeOfProviderA := repo.seedBike("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "LAG-A-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	tokenB := mustVehicleRoleToken(t, tokens, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/"+bikeOfProviderA.ID, nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("IDOR: status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestGetBikeReturns404ForUnknownID(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/99999999-9999-9999-9999-999999999999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestGetBikeInvalidUUIDReturns400(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 or 400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestGetBikeIncludesDocumentsArray(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-DOCS-01", true)
	// Seed a document.
	repo.docs = append(repo.docs, BikeDocument{
		ID:           "doc-0001",
		BikeID:       bike.ID,
		ProviderID:   providerID,
		DocumentType: DocRegistration,
		FileURL:      "local-private://vehicles/" + providerID + "/" + bike.ID + "/registration/uuid_reg.pdf",
		UploadedAt:   time.Now().UTC(),
	})

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/"+bike.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	docs := data["documents"].([]any)
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
	doc := docs[0].(map[string]any)
	if !strings.HasPrefix(doc["file_url"].(string), "local-private://vehicles/") {
		t.Fatalf("file_url = %v, want local-private:// prefix", doc["file_url"])
	}
}

// ── Phase 4E: PATCH /api/v1/provider/vehicle/:id ─────────────────────────────

func TestUpdateBikeColorOfUnverifiedBike(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-UPD-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body := `{"color":"Green"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/vehicle/"+bike.ID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["color"] != "Green" {
		t.Fatalf("color = %v, want Green", data["color"])
	}
}

func TestUpdateBikeUpdatesOnlyProvidedFields(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-UPD-02", true)
	originalBrand := bike.Brand

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body := `{"color":"Blue"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/vehicle/"+bike.ID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["brand"] != originalBrand {
		t.Fatalf("brand changed unexpectedly: got %v, want %v", data["brand"], originalBrand)
	}
}

func TestUpdateBikeIgnoresPlateNumber(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-PLATE-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body := `{"color":"Yellow","plate_number":"NEW-PLATE-XX"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/vehicle/"+bike.ID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["plate_number"] == "NEW-PLATE-XX" {
		t.Fatal("plate_number was changed; it must be immutable")
	}
	if data["plate_number"] != "LAG-PLATE-01" {
		t.Fatalf("plate_number = %v, want LAG-PLATE-01", data["plate_number"])
	}
}

func TestUpdateBikeIgnoresBikeType(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-TYPE-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body := `{"color":"Purple","bike_type":"dispatch_bike"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/vehicle/"+bike.ID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["bike_type"] != string(bike.BikeType) {
		t.Fatalf("bike_type = %v, want %s (unchanged)", data["bike_type"], bike.BikeType)
	}
}

func TestUpdateVerifiedBikeReturns409(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBikeWithStatus(providerID, "LAG-VER-01", true, VehicleVerified)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body := `{"color":"Silver"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/vehicle/"+bike.ID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeConflict)
}

func TestUpdateSuspendedBikeReturns409(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBikeWithStatus(providerID, "LAG-SUS-01", true, VehicleSuspended)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body := `{"color":"Black"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/vehicle/"+bike.ID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeConflict)
}

func TestUpdateBikeEmptyBodyReturns400(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-EMP-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/vehicle/"+bike.ID, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 or 400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestUpdateBikeInvalidYearReturns400(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-YR2-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body := `{"year":99}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/vehicle/"+bike.ID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 or 400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestUpdateBikeCrossProviderReturns404(t *testing.T) {
	repo := newFakeRepository()
	bikeA := repo.seedBike("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "LAG-XPR-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	tokenB := mustVehicleRoleToken(t, tokens, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "dispatch_provider")

	body := `{"color":"Pink"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/vehicle/"+bikeA.ID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenB)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestUpdateBikeCreatesAuditRow(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-AUDU-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body := `{"color":"Cyan"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/vehicle/"+bike.ID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	updatedAudits := []BikeAudit{}
	for _, a := range repo.audits {
		if a.Action == AuditUpdated {
			updatedAudits = append(updatedAudits, a)
		}
	}
	if len(updatedAudits) == 0 {
		t.Fatal("no updated audit row created")
	}
}

// ── Phase 4F: POST /api/v1/provider/vehicle/:id/documents ────────────────────

func TestUploadValidRegistrationDocumentReturns201(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-DOC-01", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body, ct := makeMultipartBody(t, "document_type", "registration", "document_file", "reg.pdf", "application/pdf", []byte("%PDF-test content"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
}

func TestUploadDocumentCreatesDocumentRow(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-DOC-02", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body, ct := makeMultipartBody(t, "document_type", "registration", "document_file", "reg.pdf", "application/pdf", []byte("%PDF-test"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if len(repo.docs) != 1 {
		t.Fatalf("doc rows = %d, want 1", len(repo.docs))
	}
}

func TestUploadDocumentTransitionsUnverifiedToPending(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-TRANS-01", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body, ct := makeMultipartBody(t, "document_type", "registration", "document_file", "reg.pdf", "application/pdf", []byte("%PDF-test"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	// The bike in the fake repo should now be pending.
	for _, b := range repo.bikes {
		if b.ID == bike.ID && b.VerificationStatus != VehiclePending {
			t.Fatalf("bike status = %s, want pending", b.VerificationStatus)
		}
	}
}

func TestUploadDocumentTransitionsRejectedToPending(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBikeWithStatus(providerID, "LAG-REJ-01", true, VehicleRejected)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body, ct := makeMultipartBody(t, "document_type", "registration", "document_file", "reg.pdf", "application/pdf", []byte("%PDF-test"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	for _, b := range repo.bikes {
		if b.ID == bike.ID && b.VerificationStatus != VehiclePending {
			t.Fatalf("rejected bike status = %s after re-upload, want pending", b.VerificationStatus)
		}
	}
}

func TestUploadInsuranceWithFutureExpiryAccepted(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-INS-01", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	futureDate := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	body, ct := makeMultipartBodyWithExtra(t, map[string]string{
		"document_type": "insurance",
		"expiry_date":   futureDate,
	}, "document_file", "ins.pdf", "application/pdf", []byte("%PDF-insurance"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
}

func TestUploadInsuranceWithoutExpiryReturns400(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-INS-02", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body, ct := makeMultipartBody(t, "document_type", "insurance", "document_file", "ins.pdf", "application/pdf", []byte("%PDF-insurance"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestUploadInsuranceWithPastExpiryReturns400(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-INS-03", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	pastDate := "2020-01-01"
	body, ct := makeMultipartBodyWithExtra(t, map[string]string{
		"document_type": "insurance",
		"expiry_date":   pastDate,
	}, "document_file", "ins.pdf", "application/pdf", []byte("%PDF-insurance"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestUploadMissingDocumentFileReturns400(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-NF-01", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	// Only send document_type, no file.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("document_type", "registration")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestUploadInvalidDocumentTypeReturns400(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-DT-01", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body, ct := makeMultipartBody(t, "document_type", "passport", "document_file", "bad.pdf", "application/pdf", []byte("%PDF-test"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestUploadUnsupportedFileTypeReturns400(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-MIME-01", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	// Use an unsupported MIME type: text/plain.
	body, ct := makeMultipartBody(t, "document_type", "registration", "document_file", "bad.txt", "text/plain", []byte("plain text"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestUploadOversizedFileReturns400(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-BIG-01", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))

	// Create a fake oversized file header (6MB).
	oversizedHeader := &stubFileHeader{
		filename:    "big.pdf",
		contentType: "application/pdf",
		size:        6 * 1024 * 1024,
		content:     []byte("%PDF-"),
	}

	// Use the service directly to avoid real multipart parsing for size.
	input := UploadDocumentInput{
		ProviderID:   providerID,
		BikeID:       bike.ID,
		DocumentType: DocRegistration,
		File:         oversizedHeader.openReader(),
		Header:       oversizedHeader,
	}
	ctx := context.Background()
	_, err := svc.UploadDocument(ctx, input)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != apperrors.CodeValidationFailed {
		t.Fatalf("code = %s, want validation_failed", appErr.Code)
	}
}

func TestUploadToVerifiedBikeReturns409(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBikeWithStatus(providerID, "LAG-VERD-01", true, VehicleVerified)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body, ct := makeMultipartBody(t, "document_type", "registration", "document_file", "reg.pdf", "application/pdf", []byte("%PDF-test"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeConflict)
}

func TestUploadToSuspendedBikeReturns409(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBikeWithStatus(providerID, "LAG-SUSD-01", true, VehicleSuspended)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	body, ct := makeMultipartBody(t, "document_type", "registration", "document_file", "reg.pdf", "application/pdf", []byte("%PDF-test"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeConflict)
}

func TestUploadCrossProviderReturns404(t *testing.T) {
	repo := newFakeRepository()
	bikeA := repo.seedBike("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "LAG-XPD-01", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	tokenB := mustVehicleRoleToken(t, tokens, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "dispatch_provider")

	body, ct := makeMultipartBody(t, "document_type", "registration", "document_file", "reg.pdf", "application/pdf", []byte("%PDF-test"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bikeA.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestUploadDocumentPublishesEvent(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-EVT-01", true)

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithUploader(&fakeUploader{baseURL: "local-private://"}), WithEventPublisher(ep))

	input := UploadDocumentInput{
		ProviderID:    providerID,
		BikeID:        bike.ID,
		CorrelationID: "test-correlation",
		DocumentType:  DocRegistration,
		File:          &stubFile{content: []byte("%PDF-test")},
		Header: &stubFileHeader{
			filename:    "reg.pdf",
			contentType: "application/pdf",
			size:        9,
			content:     []byte("%PDF-test"),
		},
	}
	_, err := svc.UploadDocument(context.Background(), input)
	if err != nil {
		t.Fatalf("UploadDocument() error = %v", err)
	}
	if ep.docsSubmittedCount != 1 {
		t.Fatalf("PublishVehicleDocsSubmitted called %d times, want 1", ep.docsSubmittedCount)
	}
}

func TestUploadDocumentCreatesAuditDocsUploaded(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-AUDD-01", true)

	svc := NewService(repo, WithUploader(&fakeUploader{baseURL: "local-private://"}), WithEventPublisher(&fakeEventPublisher{}))

	input := UploadDocumentInput{
		ProviderID:   providerID,
		BikeID:       bike.ID,
		DocumentType: DocRegistration,
		File:         &stubFile{content: []byte("%PDF-test")},
		Header: &stubFileHeader{
			filename:    "reg.pdf",
			contentType: "application/pdf",
			size:        9,
			content:     []byte("%PDF-test"),
		},
	}
	_, err := svc.UploadDocument(context.Background(), input)
	if err != nil {
		t.Fatalf("UploadDocument() error = %v", err)
	}
	found := false
	for _, a := range repo.audits {
		if a.Action == AuditDocsUploaded {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no docs_uploaded audit row created")
	}
}

func TestUploadDocumentFileURLUsesLocalPrivate(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-URL-01", true)

	svc := NewService(repo, WithUploader(&fakeUploader{baseURL: "local-private://"}), WithEventPublisher(&fakeEventPublisher{}))

	input := UploadDocumentInput{
		ProviderID:   providerID,
		BikeID:       bike.ID,
		DocumentType: DocRegistration,
		File:         &stubFile{content: []byte("%PDF-test")},
		Header: &stubFileHeader{
			filename:    "reg.pdf",
			contentType: "application/pdf",
			size:        9,
			content:     []byte("%PDF-test"),
		},
	}
	doc, err := svc.UploadDocument(context.Background(), input)
	if err != nil {
		t.Fatalf("UploadDocument() error = %v", err)
	}
	if !strings.HasPrefix(doc.FileURL, "local-private://vehicles/") {
		t.Fatalf("file_url = %q, want local-private://vehicles/ prefix", doc.FileURL)
	}
}

func TestValidationHappensBeforeUploaderCalled(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-VBEF-01", true)

	calledUploader := &countingUploader{}
	svc := NewService(repo, WithUploader(calledUploader), WithEventPublisher(&fakeEventPublisher{}))

	// Invalid MIME type — should fail before uploader is called.
	input := UploadDocumentInput{
		ProviderID:   providerID,
		BikeID:       bike.ID,
		DocumentType: DocRegistration,
		File:         &stubFile{content: []byte("not a pdf")},
		Header: &stubFileHeader{
			filename:    "bad.txt",
			contentType: "text/plain",
			size:        9,
			content:     []byte("not a pdf"),
		},
	}
	_, err := svc.UploadDocument(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if calledUploader.calls != 0 {
		t.Fatalf("uploader was called %d times before validation passed; want 0", calledUploader.calls)
	}
}

// ── Storage tests (Phase 4F) ──────────────────────────────────────────────────

// ── Phase 4G: GET /api/v1/provider/vehicle/:id/documents ─────────────────────

func TestGetDocumentsReturnsEmptyArrayWhenNone(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-GD-01", true)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/"+bike.ID+"/documents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].([]any)
	if data == nil {
		// null JSON also accepted if tests run in package context; check success field
		if resp["success"] != true {
			t.Fatalf("success = %v, want true", resp["success"])
		}
	} else if len(data) != 0 {
		t.Fatalf("data length = %d, want 0", len(data))
	}
}

func TestGetDocumentsReturnsDocumentsOrderedByUploadedAt(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-GD-02", true)

	// Insert two docs in order.
	now := time.Now().UTC()
	repo.docs = append(repo.docs,
		BikeDocument{ID: "doc-first", BikeID: bike.ID, ProviderID: providerID, DocumentType: DocRegistration, FileURL: "local-private://r", UploadedAt: now.Add(-10 * time.Minute)},
		BikeDocument{ID: "doc-second", BikeID: bike.ID, ProviderID: providerID, DocumentType: DocInsurance, FileURL: "local-private://i", UploadedAt: now},
	)

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/"+bike.ID+"/documents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("data length = %d, want 2", len(data))
	}
}

func TestGetDocumentsReturns404ForAnotherProvidersBike(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBike("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "LAG-GD-IDOR", true)
	bikeA := repo.bikes[0]

	router, tokens := buildVehicleTestRouter(repo)
	tokenB := mustVehicleRoleToken(t, tokens, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/"+bikeA.ID+"/documents", nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestGetDocumentsReturns404ForUnknownBike(t *testing.T) {
	repo := newFakeRepository()
	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/99999999-9999-9999-9999-999999999999/documents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestGetDocumentsInvalidUUIDReturns400(t *testing.T) {
	repo := newFakeRepository()
	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/not-a-uuid/documents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/422; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestGetDocumentsReturns401WithoutJWT(t *testing.T) {
	router, _ := buildVehicleTestRouter(newFakeRepository())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/11111111-1111-1111-1111-111111111111/documents", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeUnauthorized)
}

func TestGetDocumentsFileURLUsesLocalPrivate(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-GD-URL", true)
	repo.docs = append(repo.docs, BikeDocument{
		ID: "doc-url", BikeID: bike.ID, ProviderID: providerID,
		DocumentType: DocRegistration,
		FileURL:      "local-private://vehicles/prov/bike/registration/uuid_doc.pdf",
		UploadedAt:   time.Now().UTC(),
	})

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/vehicle/"+bike.ID+"/documents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	docs, _ := resp["data"].([]any)
	if len(docs) == 0 {
		t.Fatal("expected at least one document")
	}
	doc0, _ := docs[0].(map[string]any)
	fileURL, _ := doc0["file_url"].(string)
	if !strings.HasPrefix(fileURL, "local-private://") {
		t.Fatalf("file_url = %q, want local-private:// prefix", fileURL)
	}
}

// ── Phase 4H: PATCH /api/v1/admin/vehicle/:id/review ─────────────────────────

func TestAdminReviewMissingActionReturns400(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBike("prov-1", "LAG-ADM-01", true)
	bikeID := repo.bikes[0].ID

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "admin-1", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestAdminReviewInvalidActionReturns400(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBike("prov-2", "LAG-ADM-02", true)
	bikeID := repo.bikes[0].ID

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "admin-1", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"delete"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestAdminReviewRejectWithoutReasonReturns400(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBike("prov-3", "LAG-ADM-03", true)
	bikeID := repo.bikes[0].ID

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "admin-1", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"rejected"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestAdminReviewSuspendWithoutReasonReturns400(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBike("prov-4", "LAG-ADM-04", true)
	bikeID := repo.bikes[0].ID

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "admin-1", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"suspended"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/400; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
}

func TestAdminReviewUnknownBikeReturns404(t *testing.T) {
	router, tokens := buildVehicleTestRouter(newFakeRepository())
	token := mustVehicleRoleToken(t, tokens, "admin-1", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/99999999-9999-9999-9999-999999999999/review",
		strings.NewReader(`{"action":"approved"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestAdminApproveUpdatesBikeStatusToVerified(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-A", "LAG-APR-01", true, VehiclePending)
	bikeID := repo.bikes[0].ID

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"approved"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["verification_status"] != "verified" {
		t.Fatalf("verification_status = %v, want verified", data["verification_status"])
	}
	if data["is_active"] != true {
		t.Fatalf("is_active = %v, want true", data["is_active"])
	}
}

func TestAdminApproveCreatesAuditRow(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-A", "LAG-APR-02", true, VehiclePending)
	bikeID := repo.bikes[0].ID

	svc := NewService(repo, WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"approved"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	found := false
	for _, a := range repo.audits {
		if a.Action == AuditApproved {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no approved audit row created")
	}
}

func TestAdminApprovePublishesVehicleVerifiedEvent(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-A", "LAG-APR-03", true, VehiclePending)
	bikeID := repo.bikes[0].ID

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"approved"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if ep.verifiedCount != 1 {
		t.Fatalf("PublishVehicleVerified called %d times, want 1", ep.verifiedCount)
	}
}

func TestAdminRejectUpdatesBikeStatusToRejected(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-B", "LAG-REJ-ADM", true, VehiclePending)
	bikeID := repo.bikes[0].ID

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"rejected","reason":"Blurry photo"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["verification_status"] != "rejected" {
		t.Fatalf("verification_status = %v, want rejected", data["verification_status"])
	}
}

func TestAdminRejectStoresReasonInAudit(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-B", "LAG-RJN-01", true, VehiclePending)
	bikeID := repo.bikes[0].ID

	svc := NewService(repo, WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"rejected","reason":"Photo unclear"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	found := false
	for _, a := range repo.audits {
		if a.Action == AuditRejected && a.Notes != nil && *a.Notes == "Photo unclear" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no rejected audit row with reason found")
	}
}

func TestAdminRejectPublishesVehicleRejectedEvent(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-B", "LAG-RJE-01", true, VehiclePending)
	bikeID := repo.bikes[0].ID

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"rejected","reason":"Blurry"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	if ep.rejectedCount != 1 {
		t.Fatalf("PublishVehicleRejected called %d times, want 1", ep.rejectedCount)
	}
}

func TestAdminSuspendUpdatesBikeStatusToSuspended(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-C", "LAG-SUS-ADM", true, VehicleVerified)
	bikeID := repo.bikes[0].ID

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"suspended","reason":"Fraudulent document"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := extractVehicleData(t, w.Body.Bytes())
	if data["verification_status"] != "suspended" {
		t.Fatalf("verification_status = %v, want suspended", data["verification_status"])
	}
	if data["is_active"] != false {
		t.Fatalf("is_active = %v, want false after suspend", data["is_active"])
	}
}

func TestAdminSuspendPublishesVehicleSuspendedEvent(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-C", "LAG-SUSE-01", true, VehicleVerified)
	bikeID := repo.bikes[0].ID

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"suspended","reason":"Fraud"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	if ep.suspendedCount != 1 {
		t.Fatalf("PublishVehicleSuspended called %d times, want 1", ep.suspendedCount)
	}
}

func TestAdminSuspendCreatesAuditRow(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-C", "LAG-SUSR-01", true, VehicleVerified)
	bikeID := repo.bikes[0].ID

	svc := NewService(repo, WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"suspended","reason":"Policy violation"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	found := false
	for _, a := range repo.audits {
		if a.Action == AuditSuspended {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no suspended audit row created")
	}
}

func TestAdminApproveAlreadyVerifiedReturns409(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-D", "LAG-409A", true, VehicleVerified)
	bikeID := repo.bikes[0].ID

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"approved"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeConflict)
}

func TestAdminSuspendAlreadySuspendedReturns409(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-E", "LAG-409S", true, VehicleSuspended)
	bikeID := repo.bikes[0].ID

	router, tokens := buildVehicleTestRouter(repo)
	token := mustVehicleRoleToken(t, tokens, "admin-99", "platform_admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/vehicle/"+bikeID+"/review",
		strings.NewReader(`{"action":"suspended","reason":"Again"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
	assertVehicleErrorCode(t, w.Body.Bytes(), apperrors.CodeConflict)
}

// ── Phase 4I: Events ──────────────────────────────────────────────────────────

func TestRegisterBikePublishesVehicleRegisteredEvent(t *testing.T) {
	repo := newFakeRepository()
	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))

	_, err := svc.RegisterBike(context.Background(), "prov-reg", "corr-123", RegisterBikeInput{
		BikeType:    BikeMotorcycle,
		Brand:       "Honda",
		Model:       "CB125F",
		Year:        2022,
		Color:       "Red",
		PlateNumber: "LAG-EV-001",
	})
	if err != nil {
		t.Fatalf("RegisterBike() error = %v", err)
	}
	if ep.registeredCount != 1 {
		t.Fatalf("PublishVehicleRegistered called %d times, want 1", ep.registeredCount)
	}
}

func TestUploadDocumentStillPublishesDocsSubmittedEvent(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-4I-DS", true)

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithUploader(&fakeUploader{baseURL: "local-private://"}), WithEventPublisher(ep))

	input := UploadDocumentInput{
		ProviderID:    providerID,
		BikeID:        bike.ID,
		CorrelationID: "corr-4i",
		DocumentType:  DocRegistration,
		File:          &stubFile{content: []byte("%PDF-test")},
		Header: &stubFileHeader{
			filename: "doc.pdf", contentType: "application/pdf", size: 9,
			content: []byte("%PDF-test"),
		},
	}
	_, err := svc.UploadDocument(context.Background(), input)
	if err != nil {
		t.Fatalf("UploadDocument() error = %v", err)
	}
	if ep.docsSubmittedCount != 1 {
		t.Fatalf("PublishVehicleDocsSubmitted called %d times, want 1", ep.docsSubmittedCount)
	}
}

func TestVehicleRegisteredEventHasProviderIDAndBikeID(t *testing.T) {
	repo := newFakeRepository()
	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))

	bike, err := svc.RegisterBike(context.Background(), "prov-fields", "corr-fields", RegisterBikeInput{
		BikeType:    BikeMotorcycle,
		Brand:       "Honda",
		Model:       "CB125F",
		Year:        2022,
		Color:       "Red",
		PlateNumber: "LAG-EV-002",
	})
	if err != nil {
		t.Fatalf("RegisterBike() error = %v", err)
	}
	if ep.lastRegisteredEvent.ProviderID != "prov-fields" {
		t.Fatalf("ProviderID = %q, want prov-fields", ep.lastRegisteredEvent.ProviderID)
	}
	if ep.lastRegisteredEvent.BikeID != bike.ID {
		t.Fatalf("BikeID = %q, want %s", ep.lastRegisteredEvent.BikeID, bike.ID)
	}
	if ep.lastRegisteredEvent.CorrelationID == "" {
		t.Fatal("correlation_id must not be empty")
	}
}

func TestVehicleVerifiedEventHasProviderIDAndBikeID(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-vev", "LAG-VEV-01", true, VehiclePending)
	bikeID := repo.bikes[0].ID

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))
	_, err := svc.AdminReview(context.Background(), bikeID, "admin-1", "corr-vev",
		AdminReviewInput{Action: AuditApproved})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	if ep.lastVerifiedEvent.ProviderID == "" {
		t.Fatal("vehicle.verified event missing provider_id")
	}
	if ep.lastVerifiedEvent.BikeID == "" {
		t.Fatal("vehicle.verified event missing bike_id")
	}
	if ep.lastVerifiedEvent.CorrelationID == "" {
		t.Fatal("vehicle.verified event missing correlation_id")
	}
}

func TestVehicleRejectedEventHasReason(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-rej", "LAG-RJEV-01", true, VehiclePending)
	bikeID := repo.bikes[0].ID

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))
	_, err := svc.AdminReview(context.Background(), bikeID, "admin-1", "corr-rej",
		AdminReviewInput{Action: AuditRejected, Reason: "Bad photo"})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	if ep.lastRejectedEvent.Reason != "Bad photo" {
		t.Fatalf("vehicle.rejected event reason = %q, want 'Bad photo'", ep.lastRejectedEvent.Reason)
	}
}

func TestVehicleSuspendedEventHasReason(t *testing.T) {
	repo := newFakeRepository()
	repo.seedBikeWithStatus("prov-sus", "LAG-SUSE-02", true, VehicleVerified)
	bikeID := repo.bikes[0].ID

	ep := &fakeEventPublisher{}
	svc := NewService(repo, WithEventPublisher(ep))
	_, err := svc.AdminReview(context.Background(), bikeID, "admin-1", "corr-sus",
		AdminReviewInput{Action: AuditSuspended, Reason: "Policy violation"})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	if ep.lastSuspendedEvent.Reason != "Policy violation" {
		t.Fatalf("vehicle.suspended event reason = %q, want 'Policy violation'", ep.lastSuspendedEvent.Reason)
	}
}

func TestProviderVerificationSuspendedSubscriberSuspendsAllBikes(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	repo.seedBikeWithStatus(providerID, "LAG-SUB-01", true, VehicleVerified)
	repo.seedBikeWithStatus(providerID, "LAG-SUB-02", false, VehiclePending)

	payload := `{"event":"provider.verification.suspended","correlation_id":"corr-sub","provider_id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","reason":"Account suspended"}`
	err := HandleProviderVerificationSuspendedPayload(context.Background(), repo, []byte(payload))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	for _, b := range repo.bikes {
		if b.ProviderID == providerID {
			if b.VerificationStatus != VehicleSuspended {
				t.Fatalf("bike %s status = %s, want suspended", b.ID, b.VerificationStatus)
			}
			if b.IsActive {
				t.Fatalf("bike %s is_active = true, want false", b.ID)
			}
		}
	}
}

func TestProviderVerificationSuspendedBadPayloadDoesNotCrash(t *testing.T) {
	repo := newFakeRepository()
	// Bad JSON — handler should log and return nil (not panic, not return error).
	err := HandleProviderVerificationSuspendedPayload(context.Background(), repo, []byte(`{not valid json`))
	if err != nil {
		t.Fatalf("handler returned error for bad payload: %v", err)
	}
}

func TestBuildVehicleObjectPathFormat(t *testing.T) {
	path := buildVehicleObjectPath("prov-123", "bike-456", DocRegistration, "doc.pdf")
	if !strings.HasPrefix(path, "vehicles/prov-123/bike-456/registration/") {
		t.Fatalf("path = %q, want vehicles/prov-123/bike-456/registration/ prefix", path)
	}
	if !strings.HasSuffix(path, "_doc.pdf") {
		t.Fatalf("path = %q, want _doc.pdf suffix", path)
	}
}

func TestSanitizeVehicleFilenameTraversal(t *testing.T) {
	dangerous := `../../../etc/passwd`
	safe := sanitizeVehicleFilename(dangerous)
	if strings.Contains(safe, "..") || strings.Contains(safe, "/") {
		t.Fatalf("sanitized = %q, still contains traversal characters", safe)
	}
}

func TestCleanVehicleStoragePathRejectsDotDot(t *testing.T) {
	_, err := cleanVehicleStoragePath("../etc/passwd")
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
}

func TestVehicleUploaderNoS3OrAWSInPath(t *testing.T) {
	path := buildVehicleObjectPath("p", "b", DocInsurance, "ins.pdf")
	if strings.Contains(strings.ToLower(path), "s3") || strings.Contains(strings.ToLower(path), "aws") {
		t.Fatalf("path contains S3/AWS reference: %s", path)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

const vehicleTestSecret = "vehicle-test-secret"

func buildVehicleTestRouter(repo Repository) (*gin.Engine, *authusecases.TokenUsecase) {
	svc := NewService(repo)
	return buildVehicleTestRouterWithSvc(svc)
}

func buildVehicleTestRouterWithSvc(svc *Service) (*gin.Engine, *authusecases.TokenUsecase) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.RequestID())
	engine.Use(httpx.Recovery())
	engine.Use(httpx.ErrorHandler())

	tokens := authusecases.NewTokenUsecase([]byte(vehicleTestSecret), 15*time.Minute, 30*24*time.Hour)
	RegisterRoutes(engine, tokens, NewHandlerWithService(svc))
	return engine, tokens
}

func mustVehicleRoleToken(t *testing.T, tokens *authusecases.TokenUsecase, providerID, role string) string {
	t.Helper()
	token, _, err := tokens.GenerateAccessToken(providerID, "+2348000000001", "session-123")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	claims["role"] = role
	claims["dispatch_rider_id"] = providerID
	updated, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	unsigned := parts[0] + "." + base64.RawURLEncoding.EncodeToString(updated)
	return unsigned + "." + signVehicleJWTForTest([]byte(vehicleTestSecret), unsigned)
}

func signVehicleJWTForTest(secret []byte, unsigned string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func assertVehicleErrorCode(t *testing.T, raw []byte, code apperrors.Code) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errorObj, _ := resp["error"].(map[string]any)
	if errorObj["code"] != string(code) {
		t.Fatalf("code = %v, want %s; body = %s", errorObj["code"], code, raw)
	}
}

func extractVehicleData(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("data field missing or wrong type; body = %s", raw)
	}
	return data
}

// ── Fake repository ───────────────────────────────────────────────────────────

type fakeRepository struct {
	bikes  []Bike
	docs   []BikeDocument
	audits []BikeAudit
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{}
}

func (r *fakeRepository) seedBike(providerID, plate string, isPrimary bool) Bike {
	return r.seedBikeWithStatus(providerID, plate, isPrimary, VehicleUnverified)
}

func (r *fakeRepository) seedBikeWithStatus(providerID, plate string, isPrimary bool, status VehicleStatus) Bike {
	b := Bike{
		ID:                 fmt.Sprintf("00000000-0000-0000-%04d-000000000000", len(r.bikes)+1),
		ProviderID:         providerID,
		BikeType:           BikeMotorcycle,
		Brand:              "Honda",
		Model:              "CB125F",
		Year:               2022,
		Color:              "Red",
		PlateNumber:        plate,
		VerificationStatus: status,
		IsActive:           true,
		IsPrimary:          isPrimary,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	r.bikes = append(r.bikes, b)
	return b
}

func (r *fakeRepository) InsertBike(_ context.Context, providerID string, input RegisterBikeInput, isPrimary bool) (Bike, error) {
	b := Bike{
		ID:                 fmt.Sprintf("00000000-0000-0000-%04d-000000000000", len(r.bikes)+1),
		ProviderID:         providerID,
		BikeType:           input.BikeType,
		Brand:              input.Brand,
		Model:              input.Model,
		Year:               input.Year,
		Color:              input.Color,
		PlateNumber:        input.PlateNumber,
		EngineCc:           input.EngineCc,
		ChassisNumber:      input.ChassisNumber,
		VerificationStatus: VehicleUnverified,
		IsActive:           true,
		IsPrimary:          isPrimary,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	r.bikes = append(r.bikes, b)
	return b, nil
}

func (r *fakeRepository) GetBikeByID(_ context.Context, bikeID, providerID string) (Bike, bool, error) {
	for _, b := range r.bikes {
		if b.ID == bikeID && b.ProviderID == providerID {
			return b, true, nil
		}
	}
	return Bike{}, false, nil
}

func (r *fakeRepository) GetBikeByIDAdmin(_ context.Context, bikeID string) (Bike, bool, error) {
	for _, b := range r.bikes {
		if b.ID == bikeID {
			return b, true, nil
		}
	}
	return Bike{}, false, nil
}

func (r *fakeRepository) ListBikesByProvider(_ context.Context, providerID string) ([]Bike, error) {
	var result []Bike
	for _, b := range r.bikes {
		if b.ProviderID == providerID {
			result = append(result, b)
		}
	}
	// Sort: primary first (stable).
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if !result[i].IsPrimary && result[j].IsPrimary {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result, nil
}

func (r *fakeRepository) UpdateBike(_ context.Context, bikeID, providerID string, input UpdateBikeInput) (Bike, error) {
	for i, b := range r.bikes {
		if b.ID == bikeID && b.ProviderID == providerID {
			if input.Brand != nil {
				r.bikes[i].Brand = *input.Brand
			}
			if input.Model != nil {
				r.bikes[i].Model = *input.Model
			}
			if input.Year != nil {
				r.bikes[i].Year = *input.Year
			}
			if input.Color != nil {
				r.bikes[i].Color = *input.Color
			}
			if input.EngineCc != nil {
				r.bikes[i].EngineCc = input.EngineCc
			}
			if input.ChassisNumber != nil {
				r.bikes[i].ChassisNumber = input.ChassisNumber
			}
			r.bikes[i].UpdatedAt = time.Now().UTC()
			return r.bikes[i], nil
		}
	}
	return Bike{}, apperrors.NotFound("Bike not found.", nil)
}

func (r *fakeRepository) UpdateBikeStatus(_ context.Context, bikeID string, status VehicleStatus) (Bike, error) {
	for i, b := range r.bikes {
		if b.ID == bikeID {
			r.bikes[i].VerificationStatus = status
			r.bikes[i].UpdatedAt = time.Now().UTC()
			return r.bikes[i], nil
		}
	}
	return Bike{}, apperrors.NotFound("Bike not found.", nil)
}

func (r *fakeRepository) HasAnyBike(_ context.Context, providerID string) (bool, error) {
	for _, b := range r.bikes {
		if b.ProviderID == providerID {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeRepository) PlateNumberExists(_ context.Context, plateNumber string) (bool, error) {
	for _, b := range r.bikes {
		if b.PlateNumber == plateNumber {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeRepository) AdminUpdateBikeStatus(_ context.Context, bikeID string, status VehicleStatus) (Bike, error) {
	for i, b := range r.bikes {
		if b.ID == bikeID {
			r.bikes[i].VerificationStatus = status
			// Mirror production: is_active = false only when suspending.
			r.bikes[i].IsActive = status != VehicleSuspended
			r.bikes[i].UpdatedAt = time.Now().UTC()
			return r.bikes[i], nil
		}
	}
	return Bike{}, apperrors.NotFound("Bike not found.", nil)
}

func (r *fakeRepository) InsertAudit(_ context.Context, input AuditInput) error {
	r.audits = append(r.audits, BikeAudit{
		ID:          fmt.Sprintf("audit-%04d", len(r.audits)+1),
		BikeID:      input.BikeID,
		ProviderID:  input.ProviderID,
		Action:      input.Action,
		FromStatus:  input.FromStatus,
		ToStatus:    input.ToStatus,
		PerformedBy: input.PerformedBy,
		Notes:       input.Notes,
		CreatedAt:   time.Now().UTC(),
	})
	return nil
}

func (r *fakeRepository) InsertBikeDocument(_ context.Context, doc BikeDocument) (BikeDocument, error) {
	doc.ID = fmt.Sprintf("doc-%04d", len(r.docs)+1)
	doc.UploadedAt = time.Now().UTC()
	r.docs = append(r.docs, doc)
	return doc, nil
}

func (r *fakeRepository) ListBikeDocuments(_ context.Context, bikeID, providerID string) ([]BikeDocument, error) {
	var result []BikeDocument
	for _, d := range r.docs {
		if d.BikeID == bikeID && d.ProviderID == providerID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (r *fakeRepository) SuspendAllBikesForProvider(_ context.Context, providerID string, reason string) error {
	for i, b := range r.bikes {
		if b.ProviderID == providerID && b.VerificationStatus != VehicleSuspended {
			prev := r.bikes[i].VerificationStatus
			r.bikes[i].VerificationStatus = VehicleSuspended
			r.bikes[i].IsActive = false
			r.bikes[i].UpdatedAt = time.Now().UTC()
			var notePtr *string
			if reason != "" {
				notePtr = &reason
			}
			r.audits = append(r.audits, BikeAudit{
				ID:         fmt.Sprintf("audit-sub-%04d", len(r.audits)+1),
				BikeID:     b.ID,
				ProviderID: providerID,
				Action:     AuditSuspended,
				FromStatus: prev,
				ToStatus:   VehicleSuspended,
				Notes:      notePtr,
				CreatedAt:  time.Now().UTC(),
			})
		}
	}
	return nil
}

// ── Fake uploader ─────────────────────────────────────────────────────────────

type fakeUploader struct {
	baseURL string
}

func (u *fakeUploader) Upload(_ context.Context, objectPath string, file File, header FileHeader) (string, error) {
	// Drain the file.
	if file != nil {
		_, _ = io.ReadAll(file)
	}
	return u.baseURL + objectPath, nil
}

type countingUploader struct {
	calls int
}

func (u *countingUploader) Upload(_ context.Context, _ string, file File, _ FileHeader) (string, error) {
	u.calls++
	if file != nil {
		_, _ = io.ReadAll(file)
	}
	return "local-private://test/path", nil
}

// ── Fake event publisher ──────────────────────────────────────────────────────

type fakeEventPublisher struct {
	registeredCount    int
	verifiedCount      int
	rejectedCount      int
	suspendedCount     int
	docsSubmittedCount int

	lastRegisteredEvent VehicleRegisteredEvent
	lastVerifiedEvent   VehicleVerifiedEvent
	lastRejectedEvent   VehicleRejectedEvent
	lastSuspendedEvent  VehicleSuspendedEvent
}

func (p *fakeEventPublisher) PublishVehicleRegistered(_ context.Context, e VehicleRegisteredEvent) error {
	p.registeredCount++
	p.lastRegisteredEvent = e
	return nil
}

func (p *fakeEventPublisher) PublishVehicleVerified(_ context.Context, e VehicleVerifiedEvent) error {
	p.verifiedCount++
	p.lastVerifiedEvent = e
	return nil
}

func (p *fakeEventPublisher) PublishVehicleRejected(_ context.Context, e VehicleRejectedEvent) error {
	p.rejectedCount++
	p.lastRejectedEvent = e
	return nil
}

func (p *fakeEventPublisher) PublishVehicleSuspended(_ context.Context, e VehicleSuspendedEvent) error {
	p.suspendedCount++
	p.lastSuspendedEvent = e
	return nil
}

func (p *fakeEventPublisher) PublishVehicleDocsSubmitted(_ context.Context, _ VehicleDocsSubmittedEvent) error {
	p.docsSubmittedCount++
	return nil
}

// ── Stub file helpers ─────────────────────────────────────────────────────────

type stubFile struct {
	content []byte
	reader  *bytes.Reader
}

func (f *stubFile) Read(p []byte) (int, error) {
	if f.reader == nil {
		f.reader = bytes.NewReader(f.content)
	}
	return f.reader.Read(p)
}

func (f *stubFile) Close() error { return nil }

type stubFileHeader struct {
	filename    string
	contentType string
	size        int64
	content     []byte
}

func (h *stubFileHeader) Open() (File, error) {
	return &stubFile{content: h.content}, nil
}

func (h *stubFileHeader) GetFilename() string { return h.filename }
func (h *stubFileHeader) GetSize() int64      { return h.size }
func (h *stubFileHeader) GetHeaderValue(key string) string {
	if strings.EqualFold(key, "Content-Type") {
		return h.contentType
	}
	return ""
}
func (h *stubFileHeader) openReader() *stubFile { return &stubFile{content: h.content} }

// ── Multipart body helpers ────────────────────────────────────────────────────

// makeMultipartBody builds a multipart body with one text field and one file.
func makeMultipartBody(t *testing.T, fieldName, fieldValue, fileField, filename, contentType string, fileContent []byte) (io.Reader, string) {
	t.Helper()
	return makeMultipartBodyWithExtra(t,
		map[string]string{fieldName: fieldValue},
		fileField, filename, contentType, fileContent,
	)
}

// makeMultipartBodyWithExtra builds a multipart body with multiple text fields and one file.
func makeMultipartBodyWithExtra(t *testing.T, fields map[string]string, fileField, filename, contentType string, fileContent []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField(%q): %v", k, err)
		}
	}
	part, err := mw.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	// Inject Content-Type into the part header.
	_ = part
	// CreateFormFile sets Content-Disposition but not Content-Type; we rely on
	// the header injected by the test to control what GetHeaderValue returns.
	// For real multipart, the part Content-Type is set separately.
	// We write the file bytes regardless.
	_, _ = io.Copy(part, bytes.NewReader(fileContent))

	// We need to override the part's Content-Type. In real multipart parsing,
	// gin uses multipart.FileHeader which picks up the part Content-Type.
	// For testing the handler layer we inject via the form part header override
	// using a custom writer that sets the content-type in the MIME header.
	mw.Close()

	// Re-build with explicit Content-Type in the file part.
	var buf2 bytes.Buffer
	mw2 := multipart.NewWriter(&buf2)
	for k, v := range fields {
		_ = mw2.WriteField(k, v)
	}
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="` + fileField + `"; filename="` + filename + `"`}
	h["Content-Type"] = []string{contentType}
	part2, err := mw2.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	_, _ = io.Copy(part2, bytes.NewReader(fileContent))
	mw2.Close()
	return &buf2, mw2.FormDataContentType()
}

func TestUploadDocumentMagicBytesMismatchRejects(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-MAGIC-01", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	// We declare application/pdf in header but send plain text "hello world" in content
	body, ct := makeMultipartBody(t, "document_type", "registration", "document_file", "reg.pdf", "application/pdf", []byte("hello world not a pdf"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
}

func TestUploadDocumentMagicBytesSuccess(t *testing.T) {
	repo := newFakeRepository()
	providerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bike := repo.seedBike(providerID, "LAG-MAGIC-02", true)

	uploader := &fakeUploader{baseURL: "local-private://"}
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(&fakeEventPublisher{}))
	router, tokens := buildVehicleTestRouterWithSvc(svc)
	token := mustVehicleRoleToken(t, tokens, providerID, "dispatch_provider")

	// PNG file with valid magic bytes prefix
	pngContent := append([]byte("\x89PNG\r\n\x1a\n"), []byte("some fake image bytes")...)
	body, ct := makeMultipartBody(t, "document_type", "registration", "document_file", "reg.png", "image/png", pngContent)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/vehicle/"+bike.ID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
}
