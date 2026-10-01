package goods

import (
	"context"
	"fmt"
	"strings"

	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/walletclient"
)

const taxiGoodsSourceService = "driver-taxi-service"

type NotificationAdapter struct {
	notifier  notifications.Notifier
	publisher *notifications.RedisPublisher
}

func NewNotificationAdapter(notifier notifications.Notifier) *NotificationAdapter {
	return &NotificationAdapter{notifier: notifier}
}

func NewRedisNotificationAdapter(publisher notifications.RedisPublisher) *NotificationAdapter {
	return &NotificationAdapter{publisher: &publisher}
}

func (a *NotificationAdapter) NotifyDeliveryCredential(ctx context.Context, receiverRef, missionRef string, credential Credential) error {
	if a == nil || (a.notifier == nil && a.publisher == nil) {
		return ErrUnavailable
	}
	request := notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey(taxiGoodsSourceService, notifications.EventBookingDeliveryCodeReady, missionRef+":"+credential.ChallengeRef),
		SourceService:  taxiGoodsSourceService, EventType: notifications.EventBookingDeliveryCodeReady,
		Recipient:    notifications.Recipient{Type: notifications.RecipientCustomer, ID: receiverRef},
		TemplateKey:  notifications.EventBookingDeliveryCodeReady,
		Data:         map[string]any{"mission_ref": missionRef, "challenge_ref": credential.ChallengeRef, "expires_at": credential.ExpiresAt},
		TemplateData: map[string]any{"mission_ref": missionRef, "challenge_ref": credential.ChallengeRef, "expires_at": credential.ExpiresAt},
	}
	if a.publisher != nil {
		return a.publisher.Publish(ctx, request)
	}
	_, err := a.notifier.Send(ctx, request)
	return err
}

func (a *NotificationAdapter) NotifyPickupCredential(ctx context.Context, providerRef, missionRef string, credential Credential) error {
	if a == nil || (a.notifier == nil && a.publisher == nil) {
		return ErrUnavailable
	}
	const eventType = "taxi.goods.pickup_credential_ready"
	request := notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey(taxiGoodsSourceService, eventType, missionRef+":"+credential.ChallengeRef),
		SourceService:  taxiGoodsSourceService, EventType: eventType,
		Recipient:    notifications.Recipient{Type: notifications.RecipientProvider, ID: providerRef},
		TemplateKey:  eventType,
		Data:         map[string]any{"mission_ref": missionRef, "challenge_ref": credential.ChallengeRef, "expires_at": credential.ExpiresAt},
		TemplateData: map[string]any{"mission_ref": missionRef, "challenge_ref": credential.ChallengeRef, "expires_at": credential.ExpiresAt},
	}
	if a.publisher != nil {
		return a.publisher.Publish(ctx, request)
	}
	_, err := a.notifier.Send(ctx, request)
	return err
}

type MediaAdapter struct{ client *mediaclient.Client }

func NewMediaAdapter(client *mediaclient.Client) *MediaAdapter { return &MediaAdapter{client: client} }
func (a *MediaAdapter) AuthorizeEvidence(ctx context.Context, missionID, mediaRef string) error {
	if a == nil || a.client == nil {
		return ErrUnavailable
	}
	asset, err := a.client.GetByID(ctx, mediaRef)
	if err != nil {
		return err
	}
	if strings.TrimSpace(asset.OwnerID) != strings.TrimSpace(missionID) || strings.ToLower(strings.TrimSpace(asset.Status)) != "active" {
		return ErrMediaScopeMismatch
	}
	return nil
}

type WalletAdapter struct{ client walletclient.Client }

func NewWalletAdapter(client walletclient.Client) *WalletAdapter {
	return &WalletAdapter{client: client}
}
func (a *WalletAdapter) ReleaseCommerceDelivery(ctx context.Context, parentOrderID, vendorOrderID, providerID, completionReference string) error {
	if a == nil {
		return ErrUnavailable
	}
	key := "taxi-goods-delivery-release-" + completionReference + "-" + vendorOrderID
	result, err := a.client.ReleaseCommerceDelivery(ctx, vendorOrderID, walletclient.DeliveryReleaseRequest{ProviderType: "taxi", ProviderID: providerID, CompletionReference: completionReference, IdempotencyKey: key})
	if err != nil {
		if recovered, readErr := a.client.GetCommerceOperation(ctx, "DELIVERY_RELEASE", key); readErr == nil && strings.EqualFold(strings.TrimSpace(recovered.Status), "APPLIED") {
			return nil
		}
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(result.Status), "APPLIED") {
		return fmt.Errorf("wallet commerce settlement is not applied: %s", result.Status)
	}
	return nil
}
