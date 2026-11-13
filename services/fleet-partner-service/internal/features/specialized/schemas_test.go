package specialized

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTypedSchemasCoverEveryRecordType keeps the schema registry and the
// record-type catalogue (and the database CHECK they mirror) in step.
func TestTypedSchemasCoverEveryRecordType(t *testing.T) {
	for code, types := range moduleTypes {
		schema, ok := Schema(code)
		if !ok {
			t.Fatalf("%s has no typed schema", code)
		}
		if schema.SchemaID != code+".v2" || len(schema.SupportedVersions) != 2 || schema.FinancialValues != "EXTERNAL_REFERENCES_ONLY" {
			t.Fatalf("%s schema metadata = %+v", code, schema)
		}
		seen := map[string]bool{}
		for _, r := range schema.RecordTypes {
			if !types[r.RecordType] {
				t.Errorf("%s schema declares unknown record type %s", code, r.RecordType)
			}
			if r.AdditionalProperties {
				t.Errorf("%s/%s schema is open", code, r.RecordType)
			}
			seen[r.RecordType] = true
		}
		for rt := range types {
			if !seen[rt] {
				t.Errorf("%s/%s has no typed schema", code, rt)
			}
		}
	}
	if _, ok := Schema("TELEPORTATION"); ok {
		t.Fatal("unknown capability has a schema")
	}
	if s, _ := Schema(Taxi); !s.RestrictedVisibility {
		t.Fatal("passenger data must be restricted")
	}
	if s, _ := Schema(Medical); !s.RestrictedVisibility {
		t.Fatal("medical data must be restricted")
	}
}

func v2(capability, recordType, payload string) Record {
	return Record{CapabilityCode: capability, RecordType: recordType, SubjectType: "ORDER", SubjectReference: "order-1", Payload: json.RawMessage(payload)}
}

func TestCurrentSchemaAcceptsTypedPayloads(t *testing.T) {
	for name, r := range map[string]Record{
		"courier label":     v2(Courier, "SHIPPING_LABEL", `{"labelReference":"l","barcodeReference":"b"}`),
		"courier attempt":   v2(Courier, "DELIVERY_ATTEMPT", `{"packageId":"p","attemptNumber":2,"outcome":"FAILED","attemptedAt":"2026-11-13T10:00:00Z"}`),
		"taxi manifest":     v2(Taxi, "PASSENGER_MANIFEST", `{"passengers":[{"passengerReference":"pax-1","seatNumber":"1A"},{"passengerReference":"pax-2"}],"seatCapacity":4}`),
		"taxi no-show":      v2(Taxi, "NO_SHOW", `{"passengerReference":"pax-1","waitedMinutes":10,"recordedAt":"2026-11-13T10:00:00Z"}`),
		"taxi waiting":      v2(Taxi, "WAITING_EVENT", `{"stopReference":"s","waitStartedAt":"2026-11-13T10:00:00Z","waitEndedAt":"2026-11-13T10:05:00Z"}`),
		"taxi shuttle":      v2(Taxi, "SHUTTLE_DEFINITION", `{"scheduleReference":"weekday-am","seatCapacity":14,"stopReferences":["a","b"]}`),
		"taxi safety":       v2(Taxi, "PASSENGER_SAFETY_REQUIREMENT", `{"requirementType":"WHEELCHAIR_ACCESS"}`),
		"haulage":           v2(Hauling, "HAULAGE_PROFILE", `{"loadType":"LTL","haulingAuthorityReference":"h","palletCount":12}`),
		"heavy":             v2(Heavy, "HEAVY_PROFILE", `{"dimensions":{"length":20,"width":3.5,"height":4,"unit":"M"},"weight":40,"axleRequirements":"8","trailerRequirements":"low-loader","permitReference":"p","routeSurveyReference":"r"}`),
		"freight":           v2(Freight, "COUNTRY_LEG", `{"originCountry":"NG","destinationCountry":"GH","mode":"ROAD","dutiesReference":"ext-duty-1"}`),
		"cold chain":        v2(ColdChain, "TEMPERATURE_REQUIREMENT", `{"minimumTemperature":2,"maximumTemperature":8,"unit":"C"}`),
		"medical":           v2(Medical, "MEDICAL_HANDLING", `{"requiredQualificationIds":["q-1"],"handlingInstructions":"upright"}`),
		"agricultural":      v2(Agricultural, "AGRICULTURAL_PROFILE", `{"batchReference":"b","lotReference":"l","perishability":"HIGH"}`),
		"moving":            v2(Moving, "MOVING_INVENTORY", `{"items":[{"itemReference":"sofa","quantity":1}]}`),
		"towing":            v2(Towing, "TOWING_RECOVERY", `{"breakdownReference":"b","disabledVehicleReference":"v","incidentLocation":"l","towingAssetReference":"ast_1","destination":"d"}`),
		"notes on any type": v2(Towing, "ROADSIDE_MILESTONE", `{"milestone":"HOOKED","notes":"ok"}`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRecord(&r); err != nil {
				t.Fatalf("rejected: %v", err)
			}
			if r.SchemaVersion != SchemaVersionCurrent {
				t.Fatalf("schema version = %d", r.SchemaVersion)
			}
		})
	}
}

func TestCurrentSchemaRejectsUnsupportedAndUntypedData(t *testing.T) {
	for name, tc := range map[string]struct {
		r    Record
		want string
	}{
		"unknown field":            {v2(Courier, "SHIPPING_LABEL", `{"labelReference":"l","barcodeReference":"b","colour":"red"}`), "Unsupported"},
		"financial value":          {v2(Hauling, "DETENTION", `{"startedAt":"2026-11-13T10:00:00Z","amount":100}`), "Unsupported"},
		"passenger names":          {v2(Taxi, "PASSENGER_MANIFEST", `{"passengers":[{"passengerReference":"p","name":"Ada"}],"seatCapacity":2}`), "Unsupported"},
		"over seat capacity":       {v2(Taxi, "PASSENGER_MANIFEST", `{"passengers":[{"passengerReference":"a"},{"passengerReference":"b"},{"passengerReference":"c"}],"seatCapacity":2}`), "seat capacity"},
		"duplicate passenger":      {v2(Taxi, "PASSENGER_MANIFEST", `{"passengers":[{"passengerReference":"a"},{"passengerReference":"a"}],"seatCapacity":2}`), "unique"},
		"zero seats":               {v2(Taxi, "SHUTTLE_DEFINITION", `{"scheduleReference":"s","seatCapacity":0}`), "at least"},
		"wait ends before start":   {v2(Taxi, "WAITING_EVENT", `{"stopReference":"s","waitStartedAt":"2026-11-13T10:05:00Z","waitEndedAt":"2026-11-13T10:00:00Z"}`), "waitEndedAt"},
		"unknown safety type":      {v2(Taxi, "PASSENGER_SAFETY_REQUIREMENT", `{"requirementType":"MASSAGE"}`), "one of"},
		"fractional integer":       {v2(Courier, "DELIVERY_ATTEMPT", `{"packageId":"p","attemptNumber":1.5}`), "integer"},
		"bad timestamp":            {v2(Heavy, "PERMIT", `{"permitReference":"p","validUntil":"next tuesday"}`), "timestamp"},
		"bad country":              {v2(Freight, "COUNTRY_LEG", `{"originCountry":"Nigeria","destinationCountry":"GH"}`), "country"},
		"inverted temperature":     {v2(ColdChain, "TEMPERATURE_REQUIREMENT", `{"minimumTemperature":9,"maximumTemperature":2,"unit":"C"}`), "minimumTemperature"},
		"nested unknown field":     {v2(Heavy, "HEAVY_PROFILE", `{"dimensions":{"length":1,"width":1,"height":1,"depth":2},"weight":1,"axleRequirements":"a","trailerRequirements":"t","permitReference":"p","routeSurveyReference":"r"}`), "dimensions.depth"},
		"missing required":         {v2(Hauling, "HAULAGE_PROFILE", `{"loadType":"FTL"}`), "haulingAuthorityReference"},
		"empty qualification list": {v2(Medical, "MEDICAL_HANDLING", `{"requiredQualificationIds":[],"handlingInstructions":"x"}`), "non-empty"},
		"not an object":            {v2(Moving, "SITE_PLAN", `["x"]`), "JSON object"},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateRecord(&tc.r)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
	unsupported := v2(Courier, "SHIPPING_LABEL", `{"labelReference":"l","barcodeReference":"b"}`)
	unsupported.SchemaVersion = 3
	if err := ValidateRecord(&unsupported); err == nil {
		t.Fatal("schema version 3 accepted")
	}
}

// TestLegacySchemaVersionStaysValid proves backward compatibility: a payload
// that version 1 accepted (extra fields, untyped passengers) still validates
// when the record declares version 1, and is refused by version 2.
func TestLegacySchemaVersionStaysValid(t *testing.T) {
	legacy := v2(Taxi, "PASSENGER_MANIFEST", `{"passengers":["opaque-passenger-1"],"seatCapacity":4,"legacyNote":"kept"}`)
	legacy.SchemaVersion = SchemaVersionLegacy
	if err := ValidateRecord(&legacy); err != nil {
		t.Fatalf("version 1 payload rejected: %v", err)
	}
	if !legacy.RestrictedVisibility {
		t.Fatal("passenger records are restricted under every version")
	}
	current := v2(Taxi, "PASSENGER_MANIFEST", `{"passengers":["opaque-passenger-1"],"seatCapacity":4,"legacyNote":"kept"}`)
	if err := ValidateRecord(&current); err == nil {
		t.Fatal("version 2 accepted an untyped legacy payload")
	}
}

func TestReferenceFieldsExtractsAssetsAndQualifications(t *testing.T) {
	assets, quals := referenceFields(Towing, "TOWING_RECOVERY", json.RawMessage(`{"towingAssetReference":"ast_9"}`))
	if len(assets) != 1 || assets[0] != "ast_9" || len(quals) != 0 {
		t.Fatalf("towing refs = %v %v", assets, quals)
	}
	assets, quals = referenceFields(Medical, "MEDICAL_HANDLING", json.RawMessage(`{"requiredQualificationIds":["q1","q2"]}`))
	if len(assets) != 0 || len(quals) != 2 {
		t.Fatalf("medical refs = %v %v", assets, quals)
	}
}
