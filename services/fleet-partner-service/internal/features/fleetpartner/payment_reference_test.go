package fleetpartner

import (
	"strings"
	"testing"
	"time"
)

// FLEET-13.11. Completed work is fingerprinted as JSON. A submission without a
// payment reference must fingerprint exactly as it did before the field
// existed, or every redelivery of completed work recorded earlier would be
// refused as an idempotency conflict instead of recognised as a replay.
func TestCompletedWorkFingerprintIsUnchangedWithoutAPaymentReference(t *testing.T) {
	in := CompletedWorkInput{
		SourceService: ServiceTaxi, SourceJobID: "job_1", ProviderID: "prov_1",
		MembershipID: "mem_1", OrganizationID: "org_1",
		CompletedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Currency: "NGN",
		EligibleBasisMinor: 9000, SourceVersion: 1, IdempotencyKey: "taxi:job_1:1",
	}
	if strings.Contains(string(SnapshotJSON(in)), "PaymentSourceReference") {
		t.Fatalf("an empty payment reference leaked into the fingerprinted form: %s", SnapshotJSON(in))
	}
	// The pre-field form, written out by hand: the fields in declaration
	// order under their Go names.
	const preField = `{"SourceService":"TAXI","SourceJobID":"job_1","SourceAssignmentID":"",` +
		`"ProviderID":"prov_1","MembershipID":"mem_1","OrganizationID":"org_1",` +
		`"CompletedAt":"2026-01-02T03:04:05Z","Currency":"NGN","EligibleBasisMinor":9000,` +
		`"SourceVersion":1,"IdempotencyKey":"taxi:job_1:1","CorrelationID":""}`
	if got := string(SnapshotJSON(in)); got != preField {
		t.Fatalf("fingerprinted form changed:\n got %s\nwant %s", got, preField)
	}

	with := in
	with.PaymentSourceReference = "booking_1"
	if fingerprint(with) == fingerprint(in) {
		t.Fatal("a payment reference must be part of the completed work's identity")
	}
}
