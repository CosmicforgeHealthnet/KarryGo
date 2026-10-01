package bikecapability

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Handler exposes the capability surfaces.
type Handler struct{ service *Service }

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterProvider mounts the rider-facing capability surfaces on a group the
// caller has ALREADY authenticated.
func RegisterProvider(group *gin.RouterGroup, handler *Handler) {
	if group == nil || handler == nil || handler.service == nil {
		return
	}
	// The :id parameter name matches the vehicle group's existing routes.
	// Gin refuses two different wildcard names on one path segment, so a
	// ":bike_id" here would panic the router at startup.
	group.GET("/:id/capabilities", handler.getCapability)
	group.PUT("/:id/capabilities", handler.updateCapability)
	group.GET("/:id/eligibility", handler.listEligibility)
}

// RegisterInternal mounts the HMAC-protected surfaces on a group the caller
// has ALREADY protected.
//
// Like the shared capability contract, this installs no auth of its own: each
// service's internal boundary differs, and a package quietly adding a weaker
// one is how a surface ends up less protected than its neighbours.
func RegisterInternal(group *gin.RouterGroup, handler *Handler) {
	if group == nil || handler == nil || handler.service == nil {
		return
	}
	group.GET("/bikes/:id/capabilities", handler.getCapabilityInternal)
}

// RegisterCertificationIngress mounts the certifying authority's callback on a
// group the caller has ALREADY protected.
//
// It is separate from RegisterInternal because the two surfaces have different
// callers and must not share a boundary. The capability read is Vendor's, and
// its group trusts Vendor's secret; the certification callback is
// verification-compliance-service's alone. Mounting them together meant this
// service could only ever be certified by a caller holding Vendor's key, and
// meant the certification surface disappeared entirely in a deployment that had
// not configured Vendor -- two unrelated integrations sharing one switch.
func RegisterCertificationIngress(group *gin.RouterGroup, handler *Handler) {
	if group == nil || handler == nil || handler.service == nil {
		return
	}
	group.POST("/verification/certification-events", handler.certificationEvent)
}

func providerID(c *gin.Context) string {
	for _, key := range []string{"provider_id", "user_id"} {
		if value, ok := c.Get(key); ok {
			if id, _ := value.(string); strings.TrimSpace(id) != "" {
				return id
			}
		}
	}
	return ""
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
// Every optional field is a double pointer so the handler can tell three
// states apart: absent (leave alone), explicit null (I no longer know), and a
// value. A plain pointer collapses the first two, which would make an edit of
// one field blank every field the client did not send -- destroying a survey
// as a side effect of changing a thermal bag.
type capabilityRequest struct {
	ExpectedVersion int64 `json:"expected_version"`

	MaxPayloadGrams **int64 `json:"max_payload_grams"`
	BoxLengthMm     **int64 `json:"box_length_mm"`
	BoxWidthMm      **int64 `json:"box_width_mm"`
	BoxHeightMm     **int64 `json:"box_height_mm"`

	EnclosedBox      **bool `json:"enclosed_box"`
	WeatherProtected **bool `json:"weather_protected"`
	UprightSafe      **bool `json:"upright_safe"`
	FragileSafe      **bool `json:"fragile_safe"`

	ThermalBag       **bool  `json:"thermal_bag"`
	TemperatureClass *string `json:"temperature_class"`
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

func (r capabilityRequest) toInput(bikeID, provider string) UpdateInput {
	return UpdateInput{
		BikeID:           bikeID,
		ProviderID:       provider,
		ExpectedVersion:  r.ExpectedVersion,
		MaxPayloadGrams:  nullable(r.MaxPayloadGrams),
		BoxLengthMm:      nullable(r.BoxLengthMm),
		BoxWidthMm:       nullable(r.BoxWidthMm),
		BoxHeightMm:      nullable(r.BoxHeightMm),
		EnclosedBox:      nullable(r.EnclosedBox),
		WeatherProtected: nullable(r.WeatherProtected),
		UprightSafe:      nullable(r.UprightSafe),
		FragileSafe:      nullable(r.FragileSafe),
		ThermalBag:       nullable(r.ThermalBag),
		TemperatureClass: r.TemperatureClass,
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

// getCapabilityInternal is the authoritative read for another service.
//
// It is not scoped to an owner, because the caller is a trusted service acting
// on the platform's behalf rather than on one rider's. The HMAC boundary is
// what makes that safe, and it is installed by the caller of RegisterInternal.
func (h *Handler) getCapabilityInternal(c *gin.Context) {
	record, err := h.service.repository.GetByBike(c.Request.Context(), c.Param("id"))
	if errors.Is(err, ErrNotFound) {
		httpx.Abort(c, apperrors.NotFound("Bike not found.", err))
		return
	}
	if err != nil {
		httpx.Abort(c, apperrors.Internal("Bike capability could not be read.", err))
		return
	}
	eligibility, err := h.service.repository.ListEligibility(c.Request.Context(), record.BikeID)
	if err != nil {
		httpx.Abort(c, apperrors.Internal("Bike eligibility could not be read.", err))
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
		// surveyed distinguishes "this bike has no recorded capability" from
		// "every fact happens to be null", which look identical in the
		// projection and mean quite different things to a caller.
		"surveyed": record.Surveyed,
	}})
}

type certificationEventRequest struct {
	BikeID           string     `json:"bike_id"`
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
// A duplicate answers 200 with applied=false rather than a conflict: the
// caller is an at-least-once delivery pipeline, and turning a replay into an
// error would make it retry the same message forever. The source_version guard
// in SQL is what makes that safe.
func (h *Handler) certificationEvent(c *gin.Context) {
	var request certificationEventRequest
	if err := decodeExactlyOneObject(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	eligibility, applied, err := h.service.ApplyCertificationEvent(c.Request.Context(), CertificationEvent{
		BikeID:           strings.TrimSpace(request.BikeID),
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
