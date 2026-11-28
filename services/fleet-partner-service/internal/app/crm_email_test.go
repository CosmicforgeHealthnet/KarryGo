package app

import (
	"strings"
	"testing"

	crmmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
)

// Every CRM intent the service emits must produce a deliverable email: a
// title, a body (notification-service refuses an email without one) and a
// structured layout naming the sending organization.
func TestEveryCRMIntentProducesADeliverableEmail(t *testing.T) {
	payload := map[string]any{"quotationNumber": "QUO-1", "contractNumber": "CON-1", "currency": "ngn",
		"totalAmountMinor": float64(125000050), "validUntil": "2026-12-12T00:00:00Z",
		"metricCode": "ON_TIME_DELIVERY", "outcome": "BREACHED", "holdStatus": "ON_HOLD"}
	for _, intent := range []string{
		crmmodels.IntentQuotationIssued, crmmodels.IntentQuotationAccepted, crmmodels.IntentQuotationRejected,
		crmmodels.IntentQuotationExpired, crmmodels.IntentContractActivated, crmmodels.IntentContractSuspended,
		crmmodels.IntentContractTerminated, crmmodels.IntentContractRenewalDue, crmmodels.IntentContractExpired,
		crmmodels.IntentSLAEscalation, crmmodels.IntentCreditHold, crmmodels.IntentCreditHoldReleased, "UNKNOWN_INTENT",
	} {
		email := buildCRMEmail(intent, payload, "Ada Lovelace", "Cosmicforge")
		if email.Title == "" || strings.TrimSpace(email.Body) == "" || email.Layout.Heading == "" ||
			email.Layout.Status == nil || email.Layout.Greeting != "Hello Ada," ||
			!strings.Contains(email.Body, "Cosmicforge") {
			t.Errorf("%s: incomplete email %+v", intent, email)
		}
		if email.Layout.Support {
			t.Errorf("%s: customer emails must direct questions to the organization, not Karryam support", intent)
		}
	}
	issued := buildCRMEmail(crmmodels.IntentQuotationIssued, payload, "", "Cosmicforge")
	if issued.Layout.Details.Total == nil || issued.Layout.Details.Total.Value != "NGN 1,250,000.50" ||
		issued.Layout.Greeting != "Hello," {
		t.Fatalf("quotation summary: %+v", issued.Layout.Details)
	}
}

func TestFormatMinorAmount(t *testing.T) {
	for minor, want := range map[int64]string{0: "NGN 0.00", 99: "NGN 0.99", 100000: "NGN 1,000.00", -123456: "NGN -1,234.56"} {
		if got := formatMinorAmount("NGN", minor); got != want {
			t.Errorf("formatMinorAmount(%d) = %q, want %q", minor, got, want)
		}
	}
}
