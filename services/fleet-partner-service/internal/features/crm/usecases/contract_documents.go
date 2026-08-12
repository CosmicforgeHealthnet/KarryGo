package usecases

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// AttachContractDocumentInput links a Media object to a contract.
type AttachContractDocumentInput struct {
	MediaObjectID   string
	DocumentPurpose string
	IsMandatory     bool
	CorrelationID   string
}

// AttachContractDocument records a Media reference against a contract.
func (s *Service) AttachContractDocument(ctx context.Context, t tenant.Context,
	contractID string, input AttachContractDocumentInput) (models.DocumentReference, error) {
	if err := s.writeEnabled(s.opts.Flags.ContractsEnabled); err != nil {
		return models.DocumentReference{}, err
	}
	if strings.TrimSpace(input.MediaObjectID) == "" {
		return models.DocumentReference{}, badRequest(errorcodes.ContractDocumentNotReady,
			"mediaObjectId is required.")
	}
	switch input.DocumentPurpose {
	case models.DocPurposeDraftContract, models.DocPurposeSignedContract,
		models.DocPurposeSupporting, models.DocPurposeTerminationEvidence,
		models.DocPurposeRenewal:
	default:
		return models.DocumentReference{}, badRequest(errorcodes.ContractDocumentNotReady,
			"documentPurpose is not supported for a contract.")
	}

	contract, err := s.requireContract(ctx, nil, t, contractID)
	if err != nil {
		return models.DocumentReference{}, err
	}
	if models.TerminalContract(contract.Status) {
		return models.DocumentReference{}, conflict(errorcodes.ContractInvalidState,
			"A contract in a final state cannot receive new documents.")
	}

	statusProjection, readiness, sourceVersion := s.readMediaState(ctx, t, input.MediaObjectID)

	var attached models.DocumentReference
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		var txErr error
		attached, txErr = s.opts.Repository.AttachContractDocument(ctx, tx,
			models.DocumentReference{
				ID:               ids.New(repositories.PrefixContractDocument),
				OrganizationID:   t.OrganizationID,
				SubjectID:        contractID,
				MediaObjectID:    input.MediaObjectID,
				DocumentPurpose:  input.DocumentPurpose,
				IsMandatory:      input.IsMandatory,
				StatusProjection: statusProjection,
				ReadinessState:   readiness,
				SourceVersion:    sourceVersion,
				CreatedBy:        t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Contract document", errorcodes.ContractDocumentNotReady,
				errorcodes.ContractDocumentNotReady)
		}
		return s.record(ctx, tx, t, "crm.contract.document_attached", "contract", contractID,
			nil, map[string]any{
				"documentPurpose":  input.DocumentPurpose,
				"statusProjection": statusProjection,
				"readinessState":   readiness,
			}, "")
	})
	if err != nil {
		return models.DocumentReference{}, err
	}
	return attached, nil
}

// ContractDocuments returns the document references for a contract.
func (s *Service) ContractDocuments(ctx context.Context, t tenant.Context,
	contractID string) ([]models.DocumentReference, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireContract(ctx, nil, t, contractID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListContractDocuments(ctx, nil, t.OrganizationID, contractID)
}

// ReplaceContractDocument points a document reference at a different Media
// object.
//
// The replacement is attached and the previous reference removed only when it
// is not bound to an activated version. A document that is part of an activated
// contract version is part of an immutable record and stays where it is.
func (s *Service) ReplaceContractDocument(ctx context.Context, t tenant.Context,
	contractID, documentID string,
	input AttachContractDocumentInput) (models.DocumentReference, error) {
	if err := s.writeEnabled(s.opts.Flags.ContractsEnabled); err != nil {
		return models.DocumentReference{}, err
	}
	existing, err := s.opts.Repository.GetContractDocument(ctx, nil, t.OrganizationID,
		contractID, documentID)
	if err != nil {
		return models.DocumentReference{}, translate(err, "Contract document",
			errorcodes.ContractDocumentNotReady, errorcodes.ContractDocumentNotReady)
	}
	if existing.SubjectVersionID != "" {
		return models.DocumentReference{}, conflict(errorcodes.ContractVersionConflict,
			"A document bound to an activated contract version cannot be replaced.")
	}
	if input.DocumentPurpose == "" {
		input.DocumentPurpose = existing.DocumentPurpose
	}
	replacement, err := s.AttachContractDocument(ctx, t, contractID, input)
	if err != nil {
		return models.DocumentReference{}, err
	}
	if replacement.ID != existing.ID {
		// Fleet's pointer goes; the Media object does not. Physical deletion is
		// Media's retention workflow, and a CRM operator swapping a draft must
		// not be able to destroy the previous file.
		_ = s.opts.Repository.RemoveContractDocument(ctx, nil, t.OrganizationID, contractID,
			existing.ID)
	}
	return replacement, nil
}

// RemoveContractDocument detaches a document reference from a contract.
//
// This removes the reference only. It never asks Media to delete anything:
// deletion goes through Media's retention and legal-hold workflow, which is the
// only place that knows whether a file may lawfully be destroyed.
func (s *Service) RemoveContractDocument(ctx context.Context, t tenant.Context,
	contractID, documentID, reason string) error {
	if err := s.writeEnabled(s.opts.Flags.ContractsEnabled); err != nil {
		return err
	}
	if _, err := s.requireContract(ctx, nil, t, contractID); err != nil {
		return err
	}
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if err := s.opts.Repository.RemoveContractDocument(ctx, tx, t.OrganizationID,
			contractID, documentID); err != nil {
			return translate(err, "Contract document", errorcodes.ContractDocumentNotReady,
				errorcodes.ContractVersionConflict)
		}
		return s.record(ctx, tx, t, "crm.contract.document_removed", "contract", contractID,
			map[string]any{"documentId": documentID}, nil, reason)
	})
}

// refreshContractDocuments re-reads every mandatory document from Media.
//
// It runs immediately before activation, outside the transaction. A projection
// written when the contract was approved is a memory of what Media said then,
// and a signed document revoked since must not be able to bring an agreement
// into force.
func (s *Service) refreshContractDocuments(ctx context.Context, t tenant.Context,
	contractID string) error {
	documents, err := s.opts.Repository.ListContractDocuments(ctx, nil, t.OrganizationID,
		contractID)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if !document.IsMandatory {
			continue
		}
		statusProjection, readiness, sourceVersion := s.readMediaState(ctx, t,
			document.MediaObjectID)
		if err := s.opts.Repository.UpdateContractDocumentProjection(ctx, nil,
			t.OrganizationID, document.ID, statusProjection, readiness, "",
			sourceVersion); err != nil {
			return err
		}
	}
	return nil
}
