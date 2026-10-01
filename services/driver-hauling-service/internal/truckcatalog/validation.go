package truckcatalog

import "strings"

// ValidForProvider reports whether a provider may register a truck of this
// type. It replaces providerprofilemodels.ValidTruckTypes.
func ValidForProvider(raw string) bool {
	entry, ok := Lookup(raw)
	return ok && entry.ProviderSelectable
}

// ValidForBooking reports whether a booking may request this type. It replaces
// bookingmodels.ValidPreferredTruckTypes.
//
// An empty value is valid and means "any truck": it is how a customer says
// they do not care, and it has always skipped the matcher's type filter.
// Rejecting it here would break every booking that does not name a type.
func ValidForBooking(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	entry, ok := Lookup(raw)
	return ok && entry.CustomerSelectable
}

// Known reports whether the value names any catalogue type at all, selectable
// or not. Persistence and matching use this rather than the two functions
// above, so a type retired from new bookings still resolves for the historical
// rows that reference it.
func Known(raw string) bool {
	_, ok := Lookup(raw)
	return ok
}

// Canonical returns the persisted form of a known value, or empty when the
// value names no type. Callers persist the result so a mixed-case submission
// can never create a row the matcher will not find.
func Canonical(raw string) string {
	entry, ok := Lookup(raw)
	if !ok {
		return ""
	}
	return string(entry.Code)
}

// ProviderSelectableCodes and CustomerSelectableCodes exist for validation
// error messages and API listings, so the allowed set shown to a caller is
// always generated from the catalogue rather than retyped.
func ProviderSelectableCodes() []string { return Codes(ProviderSelectable()) }

// CustomerSelectableCodes lists the booking-requestable codes.
func CustomerSelectableCodes() []string { return Codes(CustomerSelectable()) }
