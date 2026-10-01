package customershare

import (
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
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
	v := serviceauth.NewVerifier(serviceauth.Secrets{"contract-wrong-caller": []byte("contract-test-secret")}, 5*time.Minute)
	Register(e, &Service{}, authority.ServiceBoundary(v, nil, []string{"customer-service"}), authority.ServiceBoundary(v, nil, []string{"vendor-service"}))
	ct.AssertContracts(t, e, ct.Contract{File: "../../../docs/openapi/customer-share-projection.openapi.json", Security: "HMACv2"})
}
