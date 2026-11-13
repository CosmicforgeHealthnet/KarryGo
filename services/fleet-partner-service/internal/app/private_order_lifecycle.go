package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	approvalusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	ordersmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	ordersrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privaterepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/repositories"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	shipmentsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	shipmentsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// The Section 15 private order lifecycle crosses three sibling features —
// orders, shipments and private assignments — none of which may import
// another's repositories. These adapters are the one place, in the
// composition root, allowed to touch all three. Each writes inside the
// caller's transaction, with immutable history, an audit row and an outbox
// event for every status it moves.

// lifecyclePublisher is the outbox publisher the adapters write through.
type lifecyclePublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string,
		aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

type lifecycleWriter struct {
	orders    *ordersrepos.Repository
	shipments *shipmentsrepos.Repository
	events    lifecyclePublisher
	audit     *audit.Recorder
}

func newLifecycleWriter(orders *ordersrepos.Repository, shipments *shipmentsrepos.Repository,
	events lifecyclePublisher) lifecycleWriter {
	return lifecycleWriter{orders: orders, shipments: shipments, events: events, audit: audit.NewRecorder()}
}

type lifecycleActor struct {
	organizationID, actorType, actorID, correlationID string
}

func (w lifecycleWriter) moveShipment(ctx context.Context, tx pgx.Tx, a lifecycleActor,
	shipment shipmentsmodels.Shipment, to, reason, eventType string) (shipmentsmodels.Shipment, error) {
	previous := shipment.Status
	updated, err := w.shipments.SetStatus(ctx, tx, a.organizationID, shipment.ID, shipment.Version, to, a.actorID)
	if err != nil {
		return shipment, err
	}
	if err = w.shipments.AppendHistory(ctx, tx, shipmentsmodels.ShipmentHistory{
		OrganizationID: a.organizationID, ShipmentID: shipment.ID, PreviousStatus: previous, NewStatus: to,
		ReasonCode: reason, ActorType: a.actorType, ActorID: a.actorID, CorrelationID: a.correlationID,
	}); err != nil {
		return shipment, err
	}
	if err = w.audit.Record(ctx, tx, audit.Entry{OrganizationID: a.organizationID, ActorType: a.actorType,
		ActorID: a.actorID, Action: "shipments.shipment.status_changed", ResourceType: "shipment",
		ResourceID: shipment.ID, CorrelationID: a.correlationID, Reason: reason,
		PreviousValues: map[string]any{"status": previous}, NewValues: map[string]any{"status": to}}); err != nil {
		return shipment, err
	}
	if w.events != nil {
		if err = w.events.Publish(ctx, tx, eventType, "shipment", updated.ID, updated.Version, a.organizationID,
			a.correlationID, map[string]any{"shipmentId": updated.ID, "orderId": updated.OrderID,
				"previousStatus": previous, "status": to, "reasonCode": reason}); err != nil {
			return shipment, err
		}
	}
	return updated, nil
}

func (w lifecycleWriter) moveOrder(ctx context.Context, tx pgx.Tx, a lifecycleActor, order ordersmodels.Order,
	to, reason string) (ordersmodels.Order, error) {
	previous := order.Status
	updated, err := w.orders.SetStatus(ctx, tx, a.organizationID, order.ID, order.Version, to, a.actorID)
	if err != nil {
		return order, err
	}
	if err = w.orders.AppendHistory(ctx, tx, ordersmodels.OrderHistory{
		OrganizationID: a.organizationID, OrderID: order.ID, PreviousStatus: previous, NewStatus: to,
		ReasonCode: reason, ActorType: a.actorType, ActorID: a.actorID, CorrelationID: a.correlationID,
	}); err != nil {
		return order, err
	}
	if err = w.audit.Record(ctx, tx, audit.Entry{OrganizationID: a.organizationID, ActorType: a.actorType,
		ActorID: a.actorID, Action: "orders.order.status_changed", ResourceType: "private_order",
		ResourceID: order.ID, CorrelationID: a.correlationID, Reason: reason,
		PreviousValues: map[string]any{"status": previous}, NewValues: map[string]any{"status": to}}); err != nil {
		return order, err
	}
	if w.events != nil {
		eventType := "organization.orders.order.status_changed.v1"
		switch to {
		case ordersmodels.StatusPlanned:
			eventType = "organization.orders.order.planned.v1"
		case ordersmodels.StatusAssigned:
			eventType = "organization.orders.order.assigned.v1"
		}
		if err = w.events.Publish(ctx, tx, eventType, "private_order", updated.ID, updated.Version,
			a.organizationID, a.correlationID, map[string]any{"orderId": updated.ID,
				"previousStatus": previous, "status": to, "reasonCode": reason}); err != nil {
			return order, err
		}
	}
	return updated, nil
}

// ---------------------------------------------------------------------------
// Work progress: assignment created -> PLANNED, provider accepted -> ASSIGNED
// ---------------------------------------------------------------------------

type privateWorkProgressAdapter struct{ w lifecycleWriter }

// NewPrivateWorkProgressAdapter builds the privateoperations -> orders/
// shipments progression adapter.
func NewPrivateWorkProgressAdapter(orders *ordersrepos.Repository, shipments *shipmentsrepos.Repository,
	events lifecyclePublisher) privateusecases.WorkProgressPort {
	return privateWorkProgressAdapter{w: newLifecycleWriter(orders, shipments, events)}
}

func progressActor(org string, actor privateusecases.ProgressActor) lifecycleActor {
	actorType := actor.ActorType
	if actorType == "" {
		actorType = "system"
	}
	return lifecycleActor{organizationID: org, actorType: actorType, actorID: actor.ActorID,
		correlationID: actor.CorrelationID}
}

// MarkPlanned records that a provider offer now exists for the work: a
// READY shipment becomes PLANNED and a CONFIRMED order becomes PLANNED. Work
// already further along is left alone.
func (a privateWorkProgressAdapter) MarkPlanned(ctx context.Context, tx pgx.Tx,
	actor privateusecases.ProgressActor, work privateusecases.WorkInfo, assignmentID string) error {
	org, err := organizationOfWork(ctx, tx, work, assignmentID)
	if err != nil {
		return err
	}
	la := progressActor(org, actor)
	if work.ShipmentID != "" {
		shipment, err := a.w.shipments.Lock(ctx, tx, org, work.ShipmentID)
		if err != nil {
			return err
		}
		if shipment.Status == shipmentsmodels.ShipmentReady {
			if _, err = a.w.moveShipment(ctx, tx, la, shipment, shipmentsmodels.ShipmentPlanned,
				ordersmodels.ReasonPrivateAssignment, "organization.shipments.shipment.planned.v1"); err != nil {
				return err
			}
		}
	}
	if work.PrivateOrderID != "" {
		order, err := a.w.orders.Lock(ctx, tx, org, work.PrivateOrderID)
		if err != nil {
			return err
		}
		if order.Status == ordersmodels.StatusConfirmed {
			if _, err = a.w.moveOrder(ctx, tx, la, order, ordersmodels.StatusPlanned,
				ordersmodels.ReasonPrivateAssignment); err != nil {
				return err
			}
		}
	}
	return nil
}

// assignedOrBeyond reports whether a shipment has a provider committed to
// it (or has already finished).
func assignedOrBeyond(status string) bool {
	switch status {
	case shipmentsmodels.ShipmentAssigned, shipmentsmodels.ShipmentInTransit, shipmentsmodels.ShipmentCompleted,
		shipmentsmodels.ShipmentPartiallyCompleted, shipmentsmodels.ShipmentFailed:
		return true
	}
	return false
}

// MarkAssigned records a provider's acceptance: the shipment becomes
// ASSIGNED, and the order becomes ASSIGNED once every one of its live
// (non-cancelled) shipments has a committed provider. An order with one
// shipment accepted and another still waiting stays PLANNED.
func (a privateWorkProgressAdapter) MarkAssigned(ctx context.Context, tx pgx.Tx,
	actor privateusecases.ProgressActor, work privateusecases.WorkInfo, assignmentID string) error {
	org, err := organizationOfWork(ctx, tx, work, assignmentID)
	if err != nil {
		return err
	}
	la := progressActor(org, actor)
	if work.ShipmentID != "" {
		shipment, err := a.w.shipments.Lock(ctx, tx, org, work.ShipmentID)
		if err != nil {
			return err
		}
		if shipment.Status == shipmentsmodels.ShipmentReady || shipment.Status == shipmentsmodels.ShipmentPlanned {
			for _, next := range privatemodels.ShipmentProjectionPath(shipment.Status, privatemodels.OutcomeAssigned) {
				if shipment, err = a.w.moveShipment(ctx, tx, la, shipment, next,
					ordersmodels.ReasonProviderAccepted, "organization.shipments.shipment."+lowerStatus(next)+".v1"); err != nil {
					return err
				}
			}
		}
	}
	if work.PrivateOrderID == "" {
		return nil
	}
	order, err := a.w.orders.Lock(ctx, tx, org, work.PrivateOrderID)
	if err != nil {
		return err
	}
	if order.Status != ordersmodels.StatusConfirmed && order.Status != ordersmodels.StatusPlanned {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT status FROM fleet_partner_shipments
		WHERE organization_id=$1 AND order_id=$2 AND status <> 'CANCELLED' FOR SHARE`, org, order.ID)
	if err != nil {
		return err
	}
	live, committed := 0, 0
	for rows.Next() {
		var status string
		if err = rows.Scan(&status); err != nil {
			rows.Close()
			return err
		}
		live++
		if assignedOrBeyond(status) {
			committed++
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if live == 0 || committed != live {
		return nil
	}
	for _, next := range privatemodels.OrderProjectionPath(order.Status, privatemodels.OutcomeAssigned) {
		if order, err = a.w.moveOrder(ctx, tx, la, order, next, ordersmodels.ReasonProviderAccepted); err != nil {
			return err
		}
	}
	return nil
}

func lowerStatus(status string) string {
	out := []byte(status)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + ('a' - 'A')
		}
	}
	return string(out)
}

// organizationOfWork reads the organization from the assignment row the
// caller just wrote or locked; WorkInfo itself carries no tenant.
func organizationOfWork(ctx context.Context, tx pgx.Tx, _ privateusecases.WorkInfo, assignmentID string) (string, error) {
	var org string
	err := tx.QueryRow(ctx, `SELECT organization_id FROM fleet_partner_private_assignments WHERE id=$1`,
		assignmentID).Scan(&org)
	return org, err
}

// ---------------------------------------------------------------------------
// Order cancellation: withdraw dependent shipments and pending offers
// ---------------------------------------------------------------------------

type ordersDependentWorkAdapter struct {
	w       lifecycleWriter
	private *privaterepos.Repository
}

// NewOrdersDependentWorkAdapter builds the orders -> shipments/private
// assignments cancellation compensation adapter.
func NewOrdersDependentWorkAdapter(orders *ordersrepos.Repository, shipments *shipmentsrepos.Repository,
	private *privaterepos.Repository, events lifecyclePublisher) ordersusecases.DependentWorkPort {
	return ordersDependentWorkAdapter{w: newLifecycleWriter(orders, shipments, events), private: private}
}

// orderAssignmentsSQL finds every private assignment serving an order,
// whichever way its work was addressed (the order, one of its shipments, or
// a published route stop for it).
const orderAssignmentsSQL = `SELECT a.id, a.status FROM fleet_partner_private_assignments a
	WHERE a.organization_id = $1 AND (
		a.private_order_id = $2
		OR a.shipment_id IN (SELECT id FROM fleet_partner_shipments WHERE organization_id = $1 AND order_id = $2)
		OR a.route_stop_id IN (SELECT rs.id FROM fleet_partner_route_stops rs
			WHERE rs.organization_id = $1 AND (rs.order_id = $2 OR rs.source_order_stop_id IN
				(SELECT os.id FROM fleet_partner_order_stops os WHERE os.organization_id = $1 AND os.order_id = $2))))
	ORDER BY a.id FOR UPDATE OF a`

// CancelForOrder refuses if a provider has already accepted work for the
// order, otherwise withdraws every pending offer and cancels every shipment
// that has not started moving.
func (a ordersDependentWorkAdapter) CancelForOrder(ctx context.Context, tx pgx.Tx, t tenant.Context,
	orderID, reasonCode string) error {
	rows, err := tx.Query(ctx, orderAssignmentsSQL, t.OrganizationID, orderID)
	if err != nil {
		return err
	}
	var pending []string
	for rows.Next() {
		var id, status string
		if err = rows.Scan(&id, &status); err != nil {
			rows.Close()
			return err
		}
		switch status {
		case privatemodels.StatusAccepted, privatemodels.StatusReady, privatemodels.StatusInProgress:
			rows.Close()
			return errorcodes.Conflict(errorcodes.OrderNotCancellable,
				"A provider has already accepted work for this order; cancel that private assignment so the execution service can stop it.")
		case privatemodels.StatusPending:
			pending = append(pending, id)
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	la := lifecycleActor{organizationID: t.OrganizationID, actorType: string(t.ActorType), actorID: t.ActorID,
		correlationID: t.CorrelationID}
	for _, id := range pending {
		assignment, err := a.private.LockAssignment(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		updated, err := a.private.SetAssignmentStatus(ctx, tx, t.OrganizationID, id, assignment.Version,
			privatemodels.StatusCancelled, "ORDER_CANCELLED", "", "")
		if err != nil {
			return err
		}
		if err = a.private.History(ctx, tx, t.OrganizationID, id, assignment.Status, updated.Status,
			"ORDER_CANCELLED", la.actorType, la.actorID, la.correlationID); err != nil {
			return err
		}
		if err = a.w.audit.Record(ctx, tx, audit.EntryFromTenant(t, "private_assignments.cancelled",
			"private_assignment", id)); err != nil {
			return err
		}
		if a.w.events != nil {
			if err = a.w.events.Publish(ctx, tx, "private_assignment.cancelled.v1", "private_assignment", id,
				updated.Version, t.OrganizationID, t.CorrelationID, map[string]any{"assignmentId": id,
					"reasonCode": "ORDER_CANCELLED", "executionCancellationRequested": false}); err != nil {
				return err
			}
		}
	}

	shipmentRows, err := tx.Query(ctx, `SELECT id FROM fleet_partner_shipments
		WHERE organization_id=$1 AND order_id=$2 AND status IN ('DRAFT','READY','PLANNED') ORDER BY id`,
		t.OrganizationID, orderID)
	if err != nil {
		return err
	}
	var shipmentIDs []string
	for shipmentRows.Next() {
		var id string
		if err = shipmentRows.Scan(&id); err != nil {
			shipmentRows.Close()
			return err
		}
		shipmentIDs = append(shipmentIDs, id)
	}
	shipmentRows.Close()
	if err = shipmentRows.Err(); err != nil {
		return err
	}
	for _, id := range shipmentIDs {
		shipment, err := a.w.shipments.Lock(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if _, err = a.w.moveShipment(ctx, tx, la, shipment, shipmentsmodels.ShipmentCancelled,
			"ORDER_CANCELLED:"+reasonCode, "organization.shipments.shipment.cancelled.v1"); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Approval: bind order approval to the WP9 framework
// ---------------------------------------------------------------------------

type ordersApprovalAdapter struct {
	approvals   *approvalusecases.Service
	eligibility *eligibilityusecases.Service
}

// NewOrdersApprovalAdapter binds Section 15 order approval to the WP9
// approval framework and the eligibility policy that says whether approval
// is required.
func NewOrdersApprovalAdapter(approvals *approvalusecases.Service,
	eligibility *eligibilityusecases.Service) ordersusecases.ApprovalPort {
	return ordersApprovalAdapter{approvals: approvals, eligibility: eligibility}
}

// policyAbsent reports the framework saying no policy requires approval.
func policyAbsent(err error) bool {
	code := apperrors.From(err).Code
	return code == errorcodes.EligibilityPolicyMissing || code == errorcodes.ApprovalPolicyMissing
}

func (a ordersApprovalAdapter) Required(ctx context.Context, organizationID string) (bool, error) {
	if a.eligibility == nil {
		return false, nil
	}
	policy, err := a.eligibility.Policy(ctx, organizationID, ordersusecases.ApprovalOperationType)
	if err != nil {
		if policyAbsent(err) {
			return false, nil
		}
		return false, err
	}
	return policy != nil && policy.ApprovalRequired, nil
}

func (a ordersApprovalAdapter) Request(ctx context.Context, input ordersusecases.ApprovalRequestInput) (string, error) {
	if a.approvals == nil {
		return "", nil
	}
	result, err := a.approvals.Request(ctx, approvalusecases.RequestInput{
		Actor: approvalusecases.Actor{OrganizationID: input.OrganizationID,
			OrganizationUserID: input.OrganizationUserID, BranchID: input.BranchID,
			ActorType: "ORGANIZATION_USER", ActorID: input.OrganizationUserID},
		ResourceType: "private_order", ResourceID: input.OrderID, Action: "approve",
		OperationType: ordersusecases.ApprovalOperationType, Payload: input.Payload,
		ExpectedSubjectVersion: input.ExpectedSubjectVersion,
		IdempotencyKey:         input.IdempotencyKey, CorrelationID: input.CorrelationID,
	})
	if err != nil {
		if policyAbsent(err) {
			return "", nil
		}
		return "", err
	}
	if result == nil || result.Request == nil {
		return "", nil
	}
	return result.Request.ID, nil
}

func (a ordersApprovalAdapter) RequireApproved(ctx context.Context, organizationID, requestID string,
	payload map[string]any) error {
	if a.approvals == nil {
		return errors.New("approval framework is not configured")
	}
	_, err := a.approvals.RequireApproved(ctx, organizationID, requestID, payload)
	return err
}
