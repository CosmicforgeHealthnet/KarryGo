package shop

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func signedShopInternalRequest(t *testing.T, method, path, caller string, secret, body []byte, nonce string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set(serviceauth.HeaderNonce, nonce)
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if err := serviceauth.SignRequest(request, caller, secret, body, time.Now()); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestInternalShopInventoryHMACAllowlistNonceAndBranchBinding(t *testing.T) {
	f := section12Fixture(t)
	basis := createInventoryBasis(t, f, "Internal Contract", 10)
	gin.SetMode(gin.TestMode)
	secret := []byte("shop-internal-contract-secret")
	attackerSecret := []byte("shop-internal-attacker-secret")
	secrets := serviceauth.Secrets{"order-service": secret, "attacker-service": attackerSecret}
	router := gin.New()
	RegisterInternalInventoryRoutes(router, NewInventoryInternalHandler(f.service, []string{"order-service"}), serviceauth.NewVerifier(secrets, 5*time.Minute), Flags{ShopEnabled: true, WritesEnabled: true})
	revisionPath := "/internal/vendor/v1/shop/branches/" + f.branchShop + "/revision"

	unsigned := httptest.NewRecorder()
	router.ServeHTTP(unsigned, httptest.NewRequest(http.MethodGet, revisionPath, nil))
	if unsigned.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned internal request status=%d", unsigned.Code)
	}
	wrongSignature := httptest.NewRecorder()
	router.ServeHTTP(wrongSignature, signedShopInternalRequest(t, http.MethodGet, revisionPath, "order-service", []byte("wrong-secret"), nil, "shop-wrong-signature"))
	if wrongSignature.Code != http.StatusUnauthorized {
		t.Fatalf("wrong signature status=%d", wrongSignature.Code)
	}
	wrongCaller := httptest.NewRecorder()
	router.ServeHTTP(wrongCaller, signedShopInternalRequest(t, http.MethodGet, revisionPath, "attacker-service", attackerSecret, nil, "shop-wrong-caller"))
	if wrongCaller.Code != http.StatusForbidden {
		t.Fatalf("unapproved caller status=%d", wrongCaller.Code)
	}
	valid := httptest.NewRecorder()
	router.ServeHTTP(valid, signedShopInternalRequest(t, http.MethodGet, revisionPath, "order-service", secret, nil, "shop-valid-revision"))
	if valid.Code != http.StatusOK {
		t.Fatalf("valid internal request status=%d body=%s", valid.Code, valid.Body.String())
	}
	replay := httptest.NewRecorder()
	router.ServeHTTP(replay, signedShopInternalRequest(t, http.MethodGet, revisionPath, "order-service", secret, nil, "shop-valid-revision"))
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replayed nonce status=%d", replay.Code)
	}

	body := []byte(`{"source_type":"ORDER","source_ref":"internal-order-1","lines":[{"stock_basis_id":"` + basis.ID + `","quantity":4}]}`)
	reservePath := "/internal/vendor/v1/shop/branches/" + f.branchShop + "/inventory/reservations"
	reserved := httptest.NewRecorder()
	router.ServeHTTP(reserved, signedShopInternalRequest(t, http.MethodPost, reservePath, "order-service", secret, body, "shop-valid-reserve"))
	if reserved.Code != http.StatusCreated {
		t.Fatalf("valid internal reservation status=%d body=%s", reserved.Code, reserved.Body.String())
	}

	wrongBranchPath := "/internal/vendor/v1/shop/branches/" + f.branchShop2 + "/inventory/reservations"
	wrongBranch := httptest.NewRecorder()
	router.ServeHTTP(wrongBranch, signedShopInternalRequest(t, http.MethodPost, wrongBranchPath, "order-service", secret, []byte(`{"source_type":"ORDER","source_ref":"nested-idor","lines":[{"stock_basis_id":"`+basis.ID+`","quantity":1}]}`), "shop-nested-idor"))
	if wrongBranch.Code != http.StatusNotFound {
		t.Fatalf("branch path accepted another branch stock basis: status=%d body=%s", wrongBranch.Code, wrongBranch.Body.String())
	}

	emptyAllowlistRouter := gin.New()
	RegisterInternalInventoryRoutes(emptyAllowlistRouter, NewInventoryInternalHandler(f.service, nil), serviceauth.NewVerifier(secrets, 5*time.Minute), Flags{ShopEnabled: true, WritesEnabled: true})
	emptyAllowlist := httptest.NewRecorder()
	emptyAllowlistRouter.ServeHTTP(emptyAllowlist, signedShopInternalRequest(t, http.MethodGet, revisionPath, "order-service", secret, nil, "shop-empty-allowlist"))
	if emptyAllowlist.Code != http.StatusForbidden {
		t.Fatalf("empty caller allowlist did not fail closed: status=%d", emptyAllowlist.Code)
	}
}
