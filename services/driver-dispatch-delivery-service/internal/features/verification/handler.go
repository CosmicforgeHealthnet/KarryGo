package verification

import (
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

type Handler struct {
	service *Service
}

func NewHandler(pool *pgxpool.Pool) *Handler {
	repository := NewPostgresRepository(pool)
	return &Handler{
		service: NewService(repository, NewStubSmileIdentityClient()),
	}
}

func NewHandlerWithService(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(engine *gin.Engine, tokens *authusecases.TokenUsecase, handler *Handler) {
	providerVerification := engine.Group("/api/v1/provider/verification")
	providerVerification.Use(authhttp.DispatchRiderAuthRequired(tokens), requireDispatchProviderRole())
	providerVerification.POST("/identity", handler.SubmitIdentity)
	providerVerification.POST("/licence", handler.SubmitLicence)
	providerVerification.POST("/face", handler.SubmitFace)
	providerVerification.GET("/status", handler.GetAllStatus)
	providerVerification.GET("/status/:step", handler.GetStepStatus)

	adminVerification := engine.Group("/api/v1/admin/verification")
	adminVerification.Use(authhttp.DispatchRiderAuthRequired(tokens), requirePlatformAdminRole())
	adminVerification.PATCH("/:provider_id/review", handler.AdminReview)
}

func (h *Handler) SubmitIdentity(c *gin.Context) {
	govtIDFile, err := multipartFile(c, "govt_id_file")
	if err != nil {
		httpx.Abort(c, validationError("govt_id_file", "Government ID file is invalid."))
		return
	}
	profilePhoto, err := multipartFile(c, "profile_photo")
	if err != nil {
		httpx.Abort(c, validationError("profile_photo", "Profile photo is invalid."))
		return
	}
	result, err := h.service.SubmitIdentity(c.Request.Context(), IdentitySubmissionInput{
		ProviderID:    authhttp.DispatchRiderID(c),
		CorrelationID: httpx.GetRequestID(c),
		GovtIDType:    c.PostForm("govt_id_type"),
		GovtIDNumber:  c.PostForm("govt_id_number"),
		GovtIDFile:    FileUpload{Header: govtIDFile},
		ProfilePhoto:  FileUpload{Header: profilePhoto},
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) SubmitLicence(c *gin.Context) {
	licenceFile, err := multipartFile(c, "licence_file")
	if err != nil {
		httpx.Abort(c, validationError("licence_file", "Licence file is invalid."))
		return
	}
	result, err := h.service.SubmitLicence(c.Request.Context(), LicenceSubmissionInput{
		ProviderID:    authhttp.DispatchRiderID(c),
		CorrelationID: httpx.GetRequestID(c),
		LicenceNumber: c.PostForm("licence_number"),
		ExpiryYear:    c.PostForm("expiry_year"),
		ExpiryMonth:   c.PostForm("expiry_month"),
		LicenceFile:   FileUpload{Header: licenceFile},
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) SubmitFace(c *gin.Context) {
	selfie, err := multipartFile(c, "selfie")
	if err != nil {
		httpx.Abort(c, validationError("selfie", "Selfie file is invalid."))
		return
	}
	result, err := h.service.SubmitFace(c.Request.Context(), FaceSubmissionInput{
		ProviderID:    authhttp.DispatchRiderID(c),
		CorrelationID: httpx.GetRequestID(c),
		Selfie:        FileUpload{Header: selfie},
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) GetAllStatus(c *gin.Context) {
	result, err := h.service.GetAllStatus(c.Request.Context(), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) GetStepStatus(c *gin.Context) {
	result, err := h.service.GetStepStatus(c.Request.Context(), authhttp.DispatchRiderID(c), Step(c.Param("step")))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) AdminReview(c *gin.Context) {
	var req AdminReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	result, err := h.service.AdminReview(
		c.Request.Context(),
		authhttp.DispatchRiderID(c),
		httpx.GetRequestID(c),
		c.Param("provider_id"),
		req,
	)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
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

func requirePlatformAdminRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authhttp.Role(c) != RolePlatformAdmin {
			httpx.Abort(c, apperrors.Forbidden("This route is only available to platform admins.", nil))
			return
		}
		c.Next()
	}
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

func multipartFile(c *gin.Context, field string) (FileHeader, error) {
	file, header, err := c.Request.FormFile(field)
	if file != nil {
		_ = file.Close()
	}
	if err == http.ErrMissingFile {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return multipartFileHeader{header: header}, nil
}

type multipartFileHeader struct {
	header *multipart.FileHeader
}

func (h multipartFileHeader) Open() (File, error) {
	return h.header.Open()
}

func (h multipartFileHeader) GetFilename() string {
	return h.header.Filename
}

func (h multipartFileHeader) GetSize() int64 {
	return h.header.Size
}

func (h multipartFileHeader) GetHeaderValue(key string) string {
	return h.header.Header.Get(key)
}
