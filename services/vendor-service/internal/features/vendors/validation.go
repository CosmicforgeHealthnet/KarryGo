package vendors

import (
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

var (
	phonePattern = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
	mediaPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{1,255}$`)
)

func validation(fields ...apperrors.FieldViolation) error {
	return apperrors.Validation("Check your details.", fields)
}

func validateBusiness(b Business) error {
	var fields []apperrors.FieldViolation
	if n := len(strings.TrimSpace(b.LegalName)); n < 2 || n > 200 {
		fields = append(fields, apperrors.FieldViolation{Field: "legal_name", Message: "Legal name must be between 2 and 200 characters."})
	}
	if b.TradingName != "" {
		if n := len(strings.TrimSpace(b.TradingName)); n < 2 || n > 200 {
			fields = append(fields, apperrors.FieldViolation{Field: "trading_name", Message: "Trading name must be between 2 and 200 characters."})
		}
	}
	if b.SupportEmail != "" {
		if _, err := mail.ParseAddress(b.SupportEmail); err != nil {
			fields = append(fields, apperrors.FieldViolation{Field: "support_email", Message: "Enter a valid email address."})
		}
	}
	if b.SupportPhone != "" && !phonePattern.MatchString(b.SupportPhone) {
		fields = append(fields, apperrors.FieldViolation{Field: "support_phone", Message: "Use E.164 format, for example +2348012345678."})
	}
	if !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(b.Currency) {
		fields = append(fields, apperrors.FieldViolation{Field: "currency", Message: "Use a three-letter uppercase currency code."})
	}
	if _, err := time.LoadLocation(b.Timezone); err != nil {
		fields = append(fields, apperrors.FieldViolation{Field: "timezone", Message: "Use a valid IANA timezone."})
	}
	if len(fields) > 0 {
		return validation(fields...)
	}
	return nil
}

func validateBrand(b Brand) error {
	var fields []apperrors.FieldViolation
	if n := len(strings.TrimSpace(b.DisplayName)); n < 2 || n > 160 {
		fields = append(fields, apperrors.FieldViolation{Field: "display_name", Message: "Display name must be between 2 and 160 characters."})
	}
	if len(b.Description) > 2000 {
		fields = append(fields, apperrors.FieldViolation{Field: "description", Message: "Description cannot exceed 2000 characters."})
	}
	seen := map[string]bool{}
	for i, ref := range b.MediaRefs {
		if ref.Kind != "LOGO" && ref.Kind != "COVER" && ref.Kind != "GALLERY" {
			fields = append(fields, apperrors.FieldViolation{Field: "media_refs", Message: "Media kind must be LOGO, COVER, or GALLERY."})
			break
		}
		if strings.HasPrefix(ref.MediaRef, "http://") || strings.HasPrefix(ref.MediaRef, "https://") || !mediaPattern.MatchString(ref.MediaRef) {
			fields = append(fields, apperrors.FieldViolation{Field: "media_refs", Message: "Media references must be opaque media-service identifiers, not URLs."})
			break
		}
		key := ref.Kind + ":" + ref.MediaRef
		if seen[key] {
			fields = append(fields, apperrors.FieldViolation{Field: "media_refs", Message: "Duplicate media references are not allowed."})
			break
		}
		seen[key] = true
		_ = i
	}
	if len(fields) > 0 {
		return validation(fields...)
	}
	return nil
}

func validateBranch(b Branch) error {
	var fields []apperrors.FieldViolation
	if n := len(strings.TrimSpace(b.DisplayName)); n < 2 || n > 160 {
		fields = append(fields, apperrors.FieldViolation{Field: "display_name", Message: "Display name must be between 2 and 160 characters."})
	}
	if n := len(strings.TrimSpace(b.Address.Line1)); n < 3 || n > 240 {
		fields = append(fields, apperrors.FieldViolation{Field: "address.line1", Message: "Address line 1 must be between 3 and 240 characters."})
	}
	if len(strings.TrimSpace(b.Address.City)) < 2 {
		fields = append(fields, apperrors.FieldViolation{Field: "address.city", Message: "City is required."})
	}
	if len(strings.TrimSpace(b.Address.Region)) < 2 {
		fields = append(fields, apperrors.FieldViolation{Field: "address.region", Message: "Region is required."})
	}
	if !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(b.Address.CountryCode) {
		fields = append(fields, apperrors.FieldViolation{Field: "address.country_code", Message: "Use a two-letter uppercase country code."})
	}
	if b.Latitude < -90 || b.Latitude > 90 {
		fields = append(fields, apperrors.FieldViolation{Field: "latitude", Message: "Latitude must be between -90 and 90."})
	}
	if b.Longitude < -180 || b.Longitude > 180 {
		fields = append(fields, apperrors.FieldViolation{Field: "longitude", Message: "Longitude must be between -180 and 180."})
	}
	if _, err := time.LoadLocation(b.Timezone); err != nil {
		fields = append(fields, apperrors.FieldViolation{Field: "timezone", Message: "Use a valid IANA timezone."})
	}
	if len(fields) > 0 {
		return validation(fields...)
	}
	return nil
}

func validateHours(windows []OperatingWindow) error {
	byDay := map[int][]OperatingWindow{}
	for _, w := range windows {
		if w.Weekday < 1 || w.Weekday > 7 {
			return validation(apperrors.FieldViolation{Field: "hours.weekday", Message: "Weekday must be from 1 (Monday) to 7 (Sunday)."})
		}
		if w.IsClosed {
			if w.OpensAt != "" || w.ClosesAt != "" {
				return validation(apperrors.FieldViolation{Field: "hours", Message: "Closed days cannot include opening times."})
			}
		} else {
			o, oe := time.Parse("15:04", w.OpensAt)
			c, ce := time.Parse("15:04", w.ClosesAt)
			if oe != nil || ce != nil || !o.Before(c) {
				return validation(apperrors.FieldViolation{Field: "hours", Message: "Open intervals must use HH:MM, end after start, and cannot run overnight."})
			}
		}
		byDay[w.Weekday] = append(byDay[w.Weekday], w)
	}
	for _, day := range byDay {
		sort.Slice(day, func(i, j int) bool { return day[i].OpensAt < day[j].OpensAt })
		if len(day) > 1 && day[0].IsClosed {
			return validation(apperrors.FieldViolation{Field: "hours", Message: "A closed day cannot also contain open intervals."})
		}
		for i := 1; i < len(day); i++ {
			if day[i-1].ClosesAt > day[i].OpensAt {
				return validation(apperrors.FieldViolation{Field: "hours", Message: "Opening intervals cannot overlap."})
			}
		}
	}
	return nil
}

func validateServiceArea(a ServiceArea) error {
	if a.Mode != "RADIUS" || a.CenterLatitude < -90 || a.CenterLatitude > 90 || a.CenterLongitude < -180 || a.CenterLongitude > 180 || a.RadiusMeters < 1 || a.RadiusMeters > 200000 {
		return validation(apperrors.FieldViolation{Field: "service_area", Message: "Use RADIUS with valid coordinates and a radius from 1 to 200000 metres."})
	}
	return nil
}

func validateSettings(s OperationsSettings) error {
	if s.SchemaVersion != 1 {
		return validation(apperrors.FieldViolation{Field: "schema_version", Message: "Only operations settings schema version 1 is supported."})
	}
	return nil
}

func ValidateBusiness(b Business) error { return validateBusiness(b) }
