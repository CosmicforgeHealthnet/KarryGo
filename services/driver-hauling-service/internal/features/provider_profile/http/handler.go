package providerprofilehttp

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	providerprofileusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type ProfileHandler struct {
	svc *providerprofileusecases.ProfileService
}

func NewProfileHandler(svc *providerprofileusecases.ProfileService) *ProfileHandler {
	return &ProfileHandler{svc: svc}
}

func (h *ProfileHandler) GetProfile(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	profile, err := h.svc.GetProfile(c.Request.Context(), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": profile})
}

func (h *ProfileHandler) UpdateProfile(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	profile, err := h.svc.UpdateProfile(c.Request.Context(), providerprofileusecases.UpdateProfileInput{
		ProviderID:                   claims.Subject,
		FirstName:                    req.FirstName,
		LastName:                     req.LastName,
		Email:                        req.Email,
		Phone:                        req.Phone,
		LocationState:                req.LocationState,
		LocationCity:                 req.LocationCity,
		OperationMode:                req.OperationMode,
		ServiceType:                  req.ServiceType,
		Language:                     req.Language,
		DriverLicenseNumber:          req.DriverLicenseNumber,
		LicenseExpiryYear:            req.LicenseExpiryYear,
		LicenseExpiryDate:            req.LicenseExpiryDate,
		GovIDURL:                     req.GovIDURL,
		DriverLicenseURL:             req.DriverLicenseURL,
		VehicleRegURL:                req.VehicleRegURL,
		GuarantorName:                req.GuarantorName,
		GuarantorPhone:               req.GuarantorPhone,
		EmergencyContactName:         req.EmergencyContactName,
		EmergencyContactPhone:        req.EmergencyContactPhone,
		EmergencyContactRelationship: req.EmergencyContactRelationship,
		ProfilePhotoURL:              req.ProfilePhotoURL,
		PhotoAssetID:                 req.PhotoAssetID,
		SubmitForVerification:        req.SubmitForVerification,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": profile})
}

func (h *ProfileHandler) CheckContactAvailability(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	emailTaken, phoneTaken, err := h.svc.CheckContactAvailability(
		c.Request.Context(), claims.Subject, c.Query("email"), c.Query("phone"),
	)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{
		"email_taken": emailTaken,
		"phone_taken": phoneTaken,
	}})
}

func (h *ProfileHandler) CreateTruck(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req createTruckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	truck, err := h.svc.CreateTruck(c.Request.Context(), providerprofileusecases.CreateTruckInput{
		ProviderID:        claims.Subject,
		TruckType:         req.TruckType,
		CapacityKg:        req.CapacityKg,
		PlateNumber:       req.PlateNumber,
		Year:              req.Year,
		Make:              req.Make,
		Model:             req.Model,
		Color:             req.Color,
		LicenseType:       req.LicenseType,
		NumberOfAxles:     req.NumberOfAxles,
		YearsOfExperience: req.YearsOfExperience,
		GoodsTypes:        req.GoodsTypes,
		HasInsurance:      req.HasInsurance,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, gin.H{"success": true, "data": truck})
}

func (h *ProfileHandler) ListTrucks(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	trucks, err := h.svc.ListTrucks(c.Request.Context(), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": trucks})
}

func (h *ProfileHandler) GetTruck(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	truck, err := h.svc.GetTruck(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": truck})
}

func (h *ProfileHandler) UpdateTruck(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req updateTruckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	truck, err := h.svc.UpdateTruck(c.Request.Context(), providerprofileusecases.UpdateTruckInput{
		ID:                c.Param("id"),
		ProviderID:        claims.Subject,
		TruckType:         req.TruckType,
		CapacityKg:        req.CapacityKg,
		PlateNumber:       req.PlateNumber,
		Year:              req.Year,
		Make:              req.Make,
		Model:             req.Model,
		Color:             req.Color,
		LicenseType:       req.LicenseType,
		NumberOfAxles:     req.NumberOfAxles,
		YearsOfExperience: req.YearsOfExperience,
		GoodsTypes:        req.GoodsTypes,
		HasInsurance:      req.HasInsurance,
		Status:            req.Status,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": truck})
}
