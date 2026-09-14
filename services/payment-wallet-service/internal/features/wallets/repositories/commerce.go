package walletrepositories

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/events"
	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/shared/go/apperrors"
)

type CreateCommerceIntentInput struct {
	ParentCommerceReference string
	CustomerID              string
	CustomerEmail           string
	QuoteReference          string
	QuoteVersion            int64
	QuoteFingerprint        string
	FinalAuthorizedMinor    int64
	MaxAuthorizedMinor      int64
	AllocationFingerprint   string
	Currency                string
	PaymentMethod           string
	IdempotencyKey          string
	Allocations             []walletmodels.CommerceAllocation
}

type ReleaseCommerceDeliveryInput struct {
	VendorOrderID               string
	ProviderType                string
	ProviderID                  string
	CompletionReference         string
	ExpectedSnapshotFingerprint string
	IdempotencyKey              string
	TrustedServiceCompletion    bool
}

type ReleaseVendorPayableInput struct {
	VendorOrderID               string
	CompletionReference         string
	ExpectedSnapshotFingerprint string
	IdempotencyKey              string
}

type RefundCommerceAllocationInput struct {
	VendorOrderID string
	ItemReference string

	// Kind distinguishes an ordinary refund from a dispute reversal
	// (WAL-030). Both move money the same way and draw on the same recovery
	// pool -- which is the point: in either order they can never between them
	// take back more than was paid out.
	Kind                         string
	CustomerRefundMinor          int64
	PlatformFundingReversalMinor int64
	OtherFundingReversalMinor    int64
	EscrowRecoveryMinor          int64
	VendorRecoveryMinor          int64
	ProviderRecoveryMinor        int64

	// OrganizationRecoveryMinor is what the organization gives back on a
	// corporate delivery. Without it a refund would recover the provider's and
	// the organization's money alike from the provider -- who, if salaried,
	// never received any of it.
	OrganizationRecoveryMinor       int64
	PlatformRecoveryMinor           int64
	PlatformLossFundingMinor        int64
	VendorMerchandiseReductionMinor int64
	VendorDeliveryReductionMinor    int64
	ProviderDeliveryReductionMinor  int64
	PlatformRevenueReductionMinor   int64
	AuthorizationReleaseMinor       int64
	ExpectedSnapshotFingerprint     string
	IdempotencyKey                  string
}

func recordFinancialEvidence(ctx context.Context, tx pgx.Tx, eventType, action, resourceType, resourceID, actorID string, version int64, values map[string]any) error {
	if err := audit.NewRecorder().Record(ctx, tx, audit.Entry{ActorType: "SERVICE", ActorID: actorID, Action: action, ResourceType: resourceType, ResourceID: resourceID, NewValues: values, CorrelationID: resourceID}); err != nil {
		return err
	}
	envelope, err := events.NewEnvelope(eventType, 1, strings.ToUpper(resourceType), resourceID, version, "", resourceID, "", values, true)
	if err != nil {
		return err
	}
	return events.NewOutboxRepository(tx).Append(ctx, tx, envelope)
}

func stableFingerprint(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func validateCommerceIntent(input CreateCommerceIntentInput) error {
	if strings.TrimSpace(input.ParentCommerceReference) == "" || strings.TrimSpace(input.CustomerID) == "" ||
		strings.TrimSpace(input.QuoteReference) == "" || input.QuoteVersion <= 0 || strings.TrimSpace(input.QuoteFingerprint) == "" ||
		input.FinalAuthorizedMinor <= 0 || input.MaxAuthorizedMinor < input.FinalAuthorizedMinor ||
		strings.TrimSpace(input.AllocationFingerprint) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || len(input.Allocations) == 0 {
		return apperrors.Validation("Commerce payment intent is incomplete.", nil)
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if len(currency) != 3 || (input.PaymentMethod != walletmodels.PaymentMethodWallet && input.PaymentMethod != walletmodels.PaymentMethodPaystack) {
		return apperrors.Validation("Commerce payment currency or method is invalid.", nil)
	}
	seen := map[string]bool{}
	var customerTotal, maxTotal int64
	for _, allocation := range input.Allocations {
		if seen[allocation.AllocationReference] || allocation.Validate(currency) != nil {
			return apperrors.Validation("Commerce allocation set is invalid.", nil)
		}
		seen[allocation.AllocationReference] = true
		var ok bool
		customerTotal, ok = checkedNonnegativeSum(customerTotal, allocation.CustomerPayableMinor)
		if !ok {
			return apperrors.Validation("Commerce allocation total overflowed.", nil)
		}
		maxTotal, ok = checkedNonnegativeSum(maxTotal, allocation.MaxAuthorizedMinor)
		if !ok {
			return apperrors.Validation("Commerce allocation total overflowed.", nil)
		}
	}
	if customerTotal != input.FinalAuthorizedMinor {
		return apperrors.Conflict("Commerce allocations do not match the final authorized amount.", nil)
	}
	if maxTotal != input.MaxAuthorizedMinor {
		return apperrors.Conflict("Commerce allocations do not match the maximum authorized amount.", nil)
	}
	return nil
}

func (r *PostgresWalletRepository) CreateCommercePaymentIntent(ctx context.Context, input CreateCommerceIntentInput) (walletmodels.CommercePaymentIntent, error) {
	if err := validateCommerceIntent(input); err != nil {
		return walletmodels.CommercePaymentIntent{}, err
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	requestFingerprint, err := stableFingerprint(input)
	if err != nil {
		return walletmodels.CommercePaymentIntent{}, err
	}
	var result walletmodels.CommercePaymentIntent
	err = r.withTx(ctx, func(tx pgx.Tx) error {
		if _, lockErr := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,37))`, input.ParentCommerceReference); lockErr != nil {
			return lockErr
		}
		existing, loadErr := r.getCommerceIntentTx(ctx, tx, input.ParentCommerceReference)
		if loadErr == nil {
			if existing.RequestFingerprint != requestFingerprint || existing.IdempotencyKey != input.IdempotencyKey {
				return apperrors.Conflict("Commerce payment reference or idempotency key was reused with a different snapshot.", nil)
			}
			result = existing
			return nil
		}
		if !errors.Is(loadErr, pgx.ErrNoRows) {
			return loadErr
		}
		paymentID := uuid.NewString()
		paymentRef := "pi_" + uuid.NewString()
		status := walletmodels.PaymentStatusRequiresPay
		if input.PaymentMethod == walletmodels.PaymentMethodPaystack {
			status = walletmodels.PaymentStatusPending
		}
		metadata, _ := json.Marshal(map[string]any{
			"kind": "commerce_payment", "quote_reference": input.QuoteReference,
			"quote_version": input.QuoteVersion, "quote_fingerprint": input.QuoteFingerprint,
			"allocation_fingerprint": input.AllocationFingerprint, "max_authorized_minor": input.MaxAuthorizedMinor,
		})
		if _, insertErr := tx.Exec(ctx, `INSERT INTO payment_intents
			(id,reference,source_service,source_reference,customer_id,customer_email,amount_kobo,platform_fee_kobo,currency,payment_method,status,metadata)
			VALUES($1,$2,'vendor-service',$3,$4,$5,$6,0,$7,$8,$9,$10)`, paymentID, paymentRef,
			input.ParentCommerceReference, input.CustomerID, input.CustomerEmail, input.MaxAuthorizedMinor, input.Currency,
			input.PaymentMethod, status, metadata); insertErr != nil {
			return insertErr
		}
		if _, insertErr := tx.Exec(ctx, `INSERT INTO commerce_payment_intents
			(payment_intent_id,parent_commerce_reference,quote_reference,quote_version,quote_fingerprint,final_authorized_minor,max_authorized_minor,
			 allocation_fingerprint,status,idempotency_key,request_fingerprint)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,'CREATED',$9,$10)`, paymentID, input.ParentCommerceReference, input.QuoteReference,
			input.QuoteVersion, input.QuoteFingerprint, input.FinalAuthorizedMinor, input.MaxAuthorizedMinor, input.AllocationFingerprint,
			input.IdempotencyKey, requestFingerprint); insertErr != nil {
			return insertErr
		}
		for _, allocation := range input.Allocations {
			snapshot := allocation.Snapshot
			if len(snapshot) == 0 {
				snapshot = json.RawMessage(`{}`)
			}
			if _, insertErr := tx.Exec(ctx, `INSERT INTO commerce_payment_allocations
				(id,payment_intent_id,allocation_reference,scope,vendor_order_id,vendor_id,branch_id,delivery_responsibility,
				 effective_delivery_responsibility,effective_vendor_delivery_payable_minor,effective_provider_delivery_payable_minor,currency,
				 customer_payable_minor,max_authorized_minor,merchandise_gross_minor,vendor_product_discount_minor,platform_product_discount_minor,platform_commission_minor,
				 delivery_customer_minor,vendor_delivery_subsidy_minor,platform_delivery_subsidy_minor,other_funding_minor,vendor_merchandise_payable_minor,
				 vendor_delivery_payable_minor,provider_delivery_payable_minor,platform_revenue_minor,snapshot,snapshot_fingerprint)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8,$21,$22,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)`,
				"cal_"+uuid.NewString(), paymentID, allocation.AllocationReference, allocation.Scope, allocation.VendorOrderID, allocation.VendorID,
				allocation.BranchID, allocation.DeliveryResponsibility, input.Currency, allocation.CustomerPayableMinor, allocation.MaxAuthorizedMinor, allocation.MerchandiseGrossMinor,
				allocation.VendorProductDiscountMinor, allocation.PlatformProductDiscountMinor, allocation.PlatformCommissionMinor,
				allocation.DeliveryCustomerMinor, allocation.VendorDeliverySubsidyMinor, allocation.PlatformDeliverySubsidyMinor, allocation.OtherFundingMinor,
				allocation.VendorMerchandisePayableMinor, allocation.VendorDeliveryPayableMinor, allocation.ProviderDeliveryPayableMinor,
				allocation.PlatformRevenueMinor, snapshot, allocation.SnapshotFingerprint); insertErr != nil {
				return insertErr
			}
		}
		if evidenceErr := recordFinancialEvidence(ctx, tx, "commerce.intent.created.v1", "commerce.intent.created", "commerce_payment_intent", paymentID, "vendor-service", 1, map[string]any{"parent_commerce_reference": input.ParentCommerceReference, "quote_reference": input.QuoteReference, "quote_version": input.QuoteVersion, "final_authorized_minor": input.FinalAuthorizedMinor, "max_authorized_minor": input.MaxAuthorizedMinor, "currency": input.Currency}); evidenceErr != nil {
			return evidenceErr
		}
		var readErr error
		result, readErr = r.getCommerceIntentTx(ctx, tx, input.ParentCommerceReference)
		return readErr
	})
	return result, err
}

func (r *PostgresWalletRepository) GetCommercePaymentIntent(ctx context.Context, parentReference string) (walletmodels.CommercePaymentIntent, error) {
	intent, err := scanCommerceIntent(r.db.QueryRow(ctx, commerceIntentSelect+` WHERE c.parent_commerce_reference=$1`, parentReference))
	if err != nil {
		return intent, mapNotFound(err, "Commerce payment intent could not be found.")
	}
	intent.Allocations, err = r.listCommerceAllocations(ctx, r.db, intent.PaymentIntentID)
	return intent, err
}

func (r *PostgresWalletRepository) getCommerceIntentTx(ctx context.Context, tx pgx.Tx, parentReference string) (walletmodels.CommercePaymentIntent, error) {
	intent, err := scanCommerceIntent(tx.QueryRow(ctx, commerceIntentSelect+` WHERE c.parent_commerce_reference=$1 FOR UPDATE OF c`, parentReference))
	if err != nil {
		return intent, err
	}
	intent.Allocations, err = r.listCommerceAllocations(ctx, tx, intent.PaymentIntentID)
	return intent, err
}

const commerceIntentSelect = `SELECT c.payment_intent_id::text,p.reference,c.parent_commerce_reference,p.customer_id,c.quote_reference,c.quote_version,
	c.quote_fingerprint,c.final_authorized_minor,c.max_authorized_minor,c.allocation_fingerprint,p.currency,p.payment_method,c.status,c.idempotency_key,
	c.request_fingerprint,c.version,c.created_at,c.updated_at FROM commerce_payment_intents c JOIN payment_intents p ON p.id=c.payment_intent_id`

type commerceScanner interface{ Scan(...any) error }

func scanCommerceIntent(row commerceScanner) (walletmodels.CommercePaymentIntent, error) {
	var value walletmodels.CommercePaymentIntent
	err := row.Scan(&value.PaymentIntentID, &value.PaymentReference, &value.ParentCommerceReference, &value.CustomerID, &value.QuoteReference,
		&value.QuoteVersion, &value.QuoteFingerprint, &value.FinalAuthorizedMinor, &value.MaxAuthorizedMinor, &value.AllocationFingerprint,
		&value.Currency, &value.PaymentMethod, &value.Status, &value.IdempotencyKey, &value.RequestFingerprint, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

const commerceAllocationColumns = `id,payment_intent_id::text,allocation_reference,scope,vendor_order_id,vendor_id,branch_id,
	assigned_provider_type,assigned_provider_id,effective_delivery_responsibility,currency,customer_payable_minor,max_authorized_minor,merchandise_gross_minor,
	vendor_product_discount_minor,platform_product_discount_minor,platform_commission_minor,delivery_customer_minor,vendor_delivery_subsidy_minor,
	platform_delivery_subsidy_minor,other_funding_minor,vendor_merchandise_payable_minor,effective_vendor_delivery_payable_minor,effective_provider_delivery_payable_minor,
	platform_revenue_minor,vendor_released_minor,delivery_released_minor,refunded_minor,
	refund_customer_minor,refund_platform_funding_minor,refund_other_funding_minor,refund_escrow_recovery_minor,
	refund_vendor_recovery_minor,refund_provider_recovery_minor,refund_platform_recovery_minor,refund_platform_loss_funding_minor,
	refund_vendor_merchandise_reduction_minor,refund_vendor_delivery_reduction_minor,refund_provider_delivery_reduction_minor,
	refund_platform_revenue_reduction_minor,refund_authorization_release_minor,
	refund_organization_recovery_minor,
	delivery_executor_mode,managed_organization_id,managed_membership_id,
	provider_delivery_released_minor,organization_delivery_released_minor,
	status,snapshot,snapshot_fingerprint,version,created_at,updated_at`

func scanCommerceAllocation(row commerceScanner) (walletmodels.CommerceAllocation, error) {
	var value walletmodels.CommerceAllocation
	err := row.Scan(&value.ID, &value.PaymentIntentID, &value.AllocationReference, &value.Scope, &value.VendorOrderID, &value.VendorID,
		&value.BranchID, &value.AssignedProviderType, &value.AssignedProviderID, &value.DeliveryResponsibility, &value.Currency,
		&value.CustomerPayableMinor, &value.MaxAuthorizedMinor, &value.MerchandiseGrossMinor, &value.VendorProductDiscountMinor, &value.PlatformProductDiscountMinor,
		&value.PlatformCommissionMinor, &value.DeliveryCustomerMinor, &value.VendorDeliverySubsidyMinor, &value.PlatformDeliverySubsidyMinor, &value.OtherFundingMinor,
		&value.VendorMerchandisePayableMinor, &value.VendorDeliveryPayableMinor, &value.ProviderDeliveryPayableMinor, &value.PlatformRevenueMinor,
		&value.VendorReleasedMinor, &value.DeliveryReleasedMinor, &value.RefundedMinor,
		&value.RefundCustomerMinor, &value.RefundPlatformFundingMinor, &value.RefundOtherFundingMinor, &value.RefundEscrowRecoveryMinor,
		&value.RefundVendorRecoveryMinor, &value.RefundProviderRecoveryMinor, &value.RefundPlatformRecoveryMinor, &value.RefundPlatformLossFundingMinor,
		&value.RefundVendorMerchandiseReductionMinor, &value.RefundVendorDeliveryReductionMinor, &value.RefundProviderDeliveryReductionMinor,
		&value.RefundPlatformRevenueReductionMinor, &value.RefundAuthorizationReleaseMinor,
		&value.RefundOrganizationRecoveryMinor,
		&value.DeliveryExecutorMode, &value.ManagedOrganizationID, &value.ManagedMembershipID,
		&value.ProviderDeliveryReleasedMinor, &value.OrganizationDeliveryReleasedMinor,
		&value.Status, &value.Snapshot, &value.SnapshotFingerprint,
		&value.Version, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

type commerceQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (r *PostgresWalletRepository) listCommerceAllocations(ctx context.Context, q commerceQueryer, intentID string) ([]walletmodels.CommerceAllocation, error) {
	rows, err := q.Query(ctx, `SELECT `+commerceAllocationColumns+` FROM commerce_payment_allocations WHERE payment_intent_id=$1 ORDER BY allocation_reference`, intentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []walletmodels.CommerceAllocation
	for rows.Next() {
		value, scanErr := scanCommerceAllocation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func commerceEscrowOwner(parentReference string) string { return "commerce:" + parentReference }

func (r *PostgresWalletRepository) HoldCommercePayment(ctx context.Context, parentReference, idempotencyKey string) (walletmodels.CommercePaymentIntent, error) {
	if strings.TrimSpace(parentReference) == "" || strings.TrimSpace(idempotencyKey) == "" {
		return walletmodels.CommercePaymentIntent{}, apperrors.Validation("Commerce reference and idempotency key are required.", nil)
	}
	var result walletmodels.CommercePaymentIntent
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		intent, loadErr := r.getCommerceIntentTx(ctx, tx, parentReference)
		if loadErr != nil {
			return loadErr
		}
		fingerprint, _ := stableFingerprint(struct{ Parent, Allocation string }{parentReference, intent.AllocationFingerprint})
		if operation, operationErr := r.findCommerceOperationTx(ctx, tx, "HOLD", idempotencyKey); operationErr == nil {
			if operation.RequestFingerprint != fingerprint {
				return apperrors.Conflict("Hold idempotency key was reused with different commerce evidence.", nil)
			}
			result = intent
			return nil
		} else if !errors.Is(operationErr, pgx.ErrNoRows) {
			return operationErr
		}
		if intent.Status != walletmodels.CommerceIntentCreated {
			if intent.Status == walletmodels.CommerceIntentHeld || intent.Status == walletmodels.CommerceIntentPartiallyReleased || intent.Status == walletmodels.CommerceIntentReleased {
				result = intent
				return nil
			}
			return apperrors.Conflict("Commerce intent cannot be held in its current state.", nil)
		}
		if intent.PaymentMethod != walletmodels.PaymentMethodWallet {
			return apperrors.Conflict("This commerce intent requires gateway confirmation before hold.", nil)
		}
		customer, ensureErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeCustomer, intent.CustomerID, walletmodels.AccountCustomerAvailable, intent.Currency, walletmodels.NormalCredit)
		if ensureErr != nil {
			return ensureErr
		}
		if _, lockErr := tx.Exec(ctx, `SELECT id FROM wallet_accounts WHERE id=$1 FOR UPDATE`, customer.ID); lockErr != nil {
			return lockErr
		}
		escrow, ensureErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob, commerceEscrowOwner(parentReference), walletmodels.AccountCommerceEscrow, intent.Currency, walletmodels.NormalCredit)
		if ensureErr != nil {
			return ensureErr
		}
		balance, balanceErr := r.accountBalanceTx(ctx, tx, customer.ID)
		if balanceErr != nil {
			return balanceErr
		}
		if balance < intent.MaxAuthorizedMinor {
			return apperrors.Conflict("Wallet balance is not enough for this commerce payment.", nil)
		}
		var platformFunding, otherFunding int64
		for _, allocation := range intent.Allocations {
			var ok bool
			platformFunding, ok = checkedNonnegativeSum(platformFunding, allocation.PlatformProductDiscountMinor, allocation.PlatformDeliverySubsidyMinor)
			if !ok {
				return apperrors.Validation("Commerce platform funding total overflowed.", nil)
			}
			otherFunding, ok = checkedNonnegativeSum(otherFunding, allocation.OtherFundingMinor)
			if !ok {
				return apperrors.Validation("Commerce external funding total overflowed.", nil)
			}
		}
		heldTotal, heldOK := checkedNonnegativeSum(intent.MaxAuthorizedMinor, platformFunding, otherFunding)
		if !heldOK {
			return apperrors.Validation("Commerce hold total overflowed.", nil)
		}
		entries := []walletmodels.LedgerEntry{{AccountID: customer.ID, Side: walletmodels.SideDebit, AmountKobo: intent.MaxAuthorizedMinor, Currency: intent.Currency, Memo: "Customer commerce maximum authorization hold"}}
		if platformFunding > 0 {
			expense, ensureErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypePlatform, "karry-go", walletmodels.AccountPlatformPromotionExpense, intent.Currency, walletmodels.NormalDebit)
			if ensureErr != nil {
				return ensureErr
			}
			entries = append(entries, walletmodels.LedgerEntry{AccountID: expense.ID, Side: walletmodels.SideDebit, AmountKobo: platformFunding, Currency: intent.Currency, Memo: "Platform commerce subsidy funding"})
		}
		if otherFunding > 0 {
			funding, ensureErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeSystem, "approved-commerce-funding", walletmodels.AccountOtherApprovedFunding, intent.Currency, walletmodels.NormalDebit)
			if ensureErr != nil {
				return ensureErr
			}
			entries = append(entries, walletmodels.LedgerEntry{AccountID: funding.ID, Side: walletmodels.SideDebit, AmountKobo: otherFunding, Currency: intent.Currency, Memo: "Other approved commerce funding"})
		}
		entries = append(entries, walletmodels.LedgerEntry{AccountID: escrow.ID, Side: walletmodels.SideCredit, AmountKobo: heldTotal, Currency: intent.Currency, Memo: "Immutable commerce allocations held"})
		ledgerRef := "commerce-hold-" + parentReference
		if postErr := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{ID: uuid.NewString(), Reference: ledgerRef, TransactionType: "commerce_payment_hold", Status: postedStatus, SourceService: "vendor-service", SourceReference: parentReference, IdempotencyKey: idempotencyKey}, entries); postErr != nil {
			return postErr
		}
		if _, updateErr := tx.Exec(ctx, `UPDATE commerce_payment_intents SET status='HELD',version=version+1,updated_at=now() WHERE payment_intent_id=$1;
			UPDATE payment_intents SET status=$2,updated_at=now() WHERE id=$1;
			UPDATE commerce_payment_allocations SET status='HELD',version=version+1,updated_at=now() WHERE payment_intent_id=$1`, intent.PaymentIntentID, walletmodels.PaymentStatusHeld); updateErr != nil {
			return updateErr
		}
		if insertErr := r.insertCommerceOperation(ctx, tx, intent.PaymentIntentID, "", "HOLD", parentReference, "", idempotencyKey, fingerprint, "APPLIED", heldTotal, ledgerRef); insertErr != nil {
			return insertErr
		}
		if evidenceErr := recordFinancialEvidence(ctx, tx, "commerce.hold.created.v1", "commerce.hold.created", "commerce_payment_intent", intent.PaymentIntentID, "vendor-service", intent.Version+1, map[string]any{"parent_commerce_reference": parentReference, "amount_minor": heldTotal, "currency": intent.Currency}); evidenceErr != nil {
			return evidenceErr
		}
		result, loadErr = r.getCommerceIntentTx(ctx, tx, parentReference)
		return loadErr
	})
	return result, err
}

func (r *PostgresWalletRepository) ReleaseVendorPayable(ctx context.Context, input ReleaseVendorPayableInput) (walletmodels.CommerceFinancialOperation, error) {
	return r.releaseCommerceComponent(ctx, input.VendorOrderID, input.CompletionReference, input.ExpectedSnapshotFingerprint, input.IdempotencyKey, "VENDOR_RELEASE", "", "", false)
}

func (r *PostgresWalletRepository) ReleaseCommerceDelivery(ctx context.Context, input ReleaseCommerceDeliveryInput) (walletmodels.CommerceFinancialOperation, error) {
	return r.releaseCommerceComponent(ctx, input.VendorOrderID, input.CompletionReference, input.ExpectedSnapshotFingerprint, input.IdempotencyKey, "DELIVERY_RELEASE", input.ProviderType, input.ProviderID, input.TrustedServiceCompletion)
}

func (r *PostgresWalletRepository) releaseCommerceComponent(ctx context.Context, vendorOrderID, completionReference, expectedSnapshot, idempotencyKey, operationType, providerType, providerID string, trustedServiceCompletion bool) (walletmodels.CommerceFinancialOperation, error) {
	if strings.TrimSpace(vendorOrderID) == "" || strings.TrimSpace(completionReference) == "" || (!trustedServiceCompletion && strings.TrimSpace(expectedSnapshot) == "") || strings.TrimSpace(idempotencyKey) == "" {
		return walletmodels.CommerceFinancialOperation{}, apperrors.Validation("Completion, snapshot and idempotency evidence are required.", nil)
	}
	var result walletmodels.CommerceFinancialOperation
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		allocation, parentReference, loadErr := r.lockCommerceAllocation(ctx, tx, vendorOrderID)
		if loadErr != nil {
			return loadErr
		}
		if trustedServiceCompletion {
			if operationType != "DELIVERY_RELEASE" || allocation.DeliveryResponsibility != walletmodels.DeliveryResponsibilityPlatform || strings.TrimSpace(providerType) == "" || strings.TrimSpace(providerID) == "" {
				return apperrors.Forbidden("Service-owned completion is not eligible for this allocation.", nil)
			}
			expectedSnapshot = allocation.SnapshotFingerprint
		}
		requestFingerprint, _ := stableFingerprint([]string{vendorOrderID, completionReference, expectedSnapshot, operationType, providerType, providerID})
		if existing, findErr := r.findCommerceOperationTx(ctx, tx, operationType, idempotencyKey); findErr == nil {
			if existing.RequestFingerprint != requestFingerprint {
				return apperrors.Conflict("Release idempotency key was reused with different completion evidence.", nil)
			}
			result = existing
			return nil
		} else if !errors.Is(findErr, pgx.ErrNoRows) {
			return findErr
		}
		var existingReleaseID string
		if loadErr = tx.QueryRow(ctx, `SELECT id FROM commerce_financial_operations WHERE allocation_id=$1 AND operation_type=$2`, allocation.ID, operationType).Scan(&existingReleaseID); loadErr == nil {
			return apperrors.Conflict("This allocation component already has an authoritative release operation.", nil)
		} else if !errors.Is(loadErr, pgx.ErrNoRows) {
			return loadErr
		}
		if allocation.SnapshotFingerprint != expectedSnapshot || allocation.Status == "CREATED" || allocation.Status == "REFUND_PENDING" {
			return apperrors.Conflict("Allocation is not eligible for this release.", nil)
		}
		escrow, ensureErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob, commerceEscrowOwner(parentReference), walletmodels.AccountCommerceEscrow, allocation.Currency, walletmodels.NormalCredit)
		if ensureErr != nil {
			return ensureErr
		}
		if _, lockErr := tx.Exec(ctx, `SELECT id FROM wallet_accounts WHERE id=$1 FOR UPDATE`, escrow.ID); lockErr != nil {
			return lockErr
		}
		var entries []walletmodels.LedgerEntry
		var amount int64
		if operationType == "VENDOR_RELEASE" {
			vendorAmount := allocation.VendorMerchandisePayableMinor - allocation.RefundVendorMerchandiseReductionMinor
			platformAmount := allocation.PlatformRevenueMinor - allocation.RefundPlatformRevenueReductionMinor
			amount = vendorAmount + platformAmount - allocation.VendorReleasedMinor
			if amount < 0 {
				return apperrors.Internal("Vendor release state is inconsistent.", nil)
			}
			if vendorAmount > 0 {
				vendor, e := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeVendor, allocation.VendorID, walletmodels.AccountVendorPayable, allocation.Currency, walletmodels.NormalCredit)
				if e != nil {
					return e
				}
				entries = append(entries, walletmodels.LedgerEntry{AccountID: vendor.ID, Side: walletmodels.SideCredit, AmountKobo: vendorAmount, Currency: allocation.Currency, Memo: "Vendor merchandise payable released"})
			}
			if platformAmount > 0 {
				platform, e := r.ensureAccount(ctx, tx, walletmodels.OwnerTypePlatform, "karry-go", walletmodels.AccountPlatformRevenue, allocation.Currency, walletmodels.NormalCredit)
				if e != nil {
					return e
				}
				entries = append(entries, walletmodels.LedgerEntry{AccountID: platform.ID, Side: walletmodels.SideCredit, AmountKobo: platformAmount, Currency: allocation.Currency, Memo: "Frozen platform commerce revenue"})
			}
		} else {
			vendorDeliveryAmount := allocation.VendorDeliveryPayableMinor - allocation.RefundVendorDeliveryReductionMinor
			providerDeliveryAmount := allocation.ProviderDeliveryPayableMinor - allocation.RefundProviderDeliveryReductionMinor
			amount = vendorDeliveryAmount + providerDeliveryAmount - allocation.DeliveryReleasedMinor
			if amount < 0 {
				return apperrors.Internal("Delivery release state is inconsistent.", nil)
			}
			if allocation.DeliveryResponsibility == walletmodels.DeliveryResponsibilityVendor {
				if strings.TrimSpace(providerType) != "" || strings.TrimSpace(providerID) != "" || allocation.ProviderDeliveryPayableMinor != 0 {
					return apperrors.Conflict("Vendor-owned delivery cannot create a platform provider payable.", nil)
				}
				if vendorDeliveryAmount > 0 {
					vendor, e := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeVendor, allocation.VendorID, walletmodels.AccountVendorPayable, allocation.Currency, walletmodels.NormalCredit)
					if e != nil {
						return e
					}
					entries = append(entries, walletmodels.LedgerEntry{AccountID: vendor.ID, Side: walletmodels.SideCredit, AmountKobo: vendorDeliveryAmount, Currency: allocation.Currency, Memo: "Vendor-owned delivery payable released"})
				}
			} else if allocation.DeliveryResponsibility == walletmodels.DeliveryResponsibilityPlatform {
				if strings.TrimSpace(providerType) == "" || strings.TrimSpace(providerID) == "" || allocation.VendorDeliveryPayableMinor != 0 {
					return apperrors.Validation("Authoritative provider identity is required for platform delivery settlement.", nil)
				}
				// WAL-022. A rider employed by an organization did not earn
				// the customer's delivery fee: what they are entitled to is
				// their compensation plan's answer, which Fleet resolves and
				// which is frequently zero. Crediting it here would pay a
				// salaried rider per delivery and never pay the organization
				// what it is owed, and neither is recoverable once posted.
				//
				// UNKNOWN is refused for the same reason rather than treated
				// as independent: a refused release can be retried once the
				// executor is known, a wrong payment cannot be taken back.
				if allocation.DeliveryExecutorMode == walletmodels.ExecutorManaged {
					return apperrors.Conflict(
						"Organization-managed delivery is settled through Fleet, not paid directly to the provider.", nil)
				}
				if allocation.DeliveryExecutorMode == walletmodels.ExecutorUnknown {
					return apperrors.Conflict(
						"Delivery executor is not yet known; the direct payout path is closed until it is.", nil)
				}
				provider, e := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider, providerOwnerID(providerType, providerID), walletmodels.AccountProviderPayable, allocation.Currency, walletmodels.NormalCredit)
				if e != nil {
					return e
				}
				entries = append(entries, walletmodels.LedgerEntry{AccountID: provider.ID, Side: walletmodels.SideCredit, AmountKobo: providerDeliveryAmount, Currency: allocation.Currency, Memo: "Platform delivery provider earning released"})
			} else if amount != 0 {
				return apperrors.Conflict("Allocation has no delivery beneficiary.", nil)
			}
		}
		ledgerRef := ""
		if amount > 0 {
			balance, e := r.accountBalanceTx(ctx, tx, escrow.ID)
			if e != nil {
				return e
			}
			if balance < amount {
				return apperrors.Conflict("Commerce escrow cannot cover this release.", nil)
			}
			entries = append([]walletmodels.LedgerEntry{{AccountID: escrow.ID, Side: walletmodels.SideDebit, AmountKobo: amount, Currency: allocation.Currency, Memo: "Release immutable commerce allocation"}}, entries...)
			ledgerRef = strings.ToLower(operationType) + "-" + vendorOrderID
			if e = r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{ID: uuid.NewString(), Reference: ledgerRef, TransactionType: strings.ToLower(operationType), Status: postedStatus, SourceService: "vendor-service", SourceReference: vendorOrderID, IdempotencyKey: idempotencyKey, Metadata: map[string]any{"completion_reference": completionReference}}, entries); e != nil {
				return e
			}
		}
		if operationType == "VENDOR_RELEASE" {
			_, loadErr = tx.Exec(ctx, `UPDATE commerce_payment_allocations SET vendor_released_minor=vendor_released_minor+$2,status=CASE WHEN delivery_released_minor=vendor_delivery_payable_minor-refund_vendor_delivery_reduction_minor+provider_delivery_payable_minor-refund_provider_delivery_reduction_minor THEN 'RELEASED' ELSE 'PARTIALLY_RELEASED' END,version=version+1,updated_at=now() WHERE id=$1`, allocation.ID, amount)
		} else {
			// An independent provider earned the whole frozen amount, so the
			// provider column carries all of it and the organization column
			// none. Recording the split even here keeps "what did each party
			// get?" answerable without knowing which path settled it.
			_, loadErr = tx.Exec(ctx, `UPDATE commerce_payment_allocations SET delivery_released_minor=delivery_released_minor+$2,provider_delivery_released_minor=provider_delivery_released_minor+$5,assigned_provider_type=$3,assigned_provider_id=$4,status=CASE WHEN vendor_released_minor=vendor_merchandise_payable_minor-refund_vendor_merchandise_reduction_minor+platform_revenue_minor-refund_platform_revenue_reduction_minor THEN 'RELEASED' ELSE 'PARTIALLY_RELEASED' END,version=version+1,updated_at=now() WHERE id=$1`, allocation.ID, amount, providerType, providerID, independentProviderShare(allocation, amount))
		}
		if loadErr != nil {
			return loadErr
		}
		if loadErr = r.refreshCommerceIntentState(ctx, tx, allocation.PaymentIntentID); loadErr != nil {
			return loadErr
		}
		if loadErr = r.insertCommerceOperation(ctx, tx, allocation.PaymentIntentID, allocation.ID, operationType, completionReference, "", idempotencyKey, requestFingerprint, "APPLIED", amount, ledgerRef); loadErr != nil {
			return loadErr
		}
		eventType, action := "commerce.allocation.released.v1", "commerce.allocation.released"
		if operationType == "VENDOR_RELEASE" {
			eventType, action = "vendor.payable.released.v1", "vendor.payable.released"
		}
		if loadErr = recordFinancialEvidence(ctx, tx, eventType, action, "commerce_allocation", allocation.ID, "vendor-service", allocation.Version+1, map[string]any{"vendor_order_id": vendorOrderID, "completion_reference": completionReference, "amount_minor": amount, "currency": allocation.Currency}); loadErr != nil {
			return loadErr
		}
		result, loadErr = r.findCommerceOperationTx(ctx, tx, operationType, idempotencyKey)
		return loadErr
	})
	return result, err
}

func (r *PostgresWalletRepository) RefundCommerceAllocation(ctx context.Context, input RefundCommerceAllocationInput) (walletmodels.CommerceFinancialOperation, error) {
	operationType, typeErr := refundOperationType(input.Kind)
	if typeErr != nil {
		return walletmodels.CommerceFinancialOperation{}, typeErr
	}
	values := []int64{input.CustomerRefundMinor, input.PlatformFundingReversalMinor, input.OtherFundingReversalMinor, input.EscrowRecoveryMinor, input.VendorRecoveryMinor, input.ProviderRecoveryMinor, input.OrganizationRecoveryMinor, input.PlatformRecoveryMinor, input.PlatformLossFundingMinor, input.VendorMerchandiseReductionMinor, input.VendorDeliveryReductionMinor, input.ProviderDeliveryReductionMinor, input.PlatformRevenueReductionMinor, input.AuthorizationReleaseMinor}
	for _, value := range values {
		if value < 0 {
			return walletmodels.CommerceFinancialOperation{}, apperrors.Validation("Refund amounts cannot be negative.", nil)
		}
	}
	output, outputOK := checkedNonnegativeSum(input.CustomerRefundMinor, input.PlatformFundingReversalMinor, input.OtherFundingReversalMinor)
	inputTotal, inputOK := checkedNonnegativeSum(input.EscrowRecoveryMinor, input.VendorRecoveryMinor, input.ProviderRecoveryMinor, input.OrganizationRecoveryMinor, input.PlatformRecoveryMinor, input.PlatformLossFundingMinor)
	allocationReduction, reductionOK := checkedNonnegativeSum(input.VendorMerchandiseReductionMinor, input.VendorDeliveryReductionMinor, input.ProviderDeliveryReductionMinor, input.PlatformRevenueReductionMinor, input.AuthorizationReleaseMinor)
	if strings.TrimSpace(input.VendorOrderID) == "" || strings.TrimSpace(input.ItemReference) == "" || strings.TrimSpace(input.ExpectedSnapshotFingerprint) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || output <= 0 || output != inputTotal || allocationReduction != output-input.PlatformLossFundingMinor {
		return walletmodels.CommerceFinancialOperation{}, apperrors.Validation("Refund breakdown is incomplete or unbalanced.", nil)
	}
	if !outputOK || !inputOK || !reductionOK {
		return walletmodels.CommerceFinancialOperation{}, apperrors.Validation("Refund breakdown overflows minor-unit accounting.", nil)
	}
	requestFingerprint, _ := stableFingerprint(input)
	var result walletmodels.CommerceFinancialOperation
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		if existing, e := r.findCommerceOperationTx(ctx, tx, operationType, input.IdempotencyKey); e == nil {
			if existing.RequestFingerprint != requestFingerprint {
				return apperrors.Conflict("Refund idempotency key was reused with a different allocation adjustment.", nil)
			}
			if existing.Status != "PENDING_LIABILITY" {
				result = existing
				return nil
			}
			// A pending liability moved no money and changed no allocation
			// counters. Re-enter the same immutable command so recovery can
			// converge after the responsible beneficiary balance is restored.
			tag, deleteErr := tx.Exec(ctx, `DELETE FROM commerce_financial_operations WHERE id=$1 AND status='PENDING_LIABILITY'`, existing.ID)
			if deleteErr != nil {
				return deleteErr
			}
			if tag.RowsAffected() == 0 {
				result, e = r.findCommerceOperationTx(ctx, tx, operationType, input.IdempotencyKey)
				return e
			}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		allocation, parentReference, e := r.lockCommerceAllocation(ctx, tx, input.VendorOrderID)
		if e != nil {
			return e
		}
		var priorFingerprint string
		if e = tx.QueryRow(ctx, `SELECT request_fingerprint FROM commerce_financial_operations WHERE allocation_id=$1 AND operation_type=$3 AND item_reference=$2`, allocation.ID, input.ItemReference, operationType).Scan(&priorFingerprint); e == nil {
			return apperrors.Conflict("This allocation item already has an authoritative refund operation.", nil)
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		refundTotal, refundOK := checkedNonnegativeSum(allocation.RefundedMinor, output)
		fundedTotal, fundedOK := checkedNonnegativeSum(allocation.MaxAuthorizedMinor, allocation.PlatformProductDiscountMinor, allocation.PlatformDeliverySubsidyMinor, allocation.OtherFundingMinor)
		customerTotal, customerOK := checkedNonnegativeSum(allocation.RefundCustomerMinor, input.CustomerRefundMinor)
		platformFundingTotal, platformFundingOK := checkedNonnegativeSum(allocation.RefundPlatformFundingMinor, input.PlatformFundingReversalMinor)
		platformFundingCap, platformFundingCapOK := checkedNonnegativeSum(allocation.PlatformProductDiscountMinor, allocation.PlatformDeliverySubsidyMinor)
		otherFundingTotal, otherFundingOK := checkedNonnegativeSum(allocation.RefundOtherFundingMinor, input.OtherFundingReversalMinor)
		vendorMerchTotal, vendorMerchOK := checkedNonnegativeSum(allocation.RefundVendorMerchandiseReductionMinor, input.VendorMerchandiseReductionMinor)
		vendorDeliveryTotal, vendorDeliveryOK := checkedNonnegativeSum(allocation.RefundVendorDeliveryReductionMinor, input.VendorDeliveryReductionMinor)
		providerDeliveryTotal, providerDeliveryOK := checkedNonnegativeSum(allocation.RefundProviderDeliveryReductionMinor, input.ProviderDeliveryReductionMinor)
		platformRevenueTotal, platformRevenueOK := checkedNonnegativeSum(allocation.RefundPlatformRevenueReductionMinor, input.PlatformRevenueReductionMinor)
		authorizationTotal, authorizationOK := checkedNonnegativeSum(allocation.RefundAuthorizationReleaseMinor, input.AuthorizationReleaseMinor)
		if allocation.SnapshotFingerprint != input.ExpectedSnapshotFingerprint || !refundOK || !fundedOK || !customerOK || !platformFundingOK || !platformFundingCapOK || !otherFundingOK || !vendorMerchOK || !vendorDeliveryOK || !providerDeliveryOK || !platformRevenueOK || !authorizationOK ||
			refundTotal > fundedTotal || customerTotal > allocation.MaxAuthorizedMinor ||
			platformFundingTotal > platformFundingCap || otherFundingTotal > allocation.OtherFundingMinor ||
			vendorMerchTotal > allocation.VendorMerchandisePayableMinor || vendorDeliveryTotal > allocation.VendorDeliveryPayableMinor ||
			providerDeliveryTotal > allocation.ProviderDeliveryPayableMinor || platformRevenueTotal > allocation.PlatformRevenueMinor ||
			authorizationTotal > allocation.MaxAuthorizedMinor-allocation.CustomerPayableMinor {
			return apperrors.Conflict("Refund exceeds or does not match the immutable allocation.", nil)
		}
		recoveryPool, e := r.deliveryRecoveryPool(ctx, tx, allocation)
		if e != nil {
			return e
		}
		if e = validateRefundRecoverySources(allocation, input, recoveryPool); e != nil {
			return e
		}
		escrow, e := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob, commerceEscrowOwner(parentReference), walletmodels.AccountCommerceEscrow, allocation.Currency, walletmodels.NormalCredit)
		if e != nil {
			return e
		}
		customerID := ""
		if e = tx.QueryRow(ctx, `SELECT p.customer_id FROM payment_intents p WHERE p.id=$1`, allocation.PaymentIntentID).Scan(&customerID); e != nil {
			return e
		}
		var debits []walletmodels.LedgerEntry
		pending := false
		addDebit := func(account walletmodels.Account, amount int64, memo string) error {
			if amount == 0 {
				return nil
			}
			if _, lockErr := tx.Exec(ctx, `SELECT id FROM wallet_accounts WHERE id=$1 FOR UPDATE`, account.ID); lockErr != nil {
				return lockErr
			}
			balance, balanceErr := r.accountBalanceTx(ctx, tx, account.ID)
			if balanceErr != nil {
				return balanceErr
			}
			if balance < amount {
				pending = true
				return nil
			}
			debits = append(debits, walletmodels.LedgerEntry{AccountID: account.ID, Side: walletmodels.SideDebit, AmountKobo: amount, Currency: allocation.Currency, Memo: memo})
			return nil
		}
		if e = addDebit(escrow, input.EscrowRecoveryMinor, "Refund unreleased commerce allocation"); e != nil {
			return e
		}
		if input.VendorRecoveryMinor > 0 {
			account, x := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeVendor, allocation.VendorID, walletmodels.AccountVendorPayable, allocation.Currency, walletmodels.NormalCredit)
			if x != nil {
				return x
			}
			if e = addDebit(account, input.VendorRecoveryMinor, "Recover released Vendor payable"); e != nil {
				return e
			}
		}
		if input.ProviderRecoveryMinor > 0 {
			if allocation.AssignedProviderID == "" {
				pending = true
			} else {
				account, x := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider, providerOwnerID(allocation.AssignedProviderType, allocation.AssignedProviderID), walletmodels.AccountProviderPayable, allocation.Currency, walletmodels.NormalCredit)
				if x != nil {
					return x
				}
				if e = addDebit(account, input.ProviderRecoveryMinor, "Recover provider delivery earning"); e != nil {
					return e
				}
			}
		}
		// WAL-029. On a corporate delivery the organization received part of
		// the money -- all of it, when the rider is salaried -- so it is the
		// organization that gives that part back. Recovering it from the rider
		// instead would take money they never had.
		if input.OrganizationRecoveryMinor > 0 {
			if !recoveryPool.Managed || allocation.ManagedOrganizationID == "" {
				return apperrors.Conflict("Only an organization-managed delivery can recover from an organization.", nil)
			}
			account, x := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeOrganization, allocation.ManagedOrganizationID, walletmodels.AccountOrganizationPayable, allocation.Currency, walletmodels.NormalCredit)
			if x != nil {
				return x
			}
			if e = addDebit(account, input.OrganizationRecoveryMinor, "Recover organization delivery entitlement"); e != nil {
				return e
			}
		}
		if input.PlatformRecoveryMinor > 0 {
			account, x := r.ensureAccount(ctx, tx, walletmodels.OwnerTypePlatform, "karry-go", walletmodels.AccountPlatformRevenue, allocation.Currency, walletmodels.NormalCredit)
			if x != nil {
				return x
			}
			if e = addDebit(account, input.PlatformRecoveryMinor, "Reverse platform commerce revenue"); e != nil {
				return e
			}
		}
		if input.PlatformLossFundingMinor > 0 {
			expense, x := r.ensureAccount(ctx, tx, walletmodels.OwnerTypePlatform, "karry-go", walletmodels.AccountPlatformPromotionExpense, allocation.Currency, walletmodels.NormalDebit)
			if x != nil {
				return x
			}
			if _, x = tx.Exec(ctx, `SELECT id FROM wallet_accounts WHERE id=$1 FOR UPDATE`, expense.ID); x != nil {
				return x
			}
			debits = append(debits, walletmodels.LedgerEntry{AccountID: expense.ID, Side: walletmodels.SideDebit, AmountKobo: input.PlatformLossFundingMinor, Currency: allocation.Currency, Memo: "Platform-funded customer protection refund"})
		}
		status := "APPLIED"
		ledgerRef := ""
		if pending {
			status = "PENDING_LIABILITY"
			_, e = tx.Exec(ctx, `UPDATE commerce_payment_allocations SET status='REFUND_PENDING',version=version+1,updated_at=now() WHERE id=$1`, allocation.ID)
			if e != nil {
				return e
			}
		} else {
			credits := []walletmodels.LedgerEntry{}
			if input.CustomerRefundMinor > 0 {
				customer, x := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeCustomer, customerID, walletmodels.AccountCustomerAvailable, allocation.Currency, walletmodels.NormalCredit)
				if x != nil {
					return x
				}
				credits = append(credits, walletmodels.LedgerEntry{AccountID: customer.ID, Side: walletmodels.SideCredit, AmountKobo: input.CustomerRefundMinor, Currency: allocation.Currency, Memo: "Customer allocation refund"})
			}
			if input.PlatformFundingReversalMinor > 0 {
				expense, x := r.ensureAccount(ctx, tx, walletmodels.OwnerTypePlatform, "karry-go", walletmodels.AccountPlatformPromotionExpense, allocation.Currency, walletmodels.NormalDebit)
				if x != nil {
					return x
				}
				credits = append(credits, walletmodels.LedgerEntry{AccountID: expense.ID, Side: walletmodels.SideCredit, AmountKobo: input.PlatformFundingReversalMinor, Currency: allocation.Currency, Memo: "Reverse platform subsidy funding"})
			}
			if input.OtherFundingReversalMinor > 0 {
				funding, x := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeSystem, "approved-commerce-funding", walletmodels.AccountOtherApprovedFunding, allocation.Currency, walletmodels.NormalDebit)
				if x != nil {
					return x
				}
				credits = append(credits, walletmodels.LedgerEntry{AccountID: funding.ID, Side: walletmodels.SideCredit, AmountKobo: input.OtherFundingReversalMinor, Currency: allocation.Currency, Memo: "Reverse other approved commerce funding"})
			}
			ledgerRef = "commerce-refund-" + input.IdempotencyKey
			// The ledger holds one journal per source operation. Keyed on the
			// order alone, a SECOND partial refund of the same order collided
			// with the first and failed outright -- so partial refunds were
			// only ever possible once. The operation's own identity is the
			// order, the item and the kind, which is what makes repeated
			// partial refunds work and still keeps one journal per operation.
			transactionType := "commerce_partial_refund"
			if operationType == commerceDisputeReversalOperation {
				transactionType = "commerce_dispute_reversal"
			}
			if e = r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{ID: uuid.NewString(), Reference: ledgerRef, TransactionType: transactionType, Status: postedStatus, SourceService: "vendor-service", SourceReference: input.VendorOrderID + ":" + input.ItemReference, IdempotencyKey: input.IdempotencyKey, Metadata: map[string]any{"item_reference": input.ItemReference, "vendor_order_id": input.VendorOrderID}}, append(debits, credits...)); e != nil {
				return e
			}
			if recoveryPool.Managed && (input.ProviderRecoveryMinor > 0 || input.OrganizationRecoveryMinor > 0) {
				if e = r.applyDeliveryRecovery(ctx, tx, recoveryPool, walletmodels.DeliveryRecoverySplit{
					ProviderMinor: input.ProviderRecoveryMinor, OrganizationMinor: input.OrganizationRecoveryMinor,
				}); e != nil {
					return e
				}
			}
			_, e = tx.Exec(ctx, `UPDATE commerce_payment_allocations SET
				refunded_minor=refunded_minor+$2,
				refund_customer_minor=refund_customer_minor+$3,
				refund_platform_funding_minor=refund_platform_funding_minor+$4,
				refund_other_funding_minor=refund_other_funding_minor+$5,
				refund_escrow_recovery_minor=refund_escrow_recovery_minor+$6,
				refund_vendor_recovery_minor=refund_vendor_recovery_minor+$7,
				refund_provider_recovery_minor=refund_provider_recovery_minor+$8,
				refund_platform_recovery_minor=refund_platform_recovery_minor+$9,
				refund_organization_recovery_minor=refund_organization_recovery_minor+$16,
				refund_platform_loss_funding_minor=refund_platform_loss_funding_minor+$10,
				refund_vendor_merchandise_reduction_minor=refund_vendor_merchandise_reduction_minor+$11,
				refund_vendor_delivery_reduction_minor=refund_vendor_delivery_reduction_minor+$12,
				refund_provider_delivery_reduction_minor=refund_provider_delivery_reduction_minor+$13,
				refund_platform_revenue_reduction_minor=refund_platform_revenue_reduction_minor+$14,
				refund_authorization_release_minor=refund_authorization_release_minor+$15,
				status=CASE WHEN refunded_minor+$2=max_authorized_minor+platform_product_discount_minor+platform_delivery_subsidy_minor+other_funding_minor THEN 'REFUNDED' ELSE 'PARTIALLY_REFUNDED' END,
				version=version+1,updated_at=now() WHERE id=$1`, allocation.ID, output,
				input.CustomerRefundMinor, input.PlatformFundingReversalMinor, input.OtherFundingReversalMinor,
				input.EscrowRecoveryMinor, input.VendorRecoveryMinor, input.ProviderRecoveryMinor, input.PlatformRecoveryMinor,
				input.PlatformLossFundingMinor, input.VendorMerchandiseReductionMinor, input.VendorDeliveryReductionMinor,
				input.ProviderDeliveryReductionMinor, input.PlatformRevenueReductionMinor, input.AuthorizationReleaseMinor,
				input.OrganizationRecoveryMinor)
			if e != nil {
				return e
			}
			if e = r.refreshCommerceIntentState(ctx, tx, allocation.PaymentIntentID); e != nil {
				return e
			}
		}
		if e = r.insertCommerceOperation(ctx, tx, allocation.PaymentIntentID, allocation.ID, operationType, input.VendorOrderID, input.ItemReference, input.IdempotencyKey, requestFingerprint, status, output, ledgerRef); e != nil {
			return e
		}
		eventType, action := "commerce.refund.created.v1", "commerce.refund.created"
		if operationType == commerceDisputeReversalOperation {
			eventType, action = "commerce.dispute_reversal.created.v1", "commerce.dispute_reversal.created"
		}
		if e = recordFinancialEvidence(ctx, tx, eventType, action, "commerce_allocation", allocation.ID, "vendor-service", allocation.Version+1, map[string]any{
			"vendor_order_id":           input.VendorOrderID,
			"item_reference":            input.ItemReference,
			"amount_minor":              output,
			"status":                    status,
			"currency":                  allocation.Currency,
			"provider_recovery_minor":   input.ProviderRecoveryMinor,
			"organization_recovery_min": input.OrganizationRecoveryMinor,
		}); e != nil {
			return e
		}
		result, e = r.findCommerceOperationTx(ctx, tx, operationType, input.IdempotencyKey)
		return e
	})
	return result, err
}

// commerceDisputeReversalOperation is a dispute-driven reversal.
const (
	commercePartialRefundOperation   = "PARTIAL_REFUND"
	commerceDisputeReversalOperation = "DISPUTE_REVERSAL"
)

// RefundKindRefund and RefundKindDispute name the two ways delivery money comes
// back. They move money identically and draw on one pool, so a dispute after a
// refund -- or a refund after a dispute -- can never between them take back
// more than was paid out.
const (
	RefundKindRefund  = "REFUND"
	RefundKindDispute = "DISPUTE"
)

func refundOperationType(kind string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case "", RefundKindRefund:
		return commercePartialRefundOperation, nil
	case RefundKindDispute:
		return commerceDisputeReversalOperation, nil
	default:
		return "", apperrors.Validation("Refund kind is not recognised.", nil)
	}
}

func validateRefundRecoverySources(allocation walletmodels.CommerceAllocation, input RefundCommerceAllocationInput, pool DeliveryRecoveryPool) error {
	vendorReleasedGross := int64(0)
	platformReleasedGross := int64(0)
	providerReleasedGross := int64(0)
	if allocation.VendorReleasedMinor > 0 {
		vendorReleasedGross = allocation.VendorMerchandisePayableMinor
		platformReleasedGross = allocation.PlatformRevenueMinor
	}
	if allocation.DeliveryReleasedMinor > 0 {
		if allocation.DeliveryResponsibility == walletmodels.DeliveryResponsibilityVendor {
			var ok bool
			vendorReleasedGross, ok = checkedNonnegativeSum(vendorReleasedGross, allocation.VendorDeliveryPayableMinor)
			if !ok {
				return apperrors.Conflict("Refund recovery source exceeds this allocation's authoritative balance.", nil)
			}
		} else {
			providerReleasedGross = allocation.ProviderDeliveryPayableMinor
		}
	}
	// WAL-029. A corporate delivery's caps come from what was actually paid,
	// recorded per mission at settlement time -- not from the customer-facing
	// delivery payable, which is what the CUSTOMER paid and not what the
	// provider received. Using the payable would let a refund claw the
	// organization's share out of a salaried rider who received nothing.
	//
	// Independent deliveries keep the legacy cap untouched.
	if pool.Managed {
		if input.ProviderRecoveryMinor > pool.ProviderRecoverableMinor ||
			input.OrganizationRecoveryMinor > pool.OrganizationRecoverableMinor {
			return apperrors.Conflict("Refund recovery source exceeds this allocation's authoritative balance.", nil)
		}
		// The legacy provider cap is computed from the payable and would be
		// wrong here, so the evidence-derived one replaces it outright.
		providerReleasedGross = pool.ProviderRecoverableMinor + allocation.RefundProviderRecoveryMinor
	} else if input.OrganizationRecoveryMinor > 0 {
		return apperrors.Conflict("An independent delivery has no organization to recover from.", nil)
	}
	held, heldOK := checkedNonnegativeSum(allocation.MaxAuthorizedMinor, allocation.PlatformProductDiscountMinor, allocation.PlatformDeliverySubsidyMinor, allocation.OtherFundingMinor)
	releasedAndRecovered, releasedOK := checkedNonnegativeSum(allocation.VendorReleasedMinor, allocation.DeliveryReleasedMinor, allocation.RefundEscrowRecoveryMinor)
	if !heldOK || !releasedOK || releasedAndRecovered > held || allocation.RefundVendorRecoveryMinor > vendorReleasedGross || allocation.RefundProviderRecoveryMinor > providerReleasedGross || allocation.RefundPlatformRecoveryMinor > platformReleasedGross {
		return apperrors.Conflict("Refund recovery source exceeds this allocation's authoritative balance.", nil)
	}
	escrowAvailable := held - releasedAndRecovered
	vendorReleasedAvailable := vendorReleasedGross - allocation.RefundVendorRecoveryMinor
	providerReleasedAvailable := providerReleasedGross - allocation.RefundProviderRecoveryMinor
	platformReleasedAvailable := platformReleasedGross - allocation.RefundPlatformRecoveryMinor
	if input.EscrowRecoveryMinor > escrowAvailable || input.VendorRecoveryMinor > vendorReleasedAvailable || input.ProviderRecoveryMinor > providerReleasedAvailable || input.PlatformRecoveryMinor > platformReleasedAvailable {
		return apperrors.Conflict("Refund recovery source exceeds this allocation's authoritative balance.", nil)
	}
	return nil
}

func checkedNonnegativeSum(values ...int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value < 0 || total > math.MaxInt64-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func (r *PostgresWalletRepository) lockCommerceAllocation(ctx context.Context, tx pgx.Tx, vendorOrderID string) (walletmodels.CommerceAllocation, string, error) {
	allocation, err := scanCommerceAllocation(tx.QueryRow(ctx, `SELECT `+commerceAllocationColumns+` FROM commerce_payment_allocations WHERE vendor_order_id=$1 FOR UPDATE`, vendorOrderID))
	if err != nil {
		return allocation, "", mapNotFound(err, "Commerce allocation could not be found.")
	}
	var parent string
	err = tx.QueryRow(ctx, `SELECT parent_commerce_reference FROM commerce_payment_intents WHERE payment_intent_id=$1 FOR UPDATE`, allocation.PaymentIntentID).Scan(&parent)
	return allocation, parent, err
}

func (r *PostgresWalletRepository) refreshCommerceIntentState(ctx context.Context, tx pgx.Tx, intentID string) error {
	_, err := tx.Exec(ctx, `UPDATE commerce_payment_intents c SET status=s.next_status,version=c.version+1,updated_at=now() FROM (
		SELECT payment_intent_id,CASE
		 WHEN bool_and(status='REFUNDED') THEN 'REFUNDED'
		 WHEN bool_or(status IN ('PARTIALLY_REFUNDED','REFUND_PENDING','REFUNDED')) THEN 'PARTIALLY_REFUNDED'
		 WHEN bool_and(status='RELEASED') THEN 'RELEASED'
		 WHEN bool_or(status='PARTIALLY_RELEASED') THEN 'PARTIALLY_RELEASED'
		 ELSE 'HELD' END AS next_status FROM commerce_payment_allocations WHERE payment_intent_id=$1 GROUP BY payment_intent_id
	) s WHERE c.payment_intent_id=s.payment_intent_id`, intentID)
	return err
}

func (r *PostgresWalletRepository) insertCommerceOperation(ctx context.Context, tx pgx.Tx, intentID, allocationID, operationType, sourceReference, itemReference, idempotencyKey, fingerprint, status string, amount int64, ledgerRef string) error {
	_, err := tx.Exec(ctx, `INSERT INTO commerce_financial_operations(id,payment_intent_id,allocation_id,operation_type,source_reference,item_reference,idempotency_key,request_fingerprint,status,amount_minor,ledger_reference)
		VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11)`, "cfo_"+uuid.NewString(), intentID, allocationID, operationType, sourceReference, itemReference, idempotencyKey, fingerprint, status, amount, ledgerRef)
	return err
}

const commerceOperationColumns = `id,payment_intent_id::text,COALESCE(allocation_id,''),operation_type,source_reference,item_reference,idempotency_key,status,amount_minor,ledger_reference,result,created_at,updated_at,request_fingerprint`

func scanCommerceOperation(row commerceScanner) (walletmodels.CommerceFinancialOperation, error) {
	var value walletmodels.CommerceFinancialOperation
	err := row.Scan(&value.ID, &value.PaymentIntentID, &value.AllocationID, &value.OperationType, &value.SourceReference, &value.ItemReference, &value.IdempotencyKey, &value.Status, &value.AmountMinor, &value.LedgerReference, &value.Result, &value.CreatedAt, &value.UpdatedAt, &value.RequestFingerprint)
	if value.Result == nil {
		value.Result = json.RawMessage(`{}`)
	}
	return value, err
}

func (r *PostgresWalletRepository) findCommerceOperationTx(ctx context.Context, tx pgx.Tx, operationType, idempotencyKey string) (walletmodels.CommerceFinancialOperation, error) {
	value, err := scanCommerceOperation(tx.QueryRow(ctx, `SELECT `+commerceOperationColumns+` FROM commerce_financial_operations WHERE operation_type=$1 AND idempotency_key=$2`, operationType, idempotencyKey))
	return value, err
}

func (r *PostgresWalletRepository) GetCommerceFinancialOperation(ctx context.Context, operationType, idempotencyKey string) (walletmodels.CommerceFinancialOperation, error) {
	value, err := scanCommerceOperation(r.db.QueryRow(ctx, `SELECT `+commerceOperationColumns+` FROM commerce_financial_operations WHERE operation_type=$1 AND idempotency_key=$2`, operationType, idempotencyKey))
	if err != nil {
		return value, mapNotFound(err, "Commerce financial operation could not be found.")
	}
	return value, nil
}

func (r *PostgresWalletRepository) VendorPayableSummary(ctx context.Context, vendorID, currency string) (walletmodels.WalletSummary, error) {
	currency = walletmodels.DefaultCurrency(currency)
	available, err := r.accountBalance(ctx, walletmodels.OwnerTypeVendor, vendorID, walletmodels.AccountVendorPayable, currency)
	if err != nil {
		return walletmodels.WalletSummary{}, err
	}
	var held int64
	err = r.db.QueryRow(ctx, `SELECT COALESCE(sum(GREATEST(vendor_merchandise_payable_minor+effective_vendor_delivery_payable_minor-vendor_released_minor-refunded_minor,0)),0) FROM commerce_payment_allocations WHERE vendor_id=$1 AND currency=$2 AND status IN ('CREATED','HELD','PARTIALLY_RELEASED','REFUND_PENDING','PARTIALLY_REFUNDED')`, vendorID, currency).Scan(&held)
	if err != nil {
		return walletmodels.WalletSummary{}, fmt.Errorf("vendor payable summary: %w", err)
	}
	return walletmodels.WalletSummary{OwnerType: walletmodels.OwnerTypeVendor, OwnerID: vendorID, Currency: currency, AvailableKobo: available, PendingKobo: held}, nil
}

func (r *PostgresWalletRepository) ProvisionVendorPayable(ctx context.Context, vendorID, currency string) (walletmodels.Account, error) {
	if strings.TrimSpace(vendorID) == "" {
		return walletmodels.Account{}, apperrors.Validation("Vendor id is required.", nil)
	}
	var account walletmodels.Account
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		account, err = r.ensureAccount(ctx, tx, walletmodels.OwnerTypeVendor, vendorID, walletmodels.AccountVendorPayable, walletmodels.DefaultCurrency(currency), walletmodels.NormalCredit)
		return err
	})
	return account, err
}

func (r *PostgresWalletRepository) ListVendorWithdrawals(ctx context.Context, vendorID string, limit int) ([]walletmodels.Withdrawal, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, selectWithdrawalSQL()+` WHERE provider_type='vendor_business' AND provider_id=$1 ORDER BY created_at DESC LIMIT $2`, vendorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []walletmodels.Withdrawal
	for rows.Next() {
		value, scanErr := scanWithdrawal(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r *PostgresWalletRepository) GetVendorWithdrawal(ctx context.Context, vendorID, reference string) (walletmodels.Withdrawal, error) {
	value, err := scanWithdrawal(r.db.QueryRow(ctx, selectWithdrawalSQL()+` WHERE provider_type='vendor_business' AND provider_id=$1 AND reference=$2`, vendorID, reference))
	if err != nil {
		return value, mapNotFound(err, "Vendor withdrawal could not be found.")
	}
	return value, nil
}

// independentProviderShare is what an independent provider is credited by a
// direct release.
//
// A vendor-owned delivery credits the vendor, not a provider, so the provider
// column must stay at zero there -- otherwise the split columns would claim a
// provider was paid when none was involved.
func independentProviderShare(allocation walletmodels.CommerceAllocation, amount int64) int64 {
	if allocation.DeliveryResponsibility != walletmodels.DeliveryResponsibilityPlatform {
		return 0
	}
	return amount
}
