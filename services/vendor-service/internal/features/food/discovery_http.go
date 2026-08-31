package food

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// InternalHandler is the read-only service-facing Section 9C contract. It is
// deliberately separate from the authenticated vendor-management Handler:
// internal callers consume effective truth and never receive mutation routes.
type InternalHandler struct{ service *Service }

func NewInternalHandler(service *Service) *InternalHandler { return &InternalHandler{service: service} }

var allowedDiscoveryCallers = map[string]bool{
	"customer-service":  true,
	"api-gateway":       true,
	"order-service":     true,
	"discovery-service": true,
}

func (h *InternalHandler) allowCaller(c *gin.Context) {
	value, _ := c.Get("service_name")
	caller, _ := value.(string)
	if !allowedDiscoveryCallers[caller] {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Next()
}

func (h *InternalHandler) requireNonce(c *gin.Context) {
	value, _ := c.Get("service_name")
	nonce := strings.TrimSpace(c.GetHeader(serviceauth.HeaderNonce))
	if nonce == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	tag, err := h.service.db.Exec(c, `INSERT INTO vendor_service_nonces(caller,nonce,expires_at)
		VALUES($1,$2,now()+interval '10 minutes') ON CONFLICT DO NOTHING`, value, nonce)
	if err != nil || tag.RowsAffected() != 1 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

func internalOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (h *InternalHandler) branchMatch(c *gin.Context, itemID string) (string, error) {
	var businessID, branchID string
	err := h.service.db.QueryRow(c, `SELECT business_id,branch_id FROM food_items WHERE id=$1`, strings.TrimSpace(itemID)).Scan(&businessID, &branchID)
	if err != nil || branchID != strings.TrimSpace(c.Param("branchId")) {
		return "", errNotFound("Food item", CodeItemNotFound)
	}
	return businessID, nil
}

func parseDiscoveryQuery(c *gin.Context) (DiscoveryOptions, error) {
	parseInt64 := func(name string) (*int64, error) {
		raw := strings.TrimSpace(c.Query(name))
		if raw == "" {
			return nil, nil
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: name, Message: "Value must be an integer."}})
		}
		return &value, nil
	}
	parseInt := func(name string) (*int, error) {
		raw := strings.TrimSpace(c.Query(name))
		if raw == "" {
			return nil, nil
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			return nil, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: name, Message: "Value must be an integer."}})
		}
		return &value, nil
	}
	minPrice, err := parseInt64("min_price_minor")
	if err != nil {
		return DiscoveryOptions{}, err
	}
	maxPrice, err := parseInt64("max_price_minor")
	if err != nil {
		return DiscoveryOptions{}, err
	}
	prepMax, err := parseInt("prep_max_minutes")
	if err != nil {
		return DiscoveryOptions{}, err
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil {
		return DiscoveryOptions{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "limit", Message: "Value must be an integer."}})
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil {
		return DiscoveryOptions{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "offset", Message: "Value must be an integer."}})
	}
	at := time.Time{}
	if raw := strings.TrimSpace(c.Query("at")); raw != "" {
		at, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return DiscoveryOptions{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "at", Message: "Time must be RFC3339."}})
		}
	}
	return DiscoveryOptions{BranchID: c.Param("branchId"), Capability: c.Query("capability"), MenuID: c.Query("menu_id"), SectionID: c.Query("section_id"), Text: c.Query("q"), DietaryCode: c.Query("dietary_code"), ExcludeAllergen: c.Query("exclude_allergen"), MinPriceMinor: minPrice, MaxPriceMinor: maxPrice, PrepMaxMinutes: prepMax, SellableOnly: true, Limit: limit, Offset: offset, At: at}, nil
}

func (h *InternalHandler) Catalogue(c *gin.Context) {
	in, err := parseDiscoveryQuery(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	since := int64(0)
	hasSince := false
	if raw := strings.TrimSpace(c.Query("since_revision")); raw != "" {
		hasSince = true
		var parseErr error
		since, parseErr = strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || since < 0 {
			httpx.RespondError(c, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "since_revision", Message: "Revision must be a non-negative integer."}}))
			return
		}
	}
	snapshot, err := h.service.DiscoverCatalogue(c, in)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if hasSince && since == snapshot.CatalogueRevision {
		snapshot.Menus = nil
		snapshot.Unchanged = true
	}
	internalOK(c, snapshot)
}

func (h *InternalHandler) Search(c *gin.Context) {
	in, err := parseDiscoveryQuery(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	snapshot, err := h.service.DiscoverCatalogue(c, in)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	internalOK(c, snapshot)
}

func (h *InternalHandler) Revision(c *gin.Context) {
	capability := c.Query("capability")
	revision, err := h.service.DiscoveryRevision(c, c.Param("branchId"), capability)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if raw := strings.TrimSpace(c.Query("since_revision")); raw != "" {
		if since, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil && since == revision.CatalogueRevision {
			revision.Unchanged = true
		}
	}
	internalOK(c, revision)
}

func (h *InternalHandler) Sellability(c *gin.Context) {
	businessID, err := h.branchMatch(c, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	at := time.Now().UTC()
	if raw := strings.TrimSpace(c.Query("at")); raw != "" {
		parsedAt, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			httpx.RespondError(c, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "at", Message: "Time must be RFC3339."}}))
			return
		} else {
			at = parsedAt
		}
	}
	result, err := h.service.ResolveSellability(c, Actor{VendorID: businessID}, Scope{Global: true}, c.Param("itemId"), at)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	internalOK(c, result)
}

type internalPriceRequest struct {
	ItemRef              string   `json:"item_ref"`
	VariantRef           string   `json:"variant_ref,omitempty"`
	SelectedModifierRefs []string `json:"selected_modifier_refs,omitempty"`
}

func (h *InternalHandler) ResolvePrice(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	var req internalPriceRequest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	businessID, err := h.branchMatch(c, req.ItemRef)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	result, err := h.service.ResolvePrice(c, Actor{VendorID: businessID}, Scope{Global: true}, req.ItemRef, ResolvePriceInput{VariantRef: req.VariantRef, ModifierRefs: req.SelectedModifierRefs})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	internalOK(c, result)
}

func RegisterInternalRoutes(router *gin.Engine, handler *InternalHandler, verifier *serviceauth.Verifier, flags Flags) {
	if handler == nil || verifier == nil || !flags.CatalogueEnabled {
		return
	}
	group := router.Group("/internal/vendor/v1/food")
	group.Use(serviceauth.Middleware(verifier), handler.allowCaller, handler.requireNonce)
	group.GET("/branches/:branchId/catalogue", handler.Catalogue)
	group.GET("/branches/:branchId/search", handler.Search)
	group.GET("/branches/:branchId/revision", handler.Revision)
	group.GET("/branches/:branchId/items/:itemId/sellability", handler.Sellability)
	group.POST("/branches/:branchId/pricing/resolve", handler.ResolvePrice)
}
