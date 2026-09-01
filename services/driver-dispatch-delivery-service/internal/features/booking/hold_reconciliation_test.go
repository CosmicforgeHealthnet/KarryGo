package booking

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/walletclient"
)

func TestUnknownHoldDoesNotStartMatchingOrClaimFailure(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{holdErr: &UnknownPaymentOutcome{Cause: errors.New("timeout")}}
	s := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)
	_, err := s.CreateBooking(context.Background(), testCustomer, validCreateInput())
	if err == nil || len(repo.bookings) != 1 || len(broadcast.started) != 0 {
		t.Fatal("unknown hold started matching")
	}
	for _, b := range repo.bookings {
		if b.Status != StatusCancelled || b.PaymentStatus != PaymentUnpaid || b.CancelReason != "payment_outcome_unknown" {
			t.Fatal("unknown result misrepresented")
		}
	}
}

func TestAbandonedHoldReadbackNeverRepeatsDebit(t *testing.T) {
	for _, status := range []string{"held", "pending", "completed", "refunded"} {
		t.Run(status, func(t *testing.T) {
			var refunds []walletclient.RefundRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var data any
				switch {
				case strings.HasSuffix(r.URL.Path, "/pay-from-wallet"):
					t.Error("reconciliation issued a debit")
					w.WriteHeader(500)
					return
				case strings.HasSuffix(r.URL.Path, "/payment-intents"):
					var in walletclient.PaymentIntentRequest
					_ = json.NewDecoder(r.Body).Decode(&in)
					if in.IdempotencyKey != "dispatch-hold-booking" {
						t.Error("changed hold identity")
					}
					data = walletclient.PaymentIntent{Reference: "payment"}
				case strings.HasSuffix(r.URL.Path, "/refunds"):
					var in walletclient.RefundRequest
					_ = json.NewDecoder(r.Body).Decode(&in)
					refunds = append(refunds, in)
					data = walletclient.Refund{Status: "processed"}
				default:
					data = walletclient.PaymentIntent{Reference: "payment", SourceService: walletSourceService, SourceReference: "booking", CustomerID: "customer", AmountKobo: 1500, Currency: "NGN", Status: status}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
			}))
			defer server.Close()
			client := NewWalletPaymentClient(walletclient.Client{BaseURL: server.URL, ServiceName: "driver-dispatch-delivery-service", Secret: []byte("test-secret"), HTTPClient: &http.Client{Timeout: time.Second}})
			b := Booking{ID: "booking", CustomerID: "customer", FareAmount: 1500, Currency: "NGN"}
			got, _, err := client.ReconcileAbandonedHold(context.Background(), b)
			if status == "held" || status == "refunded" {
				if err != nil || got != PaymentRefunded {
					t.Fatal(got, err)
				}
			} else if err == nil {
				t.Fatal("ambiguous/nonrefundable hold claimed resolved")
			}
			if status == "held" {
				_, _, _ = client.ReconcileAbandonedHold(context.Background(), b)
				if len(refunds) != 2 || refunds[0].IdempotencyKey != refunds[1].IdempotencyKey {
					t.Fatal("refund identity changed")
				}
			} else if len(refunds) != 0 {
				t.Fatal("unsafe refund")
			}
		})
	}
}
