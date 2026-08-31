package walletrepositories

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type ApplyDeliveryFundingInput struct {
	SourceService              string
	SourceReference            string
	VendorOrderID              string
	SponsoringVendorID         string
	ProviderType               string
	ProviderID                 string
	Currency                   string
	AuthoritativeDeliveryMinor int64
	CustomerContributionMinor  int64
	VendorSubsidyMinor         int64
	PlatformSubsidyMinor       int64
	PlatformApprovalRef        string
	CustomerPaymentReference   string
	IdempotencyKey             string
}

func fundingFingerprint(input ApplyDeliveryFundingInput) (string, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func validateDeliveryFunding(input ApplyDeliveryFundingInput) error {
	if strings.TrimSpace(input.SourceService) == "" || strings.TrimSpace(input.SourceReference) == "" || strings.TrimSpace(input.VendorOrderID) == "" ||
		strings.TrimSpace(input.SponsoringVendorID) == "" || strings.TrimSpace(input.ProviderType) == "" || strings.TrimSpace(input.ProviderID) == "" ||
		strings.TrimSpace(input.IdempotencyKey) == "" || len(strings.TrimSpace(input.Currency)) != 3 || input.AuthoritativeDeliveryMinor < 0 ||
		input.CustomerContributionMinor < 0 || input.VendorSubsidyMinor < 0 || input.PlatformSubsidyMinor < 0 ||
		input.CustomerContributionMinor+input.VendorSubsidyMinor+input.PlatformSubsidyMinor != input.AuthoritativeDeliveryMinor {
		return apperrors.Validation("Delivery funding is unbalanced.", nil)
	}
	if input.CustomerContributionMinor > 0 && strings.TrimSpace(input.CustomerPaymentReference) == "" {
		return apperrors.Validation("Customer payment evidence is required.", nil)
	}
	if input.PlatformSubsidyMinor > 0 && strings.TrimSpace(input.PlatformApprovalRef) == "" {
		return apperrors.Validation("Platform funding approval evidence is required.", nil)
	}
	return nil
}

func (r *PostgresWalletRepository) ApplyDeliveryFunding(ctx context.Context, input ApplyDeliveryFundingInput) (walletmodels.DeliveryFundingAllocation, error) {
	if err := validateDeliveryFunding(input); err != nil {
		return walletmodels.DeliveryFundingAllocation{}, err
	}
	fingerprint, err := fundingFingerprint(input)
	if err != nil {
		return walletmodels.DeliveryFundingAllocation{}, err
	}
	var result walletmodels.DeliveryFundingAllocation
	err = r.withTx(ctx, func(tx pgx.Tx) error {
		if _, lockErr := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,29))`, input.SourceService+":"+input.SourceReference); lockErr != nil {
			return lockErr
		}
		existing, loadErr := scanDeliveryFunding(tx.QueryRow(ctx, `SELECT `+deliveryFundingColumns+` FROM commerce_delivery_funding_allocations WHERE source_service=$1 AND source_reference=$2`, input.SourceService, input.SourceReference))
		if loadErr == nil {
			if existing.RequestFingerprint != fingerprint {
				return apperrors.Conflict("Delivery funding reference was reused with different amounts.", nil)
			}
			result = existing
			return nil
		}
		if !errors.Is(loadErr, pgx.ErrNoRows) {
			return loadErr
		}
		if input.CustomerContributionMinor > 0 {
			var amount int64
			var status string
			if queryErr := tx.QueryRow(ctx, `SELECT amount_kobo,status FROM payment_intents WHERE reference=$1 FOR SHARE`, input.CustomerPaymentReference).Scan(&amount, &status); queryErr != nil {
				return apperrors.Conflict("Customer payment evidence was not found.", queryErr)
			}
			if amount < input.CustomerContributionMinor || (status != walletmodels.PaymentStatusHeld && status != walletmodels.PaymentStatusCompleted) {
				return apperrors.Conflict("Customer delivery contribution is not authoritatively funded.", nil)
			}
		}
		// Funding attribution is immutable evidence, not a delivery-completion
		// event. Crediting provider/Vendor payable here made allocated subsidy
		// immediately withdrawable before delivery. The commerce allocation hold
		// now funds the complete delivery amount; the completion-authorized
		// delivery-release operation performs the only beneficiary credit.
		ledgerRef := ""
		id := "cdf_" + uuid.NewString()
		row := tx.QueryRow(ctx, `INSERT INTO commerce_delivery_funding_allocations
			(id,source_service,source_reference,vendor_order_id,sponsoring_vendor_id,provider_type,provider_id,currency,authoritative_delivery_minor,
			 customer_contribution_minor,vendor_subsidy_minor,platform_subsidy_minor,platform_approval_ref,customer_payment_reference,ledger_transaction_ref,status,idempotency_key,request_fingerprint)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'APPLIED',$16,$17) RETURNING `+deliveryFundingColumns,
			id, input.SourceService, input.SourceReference, input.VendorOrderID, input.SponsoringVendorID, input.ProviderType, input.ProviderID, strings.ToUpper(input.Currency), input.AuthoritativeDeliveryMinor,
			input.CustomerContributionMinor, input.VendorSubsidyMinor, input.PlatformSubsidyMinor, input.PlatformApprovalRef, input.CustomerPaymentReference, ledgerRef, input.IdempotencyKey, fingerprint)
		var scanErr error
		result, scanErr = scanDeliveryFunding(row)
		return scanErr
	})
	return result, err
}

const deliveryFundingColumns = `id,source_service,source_reference,vendor_order_id,sponsoring_vendor_id,provider_type,provider_id,currency,
	authoritative_delivery_minor,customer_contribution_minor,vendor_subsidy_minor,platform_subsidy_minor,platform_approval_ref,customer_payment_reference,
	ledger_transaction_ref,status,idempotency_key,request_fingerprint,created_at,updated_at`

type fundingScanner interface{ Scan(...any) error }

func scanDeliveryFunding(row fundingScanner) (walletmodels.DeliveryFundingAllocation, error) {
	var v walletmodels.DeliveryFundingAllocation
	err := row.Scan(&v.ID, &v.SourceService, &v.SourceReference, &v.VendorOrderID, &v.SponsoringVendorID, &v.ProviderType, &v.ProviderID, &v.Currency, &v.AuthoritativeDeliveryMinor, &v.CustomerContributionMinor, &v.VendorSubsidyMinor, &v.PlatformSubsidyMinor, &v.PlatformApprovalRef, &v.CustomerPaymentReference, &v.LedgerTransactionRef, &v.Status, &v.IdempotencyKey, &v.RequestFingerprint, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func (r *PostgresWalletRepository) GetDeliveryFunding(ctx context.Context, sourceService, sourceReference string) (walletmodels.DeliveryFundingAllocation, error) {
	return scanDeliveryFunding(r.db.QueryRow(ctx, `SELECT `+deliveryFundingColumns+` FROM commerce_delivery_funding_allocations WHERE source_service=$1 AND source_reference=$2`, sourceService, sourceReference))
}
