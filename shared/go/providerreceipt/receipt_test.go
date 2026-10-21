package providerreceipt

import "testing"

func TestAssignmentIntentV1DigestRemainsCompatible(t *testing.T) {
	// Golden digest from the assignment-only v1 serialization, before the
	// additive offer protocol. Existing durable receipts must remain usable.
	original := Intent{AssignmentID: "asg_legacy", ProviderID: "provider-legacy", ClientResponseID: "original", Decision: "ACCEPTED"}
	if original.Digest() != "d2e0717a7629231faaeb8688259870e1f8d645b2a12b972ecbeb449efa1fc227" {
		t.Fatal("offer fields changed the existing assignment receipt protocol")
	}
}

func TestIntentDigestBindsOriginalDecision(t *testing.T) {
	original := Intent{AssignmentID: "assignment", ProviderID: "provider", ClientResponseID: "key", Decision: "DECLINED", ReasonCode: "SAFETY", Note: "note", Evidence: []string{"b", "a"}}
	reordered := original
	reordered.Evidence = []string{"a", "b", "a"}
	if original.Digest() != reordered.Digest() {
		t.Fatal("evidence set order changed semantic identity")
	}
	for _, change := range []func(*Intent){func(i *Intent) { i.AssignmentID = "other" }, func(i *Intent) { i.ProviderID = "other" }, func(i *Intent) { i.ClientResponseID = "new" }, func(i *Intent) { i.Decision = "ACCEPTED" }, func(i *Intent) { i.Note = "changed" }, func(i *Intent) { i.ReasonCode = "OTHER" }, func(i *Intent) { i.Evidence = []string{"a"} }} {
		mutated := original
		change(&mutated)
		if mutated.Digest() == original.Digest() {
			t.Fatal("different decision has same canonical digest")
		}
	}
}
