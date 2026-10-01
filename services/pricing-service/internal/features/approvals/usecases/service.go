// Package usecases owns the approval workflow.
package usecases

import (
	"context"
	"fmt"

	"cosmicforge/logistics/services/pricing-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/pricing-service/internal/platform/ids"
)

// Store is the persistence port.
type Store interface {
	Create(ctx context.Context, request models.Request) error
	Get(ctx context.Context, id string) (models.Request, error)
	Decide(ctx context.Context, decision models.Decision) error
	Cancel(ctx context.Context, id string) error
	ListPending(ctx context.Context, limit int) ([]models.Request, error)
	ApprovedFor(ctx context.Context, subjectType, subjectID, subjectHash string) (models.Request, error)
	PendingFor(ctx context.Context, subjectType, subjectID string) (models.Request, error)
}

// Service is the approval workflow.
type Service struct{ store Store }

// NewService builds the workflow.
func NewService(store Store) *Service { return &Service{store: store} }

// Request opens a pending approval bound to the exact content being approved.
func (s *Service) Request(ctx context.Context, subjectType, subjectID, subjectHash, requestedBy, reason string) (string, error) {
	if err := models.ValidateRequest(subjectType, subjectID, subjectHash, requestedBy, reason); err != nil {
		return "", err
	}
	request := models.Request{
		ID:          ids.New(ids.PrefixApproval),
		SubjectType: subjectType,
		SubjectID:   subjectID,
		SubjectHash: subjectHash,
		RequestedBy: requestedBy,
		Reason:      reason,
	}
	if err := s.store.Create(ctx, request); err != nil {
		return "", err
	}
	return request.ID, nil
}

// DecideForSubject records the decision on the pending request for a subject.
//
// The four-eye rule is enforced by the database trigger; this maps its refusal
// onto a domain error so the caller can say something useful.
func (s *Service) DecideForSubject(ctx context.Context, subjectType, subjectID, decidedBy, reason, decision string) error {
	pending, err := s.store.PendingFor(ctx, subjectType, subjectID)
	if err != nil {
		return err
	}
	return s.store.Decide(ctx, models.Decision{
		ID:        ids.New(ids.PrefixDecision),
		RequestID: pending.ID,
		DecidedBy: decidedBy,
		Decision:  decision,
		Reason:    reason,
	})
}

// ApprovedFor returns the approval that still matches the subject's content.
func (s *Service) ApprovedFor(ctx context.Context, subjectType, subjectID, subjectHash string) (models.Request, error) {
	return s.store.ApprovedFor(ctx, subjectType, subjectID, subjectHash)
}

// ListPending returns the review queue.
func (s *Service) ListPending(ctx context.Context, limit int) ([]models.Request, error) {
	return s.store.ListPending(ctx, limit)
}

// Cancel withdraws a pending request.
func (s *Service) Cancel(ctx context.Context, subjectType, subjectID string) error {
	pending, err := s.store.PendingFor(ctx, subjectType, subjectID)
	if err != nil {
		return err
	}
	if err := s.store.Cancel(ctx, pending.ID); err != nil {
		return fmt.Errorf("approvals: cancel %s: %w", pending.ID, err)
	}
	return nil
}
