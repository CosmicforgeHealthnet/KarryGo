package notifications

import "strings"

// RouteIntent is a navigation hint, never a snapshot of business state.
// A client must read the referenced object from its owner after opening it.
type RouteIntent struct {
	Target   string `json:"target"`
	Ref      string `json:"ref"`
	ReadBack bool   `json:"read_back"`
}

func opaqueRouteRef(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 200 {
		return ""
	}
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == ':' {
			continue
		}
		return ""
	}
	return value
}

// RouteIntentFor uses only the source service, controlled event family and
// opaque references. It ignores caller-provided URLs and business state.
func RouteIntentFor(request Request) (RouteIntent, bool) {
	if request.Recipient.Type != RecipientCustomer && request.Recipient.Type != "receiver" {
		return RouteIntent{}, false
	}
	ref := func(keys ...string) string {
		for _, key := range keys {
			if raw, ok := request.Data[key].(string); ok {
				if value := opaqueRouteRef(raw); value != "" {
					return value
				}
			}
		}
		return opaqueRouteRef(request.SourceAggregateID)
	}
	var target, reference string
	switch request.SourceService {
	case "driver-taxi-service":
		if strings.HasPrefix(request.EventType, "taxi.booking.") || strings.HasPrefix(request.EventType, "network.customer.") {
			target, reference = "RIDE_DETAIL", ref("booking_id")
		} else if strings.HasPrefix(request.EventType, "taxi.goods.") || request.EventType == EventBookingDeliveryCodeReady {
			target, reference = "TAXI_GOODS_MISSION", ref("mission_ref")
		}
	case "driver-dispatch-delivery-service":
		if strings.HasPrefix(request.EventType, "delivery.") || strings.HasPrefix(request.EventType, "package.") || strings.HasPrefix(request.EventType, "network.customer.") {
			target, reference = "DISPATCH_TRIP_DETAIL", ref("booking_id", "mission_ref")
		} else if strings.HasPrefix(request.EventType, "dispatch.vendor_delivery.") || strings.HasPrefix(request.EventType, "dispatch.handover_") || request.EventType == "dispatch.delivery_handover_challenge" {
			target, reference = "DISPATCH_MISSION_DETAIL", ref("mission_ref")
		}
	case "driver-hauling-service":
		if strings.HasPrefix(request.EventType, "booking.") || strings.HasPrefix(request.EventType, "cargo.") || strings.HasPrefix(request.EventType, "network.customer.") {
			target, reference = "TRUCK_BOOKING_DETAIL", ref("booking_id")
		}
	case "vendor-service":
		if strings.HasPrefix(request.EventType, "commerce.customer.") || request.EventType == EventCommerceCustomerETAChanged {
			target, reference = "COMMERCE_ORDER_DETAIL", ref("parent_order_ref")
		}
	case "payment-wallet-service":
		if strings.HasPrefix(request.EventType, "payment.") || strings.HasPrefix(request.EventType, "refund.") {
			target, reference = "PAYMENT_DETAIL", ref("reference", "payment_ref", "refund_ref")
		}
	case "support-dispute-service":
		// Customer V4 §16.1: a support-case update opens the case, which the
		// client reads back from Support.
		if strings.HasPrefix(request.EventType, "support.") {
			target, reference = "SUPPORT_CASE_DETAIL", ref("complaint_id")
		}
	case "customer-service":
		if strings.HasPrefix(request.EventType, "share.") || strings.HasPrefix(request.EventType, "safety.") {
			target, reference = "SHARE_MANAGEMENT", ref("share_ref")
		}
	}
	if target == "" || reference == "" {
		return RouteIntent{}, false
	}
	return RouteIntent{Target: target, Ref: reference, ReadBack: true}, true
}
