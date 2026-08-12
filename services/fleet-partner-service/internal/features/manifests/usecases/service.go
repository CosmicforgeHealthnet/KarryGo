// Package usecases implements manifest editing and transactional closure.
package usecases

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type ResourceInfo struct{ ID, ShipmentID, BranchID string }
type ResourcesPort interface {
	Resolve(context.Context, database.Querier, string, string, string) (ResourceInfo, error)
}
type AuthorityPort interface {
	Validate(context.Context, string, string, string) (bool, error)
}
type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type Flags struct{ Enabled, WritesEnabled, KillSwitch bool }
type Options struct {
	Pool       *pgxpool.Pool
	Repository *repositories.Repository
	Resources  ResourcesPort
	Authority  AuthorityPort
	Events     EventPublisher
	Audit      *audit.Recorder
	Flags      Flags
	Now        func() time.Time
}
type Service struct{ opts Options }

func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o}
}
func (s *Service) gate(write bool) error {
	if s.opts.Flags.KillSwitch {
		return errorcodes.Unavailable("manifests_disabled", "Manifests are unavailable.")
	}
	if !s.opts.Flags.Enabled {
		return errorcodes.NotFound("manifest_not_found", "Manifest not found.")
	}
	if write && !s.opts.Flags.WritesEnabled {
		return errorcodes.Forbidden("manifest_invalid_state", "Manifest writes are disabled.")
	}
	return nil
}
func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.opts.Pool, fn)
}
func nf() error { return errorcodes.NotFound("manifest_not_found", "Manifest not found.") }

type CreateInput struct{ BranchID, RouteReference, VehicleReference, ProviderMembershipID, SealNumber string }

func (s *Service) Create(ctx context.Context, t tenant.Context, in CreateInput) (models.Manifest, error) {
	if e := s.gate(true); e != nil {
		return models.Manifest{}, e
	}
	if in.BranchID == "" || !t.BranchPermitted(in.BranchID) {
		return models.Manifest{}, apperrors.BadRequest("A permitted branchId is required.", nil)
	}
	id := ids.New(repositories.PrefixManifest)
	var out models.Manifest
	e := s.tx(ctx, func(tx pgx.Tx) error {
		m := models.Manifest{ID: id, OrganizationID: t.OrganizationID, BranchID: in.BranchID, ManifestReference: strings.ToUpper(strings.ReplaceAll(id, "_", "-")), RouteReference: strings.TrimSpace(in.RouteReference), VehicleReference: strings.TrimSpace(in.VehicleReference), ProviderMembershipID: strings.TrimSpace(in.ProviderMembershipID), SealNumber: strings.TrimSpace(in.SealNumber), Status: models.StatusDraft, CreatedBy: t.ActorID}
		var x error
		out, x = s.opts.Repository.Create(ctx, tx, m)
		if x != nil {
			return x
		}
		if x = s.opts.Repository.AppendHistory(ctx, tx, t.OrganizationID, out.ID, "", models.StatusDraft, "CREATED", string(t.ActorType), t.ActorID, t.CorrelationID); x != nil {
			return x
		}
		if x = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "manifests.created", "manifest", out.ID)); x != nil {
			return x
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "manifest.created.v1", "manifest", out.ID, out.Version, t.OrganizationID, t.CorrelationID, map[string]any{"manifestId": out.ID})
		}
		return nil
	})
	return out, e
}
func (s *Service) Get(ctx context.Context, t tenant.Context, id string) (models.Manifest, []models.Item, error) {
	if e := s.gate(false); e != nil {
		return models.Manifest{}, nil, e
	}
	m, e := s.opts.Repository.Get(ctx, nil, t.OrganizationID, id)
	if errors.Is(e, repositories.ErrNotFound) || e == nil && !t.BranchPermitted(m.BranchID) {
		return models.Manifest{}, nil, nf()
	}
	if e != nil {
		return m, nil, e
	}
	items, e := s.opts.Repository.ListItems(ctx, nil, t.OrganizationID, id)
	return m, items, e
}
func (s *Service) List(ctx context.Context, t tenant.Context, status, branch string) ([]models.Manifest, error) {
	if e := s.gate(false); e != nil {
		return nil, e
	}
	if branch != "" && !t.BranchPermitted(branch) {
		return []models.Manifest{}, nil
	}
	return s.opts.Repository.List(ctx, t.OrganizationID, status, branch)
}

type UpdateInput struct {
	RouteReference, VehicleReference, ProviderMembershipID, SealNumber string
	ExpectedVersion                                                    int64
}

func (s *Service) Update(ctx context.Context, t tenant.Context, id string, in UpdateInput) (models.Manifest, error) {
	if e := s.gate(true); e != nil {
		return models.Manifest{}, e
	}
	if in.ExpectedVersion <= 0 {
		return models.Manifest{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	var out models.Manifest
	e := s.tx(ctx, func(tx pgx.Tx) error {
		m, x := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, id)
		if x != nil || !t.BranchPermitted(m.BranchID) {
			return nf()
		}
		if !models.Editable(m.Status) {
			return errorcodes.Conflict("manifest_invalid_state", "Closed manifests cannot be edited.")
		}
		if m.Version != in.ExpectedVersion {
			return concurrency.ErrVersionMismatch
		}
		out, x = s.opts.Repository.Update(ctx, tx, t.OrganizationID, id, m.Version, strings.TrimSpace(in.RouteReference), strings.TrimSpace(in.VehicleReference), strings.TrimSpace(in.ProviderMembershipID), strings.TrimSpace(in.SealNumber), t.ActorID)
		if x != nil {
			return x
		}
		return s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "manifests.updated", "manifest", id))
	})
	return out, e
}

type AddItemInput struct{ ItemType, ItemID string }

func (s *Service) AddItem(ctx context.Context, t tenant.Context, id string, in AddItemInput) (models.Item, error) {
	if e := s.gate(true); e != nil {
		return models.Item{}, e
	}
	kind := strings.ToUpper(strings.TrimSpace(in.ItemType))
	if kind != "SHIPMENT" && kind != "PACKAGE" && kind != "ORDER_STOP" {
		return models.Item{}, apperrors.BadRequest("itemType is unsupported.", nil)
	}
	var out models.Item
	e := s.tx(ctx, func(tx pgx.Tx) error {
		m, x := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, id)
		if x != nil || !t.BranchPermitted(m.BranchID) {
			return nf()
		}
		if !models.Editable(m.Status) {
			return errorcodes.Conflict("manifest_invalid_state", "Manifest items are locked.")
		}
		info, x := s.opts.Resources.Resolve(ctx, tx, t.OrganizationID, kind, strings.TrimSpace(in.ItemID))
		if x != nil || info.ID == "" || info.BranchID != "" && info.BranchID != m.BranchID {
			return errorcodes.Conflict("manifest_item_conflict", "Manifest item is invalid for this organization or branch.")
		}
		active, x := s.opts.Repository.HasActiveMembership(ctx, tx, t.OrganizationID, kind, info.ID, id)
		if x != nil {
			return x
		}
		if active {
			return errorcodes.Conflict("manifest_item_conflict", "Item already belongs to an active manifest.")
		}
		out, x = s.opts.Repository.AddItem(ctx, tx, models.Item{ID: ids.New(repositories.PrefixItem), OrganizationID: t.OrganizationID, ManifestID: id, ItemType: kind, ItemID: info.ID, ShipmentID: info.ShipmentID, AddedBy: t.ActorID})
		if errors.Is(x, repositories.ErrConflict) {
			return errorcodes.Conflict("manifest_item_conflict", "Item is already in this manifest.")
		}
		return x
	})
	return out, e
}

type CloseInput struct {
	ExpectedVersion int64
	Fingerprint     string
}

func (s *Service) Close(ctx context.Context, t tenant.Context, id string, in CloseInput) (models.Manifest, models.Snapshot, error) {
	if e := s.gate(true); e != nil {
		return models.Manifest{}, models.Snapshot{}, e
	}
	if in.ExpectedVersion <= 0 {
		return models.Manifest{}, models.Snapshot{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	var out models.Manifest
	var snap models.Snapshot
	e := s.tx(ctx, func(tx pgx.Tx) error {
		m, x := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, id)
		if x != nil || !t.BranchPermitted(m.BranchID) {
			return nf()
		}
		if m.Status == models.StatusClosed {
			existing, snapshotErr := s.opts.Repository.GetSnapshot(ctx, tx, t.OrganizationID, id)
			if snapshotErr != nil {
				return snapshotErr
			}
			if existing.RequestFingerprint != in.Fingerprint {
				return errorcodes.Conflict("manifest_already_closed", "Manifest was already closed by a different request.")
			}
			out, snap = m, existing
			return nil
		}
		if m.Version != in.ExpectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if m.Status == models.StatusDraft {
			m, x = s.opts.Repository.SetStatus(ctx, tx, t.OrganizationID, id, m.Version, models.StatusOpen, t.ActorID)
			if x != nil {
				return x
			}
		}
		if m.Status != models.StatusOpen {
			return errorcodes.Conflict("manifest_not_closeable", "Manifest is not open.")
		}
		items, x := s.opts.Repository.ListItems(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		if len(items) == 0 {
			return errorcodes.Conflict("manifest_not_closeable", "Manifest has no items.")
		}
		if (m.ProviderMembershipID != "" || m.VehicleReference != "") && s.opts.Authority == nil {
			return errorcodes.Unavailable("manifest_not_closeable", "Provider or vehicle authority is unavailable.")
		}
		if s.opts.Authority != nil {
			ok, x := s.opts.Authority.Validate(ctx, t.OrganizationID, m.ProviderMembershipID, m.VehicleReference)
			if x != nil || !ok {
				return errorcodes.Conflict("manifest_not_closeable", "Provider or vehicle validation failed.")
			}
		}
		for _, i := range items {
			active, x := s.opts.Repository.HasActiveMembership(ctx, tx, t.OrganizationID, i.ItemType, i.ItemID, id)
			if x != nil {
				return x
			}
			if active {
				return errorcodes.Conflict("manifest_item_conflict", "An item belongs to another active manifest.")
			}
		}
		snapshotItems := make([]map[string]any, 0, len(items))
		for _, i := range items {
			snapshotItems = append(snapshotItems, map[string]any{"itemType": i.ItemType, "itemId": i.ItemID, "shipmentId": i.ShipmentID})
		}
		snap, x = s.opts.Repository.CreateSnapshot(ctx, tx, models.Snapshot{OrganizationID: t.OrganizationID, ManifestID: id, ManifestVersion: m.Version + 1, Snapshot: map[string]any{"manifestId": id, "manifestReference": m.ManifestReference, "branchId": m.BranchID, "routeReference": m.RouteReference, "vehicleReference": m.VehicleReference, "providerMembershipId": m.ProviderMembershipID, "sealNumber": m.SealNumber, "items": snapshotItems}, RequestFingerprint: in.Fingerprint, CreatedBy: t.ActorID})
		if x != nil {
			return x
		}
		out, x = s.opts.Repository.SetStatus(ctx, tx, t.OrganizationID, id, m.Version, models.StatusClosed, t.ActorID)
		if x != nil {
			return x
		}
		if x = s.opts.Repository.AppendHistory(ctx, tx, t.OrganizationID, id, m.Status, models.StatusClosed, "CLOSED", string(t.ActorType), t.ActorID, t.CorrelationID); x != nil {
			return x
		}
		if x = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "manifests.closed", "manifest", id)); x != nil {
			return x
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "manifest.closed.v1", "manifest", id, out.Version, t.OrganizationID, t.CorrelationID, map[string]any{"manifestId": id, "snapshotId": snap.ID})
		}
		return nil
	})
	return out, snap, e
}

type SealAmendmentInput struct {
	SealNumber, ReasonCode string
	ExpectedVersion        int64
}

func (s *Service) AmendSeal(ctx context.Context, t tenant.Context, id string, in SealAmendmentInput) (models.Manifest, error) {
	if e := s.gate(true); e != nil {
		return models.Manifest{}, e
	}
	if in.ExpectedVersion <= 0 || strings.TrimSpace(in.SealNumber) == "" || strings.TrimSpace(in.ReasonCode) == "" {
		return models.Manifest{}, apperrors.BadRequest("expectedVersion, sealNumber and reasonCode are required.", nil)
	}
	var out models.Manifest
	e := s.tx(ctx, func(tx pgx.Tx) error {
		m, x := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, id)
		if x != nil || !t.BranchPermitted(m.BranchID) {
			return nf()
		}
		if m.Status != models.StatusClosed {
			return errorcodes.Conflict("manifest_invalid_state", "Only a closed manifest seal may be amended.")
		}
		if m.Version != in.ExpectedVersion {
			return concurrency.ErrVersionMismatch
		}
		out, x = s.opts.Repository.AmendSeal(ctx, tx, t.OrganizationID, id, m.Version,
			strings.TrimSpace(in.SealNumber), t.ActorID)
		if x != nil {
			return x
		}
		if x = s.opts.Repository.AppendHistory(ctx, tx, t.OrganizationID, id, m.Status, m.Status,
			"SEAL_AMENDED:"+strings.TrimSpace(in.ReasonCode), string(t.ActorType), t.ActorID, t.CorrelationID); x != nil {
			return x
		}
		return s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "manifests.seal_amended", "manifest", id))
	})
	return out, e
}

type CancelInput struct {
	ExpectedVersion int64
	ReasonCode      string
}

func (s *Service) Cancel(ctx context.Context, t tenant.Context, id string, in CancelInput) (models.Manifest, error) {
	if e := s.gate(true); e != nil {
		return models.Manifest{}, e
	}
	if in.ExpectedVersion <= 0 || strings.TrimSpace(in.ReasonCode) == "" {
		return models.Manifest{}, apperrors.BadRequest("expectedVersion and reasonCode are required.", nil)
	}
	var out models.Manifest
	e := s.tx(ctx, func(tx pgx.Tx) error {
		m, x := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, id)
		if x != nil || !t.BranchPermitted(m.BranchID) {
			return nf()
		}
		if m.Version != in.ExpectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if !models.CanTransition(m.Status, models.StatusCancelled) {
			return errorcodes.Conflict("manifest_invalid_state", "Manifest can no longer be cancelled.")
		}
		out, x = s.opts.Repository.SetStatus(ctx, tx, t.OrganizationID, id, m.Version,
			models.StatusCancelled, t.ActorID)
		if x != nil {
			return x
		}
		if x = s.opts.Repository.AppendHistory(ctx, tx, t.OrganizationID, id, m.Status,
			models.StatusCancelled, strings.TrimSpace(in.ReasonCode), string(t.ActorType),
			t.ActorID, t.CorrelationID); x != nil {
			return x
		}
		return s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "manifests.cancelled", "manifest", id))
	})
	return out, e
}
