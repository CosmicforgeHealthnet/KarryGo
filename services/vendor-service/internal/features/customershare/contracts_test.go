package customershare

import (
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/journeyshare"
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
	Register(e, &Service{}, journeyshare.RedisBoundary(serviceauth.NewVerifier(serviceauth.Secrets{"contract-wrong-caller": []byte("contract-test-secret")}, 5*time.Minute), nil, "customer-service"))
	ct.AssertContracts(t, e, ct.Contract{File: "../../../docs/openapi/customer-share-mission-projection.openapi.json", Security: "HMACv2"})
}
