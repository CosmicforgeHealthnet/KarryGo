package orders

import (
	"errors"
	"math"
)

// UpperMeasuredQuantity derives a physical reservation ceiling from the
// frozen expected quantity, ordered count, and overfill tolerance. It uses
// integer ceiling arithmetic so stock is never under-reserved.
func UpperMeasuredQuantity(estimated, ordered int64, overfillBps int) (int64, error) {
	if estimated <= 0 || ordered <= 0 || overfillBps < 0 || overfillBps > 10000 {
		return 0, errors.New("invalid upper measurement bound")
	}
	if estimated > math.MaxInt64/ordered {
		return 0, errors.New("measurement quantity overflow")
	}
	base := estimated * ordered
	if base > math.MaxInt64/int64(10000+overfillBps) {
		return 0, errors.New("measurement quantity overflow")
	}
	return (base*int64(10000+overfillBps) + 9999) / 10000, nil
}

type MeasurementMode string

const (
	MeasurementFixedUnit      MeasurementMode = "FIXED_UNIT"
	MeasurementVariableWeight MeasurementMode = "VARIABLE_WEIGHT"
	MeasurementVariableCount  MeasurementMode = "VARIABLE_COUNT"
)

type FinancialFinalizationState string

const (
	FinancialNotRequired        FinancialFinalizationState = "NOT_REQUIRED"
	FinancialEstimated          FinancialFinalizationState = "ESTIMATED"
	FinancialActualRecorded     FinancialFinalizationState = "ACTUAL_RECORDED"
	FinancialPending            FinancialFinalizationState = "FINANCIAL_PENDING"
	FinancialReapprovalRequired FinancialFinalizationState = "REAPPROVAL_REQUIRED"
	FinancialFinalized          FinancialFinalizationState = "FINALIZED"
	FinancialRefundPending      FinancialFinalizationState = "REFUND_PENDING"
	FinancialFailedRecoverable  FinancialFinalizationState = "FAILED_RECOVERABLE"
)

type VariableMeasurement struct {
	Mode                  MeasurementMode            `json:"mode"`
	EstimatedQuantity     int64                      `json:"estimated_quantity"`
	UnitPriceMinor        int64                      `json:"unit_price_minor"`
	EstimatedAmountMinor  int64                      `json:"estimated_amount_minor"`
	ToleranceQuantity     int64                      `json:"tolerance_quantity"`
	HardMaxQuantity       int64                      `json:"hard_max_quantity"`
	ReservedUpperQuantity int64                      `json:"reserved_upper_quantity"`
	ActualQuantity        int64                      `json:"actual_quantity"`
	FinalAmountMinor      int64                      `json:"final_amount_minor"`
	State                 FinancialFinalizationState `json:"state"`
}

type MeasurementConfiguration struct {
	Mode              MeasurementMode
	Basis             string
	Eligible          bool
	MinimumQuantity   int64
	MaximumQuantity   int64
	ToleranceQuantity int64
	CountAllowed      bool
}

func (c MeasurementConfiguration) Validate() error {
	if !c.Eligible || c.Basis == "" || c.MinimumQuantity <= 0 || c.MaximumQuantity < c.MinimumQuantity || c.ToleranceQuantity < 0 {
		return errors.New("measurement configuration is not eligible")
	}
	switch c.Mode {
	case MeasurementVariableWeight:
		if c.Basis != "MASS" {
			return errors.New("variable weight requires mass basis")
		}
	case MeasurementVariableCount:
		if c.Basis != "COUNT" || !c.CountAllowed {
			return errors.New("variable count is not enabled for this category")
		}
	default:
		return errors.New("unsupported variable measurement mode")
	}
	return nil
}

func (m VariableMeasurement) Validate() error {
	if m.EstimatedQuantity <= 0 || m.UnitPriceMinor < 0 || m.HardMaxQuantity < m.EstimatedQuantity || m.ReservedUpperQuantity < m.EstimatedQuantity {
		return errors.New("invalid variable measurement bounds")
	}
	if m.ToleranceQuantity < 0 || m.ReservedUpperQuantity > m.HardMaxQuantity {
		return errors.New("invalid variable measurement tolerance")
	}
	switch m.Mode {
	case MeasurementVariableWeight, MeasurementVariableCount:
	default:
		return errors.New("measurement mode is not variable")
	}
	return nil
}

func CalculateMeasuredAmount(unitPriceMinor, actualQuantity int64) (int64, error) {
	if unitPriceMinor < 0 || actualQuantity < 0 || actualQuantity != 0 && unitPriceMinor > math.MaxInt64/actualQuantity {
		return 0, errors.New("measurement amount overflow")
	}
	return unitPriceMinor * actualQuantity, nil
}

// MultiplyRatio computes floor(numerator*price/denominator) with an explicit
// overflow check. Quantities are integer shop units; there is no float money.
func MultiplyRatio(priceMinor, numerator, denominator int64) (int64, error) {
	if priceMinor < 0 || numerator < 0 || denominator <= 0 || numerator != 0 && priceMinor > math.MaxInt64/numerator {
		return 0, errors.New("ratio amount overflow")
	}
	return priceMinor * numerator / denominator, nil
}

func (m VariableMeasurement) RecordActual(actual int64) (VariableMeasurement, error) {
	if err := m.Validate(); err != nil {
		return m, err
	}
	if actual < 0 || actual > m.ReservedUpperQuantity || actual > m.HardMaxQuantity {
		return m, errors.New("actual quantity exceeds reserved or hard maximum")
	}
	amount, err := CalculateMeasuredAmount(m.UnitPriceMinor, actual)
	if err != nil {
		return m, err
	}
	m.ActualQuantity, m.FinalAmountMinor, m.State = actual, amount, FinancialActualRecorded
	return m, nil
}

func (m VariableMeasurement) Finalize(authorizedMax int64) (VariableMeasurement, error) {
	if m.State != FinancialActualRecorded && m.State != FinancialPending {
		return m, errors.New("measurement is not ready for finalization")
	}
	if m.FinalAmountMinor < 0 || m.FinalAmountMinor > authorizedMax {
		m.State = FinancialReapprovalRequired
		return m, errors.New("measured amount exceeds authorized cap")
	}
	m.State = FinancialFinalized
	return m, nil
}

func ParentCapAllows(parentMax int64, fixedFinalized, variableFinalized, adjustments, fees, taxes, discounts int64) bool {
	if parentMax < 0 || fixedFinalized < 0 || variableFinalized < 0 || adjustments < 0 || fees < 0 || taxes < 0 || discounts < 0 {
		return false
	}
	values := []int64{fixedFinalized, variableFinalized, adjustments, fees, taxes}
	total := int64(0)
	for _, v := range values {
		if total > math.MaxInt64-v {
			return false
		}
		total += v
	}
	if discounts > total {
		return false
	}
	return total-discounts <= parentMax
}

func ReplacementInventoryCommit(originalFound, replacementFound, originalOrdered, missing int64) (int64, int64, error) {
	if originalFound < 0 || replacementFound < 0 || originalOrdered < 0 || missing < 0 || originalFound > originalOrdered || missing > originalOrdered-originalFound {
		return 0, 0, errors.New("invalid replacement quantities")
	}
	if replacementFound > missing {
		replacementFound = missing
	}
	return originalFound, replacementFound, nil
}
