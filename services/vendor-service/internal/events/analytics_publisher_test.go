package events

import (
	"testing"

	"cosmicforge/logistics/shared/go/commerceanalytics"
)

func TestAnalyticsEventTypesIncludeExistingCommerceSources(t *testing.T) {
	got := map[string]bool{}
	for _, eventType := range AnalyticsEventTypes() {
		got[eventType] = true
	}
	for _, eventType := range []string{
		commerceanalytics.EventRFQQuoteSubmitted,
		commerceanalytics.EventRFQAccepted,
		commerceanalytics.EventSupportCaseResolved,
		commerceanalytics.EventOrderCompleted,
		commerceanalytics.EventSearchDemand,
	} {
		if !got[eventType] {
			t.Fatalf("analytics publisher missing %s", eventType)
		}
	}
}
