package adminowner

import (
	"testing"
	"time"
)

func TestCustodyWireRejectsDuplicateNativeIdentities(t *testing.T) {
	fresh := func() *CustodyInventory {
		return &CustodyInventory{ContractVersion: "custody.inventory.v1", Authority: "CUSTOMER", Mode: "MULTI_MANIFEST", Stops: []CustodyStop{{Reference: "stop-1", Type: "PICKUP", Sequence: 1, Status: "PENDING", Version: 1}}, Manifests: []CustodyManifest{{Reference: "manifest-1", ManifestVersion: 1, Authority: "CUSTOMER", PackageCount: 1, Status: "REGISTERED", Version: 1}}, Packages: []CustodyPackage{{Reference: "package-1", Manifest: "manifest-1", ManifestVersion: 1, PackageCount: 1, Status: "EXPECTED"}}, Holdings: []CustodyHolding{{Reference: "holding-1", Manifest: "manifest-1", ManifestVersion: 1, Custodian: "provider-1", AcquiredVia: "PICKUP", AcquiredAt: time.Now()}}, Transfers: []CustodyTransfer{{Operation: "operation-1", Manifest: "manifest-1", ManifestVersion: 1, From: "provider-1", To: "provider-2", PackageCount: 1, At: time.Now()}}}
	}
	for name, mutate := range map[string]func(*CustodyInventory){
		"stop":     func(c *CustodyInventory) { c.Stops = append(c.Stops, c.Stops[0]) },
		"sequence": func(c *CustodyInventory) { x := c.Stops[0]; x.Reference = "another-stop"; c.Stops = append(c.Stops, x) },
		"manifest": func(c *CustodyInventory) { c.Manifests = append(c.Manifests, c.Manifests[0]) },
		"package":  func(c *CustodyInventory) { c.Packages = append(c.Packages, c.Packages[0]) },
		"holding":  func(c *CustodyInventory) { c.Holdings = append(c.Holdings, c.Holdings[0]) },
		"transfer": func(c *CustodyInventory) { c.Transfers = append(c.Transfers, c.Transfers[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			c := fresh()
			if err := validateCustodyInventory(c); err != nil {
				t.Fatal(err)
			}
			mutate(c)
			if err := validateCustodyInventory(c); err == nil {
				t.Fatal("duplicate native identity accepted")
			}
		})
	}
	// Opaque references permit slashes and colons: tuple identities must not
	// collide when a separator appears inside a legitimate source reference.
	c := fresh()
	c.Transfers[0].Operation = "operation/a"
	c.Transfers[0].Manifest = "manifest"
	x := c.Transfers[0]
	x.Operation = "operation"
	x.Manifest = "a/manifest"
	c.Transfers = append(c.Transfers, x)
	if err := validateCustodyInventory(c); err != nil {
		t.Fatal("distinct native tuples collided", err)
	}
}
