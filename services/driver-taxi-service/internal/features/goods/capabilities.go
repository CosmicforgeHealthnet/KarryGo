package goods

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/logisticscapability"
)

// Capabilities is Taxi Goods' truthful statement of what it can actually do
// (Section 8 TAX-026).
//
// It deliberately does NOT claim parity with Dispatch. Taxi Goods is a
// single-pickup authority: a mission carries one Pickup point (see
// Mission.Pickup), so MultiPickupSupported is false and MaxPickups is 1.
// Reporting otherwise would let Vendor route a multi-pickup order to an
// authority that cannot execute it, and the failure would surface only after
// a customer order had already been taken.
type Capabilities struct {
	Authority            string `json:"authority"`
	MultiPickupSupported bool   `json:"multiPickupSupported"`
	MaxPickups           int    `json:"maxPickups"`
	// CustodyEvidenceSupported and SecureHandoverSupported are true: both are
	// implemented (custody manifests, pickup/delivery credentials).
	CustodyEvidenceSupported bool `json:"custodyEvidenceSupported"`
	SecureHandoverSupported  bool `json:"secureHandoverSupported"`
	// ProviderSelectionByVendor is false and always will be: Taxi selects the
	// provider, never the Vendor.
	ProviderSelectionByVendor bool `json:"providerSelectionByVendor"`
	// CorporateProviderSupported reports whether an organization-managed
	// provider can execute Taxi Goods work in this deployment.
	CorporateProviderSupported bool `json:"corporateProviderSupported"`
}

// DeclaredCapabilities builds the capability statement.
func DeclaredCapabilities(corporateProviderSupported bool) Capabilities {
	return Capabilities{
		Authority:                  "TAXI_GOODS",
		MultiPickupSupported:       false,
		MaxPickups:                 1,
		CustodyEvidenceSupported:   true,
		SecureHandoverSupported:    true,
		ProviderSelectionByVendor:  false,
		CorporateProviderSupported: corporateProviderSupported,
	}
}

// capabilities answers the Vendor-facing capability read.
func (h *Handler) capabilities(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    DeclaredCapabilities(h.corporateProviderSupported),
	})
}

// capabilityVersion increments when the declaration below changes.
const capabilityVersion = 1

// DeclaredLogisticsCapability restates Taxi Goods' capability in the shared
// cross-authority vocabulary (VND-007).
//
// It is derived from the SAME facts as Capabilities above rather than a
// second hand-maintained copy: single pickup because a mission carries one
// Pickup point, secure handover and custody because both are implemented.
// Reporting multi-pickup here would let Vendor route a consolidated order to
// an authority that physically cannot collect from two places.
func DeclaredLogisticsCapability(enabled, corporateProviderSupported bool, now func() time.Time) logisticscapability.Capability {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	declared := DeclaredCapabilities(corporateProviderSupported)
	capability := logisticscapability.Capability{
		Authority: logisticscapability.AuthorityTaxi,
		Version:   capabilityVersion,
		Available: enabled,

		MultiPickupSupported: declared.MultiPickupSupported,
		MaxPickups:           int64(declared.MaxPickups),
		MultiStopSupported:   false,
		MaxDropoffs:          1,

		SecurePickupSupported:      declared.SecureHandoverSupported,
		SecureDeliverySupported:    declared.SecureHandoverSupported,
		CustodyEvidenceSupported:   declared.CustodyEvidenceSupported,
		CorporateProviderSupported: declared.CorporateProviderSupported,

		VehicleClasses:    []string{"CAR"},
		HandlingSupported: []string{"FRAGILE", "UPRIGHT", "HIGH_VALUE"},
		ObservedAt:        now().UTC(),
	}
	if !enabled {
		capability.UnavailableReason = "TAXI_GOODS_DELIVERY_DISABLED"
	}
	return capability
}

// LogisticsCapabilityProvider adapts the declaration to the shared server
// helper.
type LogisticsCapabilityProvider struct {
	Enabled                    bool
	CorporateProviderSupported bool
	Now                        func() time.Time
}

// DeclaredCapability satisfies logisticscapability.Provider.
func (p LogisticsCapabilityProvider) DeclaredCapability() logisticscapability.Capability {
	return DeclaredLogisticsCapability(p.Enabled, p.CorporateProviderSupported, p.Now)
}
