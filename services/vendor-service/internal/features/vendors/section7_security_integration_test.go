package vendors

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/audit"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type rejectingStepUp struct{}

func (rejectingStepUp) ConsumeGrant(context.Context, string, string, string, string, string) error {
	return context.Canceled
}

type acceptingStepUp struct{}

func (acceptingStepUp) ConsumeGrant(context.Context, string, string, string, string, string) error {
	return nil
}

type countingBank struct{ calls int }

func (b *countingBank) ResolveAndRegister(context.Context, string, string, string, string) (BankAuthorityResult, error) {
	b.calls++
	return BankAuthorityResult{ExternalRef: "bank-safe", BankCode: "058", MaskedAccount: "******6789", ResolvedAccountName: "Safe Name", SourceVersion: 1, Status: VerificationVerified, ObservedAt: time.Now().UTC()}, nil
}

type concurrentBank struct {
	mu    sync.Mutex
	calls int
}

func (b *concurrentBank) ResolveAndRegister(context.Context, string, string, string, string) (BankAuthorityResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	return BankAuthorityResult{ExternalRef: "bank-concurrent", BankCode: "058", MaskedAccount: "******6789", ResolvedAccountName: "Safe Name", SourceVersion: int64(b.calls), Status: VerificationVerified, ObservedAt: time.Now().UTC()}, nil
}
func (b *concurrentBank) GetProfile(context.Context, string) (BankAuthorityResult, error) {
	return BankAuthorityResult{}, context.Canceled
}
func (b *countingBank) GetProfile(context.Context, string) (BankAuthorityResult, error) {
	return BankAuthorityResult{}, context.Canceled
}

type branchOnlyAuth struct{ branchID string }

func (a branchOnlyAuth) Authorize(_ context.Context, _ string, _ string, branchID *string) (bool, error) {
	return branchID != nil && *branchID == a.branchID, nil
}

func signedCallback(t *testing.T, router http.Handler, caller string, secret []byte, nonce string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/internal/vendor/v1/verification-events", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(serviceauth.HeaderNonce, nonce)
	if err := serviceauth.SignRequest(request, caller, secret, body, time.Now()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestSection7MandatorySecurityMatrix(t *testing.T) {
	ctx, pool := section7Pool(t)
	_, err := pool.Exec(ctx, `
		INSERT INTO vendor_users(id,email) VALUES('vu_sec_a','sec-a@example.test'),('vu_sec_b','sec-b@example.test');
		INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('vs_sec_a','vendor_sec_a','vu_sec_a','active'),('vs_sec_b','vendor_sec_b','vu_sec_b','active')`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(pool, allowAll{}, integrationAudit{recorder: audit.NewPostgresRecorder(pool)}, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
	authority := &verificationStub{result: verified("case-created", 1, nil)}
	svc.SetOnboardingDependencies(authority, nil, mediaStub{}, nil)
	actorA := Actor{StaffID: "vs_sec_a", VendorID: "vendor_sec_a", SessionID: "session-a"}
	actorB := Actor{StaffID: "vs_sec_b", VendorID: "vendor_sec_b", SessionID: "session-b"}
	for _, actor := range []Actor{actorA, actorB} {
		if _, err = svc.CreateBusiness(ctx, actor, Business{LegalName: "Security Matrix " + actor.VendorID, Currency: "NGN", Timezone: "Africa/Lagos"}); err != nil {
			t.Fatal(err)
		}
	}
	representative, err := svc.PutRepresentative(ctx, actorA, RepresentativeInput{Capacity: "OWNER", FirstName: "Sensitive", LastName: "Person", Email: "private@example.test", Phone: "+2348000000001", Authorized: true, ExpectedVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	document, err := svc.CreateDocument(ctx, actorA, DocumentInput{RepresentativeID: representative.ID, Dimension: DimensionIdentity, MediaRef: "med_security_matrix_a", DocumentType: "IDENTITY_DOCUMENT", ExpectedVersion: 0})
	if err != nil {
		t.Fatal(err)
	}

	// Cross-tenant reads are empty/not-found and composite FKs prevent attaching
	// another Vendor's representative, branch, or brand to a document.
	if docs, readErr := svc.ListDocuments(ctx, actorB); readErr != nil || len(docs) != 0 {
		t.Fatalf("Vendor B observed Vendor A documents: docs=%v err=%v", docs, readErr)
	}
	if _, err = svc.CreateDocument(ctx, actorB, DocumentInput{RepresentativeID: representative.ID, Dimension: DimensionIdentity, MediaRef: "med_security_matrix_b", DocumentType: "IDENTITY_DOCUMENT", ExpectedVersion: 0}); err == nil {
		t.Fatal("cross-tenant representative document attachment succeeded")
	}
	if _, err = svc.SubmitVerification(ctx, actorB, SubmitVerificationInput{Dimension: DimensionBusiness, SubjectType: "BUSINESS", SubjectRef: actorA.VendorID, DocumentRefs: []string{document.ID}}, "cross-tenant"); err == nil {
		t.Fatal("Vendor B submitted Vendor A verification")
	}
	if _, err = svc.applyProjection(ctx, actorA, "verification-compliance-service", verified("case-a", 1, nil), DimensionBusiness, "BUSINESS", actorA.VendorID, "evt-sec-a"); err != nil {
		t.Fatal(err)
	}
	if projections, readErr := svc.ListVerifications(ctx, actorB); readErr != nil || len(projections) != 0 {
		t.Fatalf("Vendor B observed Vendor A verification: projections=%v err=%v", projections, readErr)
	}

	brand, err := svc.CreateBrand(ctx, actorA, actorA.VendorID, Brand{DisplayName: "Security Brand"})
	if err != nil {
		t.Fatal(err)
	}
	branchA, err := svc.CreateBranch(ctx, actorA, actorA.VendorID, Branch{BrandID: brand.ID, DisplayName: "Allowed", Address: Address{Line1: "1 Safe Street", City: "Lagos", Region: "Lagos", CountryCode: "NG"}, Latitude: 6.5, Longitude: 3.4, Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatal(err)
	}
	branchB, err := svc.CreateBranch(ctx, actorA, actorA.VendorID, Branch{BrandID: brand.ID, DisplayName: "Denied", Address: Address{Line1: "2 Safe Street", City: "Lagos", Region: "Lagos", CountryCode: "NG"}, Latitude: 6.6, Longitude: 3.5, Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatal(err)
	}
	branchDoc, err := svc.CreateDocument(ctx, actorA, DocumentInput{BranchID: branchB.ID, Dimension: DimensionLocation, MediaRef: "med_security_branch_b", DocumentType: "LOCATION_EVIDENCE", ExpectedVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	branchService := NewService(pool, branchOnlyAuth{branchID: branchA.ID}, integrationAudit{recorder: audit.NewPostgresRecorder(pool)}, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
	branchService.SetOnboardingDependencies(authority, nil, mediaStub{}, nil)
	if _, err = branchService.SubmitVerification(ctx, Actor{StaffID: actorA.StaffID, VendorID: actorA.VendorID}, SubmitVerificationInput{Dimension: DimensionLocation, SubjectType: "BRANCH", SubjectRef: branchB.ID, DocumentRefs: []string{branchDoc.ID}}, "branch-escape"); err == nil {
		t.Fatal("branch-scoped staff submitted another branch compliance record")
	}

	// Permission and step-up checks occur before Wallet is invoked.
	bank := &countingBank{}
	deniedBank := NewService(pool, denyAll{}, nil, nil)
	deniedBank.SetOnboardingDependencies(authority, bank, mediaStub{}, rejectingStepUp{})
	if _, err = deniedBank.PutBankProfile(ctx, actorA, "0123456789", "058", "bank-denied", "grant", 0); err == nil {
		t.Fatal("bank change without permission succeeded")
	}
	stepUpBank := NewService(pool, allowAll{}, integrationAudit{recorder: audit.NewPostgresRecorder(pool)}, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
	stepUpBank.SetOnboardingDependencies(authority, bank, mediaStub{}, rejectingStepUp{})
	if _, err = stepUpBank.PutBankProfile(ctx, actorA, "0123456789", "058", "bank-no-stepup", "grant", 0); err == nil || bank.calls != 0 {
		t.Fatalf("bank step-up gate err=%v wallet_calls=%d", err, bank.calls)
	}
	serializedBank := &concurrentBank{}
	concurrentService := NewService(pool, allowAll{}, integrationAudit{recorder: audit.NewPostgresRecorder(pool)}, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
	concurrentService.SetOnboardingDependencies(authority, serializedBank, mediaStub{}, acceptingStepUp{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, callErr := concurrentService.PutBankProfile(ctx, actorA, "0123456789", "058", key, "grant-"+key, 0)
			results <- callErr
		}(string(rune('a' + i)))
	}
	wg.Wait()
	close(results)
	successes := 0
	for callErr := range results {
		if callErr == nil {
			successes++
		}
	}
	if successes != 1 || serializedBank.calls != 1 {
		t.Fatalf("concurrent stale bank replacements successes=%d wallet_calls=%d", successes, serializedBank.calls)
	}

	// Representative PII is stored only in its dedicated table; generic event
	// payloads remain a safe projection.
	var eventPayload string
	if err = pool.QueryRow(ctx, `SELECT payload::text FROM vendor_outbox WHERE event_type='vendor.onboarding.representative_changed' AND aggregate_id=$1`, representative.ID).Scan(&eventPayload); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"Sensitive", "Person", "private@example.test", "+2348000000001"} {
		if strings.Contains(eventPayload, secret) {
			t.Fatalf("representative PII leaked in event payload: %s", eventPayload)
		}
	}
	var byteColumns int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='vendor_onboarding_documents' AND (data_type='bytea' OR column_name~*'(bytes|content|blob|account_number)')`).Scan(&byteColumns); err != nil || byteColumns != 0 {
		t.Fatalf("document table stores forbidden content columns=%d err=%v", byteColumns, err)
	}

	// Callback surface rejects public and wrong-caller traffic and consumes a
	// valid signed nonce once.
	gin.SetMode(gin.TestMode)
	verificationSecret := []byte("verification-secret")
	attackerSecret := []byte("attacker-secret")
	router := gin.New()
	RegisterInternalOnboardingRoutes(router, NewHandler(svc, nil, true, false), serviceauth.NewVerifier(serviceauth.Secrets{"verification-compliance-service": verificationSecret, "attacker-service": attackerSecret}, 5*time.Minute))
	event := verificationEventRequest{EventID: "evt-signed-security", BusinessID: actorA.VendorID, Dimension: DimensionBusiness, SubjectType: "BUSINESS", SubjectRef: actorA.VendorID, SourceRef: "case-signed-security", SourceVersion: 1, Status: VerificationPendingReview, ObservedAt: time.Now().UTC()}
	body, _ := json.Marshal(event)
	unsigned := httptest.NewRecorder()
	router.ServeHTTP(unsigned, httptest.NewRequest(http.MethodPost, "/internal/vendor/v1/verification-events", bytes.NewReader(body)))
	if unsigned.Code != http.StatusUnauthorized {
		t.Fatalf("public callback status=%d", unsigned.Code)
	}
	if response := signedCallback(t, router, "attacker-service", attackerSecret, "nonce-attacker", body); response.Code != http.StatusForbidden {
		t.Fatalf("wrong HMAC caller status=%d", response.Code)
	}
	invalidEvent := event
	invalidEvent.EventID = "evt-plain-pending-rejected"
	invalidEvent.Status = VerificationState("PENDING")
	invalidBody, _ := json.Marshal(invalidEvent)
	if response := signedCallback(t, router, "verification-compliance-service", verificationSecret, "nonce-plain-pending", invalidBody); response.Code != http.StatusBadRequest {
		t.Fatalf("plain PENDING verification status=%d body=%s", response.Code, response.Body.String())
	}
	if response := signedCallback(t, router, "verification-compliance-service", verificationSecret, "nonce-once", body); response.Code != http.StatusOK {
		t.Fatalf("valid callback status=%d body=%s", response.Code, response.Body.String())
	}
	var pendingReviewStatus VerificationState
	if err = pool.QueryRow(ctx, `SELECT status FROM vendor_verification_projections WHERE source_ref=$1`, event.SourceRef).Scan(&pendingReviewStatus); err != nil || pendingReviewStatus != VerificationPendingReview {
		t.Fatalf("callback projection status=%s err=%v", pendingReviewStatus, err)
	}
	if response := signedCallback(t, router, "verification-compliance-service", verificationSecret, "nonce-once", body); response.Code != http.StatusUnauthorized {
		t.Fatalf("replayed nonce status=%d", response.Code)
	}

	// Integration adapters are HTTP-only and never import another service's DB.
	for _, path := range []string{"../../../internal/integrations/verification/client.go", "../../../internal/integrations/wallet/client.go", "../../../internal/integrations/media/client.go"} {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		text := string(raw)
		if strings.Contains(text, "pgx") || strings.Contains(text, "internal/database") {
			t.Fatalf("cross-service database dependency found in %s", path)
		}
	}
}
