package crm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// Section 8H export tests.
//
// The artifact store is a deterministic in-process fake rather than a real
// object store. That is deliberate and is what §103 asks for: the properties
// under test are Vendor's — what becomes READY, what is released, what is
// deleted — and reaching a network service to prove them would make the test
// measure the wrong thing while still not exercising production credentials.

// fakeArtifactStore records exactly what Vendor asked it to hold.
type fakeArtifactStore struct {
	mu       sync.Mutex
	objects  map[string][]byte
	types    map[string]string
	deleted  map[string]string
	puts     int
	failPut  bool
	failAcc  bool
	accessed []string
}

func newFakeArtifactStore() *fakeArtifactStore {
	return &fakeArtifactStore{objects: map[string][]byte{}, types: map[string]string{},
		deleted: map[string]string{}}
}

func (f *fakeArtifactStore) Put(ctx context.Context, in PutArtifactInput) (StoredArtifact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPut {
		return StoredArtifact{}, fmt.Errorf("storage unavailable")
	}
	f.puts++
	// Keyed on the idempotency key, mirroring the real store: a retried
	// generation reuses one object instead of orphaning the first.
	ref := "media_" + in.IdempotencyKey
	f.objects[ref] = append([]byte(nil), in.Content...)
	f.types[ref] = in.MediaType
	delete(f.deleted, ref)
	sum := sha256.Sum256(in.Content)
	return StoredArtifact{Ref: ref, Bytes: int64(len(in.Content)), Checksum: hex.EncodeToString(sum[:])}, nil
}

func (f *fakeArtifactStore) Access(ctx context.Context, businessID, ref string) (ArtifactAccess, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAcc {
		return ArtifactAccess{}, fmt.Errorf("storage unavailable")
	}
	if _, ok := f.objects[ref]; !ok {
		return ArtifactAccess{}, fmt.Errorf("no such object")
	}
	f.accessed = append(f.accessed, ref)
	return ArtifactAccess{Ref: ref, Path: "/private/" + ref, Token: "tok_" + ref,
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute), MediaType: f.types[ref],
		Bytes: int64(len(f.objects[ref]))}, nil
}

func (f *fakeArtifactStore) Delete(ctx context.Context, businessID, ref, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, ref)
	f.deleted[ref] = reason
	return nil
}

func (f *fakeArtifactStore) contentOf(ref string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	content, ok := f.objects[ref]
	return string(content), ok
}

// fakeStepUp is the Section 5 elevation, with the two failure modes that
// matter: a grant that was never issued and one that has been spent.
type fakeStepUp struct {
	mu      sync.Mutex
	valid   map[string]string
	spent   map[string]bool
	refuse  bool
	granted []string
}

func newFakeStepUp() *fakeStepUp {
	return &fakeStepUp{valid: map[string]string{}, spent: map[string]bool{}}
}

func (f *fakeStepUp) issue(grantID, purpose string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.valid[grantID] = purpose
}

func (f *fakeStepUp) ConsumeGrant(ctx context.Context, grantID, sessionID, staffID, vendorID, purpose string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse {
		return fmt.Errorf("step-up refused")
	}
	want, ok := f.valid[grantID]
	if !ok || f.spent[grantID] {
		return fmt.Errorf("no usable grant")
	}
	if want != purpose {
		// A grant taken for one purpose cannot be spent on another.
		return fmt.Errorf("grant purpose mismatch")
	}
	f.spent[grantID] = true
	f.granted = append(f.granted, purpose+":"+grantID)
	return nil
}

// exportEnv wires a service with the two Section 8H ports installed.
func newExportEnv(t *testing.T) (*testEnv, *fakeArtifactStore, *fakeStepUp) {
	t.Helper()
	env := newTestEnv(t, allFlagsOn())
	store := newFakeArtifactStore()
	stepUp := newFakeStepUp()
	env.service.artifacts = store
	env.service.stepUp = stepUp
	return env, store, stepUp
}

func (e *testEnv) createExport(t *testing.T, store *fakeStepUp, in CreateExportInput) Export {
	t.Helper()
	grant := "grant_" + newID("")
	store.issue(grant, StepUpPurposeExportCreate)
	in.StepUpGrantID = grant
	in.SessionID = "session_test"
	export, err := e.service.CreateExport(context.Background(), actorFor(businessA, "vs_a"),
		globalScope(), in)
	if err != nil {
		t.Fatalf("create export %s: %v", in.ExportType, err)
	}
	return export
}

func (e *testEnv) generateExports(t *testing.T) {
	t.Helper()
	if _, err := e.service.RunExportGenerator(context.Background(), "test-export-worker", 10); err != nil {
		t.Fatalf("generate: %v", err)
	}
}

func (e *testEnv) reloadExport(t *testing.T, exportID string) Export {
	t.Helper()
	export, err := e.loadExportRaw(exportID)
	if err != nil {
		t.Fatalf("reload export: %v", err)
	}
	return export
}

func (e *testEnv) loadExportRaw(exportID string) (Export, error) {
	return scanExport(e.pool.QueryRow(context.Background(),
		`SELECT `+exportColumns+` FROM crm_exports WHERE id=$1`, exportID))
}

// seedExportFixture builds a tenant with enough of every domain that all six
// datasets have something real to say.
func (e *testEnv) seedExportFixture(t *testing.T) []Profile {
	t.Helper()
	var profiles []Profile
	for index := 0; index < 8; index++ {
		profiles = append(profiles, e.scoredCustomer(t, businessA, "vs_a",
			fmt.Sprintf("cus_exp%d", index), 3+index, int64(50000*(index+1)),
			300*24*time.Hour, time.Duration(index+1)*15*24*time.Hour))
	}
	e.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
	e.runIntelligence(t, businessA, "vs_a", ModelChurn, IntelligenceScopeBusiness, "")
	return profiles
}

// TestExports_EveryDatasetGeneratesAndCarriesNoPersonalData is the field-safety
// guard, run against real generated bytes rather than against the descriptor.
func TestExports_EveryDatasetGeneratesAndCarriesNoPersonalData(t *testing.T) {
	env, store, stepUp := newExportEnv(t)
	ctx := context.Background()
	profiles := env.seedExportFixture(t)

	segment := env.intelligenceSegment(t, businessA, "vs_a", "Export audience",
		`{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"commercial.completed_order_count","operator":"GTE","value":1}]}}`)
	env.materialize(t)

	corporate := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	env.addCorporateContact(t, businessA, "vs_a", corporate.ID, profiles[0].ID, ContactRoleProcurement, true)

	for _, exportType := range AllExportTypes {
		filter := ExportFilter{}
		switch exportType {
		case ExportSegmentMembers:
			filter.SegmentRef = segment.ID
		case ExportIntelligenceScores:
			filter.ModelKey = ModelRFM
		}
		for _, format := range []string{FormatCSV, FormatJSONL} {
			export := env.createExport(t, stepUp, CreateExportInput{
				ExportType: exportType, Format: format,
				ScopeType: IntelligenceScopeBusiness, Filter: filter})
			env.generateExports(t)
			export = env.reloadExport(t, export.ID)
			if export.Status != ExportReady {
				t.Fatalf("%s/%s finished as %s (error=%v)", exportType, format,
					export.Status, export.ErrorCode)
			}
			if export.ArtifactRef == nil || export.Checksum == nil || export.ArtifactBytes == nil {
				t.Fatalf("%s/%s is READY without a complete artifact: %+v", exportType, format, export)
			}
			content, ok := store.contentOf(*export.ArtifactRef)
			if !ok {
				t.Fatalf("%s/%s has no stored bytes", exportType, format)
			}
			// The checksum in the row describes the bytes the store holds.
			sum := sha256.Sum256([]byte(content))
			if hex.EncodeToString(sum[:]) != *export.Checksum {
				t.Fatalf("%s/%s checksum does not match the artifact", exportType, format)
			}

			// No canonical personal data, in any dataset, in either format.
			lowered := strings.ToLower(content)
			for _, forbidden := range []string{
				"@", "email", "phone", "msisdn", "date_of_birth", "password", "otp",
				"government_id", "device_token", "push_token", "card_number",
				"account_number", "home_address",
			} {
				if strings.Contains(lowered, forbidden) {
					t.Fatalf("%s/%s export contains %q:\n%s", exportType, format, forbidden,
						truncateForTest(content))
				}
			}
			// And no free-text CRM content.
			for _, forbidden := range []string{"note_body", "description", "message_body", "reason_text"} {
				if strings.Contains(lowered, forbidden) {
					t.Fatalf("%s/%s export contains free-text field %q", exportType, format, forbidden)
				}
			}
		}
	}
	_ = ctx
}

func truncateForTest(value string) string {
	if len(value) > 600 {
		return value[:600] + "..."
	}
	return value
}

// TestExports_ReadyOnlyAfterCompleteGeneration proves a partial artifact can
// never be released, and that the row count is reconciled rather than assumed.
func TestExports_ReadyOnlyAfterCompleteGeneration(t *testing.T) {
	env, store, stepUp := newExportEnv(t)
	env.seedExportFixture(t)

	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	if export.Status != ExportQueued {
		t.Fatalf("a new export is %s; want QUEUED", export.Status)
	}
	if export.RowCount != 0 || export.ArtifactRef != nil {
		t.Fatalf("a queued export already claims an artifact: %+v", export)
	}

	// A store that refuses leaves the job unfinished and nothing downloadable.
	store.failPut = true
	env.generateExports(t)
	failed := env.reloadExport(t, export.ID)
	if failed.Status == ExportReady {
		t.Fatal("an export became READY while its artifact could not be stored")
	}
	if failed.ArtifactRef != nil {
		t.Fatalf("a failed export references an artifact: %+v", failed)
	}

	// Once storage works, the job completes with a reconciled row count.
	store.failPut = false
	if _, err := env.pool.Exec(context.Background(), `UPDATE crm_exports
		SET next_attempt_at=now() WHERE id=$1`, export.ID); err != nil {
		t.Fatal(err)
	}
	env.generateExports(t)
	ready := env.reloadExport(t, export.ID)
	if ready.Status != ExportReady {
		t.Fatalf("export finished as %s (error=%v)", ready.Status, ready.ErrorCode)
	}
	content, _ := store.contentOf(*ready.ArtifactRef)
	dataRows := int64(strings.Count(strings.TrimRight(content, "\n"), "\n")) // minus the header
	if ready.RowCount != dataRows {
		t.Fatalf("row_count=%d but the file holds %d data rows", ready.RowCount, dataRows)
	}
	if ready.ExpiresAt == nil || !ready.ExpiresAt.After(*ready.CompletedAt) {
		t.Fatalf("a READY export has no future expiry: %+v", ready)
	}
}

// TestExports_RowLimitRefusesRatherThanTruncates. A short file that looks
// complete is a wrong answer nobody would notice.
func TestExports_RowLimitRefusesRatherThanTruncates(t *testing.T) {
	env, _, stepUp := newExportEnv(t)
	env.seedExportFixture(t)
	env.service.exports.MaxAttempts = 1

	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatCSV,
		ScopeType: IntelligenceScopeBusiness, RowLimit: 3})
	env.generateExports(t)
	result := env.reloadExport(t, export.ID)
	if result.Status == ExportReady {
		t.Fatalf("an over-limit export was published with %d rows", result.RowCount)
	}
	if result.ErrorCode == nil || *result.ErrorCode != CodeExportTooLarge {
		t.Fatalf("error code = %v; want %s", result.ErrorCode, CodeExportTooLarge)
	}

	// Asking for more than the hard ceiling is refused at request time.
	grant := "grant_toolarge"
	stepUp.issue(grant, StepUpPurposeExportCreate)
	if _, err := env.service.CreateExport(context.Background(), actorFor(businessA, "vs_a"), globalScope(),
		CreateExportInput{ExportType: ExportCustomerSummary, Format: FormatCSV,
			ScopeType: IntelligenceScopeBusiness, RowLimit: 999999,
			StepUpGrantID: grant, SessionID: "s"}); codeOf(t, err) != CodeExportTooLarge {
		t.Fatalf("an over-ceiling row limit was accepted (err=%v)", err)
	}
}

// TestExports_StepUpIsRequiredForCreationAndDownload is the elevation contract.
func TestExports_StepUpIsRequiredForCreationAndDownload(t *testing.T) {
	env, _, stepUp := newExportEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedExportFixture(t)

	base := CreateExportInput{ExportType: ExportCustomerSummary, Format: FormatCSV,
		ScopeType: IntelligenceScopeBusiness, SessionID: "session_test"}

	// No grant at all.
	if _, err := env.service.CreateExport(ctx, actor, globalScope(), base); codeOf(t, err) != CodeExportStepUpRequired {
		t.Fatalf("an export was created without step-up (err=%v)", err)
	}
	// An unknown grant.
	unknown := base
	unknown.StepUpGrantID = "grant_never_issued"
	if _, err := env.service.CreateExport(ctx, actor, globalScope(), unknown); codeOf(t, err) != CodeExportStepUpRequired {
		t.Fatalf("an unknown grant was accepted (err=%v)", err)
	}
	// A grant issued for a different purpose cannot be spent here.
	crossPurpose := base
	crossPurpose.StepUpGrantID = "grant_download_only"
	stepUp.issue("grant_download_only", StepUpPurposeExportDownload)
	if _, err := env.service.CreateExport(ctx, actor, globalScope(), crossPurpose); codeOf(t, err) != CodeExportStepUpRequired {
		t.Fatalf("a download grant created an export (err=%v)", err)
	}
	// A spent grant cannot be reused.
	reused := base
	reused.StepUpGrantID = "grant_once"
	stepUp.issue("grant_once", StepUpPurposeExportCreate)
	export, err := env.service.CreateExport(ctx, actor, globalScope(), reused)
	if err != nil {
		t.Fatalf("a valid grant was refused: %v", err)
	}
	if _, err = env.service.CreateExport(ctx, actor, globalScope(), reused); codeOf(t, err) != CodeExportStepUpRequired {
		t.Fatalf("a spent grant created a second export (err=%v)", err)
	}

	env.generateExports(t)
	ready := env.reloadExport(t, export.ID)
	if ready.Status != ExportReady {
		t.Fatalf("export finished as %s", ready.Status)
	}

	// Download needs its own fresh grant.
	if _, _, err = env.service.AuthorizeExportDownload(ctx, actor, globalScope(), export.ID,
		ExportAccessInput{SessionID: "session_test"}); codeOf(t, err) != CodeExportStepUpRequired {
		t.Fatalf("an artifact was released without step-up (err=%v)", err)
	}
	stepUp.issue("grant_dl", StepUpPurposeExportDownload)
	access, _, err := env.service.AuthorizeExportDownload(ctx, actor, globalScope(), export.ID,
		ExportAccessInput{StepUpGrantID: "grant_dl", SessionID: "session_test"})
	if err != nil {
		t.Fatalf("a valid download grant was refused: %v", err)
	}
	if access.Token == "" || access.Path == "" || !access.ExpiresAt.After(time.Now()) {
		t.Fatalf("the access grant is unusable: %+v", access)
	}
	// The same grant cannot release the bytes twice.
	if _, _, err = env.service.AuthorizeExportDownload(ctx, actor, globalScope(), export.ID,
		ExportAccessInput{StepUpGrantID: "grant_dl", SessionID: "session_test"}); codeOf(t, err) != CodeExportStepUpRequired {
		t.Fatalf("a spent download grant released the artifact again (err=%v)", err)
	}

	// With no step-up infrastructure at all, both operations refuse rather than
	// skipping the check.
	env.service.stepUp = nil
	if _, err = env.service.CreateExport(ctx, actor, globalScope(), base); codeOf(t, err) != CodeExportStepUpRequired {
		t.Fatalf("a missing step-up consumer allowed creation (err=%v)", err)
	}
}

// TestExports_ExpiryAndCancellationRevokeTheArtifact.
func TestExports_ExpiryAndCancellationRevokeTheArtifact(t *testing.T) {
	env, store, stepUp := newExportEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedExportFixture(t)

	// Expiry.
	expiring := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	env.generateExports(t)
	ready := env.reloadExport(t, expiring.ID)
	ref := *ready.ArtifactRef
	if _, err := env.pool.Exec(ctx, `UPDATE crm_exports
		SET completed_at=now() - interval '2 hours', expires_at=now() - interval '1 hour'
		WHERE id=$1`, expiring.ID); err != nil {
		t.Fatal(err)
	}
	stepUp.issue("grant_expired_dl", StepUpPurposeExportDownload)
	if _, _, err := env.service.AuthorizeExportDownload(ctx, actor, globalScope(), expiring.ID,
		ExportAccessInput{StepUpGrantID: "grant_expired_dl", SessionID: "s"}); codeOf(t, err) != CodeExportExpired {
		t.Fatalf("an expired artifact was released (err=%v)", err)
	}
	expired := env.reloadExport(t, expiring.ID)
	if expired.Status != ExportExpired || expired.ArtifactRef != nil {
		t.Fatalf("an expired export kept its artifact: %+v", expired)
	}
	if _, held := store.contentOf(ref); held {
		t.Fatal("the store still holds an expired artifact")
	}

	// Cancellation of a completed export revokes the bytes too.
	cancelling := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatJSONL, ScopeType: IntelligenceScopeBusiness})
	env.generateExports(t)
	completed := env.reloadExport(t, cancelling.ID)
	cancelRef := *completed.ArtifactRef
	cancelled, err := env.service.CancelExport(ctx, actor, globalScope(), cancelling.ID, completed.Version)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Status != ExportCancelled || cancelled.ArtifactRef != nil {
		t.Fatalf("a cancelled export kept its artifact: %+v", cancelled)
	}
	if _, held := store.contentOf(cancelRef); held {
		t.Fatal("the store still holds a cancelled artifact")
	}
	stepUp.issue("grant_cancelled_dl", StepUpPurposeExportDownload)
	if _, _, err = env.service.AuthorizeExportDownload(ctx, actor, globalScope(), cancelling.ID,
		ExportAccessInput{StepUpGrantID: "grant_cancelled_dl", SessionID: "s"}); err == nil {
		t.Fatal("a cancelled export was downloaded")
	}

	// The retention sweeper reaches the same conclusion without a reader.
	sweeping := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportIntelligenceScores, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	env.generateExports(t)
	swept := env.reloadExport(t, sweeping.ID)
	sweptRef := *swept.ArtifactRef
	if _, err = env.pool.Exec(ctx, `UPDATE crm_exports
		SET completed_at=now() - interval '3 hours', expires_at=now() - interval '1 hour'
		WHERE id=$1`, sweeping.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.ExportRetentionPass(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if final := env.reloadExport(t, sweeping.ID); final.Status != ExportExpired {
		t.Fatalf("the sweeper left an overdue export %s", final.Status)
	}
	if _, held := store.contentOf(sweptRef); held {
		t.Fatal("the sweeper left the artifact in the store")
	}
}

// TestExports_TenantAndBranchIsolation.
func TestExports_TenantAndBranchIsolation(t *testing.T) {
	env, _, stepUp := newExportEnv(t)
	ctx := context.Background()
	env.seedExportFixture(t)

	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	env.generateExports(t)

	// Business B cannot read, cancel or download it, and gets the same answer
	// it would get for an id that does not exist.
	actorB := actorFor(businessB, "vs_b")
	if _, err := env.service.GetExport(ctx, actorB, globalScope(), export.ID); codeOf(t, err) != CodeExportNotFound {
		t.Fatalf("business B read business A's export (err=%v)", err)
	}
	if _, err := env.service.CancelExport(ctx, actorB, globalScope(), export.ID, 0); codeOf(t, err) != CodeExportNotFound {
		t.Fatalf("business B cancelled business A's export (err=%v)", err)
	}
	stepUp.issue("grant_b", StepUpPurposeExportDownload)
	if _, _, err := env.service.AuthorizeExportDownload(ctx, actorB, globalScope(), export.ID,
		ExportAccessInput{StepUpGrantID: "grant_b", SessionID: "s"}); codeOf(t, err) != CodeExportNotFound {
		t.Fatalf("business B downloaded business A's export (err=%v)", err)
	}
	listed, _, err := env.service.ListExports(ctx, actorB, globalScope(), ExportListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("business B listed %d of business A's exports", len(listed))
	}

	// A branch actor cannot see the business-wide export at all: its existence
	// and row count describe the whole book.
	branchScope := Scope{BranchIDs: []string{branchA1}}
	if _, err = env.service.GetExport(ctx, actorFor(businessA, "vs_a"), branchScope,
		export.ID); codeOf(t, err) != CodeExportNotFound {
		t.Fatalf("a branch actor read a business-wide export (err=%v)", err)
	}
	listed, _, err = env.service.ListExports(ctx, actorFor(businessA, "vs_a"), branchScope, ExportListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range listed {
		if item.Scope == IntelligenceScopeBusiness {
			t.Fatalf("a branch actor listed a business-wide export: %+v", item)
		}
	}
	// And cannot request one.
	if _, err = env.service.CreateExport(ctx, actorFor(businessA, "vs_a"), branchScope,
		CreateExportInput{ExportType: ExportCustomerSummary, Format: FormatCSV,
			ScopeType: IntelligenceScopeBusiness, StepUpGrantID: "grant_x", SessionID: "s"}); err == nil {
		t.Fatal("a branch actor created a business-wide export")
	}
	// A segment belonging to another tenant is not a usable filter, and the
	// refusal does not confirm that it exists.
	segmentB, err := env.service.CreateSegment(ctx, actorB, globalScope(), CreateSegmentInput{
		Name: "B audience", SegmentType: SegmentStatic, ScopeType: ScopeBusiness})
	if err != nil {
		t.Fatal(err)
	}
	grant := "grant_cross"
	stepUp.issue(grant, StepUpPurposeExportCreate)
	if _, err = env.service.CreateExport(ctx, actorFor(businessA, "vs_a"), globalScope(),
		CreateExportInput{ExportType: ExportSegmentMembers, Format: FormatCSV,
			ScopeType: IntelligenceScopeBusiness, Filter: ExportFilter{SegmentRef: segmentB.ID},
			StepUpGrantID: grant, SessionID: "s"}); err == nil {
		t.Fatal("business A exported business B's segment")
	}
}

// TestExports_FiltersAreClosedAndNeverReachSQL.
func TestExports_FiltersAreClosedAndNeverReachSQL(t *testing.T) {
	env, _, stepUp := newExportEnv(t)
	ctx := context.Background()
	env.seedExportFixture(t)
	actor := actorFor(businessA, "vs_a")

	hostile := []ExportFilter{
		{ModelKey: "RFM_V1'; DROP TABLE crm_exports; --"},
		{RiskBands: []string{"HIGH'; DELETE FROM vendor_customer_profiles; --"}},
		{ScoreStates: []string{"CURRENT OR 1=1"}},
		{Statuses: []string{"ACTIVE'; TRUNCATE crm_exports; --"}},
	}
	for index, filter := range hostile {
		grant := fmt.Sprintf("grant_hostile_%d", index)
		stepUp.issue(grant, StepUpPurposeExportCreate)
		if _, err := env.service.CreateExport(ctx, actor, globalScope(), CreateExportInput{
			ExportType: ExportIntelligenceScores, Format: FormatCSV,
			ScopeType: IntelligenceScopeBusiness, Filter: filter,
			StepUpGrantID: grant, SessionID: "s"}); err == nil {
			t.Fatalf("hostile filter %d was accepted", index)
		}
	}
	// Nothing was executed.
	var profiles int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_customer_profiles
		WHERE business_id=$1`, businessA).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if profiles == 0 {
		t.Fatal("hostile filter input reached the database")
	}
	// An unsupported dataset or format is refused with the right code.
	grant := "grant_badtype"
	stepUp.issue(grant, StepUpPurposeExportCreate)
	if _, err := env.service.CreateExport(ctx, actor, globalScope(), CreateExportInput{
		ExportType: "RAW_SQL", Format: FormatCSV, ScopeType: IntelligenceScopeBusiness,
		StepUpGrantID: grant, SessionID: "s"}); codeOf(t, err) != CodeExportInvalidType {
		t.Fatalf("an unknown dataset was accepted (err=%v)", err)
	}
	stepUp.issue("grant_badformat", StepUpPurposeExportCreate)
	if _, err := env.service.CreateExport(ctx, actor, globalScope(), CreateExportInput{
		ExportType: ExportCustomerSummary, Format: "XLSX", ScopeType: IntelligenceScopeBusiness,
		StepUpGrantID: "grant_badformat", SessionID: "s"}); codeOf(t, err) != CodeExportInvalidFormat {
		t.Fatalf("an unsupported format was accepted (err=%v)", err)
	}
}

// TestExports_RedactionDuringGenerationBlocksPublication is the privacy race.
//
// The window that matters is between the generating read and the publish
// decision: a relationship that was lawfully readable when the rows were
// selected can stop being readable before the file is released. The test
// commits a real redaction inside exactly that window rather than sleeping.
func TestExports_RedactionDuringGenerationBlocksPublication(t *testing.T) {
	env, store, stepUp := newExportEnv(t)
	ctx := context.Background()
	profiles := env.seedExportFixture(t)
	env.service.exports.MaxAttempts = 5

	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	claimed, ok, err := env.service.claimExport(ctx, "race-worker")
	if err != nil || !ok {
		t.Fatalf("claim: ok=%t err=%v", ok, err)
	}

	// Read the rows the way the worker does. The victim is still lawful here,
	// so the bytes genuinely contain them.
	dataset, _ := ExportDatasetFor(claimed.ExportType)
	var fenceAt time.Time
	if err = env.pool.QueryRow(ctx, `SELECT now()`).Scan(&fenceAt); err != nil {
		t.Fatal(err)
	}
	content, _, err := env.service.renderExport(ctx, dataset, claimed)
	if err != nil {
		t.Fatal(err)
	}
	victim := profiles[0]
	if !strings.Contains(string(content), victim.ID) {
		t.Fatal("the fixture did not include the subject; the race would prove nothing")
	}

	// The redaction lands after the read and before the publish decision.
	if err = env.applyPrivacy(t, businessA, "vs_a", &victim, PrivacyInput{
		SourceService: "customer-service", SourceVersion: victim.Version + 1,
		PrivacySubjectRef: victim.ID, Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatalf("redact: %v", err)
	}

	certified, err := env.service.certifyExportPrivacy(ctx, claimed, fenceAt)
	if err != nil {
		t.Fatal(err)
	}
	if certified {
		t.Fatal("certification passed across a redaction that landed mid-generation")
	}

	// The worker's response to a failed certification is to discard the bytes
	// and send the job back with a fresh fence, recording why.
	if err = env.service.requeueExportForPrivacy(ctx, claimed); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	requeued := env.reloadExport(t, export.ID)
	if requeued.Status == ExportReady {
		t.Fatal("an export was published across a redaction without re-certification")
	}
	if requeued.ErrorCode == nil || *requeued.ErrorCode != "PRIVACY_RECERTIFICATION_REQUIRED" {
		t.Fatalf("the run did not record why it refused: %v", requeued.ErrorCode)
	}
	store.mu.Lock()
	held := len(store.objects)
	store.mu.Unlock()
	if held != 0 {
		t.Fatalf("%d artifacts were stored despite failed certification", held)
	}

	// Regenerating from the fresh fence succeeds and excludes the redacted
	// relationship entirely.
	env.generateExports(t)
	final := env.reloadExport(t, export.ID)
	if final.Status != ExportReady {
		t.Fatalf("the retried export finished as %s (error=%v)", final.Status, final.ErrorCode)
	}
	republished, _ := store.contentOf(*final.ArtifactRef)
	if strings.Contains(republished, victim.ID) {
		t.Fatalf("the republished export still contains the redacted relationship: %s",
			truncateForTest(republished))
	}
}

// TestExports_MergedSourceIsNeverExportedAlongsideItsTarget keeps a canonical
// customer dataset canonical: a merged-away relationship must not appear next
// to the one it merged into.
func TestExports_MergedSourceIsNeverExportedAlongsideItsTarget(t *testing.T) {
	env, store, stepUp := newExportEnv(t)
	profiles := env.seedExportFixture(t)
	source, target := profiles[0], profiles[1]

	if err := env.applyLink(t, businessA, "vs_a", linkInput("export_merge",
		customerSubject("cus_exp0"), customerSubject("cus_exp1"), 1)); err != nil {
		t.Fatalf("merge: %v", err)
	}

	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	env.generateExports(t)
	ready := env.reloadExport(t, export.ID)
	if ready.Status != ExportReady {
		t.Fatalf("export finished as %s (error=%v)", ready.Status, ready.ErrorCode)
	}
	content, _ := store.contentOf(*ready.ArtifactRef)
	if strings.Contains(content, source.ID) {
		t.Fatalf("a merged-away relationship was exported alongside its target: %s",
			truncateForTest(content))
	}
	if !strings.Contains(content, target.ID) {
		t.Fatal("the surviving relationship is missing from the export")
	}
}

// TestExports_CancellationDuringGenerationWinsOverPublication is the other
// race: a job cancelled while its file was being written must not be released.
func TestExports_CancellationDuringGenerationWinsOverPublication(t *testing.T) {
	env, store, stepUp := newExportEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedExportFixture(t)

	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	claimed, ok, err := env.service.claimExport(ctx, "race-worker")
	if err != nil || !ok {
		t.Fatalf("claim: ok=%t err=%v", ok, err)
	}
	// The operator cancels while the worker holds the job.
	running := env.reloadExport(t, export.ID)
	if _, err = env.service.CancelExport(ctx, actor, globalScope(), export.ID, running.Version); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// The worker finishes and tries to publish. The guarded update refuses.
	if err = env.service.generateExport(ctx, claimed); err != nil {
		t.Fatalf("generate: %v", err)
	}
	final := env.reloadExport(t, export.ID)
	if final.Status != ExportCancelled {
		t.Fatalf("a cancelled export finished as %s", final.Status)
	}
	if final.ArtifactRef != nil {
		t.Fatalf("a cancelled export references an artifact: %+v", final)
	}
	// And the bytes the worker produced were deleted rather than left behind.
	store.mu.Lock()
	orphans := len(store.objects)
	store.mu.Unlock()
	if orphans != 0 {
		t.Fatalf("%d artifacts survived a cancellation", orphans)
	}
	stepUp.issue("grant_cancel_dl", StepUpPurposeExportDownload)
	if _, _, err = env.service.AuthorizeExportDownload(ctx, actor, globalScope(), export.ID,
		ExportAccessInput{StepUpGrantID: "grant_cancel_dl", SessionID: "s"}); err == nil {
		t.Fatal("a cancelled export was downloadable")
	}
}

// TestExports_RetriedGenerationReusesOneArtifact proves a retry does not
// orphan objects in the private store.
func TestExports_RetriedGenerationReusesOneArtifact(t *testing.T) {
	env, store, stepUp := newExportEnv(t)
	ctx := context.Background()
	env.seedExportFixture(t)

	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	for attempt := 0; attempt < 2; attempt++ {
		claimed, ok, err := env.service.claimExport(ctx, fmt.Sprintf("worker-%d", attempt))
		if !ok {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if err = env.service.generateExport(ctx, claimed); err != nil {
			t.Fatal(err)
		}
		if _, err = env.pool.Exec(ctx, `UPDATE crm_exports SET status='RUNNING', next_attempt_at=now()
			WHERE id=$1 AND status='READY'`, export.ID); err != nil {
			t.Fatal(err)
		}
	}
	store.mu.Lock()
	objects := len(store.objects)
	store.mu.Unlock()
	if objects != 1 {
		t.Fatalf("a retried generation left %d artifacts; the idempotency key must reuse one", objects)
	}
}

// TestExports_DisabledFlagsRegisterAndGenerateNothing.
func TestExports_DisabledFlagsRegisterAndGenerateNothing(t *testing.T) {
	off := allFlagsOn()
	off.ExportsEnabled = false
	env := newTestEnv(t, off)
	env.service.artifacts = newFakeArtifactStore()
	env.service.stepUp = newFakeStepUp()

	if _, err := env.service.CreateExport(context.Background(), actorFor(businessA, "vs_a"),
		globalScope(), CreateExportInput{ExportType: ExportCustomerSummary, Format: FormatCSV,
			ScopeType: IntelligenceScopeBusiness}); codeOf(t, err) != CodeExportDisabled {
		t.Fatalf("an export was created while the feature is off (err=%v)", err)
	}
	processed, err := env.service.RunExportGenerator(context.Background(), "w", 5)
	if err != nil || processed != 0 {
		t.Fatalf("the generator claimed %d jobs while disabled (err=%v)", processed, err)
	}

	// With no artifact store, creation refuses rather than queueing work that
	// could never finish.
	on := newTestEnv(t, allFlagsOn())
	on.service.stepUp = newFakeStepUp()
	on.service.artifacts = nil
	if _, err = on.service.CreateExport(context.Background(), actorFor(businessA, "vs_a"), globalScope(),
		CreateExportInput{ExportType: ExportCustomerSummary, Format: FormatCSV,
			ScopeType: IntelligenceScopeBusiness}); codeOf(t, err) != CodeExportStorageUnavailable {
		t.Fatalf("an export was queued with no private store (err=%v)", err)
	}
}
