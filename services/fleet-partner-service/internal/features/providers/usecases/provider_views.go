package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Read-only managed-provider projections (spec 39.3, FLEET-10.11/10.12):
// availability, branches, vehicles and service roles. None of them owns a
// table. Each is derived from data another part of this service already
// holds -- the membership row, workforce profiles, shifts, approved leave,
// qualifications, asset assignments, live assignments and tracking
// projections -- so there is exactly one source of truth for each fact.

// VisibleMembership loads a membership the actor may see: same organization
// and, when the actor is branch-scoped, a home branch inside that scope (a
// membership with no home branch is organization-wide, matching
// ListMemberships). Anything else is not found, never forbidden, so the
// existence of another organization's or branch's provider is not revealed.
func (s *Service) VisibleMembership(ctx context.Context, tenantCtx tenant.Context, membershipID string) (*models.Membership, error) {
	membership, err := s.membership(ctx, tenantCtx.OrganizationID, membershipID)
	if err != nil {
		return nil, err
	}
	if membership.HomeBranchID != nil && !tenantCtx.BranchPermitted(*membership.HomeBranchID) {
		return nil, apperrors.NotFound("Provider not found.", nil)
	}
	return membership, nil
}

// ---- Branches ----

// ProviderBranch is one branch a provider works from, with the record(s)
// that place them there.
type ProviderBranch struct {
	BranchID string   `json:"branchId"`
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Status   string   `json:"status"`
	Sources  []string `json:"sources"`
}

// Branch sources.
const (
	BranchSourceHome      = "MEMBERSHIP_HOME_BRANCH"
	BranchSourceWorkforce = "WORKFORCE_PROFILE"
)

// ProviderBranches lists the provider's home branch and workforce-profile
// branch. Branches outside the actor's branch scope are omitted.
func (s *Service) ProviderBranches(ctx context.Context, tenantCtx tenant.Context, membershipID string) ([]ProviderBranch, error) {
	if _, err := s.VisibleMembership(ctx, tenantCtx, membershipID); err != nil {
		return nil, err
	}
	rows, err := s.opts.Pool.Query(ctx, `
		SELECT b.id, b.code, b.name, b.type, b.status, src.source
		FROM (
			SELECT home_branch_id AS branch_id, $3::text AS source, 1 AS rank
			FROM fleet_partner_organization_provider_memberships
			WHERE organization_id = $1 AND id = $2 AND home_branch_id IS NOT NULL
			UNION ALL
			SELECT branch_id, $4::text, 2
			FROM fleet_partner_workforce_profiles
			WHERE organization_id = $1 AND membership_id = $2 AND status IN ('ACTIVE', 'INACTIVE')
		) src
		JOIN fleet_partner_branches b ON b.organization_id = $1 AND b.id = src.branch_id
		ORDER BY src.rank, b.name, b.id`,
		tenantCtx.OrganizationID, membershipID, BranchSourceHome, BranchSourceWorkforce)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderBranch{}
	index := map[string]int{}
	for rows.Next() {
		var branch ProviderBranch
		var source string
		if err := rows.Scan(&branch.BranchID, &branch.Code, &branch.Name, &branch.Type, &branch.Status, &source); err != nil {
			return nil, err
		}
		if !tenantCtx.BranchPermitted(branch.BranchID) {
			continue
		}
		if position, seen := index[branch.BranchID]; seen {
			out[position].Sources = append(out[position].Sources, source)
			continue
		}
		branch.Sources = []string{source}
		index[branch.BranchID] = len(out)
		out = append(out, branch)
	}
	return out, rows.Err()
}

// ---- Vehicles ----

// ProviderVehicle is one organization asset allocated to the provider.
type ProviderVehicle struct {
	AssignmentID          string     `json:"assignmentId"`
	AssignmentType        string     `json:"assignmentType"`
	AssignmentStatus      string     `json:"assignmentStatus"`
	StartAt               time.Time  `json:"startAt"`
	EndAt                 *time.Time `json:"endAt,omitempty"`
	ShiftID               *string    `json:"shiftId,omitempty"`
	AssetID               string     `json:"assetId"`
	AssetCode             string     `json:"assetCode"`
	AssetType             string     `json:"assetType"`
	BranchID              string     `json:"branchId"`
	PlatformVehicleID     string     `json:"platformVehicleId,omitempty"`
	RegistrationReference string     `json:"registrationReference,omitempty"`
	Manufacturer          string     `json:"manufacturer,omitempty"`
	Model                 string     `json:"model,omitempty"`
	OperationalStatus     string     `json:"operationalStatus"`
	LifecycleStatus       string     `json:"lifecycleStatus"`
}

// ProviderVehicles lists asset assignments of the provider: current
// (PLANNED/ACTIVE) by default, every assignment when includeHistory is set.
// Assets in branches outside the actor's scope are omitted.
func (s *Service) ProviderVehicles(ctx context.Context, tenantCtx tenant.Context, membershipID string, includeHistory bool) ([]ProviderVehicle, error) {
	if _, err := s.VisibleMembership(ctx, tenantCtx, membershipID); err != nil {
		return nil, err
	}
	rows, err := s.opts.Pool.Query(ctx, `
		SELECT aa.id, aa.assignment_type, aa.status, aa.start_at, aa.end_at, aa.shift_id,
		       a.id, a.asset_code, a.asset_type, a.branch_id, a.platform_vehicle_id,
		       a.registration_reference, a.manufacturer, a.model, a.operational_status, a.lifecycle_status
		FROM fleet_partner_asset_assignments aa
		JOIN fleet_partner_assets a ON a.organization_id = aa.organization_id AND a.id = aa.asset_id
		WHERE aa.organization_id = $1 AND aa.provider_membership_id = $2
		  AND ($3 OR aa.status IN ('PLANNED', 'ACTIVE'))
		ORDER BY aa.start_at DESC, aa.id
		LIMIT 200`, tenantCtx.OrganizationID, membershipID, includeHistory)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderVehicle{}
	for rows.Next() {
		var v ProviderVehicle
		if err := rows.Scan(&v.AssignmentID, &v.AssignmentType, &v.AssignmentStatus, &v.StartAt, &v.EndAt, &v.ShiftID,
			&v.AssetID, &v.AssetCode, &v.AssetType, &v.BranchID, &v.PlatformVehicleID,
			&v.RegistrationReference, &v.Manufacturer, &v.Model, &v.OperationalStatus, &v.LifecycleStatus); err != nil {
			return nil, err
		}
		if !tenantCtx.BranchPermitted(v.BranchID) {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---- Service roles ----

// ServiceRoleCapability is one organization capability of the provider.
type ServiceRoleCapability struct {
	ID             string     `json:"id"`
	ServiceType    string     `json:"serviceType"`
	VehicleClasses []string   `json:"vehicleClasses"`
	OperatingZones []string   `json:"operatingZones"`
	Qualifications []string   `json:"qualifications"`
	ValidFrom      time.Time  `json:"validFrom"`
	ValidUntil     *time.Time `json:"validUntil,omitempty"`
	Status         string     `json:"status"`
	Effective      bool       `json:"effective"`
}

// ProviderServiceRoles is what the organization uses the provider for.
type ProviderServiceRoles struct {
	MembershipID     string                  `json:"membershipId"`
	ServiceCategory  string                  `json:"serviceCategory"`
	ProviderService  string                  `json:"providerService"`
	RelationshipType string                  `json:"relationshipType"`
	EmploymentType   *string                 `json:"employmentType,omitempty"`
	WorkforceRole    *string                 `json:"workforceRole,omitempty"`
	WorkforceStatus  *string                 `json:"workforceStatus,omitempty"`
	Capabilities     []ServiceRoleCapability `json:"capabilities"`
}

// ProviderServiceRoles combines the membership's service category, the
// workforce role and the organization capabilities. A capability is
// "effective" when ACTIVE and inside its validity window right now.
func (s *Service) ProviderServiceRoles(ctx context.Context, tenantCtx tenant.Context, membershipID string) (*ProviderServiceRoles, error) {
	membership, err := s.VisibleMembership(ctx, tenantCtx, membershipID)
	if err != nil {
		return nil, err
	}
	view := &ProviderServiceRoles{MembershipID: membership.ID, ServiceCategory: membership.ServiceCategory,
		ProviderService: membership.ProviderService, RelationshipType: membership.RelationshipType,
		EmploymentType: membership.EmploymentType, Capabilities: []ServiceRoleCapability{}}
	var role, status string
	err = s.opts.Pool.QueryRow(ctx, `SELECT role, status FROM fleet_partner_workforce_profiles
		WHERE organization_id = $1 AND membership_id = $2
		ORDER BY (status = 'ACTIVE') DESC, updated_at DESC LIMIT 1`,
		tenantCtx.OrganizationID, membershipID).Scan(&role, &status)
	switch {
	case err == nil:
		view.WorkforceRole, view.WorkforceStatus = &role, &status
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}
	capabilities, err := s.opts.Providers.ListCapabilities(ctx, tenantCtx.OrganizationID, membershipID)
	if err != nil {
		return nil, err
	}
	now := s.opts.Now().UTC()
	for _, capability := range capabilities {
		effective := capability.Status == "ACTIVE" && !capability.ValidFrom.After(now) &&
			(capability.ValidUntil == nil || capability.ValidUntil.After(now))
		view.Capabilities = append(view.Capabilities, ServiceRoleCapability{
			ID: capability.ID, ServiceType: capability.ServiceType, VehicleClasses: capability.VehicleClasses,
			OperatingZones: capability.OperatingZones, Qualifications: capability.Qualifications,
			ValidFrom: capability.ValidFrom, ValidUntil: capability.ValidUntil, Status: capability.Status,
			Effective: effective,
		})
	}
	return view, nil
}

// LiveMembershipForProvider resolves the one live membership a provider
// identity holds in the given owning provider service (a provider occupies
// at most one live membership per service category). Not found when the
// provider is independent there or their membership has ended.
func (s *Service) LiveMembershipForProvider(ctx context.Context, providerService string, providerID string) (*models.Membership, error) {
	membership, err := s.opts.Providers.FindLiveByProvider(ctx, nil, providerService, providerID)
	if err != nil {
		if errors.Is(err, providerrepos.ErrNotFound) {
			return nil, apperrors.NotFound("Provider membership not found.", nil)
		}
		return nil, err
	}
	return membership, nil
}
