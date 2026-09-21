package integration_test

// Section 16: network evidence -- purposes, privacy, binding and whether a
// file still counts.
//
// These run against the real PostgreSQL schema through the real signed private
// contract. The claims worth making here are about what the service REFUSES:
// a sensitive photo made public, a file offered as evidence for the wrong job,
// a revoked file still counted. Every one of those is a request that looks
// valid and must not succeed.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	mediamodels "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
)

// uploadEvidence authorises and completes one evidence upload, returning the
// final object view.
func (f *mediaFixture) uploadEvidence(t *testing.T, purpose, bindingType, bindingID string,
	extra map[string]any) map[string]any {
	t.Helper()
	key := "ev-" + uuid.NewString()
	overrides := map[string]any{
		"uploadPurpose":    purpose,
		"ownerType":        "PROVIDER",
		"ownerReference":   "prv_" + key,
		"allowedMimeTypes": []string{"image/png"},
		"bindingType":      bindingType,
		"bindingId":        bindingID,
	}
	for name, value := range extra {
		overrides[name] = value
	}
	status, response := f.authorizeUpload(t, key, overrides)
	if status != http.StatusOK {
		t.Fatalf("authorize %s: status=%d body=%v", purpose, status, response)
	}
	grant := dataOf(t, response)
	status, response = f.completeUpload(t, grant, pngFile(),
		map[string]string{"X-Declared-Content-Type": "image/png"})
	if status != http.StatusOK {
		t.Fatalf("complete %s: status=%d body=%v", purpose, status, response)
	}
	return dataOf(t, response)
}

func (f *mediaFixture) validate(t *testing.T, expectation map[string]any) (int, map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(expectation)
	if err != nil {
		t.Fatal(err)
	}
	return f.call(t, callOptions{method: http.MethodPost, path: "/evidence-validations", body: encoded})
}

func failuresOf(verdict map[string]any) []string {
	raw, _ := verdict["failures"].([]any)
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		if code, ok := value.(string); ok {
			out = append(out, code)
		}
	}
	return out
}

func containsCode(codes []string, want string) bool {
	for _, code := range codes {
		if code == want {
			return true
		}
	}
	return false
}

// TestEveryNetworkEvidencePurposeIsAccepted is the taxonomy.
func TestEveryNetworkEvidencePurposeIsAccepted(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	for _, purpose := range []string{
		mediamodels.PurposeVehicleInspection, mediamodels.PurposeCapabilityEvidence,
		mediamodels.PurposeReadinessEvidence, mediamodels.PurposeCargoCondition,
		mediamodels.PurposeDamageEvidence, mediamodels.PurposeRecoveryTransfer,
		mediamodels.PurposeSafetyIncident, mediamodels.PurposeLoadingEvidence,
	} {
		t.Run(purpose, func(t *testing.T) {
			object := fixture.uploadEvidence(t, purpose, mediamodels.BindingMission,
				"mis_"+uuid.NewString(), nil)
			if object["uploadPurpose"] != purpose {
				t.Fatalf("purpose = %v, want %s", object["uploadPurpose"], purpose)
			}
			if object["status"] != mediamodels.StatusActive {
				t.Fatalf("status = %v, want ACTIVE", object["status"])
			}
		})
	}
}

// TestExistingPurposesStillWork proves the taxonomy is additive. Every purpose
// that existed before Section 16 is still accepted, unbound, exactly as before.
func TestExistingPurposesStillWork(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{})
	for _, purpose := range []string{
		mediamodels.PurposeCompanyDocument, mediamodels.PurposeVehicleDocument,
		mediamodels.PurposeProofDocument, mediamodels.PurposeDisputeAttachment,
	} {
		key := "legacy-" + uuid.NewString()
		status, response := fixture.authorizeUpload(t, key, map[string]any{"uploadPurpose": purpose})
		if status != http.StatusOK {
			t.Fatalf("%s no longer accepted: status=%d body=%v", purpose, status, response)
		}
	}
}

// TestSensitiveEvidenceDefaultsToPrivate checks the default and the refusal.
func TestSensitiveEvidenceDefaultsToPrivate(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})

	object := fixture.uploadEvidence(t, mediamodels.PurposeDamageEvidence,
		mediamodels.BindingMission, "mis_"+uuid.NewString(), nil)
	if object["visibility"] != mediamodels.VisibilityPrivate {
		t.Fatalf("damage evidence stored as %v, want PRIVATE", object["visibility"])
	}

	for _, purpose := range []string{
		mediamodels.PurposeVehicleInspection, mediamodels.PurposeCapabilityEvidence,
		mediamodels.PurposeCargoCondition, mediamodels.PurposeDamageEvidence,
		mediamodels.PurposeRecoveryTransfer, mediamodels.PurposeSafetyIncident,
	} {
		key := "public-" + uuid.NewString()
		status, response := fixture.authorizeUpload(t, key, map[string]any{
			"uploadPurpose": purpose, "visibility": "PUBLIC",
			"allowedMimeTypes": []string{"image/png"},
			"bindingType":      mediamodels.BindingMission, "bindingId": "mis_" + key,
		})
		if status == http.StatusOK {
			t.Fatalf("%s was allowed to be PUBLIC", purpose)
		}
		if code := errorCode(response); code != "visibility_must_be_private" {
			t.Fatalf("%s public refusal code = %q", purpose, code)
		}
	}
}

// TestExplicitPublicAllowedOnlyForNonSensitivePurposes shows PUBLIC is not
// banned outright -- it is permitted exactly where exposure is not the harm.
func TestExplicitPublicAllowedOnlyForNonSensitivePurposes(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{})
	key := "explicit-public-" + uuid.NewString()
	status, response := fixture.authorizeUpload(t, key, map[string]any{
		"uploadPurpose": mediamodels.PurposeLoadingEvidence, "visibility": "PUBLIC",
		"allowedMimeTypes": []string{"image/png"},
		"bindingType":      mediamodels.BindingMission, "bindingId": "mis_" + key,
	})
	if status != http.StatusOK {
		t.Fatalf("explicit PUBLIC for a non-sensitive purpose was refused: %d %v", status, response)
	}
}

// TestSensitiveEvidenceMustBeBound refuses a damage photo bound to nothing,
// which could later be attached to any job at all.
func TestSensitiveEvidenceMustBeBound(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{})
	key := "unbound-" + uuid.NewString()
	status, response := fixture.authorizeUpload(t, key, map[string]any{
		"uploadPurpose":    mediamodels.PurposeSafetyIncident,
		"allowedMimeTypes": []string{"image/png"},
	})
	if status == http.StatusOK {
		t.Fatal("unbound safety evidence was accepted")
	}
	if code := errorCode(response); code != "binding_required" {
		t.Fatalf("refusal code = %q, want binding_required", code)
	}
}

func TestUnknownBindingTypeIsRejected(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{})
	key := "badtype-" + uuid.NewString()
	status, response := fixture.authorizeUpload(t, key, map[string]any{
		"uploadPurpose": mediamodels.PurposeDamageEvidence, "allowedMimeTypes": []string{"image/png"},
		"bindingType": "SPACESHIP", "bindingId": "x",
	})
	if status == http.StatusOK || errorCode(response) != "binding_type_unsupported" {
		t.Fatalf("status=%d code=%q", status, errorCode(response))
	}
}

// TestValidationConfirmsCorrectEvidence is the positive case the others refuse.
func TestValidationConfirmsCorrectEvidence(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	missionID := "mis_" + uuid.NewString()
	object := fixture.uploadEvidence(t, mediamodels.PurposeRecoveryTransfer,
		mediamodels.BindingMission, missionID, nil)

	status, response := fixture.validate(t, map[string]any{
		"mediaObjectId": object["mediaObjectId"], "ownerService": testCaller,
		"purposes":    []string{mediamodels.PurposeRecoveryTransfer},
		"bindingType": mediamodels.BindingMission, "bindingId": missionID,
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, response)
	}
	verdict := dataOf(t, response)
	if verdict["usable"] != true {
		t.Fatalf("correct evidence judged unusable: %v", failuresOf(verdict))
	}
}

// TestWrongBindingsAreRejected covers the three binding mistakes that matter:
// the right photo offered for the wrong mission, the wrong asset, the wrong
// owner.
func TestWrongBindingsAreRejected(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	missionID := "mis_" + uuid.NewString()
	assetID := "ast_" + uuid.NewString()
	object := fixture.uploadEvidence(t, mediamodels.PurposeDamageEvidence,
		mediamodels.BindingMission, missionID, map[string]any{
			"secondaryBindings": []map[string]string{
				{"bindingType": mediamodels.BindingAsset, "bindingId": assetID},
			},
		})
	id := object["mediaObjectId"]

	cases := map[string]struct {
		expectation map[string]any
		want        string
	}{
		"wrong mission": {map[string]any{"mediaObjectId": id,
			"bindingType": mediamodels.BindingMission, "bindingId": "mis_other"}, "WRONG_BINDING"},
		"wrong asset": {map[string]any{"mediaObjectId": id,
			"bindingType": mediamodels.BindingAsset, "bindingId": "ast_other"}, "WRONG_BINDING"},
		"wrong owner": {map[string]any{"mediaObjectId": id,
			"ownerService": "some-other-service"}, "WRONG_OWNER"},
		"wrong purpose": {map[string]any{"mediaObjectId": id,
			"purposes": []string{mediamodels.PurposeLoadingEvidence}}, "WRONG_PURPOSE"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			status, response := fixture.validate(t, tc.expectation)
			if status != http.StatusOK {
				t.Fatalf("status=%d body=%v", status, response)
			}
			verdict := dataOf(t, response)
			if verdict["usable"] != false {
				t.Fatalf("%s was judged usable", name)
			}
			if !containsCode(failuresOf(verdict), tc.want) {
				t.Fatalf("failures=%v, want %s", failuresOf(verdict), tc.want)
			}
		})
	}

	// The secondary binding is honoured: the same photo IS valid evidence for
	// the asset it was declared against.
	status, response := fixture.validate(t, map[string]any{"mediaObjectId": id,
		"bindingType": mediamodels.BindingAsset, "bindingId": assetID})
	if status != http.StatusOK || dataOf(t, response)["usable"] != true {
		t.Fatalf("secondary binding not honoured: %d %v", status, response)
	}
}

// TestRevokedEvidenceIsNotUsable proves revocation removes the file from use
// while keeping it -- the bytes and the record both survive.
func TestRevokedEvidenceIsNotUsable(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	missionID := "mis_" + uuid.NewString()
	object := fixture.uploadEvidence(t, mediamodels.PurposeCargoCondition,
		mediamodels.BindingMission, missionID, nil)
	id := object["mediaObjectId"].(string)
	checksumBefore := object["checksumSha256"]

	encoded, _ := json.Marshal(map[string]any{
		"expectedVersion": object["sourceVersion"], "reason": "UPLOADED_AGAINST_WRONG_JOB",
	})
	status, response := fixture.call(t, callOptions{method: http.MethodPost,
		path: "/objects/" + id + "/revocations", body: encoded})
	if status != http.StatusOK {
		t.Fatalf("revoke: status=%d body=%v", status, response)
	}
	revoked := dataOf(t, response)
	if revoked["status"] != mediamodels.StatusRevoked {
		t.Fatalf("status after revoke = %v", revoked["status"])
	}
	if revoked["checksumSha256"] != checksumBefore {
		t.Fatal("revocation changed the checksum; the bytes must be untouched")
	}

	status, response = fixture.validate(t, map[string]any{"mediaObjectId": id,
		"bindingType": mediamodels.BindingMission, "bindingId": missionID})
	verdict := dataOf(t, response)
	if status != http.StatusOK || verdict["usable"] != false ||
		!containsCode(failuresOf(verdict), "NOT_ACTIVE") {
		t.Fatalf("revoked evidence still usable: %d %v", status, verdict)
	}

	// A retried revocation is harmless.
	status, _ = fixture.call(t, callOptions{method: http.MethodPost,
		path: "/objects/" + id + "/revocations", body: encoded})
	if status != http.StatusOK {
		t.Fatalf("retried revocation failed: %d", status)
	}
}

// TestDeletedEvidenceIsNotUsable: a file whose bytes are gone is never
// evidence, whatever its evidence state says.
func TestDeletedEvidenceIsNotUsable(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	missionID := "mis_" + uuid.NewString()
	object := fixture.uploadEvidence(t, mediamodels.PurposeLoadingEvidence,
		mediamodels.BindingMission, missionID, nil)
	id := object["mediaObjectId"].(string)

	if _, err := fixture.pool.Exec(t.Context(), `
		UPDATE media_objects SET deletion_state='DELETED', deleted_at=now() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	status, response := fixture.validate(t, map[string]any{"mediaObjectId": id,
		"bindingType": mediamodels.BindingMission, "bindingId": missionID})
	verdict := dataOf(t, response)
	if status != http.StatusOK || verdict["status"] != mediamodels.StatusDeleted ||
		verdict["usable"] != false {
		t.Fatalf("deleted evidence reported as %v usable=%v", verdict["status"], verdict["usable"])
	}
}

// TestUnscannedEvidenceIsNotUsable: PENDING is not "not unsafe".
func TestUnscannedEvidenceIsNotUsable(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{}) // no scanner: stays PENDING
	missionID := "mis_" + uuid.NewString()
	object := fixture.uploadEvidence(t, mediamodels.PurposeLoadingEvidence,
		mediamodels.BindingMission, missionID, nil)
	status, response := fixture.validate(t, map[string]any{"mediaObjectId": object["mediaObjectId"],
		"bindingType": mediamodels.BindingMission, "bindingId": missionID})
	verdict := dataOf(t, response)
	if status != http.StatusOK || verdict["usable"] != false ||
		!containsCode(failuresOf(verdict), "NOT_SAFE") {
		t.Fatalf("unscanned evidence treated as usable: %v", verdict)
	}
}

// TestValidationIsNotAnExistenceOracle: a bare id with no context is refused,
// and an unknown id answers exactly like a deleted one.
func TestValidationIsNotAnExistenceOracle(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{})
	encoded, _ := json.Marshal(map[string]any{"mediaObjectId": "mob_does_not_exist"})
	status, response := fixture.call(t, callOptions{method: http.MethodPost,
		path: "/evidence-validations", body: encoded,
		headers: map[string]string{"X-Organization-ID": ""}})
	if status == http.StatusOK || errorCode(response) != "context_required" {
		t.Fatalf("contextless validation answered: %d %v", status, response)
	}

	status, response = fixture.validate(t, map[string]any{"mediaObjectId": "mob_does_not_exist",
		"bindingType": mediamodels.BindingMission, "bindingId": "mis_x"})
	verdict := dataOf(t, response)
	if status != http.StatusOK || verdict["status"] != mediamodels.StatusDeleted {
		t.Fatalf("unknown id answered distinctly: %v", verdict)
	}
}

// TestBindingsAreImmutable: a file cannot quietly become evidence for a job it
// was never taken on.
func TestBindingsAreImmutable(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	object := fixture.uploadEvidence(t, mediamodels.PurposeDamageEvidence,
		mediamodels.BindingMission, "mis_"+uuid.NewString(), map[string]any{
			"secondaryBindings": []map[string]string{
				{"bindingType": mediamodels.BindingParcel, "bindingId": "par_1"},
			},
		})
	_, err := fixture.pool.Exec(t.Context(), `
		UPDATE media_object_secondary_bindings SET binding_id='par_other'
		 WHERE media_object_id=$1`, object["mediaObjectId"])
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("a secondary binding was rewritable: %v", err)
	}
}

// TestSensitivePublicIsUnstorable proves the database refuses it even if the
// use case were bypassed.
func TestSensitivePublicIsUnstorable(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	object := fixture.uploadEvidence(t, mediamodels.PurposeSafetyIncident,
		mediamodels.BindingMission, "mis_"+uuid.NewString(), nil)
	_, err := fixture.pool.Exec(t.Context(),
		`UPDATE media_objects SET visibility='PUBLIC' WHERE id=$1`, object["mediaObjectId"])
	if err == nil {
		t.Fatal("the database accepted a PUBLIC safety incident")
	}
}

// TestEvidenceContractCarriesNoMoney is the standing pricing guard: nothing on
// the evidence contract names a fee, fare or amount.
func TestEvidenceContractCarriesNoMoney(t *testing.T) {
	fixture := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	missionID := "mis_" + uuid.NewString()
	object := fixture.uploadEvidence(t, mediamodels.PurposeDamageEvidence,
		mediamodels.BindingMission, missionID, nil)
	_, response := fixture.validate(t, map[string]any{"mediaObjectId": object["mediaObjectId"],
		"bindingType": mediamodels.BindingMission, "bindingId": missionID})
	for _, document := range []map[string]any{object, dataOf(t, response)} {
		for field := range document {
			lowered := strings.ToLower(field)
			for _, banned := range []string{"price", "fare", "amount", "kobo", "fee",
				"payment", "refund", "payout"} {
				if strings.Contains(lowered, banned) {
					t.Errorf("evidence field %q looks like money", field)
				}
			}
		}
	}
}
