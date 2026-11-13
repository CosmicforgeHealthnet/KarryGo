package proof

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// AuthorityOrganization is the verification authority of a decision made by
// an authorized member of the organization (spec 49.3 POST /proofs/{id}/verify).
// Media-safety verifications use AuthorityMedia.
const (
	AuthorityOrganization = "ORGANIZATION"
	AuthorityMedia        = "MEDIA"
)

// RecordDetail is one proof record with its append-only verification history
// and the status that governs it.
type RecordDetail struct {
	Record             Record
	EffectiveStatus    string
	Verifications      []Verification
	PolicySnapshotHash string
}

// effectiveStatus applies the decision order: an organization decision is
// final; otherwise a media rejection wins over the status the record was
// created with, and a media verification confirms it.
func effectiveStatus(r Record, vs []Verification) string {
	media := ""
	for _, v := range vs {
		if v.AuthorityType == AuthorityOrganization {
			return v.Status
		}
		if v.AuthorityType == AuthorityMedia && media != "REJECTED" {
			media = v.Status
		}
	}
	if media == "REJECTED" {
		return media
	}
	if media == "VERIFIED" {
		return media
	}
	return r.VerificationStatus
}

func (s *Service) verifications(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, org, id string) ([]Verification, error) {
	rows, e := q.Query(ctx, `SELECT id,organization_id,proof_record_id,status,authority_type,authority_reference,source_version,reason_codes,verified_at FROM fleet_partner_proof_verifications WHERE organization_id=$1 AND proof_record_id=$2 ORDER BY verified_at,id`, org, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Verification{}
	for rows.Next() {
		var v Verification
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.ProofRecordID, &v.Status, &v.AuthorityType, &v.AuthorityReference, &v.SourceVersion, &v.ReasonCodes, &v.VerifiedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetRecord reads one proof record of the organization with its verification
// history.
func (s *Service) GetRecord(ctx context.Context, t tenant.Context, id string) (RecordDetail, error) {
	if e := s.gate(s.o.Flags.Records, false); e != nil {
		return RecordDetail{}, e
	}
	r, e := scanRecord(s.o.Pool.QueryRow(ctx, `SELECT `+recordCols+` FROM fleet_partner_proof_records WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return RecordDetail{}, apperrors.NotFound("Proof record not found.", nil)
	}
	if e != nil {
		return RecordDetail{}, e
	}
	vs, e := s.verifications(ctx, s.o.Pool, t.OrganizationID, id)
	if e != nil {
		return RecordDetail{}, e
	}
	return RecordDetail{Record: r, EffectiveStatus: effectiveStatus(r, vs), Verifications: vs}, nil
}

// DecisionInput is an organization verification decision.
type DecisionInput struct {
	ReasonCodes []string
}

// Decide verifies or rejects one proof record against the policy snapshot it
// was submitted under (spec 49.3). The record itself is immutable: the
// decision is a new append-only verification row, and an organization
// decision is final -- a correction is a new, superseding proof record.
func (s *Service) Decide(ctx context.Context, t tenant.Context, id, decision string, in DecisionInput) (RecordDetail, error) {
	if e := s.gate(s.o.Flags.Records, true); e != nil {
		return RecordDetail{}, e
	}
	decision = strings.ToUpper(decision)
	if decision != "VERIFIED" && decision != "REJECTED" {
		return RecordDetail{}, apperrors.BadRequest("Decision must be VERIFIED or REJECTED.", nil)
	}
	codes := []string{}
	for _, c := range in.ReasonCodes {
		if c = strings.ToUpper(strings.TrimSpace(c)); c != "" {
			codes = append(codes, c)
		}
	}
	if decision == "REJECTED" && len(codes) == 0 {
		return RecordDetail{}, apperrors.BadRequest("A rejection requires at least one reason code.", nil)
	}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		r, x := scanRecord(tx.QueryRow(ctx, `SELECT `+recordCols+` FROM fleet_partner_proof_records WHERE organization_id=$1 AND id=$2 FOR SHARE`, t.OrganizationID, id))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.NotFound("Proof record not found.", nil)
		}
		if x != nil {
			return x
		}
		// Serialize decisions on one record so two reviewers cannot both
		// pass the "not yet decided" check.
		if _, x = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "proof-decision:"+r.ID); x != nil {
			return x
		}
		if r.ActorReference == t.ActorID {
			return apperrors.Forbidden("Proof cannot be verified or rejected by the actor who submitted it.", nil)
		}
		vs, x := s.verifications(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		for _, v := range vs {
			if v.AuthorityType == AuthorityOrganization {
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
		if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_proof_verifications(id,organization_id,proof_record_id,status,authority_type,authority_reference,source_version,reason_codes)VALUES($1,$2,$3,$4,$5,$6,0,$7)`, ids.New("prfver"), t.OrganizationID, id, decision, AuthorityOrganization, t.ActorID, raw); x != nil {
			return x
		}
		event := "proof.verified.v1"
		if decision == "REJECTED" {
			event = "proof.rejected.v1"
		}
		return s.record(ctx, tx, t, event, "proof_record", id, r.Version, map[string]any{"proofRecordId": id, "proofType": r.ProofType, "decision": decision, "reasonCodes": codes})
	})
	if e != nil {
		return RecordDetail{}, e
	}
	return s.GetRecord(ctx, t, id)
}

// policyFailures re-checks a record against the policy snapshot it claims to
// satisfy. A record can only be verified if it still meets every rule of that
// snapshot and no media authority rejected its evidence.
func (s *Service) policyFailures(ctx context.Context, org string, r Record, vs []Verification) []string {
	_, p, e := s.snapshot(ctx, org, r.PolicySnapshotReference)
	if e != nil {
		return []string{"POLICY_SNAPSHOT_UNAVAILABLE"}
	}
	var failed []string
	var required []string
	_ = json.Unmarshal(p.RequiredProofTypes, &required)
	found := false
	for _, x := range required {
		if strings.ToUpper(x) == r.ProofType {
			found = true
		}
	}
	if !found {
		failed = append(failed, "PROOF_TYPE_NOT_REQUIRED")
	}
	if (p.LocationRequired || r.ProofType == "LOCATION") && (r.Latitude == nil || r.Longitude == nil) {
		failed = append(failed, "LOCATION_MISSING")
	}
	if (r.ProofType == "PHOTO" || r.ProofType == "SIGNATURE" || r.ProofType == "DOCUMENT_SCAN") && r.MediaFileID == "" {
		failed = append(failed, "MEDIA_MISSING")
	}
	if r.ProofType == "OTP" && r.ValueHash == "" {
		failed = append(failed, "OTP_VALUE_MISSING")
	}
	for _, v := range vs {
		if v.AuthorityType == AuthorityMedia && v.Status == "REJECTED" {
			failed = append(failed, "MEDIA_REJECTED")
			break
		}
	}
	return failed
}
