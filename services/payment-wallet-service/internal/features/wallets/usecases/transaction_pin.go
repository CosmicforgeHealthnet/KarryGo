package walletusecases

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// TransactionPinStore is the persistence surface for withdrawal PINs. It is a
// narrow interface (rather than the concrete repository) so PIN rules can be
// unit-tested without Postgres; PostgresWalletRepository implements it.
type TransactionPinStore interface {
	GetTransactionPin(ctx context.Context, providerType string, providerID string) (walletmodels.TransactionPin, error)
	CreateTransactionPin(ctx context.Context, providerType string, providerID string, pinHash string) (walletmodels.TransactionPin, error)
	UpdateTransactionPin(ctx context.Context, providerType string, providerID string, pinHash string, failedAttempts int, lockedUntil *time.Time) (walletmodels.TransactionPin, error)
}

const (
	pinMaxFailedAttempts = 5
	pinLockDuration      = 15 * time.Minute
)

func errPinNotSet() error {
	return apperrors.New(http.StatusConflict, "pin_not_set", "Set a transaction PIN before withdrawing.", nil)
}

func errInvalidPin() error {
	return apperrors.New(http.StatusForbidden, "invalid_pin", "Transaction PIN is incorrect.", nil)
}

func errPinLocked() error {
	return apperrors.New(http.StatusLocked, "pin_locked", "Too many incorrect PIN attempts. Try again in a few minutes.", nil)
}

func validatePinFormat(pin string) error {
	if len(pin) < 4 || len(pin) > 6 {
		return apperrors.Validation("Check your PIN.", []apperrors.FieldViolation{
			{Field: "pin", Message: "PIN must be 4 to 6 digits."},
		})
	}
	for _, ch := range pin {
		if ch < '0' || ch > '9' {
			return apperrors.Validation("Check your PIN.", []apperrors.FieldViolation{
				{Field: "pin", Message: "PIN must contain digits only."},
			})
		}
	}
	return nil
}

// TransactionPinStatus reports whether the provider has a PIN set.
func (s *WalletService) TransactionPinStatus(ctx context.Context, providerType string, providerID string) (bool, error) {
	_, err := s.pinStore.GetTransactionPin(ctx, providerType, providerID)
	if err != nil {
		if appErr := apperrors.From(err); appErr != nil && appErr.Code == apperrors.CodeNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SetTransactionPin creates the provider's PIN. Fails with a conflict when a
// PIN already exists (use ChangeTransactionPin instead).
func (s *WalletService) SetTransactionPin(ctx context.Context, providerType string, providerID string, pin string) error {
	if err := validatePinFormat(pin); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return apperrors.Internal("Transaction PIN could not be saved.", err)
	}
	_, err = s.pinStore.CreateTransactionPin(ctx, providerType, providerID, string(hash))
	return err
}

// ChangeTransactionPin replaces the PIN after verifying the current one.
func (s *WalletService) ChangeTransactionPin(ctx context.Context, providerType string, providerID string, currentPin string, newPin string) error {
	if err := validatePinFormat(newPin); err != nil {
		return err
	}
	if err := s.VerifyTransactionPin(ctx, providerType, providerID, currentPin); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPin), bcrypt.DefaultCost)
	if err != nil {
		return apperrors.Internal("Transaction PIN could not be saved.", err)
	}
	_, err = s.pinStore.UpdateTransactionPin(ctx, providerType, providerID, string(hash), 0, nil)
	return err
}

// VerifyTransactionPin checks the PIN and does the lockout bookkeeping:
// while locked every attempt is refused without touching the hash; a wrong
// PIN increments the counter and locks the PIN at the threshold; a correct
// PIN resets the counter.
func (s *WalletService) VerifyTransactionPin(ctx context.Context, providerType string, providerID string, pin string) error {
	stored, err := s.pinStore.GetTransactionPin(ctx, providerType, providerID)
	if err != nil {
		if appErr := apperrors.From(err); appErr != nil && appErr.Code == apperrors.CodeNotFound {
			return errPinNotSet()
		}
		return err
	}

	now := time.Now()
	if stored.LockedUntil != nil && stored.LockedUntil.After(now) {
		return errPinLocked()
	}

	if bcrypt.CompareHashAndPassword([]byte(stored.PinHash), []byte(pin)) != nil {
		failed := stored.FailedAttempts + 1
		var lockedUntil *time.Time
		if failed >= pinMaxFailedAttempts {
			until := now.Add(pinLockDuration)
			lockedUntil = &until
			failed = 0
		}
		if _, updateErr := s.pinStore.UpdateTransactionPin(ctx, providerType, providerID, stored.PinHash, failed, lockedUntil); updateErr != nil {
			return updateErr
		}
		if lockedUntil != nil {
			return errPinLocked()
		}
		return errInvalidPin()
	}

	if stored.FailedAttempts != 0 || stored.LockedUntil != nil {
		if _, updateErr := s.pinStore.UpdateTransactionPin(ctx, providerType, providerID, stored.PinHash, 0, nil); updateErr != nil {
			return updateErr
		}
	}
	return nil
}
