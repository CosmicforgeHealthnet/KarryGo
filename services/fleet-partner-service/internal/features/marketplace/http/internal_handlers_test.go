package http

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func fakeGinContextWithCaller(caller string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("service_name", caller)
	return c
}

// TestCallerRawProviderService_ReturnsRawNameNotAbbreviation pins the same
// fix as documents/http's requireProviderCaller test: callerProviderService
// (used by ListProviderOffers/ListProviderAssignments) correctly returns the
// "DISPATCH"/"TAXI"/"HAULING" abbreviation matching marketplace's own
// assignments/offers vocabulary, but callerRawProviderService (used by
// ListProviderMemberships) must return the caller's raw HMAC service name,
// because fleet_partner_organization_provider_memberships.provider_service
// is populated with that raw name at membership creation
// (platformclients.OwningService), never the abbreviation.
func TestCallerRawProviderService_ReturnsRawNameNotAbbreviation(t *testing.T) {
	handler := &InternalHandler{allowedCallers: map[string]bool{"driver-hauling-service": true}}
	got, err := handler.callerRawProviderService(fakeGinContextWithCaller("driver-hauling-service"))
	if err != nil {
		t.Fatalf("callerRawProviderService: %v", err)
	}
	if got != "driver-hauling-service" {
		t.Fatalf("got %q, want the raw caller name %q (not an abbreviation like %q)", got, "driver-hauling-service", "HAULING")
	}
}

func TestCallerProviderService_ReturnsMarketplaceAbbreviation(t *testing.T) {
	handler := &InternalHandler{allowedCallers: map[string]bool{"driver-hauling-service": true}}
	got, err := handler.callerProviderService(fakeGinContextWithCaller("driver-hauling-service"))
	if err != nil {
		t.Fatalf("callerProviderService: %v", err)
	}
	if got != "HAULING" {
		t.Fatalf("got %q, want the marketplace abbreviation %q", got, "HAULING")
	}
}

func TestCallerRawProviderService_UnknownCallerRefused(t *testing.T) {
	handler := &InternalHandler{allowedCallers: map[string]bool{"driver-hauling-service": true}}
	if _, err := handler.callerRawProviderService(fakeGinContextWithCaller("some-unrelated-service")); err == nil {
		t.Fatal("an unallowlisted caller must be refused")
	}
}
