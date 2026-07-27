package walletusecases

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	walletclients "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/clients"
	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// fakeBankStore is an in-memory BankAccountStore mirroring the repository
// semantics: soft-removal, active-only listing sorted default-first-newest,
// and set-default clearing any previous default.
type fakeBankStore struct {
	accounts map[string]walletmodels.ProviderBankAccount
	seq      int
}

func newFakeBankStore() *fakeBankStore {
	return &fakeBankStore{accounts: map[string]walletmodels.ProviderBankAccount{}}
}

func (f *fakeBankStore) CreateProviderBankAccount(_ context.Context, account walletmodels.ProviderBankAccount) (walletmodels.ProviderBankAccount, error) {
	// Mirror the ON CONFLICT upsert: same provider+bank+number reactivates.
	for id, existing := range f.accounts {
		if existing.ProviderType == account.ProviderType && existing.ProviderID == account.ProviderID &&
			existing.BankCode == account.BankCode && existing.AccountNumber == account.AccountNumber {
			existing.AccountName = account.AccountName
			existing.RecipientCode = account.RecipientCode
			existing.Status = "active"
			existing.UpdatedAt = time.Now()
			f.accounts[id] = existing
			return existing, nil
		}
	}
	f.seq++
	account.ID = fmt.Sprintf("acct-%d", f.seq)
	account.Status = "active"
	account.CreatedAt = time.Now().Add(time.Duration(f.seq) * time.Millisecond)
	account.UpdatedAt = account.CreatedAt
	f.accounts[account.ID] = account
	return account, nil
}

func (f *fakeBankStore) GetProviderBankAccount(_ context.Context, providerType, providerID, accountID string) (walletmodels.ProviderBankAccount, error) {
	account, ok := f.accounts[accountID]
	if !ok || account.ProviderType != providerType || account.ProviderID != providerID || account.Status != "active" {
		return walletmodels.ProviderBankAccount{}, apperrors.NotFound("Bank account could not be found.", nil)
	}
	return account, nil
}

func (f *fakeBankStore) ListProviderBankAccounts(_ context.Context, providerType, providerID string) ([]walletmodels.ProviderBankAccount, error) {
	accounts := []walletmodels.ProviderBankAccount{}
	for _, account := range f.accounts {
		if account.ProviderType == providerType && account.ProviderID == providerID && account.Status == "active" {
			accounts = append(accounts, account)
		}
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].IsDefault != accounts[j].IsDefault {
			return accounts[i].IsDefault
		}
		return accounts[i].CreatedAt.After(accounts[j].CreatedAt)
	})
	return accounts, nil
}

func (f *fakeBankStore) SetDefaultProviderBankAccount(_ context.Context, providerType, providerID, accountID string) (walletmodels.ProviderBankAccount, error) {
	target, ok := f.accounts[accountID]
	if !ok || target.ProviderType != providerType || target.ProviderID != providerID || target.Status != "active" {
		return walletmodels.ProviderBankAccount{}, apperrors.NotFound("Bank account could not be found.", nil)
	}
	for id, account := range f.accounts {
		if account.ProviderType == providerType && account.ProviderID == providerID && account.IsDefault {
			account.IsDefault = false
			f.accounts[id] = account
		}
	}
	target.IsDefault = true
	f.accounts[accountID] = target
	return target, nil
}

func (f *fakeBankStore) RemoveProviderBankAccount(_ context.Context, providerType, providerID, accountID string) error {
	account, ok := f.accounts[accountID]
	if !ok || account.ProviderType != providerType || account.ProviderID != providerID || account.Status != "active" {
		return apperrors.NotFound("Bank account could not be found.", nil)
	}
	account.Status = "removed"
	account.IsDefault = false
	f.accounts[accountID] = account
	return nil
}

// fakePaystack satisfies PaystackClient for the register path.
type fakePaystack struct {
	walletclients.PaystackClient
	recipients int
}

func (f *fakePaystack) ResolveBankAccount(_ context.Context, req walletclients.ResolveBankAccountRequest) (walletclients.ResolveBankAccountResponse, error) {
	return walletclients.ResolveBankAccountResponse{
		AccountNumber: req.AccountNumber,
		AccountName:   "FIREPEMI ADEWALE",
	}, nil
}

func (f *fakePaystack) CreateTransferRecipient(_ context.Context, _ walletclients.CreateTransferRecipientRequest) (walletclients.CreateTransferRecipientResponse, error) {
	f.recipients++
	return walletclients.CreateTransferRecipientResponse{
		RecipientCode: fmt.Sprintf("RCP_%d", f.recipients),
	}, nil
}

func newBankService(store *fakeBankStore) *WalletService {
	return NewWalletService(Options{BankStore: store, Paystack: &fakePaystack{}})
}

func registerAccount(t *testing.T, service *WalletService, number string) walletmodels.ProviderBankAccount {
	t.Helper()
	account, err := service.RegisterBankAccount(context.Background(), RegisterBankAccountInput{
		ProviderType:  "hauling",
		ProviderID:    "provider-1",
		BankCode:      "035",
		BankName:      "Wema Bank",
		AccountNumber: number,
	})
	if err != nil {
		t.Fatalf("register account: %v", err)
	}
	return account
}

func TestRegisterFirstBankAccountBecomesDefault(t *testing.T) {
	service := newBankService(newFakeBankStore())

	first := registerAccount(t, service, "0450908723")
	if !first.IsDefault {
		t.Fatalf("expected first account to be default")
	}

	second := registerAccount(t, service, "0450908724")
	if second.IsDefault {
		t.Fatalf("expected second account to keep the existing default")
	}
}

func TestSetDefaultBankAccountSwitches(t *testing.T) {
	store := newFakeBankStore()
	service := newBankService(store)
	ctx := context.Background()

	first := registerAccount(t, service, "0450908723")
	second := registerAccount(t, service, "0450908724")

	updated, err := service.SetDefaultBankAccount(ctx, "hauling", "provider-1", second.ID)
	if err != nil {
		t.Fatalf("set default: %v", err)
	}
	if !updated.IsDefault {
		t.Fatalf("expected account to be default after set")
	}
	accounts, _ := service.ListBankAccounts(ctx, "hauling", "provider-1")
	if len(accounts) != 2 || accounts[0].ID != second.ID || !accounts[0].IsDefault {
		t.Fatalf("expected new default listed first, got %+v", accounts)
	}
	if accounts[1].ID != first.ID || accounts[1].IsDefault {
		t.Fatalf("expected previous default to be cleared, got %+v", accounts[1])
	}
}

func TestSetDefaultBankAccountNotFound(t *testing.T) {
	service := newBankService(newFakeBankStore())
	_, err := service.SetDefaultBankAccount(context.Background(), "hauling", "provider-1", "missing")
	assertCode(t, err, apperrors.CodeNotFound)
}

func TestDeleteDefaultBankAccountPromotesNewest(t *testing.T) {
	service := newBankService(newFakeBankStore())
	ctx := context.Background()

	first := registerAccount(t, service, "0450908723") // default
	second := registerAccount(t, service, "0450908724")
	third := registerAccount(t, service, "0450908725") // newest

	if err := service.DeleteBankAccount(ctx, "hauling", "provider-1", first.ID); err != nil {
		t.Fatalf("delete default: %v", err)
	}
	accounts, _ := service.ListBankAccounts(ctx, "hauling", "provider-1")
	if len(accounts) != 2 {
		t.Fatalf("expected 2 remaining accounts, got %d", len(accounts))
	}
	if accounts[0].ID != third.ID || !accounts[0].IsDefault {
		t.Fatalf("expected newest remaining account promoted to default, got %+v", accounts[0])
	}
	if accounts[1].ID != second.ID || accounts[1].IsDefault {
		t.Fatalf("expected other account non-default, got %+v", accounts[1])
	}
}

func TestDeleteNonDefaultBankAccountKeepsDefault(t *testing.T) {
	service := newBankService(newFakeBankStore())
	ctx := context.Background()

	first := registerAccount(t, service, "0450908723") // default
	second := registerAccount(t, service, "0450908724")

	if err := service.DeleteBankAccount(ctx, "hauling", "provider-1", second.ID); err != nil {
		t.Fatalf("delete non-default: %v", err)
	}
	accounts, _ := service.ListBankAccounts(ctx, "hauling", "provider-1")
	if len(accounts) != 1 || accounts[0].ID != first.ID || !accounts[0].IsDefault {
		t.Fatalf("expected original default untouched, got %+v", accounts)
	}
}

func TestDeleteBankAccountNotFound(t *testing.T) {
	service := newBankService(newFakeBankStore())
	err := service.DeleteBankAccount(context.Background(), "hauling", "provider-1", "missing")
	assertCode(t, err, apperrors.CodeNotFound)

	// A removed account cannot be deleted twice or used again.
	store := newFakeBankStore()
	service = newBankService(store)
	account := registerAccount(t, service, "0450908723")
	if err := service.DeleteBankAccount(context.Background(), "hauling", "provider-1", account.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	err = service.DeleteBankAccount(context.Background(), "hauling", "provider-1", account.ID)
	assertCode(t, err, apperrors.CodeNotFound)
}

func TestReRegisterRemovedAccountReactivatesAndCanBeDefault(t *testing.T) {
	service := newBankService(newFakeBankStore())
	ctx := context.Background()

	account := registerAccount(t, service, "0450908723")
	if err := service.DeleteBankAccount(ctx, "hauling", "provider-1", account.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	reRegistered := registerAccount(t, service, "0450908723")
	if reRegistered.ID != account.ID {
		t.Fatalf("expected re-registration to reactivate the same row")
	}
	if !reRegistered.IsDefault {
		t.Fatalf("expected the only active account to become default again")
	}
}

func TestOwnershipScoping(t *testing.T) {
	service := newBankService(newFakeBankStore())
	ctx := context.Background()

	account := registerAccount(t, service, "0450908723")

	// Another provider cannot delete or set-default someone else's account.
	err := service.DeleteBankAccount(ctx, "hauling", "provider-2", account.ID)
	assertCode(t, err, apperrors.CodeNotFound)
	_, err = service.SetDefaultBankAccount(ctx, "hauling", "provider-2", account.ID)
	assertCode(t, err, apperrors.CodeNotFound)
}
