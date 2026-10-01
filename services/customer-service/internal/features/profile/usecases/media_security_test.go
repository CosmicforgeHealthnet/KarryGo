package profileusecases

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	profilerepositories "cosmicforge/logistics/services/customer-service/internal/features/profile/repositories"
	"cosmicforge/logistics/shared/go/mediaclient"
)

type photoRepo struct {
	profilerepositories.CustomerRepository
	writes          int
	lastID, lastURL string
}

func (r *photoRepo) UpdateProfilePhoto(_ context.Context, customerID, assetID, photoURL string) (profilemodels.Customer, error) {
	r.writes++
	r.lastID, r.lastURL = assetID, photoURL
	return profilemodels.Customer{ID: customerID}, nil
}

func TestProfilePhotoMediaRefMustBeActiveAndOwnedByBearerCustomer(t *testing.T) {
	owner, purpose, status := "customer-one", "profile_photo", "active"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Karrygo-Service") != "customer-service" || r.Header.Get("Authorization") != "Bearer backend-only" {
			t.Error("media metadata read lacked backend authorization")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
			"id": "asset-one", "owner_service": "customer-service", "owner_id": owner,
			"purpose": purpose, "status": status, "url": "/asset-one/download",
		}})
	}))
	defer server.Close()
	repo := &photoRepo{}
	svc := NewProfileService(Options{Customers: repo, MediaBaseURL: server.URL + "/api/v1/media-files",
		MediaClient: mediaclient.New(mediaclient.Config{BaseURL: server.URL, ServiceName: "customer-service", Token: "backend-only"})})
	input := SavePhotoURLInput{CustomerID: "customer-one", AssetID: "asset-one", PhotoURL: "https://attacker.example/forged.jpg"}
	for _, tc := range []struct{ owner, purpose, status string }{
		{"someone-else", "profile_photo", "active"},
		{"customer-one", "document_file", "active"},
		{"customer-one", "profile_photo", "deleted"},
	} {
		owner, purpose, status = tc.owner, tc.purpose, tc.status
		if _, err := svc.SaveProfilePhotoURL(context.Background(), input); err == nil {
			t.Fatalf("accepted media metadata %+v", tc)
		}
	}
	if repo.writes != 0 {
		t.Fatal("invalid reference changed the customer profile")
	}
	owner, purpose, status = "customer-one", "profile_photo", "active"
	if _, err := svc.SaveProfilePhotoURL(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if repo.writes != 1 || repo.lastID != "asset-one" || repo.lastURL != server.URL+"/api/v1/media-files/asset-one/download" {
		t.Fatalf("untrusted URL used: writes=%d id=%s url=%s", repo.writes, repo.lastID, repo.lastURL)
	}
}
