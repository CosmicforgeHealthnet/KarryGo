// Package infra holds the concrete Postgres/Redis/wallet adapters that satisfy
// the partner delivery and partnerauth interfaces. It is wired in cmd/main.go
// and is not exercised by the unit tests (which use fakes).
package infra

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

// BookingRepo is the Postgres implementation of delivery.BookingRepository.
type BookingRepo struct {
	pool *pgxpool.Pool
}

// NewBookingRepo constructs a BookingRepo.
func NewBookingRepo(pool *pgxpool.Pool) *BookingRepo { return &BookingRepo{pool: pool} }

const bookingColumns = `id, booking_ref, partner_id, external_ref, pickup_address, pickup_lat, pickup_lng,
	dest_address, dest_lat, dest_lng, receiver_name, receiver_phone, package_type, package_size,
	is_fragile, service_tier, fare_amount_kobo, currency, status, payment_intent_id,
	dispatch_booking_id, idempotency_key, event_published_at, created_at, updated_at`

func (r *BookingRepo) CreatePendingBooking(ctx context.Context, b *delivery.PartnerBooking) error {
	const q = `INSERT INTO partner_bookings (
		id, booking_ref, partner_id, external_ref, pickup_address, pickup_lat, pickup_lng,
		dest_address, dest_lat, dest_lng, receiver_name, receiver_phone, package_type, package_size,
		is_fragile, service_tier, fare_amount_kobo, currency, status, dispatch_booking_id,
		idempotency_key, created_at, updated_at)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`

	_, err := r.pool.Exec(ctx, q,
		b.ID, b.BookingRef, b.PartnerID, b.ExternalRef, b.PickupAddress, b.PickupLat, b.PickupLng,
		b.DestAddress, b.DestLat, b.DestLng, b.ReceiverName, b.ReceiverPhone, b.PackageType, b.PackageSize,
		b.IsFragile, b.ServiceTier, b.FareAmountKobo, b.Currency, b.Status, b.DispatchBookingID,
		b.IdempotencyKey, b.CreatedAt, b.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "partner_bookings_booking_ref_key" {
			return delivery.ErrDuplicateBookingRef
		}
		return err
	}
	return nil
}

func (r *BookingRepo) UpdatePaymentAndDispatching(ctx context.Context, bookingID, paymentIntentID, dispatchBookingID string, eventPublishedAt time.Time) error {
	const q = `UPDATE partner_bookings
		SET payment_intent_id = $2, dispatch_booking_id = $3, status = 'dispatching',
		    event_published_at = $4, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, bookingID, paymentIntentID, dispatchBookingID, eventPublishedAt)
	return err
}

func (r *BookingRepo) FindByPartnerAndIDOrRef(ctx context.Context, partnerID, idOrRef string) (*delivery.PartnerBooking, error) {
	// Match on id (when idOrRef is a valid UUID) or booking_ref, always scoped to partner_id.
	q := `SELECT ` + bookingColumns + ` FROM partner_bookings
		WHERE partner_id = $1 AND (booking_ref = $2` + uuidClause(idOrRef) + `)`
	args := []any{partnerID, idOrRef}
	row := r.pool.QueryRow(ctx, q, args...)
	b, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *BookingRepo) ListByPartner(ctx context.Context, partnerID string, filter delivery.ListFilter) ([]delivery.PartnerBooking, int, error) {
	var total int
	countQ := `SELECT count(*) FROM partner_bookings WHERE partner_id = $1`
	countArgs := []any{partnerID}
	if filter.Status != "" {
		countQ += ` AND status = $2`
		countArgs = append(countArgs, filter.Status)
	}
	if err := r.pool.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQ := `SELECT ` + bookingColumns + ` FROM partner_bookings WHERE partner_id = $1`
	listArgs := []any{partnerID}
	if filter.Status != "" {
		listQ += ` AND status = $2`
		listArgs = append(listArgs, filter.Status)
	}
	listQ += ` ORDER BY created_at DESC LIMIT $` + itoa(len(listArgs)+1) + ` OFFSET $` + itoa(len(listArgs)+2)
	listArgs = append(listArgs, filter.Limit, filter.Offset)

	rows, err := r.pool.Query(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []delivery.PartnerBooking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *b)
	}
	return out, total, rows.Err()
}

func (r *BookingRepo) UpdateStatus(ctx context.Context, bookingID, status string) error {
	const q = `UPDATE partner_bookings SET status = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, bookingID, status)
	return err
}

// FindByDispatchBookingID resolves a partner booking by its dispatch booking id.
// Used by the webhook dispatch-event subscribers. Returns (nil, nil) when absent
// or when the id is not a valid UUID.
func (r *BookingRepo) FindByDispatchBookingID(ctx context.Context, dispatchBookingID string) (*delivery.PartnerBooking, error) {
	if _, err := uuid.Parse(dispatchBookingID); err != nil {
		return nil, nil
	}
	q := `SELECT ` + bookingColumns + ` FROM partner_bookings WHERE dispatch_booking_id = $1 LIMIT 1`
	b, err := scanBooking(r.pool.QueryRow(ctx, q, dispatchBookingID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanBooking(row rowScanner) (*delivery.PartnerBooking, error) {
	var b delivery.PartnerBooking
	err := row.Scan(
		&b.ID, &b.BookingRef, &b.PartnerID, &b.ExternalRef, &b.PickupAddress, &b.PickupLat, &b.PickupLng,
		&b.DestAddress, &b.DestLat, &b.DestLng, &b.ReceiverName, &b.ReceiverPhone, &b.PackageType, &b.PackageSize,
		&b.IsFragile, &b.ServiceTier, &b.FareAmountKobo, &b.Currency, &b.Status, &b.PaymentIntentID,
		&b.DispatchBookingID, &b.IdempotencyKey, &b.EventPublishedAt, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}
