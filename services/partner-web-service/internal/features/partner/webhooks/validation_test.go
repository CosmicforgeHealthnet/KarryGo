package webhooks

import "testing"

func TestValidateWebhookURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://pharmacy.com/karrygo-webhook", false},
		{"http rejected", "http://pharmacy.com/hook", true},
		{"empty rejected", "", true},
		{"localhost rejected", "https://localhost/hook", true},
		{"127.0.0.1 rejected", "https://127.0.0.1/hook", true},
		{"::1 rejected", "https://[::1]/hook", true},
		{"private 10.x rejected", "https://10.1.2.3/hook", true},
		{"private 172.16 rejected", "https://172.16.5.5/hook", true},
		{"private 192.168 rejected", "https://192.168.1.10/hook", true},
		{"link-local rejected", "https://169.254.1.1/hook", true},
		{"no host rejected", "https:///hook", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWebhookURL(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("validateWebhookURL(%q) = nil, want field error", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateWebhookURL(%q) = %+v, want nil", tc.url, err)
			}
		})
	}
}

func TestValidateAndNormalizeEvents(t *testing.T) {
	// dedupe preserving order
	got, ferr := validateAndNormalizeEvents([]string{
		EventDeliveryAccepted, EventDeliveryCompleted, EventDeliveryAccepted,
	})
	if ferr != nil {
		t.Fatalf("unexpected error: %+v", ferr)
	}
	if len(got) != 2 || got[0] != EventDeliveryAccepted || got[1] != EventDeliveryCompleted {
		t.Fatalf("dedupe/order wrong: %v", got)
	}

	if _, ferr := validateAndNormalizeEvents(nil); ferr == nil {
		t.Fatal("empty events accepted, want error")
	}
	if _, ferr := validateAndNormalizeEvents([]string{"delivery.bogus"}); ferr == nil {
		t.Fatal("unknown event accepted, want error")
	}
}
