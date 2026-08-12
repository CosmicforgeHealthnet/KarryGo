package specialized

import (
	"cosmicforge/logistics/shared/go/apperrors"
	"encoding/json"
	"fmt"
	"strings"
)

var moduleTypes = map[string]map[string]bool{
	Courier:      set("SHIPPING_LABEL", "DELIVERY_WAVE", "DELIVERY_ATTEMPT", "REDELIVERY_REQUEST", "RETURN_TO_SENDER"),
	Taxi:         set("PASSENGER_MANIFEST", "NO_SHOW", "WAITING_EVENT", "SHUTTLE_DEFINITION", "PASSENGER_SAFETY_REQUIREMENT"),
	Hauling:      set("HAULAGE_PROFILE", "LOADING_APPOINTMENT", "DETENTION", "BACKHAUL"),
	Heavy:        set("HEAVY_PROFILE", "ROUTE_SURVEY", "PERMIT", "ESCORT_REQUIREMENT", "LIFTING_REQUIREMENT"),
	Freight:      set("COUNTRY_LEG", "BROKER_REFERENCE", "CUSTOMS_DOCUMENT", "BORDER_EVENT", "MULTIMODAL_HANDOVER"),
	ColdChain:    set("TEMPERATURE_REQUIREMENT", "SENSOR_REFERENCE", "EXCURSION", "QUARANTINE", "AUTHORIZED_RELEASE"),
	Medical:      set("MEDICAL_HANDLING", "EMERGENCY_EXCEPTION", "ENHANCED_CUSTODY"),
	Agricultural: set("AGRICULTURAL_PROFILE", "SPOILAGE_EVENT", "DISPOSITION"),
	Moving:       set("MOVING_INVENTORY", "SITE_PLAN", "CREW_ASSIGNMENT", "CONDITION", "DAMAGE_RECORD"),
	Towing:       set("TOWING_RECOVERY", "ROADSIDE_MILESTONE"),
}

func set(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range v {
		m[x] = true
	}
	return m
}
func ValidCapability(v string) bool { _, ok := moduleTypes[v]; return ok }
func ValidateRecord(v *Record) error {
	v.CapabilityCode = strings.ToUpper(strings.TrimSpace(v.CapabilityCode))
	v.RecordType = strings.ToUpper(strings.TrimSpace(v.RecordType))
	types, ok := moduleTypes[v.CapabilityCode]
	if !ok || !types[v.RecordType] {
		return apperrors.BadRequest("Record type is not valid for this specialized capability.", nil)
	}
	if v.SubjectType == "" || v.SubjectReference == "" {
		return apperrors.BadRequest("Subject type and reference are required.", nil)
	}
	var p map[string]any
	if json.Unmarshal(v.Payload, &p) != nil {
		return apperrors.BadRequest("Payload must be a JSON object.", nil)
	}
	need := func(keys ...string) error {
		for _, k := range keys {
			if x, ok := p[k]; !ok || fmt.Sprint(x) == "" {
				return apperrors.BadRequest("Specialized payload is missing required field "+k+".", nil)
			}
		}
		return nil
	}
	rejectFinance := func() error {
		for _, k := range []string{"amount", "dutiesAmount", "taxAmount", "ledgerEntry", "walletBalance", "refundAmount"} {
			if _, ok := p[k]; ok {
				return apperrors.BadRequest("Financial values are external references only.", nil)
			}
		}
		return nil
	}
	switch v.CapabilityCode {
	case Courier:
		if e := rejectFinance(); e != nil {
			return e
		}
		switch v.RecordType {
		case "SHIPPING_LABEL":
			return need("labelReference", "barcodeReference")
		case "DELIVERY_ATTEMPT":
			return need("packageId", "attemptNumber")
		case "REDELIVERY_REQUEST", "RETURN_TO_SENDER":
			return need("packageId", "reasonCode")
		}
	case Taxi:
		if strings.Contains(v.RecordType, "EXECUTION") {
			return apperrors.Unavailable("Taxi execution is disabled.", nil)
		}
		switch v.RecordType {
		case "PASSENGER_MANIFEST":
			return need("passengers", "seatCapacity")
		case "SHUTTLE_DEFINITION":
			return need("scheduleReference", "seatCapacity")
		}
	case Hauling:
		switch v.RecordType {
		case "HAULAGE_PROFILE":
			return need("loadType", "haulingAuthorityReference")
		case "LOADING_APPOINTMENT":
			return need("appointmentReference")
		}
	case Heavy:
		switch v.RecordType {
		case "HEAVY_PROFILE":
			return need("dimensions", "weight", "axleRequirements", "trailerRequirements", "permitReference", "routeSurveyReference")
		case "PERMIT":
			return need("permitReference", "validUntil")
		case "ROUTE_SURVEY":
			return need("surveyReference")
		}
	case Freight:
		if e := rejectFinance(); e != nil {
			return e
		}
		switch v.RecordType {
		case "COUNTRY_LEG":
			return need("originCountry", "destinationCountry")
		case "CUSTOMS_DOCUMENT":
			return need("mediaFileId")
		case "BORDER_EVENT":
			return need("countryCode", "eventType")
		}
	case ColdChain:
		switch v.RecordType {
		case "TEMPERATURE_REQUIREMENT":
			return need("minimumTemperature", "maximumTemperature", "unit")
		case "SENSOR_REFERENCE":
			return need("sensorReference")
		case "AUTHORIZED_RELEASE":
			return need("evidenceReference", "authorizedBy")
		}
	case Medical:
		v.RestrictedVisibility = true
		switch v.RecordType {
		case "MEDICAL_HANDLING":
			return need("requiredQualificationIds", "handlingInstructions")
		case "EMERGENCY_EXCEPTION":
			return need("reasonCode", "approvedBy")
		}
	case Agricultural:
		switch v.RecordType {
		case "AGRICULTURAL_PROFILE":
			return need("batchReference", "lotReference", "perishability")
		case "SPOILAGE_EVENT":
			return need("batchReference", "condition", "disposition")
		}
	case Moving:
		switch v.RecordType {
		case "MOVING_INVENTORY":
			return need("items")
		case "CONDITION", "DAMAGE_RECORD":
			return need("itemReference", "condition")
		}
	case Towing:
		switch v.RecordType {
		case "TOWING_RECOVERY":
			return need("breakdownReference", "disabledVehicleReference", "incidentLocation", "towingAssetReference", "destination")
		}
	}
	return nil
}
