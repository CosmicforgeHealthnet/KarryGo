package goods

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
