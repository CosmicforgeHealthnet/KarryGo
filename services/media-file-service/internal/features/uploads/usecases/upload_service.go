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
	storage                 uploadclients.ObjectStorage
	assets                  filemetadatarepositories.MediaAssetRepository
	governance              filemetadatarepositories.LegacyGovernance
	scanner                 LegacyScanner
	publicSensitiveDownload bool
	maxUploadBytes          int64
	now                     func() time.Time
}

// LegacyScanner is the malware port for legacy media_assets reads. Legacy
// uploads were never scanned, so every sensitive legacy read is scanned at
// read time. A nil scanner, a scanner error or an unverified verdict all
// deny the read (Section 19 fail-closed).
type LegacyScanner interface {
	ScanLegacy(ctx context.Context, assetID string, content []byte) (safe bool, err error)
}

type Options struct {
	Storage        uploadclients.ObjectStorage
	Assets         filemetadatarepositories.MediaAssetRepository
	MaxUploadBytes int64
	// Governance enables adoption lookup, quarantine and tombstoning. Without
	// it, sensitive legacy reads are refused (adoption cannot be checked).
	Governance filemetadatarepositories.LegacyGovernance
	// Scanner verifies sensitive legacy bytes at read time.
	Scanner LegacyScanner
	// PublicSensitiveDownload (MEDIA_LEGACY_PUBLIC_SENSITIVE_DOWNLOAD_ENABLED,
	// default false) re-opens the unauthenticated /:id/download route for
	// non-profile-photo purposes. Even then adoption, integrity and malware
	// checks apply and the bytes are never served inline.
	PublicSensitiveDownload bool
}

func NewUploadService(opts Options) *UploadService {
	maxUploadBytes := opts.MaxUploadBytes
	if maxUploadBytes <= 0 {
		maxUploadBytes = defaultMaxUploadBytes
	}

	return &UploadService{
		storage:                 opts.Storage,
		assets:                  opts.Assets,
		governance:              opts.Governance,
		scanner:                 opts.Scanner,
		publicSensitiveDownload: opts.PublicSensitiveDownload,
		maxUploadBytes:          maxUploadBytes,
		now:                     time.Now,
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
		Public: isPublicPurpose(input.Purpose),
	})
	if errors.Is(err, errUploadTooLarge) {
		return filemetadatamodels.MediaAsset{}, validationError("file", fmt.Sprintf("File must be %d bytes or smaller.", s.maxUploadBytes))
	}
	if err != nil {
		return filemetadatamodels.MediaAsset{}, apperrors.Unavailable("File storage is temporarily unavailable.", err)
	}

	// Profile photos are shown to other people, so they get a public link:
	// the storage's public token link when there is one, otherwise the
	// public /:id/download route. Documents, proof photos and signatures are
	// never public (Section 19): their reference is the authenticated owner
	// content route, which requires the owner service's credentials.
	downloadURL := fmt.Sprintf("/files/%s/content", assetID)
	if isPublicPurpose(input.Purpose) {
		downloadURL = fmt.Sprintf("/%s/download", assetID)
		if strings.HasPrefix(object.URL, "https://") {
			downloadURL = object.URL
		}
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
		PublicURL:         downloadURL,
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

// Delete is the owner's privacy deletion. With governance configured it is a
// redacting tombstone that honours any recorded legal hold or retention and
// leaves an immutable event row; legacy rows carry no hold by default.
func (s *UploadService) Delete(ctx context.Context, actorService string, id string) error {
	asset, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if s.governance != nil {
		err := s.governance.Tombstone(ctx, asset.ID, actorService, "OWNER_DELETION", func(ctx context.Context, storagePath string) error {
			return s.storage.Delete(ctx, storagePath)
		})
		if errors.Is(err, filemetadatarepositories.ErrLegacyDeletionBlocked) {
			return apperrors.New(nethttp.StatusConflict, "deletion_blocked", "This file is under a legal hold or retention period and cannot be deleted.", err)
		}
		return err
	}
	if err := s.storage.Delete(ctx, asset.StoragePath); err != nil {
		return apperrors.Unavailable("File storage is temporarily unavailable.", err)
	}

	return s.assets.MarkDeleted(ctx, id)
}

// DownloadOutput carries verified bytes and the response policy for them.
type DownloadOutput struct {
	ContentType  string
	Size         int64
	Reader       io.ReadCloser
	Inline       bool
	CacheControl string
}

// PublicDownload serves the unauthenticated legacy route (/:id/download).
// Only profile photos are public by default; every other purpose answers
// not-found unless MEDIA_LEGACY_PUBLIC_SENSITIVE_DOWNLOAD_ENABLED is set.
func (s *UploadService) PublicDownload(ctx context.Context, id string) (DownloadOutput, error) {
	asset, err := s.legacyAsset(ctx, id)
	if err != nil {
		return DownloadOutput{}, err
	}
	if !isPublicPurpose(asset.Purpose) && !s.publicSensitiveDownload {
		// Same answer as an unknown id: a copied link does not confirm that a
		// private document exists.
		return DownloadOutput{}, apperrors.NotFound("Media file could not be found.", nil)
	}
	return s.serveLegacy(ctx, asset)
}

// OwnerDownload serves a legacy asset to its authenticated owner service.
func (s *UploadService) OwnerDownload(ctx context.Context, ownerService string, ownerID string, id string) (DownloadOutput, error) {
	asset, err := s.legacyAsset(ctx, id)
	if err != nil {
		return DownloadOutput{}, err
	}
	if asset.OwnerService != ownerService || ownerID != "" && asset.OwnerID != ownerID {
		return DownloadOutput{}, apperrors.Forbidden("You do not have access to this file.", nil)
	}
	return s.serveLegacy(ctx, asset)
}

func (s *UploadService) legacyAsset(ctx context.Context, id string) (filemetadatamodels.MediaAsset, error) {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return filemetadatamodels.MediaAsset{}, apperrors.NotFound("Media file could not be found.", nil)
	}
	asset, err := s.assets.GetByID(ctx, id)
	if err != nil {
		return filemetadatamodels.MediaAsset{}, err
	}
	if asset.Status != filemetadatamodels.StatusActive {
		return filemetadatamodels.MediaAsset{}, apperrors.NotFound("Media file could not be found.", nil)
	}
	return asset, nil
}

var errNotDownloadable = apperrors.New(nethttp.StatusForbidden, "not_downloadable", "This file cannot be released.", nil)

// serveLegacy applies the Section 19 floor to every legacy read:
//   - an adopted asset is served only through its private modern object;
//   - the stored bytes must match the recorded size and checksum and sniff
//     as the recorded content type (no MIME or length substitution);
//   - every purpose except profile photos is high-risk and must be scanned
//     SAFE at read time, failing closed when no scanner answers;
//   - only verified raster profile photos are ever served inline; SVG and
//     every other type are an attachment, never cached.
func (s *UploadService) serveLegacy(ctx context.Context, asset filemetadatamodels.MediaAsset) (DownloadOutput, error) {
	public := isPublicPurpose(asset.Purpose)
	if s.governance == nil {
		if !public {
			return DownloadOutput{}, errNotDownloadable
		}
	} else {
		_, adopted, err := s.governance.AdoptedObject(ctx, asset.ID)
		if err != nil {
			return DownloadOutput{}, apperrors.Unavailable("File metadata is temporarily unavailable.", err)
		}
		if adopted {
			return DownloadOutput{}, apperrors.New(nethttp.StatusGone, "legacy_asset_adopted",
				"This file is now held as private evidence and is only available through an authorized private grant.", nil)
		}
	}

	reader, err := s.storage.Download(ctx, asset.StoragePath)
	if err != nil {
		return DownloadOutput{}, apperrors.Unavailable("File storage is temporarily unavailable.", err)
	}
	content, err := io.ReadAll(io.LimitReader(reader, s.maxUploadBytes+1))
	reader.Close()
	if err != nil {
		return DownloadOutput{}, apperrors.Unavailable("File storage is temporarily unavailable.", err)
	}
	sum := sha256.Sum256(content)
	if int64(len(content)) != asset.SizeBytes || !strings.EqualFold(hex.EncodeToString(sum[:]), asset.ChecksumSHA256) {
		return DownloadOutput{}, apperrors.New(nethttp.StatusConflict, "integrity_check_failed", "This file failed an integrity check.", nil)
	}
	head := content
	if len(head) > 512 {
		head = head[:512]
	}
	recorded := normalizeContentType(asset.ContentType)
	if !contentMatchesRecordedType(recorded, head) {
		return DownloadOutput{}, apperrors.New(nethttp.StatusConflict, "integrity_check_failed", "This file failed an integrity check.", nil)
	}

	if !public {
		if s.scanner == nil {
			return DownloadOutput{}, errNotDownloadable
		}
		safe, err := s.scanner.ScanLegacy(ctx, asset.ID, content)
		if err != nil {
			return DownloadOutput{}, errNotDownloadable
		}
		if !safe {
			if s.governance != nil {
				_ = s.governance.Quarantine(ctx, asset.ID)
			}
			return DownloadOutput{}, errNotDownloadable
		}
	}

	out := DownloadOutput{
		ContentType:  recorded,
		Size:         int64(len(content)),
		Reader:       io.NopCloser(bytes.NewReader(content)),
		CacheControl: "private, no-store",
	}
	if public && isInlineImage(recorded) {
		out.Inline = true
		out.CacheControl = "public, max-age=300"
	}
	if recorded == "image/svg+xml" {
		// Legacy signatures may be SVG. Never let a browser render them.
		out.ContentType = "application/octet-stream"
	}
	return out, nil
}

func isInlineImage(contentType string) bool {
	return contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/webp"
}

// contentMatchesRecordedType sniffs the stored bytes and requires them to be
// what the row claims. SVG (legacy signatures only) is checked textually.
func contentMatchesRecordedType(recorded string, head []byte) bool {
	if recorded == "image/svg+xml" {
		return looksLikeSVG(head)
	}
	detected := normalizeContentType(nethttp.DetectContentType(head))
	switch recorded {
	case "image/jpeg", "image/png", "image/webp", "application/pdf":
		return detected == recorded
	default:
		return false
	}
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
	} else if !validPathToken(input.OwnerID) {
		// owner_id becomes a storage path segment. Anything but an opaque
		// token could traverse or rewrite the object key.
		fields = append(fields, apperrors.FieldViolation{Field: "owner_id", Message: "Owner id must be an opaque identifier."})
	}
	if strings.TrimSpace(input.OwnerService) != "" && !validPathToken(input.OwnerService) {
		fields = append(fields, apperrors.FieldViolation{Field: "owner_service", Message: "Owner service must be an opaque identifier."})
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
		// SVG is active content (scripts, external references). New
		// signature uploads must be raster images (Section 19).
		return image
	default:
		return false
	}
}

var pathTokenPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$`)

// validPathToken accepts an opaque identifier safe to use as one storage path
// segment: no separators, no traversal, no whitespace or control characters.
func validPathToken(value string) bool {
	return pathTokenPattern.MatchString(value) && !strings.Contains(value, "..")
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

// isPublicPurpose: only profile photos get a public link.
func isPublicPurpose(purpose string) bool {
	return purpose == filemetadatamodels.PurposeProfilePhoto
}
