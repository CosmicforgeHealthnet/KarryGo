package networkanalytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClassifyMapsEveryProducerSpelling(t *testing.T) {
	cases := map[string]struct {
		authority string
		kind      Kind
	}{
		"taxi.recovery.started":                    {AuthorityTaxi, KindRecoveryStarted},
		"taxi.recovery.started.v1":                 {AuthorityTaxi, KindRecoveryStarted},
		"taxi.guarantee.breached":                  {AuthorityTaxi, KindGuaranteeBreached},
		"hauling.recovery.resolved":                {AuthorityHauling, KindRecoveryResolved},
		"hauling.capability_mismatch":              {AuthorityHauling, KindCapabilityMismatch},
		"dispatch.capability_mismatch.v1":          {AuthorityDispatch, KindCapabilityMismatch},
		"dispatch.provider_replacement.started.v1": {AuthorityDispatch, KindRecoveryReplaced},
		"trip.completed.v1":                        {AuthorityDispatch, KindMissionCompleted},
		"dispatch.booking.unmatched.v1":            {AuthorityDispatch, KindMissionFailed},
		"dispatch.booking.cancelled.v1":            {AuthorityDispatch, KindMissionCancelled},
		"dispatch.incident_recovery.started.v1":    {AuthorityDispatch, KindRecoveryStarted},
		"dispatch.incident_recovery.resolved.v1":   {AuthorityDispatch, KindRecoveryResolved},
		"dispatch.incident_recovery.failed.v1":     {AuthorityDispatch, KindRecoveryFailed},
		"dispatch.delivery_recovery.resolved.v1":   {AuthorityDispatch, KindRecoveryResolved},
		"dispatch.delivery_recovery.failed.v1":     {AuthorityDispatch, KindRecoveryFailed},
		"taxi.booking.unmatched":                   {AuthorityTaxi, KindMissionFailed},
		"hauling.booking.unmatched.v1":             {AuthorityHauling, KindMissionFailed},
		EventBranchReadiness:                       {AuthorityVendor, KindBranchReadiness},
	}
	for name, want := range cases {
		authority, kind, ok := Classify(name, "")
		if !ok || authority != want.authority || kind != want.kind {
			t.Errorf("%s -> %s/%s ok=%v, want %s/%s", name, authority, kind, ok, want.authority, want.kind)
		}
	}
	if _, _, ok := Classify("commerce.order.completed", ""); ok {
		t.Fatal("a commerce event is not a network event")
	}
}

func TestSnapshotAuthorityComesFromThePayload(t *testing.T) {
	authority, kind, ok := Classify(EventSupplySnapshot, "hauling")
	if !ok || authority != AuthorityHauling || kind != KindSupplySnapshot || !kind.Snapshot() {
		t.Fatalf("got %s/%s ok=%v", authority, kind, ok)
	}
	if _, _, ok = Classify(EventSupplySnapshot, "VENDOR"); ok {
		t.Fatal("a supply snapshot must name a logistics authority")
	}
}

func TestMinimizeKeepsMetricsAndDropsPeopleAndMoney(t *testing.T) {
	out := Minimize(map[string]any{
		"recovery_case_id": "c1", "original_provider_id": "p1", "reason_code": "BREAKDOWN",
		"late_seconds": 42, "platform_share_ratio": 1, "shipping_mode": "x",
		"pickup_lat": 6.5, "pickup_lng": 3.4, "dropoff_address": "12 Street",
		"receiver_phone": "+234", "customer_email": "a@b", "provider_name": "Ada",
		"fare_amount_kobo": 1000, "platform_fee": 10, "delivery_code_hash": "h", "otp": "1",
		"pickupAddress": "x", "receiverPhone": "y", "fareKobo": 1, "providerId": "p9", "distanceKm": 3,
		"nested": map[string]any{"zone": "z1", "lat": 1.0},
		"items":  []any{map[string]any{"description": "fragile", "count": 2}},
	})
	for _, kept := range []string{"recovery_case_id", "original_provider_id", "reason_code",
		"late_seconds", "platform_share_ratio", "shipping_mode", "providerId", "distanceKm"} {
		if _, ok := out[kept]; !ok {
			t.Errorf("dropped %s, which a metric needs", kept)
		}
	}
	for _, dropped := range []string{"pickup_lat", "pickup_lng", "dropoff_address", "receiver_phone",
		"customer_email", "provider_name", "fare_amount_kobo", "platform_fee", "delivery_code_hash", "otp",
		"pickupAddress", "receiverPhone", "fareKobo"} {
		if _, ok := out[dropped]; ok {
			t.Errorf("kept %s", dropped)
		}
	}
	nested := out["nested"].(map[string]any)
	if _, ok := nested["lat"]; ok || nested["zone"] != "z1" {
		t.Errorf("nested object not minimized: %v", nested)
	}
	item := out["items"].([]any)[0].(map[string]any)
	if _, ok := item["description"]; ok {
		t.Error("free text survived inside a list")
	}
}

func signalServer(t *testing.T, status int, body any, delay time.Duration) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": status < 300, "data": body})
	}))
	t.Cleanup(server.Close)
	return server
}

func client(base string) Client {
	return Client{BaseURL: base, ServiceName: "vendor-service", Secret: []byte("s"), Timeout: 200 * time.Millisecond}
}

// Analytics being down, slow, broken or absent must look exactly like
// analytics having nothing to say.
func TestEveryFailureIsANeutralAnswer(t *testing.T) {
	failures := map[string]Client{
		"unconfigured": {},
		"unreachable":  client("http://127.0.0.1:1"),
		"server error": client(signalServer(t, 500, nil, 0).URL),
		"too slow":     client(signalServer(t, 200, map[string]any{"available": true, "score": 0.9}, time.Second).URL),
		"garbage":      client(signalServer(t, 200, nil, 0).URL),
	}
	for name, c := range failures {
		readiness := c.BranchReadiness(context.Background(), "branch-1")
		quality := c.ProviderQuality(context.Background(), AuthorityHauling, "p1")
		supply := c.SupplySnapshot(context.Background(), AuthorityDispatch, "zone-1")
		for label, signal := range map[string]Signal{"readiness": readiness.Signal,
			"quality": quality.Signal, "supply": supply.Signal} {
			if signal.Available || signal.Score != NeutralScore || signal.Status != StatusUnavailable {
				t.Errorf("%s/%s: got %+v, want neutral", name, label, signal)
			}
		}
		if readiness.BranchID != "branch-1" || quality.ProviderID != "p1" || supply.Zone != "zone-1" {
			t.Errorf("%s: the neutral answer lost the subject it was asked about", name)
		}
	}
}

func TestAHealthyAnswerIsPassedThrough(t *testing.T) {
	server := signalServer(t, 200, map[string]any{"available": true, "score": 0.82, "sample_size": 40,
		"status": StatusOK, "branch_id": "branch-1", "on_time_rate": 0.82}, 0)
	readiness := client(server.URL).BranchReadiness(context.Background(), "branch-1")
	if !readiness.Available || readiness.Score != 0.82 || readiness.OnTimeRate != 0.82 || readiness.SampleSize != 40 {
		t.Fatalf("got %+v", readiness)
	}
}

func TestInvalidQuestionsAreNotAsked(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	t.Cleanup(server.Close)
	c := client(server.URL)
	c.ProviderQuality(context.Background(), "VENDOR", "p1")
	c.SupplySnapshot(context.Background(), AuthorityTaxi, "")
	c.BranchReadiness(context.Background(), " ")
	if called {
		t.Fatal("an invalid question reached analytics")
	}
}

func TestKeyTokensHandleEveryConvention(t *testing.T) {
	cases := map[string]string{
		"pickupAddress": "pickup address", "receiver_phone": "receiver phone", "ETAMinutes": "eta minutes",
		"trip.providerId": "trip provider id", "distance-km": "distance km", "v2Ready": "v2 ready",
	}
	for key, want := range cases {
		if got := strings.Join(keyTokens(key), " "); got != want {
			t.Errorf("%s -> %q, want %q", key, got, want)
		}
	}
}

func TestNormalizeKeysMakesOneSpelling(t *testing.T) {
	out := NormalizeKeys(map[string]any{"providerId": "p1", "recoveryCaseId": "c1",
		"nested": map[string]any{"branchId": "b1"}, "provider_id": "p1-snake"})
	if out["recovery_case_id"] != "c1" || out["nested"].(map[string]any)["branch_id"] != "b1" {
		t.Fatalf("got %v", out)
	}
	if out["provider_id"] != "p1-snake" {
		t.Fatalf("the snake_case original must win a collision, got %v", out["provider_id"])
	}
}

func TestSafetyDeclinesAreRecognised(t *testing.T) {
	for _, reason := range []string{"UNSAFE_VEHICLE", "vehicle_defect", " SECURITY_CONCERN "} {
		if !IsSafetyDecline(reason) {
			t.Errorf("%q is a safety decline", reason)
		}
	}
	for _, reason := range []string{"SHIFT_ENDING", "EXCESSIVE_DISTANCE", "OTHER", ""} {
		if IsSafetyDecline(reason) {
			t.Errorf("%q is not a safety decline", reason)
		}
	}
	if _, kind, ok := Classify("dispatch.offer.declined.v1", ""); !ok || kind != KindProviderDeclined {
		t.Fatal("a Dispatch decline must classify")
	}
}
