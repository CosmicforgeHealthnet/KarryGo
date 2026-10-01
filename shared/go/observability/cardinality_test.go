package observability

import "testing"

func TestIdentifierLabelKeysAreRefused(t *testing.T) {
	for _, key := range []string{
		"id", "order_id", "orderId", "provider_id", "membership_id",
		"request_id", "correlation_id", "causation_id", "trip-id",
		"mission_ref", "idempotency_key", "customer_email", "driver_phone",
		"api_token", "payload_hash", "booking_uuid",
	} {
		if LabelKeyIsLowCardinality(key) {
			t.Errorf("%q was accepted as a metric label key", key)
		}
	}
}

// The check must not reject a key merely for naming a domain concept. A
// breakdown by operating mode is three values and is the whole point of having
// labels; refusing it would push people back to no labels at all.
func TestBoundedLabelKeysAreAccepted(t *testing.T) {
	for _, key := range []string{
		"service", "target", "state", "status", "outcome", "route", "method",
		"worker", "reason", "failure_class", "operation", "provider_mode",
		"resource", "action", "span", "dependency", "purpose", "result",
		"table", "category",
	} {
		if !LabelKeyIsLowCardinality(key) {
			t.Errorf("%q was refused, but it is a bounded breakdown", key)
		}
	}
}

func TestUnsafeLabelKeysReportsOnlyTheOffenders(t *testing.T) {
	unsafe := UnsafeLabelKeys(map[string]string{
		"service": "wallet", "target": "fleet", "instruction_id": "inst_1",
	})
	if len(unsafe) != 1 || unsafe[0] != "instruction_id" {
		t.Fatalf("unsafe = %v, want exactly [instruction_id]", unsafe)
	}
	if got := UnsafeLabelKeys(map[string]string{"service": "wallet"}); len(got) != 0 {
		t.Fatalf("a safe label set reported %v", got)
	}
	if got := UnsafeLabelKeys(nil); len(got) != 0 {
		t.Fatalf("a nil label set reported %v", got)
	}
}
