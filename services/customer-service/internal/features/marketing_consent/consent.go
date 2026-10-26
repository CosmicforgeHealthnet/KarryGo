// Package marketingconsent is customer-service's marketing consent authority
// (Section 17). Consent is captured per channel from the customer, kept as a
// versioned current row plus an append-only history, and exposed to
// notification-service through two narrow internal reads: the opted-in
// audience of one market/channel, and a point-in-time re-check of specific
// customers immediately before a send.
//
// Nothing here defaults, infers or backfills consent. A customer with no row
// has given no consent; an unknown market never matches a targeted market.
package marketingconsent

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusOptedIn     = "OPTED_IN"
	StatusWithdrawn   = "WITHDRAWN"
	StatusNotRecorded = "NOT_RECORDED"

	SourceCustomerApp    = "CUSTOMER_APP"
	SourcePrivacyRequest = "ADMIN_PRIVACY_WITHDRAWAL"

	// MaxPage bounds the audience page and the eligibility batch.
	MaxPage = 500

	ReasonOptedIn        = "OPTED_IN"
	ReasonNotOptedIn     = "NOT_OPTED_IN"
	ReasonAccountMissing = "ACCOUNT_NOT_FOUND"
	ReasonInactive       = "ACCOUNT_INACTIVE"
	ReasonNoEmail        = "NO_EMAIL_ADDRESS"
)

// Channels are the marketing channels consent is captured for.
var Channels = []string{"in_app", "push", "email"}

var (
	ErrInvalid     = errors.New("invalid marketing consent request")
	ErrUnavailable = errors.New("marketing consent authority unavailable")
	ErrNotFound    = errors.New("customer not found")
)

var marketPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,63}$`)

// ValidChannel reports whether c is a marketing channel.
func ValidChannel(c string) bool {
	for _, v := range Channels {
		if v == c {
			return true
		}
	}
	return false
}

// ValidMarket reports whether m is a well-formed opaque market code.
func ValidMarket(m string) bool { return marketPattern.MatchString(m) }

// Consent is the current state of one channel for one customer.
type Consent struct {
	Channel       string     `json:"channel"`
	Status        string     `json:"status"`
	Market        *string    `json:"market"`
	Version       int64      `json:"version"`
	CapturedAt    *time.Time `json:"captured_at"`
	WithdrawnAt   *time.Time `json:"withdrawn_at"`
	CaptureSource *string    `json:"capture_source"`
}

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) ready() error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return nil
}

// List returns all three channels; an unrecorded channel is NOT_RECORDED.
func (s *Store) List(ctx context.Context, customerID string) ([]Consent, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(customerID); err != nil {
		return nil, ErrInvalid
	}
	rows, err := s.db.Query(ctx, `SELECT channel,status,market,version,captured_at,withdrawn_at,capture_source
		FROM customer_marketing_consents WHERE customer_id=$1`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[string]Consent{}
	for rows.Next() {
		var c Consent
		var captured, withdrawn *time.Time
		var source string
		if err := rows.Scan(&c.Channel, &c.Status, &c.Market, &c.Version, &captured, &withdrawn, &source); err != nil {
			return nil, err
		}
		c.CapturedAt, c.WithdrawnAt, c.CaptureSource = captured, withdrawn, &source
		found[c.Channel] = c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Consent, 0, len(Channels))
	for _, ch := range Channels {
		if c, ok := found[ch]; ok {
			out = append(out, c)
			continue
		}
		out = append(out, Consent{Channel: ch, Status: StatusNotRecorded})
	}
	return out, nil
}

// SetInput is one customer statement about one channel. Market is what the
// customer's app declared at opt-in; nil means unknown and is stored as NULL.
type SetInput struct {
	CustomerID string
	Channel    string
	OptedIn    bool
	Market     *string
	Now        time.Time
}

// Set records a customer's opt-in or withdrawal. A repeat of the current
// statement changes nothing (no version bump, no history row). Withdrawal is
// always accepted, including for a channel never opted into: the customer's
// explicit refusal is recorded.
func (s *Store) Set(ctx context.Context, in SetInput) (Consent, bool, error) {
	if err := s.ready(); err != nil {
		return Consent{}, false, err
	}
	if _, err := uuid.Parse(in.CustomerID); err != nil || !ValidChannel(in.Channel) {
		return Consent{}, false, ErrInvalid
	}
	if in.Market != nil {
		m := strings.TrimSpace(*in.Market)
		if !ValidMarket(m) {
			return Consent{}, false, ErrInvalid
		}
		in.Market = &m
	}
	if !in.OptedIn {
		in.Market = nil // withdrawal never rewrites the market
	}
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Consent{}, false, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customers WHERE id=$1)`, in.CustomerID).Scan(&exists); err != nil {
		return Consent{}, false, err
	}
	if !exists {
		return Consent{}, false, ErrNotFound
	}
	changed, err := setTx(ctx, tx, in, SourceCustomerApp, "")
	if err != nil {
		return Consent{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Consent{}, false, err
	}
	list, err := s.List(ctx, in.CustomerID)
	if err != nil {
		return Consent{}, false, err
	}
	for _, c := range list {
		if c.Channel == in.Channel {
			return c, changed, nil
		}
	}
	return Consent{}, false, ErrUnavailable
}

func setTx(ctx context.Context, tx pgx.Tx, in SetInput, source, decisionRef string) (bool, error) {
	var status string
	var market *string
	var version int64
	err := tx.QueryRow(ctx, `SELECT status,market,version FROM customer_marketing_consents WHERE customer_id=$1 AND channel=$2 FOR UPDATE`, in.CustomerID, in.Channel).Scan(&status, &market, &version)
	absent := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !absent {
		return false, err
	}
	next := StatusWithdrawn
	if in.OptedIn {
		next = StatusOptedIn
	}
	if !absent && status == next && (next == StatusWithdrawn || equalPtr(market, in.Market)) {
		return false, nil
	}
	var prior *string
	if !absent {
		prior = &status
	}
	if absent {
		version = 1
		var withdrawnAt *time.Time
		if next == StatusWithdrawn {
			withdrawnAt = &in.Now
		}
		if _, err := tx.Exec(ctx, `INSERT INTO customer_marketing_consents(customer_id,channel,status,market,version,captured_at,withdrawn_at,capture_source,updated_at)
			VALUES($1,$2,$3,$4,1,$5,$6,$7,$5)`, in.CustomerID, in.Channel, next, in.Market, in.Now, withdrawnAt, source); err != nil {
			return false, err
		}
	} else {
		version++
		if next == StatusOptedIn {
			_, err = tx.Exec(ctx, `UPDATE customer_marketing_consents SET status=$3,market=$4,version=$5,captured_at=$6,withdrawn_at=NULL,capture_source=$7,updated_at=$6 WHERE customer_id=$1 AND channel=$2`,
				in.CustomerID, in.Channel, next, in.Market, version, in.Now, source)
		} else {
			_, err = tx.Exec(ctx, `UPDATE customer_marketing_consents SET status=$3,version=$4,withdrawn_at=$5,capture_source=$6,updated_at=$5 WHERE customer_id=$1 AND channel=$2`,
				in.CustomerID, in.Channel, next, version, in.Now, source)
		}
		if err != nil {
			return false, err
		}
		if next == StatusWithdrawn {
			in.Market = market
		}
	}
	var ref *string
	if decisionRef != "" {
		ref = &decisionRef
	}
	_, err = tx.Exec(ctx, `INSERT INTO customer_marketing_consent_history(customer_id,channel,prior_status,status,market,recorded_version,capture_source,decision_ref,recorded_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, in.CustomerID, in.Channel, prior, next, in.Market, version, source, ref, in.Now)
	return err == nil, err
}

// WithdrawAllTx withdraws every opted-in marketing channel of a customer inside
// the caller's transaction (privacy CONSENT_WITHDRAWAL fulfilment). It returns
// the number of channels that changed.
func WithdrawAllTx(ctx context.Context, tx pgx.Tx, customerID string, now time.Time, decisionRef string) (int64, error) {
	rows, err := tx.Query(ctx, `SELECT channel FROM customer_marketing_consents WHERE customer_id=$1 AND status='OPTED_IN' ORDER BY channel FOR UPDATE`, customerID)
	if err != nil {
		return 0, err
	}
	var channels []string
	for rows.Next() {
		var ch string
		if err := rows.Scan(&ch); err != nil {
			rows.Close()
			return 0, err
		}
		channels = append(channels, ch)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var n int64
	for _, ch := range channels {
		changed, err := setTx(ctx, tx, SetInput{CustomerID: customerID, Channel: ch, OptedIn: false, Now: now}, SourcePrivacyRequest, decisionRef)
		if err != nil {
			return n, err
		}
		if changed {
			n++
		}
	}
	return n, nil
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// AudienceMember is one opted-in customer at the read's as_of instant.
type AudienceMember struct {
	CustomerID     string `json:"customer_id"`
	ConsentVersion int64  `json:"consent_version"`
}

type AudiencePage struct {
	Channel    string           `json:"channel"`
	Market     string           `json:"market"`
	AsOf       time.Time        `json:"as_of"`
	Members    []AudienceMember `json:"members"`
	NextCursor string           `json:"next_cursor"`
}

// Audience pages the active customers currently opted into channel in market,
// ordered by customer id. A NULL (unknown) market never matches.
func (s *Store) Audience(ctx context.Context, channel, market, cursor string, limit int) (AudiencePage, error) {
	if err := s.ready(); err != nil {
		return AudiencePage{}, err
	}
	if !ValidChannel(channel) || !ValidMarket(market) {
		return AudiencePage{}, ErrInvalid
	}
	if cursor == "" {
		cursor = "00000000-0000-0000-0000-000000000000"
	} else if _, err := uuid.Parse(cursor); err != nil {
		return AudiencePage{}, ErrInvalid
	}
	if limit <= 0 || limit > MaxPage {
		limit = MaxPage
	}
	page := AudiencePage{Channel: channel, Market: market, Members: []AudienceMember{}}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&page.AsOf); err != nil {
		return page, err
	}
	rows, err := tx.Query(ctx, `SELECT m.customer_id::text,m.version FROM customer_marketing_consents m
		JOIN customers c ON c.id=m.customer_id AND c.status='active'
		WHERE m.channel=$1 AND m.status='OPTED_IN' AND m.market=$2 AND m.customer_id>$3::uuid
		ORDER BY m.customer_id LIMIT $4`, channel, market, cursor, limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var m AudienceMember
		if err := rows.Scan(&m.CustomerID, &m.ConsentVersion); err != nil {
			return page, err
		}
		page.Members = append(page.Members, m)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Members) > limit {
		page.Members = page.Members[:limit]
		page.NextCursor = page.Members[limit-1].CustomerID
	}
	page.AsOf = page.AsOf.UTC()
	return page, nil
}

// Eligibility is the current answer for one customer on one channel.
type Eligibility struct {
	CustomerID     string  `json:"customer_id"`
	Eligible       bool    `json:"eligible"`
	Reason         string  `json:"reason"`
	ConsentVersion int64   `json:"consent_version"`
	Market         *string `json:"market"`
	// Email is returned only for the email channel and only when eligible:
	// notification-service cannot send an email without the address.
	Email *string `json:"email,omitempty"`
}

type CheckResult struct {
	Channel string        `json:"channel"`
	AsOf    time.Time     `json:"as_of"`
	Results []Eligibility `json:"results"`
}

// Check re-evaluates specific customers right now. Unknown ids, inactive
// accounts and missing consent are all ineligible; nothing defaults to yes.
func (s *Store) Check(ctx context.Context, channel string, ids []string) (CheckResult, error) {
	if err := s.ready(); err != nil {
		return CheckResult{}, err
	}
	if !ValidChannel(channel) || len(ids) == 0 || len(ids) > MaxPage {
		return CheckResult{}, ErrInvalid
	}
	seen := map[string]bool{}
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed.String() != id {
			return CheckResult{}, ErrInvalid
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	sort.Strings(unique)
	res := CheckResult{Channel: channel, Results: make([]Eligibility, 0, len(unique))}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&res.AsOf); err != nil {
		return res, err
	}
	rows, err := tx.Query(ctx, `SELECT x.id::text, c.id IS NOT NULL, COALESCE(c.status,''), c.email, m.status, m.version, m.market
		FROM unnest($1::uuid[]) AS x(id)
		LEFT JOIN customers c ON c.id=x.id
		LEFT JOIN customer_marketing_consents m ON m.customer_id=x.id AND m.channel=$2
		ORDER BY x.id`, unique, channel)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	for rows.Next() {
		var e Eligibility
		var exists bool
		var accountStatus string
		var email, consentStatus *string
		var version *int64
		if err := rows.Scan(&e.CustomerID, &exists, &accountStatus, &email, &consentStatus, &version, &e.Market); err != nil {
			return res, err
		}
		if version != nil {
			e.ConsentVersion = *version
		}
		switch {
		case !exists:
			e.Reason, e.Market = ReasonAccountMissing, nil
		case accountStatus != "active":
			e.Reason = ReasonInactive
		case consentStatus == nil || *consentStatus != StatusOptedIn:
			e.Reason = ReasonNotOptedIn
		case channel == "email" && (email == nil || strings.TrimSpace(*email) == ""):
			e.Reason = ReasonNoEmail
		default:
			e.Eligible, e.Reason = true, ReasonOptedIn
			if channel == "email" {
				e.Email = email
			}
		}
		res.Results = append(res.Results, e)
	}
	res.AsOf = res.AsOf.UTC()
	return res, rows.Err()
}

// Aggregate is the admin-resource dimension: OPTED_IN when any channel is
// opted in, WITHDRAWN when consent was recorded but none is opted in, and
// NULL (unknown) when nothing was ever recorded.
const AggregateDimensionQuery = `SELECT jsonb_build_object('marketing_consent',(SELECT CASE WHEN bool_or(status='OPTED_IN') THEN 'OPTED_IN' WHEN count(*)>0 THEN 'WITHDRAWN' END FROM customer_marketing_consents WHERE customer_id::text=$1))`
