// Package provideruploadsusecases lets a signed-in driver upload their own
// documents and photos. The driver app never talks to media-file-service:
// this service checks the file and forwards it under its own credentials,
// with the driver's id as the owner, so no media credential ships in the app.
package provideruploadsusecases

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/mediaclient"
)

// MaxUploadBytes caps one upload. Phone photos of documents are a few MB.
const MaxUploadBytes = 10 << 20

// Purposes a driver may upload, matching media-file-service's purposes.
var allowedPurposes = map[string]bool{
	"profile_photo": true,
	"document_file": true,
	"proof_image":   true,
	"signature":     true,
}

// Content types are decided from the file's own bytes, never the name or
// header the client sent.
var allowedContentTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/webp":      true,
	"application/pdf": true,
}

// MediaUploader is the part of mediaclient.Client this feature needs.
type MediaUploader interface {
	Upload(ctx context.Context, upload mediaclient.UploadRequest) (mediaclient.MediaAsset, error)
}

type UploadService struct {
	media MediaUploader
}

// NewUploadService returns nil when media is not configured, so the routes
// can report the feature unavailable instead of failing every request.
func NewUploadService(media MediaUploader) *UploadService {
	return &UploadService{media: media}
}

type UploadInput struct {
	ProviderID string
	Purpose    string
	Filename   string
	Body       io.Reader
}

type UploadResult struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func (s *UploadService) Upload(ctx context.Context, in UploadInput) (UploadResult, error) {
	if s == nil || s.media == nil {
		return UploadResult{}, apperrors.Unavailable("Uploads are not available right now.", nil)
	}
	if in.ProviderID == "" {
		return UploadResult{}, apperrors.Unauthorized("Authentication is required.", nil)
	}
	if !allowedPurposes[in.Purpose] {
		return UploadResult{}, apperrors.Validation("Check your upload.", []apperrors.FieldViolation{
			{Field: "purpose", Message: "Purpose is not supported."},
		})
	}

	// Read one byte past the limit so an oversized file is refused rather
	// than silently truncated.
	data, err := io.ReadAll(io.LimitReader(in.Body, MaxUploadBytes+1))
	if err != nil {
		return UploadResult{}, apperrors.BadRequest("The file could not be read.", err)
	}
	if len(data) == 0 {
		return UploadResult{}, apperrors.Validation("Check your upload.", []apperrors.FieldViolation{
			{Field: "file", Message: "The file is empty."},
		})
	}
	if len(data) > MaxUploadBytes {
		return UploadResult{}, apperrors.Validation("Check your upload.", []apperrors.FieldViolation{
			{Field: "file", Message: "The file is larger than 10 MB."},
		})
	}
	contentType := http.DetectContentType(data)
	if !allowedContentTypes[contentType] {
		return UploadResult{}, apperrors.Validation("Check your upload.", []apperrors.FieldViolation{
			{Field: "file", Message: "Upload a JPEG, PNG, WebP or PDF file."},
		})
	}

	filename := in.Filename
	if filename == "" {
		filename = "upload"
	}
	asset, err := s.media.Upload(ctx, mediaclient.UploadRequest{
		OwnerID:     in.ProviderID,
		Purpose:     in.Purpose,
		Filename:    filename,
		ContentType: contentType,
		Body:        bytes.NewReader(data),
	})
	if err != nil {
		return UploadResult{}, apperrors.Unavailable("The file could not be uploaded. Please try again.", err)
	}
	return UploadResult{ID: asset.ID, URL: asset.PublicURL}, nil
}
