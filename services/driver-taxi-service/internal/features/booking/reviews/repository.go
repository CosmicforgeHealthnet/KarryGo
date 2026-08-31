package bookingreviews

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the persistence boundary for booking_reviews.
type Repository interface {
	// Create inserts one direction's review. Returns the same unique-
	// constraint-violation error a caller maps to "you already reviewed
	// this ride" -- this package does not interpret Postgres error codes,
	// it only stores; see bookingusecases' reviews.go for that mapping.
	Create(ctx context.Context, r Review) (Review, error)
	// GetForBooking returns every review submitted for a booking -- zero,
	// one, or both directions.
	GetForBooking(ctx context.Context, bookingID string) ([]Review, error)
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const reviewColumns = `id::text, booking_id::text, direction, customer_id::text, provider_id::text, rating, review_text, tags, recommends_driver, created_at`

func scanReview(row pgx.Row) (Review, error) {
	var r Review
	err := row.Scan(&r.ID, &r.BookingID, &r.Direction, &r.CustomerID, &r.ProviderID,
		&r.Rating, &r.ReviewText, &r.Tags, &r.RecommendsDriver, &r.CreatedAt)
	return r, err
}

func (r *PostgresRepository) Create(ctx context.Context, rev Review) (Review, error) {
	// tags is NOT NULL DEFAULT '{}' -- a Go nil []string is sent by pgx as
	// SQL NULL, not an empty array, and would violate that constraint (the
	// exact bug already found and fixed once in bookingtelemetry.Insert;
	// applying the same fix here up front rather than rediscovering it live
	// a second time).
	tags := rev.Tags
	if tags == nil {
		tags = []string{}
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO booking_reviews (booking_id, direction, customer_id, provider_id, rating, review_text, tags, recommends_driver)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+reviewColumns,
		rev.BookingID, rev.Direction, rev.CustomerID, rev.ProviderID, rev.Rating, rev.ReviewText, tags, rev.RecommendsDriver)
	return scanReview(row)
}

func (r *PostgresRepository) GetForBooking(ctx context.Context, bookingID string) ([]Review, error) {
	rows, err := r.db.Query(ctx, `SELECT `+reviewColumns+` FROM booking_reviews WHERE booking_id = $1`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Review
	for rows.Next() {
		rev, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}
