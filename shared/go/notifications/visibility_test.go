package notifications

import (
	"strings"
	"testing"
)

func providerNotification(data map[string]interface{}) Request {
	return Request{
		IDempotencyKey: "idem_1", SourceService: "driver-hauling-service",
		EventType: "booking.completed", Title: "Trip complete", Body: "Your trip is complete.",
		Recipient: Recipient{Type: RecipientProvider, ID: "prov_1"},
		Data:      data,
	}
}

// VIS-017. A push payload is a way around the provider API's own care: a field
// the provider surface withholds can reach a provider's phone through a data
// bag nobody classified.
func TestAProviderNotificationCannotCarryOrganizationEconomics(t *testing.T) {
	err := providerNotification(map[string]interface{}{
		"booking_id":               "book_1",
		"organization_share_minor": 7500,
	}).Validate()
	if err == nil {
		t.Fatal("a provider notification carrying the organization's share was accepted")
	}
	if !strings.Contains(err.Error(), "Check your notification request") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAProviderNotificationCannotCarryPlatformEconomics(t *testing.T) {
	if err := providerNotification(map[string]interface{}{
		"platform_share_minor": 1000,
	}).Validate(); err == nil {
		t.Fatal("a provider notification carrying the platform's share was accepted")
	}
}

// Nested too, since that is the shape a shallow check misses.
func TestANestedLeakInAProviderNotificationIsRefused(t *testing.T) {
	if err := providerNotification(map[string]interface{}{
		"settlement": map[string]interface{}{"organization_share_minor": 7500},
	}).Validate(); err == nil {
		t.Fatal("a nested organization share reached a provider notification")
	}
}

// Template data is rendered into the message a provider actually reads, so it
// is held to the same rule.
func TestTemplateDataIsHeldToTheSameRule(t *testing.T) {
	request := providerNotification(nil)
	request.TemplateData = map[string]interface{}{"organization_share_minor": 7500}
	if err := request.Validate(); err == nil {
		t.Fatal("template data carrying the organization's share was accepted")
	}
}

// The provider's own figures are exactly what these notifications are for.
func TestAProviderNotificationStillCarriesTheirOwnFigures(t *testing.T) {
	if err := providerNotification(map[string]interface{}{
		"booking_id": "book_1", "own_earning_kobo": 1500, "tips_kobo": 200,
	}).Validate(); err != nil {
		t.Fatalf("a provider's own earning was refused: %v", err)
	}
}

// Every provider role spelling is covered, not just the canonical one -- the
// services address riders and drivers by their own role names.
func TestEveryProviderSpellingIsCovered(t *testing.T) {
	for _, recipientType := range []string{
		RecipientProvider, "truck_provider", "taxi_provider", "dispatch_provider", "rider", "driver",
	} {
		request := providerNotification(map[string]interface{}{"organization_share_minor": 7500})
		request.Recipient.Type = recipientType
		if err := request.Validate(); err == nil {
			t.Errorf("recipient type %q was allowed the organization's share", recipientType)
		}
	}
}

// A customer notification is a different audience with a different contract,
// and this rule does not apply to it.
func TestACustomerNotificationIsNotConstrainedByProviderPolicy(t *testing.T) {
	request := providerNotification(map[string]interface{}{"organization_share_minor": 7500})
	request.Recipient.Type = RecipientCustomer
	if err := request.Validate(); err != nil {
		t.Fatalf("a customer notification was refused a field that is not theirs to hide: %v", err)
	}
}
