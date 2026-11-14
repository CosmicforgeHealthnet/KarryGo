// Package models holds the Section 18 fuel and energy domain: fuel records
// and the rule that pairs each fuel or energy type with its units. Fuel is
// organization reporting data with no wallet or accounting dependency.
package models

import "time"

type FuelRecord struct {
	ID                   string    `json:"id"`
	OrganizationID       string    `json:"organizationId"`
	AssetID              string    `json:"assetId"`
	ProviderMembershipID string    `json:"providerMembershipId"`
	FuelOrEnergyType     string    `json:"fuelOrEnergyType"`
	Unit                 string    `json:"unit"`
	ReceiptMediaID       string    `json:"receiptMediaId"`
	SupplierReference    string    `json:"supplierReference"`
	ApprovalStatus       string    `json:"approvalStatus"`
	ApprovedBy           string    `json:"approvedBy"`
	Quantity             float64   `json:"quantity"`
	Odometer             *float64  `json:"odometer"`
	OperatingHours       *float64  `json:"operatingHours"`
	RecordedAt           time.Time `json:"recordedAt"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
	Version              int64     `json:"version"`
}

// Page selects one keyset page of fuel records, newest recordedAt first.
type Page struct {
	Limit   int
	After   *time.Time
	AfterID string
}

// ValidUnits pairs each fuel or energy type with its accepted units.
var ValidUnits = map[string]map[string]bool{"PETROL": {"L": true, "LITRE": true}, "DIESEL": {"L": true, "LITRE": true}, "ELECTRIC": {"KWH": true}, "GAS": {"KG": true, "L": true}, "OTHER": {"UNIT": true}}

// ValidQuantity reports whether quantity, type and unit form a valid record
// (type and unit already upper-cased).
func ValidQuantity(quantity float64, fuelType, unit string) bool {
	return quantity > 0 && ValidUnits[fuelType][unit]
}
