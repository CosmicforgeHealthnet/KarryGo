package finance_projection

import (
	"context"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
)

type Service struct {
	authority  Authority
	authorizer Authorizer
	stepUp     StepUpConsumer
}

func NewService(authority Authority, authorizer Authorizer, stepUp StepUpConsumer) *Service {
	return &Service{authority: authority, authorizer: authorizer, stepUp: stepUp}
}

func (s *Service) require(ctx context.Context, actor Actor, permission string) error {
	if actor.StaffID == "" || actor.VendorID == "" || actor.SessionID == "" {
		return apperrors.Unauthorized("Authentication is required.", nil)
	}
	if s == nil || s.authority == nil || s.authorizer == nil {
		return apperrors.Internal("Vendor finance authority is unavailable.", nil)
	}
	allowed, err := s.authorizer.Authorize(ctx, actor.StaffID, permission, nil)
	if err != nil {
		return err
	}
	if !allowed {
		return apperrors.Forbidden("You do not have access to Vendor finance.", nil)
	}
	return nil
}

func (s *Service) Summary(ctx context.Context, actor Actor) (Summary, error) {
	if err := s.require(ctx, actor, PermissionRead); err != nil {
		return Summary{}, err
	}
	return s.authority.Summary(ctx, actor.VendorID)
}

func (s *Service) Transactions(ctx context.Context, actor Actor, limit int) ([]Transaction, error) {
	if err := s.require(ctx, actor, PermissionRead); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.authority.Transactions(ctx, actor.VendorID, limit)
}

func (s *Service) BankAccounts(ctx context.Context, actor Actor) ([]BankAccount, error) {
	if err := s.require(ctx, actor, PermissionRead); err != nil {
		return nil, err
	}
	return s.authority.BankAccounts(ctx, actor.VendorID)
}

func (s *Service) Withdrawals(ctx context.Context, actor Actor) ([]Withdrawal, error) {
	if err := s.require(ctx, actor, PermissionRead); err != nil {
		return nil, err
	}
	return s.authority.Withdrawals(ctx, actor.VendorID)
}

func (s *Service) Withdrawal(ctx context.Context, actor Actor, reference string) (Withdrawal, error) {
	if err := s.require(ctx, actor, PermissionRead); err != nil {
		return Withdrawal{}, err
	}
	if strings.TrimSpace(reference) == "" {
		return Withdrawal{}, apperrors.Validation("Withdrawal reference is required.", nil)
	}
	return s.authority.Withdrawal(ctx, actor.VendorID, reference)
}

func (s *Service) RequestWithdrawal(ctx context.Context, actor Actor, command WithdrawalCommand) (Withdrawal, error) {
	if err := s.require(ctx, actor, PermissionWithdraw); err != nil {
		return Withdrawal{}, err
	}
	if command.AmountKobo <= 0 || strings.TrimSpace(command.BankAccountID) == "" || strings.TrimSpace(command.IdempotencyKey) == "" || strings.TrimSpace(command.StepUpGrantID) == "" {
		return Withdrawal{}, apperrors.Validation("Bank account, positive amount, idempotency key and step-up grant are required.", nil)
	}
	if s.stepUp == nil || s.stepUp.ConsumeGrant(ctx, command.StepUpGrantID, actor.SessionID, actor.StaffID, actor.VendorID, WithdrawalStepUp) != nil {
		return Withdrawal{}, apperrors.Forbidden("A fresh withdrawal step-up grant is required.", nil)
	}
	return s.authority.RequestWithdrawal(ctx, actor.VendorID, command)
}
