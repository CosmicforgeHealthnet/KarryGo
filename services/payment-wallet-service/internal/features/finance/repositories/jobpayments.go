package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/ids"
)

// JobPaymentRepository owns Wallet's authoritative job payment records.
//
// Every read is tenant-scoped in SQL rather than filtered afterwards, so a
// caller that forgets to compare organizations still cannot see another
// tenant's payment.
type JobPaymentRepository struct{ db database.Querier }

// NewJobPaymentRepository wires the repository over a pool or transaction.
func NewJobPaymentRepository(db database.Querier) *JobPaymentRepository {
	return &JobPaymentRepository{db: db}
}

const jobPaymentColumns = `id,organization_id,source_service,dispatch_request_reference,
	trip_reference,booking_reference,payment_reference,amount_minor,currency,status,paid_at,
	COALESCE(journal_id,''),refund_state,reversal_state,operation_reference,idempotency_key,
	request_fingerprint,correlation_id,version,created_at,updated_at`

func scanJobPayment(row pgx.Row) (*models.JobPayment, error) {
	var value models.JobPayment
	err := row.Scan(&value.ID, &value.OrganizationID, &value.SourceService,
		&value.DispatchRequestReference, &value.TripReference, &value.BookingReference,
		&value.PaymentReference, &value.AmountMinor, &value.Currency, &value.Status,
		&value.PaidAt, &value.JournalID, &value.RefundState, &value.ReversalState,
		&value.OperationReference, &value.IdempotencyKey, &value.RequestFingerprint,
		&value.CorrelationID, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan job payment: %w", err)
	}
	return &value, nil
}

// ReferenceColumn maps a supported lookup key to its column. An unsupported
// key resolves to the empty string and the caller must refuse the lookup:
// guessing a column would turn a typo into a cross-record match.
func ReferenceColumn(referenceType string) string {
	switch strings.ToUpper(strings.TrimSpace(referenceType)) {
	case models.JobPaymentByDispatchRequest:
		return "dispatch_request_reference"
	case models.JobPaymentByTrip:
		return "trip_reference"
	case models.JobPaymentByBooking:
		return "booking_reference"
	case models.JobPaymentByPaymentRef:
		return "payment_reference"
	default:
		return ""
	}
}

// Lookup resolves one job payment by a supported authoritative reference.
func (r *JobPaymentRepository) Lookup(ctx context.Context, q database.Querier,
	organizationID, referenceType, reference string) (*models.JobPayment, error) {
	if q == nil {
		q = r.db
	}
	column := ReferenceColumn(referenceType)
	if column == "" || strings.TrimSpace(organizationID) == "" || strings.TrimSpace(reference) == "" {
		return nil, ErrNotFound
	}
	return scanJobPayment(q.QueryRow(ctx, `SELECT `+jobPaymentColumns+` FROM finance_job_payments
		WHERE organization_id=$1 AND `+column+`=$2`, organizationID, reference))
}

// Get loads one job payment by its own identifier inside a tenant.
func (r *JobPaymentRepository) Get(ctx context.Context, q database.Querier,
	organizationID, id string) (*models.JobPayment, error) {
	if q == nil {
		q = r.db
	}
	return scanJobPayment(q.QueryRow(ctx, `SELECT `+jobPaymentColumns+` FROM finance_job_payments
		WHERE id=$1 AND organization_id=$2`, id, organizationID))
}

// FindByKey resolves a replayed record by its idempotency key.
func (r *JobPaymentRepository) FindByKey(ctx context.Context, q database.Querier,
	organizationID, key string) (*models.JobPayment, error) {
	if q == nil {
		q = r.db
	}
	return scanJobPayment(q.QueryRow(ctx, `SELECT `+jobPaymentColumns+` FROM finance_job_payments
		WHERE organization_id=$1 AND idempotency_key=$2`, organizationID, key))
}

// LockExisting finds the record any of the supplied references already points
// at, taking a row lock so a concurrent recorder waits rather than inserting a
// second record for the same job.
//
// The references are checked one at a time rather than OR-ed together: if two
// of them resolved to different rows the job's payment identity would be
// ambiguous, and an ambiguous payment must never authorize money movement.
func (r *JobPaymentRepository) LockExisting(ctx context.Context, q database.Querier,
	payment models.JobPayment) (*models.JobPayment, error) {
	if q == nil {
		q = r.db
	}
	var found *models.JobPayment
	for _, candidate := range []struct{ column, value string }{
		{"payment_reference", payment.PaymentReference},
		{"dispatch_request_reference", payment.DispatchRequestReference},
		{"trip_reference", payment.TripReference},
		{"booking_reference", payment.BookingReference},
	} {
		if strings.TrimSpace(candidate.value) == "" {
			continue
		}
		existing, err := scanJobPayment(q.QueryRow(ctx, `SELECT `+jobPaymentColumns+`
			FROM finance_job_payments WHERE organization_id=$1 AND `+candidate.column+`=$2
			FOR UPDATE`, payment.OrganizationID, candidate.value))
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if found != nil && found.ID != existing.ID {
			return nil, ErrConflict
		}
		found = existing
	}
	if found == nil {
		return nil, ErrNotFound
	}
	return found, nil
}

// Insert writes a new authoritative job payment.
func (r *JobPaymentRepository) Insert(ctx context.Context, q database.Querier,
	payment *models.JobPayment) error {
	if q == nil {
		q = r.db
	}
	if payment.ID == "" {
		payment.ID = ids.New(ids.PrefixJobPayment)
	}
	if payment.RefundState == "" {
		payment.RefundState = models.JobPaymentStateNone
	}
	if payment.ReversalState == "" {
		payment.ReversalState = models.JobPaymentStateNone
	}
	err := q.QueryRow(ctx, `INSERT INTO finance_job_payments
		(id,organization_id,source_service,dispatch_request_reference,trip_reference,
		 booking_reference,payment_reference,amount_minor,currency,status,paid_at,journal_id,
		 refund_state,reversal_state,operation_reference,idempotency_key,request_fingerprint,
		 correlation_id,version,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),$13,$14,$15,$16,$17,$18,1,now(),now())
		RETURNING version,created_at,updated_at`,
		payment.ID, payment.OrganizationID, payment.SourceService,
		payment.DispatchRequestReference, payment.TripReference, payment.BookingReference,
		payment.PaymentReference, payment.AmountMinor, payment.Currency, payment.Status,
		payment.PaidAt, payment.JournalID, payment.RefundState, payment.ReversalState,
		payment.OperationReference, payment.IdempotencyKey, payment.RequestFingerprint,
		payment.CorrelationID).
		Scan(&payment.Version, &payment.CreatedAt, &payment.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: insert job payment: %w", err)
	}
	return nil
}

// Advance updates an existing record's status and back-fills any reference it
// did not yet carry. Amount and currency are never rewritten: a payment whose
// value changed is a different payment, not an update of this one.
func (r *JobPaymentRepository) Advance(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, payment models.JobPayment) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_job_payments SET
		status=$3,paid_at=COALESCE($4,paid_at),
		journal_id=COALESCE(journal_id,NULLIF($5,'')),
		refund_state=CASE WHEN $6='' THEN refund_state ELSE $6 END,
		reversal_state=CASE WHEN $7='' THEN reversal_state ELSE $7 END,
		operation_reference=CASE WHEN $8='' THEN operation_reference ELSE $8 END,
		dispatch_request_reference=CASE WHEN dispatch_request_reference='' THEN $9
			ELSE dispatch_request_reference END,
		trip_reference=CASE WHEN trip_reference='' THEN $10 ELSE trip_reference END,
		booking_reference=CASE WHEN booking_reference='' THEN $11 ELSE booking_reference END,
		payment_reference=CASE WHEN payment_reference='' THEN $12 ELSE payment_reference END,
		version=version+1,updated_at=now()
		WHERE id=$1 AND version=$2`,
		id, expectedVersion, payment.Status, payment.PaidAt, payment.JournalID,
		payment.RefundState, payment.ReversalState, payment.OperationReference,
		payment.DispatchRequestReference, payment.TripReference, payment.BookingReference,
		payment.PaymentReference)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: advance job payment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// SetOperationState advances the refund/reversal sub-state of a job payment.
// Passing an empty state leaves that dimension untouched.
func (r *JobPaymentRepository) SetOperationState(ctx context.Context, q database.Querier,
	organizationID, id, refundState, reversalState, operationReference, status string) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_job_payments SET
		refund_state=CASE WHEN $3='' THEN refund_state ELSE $3 END,
		reversal_state=CASE WHEN $4='' THEN reversal_state ELSE $4 END,
		operation_reference=CASE WHEN $5='' THEN operation_reference ELSE $5 END,
		status=CASE WHEN $6='' THEN status ELSE $6 END,
		version=version+1,updated_at=now()
		WHERE id=$1 AND organization_id=$2`,
		id, organizationID, refundState, reversalState, operationReference, status)
	if err != nil {
		return fmt.Errorf("finance: set job payment operation state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
