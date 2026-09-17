// Package contextusecases assembles the minimized customer context that a
// trusted platform service may read. Each caller is allowlisted for explicit
// scopes. Customer Service never returns data outside a caller's scopes, so
// Taxi does not receive trusted contacts and Vendor does not receive profile
// data.
package contextusecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	mobilitymodels "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/models"
	savedlocationmodels "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	ScopeMobilityPreferences = "mobility_preferences"
	ScopeAssistanceNotes     = "assistance_notes"
	ScopeSavedLocation       = "saved_location"
)

var knownScopes = map[string]bool{ScopeMobilityPreferences: true, ScopeAssistanceNotes: true, ScopeSavedLocation: true}

// CallerScopes maps an HMAC-authenticated service name to its granted scopes.
type CallerScopes map[string]map[string]bool

// ParseCallerScopes reads "svc=scope|scope;svc2=scope". Unknown scopes are
// rejected so a typo cannot silently grant or drop access.
func ParseCallerScopes(raw string) (CallerScopes, error) {
	out := CallerScopes{}
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, scopes, ok := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid customer context caller entry %q", entry)
		}
		granted := map[string]bool{}
		for _, scope := range strings.Split(scopes, "|") {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				continue
			}
			if !knownScopes[scope] {
				return nil, fmt.Errorf("unknown customer context scope %q for %s", scope, name)
			}
			granted[scope] = true
		}
		out[name] = granted
	}
	return out, nil
}

type PreferenceReader interface {
	GetPreferences(ctx context.Context, customerID string) (mobilitymodels.MobilityPreferences, bool, error)
}

type LocationReader interface {
	GetLocation(ctx context.Context, id, customerID string) (savedlocationmodels.SavedLocation, error)
}

type Service struct {
	scopes      CallerScopes
	preferences PreferenceReader
	locations   LocationReader
}

func NewService(scopes CallerScopes, preferences PreferenceReader, locations LocationReader) *Service {
	return &Service{scopes: scopes, preferences: preferences, locations: locations}
}

type Request struct {
	Caller          string
	CustomerID      string
	RequestedScopes []string
	SavedLocationID string
}

// MobilityContext deliberately omits safety sharing defaults: they concern
// trusted contacts, which no operational caller may read.
type MobilityContext struct {
	AccessibilityRequired    bool            `json:"accessibility_required"`
	WheelchairRequired       *bool           `json:"wheelchair_required"`
	ChildSeatPreference      *string         `json:"child_seat_preference"`
	PetTravelPreference      *string         `json:"pet_travel_preference"`
	LuggageDefault           string          `json:"luggage_default"`
	PreferredPassengerCount  *int            `json:"preferred_passenger_count"`
	CommunicationPreferences json.RawMessage `json:"communication_preferences"`
	SpecialAssistanceNotes   *string         `json:"special_assistance_notes,omitempty"`
	Version                  int64           `json:"version"`
}

type LocationContext struct {
	ID        string  `json:"id"`
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type CustomerContext struct {
	CustomerID          string           `json:"customer_id"`
	Scopes              []string         `json:"scopes"`
	MobilityPreferences *MobilityContext `json:"mobility_preferences,omitempty"`
	SavedLocation       *LocationContext `json:"saved_location,omitempty"`
	ContextVersion      string           `json:"context_version"`
}

// Resolve returns only the intersection of requested and granted scopes. A
// request for any ungranted scope is refused outright rather than trimmed,
// so a misconfigured caller fails loudly.
func (s *Service) Resolve(ctx context.Context, request Request) (CustomerContext, error) {
	granted, ok := s.scopes[strings.TrimSpace(request.Caller)]
	if !ok || len(granted) == 0 {
		return CustomerContext{}, apperrors.Forbidden("This service may not read customer context.", nil)
	}
	customerID := strings.TrimSpace(request.CustomerID)
	if customerID == "" {
		return CustomerContext{}, apperrors.NotFound("Customer not found.", nil)
	}
	requested := map[string]bool{}
	for _, scope := range request.RequestedScopes {
		if scope = strings.TrimSpace(scope); scope != "" {
			requested[scope] = true
		}
	}
	if len(requested) == 0 {
		return CustomerContext{}, apperrors.Validation("Specify the context scopes required.", []apperrors.FieldViolation{{Field: "scopes", Message: "At least one scope is required."}})
	}
	var scopes []string
	for scope := range requested {
		if !granted[scope] {
			return CustomerContext{}, apperrors.Forbidden("This service may not read the "+scope+" scope.", nil)
		}
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)

	out := CustomerContext{CustomerID: customerID, Scopes: scopes}
	versionParts := []string{customerID}
	if requested[ScopeMobilityPreferences] || requested[ScopeAssistanceNotes] {
		stored, found, err := s.preferences.GetPreferences(ctx, customerID)
		if err != nil {
			return CustomerContext{}, err
		}
		if !found {
			stored = mobilitymodels.DefaultPreferences(customerID)
		}
		mobility := &MobilityContext{
			AccessibilityRequired: stored.AccessibilityRequired, WheelchairRequired: stored.WheelchairRequired,
			ChildSeatPreference: stored.ChildSeatPreference, PetTravelPreference: stored.PetTravelPreference,
			LuggageDefault: stored.LuggageDefault, PreferredPassengerCount: stored.PreferredPassengerCount,
			CommunicationPreferences: stored.CommunicationPreferences, Version: stored.Version,
		}
		if !requested[ScopeMobilityPreferences] {
			mobility = &MobilityContext{Version: stored.Version}
		}
		if requested[ScopeAssistanceNotes] {
			mobility.SpecialAssistanceNotes = stored.SpecialAssistanceNotes
		}
		out.MobilityPreferences = mobility
		versionParts = append(versionParts, fmt.Sprintf("prefs:%d", stored.Version))
	}
	if requested[ScopeSavedLocation] {
		if strings.TrimSpace(request.SavedLocationID) == "" {
			return CustomerContext{}, apperrors.Validation("A saved location is required for this scope.", []apperrors.FieldViolation{{Field: "saved_location_id", Message: "saved_location_id is required."}})
		}
		location, err := s.locations.GetLocation(ctx, request.SavedLocationID, customerID)
		if err != nil {
			return CustomerContext{}, err
		}
		out.SavedLocation = &LocationContext{ID: location.ID, Address: location.Address, Latitude: location.Latitude, Longitude: location.Longitude}
		versionParts = append(versionParts, "loc:"+location.ID+":"+location.UpdatedAt.UTC().Format("20060102150405.000000"))
	}
	sum := sha256.Sum256([]byte(strings.Join(versionParts, "|")))
	out.ContextVersion = hex.EncodeToString(sum[:])[:32]
	return out, nil
}
