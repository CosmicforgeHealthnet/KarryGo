-- Expand the existing Fleet investigation workflow onto other Fleet-owned
-- administrative subjects. These are references to authoritative rows, not
-- generic mutation handles; investigation transitions never change the
-- subject's operational state.
ALTER TABLE fleet_partner_admin_investigations
    ADD COLUMN organization_subject_id text REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    ADD COLUMN asset_id text REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
    ADD COLUMN contract_id text REFERENCES fleet_partner_contracts(id) ON DELETE RESTRICT,
    ADD COLUMN incident_id text REFERENCES fleet_partner_incidents(id) ON DELETE RESTRICT,
    ADD COLUMN route_plan_id text REFERENCES fleet_partner_route_plans(id) ON DELETE RESTRICT,
    ADD COLUMN route_optimization_id text REFERENCES fleet_partner_route_optimization_requests(id) ON DELETE RESTRICT,
    ADD COLUMN sla_measurement_id text REFERENCES fleet_partner_sla_measurements(id) ON DELETE RESTRICT;

ALTER TABLE fleet_partner_admin_investigations
    DROP CONSTRAINT fleet_partner_admin_investigations_subject_kind_check,
    DROP CONSTRAINT fleet_partner_admin_investigation_subject_ck,
    DROP CONSTRAINT fleet_partner_admin_investigations_category_check;

ALTER TABLE fleet_partner_admin_investigations
    ADD CONSTRAINT fleet_partner_admin_investigations_subject_kind_check CHECK (
        subject_kind IN ('shipments', 'private-assignments', 'execution-links',
                         'organizations', 'assets', 'contracts', 'incidents',
                         'route-plans', 'route-optimizations', 'sla-measurements')),
    ADD CONSTRAINT fleet_partner_admin_investigations_category_check CHECK (
        category IN ('CARGO_DAMAGE', 'CARGO_LOSS', 'SAFETY', 'CONDUCT',
                     'SERVICE_FAILURE', 'ONBOARDING', 'ASSET_COMPLIANCE',
                     'CONTRACT_EXPIRY', 'CORPORATE_INCIDENT',
                     'ROUTE_PLANNING', 'SLA_BREACH')),
    ADD CONSTRAINT fleet_partner_admin_investigation_subject_ck CHECK (
        (subject_kind = 'shipments' AND shipment_id = subject_id AND private_assignment_id IS NULL AND execution_link_id IS NULL AND organization_subject_id IS NULL AND asset_id IS NULL AND contract_id IS NULL AND incident_id IS NULL AND route_plan_id IS NULL AND route_optimization_id IS NULL AND sla_measurement_id IS NULL)
     OR (subject_kind = 'private-assignments' AND private_assignment_id = subject_id AND shipment_id IS NULL AND execution_link_id IS NULL AND organization_subject_id IS NULL AND asset_id IS NULL AND contract_id IS NULL AND incident_id IS NULL AND route_plan_id IS NULL AND route_optimization_id IS NULL AND sla_measurement_id IS NULL)
     OR (subject_kind = 'execution-links' AND execution_link_id = subject_id AND shipment_id IS NULL AND private_assignment_id IS NULL AND organization_subject_id IS NULL AND asset_id IS NULL AND contract_id IS NULL AND incident_id IS NULL AND route_plan_id IS NULL AND route_optimization_id IS NULL AND sla_measurement_id IS NULL)
     OR (subject_kind = 'organizations' AND organization_subject_id = subject_id AND organization_id = subject_id AND shipment_id IS NULL AND private_assignment_id IS NULL AND execution_link_id IS NULL AND asset_id IS NULL AND contract_id IS NULL AND incident_id IS NULL AND route_plan_id IS NULL AND route_optimization_id IS NULL AND sla_measurement_id IS NULL)
     OR (subject_kind = 'assets' AND asset_id = subject_id AND shipment_id IS NULL AND private_assignment_id IS NULL AND execution_link_id IS NULL AND organization_subject_id IS NULL AND contract_id IS NULL AND incident_id IS NULL AND route_plan_id IS NULL AND route_optimization_id IS NULL AND sla_measurement_id IS NULL)
     OR (subject_kind = 'contracts' AND contract_id = subject_id AND shipment_id IS NULL AND private_assignment_id IS NULL AND execution_link_id IS NULL AND organization_subject_id IS NULL AND asset_id IS NULL AND incident_id IS NULL AND route_plan_id IS NULL AND route_optimization_id IS NULL AND sla_measurement_id IS NULL)
     OR (subject_kind = 'incidents' AND incident_id = subject_id AND shipment_id IS NULL AND private_assignment_id IS NULL AND execution_link_id IS NULL AND organization_subject_id IS NULL AND asset_id IS NULL AND contract_id IS NULL AND route_plan_id IS NULL AND route_optimization_id IS NULL AND sla_measurement_id IS NULL)
     OR (subject_kind = 'route-plans' AND route_plan_id = subject_id AND shipment_id IS NULL AND private_assignment_id IS NULL AND execution_link_id IS NULL AND organization_subject_id IS NULL AND asset_id IS NULL AND contract_id IS NULL AND incident_id IS NULL AND route_optimization_id IS NULL AND sla_measurement_id IS NULL)
     OR (subject_kind = 'route-optimizations' AND route_optimization_id = subject_id AND shipment_id IS NULL AND private_assignment_id IS NULL AND execution_link_id IS NULL AND organization_subject_id IS NULL AND asset_id IS NULL AND contract_id IS NULL AND incident_id IS NULL AND route_plan_id IS NULL AND sla_measurement_id IS NULL)
     OR (subject_kind = 'sla-measurements' AND sla_measurement_id = subject_id AND shipment_id IS NULL AND private_assignment_id IS NULL AND execution_link_id IS NULL AND organization_subject_id IS NULL AND asset_id IS NULL AND contract_id IS NULL AND incident_id IS NULL AND route_plan_id IS NULL AND route_optimization_id IS NULL));
