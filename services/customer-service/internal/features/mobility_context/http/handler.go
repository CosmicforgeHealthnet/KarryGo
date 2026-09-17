package mobilityhttp

import (
	"bytes"
	"encoding/json"
	"io"
	nethttp "net/http"
	"strings"

	"github.com/gin-gonic/gin"

	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	mobilitymodels "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/models"
	mobilityusecases "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxBodyBytes = 16 << 10

type Handler struct{ service *mobilityusecases.Service }

func RegisterRoutes(group *gin.RouterGroup, service *mobilityusecases.Service, signer *sharedauth.TokenSigner) {
	if group == nil || service == nil || signer == nil {
		return
	}
	h := &Handler{service: service}
	auth := sharedauth.BearerMiddleware(signer, authusecases.CustomerRole, authusecases.CustomerService)

	preferences := group.Group("/mobility-preferences", auth)
	preferences.GET("", h.getPreferences)
	preferences.PUT("", h.putPreferences)

	guests := group.Group("/guest-travellers", auth)
	guests.GET("", h.listGuests)
	guests.POST("", h.createGuest)
	guests.GET("/:id", h.getGuest)
	guests.PUT("/:id", h.updateGuest)
	guests.DELETE("/:id", h.deleteGuest)
}

// customerID only ever comes from the verified access token. Request bodies
// have no customer_id field and unknown fields are rejected.
func customerID(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || strings.TrimSpace(claims.Subject) == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return strings.TrimSpace(claims.Subject), true
}

func bindStrict(c *gin.Context, target any) bool {
	raw, err := io.ReadAll(nethttp.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes))
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is too large or unreadable.", err))
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return false
	}
	return true
}

type preferencesRequest struct {
	AccessibilityRequired    bool            `json:"accessibility_required"`
	WheelchairRequired       *bool           `json:"wheelchair_required"`
	ChildSeatPreference      *string         `json:"child_seat_preference"`
	PetTravelPreference      *string         `json:"pet_travel_preference"`
	LuggageDefault           string          `json:"luggage_default"`
	PreferredPassengerCount  *int            `json:"preferred_passenger_count"`
	CommunicationPreferences json.RawMessage `json:"communication_preferences"`
	SafetyShareDefault       bool            `json:"safety_share_default"`
	SpecialAssistanceNotes   *string         `json:"special_assistance_notes"`
	ExpectedVersion          *int64          `json:"expected_version"`
}

func (h *Handler) getPreferences(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	out, err := h.service.GetPreferences(c.Request.Context(), id)
	respond(c, nethttp.StatusOK, out, err)
}

func (h *Handler) putPreferences(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	var req preferencesRequest
	if !bindStrict(c, &req) {
		return
	}
	out, err := h.service.PutPreferences(c.Request.Context(), id, mobilitymodels.MobilityPreferences{
		AccessibilityRequired: req.AccessibilityRequired, WheelchairRequired: req.WheelchairRequired,
		ChildSeatPreference: req.ChildSeatPreference, PetTravelPreference: req.PetTravelPreference,
		LuggageDefault: req.LuggageDefault, PreferredPassengerCount: req.PreferredPassengerCount,
		CommunicationPreferences: req.CommunicationPreferences, SafetyShareDefault: req.SafetyShareDefault,
		SpecialAssistanceNotes: req.SpecialAssistanceNotes,
	}, req.ExpectedVersion)
	respond(c, nethttp.StatusOK, out, err)
}

type guestRequest struct {
	Name                     string          `json:"name"`
	Phone                    *string         `json:"phone"`
	RelationshipLabel        *string         `json:"relationship_label"`
	AccessibilityNotes       *string         `json:"accessibility_notes"`
	MobilityRequirementsJSON json.RawMessage `json:"mobility_requirements_json"`
	ExpectedVersion          *int64          `json:"expected_version,omitempty"`
}

func (r guestRequest) model() mobilitymodels.GuestTraveller {
	return mobilitymodels.GuestTraveller{Name: r.Name, Phone: r.Phone, RelationshipLabel: r.RelationshipLabel,
		AccessibilityNotes: r.AccessibilityNotes, MobilityRequirementsJSON: r.MobilityRequirementsJSON}
}

func (h *Handler) listGuests(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	out, err := h.service.ListGuests(c.Request.Context(), id)
	respond(c, nethttp.StatusOK, gin.H{"travellers": out}, err)
}

func (h *Handler) getGuest(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	out, err := h.service.GetGuest(c.Request.Context(), id, c.Param("id"))
	respond(c, nethttp.StatusOK, out, err)
}

func (h *Handler) createGuest(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	var req guestRequest
	if !bindStrict(c, &req) {
		return
	}
	if req.ExpectedVersion != nil {
		httpx.Abort(c, apperrors.BadRequest("expected_version is not allowed when creating a traveller.", nil))
		return
	}
	out, replayed, err := h.service.CreateGuest(c.Request.Context(), id, req.model(), c.GetHeader("Idempotency-Key"))
	status := nethttp.StatusCreated
	if replayed {
		status = nethttp.StatusOK
	}
	respond(c, status, out, err)
}

func (h *Handler) updateGuest(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	var req guestRequest
	if !bindStrict(c, &req) {
		return
	}
	out, err := h.service.UpdateGuest(c.Request.Context(), id, c.Param("id"), req.model(), req.ExpectedVersion)
	respond(c, nethttp.StatusOK, out, err)
}

func (h *Handler) deleteGuest(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	err := h.service.DeleteGuest(c.Request.Context(), id, c.Param("id"))
	respond(c, nethttp.StatusOK, gin.H{}, err)
}

func respond(c *gin.Context, status int, data any, err error) {
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(status, gin.H{"success": true, "data": data})
}
