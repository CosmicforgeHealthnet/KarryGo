package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// InvitationStore lists the repositories.InvitationRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type InvitationStore interface {
	Cancel(ctx context.Context, q database.Querier, organizationID string, invitationID string) error
	Create(ctx context.Context, q database.Querier, organizationID string, invitation *models.StaffInvitation) error
	GetByID(ctx context.Context, organizationID string, invitationID string) (*models.StaffInvitation, error)
	List(ctx context.Context, organizationID string, limit int) ([]models.StaffInvitation, error)
	LockByTokenHash(ctx context.Context, q database.Querier, tokenHash string) (*models.StaffInvitation, error)
	MarkAccepted(ctx context.Context, q database.Querier, invitationID string, memberID string) error
	RecordDelivery(ctx context.Context, invitationID string, status string, reference string) error
	RotateToken(ctx context.Context, q database.Querier, organizationID string, invitationID string, tokenHash string, expiresAt any) error
}

// MemberStore lists the repositories.MemberRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type MemberStore interface {
	BumpRoleVersion(ctx context.Context, q database.Querier, organizationID string, memberID string) error
	Create(ctx context.Context, q database.Querier, organizationID string, member *models.OrganizationUser) error
	Directory(ctx context.Context, organizationID string, memberID string, limit int) ([]models.StaffDirectoryEntry, error)
	DirectoryPage(ctx context.Context, organizationID string, limit int, afterCreatedAt *time.Time, afterID string) ([]models.StaffDirectoryEntry, error)
	GetByID(ctx context.Context, organizationID string, memberID string) (*models.OrganizationUser, error)
	GetByIdentity(ctx context.Context, organizationID string, identityID string) (*models.OrganizationUser, error)
	List(ctx context.Context, query tenant.Query) ([]models.OrganizationUser, error)
	ListForIdentity(ctx context.Context, identityID string) ([]models.OrganizationUser, error)
	UpdateProfile(ctx context.Context, q database.Querier, organizationID string, memberID string, fullName string, jobTitle *string, branches []string, departments []string, expectedVersion int64) error
	UpdateStatus(ctx context.Context, q database.Querier, organizationID string, memberID string, status string, expectedVersion int64) error
}

// RoleStore lists the repositories.RoleRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type RoleStore interface {
	ArchiveRole(ctx context.Context, q database.Querier, organizationID string, roleID string, expectedVersion int64) error
	CreateBinding(ctx context.Context, q database.Querier, organizationID string, binding *models.RoleBinding) error
	CreateRole(ctx context.Context, q database.Querier, organizationID string, role *models.Role, permissions []string) error
	GetRole(ctx context.Context, organizationID string, roleID string) (*models.Role, error)
	ListBindings(ctx context.Context, organizationID string, memberID string) ([]models.RoleBinding, error)
	ListPermissions(ctx context.Context) ([]models.Permission, error)
	ListRoles(ctx context.Context, organizationID string) ([]models.Role, error)
	RevokeBinding(ctx context.Context, q database.Querier, organizationID string, bindingID string, reason string) (string, error)
	UpdateRole(ctx context.Context, q database.Querier, organizationID string, roleID string, name string, description string, permissions []string, expectedVersion int64) error
}
