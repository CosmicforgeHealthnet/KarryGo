package fulfilmentgroup

import (
	"context"
	"testing"
	"time"

	vendorevents "cosmicforge/logistics/services/vendor-service/internal/events"
)

// EVT-002 / EVT-01: a domain mutation and the durable event that announces it
// commit together or not at all.
//
// This is not something a reviewer can confirm by reading, because the two
// writes are in different files and the shared transaction is an argument
// threaded through several calls. If someone later opens a second transaction
// for the publish — an easy, well-meaning change — every test still passes
// while the system starts losing events on any failure between the two
// commits. So the property is proved directly: write both inside one
// transaction, see them, roll it back, and see that neither survived.
func TestCustodyMutationAndItsEventShareOneTransaction(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, vendorevents.NewDomainPublisher(vendorevents.NewOutboxRepository(pool)))
	parcel, missionRef := custodySeed(t, store)
	ctx := context.Background()
	now := time.Now().UTC()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	event := CustodyEvent{
		ParcelID: parcel.ID, GroupID: parcel.GroupID, EventType: CustodyHandedOver,
		ToCustodianType: CustodianPlatform, Source: SourceDispatch,
		SourceEventID: "evt-atomicity", SourceVersion: 1,
		PlatformMissionRefID: missionRef, OccurredAt: now,
	}
	if err = recordCustodyTx(ctx, tx, event, now); err != nil {
		t.Fatalf("record custody: %v", err)
	}
	if err = store.publish(ctx, tx, EventCustodyRecorded, parcel.ID, 1,
		parcel.BusinessID, map[string]string{"parcel_id": parcel.ID}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Inside the transaction both are present, which is what makes the
	// rollback below meaningful rather than vacuous.
	var custodyInTx, outboxInTx int
	if err = tx.QueryRow(ctx,
		`SELECT count(*) FROM commerce_parcel_custody_events WHERE source_event_id=$1`,
		"evt-atomicity").Scan(&custodyInTx); err != nil {
		t.Fatalf("count custody in tx: %v", err)
	}
	// Filtered by event type: packing the parcel already published its own
	// creation event, and counting that one would make the assertion pass
	// for the wrong reason.
	if err = tx.QueryRow(ctx,
		`SELECT count(*) FROM vendor_outbox WHERE aggregate_id=$1 AND event_type=$2`,
		parcel.ID, EventCustodyRecorded).Scan(&outboxInTx); err != nil {
		t.Fatalf("count outbox in tx: %v", err)
	}
	if custodyInTx != 1 || outboxInTx != 1 {
		t.Fatalf("inside the transaction: custody=%d outbox=%d; want 1 and 1",
			custodyInTx, outboxInTx)
	}

	if err = tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var custodyAfter, outboxAfter int
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_parcel_custody_events WHERE source_event_id=$1`,
		"evt-atomicity").Scan(&custodyAfter); err != nil {
		t.Fatalf("count custody after rollback: %v", err)
	}
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_outbox WHERE aggregate_id=$1 AND event_type=$2`,
		parcel.ID, EventCustodyRecorded).Scan(&outboxAfter); err != nil {
		t.Fatalf("count outbox after rollback: %v", err)
	}
	if custodyAfter != 0 {
		t.Fatalf("a custody transition survived a rolled-back transaction (%d rows)", custodyAfter)
	}
	if outboxAfter != 0 {
		t.Fatalf("an event survived a rolled-back transaction (%d rows): a consumer "+
			"would be told about a custody change that never happened", outboxAfter)
	}
}

// EVT-001: the durable envelope carries the identity a consumer needs to
// deduplicate and order events. A payload with no event id or no aggregate
// version cannot be processed idempotently by anyone.
func TestDurableEventCarriesItsIdentityAndVersion(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, vendorevents.NewDomainPublisher(vendorevents.NewOutboxRepository(pool)))
	parcel, _ := custodySeed(t, store)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err = store.publish(ctx, tx, EventCustodyRecorded, parcel.ID, 7,
		parcel.BusinessID, map[string]string{"parcel_id": parcel.ID}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	var eventID, eventType, aggregateType, aggregateID, correlationID, status string
	var aggregateVersion, schemaVersion int64
	if err = tx.QueryRow(ctx, `
		SELECT id, event_type, schema_version, aggregate_type, aggregate_id,
		       aggregate_version, correlation_id, status
		  FROM vendor_outbox WHERE aggregate_id=$1 AND event_type=$2`,
		parcel.ID, EventCustodyRecorded,
	).Scan(&eventID, &eventType, &schemaVersion, &aggregateType, &aggregateID,
		&aggregateVersion, &correlationID, &status); err != nil {
		t.Fatalf("read envelope: %v", err)
	}

	switch {
	case eventID == "":
		t.Fatal("the event has no id; a consumer cannot deduplicate it")
	case eventType == "" || aggregateType == "" || aggregateID != parcel.ID:
		t.Fatalf("the event does not identify its aggregate: type=%q aggregate=%s/%s",
			eventType, aggregateType, aggregateID)
	case aggregateVersion != 7:
		t.Fatalf("aggregateVersion = %d; a consumer cannot detect a stale event without it",
			aggregateVersion)
	case schemaVersion < 1:
		t.Fatalf("schemaVersion = %d; a consumer cannot tell which shape it received", schemaVersion)
	case correlationID == "":
		t.Fatal("the event has no correlation id; one order cannot be traced across services")
	case status != vendorevents.OutboxStatusPending:
		t.Fatalf("status = %q; a newly written event must be waiting for delivery", status)
	}
}
