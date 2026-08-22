package orders

import "testing"

func TestRefundEventActionVocabulary(t *testing.T) {
	for state, want := range map[string]string{
		"REFUND_PENDING":     "commerce.vendor_order.refund_pending",
		"REFUNDED":           "commerce.vendor_order.refunded",
		"PARTIALLY_REFUNDED": "commerce.vendor_order.partially_refunded",
		"UNKNOWN":            "commerce.vendor_order.refund_unknown",
		"FAILED":             "commerce.vendor_order.refund_failed",
	} {
		if got := refundEventAction(state); got != want {
			t.Fatalf("refundEventAction(%q) = %q, want %q", state, got, want)
		}
	}
}
