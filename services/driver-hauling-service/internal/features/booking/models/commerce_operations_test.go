package bookingmodels

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeCommerceParcelsExcludesManifestAndFinancialData(t *testing.T) {
	raw := []byte(`[{
		"parcel_ref":"parcel-1","pickup_stop_ref":"stop-1",
		"manifest_ref":"manifest-1","manifest_version":2,"package_count":12,
		"manifest":{"lines":[{"description":"private vendor detail"}],"vendor_margin_minor":999},
		"handling":{"upright":true,"stack_restricted":true}
	}]`)
	parcels, err := SanitizeCommerceParcels(raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(parcels)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "private vendor detail") || strings.Contains(text, "margin") {
		t.Fatalf("provider projection leaked raw manifest/financial data: %s", text)
	}
	if len(parcels) != 1 || parcels[0].PackageCount != 12 ||
		!parcels[0].Handling.Upright || !parcels[0].Handling.StackRestricted {
		t.Fatalf("operational metadata missing: %+v", parcels)
	}
}
