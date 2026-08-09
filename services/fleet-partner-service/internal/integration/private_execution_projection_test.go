package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	marketusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	ordersmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	shipmentsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
)

func insertPrivateExecutionFixture(t *testing.T, f *shipmentsFixture, orderID, shipmentID,
	assignmentID, operationID string) {
	t.Helper()
	var branchID string
	err := f.pool.QueryRow(context.Background(), `SELECT id FROM fleet_partner_branches
		WHERE organization_id=$1 ORDER BY created_at LIMIT 1`, f.tenant.OrganizationID).Scan(&branchID)
	if err != nil {
		branchID = "brn_" + assignmentID
		if _, err = f.pool.Exec(context.Background(), `INSERT INTO fleet_partner_branches(
			id,organization_id,code,name,timezone) VALUES($1,$2,$3,'Projection Branch','Africa/Lagos')`,
			branchID, f.tenant.OrganizationID, "PROJECTION-"+assignmentID); err != nil {
			t.Fatalf("branch: %v", err)
		}
	}
	_, err = f.pool.Exec(context.Background(), `INSERT INTO fleet_partner_private_assignments(
		id,organization_id,branch_id,work_origin,shipment_id,provider_membership_id,provider_id,
		vehicle_or_truck_reference,execution_service,status,response_deadline,created_by)
		VALUES($1,$2,$3,'PRIVATE_SHIPMENT',$4,'mem_private','provider_private','vehicle_private',
		'DISPATCH','ACCEPTED',now()+interval '1 hour','test')`, assignmentID,
		f.tenant.OrganizationID, branchID, shipmentID)
	if err != nil {
		t.Fatalf("insert assignment: %v", err)
	}
	_, err = f.pool.Exec(context.Background(), `INSERT INTO fleet_partner_private_execution_operations(
		id,organization_id,settlement_organization_id,executing_provider_id,
		organization_membership_id,private_order_id,shipment_id,assignment_id,execution_service,
		status,idempotency_key,request_fingerprint,correlation_id)
		VALUES($1,$2,$2,'provider_private','mem_private',$3,$4,$5,'DISPATCH','CONFIRMED',$6,$7,$1)`,
		operationID, f.tenant.OrganizationID, orderID, shipmentID, assignmentID,
		"key-"+operationID, "fingerprint-"+operationID)
	if err != nil {
		t.Fatalf("insert execution: %v", err)
	}
}

func TestPrivateExecutionEvents_LowerVersionConflictAndMissingVersion(t *testing.T) {
	f := newShipmentsFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Private Version Co")
	item := f.activeCatalogItem(t, "PRIVATE_VERSION")
	order, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	shipment, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID,
		DeclaredValueCurrency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	insertPrivateExecutionFixture(t, f, order.ID, shipment.ID, "pas_versions", "pex_versions")
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(f.pool))
	service := marketusecases.NewService(marketusecases.Options{Pool: f.pool,
		PrivateEvents: app.NewPrivateExecutionEventProjector(f.pool, publisher)})

	v1 := privateLifecycleEvent("pex_versions", "trip_versions", "evt_versions_1",
		marketusecases.EventTripCreated, "IN_PROGRESS", 1)
	if _, err = service.IngestEvent(ctx, nil, v1); err != nil {
		t.Fatal(err)
	}
	v2 := privateLifecycleEvent("pex_versions", "trip_versions", "evt_versions_2",
		marketusecases.EventTripStatusChanged, "IN_PROGRESS", 2)
	if _, err = service.IngestEvent(ctx, nil, v2); err != nil {
		t.Fatal(err)
	}
	lower := privateLifecycleEvent("pex_versions", "trip_versions", "evt_versions_lower",
		marketusecases.EventTripCreated, "IN_PROGRESS", 1)
	if result, ingestErr := service.IngestEvent(ctx, nil, lower); ingestErr != nil || result.Status != marketusecases.IngestIgnored {
		t.Fatalf("lower version result=%+v err=%v", result, ingestErr)
	}
	var version int64
	var freshness string
	if err = f.pool.QueryRow(ctx, `SELECT source_version,freshness_state FROM fleet_partner_private_execution_projections
		WHERE execution_operation_id='pex_versions'`).Scan(&version, &freshness); err != nil || version != 2 || freshness != "CURRENT" {
		t.Fatalf("lower version regressed projection: version=%d freshness=%s err=%v", version, freshness, err)
	}

	conflict := privateLifecycleEvent("pex_versions", "trip_versions", "evt_versions_conflict",
		marketusecases.EventTripCompleted, "COMPLETED", 2)
	if result, ingestErr := service.IngestEvent(ctx, nil, conflict); ingestErr != nil || result.Status != marketusecases.IngestAccepted {
		t.Fatalf("conflict result=%+v err=%v", result, ingestErr)
	}
	var conflictCases int
	if err = f.pool.QueryRow(ctx, `SELECT freshness_state FROM fleet_partner_private_execution_projections
		WHERE execution_operation_id='pex_versions'`).Scan(&freshness); err != nil || freshness != "CONFLICT" {
		t.Fatalf("freshness=%s err=%v", freshness, err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_private_execution_reconciliation_cases
		WHERE execution_operation_id='pex_versions' AND reason_code='SOURCE_VERSION_CONFLICT'`).Scan(&conflictCases); err != nil || conflictCases != 1 {
		t.Fatalf("conflict cases=%d err=%v", conflictCases, err)
	}

	missing := privateLifecycleEvent("pex_versions", "trip_versions", "evt_versions_missing",
		marketusecases.EventTripStatusChanged, "IN_PROGRESS", 0)
	if _, ingestErr := service.IngestEvent(ctx, nil, missing); ingestErr == nil {
		t.Fatal("missing source version was accepted")
	}
}

func TestPrivateExecutionReconciliation_RepairsGapAndDerivedOutcome(t *testing.T) {
	f, privateService, _, assignmentID := privateWorkerFixture(t, "pex_repair")
	ctx := context.Background()
	var orderID, shipmentID string
	if err := f.pool.QueryRow(ctx, `SELECT private_order_id,shipment_id FROM fleet_partner_private_execution_operations
		WHERE id='pex_repair'`).Scan(&orderID, &shipmentID); err != nil {
		t.Fatal(err)
	}
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(f.pool))
	projector := app.NewPrivateExecutionEventProjector(f.pool, publisher)
	ingestion := marketusecases.NewService(marketusecases.Options{Pool: f.pool, PrivateEvents: projector})
	v1 := privateLifecycleEvent("pex_repair", "trip_repair", "evt_repair_1",
		marketusecases.EventTripCreated, "IN_PROGRESS", 1)
	if _, err := ingestion.IngestEvent(ctx, nil, v1); err != nil {
		t.Fatal(err)
	}
	v3 := privateLifecycleEvent("pex_repair", "trip_repair", "evt_repair_3",
		marketusecases.EventTripCompleted, "COMPLETED", 3)
	if _, err := ingestion.IngestEvent(ctx, nil, v3); err != nil {
		t.Fatal(err)
	}
	port := &executionPortStub{result: privateusecases.ExecutionOutcome{Definite: true, Confirmed: true,
		CanonicalRequestReference: "request-repair", CanonicalTripReference: "trip-repair",
		NormalizedStatus: "COMPLETED", SourceVersion: 3}}
	result, err := privateService.RunProjectionReconciliationPass(ctx, port, projector,
		privateusecases.ReconciliationWorkerConfig{Owner: "repair-worker", Lease: 3 * time.Second,
			BatchSize: 10, MaxAttempts: 3})
	if err != nil || result.Resolved != 1 || port.calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, port.calls, err)
	}
	var freshness, caseStatus string
	var sourceVersion int64
	if err = f.pool.QueryRow(ctx, `SELECT freshness_state,source_version FROM fleet_partner_private_execution_projections
		WHERE execution_operation_id='pex_repair'`).Scan(&freshness, &sourceVersion); err != nil || freshness != "CURRENT" || sourceVersion != 3 {
		t.Fatalf("projection freshness=%s version=%d err=%v", freshness, sourceVersion, err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_private_execution_reconciliation_cases
		WHERE execution_operation_id='pex_repair'`).Scan(&caseStatus); err != nil || caseStatus != "RESOLVED" {
		t.Fatalf("case status=%s err=%v", caseStatus, err)
	}
	shipment, err := f.shipments.Get(ctx, f.tenant, shipmentID)
	if err != nil || shipment.Status != shipmentsmodels.ShipmentCompleted {
		t.Fatalf("shipment status=%s err=%v", shipment.Status, err)
	}
	projectedOrder, err := f.orders.Get(ctx, f.tenant, orderID)
	if err != nil || projectedOrder.Status != ordersmodels.StatusCompleted {
		t.Fatalf("order status=%s err=%v", projectedOrder.Status, err)
	}
	var assignmentStatus string
	if err = f.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_private_assignments WHERE id=$1`, assignmentID).Scan(&assignmentStatus); err != nil || assignmentStatus != "COMPLETED" {
		t.Fatalf("assignment status=%s err=%v", assignmentStatus, err)
	}
}

func privateLifecycleEvent(operationID, aggregateID, eventID, eventType, status string,
	version int64) marketusecases.InboundEvent {
	payload := map[string]any{"fleetExecutionOperationId": operationID, "tripId": aggregateID,
		"requestId": "request-" + operationID, "status": status}
	raw, _ := json.Marshal(payload)
	return marketusecases.InboundEvent{EventID: eventID, EventType: eventType, SchemaVersion: 1,
		SourceService: "driver-dispatch-delivery-service", AggregateType: "trip",
		AggregateID: aggregateID, AggregateVersion: version, OccurredAt: time.Now().UTC(),
		CorrelationID: operationID, PayloadHash: events.HashPayload(raw), Payload: payload}
}

func TestPrivateExecutionEvents_DeduplicateGapAndAggregateAllShipments(t *testing.T) {
	f := newShipmentsFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Private Projection Co")
	item := f.activeCatalogItem(t, "PRIVATE_PROJECTION")
	order, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	shipment1, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID,
		DeclaredValueCurrency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	shipment2, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID,
		DeclaredValueCurrency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	insertPrivateExecutionFixture(t, f, order.ID, shipment1.ID, "pas_projection_1", "pex_projection_1")
	insertPrivateExecutionFixture(t, f, order.ID, shipment2.ID, "pas_projection_2", "pex_projection_2")

	publisher := events.NewDomainPublisher(events.NewOutboxRepository(f.pool))
	service := marketusecases.NewService(marketusecases.Options{Pool: f.pool,
		PrivateEvents: app.NewPrivateExecutionEventProjector(f.pool, publisher)})

	created1 := privateLifecycleEvent("pex_projection_1", "trip_projection_1", "evt_projection_1",
		marketusecases.EventTripCreated, "IN_PROGRESS", 1)
	if result, err := service.IngestEvent(ctx, nil, created1); err != nil || result.Status != marketusecases.IngestAccepted {
		t.Fatalf("first event: result=%+v err=%v", result, err)
	}
	if result, err := service.IngestEvent(ctx, nil, created1); err != nil || result.Status != marketusecases.IngestDuplicate {
		t.Fatalf("duplicate: result=%+v err=%v", result, err)
	}
	gap := privateLifecycleEvent("pex_projection_1", "trip_projection_1", "evt_projection_gap",
		marketusecases.EventTripCompleted, "COMPLETED", 3)
	if result, err := service.IngestEvent(ctx, nil, gap); err != nil || result.Status != marketusecases.IngestAccepted {
		t.Fatalf("gap event: result=%+v err=%v", result, err)
	}
	var freshness string
	if err := f.pool.QueryRow(ctx, `SELECT freshness_state FROM fleet_partner_private_execution_projections
		WHERE execution_operation_id='pex_projection_1'`).Scan(&freshness); err != nil || freshness != "GAP_DETECTED" {
		t.Fatalf("freshness=%q err=%v", freshness, err)
	}
	current1, err := f.shipments.Get(ctx, f.tenant, shipment1.ID)
	if err != nil || current1.Status != shipmentsmodels.ShipmentInTransit {
		t.Fatalf("gap advanced shipment: status=%s err=%v", current1.Status, err)
	}

	for _, event := range []marketusecases.InboundEvent{
		privateLifecycleEvent("pex_projection_2", "trip_projection_2", "evt_projection_2a", marketusecases.EventTripCreated, "IN_PROGRESS", 1),
		privateLifecycleEvent("pex_projection_2", "trip_projection_2", "evt_projection_2b", marketusecases.EventTripCompleted, "COMPLETED", 2),
	} {
		if _, err := service.IngestEvent(ctx, nil, event); err != nil {
			t.Fatalf("shipment 2 event: %v", err)
		}
	}
	projectedOrder, err := f.orders.Get(ctx, f.tenant, order.ID)
	if err != nil || projectedOrder.Status != ordersmodels.StatusInProgress {
		t.Fatalf("order falsely terminal while shipment 1 has a gap: status=%s err=%v", projectedOrder.Status, err)
	}
	var cases int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_private_execution_reconciliation_cases
		WHERE execution_operation_id='pex_projection_1' AND status='PENDING'`).Scan(&cases); err != nil || cases != 1 {
		t.Fatalf("reconciliation cases=%d err=%v", cases, err)
	}
}

func TestPrivateExecutionEvents_AllShipmentsCompleteOrder(t *testing.T) {
	f := newShipmentsFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Private Completion Co")
	item := f.activeCatalogItem(t, "PRIVATE_COMPLETION")
	order, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	var shipmentIDs []string
	for i := 1; i <= 2; i++ {
		shipment, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
			OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID,
			DeclaredValueCurrency: "NGN"})
		if err != nil {
			t.Fatal(err)
		}
		shipmentIDs = append(shipmentIDs, shipment.ID)
		suffix := string(rune('0' + i))
		insertPrivateExecutionFixture(t, f, order.ID, shipment.ID,
			"pas_complete_"+suffix, "pex_complete_"+suffix)
	}
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(f.pool))
	service := marketusecases.NewService(marketusecases.Options{Pool: f.pool,
		PrivateEvents: app.NewPrivateExecutionEventProjector(f.pool, publisher)})
	for i := 1; i <= 2; i++ {
		suffix := string(rune('0' + i))
		for _, event := range []marketusecases.InboundEvent{
			privateLifecycleEvent("pex_complete_"+suffix, "trip_complete_"+suffix,
				"evt_complete_"+suffix+"a", marketusecases.EventTripCreated, "IN_PROGRESS", 1),
			privateLifecycleEvent("pex_complete_"+suffix, "trip_complete_"+suffix,
				"evt_complete_"+suffix+"b", marketusecases.EventTripCompleted, "COMPLETED", 2),
		} {
			if _, err := service.IngestEvent(ctx, nil, event); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, shipmentID := range shipmentIDs {
		shipment, err := f.shipments.Get(ctx, f.tenant, shipmentID)
		if err != nil || shipment.Status != shipmentsmodels.ShipmentCompleted {
			t.Fatalf("shipment %s status=%s err=%v", shipmentID, shipment.Status, err)
		}
	}
	projectedOrder, err := f.orders.Get(ctx, f.tenant, order.ID)
	if err != nil || projectedOrder.Status != ordersmodels.StatusCompleted {
		t.Fatalf("order status=%s err=%v", projectedOrder.Status, err)
	}
}
