package specialized

import (
	"encoding/json"
	"testing"
)

func TestEverySpecializedModuleValidatesItsAuthorityReferences(t *testing.T) {
	tests := []struct {
		name, capability, recordType, payload string
	}{
		{"courier", Courier, "SHIPPING_LABEL", `{"labelReference":"label-1","barcodeReference":"barcode-1"}`},
		{"taxi foundation", Taxi, "PASSENGER_MANIFEST", `{"passengers":["opaque-passenger-1"],"seatCapacity":4}`},
		{"hauling", Hauling, "HAULAGE_PROFILE", `{"loadType":"FTL","haulingAuthorityReference":"haul-1"}`},
		{"heavy", Heavy, "HEAVY_PROFILE", `{"dimensions":{"length":20},"weight":40,"axleRequirements":"8-axle","trailerRequirements":"low-loader","permitReference":"permit-1","routeSurveyReference":"survey-1"}`},
		{"freight", Freight, "COUNTRY_LEG", `{"originCountry":"IT","destinationCountry":"FR","dutiesReference":"external-duty-1"}`},
		{"cold chain", ColdChain, "TEMPERATURE_REQUIREMENT", `{"minimumTemperature":2,"maximumTemperature":8,"unit":"C"}`},
		{"medical", Medical, "MEDICAL_HANDLING", `{"requiredQualificationIds":["qual-1"],"handlingInstructions":"restricted"}`},
		{"agricultural", Agricultural, "AGRICULTURAL_PROFILE", `{"batchReference":"batch-1","lotReference":"lot-1","perishability":"HIGH"}`},
		{"moving", Moving, "MOVING_INVENTORY", `{"items":[{"reference":"item-1"}]}`},
		{"towing", Towing, "TOWING_RECOVERY", `{"breakdownReference":"breakdown-1","disabledVehicleReference":"asset-1","incidentLocation":"opaque-location-1","towingAssetReference":"asset-2","destination":"opaque-location-2"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := Record{CapabilityCode: tt.capability, RecordType: tt.recordType, SubjectType: "SHIPMENT", SubjectReference: "shipment-1", Payload: json.RawMessage(tt.payload)}
			if err := ValidateRecord(&record); err != nil {
				t.Fatalf("valid module record rejected: %v", err)
			}
			if tt.capability == Medical && !record.RestrictedVisibility {
				t.Fatal("medical records must force restricted visibility")
			}
		})
	}
}

func TestSpecializedModulesFailClosedAtAuthorityBoundaries(t *testing.T) {
	tests := []struct {
		name, capability, recordType, payload string
	}{
		{"courier cannot create COD ledger data", Courier, "SHIPPING_LABEL", `{"labelReference":"label-1","barcodeReference":"barcode-1","amount":100}`},
		{"freight cannot calculate duties", Freight, "COUNTRY_LEG", `{"originCountry":"IT","destinationCountry":"FR","dutiesAmount":100}`},
		{"heavy needs permit", Heavy, "HEAVY_PROFILE", `{"dimensions":{},"weight":40,"axleRequirements":"8-axle","trailerRequirements":"low-loader","routeSurveyReference":"survey-1"}`},
		{"heavy needs survey", Heavy, "HEAVY_PROFILE", `{"dimensions":{},"weight":40,"axleRequirements":"8-axle","trailerRequirements":"low-loader","permitReference":"permit-1"}`},
		{"hauling needs authority", Hauling, "HAULAGE_PROFILE", `{"loadType":"FTL"}`},
		{"towing reuses breakdown", Towing, "TOWING_RECOVERY", `{"disabledVehicleReference":"asset-1","incidentLocation":"opaque","towingAssetReference":"asset-2","destination":"opaque"}`},
		{"taxi execution has no record type", Taxi, "EXECUTION", `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := Record{CapabilityCode: tt.capability, RecordType: tt.recordType, SubjectType: "SHIPMENT", SubjectReference: "shipment-1", Payload: json.RawMessage(tt.payload)}
			if err := ValidateRecord(&record); err == nil {
				t.Fatal("unsafe specialized record accepted")
			}
		})
	}
}
