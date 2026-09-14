package walletrepositories

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 12 WAL-029/030: what a corporate Commerce delivery refund may take
// back, and from whom.
//
// The answer comes from the durable record of what was actually paid, not from
// today's compensation policy. A plan that changed since the delivery would
// otherwise decide how much a provider gives back from a job they were paid
// under the old one -- which is not a rounding difference, it is taking money
// from someone under rules that did not exist when they earned it.

// DeliveryRecoveryPool is what is still recoverable from a delivery, by party.
type DeliveryRecoveryPool struct {
	// Managed reports whether this allocation's delivery was executed by an
	// organization-managed provider. An independent delivery keeps the legacy
	// single-recipient behaviour untouched.
	Managed bool

	ProviderRecoverableMinor     int64
	OrganizationRecoverableMinor int64

	// Executions is the durable evidence, oldest first, so a recovery can be
	// attributed to the missions it actually came from.
	Executions []DeliveryExecution
}

// TotalRecoverableMinor is the whole remaining pool.
func (p DeliveryRecoveryPool) TotalRecoverableMinor() int64 {
	return p.ProviderRecoverableMinor + p.OrganizationRecoverableMinor
}

// deliveryRecoveryPool reads what is still recoverable inside a transaction,
// locking the execution rows so two concurrent refunds cannot each see the full
// pool and between them recover it twice.
func (r *PostgresWalletRepository) deliveryRecoveryPool(
	ctx context.Context, tx pgx.Tx, allocation walletmodels.CommerceAllocation,
) (DeliveryRecoveryPool, error) {
	pool := DeliveryRecoveryPool{Managed: allocation.DeliveryExecutorMode == walletmodels.ExecutorManaged}
	if !pool.Managed {
		return pool, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT `+deliveryExecutionColumns+`, refunded_provider_minor, refunded_organization_minor
		FROM commerce_delivery_executions
		WHERE allocation_id = $1
		ORDER BY created_at, id
		FOR UPDATE`, allocation.ID)
	if err != nil {
		return DeliveryRecoveryPool{}, fmt.Errorf("wallet: read delivery recovery pool: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var execution DeliveryExecution
		var refundedProvider, refundedOrganization int64
		if err := rows.Scan(
			&execution.ID, &execution.AllocationID, &execution.VendorOrderID, &execution.CompletionReference,
			&execution.ExecutorMode, &execution.ProviderType, &execution.ProviderID, &execution.OrganizationID,
			&execution.MembershipID, &execution.Currency, &execution.FrozenDeliveryMinor,
			&execution.ProviderShareMinor, &execution.OrganizationShareMinor,
			&execution.InstructionID, &execution.InstructionVersion, &execution.IdempotencyKey,
			&execution.LedgerReference, &refundedProvider, &refundedOrganization,
		); err != nil {
			return DeliveryRecoveryPool{}, err
		}
		execution.RefundedProviderMinor = refundedProvider
		execution.RefundedOrganizationMinor = refundedOrganization
		pool.ProviderRecoverableMinor += execution.ProviderShareMinor - refundedProvider
		pool.OrganizationRecoverableMinor += execution.OrganizationShareMinor - refundedOrganization
		pool.Executions = append(pool.Executions, execution)
	}
	if err := rows.Err(); err != nil {
		return DeliveryRecoveryPool{}, err
	}
	return pool, nil
}

// PlanDeliveryRecovery answers "if I refund this much delivery money, who gives
// it back?" without moving anything.
//
// Callers use it so they do not have to know how a delivery was split -- and
// must not have to, because the split is Fleet's decision recorded at
// settlement time and not something a refunding service can re-derive.
func (r *PostgresWalletRepository) PlanDeliveryRecovery(
	ctx context.Context, vendorOrderID string, recoverMinor int64,
) (walletmodels.DeliveryRecoverySplit, error) {
	var split walletmodels.DeliveryRecoverySplit
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		allocation, _, err := r.lockCommerceAllocation(ctx, tx, vendorOrderID)
		if err != nil {
			return err
		}
		pool, err := r.deliveryRecoveryPool(ctx, tx, allocation)
		if err != nil {
			return err
		}
		if !pool.Managed {
			// An independent delivery has one recipient. Saying so explicitly
			// is better than returning a split that looks like a managed one.
			split = walletmodels.DeliveryRecoverySplit{ProviderMinor: recoverMinor}
			available := allocation.ProviderDeliveryPayableMinor - allocation.RefundProviderRecoveryMinor
			if allocation.DeliveryReleasedMinor == 0 {
				available = 0
			}
			if recoverMinor > available {
				return apperrors.Conflict("Refund recovery source exceeds this allocation's authoritative balance.", nil)
			}
			return nil
		}
		split, err = walletmodels.SplitDeliveryRecovery(
			pool.ProviderRecoverableMinor, pool.OrganizationRecoverableMinor, recoverMinor)
		if err != nil {
			return apperrors.Conflict(err.Error(), nil)
		}
		return nil
	})
	if err != nil {
		return walletmodels.DeliveryRecoverySplit{}, err
	}
	return split, nil
}

// applyDeliveryRecovery records a recovery against the missions it came from.
//
// Attribution walks the executions oldest first, taking what each can still
// give. The order is fixed so a replay attributes identically; any order sums
// to the same total, but an unstable one would make the same refund produce
// different evidence each time it ran.
func (r *PostgresWalletRepository) applyDeliveryRecovery(
	ctx context.Context, tx pgx.Tx, pool DeliveryRecoveryPool,
	split walletmodels.DeliveryRecoverySplit,
) error {
	remainingProvider, remainingOrganization := split.ProviderMinor, split.OrganizationMinor
	for _, execution := range pool.Executions {
		if remainingProvider == 0 && remainingOrganization == 0 {
			break
		}
		providerRoom := execution.ProviderShareMinor - execution.RefundedProviderMinor
		organizationRoom := execution.OrganizationShareMinor - execution.RefundedOrganizationMinor

		takeProvider := remainingProvider
		if takeProvider > providerRoom {
			takeProvider = providerRoom
		}
		takeOrganization := remainingOrganization
		if takeOrganization > organizationRoom {
			takeOrganization = organizationRoom
		}
		if takeProvider == 0 && takeOrganization == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE commerce_delivery_executions
			SET refunded_provider_minor = refunded_provider_minor + $2,
			    refunded_organization_minor = refunded_organization_minor + $3
			WHERE id = $1`, execution.ID, takeProvider, takeOrganization); err != nil {
			return fmt.Errorf("wallet: attribute delivery recovery: %w", err)
		}
		remainingProvider -= takeProvider
		remainingOrganization -= takeOrganization
	}
	if remainingProvider != 0 || remainingOrganization != 0 {
		// The pool was computed from these same rows under the same lock, so
		// this cannot happen -- and if it ever did, the alternative is silently
		// recovering money nobody is recorded as having received.
		return apperrors.Internal("Delivery recovery could not be attributed to the missions that were paid.", nil)
	}
	return nil
}
