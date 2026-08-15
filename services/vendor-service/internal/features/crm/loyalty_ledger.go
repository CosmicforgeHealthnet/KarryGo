package crm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// The loyalty ledger engine.
//
// Every balance change in Section 8D goes through mutateLoyalty, and every
// mutation obeys the same shape: lock the profile, lock the account, settle
// anything that has expired, then post exactly one immutable entry that
// carries its own resulting balance. Nothing recomputes a balance from
// scratch and nothing edits a posted row, so the ledger stays replayable and
// the account aggregate can never quietly drift away from it.

const loyaltyAccountCols = `id,business_id,customer_profile_id,status,current_points,
lifetime_credited_points,lifetime_debited_points,next_sequence,created_at,updated_at,version`

func scanLoyaltyAccount(row rowScanner) (LoyaltyAccount, error) {
	var a LoyaltyAccount
	err := row.Scan(&a.ID, &a.BusinessID, &a.CustomerProfileID, &a.Status, &a.CurrentPoints,
		&a.LifetimeCreditedPoints, &a.LifetimeDebitedPoints, &a.NextSequence, &a.CreatedAt, &a.UpdatedAt, &a.Version)
	return a, err
}

const loyaltyEntryCols = `id,business_id,loyalty_account_id,sequence,entry_type,delta_points,balance_after,
program_version,source_type,source_reference,idempotency_reference,reverses_entry_id,reason_code,
created_by_staff_id,created_at`

func scanLoyaltyEntry(row rowScanner) (LoyaltyEntry, error) {
	var e LoyaltyEntry
	err := row.Scan(&e.ID, &e.BusinessID, &e.LoyaltyAccountID, &e.Sequence, &e.EntryType, &e.DeltaPoints,
		&e.BalanceAfter, &e.ProgramVersion, &e.SourceType, &e.SourceReference, &e.IdempotencyReference,
		&e.ReversesEntryID, &e.ReasonCode, &e.CreatedByStaffID, &e.CreatedAt)
	return e, err
}

const loyaltyLotCols = `id,business_id,loyalty_account_id,source_entry_id,issued_points,remaining_points,expires_at,created_at`

func scanLoyaltyLot(row rowScanner) (LoyaltyPointLot, error) {
	var l LoyaltyPointLot
	err := row.Scan(&l.ID, &l.BusinessID, &l.LoyaltyAccountID, &l.SourceEntryID, &l.IssuedPoints,
		&l.RemainingPoints, &l.ExpiresAt, &l.CreatedAt)
	return l, err
}

// ---------------------------------------------------------------------------
// Profile fence and scope
// ---------------------------------------------------------------------------

// lockLoyaltyProfile is the privacy, merge and scope fence for every loyalty
// mutation.
//
// A points balance belongs to the business, not to a branch, so there is no
// branch column on a loyalty account. Reach is instead decided the same way
// Customer 360 decides it: a branch-scoped actor may act only on a customer
// who actually has a relationship at one of their branches, and a customer
// outside that reach is reported as not found rather than as forbidden — so
// loyalty lookups cannot be used to discover that another branch's customers
// exist.
func (s *Service) lockLoyaltyProfile(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, profileID string) (Profile, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return Profile{}, loyaltyValidation("customer_profile_ref", "A customer is required.", CodeLoyaltyAccountNotFound)
	}
	profile, err := scanProfile(tx.QueryRow(ctx, `/* crm_loyalty_profile_fence */ SELECT `+profileCols+`
		FROM vendor_customer_profiles WHERE business_id=$1 AND id=$2 FOR UPDATE`, actor.VendorID, profileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, errProfileNotFound()
	}
	if err != nil {
		return Profile{}, err
	}
	if profile.IsRedacted() {
		return Profile{}, crmError(apperrors.Conflict("This customer has been redacted.", nil), CodeLoyaltyCustomerRedacted)
	}
	if profile.IsMergedSource() {
		return Profile{}, crmError(apperrors.Conflict("This customer has been merged.", nil), CodeLoyaltyCustomerMerged)
	}
	if !scope.Global {
		var visible bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_customer_branch_relations
			WHERE business_id=$1 AND profile_id=$2 AND branch_id = ANY($3))`,
			actor.VendorID, profileID, scope.BranchIDs).Scan(&visible); err != nil {
			return Profile{}, err
		}
		if !visible {
			return Profile{}, errProfileNotFound()
		}
	}
	return profile, nil
}

// ---------------------------------------------------------------------------
// Account resolution
// ---------------------------------------------------------------------------

// lockLoyaltyAccount loads and row-locks the account, optionally creating it.
//
// Creation is lazy: a customer gets an account the first time points actually
// move, never because someone looked at their profile. Two concurrent first
// credits both try to insert; the unique constraint decides, and the loser
// re-reads the winner's row rather than failing the command.
func (s *Service) lockLoyaltyAccount(ctx context.Context, tx pgx.Tx, businessID, profileID string, create bool) (LoyaltyAccount, error) {
	account, err := scanLoyaltyAccount(tx.QueryRow(ctx, `SELECT `+loyaltyAccountCols+` FROM crm_loyalty_accounts
		WHERE business_id=$1 AND customer_profile_id=$2 FOR UPDATE`, businessID, profileID))
	if err == nil {
		return account, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return LoyaltyAccount{}, err
	}
	if !create {
		return LoyaltyAccount{}, errLoyaltyAccountNotFound()
	}
	account, err = scanLoyaltyAccount(tx.QueryRow(ctx, `INSERT INTO crm_loyalty_accounts
		(id,business_id,customer_profile_id) VALUES($1,$2,$3)
		ON CONFLICT (business_id,customer_profile_id) DO UPDATE SET updated_at=crm_loyalty_accounts.updated_at
		RETURNING `+loyaltyAccountCols, newID("crmla_"), businessID, profileID))
	if err != nil {
		return LoyaltyAccount{}, err
	}
	// The DO UPDATE branch returns the existing row but does not take the row
	// lock a plain SELECT ... FOR UPDATE would; re-read it under a lock so
	// every caller of this function holds the same guarantee.
	return scanLoyaltyAccount(tx.QueryRow(ctx, `SELECT `+loyaltyAccountCols+` FROM crm_loyalty_accounts
		WHERE id=$1 FOR UPDATE`, account.ID))
}

// ---------------------------------------------------------------------------
// Expiry settlement
// ---------------------------------------------------------------------------

// settleExpiredLots posts an EXPIRE entry for everything already past its
// expiry on this account and returns the number of points removed.
//
// It runs at the start of every balance-sensitive mutation, which is what
// makes the design safe when the sweeper is down: a debit can never spend a
// lot whose expiry has passed, because the debit itself expires it first.
// Calling it twice is harmless — the second call finds nothing due.
func (s *Service) settleExpiredLots(ctx context.Context, tx pgx.Tx, actor Actor, account *LoyaltyAccount, program LoyaltyProgram, now time.Time) (int64, error) {
	rows, err := tx.Query(ctx, `SELECT `+loyaltyLotCols+` FROM crm_loyalty_point_lots
		WHERE business_id=$1 AND loyalty_account_id=$2 AND remaining_points>0
		  AND expires_at IS NOT NULL AND expires_at<=$3
		ORDER BY expires_at, created_at, id FOR UPDATE`, account.BusinessID, account.ID, now)
	if err != nil {
		return 0, err
	}
	var due []LoyaltyPointLot
	for rows.Next() {
		lot, scanErr := scanLoyaltyLot(rows)
		if scanErr != nil {
			rows.Close()
			return 0, scanErr
		}
		due = append(due, lot)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	if len(due) == 0 {
		return 0, nil
	}

	var total int64
	for _, lot := range due {
		total += lot.RemainingPoints
	}
	// A lot cannot expire points that are no longer on the balance, so this
	// should be impossible; refusing loudly beats posting an entry that would
	// drive the balance negative.
	if total > account.CurrentPoints {
		return 0, errors.New("loyalty: expiring lots exceed account balance")
	}

	entry, err := s.postLoyaltyEntry(ctx, tx, actor, account, loyaltyEntryInput{
		EntryType: LoyaltyExpire, Delta: -total, Program: program,
		SourceType: LoyaltySourceExpiry, ReasonCode: strPtr("POINTS_EXPIRED"),
	})
	if err != nil {
		return 0, err
	}
	for _, lot := range due {
		if _, err = tx.Exec(ctx, `UPDATE crm_loyalty_point_lots SET remaining_points=0
			WHERE id=$1 AND business_id=$2`, lot.ID, account.BusinessID); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO crm_loyalty_lot_consumptions
			(id,business_id,loyalty_account_id,debit_entry_id,lot_id,consumed_points)
			VALUES($1,$2,$3,$4,$5,$6)`, newID("crmlc_"), account.BusinessID, account.ID, entry.ID, lot.ID, lot.RemainingPoints); err != nil {
			return 0, err
		}
	}
	if err = s.auditTx(ctx, tx, actor, "crm.loyalty.points.expired", "crm_loyalty_account", account.ID, "",
		nil, map[string]any{"entry_ref": entry.ID, "delta_points": entry.DeltaPoints, "balance_after": entry.BalanceAfter}, ""); err != nil {
		return 0, err
	}
	if err = s.publishLoyaltyEntry(ctx, tx, actor, "vendor.crm.loyalty.points.expired", *account, entry); err != nil {
		return 0, err
	}
	s.metrics.Add("crm_loyalty_expiration_total", float64(total))
	return total, nil
}

// availablePoints is the spendable balance: the settled balance minus
// anything that has expired but not yet been swept. Reads use it so a stalled
// sweeper never makes dead points look spendable.
func (s *Service) pendingExpiryPoints(ctx context.Context, q rowQuerier, businessID, accountID string, now time.Time) (int64, error) {
	var pending *int64
	err := q.QueryRow(ctx, `SELECT sum(remaining_points) FROM crm_loyalty_point_lots
		WHERE business_id=$1 AND loyalty_account_id=$2 AND remaining_points>0
		  AND expires_at IS NOT NULL AND expires_at<=$3`, businessID, accountID, now).Scan(&pending)
	if err != nil {
		return 0, err
	}
	if pending == nil {
		return 0, nil
	}
	return *pending, nil
}

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ---------------------------------------------------------------------------
// Entry posting
// ---------------------------------------------------------------------------

type loyaltyEntryInput struct {
	EntryType            string
	Delta                int64
	Program              LoyaltyProgram
	SourceType           string
	SourceReference      *string
	IdempotencyReference *string
	ReversesEntryID      *string
	ReasonCode           *string
	StaffID              *string
}

// postLoyaltyEntry appends one immutable entry and advances the account
// aggregate in the same statement pair.
//
// balance_after is computed from the locked account row, and the account
// update is guarded on the version we read, so the entry's stated balance is
// the balance that was actually committed. The caller's account struct is
// updated in place so a multi-entry transaction (a reversal that also expires
// restored points, say) keeps posting from the correct running balance.
func (s *Service) postLoyaltyEntry(ctx context.Context, tx pgx.Tx, actor Actor, account *LoyaltyAccount, in loyaltyEntryInput) (LoyaltyEntry, error) {
	if in.Delta == 0 {
		return LoyaltyEntry{}, loyaltyValidation("points", "A loyalty movement must change the balance.", CodeLoyaltyInvalidPoints)
	}
	if in.Delta > MaxLoyaltyPoints || in.Delta < -MaxLoyaltyPoints {
		return LoyaltyEntry{}, loyaltyValidation("points", "That points amount is out of range.", CodeLoyaltyInvalidPoints)
	}
	balance := account.CurrentPoints + in.Delta
	if balance < 0 {
		return LoyaltyEntry{}, errLoyaltyInsufficient()
	}
	if balance > MaxLoyaltyPoints {
		return LoyaltyEntry{}, loyaltyValidation("points", "That would take the balance beyond the supported maximum.", CodeLoyaltyInvalidPoints)
	}
	credited, debited := account.LifetimeCreditedPoints, account.LifetimeDebitedPoints
	if in.Delta > 0 {
		credited += in.Delta
	} else {
		debited += -in.Delta
	}

	entry, err := scanLoyaltyEntry(tx.QueryRow(ctx, `INSERT INTO crm_loyalty_entries
		(id,business_id,loyalty_account_id,sequence,entry_type,delta_points,balance_after,program_version,
		 source_type,source_reference,idempotency_reference,reverses_entry_id,reason_code,created_by_staff_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING `+loyaltyEntryCols,
		newID("crmle_"), account.BusinessID, account.ID, account.NextSequence, in.EntryType, in.Delta, balance,
		in.Program.Version, in.SourceType, in.SourceReference, in.IdempotencyReference, in.ReversesEntryID,
		in.ReasonCode, in.StaffID))
	if err != nil {
		return LoyaltyEntry{}, err
	}

	updated, err := scanLoyaltyAccount(tx.QueryRow(ctx, `UPDATE crm_loyalty_accounts SET
		current_points=$1,lifetime_credited_points=$2,lifetime_debited_points=$3,
		next_sequence=next_sequence+1,updated_at=now(),version=version+1
		WHERE id=$4 AND business_id=$5 AND version=$6 RETURNING `+loyaltyAccountCols,
		balance, credited, debited, account.ID, account.BusinessID, account.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		// Unreachable while the row lock is held; treated as a conflict rather
		// than a silent divergence between ledger and aggregate.
		return LoyaltyEntry{}, errLoyaltyVersionConflict()
	}
	if err != nil {
		return LoyaltyEntry{}, err
	}
	*account = updated
	return entry, nil
}

// ---------------------------------------------------------------------------
// Lot issuance and FEFO consumption
// ---------------------------------------------------------------------------

// issueLot creates the point lot behind a credit, stamping its expiry from the
// programme as it stands right now. That stamp is never revisited: a business
// that later shortens or lengthens its expiry window changes what future
// credits get, not what customers already hold.
func (s *Service) issueLot(ctx context.Context, tx pgx.Tx, account LoyaltyAccount, program LoyaltyProgram, entry LoyaltyEntry, points int64, now time.Time) error {
	var expiresAt *time.Time
	if program.ExpiryPolicy == LoyaltyExpiryFixedDays && program.ExpiryDays != nil {
		value := now.AddDate(0, 0, *program.ExpiryDays)
		expiresAt = &value
	}
	_, err := tx.Exec(ctx, `INSERT INTO crm_loyalty_point_lots
		(id,business_id,loyalty_account_id,source_entry_id,issued_points,remaining_points,expires_at)
		VALUES($1,$2,$3,$4,$5,$5,$6)`,
		newID("crmll_"), account.BusinessID, account.ID, entry.ID, points, expiresAt)
	return err
}

// consumeLotsFEFO spends `points` from unexpired lots, earliest expiry first,
// and records exactly which lot funded how much.
//
// Non-expiring lots are consumed last, so a customer's soonest-to-die points
// are always the ones spent. The allocation rows are not bookkeeping decoration:
// they are what makes a later reversal restore the same points with the same
// expiry rather than mint fresh ones.
func (s *Service) consumeLotsFEFO(ctx context.Context, tx pgx.Tx, account LoyaltyAccount, debitEntryID string, points int64, now time.Time) error {
	rows, err := tx.Query(ctx, `SELECT `+loyaltyLotCols+` FROM crm_loyalty_point_lots
		WHERE business_id=$1 AND loyalty_account_id=$2 AND remaining_points>0
		  AND (expires_at IS NULL OR expires_at>$3)
		ORDER BY expires_at ASC NULLS LAST, created_at ASC, id ASC FOR UPDATE`,
		account.BusinessID, account.ID, now)
	if err != nil {
		return err
	}
	var lots []LoyaltyPointLot
	for rows.Next() {
		lot, scanErr := scanLoyaltyLot(rows)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		lots = append(lots, lot)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}

	remaining := points
	for _, lot := range lots {
		if remaining == 0 {
			break
		}
		take := lot.RemainingPoints
		if take > remaining {
			take = remaining
		}
		if _, err = tx.Exec(ctx, `UPDATE crm_loyalty_point_lots SET remaining_points=remaining_points-$1
			WHERE id=$2 AND business_id=$3`, take, lot.ID, account.BusinessID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO crm_loyalty_lot_consumptions
			(id,business_id,loyalty_account_id,debit_entry_id,lot_id,consumed_points)
			VALUES($1,$2,$3,$4,$5,$6)`, newID("crmlc_"), account.BusinessID, account.ID, debitEntryID, lot.ID, take); err != nil {
			return err
		}
		remaining -= take
	}
	if remaining > 0 {
		// The balance said the points were there but the lots disagree. That
		// is a broken invariant, not a user error, so it aborts rather than
		// silently under-consuming.
		return errors.New("loyalty: lot coverage is short of the account balance")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Shared mutation frame
// ---------------------------------------------------------------------------

// loyaltyCommand is one balance mutation request in its resolved form.
type loyaltyCommand struct {
	ProfileID string
	// CreateAccount allows lazy first-credit creation. Debits never create.
	CreateAccount bool
	// RequireStatuses limits which programme statuses permit the command.
	// Empty means every status is acceptable (reversals are corrections and
	// stay possible even on a switched-off programme).
	RequireStatuses []string
	Apply           func(ctx context.Context, tx pgx.Tx, account *LoyaltyAccount, program LoyaltyProgram) (LoyaltyEntry, error)
}

// mutateLoyalty is the single transactional frame every balance change goes
// through: profile fence, programme load, account lock, expiry settlement,
// then the command's own work. Nothing bypasses it.
func (s *Service) mutateLoyalty(ctx context.Context, actor Actor, scope Scope, cmd loyaltyCommand) (LoyaltyEntry, LoyaltyAccount, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profile, err := s.lockLoyaltyProfile(ctx, tx, actor, scope, cmd.ProfileID)
	if err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	program, err := s.loadProgramForUpdate(ctx, tx, actor.VendorID)
	if err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	if len(cmd.RequireStatuses) > 0 && !containsString(cmd.RequireStatuses, program.Status) {
		return LoyaltyEntry{}, LoyaltyAccount{}, errLoyaltyProgramState(program.Status)
	}

	account, err := s.lockLoyaltyAccount(ctx, tx, actor.VendorID, profile.ID, cmd.CreateAccount)
	if err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	created := account.Version == 1 && account.NextSequence == 1
	if account.Status != LoyaltyAccountActive {
		return LoyaltyEntry{}, LoyaltyAccount{}, errLoyaltyAccountNotActionable()
	}
	if created {
		if err = s.auditTx(ctx, tx, actor, "crm.loyalty.account.created", "crm_loyalty_account", account.ID, "",
			nil, map[string]any{"loyalty_account_ref": account.ID, "customer_profile_ref": account.CustomerProfileID}, ""); err != nil {
			return LoyaltyEntry{}, LoyaltyAccount{}, err
		}
		if err = s.publishTx(ctx, tx, actor, "vendor.crm.loyalty.account.created", "crm_loyalty_account", account.ID, account.Version,
			map[string]any{"business_ref": account.BusinessID, "customer_profile_ref": account.CustomerProfileID,
				"loyalty_account_ref": account.ID, "occurred_at": s.now()}); err != nil {
			return LoyaltyEntry{}, LoyaltyAccount{}, err
		}
	}

	// Settle before the command so no debit can reach an expired lot and no
	// balance check is made against points that are already dead.
	if _, err = s.settleExpiredLots(ctx, tx, actor, &account, program, s.now()); err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}

	entry, err := cmd.Apply(ctx, tx, &account, program)
	if err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	return entry, account, nil
}

// ---------------------------------------------------------------------------
// Credit and debit primitives
// ---------------------------------------------------------------------------

// CreditPointsInput is a trusted points credit from an authoritative producer.
//
// There is deliberately no earning formula anywhere in vendor-service: the
// producer calculates how many points an order is worth and this primitive
// records it. Nothing here converts money to points, because the Vendor Order
// domain that would define that conversion does not exist yet.
type CreditPointsInput struct {
	CustomerProfileRef string
	Points             int64
	// SourceType must be ORDER or EXTERNAL. SourceReference is the producer's
	// own reference and is deduplicated durably, so a redelivered order event
	// credits once.
	SourceType      string
	SourceReference string
	// CommandRef is the caller's exactly-once key. A retry with the same key
	// replays the original entry instead of crediting twice.
	CommandRef string
	ReasonCode string
}

// CreditPoints applies a trusted credit. It is a domain primitive with no HTTP
// route: nothing outside this service can call it, which is exactly the point
// — an endpoint that let a client name its own points total would be a
// self-service loyalty printer.
func (s *Service) CreditPoints(ctx context.Context, actor Actor, scope Scope, in CreditPointsInput) (LoyaltyEntry, error) {
	if !s.flags.LoyaltyEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return LoyaltyEntry{}, errWritesDisabled()
	}
	if in.SourceType != LoyaltySourceOrder && in.SourceType != LoyaltySourceExternal {
		return LoyaltyEntry{}, loyaltyValidation("source_type", "A trusted credit must name ORDER or EXTERNAL.", CodeLoyaltyInvalidPoints)
	}
	if err := validLoyaltyPoints(in.Points, MaxLoyaltyPoints); err != nil {
		return LoyaltyEntry{}, err
	}
	entry, account, err := s.mutateLoyalty(ctx, actor, scope, loyaltyCommand{
		ProfileID: strings.TrimSpace(in.CustomerProfileRef), CreateAccount: true,
		RequireStatuses: []string{LoyaltyProgramActive},
		Apply: func(ctx context.Context, tx pgx.Tx, account *LoyaltyAccount, program LoyaltyProgram) (LoyaltyEntry, error) {
			return s.applyCredit(ctx, tx, actor, account, program, creditRequest{
				EntryType: LoyaltyEarn, Points: in.Points, SourceType: in.SourceType,
				SourceReference: strPtrOrNil(in.SourceReference), CommandRef: strPtrOrNil(in.CommandRef),
				ReasonCode: strPtrOrNil(in.ReasonCode),
			})
		},
	})
	if err != nil {
		s.metrics.Inc("crm_loyalty_mutation_failure_total", "operation", "credit")
		return LoyaltyEntry{}, err
	}
	s.metrics.Inc("crm_loyalty_credit_total", "entry_type", entry.EntryType)
	_ = account
	return entry, nil
}

type creditRequest struct {
	EntryType       string
	Points          int64
	SourceType      string
	SourceReference *string
	CommandRef      *string
	ReasonCode      *string
	StaffID         *string
}

// applyCredit posts the credit entry and its lot, replaying instead of
// duplicating when the command or source event has already been applied.
func (s *Service) applyCredit(ctx context.Context, tx pgx.Tx, actor Actor, account *LoyaltyAccount, program LoyaltyProgram, req creditRequest) (LoyaltyEntry, error) {
	if existing, found, err := s.findAppliedEntry(ctx, tx, account.BusinessID, req.CommandRef, req.SourceType, req.SourceReference); err != nil || found {
		return existing, err
	}
	entry, err := s.postLoyaltyEntry(ctx, tx, actor, account, loyaltyEntryInput{
		EntryType: req.EntryType, Delta: req.Points, Program: program, SourceType: req.SourceType,
		SourceReference: req.SourceReference, IdempotencyReference: req.CommandRef,
		ReasonCode: req.ReasonCode, StaffID: req.StaffID,
	})
	if err != nil {
		return LoyaltyEntry{}, err
	}
	if err = s.issueLot(ctx, tx, *account, program, entry, req.Points, s.now()); err != nil {
		return LoyaltyEntry{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "crm.loyalty.points.credited", "crm_loyalty_account", account.ID, "",
		nil, safeLoyaltyEntryAudit(entry), ""); err != nil {
		return LoyaltyEntry{}, err
	}
	if err = s.publishLoyaltyEntry(ctx, tx, actor, "vendor.crm.loyalty.points.credited", *account, entry); err != nil {
		return LoyaltyEntry{}, err
	}
	return entry, nil
}

// DebitPointsInput is a trusted redemption from a future checkout integration.
type DebitPointsInput struct {
	CustomerProfileRef string
	Points             int64
	SourceType         string
	SourceReference    string
	CommandRef         string
	ReasonCode         string
}

// DebitPoints spends points under the programme's redemption policy. Like
// CreditPoints it is deliberately service-internal: no Section 8D HTTP route
// redeems points, because there is no checkout to redeem them against yet.
func (s *Service) DebitPoints(ctx context.Context, actor Actor, scope Scope, in DebitPointsInput) (LoyaltyEntry, error) {
	if !s.flags.LoyaltyEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return LoyaltyEntry{}, errWritesDisabled()
	}
	if in.SourceType != LoyaltySourceOrder && in.SourceType != LoyaltySourceExternal {
		return LoyaltyEntry{}, loyaltyValidation("source_type", "A trusted redemption must name ORDER or EXTERNAL.", CodeLoyaltyInvalidPoints)
	}
	if err := validLoyaltyPoints(in.Points, MaxLoyaltyPoints); err != nil {
		return LoyaltyEntry{}, err
	}
	entry, _, err := s.mutateLoyalty(ctx, actor, scope, loyaltyCommand{
		ProfileID: strings.TrimSpace(in.CustomerProfileRef), CreateAccount: false,
		RequireStatuses: []string{LoyaltyProgramActive},
		Apply: func(ctx context.Context, tx pgx.Tx, account *LoyaltyAccount, program LoyaltyProgram) (LoyaltyEntry, error) {
			if program.MinimumRedemptionPoints > 0 && in.Points < program.MinimumRedemptionPoints {
				return LoyaltyEntry{}, loyaltyValidation("points", "That is below the minimum redemption for this programme.", CodeLoyaltyInvalidPoints)
			}
			if program.RedemptionIncrementPoints > 1 && in.Points%program.RedemptionIncrementPoints != 0 {
				return LoyaltyEntry{}, loyaltyValidation("points", "Redemption must be in the programme's point increment.", CodeLoyaltyInvalidPoints)
			}
			return s.applyDebit(ctx, tx, actor, account, program, debitRequest{
				EntryType: LoyaltyRedeem, Points: in.Points, SourceType: in.SourceType,
				SourceReference: strPtrOrNil(in.SourceReference), CommandRef: strPtrOrNil(in.CommandRef),
				ReasonCode: strPtrOrNil(in.ReasonCode),
			})
		},
	})
	if err != nil {
		s.metrics.Inc("crm_loyalty_mutation_failure_total", "operation", "debit")
		return LoyaltyEntry{}, err
	}
	s.metrics.Inc("crm_loyalty_debit_total", "entry_type", entry.EntryType)
	return entry, nil
}

type debitRequest struct {
	EntryType       string
	Points          int64
	SourceType      string
	SourceReference *string
	CommandRef      *string
	ReasonCode      *string
	StaffID         *string
}

func (s *Service) applyDebit(ctx context.Context, tx pgx.Tx, actor Actor, account *LoyaltyAccount, program LoyaltyProgram, req debitRequest) (LoyaltyEntry, error) {
	if existing, found, err := s.findAppliedEntry(ctx, tx, account.BusinessID, req.CommandRef, req.SourceType, req.SourceReference); err != nil || found {
		return existing, err
	}
	// Expiry has already been settled by mutateLoyalty, so the account balance
	// at this point is exactly the spendable balance.
	if account.CurrentPoints < req.Points {
		return LoyaltyEntry{}, errLoyaltyInsufficient()
	}
	entry, err := s.postLoyaltyEntry(ctx, tx, actor, account, loyaltyEntryInput{
		EntryType: req.EntryType, Delta: -req.Points, Program: program, SourceType: req.SourceType,
		SourceReference: req.SourceReference, IdempotencyReference: req.CommandRef,
		ReasonCode: req.ReasonCode, StaffID: req.StaffID,
	})
	if err != nil {
		return LoyaltyEntry{}, err
	}
	if err = s.consumeLotsFEFO(ctx, tx, *account, entry.ID, req.Points, s.now()); err != nil {
		return LoyaltyEntry{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "crm.loyalty.points.debited", "crm_loyalty_account", account.ID, "",
		nil, safeLoyaltyEntryAudit(entry), ""); err != nil {
		return LoyaltyEntry{}, err
	}
	if err = s.publishLoyaltyEntry(ctx, tx, actor, "vendor.crm.loyalty.points.debited", *account, entry); err != nil {
		return LoyaltyEntry{}, err
	}
	return entry, nil
}

// findAppliedEntry resolves a durable replay: the same command key, or the
// same trusted source event, returns the entry that was already posted rather
// than posting a second one.
func (s *Service) findAppliedEntry(ctx context.Context, tx pgx.Tx, businessID string, commandRef *string, sourceType string, sourceRef *string) (LoyaltyEntry, bool, error) {
	if commandRef != nil {
		entry, err := scanLoyaltyEntry(tx.QueryRow(ctx, `SELECT `+loyaltyEntryCols+` FROM crm_loyalty_entries
			WHERE business_id=$1 AND idempotency_reference=$2`, businessID, *commandRef))
		if err == nil {
			return entry, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return LoyaltyEntry{}, false, err
		}
	}
	if sourceRef != nil && (sourceType == LoyaltySourceOrder || sourceType == LoyaltySourceExternal) {
		entry, err := scanLoyaltyEntry(tx.QueryRow(ctx, `SELECT `+loyaltyEntryCols+` FROM crm_loyalty_entries
			WHERE business_id=$1 AND source_type=$2 AND source_reference=$3`, businessID, sourceType, *sourceRef))
		if err == nil {
			return entry, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return LoyaltyEntry{}, false, err
		}
	}
	return LoyaltyEntry{}, false, nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func validLoyaltyPoints(points, limit int64) error {
	if points <= 0 {
		return loyaltyValidation("points", "Points must be a positive whole number.", CodeLoyaltyInvalidPoints)
	}
	if points > limit {
		return loyaltyValidation("points", "That points amount is too large.", CodeLoyaltyInvalidPoints)
	}
	return nil
}

func safeLoyaltyEntryAudit(e LoyaltyEntry) map[string]any {
	// No reason text, no command key, no customer identity: an audit row
	// records what moved, not why in the operator's own words.
	return map[string]any{
		"entry_ref": e.ID, "loyalty_account_ref": e.LoyaltyAccountID, "entry_type": e.EntryType,
		"delta_points": e.DeltaPoints, "balance_after": e.BalanceAfter, "sequence": e.Sequence,
		"program_version": e.ProgramVersion, "reason_code": e.ReasonCode,
	}
}

// publishLoyaltyEntry emits the safe outbox payload for a ledger movement.
//
// Free-text reasons, command keys and source references never travel: a
// downstream consumer needs to know that a balance moved, not the operator's
// note about a customer complaint.
func (s *Service) publishLoyaltyEntry(ctx context.Context, tx pgx.Tx, actor Actor, eventType string, account LoyaltyAccount, entry LoyaltyEntry) error {
	return s.publishTx(ctx, tx, actor, eventType, "crm_loyalty_account", account.ID, account.Version, map[string]any{
		"business_ref": account.BusinessID, "customer_profile_ref": account.CustomerProfileID,
		"loyalty_account_ref": account.ID, "entry_ref": entry.ID, "entry_type": entry.EntryType,
		"delta_points": entry.DeltaPoints, "balance_after": entry.BalanceAfter,
		"program_version": entry.ProgramVersion, "occurred_at": entry.CreatedAt,
	})
}

func strPtr(value string) *string { return &value }

func strPtrOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
