package usecases

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// CreateOpportunityInput opens a sales pursuit.
type CreateOpportunityInput struct {
	BranchID                 string
	CustomerID               string
	Title                    string
	Description              string
	ExpectedValueAmountMinor *int64
	ExpectedValueCurrency    string
	ProbabilityBasisPoints   int
	ExpectedCloseDate        *time.Time
	ServiceCapabilities      []string
	AccountOwnerUserID       string
	NextAction               string
	NextActionAt             *time.Time
	IdempotencyKey           string
	CorrelationID            string
	CausationID              string
}

// CreateOpportunity opens a sales pursuit against a customer.
func (s *Service) CreateOpportunity(ctx context.Context, t tenant.Context,
	input CreateOpportunityInput) (models.Opportunity, error) {
	if err := s.writeEnabled(s.opts.Flags.OpportunitiesEnabled); err != nil {
		return models.Opportunity{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Opportunity{}, err
	}
	if strings.TrimSpace(input.Title) == "" {
		return models.Opportunity{}, badRequest(errorcodes.OpportunityNotFound,
			"title is required.")
	}
	if err := models.ValidateProbability(input.ProbabilityBasisPoints); err != nil {
		return models.Opportunity{}, badRequest(errorcodes.OpportunityInvalidTransition,
			err.Error())
	}
	// A value without a currency is a number nobody can add up.
	if (input.ExpectedValueAmountMinor == nil) != (input.ExpectedValueCurrency == "") {
		return models.Opportunity{}, badRequest(errorcodes.OpportunityInvalidTransition,
			"expectedValue.amountMinor and expectedValue.currency must both be present or both absent.")
	}
	if input.ExpectedValueCurrency != "" {
		upper := strings.ToUpper(input.ExpectedValueCurrency)
		if err := models.ValidateCurrency(upper); err != nil {
			return models.Opportunity{}, badRequest(errorcodes.OpportunityInvalidTransition,
				"expectedValue.currency must be a three-letter ISO 4217 code.")
		}
		input.ExpectedValueCurrency = upper
	}
	if input.BranchID != "" && !t.BranchPermitted(input.BranchID) {
		return models.Opportunity{}, badRequest(errorcodes.OpportunityNotFound,
			"branchId is not within your authorized scope.")
	}

	var created models.Opportunity
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		customer, txErr := s.requireCustomer(ctx, tx, t, input.CustomerID)
		if txErr != nil {
			return txErr
		}
		if customer.IsArchived() {
			return conflict(errorcodes.CustomerArchived,
				"An archived customer cannot have new opportunities.")
		}
		record := models.Opportunity{
			ID:                       ids.New(repositories.PrefixOpportunity),
			OrganizationID:           t.OrganizationID,
			BranchID:                 input.BranchID,
			CustomerID:               input.CustomerID,
			Title:                    strings.TrimSpace(input.Title),
			Description:              input.Description,
			Stage:                    models.StageLead,
			ExpectedValueAmountMinor: input.ExpectedValueAmountMinor,
			ExpectedValueCurrency:    input.ExpectedValueCurrency,
			ProbabilityBasisPoints:   input.ProbabilityBasisPoints,
			ExpectedCloseDate:        input.ExpectedCloseDate,
			ServiceCapabilities:      input.ServiceCapabilities,
			AccountOwnerUserID:       input.AccountOwnerUserID,
			NextAction:               input.NextAction,
			NextActionAt:             input.NextActionAt,
			CreatedBy:                t.OrganizationUserID,
		}
		created, txErr = s.opts.Repository.CreateOpportunity(ctx, tx, record)
		if txErr != nil {
			return translate(txErr, "Opportunity", errorcodes.OpportunityNotFound,
				errorcodes.OpportunityInvalidTransition)
		}
		if txErr = s.record(ctx, tx, t, "crm.opportunity.created", "sales_opportunity",
			created.ID, nil, opportunityAuditValues(created), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventOpportunityCreated, "sales_opportunity", created.ID,
			created.Version, t.OrganizationID, input.CorrelationID,
			opportunityEventPayload(created))
	})
	if err != nil {
		return models.Opportunity{}, err
	}
	return created, nil
}

// GetOpportunity reads one opportunity.
func (s *Service) GetOpportunity(ctx context.Context, t tenant.Context,
	opportunityID string) (models.Opportunity, error) {
	if err := s.readEnabled(); err != nil {
		return models.Opportunity{}, err
	}
	return s.requireOpportunity(ctx, nil, t, opportunityID)
}

func (s *Service) requireOpportunity(ctx context.Context, q pgx.Tx, t tenant.Context,
	opportunityID string) (models.Opportunity, error) {
	var opportunity models.Opportunity
	var err error
	if q == nil {
		opportunity, err = s.opts.Repository.GetOpportunity(ctx, nil, t.OrganizationID,
			opportunityID)
	} else {
		opportunity, err = s.opts.Repository.GetOpportunity(ctx, q, t.OrganizationID,
			opportunityID)
	}
	if err != nil {
		return opportunity, translate(err, "Opportunity", errorcodes.OpportunityNotFound,
			errorcodes.OpportunityInvalidTransition)
	}
	if !t.BranchPermitted(opportunity.BranchID) {
		return opportunity, notFound("Opportunity")
	}
	return opportunity, nil
}

// ListOpportunities returns a tenant-scoped page.
func (s *Service) ListOpportunities(ctx context.Context, t tenant.Context,
	filter repositories.OpportunityFilter) ([]models.Opportunity, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if filter.Stage != "" && !models.ValidStage(filter.Stage) {
		return nil, badRequest(errorcodes.OpportunityInvalidTransition,
			"stage filter is not supported.")
	}
	return s.opts.Repository.ListOpportunities(ctx, t.OrganizationID, t.PermittedBranchIDs,
		filter)
}

// UpdateOpportunityInput carries a partial opportunity update.
type UpdateOpportunityInput struct {
	Title                    *string
	Description              *string
	ExpectedValueAmountMinor *int64
	ExpectedValueCurrency    *string
	ClearExpectedValue       bool
	ProbabilityBasisPoints   *int
	ExpectedCloseDate        *time.Time
	ServiceCapabilities      []string
	AccountOwnerUserID       *string
	NextAction               *string
	NextActionAt             *time.Time
	ExpectedVersion          int64
	CorrelationID            string
	CausationID              string
}

// UpdateOpportunity applies a partial update to a non-terminal opportunity.
func (s *Service) UpdateOpportunity(ctx context.Context, t tenant.Context, opportunityID string,
	input UpdateOpportunityInput) (models.Opportunity, error) {
	if err := s.writeEnabled(s.opts.Flags.OpportunitiesEnabled); err != nil {
		return models.Opportunity{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Opportunity{}, err
	}
	if input.ProbabilityBasisPoints != nil {
		if err := models.ValidateProbability(*input.ProbabilityBasisPoints); err != nil {
			return models.Opportunity{}, badRequest(errorcodes.OpportunityInvalidTransition,
				err.Error())
		}
	}
	if input.ExpectedValueCurrency != nil {
		upper := strings.ToUpper(*input.ExpectedValueCurrency)
		if err := models.ValidateCurrency(upper); err != nil {
			return models.Opportunity{}, badRequest(errorcodes.OpportunityInvalidTransition,
				"expectedValue.currency must be a three-letter ISO 4217 code.")
		}
		input.ExpectedValueCurrency = &upper
	}

	var updated models.Opportunity
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.opts.Repository.LockOpportunity(ctx, tx, t.OrganizationID,
			opportunityID)
		if txErr != nil {
			return translate(txErr, "Opportunity", errorcodes.OpportunityNotFound,
				errorcodes.OpportunityInvalidTransition)
		}
		if !t.BranchPermitted(existing.BranchID) {
			return notFound("Opportunity")
		}
		if models.TerminalStage(existing.Stage) {
			return conflict(errorcodes.OpportunityInvalidTransition,
				"A won or lost opportunity cannot be edited.")
		}
		// The pairing rule has to hold after the merge, not just within the
		// request: clearing only the currency of an opportunity that has a value
		// would leave an amount nobody can interpret.
		amount := existing.ExpectedValueAmountMinor
		currency := existing.ExpectedValueCurrency
		if input.ClearExpectedValue {
			amount, currency = nil, ""
		} else {
			if input.ExpectedValueAmountMinor != nil {
				amount = input.ExpectedValueAmountMinor
			}
			if input.ExpectedValueCurrency != nil {
				currency = *input.ExpectedValueCurrency
			}
		}
		if (amount == nil) != (currency == "") {
			return badRequest(errorcodes.OpportunityInvalidTransition,
				"expectedValue.amountMinor and expectedValue.currency must both be present or both absent.")
		}

		updated, txErr = s.opts.Repository.UpdateOpportunity(ctx, tx, t.OrganizationID,
			opportunityID, input.ExpectedVersion, repositories.OpportunityUpdate{
				Title: input.Title, Description: input.Description,
				ExpectedValueAmountMinor: input.ExpectedValueAmountMinor,
				ExpectedValueCurrency:    input.ExpectedValueCurrency,
				ClearExpectedValue:       input.ClearExpectedValue,
				ProbabilityBasisPoints:   input.ProbabilityBasisPoints,
				ExpectedCloseDate:        input.ExpectedCloseDate,
				ServiceCapabilities:      input.ServiceCapabilities,
				AccountOwnerUserID:       input.AccountOwnerUserID,
				NextAction:               input.NextAction, NextActionAt: input.NextActionAt,
				UpdatedBy: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Opportunity", errorcodes.OpportunityNotFound,
				errorcodes.OpportunityInvalidTransition)
		}
		if txErr = s.record(ctx, tx, t, "crm.opportunity.updated", "sales_opportunity",
			opportunityID, opportunityAuditValues(existing), opportunityAuditValues(updated),
			""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventOpportunityUpdated, "sales_opportunity", opportunityID,
			updated.Version, t.OrganizationID, input.CorrelationID,
			opportunityEventPayload(updated))
	})
	if err != nil {
		return models.Opportunity{}, err
	}
	return updated, nil
}

// TransitionOpportunityInput is a controlled stage change.
type TransitionOpportunityInput struct {
	Stage           string
	ReasonCode      string
	ExpectedVersion int64
	IdempotencyKey  string
	CorrelationID   string
	CausationID     string
}

// TransitionOpportunity moves an opportunity through the stage matrix.
//
// Marking an opportunity WON creates nothing. It does not open a contract, does
// not activate one and does not commit the organization to anything: winning a
// deal and signing an agreement are separate decisions with separate approvals,
// and collapsing them would let a salesperson bind the company.
func (s *Service) TransitionOpportunity(ctx context.Context, t tenant.Context,
	opportunityID string, input TransitionOpportunityInput) (models.Opportunity, error) {
	if err := s.writeEnabled(s.opts.Flags.OpportunitiesEnabled); err != nil {
		return models.Opportunity{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Opportunity{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Opportunity{}, err
	}
	if !models.ValidStage(input.Stage) {
		return models.Opportunity{}, badRequest(errorcodes.OpportunityInvalidTransition,
			"stage is not supported.")
	}
	// A lost deal with no reason teaches nobody anything, and a hold with no
	// reason is indistinguishable from neglect.
	if input.Stage == models.StageLost && strings.TrimSpace(input.ReasonCode) == "" {
		return models.Opportunity{}, badRequest(errorcodes.OpportunityInvalidTransition,
			"reasonCode is required when marking an opportunity lost.")
	}
	if input.Stage == models.StageOnHold && strings.TrimSpace(input.ReasonCode) == "" {
		return models.Opportunity{}, badRequest(errorcodes.OpportunityInvalidTransition,
			"reasonCode is required when placing an opportunity on hold.")
	}

	var updated models.Opportunity
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.opts.Repository.LockOpportunity(ctx, tx, t.OrganizationID,
			opportunityID)
		if txErr != nil {
			return translate(txErr, "Opportunity", errorcodes.OpportunityNotFound,
				errorcodes.OpportunityInvalidTransition)
		}
		if !t.BranchPermitted(existing.BranchID) {
			return notFound("Opportunity")
		}
		if existing.Stage == input.Stage {
			updated = existing
			return nil
		}
		if models.TerminalStage(existing.Stage) {
			// WON and LOST are final. Reopening one silently would rewrite a
			// closed commercial outcome that reporting has already counted.
			return conflict(errorcodes.OpportunityInvalidTransition,
				"A won or lost opportunity cannot be reopened.")
		}

		var priorStages []string
		if existing.Stage == models.StageOnHold {
			// Resuming is only permitted into a stage this opportunity actually
			// reached, read from the immutable history. Without that check a
			// hold would be a way to skip qualification.
			priorStages, txErr = s.opts.Repository.OpportunityPriorStages(ctx, tx,
				t.OrganizationID, opportunityID)
			if txErr != nil {
				return txErr
			}
		}
		if !models.CanTransitionOpportunity(existing.Stage, input.Stage, priorStages) {
			return conflict(errorcodes.OpportunityInvalidTransition,
				"That stage change is not permitted from the opportunity's current stage.")
		}

		lossReason, holdReason := "", ""
		switch input.Stage {
		case models.StageLost:
			lossReason = input.ReasonCode
		case models.StageOnHold:
			holdReason = input.ReasonCode
		}
		updated, txErr = s.opts.Repository.TransitionOpportunity(ctx, tx, t.OrganizationID,
			opportunityID, input.ExpectedVersion, existing.Stage, input.Stage, lossReason,
			holdReason, t.OrganizationUserID)
		if txErr != nil {
			return translate(txErr, "Opportunity", errorcodes.OpportunityNotFound,
				errorcodes.OpportunityInvalidTransition)
		}
		if txErr = s.opts.Repository.AppendOpportunityHistory(ctx, tx,
			models.OpportunityHistory{
				OrganizationID: t.OrganizationID, OpportunityID: opportunityID,
				PreviousStage: existing.Stage, NewStage: updated.Stage,
				ReasonCode: input.ReasonCode, ActorType: string(t.ActorType),
				ActorID: t.ActorID, CorrelationID: input.CorrelationID,
			}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.opportunity.stage_changed", "sales_opportunity",
			opportunityID, map[string]any{"stage": existing.Stage},
			map[string]any{"stage": updated.Stage}, input.ReasonCode); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventOpportunityStageChanged, "sales_opportunity",
			opportunityID, updated.Version, t.OrganizationID, input.CorrelationID,
			opportunityStageEventPayload(updated, existing.Stage, input.ReasonCode))
	})
	if err != nil {
		return models.Opportunity{}, err
	}
	s.count("sales_opportunity_stage_changes_total", map[string]string{"stage": updated.Stage})
	return updated, nil
}

// OpportunityHistory returns a page of an opportunity's stage changes.
func (s *Service) OpportunityHistory(ctx context.Context, t tenant.Context,
	opportunityID string, page cursor.Cursor, hasCursor bool,
	limit int) ([]models.OpportunityHistory, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireOpportunity(ctx, nil, t, opportunityID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListOpportunityHistory(ctx, t.OrganizationID, opportunityID, page,
		hasCursor, limit)
}

// opportunityAuditValues builds the audit projection.
func opportunityAuditValues(opportunity models.Opportunity) map[string]any {
	return map[string]any{
		"title":                  opportunity.Title,
		"stage":                  opportunity.Stage,
		"probabilityBasisPoints": opportunity.ProbabilityBasisPoints,
		"branchId":               opportunity.BranchID,
		"customerId":             opportunity.CustomerID,
		"hasExpectedValue":       opportunity.ExpectedValueAmountMinor != nil,
	}
}
