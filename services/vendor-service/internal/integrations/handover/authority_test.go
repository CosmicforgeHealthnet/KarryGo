package handover

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

type canonicalMock struct {
	mu           sync.Mutex
	state        Observation
	operations   map[string]bool
	transitions  int
	loseResponse bool
}

func (m *canonicalMock) Read(context.Context, Binding, string, string) (Observation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, nil
}
func (m *canonicalMock) ConfirmPickup(_ context.Context, c Confirmation) (Observation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c.Credential != "test-presented-credential" {
		return Observation{}, ErrRejected
	}
	if !m.operations[c.OperationRef] {
		if m.state.Status == "CONSUMED" {
			return Observation{}, ErrRejected
		}
		m.operations[c.OperationRef] = true
		m.transitions++
		m.state.Status = "CONSUMED"
		m.state.SourceVersion++
	}
	if m.loseResponse {
		return Observation{}, ErrUnknown
	}
	return m.state, nil
}
func fixture() (*canonicalMock, Confirmation) {
	b := Binding{Authority: "TAXI", MissionRef: "physical-mission-a", FulfilmentRef: "fulfilment-a"}
	m := &canonicalMock{state: Observation{Binding: b, HandoverRef: "opaque-a", Stage: "PICKUP", Status: "ACTIVE", SourceVersion: 1}, operations: map[string]bool{}}
	return m, Confirmation{Binding: b, HandoverRef: "opaque-a", Method: "OTP", Credential: "test-presented-credential", OperationRef: "operation-a", CorrelationID: "request-a"}
}
func TestConcurrentIdenticalRelayUsesCanonicalIdempotency(t *testing.T) {
	m, c := fixture()
	r := Router{"TAXI": m}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; _, err := r.ConfirmPickup(context.Background(), c); results <- err }()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if m.transitions != 1 {
		t.Fatal(m.transitions)
	}
}
func TestUnknownOutcomeReadsBackWithoutSecondMutation(t *testing.T) {
	m, c := fixture()
	m.loseResponse = true
	o, err := (Router{"TAXI": m}).ConfirmPickup(context.Background(), c)
	if err != nil || o.Status != "CONSUMED" || m.transitions != 1 {
		t.Fatalf("%+v %v transitions=%d", o, err, m.transitions)
	}
}
func TestWrongMissionObservationRejected(t *testing.T) {
	m, c := fixture()
	m.state.MissionRef = "physical-mission-b"
	if _, err := (Router{"TAXI": m}).Read(context.Background(), c.Binding, "PICKUP", "request-a"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
func TestMissingAuthorityFailsClosed(t *testing.T) {
	_, c := fixture()
	c.Authority = "VENDOR_OWN_DELIVERY"
	if _, err := (Router{}).ConfirmPickup(context.Background(), c); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestCredentialNeverSerializesInVendorProjectionOrCommand(t *testing.T) {
	m, c := fixture()
	for _, v := range []any{c, m.state} {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), c.Credential) {
			t.Fatal("credential escaped")
		}
		for _, forbidden := range []string{"otp_hash", "qr_hash", "qr_token", "challenge_secret", "attempt_count", "expires_at"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatal(forbidden)
			}
		}
	}
}
func TestBadCredentialNeverAdvances(t *testing.T) {
	m, c := fixture()
	c.Credential = "invalid"
	if _, err := (Router{"TAXI": m}).ConfirmPickup(context.Background(), c); !errors.Is(err, ErrRejected) {
		t.Fatal(err)
	}
	if m.transitions != 0 || m.state.Status != "ACTIVE" {
		t.Fatal("invalid credential advanced custody")
	}
}
