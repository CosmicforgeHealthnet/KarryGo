package models

import "testing"
import "time"

func TestValidExportType(t *testing.T) {
	for _, good := range []string{TypeOrders, TypeShipments, TypeProvidersWorkforce,
		TypeAssetsMaintenance, TypeWarehouseInventory, TypeComplianceIncidents,
		TypeInvoicesReceivables, TypeFulfilmentOrders, TypeSupportCases} {
		if !ValidExportType(good) {
			t.Errorf("%s must be a recognized export type", good)
		}
	}
	for _, bad := range []string{"", "orders", "shipments", "UNKNOWN"} {
		if ValidExportType(bad) {
			t.Errorf("expected %q to be unrecognized", bad)
		}
	}
}

func TestValidFormat(t *testing.T) {
	if !ValidFormat(FormatCSV) {
		t.Fatal("CSV must be a recognized format")
	}
	for _, bad := range []string{"", "csv", "XLSX", "JSON"} {
		if ValidFormat(bad) {
			t.Errorf("expected %q to be unrecognized", bad)
		}
	}
}

func TestJobTerminal(t *testing.T) {
	nonTerminal := []string{StatusPending, StatusRunning}
	for _, status := range nonTerminal {
		if (Job{Status: status}).Terminal() {
			t.Errorf("status %s must not be terminal", status)
		}
	}
	terminal := []string{StatusReady, StatusFailed, StatusExpired, StatusCancelled, StatusDeadLetter}
	for _, status := range terminal {
		if !(Job{Status: status}).Terminal() {
			t.Errorf("status %s must be terminal", status)
		}
	}
}

func TestJobDownloadable(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	cases := []struct {
		name string
		job  Job
		want bool
	}{
		{"ready with media and no expiry", Job{Status: StatusReady, MediaFileID: "mobj_1"}, true},
		{"ready but not yet expired", Job{Status: StatusReady, MediaFileID: "mobj_1", ExpiresAt: &future}, true},
		{"ready but expired", Job{Status: StatusReady, MediaFileID: "mobj_1", ExpiresAt: &past}, false},
		{"ready but no media file yet", Job{Status: StatusReady}, false},
		{"pending", Job{Status: StatusPending, MediaFileID: "mobj_1"}, false},
		{"failed", Job{Status: StatusFailed, MediaFileID: "mobj_1"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.job.Downloadable(now); got != c.want {
				t.Errorf("Downloadable() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestJSONMapClone(t *testing.T) {
	var nilMap JSONMap
	if clone := nilMap.Clone(); clone == nil || len(clone) != 0 {
		t.Fatalf("clone of nil map = %v, want empty non-nil map", clone)
	}

	original := JSONMap{"status": "ACTIVE"}
	clone := original.Clone()
	clone["status"] = "MUTATED"
	if original["status"] != "ACTIVE" {
		t.Fatal("mutating a clone must not affect the original")
	}
}
