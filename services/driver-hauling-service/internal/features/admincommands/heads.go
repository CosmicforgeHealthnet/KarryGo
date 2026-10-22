package admincommands

import (
	"context"
	"errors"

	"cosmicforge/logistics/shared/go/admincommand"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// readHead returns the subject's administrative standing version (0 when the
// owner has never decided anything about it). lock takes the row lock.
func readHead(ctx context.Context, tx pgx.Tx, kind, id string, lock bool) (int64, string, bool, error) {
	query := `SELECT version,state FROM hauling_admin_intervention_heads WHERE subject_kind=$1 AND subject_id=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	var version int64
	var state string
	err := tx.QueryRow(ctx, query, kind, id).Scan(&version, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return version, state, true, nil
}

// fence locks the subject head and enforces the expected version.
func fence(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (string, error) {
	version, state, _, err := readHead(ctx, tx, in.SubjectKind, in.SubjectID, true)
	if err != nil {
		return "", err
	}
	if version != in.ExpectedVersion {
		return "", admincommand.ErrConflict
	}
	return state, nil
}

// advanceHead writes ExpectedVersion+1 under the version fence.
func advanceHead(ctx context.Context, tx pgx.Tx, in admincommand.Execution, state, organization string) error {
	var tag pgconn.CommandTag
	var err error
	if in.ExpectedVersion == 0 {
		tag, err = tx.Exec(ctx, `INSERT INTO hauling_admin_intervention_heads(subject_kind,subject_id,version,state,organization_id,last_decision_id) VALUES($1,$2,1,$3,$4,$5) ON CONFLICT DO NOTHING`, in.SubjectKind, in.SubjectID, state, organization, in.DecisionID)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE hauling_admin_intervention_heads SET version=version+1,state=$4,organization_id=$5,last_decision_id=$6,updated_at=now() WHERE subject_kind=$1 AND subject_id=$2 AND version=$3`, in.SubjectKind, in.SubjectID, in.ExpectedVersion, state, organization, in.DecisionID)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return admincommand.ErrConflict
	}
	return nil
}

// bumpHead advances another subject's standing as a side effect of a case
// decision (an inspection FAIL restricts its truck), so any in-flight command
// fenced on the truck's old standing is refused as stale.
func bumpHead(ctx context.Context, tx pgx.Tx, kind, id, state, organization, decision string) error {
	_, err := tx.Exec(ctx, `INSERT INTO hauling_admin_intervention_heads(subject_kind,subject_id,version,state,organization_id,last_decision_id) VALUES($1,$2,1,$3,$4,$5)
 ON CONFLICT (subject_kind,subject_id) DO UPDATE SET version=hauling_admin_intervention_heads.version+1,state=EXCLUDED.state,organization_id=EXCLUDED.organization_id,last_decision_id=EXCLUDED.last_decision_id,updated_at=now()`, kind, id, state, organization, decision)
	return err
}
