package orders

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

type supportTestLimiter struct{ counts map[string]int }

func (l *supportTestLimiter) Allow(_ context.Context, _, key string, max int, _ time.Duration) error {
	if l.counts == nil {
		l.counts = map[string]int{}
	}
	l.counts[key]++
	if l.counts[key] > max {
		return apperrors.RateLimited("support limit reached", nil)
	}
	return nil
}
func (l *supportTestLimiter) Reset() { l.counts = map[string]int{} }

func TestSupportRateLimitAllowanceIsolationAndReset(t *testing.T) {
	limiter := &supportTestLimiter{}
	service := &Service{}
	service.SetSupportCaseRateLimiter(limiter, 1, time.Hour)
	if err := service.limitSupportCase(context.Background(), "customer:a"); err != nil {
		t.Fatalf("first support request: %v", err)
	}
	if err := service.limitSupportCase(context.Background(), "customer:a"); apperrors.From(err).Code != apperrors.CodeRateLimited {
		t.Fatalf("exceeded support request: %v", err)
	}
	if err := service.limitSupportCase(context.Background(), "customer:b"); err != nil {
		t.Fatalf("different principal: %v", err)
	}
	limiter.Reset()
	if err := service.limitSupportCase(context.Background(), "customer:a"); err != nil {
		t.Fatalf("new window support request: %v", err)
	}
}
