package truckcapability

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/hauling-service/internal/truckcatalog"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Handler exposes the capability surfaces.
type Handler struct{ service *Service }

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterPublic mounts the customer-facing product catalogue.
//
// It is unauthenticated because it is exactly the information a customer needs
// before they have an account: which kinds of truck this platform runs. It
// carries no prices -- pricing has its own contract and its own surface -- and
// no supply information, because "how many reefers are near you" is not public.
func RegisterPublic(group *gin.RouterGroup, handler *Handler) {
	if group == nil || handler == nil || handler.service == nil {
		return
	}
	group.GET("/truck-products", handler.listTruckProducts)
}

// RegisterProvider mounts the provider-facing capability surfaces on an
// already-authenticated provider group.
func RegisterProvider(group *gin.RouterGroup, handler *Handler) {
	if group == nil || handler == nil || handler.service == nil {
		return
	}
	group.GET("/trucks/:id/capabilities", handler.getCapability)
	group.PUT("/trucks/:id/capabilities", handler.updateCapability)
	group.GET("/trucks/:id/eligibility", handler.listEligibility)
}

// RegisterInternal mounts the HMAC-protected internal surfaces on a group the
// caller has ALREADY protected.
//
// Like the shared capability contract, this installs no auth of its own: each
// service's internal boundary differs, and a package quietly adding a weaker
// one is how a surface ends up less protected than its neighbours.
func RegisterInternal(group *gin.RouterGroup, handler *Handler) {
	if group == nil || handler == nil || handler.service == nil {
		return
	}
	group.GET("/trucks/:id/capabilities", handler.getCapabilityInternal)
	group.POST("/verification/certification-events", handler.certificationEvent)
}

// ── Product catalogue ────────────────────────────────────────────────────────

type truckProductResponse struct {
	Code         string   `json:"code"`
	Label        string   `json:"label"`
	BodyFamily   string   `json:"body_family"`
	FamilyName   string   `json:"family_label"`
	Class        string   `json:"vehicle_class"`
	Enclosed     *bool    `json:"enclosed,omitempty"`
	Weatherproof *bool    `json:"weather_protected,omitempty"`
	Refrigerated *bool    `json:"refrigerated,omitempty"`
	Handling     []string `json:"handling,omitempty"`
	// CertificationRequired tells a customer-facing client that choosing this
	// product narrows the fleet to formally certified trucks, which is worth
	// surfacing before a booking rather than as a match failure afterwards.
	CertificationRequired bool `json:"certification_required"`
}

func (h *Handler) listTruckProducts(c *gin.Context) {
	entries := truckcatalog.CustomerSelectable()
	products := make([]truckProductResponse, 0, len(entries))
	for _, entry := range entries {
		facts := truckcatalog.FactsFor(string(entry.Code))
		product := ProductGeneralHaulage
		if entry.Refrigerated.Known() && entry.Refrigerated.String() == "YES" {
			product = ProductColdChain
		}
		products = append(products, truckProductResponse{
			Code:                  string(entry.Code),
			Label:                 entry.Label,
			BodyFamily:            string(entry.Family),
			FamilyName:            entry.Family.Label(),
			Class:                 string(entry.VehicleClass),
			Enclosed:              facts.Enclosed.Pointer(),
			Weatherproof:          facts.WeatherProtected.Pointer(),
			Refrigerated:          facts.Refrigerated.Pointer(),
			CertificationRequired: h.service.RequiresCertification(product),
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"products": products,
		"families": familyListing(),
	}})
}

func familyListing() []gin.H {
	families := truckcatalog.Families()
	out := make([]gin.H, 0, len(families))
	for _, family := range families {
		out = append(out, gin.H{
			"code":  string(family),
			"label": family.Label(),
			"types": truckcatalog.Codes(truckcatalog.InFamily(family)),
		})
	}
	return out
}

// ── Provider capability ──────────────────────────────────────────────────────

func providerID(c *gin.Context) string {
	value, _ := c.Get("user_id")
	id, _ := value.(string)
	return id
}

func (h *Handler) getCapability(c *gin.Context) {
	record, err := h.service.GetCapability(c.Request.Context(), providerID(c), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": record})
}

// capabilityRequest is the wire shape of a capability edit.
//
// Every field is a double pointer so the handler can tell three states apart:
// absent (leave alone), explicit null (I no longer know), and a value. A plain
// pointer collapses the first two, which would make an edit of one field blank
// every field the client did not send -- destroying a survey as a side effect
// of changing a tail lift.
type capabilityRequest struct {
	ExpectedVersion int64 `json:"expected_version"`

	MaxPayloadGrams  **int64 `json:"max_payload_grams"`
	InternalLengthMm **int64 `json:"internal_length_mm"`
	InternalWidthMm  **int64 `json:"internal_width_mm"`
	InternalHeightMm **int64 `json:"internal_height_mm"`
	UsableVolumeMl   **int64 `json:"usable_volume_ml"`
	OpeningWidthMm   **int64 `json:"opening_width_mm"`
	OpeningHeightMm  **int64 `json:"opening_height_mm"`

	Enclosed         **bool `json:"enclosed"`
	WeatherProtected **bool `json:"weather_protected"`
	UprightSafe      **bool `json:"upright_safe"`
	FragileSafe      **bool `json:"fragile_safe"`

	Refrigerated     **bool  `json:"refrigerated"`
	TemperatureMinC  **int   `json:"temperature_min_c"`
	TemperatureMaxC  **int   `json:"temperature_max_c"`
	TemperatureClass *string `json:"temperature_class"`

	TailLift    **bool `json:"tail_lift"`
	Crane       **bool `json:"crane"`
	Hydraulic   **bool `json:"hydraulic"`
	LoadingRamp **bool `json:"loading_ramp"`

	AxleCount               **int     `json:"axle_count"`
	TrailerCapable          **bool    `json:"trailer_capable"`
	SupportedContainerSizes *[]string `json:"supported_container_sizes"`

	SupportedGoodsClasses  *[]string `json:"supported_goods_classes"`
	ProhibitedGoodsClasses *[]string `json:"prohibited_goods_classes"`

	InspectionRef *string `json:"inspection_ref"`
}

func nullable[T any](value **T) Nullable[T] {
	if value == nil {
		return Absent[T]()
	}
	if *value == nil {
		return Cleared[T]()
	}
	return Known(**value)
}

func (r capabilityRequest) toInput(truckID, provider string) UpdateInput {
	return UpdateInput{
		TruckID:                 truckID,
		ProviderID:              provider,
		ExpectedVersion:         r.ExpectedVersion,
		MaxPayloadGrams:         nullable(r.MaxPayloadGrams),
		InternalLengthMm:        nullable(r.InternalLengthMm),
		InternalWidthMm:         nullable(r.InternalWidthMm),
		InternalHeightMm:        nullable(r.InternalHeightMm),
		UsableVolumeMl:          nullable(r.UsableVolumeMl),
		OpeningWidthMm:          nullable(r.OpeningWidthMm),
		OpeningHeightMm:         nullable(r.OpeningHeightMm),
		Enclosed:                nullable(r.Enclosed),
		WeatherProtected:        nullable(r.WeatherProtected),
		UprightSafe:             nullable(r.UprightSafe),
		FragileSafe:             nullable(r.FragileSafe),
		Refrigerated:            nullable(r.Refrigerated),
		TemperatureMinC:         nullable(r.TemperatureMinC),
		TemperatureMaxC:         nullable(r.TemperatureMaxC),
		TemperatureClass:        r.TemperatureClass,
		TailLift:                nullable(r.TailLift),
		Crane:                   nullable(r.Crane),
		Hydraulic:               nullable(r.Hydraulic),
		LoadingRamp:             nullable(r.LoadingRamp),
		AxleCount:               nullable(r.AxleCount),
		TrailerCapable:          nullable(r.TrailerCapable),
		SupportedContainerSizes: r.SupportedContainerSizes,
		SupportedGoodsClasses:   r.SupportedGoodsClasses,
		ProhibitedGoodsClasses:  r.ProhibitedGoodsClasses,
		InspectionRef:           r.InspectionRef,
	}
}

func (h *Handler) updateCapability(c *gin.Context) {
	var request capabilityRequest
	if err := decodeExactlyOneObject(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	record, err := h.service.UpdateCapability(c.Request.Context(),
		request.toInput(c.Param("id"), providerID(c)))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": record})
}

func (h *Handler) listEligibility(c *gin.Context) {
	eligibility, err := h.service.ListEligibility(c.Request.Context(), providerID(c), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"eligibility": eligibility}})
}

// ── Internal ─────────────────────────────────────────────────────────────────

// getCapabilityInternal is the authoritative read for another service.
//
// Unlike the provider read it is not scoped to an owner, because the caller is
// a trusted service acting on the platform's behalf rather than on one
// provider's. The HMAC boundary is what makes that safe, and it is installed
// by the caller of RegisterInternal.
func (h *Handler) getCapabilityInternal(c *gin.Context) {
	record, err := h.service.repository.GetByTruck(c.Request.Context(), c.Param("id"))
	if errors.Is(err, ErrNotFound) {
		httpx.Abort(c, apperrors.NotFound("Truck not found.", err))
		return
	}
	if err != nil {
		httpx.Abort(c, apperrors.Internal("Truck capability could not be read.", err))
		return
	}
	eligibility, err := h.service.repository.ListEligibility(c.Request.Context(), record.TruckID)
	if err != nil {
		httpx.Abort(c, apperrors.Internal("Truck eligibility could not be read.", err))
		return
	}
	projected := make([]PublicProductEligibility, 0, len(eligibility))
	for _, item := range eligibility {
		projected = append(projected, item.Public())
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"capability":  record.Public(),
		"eligibility": projected,
		"provider_id": record.ProviderID,
	}})
}

type certificationEventRequest struct {
	TruckID          string     `json:"truck_id"`
	ProductCode      string     `json:"product_code"`
	Status           string     `json:"status"`
	PolicyVersion    string     `json:"policy_version"`
	CertificationRef string     `json:"certification_ref"`
	EffectiveAt      *time.Time `json:"effective_at"`
	ExpiresAt        *time.Time `json:"expires_at"`
	ReasonCode       string     `json:"reason_code"`
	SourceVersion    int64      `json:"source_version"`
}

// certificationEvent records a decision from the certifying authority.
//
// It answers 200 for a duplicate rather than a conflict, with applied=false,
// because the caller is an at-least-once delivery pipeline: turning a replay
// into an error would make it retry the same message forever. The
// source_version guard in SQL is what makes that safe.
func (h *Handler) certificationEvent(c *gin.Context) {
	var request certificationEventRequest
	if err := decodeExactlyOneObject(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	eligibility, applied, err := h.service.ApplyCertificationEvent(c.Request.Context(), CertificationEvent{
		TruckID:          strings.TrimSpace(request.TruckID),
		ProductCode:      request.ProductCode,
		Status:           request.Status,
		PolicyVersion:    request.PolicyVersion,
		CertificationRef: request.CertificationRef,
		EffectiveAt:      request.EffectiveAt,
		ExpiresAt:        request.ExpiresAt,
		ReasonCode:       request.ReasonCode,
		SourceVersion:    request.SourceVersion,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"eligibility": eligibility,
		"applied":     applied,
	}})
}

// decodeExactlyOneObject refuses trailing JSON, matching the strictness the
// commerce mission handlers already apply. A body with a second object is a
// client bug, and silently ignoring the remainder hides it.
func decodeExactlyOneObject(c *gin.Context, target any) error {
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(target); err != nil {
		return apperrors.BadRequest("Request body must be a single JSON object.", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return apperrors.BadRequest("Request body must contain exactly one JSON object.", err)
	}
	return nil
}
