package providerprofileusecases

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	providerprofilemodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/models"
	providerprofilerepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ─── fakes ────────────────────────────────────────────────────────────────────

// fakeProfiles models the preserve-on-empty UPDATE in Go so the usecase's
// trimming and normalization can be asserted without a database. The SQL itself
// is covered by the live checks; what matters here is which values reach it.
type fakeProfiles struct {
	stored    providerauthmodels.Provider
	lastCall  providerprofilerepositories.UpdateProfileParams
	updateErr error
}

func (f *fakeProfiles) UpsertByPhone(_ context.Context, phone string, _ providerauthmodels.Signup) (providerauthmodels.Provider, error) {
	f.stored.Phone = phone
	return f.stored, nil
}

func (f *fakeProfiles) GetByID(context.Context, string) (providerauthmodels.Provider, error) {
	return f.stored, nil
}

func (f *fakeProfiles) UpdatePhone(_ context.Context, _, phone string) (providerauthmodels.Provider, error) {
	f.stored.Phone = phone
	return f.stored, nil
}

func (f *fakeProfiles) ContactTaken(context.Context, string, string, string) (bool, bool, error) {
	return false, false, nil
}

func (f *fakeProfiles) UpdateProfile(_ context.Context, p providerprofilerepositories.UpdateProfileParams) (providerauthmodels.Provider, error) {
	f.lastCall = p
	if f.updateErr != nil {
		return providerauthmodels.Provider{}, f.updateErr
	}
	// Mirror the SQL: assign first_name outright, preserve every other field
	// when the incoming value is empty.
	f.stored.FirstName = p.FirstName
	if p.LastName != "" {
		f.stored.LastName = p.LastName
	}
	if p.LocationCity != "" {
		f.stored.LocationCity = p.LocationCity
	}
	if p.GovIDURL != "" {
		f.stored.GovIDURL = p.GovIDURL
	}
	if p.GuarantorPhone != "" {
		f.stored.GuarantorPhone = p.GuarantorPhone
	}
	if f.stored.OnboardingStatus == providerauthmodels.OnboardingProfileNeeded {
		f.stored.OnboardingStatus = providerauthmodels.OnboardingComplete
	}
	return f.stored, nil
}

func (f *fakeProfiles) UpdatePhoto(_ context.Context, _, assetID, url string) (providerauthmodels.Provider, error) {
	f.stored.PhotoAssetID = &assetID
	f.stored.ProfilePhotoURL = &url
	return f.stored, nil
}

func (f *fakeProfiles) SetAcceptsParcels(_ context.Context, _ string, accepts bool) (providerauthmodels.Provider, error) {
	f.stored.AcceptsParcels = accepts
	return f.stored, nil
}

func (f *fakeProfiles) SetVehicleOperationalStatus(_ context.Context, _, status string) error {
	f.stored.VehicleOperationalStatus = status
	return nil
}

type fakeVehicles struct {
	byID      map[string]providerprofilemodels.Vehicle
	byPlate   map[string]providerprofilemodels.Vehicle
	createErr error
}

func newFakeVehicles() *fakeVehicles {
	return &fakeVehicles{
		byID:    map[string]providerprofilemodels.Vehicle{},
		byPlate: map[string]providerprofilemodels.Vehicle{},
	}
}

func (f *fakeVehicles) Create(_ context.Context, v providerprofilemodels.Vehicle) (providerprofilemodels.Vehicle, error) {
	if f.createErr != nil {
		return providerprofilemodels.Vehicle{}, f.createErr
	}
	if _, clash := f.byPlate[v.PlateNumber]; clash {
		return providerprofilemodels.Vehicle{}, &pgconn.PgError{Code: "23505", ConstraintName: "vehicles_plate_number_key"}
	}
	v.Status = providerprofilemodels.VehicleStatusActive
	f.byID[v.ID] = v
	f.byPlate[v.PlateNumber] = v
	return v, nil
}

func (f *fakeVehicles) ListByProvider(_ context.Context, providerID string) ([]providerprofilemodels.Vehicle, error) {
	var out []providerprofilemodels.Vehicle
	for _, v := range f.byID {
		if v.ProviderID == providerID {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeVehicles) GetByID(_ context.Context, id, providerID string) (providerprofilemodels.Vehicle, error) {
	v, ok := f.byID[id]
	if !ok || v.ProviderID != providerID {
		return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", nil)
	}
	return v, nil
}

func (f *fakeVehicles) GetByIDAnywhere(_ context.Context, id string) (providerprofilemodels.Vehicle, error) {
	v, ok := f.byID[id]
	if !ok {
		return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", nil)
	}
	return v, nil
}

func (f *fakeVehicles) GetByPlate(_ context.Context, plate string) (providerprofilemodels.Vehicle, error) {
	v, ok := f.byPlate[plate]
	if !ok {
		return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", nil)
	}
	return v, nil
}

func (f *fakeVehicles) Update(_ context.Context, v providerprofilemodels.Vehicle) (providerprofilemodels.Vehicle, error) {
	existing, ok := f.byID[v.ID]
	if !ok || existing.ProviderID != v.ProviderID {
		return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", nil)
	}
	f.byID[v.ID] = v
	f.byPlate[v.PlateNumber] = v
	return v, nil
}

func (f *fakeVehicles) CountActiveByProvider(_ context.Context, providerID string) (int, error) {
	n := 0
	for _, v := range f.byID {
		if v.ProviderID == providerID && v.Status == providerprofilemodels.VehicleStatusActive {
			n++
		}
	}
	return n, nil
}

func (f *fakeVehicles) FirstActiveByProvider(_ context.Context, providerID string) (providerprofilemodels.Vehicle, error) {
	for _, v := range f.byID {
		if v.ProviderID == providerID && v.Status == providerprofilemodels.VehicleStatusActive {
			return v, nil
		}
	}
	return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", nil)
}

func newTestService() (*ProfileService, *fakeProfiles, *fakeVehicles) {
	profiles := &fakeProfiles{stored: providerauthmodels.Provider{
		ID:               "driver-1",
		OnboardingStatus: providerauthmodels.OnboardingProfileNeeded,
	}}
	vehicles := newFakeVehicles()
	return NewProfileService(profiles, vehicles), profiles, vehicles
}

// ─── profile tests ────────────────────────────────────────────────────────────

func TestUpdateProfileRequiresFirstName(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.UpdateProfile(context.Background(), UpdateProfileInput{ProviderID: "driver-1", LastName: "Adeyemi"})
	if err == nil {
		t.Fatal("expected a validation error when first_name is missing")
	}
	if !strings.Contains(err.Error(), "check your details") && !strings.Contains(err.Error(), "Check your details") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Regression: an earlier version assigned last_name outright, mirroring hauling.
// Because only first_name is required, a documents screen sending just a first
// name silently blanked the driver's surname.
func TestUpdateProfilePreservesLastNameOnPartialSubmit(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	if _, err := svc.UpdateProfile(ctx, UpdateProfileInput{
		ProviderID: "driver-1", FirstName: "Kayode", LastName: "Adeyemi", LocationCity: "Ikeja",
	}); err != nil {
		t.Fatalf("first update: %v", err)
	}

	// A later screen submits only the required field plus its own data.
	after, err := svc.UpdateProfile(ctx, UpdateProfileInput{
		ProviderID: "driver-1", FirstName: "Kayode", GovIDURL: "https://media/govid.jpg",
	})
	if err != nil {
		t.Fatalf("second update: %v", err)
	}

	if after.LastName != "Adeyemi" {
		t.Fatalf("last_name must survive a partial submit, got %q", after.LastName)
	}
	if after.LocationCity != "Ikeja" {
		t.Fatalf("location_city must survive a partial submit, got %q", after.LocationCity)
	}
	if after.GovIDURL != "https://media/govid.jpg" {
		t.Fatalf("the new field should be written, got %q", after.GovIDURL)
	}
}

func TestUpdateProfileAdvancesOnboardingOnFirstUpdate(t *testing.T) {
	svc, _, _ := newTestService()
	out, err := svc.UpdateProfile(context.Background(), UpdateProfileInput{ProviderID: "driver-1", FirstName: "Kayode"})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if out.OnboardingStatus != providerauthmodels.OnboardingComplete {
		t.Fatalf("onboarding should complete on first profile update, got %q", out.OnboardingStatus)
	}
}

func TestUpdateProfileNormalizesLoginPhone(t *testing.T) {
	svc, profiles, _ := newTestService()
	if _, err := svc.UpdateProfile(context.Background(), UpdateProfileInput{
		ProviderID: "driver-1", FirstName: "Kayode", Phone: "08011112222",
	}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if profiles.lastCall.Phone != "+2348011112222" {
		t.Fatalf("login phone must reach the repo normalized, got %q", profiles.lastCall.Phone)
	}
}

func TestUpdateProfileDoesNotNormalizeNextOfKinPhones(t *testing.T) {
	// Guarantor and emergency contacts may legitimately be non-Nigerian numbers.
	// Normalizing them would reject valid input and block onboarding, so they are
	// only trimmed.
	svc, profiles, _ := newTestService()
	if _, err := svc.UpdateProfile(context.Background(), UpdateProfileInput{
		ProviderID: "driver-1", FirstName: "Kayode",
		GuarantorPhone:        "  +1 415 555 0100  ",
		EmergencyContactPhone: " 0700 000 0000 ",
	}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if profiles.lastCall.GuarantorPhone != "+1 415 555 0100" {
		t.Fatalf("guarantor phone should be trimmed but not normalized, got %q", profiles.lastCall.GuarantorPhone)
	}
	if profiles.lastCall.EmergencyContactPhone != "0700 000 0000" {
		t.Fatalf("emergency phone should be trimmed but not normalized, got %q", profiles.lastCall.EmergencyContactPhone)
	}
}

func TestUpdateProfileMapsUniqueViolationToFieldError(t *testing.T) {
	svc, profiles, _ := newTestService()
	profiles.updateErr = &pgconn.PgError{Code: "23505", ConstraintName: "taxi_providers_phone_key"}

	_, err := svc.UpdateProfile(context.Background(), UpdateProfileInput{ProviderID: "driver-1", FirstName: "Kayode"})
	if err == nil {
		t.Fatal("a duplicate phone should surface as a validation error, not a 500")
	}
	if !strings.Contains(err.Error(), "phone number is already in use") {
		t.Fatalf("expected a phone-specific message, got %v", err)
	}
}

// ─── vehicle tests ────────────────────────────────────────────────────────────

// Regression guard for the sync helper wired into CreateVehicle/UpdateVehicle:
// registering the first active vehicle must flip the provider's
// vehicle_operational_status, and deactivating the only vehicle must flip it
// back -- even though neither value is ever read by the go-online gate
// itself (that check is always a live count, per CanGoAvailable's comment).
func TestCreateVehicleSyncsProviderVehicleOperationalStatus(t *testing.T) {
	svc, profiles, _ := newTestService()
	ctx := context.Background()

	if profiles.stored.VehicleOperationalStatus == providerauthmodels.VehicleOperationalActive {
		t.Fatal("no vehicle registered yet -- status should not already be active")
	}

	v, err := svc.CreateVehicle(ctx, CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "economy", PlateNumber: "LAG-500-ZZ",
	})
	if err != nil {
		t.Fatalf("CreateVehicle: %v", err)
	}
	if profiles.stored.VehicleOperationalStatus != providerauthmodels.VehicleOperationalActive {
		t.Fatalf("expected vehicle_operational_status to flip to active, got %q", profiles.stored.VehicleOperationalStatus)
	}

	if _, err := svc.UpdateVehicle(ctx, UpdateVehicleInput{
		ID: v.ID, ProviderID: "driver-1", VehicleClass: "economy",
		PlateNumber: "LAG-500-ZZ", Status: "inactive",
	}); err != nil {
		t.Fatalf("UpdateVehicle: %v", err)
	}
	if profiles.stored.VehicleOperationalStatus != providerauthmodels.VehicleOperationalNone {
		t.Fatalf("expected vehicle_operational_status to flip back to none once the only vehicle is inactive, got %q", profiles.stored.VehicleOperationalStatus)
	}
}

func TestCreateVehicleDefaultsSeatsFromClass(t *testing.T) {
	svc, _, _ := newTestService()
	v, err := svc.CreateVehicle(context.Background(), CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "xl", PlateNumber: "LAG-442-XY",
	})
	if err != nil {
		t.Fatalf("CreateVehicle: %v", err)
	}
	// The matcher compares a booking's passenger count against seats, so an
	// unstated count must not persist as zero.
	if v.Seats != 6 {
		t.Fatalf("XL should default to 6 seats, got %d", v.Seats)
	}
}

func TestCreateVehicleNormalizesClassAndPlate(t *testing.T) {
	svc, _, _ := newTestService()
	v, err := svc.CreateVehicle(context.Background(), CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "  XL  ", PlateNumber: " lag-442-xy ",
	})
	if err != nil {
		t.Fatalf("CreateVehicle: %v", err)
	}
	// The matcher compares stored class against a booking's requested class by
	// exact string equality, so both sides must be normalized on write.
	if v.VehicleClass != "xl" {
		t.Fatalf("class should be normalized to %q, got %q", "xl", v.VehicleClass)
	}
	if v.PlateNumber != "LAG-442-XY" {
		t.Fatalf("plate should be trimmed and upper-cased, got %q", v.PlateNumber)
	}
}

func TestCreateVehicleRejectsUnknownClass(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.CreateVehicle(context.Background(), CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "helicopter", PlateNumber: "ABC-123",
	})
	if err == nil {
		t.Fatal("an unknown vehicle class must be rejected")
	}
	// The field violation should name the accepted values so the app can show
	// them; the top-level message stays generic.
	appErr := apperrors.From(err)
	var classMsg string
	for _, f := range appErr.Fields {
		if f.Field == "vehicle_class" {
			classMsg = f.Message
		}
	}
	if classMsg == "" {
		t.Fatalf("expected a vehicle_class field violation, got %+v", appErr.Fields)
	}
	for _, want := range []string{"economy", "comfort", "xl"} {
		if !strings.Contains(classMsg, want) {
			t.Fatalf("field violation should list accepted classes, got %q", classMsg)
		}
	}
}

func TestCreateVehicleRequiresPlate(t *testing.T) {
	svc, _, _ := newTestService()
	if _, err := svc.CreateVehicle(context.Background(), CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "economy", PlateNumber: "   ",
	}); err == nil {
		t.Fatal("a blank plate must be rejected")
	}
}

func TestCreateVehicleIsIdempotentForSameDriver(t *testing.T) {
	// Onboarding retries re-submit the same vehicle. That must not 500 or create
	// a duplicate — it should return the vehicle that already exists.
	svc, _, _ := newTestService()
	ctx := context.Background()

	first, err := svc.CreateVehicle(ctx, CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "economy", PlateNumber: "LAG-100-AA",
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := svc.CreateVehicle(ctx, CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "economy", PlateNumber: "lag-100-aa",
	})
	if err != nil {
		t.Fatalf("re-submitting the same plate should succeed, got %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the existing vehicle back, got a different id (%s vs %s)", first.ID, second.ID)
	}
}

func TestCreateVehicleRejectsPlateOwnedByAnotherDriver(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	if _, err := svc.CreateVehicle(ctx, CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "economy", PlateNumber: "LAG-100-AA",
	}); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := svc.CreateVehicle(ctx, CreateVehicleInput{
		ProviderID: "driver-2", VehicleClass: "economy", PlateNumber: "LAG-100-AA",
	})
	if err == nil {
		t.Fatal("another driver claiming the same plate must be rejected")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("expected a friendly validation error, got %v", err)
	}
}

func TestGetVehicleIsScopedToOwner(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	v, _ := svc.CreateVehicle(ctx, CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "economy", PlateNumber: "LAG-100-AA",
	})
	if _, err := svc.GetVehicle(ctx, v.ID, "driver-2"); err == nil {
		t.Fatal("a driver must not read another driver's vehicle")
	}
	if _, err := svc.GetVehicle(ctx, v.ID, "driver-1"); err != nil {
		t.Fatalf("the owner should read their own vehicle: %v", err)
	}
}

func TestGetPublicVehicleIgnoresOwnership(t *testing.T) {
	// The passenger looking at the car coming to collect them does not own it.
	svc, _, _ := newTestService()
	ctx := context.Background()

	v, _ := svc.CreateVehicle(ctx, CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "economy", PlateNumber: "LAG-100-AA",
	})
	got, err := svc.GetPublicVehicle(ctx, v.ID)
	if err != nil {
		t.Fatalf("public lookup should succeed regardless of owner: %v", err)
	}
	if got.PlateNumber != "LAG-100-AA" {
		t.Fatalf("unexpected vehicle: %+v", got)
	}
}

func TestUpdateVehicleValidatesStatus(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	v, _ := svc.CreateVehicle(ctx, CreateVehicleInput{
		ProviderID: "driver-1", VehicleClass: "economy", PlateNumber: "LAG-100-AA",
	})
	if _, err := svc.UpdateVehicle(ctx, UpdateVehicleInput{
		ID: v.ID, ProviderID: "driver-1", VehicleClass: "economy",
		PlateNumber: "LAG-100-AA", Status: "retired",
	}); err == nil {
		t.Fatal("an unknown status must be rejected")
	}

	updated, err := svc.UpdateVehicle(ctx, UpdateVehicleInput{
		ID: v.ID, ProviderID: "driver-1", VehicleClass: "economy",
		PlateNumber: "LAG-100-AA", Status: "inactive",
	})
	if err != nil {
		t.Fatalf("UpdateVehicle: %v", err)
	}
	if updated.Status != "inactive" {
		t.Fatalf("status should be inactive, got %q", updated.Status)
	}
}
