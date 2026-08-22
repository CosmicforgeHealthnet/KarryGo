package inventory

const (
	PermissionInventoryRead = "vendor.inventory.read"

	PermissionInventoryManage = "vendor.inventory.manage"

	PermissionInventoryReceive = "vendor.inventory.receive"

	PermissionInventoryAdjust = "vendor.inventory.adjust"

	PermissionInventoryReserve = "vendor.inventory.reserve"

	PermissionInventoryTransfer = "vendor.inventory.transfer"

	PermissionInventoryStocktake = "vendor.inventory.stocktake"

	PermissionInventoryQuarantine = "vendor.inventory.quarantine"

	PermissionInventoryPolicy = "vendor.inventory.policy"
)

var shopPermissionCompatibility = map[string]string{
	"vendor.shop.inventory.read": PermissionInventoryRead,

	"vendor.shop.inventory.receive": PermissionInventoryReceive,

	"vendor.shop.inventory.adjust": PermissionInventoryAdjust,

	"vendor.shop.inventory.transfer": PermissionInventoryTransfer,

	"vendor.shop.inventory.stocktake": PermissionInventoryStocktake,

	"vendor.shop.inventory.reserve": PermissionInventoryReserve,

	"vendor.shop.inventory.quarantine": PermissionInventoryQuarantine,

	"vendor.shop.inventory.manage_policy": PermissionInventoryPolicy,
}

// GenericPermissionForShopPermission returns the generic Inventory authority
// represented by an existing Shop Inventory permission.
//
// Existing Shop HTTP authorization remains unchanged during compatibility.
func GenericPermissionForShopPermission(
	shopPermission string,
) (
	string,
	bool,
) {
	generic, ok := shopPermissionCompatibility[shopPermission]

	return generic, ok
}

// CompatibleShopPermissions returns Shop Inventory permissions represented by
// one generic Inventory authority.
func CompatibleShopPermissions(
	genericPermission string,
) []string {
	out := make(
		[]string,
		0,
		4,
	)

	for shopPermission, mapped := range shopPermissionCompatibility {

		if mapped != genericPermission {
			continue
		}

		out = append(
			out,
			shopPermission,
		)
	}

	return out
}
