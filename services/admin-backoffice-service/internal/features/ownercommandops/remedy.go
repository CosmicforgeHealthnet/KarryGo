package ownercommandops

import (
	"context"
	"errors"

	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminread"
)

// ErrRemedyUnconfirmed refuses a Support dispute remedy that is not backed by
// Wallet-confirmed refund operations orchestrated through this Bridge.
var ErrRemedyUnconfirmed = errors.New("dispute remedy lacks Wallet-confirmed refund")

// walletRemedyConfirmed requires, before Support may record a remedy, that
//   - wallet_operation_id is a CONFIRMED Wallet APPROVE_REFUND operation, and
//   - a CONFIRMED Wallet REQUEST_REFUND operation exists whose case_ref is the
//     Support dispute and whose amount equals the remedy amount,
//
// both within the same delegated market/organization. Support never moves
// money; it only links a refund Wallet has itself confirmed (Section 16D).
func (s *Service) walletRemedyConfirmed(ctx context.Context, q adminread.Query, cmd admincommand.Command) error {
	if s.Owner.Spec().Owner != "support-dispute-service" || cmd.Action != "RECORD_DISPUTE_REMEDY" {
		return nil
	}
	scope := Scope(q)
	var approved, requested bool
	err := s.DB.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM admin_owner_command_operations WHERE id::text=$1 AND owner='payment-wallet-service' AND action='APPROVE_REFUND' AND status='CONFIRMED' AND market=$4 AND organization_ref=$5),
 EXISTS(SELECT 1 FROM admin_owner_command_operations WHERE owner='payment-wallet-service' AND action='REQUEST_REFUND' AND status='CONFIRMED' AND case_ref=$2 AND command->'params'->>'amount_minor'=$3 AND market=$4 AND organization_ref=$5)`,
		cmd.Params["wallet_operation_id"], cmd.SubjectID, cmd.Params["amount_minor"], scope.Market, scope.Organization).Scan(&approved, &requested)
	if err != nil {
		return err
	}
	if !approved || !requested {
		return ErrRemedyUnconfirmed
	}
	return nil
}
