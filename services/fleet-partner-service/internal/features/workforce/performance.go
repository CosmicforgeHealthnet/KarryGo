package workforce

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Provider performance snapshots (spec 17 and 46.2, FLEET-17.10).
//
// Formula v2 counts BOTH company assignment channels -- marketplace
// assignments (fleet_partner_provider_assignments with their immutable
// provider responses) and private assignments -- and keeps the categories the
// spec requires apart: company assignment quality, provider response, safety,
// service quality and productivity. Decline, expiry and cancellation are
// distinct outcomes and are never folded into each other.
//
// A snapshot is a set of explainable counts. It never ranks providers, never
// triggers a suspension, penalty or deduction, and a safety-related decline is
// counted only so it can be reviewed in context -- never as misconduct. Every
// assignment that contributed is listed in metric_inputs, so each number can
// be traced back to its records. No fare, price, margin or other money field
// is read or stored.
//
// Snapshots are append-only and keyed by (membership, period, formula
// version): v1 snapshots generated before v2 stay readable and unchanged.

// PerformanceFormulaVersion is the current formula.
const PerformanceFormulaVersion = "v2"

// maxExplainedAssignments bounds the per-assignment explanation list.
const maxExplainedAssignments = 500

// Assignment outcomes as counted by the formula.
const (
	OutcomeCompleted               = "COMPLETED"
	OutcomeInProgress              = "IN_PROGRESS"
	OutcomePendingResponse         = "PENDING_RESPONSE"
	OutcomeDeclined                = "DECLINED"
	OutcomeExpired                 = "EXPIRED"
	OutcomeCancelledBeforeResponse = "CANCELLED_BEFORE_RESPONSE"
	OutcomeCancelledAfterAccept    = "CANCELLED_AFTER_ACCEPTANCE"
	OutcomeOther                   = "OTHER"
)

// ExplainedAssignment is one assignment that contributed to a snapshot.
type ExplainedAssignment struct {
	AssignmentID    string `json:"assignmentId"`
	Origin          string `json:"origin"`
	SourceStatus    string `json:"sourceStatus"`
	Outcome         string `json:"outcome"`
	DeclineCategory string `json:"declineCategory,omitempty"`
	SafetyRelated   bool   `json:"safetyRelated,omitempty"`
}

// classifyAssignment maps an assignment record to one counted outcome.
func classifyAssignment(status string, accepted, declined, expired bool) string {
	switch {
	case status == "COMPLETED":
		return OutcomeCompleted
	case declined || status == "DECLINED":
		return OutcomeDeclined
	case status == "CANCELLED" && accepted:
		return OutcomeCancelledAfterAccept
	case status == "CANCELLED":
		return OutcomeCancelledBeforeResponse
	case expired || status == "EXPIRED":
		return OutcomeExpired
	case status == "CREATED" || status == "PENDING_PROVIDER_RESPONSE":
		return OutcomePendingResponse
	case accepted || status == "ACCEPTED" || status == "READY_FOR_EXECUTION" || status == "IN_PROGRESS" ||
		status == "EXECUTION_START_PENDING" || status == "RECONCILIATION_REQUIRED":
		return OutcomeInProgress
	default:
		return OutcomeOther
	}
}

// performanceMetrics is the v2 calculated_metrics document.
type performanceMetrics struct {
	CompanyAssignmentQuality struct {
		AssignmentsOffered               int `json:"assignmentsOffered"`
		MarketplaceAssignmentsOffered    int `json:"marketplaceAssignmentsOffered"`
		PrivateAssignmentsOffered        int `json:"privateAssignmentsOffered"`
		CancelledByCompanyBeforeResponse int `json:"cancelledByCompanyBeforeResponse"`
	} `json:"companyAssignmentQuality"`
	ProviderResponse struct {
		Accepted                  int `json:"accepted"`
		Declined                  int `json:"declined"`
		Expired                   int `json:"expired"`
		PendingResponse           int `json:"pendingResponse"`
		AcceptanceRateBasisPoints int `json:"acceptanceRateBasisPoints"`
	} `json:"providerResponse"`
	DeclineReasons map[string]int `json:"declineReasons"`
	Safety         struct {
		SafetyRelatedDeclines int `json:"safetyRelatedDeclines"`
	} `json:"safety"`
	ServiceQuality struct {
		CompletedAfterAcceptance int `json:"completedAfterAcceptance"`
		CancelledAfterAcceptance int `json:"cancelledAfterAcceptance"`
	} `json:"serviceQuality"`
	Productivity struct {
		JobsCompleted  int `json:"jobsCompleted"`
		JobsInProgress int `json:"jobsInProgress"`
	} `json:"productivity"`
	ByOrigin map[string]map[string]int `json:"byOrigin"`
}

func newPerformanceMetrics() *performanceMetrics {
	m := &performanceMetrics{
		DeclineReasons: map[string]int{"safety": 0, "operational": 0, "availability": 0, "personal": 0, "technical": 0, "other": 0},
		ByOrigin:       map[string]map[string]int{"MARKETPLACE": {}, "PRIVATE": {}},
	}
	return m
}

func (m *performanceMetrics) count(a ExplainedAssignment) {
	m.CompanyAssignmentQuality.AssignmentsOffered++
	if a.Origin == "MARKETPLACE" {
		m.CompanyAssignmentQuality.MarketplaceAssignmentsOffered++
	} else {
		m.CompanyAssignmentQuality.PrivateAssignmentsOffered++
	}
	m.ByOrigin[a.Origin][a.Outcome]++
	switch a.Outcome {
	case OutcomeCompleted:
		m.ProviderResponse.Accepted++
		m.ServiceQuality.CompletedAfterAcceptance++
		m.Productivity.JobsCompleted++
	case OutcomeInProgress:
		m.ProviderResponse.Accepted++
		m.Productivity.JobsInProgress++
	case OutcomeCancelledAfterAccept:
		m.ProviderResponse.Accepted++
		m.ServiceQuality.CancelledAfterAcceptance++
	case OutcomeDeclined:
		m.ProviderResponse.Declined++
		m.DeclineReasons[a.DeclineCategory]++
		if a.SafetyRelated {
			m.Safety.SafetyRelatedDeclines++
		}
	case OutcomeExpired:
		m.ProviderResponse.Expired++
	case OutcomePendingResponse:
		m.ProviderResponse.PendingResponse++
	case OutcomeCancelledBeforeResponse:
		m.CompanyAssignmentQuality.CancelledByCompanyBeforeResponse++
	}
}

func (m *performanceMetrics) finish() {
	answered := m.ProviderResponse.Accepted + m.ProviderResponse.Declined + m.ProviderResponse.Expired
	if answered > 0 {
		m.ProviderResponse.AcceptanceRateBasisPoints = m.ProviderResponse.Accepted * 10000 / answered
	}
}

var performanceExplanation = map[string]any{
	"formula": "v2: every marketplace and private company assignment created in [periodStart, periodEnd) for the " +
		"membership is classified into exactly one outcome and counted. Accepted = completed + in progress + " +
		"cancelled after acceptance. acceptanceRateBasisPoints = accepted * 10000 / (accepted + declined + expired).",
	"categories": map[string]string{
		"companyAssignmentQuality": "What the company offered, and offers the company itself withdrew before the provider answered.",
		"providerResponse":         "How the provider answered: accepted, declined and expired are counted separately.",
		"safety":                   "Declines flagged safety-related. Counted for contextual review only.",
		"serviceQuality":           "Accepted work that completed versus accepted work that was later cancelled.",
		"productivity":             "Completed and in-progress company work.",
	},
	"declinesExpiriesAndCancellationsAreDistinct": true,
	"ranking":                     false,
	"disciplinaryAction":          false,
	"automaticPunishment":         false,
	"safetyDeclinesAreMisconduct": false,
	"moneyFieldsRead":             false,
}

// GeneratePerformance computes (or replays) the snapshot of one membership
// over [start, end).
func (s *Service) GeneratePerformance(ctx context.Context, t tenant.Context, membership string, start, end time.Time) (PerformanceSnapshot, error) {
	if e := s.gate(s.o.Flags.Performance, true); e != nil {
		return PerformanceSnapshot{}, e
	}
	if !end.After(start) {
		return PerformanceSnapshot{}, apperrors.BadRequest("Valid performance period required.", nil)
	}
	rows, e := s.o.Pool.Query(ctx, `
		SELECT 'MARKETPLACE', a.id, a.status, a.accepted_at IS NOT NULL, a.declined_at IS NOT NULL OR r.id IS NOT NULL,
		       a.expired_at IS NOT NULL, COALESCE(r.reason_code, ''), COALESCE(r.safety_related, false), a.version
		FROM fleet_partner_provider_assignments a
		LEFT JOIN fleet_partner_assignment_responses r ON r.assignment_id = a.id AND r.response = 'DECLINED'
		WHERE a.organization_id = $1 AND a.membership_id = $2 AND a.created_at >= $3 AND a.created_at < $4
		UNION ALL
		SELECT 'PRIVATE', p.id, p.status, p.accepted_at IS NOT NULL, p.declined_at IS NOT NULL,
		       p.expired_at IS NOT NULL, p.decline_reason, COALESCE(d.safety_protected, false), p.version
		FROM fleet_partner_private_assignments p
		LEFT JOIN fleet_partner_private_assignment_decisions d ON d.assignment_id = p.id AND d.decision = 'DECLINE'
		WHERE p.organization_id = $1 AND p.provider_membership_id = $2 AND p.created_at >= $3 AND p.created_at < $4
		ORDER BY 1, 2`, t.OrganizationID, membership, start, end)
	if e != nil {
		return PerformanceSnapshot{}, e
	}
	metrics := newPerformanceMetrics()
	explained := []ExplainedAssignment{}
	maxVersion := map[string]int64{"MARKETPLACE": 0, "PRIVATE": 0}
	counts := map[string]int{"MARKETPLACE": 0, "PRIVATE": 0}
	for rows.Next() {
		var a ExplainedAssignment
		var accepted, declined, expired bool
		var reason string
		var version int64
		if e = rows.Scan(&a.Origin, &a.AssignmentID, &a.SourceStatus, &accepted, &declined, &expired, &reason, &a.SafetyRelated, &version); e != nil {
			rows.Close()
			return PerformanceSnapshot{}, e
		}
		a.Outcome = classifyAssignment(a.SourceStatus, accepted, declined, expired)
		if a.Outcome == OutcomeDeclined {
			category := declineCategory(reason)
			if category == "Safety" {
				a.SafetyRelated = true
			}
			a.DeclineCategory = map[string]string{"Safety": "safety", "Operational": "operational", "Availability": "availability",
				"Personal": "personal", "Technical": "technical", "Other": "other"}[category]
		} else {
			a.SafetyRelated = false
		}
		metrics.count(a)
		counts[a.Origin]++
		if version > maxVersion[a.Origin] {
			maxVersion[a.Origin] = version
		}
		if len(explained) < maxExplainedAssignments {
			explained = append(explained, a)
		}
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return PerformanceSnapshot{}, e
	}
	metrics.finish()

	inputs, _ := json.Marshal(map[string]any{
		"membershipId": membership, "periodStart": start, "periodEnd": end,
		"sources": []string{"fleet_partner_provider_assignments+fleet_partner_assignment_responses",
			"fleet_partner_private_assignments+fleet_partner_private_assignment_decisions"},
		"declineCategories":      []string{"safety", "operational", "availability", "personal", "technical", "other"},
		"assignments":            explained,
		"assignmentListComplete": len(explained) == counts["MARKETPLACE"]+counts["PRIVATE"],
		"automaticPunishment":    false,
	})
	calc, _ := json.Marshal(metrics)
	explain, _ := json.Marshal(performanceExplanation)
	versions, _ := json.Marshal(map[string]any{"formula": 2,
		"marketplaceAssignmentMaxVersion": maxVersion["MARKETPLACE"], "marketplaceAssignmentCount": counts["MARKETPLACE"],
		"privateAssignmentMaxVersion": maxVersion["PRIVATE"], "privateAssignmentCount": counts["PRIVATE"]})
	v := PerformanceSnapshot{ID: ids.New("pfs"), OrganizationID: t.OrganizationID, MembershipID: membership,
		PeriodStart: start, PeriodEnd: end, MetricInputs: inputs, CalculatedMetrics: calc, Explanation: explain,
		SourceVersions: versions, GenerationStatus: "COMPLETE", FormulaVersion: PerformanceFormulaVersion, Version: 1}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var inserted bool
		e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_performance_snapshots(id,organization_id,membership_id,period_start,period_end,metric_inputs,calculated_metrics,explanation,source_versions,generation_status,formula_version)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)ON CONFLICT(organization_id,membership_id,period_start,period_end,formula_version)DO NOTHING RETURNING id,generated_at,true`, v.ID, v.OrganizationID, membership, start, end, inputs, calc, explain, versions, v.GenerationStatus, v.FormulaVersion).Scan(&v.ID, &v.GeneratedAt, &inserted)
		if errors.Is(e, pgx.ErrNoRows) {
			// Already generated: replay the stored snapshot exactly, never a
			// recomputation that could differ from what was recorded.
			return tx.QueryRow(ctx, `SELECT `+performanceColumns+` FROM fleet_partner_performance_snapshots WHERE organization_id=$1 AND membership_id=$2 AND period_start=$3 AND period_end=$4 AND formula_version=$5`, t.OrganizationID, membership, start, end, v.FormulaVersion).Scan(performanceScanTargets(&v)...)
		}
		if e != nil {
			return e
		}
		if inserted {
			return s.record(ctx, tx, t, "performance_snapshot.generated.v1", "performance_snapshot", v.ID, 1, v)
		}
		return nil
	})
	return v, e
}

const performanceColumns = `id,organization_id,membership_id,period_start,period_end,metric_inputs,calculated_metrics,explanation,source_versions,generation_status,formula_version,generated_at,version`

func performanceScanTargets(v *PerformanceSnapshot) []any {
	return []any{&v.ID, &v.OrganizationID, &v.MembershipID, &v.PeriodStart, &v.PeriodEnd, &v.MetricInputs,
		&v.CalculatedMetrics, &v.Explanation, &v.SourceVersions, &v.GenerationStatus, &v.FormulaVersion, &v.GeneratedAt, &v.Version}
}
