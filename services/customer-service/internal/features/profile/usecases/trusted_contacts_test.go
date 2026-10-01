package profileusecases

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	profilerepositories "cosmicforge/logistics/services/customer-service/internal/features/profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

type fakeContacts struct {
	rows map[string]profilemodels.TrustedContact
	keys map[string]string
	fps  map[string]string
}

func newFakeContacts() *fakeContacts {
	return &fakeContacts{rows: map[string]profilemodels.TrustedContact{}, keys: map[string]string{}, fps: map[string]string{}}
}

func (f *fakeContacts) ListTrustedContacts(_ context.Context, customerID string) ([]profilemodels.TrustedContact, error) {
	var out []profilemodels.TrustedContact
	for _, row := range f.rows {
		if row.CustomerID == customerID && row.Active {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeContacts) CreateTrustedContact(_ context.Context, t profilemodels.TrustedContact, key, fp string) (profilemodels.TrustedContact, bool, error) {
	if id, ok := f.keys[t.CustomerID+"|"+key]; key != "" && ok {
		if f.fps[id] != fp {
			return profilemodels.TrustedContact{}, false, profilerepositories.ErrContactRequestKeyReused
		}
		return f.rows[id], true, nil
	}
	t.ID, t.Active, t.Version = uuid.NewString(), true, 1
	f.rows[t.ID], f.fps[t.ID] = t, fp
	if key != "" {
		f.keys[t.CustomerID+"|"+key] = t.ID
	}
	return t, false, nil
}

func (f *fakeContacts) UpdateTrustedContact(_ context.Context, t profilemodels.TrustedContact, expected int64) (profilemodels.TrustedContact, error) {
	row, ok := f.rows[t.ID]
	if !ok || row.CustomerID != t.CustomerID || !row.Active {
		return profilemodels.TrustedContact{}, apperrors.NotFound("Contact not found.", nil)
	}
	if row.Version != expected {
		return profilemodels.TrustedContact{}, profilerepositories.ErrContactVersionConflict
	}
	t.Active, t.Version = true, expected+1
	f.rows[t.ID] = t
	return t, nil
}

func (f *fakeContacts) DeactivateTrustedContact(_ context.Context, customerID, id string) error {
	row, ok := f.rows[id]
	if !ok || row.CustomerID != customerID {
		return apperrors.NotFound("Contact not found.", nil)
	}
	row.Active, row.TripSharingEnabled, row.EmergencyAlertsEnabled = false, false, false
	f.rows[id] = row
	return nil
}

func TestTrustedContactConsentIsNeverImplied(t *testing.T) {
	svc := NewTrustedContactService(newFakeContacts(), true)
	created, _, err := svc.Create(context.Background(), "c1", TrustedContactInput{Name: "Ada", Phone: "08031234567"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.TripSharingEnabled || created.EmergencyAlertsEnabled {
		t.Fatalf("omitted consent must default false: %+v", created)
	}
	if created.Phone != "+2348031234567" {
		t.Fatalf("phone = %q, want normalized +234", created.Phone)
	}
	one := int64(1)
	updated, err := svc.Update(context.Background(), "c1", created.ID, TrustedContactInput{Name: "Ada", Phone: "08031234567"}, &one)
	if err != nil || updated.TripSharingEnabled {
		t.Fatalf("update without flags must not enable sharing: %+v %v", updated, err)
	}
}

func TestTrustedContactCrossCustomerIsolation(t *testing.T) {
	svc := NewTrustedContactService(newFakeContacts(), true)
	yes := true
	created, _, err := svc.Create(context.Background(), "customer-a", TrustedContactInput{Name: "Ada", Phone: "08031234567", TripSharingEnabled: &yes}, "")
	if err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	if _, err := svc.Update(context.Background(), "customer-b", created.ID, TrustedContactInput{Name: "X", Phone: "08031234567"}, &one); apperrors.From(err).Status != http.StatusNotFound {
		t.Fatalf("cross-customer update status = %d", apperrors.From(err).Status)
	}
	if err := svc.Delete(context.Background(), "customer-b", created.ID); apperrors.From(err).Status != http.StatusNotFound {
		t.Fatalf("cross-customer delete status = %d", apperrors.From(err).Status)
	}
	if list, _ := svc.List(context.Background(), "customer-b"); len(list) != 0 {
		t.Fatalf("customer B sees %d contacts of customer A", len(list))
	}
}

func TestTrustedContactIdempotencyVersionAndFlag(t *testing.T) {
	ctx := context.Background()
	if _, err := NewTrustedContactService(newFakeContacts(), false).List(ctx, "c1"); apperrors.From(err).Status != http.StatusServiceUnavailable {
		t.Fatalf("disabled status = %d", apperrors.From(err).Status)
	}
	svc := NewTrustedContactService(newFakeContacts(), true)
	first, _, err := svc.Create(ctx, "c1", TrustedContactInput{Name: "Ada", Phone: "08031234567"}, "k1")
	if err != nil {
		t.Fatal(err)
	}
	replay, replayed, err := svc.Create(ctx, "c1", TrustedContactInput{Name: "Ada", Phone: "+2348031234567"}, "k1")
	if err != nil || !replayed || replay.ID != first.ID {
		t.Fatalf("equivalent replay must return original: %+v %t %v", replay, replayed, err)
	}
	if _, _, err := svc.Create(ctx, "c1", TrustedContactInput{Name: "Bola", Phone: "08031234567"}, "k1"); apperrors.From(err).Status != http.StatusConflict {
		t.Fatalf("key reuse status = %d", apperrors.From(err).Status)
	}
	stale := int64(7)
	if _, err := svc.Update(ctx, "c1", first.ID, TrustedContactInput{Name: "Ada", Phone: "08031234567"}, &stale); apperrors.From(err).Status != http.StatusConflict {
		t.Fatalf("stale version status = %d", apperrors.From(err).Status)
	}
	if _, _, err := svc.Create(ctx, "c1", TrustedContactInput{Name: "Ada", Phone: "123"}, ""); apperrors.From(err).Status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid phone status = %d", apperrors.From(err).Status)
	}
}
