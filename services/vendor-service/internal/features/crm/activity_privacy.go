package crm

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Interactions retain minimum lawful business history on redaction, but the
// display flag removes their subject/note from every ordinary CRM read. This
// separates retention from operational visibility without pretending a hash
// is erasure.
type interactionPrivacyHandler struct{ service *Service }

func (interactionPrivacyHandler) Domain() string                                         { return "interactions" }
func (interactionPrivacyHandler) Restrict(context.Context, pgx.Tx, string, string) error { return nil }
func (interactionPrivacyHandler) MarketingSuppress(context.Context, pgx.Tx, string, string) error {
	return nil
}
func (interactionPrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	_, err := tx.Exec(ctx, `UPDATE crm_interactions SET display_suppressed=true,updated_at=now()
		WHERE business_id=$1 AND customer_profile_id=$2 AND display_suppressed=false`, businessID, profileID)
	return err
}
func (interactionPrivacyHandler) IdentityMerge(ctx context.Context, tx pgx.Tx, businessID, sourceProfileID, targetProfileID string) error {
	_, err := tx.Exec(ctx, `UPDATE crm_interactions SET customer_profile_id=$1,updated_at=now()
		WHERE business_id=$2 AND customer_profile_id=$3`, targetProfileID, businessID, sourceProfileID)
	return err
}

type taskPrivacyHandler struct{ service *Service }

func (taskPrivacyHandler) Domain() string                                         { return "tasks" }
func (taskPrivacyHandler) Restrict(context.Context, pgx.Tx, string, string) error { return nil }
func (taskPrivacyHandler) MarketingSuppress(context.Context, pgx.Tx, string, string) error {
	return nil
}
func (h taskPrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	rows, err := tx.Query(ctx, `UPDATE crm_tasks SET
		status=CASE WHEN status IN ('OPEN','IN_PROGRESS') THEN 'CANCELLED' ELSE status END,
		cancelled_at=CASE WHEN status IN ('OPEN','IN_PROGRESS') THEN now() ELSE cancelled_at END,
		display_suppressed=true,updated_at=now(),
		version=CASE WHEN status IN ('OPEN','IN_PROGRESS') THEN version+1 ELSE version END
		WHERE business_id=$1 AND customer_profile_id=$2 AND display_suppressed=false
		RETURNING `+taskCols, businessID, profileID)
	if err != nil {
		return err
	}
	var tasks []Task
	for rows.Next() {
		task, scanErr := scanTask(rows)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		tasks = append(tasks, task)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	actor := Actor{VendorID: businessID}
	for _, task := range tasks {
		// Only an actionable task receives a new version and therefore a new
		// lifecycle snapshot. Already-terminal tasks are merely hidden.
		if task.Status == TaskCancelled && task.CancelledAt != nil {
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_task_history
				WHERE business_id=$1 AND task_id=$2 AND task_version=$3)`, businessID, task.ID, task.Version).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				if err = h.service.appendTaskHistory(ctx, tx, actor, task, "PRIVACY_SUPPRESSED"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (taskPrivacyHandler) IdentityMerge(ctx context.Context, tx pgx.Tx, businessID, sourceProfileID, targetProfileID string) error {
	_, err := tx.Exec(ctx, `UPDATE crm_tasks SET customer_profile_id=$1,updated_at=now()
		WHERE business_id=$2 AND customer_profile_id=$3`, targetProfileID, businessID, sourceProfileID)
	return err
}
