package customertracking

import (
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	ct "cosmicforge/logistics/shared/go/journeyshare/journeysharetest"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"testing"
	"time"
)

func TestMountedContractsAndAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(httpx.ErrorHandler())
	RegisterCustomer(e.Group("/api/v1/taxi"), &Postgres{}, auth.NewTokenSigner([]byte("test")), false)
	RegisterInternal(e, &Postgres{}, serviceauth.NewVerifier(serviceauth.Secrets{"contract-wrong-caller": []byte("contract-test-secret")}, 5*time.Minute), nil, false)
	ct.AssertContracts(t, e, ct.Contract{File: "../../../docs/openapi/customer-live-location.openapi.json", Security: "CustomerBearer"}, ct.Contract{File: "../../../docs/openapi/customer-share-projection.openapi.json", Security: "HMACv2"})
}
