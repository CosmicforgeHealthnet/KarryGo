package usecases

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	repos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// maxPreviewBytes matches the upload ceiling: no company document can be
// larger, so a longer response is a fault and is not buffered.
const maxPreviewBytes = 25 * 1024 * 1024

// previewableContentTypes are the types company documents may be uploaded as.
// Preview serves only these, as identified by Media's content detection.
var previewableContentTypes = map[string]string{
	"application/pdf": ".pdf",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
}

// DocumentPreview is the validated content of one attached company document.
type DocumentPreview struct {
	DocumentID   string
	DocumentType string
	ContentType  string
	Extension    string
	Content      []byte
}

// PreviewDocument returns the bytes of an attached company document so the
// organization's own users can view it.
//
// It is read-only with respect to the document: nothing about the attachment,
// its status or its Media object changes. Every gate fails closed:
//   - the document must belong to the caller's organization and be attached
//     (a removed or foreign document is indistinguishable from a missing one);
//   - Media must still report the object as owned by this organization,
//     completely uploaded, passed safety scanning and not deleted;
//   - Media must grant the download itself, so its own quarantine and
//     deletion floor applies whatever Fleet believes;
//   - the bytes must be one of the accepted document types, match Media's
//     detected type and, where Media recorded one, its SHA-256 checksum.
//
// The download grant is used once here and never returned or logged.
func (s *Service) PreviewDocument(ctx context.Context, t tenant.Context, documentID string) (*DocumentPreview, error) {
	if documentID = strings.TrimSpace(documentID); documentID == "" || len(documentID) > 128 {
		return nil, apperrors.NotFound("Document not found.", nil)
	}
	document, err := s.opts.Onboarding.GetDocument(ctx, t.OrganizationID, documentID)
	if errors.Is(err, repos.ErrNotFound) || (err == nil && document.Status == models.DocumentStatusRemoved) {
		return nil, apperrors.NotFound("Document not found.", nil)
	}
	if err != nil {
		return nil, err
	}
	if document.Status != models.DocumentStatusReady {
		return nil, apperrors.Conflict("This document is not ready to preview.", nil)
	}

	reader, ok := s.opts.Contracts.Media.(platformclients.MediaContentReader)
	if !ok || s.opts.Contracts.Media == nil || !s.opts.Contracts.Media.Configured() {
		return nil, previewUnavailable()
	}
	meta := platformclients.CallMeta{OrganizationID: t.OrganizationID, RequestID: t.RequestID, CorrelationID: t.CorrelationID}

	state, err := reader.ReadObject(ctx, meta, document.FileID)
	if err != nil {
		return nil, previewMediaError(err)
	}
	if state.OwnerReference != t.OrganizationID || state.OwnerType != "ORGANIZATION" ||
		(state.OrganizationID != "" && state.OrganizationID != t.OrganizationID) {
		// Never reveal a Media object that is not this organization's.
		return nil, apperrors.NotFound("Document not found.", nil)
	}
	if state.DeletionState != "" && state.DeletionState != "ACTIVE" {
		return nil, apperrors.NotFound("This document is no longer available.", nil)
	}
	file := platformclients.FileMetadata{UploadState: state.UploadStatus, ScanState: state.SafetyStatus, DeletionState: state.DeletionState}
	if !file.Ready() {
		return nil, apperrors.Conflict("This document has not passed upload and safety checks, so it cannot be previewed.", nil)
	}
	contentType := strings.ToLower(strings.TrimSpace(state.DetectedMIME))
	extension, supported := previewableContentTypes[contentType]
	if !supported {
		return nil, unsupportedPreview()
	}

	grant, err := reader.AuthorizeDownload(ctx, meta, document.FileID)
	if err != nil {
		return nil, previewMediaError(err)
	}
	if granted := strings.ToLower(strings.TrimSpace(grant.ContentType)); granted != "" && granted != contentType {
		return nil, unsupportedPreview()
	}
	content, err := reader.ReadContent(ctx, meta, grant, maxPreviewBytes+1)
	if err != nil {
		return nil, previewMediaError(err)
	}
	if len(content) == 0 || len(content) > maxPreviewBytes {
		return nil, unsupportedPreview()
	}
	if state.ChecksumSHA256 != "" {
		sum := sha256.Sum256(content)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), state.ChecksumSHA256) {
			return nil, apperrors.Conflict("This document failed an integrity check and cannot be previewed.", nil)
		}
	}
	if !contentMatches(contentType, content) {
		return nil, unsupportedPreview()
	}

	entry := audit.EntryFromTenant(t, "organization.document_previewed", "organization_document", document.ID)
	entry.NewValues = map[string]any{"documentType": document.DocumentType, "contentType": contentType}
	if err := s.opts.Audit.Record(ctx, s.opts.Pool, entry); err != nil {
		return nil, err
	}
	return &DocumentPreview{
		DocumentID: document.ID, DocumentType: document.DocumentType,
		ContentType: contentType, Extension: extension, Content: content,
	}, nil
}

// contentMatches confirms the bytes are what Media says they are, so a
// mislabelled object can never be handed to the browser as a renderable type.
func contentMatches(contentType string, content []byte) bool {
	switch contentType {
	case "application/pdf":
		return bytes.HasPrefix(content, []byte("%PDF-"))
	case "image/jpeg":
		return bytes.HasPrefix(content, []byte{0xFF, 0xD8, 0xFF})
	case "image/png":
		return bytes.HasPrefix(content, []byte("\x89PNG\r\n\x1a\n"))
	case "image/webp":
		return len(content) >= 12 && bytes.Equal(content[0:4], []byte("RIFF")) && bytes.Equal(content[8:12], []byte("WEBP"))
	}
	return false
}

func unsupportedPreview() error {
	return apperrors.New(http.StatusUnsupportedMediaType, apperrors.CodeBadRequest,
		"This document's format cannot be previewed.", nil)
}

func previewUnavailable() error {
	return errorcodes.Unavailable(errorcodes.MediaAuthorityUnavailable,
		"Document preview is temporarily unavailable. Try again shortly.")
}

func previewMediaError(err error) error {
	switch {
	case errors.Is(err, platformclients.ErrMediaObjectNotFound):
		return apperrors.NotFound("This document is no longer available.", nil)
	case errors.Is(err, platformclients.ErrMediaNotDownloadable):
		return apperrors.Conflict("The document service is not releasing this file. It may be under safety review.", nil)
	}
	var remote *platformclients.Error
	if errors.As(err, &remote) {
		switch remote.Outcome {
		case platformclients.OutcomeNotFound:
			return apperrors.NotFound("This document is no longer available.", nil)
		case platformclients.OutcomeForbidden:
			return apperrors.Conflict("The document service is not releasing this file. It may be under safety review.", nil)
		}
	}
	return previewUnavailable()
}
