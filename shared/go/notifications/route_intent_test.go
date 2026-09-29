package notifications

import "testing"

func TestRouteIntentCatalogueUsesOpaqueOwnerReadBack(t *testing.T) {
	for _, tc := range []struct{ service, event, key, target string }{
		{"driver-taxi-service", "taxi.booking.arrived_at_pickup", "booking_id", "RIDE_DETAIL"},
		{"driver-taxi-service", EventBookingDeliveryCodeReady, "mission_ref", "TAXI_GOODS_MISSION"},
		{"driver-dispatch-delivery-service", EventPackageDelivered, "booking_id", "DISPATCH_TRIP_DETAIL"},
		{"driver-dispatch-delivery-service", "dispatch.delivery_handover_challenge", "mission_ref", "DISPATCH_MISSION_DETAIL"},
		{"driver-dispatch-delivery-service", "dispatch.vendor_delivery.handover_challenge", "mission_ref", "DISPATCH_MISSION_DETAIL"},
		{"driver-hauling-service", EventCargoDelivered, "booking_id", "TRUCK_BOOKING_DETAIL"},
		{"vendor-service", "commerce.customer.delivered", "parent_order_ref", "COMMERCE_ORDER_DETAIL"},
		{"payment-wallet-service", EventRefundCompleted, "reference", "PAYMENT_DETAIL"},
		{"customer-service", "share.created", "share_ref", "SHARE_MANAGEMENT"},
		{"support-dispute-service", "support.status_changed", "complaint_id", "SUPPORT_CASE_DETAIL"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			request := Request{SourceService: tc.service, EventType: tc.event, Recipient: Recipient{Type: RecipientCustomer}, Data: map[string]interface{}{tc.key: "opaque_123", "url": "https://untrusted.example"}}
			intent, ok := RouteIntentFor(request)
			if !ok || intent.Target != tc.target || intent.Ref != "opaque_123" || !intent.ReadBack {
				t.Fatalf("unsafe or missing hint: %+v %t", intent, ok)
			}
			request.Data[tc.key] = "https://untrusted.example/path"
			if _, ok := RouteIntentFor(request); ok {
				t.Fatal("accepted URL as route reference")
			}
			request.Data[tc.key] = "opaque_123"
			request.Recipient.Type = RecipientProvider
			if _, ok := RouteIntentFor(request); ok {
				t.Fatal("customer route sent to provider")
			}
		})
	}
}

func TestReceiverRouteIntentRequiresOpaqueReference(t *testing.T) {
	r := Request{SourceService: "driver-taxi-service", EventType: EventBookingDeliveryCodeReady,
		Recipient: Recipient{Type: "receiver", ID: "receiver-ref"}, Data: map[string]interface{}{"mission_ref": "mission-1"}}
	if intent, ok := RouteIntentFor(r); !ok || intent.Target != "TAXI_GOODS_MISSION" || intent.Ref != "mission-1" {
		t.Fatalf("receiver hint = %+v %t", intent, ok)
	}
	r.Data["mission_ref"] = "https://example.test/token"
	if _, ok := RouteIntentFor(r); ok {
		t.Fatal("accepted a caller URL as receiver route intent")
	}
}
