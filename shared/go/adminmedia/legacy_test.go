package adminmedia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

func TestLegacyAdoptionContractIsBoundToOwnerCaseRegistry(t *testing.T) {
	owner := "driver-dispatch-delivery-service"
	a := LegacyAdoption{ContractVersion: LegacyAdoptionContract, LegacyAssetID: uuid.NewString(), LegacyOwnerReference: "trip:1", OrganizationID: "org-1", CaseKind: "delivery-recoveries", CaseReference: uuid.NewString(), SubjectType: "TRIP", SubjectReference: uuid.NewString(), Purpose: "CARGO_CONDITION"}
	if !a.Valid(owner) {
		t.Fatal("valid attestation refused")
	}
	for name, mutate := range map[string]func(*LegacyAdoption){
		"foreign-owner-kind": func(x *LegacyAdoption) { x.CaseKind = "investigations" },
		"subject":            func(x *LegacyAdoption) { x.SubjectType = "TRUCK" },
		"purpose":            func(x *LegacyAdoption) { x.Purpose = "PROFILE_PHOTO" },
		"legacy-id":          func(x *LegacyAdoption) { x.LegacyAssetID = "not-a-uuid" },
		"contract":           func(x *LegacyAdoption) { x.ContractVersion = "v0" },
		"organization":       func(x *LegacyAdoption) { x.OrganizationID = "" },
	} {
		b := a
		mutate(&b)
		if b.Valid(owner) {
			t.Fatal("invalid attestation accepted", name)
		}
	}
	other := a
	other.CaseReference = uuid.NewString()
	if a.Digest(owner) == other.Digest(owner) || a.Digest(owner) == a.Digest("driver-hauling-service") || a.Digest(owner) != a.Digest(owner) {
		t.Fatal("digest does not bind owner and facts")
	}
	if _, err := NewLegacyAdoptionClient("admin-backoffice-service", "http://127.0.0.1", bytes.Repeat([]byte("k"), 32)); err == nil {
		t.Fatal("non-source owner client constructed")
	}
	key := bytes.Repeat([]byte("k"), 32)
	status, digest := 200, a.Digest(owner)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if caller, err := serviceauth.NewVerifier(serviceauth.Secrets{owner: key}, time.Minute).VerifyRequest(r); err != nil || caller != owner {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(LegacyAdoptionReceipt{ContractVersion: LegacyAdoptionContract, LegacyAssetID: a.LegacyAssetID, MediaObjectID: "mobj_1", ChecksumSHA256: string(bytes.Repeat([]byte("a"), 64)), AttestationDigest: digest, RecordedAt: time.Now()})
	}))
	defer server.Close()
	client, err := NewLegacyAdoptionClient(owner, server.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Adopt(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	digest = other.Digest(owner)
	if _, err := client.Adopt(context.Background(), a); !errors.Is(err, ErrInvalid) {
		t.Fatal("receipt for another attestation accepted", err)
	}
	status = 409
	if _, err := client.Adopt(context.Background(), a); !errors.Is(err, ErrConflict) {
		t.Fatal("conflict not distinguished", err)
	}
}
