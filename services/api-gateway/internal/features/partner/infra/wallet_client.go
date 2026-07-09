package infra

import (
	"context"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
	"cosmicforge/logistics/shared/go/walletclient"
)

// WalletAdapter adapts shared/go/walletclient to delivery.WalletClient.
type WalletAdapter struct {
	client walletclient.Client
}

// NewWalletAdapter constructs a WalletAdapter. baseURL is a bare origin
// (e.g. http://localhost:8105); the shared client appends the payment-wallet path.
func NewWalletAdapter(baseURL, serviceName string, secret []byte) *WalletAdapter {
	return &WalletAdapter{
		client: walletclient.Client{
			BaseURL:     baseURL,
			ServiceName: serviceName,
			Secret:      secret,
		},
	}
}

func (a *WalletAdapter) CreatePaymentIntent(ctx context.Context, in delivery.PaymentIntentInput) (delivery.PaymentIntentResult, error) {
	intent, err := a.client.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{
		SourceService:   in.SourceService,
		SourceReference: in.SourceReference,
		CustomerID:      in.CustomerID,
		CustomerEmail:   in.CustomerEmail,
		AmountKobo:      in.AmountKobo,
		Currency:        in.Currency,
		PaymentMethod:   in.PaymentMethod,
		IdempotencyKey:  in.IdempotencyKey,
	})
	if err != nil {
		return delivery.PaymentIntentResult{}, err
	}
	return delivery.PaymentIntentResult{
		ID:               intent.ID,
		Reference:        intent.Reference,
		AuthorizationURL: intent.AuthorizationURL,
	}, nil
}

func (a *WalletAdapter) PayFromWallet(ctx context.Context, intentID, idempotencyKey string) error {
	_, err := a.client.PayFromWallet(ctx, intentID, idempotencyKey)
	return err
}

func (a *WalletAdapter) RequestRefund(ctx context.Context, in delivery.RefundInput) error {
	_, err := a.client.RequestRefund(ctx, walletclient.RefundRequest{
		PaymentReference: in.PaymentReference,
		SourceService:    in.SourceService,
		SourceReference:  in.SourceReference,
		AmountKobo:       in.AmountKobo,
		Currency:         in.Currency,
		Reason:           in.Reason,
		IdempotencyKey:   in.IdempotencyKey,
	})
	return err
}
