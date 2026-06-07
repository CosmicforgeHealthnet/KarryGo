package availability

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	EnsureAvailability(ctx context.Context, providerID string) (Availability, error)
	GetAvailability(ctx context.Context, providerID string) (Availability, bool, error)
	SetVerifiedToGoOnline(ctx context.Context, providerID string, verified bool) (Availability, error)
	SetOnline(ctx context.Context, providerID string, changedAt time.Time) (Availability, error)
	SetOffline(ctx context.Context, providerID string, changedAt time.Time) (Availability, error)
	// SetBusy marks the provider as busy (set by trip.started event).
	SetBusy(ctx context.Context, providerID string, changedAt time.Time) (Availability, error)
	GetProviderGateState(ctx context.Context, providerID string) (ProviderGateState, bool, error)
	HasVerifiedActiveBike(ctx context.Context, providerID string) (bool, error)
	IsVehicleStepApproved(ctx context.Context, providerID string) (bool, error)
	CreateSessionIfNoneOpen(ctx context.Context, providerID string, wentOnlineAt time.Time) (AvailabilitySession, bool, error)
	GetOpenSession(ctx context.Context, providerID string) (AvailabilitySession, bool, error)
	EndOpenSession(ctx context.Context, providerID string, wentOfflineAt time.Time, forced bool) (AvailabilitySession, bool, error)
	// IncrementOpenSessionTrips adds 1 to trips_in_session on the open session.
	IncrementOpenSessionTrips(ctx context.Context, providerID string) error
	GetTodayAvailabilityStats(ctx context.Context, providerID string, todayStart time.Time, now time.Time) (TodayAvailabilityStats, error)
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) EnsureAvailability(ctx context.Context, providerID string) (Availability, error) {
	row := r.db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO provider_availability (provider_id)
			VALUES ($1)
			ON CONFLICT (provider_id) DO NOTHING
			RETURNING id::text, provider_id::text, status, verified_to_go_online,
				session_start, last_changed_at, created_at
		)
		SELECT id, provider_id, status, verified_to_go_online,
			session_start, last_changed_at, created_at
		FROM inserted
		UNION ALL
		SELECT id::text, provider_id::text, status, verified_to_go_online,
			session_start, last_changed_at, created_at
		FROM provider_availability
		WHERE provider_id = $1
		LIMIT 1
	`, providerID)
	return scanAvailability(row)
}

func (r *PostgresRepository) GetAvailability(ctx context.Context, providerID string) (Availability, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_id::text, status, verified_to_go_online,
			session_start, last_changed_at, created_at
		FROM provider_availability
		WHERE provider_id = $1
	`, providerID)
	availability, err := scanAvailability(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Availability{}, false, nil
	}
	return availability, err == nil, err
}

func (r *PostgresRepository) SetVerifiedToGoOnline(ctx context.Context, providerID string, verified bool) (Availability, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO provider_availability (provider_id, verified_to_go_online)
		VALUES ($1, $2)
		ON CONFLICT (provider_id)
		DO UPDATE SET
			verified_to_go_online = EXCLUDED.verified_to_go_online,
			last_changed_at = now()
		RETURNING id::text, provider_id::text, status, verified_to_go_online,
			session_start, last_changed_at, created_at
	`, providerID, verified)
	return scanAvailability(row)
}

func (r *PostgresRepository) SetOnline(ctx context.Context, providerID string, changedAt time.Time) (Availability, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE provider_availability
		SET status = 'online',
			session_start = COALESCE(session_start, $2::timestamptz),
			last_changed_at = $2::timestamptz
		WHERE provider_id = $1
		RETURNING id::text, provider_id::text, status, verified_to_go_online,
			session_start, last_changed_at, created_at
	`, providerID, changedAt)
	return scanAvailability(row)
}

func (r *PostgresRepository) SetOffline(ctx context.Context, providerID string, changedAt time.Time) (Availability, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE provider_availability
		SET status = 'offline',
			session_start = NULL,
			last_changed_at = $2::timestamptz
		WHERE provider_id = $1
		RETURNING id::text, provider_id::text, status, verified_to_go_online,
			session_start, last_changed_at, created_at
	`, providerID, changedAt)
	return scanAvailability(row)
}

func (r *PostgresRepository) SetBusy(ctx context.Context, providerID string, changedAt time.Time) (Availability, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE provider_availability
		SET status = 'busy',
			last_changed_at = $2::timestamptz
		WHERE provider_id = $1
		RETURNING id::text, provider_id::text, status, verified_to_go_online,
			session_start, last_changed_at, created_at
	`, providerID, changedAt)
	return scanAvailability(row)
}

func (r *PostgresRepository) IncrementOpenSessionTrips(ctx context.Context, providerID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE availability_sessions
		SET trips_in_session = trips_in_session + 1
		WHERE provider_id = $1
			AND went_offline_at IS NULL
	`, providerID)
	return err
}

func (r *PostgresRepository) GetProviderGateState(ctx context.Context, providerID string) (ProviderGateState, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, is_active, verification_status
		FROM providers
		WHERE id = $1
	`, providerID)

	var state ProviderGateState
	if err := row.Scan(&state.ProviderID, &state.IsActive, &state.VerificationStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProviderGateState{}, false, nil
		}
		return ProviderGateState{}, false, err
	}
	return state, true, nil
}

func (r *PostgresRepository) HasVerifiedActiveBike(ctx context.Context, providerID string) (bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM bikes
			WHERE provider_id = $1
				AND verification_status = 'verified'
				AND is_active = true
		)
	`, providerID)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *PostgresRepository) IsVehicleStepApproved(ctx context.Context, providerID string) (bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM verification_steps
			WHERE provider_id = $1
				AND step = 'vehicle'
				AND status = 'approved'
		)
	`, providerID)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *PostgresRepository) CreateSessionIfNoneOpen(ctx context.Context, providerID string, wentOnlineAt time.Time) (AvailabilitySession, bool, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO availability_sessions (provider_id, went_online_at)
		VALUES ($1, $2::timestamptz)
		ON CONFLICT (provider_id) WHERE went_offline_at IS NULL DO NOTHING
		RETURNING id::text, provider_id::text, went_online_at, went_offline_at,
			duration_minutes, trips_in_session, forced_offline, created_at
	`, providerID, wentOnlineAt)
	session, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return AvailabilitySession{}, false, nil
	}
	return session, err == nil, err
}

func (r *PostgresRepository) GetOpenSession(ctx context.Context, providerID string) (AvailabilitySession, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_id::text, went_online_at, went_offline_at,
			duration_minutes, trips_in_session, forced_offline, created_at
		FROM availability_sessions
		WHERE provider_id = $1
			AND went_offline_at IS NULL
		ORDER BY went_online_at DESC
		LIMIT 1
	`, providerID)
	session, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return AvailabilitySession{}, false, nil
	}
	return session, err == nil, err
}

func (r *PostgresRepository) EndOpenSession(ctx context.Context, providerID string, wentOfflineAt time.Time, forced bool) (AvailabilitySession, bool, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE availability_sessions
		SET went_offline_at = $2::timestamptz,
			duration_minutes = GREATEST(
				0,
				CEIL(EXTRACT(EPOCH FROM ($2::timestamptz - went_online_at)) / 60.0)::INT
			),
			forced_offline = $3
		WHERE provider_id = $1
			AND went_offline_at IS NULL
		RETURNING id::text, provider_id::text, went_online_at, went_offline_at,
			duration_minutes, trips_in_session, forced_offline, created_at
	`, providerID, wentOfflineAt, forced)
	session, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return AvailabilitySession{}, false, nil
	}
	return session, err == nil, err
}

func (r *PostgresRepository) GetTodayAvailabilityStats(ctx context.Context, providerID string, todayStart time.Time, now time.Time) (TodayAvailabilityStats, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(
				CASE
					WHEN went_offline_at IS NULL THEN GREATEST(
						0,
						CEIL(EXTRACT(EPOCH FROM ($3::timestamptz - went_online_at)) / 60.0)::INT
					)
					ELSE COALESCE(duration_minutes, GREATEST(
						0,
						CEIL(EXTRACT(EPOCH FROM (went_offline_at - went_online_at)) / 60.0)::INT
					))
				END
			), 0)::INT AS minutes_online,
			COALESCE(SUM(trips_in_session), 0)::INT AS trips_today
		FROM availability_sessions
		WHERE provider_id = $1
			AND went_online_at >= $2::timestamptz
	`, providerID, todayStart, now)

	var stats TodayAvailabilityStats
	if err := row.Scan(&stats.MinutesOnline, &stats.Trips); err != nil {
		return TodayAvailabilityStats{}, err
	}
	return stats, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAvailability(row rowScanner) (Availability, error) {
	var availability Availability
	var status string
	var sessionStart pgtype.Timestamptz

	if err := row.Scan(
		&availability.ID,
		&availability.ProviderID,
		&status,
		&availability.VerifiedToGoOnline,
		&sessionStart,
		&availability.LastChangedAt,
		&availability.CreatedAt,
	); err != nil {
		return Availability{}, err
	}

	availability.Status = AvailabilityStatus(status)
	if sessionStart.Valid {
		value := sessionStart.Time
		availability.SessionStart = &value
	}
	return availability, nil
}

func scanSession(row rowScanner) (AvailabilitySession, error) {
	var session AvailabilitySession
	var wentOfflineAt pgtype.Timestamptz
	var durationMinutes pgtype.Int4

	if err := row.Scan(
		&session.ID,
		&session.ProviderID,
		&session.WentOnlineAt,
		&wentOfflineAt,
		&durationMinutes,
		&session.TripsInSession,
		&session.ForcedOffline,
		&session.CreatedAt,
	); err != nil {
		return AvailabilitySession{}, err
	}
	if wentOfflineAt.Valid {
		value := wentOfflineAt.Time
		session.WentOfflineAt = &value
	}
	if durationMinutes.Valid {
		value := int(durationMinutes.Int32)
		session.DurationMinutes = &value
	}
	return session, nil
}
