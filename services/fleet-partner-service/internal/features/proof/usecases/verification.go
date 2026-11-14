package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/proof/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// GetRecord reads one proof record of the organization with its verification
// history.
func (s *Service) GetRecord(ctx context.Context, t tenant.Context, id string) (models.RecordDetail, error) {
	if e := s.gate(s.o.Flags.Records, false); e != nil {
		return models.RecordDetail{}, e
	}
	r, e := s.repo.GetRecord(ctx, nil, t.OrganizationID, id, false)
	if errors.Is(e, pgx.ErrNoRows) {
		return models.RecordDetail{}, apperrors.NotFound("Proof record not found.", nil)
	}
	if e != nil {
		return models.RecordDetail{}, e
	}
	vs, e := s.repo.ListVerifications(ctx, nil, t.OrganizationID, id)
	if e != nil {
		return models.RecordDetail{}, e
	}
	return models.RecordDetail{Record: r, EffectiveStatus: models.EffectiveStatus(r, vs), Verifications: vs}, nil
}

// Decide verifies or rejects one proof record against the policy snapshot it
// was submitted under (spec 49.3). The record itself is immutable: the
// decision is a new append-only verification row, and an organization
// decision is final -- a correction is a new, superseding proof record.
func (s *Service) Decide(ctx context.Context, t tenant.Context, id, decision string, in models.DecisionInput) (models.RecordDetail, error) {
	if e := s.gate(s.o.Flags.Records, true); e != nil {
		return models.RecordDetail{}, e
	}
	decision = strings.ToUpper(decision)
	if decision != "VERIFIED" && decision != "REJECTED" {
		return models.RecordDetail{}, apperrors.BadRequest("Decision must be VERIFIED or REJECTED.", nil)
	}
	codes := []string{}
	for _, c := range in.ReasonCodes {
		if c = strings.ToUpper(strings.TrimSpace(c)); c != "" {
			codes = append(codes, c)
		}
	}
	if decision == "REJECTED" && len(codes) == 0 {
		return models.RecordDetail{}, apperrors.BadRequest("A rejection requires at least one reason code.", nil)
	}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		r, x := s.repo.GetRecord(ctx, tx, t.OrganizationID, id, true)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.NotFound("Proof record not found.", nil)
		}
		if x != nil {
			return x
		}
		// Serialize decisions on one record so two reviewers cannot both
		// pass the "not yet decided" check.
		if x = s.repo.LockDecision(ctx, tx, r.ID); x != nil {
			return x
		}
		if r.ActorReference == t.ActorID {
			return apperrors.Forbidden("Proof cannot be verified or rejected by the actor who submitted it.", nil)
		}
		vs, x := s.repo.ListVerifications(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		for _, v := range vs {
			if v.AuthorityType == models.AuthorityOrganization {
				return apperrors.Conflict("Proof has already been decided; submit a superseding proof record instead.", nil)
			}
		}
		if decision == "VERIFIED" {
			if failed := s.policyFailures(ctx, t.OrganizationID, r, vs); len(failed) > 0 {
				return apperrors.Conflict("Proof does not satisfy its policy snapshot: "+strings.Join(failed, ", ")+".", nil)
			}
			if len(codes) == 0 {
				codes = []string{"POLICY_REQUIREMENTS_MET"}
			}
		}
		raw, _ := json.Marshal(codes)
		if x = s.repo.InsertDecision(ctx, tx, t.OrganizationID, id, decision, models.AuthorityOrganization, t.ActorID, raw); x != nil {
			return x
		}
		event := "proof.verified.v1"
		if decision == "REJECTED" {
			event = "proof.rejected.v1"
		}
		return s.record(ctx, tx, t, event, "proof_record", id, r.Version, map[string]any{"proofRecordId": id, "proofType": r.ProofType, "decision": decision, "reasonCodes": codes})
	})
	if e != nil {
		return models.RecordDetail{}, e
	}
	return s.GetRecord(ctx, t, id)
}

// policyFailures loads the record's policy snapshot and applies the pure
// policy rules to it.
func (s *Service) policyFailures(ctx context.Context, org string, r models.Record, vs []models.Verification) []string {
	_, p, e := s.snapshot(ctx, org, r.PolicySnapshotReference)
	if e != nil {
		return []string{"POLICY_SNAPSHOT_UNAVAILABLE"}
	}
	return models.PolicyFailures(p, r, vs)
}
