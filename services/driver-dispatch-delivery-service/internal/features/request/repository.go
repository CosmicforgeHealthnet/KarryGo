package request

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
)

type Repository interface {
	CreateBroadcast(ctx context.Context, input CreateBroadcastInput) (RequestBroadcast, error)
	GetBroadcastByID(ctx context.Context, broadcastID string) (RequestBroadcast, bool, error)
	GetBroadcastByBookingID(ctx context.Context, bookingID string) (RequestBroadcast, bool, error)
	UpdateBroadcastStatus(ctx context.Context, broadcastID string, status BroadcastStatus) error
	MarkBroadcastAccepted(ctx context.Context, broadcastID, bookingID, inboxID, providerID string, respondedAt time.Time) error
	MarkBroadcastExpired(ctx context.Context, broadcastID string) error
	MarkBroadcastNoProviderFound(ctx context.Context, broadcastID string) error
	// CancelBroadcast sets status=cancelled if current status is broadcasting or expired (Phase 6I).
	CancelBroadcast(ctx context.Context, broadcastID string) error
	UpdateBroadcastAttempt(ctx context.Context, broadcastID string, attempt int, radius float64, providersNotified int, broadcastAt, expiresAt time.Time) error
	CreateInboxRows(ctx context.Context, broadcastID, bookingID string, providerIDs []string) ([]ProviderRequestInbox, error)
	ListProviderInbox(ctx context.Context, providerID string, options ListInboxOptions) ([]ProviderRequestInbox, error)
	GetProviderInboxByID(ctx context.Context, inboxID, providerID string) (ProviderRequestInbox, bool, error)
	MarkInboxRejected(ctx context.Context, inboxID, providerID string, respondedAt time.Time) (bool, error)
	MarkPendingInboxExpired(ctx context.Context, broadcastID string) error
	MarkFCMSent(ctx context.Context, inboxID string, sentAt time.Time) error
	ListAlreadyNotifiedProviders(ctx context.Context, bookingID string) ([]string, error)
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateBroadcast(ctx context.Context, input CreateBroadcastInput) (RequestBroadcast, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO request_broadcasts (
			booking_id, service_type, broadcast_radius_km, attempt_number,
			providers_notified, broadcast_at, expires_at, booking_payload
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id::text, booking_id::text, service_type, broadcast_radius_km::float8,
			attempt_number, providers_notified, status, broadcast_at, expires_at,
			accepted_by_provider_id::text, booking_payload, created_at, updated_at
	`, input.BookingID, input.ServiceType, input.RadiusKM, input.Attempt, input.ProvidersNotified,
		input.BroadcastAt, input.ExpiresAt, input.BookingPayload)
	broadcast, err := scanBroadcast(row)
	if isUniqueViolation(err) {
		return RequestBroadcast{}, apperrors.Conflict("A broadcast already exists for this booking.", err)
	}
	return broadcast, err
}

func (r *PostgresRepository) GetBroadcastByID(ctx context.Context, broadcastID string) (RequestBroadcast, bool, error) {
	return getBroadcast(r.db.QueryRow(ctx, broadcastSelect+` WHERE id = $1`, broadcastID))
}

func (r *PostgresRepository) GetBroadcastByBookingID(ctx context.Context, bookingID string) (RequestBroadcast, bool, error) {
	return getBroadcast(r.db.QueryRow(ctx, broadcastSelect+` WHERE booking_id = $1`, bookingID))
}

func (r *PostgresRepository) UpdateBroadcastStatus(ctx context.Context, broadcastID string, status BroadcastStatus) error {
	_, err := r.db.Exec(ctx, `UPDATE request_broadcasts SET status=$2, updated_at=now() WHERE id=$1`, broadcastID, status)
	return err
}

func (r *PostgresRepository) MarkBroadcastAccepted(ctx context.Context, broadcastID, bookingID, inboxID, providerID string, respondedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx, `
		UPDATE request_broadcasts
		SET status='accepted', accepted_by_provider_id=$3, updated_at=$4
		WHERE id=$1 AND booking_id=$2 AND status='broadcasting' AND expires_at > $4
	`, broadcastID, bookingID, providerID, respondedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperrors.Conflict("This request is no longer available.", nil)
	}
	tag, err = tx.Exec(ctx, `
		UPDATE provider_request_inbox
		SET status='accepted', responded_at=$4
		WHERE id=$1 AND broadcast_id=$2 AND provider_id=$3 AND status='pending'
	`, inboxID, broadcastID, providerID, respondedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperrors.Conflict("This inbox request is no longer pending.", nil)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE provider_request_inbox
		SET status='expired', responded_at=COALESCE(responded_at, $3)
		WHERE broadcast_id=$1 AND id<>$2 AND status='pending'
	`, broadcastID, inboxID, respondedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) MarkBroadcastExpired(ctx context.Context, broadcastID string) error {
	_, err := r.db.Exec(ctx, `UPDATE request_broadcasts SET status='expired', updated_at=now() WHERE id=$1 AND status='broadcasting'`, broadcastID)
	return err
}

func (r *PostgresRepository) MarkBroadcastNoProviderFound(ctx context.Context, broadcastID string) error {
	_, err := r.db.Exec(ctx, `UPDATE request_broadcasts SET status='no_provider_found', updated_at=now() WHERE id=$1 AND status='broadcasting'`, broadcastID)
	return err
}

func (r *PostgresRepository) CancelBroadcast(ctx context.Context, broadcastID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE request_broadcasts SET status='cancelled', updated_at=now() WHERE id=$1 AND status IN ('broadcasting','expired')`,
		broadcastID)
	return err
}

func (r *PostgresRepository) UpdateBroadcastAttempt(ctx context.Context, broadcastID string, attempt int, radius float64, providersNotified int, broadcastAt, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE request_broadcasts
		SET attempt_number=$2, broadcast_radius_km=$3, providers_notified=providers_notified+$4,
			broadcast_at=$5, expires_at=$6, updated_at=$5
		WHERE id=$1 AND status='broadcasting'
	`, broadcastID, attempt, radius, providersNotified, broadcastAt, expiresAt)
	return err
}

func (r *PostgresRepository) CreateInboxRows(ctx context.Context, broadcastID, bookingID string, providerIDs []string) ([]ProviderRequestInbox, error) {
	rows := make([]ProviderRequestInbox, 0, len(providerIDs))
	for _, providerID := range providerIDs {
		row := r.db.QueryRow(ctx, `
			INSERT INTO provider_request_inbox (broadcast_id, booking_id, provider_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (provider_id, booking_id) DO NOTHING
			RETURNING id::text, broadcast_id::text, booking_id::text, provider_id::text,
				status, received_at, responded_at, fcm_sent, fcm_sent_at
		`, broadcastID, bookingID, providerID)
		inbox, err := scanInboxBase(row)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, inbox)
	}
	return rows, nil
}

func (r *PostgresRepository) ListProviderInbox(ctx context.Context, providerID string, options ListInboxOptions) ([]ProviderRequestInbox, error) {
	if options.Limit <= 0 || options.Limit > 100 {
		options.Limit = 20
	}
	if options.Status == "" {
		options.Status = InboxStatusPending
	}
	query := inboxDetailSelect + ` WHERE i.provider_id=$1`
	args := []any{providerID}
	if options.Status != "" {
		query += ` AND i.status=$2`
		if options.Status == InboxStatusPending {
			query += ` AND b.status='broadcasting' AND b.expires_at > now()`
		}
		query += ` ORDER BY i.received_at DESC LIMIT $3 OFFSET $4`
		args = append(args, options.Status, options.Limit, options.Offset)
	} else {
		query += ` ORDER BY i.received_at DESC LIMIT $2 OFFSET $3`
		args = append(args, options.Limit, options.Offset)
	}
	dbRows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer dbRows.Close()
	result := []ProviderRequestInbox{}
	for dbRows.Next() {
		inbox, err := scanInboxDetail(dbRows)
		if err != nil {
			return nil, err
		}
		result = append(result, inbox)
	}
	return result, dbRows.Err()
}

func (r *PostgresRepository) GetProviderInboxByID(ctx context.Context, inboxID, providerID string) (ProviderRequestInbox, bool, error) {
	row := r.db.QueryRow(ctx, inboxDetailSelect+` WHERE i.id=$1 AND i.provider_id=$2`, inboxID, providerID)
	inbox, err := scanInboxDetail(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderRequestInbox{}, false, nil
	}
	return inbox, err == nil, err
}

func (r *PostgresRepository) MarkInboxRejected(ctx context.Context, inboxID, providerID string, respondedAt time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE provider_request_inbox SET status='rejected', responded_at=$3
		WHERE id=$1 AND provider_id=$2 AND status='pending'
	`, inboxID, providerID, respondedAt)
	return tag.RowsAffected() == 1, err
}

func (r *PostgresRepository) MarkPendingInboxExpired(ctx context.Context, broadcastID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE provider_request_inbox SET status='expired', responded_at=COALESCE(responded_at, now())
		WHERE broadcast_id=$1 AND status='pending'
	`, broadcastID)
	return err
}

func (r *PostgresRepository) MarkFCMSent(ctx context.Context, inboxID string, sentAt time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE provider_request_inbox SET fcm_sent=true, fcm_sent_at=$2 WHERE id=$1`, inboxID, sentAt)
	return err
}

func (r *PostgresRepository) ListAlreadyNotifiedProviders(ctx context.Context, bookingID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT provider_id::text FROM provider_request_inbox WHERE booking_id=$1`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

const broadcastSelect = `
	SELECT id::text, booking_id::text, service_type, broadcast_radius_km::float8,
		attempt_number, providers_notified, status, broadcast_at, expires_at,
		accepted_by_provider_id::text, booking_payload, created_at, updated_at
	FROM request_broadcasts`

const inboxDetailSelect = `
	SELECT i.id::text, i.broadcast_id::text, i.booking_id::text, i.provider_id::text,
		i.status, i.received_at, i.responded_at, i.fcm_sent, i.fcm_sent_at,
		b.expires_at, b.booking_payload
	FROM provider_request_inbox i
	JOIN request_broadcasts b ON b.id=i.broadcast_id`

type rowScanner interface {
	Scan(dest ...any) error
}

func getBroadcast(row rowScanner) (RequestBroadcast, bool, error) {
	broadcast, err := scanBroadcast(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return RequestBroadcast{}, false, nil
	}
	return broadcast, err == nil, err
}

func scanBroadcast(row rowScanner) (RequestBroadcast, error) {
	var b RequestBroadcast
	var accepted pgtype.Text
	var status string
	if err := row.Scan(&b.ID, &b.BookingID, &b.ServiceType, &b.BroadcastRadiusKM,
		&b.AttemptNumber, &b.ProvidersNotified, &status, &b.BroadcastAt, &b.ExpiresAt,
		&accepted, &b.BookingPayload, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return RequestBroadcast{}, err
	}
	b.Status = BroadcastStatus(status)
	if accepted.Valid {
		b.AcceptedByProviderID = &accepted.String
	}
	return b, nil
}

func scanInboxBase(row rowScanner) (ProviderRequestInbox, error) {
	var i ProviderRequestInbox
	var status string
	var responded, fcmSentAt pgtype.Timestamptz
	if err := row.Scan(&i.ID, &i.BroadcastID, &i.BookingID, &i.ProviderID, &status,
		&i.ReceivedAt, &responded, &i.FCMSent, &fcmSentAt); err != nil {
		return ProviderRequestInbox{}, err
	}
	i.Status = InboxStatus(status)
	setInboxNullableTimes(&i, responded, fcmSentAt)
	return i, nil
}

func scanInboxDetail(row rowScanner) (ProviderRequestInbox, error) {
	i, err := scanInboxBaseWithExtra(row)
	return i, err
}

func scanInboxBaseWithExtra(row rowScanner) (ProviderRequestInbox, error) {
	var i ProviderRequestInbox
	var status string
	var responded, fcmSentAt pgtype.Timestamptz
	if err := row.Scan(&i.ID, &i.BroadcastID, &i.BookingID, &i.ProviderID, &status,
		&i.ReceivedAt, &responded, &i.FCMSent, &fcmSentAt, &i.ExpiresAt, &i.BookingPayload); err != nil {
		return ProviderRequestInbox{}, err
	}
	i.Status = InboxStatus(status)
	setInboxNullableTimes(&i, responded, fcmSentAt)
	return i, nil
}

func setInboxNullableTimes(i *ProviderRequestInbox, responded, fcmSentAt pgtype.Timestamptz) {
	if responded.Valid {
		value := responded.Time
		i.RespondedAt = &value
	}
	if fcmSentAt.Valid {
		value := fcmSentAt.Time
		i.FCMSentAt = &value
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
