package fleetpartner

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
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string, aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

type ProviderMembershipReader interface {
	FindLiveByProvider(ctx context.Context, providerService, providerID string) (ProviderMembership, error)
}

type ProviderMembership struct {
	ID               string
	OrganizationID   string
	ProviderService  string
	ServiceCategory  string
	RelationshipType string
	Status           string
	Version          int64
}

type Service struct {
	pool      *pgxpool.Pool
	repo      *Repository
	providers ProviderMembershipReader
	audit     *audit.Recorder
	events    EventPublisher
	flags     Flags
	now       func() time.Time
}

type Flags struct {
	OperatingModeEnforcement bool
	CompensationResolver     bool
	CompletedWorkProjection  bool
	SettlementInstruction    bool
	TypedFinancialVisibility bool
}

func NewService(pool *pgxpool.Pool, repo *Repository, providers ProviderMembershipReader, auditRecorder *audit.Recorder, events EventPublisher, flags Flags) *Service {
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
	if !OperatingModeValid(mode) {
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

func (s *Service) CreateCompensationPlan(ctx context.Context, t tenant.Context, plan CompensationPlan, rules []CompensationRule) (CompensationPlan, error) {
	if plan.Currency == "" {
		return CompensationPlan{}, apperrors.BadRequest("currency is required.", nil)
	}
	if err := ValidateMoney(0, plan.Currency); err != nil {
		return CompensationPlan{}, apperrors.BadRequest("currency is invalid.", err)
	}
	for _, rule := range rules {
		if err := ValidateRule(rule); err != nil {
			return CompensationPlan{}, apperrors.BadRequest(err.Error(), err)
		}
	}
	plan.OrganizationID = t.OrganizationID
	if plan.CreatedBy == "" {
		plan.CreatedBy = t.OrganizationUserID
	}
	var created CompensationPlan
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

func (s *Service) AssignCompensationPlan(ctx context.Context, t tenant.Context, in MembershipCompensationAssignment) (MembershipCompensationAssignment, error) {
	if in.Currency == "" || ValidateMoney(0, in.Currency) != nil {
		return MembershipCompensationAssignment{}, apperrors.BadRequest("currency is invalid.", nil)
	}
	if in.ServiceContext == "" {
		in.ServiceContext = "*"
	}
	if in.Status == "" {
		in.Status = PlanActive
	}
	if in.EffectiveFrom.IsZero() {
		in.EffectiveFrom = s.now()
	}
	in.OrganizationID = t.OrganizationID
	if in.AssignedBy == "" {
		in.AssignedBy = t.OrganizationUserID
	}
	var out MembershipCompensationAssignment
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
}

func (s *Service) RecordCompletedWork(ctx context.Context, caller string, in CompletedWorkInput) (CompletedWorkRecord, CompensationResolution, ManagedWorkSettlementInstruction, error) {
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
		return CompletedWorkRecord{}, CompensationResolution{}, ManagedWorkSettlementInstruction{}, apperrors.Unavailable("Completed-work projection is not enabled.", nil)
	}
	if !callerOwnsSource(caller, in.SourceService) {
		return CompletedWorkRecord{}, CompensationResolution{}, ManagedWorkSettlementInstruction{}, apperrors.Forbidden("Caller does not own the source service.", nil)
	}
	if in.IdempotencyKey == "" {
		return CompletedWorkRecord{}, CompensationResolution{}, ManagedWorkSettlementInstruction{}, apperrors.BadRequest("idempotencyKey is required.", nil)
	}
	if !ValidateSourceService(in.SourceService) || in.SourceService == ServiceFleet || in.SourceService == ServiceVendor {
		return CompletedWorkRecord{}, CompensationResolution{}, ManagedWorkSettlementInstruction{}, apperrors.BadRequest("sourceService is invalid for this ingestion contract.", nil)
	}
	if err := ValidateMoney(in.EligibleBasisMinor, in.Currency); err != nil {
		return CompletedWorkRecord{}, CompensationResolution{}, ManagedWorkSettlementInstruction{}, apperrors.BadRequest("eligible monetary basis is invalid.", err)
	}
	if in.CompletedAt.Location() != time.UTC {
		return CompletedWorkRecord{}, CompensationResolution{}, ManagedWorkSettlementInstruction{}, apperrors.BadRequest("completedAt must be UTC.", nil)
	}
	role, _, err := s.repo.VerifyMembership(ctx, in.OrganizationID, in.MembershipID, in.ProviderID, in.SourceService)
	if err != nil {
		return CompletedWorkRecord{}, CompensationResolution{}, ManagedWorkSettlementInstruction{}, asAppError(err)
	}
	record := CompletedWorkRecord{
		OrganizationID: in.OrganizationID, MembershipID: in.MembershipID, ProviderID: in.ProviderID,
		SourceService: in.SourceService, SourceJobID: in.SourceJobID, SourceAssignmentID: in.SourceAssignmentID,
		WorkType: "MANAGED_WORK", CompletedAt: in.CompletedAt, Currency: in.Currency,
		EligibleBasisMinor: in.EligibleBasisMinor, SourceVersion: in.SourceVersion,
		IdempotencyKey: in.IdempotencyKey, CorrelationID: in.CorrelationID,
	}
	record.RequestFingerprint = fingerprint(in)
	var resolved CompensationResolution
	var instruction ManagedWorkSettlementInstruction
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
		if !s.flags.CompensationResolver {
			if s.events != nil {
				return s.events.Publish(ctx, tx, "fleet.completed_work.recorded.v1", "completed_work", record.ID, record.Version, record.OrganizationID, record.CorrelationID, map[string]any{"completedWorkId": record.ID, "sourceService": record.SourceService, "sourceJobId": record.SourceJobID})
			}
			return nil
		}
		resolved, err = s.resolveRecordedWork(ctx, tx, record, role)
		if err != nil {
			return err
		}
		if !s.flags.SettlementInstruction {
			if s.events != nil {
				if err := s.events.Publish(ctx, tx, "fleet.completed_work.recorded.v1", "completed_work", record.ID, record.Version, record.OrganizationID, record.CorrelationID, map[string]any{"completedWorkId": record.ID, "sourceService": record.SourceService, "sourceJobId": record.SourceJobID}); err != nil {
					return err
				}
				return s.events.Publish(ctx, tx, "fleet.compensation.resolved.v1", "compensation_resolution", resolved.ID, resolved.ResolutionVersion, record.OrganizationID, record.CorrelationID, map[string]any{"resolutionId": resolved.ID, "completedWorkId": record.ID})
			}
			return nil
		}
		instruction, _, err = s.repo.InsertSettlementInstruction(ctx, tx, ManagedWorkSettlementInstruction{
			OrganizationID: record.OrganizationID, MembershipID: record.MembershipID, ProviderID: record.ProviderID,
			SourceService: record.SourceService, SourceJobID: record.SourceJobID, SourceAssignmentID: record.SourceAssignmentID,
			CompensationPlanID: resolved.CompensationPlanID, CompensationPlanVersion: resolved.CompensationPlanVersion,
			CompensationResolutionVersion: resolved.ResolutionVersion, GrossBasisMinor: record.EligibleBasisMinor,
			ProviderShareMinor: resolved.ProviderShareMinor, OrganizationShareMinor: resolved.OrganizationShareMinor,
			Currency: record.Currency, VisibilityPolicySnapshot: resolved.VisibilityPolicySnapshot,
			IdempotencyKey: "settlement:" + record.IdempotencyKey, RequestFingerprint: record.RequestFingerprint,
		})
		if err != nil {
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

func (s *Service) CorporateContext(ctx context.Context, providerID, service string) (CorporateContext, error) {
	providerService := providerServiceName(service)
	membership, err := s.providers.FindLiveByProvider(ctx, providerService, providerID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return CorporateContext{
				ProviderID: providerID, IsManagedProvider: false,
				ServiceEligibility:        map[string]bool{service: false},
				FinancialVisibilityPolicy: ConservativeVisibilityPolicy(),
				Version:                   1,
			}, nil
		}
		return CorporateContext{}, err
	}
	mode, policyMap, settingsVersion, err := s.repo.OperatingSettings(ctx, membership.OrganizationID)
	if err != nil {
		return CorporateContext{}, asAppError(err)
	}
	eligible := membership.Status == "ACTIVE" && membership.ServiceCategory == service
	return CorporateContext{
		ProviderID: providerID, IsManagedProvider: true, MembershipID: membership.ID,
		OrganizationID: membership.OrganizationID, MembershipStatus: membership.Status,
		OrganizationRole: membership.RelationshipType, OperatingMode: mode,
		ServiceEligibility:        map[string]bool{service: eligible},
		CompensationContext:       map[string]any{"managedByFleet": true},
		FinancialVisibilityPolicy: s.visibilityPolicy(policyMap),
		Version:                   settingsVersion + membership.Version,
	}, nil
}

func (s *Service) visibilityPolicy(policyMap map[string]bool) ProviderFinancialVisibilityPolicy {
	if !s.flags.TypedFinancialVisibility {
		return ConservativeVisibilityPolicy()
	}
	return VisibilityPolicyFromMap(policyMap)
}

func (s *Service) resolveRecordedWork(ctx context.Context, tx pgx.Tx, record CompletedWorkRecord, role string) (CompensationResolution, error) {
	assignment, err := s.repo.ResolveAssignment(ctx, record.MembershipID, record.OrganizationID, record.SourceService, record.Currency, record.CompletedAt)
	if err != nil {
		return CompensationResolution{}, err
	}
	rules, err := s.repo.RulesForPlan(ctx, record.OrganizationID, assignment.CompensationPlanID, assignment.CompensationPlanVersion)
	if err != nil {
		return CompensationResolution{}, err
	}
	mode, policyMap, _, err := s.repo.OperatingSettings(ctx, record.OrganizationID)
	if err != nil {
		return CompensationResolution{}, err
	}
	_ = mode
	var matched []MatchedRule
	var providerShare int64
	for _, rule := range rules {
		if !RuleApplies(rule, record.SourceService, role, record.CompletedAt) {
			continue
		}
		m, err := CalculateRuleShare(rule, record.EligibleBasisMinor, record.Currency)
		if err != nil {
			return CompensationResolution{}, err
		}
		if m.RuleID != "" {
			matched = append(matched, m)
			providerShare += m.AmountMinor
		}
	}
	orgShare := record.EligibleBasisMinor - providerShare
	if orgShare < 0 {
		orgShare = 0
	}
	policy := s.visibilityPolicy(policyMap)
	res := CompensationResolution{
		OrganizationID: record.OrganizationID, CompletedWorkID: record.ID, MembershipID: record.MembershipID,
		CompensationPlanID: assignment.CompensationPlanID, CompensationPlanVersion: assignment.CompensationPlanVersion,
		MatchedRules: matched, ProviderShareMinor: providerShare, OrganizationShareMinor: orgShare,
		Currency: record.Currency, CalculationBasis: map[string]any{"eligibleBasisMinor": record.EligibleBasisMinor, "sourceService": record.SourceService},
		VisibilityPolicySnapshot: policy,
	}
	return s.repo.InsertResolution(ctx, tx, res)
}

func callerOwnsSource(caller, source string) bool {
	switch source {
	case ServiceTaxi:
		return caller == "driver-taxi-service" || caller == ServiceTaxi
	case ServiceDispatch:
		return caller == "driver-dispatch-delivery-service" || caller == ServiceDispatch
	case ServiceHauling:
		return caller == "driver-hauling-service" || caller == ServiceHauling
	default:
		return false
	}
}

func providerServiceName(service string) string {
	switch service {
	case ServiceTaxi:
		return "driver-taxi-service"
	case ServiceHauling:
		return "driver-hauling-service"
	default:
		return "driver-dispatch-delivery-service"
	}
}

func fingerprint(v any) string {
	sum := sha256.Sum256(SnapshotJSON(v))
	return hex.EncodeToString(sum[:])
}

func asAppError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return apperrors.NotFound("Required Fleet partner record was not found.", err)
	case errors.Is(err, ErrIdempotencyConflict):
		return apperrors.Conflict("This idempotency key was already used with different input.", err)
	case errors.Is(err, ErrDuplicateCompletedWork):
		return apperrors.Conflict("Completed work has already been recorded.", err)
	default:
		if strings.Contains(fmt.Sprint(err), "version") {
			return apperrors.Conflict("The resource version is stale.", err)
		}
		return err
	}
}
