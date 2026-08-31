package http

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
)

type analyticsTestLimiter struct{ counts map[string]int }

func (l *analyticsTestLimiter) Allow(_ context.Context, _, key string, max int, _ time.Duration) error {
	if l.counts == nil {
		l.counts = map[string]int{}
	}
	l.counts[key]++
	if l.counts[key] > max {
		return apperrors.RateLimited("analytics limit reached", nil)
	}
	return nil
}
func (l *analyticsTestLimiter) Reset() { l.counts = map[string]int{} }

func TestAnalyticsReadRateLimitAllowanceIsolationAndReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := &analyticsTestLimiter{}
	handler := (&Handler{}).WithReadRateLimit(limiter, 1, time.Minute)
	check := func(caller, business string) bool {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest("GET", "/?branch_id=branch-a", nil)
		return handler.allowRead(context, caller, business)
	}
	if !check("vendor-service", "business-a") {
		t.Fatal("first analytics read was limited")
	}
	if check("vendor-service", "business-a") {
		t.Fatal("exceeded analytics read was allowed")
	}
	if !check("vendor-service", "business-b") {
		t.Fatal("different business shared analytics bucket")
	}
	limiter.Reset()
	if !check("vendor-service", "business-a") {
		t.Fatal("new analytics window was not allowed")
	}
}
