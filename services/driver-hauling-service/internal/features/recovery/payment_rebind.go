package recovery

// Section 17 gap closure: the durable obligation that drives a booking's held
// Wallet payment to the provider recovery bound.
//
// The obligation row is written in the SAME transaction that rebinds the
// booking (every replacement path: automatic selection, custody-transfer
// completion and the operator-directed replacement). This worker then moves
// it to a terminal state through Wallet's execution-provider contract.
//
// Four rules shape it:
//
//   - Hauling state is re-read before Wallet is ever called. An obligation
//     whose provider is no longer the booking's provider, or no longer the
//     case's replacement, is SUPERSEDED and Wallet is not called for it: a
//     stale obligation must never move money to someone who is no longer
//     doing the work.
//
//   - Every attempt reads Wallet's authoritative binding first and decides
//     from it. After a lost response (UNKNOWN_OUTCOME) that read is what
//     resolves the outcome: if Wallet applied the rebind, the binding already
//     names the replacement and nothing is sent again.
//
//   - A rebind is version-fenced and its idempotency key is derived from the
//     recovery case AND the binding version it was decided against -- the same
//     key the settlement guard derives -- so the worker, the post-commit fast
//     path and the settlement guard all collapse onto one Wallet effect.
//
//   - Nothing is silently dropped. Attempts are bounded; an obligation that
//     exhausts them is RECONCILIATION_REQUIRED, stays visible, and its recovery
//     case still explains the replacement to the settlement guard.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/walletclient"
)

// RebindState is the payment rebind obligation lifecycle.
type RebindState string

const (
	RebindPending                RebindState = "PENDING"
	RebindUnknownOutcome         RebindState = "UNKNOWN_OUTCOME"
	RebindCompleted              RebindState = "COMPLETED"
	RebindCompletedNotHeld       RebindState = "COMPLETED_NOT_HELD"
	RebindSuperseded             RebindState = "SUPERSEDED"
	RebindReconciliationRequired RebindState = "RECONCILIATION_REQUIRED"
)

// Terminal reports whether the worker is finished with an obligation.
func (s RebindState) Terminal() bool {
	switch s {
	case RebindCompleted, RebindCompletedNotHeld, RebindSuperseded, RebindReconciliationRequired:
		return true
	default:
		return false
	}
}

// Error codes recorded on an obligation. Wallet refusals record Wallet's own
// code; these are Hauling's classifications for everything else.
const (
	RebindErrStaleProvider     = "STALE_PROVIDER_REFERENCE"
	RebindErrWalletUnreachable = "WALLET_OUTCOME_UNKNOWN"
	RebindErrNoWalletPayment   = "NO_WALLET_PAYMENT"
	RebindErrHaulingState      = "HAULING_STATE_UNREADABLE"
)

// Wallet identity of a Hauling job payment. They match the booking payments
// adapter: the binding is keyed by (source service, booking id) and the
// beneficiary is a truck provider.
const (
	rebindSourceService = "driver-hauling-service"
	rebindProviderType  = "truck_provider"
	rebindReasonCode    = "PROVIDER_REPLACED"
)

// RebindIdempotencyKey is the Wallet idempotency key for moving a booking's
// payment under a recovery case, decided against a binding version. It is
// byte-identical to the key walletclient.RebindToReplacement derives, so the
// settlement guard and this worker replay each other rather than racing.
func RebindIdempotencyKey(bookingID, caseID string, version int64) string {
	return "rebind-" + bookingID + "-" + caseID + "-v" + strconv.FormatInt(version, 10)
}

// PaymentRebindObligation is one durable obligation.
type PaymentRebindObligation struct {
	ID                   string      `json:"id"`
	BookingID            string      `json:"booking_id"`
	RecoveryCaseID       string      `json:"recovery_case_id"`
	ProviderID           string      `json:"provider_id"`
	State                RebindState `json:"state"`
	Attempts             int         `json:"attempts"`
	LastErrorCode        string      `json:"last_error_code,omitempty"`
	WalletBindingVersion *int64      `json:"wallet_binding_version,omitempty"`
	WalletRebindID       string      `json:"wallet_rebind_id,omitempty"`
	NextAttemptAt        time.Time   `json:"next_attempt_at"`
	CreatedAt            time.Time   `json:"created_at"`
	UpdatedAt            time.Time   `json:"updated_at"`
	CompletedAt          *time.Time  `json:"completed_at,omitempty"`
}

const obligationColumns = `id::text, booking_id::text, recovery_case_id::text, provider_id::text,
	state, attempts, last_error_code, wallet_binding_version, wallet_rebind_id,
	next_attempt_at, created_at, updated_at, completed_at`

func scanObligation(row pgx.Row) (PaymentRebindObligation, error) {
	var o PaymentRebindObligation
	var state string
	err := row.Scan(&o.ID, &o.BookingID, &o.RecoveryCaseID, &o.ProviderID, &state, &o.Attempts,
		&o.LastErrorCode, &o.WalletBindingVersion, &o.WalletRebindID,
		&o.NextAttemptAt, &o.CreatedAt, &o.UpdatedAt, &o.CompletedAt)
	o.State = RebindState(state)
	return o, err
}

// RecordPaymentRebindTx writes the obligation to move the booking's held
// payment to providerID, inside the caller's transaction -- the one that
// rebinds the booking. It is idempotent on (case, provider): a retried or
// replayed command collapses onto the existing row.
func (r *Repository) RecordPaymentRebindTx(ctx context.Context, tx pgx.Tx, bookingID, caseID, providerID string) error {
	_, err := tx.Exec(ctx, `
INSERT INTO hauling_payment_rebind_obligations (booking_id, recovery_case_id, provider_id)
VALUES ($1::uuid, $2::uuid, $3::uuid)
ON CONFLICT (recovery_case_id, provider_id) DO NOTHING`, bookingID, caseID, providerID)
	return err
}

// PaymentRebindsForBooking returns a booking's obligations, newest first, for
// the operator read surface.
func (r *Repository) PaymentRebindsForBooking(ctx context.Context, bookingID string) ([]PaymentRebindObligation, error) {
	rows, err := r.q.Query(ctx, `SELECT `+obligationColumns+`
		FROM hauling_payment_rebind_obligations WHERE booking_id::text=$1 ORDER BY created_at DESC`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PaymentRebindObligation{}
	for rows.Next() {
		found, err := scanObligation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, found)
	}
	return out, rows.Err()
}

// WalletBindingAuthority is the slice of Wallet's execution-provider contract
// the worker uses. walletclient.Client satisfies it.
type WalletBindingAuthority interface {
	ExecutionProviderBinding(ctx context.Context, sourceService, sourceReference string) (walletclient.ExecutionProviderBinding, error)
	RebindExecutionProvider(ctx context.Context, sourceService, sourceReference string,
		request walletclient.ExecutionProviderRebindRequest) (walletclient.ExecutionProviderRebind, error)
}

// PaymentRebindWorker drives obligations to a terminal state.
type PaymentRebindWorker struct {
	DB     *pgxpool.Pool
	Wallet WalletBindingAuthority
	// MaxAttempts bounds Wallet attempts before RECONCILIATION_REQUIRED.
	MaxAttempts int
	// Lease is how long a claimed obligation is held before another worker
	// may take it over. It must exceed two Wallet round trips.
	Lease       time.Duration
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	BatchSize   int
	Now         func() time.Time
}

func (w *PaymentRebindWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *PaymentRebindWorker) maxAttempts() int {
	if w.MaxAttempts > 0 {
		return w.MaxAttempts
	}
	return 12
}

func (w *PaymentRebindWorker) lease() time.Duration {
	if w.Lease > 0 {
		return w.Lease
	}
	return time.Minute
}

func (w *PaymentRebindWorker) backoff(attempts int) time.Duration {
	base, ceiling := w.BaseBackoff, w.MaxBackoff
	if base <= 0 {
		base = 5 * time.Second
	}
	if ceiling <= 0 {
		ceiling = 10 * time.Minute
	}
	delay := base
	for i := 1; i < attempts && delay < ceiling; i++ {
		delay *= 2
	}
	if delay > ceiling {
		delay = ceiling
	}
	return delay
}

// Run processes due obligations every interval until ctx is cancelled.
func (w *PaymentRebindWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
			logging.Error("hauling-recovery-payment-rebind", "sweep: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce claims one batch of due obligations and processes each. It returns
// how many it claimed.
func (w *PaymentRebindWorker) RunOnce(ctx context.Context) (int, error) {
	if w == nil || w.DB == nil || w.Wallet == nil {
		return 0, nil
	}
	limit := w.BatchSize
	if limit <= 0 {
		limit = 20
	}
	token := uuid.NewString()
	claimed, err := w.claim(ctx, token, `
	   AND next_attempt_at <= $2
	 ORDER BY next_attempt_at, created_at
	 LIMIT $4`, limit)
	if err != nil {
		return 0, err
	}
	for _, obligation := range claimed {
		if err := w.process(ctx, token, obligation); err != nil {
			logging.Error("hauling-recovery-payment-rebind", "obligation %s: %v", obligation.ID, err)
		}
	}
	return len(claimed), nil
}

// Nudge processes the obligation for (case, provider) now, if no other worker
// holds it. It is the post-commit fast path: the same claim, revalidation and
// Wallet readback the sweep uses, so its outcome is recorded durably and a
// failure simply leaves the obligation for the sweep.
func (w *PaymentRebindWorker) Nudge(ctx context.Context, caseID, providerID string) (PaymentRebindObligation, error) {
	if w == nil || w.DB == nil || w.Wallet == nil {
		return PaymentRebindObligation{}, nil
	}
	token := uuid.NewString()
	claimed, err := w.claim(ctx, token, `
	   AND recovery_case_id::text = $5 AND provider_id::text = $6
	 LIMIT $4`, 1, caseID, providerID)
	if err != nil || len(claimed) == 0 {
		return PaymentRebindObligation{}, err
	}
	if err := w.process(ctx, token, claimed[0]); err != nil {
		return PaymentRebindObligation{}, err
	}
	return scanObligation(w.DB.QueryRow(ctx, `SELECT `+obligationColumns+`
		FROM hauling_payment_rebind_obligations WHERE id=$1`, claimed[0].ID))
}

// claim leases due obligations with SKIP LOCKED, so concurrent workers never
// process the same row, and a crashed worker's lease simply expires.
func (w *PaymentRebindWorker) claim(ctx context.Context, token, filter string, limit int, extra ...any) ([]PaymentRebindObligation, error) {
	now := w.now()
	args := append([]any{token, now, w.lease().Milliseconds(), limit}, extra...)
	rows, err := w.DB.Query(ctx, `
UPDATE hauling_payment_rebind_obligations
   SET lease_owner = $1,
       lease_expires_at = $2::timestamptz + ($3::bigint * interval '1 millisecond'),
       updated_at = $2
 WHERE id IN (
	SELECT id FROM hauling_payment_rebind_obligations
	 WHERE state IN ('PENDING','UNKNOWN_OUTCOME')
	   AND (lease_expires_at IS NULL OR lease_expires_at <= $2)`+filter+`
	   FOR UPDATE SKIP LOCKED)
RETURNING `+obligationColumns, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PaymentRebindObligation
	for rows.Next() {
		found, err := scanObligation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, found)
	}
	return out, rows.Err()
}

// outcome is what one attempt decided.
type outcome struct {
	state      RebindState
	errorCode  string
	version    *int64
	rebindID   string
	countsTry  bool
	retryDelay time.Duration
}

func (w *PaymentRebindWorker) process(ctx context.Context, token string, obligation PaymentRebindObligation) error {
	decided, err := w.decide(ctx, obligation)
	if err != nil {
		// Hauling's own state could not be read. Nothing was sent; release
		// the lease and try again later without spending an attempt.
		decided = outcome{state: obligation.State, errorCode: RebindErrHaulingState, retryDelay: w.backoff(1)}
	}
	attempts := obligation.Attempts
	if decided.countsTry {
		attempts++
	}
	if !decided.state.Terminal() && attempts >= w.maxAttempts() {
		decided.state = RebindReconciliationRequired
	}
	now := w.now()
	var completedAt any
	if decided.state.Terminal() {
		completedAt = now
	}
	next := now
	if !decided.state.Terminal() {
		delay := decided.retryDelay
		if delay <= 0 {
			delay = w.backoff(attempts)
		}
		next = now.Add(delay)
	}
	if decided.state == RebindReconciliationRequired {
		logging.Notice("hauling-recovery-payment-rebind",
			"booking %s: payment rebind to the replacement needs reconciliation after %d attempts (%s)",
			obligation.BookingID, attempts, decided.errorCode)
	}
	tag, writeErr := w.DB.Exec(ctx, `
UPDATE hauling_payment_rebind_obligations
   SET state=$3, attempts=$4, last_error_code=$5,
       wallet_binding_version=COALESCE($6, wallet_binding_version),
       wallet_rebind_id=COALESCE(NULLIF($7,''), wallet_rebind_id),
       next_attempt_at=$8, lease_owner='', lease_expires_at=NULL,
       updated_at=$9, completed_at=$10
 WHERE id=$1 AND lease_owner=$2`,
		obligation.ID, token, string(decided.state), attempts, decided.errorCode,
		decided.version, decided.rebindID, next, now, completedAt)
	if writeErr != nil {
		return writeErr
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("lease on obligation %s was lost before its outcome was recorded", obligation.ID)
	}
	return err
}

// decide runs one attempt: revalidate Hauling, read Wallet, send at most one
// rebind, classify the answer.
func (w *PaymentRebindWorker) decide(ctx context.Context, obligation PaymentRebindObligation) (outcome, error) {
	var bookingProvider, caseReplacement string
	if err := w.DB.QueryRow(ctx, `
SELECT COALESCE(b.provider_id::text,''), COALESCE(c.replacement_provider_id::text,'')
  FROM haulage_bookings b, hauling_recovery_cases c
 WHERE b.id=$1::uuid AND c.id=$2::uuid AND c.booking_id=b.id`,
		obligation.BookingID, obligation.RecoveryCaseID).Scan(&bookingProvider, &caseReplacement); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return outcome{state: RebindSuperseded, errorCode: RebindErrStaleProvider}, nil
		}
		return outcome{}, err
	}
	if bookingProvider != obligation.ProviderID || caseReplacement != obligation.ProviderID {
		// The booking was re-replaced, or the case no longer names this
		// provider. Moving the payment now would pay someone who is no longer
		// doing the work, so Wallet is never called for it.
		return outcome{state: RebindSuperseded, errorCode: RebindErrStaleProvider}, nil
	}

	// Authoritative readback FIRST. This is what resolves an UNKNOWN_OUTCOME:
	// a rebind Wallet applied but whose answer was lost already shows here.
	binding, err := w.Wallet.ExecutionProviderBinding(ctx, rebindSourceService, obligation.BookingID)
	if err != nil {
		if walletNotFound(err) {
			return outcome{state: RebindCompletedNotHeld, errorCode: RebindErrNoWalletPayment}, nil
		}
		// Nothing was sent on this attempt, so the previous state stands: an
		// UNKNOWN_OUTCOME stays unknown until a read resolves it.
		return outcome{state: obligation.State, errorCode: walletErrorCode(err), countsTry: true}, nil
	}
	version := binding.Version
	if binding.ProviderID == obligation.ProviderID && binding.ProviderType == rebindProviderType {
		return outcome{state: RebindCompleted, version: &version,
			rebindID: rebindIDFor(binding, obligation)}, nil
	}
	if binding.Status != "held" {
		return outcome{state: RebindCompletedNotHeld, errorCode: "payment_not_held", version: &version}, nil
	}

	rebind, err := w.Wallet.RebindExecutionProvider(ctx, rebindSourceService, obligation.BookingID,
		walletclient.ExecutionProviderRebindRequest{
			ProviderType: rebindProviderType, ProviderID: obligation.ProviderID,
			ExpectedVersion: version, RecoveryCaseRef: obligation.RecoveryCaseID,
			ReasonCode:     rebindReasonCode,
			IdempotencyKey: RebindIdempotencyKey(obligation.BookingID, obligation.RecoveryCaseID, version),
		})
	if err == nil {
		applied := rebind.ToVersion
		return outcome{state: RebindCompleted, version: &applied, rebindID: rebind.ID}, nil
	}
	switch walletclient.RequestErrorCode(err) {
	case walletclient.CodeProviderAlreadyBound:
		return outcome{state: RebindCompleted, version: &version}, nil
	case walletclient.CodePaymentNotHeld:
		return outcome{state: RebindCompletedNotHeld, errorCode: walletclient.CodePaymentNotHeld, version: &version}, nil
	case walletclient.CodeBindingVersionConflict:
		// Someone else moved the binding since the read. Re-read soon and
		// decide again; this is a definite refusal, not an unknown outcome.
		return outcome{state: RebindPending, errorCode: walletclient.CodeBindingVersionConflict,
			countsTry: true, retryDelay: w.backoff(1)}, nil
	}
	if walletNotFound(err) {
		return outcome{state: RebindCompletedNotHeld, errorCode: RebindErrNoWalletPayment, version: &version}, nil
	}
	if walletDefiniteRefusal(err) {
		return outcome{state: RebindPending, errorCode: walletErrorCode(err), countsTry: true}, nil
	}
	// Transport failure, timeout or Wallet 5xx after a send: the rebind may
	// or may not have landed. The next attempt reads before it sends.
	return outcome{state: RebindUnknownOutcome, errorCode: walletErrorCode(err), countsTry: true}, nil
}

func rebindIDFor(binding walletclient.ExecutionProviderBinding, obligation PaymentRebindObligation) string {
	for i := len(binding.History) - 1; i >= 0; i-- {
		entry := binding.History[i]
		if entry.RecoveryCaseRef == obligation.RecoveryCaseID && entry.NewProviderID == obligation.ProviderID {
			return entry.ID
		}
	}
	return ""
}

func walletNotFound(err error) bool {
	var requestErr *walletclient.RequestError
	return errors.As(err, &requestErr) && requestErr.Status == http.StatusNotFound
}

// walletDefiniteRefusal is a 4xx: Wallet answered and did not apply anything.
func walletDefiniteRefusal(err error) bool {
	var requestErr *walletclient.RequestError
	return errors.As(err, &requestErr) && requestErr.Status >= 400 && requestErr.Status < 500
}

// walletErrorCode is a stable code for the record: Wallet's own code for a
// 4xx refusal, otherwise Hauling's "outcome unknown" classification. It never
// carries a message body.
func walletErrorCode(err error) string {
	if walletDefiniteRefusal(err) {
		if code := walletclient.RequestErrorCode(err); code != "" {
			return code
		}
		return "wallet_refused"
	}
	return RebindErrWalletUnreachable
}
