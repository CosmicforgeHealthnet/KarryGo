package uploadusecases

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	filemetadatamodels "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/models"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
	"cosmicforge/logistics/shared/go/apperrors"
)

func TestUploadStoresObjectAndMetadata(t *testing.T) {
	storage := &fakeStorage{}
	repo := newFakeAssetRepository()
	service := NewUploadService(Options{
		Storage:        storage,
		Assets:         repo,
		MaxUploadBytes: 1024,
	})

	asset, err := service.Upload(context.Background(), UploadInput{
		CallerService:    "customer-service",
		OwnerService:     "customer-service",
		OwnerID:          "customer-1",
		Purpose:          filemetadatamodels.PurposeProfilePhoto,
		OriginalFilename: "../avatar.jpg",
		ContentType:      "image/jpeg",
		Body:             bytes.NewReader(jpegBytes()),
		Metadata:         map[string]interface{}{"slot": "primary"},
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if asset.ID == "" || asset.PublicURL == "" {
		t.Fatalf("expected stored asset with url, got %+v", asset)
	}
	if !strings.Contains(asset.StoragePath, "media/customer-service/profile_photo/customer-1/") {
		t.Fatalf("unexpected storage path: %s", asset.StoragePath)
	}
	if asset.OriginalFilename != "avatar.jpg" {
		t.Fatalf("expected sanitized filename, got %s", asset.OriginalFilename)
	}
	if len(storage.uploaded) != 1 {
		t.Fatalf("expected one uploaded object, got %d", len(storage.uploaded))
	}
}

func TestUploadRejectsOwnerServiceMismatch(t *testing.T) {
	service := NewUploadService(Options{
		Storage:        &fakeStorage{},
		Assets:         newFakeAssetRepository(),
		MaxUploadBytes: 1024,
	})

	_, err := service.Upload(context.Background(), UploadInput{
		CallerService:    "customer-service",
		OwnerService:     "taxi-service",
		OwnerID:          "customer-1",
		Purpose:          filemetadatamodels.PurposeProfilePhoto,
		OriginalFilename: "avatar.jpg",
		ContentType:      "image/jpeg",
		Body:             bytes.NewReader(jpegBytes()),
	})
	assertValidation(t, err)
}

func TestUploadRejectsUnsupportedContentType(t *testing.T) {
	service := NewUploadService(Options{
		Storage:        &fakeStorage{},
		Assets:         newFakeAssetRepository(),
		MaxUploadBytes: 1024,
	})

	_, err := service.Upload(context.Background(), UploadInput{
		CallerService:    "customer-service",
		OwnerService:     "customer-service",
		OwnerID:          "customer-1",
		Purpose:          filemetadatamodels.PurposeProfilePhoto,
		OriginalFilename: "notes.txt",
		ContentType:      "text/plain",
		Body:             strings.NewReader("plain text"),
	})
	assertValidation(t, err)
}

func TestUploadRejectsOversizedFile(t *testing.T) {
	service := NewUploadService(Options{
		Storage:        &fakeStorage{},
		Assets:         newFakeAssetRepository(),
		MaxUploadBytes: 4,
	})

	_, err := service.Upload(context.Background(), UploadInput{
		CallerService:    "customer-service",
		OwnerService:     "customer-service",
		OwnerID:          "customer-1",
		Purpose:          filemetadatamodels.PurposeProfilePhoto,
		OriginalFilename: "avatar.jpg",
		ContentType:      "image/jpeg",
		SizeBytes:        int64(len(jpegBytes())),
		Body:             bytes.NewReader(jpegBytes()),
	})
	assertValidation(t, err)
}

func assertValidation(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeValidationFailed {
		t.Fatalf("expected validation_failed error, got %v", err)
	}
}

func jpegBytes() []byte {
	return []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xff, 0xd9}
}

type fakeStorage struct {
	uploaded map[string][]byte
	deleted  map[string]bool
}

func (s *fakeStorage) Upload(ctx context.Context, input uploadclients.UploadObjectInput) (uploadclients.StorageObject, error) {
	if s.uploaded == nil {
		s.uploaded = map[string][]byte{}
	}
	var payload bytes.Buffer
	if _, err := payload.ReadFrom(input.Body); err != nil {
		return uploadclients.StorageObject{}, err
	}
	s.uploaded[input.Path] = payload.Bytes()
	return uploadclients.StorageObject{
		Bucket: "test-bucket",
		Path:   input.Path,
		URL:    "https://storage.googleapis.com/test-bucket/" + input.Path,
	}, nil
}

func (s *fakeStorage) Delete(ctx context.Context, path string) error {
	if s.deleted == nil {
		s.deleted = map[string]bool{}
	}
	s.deleted[path] = true
	return nil
}

func (s *fakeStorage) Check(ctx context.Context) error {
	return nil
}

type fakeAssetRepository struct {
	assets map[string]filemetadatamodels.MediaAsset
}

func newFakeAssetRepository() *fakeAssetRepository {
	return &fakeAssetRepository{assets: map[string]filemetadatamodels.MediaAsset{}}
}

func (r *fakeAssetRepository) Create(ctx context.Context, input filemetadatamodels.CreateMediaAssetInput) (filemetadatamodels.MediaAsset, error) {
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

func (r *fakeAssetRepository) GetByID(ctx context.Context, id string) (filemetadatamodels.MediaAsset, error) {
	return r.assets[id], nil
}

func (r *fakeAssetRepository) List(ctx context.Context, filter filemetadatamodels.ListMediaAssetsFilter) ([]filemetadatamodels.MediaAsset, error) {
	assets := []filemetadatamodels.MediaAsset{}
	for _, asset := range r.assets {
		assets = append(assets, asset)
	}
	return assets, nil
}

func (r *fakeAssetRepository) MarkDeleted(ctx context.Context, id string) error {
	asset := r.assets[id]
	now := time.Now()
	asset.Status = filemetadatamodels.StatusDeleted
	asset.DeletedAt = &now
	r.assets[id] = asset
	return nil
}
