// Package usecases implements the Section 15 shipment and cargo item
// workflows.
//
// A shipment is a unit of work inside a private order; creating or editing
// one starts no execution, reserves no capacity and moves no money. This
// package depends on Section 15 private orders only through the narrow
// OrdersPort interface below — never by importing orders/repositories
// directly — matching the CRMPort/CatalogPort pattern the orders feature
// itself established.
package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ErrPortNotFound is returned by OrdersPort methods when the referenced
// order does not exist inside the organization.
var ErrPortNotFound = errors.New("shipments: referenced record not found")

// EventPublisher appends corporate domain events to the outbox.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string,
		aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

// OrderInfo is this feature's own projection of a Section 15 private order.
type OrderInfo struct {
	ID        string
	BranchID  string
	Confirmed bool // true once the order has passed CONFIRMED
}

// OrdersPort is the narrow slice of Section 15 private orders this feature
// depends on.
type OrdersPort interface {
	GetOrder(ctx context.Context, q database.Querier, organizationID, orderID string) (OrderInfo, error)
	// ListOrderStopIDs returns the order's stop IDs, so a shipment's origin
	// and destination can be checked against the order they actually belong
	// to, not just any stop that happens to exist.
	ListOrderStopIDs(ctx context.Context, q database.Querier, organizationID, orderID string) ([]string, error)
}

// Flags gates the Section 15 shipment read and write surfaces.
type Flags struct {
	Enabled       bool
	WritesEnabled bool
	KillSwitch    bool
}

// Options configure the shipments service.
type Options struct {
	Pool       *pgxpool.Pool
	Repository *repositories.Repository
	Orders     OrdersPort
	Events     EventPublisher
	Audit      *audit.Recorder
	Metrics    *observability.Metrics
	Flags      Flags
	Now        func() time.Time
}

// Service implements the Section 15 shipment and cargo item workflows.
type Service struct{ opts Options }

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	if opts.Audit == nil {
		opts.Audit = audit.NewRecorder()
	}
	return &Service{opts: opts}
}

func (s *Service) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.opts.Pool, fn)
}

func shipmentNotFound() error {
	return errorcodes.NotFound(errorcodes.ShipmentNotFound, "Shipment not found.")
}

func translateShipmentNotFound(err error) error {
	if errors.Is(err, repositories.ErrNotFound) {
		return shipmentNotFound()
	}
	return err
}

func translateVersionErr(err error) error {
	if errors.Is(err, repositories.ErrVersionConflict) {
		return concurrency.ErrVersionMismatch
	}
	return err
}

func (s *Service) requireEnabled() error {
	if s.opts.Flags.KillSwitch {
		return errorcodes.Unavailable(errorcodes.ShipmentInvalidState, "Shipments are temporarily unavailable.")
	}
	if !s.opts.Flags.Enabled {
		return shipmentNotFound()
	}
	return nil
}

func (s *Service) requireWritable() error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
	if !s.opts.Flags.WritesEnabled {
		return errorcodes.Forbidden(errorcodes.ShipmentInvalidState, "Shipment writes are not enabled for this organization.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// ListInput is the allowlisted shipment list query.
type ListInput struct {
	Status   string
	OrderID  string
	BranchID string
	Cursor   string
	Limit    int
}

// List returns a tenant-scoped page of shipments, newest first.
func (s *Service) List(ctx context.Context, t tenant.Context, in ListInput) ([]models.Shipment, string, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, "", err
	}
	decoded, hasCursor, err := cursor.Decode(in.Cursor)
	if err != nil {
		return nil, "", apperrors.BadRequest("cursor is malformed.", err)
	}
	shipments, err := s.opts.Repository.List(ctx, t.OrganizationID, repositories.Filter{
		Status: in.Status, OrderID: in.OrderID, BranchID: in.BranchID,
		Cursor: decoded, HasCursor: hasCursor, Limit: in.Limit,
	})
	if err != nil {
		return nil, "", err
	}
	var next string
	if len(shipments) > 0 {
		last := shipments[len(shipments)-1]
		next = cursor.EncodeTime(last.CreatedAt, last.ID)
	}
	return shipments, next, nil
}

// Get reads one shipment, refusing a cross-branch read the same way it
// refuses a missing one.
func (s *Service) Get(ctx context.Context, t tenant.Context, shipmentID string) (models.Shipment, error) {
	if err := s.requireEnabled(); err != nil {
		return models.Shipment{}, err
	}
	shipment, err := s.opts.Repository.Get(ctx, nil, t.OrganizationID, shipmentID)
	if err != nil {
		return models.Shipment{}, translateShipmentNotFound(err)
	}
	if !t.BranchPermitted(shipment.BranchID) {
		return models.Shipment{}, shipmentNotFound()
	}
	return shipment, nil
}

// ListCargoItems returns a shipment's cargo items.
func (s *Service) ListCargoItems(ctx context.Context, t tenant.Context, shipmentID string) ([]models.CargoItem, error) {
	if _, err := s.Get(ctx, t, shipmentID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListCargoItems(ctx, nil, t.OrganizationID, shipmentID)
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateInput is the validated shape of a new shipment.
type CreateInput struct {
	BranchID                  string
	OrderID                   string
	OriginStopID              string
	DestinationStopID         string
	CargoProfile              models.JSONMap
	PassengerProfile          models.JSONMap
	RequiredVehicleClass      string
	RequiredServiceCapability string
	TemperatureRequirements   models.JSONMap
	DeclaredValueAmountMinor  int64
	DeclaredValueCurrency     string
}

func (in CreateInput) validate() error {
	if strings.TrimSpace(in.OrderID) == "" {
		return apperrors.Validation("orderId is required.", []apperrors.FieldViolation{{Field: "orderId", Message: "required"}})
	}
	if strings.TrimSpace(in.OriginStopID) == "" || strings.TrimSpace(in.DestinationStopID) == "" {
		return apperrors.Validation("originStopId and destinationStopId are required.",
			[]apperrors.FieldViolation{{Field: "originStopId", Message: "required"}})
	}
	if in.OriginStopID == in.DestinationStopID {
		return errorcodes.UnprocessableEntity(errorcodes.ShipmentDestinationInvalid,
			"Origin and destination stops must be different.")
	}
	if in.DeclaredValueAmountMinor < 0 {
		return apperrors.Validation("declaredValueAmountMinor must not be negative.",
			[]apperrors.FieldViolation{{Field: "declaredValueAmountMinor", Message: "negative"}})
	}
	if !models.ValidateCurrency(in.DeclaredValueCurrency) {
		return apperrors.Validation("declaredValueCurrency must be a three-letter ISO 4217 code.",
			[]apperrors.FieldViolation{{Field: "declaredValueCurrency", Message: "invalid"}})
	}
	return nil
}

// Create adds a new DRAFT shipment attached to a confirmed private order.
func (s *Service) Create(ctx context.Context, t tenant.Context, in CreateInput) (models.Shipment, error) {
	if err := s.requireWritable(); err != nil {
		return models.Shipment{}, err
	}
	if err := in.validate(); err != nil {
		return models.Shipment{}, err
	}

	order, err := s.opts.Orders.GetOrder(ctx, nil, t.OrganizationID, in.OrderID)
	if err != nil {
		if errors.Is(err, ErrPortNotFound) {
			return models.Shipment{}, errorcodes.NotFound(errorcodes.OrderNotFound, "Order not found.")
		}
		return models.Shipment{}, err
	}
	if !t.BranchPermitted(order.BranchID) {
		return models.Shipment{}, errorcodes.NotFound(errorcodes.OrderNotFound, "Order not found.")
	}
	if !order.Confirmed {
		return models.Shipment{}, errorcodes.Conflict(errorcodes.ShipmentInvalidState,
			"A shipment can only be created for a confirmed order.")
	}

	stopIDs, err := s.opts.Orders.ListOrderStopIDs(ctx, nil, t.OrganizationID, in.OrderID)
	if err != nil {
		return models.Shipment{}, err
	}
	stopSet := make(map[string]bool, len(stopIDs))
	for _, id := range stopIDs {
		stopSet[id] = true
	}
	if !stopSet[in.OriginStopID] {
		return models.Shipment{}, errorcodes.UnprocessableEntity(errorcodes.ShipmentOriginInvalid,
			"The origin stop does not belong to this order.")
	}
	if !stopSet[in.DestinationStopID] {
		return models.Shipment{}, errorcodes.UnprocessableEntity(errorcodes.ShipmentDestinationInvalid,
			"The destination stop does not belong to this order.")
	}

	shipment := models.Shipment{
		ID:                        ids.New(repositories.PrefixShipment),
		OrganizationID:            t.OrganizationID,
		BranchID:                  in.BranchID,
		OrderID:                   in.OrderID,
		OriginStopID:              in.OriginStopID,
		DestinationStopID:         in.DestinationStopID,
		CargoProfile:              in.CargoProfile.Clone(),
		PassengerProfile:          in.PassengerProfile.Clone(),
		RequiredVehicleClass:      strings.TrimSpace(in.RequiredVehicleClass),
		RequiredServiceCapability: strings.TrimSpace(in.RequiredServiceCapability),
		TemperatureRequirements:   in.TemperatureRequirements.Clone(),
		DeclaredValueAmountMinor:  in.DeclaredValueAmountMinor,
		DeclaredValueCurrency:     in.DeclaredValueCurrency,
		Status:                    models.ShipmentDraft,
		CreatedBy:                 t.ActorID,
	}

	var created models.Shipment
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		var txErr error
		created, txErr = s.opts.Repository.Create(ctx, tx, shipment)
		if txErr != nil {
			return txErr
		}
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "shipments.shipment.created",
			"shipment", created.ID)); auditErr != nil {
			return auditErr
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "organization.shipments.shipment.created.v1", "shipment",
				created.ID, created.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"shipmentId": created.ID, "orderId": created.OrderID})
		}
		return nil
	})
	if err != nil {
		return models.Shipment{}, err
	}
	return created, nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// UpdateInput is the validated shape of a shipment edit, reachable only
// while the shipment is still DRAFT.
type UpdateInput struct {
	CargoProfile              models.JSONMap
	PassengerProfile          models.JSONMap
	RequiredVehicleClass      string
	RequiredServiceCapability string
	TemperatureRequirements   models.JSONMap
	DeclaredValueAmountMinor  int64
	DeclaredValueCurrency     string
}

// Update edits the mutable fields of a shipment under optimistic concurrency.
// Once a shipment has moved past DRAFT its route, cargo profile and declared
// value are locked — the shipment has committed to what it is.
func (s *Service) Update(ctx context.Context, t tenant.Context, shipmentID string, expectedVersion int64,
	in UpdateInput) (models.Shipment, error) {
	if err := s.requireWritable(); err != nil {
		return models.Shipment{}, err
	}
	if expectedVersion <= 0 {
		return models.Shipment{}, apperrors.BadRequest("Provide the expected version via If-Match or expectedVersion.", nil)
	}
	if in.DeclaredValueAmountMinor < 0 {
		return models.Shipment{}, apperrors.Validation("declaredValueAmountMinor must not be negative.",
			[]apperrors.FieldViolation{{Field: "declaredValueAmountMinor", Message: "negative"}})
	}
	if !models.ValidateCurrency(in.DeclaredValueCurrency) {
		return models.Shipment{}, apperrors.Validation("declaredValueCurrency must be a three-letter ISO 4217 code.",
			[]apperrors.FieldViolation{{Field: "declaredValueCurrency", Message: "invalid"}})
	}

	var updated models.Shipment
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, err := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, shipmentID)
		if err != nil {
			return translateShipmentNotFound(err)
		}
		if !t.BranchPermitted(existing.BranchID) {
			return shipmentNotFound()
		}
		if models.ShipmentLocked(existing.Status) {
			return errorcodes.Conflict(errorcodes.ShipmentLockedCode,
				"This shipment has moved past DRAFT and can no longer be edited directly.")
		}

		edited := existing
		edited.CargoProfile = in.CargoProfile.Clone()
		edited.PassengerProfile = in.PassengerProfile.Clone()
		edited.RequiredVehicleClass = strings.TrimSpace(in.RequiredVehicleClass)
		edited.RequiredServiceCapability = strings.TrimSpace(in.RequiredServiceCapability)
		edited.TemperatureRequirements = in.TemperatureRequirements.Clone()
		edited.DeclaredValueAmountMinor = in.DeclaredValueAmountMinor
		edited.DeclaredValueCurrency = in.DeclaredValueCurrency
		edited.UpdatedBy = t.ActorID

		updated, err = s.opts.Repository.UpdateFields(ctx, tx, t.OrganizationID, shipmentID, expectedVersion, edited)
		if err != nil {
			return concurrency.AsConflict(translateVersionErr(err))
		}
		return s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "shipments.shipment.updated",
			"shipment", updated.ID))
	})
	if err != nil {
		return models.Shipment{}, err
	}
	return updated, nil
}

// ---------------------------------------------------------------------------
// Cargo items
// ---------------------------------------------------------------------------

// AddCargoItemInput is the validated shape of a new cargo item.
type AddCargoItemInput struct {
	Description              string
	Quantity                 string
	Unit                     string
	HandlingCategory         string
	HazardousClassification  string
	TemperatureMin           *string
	TemperatureMax           *string
	TemperatureUnit          string
	DeclaredValueAmountMinor int64
	DeclaredValueCurrency    string
}

// AddCargoItem attaches a cargo item to a still-editable shipment.
//
// The restricted-item status is never caller-settable: it is computed
// server-side from the handling category alone, so a client cannot describe
// dangerous goods as "not restricted" in free text and have that accepted.
func (s *Service) AddCargoItem(ctx context.Context, t tenant.Context, shipmentID string,
	in AddCargoItemInput) (models.CargoItem, error) {
	if err := s.requireWritable(); err != nil {
		return models.CargoItem{}, err
	}
	description := strings.TrimSpace(in.Description)
	if description == "" {
		return models.CargoItem{}, apperrors.Validation("description is required.",
			[]apperrors.FieldViolation{{Field: "description", Message: "required"}})
	}
	if err := models.ValidateQuantity(in.Quantity); err != nil {
		return models.CargoItem{}, apperrors.Validation("quantity must be a positive decimal.",
			[]apperrors.FieldViolation{{Field: "quantity", Message: "invalid"}})
	}
	if strings.TrimSpace(in.Unit) == "" {
		return models.CargoItem{}, apperrors.Validation("unit is required.",
			[]apperrors.FieldViolation{{Field: "unit", Message: "required"}})
	}
	category := in.HandlingCategory
	if category == "" {
		category = models.HandlingGeneral
	}
	if !models.ValidHandlingCategory(category) {
		return models.CargoItem{}, apperrors.Validation("handlingCategory is not recognized.",
			[]apperrors.FieldViolation{{Field: "handlingCategory", Message: "unsupported"}})
	}
	if in.TemperatureMin != nil && in.TemperatureMax != nil && *in.TemperatureMax < *in.TemperatureMin {
		return models.CargoItem{}, apperrors.Validation("temperatureMax must not be below temperatureMin.",
			[]apperrors.FieldViolation{{Field: "temperatureMax", Message: "below_min"}})
	}
	if in.DeclaredValueAmountMinor < 0 {
		return models.CargoItem{}, apperrors.Validation("declaredValueAmountMinor must not be negative.",
			[]apperrors.FieldViolation{{Field: "declaredValueAmountMinor", Message: "negative"}})
	}
	if !models.ValidateCurrency(in.DeclaredValueCurrency) {
		return models.CargoItem{}, apperrors.Validation("declaredValueCurrency must be a three-letter ISO 4217 code.",
			[]apperrors.FieldViolation{{Field: "declaredValueCurrency", Message: "invalid"}})
	}

	restrictedStatus := models.RestrictionNotRestricted
	if models.RequiresComplianceReview(category) {
		restrictedStatus = models.RestrictionRequiresReview
	}

	var created models.CargoItem
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		shipment, err := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, shipmentID)
		if err != nil {
			return translateShipmentNotFound(err)
		}
		if !t.BranchPermitted(shipment.BranchID) {
			return shipmentNotFound()
		}
		if !models.ShipmentEditable(shipment.Status) {
			return errorcodes.Conflict(errorcodes.ShipmentLockedCode,
				"Cargo can only be added while the shipment is still editable.")
		}

		item := models.CargoItem{
			ID: ids.New(repositories.PrefixCargo), OrganizationID: t.OrganizationID, ShipmentID: shipmentID,
			Description: description, Quantity: in.Quantity, Unit: strings.TrimSpace(in.Unit),
			HandlingCategory: category, HazardousClassification: strings.TrimSpace(in.HazardousClassification),
			TemperatureMin: in.TemperatureMin, TemperatureMax: in.TemperatureMax,
			TemperatureUnit:          strings.TrimSpace(in.TemperatureUnit),
			DeclaredValueAmountMinor: in.DeclaredValueAmountMinor, DeclaredValueCurrency: in.DeclaredValueCurrency,
			RestrictedItemStatus: restrictedStatus, CreatedBy: t.ActorID,
		}
		created, err = s.opts.Repository.CreateCargoItem(ctx, tx, item)
		if err != nil {
			return err
		}
		auditAction := "shipments.cargo_item.created"
		if restrictedStatus == models.RestrictionRequiresReview {
			auditAction = "shipments.cargo_item.compliance_review_required"
		}
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, auditAction,
			"cargo_item", created.ID)); auditErr != nil {
			return auditErr
		}
		if s.opts.Events != nil {
			eventType := "organization.shipments.cargo_item.created.v1"
			if restrictedStatus == models.RestrictionRequiresReview {
				eventType = "organization.shipments.cargo_item.compliance_required.v1"
			}
			return s.opts.Events.Publish(ctx, tx, eventType, "cargo_item", created.ID, created.Version,
				t.OrganizationID, t.CorrelationID,
				map[string]any{"cargoItemId": created.ID, "shipmentId": shipmentID, "handlingCategory": category})
		}
		return nil
	})
	if err != nil {
		return models.CargoItem{}, err
	}
	return created, nil
}
