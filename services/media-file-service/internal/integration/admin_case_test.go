package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/media-file-service/internal/database"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	mediarepositories "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/repositories"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
)

// This TEST_ONLY authority isolates native Media state-machine failures. The
// native Customer process integration is tested separately without this stub.
type testCaseAuthority struct {
	proof  usecases.CaseProof
	denied bool
}

func (a *testCaseAuthority) AuthorizeCase(_ context.Context, _ usecases.CaseActor, _ usecases.CaseRequest) (usecases.CaseProof, error) {
	if a.denied {
		return usecases.CaseProof{}, adminpolicy.ErrDenied
	}
	return a.proof, nil
}

type caseFixture struct {
	media     *mediaFixture
	access    *usecases.CaseAccessService
	authority *testCaseAuthority
	policy    *policyfixture.Fixture
	actor     usecases.CaseActor
	request   usecases.CaseRequest
}

func newCaseFixture(t *testing.T) *caseFixture {
	t.Helper()
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	view := f.uploadEvidence(t, models.PurposeRecoveryTransfer, models.BindingMission, "mission-case-native", nil)
	id := view["mediaObjectId"].(string)
	object, err := f.service.GetObject(context.Background(), usecases.Caller{Service: testCaller, OrganizationID: testOrgID}, id)
	if err != nil {
		t.Fatal(err)
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: testOrgID, Tenant: testOrgID}
	q := usecases.CaseRequest{Scope: scope, Purpose: "case-1", SourceCapability: "fleet.media.case.read", CaseOwner: "fleet-partner-service", CaseKind: "TEST_ONLY_RECOVERY_CASE", CaseReference: "TEST_ONLY_CASE_1", SubjectType: object.OwnerType, SubjectReference: object.OwnerReference, MediaObjectID: id}
	p := policyfixture.New(t, scope, q.CaseOwner, q.SourceCapability, "fleet.admin-case-binding.v1")
	p.Document.ApprovedOwners = append(p.Document.ApprovedOwners, "media-file-service")
	mediaRule := p.Document.Rules[0]
	mediaRule.Owner = "media-file-service"
	mediaRule.Capability = usecases.CaseContentPermission
	mediaRule.Permission = usecases.CaseContentPermission
	mediaRule.SourceContract = usecases.CaseContentContract
	p.Document.Rules = append(p.Document.Rules, mediaRule)
	p.Document.Media = &adminpolicy.MediaDisclosure{MaxGrantSeconds: 60, SourceCapabilities: []string{q.SourceCapability}} // TEST_ONLY value, not production approval.
	p.Sign(t)
	actor := usecases.CaseActor{Delegation: adminpolicy.Actor{Product: "karryam", Staff: "staff-1", Roles: []string{"staff"}, Permissions: []string{q.SourceCapability, usecases.CaseContentPermission}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}}, ExpiresAt: f.now.Add(5 * time.Minute)}
	a := &testCaseAuthority{proof: usecases.CaseProof{Facts: adminpolicy.Facts{Owner: q.CaseOwner, Reference: q.CaseReference, SourceContract: "fleet.admin-case-binding.v1", Scope: scope}, CaseKind: q.CaseKind, SubjectType: q.SubjectType, SubjectReference: q.SubjectReference, MediaObjectID: id, ObjectOwnerService: object.OwnerService, ObjectOwnerType: object.OwnerType, ObjectOwnerReference: object.OwnerReference, ObjectPurpose: object.UploadPurpose, BindingType: models.BindingMission, BindingID: "mission-case-native", NativeFingerprint: admincontext.Fingerprint("TEST_ONLY_NATIVE_CASE_RECORD"), ExpiresAt: f.now.Add(30 * time.Minute)}}
	access := f.service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: p.Engine, Authority: a})
	return &caseFixture{media: f, access: access, authority: a, policy: p, actor: actor, request: q}
}
func (f *caseFixture) issue(t *testing.T) *usecases.CaseGrant {
	t.Helper()
	g, err := f.access.Issue(context.Background(), f.actor, f.request)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestPostgresMediaCaseGrantCannotOutliveSignedDisclosurePolicy(t *testing.T) {
	f := newCaseFixture(t)
	f.policy.Document.ExpiresAt = f.media.now.Add(25 * time.Second)
	f.policy.Sign(t)
	f.access = f.media.service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: f.policy.Engine, Authority: f.authority})
	grant := f.issue(t)
	if grant.ExpiresAt.After(f.policy.Document.ExpiresAt) {
		t.Fatal("grant outlived signed disclosure policy")
	}
}
func (f *caseFixture) read(t *testing.T, g *usecases.CaseGrant) {
	t.Helper()
	b, err := f.access.Read(context.Background(), f.actor, f.request, g.ID, g.Token)
	if err != nil || len(b) == 0 {
		t.Fatal("authorized case bytes unavailable", err)
	}
}
func TestPostgresMediaCaseGrantBindsStaffCaseSubjectScopeAndNativeObject(t *testing.T) {
	f := newCaseFixture(t)
	g := f.issue(t)
	f.read(t, g)
	if g.ExpiresAt.After(f.media.now.Add(60 * time.Second)) {
		t.Fatal("unsigned/default grant lifetime")
	}
	var persisted string
	if err := f.media.pool.QueryRow(context.Background(), `SELECT row_to_json(g)::text FROM media_admin_case_grants g WHERE id=$1`, g.ID).Scan(&persisted); err != nil || strings.Contains(persisted, g.Token) {
		t.Fatal("raw case credential persisted", err)
	}
	var audits string
	if err := f.media.pool.QueryRow(context.Background(), `SELECT COALESCE(string_agg(new_summary::text,''),'') FROM media_audit_records`).Scan(&audits); err != nil || strings.Contains(audits, g.Token) {
		t.Fatal("case credential entered audit", err)
	}
	// A case token must never become an ambient owner credential.
	if b, _, err := f.media.service.ReadContent(context.Background(), f.request.MediaObjectID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("case token bypassed owner download namespace")
	}
	for _, name := range []string{"staff", "product", "source permission", "media permission", "role", "purpose", "subject", "case", "object", "zone", "tenant", "token"} {
		t.Run(name, func(t *testing.T) {
			actor, q, token := f.actor, f.request, g.Token
			switch name {
			case "staff":
				actor.Delegation.Staff = "staff-2"
			case "product":
				actor.Delegation.Product = "other"
			case "source permission":
				actor.Delegation.Permissions = []string{usecases.CaseContentPermission}
			case "media permission":
				actor.Delegation.Permissions = []string{q.SourceCapability}
			case "role":
				actor.Delegation.Roles = []string{"other"}
			case "purpose":
				q.Purpose = "CASE-123"
			case "subject":
				q.SubjectReference = "other-subject"
			case "case":
				q.CaseReference = "other-case"
			case "object":
				q.MediaObjectID = "mobj_other"
			case "zone":
				q.Scope.Zone = "Z2"
			case "tenant":
				q.Scope.Tenant = "other-tenant"
			case "token":
				token = strings.Repeat("x", 43)
			}
			if b, err := f.access.Read(context.Background(), actor, q, g.ID, token); err == nil || len(b) > 0 {
				t.Fatal("case grant boundary bypass", name)
			}
		})
	}
	f.authority.denied = true
	if b, err := f.access.Read(context.Background(), f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("closed/revoked source case served bytes")
	}
	f.authority.denied = false
	f.authority.proof.NativeFingerprint = admincontext.Fingerprint("TEST_ONLY_CHANGED_CASE_RECORD")
	if b, err := f.access.Read(context.Background(), f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("changed source binding served old grant")
	}
}
func TestPostgresMediaCaseCurrentNativeSafetyBindingAndRevocationFloor(t *testing.T) {
	f := newCaseFixture(t)
	g := f.issue(t)
	ctx := context.Background()
	for _, state := range []string{models.SafetyUnknown, models.SafetyPending, models.SafetyUnsafe, models.SafetyQuarantined} {
		t.Run(state, func(t *testing.T) {
			if _, err := f.media.pool.Exec(ctx, `UPDATE media_objects SET safety_status=$2 WHERE id=$1`, g.MediaObjectID, state); err != nil {
				t.Fatal(err)
			}
			if _, err := f.access.Issue(ctx, f.actor, f.request); err == nil {
				t.Fatal("unconfirmed case file authorized")
			}
			if b, err := f.access.Read(ctx, f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
				t.Fatal("old token bypassed current safety")
			}
			if _, err := f.media.pool.Exec(ctx, `UPDATE media_objects SET safety_status='SAFE' WHERE id=$1`, g.MediaObjectID); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, field := range []string{"organization_id", "owner_service", "owner_reference", "binding_id"} {
		t.Run(field, func(t *testing.T) {
			var before string
			if err := f.media.pool.QueryRow(ctx, "SELECT "+field+" FROM media_objects WHERE id=$1", g.MediaObjectID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err := f.media.pool.Exec(ctx, "UPDATE media_objects SET "+field+"='FOREIGN_NATIVE_VALUE' WHERE id=$1", g.MediaObjectID); err != nil {
				t.Fatal(err)
			}
			if b, err := f.access.Read(ctx, f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
				t.Fatal("native owner/tenant/binding change accepted")
			}
			if _, err := f.media.pool.Exec(ctx, "UPDATE media_objects SET "+field+"=$2 WHERE id=$1", g.MediaObjectID, before); err != nil {
				t.Fatal(err)
			}
		})
	}
	object, err := f.media.service.GetObject(ctx, usecases.Caller{Service: testCaller, OrganizationID: testOrgID}, g.MediaObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.media.service.RevokeEvidence(ctx, usecases.RevokeInput{Caller: usecases.Caller{Service: testCaller, OrganizationID: testOrgID}, MediaObjectID: object.ID, ExpectedVersion: object.Version, Reason: "TEST_ONLY_REVOCATION"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.access.Issue(ctx, f.actor, f.request); err == nil {
		t.Fatal("revoked modern object authorized")
	}
	if b, err := f.access.Read(ctx, f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("revoked modern object redeemed")
	}
}
func TestPostgresMediaCaseAuditAtomicityImmutabilityAndRevocationAfterKill(t *testing.T) {
	f := newCaseFixture(t)
	ctx := context.Background()
	fail := func() {
		t.Helper()
		if _, err := f.media.pool.Exec(ctx, `CREATE FUNCTION test_case_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'TEST_ONLY_AUDIT_FAILURE'; END $$;CREATE TRIGGER test_case_audit_failure BEFORE INSERT ON media_audit_records FOR EACH STATEMENT EXECUTE FUNCTION test_case_audit_failure()`); err != nil {
			t.Fatal(err)
		}
	}
	restore := func() {
		t.Helper()
		if _, err := f.media.pool.Exec(ctx, `DROP TRIGGER test_case_audit_failure ON media_audit_records;DROP FUNCTION test_case_audit_failure()`); err != nil {
			t.Fatal(err)
		}
	}
	fail()
	if _, err := f.access.Issue(ctx, f.actor, f.request); err == nil {
		t.Fatal("grant without immutable audit")
	}
	var n int
	if err := f.media.pool.QueryRow(ctx, `SELECT count(*) FROM media_admin_case_grants`).Scan(&n); err != nil || n != 0 {
		t.Fatal("failed grant persisted", n, err)
	}
	restore()
	g := f.issue(t)
	fail()
	if b, err := f.access.Read(ctx, f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("bytes released after audit failure")
	}
	if err := f.access.Revoke(ctx, f.actor, g.ID, "TEST_ONLY_WITHDRAWAL"); err == nil {
		t.Fatal("unaudited revocation")
	}
	if err := f.media.pool.QueryRow(ctx, `SELECT count(*) FROM media_admin_case_revocations`).Scan(&n); err != nil || n != 0 {
		t.Fatal("failed revoke committed", err)
	}
	restore()
	f.read(t, g)
	killed := f.media.service.NewCaseAccess(usecases.CaseAccessConfig{})
	if b, err := killed.Read(ctx, f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("kill switch did not deny bytes")
	}
	f.authority.denied = true
	if err := killed.Revoke(ctx, f.actor, g.ID, "TEST_ONLY_WITHDRAWAL"); err != nil {
		t.Fatal("own grant could not be withdrawn after source closure/read kill", err)
	}
	f.authority.denied = false
	if b, err := f.access.Read(ctx, f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("revoked grant redeemed")
	}
	for _, sql := range []string{`UPDATE media_admin_case_grants SET staff_ref='rewrite'`, `DELETE FROM media_admin_case_grants`, `TRUNCATE media_admin_case_grants CASCADE`, `UPDATE media_admin_case_revocations SET reason_code='rewrite'`, `DELETE FROM media_admin_case_revocations`, `TRUNCATE media_admin_case_revocations`} {
		if _, err := f.media.pool.Exec(ctx, sql); err == nil {
			t.Fatal("grant evidence mutable")
		}
	}
	if err := database.RollbackLastMigration(ctx, f.media.pool, migrationsDir); err == nil {
		t.Fatal("populated case evidence destroyed by rollback")
	}
}
func TestPostgresMediaCaseExpiryAndPolicyRollbackCannotRestoreAccess(t *testing.T) {
	f := newCaseFixture(t)
	ctx := context.Background()
	f.actor.ExpiresAt = f.media.now.Add(20 * time.Second)
	g := f.issue(t)
	if g.ExpiresAt.After(f.actor.ExpiresAt) {
		t.Fatal("grant outlived delegated staff assertion")
	}
	oldPolicy := f.policy.Engine
	f.policy.Document.Version = 2
	f.policy.Document.Media.MaxGrantSeconds = 10
	f.policy.Sign(t)
	newer := f.media.service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: f.policy.Engine, Authority: f.authority})
	if b, err := newer.Read(ctx, f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("old policy grant survived accepted revision")
	}
	restartedOld := f.media.service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: oldPolicy, Authority: f.authority})
	if _, err := restartedOld.Issue(ctx, f.actor, f.request); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatal("policy rollback resurrected access", err)
	}
	*f.media.now = f.media.now.Add(21 * time.Second)
	if b, err := f.access.Read(ctx, f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("expired staff assertion served bytes")
	}
}
func TestMediaCaseAccessOffOrMissingAuthorityCannotMintGrant(t *testing.T) {
	service := usecases.NewService(usecases.Options{})
	for _, cfg := range []usecases.CaseAccessConfig{{}, {Enabled: true}} {
		access := service.NewCaseAccess(cfg)
		if _, err := access.Issue(context.Background(), usecases.CaseActor{}, usecases.CaseRequest{}); !errors.Is(err, adminpolicy.ErrBlocked) {
			t.Fatal("missing case policy became authority", err)
		}
	}
	var policy adminpolicy.Engine
	if _, err := policy.MediaDisclosure(time.Now()); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatal("missing signed Media values acquired defaults")
	}
	// Assert requests persist no opaque actor assertion or secret transport data.
	if raw, err := json.Marshal(usecases.CaseRequest{}); err != nil || strings.Contains(string(raw), "token") {
		t.Fatal(err)
	}
}

// Embedding retains the real filesystem implementation for every operation.
type pausedCaseStorage struct {
	uploadclients.PrivateStorage
	entered, release chan struct{}
}

func (s *pausedCaseStorage) Get(ctx context.Context, key string) ([]byte, error) {
	close(s.entered)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
	}
	return s.PrivateStorage.Get(ctx, key)
}
func TestPostgresMediaCaseRedemptionAndRevocationSerializeOnOriginalGrant(t *testing.T) {
	f := newCaseFixture(t)
	storage := &pausedCaseStorage{PrivateStorage: f.media.storage, entered: make(chan struct{}), release: make(chan struct{})}
	native := usecases.NewService(usecases.Options{Pool: f.media.pool, Objects: mediarepositories.NewRepository(f.media.pool), Storage: storage, Now: func() time.Time { return *f.media.now }})
	access := native.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: f.policy.Engine, Authority: f.authority})
	g, err := access.Issue(context.Background(), f.actor, f.request)
	if err != nil {
		t.Fatal(err)
	}
	read := make(chan error, 1)
	revoked := make(chan error, 1)
	go func() {
		b, err := access.Read(context.Background(), f.actor, f.request, g.ID, g.Token)
		if err == nil && len(b) == 0 {
			err = errors.New("missing authorized bytes")
		}
		read <- err
	}()
	select {
	case <-storage.entered:
	case <-time.After(time.Second):
		t.Fatal("redemption did not reach real storage")
	}
	go func() { revoked <- access.Revoke(context.Background(), f.actor, g.ID, "TEST_ONLY_CONCURRENT_REVOKE") }()
	select {
	case err := <-revoked:
		t.Fatal("revocation raced the in-flight grant lock", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(storage.release)
	if err := <-read; err != nil {
		t.Fatal("authorized in-flight read failed", err)
	}
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	if b, err := access.Read(context.Background(), f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("post-revocation read escaped")
	}
}
