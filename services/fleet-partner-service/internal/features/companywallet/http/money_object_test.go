package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// FLEET-6.1: every money pair in a response also carries the standard Money
// object, and the object always equals the flat fields it sits beside.

type moneyTestGrants map[string][]staffmodels.RoleBinding

func (g moneyTestGrants) EffectivePermissions(context.Context, string, string) (map[string][]staffmodels.RoleBinding, error) {
	return g, nil
}

// moneyTestAuthorizer grants exactly one permission at organization scope.
func moneyTestAuthorizer(permission string) *accesscontrol.Authorizer {
	binding := staffmodels.RoleBinding{OrganizationID: "org_test", OrganizationUserID: "member_test",
		ScopeType: staffmodels.ScopeOrganization, EffectiveFrom: time.Now().Add(-time.Minute)}
	return accesscontrol.NewAuthorizer(moneyTestGrants{permission: {binding}}, nil, 0)
}

// serveMoneyRoute calls one GET route through gin and returns the decoded
// success envelope's data.
func serveMoneyRoute(t *testing.T, register func(*gin.RouterGroup), path string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: "org_test",
			OrganizationUserID: "member_test", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "member_test"})
		c.Next()
	})
	register(router.Group("/fleet-partner/v1"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/fleet-partner/v1"+path, nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d body %s", path, resp.Code, resp.Body.String())
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(resp.Body.Bytes()))
	decoder.UseNumber()
	if err := decoder.Decode(&body); err != nil {
		t.Fatalf("decode %s: %v", resp.Body.String(), err)
	}
	return body.Data
}

// assertMoneyObject checks object[moneyKey] == {amountMinor: object[minorKey],
// currency: object[currencyKey] or the given fixed currency}.
func assertMoneyObject(t *testing.T, object map[string]any, moneyKey, minorKey, currencyKey, wantCurrency string) {
	t.Helper()
	got, ok := object[moneyKey].(map[string]any)
	if !ok {
		t.Fatalf("%s missing or not an object in %v", moneyKey, object)
	}
	if got["amountMinor"] != object[minorKey] {
		t.Fatalf("%s.amountMinor = %v, flat %s = %v", moneyKey, got["amountMinor"], minorKey, object[minorKey])
	}
	currency := wantCurrency
	if currencyKey != "" {
		currency, _ = object[currencyKey].(string)
	}
	if got["currency"] != currency || currency == "" {
		t.Fatalf("%s.currency = %v, want %q", moneyKey, got["currency"], currency)
	}
	if len(got) != 2 {
		t.Fatalf("%s has unexpected keys: %v", moneyKey, got)
	}
}

type moneyWalletUseCases struct{ UseCases }

func (moneyWalletUseCases) GetTransaction(_ context.Context, _ tenant.Context, id string) (models.TransactionProjection, error) {
	return models.TransactionProjection{ID: id, AmountMinor: 4200, Currency: "NGN", OccurredAt: time.Now()}, nil
}

func TestWalletTransactionResponseCarriesMoneyObject(t *testing.T) {
	h := NewHandler(moneyWalletUseCases{}, moneyTestAuthorizer(PermissionTransactionsRead))
	data := serveMoneyRoute(t, h.RegisterTenantRoutes, "/wallet/transactions/wtx_money")
	assertMoneyObject(t, data, "amount", "amountMinor", "currency", "")
}
