package shop

import (
	"bytes"
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

// Handler exposes authenticated Shop catalogue and vendor-owned inventory
// management. Consumer discovery remains on the root-mounted HMAC contract.
type Handler struct {
	service *Service
	store   *idempotency.Store
}

func NewHandler(service *Service, store *idempotency.Store) *Handler {
	return &Handler{service: service, store: store}
}

func (h *Handler) actor(c *gin.Context) (Actor, error) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		return Actor{}, apperrors.Unauthorized("Authentication is required.", nil)
	}
	return Actor{StaffID: staff.StaffID, VendorID: staff.VendorID, SessionID: staff.SessionID,
		CorrelationID: httpx.GetRequestID(c), CausationID: c.GetHeader("X-Causation-ID"),
		CommandRef: c.GetHeader("Idempotency-Key")}, nil
}

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

func respond(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"success": true, "data": data})
}

func respondPage(c *gin.Context, data any, hasMore bool) {
	respond(c, http.StatusOK, gin.H{"items": data, "has_more": hasMore})
}

func (h *Handler) command(c *gin.Context, permission string, status int,
	fn func(Actor, Scope, []byte) (any, error)) {
	actor, scope, ok := h.authorized(c, permission)
	if !ok {
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
	if h.store == nil {
		httpx.RespondError(c, apperrors.Unavailable("Idempotency storage is unavailable.", nil))
		return
	}
	record, err := h.store.Begin(c, actor.VendorID, endpoint, key, fingerprint)
	if err != nil {
		if errors.Is(err, idempotency.ErrFingerprintMismatch) || errors.Is(err, idempotency.ErrInProgress) {
			httpx.RespondError(c, coded(apperrors.Conflict("This idempotency key is already in use for another request.", err), CodeIdempotencyConflict))
		} else {
			httpx.RespondError(c, apperrors.BadRequest("Idempotency-Key is required.", err))
		}
		return
	}
	if record != nil {
		c.Data(record.ResponseStatus, "application/json", record.ResponseBody)
		return
	}
	data, err := fn(actor, scope, raw)
	if err != nil {
		_ = h.store.Release(c, actor.VendorID, endpoint, key)
		httpx.RespondError(c, err)
		return
	}
	body, err := json.Marshal(gin.H{"success": true, "data": data})
	if err != nil {
		_ = h.store.Release(c, actor.VendorID, endpoint, key)
		httpx.RespondError(c, err)
		return
	}
	if err = h.store.Complete(c, actor.VendorID, endpoint, key, status, body); err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.Data(status, "application/json", body)
}

// decode accepts explicit snake_case transport tags where present and retains
// the legacy normalized-field path for the tag-free Section 12A commands.
// Both paths reject unknown fields; trying the tagged form first also preserves
// raw nested batch rows exactly as submitted.
func decode(raw []byte, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}
	direct := json.NewDecoder(bytes.NewReader(raw))
	direct.DisallowUnknownFields()
	if err := direct.Decode(target); err == nil {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return apperrors.BadRequest("The request body could not be read.", err)
	}
	value = normalizeKeys(value)
	normalized, err := json.Marshal(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return apperrors.BadRequest("The request body could not be read.", err)
	}
	return nil
}

func explicitNull(raw []byte, key string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	value, ok := fields[key]
	return ok && bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func normalizeKeys(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			parts := strings.Split(key, "_")
			for i := range parts {
				if parts[i] != "" {
					parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
				}
			}
			out[strings.Join(parts, "")] = normalizeKeys(child)
		}
		return out
	case []any:
		for i := range typed {
			typed[i] = normalizeKeys(typed[i])
		}
	}
	return value
}

func queryInt(c *gin.Context, name string, fallback, minimum, maximum int) (int, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || (maximum > 0 && value > maximum) {
		return 0, fieldError(name, "Provide a supported whole number.")
	}
	return value, nil
}

func page(c *gin.Context) (int, int, error) {
	limit, err := queryInt(c, "limit", 50, 1, 100)
	if err != nil {
		return 0, 0, err
	}
	offset, err := queryInt(c, "offset", 0, 0, 0)
	return limit, offset, err
}

func includeArchived(c *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(c.Query("include_archived")), "true")
}

func mapSlice[T any, V any](items []T, view func(T) V) []V {
	out := make([]V, 0, len(items))
	for _, item := range items {
		out = append(out, view(item))
	}
	return out
}

type versionCommand struct {
	ExpectedVersion int64
}

type aliasCommand struct {
	Alias      string
	RegionCode string
}

type identifierCommand struct {
	IdentifierType  string
	IdentifierValue string
	PackageLevel    string
}

type packageCommand struct {
	ParentVariantID string
	ChildVariantID  string
	UnitsPerParent  int
}

func (h *Handler) createCategory(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in CreateCategoryInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.CreateCategory(c, a, s, in)
		return CategoryViewOf(v), err
	})
}

func (h *Handler) listCategories(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListCategories(c, a, s, CategoryListOptions{ParentID: c.Query("parent_id"), IncludeArchived: includeArchived(c), Limit: limit, Offset: offset})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, mapSlice(items, CategoryViewOf), more)
}

func (h *Handler) getCategory(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	v, err := h.service.GetCategory(c, a, s, c.Param("categoryId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, CategoryViewOf(v))
}

func (h *Handler) updateCategory(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in UpdateCategoryInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.UpdateCategory(c, a, s, c.Param("categoryId"), in)
		return CategoryViewOf(v), err
	})
}

func (h *Handler) categoryLifecycle(restore bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.command(c, PermCatalogueManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
			var in versionCommand
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			var v Category
			var err error
			if restore {
				v, err = h.service.RestoreCategory(c, a, s, c.Param("categoryId"), in.ExpectedVersion)
			} else {
				v, err = h.service.ArchiveCategory(c, a, s, c.Param("categoryId"), in.ExpectedVersion)
			}
			return CategoryViewOf(v), err
		})
	}
}

func (h *Handler) createMeasure(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in CreateMeasureInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.CreateMeasure(c, a, s, in)
		return MeasureViewOf(v), err
	})
}

func (h *Handler) listMeasures(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListMeasures(c, a, s, MeasureListOptions{Family: c.Query("family"), Origin: c.Query("origin"), Search: c.Query("search"), IncludeArchived: includeArchived(c), Limit: limit, Offset: offset})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, mapSlice(items, MeasureViewOf), more)
}

func (h *Handler) getMeasure(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	v, aliases, err := h.service.GetMeasure(c, a, s, c.Param("measureId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"measurement": MeasureViewOf(v), "aliases": mapSlice(aliases, AliasViewOf)})
}

func (h *Handler) updateMeasure(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in UpdateMeasureInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.UpdateMeasure(c, a, s, c.Param("measureId"), in)
		return MeasureViewOf(v), err
	})
}

func (h *Handler) archiveMeasure(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchiveMeasure(c, a, s, c.Param("measureId"), in.ExpectedVersion)
		return MeasureViewOf(v), err
	})
}

func (h *Handler) listAliases(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	items, err := h.service.ListMeasureAliases(c, a, s, c.Param("measureId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, mapSlice(items, AliasViewOf))
}

func (h *Handler) addAlias(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in aliasCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.AddMeasureAlias(c, a, s, c.Param("measureId"), in.Alias, in.RegionCode)
		return AliasViewOf(v), err
	})
}

func (h *Handler) removeAlias(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		err := h.service.RemoveMeasureAlias(c, a, s, c.Param("measureId"), c.Param("aliasId"))
		return gin.H{"removed": err == nil}, err
	})
}

func (h *Handler) createProduct(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in CreateProductInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.CreateProduct(c, a, s, in)
		return ProductViewOf(v), err
	})
}

func (h *Handler) listProducts(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListProducts(c, a, s, ProductListOptions{CategoryID: c.Query("category_id"), ProductClass: c.Query("product_class"), Brand: c.Query("brand"), Status: c.Query("status"), Search: c.Query("search"), IncludeArchived: includeArchived(c), Limit: limit, Offset: offset})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, mapSlice(items, ProductViewOf), more)
}

func (h *Handler) getProduct(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	v, err := h.service.GetProduct(c, a, s, c.Param("productId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, ProductViewOf(v))
}

func (h *Handler) updateProduct(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in UpdateProductInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.UpdateProduct(c, a, s, c.Param("productId"), in)
		return ProductViewOf(v), err
	})
}

func (h *Handler) archiveProduct(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchiveProduct(c, a, s, c.Param("productId"), in.ExpectedVersion)
		return ProductViewOf(v), err
	})
}

func (h *Handler) createVariant(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in CreateVariantInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.CreateVariant(c, a, s, c.Param("productId"), in)
		return VariantViewOf(v), err
	})
}

func (h *Handler) listVariants(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	items, err := h.service.ListVariants(c, a, s, c.Param("productId"), includeArchived(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, mapSlice(items, VariantViewOf))
}

func (h *Handler) getVariant(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	v, err := h.service.GetVariant(c, a, s, c.Param("variantId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, VariantViewOf(v))
}

func (h *Handler) updateVariant(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in UpdateVariantInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if explicitNull(raw, "retail_price_minor") {
			in.RetailPriceMinor = new(*int64)
		}
		if explicitNull(raw, "wholesale_price_minor") {
			in.WholesalePriceMinor = new(*int64)
		}
		v, err := h.service.UpdateVariant(c, a, s, c.Param("productId"), c.Param("variantId"), in)
		return VariantViewOf(v), err
	})
}

func (h *Handler) archiveVariant(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchiveVariant(c, a, s, c.Param("productId"), c.Param("variantId"), in.ExpectedVersion)
		return VariantViewOf(v), err
	})
}

func (h *Handler) addIdentifier(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in identifierCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.AddIdentifier(c, a, s, c.Param("variantId"), in.IdentifierType, in.IdentifierValue, in.PackageLevel)
		return IdentifierViewOf(v), err
	})
}

func (h *Handler) listIdentifiers(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	items, err := h.service.ListIdentifiers(c, a, s, c.Param("variantId"), includeArchived(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, mapSlice(items, IdentifierViewOf))
}

func (h *Handler) archiveIdentifier(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchiveIdentifier(c, a, s, c.Param("variantId"), c.Param("identifierId"), in.ExpectedVersion)
		return IdentifierViewOf(v), err
	})
}

func (h *Handler) lookupIdentifier(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	i, v, p, err := h.service.LookupByIdentifier(c, a, s, c.Query("value"), c.Query("type"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"identifier": IdentifierViewOf(i), "variant": VariantViewOf(v), "product": ProductViewOf(p)})
}

func (h *Handler) addPackage(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in packageCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.AddPackageRelation(c, a, s, c.Param("productId"), in.ParentVariantID, in.ChildVariantID, in.UnitsPerParent)
		return PackageRelationViewOf(v), err
	})
}

func (h *Handler) listPackages(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	items, err := h.service.ListPackageRelations(c, a, s, c.Param("productId"), includeArchived(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, mapSlice(items, PackageRelationViewOf))
}

func (h *Handler) archivePackage(c *gin.Context) {
	h.command(c, PermCatalogueManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchivePackageRelation(c, a, s, c.Param("productId"), c.Param("relationId"), in.ExpectedVersion)
		return PackageRelationViewOf(v), err
	})
}

func (h *Handler) createCalibration(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in CreateCalibrationInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.CreateCalibration(c, a, s, c.Param("productId"), in)
		return CalibrationViewOf(v), err
	})
}

func (h *Handler) listCalibrations(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	items, err := h.service.ListCalibrations(c, a, s, c.Param("productId"), includeArchived(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, mapSlice(items, CalibrationViewOf))
}

func (h *Handler) updateCalibration(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in UpdateCalibrationInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.UpdateCalibration(c, a, s, c.Param("productId"), c.Param("calibrationId"), in)
		return CalibrationViewOf(v), err
	})
}

func (h *Handler) archiveCalibration(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchiveCalibration(c, a, s, c.Param("productId"), c.Param("calibrationId"), in.ExpectedVersion)
		return CalibrationViewOf(v), err
	})
}

func (h *Handler) createSellUnit(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in CreateSellUnitInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.CreateSellUnit(c, a, s, c.Param("productId"), in)
		return SellUnitViewOf(v), err
	})
}

func (h *Handler) listSellUnits(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	items, err := h.service.ListSellUnits(c, a, s, c.Param("productId"), c.Query("variant_id"), includeArchived(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, mapSlice(items, SellUnitViewOf))
}

func (h *Handler) updateSellUnit(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in UpdateSellUnitInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if explicitNull(raw, "calibration_id") {
			in.CalibrationID = new(*string)
		}
		if explicitNull(raw, "estimated_base_qty") {
			in.EstimatedBaseQty = new(*int64)
		}
		if explicitNull(raw, "price_minor") {
			in.PriceMinor = new(*int64)
		}
		if explicitNull(raw, "wholesale_price_minor") {
			in.WholesalePriceMinor = new(*int64)
		}
		if explicitNull(raw, "maximum_order_qty") {
			in.MaximumOrderQty = new(*int64)
		}
		v, err := h.service.UpdateSellUnit(c, a, s, c.Param("productId"), c.Param("sellUnitId"), in)
		return SellUnitViewOf(v), err
	})
}

func (h *Handler) archiveSellUnit(c *gin.Context) {
	h.command(c, PermMeasurementsMngt, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchiveSellUnit(c, a, s, c.Param("productId"), c.Param("sellUnitId"), in.ExpectedVersion)
		return SellUnitViewOf(v), err
	})
}

func (h *Handler) configureFresh(c *gin.Context) {
	h.command(c, PermFreshManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in ConfigureFreshAttributeInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ConfigureFreshAttribute(c, a, s, c.Param("productId"), in)
		return FreshAttributeViewOf(v), err
	})
}

func (h *Handler) listFresh(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	items, err := h.service.ListFreshAttributes(c, a, s, c.Param("productId"), includeArchived(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, mapSlice(items, FreshAttributeViewOf))
}

func (h *Handler) archiveFresh(c *gin.Context) {
	h.command(c, PermFreshManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchiveFreshAttribute(c, a, s, c.Param("productId"), c.Param("attributeId"), in.ExpectedVersion)
		return FreshAttributeViewOf(v), err
	})
}

func (h *Handler) putOperatingModels(c *gin.Context) {
	h.command(c, PermListingsManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in SetOperatingModelsInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.SetOperatingModels(c, a, s, c.Param("branchId"), in)
		return OperatingModelViewOf(v), err
	})
}

func (h *Handler) getOperatingModels(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	v, capabilities, err := h.service.GetOperatingModels(c, a, s, c.Param("branchId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"configuration": OperatingModelViewOf(v), "eligible_capabilities": capabilities})
}

func (h *Handler) archiveOperatingModels(c *gin.Context) {
	h.command(c, PermListingsManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchiveOperatingModels(c, a, s, c.Param("branchId"), in.ExpectedVersion)
		return OperatingModelViewOf(v), err
	})
}

func (h *Handler) createListing(c *gin.Context) {
	h.command(c, PermListingsManage, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in CreateListingInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.CreateListing(c, a, s, c.Param("branchId"), in)
		return ListingViewOf(v), err
	})
}

func (h *Handler) listListings(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListListings(c, a, s, c.Param("branchId"), ListingListOptions{ProductID: c.Query("product_id"), VariantID: c.Query("variant_id"), Status: c.Query("status"), IncludeArchived: includeArchived(c), Limit: limit, Offset: offset})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, mapSlice(items, ListingViewOf), more)
}

func (h *Handler) getListing(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	v, units, err := h.service.GetListing(c, a, s, c.Param("listingId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"listing": ListingViewOf(v), "sell_units": mapSlice(units, ListingSellUnitViewOf)})
}

func (h *Handler) updateListing(c *gin.Context) {
	h.command(c, PermListingsManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in UpdateListingInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if explicitNull(raw, "retail_override") {
			in.RetailOverride = new(*int64)
		}
		if explicitNull(raw, "wholesale_override") {
			in.WholesaleOverride = new(*int64)
		}
		if explicitNull(raw, "maximum_order_qty") {
			in.MaximumOrderQty = new(*int64)
		}
		v, err := h.service.UpdateListing(c, a, s, c.Param("listingId"), in)
		return ListingViewOf(v), err
	})
}

func (h *Handler) archiveListing(c *gin.Context) {
	h.command(c, PermListingsManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchiveListing(c, a, s, c.Param("listingId"), in.ExpectedVersion)
		return ListingViewOf(v), err
	})
}

func (h *Handler) setListingUnitPrice(c *gin.Context) {
	h.command(c, PermListingsManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in SetListingSellUnitPriceInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if explicitNull(raw, "price_override") {
			in.PriceOverride = new(*int64)
		}
		if explicitNull(raw, "wholesale_override") {
			in.WholesaleOverride = new(*int64)
		}
		if explicitNull(raw, "calibration_id") {
			in.CalibrationID = new(*string)
		}
		v, err := h.service.SetListingSellUnitPrice(c, a, s, c.Param("listingId"), c.Param("listingUnitId"), in)
		return ListingSellUnitViewOf(v), err
	})
}

func (h *Handler) createQuantityBreak(c *gin.Context) {
	h.command(c, PermWholesaleManage, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in CreateQuantityBreakInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.CreateQuantityBreak(c, a, s, c.Param("listingId"), in)
		return QuantityBreakViewOf(v), err
	})
}

func (h *Handler) listQuantityBreaks(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	items, err := h.service.ListQuantityBreaks(c, a, s, c.Param("listingId"), includeArchived(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, mapSlice(items, QuantityBreakViewOf))
}

func (h *Handler) updateQuantityBreak(c *gin.Context) {
	h.command(c, PermWholesaleManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in UpdateQuantityBreakInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if explicitNull(raw, "max_quantity") {
			in.MaxQuantity = new(*int64)
		}
		if explicitNull(raw, "lead_time_days") {
			in.LeadTimeDays = new(*int)
		}
		v, err := h.service.UpdateQuantityBreak(c, a, s, c.Param("listingId"), c.Param("breakId"), in)
		return QuantityBreakViewOf(v), err
	})
}

func (h *Handler) archiveQuantityBreak(c *gin.Context) {
	h.command(c, PermWholesaleManage, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in versionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		v, err := h.service.ArchiveQuantityBreak(c, a, s, c.Param("listingId"), c.Param("breakId"), in.ExpectedVersion)
		return QuantityBreakViewOf(v), err
	})
}

func (h *Handler) resolvePrice(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	quantity, err := queryInt(c, "quantity", 1, 1, 1000000000)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	wholesale := strings.EqualFold(c.Query("channel"), "wholesale")
	v, err := h.service.ResolvePrice(c, a, s, c.Param("listingId"), c.Query("sell_unit_id"), int64(quantity), wholesale)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, ResolvedPriceViewOf(v))
}

func (h *Handler) priceHistory(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListPriceHistory(c, a, s, PriceHistoryOptions{SubjectType: c.Query("subject_type"), SubjectRef: c.Query("subject_ref"), VariantRef: c.Query("variant_ref"), ProductRef: c.Query("product_ref"), Limit: limit, Offset: offset})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, mapSlice(items, PriceVersionViewOf), more)
}

func (h *Handler) catalogueRevision(c *gin.Context) {
	a, s, ok := h.authorized(c, PermRead)
	if !ok {
		return
	}
	revision, updatedAt, err := h.service.CatalogueRevision(c, a, s, c.Query("branch_id"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"revision": revision, "updated_at": updatedAt, "domain": "SHOP"})
}

// RegisterRoutes mounts only management APIs. No route in this function
// exposes stock, availability, reservations, customer discovery or orders.
func RegisterRoutes(group *gin.RouterGroup, h *Handler, flags Flags) {
	if !flags.ShopEnabled {
		return
	}
	root := group.Group("/shop")

	root.GET("/catalogue-revision", h.catalogueRevision)

	categories := root.Group("/categories")
	categories.POST("", h.createCategory)
	categories.GET("", h.listCategories)
	categories.GET("/:categoryId", h.getCategory)
	categories.PATCH("/:categoryId", h.updateCategory)
	categories.POST("/:categoryId/archive", h.categoryLifecycle(false))
	categories.POST("/:categoryId/restore", h.categoryLifecycle(true))

	measurements := root.Group("/measurements")
	measurements.POST("", h.createMeasure)
	measurements.GET("", h.listMeasures)
	measurements.GET("/:measureId", h.getMeasure)
	measurements.PATCH("/:measureId", h.updateMeasure)
	measurements.POST("/:measureId/archive", h.archiveMeasure)
	measurements.GET("/:measureId/aliases", h.listAliases)
	measurements.POST("/:measureId/aliases", h.addAlias)
	measurements.DELETE("/:measureId/aliases/:aliasId", h.removeAlias)

	products := root.Group("/products")
	products.POST("", h.createProduct)
	products.GET("", h.listProducts)
	products.GET("/:productId", h.getProduct)
	products.PATCH("/:productId", h.updateProduct)
	products.POST("/:productId/archive", h.archiveProduct)
	products.POST("/:productId/variants", h.createVariant)
	products.GET("/:productId/variants", h.listVariants)
	products.PATCH("/:productId/variants/:variantId", h.updateVariant)
	products.POST("/:productId/variants/:variantId/archive", h.archiveVariant)
	products.POST("/:productId/measure-calibrations", h.createCalibration)
	products.GET("/:productId/measure-calibrations", h.listCalibrations)
	products.PATCH("/:productId/measure-calibrations/:calibrationId", h.updateCalibration)
	products.POST("/:productId/measure-calibrations/:calibrationId/archive", h.archiveCalibration)
	products.POST("/:productId/sell-units", h.createSellUnit)
	products.GET("/:productId/sell-units", h.listSellUnits)
	products.PATCH("/:productId/sell-units/:sellUnitId", h.updateSellUnit)
	products.POST("/:productId/sell-units/:sellUnitId/archive", h.archiveSellUnit)
	products.POST("/:productId/packaging", h.addPackage)
	products.GET("/:productId/packaging", h.listPackages)
	products.POST("/:productId/packaging/:relationId/archive", h.archivePackage)
	products.PUT("/:productId/fresh-attributes", h.configureFresh)
	products.GET("/:productId/fresh-attributes", h.listFresh)
	products.POST("/:productId/fresh-attributes/:attributeId/archive", h.archiveFresh)

	variants := root.Group("/variants")
	variants.GET("/:variantId", h.getVariant)
	variants.POST("/:variantId/identifiers", h.addIdentifier)
	variants.GET("/:variantId/identifiers", h.listIdentifiers)
	variants.POST("/:variantId/identifiers/:identifierId/archive", h.archiveIdentifier)
	root.GET("/identifier-lookup", h.lookupIdentifier)

	branches := root.Group("/branches")
	branches.GET("/:branchId/operating-models", h.getOperatingModels)
	branches.PUT("/:branchId/operating-models", h.putOperatingModels)
	branches.POST("/:branchId/operating-models/archive", h.archiveOperatingModels)
	branches.POST("/:branchId/listings", h.createListing)
	branches.GET("/:branchId/listings", h.listListings)

	listings := root.Group("/listings")
	listings.GET("/:listingId", h.getListing)
	listings.PATCH("/:listingId", h.updateListing)
	listings.POST("/:listingId/archive", h.archiveListing)
	listings.PUT("/:listingId/sell-units/:listingUnitId", h.setListingUnitPrice)
	listings.POST("/:listingId/quantity-breaks", h.createQuantityBreak)
	listings.GET("/:listingId/quantity-breaks", h.listQuantityBreaks)
	listings.PATCH("/:listingId/quantity-breaks/:breakId", h.updateQuantityBreak)
	listings.POST("/:listingId/quantity-breaks/:breakId/archive", h.archiveQuantityBreak)
	listings.GET("/:listingId/resolved-price", h.resolvePrice)
	root.GET("/price-history", h.priceHistory)
	RegisterInventoryRoutes(root, h)
}
