package adminresources

import "cosmicforge/logistics/shared/go/adminowner"

// Native createReturnWorkflow atomically creates the return:<trip> mission,
// its same-customer booking and its execution link. A UUID FK alone cannot
// prove this association. Draft rows may have neither child yet; partial or
// foreign child links deny both the existing return read and the new view.
const returnParentPredicate = `EXISTS(SELECT 1 FROM dispatch_delivery_recovery_actions a WHERE a.id=t.recovery_action_id AND a.trip_id=p.id AND a.booking_id=p.booking_id AND a.customer_id=p.customer_id AND EXISTS(SELECT 1 FROM dispatch_delivery_attempts d WHERE d.id=a.delivery_attempt_id AND d.trip_id=p.id))
 AND ((t.return_mission_id IS NULL AND t.return_booking_id IS NULL) OR EXISTS(
 SELECT 1 FROM dispatch_delivery_missions rm
 JOIN dispatch_delivery_groups g ON g.id=rm.delivery_group_id AND g.id=t.delivery_group_id
 JOIN dispatch_bookings rb ON rb.id=t.return_booking_id AND rb.id=rm.booking_id AND rb.customer_id=p.customer_id
 JOIN dispatch_mission_execution_links l ON l.mission_id=rm.id AND l.booking_id=rb.id AND l.source_type=rm.source_type AND l.source_ref=rm.source_ref
 WHERE rm.id=t.return_mission_id AND rm.source_ref='return:'||p.id::text
 AND (g.organization_id IS NULL OR g.organization_id::text=p.organization_id::text)
 AND (g.customer_id IS NULL OR g.customer_id::text=p.customer_id::text)))`

// These are native execution observations. Captured Trip revisions and queue
// attempt counts cannot become full workflow mutation versions or replay grants.
// Lease owners, fingerprints, financial references, free-text errors, custody
// credentials and evidence references are deliberately absent.
func incidentRecoveryStatusSource() adminowner.SQLSpec {
	spec := tripScoped(adminowner.SQLSpec{Table: "dispatch_delivery_incidents", StateColumn: "state", Dimensions: map[string]string{"recovery_result": "recovery_result", "replacement_strategy": "replacement_strategy", "next_attempt_at_utc": "", "deadline_utc": ""}, Counters: map[string]string{"recovery_attempts": "recovery_attempts", "captured_trip_revision": "source_version"}, References: map[string]string{"trip": "trip_id", "original_provider": "provider_id", "replacement_provider": "replacement_provider_id"}, BindingColumns: []string{"trip_id", "provider_id", "replacement_provider_id"}}, "trip_id", "")
	spec.DimensionQuery = `SELECT COALESCE((SELECT jsonb_build_object('next_attempt_at_utc',to_char(t.recovery_next_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'deadline_utc',to_char(t.recovery_deadline AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM ` + spec.Table + ` t ` + spec.JoinClause + ` WHERE t.id::text=$1),'{}'::jsonb)`
	return spec
}
func returnExecutionStatusSource() adminowner.SQLSpec {
	spec := tripScoped(adminowner.SQLSpec{Table: "dispatch_return_workflows", StateColumn: "state", Dimensions: map[string]string{"assignment_state": "assignment_state", "next_attempt_at_utc": ""}, Counters: map[string]string{"assignment_attempts": "assignment_attempt_count"}, References: map[string]string{"trip": "original_trip_id", "recovery_action": "recovery_action_id", "return_mission": "return_mission_id", "return_booking": "return_booking_id"}, BindingColumns: []string{"original_trip_id", "recovery_action_id", "return_mission_id", "return_booking_id"}}, "original_trip_id", returnParentPredicate)
	spec.DimensionQuery = `SELECT COALESCE((SELECT jsonb_build_object('next_attempt_at_utc',to_char(t.assignment_next_attempt_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM ` + spec.Table + ` t ` + spec.JoinClause + ` WHERE t.id::text=$1 AND (` + spec.Predicate + `)),'{}'::jsonb)`
	return spec
}
