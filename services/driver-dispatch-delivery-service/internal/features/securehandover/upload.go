package securehandover

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/mediaclient"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxCaptureBytes = 5 << 20

func (h *Handler) uploadEvidence(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCaptureBytes+(1<<20))
	header, err := c.FormFile("photo")
	if err != nil {
		respond(c, 0, nil, apperrors.BadRequest("Choose a JPG or PNG photo smaller than 5 MB.", err))
		return
	}
	file, err := header.Open()
	if err != nil {
		respond(c, 0, nil, apperrors.BadRequest("The photo could not be read.", err))
		return
	}
	defer file.Close()
	result, err := h.service.UploadCapture(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("id"), c.Param("stopId"), c.PostForm("capture_session_ref"), file, header)
	respond(c, http.StatusCreated, gin.H{"media_ref": result}, err)
}

func captureContent(file io.Reader, size int64) ([]byte, string, error) {
	if size <= 0 || size > maxCaptureBytes {
		return nil, "", apperrors.BadRequest("Photo size must be between 1 byte and 5 MB.", nil)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCaptureBytes+1))
	if err != nil || int64(len(data)) != size {
		return nil, "", apperrors.BadRequest("The photo could not be read completely.", err)
	}
	mime := http.DetectContentType(data)
	if mime != "image/jpeg" && mime != "image/png" {
		return nil, "", apperrors.BadRequest("Choose a JPG or PNG photo.", nil)
	}
	return data, mime, nil
}

func (s *Service) UploadCapture(ctx context.Context, providerID, tripID, stopRef, sessionRef string, file multipart.File, header *multipart.FileHeader) (string, error) {
	if !s.Enabled() || s.uploads == nil {
		return "", apperrors.Unavailable("Evidence uploads are not configured.", nil)
	}
	if header == nil {
		return "", apperrors.BadRequest("Choose a photo.", nil)
	}
	if err := s.limit(ctx, "capture-upload:"+providerID, 12, time.Minute); err != nil {
		return "", err
	}
	stage, err := s.repo.CaptureSessionStage(ctx, sessionRef)
	if err != nil {
		return "", mapError(err)
	}
	bound, err := s.repo.ResolveByTrip(ctx, tripID, stage, stopRef)
	if err != nil || bound.ProviderID != providerID {
		return "", apperrors.NotFound("Assigned evidence context was not found.", err)
	}
	var evidenceType string
	err = s.repo.db.QueryRow(ctx, `SELECT COALESCE((SELECT s.evidence_type FROM dispatch_evidence_capture_sessions s
		JOIN dispatch_handover_challenges c ON c.id=s.challenge_id
		WHERE s.capture_session_ref=$1 AND s.trip_id=$2::uuid AND s.provider_id=$3::uuid
		AND COALESCE(s.mission_id::text,'')=$4 AND COALESCE(s.stop_id::text,'')=$5
		AND s.manifest_ref=$6 AND s.manifest_version=$7 AND s.stage=$8
		AND s.evidence_type IN ('PHOTO','SIGNATURE') AND s.status='ACTIVE' AND s.expires_at>$9 AND c.status='CONSUMED'),'')`,
		sessionRef, tripID, providerID, bound.MissionID, bound.StopID, bound.ManifestRef, bound.ManifestVersion, stage, s.now()).Scan(&evidenceType)
	if err != nil {
		return "", err
	}
	if evidenceType == "" {
		return "", apperrors.Conflict("This capture session is no longer available. Start a new capture.", nil)
	}
	purpose := "proof_image"
	if evidenceType == "SIGNATURE" {
		purpose = "signature"
	}
	data, mime, err := captureContent(file, header.Size)
	if err != nil {
		return "", err
	}
	owner := bound.MissionRef
	if bound.MissionID == "" {
		owner = bound.TripID
	}
	parcels, err := s.repo.ParcelRefs(ctx, bound)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	checksum := hex.EncodeToString(digest[:])
	assets, err := s.uploads.List(ctx, mediaclient.ListFilter{OwnerID: owner, Purpose: purpose})
	if err != nil {
		return "", apperrors.Unavailable("Evidence storage could not be reached.", err)
	}
	for _, asset := range assets {
		if asset.OwnerService == "dispatch-delivery-service" && asset.OwnerID == owner && asset.Status == "active" &&
			asset.Metadata["capture_session_ref"] == sessionRef && asset.ChecksumSHA256 == checksum {
			return asset.ID, nil
		}
	}
	asset, err := s.uploads.Upload(ctx, mediaclient.UploadRequest{OwnerID: owner, Purpose: purpose,
		Filename: uuid.NewString() + map[string]string{"image/png": ".png", "image/jpeg": ".jpg"}[mime], ContentType: mime, Body: bytes.NewReader(data), Metadata: map[string]interface{}{
			"mission_ref": owner, "parcel_refs": parcels, "capture_session_ref": sessionRef,
			"provider_id": providerID, "trip_id": tripID,
		}})
	if err != nil {
		return "", apperrors.Unavailable("The photo upload could not be confirmed. Retry this capture.", err)
	}
	return asset.ID, nil
}
