package cart

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func section17ATestRouter(
	t *testing.T,
) (*gin.Engine, *sharedauth.TokenSigner) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	router := gin.New()

	signer := sharedauth.NewTokenSigner(
		[]byte("section17a-customer-secret"),
	)

	service := NewService(
		NewStore(nil),
		nil,
		nil,
		nil,
	)

	group := router.Group(
		"/api/v1/commerce",
	)

	RegisterCustomerRoutes(
		group,
		service,
		signer,
		nil,
	)

	return router, signer
}

func TestCustomerCartRouteContract(
	t *testing.T,
) {
	router, _ := section17ATestRouter(t)

	actual := make(
		[]string,
		0,
		len(router.Routes()),
	)

	for _, route := range router.Routes() {
		actual = append(
			actual,
			route.Method+" "+route.Path,
		)
	}

	sort.Strings(actual)

	expected := []string{
		"DELETE /api/v1/commerce/carts/:cartId/delivery-address",
		"DELETE /api/v1/commerce/carts/:cartId/items",
		"DELETE /api/v1/commerce/carts/:cartId/items/:itemId",
		"DELETE /api/v1/commerce/carts/:cartId/receiver",
		"GET /api/v1/commerce/carts/:cartId",
		"GET /api/v1/commerce/carts/:cartId/groups",
		"PATCH /api/v1/commerce/carts/:cartId/items/:itemId",
		"POST /api/v1/commerce/carts",
		"POST /api/v1/commerce/carts/:cartId/abandon",
		"POST /api/v1/commerce/carts/:cartId/items",
		"PUT /api/v1/commerce/carts/:cartId/delivery-address",
		"PUT /api/v1/commerce/carts/:cartId/receiver",
	}

	sort.Strings(expected)

	if len(actual) != len(expected) {
		t.Fatalf(
			"route count=%d want=%d\nactual=%v",
			len(actual),
			len(expected),
			actual,
		)
	}

	for i := range expected {
		if actual[i] != expected[i] {
			t.Fatalf(
				"route[%d]=%q want=%q",
				i,
				actual[i],
				expected[i],
			)
		}
	}
}

func TestCreateCartStrictJSONRejectsWritableCustomerID(
	t *testing.T,
) {
	router, signer := section17ATestRouter(t)

	token, err := signer.Sign(
		sharedauth.Claims{
			Subject:   "customer-a",
			Role:      "customer",
			Service:   "customer",
			SessionID: "session-a",
			Type:      sharedauth.TokenTypeAccess,
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/commerce/carts",
		bytes.NewBufferString(
			`{"domain":"FOOD","customer_id":"customer-b"}`,
		),
	)

	request.Header.Set(
		"Authorization",
		"Bearer "+token,
	)

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"status=%d body=%s; writable customer_id must fail before database access",
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestIfMatchParserAcceptsQuotedVersion(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(
		httptest.NewRecorder(),
	)

	c.Request = httptest.NewRequest(
		http.MethodPatch,
		"/",
		nil,
	)

	c.Request.Header.Set(
		"If-Match",
		`"17"`,
	)

	version, err := customerCartExpectedVersion(c)
	if err != nil {
		t.Fatal(err)
	}

	if version != 17 {
		t.Fatalf(
			"version=%d want=17",
			version,
		)
	}
}

func TestIfMatchParserRejectsMissingVersion(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(
		httptest.NewRecorder(),
	)

	c.Request = httptest.NewRequest(
		http.MethodPatch,
		"/",
		nil,
	)

	if _, err := customerCartExpectedVersion(c); err == nil {
		t.Fatal("missing If-Match must fail")
	}
}
