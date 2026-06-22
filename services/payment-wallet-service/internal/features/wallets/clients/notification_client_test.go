package walletclients

import (
	"context"
	"testing"

	"cosmicforge/logistics/shared/go/notifications"
)

type recordingNotifier struct {
	requests []notifications.Request
}

func (r *recordingNotifier) Send(_ context.Context, request notifications.Request) (notifications.SendResponse, error) {
	r.requests = append(r.requests, request)
	return notifications.SendResponse{}, nil
}

func TestWalletNotifierSendsExpectedEvents(t *testing.T) {
	cases := []struct {
		name          string
		call          func(n *WalletNotifier, ctx context.Context)
		wantEvent     string
		wantRecipient string
		wantID        string
	}{
		{
			name:          "topup success",
			call:          func(n *WalletNotifier, ctx context.Context) { n.NotifyTopUpSuccess(ctx, "cust-1", "ref-1", 5000) },
			wantEvent:     notifications.EventPaymentTopupSuccess,
			wantRecipient: notifications.RecipientCustomer,
			wantID:        "cust-1",
		},
		{
			name:          "payment success",
			call:          func(n *WalletNotifier, ctx context.Context) { n.NotifyPaymentSuccess(ctx, "cust-1", "ref-2", 9000) },
			wantEvent:     notifications.EventPaymentSuccess,
			wantRecipient: notifications.RecipientCustomer,
			wantID:        "cust-1",
		},
		{
			name: "withdrawal completed",
			call: func(n *WalletNotifier, ctx context.Context) {
				n.NotifyWithdrawalCompleted(ctx, "prov-1", "ref-3", 12000)
			},
			wantEvent:     notifications.EventWithdrawalCompleted,
			wantRecipient: notifications.RecipientProvider,
			wantID:        "prov-1",
		},
		{
			name: "withdrawal failed",
			call: func(n *WalletNotifier, ctx context.Context) {
				n.NotifyWithdrawalFailed(ctx, "prov-1", "ref-4", "bank error")
			},
			wantEvent:     notifications.EventWithdrawalFailed,
			wantRecipient: notifications.RecipientProvider,
			wantID:        "prov-1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingNotifier{}
			n := NewWalletNotifierWith(rec)
			tc.call(n, context.Background())

			if len(rec.requests) != 1 {
				t.Fatalf("expected 1 request, got %d", len(rec.requests))
			}
			req := rec.requests[0]
			if req.EventType != tc.wantEvent {
				t.Errorf("event = %q, want %q", req.EventType, tc.wantEvent)
			}
			if req.Recipient.Type != tc.wantRecipient || req.Recipient.ID != tc.wantID {
				t.Errorf("recipient = %s/%s, want %s/%s", req.Recipient.Type, req.Recipient.ID, tc.wantRecipient, tc.wantID)
			}
			if req.IDempotencyKey == "" {
				t.Error("idempotency key is empty")
			}
		})
	}
}

func TestWalletNotifierSkipsEmptyRecipient(t *testing.T) {
	rec := &recordingNotifier{}
	n := NewWalletNotifierWith(rec)
	n.NotifyPaymentSuccess(context.Background(), "", "ref", 100)
	if len(rec.requests) != 0 {
		t.Fatalf("expected no request for empty recipient, got %d", len(rec.requests))
	}
}
