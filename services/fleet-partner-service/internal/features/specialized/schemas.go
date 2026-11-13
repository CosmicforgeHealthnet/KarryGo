package specialized

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Capability extension schemas (spec 50.2, 50.3 GET /capability-schemas/{code}).
//
// Every capability shares one extension store (fleet_partner_specialized_records)
// and one validation framework; a capability contributes only its schema here.
// Version 1 is the original presence-only contract and is kept so records
// written before version 2 still validate on update (backward-compatible
// extension version). Version 2 is typed and closed: unknown fields are
// rejected rather than silently stored.
const (
	SchemaVersionLegacy  = 1
	SchemaVersionCurrent = 2
)

// FieldKind is the type of one extension field.
type FieldKind string

const (
	KindString        FieldKind = "string"
	KindInteger       FieldKind = "integer"
	KindNumber        FieldKind = "number"
	KindBoolean       FieldKind = "boolean"
	KindTimestamp     FieldKind = "timestamp"
	KindCountry       FieldKind = "countryCode"
	KindStringArray   FieldKind = "stringArray"
	KindObject        FieldKind = "object"
	KindObjectArray   FieldKind = "objectArray"
	KindMediaRef      FieldKind = "mediaFileReference"
	KindAssetRef      FieldKind = "assetReference"
	KindQualification FieldKind = "qualificationReferences"
)

// Field is one typed extension field.
type Field struct {
	Name     string    `json:"name"`
	Kind     FieldKind `json:"kind"`
	Required bool      `json:"required"`
	Enum     []string  `json:"enum,omitempty"`
	Minimum  *float64  `json:"minimum,omitempty"`
	Fields   []Field   `json:"fields,omitempty"`
}

// RecordSchema is the closed schema of one record type.
type RecordSchema struct {
	RecordType           string   `json:"recordType"`
	Fields               []Field  `json:"fields"`
	AdditionalProperties bool     `json:"additionalProperties"`
	Rules                []string `json:"rules,omitempty"`
}

// CapabilitySchema describes one capability's extension contract.
type CapabilitySchema struct {
	CapabilityCode       string         `json:"capabilityCode"`
	SchemaID             string         `json:"schemaId"`
	Version              int            `json:"version"`
	SupportedVersions    []int          `json:"supportedVersions"`
	RestrictedVisibility bool           `json:"restrictedVisibility"`
	ExecutionAdapter     string         `json:"executionAdapter"`
	RequiredProofTypes   []string       `json:"requiredProofTypes"`
	FinancialValues      string         `json:"financialValues"`
	RecordTypes          []RecordSchema `json:"recordTypes"`
}

// SchemaID names a capability schema version, e.g. COLD_CHAIN.v2.
func SchemaID(code string, version int) string { return fmt.Sprintf("%s.v%d", code, version) }

func f(name string, kind FieldKind) Field      { return Field{Name: name, Kind: kind} }
func req(name string, kind FieldKind) Field    { return Field{Name: name, Kind: kind, Required: true} }
func enum(field Field, values ...string) Field { field.Enum = values; return field }
func atLeast(field Field, min float64) Field   { field.Minimum = &min; return field }
func object(field Field, fields ...Field) Field {
	field.Fields = fields
	return field
}

var notes = f("notes", KindString)

func record(t string, fields ...Field) RecordSchema {
	return RecordSchema{RecordType: t, Fields: append(fields, notes)}
}

// capabilityMeta is the declarative, non-schema part of each capability.
var capabilityMeta = map[string]struct {
	restricted bool
	adapter    string
	proof      []string
}{
	Courier:      {false, "driver-dispatch-delivery-service", []string{"PACKAGE_SCAN", "SIGNATURE", "PHOTO"}},
	Taxi:         {true, "CORPORATE_INTERNAL_RECORDS_ONLY", []string{"TIMESTAMP", "LOCATION"}},
	Hauling:      {false, "driver-hauling-service", []string{"DOCUMENT_SCAN", "SIGNATURE", "SEAL"}},
	Heavy:        {false, "driver-hauling-service", []string{"DOCUMENT_SCAN", "PHOTO", "SIGNATURE"}},
	Freight:      {false, "CORPORATE_INTERNAL", []string{"DOCUMENT_SCAN", "SEAL"}},
	ColdChain:    {false, "CORPORATE_INTERNAL", []string{"TEMPERATURE_READING", "SEAL"}},
	Medical:      {true, "CORPORATE_INTERNAL", []string{"SIGNATURE", "IDENTITY_REFERENCE"}},
	Agricultural: {false, "CORPORATE_INTERNAL", []string{"TEMPERATURE_READING", "PHOTO"}},
	Moving:       {false, "CORPORATE_INTERNAL", []string{"PHOTO", "SIGNATURE"}},
	Towing:       {false, "CORPORATE_INTERNAL", []string{"PHOTO", "LOCATION", "SIGNATURE"}},
}

// typedSchemas is the version 2 contract of every capability.
var typedSchemas = map[string][]RecordSchema{
	Courier: {
		record("SHIPPING_LABEL", req("labelReference", KindString), req("barcodeReference", KindString), f("packageId", KindString), f("carrierReference", KindString)),
		record("DELIVERY_WAVE", req("waveReference", KindString), f("plannedStart", KindTimestamp), f("plannedEnd", KindTimestamp), f("shipmentIds", KindStringArray)),
		record("DELIVERY_ATTEMPT", req("packageId", KindString), atLeast(req("attemptNumber", KindInteger), 1), enum(f("outcome", KindString), "DELIVERED", "FAILED", "PARTIAL"), f("failureReasonCode", KindString), f("attemptedAt", KindTimestamp), f("evidenceMediaId", KindMediaRef), f("cashOnDeliveryReference", KindString)),
		record("REDELIVERY_REQUEST", req("packageId", KindString), req("reasonCode", KindString), f("requestedWindowStart", KindTimestamp), f("requestedWindowEnd", KindTimestamp)),
		record("RETURN_TO_SENDER", req("packageId", KindString), req("reasonCode", KindString), f("returnLocationReference", KindString)),
	},
	Taxi: {
		{RecordType: "PASSENGER_MANIFEST", Fields: []Field{
			object(req("passengers", KindObjectArray), req("passengerReference", KindString), f("seatNumber", KindString), f("boardingStopReference", KindString), f("alightingStopReference", KindString), f("assistanceRequirement", KindString)),
			atLeast(req("seatCapacity", KindInteger), 1), f("tripReference", KindString), f("vehicleReference", KindString), notes},
			Rules: []string{"passengers must not exceed seatCapacity", "passengerReference values are unique opaque references; names and contact details are not accepted"}},
		{RecordType: "NO_SHOW", Fields: []Field{req("passengerReference", KindString), f("stopReference", KindString), atLeast(req("waitedMinutes", KindInteger), 0), req("recordedAt", KindTimestamp), notes}},
		{RecordType: "WAITING_EVENT", Fields: []Field{req("stopReference", KindString), req("waitStartedAt", KindTimestamp), f("waitEndedAt", KindTimestamp), f("reasonCode", KindString), notes},
			Rules: []string{"waitEndedAt is not before waitStartedAt"}},
		record("SHUTTLE_DEFINITION", req("scheduleReference", KindString), atLeast(req("seatCapacity", KindInteger), 1), f("routeReference", KindString), f("recurrence", KindString), f("stopReferences", KindStringArray)),
		record("PASSENGER_SAFETY_REQUIREMENT", enum(req("requirementType", KindString), "CHILD_SEAT", "WHEELCHAIR_ACCESS", "MOBILITY_ASSISTANCE", "ESCORT", "OTHER"), f("passengerReference", KindString), f("description", KindString)),
	},
	Hauling: {
		record("HAULAGE_PROFILE", enum(req("loadType", KindString), "FTL", "LTL"), req("haulingAuthorityReference", KindString), atLeast(f("palletCount", KindInteger), 0), f("trailerReference", KindString), f("containerReference", KindString)),
		record("LOADING_APPOINTMENT", req("appointmentReference", KindString), f("locationReference", KindString), f("windowStart", KindTimestamp), f("windowEnd", KindTimestamp)),
		record("DETENTION", req("startedAt", KindTimestamp), f("endedAt", KindTimestamp), f("appointmentReference", KindString), enum(f("detentionType", KindString), "DETENTION", "DEMURRAGE"), f("reasonCode", KindString)),
		record("BACKHAUL", req("originReference", KindString), req("destinationReference", KindString), f("availableFrom", KindTimestamp)),
	},
	Heavy: {
		record("HEAVY_PROFILE", object(req("dimensions", KindObject), atLeast(req("length", KindNumber), 0), atLeast(req("width", KindNumber), 0), atLeast(req("height", KindNumber), 0), enum(f("unit", KindString), "M", "CM", "FT")),
			atLeast(req("weight", KindNumber), 0), req("axleRequirements", KindString), req("trailerRequirements", KindString), req("permitReference", KindString), req("routeSurveyReference", KindString)),
		record("ROUTE_SURVEY", req("surveyReference", KindString), f("surveyedAt", KindTimestamp), f("mediaFileId", KindMediaRef)),
		record("PERMIT", req("permitReference", KindString), req("validUntil", KindTimestamp), f("issuingAuthority", KindString)),
		record("ESCORT_REQUIREMENT", req("escortType", KindString), atLeast(f("escortCount", KindInteger), 1)),
		record("LIFTING_REQUIREMENT", req("equipmentType", KindString), atLeast(f("capacity", KindNumber), 0)),
	},
	Freight: {
		record("COUNTRY_LEG", req("originCountry", KindCountry), req("destinationCountry", KindCountry), enum(f("mode", KindString), "ROAD", "RAIL", "SEA", "AIR"), atLeast(f("sequence", KindInteger), 1), f("dutiesReference", KindString), f("taxReference", KindString)),
		record("BROKER_REFERENCE", req("brokerReference", KindString), f("countryCode", KindCountry)),
		record("CUSTOMS_DOCUMENT", req("mediaFileId", KindMediaRef), f("documentType", KindString), f("countryCode", KindCountry), f("dutiesReference", KindString)),
		record("BORDER_EVENT", req("countryCode", KindCountry), req("eventType", KindString), f("occurredAt", KindTimestamp)),
		record("MULTIMODAL_HANDOVER", req("fromMode", KindString), req("toMode", KindString), f("locationReference", KindString), f("occurredAt", KindTimestamp)),
	},
	ColdChain: {
		{RecordType: "TEMPERATURE_REQUIREMENT", Fields: []Field{req("minimumTemperature", KindNumber), req("maximumTemperature", KindNumber), enum(req("unit", KindString), "C", "F"), f("releaseEvidenceReference", KindString), f("authorizedBy", KindString), notes},
			Rules: []string{"minimumTemperature is not above maximumTemperature"}},
		record("SENSOR_REFERENCE", req("sensorReference", KindString)),
		record("EXCURSION", f("sensorReference", KindString), f("observedTemperature", KindNumber), enum(f("unit", KindString), "C", "F"), f("occurredAt", KindTimestamp)),
		record("QUARANTINE", req("reasonCode", KindString)),
		record("AUTHORIZED_RELEASE", req("evidenceReference", KindString), req("authorizedBy", KindString), f("releaseEvidenceReference", KindString)),
	},
	Medical: {
		record("MEDICAL_HANDLING", req("requiredQualificationIds", KindQualification), req("handlingInstructions", KindString)),
		record("EMERGENCY_EXCEPTION", req("reasonCode", KindString), req("approvedBy", KindString)),
		record("ENHANCED_CUSTODY", f("signatureRequired", KindBoolean), f("custodyInstructions", KindString)),
	},
	Agricultural: {
		record("AGRICULTURAL_PROFILE", req("batchReference", KindString), req("lotReference", KindString), enum(req("perishability", KindString), "LOW", "MEDIUM", "HIGH"), f("harvestWindowStart", KindTimestamp), f("marketWindowEnd", KindTimestamp)),
		record("SPOILAGE_EVENT", req("batchReference", KindString), req("condition", KindString), req("disposition", KindString)),
		record("DISPOSITION", req("batchReference", KindString), req("disposition", KindString)),
	},
	Moving: {
		record("MOVING_INVENTORY", object(req("items", KindObjectArray), req("itemReference", KindString), f("description", KindString), f("room", KindString), atLeast(f("quantity", KindInteger), 1), f("condition", KindString))),
		record("SITE_PLAN", req("siteReference", KindString), f("mediaFileId", KindMediaRef)),
		record("CREW_ASSIGNMENT", req("crewReferences", KindStringArray)),
		record("CONDITION", req("itemReference", KindString), req("condition", KindString), f("evidenceMediaId", KindMediaRef)),
		record("DAMAGE_RECORD", req("itemReference", KindString), req("condition", KindString), f("evidenceMediaId", KindMediaRef)),
	},
	Towing: {
		record("TOWING_RECOVERY", req("breakdownReference", KindString), req("disabledVehicleReference", KindString), req("incidentLocation", KindString), req("towingAssetReference", KindAssetRef), req("destination", KindString)),
		record("ROADSIDE_MILESTONE", req("milestone", KindString), f("occurredAt", KindTimestamp)),
	},
}

// Schema returns the current typed schema of a capability.
func Schema(code string) (CapabilitySchema, bool) {
	types, ok := typedSchemas[code]
	if !ok {
		return CapabilitySchema{}, false
	}
	m := capabilityMeta[code]
	return CapabilitySchema{CapabilityCode: code, SchemaID: SchemaID(code, SchemaVersionCurrent), Version: SchemaVersionCurrent, SupportedVersions: []int{SchemaVersionLegacy, SchemaVersionCurrent}, RestrictedVisibility: m.restricted, ExecutionAdapter: m.adapter, RequiredProofTypes: m.proof, FinancialValues: "EXTERNAL_REFERENCES_ONLY", RecordTypes: types}, true
}

func recordSchema(code, recordType string) (RecordSchema, bool) {
	for _, r := range typedSchemas[code] {
		if r.RecordType == recordType {
			return r, true
		}
	}
	return RecordSchema{}, false
}

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

// validateTyped applies a closed field list to one JSON object.
func validateTyped(path string, fields []Field, p map[string]any) error {
	known := map[string]Field{}
	for _, fd := range fields {
		known[fd.Name] = fd
	}
	var unknown []string
	for k := range p {
		if _, ok := known[k]; !ok {
			unknown = append(unknown, path+k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return apperrors.BadRequest("Unsupported specialized fields: "+strings.Join(unknown, ", ")+".", nil)
	}
	for _, fd := range fields {
		v, present := p[fd.Name]
		if !present || v == nil {
			if fd.Required {
				return apperrors.BadRequest("Specialized payload is missing required field "+path+fd.Name+".", nil)
			}
			continue
		}
		if e := validateValue(path+fd.Name, fd, v); e != nil {
			return e
		}
	}
	return nil
}

func invalid(name, want string) error {
	return apperrors.BadRequest("Specialized field "+name+" must be "+want+".", nil)
}

func validateValue(name string, fd Field, v any) error {
	switch fd.Kind {
	case KindString, KindMediaRef, KindAssetRef:
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return invalid(name, "a non-empty string")
		}
		if len(fd.Enum) > 0 && !contains(fd.Enum, s) {
			return invalid(name, "one of "+strings.Join(fd.Enum, ", "))
		}
	case KindCountry:
		s, ok := v.(string)
		if !ok || !countryPattern.MatchString(s) {
			return invalid(name, "an ISO 3166-1 alpha-2 country code")
		}
	case KindTimestamp:
		s, ok := v.(string)
		if !ok {
			return invalid(name, "an RFC 3339 timestamp")
		}
		if _, e := time.Parse(time.RFC3339, s); e != nil {
			return invalid(name, "an RFC 3339 timestamp")
		}
	case KindBoolean:
		if _, ok := v.(bool); !ok {
			return invalid(name, "a boolean")
		}
	case KindInteger, KindNumber:
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return invalid(name, "a number")
		}
		if fd.Kind == KindInteger && n != math.Trunc(n) {
			return invalid(name, "an integer")
		}
		if fd.Minimum != nil && n < *fd.Minimum {
			return invalid(name, fmt.Sprintf("at least %v", *fd.Minimum))
		}
	case KindStringArray, KindQualification:
		a, ok := v.([]any)
		if !ok || len(a) == 0 && fd.Required {
			return invalid(name, "a non-empty array of strings")
		}
		for _, x := range a {
			if s, ok := x.(string); !ok || strings.TrimSpace(s) == "" {
				return invalid(name, "a non-empty array of strings")
			}
		}
	case KindObject:
		o, ok := v.(map[string]any)
		if !ok {
			return invalid(name, "an object")
		}
		return validateTyped(name+".", fd.Fields, o)
	case KindObjectArray:
		a, ok := v.([]any)
		if !ok || len(a) == 0 && fd.Required {
			return invalid(name, "a non-empty array of objects")
		}
		for i, x := range a {
			o, ok := x.(map[string]any)
			if !ok {
				return invalid(name, "a non-empty array of objects")
			}
			if e := validateTyped(fmt.Sprintf("%s[%d].", name, i), fd.Fields, o); e != nil {
				return e
			}
		}
	}
	return nil
}

func contains(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}

// crossFieldRules enforces the rules a field list cannot express.
func crossFieldRules(code, recordType string, p map[string]any) error {
	switch {
	case code == Taxi && recordType == "PASSENGER_MANIFEST":
		passengers, _ := p["passengers"].([]any)
		capacity, _ := p["seatCapacity"].(float64)
		if float64(len(passengers)) > capacity {
			return apperrors.BadRequest("Passenger manifest exceeds the seat capacity.", nil)
		}
		seen := map[string]bool{}
		for _, x := range passengers {
			ref, _ := x.(map[string]any)["passengerReference"].(string)
			if seen[ref] {
				return apperrors.BadRequest("Passenger references must be unique within a manifest.", nil)
			}
			seen[ref] = true
		}
	case code == Taxi && recordType == "WAITING_EVENT":
		if end, ok := p["waitEndedAt"].(string); ok {
			start, _ := time.Parse(time.RFC3339, p["waitStartedAt"].(string))
			stop, _ := time.Parse(time.RFC3339, end)
			if stop.Before(start) {
				return apperrors.BadRequest("waitEndedAt cannot be before waitStartedAt.", nil)
			}
		}
	case code == ColdChain && recordType == "TEMPERATURE_REQUIREMENT":
		if p["minimumTemperature"].(float64) > p["maximumTemperature"].(float64) {
			return apperrors.BadRequest("minimumTemperature cannot be above maximumTemperature.", nil)
		}
	}
	return nil
}

// validateCurrent validates a record against the typed version 2 schema.
func validateCurrent(v *Record) error {
	schema, ok := recordSchema(v.CapabilityCode, v.RecordType)
	if !ok {
		return apperrors.BadRequest("Record type is not valid for this specialized capability.", nil)
	}
	var p map[string]any
	if json.Unmarshal(v.Payload, &p) != nil || p == nil {
		return apperrors.BadRequest("Payload must be a JSON object.", nil)
	}
	if e := validateTyped("", schema.Fields, p); e != nil {
		return e
	}
	return crossFieldRules(v.CapabilityCode, v.RecordType, p)
}

// referenceFields returns the values of fields whose kind needs an
// organization-scoped existence check (assets, qualifications).
func referenceFields(code, recordType string, payload json.RawMessage) (assets, qualifications []string) {
	schema, ok := recordSchema(code, recordType)
	if !ok {
		return nil, nil
	}
	var p map[string]any
	_ = json.Unmarshal(payload, &p)
	for _, fd := range schema.Fields {
		switch fd.Kind {
		case KindAssetRef:
			if s, ok := p[fd.Name].(string); ok && s != "" {
				assets = append(assets, s)
			}
		case KindQualification:
			a, _ := p[fd.Name].([]any)
			for _, x := range a {
				if s, ok := x.(string); ok && s != "" {
					qualifications = append(qualifications, s)
				}
			}
		}
	}
	return assets, qualifications
}
