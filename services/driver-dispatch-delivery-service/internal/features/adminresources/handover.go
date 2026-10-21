package adminresources

import "cosmicforge/logistics/shared/go/adminowner"

// The minimal handover state is observational, never a credential-validity or
// custody-completion assertion. Native notification leasing does not always
// advance optimistic_version, so no complete mutation version is advertised.
func handoverStates() adminowner.SQLSpec {
	const parent = `t.booking_id=p.booking_id
 AND EXISTS(SELECT 1 FROM dispatch_bookings b WHERE b.id=p.booking_id AND b.customer_id=p.customer_id)
 AND (
  (t.mission_id IS NULL AND t.delivery_group_id IS NULL AND t.stop_id IS NULL
   AND t.mission_ref='trip:'||p.id::text AND t.stop_ref=lower(t.stage)
   AND t.manifest_ref='legacy-trip:'||p.id::text AND t.manifest_version=1)
  OR EXISTS(SELECT 1 FROM dispatch_delivery_missions m
   JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id
   WHERE m.id=t.mission_id AND m.trip_id=p.id AND m.id::text=t.mission_ref
    AND m.delivery_group_id=t.delivery_group_id
    AND (m.booking_id IS NULL OR m.booking_id=p.booking_id)
    AND (g.trip_id IS NULL OR g.trip_id=p.id)
    AND (g.booking_id IS NULL OR g.booking_id=p.booking_id)
    AND (g.customer_id IS NULL OR g.customer_id=p.customer_id)
    AND (g.organization_id IS NULL OR g.organization_id::text=p.organization_id)
    AND EXISTS(SELECT 1 FROM dispatch_mission_stops s WHERE s.id=t.stop_id AND s.mission_id=m.id AND s.stop_type=t.stage AND COALESCE(NULLIF(s.stop_ref,''),s.id::text)=t.stop_ref)
    AND EXISTS(SELECT 1 FROM dispatch_parcel_manifests f WHERE f.mission_id=m.id AND f.manifest_ref=t.manifest_ref AND f.version=t.manifest_version)
  )
 )`
	return tripScoped(adminowner.SQLSpec{Table: "dispatch_handover_challenges", StateColumn: "status",
		Dimensions:     map[string]string{"stage": "stage", "notification": "notification_state"},
		References:     map[string]string{"trip": "trip_id"},
		BindingColumns: []string{"trip_id", "booking_id", "mission_id", "mission_ref", "delivery_group_id", "stop_id", "stop_ref", "manifest_ref", "manifest_version", "stage", "context_key"},
	}, "trip_id", parent)
}
