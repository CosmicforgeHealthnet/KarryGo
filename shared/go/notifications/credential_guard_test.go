package notifications

import "testing"

func TestOrdinaryNotificationsRejectCredentialKeys(t *testing.T) {
	for _, payload := range []map[string]interface{}{
		{"otp": "123456"}, {"qr_token": "secret"}, {"share_token": "secret"}, {"account_number": "1234567890"},
		{"nested": map[string]interface{}{"hmac_secret": "secret"}},
	} {
		r := Request{IDempotencyKey: "event", SourceService: "customer-service", EventType: "share.created",
			Recipient: Recipient{Type: RecipientCustomer, ID: "customer"}, TemplateKey: "share.created", Data: payload}
		if err := r.Validate(); err == nil {
			t.Fatalf("credential payload was accepted: %v", payload)
		}
	}
	good := Request{IDempotencyKey: "event", SourceService: "customer-service", EventType: "share.created",
		Recipient: Recipient{Type: RecipientCustomer, ID: "customer"}, TemplateKey: "share.created", Data: map[string]interface{}{"share_ref": "opaque"}}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCustodyNotificationsRejectCredentialKeys(t *testing.T) {
	for _, event := range []string{"dispatch.delivery_handover_challenge", "dispatch.handover_resend", "dispatch.vendor_delivery.handover_challenge", "booking.delivery_code_ready", "package.delivery_code_ready", "taxi.goods.pickup_credential_ready"} {
		for _, field := range []string{"qr_token", "code", "credential"} {
			r := Request{IDempotencyKey: "event", SourceService: "driver-dispatch-delivery-service", EventType: event,
				Recipient: Recipient{Type: RecipientCustomer, ID: "receiver"}, TemplateKey: event,
				TemplateData: map[string]interface{}{field: "opaque-secret"}}
			if err := r.Validate(); err == nil {
				t.Fatalf("%s accepted handover credential field %s", event, field)
			}
		}
	}
	nested := Request{IDempotencyKey: "event", SourceService: "driver-dispatch-delivery-service", EventType: "booking.delivery_code_ready",
		Recipient: Recipient{Type: RecipientCustomer, ID: "receiver"}, TemplateKey: "booking.delivery_code_ready",
		Data: map[string]interface{}{"items": []interface{}{map[string]interface{}{"credential": "secret"}}}}
	if err := nested.Validate(); err == nil {
		t.Fatal("credential in array payload was accepted")
	}
}
