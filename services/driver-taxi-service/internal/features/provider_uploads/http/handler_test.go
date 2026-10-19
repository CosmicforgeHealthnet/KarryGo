package provideruploadshttp

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"
	provideruploadsusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_uploads/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/mediaclient"
)

type okMedia struct{ owner string }

func (m *okMedia) Upload(_ context.Context, r mediaclient.UploadRequest) (mediaclient.MediaAsset, error) {
	m.owner = r.OwnerID
	_, _ = io.Copy(io.Discard, r.Body)
	return mediaclient.MediaAsset{ID: "a1", PublicURL: "https://files/a1"}, nil
}

func router(media *okMedia) (*gin.Engine, string) {
	gin.SetMode(gin.TestMode)
	secret := []byte("access-secret")
	r := gin.New()
	r.Use(httpx.ErrorHandler())
	RegisterUploadRoutes(r.Group("/api/v1/hauling"), provideruploadsusecases.NewUploadService(media), sharedauth.NewTokenSigner(secret))
	now := time.Now()
	token, _ := sharedauth.NewTokenSigner(secret).Sign(sharedauth.Claims{
		Subject: "driver-9", Role: providerauthusecases.ProviderRole,
		Service: providerauthusecases.ProviderService, SessionID: "s",
		Type: sharedauth.TokenTypeAccess, IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(),
	})
	return r, token
}

func form(withFile bool) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("purpose", "document_file")
	if withFile {
		part, _ := w.CreateFormFile("file", "licence.jpg")
		_, _ = part.Write(append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 64)...))
	}
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func do(r *gin.Engine, token string, withFile bool) *httptest.ResponseRecorder {
	body, ct := form(withFile)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hauling/provider/uploads", body)
	req.Header.Set("Content-Type", ct)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestUploadRoute(t *testing.T) {
	media := &okMedia{}
	r, token := router(media)

	if rec := do(r, "", true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned request: want 401, got %d", rec.Code)
	}
	if rec := do(r, token, false); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing file: want 422, got %d %s", rec.Code, rec.Body)
	}
	rec := do(r, token, true)
	if rec.Code != http.StatusCreated || !bytes.Contains(rec.Body.Bytes(), []byte("https://files/a1")) {
		t.Fatalf("upload: got %d %s", rec.Code, rec.Body)
	}
	if media.owner != "driver-9" {
		t.Fatalf("owner must come from the token, got %q", media.owner)
	}
}
