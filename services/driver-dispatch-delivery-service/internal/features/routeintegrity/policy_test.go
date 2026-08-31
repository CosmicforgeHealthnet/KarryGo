package routeintegrity

import (
	"math"
	"testing"
)

func TestDistanceToExpectedCorridor(t *testing.T) {
	route := []Point{{Latitude: 6.5, Longitude: 3.4}, {Latitude: 6.6, Longitude: 3.5}}
	on := distanceToRouteMeters(Point{Latitude: 6.55, Longitude: 3.45}, route)
	off := distanceToRouteMeters(Point{Latitude: 6.55, Longitude: 3.55}, route)
	if on > 50 {
		t.Fatalf("on-route distance = %f", on)
	}
	if off < 5000 {
		t.Fatalf("off-route distance = %f", off)
	}
}
func TestWrongWayAngleIsBounded(t *testing.T) {
	bearing := bearingDegrees(Point{Latitude: 0, Longitude: 0}, Point{Latitude: 1, Longitude: 0})
	if math.Abs(bearing) > 0.1 {
		t.Fatalf("north bearing=%f", bearing)
	}
	if angleDelta(350, 10) != 20 {
		t.Fatalf("wrapped delta=%f", angleDelta(350, 10))
	}
}
func TestOfflineFingerprintBindsSemantics(t *testing.T) {
	base := DeviceCommand{DeviceID: "device", DeviceCommandID: "command", CommandType: "LOCATION_FIX", Payload: map[string]any{"latitude": 6.5}, LastKnownServerVersion: 2}
	same := base
	changed := base
	changed.Payload = map[string]any{"latitude": 7.5}
	if commandFingerprint("provider", base) != commandFingerprint("provider", same) {
		t.Fatal("same command fingerprint changed")
	}
	if commandFingerprint("provider", base) == commandFingerprint("provider", changed) {
		t.Fatal("changed payload retained fingerprint")
	}
}

func TestOfflineCommandPolicyFailsClosedForCustodyAndHandover(t *testing.T) {
	tests := []struct {
		kind, state, reason string
	}{
		{"LOCATION_FIX", "ACCEPTED", ""},
		{"EVIDENCE_REFERENCE", "PENDING_EVIDENCE_SYNC", "MEDIA_AUTHORIZATION_REQUIRED"},
		{"PARCEL_SCAN", "REJECTED", "ONLINE_MANIFEST_VALIDATION_REQUIRED"},
		{"PICKUP_COMPLETE", "REJECTED", "ONLINE_HANDOVER_VERIFICATION_REQUIRED"},
		{"DELIVERY_COMPLETE", "REJECTED", "ONLINE_HANDOVER_VERIFICATION_REQUIRED"},
		{"PROVIDER_TRANSFER", "REJECTED", "ONLINE_HANDOVER_VERIFICATION_REQUIRED"},
	}
	for _, tt := range tests {
		state, reason, ok := classifyOfflineCommand(tt.kind)
		if !ok || state != tt.state || reason != tt.reason {
			t.Fatalf("%s => %s/%s/%v", tt.kind, state, reason, ok)
		}
	}
	if _, _, ok := classifyOfflineCommand("CLIENT_SET_DELIVERED"); ok {
		t.Fatal("unknown client command was accepted")
	}
}
