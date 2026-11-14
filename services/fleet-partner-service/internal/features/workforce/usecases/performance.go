package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// GeneratePerformance computes (or replays) the snapshot of one membership
// over [start, end). The formula itself lives in models (FLEET-17.10).
func (s *Service) GeneratePerformance(ctx context.Context, t tenant.Context, membership string, start, end time.Time) (models.PerformanceSnapshot, error) {
	if e := s.gate(s.o.Flags.Performance, true); e != nil {
		return models.PerformanceSnapshot{}, e
	}
	if !end.After(start) {
		return models.PerformanceSnapshot{}, apperrors.BadRequest("Valid performance period required.", nil)
	}
	records, e := s.repo.AssignmentRecords(ctx, s.o.Pool, t.OrganizationID, membership, start, end)
	if e != nil {
		return models.PerformanceSnapshot{}, e
	}
	metrics := models.NewPerformanceMetrics()
	explained := []models.ExplainedAssignment{}
	maxVersion := map[string]int64{"MARKETPLACE": 0, "PRIVATE": 0}
	counts := map[string]int{"MARKETPLACE": 0, "PRIVATE": 0}
	for _, r := range records {
		a := models.ExplainedAssignment{AssignmentID: r.AssignmentID, Origin: r.Origin, SourceStatus: r.SourceStatus, SafetyRelated: r.SafetyRelated}
		a.Outcome = models.ClassifyAssignment(a.SourceStatus, r.Accepted, r.Declined, r.Expired)
		if a.Outcome == models.OutcomeDeclined {
			category := models.DeclineCategory(r.Reason)
			if category == "Safety" {
				a.SafetyRelated = true
			}
			a.DeclineCategory = map[string]string{"Safety": "safety", "Operational": "operational", "Availability": "availability",
				"Personal": "personal", "Technical": "technical", "Other": "other"}[category]
		} else {
			a.SafetyRelated = false
		}
		metrics.Count(a)
		counts[a.Origin]++
		if r.Version > maxVersion[a.Origin] {
			maxVersion[a.Origin] = r.Version
		}
		if len(explained) < models.MaxExplainedAssignments {
			explained = append(explained, a)
		}
	}
	metrics.Finish()

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
	explain, _ := json.Marshal(models.PerformanceExplanation)
	versions, _ := json.Marshal(map[string]any{"formula": 2,
		"marketplaceAssignmentMaxVersion": maxVersion["MARKETPLACE"], "marketplaceAssignmentCount": counts["MARKETPLACE"],
		"privateAssignmentMaxVersion": maxVersion["PRIVATE"], "privateAssignmentCount": counts["PRIVATE"]})
	v := models.PerformanceSnapshot{ID: ids.New("pfs"), OrganizationID: t.OrganizationID, MembershipID: membership,
		PeriodStart: start, PeriodEnd: end, MetricInputs: inputs, CalculatedMetrics: calc, Explanation: explain,
		SourceVersions: versions, GenerationStatus: "COMPLETE", FormulaVersion: models.PerformanceFormulaVersion, Version: 1}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		stored, e := s.repo.InsertPerformanceSnapshot(ctx, tx, v)
		if errors.Is(e, pgx.ErrNoRows) {
			// Already generated: replay the stored snapshot exactly, never a
			// recomputation that could differ from what was recorded.
			replay, e := s.repo.GetPerformanceSnapshot(ctx, tx, t.OrganizationID, membership, start, end, v.FormulaVersion)
			v = replay
			return e
		}
		if e != nil {
			return e
		}
		v = stored
		return s.record(ctx, tx, t, "performance_snapshot.generated.v1", "performance_snapshot", v.ID, 1, v)
	})
	return v, e
}
