package walletusecases

import (
	"context"
	"testing"
	"time"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// fakePinStore is an in-memory TransactionPinStore keyed by type:id.
type fakePinStore struct {
	pins map[string]walletmodels.TransactionPin
}

func newFakePinStore() *fakePinStore {
	return &fakePinStore{pins: map[string]walletmodels.TransactionPin{}}
}

func pinKey(providerType, providerID string) string { return providerType + ":" + providerID }

func (f *fakePinStore) GetTransactionPin(_ context.Context, providerType, providerID string) (walletmodels.TransactionPin, error) {
	pin, ok := f.pins[pinKey(providerType, providerID)]
	if !ok {
		return walletmodels.TransactionPin{}, apperrors.NotFound("Transaction PIN is not set.", nil)
	}
	return pin, nil
}

func (f *fakePinStore) CreateTransactionPin(_ context.Context, providerType, providerID, pinHash string) (walletmodels.TransactionPin, error) {
	key := pinKey(providerType, providerID)
	if _, ok := f.pins[key]; ok {
		return walletmodels.TransactionPin{}, apperrors.Conflict("A transaction PIN is already set.", nil)
	}
	pin := walletmodels.TransactionPin{
		ID:           key,
		ProviderType: providerType,
		ProviderID:   providerID,
		PinHash:      pinHash,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	f.pins[key] = pin
	return pin, nil
}

func (f *fakePinStore) UpdateTransactionPin(_ context.Context, providerType, providerID, pinHash string, failedAttempts int, lockedUntil *time.Time) (walletmodels.TransactionPin, error) {
	key := pinKey(providerType, providerID)
	pin, ok := f.pins[key]
	if !ok {
		return walletmodels.TransactionPin{}, apperrors.NotFound("Transaction PIN is not set.", nil)
	}
	pin.PinHash = pinHash
	pin.FailedAttempts = failedAttempts
	pin.LockedUntil = lockedUntil
	pin.UpdatedAt = time.Now()
	f.pins[key] = pin
	return pin, nil
}

func newPinService(store *fakePinStore) *WalletService {
	return NewWalletService(Options{PinStore: store})
}

func assertCode(t *testing.T, err error, code apperrors.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %q, got nil", code)
	}
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code != code {
		t.Fatalf("expected code %q, got %v", code, err)
	}
}

func TestSetTransactionPin(t *testing.T) {
	store := newFakePinStore()
	service := newPinService(store)
	ctx := context.Background()

	// Format validation.
	for _, bad := range []string{"", "12", "1234567", "12a4", "abcd"} {
		if err := service.SetTransactionPin(ctx, "hauling", "p1", bad); err == nil {
			t.Fatalf("expected validation error for pin %q", bad)
		}
	}

	if err := service.SetTransactionPin(ctx, "hauling", "p1", "1234"); err != nil {
		t.Fatalf("set pin: %v", err)
	}
	set, err := service.TransactionPinStatus(ctx, "hauling", "p1")
	if err != nil || !set {
		t.Fatalf("expected pin_set=true, got %v %v", set, err)
	}

	// Setting again conflicts.
	assertCode(t, service.SetTransactionPin(ctx, "hauling", "p1", "5678"), apperrors.Code("conflict"))
}

func TestVerifyTransactionPinAndLockout(t *testing.T) {
	store := newFakePinStore()
	service := newPinService(store)
	ctx := context.Background()

	// Not set yet.
	assertCode(t, service.VerifyTransactionPin(ctx, "hauling", "p1", "1234"), apperrors.Code("pin_not_set"))

	if err := service.SetTransactionPin(ctx, "hauling", "p1", "1234"); err != nil {
		t.Fatalf("set pin: %v", err)
	}

	// Correct PIN verifies.
	if err := service.VerifyTransactionPin(ctx, "hauling", "p1", "1234"); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Four wrong attempts -> invalid_pin each time.
	for i := 0; i < pinMaxFailedAttempts-1; i++ {
		assertCode(t, service.VerifyTransactionPin(ctx, "hauling", "p1", "0000"), apperrors.Code("invalid_pin"))
	}
	// Fifth wrong attempt locks.
	assertCode(t, service.VerifyTransactionPin(ctx, "hauling", "p1", "0000"), apperrors.Code("pin_locked"))
	// Even the correct PIN is refused while locked.
	assertCode(t, service.VerifyTransactionPin(ctx, "hauling", "p1", "1234"), apperrors.Code("pin_locked"))

	// Expire the lock manually; correct PIN works again and resets state.
	pin := store.pins[pinKey("hauling", "p1")]
	past := time.Now().Add(-time.Minute)
	pin.LockedUntil = &past
	store.pins[pinKey("hauling", "p1")] = pin
	if err := service.VerifyTransactionPin(ctx, "hauling", "p1", "1234"); err != nil {
		t.Fatalf("verify after lock expiry: %v", err)
	}
	if got := store.pins[pinKey("hauling", "p1")]; got.FailedAttempts != 0 || got.LockedUntil != nil {
		t.Fatalf("expected reset state, got attempts=%d locked=%v", got.FailedAttempts, got.LockedUntil)
	}
}

func TestChangeTransactionPin(t *testing.T) {
	store := newFakePinStore()
	service := newPinService(store)
	ctx := context.Background()

	if err := service.SetTransactionPin(ctx, "hauling", "p1", "1234"); err != nil {
		t.Fatalf("set pin: %v", err)
	}

	// Wrong current PIN refused.
	assertCode(t, service.ChangeTransactionPin(ctx, "hauling", "p1", "0000", "5678"), apperrors.Code("invalid_pin"))

	// Correct current PIN changes it.
	if err := service.ChangeTransactionPin(ctx, "hauling", "p1", "1234", "5678"); err != nil {
		t.Fatalf("change pin: %v", err)
	}
	if err := service.VerifyTransactionPin(ctx, "hauling", "p1", "5678"); err != nil {
		t.Fatalf("verify new pin: %v", err)
	}
	assertCode(t, service.VerifyTransactionPin(ctx, "hauling", "p1", "1234"), apperrors.Code("invalid_pin"))
}

func TestWithdrawalRequiresPin(t *testing.T) {
	service := NewWalletService(Options{PinStore: newFakePinStore(), WithdrawalMinKobo: 100})

	_, err := service.RequestWithdrawal(context.Background(), RequestWithdrawalInput{
		ProviderType:   "hauling",
		ProviderID:     "p1",
		BankAccountID:  "acct-1",
		AmountKobo:     100000,
		Currency:       "NGN",
		IdempotencyKey: "idem-1",
	})
	assertCode(t, err, apperrors.CodeValidationFailed)

	// With a PIN supplied but none set, the pin_not_set error surfaces before
	// any repository access (repository is nil here, so reaching it would panic).
	_, err = service.RequestWithdrawal(context.Background(), RequestWithdrawalInput{
		ProviderType:   "hauling",
		ProviderID:     "p1",
		BankAccountID:  "acct-1",
		AmountKobo:     100000,
		Currency:       "NGN",
		IdempotencyKey: "idem-1",
		Pin:            "1234",
	})
	assertCode(t, err, apperrors.Code("pin_not_set"))
}
