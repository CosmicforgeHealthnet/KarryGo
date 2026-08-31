package orders

import (
	"errors"
	"time"
)

type PickingState string

const (
	PickingPending    PickingState = "PENDING"
	PickingInProgress PickingState = "IN_PROGRESS"
	PickingComplete   PickingState = "COMPLETE"
	PickingBlocked    PickingState = "BLOCKED"
)

type Availability string

const (
	AvailabilityFull    Availability = "FULL_AVAILABLE"
	AvailabilityPartial Availability = "PARTIAL_AVAILABLE"
	AvailabilityOut     Availability = "OUT_OF_STOCK"
)

type ShopPickingLine struct {
	ParentOrderID, VendorOrderID, OrderLineID         string
	OrderedQuantity, QuantityFound, RemainingQuantity int64
	VariantRef, SellUnitRef                           string
	MeasurementMode                                   MeasurementMode
	EstimatedVariableQuantity                         int64
	SubstitutionPolicy                                SubstitutionPolicy
	QualityPreferences                                []QualityPreference
	PreselectedReplacementRef                         string
	State                                             PickingState
	ExpectedVendorOrderVersion                        int64
	Version                                           int64
	UpdatedAt                                         time.Time
}

func (p ShopPickingLine) Validate() error {
	if p.ParentOrderID == "" || p.VendorOrderID == "" || p.OrderLineID == "" || p.OrderedQuantity <= 0 {
		return errors.New("picking line identity and ordered quantity are required")
	}
	if p.QuantityFound < 0 || p.QuantityFound > p.OrderedQuantity {
		return errors.New("picked quantity is outside ordered quantity")
	}
	if p.RemainingQuantity != p.OrderedQuantity-p.QuantityFound {
		return errors.New("remaining quantity is inconsistent")
	}
	if !p.SubstitutionPolicy.Valid() {
		return errors.New("invalid frozen substitution policy")
	}
	if err := ValidateQualityPreferences(p.QualityPreferences); err != nil {
		return err
	}
	if p.ExpectedVendorOrderVersion < 1 || p.Version < 1 {
		return errors.New("picking versions must be positive")
	}
	return nil
}

func (p ShopPickingLine) RecordAvailability(found int64) (ShopPickingLine, error) {
	if found < 0 || found > p.OrderedQuantity {
		return p, errors.New("picked quantity is outside ordered quantity")
	}
	p.QuantityFound = found
	p.RemainingQuantity = p.OrderedQuantity - found
	if found == p.OrderedQuantity {
		p.State = PickingComplete
	} else {
		p.State = PickingBlocked
	}
	return p, nil
}

func (p ShopPickingLine) PackingBlocked(unresolvedSubstitution, unresolvedQuality, unresolvedMeasurement, inventoryFinalized, financialFinalized bool) bool {
	return unresolvedSubstitution || unresolvedQuality || unresolvedMeasurement || !inventoryFinalized || !financialFinalized || p.State != PickingComplete
}
