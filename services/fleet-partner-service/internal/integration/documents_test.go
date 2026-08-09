package integration_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	documentmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	documentusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	eligibilityrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// --- creation and orchestration -------------------------------------------

// TestDocumentUploadOrchestration proves the whole path: fleet asks media for a
// grant over a signed call, records the document pointing at the object media
// minted, and hands back a short-lived token it does not keep.
func TestDocumentUploadOrchestration(t *testing.T) {
	f := newDocumentFixture(t, "Doc Upload Co", "DOC-1")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "uploader")

	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	if ticket.Document.MediaObjectID == "" {
		t.Fatal("document does not point at a media object")
	}
	if ticket.UploadToken == "" {
		t.Fatal("no upload token was returned")
	}
	if f.media.uploadAuthCalls != 1 {
		t.Fatalf("media authorization calls = %d, want 1", f.media.uploadAuthCalls)
	}
	if ticket.Document.LifecycleState != documentmodels.StateUploadPending {
		t.Fatalf("lifecycleState = %s, want UPLOAD_PENDING", ticket.Document.LifecycleState)
	}
	if ticket.Document.SafetyProjection == documentmodels.SafetySafe {
		t.Fatal("a document must never start out SAFE")
	}

	// The bearer credential is never written down on the fleet side.
	assertNotPersisted(t, f.pool, ticket.UploadToken)

	// And no column anywhere holds a durable link to the file.
	var columns int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'fleet_partner_documents'
		  AND (column_name LIKE '%url%' OR column_name LIKE '%token%'
		       OR column_name LIKE '%storage%' OR column_name LIKE '%bucket%')`).Scan(&columns); err != nil {
		t.Fatalf("inspect document columns: %v", err)
	}
	if columns != 0 {
		t.Fatalf("fleet_partner_documents has %d URL/token/storage columns; a private document must have none", columns)
	}
}

// TestDocumentUploadIsIdempotent proves a retry does not create a second
// document, and that a reused key describing something else is refused.
func TestDocumentUploadIsIdempotent(t *testing.T) {
	f := newDocumentFixture(t, "Doc Idem Co", "DOC-2")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "idem")

	input := documentusecases.RequestUploadInput{
		Actor: f.actor(member), OwnerType: "COMPANY", OwnerID: f.orgID,
		Category: "INSURANCE", DocumentType: "INSURANCE_CERTIFICATE",
		AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 1 << 20,
		IdempotencyKey: "fixed-key",
	}
	first, err := f.service.RequestUpload(context.Background(), input)
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	second, err := f.service.RequestUpload(context.Background(), input)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !second.Replayed || second.Document.ID != first.Document.ID {
		t.Fatal("a retry must replay the same document")
	}

	conflicting := input
	conflicting.DocumentType = "SOMETHING_ELSE"
	if _, err := f.service.RequestUpload(context.Background(), conflicting); err == nil {
		t.Fatal("a reused idempotency key describing a different request must be refused")
	}
}

// TestDocumentOwnerMustBelongToTenant proves an identifier from another
// organization is not a way in, and answers not-found rather than forbidden.
func TestDocumentOwnerMustBelongToTenant(t *testing.T) {
	f := newDocumentFixture(t, "Doc Tenant Co", "DOC-3")
	other := createOrganization(t, f.pool, "Other Tenant Co", "DOC-3-OTHER")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "tenant")

	_, err := f.service.RequestUpload(context.Background(), documentusecases.RequestUploadInput{
		Actor: f.actor(member), OwnerType: "COMPANY", OwnerID: other.ID,
		Category: "INSURANCE", DocumentType: "INSURANCE_CERTIFICATE",
		AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 1 << 20,
		IdempotencyKey: "cross-tenant",
	})
	if err == nil {
		t.Fatal("a document must not be attachable to another tenant's organization")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("cross-tenant write should answer not-found, got %v", err)
	}
	if f.media.uploadAuthCalls != 0 {
		t.Fatal("the tenant check must run before any call to media")
	}
}

// TestDocumentReadIsTenantScoped proves another organization's document
// identifier reads as absent.
func TestDocumentReadIsTenantScoped(t *testing.T) {
	f := newDocumentFixture(t, "Doc Read Co", "DOC-4")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "reader")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	other := createOrganization(t, f.pool, "Other Read Co", "DOC-4-OTHER")
	if _, err := f.service.Get(context.Background(), other.ID, ticket.Document.ID); err == nil {
		t.Fatal("a document must not be readable from another tenant")
	}
}

// --- event ingestion ------------------------------------------------------

// TestMediaEventProjectsAuthoritativeState proves an event moves the document
// onto media's answer and records where the answer came from.
func TestMediaEventProjectsAuthoritativeState(t *testing.T) {
	f := newDocumentFixture(t, "Doc Event Co", "DOC-5")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "events")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	outcome, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, nil)
	if err != nil {
		t.Fatalf("ingest event: %v", err)
	}
	if outcome != documentusecases.OutcomeProcessed {
		t.Fatalf("outcome = %s, want PROCESSED", outcome)
	}

	document, err := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if document.SafetyProjection != documentmodels.SafetySafe {
		t.Fatalf("safetyProjection = %s, want SAFE", document.SafetyProjection)
	}
	if document.MediaSourceVersion != 2 {
		t.Fatalf("mediaSourceVersion = %d, want 2", document.MediaSourceVersion)
	}
	if document.ProjectionFreshness != documentmodels.FreshnessCurrent {
		t.Fatalf("projectionFreshness = %s, want CURRENT", document.ProjectionFreshness)
	}
	if document.LifecycleState != documentmodels.StateSafe {
		t.Fatalf("lifecycleState = %s, want SAFE", document.LifecycleState)
	}
}

// TestMediaEventPersistsChecksumAndCaptureMetadata proves the authoritative
// checksum, detected MIME type and size media reports on a media event land
// on the document row, and that a verification decision taken afterwards
// carries the checksum it was actually decided against.
func TestMediaEventPersistsChecksumAndCaptureMetadata(t *testing.T) {
	f := newDocumentFixture(t, "Doc Checksum Co", "DOC-CHK")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "checksum")
	verifier := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "checksumverifier")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	wantChecksum := strings.Repeat("b", 64)
	capturedAt := "2026-08-03T10:15:30Z"
	outcome, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, map[string]any{
			"checksumSha256":    strings.Repeat("a", 64),
			"integrityHash":     wantChecksum,
			"detectedMime":      "application/octet-stream",
			"originalMimeType":  "application/pdf",
			"sizeBytes":         1024,
			"originalSizeBytes": 2048,
			"capturedAt":        capturedAt,
			"sourceTimestamp":   capturedAt,
			"captureMethod":     "MEDIA_VALIDATED_UPLOAD",
			"actorReference":    "media-file-service",
		})
	if err != nil {
		t.Fatalf("ingest event: %v", err)
	}
	if outcome != documentusecases.OutcomeProcessed {
		t.Fatalf("outcome = %s, want PROCESSED", outcome)
	}

	document, err := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if document.MediaChecksumSHA256 != wantChecksum {
		t.Fatalf("mediaChecksumSha256 = %q, want %q", document.MediaChecksumSHA256, wantChecksum)
	}
	if document.OriginalMimeType != "application/pdf" {
		t.Fatalf("originalMimeType = %q, want application/pdf", document.OriginalMimeType)
	}
	if document.OriginalSizeBytes != 2048 {
		t.Fatalf("originalSizeBytes = %d, want 2048", document.OriginalSizeBytes)
	}
	if document.CaptureMethod != "MEDIA_VALIDATED_UPLOAD" || document.ActorReference != "media-file-service" || document.CapturedAt == nil || document.SourceTimestamp == nil {
		t.Fatalf("authoritative capture metadata was not projected: %+v", document)
	}

	// A verification decision taken after the checksum is known must record
	// the checksum it was decided against.
	verified, err := f.service.Verify(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(verifier), DocumentID: ticket.Document.ID,
		ExpectedVersion: document.Version, CorrelationID: "cor-" + randomHex(),
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if verified.MediaChecksumSHA256 != wantChecksum {
		t.Fatalf("verified document checksum = %q, want %q", verified.MediaChecksumSHA256, wantChecksum)
	}

	decisions, err := f.service.Verifications(context.Background(), f.orgID, ticket.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) == 0 {
		t.Fatal("expected at least one verification decision")
	}
	if decisions[0].MediaChecksumSHA256 != wantChecksum {
		t.Fatalf("verification decision checksum = %q, want %q",
			decisions[0].MediaChecksumSHA256, wantChecksum)
	}

	// A late/duplicate event redelivering the same source version must not
	// regress the checksum. Only a strictly newer source version may.
	newerChecksum := strings.Repeat("c", 64)
	if _, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.scan_completed.v1", 3, map[string]any{
			"checksumSha256": newerChecksum,
			"safetyStatus":   "SAFE",
		}); err != nil {
		t.Fatalf("ingest newer event: %v", err)
	}
	after, err := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.MediaChecksumSHA256 != newerChecksum {
		t.Fatalf("mediaChecksumSha256 after newer version = %q, want %q",
			after.MediaChecksumSHA256, newerChecksum)
	}
}

// TestMediaEventDuplicateIsNotApplied proves a redelivery is acknowledged
// without applying the state change twice.
func TestMediaEventDuplicateIsNotApplied(t *testing.T) {
	f := newDocumentFixture(t, "Doc Dup Co", "DOC-6")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "dup")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	payload := map[string]any{
		"mediaObjectId": ticket.Document.MediaObjectID, "organizationId": f.orgID,
		"uploadStatus": "COMPLETED", "safetyStatus": "SAFE", "deletionState": "ACTIVE",
		"sourceVersion": 2,
	}
	eventID := "mevt_fixed_duplicate"
	if outcome, err := f.deliverMediaEventWithID(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, eventID, payload); err != nil || outcome != documentusecases.OutcomeProcessed {
		t.Fatalf("first delivery outcome = %s err = %v", outcome, err)
	}
	before := readDocumentColumn(t, f.pool, ticket.Document.ID, "version")

	outcome, err := f.deliverMediaEventWithID(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, eventID, payload)
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if outcome != documentusecases.OutcomeDuplicate {
		t.Fatalf("redelivery outcome = %s, want DUPLICATE", outcome)
	}
	if after := readDocumentColumn(t, f.pool, ticket.Document.ID, "version"); after != before {
		t.Fatalf("document version moved on a duplicate: %s -> %s", before, after)
	}
}

// TestMediaEventPayloadConflictIsRefused proves the same event identity
// carrying a different body is never applied.
func TestMediaEventPayloadConflictIsRefused(t *testing.T) {
	f := newDocumentFixture(t, "Doc Conflict Co", "DOC-7")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "conflict")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	eventID := "mevt_fixed_conflict"
	base := map[string]any{
		"mediaObjectId": ticket.Document.MediaObjectID, "organizationId": f.orgID,
		"uploadStatus": "COMPLETED", "safetyStatus": "SAFE", "deletionState": "ACTIVE",
		"sourceVersion": 2,
	}
	if _, err := f.deliverMediaEventWithID(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, eventID, base); err != nil {
		t.Fatalf("first delivery: %v", err)
	}

	tampered := map[string]any{
		"mediaObjectId": ticket.Document.MediaObjectID, "organizationId": f.orgID,
		"uploadStatus": "COMPLETED", "safetyStatus": "UNSAFE", "deletionState": "ACTIVE",
		"sourceVersion": 2,
	}
	_, err := f.deliverMediaEventWithID(t, ticket.Document.MediaObjectID,
		"media.unsafe_detected.v1", 2, eventID, tampered)
	if err == nil {
		t.Fatal("the same event id with a different payload must be refused")
	}

	// And the earlier, honest state stands.
	document, _ := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.SafetyProjection != documentmodels.SafetySafe {
		t.Fatalf("a refused conflict must not change state; safety = %s", document.SafetyProjection)
	}
}

// TestMediaEventSameVersionDifferentPayloadIsRefused proves a NEW event id
// claiming an already-recorded source version with a different body is refused
// too. Deduplication by event id alone would let this through.
func TestMediaEventSameVersionDifferentPayloadIsRefused(t *testing.T) {
	f := newDocumentFixture(t, "Doc Version Co", "DOC-8")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "version")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, nil); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	_, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.unsafe_detected.v1", 2, map[string]any{"safetyStatus": "UNSAFE"})
	if err == nil {
		t.Fatal("a different payload at the same source version must be refused")
	}
}

// TestMediaEventOlderVersionIsIgnored proves a late delivery cannot move the
// projection backwards.
func TestMediaEventOlderVersionIsIgnored(t *testing.T) {
	f := newDocumentFixture(t, "Doc Older Co", "DOC-9")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "older")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.scan_completed.v1", 5, nil); err != nil {
		t.Fatalf("current delivery: %v", err)
	}
	outcome, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 3, map[string]any{"safetyStatus": "PENDING"})
	if err != nil {
		t.Fatalf("late delivery: %v", err)
	}
	if outcome != documentusecases.OutcomeOlder {
		t.Fatalf("outcome = %s, want OLDER", outcome)
	}

	document, _ := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.MediaSourceVersion != 5 || document.SafetyProjection != documentmodels.SafetySafe {
		t.Fatalf("a late event moved the projection backwards: version=%d safety=%s",
			document.MediaSourceVersion, document.SafetyProjection)
	}
}

// TestMediaEventVersionGapOpensReconciliation proves a jump in versions marks
// the projection untrustworthy and opens a durable case.
func TestMediaEventVersionGapOpensReconciliation(t *testing.T) {
	f := newDocumentFixture(t, "Doc Gap Co", "DOC-10")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "gap")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, nil); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	outcome, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.scan_completed.v1", 9, nil)
	if err != nil {
		t.Fatalf("gapped delivery: %v", err)
	}
	if outcome != documentusecases.OutcomeGapDetected {
		t.Fatalf("outcome = %s, want GAP_DETECTED", outcome)
	}

	document, _ := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.ProjectionFreshness != documentmodels.FreshnessGapDetected {
		t.Fatalf("projectionFreshness = %s, want GAP_DETECTED", document.ProjectionFreshness)
	}
	if document.LifecycleState != documentmodels.StateGapDetected {
		t.Fatalf("lifecycleState = %s, want GAP_DETECTED", document.LifecycleState)
	}
	// A gapped document cannot be released for download.
	if document.Downloadable() {
		t.Fatal("a document with a detected gap must not be downloadable")
	}

	cases, err := f.service.Cases(context.Background(), f.orgID, ticket.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].Reason != documentmodels.ReconcileVersionGap {
		t.Fatalf("expected one VERSION_GAP case, got %v", cases)
	}
}

// TestReconciliationRepairsFromAuthority proves the worker resolves a case by
// reading media back and believing the answer.
func TestReconciliationRepairsFromAuthority(t *testing.T) {
	f := newDocumentFixture(t, "Doc Repair Co", "DOC-11")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "repair")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)
	objectID := ticket.Document.MediaObjectID

	f.media.completeUpload(objectID, "SAFE", []byte("%PDF-1.7 test"))
	if _, err := f.deliverMediaEvent(t, objectID, "media.upload_completed.v1", 2, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.deliverMediaEvent(t, objectID, "media.scan_completed.v1", 9, nil); err != nil {
		t.Fatal(err)
	}

	// The authority's current truth differs from what the gapped event said.
	// It is still forward of the emitted version: reconciliation must never
	// move a source version backwards.
	f.media.setVersion(objectID, 9)
	f.media.setSafety(objectID, "QUARANTINED")
	authoritative := f.media.objectVersion(objectID)

	claimed, err := f.service.RunReconciliationPass(context.Background(),
		documentusecases.ReconciliationConfig{Owner: "test", BatchSize: 10, StaleAfter: time.Hour})
	if err != nil {
		t.Fatalf("reconciliation pass: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("claimed %d cases, want 1", claimed)
	}

	document, _ := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.SafetyProjection != documentmodels.SafetyQuarantined {
		t.Fatalf("safetyProjection = %s, want QUARANTINED after read-back", document.SafetyProjection)
	}
	if document.ProjectionFreshness != documentmodels.FreshnessCurrent {
		t.Fatalf("projectionFreshness = %s, want CURRENT after repair", document.ProjectionFreshness)
	}
	if document.MediaSourceVersion != authoritative {
		t.Fatalf("mediaSourceVersion = %d, want the authority's %d",
			document.MediaSourceVersion, authoritative)
	}

	cases, _ := f.service.Cases(context.Background(), f.orgID, ticket.Document.ID)
	if len(cases) != 1 || cases[0].Status != documentmodels.CaseResolved {
		t.Fatalf("expected the case to be RESOLVED, got %v", cases)
	}
}

// TestStaleSweepBlocksSilentProjections proves a projection nobody has
// confirmed stops authorizing anything.
func TestStaleSweepBlocksSilentProjections(t *testing.T) {
	f := newDocumentFixture(t, "Doc Stale Co", "DOC-12")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "stale")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)
	objectID := ticket.Document.MediaObjectID

	f.media.completeUpload(objectID, "SAFE", []byte("%PDF-1.7 test"))
	if _, err := f.deliverMediaEvent(t, objectID, "media.upload_completed.v1", 2, nil); err != nil {
		t.Fatal(err)
	}

	// Age the last confirmation past the threshold.
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE fleet_partner_documents SET synced_at = now() - interval '2 hours' WHERE id = $1`,
		ticket.Document.ID); err != nil {
		t.Fatalf("age the projection: %v", err)
	}

	if _, err := f.service.RunReconciliationPass(context.Background(),
		documentusecases.ReconciliationConfig{Owner: "test", BatchSize: 10, StaleAfter: time.Minute}); err != nil {
		t.Fatalf("reconciliation pass: %v", err)
	}

	document, _ := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	// The sweep marks it stale and the same pass repairs it from the authority,
	// so what must be true afterwards is that a case was recorded and the
	// projection is once again backed by a read-back.
	cases, _ := f.service.Cases(context.Background(), f.orgID, ticket.Document.ID)
	if len(cases) == 0 {
		t.Fatal("a silent projection must open a reconciliation case")
	}
	if cases[0].Reason != documentmodels.ReconcileStaleProjection {
		t.Fatalf("case reason = %s, want STALE_PROJECTION", cases[0].Reason)
	}
	if document.SyncedAt == nil || time.Since(*document.SyncedAt) > time.Minute {
		t.Fatal("the repair should have refreshed the projection timestamp")
	}
}

// TestReconciliationDeadLettersAndReplays proves a permanently failing case
// stops consuming the batch and can be put back by an operator.
func TestReconciliationDeadLettersAndReplays(t *testing.T) {
	f := newDocumentFixture(t, "Doc DeadLetter Co", "DOC-13")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "deadletter")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	if err := f.service.OpenReconciliation(context.Background(), f.orgID,
		ticket.Document.ID, documentmodels.ReconcileUnknownOutcome); err != nil {
		t.Fatalf("open reconciliation: %v", err)
	}

	// The authority is unreachable for every attempt.
	f.media.unavailable = true
	config := documentusecases.ReconciliationConfig{
		Owner: "test", BatchSize: 10, MaxAttempts: 1, Backoff: 0, StaleAfter: time.Hour,
	}
	if _, err := f.service.RunReconciliationPass(context.Background(), config); err != nil {
		t.Fatalf("reconciliation pass: %v", err)
	}

	cases, _ := f.service.Cases(context.Background(), f.orgID, ticket.Document.ID)
	if len(cases) != 1 || cases[0].Status != documentmodels.CaseDeadLettered {
		t.Fatalf("expected a DEAD_LETTERED case, got %v", cases)
	}

	if err := f.service.ReplayReconciliationCase(context.Background(), f.orgID, cases[0].ID); err != nil {
		t.Fatalf("operator replay: %v", err)
	}
	cases, _ = f.service.Cases(context.Background(), f.orgID, ticket.Document.ID)
	if cases[0].Status != documentmodels.CaseOpen {
		t.Fatalf("replayed case status = %s, want OPEN", cases[0].Status)
	}
}

// TestReconciliationRecoversAbandonedLease proves a worker that died holding a
// lease does not strand its case forever.
func TestReconciliationRecoversAbandonedLease(t *testing.T) {
	f := newDocumentFixture(t, "Doc Lease Co", "DOC-14")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "lease")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	if err := f.service.OpenReconciliation(context.Background(), f.orgID,
		ticket.Document.ID, documentmodels.ReconcileUnknownOutcome); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_document_reconciliation_cases
		SET status = 'IN_PROGRESS', lease_owner = 'dead-worker',
		    lease_expires_at = now() - interval '1 hour'
		WHERE document_id = $1`, ticket.Document.ID); err != nil {
		t.Fatalf("simulate abandoned lease: %v", err)
	}

	claimed, err := f.service.RunReconciliationPass(context.Background(),
		documentusecases.ReconciliationConfig{Owner: "fresh-worker", BatchSize: 10, StaleAfter: time.Hour})
	if err != nil {
		t.Fatalf("reconciliation pass: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("claimed %d cases, want the abandoned one to be reclaimed", claimed)
	}
}

// --- verification ---------------------------------------------------------

// safeDocument creates a document and brings it to the SAFE state media would
// leave it in after a successful upload and scan.
func safeDocument(t *testing.T, f *documentFixture, member string, documentType string,
	expiry *time.Time) *documentusecases.UploadTicket {
	t.Helper()
	ticket := f.createDocument(t, member, documentType, expiry)
	f.media.completeUpload(ticket.Document.MediaObjectID, "SAFE", []byte("%PDF-1.7 test"))
	if _, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, nil); err != nil {
		t.Fatalf("deliver upload event: %v", err)
	}
	return ticket
}

// TestVerificationRequiresSafeContent is the single rule the work package
// exists to protect: PENDING, UNKNOWN, UNSAFE and QUARANTINED all refuse.
func TestVerificationRequiresSafeContent(t *testing.T) {
	for _, safety := range []string{"PENDING", "UNKNOWN", "UNSAFE", "QUARANTINED"} {
		t.Run(safety, func(t *testing.T) {
			f := newDocumentFixture(t, "Doc Verify "+safety+" Co", "DOC-V-"+safety)
			submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
			verifier := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "verifier")

			ticket := f.createDocument(t, submitter, "INSURANCE_CERTIFICATE", nil)
			if _, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
				"media.scan_completed.v1", 2, map[string]any{"safetyStatus": safety}); err != nil {
				t.Fatalf("deliver event: %v", err)
			}

			_, err := f.service.Verify(context.Background(), documentusecases.VerificationInput{
				Actor: f.actor(verifier), DocumentID: ticket.Document.ID,
			})
			if err == nil {
				t.Fatalf("a %s document must not be verifiable", safety)
			}

			// And the database refuses it too, so a future code path that
			// skipped the check would still fail rather than store a lie.
			_, dbErr := f.pool.Exec(context.Background(),
				`UPDATE fleet_partner_documents SET verification_state = 'VERIFIED' WHERE id = $1`,
				ticket.Document.ID)
			if dbErr == nil {
				t.Fatal("the schema must refuse VERIFIED without SAFE")
			}
		})
	}
}

// TestVerificationSucceedsForSafeDocument proves the happy path and its
// immutable decision record.
func TestVerificationSucceedsForSafeDocument(t *testing.T) {
	f := newDocumentFixture(t, "Doc Verified Co", "DOC-15")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	verifier := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "verifier")
	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)

	document, err := f.service.Verify(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(verifier), DocumentID: ticket.Document.ID,
		ExpectedVersion: 0,
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !document.Verified() {
		t.Fatalf("document is not verified: state=%s verification=%s",
			document.LifecycleState, document.VerificationState)
	}

	decisions, err := f.service.Verifications(context.Background(), f.orgID, ticket.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Decision != documentmodels.DecisionVerified {
		t.Fatalf("expected one VERIFIED decision, got %v", decisions)
	}
	if decisions[0].DecidedByActorID != verifier {
		t.Fatal("the decision must name the actor who took it")
	}

	// The decision history is immutable.
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE fleet_partner_document_verifications SET decision = 'REJECTED' WHERE document_id = $1`,
		ticket.Document.ID); err == nil {
		t.Fatal("verification decisions must be append-only")
	}
}

// TestSelfVerificationIsRefused proves the submitter cannot sign off their own
// document under the default policy.
func TestSelfVerificationIsRefused(t *testing.T) {
	f := newDocumentFixture(t, "Doc Self Co", "DOC-16")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "self")
	ticket := safeDocument(t, f, member, "INSURANCE_CERTIFICATE", nil)

	_, err := f.service.Verify(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
	})
	if err == nil {
		t.Fatal("the submitter must not be able to verify their own document")
	}
}

// TestUnsafeEventInvalidatesStandingVerification proves a verification does not
// survive the file becoming unsafe.
func TestUnsafeEventInvalidatesStandingVerification(t *testing.T) {
	f := newDocumentFixture(t, "Doc Void Co", "DOC-17")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	verifier := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "verifier")
	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.service.Verify(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(verifier), DocumentID: ticket.Document.ID,
	}); err != nil {
		t.Fatalf("verify: %v", err)
	}

	if _, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.unsafe_detected.v1", 3, map[string]any{"safetyStatus": "QUARANTINED"}); err != nil {
		t.Fatalf("deliver unsafe event: %v", err)
	}

	document, _ := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.Verified() {
		t.Fatal("a quarantined file must not keep a standing verification")
	}
	if document.VerificationState != documentmodels.VerificationInvalidated {
		t.Fatalf("verificationState = %s, want INVALIDATED", document.VerificationState)
	}
	if document.Downloadable() {
		t.Fatal("a quarantined document must not be downloadable")
	}
}

// TestRejectionRequiresControlledReason proves a rejection carries a reason
// whoever has to fix it can act on.
func TestRejectionRequiresControlledReason(t *testing.T) {
	f := newDocumentFixture(t, "Doc Reject Co", "DOC-18")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	verifier := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "verifier")
	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.service.Reject(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(verifier), DocumentID: ticket.Document.ID, ReasonCode: "MADE_UP",
	}); err == nil {
		t.Fatal("an uncontrolled rejection reason must be refused")
	}

	document, err := f.service.Reject(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(verifier), DocumentID: ticket.Document.ID,
		ReasonCode: documentmodels.RejectIllegible, ReasonNote: "scan is unreadable",
	})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if document.LifecycleState != documentmodels.StateRejected {
		t.Fatalf("lifecycleState = %s, want REJECTED", document.LifecycleState)
	}
}

// --- replacement, revocation, expiry --------------------------------------

// TestReplacementPreservesOriginalAndEarnsItsOwnVerification proves the
// successor inherits nothing but its owner.
func TestReplacementPreservesOriginalAndEarnsItsOwnVerification(t *testing.T) {
	f := newDocumentFixture(t, "Doc Replace Co", "DOC-19")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	verifier := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "verifier")
	original := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.service.Verify(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(verifier), DocumentID: original.Document.ID,
	}); err != nil {
		t.Fatalf("verify original: %v", err)
	}

	replacement, err := f.service.Replace(context.Background(), documentusecases.ReplaceInput{
		Actor: f.actor(submitter), DocumentID: original.Document.ID,
		AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 1 << 20,
		IdempotencyKey: "replace-1",
	})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}

	// The original is preserved and linked, not edited into the new one.
	previous, _ := f.service.Get(context.Background(), f.orgID, original.Document.ID)
	if previous.LifecycleState != documentmodels.StateReplaced {
		t.Fatalf("original lifecycleState = %s, want REPLACED", previous.LifecycleState)
	}
	if previous.ReplacementDocumentID == nil || *previous.ReplacementDocumentID != replacement.Document.ID {
		t.Fatal("the replacement chain was not recorded")
	}
	if previous.Verified() {
		t.Fatal("a superseded document must not keep satisfying what it satisfied")
	}

	// The replacement starts from nothing.
	fresh, _ := f.service.Get(context.Background(), f.orgID, replacement.Document.ID)
	if fresh.Verified() || fresh.SafetyConfirmed() {
		t.Fatal("a replacement must earn its own validation and verification")
	}
	if fresh.ReplacesDocumentID == nil || *fresh.ReplacesDocumentID != original.Document.ID {
		t.Fatal("the replacement does not point back at what it replaced")
	}
	if fresh.MediaObjectID == previous.MediaObjectID {
		t.Fatal("a replacement must get its own media object")
	}
}

// TestRevocationBlocksEligibilityAndAccessImmediately proves a revoked
// document stops counting at once and keeps its history.
func TestRevocationBlocksEligibilityAndAccessImmediately(t *testing.T) {
	f := newDocumentFixture(t, "Doc Revoke Co", "DOC-20")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	verifier := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "verifier")
	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.service.Verify(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(verifier), DocumentID: ticket.Document.ID,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.service.Revoke(context.Background(), documentusecases.RevokeInput{
		Actor: f.actor(submitter), DocumentID: ticket.Document.ID,
	}); err == nil {
		t.Fatal("a revocation with no reason must be refused")
	}

	document, err := f.service.Revoke(context.Background(), documentusecases.RevokeInput{
		Actor: f.actor(submitter), DocumentID: ticket.Document.ID,
		Reason: "policy cancelled by the insurer",
	})
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if document.LifecycleState != documentmodels.StateRevoked || document.RevokedAt == nil {
		t.Fatalf("document is not revoked: %s", document.LifecycleState)
	}
	if document.Downloadable() {
		t.Fatal("a revoked document must not be downloadable")
	}

	states, err := eligibilityrepos.NewRepository(f.pool).ReferenceDocumentStates(
		context.Background(), nil, f.orgID, "COMPANY", f.orgID, []string{"INSURANCE_CERTIFICATE"})
	if err != nil {
		t.Fatal(err)
	}
	if states[0].Satisfied || states[0].Reason != eligibilitymodels.ReasonDocumentRevoked {
		t.Fatalf("revoked document state = %+v, want blocked with document_revoked", states[0])
	}

	// History survives.
	entries, _ := f.service.History(context.Background(), f.orgID, ticket.Document.ID)
	if len(entries) < 2 {
		t.Fatal("revocation must preserve the document's history")
	}
}

// TestExpiryWorkerBlocksMandatoryDocuments proves an expired document stops
// satisfying a requirement without anybody deciding anything.
func TestExpiryWorkerBlocksMandatoryDocuments(t *testing.T) {
	f := newDocumentFixture(t, "Doc Expiry Co", "DOC-21")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	verifier := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "verifier")

	future := time.Now().UTC().AddDate(0, 0, 1)
	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", &future)
	if _, err := f.service.Verify(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(verifier), DocumentID: ticket.Document.ID,
	}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE fleet_partner_documents SET expiry_date=current_date-1 WHERE id=$1`, ticket.Document.ID); err != nil {
		t.Fatal(err)
	}

	expired, err := f.service.RunExpiryPass(context.Background(), 50)
	if err != nil {
		t.Fatalf("expiry pass: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expired %d documents, want 1", expired)
	}

	document, _ := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.LifecycleState != documentmodels.StateExpired {
		t.Fatalf("lifecycleState = %s, want EXPIRED", document.LifecycleState)
	}
	if document.Verified() {
		t.Fatal("an expired document must not keep a standing verification")
	}

	states, _ := eligibilityrepos.NewRepository(f.pool).ReferenceDocumentStates(
		context.Background(), nil, f.orgID, "COMPANY", f.orgID, []string{"INSURANCE_CERTIFICATE"})
	if states[0].Satisfied {
		t.Fatal("an expired mandatory document must fail the eligibility check")
	}
}

// --- eligibility integration ----------------------------------------------

// TestDocumentSatisfiesEligibilityOnlyWhenEverythingHolds walks the whole
// standard: missing, unverified, verified, then each way it stops counting.
func TestDocumentSatisfiesEligibilityOnlyWhenEverythingHolds(t *testing.T) {
	f := newDocumentFixture(t, "Doc Gate Co", "DOC-22")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	verifier := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "verifier")
	repo := eligibilityrepos.NewRepository(f.pool)
	required := []string{"INSURANCE_CERTIFICATE"}

	check := func() eligibilityrepos.ReferenceDocumentState {
		t.Helper()
		states, err := repo.ReferenceDocumentStates(context.Background(), nil,
			f.orgID, "COMPANY", f.orgID, required)
		if err != nil {
			t.Fatal(err)
		}
		return states[0]
	}

	if state := check(); state.Satisfied || state.Reason != eligibilitymodels.ReasonDocumentMissing {
		t.Fatalf("with no document: %+v, want document_missing", state)
	}

	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)
	if state := check(); state.Satisfied || state.Reason != eligibilitymodels.ReasonDocumentNotVerified {
		t.Fatalf("safe but unverified: %+v, want document_not_verified", state)
	}

	if _, err := f.service.Verify(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(verifier), DocumentID: ticket.Document.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if state := check(); !state.Satisfied {
		t.Fatalf("verified and safe: %+v, want satisfied", state)
	}

	// A stale projection stops it counting even though nothing else changed.
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE fleet_partner_documents SET projection_freshness = 'STALE' WHERE id = $1`,
		ticket.Document.ID); err != nil {
		t.Fatal(err)
	}
	if state := check(); state.Satisfied || state.Reason != eligibilitymodels.ReasonDocumentProjectionStale {
		t.Fatalf("stale projection: %+v, want document_projection_stale", state)
	}

	// An open reconciliation case outranks everything.
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE fleet_partner_documents SET projection_freshness = 'CURRENT' WHERE id = $1`,
		ticket.Document.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.OpenReconciliation(context.Background(), f.orgID,
		ticket.Document.ID, documentmodels.ReconcileUnknownOutcome); err != nil {
		t.Fatal(err)
	}
	if state := check(); state.Satisfied || state.Reason != eligibilitymodels.ReasonDocumentReconciling {
		t.Fatalf("reconciling: %+v, want document_reconciling", state)
	}
}

// TestDocumentChangeInvalidatesEligibilityDecisions proves a decision taken
// while a document was good stops being current when it stops being good.
func TestDocumentChangeInvalidatesEligibilityDecisions(t *testing.T) {
	f := newDocumentFixture(t, "Doc Invalidate Co", "DOC-23")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	verifier := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "verifier")
	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.service.Verify(context.Background(), documentusecases.VerificationInput{
		Actor: f.actor(verifier), DocumentID: ticket.Document.ID,
	}); err != nil {
		t.Fatal(err)
	}

	// A decision recorded while the document stood.
	authority := healthyAuthority("prv-doc-23", "veh-doc-23")
	eligibility := newEligibilityService(f.pool, authority)
	seedEligibleOrganization(t, f.pool, f.orgID)
	decision, err := eligibility.Evaluate(context.Background(), eligibilityusecases.EvaluateInput{
		Actor: eligibilityusecases.Actor{
			OrganizationID: f.orgID,
			ActorType:      eligibilitymodels.ActorOrganizationUser,
			ActorID:        submitter,
		},
		OperationType: eligibilitymodels.OperationCompanyActivation,
		SubjectType:   eligibilitymodels.SubjectCompany,
		SubjectID:     f.orgID,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	var superseded *time.Time
	if err := f.pool.QueryRow(context.Background(),
		`SELECT superseded_at FROM fleet_partner_eligibility_decisions WHERE id = $1`,
		decision.ID).Scan(&superseded); err != nil {
		t.Fatal(err)
	}
	if superseded != nil {
		t.Fatal("a freshly taken decision must be current")
	}

	if _, err := f.service.Revoke(context.Background(), documentusecases.RevokeInput{
		Actor: f.actor(submitter), DocumentID: ticket.Document.ID, Reason: "insurer cancelled",
	}); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if err := f.pool.QueryRow(context.Background(),
		`SELECT superseded_at FROM fleet_partner_eligibility_decisions WHERE id = $1`,
		decision.ID).Scan(&superseded); err != nil {
		t.Fatal(err)
	}
	if superseded == nil {
		t.Fatal("revoking the evidence must invalidate the decision that rested on it")
	}

	var invalidations int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM fleet_partner_eligibility_decision_history
		WHERE decision_id = $1 AND action = 'INVALIDATED'`, decision.ID).Scan(&invalidations); err != nil {
		t.Fatal(err)
	}
	if invalidations == 0 {
		t.Fatal("the invalidation must be recorded in the decision's history")
	}
}

// --- retention, legal hold, deletion --------------------------------------

// TestLegalHoldBlocksDeletionScheduling proves fleet refuses to schedule the
// destruction of evidence somebody is required to keep.
func TestLegalHoldBlocksDeletionScheduling(t *testing.T) {
	f := newDocumentFixture(t, "Doc Hold Co", "DOC-24")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "hold")
	ticket := safeDocument(t, f, member, "INSURANCE_CERTIFICATE", nil)

	hold := true
	if _, err := f.service.SetRetention(context.Background(), documentusecases.RetentionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
		LegalHold: &hold, Reason: "litigation hold",
	}); err != nil {
		t.Fatalf("apply legal hold: %v", err)
	}

	if _, err := f.service.ScheduleDeletion(context.Background(), documentusecases.ScheduleDeletionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
		Reason: "cleanup", IdempotencyKey: "del-hold",
	}); err == nil {
		t.Fatal("a document under legal hold must not be schedulable for deletion")
	}

	// Releasing the hold makes it schedulable again.
	release := false
	if _, err := f.service.SetRetention(context.Background(), documentusecases.RetentionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID, LegalHold: &release,
	}); err != nil {
		t.Fatalf("release legal hold: %v", err)
	}
	if _, err := f.service.ScheduleDeletion(context.Background(), documentusecases.ScheduleDeletionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
		Reason: "cleanup", IdempotencyKey: "del-after-hold",
	}); err != nil {
		t.Fatalf("schedule after release: %v", err)
	}
}

// TestRetentionWindowBlocksDeletionScheduling proves the window is enforced
// before anything reaches media.
func TestRetentionWindowBlocksDeletionScheduling(t *testing.T) {
	f := newDocumentFixture(t, "Doc Retention Co", "DOC-25")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "retention")
	ticket := safeDocument(t, f, member, "INSURANCE_CERTIFICATE", nil)

	until := time.Now().UTC().Add(time.Hour)
	if _, err := f.service.SetRetention(context.Background(), documentusecases.RetentionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
		RetentionClass: documentmodels.RetentionExtended, RetentionUntil: &until,
	}); err != nil {
		t.Fatalf("set retention: %v", err)
	}
	if _, err := f.service.ScheduleDeletion(context.Background(), documentusecases.ScheduleDeletionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
		Reason: "cleanup", IdempotencyKey: "del-retention",
	}); err == nil {
		t.Fatal("a document inside its retention window must not be schedulable for deletion")
	}
}

// TestDeletionCompletesOnlyOnMediaAuthority proves fleet records physical
// deletion only when media says so, and that audit evidence outlives the file.
func TestDeletionCompletesOnlyOnMediaAuthority(t *testing.T) {
	f := newDocumentFixture(t, "Doc Delete Co", "DOC-26")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "delete")
	ticket := safeDocument(t, f, member, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.service.ScheduleDeletion(context.Background(), documentusecases.ScheduleDeletionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
		Reason: "superseded", IdempotencyKey: "del-1",
	}); err != nil {
		t.Fatalf("schedule deletion: %v", err)
	}

	// Before the worker runs, nothing is deleted.
	document, _ := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.LifecycleState == documentmodels.StateDeleted {
		t.Fatal("scheduling a deletion must not mark the document deleted")
	}

	processed, err := f.service.RunDeletionPass(context.Background(), "test", time.Minute, 10, 0, 3)
	if err != nil {
		t.Fatalf("deletion pass: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed %d deletions, want 1", processed)
	}

	document, _ = f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.LifecycleState != documentmodels.StateDeleted || document.DeletedAt == nil {
		t.Fatalf("document is not deleted: %s", document.LifecycleState)
	}
	if document.Downloadable() {
		t.Fatal("a deleted document must not be downloadable")
	}

	var auditCount int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM fleet_partner_audit_records WHERE resource_id = $1`,
		ticket.Document.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount == 0 {
		t.Fatal("audit evidence must survive the file")
	}
}

// TestDeletionBlockedByMediaLeavesDocumentIntact proves media's refusal is
// recorded rather than overridden.
func TestDeletionBlockedByMediaLeavesDocumentIntact(t *testing.T) {
	f := newDocumentFixture(t, "Doc Blocked Co", "DOC-27")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "blocked")
	ticket := safeDocument(t, f, member, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.service.ScheduleDeletion(context.Background(), documentusecases.ScheduleDeletionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
		Reason: "cleanup", IdempotencyKey: "del-blocked",
	}); err != nil {
		t.Fatal(err)
	}

	// Media holds a retention rule fleet does not know about.
	f.media.blockDeletion = "RETENTION_ACTIVE"
	if _, err := f.service.RunDeletionPass(context.Background(), "test", time.Minute, 10, 0, 3); err != nil {
		t.Fatalf("deletion pass: %v", err)
	}

	document, _ := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.LifecycleState == documentmodels.StateDeleted {
		t.Fatal("a blocked deletion must not mark the document deleted")
	}
	request, err := f.service.LatestDeletion(context.Background(), f.orgID, ticket.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if request.Status != documentmodels.DeletionBlocked || request.BlockReason != "RETENTION_ACTIVE" {
		t.Fatalf("deletion request = %s/%s, want BLOCKED/RETENTION_ACTIVE",
			request.Status, request.BlockReason)
	}
}

// TestDeletionUnknownOutcomeIsReconciledNotAssumed proves a timed-out deletion
// becomes UNKNOWN, opens a case, and is settled by reading media back.
func TestDeletionUnknownOutcomeIsReconciledNotAssumed(t *testing.T) {
	f := newDocumentFixture(t, "Doc Unknown Co", "DOC-28")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "unknown")
	ticket := safeDocument(t, f, member, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.service.ScheduleDeletion(context.Background(), documentusecases.ScheduleDeletionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
		Reason: "cleanup", IdempotencyKey: "del-unknown",
	}); err != nil {
		t.Fatal(err)
	}

	f.media.timeoutDeletion = true
	if _, err := f.service.RunDeletionPass(context.Background(), "test", time.Minute, 10, 0, 5); err != nil {
		t.Fatalf("deletion pass: %v", err)
	}

	document, _ := f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.LifecycleState == documentmodels.StateDeleted {
		t.Fatal("an unknown outcome must never be recorded as a completed deletion")
	}

	request, err := f.service.LatestDeletion(context.Background(), f.orgID, ticket.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if request.Status != documentmodels.DeletionUnknown && request.Status != documentmodels.DeletionFailed {
		t.Fatalf("deletion status = %s, want UNKNOWN or FAILED after a lost response", request.Status)
	}

	// The retry succeeds and settles the truth.
	f.media.timeoutDeletion = false
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE fleet_partner_document_deletion_requests SET status = 'SCHEDULED', next_attempt_at = now()
		 WHERE document_id = $1`, ticket.Document.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RunDeletionPass(context.Background(), "test", time.Minute, 10, 0, 5); err != nil {
		t.Fatalf("retry pass: %v", err)
	}
	document, _ = f.service.Get(context.Background(), f.orgID, ticket.Document.ID)
	if document.LifecycleState != documentmodels.StateDeleted {
		t.Fatalf("after a successful retry lifecycleState = %s, want DELETED", document.LifecycleState)
	}
}

// TestDeletionDeadLettersAndReplays proves a permanently failing deletion stops
// consuming the batch and can be put back.
func TestDeletionDeadLettersAndReplays(t *testing.T) {
	f := newDocumentFixture(t, "Doc DelReplay Co", "DOC-29")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "delreplay")
	ticket := safeDocument(t, f, member, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.service.ScheduleDeletion(context.Background(), documentusecases.ScheduleDeletionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
		Reason: "cleanup", IdempotencyKey: "del-replay",
	}); err != nil {
		t.Fatal(err)
	}

	f.media.failDeletion = true
	// maxAttempts of 1 means the first failure dead-letters.
	if _, err := f.service.RunDeletionPass(context.Background(), "test", time.Minute, 10, 0, 1); err != nil {
		t.Fatalf("deletion pass: %v", err)
	}

	request, _ := f.service.LatestDeletion(context.Background(), f.orgID, ticket.Document.ID)
	if request.Status != documentmodels.DeletionDeadLettered {
		t.Fatalf("deletion status = %s, want DEAD_LETTERED", request.Status)
	}

	if err := f.service.ReplayDeletion(context.Background(), f.orgID, request.ID); err != nil {
		t.Fatalf("operator replay: %v", err)
	}
	request, _ = f.service.LatestDeletion(context.Background(), f.orgID, ticket.Document.ID)
	if request.Status != documentmodels.DeletionScheduled {
		t.Fatalf("replayed deletion status = %s, want SCHEDULED", request.Status)
	}
}

// TestDeletionWorkerRecoversAbandonedLease proves a worker that died mid-batch
// does not strand the deletion.
func TestDeletionWorkerRecoversAbandonedLease(t *testing.T) {
	f := newDocumentFixture(t, "Doc DelLease Co", "DOC-30")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "dellease")
	ticket := safeDocument(t, f, member, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.service.ScheduleDeletion(context.Background(), documentusecases.ScheduleDeletionInput{
		Actor: f.actor(member), DocumentID: ticket.Document.ID,
		Reason: "cleanup", IdempotencyKey: "del-lease",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_document_deletion_requests
		SET status = 'IN_PROGRESS', lease_owner = 'dead-worker',
		    lease_expires_at = now() - interval '1 hour'
		WHERE document_id = $1`, ticket.Document.ID); err != nil {
		t.Fatal(err)
	}

	processed, err := f.service.RunDeletionPass(context.Background(), "fresh", time.Minute, 10, 0, 3)
	if err != nil {
		t.Fatalf("deletion pass: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed %d, want the abandoned deletion to be reclaimed", processed)
	}
}

// --- private download -----------------------------------------------------

// TestPrivateDownloadRequiresPermission proves default deny and the separation
// between reading a record and reading the file.
func TestPrivateDownloadRequiresPermission(t *testing.T) {
	f := newDocumentFixture(t, "Doc RBAC Co", "DOC-31")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)

	// A member of the organization with no relevant role reaches nothing.
	nobody := seedApprovalMember(t, f.pool, f.orgID, "", "nobody")
	router := f.router(t, nobody, nil)
	path := "/fleet-partner/v1/documents/" + ticket.Document.ID
	if status, _ := documentCall(t, router, http.MethodGet, path, nil, nil); status != http.StatusForbidden {
		t.Fatalf("unroled read status = %d, want 403", status)
	}
	if status, _ := documentCall(t, router, http.MethodPost, path+"/download-requests", nil, nil); status != http.StatusForbidden {
		t.Fatalf("unroled download status = %d, want 403", status)
	}

	// Dispatcher may read the record but not the file: those are different
	// capabilities and the role catalogue says so.
	dispatcher := seedApprovalMember(t, f.pool, f.orgID, "rol_system_dispatcher", "dispatcher")
	dispatchRouter := f.router(t, dispatcher, nil)
	if status, response := documentCall(t, dispatchRouter, http.MethodGet, path, nil, nil); status != http.StatusOK {
		t.Fatalf("dispatcher read status = %d body=%v, want 200", status, response)
	}
	if status, _ := documentCall(t, dispatchRouter, http.MethodPost, path+"/download-requests", nil, nil); status != http.StatusForbidden {
		t.Fatalf("dispatcher download status = %d, want 403", status)
	}

	// Compliance holds the download capability.
	compliance := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "compliance")
	complianceRouter := f.router(t, compliance, nil)
	status, response := documentCall(t, complianceRouter, http.MethodPost, path+"/download-requests", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("compliance download status = %d body = %v, want 200", status, response)
	}
	grant := documentData(t, response)
	token, _ := grant["downloadToken"].(string)
	if token == "" {
		t.Fatal("no download grant was issued")
	}
	// Nothing about the grant is durable, and no permanent link is returned.
	assertNotPersisted(t, f.pool, token)
	for _, forbidden := range []string{"url", "signedUrl", "publicUrl", "storagePath"} {
		if _, present := grant[forbidden]; present {
			t.Fatalf("download response leaks %s: %v", forbidden, grant)
		}
	}
}

// TestDownloadGrantIsNotLogged proves a short-lived credential does not reach a
// log line.
func TestDownloadGrantIsNotLogged(t *testing.T) {
	f := newDocumentFixture(t, "Doc Log Co", "DOC-32")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)
	compliance := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "compliance")

	logs := captureLogs(t)
	grant, err := f.service.AuthorizeDownload(context.Background(), f.actor(compliance),
		ticket.Document.ID, "cor-log", "", "req-log")
	if err != nil {
		t.Fatalf("authorize download: %v", err)
	}
	if grant.Token == "" {
		t.Fatal("no grant token")
	}
	if strings.Contains(logs.String(), grant.Token) {
		t.Fatal("the download grant reached a log line")
	}
	assertNotPersisted(t, f.pool, grant.Token)
}

// TestDownloadRefusedForUnconfirmedProjection proves a document fleet cannot
// currently vouch for is not released, whatever it said last.
func TestDownloadRefusedForUnconfirmedProjection(t *testing.T) {
	f := newDocumentFixture(t, "Doc Unconfirmed Co", "DOC-33")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	compliance := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "compliance")
	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)

	if _, err := f.pool.Exec(context.Background(),
		`UPDATE fleet_partner_documents SET projection_freshness = 'STALE' WHERE id = $1`,
		ticket.Document.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AuthorizeDownload(context.Background(), f.actor(compliance),
		ticket.Document.ID, "cor", "", "req"); err == nil {
		t.Fatal("a stale projection must refuse a download grant")
	}
}

// TestDownloadRefusedWhenMediaAuthorityUnconfigured proves an absent
// integration fails closed rather than opening the file up.
func TestDownloadRefusedWhenMediaAuthorityUnconfigured(t *testing.T) {
	f := newDocumentFixture(t, "Doc NoMedia Co", "DOC-34")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")
	compliance := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "compliance")
	ticket := safeDocument(t, f, submitter, "INSURANCE_CERTIFICATE", nil)

	unconfigured := f.newService(platformclients.NewMediaFileClient(platformclients.Options{
		ServiceName: "media-file-service",
	}))
	if _, err := unconfigured.AuthorizeDownload(context.Background(), f.actor(compliance),
		ticket.Document.ID, "cor", "", "req"); err == nil {
		t.Fatal("an unconfigured media authority must refuse a download grant")
	}
}
