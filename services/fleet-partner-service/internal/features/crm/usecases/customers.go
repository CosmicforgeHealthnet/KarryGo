package usecases

import (
	"context"
	"errors"
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

// CreateCustomerInput is a request to open a CRM customer record.
type CreateCustomerInput struct {
	BranchID              string
	CustomerType          string
	LegalName             string
	DisplayName           string
	RegistrationNumber    string
	TaxReference          string
	MarketplaceCustomerID string
	AccountOwnerUserID    string
	DefaultCurrency       string
	PaymentTermsReference string
	IdempotencyKey        string
	CorrelationID         string
	CausationID           string
}

// CreateCustomer opens a CRM customer record.
//
// The record is a commercial relationship, not an identity. No marketplace
// account is created, no credential is stored, and MarketplaceCustomerID — if
// supplied — is kept as an opaque pointer that this service never resolves by
// reaching into customer-service's database.
func (s *Service) CreateCustomer(ctx context.Context, t tenant.Context,
	input CreateCustomerInput) (models.Customer, error) {
	if err := s.writeEnabled(s.opts.Flags.CustomerWritesEnabled); err != nil {
		return models.Customer{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Customer{}, err
	}
	if err := s.validateCustomerInput(t, &input); err != nil {
		return models.Customer{}, err
	}

	normalized := models.NormalizeName(input.LegalName)
	customer := models.Customer{
		ID:                    ids.New(repositories.PrefixCustomer),
		OrganizationID:        t.OrganizationID,
		BranchID:              input.BranchID,
		CustomerType:          input.CustomerType,
		LegalName:             strings.TrimSpace(input.LegalName),
		DisplayName:           strings.TrimSpace(input.DisplayName),
		NormalizedName:        normalized,
		RegistrationNumber:    strings.TrimSpace(input.RegistrationNumber),
		TaxReference:          strings.TrimSpace(input.TaxReference),
		MarketplaceCustomerID: strings.TrimSpace(input.MarketplaceCustomerID),
		Status:                models.CustomerStatusProspect,
		RiskStatus:            models.RiskNotAssessed,
		AccountOwnerUserID:    input.AccountOwnerUserID,
		DefaultCurrency:       strings.ToUpper(input.DefaultCurrency),
		PaymentTermsReference: input.PaymentTermsReference,
		CreatedBy:             t.OrganizationUserID,
	}
	customer.DuplicateFingerprint = models.DuplicateFingerprint(customer.CustomerType,
		normalized, customer.RegistrationNumber, customer.TaxReference)

	var created models.Customer
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var txErr error
		created, txErr = s.opts.Repository.CreateCustomer(ctx, tx, customer)
		if txErr != nil {
			if errors.Is(txErr, repositories.ErrDuplicate) {
				// Scoped to this organization only. The same legal entity may
				// legitimately be a customer of two different fleet operators,
				// and treating that as a conflict would make one tenant's data
				// a constraint on another's.
				return conflict(errorcodes.CustomerAlreadyExists,
					"A customer with these identifying details already exists in this organization.")
			}
			return translate(txErr, "Customer", errorcodes.CustomerNotFound,
				errorcodes.CustomerAlreadyExists)
		}
		if txErr = s.opts.Repository.AppendCustomerHistory(ctx, tx, models.CustomerHistory{
			OrganizationID: t.OrganizationID, CustomerID: created.ID,
			ChangeType: models.ChangeCreated, NewStatus: created.Status,
			NewRiskStatus: created.RiskStatus, ActorType: string(t.ActorType),
			ActorID: t.ActorID, CorrelationID: input.CorrelationID,
			CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.customer.created", "crm_customer", created.ID,
			nil, customerAuditValues(created), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventCustomerCreated, "crm_customer", created.ID,
			created.Version, t.OrganizationID, input.CorrelationID,
			customerEventPayload(created))
	})
	if err != nil {
		return models.Customer{}, err
	}
	s.count("crm_customers_created_total", map[string]string{"type": created.CustomerType})
	return created, nil
}

func (s *Service) validateCustomerInput(t tenant.Context, input *CreateCustomerInput) error {
	if !models.ValidCustomerType(input.CustomerType) {
		return badRequest(errorcodes.CustomerInvalidState, "customerType is not supported.")
	}
	if strings.TrimSpace(input.LegalName) == "" {
		return badRequest(errorcodes.CustomerInvalidState, "legalName is required.")
	}
	if strings.TrimSpace(input.DisplayName) == "" {
		input.DisplayName = input.LegalName
	}
	if err := models.ValidateCurrency(strings.ToUpper(input.DefaultCurrency)); err != nil {
		return badRequest(errorcodes.CustomerInvalidState,
			"defaultCurrency must be a three-letter ISO 4217 code.")
	}
	if input.BranchID != "" && !t.BranchPermitted(input.BranchID) {
		return badRequest(errorcodes.CustomerInvalidState,
			"branchId is not within your authorized scope.")
	}
	return nil
}

// GetCustomer reads one customer.
func (s *Service) GetCustomer(ctx context.Context, t tenant.Context,
	customerID string) (models.Customer, error) {
	if err := s.readEnabled(); err != nil {
		return models.Customer{}, err
	}
	return s.requireCustomer(ctx, nil, t, customerID)
}

// ListCustomersInput is the allowlisted customer list query.
type ListCustomersInput struct {
	Status             string
	RiskStatus         string
	CustomerType       string
	BranchID           string
	AccountOwnerUserID string
	CreatedAfter       *time.Time
	CreatedBefore      *time.Time
	Search             string
	Cursor             cursor.Cursor
	HasCursor          bool
	Limit              int
}

// ListCustomers returns a tenant-scoped page.
func (s *Service) ListCustomers(ctx context.Context, t tenant.Context,
	input ListCustomersInput) ([]models.Customer, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if input.Status != "" && !validCustomerStatus(input.Status) {
		return nil, badRequest(errorcodes.CustomerInvalidState, "status filter is not supported.")
	}
	if input.RiskStatus != "" && !models.ValidRiskStatus(input.RiskStatus) {
		return nil, badRequest(errorcodes.CustomerInvalidState,
			"riskStatus filter is not supported.")
	}
	if input.CustomerType != "" && !models.ValidCustomerType(input.CustomerType) {
		return nil, badRequest(errorcodes.CustomerInvalidState,
			"customerType filter is not supported.")
	}
	return s.opts.Repository.ListCustomers(ctx, t.OrganizationID, t.PermittedBranchIDs,
		repositories.CustomerFilter{
			Status: input.Status, RiskStatus: input.RiskStatus,
			CustomerType: input.CustomerType, BranchID: input.BranchID,
			AccountOwnerUserID: input.AccountOwnerUserID, CreatedAfter: input.CreatedAfter,
			CreatedBefore: input.CreatedBefore, Search: input.Search,
			Cursor: input.Cursor, HasCursor: input.HasCursor, Limit: input.Limit,
		})
}

func validCustomerStatus(status string) bool {
	switch status {
	case models.CustomerStatusProspect, models.CustomerStatusActive, models.CustomerStatusOnHold,
		models.CustomerStatusSuspended, models.CustomerStatusInactive,
		models.CustomerStatusArchived:
		return true
	}
	return false
}

// UpdateCustomerInput carries a partial customer update.
type UpdateCustomerInput struct {
	BranchID              *string
	LegalName             *string
	DisplayName           *string
	RegistrationNumber    *string
	TaxReference          *string
	MarketplaceCustomerID *string
	AccountOwnerUserID    *string
	DefaultCurrency       *string
	PaymentTermsReference *string
	ExpectedVersion       int64
	CorrelationID         string
	CausationID           string
}

// UpdateCustomer applies a partial update to a non-archived customer.
func (s *Service) UpdateCustomer(ctx context.Context, t tenant.Context, customerID string,
	input UpdateCustomerInput) (models.Customer, error) {
	if err := s.writeEnabled(s.opts.Flags.CustomerWritesEnabled); err != nil {
		return models.Customer{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Customer{}, err
	}
	if input.DefaultCurrency != nil {
		upper := strings.ToUpper(*input.DefaultCurrency)
		if err := models.ValidateCurrency(upper); err != nil {
			return models.Customer{}, badRequest(errorcodes.CustomerInvalidState,
				"defaultCurrency must be a three-letter ISO 4217 code.")
		}
		input.DefaultCurrency = &upper
	}
	if input.BranchID != nil && *input.BranchID != "" && !t.BranchPermitted(*input.BranchID) {
		return models.Customer{}, badRequest(errorcodes.CustomerInvalidState,
			"branchId is not within your authorized scope.")
	}

	var updated models.Customer
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.opts.Repository.LockCustomer(ctx, tx, t.OrganizationID, customerID)
		if txErr != nil {
			return translate(txErr, "Customer", errorcodes.CustomerNotFound,
				errorcodes.CustomerInvalidState)
		}
		if !t.BranchPermitted(existing.BranchID) {
			return notFound("Customer")
		}
		if existing.IsArchived() {
			return conflict(errorcodes.CustomerArchived,
				"An archived customer cannot be modified.")
		}

		update := repositories.CustomerUpdate{
			BranchID: input.BranchID, LegalName: input.LegalName,
			DisplayName: input.DisplayName, RegistrationNumber: input.RegistrationNumber,
			TaxReference:          input.TaxReference,
			MarketplaceCustomerID: input.MarketplaceCustomerID,
			AccountOwnerUserID:    input.AccountOwnerUserID,
			DefaultCurrency:       input.DefaultCurrency,
			PaymentTermsReference: input.PaymentTermsReference,
			UpdatedBy:             t.OrganizationUserID,
		}
		// A change to the name, type or an identity reference moves the
		// duplicate key with it. Recomputing here keeps the organization-scoped
		// uniqueness index meaningful after an edit.
		if input.LegalName != nil || input.RegistrationNumber != nil ||
			input.TaxReference != nil {
			legalName := existing.LegalName
			if input.LegalName != nil {
				legalName = *input.LegalName
			}
			registration := existing.RegistrationNumber
			if input.RegistrationNumber != nil {
				registration = *input.RegistrationNumber
			}
			tax := existing.TaxReference
			if input.TaxReference != nil {
				tax = *input.TaxReference
			}
			normalized := models.NormalizeName(legalName)
			duplicateKey := models.DuplicateFingerprint(existing.CustomerType, normalized,
				registration, tax)
			update.NormalizedName = &normalized
			update.DuplicateFingerprint = &duplicateKey
		}

		updated, txErr = s.opts.Repository.UpdateCustomer(ctx, tx, t.OrganizationID,
			customerID, input.ExpectedVersion, update)
		if txErr != nil {
			if errors.Is(txErr, repositories.ErrDuplicate) {
				return conflict(errorcodes.CustomerAlreadyExists,
					"A customer with these identifying details already exists in this organization.")
			}
			return translate(txErr, "Customer", errorcodes.CustomerNotFound,
				errorcodes.CustomerInvalidState)
		}
		changed := changedCustomerFields(input)
		if txErr = s.opts.Repository.AppendCustomerHistory(ctx, tx, models.CustomerHistory{
			OrganizationID: t.OrganizationID, CustomerID: customerID,
			ChangeType: models.ChangeUpdated, PreviousStatus: existing.Status,
			NewStatus: updated.Status, ChangedFields: changed,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.customer.updated", "crm_customer", customerID,
			customerAuditValues(existing), customerAuditValues(updated), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventCustomerUpdated, "crm_customer", customerID,
			updated.Version, t.OrganizationID, input.CorrelationID,
			customerUpdatedEventPayload(updated, changed))
	})
	if err != nil {
		return models.Customer{}, err
	}
	return updated, nil
}

// changedCustomerFields names what a caller asked to change.
//
// Only names, never values: a history row that copied a tax reference would
// turn an audit trail into a second, less guarded, copy of the PII it exists to
// account for.
func changedCustomerFields(input UpdateCustomerInput) []string {
	changed := make([]string, 0, 9)
	for name, present := range map[string]bool{
		"branchId":              input.BranchID != nil,
		"legalName":             input.LegalName != nil,
		"displayName":           input.DisplayName != nil,
		"registrationNumber":    input.RegistrationNumber != nil,
		"taxReference":          input.TaxReference != nil,
		"marketplaceCustomerId": input.MarketplaceCustomerID != nil,
		"accountOwnerUserId":    input.AccountOwnerUserID != nil,
		"defaultCurrency":       input.DefaultCurrency != nil,
		"paymentTermsReference": input.PaymentTermsReference != nil,
	} {
		if present {
			changed = append(changed, name)
		}
	}
	sortStrings(changed)
	return changed
}

// ChangeCustomerStatusInput is a controlled customer status change.
type ChangeCustomerStatusInput struct {
	Status          string
	ReasonCode      string
	ExpectedVersion int64
	IdempotencyKey  string
	CorrelationID   string
	CausationID     string
}

// ChangeCustomerStatus moves a customer through the status matrix.
//
// There is no path here that assigns an arbitrary status. A change the matrix
// does not admit is refused, so a customer cannot be moved out of ARCHIVED or
// jumped straight from PROSPECT to SUSPENDED by a caller that constructed the
// request by hand.
func (s *Service) ChangeCustomerStatus(ctx context.Context, t tenant.Context,
	customerID string, input ChangeCustomerStatusInput) (models.Customer, error) {
	if err := s.writeEnabled(s.opts.Flags.CustomerWritesEnabled); err != nil {
		return models.Customer{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Customer{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Customer{}, err
	}
	if !validCustomerStatus(input.Status) {
		return models.Customer{}, badRequest(errorcodes.CustomerInvalidState,
			"status is not supported.")
	}

	var updated models.Customer
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.opts.Repository.LockCustomer(ctx, tx, t.OrganizationID, customerID)
		if txErr != nil {
			return translate(txErr, "Customer", errorcodes.CustomerNotFound,
				errorcodes.CustomerInvalidState)
		}
		if !t.BranchPermitted(existing.BranchID) {
			return notFound("Customer")
		}
		if existing.Status == input.Status {
			// Already there. A repeat of a status change that has landed is a
			// replay, not a conflict.
			updated = existing
			return nil
		}
		if !models.CanTransitionCustomer(existing.Status, input.Status) {
			return conflict(errorcodes.CustomerInvalidState,
				"That status change is not permitted from the customer's current status.")
		}

		archive := input.Status == models.CustomerStatusArchived
		updated, txErr = s.opts.Repository.SetCustomerStatus(ctx, tx, t.OrganizationID,
			customerID, input.ExpectedVersion, input.Status, t.OrganizationUserID, archive)
		if txErr != nil {
			return translate(txErr, "Customer", errorcodes.CustomerNotFound,
				errorcodes.CustomerInvalidState)
		}

		changeType := models.ChangeStatusChanged
		if archive {
			changeType = models.ChangeArchived
		}
		if txErr = s.opts.Repository.AppendCustomerHistory(ctx, tx, models.CustomerHistory{
			OrganizationID: t.OrganizationID, CustomerID: customerID, ChangeType: changeType,
			PreviousStatus: existing.Status, NewStatus: updated.Status,
			ReasonCode: input.ReasonCode, ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.customer.status_changed", "crm_customer",
			customerID, map[string]any{"status": existing.Status},
			map[string]any{"status": updated.Status}, input.ReasonCode); txErr != nil {
			return txErr
		}
		eventType := EventCustomerStatusChanged
		if archive {
			eventType = EventCustomerArchived
		}
		return s.publish(ctx, tx, eventType, "crm_customer", customerID, updated.Version,
			t.OrganizationID, input.CorrelationID,
			customerStatusEventPayload(updated, existing.Status, input.ReasonCode))
	})
	if err != nil {
		return models.Customer{}, err
	}
	if updated.Status == models.CustomerStatusArchived {
		s.count("crm_customers_archived_total", nil)
	}
	s.count("crm_customer_status_changes_total", map[string]string{"status": updated.Status})
	return updated, nil
}

// ArchiveCustomer closes a customer record.
//
// Archive rather than delete: quotations, contracts and their immutable
// versions reference this row, and a historical record pointing at a deleted
// customer is a record with a hole in it.
func (s *Service) ArchiveCustomer(ctx context.Context, t tenant.Context, customerID string,
	expectedVersion int64, reason, idempotencyKey, correlationID string) (models.Customer, error) {
	return s.ChangeCustomerStatus(ctx, t, customerID, ChangeCustomerStatusInput{
		Status: models.CustomerStatusArchived, ReasonCode: reason,
		ExpectedVersion: expectedVersion, IdempotencyKey: idempotencyKey,
		CorrelationID: correlationID,
	})
}

// ChangeCustomerRiskInput is a controlled customer risk change.
type ChangeCustomerRiskInput struct {
	RiskStatus      string
	ReasonCode      string
	ExpectedVersion int64
	IdempotencyKey  string
	CorrelationID   string
	CausationID     string
}

// ChangeCustomerRisk moves a customer's risk status.
//
// Escalation to RESTRICTED is applied to the credit profile in the same
// transaction. Leaving the two to disagree would mean a risk decision that
// never reached the gate that actually blocks contract activation.
//
// A reason is mandatory. A risk decision nobody has to justify is a risk
// decision nobody can review.
func (s *Service) ChangeCustomerRisk(ctx context.Context, t tenant.Context, customerID string,
	input ChangeCustomerRiskInput) (models.Customer, error) {
	if err := s.writeEnabled(s.opts.Flags.CustomerWritesEnabled); err != nil {
		return models.Customer{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Customer{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Customer{}, err
	}
	if !models.ValidRiskStatus(input.RiskStatus) {
		return models.Customer{}, badRequest(errorcodes.CustomerInvalidState,
			"riskStatus is not supported.")
	}
	if strings.TrimSpace(input.ReasonCode) == "" {
		return models.Customer{}, badRequest(errorcodes.CustomerInvalidState,
			"reasonCode is required for a risk status change.")
	}

	var updated models.Customer
	var approvalID string
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.opts.Repository.LockCustomer(ctx, tx, t.OrganizationID, customerID)
		if txErr != nil {
			return translate(txErr, "Customer", errorcodes.CustomerNotFound,
				errorcodes.CustomerInvalidState)
		}
		if !t.BranchPermitted(existing.BranchID) {
			return notFound("Customer")
		}
		if existing.IsArchived() {
			return conflict(errorcodes.CustomerArchived,
				"An archived customer's risk status cannot be changed.")
		}
		if existing.RiskStatus == input.RiskStatus {
			updated = existing
			return nil
		}

		// De-escalating away from RESTRICTED is the change worth reviewing: it
		// removes a block somebody deliberately applied. The approval framework
		// decides whether one is actually required.
		if existing.RiskStatus == models.RiskRestricted && s.opts.Approvals != nil {
			payload := map[string]any{
				"customerId":         customerID,
				"previousRiskStatus": existing.RiskStatus,
				"newRiskStatus":      input.RiskStatus,
				"reasonCode":         input.ReasonCode,
			}
			approvalID, txErr = s.opts.Approvals.Request(ctx, ApprovalRequestInput{
				OrganizationID: t.OrganizationID, OrganizationUserID: t.OrganizationUserID,
				BranchID: existing.BranchID, ResourceType: "crm_customer",
				ResourceID: customerID, Action: "risk_de_escalation",
				OperationType: "CRM_CUSTOMER_RISK_CHANGE", Payload: payload,
				ExpectedSubjectVersion: existing.Version,
				IdempotencyKey:         input.IdempotencyKey,
				CorrelationID:          input.CorrelationID, CausationID: input.CausationID,
			})
			if txErr != nil {
				return txErr
			}
		}

		updated, txErr = s.opts.Repository.SetCustomerRisk(ctx, tx, t.OrganizationID,
			customerID, input.ExpectedVersion, input.RiskStatus, t.OrganizationUserID)
		if txErr != nil {
			return translate(txErr, "Customer", errorcodes.CustomerNotFound,
				errorcodes.CustomerInvalidState)
		}
		if txErr = s.opts.Repository.SyncCreditRiskSnapshot(ctx, tx, t.OrganizationID,
			customerID, input.RiskStatus,
			input.RiskStatus == models.RiskRestricted); txErr != nil {
			return txErr
		}
		if txErr = s.opts.Repository.AppendCustomerHistory(ctx, tx, models.CustomerHistory{
			OrganizationID: t.OrganizationID, CustomerID: customerID,
			ChangeType: models.ChangeRiskChanged, PreviousRiskStatus: existing.RiskStatus,
			NewRiskStatus: updated.RiskStatus, ReasonCode: input.ReasonCode,
			ApprovalRequestID: approvalID, ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.customer.risk_changed", "crm_customer",
			customerID, map[string]any{"riskStatus": existing.RiskStatus},
			map[string]any{"riskStatus": updated.RiskStatus},
			input.ReasonCode); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventCustomerRiskChanged, "crm_customer", customerID,
			updated.Version, t.OrganizationID, input.CorrelationID,
			customerRiskEventPayload(updated, existing.RiskStatus))
	})
	if err != nil {
		return models.Customer{}, err
	}
	s.count("crm_customer_risk_changes_total",
		map[string]string{"risk_status": updated.RiskStatus})
	return updated, nil
}

// CustomerHistory returns a page of a customer's immutable change records.
func (s *Service) CustomerHistory(ctx context.Context, t tenant.Context, customerID string,
	page cursor.Cursor, hasCursor bool, limit int) ([]models.CustomerHistory, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireCustomer(ctx, nil, t, customerID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListCustomerHistory(ctx, t.OrganizationID, customerID, page,
		hasCursor, limit)
}

// customerAuditValues builds the audit projection.
//
// Names and statuses only. The registration number and tax reference are the
// customer's identity documents, and an audit record does not need to hold a
// second copy of them to be useful.
func customerAuditValues(customer models.Customer) map[string]any {
	return map[string]any{
		"customerType": customer.CustomerType,
		"displayName":  customer.DisplayName,
		"status":       customer.Status,
		"riskStatus":   customer.RiskStatus,
		"branchId":     customer.BranchID,
		"currency":     customer.DefaultCurrency,
	}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
