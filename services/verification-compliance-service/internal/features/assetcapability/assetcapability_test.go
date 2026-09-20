package assetcapability

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/mediaclient"
)

// TestOwningServiceMapping pins which service may speak for which authority.
//
// This table is the entire "only the owning authority may open a case" rule.
// A wrong entry would not fail loudly: it would let one driver service open
// cases about another's vehicles, which reads as a permissions bug months later.
func TestOwningServiceMapping(t *testing.T) {
	expected := map[Authority]string{
		AuthorityTaxi:     "driver-taxi-service",
		AuthorityDispatch: "driver-dispatch-delivery-service",
		AuthorityHauling:  "driver-hauling-service",
	}
	for authority, service := range expected {
		if got := authority.OwningService(); got != service {
			t.Fatalf("%s owning service = %q, want %q", authority, got, service)
		}
	}
	if Authority("FLEET").OwningService() != "" {
		t.Fatal("an unknown authority must own no service, or an unrecognised value would authorize a caller")
	}
}

// TestDecisionToCertificationMapping pins the one documented translation
// between Verification's review vocabulary and the operational eligibility
// vocabulary the three driver services store.
func TestDecisionToCertificationMapping(t *testing.T) {
	cases := map[Decision]CertificationStatus{
		DecisionVerified:  CertEligible,
		DecisionRejected:  CertRejected,
		DecisionSuspended: CertSuspended,
		DecisionExpired:   CertExpired,
	}
	for decision, status := range cases {
		if got := decision.CertificationStatus(); got != status {
			t.Fatalf("%s -> %q, want %q", decision, got, status)
		}
	}
	if Decision("APPROVED").Valid() {
		t.Fatal("APPROVED is not a decision this service recognises")
	}
}

// TestTaxiDecisionWordHasNoSubstituteForPending is the guard against inventing
// a vocabulary the consumer does not have.
//
// Taxi's ingress accepts CERTIFIED/REVOKED/SUSPENDED/EXPIRED/REJECTED. It has
// no word for "under review". Mapping PENDING onto any of those would tell
// Taxi a reviewer reached a conclusion they have not reached; the honest answer
// is that there is nothing to send.
func TestTaxiDecisionWordHasNoSubstituteForPending(t *testing.T) {
	if word := CertPending.TaxiDecisionWord(); word != "" {
		t.Fatalf("PENDING mapped to Taxi decision %q; it must map to nothing", word)
	}
	if CertEligible.TaxiDecisionWord() != "CERTIFIED" {
		t.Fatal("ELIGIBLE must reach Taxi as CERTIFIED")
	}
	if CertSuspended.TaxiDecisionWord() != "SUSPENDED" {
		t.Fatal("SUSPENDED must reach Taxi unchanged")
	}
}

// TestAdaptPayloadUsesEachAuthoritysOwnAssetField proves the adapter carries
// the three shipped wire shapes instead of imposing a fourth.
func TestAdaptPayloadUsesEachAuthoritysOwnAssetField(t *testing.T) {
	canonical := map[string]any{
		"asset_id": "asset-1", "product_code": "TAXI_COMFORT", "status": "ELIGIBLE",
		"policy_version": "p1", "certification_ref": "cert-1", "reason_code": "",
		"source_version": int64(3), "event_id": "evt-1",
	}

	taxi := AdaptPayload(AuthorityTaxi, canonical)
	if taxi["vehicle_id"] != "asset-1" {
		t.Fatalf("taxi payload keys the asset as %v, want vehicle_id", taxi)
	}
	if taxi["decision"] != "CERTIFIED" {
		t.Fatalf("taxi decision = %v, want CERTIFIED", taxi["decision"])
	}
	if _, present := taxi["status"]; present {
		t.Fatal("taxi ingress takes a decision word, not a status; sending both invites it to read the wrong one")
	}

	dispatch := AdaptPayload(AuthorityDispatch, canonical)
	if dispatch["bike_id"] != "asset-1" || dispatch["status"] != "ELIGIBLE" {
		t.Fatalf("dispatch payload = %v, want bike_id + status", dispatch)
	}

	hauling := AdaptPayload(AuthorityHauling, canonical)
	if hauling["truck_id"] != "asset-1" || hauling["status"] != "ELIGIBLE" {
		t.Fatalf("hauling payload = %v, want truck_id + status", hauling)
	}
}

// TestAdaptPayloadRefusesToInventATaxiPendingWord is test 20's producer-side
// half: a status Taxi cannot express is not delivered at all.
func TestAdaptPayloadRefusesToInventATaxiPendingWord(t *testing.T) {
	pending := map[string]any{"asset_id": "asset-1", "product_code": "TAXI_COMFORT", "status": "PENDING"}
	if adapted := AdaptPayload(AuthorityTaxi, pending); adapted != nil {
		t.Fatalf("a PENDING certification produced a Taxi body %v; it must produce none", adapted)
	}
	// Dispatch and Hauling do have PENDING, so they are still deliverable.
	if AdaptPayload(AuthorityDispatch, pending) == nil {
		t.Fatal("dispatch accepts PENDING and must still receive it")
	}
}

// ── Evidence binding ────────────────────────────────────────────────────────

type stubMedia struct {
	assets map[string]mediaclient.MediaAsset
	err    error
}

func (s stubMedia) GetByID(_ context.Context, id string) (mediaclient.MediaAsset, error) {
	if s.err != nil {
		return mediaclient.MediaAsset{}, s.err
	}
	asset, ok := s.assets[id]
	if !ok {
		return mediaclient.MediaAsset{}, errors.New("not found")
	}
	return asset, nil
}

func binding() EvidenceBinding {
	return EvidenceBinding{Authority: AuthorityTaxi, ProviderID: "prv-1", AssetID: "veh-1"}
}

// TestEvidenceMustBelongToTheAssetItDescribes is mandatory tests 12 and 13.
//
// The failure this prevents is specific: one provider's inspection photograph
// cited as proof of another provider's vehicle. Nothing about the bytes would
// look wrong, and the reviewer would be looking at a real photograph of a real
// vehicle -- just not this one.
func TestEvidenceMustBelongToTheAssetItDescribes(t *testing.T) {
	validator := NewEvidenceValidator(stubMedia{assets: map[string]mediaclient.MediaAsset{
		"mine": {ID: "mine", OwnerService: "driver-taxi-service", OwnerID: "veh-1",
			Purpose: "document_file", Status: "active"},
		"someone-else": {ID: "someone-else", OwnerService: "driver-taxi-service", OwnerID: "veh-9",
			Purpose: "document_file", Status: "active"},
		"wrong-service": {ID: "wrong-service", OwnerService: "vendor-service", OwnerID: "veh-1",
			Purpose: "document_file", Status: "active"},
	}})

	if err := validator.Validate(context.Background(), binding(), []string{"mine"}); err != nil {
		t.Fatalf("evidence bound to this asset was refused: %v", err)
	}
	err := validator.Validate(context.Background(), binding(), []string{"someone-else"})
	if !errors.Is(err, ErrEvidence) || !strings.Contains(err.Error(), ReasonEvidenceWrongOwner) {
		t.Fatalf("another asset's evidence = %v, want %s", err, ReasonEvidenceWrongOwner)
	}
	err = validator.Validate(context.Background(), binding(), []string{"wrong-service"})
	if !errors.Is(err, ErrEvidence) || !strings.Contains(err.Error(), ReasonEvidenceWrongOwner) {
		t.Fatalf("evidence owned by an unrelated service = %v, want %s", err, ReasonEvidenceWrongOwner)
	}
}

// TestRevokedEvidenceIsRefused is mandatory test 14.
func TestRevokedEvidenceIsRefused(t *testing.T) {
	validator := NewEvidenceValidator(stubMedia{assets: map[string]mediaclient.MediaAsset{
		"deleted": {ID: "deleted", OwnerService: "driver-taxi-service", OwnerID: "veh-1",
			Purpose: "document_file", Status: "deleted"},
	}})
	err := validator.Validate(context.Background(), binding(), []string{"deleted"})
	if !errors.Is(err, ErrEvidence) || !strings.Contains(err.Error(), ReasonEvidenceNotActive) {
		t.Fatalf("deleted evidence = %v, want %s", err, ReasonEvidenceNotActive)
	}
	missing := validator.Validate(context.Background(), binding(), []string{"never-existed"})
	if !errors.Is(missing, ErrEvidence) || !strings.Contains(missing.Error(), ReasonEvidenceNotFound) {
		t.Fatalf("missing evidence = %v, want %s", missing, ReasonEvidenceNotFound)
	}
}

// TestEvidencePurposeMustDescribeTheAsset refuses an object that exists and is
// correctly owned but is the wrong KIND of object -- a profile photo or a
// delivery signature cited as proof of a vehicle's condition.
func TestEvidencePurposeMustDescribeTheAsset(t *testing.T) {
	validator := NewEvidenceValidator(stubMedia{assets: map[string]mediaclient.MediaAsset{
		"selfie": {ID: "selfie", OwnerService: "driver-taxi-service", OwnerID: "veh-1",
			Purpose: "profile_photo", Status: "active"},
	}})
	err := validator.Validate(context.Background(), binding(), []string{"selfie"})
	if !errors.Is(err, ErrEvidence) || !strings.Contains(err.Error(), ReasonEvidenceWrongPurpose) {
		t.Fatalf("profile photo as capability evidence = %v, want %s", err, ReasonEvidenceWrongPurpose)
	}
}

// TestEvidenceFailsClosedWithoutMedia proves an unconfigured Media integration
// refuses citations rather than approving them unchecked.
func TestEvidenceFailsClosedWithoutMedia(t *testing.T) {
	validator := NewEvidenceValidator(nil)
	if err := validator.Validate(context.Background(), binding(), nil); err != nil {
		t.Fatalf("no citations should need no Media: %v", err)
	}
	err := validator.Validate(context.Background(), binding(), []string{"anything"})
	if !errors.Is(err, ErrEvidence) {
		t.Fatalf("citation without Media = %v, want a refusal", err)
	}
}

// ── Documents never self-certify ────────────────────────────────────────────

// TestDocumentValidityIsNotCapability is mandatory test 23, enforced
// structurally rather than by review.
//
// VehicleDocument can answer whether a document is valid. There is deliberately
// no method, function or field anywhere in this package that turns a set of
// valid documents into an eligible certification -- the only writer of
// CertEligible is the reviewed decision path. This test asserts both halves:
// that validity is answerable, and that answering it yields nothing about
// eligibility.
func TestDocumentValidityIsNotCapability(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	valid := VehicleDocument{Status: DocumentValid, ExpiresAt: &future}
	if !valid.CurrentlyValid(now) {
		t.Fatal("an unexpired VALID document should read as currently valid")
	}
	expired := VehicleDocument{Status: DocumentValid, ExpiresAt: &past}
	if expired.CurrentlyValid(now) {
		t.Fatal("a VALID document past its expiry is not currently valid")
	}
	pending := VehicleDocument{Status: DocumentPending}
	if pending.CurrentlyValid(now) {
		t.Fatal("a PENDING document is not valid evidence")
	}

	// The structural half. A document carries no certification status, no
	// product code and no eligibility, so there is nothing for a caller to read
	// off it and mistake for admission.
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ELIGIBLE", "certification_ref", "product_code"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("a vehicle document exposes %q; a document must not be readable as an admission", forbidden)
		}
	}
}

// ── Published events carry nothing they should not ──────────────────────────

func sampleService() *Service {
	fixed := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return NewService(Options{Now: func() time.Time { return fixed }})
}

// forbiddenEventKeys are field names that must never appear in a certification
// event.
//
// The two groups fail differently. Customer data leaking here would spread
// passenger identity into three driver services that have no reason to hold it.
// A price leaking here would make a certification look like a commercial term,
// and the first consumer to read it would start making dispatch decisions on a
// number this service has no authority over.
func forbiddenEventKeys() []string {
	return []string{
		"customer", "customer_id", "passenger", "email", "phone", "address",
		"latitude", "longitude", "location",
		"price", "fare", "amount", "amount_minor", "currency", "tariff",
		"rate", "commission", "surge", "cost", "fee",
	}
}

// TestEventsCarryNoCustomerDataAndNoPricing is mandatory tests 24 and 25.
//
// It walks the actual published payloads rather than asserting against a list
// of expected keys, so a field added later is caught by this test rather than
// by whoever notices a fare in a certification feed.
func TestEventsCarryNoCustomerDataAndNoPricing(t *testing.T) {
	service := sampleService()
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	record := Case{
		CaseRef: "case-1", Authority: AuthorityTaxi, ProviderID: "prv-1", AssetID: "veh-1",
		CapabilityCode: "PREMIUM_CONDITION", ProductCode: "TAXI_COMFORT", CorrelationID: "corr-1",
	}
	certification := Certification{
		CertificationID: "cert-1", Authority: AuthorityTaxi, ProviderID: "prv-1", AssetID: "veh-1",
		ProductCode: "TAXI_COMFORT", Status: CertEligible, CertificationRef: "dec-1",
		SourceVersion: 2, ExpiresAt: &expires,
	}
	decision := DecisionRecord{DecisionRef: "dec-1", Decision: DecisionVerified, CorrelationID: "corr-1"}

	payloads := map[string]map[string]any{
		"capability":    service.capabilityPayload(record, certification),
		"certification": service.certificationPayload(certification, decision, map[string]any{"total_seats": 4}),
		"inspection": service.inspectionPayload(Inspection{
			InspectionRef: "insp-1", Authority: AuthorityTaxi, AssetID: "veh-1",
			InspectionType: InspectionPhysical, Outcome: InspectionPassed, SourceVersion: 1,
		}),
		"expiry": service.expiredCertificationPayload(certification),
	}

	for name, payload := range payloads {
		for key := range payload {
			lowered := strings.ToLower(key)
			for _, forbidden := range forbiddenEventKeys() {
				if lowered == forbidden {
					t.Fatalf("%s event carries %q", name, key)
				}
			}
		}
	}
}

// TestCertificationEventCarriesItsProvenance checks the fields Section 5.12
// requires are actually present, so a consumer can dedupe, order and trace.
func TestCertificationEventCarriesItsProvenance(t *testing.T) {
	service := sampleService()
	certification := Certification{
		CertificationID: "cert-1", Authority: AuthorityHauling, ProviderID: "prv-1",
		AssetID: "truck-1", ProductCode: "HAUL_REEFER", Status: CertEligible,
		CertificationRef: "dec-1", SourceVersion: 4, PolicyVersion: "p2",
	}
	decision := DecisionRecord{DecisionRef: "dec-1", Decision: DecisionVerified, CorrelationID: "corr-9"}
	payload := service.certificationPayload(certification, decision, nil)

	for _, required := range []string{
		"schema_version", "authority", "provider_id", "asset_id", "product_code",
		"status", "policy_version", "source_version", "certification_ref",
		"occurred_at", "correlation_id", "causation_id",
	} {
		if _, ok := payload[required]; !ok {
			t.Fatalf("certification event is missing %q", required)
		}
	}
	if payload["causation_id"] != "dec-1" {
		t.Fatalf("causation_id = %v, want the decision that caused it", payload["causation_id"])
	}
	if payload["source_version"] != int64(4) {
		t.Fatalf("source_version = %v, want the certification's own version", payload["source_version"])
	}
}

// TestDisabledServiceRefusesWritesButNotReads is Section 5.16.
//
// The asymmetry is the point. Turning the flag off stops new certifications
// being issued; it must not hide the ones already issued, because a
// certification-gated product whose certifications have gone quiet looks
// exactly like a product that was never gated.
func TestDisabledServiceRefusesWritesButNotReads(t *testing.T) {
	service := NewService(Options{Enabled: false})
	if service.Enabled() {
		t.Fatal("service reports enabled with the flag off")
	}
	if err := service.writable(); !errors.Is(err, ErrDisabled) {
		t.Fatalf("write gate = %v, want ErrDisabled", err)
	}
	// The read paths take no writable() gate at all; this asserts the
	// authorization check is what refuses them, not the flag.
	_, err := service.ReadCertifications(context.Background(), "vendor-service", AuthorityTaxi, "veh-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("an unrelated caller reading certifications = %v, want ErrForbidden", err)
	}
}

// TestAcceptedEvidencePurposesAreOnesMediaActuallyHas guards against the
// specification's assumption that vehicle_inspection and capability_evidence
// exist as media purposes. They do not, and citing one would be unuploadable.
func TestAcceptedEvidencePurposesAreOnesMediaActuallyHas(t *testing.T) {
	// The four purposes media-file-service defines.
	known := map[string]bool{
		"profile_photo": true, "document_file": true, "proof_image": true, "signature": true,
	}
	for _, purpose := range AcceptedEvidencePurposes() {
		if !known[purpose] {
			t.Fatalf("accepted purpose %q is not one media-file-service defines", purpose)
		}
	}
}

// TestValidCodeMatchesTheDriverServicesOwnPattern keeps this service from
// issuing a certification the receiving projection cannot store.
func TestValidCodeMatchesTheDriverServicesOwnPattern(t *testing.T) {
	for _, good := range []string{"TAXI_COMFORT", "HAUL_REEFER", "D1"} {
		if !ValidCode(good) {
			t.Fatalf("%q should be a valid product code", good)
		}
	}
	for _, bad := range []string{"", "lowercase", "1LEADING_DIGIT", "HAS SPACE", "X"} {
		if ValidCode(bad) {
			t.Fatalf("%q should not be a valid product code", bad)
		}
	}
}
