// Package models holds the Section 15 service catalogue domain.
//
// A catalogue item is a definition an operator's own commercial and
// operational systems point at; it is never a price, a fare formula or a
// commission. Nothing here imports gin, pgx or an HTTP client, and the status
// machine is the point of the package: a catalogue item can only ever reach a
// status this file admits a transition to.
package models

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ErrInvalidTransition is returned when a status change is not in the matrix.
var ErrInvalidTransition = errors.New("servicecatalog: transition is not permitted")

// JSONMap is an untyped jsonb column.
type JSONMap map[string]any

// Clone returns a defensive copy so a stored value cannot be edited through
// the map that produced it.
func (m JSONMap) Clone() JSONMap {
	if m == nil {
		return nil
	}
	out := make(JSONMap, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Categories. TAXI and PASSENGER_TRANSPORT are valid catalogue categories —
// an organization may describe the capability — but Activate refuses them
// unconditionally: Taxi execution is fail-closed platform-wide, and a
// catalogue item is not allowed to imply otherwise.
const (
	CategoryPackageDelivery    = "PACKAGE_DELIVERY"
	CategoryCourierLastMile    = "COURIER_LAST_MILE"
	CategorySameDayDelivery    = "SAME_DAY_DELIVERY"
	CategoryScheduledDelivery  = "SCHEDULED_DELIVERY"
	CategoryTaxi               = "TAXI"
	CategoryPassengerTransport = "PASSENGER_TRANSPORT"
	CategoryHauling            = "HAULING"
	CategoryTrucking           = "TRUCKING"
	CategoryHeavyHaul          = "HEAVY_HAUL"
	CategoryFreightForwarding  = "FREIGHT_FORWARDING"
	CategoryMovingRelocation   = "MOVING_RELOCATION"
	CategoryColdChain          = "COLD_CHAIN"
	CategoryMedicalLogistics   = "MEDICAL_LOGISTICS"
	CategoryAgricultureFood    = "AGRICULTURE_FOOD"
	CategoryTowingRecovery     = "TOWING_RECOVERY"
	CategoryWarehousing        = "WAREHOUSING"
	CategoryFulfilment         = "FULFILMENT"
	CategoryPrivateLogistics   = "PRIVATE_LOGISTICS"
	CategoryOther              = "OTHER"
)

var validCategories = map[string]bool{
	CategoryPackageDelivery: true, CategoryCourierLastMile: true, CategorySameDayDelivery: true,
	CategoryScheduledDelivery: true, CategoryTaxi: true, CategoryPassengerTransport: true,
	CategoryHauling: true, CategoryTrucking: true, CategoryHeavyHaul: true,
	CategoryFreightForwarding: true, CategoryMovingRelocation: true, CategoryColdChain: true,
	CategoryMedicalLogistics: true, CategoryAgricultureFood: true, CategoryTowingRecovery: true,
	CategoryWarehousing: true, CategoryFulfilment: true, CategoryPrivateLogistics: true,
	CategoryOther: true,
}

// ValidCategory reports whether the category is a recognized value.
func ValidCategory(value string) bool { return validCategories[value] }

// IsTaxiCategory reports whether a category belongs to the platform-wide
// fail-closed Taxi capability, regardless of any organization capability flag.
func IsTaxiCategory(category string) bool {
	return category == CategoryTaxi || category == CategoryPassengerTransport
}

// Statuses.
const (
	StatusDraft    = "DRAFT"
	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"
	StatusArchived = "ARCHIVED"
)

// catalogTransitions is the whole catalogue item status matrix. ARCHIVED has
// no outgoing edges: an archived item is closed, not paused, and its code
// stays permanently reserved (enforced by the database unique index).
var catalogTransitions = map[string]map[string]bool{
	StatusDraft:    {StatusActive: true, StatusArchived: true},
	StatusActive:   {StatusInactive: true, StatusArchived: true},
	StatusInactive: {StatusActive: true, StatusArchived: true},
	StatusArchived: {},
}

// CanTransition reports whether a catalogue item status change is permitted
// by the state machine alone. Category-specific fail-closed rules (Taxi) are
// enforced separately by the usecase, not here, because they are a business
// policy over an otherwise-valid transition rather than a shape of the
// machine itself.
func CanTransition(from, to string) bool { return catalogTransitions[from][to] }

// Terminal reports whether the status admits no further movement.
func Terminal(status string) bool { return status == StatusArchived }

var codeNonAllowed = regexp.MustCompile(`[^A-Z0-9_]+`)
var codeWhitespace = regexp.MustCompile(`\s+`)

// NormalizeCode upper-cases and collapses a caller-supplied code into the
// stored form: internal whitespace becomes an underscore, and any character
// outside [A-Z0-9_] is dropped. Two operators typing "Same Day" and
// "same-day " must land on the same reserved code, not two similar ones.
func NormalizeCode(raw string) string {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	collapsed := codeWhitespace.ReplaceAllString(upper, "_")
	return codeNonAllowed.ReplaceAllString(collapsed, "")
}

// CatalogItem is an organization-owned service catalogue definition.
type CatalogItem struct {
	ID                      string
	OrganizationID          string
	BranchID                string
	Name                    string
	Code                    string
	Category                string
	Description             string
	CapabilityReference     string
	RequiredFieldsSchema    JSONMap
	SupportedVehicleClasses []string
	SupportedPackageTypes   []string
	PricingPolicyReference  string
	ApprovalPolicyReference string
	ProofPolicyReference    string
	Status                  string
	Version                 int64
	CreatedBy               string
	UpdatedBy               string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	ArchivedAt              *time.Time
}

// Snapshot returns the immutable projection an order freezes at confirmation.
//
// Only fields an executing order actually needs are captured — never the
// pricing/approval/proof policy references, which stay live pointers an order
// resolves through its own confirmation path, not through a frozen copy.
func (c CatalogItem) Snapshot() JSONMap {
	return JSONMap{
		"id":                      c.ID,
		"name":                    c.Name,
		"code":                    c.Code,
		"category":                c.Category,
		"requiredFieldsSchema":    map[string]any(c.RequiredFieldsSchema),
		"supportedVehicleClasses": c.SupportedVehicleClasses,
		"supportedPackageTypes":   c.SupportedPackageTypes,
		"version":                 c.Version,
	}
}

// ---------------------------------------------------------------------------
// Required-fields schema validation
// ---------------------------------------------------------------------------
//
// The schema is declarative only: a bounded set of field descriptors, never
// an executable expression. Unsupported keywords are rejected rather than
// silently ignored, and depth, field count, enum size and payload size are
// all bounded so a schema can never become a way to smuggle a large or
// deeply nested structure through this API.

const (
	maxSchemaFields    = 50
	maxEnumValues      = 50
	maxDescriptorDepth = 1
)

var allowedFieldTypes = map[string]bool{
	"string": true, "number": true, "boolean": true, "date": true, "enum": true,
}

var allowedDescriptorKeys = map[string]bool{
	"type": true, "required": true, "enum": true, "maxLength": true, "description": true,
}

// ValidateRequiredFieldsSchema enforces the allowlisted, bounded shape.
func ValidateRequiredFieldsSchema(schema JSONMap) error {
	if len(schema) > maxSchemaFields {
		return fmt.Errorf("servicecatalog: required fields schema exceeds %d fields", maxSchemaFields)
	}
	for field, raw := range schema {
		if field == "" {
			return errors.New("servicecatalog: required fields schema has an empty field name")
		}
		descriptor, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("servicecatalog: field %q descriptor must be an object", field)
		}
		if err := validateDescriptor(field, descriptor); err != nil {
			return err
		}
	}
	return nil
}

func validateDescriptor(field string, descriptor map[string]any) error {
	for key, value := range descriptor {
		if !allowedDescriptorKeys[key] {
			return fmt.Errorf("servicecatalog: field %q uses unsupported keyword %q", field, key)
		}
		switch key {
		case "type":
			typeValue, ok := value.(string)
			if !ok || !allowedFieldTypes[typeValue] {
				return fmt.Errorf("servicecatalog: field %q has an unsupported type", field)
			}
		case "required":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("servicecatalog: field %q required flag must be boolean", field)
			}
		case "enum":
			values, ok := value.([]any)
			if !ok {
				return fmt.Errorf("servicecatalog: field %q enum must be a list", field)
			}
			if len(values) > maxEnumValues {
				return fmt.Errorf("servicecatalog: field %q enum exceeds %d values", field, maxEnumValues)
			}
			for _, entry := range values {
				if _, ok := entry.(string); !ok {
					return fmt.Errorf("servicecatalog: field %q enum values must be strings", field)
				}
			}
		case "maxLength":
			switch n := value.(type) {
			case float64:
				if n < 0 {
					return fmt.Errorf("servicecatalog: field %q maxLength must be non-negative", field)
				}
			default:
				return fmt.Errorf("servicecatalog: field %q maxLength must be a number", field)
			}
		case "description":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("servicecatalog: field %q description must be a string", field)
			}
		}
	}
	if _, hasType := descriptor["type"]; !hasType {
		return fmt.Errorf("servicecatalog: field %q is missing a type", field)
	}
	return nil
}
