package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	marketrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	marketusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	ordermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	orderrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privaterepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/repositories"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	shipmentmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	shipmentrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

type privateExecutionEventProjector struct {
	private   *privaterepos.Repository
	shipments *shipmentrepos.Repository
	orders    *orderrepos.Repository
	events    marketusecases.EventPublisher
	audit     *audit.Recorder
}

func NewPrivateExecutionEventProjector(pool *pgxpool.Pool,
	publisher marketusecases.EventPublisher) *privateExecutionEventProjector {
	return NewPrivateExecutionEventProjectorWithDependencies(
		privaterepos.NewRepository(pool), shipmentrepos.NewRepository(pool),
		orderrepos.NewRepository(pool), publisher)
}

func NewPrivateExecutionEventProjectorWithDependencies(
	private *privaterepos.Repository,
	shipments *shipmentrepos.Repository,
	orders *orderrepos.Repository,
	publisher marketusecases.EventPublisher,
) *privateExecutionEventProjector {
	return &privateExecutionEventProjector{private: private, shipments: shipments,
		orders: orders, events: publisher, audit: audit.NewRecorder()}
}

type privateProjection struct {
	ID, AggregateID, PayloadHash, Freshness string
	SourceVersion                           int64
}

type privateOperationRef struct {
	ID, OrganizationID, OrderID, ShipmentID, AssignmentID, ExecutionService string
}

func (p *privateExecutionEventProjector) ProjectPrivateExecutionEvent(ctx context.Context,
	tx pgx.Tx, event marketusecases.InboundEvent) (marketrepos.SourceEventOutcome, error) {
	opID := payloadString(event.Payload, "fleetExecutionOperationId", "fleetOperationId")
	var op privateOperationRef
	if err := tx.QueryRow(ctx, `SELECT id,organization_id,private_order_id,shipment_id,
		assignment_id,execution_service FROM fleet_partner_private_execution_operations
		WHERE id=$1 FOR UPDATE`, opID).Scan(&op.ID, &op.OrganizationID, &op.OrderID,
		&op.ShipmentID, &op.AssignmentID, &op.ExecutionService); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", privaterepos.ErrNotFound
		}
		return "", err
	}
	if expectedSource(op.ExecutionService) != event.SourceService {
		return "", &marketusecases.IngestError{Code: "private_execution_source_mismatch",
			Message: "Event producer does not own this private execution.", Status: 409}
	}

	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return "", err
	}
	payloadHash := events.HashPayload(payloadBytes)
	if strings.TrimSpace(event.PayloadHash) != "" {
		payloadHash = event.PayloadHash
	}

	projection, found, err := lockPrivateProjection(ctx, tx, op.ID)
	if err != nil {
		return "", err
	}
	if !found {
		if event.AggregateVersion != 1 {
			projection = privateProjection{ID: ids.New("pxp"), AggregateID: event.AggregateID,
				PayloadHash: payloadHash, Freshness: "GAP_DETECTED"}
			if err := insertPrivateProjection(ctx, tx, projection, op, event, "", 0); err != nil {
				return "", err
			}
			if err := openPrivateReconciliation(ctx, tx, op, projection.ID,
				"SOURCE_VERSION_GAP", event.CorrelationID); err != nil {
				return "", err
			}
			return marketrepos.SourceEventGap, nil
		}
		projection = privateProjection{ID: ids.New("pxp"), AggregateID: event.AggregateID,
			PayloadHash: payloadHash, Freshness: "CURRENT", SourceVersion: 1}
		outcome := privatemodels.NormalizeExecutionStatus(event.EventType, payloadString(event.Payload, "status"))
		if err := insertPrivateProjection(ctx, tx, projection, op, event, outcome, 1); err != nil {
			return "", err
		}
		if err := p.applyDerivedOutcome(ctx, tx, op, event, outcome); err != nil {
			return "", err
		}
		return marketrepos.SourceEventApplied, nil
	}

	if event.AggregateID != projection.AggregateID {
		if err := markPrivateProjection(ctx, tx, projection.ID, "CONFLICT"); err != nil {
			return "", err
		}
		if err := openPrivateReconciliation(ctx, tx, op, projection.ID,
			"SOURCE_AGGREGATE_CONFLICT", event.CorrelationID); err != nil {
			return "", err
		}
		return marketrepos.SourceEventGap, nil
	}
	if event.AggregateVersion < projection.SourceVersion ||
		(event.AggregateVersion == projection.SourceVersion && payloadHash == projection.PayloadHash) {
		return marketrepos.SourceEventStale, nil
	}
	if event.AggregateVersion == projection.SourceVersion {
		if err := markPrivateProjection(ctx, tx, projection.ID, "CONFLICT"); err != nil {
			return "", err
		}
		if err := openPrivateReconciliation(ctx, tx, op, projection.ID,
			"SOURCE_VERSION_CONFLICT", event.CorrelationID); err != nil {
			return "", err
		}
		return marketrepos.SourceEventGap, nil
	}
	if event.AggregateVersion > projection.SourceVersion+1 || projection.Freshness != "CURRENT" {
		if err := markPrivateProjection(ctx, tx, projection.ID, "GAP_DETECTED"); err != nil {
			return "", err
		}
		if err := openPrivateReconciliation(ctx, tx, op, projection.ID,
			"SOURCE_VERSION_GAP", event.CorrelationID); err != nil {
			return "", err
		}
		return marketrepos.SourceEventGap, nil
	}

	outcome := privatemodels.NormalizeExecutionStatus(event.EventType, payloadString(event.Payload, "status"))
	if err := updatePrivateProjection(ctx, tx, projection.ID, event, payloadHash, outcome); err != nil {
		return "", err
	}
	if err := p.applyDerivedOutcome(ctx, tx, op, event, outcome); err != nil {
		return "", err
	}
	return marketrepos.SourceEventApplied, nil
}

func (p *privateExecutionEventProjector) RepairPrivateExecutionProjection(ctx context.Context,
	tx pgx.Tx, operation privatemodels.ExecutionOperation, outcome privateusecases.ExecutionOutcome) error {
	if !outcome.Definite || outcome.SourceVersion <= 0 {
		return errors.New("private execution reconciliation did not return authoritative state")
	}
	normalized := privatemodels.NormalizeExecutionStatus("", outcome.NormalizedStatus)
	if outcome.Cancelled {
		normalized = privatemodels.OutcomeCancelled
	} else if outcome.Confirmed && normalized == "" {
		normalized = privatemodels.OutcomeAssigned
	}
	if normalized == "" {
		return errors.New("private execution reconciliation returned an unsupported status")
	}
	tag, err := tx.Exec(ctx, `UPDATE fleet_partner_private_execution_projections SET
		canonical_request_reference=COALESCE(NULLIF($2,''),canonical_request_reference),
		canonical_trip_reference=COALESCE(NULLIF($3,''),canonical_trip_reference),
		normalized_status=$4,source_version=$5,freshness_state='CURRENT',
		last_event_id=$6,last_payload_hash=$7,last_event_at=now(),version=version+1,updated_at=now()
		WHERE execution_operation_id=$1 AND source_version<=$5`, operation.ID,
		outcome.CanonicalRequestReference, outcome.CanonicalTripReference, normalized,
		outcome.SourceVersion, "reconciliation:"+operation.ID,
		events.HashPayload([]byte(outcome.NormalizedStatus)))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("private execution projection is missing or ahead of read-back")
	}
	event := marketusecases.InboundEvent{
		EventID: "reconciliation:" + operation.ID, EventType: "trip.status_changed.v1",
		SourceService: expectedSource(operation.ExecutionService), AggregateVersion: outcome.SourceVersion,
		CorrelationID: operation.CorrelationID, Payload: map[string]any{
			"requestId": outcome.CanonicalRequestReference, "tripId": outcome.CanonicalTripReference,
			"status": outcome.NormalizedStatus,
		},
	}
	return p.applyDerivedOutcome(ctx, tx, privateOperationRef{
		ID: operation.ID, OrganizationID: operation.OrganizationID, OrderID: operation.PrivateOrderID,
		ShipmentID: operation.ShipmentID, AssignmentID: operation.AssignmentID,
		ExecutionService: operation.ExecutionService,
	}, event, normalized)
}

func lockPrivateProjection(ctx context.Context, tx pgx.Tx, operationID string) (privateProjection, bool, error) {
	var p privateProjection
	err := tx.QueryRow(ctx, `SELECT id,source_aggregate_id,source_version,last_payload_hash,
		freshness_state FROM fleet_partner_private_execution_projections
		WHERE execution_operation_id=$1 FOR UPDATE`, operationID).Scan(
		&p.ID, &p.AggregateID, &p.SourceVersion, &p.PayloadHash, &p.Freshness)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, false, nil
	}
	return p, err == nil, err
}

func insertPrivateProjection(ctx context.Context, tx pgx.Tx, projection privateProjection,
	op privateOperationRef, event marketusecases.InboundEvent, outcome string, sourceVersion int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_private_execution_projections(
		id,organization_id,execution_operation_id,source_service,source_aggregate_id,
		canonical_request_reference,canonical_trip_reference,normalized_status,source_version,
		freshness_state,last_event_id,last_payload_hash,last_event_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, projection.ID,
		op.OrganizationID, op.ID, event.SourceService, event.AggregateID,
		payloadString(event.Payload, "requestId", "bookingId"), payloadString(event.Payload, "tripId"),
		outcome, sourceVersion, projection.Freshness, event.EventID, projection.PayloadHash, event.OccurredAt)
	return err
}

func updatePrivateProjection(ctx context.Context, tx pgx.Tx, id string,
	event marketusecases.InboundEvent, payloadHash, outcome string) error {
	_, err := tx.Exec(ctx, `UPDATE fleet_partner_private_execution_projections SET
		canonical_request_reference=COALESCE(NULLIF($2,''),canonical_request_reference),
		canonical_trip_reference=COALESCE(NULLIF($3,''),canonical_trip_reference),
		normalized_status=COALESCE(NULLIF($4,''),normalized_status),source_version=$5,
		freshness_state='CURRENT',last_event_id=$6,last_payload_hash=$7,last_event_at=$8,
		version=version+1,updated_at=now() WHERE id=$1`, id,
		payloadString(event.Payload, "requestId", "bookingId"), payloadString(event.Payload, "tripId"),
		outcome, event.AggregateVersion, event.EventID, payloadHash, event.OccurredAt)
	return err
}

func markPrivateProjection(ctx context.Context, tx pgx.Tx, id, freshness string) error {
	_, err := tx.Exec(ctx, `UPDATE fleet_partner_private_execution_projections SET
		freshness_state=$2,version=version+1,updated_at=now() WHERE id=$1`, id, freshness)
	return err
}

func openPrivateReconciliation(ctx context.Context, tx pgx.Tx, op privateOperationRef,
	projectionID, reason, correlationID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_private_execution_reconciliation_cases(
		id,organization_id,execution_operation_id,projection_id,reason_code,correlation_id)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, ids.New("pxr"),
		op.OrganizationID, op.ID, projectionID, reason, correlationID)
	return err
}

func expectedSource(service string) string {
	switch service {
	case "DISPATCH":
		return "driver-dispatch-delivery-service"
	case "HAULING":
		return "driver-hauling-service"
	default:
		return ""
	}
}

func payloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (p *privateExecutionEventProjector) applyDerivedOutcome(ctx context.Context, tx pgx.Tx,
	op privateOperationRef, event marketusecases.InboundEvent, outcome string) error {
	if outcome == "" {
		return nil
	}
	requestRef := payloadString(event.Payload, "requestId", "bookingId")
	tripRef := payloadString(event.Payload, "tripId")
	opStatus := "CONFIRMED"
	if outcome == privatemodels.OutcomeFailed {
		opStatus = "FAILED"
	} else if outcome == privatemodels.OutcomeCancelled {
		opStatus = "CANCELLED"
	}
	if _, err := tx.Exec(ctx, `UPDATE fleet_partner_private_execution_operations SET
		status=$2,canonical_request_reference=COALESCE(NULLIF($3,''),canonical_request_reference),
		canonical_trip_reference=COALESCE(NULLIF($4,''),canonical_trip_reference),
		source_version=GREATEST(source_version,$5),completed_at=CASE WHEN $2 IN('CONFIRMED','FAILED','CANCELLED') THEN now() ELSE completed_at END,
		version=version+1,updated_at=now() WHERE id=$1`, op.ID, opStatus, requestRef, tripRef,
		event.AggregateVersion); err != nil {
		return err
	}
	if err := p.projectAssignment(ctx, tx, op, event, outcome); err != nil {
		return err
	}
	if op.ShipmentID == "" {
		return nil
	}
	shipment, err := p.shipments.Lock(ctx, tx, op.OrganizationID, op.ShipmentID)
	if err != nil {
		return err
	}
	for _, next := range privatemodels.ShipmentProjectionPath(shipment.Status, outcome) {
		previous := shipment.Status
		shipment, err = p.shipments.SetStatus(ctx, tx, op.OrganizationID, shipment.ID,
			shipment.Version, next, "private-execution-projector")
		if err != nil {
			return err
		}
		if err = p.shipments.AppendHistory(ctx, tx, shipmentmodels.ShipmentHistory{
			OrganizationID: op.OrganizationID, ShipmentID: shipment.ID, PreviousStatus: previous,
			NewStatus: next, ReasonCode: "AUTHORITATIVE_EXECUTION_EVENT", ActorType: "system",
			ActorID: "private-execution-projector", CorrelationID: event.CorrelationID,
		}); err != nil {
			return err
		}
		if err = p.audit.Record(ctx, tx, audit.Entry{OrganizationID: op.OrganizationID,
			ActorType: "system", ActorID: "private-execution-projector", Action: "shipments.projected",
			ResourceType: "shipment", ResourceID: shipment.ID, CorrelationID: event.CorrelationID,
			PreviousValues: map[string]any{"status": previous}, NewValues: map[string]any{"status": next}}); err != nil {
			return err
		}
		if p.events != nil {
			if err = p.events.Publish(ctx, tx, "shipment.status_projected.v1", "shipment",
				shipment.ID, shipment.Version, op.OrganizationID, event.CorrelationID,
				map[string]any{"shipmentId": shipment.ID, "status": next, "executionOperationId": op.ID}); err != nil {
				return err
			}
		}
	}
	return p.aggregateOrder(ctx, tx, op, event, shipment.OrderID)
}

func (p *privateExecutionEventProjector) projectAssignment(ctx context.Context, tx pgx.Tx,
	op privateOperationRef, event marketusecases.InboundEvent, outcome string) error {
	assignment, err := p.private.LockAssignment(ctx, tx, op.OrganizationID, op.AssignmentID)
	if err != nil {
		return err
	}
	next := ""
	switch outcome {
	case privatemodels.OutcomeAssigned:
		if assignment.Status == privatemodels.StatusAccepted {
			next = privatemodels.StatusReady
		}
	case privatemodels.OutcomeActive:
		if assignment.Status == privatemodels.StatusAccepted || assignment.Status == privatemodels.StatusReady {
			next = privatemodels.StatusInProgress
		}
	case privatemodels.OutcomeCompleted:
		if assignment.Status == privatemodels.StatusAccepted || assignment.Status == privatemodels.StatusReady || assignment.Status == privatemodels.StatusInProgress {
			next = privatemodels.StatusCompleted
		}
	case privatemodels.OutcomeCancelled:
		if assignment.Status != privatemodels.StatusCompleted {
			next = privatemodels.StatusCancelled
		}
	}
	if next == "" || next == assignment.Status {
		return nil
	}
	updated, err := p.private.SetAssignmentStatus(ctx, tx, op.OrganizationID, assignment.ID,
		assignment.Version, next, "AUTHORITATIVE_EXECUTION_EVENT", "", "")
	if err != nil {
		return err
	}
	return p.private.History(ctx, tx, op.OrganizationID, assignment.ID, assignment.Status,
		updated.Status, "AUTHORITATIVE_EXECUTION_EVENT", "system", "private-execution-projector",
		event.CorrelationID)
}

func (p *privateExecutionEventProjector) aggregateOrder(ctx context.Context, tx pgx.Tx,
	op privateOperationRef, event marketusecases.InboundEvent, orderID string) error {
	rows, err := tx.Query(ctx, `SELECT status FROM fleet_partner_shipments
		WHERE organization_id=$1 AND order_id=$2 ORDER BY id FOR SHARE`, op.OrganizationID, orderID)
	if err != nil {
		return err
	}
	var statuses []string
	for rows.Next() {
		var status string
		if err = rows.Scan(&status); err != nil {
			rows.Close()
			return err
		}
		statuses = append(statuses, status)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	outcome := privatemodels.AggregateOrderOutcome(statuses)
	if outcome == "" {
		return nil
	}
	order, err := p.orders.Lock(ctx, tx, op.OrganizationID, orderID)
	if err != nil {
		return err
	}
	for _, next := range privatemodels.OrderProjectionPath(order.Status, outcome) {
		previous := order.Status
		order, err = p.orders.SetStatus(ctx, tx, op.OrganizationID, order.ID, order.Version,
			next, "private-execution-projector")
		if err != nil {
			return err
		}
		if err = p.orders.AppendHistory(ctx, tx, ordermodels.OrderHistory{
			OrganizationID: op.OrganizationID, OrderID: order.ID, PreviousStatus: previous,
			NewStatus: next, ReasonCode: "SHIPMENT_OUTCOME_AGGREGATION", ActorType: "system",
			ActorID: "private-execution-projector", CorrelationID: event.CorrelationID,
		}); err != nil {
			return err
		}
		if err = p.audit.Record(ctx, tx, audit.Entry{OrganizationID: op.OrganizationID,
			ActorType: "system", ActorID: "private-execution-projector", Action: "orders.projected",
			ResourceType: "private_order", ResourceID: order.ID, CorrelationID: event.CorrelationID,
			PreviousValues: map[string]any{"status": previous}, NewValues: map[string]any{"status": next}}); err != nil {
			return err
		}
		if p.events != nil {
			if err = p.events.Publish(ctx, tx, "private_order.status_projected.v1", "private_order",
				order.ID, order.Version, op.OrganizationID, event.CorrelationID,
				map[string]any{"privateOrderId": order.ID, "status": next}); err != nil {
				return err
			}
		}
	}
	return nil
}
