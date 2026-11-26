package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// The recommendation models are deterministic rules registered in
// fleet_partner_model_versions. An ML model would plug in behind the same
// Recommendation shape with kind=ML; none is deployed, and nothing here
// claims otherwise. Every output carries its model and version, a
// confidence, the inputs it used (by reference and hash) and a plain
// explanation, and is only ever a proposal: accepting one records an action
// DRAFT for a human to carry out through the normal, authorized command. No
// recommendation changes an assignment, money, safety or compliance state.

func inputsHash(v map[string]any) string {
	var b strings.Builder
	for _, k := range models.SortedKeys(v) {
		fmt.Fprintf(&b, "%s=%v;", k, v[k])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func (s *Service) runModels(ctx context.Context, organizationID string, types map[string]bool) ([]models.Recommendation, error) {
	var out []models.Recommendation
	if types["PREVENTIVE_MAINTENANCE"] {
		rows, err := s.o.Store.MaintenanceCandidates(ctx, organizationID)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			severity := toString(r["severity"])
			confidence := 0.75
			if severity == "CRITICAL" {
				confidence = 0.9
			}
			out = append(out, models.Recommendation{
				Type: "PREVENTIVE_MAINTENANCE", SubjectType: "ASSET_DEFECT", SubjectID: toString(r["defectId"]),
				ModelCode: "maintenance-defect-rules", ModelVersion: "1", Confidence: confidence,
				Explanation:    map[string]any{"rule": "Open " + severity + " defect has no work order.", "severity": severity, "assetId": r["assetId"], "reportedAt": r["reportedAt"]},
				InputReference: map[string]any{"table": "fleet_partner_asset_defects", "defectId": r["defectId"], "assetId": r["assetId"], "severity": severity},
				ActionDraft:    map[string]any{"type": "WORK_ORDER_DRAFT", "assetId": r["assetId"], "defectId": r["defectId"], "executeVia": "POST /fleet-partner/v1/maintenance/work-orders"},
			})
		}
	}
	if types["ANOMALY_REVIEW"] {
		rows, err := s.o.Store.FuelOdometerAnomalies(ctx, organizationID)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, models.Recommendation{
				Type: "ANOMALY_REVIEW", SubjectType: "FUEL_RECORD", SubjectID: toString(r["fuelRecordId"]),
				ModelCode: "fuel-odometer-anomaly-rules", ModelVersion: "1", Confidence: 0.95,
				Explanation:    map[string]any{"rule": "Odometer is lower than the previous fuel record for the same asset.", "odometer": r["odometer"], "previousOdometer": r["previousOdometer"], "humanReviewOnly": true},
				InputReference: map[string]any{"table": "fleet_partner_fuel_records", "fuelRecordId": r["fuelRecordId"], "previousFuelRecordId": r["previousFuelRecordId"], "assetId": r["assetId"]},
				ActionDraft:    map[string]any{"type": "REVIEW_FUEL_RECORD", "fuelRecordId": r["fuelRecordId"], "note": "Review only; no deduction or sanction follows automatically."},
			})
		}
	}
	if types["DELAY_RISK"] {
		rows, err := s.o.Store.StaleLiveTrips(ctx, organizationID)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, models.Recommendation{
				Type: "DELAY_RISK", SubjectType: "TRIP_PROJECTION", SubjectID: toString(r["tripProjectionId"]),
				ModelCode: "trip-freshness-delay-rules", ModelVersion: "1", Confidence: 0.6,
				Explanation:    map[string]any{"rule": "A live trip's projection is not current; its progress cannot be confirmed.", "tripStatus": r["tripStatus"], "freshnessState": r["freshnessState"]},
				InputReference: map[string]any{"table": "fleet_partner_trip_projections", "tripProjectionId": r["tripProjectionId"], "sourceService": r["sourceService"]},
				ActionDraft:    map[string]any{"type": "CONTACT_PROVIDER", "tripProjectionId": r["tripProjectionId"]},
			})
		}
	}
	return out, nil
}

// GenerateRecommendations runs the selected models now and records new
// recommendations; a live recommendation for the same subject and model
// version is not duplicated.
func (s *Service) GenerateRecommendations(ctx context.Context, t tenant.Context, in models.GenerateInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Recommendations, true); err != nil {
		return nil, err
	}
	types := map[string]bool{}
	for _, x := range in.Types {
		x = strings.ToUpper(strings.TrimSpace(x))
		if !models.RecommendationTypes[x] {
			return nil, apperrors.Validation("Unsupported recommendation type: "+x, nil)
		}
		types[x] = true
	}
	if len(types) == 0 {
		for k := range models.RecommendationTypes {
			types[k] = true
		}
	}
	created, err := s.generateFor(ctx, t, types)
	if err != nil {
		return nil, err
	}
	return map[string]any{"created": created}, nil
}

func (s *Service) generateFor(ctx context.Context, t tenant.Context, types map[string]bool) (int, error) {
	recs, err := s.runModels(ctx, t.OrganizationID, types)
	if err != nil {
		return 0, err
	}
	created := 0
	err = s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		for _, r := range recs {
			id := ids.New("rec")
			ok, err := s.o.Store.InsertRecommendation(ctx, tx, id, t.OrganizationID, r, inputsHash(r.InputReference), s.o.Now().Add(s.o.RecommendationTTL))
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			created++
			if err = s.record(ctx, tx, t, "recommendation.created.v1", "recommendation", id, 1, map[string]any{"recommendationId": id, "type": r.Type, "modelCode": r.ModelCode, "modelVersion": r.ModelVersion, "confidence": r.Confidence}); err != nil {
				return err
			}
		}
		return nil
	})
	return created, err
}

func (s *Service) ListRecommendations(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Recommendations, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListRecommendations(ctx, t.OrganizationID)
}

func (s *Service) GetRecommendation(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Recommendations, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetRecommendation(ctx, t.OrganizationID, id)
}

func (s *Service) ListModelVersions(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Recommendations, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListModelVersions(ctx)
}

// DecideRecommendation records a human ACCEPT or DISMISS. Acceptance keeps
// the stored action draft for a person to execute through the normal
// authorized command; it executes nothing itself.
func (s *Service) DecideRecommendation(ctx context.Context, t tenant.Context, id string, in models.RecommendationDecision) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Recommendations, true); err != nil {
		return nil, err
	}
	decision := strings.ToUpper(strings.TrimSpace(in.Decision))
	status := map[string]string{"ACCEPT": "ACCEPTED", "DISMISS": "DISMISSED"}[decision]
	if status == "" {
		return nil, apperrors.Validation("decision must be ACCEPT or DISMISS.", nil)
	}
	if in.ExpectedVersion <= 0 {
		return nil, apperrors.Validation("expectedVersion is required.", nil)
	}
	if len(in.Reason) > 1000 {
		return nil, apperrors.Validation("reason must be at most 1000 characters.", nil)
	}
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		current, version, expires, err := s.o.Store.LockRecommendation(ctx, tx, t.OrganizationID, id)
		if err == pgx.ErrNoRows {
			return apperrors.NotFound("Resource not found.", nil)
		}
		if err != nil {
			return err
		}
		if version != in.ExpectedVersion {
			return conflict("version_conflict", "Recommendation version changed.")
		}
		if current != "OPEN" || !expires.After(s.o.Now()) {
			return conflict("state_transition_invalid", "Only an open, unexpired recommendation can be decided.")
		}
		v, err := s.o.Store.DecideRecommendation(ctx, tx, t.OrganizationID, id, status, t.ActorID, in.Reason)
		if err != nil {
			return err
		}
		event := "recommendation.accepted.v1"
		if status == "DISMISSED" {
			event = "recommendation.rejected.v1"
		}
		return s.record(ctx, tx, t, event, "recommendation", id, v, map[string]any{"recommendationId": id, "status": status})
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetRecommendation(ctx, t.OrganizationID, id)
}

// encode is a small helper for JSON payloads.
func encode(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
