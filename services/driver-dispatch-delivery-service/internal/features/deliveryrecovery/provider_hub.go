package deliveryrecovery

import (
	"bytes"
	"context"
	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/evidenceimage"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/mediaclient"
	"crypto/sha256"
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"strings"
)

type ProviderHubHold struct {
	HubRef       string   `json:"hub_ref"`
	State        string   `json:"state"`
	ManifestRef  string   `json:"manifest_ref"`
	ParcelRefs   []string `json:"parcel_refs"`
	ParcelCodes  []string `json:"parcel_codes"`
	AllowedStage string   `json:"allowed_stage,omitempty"`
}

func hubStage(state string) string {
	switch state {
	case "APPROVED", "EN_ROUTE", "CUSTODY_PENDING":
		return "ARRIVAL"
	case "HELD", "PICKUP_PENDING":
		return "RELEASE"
	}
	return ""
}
func (s *Service) providerHub(ctx context.Context, providerID, tripID string) (HubCheckpointContext, error) {
	if s == nil || s.repo == nil || !s.policy.HubHoldEnabled {
		return HubCheckpointContext{}, apperrors.Unavailable("Approved hub hold is unavailable.", nil)
	}
	if _, err := uuid.Parse(tripID); err != nil {
		return HubCheckpointContext{}, apperrors.BadRequest("Delivery reference is invalid.", nil)
	}
	return s.repo.HubCheckpointContext(ctx, HubCheckpointCommand{TripID: tripID, ProviderID: providerID})
}
func (h *Handler) readHub(c *gin.Context) {
	item, err := h.service.providerHub(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ProviderHubHold{HubRef: item.HubRef, State: item.State, ManifestRef: item.ManifestRef, ParcelRefs: item.ParcelRefs, ParcelCodes: item.ParcelCodes, AllowedStage: hubStage(item.State)}})
}
func (h *Handler) uploadHubEvidence(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, evidenceimage.MaxBytes+(1<<20))
	file, err := c.FormFile("photo")
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("Choose a photo smaller than 5 MB.", err))
		return
	}
	defer func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()
	operation := strings.TrimSpace(c.PostForm("operation_ref"))
	if operation == "" || len(operation) > 128 || c.GetHeader("Idempotency-Key") != operation {
		httpx.Abort(c, apperrors.BadRequest("A matching upload request reference is required.", nil))
		return
	}
	item, err := h.service.providerHub(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if hubStage(item.State) == "" || c.PostForm("manifest_ref") != item.ManifestRef {
		httpx.Abort(c, apperrors.Conflict("Hub custody or its manifest changed. Refresh this delivery.", nil))
		return
	}
	if h.service.uploads == nil {
		httpx.Abort(c, apperrors.Unavailable("Hub evidence uploads are unavailable.", nil))
		return
	}
	input, err := file.Open()
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("The photo could not be read.", err))
		return
	}
	defer input.Close()
	data, mime, err := evidenceimage.Read(input, file.Size)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	provider := authhttp.DispatchRiderID(c)
	assets, err := h.service.uploads.List(c.Request.Context(), mediaclient.ListFilter{OwnerID: item.MissionRef, Purpose: "proof_image"})
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Hub evidence storage could not be reached.", err))
		return
	}
	for _, asset := range assets {
		if asset.OwnerID == item.MissionRef && asset.OwnerService == "dispatch-delivery-service" && asset.Metadata["provider_id"] == provider && asset.Metadata["operation_ref"] == operation {
			if asset.ChecksumSHA256 != checksum || asset.Metadata["manifest_ref"] != item.ManifestRef {
				httpx.Abort(c, apperrors.Conflict("This upload reference belongs to different evidence.", nil))
				return
			}
			if asset.ID != "" && strings.EqualFold(asset.Status, "active") {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"media_ref": asset.ID}})
				return
			}
			httpx.Abort(c, apperrors.Unavailable("The photo is still being processed. Retry shortly.", nil))
			return
		}
	}
	extension := ".jpg"
	if mime == "image/png" {
		extension = ".png"
	}
	asset, err := h.service.uploads.Upload(c.Request.Context(), mediaclient.UploadRequest{OwnerID: item.MissionRef, Purpose: "proof_image", Filename: uuid.NewString() + extension, ContentType: mime, Body: bytes.NewReader(data), Metadata: map[string]interface{}{"mission_ref": item.MissionRef, "parcel_refs": item.ParcelRefs, "manifest_ref": item.ManifestRef, "provider_id": provider, "operation_ref": operation}})
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Hub photo upload could not be confirmed. Retry the same photo.", err))
		return
	}
	if asset.ID == "" || asset.OwnerID != item.MissionRef || asset.OwnerService != "dispatch-delivery-service" || !strings.EqualFold(asset.Status, "active") {
		httpx.Abort(c, apperrors.Unavailable("Hub photo ownership or processing could not be confirmed. Retry shortly.", nil))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"media_ref": asset.ID}})
}
