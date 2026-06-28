package providerprofileusecases

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	providerauthmodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/models"
	providerprofilemodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/models"
	providerprofilerepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

// fakeProfileRepo is a minimal ProfileRepository for usecase tests. UpdateProfile
// returns whatever updateErr/updateProvider are set to; everything else is a
// no-op (UpdateProfile is the only method ProfileService.UpdateProfile calls).
type fakeProfileRepo struct {
	updateErr       error
	updateProvider  providerauthmodels.Provider
	updateCalled    bool
	lastUpdatePhone string

	contactEmailTaken bool
	contactPhoneTaken bool
	lastContactPhone  string
}

func (f *fakeProfileRepo) UpdateProfile(ctx context.Context, params providerprofilerepositories.UpdateProfileParams) (providerauthmodels.Provider, error) {
	f.updateCalled = true
	f.lastUpdatePhone = params.Phone
	if f.updateErr != nil {
		return providerauthmodels.Provider{}, f.updateErr
	}
	return f.updateProvider, nil
}

func (f *fakeProfileRepo) UpdatePhoto(ctx context.Context, id, assetID, photoURL string) (providerauthmodels.Provider, error) {
	return providerauthmodels.Provider{}, nil
}

func (f *fakeProfileRepo) UpsertByPhone(ctx context.Context, phone string) (providerauthmodels.Provider, error) {
	return providerauthmodels.Provider{}, nil
}

func (f *fakeProfileRepo) UpsertByEmail(ctx context.Context, email string) (providerauthmodels.Provider, error) {
	return providerauthmodels.Provider{}, nil
}

func (f *fakeProfileRepo) GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error) {
	return providerauthmodels.Provider{}, nil
}

func (f *fakeProfileRepo) UpdatePhone(ctx context.Context, id, phone string) (providerauthmodels.Provider, error) {
	return providerauthmodels.Provider{}, nil
}

func (f *fakeProfileRepo) ContactTaken(ctx context.Context, excludeID, email, phone string) (bool, bool, error) {
	f.lastContactPhone = phone
	return f.contactEmailTaken, f.contactPhoneTaken, nil
}

func newService(repo providerprofilerepositories.ProfileRepository) *ProfileService {
	// trucks repo is not exercised by UpdateProfile.
	return NewProfileService(repo, nil)
}

func TestUpdateProfile_DuplicateEmailReturnsValidation(t *testing.T) {
	repo := &fakeProfileRepo{updateErr: &pgconn.PgError{Code: "23505", ConstraintName: "truck_providers_email_key"}}
	_, err := newService(repo).UpdateProfile(context.Background(), UpdateProfileInput{
		ProviderID: "p1", FirstName: "Ada", Email: "dup@example.com",
	})

	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperrors.Error, got %v", err)
	}
	if appErr.Code != apperrors.CodeValidationFailed {
		t.Fatalf("expected validation_failed, got %s", appErr.Code)
	}
	if len(appErr.Fields) != 1 || appErr.Fields[0].Field != "email" {
		t.Fatalf("expected an email field violation, got %+v", appErr.Fields)
	}
}

func TestUpdateProfile_DuplicatePhoneReturnsValidation(t *testing.T) {
	repo := &fakeProfileRepo{updateErr: &pgconn.PgError{Code: "23505", ConstraintName: "truck_providers_phone_key"}}
	_, err := newService(repo).UpdateProfile(context.Background(), UpdateProfileInput{
		ProviderID: "p1", FirstName: "Ada", Phone: "08023456789",
	})

	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperrors.Error, got %v", err)
	}
	if appErr.Code != apperrors.CodeValidationFailed {
		t.Fatalf("expected validation_failed, got %s", appErr.Code)
	}
	if len(appErr.Fields) != 1 || appErr.Fields[0].Field != "phone" {
		t.Fatalf("expected a phone field violation, got %+v", appErr.Fields)
	}
}

func TestUpdateProfile_OtherErrorPassesThrough(t *testing.T) {
	sentinel := errors.New("connection reset")
	repo := &fakeProfileRepo{updateErr: sentinel}
	_, err := newService(repo).UpdateProfile(context.Background(), UpdateProfileInput{
		ProviderID: "p1", FirstName: "Ada",
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the original error to pass through, got %v", err)
	}
}

func TestUpdateProfile_InvalidPhoneRejectedBeforeRepo(t *testing.T) {
	repo := &fakeProfileRepo{}
	_, err := newService(repo).UpdateProfile(context.Background(), UpdateProfileInput{
		ProviderID: "p1", FirstName: "Ada", Phone: "123",
	})
	if err == nil {
		t.Fatal("expected an error for an invalid phone number")
	}
	if repo.updateCalled {
		t.Fatal("repo.UpdateProfile should not be called when the phone is invalid")
	}
}

func TestUpdateProfile_NormalizesPhoneBeforePersisting(t *testing.T) {
	repo := &fakeProfileRepo{}
	_, err := newService(repo).UpdateProfile(context.Background(), UpdateProfileInput{
		ProviderID: "p1", FirstName: "Ada", Phone: "08023456789",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastUpdatePhone != "+2348023456789" {
		t.Fatalf("expected normalized phone +2348023456789, got %q", repo.lastUpdatePhone)
	}
}

func TestCheckContactAvailability_EmailTaken(t *testing.T) {
	repo := &fakeProfileRepo{contactEmailTaken: true}
	emailTaken, phoneTaken, err := newService(repo).CheckContactAvailability(
		context.Background(), "p1", "dup@example.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !emailTaken || phoneTaken {
		t.Fatalf("expected (emailTaken=true, phoneTaken=false), got (%v, %v)", emailTaken, phoneTaken)
	}
}

func TestCheckContactAvailability_PhoneTakenAndNormalized(t *testing.T) {
	repo := &fakeProfileRepo{contactPhoneTaken: true}
	emailTaken, phoneTaken, err := newService(repo).CheckContactAvailability(
		context.Background(), "p1", "", "08023456789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if emailTaken || !phoneTaken {
		t.Fatalf("expected (emailTaken=false, phoneTaken=true), got (%v, %v)", emailTaken, phoneTaken)
	}
	if repo.lastContactPhone != "+2348023456789" {
		t.Fatalf("expected normalized phone passed to repo, got %q", repo.lastContactPhone)
	}
}

func TestCheckContactAvailability_InvalidPhoneRejected(t *testing.T) {
	repo := &fakeProfileRepo{}
	_, _, err := newService(repo).CheckContactAvailability(
		context.Background(), "p1", "", "123")
	if err == nil {
		t.Fatal("expected an error for an invalid phone number")
	}
}
