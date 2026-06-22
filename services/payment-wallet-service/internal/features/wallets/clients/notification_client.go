package walletclients

import (
	"context"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
)

const notificationSourceService = "payment-wallet-service"

// WalletNotifier sends financial notifications to the central
// notification-service. Like the booking notifier it is fire-and-forget: a
// failed notification must never roll back or block a financial operation, so
// errors are logged and swallowed.
type WalletNotifier struct {
	notifier notifications.Notifier
}

// NewWalletNotifier builds a WalletNotifier from the notification-service base
// URL and the shared HMAC secret. When either is empty it returns a no-op
// notifier so the service runs locally without notification-service.
func NewWalletNotifier(baseURL string, secret []byte) *WalletNotifier {
	if baseURL == "" || len(secret) == 0 {
		return &WalletNotifier{notifier: noopNotifier{}}
	}
	return &WalletNotifier{
		notifier: notifications.Client{
			BaseURL:     baseURL,
			ServiceName: notificationSourceService,
			Secret:      secret,
		},
	}
}

// NewWalletNotifierWith wraps an explicit notifier. Useful in tests.
func NewWalletNotifierWith(notifier notifications.Notifier) *WalletNotifier {
	return &WalletNotifier{notifier: notifier}
}

// NotifyTopUpSuccess tells a customer their wallet top-up succeeded.
func (n *WalletNotifier) NotifyTopUpSuccess(ctx context.Context, customerID, reference string, amountKobo int64) {
	n.send(ctx, notifications.EventPaymentTopupSuccess, notifications.RecipientCustomer, customerID, reference, map[string]interface{}{
		"reference":   reference,
		"amount_kobo": amountKobo,
	})
}

// NotifyPaymentSuccess tells a customer a payment completed.
func (n *WalletNotifier) NotifyPaymentSuccess(ctx context.Context, customerID, reference string, amountKobo int64) {
	n.send(ctx, notifications.EventPaymentSuccess, notifications.RecipientCustomer, customerID, reference, map[string]interface{}{
		"reference":   reference,
		"amount_kobo": amountKobo,
	})
}

// NotifyWithdrawalCompleted tells a provider their withdrawal was paid out.
func (n *WalletNotifier) NotifyWithdrawalCompleted(ctx context.Context, providerID, reference string, amountKobo int64) {
	n.send(ctx, notifications.EventWithdrawalCompleted, notifications.RecipientProvider, providerID, reference, map[string]interface{}{
		"reference":   reference,
		"amount_kobo": amountKobo,
	})
}

// NotifyWithdrawalFailed tells a provider their withdrawal failed.
func (n *WalletNotifier) NotifyWithdrawalFailed(ctx context.Context, providerID, reference, reason string) {
	n.send(ctx, notifications.EventWithdrawalFailed, notifications.RecipientProvider, providerID, reference, map[string]interface{}{
		"reference": reference,
		"reason":    reason,
	})
}

// NotifyWithdrawalReversed tells a provider their withdrawal was reversed.
func (n *WalletNotifier) NotifyWithdrawalReversed(ctx context.Context, providerID, reference, reason string) {
	n.send(ctx, notifications.EventWithdrawalReversed, notifications.RecipientProvider, providerID, reference, map[string]interface{}{
		"reference": reference,
		"reason":    reason,
	})
}

func (n *WalletNotifier) send(ctx context.Context, eventType, recipientType, recipientID, entityID string, data map[string]interface{}) {
	if recipientID == "" {
		return
	}
	_, err := n.notifier.Send(ctx, notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey(notificationSourceService, eventType, entityID),
		SourceService:  notificationSourceService,
		EventType:      eventType,
		Recipient: notifications.Recipient{
			Type: recipientType,
			ID:   recipientID,
		},
		TemplateKey:  eventType,
		Data:         data,
		TemplateData: data,
	})
	if err != nil {
		logging.Error("notification", "send event=%s entity=%s recipient=%s: %v", eventType, entityID, recipientID, err)
	}
}

type noopNotifier struct{}

func (noopNotifier) Send(context.Context, notifications.Request) (notifications.SendResponse, error) {
	return notifications.SendResponse{}, nil
}
