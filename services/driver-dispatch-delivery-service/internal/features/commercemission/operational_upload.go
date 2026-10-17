package commercemission

import (
	"bytes"
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/evidenceimage"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/mediaclient"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const maxOperationalImage = 5 << 20

func operationalImage(reader io.Reader, size int64) ([]byte, string, error) {
	return evidenceimage.Read(reader, size)
}

func (h *OperationalHandler) uploadOperationalEvidence(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxOperationalImage+(1<<20))
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
	var parcels []string
	if json.Unmarshal([]byte(c.PostForm("parcel_refs")), &parcels) != nil || len(parcels) < 1 || len(parcels) > 100 {
		httpx.Abort(c, apperrors.BadRequest("Choose the parcels this photo belongs to.", nil))
		return
	}
	operation := strings.TrimSpace(c.PostForm("operation_ref"))
	if !validProviderOperationHeader(c, operation) {
		return
	}
	source, err := file.Open()
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("The photo could not be read.", err))
		return
	}
	defer source.Close()
	data, mime, err := operationalImage(source, file.Size)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	ref, err := h.service.UploadOperationalEvidence(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("missionRef"), operation, parcels, data, mime)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"media_ref": ref}})
}
func (s *OperationalService) UploadOperationalEvidence(ctx context.Context, providerID, missionRef, operation string, parcels []string, data []byte, mime string) (string, error) {
	if s == nil || s.repo == nil || s.uploads == nil {
		return "", apperrors.Unavailable("Evidence uploads are unavailable.", nil)
	}
	parcels = canonicalRefs(parcels)
	if len(parcels) == 0 || len(parcels) > 100 || operation == "" || len(operation) > 128 {
		return "", apperrors.BadRequest("Photo evidence context is invalid.", nil)
	}
	tx, err := s.repo.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var missionID, state, tenantRef string
	err = tx.QueryRow(ctx, `SELECT id::text,runtime_state,parent_order_ref FROM commerce_missions WHERE canonical_ref=$1 AND (current_provider_id=$2::uuid OR replacement_provider_id=$2::uuid) FOR SHARE`, missionRef, providerID).Scan(&missionID, &state, &tenantRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.NotFound("Assigned Commerce mission not found.", nil)
	}
	if err != nil {
		return "", err
	}
	if state == "COMPLETED" || state == "CANCELLED" || state == "RETURNED" {
		return "", apperrors.Conflict("Evidence cannot be added to a finished mission.", nil)
	}
	if err = assertParcelScopeTx(ctx, tx, missionID, "", parcels, false); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	scope, _ := json.Marshal(parcels)
	assets, err := s.uploads.List(ctx, mediaclient.ListFilter{OwnerID: missionRef, Purpose: "proof_image"})
	if err != nil {
		return "", apperrors.Unavailable("Evidence storage could not be reached.", err)
	}
	for _, asset := range assets {
		if asset.OwnerService == "dispatch-delivery-service" && asset.OwnerID == missionRef && asset.Metadata["provider_id"] == providerID && asset.Metadata["operation_ref"] == operation {
			if asset.ChecksumSHA256 != checksum || asset.Metadata["parcel_scope"] != string(scope) {
				return "", apperrors.Conflict("This photo request was already used for different evidence.", nil)
			}
			if asset.ID != "" && strings.EqualFold(asset.Status, "active") {
				return asset.ID, nil
			}
			return "", apperrors.Unavailable("The photo is still being processed. Retry the same photo shortly.", nil)
		}
	}
	extension := ".jpg"
	if mime == "image/png" {
		extension = ".png"
	}
	asset, err := s.uploads.Upload(ctx, mediaclient.UploadRequest{OwnerID: missionRef, Purpose: "proof_image", Filename: uuid.NewString() + extension, ContentType: mime, Body: bytes.NewReader(data), Metadata: map[string]interface{}{"tenant_ref": tenantRef, "upload_intent": "CUSTODY_EVIDENCE", "mission_ref": missionRef, "provider_id": providerID, "operation_ref": operation, "parcel_refs": parcels, "parcel_scope": string(scope)}})
	if err != nil {
		return "", apperrors.Unavailable("Photo upload could not be confirmed. Retry the same photo.", err)
	}
	if asset.ID == "" || asset.OwnerID != missionRef || asset.OwnerService != "dispatch-delivery-service" {
		return "", apperrors.Unavailable("Photo ownership could not be confirmed.", nil)
	}
	if !strings.EqualFold(asset.Status, "active") {
		return "", apperrors.Unavailable("The photo is still being processed. Retry the same photo shortly.", nil)
	}
	return asset.ID, nil
}
