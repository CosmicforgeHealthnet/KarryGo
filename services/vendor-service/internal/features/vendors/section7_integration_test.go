package vendors

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/audit"
	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/events"
)

func section7Pool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	if e = database.ApplyMigrations(ctx, pool, "../../../migrations"); e != nil {
		pool.Close()
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `TRUNCATE vendor_businesses,vendor_outbox,vendor_inbox_processed_events,vendor_service_nonces,vendor_idempotency_keys,vendor_audit_log,vendor_staff_role_assignments,vendor_staff,vendor_users RESTART IDENTITY CASCADE`); e != nil {
		pool.Close()
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	return ctx, pool
}

func verified(ref string, version int64, expiry *time.Time) VerificationCaseResult {
	return VerificationCaseResult{SourceRef: ref, SourceVersion: version, Status: VerificationVerified, ObservedAt: time.Now().UTC(), ExpiresAt: expiry}
}

func pendingReview(ref string, version int64) VerificationCaseResult {
	return VerificationCaseResult{SourceRef: ref, SourceVersion: version, Status: VerificationPendingReview, ObservedAt: time.Now().UTC()}
}

func TestSection7ActivationMatrixExpirySuspensionAndSourceIntegrity(t *testing.T) {
	ctx, pool := section7Pool(t)
	_, e := pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES('vu_s7','s7@example.test');INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('vs_s7','vendor_s7','vu_s7','active')`)
	if e != nil {
		t.Fatal(e)
	}
	svc := NewService(pool, allowAll{}, integrationAudit{recorder: audit.NewPostgresRecorder(pool)}, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
	actor := Actor{StaffID: "vs_s7", VendorID: "vendor_s7", SessionID: "session_s7", CorrelationID: "corr_s7"}
	business, e := svc.CreateBusiness(ctx, actor, Business{LegalName: "Section Seven Foods Limited", Currency: "NGN", Timezone: "Africa/Lagos"})
	if e != nil {
		t.Fatal(e)
	}
	representative, e := svc.PutRepresentative(ctx, actor, RepresentativeInput{Capacity: "OWNER", FirstName: "Ada", LastName: "Okafor", Email: "ada@example.test", Phone: "+2348000000000", Authorized: true, ExpectedVersion: 0})
	if e != nil {
		t.Fatal(e)
	}
	restaurant, e := svc.CreateCapability(ctx, actor, business.ID, CapabilityRestaurant)
	if e != nil {
		t.Fatal(e)
	}
	shop, e := svc.CreateCapability(ctx, actor, business.ID, CapabilityShopGrocery)
	if e != nil {
		t.Fatal(e)
	}
	brand, e := svc.CreateBrand(ctx, actor, business.ID, Brand{DisplayName: "Section Seven"})
	if e != nil {
		t.Fatal(e)
	}
	branch1, e := svc.CreateBranch(ctx, actor, business.ID, Branch{BrandID: brand.ID, DisplayName: "Lekki", Address: Address{Line1: "1 Admiralty Way", City: "Lagos", Region: "Lagos", CountryCode: "NG"}, Latitude: 6.4474, Longitude: 3.4723, Timezone: "Africa/Lagos"})
	if e != nil {
		t.Fatal(e)
	}
	branch2, e := svc.CreateBranch(ctx, actor, business.ID, Branch{BrandID: brand.ID, DisplayName: "Ikeja", Address: Address{Line1: "10 Allen Avenue", City: "Lagos", Region: "Lagos", CountryCode: "NG"}, Latitude: 6.6018, Longitude: 3.3515, Timezone: "Africa/Lagos"})
	if e != nil {
		t.Fatal(e)
	}
	rest1, e := svc.PutBranchCapability(ctx, actor, business.ID, branch1.ID, restaurant.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	rest2, e := svc.PutBranchCapability(ctx, actor, business.ID, branch2.ID, restaurant.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	_, e = svc.PutBranchCapability(ctx, actor, business.ID, branch1.ID, shop.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.PutBranchCapability(ctx, actor, business.ID, branch2.ID, shop.ID, 0); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.SubmitOnboarding(ctx, actor); e != nil {
		t.Fatal(e)
	}
	apply := func(source, event string, result VerificationCaseResult, dimension VerificationDimension, subjectType, subjectRef string) {
		t.Helper()
		if _, err := svc.applyProjection(ctx, actor, source, result, dimension, subjectType, subjectRef, event); err != nil {
			t.Fatalf("apply %s/%s: %v", dimension, subjectRef, err)
		}
	}
	apply("verification-compliance-service", "evt-id", verified("case-id", 1, nil), DimensionIdentity, "REPRESENTATIVE", representative.ID)
	apply("verification-compliance-service", "evt-business", verified("case-business", 1, nil), DimensionBusiness, "BUSINESS", business.ID)
	r, e := svc.Readiness(ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	if r.Ready || !hasBlocker(r, "BANK_NOT_STARTED") {
		t.Fatalf("bank-pending matrix should block: %+v", r.Blockers)
	}
	apply("verification-compliance-service", "evt-bank-pending-review", pendingReview("case-bank-pending", 1), DimensionBank, "BUSINESS", business.ID)
	r, e = svc.Readiness(ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	if r.Ready || !hasBlocker(r, "BANK_PENDING_REVIEW") {
		t.Fatalf("mandatory BANK=PENDING_REVIEW must block business activation: %+v", r.Blockers)
	}
	var businessStatus BusinessStatus
	if e = pool.QueryRow(ctx, `SELECT status FROM vendor_businesses WHERE id=$1`, business.ID).Scan(&businessStatus); e != nil || businessStatus == BusinessActive {
		t.Fatalf("PENDING_REVIEW activated business status=%s err=%v", businessStatus, e)
	}
	apply("payment-wallet-service", "", verified("bank-profile-1", 1, nil), DimensionBank, "BUSINESS", business.ID)
	apply("verification-compliance-service", "evt-loc-1-pending-review", pendingReview("case-loc-1", 1), DimensionLocation, "BRANCH", branch1.ID)
	r, e = svc.Readiness(ctx, actor)
	if e != nil || r.Ready || !hasSubjectBlockerWithStatus(r, branch1.ID, "PENDING_REVIEW") {
		t.Fatalf("LOCATION=PENDING_REVIEW must block only its branch scope: readiness=%+v err=%v", r, e)
	}
	apply("verification-compliance-service", "evt-loc-1", verified("case-loc-1", 2, nil), DimensionLocation, "BRANCH", branch1.ID)
	apply("verification-compliance-service", "evt-loc-2", verified("case-loc-2", 1, nil), DimensionLocation, "BRANCH", branch2.ID)
	nextYear := time.Now().UTC().Add(365 * 24 * time.Hour)
	apply("verification-compliance-service", "evt-food-1-pending-review", pendingReview("case-food-1", 1), DimensionFoodKitchen, "BRANCH_CAPABILITY", rest1.ID)
	r, e = svc.Readiness(ctx, actor)
	if e != nil || r.Ready || !hasSubjectBlockerWithStatus(r, rest1.ID, "PENDING_REVIEW") {
		t.Fatalf("FOOD_KITCHEN=PENDING_REVIEW must block its branch-capability scope: readiness=%+v err=%v", r, e)
	}
	apply("verification-compliance-service", "evt-food-1", verified("case-food-1", 2, &nextYear), DimensionFoodKitchen, "BRANCH_CAPABILITY", rest1.ID)
	r, e = svc.Readiness(ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	if r.Ready || !hasSubjectBlocker(r, rest2.ID) {
		t.Fatalf("second branch Food/Kitchen must block: %+v", r.Blockers)
	}
	apply("verification-compliance-service", "evt-food-2", verified("case-food-2", 1, &nextYear), DimensionFoodKitchen, "BRANCH_CAPABILITY", rest2.ID)
	r, e = svc.Readiness(ctx, actor)
	if e != nil || !r.Ready || r.Progress != 100 {
		t.Fatalf("ready=%+v err=%v", r, e)
	}
	var statuses []string
	if e = pool.QueryRow(ctx, `SELECT ARRAY[(SELECT status FROM vendor_businesses WHERE id=$1),(SELECT status FROM vendor_capabilities WHERE id=$2),(SELECT status FROM vendor_branches WHERE id=$3),(SELECT status FROM vendor_branch_capabilities WHERE id=$4)]`, business.ID, restaurant.ID, branch1.ID, rest1.ID).Scan(&statuses); e != nil {
		t.Fatal(e)
	}
	for _, status := range statuses {
		if status != "ACTIVE" {
			t.Fatalf("activation did not atomically activate graph: %v", statuses)
		}
	}
	var activationEvents int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE event_type='vendor.business.activated' AND aggregate_id=$1`, business.ID).Scan(&activationEvents); e != nil || activationEvents != 1 {
		t.Fatalf("activation event count=%d err=%v", activationEvents, e)
	}
	if _, e = pool.Exec(ctx, `UPDATE vendor_verification_projections SET expires_at=now()-interval '1 second' WHERE source_ref='case-food-1'`); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.ExpireVerifications(ctx, 100); e != nil {
		t.Fatal(e)
	}
	list, e := svc.ListBranchCapabilities(ctx, actor, business.ID, branch1.ID)
	if e != nil {
		t.Fatal(e)
	}
	states := map[CapabilityType]BranchCapability{}
	for _, item := range list {
		states[item.CapabilityType] = item
	}
	if states[CapabilityRestaurant].Effective {
		t.Fatal("expired Food/Kitchen did not gate restaurant")
	}
	if !states[CapabilityShopGrocery].Effective {
		t.Fatalf("Food/Kitchen expiry incorrectly gated Shop: %+v", states[CapabilityShopGrocery])
	}
	suspended := VerificationCaseResult{SourceRef: "case-loc-2", SourceVersion: 2, Status: VerificationSuspended, ObservedAt: time.Now().UTC(), ReasonCode: "LOCATION_SUSPENDED"}
	apply("verification-compliance-service", "evt-loc-2-suspend", suspended, DimensionLocation, "BRANCH", branch2.ID)
	if _, e = svc.ApplyVerificationEvent(ctx, "verification-compliance-service", "evt-loc-2-suspend", business.ID, suspended, DimensionLocation, "BRANCH", branch2.ID); e != nil {
		t.Fatalf("duplicate event was not idempotent: %v", e)
	}
	_, e = svc.ApplyVerificationEvent(ctx, "verification-compliance-service", "evt-loc-stale", business.ID, verified("case-loc-2", 1, nil), DimensionLocation, "BRANCH", branch2.ID)
	if e == nil {
		t.Fatal("stale source version overwrote newer suspension")
	}
	var historyCount int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_verification_history WHERE source_ref='case-loc-2'`).Scan(&historyCount); e != nil || historyCount != 2 {
		t.Fatalf("history count=%d err=%v", historyCount, e)
	}
	var duplicateClaims int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_inbox_processed_events WHERE source_service='verification-compliance-service' AND event_id='evt-loc-2-suspend'`).Scan(&duplicateClaims); e != nil || duplicateClaims != 1 {
		t.Fatalf("event dedupe count=%d err=%v", duplicateClaims, e)
	}
}

func hasBlocker(r ActivationReadiness, want string) bool {
	for _, b := range r.Blockers {
		if b.Blocker == want {
			return true
		}
	}
	return false
}
func hasSubjectBlocker(r ActivationReadiness, subject string) bool {
	for _, b := range r.Blockers {
		if b.SubjectRef == subject {
			return true
		}
	}
	return false
}

func hasSubjectBlockerWithStatus(r ActivationReadiness, subject, status string) bool {
	for _, b := range r.Blockers {
		if b.SubjectRef == subject && b.Status == status {
			return true
		}
	}
	return false
}

func TestSection7MediaValidationAndSpoofProtection(t *testing.T) {
	if validDimension(VerificationDimension("VERIFIED")) {
		t.Fatal("status spoof accepted as dimension")
	}
	if validSubject("ADMIN") {
		t.Fatal("unsupported verification subject accepted")
	}
	if opaqueMediaRefPattern.MatchString("../../secret") {
		t.Fatal("unsafe media reference accepted")
	}
	if err := validateRepresentative(RepresentativeInput{Capacity: "ADMIN", FirstName: "A", LastName: "B", Email: "not-email", Phone: "1"}); err == nil {
		t.Fatal("invalid representative accepted")
	}
}

type verificationStub struct{ result VerificationCaseResult }

func (s *verificationStub) CreateCase(context.Context, VerificationCaseInput) (VerificationCaseResult, error) {
	return s.result, nil
}
func (s *verificationStub) GetCaseByClientReference(context.Context, string) (VerificationCaseResult, error) {
	return s.result, nil
}

type mediaStub struct{}

func (mediaStub) GetObject(_ context.Context, businessID, mediaRef string) (MediaObject, error) {
	return MediaObject{ID: mediaRef, OwnerReference: businessID, UploadStatus: "COMPLETED", SafetyStatus: "SAFE", DeletionState: "ACTIVE", SourceVersion: 1}, nil
}

func TestSection7SubmissionLifecycleReadBackAndReplay(t *testing.T) {
	ctx, pool := section7Pool(t)
	_, err := pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES('vu_submit','submit@example.test');INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('vs_submit','vendor_submit','vu_submit','active')`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(pool, allowAll{}, integrationAudit{recorder: audit.NewPostgresRecorder(pool)}, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
	actor := Actor{StaffID: "vs_submit", VendorID: "vendor_submit", SessionID: "session_submit"}
	if _, err = svc.CreateBusiness(ctx, actor, Business{LegalName: "Submission Lifecycle Limited", Currency: "NGN", Timezone: "Africa/Lagos"}); err != nil {
		t.Fatal(err)
	}
	mediaRef := "med_1234567890abcdef"
	svc.SetOnboardingDependencies(nil, nil, mediaStub{}, nil)
	document, err := svc.CreateDocument(ctx, actor, DocumentInput{Dimension: DimensionBusiness, MediaRef: mediaRef, DocumentType: "INCORPORATION_CERTIFICATE", ExpectedVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	input := SubmitVerificationInput{Dimension: DimensionBusiness, SubjectType: "BUSINESS", SubjectRef: actor.VendorID, DocumentRefs: []string{document.ID}}
	if _, err = svc.SubmitVerification(ctx, actor, input, "disabled-key"); err == nil {
		t.Fatal("disabled verification integration accepted a submission")
	}
	var operationCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_verification_operations`).Scan(&operationCount); err != nil || operationCount != 0 {
		t.Fatalf("disabled integration wrote operations=%d err=%v", operationCount, err)
	}
	authority := &verificationStub{result: VerificationCaseResult{SourceRef: "case-submit", SourceVersion: 1, Status: VerificationPendingReview, ObservedAt: time.Now().UTC()}}
	svc.SetOnboardingDependencies(authority, nil, mediaStub{}, nil)
	if _, err = svc.SubmitVerification(ctx, actor, input, "enabled-key"); err != nil {
		t.Fatal(err)
	}
	var documentStatus, operationStatus, operationID string
	if err = pool.QueryRow(ctx, `SELECT d.status,o.status,o.id FROM vendor_onboarding_documents d CROSS JOIN vendor_verification_operations o WHERE d.id=$1`, document.ID).Scan(&documentStatus, &operationStatus, &operationID); err != nil {
		t.Fatal(err)
	}
	if documentStatus != "SUBMITTED" || operationStatus != "PENDING" {
		t.Fatalf("document=%s operation=%s", documentStatus, operationStatus)
	}
	authority.result = verified("case-submit", 2, nil)
	if _, err = pool.Exec(ctx, `UPDATE vendor_verification_operations SET next_attempt_at=now()`); err != nil {
		t.Fatal(err)
	}
	if err = svc.ReconcileVerifications(ctx, "test-reconciler", 10); err != nil {
		t.Fatal(err)
	}
	var projectionStatus VerificationState
	if err = pool.QueryRow(ctx, `SELECT p.status,o.status FROM vendor_verification_projections p JOIN vendor_verification_operations o ON o.source_ref=p.source_ref WHERE p.source_ref='case-submit'`).Scan(&projectionStatus, &operationStatus); err != nil {
		t.Fatal(err)
	}
	if projectionStatus != VerificationVerified || operationStatus != "COMPLETED" {
		t.Fatalf("projection=%s operation=%s", projectionStatus, operationStatus)
	}
	if _, err = pool.Exec(ctx, `UPDATE vendor_verification_operations SET status='DEAD_LETTER' WHERE id=$1`, operationID); err != nil {
		t.Fatal(err)
	}
	if err = svc.ReplayVerificationOperation(ctx, operationID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM vendor_verification_operations WHERE id=$1`, operationID).Scan(&operationStatus); err != nil || operationStatus != "PENDING" {
		t.Fatalf("replay status=%s err=%v", operationStatus, err)
	}
}
