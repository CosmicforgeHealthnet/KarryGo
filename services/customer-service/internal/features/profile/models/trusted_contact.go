package profilemodels

import (
	"strings"
	"time"
	"unicode/utf8"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/phonenumber"
)

// TrustedContact is the broader presentation of the canonical
// emergency_contacts row. One person is one record. Consent is explicit per
// capability, and trip sharing is never implied by being a contact.
type TrustedContact struct {
	ID                     string    `json:"id"`
	CustomerID             string    `json:"-"`
	Name                   string    `json:"name"`
	Phone                  string    `json:"phone"`
	RelationshipLabel      *string   `json:"relationship_label"`
	EmergencyAlertsEnabled bool      `json:"emergency_alerts_enabled"`
	TripSharingEnabled     bool      `json:"trip_sharing_enabled"`
	Active                 bool      `json:"active"`
	Version                int64     `json:"version"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// Normalize validates the contact and canonicalizes the phone to +234.
func (t *TrustedContact) Normalize() error {
	var fields []apperrors.FieldViolation
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" || utf8.RuneCountInString(t.Name) > 120 {
		fields = append(fields, apperrors.FieldViolation{Field: "name", Message: "Name is required and must be 120 characters or fewer."})
	}
	phone, err := phonenumber.NormalizeNigerianPhoneNumber(strings.TrimSpace(t.Phone))
	if err != nil {
		fields = append(fields, apperrors.FieldViolation{Field: "phone", Message: "Phone number is not valid."})
	} else {
		t.Phone = phone
	}
	if t.RelationshipLabel != nil {
		label := strings.TrimSpace(*t.RelationshipLabel)
		if label == "" {
			t.RelationshipLabel = nil
		} else if utf8.RuneCountInString(label) > 60 {
			fields = append(fields, apperrors.FieldViolation{Field: "relationship_label", Message: "Relationship must be 60 characters or fewer."})
		} else {
			t.RelationshipLabel = &label
		}
	}
	if len(fields) > 0 {
		return apperrors.Validation("Check the contact's details.", fields)
	}
	return nil
}
