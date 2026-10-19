package providerreceipt

import "testing"

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
