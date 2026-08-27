package searchprojection

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type searchTestLimiter struct{ counts map[string]int }

func (l *searchTestLimiter) Allow(_ context.Context, _, key string, max int, _ time.Duration) error {
	if l.counts == nil {
		l.counts = map[string]int{}
	}
	l.counts[key]++
	if l.counts[key] > max {
		return apperrors.RateLimited("search limit reached", nil)
	}
	return nil
}
func (l *searchTestLimiter) Reset() { l.counts = map[string]int{} }

func TestSearchRateLimitAllowanceIsolationAndReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := &searchTestLimiter{}
	handler := NewInternalHandler(InternalHandlerOptions{Limiter: limiter, Limit: RequestLimitPolicy{Max: 1, Window: time.Minute}})
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	router.GET("/search/:caller", func(c *gin.Context) { c.Set("service_name", c.Param("caller")); c.Next() }, handler.limitSearch, func(c *gin.Context) { c.Status(http.StatusOK) })
	request := func(caller string) int {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/search/"+caller, nil))
		return recorder.Code
	}
	if got := request("customer-service"); got != http.StatusOK {
		t.Fatalf("first search status=%d", got)
	}
	if got := request("customer-service"); got != http.StatusTooManyRequests {
		t.Fatalf("exceeded search status=%d", got)
	}
	if got := request("api-gateway"); got != http.StatusOK {
		t.Fatalf("different caller status=%d", got)
	}
	limiter.Reset()
	if got := request("customer-service"); got != http.StatusOK {
		t.Fatalf("new window search status=%d", got)
	}
}
