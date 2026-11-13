package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Ports
// ---------------------------------------------------------------------------

// ApprovalOperationType is the eligibility/approval policy key that governs
// private order approval. An organization that publishes an approval-
// requiring policy under this key gets a WP9 approval request on Submit and
// cannot use the direct Confirm path.
const ApprovalOperationType = "PRIVATE_ORDER_APPROVAL"

// ApprovalRequestInput is what this feature needs to raise an approval.
type ApprovalRequestInput struct {
	OrganizationID         string
	OrganizationUserID     string
	BranchID               string
	OrderID                string
	Payload                map[string]any
	ExpectedSubjectVersion int64
	IdempotencyKey         string
	CorrelationID          string
}

// ApprovalPort is the narrow slice of the WP9 approval framework this feature
// uses. There is no order-specific approval engine: quorum, eligible roles,
// separation of duties and expiry stay in the one framework that implements
// them.
type ApprovalPort interface {
	// Required reports whether the organization's policy requires private
	// orders to be approved before confirmation.
	Required(ctx context.Context, organizationID string) (bool, error)
	// Request raises an approval request. An empty ID with a nil error means
	// the policy did not require one.
	Request(ctx context.Context, input ApprovalRequestInput) (string, error)
	// RequireApproved fails unless the request is approved, unexpired and
	// bound to exactly this payload.
	RequireApproved(ctx context.Context, organizationID, requestID string, payload map[string]any) error
}

// DependentWorkPort withdraws an order's dependent work when the order is
// cancelled: not-yet-executing shipments are cancelled and pending provider
// offers are withdrawn. It must refuse (and so abort the cancellation) if a
// provider has already accepted work for the order.
type DependentWorkPort interface {
	CancelForOrder(ctx context.Context, tx pgx.Tx, t tenant.Context, orderID, reasonCode string) error
}

// WithApprovals binds the WP9 approval framework.
func (s *Service) WithApprovals(port ApprovalPort) *Service {
	s.opts.Approvals = port
	return s
}

// WithDependentWork binds the cancellation compensation port.
func (s *Service) WithDependentWork(port DependentWorkPort) *Service {
	s.opts.DependentWork = port
	return s
}

func (s *Service) approvalRequired(ctx context.Context, t tenant.Context) (bool, error) {
	if s.opts.Approvals == nil {
		return false, nil
	}
	return s.opts.Approvals.Required(ctx, t.OrganizationID)
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (s *Service) lockOrder(ctx context.Context, tx pgx.Tx, t tenant.Context, orderID string) (models.Order, error) {
	order, err := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, orderID)
	if err != nil {
		return models.Order{}, translateOrderNotFound(err)
	}
	if !t.BranchPermitted(order.BranchID) {
		return models.Order{}, orderNotFound()
	}
	return order, nil
}

// commitment is the validated state an order is committed against.
type commitment struct {
	customer CustomerInfo
	item     CatalogItemInfo
	stops    []models.OrderStop
}

// validateCommitment re-checks everything an order must satisfy before it
// can be submitted or confirmed: a committable customer with no blocking
// credit hold, an active catalogue item with its required fields, an active
// contract when one is referenced, and a contiguous, non-empty stop plan.
func (s *Service) validateCommitment(ctx context.Context, tx pgx.Tx, t tenant.Context,
	order models.Order) (commitment, error) {
	customer, err := s.requireActiveCustomer(ctx, tx, t, order.CustomerID)
	if err != nil {
		return commitment{}, err
	}
	item, err := s.requireActiveCatalogItem(ctx, tx, t, order.ServiceCatalogItemID)
	if err != nil {
		return commitment{}, err
	}
	if err := models.ValidateServiceFields(item.RequiredFieldsSchema, order.ServiceFields); err != nil {
		return commitment{}, errorcodes.UnprocessableEntity(errorcodes.OrderStopInvalid, err.Error())
	}
	if strings.TrimSpace(order.ContractID) != "" {
		at := s.opts.Now()
		if order.RequestedScheduleStart != nil {
			at = *order.RequestedScheduleStart
		}
		if _, err := s.requireActiveContract(ctx, tx, t, order.ContractID, customer.ID, at); err != nil {
			return commitment{}, err
		}
	}
	stops, err := s.opts.Repository.ListStops(ctx, tx, t.OrganizationID, order.ID)
	if err != nil {
		return commitment{}, err
	}
	if len(stops) == 0 {
		return commitment{}, errorcodes.UnprocessableEntity(errorcodes.OrderStopsRequired,
			"An order must have at least one stop before it can be confirmed.")
	}
	for i, stop := range stops {
		if stop.Sequence != i {
			return commitment{}, errorcodes.Conflict(errorcodes.OrderStopSequenceConflict,
				"Order stop sequence is not contiguous.")
		}
	}
	return commitment{customer: customer, item: item, stops: stops}, nil
}

// commitConfirmation freezes the snapshots and moves the order to CONFIRMED
// with history, audit and outbox rows. reason distinguishes an approval from
// a direct confirmation on the history row.
func (s *Service) commitConfirmation(ctx context.Context, tx pgx.Tx, t tenant.Context, order models.Order,
	expectedVersion int64, c commitment, reason string) (models.Order, error) {
	confirmationSnapshot := models.JSONMap{
		"customerId":          c.customer.ID,
		"customerLegalName":   c.customer.LegalName,
		"customerDisplayName": c.customer.DisplayName,
		"customerType":        c.customer.CustomerType,
		"stopCount":           len(c.stops),
	}
	if order.ApprovalRequestID != "" {
		confirmationSnapshot["approvalRequestId"] = order.ApprovalRequestID
	}
	confirmed, err := s.opts.Repository.Confirm(ctx, tx, t.OrganizationID, order.ID, expectedVersion,
		c.item.Snapshot, confirmationSnapshot, t.ActorID)
	if err != nil {
		return models.Order{}, concurrency.AsConflict(translateVersionErr(err))
	}
	if err = s.opts.Repository.AppendHistory(ctx, tx, models.OrderHistory{
		OrganizationID: t.OrganizationID, OrderID: order.ID, PreviousStatus: order.Status,
		NewStatus: models.StatusConfirmed, ReasonCode: reason, ActorType: string(t.ActorType),
		ActorID: t.ActorID, CorrelationID: t.CorrelationID,
	}); err != nil {
		return models.Order{}, err
	}
	action := "orders.order.confirmed"
	if reason == models.ReasonApproved {
		action = "orders.order.approved"
	}
	if err = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, action, "private_order", confirmed.ID)); err != nil {
		return models.Order{}, err
	}
	if s.opts.Events != nil {
		if reason == models.ReasonApproved {
			if err = s.opts.Events.Publish(ctx, tx, "organization.orders.order.approved.v1", "private_order",
				confirmed.ID, confirmed.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"orderId": confirmed.ID, "approvalRequestId": order.ApprovalRequestID}); err != nil {
				return models.Order{}, err
			}
		}
		if err = s.opts.Events.Publish(ctx, tx, "organization.orders.order.confirmed.v1", "private_order",
			confirmed.ID, confirmed.Version, t.OrganizationID, t.CorrelationID,
			map[string]any{"orderId": confirmed.ID}); err != nil {
			return models.Order{}, err
		}
	}
	return confirmed, nil
}

// contentFingerprint hashes what an approver is asked to approve: the
// order's commercial content and its stop plan. Any edit changes it, so an
// approval can never be spent on content it did not see.
func contentFingerprint(order models.Order, stops []models.OrderStop) string {
	stopFacts := make([]map[string]any, 0, len(stops))
	for _, stop := range stops {
		stopFacts = append(stopFacts, map[string]any{
			"id": stop.ID, "sequence": stop.Sequence, "type": stop.StopType,
			"latitude": stop.Latitude, "longitude": stop.Longitude,
			"windowStart": stop.TimeWindowStart, "windowEnd": stop.TimeWindowEnd,
			"address": map[string]any(stop.AddressSnapshot),
		})
	}
	encoded, _ := json.Marshal(map[string]any{
		"customerId": order.CustomerID, "serviceCatalogItemId": order.ServiceCatalogItemID,
		"serviceFields": map[string]any(order.ServiceFields), "contractId": order.ContractID,
		"quotationVersionId": order.QuotationVersionID, "customerReference": order.CustomerReference,
		"scheduleStart": order.RequestedScheduleStart, "scheduleEnd": order.RequestedScheduleEnd,
		"timezone": order.Timezone, "priority": order.Priority,
		"specialInstructions": order.SpecialInstructions, "stops": stopFacts,
	})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// approvalPayload is the exact controlled action an approval is bound to.
// orderVersion is the version the order holds while AWAITING_APPROVAL; any
// edit or resubmission moves it, so an old approval cannot be replayed.
func approvalPayload(order models.Order, stops []models.OrderStop, awaitingVersion int64) map[string]any {
	return map[string]any{
		"orderId":            order.ID,
		"orderNumber":        order.OrderNumber,
		"customerId":         order.CustomerID,
		"contentFingerprint": contentFingerprint(order, stops),
		"orderVersion":       awaitingVersion,
	}
}

// ---------------------------------------------------------------------------
// Update (PATCH)
// ---------------------------------------------------------------------------

// UpdateInput is a partial edit of an order's commercial content. Nil means
// unchanged. The customer, catalogue item, branch and source are identity,
// not content, and are not editable: changing them is a different order.
type UpdateInput struct {
	CustomerReference      *string
	ServiceFields          models.JSONMap
	RequestedScheduleStart *time.Time
	RequestedScheduleEnd   *time.Time
	ClearSchedule          bool
	Timezone               *string
	Priority               *string
	SpecialInstructions    *string
	ExpectedVersion        int64
}

// Update edits an order that is still DRAFT or AWAITING_APPROVAL.
//
// Editing an order under review returns it to DRAFT and detaches its
// approval request: the reviewer approved content that no longer exists, so
// the order must be submitted again.
func (s *Service) Update(ctx context.Context, t tenant.Context, orderID string, in UpdateInput) (models.Order, error) {
	if err := s.requireWritable(); err != nil {
		return models.Order{}, err
	}
	if in.ExpectedVersion <= 0 {
		return models.Order{}, apperrors.BadRequest("Provide the expected version via If-Match or expectedVersion.", nil)
	}
	if in.Timezone != nil && !validTimezone(*in.Timezone) {
		return models.Order{}, apperrors.Validation("timezone must be a valid IANA timezone.",
			[]apperrors.FieldViolation{{Field: "timezone", Message: "invalid"}})
	}
	if in.Priority != nil && !models.ValidPriority(*in.Priority) {
		return models.Order{}, apperrors.Validation("priority is not recognized.",
			[]apperrors.FieldViolation{{Field: "priority", Message: "unsupported"}})
	}

	var updated models.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, err := s.lockOrder(ctx, tx, t, orderID)
		if err != nil {
			return err
		}
		if existing.Version != in.ExpectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if !models.Editable(existing.Status) {
			return errorcodes.Conflict(errorcodes.OrderInvalidTransition,
				"Only a DRAFT or AWAITING_APPROVAL order can be edited.")
		}

		edited := existing
		if in.CustomerReference != nil {
			edited.CustomerReference = strings.TrimSpace(*in.CustomerReference)
		}
		if in.ServiceFields != nil {
			edited.ServiceFields = in.ServiceFields.Clone()
		}
		if in.ClearSchedule {
			edited.RequestedScheduleStart, edited.RequestedScheduleEnd = nil, nil
		}
		if in.RequestedScheduleStart != nil {
			edited.RequestedScheduleStart = in.RequestedScheduleStart
		}
		if in.RequestedScheduleEnd != nil {
			edited.RequestedScheduleEnd = in.RequestedScheduleEnd
		}
		if in.Timezone != nil {
			edited.Timezone = *in.Timezone
		}
		if in.Priority != nil {
			edited.Priority = *in.Priority
		}
		if in.SpecialInstructions != nil {
			edited.SpecialInstructions = strings.TrimSpace(*in.SpecialInstructions)
		}
		if edited.RequestedScheduleStart != nil && edited.RequestedScheduleEnd != nil &&
			edited.RequestedScheduleEnd.Before(*edited.RequestedScheduleStart) {
			return apperrors.Validation("requestedScheduleEnd must not precede requestedScheduleStart.",
				[]apperrors.FieldViolation{{Field: "requestedScheduleEnd", Message: "before_start"}})
		}
		if in.ServiceFields != nil {
			item, itemErr := s.requireActiveCatalogItem(ctx, tx, t, existing.ServiceCatalogItemID)
			if itemErr != nil {
				return itemErr
			}
			if fieldErr := models.ValidateServiceFields(item.RequiredFieldsSchema, edited.ServiceFields); fieldErr != nil {
				return errorcodes.UnprocessableEntity(errorcodes.OrderStopInvalid, fieldErr.Error())
			}
		}
		edited.UpdatedBy = t.ActorID

		returnToDraft := existing.Status == models.StatusAwaitingApproval
		updated, err = s.opts.Repository.UpdateFields(ctx, tx, t.OrganizationID, orderID, in.ExpectedVersion,
			edited, returnToDraft)
		if err != nil {
			return concurrency.AsConflict(translateVersionErr(err))
		}
		if returnToDraft {
			if err = s.opts.Repository.AppendHistory(ctx, tx, models.OrderHistory{
				OrganizationID: t.OrganizationID, OrderID: orderID, PreviousStatus: existing.Status,
				NewStatus: models.StatusDraft, ReasonCode: models.ReasonEditedDuringApproval,
				ActorType: string(t.ActorType), ActorID: t.ActorID, CorrelationID: t.CorrelationID,
			}); err != nil {
				return err
			}
		}
		entry := audit.EntryFromTenant(t, "orders.order.updated", "private_order", updated.ID)
		entry.PreviousValues = map[string]any{"status": existing.Status, "version": existing.Version}
		entry.NewValues = map[string]any{"status": updated.Status, "version": updated.Version}
		if err = s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "organization.orders.order.updated.v1", "private_order",
				updated.ID, updated.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"orderId": updated.ID, "status": updated.Status,
					"returnedToDraft": returnToDraft})
		}
		return nil
	})
	if err != nil {
		return models.Order{}, concurrency.AsConflict(err)
	}
	s.reindex(ctx, updated)
	return updated, nil
}

// ---------------------------------------------------------------------------
// Submit / Approve / Reject
// ---------------------------------------------------------------------------

// Submit moves a valid DRAFT order to AWAITING_APPROVAL.
//
// The order is fully validated first — a reviewer is never asked to approve
// an order that could not be confirmed anyway. When the organization's policy
// requires approval, a WP9 approval request bound to the order's exact
// content is raised in the same call and its ID recorded on the order.
func (s *Service) Submit(ctx context.Context, t tenant.Context, orderID string, expectedVersion int64,
	idempotencyKey string) (models.Order, error) {
	if err := s.requireWritable(); err != nil {
		return models.Order{}, err
	}
	if expectedVersion <= 0 {
		return models.Order{}, apperrors.BadRequest("Provide the expected version via If-Match or expectedVersion.", nil)
	}

	var submitted models.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := s.lockOrder(ctx, tx, t, orderID)
		if err != nil {
			return err
		}
		if order.Version != expectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if order.Status != models.StatusDraft {
			return errorcodes.Conflict(errorcodes.OrderInvalidTransition,
				"Only a DRAFT order can be submitted for approval.")
		}
		c, err := s.validateCommitment(ctx, tx, t, order)
		if err != nil {
			return err
		}
		approvalID := ""
		if s.opts.Approvals != nil {
			if strings.TrimSpace(idempotencyKey) == "" {
				// The approval framework replays by key; a deterministic key
				// per submission keeps a retried submit from raising twice.
				idempotencyKey = "order-submit:" + order.ID + ":" + strconv.FormatInt(order.Version, 10)
			}
			approvalID, err = s.opts.Approvals.Request(ctx, ApprovalRequestInput{
				OrganizationID: t.OrganizationID, OrganizationUserID: t.OrganizationUserID,
				BranchID: order.BranchID, OrderID: order.ID,
				Payload:                approvalPayload(order, c.stops, order.Version+1),
				ExpectedSubjectVersion: order.Version + 1,
				IdempotencyKey:         idempotencyKey, CorrelationID: t.CorrelationID,
			})
			if err != nil {
				return err
			}
		}
		submitted, err = s.opts.Repository.MarkAwaitingApproval(ctx, tx, t.OrganizationID, orderID,
			expectedVersion, approvalID, t.ActorID)
		if err != nil {
			return concurrency.AsConflict(translateVersionErr(err))
		}
		if err = s.opts.Repository.AppendHistory(ctx, tx, models.OrderHistory{
			OrganizationID: t.OrganizationID, OrderID: orderID, PreviousStatus: order.Status,
			NewStatus: models.StatusAwaitingApproval, ReasonCode: models.ReasonSubmittedForApproval,
			ActorType: string(t.ActorType), ActorID: t.ActorID, CorrelationID: t.CorrelationID,
		}); err != nil {
			return err
		}
		entry := audit.EntryFromTenant(t, "orders.order.submitted", "private_order", submitted.ID)
		entry.NewValues = map[string]any{"approvalRequestId": approvalID}
		if err = s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "organization.orders.order.submitted.v1", "private_order",
				submitted.ID, submitted.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"orderId": submitted.ID, "approvalRequestId": approvalID})
		}
		return nil
	})
	if err != nil {
		return models.Order{}, concurrency.AsConflict(err)
	}
	s.reindex(ctx, submitted)
	return submitted, nil
}

// Approve confirms an order that is AWAITING_APPROVAL.
//
// When a WP9 approval request is attached, the request must be approved,
// unexpired and bound to exactly the order's current content; quorum,
// eligible roles and separation of duties were enforced by the framework that
// decided it. Without one (no organization policy), the four-eyes rule is
// applied here: the person who submitted the order cannot approve it.
//
// Every confirmation prerequisite is re-validated: a customer placed on
// credit hold while the order sat in review is refused at approval.
func (s *Service) Approve(ctx context.Context, t tenant.Context, orderID string, expectedVersion int64) (models.Order, error) {
	if err := s.requireWritable(); err != nil {
		return models.Order{}, err
	}
	if expectedVersion <= 0 {
		return models.Order{}, apperrors.BadRequest("Provide the expected version via If-Match or expectedVersion.", nil)
	}

	var approved models.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := s.lockOrder(ctx, tx, t, orderID)
		if err != nil {
			return err
		}
		if order.Version != expectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if order.Status != models.StatusAwaitingApproval {
			return errorcodes.Conflict(errorcodes.OrderNotConfirmable,
				"Only an order awaiting approval can be approved.")
		}
		c, err := s.validateCommitment(ctx, tx, t, order)
		if err != nil {
			return err
		}
		if order.ApprovalRequestID != "" {
			if s.opts.Approvals == nil {
				return errorcodes.Unavailable(errorcodes.OrderApprovalRequired,
					"The approval service is unavailable; the order cannot be approved.")
			}
			if err = s.opts.Approvals.RequireApproved(ctx, t.OrganizationID, order.ApprovalRequestID,
				approvalPayload(order, c.stops, order.Version)); err != nil {
				return err
			}
		} else if order.UpdatedBy != "" && order.UpdatedBy == t.ActorID {
			return errorcodes.Forbidden(errorcodes.OrderApprovalRequired,
				"The person who submitted an order cannot approve it.")
		}
		approved, err = s.commitConfirmation(ctx, tx, t, order, expectedVersion, c, models.ReasonApproved)
		return err
	})
	if err != nil {
		return models.Order{}, concurrency.AsConflict(err)
	}
	s.reindex(ctx, approved)
	return approved, nil
}

// Reject returns an order under review to DRAFT with a recorded reason. The
// order is not killed: the commercial intent survives the review and may be
// corrected and resubmitted, or cancelled.
func (s *Service) Reject(ctx context.Context, t tenant.Context, orderID string, expectedVersion int64,
	reasonCode string) (models.Order, error) {
	if err := s.requireWritable(); err != nil {
		return models.Order{}, err
	}
	if expectedVersion <= 0 {
		return models.Order{}, apperrors.BadRequest("Provide the expected version via If-Match or expectedVersion.", nil)
	}
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		return models.Order{}, apperrors.Validation("reasonCode is required.",
			[]apperrors.FieldViolation{{Field: "reasonCode", Message: "required"}})
	}

	var rejected models.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := s.lockOrder(ctx, tx, t, orderID)
		if err != nil {
			return err
		}
		if order.Version != expectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if order.Status != models.StatusAwaitingApproval {
			return errorcodes.Conflict(errorcodes.OrderInvalidTransition,
				"Only an order awaiting approval can be rejected.")
		}
		rejected, err = s.opts.Repository.ReturnToDraft(ctx, tx, t.OrganizationID, orderID, expectedVersion, t.ActorID)
		if err != nil {
			return concurrency.AsConflict(translateVersionErr(err))
		}
		if err = s.opts.Repository.AppendHistory(ctx, tx, models.OrderHistory{
			OrganizationID: t.OrganizationID, OrderID: orderID, PreviousStatus: order.Status,
			NewStatus: models.StatusDraft, ReasonCode: reasonCode, ActorType: string(t.ActorType),
			ActorID: t.ActorID, CorrelationID: t.CorrelationID,
		}); err != nil {
			return err
		}
		entry := audit.EntryFromTenant(t, "orders.order.rejected", "private_order", rejected.ID)
		entry.Reason = reasonCode
		if err = s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "organization.orders.order.rejected.v1", "private_order",
				rejected.ID, rejected.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"orderId": rejected.ID, "reasonCode": reasonCode,
					"approvalRequestId": order.ApprovalRequestID})
		}
		return nil
	})
	if err != nil {
		return models.Order{}, concurrency.AsConflict(err)
	}
	s.reindex(ctx, rejected)
	return rejected, nil
}
