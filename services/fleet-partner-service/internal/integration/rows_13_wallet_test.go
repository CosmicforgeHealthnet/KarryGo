package integration_test

// Requirement-row tests for spec Section 13 (company wallet and provider
// sub-wallets) and its detail Section 42.
//
// Every test drives the real Section 13 HTTP handler and the service exactly as
// production assembles it (app.BuildCompanyWalletService), against PostgreSQL.
// payment-wallet-service is replaced by an httptest server that runs the same
// shared/go/serviceauth HMAC verifier Wallet runs, so the signed client, its
// paths and its error classification are the production code. Fleet never
// holds a balance: nothing here seeds or asserts one.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	wallethttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/http"
	walletmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	walletusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/usecases"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

const (
	rows13WalletSecret = "rows13-payment-wallet-shared-secret"
	rows13FinanceRole  = "rol_system_finance"
)

// rows13Wallet is a payment-wallet-service stand-in for the finance routes
// Fleet's Section 13 client calls.
type rows13Wallet struct {
	server *httptest.Server

	mu sync.Mutex
	// accounts maps an account id to its owning organization and state.
	accounts map[string]rows13Account
	// failSubmissions answers every money-moving POST with 503.
	failSubmissions bool
	// calls counts requests per route template.
	calls map[string]int
	// bodies keeps the last JSON body per route template.
	bodies map[string]map[string]any
}

type rows13Account struct {
	organizationID string
	status         string
	version        int64
}

func newRows13Wallet(t *testing.T) *rows13Wallet {
	t.Helper()
	w := &rows13Wallet{accounts: map[string]rows13Account{}, calls: map[string]int{},
		bodies: map[string]map[string]any{}}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	group := engine.Group("/api/v1/payment-wallet/internal/finance")
	group.Use(serviceauth.Middleware(serviceauth.NewVerifier(
		serviceauth.Secrets{fleetCallerName: []byte(rows13WalletSecret)}, 5*time.Minute)))
	record := func(c *gin.Context, route string) {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.calls[route]++
		if c.Request.Method == http.MethodPost {
			var body map[string]any
			_ = c.ShouldBindJSON(&body)
			w.bodies[route] = body
			if key, ok := body["idempotencyKey"].(string); ok {
				w.calls[route+"|"+key]++
			}
		}
	}
	readAccount := func(route string) gin.HandlerFunc {
		return func(c *gin.Context) {
			record(c, route)
			w.mu.Lock()
			account, ok := w.accounts[c.Param("id")]
			w.mu.Unlock()
			// An account of another organization and an unknown account are
			// the same answer, exactly as Wallet answers them.
			if !ok || account.organizationID != c.Query("organizationId") {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "not_found"}})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
				"id": c.Param("id"), "status": account.status, "version": account.version}})
		}
	}
	group.GET("/earning-accounts/:id", readAccount("GET /earning-accounts/:id"))
	group.GET("/wallets/:id", readAccount("GET /wallets/:id"))
	submit := func(route, prefix string) gin.HandlerFunc {
		return func(c *gin.Context) {
			record(c, route)
			w.mu.Lock()
			fail := w.failSubmissions
			w.mu.Unlock()
			if fail {
				// Retry-After: 0 keeps the client's bounded retries instant.
				c.Header("Retry-After", "0")
				c.JSON(http.StatusServiceUnavailable, gin.H{"success": false,
					"error": gin.H{"code": "temporarily_unavailable"}})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
				"id": prefix + "_rows13", "status": "POSTED", "version": 1}})
		}
	}
	// Provider sub-wallet provisioning (EnsureProviderSubWallet). Idempotent
	// per provider reference; the created account becomes readable.
	group.POST("/earning-accounts/ensure", func(c *gin.Context) {
		record(c, "POST /earning-accounts/ensure")
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.failSubmissions {
			c.Header("Retry-After", "0")
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false,
				"error": gin.H{"code": "temporarily_unavailable"}})
			return
		}
		body := w.bodies["POST /earning-accounts/ensure"]
		organizationID, _ := body["organizationId"].(string)
		reference, _ := body["providerReference"].(string)
		id := "fear_" + reference
		w.accounts[id] = rows13Account{organizationID: organizationID, status: "ACTIVE", version: 1}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": id, "status": "ACTIVE", "version": 1}})
	})
	group.POST("/provider-allocations", submit("POST /provider-allocations", "fpal"))
	group.POST("/provider-withdrawals", submit("POST /provider-withdrawals", "fpwd"))
	w.server = httptest.NewServer(engine)
	t.Cleanup(w.server.Close)
	return w
}

func (w *rows13Wallet) setAccount(id, organizationID, status string, version int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.accounts[id] = rows13Account{organizationID: organizationID, status: status, version: version}
}

func (w *rows13Wallet) removeAccount(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.accounts, id)
}

func (w *rows13Wallet) setFailSubmissions(fail bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failSubmissions = fail
}

func (w *rows13Wallet) callCount(route string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls[route]
}

func (w *rows13Wallet) lastBody(route string) map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.bodies[route]
}

// rows13Flags names which Section 13 switches are on for one service.
type rows13Flags struct{ read, subWallet, allocation, withdrawal, refresh bool }

func rows13AllOn() rows13Flags { return rows13Flags{true, true, true, true, true} }

// rows13Service assembles the Section 13 service exactly as the composition
// root does, pointed at the Wallet stand-in. Each call builds a fresh client
// (and circuit breaker), which is what a process restart looks like.
func rows13Service(pool *pgxpool.Pool, wallet *rows13Wallet, flags rows13Flags) *walletusecases.Service {
	cfg := config.Config{
		Clients: map[string]config.ClientConfig{"payment-wallet-service": {
			BaseURL: wallet.server.URL, Secret: []byte(rows13WalletSecret)}},
		HTTPClientTimeout:              5 * time.Second,
		CompanyWalletStaleAfter:        15 * time.Minute,
		OrganizationWalletReadEnabled:  flags.read,
		ProviderSubWalletEnabled:       flags.subWallet,
		ProviderAllocationEnabled:      flags.allocation,
		ProviderWithdrawalEnabled:      flags.withdrawal,
		WalletProjectionRefreshEnabled: flags.refresh,
		WalletReconciliationEnabled:    true,
	}
	return app.BuildCompanyWalletService(cfg, pool, observability.NewMetrics(),
		events.NewDomainPublisher(events.NewOutboxRepository(pool)), nil, nil, nil)
}

// rows13Router mounts the real Section 13 tenant routes behind the real
// database-backed authorizer. The tenant context (organization and member) is
// taken from test headers in place of the session middleware; permission and
// branch scope are decided by the production authorizer from real bindings.
func rows13Router(pool *pgxpool.Pool, service *walletusecases.Service) http.Handler {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
			RequestID: "req-rows13", CorrelationID: "corr-rows13"})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	wallethttp.NewHandler(service, authorizer).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	return engine
}

func as(orgID, memberID string, extra ...string) map[string]string {
	headers := map[string]string{"X-Test-Org": orgID, "X-Test-Member": memberID}
	for i := 0; i+1 < len(extra); i += 2 {
		headers[extra[i]] = extra[i+1]
	}
	return headers
}

// branchMember seeds a member whose only grant is roleID at one branch.
func branchMember(t *testing.T, pool *pgxpool.Pool, orgID, roleID, branchID, label string) string {
	t.Helper()
	member := seedMember(t, pool, orgID, seedIdentity(t, pool, label+"-"+randomHex()[:6]+"@example.com",
		"+23481"+randomHex()[:8]), false, nil)
	bindRole(t, pool, orgID, member.ID, roleID, staffmodels.ScopeBranch, branchID)
	return member.ID
}

func walletKeyAbsent(t *testing.T, data map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, present := data[key]; present {
			t.Fatalf("Fleet must not report %q (payment-wallet-service owns balances): %v", key, data)
		}
	}
}

// FLEET-13.1 / 42.3 GET /wallet: the organization wallet projection is
// read from payment-wallet-service; Fleet stores an opaque id and freshness,
// never a balance, and downgrades its copy when the authority cannot answer.
func TestRow13_1_OrganizationWalletProjection(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	wallet := newRows13Wallet(t)
	router := rows13Router(f.pool, rows13Service(f.pool, wallet, rows13AllOn()))
	finance := seedApprovalMember(t, f.pool, f.organization, rows13FinanceRole, "r13-finance")
	analyst := seedApprovalMember(t, f.pool, f.organization, "rol_system_analyst", "r13-analyst")
	operations := seedApprovalMember(t, f.pool, f.organization, "rol_system_operations", "r13-ops")

	// Local projection: id, status and freshness, and no balance at all.
	code, body := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet", nil, as(f.organization, finance))
	if code != http.StatusOK {
		t.Fatalf("GET /wallet = %d %v", code, body)
	}
	view := rowsData(t, body)
	if view["walletId"] != "fwal_org_1" || view["freshnessState"] != walletmodels.FreshnessCurrent ||
		view["sourceVersion"].(float64) != 3 {
		t.Fatalf("local projection = %v", view)
	}
	walletKeyAbsent(t, view, "availableBalanceMinor", "pendingBalanceMinor", "reservedBalanceMinor")
	if wallet.callCount("GET /earning-accounts/:id") != 0 {
		t.Fatal("the default read must use the local projection, not call Wallet")
	}

	// Authoritative read: Wallet answers, the projection follows it.
	wallet.setAccount("fwal_org_1", f.organization, "ACTIVE", 9)
	code, body = rowsCall(t, router, "GET", "/fleet-partner/v1/wallet?authoritative=true", nil, as(f.organization, finance))
	if code != http.StatusOK {
		t.Fatalf("authoritative GET /wallet = %d %v", code, body)
	}
	if view = rowsData(t, body); view["sourceVersion"].(float64) != 9 || view["freshnessState"] != walletmodels.FreshnessCurrent {
		t.Fatalf("authoritative projection = %v", view)
	}
	var stored int64
	if err := f.pool.QueryRow(context.Background(), `SELECT source_version FROM fleet_partner_organization_wallet_provisioning WHERE organization_id = $1`, f.organization).Scan(&stored); err != nil || stored != 9 {
		t.Fatalf("stored source_version = %d (%v), want 9", stored, err)
	}

	// Wallet cannot confirm the account: the answer is honest (UNKNOWN), no
	// figure is invented, and the stored copy stops being CURRENT so the
	// money-moving gates refuse until it is repaired.
	wallet.removeAccount("fwal_org_1")
	code, body = rowsCall(t, router, "GET", "/fleet-partner/v1/wallet?authoritative=true", nil, as(f.organization, finance))
	if code != http.StatusOK || rowsData(t, body)["freshnessState"] != walletmodels.FreshnessUnknown {
		t.Fatalf("unconfirmed authoritative read = %d %v", code, body)
	}
	if freshness := rows13Freshness(t, f.pool, f.organization); freshness == walletmodels.FreshnessCurrent {
		t.Fatal("a failed authoritative read left the projection CURRENT")
	}

	// wallet.read is enough; a role without it is refused.
	if code, _ := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet", nil, as(f.organization, analyst)); code != http.StatusOK {
		t.Fatalf("analyst (wallet.read) = %d", code)
	}
	if code, _ := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet", nil, as(f.organization, operations)); code != http.StatusForbidden {
		t.Fatalf("operations (no wallet.read) = %d, want 403", code)
	}

	// Another organization sees no wallet: not this one, and not an empty one.
	other := createOrganization(t, f.pool, "Rows13 Other Wallet Co", "RC-R13-OTHER-1")
	otherFinance := seedApprovalMember(t, f.pool, other.ID, rows13FinanceRole, "r13-other")
	if code, _ := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet", nil, as(other.ID, otherFinance)); code != http.StatusNotFound {
		t.Fatalf("other organization GET /wallet = %d, want 404", code)
	}

	// The read surface is dark until ORGANIZATION_WALLET_READ_ENABLED is set.
	dark := rows13Router(f.pool, rows13Service(f.pool, wallet, rows13Flags{}))
	if code, _ := rowsCall(t, dark, "GET", "/fleet-partner/v1/wallet", nil, as(f.organization, finance)); code != http.StatusNotFound {
		t.Fatalf("GET /wallet with the read flag off = %d, want 404", code)
	}
}

func rows13Freshness(t *testing.T, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	var freshness string
	if err := pool.QueryRow(context.Background(), `SELECT freshness_state FROM fleet_partner_organization_wallet_provisioning WHERE organization_id = $1`, orgID).Scan(&freshness); err != nil {
		t.Fatal(err)
	}
	return freshness
}

// FLEET-13.2 / 42.3 GET /wallet/transactions[/:id]: organization-scoped
// transaction projection with cursor pagination, allowlisted filters, branch
// scope and a dedicated permission.
func TestRow13_2_WalletTransactionsListAndDetail(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	wallet := newRows13Wallet(t)
	router := rows13Router(f.pool, rows13Service(f.pool, wallet, rows13AllOn()))
	finance := seedApprovalMember(t, f.pool, f.organization, rows13FinanceRole, "r13-tx-finance")
	analyst := seedApprovalMember(t, f.pool, f.organization, "rol_system_analyst", "r13-tx-analyst")
	branchA := seedBranch(t, f.pool, f.organization, "R13 Tx A")
	branchB := seedBranch(t, f.pool, f.organization, "R13 Tx B")
	branchOnly := branchMember(t, f.pool, f.organization, rows13FinanceRole, branchA, "r13-tx-branch")

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	seed := []walletmodels.TransactionProjection{
		{OrganizationID: f.organization, WalletReference: "fwal_org_1", TransactionReference: "tx-1", SubjectType: "ORGANIZATION", Direction: "CREDIT", Category: "SETTLEMENT", AmountMinor: 150_000, Currency: "NGN", SafeStatus: "POSTED", OccurredAt: base, SourceVersion: 1, FreshnessState: "CURRENT"},
		{OrganizationID: f.organization, BranchID: branchA, WalletReference: "fwal_org_1", TransactionReference: "tx-2", SubjectType: "PROVIDER", SubjectID: f.membership, Direction: "DEBIT", Category: "PROVIDER_ALLOCATION", AmountMinor: 40_000, Currency: "NGN", SafeStatus: "POSTED", OccurredAt: base.Add(time.Minute), SourceVersion: 1, FreshnessState: "CURRENT"},
		{OrganizationID: f.organization, BranchID: branchB, WalletReference: "fwal_org_1", TransactionReference: "tx-3", SubjectType: "ORGANIZATION", Direction: "CREDIT", Category: "SETTLEMENT", AmountMinor: 90_000, Currency: "NGN", SafeStatus: "POSTED", OccurredAt: base.Add(2 * time.Minute), SourceVersion: 1, FreshnessState: "CURRENT"},
	}
	for _, row := range seed {
		if err := f.repo.UpsertTransaction(ctx, nil, row); err != nil {
			t.Fatal(err)
		}
	}
	other := createOrganization(t, f.pool, "Rows13 Other Tx Co", "RC-R13-OTHER-2")
	if err := f.repo.UpsertTransaction(ctx, nil, walletmodels.TransactionProjection{OrganizationID: other.ID, WalletReference: "fwal_other", TransactionReference: "tx-other", SubjectType: "ORGANIZATION", Direction: "CREDIT", Category: "SETTLEMENT", AmountMinor: 1, Currency: "NGN", SafeStatus: "POSTED", OccurredAt: base, SourceVersion: 1, FreshnessState: "CURRENT"}); err != nil {
		t.Fatal(err)
	}
	var otherTxID string
	if err := f.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_wallet_transaction_projections WHERE organization_id = $1`, other.ID).Scan(&otherTxID); err != nil {
		t.Fatal(err)
	}

	list := func(member, query string) ([]any, string) {
		t.Helper()
		code, body := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet/transactions"+query, nil, as(f.organization, member))
		if code != http.StatusOK {
			t.Fatalf("list %s = %d %v", query, code, body)
		}
		meta, _ := body["meta"].(map[string]any)
		next, _ := meta["nextCursor"].(string)
		return rowsList(t, body), next
	}
	all, _ := list(finance, "")
	if len(all) != 3 {
		t.Fatalf("organization transactions = %d, want 3 (another organization's must not appear)", len(all))
	}
	newest := all[0].(map[string]any)
	if newest["transactionReference"] != "tx-3" || newest["amountMinor"].(float64) != 90_000 || newest["currency"] != "NGN" {
		t.Fatalf("newest first, minor units + currency: %v", newest)
	}
	if occurred, _ := newest["occurredAt"].(string); !strings.HasSuffix(occurred, "Z") {
		t.Fatalf("occurredAt must be UTC: %q", occurred)
	}

	// Cursor pagination walks every row exactly once.
	page1, next := list(finance, "?limit=2")
	if len(page1) != 2 || next == "" {
		t.Fatalf("page 1 = %d rows, next=%q", len(page1), next)
	}
	page2, _ := list(finance, "?limit=2&cursor="+next)
	if len(page2) != 1 || page2[0].(map[string]any)["transactionReference"] != "tx-1" {
		t.Fatalf("page 2 = %v", page2)
	}
	if code, _ := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet/transactions?cursor=not-a-cursor", nil, as(f.organization, finance)); code != http.StatusBadRequest {
		t.Fatalf("malformed cursor = %d, want 400", code)
	}

	// Allowlisted filters.
	debits, _ := list(finance, "?direction=debit")
	if len(debits) != 1 || debits[0].(map[string]any)["transactionReference"] != "tx-2" {
		t.Fatalf("direction filter = %v", debits)
	}

	// A branch-scoped member sees organization-level rows and its branch only.
	scoped, _ := list(branchOnly, "")
	for _, item := range scoped {
		if item.(map[string]any)["transactionReference"] == "tx-3" {
			t.Fatal("a branch-A member listed a branch-B transaction")
		}
	}
	if len(scoped) != 2 {
		t.Fatalf("branch-scoped list = %d rows, want 2", len(scoped))
	}

	// Detail: own row 200; another organization's or another branch's 404.
	id := newest["id"].(string)
	if code, body := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet/transactions/"+id, nil, as(f.organization, finance)); code != http.StatusOK || rowsData(t, body)["transactionReference"] != "tx-3" {
		t.Fatalf("detail = %d %v", code, body)
	}
	if code, _ := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet/transactions/"+otherTxID, nil, as(f.organization, finance)); code != http.StatusNotFound {
		t.Fatalf("cross-tenant detail = %d, want 404", code)
	}
	if code, _ := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet/transactions/"+id, nil, as(f.organization, branchOnly)); code != http.StatusNotFound {
		t.Fatalf("cross-branch detail = %d, want 404", code)
	}

	// wallet.read alone is not wallet.transactions.read.
	if code, _ := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet/transactions", nil, as(f.organization, analyst)); code != http.StatusForbidden {
		t.Fatalf("analyst list = %d, want 403", code)
	}

	// Like GET /wallet, the transaction projection is dark until the
	// organization wallet read surface is switched on.
	dark := rows13Router(f.pool, rows13Service(f.pool, wallet, rows13Flags{}))
	for _, path := range []string{"/fleet-partner/v1/wallet/transactions", "/fleet-partner/v1/wallet/transactions/" + id} {
		if code, body := rowsCall(t, dark, "GET", path, nil, as(f.organization, finance)); code != http.StatusNotFound {
			t.Fatalf("%s with the read flag off = %d %v, want 404", path, code, body)
		}
	}
}

// FLEET-13.3 / 42.3 GET /providers/{membershipId}/sub-wallet: the corporate
// view of one provider's sub-wallet projection, with no balance.
func TestRow13_3_ProviderSubWalletProjection(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	wallet := newRows13Wallet(t)
	router := rows13Router(f.pool, rows13Service(f.pool, wallet, rows13AllOn()))
	finance := seedApprovalMember(t, f.pool, f.organization, rows13FinanceRole, "r13-sw-finance")
	operations := seedApprovalMember(t, f.pool, f.organization, "rol_system_operations", "r13-sw-ops")
	analyst := seedApprovalMember(t, f.pool, f.organization, "rol_system_analyst", "r13-sw-analyst")
	path := "/fleet-partner/v1/providers/" + f.membership + "/sub-wallet"

	for _, member := range []string{finance, operations} {
		code, body := rowsCall(t, router, "GET", path, nil, as(f.organization, member))
		if code != http.StatusOK {
			t.Fatalf("GET sub-wallet = %d %v", code, body)
		}
		view := rowsData(t, body)
		if view["providerSubWalletId"] != "fear_sub_1" || view["membershipId"] != f.membership ||
			view["status"] != walletmodels.LinkActive || view["freshnessState"] != walletmodels.FreshnessCurrent {
			t.Fatalf("sub-wallet view = %v", view)
		}
		walletKeyAbsent(t, view, "availableBalanceMinor", "balanceMinor", "pendingBalanceMinor")
	}
	if code, _ := rowsCall(t, router, "GET", path, nil, as(f.organization, analyst)); code != http.StatusForbidden {
		t.Fatalf("analyst = %d, want 403", code)
	}
	if code, _ := rowsCall(t, router, "GET", "/fleet-partner/v1/providers/opm_unknown/sub-wallet", nil, as(f.organization, finance)); code != http.StatusNotFound {
		t.Fatalf("unknown membership = %d, want 404", code)
	}
	other := createOrganization(t, f.pool, "Rows13 Other SW Co", "RC-R13-OTHER-3")
	otherFinance := seedApprovalMember(t, f.pool, other.ID, rows13FinanceRole, "r13-sw-other")
	if code, _ := rowsCall(t, router, "GET", path, nil, as(other.ID, otherFinance)); code != http.StatusNotFound {
		t.Fatalf("cross-tenant sub-wallet = %d, want 404", code)
	}
	dark := rows13Router(f.pool, rows13Service(f.pool, wallet, rows13Flags{read: true}))
	if code, _ := rowsCall(t, dark, "GET", path, nil, as(f.organization, finance)); code != http.StatusNotFound {
		t.Fatalf("sub-wallet with PROVIDER_SUBWALLET_ENABLED off = %d, want 404", code)
	}
}

// rows13Allocation requests and (when approve) approves one salary
// allocation through the real use case.
func rows13Allocation(t *testing.T, f *walletFixture, key, reference, branchID string, approve bool) walletmodels.AllocationInstruction {
	t.Helper()
	ctx := context.Background()
	input := salaryAllocation(f, key)
	input.BusinessReferenceID = reference
	input.BranchID = branchID
	input.PrivateNote = "internal note " + key
	instruction, err := f.service.RequestAllocation(ctx, input)
	if err != nil {
		t.Fatalf("request allocation %s: %v", key, err)
	}
	if approve && instruction.Status == walletmodels.AllocationPendingApproval {
		approver := f.tenant
		approver.ActorID, approver.OrganizationUserID = "usr_rows13_approver", "usr_rows13_approver"
		if instruction, err = f.service.DecideAllocation(ctx, approver, instruction.ID, instruction.Version, true, "approved"); err != nil {
			t.Fatalf("approve allocation %s: %v", key, err)
		}
	}
	return instruction
}

func rows13AllocationStatus(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM fleet_partner_provider_allocation_instructions WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

// FLEET-13.4b / 42.3 GET .../allocations: tenant- and membership-scoped
// listing with cursor pagination and allowlisted filters.
func TestRow13_4b_AllocationList(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	wallet := newRows13Wallet(t)
	router := rows13Router(f.pool, rows13Service(f.pool, wallet, rows13AllOn()))
	finance := seedApprovalMember(t, f.pool, f.organization, rows13FinanceRole, "r13-al-finance")
	analyst := seedApprovalMember(t, f.pool, f.organization, "rol_system_analyst", "r13-al-analyst")
	first := rows13Allocation(t, f, "r13-al-1", "payroll-2026-01", "", false)
	second := rows13Allocation(t, f, "r13-al-2", "payroll-2026-02", "", true)
	path := "/fleet-partner/v1/providers/" + f.membership + "/sub-wallet/allocations"

	code, body := rowsCall(t, router, "GET", path, nil, as(f.organization, finance))
	if code != http.StatusOK {
		t.Fatalf("list allocations = %d %v", code, body)
	}
	items := rowsList(t, body)
	if len(items) != 2 || items[0].(map[string]any)["id"] != second.ID {
		t.Fatalf("allocations (newest first) = %v", items)
	}
	top := items[0].(map[string]any)
	if top["amountMinor"].(float64) != 700_000 || top["currency"] != "NGN" || top["privateNote"] != "internal note r13-al-2" {
		t.Fatalf("allocation view = %v", top)
	}

	_, body = rowsCall(t, router, "GET", path+"?limit=1", nil, as(f.organization, finance))
	meta, _ := body["meta"].(map[string]any)
	next, _ := meta["nextCursor"].(string)
	if len(rowsList(t, body)) != 1 || next == "" {
		t.Fatalf("first page = %v", body)
	}
	_, body = rowsCall(t, router, "GET", path+"?limit=1&cursor="+next, nil, as(f.organization, finance))
	if page := rowsList(t, body); len(page) != 1 || page[0].(map[string]any)["id"] != first.ID {
		t.Fatalf("second page = %v", page)
	}

	_, body = rowsCall(t, router, "GET", path+"?status="+first.Status, nil, as(f.organization, finance))
	for _, item := range rowsList(t, body) {
		if item.(map[string]any)["status"] != first.Status {
			t.Fatalf("status filter leaked %v", item)
		}
	}

	if code, _ := rowsCall(t, router, "GET", path, nil, as(f.organization, analyst)); code != http.StatusForbidden {
		t.Fatalf("analyst list = %d, want 403", code)
	}
	// Another organization naming this membership learns nothing.
	other := createOrganization(t, f.pool, "Rows13 Other Al Co", "RC-R13-OTHER-4")
	otherFinance := seedApprovalMember(t, f.pool, other.ID, rows13FinanceRole, "r13-al-other")
	code, body = rowsCall(t, router, "GET", path, nil, as(other.ID, otherFinance))
	if code == http.StatusOK && len(rowsList(t, body)) != 0 {
		t.Fatalf("cross-tenant allocation list leaked rows: %v", body)
	}
}

// FLEET-13.4d POST .../allocations/:id/replay: only a dead-lettered
// instruction returns to the execution queue, under the same idempotency key,
// and the next pass submits it to Wallet.
func TestRow13_4d_AllocationReplay(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	wallet := newRows13Wallet(t)
	finance := seedApprovalMember(t, f.pool, f.organization, rows13FinanceRole, "r13-rp-finance")
	operations := seedApprovalMember(t, f.pool, f.organization, "rol_system_operations", "r13-rp-ops")
	branchA := seedBranch(t, f.pool, f.organization, "R13 Replay A")
	branchB := seedBranch(t, f.pool, f.organization, "R13 Replay B")
	branchAFinance := branchMember(t, f.pool, f.organization, rows13FinanceRole, branchA, "r13-rp-branch")

	instruction := rows13Allocation(t, f, "r13-rp-1", "payroll-2026-03", branchB, true)
	if instruction.Status != walletmodels.AllocationApproved {
		t.Fatalf("seeded allocation status = %s, want APPROVED", instruction.Status)
	}

	// Wallet is down and the attempt budget is spent: the worker dead-letters.
	wallet.setFailSubmissions(true)
	first := rows13Service(f.pool, wallet, rows13AllOn())
	if err := first.RunPass(ctx, walletusecases.WorkerConfig{Owner: "rows13", MaxAttempts: 1}); err != nil {
		t.Fatal(err)
	}
	if status := rows13AllocationStatus(t, f.pool, instruction.ID); status != walletmodels.AllocationDeadLetter {
		t.Fatalf("after exhausted retries = %s, want DEAD_LETTER", status)
	}
	if wallet.callCount("POST /provider-allocations") == 0 {
		t.Fatal("the worker never reached Wallet")
	}

	// Created after the failed pass, so it is APPROVED but not dead-lettered.
	live := rows13Allocation(t, f, "r13-rp-2", "payroll-2026-04", "", true)
	router := rows13Router(f.pool, first)
	replay := func(member, membership, id string) (int, map[string]any) {
		return rowsCall(t, router, "POST", "/fleet-partner/v1/providers/"+membership+"/sub-wallet/allocations/"+id+"/replay", map[string]any{}, as(f.organization, member))
	}
	// Refusals: no approve permission; not dead-lettered; another branch;
	// another membership in the path; another organization.
	if code, _ := replay(operations, f.membership, instruction.ID); code != http.StatusForbidden {
		t.Fatalf("replay without approve permission = %d, want 403", code)
	}
	if code, _ := replay(finance, f.membership, live.ID); code != http.StatusNotFound {
		t.Fatalf("replay of a non-dead-lettered allocation = %d, want 404", code)
	}
	if code, body := replay(branchAFinance, f.membership, instruction.ID); code != http.StatusNotFound {
		t.Fatalf("branch-A approver replaying a branch-B allocation = %d %v, want 404", code, body)
	}
	if code, body := replay(finance, "opm_someone_else", instruction.ID); code != http.StatusNotFound {
		t.Fatalf("replay through another membership's path = %d %v, want 404", code, body)
	}
	other := createOrganization(t, f.pool, "Rows13 Other Rp Co", "RC-R13-OTHER-5")
	otherFinance := seedApprovalMember(t, f.pool, other.ID, rows13FinanceRole, "r13-rp-other")
	if code, _ := rowsCall(t, router, "POST", "/fleet-partner/v1/providers/"+f.membership+"/sub-wallet/allocations/"+instruction.ID+"/replay", map[string]any{}, as(other.ID, otherFinance)); code != http.StatusNotFound {
		t.Fatalf("cross-tenant replay = %d, want 404", code)
	}
	if status := rows13AllocationStatus(t, f.pool, instruction.ID); status != walletmodels.AllocationDeadLetter {
		t.Fatalf("a refused replay changed the status to %s", status)
	}

	code, body := replay(finance, f.membership, instruction.ID)
	if code != http.StatusOK || rowsData(t, body)["status"] != walletmodels.AllocationApproved {
		t.Fatalf("replay = %d %v", code, body)
	}
	if status := rows13AllocationStatus(t, f.pool, instruction.ID); status != walletmodels.AllocationApproved {
		t.Fatalf("after replay = %s, want APPROVED", status)
	}
	if n := rowsCount(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id = $1 AND action = 'wallet.provider_allocation.replayed' AND resource_id = $2`, f.organization, instruction.ID); n != 1 {
		t.Fatalf("replay audit records = %d, want 1", n)
	}
	// A second replay finds nothing dead-lettered.
	if code, _ := replay(finance, f.membership, instruction.ID); code != http.StatusNotFound {
		t.Fatalf("second replay = %d, want 404", code)
	}

	// Wallet recovers; the next pass (a fresh process) submits it once, under
	// the instruction's original idempotency key.
	wallet.setFailSubmissions(false)
	before := wallet.callCount("POST /provider-allocations|r13-rp-1")
	if err := rows13Service(f.pool, wallet, rows13AllOn()).RunPass(ctx, walletusecases.WorkerConfig{Owner: "rows13", MaxAttempts: 8}); err != nil {
		t.Fatal(err)
	}
	if status := rows13AllocationStatus(t, f.pool, instruction.ID); status != walletmodels.AllocationCompleted {
		t.Fatalf("after replayed execution = %s, want COMPLETED", status)
	}
	if got := wallet.callCount("POST /provider-allocations|r13-rp-1"); got != before+1 {
		t.Fatalf("Wallet submissions of the replayed instruction = %d more, want exactly one", got-before)
	}
}

func rows13WithdrawalStatus(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM fleet_partner_provider_withdrawal_requests WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

// FLEET-13.6c GET .../withdrawals and POST .../withdrawals/:id/replay: the
// listing is scoped and allowlisted; a dead-lettered withdrawal replays into
// the execution queue and the next pass submits it to Wallet's
// provider-withdrawal route.
func TestRow13_6c_WithdrawalListAndReplay(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	wallet := newRows13Wallet(t)
	finance := seedApprovalMember(t, f.pool, f.organization, rows13FinanceRole, "r13-wd-finance")
	operations := seedApprovalMember(t, f.pool, f.organization, "rol_system_operations", "r13-wd-ops")
	analyst := seedApprovalMember(t, f.pool, f.organization, "rol_system_analyst", "r13-wd-analyst")

	request, err := f.service.RequestWithdrawal(ctx, walletusecases.WithdrawalInput{
		Tenant: f.tenant, MembershipID: f.membership, RequestedByActorType: walletusecases.RequesterStaff,
		AmountMinor: 55_000, Currency: "NGN", DestinationReference: "bank_ref_rows13", IdempotencyKey: "r13-wd-1"})
	if err != nil {
		t.Fatal(err)
	}
	approver := f.tenant
	approver.ActorID, approver.OrganizationUserID = "usr_rows13_wd_approver", "usr_rows13_wd_approver"
	if request, err = f.service.DecideWithdrawal(ctx, approver, f.membership, request.ID, request.Version, true, "ok"); err != nil {
		t.Fatal(err)
	}

	router := rows13Router(f.pool, rows13Service(f.pool, wallet, rows13AllOn()))
	listPath := "/fleet-partner/v1/providers/" + f.membership + "/sub-wallet/withdrawals"
	code, body := rowsCall(t, router, "GET", listPath, nil, as(f.organization, operations))
	if code != http.StatusOK {
		t.Fatalf("list withdrawals = %d %v", code, body)
	}
	items := rowsList(t, body)
	if len(items) != 1 {
		t.Fatalf("withdrawals = %v", items)
	}
	view := items[0].(map[string]any)
	if view["id"] != request.ID || view["amountMinor"].(float64) != 55_000 || view["status"] != walletmodels.WithdrawalApproved {
		t.Fatalf("withdrawal view = %v", view)
	}
	if _, leaked := view["destinationBankAccountReference"]; leaked {
		t.Fatal("the destination reference must not appear in a listing")
	}
	if code, _ := rowsCall(t, router, "GET", listPath+"?status=REJECTED", nil, as(f.organization, operations)); code != http.StatusOK {
		t.Fatalf("status filter = %d", code)
	}
	_, body = rowsCall(t, router, "GET", listPath+"?status=REJECTED", nil, as(f.organization, operations))
	if len(rowsList(t, body)) != 0 {
		t.Fatalf("REJECTED filter returned %v", body)
	}
	if code, _ := rowsCall(t, router, "GET", listPath, nil, as(f.organization, analyst)); code != http.StatusForbidden {
		t.Fatalf("analyst list = %d, want 403", code)
	}

	// Wallet down, attempts spent: dead-lettered.
	wallet.setFailSubmissions(true)
	if err := rows13Service(f.pool, wallet, rows13AllOn()).RunPass(ctx, walletusecases.WorkerConfig{Owner: "rows13", MaxAttempts: 1}); err != nil {
		t.Fatal(err)
	}
	if status := rows13WithdrawalStatus(t, f.pool, request.ID); status != walletmodels.WithdrawalDeadLetter {
		t.Fatalf("after exhausted retries = %s, want DEAD_LETTER", status)
	}

	replayPath := listPath + "/" + request.ID + "/replay"
	if code, _ := rowsCall(t, router, "POST", replayPath, map[string]any{}, as(f.organization, operations)); code != http.StatusForbidden {
		t.Fatalf("replay without approve permission = %d, want 403", code)
	}
	if code, _ := rowsCall(t, router, "POST", "/fleet-partner/v1/providers/opm_other/sub-wallet/withdrawals/"+request.ID+"/replay", map[string]any{}, as(f.organization, finance)); code != http.StatusNotFound {
		t.Fatalf("replay through another membership's path = %d, want 404", code)
	}
	code, body = rowsCall(t, router, "POST", replayPath, map[string]any{}, as(f.organization, finance))
	if code != http.StatusOK {
		t.Fatalf("replay = %d %v", code, body)
	}
	if status := rows13WithdrawalStatus(t, f.pool, request.ID); status != walletmodels.WithdrawalApproved {
		t.Fatalf("after replay = %s, want APPROVED", status)
	}
	if n := rowsCount(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id = $1 AND action = 'wallet.provider_withdrawal.replayed'`, f.organization); n != 1 {
		t.Fatalf("replay audit records = %d", n)
	}
	if code, _ := rowsCall(t, router, "POST", replayPath, map[string]any{}, as(f.organization, finance)); code != http.StatusNotFound {
		t.Fatalf("second replay = %d, want 404", code)
	}

	// Wallet recovers: the next pass submits to Wallet's provider-withdrawal
	// route and the request completes.
	wallet.setFailSubmissions(false)
	if err := rows13Service(f.pool, wallet, rows13AllOn()).RunPass(ctx, walletusecases.WorkerConfig{Owner: "rows13", MaxAttempts: 8}); err != nil {
		t.Fatal(err)
	}
	if status := rows13WithdrawalStatus(t, f.pool, request.ID); status != walletmodels.WithdrawalCompleted {
		t.Fatalf("after replayed execution = %s, want COMPLETED (Wallet saw %d withdrawal submissions)", status, wallet.callCount("POST /provider-withdrawals"))
	}
	sent := wallet.lastBody("POST /provider-withdrawals")
	if sent["idempotencyKey"] != "r13-wd-1" || sent["amountMinor"].(float64) != 55_000 ||
		sent["earningAccountId"] != "fear_sub_1" || sent["organizationId"] != f.organization {
		t.Fatalf("withdrawal submission body = %v", sent)
	}
}

// FLEET-13.8 GET/PUT /wallet/policy: the withdrawal/allocation policy is read
// with a safe default, replaced only with the dedicated permission and an
// expected version, validated, audited and announced.
func TestRow13_8_WithdrawalPolicyGetPut(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	wallet := newRows13Wallet(t)
	router := rows13Router(f.pool, rows13Service(f.pool, wallet, rows13AllOn()))
	finance := seedApprovalMember(t, f.pool, f.organization, rows13FinanceRole, "r13-pol-finance")
	analyst := seedApprovalMember(t, f.pool, f.organization, "rol_system_analyst", "r13-pol-analyst")

	// Default before anything is stored: company approval, version 1.
	code, body := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet/policy", nil, as(f.organization, analyst))
	if code != http.StatusOK {
		t.Fatalf("GET policy = %d %v", code, body)
	}
	policy := rowsData(t, body)
	if policy["policyMode"] != walletmodels.PolicyCompanyApproval || policy["version"].(float64) != 1 {
		t.Fatalf("default policy = %v", policy)
	}

	put := func(member string, payload map[string]any, ifMatch string) (int, map[string]any) {
		headers := as(f.organization, member)
		if ifMatch != "" {
			headers["If-Match"] = ifMatch
		}
		return rowsCall(t, router, "PUT", "/fleet-partner/v1/wallet/policy", payload, headers)
	}
	valid := map[string]any{"policyMode": "PROVIDER_DIRECT", "minimumAmountMinor": 5_000,
		"approvalThresholdMinor": 500_000, "allocationApprovalThresholdMinor": 250_000, "currency": "NGN"}

	if code, _ := put(analyst, valid, "1"); code != http.StatusForbidden {
		t.Fatalf("PUT without wallet.policies.manage = %d, want 403", code)
	}
	for name, payload := range map[string]map[string]any{
		"unknown mode":       {"policyMode": "ANYTHING_GOES", "currency": "NGN"},
		"negative threshold": {"policyMode": "PROVIDER_DIRECT", "minimumAmountMinor": -1, "currency": "NGN"},
		"bad currency":       {"policyMode": "PROVIDER_DIRECT", "currency": "NAIRA"},
	} {
		if code, body := put(finance, payload, "1"); code != http.StatusBadRequest {
			t.Fatalf("%s = %d %v, want 400", name, code, body)
		}
	}

	code, body = put(finance, valid, "1")
	if code != http.StatusOK {
		t.Fatalf("first PUT = %d %v", code, body)
	}
	stored := rowsData(t, body)
	if stored["policyMode"] != "PROVIDER_DIRECT" || stored["approvalThresholdMinor"].(float64) != 500_000 || stored["currency"] != "NGN" {
		t.Fatalf("stored policy = %v", stored)
	}
	version := int(stored["version"].(float64))

	// Once a policy exists, a change must state the version it read.
	changed := map[string]any{"policyMode": "COMPANY_INITIATED_ONLY", "currency": "NGN"}
	if code, body := put(finance, changed, ""); code != http.StatusBadRequest {
		t.Fatalf("PUT without an expected version = %d %v, want 400", code, body)
	}
	if code, _ := put(finance, changed, "99"); code != http.StatusConflict {
		t.Fatalf("stale PUT = %d, want 409", code)
	}
	code, body = put(finance, changed, itoaRows(version))
	if code != http.StatusOK || rowsData(t, body)["policyMode"] != "COMPANY_INITIATED_ONLY" ||
		int(rowsData(t, body)["version"].(float64)) != version+1 {
		t.Fatalf("versioned PUT = %d %v", code, body)
	}
	if code, body := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet/policy", nil, as(f.organization, finance)); code != http.StatusOK || rowsData(t, body)["policyMode"] != "COMPANY_INITIATED_ONLY" {
		t.Fatalf("GET after PUT = %d %v", code, body)
	}

	if n := rowsCount(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id = $1 AND action = 'wallet.policy.changed'`, f.organization); n != 2 {
		t.Fatalf("policy audit records = %d, want 2", n)
	}
	if n := rowsCount(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id = $1 AND event_type = 'organization.wallet.policy_changed.v1'`, f.organization); n != 2 {
		t.Fatalf("policy events = %d, want 2", n)
	}

	// Another organization's policy is its own.
	other := createOrganization(t, f.pool, "Rows13 Other Pol Co", "RC-R13-OTHER-6")
	otherFinance := seedApprovalMember(t, f.pool, other.ID, rows13FinanceRole, "r13-pol-other")
	if code, body := rowsCall(t, router, "GET", "/fleet-partner/v1/wallet/policy", nil, as(other.ID, otherFinance)); code != http.StatusOK || rowsData(t, body)["policyMode"] != walletmodels.PolicyCompanyApproval {
		t.Fatalf("other organization's policy = %d %v", code, body)
	}

	// The company-wallet kill switch freezes policy changes.
	killed := app.BuildCompanyWalletService(config.Config{CompanyWalletKillSwitch: true}, f.pool, observability.NewMetrics(), nil, nil, nil, nil)
	if code, _ := rowsCall(t, rows13Router(f.pool, killed), "PUT", "/fleet-partner/v1/wallet/policy", changed, as(f.organization, finance, "If-Match", itoaRows(version+1))); code != http.StatusServiceUnavailable {
		t.Fatalf("PUT under the kill switch = %d, want 503", code)
	}
}

func itoaRows(value int) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// FLEET-13.9a projection refresh worker: stale organization wallet links are
// re-read from payment-wallet-service and restored only from its answer; a
// link Wallet cannot confirm stays not-current, and the worker refuses to run
// when disabled.
func TestRow13_9a_ProjectionRefreshWorker(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	wallet := newRows13Wallet(t)
	service := rows13Service(f.pool, wallet, rows13AllOn())

	// Organization A: stale copy that Wallet will confirm (now RESTRICTED).
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_organization_wallet_provisioning SET freshness_state = 'STALE', synced_at = now() - interval '2 hours' WHERE organization_id = $1`, f.organization); err != nil {
		t.Fatal(err)
	}
	wallet.setAccount("fwal_org_1", f.organization, "RESTRICTED", 11)
	// Organization B: stale copy Wallet does not know.
	other := createOrganization(t, f.pool, "Rows13 Refresh Other", "RC-R13-OTHER-7")
	if _, err := f.pool.Exec(ctx, `INSERT INTO fleet_partner_organization_wallet_provisioning (organization_id, status, wallet_account_id, currency, freshness_state, source_version, synced_at) VALUES ($1, 'ACTIVE', 'fwal_org_missing', 'NGN', 'STALE', 2, now() - interval '2 hours')`, other.ID); err != nil {
		t.Fatal(err)
	}
	// Organization C: current copy; must not be re-read.
	current := createOrganization(t, f.pool, "Rows13 Refresh Current", "RC-R13-OTHER-8")
	if _, err := f.pool.Exec(ctx, `INSERT INTO fleet_partner_organization_wallet_provisioning (organization_id, status, wallet_account_id, currency, freshness_state, source_version, synced_at) VALUES ($1, 'ACTIVE', 'fwal_org_current', 'NGN', 'CURRENT', 4, now())`, current.ID); err != nil {
		t.Fatal(err)
	}

	refreshed, err := service.RefreshProjections(ctx, walletusecases.WorkerConfig{Owner: "rows13-refresh"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed != 1 {
		t.Fatalf("refreshed = %d, want 1", refreshed)
	}
	var status, freshness string
	var version int64
	if err := f.pool.QueryRow(ctx, `SELECT status, freshness_state, source_version FROM fleet_partner_organization_wallet_provisioning WHERE organization_id = $1`, f.organization).Scan(&status, &freshness, &version); err != nil {
		t.Fatal(err)
	}
	if status != walletmodels.LinkRestricted || freshness != walletmodels.FreshnessCurrent || version != 11 {
		t.Fatalf("refreshed link = %s/%s/v%d, want RESTRICTED/CURRENT/v11", status, freshness, version)
	}
	if freshness := rows13Freshness(t, f.pool, other.ID); freshness == walletmodels.FreshnessCurrent {
		t.Fatal("a link Wallet could not confirm was marked CURRENT")
	}
	if freshness := rows13Freshness(t, f.pool, current.ID); freshness != walletmodels.FreshnessCurrent {
		t.Fatalf("current link changed to %s", freshness)
	}
	if wallet.callCount("GET /wallets/:id")+wallet.callCount("GET /earning-accounts/:id") == 0 {
		t.Fatal("the worker never asked Wallet")
	}

	// Disabled: the worker does nothing and says so.
	disabled := rows13Service(f.pool, wallet, rows13Flags{read: true})
	if n, err := disabled.RefreshProjections(ctx, walletusecases.WorkerConfig{}, 50); err == nil || n != 0 {
		t.Fatalf("disabled refresh = %d, %v; want 0 and an error", n, err)
	}
}
