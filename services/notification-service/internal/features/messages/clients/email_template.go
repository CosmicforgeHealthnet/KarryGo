package messageclients

import "cosmicforge/logistics/shared/go/emailbrand"

// formatEmail delegates to the original renderer, shared with Partner Web.
// Delivery metadata is forwarded unchanged to preserve retry idempotency.
func formatEmail(from string, message EmailMessage) ([]byte, error) {
	return emailbrand.Format(from, emailbrand.Message{
		DeliveryID: message.DeliveryID, CreatedAt: message.CreatedAt,
		To: message.To, Subject: message.Subject, Body: message.Body,
		SourceService: message.SourceService, EventType: message.EventType,
	})
}
