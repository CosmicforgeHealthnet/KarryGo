package idempotency

import "testing"

func TestFingerprintCanonicalizesJSON(t *testing.T) {
	a := Fingerprint("post", "/orders", []byte(`{"branch":"b1","items":[{"qty":2,"sku":"x"}]}`))
	b := Fingerprint(" POST ", "/orders", []byte(" { \n \"items\" : [ { \"sku\" : \"x\", \"qty\" : 2 } ], \"branch\" : \"b1\" } "))
	if a != b {
		t.Fatalf("semantically equal JSON produced different fingerprints: %s != %s", a, b)
	}
	if a == Fingerprint("POST", "/orders", []byte(`{"branch":"b2","items":[{"qty":2,"sku":"x"}]}`)) {
		t.Fatal("operation-critical content change did not change fingerprint")
	}
	if a == Fingerprint("POST", "/other", []byte(`{"branch":"b1","items":[{"qty":2,"sku":"x"}]}`)) {
		t.Fatal("path was not bound into fingerprint")
	}
}

func TestFingerprintPreservesNonJSONByteIdentity(t *testing.T) {
	if Fingerprint("POST", "/upload", []byte("a b")) == Fingerprint("POST", "/upload", []byte("a  b")) {
		t.Fatal("non-JSON transport bytes were incorrectly normalized")
	}
}
