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
	"time"

	mobilitymodels "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/models"
	savedlocationmodels "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	ScopeMobilityPreferences = "mobility_preferences"
	ScopeAssistanceNotes     = "assistance_notes"
	ScopeSavedLocation       = "saved_location"
	// ScopeGuestTraveller lets a booking authority freeze who is actually
	// travelling when a customer books for someone else (Customer V4 §5.1).
	ScopeGuestTraveller = "guest_traveller"
	// ScopeAccountStanding lets a booking authority ask ONLY whether the
	// customer may create a new booking, without reading any other context.
	// It is answered only from the source-owned standing; with no standing
	// reader configured it is refused as unavailable, never answered
	// "eligible".
	ScopeAccountStanding = "account_standing"
)

var knownScopes = map[string]bool{ScopeMobilityPreferences: true, ScopeAssistanceNotes: true, ScopeSavedLocation: true, ScopeGuestTraveller: true, ScopeAccountStanding: true}

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

// GuestReader reads one active traveller owned by the customer; another
// customer's traveller reads as not found.
type GuestReader interface {
	GetGuest(ctx context.Context, customerID, id string) (mobilitymodels.GuestTraveller, error)
}

// StandingReader reads the customer's source-owned administrative account
// standing. Restriction is NONE, LOGIN_BLOCKED or BOOKING_HOLD; version 0
// means no administrative decision has been recorded.
type StandingReader interface {
	AccountStanding(ctx context.Context, customerID string) (restriction string, version int64, err error)
}

type Service struct {
	standing    StandingReader
	scopes      CallerScopes
	preferences PreferenceReader
	locations   LocationReader
	guests      GuestReader
	now         func() time.Time
}

func NewService(scopes CallerScopes, preferences PreferenceReader, locations LocationReader) *Service {
	return &Service{scopes: scopes, preferences: preferences, locations: locations, now: time.Now}
}

// WithClock overrides the clock used for account_standing.as_of (tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// WithGuests enables the guest_traveller scope. Without a reader the scope
// is refused as unavailable rather than answered empty.
func (s *Service) WithGuests(guests GuestReader) *Service {
	s.guests = guests
	return s
}

// WithStanding makes every context response carry the authoritative account
// standing so booking authorities can enforce administrative booking holds.
func (s *Service) WithStanding(standing StandingReader) *Service {
	s.standing = standing
	return s
}

// AccountStandingContext is the additive, minimal booking-eligibility fact.
// It never carries the administrative reason or decision history. AsOf is
// when Customer Service read the standing from its own database (UTC,
// RFC3339Nano) so a consumer can refuse a stale answer.
type AccountStandingContext struct {
	BookingEligible bool   `json:"booking_eligible"`
	Restriction     string `json:"restriction"`
	StandingVersion int64  `json:"standing_version"`
	AsOf            string `json:"as_of"`
}

type Request struct {
	Caller           string
	CustomerID       string
	RequestedScopes  []string
	SavedLocationID  string
	GuestTravellerID string
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

// GuestTravellerContext is the minimum a booking authority needs to freeze
// the traveller: free-text accessibility notes and the relationship label are
// deliberately absent. Requirements are Customer Service's normalized,
// enumerated object, never free text.
type GuestTravellerContext struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Phone                *string         `json:"phone,omitempty"`
	MobilityRequirements json.RawMessage `json:"mobility_requirements,omitempty"`
	Version              int64           `json:"version"`
}

type CustomerContext struct {
	CustomerID          string                  `json:"customer_id"`
	Scopes              []string                `json:"scopes"`
	MobilityPreferences *MobilityContext        `json:"mobility_preferences,omitempty"`
	SavedLocation       *LocationContext        `json:"saved_location,omitempty"`
	GuestTraveller      *GuestTravellerContext  `json:"guest_traveller,omitempty"`
	AccountStanding     *AccountStandingContext `json:"account_standing,omitempty"`
	ContextVersion      string                  `json:"context_version"`
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
	if requested[ScopeGuestTraveller] {
		if strings.TrimSpace(request.GuestTravellerID) == "" {
			return CustomerContext{}, apperrors.Validation("A traveller is required for this scope.", []apperrors.FieldViolation{{Field: "guest_traveller_id", Message: "guest_traveller_id is required."}})
		}
		if s.guests == nil {
			return CustomerContext{}, apperrors.Unavailable("Traveller context is unavailable.", nil)
		}
		guest, err := s.guests.GetGuest(ctx, customerID, strings.TrimSpace(request.GuestTravellerID))
		if err != nil {
			return CustomerContext{}, err
		}
		out.GuestTraveller = &GuestTravellerContext{ID: guest.ID, Name: guest.Name, Phone: guest.Phone,
			MobilityRequirements: guest.MobilityRequirementsJSON, Version: guest.Version}
		versionParts = append(versionParts, fmt.Sprintf("guest:%s:%d", guest.ID, guest.Version))
	}
	if requested[ScopeAccountStanding] && s.standing == nil {
		return CustomerContext{}, apperrors.Unavailable("Customer standing is unavailable.", nil)
	}
	if s.standing != nil {
		// as_of is taken before the read so it never claims a fresher
		// answer than the database actually gave.
		asOf := s.now().UTC()
		restriction, version, err := s.standing.AccountStanding(ctx, customerID)
		if err != nil {
			return CustomerContext{}, apperrors.Unavailable("Customer standing is unavailable.", err)
		}
		out.AccountStanding = &AccountStandingContext{BookingEligible: restriction == "NONE", Restriction: restriction,
			StandingVersion: version, AsOf: asOf.Format(time.RFC3339Nano)}
		versionParts = append(versionParts, fmt.Sprintf("standing:%d", version))
	}
	sum := sha256.Sum256([]byte(strings.Join(versionParts, "|")))
	out.ContextVersion = hex.EncodeToString(sum[:])[:32]
	return out, nil
}
