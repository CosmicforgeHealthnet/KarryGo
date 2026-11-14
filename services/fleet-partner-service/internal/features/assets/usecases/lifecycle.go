package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 18.12 asset lifecycle gaps (spec 47.3, 48.3): provider
// unassignment, meter readings, operating-cost history and asset history.

func (s *Service) assetHistory(ctx context.Context, tx pgx.Tx, t tenant.Context, assetID, prevOp, newOp, life, reason string, version int64, snapshot any) error {
	return s.repo.InsertAssetHistory(ctx, tx, t.OrganizationID, assetID, prevOp, newOp, life, life, reason, t.ActorID, version, snapshot)
}

// UnassignProvider ends a provider-to-asset allocation. It never releases a
// safety block: an asset that is BLOCKED (or otherwise out of service) keeps
// that status; only an ASSIGNED asset with no other active allocation returns
// to AVAILABLE. A provider with active company work on the asset cannot be
// unassigned from it.
func (s *Service) UnassignProvider(ctx context.Context, t tenant.Context, assetID string, in models.UnassignInput) (models.Assignment, error) {
	if e := s.gate(s.o.Flags.Assignments, true); e != nil {
		return models.Assignment{}, e
	}
	a, e := s.GetAsset(ctx, t, assetID)
	if e != nil {
		return models.Assignment{}, e
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || in.ExpectedVersion <= 0 {
		return models.Assignment{}, apperrors.BadRequest("reason and expectedVersion are required.", nil)
	}
	var out models.Assignment
	e = s.tx(ctx, func(tx pgx.Tx) error {
		lock, x := s.repo.LockAsset(ctx, tx, t.OrganizationID, assetID)
		if x != nil {
			return x
		}
		if lock.Version != in.ExpectedVersion {
			return apperrors.Conflict("Asset version conflict.", nil)
		}
		out, x = s.repo.LockCurrentAssignment(ctx, tx, t.OrganizationID, assetID, in.AssignmentID)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.NotFound("No current provider assignment for this asset.", nil)
		}
		if x != nil {
			return x
		}
		activeWork, x := s.repo.ProviderHasActiveWork(ctx, tx, t.OrganizationID, out.ProviderMembershipID, assetID, a.PlatformVehicleID)
		if x != nil {
			return x
		}
		if activeWork {
			return apperrors.Conflict("The provider has active company work on this asset.", nil)
		}
		previous := out.Status
		if x = s.repo.EndAssignment(ctx, tx, t.OrganizationID, t.ActorID, in.Reason, &out); x != nil {
			return x
		}
		next := lock.OperationalStatus
		otherActive, x := s.repo.ActiveAssignmentExists(ctx, tx, t.OrganizationID, assetID)
		if x != nil {
			return x
		}
		if lock.OperationalStatus == "ASSIGNED" && !otherActive {
			next = "AVAILABLE"
		}
		assetVersion, x := s.repo.SetOperationalStatus(ctx, tx, t.OrganizationID, assetID, next)
		if x != nil {
			return x
		}
		if x = s.assetHistory(ctx, tx, t, assetID, lock.OperationalStatus, next, lock.LifecycleStatus, "UNASSIGN_PROVIDER", assetVersion, map[string]any{"assignmentId": out.ID, "previousAssignmentStatus": previous, "reason": in.Reason}); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "asset.unassigned.v1", "asset", assetID, assetVersion, map[string]any{"assetId": assetID, "assignmentId": out.ID, "providerMembershipId": out.ProviderMembershipID, "assignmentStatus": out.Status, "operationalStatus": next})
	})
	return out, e
}

// RecordMeterReading appends a monotonic odometer or engine-hours reading,
// advances the asset meter that preventive maintenance schedules are
// evaluated against, and reports the schedules the reading made due.
func (s *Service) RecordMeterReading(ctx context.Context, t tenant.Context, in models.MeterReadingInput) (models.MeterReading, error) {
	if e := s.gate(s.o.Flags.Assets, true); e != nil {
		return models.MeterReading{}, e
	}
	in.MeterType = strings.ToUpper(strings.TrimSpace(in.MeterType))
	in.Source = strings.ToUpper(strings.TrimSpace(in.Source))
	if in.Source == "" {
		in.Source = "MANUAL"
	}
	if (in.MeterType != "ODOMETER" && in.MeterType != "ENGINE_HOURS") || in.Reading < 0 || !map[string]bool{"MANUAL": true, "INSPECTION": true, "FUEL_RECORD": true, "TELEMATICS": true}[in.Source] {
		return models.MeterReading{}, apperrors.BadRequest("meterType, reading or source is invalid.", nil)
	}
	now := s.o.Now()
	if in.RecordedAt.IsZero() {
		in.RecordedAt = now
	}
	if in.RecordedAt.After(now.Add(5 * time.Minute)) {
		return models.MeterReading{}, apperrors.BadRequest("recordedAt cannot be in the future.", nil)
	}
	if _, e := s.GetAsset(ctx, t, in.AssetID); e != nil {
		return models.MeterReading{}, e
	}
	var out models.MeterReading
	anomaly := false
	e := s.tx(ctx, func(tx pgx.Tx) error {
		lock, x := s.repo.LockAsset(ctx, tx, t.OrganizationID, in.AssetID)
		if x != nil {
			return x
		}
		if lock.LifecycleStatus != "ACTIVE" {
			return apperrors.Conflict("Readings cannot be recorded for an archived or retired asset.", nil)
		}
		previous := lock.Odometer
		if in.MeterType == "ENGINE_HOURS" {
			previous = lock.OperatingHours
		}
		if in.Reading < previous {
			return apperrors.Conflict("Meter readings cannot decrease.", nil)
		}
		lastAt, x := s.repo.LastMeterReadingAt(ctx, tx, t.OrganizationID, in.AssetID, in.MeterType)
		if x != nil {
			return x
		}
		if lastAt != nil && in.RecordedAt.Before(*lastAt) {
			return apperrors.Conflict("A later reading is already recorded for this meter.", nil)
		}
		reasons := models.MeterAnomalies(in.MeterType, previous, in.Reading, lastAt, in.RecordedAt)
		anomaly = len(reasons) > 0
		out = models.MeterReading{ID: ids.New("mtr"), OrganizationID: t.OrganizationID, AssetID: in.AssetID, MeterType: in.MeterType, Reading: in.Reading, PreviousReading: previous, Source: in.Source, SourceReference: in.SourceReference, RecordedAt: in.RecordedAt, Anomaly: anomaly, AnomalyReasons: reasons, RecordedBy: t.ActorID}
		if x = s.repo.InsertMeterReading(ctx, tx, &out); x != nil {
			return x
		}
		assetVersion, x := s.repo.SetMeter(ctx, tx, t.OrganizationID, in.AssetID, in.MeterType, in.Reading)
		if x != nil {
			return x
		}
		if x = s.assetHistory(ctx, tx, t, in.AssetID, lock.OperationalStatus, lock.OperationalStatus, lock.LifecycleStatus, "METER_READING", assetVersion, map[string]any{"meterReadingId": out.ID, "meterType": in.MeterType, "reading": in.Reading}); x != nil {
			return x
		}
		if out.DueScheduleIDs, x = s.repo.SchedulesCrossed(ctx, tx, t.OrganizationID, in.AssetID, in.MeterType, in.Reading, previous); x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "meter_reading.recorded.v1", "meter_reading", out.ID, 1, map[string]any{"meterReading": out, "dueScheduleIds": out.DueScheduleIDs}); x != nil {
			return x
		}
		if anomaly {
			return s.record(ctx, tx, t, "meter_reading.anomaly_detected.v1", "meter_reading", out.ID, 1, map[string]any{"meterReadingId": out.ID, "assetId": in.AssetID, "anomalyReasons": reasons})
		}
		return nil
	})
	if e == nil && anomaly {
		s.alert(ctx, t.OrganizationID, "METER_READING_ANOMALY", in.AssetID, out.ID, "MEDIUM", "Meter reading flagged for review")
	}
	return out, e
}

func (s *Service) ListMeterReadings(ctx context.Context, t tenant.Context, assetID, meterType string) ([]models.MeterReading, error) {
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return nil, e
	}
	return s.repo.ListMeterReadings(ctx, t.OrganizationID, assetID, meterType)
}

// AssetHistory is the asset's status and meter history (spec 47.3).
func (s *Service) AssetHistory(ctx context.Context, t tenant.Context, assetID string) ([]models.AssetHistoryEntry, error) {
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return nil, e
	}
	return s.repo.ListAssetHistory(ctx, t.OrganizationID, assetID)
}

// RecordCost appends an operating cost for an asset. Amounts are integer minor
// units with an ISO currency; this is organization reporting data and never a
// wallet or ledger mutation. A sourced cost must reference a record of the
// same asset in the same organization.
func (s *Service) RecordCost(ctx context.Context, t tenant.Context, assetID string, in models.CostInput) (models.CostEntry, error) {
	if e := s.gate(s.o.Flags.Assets, true); e != nil {
		return models.CostEntry{}, e
	}
	in.CostType = strings.ToUpper(strings.TrimSpace(in.CostType))
	in.SourceType = strings.ToUpper(strings.TrimSpace(in.SourceType))
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.SourceType == "" {
		in.SourceType = "MANUAL"
	}
	if !models.CostTypes[in.CostType] || !models.CostSourceTypes[in.SourceType] {
		return models.CostEntry{}, apperrors.BadRequest("costType or sourceType is invalid.", nil)
	}
	if in.AmountMinor <= 0 || !models.CurrencyPattern.MatchString(in.Currency) {
		return models.CostEntry{}, apperrors.BadRequest("amountMinor must be a positive integer and currency an ISO 4217 code.", nil)
	}
	if in.SourceType != "MANUAL" && strings.TrimSpace(in.SourceReference) == "" {
		return models.CostEntry{}, apperrors.BadRequest("sourceReference is required for a sourced cost.", nil)
	}
	now := s.o.Now()
	if in.IncurredAt.IsZero() {
		in.IncurredAt = now
	}
	if in.IncurredAt.After(now.Add(5 * time.Minute)) {
		return models.CostEntry{}, apperrors.BadRequest("incurredAt cannot be in the future.", nil)
	}
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return models.CostEntry{}, e
	}
	linked, e := s.repo.CostSourceLinked(ctx, t.OrganizationID, in.SourceType, in.SourceReference, assetID)
	if e != nil {
		return models.CostEntry{}, e
	}
	if !linked {
		return models.CostEntry{}, apperrors.NotFound("Cost source not found for this asset.", nil)
	}
	out := models.CostEntry{ID: ids.New("acs"), OrganizationID: t.OrganizationID, AssetID: assetID, CostType: in.CostType, SourceType: in.SourceType, SourceReference: strings.TrimSpace(in.SourceReference), AmountMinor: in.AmountMinor, Currency: in.Currency, IncurredAt: in.IncurredAt, Description: strings.TrimSpace(in.Description), RecordedBy: t.ActorID}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if x := s.repo.InsertCost(ctx, tx, &out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "asset_cost.recorded.v1", "asset_cost", out.ID, 1, map[string]any{"cost": out, "walletMutation": false, "ledgerMutation": false})
	})
	return out, e
}

func (s *Service) ListCosts(ctx context.Context, t tenant.Context, assetID string) (models.CostHistory, error) {
	if e := s.gate(s.o.Flags.Assets, false); e != nil {
		return models.CostHistory{}, e
	}
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return models.CostHistory{}, e
	}
	out := models.CostHistory{AssetID: assetID, Entries: []models.CostEntry{}, Totals: []models.CostTotal{}}
	entries, e := s.repo.ListCostEntries(ctx, t.OrganizationID, assetID)
	if e != nil {
		return out, e
	}
	out.Entries = entries
	totals, e := s.repo.CostTotals(ctx, t.OrganizationID, assetID)
	if e != nil {
		return out, e
	}
	out.Totals = totals
	return out, nil
}
