package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Shipment lifecycle commands owned by Fleet (Section 58.6 draft -> ready and
// pre-execution cancellation). Every later edge — PLANNED when a private
// assignment is created for the shipment, ASSIGNED when the provider accepts,
// IN_TRANSIT and the terminal outcomes from the execution service's trip
// events — is driven by those producers, never by a local status write here.

// Reason codes on shipment history.
const (
	ReasonShipmentReady = "SHIPMENT_READY"
)

func (s *Service) lockShipment(ctx context.Context, tx pgx.Tx, t tenant.Context, shipmentID string) (models.Shipment, error) {
	shipment, err := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, shipmentID)
	if err != nil {
		return models.Shipment{}, translateShipmentNotFound(err)
	}
	if !t.BranchPermitted(shipment.BranchID) {
		return models.Shipment{}, shipmentNotFound()
	}
	return shipment, nil
}

func (s *Service) transition(ctx context.Context, tx pgx.Tx, t tenant.Context, shipment models.Shipment,
	to, reason, auditAction, eventType string) (models.Shipment, error) {
	updated, err := s.opts.Repository.SetStatus(ctx, tx, t.OrganizationID, shipment.ID, shipment.Version, to, t.ActorID)
	if err != nil {
		return models.Shipment{}, concurrency.AsConflict(translateVersionErr(err))
	}
	if err = s.opts.Repository.AppendHistory(ctx, tx, models.ShipmentHistory{
		OrganizationID: t.OrganizationID, ShipmentID: shipment.ID, PreviousStatus: shipment.Status,
		NewStatus: to, ReasonCode: reason, ActorType: string(t.ActorType), ActorID: t.ActorID,
		CorrelationID: t.CorrelationID,
	}); err != nil {
		return models.Shipment{}, err
	}
	entry := audit.EntryFromTenant(t, auditAction, "shipment", updated.ID)
	entry.PreviousValues = map[string]any{"status": shipment.Status}
	entry.NewValues = map[string]any{"status": to}
	entry.Reason = reason
	if err = s.opts.Audit.Record(ctx, tx, entry); err != nil {
		return models.Shipment{}, err
	}
	if s.opts.Events != nil {
		if err = s.opts.Events.Publish(ctx, tx, eventType, "shipment", updated.ID, updated.Version,
			t.OrganizationID, t.CorrelationID, map[string]any{"shipmentId": updated.ID,
				"orderId": updated.OrderID, "previousStatus": shipment.Status, "status": to,
				"reasonCode": reason}); err != nil {
			return models.Shipment{}, err
		}
	}
	return updated, nil
}

// MarkReady moves a DRAFT shipment to READY (Section 58.6: contents,
// handling and contacts complete). Only a READY shipment can be offered to a
// provider, so this is the gate between describing work and planning it.
//
// Preconditions: the parent order is confirmed and not terminal; the
// shipment describes what is moving (a cargo or passenger profile, a cargo
// item or a package); and no cargo item has been rejected by compliance.
func (s *Service) MarkReady(ctx context.Context, t tenant.Context, shipmentID string,
	expectedVersion int64) (models.Shipment, error) {
	if err := s.requireWritable(); err != nil {
		return models.Shipment{}, err
	}
	if expectedVersion <= 0 {
		return models.Shipment{}, apperrors.BadRequest("Provide the expected version via If-Match or expectedVersion.", nil)
	}
	var updated models.Shipment
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		shipment, err := s.lockShipment(ctx, tx, t, shipmentID)
		if err != nil {
			return err
		}
		if shipment.Version != expectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if !models.CanTransitionShipment(shipment.Status, models.ShipmentReady) || shipment.Status != models.ShipmentDraft {
			return errorcodes.Conflict(errorcodes.ShipmentInvalidState, "Only a DRAFT shipment can be marked ready.")
		}
		order, err := s.opts.Orders.GetOrder(ctx, tx, t.OrganizationID, shipment.OrderID)
		if err != nil {
			if errors.Is(err, ErrPortNotFound) {
				return errorcodes.NotFound(errorcodes.OrderNotFound, "Order not found.")
			}
			return err
		}
		if !order.Confirmed {
			return errorcodes.Conflict(errorcodes.ShipmentInvalidState,
				"A shipment can only become ready while its order is confirmed and not closed.")
		}
		items, err := s.opts.Repository.ListCargoItems(ctx, tx, t.OrganizationID, shipmentID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.RestrictedItemStatus == models.RestrictionRejected {
				return errorcodes.Conflict(errorcodes.ShipmentInvalidState,
					"A cargo item on this shipment was rejected by compliance review.")
			}
		}
		if len(shipment.CargoProfile) == 0 && len(shipment.PassengerProfile) == 0 &&
			len(items) == 0 && shipment.PackageCount == 0 {
			return errorcodes.UnprocessableEntity(errorcodes.ShipmentInvalidState,
				"Describe what is moving (cargo profile, passenger profile, cargo items or packages) before marking the shipment ready.")
		}
		updated, err = s.transition(ctx, tx, t, shipment, models.ShipmentReady, ReasonShipmentReady,
			"shipments.shipment.ready", "organization.shipments.shipment.ready.v1")
		return err
	})
	if err != nil {
		return models.Shipment{}, concurrency.AsConflict(err)
	}
	return updated, nil
}

// Cancel cancels a shipment that has not been offered to a provider (DRAFT
// or READY). A PLANNED shipment has a live provider offer; it is withdrawn by
// cancelling the private assignment or the whole order, never by a local
// status write that would leave the offer dangling.
func (s *Service) Cancel(ctx context.Context, t tenant.Context, shipmentID string, expectedVersion int64,
	reasonCode string) (models.Shipment, error) {
	if err := s.requireWritable(); err != nil {
		return models.Shipment{}, err
	}
	if expectedVersion <= 0 {
		return models.Shipment{}, apperrors.BadRequest("Provide the expected version via If-Match or expectedVersion.", nil)
	}
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		return models.Shipment{}, apperrors.Validation("reasonCode is required.",
			[]apperrors.FieldViolation{{Field: "reasonCode", Message: "required"}})
	}
	var updated models.Shipment
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		shipment, err := s.lockShipment(ctx, tx, t, shipmentID)
		if err != nil {
			return err
		}
		if shipment.Version != expectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if shipment.Status != models.ShipmentDraft && shipment.Status != models.ShipmentReady {
			return errorcodes.Conflict(errorcodes.ShipmentInvalidState,
				"Only a DRAFT or READY shipment can be cancelled directly; withdraw its private assignment or cancel the order.")
		}
		updated, err = s.transition(ctx, tx, t, shipment, models.ShipmentCancelled, reasonCode,
			"shipments.shipment.cancelled", "organization.shipments.shipment.cancelled.v1")
		return err
	})
	if err != nil {
		return models.Shipment{}, concurrency.AsConflict(err)
	}
	return updated, nil
}
