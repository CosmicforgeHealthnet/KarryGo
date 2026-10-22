package adminresources

import (
	"context"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "fleet-partner-service"

func Source() *adminowner.SQLSource {
	return adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"shipments":           {Table: "fleet_partner_shipments", VersionColumn: "version", StateColumn: "status", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "order_id"}, Dimensions: map[string]string{"planning": "status", "custody": "", "return": ""}, References: map[string]string{"order": "order_id", "branch": "branch_id"}},
		"private-assignments": {Table: "fleet_partner_private_assignments", VersionColumn: "version", StateColumn: "status", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "private_order_id", "shipment_id", "work_origin"}, Dimensions: map[string]string{"execution_service": "execution_service", "work_origin": "work_origin"}, References: map[string]string{"provider": "provider_id", "vehicle": "vehicle_or_truck_reference", "shipment": "shipment_id", "order": "private_order_id"}},
		// Fleet-owned standalone investigations. Status is the case state; the
		// organization is the one copied from the subject's authoritative row.
		"investigations":  {Table: "fleet_partner_admin_investigations", ReferenceExpression: "t.case_ref", VersionColumn: "version", StateColumn: "state", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "subject_kind", "subject_id"}, Dimensions: map[string]string{"subject_kind": "subject_kind", "category": "category", "severity": "severity", "occurred_on": "occurred_on", "finding": "finding", "closure": "closure"}, References: map[string]string{"organization": "organization_id", "shipment": "shipment_id", "private_assignment": "private_assignment_id", "execution_link": "execution_link_id"}},
		"execution-links": {Table: "fleet_partner_private_execution_operations", VersionColumn: "version", StateColumn: "status", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "private_order_id", "shipment_id", "assignment_id"}, Dimensions: map[string]string{"execution_service": "execution_service"}, References: map[string]string{"provider": "executing_provider_id", "vehicle": "vehicle_or_truck_reference", "shipment": "shipment_id", "assignment": "assignment_id", "response": "provider_response_id", "canonical_mission": "canonical_trip_reference"}},
	})
}
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	profile, _ := adminowner.ForOwner(Owner)
	return adminowner.FromEnvironment(ctx, db, Source(), profile.Definitions, Owner, profile.Prefix, profile.Path)
}
