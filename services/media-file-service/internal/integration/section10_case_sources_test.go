package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mediahttp "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/http"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// casePolicy signs a TEST_ONLY policy allowing the Media content permission
// and each given source case permission for one scope.
func casePolicy(t *testing.T, scope adminpolicy.Scope, specs ...admincase.Spec) *policyfixture.Fixture {
	t.Helper()
	p := policyfixture.New(t, scope, "media-file-service", adminmedia.Permission, adminmedia.Contract)
	capabilities := []string{}
	for _, spec := range specs {
		rule := p.Document.Rules[0]
		rule.Owner, rule.Capability, rule.Permission, rule.SourceContract = spec.Owner, spec.Permission, spec.Permission, spec.SourceContract
		p.Document.Rules = append(p.Document.Rules, rule)
		p.Document.ApprovedOwners = append(p.Document.ApprovedOwners, spec.Owner)
		capabilities = append(capabilities, spec.Permission)
	}
	p.Document.Media = &adminpolicy.MediaDisclosure{MaxGrantSeconds: 60, SourceCapabilities: capabilities} // TEST_ONLY value.
	p.Sign(t)
	return p
}

// The expiry is the source allocation expiry: stable across proofs, as a
// real owner returns it, so the grant binding fingerprint stays stable.
var sourceAllocationExpiry = time.Now().UTC().Add(10 * time.Minute).Truncate(time.Second)

func proofFor(spec admincase.Spec, q adminread.Query, ref, subject, subjectRef string) admincase.Proof {
	return admincase.Proof{ContractVersion: admincase.Contract, Owner: spec.Owner, CaseKind: spec.Kind, CaseReference: ref, SubjectType: subject, SubjectReference: subjectRef, Scope: adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}, SourceContract: spec.SourceContract, NativeFingerprint: admincontext.Fingerprint("TEST_ONLY_" + ref), ExpiresAt: sourceAllocationExpiry, PolicyID: q.PolicyID, PolicyVersion: q.PolicyVersion, PolicyChecksum: q.PolicyChecksum}
}

// signedCaseSource is a TEST_ONLY stand-in for an owner's case-authority
// proof route, reached by the real admincase.Client over signed HTTP. The
// actual Fleet route is certified in fleet-partner-service against its own
// migrations; this double only holds case state and subject membership.
type signedCaseSource struct {
	mu       sync.Mutex
	spec     admincase.Spec
	key      []byte
	nonces   map[string]bool
	open     map[string]bool
	subjects map[string]map[string]string // case -> subject type -> reference
}

func (s *signedCaseSource) handler() http.Handler {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(s.spec.Path()+"/proofs/:kind/:id/:subject/:reference", func(c *gin.Context) {
		if _, err := serviceauth.NewVerifier(serviceauth.Secrets{admincase.Caller: s.key}, 30*time.Second).VerifyRequest(c.Request); err != nil {
			c.JSON(401, gin.H{})
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		nonce := c.GetHeader(serviceauth.HeaderNonce)
		if s.nonces[nonce] {
			c.JSON(409, gin.H{})
			return
		}
		s.nonces[nonce] = true
		values, _ := url.ParseQuery(c.Request.URL.RawQuery)
		q, err := adminread.Parse(values, time.Now())
		ref, subject, reference := c.Param("id"), c.Param("subject"), c.Param("reference")
		member := subject == "CASE" && reference == ref || s.subjects[ref][subject] == reference
		if err != nil || c.Param("kind") != s.spec.Kind || !s.open[ref] || !member {
			c.JSON(404, gin.H{})
			return
		}
		c.JSON(200, proofFor(s.spec, q, ref, subject, reference))
	})
	return router
}

// stubSource is an in-process CaseSource for a kind constructed in a test.
type stubSource struct {
	spec admincase.Spec
	open map[string]bool
}

func (s *stubSource) Authorize(_ context.Context, q adminread.Query, kind, ref, subject, subjectRef string) (admincase.Proof, error) {
	if kind != s.spec.Kind || !s.open[ref] {
		return admincase.Proof{}, admincase.ErrUnavailable
	}
	return proofFor(s.spec, q, ref, subject, subjectRef), nil
}

func uploadCaseEvidence(t *testing.T, f *mediaFixture, owner, ownerType, ownerRef, purpose, bindingType, bindingID string, payload []byte) string {
	t.Helper()
	caller := usecases.Caller{Service: owner, OrganizationID: testOrgID}
	grant, err := f.service.AuthorizeUpload(context.Background(), usecases.AuthorizeUploadInput{Caller: caller, OwnerType: ownerType, OwnerReference: ownerRef, UploadPurpose: purpose, Visibility: "PRIVATE", FileName: "evidence.pdf", AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 4096, IdempotencyKey: "case-" + uuid.NewString(), RequestFingerprint: "fp-" + uuid.NewString(), BindingType: bindingType, BindingID: bindingID})
	if err != nil {
		t.Fatal("authorize case evidence", err)
	}
	object, err := f.service.CompleteUpload(context.Background(), usecases.CompleteUploadInput{Caller: caller, MediaObjectID: grant.MediaObjectID, AuthorizationID: grant.AuthorizationID, UploadToken: grant.UploadToken, DeclaredMIME: "application/pdf", DeclaredSize: int64(len(payload)), Content: payload})
	if err != nil || !object.SafetyConfirmed() {
		t.Fatal("complete case evidence", err)
	}
	return object.ID
}

func caseActorFor(scope adminpolicy.Scope, permissions ...string) adminmedia.Actor {
	return adminmedia.Actor{Product: "karryam", Staff: "staff-1", Roles: []string{"staff"}, Permissions: append([]string{adminmedia.Permission}, permissions...), Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}, ExpiresAt: time.Now().Add(5 * time.Minute)}
}

// Fleet is the fifth enrolled source owner. Its case kind and binding type
// come from admincase.Specs; grant, content and revocation reuse the existing
// private Media services, real PostgreSQL and real filesystem bytes.
func TestPostgresMediaFleetInvestigationEvidenceThroughExistingCaseServices(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	ctx := context.Background()
	spec, ok := admincase.Find("fleet-partner-service", "investigations")
	if !ok || !models.ValidBindingType(spec.BindingType) || !models.CaseOnlyBindingType(spec.BindingType) {
		t.Fatal("fleet investigation case vocabulary not recognized by Media")
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: testOrgID, Tenant: testOrgID}
	policy := casePolicy(t, scope, spec)
	caseRef, assignment := "INV-"+uuid.NewString(), "asn_"+strings.ReplaceAll(uuid.NewString(), "-", "")
	sourceKey := bytes.Repeat([]byte("f"), 32)
	source := &signedCaseSource{spec: spec, key: sourceKey, nonces: map[string]bool{}, open: map[string]bool{caseRef: true}, subjects: map[string]map[string]string{caseRef: {"ASSIGNMENT": assignment}}}
	sourceServer := httptest.NewServer(source.handler())
	defer sourceServer.Close()
	client, err := admincase.NewClient(spec.Owner, sourceServer.URL, sourceKey)
	if err != nil {
		t.Fatal("fleet case source client", err)
	}
	defer client.Close()
	payload := pdfFile(64)
	objectID := uploadCaseEvidence(t, f, spec.Owner, "ASSIGNMENT", assignment, "CARGO_CONDITION", spec.BindingType, caseRef, payload)
	// Case-registry binding types are case evidence and may never be public.
	if _, err := f.service.AuthorizeUpload(ctx, usecases.AuthorizeUploadInput{Caller: usecases.Caller{Service: spec.Owner, OrganizationID: testOrgID}, OwnerType: "ASSIGNMENT", OwnerReference: assignment, UploadPurpose: "INCIDENT_ATTACHMENT", Visibility: "PUBLIC", FileName: "x.pdf", AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 4096, IdempotencyKey: "public-" + uuid.NewString(), RequestFingerprint: "fp", BindingType: spec.BindingType, BindingID: caseRef}); err == nil {
		t.Fatal("public case evidence authorized")
	}
	authority := &usecases.OwnerCaseAuthority{Media: f.service, Policy: policy.Engine, Sources: map[string]usecases.CaseSource{spec.Owner: client}}
	access := f.service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: policy.Engine, Authority: authority})
	router := gin.New()
	mediaKey := bytes.Repeat([]byte("m"), 32)
	mediahttp.NewAdminCaseHandler(access, f.pool, mediaKey, true, true).Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	media, err := adminmedia.NewClient(server.URL, mediaKey)
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()
	actor := caseActorFor(scope, spec.Permission)
	request := adminmedia.Request{Scope: scope, Purpose: "case-1", SourceCapability: spec.Permission, CaseOwner: spec.Owner, CaseKind: spec.Kind, CaseReference: caseRef, SubjectType: "ASSIGNMENT", SubjectReference: assignment, MediaObjectID: objectID}
	grant, err := media.Issue(ctx, actor, request)
	if err != nil {
		t.Fatal("fleet case grant", err)
	}
	content, err := media.Read(ctx, actor, request, grant.ID, grant.Token)
	if err != nil || !bytes.Equal(content, payload) {
		t.Fatal("fleet case content", err)
	}
	caseRequest := request
	caseRequest.SubjectType, caseRequest.SubjectReference = "CASE", caseRef
	if _, err := media.Issue(ctx, actor, caseRequest); err == nil {
		t.Fatal("object owned by the assignment released under the CASE subject")
	}
	for _, name := range []string{"subject", "tenant", "case", "kind"} {
		bad := request
		switch name {
		case "subject":
			bad.SubjectReference = "asn_foreign"
		case "tenant":
			bad.Scope.Tenant = "foreign"
		case "case":
			bad.CaseReference = "INV-" + uuid.NewString()
		case "kind":
			bad.CaseKind = "recoveries"
		}
		if _, err := media.Issue(ctx, actor, bad); err == nil {
			t.Fatal("foreign fleet case evidence authorized", name)
		}
	}
	if err := media.Revoke(ctx, actor, grant.ID, "CASE_REVIEW_DONE"); err != nil {
		t.Fatal("revoke", err)
	}
	if body, err := media.Read(ctx, actor, request, grant.ID, grant.Token); err == nil || len(body) > 0 {
		t.Fatal("revoked fleet grant served content")
	}
	// Closing the Fleet case ends issuance and redemption of fresh grants.
	fresh, err := media.Issue(ctx, actor, request)
	if err != nil {
		t.Fatal(err)
	}
	source.mu.Lock()
	source.open[caseRef] = false
	source.mu.Unlock()
	if body, err := media.Read(ctx, actor, request, fresh.ID, fresh.Token); err == nil || len(body) > 0 {
		t.Fatal("closed fleet investigation served content")
	}
	if _, err := media.Issue(ctx, actor, request); err == nil {
		t.Fatal("closed fleet investigation minted a grant")
	}
}

// A case kind constructed only in this test proves Media's authority is
// driven by the case registry it is given, not by a hardcoded owner list.
func TestPostgresMediaCaseAuthorityFollowsRegistryForConstructedKind(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	ctx := context.Background()
	stub := admincase.Spec{Owner: "test-only-owner-service", Domain: "test-only", Kind: "test-only-cases", Permission: "test_only.case.evidence.read", SourceContract: "test_only.admin-case-evidence.v1", BindingType: models.BindingComplaint, Subjects: map[string][]string{"CASE": {"$CASE"}, "PARCEL": {"parcel"}}, States: []string{"OPEN"}, Purposes: []string{models.PurposeDisputeAttachment}}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: testOrgID, Tenant: testOrgID}
	policy := casePolicy(t, scope, stub)
	caseRef, parcel := "STUB-"+uuid.NewString(), "parcel-1"
	payload := pdfFile(8)
	objectID := uploadCaseEvidence(t, f, stub.Owner, "PARCEL", parcel, models.PurposeDisputeAttachment, stub.BindingType, caseRef, payload)
	source := &stubSource{spec: stub, open: map[string]bool{caseRef: true}}
	q := usecases.CaseRequest{Scope: scope, Purpose: "case-1", SourceCapability: stub.Permission, CaseOwner: stub.Owner, CaseKind: stub.Kind, CaseReference: caseRef, SubjectType: "PARCEL", SubjectReference: parcel, MediaObjectID: objectID}
	a := caseActorFor(scope, stub.Permission)
	actor := usecases.CaseActor{Delegation: a.Delegation(), ExpiresAt: a.ExpiresAt}
	registry := &usecases.OwnerCaseAuthority{Media: f.service, Policy: policy.Engine, Sources: map[string]usecases.CaseSource{stub.Owner: source}, Specs: append(admincase.Specs(), stub)}
	compiled := &usecases.OwnerCaseAuthority{Media: f.service, Policy: policy.Engine, Sources: map[string]usecases.CaseSource{stub.Owner: source}}
	if _, err := f.service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: policy.Engine, Authority: compiled}).Issue(ctx, actor, q); err == nil {
		t.Fatal("kind absent from the compiled registry authorized")
	}
	access := f.service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: policy.Engine, Authority: registry})
	grant, err := access.Issue(ctx, actor, q)
	if err != nil {
		t.Fatal("registry kind grant", err)
	}
	if body, err := access.Read(ctx, actor, q, grant.ID, grant.Token); err != nil || !bytes.Equal(body, payload) {
		t.Fatal("registry kind content", err)
	}
	if err := access.Revoke(ctx, actor, grant.ID, "DONE"); err != nil {
		t.Fatal(err)
	}
	if body, err := access.Read(ctx, actor, q, grant.ID, grant.Token); err == nil || len(body) > 0 {
		t.Fatal("revoked registry grant served")
	}
	source.open[caseRef] = false
	if _, err := access.Issue(ctx, actor, q); err == nil {
		t.Fatal("closed registry case authorized")
	}
}

type fileLegacyContent struct{ dir string }

func (s fileLegacyContent) Download(_ context.Context, path string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.dir, filepath.Clean("/"+path)))
}

func insertLegacyAsset(t *testing.T, f *mediaFixture, dir, ownerService, ownerID, purpose, status string, content []byte) string {
	t.Helper()
	id := uuid.NewString()
	path := "legacy/" + id
	if err := os.MkdirAll(filepath.Join(dir, "legacy"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, path), content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO media_assets(id,owner_service,owner_id,purpose,original_filename,content_type,size_bytes,checksum_sha256,storage_bucket,storage_path,public_url,status)VALUES($1::uuid,$2,$3,$4,'proof.pdf','application/pdf',$5,$6,'test-legacy',$7,'https://test.invalid/legacy',$8)`, id, ownerService, ownerID, purpose, len(content), hex.EncodeToString(sum[:]), path, status); err != nil {
		t.Fatal(err)
	}
	return id
}

// Owner-attested adoption maps one legacy Dispatch capture to one modern,
// case-bound, PRIVATE object built from the verified stored bytes.
func TestPostgresMediaLegacyAdoptionOwnerAttestedOneToOneMapping(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	ctx := context.Background()
	dir := t.TempDir()
	owner := "driver-dispatch-delivery-service"
	spec, _ := admincase.Find(owner, "delivery-recoveries")
	adoption := f.service.NewLegacyAdoption(usecases.LegacyAdoptionConfig{Content: fileLegacyContent{dir}, Bucket: "test-legacy"})
	ownerKey, otherKey := bytes.Repeat([]byte("d"), 32), bytes.Repeat([]byte("h"), 32)
	router := gin.New()
	mediahttp.NewLegacyAdoptionHandler(adoption, f.pool, serviceauth.Secrets{owner: ownerKey}, true).Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := adminmedia.NewLegacyAdoptionClient(owner, server.URL, ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	mission, trip, recovery := "mission-"+uuid.NewString(), uuid.NewString(), uuid.NewString()
	payload := pdfFile(128)
	legacyID := insertLegacyAsset(t, f, dir, "dispatch-delivery-service", mission, "proof_image", "active", payload)
	attest := adminmedia.LegacyAdoption{ContractVersion: adminmedia.LegacyAdoptionContract, LegacyAssetID: legacyID, LegacyOwnerReference: mission, OrganizationID: testOrgID, CaseKind: spec.Kind, CaseReference: recovery, SubjectType: "TRIP", SubjectReference: trip, Purpose: "CARGO_CONDITION"}
	receipt, err := client.Adopt(ctx, attest)
	if err != nil || receipt.Replayed {
		t.Fatal("adoption", err)
	}
	sum := sha256.Sum256(payload)
	object, err := f.service.GetObject(ctx, usecases.Caller{Service: owner, OrganizationID: testOrgID}, receipt.MediaObjectID)
	if err != nil || object.ChecksumSHA256 != hex.EncodeToString(sum[:]) || receipt.ChecksumSHA256 != object.ChecksumSHA256 || object.BindingType != spec.BindingType || object.BindingID != recovery || object.OwnerType != "TRIP" || object.OwnerReference != trip || object.Visibility != "PRIVATE" || !object.SafetyConfirmed() || object.UploadPurpose != "CARGO_CONDITION" {
		t.Fatal("adopted object", err, object)
	}
	stored, err := f.storage.Get(ctx, object.StorageObjectKey)
	if err != nil || !bytes.Equal(stored, payload) {
		t.Fatal("adopted bytes differ from verified legacy bytes", err)
	}
	replay, err := client.Adopt(ctx, attest)
	if err != nil || !replay.Replayed || replay.MediaObjectID != receipt.MediaObjectID {
		t.Fatal("identical attestation not idempotent", err)
	}
	conflict := attest
	conflict.CaseReference = uuid.NewString()
	if _, err := client.Adopt(ctx, conflict); !errors.Is(err, adminmedia.ErrConflict) {
		t.Fatal("conflicting attestation accepted", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE media_legacy_adoptions SET case_reference='x'`); err == nil {
		t.Fatal("adoption mapping mutable")
	}
	var mappings int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM media_legacy_adoptions WHERE legacy_asset_id=$1::uuid`, legacyID).Scan(&mappings)
	if mappings != 1 {
		t.Fatal("mapping not one-to-one", mappings)
	}

	deny := func(name string, mutate func(*adminmedia.LegacyAdoption) string, want error) {
		t.Run(name, func(t *testing.T) {
			a := attest
			a.LegacyAssetID = mutate(&a)
			if _, err := client.Adopt(ctx, a); !errors.Is(err, want) {
				t.Fatalf("%s: got %v want %v", name, err, want)
			}
			var count int
			f.pool.QueryRow(ctx, `SELECT count(*) FROM media_legacy_adoptions WHERE legacy_asset_id=$1::uuid`, a.LegacyAssetID).Scan(&count)
			if count != 0 {
				t.Fatal("denied adoption recorded a mapping")
			}
		})
	}
	deny("wrong-owner-asset", func(a *adminmedia.LegacyAdoption) string {
		return insertLegacyAsset(t, f, dir, "hauling-service", mission, "proof_image", "active", payload)
	}, adminmedia.ErrDenied)
	deny("wrong-owner-reference", func(a *adminmedia.LegacyAdoption) string {
		id := insertLegacyAsset(t, f, dir, "dispatch-delivery-service", mission, "proof_image", "active", payload)
		a.LegacyOwnerReference = "mission-other"
		return id
	}, adminmedia.ErrDenied)
	for _, status := range []string{"revoked", "quarantined", "deleted"} {
		status := status
		deny("legacy-"+status, func(a *adminmedia.LegacyAdoption) string {
			return insertLegacyAsset(t, f, dir, "dispatch-delivery-service", mission, "proof_image", status, payload)
		}, adminmedia.ErrDenied)
	}
	deny("soft-deleted", func(a *adminmedia.LegacyAdoption) string {
		id := insertLegacyAsset(t, f, dir, "dispatch-delivery-service", mission, "proof_image", "active", payload)
		if _, err := f.pool.Exec(ctx, `UPDATE media_assets SET deleted_at=now() WHERE id=$1::uuid`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}, adminmedia.ErrDenied)
	deny("unmapped-legacy-purpose", func(a *adminmedia.LegacyAdoption) string {
		return insertLegacyAsset(t, f, dir, "dispatch-delivery-service", mission, "signature", "active", payload)
	}, adminmedia.ErrDenied)
	deny("tampered-bytes", func(a *adminmedia.LegacyAdoption) string {
		id := insertLegacyAsset(t, f, dir, "dispatch-delivery-service", mission, "proof_image", "active", payload)
		if err := os.WriteFile(filepath.Join(dir, "legacy", id), pdfFile(129), 0o600); err != nil {
			t.Fatal(err)
		}
		return id
	}, adminmedia.ErrDenied)
	deny("unknown-legacy-asset", func(a *adminmedia.LegacyAdoption) string { return uuid.NewString() }, adminmedia.ErrNotFound)

	// Only the owner itself, with its own adoption key, can attest.
	foreign, _ := adminmedia.NewLegacyAdoptionClient("driver-hauling-service", server.URL, otherKey)
	other := attest
	other.LegacyAssetID = insertLegacyAsset(t, f, dir, "dispatch-delivery-service", mission, "proof_image", "active", payload)
	other.CaseKind = "recoveries"
	other.SubjectType = "TRUCK"
	if _, err := foreign.Adopt(ctx, other); !errors.Is(err, adminmedia.ErrDenied) {
		t.Fatal("non-owner attestation accepted", err)
	}
	// A captured signed attestation cannot be replayed.
	fresh := attest
	fresh.LegacyAssetID = other.LegacyAssetID
	raw, _ := json.Marshal(fresh)
	signed, _ := http.NewRequest("POST", server.URL+adminmedia.LegacyAdoptionPath, bytes.NewReader(raw))
	signed.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(signed, owner, ownerKey, raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	header := signed.Header.Clone()
	first, err := http.DefaultClient.Do(signed)
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	again, _ := http.NewRequest("POST", server.URL+adminmedia.LegacyAdoptionPath, bytes.NewReader(raw))
	again.Header = header
	second, err := http.DefaultClient.Do(again)
	if err != nil {
		t.Fatal(err)
	}
	second.Body.Close()
	if first.StatusCode != 200 || second.StatusCode != 409 {
		t.Fatal("adoption nonce replay", first.StatusCode, second.StatusCode)
	}

	// The admin case read path reaches the mapped modern object only; a legacy
	// asset ID, mapped or not, is never a modern object identity.
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: testOrgID, Tenant: testOrgID}
	policy := casePolicy(t, scope, spec)
	source := &stubSource{spec: spec, open: map[string]bool{recovery: true}}
	authority := &usecases.OwnerCaseAuthority{Media: f.service, Policy: policy.Engine, Sources: map[string]usecases.CaseSource{owner: source}}
	access := f.service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: policy.Engine, Authority: authority})
	a := caseActorFor(scope, spec.Permission)
	actor := usecases.CaseActor{Delegation: a.Delegation(), ExpiresAt: a.ExpiresAt}
	q := usecases.CaseRequest{Scope: scope, Purpose: "case-1", SourceCapability: spec.Permission, CaseOwner: owner, CaseKind: spec.Kind, CaseReference: recovery, SubjectType: "TRIP", SubjectReference: trip, MediaObjectID: receipt.MediaObjectID}
	grant, err := access.Issue(ctx, actor, q)
	if err != nil {
		t.Fatal("mapped modern object unavailable to case read", err)
	}
	if body, err := access.Read(ctx, actor, q, grant.ID, grant.Token); err != nil || !bytes.Equal(body, payload) {
		t.Fatal("mapped content", err)
	}
	unmapped := insertLegacyAsset(t, f, dir, "dispatch-delivery-service", mission, "proof_image", "active", payload)
	for _, legacy := range []string{legacyID, unmapped} {
		bad := q
		bad.MediaObjectID = legacy
		if _, err := access.Issue(ctx, actor, bad); err == nil {
			t.Fatal("legacy asset ID used as modern object identity", legacy)
		}
	}
	if id, err := adoption.LegacyMapping(ctx, unmapped); err == nil || id != "" {
		t.Fatal("unmapped legacy asset resolved")
	}
}
