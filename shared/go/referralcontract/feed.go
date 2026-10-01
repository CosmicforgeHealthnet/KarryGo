package referralcontract

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/outbox"
)

// Feed is the producer side of the referral feed for one service: it turns
// account facts into contract events and appends them to that service's
// referral_outbox inside the caller's transaction. Raw phone numbers, emails
// and device ids are hashed here and never leave the producing service.
type Feed struct {
	store    *outbox.Store
	producer string
	pepper   []byte
}

// NewFeed binds a feed to the producing service's outbox table. db is the
// pool the relay claims rows through. The pepper must be the same in every
// producer, or one person's accounts will not match up.
func NewFeed(db outbox.Querier, producer string, pepper []byte) (*Feed, error) {
	if strings.TrimSpace(producer) == "" {
		return nil, errors.New("referralcontract: feed needs the producing service name")
	}
	if len(pepper) == 0 {
		return nil, errors.New("referralcontract: feed needs the shared identity pepper")
	}
	store, err := outbox.NewStore(db, OutboxTable)
	if err != nil {
		return nil, err
	}
	return &Feed{store: store, producer: producer, pepper: pepper}, nil
}

// Account is a sign-up as the owning service knows it. Phone must already be
// normalized (+234...).
type Account struct {
	ActorType     string
	ActorID       string
	Phone         string
	Email         string
	DeviceID      string
	LinkCode      string
	TypedCode     string
	LocationState string
	SignupAt      time.Time
	// IsNewAccount is false only for the one-time backfill of accounts that
	// existed before the programme.
	IsNewAccount bool
}

// AccountRegistered appends the announcement of an account. Call it in the
// same transaction that created the account.
func (f *Feed) AccountRegistered(ctx context.Context, tx outbox.Querier, account Account) error {
	eventType := EventProviderRegistered
	if account.ActorType == ActorCustomer {
		eventType = EventCustomerRegistered
	}
	signupAt := account.SignupAt
	if signupAt.IsZero() {
		signupAt = time.Now()
	}
	payload := AccountRegistered{
		ActorType:     account.ActorType,
		ActorID:       account.ActorID,
		PhoneHash:     HashPhone(f.pepper, account.Phone),
		EmailHash:     HashEmail(f.pepper, account.Email),
		DeviceHash:    HashDevice(f.pepper, account.DeviceID),
		LinkCode:      strings.TrimSpace(account.LinkCode),
		TypedCode:     strings.TrimSpace(account.TypedCode),
		LocationState: strings.TrimSpace(account.LocationState),
		SignupAt:      signupAt.UTC(),
		IsNewAccount:  account.IsNewAccount,
	}
	if err := payload.Validate(); err != nil {
		return err
	}
	envelope, err := outbox.NewEnvelope(f.producer, eventType, "account", account.ActorType+":"+account.ActorID, 1, payload)
	if err != nil {
		return err
	}
	return f.store.Append(ctx, tx, envelope)
}

// ProviderOnboardingCompleted appends "this driver is now fully approved".
// Call it in the transaction that records the approval.
func (f *Feed) ProviderOnboardingCompleted(ctx context.Context, tx outbox.Querier, actorType, actorID string, completedAt time.Time) error {
	payload := ProviderOnboardingCompleted{ActorType: actorType, ActorID: actorID, CompletedAt: completedAt.UTC()}
	if err := payload.Validate(); err != nil {
		return err
	}
	envelope, err := outbox.NewEnvelope(f.producer, EventProviderOnboardingCompleted, "account", actorType+":"+actorID, 2, payload)
	if err != nil {
		return err
	}
	return f.store.Append(ctx, tx, envelope)
}

// Relay builds the worker that delivers this feed to referral-service at
// targetURL (a bare origin), signing as the producing service.
func (f *Feed) Relay(targetURL string, secret []byte, timeout time.Duration, onError func(error)) (*outbox.Worker, error) {
	publisher, err := outbox.NewHTTPPublisher(targetURL, IngestPath, f.producer, secret, timeout)
	if err != nil {
		return nil, err
	}
	return outbox.NewWorker(f.store, publisher, outbox.WorkerConfig{
		Owner:   f.producer + "-referral-relay",
		OnError: onError,
	}), nil
}

// ApprovalSource tells AnnounceApprovals where a producing service keeps its
// drivers and what "fully approved" means there. Approved and ApprovedAt are
// SQL written by the service itself (never user input); the table must have
// an id column and a referral_approval_announced_at timestamptz column.
type ApprovalSource struct {
	ActorType string
	Table     string
	// Approved is the SQL condition a fully approved driver row satisfies.
	Approved string
	// ApprovedAt is an SQL expression for when the driver was approved;
	// empty means the moment the sweep noticed.
	ApprovedAt string
}

// TxBeginner is satisfied by *pgxpool.Pool.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

var sqlIdentifier = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// AnnounceApprovals finds drivers who are fully approved but not yet
// announced, then announces and marks each one in a single transaction, so an
// approval is announced exactly once however it happened -- in code, through
// an admin tool, or by a manual database change. Concurrent sweeps skip each
// other's rows. It returns how many drivers it announced.
func (f *Feed) AnnounceApprovals(ctx context.Context, db TxBeginner, source ApprovalSource, limit int) (int, error) {
	if !IsProviderActor(source.ActorType) || !sqlIdentifier.MatchString(source.Table) || strings.TrimSpace(source.Approved) == "" {
		return 0, errors.New("referralcontract: approval source needs a driver actor type, a table and an approval condition")
	}
	if limit <= 0 {
		limit = 100
	}
	approvedAt := source.ApprovedAt
	if strings.TrimSpace(approvedAt) == "" {
		approvedAt = "now()"
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT %[1]s.id, %[2]s FROM %[1]s
		WHERE %[1]s.referral_approval_announced_at IS NULL AND (%[3]s)
		ORDER BY %[1]s.id LIMIT $1
		FOR UPDATE OF %[1]s SKIP LOCKED`, source.Table, approvedAt, source.Approved), limit)
	if err != nil {
		return 0, fmt.Errorf("referralcontract: find approvals: %w", err)
	}
	type approval struct {
		id string
		at time.Time
	}
	var approvals []approval
	for rows.Next() {
		var a approval
		if err := rows.Scan(&a.id, &a.at); err != nil {
			rows.Close()
			return 0, err
		}
		approvals = append(approvals, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, a := range approvals {
		if err := f.ProviderOnboardingCompleted(ctx, tx, source.ActorType, a.id, a.at); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET referral_approval_announced_at = now() WHERE id = $1`, source.Table), a.id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(approvals), nil
}
