package main

import (
	"context"
	shared "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/logging"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
)

type groupingBackfillCounts struct{ Eligible, Repaired, AlreadyCorrect, InsufficientEvidence, Failed int }

// Batch resumes a durable named run. dryRun is strictly read-only: its caller
// supplies/retains the returned cursor. A new run re-evaluates UNRECOVERABLE rows
// after canonical evidence is repaired. Each mutation/checkpoint is atomic.
func (p *customerGroupingPublisher) BackfillBatch(ctx context.Context, runID, after string, dryRun bool) (groupingBackfillCounts, string, bool, error) {
	var counts groupingBackfillCounts
	if runID == "" || len(runID) > 100 {
		return counts, after, false, fmt.Errorf("invalid backfill run")
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return counts, after, false, err
	}
	defer tx.Rollback(ctx)
	if !dryRun {
		if _, err = tx.Exec(ctx, `INSERT INTO commerce_customer_grouping_backfill_runs(id) VALUES($1) ON CONFLICT DO NOTHING`, runID); err != nil {
			return counts, after, false, err
		}
		var complete bool
		if err = tx.QueryRow(ctx, `SELECT cursor_ref,completed FROM commerce_customer_grouping_backfill_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&after, &complete); err != nil {
			return counts, after, false, err
		}
		if complete {
			return counts, after, true, tx.Commit(ctx)
		}
	}
	rows, err := tx.Query(ctx, `SELECT id FROM commerce_parent_orders WHERE id>$1 ORDER BY id LIMIT 50`, after)
	if err != nil {
		return counts, after, false, err
	}
	var refs []string
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			rows.Close()
			return counts, after, false, err
		}
		refs = append(refs, ref)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return counts, after, false, err
	}
	for _, ref := range refs {
		if _, err = tx.Exec(ctx, `SELECT id FROM commerce_parent_orders WHERE id=$1 FOR UPDATE`, ref); err != nil {
			return counts, after, false, err
		}
		var present bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_customer_grouping_emissions WHERE parent_order_id=$1)`, ref).Scan(&present); err != nil {
			return counts, after, false, err
		}
		outcome, reason, plan := "ALREADY_CORRECT", "EXISTING_CANONICAL_CHECKPOINT", ""
		if present {
			counts.AlreadyCorrect++
		} else {
			grouping, canonicalPlan, valid, e := canonicalHistoricalGrouping(ctx, tx, ref)
			if e != nil {
				counts.Failed++
				journeyshare.Observe("grouping_backfill_failed", 0)
				return counts, after, false, e
			}
			plan = canonicalPlan
			if !valid {
				counts.InsufficientEvidence++
				outcome = "UNRECOVERABLE"
				reason = "NO_COMPLETE_ORDERED_MISSION_BINDING"
			} else {
				counts.Eligible++
				outcome = "REPAIRED"
				reason = "PERSISTED_ACTIVE_PLAN_SEQUENCE"
				if !dryRun {
					raw, e := json.Marshal(grouping)
					if e != nil {
						return counts, after, false, e
					}
					if e = p.publishTx(ctx, tx, ref, grouping, raw, sha256.Sum256(raw), true); e != nil {
						counts.Failed++
						return counts, after, false, e
					}
					counts.Repaired++
				}
			}
		}
		if !dryRun {
			if _, err = tx.Exec(ctx, `INSERT INTO commerce_customer_grouping_backfill_results(run_id,parent_order_id,outcome,reason_code,plan_id) VALUES($1,$2,$3,$4,$5)`, runID, ref, outcome, reason, plan); err != nil {
				return counts, after, false, err
			}
		}
		after = ref
	}
	done := len(refs) < 50
	if !dryRun {
		if _, err = tx.Exec(ctx, `UPDATE commerce_customer_grouping_backfill_runs SET cursor_ref=$2,eligible=eligible+$3,repaired=repaired+$4,already_correct=already_correct+$5,insufficient_evidence=insufficient_evidence+$6,failed=failed+$7,completed=$8,updated_at=now() WHERE id=$1`, runID, after, counts.Eligible, counts.Repaired, counts.AlreadyCorrect, counts.InsufficientEvidence, counts.Failed, done); err != nil {
			return counts, after, false, err
		}
		err = tx.Commit(ctx)
	}
	for key, value := range map[string]int{"eligible": counts.Eligible, "repaired": counts.Repaired, "already_correct": counts.AlreadyCorrect, "insufficient_evidence": counts.InsufficientEvidence, "failed": counts.Failed} {
		for i := 0; i < value; i++ {
			journeyshare.Observe("grouping_backfill_"+key, 0)
		}
	}
	logging.Structured("info", "commerce_grouping_backfill", map[string]any{"dry_run": dryRun, "eligible": counts.Eligible, "repaired": counts.Repaired, "already_correct": counts.AlreadyCorrect, "insufficient_evidence": counts.InsufficientEvidence, "failed": counts.Failed, "completed": done})
	return counts, after, done, err
}

func canonicalHistoricalGrouping(ctx context.Context, tx pgx.Tx, ref string) (shared.MovementGrouping, string, bool, error) {
	var g shared.MovementGrouping
	g.ParentContextType = "COMMERCE_ORDER"
	g.ParentContextRef = ref
	var plan string
	err := tx.QueryRow(ctx, `SELECT o.customer_id,o.domain,COALESCE(g.current_plan_id,'') FROM commerce_parent_orders o LEFT JOIN commerce_fulfilment_groups g ON g.parent_order_id=o.id AND g.customer_id=o.customer_id WHERE o.id=$1`, ref).Scan(&g.CustomerID, &g.ProductCode, &plan)
	if err != nil {
		return g, plan, false, err
	}
	if plan == "" || (g.ProductCode != "FOOD" && g.ProductCode != "SHOP") {
		return g, plan, false, nil
	}
	// Lock the active pointer against replan while deriving and publishing. The
	// immutable slot sequence, not UUID/date order, is the mission order.
	var lockedPlan string
	if err = tx.QueryRow(ctx, `SELECT current_plan_id FROM commerce_fulfilment_groups WHERE parent_order_id=$1 FOR SHARE`, ref).Scan(&lockedPlan); err != nil {
		return g, plan, false, err
	}
	if lockedPlan != plan {
		return g, plan, false, nil
	}
	rows, err := tx.Query(ctx, `SELECT slot.sequence,slot.owner,slot.authority,
 COALESCE((SELECT string_agg(m.mission_ref,',') FROM commerce_order_missions m
    WHERE m.parent_order_id=$2 AND m.plan_mission_ref=slot.slot_ref AND m.service=slot.authority AND m.status='ACCEPTED'),
   (SELECT string_agg(p.mission_ref,',') FROM commerce_platform_mission_refs p WHERE p.plan_mission_id=slot.id AND p.plan_id=slot.plan_id AND p.authority=slot.authority AND p.link_state='LINKED'),''),
 COALESCE((SELECT string_agg(v.id,',') FROM commerce_vendor_missions v WHERE v.plan_mission_id=slot.id AND v.plan_id=slot.plan_id),'')
 FROM commerce_fulfilment_plan_missions slot JOIN commerce_fulfilment_plan_versions version ON version.id=slot.plan_id AND version.group_id=slot.group_id AND version.status='ACTIVE'
 WHERE slot.plan_id=$1 ORDER BY slot.sequence`, plan, ref)
	if err != nil {
		return g, plan, false, err
	}
	defer rows.Close()
	valid := true
	for rows.Next() {
		var sequence int
		var owner, authority, platform, vendor string
		if err = rows.Scan(&sequence, &owner, &authority, &platform, &vendor); err != nil {
			return g, plan, false, err
		}
		mission := ""
		if owner == "PLATFORM" && (authority == "DISPATCH" || authority == "HAULING" || authority == "TAXI") && vendor == "" {
			mission = platform
		}
		if owner == "VENDOR" && authority == "VENDOR" && platform == "" {
			mission = vendor
		}
		if sequence != len(g.Missions)+1 || mission == "" {
			valid = false
		}
		// More than one binding is ambiguous; never choose a replacement arbitrarily.
		for _, c := range mission {
			if c == ',' {
				valid = false
			}
		}
		g.Missions = append(g.Missions, shared.GroupingMission{FulfilmentGroupRef: mission, MissionSequence: sequence})
	}
	if err = rows.Err(); err != nil {
		return g, plan, false, err
	}
	if len(g.Missions) == 0 {
		valid = false
	}
	for i := range g.Missions {
		g.Missions[i].MissionCount = len(g.Missions)
	}
	if valid && g.Validate() != nil {
		valid = false
	}
	return g, plan, valid, nil
}
