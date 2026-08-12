// Package models holds company staff and IAM domain models.
//
// Corporate staff identities are separate from managed provider identities:
// corporate permissions are never provider permissions, and no identity is
// automatically merged across domains.
package models

import "time"

// Organization-user statuses.
const (
	UserStatusInvited   = "INVITED"
	UserStatusActive    = "ACTIVE"
	UserStatusSuspended = "SUSPENDED"
	UserStatusEnded     = "ENDED"
	UserStatusExpired   = "EXPIRED"
	UserStatusCancelled = "CANCELLED"
)

// Invitation statuses.
const (
	InvitationStatusPending   = "PENDING"
	InvitationStatusAccepted  = "ACCEPTED"
	InvitationStatusExpired   = "EXPIRED"
	InvitationStatusCancelled = "CANCELLED"
)

// Role binding scopes.
const (
	ScopeOrganization = "ORGANIZATION"
	ScopeBranch       = "BRANCH"
	ScopeDepartment   = "DEPARTMENT"
)

// System role codes seeded by migration 000005.
const (
	RoleOrganizationOwner = "ORGANIZATION_OWNER"
	RoleCompanyAdmin      = "COMPANY_ADMIN"
	RoleViewOnly          = "VIEW_ONLY"
)

// SystemOwnerRoleID is the seeded owner role template.
const SystemOwnerRoleID = "rol_system_owner"

// OrganizationUser is a company staff membership.
type OrganizationUser struct {
	ID                     string
	OrganizationID         string
	IdentityID             string
	FullName               string
	JobTitle               *string
	Status                 string
	IsOwner                bool
	PermittedBranchIDs     []string
	PermittedDepartmentIDs []string
	RoleVersion            int64
	InvitedBy              *string
	InvitedAt              *time.Time
	ActivatedAt            *time.Time
	SuspendedAt            *time.Time
	EndedAt                *time.Time
	Version                int64
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// Live reports whether the membership still occupies its organization slot.
func (u OrganizationUser) Live() bool {
	return u.Status == UserStatusInvited || u.Status == UserStatusActive || u.Status == UserStatusSuspended
}

// StaffInvitation is a pending company staff invitation. Only the hash of the
// single-use token is persisted.
type StaffInvitation struct {
	ID                     string
	OrganizationID         string
	OrganizationUserID     *string
	FullName               string
	EmailNormalized        string
	PhoneE164              string
	InvitationTokenHash    string
	RoleIDs                []string
	PermittedBranchIDs     []string
	PermittedDepartmentIDs []string
	Status                 string
	ExpiresAt              time.Time
	DeliveryStatus         string
	NotificationReference  *string
	Attempts               int
	CreatedBy              string
	AcceptedAt             *time.Time
	ConsumedAt             *time.Time
	Version                int64
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// Usable reports whether the invitation can still be accepted.
func (i StaffInvitation) Usable(now time.Time) bool {
	return i.Status == InvitationStatusPending && now.Before(i.ExpiresAt)
}

// Permission is a platform permission catalogue entry.
type Permission struct {
	Code        string
	Category    string
	Description string
	IsActive    bool
}

// Role is a system template (OrganizationID nil) or an organization's custom
// role.
type Role struct {
	ID             string
	OrganizationID *string
	Code           string
	Name           string
	Description    string
	IsSystem       bool
	Status         string
	Permissions    []string
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// RoleBinding grants a role to a membership within a scope.
type RoleBinding struct {
	ID                 string
	OrganizationID     string
	OrganizationUserID string
	RoleID             string
	ScopeType          string
	ScopeID            *string
	EffectiveFrom      time.Time
	ExpiresAt          *time.Time
	GrantedBy          string
	RevokedAt          *time.Time
	RevocationReason   *string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Active reports whether the binding grants authority at the given time.
// Expired or revoked bindings grant nothing.
func (b RoleBinding) Active(now time.Time) bool {
	if b.RevokedAt != nil {
		return false
	}
	if now.Before(b.EffectiveFrom) {
		return false
	}
	if b.ExpiresAt != nil && !now.Before(*b.ExpiresAt) {
		return false
	}
	return true
}
