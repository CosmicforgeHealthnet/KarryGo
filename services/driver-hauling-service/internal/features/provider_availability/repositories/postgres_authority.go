package availabilityrepositories

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthorityStore is the durable availability source used by eligibility.
// Redis remains only the short-lived geospatial/matching accelerator.
type AuthorityStore interface {
	SetAuthorityState(context.Context, string, string, bool, string) error
}
type PostgresAuthorityStore struct{ db *pgxpool.Pool }

func NewPostgresAuthorityStore(db *pgxpool.Pool) *PostgresAuthorityStore {
	return &PostgresAuthorityStore{db: db}
}
func (s *PostgresAuthorityStore) SetAuthorityState(ctx context.Context, providerID, state string, shift bool, reason string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO hauling_provider_availability(provider_id,state,shift_open,changed_by,reason_code) VALUES($1,$2,$3,'provider',$4) ON CONFLICT(provider_id) DO UPDATE SET state=excluded.state,shift_open=excluded.shift_open,version=hauling_provider_availability.version+1,changed_at=now(),changed_by=excluded.changed_by,reason_code=excluded.reason_code`, providerID, state, shift, reason)
	return err
}
