package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type safetyDecline struct {
	OrganizationID, AssignmentID, AssetReference, MembershipID, ReasonCode, Note string
	// Source is PRIVATE_ASSIGNMENT or MARKETPLACE_ASSIGNMENT (spec 11.13,
	// 18.11): an unsafe-vehicle decline starts the same safety workflow
	// whichever kind of company work it was declined from.
	Source string
}

// ReconcileSafetyDeclines consumes the durable Section 15 assignment projection.
// The inbox and source uniqueness constraints make repeated sweeps idempotent.
func (s *Service) ReconcileSafetyDeclines(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch || !s.o.Flags.Inspections || !s.o.Flags.Maintenance {
		return nil
	}
	rows, e := s.o.Pool.Query(ctx, `
		SELECT organization_id, id, vehicle_or_truck_reference, provider_membership_id, decline_reason, decline_note, 'PRIVATE_ASSIGNMENT', declined_at
		FROM fleet_partner_private_assignments a
		WHERE status='DECLINED' AND decline_reason IN('UNSAFE_VEHICLE','VEHICLE_DEFECT') AND vehicle_or_truck_reference<>''
		  AND NOT EXISTS(SELECT 1 FROM fleet_partner_safety_decline_inbox i WHERE i.organization_id=a.organization_id AND i.assignment_id=a.id AND i.reason_code=a.decline_reason)
		UNION ALL
		SELECT r.organization_id, r.assignment_id, a.asset_reference, r.membership_id, r.reason_code, '', 'MARKETPLACE_ASSIGNMENT', r.occurred_at
		FROM fleet_partner_assignment_responses r
		JOIN fleet_partner_provider_assignments a ON a.id = r.assignment_id AND a.organization_id = r.organization_id
		WHERE r.response='DECLINED' AND r.reason_code IN('UNSAFE_VEHICLE','VEHICLE_DEFECT') AND COALESCE(a.asset_reference,'')<>''
		  AND NOT EXISTS(SELECT 1 FROM fleet_partner_safety_decline_inbox i WHERE i.organization_id=r.organization_id AND i.assignment_id=r.assignment_id AND i.reason_code=r.reason_code)
		ORDER BY 8 LIMIT 100`)
	if e != nil {
		return e
	}
	items := []safetyDecline{}
	for rows.Next() {
		var v safetyDecline
		var declinedAt any
		if e = rows.Scan(&v.OrganizationID, &v.AssignmentID, &v.AssetReference, &v.MembershipID, &v.ReasonCode, &v.Note, &v.Source, &declinedAt); e != nil {
			rows.Close()
			return e
		}
		items = append(items, v)
	}
	rows.Close()
	for _, v := range items {
		if e = s.processSafetyDecline(ctx, v); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) processSafetyDecline(ctx context.Context, v safetyDecline) error {
	var alertAsset string
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var assetID string
		h := sha256.Sum256([]byte(v.OrganizationID + "|" + v.AssignmentID + "|" + v.ReasonCode + "|" + v.Note))
		if x := tx.QueryRow(ctx, `SELECT id FROM fleet_partner_assets WHERE organization_id=$1 AND(id=$2 OR platform_vehicle_id=$2)`, v.OrganizationID, v.AssetReference).Scan(&assetID); x != nil {
			if x != pgx.ErrNoRows {
				return x
			}
			// The vehicle is not a Fleet-registered asset (for example a
			// provider's own vehicle on marketplace work). There is nothing
			// here to block; the owning execution service handles its own
			// vehicle safety. Record the decline as handled so one such
			// item can never wedge the sweep.
			_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_safety_decline_inbox(event_id,organization_id,assignment_id,asset_reference,reason_code,payload_hash)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(organization_id,assignment_id,reason_code)DO NOTHING`, ids.New("sdi"), v.OrganizationID, v.AssignmentID, v.AssetReference, v.ReasonCode, hex.EncodeToString(h[:]))
			return x
		}
		alertAsset = assetID
		var inserted string
		if x := tx.QueryRow(ctx, `INSERT INTO fleet_partner_safety_decline_inbox(event_id,organization_id,assignment_id,asset_reference,reason_code,payload_hash)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(organization_id,assignment_id,reason_code)DO NOTHING RETURNING event_id`, ids.New("sdi"), v.OrganizationID, v.AssignmentID, v.AssetReference, v.ReasonCode, hex.EncodeToString(h[:])).Scan(&inserted); x != nil {
			if x == pgx.ErrNoRows {
				return nil
			}
			return x
		}
		// Serialize creation of the organization-owned system template. Different
		// safety declines may be reconciled concurrently by separate workers.
		if _, x := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, v.OrganizationID+":SYSTEM_UNSAFE_VEHICLE"); x != nil {
			return x
		}
		var templateID string
		x := tx.QueryRow(ctx, `SELECT id FROM fleet_partner_inspection_templates WHERE organization_id=$1 AND name='SYSTEM_UNSAFE_VEHICLE' AND status='ACTIVE' ORDER BY created_at LIMIT 1`, v.OrganizationID).Scan(&templateID)
		if x == pgx.ErrNoRows {
			templateID = ids.New("ipt")
			items, _ := json.Marshal([]map[string]any{{"code": "vehicle_safety", "required": true, "evidenceRequired": true, "critical": true}})
			rules, _ := json.Marshal(map[string]any{"criticalFailure": "BLOCK_ASSET"})
			if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_inspection_templates(id,organization_id,name,asset_types,service_types,checklist_items,criticality_rules)VALUES($1,$2,'SYSTEM_UNSAFE_VEHICLE','[]','[]',$3,$4)`, templateID, v.OrganizationID, items, rules); x != nil {
				return x
			}
		} else if x != nil {
			return x
		}
		inspectionID := ids.New("ins")
		if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_inspections(id,organization_id,asset_id,template_id,inspector_membership_id,assignment_id,status,defect_summary)VALUES($1,$2,$3,$4,$5,$6,'DRAFT',$7)`, inspectionID, v.OrganizationID, assetID, templateID, v.MembershipID, v.AssignmentID, mustJSON([]map[string]string{{"reasonCode": v.ReasonCode, "safeNote": v.Note}})); x != nil {
			return x
		}
		workID := ids.New("wor")
		if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_work_orders(id,organization_id,asset_id,source_type,source_reference,priority,required_work)VALUES($1,$2,$3,$5,$4,'HIGH','Inspect provider-reported unsafe vehicle condition')ON CONFLICT(organization_id,source_type,source_reference)DO NOTHING`, workID, v.OrganizationID, assetID, v.AssignmentID, v.Source); x != nil {
			return x
		}
		if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_asset_safety_blocks(id,organization_id,asset_id,source_type,source_reference,status,reason,created_by)VALUES($1,$2,$3,$6,$4,'ACTIVE',$5,'safety-decline-reconciler')ON CONFLICT(organization_id,source_type,source_reference)DO NOTHING`, ids.New("asb"), v.OrganizationID, assetID, v.AssignmentID, v.ReasonCode, v.Source); x != nil {
			return x
		}
		// The reported condition is a critical defect linked to the repair
		// work order (spec 70.9): the asset is released only by authorized
		// return to service.
		var repairID, assetBranch string
		if x = tx.QueryRow(ctx, `SELECT w.id,a.branch_id FROM fleet_partner_work_orders w JOIN fleet_partner_assets a ON a.organization_id=w.organization_id AND a.id=w.asset_id WHERE w.organization_id=$1 AND w.source_type=$2 AND w.source_reference=$3`, v.OrganizationID, v.Source, v.AssignmentID).Scan(&repairID, &assetBranch); x != nil {
			return x
		}
		reporter := tenant.Context{OrganizationID: v.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "safety-decline-reconciler", CorrelationID: v.AssignmentID}
		description := "Provider declined work: " + v.ReasonCode
		if v.Note != "" {
			description += " (" + v.Note + ")"
		}
		if _, _, x = s.insertDefectTx(ctx, tx, reporter, assetBranch, ReportDefectInput{AssetID: assetID, SourceType: "SAFETY_DECLINE", SourceReference: v.AssignmentID, ItemCode: v.ReasonCode, Description: description, Severity: "CRITICAL"}, repairID); x != nil {
			return x
		}
		if _, x = tx.Exec(ctx, `UPDATE fleet_partner_assets SET operational_status='BLOCKED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND operational_status NOT IN('BLOCKED','RETIRED','ARCHIVED')`, v.OrganizationID, assetID); x != nil {
			return x
		}
		t := tenant.Context{OrganizationID: v.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "safety-decline-reconciler", CorrelationID: v.AssignmentID}
		return s.record(ctx, tx, t, "asset.blocked.v1", "asset", assetID, 1, map[string]any{"assetId": assetID, "assignmentId": v.AssignmentID, "reasonCode": v.ReasonCode, "automaticProviderPenalty": false, "inspectionId": inspectionID})
	})
	if e == nil && alertAsset != "" && s.o.Alerts != nil {
		_ = s.o.Alerts.CreateSystemAlert(ctx, v.OrganizationID, "UNSAFE_VEHICLE_DECLINE", alertAsset, v.AssignmentID, "HIGH", "Provider reported an unsafe vehicle; inspection and maintenance review required")
	}
	return e
}
