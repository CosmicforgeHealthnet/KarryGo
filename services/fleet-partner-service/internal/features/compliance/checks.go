package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Check is one immutable compliance check run (spec 49.1 compliance_checks):
// every active requirement that applies to a subject, evaluated together.
// The strictest result governs (spec 49.2).
type Check struct {
	ID, OrganizationID, SubjectType, SubjectReference string
	SourceType, SourceReference                       string
	SourceVersion                                     int64
	OverallState, GoverningRequirementID              string
	RequirementCount                                  int
	Results                                           []CheckResult
	RestrictionReferences                             []string
	RequestedBy, CorrelationID                        string
	CreatedAt                                         time.Time
}

// CheckResult links one requirement to the evaluation the check produced.
type CheckResult struct {
	RequirementID        string `json:"requirementId"`
	RequirementCode      string `json:"requirementCode"`
	Severity             string `json:"severity"`
	BlocksOperation      bool   `json:"blocksOperation"`
	ComplianceStatusID   string `json:"complianceStatusId"`
	State                string `json:"state"`
	RestrictionReference string `json:"restrictionReference,omitempty"`
}

// CheckInput requests a compliance check of one subject.
type CheckInput struct {
	SubjectType, SubjectReference, SourceType, SourceReference string
	SourceVersion                                              int64
}

// stateRank orders evaluation states from least to most restrictive, so the
// strictest applicable requirement governs the overall result.
var stateRank = map[string]int{
	StateNotApplicable: 0, StateCompliant: 1, StateDueSoon: 2, StatePending: 3,
	StateUnknown: 4, StateExpired: 5, StateNonCompliant: 6, StateRestricted: 7,
}

// checkSubjectVisible proves the subject belongs to the caller's organization
// (and branch, where the subject has one). A subject of another organization
// is a safe 404, so a check can never evaluate or restrict it.
func (s *Service) checkSubjectVisible(ctx context.Context, tx pgx.Tx, t tenant.Context, typ, ref string) error {
	var branch string
	var err error
	switch typ {
	case "ORGANIZATION":
		if ref != t.OrganizationID {
			return apperrors.NotFound("Compliance subject not found.", nil)
		}
		return nil
	case "PROVIDER":
		err = tx.QueryRow(ctx, `SELECT COALESCE(home_branch_id,'') FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND id=$2`, t.OrganizationID, ref).Scan(&branch)
	case "ASSET":
		err = tx.QueryRow(ctx, `SELECT branch_id FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2`, t.OrganizationID, ref).Scan(&branch)
	case "ORDER":
		err = tx.QueryRow(ctx, `SELECT COALESCE(branch_id,'') FROM fleet_partner_private_orders WHERE organization_id=$1 AND id=$2`, t.OrganizationID, ref).Scan(&branch)
	case "SHIPMENT":
		err = tx.QueryRow(ctx, `SELECT COALESCE(branch_id,'') FROM fleet_partner_shipments WHERE organization_id=$1 AND id=$2`, t.OrganizationID, ref).Scan(&branch)
	default:
		// CARGO, WAREHOUSE and LOCATION subjects are references whose
		// owning records are validated by their own features.
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !t.BranchPermitted(branch) {
		return apperrors.NotFound("Compliance subject not found.", nil)
	}
	return err
}

// RunCheck evaluates every active requirement for the subject in one
// transaction and records the check, its evaluations, any restrictions they
// applied, the audit entry and the outbox event atomically. Restrictions are
// propagated to the execution services after commit.
func (s *Service) RunCheck(ctx context.Context, t tenant.Context, in CheckInput) (Check, error) {
	if err := s.gate(true); err != nil {
		return Check{}, err
	}
	in.SubjectType = strings.ToUpper(strings.TrimSpace(in.SubjectType))
	in.SubjectReference = strings.TrimSpace(in.SubjectReference)
	in.SourceType = strings.ToUpper(strings.TrimSpace(in.SourceType))
	if !validSubject(in.SubjectType) || in.SubjectReference == "" || in.SourceType == "" || in.SourceVersion < 0 {
		return Check{}, apperrors.BadRequest("Subject type, subject reference, source type and a non-negative sourceVersion are required.", nil)
	}
	now := s.o.Now()
	out := Check{ID: ids.New("cmpchk"), OrganizationID: t.OrganizationID, SubjectType: in.SubjectType, SubjectReference: in.SubjectReference, SourceType: in.SourceType, SourceReference: in.SourceReference, SourceVersion: in.SourceVersion, OverallState: StateNotApplicable, Results: []CheckResult{}, RestrictionReferences: []string{}, RequestedBy: t.ActorID, CorrelationID: t.CorrelationID}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if e := s.checkSubjectVisible(ctx, tx, t, in.SubjectType, in.SubjectReference); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+requirementColumns+` FROM fleet_partner_compliance_requirements WHERE organization_id=$1 AND subject_type=$2 AND status='ACTIVE' ORDER BY code,id`, t.OrganizationID, in.SubjectType)
		if e != nil {
			return e
		}
		var reqs []Requirement
		for rows.Next() {
			r, x := scanRequirement(rows)
			if x != nil {
				rows.Close()
				return x
			}
			if r.ActiveAt(now) {
				reqs = append(reqs, r)
			}
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		for _, r := range reqs {
			st, restriction, x := s.evaluateTx(ctx, tx, t, r, EvaluateInput{SubjectType: in.SubjectType, SubjectReference: in.SubjectReference, RequirementID: r.ID, SourceType: in.SourceType, SourceReference: in.SourceReference, SourceVersion: in.SourceVersion})
			if x != nil {
				return x
			}
			out.Results = append(out.Results, CheckResult{RequirementID: r.ID, RequirementCode: r.Code, Severity: r.Severity, BlocksOperation: r.BlocksOperation, ComplianceStatusID: st.ID, State: st.State, RestrictionReference: st.RestrictionReference})
			if restriction != "" {
				out.RestrictionReferences = append(out.RestrictionReferences, restriction)
			}
			if stateRank[st.State] > stateRank[out.OverallState] {
				out.OverallState, out.GoverningRequirementID = st.State, r.ID
			}
		}
		out.RequirementCount = len(out.Results)
		results, _ := json.Marshal(out.Results)
		restrictions, _ := json.Marshal(out.RestrictionReferences)
		if e = tx.QueryRow(ctx, `INSERT INTO fleet_partner_compliance_checks(id,organization_id,subject_type,subject_reference,source_type,source_reference,source_version,overall_state,governing_requirement_id,requirement_count,results,restriction_references,requested_by,correlation_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING created_at`, out.ID, out.OrganizationID, out.SubjectType, out.SubjectReference, out.SourceType, out.SourceReference, out.SourceVersion, out.OverallState, out.GoverningRequirementID, out.RequirementCount, results, restrictions, out.RequestedBy, out.CorrelationID).Scan(&out.CreatedAt); e != nil {
			return e
		}
		return s.record(ctx, tx, t, "compliance_check.completed.v1", "compliance_check", out.ID, 1, map[string]any{"checkId": out.ID, "subjectType": out.SubjectType, "subjectReference": out.SubjectReference, "overallState": out.OverallState, "requirementCount": out.RequirementCount, "restrictionReferences": out.RestrictionReferences})
	})
	if err != nil {
		return Check{}, err
	}
	for _, id := range out.RestrictionReferences {
		_ = s.ProcessRestriction(ctx, t.OrganizationID, id)
	}
	return out, nil
}

const checkColumns = `id,organization_id,subject_type,subject_reference,source_type,source_reference,source_version,overall_state,governing_requirement_id,requirement_count,results,restriction_references,requested_by,correlation_id,created_at`

func scanCheck(row pgx.Row) (Check, error) {
	var v Check
	var results, restrictions []byte
	e := row.Scan(&v.ID, &v.OrganizationID, &v.SubjectType, &v.SubjectReference, &v.SourceType, &v.SourceReference, &v.SourceVersion, &v.OverallState, &v.GoverningRequirementID, &v.RequirementCount, &results, &restrictions, &v.RequestedBy, &v.CorrelationID, &v.CreatedAt)
	if e != nil {
		return v, e
	}
	v.Results, v.RestrictionReferences = []CheckResult{}, []string{}
	if e = json.Unmarshal(results, &v.Results); e != nil {
		return v, e
	}
	return v, json.Unmarshal(restrictions, &v.RestrictionReferences)
}

// GetCheck reads one compliance check of the organization.
func (s *Service) GetCheck(ctx context.Context, t tenant.Context, id string) (Check, error) {
	if err := s.gate(false); err != nil {
		return Check{}, err
	}
	v, e := scanCheck(s.o.Pool.QueryRow(ctx, `SELECT `+checkColumns+` FROM fleet_partner_compliance_checks WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return Check{}, apperrors.NotFound("Compliance check not found.", nil)
	}
	return v, e
}

// ListChecks lists the organization's compliance checks, newest first,
// optionally for one subject.
func (s *Service) ListChecks(ctx context.Context, t tenant.Context, typ, ref string) ([]Check, error) {
	if err := s.gate(false); err != nil {
		return nil, err
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+checkColumns+` FROM fleet_partner_compliance_checks WHERE organization_id=$1 AND($2=''OR subject_type=$2)AND($3=''OR subject_reference=$3) ORDER BY created_at DESC,id DESC LIMIT 200`, t.OrganizationID, strings.ToUpper(typ), ref)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Check{}
	for rows.Next() {
		v, e := scanCheck(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Propagation is the confirmation state of one restriction at one execution
// authority (spec 19.1: confirmed everywhere by reconciliation).
type Propagation struct {
	Authority        string     `json:"authority"`
	Action           string     `json:"action"`
	Status           string     `json:"status"`
	Attempts         int        `json:"attempts"`
	LastErrorCode    string     `json:"lastErrorCode"`
	NextAttemptAt    time.Time  `json:"nextAttemptAt"`
	ConfirmedAt      *time.Time `json:"confirmedAt"`
	AuthorityVersion int64      `json:"authorityVersion"`
}

// RestrictionView is a restriction with its propagation state.
type RestrictionView struct {
	Restriction
	Propagation     []Propagation
	FullyPropagated bool
}

// RestrictionDetail reads a restriction and where it has been confirmed.
func (s *Service) RestrictionDetail(ctx context.Context, t tenant.Context, id string) (RestrictionView, error) {
	r, e := s.GetRestriction(ctx, t.OrganizationID, id)
	if e != nil {
		return RestrictionView{}, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT authority_service,action,status,attempts,last_error_code,next_attempt_at,confirmed_at,authority_version FROM fleet_partner_restriction_operations WHERE organization_id=$1 AND restriction_id=$2 ORDER BY created_at,authority_service`, t.OrganizationID, id)
	if e != nil {
		return RestrictionView{}, e
	}
	defer rows.Close()
	out := RestrictionView{Restriction: r, Propagation: []Propagation{}, FullyPropagated: true}
	for rows.Next() {
		var p Propagation
		if e = rows.Scan(&p.Authority, &p.Action, &p.Status, &p.Attempts, &p.LastErrorCode, &p.NextAttemptAt, &p.ConfirmedAt, &p.AuthorityVersion); e != nil {
			return RestrictionView{}, e
		}
		if p.Status != "CONFIRMED" {
			out.FullyPropagated = false
		}
		out.Propagation = append(out.Propagation, p)
	}
	return out, rows.Err()
}

// ListRestrictions lists the organization's restrictions, optionally for one
// subject or state.
func (s *Service) ListRestrictions(ctx context.Context, t tenant.Context, typ, ref, state string) ([]Restriction, error) {
	if !s.o.Flags.Restrictions {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+restrictionColumns+` FROM fleet_partner_restrictions WHERE organization_id=$1 AND($2=''OR subject_type=$2)AND($3=''OR subject_reference=$3)AND($4=''OR state=$4) ORDER BY effective_at DESC,id DESC LIMIT 200`, t.OrganizationID, strings.ToUpper(typ), ref, strings.ToUpper(state))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Restriction{}
	for rows.Next() {
		v, e := scanRestriction(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
