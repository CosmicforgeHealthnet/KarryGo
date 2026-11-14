// Package models defines Section 15 package, scan and custody records.
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"time"
)

const (
	StatusCreated        = "CREATED"
	StatusReceived       = "RECEIVED"
	StatusSorted         = "SORTED"
	StatusLoaded         = "LOADED"
	StatusInTransit      = "IN_TRANSIT"
	StatusOutForDelivery = "OUT_FOR_DELIVERY"
	StatusDelivered      = "DELIVERED"
	StatusFailedAttempt  = "FAILED_ATTEMPT"
	StatusReturning      = "RETURNING"
	StatusReturned       = "RETURNED"
	StatusDamaged        = "DAMAGED"
	StatusLost           = "LOST"
	StatusQuarantined    = "QUARANTINED"
)

var transitions = map[string]map[string]bool{
	StatusCreated:        {StatusReceived: true, StatusDamaged: true, StatusLost: true, StatusQuarantined: true},
	StatusReceived:       {StatusSorted: true, StatusDamaged: true, StatusLost: true, StatusQuarantined: true},
	StatusSorted:         {StatusLoaded: true, StatusDamaged: true, StatusLost: true, StatusQuarantined: true},
	StatusLoaded:         {StatusInTransit: true, StatusDamaged: true, StatusLost: true, StatusQuarantined: true},
	StatusInTransit:      {StatusOutForDelivery: true, StatusFailedAttempt: true, StatusDamaged: true, StatusLost: true, StatusQuarantined: true},
	StatusOutForDelivery: {StatusDelivered: true, StatusFailedAttempt: true, StatusDamaged: true, StatusLost: true, StatusQuarantined: true},
	StatusFailedAttempt:  {StatusReturning: true, StatusOutForDelivery: true, StatusDamaged: true, StatusLost: true, StatusQuarantined: true},
	StatusReturning:      {StatusReturned: true, StatusDamaged: true, StatusLost: true, StatusQuarantined: true},
	StatusReturned:       {}, StatusDelivered: {}, StatusDamaged: {}, StatusLost: {}, StatusQuarantined: {},
}

func CanTransition(from, to string) bool { return transitions[from][to] }
func Exceptional(status string) bool {
	return status == StatusDamaged || status == StatusLost || status == StatusQuarantined
}

var decimalPattern = regexp.MustCompile(`^[0-9]{1,9}(\.[0-9]{1,3})?$`)

func ValidPositiveDecimal(v string) bool {
	return decimalPattern.MatchString(v) && v != "0" && v != "0.0" && v != "0.00" && v != "0.000"
}

func ScanFingerprint(packageID, scanType, barcode, qr, location, device, clientID string, occurred time.Time) string {
	raw := packageID + "\x00" + scanType + "\x00" + barcode + "\x00" + qr + "\x00" + location + "\x00" + device + "\x00" + clientID + "\x00" + occurred.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type Package struct {
	ID, OrganizationID, BranchID, ShipmentID                       string
	PackageCode, BarcodeValue, QRValue, PackageType                string
	Length, Width, Height, DimensionUnit                           string
	Weight, WeightUnit                                             string
	DeclaredValueAmountMinor                                       int64
	DeclaredValueCurrency, HandlingInstructions, Condition, Status string
	Version                                                        int64
	CreatedBy, UpdatedBy                                           string
	CreatedAt, UpdatedAt                                           time.Time
	ArchivedAt                                                     *time.Time
}

type History struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	PackageID      string    `json:"packageId"`
	PreviousStatus string    `json:"previousStatus"`
	NewStatus      string    `json:"newStatus"`
	ReasonCode     string    `json:"reasonCode"`
	SafeNote       string    `json:"safeNote"`
	ActorType      string    `json:"actorType"`
	ActorReference string    `json:"actorReference"`
	Source         string    `json:"source"`
	RequestID      string    `json:"requestId"`
	CorrelationID  string    `json:"correlationId"`
	OccurredAt     time.Time `json:"occurredAt"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Scan struct {
	ID                 string    `json:"id"`
	OrganizationID     string    `json:"organizationId"`
	PackageID          string    `json:"packageId"`
	ScanType           string    `json:"scanType"`
	BarcodeValue       string    `json:"barcodeValue"`
	QRValue            string    `json:"qrValue"`
	LocationReference  string    `json:"locationReference"`
	DeviceReference    string    `json:"deviceReference"`
	ClientScanID       string    `json:"clientScanId"`
	RequestFingerprint string    `json:"requestFingerprint"`
	Disposition        string    `json:"disposition"`
	ActorType          string    `json:"actorType"`
	ActorReference     string    `json:"actorReference"`
	OccurredAt         time.Time `json:"occurredAt"`
	ReceivedAt         time.Time `json:"receivedAt"`
}

type CustodyEvent struct {
	ID                    string    `json:"id"`
	OrganizationID        string    `json:"organizationId"`
	PackageID             string    `json:"packageId"`
	ShipmentID            string    `json:"shipmentId"`
	ManifestID            string    `json:"manifestId"`
	FromActorType         string    `json:"fromActorType"`
	FromActorReference    string    `json:"fromActorReference"`
	ToActorType           string    `json:"toActorType"`
	ToActorReference      string    `json:"toActorReference"`
	FromLocationReference string    `json:"fromLocationReference"`
	ToLocationReference   string    `json:"toLocationReference"`
	Condition             string    `json:"condition"`
	EvidenceMediaID       string    `json:"evidenceMediaId"`
	CorrectionOfEventID   string    `json:"correctionOfEventId"`
	Source                string    `json:"source"`
	CorrelationID         string    `json:"correlationId"`
	OccurredAt            time.Time `json:"occurredAt"`
	ReceivedAt            time.Time `json:"receivedAt"`
	CreatedAt             time.Time `json:"createdAt"`
	// SubjectType and SubjectReference name what changed hands. A package
	// event reports PACKAGE and its package ID; the generic chain (Section
	// 19.12) also covers shipments, orders, assets and specialized records.
	SubjectType      string   `json:"subjectType"`
	SubjectReference string   `json:"subjectReference"`
	EvidenceMediaIDs []string `json:"evidenceMediaIds"`
}

// Custody subject types accepted by the generic chain of custody.
const (
	CustodySubjectPackage           = "PACKAGE"
	CustodySubjectShipment          = "SHIPMENT"
	CustodySubjectOrder             = "ORDER"
	CustodySubjectAsset             = "ASSET"
	CustodySubjectSpecializedRecord = "SPECIALIZED_RECORD"
)

// ValidCustodySubject reports whether a subject type may carry custody events.
func ValidCustodySubject(v string) bool {
	switch v {
	case CustodySubjectPackage, CustodySubjectShipment, CustodySubjectOrder, CustodySubjectAsset, CustodySubjectSpecializedRecord:
		return true
	}
	return false
}
