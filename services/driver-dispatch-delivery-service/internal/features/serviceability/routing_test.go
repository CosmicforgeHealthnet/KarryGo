package serviceability

import "testing"

func TestCircularZoneAndRestrictedAreas(t *testing.T) {
	zone := NewCircularZone(Point{Latitude: 6.5244, Longitude: 3.3792}, 20, "6.55,3.40,1")
	pickup, dropoff, restricted, available := zone.Evaluate(
		Point{Latitude: 6.52, Longitude: 3.38}, Point{Latitude: 6.55, Longitude: 3.40},
	)
	if !available || !pickup || !dropoff || !restricted {
		t.Fatalf("zone decision=%t %t %t %t", pickup, dropoff, restricted, available)
	}
	bad := NewCircularZone(Point{Latitude: 6.5244, Longitude: 3.3792}, 20, "bad-policy")
	_, _, _, available = bad.Evaluate(Point{Latitude: 6.52, Longitude: 3.38}, Point{Latitude: 6.53, Longitude: 3.39})
	if available {
		t.Fatal("malformed restricted-zone policy did not fail closed")
	}
}
