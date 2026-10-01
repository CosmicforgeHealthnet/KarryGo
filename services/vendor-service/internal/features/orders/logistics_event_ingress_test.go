package orders

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLogisticsIngressAllowlistMapsOnlyCanonicalCallers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name       string
		caller     string
		wantStatus int
	}{
		{"dispatch", DispatchServiceCaller, 200},
		{"hauling", HaulingServiceCaller, 200},
		{"taxi goods", TaxiServiceCaller, 200},
		{"taxi forbidden", "taxi-service", 403},
		{"forged forbidden", "attacker-service", 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("service_name", test.caller)
			called := false
			(&logisticsEventIngress{}).allowCaller(ctx)
			if test.wantStatus == 200 {
				called = !ctx.IsAborted()
			} else {
				called = ctx.IsAborted() && ctx.Writer.Status() == test.wantStatus
			}
			if !called {
				t.Fatalf("caller %q did not produce expected status %d", test.caller, test.wantStatus)
			}
		})
	}
}
