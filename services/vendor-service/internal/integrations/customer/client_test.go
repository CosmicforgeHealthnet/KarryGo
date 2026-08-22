package customer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestResolveSavedAddressUsesHMACAndReturnsSnapshot(t *testing.T) {
	secret := []byte("customer-address-secret")

	verifier := serviceauth.NewVerifier(
		serviceauth.Secrets{
			"vendor-service": secret,
		},
		5*time.Minute,
	)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("method=%s", r.Method)
			}

			wantPath := "/api/v1/customer/internal/customers/customer-a/saved-locations/location-a"

			if r.URL.Path != wantPath {
				t.Fatalf("path=%q want=%q", r.URL.Path, wantPath)
			}

			caller, err := verifier.VerifyRequest(r)
			if err != nil {
				t.Fatalf("VerifyRequest: %v", err)
			}

			if caller != "vendor-service" {
				t.Fatalf("caller=%q", caller)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			_, _ = w.Write([]byte(`{
                "success": true,
                "data": {
                    "id": "location-a",
                    "title": "Home",
                    "address": "23 Akeju Street, Lagos",
                    "latitude": 6.43,
                    "longitude": 3.42,
                    "description": "Gate beside pharmacy"
                }
            }`))
		}),
	)
	defer server.Close()

	client := NewClient(
		server.URL,
		secret,
		2*time.Second,
	)

	snapshot, err := client.ResolveSavedAddress(
		context.Background(),
		"customer-a",
		"location-a",
	)
	if err != nil {
		t.Fatalf("ResolveSavedAddress: %v", err)
	}

	if snapshot.SourceAddressRef != "location-a" {
		t.Fatalf("source_address_ref=%q", snapshot.SourceAddressRef)
	}

	if snapshot.Address != "23 Akeju Street, Lagos" {
		t.Fatalf("address=%q", snapshot.Address)
	}

	if snapshot.Title != "Home" {
		t.Fatalf("title=%q", snapshot.Title)
	}

	if snapshot.SnapshottedAt.IsZero() {
		t.Fatal("expected snapshot timestamp")
	}
}

func TestResolveSavedAddressMapsNotFoundToInvalidAddress(t *testing.T) {
	secret := []byte("customer-address-secret")

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer server.Close()

	client := NewClient(
		server.URL,
		secret,
		2*time.Second,
	)

	_, err := client.ResolveSavedAddress(
		context.Background(),
		"customer-a",
		"foreign-location",
	)

	if !errors.Is(err, cart.ErrInvalidAddress) {
		t.Fatalf("err=%v want ErrInvalidAddress", err)
	}
}

func TestResolveSavedAddressFailsClosedWhenNotConfigured(t *testing.T) {
	client := NewClient(
		"",
		nil,
		time.Second,
	)

	_, err := client.ResolveSavedAddress(
		context.Background(),
		"customer-a",
		"location-a",
	)

	if !errors.Is(err, cart.ErrDependencyMissing) {
		t.Fatalf("err=%v want ErrDependencyMissing", err)
	}
}

func TestResolveSavedAddressRejectsResponseReferenceMismatch(t *testing.T) {
	secret := []byte("customer-address-secret")

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			_, _ = w.Write([]byte(`{
                "success": true,
                "data": {
                    "id": "different-location",
                    "title": "Home",
                    "address": "23 Akeju Street, Lagos",
                    "latitude": 6.43,
                    "longitude": 3.42,
                    "description": ""
                }
            }`))
		}),
	)
	defer server.Close()

	client := NewClient(
		server.URL,
		secret,
		2*time.Second,
	)

	_, err := client.ResolveSavedAddress(
		context.Background(),
		"customer-a",
		"location-a",
	)

	if !errors.Is(err, cart.ErrInvalidAddress) {
		t.Fatalf("err=%v want ErrInvalidAddress", err)
	}
}
