package movementrepositories

import (
	"context"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/events"
)

// Commerce grouping is applied to the movement projection, never merged into
// it as state. A grouping row may arrive before or after the movements it
// describes, so it is stored on its own and applied to whatever is already
// projected; a movement that arrives later reads it at projection time.

// ApplyGrouping stores one purchase's grouping and decorates the movements it
// names. It returns how many projected movements it touched.
func (r *PostgresRepository) ApplyGrouping(ctx context.Context, envelope events.Envelope, grouping events.MovementGrouping) (string, int, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var seen bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer_movement_event_inbox WHERE event_id=$1)`, envelope.EventID).Scan(&seen); err != nil {
		return "", 0, err
	}
	if seen {
		return "DUPLICATE", 0, nil
	}
	applied := 0
	stale := 0
	for _, mission := range grouping.Missions {
		tag, err := tx.Exec(ctx, `
			INSERT INTO customer_movement_groupings (customer_id, fulfilment_group_ref, parent_context_type, parent_context_ref,
				fulfilment_plan_ref, mission_sequence, mission_count, source_version, last_event_id, product_code)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''))
			ON CONFLICT (fulfilment_group_ref) DO UPDATE SET customer_id=EXCLUDED.customer_id,
				parent_context_type=EXCLUDED.parent_context_type, parent_context_ref=EXCLUDED.parent_context_ref,
				fulfilment_plan_ref=EXCLUDED.fulfilment_plan_ref, mission_sequence=EXCLUDED.mission_sequence,
				mission_count=EXCLUDED.mission_count, source_version=EXCLUDED.source_version,
				last_event_id=EXCLUDED.last_event_id, product_code=COALESCE(EXCLUDED.product_code,customer_movement_groupings.product_code), updated_at=NOW()
			WHERE customer_movement_groupings.source_version < EXCLUDED.source_version
			AND customer_movement_groupings.customer_id=EXCLUDED.customer_id`,
			grouping.CustomerID, mission.FulfilmentGroupRef, grouping.ParentContextType, grouping.ParentContextRef,
			grouping.FulfilmentPlanRef, mission.MissionSequence, mission.MissionCount, envelope.SourceVersion, envelope.EventID, grouping.ProductCode)
		if err != nil {
			return "", 0, err
		}
		if tag.RowsAffected() == 0 {
			stale++
			continue
		}
		// Only the grouping columns are touched. State, timestamps and
		// replacement counts belong to the executing authority alone.
		tag, err = tx.Exec(ctx, `
			UPDATE customer_movement_history_projection
			   SET parent_context_type=$2, parent_context_ref=$3, fulfilment_plan_ref=NULLIF($4,''),
			       mission_sequence=$5, mission_count=$6, product_code=COALESCE(NULLIF($8,''),product_code), updated_at=NOW()
			 WHERE fulfilment_group_ref=$1 AND customer_id=$7`,
			mission.FulfilmentGroupRef, grouping.ParentContextType, grouping.ParentContextRef,
			grouping.FulfilmentPlanRef, mission.MissionSequence, mission.MissionCount, grouping.CustomerID, grouping.ProductCode)
		if err != nil {
			return "", 0, err
		}
		applied += int(tag.RowsAffected())
	}
	outcome := "APPLIED"
	if applied == 0 && stale > 0 {
		outcome = "STALE"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO customer_movement_event_inbox (event_id, source_service, event_name, authority, source_ref, source_version, outcome)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (event_id) DO NOTHING`,
		envelope.EventID, envelope.SourceService, envelope.EventName, "COMMERCE", grouping.ParentContextRef, envelope.SourceVersion, outcome); err != nil {
		return "", 0, err
	}
	return outcome, applied, tx.Commit(ctx)
}

// ProjectedVersions reports the source_version this projection holds for each
// requested movement of one authority. It is the reconciliation answer an
// authority needs to republish only what is actually missing or behind,
// instead of replaying its whole history.
func (r *PostgresRepository) ProjectedVersions(ctx context.Context, authority string, sourceRefs []string) (map[string]int64, error) {
	versions := map[string]int64{}
	if len(sourceRefs) == 0 {
		return versions, nil
	}
	query := `SELECT source_ref, source_version FROM customer_movement_history_projection WHERE authority=$1 AND source_ref = ANY($2)`
	if authority == "COMMERCE" {
		query = `SELECT parent_context_ref,max(source_version) FROM customer_movement_groupings WHERE $1='COMMERCE' AND parent_context_ref=ANY($2) GROUP BY parent_context_ref`
	}
	rows, err := r.db.Query(ctx, query, authority, sourceRefs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ref string
		var version int64
		if err = rows.Scan(&ref, &version); err != nil {
			return nil, err
		}
		versions[ref] = version
	}
	return versions, rows.Err()
}

// GroupingFor reads the stored grouping of one fulfilment group, if Vendor
// has published it.
func (r *PostgresRepository) GroupingFor(ctx context.Context, q pgx.Tx, customerID, groupRef string) (events.GroupingMission, string, string, string, bool, error) {
	var mission events.GroupingMission
	var parentType, parentRef, planRef string
	err := q.QueryRow(ctx, `SELECT parent_context_type, parent_context_ref, fulfilment_plan_ref, mission_sequence, mission_count
		FROM customer_movement_groupings WHERE fulfilment_group_ref=$1 AND customer_id=$2`, groupRef, customerID).
		Scan(&parentType, &parentRef, &planRef, &mission.MissionSequence, &mission.MissionCount)
	if err == pgx.ErrNoRows {
		return events.GroupingMission{}, "", "", "", false, nil
	}
	if err != nil {
		return events.GroupingMission{}, "", "", "", false, err
	}
	mission.FulfilmentGroupRef = groupRef
	return mission, parentType, parentRef, planRef, true, nil
}
