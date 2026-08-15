package crm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Loyalty read models.
//
// Reads never write. A customer who has never held points has no account row
// and gets one back either — the summary reports NOT_ENROLLED, which is a
// different and more honest answer than a zero balance the service invented
// by creating a row when someone opened a screen.

// LoyaltySummaryFor composes the customer loyalty read model.
//
// The caller must already have passed the profile through GetProfile (or the
// loyalty fence), so tenancy and branch reach are settled before this runs.
func (s *Service) LoyaltySummaryFor(ctx context.Context, actor Actor, profile Profile) (LoyaltySummary, error) {
	program, err := s.GetLoyaltyProgram(ctx, actor)
	if err != nil {
		return LoyaltySummary{}, err
	}
	summary := LoyaltySummary{
		ProgramStatus: program.Status, Enrollment: LoyaltyNotEnrolled,
		PointsNameSingular: program.PointsNameSingular, PointsNamePlural: program.PointsNamePlural,
	}
	// A redacted or merged-away relationship has no independent loyalty state
	// to report, whatever the ledger still retains for audit purposes.
	if profile.IsRedacted() || profile.IsMergedSource() {
		return summary, nil
	}

	account, err := scanLoyaltyAccount(s.db.QueryRow(ctx, `SELECT `+loyaltyAccountCols+` FROM crm_loyalty_accounts
		WHERE business_id=$1 AND customer_profile_id=$2`, actor.VendorID, profile.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return summary, nil
	}
	if err != nil {
		return LoyaltySummary{}, err
	}
	if account.Status != LoyaltyAccountActive {
		summary.AccountStatus = account.Status
		return summary, nil
	}

	now := s.now()
	pending, err := s.pendingExpiryPoints(ctx, s.db, actor.VendorID, account.ID, now)
	if err != nil {
		return LoyaltySummary{}, err
	}
	summary.Enrollment = LoyaltyEnrolled
	summary.AccountStatus = account.Status
	// The settled balance minus anything already past its expiry: a sweeper
	// that is behind must never make dead points look spendable.
	summary.AvailablePoints = account.CurrentPoints - pending
	if summary.AvailablePoints < 0 {
		summary.AvailablePoints = 0
	}
	summary.PendingExpiryPoints = pending
	summary.LifetimeCreditedPoints = account.LifetimeCreditedPoints
	summary.LifetimeDebitedPoints = account.LifetimeDebitedPoints
	summary.Version = account.Version

	var expiresAt *time.Time
	var expiring *int64
	if err = s.db.QueryRow(ctx, `SELECT expires_at,sum(remaining_points) FROM crm_loyalty_point_lots
		WHERE business_id=$1 AND loyalty_account_id=$2 AND remaining_points>0
		  AND expires_at IS NOT NULL AND expires_at>$3
		GROUP BY expires_at ORDER BY expires_at LIMIT 1`,
		actor.VendorID, account.ID, now).Scan(&expiresAt, &expiring); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return LoyaltySummary{}, err
	}
	if expiresAt != nil && expiring != nil {
		summary.NextExpiryAt = expiresAt
		summary.PointsExpiringNext = *expiring
	}
	return summary, nil
}

// ---------------------------------------------------------------------------
// Ledger history
// ---------------------------------------------------------------------------

// LoyaltyLedgerOptions are the safe filters over points history. There is no
// free-text search and no caller-chosen sort column: ordering comes from a
// closed registry so no request can shape the SQL.
type LoyaltyLedgerOptions struct {
	EntryType     string
	CreatedFrom   *time.Time
	CreatedTo     *time.Time
	Limit, Offset int
}

var loyaltyEntryTypes = map[string]bool{
	LoyaltyEarn: true, LoyaltyRedeem: true, LoyaltyExpire: true,
	LoyaltyAdjustmentCredit: true, LoyaltyAdjustmentDebit: true,
	LoyaltyReversal: true, LoyaltyMergeIn: true, LoyaltyMergeOut: true,
}

func (o LoyaltyLedgerOptions) bounded() LoyaltyLedgerOptions {
	if o.Limit <= 0 {
		o.Limit = LoyaltyLedgerDefaultPageSize
	}
	if o.Limit > LoyaltyLedgerMaxPageSize {
		o.Limit = LoyaltyLedgerMaxPageSize
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	return o
}

// ListLoyaltyEntries returns one customer's points history, newest first.
//
// The account is resolved from the already-scoped profile, so tenancy and
// branch reach are decided before any row is read and the page cannot contain
// another business's or another branch's customer.
func (s *Service) ListLoyaltyEntries(ctx context.Context, actor Actor, profile Profile, opts LoyaltyLedgerOptions) ([]LoyaltyEntry, bool, error) {
	opts = opts.bounded()
	if opts.EntryType != "" {
		opts.EntryType = strings.ToUpper(strings.TrimSpace(opts.EntryType))
		if !loyaltyEntryTypes[opts.EntryType] {
			return nil, false, loyaltyValidation("entry_type", "Unsupported loyalty entry type.", CodeLoyaltyInvalidPoints)
		}
	}
	if profile.IsRedacted() || profile.IsMergedSource() {
		return nil, false, errLoyaltyAccountNotFound()
	}

	var accountID string
	err := s.db.QueryRow(ctx, `SELECT id FROM crm_loyalty_accounts
		WHERE business_id=$1 AND customer_profile_id=$2`, actor.VendorID, profile.ID).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, errLoyaltyAccountNotFound()
	}
	if err != nil {
		return nil, false, err
	}

	args := []any{actor.VendorID, accountID}
	where := []string{"business_id=$1", "loyalty_account_id=$2"}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, strings.Replace(clause, "?", "$"+itoa(len(args)), 1))
	}
	if opts.EntryType != "" {
		add("entry_type=?", opts.EntryType)
	}
	if opts.CreatedFrom != nil {
		add("created_at>=?", opts.CreatedFrom.UTC())
	}
	if opts.CreatedTo != nil {
		add("created_at<=?", opts.CreatedTo.UTC())
	}
	args = append(args, opts.Limit+1, opts.Offset)
	rows, err := s.db.Query(ctx, `SELECT `+loyaltyEntryCols+` FROM crm_loyalty_entries
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC, sequence DESC
		LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	entries := make([]LoyaltyEntry, 0, opts.Limit)
	for rows.Next() {
		entry, scanErr := scanLoyaltyEntry(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		entries = append(entries, entry)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(entries) > opts.Limit
	if more {
		entries = entries[:opts.Limit]
	}
	return entries, more, nil
}

// LoyaltyEntryViewsWithReversals renders a page of entries and marks which of
// them have since been reversed, so a history screen does not have to guess
// from the surrounding rows.
func (s *Service) LoyaltyEntryViewsWithReversals(ctx context.Context, businessID string, entries []LoyaltyEntry) ([]LoyaltyEntryView, error) {
	views := make([]LoyaltyEntryView, 0, len(entries))
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		views = append(views, LoyaltyEntryViewOf(entry))
		ids = append(ids, entry.ID)
	}
	if len(ids) == 0 {
		return views, nil
	}
	rows, err := s.db.Query(ctx, `SELECT reverses_entry_id,id FROM crm_loyalty_entries
		WHERE business_id=$1 AND reverses_entry_id = ANY($2)`, businessID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reversedBy := map[string]string{}
	for rows.Next() {
		var original, reversal string
		if err = rows.Scan(&original, &reversal); err != nil {
			return nil, err
		}
		reversedBy[original] = reversal
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range views {
		if ref, ok := reversedBy[views[i].EntryRef]; ok {
			value := ref
			views[i].ReversedByRef = &value
		}
	}
	return views, nil
}

// ---------------------------------------------------------------------------
// Customer 360
// ---------------------------------------------------------------------------

// attachLoyaltyBlock contributes the Section 8D block through the same
// composition point Sections 8B and 8C use, rather than by rewriting
// Customer360For.
func (s *Service) attachLoyaltyBlock(ctx context.Context, actor Actor, profile Profile, view *Customer360) error {
	if !s.flags.LoyaltyEnabled {
		return nil
	}
	summary, err := s.LoyaltySummaryFor(ctx, actor, profile)
	if err != nil {
		return err
	}
	view.Loyalty = &summary

	// Tier standing rides with the loyalty block rather than becoming its own
	// module. It is a reading of the same ledger through the vendor's ladder,
	// and a caller that has the balance almost always wants the standing too;
	// splitting them would mean two round trips to answer one question.
	tier, err := s.ResolveCustomerLoyaltyTier(ctx, actor.VendorID, profile.ID)
	if err != nil {
		return err
	}
	view.LoyaltyTier = &tier

	view.Modules["loyalty"] = ModuleAvailable
	return nil
}
