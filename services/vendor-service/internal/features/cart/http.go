package cart

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxCustomerCartCommandBody = 64 << 10

type createCartRequest struct {
	Domain string `json:"domain"`
}

type addCartItemRequest struct {
	ItemRef     string `json:"item_ref"`
	VariantRef  string `json:"variant_ref,omitempty"`
	MenuRef     string `json:"menu_ref,omitempty"`
	ListingRef  string `json:"listing_ref,omitempty"`
	SellUnitRef string `json:"sell_unit_ref,omitempty"`
	SizeRef     string `json:"size_ref,omitempty"`

	Quantity int64 `json:"quantity"`

	RequestedMeasureRef     string `json:"requested_measure_ref,omitempty"`
	ExpectedMeasureQuantity int64  `json:"expected_measure_quantity,omitempty"`
	PricingBasis            string `json:"pricing_basis,omitempty"`

	SubstitutionPolicy string `json:"substitution_policy,omitempty"`

	Options []RequestedOption `json:"options,omitempty"`
}

type updateCartQuantityRequest struct {
	Quantity int64 `json:"quantity"`
}

type setCartAddressRequest struct {
	AddressRef string `json:"address_ref"`
}

type setCartReceiverRequest struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

func strictCartJSON(
	c *gin.Context,
	target any,
) error {
	body, err := io.ReadAll(
		io.LimitReader(
			c.Request.Body,
			maxCustomerCartCommandBody+1,
		),
	)
	if err != nil {
		return apperrors.BadRequest(
			"Request body is invalid.",
			err,
		)
	}

	if len(body) > maxCustomerCartCommandBody {
		return apperrors.BadRequest(
			"Request body is too large.",
			nil,
		)
	}

	decoder := json.NewDecoder(
		bytes.NewReader(body),
	)
	decoder.DisallowUnknownFields()

	if err = decoder.Decode(target); err != nil {
		return apperrors.BadRequest(
			"Request body is invalid.",
			err,
		)
	}

	var trailing any

	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return apperrors.BadRequest(
			"Request body must contain exactly one JSON object.",
			err,
		)
	}

	return nil
}

func customerCartSubject(
	c *gin.Context,
) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)

	if !ok ||
		claims.Type != sharedauth.TokenTypeAccess ||
		claims.Role != "customer" ||
		claims.Service != "customer" ||
		strings.TrimSpace(claims.Subject) == "" {
		httpx.RespondError(
			c,
			apperrors.Unauthorized(
				"Authentication is required.",
				nil,
			),
		)
		return "", false
	}

	return strings.TrimSpace(claims.Subject), true
}

func customerCartExpectedVersion(
	c *gin.Context,
) (int64, error) {
	raw := strings.TrimSpace(
		c.GetHeader("If-Match"),
	)

	if raw == "" {
		return 0, apperrors.Validation(
			"Check your details.",
			[]apperrors.FieldViolation{
				{
					Field:   "If-Match",
					Message: "The current cart version is required.",
				},
			},
		)
	}

	if strings.HasPrefix(
		strings.ToUpper(raw),
		"W/",
	) {
		return 0, apperrors.Validation(
			"Check your details.",
			[]apperrors.FieldViolation{
				{
					Field:   "If-Match",
					Message: "Weak entity tags are not accepted for cart mutations.",
				},
			},
		)
	}

	if strings.HasPrefix(raw, `"`) &&
		strings.HasSuffix(raw, `"`) &&
		len(raw) >= 2 {
		raw = strings.TrimSuffix(
			strings.TrimPrefix(raw, `"`),
			`"`,
		)
	}

	version, err := strconv.ParseInt(
		strings.TrimSpace(raw),
		10,
		64,
	)

	if err != nil || version < 1 {
		return 0, apperrors.Validation(
			"Check your details.",
			[]apperrors.FieldViolation{
				{
					Field:   "If-Match",
					Message: "The cart version must be a positive integer.",
				},
			},
		)
	}

	return version, nil
}

func cartHTTPError(
	err error,
) error {
	switch {
	case errors.Is(err, ErrCartNotFound):
		return apperrors.NotFound(
			"Cart not found.",
			err,
		)

	case errors.Is(err, ErrVersionConflict):
		return apperrors.Conflict(
			"The cart changed. Reload it and retry with the current version.",
			err,
		)

	case errors.Is(err, ErrCartNotMutable):
		return apperrors.Conflict(
			"This cart can no longer be changed.",
			err,
		)

	case errors.Is(err, ErrDomainConflict):
		return apperrors.Conflict(
			"Food and Shop items cannot be mixed in the same cart.",
			err,
		)

	case errors.Is(err, ErrFoodBranchConflict):
		return apperrors.Conflict(
			"This Food cart already belongs to another vendor branch. Use a separate Food cart.",
			err,
		)

	case errors.Is(err, ErrInvalidDomain):
		return apperrors.BadRequest(
			"Cart domain must be FOOD or SHOP.",
			err,
		)

	case errors.Is(err, ErrDomainDisabled):
		return apperrors.Unavailable(
			"This commerce service is not available right now.",
			err,
		)

	case errors.Is(err, ErrInvalidQuantity):
		return apperrors.BadRequest(
			"Cart quantity is invalid.",
			err,
		)

	case errors.Is(err, ErrInvalidReceiver):
		return apperrors.BadRequest(
			"Receiver details are invalid.",
			err,
		)

	case errors.Is(err, ErrInvalidAddress):
		return apperrors.BadRequest(
			"The selected delivery address is invalid.",
			err,
		)

	case errors.Is(err, ErrInvalidSubstitutionPolicy):
		return apperrors.BadRequest(
			"Choose EXACT_ONLY, AUTO_REFUND, CONTACT_FIRST or VENDOR_SUGGEST_AND_APPROVE; Food items cannot be substituted.",
			err,
		)

	case errors.Is(err, ErrInvalidCatalogueItem):
		return apperrors.BadRequest(
			"The selected catalogue item or option is invalid.",
			err,
		)

	case errors.Is(err, ErrDependencyMissing):
		return apperrors.Internal(
			"A required commerce authority is unavailable.",
			err,
		)

	default:
		// Food/Shop authoritative services already return platform application
		// errors. Preserve those rather than flattening their domain code.
		return err
	}
}

func respondCartError(
	c *gin.Context,
	err error,
) {
	httpx.RespondError(
		c,
		cartHTTPError(err),
	)
}

func respondCart(
	c *gin.Context,
	status int,
	value Cart,
) {
	c.Header(
		"ETag",
		`"`+strconv.FormatInt(value.Version, 10)+`"`,
	)

	c.JSON(
		status,
		gin.H{
			"success": true,
			"data":    value,
		},
	)
}

type CustomerHandler struct {
	service *Service
}

func NewCustomerHandler(
	service *Service,
) *CustomerHandler {
	return &CustomerHandler{
		service: service,
	}
}

func (h *CustomerHandler) createOrGetActive(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	var request createCartRequest

	if err := strictCartJSON(
		c,
		&request,
	); err != nil {
		httpx.RespondError(c, err)
		return
	}

	domain, err := ParseDomain(
		request.Domain,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	current, err := h.service.CreateOrGetActiveCart(
		c,
		customerID,
		domain,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		current,
	)
}

func (h *CustomerHandler) get(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	current, err := h.service.GetCart(
		c,
		customerID,
		c.Param("cartId"),
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		current,
	)
}

func (h *CustomerHandler) groupedShop(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	current, err := h.service.GetCart(
		c,
		customerID,
		c.Param("cartId"),
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	groups, err := GroupShopCart(
		current,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	c.Header(
		"ETag",
		`"`+strconv.FormatInt(current.Version, 10)+`"`,
	)

	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"data": gin.H{
				"cart":   current,
				"groups": groups,
			},
		},
	)
}

func (h *CustomerHandler) addItem(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	version, err := customerCartExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	var request addCartItemRequest

	if err = strictCartJSON(
		c,
		&request,
	); err != nil {
		httpx.RespondError(c, err)
		return
	}

	updated, err := h.service.AddItem(
		c,
		customerID,
		c.Param("cartId"),
		version,
		AddItemInput{
			ItemRef:                 request.ItemRef,
			VariantRef:              request.VariantRef,
			MenuRef:                 request.MenuRef,
			ListingRef:              request.ListingRef,
			SellUnitRef:             request.SellUnitRef,
			SizeRef:                 request.SizeRef,
			Quantity:                request.Quantity,
			RequestedMeasureRef:     request.RequestedMeasureRef,
			ExpectedMeasureQuantity: request.ExpectedMeasureQuantity,
			PricingBasis:            request.PricingBasis,
			SubstitutionPolicy:      request.SubstitutionPolicy,
			Options:                 request.Options,
		},
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		updated,
	)
}

func (h *CustomerHandler) updateQuantity(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	version, err := customerCartExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	var request updateCartQuantityRequest

	if err = strictCartJSON(
		c,
		&request,
	); err != nil {
		httpx.RespondError(c, err)
		return
	}

	updated, err := h.service.UpdateQuantity(
		c,
		customerID,
		c.Param("cartId"),
		c.Param("itemId"),
		request.Quantity,
		version,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		updated,
	)
}

func (h *CustomerHandler) removeItem(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	version, err := customerCartExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	updated, err := h.service.RemoveItem(
		c,
		customerID,
		c.Param("cartId"),
		c.Param("itemId"),
		version,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		updated,
	)
}

func (h *CustomerHandler) clear(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	version, err := customerCartExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	updated, err := h.service.ClearCart(
		c,
		customerID,
		c.Param("cartId"),
		version,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		updated,
	)
}

func (h *CustomerHandler) setAddress(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	version, err := customerCartExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	var request setCartAddressRequest

	if err = strictCartJSON(
		c,
		&request,
	); err != nil {
		httpx.RespondError(c, err)
		return
	}

	updated, err := h.service.SetDeliveryAddress(
		c,
		customerID, c.Param("cartId"), request.AddressRef, version,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		updated,
	)
}

func (h *CustomerHandler) removeAddress(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	version, err := customerCartExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	updated, err := h.service.RemoveDeliveryAddress(
		c,
		customerID,
		c.Param("cartId"),
		version,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		updated,
	)
}

func (h *CustomerHandler) setReceiver(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	version, err := customerCartExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	var request setCartReceiverRequest

	if err = strictCartJSON(
		c,
		&request,
	); err != nil {
		httpx.RespondError(c, err)
		return
	}

	updated, err := h.service.SetReceiver(
		c,
		customerID, c.Param("cartId"), Receiver{Name: request.Name, Phone: request.Phone}, version,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		updated,
	)
}

func (h *CustomerHandler) removeReceiver(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	version, err := customerCartExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	updated, err := h.service.RemoveReceiver(
		c,
		customerID,
		c.Param("cartId"),
		version,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		updated,
	)
}

func (h *CustomerHandler) abandon(
	c *gin.Context,
) {
	customerID, ok := customerCartSubject(c)
	if !ok {
		return
	}

	version, err := customerCartExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	updated, err := h.service.AbandonCart(
		c,
		customerID,
		c.Param("cartId"),
		version,
	)
	if err != nil {
		respondCartError(c, err)
		return
	}

	respondCart(
		c,
		http.StatusOK,
		updated,
	)
}

// RegisterCustomerRoutes mounts only customer-facing mutable cart state.
//
// Customer identity comes exclusively from the verified Customer Service bearer
// token. There is intentionally no customerId path, query parameter or request
// field anywhere in this contract.
func RegisterCustomerRoutes(
	root *gin.RouterGroup,
	service *Service,
	customerSigner *sharedauth.TokenSigner,
	idempotencyStore *idempotency.Store,
) {
	if root == nil ||
		service == nil ||
		customerSigner == nil {
		return
	}

	handler := NewCustomerHandler(
		service,
	)

	carts := root.Group(
		"/carts",
	)

	carts.Use(
		sharedauth.BearerMiddleware(
			customerSigner,
			"customer",
			"customer",
		),
	)

	carts.Use(
		idempotency.CustomerCommandMiddleware(
			idempotencyStore,
		),
	)

	carts.POST(
		"",
		handler.createOrGetActive,
	)

	carts.GET(
		"/:cartId",
		handler.get,
	)

	carts.GET(
		"/:cartId/groups",
		handler.groupedShop,
	)

	carts.POST(
		"/:cartId/items",
		handler.addItem,
	)

	carts.PATCH(
		"/:cartId/items/:itemId",
		handler.updateQuantity,
	)

	carts.DELETE(
		"/:cartId/items/:itemId",
		handler.removeItem,
	)

	carts.DELETE(
		"/:cartId/items",
		handler.clear,
	)

	carts.PUT(
		"/:cartId/delivery-address",
		handler.setAddress,
	)

	carts.DELETE(
		"/:cartId/delivery-address",
		handler.removeAddress,
	)

	carts.PUT(
		"/:cartId/receiver",
		handler.setReceiver,
	)

	carts.DELETE(
		"/:cartId/receiver",
		handler.removeReceiver,
	)

	carts.POST(
		"/:cartId/abandon",
		handler.abandon,
	)
}
