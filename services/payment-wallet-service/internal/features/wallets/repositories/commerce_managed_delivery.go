package walletrepositories

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 12 Part B: corporate delivery inside Commerce.
//
// A platform Commerce delivery releases the customer's frozen delivery payable.
// Who that money belongs to depends on who executed it, and that is the whole
// distinction this file exists to hold:
//
//   - An independent rider earned it. It is credited to them, exactly as
//     before this file existed.
//   - A rider employed by an organization did not. What they are entitled to is
//     decided by their compensation plan, which is Fleet's to resolve and is
//     frequently zero -- a salaried rider is paid by their employer, not per
//     delivery. The rest belongs to the organization.
//
// The customer's amount does not change either way. That number was frozen when
// they approved the quote, and a delivery costing more because a corporate
// rider took it would be a price change made after the fact.

// SetCommerceDeliveryExecutorInput records who will execute a delivery.
//
// Called when the executor is assigned -- the moment managed-ness becomes
// known, and before any completion can race the release.
type SetCommerceDeliveryExecutorInput struct {
	VendorOrderID  string
	ExecutorMode   string
	OrganizationID string
	MembershipID   string
}

// ReleaseManagedCommerceDeliveryInput is Fleet's instruction for one corporate
// Commerce delivery.
//
// The shares are Fleet's answers, not inputs Wallet computes from. There is no
// rate here and no plan: Wallet checks the arithmetic against the money it
// actually holds and refuses anything that does not reconcile, which is the
// only thing it is in a position to judge.
type ReleaseManagedCommerceDeliveryInput struct {
	VendorOrderID string

	// CompletionReference is the mission this settles. One release per
	// mission, which is what lets one vendor order be delivered by more than
	// one mission without the second overwriting the first.
	CompletionReference string

	ProviderType   string
	ProviderID     string
	OrganizationID string
	MembershipID   string

	// ProviderShareMinor may legitimately be zero. That is the salaried case
	// and it is entirely normal.
	ProviderShareMinor     int64
	OrganizationShareMinor int64

	InstructionID      string
	InstructionVersion int64

	IdempotencyKey string
}

// SetCommerceDeliveryExecutor records the executor mode on an allocation.
//
// Setting it is what closes the direct-payout path for corporate work. An
// allocation left UNKNOWN is refused a direct release rather than defaulting to
// independent, because a payment made to the wrong party cannot be taken back
// and a refused release can simply be retried once the mode is known.
func (r *PostgresWalletRepository) SetCommerceDeliveryExecutor(
	ctx context.Context, input SetCommerceDeliveryExecutorInput,
) error {
	mode := strings.ToUpper(strings.TrimSpace(input.ExecutorMode))
	switch mode {
	case walletmodels.ExecutorIndependent:
		if strings.TrimSpace(input.OrganizationID) != "" || strings.TrimSpace(input.MembershipID) != "" {
			return apperrors.Validation("An independent delivery has no organization.", nil)
		}
	case walletmodels.ExecutorManaged:
		if strings.TrimSpace(input.OrganizationID) == "" || strings.TrimSpace(input.MembershipID) == "" {
			return apperrors.Validation("A managed delivery must name its organization and membership.", nil)
		}
	default:
		return apperrors.Validation("Delivery executor mode is invalid.", nil)
	}
	if strings.TrimSpace(input.VendorOrderID) == "" {
		return apperrors.Validation("Vendor order is required.", nil)
	}

	return r.withTx(ctx, func(tx pgx.Tx) error {
		allocation, _, err := r.lockCommerceAllocation(ctx, tx, input.VendorOrderID)
		if err != nil {
			return err
		}
		// Changing the executor after the money has moved would mean the
		// release was decided against a different fact than the one now
		// recorded. Corrections go through a reversal, not an edit.
		if allocation.DeliveryReleasedMinor > 0 {
			return apperrors.Conflict("Delivery has already been released for this allocation.", nil)
		}
		if allocation.DeliveryExecutorMode != walletmodels.ExecutorUnknown &&
			allocation.DeliveryExecutorMode != mode {
			return apperrors.Conflict("Delivery executor mode has already been decided.", nil)
		}
		_, err = tx.Exec(ctx, `
			UPDATE commerce_payment_allocations
			SET delivery_executor_mode = $2, managed_organization_id = $3, managed_membership_id = $4,
			    version = version + 1, updated_at = now()
			WHERE id = $1`,
			allocation.ID, mode, strings.TrimSpace(input.OrganizationID), strings.TrimSpace(input.MembershipID))
		return err
	})
}

// ReleaseManagedCommerceDelivery settles one corporate Commerce delivery.
//
// One balanced journal: the customer's frozen delivery payable leaves escrow,
// the provider is credited what Fleet resolved, and the organization is
// credited the rest. A zero provider share writes no provider line at all --
// the ledger rejects a zero entry, and inventing a token credit would make a
// salaried rider look like they earned per delivery.
func (r *PostgresWalletRepository) ReleaseManagedCommerceDelivery(
	ctx context.Context, input ReleaseManagedCommerceDeliveryInput,
) (walletmodels.CommerceFinancialOperation, error) {
	if err := validateManagedDeliveryInput(input); err != nil {
		return walletmodels.CommerceFinancialOperation{}, err
	}

	var result walletmodels.CommerceFinancialOperation
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		allocation, parentReference, err := r.lockCommerceAllocation(ctx, tx, input.VendorOrderID)
		if err != nil {
			return err
		}
		if allocation.DeliveryResponsibility != walletmodels.DeliveryResponsibilityPlatform {
			return apperrors.Conflict("Only a platform delivery is settled through Fleet.", nil)
		}
		if allocation.DeliveryExecutorMode != walletmodels.ExecutorManaged {
			return apperrors.Conflict("This allocation's delivery is not organization-managed.", nil)
		}
		if allocation.ManagedOrganizationID != strings.TrimSpace(input.OrganizationID) {
			return apperrors.Conflict("Instruction organization does not own this delivery.", nil)
		}

		// WAL-028. A replayed instruction returns what the first one did
		// rather than paying again. Keyed on the mission, so a retry of one
		// mission's release is a repeat while a second mission's release is a
		// new fact.
		existing, findErr := r.findDeliveryExecution(ctx, tx, allocation.ID, input.CompletionReference)
		if findErr == nil {
			if existing.ProviderShareMinor != input.ProviderShareMinor ||
				existing.OrganizationShareMinor != input.OrganizationShareMinor {
				return apperrors.Conflict("This mission was already settled with a different split.", nil)
			}
			result, err = r.findCommerceOperationTx(ctx, tx, "DELIVERY_RELEASE", existing.IdempotencyKey)
			return err
		} else if !errors.Is(findErr, pgx.ErrNoRows) {
			return findErr
		}

		// The frozen amount. Not recomputed, not adjusted: the customer
		// approved this figure and assigning a corporate rider does not change
		// what they pay.
		frozen := allocation.ProviderDeliveryPayableMinor - allocation.RefundProviderDeliveryReductionMinor
		outstanding := frozen - allocation.DeliveryReleasedMinor
		if outstanding < 0 {
			return apperrors.Internal("Delivery release state is inconsistent.", nil)
		}
		if input.ProviderShareMinor+input.OrganizationShareMinor != outstanding {
			return apperrors.Conflict(fmt.Sprintf(
				"Allocation splits %d minor units but %d is outstanding on this delivery.",
				input.ProviderShareMinor+input.OrganizationShareMinor, outstanding), nil)
		}

		escrow, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob,
			commerceEscrowOwner(parentReference), walletmodels.AccountCommerceEscrow,
			allocation.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `SELECT id FROM wallet_accounts WHERE id=$1 FOR UPDATE`, escrow.ID); err != nil {
			return err
		}

		ledgerReference := ""
		if outstanding > 0 {
			balance, balanceErr := r.accountBalanceTx(ctx, tx, escrow.ID)
			if balanceErr != nil {
				return balanceErr
			}
			if balance < outstanding {
				return apperrors.Conflict("Commerce escrow cannot cover this release.", nil)
			}
			entries := []walletmodels.LedgerEntry{{
				AccountID: escrow.ID, Side: walletmodels.SideDebit,
				AmountKobo: outstanding, Currency: allocation.Currency,
				Memo: "Release managed commerce delivery",
			}}
			if input.ProviderShareMinor > 0 {
				provider, accErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider,
					providerOwnerID(input.ProviderType, input.ProviderID),
					walletmodels.AccountProviderPayable, allocation.Currency, walletmodels.NormalCredit)
				if accErr != nil {
					return accErr
				}
				entries = append(entries, walletmodels.LedgerEntry{
					AccountID: provider.ID, Side: walletmodels.SideCredit,
					AmountKobo: input.ProviderShareMinor, Currency: allocation.Currency,
					Memo: "Provider delivery entitlement",
				})
			}
			if input.OrganizationShareMinor > 0 {
				organization, accErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeOrganization,
					input.OrganizationID, walletmodels.AccountOrganizationPayable,
					allocation.Currency, walletmodels.NormalCredit)
				if accErr != nil {
					return accErr
				}
				entries = append(entries, walletmodels.LedgerEntry{
					AccountID: organization.ID, Side: walletmodels.SideCredit,
					AmountKobo: input.OrganizationShareMinor, Currency: allocation.Currency,
					Memo: "Organization delivery entitlement",
				})
			}
			// The vendor order is part of the reference, not just the
			// mission. A mission reference is only unique within its own
			// order, so two orders that each name their first mission the
			// same way would collide on the ledger's unique reference and the
			// second release would fail outright.
			ledgerReference = "managed-delivery-release-" + input.VendorOrderID + "-" + input.CompletionReference
			if err = r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
				ID: uuid.NewString(), Reference: ledgerReference,
				TransactionType: "managed_delivery_release", Status: postedStatus,
				SourceService: "fleet-partner-service", SourceReference: input.VendorOrderID,
				IdempotencyKey: input.IdempotencyKey,
				Metadata: map[string]any{
					"completion_reference": input.CompletionReference,
					"instruction_id":       input.InstructionID,
					"instruction_version":  input.InstructionVersion,
					"organization_id":      input.OrganizationID,
				},
			}, entries); err != nil {
				return err
			}
		}

		if err = r.insertDeliveryExecution(ctx, tx, allocation, input, outstanding, ledgerReference); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `
			UPDATE commerce_payment_allocations
			SET delivery_released_minor = delivery_released_minor + $2,
			    provider_delivery_released_minor = provider_delivery_released_minor + $3,
			    organization_delivery_released_minor = organization_delivery_released_minor + $4,
			    assigned_provider_type = $5, assigned_provider_id = $6,
			    status = CASE WHEN vendor_released_minor = vendor_merchandise_payable_minor
			                       - refund_vendor_merchandise_reduction_minor
			                       + platform_revenue_minor - refund_platform_revenue_reduction_minor
			                  THEN 'RELEASED' ELSE 'PARTIALLY_RELEASED' END,
			    version = version + 1, updated_at = now()
			WHERE id = $1`,
			allocation.ID, outstanding, input.ProviderShareMinor, input.OrganizationShareMinor,
			input.ProviderType, input.ProviderID); err != nil {
			return err
		}
		if err = r.refreshCommerceIntentState(ctx, tx, allocation.PaymentIntentID); err != nil {
			return err
		}
		if err = r.insertCommerceOperation(ctx, tx, allocation.PaymentIntentID, allocation.ID,
			"DELIVERY_RELEASE", input.CompletionReference, "", input.IdempotencyKey,
			managedDeliveryFingerprint(input), "APPLIED", outstanding, ledgerReference); err != nil {
			return err
		}
		if err = recordFinancialEvidence(ctx, tx,
			"commerce.managed_delivery.released.v1", "commerce.managed_delivery.released",
			"commerce_allocation", allocation.ID, "fleet-partner-service", allocation.Version+1,
			map[string]any{
				"vendor_order_id":          input.VendorOrderID,
				"completion_reference":     input.CompletionReference,
				"organization_id":          input.OrganizationID,
				"provider_share_minor":     input.ProviderShareMinor,
				"organization_share_minor": input.OrganizationShareMinor,
				"frozen_delivery_minor":    outstanding,
				"currency":                 allocation.Currency,
			}); err != nil {
			return err
		}
		result, err = r.findCommerceOperationTx(ctx, tx, "DELIVERY_RELEASE", input.IdempotencyKey)
		return err
	})
	if err != nil {
		return walletmodels.CommerceFinancialOperation{}, err
	}
	return result, nil
}

func validateManagedDeliveryInput(input ReleaseManagedCommerceDeliveryInput) error {
	if strings.TrimSpace(input.VendorOrderID) == "" || strings.TrimSpace(input.CompletionReference) == "" ||
		strings.TrimSpace(input.IdempotencyKey) == "" {
		return apperrors.Validation("Vendor order, completion reference and idempotency key are required.", nil)
	}
	if strings.TrimSpace(input.ProviderType) == "" || strings.TrimSpace(input.ProviderID) == "" {
		return apperrors.Validation("Managed delivery settlement requires the executing provider.", nil)
	}
	if strings.TrimSpace(input.OrganizationID) == "" || strings.TrimSpace(input.MembershipID) == "" {
		return apperrors.Validation("Managed delivery settlement requires the organization and membership.", nil)
	}
	// A negative share is not a discount, it is a mistake: money is moved by
	// crediting somebody, and "credit minus one" has no meaning in a ledger.
	if input.ProviderShareMinor < 0 || input.OrganizationShareMinor < 0 {
		return apperrors.Validation("A share cannot be negative.", nil)
	}
	if strings.TrimSpace(input.InstructionID) == "" || input.InstructionVersion <= 0 {
		return apperrors.Validation("Managed delivery settlement must cite Fleet's instruction.", nil)
	}
	return nil
}

func managedDeliveryFingerprint(input ReleaseManagedCommerceDeliveryInput) string {
	fingerprint, _ := stableFingerprint([]any{
		input.VendorOrderID, input.CompletionReference, input.ProviderType, input.ProviderID,
		input.OrganizationID, input.ProviderShareMinor, input.OrganizationShareMinor,
		input.InstructionID, input.InstructionVersion,
	})
	return fingerprint
}

// DeliveryExecution is one mission's settled delivery.
type DeliveryExecution struct {
	ID                     string
	AllocationID           string
	VendorOrderID          string
	CompletionReference    string
	ExecutorMode           string
	ProviderType           string
	ProviderID             string
	OrganizationID         string
	MembershipID           string
	Currency               string
	FrozenDeliveryMinor    int64
	ProviderShareMinor     int64
	OrganizationShareMinor int64
	InstructionID          string
	InstructionVersion     int64
	IdempotencyKey         string
	LedgerReference        string
}

const deliveryExecutionColumns = `id,allocation_id,vendor_order_id,completion_reference,executor_mode,
	provider_type,provider_id,organization_id,membership_id,currency,
	frozen_delivery_minor,provider_share_minor,organization_share_minor,
	instruction_id,instruction_version,idempotency_key,ledger_reference`

func scanDeliveryExecution(row commerceScanner) (DeliveryExecution, error) {
	var value DeliveryExecution
	err := row.Scan(&value.ID, &value.AllocationID, &value.VendorOrderID, &value.CompletionReference,
		&value.ExecutorMode, &value.ProviderType, &value.ProviderID, &value.OrganizationID,
		&value.MembershipID, &value.Currency, &value.FrozenDeliveryMinor,
		&value.ProviderShareMinor, &value.OrganizationShareMinor,
		&value.InstructionID, &value.InstructionVersion, &value.IdempotencyKey, &value.LedgerReference)
	return value, err
}

func (r *PostgresWalletRepository) findDeliveryExecution(
	ctx context.Context, tx pgx.Tx, allocationID, completionReference string,
) (DeliveryExecution, error) {
	return scanDeliveryExecution(tx.QueryRow(ctx,
		`SELECT `+deliveryExecutionColumns+` FROM commerce_delivery_executions
		 WHERE allocation_id=$1 AND completion_reference=$2`, allocationID, completionReference))
}

func (r *PostgresWalletRepository) insertDeliveryExecution(
	ctx context.Context, tx pgx.Tx, allocation walletmodels.CommerceAllocation,
	input ReleaseManagedCommerceDeliveryInput, frozen int64, ledgerReference string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO commerce_delivery_executions (
			id, allocation_id, vendor_order_id, completion_reference, executor_mode,
			provider_type, provider_id, organization_id, membership_id, currency,
			frozen_delivery_minor, provider_share_minor, organization_share_minor,
			instruction_id, instruction_version, idempotency_key, ledger_reference
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		uuid.NewString(), allocation.ID, input.VendorOrderID, input.CompletionReference,
		walletmodels.ExecutorManaged, input.ProviderType, input.ProviderID,
		input.OrganizationID, input.MembershipID, allocation.Currency,
		frozen, input.ProviderShareMinor, input.OrganizationShareMinor,
		input.InstructionID, input.InstructionVersion, input.IdempotencyKey, ledgerReference)
	return err
}

// ListDeliveryExecutions returns every mission that settled this allocation's
// delivery.
//
// WAL-026: more than one is a legitimate answer. The allocation's own
// assigned_provider columns record whoever was released last and cannot express
// a delivery executed by two missions; these rows can.
func (r *PostgresWalletRepository) ListDeliveryExecutions(
	ctx context.Context, vendorOrderID string,
) ([]DeliveryExecution, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+deliveryExecutionColumns+` FROM commerce_delivery_executions
		 WHERE vendor_order_id=$1 ORDER BY created_at`, vendorOrderID)
	if err != nil {
		return nil, fmt.Errorf("wallet: list delivery executions: %w", err)
	}
	defer rows.Close()
	var executions []DeliveryExecution
	for rows.Next() {
		execution, scanErr := scanDeliveryExecution(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		executions = append(executions, execution)
	}
	return executions, rows.Err()
}
