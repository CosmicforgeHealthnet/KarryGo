package usecases

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// SetCreditProfileInput is an organization's credit policy for a customer.
//
// This is policy and nothing else. Fleet owns no receivables ledger, calculates
// no debt, ages no invoices and derives no available credit from transactions:
// a second, wrong set of books is worse than no books, and the accounting
// system that owns the real ones is not this service.
type SetCreditProfileInput struct {
	CreditLimitAmountMinor *int64
	Currency               string
	PaymentTermsDays       int
	ExpectedVersion        int64
	IdempotencyKey         string
	ReasonCode             string
	CorrelationID          string
	CausationID            string
}

// SetCreditProfile creates or updates a customer's credit policy.
//
// A change to the limit is routed through the WP9 approval policy. Extending
// credit is a finance decision with a real exposure attached, and the framework
// — not this feature — decides at what value a second pair of eyes is required.
func (s *Service) SetCreditProfile(ctx context.Context, t tenant.Context, customerID string,
	input SetCreditProfileInput) (models.CreditProfile, error) {
	if err := s.writeEnabled(s.opts.Flags.CreditEnabled); err != nil {
		return models.CreditProfile{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.CreditProfile{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if err := models.ValidateCurrency(currency); err != nil {
		return models.CreditProfile{}, badRequest(errorcodes.CreditProfileNotFound,
			"currency must be a three-letter ISO 4217 code.")
	}
	if input.CreditLimitAmountMinor != nil && *input.CreditLimitAmountMinor < 0 {
		return models.CreditProfile{}, badRequest(errorcodes.CreditProfileNotFound,
			"creditLimit.amountMinor must not be negative.")
	}
	if input.PaymentTermsDays < 0 {
		return models.CreditProfile{}, badRequest(errorcodes.CreditProfileNotFound,
			"paymentTermsDays must not be negative.")
	}

	var saved models.CreditProfile
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		customer, txErr := s.requireCustomer(ctx, tx, t, customerID)
		if txErr != nil {
			return txErr
		}
		if customer.IsArchived() {
			return conflict(errorcodes.CustomerArchived,
				"An archived customer's credit policy cannot be changed.")
		}

		existing, lookupErr := s.opts.Repository.LockCreditProfile(ctx, tx, t.OrganizationID,
			customerID)
		isNew := lookupErr != nil
		if !isNew && input.ExpectedVersion <= 0 {
			return badRequest(errorcodes.CreditProfileNotFound,
				"expectedVersion is required when updating an existing credit profile.")
		}

		approvalID := ""
		limitChanged := isNew ||
			!sameLimit(existing.CreditLimitAmountMinor, input.CreditLimitAmountMinor)
		if limitChanged && s.opts.Approvals != nil {
			approvalID, txErr = s.opts.Approvals.Request(ctx, ApprovalRequestInput{
				OrganizationID: t.OrganizationID, OrganizationUserID: t.OrganizationUserID,
				BranchID: customer.BranchID, ResourceType: "customer_credit_profile",
				ResourceID: customerID, Action: "set_limit",
				OperationType: "CRM_CREDIT_LIMIT_CHANGE",
				Payload: map[string]any{
					"customerId":       customerID,
					"currency":         currency,
					"creditLimitMinor": limitValue(input.CreditLimitAmountMinor),
					"paymentTermsDays": input.PaymentTermsDays,
				},
				ExpectedSubjectVersion: existing.Version,
				IdempotencyKey:         input.IdempotencyKey,
				CorrelationID:          input.CorrelationID, CausationID: input.CausationID,
			})
			if txErr != nil {
				return txErr
			}
			if approvalID != "" {
				// The framework enforces quorum, eligible roles, self-approval
				// policy and separation of duties; this feature only asks.
				if txErr = s.opts.Approvals.RequireApproved(ctx, t.OrganizationID, approvalID,
					map[string]any{
						"customerId":       customerID,
						"currency":         currency,
						"creditLimitMinor": limitValue(input.CreditLimitAmountMinor),
						"paymentTermsDays": input.PaymentTermsDays,
					}); txErr != nil {
					return conflict(errorcodes.CreditChangeApprovalRequired,
						"This credit limit change requires an approval that has not been granted.")
				}
			}
		}

		profile := models.CreditProfile{
			ID:                     existing.ID,
			OrganizationID:         t.OrganizationID,
			CustomerID:             customerID,
			CreditLimitAmountMinor: input.CreditLimitAmountMinor,
			Currency:               currency,
			PaymentTermsDays:       input.PaymentTermsDays,
			HoldStatus:             models.HoldNone,
			RiskStatusSnapshot:     customer.RiskStatus,
			ApprovedBy:             t.OrganizationUserID,
			ApprovalRequestID:      approvalID,
			CreatedBy:              t.OrganizationUserID,
			UpdatedBy:              t.OrganizationUserID,
		}
		if isNew {
			profile.ID = ids.New(repositories.PrefixCreditProfile)
			// A restricted customer's profile is created already restricted. A
			// fresh profile that starts clean would be a way to launder a risk
			// decision away by deleting and recreating the policy.
			if customer.RiskStatus == models.RiskRestricted {
				profile.HoldStatus = models.HoldRestricted
				profile.HoldReason = "customer_risk_restricted"
			}
		}

		saved, txErr = s.opts.Repository.UpsertCreditProfile(ctx, tx, profile,
			input.ExpectedVersion, !isNew)
		if txErr != nil {
			return translate(txErr, "Credit profile", errorcodes.CreditProfileNotFound,
				errorcodes.CreditProfileNotFound)
		}
		if isNew {
			if txErr = s.opts.Repository.AttachCreditProfile(ctx, tx, t.OrganizationID,
				customerID, saved.ID); txErr != nil {
				return txErr
			}
		}

		previousLimit := existing.CreditLimitAmountMinor
		previousHold := existing.HoldStatus
		if isNew {
			previousLimit, previousHold = nil, ""
		}
		if txErr = s.opts.Repository.AppendCreditHistory(ctx, tx, models.CreditHistory{
			OrganizationID: t.OrganizationID, CreditProfileID: saved.ID,
			CustomerID: customerID, PreviousHoldStatus: previousHold,
			NewHoldStatus: saved.HoldStatus, PreviousLimitMinor: previousLimit,
			NewLimitMinor: saved.CreditLimitAmountMinor, ReasonCode: input.ReasonCode,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			ApprovalRequestID: approvalID, CorrelationID: input.CorrelationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.credit_profile.updated",
			"customer_credit_profile", saved.ID,
			map[string]any{"holdStatus": previousHold},
			map[string]any{"holdStatus": saved.HoldStatus, "currency": saved.Currency},
			input.ReasonCode); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventCreditUpdated, "customer_credit_profile", saved.ID,
			saved.Version, t.OrganizationID, input.CorrelationID, creditEventPayload(saved))
	})
	if err != nil {
		return models.CreditProfile{}, err
	}
	return saved, nil
}

func sameLimit(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func limitValue(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

// GetCreditProfile reads a customer's credit policy.
func (s *Service) GetCreditProfile(ctx context.Context, t tenant.Context,
	customerID string) (models.CreditProfile, error) {
	if err := s.readEnabled(); err != nil {
		return models.CreditProfile{}, err
	}
	if _, err := s.requireCustomer(ctx, nil, t, customerID); err != nil {
		return models.CreditProfile{}, err
	}
	profile, err := s.opts.Repository.GetCreditProfile(ctx, nil, t.OrganizationID, customerID)
	return profile, translate(err, "Credit profile", errorcodes.CreditProfileNotFound,
		errorcodes.CreditProfileNotFound)
}

// CreditHoldInput applies or releases a credit hold.
type CreditHoldInput struct {
	HoldStatus      string
	ReasonCode      string
	ExpectedVersion int64
	IdempotencyKey  string
	CorrelationID   string
	CausationID     string
}

// ApplyCreditHold places a customer on credit hold.
func (s *Service) ApplyCreditHold(ctx context.Context, t tenant.Context, customerID string,
	input CreditHoldInput) (models.CreditProfile, error) {
	if input.HoldStatus == "" {
		input.HoldStatus = models.HoldOnHold
	}
	if !models.BlockingHold(input.HoldStatus) &&
		input.HoldStatus != models.HoldReviewRequired {
		return models.CreditProfile{}, badRequest(errorcodes.CustomerCreditHold,
			"holdStatus must be REVIEW_REQUIRED, ON_HOLD or RESTRICTED.")
	}
	return s.changeCreditHold(ctx, t, customerID, input, false)
}

// ReleaseCreditHold clears a customer's credit hold.
//
// Releasing a hold requires an approval where the policy asks for one: a hold
// is a deliberate block, and removing one is the change that lets new
// commitments through.
func (s *Service) ReleaseCreditHold(ctx context.Context, t tenant.Context, customerID string,
	input CreditHoldInput) (models.CreditProfile, error) {
	input.HoldStatus = models.HoldNone
	return s.changeCreditHold(ctx, t, customerID, input, true)
}

func (s *Service) changeCreditHold(ctx context.Context, t tenant.Context, customerID string,
	input CreditHoldInput, isRelease bool) (models.CreditProfile, error) {
	if err := s.writeEnabled(s.opts.Flags.CreditEnabled); err != nil {
		return models.CreditProfile{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.CreditProfile{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.CreditProfile{}, err
	}
	if strings.TrimSpace(input.ReasonCode) == "" {
		return models.CreditProfile{}, badRequest(errorcodes.CustomerCreditHold,
			"reasonCode is required for a credit hold change.")
	}

	var updated models.CreditProfile
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		customer, txErr := s.requireCustomer(ctx, tx, t, customerID)
		if txErr != nil {
			return txErr
		}
		existing, txErr := s.opts.Repository.LockCreditProfile(ctx, tx, t.OrganizationID,
			customerID)
		if txErr != nil {
			return translate(txErr, "Credit profile", errorcodes.CreditProfileNotFound,
				errorcodes.CreditProfileNotFound)
		}
		if existing.HoldStatus == input.HoldStatus {
			updated = existing
			return nil
		}
		// A restricted customer's hold cannot be released. Doing so would use
		// the credit surface to undo a risk decision that has its own,
		// separately permissioned, workflow.
		if isRelease && customer.RiskStatus == models.RiskRestricted {
			return conflict(errorcodes.CustomerRiskRestricted,
				"A restricted customer's credit hold cannot be released while the restriction stands.")
		}

		approvalID := ""
		if isRelease && s.opts.Approvals != nil {
			approvalID, txErr = s.opts.Approvals.Request(ctx, ApprovalRequestInput{
				OrganizationID: t.OrganizationID, OrganizationUserID: t.OrganizationUserID,
				BranchID: customer.BranchID, ResourceType: "customer_credit_profile",
				ResourceID: customerID, Action: "release_hold",
				OperationType: "CRM_CREDIT_HOLD_RELEASE",
				Payload: map[string]any{
					"customerId":         customerID,
					"previousHoldStatus": existing.HoldStatus,
					"reasonCode":         input.ReasonCode,
				},
				ExpectedSubjectVersion: existing.Version,
				IdempotencyKey:         input.IdempotencyKey,
				CorrelationID:          input.CorrelationID, CausationID: input.CausationID,
			})
			if txErr != nil {
				return txErr
			}
			if approvalID != "" {
				if txErr = s.opts.Approvals.RequireApproved(ctx, t.OrganizationID, approvalID,
					map[string]any{
						"customerId":         customerID,
						"previousHoldStatus": existing.HoldStatus,
						"reasonCode":         input.ReasonCode,
					}); txErr != nil {
					return conflict(errorcodes.CreditChangeApprovalRequired,
						"Releasing this credit hold requires an approval that has not been granted.")
				}
			}
		}

		updated, txErr = s.opts.Repository.SetCreditHold(ctx, tx, t.OrganizationID, customerID,
			input.ExpectedVersion, existing.HoldStatus, input.HoldStatus, input.ReasonCode,
			approvalID, t.OrganizationUserID)
		if txErr != nil {
			return translate(txErr, "Credit profile", errorcodes.CreditProfileNotFound,
				errorcodes.CustomerCreditHold)
		}
		if txErr = s.opts.Repository.AppendCreditHistory(ctx, tx, models.CreditHistory{
			OrganizationID: t.OrganizationID, CreditProfileID: updated.ID,
			CustomerID: customerID, PreviousHoldStatus: existing.HoldStatus,
			NewHoldStatus:      updated.HoldStatus,
			PreviousLimitMinor: existing.CreditLimitAmountMinor,
			NewLimitMinor:      updated.CreditLimitAmountMinor,
			ReasonCode:         input.ReasonCode, ActorType: string(t.ActorType),
			ActorID: t.ActorID, ApprovalRequestID: approvalID,
			CorrelationID: input.CorrelationID,
		}); txErr != nil {
			return txErr
		}
		action := "crm.credit_profile.hold_applied"
		intent := models.IntentCreditHold
		if isRelease {
			action = "crm.credit_profile.hold_released"
			intent = models.IntentCreditHoldReleased
		}
		if txErr = s.record(ctx, tx, t, action, "customer_credit_profile", updated.ID,
			map[string]any{"holdStatus": existing.HoldStatus},
			map[string]any{"holdStatus": updated.HoldStatus}, input.ReasonCode); txErr != nil {
			return txErr
		}
		primary := s.primaryContactFor(ctx, tx, t.OrganizationID, customerID,
			models.PurposeBilling)
		if txErr = s.enqueueNotification(ctx, tx, t.OrganizationID, "customer_credit_profile",
			updated.ID, intent, primary.ID, notificationChannelFor(primary),
			models.JSONMap{"holdStatus": updated.HoldStatus},
			"credit_hold:"+updated.ID+":"+input.HoldStatus, input.CorrelationID,
			input.CausationID); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventCreditHoldChanged, "customer_credit_profile",
			updated.ID, updated.Version, t.OrganizationID, input.CorrelationID,
			creditHoldEventPayload(updated, existing.HoldStatus, input.ReasonCode))
	})
	if err != nil {
		return models.CreditProfile{}, err
	}
	s.count("customer_credit_holds_total", map[string]string{"hold": input.HoldStatus})
	return updated, nil
}

// CreditHistory returns a page of a customer's credit policy changes.
func (s *Service) CreditHistory(ctx context.Context, t tenant.Context, customerID string,
	page cursor.Cursor, hasCursor bool, limit int) ([]models.CreditHistory, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	profile, err := s.GetCreditProfile(ctx, t, customerID)
	if err != nil {
		return nil, err
	}
	return s.opts.Repository.ListCreditHistory(ctx, t.OrganizationID, profile.ID, page,
		hasCursor, limit)
}
