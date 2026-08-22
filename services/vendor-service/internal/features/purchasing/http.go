package purchasing

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	service     *Service
	idempotency *idempotency.Store
}

func NewHandler(
	service *Service,
	store *idempotency.Store,
) *Handler {
	return &Handler{
		service:     service,
		idempotency: store,
	}
}

func (h *Handler) actor(c *gin.Context) (Actor, error) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		return Actor{}, apperrors.Unauthorized(
			"Authentication is required.",
			nil,
		)
	}

	return Actor{
		StaffID:       staff.StaffID,
		VendorID:      staff.VendorID,
		SessionID:     staff.SessionID,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
		CommandRef:    c.GetHeader("Idempotency-Key"),
	}, nil
}

func (h *Handler) authorized(
	c *gin.Context,
	permission string,
) (Actor, Scope, bool) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return Actor{}, Scope{}, false
	}

	if h.service == nil {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Purchasing access is not permitted.",
				nil,
			),
		)
		return Actor{}, Scope{}, false
	}

	scope, err := h.service.ResolveScope(
		c,
		actor,
		permission,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return Actor{}, Scope{}, false
	}

	return actor, scope, true
}

func strictJSON(c *gin.Context, target any) error {
	raw, err := io.ReadAll(
		io.LimitReader(c.Request.Body, 1<<20),
	)
	if err != nil {
		return apperrors.BadRequest(
			"Request body is invalid.",
			err,
		)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err = decoder.Decode(target); err != nil {
		return apperrors.BadRequest(
			"Request body is invalid.",
			err,
		)
	}

	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return apperrors.BadRequest(
			"Request body must contain exactly one JSON object.",
			err,
		)
	}

	return nil
}

func page(c *gin.Context) (int, int, error) {
	limit, err := strconv.Atoi(
		c.DefaultQuery("limit", "100"),
	)
	if err != nil || limit <= 0 {
		return 0, 0, apperrors.Validation(
			"Check your details.",
			[]apperrors.FieldViolation{{
				Field:   "limit",
				Message: "Limit must be a positive integer.",
			}},
		)
	}

	offset, err := strconv.Atoi(
		c.DefaultQuery("offset", "0"),
	)
	if err != nil || offset < 0 {
		return 0, 0, apperrors.Validation(
			"Check your details.",
			[]apperrors.FieldViolation{{
				Field:   "offset",
				Message: "Offset must be a non-negative integer.",
			}},
		)
	}

	if limit > 200 {
		limit = 200
	}
	return limit, offset, nil
}

func purchasingOK(c *gin.Context, data any) {
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"data":    data,
		},
	)
}

func purchasingCreated(c *gin.Context, data any) {
	c.JSON(
		http.StatusCreated,
		gin.H{
			"success": true,
			"data":    data,
		},
	)
}

func (h *Handler) listCategories(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionRead,
	)
	if !ok {
		return
	}

	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.ListCategories(
		c,
		actor,
		scope,
		limit,
		offset,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) createCategory(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input CreateCategoryInput
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.CreateCategory(
		c,
		actor,
		scope,
		input,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingCreated(c, out)
}

func (h *Handler) updateCategory(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input UpdateCategoryInput
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.UpdateCategory(
		c,
		actor,
		scope,
		c.Param("categoryId"),
		input,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) archiveCategory(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input VersionCommand
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.ArchiveCategory(
		c,
		actor,
		scope,
		c.Param("categoryId"),
		input.ExpectedVersion,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) listSuppliers(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionRead,
	)
	if !ok {
		return
	}

	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.ListSuppliers(
		c,
		actor,
		scope,
		strings.TrimSpace(c.Query("branch_id")),
		limit,
		offset,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) createSupplier(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input CreateSupplierInput
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.CreateSupplier(
		c,
		actor,
		scope,
		input,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingCreated(c, out)
}

func (h *Handler) getSupplier(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionRead,
	)
	if !ok {
		return
	}

	out, err := h.service.GetSupplier(
		c,
		actor,
		scope,
		c.Param("supplierId"),
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) updateSupplier(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input UpdateSupplierInput
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.UpdateSupplier(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		input,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) archiveSupplier(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input VersionCommand
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.ArchiveSupplier(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		input.ExpectedVersion,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) listContacts(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionRead,
	)
	if !ok {
		return
	}

	out, err := h.service.ListContacts(
		c,
		actor,
		scope,
		c.Param("supplierId"),
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) createContact(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input CreateContactInput
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.CreateContact(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		input,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingCreated(c, out)
}

func (h *Handler) updateContact(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input UpdateContactInput
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.UpdateContact(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		c.Param("contactId"),
		input,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) archiveContact(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input VersionCommand
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.ArchiveContact(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		c.Param("contactId"),
		input.ExpectedVersion,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) getTerms(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionRead,
	)
	if !ok {
		return
	}

	out, err := h.service.GetTerms(
		c,
		actor,
		scope,
		c.Param("supplierId"),
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) upsertTerms(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input UpsertTermsInput
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.UpsertTerms(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		input,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) listSupplierProducts(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionRead,
	)
	if !ok {
		return
	}

	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.ListSupplierProducts(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		strings.TrimSpace(c.Query("branch_id")),
		limit,
		offset,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) createSupplierProduct(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input CreateSupplierProductInput
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.CreateSupplierProduct(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		input,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingCreated(c, out)
}

func (h *Handler) getSupplierProduct(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionRead,
	)
	if !ok {
		return
	}

	out, err := h.service.GetSupplierProduct(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		c.Param("supplierProductId"),
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) updateSupplierProduct(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input UpdateSupplierProductInput
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.UpdateSupplierProduct(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		c.Param("supplierProductId"),
		input,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func (h *Handler) archiveSupplierProduct(c *gin.Context) {
	actor, scope, ok := h.authorized(
		c,
		PermissionManage,
	)
	if !ok {
		return
	}

	var input VersionCommand
	if err := strictJSON(c, &input); err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.ArchiveSupplierProduct(
		c,
		actor,
		scope,
		c.Param("supplierId"),
		c.Param("supplierProductId"),
		input.ExpectedVersion,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	purchasingOK(c, out)
}

func RegisterRoutes(
	router *gin.RouterGroup,
	handler *Handler,
	enabled bool,
) {
	if router == nil || handler == nil || !enabled {
		return
	}

	group := router.Group("/purchasing")

	// Reuse the existing Vendor command idempotency boundary. It skips safe
	// methods and requires/replays Idempotency-Key on mutation methods.
	group.Use(
		idempotency.CommandMiddleware(
			handler.idempotency,
		),
	)

	categories := group.Group("/supplier-categories")
	categories.GET("", handler.listCategories)
	categories.POST("", handler.createCategory)
	categories.PATCH("/:categoryId", handler.updateCategory)
	categories.POST("/:categoryId/archive", handler.archiveCategory)

	suppliers := group.Group("/suppliers")
	suppliers.GET("", handler.listSuppliers)
	suppliers.POST("", handler.createSupplier)
	suppliers.GET("/:supplierId", handler.getSupplier)
	suppliers.PATCH("/:supplierId", handler.updateSupplier)
	suppliers.POST("/:supplierId/archive", handler.archiveSupplier)

	suppliers.GET("/:supplierId/contacts", handler.listContacts)
	suppliers.POST("/:supplierId/contacts", handler.createContact)
	suppliers.PATCH(
		"/:supplierId/contacts/:contactId",
		handler.updateContact,
	)
	suppliers.POST(
		"/:supplierId/contacts/:contactId/archive",
		handler.archiveContact,
	)

	suppliers.GET("/:supplierId/terms", handler.getTerms)
	suppliers.PUT("/:supplierId/terms", handler.upsertTerms)

	suppliers.GET(
		"/:supplierId/products",
		handler.listSupplierProducts,
	)
	suppliers.POST(
		"/:supplierId/products",
		handler.createSupplierProduct,
	)
	suppliers.GET(
		"/:supplierId/products/:supplierProductId",
		handler.getSupplierProduct,
	)
	suppliers.PATCH(
		"/:supplierId/products/:supplierProductId",
		handler.updateSupplierProduct,
	)
	suppliers.POST(
		"/:supplierId/products/:supplierProductId/archive",
		handler.archiveSupplierProduct,
	)
}
