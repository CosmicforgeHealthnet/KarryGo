package orders

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseLogisticsTimingRequiresAuthoritativeWindow(t *testing.T) {
	if !isLogisticsTimingEvent("ETA_UPDATED") || isLogisticsTimingEvent("PRICE_UPDATED") {
		t.Fatal("timing event vocabulary is not fail-closed")
	}
	if _, err := parseLogisticsTiming(json.RawMessage(`{"timing_status":"AVAILABLE"}`), "ETA_UPDATED"); err == nil {
		t.Fatal("available timing without a window accepted")
	}
	start := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	payload, _ := json.Marshal(map[string]any{"timing_status": "APPLIED", "eta_start": start, "eta_end": end})
	parsed, err := parseLogisticsTiming(payload, "ETA_UPDATED")
	if err != nil || parsed.LogisticsWindowStart == nil || parsed.LogisticsWindowEnd == nil || parsed.TimingStatus != "APPLIED" {
		t.Fatalf("timing payload not accepted: %+v, %v", parsed, err)
	}
}

func TestParseLogisticsTimingPreservesHonestUnknown(t *testing.T) {
	parsed, err := parseLogisticsTiming(json.RawMessage(`{"timing_status":"UNKNOWN"}`), "TIMING_UNKNOWN")
	if err != nil || parsed.TimingStatus != "UNKNOWN" || parsed.LogisticsWindowStart != nil || parsed.LogisticsWindowEnd != nil {
		t.Fatalf("unknown timing fabricated an ETA: %+v, %v", parsed, err)
	}
}

func TestMaterialETAChangeSuppressesTinyJitter(t *testing.T) {
	start := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	tinyStart := start.Add(2 * time.Minute)
	tinyEnd := end.Add(2 * time.Minute)
	if materialETAChange("AVAILABLE", "AVAILABLE", &start, &end, &tinyStart, &tinyEnd) {
		t.Fatal("sub-threshold ETA jitter was treated as material")
	}
	largeStart := start.Add(6 * time.Minute)
	if !materialETAChange("AVAILABLE", "AVAILABLE", &start, &end, &largeStart, &end) {
		t.Fatal("material ETA movement was suppressed")
	}
}
