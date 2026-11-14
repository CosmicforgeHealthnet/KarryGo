package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fleetpartner/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string, aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

type ProviderMembershipReader interface {
	FindLiveByProvider(ctx context.Context, providerService, providerID string) (models.ProviderMembership, error)
}

// Repository is the persistence this feature's use cases need. The Postgres
// implementation lives in fleetpartner/repositories and is wired at the
// composition root.
type Repository interface {
	OperatingSettings(ctx context.Context, organizationID string) (string, map[string]bool, int64, error)
	UpdateOperatingMode(ctx context.Context, q database.Querier, organizationID, mode string, expectedVersion int64) (int64, error)
	OrganizationStatus(ctx context.Context, organizationID string) (string, error)
	VerifyMembership(ctx context.Context, organizationID, membershipID, providerID, service string) (string, int64, error)
	CreatePlan(ctx context.Context, q database.Querier, plan models.CompensationPlan, rules []models.CompensationRule) (models.CompensationPlan, error)
	AssignPlan(ctx context.Context, q database.Querier, a models.MembershipCompensationAssignment) (models.MembershipCompensationAssignment, error)
	ResolveAssignment(ctx context.Context, membershipID, organizationID, service, currency string, at time.Time) (models.MembershipCompensationAssignment, error)
	RulesForPlan(ctx context.Context, organizationID, planID string, planVersion int64) ([]models.CompensationRule, error)
	InsertCompletedWork(ctx context.Context, q database.Querier, record models.CompletedWorkRecord) (models.CompletedWorkRecord, bool, error)
	CompletedWorkByIdempotency(ctx context.Context, organizationID, key string) (models.CompletedWorkRecord, error)
	InsertResolution(ctx context.Context, q database.Querier, res models.CompensationResolution) (models.CompensationResolution, error)
	InsertSettlementInstruction(ctx context.Context, q database.Querier, in models.ManagedWorkSettlementInstruction) (models.ManagedWorkSettlementInstruction, bool, error)
	RecordWorkSettledComponents(ctx context.Context, q database.Querier, components []models.WorkSettledComponent) error
	SettlementInstruction(ctx context.Context, q database.Querier, organizationID, instructionID string, forUpdate bool) (models.ManagedWorkSettlementInstruction, error)
	RecordSettlementDelivery(ctx context.Context, q database.Querier, current models.ManagedWorkSettlementInstruction, status, operationID, reference, errorCode string) (models.ManagedWorkSettlementInstruction, error)
	LinkWorkSettledComponentsToPosting(ctx context.Context, q database.Querier, organizationID, instructionID, operationID string) error
	UndeliveredSettlementInstructions(ctx context.Context, jobType string, limit int) ([]models.UndeliveredInstruction, error)
	LockSettlementDelivery(ctx context.Context, instructionID string) (release func(), held bool, err error)
}

type Service struct {
	pool      *pgxpool.Pool
	repo      Repository
	providers ProviderMembershipReader
	audit     *audit.Recorder
	events    EventPublisher
	flags     Flags
	now       func() time.Time

	// settlement is the money-authority port (FLEET-13.11).
	settlement SettlementSubmitter
}

type Flags struct {
	OperatingModeEnforcement bool

	// CompensationResolver is the legacy boolean. ResolverMode supersedes it;
	// when the mode is unset the boolean still decides, so an existing
	// deployment keeps behaving exactly as it did (CMP-019).
	CompensationResolver bool

	// ResolverMode is OFF, SHADOW or ON. Shadow resolves and records without
	// anything acting on the figures, so they can be checked before money
	// depends on them.
	ResolverMode             string
	CompletedWorkProjection  bool
	SettlementInstruction    bool
	TypedFinancialVisibility bool
}

func NewService(pool *pgxpool.Pool, repo Repository, providers ProviderMembershipReader, auditRecorder *audit.Recorder, events EventPublisher, flags Flags) *Service {
	return &Service{pool: pool, repo: repo, providers: providers, audit: auditRecorder, events: events, flags: flags, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) ReadOperatingMode(ctx context.Context, organizationID string) (string, int64, error) {
	mode, _, version, err := s.repo.OperatingSettings(ctx, organizationID)
	if err != nil {
		return "", 0, asAppError(err)
	}
	return mode, version, nil
}

func (s *Service) UpdateOperatingMode(ctx context.Context, t tenant.Context, mode string, expectedVersion int64) (int64, error) {
	if !models.OperatingModeValid(mode) {
		return 0, apperrors.Validation("Invalid operating mode.", []apperrors.FieldViolation{{Field: "operatingMode", Message: "must be CENTRALIZED, DELEGATED or HYBRID"}})
	}
	var version int64
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		previous, _, _, err := s.repo.OperatingSettings(ctx, t.OrganizationID)
		if err != nil {
			return err
		}
		version, err = s.repo.UpdateOperatingMode(ctx, tx, t.OrganizationID, mode, expectedVersion)
		if err != nil {
			return err
		}
		if s.audit != nil {
			entry := audit.EntryFromTenant(t, "fleet.organization.operating_mode.changed", "organization_settings", t.OrganizationID)
			entry.PreviousValues = map[string]any{"operatingMode": previous, "version": expectedVersion}
			entry.NewValues = map[string]any{"operatingMode": mode, "version": version}
			if err := s.audit.Record(ctx, tx, entry); err != nil {
				return err
			}
		}
		if s.events != nil {
			return s.events.Publish(ctx, tx, "fleet.organization.operating_mode.changed.v1", "organization_settings",
				t.OrganizationID, version, t.OrganizationID, t.CorrelationID, map[string]any{
					"organizationId":      t.OrganizationID,
					"previousMode":        previous,
					"newMode":             mode,
					"organizationVersion": version,
					"changedAt":           s.now(),
				})
		}
		return nil
	})
	return version, asAppError(err)
}

func (s *Service) CreateCompensationPlan(ctx context.Context, t tenant.Context, plan models.CompensationPlan, rules []models.CompensationRule) (models.CompensationPlan, error) {
	if plan.Currency == "" {
		return models.CompensationPlan{}, apperrors.BadRequest("currency is required.", nil)
	}
	if err := models.ValidateMoney(0, plan.Currency); err != nil {
		return models.CompensationPlan{}, apperrors.BadRequest("currency is invalid.", err)
	}
	for _, rule := range rules {
		if err := models.ValidateRule(rule); err != nil {
			return models.CompensationPlan{}, apperrors.BadRequest(err.Error(), err)
		}
	}
	plan.OrganizationID = t.OrganizationID
	if plan.CreatedBy == "" {
		plan.CreatedBy = t.OrganizationUserID
	}
	var created models.CompensationPlan
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		created, err = s.repo.CreatePlan(ctx, tx, plan, rules)
		if err != nil {
			return err
		}
		if s.audit != nil {
			entry := audit.EntryFromTenant(t, "fleet.compensation_plan.created", "compensation_plan", created.ID)
			entry.NewValues = map[string]any{"name": created.Name, "currency": created.Currency, "status": created.Status, "version": created.Version}
			if err := s.audit.Record(ctx, tx, entry); err != nil {
				return err
			}
		}
		if s.events != nil {
			return s.events.Publish(ctx, tx, "fleet.compensation_plan.created.v1", "compensation_plan", created.ID, created.Version, t.OrganizationID, t.CorrelationID, map[string]any{"planId": created.ID, "currency": created.Currency, "status": created.Status})
		}
		return nil
	})
	return created, asAppError(err)
}

func (s *Service) AssignCompensationPlan(ctx context.Context, t tenant.Context, in models.MembershipCompensationAssignment) (models.MembershipCompensationAssignment, error) {
	if in.Currency == "" || models.ValidateMoney(0, in.Currency) != nil {
		return models.MembershipCompensationAssignment{}, apperrors.BadRequest("currency is invalid.", nil)
	}
	if in.ServiceContext == "" {
		in.ServiceContext = "*"
	}
	if in.Status == "" {
		in.Status = models.PlanActive
	}
	if in.EffectiveFrom.IsZero() {
		in.EffectiveFrom = s.now()
	}
	in.OrganizationID = t.OrganizationID
	if in.AssignedBy == "" {
		in.AssignedBy = t.OrganizationUserID
	}
	var out models.MembershipCompensationAssignment
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		out, err = s.repo.AssignPlan(ctx, tx, in)
		if err != nil {
			return err
		}
		if s.audit != nil {
			entry := audit.EntryFromTenant(t, "fleet.membership.compensation_assigned", "membership_compensation_assignment", out.ID)
			entry.NewValues = map[string]any{"membershipId": out.MembershipID, "planId": out.CompensationPlanID, "planVersion": out.CompensationPlanVersion}
			if err := s.audit.Record(ctx, tx, entry); err != nil {
				return err
			}
		}
		if s.events != nil {
			return s.events.Publish(ctx, tx, "fleet.membership.compensation_assigned.v1", "membership_compensation_assignment", out.ID, 1, t.OrganizationID, t.CorrelationID, map[string]any{"assignmentId": out.ID, "membershipId": out.MembershipID, "planId": out.CompensationPlanID})
		}
		return nil
	})
	return out, asAppError(err)
}

type CompletedWorkInput struct {
	SourceService, SourceJobID, SourceAssignmentID string
	ProviderID, MembershipID, OrganizationID       string
	CompletedAt                                    time.Time
	Currency                                       string
	EligibleBasisMinor, SourceVersion              int64
	IdempotencyKey, CorrelationID                  string

	// PaymentSourceReference is the reference the execution service created
	// the job's payment under, when that is not SourceJobID (Dispatch sends
	// the booking a trip executes). It is copied unchanged into the
	// settlement instruction.
	//
	// The tag matters: this struct is fingerprinted as JSON, and every other
	// field marshals under its Go name. omitempty keeps the fingerprint of a
	// submission without a payment reference exactly what it was before the
	// field existed, so a redelivery of completed work recorded earlier is
	// still recognised as a replay rather than refused as a conflict.
	PaymentSourceReference string `json:"PaymentSourceReference,omitempty"`
}

func (s *Service) RecordCompletedWork(ctx context.Context, caller string, in CompletedWorkInput) (models.CompletedWorkRecord, models.CompensationResolution, models.ManagedWorkSettlementInstruction, error) {
	// Only INGESTION is gated here. Compensation resolution and settlement
	// instruction generation are separate, later stages (Sections 12/13) and
	// are gated individually below.
	//
	// These three were previously one combined gate, which made canonical
	// ingestion impossible in exactly the configuration Sections 8/9 ship in
	// (projection on, resolver and settlement off): every Taxi and Dispatch
	// completed-work submission would have been rejected 503, and each
	// service's outbox would have accumulated undeliverable rows forever.
	if !s.flags.CompletedWorkProjection {
		return models.CompletedWorkRecord{}, models.CompensationResolution{}, models.ManagedWorkSettlementInstruction{}, apperrors.Unavailable("Completed-work projection is not enabled.", nil)
	}
	if !callerOwnsSource(caller, in.SourceService) {
		return models.CompletedWorkRecord{}, models.CompensationResolution{}, models.ManagedWorkSettlementInstruction{}, apperrors.Forbidden("Caller does not own the source service.", nil)
	}
	if in.IdempotencyKey == "" {
		return models.CompletedWorkRecord{}, models.CompensationResolution{}, models.ManagedWorkSettlementInstruction{}, apperrors.BadRequest("idempotencyKey is required.", nil)
	}
	in.PaymentSourceReference = strings.TrimSpace(in.PaymentSourceReference)
	if len(in.PaymentSourceReference) > 200 {
		return models.CompletedWorkRecord{}, models.CompensationResolution{}, models.ManagedWorkSettlementInstruction{}, apperrors.BadRequest("paymentSourceReference is too long.", nil)
	}
	if !models.ValidateSourceService(in.SourceService) || in.SourceService == models.ServiceFleet || in.SourceService == models.ServiceVendor {
		return models.CompletedWorkRecord{}, models.CompensationResolution{}, models.ManagedWorkSettlementInstruction{}, apperrors.BadRequest("sourceService is invalid for this ingestion contract.", nil)
	}
	if err := models.ValidateMoney(in.EligibleBasisMinor, in.Currency); err != nil {
		return models.CompletedWorkRecord{}, models.CompensationResolution{}, models.ManagedWorkSettlementInstruction{}, apperrors.BadRequest("eligible monetary basis is invalid.", err)
	}
	if in.CompletedAt.Location() != time.UTC {
		return models.CompletedWorkRecord{}, models.CompensationResolution{}, models.ManagedWorkSettlementInstruction{}, apperrors.BadRequest("completedAt must be UTC.", nil)
	}
	role, _, err := s.repo.VerifyMembership(ctx, in.OrganizationID, in.MembershipID, in.ProviderID, in.SourceService)
	if err != nil {
		return models.CompletedWorkRecord{}, models.CompensationResolution{}, models.ManagedWorkSettlementInstruction{}, asAppError(err)
	}
	record := models.CompletedWorkRecord{
		OrganizationID: in.OrganizationID, MembershipID: in.MembershipID, ProviderID: in.ProviderID,
		SourceService: in.SourceService, SourceJobID: in.SourceJobID, SourceAssignmentID: in.SourceAssignmentID,
		WorkType: "MANAGED_WORK", CompletedAt: in.CompletedAt, Currency: in.Currency,
		EligibleBasisMinor: in.EligibleBasisMinor, SourceVersion: in.SourceVersion,
		IdempotencyKey: in.IdempotencyKey, CorrelationID: in.CorrelationID,
		PaymentSourceReference: in.PaymentSourceReference,
	}
	record.RequestFingerprint = fingerprint(in)
	var resolved models.CompensationResolution
	var instruction models.ManagedWorkSettlementInstruction
	var replay bool
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		record, replay, err = s.repo.InsertCompletedWork(ctx, tx, record)
		if err != nil || replay {
			return err
		}
		// Ingestion is complete and durable at this point. Resolution and
		// settlement-instruction generation are downstream stages that stay
		// dark until their own flags are enabled, so a Sections 8/9
		// deployment records canonical completed work without producing any
		// compensation or settlement side effect.
		if !models.ResolvesCompensation(s.resolverMode()) {
			if s.events != nil {
				return s.events.Publish(ctx, tx, "fleet.completed_work.recorded.v1", "completed_work", record.ID, record.Version, record.OrganizationID, record.CorrelationID, map[string]any{"completedWorkId": record.ID, "sourceService": record.SourceService, "sourceJobId": record.SourceJobID})
			}
			return nil
		}
		resolved, err = s.resolveRecordedWork(ctx, tx, record, role)
		if err != nil {
			return err
		}
		// Shadow stops here on purpose: the resolution exists and is durable,
		// and nothing downstream may act on it. That is what makes the figures
		// checkable before any money depends on them.
		if !models.ActsOnResolution(s.resolverMode()) || !s.flags.SettlementInstruction {
			if s.events != nil {
				if err := s.events.Publish(ctx, tx, "fleet.completed_work.recorded.v1", "completed_work", record.ID, record.Version, record.OrganizationID, record.CorrelationID, map[string]any{"completedWorkId": record.ID, "sourceService": record.SourceService, "sourceJobId": record.SourceJobID}); err != nil {
					return err
				}
				return s.events.Publish(ctx, tx, "fleet.compensation.resolved.v1", "compensation_resolution", resolved.ID, resolved.ResolutionVersion, record.OrganizationID, record.CorrelationID, map[string]any{"resolutionId": resolved.ID, "completedWorkId": record.ID})
			}
			return nil
		}
		instruction, _, err = s.repo.InsertSettlementInstruction(ctx, tx, models.ManagedWorkSettlementInstruction{
			OrganizationID: record.OrganizationID, MembershipID: record.MembershipID, ProviderID: record.ProviderID,
			SourceService: record.SourceService, SourceJobID: record.SourceJobID, SourceAssignmentID: record.SourceAssignmentID,
			PaymentSourceReference: record.PaymentSourceReference,
			CompensationPlanID:     resolved.CompensationPlanID, CompensationPlanVersion: resolved.CompensationPlanVersion,
			CompensationResolutionVersion: resolved.ResolutionVersion, GrossBasisMinor: record.EligibleBasisMinor,
			ProviderShareMinor: resolved.ProviderShareMinor, OrganizationShareMinor: resolved.OrganizationShareMinor,
			Currency: record.Currency, VisibilityPolicySnapshot: resolved.VisibilityPolicySnapshot,
			IdempotencyKey: "settlement:" + record.IdempotencyKey, RequestFingerprint: record.RequestFingerprint,
		})
		if err != nil {
			return err
		}
		// FLEET-13.11. The delivery job commits with the instruction: an
		// instruction that exists always has a way to reach Wallet.
		if err := s.enqueueSettlementDelivery(ctx, tx, instruction.OrganizationID, instruction.InstructionID, record.CorrelationID); err != nil {
			return err
		}
		// CMP-027. The components this job settled with its own money are
		// recorded here, in the same transaction that created the instruction
		// authorising the payment. Payroll reads these and skips them.
		//
		// Recording them only once an instruction exists is deliberate: with
		// the settlement-instruction stage dark, no money moves at work level,
		// so payroll must still pay these components and an exclusion row
		// would wrongly suppress them.
		if err := s.repo.RecordWorkSettledComponents(ctx, tx,
			models.WorkSettledComponentsFor(record, resolved, instruction.InstructionID, "")); err != nil {
			return err
		}
		if s.events != nil {
			if err := s.events.Publish(ctx, tx, "fleet.completed_work.recorded.v1", "completed_work", record.ID, record.Version, record.OrganizationID, record.CorrelationID, map[string]any{"completedWorkId": record.ID, "sourceService": record.SourceService, "sourceJobId": record.SourceJobID}); err != nil {
				return err
			}
			if err := s.events.Publish(ctx, tx, "fleet.compensation.resolved.v1", "compensation_resolution", resolved.ID, resolved.ResolutionVersion, record.OrganizationID, record.CorrelationID, map[string]any{"resolutionId": resolved.ID, "completedWorkId": record.ID}); err != nil {
				return err
			}
			return s.events.Publish(ctx, tx, "fleet.settlement_instruction.created.v1", "settlement_instruction", instruction.InstructionID, instruction.Version, record.OrganizationID, record.CorrelationID, map[string]any{"instructionId": instruction.InstructionID, "completedWorkId": record.ID})
		}
		return nil
	})
	if replay {
		return record, resolved, instruction, nil
	}
	return record, resolved, instruction, asAppError(err)
}

func (s *Service) CorporateContext(ctx context.Context, providerID, service string) (models.CorporateContext, error) {
	providerService := providerServiceName(service)
	membership, err := s.providers.FindLiveByProvider(ctx, providerService, providerID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return models.CorporateContext{
				ProviderID: providerID, IsManagedProvider: false,
				ServiceEligibility:        map[string]bool{service: false},
				FinancialVisibilityPolicy: models.ConservativeVisibilityPolicy(),
				Version:                   1,
			}, nil
		}
		return models.CorporateContext{}, err
	}
	mode, policyMap, settingsVersion, err := s.repo.OperatingSettings(ctx, membership.OrganizationID)
	if err != nil {
		return models.CorporateContext{}, asAppError(err)
	}
	// Spec 10.3: suspending (or closing) the organization immediately removes
	// every managed provider from organization work, whatever the membership
	// row still says.
	organizationStatus, err := s.repo.OrganizationStatus(ctx, membership.OrganizationID)
	if err != nil {
		return models.CorporateContext{}, asAppError(err)
	}
	eligible := membership.Status == "ACTIVE" && membership.ServiceCategory == service &&
		organizationStatus == "ACTIVE"
	return models.CorporateContext{
		ProviderID: providerID, IsManagedProvider: true, MembershipID: membership.ID,
		OrganizationID: membership.OrganizationID, MembershipStatus: membership.Status,
		OrganizationRole: membership.RelationshipType, OperatingMode: mode,
		ServiceEligibility:        map[string]bool{service: eligible},
		CompensationContext:       map[string]any{"managedByFleet": true},
		FinancialVisibilityPolicy: s.visibilityPolicy(policyMap),
		Version:                   settingsVersion + membership.Version,
	}, nil
}

func (s *Service) visibilityPolicy(policyMap map[string]bool) models.ProviderFinancialVisibilityPolicy {
	if !s.flags.TypedFinancialVisibility {
		return models.ConservativeVisibilityPolicy()
	}
	return models.VisibilityPolicyFromMap(policyMap)
}

func (s *Service) resolveRecordedWork(ctx context.Context, tx pgx.Tx, record models.CompletedWorkRecord, role string) (models.CompensationResolution, error) {
	assignment, err := s.repo.ResolveAssignment(ctx, record.MembershipID, record.OrganizationID, record.SourceService, record.Currency, record.CompletedAt)
	if err != nil {
		return models.CompensationResolution{}, err
	}
	rules, err := s.repo.RulesForPlan(ctx, record.OrganizationID, assignment.CompensationPlanID, assignment.CompensationPlanVersion)
	if err != nil {
		return models.CompensationResolution{}, err
	}
	mode, policyMap, _, err := s.repo.OperatingSettings(ctx, record.OrganizationID)
	if err != nil {
		return models.CompensationResolution{}, err
	}
	_ = mode
	// Section 13. Work-level components (per-job, commission) are settled with
	// this job's money; payroll-level components (salary, per-shift,
	// allowances, overtime, bonuses, deductions) are recorded but contribute
	// nothing here -- they belong to a payroll period. Paying them per job is
	// what would pay a salaried provider twice.
	resolved, err := models.ResolveWorkCompensation(
		rules, record.EligibleBasisMinor, record.Currency, record.SourceService, role, record.CompletedAt)
	if err != nil {
		return models.CompensationResolution{}, err
	}
	policy := s.visibilityPolicy(policyMap)
	res := models.CompensationResolution{
		OrganizationID: record.OrganizationID, CompletedWorkID: record.ID, MembershipID: record.MembershipID,
		CompensationPlanID: assignment.CompensationPlanID, CompensationPlanVersion: assignment.CompensationPlanVersion,
		MatchedRules:       resolved.Matched,
		ProviderShareMinor: resolved.ProviderShareMinor, OrganizationShareMinor: resolved.OrganizationShareMinor,
		Currency: record.Currency,
		CalculationBasis: map[string]any{
			"eligibleBasisMinor": record.EligibleBasisMinor,
			"sourceService":      record.SourceService,
			"role":               role,
			"completedAt":        record.CompletedAt.UTC().Format(time.RFC3339),
		},
		VisibilityPolicySnapshot: policy,
	}
	return s.repo.InsertResolution(ctx, tx, res)
}

func callerOwnsSource(caller, source string) bool {
	switch source {
	case models.ServiceTaxi:
		return caller == "driver-taxi-service" || caller == models.ServiceTaxi
	case models.ServiceDispatch:
		return caller == "driver-dispatch-delivery-service" || caller == models.ServiceDispatch
	case models.ServiceHauling:
		return caller == "driver-hauling-service" || caller == models.ServiceHauling
	default:
		return false
	}
}

func providerServiceName(service string) string {
	switch service {
	case models.ServiceTaxi:
		return "driver-taxi-service"
	case models.ServiceHauling:
		return "driver-hauling-service"
	default:
		return "driver-dispatch-delivery-service"
	}
}

func fingerprint(v any) string {
	sum := sha256.Sum256(models.SnapshotJSON(v))
	return hex.EncodeToString(sum[:])
}

func asAppError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, models.ErrNotFound):
		return apperrors.NotFound("Required Fleet partner record was not found.", err)
	case errors.Is(err, models.ErrIdempotencyConflict):
		return apperrors.Conflict("This idempotency key was already used with different input.", err)
	case errors.Is(err, models.ErrDuplicateCompletedWork):
		return apperrors.Conflict("Completed work has already been recorded.", err)
	default:
		if strings.Contains(fmt.Sprint(err), "version") {
			return apperrors.Conflict("The resource version is stale.", err)
		}
		return err
	}
}

// resolverMode is the effective compensation resolver mode.
//
// An unrecognised configured mode resolves to OFF rather than being guessed at:
// the safe direction for a money-deciding stage is to do nothing.
func (s *Service) resolverMode() string {
	mode, err := models.NormalizeResolverMode(s.flags.ResolverMode, s.flags.CompensationResolver)
	if err != nil {
		return models.ResolverOff
	}
	return mode
}

// ResolverMode reports the effective mode, for operators and for readiness
// reporting during a staged rollout.
func (s *Service) ResolverMode() string { return s.resolverMode() }
