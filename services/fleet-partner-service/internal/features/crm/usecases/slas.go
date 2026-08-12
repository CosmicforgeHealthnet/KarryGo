package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// CreateSLAInput opens a draft service level agreement.
type CreateSLAInput struct {
	CustomerID                 string
	ContractID                 string
	FutureServiceCatalogItemID string
	MetricCode                 string
	TargetValue                string
	MeasurementUnit            string
	MeasurementWindow          string
	EscalationPolicy           models.JSONMap
	EffectiveFrom              time.Time
	EffectiveTo                *time.Time
	IdempotencyKey             string
	CorrelationID              string
	CausationID                string
}

// CreateSLA opens a draft service level agreement.
//
// FutureServiceCatalogItemID is an opaque forward reference. Section 15 owns
// the service catalogue; nothing here resolves the value, and storing it now
// only means an operator does not have to restate their commitments later.
func (s *Service) CreateSLA(ctx context.Context, t tenant.Context,
	input CreateSLAInput) (models.SLA, error) {
	if err := s.writeEnabled(s.opts.Flags.SLAEnabled); err != nil {
		return models.SLA{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.SLA{}, err
	}
	if err := validateSLADefinition(input.MetricCode, input.MeasurementUnit,
		input.MeasurementWindow, input.TargetValue); err != nil {
		return models.SLA{}, err
	}
	if input.EffectiveTo != nil && !input.EffectiveTo.After(input.EffectiveFrom) {
		return models.SLA{}, badRequest(errorcodes.SLAInvalidState,
			"effectiveTo must be after effectiveFrom.")
	}

	var created models.SLA
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		customer, txErr := s.requireCustomer(ctx, tx, t, input.CustomerID)
		if txErr != nil {
			return txErr
		}
		if customer.IsArchived() {
			return conflict(errorcodes.CustomerArchived,
				"An archived customer cannot have new service level agreements.")
		}
		if input.ContractID != "" {
			contract, contractErr := s.opts.Repository.GetContract(ctx, tx, t.OrganizationID,
				input.ContractID)
			if contractErr != nil || contract.CustomerID != input.CustomerID {
				return errorcodes.NotFound(errorcodes.ContractNotFound,
					"Contract was not found.")
			}
		}
		created, txErr = s.opts.Repository.CreateSLA(ctx, tx, models.SLA{
			ID:                         ids.New(repositories.PrefixSLA),
			OrganizationID:             t.OrganizationID,
			CustomerID:                 input.CustomerID,
			ContractID:                 input.ContractID,
			FutureServiceCatalogItemID: input.FutureServiceCatalogItemID,
			MetricCode:                 input.MetricCode,
			TargetValue:                input.TargetValue,
			MeasurementUnit:            input.MeasurementUnit,
			MeasurementWindow:          input.MeasurementWindow,
			EscalationPolicy:           input.EscalationPolicy,
			EffectiveFrom:              input.EffectiveFrom,
			EffectiveTo:                input.EffectiveTo,
			Status:                     models.SLADraft,
			CreatedBy:                  t.OrganizationUserID,
		})
		if txErr != nil {
			return translate(txErr, "Service level agreement", errorcodes.SLANotFound,
				errorcodes.SLAInvalidState)
		}
		if txErr = s.opts.Repository.AppendSLAHistory(ctx, tx, models.SLAHistory{
			OrganizationID: t.OrganizationID, SLAID: created.ID, PreviousStatus: "",
			NewStatus: created.Status, ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.sla.created", "service_level_agreement",
			created.ID, nil, slaAuditValues(created), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventSLACreated, "service_level_agreement", created.ID,
			created.Version, t.OrganizationID, input.CorrelationID, slaEventPayload(created))
	})
	if err != nil {
		return models.SLA{}, err
	}
	return created, nil
}

// validateSLADefinition refuses a commitment nobody can evaluate.
//
// The metric list is closed and each metric admits only the units it can
// actually be measured in. An SLA with an unrecognised metric or a unit that
// does not fit it would produce UNKNOWN forever, which is a promise the
// operator cannot keep and the customer cannot check.
func validateSLADefinition(metricCode, unit, window, target string) error {
	if !models.ValidMetric(metricCode) {
		return badRequest(errorcodes.SLAInvalidState,
			"metricCode is not supported. Supported metrics: "+
				strings.Join(models.SupportedMetrics(), ", ")+".")
	}
	if !models.ValidMetricUnit(metricCode, unit) {
		return badRequest(errorcodes.SLAInvalidState,
			"measurementUnit is not valid for this metric.")
	}
	if !models.ValidMeasurementWindow(window) {
		return badRequest(errorcodes.SLAInvalidState, "measurementWindow is not supported.")
	}
	if _, err := models.NormalizeQuantity(target); err != nil {
		return badRequest(errorcodes.SLAInvalidState,
			"targetValue must be a positive decimal.")
	}
	return nil
}

// GetSLA reads one service level agreement.
func (s *Service) GetSLA(ctx context.Context, t tenant.Context,
	slaID string) (models.SLA, error) {
	if err := s.readEnabled(); err != nil {
		return models.SLA{}, err
	}
	sla, err := s.opts.Repository.GetSLA(ctx, nil, t.OrganizationID, slaID)
	return sla, translate(err, "Service level agreement", errorcodes.SLANotFound,
		errorcodes.SLAInvalidState)
}

// ListSLAs returns a tenant-scoped page.
func (s *Service) ListSLAs(ctx context.Context, t tenant.Context,
	filter repositories.SLAFilter) ([]models.SLA, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if filter.MetricCode != "" && !models.ValidMetric(filter.MetricCode) {
		return nil, badRequest(errorcodes.SLAInvalidState, "metricCode filter is not supported.")
	}
	return s.opts.Repository.ListSLAs(ctx, t.OrganizationID, filter)
}

// UpdateSLAInput carries a partial draft SLA update.
type UpdateSLAInput struct {
	TargetValue       *string
	MeasurementUnit   *string
	MeasurementWindow *string
	EscalationPolicy  models.JSONMap
	EffectiveFrom     *time.Time
	EffectiveTo       *time.Time
	ExpectedVersion   int64
	CorrelationID     string
}

// UpdateSLA edits a draft service level agreement.
func (s *Service) UpdateSLA(ctx context.Context, t tenant.Context, slaID string,
	input UpdateSLAInput) (models.SLA, error) {
	if err := s.writeEnabled(s.opts.Flags.SLAEnabled); err != nil {
		return models.SLA{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.SLA{}, err
	}

	var updated models.SLA
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.opts.Repository.LockSLA(ctx, tx, t.OrganizationID, slaID)
		if txErr != nil {
			return translate(txErr, "Service level agreement", errorcodes.SLANotFound,
				errorcodes.SLAInvalidState)
		}
		if existing.Status != models.SLADraft {
			return conflict(errorcodes.SLAInvalidState,
				"Only a draft service level agreement can be edited.")
		}
		unit := existing.MeasurementUnit
		if input.MeasurementUnit != nil {
			unit = *input.MeasurementUnit
		}
		window := existing.MeasurementWindow
		if input.MeasurementWindow != nil {
			window = *input.MeasurementWindow
		}
		target := existing.TargetValue
		if input.TargetValue != nil {
			target = *input.TargetValue
		}
		if txErr = validateSLADefinition(existing.MetricCode, unit, window, target); txErr != nil {
			return txErr
		}

		updated, txErr = s.opts.Repository.UpdateSLA(ctx, tx, t.OrganizationID, slaID,
			input.ExpectedVersion, repositories.SLAUpdate{
				TargetValue: input.TargetValue, MeasurementUnit: input.MeasurementUnit,
				MeasurementWindow: input.MeasurementWindow,
				EscalationPolicy:  input.EscalationPolicy,
				EffectiveFrom:     input.EffectiveFrom, EffectiveTo: input.EffectiveTo,
				UpdatedBy: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Service level agreement", errorcodes.SLANotFound,
				errorcodes.SLAInvalidState)
		}
		if txErr = s.record(ctx, tx, t, "crm.sla.updated", "service_level_agreement", slaID,
			slaAuditValues(existing), slaAuditValues(updated), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventSLACreated, "service_level_agreement", slaID,
			updated.Version, t.OrganizationID, input.CorrelationID, slaEventPayload(updated))
	})
	if err != nil {
		return models.SLA{}, err
	}
	return updated, nil
}

// SLALifecycleInput is a controlled SLA status change.
type SLALifecycleInput struct {
	ReasonCode      string
	ExpectedVersion int64
	IdempotencyKey  string
	CorrelationID   string
	CausationID     string
}

// ActivateSLA brings a service level agreement into force.
//
// Where the SLA is attached to a contract, that contract must be active: a
// service commitment under an agreement that is not in force is a commitment to
// nothing.
func (s *Service) ActivateSLA(ctx context.Context, t tenant.Context, slaID string,
	input SLALifecycleInput) (models.SLA, error) {
	return s.transitionSLA(ctx, t, slaID, models.SLAActive, EventSLAActivated,
		"crm.sla.activated", input)
}

// SuspendSLA pauses a service level agreement.
func (s *Service) SuspendSLA(ctx context.Context, t tenant.Context, slaID string,
	input SLALifecycleInput) (models.SLA, error) {
	return s.transitionSLA(ctx, t, slaID, models.SLASuspended, EventSLASuspended,
		"crm.sla.suspended", input)
}

// TerminateSLA ends a service level agreement.
func (s *Service) TerminateSLA(ctx context.Context, t tenant.Context, slaID string,
	input SLALifecycleInput) (models.SLA, error) {
	return s.transitionSLA(ctx, t, slaID, models.SLATerminated, EventSLASuspended,
		"crm.sla.terminated", input)
}

func (s *Service) transitionSLA(ctx context.Context, t tenant.Context, slaID, toStatus,
	eventType, auditAction string, input SLALifecycleInput) (models.SLA, error) {
	if err := s.writeEnabled(s.opts.Flags.SLAEnabled); err != nil {
		return models.SLA{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.SLA{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.SLA{}, err
	}

	var updated models.SLA
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.opts.Repository.LockSLA(ctx, tx, t.OrganizationID, slaID)
		if txErr != nil {
			return translate(txErr, "Service level agreement", errorcodes.SLANotFound,
				errorcodes.SLAInvalidState)
		}
		if existing.Status == toStatus {
			updated = existing
			return nil
		}
		if !models.CanTransitionSLA(existing.Status, toStatus) {
			return conflict(errorcodes.SLAInvalidState,
				"That change is not permitted from the agreement's current status.")
		}
		if toStatus == models.SLAActive {
			customer, customerErr := s.requireCustomer(ctx, tx, t, existing.CustomerID)
			if customerErr != nil {
				return customerErr
			}
			if customerErr = requireCommittableCustomer(customer); customerErr != nil {
				return customerErr
			}
			if existing.ContractID != "" {
				contract, contractErr := s.opts.Repository.GetContract(ctx, tx,
					t.OrganizationID, existing.ContractID)
				if contractErr != nil {
					return errorcodes.NotFound(errorcodes.ContractNotFound,
						"Contract was not found.")
				}
				if contract.Status != models.ContractActive {
					return conflict(errorcodes.ContractNotActive,
						"The contract must be active before its service level agreement is activated.")
				}
			}
		}

		updated, txErr = s.opts.Repository.TransitionSLA(ctx, tx, t.OrganizationID, slaID,
			input.ExpectedVersion, existing.Status, toStatus, t.OrganizationUserID)
		if txErr != nil {
			return translate(txErr, "Service level agreement", errorcodes.SLANotFound,
				errorcodes.SLAInvalidState)
		}
		if txErr = s.opts.Repository.AppendSLAHistory(ctx, tx, models.SLAHistory{
			OrganizationID: t.OrganizationID, SLAID: slaID, PreviousStatus: existing.Status,
			NewStatus: updated.Status, ReasonCode: input.ReasonCode,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, auditAction, "service_level_agreement", slaID,
			map[string]any{"status": existing.Status},
			map[string]any{"status": updated.Status}, input.ReasonCode); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, eventType, "service_level_agreement", slaID,
			updated.Version, t.OrganizationID, input.CorrelationID, slaEventPayload(updated))
	})
	if err != nil {
		return models.SLA{}, err
	}
	return updated, nil
}

// SLAHistory returns a page of an SLA's status changes.
func (s *Service) SLAHistory(ctx context.Context, t tenant.Context, slaID string,
	page cursor.Cursor, hasCursor bool, limit int) ([]models.SLAHistory, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.GetSLA(ctx, t, slaID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListSLAHistory(ctx, t.OrganizationID, slaID, page, hasCursor,
		limit)
}

// SLAObservations returns a page of an SLA's measurements.
func (s *Service) SLAObservations(ctx context.Context, t tenant.Context, slaID, outcome string,
	page cursor.Cursor, hasCursor bool, limit int) ([]models.SLAObservation, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.GetSLA(ctx, t, slaID); err != nil {
		return nil, err
	}
	if outcome != "" && !models.ValidOutcome(outcome) {
		return nil, badRequest(errorcodes.SLAInvalidState, "outcome filter is not supported.")
	}
	return s.opts.Repository.ListObservations(ctx, t.OrganizationID, slaID, outcome, page,
		hasCursor, limit)
}

// IngestObservationInput is one measurement from an authoritative source.
type IngestObservationInput struct {
	OrganizationID  string
	SLAID           string
	SourceType      string
	SourceReference string
	MeasuredValue   *string
	MeasurementUnit string
	MeasuredAt      time.Time
	WindowStart     time.Time
	WindowEnd       time.Time
	SourceVersion   int64
	EventID         string
	PayloadHash     string
	CorrelationID   string
	CausationID     string
}

// IngestObservationResult reports what happened to a submitted measurement.
type IngestObservationResult struct {
	Observation models.SLAObservation
	Duplicate   bool
}

// approvedObservationSources are the services permitted to report a
// measurement.
//
// The list is closed. An SLA outcome is a claim about service quality, and a
// caller that is not one of the systems that actually performs the work has no
// standing to make it — however well-formed its HMAC signature is.
var approvedObservationSources = map[string]bool{
	"driver-dispatch-delivery-service": true,
	"driver-hauling-service":           true,
	"media-file-service":               false,
	"support-dispute-service":          true,
}

// IngestObservation records one append-only SLA measurement.
//
// The caller identity comes from the verified HMAC context, not from the body.
// The organization ID in the payload is checked against the SLA's actual owner
// rather than trusted: a signed request is proof of who is calling, never proof
// of whose data they may write.
func (s *Service) IngestObservation(ctx context.Context, verifiedCaller string,
	input IngestObservationInput) (IngestObservationResult, error) {
	if err := s.writeEnabled(s.opts.Flags.SLAEnabled); err != nil {
		return IngestObservationResult{}, err
	}
	if !approvedObservationSources[verifiedCaller] {
		return IngestObservationResult{}, errorcodes.Forbidden(errorcodes.SLASourceUnknown,
			"This service is not an approved source of service level observations.")
	}
	if strings.TrimSpace(input.PayloadHash) == "" {
		return IngestObservationResult{}, badRequest(errorcodes.SLAObservationConflict,
			"payloadHash is required.")
	}
	if input.WindowEnd.Before(input.WindowStart) {
		return IngestObservationResult{}, badRequest(errorcodes.SLAObservationConflict,
			"windowEnd must not be before windowStart.")
	}

	sla, err := s.opts.Repository.GetSLA(ctx, nil, input.OrganizationID, input.SLAID)
	if err != nil {
		// A cross-tenant SLA ID and an unknown one are answered identically.
		return IngestObservationResult{}, errorcodes.NotFound(errorcodes.SLANotFound,
			"Service level agreement was not found.")
	}

	// An out-of-order or gapped delivery is not interpolated. Where a version is
	// missing the outcome is UNKNOWN, because an unmeasured window is not a met
	// one, and treating a monitoring gap as success would let an outage read as
	// perfect service.
	latest, err := s.opts.Repository.LatestObservationVersion(ctx, nil, input.SLAID,
		input.SourceType, input.SourceReference)
	if err != nil {
		return IngestObservationResult{}, err
	}
	outcome := models.EvaluateOutcome(sla.MetricCode, input.MeasurementUnit, sla.TargetValue,
		input.MeasuredValue)
	if latest > 0 && input.SourceVersion > latest+1 {
		outcome = models.OutcomeUnknown
	}
	if input.SourceVersion > 0 && input.SourceVersion < latest {
		// Older than what is stored. Recording it is harmless and honest; it
		// simply does not become the newest word on the window.
		outcome = models.OutcomeNotEvaluated
	}

	var result IngestObservationResult
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		observation, duplicate, txErr := s.opts.Repository.AppendObservation(ctx, tx,
			models.SLAObservation{
				OrganizationID:  input.OrganizationID,
				SLAID:           input.SLAID,
				SourceType:      input.SourceType,
				SourceReference: input.SourceReference,
				MeasuredValue:   input.MeasuredValue,
				MeasurementUnit: input.MeasurementUnit,
				MeasuredAt:      input.MeasuredAt,
				WindowStart:     input.WindowStart,
				WindowEnd:       input.WindowEnd,
				Outcome:         outcome,
				SourceVersion:   input.SourceVersion,
				EventID:         input.EventID,
				PayloadHash:     input.PayloadHash,
				CorrelationID:   input.CorrelationID,
				CausationID:     input.CausationID,
			})
		if txErr != nil {
			if errors.Is(txErr, repositories.ErrObservationConflict) {
				return conflict(errorcodes.SLAObservationConflict,
					"A different measurement is already recorded for this source version.")
			}
			return txErr
		}
		result = IngestObservationResult{Observation: observation, Duplicate: duplicate}
		if duplicate {
			// A replay changes nothing and announces nothing.
			return nil
		}
		return s.publish(ctx, tx, EventSLAOutcomeRecorded, "service_level_agreement",
			input.SLAID, sla.Version, input.OrganizationID, input.CorrelationID,
			slaOutcomeEventPayload(sla, observation))
	})
	if err != nil {
		return IngestObservationResult{}, err
	}
	s.count("sla_observations_total", map[string]string{"outcome": result.Observation.Outcome})
	if result.Observation.Outcome == models.OutcomeUnknown {
		s.count("sla_unknown_outcomes_total", nil)
	}
	return result, nil
}

// slaAuditValues builds the audit projection.
func slaAuditValues(sla models.SLA) map[string]any {
	return map[string]any{
		"metricCode":        sla.MetricCode,
		"targetValue":       sla.TargetValue,
		"measurementUnit":   sla.MeasurementUnit,
		"measurementWindow": sla.MeasurementWindow,
		"status":            sla.Status,
		"customerId":        sla.CustomerID,
		"contractId":        sla.ContractID,
	}
}
