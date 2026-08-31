package bookingtelemetry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the persistence boundary for the trip location trail.
type Repository interface {
	// Insert stores a point durably -- always, regardless of its own
	// IntegrityFlags (see the package doc comment: a flagged point is still
	// a real, kept record). inserted is false when this exact
	// (booking_id, provider_id, sequence) triple was already stored -- the
	// original row is left untouched, not overwritten, since it is the
	// source of truth for what this device actually reported first.
	Insert(ctx context.Context, p Point) (inserted bool, err error)
	// Latest returns the highest-sequence point already stored for this
	// booking+provider, and whether any point exists yet at all. Used to
	// classify the next incoming point against (see Classify).
	Latest(ctx context.Context, bookingID, providerID string) (Point, bool, error)
	// ValidatedTrail returns every stored point for a booking that carries
	// NO integrity flags, ordered by sequence -- the only points trusted
	// enough to derive a server-computed distance from (see
	// ActualDistanceKm).
	ValidatedTrail(ctx context.Context, bookingID string) ([]Point, error)
	// ListAll returns EVERY stored point for a booking regardless of
	// IntegrityFlags, ordered by sequence -- phase B10 route replay's own
	// requirement to show the complete, honest trail (including flagged
	// points), never the filtered "what we trusted" view ValidatedTrail
	// gives fare reconciliation.
	ListAll(ctx context.Context, bookingID string) ([]Point, error)
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const pointColumns = `id::text, booking_id::text, provider_id::text, sequence, lat, lng, accuracy_m, speed_mps, heading_deg, recorded_at, received_at, source, integrity_flags, coalesce(client_event_id, '')`

func scanPoint(row pgx.Row) (Point, error) {
	var p Point
	err := row.Scan(&p.ID, &p.BookingID, &p.ProviderID, &p.Sequence, &p.Lat, &p.Lng,
		&p.AccuracyM, &p.SpeedMps, &p.HeadingDeg, &p.RecordedAt, &p.ReceivedAt,
		&p.Source, &p.IntegrityFlags, &p.ClientEventID)
	return p, err
}

func (r *PostgresRepository) Insert(ctx context.Context, p Point) (bool, error) {
	// integrity_flags is NOT NULL DEFAULT '{}' -- a Go nil []string (the
	// normal, common case: a clean point with no flags) is sent by pgx as
	// SQL NULL, not an empty array, and violates that constraint. Found by
	// this package's own integration test (a fake client cannot catch this
	// class of bug -- see the B4 payments lesson in project memory for the
	// same failure mode against a different service).
	flags := p.IntegrityFlags
	if flags == nil {
		flags = []string{}
	}
	tag, err := r.db.Exec(ctx, `
		INSERT INTO taxi_trip_location_points
			(booking_id, provider_id, sequence, lat, lng, accuracy_m, speed_mps, heading_deg,
			 recorded_at, received_at, source, integrity_flags, client_event_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, nullif($13, ''))
		ON CONFLICT (booking_id, provider_id, sequence) DO NOTHING
	`, p.BookingID, p.ProviderID, p.Sequence, p.Lat, p.Lng, p.AccuracyM, p.SpeedMps, p.HeadingDeg,
		p.RecordedAt, p.ReceivedAt, p.Source, flags, p.ClientEventID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) Latest(ctx context.Context, bookingID, providerID string) (Point, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+pointColumns+`
		FROM taxi_trip_location_points
		WHERE booking_id = $1 AND provider_id = $2
		ORDER BY sequence DESC
		LIMIT 1
	`, bookingID, providerID)
	p, err := scanPoint(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Point{}, false, nil
	}
	if err != nil {
		return Point{}, false, err
	}
	return p, true, nil
}

func (r *PostgresRepository) ListAll(ctx context.Context, bookingID string) ([]Point, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+pointColumns+`
		FROM taxi_trip_location_points
		WHERE booking_id = $1
		ORDER BY sequence ASC
	`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Point
	for rows.Next() {
		p, err := scanPoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ValidatedTrail(ctx context.Context, bookingID string) ([]Point, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+pointColumns+`
		FROM taxi_trip_location_points
		WHERE booking_id = $1 AND integrity_flags = '{}'
		ORDER BY sequence ASC
	`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Point
	for rows.Next() {
		p, err := scanPoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
