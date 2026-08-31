package supportrepositories

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/supportcase"
)

type CaseOutboxRecord struct {
	ID       string
	Event    supportcase.ResolutionEvent
	Attempts int
}

func (r *PostgresSupportRepository) ClaimCaseOutbox(ctx context.Context, limit int) ([]CaseOutboxRecord, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id::text,payload,attempts,status FROM support_case_outbox WHERE status IN ('PENDING','DEAD_LETTER') AND next_attempt_at<=now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CaseOutboxRecord
	for rows.Next() {
		var rec CaseOutboxRecord
		var raw []byte
		var status string
		if err = rows.Scan(&rec.ID, &raw, &rec.Attempts, &status); err != nil {
			return nil, err
		}
		if status == "DEAD_LETTER" {
			if _, err = tx.Exec(ctx, `UPDATE support_case_outbox SET status='PENDING',attempts=0,last_error='automatic dead-letter recovery' WHERE id=$1`, rec.ID); err != nil {
				return nil, err
			}
			rec.Attempts = 0
		}
		if err = json.Unmarshal(raw, &rec.Event); err != nil {
			return nil, err
		}
		result = append(result, rec)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *PostgresSupportRepository) CompleteCaseOutbox(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE support_case_outbox SET status='PUBLISHED',published_at=now(),last_error=NULL WHERE id=$1 AND status='PENDING'`, id)
	return err
}
func (r *PostgresSupportRepository) FailCaseOutbox(ctx context.Context, id string, attempts int, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	next := time.Now().UTC().Add(time.Duration(1<<min(attempts, 8)) * time.Second)
	status := "PENDING"
	if attempts >= 10 {
		status = "DEAD_LETTER"
	}
	_, err := r.db.Exec(ctx, `UPDATE support_case_outbox SET status=$2,attempts=$3,next_attempt_at=$4,last_error=$5 WHERE id=$1 AND status='PENDING'`, id, status, attempts, next, message)
	return err
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
