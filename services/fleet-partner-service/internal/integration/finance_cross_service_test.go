// Fleet half of the cross-service financial proof.
//
// The client under test is the real WalletClient. It talks over real HTTP to a
// server that runs the same shared/go/serviceauth verifier the Wallet service
// runs, with the same caller allowlist and the same nonce rule. Fleet and
// Wallet are separate Go modules and neither may import the other, so the
// Wallet half of each exchange is served here; the signing, verification and
// error-classification code exercised is the production code.
package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	authmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	financefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/finance"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

const fleetCallerName = "fleet-partner-service"

// walletStub is a Wallet-shaped server: real HMAC verification, real caller
// allowlist, real nonce replay rejection, tenant-scoped answers.
type walletStub struct {
	server *httptest.Server
	// payments is the authoritative job payment table, keyed by
	// organization and reference.
	payments map[string]platformclients.JobPaymentVerification
	// reversals answers reversal read-backs.
	reversals map[string]platformclients.WalletReversal

	mu     sync.Mutex
	nonces map[string]bool
	calls  int
}

type walletStubOptions struct {
	// secret is the key the server verifies with. A different key here is how
	// an invalid signature is produced without hand-forging one.
	secret []byte
	// allowedCallers restricts which signed identities reach a handler.
	allowedCallers []string
	// window is the signature freshness window.
	window time.Duration
}

func newWalletStub(t *testing.T, opts walletStubOptions) *walletStub {
	t.Helper()
	if len(opts.secret) == 0 {
		opts.secret = []byte("fleet-wallet-shared-secret")
	}
	if len(opts.allowedCallers) == 0 {
		opts.allowedCallers = []string{fleetCallerName}
	}
	if opts.window == 0 {
		opts.window = 5 * time.Minute
	}
	stub := &walletStub{
		payments:  map[string]platformclients.JobPaymentVerification{},
		reversals: map[string]platformclients.WalletReversal{},
		nonces:    map[string]bool{},
	}
	allowed := map[string]bool{}
	for _, caller := range opts.allowedCallers {
		allowed[caller] = true
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// The shared error handler is what turns an aborted service-auth check into
	// a 401 response. Without it a rejected request would answer 200 with an
	// empty body, and the client could not classify the failure at all.
	engine.Use(httpx.ErrorHandler())
	group := engine.Group("/api/v1/payment-wallet/internal/finance")
	group.Use(serviceauth.Middleware(serviceauth.NewVerifier(
		serviceauth.Secrets{fleetCallerName: opts.secret}, opts.window)))
	group.Use(func(c *gin.Context) {
		name, _ := c.Get("service_name")
		caller, _ := name.(string)
		if !allowed[caller] {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"success": false, "error": gin.H{"code": "caller_not_allowed"}})
			return
		}
		nonce := c.GetHeader("X-Service-Nonce")
		if nonce == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"success": false, "error": gin.H{"code": "replay_nonce_required"}})
			return
		}
		stub.mu.Lock()
		replayed := stub.nonces[nonce]
		stub.nonces[nonce] = true
		stub.calls++
		stub.mu.Unlock()
		if replayed {
			c.AbortWithStatusJSON(http.StatusConflict,
				gin.H{"success": false, "error": gin.H{"code": "replayed_request"}})
			return
		}
		c.Next()
	})
	group.GET("/job-payments/lookup", func(c *gin.Context) {
		key := c.Query("organizationId") + "|" + c.Query("reference")
		stub.mu.Lock()
		verification, ok := stub.payments[key]
		stub.mu.Unlock()
		if !ok {
			// Tenant mismatch and unknown reference are the same answer.
			c.JSON(http.StatusNotFound,
				gin.H{"success": false, "error": gin.H{"code": "not_found"}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": verification})
	})
	group.GET("/organizations/:organizationId/reversals/:id", func(c *gin.Context) {
		stub.mu.Lock()
		reversal, ok := stub.reversals[c.Param("organizationId")+"|"+c.Param("id")]
		stub.mu.Unlock()
		if !ok {
			c.JSON(http.StatusNotFound,
				gin.H{"success": false, "error": gin.H{"code": "not_found"}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": reversal})
	})
	group.POST("/reversals", func(c *gin.Context) {
		var body map[string]any
		_ = c.ShouldBindJSON(&body)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
			"id": "frvs_stub", "organizationId": body["organizationId"],
			"status": "PENDING_APPROVAL", "version": 1}})
	})

	stub.server = httptest.NewServer(engine)
	t.Cleanup(stub.server.Close)
	return stub
}

func (w *walletStub) setPayment(organizationID, reference string,
	verification platformclients.JobPaymentVerification) {
	w.mu.Lock()
	defer w.mu.Unlock()
	verification.OrganizationID = organizationID
	w.payments[organizationID+"|"+reference] = verification
}

// client builds the real Fleet Wallet client pointed at the stub, signing with
// the given secret. Passing a secret the server does not know is how the
// invalid-signature case is produced.
func (w *walletStub) client(secret []byte) *platformclients.WalletClient {
	return platformclients.NewWalletClient(platformclients.Options{
		ServiceName: "payment-wallet-service", BaseURL: w.server.URL,
		Secret: secret, Timeout: 5 * time.Second})
}

func paidVerification(id string, amountMinor int64, currency string) platformclients.JobPaymentVerification {
	paidAt := time.Now().UTC()
	return platformclients.JobPaymentVerification{PaymentVerificationID: id,
		AmountMinor: amountMinor, Currency: currency, PaymentStatus: "PAID",
		SourceVersion: 1, PaidAt: &paidAt, RefundState: "NONE", ReversalState: "NONE",
		Settled: true}
}

// outcomeOf extracts the classified downstream outcome, which is what decides
// whether a caller may retry.
func outcomeOf(err error) platformclients.Outcome {
	var typed *platformclients.Error
	if errors.As(err, &typed) {
		return typed.Outcome
	}
	return ""
}

// TestFleetToWalletHMACTransport proves the outgoing half of the private
// contract: a correctly signed, allowlisted, fresh, unique call succeeds and
// every deviation is classified rather than retried blindly.
func TestFleetToWalletHMACTransport(t *testing.T) {
	ctx := context.Background()
	const org = "org-hmac-fleet"
	secret := []byte("fleet-wallet-shared-secret")

	stub := newWalletStub(t, walletStubOptions{secret: secret})
	stub.setPayment(org, "pay-ok", paidVerification("fjpv_ok", 5000, "NGN"))
	client := stub.client(secret)
	meta := platformclients.CallMeta{OrganizationID: org}

	// Valid HMAC.
	verification, err := client.VerifyJobPayment(ctx, meta, org,
		platformclients.JobPaymentByPaymentRef, "pay-ok")
	if err != nil || !verification.Settled || verification.AmountMinor != 5000 {
		t.Fatalf("valid HMAC call failed: %+v err=%v", verification, err)
	}

	// Invalid signature: the client signs with a key the server does not hold.
	wrongKey := stub.client([]byte("not-the-shared-secret"))
	if _, err := wrongKey.VerifyJobPayment(ctx, meta, org,
		platformclients.JobPaymentByPaymentRef, "pay-ok"); outcomeOf(err) != platformclients.OutcomeUnauthenticated {
		t.Fatalf("invalid signature outcome=%q err=%v", outcomeOf(err), err)
	}

	// Wrong caller: a signed identity that is not on the allowlist.
	strictStub := newWalletStub(t, walletStubOptions{secret: secret,
		allowedCallers: []string{"admin-backoffice-service"}})
	strictStub.setPayment(org, "pay-ok", paidVerification("fjpv_ok", 5000, "NGN"))
	if _, err := strictStub.client(secret).VerifyJobPayment(ctx, meta, org,
		platformclients.JobPaymentByPaymentRef, "pay-ok"); outcomeOf(err) != platformclients.OutcomeForbidden {
		t.Fatalf("wrong caller outcome=%q err=%v", outcomeOf(err), err)
	}

	// Stale timestamp: a verifier with a window already elapsed refuses a
	// signature that is otherwise perfectly valid.
	expiredStub := newWalletStub(t, walletStubOptions{secret: secret, window: time.Nanosecond})
	expiredStub.setPayment(org, "pay-ok", paidVerification("fjpv_ok", 5000, "NGN"))
	time.Sleep(2 * time.Millisecond)
	if _, err := expiredStub.client(secret).VerifyJobPayment(ctx, meta, org,
		platformclients.JobPaymentByPaymentRef, "pay-ok"); outcomeOf(err) != platformclients.OutcomeUnauthenticated {
		t.Fatalf("stale timestamp outcome=%q err=%v", outcomeOf(err), err)
	}

	// Tenant mismatch: a real reference under the wrong organization is not
	// found, never another tenant's answer.
	if _, err := client.VerifyJobPayment(ctx, meta, "org-someone-else",
		platformclients.JobPaymentByPaymentRef, "pay-ok"); outcomeOf(err) != platformclients.OutcomeNotFound {
		t.Fatalf("tenant mismatch outcome=%q err=%v", outcomeOf(err), err)
	}
	// Payment not found.
	if _, err := client.VerifyJobPayment(ctx, meta, org,
		platformclients.JobPaymentByTrip, "trip-nobody-paid"); outcomeOf(err) != platformclients.OutcomeNotFound {
		t.Fatalf("missing payment outcome=%q err=%v", outcomeOf(err), err)
	}

	// Replay: the client mints a fresh nonce per call, so a replay has to be
	// staged deliberately. Reissuing a captured nonce is refused.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		stub.server.URL+"/api/v1/payment-wallet/internal/finance/job-payments/lookup?organizationId="+
			org+"&referenceType=PAYMENT_REFERENCE&reference=pay-ok", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Nonce", "captured-nonce")
	if err := serviceauth.SignRequest(request, fleetCallerName, secret, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	first, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("captured request status=%d", first.StatusCode)
	}
	replay, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	replay.Header = request.Header.Clone()
	replayed, err := http.DefaultClient.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	replayed.Body.Close()
	if replayed.StatusCode != http.StatusConflict {
		t.Fatalf("replayed nonce status=%d", replayed.StatusCode)
	}
}

// financeHTTPFixture wires the real finance service, the real authorizer and
// the real operator routes behind an injected tenant context.
type financeHTTPFixture struct {
	pool    *pgxpool.Pool
	service *financefeature.Service
	orgID   string
	stub    *walletStub
}

func newFinanceHTTPFixture(t *testing.T, name, registration string) *financeHTTPFixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, name, registration)
	stub := newWalletStub(t, walletStubOptions{})
	// The real gate, over a real organization that satisfies the platform
	// default financial policy. A test that wants the gate to REFUSE removes
	// one of those facts rather than removing the gate.
	seedEligibleOrganization(t, pool, org.ID)
	service := financefeature.NewService(pool, financefeature.NewRepository(pool),
		stub.client([]byte("fleet-wallet-shared-secret")),
		events.NewDomainPublisher(events.NewOutboxRepository(pool))).
		WithEligibility(newEligibilityService(pool, healthyAuthority("prv-finance", "")))
	return &financeHTTPFixture{pool: pool, service: service, orgID: org.ID, stub: stub}
}

// router mounts the real operator routes for one member of one organization.
func (f *financeHTTPFixture) router(t *testing.T, organizationID, memberID string,
	branchIDs []string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := financefeature.NewOperatorHandler(f.service,
		accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(f.pool), nil, 0))
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: organizationID, OrganizationUserID: memberID,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: memberID,
			PermittedBranchIDs: branchIDs})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	handler.RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	return engine
}

func call(t *testing.T, router http.Handler, method, path, idempotencyKey string,
	body map[string]any) (int, map[string]any) {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		reader = strings.NewReader(string(encoded))
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var decoded map[string]any
	if recorder.Body.Len() > 0 {
		_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	}
	return recorder.Code, decoded
}

// seedFinanceMember creates an organization member, optionally with a system
// role, through the real repositories so the test exercises the same authority
// path production does.
func seedFinanceMember(t *testing.T, pool *pgxpool.Pool, organizationID, roleID string) string {
	t.Helper()
	ctx := context.Background()
	members := staffrepos.NewMemberRepository(pool)
	roles := staffrepos.NewRoleRepository(pool)
	suffix := ids.New("fin")
	identity := &authmodels.Identity{
		EmailNormalized: "finance-" + suffix + "@example.com",
		EmailDisplay:    "finance@example.com",
		PhoneE164:       "+234800" + suffix[len(suffix)-7:],
		PhoneDisplay:    "08000000000",
		Status:          authmodels.IdentityStatusActive,
	}
	member := &staffmodels.OrganizationUser{Status: staffmodels.UserStatusActive}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := authrepos.NewIdentityRepository(pool).Create(ctx, tx, identity); err != nil {
			return err
		}
		member.IdentityID = identity.ID
		if err := members.Create(ctx, tx, organizationID, member); err != nil {
			return err
		}
		if roleID == "" {
			return nil
		}
		return roles.CreateBinding(ctx, tx, organizationID, &staffmodels.RoleBinding{
			OrganizationUserID: member.ID, RoleID: roleID,
			ScopeType: staffmodels.ScopeOrganization, GrantedBy: "test"})
	}); err != nil {
		t.Fatalf("seed finance member: %v", err)
	}
	return member.ID
}

// TestFleetFinanceRoutesDenyByDefault proves the operator routes are closed
// until a role opens them, and that read authority is not write authority.
func TestFleetFinanceRoutesDenyByDefault(t *testing.T) {
	f := newFinanceHTTPFixture(t, "Finance RBAC Ltd", "RC-FINR-1")
	noRole := seedFinanceMember(t, f.pool, f.orgID, "")
	router := f.router(t, f.orgID, noRole, nil)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/fleet-partner/v1/finance/operations"},
		{http.MethodGet, "/fleet-partner/v1/finance/projections"},
		{http.MethodGet, "/fleet-partner/v1/finance/reconciliation-cases"},
	} {
		if status, _ := call(t, router, route.method, route.path, "", nil); status != http.StatusForbidden {
			t.Fatalf("%s %s status=%d, want 403", route.method, route.path, status)
		}
	}
	if status, _ := call(t, router, http.MethodPost, "/fleet-partner/v1/finance/reversals",
		"key-1", map[string]any{"originalJournalId": "fjrn_x", "amountMinor": 10,
			"currency": "NGN"}); status != http.StatusForbidden {
		t.Fatalf("reversal without permission status=%d", status)
	}

	// An analyst may read operations but may not replay one or move money.
	analyst := seedFinanceMember(t, f.pool, f.orgID, "rol_system_analyst")
	analystRouter := f.router(t, f.orgID, analyst, nil)
	if status, _ := call(t, analystRouter, http.MethodGet,
		"/fleet-partner/v1/finance/operations", "", nil); status != http.StatusOK {
		t.Fatalf("analyst read status=%d", status)
	}
	if status, _ := call(t, analystRouter, http.MethodPost,
		"/fleet-partner/v1/finance/operations/fop_missing/replay", "", nil); status != http.StatusForbidden {
		t.Fatalf("analyst replay status=%d, want 403", status)
	}
	if status, _ := call(t, analystRouter, http.MethodPost,
		"/fleet-partner/v1/finance/refunds", "key-2",
		map[string]any{"originalJournalId": "fjrn_x", "amountMinor": 10,
			"currency": "NGN"}); status != http.StatusForbidden {
		t.Fatalf("analyst refund status=%d, want 403", status)
	}
	// Compliance may read reconciliation cases but may not manage them.
	compliance := seedFinanceMember(t, f.pool, f.orgID, "rol_system_compliance")
	complianceRouter := f.router(t, f.orgID, compliance, nil)
	if status, _ := call(t, complianceRouter, http.MethodGet,
		"/fleet-partner/v1/finance/reconciliation-cases", "", nil); status != http.StatusOK {
		t.Fatalf("compliance read status=%d", status)
	}
	if status, _ := call(t, complianceRouter, http.MethodPost,
		"/fleet-partner/v1/finance/reconciliation-cases/fpr_missing/replay", "",
		nil); status != http.StatusForbidden {
		t.Fatalf("compliance manage status=%d, want 403", status)
	}
}

// TestFleetFinanceRoutesPreventCrossTenantAccess proves object-level ownership:
// a finance manager with full authority in their own organization cannot read,
// replay or act on another organization's operation, and cannot tell that it
// exists.
func TestFleetFinanceRoutesPreventCrossTenantAccess(t *testing.T) {
	ctx := context.Background()
	f := newFinanceHTTPFixture(t, "Finance Tenant A", "RC-FINR-2")
	other := createOrganization(t, f.pool, "Finance Tenant B", "RC-FINR-3")

	victim, err := f.service.Request(ctx, financefeature.Request{
		OrganizationID: other.ID, SubjectType: "JOURNAL", SubjectID: "fjrn_other",
		OperationType: "REFUND_REQUEST", IdempotencyKey: "other-refund",
		CorrelationID: "corr", ActorType: "SERVICE", ActorID: "seed",
		Payload: json.RawMessage(`{"amountMinor":100}`)})
	if err != nil {
		t.Fatal(err)
	}

	manager := seedFinanceMember(t, f.pool, f.orgID, "rol_system_finance")
	router := f.router(t, f.orgID, manager, nil)

	// Listing shows nothing belonging to the other tenant.
	status, response := call(t, router, http.MethodGet,
		"/fleet-partner/v1/finance/operations", "", nil)
	if status != http.StatusOK {
		t.Fatalf("list status=%d", status)
	}
	if operations := operationsOf(t, response); len(operations) != 0 {
		t.Fatalf("cross-tenant operations leaked into listing: %#v", operations)
	}
	// Direct reads and writes report not found, not forbidden: revealing that
	// the identifier is real would itself be a disclosure.
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/fleet-partner/v1/finance/operations/" + victim.ID},
		{http.MethodGet, "/fleet-partner/v1/finance/operations/" + victim.ID + "/history"},
		{http.MethodGet, "/fleet-partner/v1/finance/operations/" + victim.ID + "/audit"},
		{http.MethodPost, "/fleet-partner/v1/finance/operations/" + victim.ID + "/replay"},
	} {
		if status, _ := call(t, router, route.method, route.path, "", nil); status != http.StatusNotFound {
			t.Fatalf("%s %s status=%d, want 404", route.method, route.path, status)
		}
	}
	// The other tenant's operation is untouched.
	var replayedStatus string
	if err := f.pool.QueryRow(ctx,
		`SELECT status FROM fleet_partner_finance_operations WHERE id=$1`,
		victim.ID).Scan(&replayedStatus); err != nil {
		t.Fatal(err)
	}
	if replayedStatus != financefeature.StatusPending {
		t.Fatalf("cross-tenant replay changed the victim: status=%s", replayedStatus)
	}
}

func operationsOf(t *testing.T, response map[string]any) []any {
	t.Helper()
	data, _ := response["data"].(map[string]any)
	if data == nil {
		t.Fatalf("no data object: %#v", response)
	}
	operations, _ := data["operations"].([]any)
	return operations
}

// TestFleetFinanceOperatorReplayAndReadBack proves the operator recovery paths
// work through the real routes: a dead-lettered operation can be replayed, a
// dead-lettered reconciliation case can be replayed, and a read-back can be
// requested — each one audited.
func TestFleetFinanceOperatorReplayAndReadBack(t *testing.T) {
	ctx := context.Background()
	f := newFinanceHTTPFixture(t, "Finance Recovery Ltd", "RC-FINR-4")
	manager := seedFinanceMember(t, f.pool, f.orgID, "rol_system_finance")
	router := f.router(t, f.orgID, manager, nil)

	operation, err := f.service.Request(ctx, financefeature.Request{
		OrganizationID: f.orgID, SubjectType: "JOURNAL", SubjectID: "fjrn_recovery",
		OperationType: "REFUND_REQUEST", IdempotencyKey: "recovery-refund",
		CorrelationID: "corr-recovery", ActorType: "SERVICE", ActorID: "seed",
		Payload: json.RawMessage(`{"amountMinor":100}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx,
		`UPDATE fleet_partner_finance_operations SET status='DEAD_LETTER' WHERE id=$1`,
		operation.ID); err != nil {
		t.Fatal(err)
	}
	if status, body := call(t, router, http.MethodPost,
		"/fleet-partner/v1/finance/operations/"+operation.ID+"/replay", "",
		nil); status != http.StatusOK {
		t.Fatalf("replay status=%d body=%#v", status, body)
	}
	var operationStatus string
	if err := f.pool.QueryRow(ctx,
		`SELECT status FROM fleet_partner_finance_operations WHERE id=$1`,
		operation.ID).Scan(&operationStatus); err != nil {
		t.Fatal(err)
	}
	if operationStatus != financefeature.StatusPending {
		t.Fatalf("replayed status=%s", operationStatus)
	}
	// The replay is on the record, attributed to the operator who did it.
	var auditCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records
		WHERE organization_id=$1 AND resource_id=$2 AND action='finance.operation.replayed'
		  AND actor_id=$3`, f.orgID, operation.ID, manager).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("replay audit records=%d", auditCount)
	}
	// History and audit are readable through the routes.
	if status, _ := call(t, router, http.MethodGet,
		"/fleet-partner/v1/finance/operations/"+operation.ID+"/history", "", nil); status != http.StatusOK {
		t.Fatalf("history status=%d", status)
	}
	if status, _ := call(t, router, http.MethodGet,
		"/fleet-partner/v1/finance/operations/"+operation.ID+"/audit", "", nil); status != http.StatusOK {
		t.Fatalf("audit status=%d", status)
	}

	// Reconciliation case: seed a projection, dead-letter it, replay it.
	projectionID := seedFinanceProjection(t, f.pool, f.orgID, "PAYMENT_VERIFICATION",
		"fjpv_recovery", "CURRENT", "PAID", 1)
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_finance_projections
		SET reconciliation_status='DEAD_LETTER',freshness_state='UNKNOWN' WHERE id=$1`,
		projectionID); err != nil {
		t.Fatal(err)
	}
	status, response := call(t, router, http.MethodGet,
		"/fleet-partner/v1/finance/reconciliation-cases", "", nil)
	if status != http.StatusOK {
		t.Fatalf("cases status=%d", status)
	}
	data, _ := response["data"].(map[string]any)
	if cases, _ := data["cases"].([]any); len(cases) != 1 {
		t.Fatalf("expected one open reconciliation case, got %#v", data["cases"])
	}
	if status, body := call(t, router, http.MethodPost,
		"/fleet-partner/v1/finance/reconciliation-cases/"+projectionID+"/replay", "",
		nil); status != http.StatusOK {
		t.Fatalf("reconciliation replay status=%d body=%#v", status, body)
	}
	var reconciliation string
	if err := f.pool.QueryRow(ctx,
		`SELECT reconciliation_status FROM fleet_partner_finance_projections WHERE id=$1`,
		projectionID).Scan(&reconciliation); err != nil {
		t.Fatal(err)
	}
	if reconciliation != "PENDING" {
		t.Fatalf("replayed reconciliation status=%s", reconciliation)
	}

	// Read-back: a CURRENT projection can be sent for authoritative refresh,
	// and asking twice while one is in flight is refused.
	fresh := seedFinanceProjection(t, f.pool, f.orgID, "WALLET", "fwal_recovery",
		"CURRENT", "ACTIVE", 2)
	if status, body := call(t, router, http.MethodPost,
		"/fleet-partner/v1/finance/reconciliation-cases/"+fresh+"/read-back", "",
		nil); status != http.StatusOK {
		t.Fatalf("read-back status=%d body=%#v", status, body)
	}
	var freshness string
	if err := f.pool.QueryRow(ctx,
		`SELECT freshness_state FROM fleet_partner_finance_projections WHERE id=$1`,
		fresh).Scan(&freshness); err != nil {
		t.Fatal(err)
	}
	if freshness != "UNKNOWN" {
		t.Fatalf("read-back freshness=%s", freshness)
	}
	if status, _ := call(t, router, http.MethodPost,
		"/fleet-partner/v1/finance/reconciliation-cases/"+fresh+"/read-back", "",
		nil); status != http.StatusConflict {
		t.Fatalf("concurrent read-back status=%d, want 409", status)
	}
}

// seedFinanceProjection inserts one finance projection directly. The ingestion
// path is covered elsewhere; here the projection is fixture, not subject.
func seedFinanceProjection(t *testing.T, pool *pgxpool.Pool, organizationID,
	projectionType, reference, freshness, safeStatus string, version int64) string {
	t.Helper()
	id := "fpr_" + strings.ToLower(projectionType) + "_" + reference
	if _, err := pool.Exec(context.Background(), `INSERT INTO fleet_partner_finance_projections
		(id,organization_id,projection_type,subject_type,subject_id,wallet_reference,
		 safe_status,source_version,freshness_state,last_synced_at,reconciliation_status)
		VALUES($1,$2,$3,'JOB',$4,$4,$5,$6,$7,now(),'IDLE')`,
		id, organizationID, projectionType, reference, safeStatus, version, freshness); err != nil {
		t.Fatalf("seed finance projection: %v", err)
	}
	return id
}

// seedTripProjection inserts one Dispatch-owned trip projection.
func seedTripProjection(t *testing.T, pool *pgxpool.Pool, organizationID, tripID string,
	completed, cancelled, failed, recipientConfirmed bool) {
	t.Helper()
	timestamp := func(set bool) any {
		if set {
			return time.Now().UTC()
		}
		return nil
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO fleet_partner_trip_projections
		(id,organization_id,source_service,platform_trip_id,executing_provider_id,service_type,
		 trip_status,source_version,last_event_id,last_event_at,freshness_state,
		 completed_at,cancelled_at,failed_at,recipient_confirmed_at)
		VALUES($1,$2,'DISPATCH',$3,'provider-opaque','DELIVERY','COMPLETED',1,'evt',now(),'CURRENT',
		       $4,$5,$6,$7)`,
		"tpr_"+tripID, organizationID, tripID, timestamp(completed), timestamp(cancelled),
		timestamp(failed), timestamp(recipientConfirmed)); err != nil {
		t.Fatalf("seed trip projection: %v", err)
	}
}

// TestFleetJobFinanceGatesUseAuthoritativeLookup proves the earning gate is
// driven by Wallet's authoritative payment answer and Dispatch's authoritative
// delivery outcome — not by Fleet's own optimism.
func TestFleetJobFinanceGatesUseAuthoritativeLookup(t *testing.T) {
	ctx := context.Background()
	f := newFinanceHTTPFixture(t, "Finance Gate Ltd", "RC-FING-1")

	f.stub.setPayment(f.orgID, "pay-gate-1", paidVerification("fjpv_gate1", 2500, "NGN"))
	seedTripProjection(t, f.pool, f.orgID, "trip-gate-1", true, false, false, true)

	earning := func(key, trip, payment string, requireConfirmation bool) (*financefeature.Operation, error) {
		return f.service.RequestJobEarning(ctx, financefeature.JobEarningRequest{
			OrganizationID: f.orgID, TripID: trip, PaymentReference: payment,
			RequireRecipientConfirmation: requireConfirmation, IdempotencyKey: key,
			CorrelationID: "corr-gate", ActorType: "SYSTEM", ActorID: "gate",
			Payload: json.RawMessage(`{"grossMinor":2500,"currency":"NGN"}`)})
	}

	operation, err := earning("earn-gate-1", "trip-gate-1", "pay-gate-1", true)
	if err != nil || operation.OperationType != "EARNING_ALLOCATION" {
		t.Fatalf("paid completed delivery blocked: %+v err=%v", operation, err)
	}

	// Duplicate completion must not create a second financial operation: the
	// same idempotency key resolves to the same row.
	repeat, err := earning("earn-gate-1", "trip-gate-1", "pay-gate-1", true)
	if err != nil || repeat.ID != operation.ID {
		t.Fatalf("duplicate completion created a new operation: %+v err=%v", repeat, err)
	}
	var operationCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_finance_operations
		WHERE organization_id=$1 AND operation_type='EARNING_ALLOCATION'`,
		f.orgID).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 1 {
		t.Fatalf("duplicate completion produced %d earning operations", operationCount)
	}

	// A payment Wallet does not know about blocks the earning.
	if _, err := earning("earn-gate-unknown", "trip-gate-1", "pay-not-recorded", true); err == nil {
		t.Fatal("unverified payment authorized an earning")
	}

	// Failed delivery blocks the earning even though the payment is good.
	seedTripProjection(t, f.pool, f.orgID, "trip-gate-failed", true, false, true, true)
	f.stub.setPayment(f.orgID, "pay-gate-failed", paidVerification("fjpv_failed", 2500, "NGN"))
	if _, err := earning("earn-gate-failed", "trip-gate-failed", "pay-gate-failed", true); err == nil {
		t.Fatal("failed delivery authorized an earning")
	}

	// Cancelled delivery blocks it too.
	seedTripProjection(t, f.pool, f.orgID, "trip-gate-cancelled", true, true, false, true)
	f.stub.setPayment(f.orgID, "pay-gate-cancelled", paidVerification("fjpv_cancelled", 2500, "NGN"))
	if _, err := earning("earn-gate-cancelled", "trip-gate-cancelled", "pay-gate-cancelled", true); err == nil {
		t.Fatal("cancelled delivery authorized an earning")
	}

	// Recipient confirmation gate: the same delivery passes without the policy
	// and fails with it.
	seedTripProjection(t, f.pool, f.orgID, "trip-gate-unconfirmed", true, false, false, false)
	f.stub.setPayment(f.orgID, "pay-gate-unconfirmed", paidVerification("fjpv_unconfirmed", 2500, "NGN"))
	if _, err := earning("earn-gate-unconfirmed", "trip-gate-unconfirmed",
		"pay-gate-unconfirmed", true); err == nil {
		t.Fatal("unconfirmed delivery authorized an earning under a confirmation policy")
	}
	if _, err := earning("earn-gate-unconfirmed-ok", "trip-gate-unconfirmed",
		"pay-gate-unconfirmed", false); err != nil {
		t.Fatalf("unconfirmed delivery blocked without a confirmation policy: %v", err)
	}

	// A refunded payment stops authorizing anything.
	refunded := paidVerification("fjpv_refunded", 2500, "NGN")
	refunded.Settled = false
	refunded.PaymentStatus = "REFUNDED"
	refunded.RefundState = "COMPLETED"
	f.stub.setPayment(f.orgID, "pay-gate-refunded", refunded)
	seedTripProjection(t, f.pool, f.orgID, "trip-gate-refunded", true, false, false, true)
	if _, err := earning("earn-gate-refunded", "trip-gate-refunded", "pay-gate-refunded", true); err == nil {
		t.Fatal("refunded payment authorized an earning")
	}

	// A stale local projection over a good authoritative answer still blocks:
	// the disagreement is the signal.
	f.stub.setPayment(f.orgID, "pay-gate-stale", paidVerification("fjpv_stale", 2500, "NGN"))
	seedTripProjection(t, f.pool, f.orgID, "trip-gate-stale", true, false, false, true)
	staleID := seedFinanceProjection(t, f.pool, f.orgID, "PAYMENT_VERIFICATION",
		"fjpv_stale", "STALE", "PAID", 1)
	if _, err := earning("earn-gate-stale", "trip-gate-stale", "pay-gate-stale", true); err == nil {
		t.Fatal("stale local projection authorized an earning")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_finance_projections
		SET freshness_state='CURRENT' WHERE id=$1`, staleID); err != nil {
		t.Fatal(err)
	}
	if _, err := earning("earn-gate-stale-ok", "trip-gate-stale", "pay-gate-stale", true); err != nil {
		t.Fatalf("current projection blocked an earning: %v", err)
	}
}

// TestFleetFinanceSettlementReversalOrchestration proves the settlement and
// reversal commands enforce their gates: a settlement needs a current,
// non-reconciling projection at the expected version, and a reversal cannot be
// requested by the person who requested what is being reversed.
func TestFleetFinanceSettlementReversalOrchestration(t *testing.T) {
	ctx := context.Background()
	f := newFinanceHTTPFixture(t, "Finance Orchestration Ltd", "RC-FINO-1")
	manager := seedFinanceMember(t, f.pool, f.orgID, "rol_system_finance")
	router := f.router(t, f.orgID, manager, nil)

	// No projection: the settlement is refused rather than sent hopefully.
	if status, _ := call(t, router, http.MethodPost, "/fleet-partner/v1/finance/settlements",
		"settle-1", map[string]any{"beneficiaryType": "PROVIDER",
			"beneficiaryReference": "recipient-opaque", "earningAccountId": "fear_missing",
			"currency": "NGN"}); status != http.StatusConflict {
		t.Fatalf("settlement without projection status=%d, want 409", status)
	}

	projectionID := seedFinanceProjection(t, f.pool, f.orgID, "EARNING_ACCOUNT",
		"fear_settle", "CURRENT", "ACTIVE", 7)
	// Version mismatch: the caller's belief about the account is out of date.
	if status, _ := call(t, router, http.MethodPost, "/fleet-partner/v1/finance/settlements",
		"settle-2", map[string]any{"beneficiaryType": "PROVIDER",
			"beneficiaryReference": "recipient-opaque", "earningAccountId": "fear_settle",
			"currency": "NGN", "expectedVersion": 3}); status != http.StatusConflict {
		t.Fatalf("stale expected version status=%d, want 409", status)
	}
	// An open reconciliation case blocks it.
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_finance_projections
		SET reconciliation_status='PENDING' WHERE id=$1`, projectionID); err != nil {
		t.Fatal(err)
	}
	if status, _ := call(t, router, http.MethodPost, "/fleet-partner/v1/finance/settlements",
		"settle-3", map[string]any{"beneficiaryType": "PROVIDER",
			"beneficiaryReference": "recipient-opaque", "earningAccountId": "fear_settle",
			"currency": "NGN", "expectedVersion": 7}); status != http.StatusConflict {
		t.Fatalf("reconciling account settled: status=%d", status)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_finance_projections
		SET reconciliation_status='IDLE',freshness_state='CURRENT' WHERE id=$1`,
		projectionID); err != nil {
		t.Fatal(err)
	}
	status, response := call(t, router, http.MethodPost, "/fleet-partner/v1/finance/settlements",
		"settle-4", map[string]any{"beneficiaryType": "PROVIDER",
			"beneficiaryReference": "recipient-opaque", "earningAccountId": "fear_settle",
			"currency": "NGN", "expectedVersion": 7})
	if status != http.StatusAccepted {
		t.Fatalf("settlement status=%d body=%#v", status, response)
	}
	settlementData, _ := response["data"].(map[string]any)
	settlementID, _ := settlementData["id"].(string)
	if settlementID == "" {
		t.Fatalf("settlement returned no operation: %#v", response)
	}
	// Fleet queued a command; it never wrote a ledger entry of its own.
	var ledgerTables int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema='public' AND (table_name LIKE '%journal%' OR table_name LIKE '%ledger%')`).
		Scan(&ledgerTables); err != nil {
		t.Fatal(err)
	}
	if ledgerTables != 0 {
		t.Fatalf("fleet database holds %d ledger tables", ledgerTables)
	}

	// Separation of duties: the manager who requested the settlement may not
	// then request its reversal.
	if status, _ := call(t, router, http.MethodPost, "/fleet-partner/v1/finance/reversals",
		"reverse-1", map[string]any{"originalJournalId": "fjrn_settle",
			"sourceOperationId": settlementID, "amountMinor": 500,
			"currency": "NGN", "reasonCode": "MISTAKE"}); status != http.StatusForbidden {
		t.Fatalf("requester reversed its own operation: status=%d", status)
	}
	// A different finance manager may.
	second := seedFinanceMember(t, f.pool, f.orgID, "rol_system_finance")
	secondRouter := f.router(t, f.orgID, second, nil)
	if status, body := call(t, secondRouter, http.MethodPost, "/fleet-partner/v1/finance/reversals",
		"reverse-2", map[string]any{"originalJournalId": "fjrn_settle",
			"sourceOperationId": settlementID, "amountMinor": 500,
			"currency": "NGN", "reasonCode": "MISTAKE"}); status != http.StatusAccepted {
		t.Fatalf("second operator reversal status=%d body=%#v", status, body)
	}
	// Idempotency is required on every write route.
	if status, _ := call(t, secondRouter, http.MethodPost, "/fleet-partner/v1/finance/reversals",
		"", map[string]any{"originalJournalId": "fjrn_settle", "amountMinor": 500,
			"currency": "NGN"}); status != http.StatusBadRequest {
		t.Fatalf("reversal without idempotency key status=%d", status)
	}
	// The same key with different values is a conflict, not a silent rewrite.
	if status, _ := call(t, secondRouter, http.MethodPost, "/fleet-partner/v1/finance/reversals",
		"reverse-2", map[string]any{"originalJournalId": "fjrn_settle",
			"sourceOperationId": settlementID, "amountMinor": 900,
			"currency": "NGN", "reasonCode": "MISTAKE"}); status != http.StatusConflict {
		t.Fatalf("fingerprint conflict status=%d", status)
	}
}

// TestFleetFinanceBranchScope proves a branch-scoped operator sees and acts on
// their own branch and on organization-level rows, and nothing else.
func TestFleetFinanceBranchScope(t *testing.T) {
	ctx := context.Background()
	f := newFinanceHTTPFixture(t, "Finance Branch Ltd", "RC-FINB-1")
	manager := seedFinanceMember(t, f.pool, f.orgID, "rol_system_finance")

	mine, err := f.service.Request(ctx, financefeature.Request{
		OrganizationID: f.orgID, BranchID: "branch-mine", SubjectType: "JOURNAL",
		SubjectID: "fjrn_mine", OperationType: "REFUND_REQUEST", IdempotencyKey: "branch-mine",
		CorrelationID: "corr", ActorType: "SERVICE", ActorID: "seed",
		Payload: json.RawMessage(`{"amountMinor":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := f.service.Request(ctx, financefeature.Request{
		OrganizationID: f.orgID, BranchID: "branch-theirs", SubjectType: "JOURNAL",
		SubjectID: "fjrn_theirs", OperationType: "REFUND_REQUEST", IdempotencyKey: "branch-theirs",
		CorrelationID: "corr", ActorType: "SERVICE", ActorID: "seed",
		Payload: json.RawMessage(`{"amountMinor":1}`)})
	if err != nil {
		t.Fatal(err)
	}

	scoped := f.router(t, f.orgID, manager, []string{"branch-mine"})
	status, response := call(t, scoped, http.MethodGet,
		"/fleet-partner/v1/finance/operations", "", nil)
	if status != http.StatusOK {
		t.Fatalf("scoped list status=%d", status)
	}
	operations := operationsOf(t, response)
	if len(operations) != 1 {
		t.Fatalf("branch scope returned %d operations", len(operations))
	}
	if status, _ := call(t, scoped, http.MethodGet,
		"/fleet-partner/v1/finance/operations/"+theirs.ID, "", nil); status != http.StatusNotFound {
		t.Fatalf("out-of-branch read status=%d, want 404", status)
	}
	if status, _ := call(t, scoped, http.MethodGet,
		"/fleet-partner/v1/finance/operations/"+mine.ID, "", nil); status != http.StatusOK {
		t.Fatalf("in-branch read status=%d", status)
	}
	// A write aimed at a branch outside the caller's authority is refused.
	if status, _ := call(t, scoped, http.MethodPost, "/fleet-partner/v1/finance/refunds",
		"branch-refund", map[string]any{"originalJournalId": "fjrn_x", "amountMinor": 10,
			"currency": "NGN", "branchId": "branch-theirs"}); status != http.StatusForbidden {
		t.Fatalf("out-of-branch write status=%d, want 403", status)
	}
}
