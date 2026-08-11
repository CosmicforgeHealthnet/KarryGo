package staffmodels

import "time"

// Permission is a global capability-string catalogue entry
// ("vendor.inventory.manage"). Never vendor-scoped.
type Permission struct {
	ID          string
	Code        string
	Description string
	Category    string
	IsHighRisk  bool
}

// Role is either a system template (VendorID == nil, IsSystem true, one of
// the 11 default role families) or a vendor's own custom role (VendorID set).
type Role struct {
	ID          string
	VendorID    *string
	Code        string
	Name        string
	Description string
	IsSystem    bool
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r Role) IsCustom() bool {
	return !r.IsSystem
}

// Assignment binds a staff member to a role, optionally scoped to one branch
// (BranchID nil == vendor-wide). Soft-revoked (RevokedAt set) rather than
// deleted, preserving assignment history for audit.
type Assignment struct {
	ID         string
	StaffID    string
	RoleID     string
	BranchID   *string
	AssignedBy *string
	CreatedAt  time.Time
	RevokedAt  *time.Time
	RevokedBy  *string
}

func (a Assignment) IsActive() bool {
	return a.RevokedAt == nil
}

// DefaultRoleCodes lists the 11 approved role families, for validation and
// documentation. Custom roles must never collide with these codes at the
// system-role level — the migration's partial unique index
// (vendor_roles_system_code_unique) already enforces this at vendor_id IS
// NULL; a vendor's own custom role is scoped by vendor_id so it may reuse a
// system code without conflict (it is simply a different row).
var DefaultRoleCodes = []string{
	"owner", "general_manager", "branch_manager", "kitchen_pastry", "shop_inventory",
	"procurement", "crm_support", "marketing", "finance", "vendor_rider", "analyst_auditor",
}

// OwnerOnlyPermissionCodes are permissions no role other than Owner may ever
// hold — even a General Manager, and even a vendor's own custom role. This is
// the platform invariant enforced by RoleService.SetPermissions /
// AssignRole: no actor may grant a permission it is not itself authorized to
// delegate, and no custom or non-Owner system role may carry an owner-only
// capability at all, regardless of who is creating it.
var OwnerOnlyPermissionCodes = map[string]bool{
	"vendor.owner.transfer": true,
}
