package delivery

import (
	"regexp"
	"strings"
)

// e164 is a permissive E.164 phone matcher: leading +, then 8-15 digits.
var e164 = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

func normalizeTier(tier string) string {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case ServiceTierExpress:
		return ServiceTierExpress
	case ServiceTierStandard, "":
		return ServiceTierStandard
	default:
		return strings.ToLower(strings.TrimSpace(tier)) // invalid; caught by validation
	}
}

func validLatitude(v float64) bool  { return v >= -90 && v <= 90 }
func validLongitude(v float64) bool { return v >= -180 && v <= 180 }

// validateCoord appends field errors for a required lat/lng pair.
func validateCoord(fields *[]FieldError, latField string, lat *float64, lngField string, lng *float64) {
	switch {
	case lat == nil:
		*fields = append(*fields, FieldError{latField, "This field is required."})
	case !validLatitude(*lat):
		*fields = append(*fields, FieldError{latField, "Must be a valid latitude between -90 and 90."})
	}
	switch {
	case lng == nil:
		*fields = append(*fields, FieldError{lngField, "This field is required."})
	case !validLongitude(*lng):
		*fields = append(*fields, FieldError{lngField, "Must be a valid longitude between -180 and 180."})
	}
	// Reject the (0,0) null island as an unusable coordinate.
	if lat != nil && lng != nil && *lat == 0 && *lng == 0 {
		*fields = append(*fields, FieldError{latField, "Coordinates must not be zero."})
	}
}

func validateQuote(req QuoteRequest) []FieldError {
	fields := []FieldError{}
	validateCoord(&fields, "pickup_lat", req.PickupLat, "pickup_lng", req.PickupLng)
	validateCoord(&fields, "dest_lat", req.DestLat, "dest_lng", req.DestLng)
	if s := strings.TrimSpace(req.PackageSize); s != "" {
		if _, ok := validPackageSizes[strings.ToLower(s)]; !ok {
			fields = append(fields, FieldError{"package_size", "Must be one of: small, medium, large."})
		}
	}
	if t := strings.TrimSpace(req.ServiceTier); t != "" {
		if !isValidTier(t) {
			fields = append(fields, FieldError{"service_tier", "Must be one of: standard, express."})
		}
	}
	return fields
}

func validateDelivery(req DeliveryRequest) []FieldError {
	fields := []FieldError{}

	if strings.TrimSpace(req.PickupAddress) == "" {
		fields = append(fields, FieldError{"pickup_address", "This field is required."})
	}
	if strings.TrimSpace(req.DestAddress) == "" {
		fields = append(fields, FieldError{"dest_address", "This field is required."})
	}
	validateCoord(&fields, "pickup_lat", req.PickupLat, "pickup_lng", req.PickupLng)
	validateCoord(&fields, "dest_lat", req.DestLat, "dest_lng", req.DestLng)

	if strings.TrimSpace(req.ReceiverName) == "" {
		fields = append(fields, FieldError{"receiver_name", "This field is required."})
	}
	if phone := strings.TrimSpace(req.ReceiverPhone); phone == "" {
		fields = append(fields, FieldError{"receiver_phone", "This field is required."})
	} else if !e164.MatchString(phone) {
		fields = append(fields, FieldError{"receiver_phone", "Must be E.164 format."})
	}

	if s := strings.TrimSpace(req.PackageType); s != "" {
		if _, ok := validPackageTypes[strings.ToLower(s)]; !ok {
			fields = append(fields, FieldError{"package_type", "Must be one of: documents, food, electronics, clothing, medical, other."})
		}
	}
	if s := strings.TrimSpace(req.PackageSize); s != "" {
		if _, ok := validPackageSizes[strings.ToLower(s)]; !ok {
			fields = append(fields, FieldError{"package_size", "Must be one of: small, medium, large."})
		}
	}
	if t := strings.TrimSpace(req.ServiceTier); t != "" && !isValidTier(t) {
		fields = append(fields, FieldError{"service_tier", "Must be one of: standard, express."})
	}

	switch strings.TrimSpace(req.PaymentMethod) {
	case PaymentMethodWallet:
	case PaymentMethodPaystack:
		if strings.TrimSpace(req.CustomerEmail) == "" {
			fields = append(fields, FieldError{"customer_email", "Required for card payments."})
		}
	case "":
		fields = append(fields, FieldError{"payment_method", "This field is required."})
	default:
		fields = append(fields, FieldError{"payment_method", "Must be one of: wallet, paystack."})
	}

	if strings.TrimSpace(req.CustomerID) == "" {
		fields = append(fields, FieldError{"customer_id", "This field is required."})
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		fields = append(fields, FieldError{"idempotency_key", "This field is required."})
	}

	return fields
}

func isValidTier(tier string) bool {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case ServiceTierStandard, ServiceTierExpress:
		return true
	default:
		return false
	}
}

var validStatuses = map[string]struct{}{
	StatusPending: {}, StatusDispatching: {}, StatusAccepted: {}, StatusInProgress: {},
	StatusCompleted: {}, StatusCancelled: {}, StatusFailed: {}, StatusNoProvider: {},
}

func isValidStatus(status string) bool {
	_, ok := validStatuses[strings.ToLower(strings.TrimSpace(status))]
	return ok
}
