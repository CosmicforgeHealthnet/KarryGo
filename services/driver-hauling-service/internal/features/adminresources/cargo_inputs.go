package adminresources

import (
	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminowner"
)

// These are the current booking fields consumed by native recovery's
// requirementFor, not a historical full cargofit.Requirement snapshot. No
// dimension, temperature, cargo class, equipment or settlement fact is inferred.
func bookingCargoInputsSource() adminowner.SQLSpec {
	return adminowner.SQLSpec{Table: "haulage_bookings", StateColumn: "status",
		CanonicalQuery: `SELECT '','',COALESCE(NULLIF(corporate_organization_id,''),winner_organization_id::text,''),COALESCE(NULLIF(corporate_organization_id,''),winner_organization_id::text,'') FROM haulage_bookings WHERE id::text=$1 AND (NULLIF(corporate_organization_id,'') IS NULL OR winner_organization_id IS NULL OR corporate_organization_id=winner_organization_id::text)`,
		Predicate:      `(NULLIF(t.corporate_organization_id,'') IS NULL OR t.winner_organization_id IS NULL OR t.corporate_organization_id=t.winner_organization_id::text)`,
		BindingColumns: []string{"customer_id", "corporate_organization_id", "winner_organization_id", "execution_mode"},
		Dimensions:     map[string]string{"preferred_truck_type": "preferred_truck_type", "fragile_handling_required": "is_fragile"},
		Counters:       map[string]string{"recorded_weight_kg": "cargo_weight_kg", "normalized_weight_grams": ""},
	}
}
func enrichBookingCargoInputs(resource adminowner.Resource) (adminowner.Resource, error) {
	var kilograms, grams *int64
	for _, counter := range resource.Counters {
		if counter.Name == "recorded_weight_kg" && counter.Value != nil {
			if *counter.Value < 0 || *counter.Value > 2147483647 {
				return adminowner.Resource{}, adminowner.ErrContract
			}
			if *counter.Value > 0 {
				value := *counter.Value
				kilograms = &value
				converted := cargofit.FromKilograms(int(value))
				grams = &converted
			}
		}
	}
	for i := range resource.Counters {
		switch resource.Counters[i].Name {
		case "recorded_weight_kg":
			resource.Counters[i].Value = kilograms
		case "normalized_weight_grams":
			resource.Counters[i].Value = grams
		}
	}
	resource.SnapshotETag = ""
	resource.SnapshotETag = admincontext.Fingerprint(resource)
	return resource, nil
}
