package providerapprovalusecases

import (
	"context"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
)

type fakeRepo struct {
	byID     map[string]Driver
	approved []string
}

func (f *fakeRepo) FindByID(_ context.Context, id string) (Driver, error) {
	if d, ok := f.byID[id]; ok {
		return d, nil
	}
	return Driver{}, apperrors.NotFound("Driver could not be found.", nil)
}

func (f *fakeRepo) FindByPhone(_ context.Context, phone string) (Driver, error) {
	for _, d := range f.byID {
		if d.Phone == phone {
			return d, nil
		}
	}
	return Driver{}, apperrors.NotFound("Driver could not be found.", nil)
}

func (f *fakeRepo) Approve(_ context.Context, id string) (Driver, error) {
	f.approved = append(f.approved, id)
	d := f.byID[id]
	d.IdentityStatus, d.DocumentStatus = "verified", "compliant"
	return d, nil
}

func code(err error) apperrors.Code {
	if e := apperrors.From(err); e != nil {
		return e.Code
	}
	return ""
}

func TestApprove(t *testing.T) {
	repo := &fakeRepo{byID: map[string]Driver{
		"ok":        {ID: "ok", Phone: "+2348000003003", AccountStatus: "active", OnboardingStatus: "complete"},
		"suspended": {ID: "suspended", AccountStatus: "suspended", OnboardingStatus: "complete"},
		"new":       {ID: "new", AccountStatus: "active", OnboardingStatus: "profile_required"},
	}}
	svc := NewService(repo)
	ctx := context.Background()

	d, err := svc.Approve(ctx, ApproveInput{Phone: "0800 000 3003"})
	if err != nil || d.IdentityStatus != "verified" {
		t.Fatalf("approve by phone: %+v %v", d, err)
	}
	if _, err := svc.Approve(ctx, ApproveInput{}); code(err) != apperrors.CodeValidationFailed {
		t.Fatalf("no identifier must be a validation error, got %v", err)
	}
	if _, err := svc.Approve(ctx, ApproveInput{ProviderID: "ok", Phone: "0800 000 3003"}); code(err) != apperrors.CodeValidationFailed {
		t.Fatalf("both identifiers must be a validation error, got %v", err)
	}
	if _, err := svc.Approve(ctx, ApproveInput{ProviderID: "suspended"}); code(err) != apperrors.CodeConflict {
		t.Fatalf("a suspension must not be lifted, got %v", err)
	}
	if _, err := svc.Approve(ctx, ApproveInput{ProviderID: "new"}); code(err) != apperrors.CodeConflict {
		t.Fatalf("an unfinished profile must not be approved, got %v", err)
	}
	if _, err := svc.Approve(ctx, ApproveInput{ProviderID: "missing"}); code(err) != apperrors.CodeNotFound {
		t.Fatalf("unknown driver must be not found, got %v", err)
	}
	if len(repo.approved) != 1 || repo.approved[0] != "ok" {
		t.Fatalf("only the eligible driver is approved, got %v", repo.approved)
	}
}
