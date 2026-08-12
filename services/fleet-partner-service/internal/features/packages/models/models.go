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
	ID, OrganizationID, PackageID, PreviousStatus, NewStatus string
	ReasonCode, SafeNote, ActorType, ActorReference, Source  string
	RequestID, CorrelationID                                 string
	OccurredAt, CreatedAt                                    time.Time
}

type Scan struct {
	ID, OrganizationID, PackageID, ScanType, BarcodeValue, QRValue       string
	LocationReference, DeviceReference, ClientScanID, RequestFingerprint string
	Disposition, ActorType, ActorReference                               string
	OccurredAt, ReceivedAt                                               time.Time
}

type CustodyEvent struct {
	ID, OrganizationID, PackageID, ShipmentID, ManifestID                  string
	FromActorType, FromActorReference, ToActorType, ToActorReference       string
	FromLocationReference, ToLocationReference, Condition, EvidenceMediaID string
	CorrectionOfEventID, Source, CorrelationID                             string
	OccurredAt, ReceivedAt, CreatedAt                                      time.Time
}
