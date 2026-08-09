// Package usecases implements the Section 15 service catalogue workflows.
//
// A catalogue item describes what an organization offers; it never prices it.
// There is no rate table, fare formula or commission calculation anywhere in
// this package, and Taxi/PassengerTransport activation is refused
// unconditionally regardless of any organization capability flag, matching
// the platform-wide fail-closed rule for Taxi execution.
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
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// EventPublisher appends corporate domain events to the outbox.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string,
		aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

// Flags gates the Section 15 catalogue read and write surfaces.
type Flags struct {
	Enabled       bool
	WritesEnabled bool
	KillSwitch    bool
}

// Options configure the service catalogue service.
type Options struct {
	Pool       *pgxpool.Pool
	Repository *repositories.Repository
	Events     EventPublisher
	Audit      *audit.Recorder
	Metrics    *observability.Metrics
	Flags      Flags
	Now        func() time.Time
}

// Service implements the Section 15 service catalogue workflows.
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

func notFound() error {
	return errorcodes.NotFound(errorcodes.ServiceCatalogItemNotFound, "Service catalogue item not found.")
}

func translateNotFound(err error) error {
	if errors.Is(err, repositories.ErrNotFound) {
		return notFound()
	}
	return err
}

func (s *Service) requireEnabled() error {
	if s.opts.Flags.KillSwitch {
		return errorcodes.Unavailable(errorcodes.ServiceCatalogItemInvalidState,
			"The service catalogue is temporarily unavailable.")
	}
	if !s.opts.Flags.Enabled {
		return notFound()
	}
	return nil
}

func (s *Service) requireWritable() error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
	if !s.opts.Flags.WritesEnabled {
		return errorcodes.Forbidden(errorcodes.ServiceCatalogItemInvalidState,
			"Service catalogue writes are not enabled for this organization.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// GetInput identifies a single catalogue item read.
type ListInput struct {
	Status   string
	Category string
	BranchID string
	Cursor   string
	Limit    int
}

// List returns a tenant-scoped page of catalogue items, newest first.
func (s *Service) List(ctx context.Context, t tenant.Context, input ListInput) ([]models.CatalogItem, string, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, "", err
	}
	decoded, hasCursor, err := cursor.Decode(input.Cursor)
	if err != nil {
		return nil, "", apperrors.BadRequest("cursor is malformed.", err)
	}
	items, err := s.opts.Repository.List(ctx, t.OrganizationID, repositories.Filter{
		Status: input.Status, Category: input.Category, BranchID: input.BranchID,
		Cursor: decoded, HasCursor: hasCursor, Limit: input.Limit,
	})
	if err != nil {
		return nil, "", err
	}
	var next string
	if len(items) > 0 {
		last := items[len(items)-1]
		next = cursor.EncodeTime(last.CreatedAt, last.ID)
	}
	return items, next, nil
}

// Get reads one catalogue item, refusing a cross-branch read the same way it
// refuses a missing one.
func (s *Service) Get(ctx context.Context, t tenant.Context, itemID string) (models.CatalogItem, error) {
	if err := s.requireEnabled(); err != nil {
		return models.CatalogItem{}, err
	}
	item, err := s.opts.Repository.Get(ctx, nil, t.OrganizationID, itemID)
	if err != nil {
		return models.CatalogItem{}, translateNotFound(err)
	}
	if !t.BranchPermitted(item.BranchID) {
		return models.CatalogItem{}, notFound()
	}
	return item, nil
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateInput is the validated shape of a new catalogue item.
type CreateInput struct {
	BranchID                string
	Name                    string
	Code                    string
	Category                string
	Description             string
	CapabilityReference     string
	RequiredFieldsSchema    models.JSONMap
	SupportedVehicleClasses []string
	SupportedPackageTypes   []string
	PricingPolicyReference  string
	ApprovalPolicyReference string
	ProofPolicyReference    string
}

func (in CreateInput) validate() (string, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return "", "", apperrors.Validation("name is required.", []apperrors.FieldViolation{{Field: "name", Message: "required"}})
	}
	code := models.NormalizeCode(in.Code)
	if code == "" {
		return "", "", apperrors.Validation("code is required.", []apperrors.FieldViolation{{Field: "code", Message: "required"}})
	}
	if !models.ValidCategory(in.Category) {
		return "", "", apperrors.Validation("category is not recognized.",
			[]apperrors.FieldViolation{{Field: "category", Message: "unsupported"}})
	}
	if err := models.ValidateRequiredFieldsSchema(in.RequiredFieldsSchema); err != nil {
		return "", "", errorcodes.BadRequest(errorcodes.ServiceCatalogSchemaInvalid, err.Error())
	}
	return name, code, nil
}

// Create adds a new DRAFT catalogue item.
func (s *Service) Create(ctx context.Context, t tenant.Context, in CreateInput) (models.CatalogItem, error) {
	if err := s.requireWritable(); err != nil {
		return models.CatalogItem{}, err
	}
	name, code, err := in.validate()
	if err != nil {
		return models.CatalogItem{}, err
	}

	item := models.CatalogItem{
		ID:                      ids.New(repositories.PrefixCatalogItem),
		OrganizationID:          t.OrganizationID,
		BranchID:                in.BranchID,
		Name:                    name,
		Code:                    code,
		Category:                in.Category,
		Description:             strings.TrimSpace(in.Description),
		CapabilityReference:     strings.TrimSpace(in.CapabilityReference),
		RequiredFieldsSchema:    in.RequiredFieldsSchema.Clone(),
		SupportedVehicleClasses: in.SupportedVehicleClasses,
		SupportedPackageTypes:   in.SupportedPackageTypes,
		PricingPolicyReference:  strings.TrimSpace(in.PricingPolicyReference),
		ApprovalPolicyReference: strings.TrimSpace(in.ApprovalPolicyReference),
		ProofPolicyReference:    strings.TrimSpace(in.ProofPolicyReference),
		Status:                  models.StatusDraft,
		CreatedBy:               t.ActorID,
	}

	var created models.CatalogItem
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		var txErr error
		created, txErr = s.opts.Repository.Create(ctx, tx, item)
		if txErr != nil {
			if errors.Is(txErr, repositories.ErrDuplicate) {
				return errorcodes.Conflict(errorcodes.ServiceCatalogCodeConflict,
					"This code is already used by another catalogue item in this organization.")
			}
			return txErr
		}
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "servicecatalog.item.created",
			"service_catalog_item", created.ID)); auditErr != nil {
			return auditErr
		}
		if s.opts.Events != nil {
			if pubErr := s.opts.Events.Publish(ctx, tx, "organization.servicecatalog.item.created.v1",
				"service_catalog_item", created.ID, created.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"itemId": created.ID, "code": created.Code, "category": created.Category}); pubErr != nil {
				return pubErr
			}
		}
		return nil
	})
	if err != nil {
		return models.CatalogItem{}, err
	}
	return created, nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// UpdateInput is the validated shape of a catalogue item edit. Code and
// category are immutable once created: a code is permanently reserved and a
// category is the item's core identity, so changing either is a new item,
// not an edit of this one.
type UpdateInput struct {
	Name                    string
	Description             string
	CapabilityReference     string
	RequiredFieldsSchema    models.JSONMap
	SupportedVehicleClasses []string
	SupportedPackageTypes   []string
	PricingPolicyReference  string
	ApprovalPolicyReference string
	ProofPolicyReference    string
}

// Update edits the mutable fields of a catalogue item under optimistic
// concurrency.
func (s *Service) Update(ctx context.Context, t tenant.Context, itemID string, expectedVersion int64,
	in UpdateInput) (models.CatalogItem, error) {
	if err := s.requireWritable(); err != nil {
		return models.CatalogItem{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return models.CatalogItem{}, apperrors.Validation("name is required.",
			[]apperrors.FieldViolation{{Field: "name", Message: "required"}})
	}
	if err := models.ValidateRequiredFieldsSchema(in.RequiredFieldsSchema); err != nil {
		return models.CatalogItem{}, errorcodes.BadRequest(errorcodes.ServiceCatalogSchemaInvalid, err.Error())
	}

	existing, err := s.opts.Repository.Get(ctx, nil, t.OrganizationID, itemID)
	if err != nil {
		return models.CatalogItem{}, translateNotFound(err)
	}
	if !t.BranchPermitted(existing.BranchID) {
		return models.CatalogItem{}, notFound()
	}
	if models.Terminal(existing.Status) {
		return models.CatalogItem{}, errorcodes.Conflict(errorcodes.ServiceCatalogItemInvalidState,
			"An archived catalogue item cannot be edited.")
	}

	edited := existing
	edited.Name = name
	edited.Description = strings.TrimSpace(in.Description)
	edited.CapabilityReference = strings.TrimSpace(in.CapabilityReference)
	edited.RequiredFieldsSchema = in.RequiredFieldsSchema.Clone()
	edited.SupportedVehicleClasses = in.SupportedVehicleClasses
	edited.SupportedPackageTypes = in.SupportedPackageTypes
	edited.PricingPolicyReference = strings.TrimSpace(in.PricingPolicyReference)
	edited.ApprovalPolicyReference = strings.TrimSpace(in.ApprovalPolicyReference)
	edited.ProofPolicyReference = strings.TrimSpace(in.ProofPolicyReference)
	edited.UpdatedBy = t.ActorID

	var updated models.CatalogItem
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		var txErr error
		updated, txErr = s.opts.Repository.UpdateFields(ctx, tx, t.OrganizationID, itemID, expectedVersion, edited)
		if txErr != nil {
			return concurrency.AsConflict(translateVersionErr(txErr))
		}
		return s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "servicecatalog.item.updated",
			"service_catalog_item", updated.ID))
	})
	if err != nil {
		return models.CatalogItem{}, err
	}
	return updated, nil
}

func translateVersionErr(err error) error {
	if errors.Is(err, repositories.ErrVersionConflict) {
		return concurrency.ErrVersionMismatch
	}
	return err
}

// ---------------------------------------------------------------------------
// Lifecycle transitions
// ---------------------------------------------------------------------------

func (s *Service) transition(ctx context.Context, t tenant.Context, itemID string, expectedVersion int64,
	target string, archived bool, auditAction string, eventType string) (models.CatalogItem, error) {
	if err := s.requireWritable(); err != nil {
		return models.CatalogItem{}, err
	}

	existing, err := s.opts.Repository.Get(ctx, nil, t.OrganizationID, itemID)
	if err != nil {
		return models.CatalogItem{}, translateNotFound(err)
	}
	if !t.BranchPermitted(existing.BranchID) {
		return models.CatalogItem{}, notFound()
	}
	if !models.CanTransition(existing.Status, target) {
		return models.CatalogItem{}, errorcodes.Conflict(errorcodes.ServiceCatalogItemInvalidState,
			"This catalogue item cannot move to the requested status from its current status.")
	}
	// Taxi/PassengerTransport activation is refused unconditionally, regardless
	// of capability state: Taxi execution is fail-closed platform-wide, and a
	// catalogue item must never be allowed to imply otherwise.
	if target == models.StatusActive && models.IsTaxiCategory(existing.Category) {
		return models.CatalogItem{}, errorcodes.Forbidden(errorcodes.ServiceCatalogCategoryDisabled,
			"Taxi and passenger transport catalogue items cannot be activated in this deployment.")
	}
	if target == models.StatusActive && strings.TrimSpace(existing.CapabilityReference) == "" {
		return models.CatalogItem{}, errorcodes.UnprocessableEntity(errorcodes.ServiceCatalogCapabilityRequired,
			"Activation requires a linked organization capability reference.")
	}

	var updated models.CatalogItem
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		var txErr error
		updated, txErr = s.opts.Repository.SetStatus(ctx, tx, t.OrganizationID, itemID, expectedVersion,
			target, t.ActorID, archived)
		if txErr != nil {
			return concurrency.AsConflict(translateVersionErr(txErr))
		}
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, auditAction,
			"service_catalog_item", updated.ID)); auditErr != nil {
			return auditErr
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, eventType, "service_catalog_item", updated.ID,
				updated.Version, t.OrganizationID, t.CorrelationID,
				map[string]any{"itemId": updated.ID, "status": updated.Status})
		}
		return nil
	})
	if err != nil {
		return models.CatalogItem{}, err
	}
	return updated, nil
}

// Activate moves a DRAFT or INACTIVE item to ACTIVE.
func (s *Service) Activate(ctx context.Context, t tenant.Context, itemID string, expectedVersion int64) (models.CatalogItem, error) {
	return s.transition(ctx, t, itemID, expectedVersion, models.StatusActive, false,
		"servicecatalog.item.activated", "organization.servicecatalog.item.activated.v1")
}

// Deactivate moves an ACTIVE item to INACTIVE.
func (s *Service) Deactivate(ctx context.Context, t tenant.Context, itemID string, expectedVersion int64) (models.CatalogItem, error) {
	return s.transition(ctx, t, itemID, expectedVersion, models.StatusInactive, false,
		"servicecatalog.item.deactivated", "organization.servicecatalog.item.deactivated.v1")
}

// Archive permanently closes a catalogue item.
func (s *Service) Archive(ctx context.Context, t tenant.Context, itemID string, expectedVersion int64) (models.CatalogItem, error) {
	return s.transition(ctx, t, itemID, expectedVersion, models.StatusArchived, true,
		"servicecatalog.item.archived", "organization.servicecatalog.item.archived.v1")
}
