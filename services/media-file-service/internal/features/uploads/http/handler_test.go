package uploadhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	filemetadatamodels "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/models"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
	uploadusecases "cosmicforge/logistics/services/media-file-service/internal/features/uploads/usecases"
	"cosmicforge/logistics/shared/go/httpx"
)

func TestUploadHTTPSuccess(t *testing.T) {
	router := newHTTPTestRouter(1024)

	response := uploadMultipart(t, router, uploadHTTPInput{
		Token:       "test-token",
		Service:     "customer-service",
		OwnerID:     "customer-1",
		Purpose:     filemetadatamodels.PurposeProfilePhoto,
		Filename:    "avatar.jpg",
		ContentType: "image/jpeg",
		File:        jpegPayload(),
	}, http.StatusCreated)

	data := response["data"].(map[string]interface{})
	if data["url"] == "" || data["id"] == "" {
		t.Fatalf("expected id and url, got %+v", data)
	}
}

func TestUploadHTTPRejectsInvalidToken(t *testing.T) {
	router := newHTTPTestRouter(1024)

	uploadMultipart(t, router, uploadHTTPInput{
		Token:       "wrong-token",
		Service:     "customer-service",
		OwnerID:     "customer-1",
		Purpose:     filemetadatamodels.PurposeProfilePhoto,
		Filename:    "avatar.jpg",
		ContentType: "image/jpeg",
		File:        jpegPayload(),
	}, http.StatusUnauthorized)
}

func TestUploadHTTPRejectsMissingFile(t *testing.T) {
	router := newHTTPTestRouter(1024)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("owner_service", "customer-service")
	_ = writer.WriteField("owner_id", "customer-1")
	_ = writer.WriteField("purpose", filemetadatamodels.PurposeProfilePhoto)
	_ = writer.Close()

	request := httptest.NewRequest(http.MethodPost, "/api/v1/media-files/uploads", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Karrygo-Service", "customer-service")
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestUploadHTTPRejectsUnsupportedMIME(t *testing.T) {
	router := newHTTPTestRouter(1024)

	uploadMultipart(t, router, uploadHTTPInput{
		Token:       "test-token",
		Service:     "customer-service",
		OwnerID:     "customer-1",
		Purpose:     filemetadatamodels.PurposeProfilePhoto,
		Filename:    "notes.txt",
		ContentType: "text/plain",
		File:        []byte("plain text"),
	}, http.StatusUnprocessableEntity)
}

func TestUploadHTTPRejectsOversizedFile(t *testing.T) {
	router := newHTTPTestRouter(4)

	uploadMultipart(t, router, uploadHTTPInput{
		Token:       "test-token",
		Service:     "customer-service",
		OwnerID:     "customer-1",
		Purpose:     filemetadatamodels.PurposeProfilePhoto,
		Filename:    "avatar.jpg",
		ContentType: "image/jpeg",
		File:        jpegPayload(),
	}, http.StatusUnprocessableEntity)
}

type uploadHTTPInput struct {
	Token       string
	Service     string
	OwnerID     string
	Purpose     string
	Filename    string
	ContentType string
	File        []byte
}

func newHTTPTestRouter(maxUploadBytes int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.ErrorHandler())

	service := uploadusecases.NewUploadService(uploadusecases.Options{
		Storage:        &httpFakeStorage{},
		Assets:         newHTTPFakeAssetRepository(),
		MaxUploadBytes: maxUploadBytes,
	})
	RegisterUploadRoutes(router.Group("/api/v1/media-files"), service, map[string]string{
		"customer-service": "test-token",
	}, maxUploadBytes)

	return router
}

func uploadMultipart(t *testing.T, router *gin.Engine, input uploadHTTPInput, expectedStatus int) map[string]interface{} {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("owner_service", input.Service)
	_ = writer.WriteField("owner_id", input.OwnerID)
	_ = writer.WriteField("purpose", input.Purpose)
	_ = writer.WriteField("metadata", `{"slot":"primary"}`)
	part, err := writer.CreatePart(fileHeader(input.Filename, input.ContentType))
	if err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	if _, err := part.Write(input.File); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/media-files/uploads", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Karrygo-Service", input.Service)
	request.Header.Set("Authorization", "Bearer "+input.Token)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	if response.Code != expectedStatus {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("Unmarshal() error = %v body=%s", err, response.Body.String())
	}

	return parsed
}

func fileHeader(filename string, contentType string) textproto.MIMEHeader {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	return header
}

func jpegPayload() []byte {
	return []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xff, 0xd9}
}

type httpFakeStorage struct{}

func (s *httpFakeStorage) Upload(ctx context.Context, input uploadclients.UploadObjectInput) (uploadclients.StorageObject, error) {
	var payload bytes.Buffer
	if _, err := payload.ReadFrom(input.Body); err != nil {
		return uploadclients.StorageObject{}, err
	}

	return uploadclients.StorageObject{
		Bucket: "test-bucket",
		Path:   input.Path,
		URL:    "https://storage.googleapis.com/test-bucket/" + input.Path,
	}, nil
}

func (s *httpFakeStorage) Delete(ctx context.Context, path string) error {
	return nil
}

func (s *httpFakeStorage) Check(ctx context.Context) error {
	return nil
}

type httpFakeAssetRepository struct {
	assets map[string]filemetadatamodels.MediaAsset
}

func newHTTPFakeAssetRepository() *httpFakeAssetRepository {
	return &httpFakeAssetRepository{assets: map[string]filemetadatamodels.MediaAsset{}}
}

func (r *httpFakeAssetRepository) Create(ctx context.Context, input filemetadatamodels.CreateMediaAssetInput) (filemetadatamodels.MediaAsset, error) {
	asset := filemetadatamodels.MediaAsset{
		ID:                input.ID,
		OwnerService:      input.OwnerService,
		OwnerID:           input.OwnerID,
		Purpose:           input.Purpose,
		OriginalFilename:  input.OriginalFilename,
		ContentType:       input.ContentType,
		SizeBytes:         input.SizeBytes,
		ChecksumSHA256:    input.ChecksumSHA256,
		StorageBucket:     input.StorageBucket,
		StoragePath:       input.StoragePath,
		PublicURL:         input.PublicURL,
		Status:            filemetadatamodels.StatusActive,
		Metadata:          input.Metadata,
		UploadedByService: input.UploadedByService,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	r.assets[asset.ID] = asset
	return asset, nil
}

func (r *httpFakeAssetRepository) GetByID(ctx context.Context, id string) (filemetadatamodels.MediaAsset, error) {
	return r.assets[id], nil
}

func (r *httpFakeAssetRepository) List(ctx context.Context, filter filemetadatamodels.ListMediaAssetsFilter) ([]filemetadatamodels.MediaAsset, error) {
	assets := []filemetadatamodels.MediaAsset{}
	for _, asset := range r.assets {
		assets = append(assets, asset)
	}
	return assets, nil
}

func (r *httpFakeAssetRepository) MarkDeleted(ctx context.Context, id string) error {
	asset := r.assets[id]
	now := time.Now()
	asset.Status = filemetadatamodels.StatusDeleted
	asset.DeletedAt = &now
	r.assets[id] = asset
	return nil
}
