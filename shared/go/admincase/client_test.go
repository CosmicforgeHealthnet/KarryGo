package admincase

import (
	"bytes"
	"context"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSourceCaseClientPinsOwnerMembershipScopePolicyAndAudience(t *testing.T) {
	key := bytes.Repeat([]byte("s"), 32)
	q := adminread.Query{Scope: adminread.Scope{Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: "policy-1", PolicyVersion: 1, PolicyChecksum: strings.Repeat("a", 64), Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1}
	spec, _ := Find("customer-service", "review-work-items")
	expected := Proof{ContractVersion: Contract, Owner: spec.Owner, CaseKind: spec.Kind, CaseReference: "case-1", SubjectType: "CUSTOMER", SubjectReference: "customer-1", Scope: adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}, SourceContract: spec.SourceContract, NativeFingerprint: strings.Repeat("b", 64), ExpiresAt: time.Now().Add(time.Minute), PolicyID: q.PolicyID, PolicyVersion: q.PolicyVersion, PolicyChecksum: q.PolicyChecksum}
	for _, name := range []string{"valid", "owner", "case", "subject", "contract", "scope", "policy", "expiry", "fingerprint", "unknown", "duplicate", "alias", "raw-url", "oversize", "redirect"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != spec.Path()+"/proofs/review-work-items/case-1/CUSTOMER/customer-1" || r.URL.Query().Encode() != q.Values().Encode() {
					t.Error("arbitrary source path/query")
				}
				if _, err := serviceauth.NewVerifier(serviceauth.Secrets{Caller: key}, 30*time.Second).VerifyRequest(r); err != nil {
					t.Error("wrong source workload audience", err)
				}
				proof := expected
				switch name {
				case "owner":
					proof.Owner = "driver-taxi-service"
				case "case":
					proof.CaseReference = "case-2"
				case "subject":
					proof.SubjectReference = "foreign"
				case "contract":
					proof.SourceContract = "other.v1"
				case "scope":
					proof.Scope.Tenant = "foreign"
				case "policy":
					proof.PolicyVersion++
				case "expiry":
					proof.ExpiresAt = time.Now().Add(-time.Second)
				case "fingerprint":
					proof.NativeFingerprint = "not-a-digest"
				case "redirect":
					w.Header().Set("Location", "https://test.invalid/private")
					w.WriteHeader(302)
					return
				}
				raw, _ := json.Marshal(proof)
				switch name {
				case "unknown", "raw-url":
					raw = append(raw[:len(raw)-1], []byte(`,"private_url":"https://test.invalid/private"}`)...)
				case "duplicate":
					raw = append(raw[:len(raw)-1], []byte(`,"owner":"customer-service"}`)...)
				case "alias":
					raw = append(raw[:len(raw)-1], []byte(`,"Owner":"customer-service"}`)...)
				case "oversize":
					raw = []byte(strings.Repeat("x", 16385))
				}
				w.Write(raw)
			}))
			defer server.Close()
			client, err := NewClient(spec.Owner, server.URL, key)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			proof, err := client.Authorize(context.Background(), q, spec.Kind, "case-1", "CUSTOMER", "customer-1")
			if name == "valid" {
				if err != nil || proof.SubjectReference != expected.SubjectReference {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("unbound source response accepted")
			}
		})
	}
	for _, owner := range []string{"admin-backoffice-service", "media-file-service", "https://test.invalid", "unknown"} {
		if _, err := NewClient(owner, "http://127.0.0.1", key); err == nil {
			t.Fatal("invented source case owner", owner)
		}
	}
}

func TestCaseRegistryHasOnlyNativeCasesAndExactSubjectRoles(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Specs() {
		identity := s.Owner + ":" + s.Kind
		if seen[identity] || s.Path() == "" || len(s.States) == 0 || len(s.Subjects) == 0 || len(s.Purposes) == 0 {
			t.Fatal("invalid case dictionary", identity)
		}
		seen[identity] = true
		for subject, roles := range s.Subjects {
			if subject == "" || len(roles) == 0 {
				t.Fatal("unbound subject")
			}
		}
	}
	// Every enrolled case belongs to one of the five source owners; the
	// original native cases and Fleet's standalone investigations stay present.
	owners := map[string]bool{"customer-service": true, "driver-taxi-service": true, "driver-dispatch-delivery-service": true, "driver-hauling-service": true, "fleet-partner-service": true}
	for identity := range seen {
		if !owners[identity[:strings.Index(identity, ":")]] {
			t.Fatal("case enrolled for a non-source owner", identity)
		}
	}
	for _, identity := range []string{"customer-service:review-work-items", "driver-taxi-service:recoveries", "driver-dispatch-delivery-service:incidents", "driver-dispatch-delivery-service:delivery-recoveries", "driver-dispatch-delivery-service:returns", "driver-hauling-service:recoveries", "fleet-partner-service:investigations"} {
		if !seen[identity] {
			t.Fatal("native case missing", identity)
		}
	}
	if fleet, _ := Find("fleet-partner-service", "investigations"); fleet.Path() != "/internal/fleet-partner/v1/admin/case-authority" || Contains(fleet.States, "CLOSED") {
		t.Fatal("fleet investigation case contract drifted")
	}
	taxi, _ := Find("driver-taxi-service", "recoveries")
	hauling, _ := Find("driver-hauling-service", "recoveries")
	if !Contains(taxi.States, "EVALUATING") || !Contains(hauling.States, "EVALUATING") || len(taxi.Subjects["VEHICLE"]) != 2 || len(taxi.Subjects["TRUCK"]) != 0 || len(hauling.Subjects["VEHICLE"]) != 0 {
		t.Fatal("recovery cases lost native state/subject vocabulary")
	}
	for _, raw := range []string{`{"scope":{"tenant":"one","tenant":"two"}}`, `{"Owner":"x"}`, `{"owner":"one","owner":"two"}`, `{"owner":"x"} {}`, `{"extra":"x"}`} {
		var p Proof
		if Strict([]byte(raw), &p) == nil {
			t.Fatal("ambiguous source contract accepted", raw)
		}
	}
}
