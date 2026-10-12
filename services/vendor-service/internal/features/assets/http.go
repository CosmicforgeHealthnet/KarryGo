package assets

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	"cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	media "cosmicforge/logistics/services/vendor-service/internal/integrations/media"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"strings"
)

const MaxBytes int64 = 10 << 20

type Options struct {
	DB         *pgxpool.Pool
	Food       *food.Service
	Shop       *shop.Service
	Vendors    *vendors.Service
	Production *production.Service
	Media      *media.Client
	Enabled    map[string]bool
	Writable   map[string]bool
}
type Handler struct{ Options }

func NewHandler(opts Options) *Handler { return &Handler{opts} }

type uploadRequest struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
	FileName   string `json:"file_name"`
	MIMEType   string `json:"mime_type"`
	ByteSize   int64  `json:"byte_size"`
}

func purpose(kind string) string {
	switch kind {
	case "FOOD_ITEM", "SHOP_PRODUCT":
		return "PRODUCT_IMAGE"
	case "BRAND":
		return "BRAND_IMAGE"
	case "PASTRY_REQUEST":
		return "PASTRY_REFERENCE"
	}
	return ""
}
func validUpload(in uploadRequest) bool {
	if purpose(in.TargetType) == "" || in.TargetRef == "" || len(in.TargetRef) > 200 || in.ByteSize <= 0 || in.ByteSize > MaxBytes || in.FileName == "" || len(in.FileName) > 250 || strings.ContainsAny(in.FileName, "/\\\x00\r\n") {
		return false
	}
	return in.MIMEType == "image/jpeg" || in.MIMEType == "image/png" || in.MIMEType == "image/webp" || in.TargetType == "PASTRY_REQUEST" && in.MIMEType == "application/pdf"
}
func (h *Handler) target(ctx context.Context, actor production.Actor, kind, id string, write bool) error {
	if !h.Enabled[kind] || h.Media == nil {
		return apperrors.Unavailable("Media uploads are unavailable for this feature.", nil)
	}
	if write && !h.Writable[kind] {
		return apperrors.Unavailable("Media changes are temporarily unavailable.", nil)
	}
	switch kind {
	case "FOOD_ITEM":
		a := food.Actor{StaffID: actor.StaffID, VendorID: actor.VendorID, SessionID: actor.SessionID}
		perm := food.PermCatalogueRead
		if write {
			perm = food.PermCatalogueManage
		}
		scope, err := h.Food.Authorize(ctx, a, perm)
		if err != nil {
			return err
		}
		item, _, _, err := h.Food.GetItem(ctx, a, scope, id)
		if err != nil {
			return err
		}
		if write && item.Status == "ARCHIVED" {
			return apperrors.BadRequest("Archived items cannot receive uploads.", nil)
		}
	case "SHOP_PRODUCT":
		a := shop.Actor{StaffID: actor.StaffID, VendorID: actor.VendorID, SessionID: actor.SessionID}
		perm := shop.PermRead
		if write {
			perm = shop.PermCatalogueManage
		}
		scope, err := h.Shop.Authorize(ctx, a, perm)
		if err != nil {
			return err
		}
		p, err := h.Shop.GetProduct(ctx, a, scope, id)
		if err != nil {
			return err
		}
		if write && p.Status == "ARCHIVED" {
			return apperrors.BadRequest("Archived products cannot receive uploads.", nil)
		}
	case "BRAND":
		perm := "vendor.brands.read"
		if write {
			perm = "vendor.brands.manage"
		}
		scope, err := h.Production.Authorize(ctx, actor, perm)
		if err != nil {
			return err
		}
		if !scope.Global {
			return apperrors.Forbidden("Brand media requires business-wide access.", nil)
		}
		_, err = h.Vendors.GetBrand(ctx, vendors.Actor{StaffID: actor.StaffID, VendorID: actor.VendorID, SessionID: actor.SessionID}, actor.VendorID, id)
		if err != nil {
			return err
		}
	case "PASTRY_REQUEST":
		perm := "vendor.pastry.read"
		if write {
			perm = "vendor.pastry.manage"
		}
		scope, err := h.Production.Authorize(ctx, actor, perm)
		if err != nil {
			return err
		}
		var branch, status string
		if err = h.DB.QueryRow(ctx, `SELECT branch_id,status FROM pastry_custom_requests WHERE business_id=$1 AND id=$2`, actor.VendorID, id).Scan(&branch, &status); err != nil || !scope.Covers(branch) {
			return apperrors.NotFound("Pastry request not found.", nil)
		}
		if write && (status == "CANCELLED" || status == "FULFILLED" || status == "DECLINED" || status == "COMPLETED") {
			return apperrors.BadRequest("This request cannot receive uploads.", nil)
		}
	default:
		return apperrors.BadRequest("Unsupported media target.", nil)
	}
	return nil
}
func actor(c *gin.Context) (production.Actor, bool) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Sign in to manage media.", nil))
		return production.Actor{}, false
	}
	return production.Actor{StaffID: staff.StaffID, VendorID: staff.VendorID, SessionID: staff.SessionID}, true
}
func (h *Handler) upload(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var in uploadRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF || !validUpload(in) {
		httpx.RespondError(c, apperrors.BadRequest("Choose a supported file up to 10 MB.", nil))
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if key == "" || len(key) > 200 {
		httpx.RespondError(c, apperrors.BadRequest("An upload key is required.", nil))
		return
	}
	if err := h.target(c, a, in.TargetType, in.TargetRef, true); err != nil {
		httpx.RespondError(c, err)
		return
	}
	grant, err := h.Media.AuthorizeAssetUpload(c, a.VendorID, in.TargetType, in.TargetRef, purpose(in.TargetType), in.FileName, in.MIMEType, in.ByteSize, a.VendorID+":"+a.StaffID+":"+key)
	if err != nil {
		httpx.RespondError(c, apperrors.Unavailable("Upload authorization could not be issued.", nil))
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"success": true, "data": gin.H{"upload_session_id": grant.SessionID, "media_ref": grant.MediaRef, "upload_url": grant.URL, "upload_token": grant.Token, "expires_at": grant.ExpiresAt, "max_bytes": grant.MaxBytes, "allowed_mime_types": grant.AllowedMIMETypes}})
}
func (h *Handler) object(c *gin.Context) (media.Object, bool) {
	a, ok := actor(c)
	if !ok {
		return media.Object{}, false
	}
	kind, id := c.Query("target_type"), c.Query("target_ref")
	if err := h.target(c, a, kind, id, false); err != nil {
		httpx.RespondError(c, err)
		return media.Object{}, false
	}
	object, err := h.Media.GetObject(c, a.VendorID, c.Param("mediaRef"))
	if err != nil {
		httpx.RespondError(c, apperrors.Unavailable("Media status could not be read.", nil))
		return media.Object{}, false
	}
	if object.ID != c.Param("mediaRef") || object.OwnerReference != a.VendorID || object.OwnerService != "vendor-service" || object.Visibility != "PRIVATE" || object.UploadPurpose != purpose(kind) || object.BindingType != "ASSET" || object.BindingID != kind+":"+id || object.DeletionState != "ACTIVE" {
		httpx.RespondError(c, apperrors.NotFound("Media file not found.", nil))
		return media.Object{}, false
	}
	return object, true
}
func (h *Handler) status(c *gin.Context) {
	object, ok := h.object(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"success": true, "data": gin.H{"media_ref": object.ID, "upload_status": object.UploadStatus, "safety_status": object.SafetyStatus, "mime_type": object.MIMEType, "file_name": object.FileName, "byte_size": object.SizeBytes, "usable": object.UploadStatus == "COMPLETED" && object.SafetyStatus == "SAFE"}})
}
func (h *Handler) content(c *gin.Context) {
	object, ok := h.object(c)
	if !ok {
		return
	}
	if object.UploadStatus != "COMPLETED" || object.SafetyStatus != "SAFE" || object.SizeBytes <= 0 || object.SizeBytes > MaxBytes || !validUpload(uploadRequest{TargetType: c.Query("target_type"), TargetRef: c.Query("target_ref"), FileName: object.FileName, MIMEType: object.MIMEType, ByteSize: object.SizeBytes}) {
		httpx.RespondError(c, apperrors.Unavailable("This file is not ready for viewing.", nil))
		return
	}
	a, _ := actor(c)
	bytes, err := h.Media.AssetContent(c, a.VendorID, object.ID, MaxBytes)
	sum := sha256.Sum256(bytes)
	if err != nil || int64(len(bytes)) != object.SizeBytes || hex.EncodeToString(sum[:]) != object.Checksum {
		httpx.RespondError(c, apperrors.Unavailable("File content could not be verified.", nil))
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, object.MIMEType, bytes)
}
func RegisterRoutes(group *gin.RouterGroup, h *Handler) {
	g := group.Group("/media")
	g.POST("/uploads", h.upload)
	g.GET("/objects/:mediaRef", h.status)
	g.GET("/objects/:mediaRef/content", h.content)
}
