package uploadusecases

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	nethttp "net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	filemetadatamodels "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/models"
	filemetadatarepositories "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/repositories"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
	"cosmicforge/logistics/shared/go/apperrors"
)

const defaultMaxUploadBytes int64 = 25 * 1024 * 1024

var errUploadTooLarge = errors.New("upload exceeds maximum size")

type UploadService struct {
	storage        uploadclients.ObjectStorage
	assets         filemetadatarepositories.MediaAssetRepository
	maxUploadBytes int64
	now            func() time.Time
}

type Options struct {
	Storage        uploadclients.ObjectStorage
	Assets         filemetadatarepositories.MediaAssetRepository
	MaxUploadBytes int64
}

func NewUploadService(opts Options) *UploadService {
	maxUploadBytes := opts.MaxUploadBytes
	if maxUploadBytes <= 0 {
		maxUploadBytes = defaultMaxUploadBytes
	}

	return &UploadService{
		storage:        opts.Storage,
		assets:         opts.Assets,
		maxUploadBytes: maxUploadBytes,
		now:            time.Now,
	}
}

type UploadInput struct {
	CallerService    string
	OwnerService     string
	OwnerID          string
	Purpose          string
	OriginalFilename string
	ContentType      string
	SizeBytes        int64
	Body             io.Reader
	Metadata         map[string]interface{}
}

func (s *UploadService) Upload(ctx context.Context, input UploadInput) (filemetadatamodels.MediaAsset, error) {
	if err := s.validateUploadInput(input); err != nil {
		return filemetadatamodels.MediaAsset{}, err
	}

	head, body, err := readHead(input.Body)
	if err != nil {
		return filemetadatamodels.MediaAsset{}, err
	}
	if len(head) == 0 {
		return filemetadatamodels.MediaAsset{}, validationError("file", "File is required.")
	}

	contentType := detectContentType(input.ContentType, head)
	if !isAllowedContentType(input.Purpose, contentType) {
		return filemetadatamodels.MediaAsset{}, validationError("file", "File type is not supported for this purpose.")
	}

	assetID := uuid.NewString()
	filename := sanitizeFilename(input.OriginalFilename)
	storagePath := buildStoragePath(input.OwnerService, input.Purpose, input.OwnerID, assetID, filename)
	limiter := &maxBytesReader{
		reader: io.MultiReader(bytes.NewReader(head), body),
		max:    s.maxUploadBytes,
	}
	hasher := sha256.New()

	object, err := s.storage.Upload(ctx, uploadclients.UploadObjectInput{
		Path:        storagePath,
		Body:        io.TeeReader(limiter, hasher),
		ContentType: contentType,
		Metadata: map[string]string{
			"media_asset_id": assetID,
			"owner_service":  input.OwnerService,
			"owner_id":       input.OwnerID,
			"purpose":        input.Purpose,
		},
	})
	if errors.Is(err, errUploadTooLarge) {
		return filemetadatamodels.MediaAsset{}, validationError("file", fmt.Sprintf("File must be %d bytes or smaller.", s.maxUploadBytes))
	}
	if err != nil {
		return filemetadatamodels.MediaAsset{}, apperrors.Unavailable("File storage is temporarily unavailable.", err)
	}

	asset, err := s.assets.Create(ctx, filemetadatamodels.CreateMediaAssetInput{
		ID:                assetID,
		OwnerService:      input.OwnerService,
		OwnerID:           input.OwnerID,
		Purpose:           input.Purpose,
		OriginalFilename:  filename,
		ContentType:       contentType,
		SizeBytes:         limiter.bytesRead,
		ChecksumSHA256:    hex.EncodeToString(hasher.Sum(nil)),
		StorageBucket:     object.Bucket,
		StoragePath:       object.Path,
		PublicURL:         object.URL,
		Metadata:          input.Metadata,
		UploadedByService: input.CallerService,
	})
	if err != nil {
		_ = s.storage.Delete(ctx, object.Path)
		return filemetadatamodels.MediaAsset{}, err
	}

	return asset, nil
}

func (s *UploadService) GetByID(ctx context.Context, id string) (filemetadatamodels.MediaAsset, error) {
	if strings.TrimSpace(id) == "" {
		return filemetadatamodels.MediaAsset{}, validationError("id", "Media file id is required.")
	}

	return s.assets.GetByID(ctx, id)
}

func (s *UploadService) List(ctx context.Context, filter filemetadatamodels.ListMediaAssetsFilter) ([]filemetadatamodels.MediaAsset, error) {
	if filter.OwnerService == "" && filter.OwnerID == "" && filter.Purpose == "" {
		return nil, validationError("owner_service", "At least one filter is required.")
	}
	if filter.Purpose != "" && !isAllowedPurpose(filter.Purpose) {
		return nil, validationError("purpose", "File purpose is not supported.")
	}

	return s.assets.List(ctx, filter)
}

func (s *UploadService) Delete(ctx context.Context, id string) error {
	asset, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, asset.StoragePath); err != nil {
		return apperrors.Unavailable("File storage is temporarily unavailable.", err)
	}

	return s.assets.MarkDeleted(ctx, id)
}

func (s *UploadService) validateUploadInput(input UploadInput) error {
	fields := []apperrors.FieldViolation{}
	if strings.TrimSpace(input.CallerService) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "service", Message: "Calling service is required."})
	}
	if strings.TrimSpace(input.OwnerService) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "owner_service", Message: "Owner service is required."})
	}
	if input.CallerService != "" && input.OwnerService != "" && input.CallerService != input.OwnerService {
		fields = append(fields, apperrors.FieldViolation{Field: "owner_service", Message: "Owner service must match the calling service."})
	}
	if strings.TrimSpace(input.OwnerID) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "owner_id", Message: "Owner id is required."})
	}
	if !isAllowedPurpose(input.Purpose) {
		fields = append(fields, apperrors.FieldViolation{Field: "purpose", Message: "File purpose is not supported."})
	}
	if input.Body == nil {
		fields = append(fields, apperrors.FieldViolation{Field: "file", Message: "File is required."})
	}
	if input.SizeBytes > s.maxUploadBytes {
		fields = append(fields, apperrors.FieldViolation{Field: "file", Message: fmt.Sprintf("File must be %d bytes or smaller.", s.maxUploadBytes)})
	}
	if len(fields) > 0 {
		return apperrors.Validation("Check your upload details.", fields)
	}

	return nil
}

func readHead(reader io.Reader) ([]byte, io.Reader, error) {
	buffer := make([]byte, 512)
	n, err := io.ReadFull(reader, buffer)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, nil, apperrors.BadRequest("File could not be read.", err)
	}

	return buffer[:n], reader, nil
}

func detectContentType(declared string, head []byte) string {
	detected := normalizeContentType(nethttp.DetectContentType(head))
	declared = normalizeContentType(declared)

	if declared == "image/svg+xml" && looksLikeSVG(head) {
		return declared
	}
	if detected == "application/octet-stream" && declared != "" {
		return declared
	}

	return detected
}

func normalizeContentType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(value)
	}

	return strings.ToLower(parsed)
}

func looksLikeSVG(head []byte) bool {
	value := strings.ToLower(string(head))
	return strings.Contains(value, "<svg")
}

func isAllowedPurpose(purpose string) bool {
	switch purpose {
	case filemetadatamodels.PurposeProfilePhoto,
		filemetadatamodels.PurposeDocumentFile,
		filemetadatamodels.PurposeProofImage,
		filemetadatamodels.PurposeSignature:
		return true
	default:
		return false
	}
}

func isAllowedContentType(purpose string, contentType string) bool {
	image := contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/webp"
	switch purpose {
	case filemetadatamodels.PurposeProfilePhoto, filemetadatamodels.PurposeProofImage:
		return image
	case filemetadatamodels.PurposeDocumentFile:
		return image || contentType == "application/pdf"
	case filemetadatamodels.PurposeSignature:
		return image || contentType == "image/svg+xml"
	default:
		return false
	}
}

func buildStoragePath(ownerService string, purpose string, ownerID string, assetID string, filename string) string {
	return path.Join("media", ownerService, purpose, ownerID, assetID, filename)
}

var filenamePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeFilename(filename string) string {
	filename = strings.ReplaceAll(filename, "\\", "/")
	filename = path.Base(filename)
	filename = strings.TrimSpace(filename)
	filename = filenamePattern.ReplaceAllString(filename, "-")
	filename = strings.Trim(filename, ".-")
	if filename == "" {
		return "upload"
	}

	return filename
}

func validationError(field string, message string) error {
	return apperrors.Validation("Check your upload details.", []apperrors.FieldViolation{
		{Field: field, Message: message},
	})
}

type maxBytesReader struct {
	reader    io.Reader
	max       int64
	bytesRead int64
}

func (r *maxBytesReader) Read(p []byte) (int, error) {
	remaining := r.max - r.bytesRead
	if remaining < 0 {
		return 0, errUploadTooLarge
	}
	readBuffer := p
	if int64(len(readBuffer)) > remaining+1 {
		readBuffer = readBuffer[:remaining+1]
	}

	n, err := r.reader.Read(readBuffer)
	if int64(n) > remaining {
		if remaining > 0 {
			copy(p, readBuffer[:remaining])
		}
		r.bytesRead += int64(n)
		return int(remaining), errUploadTooLarge
	}

	r.bytesRead += int64(n)
	return n, err
}
