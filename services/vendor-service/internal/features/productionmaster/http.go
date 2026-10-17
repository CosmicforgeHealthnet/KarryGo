package productionmaster

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	service *Service
	scopes  inventory.ScopeResolver
	idem    *idempotency.Store
}

func NewHandler(
	service *Service,
	scopes inventory.ScopeResolver,
	idem *idempotency.Store,
) *Handler {
	return &Handler{
		service: service,
		scopes:  scopes,
		idem:    idem,
	}
}

func strictJSON(
	c *gin.Context,
	dst any,
) error {
	body, err := io.ReadAll(
		io.LimitReader(
			c.Request.Body,
			1<<20,
		),
	)
	if err != nil {
		return err
	}

	decoder :=
		json.NewDecoder(
			strings.NewReader(
				string(body),
			),
		)

	decoder.DisallowUnknownFields()

	if err = decoder.Decode(dst); err != nil {
		return err
	}

	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New(
			"request contains more than one JSON value",
		)
	}

	return nil
}

func expectedVersion(
	c *gin.Context,
) (int64, error) {
	raw :=
		strings.TrimSpace(
			c.GetHeader("If-Match"),
		)

	raw =
		strings.TrimPrefix(
			raw,
			"W/",
		)

	raw =
		strings.Trim(
			raw,
			`"`,
		)

	version, err :=
		strconv.ParseInt(
			raw,
			10,
			64,
		)

	if err != nil ||
		version <= 0 {
		return 0, errors.New(
			"If-Match must contain a positive resource version",
		)
	}

	return version, nil
}

func handlerError(
	err error,
) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return apperrors.NotFound(
			"Production master resource was not found.",
			err,
		)

	case errors.Is(err, ErrVersionConflict):
		return apperrors.Conflict(
			"The resource changed before the operation completed.",
			err,
		)

	case errors.Is(err, ErrInvalidState):
		return apperrors.Conflict(
			"The production master resource cannot be changed in its current state.",
			err,
		)

	case errors.Is(err, ErrInventoryMapping):
		return apperrors.BadRequest(
			"The Inventory mapping is invalid.",
			err,
		)

	case errors.Is(err, ErrUnsafeConversion):
		return apperrors.BadRequest(
			"The conversion cannot be used as authoritative stock arithmetic.",
			err,
		)

	case errors.Is(err, ErrInvalidInput):
		return apperrors.BadRequest(
			"The production master request is invalid.",
			err,
		)

	default:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return apperrors.Conflict(
				"That code is already in use. Choose a different code.",
				err,
			)
		}
		return err
	}
}

func success(
	c *gin.Context,
	status int,
	data any,
) {
	c.JSON(
		status,
		gin.H{
			"success": true,
			"data":    data,
		},
	)
}

func (h *Handler) actor(
	c *gin.Context,
) (Actor, error) {
	auth, ok :=
		authctx.FromContext(c)

	if !ok {
		return Actor{},
			apperrors.Unauthorized(
				"Authentication is required.",
				nil,
			)
	}

	return Actor{
		StaffID:       auth.StaffID,
		BusinessID:    auth.VendorID,
		SessionID:     auth.SessionID,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
		RequestID:     httpx.GetRequestID(c),
	}, nil
}

func (h *Handler) authorizeBusiness(
	c *gin.Context,
	actor Actor,
	permission string,
) bool {
	if h.scopes == nil {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Production master access is not permitted.",
				nil,
			),
		)
		return false
	}

	_, err :=
		h.scopes.ResolveScope(
			c,
			actor.StaffID,
			permission,
		)

	if err != nil {
		httpx.RespondError(c, err)
		return false
	}

	return true
}

func (h *Handler) authorizeBranch(
	c *gin.Context,
	actor Actor,
	branchID string,
	permission string,
) bool {
	if h.scopes == nil {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Production master access is not permitted.",
				nil,
			),
		)
		return false
	}

	scope, err :=
		h.scopes.ResolveScope(
			c,
			actor.StaffID,
			permission,
		)

	if err != nil {
		httpx.RespondError(c, err)
		return false
	}

	if !scope.Covers(
		strings.TrimSpace(branchID),
	) {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Production master access is not permitted for this branch.",
				nil,
			),
		)
		return false
	}

	return true
}

func (h *Handler) createIngredient(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionIngredientManage,
	) {
		return
	}

	var input CreateIngredientInput

	if err = strictJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.CreateIngredient(
			c,
			actor,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *Handler) listIngredients(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionIngredientRead,
	) {
		return
	}

	limit, _ :=
		strconv.Atoi(
			c.DefaultQuery(
				"limit",
				"50",
			),
		)

	out, err :=
		h.service.ListIngredients(
			c,
			actor,
			c.Query("q"),
			c.Query("status"),
			limit,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) getIngredient(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionIngredientRead,
	) {
		return
	}

	out, err :=
		h.service.GetIngredient(
			c,
			actor,
			c.Param("ingredientId"),
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) updateIngredient(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionIngredientManage,
	) {
		return
	}

	version, err :=
		expectedVersion(c)
	if err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				err.Error(),
				err,
			),
		)
		return
	}

	var input UpdateIngredientInput

	if err = strictJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.UpdateIngredient(
			c,
			actor,
			c.Param("ingredientId"),
			version,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	c.Header(
		"ETag",
		strconv.FormatInt(
			out.Version,
			10,
		),
	)

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) archiveIngredient(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionIngredientManage,
	) {
		return
	}

	version, err :=
		expectedVersion(c)
	if err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				err.Error(),
				err,
			),
		)
		return
	}

	out, err :=
		h.service.ArchiveIngredient(
			c,
			actor,
			c.Param("ingredientId"),
			version,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) createIngredientConversion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionIngredientManage,
	) {
		return
	}

	var input PutUnitConversionInput

	if err = strictJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.CreateIngredientConversion(
			c,
			actor,
			c.Param("ingredientId"),
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *Handler) listIngredientConversions(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionIngredientRead,
	) {
		return
	}

	out, err :=
		h.service.ListIngredientConversions(
			c,
			actor,
			c.Param("ingredientId"),
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) updateIngredientConversion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionIngredientManage,
	) {
		return
	}

	version, err :=
		expectedVersion(c)
	if err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				err.Error(),
				err,
			),
		)
		return
	}

	var input PutUnitConversionInput

	if err = strictJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.UpdateIngredientConversion(
			c,
			actor,
			c.Param("ingredientId"),
			c.Param("conversionId"),
			version,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) putIngredientBranchConfig(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID :=
		c.Param("branchId")

	if !h.authorizeBranch(
		c,
		actor,
		branchID,
		PermissionIngredientInventoryMapping,
	) {
		return
	}

	var input PutBranchConfigInput

	if err = strictJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.PutIngredientBranchConfig(
			c,
			actor,
			c.Param("ingredientId"),
			branchID,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) getIngredientBranchConfig(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID :=
		c.Param("branchId")

	if !h.authorizeBranch(
		c,
		actor,
		branchID,
		PermissionIngredientRead,
	) {
		return
	}

	out, err :=
		h.service.GetIngredientBranchConfig(
			c,
			actor,
			c.Param("ingredientId"),
			branchID,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) createPackaging(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionPackagingManage,
	) {
		return
	}

	var input CreatePackagingMaterialInput

	if err = strictJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.CreatePackagingMaterial(
			c,
			actor,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *Handler) listPackaging(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionPackagingRead,
	) {
		return
	}

	limit, _ :=
		strconv.Atoi(
			c.DefaultQuery(
				"limit",
				"50",
			),
		)

	out, err :=
		h.service.ListPackagingMaterials(
			c,
			actor,
			c.Query("q"),
			c.Query("status"),
			limit,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) getPackaging(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionPackagingRead,
	) {
		return
	}

	out, err :=
		h.service.GetPackagingMaterial(
			c,
			actor,
			c.Param("packagingId"),
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) updatePackaging(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionPackagingManage,
	) {
		return
	}

	version, err :=
		expectedVersion(c)
	if err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				err.Error(),
				err,
			),
		)
		return
	}

	var input UpdatePackagingMaterialInput

	if err = strictJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.UpdatePackagingMaterial(
			c,
			actor,
			c.Param("packagingId"),
			version,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) archivePackaging(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionPackagingManage,
	) {
		return
	}

	version, err :=
		expectedVersion(c)
	if err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				err.Error(),
				err,
			),
		)
		return
	}

	out, err :=
		h.service.ArchivePackagingMaterial(
			c,
			actor,
			c.Param("packagingId"),
			version,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) createPackagingConversion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionPackagingManage,
	) {
		return
	}

	var input PutUnitConversionInput

	if err = strictJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.CreatePackagingConversion(
			c,
			actor,
			c.Param("packagingId"),
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *Handler) listPackagingConversions(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionPackagingRead,
	) {
		return
	}

	out, err :=
		h.service.ListPackagingConversions(
			c,
			actor,
			c.Param("packagingId"),
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) updatePackagingConversion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if !h.authorizeBusiness(
		c,
		actor,
		PermissionPackagingManage,
	) {
		return
	}

	version, err :=
		expectedVersion(c)
	if err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				err.Error(),
				err,
			),
		)
		return
	}

	var input PutUnitConversionInput

	if err = strictJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.UpdatePackagingConversion(
			c,
			actor,
			c.Param("packagingId"),
			c.Param("conversionId"),
			version,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) putPackagingBranchConfig(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID :=
		c.Param("branchId")

	if !h.authorizeBranch(
		c,
		actor,
		branchID,
		PermissionPackagingInventoryMapping,
	) {
		return
	}

	var input PutBranchConfigInput

	if err = strictJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.PutPackagingBranchConfig(
			c,
			actor,
			c.Param("packagingId"),
			branchID,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) getPackagingBranchConfig(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID :=
		c.Param("branchId")

	if !h.authorizeBranch(
		c,
		actor,
		branchID,
		PermissionPackagingRead,
	) {
		return
	}

	out, err :=
		h.service.GetPackagingBranchConfig(
			c,
			actor,
			c.Param("packagingId"),
			branchID,
		)
	if err != nil {
		httpx.RespondError(
			c,
			handlerError(err),
		)
		return
	}

	success(
		c,
		http.StatusOK,
		out,
	)
}

func RegisterRoutes(
	root *gin.RouterGroup,
	h *Handler,
) {
	if h == nil {
		return
	}

	group := root.Group("")
	group.Use(
		idempotency.CommandMiddleware(
			h.idem,
		),
	)

	ingredients :=
		group.Group(
			"/ingredients",
		)

	ingredients.POST(
		"",
		h.createIngredient,
	)
	ingredients.GET(
		"",
		h.listIngredients,
	)
	ingredients.GET(
		"/:ingredientId",
		h.getIngredient,
	)
	ingredients.PUT(
		"/:ingredientId",
		h.updateIngredient,
	)
	ingredients.POST(
		"/:ingredientId/archive",
		h.archiveIngredient,
	)

	ingredients.GET(
		"/:ingredientId/conversions",
		h.listIngredientConversions,
	)
	ingredients.POST(
		"/:ingredientId/conversions",
		h.createIngredientConversion,
	)
	ingredients.PUT(
		"/:ingredientId/conversions/:conversionId",
		h.updateIngredientConversion,
	)

	ingredients.GET(
		"/:ingredientId/branches/:branchId",
		h.getIngredientBranchConfig,
	)
	ingredients.PUT(
		"/:ingredientId/branches/:branchId",
		h.putIngredientBranchConfig,
	)

	packaging :=
		group.Group(
			"/packaging-materials",
		)

	packaging.POST(
		"",
		h.createPackaging,
	)
	packaging.GET(
		"",
		h.listPackaging,
	)
	packaging.GET(
		"/:packagingId",
		h.getPackaging,
	)
	packaging.PUT(
		"/:packagingId",
		h.updatePackaging,
	)
	packaging.POST(
		"/:packagingId/archive",
		h.archivePackaging,
	)

	packaging.GET(
		"/:packagingId/conversions",
		h.listPackagingConversions,
	)
	packaging.POST(
		"/:packagingId/conversions",
		h.createPackagingConversion,
	)
	packaging.PUT(
		"/:packagingId/conversions/:conversionId",
		h.updatePackagingConversion,
	)

	packaging.GET(
		"/:packagingId/branches/:branchId",
		h.getPackagingBranchConfig,
	)
	packaging.PUT(
		"/:packagingId/branches/:branchId",
		h.putPackagingBranchConfig,
	)
}
