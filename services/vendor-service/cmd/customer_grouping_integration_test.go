package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	shared "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCustomerGroupingDurableVersionsAndHTTPShape(t *testing.T) {
	fixture := newSection17EFixture(t, false)
	ctx := context.Background()
	parent, _, err := fixture.service.CreateFromQuote(ctx, fixture.customerID, fixture.quoteID, fixture.finger)
	if err != nil {
		t.Fatal(err)
	}
	publisher := &customerGroupingPublisher{db: fixture.env.pool, outbox: events.NewOutboxRepository(fixture.env.pool)}
	grouping := shared.MovementGrouping{CustomerID: fixture.customerID, ProductCode: parent.Domain, ParentContextType: "COMMERCE_ORDER", ParentContextRef: parent.ID, Missions: []shared.GroupingMission{{FulfilmentGroupRef: parent.ID + "-mission", MissionSequence: 1, MissionCount: 1}}}
	if err = publisher.PublishGrouping(ctx, parent.ID, grouping); err != nil {
		t.Fatal(err)
	}
	if err = publisher.PublishGrouping(ctx, parent.ID, grouping); err != nil {
		t.Fatal(err)
	}
	var count int
	var first int64
	if err = fixture.env.pool.QueryRow(ctx, `SELECT count(*),max(aggregate_version) FROM vendor_outbox WHERE event_type=$1 AND aggregate_id=$2`, shared.CommerceGroupingEventName, parent.ID).Scan(&count, &first); err != nil || count != 1 {
		t.Fatal("idempotent publication", count, err)
	}
	grouping.Missions[0].MissionCount = 2
	grouping.Missions = append(grouping.Missions, shared.GroupingMission{FulfilmentGroupRef: parent.ID + "-second", MissionSequence: 2, MissionCount: 2})
	if err = publisher.PublishGrouping(ctx, parent.ID, grouping); err != nil {
		t.Fatal(err)
	}
	var version int64
	var record events.OutboxRecord
	if err = fixture.env.pool.QueryRow(ctx, `SELECT id,payload,aggregate_version FROM vendor_outbox WHERE event_type=$1 AND aggregate_id=$2 ORDER BY aggregate_version DESC LIMIT 1`, shared.CommerceGroupingEventName, parent.ID).Scan(&record.ID, &record.Payload, &version); err != nil || version != first+1 {
		t.Fatal("non-monotonic grouping", version, first, err)
	}
	secret := []byte("grouping-http-test")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": secret}, 5*time.Minute)
	delivered := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, e := verifier.VerifyRequest(r); e != nil {
			t.Error(e)
			w.WriteHeader(401)
			return
		}
		if r.Method == "GET" {
			versions := map[string]int64{}
			for _, ref := range strings.Split(r.URL.Query().Get("source_refs"), ",") {
				versions[ref] = int64(1 << 62)
				if ref == parent.ID {
					versions[ref] = 0
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"versions": versions}})
			return
		}
		var envelope shared.Envelope
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if e := decoder.Decode(&envelope); e != nil || envelope.Validate() != nil || envelope.EventName != shared.CommerceGroupingEventName || envelope.SourceVersion != version {
			t.Error("wrong Customer envelope", e, envelope.EventName)
			w.WriteHeader(400)
			return
		}
		delivered = true
		w.WriteHeader(200)
	}))
	defer server.Close()
	inner, err := events.NewHTTPPublisher(server.URL, shared.CustomerMovementGroupingPath, "vendor-service", secret, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err = (customerGroupingHTTPPublisher{inner: inner}).Publish(ctx, record); err != nil || !delivered {
		t.Fatal("grouping HTTP", delivered, err)
	}
	if _, err = fixture.env.pool.Exec(ctx, `UPDATE vendor_outbox SET status='PUBLISHED' WHERE event_type=$1 AND aggregate_id=$2`, shared.CommerceGroupingEventName, parent.ID); err != nil {
		t.Fatal(err)
	}
	repair := &customerGroupingReconciler{publisher: publisher, baseURL: server.URL, secret: secret}
	if _, err = repair.Sweep(ctx); err != nil {
		t.Fatal("repair", err)
	}
	if err = fixture.env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE event_type=$1 AND aggregate_id=$2 AND aggregate_version=$3 AND status='PENDING'`, shared.CommerceGroupingEventName, parent.ID, version).Scan(&count); err != nil || count != 1 {
		t.Fatal("repair must preserve version", count, err)
	}
	repair.after = ""
	if _, err = repair.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if err = fixture.env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE event_type=$1 AND aggregate_id=$2 AND aggregate_version=$3 AND status='PENDING'`, shared.CommerceGroupingEventName, parent.ID, version).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate repair", count, err)
	}
}
