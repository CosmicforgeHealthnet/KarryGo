package usecases

import (
	"context"
	"errors"
	"fmt"
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

// CreateContractInput opens a draft contract.
type CreateContractInput struct {
	BranchID           string
	CustomerID         string
	QuotationID        string
	EffectiveFrom      time.Time
	EffectiveTo        *time.Time
	RenewalTerms       models.JSONMap
	ServiceScope       models.JSONMap
	CommercialSnapshot models.JSONMap
	TerminationTerms   models.JSONMap
	SupersedesID       string
	IdempotencyKey     string
	CorrelationID      string
	CausationID        string
}

// CreateContract opens a draft contract.
//
// Where a quotation is referenced it must be one this customer actually
// accepted, and the commercial snapshot is taken from the accepted immutable
// version rather than recomputed. Nothing here derives a price: no rate card,
// no marketplace fare, no commission and no tax.
func (s *Service) CreateContract(ctx context.Context, t tenant.Context,
	input CreateContractInput) (models.Contract, error) {
	if err := s.writeEnabled(s.opts.Flags.ContractsEnabled); err != nil {
		return models.Contract{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Contract{}, err
	}
	if input.EffectiveTo != nil && !input.EffectiveTo.After(input.EffectiveFrom) {
		return models.Contract{}, badRequest(errorcodes.ContractInvalidState,
			"effectiveTo must be after effectiveFrom.")
	}
	if input.BranchID != "" && !t.BranchPermitted(input.BranchID) {
		return models.Contract{}, badRequest(errorcodes.ContractInvalidState,
			"branchId is not within your authorized scope.")
	}

	var created models.Contract
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		customer, txErr := s.requireCustomer(ctx, tx, t, input.CustomerID)
		if txErr != nil {
			return txErr
		}
		if customer.IsArchived() {
			return conflict(errorcodes.CustomerArchived,
				"An archived customer cannot have new contracts.")
		}

		commercial := input.CommercialSnapshot
		if input.QuotationID != "" {
			quotation, quotationErr := s.opts.Repository.GetQuotation(ctx, tx,
				t.OrganizationID, input.QuotationID)
			if quotationErr != nil || quotation.CustomerID != input.CustomerID {
				return errorcodes.NotFound(errorcodes.QuotationNotFound,
					"Quotation was not found.")
			}
			if quotation.Status != models.QuotationAccepted ||
				quotation.AcceptedVersionID == "" {
				return conflict(errorcodes.QuotationInvalidState,
					"Only an accepted quotation can back a contract.")
			}
			version, versionErr := s.opts.Repository.GetQuotationVersion(ctx, tx,
				t.OrganizationID, input.QuotationID, quotation.AcceptedVersionID)
			if versionErr != nil {
				return translate(versionErr, "Quotation version",
					errorcodes.QuotationNotFound, errorcodes.QuotationVersionConflict)
			}
			if len(commercial) == 0 {
				// Sourced from the immutable accepted version, not recomputed.
				// The customer agreed to those numbers; recalculating them would
				// be a different agreement wearing the same reference.
				commercial = models.JSONMap{
					"source":                 "ACCEPTED_QUOTATION_VERSION",
					"quotationId":            input.QuotationID,
					"acceptedVersionId":      version.ID,
					"currency":               version.Currency,
					"totalAmountMinor":       version.TotalAmountMinor,
					"quotationContentDigest": version.ContentFingerprint,
				}
			}
		}

		number, txErr := s.opts.Repository.NextContractNumber(ctx, tx, t.OrganizationID,
			s.now())
		if txErr != nil {
			return txErr
		}
		created, txErr = s.opts.Repository.CreateContract(ctx, tx, models.Contract{
			ID:                 ids.New(repositories.PrefixContract),
			OrganizationID:     t.OrganizationID,
			BranchID:           input.BranchID,
			CustomerID:         input.CustomerID,
			QuotationID:        input.QuotationID,
			ContractNumber:     number,
			EffectiveFrom:      input.EffectiveFrom,
			EffectiveTo:        input.EffectiveTo,
			RenewalTerms:       input.RenewalTerms,
			ServiceScope:       input.ServiceScope,
			CommercialSnapshot: commercial,
			TerminationTerms:   input.TerminationTerms,
			Status:             models.ContractDraft,
			SupersedesID:       input.SupersedesID,
			CreatedBy:          t.OrganizationUserID,
		})
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if txErr = s.opts.Repository.AppendContractHistory(ctx, tx, models.ContractHistory{
			OrganizationID: t.OrganizationID, ContractID: created.ID, PreviousStatus: "",
			NewStatus: created.Status, ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.contract.created", "contract", created.ID, nil,
			contractAuditValues(created), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventContractCreated, "contract", created.ID,
			created.Version, t.OrganizationID, input.CorrelationID,
			contractEventPayload(created))
	})
	if err != nil {
		return models.Contract{}, err
	}
	s.count("contracts_created_total", nil)
	return created, nil
}

// GetContract reads one contract.
func (s *Service) GetContract(ctx context.Context, t tenant.Context,
	contractID string) (models.Contract, error) {
	if err := s.readEnabled(); err != nil {
		return models.Contract{}, err
	}
	return s.requireContract(ctx, nil, t, contractID)
}

func (s *Service) requireContract(ctx context.Context, q pgx.Tx, t tenant.Context,
	contractID string) (models.Contract, error) {
	var contract models.Contract
	var err error
	if q == nil {
		contract, err = s.opts.Repository.GetContract(ctx, nil, t.OrganizationID, contractID)
	} else {
		contract, err = s.opts.Repository.GetContract(ctx, q, t.OrganizationID, contractID)
	}
	if err != nil {
		return contract, translate(err, "Contract", errorcodes.ContractNotFound,
			errorcodes.ContractVersionConflict)
	}
	if !t.BranchPermitted(contract.BranchID) {
		return contract, notFound("Contract")
	}
	return contract, nil
}

// ListContracts returns a tenant-scoped page.
func (s *Service) ListContracts(ctx context.Context, t tenant.Context,
	filter repositories.ContractFilter) ([]models.Contract, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListContracts(ctx, t.OrganizationID, t.PermittedBranchIDs, filter)
}

// UpdateContractInput carries a partial draft contract update.
type UpdateContractInput struct {
	EffectiveFrom      *time.Time
	EffectiveTo        *time.Time
	RenewalTerms       models.JSONMap
	ServiceScope       models.JSONMap
	CommercialSnapshot models.JSONMap
	TerminationTerms   models.JSONMap
	QuotationID        *string
	ExpectedVersion    int64
	CorrelationID      string
	CausationID        string
}

// UpdateContract edits a draft contract.
func (s *Service) UpdateContract(ctx context.Context, t tenant.Context, contractID string,
	input UpdateContractInput) (models.Contract, error) {
	if err := s.writeEnabled(s.opts.Flags.ContractsEnabled); err != nil {
		return models.Contract{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Contract{}, err
	}

	var updated models.Contract
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.opts.Repository.LockContract(ctx, tx, t.OrganizationID, contractID)
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if !t.BranchPermitted(existing.BranchID) {
			return notFound("Contract")
		}
		if existing.Status != models.ContractDraft {
			return conflict(errorcodes.ContractInvalidState,
				"Only a draft contract can be edited.")
		}
		effectiveFrom := existing.EffectiveFrom
		if input.EffectiveFrom != nil {
			effectiveFrom = *input.EffectiveFrom
		}
		effectiveTo := existing.EffectiveTo
		if input.EffectiveTo != nil {
			effectiveTo = input.EffectiveTo
		}
		if effectiveTo != nil && !effectiveTo.After(effectiveFrom) {
			return badRequest(errorcodes.ContractInvalidState,
				"effectiveTo must be after effectiveFrom.")
		}

		updated, txErr = s.opts.Repository.UpdateContract(ctx, tx, t.OrganizationID,
			contractID, input.ExpectedVersion, repositories.ContractUpdate{
				EffectiveFrom: input.EffectiveFrom, EffectiveTo: input.EffectiveTo,
				RenewalTerms: input.RenewalTerms, ServiceScope: input.ServiceScope,
				CommercialSnapshot: input.CommercialSnapshot,
				TerminationTerms:   input.TerminationTerms, QuotationID: input.QuotationID,
				UpdatedBy: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if txErr = s.record(ctx, tx, t, "crm.contract.updated", "contract", contractID,
			contractAuditValues(existing), contractAuditValues(updated), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventContractCreated, "contract", contractID,
			updated.Version, t.OrganizationID, input.CorrelationID,
			contractEventPayload(updated))
	})
	if err != nil {
		return models.Contract{}, err
	}
	return updated, nil
}

// SubmitContract routes a draft contract through the WP9 approval policy.
func (s *Service) SubmitContract(ctx context.Context, t tenant.Context, contractID string,
	expectedVersion int64, idempotencyKey, correlationID,
	causationID string) (models.Contract, error) {
	if err := s.writeEnabled(s.opts.Flags.ContractsEnabled); err != nil {
		return models.Contract{}, err
	}
	if err := requireExpectedVersion(expectedVersion); err != nil {
		return models.Contract{}, err
	}
	if err := requireIdempotencyKey(idempotencyKey); err != nil {
		return models.Contract{}, err
	}

	var updated models.Contract
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		contract, txErr := s.opts.Repository.LockContract(ctx, tx, t.OrganizationID, contractID)
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if !t.BranchPermitted(contract.BranchID) {
			return notFound("Contract")
		}
		if contract.Status == models.ContractAwaitingApproval ||
			contract.Status == models.ContractApproved {
			updated = contract
			return nil
		}
		if contract.Status != models.ContractDraft {
			return conflict(errorcodes.ContractInvalidState,
				"Only a draft contract can be submitted for approval.")
		}

		customer, txErr := s.requireCustomer(ctx, tx, t, contract.CustomerID)
		if txErr != nil {
			return txErr
		}
		if txErr = requireCommittableCustomer(customer); txErr != nil {
			return txErr
		}
		if txErr = s.requireNoBlockingCreditHold(ctx, tx, t.OrganizationID,
			contract.CustomerID); txErr != nil {
			return txErr
		}
		if contract.EffectiveTo != nil &&
			!contract.EffectiveTo.After(contract.EffectiveFrom) {
			return badRequest(errorcodes.ContractInvalidState,
				"effectiveTo must be after effectiveFrom.")
		}
		if len(contract.CommercialSnapshot) == 0 {
			return badRequest(errorcodes.ContractInvalidState,
				"A contract needs a commercial snapshot before it is submitted.")
		}
		if len(contract.TerminationTerms) == 0 {
			return badRequest(errorcodes.ContractInvalidState,
				"A contract needs termination terms before it is submitted.")
		}

		contentFingerprint := contractFingerprint(contract)
		nextStatus := models.ContractApproved
		approvalID := ""
		if s.opts.Approvals != nil {
			approvalID, txErr = s.opts.Approvals.Request(ctx, ApprovalRequestInput{
				OrganizationID: t.OrganizationID, OrganizationUserID: t.OrganizationUserID,
				BranchID: contract.BranchID, ResourceType: "contract",
				ResourceID: contractID, Action: "activate",
				OperationType: "CRM_CONTRACT_ACTIVATION",
				Payload: map[string]any{
					"contractId":         contractID,
					"contractNumber":     contract.ContractNumber,
					"customerId":         contract.CustomerID,
					"effectiveFrom":      contract.EffectiveFrom.UTC().Format(time.RFC3339),
					"contentFingerprint": contentFingerprint,
				},
				ExpectedSubjectVersion: contract.Version,
				IdempotencyKey:         idempotencyKey, CorrelationID: correlationID,
				CausationID: causationID,
			})
			if txErr != nil {
				return txErr
			}
			if approvalID != "" {
				nextStatus = models.ContractAwaitingApproval
			}
		}

		updated, txErr = s.opts.Repository.TransitionContract(ctx, tx, t.OrganizationID,
			contractID, expectedVersion, repositories.ContractTransition{
				FromStatus: models.ContractDraft, ToStatus: nextStatus,
				ApprovalRequestID: approvalID, Actor: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if txErr = s.opts.Repository.AppendContractHistory(ctx, tx, models.ContractHistory{
			OrganizationID: t.OrganizationID, ContractID: contractID,
			PreviousStatus: contract.Status, NewStatus: updated.Status,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: correlationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.contract.submitted", "contract", contractID,
			map[string]any{"status": contract.Status},
			map[string]any{"status": updated.Status, "approvalRequestId": approvalID},
			""); txErr != nil {
			return txErr
		}
		eventType := EventContractCreated
		if updated.Status == models.ContractApproved {
			eventType = EventContractApproved
		}
		return s.publish(ctx, tx, eventType, "contract", contractID, updated.Version,
			t.OrganizationID, correlationID,
			contractStatusEventPayload(updated, contract.Status, ""))
	})
	if err != nil {
		return models.Contract{}, err
	}
	return updated, nil
}

// contractFingerprint hashes everything that defines the agreement.
func contractFingerprint(contract models.Contract) string {
	parts := []string{contract.ID, contract.CustomerID, contract.ContractNumber,
		contract.EffectiveFrom.UTC().Format(time.RFC3339Nano)}
	if contract.EffectiveTo != nil {
		parts = append(parts, contract.EffectiveTo.UTC().Format(time.RFC3339Nano))
	}
	for _, section := range []models.JSONMap{contract.ServiceScope,
		contract.CommercialSnapshot, contract.TerminationTerms, contract.RenewalTerms} {
		if encoded, err := models.Fingerprint(section); err == nil {
			parts = append(parts, encoded)
		}
	}
	return fingerprint(parts...)
}

// ActivateContractInput brings an approved contract into force.
type ActivateContractInput struct {
	SignatoryReferences []string
	ExpectedVersion     int64
	IdempotencyKey      string
	CorrelationID       string
	CausationID         string
}

// ActivateContract freezes an approved contract into an immutable version and
// brings it into force.
//
// Every dependency is revalidated at activation rather than trusted from
// approval time: the customer may have been suspended, their risk may have been
// escalated, a credit hold may have been applied, and a signed document may
// have been revoked, all since the reviewer said yes. An approval is permission
// to activate a specific agreement, not a promise that the world stood still.
func (s *Service) ActivateContract(ctx context.Context, t tenant.Context, contractID string,
	input ActivateContractInput) (models.Contract, models.ContractVersion, error) {
	if err := s.writeEnabled(s.opts.Flags.ContractActivation); err != nil {
		return models.Contract{}, models.ContractVersion{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Contract{}, models.ContractVersion{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Contract{}, models.ContractVersion{}, err
	}

	preflight, err := s.requireContract(ctx, nil, t, contractID)
	if err != nil {
		return models.Contract{}, models.ContractVersion{}, err
	}
	if preflight.Status == models.ContractActive {
		version, versionErr := s.opts.Repository.GetContractVersion(ctx, nil, t.OrganizationID,
			contractID, preflight.ActiveVersionID)
		if versionErr != nil {
			return models.Contract{}, models.ContractVersion{}, translate(versionErr,
				"Contract version", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		return preflight, version, nil
	}
	// Re-read Media outside the transaction so a document revoked since approval
	// blocks the activation.
	if err := s.refreshContractDocuments(ctx, t, contractID); err != nil {
		return models.Contract{}, models.ContractVersion{}, err
	}

	var activated models.Contract
	var activeVersion models.ContractVersion
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		contract, txErr := s.opts.Repository.LockContract(ctx, tx, t.OrganizationID, contractID)
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if !t.BranchPermitted(contract.BranchID) {
			return notFound("Contract")
		}
		if contract.Status != models.ContractApproved {
			return conflict(errorcodes.ContractApprovalRequired,
				"Only an approved contract can be activated.")
		}

		customer, txErr := s.requireCustomer(ctx, tx, t, contract.CustomerID)
		if txErr != nil {
			return txErr
		}
		if customer.Status != models.CustomerStatusActive {
			return conflict(errorcodes.ContractActivationBlocked,
				"The customer must be active before a contract can be activated.")
		}
		if customer.RiskStatus == models.RiskRestricted {
			return conflict(errorcodes.CustomerRiskRestricted,
				"A restricted customer cannot have a contract activated.")
		}
		if txErr = s.requireNoBlockingCreditHold(ctx, tx, t.OrganizationID,
			contract.CustomerID); txErr != nil {
			return txErr
		}
		if txErr = s.opts.Repository.RequireMandatoryContractDocumentsReady(ctx, tx,
			t.OrganizationID, contractID); txErr != nil {
			if errors.Is(txErr, repositories.ErrDocumentNotReady) {
				return conflict(errorcodes.ContractDocumentNotReady,
					"A required contract document is not confirmed safe and ready.")
			}
			return txErr
		}

		contentFingerprint := contractFingerprint(contract)
		if contract.ApprovalRequestID != "" && s.opts.Approvals != nil {
			if txErr = s.opts.Approvals.RequireApproved(ctx, t.OrganizationID,
				contract.ApprovalRequestID, map[string]any{
					"contractId":         contractID,
					"contractNumber":     contract.ContractNumber,
					"customerId":         contract.CustomerID,
					"effectiveFrom":      contract.EffectiveFrom.UTC().Format(time.RFC3339),
					"contentFingerprint": contentFingerprint,
				}); txErr != nil {
				return txErr
			}
		}

		documents, txErr := s.opts.Repository.ListContractDocuments(ctx, tx, t.OrganizationID,
			contractID)
		if txErr != nil {
			return txErr
		}
		documentIDs := make([]string, 0, len(documents))
		for _, document := range documents {
			documentIDs = append(documentIDs, document.ID)
		}

		activeVersion, txErr = s.opts.Repository.CreateContractVersion(ctx, tx,
			models.ContractVersion{
				ID:                   ids.New(repositories.PrefixContractVersion),
				OrganizationID:       t.OrganizationID,
				ContractID:           contractID,
				VersionNumber:        contract.CurrentVersion,
				CustomerSnapshot:     customerSnapshot(customer),
				ServiceScopeSnapshot: contract.ServiceScope,
				CommercialSnapshot:   contract.CommercialSnapshot,
				EffectiveFrom:        contract.EffectiveFrom,
				EffectiveTo:          contract.EffectiveTo,
				RenewalTerms:         contract.RenewalTerms,
				TerminationTerms:     contract.TerminationTerms,
				DocumentReferences:   documentIDs,
				ApprovalRequestID:    contract.ApprovalRequestID,
				SignatoryReferences:  input.SignatoryReferences,
				ContentFingerprint:   contentFingerprint,
				ActivatedBy:          t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Contract version", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if txErr = s.opts.Repository.BindContractDocumentsToVersion(ctx, tx, t.OrganizationID,
			contractID, activeVersion.ID); txErr != nil {
			return txErr
		}

		activated, txErr = s.opts.Repository.TransitionContract(ctx, tx, t.OrganizationID,
			contractID, input.ExpectedVersion, repositories.ContractTransition{
				FromStatus: models.ContractApproved, ToStatus: models.ContractActive,
				ActiveVersionID: activeVersion.ID, MarkActivated: true,
				Actor: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if txErr = s.opts.Repository.AppendContractHistory(ctx, tx, models.ContractHistory{
			OrganizationID: t.OrganizationID, ContractID: contractID,
			PreviousStatus: contract.Status, NewStatus: activated.Status,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.contract.activated", "contract", contractID,
			map[string]any{"status": contract.Status},
			map[string]any{"status": activated.Status, "activeVersionId": activeVersion.ID},
			""); txErr != nil {
			return txErr
		}

		primary := s.primaryContactFor(ctx, tx, t.OrganizationID, contract.CustomerID,
			models.PurposeLegal)
		if txErr = s.enqueueNotification(ctx, tx, t.OrganizationID, "contract", contractID,
			models.IntentContractActivated, primary.ID, notificationChannelFor(primary),
			models.JSONMap{"contractNumber": activated.ContractNumber},
			"contract_activated:"+activeVersion.ID, input.CorrelationID,
			input.CausationID); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventContractActivated, "contract", contractID,
			activated.Version, t.OrganizationID, input.CorrelationID,
			contractStatusEventPayload(activated, contract.Status, ""))
	})
	if err != nil {
		return models.Contract{}, models.ContractVersion{}, err
	}
	s.count("contracts_activated_total", nil)
	return activated, activeVersion, nil
}

// ContractLifecycleInput is a controlled contract status change.
type ContractLifecycleInput struct {
	ReasonCode                string
	EffectiveAt               *time.Time
	EvidenceDocumentReference string
	ExpectedVersion           int64
	IdempotencyKey            string
	CorrelationID             string
	CausationID               string
}

// SuspendContract pauses an active contract.
//
// Suspension preserves the active version untouched. It calculates no penalty,
// creates no refund and makes no provider deduction: pausing an agreement is an
// operational decision, and turning it into a financial one would be this
// service inventing money movements it has no authority over.
func (s *Service) SuspendContract(ctx context.Context, t tenant.Context, contractID string,
	input ContractLifecycleInput) (models.Contract, error) {
	if strings.TrimSpace(input.ReasonCode) == "" {
		return models.Contract{}, badRequest(errorcodes.ContractInvalidState,
			"reasonCode is required to suspend a contract.")
	}
	return s.transitionContract(ctx, t, contractID, models.ContractActive,
		models.ContractSuspended, EventContractSuspended, "crm.contract.suspended",
		models.IntentContractSuspended, s.opts.Flags.ContractsEnabled, input)
}

// ResumeContract returns a suspended contract to active.
func (s *Service) ResumeContract(ctx context.Context, t tenant.Context, contractID string,
	input ContractLifecycleInput) (models.Contract, error) {
	return s.transitionContract(ctx, t, contractID, models.ContractSuspended,
		models.ContractActive, EventContractActivated, "crm.contract.resumed", "",
		s.opts.Flags.ContractActivation, input)
}

// TerminateContract ends a contract.
//
// Termination preserves every version and every document. It calculates no
// termination fee, creates no refund and posts nothing to a wallet: what a
// party owes on early termination is a commercial question settled elsewhere,
// and a number invented here would be a number somebody acted on.
func (s *Service) TerminateContract(ctx context.Context, t tenant.Context, contractID string,
	input ContractLifecycleInput) (models.Contract, error) {
	if err := s.writeEnabled(s.opts.Flags.ContractsEnabled); err != nil {
		return models.Contract{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Contract{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Contract{}, err
	}
	if strings.TrimSpace(input.ReasonCode) == "" {
		return models.Contract{}, badRequest(errorcodes.ContractInvalidState,
			"reasonCode is required to terminate a contract.")
	}

	var terminated models.Contract
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		contract, txErr := s.opts.Repository.LockContract(ctx, tx, t.OrganizationID, contractID)
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if !t.BranchPermitted(contract.BranchID) {
			return notFound("Contract")
		}
		if contract.Status == models.ContractTerminated {
			terminated = contract
			return nil
		}
		if models.TerminalContract(contract.Status) {
			return conflict(errorcodes.ContractInvalidState,
				"This contract has already reached a final state.")
		}
		if !models.CanTransitionContract(contract.Status, models.ContractTerminated) {
			return conflict(errorcodes.ContractInvalidState,
				"A contract in this state cannot be terminated.")
		}

		approvalID := ""
		if s.opts.Approvals != nil {
			approvalID, txErr = s.opts.Approvals.Request(ctx, ApprovalRequestInput{
				OrganizationID: t.OrganizationID, OrganizationUserID: t.OrganizationUserID,
				BranchID: contract.BranchID, ResourceType: "contract",
				ResourceID: contractID, Action: "terminate",
				OperationType: "CRM_CONTRACT_TERMINATION",
				Payload: map[string]any{
					"contractId":     contractID,
					"contractNumber": contract.ContractNumber,
					"reasonCode":     input.ReasonCode,
				},
				ExpectedSubjectVersion: contract.Version,
				IdempotencyKey:         input.IdempotencyKey,
				CorrelationID:          input.CorrelationID, CausationID: input.CausationID,
			})
			if txErr != nil {
				return txErr
			}
			if approvalID != "" {
				if txErr = s.opts.Approvals.RequireApproved(ctx, t.OrganizationID, approvalID,
					map[string]any{
						"contractId":     contractID,
						"contractNumber": contract.ContractNumber,
						"reasonCode":     input.ReasonCode,
					}); txErr != nil {
					return txErr
				}
			}
		}

		terminated, txErr = s.opts.Repository.TransitionContract(ctx, tx, t.OrganizationID,
			contractID, input.ExpectedVersion, repositories.ContractTransition{
				FromStatus: contract.Status, ToStatus: models.ContractTerminated,
				ApprovalRequestID: approvalID, EffectiveTo: input.EffectiveAt,
				MarkTerminated: true, Actor: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if txErr = s.opts.Repository.AppendContractHistory(ctx, tx, models.ContractHistory{
			OrganizationID: t.OrganizationID, ContractID: contractID,
			PreviousStatus: contract.Status, NewStatus: terminated.Status,
			ReasonCode: input.ReasonCode, ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.contract.terminated", "contract", contractID,
			map[string]any{"status": contract.Status},
			map[string]any{"status": terminated.Status}, input.ReasonCode); txErr != nil {
			return txErr
		}
		primary := s.primaryContactFor(ctx, tx, t.OrganizationID, contract.CustomerID,
			models.PurposeLegal)
		if txErr = s.enqueueNotification(ctx, tx, t.OrganizationID, "contract", contractID,
			models.IntentContractTerminated, primary.ID, notificationChannelFor(primary),
			models.JSONMap{"contractNumber": terminated.ContractNumber},
			"contract_terminated:"+contractID, input.CorrelationID,
			input.CausationID); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventContractTerminated, "contract", contractID,
			terminated.Version, t.OrganizationID, input.CorrelationID,
			contractStatusEventPayload(terminated, contract.Status, input.ReasonCode))
	})
	if err != nil {
		return models.Contract{}, err
	}
	s.count("contracts_terminated_total", nil)
	return terminated, nil
}

// transitionContract performs a guarded contract status change.
func (s *Service) transitionContract(ctx context.Context, t tenant.Context, contractID,
	fromStatus, toStatus, eventType, auditAction, notificationIntent string, flag bool,
	input ContractLifecycleInput) (models.Contract, error) {
	if err := s.writeEnabled(flag); err != nil {
		return models.Contract{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Contract{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Contract{}, err
	}

	var updated models.Contract
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		contract, txErr := s.opts.Repository.LockContract(ctx, tx, t.OrganizationID, contractID)
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if !t.BranchPermitted(contract.BranchID) {
			return notFound("Contract")
		}
		if contract.Status == toStatus {
			updated = contract
			return nil
		}
		if contract.Status != fromStatus {
			return conflict(errorcodes.ContractInvalidState,
				"That change is not permitted from the contract's current status.")
		}
		if !models.CanTransitionContract(fromStatus, toStatus) {
			return conflict(errorcodes.ContractInvalidState,
				"That change is not permitted from the contract's current status.")
		}
		// Resuming a suspended contract is a fresh commitment: the reasons it
		// might now be blocked — a hold, a restriction — are exactly the reasons
		// activation checks.
		if toStatus == models.ContractActive {
			customer, customerErr := s.requireCustomer(ctx, tx, t, contract.CustomerID)
			if customerErr != nil {
				return customerErr
			}
			if customer.RiskStatus == models.RiskRestricted {
				return conflict(errorcodes.CustomerRiskRestricted,
					"A restricted customer's contract cannot be resumed.")
			}
			if txErr = s.requireNoBlockingCreditHold(ctx, tx, t.OrganizationID,
				contract.CustomerID); txErr != nil {
				return txErr
			}
		}

		updated, txErr = s.opts.Repository.TransitionContract(ctx, tx, t.OrganizationID,
			contractID, input.ExpectedVersion, repositories.ContractTransition{
				FromStatus: fromStatus, ToStatus: toStatus, Actor: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if txErr = s.opts.Repository.AppendContractHistory(ctx, tx, models.ContractHistory{
			OrganizationID: t.OrganizationID, ContractID: contractID,
			PreviousStatus: contract.Status, NewStatus: updated.Status,
			ReasonCode: input.ReasonCode, ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, auditAction, "contract", contractID,
			map[string]any{"status": contract.Status},
			map[string]any{"status": updated.Status}, input.ReasonCode); txErr != nil {
			return txErr
		}
		if notificationIntent != "" {
			primary := s.primaryContactFor(ctx, tx, t.OrganizationID, contract.CustomerID,
				models.PurposeLegal)
			if txErr = s.enqueueNotification(ctx, tx, t.OrganizationID, "contract", contractID,
				notificationIntent, primary.ID, notificationChannelFor(primary),
				models.JSONMap{"contractNumber": updated.ContractNumber},
				fmt.Sprintf("contract_%s:%s:%d", strings.ToLower(toStatus), contractID,
					updated.Version), input.CorrelationID, input.CausationID); txErr != nil {
				return txErr
			}
		}
		return s.publish(ctx, tx, eventType, "contract", contractID, updated.Version,
			t.OrganizationID, input.CorrelationID,
			contractStatusEventPayload(updated, contract.Status, input.ReasonCode))
	})
	if err != nil {
		return models.Contract{}, err
	}
	return updated, nil
}

// RenewContractInput opens a renewal.
type RenewContractInput struct {
	EffectiveFrom      time.Time
	EffectiveTo        *time.Time
	RenewalTerms       models.JSONMap
	CommercialSnapshot models.JSONMap
	ExpectedVersion    int64
	IdempotencyKey     string
	CorrelationID      string
	CausationID        string
}

// RenewContract opens a replacement contract for a continuing relationship.
//
// The existing contract and its activated version are left untouched: the
// renewal is a new draft contract that supersedes the old one once it is
// approved and activated in its own right. A renewal never activates itself,
// because a renewal is a new commitment and new commitments go through
// approval.
func (s *Service) RenewContract(ctx context.Context, t tenant.Context, contractID string,
	input RenewContractInput) (models.Contract, error) {
	if err := s.writeEnabled(s.opts.Flags.ContractsEnabled); err != nil {
		return models.Contract{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Contract{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Contract{}, err
	}
	if input.EffectiveTo != nil && !input.EffectiveTo.After(input.EffectiveFrom) {
		return models.Contract{}, badRequest(errorcodes.ContractInvalidState,
			"effectiveTo must be after effectiveFrom.")
	}

	var renewal models.Contract
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		contract, txErr := s.opts.Repository.LockContract(ctx, tx, t.OrganizationID, contractID)
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}
		if !t.BranchPermitted(contract.BranchID) {
			return notFound("Contract")
		}
		switch contract.Status {
		case models.ContractActive, models.ContractSuspended, models.ContractRenewalPending:
		default:
			return conflict(errorcodes.ContractInvalidState,
				"Only an active, suspended or renewal-pending contract can be renewed.")
		}

		customer, txErr := s.requireCustomer(ctx, tx, t, contract.CustomerID)
		if txErr != nil {
			return txErr
		}
		if txErr = requireCommittableCustomer(customer); txErr != nil {
			return txErr
		}
		if txErr = s.requireNoBlockingCreditHold(ctx, tx, t.OrganizationID,
			contract.CustomerID); txErr != nil {
			return txErr
		}

		commercial := input.CommercialSnapshot
		if len(commercial) == 0 {
			commercial = contract.CommercialSnapshot
		}
		renewalTerms := input.RenewalTerms
		if len(renewalTerms) == 0 {
			renewalTerms = contract.RenewalTerms
		}
		number, txErr := s.opts.Repository.NextContractNumber(ctx, tx, t.OrganizationID,
			s.now())
		if txErr != nil {
			return txErr
		}
		renewal, txErr = s.opts.Repository.CreateContract(ctx, tx, models.Contract{
			ID:                 ids.New(repositories.PrefixContract),
			OrganizationID:     t.OrganizationID,
			BranchID:           contract.BranchID,
			CustomerID:         contract.CustomerID,
			QuotationID:        contract.QuotationID,
			ContractNumber:     number,
			EffectiveFrom:      input.EffectiveFrom,
			EffectiveTo:        input.EffectiveTo,
			RenewalTerms:       renewalTerms,
			ServiceScope:       contract.ServiceScope,
			CommercialSnapshot: commercial,
			TerminationTerms:   contract.TerminationTerms,
			Status:             models.ContractDraft,
			SupersedesID:       contractID,
			CreatedBy:          t.OrganizationUserID,
		})
		if txErr != nil {
			return translate(txErr, "Contract", errorcodes.ContractNotFound,
				errorcodes.ContractVersionConflict)
		}

		// The original moves to RENEWAL_PENDING and keeps its activated version
		// exactly as it is. Nothing about the agreement currently in force
		// changes because a successor is being drafted.
		if contract.Status == models.ContractActive ||
			contract.Status == models.ContractSuspended {
			if _, txErr = s.opts.Repository.TransitionContract(ctx, tx, t.OrganizationID,
				contractID, input.ExpectedVersion, repositories.ContractTransition{
					FromStatus: contract.Status, ToStatus: models.ContractRenewalPending,
					Actor: t.OrganizationUserID,
				}); txErr != nil {
				return translate(txErr, "Contract", errorcodes.ContractNotFound,
					errorcodes.ContractVersionConflict)
			}
			if txErr = s.opts.Repository.AppendContractHistory(ctx, tx,
				models.ContractHistory{
					OrganizationID: t.OrganizationID, ContractID: contractID,
					PreviousStatus: contract.Status,
					NewStatus:      models.ContractRenewalPending,
					ReasonCode:     "renewal_opened", ActorType: string(t.ActorType),
					ActorID: t.ActorID, CorrelationID: input.CorrelationID,
				}); txErr != nil {
				return txErr
			}
		}

		if txErr = s.opts.Repository.AppendContractHistory(ctx, tx, models.ContractHistory{
			OrganizationID: t.OrganizationID, ContractID: renewal.ID, PreviousStatus: "",
			NewStatus: renewal.Status, ReasonCode: "renewal_of:" + contractID,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.contract.renewed", "contract", contractID, nil,
			map[string]any{"renewalContractId": renewal.ID}, ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventContractRenewalPending, "contract", contractID,
			contract.Version+1, t.OrganizationID, input.CorrelationID,
			contractStatusEventPayload(renewal, contract.Status, "renewal_opened"))
	})
	if err != nil {
		return models.Contract{}, err
	}
	return renewal, nil
}

// ContractVersions returns every immutable version of a contract.
func (s *Service) ContractVersions(ctx context.Context, t tenant.Context, contractID string,
	limit int) ([]models.ContractVersion, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireContract(ctx, nil, t, contractID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListContractVersions(ctx, t.OrganizationID, contractID, limit)
}

// ContractVersion returns one immutable contract version.
func (s *Service) ContractVersion(ctx context.Context, t tenant.Context,
	contractID, versionID string) (models.ContractVersion, error) {
	if err := s.readEnabled(); err != nil {
		return models.ContractVersion{}, err
	}
	if _, err := s.requireContract(ctx, nil, t, contractID); err != nil {
		return models.ContractVersion{}, err
	}
	version, err := s.opts.Repository.GetContractVersion(ctx, nil, t.OrganizationID, contractID,
		versionID)
	return version, translate(err, "Contract version", errorcodes.ContractNotFound,
		errorcodes.ContractVersionConflict)
}

// ContractHistory returns a page of a contract's status changes.
func (s *Service) ContractHistory(ctx context.Context, t tenant.Context, contractID string,
	page cursor.Cursor, hasCursor bool, limit int) ([]models.ContractHistory, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireContract(ctx, nil, t, contractID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListContractHistory(ctx, t.OrganizationID, contractID, page,
		hasCursor, limit)
}

// contractAuditValues builds the audit projection.
//
// The commercial snapshot is deliberately absent: it is the negotiated
// substance of the agreement, and an audit trail records that a contract moved,
// not what it was worth.
func contractAuditValues(contract models.Contract) map[string]any {
	return map[string]any{
		"contractNumber": contract.ContractNumber,
		"customerId":     contract.CustomerID,
		"status":         contract.Status,
		"currentVersion": contract.CurrentVersion,
		"effectiveFrom":  contract.EffectiveFrom.UTC().Format(time.RFC3339Nano),
	}
}
