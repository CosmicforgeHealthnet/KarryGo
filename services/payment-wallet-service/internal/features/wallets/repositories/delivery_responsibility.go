package walletrepositories

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type TransitionDeliveryResponsibilityInput struct {
	VendorOrderID, OperationRef, IdempotencyKey, ReasonCode string
	PlatformExecutionCostMinor                              int64
}

func (r *PostgresWalletRepository) TransitionDeliveryResponsibility(ctx context.Context, input TransitionDeliveryResponsibilityInput) (walletmodels.DeliveryResponsibilityTransition, error) {
	input.VendorOrderID = strings.TrimSpace(input.VendorOrderID)
	input.OperationRef = strings.TrimSpace(input.OperationRef)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.ReasonCode = strings.TrimSpace(input.ReasonCode)
	if input.VendorOrderID == "" || input.OperationRef == "" || input.IdempotencyKey == "" || input.ReasonCode == "" || input.PlatformExecutionCostMinor < 0 {
		return walletmodels.DeliveryResponsibilityTransition{}, apperrors.Validation("Delivery responsibility transition is incomplete.", nil)
	}
	fingerprint, err := stableFingerprint(input)
	if err != nil {
		return walletmodels.DeliveryResponsibilityTransition{}, err
	}
	var result walletmodels.DeliveryResponsibilityTransition
	err = r.withTx(ctx, func(tx pgx.Tx) error {
		existing, loadErr := scanDeliveryResponsibilityTransition(tx.QueryRow(ctx, `SELECT `+deliveryResponsibilityTransitionColumns+` FROM commerce_delivery_responsibility_transitions WHERE operation_ref=$1`, input.OperationRef))
		if loadErr == nil {
			if existing.RequestFingerprint != fingerprint || existing.IdempotencyKey != input.IdempotencyKey || existing.VendorOrderID != input.VendorOrderID {
				return apperrors.Conflict("Delivery responsibility operation was reused with different semantics.", nil)
			}
			result = existing
			return nil
		}
		if !errors.Is(loadErr, pgx.ErrNoRows) {
			return loadErr
		}

		var allocationID, responsibility, currency, parentReference, intentStatus string
		var vendorBefore, providerBefore, released, customer, vendorSubsidy, platformSubsidy int64
		loadErr = tx.QueryRow(ctx, `SELECT a.id,a.effective_delivery_responsibility,a.effective_vendor_delivery_payable_minor,
			effective_provider_delivery_payable_minor,delivery_released_minor,delivery_customer_minor,
			vendor_delivery_subsidy_minor,platform_delivery_subsidy_minor,a.currency,c.parent_commerce_reference,c.status
			FROM commerce_payment_allocations a JOIN commerce_payment_intents c ON c.payment_intent_id=a.payment_intent_id WHERE a.vendor_order_id=$1 FOR UPDATE OF a`, input.VendorOrderID).
			Scan(&allocationID, &responsibility, &vendorBefore, &providerBefore, &released, &customer, &vendorSubsidy, &platformSubsidy, &currency, &parentReference, &intentStatus)
		if errors.Is(loadErr, pgx.ErrNoRows) {
			return apperrors.NotFound("Commerce allocation was not found.", nil)
		}
		if loadErr != nil {
			return loadErr
		}
		// A concurrent same-operation transaction may have committed while this
		// transaction waited for the allocation lock. Read it back now rather
		// than misclassifying an exactly-once retry as an ineligible transition.
		existing, loadErr = scanDeliveryResponsibilityTransition(tx.QueryRow(ctx, `SELECT `+deliveryResponsibilityTransitionColumns+` FROM commerce_delivery_responsibility_transitions WHERE operation_ref=$1`, input.OperationRef))
		if loadErr == nil {
			if existing.RequestFingerprint != fingerprint || existing.IdempotencyKey != input.IdempotencyKey || existing.VendorOrderID != input.VendorOrderID {
				return apperrors.Conflict("Delivery responsibility operation was reused with different semantics.", nil)
			}
			result = existing
			return nil
		}
		if !errors.Is(loadErr, pgx.ErrNoRows) {
			return loadErr
		}
		if intentStatus != walletmodels.CommerceIntentHeld && intentStatus != walletmodels.CommerceIntentPartiallyReleased {
			return apperrors.Conflict("Delivery responsibility can change only for a funded commerce allocation.", nil)
		}
		if responsibility != walletmodels.DeliveryResponsibilityVendor || providerBefore != 0 || released != 0 {
			return apperrors.Conflict("Delivery responsibility is no longer eligible for pre-custody transition.", nil)
		}
		originalDeliveryPayable := vendorBefore + providerBefore
		if input.PlatformExecutionCostMinor < originalDeliveryPayable {
			return apperrors.Conflict("A cheaper platform execution requires the existing customer-benefit/refund authority.", nil)
		}
		fallbackFunding := input.PlatformExecutionCostMinor - originalDeliveryPayable
		if fallbackFunding > 0 {
			expense, ensureErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypePlatform, "karry-go", walletmodels.AccountPlatformPromotionExpense, currency, walletmodels.NormalDebit)
			if ensureErr != nil {
				return ensureErr
			}
			escrow, ensureErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob, commerceEscrowOwner(parentReference), walletmodels.AccountCommerceEscrow, currency, walletmodels.NormalCredit)
			if ensureErr != nil {
				return ensureErr
			}
			ledgerRef := "delivery-fallback-funding-" + input.OperationRef
			if ensureErr = r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{ID: uuid.NewString(), Reference: ledgerRef, TransactionType: "delivery_fallback_funding", Status: "POSTED", SourceService: "vendor-service", SourceReference: input.OperationRef, IdempotencyKey: input.IdempotencyKey + ":funding"}, []walletmodels.LedgerEntry{{AccountID: expense.ID, Side: walletmodels.SideDebit, AmountKobo: fallbackFunding, Currency: currency, Memo: "Platform-funded Hybrid delivery fallback"}, {AccountID: escrow.ID, Side: walletmodels.SideCredit, AmountKobo: fallbackFunding, Currency: currency, Memo: "Hybrid fallback delivery escrow funding"}}); ensureErr != nil {
				return ensureErr
			}
		}

		result = walletmodels.DeliveryResponsibilityTransition{
			ID: "cdrt_" + uuid.NewString(), AllocationID: allocationID, VendorOrderID: input.VendorOrderID,
			OperationRef: input.OperationRef, FromResponsibility: responsibility, ToResponsibility: walletmodels.DeliveryResponsibilityPlatform,
			VendorDeliveryPayableBeforeMinor: vendorBefore, ProviderDeliveryPayableBeforeMinor: providerBefore,
			VendorDeliveryPayableAfterMinor: 0, ProviderDeliveryPayableAfterMinor: input.PlatformExecutionCostMinor,
			PlatformExecutionCostMinor: input.PlatformExecutionCostMinor, FallbackPlatformFundingMinor: fallbackFunding,
			CustomerContributionMinor: customer, VendorSubsidyMinor: vendorSubsidy, PlatformSubsidyMinor: platformSubsidy,
			Status: "APPLIED", IdempotencyKey: input.IdempotencyKey, RequestFingerprint: fingerprint,
		}
		if _, loadErr = tx.Exec(ctx, `UPDATE commerce_payment_allocations SET
			effective_delivery_responsibility='PLATFORM',effective_vendor_delivery_payable_minor=0,
			effective_provider_delivery_payable_minor=$2,fallback_platform_funding_minor=$3,delivery_transition_version=delivery_transition_version+1,
			delivery_transition_ref=$4,version=version+1,updated_at=now() WHERE id=$1`, allocationID, input.PlatformExecutionCostMinor, fallbackFunding, input.OperationRef); loadErr != nil {
			return loadErr
		}
		result, loadErr = scanDeliveryResponsibilityTransition(tx.QueryRow(ctx, `INSERT INTO commerce_delivery_responsibility_transitions
			(id,allocation_id,vendor_order_id,operation_ref,idempotency_key,request_fingerprint,from_responsibility,to_responsibility,
			 vendor_delivery_payable_before_minor,provider_delivery_payable_before_minor,vendor_delivery_payable_after_minor,
			 provider_delivery_payable_after_minor,platform_execution_cost_minor,fallback_platform_funding_minor,customer_contribution_minor,vendor_subsidy_minor,platform_subsidy_minor,status)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,0,$11,$11,$12,$13,$14,$15,'APPLIED') RETURNING `+deliveryResponsibilityTransitionColumns,
			result.ID, allocationID, input.VendorOrderID, input.OperationRef, input.IdempotencyKey, fingerprint,
			responsibility, walletmodels.DeliveryResponsibilityPlatform, vendorBefore, providerBefore, input.PlatformExecutionCostMinor, fallbackFunding, customer, vendorSubsidy, platformSubsidy))
		if loadErr != nil {
			return loadErr
		}
		return recordFinancialEvidence(ctx, tx, "commerce.delivery_responsibility.transitioned.v1", "commerce.delivery_responsibility.transitioned", "commerce_payment_allocation", allocationID, "vendor-service", 1, map[string]any{
			"vendor_order_id": input.VendorOrderID, "operation_ref": input.OperationRef, "from": responsibility, "to": "PLATFORM",
			"customer_contribution_minor": customer, "vendor_subsidy_minor": vendorSubsidy, "platform_subsidy_minor": platformSubsidy, "platform_execution_cost_minor": input.PlatformExecutionCostMinor, "fallback_platform_funding_minor": fallbackFunding,
		})
	})
	return result, err
}

const deliveryResponsibilityTransitionColumns = `id,allocation_id,vendor_order_id,operation_ref,idempotency_key,request_fingerprint,
	from_responsibility,to_responsibility,vendor_delivery_payable_before_minor,provider_delivery_payable_before_minor,
	vendor_delivery_payable_after_minor,provider_delivery_payable_after_minor,platform_execution_cost_minor,fallback_platform_funding_minor,customer_contribution_minor,vendor_subsidy_minor,
	platform_subsidy_minor,status,created_at`

func scanDeliveryResponsibilityTransition(row commerceScanner) (walletmodels.DeliveryResponsibilityTransition, error) {
	var v walletmodels.DeliveryResponsibilityTransition
	err := row.Scan(&v.ID, &v.AllocationID, &v.VendorOrderID, &v.OperationRef, &v.IdempotencyKey, &v.RequestFingerprint,
		&v.FromResponsibility, &v.ToResponsibility, &v.VendorDeliveryPayableBeforeMinor, &v.ProviderDeliveryPayableBeforeMinor,
		&v.VendorDeliveryPayableAfterMinor, &v.ProviderDeliveryPayableAfterMinor, &v.PlatformExecutionCostMinor, &v.FallbackPlatformFundingMinor, &v.CustomerContributionMinor, &v.VendorSubsidyMinor,
		&v.PlatformSubsidyMinor, &v.Status, &v.CreatedAt)
	return v, err
}

func (r *PostgresWalletRepository) GetDeliveryResponsibilityTransition(ctx context.Context, operationRef string) (walletmodels.DeliveryResponsibilityTransition, error) {
	v, err := scanDeliveryResponsibilityTransition(r.db.QueryRow(ctx, `SELECT `+deliveryResponsibilityTransitionColumns+` FROM commerce_delivery_responsibility_transitions WHERE operation_ref=$1`, strings.TrimSpace(operationRef)))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, apperrors.NotFound("Delivery responsibility transition was not found.", nil)
	}
	return v, err
}
