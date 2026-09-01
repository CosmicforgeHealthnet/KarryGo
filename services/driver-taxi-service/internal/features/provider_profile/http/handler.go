package providerprofilehttp

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	providerprofileusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/usecases"
	"cosmicforge/logistics/services/taxi-service/internal/vehicleclass"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type ProfileHandler struct {
	profiles *providerprofileusecases.ProfileService
}

func NewProfileHandler(profiles *providerprofileusecases.ProfileService) *ProfileHandler {
	return &ProfileHandler{profiles: profiles}
}

type successResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

func respond(c *gin.Context, status int, data interface{}) {
	c.JSON(status, successResponse{Success: true, Data: data})
}

func providerID(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return claims.Subject, true
}

func (h *ProfileHandler) GetProfile(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	profile, err := h.profiles.GetProfile(c.Request.Context(), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, profile)
}

// SetParcelPreference toggles whether this driver may be matched to package
// deliveries. Drivers are opted out by default.
func (h *ProfileHandler) SetParcelPreference(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req setParcelPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	profile, err := h.profiles.SetAcceptsParcels(c.Request.Context(), id, req.AcceptsParcels)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, profile)
}

func (h *ProfileHandler) UpdateProfile(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	profile, err := h.profiles.UpdateProfile(c.Request.Context(), providerprofileusecases.UpdateProfileInput{
		ProviderID:                   id,
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
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, profile)
}

func (h *ProfileHandler) CheckContactAvailability(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	emailTaken, phoneTaken, err := h.profiles.CheckContactAvailability(
		c.Request.Context(), id, c.Query("email"), c.Query("phone"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, gin.H{"email_taken": emailTaken, "phone_taken": phoneTaken})
}

func (h *ProfileHandler) CreateVehicle(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req createVehicleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	vehicle, err := h.profiles.CreateVehicle(c.Request.Context(), providerprofileusecases.CreateVehicleInput{
		ProviderID:   id,
		VehicleClass: req.VehicleClass,
		PlateNumber:  req.PlateNumber,
		Seats:        req.Seats,
		Year:         req.Year,
		Make:         req.Make,
		Model:        req.Model,
		Color:        req.Color,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, vehicle)
}

func (h *ProfileHandler) ListVehicles(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	vehicles, err := h.profiles.ListVehicles(c.Request.Context(), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, gin.H{"vehicles": vehicles})
}

func (h *ProfileHandler) GetVehicle(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	vehicle, err := h.profiles.GetVehicle(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, vehicle)
}

func (h *ProfileHandler) UpdateVehicle(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req updateVehicleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	vehicle, err := h.profiles.UpdateVehicle(c.Request.Context(), providerprofileusecases.UpdateVehicleInput{
		ID:           c.Param("id"),
		ProviderID:   id,
		VehicleClass: req.VehicleClass,
		PlateNumber:  req.PlateNumber,
		Seats:        req.Seats,
		Year:         req.Year,
		Make:         req.Make,
		Model:        req.Model,
		Color:        req.Color,
		Status:       req.Status,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, vehicle)
}

// ListVehicleClasses is public: the passenger app needs the bookable classes
// and their seat limits to render the fare tiles.
func (h *ProfileHandler) ListVehicleClasses(c *gin.Context) {
	specs := vehicleclass.All()
	out := make([]gin.H, 0, len(specs))
	for _, s := range specs {
		out = append(out, gin.H{
			"vehicle_class":  s.Name,
			"label":          s.Label,
			"max_passengers": s.MaxPassengers,
		})
	}
	respond(c, nethttp.StatusOK, gin.H{"classes": out})
}
