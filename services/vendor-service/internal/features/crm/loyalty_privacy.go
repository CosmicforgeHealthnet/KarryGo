package crm

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Section 8D privacy and identity-merge handling.
//
// Both plug into the Section 8A registry rather than introducing a second
// privacy or merge authority: customer-service decides that a subject is
// redacted or that two identities are the same person, and vendor-service's
// job is to make its own domain obey that decision inside the same
// transaction.

type loyaltyPrivacyHandler struct{ service *Service }

func (loyaltyPrivacyHandler) Domain() string { return "loyalty" }

// Restrict is a no-op. Restriction limits processing; it does not confiscate
// points a customer has already earned.
func (loyaltyPrivacyHandler) Restrict(context.Context, pgx.Tx, string, string) error { return nil }

// MarketingSuppress is a no-op, and deliberately so. Holding a points balance
// is not consent to be marketed at, and withdrawing marketing consent is not
// forfeiture of the balance. Conflating the two would quietly punish a customer
// for opting out.
func (loyaltyPrivacyHandler) MarketingSuppress(context.Context, pgx.Tx, string, string) error {
	return nil
}

// Redact makes the account non-actionable and removes it from every ordinary
// loyalty read.
//
// The ledger rows themselves are retained: they hold no name, email, phone or
// address to erase — only a vendor-local profile reference and integer point
// movements — and they are the evidence that a balance was legitimately
// granted and spent. Destroying them would break the ledger's integrity
// without deleting a single piece of personal data.
func (loyaltyPrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	_, err := tx.Exec(ctx, `UPDATE crm_loyalty_accounts SET status=$1,updated_at=now(),version=version+1
		WHERE business_id=$2 AND customer_profile_id=$3 AND status<>$1`,
		LoyaltyAccountRedacted, businessID, profileID)
	return err
}

// IdentityMerge moves a surviving points balance onto the target relationship.
//
// The source ledger is not rewritten. Other CRM domains re-point their rows at
// the target because an interaction is just a record about a person; a loyalty
// account is a balance with its own sequence, its own lots and its own
// history, and rewriting whose it was would falsify the audit trail (and
// collide with the one-account-per-relationship constraint whenever the target
// already had one). Instead the transfer is posted as an explicit MERGE_OUT /
// MERGE_IN pair: the source drains to zero and becomes non-actionable, the
// target receives the live points, and both ledgers say exactly what happened.
func (h loyaltyPrivacyHandler) IdentityMerge(ctx context.Context, tx pgx.Tx, businessID, sourceProfileID, targetProfileID string) error {
	s := h.service
	actor := Actor{VendorID: businessID, CorrelationID: "identity-merge"}

	source, err := scanLoyaltyAccount(tx.QueryRow(ctx, `SELECT `+loyaltyAccountCols+` FROM crm_loyalty_accounts
		WHERE business_id=$1 AND customer_profile_id=$2 FOR UPDATE`, businessID, sourceProfileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // nothing to merge
	}
	if err != nil {
		return err
	}
	if source.Status != LoyaltyAccountActive {
		return nil // already merged away or redacted
	}

	program, err := s.loadProgramForUpdate(ctx, tx, businessID)
	if err != nil {
		return err
	}
	// Expire what is already dead on the source before transferring, so the
	// target inherits live points only and never a lot that is due to vanish.
	if _, err = s.settleExpiredLots(ctx, tx, actor, &source, program, s.now()); err != nil {
		return err
	}

	transferable := source.CurrentPoints
	if transferable == 0 {
		return h.closeSource(ctx, tx, businessID, source.ID)
	}

	// Read the surviving lots before draining them: each becomes a lot on the
	// target carrying its original expiry, so a merge never buys the customer
	// a fresh expiry window and never shortens one either.
	rows, err := tx.Query(ctx, `SELECT `+loyaltyLotCols+` FROM crm_loyalty_point_lots
		WHERE business_id=$1 AND loyalty_account_id=$2 AND remaining_points>0
		ORDER BY expires_at ASC NULLS LAST, created_at ASC, id ASC FOR UPDATE`, businessID, source.ID)
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

	outEntry, err := s.postLoyaltyEntry(ctx, tx, actor, &source, loyaltyEntryInput{
		EntryType: LoyaltyMergeOut, Delta: -transferable, Program: program,
		SourceType: LoyaltySourceMerge, SourceReference: &targetProfileID, ReasonCode: strPtr("IDENTITY_MERGE"),
	})
	if err != nil {
		return err
	}
	for _, lot := range lots {
		if _, err = tx.Exec(ctx, `UPDATE crm_loyalty_point_lots SET remaining_points=0
			WHERE id=$1 AND business_id=$2`, lot.ID, businessID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO crm_loyalty_lot_consumptions
			(id,business_id,loyalty_account_id,debit_entry_id,lot_id,consumed_points)
			VALUES($1,$2,$3,$4,$5,$6)`, newID("crmlc_"), businessID, source.ID, outEntry.ID, lot.ID, lot.RemainingPoints); err != nil {
			return err
		}
	}
	if err = h.closeSource(ctx, tx, businessID, source.ID); err != nil {
		return err
	}

	target, err := s.lockLoyaltyAccount(ctx, tx, businessID, targetProfileID, true)
	if err != nil {
		return err
	}
	if target.Status != LoyaltyAccountActive {
		return errLoyaltyAccountNotActionable()
	}
	inEntry, err := s.postLoyaltyEntry(ctx, tx, actor, &target, loyaltyEntryInput{
		EntryType: LoyaltyMergeIn, Delta: transferable, Program: program,
		SourceType: LoyaltySourceMerge, SourceReference: &sourceProfileID, ReasonCode: strPtr("IDENTITY_MERGE"),
	})
	if err != nil {
		return err
	}
	for _, lot := range lots {
		if _, err = tx.Exec(ctx, `INSERT INTO crm_loyalty_point_lots
			(id,business_id,loyalty_account_id,source_entry_id,issued_points,remaining_points,expires_at,created_at)
			VALUES($1,$2,$3,$4,$5,$5,$6,$7)`,
			newID("crmll_"), businessID, target.ID, inEntry.ID, lot.RemainingPoints, lot.ExpiresAt, lot.CreatedAt); err != nil {
			return err
		}
	}

	if err = s.auditTx(ctx, tx, actor, "crm.loyalty.account.merged", "crm_loyalty_account", source.ID, "",
		map[string]any{"loyalty_account_ref": source.ID, "customer_profile_ref": sourceProfileID},
		map[string]any{"loyalty_account_ref": target.ID, "customer_profile_ref": targetProfileID,
			"transferred_points": transferable}, ""); err != nil {
		return err
	}
	return s.publishTx(ctx, tx, actor, "vendor.crm.loyalty.account.merged", "crm_loyalty_account", target.ID, target.Version,
		map[string]any{
			"business_ref": businessID, "loyalty_account_ref": target.ID,
			"customer_profile_ref": targetProfileID, "source_loyalty_account_ref": source.ID,
			"delta_points": transferable, "balance_after": target.CurrentPoints,
			"program_version": program.Version, "occurred_at": s.now(),
		})
}

func (loyaltyPrivacyHandler) closeSource(ctx context.Context, tx pgx.Tx, businessID, accountID string) error {
	_, err := tx.Exec(ctx, `UPDATE crm_loyalty_accounts SET status=$1,updated_at=now(),version=version+1
		WHERE id=$2 AND business_id=$3`, LoyaltyAccountMerged, accountID, businessID)
	return err
}
