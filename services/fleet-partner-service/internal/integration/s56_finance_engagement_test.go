package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	engagementmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	engagementrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/repositories"
	engagementusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/usecases"
	financeopshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/http"
	financeopsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	financeopsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/repositories"
	financeopsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// A support event for a case Fleet has never seen creates the projection.
// The insert used to fail with "inconsistent types deduced for parameter $6"
// because the source version was both a bigint column value and compared
// with an integer literal.
func TestSupportEventForUnknownCaseCreatesProjection(t *testing.T) {
	pool := testPool(t)
	scope, _, _ := section2223Scope(t, pool)
	ctx := context.Background()
	svc := engagementusecases.NewService(engagementusecases.Options{Store: engagementrepos.NewRepository(pool),
		Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Flags: engagementmodels.Flags{Support: true}})
	for _, tc := range []struct {
		version int64
		gap     string
	}{{1, "CURRENT"}, {4, "GAP_DETECTED"}} {
		caseID := ids.New("sup")
		if err := svc.ApplySupportEvent(ctx, engagementmodels.SupportEvent{EventID: ids.New("sev"), OrganizationID: scope.OrganizationID, CaseID: caseID, EventType: "created", Status: "open", Priority: "high", SourceVersion: tc.version}); err != nil {
			t.Fatalf("version %d: %v", tc.version, err)
		}
		projection, err := svc.GetSupportCase(ctx, scope, caseID)
		if err != nil {
			t.Fatal(err)
		}
		if projection["gap_status"] != tc.gap || v64(projection, "source_version") != tc.version || projection["case_type"] != "PLATFORM" {
			t.Fatalf("version %d projection = %+v", tc.version, projection)
		}
	}
}

// Finance-operations responses carry the spec 6.1 Money object beside each
// flat money pair, equal to the flat fields, which stay unchanged.
func TestFinanceOpsExpenseResponseCarriesMoneyObject(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Money Object Co "+ids.New("org"), "RC-MONEY-"+ids.New("reg"))
	owner := seedApprovalMember(t, pool, org.ID, "rol_system_owner", "owner")
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: owner, ActorType: tenant.ActorTypeOrganizationUser, ActorID: owner}
	service := financeopsusecases.NewService(financeopsusecases.Options{Store: financeopsrepos.NewRepository(pool), Flags: financeopsmodels.Flags{Expenses: true}})
	expense, err := service.CreateExpense(context.Background(), scope, financeopsmodels.ExpenseInput{Category: "fuel", CostCentre: "OPS", AmountMinor: 125050, Currency: "ngn", Description: "Diesel"})
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, scope)
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	financeopshttp.NewHandler(service, authorizer, nil).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))

	get := func(path string) json.RawMessage {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, recorder.Code, recorder.Body.String())
		}
		var body struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Data
	}
	type record struct {
		AmountMinor int64  `json:"amount_minor"`
		Currency    string `json:"currency"`
		Amount      *struct {
			AmountMinor int64  `json:"amountMinor"`
			Currency    string `json:"currency"`
		} `json:"amount"`
	}
	check := func(label string, r record) {
		if r.AmountMinor != 125050 || r.Currency != "NGN" {
			t.Fatalf("%s flat fields changed: %+v", label, r)
		}
		if r.Amount == nil || r.Amount.AmountMinor != r.AmountMinor || r.Amount.Currency != r.Currency {
			t.Fatalf("%s money object = %+v, want the flat amount", label, r.Amount)
		}
	}
	var one record
	if err := json.Unmarshal(get("/fleet-partner/v1/expenses/"+expense["id"].(string)), &one); err != nil {
		t.Fatal(err)
	}
	check("detail", one)
	var list []record
	if err := json.Unmarshal(get("/fleet-partner/v1/expenses"), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %d items", len(list))
	}
	check("list", list[0])
}
