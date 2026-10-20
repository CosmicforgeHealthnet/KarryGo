// Package callbacks exposes the internal HTTP callback endpoints dispatch and
// payment-wallet call as they migrate off Redis Pub/Sub. The endpoints are
// service-to-service only (X-Internal-Service-Key) and are NEVER exposed to
// partners. Dispatch callbacks reuse the live partner/webhooks pipeline
// (status update + rider snapshot + webhook fan-out) - no duplicate system.
package callbacks

import (
	"context"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/billing"
	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/webhooks"
	"cosmicforge/logistics/services/partner-web-service/internal/features/partneraccount"
)

// CallbackBasePath is the internal callback route prefix.
const CallbackBasePath = "/api/v1/internal/callbacks"

// DispatchProcessor is the webhooks-service surface the dispatch callbacks use.
type DispatchProcessor interface {
	ProcessDispatchEventResult(ctx context.Context, topic string, raw []byte) (bool, error)
}

// ChargeResultHandler is the billing-service surface the charge-result callback uses.
type ChargeResultHandler interface {
	HandleChargeResult(ctx context.Context, in billing.ChargeResultInput) (*billing.ChargeResultOutcome, error)
}

// Dependencies holds what the internal callback routes need. Dispatch and
// Billing may be nil (when webhooks/billing are not configured); their
// endpoints then return 503 service_unavailable instead of panicking.
type Dependencies struct {
	InternalServiceKey string
	Dispatch           DispatchProcessor
	Billing            ChargeResultHandler
}

// dispatchCallbackTopics maps callback paths to the dispatch topics the live
// webhooks pipeline already understands (status mapping + webhook event live in
// webhooks.MappingForTopic).
var dispatchCallbackTopics = map[string]string{
	"request-accepted":  webhooks.DispatchTopicRequestAccepted,
	"no-provider-found": webhooks.DispatchTopicNoProviderFound,
	"provider-arrived":  webhooks.DispatchTopicProviderArrived,
	"trip-started":      webhooks.DispatchTopicTripStarted,
	"trip-completed":    webhooks.DispatchTopicTripCompleted,
	"trip-cancelled":    webhooks.DispatchTopicTripCancelled,
}

// RegisterCallbackRoutes mounts the internal callback endpoints under
// /api/v1/internal/callbacks, guarded by X-Internal-Service-Key.
func RegisterCallbackRoutes(r *gin.Engine, deps Dependencies) {
	h := &handler{dispatch: deps.Dispatch, billing: deps.Billing}

	grp := r.Group(CallbackBasePath)
	grp.Use(partneraccount.RequireServiceKey(deps.InternalServiceKey))

	for path, topic := range dispatchCallbackTopics {
		grp.POST("/"+path, h.dispatchCallback(topic))
	}
	grp.POST("/charge-result", h.chargeResult)
}
