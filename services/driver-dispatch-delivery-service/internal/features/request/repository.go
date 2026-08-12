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
	// ListStrandedBroadcasts returns broadcasts still 'broadcasting' whose
	// window expired before cutoff — rows whose ExpireWindow task was never
	// scheduled (crash between CreateBroadcast and scheduleExpire).
	ListStrandedBroadcasts(ctx context.Context, cutoff time.Time) ([]RequestBroadcast, error)
}

type PostgresRepository struct {
	db *pgxpool.Pool
	// emitter is optional; nil means this repository publishes no events,
	// which is the behaviour every caller had before the marketplace outbox.
	emitter EventEmitter
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateBroadcast(ctx context.Context, input CreateBroadcastInput) (RequestBroadcast, error) {
	const statement = `
		INSERT INTO request_broadcasts (
			booking_id, service_type, broadcast_radius_km, attempt_number,
			providers_notified, broadcast_at, expires_at, booking_payload
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id::text, booking_id::text, service_type, broadcast_radius_km::float8,
			attempt_number, providers_notified, status, broadcast_at, expires_at,
			accepted_by_provider_id::text, booking_payload, created_at, updated_at`
	args := []any{input.BookingID, input.ServiceType, input.RadiusKM, input.Attempt,
		input.ProvidersNotified, input.BroadcastAt, input.ExpiresAt, input.BookingPayload}

	if !r.emitting() {
		broadcast, err := scanBroadcast(r.db.QueryRow(ctx, statement, args...))
		if isUniqueViolation(err) {
			return RequestBroadcast{}, apperrors.Conflict("A broadcast already exists for this booking.", err)
		}
		return broadcast, err
	}

	var broadcast RequestBroadcast
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var scanErr error
		broadcast, scanErr = scanBroadcast(tx.QueryRow(ctx, statement, args...))
		if isUniqueViolation(scanErr) {
			return apperrors.Conflict("A broadcast already exists for this booking.", scanErr)
		}
		if scanErr != nil {
			return scanErr
		}
		// The row is inserted with source_version 1; emitting bumps it to 2.
		// Consumers therefore see this aggregate open at 2 rather than 1. That
		// is deliberate and harmless — gap detection compares each event
		// against the version the consumer last stored, never against a fixed
		// starting number — and it keeps one rule for every emission site
		// instead of a special case for the first.
		return r.emitBroadcastEvent(ctx, tx, eventRequestOffered, broadcast.ID, broadcast.BookingID,
			map[string]any{
				"status":            string(broadcast.Status),
				"serviceType":       broadcast.ServiceType,
				"attemptNumber":     broadcast.AttemptNumber,
				"broadcastRadiusKm": broadcast.BroadcastRadiusKM,
				"broadcastAt":       broadcast.BroadcastAt,
				"expiresAt":         broadcast.ExpiresAt,
			})
	})
	if err != nil {
		return RequestBroadcast{}, err
	}
	return broadcast, nil
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
	return r.terminalTransition(ctx, broadcastID, eventRequestExpired,
		`UPDATE request_broadcasts SET status='expired', updated_at=now()
		 WHERE id=$1 AND status='broadcasting'
		 RETURNING booking_id::text`,
		map[string]any{"status": "expired", "reason": "offer_window_elapsed"})
}

func (r *PostgresRepository) MarkBroadcastNoProviderFound(ctx context.Context, broadcastID string) error {
	// Emitted as expired rather than as a type of its own. To a managing
	// organization the operational fact is identical — the request is no
	// longer on offer — and a twelfth event type would oblige every consumer
	// to learn a distinction that changes nothing they do. The reason field
	// carries the difference for anyone who needs it.
	return r.terminalTransition(ctx, broadcastID, eventRequestExpired,
		`UPDATE request_broadcasts SET status='no_provider_found', updated_at=now()
		 WHERE id=$1 AND status='broadcasting'
		 RETURNING booking_id::text`,
		map[string]any{"status": "no_provider_found", "reason": "no_provider_found"})
}

func (r *PostgresRepository) CancelBroadcast(ctx context.Context, broadcastID string) error {
	return r.terminalTransition(ctx, broadcastID, eventRequestCancelled,
		`UPDATE request_broadcasts SET status='cancelled', updated_at=now()
		 WHERE id=$1 AND status IN ('broadcasting','expired')
		 RETURNING booking_id::text`,
		map[string]any{"status": "cancelled"})
}

// terminalTransition applies a guarded status change and emits its event.
//
// The UPDATE is guarded on the current status, so a repeat call affects no
// rows and publishes nothing. That is what makes these transitions safe to
// retry: expiry and cancellation both arrive from schedulers that may fire
// twice, and an event per attempt would have consumers watching one request
// expire over and over.
func (r *PostgresRepository) terminalTransition(ctx context.Context, broadcastID string,
	eventType string, statement string, extra map[string]any) error {
	if !r.emitting() {
		_, err := r.db.Exec(ctx, statement, broadcastID)
		return err
	}
	return r.withTx(ctx, func(tx pgx.Tx) error {
		var bookingID string
		err := tx.QueryRow(ctx, statement, broadcastID).Scan(&bookingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // Already terminal: nothing changed, so nothing to announce.
		}
		if err != nil {
			return err
		}
		return r.emitBroadcastEvent(ctx, tx, eventType, broadcastID, bookingID, extra)
	})
}

func (r *PostgresRepository) UpdateBroadcastAttempt(ctx context.Context, broadcastID string, attempt int, radius float64, providersNotified int, broadcastAt, expiresAt time.Time) error {
	const statement = `
		UPDATE request_broadcasts
		SET attempt_number=$2, broadcast_radius_km=$3, providers_notified=providers_notified+$4,
			broadcast_at=$5, expires_at=$6, updated_at=$5
		WHERE id=$1 AND status='broadcasting'
		RETURNING booking_id::text`
	if !r.emitting() {
		_, err := r.db.Exec(ctx, statement, broadcastID, attempt, radius, providersNotified,
			broadcastAt, expiresAt)
		return err
	}
	return r.withTx(ctx, func(tx pgx.Tx) error {
		var bookingID string
		err := tx.QueryRow(ctx, statement, broadcastID, attempt, radius, providersNotified,
			broadcastAt, expiresAt).Scan(&bookingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return r.emitBroadcastEvent(ctx, tx, eventRequestUpdated, broadcastID, bookingID,
			map[string]any{
				"status":            "broadcasting",
				"attemptNumber":     attempt,
				"broadcastRadiusKm": radius,
				"expiresAt":         expiresAt,
			})
	})
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

func (r *PostgresRepository) ListStrandedBroadcasts(ctx context.Context, cutoff time.Time) ([]RequestBroadcast, error) {
	rows, err := r.db.Query(ctx, broadcastSelect+`
		WHERE status = 'broadcasting' AND expires_at < $1
		ORDER BY expires_at ASC
		LIMIT 100`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RequestBroadcast
	for rows.Next() {
		broadcast, err := scanBroadcast(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, broadcast)
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
