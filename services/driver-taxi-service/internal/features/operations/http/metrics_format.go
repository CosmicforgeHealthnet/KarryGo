package operationshttp

import (
	"fmt"
	"sort"
	"strings"

	bookingusecases "cosmicforge/logistics/services/taxi-service/internal/features/booking/usecases"
)

// formatPrometheusText renders a MetricsSnapshot in the standard Prometheus
// text exposition format. Sorted by status so repeated scrapes diff
// cleanly; no third-party client library involved (none exists anywhere in
// this monorepo -- confirmed before writing this, per the build plan's own
// "confirm at implementation time" instruction for phase B10 metrics).
func formatPrometheusText(s bookingusecases.MetricsSnapshot) string {
	var b strings.Builder
	b.WriteString("# HELP taxi_bookings_total Current count of bookings by status.\n")
	b.WriteString("# TYPE taxi_bookings_total gauge\n")

	statuses := make([]string, 0, len(s.BookingsByStatus))
	for status := range s.BookingsByStatus {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	for _, status := range statuses {
		fmt.Fprintf(&b, "taxi_bookings_total{status=%q} %d\n", status, s.BookingsByStatus[status])
	}

	b.WriteString("# HELP taxi_providers_online Current count of online providers.\n")
	b.WriteString("# TYPE taxi_providers_online gauge\n")
	fmt.Fprintf(&b, "taxi_providers_online %d\n", s.ProvidersOnline)

	return b.String()
}
