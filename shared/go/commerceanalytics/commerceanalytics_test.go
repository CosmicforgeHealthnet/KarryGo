package commerceanalytics

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValidateEnvelopeRejectsUnknownSchemaAndEvent(t *testing.T) {
	payload, _ := json.Marshal(OrderCompletedPayload{OrderID: "ord_1", OrderValueKobo: 1200})
	event := Envelope{
		EventID:         "evt_1",
		EventType:       EventOrderCompleted,
		SchemaVersion:   EventSchemaVersion,
		OccurredAt:      time.Now().UTC(),
		SourceService:   "vendor-service",
		SourceAggregate: "VendorOrder",
		SourceID:        "ord_1",
		SourceVersion:   1,
		BusinessID:      "biz_1",
		Payload:         payload,
	}
	if err := ValidateEnvelope(event); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	event.SchemaVersion = 99
	if err := ValidateEnvelope(event); err == nil {
		t.Fatal("expected unsupported schema to fail closed")
	}
	event.SchemaVersion = EventSchemaVersion
	event.EventType = "commerce.future.unknown"
	if err := ValidateEnvelope(event); err == nil {
		t.Fatal("expected unknown event type to fail closed")
	}
}

func TestGuaranteeStatusIsFutureDisabled(t *testing.T) {
	if GuaranteeStatusFutureDisabled != "FUTURE_DISABLED" {
		t.Fatalf("guarantee must remain disabled, got %s", GuaranteeStatusFutureDisabled)
	}
}

func TestReliabilityWeightsUseNeutralDefaultWithoutApprovedPolicy(t *testing.T) {
	policy := CurrentReliabilityWeightPolicy()
	if policy.Version != ReliabilityPolicyVersion {
		t.Fatalf("policy version = %s", policy.Version)
	}
	if policy.Source != "technical_default_no_approved_business_policy_found" {
		t.Fatalf("unexpected policy source: %s", policy.Source)
	}
	for name, weight := range policy.Weights {
		if weight != 0.20 {
			t.Fatalf("%s weight = %.2f, want neutral 0.20", name, weight)
		}
	}
}

func TestPriceBenchmarkRequiresExternalCohortAndCoarsensRange(t *testing.T) {
	valid := PriceSnapshotPayload{
		ItemID:                     "item_1",
		OwnPriceKobo:               1200,
		SampleSize:                 MinimumBenchmarkCohort,
		ExternalVendorCount:        MinimumBenchmarkCohort,
		RequestingVendorExcluded:   true,
		ComparableProductAuthority: "catalogue_equivalence_v1",
		GeoBucketRef:               "geo_6km_bucket",
		UnitNormalized:             true,
		WindowRef:                  "30D",
	}
	if !PriceBenchmarkAllowed(valid) {
		t.Fatal("valid privacy-safe benchmark rejected")
	}
	valid.ExternalVendorCount = MinimumBenchmarkCohort - 1
	if PriceBenchmarkAllowed(valid) {
		t.Fatal("low independent external vendor cohort must be suppressed")
	}
	valid.ExternalVendorCount = MinimumBenchmarkCohort
	valid.RequestingVendorExcluded = false
	if PriceBenchmarkAllowed(valid) {
		t.Fatal("benchmark must exclude requesting vendor")
	}
	low, high := CoarseBenchmarkRange(1234, 1988)
	if low != 1200 || high != 2000 {
		t.Fatalf("coarse range = %d-%d, want 1200-2000", low, high)
	}
}
