package hqevents

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestSnapshotClientSignsFixedPathAndAcceptsEnvelopeOrRawPage(t *testing.T) {
	secret := bytes.Repeat([]byte{8}, 32)
	page := adminevents.SnapshotPage{Contract: adminevents.SnapshotContract, Source: "notification-service", EntityType: "campaign_revision", Watermark: time.Now().UTC(), Events: []adminevents.SourceEvent{}, Complete: true}
	for _, mode := range []string{"raw", "envelope", "foreign", "unsigned_path"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name, err := serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": secret}, time.Minute).VerifyRequest(r)
			if err != nil || name != "admin-backoffice-service" || r.URL.Path != "/internal/notification/v1/admin-events/snapshot" {
				w.WriteHeader(401)
				return
			}
			switch mode {
			case "raw":
				json.NewEncoder(w).Encode(page)
			case "envelope":
				json.NewEncoder(w).Encode(map[string]any{"success": true, "data": page})
			case "foreign":
				p := page
				p.Source = "support-dispute-service"
				json.NewEncoder(w).Encode(p)
			}
		}))
		c, err := NewSnapshotClient("notification-service", srv.URL, secret)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Page(context.Background(), "")
		if (mode == "raw" || mode == "envelope") != (err == nil) {
			t.Fatalf("%s: %v", mode, err)
		}
		srv.Close()
	}
	if _, err := NewSnapshotClient("notification-service", "http://host/evil-path", secret); err == nil {
		t.Fatal("origin with path accepted")
	}
	if _, err := NewSnapshotClient("unknown-service", "http://host", secret); err == nil {
		t.Fatal("unknown owner accepted")
	}
}
