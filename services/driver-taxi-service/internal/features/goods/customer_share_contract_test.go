package goods

import (
	"cosmicforge/logistics/shared/go/httpx"
	ct "cosmicforge/logistics/shared/go/journeyshare/journeysharetest"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"testing"
	"time"
)

func TestMountedGoodsShareContractAndAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(httpx.ErrorHandler())
	RegisterCustomerShare(e, NewService(Options{}), serviceauth.NewVerifier(serviceauth.Secrets{"contract-wrong-caller": []byte("contract-test-secret")}, 5*time.Minute))
	ct.AssertContracts(t, e, ct.Contract{File: "../../../docs/openapi/commerce-share-mission.openapi.json", Security: "HMACv2"})
}
