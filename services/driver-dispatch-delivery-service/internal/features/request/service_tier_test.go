package request

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ── normalizeServiceTier ──────────────────────────────────────────────────────

func TestNormalizeServiceTier_Express(t *testing.T) {
	for _, input := range []string{"express", "Express", "EXPRESS", "  express  "} {
		if got := normalizeServiceTier(input); got != "express" {
			t.Errorf("normalizeServiceTier(%q) = %q, want express", input, got)
		}
	}
}

func TestNormalizeServiceTier_Standard(t *testing.T) {
	for _, input := range []string{"standard", "Standard", "STANDARD"} {
		if got := normalizeServiceTier(input); got != "standard" {
			t.Errorf("normalizeServiceTier(%q) = %q, want standard", input, got)
		}
	}
}

func TestNormalizeServiceTier_DefaultsToStandard(t *testing.T) {
	for _, input := range []string{"", "unknown", "premium"} {
		if got := normalizeServiceTier(input); got != "standard" {
			t.Errorf("normalizeServiceTier(%q) = %q, want standard (default)", input, got)
		}
	}
}

func TestServiceTierLabel(t *testing.T) {
	cases := []struct{ input, want string }{
		{"express", "Express Delivery"},
		{"Express", "Express Delivery"},
		{"standard", "Standard Delivery"},
		{"", "Standard Delivery"},
		{"unknown", "Standard Delivery"},
	}
	for _, tc := range cases {
		if got := serviceTierLabel(tc.input); got != tc.want {
			t.Errorf("serviceTierLabel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── NewProviderRequestInboxItem includes service_tier ─────────────────────────

func TestInboxItem_ServiceTierNormalized(t *testing.T) {
	event := BookingDispatchCreatedEvent{
		BookingID: uuid.NewString(), CustomerID: uuid.NewString(),
		FareAmount: 100000, Currency: "NGN", ServiceTier: "EXPRESS",
	}
	payload, _ := json.Marshal(event)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: uuid.NewString(),
		BookingID: event.BookingID, ProviderID: uuid.NewString(),
		Status: InboxStatusPending, ExpiresAt: time.Now().Add(30 * time.Second),
		BookingPayload: payload, ReceivedAt: time.Now(),
	}
	item, active, err := NewProviderRequestInboxItem(inbox, time.Now())
	if err != nil {
		t.Fatalf("NewProviderRequestInboxItem: %v", err)
	}
	if !active {
		t.Fatal("expected active=true")
	}
	if item.ServiceTier != "express" {
		t.Errorf("service_tier = %q, want express", item.ServiceTier)
	}
}

func TestInboxItem_MissingServiceTierDefaultsToStandard(t *testing.T) {
	// Simulate old payload without service_tier field.
	payload := json.RawMessage(`{"booking_id":"` + uuid.NewString() + `","customer_id":"` + uuid.NewString() + `","fare_amount":50000,"currency":"NGN"}`)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: uuid.NewString(),
		BookingID: uuid.NewString(), ProviderID: uuid.NewString(),
		Status: InboxStatusPending, ExpiresAt: time.Now().Add(30 * time.Second),
		BookingPayload: payload, ReceivedAt: time.Now(),
	}
	item, _, err := NewProviderRequestInboxItem(inbox, time.Now())
	if err != nil {
		t.Fatalf("NewProviderRequestInboxItem: %v", err)
	}
	if item.ServiceTier != "standard" {
		t.Errorf("service_tier = %q, want standard (default for missing field)", item.ServiceTier)
	}
}

// ── GET /provider/requests list includes service_tier ─────────────────────────

func TestListInbox_IncludesServiceTier(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	event := fullBookingEvent()
	event.ServiceTier = "express"
	payload, _ := json.Marshal(event)
	now := time.Now().UTC()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusBroadcasting,
		ExpiresAt: now.Add(30 * time.Second), BookingPayload: payload,
	}
	env.repo.broadcasts = append(env.repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID,
		ProviderID: providerID, Status: InboxStatusPending,
		ExpiresAt: broadcast.ExpiresAt, BookingPayload: payload, ReceivedAt: now,
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests", tok, "")
	assertStatus(t, w, http.StatusOK)

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body = %s", err, w.Body.String())
	}
	if len(resp.Data) == 0 {
		t.Fatalf("expected at least one inbox item; body = %s", w.Body.String())
	}
	if tier, _ := resp.Data[0]["service_tier"].(string); tier != "express" {
		t.Errorf("service_tier = %q, want express; body = %s", tier, w.Body.String())
	}
}

// ── GET /provider/requests/:id detail includes tier + label ──────────────────

func TestDetail_ServiceTierAndLabel(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	event := fullBookingEvent()
	event.ServiceTier = "express"
	payload, _ := json.Marshal(event)
	now := time.Now().UTC()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusBroadcasting,
		ExpiresAt: now.Add(30 * time.Second), BookingPayload: payload,
	}
	env.repo.broadcasts = append(env.repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID,
		ProviderID: providerID, Status: InboxStatusPending,
		ExpiresAt: broadcast.ExpiresAt, BookingPayload: payload, ReceivedAt: now,
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests/"+inbox.ID, tok, "")
	assertStatus(t, w, http.StatusOK)
	data := extractData(t, w)

	if tier, _ := data["service_tier"].(string); tier != "express" {
		t.Errorf("service_tier = %q, want express", tier)
	}
	if label, _ := data["service_tier_label"].(string); label != "Express Delivery" {
		t.Errorf("service_tier_label = %q, want Express Delivery", label)
	}
}

func TestDetail_MissingTierDefaultsToStandard(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	// Old payload without service_tier.
	rawPayload := `{"booking_id":"` + uuid.NewString() + `","customer_id":"` + uuid.NewString() + `",` +
		`"pickup_lat":6.4,"pickup_lng":3.4,"dropoff_lat":6.5,"dropoff_lng":3.3,` +
		`"pickup_address":"A","dropoff_address":"B","fare_amount":100000,"currency":"NGN",` +
		`"receiver_name":"X","receiver_phone":"+2348011223344"}`
	var event BookingDispatchCreatedEvent
	_ = json.Unmarshal([]byte(rawPayload), &event)
	payload := json.RawMessage(rawPayload)
	now := time.Now().UTC()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusBroadcasting,
		ExpiresAt: now.Add(30 * time.Second), BookingPayload: payload,
	}
	env.repo.broadcasts = append(env.repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID,
		ProviderID: providerID, Status: InboxStatusPending,
		ExpiresAt: broadcast.ExpiresAt, BookingPayload: payload, ReceivedAt: now,
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests/"+inbox.ID, tok, "")
	assertStatus(t, w, http.StatusOK)
	data := extractData(t, w)

	if tier, _ := data["service_tier"].(string); tier != "standard" {
		t.Errorf("service_tier = %q, want standard (default)", tier)
	}
	if label, _ := data["service_tier_label"].(string); label != "Standard Delivery" {
		t.Errorf("service_tier_label = %q, want Standard Delivery", label)
	}
}

// ── Accept event includes service_tier ────────────────────────────────────────

func TestAccept_PublishesServiceTier(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	event := fullBookingEvent()
	event.ServiceTier = "express"
	payload, _ := json.Marshal(event)
	now := time.Now().UTC()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusBroadcasting,
		ExpiresAt: now.Add(30 * time.Second), BookingPayload: payload,
	}
	env.repo.broadcasts = append(env.repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID,
		ProviderID: providerID, Status: InboxStatusPending,
		ExpiresAt: broadcast.ExpiresAt, BookingPayload: payload, ReceivedAt: now,
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox)
	_ = env.redis.Set(context.Background(), RequestBroadcastingKey(event.BookingID), broadcast.ID, 35*time.Second)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusOK)

	if len(env.events.accepted) != 1 {
		t.Fatalf("accepted events = %d, want 1", len(env.events.accepted))
	}
	if got := env.events.accepted[0].ServiceTier; got != "express" {
		t.Errorf("request.accepted event service_tier = %q, want express", got)
	}
}
