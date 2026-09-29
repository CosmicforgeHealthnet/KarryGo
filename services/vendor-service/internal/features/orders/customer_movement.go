package orders

// Customer V4 §22: one purchase-level Activity row per ParentOrder.
//
// Execution Activity for a Food/Shop purchase already comes from the executing
// authority (Dispatch, Hauling, Taxi Goods) joined by parent_context_ref. That
// left no Activity at all for a vendor-owned delivery, or for a purchase
// cancelled or refunded before any platform mission existed. Vendor, as the
// ParentOrder authority, publishes the purchase row itself.
//
// The row carries ORDER state only, derived from the persisted aggregate. Its
// source_version is the ParentOrder's own monotonic version, so the Customer
// projection's version guard needs no Vendor bookkeeping. It is appended to
// the existing transactional outbox in the same transaction as the state
// change and delivered by its own worker to Customer's movement ingress.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	vendorevents "cosmicforge/logistics/services/vendor-service/internal/events"
	sharedevents "cosmicforge/logistics/shared/go/events"
)

// CustomerOrderMovementEvent is the Vendor outbox event type that carries the
// shared movement envelope to Customer Service.
const CustomerOrderMovementEvent = "commerce.customer_order.movement"

// CustomerOrderMovementSourceService is Vendor's identity on the envelope.
const CustomerOrderMovementSourceService = "vendor-service"

// SetCustomerOrderMovements enables purchase-row publication. Off by default:
// with the Customer integration disabled nothing is queued.
func (s *Store) SetCustomerOrderMovements(enabled bool) {
	if s != nil {
		s.customerMovements = enabled
	}
}

func (s *Store) movements() *vendorevents.DomainPublisher {
	if s == nil || !s.customerMovements {
		return nil
	}
	return s.outbox
}

// CustomerOrderMovementState maps the derived ParentOrder state onto the
// shared movement vocabulary. A cancelled purchase is terminal in Activity;
// refund resolution after that stays detail owned by Vendor and Wallet.
func CustomerOrderMovementState(state ParentState) (string, string) {
	switch state {
	case ParentStateCreated:
		return sharedevents.MovementStateRequested, ""
	case ParentStateInProgress:
		return sharedevents.MovementStateInProgress, ""
	case ParentStateRefundPending:
		return sharedevents.MovementStateInProgress, sharedevents.MovementStageRefundPending
	case ParentStateCompleted, ParentStatePartiallyCompleted:
		return sharedevents.MovementStateCompleted, sharedevents.MovementStageCompleted
	case ParentStatePartiallyRefunded:
		return sharedevents.MovementStateCompleted, sharedevents.MovementStageRefunded
	case ParentStateCancelled:
		return sharedevents.MovementStateCancelled, sharedevents.MovementStageCancelled
	case ParentStateRefunded:
		return sharedevents.MovementStateCancelled, sharedevents.MovementStageRefunded
	}
	return "", ""
}

// CustomerOrderMovementEnvelope builds the current purchase row from the
// persisted aggregate. ok is false when nothing may be published: an unmapped
// state, or a customer reference that is not a Customer Service identity.
func CustomerOrderMovementEnvelope(ctx context.Context, q queryer, parentID string, eventID string) (sharedevents.Envelope, bool, error) {
	var customerID, domain, state string
	var version int64
	var terminalAt *time.Time
	var address []byte
	err := q.QueryRow(ctx, `SELECT customer_id,domain,state,version,terminal_at,
		COALESCE(operational_address_snapshot,address_snapshot)
		FROM commerce_parent_orders WHERE id=$1`, parentID).
		Scan(&customerID, &domain, &state, &version, &terminalAt, &address)
	if errors.Is(err, pgx.ErrNoRows) {
		return sharedevents.Envelope{}, false, nil
	}
	if err != nil {
		return sharedevents.Envelope{}, false, err
	}
	if _, parseErr := uuid.Parse(customerID); parseErr != nil || (domain != "FOOD" && domain != "SHOP") {
		return sharedevents.Envelope{}, false, nil
	}
	movementState, stage := CustomerOrderMovementState(ParentState(state))
	if movementState == "" {
		return sharedevents.Envelope{}, false, nil
	}
	summary := sharedevents.MovementSummary{
		CustomerID: customerID, Authority: sharedevents.MovementAuthorityVendorCommerce, SourceRef: parentID,
		ProductCode: domain, MovementType: sharedevents.MovementTypeCommerceOrder,
		State: movementState, Stage: stage,
		ParentContextType: "COMMERCE_ORDER", ParentContextRef: parentID,
		Destination: destinationSummary(address),
	}
	if terminalAt != nil && sharedevents.IsTerminalMovementState(movementState) {
		completed := terminalAt.UTC()
		summary.CompletedAt = &completed
	}
	if eventID == "" {
		eventID = uuid.NewString()
	}
	envelope, err := sharedevents.NewMovementEnvelope(eventID,
		sharedevents.EventNameFor(sharedevents.MovementAuthorityVendorCommerce, movementState),
		CustomerOrderMovementSourceService, "commerce_parent_order", version, time.Now().UTC(), parentID, "", summary)
	if err != nil {
		return sharedevents.Envelope{}, false, err
	}
	return envelope, true, nil
}

// destinationSummary keeps only the customer's own drop-off label and point.
func destinationSummary(raw []byte) *sharedevents.PlaceSummary {
	var address struct {
		Address   string   `json:"address"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &address) != nil || address.Latitude == nil || address.Longitude == nil {
		return nil
	}
	return &sharedevents.PlaceSummary{Label: address.Address, Latitude: *address.Latitude, Longitude: *address.Longitude}
}

// appendCustomerOrderMovement queues the current purchase row in the caller's
// transaction. A nil publisher (feature off) queues nothing.
func appendCustomerOrderMovement(ctx context.Context, tx pgx.Tx, outbox *vendorevents.DomainPublisher, parentID string) error {
	if outbox == nil {
		return nil
	}
	envelope, ok, err := CustomerOrderMovementEnvelope(ctx, tx, parentID, "")
	if err != nil || !ok {
		return err
	}
	return outbox.Publish(ctx, tx, CustomerOrderMovementEvent, "ParentOrder", parentID,
		envelope.SourceVersion, "", parentID, "", envelope)
}
