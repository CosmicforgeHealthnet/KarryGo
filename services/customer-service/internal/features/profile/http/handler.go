package profilehttp

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	profileusecases "cosmicforge/logistics/services/customer-service/internal/features/profile/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxPhotoBytes = 10 * 1024 * 1024 // 10 MB

type ProfileHandler struct {
	profile *profileusecases.ProfileService
}

func NewProfileHandler(profile *profileusecases.ProfileService) *ProfileHandler {
	return &ProfileHandler{profile: profile}
}

type updateProfileRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func (h *ProfileHandler) GetProfile(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	customer, err := h.profile.GetProfile(c.Request.Context(), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": customer.Public()})
}

func (h *ProfileHandler) UpdateProfile(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}

	customer, err := h.profile.UpdateProfile(c.Request.Context(), profileusecases.UpdateProfileInput{
		CustomerID: claims.Subject,
		FirstName:  req.FirstName,
		LastName:   req.LastName,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": customer.Public()})
}

type savePhotoURLRequest struct {
	PhotoURL string `json:"photo_url"`
	AssetID  string `json:"asset_id"`
}

func (h *ProfileHandler) SavePhotoURL(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req savePhotoURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	if req.PhotoURL == "" || req.AssetID == "" {
		httpx.Abort(c, apperrors.Validation("photo_url and asset_id are required.", nil))
		return
	}

	customer, err := h.profile.SaveProfilePhotoURL(c.Request.Context(), profileusecases.SavePhotoURLInput{
		CustomerID: claims.Subject,
		PhotoURL:   req.PhotoURL,
		AssetID:    req.AssetID,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": customer.Public()})
}

// ─── Emergency contact handlers ───────────────────────────────────────────────

func (h *ProfileHandler) GetEmergencyContacts(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	contacts, err := h.profile.GetEmergencyContacts(c.Request.Context(), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	public := make([]interface{}, len(contacts))
	for i, ec := range contacts {
		public[i] = ec.Public()
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{"contacts": public}})
}

type addEmergencyContactRequest struct {
	Name         string `json:"name"`
	Phone        string `json:"phone"`
	Relationship string `json:"relationship"`
}

func (h *ProfileHandler) AddEmergencyContact(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req addEmergencyContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}

	contact, err := h.profile.AddEmergencyContact(c.Request.Context(), profileusecases.AddEmergencyContactInput{
		CustomerID:   claims.Subject,
		Name:         req.Name,
		Phone:        req.Phone,
		Relationship: req.Relationship,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, gin.H{"success": true, "data": contact.Public()})
}

func (h *ProfileHandler) DeleteEmergencyContact(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	if err := h.profile.DeleteEmergencyContact(c.Request.Context(), c.Param("id"), claims.Subject); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{}})
}

func (h *ProfileHandler) UploadPhoto(c *gin.Context) {
	c.Request.Body = nethttp.MaxBytesReader(c.Writer, c.Request.Body, maxPhotoBytes+512*1024)

	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	file, header, err := c.Request.FormFile("photo")
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("A photo file is required.", err))
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	customer, err := h.profile.UploadProfilePhoto(c.Request.Context(), profileusecases.UploadPhotoInput{
		CustomerID:  claims.Subject,
		Filename:    header.Filename,
		ContentType: contentType,
		Body:        file,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	c.JSON(nethttp.StatusOK, gin.H{
		"success": true,
		"data":    customer.Public(),
	})
}
