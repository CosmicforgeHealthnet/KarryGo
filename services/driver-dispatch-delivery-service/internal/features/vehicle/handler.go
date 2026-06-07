package vehicle

import (
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	authhttp "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/http"
	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

// Handler wires HTTP to the service.
type Handler struct {
	service *Service
}

// NewHandler constructs a production Handler wired to Postgres and Redis.
func NewHandler(pool *pgxpool.Pool, uploader FileUploader, events EventPublisher) *Handler {
	repo := NewPostgresRepository(pool)
	svc := NewService(repo, WithUploader(uploader), WithEventPublisher(events))
	return &Handler{service: svc}
}

// NewHandlerWithService allows test injection.
func NewHandlerWithService(svc *Service) *Handler {
	return &Handler{service: svc}
}

func RegisterRoutes(engine *gin.Engine, tokens *authusecases.TokenUsecase, handler *Handler) {
	provider := engine.Group("/api/v1/provider/vehicle")
	provider.Use(authhttp.DispatchRiderAuthRequired(tokens), requireDispatchProviderRole())
	provider.POST("", handler.RegisterBike)
	provider.GET("", handler.ListBikes)
	provider.GET("/:id", handler.GetBike)
	provider.PATCH("/:id", handler.UpdateBike)
	provider.POST("/:id/documents", handler.UploadDocument)
	provider.GET("/:id/documents", handler.GetDocuments)

	admin := engine.Group("/api/v1/admin/vehicle")
	admin.Use(authhttp.DispatchRiderAuthRequired(tokens), requirePlatformAdminRole())
	admin.PATCH("/:id/review", handler.AdminReview)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (h *Handler) RegisterBike(c *gin.Context) {
	var input RegisterBikeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	bike, err := h.service.RegisterBike(
		c.Request.Context(),
		authhttp.DispatchRiderID(c),
		httpx.GetRequestID(c),
		input,
	)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": bike})
}

func (h *Handler) ListBikes(c *gin.Context) {
	bikes, err := h.service.ListMyBikes(c.Request.Context(), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": bikes})
}

func (h *Handler) GetBike(c *gin.Context) {
	bikeID := c.Param("id")
	if !isValidUUID(bikeID) {
		httpx.Abort(c, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "id", Message: "Bike ID must be a valid UUID."},
		}))
		return
	}
	bike, err := h.service.GetBike(c.Request.Context(), bikeID, authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": bike})
}

func (h *Handler) UpdateBike(c *gin.Context) {
	bikeID := c.Param("id")
	if !isValidUUID(bikeID) {
		httpx.Abort(c, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "id", Message: "Bike ID must be a valid UUID."},
		}))
		return
	}
	var input UpdateBikeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	// Immutable fields — always ignored by dropping them from the input struct before service call.
	// (plate_number and bike_type are not fields on UpdateBikeInput, so they cannot be set.)
	updated, err := h.service.UpdateBike(
		c.Request.Context(),
		bikeID,
		authhttp.DispatchRiderID(c),
		httpx.GetRequestID(c),
		input,
	)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": updated})
}

func (h *Handler) UploadDocument(c *gin.Context) {
	bikeID := c.Param("id")
	if !isValidUUID(bikeID) {
		httpx.Abort(c, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "id", Message: "Bike ID must be a valid UUID."},
		}))
		return
	}

	docTypeStr := strings.TrimSpace(c.PostForm("document_type"))
	expiryDateStr := strings.TrimSpace(c.PostForm("expiry_date"))

	fileHeader, err := vehicleMultipartFile(c, "document_file")
	if err != nil {
		httpx.Abort(c, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "document_file", Message: "Document file is required."},
		}))
		return
	}

	var expiryPtr *string
	if expiryDateStr != "" {
		expiryPtr = &expiryDateStr
	}

	var file File
	if fileHeader != nil {
		opened, err := fileHeader.Open()
		if err != nil {
			httpx.Abort(c, apperrors.Internal("Could not read document file.", err))
			return
		}
		defer opened.Close()
		file = opened
	}

	input := UploadDocumentInput{
		ProviderID:    authhttp.DispatchRiderID(c),
		BikeID:        bikeID,
		CorrelationID: httpx.GetRequestID(c),
		DocumentType:  DocumentType(docTypeStr),
		File:          file,
		Header:        fileHeader,
		ExpiryDate:    expiryPtr,
	}
	doc, err := h.service.UploadDocument(c.Request.Context(), input)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": doc})
}

func (h *Handler) GetDocuments(c *gin.Context) {
	bikeID := c.Param("id")
	if !isValidUUID(bikeID) {
		httpx.Abort(c, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "id", Message: "Bike ID must be a valid UUID."},
		}))
		return
	}
	docs, err := h.service.ListDocuments(c.Request.Context(), bikeID, authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": docs})
}

func (h *Handler) AdminReview(c *gin.Context) {
	bikeID := c.Param("id")
	if !isValidUUID(bikeID) {
		httpx.Abort(c, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "id", Message: "Bike ID must be a valid UUID."},
		}))
		return
	}
	var input AdminReviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	updated, err := h.service.AdminReview(
		c.Request.Context(),
		bikeID,
		authhttp.DispatchRiderID(c),
		httpx.GetRequestID(c),
		input,
	)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": updated})
}

// ── Role middleware ────────────────────────────────────────────────────────────

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
		if authhttp.Role(c) != "platform_admin" {
			httpx.Abort(c, apperrors.Forbidden("This route is only available to platform admins.", nil))
			return
		}
		c.Next()
	}
}

// ── Multipart helpers ─────────────────────────────────────────────────────────

func vehicleMultipartFile(c *gin.Context, field string) (FileHeader, error) {
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
	return vehicleMultipartFileHeader{header: header}, nil
}

type vehicleMultipartFileHeader struct {
	header *multipart.FileHeader
}

func (h vehicleMultipartFileHeader) Open() (File, error) {
	return h.header.Open()
}

func (h vehicleMultipartFileHeader) GetFilename() string {
	return h.header.Filename
}

func (h vehicleMultipartFileHeader) GetSize() int64 {
	return h.header.Size
}

func (h vehicleMultipartFileHeader) GetHeaderValue(key string) string {
	return h.header.Header.Get(key)
}

// ── UUID validation ───────────────────────────────────────────────────────────

func isValidUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, r := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
		} else {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}

// respondOK is kept for internal use.
func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}
