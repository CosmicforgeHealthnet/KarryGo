package main

import "testing"

func TestCanonicalCertificationRejectsSkippedDatabaseCase(t *testing.T) {
	data := []byte("{\"Action\":\"pass\",\"Test\":\"TestUnit\"}\n{\"Action\":\"skip\",\"Test\":\"TestPostgresMissingFixture\"}\n")
	if _, _, err := testEvidence(data); err == nil {
		t.Fatal("skipped database certification accepted")
	}
	if _, _, err := testEvidence([]byte("{\"Action\":\"pass\",\"Package\":\"empty\"}\n")); err == nil {
		t.Fatal("empty package certification accepted")
	}
	if n, _, err := testEvidence([]byte("{\"Action\":\"pass\",\"Test\":\"TestActualDatabase\"}\n")); err != nil || n != 1 {
		t.Fatal(n, err)
	}
}
