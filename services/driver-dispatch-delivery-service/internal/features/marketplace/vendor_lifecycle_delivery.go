package marketplace

import (
	"context"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// Existing commerce.* envelopes are unchanged. New lifecycle envelopes carry
// only the immutable approved projection, never offers, locations or challenges.
type VendorLifecycleDeliverer struct{ Transport EventDeliverer }

func (d VendorLifecycleDeliverer) Configured() bool {
	return d.Transport != nil && d.Transport.Configured()
}
func (d VendorLifecycleDeliverer) Deliver(ctx context.Context, e Envelope) (DeliveryResult, error) {
	if !strings.HasPrefix(e.EventType, "dispatch.") {
		return d.Transport.Deliver(ctx, e)
	}
	if !dispatchcommerce.VendorLifecycleEvent(e.EventType) {
		return DeliveryResult{Outcome: DeliveryRejected, Code: "VENDOR_EVENT_NOT_ALLOWLISTED"}, nil
	}
	raw, _ := json.Marshal(e.Payload["delivery_projection"])
	if _, err := dispatchcommerce.DecodeLifecycleProjection(raw); err != nil {
		return DeliveryResult{Outcome: DeliveryRejected, Code: "VENDOR_SOURCE_BINDING_MISSING"}, err
	}
	e.Payload = map[string]any{}
	_ = json.Unmarshal(raw, &e.Payload)
	sum := sha256.Sum256(raw)
	e.PayloadHash = hex.EncodeToString(sum[:])
	return d.Transport.Deliver(ctx, e)
}
