// Package financereportrepositories reads the aggregate source figures behind
// the Section 26 group finance report.
//
// Everything here is READ ONLY. Every query runs inside one REPEATABLE READ,
// READ ONLY transaction so all nine measures describe the same instant, and
// every query returns aggregates per currency only -- never a customer,
// provider, organization or account identifier.
//
// Each row is counted once, by its own state timestamp:
//   - window measures use the posting time of the ledger transaction (or the
//     completion column) that moved the row into the counted state;
//   - snapshot measures read the state visible at data_as_of.
package financereportrepositories

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Bucket is one currency's aggregate for one query.
type Bucket struct {
	Currency string
	Amount   int64
	Count    int64
	MaxTS    *time.Time
}

// Snapshot is everything the usecase needs, read at one instant.
type Snapshot struct {
	DataAsOf time.Time
	// Per measure, per currency. Window measures cover [start, min(end, DataAsOf)).
	Measures map[string][]Bucket
	// Commerce allocation releases in the window, per currency. They settle
	// per allocation, not per intent, so successful_settlements does not
	// count them; their presence marks that measure partial.
	CommerceReleases []Bucket
	// Open reconciliation items that carry no currency (unmatched provider
	// events, open corporate reconciliation findings).
	UnattributableOpen      int64
	UnattributableMaxTS     *time.Time
	CompletedReconciliation bool
	ReconciliationMaxTS     *time.Time
	// Currencies the ledger knows at DataAsOf (accounts and intents).
	Currencies []string
}

// Funding transaction types: the posting that secures the customer's money
// for a job or commerce order. A top-up (source_service payment-wallet-service)
// is wallet funding, not a transaction, and is excluded.
const SQLGrossTransactionValue = `
SELECT p.currency,
       COALESCE(sum(COALESCE(c.final_authorized_minor, p.amount_kobo)), 0)::bigint,
       count(*),
       max(t.created_at)
FROM ledger_transactions t
JOIN payment_intents p
  ON p.source_service = t.source_service AND p.source_reference = t.source_reference
LEFT JOIN commerce_payment_intents c ON c.payment_intent_id = p.id
WHERE t.transaction_type IN ('wallet_payment_hold', 'paystack_charge_success', 'commerce_payment_hold')
  AND t.created_at >= $1 AND t.created_at < $2
  AND p.source_service <> 'payment-wallet-service'
  AND NOT EXISTS (
        SELECT 1 FROM ledger_transactions f
        WHERE f.source_service = t.source_service
          AND f.source_reference = t.source_reference
          AND f.transaction_type IN ('wallet_payment_hold', 'paystack_charge_success', 'commerce_payment_hold')
          AND (f.created_at, f.id) < (t.created_at, t.id))
GROUP BY p.currency`

// Net platform fee: every posting to a platform_revenue account, credits
// minus debits, by the posting's own time. A settlement credits it once; a
// reversal or refund recovery debits it once, in the window it happened.
const SQLNetPlatformFee = `
SELECT e.currency,
       COALESCE(sum(CASE WHEN e.side = 'credit' THEN e.amount_kobo ELSE -e.amount_kobo END), 0)::bigint,
       count(DISTINCT t.id),
       max(t.created_at)
FROM ledger_entries e
JOIN ledger_transactions t ON t.id = e.transaction_id
JOIN wallet_accounts a ON a.id = e.account_id
WHERE a.account_type = 'platform_revenue'
  AND t.created_at >= $1 AND t.created_at < $2
GROUP BY e.currency`

// Provider earned: credit entries on provider_payable accounts, excluding
// withdrawal_* postings (a cancelled or reversed withdrawal returns money the
// provider already earned; it is not new earning).
const SQLProviderEarned = `
SELECT e.currency,
       COALESCE(sum(e.amount_kobo), 0)::bigint,
       count(*),
       max(t.created_at)
FROM ledger_entries e
JOIN ledger_transactions t ON t.id = e.transaction_id
JOIN wallet_accounts a ON a.id = e.account_id
WHERE a.account_type = 'provider_payable'
  AND e.side = 'credit'
  AND t.transaction_type NOT LIKE 'withdrawal\_%'
  AND t.created_at >= $1 AND t.created_at < $2
GROUP BY e.currency`

// Successful settlements: job payment intents whose escrow-release posting
// (independent job_settlement or managed_work_settlement) is in the window.
const SQLSuccessfulSettlements = `
SELECT p.currency,
       COALESCE(sum(p.amount_kobo), 0)::bigint,
       count(*),
       max(t.created_at)
FROM ledger_transactions t
JOIN payment_intents p
  ON p.source_service = t.source_service AND p.source_reference = t.source_reference
WHERE t.transaction_type IN ('job_settlement', 'managed_work_settlement')
  AND t.created_at >= $1 AND t.created_at < $2
GROUP BY p.currency`

const sqlCommerceReleases = `
SELECT p.currency, 0::bigint, count(*), max(o.created_at)
FROM commerce_financial_operations o
JOIN payment_intents p ON p.id = o.payment_intent_id
WHERE o.operation_type IN ('VENDOR_RELEASE', 'DELIVERY_RELEASE')
  AND o.status = 'APPLIED'
  AND o.created_at >= $1 AND o.created_at < $2
GROUP BY p.currency`

// Pending payouts at data_as_of ($1): legacy withdrawals and corporate
// provider withdrawals that are neither paid/completed nor terminally failed,
// rejected, cancelled or reversed.
const SQLPendingPayouts = `
SELECT currency, COALESCE(sum(amount), 0)::bigint, count(*), max(ts)
FROM (
    SELECT w.currency AS currency, w.amount_kobo AS amount, w.updated_at AS ts
    FROM withdrawals w
    WHERE w.status IN ('pending', 'processing', 'requires_otp') AND w.created_at <= $1
    UNION ALL
    SELECT f.currency::text, f.amount_minor, f.updated_at
    FROM finance_provider_withdrawals f
    WHERE f.status IN ('REQUESTED', 'PENDING_APPROVAL', 'APPROVED', 'PROCESSING', 'FAILED_RETRYABLE', 'DEAD_LETTER')
      AND f.created_at <= $1
) x
GROUP BY currency`

// Refunds completed in the window, each by its own completion time:
// legacy refunds (status processed) at their refund_processed /
// wallet_refund_processed posting, corporate finance_refunds at completed_at,
// commerce PARTIAL_REFUND operations at their APPLIED creation.
const SQLRefunds = `
SELECT currency, COALESCE(sum(amount), 0)::bigint, count(*), max(done)
FROM (
    SELECT r.currency AS currency, r.amount_kobo AS amount,
           LEAST(
             (SELECT min(t.created_at) FROM ledger_transactions t
               WHERE t.transaction_type = 'refund_processed'
                 AND t.source_service = 'payment-wallet-service'
                 AND t.source_reference = r.reference),
             (SELECT min(t.created_at) FROM ledger_transactions t
               WHERE t.transaction_type = 'wallet_refund_processed'
                 AND t.source_service = p.source_service
                 AND t.source_reference = p.source_reference)) AS done
    FROM refunds r
    JOIN payment_intents p ON p.id = r.payment_intent_id
    WHERE r.status = 'processed'
    UNION ALL
    SELECT f.currency::text, f.amount_minor, f.completed_at
    FROM finance_refunds f
    WHERE f.status = 'COMPLETED'
    UNION ALL
    SELECT p.currency, o.amount_minor, o.created_at
    FROM commerce_financial_operations o
    JOIN payment_intents p ON p.id = o.payment_intent_id
    WHERE o.operation_type = 'PARTIAL_REFUND' AND o.status = 'APPLIED' AND o.amount_minor > 0
) x
WHERE done >= $1 AND done < $2
GROUP BY currency`

// Pending liabilities at data_as_of ($1): the balance still held in job and
// commerce escrow accounts (credits minus debits). Count is the number of
// escrow accounts with a non-zero balance.
const SQLPendingLiabilities = `
SELECT currency, COALESCE(sum(balance), 0)::bigint, count(*) FILTER (WHERE balance <> 0), max(ts)
FROM (
    SELECT a.id, a.currency,
           sum(CASE WHEN e.side = 'credit' THEN e.amount_kobo ELSE -e.amount_kobo END) AS balance,
           max(t.created_at) AS ts
    FROM wallet_accounts a
    JOIN ledger_entries e ON e.account_id = a.id
    JOIN ledger_transactions t ON t.id = e.transaction_id
    WHERE a.account_type IN ('job_escrow', 'commerce_escrow') AND t.created_at <= $1
    GROUP BY a.id, a.currency
) x
GROUP BY currency`

// Provider events matched to exactly one Wallet row (intent by
// paystack_reference, else withdrawal by reference/transfer code, else refund
// by paystack_refund_reference), received at or before data_as_of.
func matchedEvents(asOf string) string {
	return strings.ReplaceAll(sqlMatchedEventsTemplate, "$AS_OF", asOf)
}

const sqlMatchedEventsTemplate = `
WITH m AS (
    SELECT w.received_at, w.processed_at, w.processing_error,
           COALESCE(pi.currency, wd.currency, rf.currency) AS currency,
           COALESCE(pi.amount_kobo, wd.amount_kobo, rf.amount_kobo) AS amount
    FROM paystack_webhook_events w
    LEFT JOIN payment_intents pi
      ON w.reference <> '' AND pi.paystack_reference = w.reference
    LEFT JOIN LATERAL (
        SELECT x.currency, x.amount_kobo FROM withdrawals x
        WHERE pi.id IS NULL AND w.reference <> ''
          AND (x.reference = w.reference OR x.paystack_transfer_code = w.reference)
        ORDER BY x.id LIMIT 1) wd ON true
    LEFT JOIN refunds rf
      ON pi.id IS NULL AND wd.currency IS NULL AND w.reference <> ''
     AND rf.paystack_refund_reference = w.reference
    WHERE w.received_at <= $AS_OF
)`

// Reconciled: matched events processed without error in the window.
var SQLReconciledTransactions = matchedEvents("$3") + `
SELECT currency, COALESCE(sum(amount), 0)::bigint, count(*), max(processed_at)
FROM m
WHERE currency IS NOT NULL AND processing_error IS NULL
  AND processed_at >= $1 AND processed_at < $2
GROUP BY currency`

// Unreconciled at data_as_of, attributable to a currency: matched events not
// yet processed or processed with an error, plus OPEN
// managed_settlement_reconciliations.
var SQLUnreconciledTransactions = matchedEvents("$1") + `
SELECT currency, COALESCE(sum(amount), 0)::bigint, count(*), max(ts)
FROM (
    SELECT currency, amount, GREATEST(received_at, processed_at) AS ts FROM m
    WHERE currency IS NOT NULL AND (processed_at IS NULL OR processing_error IS NOT NULL)
    UNION ALL
    SELECT r.currency, r.held_amount_minor, r.last_observed_at
    FROM managed_settlement_reconciliations r
    WHERE r.status = 'OPEN' AND r.first_observed_at <= $1
) x
GROUP BY currency`

// Open items that carry no currency: provider events matched to no Wallet
// row, and unresolved corporate reconciliation findings.
var sqlUnattributable = matchedEvents("$1") + `
SELECT count(*), max(ts) FROM (
    SELECT GREATEST(received_at, processed_at) AS ts FROM m WHERE currency IS NULL
    UNION ALL
    SELECT created_at FROM finance_reconciliation_findings
    WHERE resolved_at IS NULL AND created_at <= $1
) x`

const sqlCompletedReconciliation = `
SELECT count(*) > 0, max(completed_at) FROM finance_reconciliation_runs
WHERE status = 'COMPLETED' AND completed_at <= $1`

const sqlCurrencies = `
SELECT currency FROM wallet_accounts WHERE created_at <= $1
UNION
SELECT currency FROM payment_intents WHERE created_at <= $1`

// Measure names, mirrored from shared/go/financereport.Measures().
const (
	MeasureGTV            = "gross_transaction_value"
	MeasureNetPlatformFee = "net_platform_fee"
	MeasureProviderEarned = "provider_earned"
	MeasureSettlements    = "successful_settlements"
	MeasurePendingPayouts = "pending_payouts"
	MeasureRefunds        = "refunds"
	MeasureLiabilities    = "pending_liabilities"
	MeasureReconciled     = "reconciled_transactions"
	MeasureUnreconciled   = "unreconciled_transactions"
)

// SQLByMeasure exposes each measure's query so tests can pin the published
// definitions to the SQL actually executed.
var SQLByMeasure = map[string]string{
	MeasureGTV:            SQLGrossTransactionValue,
	MeasureNetPlatformFee: SQLNetPlatformFee,
	MeasureProviderEarned: SQLProviderEarned,
	MeasureSettlements:    SQLSuccessfulSettlements,
	MeasurePendingPayouts: SQLPendingPayouts,
	MeasureRefunds:        SQLRefunds,
	MeasureLiabilities:    SQLPendingLiabilities,
	MeasureReconciled:     SQLReconciledTransactions,
	MeasureUnreconciled:   SQLUnreconciledTransactions,
}

type Repository struct{ DB *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{DB: db} }

// Read computes the snapshot in one REPEATABLE READ, READ ONLY transaction.
// data_as_of is clock_timestamp() of the first statement: the snapshot is
// taken at that statement's start, so every visible row committed -- and was
// timestamped -- before it.
func (r *Repository) Read(ctx context.Context, start, end time.Time) (Snapshot, error) {
	snap := Snapshot{Measures: map[string][]Bucket{}}
	tx, err := r.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return snap, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&snap.DataAsOf); err != nil {
		return snap, err
	}
	asOf := snap.DataAsOf
	windowEnd := end
	if asOf.Before(windowEnd) {
		windowEnd = asOf
	}
	if !windowEnd.After(start) {
		// The whole window is after data_as_of: nothing can have happened in it
		// yet. An empty half-open interval keeps every query uniform.
		windowEnd = start
	}

	windowQueries := []struct{ measure, sql string }{
		{MeasureGTV, SQLGrossTransactionValue},
		{MeasureNetPlatformFee, SQLNetPlatformFee},
		{MeasureProviderEarned, SQLProviderEarned},
		{MeasureSettlements, SQLSuccessfulSettlements},
		{MeasureRefunds, SQLRefunds},
	}
	for _, q := range windowQueries {
		if snap.Measures[q.measure], err = buckets(ctx, tx, q.sql, start, windowEnd); err != nil {
			return snap, err
		}
	}
	if snap.CommerceReleases, err = buckets(ctx, tx, sqlCommerceReleases, start, windowEnd); err != nil {
		return snap, err
	}
	for _, q := range []struct{ measure, sql string }{
		{MeasurePendingPayouts, SQLPendingPayouts},
		{MeasureLiabilities, SQLPendingLiabilities},
	} {
		if snap.Measures[q.measure], err = buckets(ctx, tx, q.sql, asOf); err != nil {
			return snap, err
		}
	}
	if snap.Measures[MeasureReconciled], err = buckets(ctx, tx, SQLReconciledTransactions, start, windowEnd, asOf); err != nil {
		return snap, err
	}
	if snap.Measures[MeasureUnreconciled], err = buckets(ctx, tx, SQLUnreconciledTransactions, asOf); err != nil {
		return snap, err
	}
	if err = tx.QueryRow(ctx, sqlUnattributable, asOf).Scan(&snap.UnattributableOpen, &snap.UnattributableMaxTS); err != nil {
		return snap, err
	}
	if err = tx.QueryRow(ctx, sqlCompletedReconciliation, asOf).Scan(&snap.CompletedReconciliation, &snap.ReconciliationMaxTS); err != nil {
		return snap, err
	}
	rows, err := tx.Query(ctx, sqlCurrencies, asOf)
	if err != nil {
		return snap, err
	}
	for rows.Next() {
		var currency string
		if err = rows.Scan(&currency); err != nil {
			rows.Close()
			return snap, err
		}
		snap.Currencies = append(snap.Currencies, currency)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return snap, err
	}
	return snap, tx.Commit(ctx)
}

func buckets(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]Bucket, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bucket
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.Currency, &b.Amount, &b.Count, &b.MaxTS); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
