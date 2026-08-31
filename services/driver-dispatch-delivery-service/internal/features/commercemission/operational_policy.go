package commercemission

import (
	"errors"
	"sort"
	"strings"
	"time"
)

type EvidenceType string

const (
	EvidencePickup           EvidenceType = "PICKUP"
	EvidenceDelivery         EvidenceType = "DELIVERY"
	EvidenceDamage           EvidenceType = "DAMAGE"
	EvidenceDiscrepancy      EvidenceType = "DISCREPANCY"
	EvidenceFailedAttempt    EvidenceType = "FAILED_ATTEMPT"
	EvidenceReturn           EvidenceType = "RETURN"
	EvidenceProviderTransfer EvidenceType = "PROVIDER_TRANSFER"
)

func (t EvidenceType) Valid() bool {
	switch t {
	case EvidencePickup, EvidenceDelivery, EvidenceDamage, EvidenceDiscrepancy,
		EvidenceFailedAttempt, EvidenceReturn, EvidenceProviderTransfer:
		return true
	default:
		return false
	}
}

type ExceptionReason string

const (
	ReasonVendorNotReady      ExceptionReason = "VENDOR_NOT_READY"
	ReasonPackageMismatch     ExceptionReason = "PACKAGE_MISMATCH"
	ReasonProhibitedItem      ExceptionReason = "PROHIBITED_ITEM"
	ReasonRestrictedItem      ExceptionReason = "RESTRICTED_ITEM"
	ReasonDamagedPackage      ExceptionReason = "DAMAGED_PACKAGE"
	ReasonReceiverUnavailable ExceptionReason = "RECEIVER_UNAVAILABLE"
	ReasonReceiverRefused     ExceptionReason = "RECEIVER_REFUSED"
	ReasonWrongAddress        ExceptionReason = "WRONG_ADDRESS"
	ReasonAddressInaccessible ExceptionReason = "ADDRESS_INACCESSIBLE"
	ReasonVehicleBreakdown    ExceptionReason = "VEHICLE_BREAKDOWN"
	ReasonProviderIllness     ExceptionReason = "PROVIDER_ILLNESS"
	ReasonWeatherRoad         ExceptionReason = "WEATHER_ROAD"
	ReasonSecurityRisk        ExceptionReason = "SECURITY_RISK"
	ReasonSafetyIncident      ExceptionReason = "SAFETY_INCIDENT"
)

func (r ExceptionReason) Valid() bool {
	switch r {
	case ReasonVendorNotReady, ReasonPackageMismatch, ReasonProhibitedItem,
		ReasonRestrictedItem, ReasonDamagedPackage, ReasonReceiverUnavailable,
		ReasonReceiverRefused, ReasonWrongAddress, ReasonAddressInaccessible,
		ReasonVehicleBreakdown, ReasonProviderIllness, ReasonWeatherRoad,
		ReasonSecurityRisk, ReasonSafetyIncident:
		return true
	default:
		return false
	}
}

func (r ExceptionReason) EvidenceType() EvidenceType {
	switch r {
	case ReasonPackageMismatch:
		return EvidenceDiscrepancy
	case ReasonDamagedPackage:
		return EvidenceDamage
	default:
		return EvidenceFailedAttempt
	}
}

func (r ExceptionReason) RequiresReplacement() bool {
	return r == ReasonVehicleBreakdown || r == ReasonProviderIllness || r == ReasonSafetyIncident
}

func (r ExceptionReason) FailClosedBeforeCustody() bool {
	return r == ReasonPackageMismatch || r == ReasonProhibitedItem || r == ReasonRestrictedItem
}

type RecoveryAction string

const (
	RecoveryWait                RecoveryAction = "WAIT"
	RecoveryRetryPickup         RecoveryAction = "RETRY_PICKUP"
	RecoveryRetryDelivery       RecoveryAction = "RETRY_DELIVERY"
	RecoveryReschedule          RecoveryAction = "RESCHEDULE"
	RecoveryReplan              RecoveryAction = "REPLAN"
	RecoveryAlternativeReceiver RecoveryAction = "ALTERNATIVE_RECEIVER"
	RecoveryReturnSender        RecoveryAction = "RETURN_TO_SENDER"
	RecoveryReturnVendor        RecoveryAction = "RETURN_TO_VENDOR"
	RecoveryProviderReplacement RecoveryAction = "PROVIDER_REPLACEMENT"
	RecoverySupport             RecoveryAction = "SUPPORT"
)

func RecoveryAllowed(reason ExceptionReason, action RecoveryAction, alternativeReceiverAuthorized bool) bool {
	if !reason.Valid() {
		return false
	}
	switch reason {
	case ReasonVendorNotReady:
		return action == RecoveryWait || action == RecoveryRetryPickup || action == RecoveryReschedule || action == RecoveryReplan
	case ReasonPackageMismatch, ReasonProhibitedItem, ReasonRestrictedItem:
		return action == RecoveryReturnVendor || action == RecoverySupport
	case ReasonDamagedPackage:
		return action == RecoveryReturnVendor || action == RecoverySupport || action == RecoveryReplan
	case ReasonReceiverUnavailable:
		return action == RecoveryWait || action == RecoveryRetryDelivery || action == RecoveryReschedule ||
			(action == RecoveryAlternativeReceiver && alternativeReceiverAuthorized) || action == RecoveryReturnSender || action == RecoveryReturnVendor
	case ReasonReceiverRefused:
		return action == RecoveryReturnSender || action == RecoveryReturnVendor || action == RecoverySupport
	case ReasonWrongAddress, ReasonAddressInaccessible:
		return action == RecoveryWait || action == RecoveryReschedule || action == RecoveryReplan || action == RecoverySupport
	case ReasonVehicleBreakdown, ReasonProviderIllness, ReasonSafetyIncident:
		return action == RecoveryProviderReplacement || action == RecoveryReturnVendor || action == RecoverySupport
	case ReasonWeatherRoad, ReasonSecurityRisk:
		return action == RecoveryWait || action == RecoveryReschedule || action == RecoveryReturnVendor || action == RecoveryReplan || action == RecoverySupport
	default:
		return false
	}
}

type ProductClass string

const (
	ProductPreparedFood ProductClass = "PREPARED_FOOD"
	ProductShopRetail   ProductClass = "SHOP_RETAIL"
)

type ReturnDisposition string

const (
	DispositionReturnVendor   ReturnDisposition = "RETURN_TO_VENDOR"
	DispositionFoodPolicy     ReturnDisposition = "FOOD_DISPOSAL_POLICY"
	DispositionShopInspection ReturnDisposition = "SHOP_INSPECTION"
	DispositionSupportReview  ReturnDisposition = "SUPPORT_REVIEW"
)

func ReturnDispositionAllowed(class ProductClass, disposition ReturnDisposition) bool {
	switch class {
	case ProductPreparedFood:
		return disposition == DispositionReturnVendor || disposition == DispositionFoodPolicy || disposition == DispositionSupportReview
	case ProductShopRetail:
		return disposition == DispositionReturnVendor || disposition == DispositionShopInspection || disposition == DispositionSupportReview
	default:
		return false
	}
}

type EvidenceCommand struct {
	OperationRef    string       `json:"operation_ref"`
	Type            EvidenceType `json:"evidence_type"`
	ParcelRefs      []string     `json:"parcel_refs"`
	ManifestRef     string       `json:"manifest_ref,omitempty"`
	ManifestVersion int64        `json:"manifest_version,omitempty"`
	PackageCount    int64        `json:"package_count,omitempty"`
	CapturedAt      time.Time    `json:"captured_at"`
	Latitude        *float64     `json:"latitude,omitempty"`
	Longitude       *float64     `json:"longitude,omitempty"`
	MediaRefs       []string     `json:"media_refs,omitempty"`
	Description     string       `json:"description,omitempty"`
	ReasonCode      string       `json:"reason_code,omitempty"`
}

func (c EvidenceCommand) Validate(now time.Time) error {
	if strings.TrimSpace(c.OperationRef) == "" || !c.Type.Valid() || len(c.ParcelRefs) == 0 || len(c.ParcelRefs) > 100 {
		return errors.New("invalid evidence identity")
	}
	if c.CapturedAt.IsZero() || c.CapturedAt.Location() == time.Local || c.CapturedAt.After(now.Add(5*time.Minute)) {
		return errors.New("invalid capture time")
	}
	if (c.Latitude == nil) != (c.Longitude == nil) {
		return errors.New("incomplete location")
	}
	if c.Latitude != nil && (*c.Latitude < -90 || *c.Latitude > 90 || *c.Longitude < -180 || *c.Longitude > 180) {
		return errors.New("invalid location")
	}
	if len(c.MediaRefs) > 12 || len(c.Description) > 2000 {
		return errors.New("evidence bounds exceeded")
	}
	seen := make(map[string]bool, len(c.ParcelRefs))
	for _, ref := range c.ParcelRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			return errors.New("invalid parcel references")
		}
		seen[ref] = true
	}
	for _, ref := range c.MediaRefs {
		if strings.TrimSpace(ref) == "" || strings.Contains(strings.ToLower(ref), "base64") || strings.HasPrefix(strings.ToLower(ref), "data:") {
			return errors.New("invalid media reference")
		}
	}
	if c.Type == EvidencePickup || c.Type == EvidenceProviderTransfer {
		if strings.TrimSpace(c.ManifestRef) == "" || c.ManifestVersion < 1 || c.PackageCount < 1 {
			return errors.New("manifest binding required")
		}
	}
	return nil
}

func canonicalRefs(values []string) []string {
	result := append([]string(nil), values...)
	for i := range result {
		result[i] = strings.TrimSpace(result[i])
	}
	sort.Strings(result)
	return result
}
