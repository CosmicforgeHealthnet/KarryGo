// Package usecases implements the Section 28/56 governance workflows: the
// organization security-event read model and traceable privacy requests
// that honour legal holds.
package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Events publishes domain events inside the caller's transaction.
type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// Options configures the service.
type Options struct {
	Store  Repository
	Events Events
	Flags  models.Flags
	Now    func() time.Time
	// PrivacyResponseWindow is how long the organization has to answer a
	// privacy request; zero means 30 days.
	PrivacyResponseWindow time.Duration
}

// Service implements the governance workflows.
type Service struct {
	o     Options
	audit *audit.Recorder
}

// NewService builds the service.
func NewService(o Options) *Service {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.PrivacyResponseWindow <= 0 {
		o.PrivacyResponseWindow = 30 * 24 * time.Hour
	}
	return &Service{o: o, audit: audit.NewRecorder()}
}

func (s *Service) gate(on, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet integrations are disabled.", nil)
	}
	if !on {
		if write {
			return apperrors.Forbidden("This governance capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, id string, v int64, p any) error {
	if err := s.audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), "privacy_request", id)); err != nil {
		return err
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, "privacy_request", id, v, t.OrganizationID, t.CorrelationID, p)
	}
	return nil
}

// SecurityEvents lists the last 90 days of the organization's
// authentication security events.
func (s *Service) SecurityEvents(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.SecurityEvents, false); err != nil {
		return nil, err
	}
	return s.o.Store.SecurityEvents(ctx, t.OrganizationID, s.o.Now().Add(-90*24*time.Hour))
}

// SubmitPrivacyRequest opens a privacy request. One open request per type
// and subject; a repeat returns the open one.
func (s *Service) SubmitPrivacyRequest(ctx context.Context, t tenant.Context, in models.PrivacyRequestInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.PrivacyRequests, true); err != nil {
		return nil, err
	}
	in.RequestType = strings.ToUpper(strings.TrimSpace(in.RequestType))
	in.SubjectType = strings.ToUpper(strings.TrimSpace(in.SubjectType))
	in.Reason = strings.TrimSpace(in.Reason)
	if !models.PrivacyRequestTypes[in.RequestType] || !models.PrivacySubjectTypes[in.SubjectType] || in.SubjectReference == "" || in.Reason == "" || len(in.Reason) > 2000 {
		return nil, apperrors.Validation("requestType, subjectType, subjectReference and a reason are required.", nil)
	}
	id := ids.New("prq")
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		created, err := s.o.Store.InsertPrivacyRequest(ctx, tx, id, t.OrganizationID, in, t.ActorID, s.o.Now().Add(s.o.PrivacyResponseWindow))
		if err != nil {
			return err
		}
		if !created {
			id, err = s.o.Store.OpenPrivacyRequest(ctx, tx, t.OrganizationID, in)
			return err
		}
		if err = s.o.Store.InsertPrivacyHistory(ctx, tx, t.OrganizationID, id, "", "SUBMITTED", t.ActorID, in.Reason); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "privacy_request.submitted.v1", id, 1, map[string]any{"privacyRequestId": id, "requestType": in.RequestType, "subjectType": in.SubjectType})
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetPrivacyRequest(ctx, t.OrganizationID, id)
}

func (s *Service) GetPrivacyRequest(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.PrivacyRequests, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetPrivacyRequest(ctx, t.OrganizationID, id)
}

func (s *Service) ListPrivacyRequests(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.PrivacyRequests, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListPrivacyRequests(ctx, t.OrganizationID)
}

// DecidePrivacyRequest approves or rejects a submitted request. The
// requester cannot decide their own request. Approving an ERASURE while an
// active legal hold covers the subject (or the organization) records
// BLOCKED_BY_LEGAL_HOLD with the hold IDs instead (spec 28.5, 56.2).
func (s *Service) DecidePrivacyRequest(ctx context.Context, t tenant.Context, id string, in models.PrivacyDecisionInput) (map[string]any, error) {
	return s.transition(ctx, t, id, in, func(tx pgx.Tx, st models.PrivacyState) (string, []string, error) {
		if st.Status != "SUBMITTED" {
			return "", nil, apperrors.New(http.StatusConflict, "state_transition_invalid", "Only a submitted request can be decided.", nil)
		}
		if st.RequestedBy == t.ActorID {
			return "", nil, apperrors.New(http.StatusForbidden, "segregation_of_duties_violation", "The requester cannot decide their own privacy request.", nil)
		}
		switch strings.ToUpper(in.Decision) {
		case "REJECT":
			if strings.TrimSpace(in.Reason) == "" {
				return "", nil, apperrors.Validation("A rejection reason is required.", nil)
			}
			return "REJECTED", nil, nil
		case "APPROVE":
			if st.RequestType == "ERASURE" {
				holds, err := s.o.Store.ActiveHolds(ctx, tx, t.OrganizationID, models.HoldScopeFor(st.SubjectType), st.SubjectReference)
				if err != nil {
					return "", nil, err
				}
				if len(holds) > 0 {
					return "BLOCKED_BY_LEGAL_HOLD", holds, nil
				}
			}
			return "APPROVED", nil, nil
		}
		return "", nil, apperrors.Validation("decision must be APPROVE or REJECT.", nil)
	})
}

// CompletePrivacyRequest records what was done for an approved request. An
// erasure is re-checked against legal holds at completion.
func (s *Service) CompletePrivacyRequest(ctx context.Context, t tenant.Context, id string, in models.PrivacyDecisionInput) (map[string]any, error) {
	return s.transition(ctx, t, id, in, func(tx pgx.Tx, st models.PrivacyState) (string, []string, error) {
		if st.Status != "APPROVED" {
			return "", nil, apperrors.New(http.StatusConflict, "state_transition_invalid", "Only an approved request can be completed.", nil)
		}
		var report map[string]any
		if len(in.CompletionReport) == 0 || json.Unmarshal(in.CompletionReport, &report) != nil || len(report) == 0 {
			return "", nil, apperrors.Validation("A completion report object is required.", nil)
		}
		if st.RequestType == "ERASURE" {
			holds, err := s.o.Store.ActiveHolds(ctx, tx, t.OrganizationID, models.HoldScopeFor(st.SubjectType), st.SubjectReference)
			if err != nil {
				return "", nil, err
			}
			if len(holds) > 0 {
				return "BLOCKED_BY_LEGAL_HOLD", holds, nil
			}
		}
		return "COMPLETED", nil, nil
	})
}

func (s *Service) transition(ctx context.Context, t tenant.Context, id string, in models.PrivacyDecisionInput, decide func(pgx.Tx, models.PrivacyState) (string, []string, error)) (map[string]any, error) {
	if err := s.gate(s.o.Flags.PrivacyRequests, true); err != nil {
		return nil, err
	}
	if in.ExpectedVersion <= 0 {
		return nil, apperrors.Validation("expectedVersion is required.", nil)
	}
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		st, err := s.o.Store.LockPrivacyRequest(ctx, tx, t.OrganizationID, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound("Resource not found.", nil)
		}
		if err != nil {
			return err
		}
		if st.Version != in.ExpectedVersion {
			return apperrors.New(http.StatusConflict, "version_conflict", "Privacy request version changed.", nil)
		}
		next, holds, err := decide(tx, st)
		if err != nil {
			return err
		}
		var report json.RawMessage
		if next == "COMPLETED" {
			report = in.CompletionReport
		}
		v, err := s.o.Store.UpdatePrivacyRequest(ctx, tx, t.OrganizationID, id, next, t.ActorID, in.Reason, holds, report)
		if err != nil {
			return err
		}
		if err = s.o.Store.InsertPrivacyHistory(ctx, tx, t.OrganizationID, id, st.Status, next, t.ActorID, in.Reason); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "privacy_request."+strings.ToLower(next)+".v1", id, v, map[string]any{"privacyRequestId": id, "status": next, "blockingHoldCount": len(holds)})
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetPrivacyRequest(ctx, t.OrganizationID, id)
}
