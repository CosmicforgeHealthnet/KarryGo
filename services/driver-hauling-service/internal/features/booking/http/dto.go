package bookinghttp

import "time"

type estimateFareRequest struct {
	PickupLat     float64 `json:"pickup_lat"`
	PickupLng     float64 `json:"pickup_lng"`
	DropoffLat    float64 `json:"dropoff_lat"`
	DropoffLng    float64 `json:"dropoff_lng"`
	CargoWeightKg int     `json:"cargo_weight_kg"`
	HelperCount   int     `json:"helper_count"`
}

type createBookingRequest struct {
	PickupAddress      string     `json:"pickup_address"`
	PickupLat          float64    `json:"pickup_lat"`
	PickupLng          float64    `json:"pickup_lng"`
	DropoffAddress     string     `json:"dropoff_address"`
	DropoffLat         float64    `json:"dropoff_lat"`
	DropoffLng         float64    `json:"dropoff_lng"`
	PreferredTruckType string     `json:"preferred_truck_type"`
	CargoWeightKg      int        `json:"cargo_weight_kg"`
	CargoDescription   string     `json:"cargo_description"`
	RequiresHelpers    bool       `json:"requires_helpers"`
	HelperCount        int        `json:"helper_count"`
	WeightCategory     string     `json:"weight_category"`
	ReceiverName       string     `json:"receiver_name"`
	ReceiverPhone      string     `json:"receiver_phone"`
	ReceiverEmail      string     `json:"receiver_email,omitempty"`
	ReceiverCustomerID string     `json:"receiver_customer_id,omitempty"`
	PackageContent     string     `json:"package_content"`
	PackageSize        string     `json:"package_size"`
	IsFragile          bool       `json:"is_fragile"`
	PaymentMethod      string     `json:"payment_method"`
	ScheduledAt        *time.Time `json:"scheduled_at"`
}

// initiateCardPaymentRequest carries the email Paystack requires to start the
// up-front card payment for a booking.
type initiateCardPaymentRequest struct {
	CustomerEmail string `json:"customer_email"`
}

type submitReviewRequest struct {
	Rating           int    `json:"rating"`
	ReviewText       string `json:"review_text"`
	RecommendsDriver *bool  `json:"recommends_driver"`
}

type cancelBookingRequest struct {
	Reason string `json:"reason"`
}

// confirmDeliveryRequest carries the customer's delivery verification code
// (required) plus the optional proof-of-delivery URLs uploaded to
// media-file-service by the provider app before confirming.
type confirmDeliveryRequest struct {
	Code          string `json:"code"`
	QRToken       string `json:"qr_token"`
	ProofPhotoURL string `json:"proof_photo_url"`
	SignatureURL  string `json:"signature_url"`
}

// verifyDriverRequest carries the driver's pickup credential as entered or
// scanned by the customer.
type verifyDriverRequest struct {
	Code    string `json:"code"`
	QRToken string `json:"qr_token"`
}
