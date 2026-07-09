package savedlocationusecases

import (
	"context"
	"errors"
	"testing"

	savedlocationmodels "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type fakeRepo struct {
	items  map[string]savedlocationmodels.SavedLocation
	nextID int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]savedlocationmodels.SavedLocation{}}
}

func (r *fakeRepo) List(_ context.Context, customerID string) ([]savedlocationmodels.SavedLocation, error) {
	var out []savedlocationmodels.SavedLocation
	for _, l := range r.items {
		if l.CustomerID == customerID {
			out = append(out, l)
		}
	}
	return out, nil
}

func (r *fakeRepo) Create(_ context.Context, location savedlocationmodels.SavedLocation) (savedlocationmodels.SavedLocation, error) {
	r.nextID++
	location.ID = string(rune('a' + r.nextID))
	r.items[location.ID] = location
	return location, nil
}

func (r *fakeRepo) Update(_ context.Context, location savedlocationmodels.SavedLocation) (savedlocationmodels.SavedLocation, error) {
	existing, ok := r.items[location.ID]
	if !ok || existing.CustomerID != location.CustomerID {
		return savedlocationmodels.SavedLocation{}, apperrors.NotFound("Saved location not found.", nil)
	}
	r.items[location.ID] = location
	return location, nil
}

func (r *fakeRepo) Delete(_ context.Context, id, customerID string) error {
	existing, ok := r.items[id]
	if !ok || existing.CustomerID != customerID {
		return apperrors.NotFound("Saved location not found.", nil)
	}
	delete(r.items, id)
	return nil
}

func validInput(customerID string) SaveLocationInput {
	return SaveLocationInput{
		CustomerID:  customerID,
		Title:       "Home",
		Address:     "23 Akeju Victoria Island",
		Latitude:    6.43,
		Longitude:   3.42,
		Description: "Gate 2",
	}
}

func TestAddLocation_TrimsAndStores(t *testing.T) {
	svc := NewSavedLocationService(newFakeRepo())
	input := validInput("cust-1")
	input.Title = "  Home  "
	input.Description = "  Gate 2 "

	loc, err := svc.AddLocation(context.Background(), input)
	if err != nil {
		t.Fatalf("AddLocation: %v", err)
	}
	if loc.Title != "Home" || loc.Description != "Gate 2" {
		t.Fatalf("expected trimmed fields, got %q / %q", loc.Title, loc.Description)
	}
}

func TestAddLocation_ValidationErrors(t *testing.T) {
	svc := NewSavedLocationService(newFakeRepo())

	cases := []struct {
		name   string
		mutate func(*SaveLocationInput)
	}{
		{"missing title", func(i *SaveLocationInput) { i.Title = "  " }},
		{"missing address", func(i *SaveLocationInput) { i.Address = "" }},
		{"null island coords", func(i *SaveLocationInput) { i.Latitude = 0; i.Longitude = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := validInput("cust-1")
			tc.mutate(&input)
			_, err := svc.AddLocation(context.Background(), input)
			var appErr *apperrors.Error
			if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeValidationFailed {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func TestUpdateLocation_OwnershipScoped(t *testing.T) {
	repo := newFakeRepo()
	svc := NewSavedLocationService(repo)

	created, err := svc.AddLocation(context.Background(), validInput("cust-1"))
	if err != nil {
		t.Fatalf("AddLocation: %v", err)
	}

	otherInput := validInput("cust-2")
	if _, err := svc.UpdateLocation(context.Background(), created.ID, otherInput); err == nil {
		t.Fatal("expected not-found updating another customer's location")
	}

	updated := validInput("cust-1")
	updated.Title = "Office"
	loc, err := svc.UpdateLocation(context.Background(), created.ID, updated)
	if err != nil {
		t.Fatalf("UpdateLocation: %v", err)
	}
	if loc.Title != "Office" {
		t.Fatalf("expected updated title, got %q", loc.Title)
	}
}

func TestDeleteLocation_OwnershipScoped(t *testing.T) {
	repo := newFakeRepo()
	svc := NewSavedLocationService(repo)

	created, err := svc.AddLocation(context.Background(), validInput("cust-1"))
	if err != nil {
		t.Fatalf("AddLocation: %v", err)
	}

	if err := svc.DeleteLocation(context.Background(), created.ID, "cust-2"); err == nil {
		t.Fatal("expected not-found deleting another customer's location")
	}
	if err := svc.DeleteLocation(context.Background(), created.ID, "cust-1"); err != nil {
		t.Fatalf("DeleteLocation: %v", err)
	}
	if len(repo.items) != 0 {
		t.Fatal("expected location removed")
	}
}
