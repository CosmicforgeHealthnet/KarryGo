// Package repositories implements PostgreSQL persistence for Section 15
// private orders and order stops.
//
// Every read and every write carries organization_id in SQL; a handler check
// is a convenience, the WHERE clause is the control.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Errors returned by the orders repository.
var (
	ErrNotFound        = errors.New("orders: record not found")
	ErrVersionConflict = errors.New("orders: expected version does not match")
	ErrDuplicate       = errors.New("orders: conflicting record")
)

// ID prefixes for Section 15 order records.
const (
	PrefixOrder   = "ord"
	PrefixStop    = "ost"
	PrefixHistory = "ohs"
)

// Repository persists Section 15 private orders and order stops.
type Repository struct{ db database.Querier }

// NewRepository builds the repository.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.db
	}
	return q
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

func classify(err error, operation string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case isUniqueViolation(err):
		return ErrDuplicate
	default:
		return fmt.Errorf("orders: %s: %w", operation, err)
	}
}

func scanErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func encodeJSONObject(value models.JSONMap) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	encoded, err := json.Marshal(map[string]any(value))
	if err != nil {
		return nil, fmt.Errorf("orders: encode json: %w", err)
	}
	return encoded, nil
}

func decodeJSONObject(raw []byte) models.JSONMap {
	if len(raw) == 0 {
		return models.JSONMap{}
	}
	var out models.JSONMap
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return models.JSONMap{}
	}
	return out
}

// ---------------------------------------------------------------------------
// Orders
// ---------------------------------------------------------------------------

const orderColumns = `id, organization_id, COALESCE(branch_id, ''), order_number, customer_id,
	customer_reference, source, source_reference, service_catalog_item_id, service_catalog_snapshot,
	service_fields,
	COALESCE(contract_id, ''), COALESCE(contract_version_id, ''), COALESCE(quotation_id, ''),
	COALESCE(quotation_version_id, ''), COALESCE(conversion_intent_id, ''), requested_schedule_start,
	requested_schedule_end, timezone, priority, special_instructions, status, confirmation_snapshot,
	COALESCE(approval_request_id, ''), cancel_reason_code, version, created_by, updated_by,
	created_at, updated_at, confirmed_at, cancelled_at, completed_at`

func scanOrder(row pgx.Row) (models.Order, error) {
	var o models.Order
	var catalogSnapshotRaw, serviceFieldsRaw, confirmationSnapshotRaw []byte
	err := row.Scan(&o.ID, &o.OrganizationID, &o.BranchID, &o.OrderNumber, &o.CustomerID,
		&o.CustomerReference, &o.Source, &o.SourceReference, &o.ServiceCatalogItemID,
		&catalogSnapshotRaw, &serviceFieldsRaw, &o.ContractID, &o.ContractVersionID, &o.QuotationID,
		&o.QuotationVersionID, &o.ConversionIntentID, &o.RequestedScheduleStart,
		&o.RequestedScheduleEnd, &o.Timezone, &o.Priority, &o.SpecialInstructions, &o.Status,
		&confirmationSnapshotRaw, &o.ApprovalRequestID, &o.CancelReasonCode, &o.Version,
		&o.CreatedBy, &o.UpdatedBy, &o.CreatedAt, &o.UpdatedAt, &o.ConfirmedAt, &o.CancelledAt,
		&o.CompletedAt)
	if err != nil {
		return o, scanErr(err)
	}
	o.ServiceCatalogSnapshot = decodeJSONObject(catalogSnapshotRaw)
	o.ServiceFields = decodeJSONObject(serviceFieldsRaw)
	o.ConfirmationSnapshot = decodeJSONObject(confirmationSnapshotRaw)
	return o, nil
}

// Create inserts a new DRAFT order.
func (r *Repository) Create(ctx context.Context, q database.Querier, o models.Order) (models.Order, error) {
	catalogSnapshot, err := encodeJSONObject(o.ServiceCatalogSnapshot)
	if err != nil {
		return models.Order{}, err
	}
	serviceFields, err := encodeJSONObject(o.ServiceFields)
	if err != nil {
		return models.Order{}, err
	}

	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_private_orders
			(id, organization_id, branch_id, order_number, customer_id, customer_reference,
			 source, source_reference, service_catalog_item_id, service_catalog_snapshot,
			 service_fields, contract_id, contract_version_id, quotation_id, quotation_version_id,
			 conversion_intent_id, requested_schedule_start, requested_schedule_end, timezone,
			 priority, special_instructions, status, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23, $23)
		RETURNING `+orderColumns,
		o.ID, o.OrganizationID, nullable(o.BranchID), o.OrderNumber, o.CustomerID,
		o.CustomerReference, o.Source, o.SourceReference, o.ServiceCatalogItemID, catalogSnapshot,
		serviceFields, nullable(o.ContractID), nullable(o.ContractVersionID), nullable(o.QuotationID),
		nullable(o.QuotationVersionID), nullable(o.ConversionIntentID), o.RequestedScheduleStart,
		o.RequestedScheduleEnd, o.Timezone, o.Priority, o.SpecialInstructions, o.Status,
		o.CreatedBy)
	created, err := scanOrder(row)
	if err != nil {
		return created, classify(err, "create order")
	}
	return created, nil
}

// Get reads one order inside the organization.
func (r *Repository) Get(ctx context.Context, q database.Querier, organizationID, orderID string) (models.Order, error) {
	return scanOrder(r.q(q).QueryRow(ctx, `SELECT `+orderColumns+`
		FROM fleet_partner_private_orders WHERE organization_id = $1 AND id = $2`,
		organizationID, orderID))
}

// Lock reads one order FOR UPDATE, serializing concurrent transitions.
func (r *Repository) Lock(ctx context.Context, q database.Querier, organizationID, orderID string) (models.Order, error) {
	return scanOrder(r.q(q).QueryRow(ctx, `SELECT `+orderColumns+`
		FROM fleet_partner_private_orders WHERE organization_id = $1 AND id = $2
		FOR UPDATE`, organizationID, orderID))
}

// SetStatus applies one version-checked lifecycle transition. Callers must
// validate the transition with the order state machine before invoking it.
func (r *Repository) SetStatus(ctx context.Context, q database.Querier, organizationID,
	orderID string, expectedVersion int64, status, actorID string) (models.Order, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_private_orders SET status=$1, updated_by=$2,
			version=version+1, updated_at=now(),
			completed_at=CASE WHEN $1='COMPLETED' THEN now() ELSE completed_at END,
			cancelled_at=CASE WHEN $1='CANCELLED' THEN now() ELSE cancelled_at END
		WHERE organization_id=$3 AND id=$4 AND version=$5
		RETURNING `+orderColumns,
		status, actorID, organizationID, orderID, expectedVersion)
	updated, err := scanOrder(row)
	if errors.Is(err, ErrNotFound) {
		return updated, ErrVersionConflict
	}
	if err != nil {
		return updated, classify(err, "set order status")
	}
	return updated, nil
}

// GetByConversionIntent finds the order a consumed intent produced, which is
// what makes a repeated conversion attempt an idempotent replay.
func (r *Repository) GetByConversionIntent(ctx context.Context, q database.Querier,
	organizationID, conversionIntentID string) (models.Order, error) {
	return scanOrder(r.q(q).QueryRow(ctx, `SELECT `+orderColumns+`
		FROM fleet_partner_private_orders WHERE organization_id = $1 AND conversion_intent_id = $2`,
		organizationID, conversionIntentID))
}

// Filter is the allowlisted order list query.
type Filter struct {
	Status     string
	CustomerID string
	BranchID   string
	Cursor     cursor.Cursor
	HasCursor  bool
	Limit      int
}

// List returns a tenant-scoped page, newest first.
func (r *Repository) List(ctx context.Context, organizationID string, filter Filter) ([]models.Order, error) {
	args := []any{organizationID, filter.Status, filter.CustomerID, filter.BranchID}
	where := `organization_id = $1
		AND ($2 = '' OR status = $2)
		AND ($3 = '' OR customer_id = $3)
		AND ($4 = '' OR branch_id = $4)`

	if filter.HasCursor {
		args = append(args, filter.Cursor.SortTime(), filter.Cursor.LastID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	limit := cursor.ClampLimit(filter.Limit)
	args = append(args, limit)

	rows, err := r.db.Query(ctx, `SELECT `+orderColumns+`
		FROM fleet_partner_private_orders
		WHERE `+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprintf("%d", len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("orders: list: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("orders: list scan: %w", err)
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

// MarkAwaitingApproval records that confirmation requires approval.
func (r *Repository) MarkAwaitingApproval(ctx context.Context, q database.Querier, organizationID, orderID string,
	expectedVersion int64, approvalRequestID, actorID string) (models.Order, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_private_orders SET
			status = $1, approval_request_id = $2, updated_by = $3, version = version + 1, updated_at = now()
		WHERE organization_id = $4 AND id = $5 AND version = $6
		RETURNING `+orderColumns,
		models.StatusAwaitingApproval, approvalRequestID, actorID, organizationID, orderID, expectedVersion)
	updated, err := scanOrder(row)
	if err != nil {
		if errors.Is(scanErr(err), ErrNotFound) {
			return updated, ErrVersionConflict
		}
		return updated, classify(err, "mark order awaiting approval")
	}
	return updated, nil
}

// Confirm freezes the catalogue and confirmation snapshots and moves the
// order to CONFIRMED.
func (r *Repository) Confirm(ctx context.Context, q database.Querier, organizationID, orderID string,
	expectedVersion int64, catalogSnapshot, confirmationSnapshot models.JSONMap, actorID string) (models.Order, error) {
	catalog, err := encodeJSONObject(catalogSnapshot)
	if err != nil {
		return models.Order{}, err
	}
	confirmation, err := encodeJSONObject(confirmationSnapshot)
	if err != nil {
		return models.Order{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_private_orders SET
			status = $1, service_catalog_snapshot = $2, confirmation_snapshot = $3,
			updated_by = $4, version = version + 1, updated_at = now(), confirmed_at = now()
		WHERE organization_id = $5 AND id = $6 AND version = $7
		RETURNING `+orderColumns,
		models.StatusConfirmed, catalog, confirmation, actorID, organizationID, orderID, expectedVersion)
	updated, err := scanOrder(row)
	if err != nil {
		if errors.Is(scanErr(err), ErrNotFound) {
			return updated, ErrVersionConflict
		}
		return updated, classify(err, "confirm order")
	}
	return updated, nil
}

// Cancel moves the order to CANCELLED.
func (r *Repository) Cancel(ctx context.Context, q database.Querier, organizationID, orderID string,
	expectedVersion int64, reasonCode, actorID string) (models.Order, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_private_orders SET
			status = $1, cancel_reason_code = $2, updated_by = $3, version = version + 1,
			updated_at = now(), cancelled_at = now()
		WHERE organization_id = $4 AND id = $5 AND version = $6
		RETURNING `+orderColumns,
		models.StatusCancelled, reasonCode, actorID, organizationID, orderID, expectedVersion)
	updated, err := scanOrder(row)
	if err != nil {
		if errors.Is(scanErr(err), ErrNotFound) {
			return updated, ErrVersionConflict
		}
		return updated, classify(err, "cancel order")
	}
	return updated, nil
}

// AppendHistory writes one immutable order status change.
func (r *Repository) AppendHistory(ctx context.Context, q database.Querier, h models.OrderHistory) error {
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_private_order_history
			(id, organization_id, order_id, previous_status, new_status, reason_code,
			 actor_type, actor_id, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		ids.New(PrefixHistory), h.OrganizationID, h.OrderID, h.PreviousStatus, h.NewStatus,
		h.ReasonCode, h.ActorType, h.ActorID, h.CorrelationID)
	if err != nil {
		return fmt.Errorf("orders: append history: %w", err)
	}
	return nil
}

// ListHistory returns the order's immutable history, oldest first.
func (r *Repository) ListHistory(ctx context.Context, organizationID, orderID string) ([]models.OrderHistory, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, order_id, previous_status, new_status, reason_code,
			actor_type, actor_id, correlation_id, occurred_at
		FROM fleet_partner_private_order_history
		WHERE organization_id = $1 AND order_id = $2
		ORDER BY occurred_at ASC`, organizationID, orderID)
	if err != nil {
		return nil, fmt.Errorf("orders: list history: %w", err)
	}
	defer rows.Close()

	var history []models.OrderHistory
	for rows.Next() {
		var h models.OrderHistory
		if err := rows.Scan(&h.ID, &h.OrganizationID, &h.OrderID, &h.PreviousStatus, &h.NewStatus,
			&h.ReasonCode, &h.ActorType, &h.ActorID, &h.CorrelationID, &h.OccurredAt); err != nil {
			return nil, fmt.Errorf("orders: list history scan: %w", err)
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

// ---------------------------------------------------------------------------
// Order stops
// ---------------------------------------------------------------------------

const stopColumns = `id, organization_id, order_id, sequence, stop_type, COALESCE(customer_location_id, ''),
	address_snapshot, latitude, longitude, time_window_start, time_window_end, timezone,
	contact_snapshot, instructions, status, version, created_at, updated_at`

func scanStop(row pgx.Row) (models.OrderStop, error) {
	var stop models.OrderStop
	var addressRaw, contactRaw []byte
	err := row.Scan(&stop.ID, &stop.OrganizationID, &stop.OrderID, &stop.Sequence, &stop.StopType,
		&stop.CustomerLocationID, &addressRaw, &stop.Latitude, &stop.Longitude,
		&stop.TimeWindowStart, &stop.TimeWindowEnd, &stop.Timezone, &contactRaw,
		&stop.Instructions, &stop.Status, &stop.Version, &stop.CreatedAt, &stop.UpdatedAt)
	if err != nil {
		return stop, scanErr(err)
	}
	stop.AddressSnapshot = decodeJSONObject(addressRaw)
	stop.ContactSnapshot = decodeJSONObject(contactRaw)
	return stop, nil
}

// GetStop reads one organization-owned stop by its opaque identifier. This
// narrow additive read supports composition-root adapters without allowing a
// sibling feature to query the orders table directly.
func (r *Repository) GetStop(ctx context.Context, q database.Querier, organizationID, stopID string) (models.OrderStop, error) {
	return scanStop(r.q(q).QueryRow(ctx, `SELECT `+stopColumns+`
		FROM fleet_partner_order_stops WHERE organization_id = $1 AND id = $2`, organizationID, stopID))
}

// CreateStop inserts one order stop.
func (r *Repository) CreateStop(ctx context.Context, q database.Querier, stop models.OrderStop) (models.OrderStop, error) {
	address, err := encodeJSONObject(stop.AddressSnapshot)
	if err != nil {
		return models.OrderStop{}, err
	}
	contact, err := encodeJSONObject(stop.ContactSnapshot)
	if err != nil {
		return models.OrderStop{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_order_stops
			(id, organization_id, order_id, sequence, stop_type, customer_location_id,
			 address_snapshot, latitude, longitude, time_window_start, time_window_end,
			 timezone, contact_snapshot, instructions, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING `+stopColumns,
		stop.ID, stop.OrganizationID, stop.OrderID, stop.Sequence, stop.StopType,
		nullable(stop.CustomerLocationID), address, stop.Latitude, stop.Longitude,
		stop.TimeWindowStart, stop.TimeWindowEnd, stop.Timezone, contact, stop.Instructions,
		stop.Status)
	created, err := scanStop(row)
	if err != nil {
		return created, classify(err, "create order stop")
	}
	return created, nil
}

// ListStops returns an order's stops in sequence order.
func (r *Repository) ListStops(ctx context.Context, q database.Querier, organizationID, orderID string) ([]models.OrderStop, error) {
	rows, err := r.q(q).Query(ctx, `SELECT `+stopColumns+`
		FROM fleet_partner_order_stops WHERE organization_id = $1 AND order_id = $2
		ORDER BY sequence ASC`, organizationID, orderID)
	if err != nil {
		return nil, fmt.Errorf("orders: list stops: %w", err)
	}
	defer rows.Close()

	var stops []models.OrderStop
	for rows.Next() {
		stop, err := scanStop(rows)
		if err != nil {
			return nil, fmt.Errorf("orders: list stops scan: %w", err)
		}
		stops = append(stops, stop)
	}
	return stops, rows.Err()
}
