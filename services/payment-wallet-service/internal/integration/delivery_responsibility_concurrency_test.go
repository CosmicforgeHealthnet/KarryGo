package integration_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

func TestDeliveryResponsibilityTransitionConcurrentRetryIsExactlyOnce(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repository := walletrepositories.NewPostgresWalletRepository(pool)

	suffix := uuid.NewString()
	paymentID := uuid.New()
	vendorOrderID := "vo_fallback_" + suffix
	operationRef := "fallback_" + suffix
	if _, err := pool.Exec(ctx, `INSERT INTO payment_intents
			(id,reference,source_service,source_reference,customer_id,amount_kobo,platform_fee_kobo,currency,payment_method,status)
		VALUES($1,$2,'vendor-service',$3,$4,1000,0,'NGN','wallet','held')`, paymentID, "pi_"+suffix, "parent_"+suffix, "customer_"+suffix); err != nil {
		t.Fatalf("seed payment intent: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO commerce_payment_intents
			(payment_intent_id,parent_commerce_reference,quote_reference,quote_version,quote_fingerprint,final_authorized_minor,max_authorized_minor,allocation_fingerprint,status,idempotency_key,request_fingerprint)
		VALUES($1,$2,$3,1,$4,1000,1000,$5,'HELD',$6,$7)`, paymentID, "parent_"+suffix, "quote_"+suffix,
		"quote-fingerprint-"+suffix, "allocation-fingerprint-"+suffix, "intent-key-"+suffix, "intent-fingerprint-"+suffix); err != nil {
		t.Fatalf("seed commerce intent: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO commerce_payment_allocations
			(id,payment_intent_id,allocation_reference,scope,vendor_order_id,vendor_id,branch_id,delivery_responsibility,
			 effective_delivery_responsibility,effective_vendor_delivery_payable_minor,effective_provider_delivery_payable_minor,
			 currency,customer_payable_minor,max_authorized_minor,delivery_customer_minor,vendor_delivery_payable_minor,
			 provider_delivery_payable_minor,snapshot,snapshot_fingerprint,status)
		VALUES($1,$2,$3,'VENDOR',$4,$5,$6,'VENDOR','VENDOR',1000,0,'NGN',1000,1000,1000,1000,0,'{}',$7,'HELD')`,
		"allocation_"+suffix, paymentID, "vendor-allocation-"+suffix, vendorOrderID,
		"vendor_"+suffix, "branch_"+suffix, "snapshot-"+suffix); err != nil {
		t.Fatalf("seed funded Vendor allocation: %v", err)
	}

	input := walletrepositories.TransitionDeliveryResponsibilityInput{
		VendorOrderID: vendorOrderID, OperationRef: operationRef,
		IdempotencyKey: operationRef, ReasonCode: "CAPACITY_OVERFLOW",
		PlatformExecutionCostMinor: 1_250,
	}
	results := make([]string, 2)
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			result, err := repository.TransitionDeliveryResponsibility(ctx, input)
			results[index], errs[index] = result.ID, err
		}(index)
	}
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("same-key transition retry: %v", err)
		}
	}
	if results[0] == "" || results[0] != results[1] {
		t.Fatalf("retry results differ: %q and %q", results[0], results[1])
	}

	var transitions int
	var originalVendor, originalProvider, effectiveVendor, effectiveProvider, fallbackFunding int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM commerce_delivery_responsibility_transitions WHERE operation_ref=$1`, operationRef).Scan(&transitions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT vendor_delivery_payable_minor,provider_delivery_payable_minor,
		effective_vendor_delivery_payable_minor,effective_provider_delivery_payable_minor,fallback_platform_funding_minor
		FROM commerce_payment_allocations WHERE vendor_order_id=$1`, vendorOrderID).
		Scan(&originalVendor, &originalProvider, &effectiveVendor, &effectiveProvider, &fallbackFunding); err != nil {
		t.Fatal(err)
	}
	if transitions != 1 || originalVendor != 1_000 || originalProvider != 0 || effectiveVendor != 0 || effectiveProvider != 1_250 || fallbackFunding != 250 {
		t.Fatalf("transition invariant rows=%d original=%d/%d effective=%d/%d funding=%d", transitions, originalVendor, originalProvider, effectiveVendor, effectiveProvider, fallbackFunding)
	}
}
