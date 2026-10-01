package customershare

import "testing"

func TestExecutorAuthorityFailsClosed(t *testing.T) {
	for service, want := range map[string]string{"DISPATCH": "DISPATCH", "HAULING": "HAULING", "TAXI": "TAXI_GOODS", "TAXI_GOODS": "TAXI_GOODS"} {
		if got, ok := executorAuthority(service); !ok || got != want {
			t.Fatalf("%s -> %q,%v want %q", service, got, ok, want)
		}
	}
	// Passenger Ride, Vendor's own runtime and anything unknown never resolve
	// to a logistics read authority.
	for _, service := range []string{"", "RIDE", "VENDOR", "VENDOR_OWN_DELIVERY", "TELEPORT", "dispatch"} {
		if got, ok := executorAuthority(service); ok {
			t.Fatalf("%q resolved to %q", service, got)
		}
	}
}
