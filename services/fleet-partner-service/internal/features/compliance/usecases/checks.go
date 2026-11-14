package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// checkSubjectVisible proves the subject belongs to the caller's organization
// (and branch, where the subject has one). A subject of another organization
// is a safe 404, so a check can never evaluate or restrict it.
func (s *Service) checkSubjectVisible(ctx context.Context, tx pgx.Tx, t tenant.Context, typ, ref string) error {
	if typ == "ORGANIZATION" {
		if ref != t.OrganizationID {
			return apperrors.NotFound("Compliance subject not found.", nil)
		}
		return nil
	}
	// CARGO, WAREHOUSE and LOCATION subjects are references whose owning
	// records are validated by their own features (owned=false).
	branch, owned, err := s.repo.SubjectBranch(ctx, tx, t.OrganizationID, typ, ref)
	if !owned {
		return nil
	}
	if errors.Is(err, models.ErrNotFound) || err == nil && !t.BranchPermitted(branch) {
		return apperrors.NotFound("Compliance subject not found.", nil)
	}
	return err
}

// RunCheck evaluates every active requirement for the subject in one
// transaction and records the check, its evaluations, any restrictions they
// applied, the audit entry and the outbox event atomically. Restrictions are
// propagated to the execution services after commit.
func (s *Service) RunCheck(ctx context.Context, t tenant.Context, in models.CheckInput) (models.Check, error) {
	if err := s.gate(true); err != nil {
		return models.Check{}, err
	}
	in.SubjectType = strings.ToUpper(strings.TrimSpace(in.SubjectType))
	in.SubjectReference = strings.TrimSpace(in.SubjectReference)
	in.SourceType = strings.ToUpper(strings.TrimSpace(in.SourceType))
	if !models.ValidSubject(in.SubjectType) || in.SubjectReference == "" || in.SourceType == "" || in.SourceVersion < 0 {
		return models.Check{}, apperrors.BadRequest("Subject type, subject reference, source type and a non-negative sourceVersion are required.", nil)
	}
	now := s.o.Now()
	out := models.Check{ID: ids.New("cmpchk"), OrganizationID: t.OrganizationID, SubjectType: in.SubjectType, SubjectReference: in.SubjectReference, SourceType: in.SourceType, SourceReference: in.SourceReference, SourceVersion: in.SourceVersion, OverallState: models.StateNotApplicable, Results: []models.CheckResult{}, RestrictionReferences: []string{}, RequestedBy: t.ActorID, CorrelationID: t.CorrelationID}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if e := s.checkSubjectVisible(ctx, tx, t, in.SubjectType, in.SubjectReference); e != nil {
			return e
		}
		all, e := s.repo.ListActiveRequirements(ctx, tx, t.OrganizationID, in.SubjectType)
		if e != nil {
			return e
		}
		var reqs []models.Requirement
		for _, r := range all {
			if r.ActiveAt(now) {
				reqs = append(reqs, r)
			}
		}
		for _, r := range reqs {
			st, restriction, x := s.evaluateTx(ctx, tx, t, r, models.EvaluateInput{SubjectType: in.SubjectType, SubjectReference: in.SubjectReference, RequirementID: r.ID, SourceType: in.SourceType, SourceReference: in.SourceReference, SourceVersion: in.SourceVersion})
			if x != nil {
				return x
			}
			out.Results = append(out.Results, models.CheckResult{RequirementID: r.ID, RequirementCode: r.Code, Severity: r.Severity, BlocksOperation: r.BlocksOperation, ComplianceStatusID: st.ID, State: st.State, RestrictionReference: st.RestrictionReference})
			if restriction != "" {
				out.RestrictionReferences = append(out.RestrictionReferences, restriction)
			}
			if models.StateRank[st.State] > models.StateRank[out.OverallState] {
				out.OverallState, out.GoverningRequirementID = st.State, r.ID
			}
		}
		out.RequirementCount = len(out.Results)
		if out.CreatedAt, e = s.repo.InsertCheck(ctx, tx, out); e != nil {
			return e
		}
		return s.record(ctx, tx, t, "compliance_check.completed.v1", "compliance_check", out.ID, 1, map[string]any{"checkId": out.ID, "subjectType": out.SubjectType, "subjectReference": out.SubjectReference, "overallState": out.OverallState, "requirementCount": out.RequirementCount, "restrictionReferences": out.RestrictionReferences})
	})
	if err != nil {
		return models.Check{}, err
	}
	for _, id := range out.RestrictionReferences {
		_ = s.ProcessRestriction(ctx, t.OrganizationID, id)
	}
	return out, nil
}

// GetCheck reads one compliance check of the organization.
func (s *Service) GetCheck(ctx context.Context, t tenant.Context, id string) (models.Check, error) {
	if err := s.gate(false); err != nil {
		return models.Check{}, err
	}
	v, e := s.repo.GetCheck(ctx, t.OrganizationID, id)
	if errors.Is(e, models.ErrNotFound) {
		return models.Check{}, apperrors.NotFound("Compliance check not found.", nil)
	}
	return v, e
}

// ListChecks lists the organization's compliance checks, newest first,
// optionally for one subject.
func (s *Service) ListChecks(ctx context.Context, t tenant.Context, typ, ref string) ([]models.Check, error) {
	if err := s.gate(false); err != nil {
		return nil, err
	}
	return s.repo.ListChecks(ctx, t.OrganizationID, strings.ToUpper(typ), ref)
}

// RestrictionDetail reads a restriction and where it has been confirmed.
func (s *Service) RestrictionDetail(ctx context.Context, t tenant.Context, id string) (models.RestrictionView, error) {
	r, e := s.GetRestriction(ctx, t.OrganizationID, id)
	if e != nil {
		return models.RestrictionView{}, e
	}
	propagation, e := s.repo.ListPropagation(ctx, t.OrganizationID, id)
	if e != nil {
		return models.RestrictionView{}, e
	}
	out := models.RestrictionView{Restriction: r, Propagation: propagation, FullyPropagated: true}
	for _, p := range propagation {
		if p.Status != "CONFIRMED" {
			out.FullyPropagated = false
		}
	}
	return out, nil
}

// ListRestrictions lists the organization's restrictions, optionally for one
// subject or state.
func (s *Service) ListRestrictions(ctx context.Context, t tenant.Context, typ, ref, state string) ([]models.Restriction, error) {
	if !s.o.Flags.Restrictions {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	return s.repo.ListRestrictions(ctx, t.OrganizationID, strings.ToUpper(typ), ref, strings.ToUpper(state))
}
