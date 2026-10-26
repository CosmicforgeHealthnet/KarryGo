package adminevents

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func supportEvent(t *testing.T, extra map[string]any) SourceEvent {
	now := time.Now().UTC()
	payload := map[string]any{"status": "OPEN", "priority": "EMERGENCY", "service_type": "taxi", "category": "emergency", "complainant_type": "customer", "context_type": nil, "locale": nil, "sla_state": "POLICY_UNAVAILABLE", "consent_classification": "SAFETY_EVIDENCE", "assignee": nil, "escalation_level": nil, "mission_ref": nil, "order_ref": nil, "dispute_ref": nil, "unread_count": 1, "message_count": 2, "evidence_count": 0, "created_at": now, "updated_at": now}
	for k, v := range extra {
		payload[k] = v
	}
	ev, err := NewSourceEvent("support-dispute-service", SupportCaseChanged, "support_case", "6f1c1f8e-1b1e-4b7e-9f53-2b0d2a1b3c4d", 3, now, Scope{Organization: "org-1"}, "", payload, false)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestRoundTripAndMinimize(t *testing.T) {
	ev := supportEvent(t, nil)
	raw, _ := json.Marshal(ev)
	back, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := Lookup(back.SourceService, back.EventType)
	if !ok {
		t.Fatal("adapter missing")
	}
	out, err := a.Minimize(back)
	if err != nil || out["priority"] != "EMERGENCY" || out["unread_count"] != int64(1) {
		t.Fatalf("minimize %v %v", out, err)
	}
}

func TestUnknownFieldAndFreeTextQuarantine(t *testing.T) {
	for _, extra := range []map[string]any{{"subject": "my phone is 0803"}, {"complainant_phone": "+2348030000000"}} {
		ev := supportEvent(t, extra)
		a, _ := Lookup(ev.SourceService, ev.EventType)
		if _, err := a.Minimize(ev); err != ErrUnknown {
			t.Fatalf("expected quarantine, got %v", err)
		}
	}
	ev := supportEvent(t, map[string]any{"assignee": "agent with spaces"})
	a, _ := Lookup(ev.SourceService, ev.EventType)
	if _, err := a.Minimize(ev); err != ErrInvalid {
		t.Fatalf("free text accepted: %v", err)
	}
}

func TestStrictDecodeRejectsDuplicatesUnknownAndOversize(t *testing.T) {
	ev := supportEvent(t, nil)
	raw, _ := json.Marshal(ev)
	dup := strings.Replace(string(raw), `"product":"karryam"`, `"product":"karryam","product":"healthnet"`, 1)
	if _, err := Decode([]byte(dup)); err == nil {
		t.Fatal("duplicate key accepted")
	}
	other := strings.Replace(string(raw), `"product":"karryam"`, `"product":"healthnet"`, 1)
	if _, err := Decode([]byte(other)); err == nil {
		t.Fatal("foreign product accepted")
	}
	if _, err := Decode(append(raw, ' ', '{', '}')); err == nil {
		t.Fatal("trailing data accepted")
	}
	if _, err := Decode(make([]byte, MaxEventBytes+1)); err == nil {
		t.Fatal("oversize accepted")
	}
}

func TestSourceBindingMismatch(t *testing.T) {
	ev := supportEvent(t, nil)
	ev.SourceService = "notification-service"
	a, _ := Lookup("support-dispute-service", SupportCaseChanged)
	if _, err := a.Minimize(ev); err != ErrMismatch {
		t.Fatalf("expected mismatch, got %v", err)
	}
}

func TestRegistryNeverExportsRestrictedClasses(t *testing.T) {
	for _, a := range Adapters() {
		if !Exportable(a.Classification) {
			t.Fatalf("%s exports %s", a.SourceType, a.Classification)
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("restricted adapter registered")
		}
	}()
	Register(Adapter{SourceService: "x", SourceType: "x.y.z.v1", ExportType: "a.b.c.v1", EntityType: "e", Classification: ClassRestrictedPII})
}
