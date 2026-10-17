package trip

import (
	"context"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

func TestActivityFilterSQLScopesCountAndPage(t *testing.T) {
	from := time.Date(2026, 10, 15, 0, 0, 0, 0, time.FixedZone("WAT", 3600))
	before := from.Add(24 * time.Hour)
	where, args := providerTripFilters("owner", ListTripsOptions{Status: "active", CreatedFrom: &from, CreatedBefore: &before})
	for _, required := range []string{"provider_id=$1", "NOT IN ('completed','cancelled','failed')", "created_at>=$2", "created_at<$3"} {
		if !strings.Contains(where, required) {
			t.Fatalf("missing %s in %s", required, where)
		}
	}
	if len(args) != 3 || args[0] != "owner" || !args[1].(time.Time).Equal(from) || !args[2].(time.Time).Equal(before) {
		t.Fatalf("incorrect filter args: %v", args)
	}
}

func TestActivityInvalidDateRangeRejected(t *testing.T) {
	start := time.Now()
	_, err := NewService(newFakeRepository(), nil).ListProviderTrips(context.Background(), uuid.NewString(), ListTripsOptions{Limit: 20, CreatedFrom: &start, CreatedBefore: &start})
	if err == nil {
		t.Fatal("equal date bounds accepted")
	}
}
