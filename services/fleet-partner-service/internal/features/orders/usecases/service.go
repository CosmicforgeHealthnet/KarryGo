// Package usecases implements the Section 15 private order workflows.
//
// A private order is company-owned work — manual, converted from an accepted
// Section 14 quotation, generated from a contract or a recurring template, or
// bulk-imported — and is never a marketplace request. Confirming an order
// creates no assignment, canonical trip or wallet movement here: that begins
// only once a later Section 15 slice (private execution orchestration) wires
// a confirmed order forward. Nothing in this package invents pricing, a fare
// formula or a commission.
//
// This package depends on Section 14 CRM and the Section 15 service
// catalogue only through the narrow CRMPort and CatalogPort interfaces below
// — never by importing crm/repositories or servicecatalog/repositories
// directly. That is not a style preference: feature packages import only
// interfaces of other features, and the composition root (internal/app)
// supplies the real adapters, matching how CRM itself depends on Media,
// Notifications and Approvals through ports rather than concrete packages.
//
// Approval-policy evaluation for confirmation is intentionally not wired in
// this slice: AWAITING_APPROVAL exists in the state machine for forward
// compatibility, but no usecase here transitions an order into it. Wiring the
// existing WP9 approval framework (the same one CRM binds through
// ApprovalPort) is a follow-up, not a placeholder invented here.
package usecases

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ErrPortNotFound is returned by CRMPort/CatalogPort methods when the
// referenced record does not exist inside the organization. It is one
// sentinel for every port method because the caller always knows from
// context what it was looking up.
var ErrPortNotFound = errors.New("orders: referenced record not found")

// EventPublisher appends corporate domain events to the outbox.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string,
		aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

// ---------------------------------------------------------------------------
// Ports: Section 14 CRM and Section 15 service catalogue, as this feature
// needs them and nothing more.
// ---------------------------------------------------------------------------

// CustomerInfo is this feature's own projection of a Section 14 CRM customer
// — just enough to validate and confirm an order against.
type CustomerInfo struct {
	ID            string
	BranchID      string
	LegalName     string
	DisplayName   string
	CustomerType  string
	Blocked       bool
	BlockedReason string // "customer_archived" | "customer_risk_restricted" | "customer_on_hold"
}

// ContractInfo is this feature's own projection of a Section 14 contract.
type ContractInfo struct {
	ID              string
	BranchID        string
	CustomerID      string
	ActiveVersionID string
	Active          bool
	EffectiveFrom   time.Time
	EffectiveTo     *time.Time
}

// QuotationInfo is this feature's own projection of a Section 14 quotation.
type QuotationInfo struct {
	ID                string
	CustomerID        string
	AcceptedVersionID string
}

// ConversionIntentInfo is this feature's own projection of a Section 14
// quotation-acceptance conversion intent.
type ConversionIntentInfo struct {
	ID          string
	QuotationID string
}

// CRMPort is the narrow slice of Section 14 CRM this feature depends on.
type CRMPort interface {
	GetCustomer(ctx context.Context, q database.Querier, organizationID, customerID string) (CustomerInfo, error)
	// CreditHoldBlocks reports whether the customer's credit policy blocks new
	// commitments. A customer with no credit profile is not blocked — an
	// organization that never set a policy has not decided to refuse them.
	CreditHoldBlocks(ctx context.Context, q database.Querier, organizationID, customerID string) (bool, error)
	GetContract(ctx context.Context, q database.Querier, organizationID, contractID string) (ContractInfo, error)
	GetQuotation(ctx context.Context, q database.Querier, organizationID, quotationID string) (QuotationInfo, error)
	GetConversionIntent(ctx context.Context, q database.Querier, organizationID, acceptedVersionID string) (ConversionIntentInfo, error)
	// ConversionIntentConsumption reports whether an intent has already been
	// consumed and, if so, which private order claimed it.
	ConversionIntentConsumption(ctx context.Context, q database.Querier, organizationID, intentID string) (consumed bool, privateOrderID string, err error)
	// ConsumeConversionIntent marks the intent consumed, guarded so exactly one
	// order can ever claim it under concurrency. Returns ErrPortNotFound if the
	// intent was consumed by someone else since it was last read.
	ConsumeConversionIntent(ctx context.Context, q database.Querier, organizationID, intentID, privateOrderID string) error
}

// CatalogItemInfo is this feature's own projection of a Section 15 service
// catalogue item.
type CatalogItemInfo struct {
	ID                   string
	BranchID             string
	Active               bool
	RequiredFieldsSchema models.JSONMap
	// Snapshot is the immutable projection an order freezes at confirmation.
	Snapshot models.JSONMap
}

// CatalogPort is the narrow slice of the Section 15 service catalogue this
// feature depends on.
type CatalogPort interface {
	GetCatalogItem(ctx context.Context, q database.Querier, organizationID, itemID string) (CatalogItemInfo, error)
}

// SearchIndexInput is this feature's own projection into the Section 25
// search index — no dependency on the search feature's models package, so
// this stays a plain port like every other cross-feature dependency here.
type SearchIndexInput struct {
	BranchID             string
	SubjectType          string
	SubjectReference     string
	Title                string
	SearchableText       string
	ExactReferenceFields map[string]any
	Status               string
	RequiredPermission   string
	SourceVersion        int64
}

// SearchIndexer is the narrow port into Section 25 search indexing.
//
// Indexing is fire-and-forget: search is a read-side projection, and an
// order write must never fail or roll back because a downstream index
// update failed. Every call site here treats a SearchIndexer error as
// non-fatal, and a nil SearchIndexer makes indexing a no-op — a deployment
// that has not wired search still writes orders correctly, it simply is not
// findable through search until it is.
type SearchIndexer interface {
	Index(ctx context.Context, organizationID string, in SearchIndexInput) error
	RemoveFromIndex(ctx context.Context, organizationID, subjectType, subjectReference string) error
}

// SubjectTypeOrder is this feature's search subject type.
const SubjectTypeOrder = "PRIVATE_ORDER"

// PermissionOrdersRead must match the exact code orders/http declares —
// duplicated here (as a literal, not an import, to respect the feature
// boundary) because indexing needs it to gate search visibility and this
// package may not import orders/http.
const searchPermissionOrdersRead = "orders.read"

// Flags gates the Section 15 order read and write surfaces.
type Flags struct {
	Enabled       bool
	WritesEnabled bool
	KillSwitch    bool
}

// Options configure the orders service.
type Options struct {
	Pool       *pgxpool.Pool
	Repository *repositories.Repository
	Catalog    CatalogPort
	CRM        CRMPort
	Events     EventPublisher
	Audit      *audit.Recorder
	// Search indexes orders into Section 25 search after a commit succeeds.
	// Nil disables indexing without affecting order writes.
	Search  SearchIndexer
	Metrics *observability.Metrics
	Flags   Flags
	Now     func() time.Time
}

// Service implements the Section 15 private order workflows.
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

// reindex projects one order into Section 25 search, best-effort, after its
// own commit has already succeeded.
//
// Errors are swallowed rather than returned: the write this follows already
// committed, and a search-indexing failure must never be reported back to
// the caller as if the order write itself failed. A metrics counter, not an
// error return, is how this failure mode is meant to be observed.
func (s *Service) reindex(ctx context.Context, order models.Order) {
	if s.opts.Search == nil {
		return
	}
	title := order.OrderNumber
	if order.CustomerReference != "" {
		title += " — " + order.CustomerReference
	}
	searchable := strings.Join([]string{
		order.OrderNumber, order.CustomerReference, order.SpecialInstructions, order.Status,
	}, " ")
	err := s.opts.Search.Index(ctx, order.OrganizationID, SearchIndexInput{
		BranchID: order.BranchID, SubjectType: SubjectTypeOrder, SubjectReference: order.ID,
		Title: title, SearchableText: searchable,
		ExactReferenceFields: map[string]any{
			"orderNumber": order.OrderNumber, "customerId": order.CustomerID,
		},
		Status: order.Status, RequiredPermission: searchPermissionOrdersRead,
		SourceVersion: order.Version,
	})
	if err != nil {
		s.opts.Metrics.IncCounter("fleet_partner_orders_search_index_failures_total", nil, 1)
	}
}

func orderNotFound() error {
	return errorcodes.NotFound(errorcodes.OrderNotFound, "Order not found.")
}

func translateOrderNotFound(err error) error {
	if errors.Is(err, repositories.ErrNotFound) {
		return orderNotFound()
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
		return errorcodes.Unavailable(errorcodes.OrderInvalidTransition, "Private orders are temporarily unavailable.")
	}
	if !s.opts.Flags.Enabled {
		return orderNotFound()
	}
	return nil
}

func (s *Service) requireWritable() error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
	if !s.opts.Flags.WritesEnabled {
		return errorcodes.Forbidden(errorcodes.OrderInvalidTransition, "Order writes are not enabled for this organization.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Shared validation
// ---------------------------------------------------------------------------

func requireCommittableCustomer(customer CustomerInfo) error {
	if !customer.Blocked {
		return nil
	}
	switch customer.BlockedReason {
	case "customer_archived":
		return errorcodes.Conflict(errorcodes.CustomerArchived,
			"This customer is archived and cannot receive new orders.")
	case "customer_risk_restricted":
		return errorcodes.Conflict(errorcodes.CustomerRiskRestricted,
			"This customer is restricted and cannot receive new orders.")
	default:
		return errorcodes.Conflict(errorcodes.CustomerOnHold,
			"This customer is on hold and cannot receive new orders.")
	}
}

func (s *Service) requireActiveCustomer(ctx context.Context, q database.Querier, t tenant.Context,
	customerID string) (CustomerInfo, error) {
	customer, err := s.opts.CRM.GetCustomer(ctx, q, t.OrganizationID, customerID)
	if err != nil {
		if errors.Is(err, ErrPortNotFound) {
			return customer, errorcodes.NotFound(errorcodes.CustomerNotFound, "Customer not found.")
		}
		return customer, err
	}
	if !t.BranchPermitted(customer.BranchID) {
		return customer, errorcodes.NotFound(errorcodes.CustomerNotFound, "Customer not found.")
	}
	if err := requireCommittableCustomer(customer); err != nil {
		return customer, err
	}
	blocked, err := s.opts.CRM.CreditHoldBlocks(ctx, q, t.OrganizationID, customerID)
	if err != nil {
		return customer, err
	}
	if blocked {
		return customer, errorcodes.Conflict(errorcodes.CustomerCreditHold,
			"A credit hold on this customer prevents this order.")
	}
	return customer, nil
}

// requireActiveCatalogItem loads a catalogue item and refuses one that is not
// ACTIVE or does not belong to this organization/branch.
func (s *Service) requireActiveCatalogItem(ctx context.Context, q database.Querier, t tenant.Context,
	itemID string) (CatalogItemInfo, error) {
	item, err := s.opts.Catalog.GetCatalogItem(ctx, q, t.OrganizationID, itemID)
	if err != nil {
		if errors.Is(err, ErrPortNotFound) {
			return item, errorcodes.NotFound(errorcodes.ServiceCatalogItemNotFound, "Service catalogue item not found.")
		}
		return item, err
	}
	if !t.BranchPermitted(item.BranchID) {
		return item, errorcodes.NotFound(errorcodes.ServiceCatalogItemNotFound, "Service catalogue item not found.")
	}
	if !item.Active {
		return item, errorcodes.Conflict(errorcodes.OrderCatalogItemNotActive,
			"This service catalogue item is not active.")
	}
	return item, nil
}

// requireActiveContract validates a contract belongs to the organization and
// customer and is active for the requested date.
func (s *Service) requireActiveContract(ctx context.Context, q database.Querier, t tenant.Context,
	contractID, customerID string, at time.Time) (ContractInfo, error) {
	contract, err := s.opts.CRM.GetContract(ctx, q, t.OrganizationID, contractID)
	if err != nil {
		if errors.Is(err, ErrPortNotFound) {
			return contract, errorcodes.NotFound(errorcodes.ContractNotFound, "Contract not found.")
		}
		return contract, err
	}
	if !t.BranchPermitted(contract.BranchID) {
		return contract, errorcodes.NotFound(errorcodes.ContractNotFound, "Contract not found.")
	}
	if contract.CustomerID != customerID {
		return contract, errorcodes.Conflict(errorcodes.OrderContractNotActive,
			"This contract does not belong to the order's customer.")
	}
	if !contract.Active {
		return contract, errorcodes.Conflict(errorcodes.OrderContractNotActive, "This contract is not active.")
	}
	if at.Before(contract.EffectiveFrom) || (contract.EffectiveTo != nil && at.After(*contract.EffectiveTo)) {
		return contract, errorcodes.Conflict(errorcodes.OrderContractNotActive,
			"This contract is not active for the requested schedule.")
	}
	return contract, nil
}

func validTimezone(tz string) bool {
	_, err := time.LoadLocation(tz)
	return err == nil
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// ListInput is the allowlisted order list query.
type ListInput struct {
	Status     string
	CustomerID string
	BranchID   string
	Cursor     string
	Limit      int
}

// List returns a tenant-scoped page of orders, newest first.
func (s *Service) List(ctx context.Context, t tenant.Context, in ListInput) ([]models.Order, string, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, "", err
	}
	decoded, hasCursor, err := cursor.Decode(in.Cursor)
	if err != nil {
		return nil, "", apperrors.BadRequest("cursor is malformed.", err)
	}
	orders, err := s.opts.Repository.List(ctx, t.OrganizationID, repositories.Filter{
		Status: in.Status, CustomerID: in.CustomerID, BranchID: in.BranchID,
		Cursor: decoded, HasCursor: hasCursor, Limit: in.Limit,
	})
	if err != nil {
		return nil, "", err
	}
	var next string
	if len(orders) > 0 {
		last := orders[len(orders)-1]
		next = cursor.EncodeTime(last.CreatedAt, last.ID)
	}
	return orders, next, nil
}

// Get reads one order, refusing a cross-branch read the same way it refuses
// a missing one.
func (s *Service) Get(ctx context.Context, t tenant.Context, orderID string) (models.Order, error) {
	if err := s.requireEnabled(); err != nil {
		return models.Order{}, err
	}
	order, err := s.opts.Repository.Get(ctx, nil, t.OrganizationID, orderID)
	if err != nil {
		return models.Order{}, translateOrderNotFound(err)
	}
	if !t.BranchPermitted(order.BranchID) {
		return models.Order{}, orderNotFound()
	}
	return order, nil
}

// ListStops returns an order's stops in sequence order.
func (s *Service) ListStops(ctx context.Context, t tenant.Context, orderID string) ([]models.OrderStop, error) {
	if _, err := s.Get(ctx, t, orderID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListStops(ctx, nil, t.OrganizationID, orderID)
}

// History returns an order's immutable status history.
func (s *Service) History(ctx context.Context, t tenant.Context, orderID string) ([]models.OrderHistory, error) {
	if _, err := s.Get(ctx, t, orderID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListHistory(ctx, t.OrganizationID, orderID)
}

// ---------------------------------------------------------------------------
// Create (manual)
// ---------------------------------------------------------------------------

// CreateInput is the validated shape of a new manual private order.
type CreateInput struct {
	BranchID               string
	CustomerID             string
	CustomerReference      string
	ServiceCatalogItemID   string
	ServiceFields          models.JSONMap
	ContractID             string
	RequestedScheduleStart *time.Time
	RequestedScheduleEnd   *time.Time
	Timezone               string
	Priority               string
	SpecialInstructions    string
	// Source and SourceReference are set only by trusted composition-root
	// adapters (recurring generation and bulk import). HTTP manual creation
	// leaves them empty and remains MANUAL.
	Source          string
	SourceReference string
}

// newOrderNumber mints a display reference. It is randomly generated, never
// a database sequence, so an order number never discloses how many orders an
// organization has placed.
func newOrderNumber() string {
	buf := make([]byte, 5)
	_, _ = rand.Read(buf)
	return "ORD-" + strings.ToUpper(hex.EncodeToString(buf))
}

// Create adds a new DRAFT manual order. It creates no assignment, execution
// request, canonical trip or wallet movement.
func (s *Service) Create(ctx context.Context, t tenant.Context, in CreateInput) (models.Order, error) {
	if err := s.requireWritable(); err != nil {
		return models.Order{}, err
	}
	if strings.TrimSpace(in.CustomerID) == "" {
		return models.Order{}, apperrors.Validation("customerId is required.",
			[]apperrors.FieldViolation{{Field: "customerId", Message: "required"}})
	}
	if strings.TrimSpace(in.ServiceCatalogItemID) == "" {
		return models.Order{}, apperrors.Validation("serviceCatalogItemId is required.",
			[]apperrors.FieldViolation{{Field: "serviceCatalogItemId", Message: "required"}})
	}
	if !validTimezone(in.Timezone) {
		return models.Order{}, apperrors.Validation("timezone must be a valid IANA timezone.",
			[]apperrors.FieldViolation{{Field: "timezone", Message: "invalid"}})
	}
	priority := in.Priority
	if priority == "" {
		priority = models.PriorityStandard
	}
	if !models.ValidPriority(priority) {
		return models.Order{}, apperrors.Validation("priority is not recognized.",
			[]apperrors.FieldViolation{{Field: "priority", Message: "unsupported"}})
	}
	if in.RequestedScheduleStart != nil && in.RequestedScheduleEnd != nil &&
		in.RequestedScheduleEnd.Before(*in.RequestedScheduleStart) {
		return models.Order{}, apperrors.Validation("requestedScheduleEnd must not precede requestedScheduleStart.",
			[]apperrors.FieldViolation{{Field: "requestedScheduleEnd", Message: "before_start"}})
	}

	customer, err := s.requireActiveCustomer(ctx, nil, t, in.CustomerID)
	if err != nil {
		return models.Order{}, err
	}
	item, err := s.requireActiveCatalogItem(ctx, nil, t, in.ServiceCatalogItemID)
	if err != nil {
		return models.Order{}, err
	}
	if err := models.ValidateServiceFields(item.RequiredFieldsSchema, in.ServiceFields); err != nil {
		return models.Order{}, errorcodes.UnprocessableEntity(errorcodes.OrderStopInvalid, err.Error())
	}

	var contractVersionID string
	if strings.TrimSpace(in.ContractID) != "" {
		at := s.opts.Now()
		if in.RequestedScheduleStart != nil {
			at = *in.RequestedScheduleStart
		}
		contract, err := s.requireActiveContract(ctx, nil, t, in.ContractID, customer.ID, at)
		if err != nil {
			return models.Order{}, err
		}
		contractVersionID = contract.ActiveVersionID
	}

	source := in.Source
	if source == "" {
		source = models.SourceManual
	}
	if !models.ValidSource(source) {
		return models.Order{}, apperrors.BadRequest("order source is not recognized.", nil)
	}
	order := models.Order{
		ID:                     ids.New(repositories.PrefixOrder),
		OrganizationID:         t.OrganizationID,
		BranchID:               in.BranchID,
		OrderNumber:            newOrderNumber(),
		CustomerID:             customer.ID,
		CustomerReference:      strings.TrimSpace(in.CustomerReference),
		Source:                 source,
		SourceReference:        strings.TrimSpace(in.SourceReference),
		ServiceCatalogItemID:   item.ID,
		ServiceCatalogSnapshot: item.Snapshot.Clone(),
		ServiceFields:          in.ServiceFields.Clone(),
		ContractID:             in.ContractID,
		ContractVersionID:      contractVersionID,
		RequestedScheduleStart: in.RequestedScheduleStart,
		RequestedScheduleEnd:   in.RequestedScheduleEnd,
		Timezone:               in.Timezone,
		Priority:               priority,
		SpecialInstructions:    strings.TrimSpace(in.SpecialInstructions),
		Status:                 models.StatusDraft,
		CreatedBy:              t.ActorID,
	}

	var created models.Order
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		var txErr error
		created, txErr = s.opts.Repository.Create(ctx, tx, order)
		if txErr != nil {
			return txErr
		}
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "orders.order.created",
			"private_order", created.ID)); auditErr != nil {
			return auditErr
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "organization.orders.order.created.v1", "private_order",
				created.ID, created.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"orderId": created.ID, "customerId": created.CustomerID, "source": created.Source})
		}
		return nil
	})
	if err != nil {
		return models.Order{}, err
	}
	s.reindex(ctx, created)
	return created, nil
}

// ---------------------------------------------------------------------------
// Stops
// ---------------------------------------------------------------------------

// AddStopInput is the validated shape of a new order stop. Sequence is
// assigned server-side (current count) so stop ordering is always contiguous.
type AddStopInput struct {
	StopType           string
	CustomerLocationID string
	AddressSnapshot    models.JSONMap
	Latitude           *float64
	Longitude          *float64
	TimeWindowStart    *time.Time
	TimeWindowEnd      *time.Time
	Timezone           string
	ContactSnapshot    models.JSONMap
	Instructions       string
}

// AddStop appends a stop to a still-editable order.
func (s *Service) AddStop(ctx context.Context, t tenant.Context, orderID string, in AddStopInput) (models.OrderStop, error) {
	if err := s.requireWritable(); err != nil {
		return models.OrderStop{}, err
	}
	if !models.ValidStopType(in.StopType) {
		return models.OrderStop{}, apperrors.Validation("stopType is not recognized.",
			[]apperrors.FieldViolation{{Field: "stopType", Message: "unsupported"}})
	}
	if !validTimezone(in.Timezone) {
		return models.OrderStop{}, apperrors.Validation("timezone must be a valid IANA timezone.",
			[]apperrors.FieldViolation{{Field: "timezone", Message: "invalid"}})
	}
	if err := models.ValidateCoordinates(in.Latitude, in.Longitude); err != nil {
		return models.OrderStop{}, errorcodes.UnprocessableEntity(errorcodes.OrderStopInvalid, err.Error())
	}
	if in.TimeWindowStart != nil && in.TimeWindowEnd != nil && in.TimeWindowEnd.Before(*in.TimeWindowStart) {
		return models.OrderStop{}, apperrors.Validation("timeWindowEnd must not precede timeWindowStart.",
			[]apperrors.FieldViolation{{Field: "timeWindowEnd", Message: "before_start"}})
	}

	var created models.OrderStop
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, orderID)
		if err != nil {
			return translateOrderNotFound(err)
		}
		if !t.BranchPermitted(order.BranchID) {
			return orderNotFound()
		}
		if models.Terminal(order.Status) || order.Status == models.StatusPlanned ||
			order.Status == models.StatusAssigned || order.Status == models.StatusInProgress {
			return errorcodes.Conflict(errorcodes.OrderInvalidTransition,
				"Stops can only be added while an order is still editable.")
		}

		existing, err := s.opts.Repository.ListStops(ctx, tx, t.OrganizationID, orderID)
		if err != nil {
			return err
		}

		stop := models.OrderStop{
			ID:                 ids.New(repositories.PrefixStop),
			OrganizationID:     t.OrganizationID,
			OrderID:            orderID,
			Sequence:           len(existing),
			StopType:           in.StopType,
			CustomerLocationID: in.CustomerLocationID,
			AddressSnapshot:    in.AddressSnapshot.Clone(),
			Latitude:           in.Latitude,
			Longitude:          in.Longitude,
			TimeWindowStart:    in.TimeWindowStart,
			TimeWindowEnd:      in.TimeWindowEnd,
			Timezone:           in.Timezone,
			ContactSnapshot:    in.ContactSnapshot.Clone(),
			Instructions:       strings.TrimSpace(in.Instructions),
			Status:             models.StopStatusPending,
		}
		created, err = s.opts.Repository.CreateStop(ctx, tx, stop)
		if err != nil {
			return err
		}
		return s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "orders.stop.added", "order_stop", created.ID))
	})
	if err != nil {
		return models.OrderStop{}, err
	}
	return created, nil
}

// ---------------------------------------------------------------------------
// Confirm
// ---------------------------------------------------------------------------

// Confirm validates every prerequisite and freezes the catalogue and
// confirmation snapshots. No execution request, assignment or canonical trip
// is created here — that is a later Section 15 slice's responsibility.
func (s *Service) Confirm(ctx context.Context, t tenant.Context, orderID string, expectedVersion int64) (models.Order, error) {
	if err := s.requireWritable(); err != nil {
		return models.Order{}, err
	}
	if expectedVersion <= 0 {
		return models.Order{}, apperrors.BadRequest("Provide the expected version via If-Match or expectedVersion.", nil)
	}

	var confirmed models.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, orderID)
		if err != nil {
			return translateOrderNotFound(err)
		}
		if !t.BranchPermitted(order.BranchID) {
			return orderNotFound()
		}
		if order.Version != expectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if !models.CanTransition(order.Status, models.StatusConfirmed) {
			return errorcodes.Conflict(errorcodes.OrderNotConfirmable,
				"This order cannot be confirmed from its current status.")
		}

		customer, err := s.requireActiveCustomer(ctx, tx, t, order.CustomerID)
		if err != nil {
			return err
		}
		item, err := s.requireActiveCatalogItem(ctx, tx, t, order.ServiceCatalogItemID)
		if err != nil {
			return err
		}
		if err := models.ValidateServiceFields(item.RequiredFieldsSchema, order.ServiceFields); err != nil {
			return errorcodes.UnprocessableEntity(errorcodes.OrderStopInvalid, err.Error())
		}
		if strings.TrimSpace(order.ContractID) != "" {
			at := s.opts.Now()
			if order.RequestedScheduleStart != nil {
				at = *order.RequestedScheduleStart
			}
			if _, err := s.requireActiveContract(ctx, tx, t, order.ContractID, customer.ID, at); err != nil {
				return err
			}
		}

		stops, err := s.opts.Repository.ListStops(ctx, tx, t.OrganizationID, orderID)
		if err != nil {
			return err
		}
		if len(stops) == 0 {
			return errorcodes.UnprocessableEntity(errorcodes.OrderStopsRequired,
				"An order must have at least one stop before it can be confirmed.")
		}
		for i, stop := range stops {
			if stop.Sequence != i {
				return errorcodes.Conflict(errorcodes.OrderStopSequenceConflict,
					"Order stop sequence is not contiguous.")
			}
		}

		confirmationSnapshot := models.JSONMap{
			"customerId":          customer.ID,
			"customerLegalName":   customer.LegalName,
			"customerDisplayName": customer.DisplayName,
			"customerType":        customer.CustomerType,
			"stopCount":           len(stops),
		}

		confirmed, err = s.opts.Repository.Confirm(ctx, tx, t.OrganizationID, orderID, expectedVersion,
			item.Snapshot, confirmationSnapshot, t.ActorID)
		if err != nil {
			return concurrency.AsConflict(translateVersionErr(err))
		}
		if histErr := s.opts.Repository.AppendHistory(ctx, tx, models.OrderHistory{
			OrganizationID: t.OrganizationID, OrderID: orderID, PreviousStatus: order.Status,
			NewStatus: models.StatusConfirmed, ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: t.CorrelationID,
		}); histErr != nil {
			return histErr
		}
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "orders.order.confirmed",
			"private_order", confirmed.ID)); auditErr != nil {
			return auditErr
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "organization.orders.order.confirmed.v1", "private_order",
				confirmed.ID, confirmed.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"orderId": confirmed.ID})
		}
		return nil
	})
	if err != nil {
		return models.Order{}, concurrency.AsConflict(err)
	}
	s.reindex(ctx, confirmed)
	return confirmed, nil
}

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

// Cancel moves a still-cancellable order to CANCELLED. Once an order reaches
// ASSIGNED or IN_PROGRESS it has no direct cancel edge — a later
// orchestration slice must route the request through the owning execution
// service instead of writing a local status.
func (s *Service) Cancel(ctx context.Context, t tenant.Context, orderID string, expectedVersion int64, reasonCode string) (models.Order, error) {
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

	var cancelled models.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, orderID)
		if err != nil {
			return translateOrderNotFound(err)
		}
		if !t.BranchPermitted(order.BranchID) {
			return orderNotFound()
		}
		if order.Version != expectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if !models.Cancellable(order.Status) {
			return errorcodes.Conflict(errorcodes.OrderNotCancellable,
				"This order can no longer be cancelled directly; it has moved past planning.")
		}

		cancelled, err = s.opts.Repository.Cancel(ctx, tx, t.OrganizationID, orderID, expectedVersion,
			reasonCode, t.ActorID)
		if err != nil {
			return concurrency.AsConflict(translateVersionErr(err))
		}
		if histErr := s.opts.Repository.AppendHistory(ctx, tx, models.OrderHistory{
			OrganizationID: t.OrganizationID, OrderID: orderID, PreviousStatus: order.Status,
			NewStatus: models.StatusCancelled, ReasonCode: reasonCode, ActorType: string(t.ActorType),
			ActorID: t.ActorID, CorrelationID: t.CorrelationID,
		}); histErr != nil {
			return histErr
		}
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "orders.order.cancelled",
			"private_order", cancelled.ID)); auditErr != nil {
			return auditErr
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "organization.orders.order.cancelled.v1", "private_order",
				cancelled.ID, cancelled.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"orderId": cancelled.ID, "reasonCode": reasonCode})
		}
		return nil
	})
	if err != nil {
		return models.Order{}, concurrency.AsConflict(err)
	}
	s.reindex(ctx, cancelled)
	return cancelled, nil
}

// ---------------------------------------------------------------------------
// Section 14 conversion-intent consumption
// ---------------------------------------------------------------------------

// ConvertConversionIntentInput supplies the operational detail a conversion
// intent alone does not carry — a quotation records what was sold, not a
// schedule, timezone or stop plan.
type ConvertConversionIntentInput struct {
	BranchID               string
	ServiceCatalogItemID   string
	ServiceFields          models.JSONMap
	RequestedScheduleStart *time.Time
	RequestedScheduleEnd   *time.Time
	Timezone               string
	Priority               string
	SpecialInstructions    string
}

// ConvertConversionIntent consumes an accepted quotation's Section 14
// conversion intent exactly once and produces the matching private order.
//
//  1. Loads the intent by accepted version, tenant- and existence-checked.
//  2. If already consumed, returns the order it produced — an idempotent
//     replay, not a conflict.
//  3. Otherwise creates the order and consumes the intent in the same
//     transaction, so an intent can never end up consumed without an order or
//     an order without a consumed intent.
func (s *Service) ConvertConversionIntent(ctx context.Context, t tenant.Context, acceptedVersionID string,
	in ConvertConversionIntentInput) (models.Order, error) {
	if err := s.requireWritable(); err != nil {
		return models.Order{}, err
	}
	if strings.TrimSpace(acceptedVersionID) == "" {
		return models.Order{}, apperrors.Validation("acceptedVersionId is required.",
			[]apperrors.FieldViolation{{Field: "acceptedVersionId", Message: "required"}})
	}
	if !validTimezone(in.Timezone) {
		return models.Order{}, apperrors.Validation("timezone must be a valid IANA timezone.",
			[]apperrors.FieldViolation{{Field: "timezone", Message: "invalid"}})
	}
	priority := in.Priority
	if priority == "" {
		priority = models.PriorityStandard
	}
	if !models.ValidPriority(priority) {
		return models.Order{}, apperrors.Validation("priority is not recognized.",
			[]apperrors.FieldViolation{{Field: "priority", Message: "unsupported"}})
	}

	intent, err := s.opts.CRM.GetConversionIntent(ctx, nil, t.OrganizationID, acceptedVersionID)
	if err != nil {
		if errors.Is(err, ErrPortNotFound) {
			return models.Order{}, errorcodes.NotFound(errorcodes.ConversionIntentNotFound,
				"No conversion intent exists for this accepted quotation version.")
		}
		return models.Order{}, err
	}

	consumed, existingOrderID, err := s.opts.CRM.ConversionIntentConsumption(ctx, nil, t.OrganizationID, intent.ID)
	if err != nil {
		return models.Order{}, err
	}
	if consumed {
		if existingOrderID == "" {
			return models.Order{}, errorcodes.Conflict(errorcodes.ConversionIntentAlreadyConsumed,
				"This conversion intent has already been consumed.")
		}
		return s.Get(ctx, t, existingOrderID)
	}

	quotation, err := s.opts.CRM.GetQuotation(ctx, nil, t.OrganizationID, intent.QuotationID)
	if err != nil {
		if errors.Is(err, ErrPortNotFound) {
			return models.Order{}, errorcodes.NotFound(errorcodes.QuotationNotFound, "Quotation not found.")
		}
		return models.Order{}, err
	}
	if quotation.AcceptedVersionID != acceptedVersionID {
		return models.Order{}, errorcodes.Conflict(errorcodes.ConversionIntentMismatch,
			"This conversion intent no longer matches the quotation's accepted version.")
	}

	customer, err := s.requireActiveCustomer(ctx, nil, t, quotation.CustomerID)
	if err != nil {
		return models.Order{}, err
	}
	item, err := s.requireActiveCatalogItem(ctx, nil, t, in.ServiceCatalogItemID)
	if err != nil {
		return models.Order{}, err
	}
	if err := models.ValidateServiceFields(item.RequiredFieldsSchema, in.ServiceFields); err != nil {
		return models.Order{}, errorcodes.UnprocessableEntity(errorcodes.OrderStopInvalid, err.Error())
	}

	order := models.Order{
		ID:                     ids.New(repositories.PrefixOrder),
		OrganizationID:         t.OrganizationID,
		BranchID:               in.BranchID,
		OrderNumber:            newOrderNumber(),
		CustomerID:             customer.ID,
		Source:                 models.SourceQuotation,
		SourceReference:        quotation.ID,
		ServiceCatalogItemID:   item.ID,
		ServiceCatalogSnapshot: item.Snapshot.Clone(),
		ServiceFields:          in.ServiceFields.Clone(),
		QuotationID:            quotation.ID,
		QuotationVersionID:     acceptedVersionID,
		ConversionIntentID:     intent.ID,
		RequestedScheduleStart: in.RequestedScheduleStart,
		RequestedScheduleEnd:   in.RequestedScheduleEnd,
		Timezone:               in.Timezone,
		Priority:               priority,
		SpecialInstructions:    strings.TrimSpace(in.SpecialInstructions),
		Status:                 models.StatusDraft,
		CreatedBy:              t.ActorID,
	}

	var created models.Order
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		var txErr error
		created, txErr = s.opts.Repository.Create(ctx, tx, order)
		if txErr != nil {
			return txErr
		}
		if consumeErr := s.opts.CRM.ConsumeConversionIntent(ctx, tx, t.OrganizationID, intent.ID, created.ID); consumeErr != nil {
			if errors.Is(consumeErr, ErrPortNotFound) {
				// Lost the race: someone else consumed this intent between our
				// read and this write. Abort the whole transaction so no order
				// is left dangling without a consumed intent.
				return errorcodes.Conflict(errorcodes.ConversionIntentAlreadyConsumed,
					"This conversion intent has already been consumed.")
			}
			return consumeErr
		}
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "orders.order.created_from_quotation",
			"private_order", created.ID)); auditErr != nil {
			return auditErr
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "organization.orders.order.created.v1", "private_order",
				created.ID, created.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"orderId": created.ID, "customerId": created.CustomerID, "source": created.Source,
					"quotationId": quotation.ID})
		}
		return nil
	})
	if err != nil {
		return models.Order{}, err
	}
	s.reindex(ctx, created)
	return created, nil
}
