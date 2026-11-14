// Package models holds the Section 20 specialized-logistics domain: module
// catalogue, typed extension schemas, records and the pure validation rules.
package models

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ErrNotFound is returned by the repository when a scoped row is absent.
var ErrNotFound = errors.New("specialized: not found")

// JobSpecializedSweep is the Section 20 maintenance job type.
const JobSpecializedSweep = "section20.specialized_sweep"

const (
	Courier      = "COURIER_LAST_MILE"
	Taxi         = "TAXI_PASSENGER"
	Hauling      = "TRUCKING_HAULAGE"
	Heavy        = "HEAVY_SPECIALIZED"
	Freight      = "FREIGHT_FORWARDING"
	ColdChain    = "COLD_CHAIN"
	Medical      = "MEDICAL_SENSITIVE"
	Agricultural = "AGRICULTURAL_FOOD"
	Moving       = "MOVING_RELOCATION"
	Towing       = "TOWING_RECOVERY"
)

// SubjectOrder is the subject type of order-level specialized data
// (spec 50.3 /orders/{id}/specialized-data).
const SubjectOrder = "ORDER"

type Flags struct {
	Enabled, Workers, KillSwitch bool
	Modules                      map[string]bool
}
type Capability struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	CapabilityCode string          `json:"capabilityCode"`
	Status         string          `json:"status"`
	Configuration  json.RawMessage `json:"configuration"`
	EnabledAt      *time.Time      `json:"enabledAt"`
	DisabledAt     *time.Time      `json:"disabledAt"`
	Version        int64           `json:"version"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}
type Record struct {
	ID                         string          `json:"id"`
	OrganizationID             string          `json:"organizationId"`
	BranchID                   string          `json:"branchId"`
	CapabilityCode             string          `json:"capabilityCode"`
	RecordType                 string          `json:"recordType"`
	SubjectType                string          `json:"subjectType"`
	SubjectReference           string          `json:"subjectReference"`
	Status                     string          `json:"status"`
	RestrictedVisibility       bool            `json:"restrictedVisibility"`
	Payload                    json.RawMessage `json:"payload"`
	ExternalFinancialReference string          `json:"externalFinancialReference"`
	SourceAuthority            string          `json:"sourceAuthority"`
	SourceReference            string          `json:"sourceReference"`
	IdempotencyKey             string          `json:"idempotencyKey"`
	Version                    int64           `json:"version"`
	CreatedBy                  string          `json:"createdBy"`
	CreatedAt                  time.Time       `json:"createdAt"`
	UpdatedAt                  time.Time       `json:"updatedAt"`
	CompletedAt                *time.Time      `json:"completedAt"`
	// SchemaVersion is the extension schema version the payload was
	// validated against; SchemaID names it (e.g. COLD_CHAIN.v2).
	SchemaVersion int    `json:"schemaVersion"`
	SchemaID      string `json:"schemaId"`
}
type Milestone struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	RecordID       string          `json:"recordId"`
	MilestoneType  string          `json:"milestoneType"`
	Status         string          `json:"status"`
	SafeMetadata   json.RawMessage `json:"safeMetadata"`
	SourceTime     time.Time       `json:"sourceTime"`
	ReceivedAt     time.Time       `json:"receivedAt"`
	ActorReference string          `json:"actorReference"`
	CorrelationID  string          `json:"correlationId"`
}
type TemperatureEvent struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organizationId"`
	RecordID        string          `json:"recordId"`
	SensorReference string          `json:"sensorReference"`
	Temperature     float64         `json:"temperature"`
	Unit            string          `json:"unit"`
	SourceVersion   int64           `json:"sourceVersion"`
	SourceTime      time.Time       `json:"sourceTime"`
	ReceivedAt      time.Time       `json:"receivedAt"`
	Excursion       bool            `json:"excursion"`
	SafeMetadata    json.RawMessage `json:"safeMetadata"`
}

// OrderDataInput attaches one capability record to a common order.
type OrderDataInput struct {
	CapabilityCode, RecordType string
	SchemaVersion              int
	Payload                    []byte
	IdempotencyKey             string
}

// RedactedPayload replaces a restricted record's payload in list views. The
// full payload is only returned by the single-record read, which writes a
// sensitive-access audit row.
var RedactedPayload = json.RawMessage(`{"redacted":true}`)

// Redact hides the payload of a restricted record.
func Redact(v Record) Record {
	if v.RestrictedVisibility {
		v.Payload = RedactedPayload
	}
	return v
}

// TemperatureThresholds reads the requirement's bounds. Both bounds must be
// present and numeric (a JSON number or a numeric string), and the minimum
// may not exceed the maximum.
func TemperatureThresholds(payload json.RawMessage) (float64, float64, string, error) {
	var p map[string]any
	if json.Unmarshal(payload, &p) != nil {
		return 0, 0, "", apperrors.Validation("The temperature requirement is unreadable.", nil)
	}
	unit, _ := p["unit"].(string)
	minimum, minOK := thresholdNumber(p["minimumTemperature"])
	maximum, maxOK := thresholdNumber(p["maximumTemperature"])
	var fields []apperrors.FieldViolation
	if !minOK {
		fields = append(fields, apperrors.FieldViolation{Field: "minimumTemperature", Message: "The requirement record has no numeric minimum temperature."})
	}
	if !maxOK {
		fields = append(fields, apperrors.FieldViolation{Field: "maximumTemperature", Message: "The requirement record has no numeric maximum temperature."})
	}
	if len(fields) > 0 {
		return 0, 0, "", apperrors.Validation("The temperature requirement has no usable thresholds; update the record before recording readings.", fields)
	}
	if minimum > maximum {
		return 0, 0, "", apperrors.Validation("The temperature requirement's minimum exceeds its maximum.", []apperrors.FieldViolation{{Field: "minimumTemperature", Message: "Must not exceed maximumTemperature."}})
	}
	return minimum, maximum, unit, nil
}

func thresholdNumber(raw any) (float64, bool) {
	var f float64
	switch x := raw.(type) {
	case float64:
		f = x
	case json.Number:
		var err error
		if f, err = x.Float64(); err != nil {
			return 0, false
		}
	case string:
		var err error
		if f, err = strconv.ParseFloat(strings.TrimSpace(x), 64); err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}
