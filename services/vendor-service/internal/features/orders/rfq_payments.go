package orders

// RFQ payment policy and conversion. The Vendor service stores the accepted
// commercial obligation and recovery evidence; Wallet remains the authority
// for every payment, hold, and refund effect.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"github.com/jackc/pgx/v5"
)

var (
	ErrSection21Payment       = errors.New("section21: payment authority unavailable")
	ErrSection21PaymentPolicy = errors.New("section21: invalid payment policy")
)

type RFQPaymentPolicy struct {
	Type         string
	DepositMinor int64
	DepositBPS   int64
}

func parseRFQPaymentPolicy(raw []byte, total int64) (RFQPaymentPolicy, error) {
	policy := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &policy); err != nil {
			return RFQPaymentPolicy{}, ErrSection21PaymentPolicy
		}
	}
	typ, _ := policy["type"].(string)
	typ = strings.ToUpper(strings.TrimSpace(typ))
	if typ == "" {
		typ = "FULL_PAYMENT"
	}
	out := RFQPaymentPolicy{Type: typ}
	switch typ {
	case "FULL_PAYMENT":
		out.DepositMinor = total
	case "DEPOSIT_FIXED":
		out.DepositMinor = policyInt64(policy, "amount_minor")
		if out.DepositMinor <= 0 || out.DepositMinor > total {
			return RFQPaymentPolicy{}, ErrSection21PaymentPolicy
		}
	case "DEPOSIT_PERCENTAGE":
		out.DepositBPS = policyInt64(policy, "basis_points")
		if out.DepositBPS <= 0 || out.DepositBPS > 10000 || total < 0 {
			return RFQPaymentPolicy{}, ErrSection21PaymentPolicy
		}
		// Percentage deposits use deterministic ceiling rounding so a positive
		// policy never silently becomes a zero authorization.
		product, ok := safeMultiply(total, out.DepositBPS)
		if !ok {
			return RFQPaymentPolicy{}, ErrSection21PaymentPolicy
		}
		out.DepositMinor = product / 10000
		if product%10000 != 0 {
			out.DepositMinor++
		}
		if out.DepositMinor > total {
			out.DepositMinor = total
		}
	default:
		return RFQPaymentPolicy{}, ErrSection21PaymentPolicy
	}
	return out, nil
}

func policyInt64(policy map[string]any, key string) int64 {
	switch value := policy[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	case json.Number:
		var out int64
		_, _ = fmt.Sscan(string(value), &out)
		return out
	default:
		return 0
	}
}

func (s *Section21Service) ensureRFQPaymentAndOrder(ctx context.Context, requesterID, rfqID, bindingID, checkoutID, fingerprint, vendor, currency string, total int64, paymentRaw, cancellationRaw []byte) error {
	policy, err := parseRFQPaymentPolicy(paymentRaw, total)
	if err != nil {
		return err
	}
	if s.orders == nil {
		return ErrSection21CheckoutNeeded
	}
	var obligationID, status, walletRef, parentOrderID, paymentOperationID string
	var required, remaining int64
	if err = s.db.QueryRow(ctx, `INSERT INTO commerce_rfq_payment_obligations(id,rfq_id,binding_id,checkout_quote_id,payment_policy,cancellation_policy,total_minor,required_minor,remaining_minor,currency,status,wallet_idempotency_key) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,CASE WHEN $8=0 THEN 'NOT_REQUIRED' ELSE 'PENDING' END,$11 ON CONFLICT (rfq_id) DO UPDATE SET updated_at=commerce_rfq_payment_obligations.updated_at RETURNING id,status,required_minor,remaining_minor,COALESCE(wallet_operation_ref,''),COALESCE(parent_order_id,''),COALESCE(payment_operation_id,'')`, sid("rfqo_"), rfqID, bindingID, checkoutID, paymentRaw, cancellationRaw, total, policy.DepositMinor, total-policy.DepositMinor, currency, "rfq-deposit-"+rfqID).Scan(&obligationID, &status, &required, &remaining, &walletRef, &parentOrderID, &paymentOperationID); err != nil {
		return err
	}
	if status == "CONVERTED" || status == "ORDER_CREATED" {
		return nil
	}
	if policy.Type == "FULL_PAYMENT" {
		order, operation, startErr := s.orders.StartPayment(ctx, requesterID, checkoutID, fingerprint)
		if startErr != nil {
			return startErr
		}
		_, err = s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status='ORDER_CREATED',parent_order_id=$2,payment_operation_id=$3,updated_at=now(),version=version+1 WHERE id=$1 AND status IN ('PENDING','UNKNOWN','ORDER_CREATED')`, obligationID, order.ID, operation.ID)
		return err
	}
	if required <= 0 {
		return ErrSection21PaymentPolicy
	}
	if status != "HELD" {
		if status == "AUTHORIZING" {
			return ErrSection21Payment
		}
		claim, claimErr := s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status='AUTHORIZING',updated_at=now(),version=version+1 WHERE id=$1 AND status IN ('PENDING','UNKNOWN')`, obligationID)
		if claimErr != nil {
			return claimErr
		}
		if claim.RowsAffected() == 0 {
			return ErrSection21Payment
		}
		if s.payments == nil || !s.payments.Configured() {
			_, _ = s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status='UNKNOWN',updated_at=now(),version=version+1 WHERE id=$1 AND status='AUTHORIZING'`, obligationID)
			return ErrSection21Payment
		}
		result, paymentErr := s.payments.HoldLeg(ctx, HoldLegRequest{
			IdempotencyKey:  "rfq-deposit-" + rfqID,
			SourceService:   PaymentSourceService,
			SourceReference: rfqID,
			CustomerID:      requesterID,
			AmountKobo:      required,
			Currency:        currency,
			ProviderType:    PaymentProviderTypeVendor,
			ProviderID:      vendor,
			Metadata: map[string]any{
				"rfq_id": rfqID, "checkout_quote_id": checkoutID, "payment_policy": paymentRaw,
			},
		})
		if paymentErr != nil {
			_, _ = s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status='UNKNOWN',wallet_operation_ref=COALESCE(NULLIF($2,''),wallet_operation_ref),updated_at=now(),version=version+1 WHERE id=$1`, obligationID, result.Reference)
			return ErrSection21Payment
		}
		if !result.Held() {
			_, _ = s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status='FAILED',wallet_operation_ref=COALESCE(NULLIF($2,''),wallet_operation_ref),updated_at=now(),version=version+1 WHERE id=$1`, obligationID, result.Reference)
			return ErrSection21Payment
		}
		walletRef = result.Reference
		if _, err = s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status='HELD',wallet_operation_ref=$2,updated_at=now(),version=version+1 WHERE id=$1 AND status IN ('PENDING','UNKNOWN','AUTHORIZING')`, obligationID, walletRef); err != nil {
			return err
		}
	}
	order, _, err := s.orders.CreateFromQuote(ctx, requesterID, checkoutID, fingerprint)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status='ORDER_CREATED',parent_order_id=$2,updated_at=now(),version=version+1 WHERE id=$1 AND status='HELD'`, obligationID, order.ID)
	return err
}

type RFQConversionStatus struct {
	RFQID, Status, WalletOperationRef, ParentOrderID, PaymentOperationID string
	RequiredMinor, RemainingMinor, TotalMinor                            int64
	Currency                                                             string
	UpdatedAt                                                            time.Time
}

func (s *Section21Service) GetRFQConversionStatus(ctx context.Context, requesterID, rfqID string) (RFQConversionStatus, error) {
	if err := s.available(); err != nil {
		return RFQConversionStatus{}, err
	}
	var out RFQConversionStatus
	err := s.db.QueryRow(ctx, `SELECT o.rfq_id,o.status,COALESCE(o.wallet_operation_ref,''),COALESCE(o.parent_order_id,''),COALESCE(o.payment_operation_id,''),o.required_minor,o.remaining_minor,o.total_minor,o.currency,o.updated_at FROM commerce_rfq_payment_obligations o JOIN commerce_rfqs r ON r.id=o.rfq_id WHERE o.rfq_id=$1 AND r.requester_id=$2`, rfqID, requesterID).Scan(&out.RFQID, &out.Status, &out.WalletOperationRef, &out.ParentOrderID, &out.PaymentOperationID, &out.RequiredMinor, &out.RemainingMinor, &out.TotalMinor, &out.Currency, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RFQConversionStatus{}, ErrSection21NotFound
	}
	return out, err
}

func (s *Section21Service) completeRFQPaymentConversion(ctx context.Context, rfqID string) error {
	_, err := s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status='CONVERTED',updated_at=now(),version=version+1 WHERE rfq_id=$1 AND status IN ('ORDER_CREATED','CONVERTED')`, rfqID)
	return err
}

func (s *Section21Service) RecoverRFQPayments(ctx context.Context, limit int) (int64, error) {
	if err := s.available(); err != nil {
		return 0, err
	}
	if s.payments == nil || !s.payments.Configured() {
		return 0, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id,COALESCE(wallet_operation_ref,''),updated_at FROM commerce_rfq_payment_obligations WHERE status IN ('AUTHORIZING','UNKNOWN') ORDER BY updated_at,id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var recovered int64
	for rows.Next() {
		var id, reference string
		var updatedAt time.Time
		if err = rows.Scan(&id, &reference, &updatedAt); err != nil {
			return recovered, err
		}
		if reference == "" {
			if updatedAt.Before(s.now().UTC().Add(-2 * time.Minute)) {
				_, _ = s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status='UNKNOWN',updated_at=now(),version=version+1 WHERE id=$1 AND status='AUTHORIZING'`, id)
			}
			continue
		}
		result, readErr := s.payments.GetLeg(ctx, reference)
		if readErr != nil {
			continue
		}
		next := "UNKNOWN"
		if result.Held() {
			next = "HELD"
		} else if result.Failed() {
			next = "FAILED"
		}
		if _, err = s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status=$2,updated_at=now(),version=version+1 WHERE id=$1 AND status IN ('AUTHORIZING','UNKNOWN')`, id, next); err != nil {
			return recovered, err
		}
		recovered++
	}
	return recovered, rows.Err()
}

func (s *Section21Service) CancelRFQ(ctx context.Context, requesterID, rfqID string) error {
	if err := s.available(); err != nil {
		return err
	}
	var status, reservationID, vendorID string
	if err := s.db.QueryRow(ctx, `SELECT r.status,COALESCE(b.capacity_reservation_id,''),COALESCE(b.vendor_id,'') FROM commerce_rfqs r LEFT JOIN commerce_rfq_checkout_bindings b ON b.rfq_id=r.id WHERE r.id=$1 AND r.requester_id=$2`, rfqID, requesterID).Scan(&status, &reservationID, &vendorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSection21NotFound
		}
		return err
	}
	if status == "CANCELLED" {
		return nil
	}
	if status == "CONVERTED" {
		return ErrSection21InvalidState
	}
	var obligationStatus, walletRef, currency, cancellationRaw, parentOrderID string
	var amount int64
	err := s.db.QueryRow(ctx, `SELECT status,COALESCE(wallet_operation_ref,''),currency,required_minor,cancellation_policy::text,COALESCE(parent_order_id,'') FROM commerce_rfq_payment_obligations WHERE rfq_id=$1`, rfqID).Scan(&obligationStatus, &walletRef, &currency, &amount, &cancellationRaw, &parentOrderID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && parentOrderID != "" && walletRef == "" {
		var policy map[string]any
		_ = json.Unmarshal([]byte(cancellationRaw), &policy)
		mode, _ := policy["deposit_refund_mode"].(string)
		if strings.ToUpper(strings.TrimSpace(mode)) != "FULL_BEFORE_CUTOFF" {
			return ErrSection21Conflict
		}
		if cutoff, ok := policy["cutoff_at"].(string); ok && strings.TrimSpace(cutoff) != "" {
			cutoffAt, parseErr := time.Parse(time.RFC3339, cutoff)
			if parseErr != nil || s.now().UTC().After(cutoffAt) {
				return ErrSection21Conflict
			}
		}
		parent, parentErr := s.orders.GetForCustomer(ctx, requesterID, parentOrderID)
		if parentErr != nil {
			return parentErr
		}
		for _, vendorOrder := range parent.VendorOrders {
			if !CancellableByCustomer(vendorOrder.Status) {
				continue
			}
			if _, _, cancelErr := s.orders.CancelByCustomer(ctx, requesterID, vendorOrder.ID, "RFQ_CANCELLED", vendorOrder.Version); cancelErr != nil {
				return cancelErr
			}
		}
	} else if err == nil && walletRef != "" && amount > 0 && obligationStatus != "REFUNDED" {
		var policy map[string]any
		_ = json.Unmarshal([]byte(cancellationRaw), &policy)
		mode, _ := policy["deposit_refund_mode"].(string)
		if strings.ToUpper(strings.TrimSpace(mode)) == "FULL_BEFORE_CUTOFF" {
			if cutoff, ok := policy["cutoff_at"].(string); ok && strings.TrimSpace(cutoff) != "" {
				cutoffAt, parseErr := time.Parse(time.RFC3339, cutoff)
				if parseErr != nil || s.now().UTC().After(cutoffAt) {
					return ErrSection21Conflict
				}
			}
			if s.payments == nil || !s.payments.Configured() {
				return ErrSection21Payment
			}
			result, refundErr := s.payments.RefundLeg(ctx, RefundLegRequest{IdempotencyKey: "rfq-deposit-refund-" + rfqID, PaymentReference: walletRef, SourceService: PaymentSourceService, SourceReference: rfqID, AmountKobo: amount, Currency: currency, Reason: "rfq cancellation"})
			if refundErr != nil || !result.Refunded() {
				return ErrSection21Payment
			}
			if _, err = s.db.Exec(ctx, `UPDATE commerce_rfq_payment_obligations SET status='REFUNDED',updated_at=now(),version=version+1 WHERE rfq_id=$1 AND status<>'REFUNDED'`, rfqID); err != nil {
				return err
			}
		} else if strings.ToUpper(strings.TrimSpace(mode)) != "NON_REFUNDABLE" {
			return ErrSection21Conflict
		}
	}
	if reservationID != "" && s.production != nil {
		var version int64
		if err = s.db.QueryRow(ctx, `SELECT version FROM capacity_reservations WHERE business_id=$1 AND id=$2`, vendorID, reservationID).Scan(&version); err != nil {
			return err
		}
		if _, _, err = s.production.Release(ctx, production.Actor{VendorID: vendorID, CommandRef: "rfq-cancel:" + rfqID}, production.Scope{Global: true}, reservationID, production.ReleaseInput{ExpectedVersion: version, Reason: "rfq cancelled"}); err != nil {
			return err
		}
	}
	tag, err := s.db.Exec(ctx, `UPDATE commerce_rfqs SET status='CANCELLED',version=version+1,updated_at=now() WHERE id=$1 AND requester_id=$2 AND status IN ('DRAFT','OPEN','QUOTED','ACCEPTED')`, rfqID, requesterID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSection21InvalidState
	}
	return nil
}
