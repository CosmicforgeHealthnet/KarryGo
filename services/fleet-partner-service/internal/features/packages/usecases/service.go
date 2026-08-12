// Package usecases implements package, scan and custody workflows.
package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

type ShipmentInfo struct {
	ID, BranchID, Status string
	Editable             bool
}

var ErrShipmentNotFound = errors.New("packages: shipment not found")

type ShipmentsPort interface {
	GetShipment(context.Context, database.Querier, string, string) (ShipmentInfo, error)
}
type MediaPort interface {
	ValidateEvidence(context.Context, tenant.Context, string) (bool, error)
}
type DevicePort interface {
	ValidateTrustedDevice(context.Context, tenant.Context, string) (bool, error)
}
type ProviderScopePort interface {
	ValidatePackageScope(context.Context, tenant.Context, string, string) (bool, error)
}
type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type Flags struct{ Enabled, WritesEnabled, ScanningEnabled, KillSwitch bool }
type Options struct {
	Pool          *pgxpool.Pool
	Repository    *repositories.Repository
	Shipments     ShipmentsPort
	Media         MediaPort
	Devices       DevicePort
	ProviderScope ProviderScopePort
	Events        EventPublisher
	Audit         *audit.Recorder
	Flags         Flags
	Now           func() time.Time
}
type Service struct{ opts Options }

func NewService(o Options) *Service {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	return &Service{opts: o}
}
func (s *Service) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.opts.Pool, fn)
}
func (s *Service) enabled(write bool) error {
	if s.opts.Flags.KillSwitch {
		return errorcodes.Unavailable("packages_disabled", "Package operations are temporarily unavailable.")
	}
	if !s.opts.Flags.Enabled {
		return errorcodes.NotFound("package_not_found", "Package not found.")
	}
	if write && !s.opts.Flags.WritesEnabled {
		return errorcodes.Forbidden("package_invalid_state", "Package writes are disabled.")
	}
	return nil
}
func notFound() error                                    { return errorcodes.NotFound("package_not_found", "Package not found.") }
func branchAllowed(t tenant.Context, branch string) bool { return t.BranchPermitted(branch) }

type CreateInput struct {
	ShipmentID, PackageType, Length, Width, Height, DimensionUnit, Weight, WeightUnit string
	DeclaredValueAmountMinor                                                          int64
	DeclaredValueCurrency, HandlingInstructions                                       string
}

func (s *Service) Create(ctx context.Context, t tenant.Context, in CreateInput) (models.Package, error) {
	if err := s.enabled(true); err != nil {
		return models.Package{}, err
	}
	if !models.ValidPositiveDecimal(in.Length) || !models.ValidPositiveDecimal(in.Width) || !models.ValidPositiveDecimal(in.Height) || !models.ValidPositiveDecimal(in.Weight) {
		return models.Package{}, apperrors.BadRequest("Dimensions and weight must be positive decimal values with at most three decimal places.", nil)
	}
	dim := strings.ToUpper(strings.TrimSpace(in.DimensionUnit))
	weightUnit := strings.ToUpper(strings.TrimSpace(in.WeightUnit))
	if dim != "CM" && dim != "M" && dim != "IN" {
		return models.Package{}, apperrors.BadRequest("dimensionUnit must be CM, M or IN.", nil)
	}
	if weightUnit != "KG" && weightUnit != "G" && weightUnit != "LB" {
		return models.Package{}, apperrors.BadRequest("weightUnit must be KG, G or LB.", nil)
	}
	currency := strings.ToUpper(strings.TrimSpace(in.DeclaredValueCurrency))
	if len(currency) != 3 || in.DeclaredValueAmountMinor < 0 {
		return models.Package{}, apperrors.BadRequest("Declared value is invalid.", nil)
	}
	var created models.Package
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		shipment, e := s.opts.Shipments.GetShipment(ctx, tx, t.OrganizationID, strings.TrimSpace(in.ShipmentID))
		if e != nil {
			return notFound()
		}
		if !branchAllowed(t, shipment.BranchID) {
			return notFound()
		}
		if !shipment.Editable {
			return errorcodes.Conflict("package_invalid_state", "Packages cannot be added in the shipment's current state.")
		}
		id := ids.New(repositories.PrefixPackage)
		identifier := strings.ToUpper(strings.ReplaceAll(id, "_", "-"))
		p := models.Package{ID: id, OrganizationID: t.OrganizationID, BranchID: shipment.BranchID, ShipmentID: shipment.ID, PackageCode: identifier, BarcodeValue: "PKG-" + identifier, QRValue: "KARRYGO:" + identifier, PackageType: strings.ToUpper(strings.TrimSpace(in.PackageType)), Length: in.Length, Width: in.Width, Height: in.Height, DimensionUnit: dim, Weight: in.Weight, WeightUnit: weightUnit, DeclaredValueAmountMinor: in.DeclaredValueAmountMinor, DeclaredValueCurrency: currency, HandlingInstructions: strings.TrimSpace(in.HandlingInstructions), Condition: "GOOD", Status: models.StatusCreated, CreatedBy: t.ActorID}
		if p.PackageType == "" {
			p.PackageType = "PARCEL"
		}
		created, e = s.opts.Repository.Create(ctx, tx, p)
		if errors.Is(e, repositories.ErrIdentifierConflict) {
			return errorcodes.Conflict("package_identifier_conflict", "Package identifiers already exist.")
		}
		if e != nil {
			return e
		}
		if e = s.opts.Repository.IncrementShipmentPackageCount(ctx, tx, t.OrganizationID, shipment.ID); e != nil {
			return e
		}
		e = s.opts.Repository.AppendHistory(ctx, tx, models.History{OrganizationID: t.OrganizationID, PackageID: created.ID, PreviousStatus: "", NewStatus: models.StatusCreated, ReasonCode: "CREATED", ActorType: string(t.ActorType), ActorReference: t.ActorID, Source: "CORPORATE_API", RequestID: t.RequestID, CorrelationID: t.CorrelationID, OccurredAt: s.opts.Now().UTC()})
		if e != nil {
			return e
		}
		if e = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "packages.created", "package", created.ID)); e != nil {
			return e
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "package.created.v1", "package", created.ID, created.Version, t.OrganizationID, t.CorrelationID, map[string]any{"packageId": created.ID, "shipmentId": created.ShipmentID, "status": created.Status})
		}
		return nil
	})
	return created, err
}
func (s *Service) Get(ctx context.Context, t tenant.Context, id string) (models.Package, error) {
	if e := s.enabled(false); e != nil {
		return models.Package{}, e
	}
	p, e := s.opts.Repository.Get(ctx, nil, t.OrganizationID, id)
	if errors.Is(e, repositories.ErrNotFound) || e == nil && !branchAllowed(t, p.BranchID) {
		return models.Package{}, notFound()
	}
	return p, e
}
func (s *Service) List(ctx context.Context, t tenant.Context, shipment string) ([]models.Package, error) {
	if e := s.enabled(false); e != nil {
		return nil, e
	}
	info, e := s.opts.Shipments.GetShipment(ctx, nil, t.OrganizationID, shipment)
	if e != nil || !branchAllowed(t, info.BranchID) {
		return nil, notFound()
	}
	return s.opts.Repository.ListByShipment(ctx, nil, t.OrganizationID, shipment)
}
func (s *Service) History(ctx context.Context, t tenant.Context, id string) ([]models.History, error) {
	if _, e := s.Get(ctx, t, id); e != nil {
		return nil, e
	}
	return s.opts.Repository.ListHistory(ctx, t.OrganizationID, id)
}

type TransitionInput struct {
	NewStatus, ReasonCode, SafeNote, Source string
	ExpectedVersion                         int64
	AuthoritativeDeliveryConfirmed          bool
}

func (s *Service) Transition(ctx context.Context, t tenant.Context, id string, in TransitionInput) (models.Package, error) {
	if e := s.enabled(true); e != nil {
		return models.Package{}, e
	}
	if in.ExpectedVersion <= 0 {
		return models.Package{}, apperrors.BadRequest("Provide expectedVersion or If-Match.", nil)
	}
	to := strings.ToUpper(strings.TrimSpace(in.NewStatus))
	if models.Exceptional(to) && strings.TrimSpace(in.ReasonCode) == "" {
		return models.Package{}, apperrors.BadRequest("Exceptional package states require a reasonCode.", nil)
	}
	if to == models.StatusDelivered && !in.AuthoritativeDeliveryConfirmed {
		return models.Package{}, errorcodes.Conflict("package_invalid_state", "Delivery requires authoritative execution or proof confirmation.")
	}
	var updated models.Package
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		current, e := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, id)
		if e != nil {
			return notFound()
		}
		if !branchAllowed(t, current.BranchID) {
			return notFound()
		}
		if current.Version != in.ExpectedVersion {
			return concurrency.ErrVersionMismatch
		}
		if !models.CanTransition(current.Status, to) {
			return errorcodes.Conflict("package_transition_invalid", "The requested package transition is not allowed.")
		}
		condition := current.Condition
		if to == models.StatusDamaged {
			condition = "DAMAGED"
		}
		updated, e = s.opts.Repository.Transition(ctx, tx, t.OrganizationID, id, current.Version, to, condition, t.ActorID)
		if e != nil {
			return e
		}
		now := s.opts.Now().UTC()
		e = s.opts.Repository.AppendHistory(ctx, tx, models.History{OrganizationID: t.OrganizationID, PackageID: id, PreviousStatus: current.Status, NewStatus: to, ReasonCode: strings.TrimSpace(in.ReasonCode), SafeNote: strings.TrimSpace(in.SafeNote), ActorType: string(t.ActorType), ActorReference: t.ActorID, Source: strings.TrimSpace(in.Source), RequestID: t.RequestID, CorrelationID: t.CorrelationID, OccurredAt: now})
		if e != nil {
			return e
		}
		if e = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "packages.status_changed", "package", id)); e != nil {
			return e
		}
		if s.opts.Events != nil {
			eventType := "package.status_changed.v1"
			if models.Exceptional(to) {
				eventType = "package.exception_recorded.v1"
			}
			return s.opts.Events.Publish(ctx, tx, eventType, "package", id, updated.Version, t.OrganizationID, t.CorrelationID, map[string]any{"packageId": id, "previousStatus": current.Status, "status": to, "reasonCode": in.ReasonCode})
		}
		return nil
	})
	return updated, err
}

var scanTransitions = map[string]string{"RECEIVED": models.StatusReceived, "SORTED": models.StatusSorted, "LOADED": models.StatusLoaded, "IN_TRANSIT": models.StatusInTransit, "OUT_FOR_DELIVERY": models.StatusOutForDelivery, "FAILED_ATTEMPT": models.StatusFailedAttempt, "RETURNING": models.StatusReturning, "RETURNED": models.StatusReturned, "DAMAGED": models.StatusDamaged, "LOST": models.StatusLost, "QUARANTINED": models.StatusQuarantined}

type ScanInput struct {
	ScanType, Barcode, QR, LocationReference, DeviceReference, ClientScanID, ReasonCode string
	OccurredAt                                                                          time.Time
	ExpectedVersion                                                                     int64
}

func (s *Service) Scan(ctx context.Context, t tenant.Context, id string, in ScanInput) (models.Scan, error) {
	if e := s.enabled(true); e != nil {
		return models.Scan{}, e
	}
	if !s.opts.Flags.ScanningEnabled {
		return models.Scan{}, errorcodes.Forbidden("package_scan_requires_review", "Package scanning is disabled.")
	}
	in.ScanType = strings.ToUpper(strings.TrimSpace(in.ScanType))
	to, ok := scanTransitions[in.ScanType]
	if !ok {
		return models.Scan{}, apperrors.BadRequest("scanType is not supported.", nil)
	}
	if strings.TrimSpace(in.ClientScanID) == "" || in.OccurredAt.IsZero() {
		return models.Scan{}, apperrors.BadRequest("clientScanId and occurredAt are required.", nil)
	}
	if strings.TrimSpace(in.DeviceReference) == "" || s.opts.Devices == nil {
		return models.Scan{}, errorcodes.Forbidden("package_scan_device_untrusted", "A trusted scanning device is required.")
	}
	trusted, deviceErr := s.opts.Devices.ValidateTrustedDevice(ctx, t, strings.TrimSpace(in.DeviceReference))
	if deviceErr != nil || !trusted {
		return models.Scan{}, errorcodes.Forbidden("package_scan_device_untrusted", "The scanning device is not trusted.")
	}
	fingerprint := models.ScanFingerprint(id, in.ScanType, in.Barcode, in.QR, in.LocationReference, in.DeviceReference, in.ClientScanID, in.OccurredAt)
	if prior, e := s.opts.Repository.GetScanByClientID(ctx, nil, t.OrganizationID, in.ClientScanID); e == nil {
		if prior.RequestFingerprint != fingerprint {
			return models.Scan{}, errorcodes.Conflict("package_scan_conflict", "clientScanId was reused with changed scan data.")
		}
		return prior, nil
	}
	var scan models.Scan
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		p, e := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, id)
		if e != nil {
			return notFound()
		}
		if !branchAllowed(t, p.BranchID) {
			return notFound()
		}
		if t.ActorType == tenant.ActorTypePlatformService {
			if s.opts.ProviderScope == nil {
				return errorcodes.Forbidden("package_not_in_provider_scope", "Package is not assigned to this provider.")
			}
			allowed, scopeErr := s.opts.ProviderScope.ValidatePackageScope(ctx, t, p.ShipmentID, id)
			if scopeErr != nil || !allowed {
				return errorcodes.Forbidden("package_not_in_provider_scope", "Package is not assigned to this provider.")
			}
		}
		if in.Barcode != "" && in.Barcode != p.BarcodeValue {
			return errorcodes.Conflict("package_barcode_mismatch", "Barcode does not match this package.")
		}
		if in.QR != "" && in.QR != p.QRValue {
			return errorcodes.Conflict("package_qr_mismatch", "QR value does not match this package.")
		}
		if in.Barcode == "" && in.QR == "" {
			return apperrors.BadRequest("barcode or qr is required.", nil)
		}
		disposition := "APPLIED"
		latest, e := s.opts.Repository.LatestScan(ctx, tx, t.OrganizationID, id)
		if e == nil && in.OccurredAt.Before(latest.OccurredAt) {
			disposition = "REVIEW_REQUIRED"
		}
		if disposition == "APPLIED" && !models.CanTransition(p.Status, to) {
			disposition = "REVIEW_REQUIRED"
		}
		scan, e = s.opts.Repository.CreateScan(ctx, tx, models.Scan{ID: ids.New(repositories.PrefixScan), OrganizationID: t.OrganizationID, PackageID: id, ScanType: in.ScanType, BarcodeValue: in.Barcode, QRValue: in.QR, LocationReference: strings.TrimSpace(in.LocationReference), DeviceReference: strings.TrimSpace(in.DeviceReference), ClientScanID: strings.TrimSpace(in.ClientScanID), RequestFingerprint: fingerprint, Disposition: disposition, ActorType: string(t.ActorType), ActorReference: t.ActorID, OccurredAt: in.OccurredAt.UTC()})
		if errors.Is(e, repositories.ErrScanConflict) {
			return errorcodes.Conflict("package_scan_conflict", "clientScanId already exists.")
		}
		if e != nil {
			return e
		}
		version := p.Version
		if disposition == "APPLIED" {
			updated, e := s.opts.Repository.Transition(ctx, tx, t.OrganizationID, id, p.Version, to, p.Condition, t.ActorID)
			if e != nil {
				return e
			}
			version = updated.Version
			e = s.opts.Repository.AppendHistory(ctx, tx, models.History{OrganizationID: t.OrganizationID, PackageID: id, PreviousStatus: p.Status, NewStatus: to, ReasonCode: in.ReasonCode, ActorType: string(t.ActorType), ActorReference: t.ActorID, Source: "PACKAGE_SCAN", RequestID: t.RequestID, CorrelationID: t.CorrelationID, OccurredAt: in.OccurredAt.UTC()})
			if e != nil {
				return e
			}
		}
		if e = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "packages.scanned", "package", id)); e != nil {
			return e
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "package.scanned.v1", "package", id, version, t.OrganizationID, t.CorrelationID, map[string]any{"packageId": id, "scanId": scan.ID, "scanType": in.ScanType, "disposition": disposition})
		}
		return nil
	})
	return scan, err
}

type CustodyInput struct {
	FromActorType, FromActorReference, ToActorType, ToActorReference, FromLocationReference, ToLocationReference, Condition, EvidenceMediaID, CorrectionOfEventID, Source string
	OccurredAt                                                                                                                                                            time.Time
}

func (s *Service) RecordCustody(ctx context.Context, t tenant.Context, id string, in CustodyInput) (models.CustodyEvent, error) {
	if e := s.enabled(true); e != nil {
		return models.CustodyEvent{}, e
	}
	if in.OccurredAt.IsZero() || strings.TrimSpace(in.ToActorType) == "" || strings.TrimSpace(in.ToActorReference) == "" {
		return models.CustodyEvent{}, apperrors.BadRequest("occurredAt and receiving actor are required.", nil)
	}
	if in.EvidenceMediaID != "" {
		if s.opts.Media == nil {
			return models.CustodyEvent{}, apperrors.Unavailable("Media evidence validation is unavailable.", nil)
		}
		ok, e := s.opts.Media.ValidateEvidence(ctx, t, in.EvidenceMediaID)
		if e != nil || !ok {
			return models.CustodyEvent{}, apperrors.Unavailable("Media evidence could not be validated.", e)
		}
	}
	var event models.CustodyEvent
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		p, e := s.opts.Repository.Lock(ctx, tx, t.OrganizationID, id)
		if e != nil || !branchAllowed(t, p.BranchID) {
			return notFound()
		}
		latest, latestErr := s.opts.Repository.LatestCustody(ctx, tx, t.OrganizationID, id)
		if latestErr == nil && strings.TrimSpace(in.CorrectionOfEventID) == "" {
			if latest.ToActorType != strings.TrimSpace(in.FromActorType) ||
				latest.ToActorReference != strings.TrimSpace(in.FromActorReference) {
				return errorcodes.Conflict("custody_sequence_invalid", "Custody must continue from the current holder.")
			}
			if in.OccurredAt.Before(latest.OccurredAt) {
				return errorcodes.Conflict("custody_sequence_invalid", "Custody events must be chronological.")
			}
		} else if latestErr != nil && !errors.Is(latestErr, repositories.ErrNotFound) {
			return latestErr
		}
		if correctionID := strings.TrimSpace(in.CorrectionOfEventID); correctionID != "" {
			corrected, correctionErr := s.opts.Repository.GetCustody(ctx, tx, t.OrganizationID, id, correctionID)
			if correctionErr != nil || corrected.ID == "" {
				return errorcodes.Conflict("custody_correction_invalid", "Correction target does not belong to this package.")
			}
		}
		event, e = s.opts.Repository.CreateCustody(ctx, tx, models.CustodyEvent{OrganizationID: t.OrganizationID, PackageID: id, ShipmentID: p.ShipmentID, FromActorType: in.FromActorType, FromActorReference: in.FromActorReference, ToActorType: in.ToActorType, ToActorReference: in.ToActorReference, FromLocationReference: in.FromLocationReference, ToLocationReference: in.ToLocationReference, Condition: in.Condition, EvidenceMediaID: in.EvidenceMediaID, CorrectionOfEventID: in.CorrectionOfEventID, OccurredAt: in.OccurredAt.UTC(), Source: in.Source, CorrelationID: t.CorrelationID})
		if e != nil {
			return e
		}
		if e = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "custody.recorded", "custody_event", event.ID)); e != nil {
			return e
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "custody_event.recorded.v1", "package", id, p.Version, t.OrganizationID, t.CorrelationID, map[string]any{"packageId": id, "custodyEventId": event.ID})
		}
		return nil
	})
	return event, err
}
func (s *Service) Custody(ctx context.Context, t tenant.Context, id string) ([]models.CustodyEvent, error) {
	if _, e := s.Get(ctx, t, id); e != nil {
		return nil, e
	}
	return s.opts.Repository.ListCustody(ctx, t.OrganizationID, id)
}
