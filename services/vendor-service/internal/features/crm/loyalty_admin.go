package crm

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Administrative loyalty commands: manual adjustment and reversal.
//
// These are the only two Section 8D operations exposed over HTTP that move
// points, and both are abuse-sensitive — a manual credit is the closest thing
// in the service to printing value. They are therefore bounded by the
// programme's own adjustment limit, gated behind their own permissions,
// keyed for exactly-once replay, and recorded with the operator's reason in
// the audit log (and only there).

// Adjustment directions.
const (
	LoyaltyDirectionCredit = "CREDIT"
	LoyaltyDirectionDebit  = "DEBIT"
)

// LoyaltyAdjustmentReasonMax bounds the operator's free-text justification.
const LoyaltyAdjustmentReasonMax = 500

// AdjustPointsInput is one manual adjustment.
//
// The amount is always positive: direction carries the sign, so a caller can
// never smuggle a credit through the debit route by sending a negative number.
type AdjustPointsInput struct {
	CustomerProfileRef string
	Direction          string
	Points             int64
	Reason             string
	CommandRef         string
}

// AdjustPoints applies a manual credit or debit.
//
// It is permitted while the programme is ACTIVE or PAUSED but not while it is
// DISABLED: undoing a mistake stays possible in every state (see
// ReverseLoyaltyEntry), but granting new points under a programme the business
// has switched off is not a correction, it is an award, and it should require
// turning the programme back on first.
func (s *Service) AdjustPoints(ctx context.Context, actor Actor, scope Scope, in AdjustPointsInput) (LoyaltyEntry, LoyaltyAccount, error) {
	if !s.flags.LoyaltyEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return LoyaltyEntry{}, LoyaltyAccount{}, errWritesDisabled()
	}
	direction := strings.ToUpper(strings.TrimSpace(in.Direction))
	if direction != LoyaltyDirectionCredit && direction != LoyaltyDirectionDebit {
		return LoyaltyEntry{}, LoyaltyAccount{}, loyaltyValidation("direction", "Direction must be CREDIT or DEBIT.", CodeLoyaltyInvalidPoints)
	}
	if err := validLoyaltyPoints(in.Points, MaxLoyaltyPoints); err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	reason, err := normalizePlainText(in.Reason, LoyaltyAdjustmentReasonMax, true, false, "reason")
	if err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	commandRef := strPtrOrNil(in.CommandRef)

	entry, account, err := s.mutateLoyalty(ctx, actor, scope, loyaltyCommand{
		ProfileID: in.CustomerProfileRef,
		// A manual credit is a legitimate first touch, so it may enrol the
		// customer. A manual debit against a customer who has never held
		// points is a mistake, and creating an empty account to reject it
		// would leave a misleading row behind.
		CreateAccount:   direction == LoyaltyDirectionCredit,
		RequireStatuses: []string{LoyaltyProgramActive, LoyaltyProgramPaused},
		Apply: func(ctx context.Context, tx pgx.Tx, account *LoyaltyAccount, program LoyaltyProgram) (LoyaltyEntry, error) {
			if in.Points > program.ManualAdjustmentLimit {
				return LoyaltyEntry{}, crmError(
					apperrors.Validation("That adjustment is larger than this programme allows.",
						[]apperrors.FieldViolation{{Field: "points", Message: "Above the configured manual adjustment limit."}}),
					CodeLoyaltyAdjustmentLimit)
			}
			var posted LoyaltyEntry
			var applyErr error
			if direction == LoyaltyDirectionCredit {
				posted, applyErr = s.applyCredit(ctx, tx, actor, account, program, creditRequest{
					EntryType: LoyaltyAdjustmentCredit, Points: in.Points, SourceType: LoyaltySourceManual,
					CommandRef: commandRef, ReasonCode: strPtr("MANUAL_CREDIT"), StaffID: staffRef(actor),
				})
			} else {
				posted, applyErr = s.applyDebit(ctx, tx, actor, account, program, debitRequest{
					EntryType: LoyaltyAdjustmentDebit, Points: in.Points, SourceType: LoyaltySourceManual,
					CommandRef: commandRef, ReasonCode: strPtr("MANUAL_DEBIT"), StaffID: staffRef(actor),
				})
			}
			if applyErr != nil {
				return LoyaltyEntry{}, applyErr
			}
			// The operator's own words live in the audit log and nowhere else:
			// not in the ledger row, not in the outbox payload, not in a log
			// line. The ledger carries a coarse reason code instead.
			if auditErr := s.auditTx(ctx, tx, actor, "crm.loyalty.adjustment.applied", "crm_loyalty_account", account.ID, "",
				nil, map[string]any{"entry_ref": posted.ID, "direction": direction, "points": in.Points,
					"balance_after": posted.BalanceAfter}, reason); auditErr != nil {
				return LoyaltyEntry{}, auditErr
			}
			if pubErr := s.publishLoyaltyEntry(ctx, tx, actor, "vendor.crm.loyalty.adjustment.applied", *account, posted); pubErr != nil {
				return LoyaltyEntry{}, pubErr
			}
			return posted, nil
		},
	})
	if err != nil {
		s.metrics.Inc("crm_loyalty_mutation_failure_total", "operation", "adjustment")
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	s.metrics.Inc("crm_loyalty_adjustment_total", "direction", direction)
	return entry, account, nil
}

func staffRef(actor Actor) *string {
	if strings.TrimSpace(actor.StaffID) == "" {
		return nil
	}
	return &actor.StaffID
}

// ---------------------------------------------------------------------------
// Reversal
// ---------------------------------------------------------------------------

// reversibleLoyaltyEntryTypes are the movements an operator may undo.
//
// EXPIRE, MERGE_IN, MERGE_OUT and REVERSAL itself are excluded on purpose.
// Expiry is the programme's own rule applying, merges are identity decisions
// owned by customer-service, and allowing a reversal to be reversed would turn
// a correction into an unbounded chain where the net effect of a ledger is no
// longer obvious from reading it.
var reversibleLoyaltyEntryTypes = map[string]bool{
	LoyaltyEarn: true, LoyaltyRedeem: true,
	LoyaltyAdjustmentCredit: true, LoyaltyAdjustmentDebit: true,
}

func isLoyaltyCredit(entryType string) bool {
	return entryType == LoyaltyEarn || entryType == LoyaltyAdjustmentCredit || entryType == LoyaltyMergeIn
}

// ReverseLoyaltyEntryInput is one full reversal. Partial reversal is not
// supported: an exact undo is unambiguous, whereas a partial one raises the
// question of which lots it came out of and cannot be answered consistently
// once the original allocation has been partly restored.
type ReverseLoyaltyEntryInput struct {
	Reason     string
	CommandRef string
}

// ReverseLoyaltyEntry posts a compensating entry for a previously posted one.
//
// The original row is never edited or deleted — that is what "immutable
// ledger" has to mean to be worth anything — and exactly one reversal can ever
// exist per entry, enforced by a unique index rather than by a read-then-write
// check a concurrent request could slip through.
//
// Reversal stays available in every programme status, including DISABLED,
// because a business that has switched loyalty off must still be able to
// correct an entry it posted while it was on.
func (s *Service) ReverseLoyaltyEntry(ctx context.Context, actor Actor, scope Scope, entryID string, in ReverseLoyaltyEntryInput) (LoyaltyEntry, LoyaltyAccount, error) {
	if !s.flags.LoyaltyEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return LoyaltyEntry{}, LoyaltyAccount{}, errWritesDisabled()
	}
	entryID = strings.TrimSpace(entryID)
	reason, err := normalizePlainText(in.Reason, LoyaltyAdjustmentReasonMax, true, false, "reason")
	if err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	commandRef := strPtrOrNil(in.CommandRef)

	// Resolve the entry's owner before opening the mutation, so the profile
	// fence runs against the customer this entry actually belongs to. A cross-
	// tenant entry id simply does not resolve.
	var profileID string
	err = s.db.QueryRow(ctx, `SELECT a.customer_profile_id FROM crm_loyalty_entries e
		JOIN crm_loyalty_accounts a ON a.id=e.loyalty_account_id AND a.business_id=e.business_id
		WHERE e.business_id=$1 AND e.id=$2`, actor.VendorID, entryID).Scan(&profileID)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoyaltyEntry{}, LoyaltyAccount{}, errLoyaltyEntryNotFound()
	}
	if err != nil {
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}

	entry, account, err := s.mutateLoyalty(ctx, actor, scope, loyaltyCommand{
		ProfileID: profileID, CreateAccount: false,
		Apply: func(ctx context.Context, tx pgx.Tx, account *LoyaltyAccount, program LoyaltyProgram) (LoyaltyEntry, error) {
			if existing, found, findErr := s.findAppliedEntry(ctx, tx, account.BusinessID, commandRef, "", nil); findErr != nil || found {
				return existing, findErr
			}
			original, loadErr := scanLoyaltyEntry(tx.QueryRow(ctx, `SELECT `+loyaltyEntryCols+` FROM crm_loyalty_entries
				WHERE business_id=$1 AND id=$2 AND loyalty_account_id=$3`, account.BusinessID, entryID, account.ID))
			if errors.Is(loadErr, pgx.ErrNoRows) {
				return LoyaltyEntry{}, errLoyaltyEntryNotFound()
			}
			if loadErr != nil {
				return LoyaltyEntry{}, loadErr
			}
			if !reversibleLoyaltyEntryTypes[original.EntryType] {
				return LoyaltyEntry{}, crmError(
					apperrors.Conflict("This kind of loyalty entry cannot be reversed.", nil), CodeLoyaltyEntryNotReversible)
			}
			var reversed bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_loyalty_entries
				WHERE business_id=$1 AND reverses_entry_id=$2)`, account.BusinessID, original.ID).Scan(&reversed); err != nil {
				return LoyaltyEntry{}, err
			}
			if reversed {
				return LoyaltyEntry{}, crmError(
					apperrors.Conflict("This loyalty entry has already been reversed.", nil), CodeLoyaltyAlreadyReversed)
			}

			var posted LoyaltyEntry
			var applyErr error
			if isLoyaltyCredit(original.EntryType) {
				posted, applyErr = s.reverseCredit(ctx, tx, actor, account, program, original, commandRef)
			} else {
				posted, applyErr = s.reverseDebit(ctx, tx, actor, account, program, original, commandRef)
			}
			if applyErr != nil {
				return LoyaltyEntry{}, applyErr
			}
			if auditErr := s.auditTx(ctx, tx, actor, "crm.loyalty.entry.reversed", "crm_loyalty_account", account.ID, "",
				safeLoyaltyEntryAudit(original), safeLoyaltyEntryAudit(posted), reason); auditErr != nil {
				return LoyaltyEntry{}, auditErr
			}
			if pubErr := s.publishLoyaltyEntry(ctx, tx, actor, "vendor.crm.loyalty.entry.reversed", *account, posted); pubErr != nil {
				return LoyaltyEntry{}, pubErr
			}
			// Restoring a debit can hand back points whose expiry has since
			// passed. Settling again inside the same transaction is what stops
			// a reversal from resurrecting them as fresh spendable points.
			if _, expErr := s.settleExpiredLots(ctx, tx, actor, account, program, s.now()); expErr != nil {
				return LoyaltyEntry{}, expErr
			}
			return posted, nil
		},
	})
	if err != nil {
		if isUniqueViolation(err) {
			s.metrics.Inc("crm_loyalty_mutation_failure_total", "operation", "reversal")
			return LoyaltyEntry{}, LoyaltyAccount{}, crmError(
				apperrors.Conflict("This loyalty entry has already been reversed.", nil), CodeLoyaltyAlreadyReversed)
		}
		s.metrics.Inc("crm_loyalty_mutation_failure_total", "operation", "reversal")
		return LoyaltyEntry{}, LoyaltyAccount{}, err
	}
	s.metrics.Inc("crm_loyalty_reversal_total", "entry_type", entry.EntryType)
	return entry, account, nil
}

// reverseCredit undoes an issuance by voiding its own lot.
//
// It deliberately refuses when the lot has been touched at all. Taking the
// points from some other lot instead would be arithmetically tidy and
// semantically wrong: those points came from a different issuance, possibly
// with a different expiry, and the customer would silently lose them. A
// refusal is recoverable — the operator can post a manual debit and say why.
func (s *Service) reverseCredit(ctx context.Context, tx pgx.Tx, actor Actor, account *LoyaltyAccount, program LoyaltyProgram, original LoyaltyEntry, commandRef *string) (LoyaltyEntry, error) {
	lot, err := scanLoyaltyLot(tx.QueryRow(ctx, `SELECT `+loyaltyLotCols+` FROM crm_loyalty_point_lots
		WHERE business_id=$1 AND source_entry_id=$2 FOR UPDATE`, account.BusinessID, original.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return LoyaltyEntry{}, crmError(
			apperrors.Conflict("These points can no longer be reversed.", nil), CodeLoyaltyAlreadyConsumed)
	}
	if err != nil {
		return LoyaltyEntry{}, err
	}
	// Spent or expired both land here: in either case the exact points this
	// entry created are no longer available to take back.
	if lot.RemainingPoints < lot.IssuedPoints {
		return LoyaltyEntry{}, crmError(
			apperrors.Conflict("Some of these points have already been used or have expired, so this entry cannot be reversed.", nil),
			CodeLoyaltyAlreadyConsumed)
	}
	posted, err := s.postLoyaltyEntry(ctx, tx, actor, account, loyaltyEntryInput{
		EntryType: LoyaltyReversal, Delta: -lot.IssuedPoints, Program: program,
		SourceType: LoyaltySourceReversal, IdempotencyReference: commandRef,
		ReversesEntryID: &original.ID, ReasonCode: strPtr("REVERSAL_CREDIT"), StaffID: staffRef(actor),
	})
	if err != nil {
		return LoyaltyEntry{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE crm_loyalty_point_lots SET remaining_points=0
		WHERE id=$1 AND business_id=$2`, lot.ID, account.BusinessID); err != nil {
		return LoyaltyEntry{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO crm_loyalty_lot_consumptions
		(id,business_id,loyalty_account_id,debit_entry_id,lot_id,consumed_points)
		VALUES($1,$2,$3,$4,$5,$6)`, newID("crmlc_"), account.BusinessID, account.ID, posted.ID, lot.ID, lot.IssuedPoints); err != nil {
		return LoyaltyEntry{}, err
	}
	return posted, nil
}

// reverseDebit gives back exactly the points the debit took, to exactly the
// lots it took them from.
//
// The allocation rows recorded at debit time are the whole reason this is
// possible: each restored point returns to the lot that funded it and keeps
// that lot's original expiry. If that expiry has since passed, the caller
// settles immediately afterwards so the restoration is honest rather than a
// quiet extension of the customer's window.
func (s *Service) reverseDebit(ctx context.Context, tx pgx.Tx, actor Actor, account *LoyaltyAccount, program LoyaltyProgram, original LoyaltyEntry, commandRef *string) (LoyaltyEntry, error) {
	type allocation struct {
		id       string
		lotID    string
		restore  int64
		consumed int64
	}
	rows, err := tx.Query(ctx, `SELECT c.id,c.lot_id,c.consumed_points,c.restored_points
		FROM crm_loyalty_lot_consumptions c
		JOIN crm_loyalty_point_lots l ON l.id=c.lot_id AND l.business_id=c.business_id
		WHERE c.business_id=$1 AND c.debit_entry_id=$2
		ORDER BY c.id FOR UPDATE OF c,l`, account.BusinessID, original.ID)
	if err != nil {
		return LoyaltyEntry{}, err
	}
	var allocations []allocation
	var total int64
	for rows.Next() {
		var a allocation
		var restored int64
		if err = rows.Scan(&a.id, &a.lotID, &a.consumed, &restored); err != nil {
			rows.Close()
			return LoyaltyEntry{}, err
		}
		a.restore = a.consumed - restored
		if a.restore > 0 {
			allocations = append(allocations, a)
			total += a.restore
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return LoyaltyEntry{}, err
	}
	if total == 0 {
		return LoyaltyEntry{}, crmError(
			apperrors.Conflict("These points have already been returned.", nil), CodeLoyaltyAlreadyReversed)
	}

	posted, err := s.postLoyaltyEntry(ctx, tx, actor, account, loyaltyEntryInput{
		EntryType: LoyaltyReversal, Delta: total, Program: program,
		SourceType: LoyaltySourceReversal, IdempotencyReference: commandRef,
		ReversesEntryID: &original.ID, ReasonCode: strPtr("REVERSAL_DEBIT"), StaffID: staffRef(actor),
	})
	if err != nil {
		return LoyaltyEntry{}, err
	}
	for _, a := range allocations {
		if _, err = tx.Exec(ctx, `UPDATE crm_loyalty_point_lots SET remaining_points=remaining_points+$1
			WHERE id=$2 AND business_id=$3`, a.restore, a.lotID, account.BusinessID); err != nil {
			return LoyaltyEntry{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE crm_loyalty_lot_consumptions SET restored_points=consumed_points
			WHERE id=$1 AND business_id=$2`, a.id, account.BusinessID); err != nil {
			return LoyaltyEntry{}, err
		}
	}
	return posted, nil
}
