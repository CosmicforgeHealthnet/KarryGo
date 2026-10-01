// Package mobilitymodels holds customer-owned reusable mobility context:
// mobility preferences and guest travellers. None of these types is a booking
// requirement; operational services freeze hard requirements per booking.
package mobilitymodels

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/phonenumber"
)

const (
	maxNotesRunes        = 500
	maxNameRunes         = 120
	maxRelationshipRunes = 60
	maxJSONBytes         = 2048
)

var (
	childSeatValues = set("NOT_NEEDED", "INFANT", "TODDLER", "BOOSTER")
	petValues       = set("NONE", "SMALL_PET", "LARGE_PET", "SERVICE_ANIMAL")
	luggageValues   = set("NONE", "CARRY_ON", "STANDARD", "LARGE")
	channelValues   = set("CALL", "TEXT", "IN_APP")
)

// MobilityPreferences is the single per-customer preference aggregate.
type MobilityPreferences struct {
	CustomerID               string          `json:"customer_id"`
	AccessibilityRequired    bool            `json:"accessibility_required"`
	WheelchairRequired       *bool           `json:"wheelchair_required"`
	ChildSeatPreference      *string         `json:"child_seat_preference"`
	PetTravelPreference      *string         `json:"pet_travel_preference"`
	LuggageDefault           string          `json:"luggage_default"`
	PreferredPassengerCount  *int            `json:"preferred_passenger_count"`
	CommunicationPreferences json.RawMessage `json:"communication_preferences"`
	SafetyShareDefault       bool            `json:"safety_share_default"`
	SpecialAssistanceNotes   *string         `json:"special_assistance_notes"`
	Version                  int64           `json:"version"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
}

// DefaultPreferences is what a customer without a stored row reads. Version 0
// tells the client the first PUT must send expected_version 0.
func DefaultPreferences(customerID string) MobilityPreferences {
	return MobilityPreferences{CustomerID: customerID, LuggageDefault: "NONE", CommunicationPreferences: json.RawMessage(`{}`)}
}

// Normalize trims and upper-cases enumerations, then validates every field.
func (p *MobilityPreferences) Normalize() error {
	var fields []apperrors.FieldViolation
	add := func(field, message string) {
		fields = append(fields, apperrors.FieldViolation{Field: field, Message: message})
	}
	p.ChildSeatPreference = normalizeEnum(p.ChildSeatPreference)
	if p.ChildSeatPreference != nil && !childSeatValues[*p.ChildSeatPreference] {
		add("child_seat_preference", "Child seat preference is not supported.")
	}
	p.PetTravelPreference = normalizeEnum(p.PetTravelPreference)
	if p.PetTravelPreference != nil && !petValues[*p.PetTravelPreference] {
		add("pet_travel_preference", "Pet travel preference is not supported.")
	}
	p.LuggageDefault = strings.ToUpper(strings.TrimSpace(p.LuggageDefault))
	if p.LuggageDefault == "" {
		p.LuggageDefault = "NONE"
	}
	if !luggageValues[p.LuggageDefault] {
		add("luggage_default", "Luggage default is not supported.")
	}
	if p.PreferredPassengerCount != nil && (*p.PreferredPassengerCount < 1 || *p.PreferredPassengerCount > 8) {
		add("preferred_passenger_count", "Passenger count must be between 1 and 8.")
	}
	p.SpecialAssistanceNotes = normalizeText(p.SpecialAssistanceNotes)
	if p.SpecialAssistanceNotes != nil && utf8.RuneCountInString(*p.SpecialAssistanceNotes) > maxNotesRunes {
		add("special_assistance_notes", "Notes must be 500 characters or fewer.")
	}
	normalized, err := normalizeCommunication(p.CommunicationPreferences)
	if err != "" {
		add("communication_preferences", err)
	} else {
		p.CommunicationPreferences = normalized
	}
	if len(fields) > 0 {
		return apperrors.Validation("Check your mobility preferences.", fields)
	}
	return nil
}

// normalizeCommunication accepts only a small, typed vocabulary so free-form
// JSON can never smuggle arbitrary personal data into downstream contexts.
func normalizeCommunication(raw json.RawMessage) (json.RawMessage, string) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return json.RawMessage(`{}`), ""
	}
	if len(raw) > maxJSONBytes {
		return nil, "Communication preferences are too large."
	}
	var input map[string]json.RawMessage
	if json.Unmarshal(raw, &input) != nil {
		return nil, "Communication preferences must be an object."
	}
	out := map[string]any{}
	for key, value := range input {
		switch key {
		case "preferred_channel":
			var channel string
			if json.Unmarshal(value, &channel) != nil || !channelValues[strings.ToUpper(strings.TrimSpace(channel))] {
				return nil, "preferred_channel must be CALL, TEXT or IN_APP."
			}
			out[key] = strings.ToUpper(strings.TrimSpace(channel))
		case "quiet_ride", "hearing_impaired", "speech_impaired":
			var flag bool
			if json.Unmarshal(value, &flag) != nil {
				return nil, key + " must be a boolean."
			}
			out[key] = flag
		case "language":
			var language string
			if json.Unmarshal(value, &language) != nil || len(strings.TrimSpace(language)) < 2 || len(strings.TrimSpace(language)) > 12 {
				return nil, "language must be a short language tag."
			}
			out[key] = strings.ToLower(strings.TrimSpace(language))
		default:
			return nil, "Unsupported communication preference: " + key + "."
		}
	}
	encoded, _ := json.Marshal(out)
	return encoded, ""
}

// GuestTraveller is reusable context for someone the customer books for.
type GuestTraveller struct {
	ID                       string          `json:"id"`
	CustomerID               string          `json:"-"`
	Name                     string          `json:"name"`
	Phone                    *string         `json:"phone"`
	RelationshipLabel        *string         `json:"relationship_label"`
	AccessibilityNotes       *string         `json:"accessibility_notes"`
	MobilityRequirementsJSON json.RawMessage `json:"mobility_requirements_json"`
	Active                   bool            `json:"active"`
	Version                  int64           `json:"version"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
}

// Normalize validates a guest traveller and canonicalizes the phone number.
func (g *GuestTraveller) Normalize() error {
	var fields []apperrors.FieldViolation
	add := func(field, message string) {
		fields = append(fields, apperrors.FieldViolation{Field: field, Message: message})
	}
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" || utf8.RuneCountInString(g.Name) > maxNameRunes {
		add("name", "Name is required and must be 120 characters or fewer.")
	}
	g.Phone = normalizeText(g.Phone)
	if g.Phone != nil {
		normalized, err := phonenumber.NormalizeNigerianPhoneNumber(*g.Phone)
		if err != nil {
			add("phone", "Phone number is not valid.")
		} else {
			g.Phone = &normalized
		}
	}
	g.RelationshipLabel = normalizeText(g.RelationshipLabel)
	if g.RelationshipLabel != nil && utf8.RuneCountInString(*g.RelationshipLabel) > maxRelationshipRunes {
		add("relationship_label", "Relationship must be 60 characters or fewer.")
	}
	g.AccessibilityNotes = normalizeText(g.AccessibilityNotes)
	if g.AccessibilityNotes != nil && utf8.RuneCountInString(*g.AccessibilityNotes) > maxNotesRunes {
		add("accessibility_notes", "Accessibility notes must be 500 characters or fewer.")
	}
	requirements, message := normalizeGuestRequirements(g.MobilityRequirementsJSON)
	if message != "" {
		add("mobility_requirements_json", message)
	} else {
		g.MobilityRequirementsJSON = requirements
	}
	if len(fields) > 0 {
		return apperrors.Validation("Check the traveller's details.", fields)
	}
	return nil
}

func normalizeGuestRequirements(raw json.RawMessage) (json.RawMessage, string) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, ""
	}
	if len(raw) > maxJSONBytes {
		return nil, "Mobility requirements are too large."
	}
	var input map[string]json.RawMessage
	if json.Unmarshal(raw, &input) != nil {
		return nil, "Mobility requirements must be an object."
	}
	out := map[string]any{}
	for key, value := range input {
		switch key {
		case "wheelchair_required", "accessibility_required":
			var flag bool
			if json.Unmarshal(value, &flag) != nil {
				return nil, key + " must be a boolean."
			}
			out[key] = flag
		case "child_seat_preference", "pet_travel_preference", "luggage_default":
			var text string
			if json.Unmarshal(value, &text) != nil {
				return nil, key + " must be a string."
			}
			text = strings.ToUpper(strings.TrimSpace(text))
			allowed := map[string]map[string]bool{"child_seat_preference": childSeatValues, "pet_travel_preference": petValues, "luggage_default": luggageValues}[key]
			if !allowed[text] {
				return nil, key + " is not supported."
			}
			out[key] = text
		default:
			return nil, "Unsupported mobility requirement: " + key + "."
		}
	}
	encoded, _ := json.Marshal(out)
	return encoded, ""
}

func normalizeEnum(value *string) *string {
	value = normalizeText(value)
	if value == nil {
		return nil
	}
	upper := strings.ToUpper(*value)
	return &upper
}

func normalizeText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func set(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}
