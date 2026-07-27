package usecases

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Reconcile re-drives provisioning sagas that are incomplete or whose outcome
// is unknown.
//
// Unknown external write outcomes are resolved by READ-BACK, never by blindly
// repeating the write: a second ProvisionSponsored call could otherwise
// create a duplicate provider identity.
func (s *Service) Reconcile(ctx context.Context, limit int) (int, error) {
	sagas, err := s.opts.Providers.ClaimDueSagas(ctx, limit)
	if err != nil {
		return 0, err
	}

	reconciled := 0
	for _, saga := range sagas {
		if s.reconcileOne(ctx, saga) {
			reconciled++
		}
	}
	if reconciled > 0 {
		s.opts.Metrics.IncCounter("fleet_partner_provider_reconciliation_total",
			map[string]string{"outcome": "progressed"}, float64(reconciled))
	}
	return reconciled, nil
}

func (s *Service) reconcileOne(ctx context.Context, saga models.ProvisioningSaga) bool {
	membership, err := s.opts.Providers.GetMembership(ctx, saga.OrganizationID, saga.MembershipID)
	if err != nil {
		return false
	}
	contract, ok := s.opts.Contracts.Providers[membership.ServiceCategory]
	if !ok {
		return false
	}
	meta := platformclients.CallMeta{}
	progressed := false

	// Step 1: an unknown provisioning outcome is resolved by resolving the
	// contact pair again — if the identity exists, the earlier write landed.
	if membership.ProviderID == nil &&
		(saga.ProviderIdentityStatus == "UNKNOWN_OUTCOME" || saga.ProviderResolutionStatus == "UNAVAILABLE") {
		invitation, err := s.opts.Providers.GetInvitationByMembership(ctx, saga.OrganizationID, saga.MembershipID)
		if err == nil {
			resolution, err := contract.Resolve(ctx, meta, platformclients.ProviderResolveRequest{
				EmailNormalized: invitation.EmailNormalized,
				PhoneE164:       invitation.PhoneE164,
			})
			switch {
			case err != nil:
				s.scheduleRetry(ctx, saga.MembershipID, "resolve_unavailable")
				return false
			case resolution.Outcome == platformclients.ProviderResolveMatched:
				_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
					return s.opts.Providers.SetProviderID(ctx, tx, saga.OrganizationID,
						saga.MembershipID, resolution.ProviderID, resolution.SourceVersion)
				})
				s.markSagaStep(ctx, saga.MembershipID, "provider_identity_status", "PROVISIONED",
					models.SagaInProgress, "")
				progressed = true
			case resolution.Outcome == platformclients.ProviderResolveConflict:
				s.markSagaStep(ctx, saga.MembershipID, "provider_resolution_status", "CONFLICT",
					models.SagaBlocked, "provider_identity_conflict")
				return true
			}
		}
	}

	// Step 2: read back the sub-wallet rather than re-requesting it.
	if saga.SubWalletStatus == "UNAVAILABLE" || saga.SubWalletStatus == "UNKNOWN_OUTCOME" {
		if link, err := s.opts.Providers.GetSubWalletLink(ctx, saga.OrganizationID, saga.MembershipID); err == nil {
			if link.WalletAccountID != nil {
				account, err := s.opts.Contracts.Wallet.ReadAccount(ctx, meta, *link.WalletAccountID)
				if err == nil && account.Status == "ACTIVE" {
					_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
						return s.opts.Providers.SetSubWalletStatus(ctx, tx, saga.MembershipID,
							models.SubWalletActive, account.AccountID, account.SourceVersion)
					})
					s.markSagaStep(ctx, saga.MembershipID, "subwallet_status", "ACTIVE", models.SagaInProgress, "")
					progressed = true
				}
			} else {
				// No account recorded: it is safe to request again because the
				// call is idempotent on the stored key.
				s.EnsureSubWallet(ctx, saga.OrganizationID, saga.MembershipID, "")
				progressed = true
			}
		}
	}

	// Step 3: try to complete activation.
	if activated, _ := s.TryActivate(ctx, saga.OrganizationID, saga.MembershipID, ""); activated {
		s.markSagaStep(ctx, saga.MembershipID, "association_status", "ACCEPTED", models.SagaCompleted, "")
		return true
	}
	if !progressed {
		s.scheduleRetry(ctx, saga.MembershipID, "awaiting_provider_or_dependencies")
	}
	return progressed
}

func (s *Service) scheduleRetry(ctx context.Context, membershipID string, reason string) {
	next := s.opts.Now().Add(5 * time.Minute)
	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Providers.UpdateSagaStep(ctx, tx, membershipID, "notification_status",
			"PENDING", models.SagaReconciliationRequired, reason, &next)
	})
}
