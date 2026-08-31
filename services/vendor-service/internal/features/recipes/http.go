package recipes

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

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
	body, err :=
		io.ReadAll(
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
		return 0,
			errors.New(
				"If-Match must contain a positive resource version",
			)
	}

	return version, nil
}

func recipeHTTPError(
	err error,
) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return apperrors.NotFound(
			"Recipe resource was not found.",
			err,
		)

	case errors.Is(err, ErrTargetInvalid):
		return apperrors.BadRequest(
			"The recipe target is invalid.",
			err,
		)

	case errors.Is(err, ErrVersionConflict):
		return apperrors.Conflict(
			"The recipe changed before this operation completed.",
			err,
		)

	case errors.Is(err, ErrInvalidState):
		return apperrors.Conflict(
			"The recipe cannot perform this operation in its current state.",
			err,
		)

	case errors.Is(err, ErrExactConversion):
		return apperrors.BadRequest(
			"An exact authoritative unit conversion is required.",
			err,
		)

	case errors.Is(err, ErrInvalidInput):
		return apperrors.BadRequest(
			"The recipe request is invalid.",
			err,
		)

	default:
		return err
	}
}

func recipeOK(
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
		StaffID: auth.StaffID,

		BusinessID: auth.VendorID,

		SessionID: auth.SessionID,

		CorrelationID: httpx.GetRequestID(c),

		CausationID: c.GetHeader(
			"X-Causation-ID",
		),

		RequestID: httpx.GetRequestID(c),
	}, nil
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
				"Recipe access is not permitted.",
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
				"Recipe access is not permitted for this branch.",
				nil,
			),
		)
		return false
	}

	return true
}

func (h *Handler) createRecipe(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	var input CreateRecipeInput

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

	target, err :=
		h.service.ResolveTarget(
			c,
			actor.BusinessID,
			input.TargetType,
			input.TargetRef,
			input.TargetVariantRef,
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return
	}

	if !h.authorizeBranch(
		c,
		actor,
		target.BranchID,
		PermissionManage,
	) {
		return
	}

	out, err :=
		h.service.CreateRecipe(
			c,
			actor,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
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

	recipeOK(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *Handler) listRecipes(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if h.scopes == nil {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Recipe access is not permitted.",
				nil,
			),
		)
		return
	}

	scope, err :=
		h.scopes.ResolveScope(
			c,
			actor.StaffID,
			PermissionRead,
		)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	limit, _ :=
		strconv.Atoi(
			c.DefaultQuery(
				"limit",
				"50",
			),
		)

	offset, _ :=
		strconv.Atoi(
			c.DefaultQuery(
				"offset",
				"0",
			),
		)

	items, err :=
		h.service.ListRecipes(
			c,
			actor,
			c.Query("q"),
			c.Query("target_type"),
			c.Query("status"),
			limit,
			offset,
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return
	}

	filtered :=
		make(
			[]Recipe,
			0,
			len(items),
		)

	for _, item := range items {
		if scope.Covers(
			item.BranchID,
		) {
			filtered =
				append(
					filtered,
					item,
				)
		}
	}

	recipeOK(
		c,
		http.StatusOK,
		filtered,
	)
}

func (h *Handler) loadAuthorizedRecipe(
	c *gin.Context,
	actor Actor,
	permission string,
) (Recipe, bool) {
	recipe, err :=
		h.service.GetRecipe(
			c,
			actor,
			c.Param("recipeId"),
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return Recipe{}, false
	}

	if !h.authorizeBranch(
		c,
		actor,
		recipe.BranchID,
		permission,
	) {
		return Recipe{}, false
	}

	return recipe, true
}

func (h *Handler) getRecipe(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	recipe, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionRead,
		)
	if !ok {
		return
	}

	recipeOK(
		c,
		http.StatusOK,
		recipe,
	)
}

func (h *Handler) archiveRecipe(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionManage,
		); !ok {
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
		h.service.ArchiveRecipe(
			c,
			actor,
			c.Param("recipeId"),
			version,
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return
	}

	recipeOK(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) createVersion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionManage,
		); !ok {
		return
	}

	var input CreateVersionInput

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
		h.service.CreateDraftVersion(
			c,
			actor,
			c.Param("recipeId"),
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
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

	recipeOK(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *Handler) listVersions(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionRead,
		); !ok {
		return
	}

	out, err :=
		h.service.ListVersions(
			c,
			actor,
			c.Param("recipeId"),
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return
	}

	recipeOK(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) getVersion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionRead,
		); !ok {
		return
	}

	out, err :=
		h.service.GetVersionView(
			c,
			actor,
			c.Param("recipeId"),
			c.Param("versionId"),
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return
	}

	recipeOK(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) updateVersion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionManage,
		); !ok {
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

	var input UpdateVersionInput

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
		h.service.UpdateDraftVersion(
			c,
			actor,
			c.Param("recipeId"),
			c.Param("versionId"),
			version,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
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

	recipeOK(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) replaceComponents(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionManage,
		); !ok {
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

	var input ReplaceComponentsInput

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
		h.service.ReplaceComponents(
			c,
			actor,
			c.Param("recipeId"),
			c.Param("versionId"),
			version,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return
	}

	recipeOK(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) replacePackaging(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionManage,
		); !ok {
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

	var input ReplacePackagingInput

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
		h.service.ReplacePackaging(
			c,
			actor,
			c.Param("recipeId"),
			c.Param("versionId"),
			version,
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return
	}

	recipeOK(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) validateVersion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionRead,
		); !ok {
		return
	}

	out, err :=
		h.service.ValidateVersion(
			c,
			actor,
			c.Param("recipeId"),
			c.Param("versionId"),
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return
	}

	recipeOK(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) activateVersion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionManage,
		); !ok {
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

	out, issues, err :=
		h.service.ActivateVersion(
			c,
			actor,
			c.Param("recipeId"),
			c.Param("versionId"),
			version,
		)

	if errors.Is(
		err,
		ErrValidationFailed,
	) {
		c.JSON(
			http.StatusUnprocessableEntity,
			gin.H{
				"success": false,
				"error": gin.H{
					"code":    "RECIPE_VALIDATION_FAILED",
					"message": "Recipe version cannot be activated.",
					"issues":  issues,
				},
			},
		)
		return
	}

	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
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

	recipeOK(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) retireVersion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionManage,
		); !ok {
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
		h.service.RetireVersion(
			c,
			actor,
			c.Param("recipeId"),
			c.Param("versionId"),
			version,
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return
	}

	recipeOK(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) scaleVersion(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	if _, ok :=
		h.loadAuthorizedRecipe(
			c,
			actor,
			PermissionRead,
		); !ok {
		return
	}

	var input ScaleInput

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
		h.service.ScaleVersion(
			c,
			actor,
			c.Param("recipeId"),
			c.Param("versionId"),
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			recipeHTTPError(err),
		)
		return
	}

	recipeOK(
		c,
		http.StatusOK,
		out,
	)
}

func RegisterRoutes(
	root *gin.RouterGroup,
	h *Handler,
	enabled bool,
) {
	if h == nil ||
		!enabled {
		return
	}

	group :=
		root.Group(
			"/recipes",
		)

	group.Use(
		idempotency.CommandMiddleware(
			h.idem,
		),
	)

	group.POST(
		"",
		h.createRecipe,
	)

	group.GET(
		"",
		h.listRecipes,
	)

	group.GET(
		"/:recipeId",
		h.getRecipe,
	)

	group.POST(
		"/:recipeId/archive",
		h.archiveRecipe,
	)

	group.POST(
		"/:recipeId/versions",
		h.createVersion,
	)

	group.GET(
		"/:recipeId/versions",
		h.listVersions,
	)

	group.GET(
		"/:recipeId/versions/:versionId",
		h.getVersion,
	)

	group.PUT(
		"/:recipeId/versions/:versionId",
		h.updateVersion,
	)

	group.PUT(
		"/:recipeId/versions/:versionId/components",
		h.replaceComponents,
	)

	group.PUT(
		"/:recipeId/versions/:versionId/packaging",
		h.replacePackaging,
	)

	group.POST(
		"/:recipeId/versions/:versionId/validate",
		h.validateVersion,
	)

	group.POST(
		"/:recipeId/versions/:versionId/activate",
		h.activateVersion,
	)

	group.POST(
		"/:recipeId/versions/:versionId/retire",
		h.retireVersion,
	)

	group.POST(
		"/:recipeId/versions/:versionId/scale",
		h.scaleVersion,
	)
}
