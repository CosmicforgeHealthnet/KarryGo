package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/ids"
)

// LedgerRepository owns the chart of accounts, the journal and the atomic
// balance projection.
type LedgerRepository struct {
	db database.Querier
}

// NewLedgerRepository wires the repository.
func NewLedgerRepository(db database.Querier) *LedgerRepository {
	return &LedgerRepository{db: db}
}

const ledgerAccountColumns = `id, COALESCE(organization_id, ''), owner_type, owner_ref,
	account_code, currency, normal_balance, status, version, created_at, updated_at`

func scanLedgerAccount(row pgx.Row) (*models.LedgerAccount, error) {
	var account models.LedgerAccount
	err := row.Scan(&account.ID, &account.OrganizationID, &account.OwnerType, &account.OwnerRef,
		&account.AccountCode, &account.Currency, &account.NormalBalance, &account.Status,
		&account.Version, &account.CreatedAt, &account.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan ledger account: %w", err)
	}
	return &account, nil
}

// EnsureAccount returns the account for the given identity, creating it (and
// its zeroed balance row) on first use.
//
// Account creation is idempotent by the natural key so two concurrent
// postings against a brand new wallet cannot create two accounts.
func (r *LedgerRepository) EnsureAccount(ctx context.Context, q database.Querier,
	organizationID string, ownerType string, ownerRef string, accountCode string,
	currency string, normalBalance string) (*models.LedgerAccount, error) {
	if q == nil {
		q = r.db
	}
	id := ids.New(ids.PrefixLedgerAccount)
	_, err := q.Exec(ctx, `
		INSERT INTO finance_ledger_accounts
			(id, organization_id, owner_type, owner_ref, account_code, currency,
			 normal_balance, status, version, created_at, updated_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, 'ACTIVE', 1, now(), now())
		ON CONFLICT (owner_type, owner_ref, account_code, currency) DO NOTHING`,
		id, organizationID, ownerType, ownerRef, accountCode, currency, normalBalance)
	if err != nil {
		return nil, fmt.Errorf("finance: ensure ledger account: %w", err)
	}

	account, err := scanLedgerAccount(q.QueryRow(ctx, `
		SELECT `+ledgerAccountColumns+` FROM finance_ledger_accounts
		WHERE owner_type = $1 AND owner_ref = $2 AND account_code = $3 AND currency = $4`,
		ownerType, ownerRef, accountCode, currency))
	if err != nil {
		return nil, err
	}

	if _, err := q.Exec(ctx, `
		INSERT INTO finance_account_balances (ledger_account_id, currency, debit_minor, credit_minor, version, updated_at)
		VALUES ($1, $2, 0, 0, 1, now())
		ON CONFLICT (ledger_account_id) DO NOTHING`, account.ID, currency); err != nil {
		return nil, fmt.Errorf("finance: ensure balance row: %w", err)
	}
	return account, nil
}

// GetAccount loads an account by ID.
func (r *LedgerRepository) GetAccount(ctx context.Context, q database.Querier,
	id string) (*models.LedgerAccount, error) {
	if q == nil {
		q = r.db
	}
	return scanLedgerAccount(q.QueryRow(ctx,
		`SELECT `+ledgerAccountColumns+` FROM finance_ledger_accounts WHERE id = $1`, id))
}

// SetAccountStatus restricts or closes an account.
func (r *LedgerRepository) SetAccountStatus(ctx context.Context, q database.Querier,
	id string, status string) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `
		UPDATE finance_ledger_accounts SET status = $2, version = version + 1, updated_at = now()
		WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("finance: set account status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Balance reads the atomic balance projection for one account.
func (r *LedgerRepository) Balance(ctx context.Context, q database.Querier,
	ledgerAccountID string) (*models.Balance, error) {
	if q == nil {
		q = r.db
	}
	var balance models.Balance
	err := q.QueryRow(ctx, `
		SELECT b.ledger_account_id, b.currency, b.debit_minor, b.credit_minor,
		       a.normal_balance, b.version, b.updated_at
		FROM finance_account_balances b
		JOIN finance_ledger_accounts a ON a.id = b.ledger_account_id
		WHERE b.ledger_account_id = $1`, ledgerAccountID).
		Scan(&balance.LedgerAccountID, &balance.Currency, &balance.DebitMinor,
			&balance.CreditMinor, &balance.NormalBalance, &balance.Version, &balance.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: read balance: %w", err)
	}
	return &balance, nil
}

// RecomputeBalance re-aggregates a balance from the journal lines themselves.
// Reconciliation uses it to detect drift between the projection and the
// authoritative postings.
func (r *LedgerRepository) RecomputeBalance(ctx context.Context, q database.Querier,
	ledgerAccountID string) (debitMinor int64, creditMinor int64, err error) {
	if q == nil {
		q = r.db
	}
	err = q.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_minor) FILTER (WHERE side = 'DEBIT'), 0),
		       COALESCE(sum(amount_minor) FILTER (WHERE side = 'CREDIT'), 0)
		FROM finance_journal_lines WHERE ledger_account_id = $1`, ledgerAccountID).
		Scan(&debitMinor, &creditMinor)
	if err != nil {
		return 0, 0, fmt.Errorf("finance: recompute balance: %w", err)
	}
	return debitMinor, creditMinor, nil
}

// PostingLine describes one line of a proposed journal.
type PostingLine struct {
	LedgerAccountID string
	Side            string
	AmountMinor     int64
	Memo            string
}

// Posting describes a complete proposed journal.
type Posting struct {
	OrganizationID    string
	JournalType       string
	Currency          string
	Reference         string
	SourceService     string
	SourceReference   string
	IdempotencyKey    string
	Fingerprint       string
	CorrelationID     string
	CausationID       string
	ReversesJournalID string
	ActorType         string
	ActorID           string
	Memo              string
	Lines             []PostingLine
}

// Post writes one balanced journal and moves the balance projection, inside
// the caller's transaction.
//
// The sequence matters:
//
//  1. the lines are validated in Go so the caller gets a precise error;
//  2. the journal header and lines are inserted;
//  3. the balance projection is moved by the same amounts.
//
// The database's deferred constraint trigger re-checks the balance at COMMIT,
// so even a caller that bypassed step 1 cannot leave an unbalanced journal
// behind. Step 3 runs in the same transaction, which is what makes the
// projection and the postings incapable of disagreeing.
func (r *LedgerRepository) Post(ctx context.Context, q database.Querier,
	posting Posting) (*models.Journal, error) {
	if q == nil {
		q = r.db
	}
	if posting.Currency == "" {
		return nil, fmt.Errorf("finance: a journal requires an explicit currency")
	}

	lines := make([]models.JournalLine, 0, len(posting.Lines))
	for index, line := range posting.Lines {
		lines = append(lines, models.JournalLine{
			LineNo:          index + 1,
			LedgerAccountID: line.LedgerAccountID,
			Side:            line.Side,
			AmountMinor:     line.AmountMinor,
			Currency:        posting.Currency,
			Memo:            line.Memo,
		})
	}
	if err := models.ValidateBalanced(posting.Currency, lines); err != nil {
		return nil, err
	}

	journal := &models.Journal{
		ID:                 ids.New(ids.PrefixJournal),
		OrganizationID:     posting.OrganizationID,
		JournalType:        posting.JournalType,
		Currency:           posting.Currency,
		Status:             "POSTED",
		Reference:          posting.Reference,
		SourceService:      posting.SourceService,
		SourceReference:    posting.SourceReference,
		IdempotencyKey:     posting.IdempotencyKey,
		RequestFingerprint: posting.Fingerprint,
		CorrelationID:      posting.CorrelationID,
		CausationID:        posting.CausationID,
		ReversesJournalID:  posting.ReversesJournalID,
		PostedByActorType:  defaultString(posting.ActorType, "SYSTEM"),
		PostedByActorID:    defaultString(posting.ActorID, "system"),
		Memo:               posting.Memo,
		PostedAt:           time.Now().UTC(),
	}

	_, err := q.Exec(ctx, `
		INSERT INTO finance_journals
			(id, organization_id, journal_type, currency, status, reference, source_service,
			 source_reference, idempotency_key, request_fingerprint, correlation_id, causation_id,
			 reverses_journal_id, posted_by_actor_type, posted_by_actor_id, memo, posted_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, 'POSTED', $5, $6, $7, $8, $9, $10, $11,
		        NULLIF($12, ''), $13, $14, $15, $16)`,
		journal.ID, journal.OrganizationID, journal.JournalType, journal.Currency,
		journal.Reference, journal.SourceService, journal.SourceReference,
		journal.IdempotencyKey, journal.RequestFingerprint, journal.CorrelationID,
		journal.CausationID, journal.ReversesJournalID, journal.PostedByActorType,
		journal.PostedByActorID, journal.Memo, journal.PostedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("finance: insert journal: %w", err)
	}

	for index := range lines {
		line := &lines[index]
		line.ID = ids.New(ids.PrefixJournalLine)
		line.JournalID = journal.ID
		if _, err := q.Exec(ctx, `
			INSERT INTO finance_journal_lines
				(id, journal_id, line_no, ledger_account_id, side, amount_minor, currency, memo, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())`,
			line.ID, line.JournalID, line.LineNo, line.LedgerAccountID,
			line.Side, line.AmountMinor, line.Currency, line.Memo); err != nil {
			return nil, fmt.Errorf("finance: insert journal line: %w", err)
		}

		column := "debit_minor"
		if line.Side == models.SideCredit {
			column = "credit_minor"
		}
		tag, err := q.Exec(ctx, `
			UPDATE finance_account_balances SET
				`+column+` = `+column+` + $2, version = version + 1, updated_at = now()
			WHERE ledger_account_id = $1 AND currency = $3`,
			line.LedgerAccountID, line.AmountMinor, line.Currency)
		if err != nil {
			return nil, fmt.Errorf("finance: move balance projection: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// A posting against an account with no balance row (or with a
			// different currency) would silently lose the movement.
			return nil, fmt.Errorf("finance: no %s balance row for ledger account %s",
				line.Currency, line.LedgerAccountID)
		}
	}

	journal.Lines = lines
	return journal, nil
}

const journalColumns = `id, COALESCE(organization_id, ''), journal_type, currency, status,
	reference, source_service, source_reference, idempotency_key, request_fingerprint,
	correlation_id, causation_id, COALESCE(reverses_journal_id, ''),
	posted_by_actor_type, posted_by_actor_id, memo, posted_at`

func scanJournal(row pgx.Row) (*models.Journal, error) {
	var journal models.Journal
	err := row.Scan(&journal.ID, &journal.OrganizationID, &journal.JournalType, &journal.Currency,
		&journal.Status, &journal.Reference, &journal.SourceService, &journal.SourceReference,
		&journal.IdempotencyKey, &journal.RequestFingerprint, &journal.CorrelationID,
		&journal.CausationID, &journal.ReversesJournalID, &journal.PostedByActorType,
		&journal.PostedByActorID, &journal.Memo, &journal.PostedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan journal: %w", err)
	}
	return &journal, nil
}

// GetJournal loads one journal with its lines.
func (r *LedgerRepository) GetJournal(ctx context.Context, q database.Querier,
	id string) (*models.Journal, error) {
	if q == nil {
		q = r.db
	}
	journal, err := scanJournal(q.QueryRow(ctx,
		`SELECT `+journalColumns+` FROM finance_journals WHERE id = $1`, id))
	if err != nil {
		return nil, err
	}
	lines, err := r.JournalLines(ctx, q, id)
	if err != nil {
		return nil, err
	}
	journal.Lines = lines
	return journal, nil
}

// FindJournalByIdempotencyKey resolves a replayed posting.
func (r *LedgerRepository) FindJournalByIdempotencyKey(ctx context.Context, q database.Querier,
	key string) (*models.Journal, error) {
	if q == nil {
		q = r.db
	}
	if key == "" {
		return nil, ErrNotFound
	}
	return scanJournal(q.QueryRow(ctx,
		`SELECT `+journalColumns+` FROM finance_journals WHERE idempotency_key = $1`, key))
}

// JournalLines loads the ordered lines of a journal.
func (r *LedgerRepository) JournalLines(ctx context.Context, q database.Querier,
	journalID string) ([]models.JournalLine, error) {
	if q == nil {
		q = r.db
	}
	rows, err := q.Query(ctx, `
		SELECT id, journal_id, line_no, ledger_account_id, side, amount_minor, currency, memo, created_at
		FROM finance_journal_lines WHERE journal_id = $1 ORDER BY line_no`, journalID)
	if err != nil {
		return nil, fmt.Errorf("finance: list journal lines: %w", err)
	}
	defer rows.Close()

	var lines []models.JournalLine
	for rows.Next() {
		var line models.JournalLine
		if err := rows.Scan(&line.ID, &line.JournalID, &line.LineNo, &line.LedgerAccountID,
			&line.Side, &line.AmountMinor, &line.Currency, &line.Memo, &line.CreatedAt); err != nil {
			return nil, fmt.Errorf("finance: scan journal line: %w", err)
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

// IsReversed reports whether a compensating journal already exists for id.
func (r *LedgerRepository) IsReversed(ctx context.Context, q database.Querier,
	journalID string) (bool, error) {
	if q == nil {
		q = r.db
	}
	var exists bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM finance_journals WHERE reverses_journal_id = $1)`,
		journalID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("finance: check reversal: %w", err)
	}
	return exists, nil
}

// Reverse posts the compensating entry for journalID: the same lines with
// every side flipped.
//
// The original is never edited. Reading the pair together is what makes a
// correction auditable — the mistake and its correction both remain visible.
func (r *LedgerRepository) Reverse(ctx context.Context, q database.Querier, journalID string,
	journalType string, reason string, actorType string, actorID string,
	idempotencyKey string, correlationID string) (*models.Journal, error) {
	if q == nil {
		q = r.db
	}
	original, err := r.GetJournal(ctx, q, journalID)
	if err != nil {
		return nil, err
	}
	reversed, err := r.IsReversed(ctx, q, journalID)
	if err != nil {
		return nil, err
	}
	if reversed {
		return nil, ErrConflict
	}

	lines := make([]PostingLine, 0, len(original.Lines))
	for _, line := range original.Lines {
		side := models.SideCredit
		if line.Side == models.SideCredit {
			side = models.SideDebit
		}
		lines = append(lines, PostingLine{
			LedgerAccountID: line.LedgerAccountID,
			Side:            side,
			AmountMinor:     line.AmountMinor,
			Memo:            "reversal of " + line.Memo,
		})
	}

	return r.Post(ctx, q, Posting{
		OrganizationID:    original.OrganizationID,
		JournalType:       defaultString(journalType, models.JournalReversal),
		Currency:          original.Currency,
		Reference:         original.Reference,
		SourceService:     original.SourceService,
		SourceReference:   original.SourceReference,
		IdempotencyKey:    idempotencyKey,
		CorrelationID:     correlationID,
		CausationID:       original.ID,
		ReversesJournalID: original.ID,
		ActorType:         actorType,
		ActorID:           actorID,
		Memo:              reason,
		Lines:             lines,
	})
}

// ReversibleMinor reports how much of a posted journal may still be reversed.
//
// The ceiling is the original's debit total less everything already claimed
// against it — live refunds and live reversals both count. Rejected and
// permanently failed attempts released their claim and do not.
func (r *LedgerRepository) ReversibleMinor(ctx context.Context, q database.Querier,
	journalID string) (int64, error) {
	if q == nil {
		q = r.db
	}
	var totalMinor int64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_minor) FILTER (WHERE side = 'DEBIT'), 0)
		FROM finance_journal_lines WHERE journal_id = $1`, journalID).Scan(&totalMinor)
	if err != nil {
		return 0, fmt.Errorf("finance: read original journal total: %w", err)
	}
	if totalMinor == 0 {
		return 0, ErrNotFound
	}
	var claimedMinor int64
	err = q.QueryRow(ctx, `
		SELECT COALESCE((SELECT sum(amount_minor) FROM finance_refunds
		                 WHERE original_journal_id = $1
		                   AND status IN ('REQUESTED', 'APPROVED', 'PROCESSING', 'UNKNOWN',
		                                  'COMPLETED', 'RECONCILING')), 0)
		     + COALESCE((SELECT sum(amount_minor) FROM finance_reversals
		                 WHERE original_journal_id = $1
		                   AND status IN ('REQUESTED', 'PENDING_APPROVAL', 'APPROVED',
		                                  'PROCESSING', 'UNKNOWN', 'COMPLETED',
		                                  'RECONCILING', 'FAILED_RETRYABLE')), 0)`,
		journalID).Scan(&claimedMinor)
	if err != nil {
		return 0, fmt.Errorf("finance: read cumulative reversals: %w", err)
	}
	remaining := totalMinor - claimedMinor
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}

// JournalTotalMinor is the original's debit total: the largest amount a single
// compensating entry could ever carry.
func (r *LedgerRepository) JournalTotalMinor(ctx context.Context, q database.Querier,
	journalID string) (int64, error) {
	if q == nil {
		q = r.db
	}
	var totalMinor int64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_minor) FILTER (WHERE side = 'DEBIT'), 0)
		FROM finance_journal_lines WHERE journal_id = $1`, journalID).Scan(&totalMinor)
	if err != nil {
		return 0, fmt.Errorf("finance: read journal total: %w", err)
	}
	if totalMinor == 0 {
		return 0, ErrNotFound
	}
	return totalMinor, nil
}

// PostCompensating posts the compensating entry for amountMinor of journalID.
//
// A full-value compensation is the exact mirror of the original and is linked
// through reverses_journal_id, which the database allows only once. A partial
// compensation cannot claim that link — a second partial would then be
// impossible — so it is linked by causation instead, and its lines are scaled
// by largest remainder within each side. Scaling per side is what guarantees
// debits and credits still agree exactly after rounding.
func (r *LedgerRepository) PostCompensating(ctx context.Context, q database.Querier,
	journalID string, amountMinor int64, journalType string, reason string,
	actorType string, actorID string, idempotencyKey string,
	correlationID string) (*models.Journal, error) {
	if q == nil {
		q = r.db
	}
	if amountMinor <= 0 {
		return nil, fmt.Errorf("finance: compensating amount must be positive")
	}
	original, err := r.GetJournal(ctx, q, journalID)
	if err != nil {
		return nil, err
	}
	total, err := r.JournalTotalMinor(ctx, q, journalID)
	if err != nil {
		return nil, err
	}
	if amountMinor > total {
		return nil, fmt.Errorf("finance: compensating amount exceeds the original journal")
	}
	if amountMinor == total {
		return r.Reverse(ctx, q, journalID, defaultString(journalType, models.JournalReversal),
			reason, actorType, actorID, idempotencyKey, correlationID)
	}

	debits := make([]models.JournalLine, 0, len(original.Lines))
	credits := make([]models.JournalLine, 0, len(original.Lines))
	for _, line := range original.Lines {
		if line.Side == models.SideDebit {
			debits = append(debits, line)
		} else {
			credits = append(credits, line)
		}
	}
	debitShares := allocateProportionally(debits, amountMinor)
	creditShares := allocateProportionally(credits, amountMinor)

	lines := make([]PostingLine, 0, len(original.Lines))
	// Sides are flipped: the compensating entry undoes the original.
	for index, line := range debits {
		if debitShares[index] <= 0 {
			continue
		}
		lines = append(lines, PostingLine{LedgerAccountID: line.LedgerAccountID,
			Side: models.SideCredit, AmountMinor: debitShares[index],
			Memo: "partial reversal of " + line.Memo})
	}
	for index, line := range credits {
		if creditShares[index] <= 0 {
			continue
		}
		lines = append(lines, PostingLine{LedgerAccountID: line.LedgerAccountID,
			Side: models.SideDebit, AmountMinor: creditShares[index],
			Memo: "partial reversal of " + line.Memo})
	}

	return r.Post(ctx, q, Posting{
		OrganizationID:  original.OrganizationID,
		JournalType:     defaultString(journalType, models.JournalReversal),
		Currency:        original.Currency,
		Reference:       original.Reference,
		SourceService:   original.SourceService,
		SourceReference: original.SourceReference,
		IdempotencyKey:  idempotencyKey,
		CorrelationID:   correlationID,
		CausationID:     original.ID,
		ActorType:       actorType,
		ActorID:         actorID,
		Memo:            reason,
		Lines:           lines,
	})
}

// allocateProportionally splits amount across lines in proportion to their
// value, giving the rounding remainder to the largest fractional parts. The
// result always sums to exactly amount.
func allocateProportionally(lines []models.JournalLine, amount int64) []int64 {
	shares := make([]int64, len(lines))
	if len(lines) == 0 || amount <= 0 {
		return shares
	}
	var total int64
	for _, line := range lines {
		total += line.AmountMinor
	}
	if total <= 0 {
		return shares
	}
	remainders := make([]int64, len(lines))
	var assigned int64
	for index, line := range lines {
		product := line.AmountMinor * amount
		shares[index] = product / total
		remainders[index] = product % total
		assigned += shares[index]
	}
	for assigned < amount {
		best, bestRemainder := -1, int64(-1)
		for index := range lines {
			if remainders[index] > bestRemainder {
				best, bestRemainder = index, remainders[index]
			}
		}
		if best < 0 {
			break
		}
		shares[best]++
		remainders[best] = -1
		assigned++
	}
	return shares
}

// JournalFilter narrows a journal listing.
type JournalFilter struct {
	OrganizationID string
	Currency       string
	JournalType    string
	From           *time.Time
	To             *time.Time
	Limit          int
}

// ListJournals returns journals for one organization, newest first.
func (r *LedgerRepository) ListJournals(ctx context.Context, q database.Querier,
	filter JournalFilter) ([]models.Journal, error) {
	if q == nil {
		q = r.db
	}
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
	}
	rows, err := q.Query(ctx, `
		SELECT `+journalColumns+` FROM finance_journals
		WHERE organization_id = $1
		  AND ($2 = '' OR currency = $2)
		  AND ($3 = '' OR journal_type = $3)
		  AND ($4::timestamptz IS NULL OR posted_at >= $4)
		  AND ($5::timestamptz IS NULL OR posted_at < $5)
		ORDER BY posted_at DESC, id DESC
		LIMIT $6`,
		filter.OrganizationID, filter.Currency, filter.JournalType,
		filter.From, filter.To, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list journals: %w", err)
	}
	defer rows.Close()

	var journals []models.Journal
	for rows.Next() {
		var journal models.Journal
		if err := rows.Scan(&journal.ID, &journal.OrganizationID, &journal.JournalType,
			&journal.Currency, &journal.Status, &journal.Reference, &journal.SourceService,
			&journal.SourceReference, &journal.IdempotencyKey, &journal.RequestFingerprint,
			&journal.CorrelationID, &journal.CausationID, &journal.ReversesJournalID,
			&journal.PostedByActorType, &journal.PostedByActorID, &journal.Memo,
			&journal.PostedAt); err != nil {
			return nil, fmt.Errorf("finance: scan journal: %w", err)
		}
		journals = append(journals, journal)
	}
	return journals, rows.Err()
}

// RefundableMinor reports how much of a posted journal has not already been
// refunded. A refund may never exceed this.
func (r *LedgerRepository) RefundableMinor(ctx context.Context, q database.Querier,
	journalID string) (int64, error) {
	if q == nil {
		q = r.db
	}
	var totalMinor, refundedMinor int64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_minor) FILTER (WHERE side = 'DEBIT'), 0)
		FROM finance_journal_lines WHERE journal_id = $1`, journalID).Scan(&totalMinor)
	if err != nil {
		return 0, fmt.Errorf("finance: read original journal total: %w", err)
	}
	if totalMinor == 0 {
		return 0, ErrNotFound
	}
	// Only refunds that are still live count against the ceiling: rejected and
	// permanently failed attempts released their claim on the original.
	err = q.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_minor), 0) FROM finance_refunds
		WHERE original_journal_id = $1
		  AND status IN ('REQUESTED', 'APPROVED', 'PROCESSING', 'UNKNOWN',
		                 'COMPLETED', 'RECONCILING')`, journalID).Scan(&refundedMinor)
	if err != nil {
		return 0, fmt.Errorf("finance: read cumulative refunds: %w", err)
	}
	remaining := totalMinor - refundedMinor
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}

// UnbalancedJournals finds journals whose lines no longer balance. It should
// always return nothing; reconciliation runs it as a standing proof.
func (r *LedgerRepository) UnbalancedJournals(ctx context.Context, q database.Querier,
	limit int) ([]string, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.Query(ctx, `
		SELECT journal_id FROM finance_journal_lines
		GROUP BY journal_id
		HAVING COALESCE(sum(amount_minor) FILTER (WHERE side = 'DEBIT'), 0)
		    <> COALESCE(sum(amount_minor) FILTER (WHERE side = 'CREDIT'), 0)
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("finance: find unbalanced journals: %w", err)
	}
	defer rows.Close()

	var journalIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("finance: scan unbalanced journal: %w", err)
		}
		journalIDs = append(journalIDs, id)
	}
	return journalIDs, rows.Err()
}

// DriftedBalances finds accounts whose projection disagrees with the postings.
func (r *LedgerRepository) DriftedBalances(ctx context.Context, q database.Querier,
	limit int) ([]models.Balance, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.Query(ctx, `
		SELECT b.ledger_account_id, b.currency, b.debit_minor, b.credit_minor,
		       a.normal_balance, b.version, b.updated_at
		FROM finance_account_balances b
		JOIN finance_ledger_accounts a ON a.id = b.ledger_account_id
		LEFT JOIN (
			SELECT ledger_account_id,
			       COALESCE(sum(amount_minor) FILTER (WHERE side = 'DEBIT'), 0) AS debit_minor,
			       COALESCE(sum(amount_minor) FILTER (WHERE side = 'CREDIT'), 0) AS credit_minor
			FROM finance_journal_lines GROUP BY ledger_account_id
		) posted ON posted.ledger_account_id = b.ledger_account_id
		WHERE b.debit_minor <> COALESCE(posted.debit_minor, 0)
		   OR b.credit_minor <> COALESCE(posted.credit_minor, 0)
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("finance: find drifted balances: %w", err)
	}
	defer rows.Close()

	var balances []models.Balance
	for rows.Next() {
		var balance models.Balance
		if err := rows.Scan(&balance.LedgerAccountID, &balance.Currency, &balance.DebitMinor,
			&balance.CreditMinor, &balance.NormalBalance, &balance.Version,
			&balance.UpdatedAt); err != nil {
			return nil, fmt.Errorf("finance: scan drifted balance: %w", err)
		}
		balances = append(balances, balance)
	}
	return balances, rows.Err()
}

// StatementLines returns the postings touching an owner's accounts in a
// period, oldest first.
func (r *LedgerRepository) StatementLines(ctx context.Context, q database.Querier,
	ownerType string, ownerRef string, currency string,
	from time.Time, to time.Time) ([]models.StatementLine, error) {
	if q == nil {
		q = r.db
	}
	rows, err := q.Query(ctx, `
		SELECT j.id, j.journal_type, j.reference, l.side, l.amount_minor, l.currency,
		       l.memo, j.posted_at
		FROM finance_journal_lines l
		JOIN finance_journals j ON j.id = l.journal_id
		JOIN finance_ledger_accounts a ON a.id = l.ledger_account_id
		WHERE a.owner_type = $1 AND a.owner_ref = $2 AND l.currency = $3
		  AND j.posted_at >= $4 AND j.posted_at < $5
		ORDER BY j.posted_at, l.line_no`,
		ownerType, ownerRef, currency, from, to)
	if err != nil {
		return nil, fmt.Errorf("finance: list statement lines: %w", err)
	}
	defer rows.Close()

	var lines []models.StatementLine
	for rows.Next() {
		var line models.StatementLine
		if err := rows.Scan(&line.JournalID, &line.JournalType, &line.Reference, &line.Side,
			&line.AmountMinor, &line.Currency, &line.Memo, &line.PostedAt); err != nil {
			return nil, fmt.Errorf("finance: scan statement line: %w", err)
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// rawJSON marshals a value for a jsonb column.
func rawJSON(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("finance: marshal json column: %w", err)
	}
	return encoded, nil
}
