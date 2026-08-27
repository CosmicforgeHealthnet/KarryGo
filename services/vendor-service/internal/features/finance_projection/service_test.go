package finance_projection

import (
	"context"
	"errors"
	"testing"
)

type financeAuth struct{ allowed bool }

func (a financeAuth) Authorize(context.Context, string, string, *string) (bool, error) {
	return a.allowed, nil
}

type financeStepUp struct {
	allowed bool
	calls   int
}

func (s *financeStepUp) ConsumeGrant(context.Context, string, string, string, string, string) error {
	s.calls++
	if !s.allowed {
		return errors.New("denied")
	}
	return nil
}

type financeAuthorityFake struct {
	vendorID    string
	withdrawals int
}

func (f *financeAuthorityFake) Summary(_ context.Context, id string) (Summary, error) {
	f.vendorID = id
	return Summary{OwnerID: id}, nil
}
func (f *financeAuthorityFake) Transactions(context.Context, string, int) ([]Transaction, error) {
	return nil, nil
}
func (f *financeAuthorityFake) BankAccounts(context.Context, string) ([]BankAccount, error) {
	return nil, nil
}
func (f *financeAuthorityFake) RequestWithdrawal(_ context.Context, id string, c WithdrawalCommand) (Withdrawal, error) {
	f.vendorID = id
	f.withdrawals++
	return Withdrawal{Reference: "wd_1", AmountKobo: c.AmountKobo}, nil
}
func (f *financeAuthorityFake) Withdrawals(context.Context, string) ([]Withdrawal, error) {
	return nil, nil
}
func (f *financeAuthorityFake) Withdrawal(_ context.Context, id, reference string) (Withdrawal, error) {
	f.vendorID = id
	return Withdrawal{Reference: reference}, nil
}

func TestFinanceUsesAuthenticatedVendorIdentity(t *testing.T) {
	authority := &financeAuthorityFake{}
	service := NewService(authority, financeAuth{allowed: true}, &financeStepUp{allowed: true})
	actor := Actor{StaffID: "staff", VendorID: "vendor_a", SessionID: "session"}
	if _, err := service.Summary(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	if authority.vendorID != "vendor_a" {
		t.Fatalf("authority vendor=%q", authority.vendorID)
	}
	if _, err := service.Withdrawal(context.Background(), actor, "vendor_b-withdrawal"); err != nil {
		t.Fatal(err)
	}
	if authority.vendorID != "vendor_a" {
		t.Fatal("path reference changed authoritative Vendor scope")
	}
}

func TestWithdrawalRequiresPermissionAndFreshStepUp(t *testing.T) {
	actor := Actor{StaffID: "staff", VendorID: "vendor_a", SessionID: "session"}
	command := WithdrawalCommand{BankAccountID: "bank", AmountKobo: 100, Currency: "NGN", IdempotencyKey: "idem", StepUpGrantID: "grant"}
	authority := &financeAuthorityFake{}
	if _, err := NewService(authority, financeAuth{}, &financeStepUp{allowed: true}).RequestWithdrawal(context.Background(), actor, command); err == nil {
		t.Fatal("missing permission accepted")
	}
	if _, err := NewService(authority, financeAuth{allowed: true}, &financeStepUp{}).RequestWithdrawal(context.Background(), actor, command); err == nil {
		t.Fatal("missing step-up accepted")
	}
	if authority.withdrawals != 0 {
		t.Fatal("Wallet called before authorization completed")
	}
	if _, err := NewService(authority, financeAuth{allowed: true}, &financeStepUp{allowed: true}).RequestWithdrawal(context.Background(), actor, command); err != nil {
		t.Fatal(err)
	}
	if authority.withdrawals != 1 || authority.vendorID != actor.VendorID {
		t.Fatal("withdrawal was not scoped to authenticated Vendor")
	}
}

func TestFinanceRequiresAuthentication(t *testing.T) {
	if _, err := NewService(&financeAuthorityFake{}, financeAuth{allowed: true}, &financeStepUp{allowed: true}).Summary(context.Background(), Actor{}); err == nil {
		t.Fatal("anonymous finance read accepted")
	}
}
