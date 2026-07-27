package usecases

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Closure blocker codes. Each names the authoritative service that owns the
// obligation, because "no local row" never proves "no external obligation".
const (
	BlockerActiveMarketplaceRequests = "active_marketplace_requests"
	BlockerActiveAssignments         = "active_provider_assignments"
	BlockerActiveTrips               = "active_trips"
	BlockerUnsettledWallet           = "unsettled_wallet_operations"
	BlockerPendingAllocations        = "pending_provider_allocations"
	BlockerPendingWithdrawals        = "pending_withdrawals"
	BlockerActiveProviderMemberships = "active_provider_memberships"
	BlockerOpenPayroll               = "open_payroll_periods"
	BlockerOpenInvoices              = "open_invoices"
	BlockerOpenClaims                = "open_claims"
	BlockerOpenIncidents             = "open_incidents"
	BlockerLegalHold                 = "legal_hold"
	BlockerRetentionRequirement      = "retention_requirement"
	BlockerPendingExports            = "pending_exports"
	BlockerComplianceObligations     = "unresolved_compliance_obligations"
	BlockerIntegrationCredentials    = "active_integration_credentials"
)

// ClosureBlockerChecker reports whether one obligation blocks closure. A
// checker that cannot reach its authoritative service must return
// BlockerStateUnknown, never BlockerStateCleared.
type ClosureBlockerChecker interface {
	Check(ctx context.Context, organizationID string) models.ClosureBlocker
}

// ProviderMembershipCounter counts live managed providers locally.
type ProviderMembershipCounter interface {
	CountLiveMemberships(ctx context.Context, organizationID string) (int, error)
}

// RequestClosureInput opens a closure workflow.
type RequestClosureInput struct {
	Tenant     tenant.Context
	ReasonCode string
	ReasonText string
}

// RequestClosure opens a controlled closure request.
//
// There is no account deletion: the organization keeps its lifecycle state
// until closure completes, and immutable records are always retained.
func (s *Service) RequestClosure(ctx context.Context, input RequestClosureInput) (*models.ClosureRequest, error) {
	organization, err := s.opts.Organizations.GetByID(ctx, input.Tenant.OrganizationID)
	if err != nil {
		return nil, err
	}
	if organization.Status == models.OrganizationStatusClosed {
		return nil, apperrors.Conflict("This organization is already closed.", nil)
	}

	if existing, err := s.opts.Onboarding.GetOpenClosureRequest(ctx, input.Tenant.OrganizationID); err == nil {
		// Duplicate request: return the open one rather than creating another.
		return existing, nil
	} else if !errors.Is(err, orgrepos.ErrNotFound) {
		return nil, err
	}

	request := &models.ClosureRequest{
		ReasonCode:    input.ReasonCode,
		RequestedBy:   input.Tenant.ActorID,
		RequestID:     input.Tenant.RequestID,
		CorrelationID: input.Tenant.CorrelationID,
	}
	if input.ReasonText != "" {
		request.ReasonText = &input.ReasonText
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Onboarding.CreateClosureRequest(ctx, tx, input.Tenant.OrganizationID, request); err != nil {
			return err
		}
		entry := audit.EntryFromTenant(input.Tenant, "organization.closure_requested", "closure_request", request.ID)
		entry.Reason = input.ReasonCode
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.closure_requested.v1", "closure_request",
			request.ID, 1, input.Tenant.OrganizationID, input.Tenant.CorrelationID, map[string]any{
				"closureRequestId": request.ID,
				"organizationId":   input.Tenant.OrganizationID,
				"reasonCode":       input.ReasonCode,
			})
	})
	if err != nil {
		return nil, err
	}
	return request, nil
}

// AssessClosure evaluates every blocker and records the outcome.
//
// An unreachable authoritative service yields an UNKNOWN blocker, which keeps
// the request BLOCKED — uncertainty never reads as permission to close.
func (s *Service) AssessClosure(ctx context.Context, organizationID string, checkers []ClosureBlockerChecker, providers ProviderMembershipCounter) (*models.ClosureRequest, []models.ClosureBlocker, error) {
	request, err := s.opts.Onboarding.GetOpenClosureRequest(ctx, organizationID)
	if err != nil {
		return nil, nil, err
	}

	blockers := make([]models.ClosureBlocker, 0, len(checkers)+1)
	for _, checker := range checkers {
		blockers = append(blockers, checker.Check(ctx, organizationID))
	}

	if providers != nil {
		count, err := providers.CountLiveMemberships(ctx, organizationID)
		blocker := models.ClosureBlocker{
			BlockerCode:   BlockerActiveProviderMemberships,
			OwningService: "fleet-partner-service",
			State:         models.BlockerStateCleared,
		}
		switch {
		case err != nil:
			blocker.State = models.BlockerStateUnknown
			blocker.Detail = "could not evaluate provider memberships"
		case count > 0:
			blocker.State = models.BlockerStateBlocking
			blocker.Detail = "managed providers must be offboarded first"
		}
		blockers = append(blockers, blocker)
	}

	blocking := 0
	for _, blocker := range blockers {
		if blocker.State != models.BlockerStateCleared {
			blocking++
		}
	}

	nextStatus := models.ClosureStatusApprovalRequired
	if blocking > 0 {
		nextStatus = models.ClosureStatusBlocked
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Onboarding.ReplaceClosureBlockers(ctx, tx, organizationID, request.ID, blockers); err != nil {
			return err
		}
		return s.opts.Onboarding.UpdateClosureStatus(ctx, tx, organizationID, request.ID, nextStatus, request.Version)
	})
	if err != nil {
		return nil, nil, err
	}
	s.opts.Metrics.SetGauge("fleet_partner_closure_blockers", nil, float64(blocking))

	request.Status = nextStatus
	return request, blockers, nil
}

// SessionRevoker revokes organization sessions during closure execution.
type SessionRevoker interface {
	RevokeForOrganization(ctx context.Context, q database.Querier, organizationID string, reason string) (int64, error)
}

// ExecuteClosure performs the non-destructive closure steps.
//
// It revokes corporate sessions, stops new operations and marks the
// organization CLOSED. Immutable financial, trip, safety, compliance and
// audit records are retained; nothing is hard-deleted.
func (s *Service) ExecuteClosure(ctx context.Context, organizationID string, sessions SessionRevoker, correlationID string) error {
	request, err := s.opts.Onboarding.GetOpenClosureRequest(ctx, organizationID)
	if err != nil {
		return err
	}
	if request.Status != models.ClosureStatusApproved {
		return errorcodes.Conflict(errorcodes.OrganizationClosureBlocked,
			"This closure request is not approved for execution.")
	}

	organization, err := s.opts.Organizations.GetByID(ctx, organizationID)
	if err != nil {
		return err
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Onboarding.UpdateClosureStatus(ctx, tx, organizationID, request.ID,
			models.ClosureStatusExecuting, request.Version); err != nil {
			return err
		}
		if sessions != nil {
			if _, err := sessions.RevokeForOrganization(ctx, tx, organizationID, "organization_closed"); err != nil {
				return err
			}
		}
		if err := s.opts.Organizations.UpdateStatus(ctx, tx, organizationID, organization.Version, models.StatusChange{
			PreviousStatus: organization.Status,
			NewStatus:      models.OrganizationStatusClosed,
			ActorType:      string(tenant.ActorTypeSystem),
			ActorID:        "closure-executor",
			ReasonCode:     request.ReasonCode,
			RequestID:      request.RequestID,
			CorrelationID:  correlationID,
		}); err != nil {
			return err
		}
		if err := s.opts.Onboarding.UpdateClosureStatus(ctx, tx, organizationID, request.ID,
			models.ClosureStatusCompleted, request.Version+1); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.closed.v1", "organization",
			organizationID, organization.Version+1, organizationID, correlationID, map[string]any{
				"organizationId":   organizationID,
				"closureRequestId": request.ID,
			})
	})
	if err != nil {
		return err
	}
	return nil
}

// GetClosureStatus returns the open closure request and its blockers.
func (s *Service) GetClosureStatus(ctx context.Context, organizationID string) (*models.ClosureRequest, []models.ClosureBlocker, error) {
	request, err := s.opts.Onboarding.GetOpenClosureRequest(ctx, organizationID)
	if err != nil {
		return nil, nil, err
	}
	blockers, err := s.opts.Onboarding.ListClosureBlockers(ctx, organizationID, request.ID)
	if err != nil {
		return nil, nil, err
	}
	return request, blockers, nil
}

// UnknownBlockerChecker is the default checker for a capability whose
// authoritative service contract does not exist yet. It always reports
// UNKNOWN, which blocks closure until a real checker is wired.
type UnknownBlockerChecker struct {
	Code          string
	OwningService string
}

// Check always returns an UNKNOWN blocker.
func (c UnknownBlockerChecker) Check(context.Context, string) models.ClosureBlocker {
	return models.ClosureBlocker{
		BlockerCode:   c.Code,
		OwningService: c.OwningService,
		State:         models.BlockerStateUnknown,
		Detail:        "authoritative service contract is not configured",
	}
}

// DefaultClosureCheckers returns the fail-closed checker set used until the
// authoritative closure contracts exist.
func DefaultClosureCheckers() []ClosureBlockerChecker {
	return []ClosureBlockerChecker{
		UnknownBlockerChecker{Code: BlockerActiveMarketplaceRequests, OwningService: "driver-dispatch-delivery-service"},
		UnknownBlockerChecker{Code: BlockerActiveTrips, OwningService: "driver-dispatch-delivery-service"},
		UnknownBlockerChecker{Code: BlockerUnsettledWallet, OwningService: "payment-wallet-service"},
		UnknownBlockerChecker{Code: BlockerPendingWithdrawals, OwningService: "payment-wallet-service"},
		UnknownBlockerChecker{Code: BlockerOpenClaims, OwningService: "support-dispute-service"},
		UnknownBlockerChecker{Code: BlockerComplianceObligations, OwningService: "verification-compliance-service"},
	}
}
