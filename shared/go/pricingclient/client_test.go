package pricingclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func validRequest() QuoteRequest {
	return QuoteRequest{
		ConsumerRef: "booking-1",
		Scope:       ScopeTaxi,
		Channel:     ChannelCustomerApp,
		Classes:     []string{"economy"},
		Pickup:      &Point{Lat: 6.5, Lng: 3.4},
		Route:       Route{DistanceMeters: 8000, DurationSeconds: 1230},
	}
}

func TestQuoteIsSignedAndDecoded(t *testing.T) {
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"driver-taxi-service": []byte("s3cret")}, time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != QuotesPath {
			t.Errorf("path = %s", r.URL.Path)
		}
		if _, err := verifier.VerifyRequest(r); err != nil {
			t.Errorf("signature rejected: %v", err)
		}
		_, _ = io.WriteString(w, `{"success":true,"data":{"quotes":[{"id":"pq_1","class":"economy","mode":"SHADOW","gross_kobo":170000}]}}`)
	}))
	defer server.Close()

	client := New(Options{BaseURL: server.URL, ServiceName: "driver-taxi-service", Secret: "s3cret"})
	response, err := client.Quote(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if len(response.Quotes) != 1 || response.Quotes[0].ID != "pq_1" || response.Quotes[0].GrossKobo != 170000 {
		t.Fatalf("decoded %+v", response)
	}
}

func TestRefusalAndOutageAreDistinct(t *testing.T) {
	status := http.StatusUnprocessableEntity
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"success":false,"error":{"code":"no_market","message":"No pricing market covers this pickup."}}`)
	}))
	defer server.Close()
	client := New(Options{BaseURL: server.URL, ServiceName: "x", Secret: "y"})

	_, err := client.Quote(context.Background(), validRequest())
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "No pricing market") {
		t.Fatalf("422 should be a refusal carrying pricing's message, got %v", err)
	}

	status = http.StatusBadGateway
	_, err = client.Quote(context.Background(), validRequest())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("502 should be unavailable, got %v", err)
	}
}

func TestUnconfiguredAndInvalidRequestsSendNothing(t *testing.T) {
	if _, err := New(Options{}).Quote(context.Background(), validRequest()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
	client := New(Options{BaseURL: "http://127.0.0.1:1", ServiceName: "x", Secret: "y"})
	bad := validRequest()
	bad.Pickup = nil
	if _, err := client.Quote(context.Background(), bad); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest for a request with no place, got %v", err)
	}
	if _, err := client.Bind(context.Background(), "", BindRequest{BookingRef: "b"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest for a blank quote id, got %v", err)
	}
}
