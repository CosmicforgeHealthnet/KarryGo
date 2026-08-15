package food

import (
	"encoding/json"
	"errors"
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

// Handler exposes the Section 9A vendor-management surface.
//
// It is management only. A GET here is a staff member reading their own
// catalogue behind a session and a permission; it is not customer discovery,
// which Section 9C will design with its own read contract.
type Handler struct {
	service     *Service
	idempotency *idempotency.Store
}

func NewHandler(service *Service, store *idempotency.Store) *Handler {
	return &Handler{service: service, idempotency: store}
}

func (h *Handler) actor(c *gin.Context) (Actor, error) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		return Actor{}, apperrors.Unauthorized("Authentication is required.", nil)
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

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// readLimitedBody is used by read-only POST previews as well as commands.
// Keep the same one-megabyte bound used by command so a pricing preview cannot
// become an unbounded request-body sink.
func readLimitedBody(c *gin.Context) ([]byte, error) {
	return io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
}

// authorized resolves the actor and their scope, responding on failure.
func (h *Handler) authorized(c *gin.Context, permission string) (Actor, Scope, bool) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return Actor{}, Scope{}, false
	}
	scope, err := h.service.Authorize(c, actor, permission)
	if err != nil {
		httpx.RespondError(c, err)
		return Actor{}, Scope{}, false
	}
	return actor, scope, true
}

// command runs a state-changing operation behind the shared Idempotency-Key
// store — the same store, the same key scoping and the same replay/conflict
// semantics Sections 6, 7 and 8 use. There is no second idempotency mechanism
// in this service.
func (h *Handler) command(c *gin.Context, status int, fn func(Actor, []byte) (any, error)) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Could not read request body.", err))
		return
	}
	key := c.GetHeader("Idempotency-Key")
	endpoint := c.Request.Method + " " + c.FullPath()
	fingerprint := idempotency.Fingerprint(c.Request.Method, c.Request.URL.Path, raw)
	record, err := h.idempotency.Begin(c, actor.VendorID, endpoint, key, fingerprint)
	if err != nil {
		if errors.Is(err, idempotency.ErrFingerprintMismatch) || errors.Is(err, idempotency.ErrInProgress) {
			httpx.RespondError(c, coded(
				apperrors.Conflict("This idempotency key is already in use for another request.", err),
				"FOOD_IDEMPOTENCY_CONFLICT"))
		} else {
			httpx.RespondError(c, apperrors.BadRequest("Idempotency-Key is required.", err))
		}
		return
	}
	if record != nil {
		c.Data(record.ResponseStatus, "application/json", record.ResponseBody)
		return
	}
	data, err := fn(actor, raw)
	if err != nil {
		_ = h.idempotency.Release(c, actor.VendorID, endpoint, key)
		httpx.RespondError(c, err)
		return
	}
	body, err := json.Marshal(gin.H{"success": true, "data": data})
	if err != nil {
		_ = h.idempotency.Release(c, actor.VendorID, endpoint, key)
		httpx.RespondError(c, err)
		return
	}
	if err = h.idempotency.Complete(c, actor.VendorID, endpoint, key, status, body); err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.Data(status, "application/json", body)
}

func decodeBody(raw []byte, target any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return apperrors.BadRequest("The request body could not be read.", err)
	}
	return nil
}

func pageFrom(c *gin.Context) (int, int, error) {
	limit, offset := 0, 0
	var violations []apperrors.FieldViolation
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			violations = append(violations, apperrors.FieldViolation{Field: "limit", Message: "Limit must be from 1 to 100."})
		} else {
			limit = value
		}
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			violations = append(violations, apperrors.FieldViolation{Field: "offset", Message: "Offset must be zero or greater."})
		} else {
			offset = value
		}
	}
	if len(violations) > 0 {
		return 0, 0, apperrors.Validation("Check your details.", violations)
	}
	return limit, offset, nil
}

func includeArchivedFrom(c *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(c.Query("include_archived")), "true")
}

// ---------------------------------------------------------------------------
// Menus
// ---------------------------------------------------------------------------

type createMenuRequest struct {
	BranchID       string `json:"branch_id"`
	CapabilityType string `json:"capability_type"`
	Name           string `json:"name"`
	Description    string `json:"description"`
}

func (h *Handler) CreateMenu(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req createMenuRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		menu, err := h.service.CreateMenu(c, actor, scope, CreateMenuInput{
			BranchID: req.BranchID, CapabilityType: req.CapabilityType,
			Name: req.Name, Description: req.Description})
		if err != nil {
			return nil, err
		}
		return MenuViewOf(menu), nil
	})
}

func (h *Handler) ListMenus(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	limit, offset, err := pageFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	menus, hasMore, err := h.service.ListMenus(c, actor, scope, MenuListOptions{
		BranchID: c.Query("branch_id"), CapabilityType: c.Query("capability_type"),
		Status: c.Query("status"), Limit: limit, Offset: offset})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]MenuView, 0, len(menus))
	for _, menu := range menus {
		views = append(views, MenuViewOf(menu))
	}
	bounded := MenuListOptions{Limit: limit, Offset: offset}.bounded()
	ok(c, gin.H{"menus": views, "pagination": gin.H{
		"limit": bounded.Limit, "offset": bounded.Offset, "has_more": hasMore}})
}

func (h *Handler) GetMenu(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	menu, err := h.service.GetMenu(c, actor, scope, c.Param("menuId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, MenuViewOf(menu))
}

type updateMenuRequest struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	ExpectedVersion int64   `json:"expected_version"`
}

func (h *Handler) UpdateMenu(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req updateMenuRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		menu, err := h.service.UpdateMenu(c, actor, scope, c.Param("menuId"), UpdateMenuInput{
			Name: req.Name, Description: req.Description, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return MenuViewOf(menu), nil
	})
}

type versionedRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

// menuStatusCommand backs activate/pause/archive. Publishing needs the publish
// permission; archiving is also a publication decision (it withdraws a menu),
// so it shares it. Pausing does too, for the same reason.
func (h *Handler) menuStatusCommand(target string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
			scope, err := h.service.Authorize(c, actor, PermCataloguePublish)
			if err != nil {
				return nil, err
			}
			var req versionedRequest
			if err = decodeBody(raw, &req); err != nil {
				return nil, err
			}
			menu, err := h.service.SetMenuStatus(c, actor, scope, c.Param("menuId"), target, req.ExpectedVersion)
			if err != nil {
				return nil, err
			}
			return MenuViewOf(menu), nil
		})
	}
}

// ---------------------------------------------------------------------------
// Sections
// ---------------------------------------------------------------------------

type createSectionRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) CreateSection(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req createSectionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		section, err := h.service.CreateSection(c, actor, scope, c.Param("menuId"),
			CreateSectionInput{Name: req.Name, Description: req.Description})
		if err != nil {
			return nil, err
		}
		return SectionViewOf(section), nil
	})
}

func (h *Handler) ListSections(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	sections, err := h.service.ListSections(c, actor, scope, c.Param("menuId"), includeArchivedFrom(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]SectionView, 0, len(sections))
	for _, section := range sections {
		views = append(views, SectionViewOf(section))
	}
	ok(c, gin.H{"sections": views})
}

type updateSectionRequest struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	ExpectedVersion int64   `json:"expected_version"`
}

func (h *Handler) UpdateSection(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req updateSectionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		section, err := h.service.UpdateSection(c, actor, scope, c.Param("menuId"), c.Param("sectionId"),
			UpdateSectionInput{Name: req.Name, Description: req.Description, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return SectionViewOf(section), nil
	})
}

func (h *Handler) ArchiveSection(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req versionedRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		section, err := h.service.ArchiveSection(c, actor, scope, c.Param("menuId"), c.Param("sectionId"), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return SectionViewOf(section), nil
	})
}

type reorderRequest struct {
	OrderedIDs []string `json:"ordered_ids"`
}

func (h *Handler) ReorderSections(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req reorderRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		sections, err := h.service.ReorderSections(c, actor, scope, c.Param("menuId"), req.OrderedIDs)
		if err != nil {
			return nil, err
		}
		views := make([]SectionView, 0, len(sections))
		for _, section := range sections {
			views = append(views, SectionViewOf(section))
		}
		return gin.H{"sections": views}, nil
	})
}

// ---------------------------------------------------------------------------
// Items
// ---------------------------------------------------------------------------

type createItemRequest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	BasePriceMinor int64    `json:"base_price_minor"`
	DietaryCodes   []string `json:"dietary_codes"`
	AllergenCodes  []string `json:"allergen_codes"`
}

func (h *Handler) CreateItem(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req createItemRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		item, err := h.service.CreateItem(c, actor, scope, c.Param("menuId"), c.Param("sectionId"), CreateItemInput{
			Name: req.Name, Description: req.Description, BasePriceMinor: req.BasePriceMinor,
			DietaryCodes: req.DietaryCodes, AllergenCodes: req.AllergenCodes})
		if err != nil {
			return nil, err
		}
		currency, err := h.service.businessCurrency(c, h.service.db, actor.VendorID)
		if err != nil {
			return nil, err
		}
		return ItemViewOf(item, currency, nil), nil
	})
}

func (h *Handler) ListItems(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	limit, offset, err := pageFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, media, currency, hasMore, err := h.service.ListItems(c, actor, scope,
		c.Param("menuId"), c.Param("sectionId"), includeArchivedFrom(c), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]ItemView, 0, len(items))
	for _, item := range items {
		views = append(views, ItemViewOf(item, currency, media[item.ID]))
	}
	if limit <= 0 {
		limit = 50
	}
	ok(c, gin.H{"items": views, "pagination": gin.H{
		"limit": limit, "offset": offset, "has_more": hasMore}})
}

func (h *Handler) GetItem(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	item, media, currency, err := h.service.GetItem(c, actor, scope, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, ItemViewOf(item, currency, media))
}

type updateItemRequest struct {
	Name            *string   `json:"name,omitempty"`
	Description     *string   `json:"description,omitempty"`
	BasePriceMinor  *int64    `json:"base_price_minor,omitempty"`
	DietaryCodes    *[]string `json:"dietary_codes,omitempty"`
	AllergenCodes   *[]string `json:"allergen_codes,omitempty"`
	Status          *string   `json:"status,omitempty"`
	ExpectedVersion int64     `json:"expected_version"`
}

func (h *Handler) UpdateItem(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req updateItemRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		// Publishing an item is a publication decision, so it needs the publish
		// grant on top of manage. Editing its text and price does not.
		if req.Status != nil && strings.EqualFold(strings.TrimSpace(*req.Status), StatusActive) {
			if _, err = h.service.Authorize(c, actor, PermCataloguePublish); err != nil {
				return nil, err
			}
		}
		item, err := h.service.UpdateItem(c, actor, scope, c.Param("itemId"), UpdateItemInput{
			Name: req.Name, Description: req.Description, BasePriceMinor: req.BasePriceMinor,
			DietaryCodes: req.DietaryCodes, AllergenCodes: req.AllergenCodes, Status: req.Status,
			ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		currency, err := h.service.businessCurrency(c, h.service.db, actor.VendorID)
		if err != nil {
			return nil, err
		}
		media, err := h.service.mediaForItems(c, h.service.db, actor.VendorID, []string{item.ID})
		if err != nil {
			return nil, err
		}
		return ItemViewOf(item, currency, media[item.ID]), nil
	})
}

func (h *Handler) ArchiveItem(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req versionedRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		item, err := h.service.ArchiveItem(c, actor, scope, c.Param("itemId"), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return ItemViewOf(item, "", nil), nil
	})
}

func (h *Handler) ReorderItems(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req reorderRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		items, err := h.service.ReorderItems(c, actor, scope, c.Param("menuId"), c.Param("sectionId"), req.OrderedIDs)
		if err != nil {
			return nil, err
		}
		order := make([]string, 0, len(items))
		for _, item := range items {
			order = append(order, item.ID)
		}
		return gin.H{"ordered_item_ids": order}, nil
	})
}

// ---------------------------------------------------------------------------
// Item media
// ---------------------------------------------------------------------------

type replaceMediaRequest struct {
	Media []struct {
		MediaRef  string `json:"media_ref"`
		IsPrimary bool   `json:"is_primary"`
	} `json:"media"`
	ExpectedVersion int64 `json:"expected_version"`
}

func (h *Handler) ListItemMedia(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	media, err := h.service.ListItemMedia(c, actor, scope, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, gin.H{"media": mediaViewsOf(media)})
}

// ReplaceItemMedia sets the whole ordered reference list. There is deliberately
// no upload route here: bytes belong to Media File Service, and this endpoint
// accepts only references it has already issued.
func (h *Handler) ReplaceItemMedia(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req replaceMediaRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		refs := make([]MediaRefInput, 0, len(req.Media))
		for _, entry := range req.Media {
			refs = append(refs, MediaRefInput{MediaRef: entry.MediaRef, IsPrimary: entry.IsPrimary})
		}
		item, media, err := h.service.ReplaceItemMedia(c, actor, scope, c.Param("itemId"), refs, req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return gin.H{"item_id": item.ID, "version": item.Version, "media": mediaViewsOf(media)}, nil
	})
}

// ---------------------------------------------------------------------------
// Variants
// ---------------------------------------------------------------------------

type createVariantRequest struct {
	Name               string `json:"name"`
	PriceOverrideMinor *int64 `json:"price_override_minor,omitempty"`
}

func (h *Handler) CreateVariant(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req createVariantRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		variant, err := h.service.CreateVariant(c, actor, scope, c.Param("itemId"),
			CreateVariantInput{Name: req.Name, PriceOverrideMinor: req.PriceOverrideMinor})
		if err != nil {
			return nil, err
		}
		return VariantViewOf(variant), nil
	})
}

func (h *Handler) ListVariants(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	variants, err := h.service.ListVariants(c, actor, scope, c.Param("itemId"), includeArchivedFrom(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]VariantView, 0, len(variants))
	for _, variant := range variants {
		views = append(views, VariantViewOf(variant))
	}
	ok(c, gin.H{"variants": views})
}

type updateVariantRequest struct {
	Name               *string `json:"name,omitempty"`
	PriceOverrideMinor *int64  `json:"price_override_minor,omitempty"`
	ClearPriceOverride bool    `json:"clear_price_override,omitempty"`
	ExpectedVersion    int64   `json:"expected_version"`
}

func (h *Handler) UpdateVariant(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req updateVariantRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		variant, err := h.service.UpdateVariant(c, actor, scope, c.Param("itemId"), c.Param("variantId"),
			UpdateVariantInput{Name: req.Name, PriceOverrideMinor: req.PriceOverrideMinor,
				ClearPriceOverride: req.ClearPriceOverride, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return VariantViewOf(variant), nil
	})
}

func (h *Handler) ArchiveVariant(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req versionedRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		variant, err := h.service.ArchiveVariant(c, actor, scope, c.Param("itemId"), c.Param("variantId"), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return VariantViewOf(variant), nil
	})
}

func (h *Handler) ReorderVariants(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req reorderRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		variants, err := h.service.ReorderVariants(c, actor, scope, c.Param("itemId"), req.OrderedIDs)
		if err != nil {
			return nil, err
		}
		order := make([]string, 0, len(variants))
		for _, variant := range variants {
			order = append(order, variant.ID)
		}
		return gin.H{"ordered_variant_ids": order}, nil
	})
}

// ---------------------------------------------------------------------------
// Modifier groups and modifiers
// ---------------------------------------------------------------------------

type createModifierGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	MinSelect   int    `json:"min_select"`
	MaxSelect   int    `json:"max_select"`
}

func (h *Handler) CreateModifierGroup(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req createModifierGroupRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		group, err := h.service.CreateModifierGroup(c, actor, scope, CreateModifierGroupInput{
			Name: req.Name, Description: req.Description, MinSelect: req.MinSelect, MaxSelect: req.MaxSelect})
		if err != nil {
			return nil, err
		}
		return ModifierGroupViewOf(group), nil
	})
}

func (h *Handler) ListModifierGroups(c *gin.Context) {
	actor, _, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	limit, offset, err := pageFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	groups, hasMore, err := h.service.ListModifierGroups(c, actor, includeArchivedFrom(c), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]ModifierGroupView, 0, len(groups))
	for _, group := range groups {
		views = append(views, ModifierGroupViewOf(group))
	}
	if limit <= 0 {
		limit = 50
	}
	ok(c, gin.H{"modifier_groups": views, "pagination": gin.H{
		"limit": limit, "offset": offset, "has_more": hasMore}})
}

func (h *Handler) GetModifierGroup(c *gin.Context) {
	actor, _, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	group, err := h.service.GetModifierGroup(c, actor, c.Param("groupId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, ModifierGroupViewOf(group))
}

type updateModifierGroupRequest struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	MinSelect       *int    `json:"min_select,omitempty"`
	MaxSelect       *int    `json:"max_select,omitempty"`
	ExpectedVersion int64   `json:"expected_version"`
}

func (h *Handler) UpdateModifierGroup(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req updateModifierGroupRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		group, err := h.service.UpdateModifierGroup(c, actor, scope, c.Param("groupId"), UpdateModifierGroupInput{
			Name: req.Name, Description: req.Description, MinSelect: req.MinSelect, MaxSelect: req.MaxSelect,
			ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return ModifierGroupViewOf(group), nil
	})
}

func (h *Handler) ArchiveModifierGroup(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req versionedRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		group, err := h.service.ArchiveModifierGroup(c, actor, scope, c.Param("groupId"), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return ModifierGroupViewOf(group), nil
	})
}

type createModifierRequest struct {
	Name            string `json:"name"`
	PriceDeltaMinor int64  `json:"price_delta_minor"`
}

func (h *Handler) CreateModifier(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req createModifierRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		modifier, err := h.service.CreateModifier(c, actor, scope, c.Param("groupId"),
			CreateModifierInput{Name: req.Name, PriceDeltaMinor: req.PriceDeltaMinor})
		if err != nil {
			return nil, err
		}
		return ModifierViewOf(modifier), nil
	})
}

func (h *Handler) ListModifiers(c *gin.Context) {
	actor, _, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	modifiers, err := h.service.ListModifiers(c, actor, c.Param("groupId"), includeArchivedFrom(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]ModifierView, 0, len(modifiers))
	for _, modifier := range modifiers {
		views = append(views, ModifierViewOf(modifier))
	}
	ok(c, gin.H{"modifiers": views})
}

type updateModifierRequest struct {
	Name            *string `json:"name,omitempty"`
	PriceDeltaMinor *int64  `json:"price_delta_minor,omitempty"`
	ExpectedVersion int64   `json:"expected_version"`
}

func (h *Handler) UpdateModifier(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req updateModifierRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		modifier, err := h.service.UpdateModifier(c, actor, scope, c.Param("groupId"), c.Param("modifierId"),
			UpdateModifierInput{Name: req.Name, PriceDeltaMinor: req.PriceDeltaMinor, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return ModifierViewOf(modifier), nil
	})
}

func (h *Handler) ArchiveModifier(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req versionedRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		modifier, err := h.service.ArchiveModifier(c, actor, scope, c.Param("groupId"), c.Param("modifierId"), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return ModifierViewOf(modifier), nil
	})
}

func (h *Handler) ReorderModifiers(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req reorderRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		modifiers, err := h.service.ReorderModifiers(c, actor, scope, c.Param("groupId"), req.OrderedIDs)
		if err != nil {
			return nil, err
		}
		order := make([]string, 0, len(modifiers))
		for _, modifier := range modifiers {
			order = append(order, modifier.ID)
		}
		return gin.H{"ordered_modifier_ids": order}, nil
	})
}

// ---------------------------------------------------------------------------
// Item <-> modifier group relationships
// ---------------------------------------------------------------------------

func (h *Handler) ListAttachedModifierGroups(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	attached, err := h.service.ListAttachedModifierGroups(c, actor, scope, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, gin.H{"modifier_groups": attached})
}

// AttachModifierGroup is a PUT because the relationship is identified entirely
// by its path: repeating the same call is the same statement about the world,
// and the duplicate is reported rather than silently creating a second link.
func (h *Handler) AttachModifierGroup(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		attachment, err := h.service.AttachModifierGroup(c, actor, scope, c.Param("itemId"), c.Param("groupId"))
		if err != nil {
			return nil, err
		}
		return gin.H{"item_id": attachment.ItemID, "modifier_group_id": attachment.ModifierGroupID,
			"position": attachment.Position, "version": attachment.Version}, nil
	})
}

func (h *Handler) DetachModifierGroup(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		if err = h.service.DetachModifierGroup(c, actor, scope, c.Param("itemId"), c.Param("groupId")); err != nil {
			return nil, err
		}
		return gin.H{"item_id": strings.TrimSpace(c.Param("itemId")),
			"modifier_group_id": strings.TrimSpace(c.Param("groupId")), "detached": true}, nil
	})
}

func (h *Handler) ReorderAttachedModifierGroups(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCatalogueManage)
		if err != nil {
			return nil, err
		}
		var req reorderRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		attached, err := h.service.ReorderAttachedModifierGroups(c, actor, scope, c.Param("itemId"), req.OrderedIDs)
		if err != nil {
			return nil, err
		}
		order := make([]string, 0, len(attached))
		for _, entry := range attached {
			order = append(order, entry.ModifierGroupID)
		}
		return gin.H{"ordered_modifier_group_ids": order}, nil
	})
}

// RegisterRoutes wires the Section 9A management surface under the
// authenticated vendor API group.
//
// Nothing is registered when the feature is disabled, so a disabled catalogue
// has no attack surface at all rather than routes that answer 503 and thereby
// confirm the feature exists. Registration depends only on the flag, never on
// anything request-scoped, so the route set stays deterministic for
// OpenAPI/route parity.
func RegisterRoutes(group *gin.RouterGroup, handler *Handler, flags Flags) {
	if !flags.CatalogueEnabled {
		return
	}
	food := group.Group("/food")

	menus := food.Group("/menus")
	menus.POST("", handler.CreateMenu)
	menus.GET("", handler.ListMenus)
	menus.GET("/:menuId", handler.GetMenu)
	menus.PATCH("/:menuId", handler.UpdateMenu)
	menus.POST("/:menuId/activate", handler.menuStatusCommand(StatusActive))
	menus.POST("/:menuId/pause", handler.menuStatusCommand(StatusPaused))
	menus.POST("/:menuId/archive", handler.menuStatusCommand(StatusArchived))

	menus.POST("/:menuId/sections", handler.CreateSection)
	menus.GET("/:menuId/sections", handler.ListSections)
	menus.POST("/:menuId/sections/reorder", handler.ReorderSections)
	menus.PATCH("/:menuId/sections/:sectionId", handler.UpdateSection)
	menus.POST("/:menuId/sections/:sectionId/archive", handler.ArchiveSection)

	menus.POST("/:menuId/sections/:sectionId/items", handler.CreateItem)
	menus.GET("/:menuId/sections/:sectionId/items", handler.ListItems)
	menus.POST("/:menuId/sections/:sectionId/items/reorder", handler.ReorderItems)

	items := food.Group("/items")
	items.GET("/:itemId", handler.GetItem)
	items.PATCH("/:itemId", handler.UpdateItem)
	items.POST("/:itemId/archive", handler.ArchiveItem)
	items.GET("/:itemId/media", handler.ListItemMedia)
	items.PUT("/:itemId/media", handler.ReplaceItemMedia)

	items.POST("/:itemId/variants", handler.CreateVariant)
	items.GET("/:itemId/variants", handler.ListVariants)
	items.POST("/:itemId/variants/reorder", handler.ReorderVariants)
	items.PATCH("/:itemId/variants/:variantId", handler.UpdateVariant)
	items.POST("/:itemId/variants/:variantId/archive", handler.ArchiveVariant)

	items.GET("/:itemId/modifier-groups", handler.ListAttachedModifierGroups)
	items.POST("/:itemId/modifier-groups/reorder", handler.ReorderAttachedModifierGroups)
	items.PUT("/:itemId/modifier-groups/:groupId", handler.AttachModifierGroup)
	items.DELETE("/:itemId/modifier-groups/:groupId", handler.DetachModifierGroup)

	groups := food.Group("/modifier-groups")
	groups.POST("", handler.CreateModifierGroup)
	groups.GET("", handler.ListModifierGroups)
	groups.GET("/:groupId", handler.GetModifierGroup)
	groups.PATCH("/:groupId", handler.UpdateModifierGroup)
	groups.POST("/:groupId/archive", handler.ArchiveModifierGroup)

	groups.POST("/:groupId/modifiers", handler.CreateModifier)
	groups.GET("/:groupId/modifiers", handler.ListModifiers)
	groups.POST("/:groupId/modifiers/reorder", handler.ReorderModifiers)
	groups.PATCH("/:groupId/modifiers/:modifierId", handler.UpdateModifier)
	groups.POST("/:groupId/modifiers/:modifierId/archive", handler.ArchiveModifier)
}
