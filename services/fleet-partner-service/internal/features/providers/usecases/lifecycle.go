package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// RecordAssociationOutcome applies the provider's own accept/decline decision.
//
// It is driven by the owning provider service (event or internal callback) —
// never by a corporate user. The company cannot consent on a provider's behalf.
func (s *Service) RecordAssociationOutcome(ctx context.Context, organizationID string, membershipID string, accepted bool, correlationID string) error {
	membership, err := s.opts.Providers.GetMembership(ctx, organizationID, membershipID)
	if err != nil {
		return err
	}

	target := models.StatusPendingSetup
	eventType := "organization.provider.association_accepted.v1"
	if !accepted {
		target = models.StatusDeclined
		eventType = "organization.provider.association_declined.v1"
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if invitation, err := s.opts.Providers.GetInvitationByMembership(ctx, organizationID, membershipID); err == nil {
			outcome := models.InvitationAccepted
			if !accepted {
				outcome = models.InvitationDeclined
			}
			if err := s.opts.Providers.SetInvitationOutcome(ctx, tx, invitation.ID, outcome); err != nil &&
				!errors.Is(err, providerrepos.ErrInvitationNotUsable) {
				return err
			}
		} else if !errors.Is(err, providerrepos.ErrNotFound) {
			return err
		}

		if err := s.opts.Providers.TransitionMembership(ctx, tx, organizationID, membershipID, target,
			membership.Version, models.HistoryEntry{
				ActorType:     "provider",
				ActorID:       derefString(membership.ProviderID),
				ReasonCode:    "provider_association_decision",
				CorrelationID: correlationID,
			}); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, eventType, "provider_membership", membershipID,
			membership.Version+1, organizationID, correlationID, map[string]any{
				"membershipId":   membershipID,
				"organizationId": organizationID,
				"accepted":       accepted,
			})
	})
	if err != nil {
		return err
	}
	s.opts.Metrics.IncCounter("fleet_partner_provider_association_total",
		map[string]string{"outcome": target}, 1)

	if accepted {
		// Consent granted: request the sub-wallet, then evaluate activation.
		s.EnsureSubWallet(ctx, organizationID, membershipID, correlationID)
		_, _ = s.TryActivate(ctx, organizationID, membershipID, correlationID)
	}
	return nil
}

// EnsureSubWallet requests the provider sub-wallet from payment-wallet-service.
//
// Balances are never stored locally, and customer trip settlement is never
// credited here — this only records the account identity projection.
func (s *Service) EnsureSubWallet(ctx context.Context, organizationID string, membershipID string, correlationID string) {
	idempotencyKey := "provider-subwallet:" + membershipID
	currency, _ := s.opts.Organizations.OrganizationCurrency(ctx, organizationID)
	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Providers.EnsureSubWalletLink(ctx, tx, organizationID, membershipID, idempotencyKey)
	})

	account, err := s.opts.Contracts.Wallet.EnsureProviderSubWallet(ctx,
		platformclients.CallMeta{OrganizationID: organizationID, CorrelationID: correlationID, IdempotencyKey: idempotencyKey},
		platformclients.WalletProvisionRequest{
			OrganizationID:       organizationID,
			ProviderMembershipID: membershipID,
			Currency:             currency,
			IdempotencyKey:       idempotencyKey,
		})
	if err != nil {
		status := models.SubWalletFailedRetryable
		if clientErr, ok := err.(*platformclients.Error); ok && clientErr.Outcome == platformclients.OutcomeUnknown {
			status = models.SubWalletReconciliationRequired
		}
		_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Providers.SetSubWalletStatus(ctx, tx, membershipID, status, "", 0)
		})
		s.markSagaStep(ctx, membershipID, "subwallet_status", "UNAVAILABLE",
			models.SagaReconciliationRequired, "subwallet_unavailable")
		s.opts.Metrics.IncCounter("fleet_partner_provider_subwallet_provisioning_total",
			map[string]string{"outcome": status}, 1)
		return
	}

	status := models.SubWalletRequested
	if account.Status == "ACTIVE" {
		status = models.SubWalletActive
	}
	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Providers.SetSubWalletStatus(ctx, tx, membershipID, status, account.AccountID, account.SourceVersion)
	})
	s.markSagaStep(ctx, membershipID, "subwallet_status", status, models.SagaInProgress, "")
	s.opts.Metrics.IncCounter("fleet_partner_provider_subwallet_provisioning_total",
		map[string]string{"outcome": status}, 1)
}

// ActivationGate is one managed-provider activation precondition.
type ActivationGate struct {
	Name   string
	Passed bool
	Detail string
}

// EvaluateActivation checks every provider activation gate.
func (s *Service) EvaluateActivation(ctx context.Context, organizationID string, membershipID string) ([]ActivationGate, bool, error) {
	membership, err := s.opts.Providers.GetMembership(ctx, organizationID, membershipID)
	if err != nil {
		return nil, false, err
	}

	gates := []ActivationGate{
		{Name: "provider_identity_exists", Passed: membership.ProviderID != nil},
		{Name: "association_accepted", Passed: membership.AssociationAcceptedAt != nil},
		{Name: "membership_pending_setup", Passed: membership.Status == models.StatusPendingSetup, Detail: membership.Status},
		{Name: "home_branch_assigned", Passed: membership.HomeBranchID != nil},
	}

	_, organizationStatus, err := s.opts.Organizations.OrganizationStatus(ctx, organizationID)
	if err != nil {
		return nil, false, err
	}
	gates = append(gates, ActivationGate{
		Name: "organization_active", Passed: organizationStatus == "ACTIVE", Detail: organizationStatus,
	})

	// Every active managed provider must have a confirmed sub-wallet.
	subWalletActive := false
	if link, err := s.opts.Providers.GetSubWalletLink(ctx, organizationID, membershipID); err == nil {
		subWalletActive = link.Status == models.SubWalletActive && link.WalletAccountID != nil
	} else if !errors.Is(err, providerrepos.ErrNotFound) {
		return nil, false, err
	}
	gates = append(gates, ActivationGate{Name: "subwallet_active", Passed: subWalletActive})

	// Platform compliance still applies: organization sponsorship never
	// bypasses provider licence, vehicle or document requirements.
	platformOK := false
	if membership.ProviderID != nil {
		if contract, ok := s.opts.Contracts.Providers[membership.ServiceCategory]; ok {
			identity, err := contract.ReadProvider(ctx, platformclients.CallMeta{}, *membership.ProviderID)
			platformOK = err == nil && identity.PlatformStatus == "ACTIVE"
		}
	}
	gates = append(gates, ActivationGate{Name: "provider_platform_status_active", Passed: platformOK})

	ready := true
	for _, gate := range gates {
		if !gate.Passed {
			ready = false
			break
		}
	}
	return gates, ready, nil
}

// TryActivate activates the membership when every gate passes.
func (s *Service) TryActivate(ctx context.Context, organizationID string, membershipID string, correlationID string) (bool, error) {
	_, ready, err := s.EvaluateActivation(ctx, organizationID, membershipID)
	if err != nil || !ready {
		return false, err
	}
	membership, err := s.opts.Providers.GetMembership(ctx, organizationID, membershipID)
	if err != nil {
		return false, err
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Providers.TransitionMembership(ctx, tx, organizationID, membershipID,
			models.StatusActive, membership.Version, models.HistoryEntry{
				ActorType: "system", ActorID: "provider-activation-evaluator",
				ReasonCode: "all_activation_gates_passed", CorrelationID: correlationID,
			}); err != nil {
			return err
		}
		// Default policy: the decline right is a platform invariant.
		if err := s.opts.Providers.UpsertPolicy(ctx, tx, organizationID, &models.Policy{
			MembershipID:            membershipID,
			OrganizationID:          organizationID,
			AssignmentMode:          "OFFER",
			ProviderDeclineRequired: true,
			VisibilityPolicy:        "ORGANIZATION",
		}); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.provider.activated.v1", "provider_membership",
			membershipID, membership.Version+1, organizationID, correlationID, map[string]any{
				"membershipId":   membershipID,
				"organizationId": organizationID,
			})
	})
	if err != nil {
		return false, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_provider_provisioning_total",
		map[string]string{"outcome": "activated"}, 1)
	return true, nil
}

// ChangeStatusInput drives suspend / reactivate / remove.
type ChangeStatusInput struct {
	Tenant          tenant.Context
	MembershipID    string
	TargetStatus    string
	ReasonCode      string
	ExpectedVersion int64
}

// ChangeStatus suspends, reactivates, ends or removes a managed provider.
//
// Removing a membership never deletes the underlying provider identity: the
// person keeps their platform account and can work independently again.
func (s *Service) ChangeStatus(ctx context.Context, input ChangeStatusInput) error {
	membership, err := s.opts.Providers.GetMembership(ctx, input.Tenant.OrganizationID, input.MembershipID)
	if err != nil {
		if errors.Is(err, providerrepos.ErrNotFound) {
			return apperrors.NotFound("Provider not found.", nil)
		}
		return err
	}
	if !models.CanTransition(membership.Status, input.TargetStatus) {
		return errorcodes.Conflict(errorcodes.ProviderMembershipNotActive,
			"This provider cannot move to that state right now.")
	}
	expected := input.ExpectedVersion
	if expected == 0 {
		expected = membership.Version
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Providers.TransitionMembership(ctx, tx, input.Tenant.OrganizationID,
			input.MembershipID, input.TargetStatus, expected, models.HistoryEntry{
				ActorType:     string(input.Tenant.ActorType),
				ActorID:       input.Tenant.ActorID,
				ReasonCode:    input.ReasonCode,
				RequestID:     input.Tenant.RequestID,
				CorrelationID: input.Tenant.CorrelationID,
			}); err != nil {
			return concurrency.AsConflict(err)
		}

		entry := audit.EntryFromTenant(input.Tenant, "provider.status_changed", "provider_membership", input.MembershipID)
		entry.PreviousValues = map[string]any{"status": membership.Status}
		entry.NewValues = map[string]any{"status": input.TargetStatus}
		entry.Reason = input.ReasonCode
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}

		return s.opts.Events.Publish(ctx, tx, providerEventFor(input.TargetStatus), "provider_membership",
			input.MembershipID, expected+1, input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{
				"membershipId":   input.MembershipID,
				"organizationId": input.Tenant.OrganizationID,
				"status":         input.TargetStatus,
			})
	})
	if err != nil {
		return err
	}

	// Suspension and removal revoke organization matching eligibility in the
	// owning provider service; the provider identity itself is untouched.
	if input.TargetStatus == models.StatusSuspended || input.TargetStatus == models.StatusRemoved {
		if membership.ProviderID != nil {
			if contract, ok := s.opts.Contracts.Providers[membership.ServiceCategory]; ok {
				_ = contract.RevokeOrganizationEligibility(ctx,
					platformclients.CallMeta{RequestID: input.Tenant.RequestID, CorrelationID: input.Tenant.CorrelationID},
					*membership.ProviderID, input.Tenant.OrganizationID, input.ReasonCode)
			}
		}
	}
	return nil
}

func providerEventFor(status string) string {
	switch status {
	case models.StatusSuspended:
		return "organization.provider.suspended.v1"
	case models.StatusActive:
		return "organization.provider.reactivated.v1"
	case models.StatusEnding:
		return "organization.provider.ending.v1"
	case models.StatusRemoved:
		return "organization.provider.removed.v1"
	default:
		return "organization.provider.activated.v1"
	}
}

// Repository exposes read paths to handlers.
func (s *Service) Repository() *providerrepos.Repository { return s.opts.Providers }

// CountLiveMemberships backs the organization closure blocker check.
func (s *Service) CountLiveMemberships(ctx context.Context, organizationID string) (int, error) {
	return s.opts.Providers.CountLiveMemberships(ctx, organizationID)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

var _ = time.Now
