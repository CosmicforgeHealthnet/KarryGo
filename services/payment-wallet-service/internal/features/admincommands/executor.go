// Package admincommands binds the common private Admin command contract to
// Wallet's existing financial repositories and permission model.
package admincommands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"cosmicforge/logistics/services/payment-wallet-service/internal/events"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/adminresources"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5"
)

const Owner = adminresources.Owner

type Executor struct {
	Accounts    *repositories.AccountRepository
	Ledger      *repositories.LedgerRepository
	Operations  *repositories.OperationsRepository
	Reversals   *repositories.ReversalRepository
	ProviderOps *repositories.ProviderOperationsRepository
	Marketplace *repositories.MarketplaceRepository
	Audit       *audit.Recorder
	Events      *events.DomainPublisher
}

func Spec() admincommand.Spec { return admincommand.WalletSpec() }

func (e Executor) AuthorizeStaff(ctx context.Context, tx pgx.Tx, staff, kind, id string) error {
	return adminresources.Source().AuthorizeStaff(ctx, tx, staff, kind, id)
}

func (e Executor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	item, err := adminresources.Source().Read(ctx, tx, kind, id, false)
	if err != nil {
		return 0, "", err
	}
	var version int64
	state := "UNKNOWN"
	if item.SourceVersion != nil {
		version = *item.SourceVersion
	}
	if item.Status != nil {
		state = *item.Status
	}
	return version, state, nil
}

func commandFingerprint(values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = fmt.Fprintf(h, "%s\x00", value)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func actor(in admincommand.Execution, organization string) financeauth.Actor {
	return financeauth.Actor{Type: financeauth.ActorUser, ID: in.Staff, OrganizationID: organization, CorrelationID: in.OperationID, RequestID: in.OperationID}
}

func (e Executor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	if in.Spec.VersionOf == admincommand.VersionCase {
		return e.applyCase(ctx, tx, in)
	}
	organization, _, err := adminresources.NativeTarget(ctx, tx, in.SubjectKind, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	switch in.Action {
	case "RESTRICT_ACCOUNT", "SUSPEND_ACCOUNT", "ACTIVATE_ACCOUNT":
		return e.applyWalletStatus(ctx, tx, in, organization)
	case "APPROVE_REFUND":
		return e.approveRefund(ctx, tx, in, organization)
	case "APPROVE_REVERSAL", "REJECT_REVERSAL":
		return e.decideReversal(ctx, tx, in, organization)
	case "APPROVE_PROVIDER_WITHDRAWAL", "REJECT_PROVIDER_WITHDRAWAL":
		return e.decideProviderWithdrawal(ctx, tx, in, organization)
	case "APPROVE_SETTLEMENT":
		return e.approveSettlement(ctx, tx, in, organization)
	default:
		return admincommand.Transition{}, admincommand.ErrRejected
	}
}

func (e Executor) applyWalletStatus(ctx context.Context, tx pgx.Tx, in admincommand.Execution, organization string) (admincommand.Transition, error) {
	if e.Accounts == nil || e.Audit == nil {
		return admincommand.Transition{}, admincommand.ErrDisabled
	}
	permission := financeauth.PermWalletRestrict
	status := models.StatusRestricted
	restrictions := []string{in.Params["restriction"]}
	if in.Action == "SUSPEND_ACCOUNT" {
		permission, status = financeauth.PermWalletSuspend, models.StatusSuspended
	}
	if in.Action == "ACTIVATE_ACCOUNT" {
		status, restrictions = models.StatusActive, []string{}
	}
	if err := adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, organization, permission); err != nil {
		return admincommand.Transition{}, admincommand.ErrNotFound
	}
	wallet, err := e.Accounts.GetWalletByID(ctx, tx, organization, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, mapRepositoryError(err)
	}
	if wallet.Version != in.ExpectedVersion {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	if wallet.Status == status && in.Action == "ACTIVATE_ACCOUNT" {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	if err = e.Accounts.SetWalletStatus(ctx, tx, wallet.ID, wallet.Version, status, restrictions); err != nil {
		return admincommand.Transition{}, mapRepositoryError(err)
	}
	if err = repositories.AppendHistory(ctx, tx, organization, "WALLET", wallet.ID, wallet.Status, status, financeauth.ActorUser, in.Staff, in.ReasonCode, nil, in.OperationID, in.IdempotencyKey); err != nil {
		return admincommand.Transition{}, err
	}
	if err = e.Audit.Record(ctx, tx, audit.Entry{OrganizationID: organization, ActorType: financeauth.ActorUser, ActorID: in.Staff, Action: "wallet.status_changed", ResourceType: "wallet", ResourceID: wallet.ID, PreviousValues: map[string]any{"status": wallet.Status}, NewValues: map[string]any{"status": status, "restrictions": restrictions}, Reason: in.ReasonCode, RequestID: in.OperationID, CorrelationID: in.OperationID}); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: wallet.Version, PriorState: wallet.Status, ResultingState: status, Outcome: "APPLIED_BY_WALLET_OWNER"}, nil
}

func (e Executor) approveRefund(ctx context.Context, tx pgx.Tx, in admincommand.Execution, organization string) (admincommand.Transition, error) {
	if e.Operations == nil || e.Audit == nil {
		return admincommand.Transition{}, admincommand.ErrDisabled
	}
	if err := adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, organization, financeauth.PermRefundApprove); err != nil {
		return admincommand.Transition{}, admincommand.ErrNotFound
	}
	refund, err := e.Operations.GetRefund(ctx, tx, organization, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, mapRepositoryError(err)
	}
	if refund.Version != in.ExpectedVersion {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	if _, err = financeauth.NewAuthorizer(tx).RequireApproval(ctx, actor(in, organization), financeauth.PermRefundApprove, financeauth.OperationRefundApproval, refund.RequesterIdentity()); err != nil {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if err = e.Operations.ApproveRefund(ctx, tx, refund.ID, refund.Version, financeauth.ActorUser, in.Staff); err != nil {
		return admincommand.Transition{}, mapRepositoryError(err)
	}
	if err = repositories.AppendHistory(ctx, tx, organization, "REFUND", refund.ID, refund.Status, models.RefundApproved, financeauth.ActorUser, in.Staff, in.ReasonCode, nil, in.OperationID, in.IdempotencyKey); err != nil {
		return admincommand.Transition{}, err
	}
	if err = e.Audit.Record(ctx, tx, audit.Entry{OrganizationID: organization, ActorType: financeauth.ActorUser, ActorID: in.Staff, Action: "refund.approved", ResourceType: "refund", ResourceID: refund.ID, PreviousValues: map[string]any{"status": refund.Status}, NewValues: map[string]any{"status": models.RefundApproved}, Reason: in.ReasonCode, RequestID: in.OperationID, CorrelationID: in.OperationID}); err != nil {
		return admincommand.Transition{}, err
	}
	if e.Events != nil {
		if err = e.Events.Publish(ctx, tx, "wallet.refund.status_changed.v1", "REFUND", refund.ID, refund.Version+1, organization, in.OperationID, in.IdempotencyKey, map[string]any{"refundId": refund.ID, "status": models.RefundApproved}); err != nil {
			return admincommand.Transition{}, err
		}
	}
	return admincommand.Transition{PriorVersion: refund.Version, PriorState: refund.Status, ResultingState: models.RefundApproved, Outcome: "REFUND_APPROVED_BY_WALLET"}, nil
}

func (e Executor) applyCase(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	organization, permission, err := adminresources.NativeTarget(ctx, tx, in.SubjectKind, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if err = adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, organization, permission); err != nil {
		return admincommand.Transition{}, admincommand.ErrNotFound
	}
	var storedOrganization, kind, id, state string
	var version int64
	var ownerOperation string
	err = tx.QueryRow(ctx, `SELECT organization_id,subject_kind,subject_id,state,version,owner_operation_ref FROM wallet_admin_cases WHERE case_ref=$1 FOR UPDATE`, in.CaseRef).Scan(&storedOrganization, &kind, &id, &state, &version, &ownerOperation)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return admincommand.Transition{}, err
	}
	if exists && (storedOrganization != organization || kind != in.SubjectKind || id != in.SubjectID) {
		return admincommand.Transition{}, admincommand.ErrNotFound
	}
	if version != in.ExpectedVersion {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	prior, next, outcome := state, "", "WALLET_CASE_TRANSITION_RECORDED"
	switch in.Action {
	case "OPEN_FINANCIAL_INVESTIGATION":
		if exists {
			return admincommand.Transition{}, admincommand.ErrConflict
		}
		prior, next = "NONE", "OPEN"
	case "ESCALATE_FINANCIAL_INVESTIGATION":
		if state != "OPEN" && state != "REVIEWED" {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		next = "ESCALATED"
	case "RECORD_FINANCIAL_FINDING":
		if state != "OPEN" && state != "ESCALATED" {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		next = "REVIEWED"
	case "REQUEST_REFUND":
		if state != "REVIEWED" || ownerOperation != "" {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		ownerOperation, err = e.requestRefund(ctx, tx, in, organization)
		if err != nil {
			return admincommand.Transition{}, err
		}
		next, outcome = "OWNER_ACTION_REQUESTED", "REFUND_REQUESTED_FROM_WALLET"
	case "REQUEST_REVERSAL":
		if state != "REVIEWED" || ownerOperation != "" {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		ownerOperation, err = e.requestReversal(ctx, tx, in, organization)
		if err != nil {
			return admincommand.Transition{}, err
		}
		next, outcome = "OWNER_ACTION_REQUESTED", "REVERSAL_REQUESTED_FROM_WALLET"
	case "REQUEST_SETTLEMENT":
		if state != "REVIEWED" || ownerOperation != "" {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		ownerOperation, err = e.requestSettlement(ctx, tx, in, organization)
		if err != nil {
			return admincommand.Transition{}, err
		}
		next, outcome = "OWNER_ACTION_REQUESTED", "SETTLEMENT_REQUESTED_FROM_WALLET"
	case "CLOSE_FINANCIAL_INVESTIGATION":
		if state != "REVIEWED" && state != "OWNER_ACTION_REQUESTED" {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		next = "CLOSED"
	default:
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if !exists {
		_, err = tx.Exec(ctx, `INSERT INTO wallet_admin_cases(case_ref,organization_id,subject_kind,subject_id,category,severity,state,version,opened_by) VALUES($1,$2,$3,$4,$5,$6,$7,1,$8)`, in.CaseRef, organization, in.SubjectKind, in.SubjectID, in.Params["category"], in.Params["severity"], next, in.Staff)
	} else {
		_, err = tx.Exec(ctx, `UPDATE wallet_admin_cases SET state=$2,version=version+1,owner_operation_ref=$3,updated_at=now() WHERE case_ref=$1 AND version=$4`, in.CaseRef, next, ownerOperation, version)
	}
	if err != nil {
		return admincommand.Transition{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO wallet_admin_case_history(id,case_ref,operation_id,action,prior_state,resulting_state,prior_version,resulting_version,staff_ref,reason_code,owner_operation_ref) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, in.CaseRef, in.OperationID, in.Action, prior, next, version, version+1, in.Staff, in.ReasonCode, ownerOperation)
	return admincommand.Transition{PriorVersion: version, PriorState: prior, ResultingState: next, Outcome: outcome}, err
}

func (e Executor) requestReversal(ctx context.Context, tx pgx.Tx, in admincommand.Execution, organization string) (string, error) {
	if e.Ledger == nil || e.Reversals == nil || e.Audit == nil {
		return "", admincommand.ErrDisabled
	}
	if err := adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, organization, financeauth.PermReversalRequest); err != nil {
		return "", admincommand.ErrNotFound
	}
	amount, err := strconv.ParseInt(in.Params["amount_minor"], 10, 64)
	if err != nil || amount <= 0 {
		return "", admincommand.ErrRejected
	}
	journal, err := e.Ledger.GetJournal(ctx, tx, in.SubjectID)
	if err != nil {
		return "", mapRepositoryError(err)
	}
	if journal.OrganizationID != organization || journal.Currency == "" {
		return "", admincommand.ErrNotFound
	}
	reversible, err := e.Ledger.ReversibleMinor(ctx, tx, journal.ID)
	if err != nil {
		return "", mapRepositoryError(err)
	}
	if amount > reversible {
		return "", admincommand.ErrRejected
	}
	policy, err := financeauth.NewAuthorizer(tx).ApprovalPolicyFor(ctx, organization, financeauth.OperationReversalApproval)
	if err != nil {
		return "", admincommand.ErrRejected
	}
	status := models.ReversalRequested
	if policy.MinimumApprovals > 0 {
		status = models.ReversalPendingApproval
	}
	reversal := &models.Reversal{OrganizationID: organization, OriginalJournalID: journal.ID, OriginalReference: journal.Reference, AmountMinor: amount, Currency: journal.Currency, ReasonCode: in.ReasonCode, Status: status, IdempotencyKey: in.IdempotencyKey, RequestFingerprint: commandFingerprint(organization, journal.ID, strconv.FormatInt(amount, 10), in.ReasonCode), CorrelationID: in.OperationID, CausationID: in.CaseRef, CreatedByActorType: financeauth.ActorUser, CreatedByActorID: in.Staff}
	if err = e.Reversals.Create(ctx, tx, reversal); err != nil {
		return "", mapRepositoryError(err)
	}
	if err = repositories.AppendHistory(ctx, tx, organization, "REVERSAL", reversal.ID, "", reversal.Status, financeauth.ActorUser, in.Staff, in.ReasonCode, map[string]any{"originalJournalId": journal.ID, "amountMinor": amount, "currency": journal.Currency}, in.OperationID, in.CaseRef); err != nil {
		return "", err
	}
	if err = e.Audit.Record(ctx, tx, audit.Entry{OrganizationID: organization, ActorType: financeauth.ActorUser, ActorID: in.Staff, Action: "reversal.requested", ResourceType: "reversal", ResourceID: reversal.ID, NewValues: map[string]any{"status": reversal.Status, "amountMinor": amount, "currency": journal.Currency, "originalJournalId": journal.ID}, Reason: in.ReasonCode, RequestID: in.OperationID, CorrelationID: in.OperationID}); err != nil {
		return "", err
	}
	if e.Events != nil {
		if err = e.Events.Publish(ctx, tx, "wallet.reversal.requested.v1", "REVERSAL", reversal.ID, reversal.Version, organization, in.OperationID, in.CaseRef, map[string]any{"reversalId": reversal.ID, "status": reversal.Status, "originalJournalId": journal.ID, "amountMinor": amount, "currency": journal.Currency}); err != nil {
			return "", err
		}
	}
	return reversal.ID, nil
}

func (e Executor) decideReversal(ctx context.Context, tx pgx.Tx, in admincommand.Execution, organization string) (admincommand.Transition, error) {
	if e.Reversals == nil || e.Audit == nil {
		return admincommand.Transition{}, admincommand.ErrDisabled
	}
	if err := adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, organization, financeauth.PermReversalApprove); err != nil {
		return admincommand.Transition{}, admincommand.ErrNotFound
	}
	reversal, err := e.Reversals.Get(ctx, tx, organization, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, mapRepositoryError(err)
	}
	if reversal.Version != in.ExpectedVersion {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	policy, err := financeauth.NewAuthorizer(tx).RequireApproval(ctx, actor(in, organization), financeauth.PermReversalApprove, financeauth.OperationReversalApproval, reversal.RequesterIdentity())
	if err != nil {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	next, event := models.ReversalApproved, "reversal.approved"
	if in.Action == "REJECT_REVERSAL" {
		next, event = models.ReversalRejected, "reversal.rejected"
		err = e.Reversals.Reject(ctx, tx, reversal.ID, reversal.Version, financeauth.ActorUser, in.Staff, in.ReasonCode)
	} else {
		err = e.Reversals.Approve(ctx, tx, reversal.ID, reversal.Version, financeauth.ActorUser, in.Staff, policy.ID, policy.PolicyVersion)
	}
	if err != nil {
		return admincommand.Transition{}, mapRepositoryError(err)
	}
	if err = repositories.AppendHistory(ctx, tx, organization, "REVERSAL", reversal.ID, reversal.Status, next, financeauth.ActorUser, in.Staff, in.ReasonCode, map[string]any{"approvalPolicyId": policy.ID}, in.OperationID, in.IdempotencyKey); err != nil {
		return admincommand.Transition{}, err
	}
	if err = e.Audit.Record(ctx, tx, audit.Entry{OrganizationID: organization, ActorType: financeauth.ActorUser, ActorID: in.Staff, Action: event, ResourceType: "reversal", ResourceID: reversal.ID, PreviousValues: map[string]any{"status": reversal.Status}, NewValues: map[string]any{"status": next, "approvalPolicyId": policy.ID}, Reason: in.ReasonCode, RequestID: in.OperationID, CorrelationID: in.OperationID}); err != nil {
		return admincommand.Transition{}, err
	}
	if e.Events != nil {
		if err = e.Events.Publish(ctx, tx, "wallet.reversal.status_changed.v1", "REVERSAL", reversal.ID, reversal.Version+1, organization, in.OperationID, in.IdempotencyKey, map[string]any{"reversalId": reversal.ID, "status": next}); err != nil {
			return admincommand.Transition{}, err
		}
	}
	return admincommand.Transition{PriorVersion: reversal.Version, PriorState: reversal.Status, ResultingState: next, Outcome: "REVERSAL_DECIDED_BY_WALLET"}, nil
}

func (e Executor) requestSettlement(ctx context.Context, tx pgx.Tx, in admincommand.Execution, organization string) (string, error) {
	if e.Accounts == nil || e.Marketplace == nil || e.Operations == nil || e.Audit == nil {
		return "", admincommand.ErrDisabled
	}
	if err := adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, organization, financeauth.PermSettlementInitiate); err != nil {
		return "", admincommand.ErrNotFound
	}
	expected, err := strconv.ParseInt(in.Params["expected_account_version"], 10, 64)
	if err != nil || expected <= 0 {
		return "", admincommand.ErrRejected
	}
	limit := 100
	if raw := in.Params["limit"]; raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed <= 0 || parsed > 200 {
			return "", admincommand.ErrRejected
		}
		limit = parsed
	}
	account, err := e.Accounts.GetEarningAccountByID(ctx, tx, in.SubjectID)
	if err != nil {
		return "", mapRepositoryError(err)
	}
	if account.OrganizationID != organization || account.Version != expected || !account.Payable() || account.PayoutDestinationRef == "" {
		return "", admincommand.ErrConflict
	}
	allocations, err := e.Marketplace.SettleableAllocations(ctx, tx, account.ID, account.Currency, limit)
	if err != nil {
		return "", mapRepositoryError(err)
	}
	if len(allocations) == 0 {
		return "", admincommand.ErrRejected
	}
	batch := &models.SettlementBatch{OrganizationID: organization, BeneficiaryType: models.BeneficiaryProvider, BeneficiaryReference: account.ProviderReference, EarningAccountID: account.ID, Currency: account.Currency, Status: models.BatchPendingApproval, RequestedByType: financeauth.ActorUser, RequestedByID: in.Staff, IdempotencyKey: in.IdempotencyKey, RequestFingerprint: commandFingerprint(organization, account.ID, strconv.FormatInt(expected, 10)), CorrelationID: in.OperationID}
	if err = e.Operations.CreateSettlementBatch(ctx, tx, batch, allocations); err != nil {
		return "", mapRepositoryError(err)
	}
	if err = repositories.AppendHistory(ctx, tx, organization, "SETTLEMENT_BATCH", batch.ID, "", batch.Status, financeauth.ActorUser, in.Staff, in.ReasonCode, map[string]any{"itemCount": batch.ItemCount}, in.OperationID, in.CaseRef); err != nil {
		return "", err
	}
	if err = e.Audit.Record(ctx, tx, audit.Entry{OrganizationID: organization, ActorType: financeauth.ActorUser, ActorID: in.Staff, Action: "settlement.requested", ResourceType: "settlement_batch", ResourceID: batch.ID, NewValues: map[string]any{"totalMinor": batch.TotalMinor, "currency": batch.Currency, "itemCount": batch.ItemCount}, Reason: in.ReasonCode, RequestID: in.OperationID, CorrelationID: in.OperationID}); err != nil {
		return "", err
	}
	if e.Events != nil {
		if err = e.Events.Publish(ctx, tx, "wallet.settlement.status_changed.v1", "SETTLEMENT_BATCH", batch.ID, batch.Version, organization, in.OperationID, in.CaseRef, map[string]any{"settlementBatchId": batch.ID, "status": batch.Status, "totalMinor": batch.TotalMinor, "currency": batch.Currency, "itemCount": batch.ItemCount}); err != nil {
			return "", err
		}
	}
	return batch.ID, nil
}

func (e Executor) approveSettlement(ctx context.Context, tx pgx.Tx, in admincommand.Execution, organization string) (admincommand.Transition, error) {
	if e.Operations == nil || e.Audit == nil {
		return admincommand.Transition{}, admincommand.ErrDisabled
	}
	if err := adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, organization, financeauth.PermSettlementApprove); err != nil {
		return admincommand.Transition{}, admincommand.ErrNotFound
	}
	batch, err := e.Operations.GetSettlementBatch(ctx, tx, organization, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, mapRepositoryError(err)
	}
	if batch.Version != in.ExpectedVersion {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	if _, err = financeauth.NewAuthorizer(tx).RequireApproval(ctx, actor(in, organization), financeauth.PermSettlementApprove, financeauth.OperationSettlementApproval, batch.RequesterIdentity()); err != nil {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if err = e.Operations.ApproveSettlementBatch(ctx, tx, batch.ID, batch.Version, financeauth.ActorUser, in.Staff); err != nil {
		return admincommand.Transition{}, mapRepositoryError(err)
	}
	if err = repositories.AppendHistory(ctx, tx, organization, "SETTLEMENT_BATCH", batch.ID, batch.Status, models.BatchApproved, financeauth.ActorUser, in.Staff, in.ReasonCode, nil, in.OperationID, in.IdempotencyKey); err != nil {
		return admincommand.Transition{}, err
	}
	if err = e.Audit.Record(ctx, tx, audit.Entry{OrganizationID: organization, ActorType: financeauth.ActorUser, ActorID: in.Staff, Action: "settlement.approved", ResourceType: "settlement_batch", ResourceID: batch.ID, PreviousValues: map[string]any{"status": batch.Status}, NewValues: map[string]any{"status": models.BatchApproved}, Reason: in.ReasonCode, RequestID: in.OperationID, CorrelationID: in.OperationID}); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: batch.Version, PriorState: batch.Status, ResultingState: models.BatchApproved, Outcome: "SETTLEMENT_APPROVED_BY_WALLET"}, nil
}

func (e Executor) requestRefund(ctx context.Context, tx pgx.Tx, in admincommand.Execution, organization string) (string, error) {
	if e.Ledger == nil || e.Operations == nil || e.Audit == nil {
		return "", admincommand.ErrDisabled
	}
	if err := adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, organization, financeauth.PermRefundRequest); err != nil {
		return "", admincommand.ErrNotFound
	}
	amount, err := strconv.ParseInt(in.Params["amount_minor"], 10, 64)
	if err != nil || amount <= 0 {
		return "", admincommand.ErrRejected
	}
	journal, err := e.Ledger.GetJournal(ctx, tx, in.SubjectID)
	if err != nil {
		return "", mapRepositoryError(err)
	}
	if journal.OrganizationID != organization || journal.Currency == "" {
		return "", admincommand.ErrNotFound
	}
	remaining, err := e.Ledger.RefundableMinor(ctx, tx, journal.ID)
	if err != nil {
		return "", mapRepositoryError(err)
	}
	if amount > remaining {
		return "", admincommand.ErrRejected
	}
	refund := &models.Refund{OrganizationID: organization, OriginalJournalID: journal.ID, OriginalReference: journal.Reference, AmountMinor: amount, Currency: journal.Currency, ReasonCode: in.ReasonCode, Status: models.RefundRequested, RequestedByType: financeauth.ActorUser, RequestedByID: in.Staff, IdempotencyKey: in.IdempotencyKey, RequestFingerprint: commandFingerprint(organization, journal.ID, strconv.FormatInt(amount, 10), in.ReasonCode), CorrelationID: in.OperationID}
	if err = e.Operations.CreateRefund(ctx, tx, refund); err != nil {
		return "", mapRepositoryError(err)
	}
	if err = repositories.AppendHistory(ctx, tx, organization, "REFUND", refund.ID, "", refund.Status, financeauth.ActorUser, in.Staff, in.ReasonCode, nil, in.OperationID, journal.ID); err != nil {
		return "", err
	}
	if err = e.Audit.Record(ctx, tx, audit.Entry{OrganizationID: organization, ActorType: financeauth.ActorUser, ActorID: in.Staff, Action: "refund.requested", ResourceType: "refund", ResourceID: refund.ID, NewValues: map[string]any{"amountMinor": amount, "currency": refund.Currency, "originalJournalId": journal.ID}, Reason: in.ReasonCode, RequestID: in.OperationID, CorrelationID: in.OperationID}); err != nil {
		return "", err
	}
	if e.Events != nil {
		if err = e.Events.Publish(ctx, tx, "wallet.refund.status_changed.v1", "REFUND", refund.ID, refund.Version, organization, in.OperationID, journal.ID, map[string]any{"refundId": refund.ID, "status": refund.Status, "amountMinor": amount, "currency": refund.Currency, "originalJournalId": journal.ID}); err != nil {
			return "", err
		}
	}
	return refund.ID, nil
}

func (e Executor) decideProviderWithdrawal(ctx context.Context, tx pgx.Tx, in admincommand.Execution, organization string) (admincommand.Transition, error) {
	if e.ProviderOps == nil || e.Accounts == nil || e.Audit == nil {
		return admincommand.Transition{}, admincommand.ErrDisabled
	}
	if err := adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, organization, financeauth.PermProviderWithdrawalApprove); err != nil {
		return admincommand.Transition{}, admincommand.ErrNotFound
	}
	withdrawal, err := e.ProviderOps.LockWithdrawal(ctx, tx, organization, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, mapRepositoryError(err)
	}
	if withdrawal.Resolved() || withdrawal.Version != in.ExpectedVersion {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	if _, err = financeauth.NewAuthorizer(tx).RequireApproval(ctx, actor(in, organization), financeauth.PermProviderWithdrawalApprove, models.OperationProviderWithdrawalApproval, withdrawal.RequesterIdentity()); err != nil {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	status, event := models.WithdrawalApproved, "wallet.withdrawal.approved.v1"
	if in.Action == "REJECT_PROVIDER_WITHDRAWAL" {
		status, event = models.WithdrawalRejected, "wallet.withdrawal.rejected.v1"
	}
	approvalRef := ""
	if in.Command.Approval != nil {
		approvalRef = in.Command.Approval.ApprovalID
	}
	if err = e.ProviderOps.DecideWithdrawal(ctx, tx, withdrawal.ID, withdrawal.Version, status, financeauth.ActorUser, in.Staff, approvalRef); err != nil {
		return admincommand.Transition{}, mapRepositoryError(err)
	}
	if status == models.WithdrawalRejected {
		account, accountErr := e.Accounts.LockEarningAccount(ctx, tx, withdrawal.EarningAccountID)
		if accountErr != nil {
			return admincommand.Transition{}, mapRepositoryError(accountErr)
		}
		if moveErr := e.Accounts.MoveEarningBalances(ctx, tx, account.ID, account.Version, withdrawal.AmountMinor, -withdrawal.AmountMinor, 0); moveErr != nil {
			return admincommand.Transition{}, mapRepositoryError(moveErr)
		}
	}
	if err = repositories.AppendHistory(ctx, tx, organization, "PROVIDER_WITHDRAWAL", withdrawal.ID, withdrawal.Status, status, financeauth.ActorUser, in.Staff, in.ReasonCode, nil, in.OperationID, in.IdempotencyKey); err != nil {
		return admincommand.Transition{}, err
	}
	if err = e.Audit.Record(ctx, tx, audit.Entry{OrganizationID: organization, ActorType: financeauth.ActorUser, ActorID: in.Staff, Action: "provider_withdrawal.decided", ResourceType: "provider_withdrawal", ResourceID: withdrawal.ID, PreviousValues: map[string]any{"status": withdrawal.Status}, NewValues: map[string]any{"status": status, "reasonCode": in.ReasonCode}, Reason: in.ReasonCode, RequestID: in.OperationID, CorrelationID: in.OperationID}); err != nil {
		return admincommand.Transition{}, err
	}
	if e.Events != nil {
		if err = e.Events.Publish(ctx, tx, event, "PROVIDER_WITHDRAWAL", withdrawal.ID, withdrawal.Version+1, organization, in.OperationID, in.IdempotencyKey, map[string]any{"withdrawalId": withdrawal.ID, "providerSubWalletId": withdrawal.EarningAccountID, "amountMinor": withdrawal.AmountMinor, "currency": withdrawal.Currency, "status": status}); err != nil {
			return admincommand.Transition{}, err
		}
	}
	return admincommand.Transition{PriorVersion: withdrawal.Version, PriorState: withdrawal.Status, ResultingState: status, Outcome: "PROVIDER_WITHDRAWAL_DECIDED_BY_WALLET"}, nil
}

func mapRepositoryError(err error) error {
	switch {
	case errors.Is(err, repositories.ErrNotFound):
		return admincommand.ErrNotFound
	case errors.Is(err, repositories.ErrConflict), errors.Is(err, repositories.ErrVersionMismatch):
		return admincommand.ErrConflict
	default:
		return err
	}
}

var _ adminowner.StaffAuthorizer = Executor{}
