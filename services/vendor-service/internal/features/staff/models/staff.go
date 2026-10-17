// Package staffmodels implements Vendor Service's Staff IAM domain rules:
// staff lifecycle, roles, permissions, and role assignments. Authorization is
// always by explicit capability string ("vendor.inventory.stock.read"-shape),
// never by checking a role NAME — see
// docs/architecture/09_SECURITY_AND_AUTHORITY_MODEL.md and this Section 5
// task's own non-negotiable rule 16.
package staffmodels

import "time"

const (
	StaffStatusActive    = "active"
	StaffStatusSuspended = "suspended"
)

// Staff binds a vendor_user (see vendorauthrepositories.User) to one vendor
// business. VendorID is an opaque text reference — Section 6 (Business/
// Branch) does not exist yet; this is intentionally not a foreign key.
type Staff struct {
	ID              string
	VendorID        string
	UserID          string
	Status          string
	SuspendedAt     *time.Time
	SuspendedReason *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int64
	// Contact the staff member signs in with (read-only, from vendor_users).
	Phone string
	Email string
}

func (s Staff) IsActive() bool {
	return s.Status == StaffStatusActive
}

// CanTransitionTo enforces the only two legal transitions: active<->suspended.
// Suspending an already-suspended staff member, or reactivating an
// already-active one, is refused as a conflict rather than silently
// succeeding — the caller should treat that refusal as "nothing to do", not
// retry indefinitely.
func (s Staff) CanTransitionTo(target string) bool {
	switch target {
	case StaffStatusSuspended:
		return s.Status == StaffStatusActive
	case StaffStatusActive:
		return s.Status == StaffStatusSuspended
	default:
		return false
	}
}
