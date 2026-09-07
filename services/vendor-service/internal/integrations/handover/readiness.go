package handover

// CapabilityStatus is the audited backend capability level. It is derived
// from authoritative implementations; it is not a deployment flag.
type CapabilityStatus string

const (
	StatusAvailable  CapabilityStatus = "AVAILABLE"
	StatusPartial    CapabilityStatus = "PARTIAL"
	StatusNotPresent CapabilityStatus = "NOT_PRESENT"
)

// Readiness is the safe dependency projection consumed by rollout/config
// gates. It contains no OTP, QR payload or signing material.
type Readiness struct {
	Dependency string           `json:"dependency"`
	OTPStatus  CapabilityStatus `json:"otp_status"`
	QRStatus   CapabilityStatus `json:"qr_status"`
	Status     CapabilityStatus `json:"status"`
	Activation string           `json:"activation"`
	Reason     string           `json:"reason,omitempty"`
}

// HaulingSecureHandover reports the current repository-audited truth.
//
// Hauling owns a versioned, hash-only challenge whose independently random OTP
// and opaque QR credentials share expiry, attempts and single-use consumption.
// This projection removes the dependency gate; it does not enable Commerce.
func HaulingSecureHandover() Readiness {
	return Readiness{
		Dependency: "HAULING_SECURE_HANDOVER",
		OTPStatus:  StatusAvailable,
		QRStatus:   StatusAvailable,
		Status:     StatusAvailable,
		Activation: "READY",
	}
}

func DispatchSecureHandover() Readiness {
	return Readiness{Dependency: "DISPATCH_SECURE_HANDOVER", OTPStatus: StatusAvailable, QRStatus: StatusAvailable, Status: StatusAvailable, Activation: "READY"}
}

func TaxiSecureHandover() Readiness {
	return Readiness{Dependency: "TAXI_GOODS_SECURE_HANDOVER", OTPStatus: StatusAvailable, QRStatus: StatusAvailable, Status: StatusAvailable, Activation: "READY"}
}

func VendorOwnedSecureHandover() Readiness {
	return Readiness{Dependency: "DISPATCH_VENDOR_OWNED_SECURE_HANDOVER", OTPStatus: StatusAvailable, QRStatus: StatusAvailable, Status: StatusAvailable, Activation: "READY"}
}

func (r Readiness) Available() bool {
	return r.Status == StatusAvailable && r.Activation == "READY"
}
