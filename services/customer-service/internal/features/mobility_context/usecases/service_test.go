package mobilityusecases

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	mobilitymodels "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/models"
	mobilityrepositories "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

// fakeRepo mirrors the Postgres repository's ownership, version and
// idempotency semantics so usecase rules are testable without a database.
type fakeRepo struct {
	prefs  map[string]mobilitymodels.MobilityPreferences
	guests map[string]mobilitymodels.GuestTraveller
	keys   map[string]string // customer|key -> guest id
	fps    map[string]string // guest id -> fingerprint
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{prefs: map[string]mobilitymodels.MobilityPreferences{}, guests: map[string]mobilitymodels.GuestTraveller{}, keys: map[string]string{}, fps: map[string]string{}}
}

func (r *fakeRepo) GetPreferences(_ context.Context, id string) (mobilitymodels.MobilityPreferences, bool, error) {
	p, ok := r.prefs[id]
	return p, ok, nil
}

func (r *fakeRepo) SavePreferences(_ context.Context, p mobilitymodels.MobilityPreferences, expected int64) (mobilitymodels.MobilityPreferences, error) {
	current, ok := r.prefs[p.CustomerID]
	if (expected == 0 && ok) || (expected > 0 && (!ok || current.Version != expected)) {
		return mobilitymodels.MobilityPreferences{}, mobilityrepositories.ErrVersionConflict
	}
	p.Version = expected + 1
	r.prefs[p.CustomerID] = p
	return p, nil
}

func (r *fakeRepo) ListGuests(_ context.Context, customerID string) ([]mobilitymodels.GuestTraveller, error) {
	var out []mobilitymodels.GuestTraveller
	for _, g := range r.guests {
		if g.CustomerID == customerID && g.Active {
			out = append(out, g)
		}
	}
	return out, nil
}

func (r *fakeRepo) GetGuest(_ context.Context, customerID, id string) (mobilitymodels.GuestTraveller, error) {
	g, ok := r.guests[id]
	if !ok || g.CustomerID != customerID || !g.Active {
		return mobilitymodels.GuestTraveller{}, apperrors.NotFound("Traveller not found.", nil)
	}
	return g, nil
}

func (r *fakeRepo) CreateGuest(_ context.Context, g mobilitymodels.GuestTraveller, key, fp string) (mobilitymodels.GuestTraveller, bool, error) {
	if key != "" {
		if id, ok := r.keys[g.CustomerID+"|"+key]; ok {
			if r.fps[id] != fp {
				return mobilitymodels.GuestTraveller{}, false, mobilityrepositories.ErrRequestKeyReused
			}
			return r.guests[id], true, nil
		}
	}
	g.ID, g.Active, g.Version = uuid.NewString(), true, 1
	r.guests[g.ID] = g
	r.fps[g.ID] = fp
	if key != "" {
		r.keys[g.CustomerID+"|"+key] = g.ID
	}
	return g, false, nil
}

func (r *fakeRepo) UpdateGuest(ctx context.Context, g mobilitymodels.GuestTraveller, expected int64) (mobilitymodels.GuestTraveller, error) {
	current, err := r.GetGuest(ctx, g.CustomerID, g.ID)
	if err != nil {
		return current, err
	}
	if current.Version != expected {
		return mobilitymodels.GuestTraveller{}, mobilityrepositories.ErrVersionConflict
	}
	g.Active, g.Version = true, expected+1
	r.guests[g.ID] = g
	return g, nil
}

func (r *fakeRepo) DeactivateGuest(_ context.Context, customerID, id string) error {
	g, ok := r.guests[id]
	if !ok || g.CustomerID != customerID {
		return apperrors.NotFound("Traveller not found.", nil)
	}
	g.Active = false
	r.guests[id] = g
	return nil
}

func enabled(repo Repository) *Service {
	return NewService(repo, Flags{MobilityPreferences: true, GuestTravellers: true})
}

func statusOf(err error) int { return apperrors.From(err).Status }

func version(v int64) *int64 { return &v }

func TestDisabledFlagsReturnUnavailable(t *testing.T) {
	svc := NewService(newFakeRepo(), Flags{})
	if _, err := svc.GetPreferences(context.Background(), "c1"); statusOf(err) != http.StatusServiceUnavailable {
		t.Fatalf("preferences disabled status = %d", statusOf(err))
	}
	if _, err := svc.ListGuests(context.Background(), "c1"); statusOf(err) != http.StatusServiceUnavailable {
		t.Fatalf("guests disabled status = %d", statusOf(err))
	}
}

func TestPreferencesDefaultThenOptimisticVersioning(t *testing.T) {
	svc := enabled(newFakeRepo())
	ctx := context.Background()
	got, err := svc.GetPreferences(ctx, "c1")
	if err != nil || got.Version != 0 || got.LuggageDefault != "NONE" {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
	if _, err := svc.PutPreferences(ctx, "c1", mobilitymodels.MobilityPreferences{}, nil); statusOf(err) != http.StatusUnprocessableEntity {
		t.Fatalf("missing expected_version status = %d", statusOf(err))
	}
	seat := "infant"
	saved, err := svc.PutPreferences(ctx, "c1", mobilitymodels.MobilityPreferences{ChildSeatPreference: &seat, CommunicationPreferences: json.RawMessage(`{"preferred_channel":"text","quiet_ride":true}`)}, version(0))
	if err != nil || saved.Version != 1 || *saved.ChildSeatPreference != "INFANT" {
		t.Fatalf("create = %+v, %v", saved, err)
	}
	if _, err := svc.PutPreferences(ctx, "c1", mobilitymodels.MobilityPreferences{}, version(0)); statusOf(err) != http.StatusConflict {
		t.Fatalf("second create status = %d, want 409", statusOf(err))
	}
	if _, err := svc.PutPreferences(ctx, "c1", mobilitymodels.MobilityPreferences{}, version(1)); err != nil {
		t.Fatalf("update v1: %v", err)
	}
	if _, err := svc.PutPreferences(ctx, "c1", mobilitymodels.MobilityPreferences{}, version(1)); statusOf(err) != http.StatusConflict {
		t.Fatalf("stale update status = %d, want 409", statusOf(err))
	}
}

func TestPreferencesRejectUnsupportedValues(t *testing.T) {
	svc := enabled(newFakeRepo())
	count := 9
	cases := []mobilitymodels.MobilityPreferences{
		{PreferredPassengerCount: &count},
		{CommunicationPreferences: json.RawMessage(`{"home_address":"x"}`)},
		{CommunicationPreferences: json.RawMessage(`[1]`)},
		{LuggageDefault: "TRUNK"},
	}
	for i, input := range cases {
		if _, err := svc.PutPreferences(context.Background(), "c1", input, version(0)); statusOf(err) != http.StatusUnprocessableEntity {
			t.Fatalf("case %d status = %d, want 422", i, statusOf(err))
		}
	}
}

func TestGuestTravellerOwnershipIsolation(t *testing.T) {
	svc := enabled(newFakeRepo())
	ctx := context.Background()
	guest, _, err := svc.CreateGuest(ctx, "customer-a", mobilitymodels.GuestTraveller{Name: "Mum"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetGuest(ctx, "customer-b", guest.ID); statusOf(err) != http.StatusNotFound {
		t.Fatalf("B reading A's guest status = %d, want 404", statusOf(err))
	}
	if _, err := svc.UpdateGuest(ctx, "customer-b", guest.ID, mobilitymodels.GuestTraveller{Name: "Hijack"}, version(1)); statusOf(err) != http.StatusNotFound {
		t.Fatalf("B updating A's guest status = %d, want 404", statusOf(err))
	}
	if err := svc.DeleteGuest(ctx, "customer-b", guest.ID); statusOf(err) != http.StatusNotFound {
		t.Fatalf("B deleting A's guest status = %d, want 404", statusOf(err))
	}
	if list, _ := svc.ListGuests(ctx, "customer-b"); len(list) != 0 {
		t.Fatalf("B sees %d guests", len(list))
	}
	if _, err := svc.GetGuest(ctx, "customer-a", "not-a-uuid"); statusOf(err) != http.StatusNotFound {
		t.Fatalf("malformed id status = %d, want 404", statusOf(err))
	}
}

func TestGuestTravellerSoftDeleteAndIdempotentCreate(t *testing.T) {
	repo := newFakeRepo()
	svc := enabled(repo)
	ctx := context.Background()
	phone := "08031234567"
	first, replayed, err := svc.CreateGuest(ctx, "c1", mobilitymodels.GuestTraveller{Name: "Dad", Phone: &phone}, "key-1")
	if err != nil || replayed || *first.Phone != "+2348031234567" {
		t.Fatalf("create = %+v replayed=%t err=%v", first, replayed, err)
	}
	again, replayed, err := svc.CreateGuest(ctx, "c1", mobilitymodels.GuestTraveller{Name: "Dad", Phone: &phone}, "key-1")
	if err != nil || !replayed || again.ID != first.ID {
		t.Fatalf("replay = %+v replayed=%t err=%v", again, replayed, err)
	}
	if _, _, err := svc.CreateGuest(ctx, "c1", mobilitymodels.GuestTraveller{Name: "Uncle"}, "key-1"); statusOf(err) != http.StatusConflict {
		t.Fatalf("key reuse status = %d, want 409", statusOf(err))
	}
	if err := svc.DeleteGuest(ctx, "c1", first.ID); err != nil {
		t.Fatal(err)
	}
	if repo.guests[first.ID].Active {
		t.Fatal("guest must be soft deleted, not removed")
	}
	if _, ok := repo.guests[first.ID]; !ok {
		t.Fatal("soft delete must keep the row")
	}
	if _, err := svc.GetGuest(ctx, "c1", first.ID); statusOf(err) != http.StatusNotFound {
		t.Fatalf("deactivated guest status = %d, want 404", statusOf(err))
	}
}
