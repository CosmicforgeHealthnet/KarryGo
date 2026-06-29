package trip

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/apiresponse"
	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(engine *gin.Engine, tokens *authusecases.TokenUsecase, handler *Handler) {
	trips := engine.Group("/api/v1/provider/trips")
	trips.Use(authhttp.DispatchRiderAuthRequired(tokens), requireDispatchProviderRole())
	trips.GET("", handler.ListTrips)
	trips.GET("/active", handler.GetActiveTrip)
	trips.GET("/:id", handler.GetTrip)
	trips.POST("/:id/arrived", handler.MarkArrived)
	trips.POST("/:id/start", handler.StartTrip)
	trips.POST("/:id/proof", handler.SubmitProof)
	trips.GET("/:id/proof", handler.GetProof)
	trips.POST("/:id/complete", handler.CompleteTrip)
	trips.POST("/:id/cancel", handler.CancelTrip)
	trips.POST("/:id/rate-customer", handler.RateCustomer)
}

func (h *Handler) ListTrips(c *gin.Context) {
	page, err := positiveIntQuery(c, "page", 1, 0)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	limit, err := positiveIntQuery(c, "limit", 20, 50)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	result, err := h.service.ListProviderTrips(c.Request.Context(), authhttp.DispatchRiderID(c), ListTripsOptions{
		Status: TripStatus(strings.TrimSpace(c.Query("status"))), Limit: limit, Offset: (page - 1) * limit,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Trips loaded.", result)
}

func (h *Handler) GetActiveTrip(c *gin.Context) {
	result, err := h.service.GetProviderActiveTrip(c.Request.Context(), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Active trip loaded.", result)
}

func (h *Handler) GetTrip(c *gin.Context) {
	result, err := h.service.GetProviderTripDetail(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Trip loaded.", result)
}

func positiveIntQuery(c *gin.Context, key string, fallback, max int) (int, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || (max > 0 && value > max) {
		message := "Must be a positive integer."
		if max > 0 {
			message = "Must be between 1 and " + strconv.Itoa(max) + "."
		}
		return 0, validationError(key, message)
	}
	return value, nil
}

func (h *Handler) GetProof(c *gin.Context) {
	result, err := h.service.GetProviderProof(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Delivery proof loaded.", result)
}

// MarkArrived — Phase 7F: provider has reached the pickup location.
func (h *Handler) MarkArrived(c *gin.Context) {
	result, err := h.service.MarkArrived(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, result.Message, result)
}

// StartTrip — Phase 7G: provider has collected the package and is heading to destination.
func (h *Handler) StartTrip(c *gin.Context) {
	result, err := h.service.StartTrip(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Delivery started. Head to the drop-off location.", result)
}

// SubmitProof — Phase 7H: multipart proof of delivery upload.
func (h *Handler) SubmitProof(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(12 << 20); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Failed to parse multipart form.", err))
		return
	}
	photoFile, photoHeader, photoErr := c.Request.FormFile("delivery_photo")
	if photoErr != nil {
		if photoErr == http.ErrMissingFile {
			httpx.Abort(c, validationError("delivery_photo", "Delivery photo is required."))
		} else {
			httpx.Abort(c, apperrors.BadRequest("Failed to read delivery photo.", photoErr))
		}
		return
	}
	defer photoFile.Close()

	sigFile, sigHeader, sigErr := c.Request.FormFile("signature")
	if sigErr != nil {
		if sigErr == http.ErrMissingFile {
			httpx.Abort(c, validationError("signature", "Signature is required."))
		} else {
			httpx.Abort(c, apperrors.BadRequest("Failed to read signature.", sigErr))
		}
		return
	}
	defer sigFile.Close()

	result, err := h.service.SubmitProof(c.Request.Context(), ProofSubmitInput{
		TripID:        c.Param("id"),
		ProviderID:    authhttp.DispatchRiderID(c),
		ReceiverName:  c.Request.FormValue("receiver_name"),
		ReceiverPhone: c.Request.FormValue("receiver_phone"),
		PhotoFile:     photoFile,
		PhotoHeader:   photoHeader,
		SigFile:       sigFile,
		SigHeader:     sigHeader,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusCreated, result.Message, result)
}

// CompleteTrip — Phase 7J: finalize delivery after proof submission.
func (h *Handler) CompleteTrip(c *gin.Context) {
	result, err := h.service.CompleteTrip(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, result.Message, result)
}

// CancelTrip — Phase 7K: provider cancels a trip with a reason code.
func (h *Handler) CancelTrip(c *gin.Context) {
	var req CancelRequest
	_ = c.ShouldBindJSON(&req) // Body is optional-shaped; service validates reason_code.
	result, err := h.service.CancelTrip(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c), req)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Trip cancelled.", result)
}

// RateCustomer handles POST /api/v1/provider/trips/:id/rate-customer.
func (h *Handler) RateCustomer(c *gin.Context) {
	var req RateCustomerInput
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "score", Message: "Invalid request body."},
		}))
		return
	}
	result, err := h.service.RateCustomer(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c), req)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusCreated, "Customer rated successfully.", result)
}

func (h *Handler) foundationAction(c *gin.Context) {
	if err := h.service.FoundationOperation(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c)); err != nil {
		httpx.Abort(c, err)
		return
	}
}

func requireDispatchProviderRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authhttp.Role(c) != authmodels.RoleDispatchProvider {
			httpx.Abort(c, apperrors.Forbidden("This route is only available to dispatch providers.", nil))
			return
		}
		c.Next()
	}
}
