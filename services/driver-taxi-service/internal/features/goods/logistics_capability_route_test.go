package goods

// The shared cross-authority capability contract must actually be mounted and
// actually be protected.
//
// Vendor's capability gate treats an authority it cannot reach as "no
// refusal", so an unmounted route would not fail loudly -- it would silently
// disable the gate for Taxi Goods and let a multi-pickup mission through to a
// single-pickup authority. That is worth one test.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/logisticscapability"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const capabilityRouteSecret = "taxi-goods-capability-secret"

func newCapabilityRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{
		commercemission.SignatureCaller: []byte(capabilityRouteSecret),
	}, 5*time.Minute)
	// The service is nil-safe for this surface: the capability document is
	// declared from configuration, not from mission state.
	RegisterRoutes(engine.Group("/api/v1/taxi"), &Service{}, verifier, nil, true)
	return engine
}

func TestTaxiGoodsDeclaresSinglePickupTruthfully(t *testing.T) {
	capability := DeclaredLogisticsCapability(true, true, func() time.Time {
		return time.Unix(1_700_000_000, 0).UTC()
	})
	if capability.Authority != logisticscapability.AuthorityTaxi {
		t.Fatalf("authority=%q", capability.Authority)
	}
	if !capability.Available {
		t.Fatal("an enabled Taxi Goods integration must declare itself available")
	}
	// Taxi Goods collects from exactly one place. Declaring otherwise would
	// have Vendor route consolidated orders here.
	if capability.MultiPickupSupported || capability.MaxPickups != 1 {
		t.Fatalf("Taxi Goods must declare single pickup, got multi=%v max=%d",
			capability.MultiPickupSupported, capability.MaxPickups)
	}
	if capability.MaxDropoffs != 1 {
		t.Fatalf("a goods mission has one delivery, got %d", capability.MaxDropoffs)
	}
	if !capability.SecurePickupSupported || !capability.SecureDeliverySupported {
		t.Fatal("Taxi Goods implements protected handover and must say so, or Vendor will never select it for one")
	}

	disabled := DeclaredLogisticsCapability(false, true, nil)
	if disabled.Available || disabled.UnavailableReason == "" {
		t.Fatalf("a disabled integration must declare itself unavailable with a reason: %+v", disabled)
	}
}

// TestTaxiGoodsCapabilityRefusesAMultiPickupCommerceMission is VND-007 at the
// contract level: the declaration is only useful if the matcher actually
// refuses on it.
func TestTaxiGoodsCapabilityRefusesAMultiPickupCommerceMission(t *testing.T) {
	capability := DeclaredLogisticsCapability(true, true, nil)
	answer := logisticscapability.Evaluate(capability, logisticscapability.Requirement{
		PickupCount: 3, DropoffCount: 1,
	})
	if answer.Serviceable {
		t.Fatal("a three-pickup mission must not be serviceable by a single-pickup authority")
	}
	if answer.ReasonCode != logisticscapability.ReasonMultiPickupNotAllowed {
		t.Fatalf("reason=%q", answer.ReasonCode)
	}
	single := logisticscapability.Evaluate(capability, logisticscapability.Requirement{
		PickupCount: 1, DropoffCount: 1, SecurePickup: true, SecureDelivery: true,
	})
	if !single.Serviceable {
		t.Fatalf("a single-pickup protected mission must remain serviceable: %+v", single)
	}
}

func TestTaxiGoodsCapabilityRequiresASignedVendorCaller(t *testing.T) {
	engine := newCapabilityRouter(t)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/v1/taxi"+logisticscapability.Path, nil,
	))
	if recorder.Code == http.StatusOK {
		t.Fatal("capability is internal contract detail and must not be readable unsigned")
	}
}

// TestTheSharedCapabilityPathDoesNotCollideWithTaxisOwn documents why the
// shared contract does not reuse /internal/commerce/capabilities: Taxi already
// serves a differently-shaped document there, and two contracts on one path
// would either collide at registration or answer the wrong shape.
func TestTheSharedCapabilityPathDoesNotCollideWithTaxisOwn(t *testing.T) {
	if strings.HasSuffix(logisticscapability.Path, "/internal/commerce/capabilities") {
		t.Fatal("the shared capability path must not be Taxi's own capabilities path")
	}
	engine := newCapabilityRouter(t)
	var sharedPath, taxiPath bool
	for _, route := range engine.Routes() {
		switch route.Path {
		case "/api/v1/taxi" + logisticscapability.Path:
			sharedPath = true
		case "/api/v1/taxi/internal/commerce/capabilities":
			taxiPath = true
		}
	}
	if !sharedPath || !taxiPath {
		t.Fatalf("both capability documents must be mounted: shared=%v taxi=%v", sharedPath, taxiPath)
	}
}

// Taxi Goods carries one pickup, so at most one can be protected. Declared
// rather than left to the MaxPickups fallback, so the protected bound cannot
// silently widen if the routing bound ever changes.
func TestTaxiGoodsDeclaresOneProtectedPickup(t *testing.T) {
	capability := DeclaredLogisticsCapability(true, true, nil)
	if capability.MaxSecurePickups != 1 || capability.SecurePickupLimit() != 1 {
		t.Fatalf("MaxSecurePickups=%d limit=%d; Taxi Goods carries one pickup",
			capability.MaxSecurePickups, capability.SecurePickupLimit())
	}
}
