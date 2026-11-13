package assets

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 18.12 asset lifecycle gaps (spec 47.3, 48.3): provider
// unassignment, meter readings, asset defects, tyres, operating-cost history
// and asset history.

// Meter anomaly thresholds. An anomalous reading is still recorded (spec 48.2:
// review flags, never silent rejection of a valid emergency); it is flagged
// and raises a review alert.
const (
	maxOdometerRatePerHour  = 150.0  // distance units per elapsed hour
	maxOdometerSingleJump   = 5000.0 // distance units in one reading
	maxEngineHoursSlack     = 0.25   // engine hours beyond elapsed wall-clock hours
	maxEngineHoursSingleJmp = 500.0
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func uniqueViolation(e error) bool {
	var pg *pgconn.PgError
	return errors.As(e, &pg) && pg.Code == "23505"
}

func (s *Service) assetHistory(ctx context.Context, tx pgx.Tx, t tenant.Context, assetID, prevOp, newOp, life, reason string, version int64, snapshot any) error {
	_, e := tx.Exec(ctx, `INSERT INTO fleet_partner_asset_history(id,organization_id,asset_id,previous_operational_status,new_operational_status,previous_lifecycle_status,new_lifecycle_status,reason,actor_id,version,snapshot)VALUES($1,$2,$3,$4,$5,$6,$6,$7,$8,$9,$10)`, ids.New("ash"), t.OrganizationID, assetID, prevOp, newOp, life, reason, t.ActorID, version, mustJSON(snapshot))
	return e
}

// ---------------------------------------------------------------------------
// Provider unassignment (spec 47.3 POST /assets/{id}/unassign-provider)
// ---------------------------------------------------------------------------

type UnassignInput struct {
	AssignmentID    string `json:"assignmentId"`
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

// UnassignProvider ends a provider-to-asset allocation. It never releases a
// safety block: an asset that is BLOCKED (or otherwise out of service) keeps
// that status; only an ASSIGNED asset with no other active allocation returns
// to AVAILABLE. A provider with active company work on the asset cannot be
// unassigned from it.
func (s *Service) UnassignProvider(ctx context.Context, t tenant.Context, assetID string, in UnassignInput) (Assignment, error) {
	if e := s.gate(s.o.Flags.Assignments, true); e != nil {
		return Assignment{}, e
	}
	a, e := s.GetAsset(ctx, t, assetID)
	if e != nil {
		return Assignment{}, e
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || in.ExpectedVersion <= 0 {
		return Assignment{}, apperrors.BadRequest("reason and expectedVersion are required.", nil)
	}
	var out Assignment
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var version int64
		var operational, lifecycle string
		if x := tx.QueryRow(ctx, `SELECT version,operational_status,lifecycle_status FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, assetID).Scan(&version, &operational, &lifecycle); x != nil {
			return x
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Asset version conflict.", nil)
		}
		x := tx.QueryRow(ctx, `SELECT id,organization_id,asset_id,provider_membership_id,COALESCE(shift_id,''),assignment_type,start_at,end_at,status,version,created_at,updated_at FROM fleet_partner_asset_assignments WHERE organization_id=$1 AND asset_id=$2 AND status IN('PLANNED','ACTIVE') AND($3=''OR id=$3) ORDER BY(status='ACTIVE')DESC,start_at LIMIT 1 FOR UPDATE`, t.OrganizationID, assetID, in.AssignmentID).Scan(&out.ID, &out.OrganizationID, &out.AssetID, &out.ProviderMembershipID, &out.ShiftID, &out.AssignmentType, &out.StartAt, &out.EndAt, &out.Status, &out.Version, &out.CreatedAt, &out.UpdatedAt)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.NotFound("No current provider assignment for this asset.", nil)
		}
		if x != nil {
			return x
		}
		var activeWork bool
		if x = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_private_assignments WHERE organization_id=$1 AND provider_membership_id=$2 AND vehicle_or_truck_reference IN($3,NULLIF($4,'')) AND status IN('ACCEPTED','READY_FOR_EXECUTION','IN_PROGRESS'))
OR EXISTS(SELECT 1 FROM fleet_partner_provider_assignments WHERE organization_id=$1 AND membership_id=$2 AND asset_reference IN($3,NULLIF($4,'')) AND status IN('ACCEPTED','READY_FOR_EXECUTION','IN_PROGRESS','EXECUTION_START_PENDING'))`, t.OrganizationID, out.ProviderMembershipID, assetID, a.PlatformVehicleID).Scan(&activeWork); x != nil {
			return x
		}
		if activeWork {
			return apperrors.Conflict("The provider has active company work on this asset.", nil)
		}
		previous := out.Status
		if x = tx.QueryRow(ctx, `UPDATE fleet_partner_asset_assignments SET status=CASE WHEN start_at<now() THEN 'COMPLETED' ELSE 'CANCELLED' END,end_at=CASE WHEN start_at<now() AND(end_at IS NULL OR end_at>now()) THEN now() ELSE end_at END,ended_by=$3,end_reason=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING status,end_at,version,updated_at`, t.OrganizationID, out.ID, t.ActorID, in.Reason).Scan(&out.Status, &out.EndAt, &out.Version, &out.UpdatedAt); x != nil {
			return x
		}
		next := operational
		var otherActive bool
		if x = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_assignments WHERE organization_id=$1 AND asset_id=$2 AND status='ACTIVE')`, t.OrganizationID, assetID).Scan(&otherActive); x != nil {
			return x
		}
		if operational == "ASSIGNED" && !otherActive {
			next = "AVAILABLE"
		}
		var assetVersion int64
		if x = tx.QueryRow(ctx, `UPDATE fleet_partner_assets SET operational_status=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, assetID, next).Scan(&assetVersion); x != nil {
			return x
		}
		if x = s.assetHistory(ctx, tx, t, assetID, operational, next, lifecycle, "UNASSIGN_PROVIDER", assetVersion, map[string]any{"assignmentId": out.ID, "previousAssignmentStatus": previous, "reason": in.Reason}); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "asset.unassigned.v1", "asset", assetID, assetVersion, map[string]any{"assetId": assetID, "assignmentId": out.ID, "providerMembershipId": out.ProviderMembershipID, "assignmentStatus": out.Status, "operationalStatus": next})
	})
	return out, e
}

// ---------------------------------------------------------------------------
// Meter readings (spec 47.3 POST /meter-readings)
// ---------------------------------------------------------------------------

type MeterReadingInput struct {
	AssetID         string    `json:"assetId"`
	MeterType       string    `json:"meterType"`
	Reading         float64   `json:"reading"`
	Source          string    `json:"source"`
	SourceReference string    `json:"sourceReference"`
	RecordedAt      time.Time `json:"recordedAt"`
}

type MeterReading struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organizationId"`
	AssetID         string    `json:"assetId"`
	MeterType       string    `json:"meterType"`
	Reading         float64   `json:"reading"`
	PreviousReading float64   `json:"previousReading"`
	Source          string    `json:"source"`
	SourceReference string    `json:"sourceReference,omitempty"`
	RecordedAt      time.Time `json:"recordedAt"`
	Anomaly         bool      `json:"anomaly"`
	AnomalyReasons  []string  `json:"anomalyReasons"`
	RecordedBy      string    `json:"recordedBy"`
	CreatedAt       time.Time `json:"createdAt"`
	DueScheduleIDs  []string  `json:"dueScheduleIds,omitempty"`
}

// MeterAnomalies is the pure anomaly rule set for one reading.
func MeterAnomalies(meterType string, previous, reading float64, previousAt *time.Time, at time.Time) []string {
	reasons := []string{}
	delta := reading - previous
	hours := -1.0
	if previousAt != nil {
		hours = at.Sub(*previousAt).Hours()
	}
	switch meterType {
	case "ODOMETER":
		if delta > maxOdometerSingleJump {
			reasons = append(reasons, "LARGE_SINGLE_JUMP")
		}
		if hours >= 0 && delta > 0 && delta > maxOdometerRatePerHour*hours {
			reasons = append(reasons, "IMPLAUSIBLE_DISTANCE_RATE")
		}
	case "ENGINE_HOURS":
		if delta > maxEngineHoursSingleJmp {
			reasons = append(reasons, "LARGE_SINGLE_JUMP")
		}
		if hours >= 0 && delta > hours+maxEngineHoursSlack {
			reasons = append(reasons, "ENGINE_HOURS_EXCEED_ELAPSED_TIME")
		}
	}
	return reasons
}

// RecordMeterReading appends a monotonic odometer or engine-hours reading,
// advances the asset meter that preventive maintenance schedules are
// evaluated against, and reports the schedules the reading made due.
func (s *Service) RecordMeterReading(ctx context.Context, t tenant.Context, in MeterReadingInput) (MeterReading, error) {
	if e := s.gate(s.o.Flags.Assets, true); e != nil {
		return MeterReading{}, e
	}
	in.MeterType = strings.ToUpper(strings.TrimSpace(in.MeterType))
	in.Source = strings.ToUpper(strings.TrimSpace(in.Source))
	if in.Source == "" {
		in.Source = "MANUAL"
	}
	if (in.MeterType != "ODOMETER" && in.MeterType != "ENGINE_HOURS") || in.Reading < 0 || !map[string]bool{"MANUAL": true, "INSPECTION": true, "FUEL_RECORD": true, "TELEMATICS": true}[in.Source] {
		return MeterReading{}, apperrors.BadRequest("meterType, reading or source is invalid.", nil)
	}
	now := s.o.Now()
	if in.RecordedAt.IsZero() {
		in.RecordedAt = now
	}
	if in.RecordedAt.After(now.Add(5 * time.Minute)) {
		return MeterReading{}, apperrors.BadRequest("recordedAt cannot be in the future.", nil)
	}
	if _, e := s.GetAsset(ctx, t, in.AssetID); e != nil {
		return MeterReading{}, e
	}
	var out MeterReading
	anomaly := false
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var operational, lifecycle string
		var odometer, hours float64
		if x := tx.QueryRow(ctx, `SELECT operational_status,lifecycle_status,odometer,operating_hours FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, in.AssetID).Scan(&operational, &lifecycle, &odometer, &hours); x != nil {
			return x
		}
		if lifecycle != "ACTIVE" {
			return apperrors.Conflict("Readings cannot be recorded for an archived or retired asset.", nil)
		}
		previous := odometer
		if in.MeterType == "ENGINE_HOURS" {
			previous = hours
		}
		if in.Reading < previous {
			return apperrors.Conflict("Meter readings cannot decrease.", nil)
		}
		var lastAt *time.Time
		if x := tx.QueryRow(ctx, `SELECT max(recorded_at) FROM fleet_partner_meter_readings WHERE organization_id=$1 AND asset_id=$2 AND meter_type=$3`, t.OrganizationID, in.AssetID, in.MeterType).Scan(&lastAt); x != nil {
			return x
		}
		if lastAt != nil && in.RecordedAt.Before(*lastAt) {
			return apperrors.Conflict("A later reading is already recorded for this meter.", nil)
		}
		reasons := MeterAnomalies(in.MeterType, previous, in.Reading, lastAt, in.RecordedAt)
		anomaly = len(reasons) > 0
		out = MeterReading{ID: ids.New("mtr"), OrganizationID: t.OrganizationID, AssetID: in.AssetID, MeterType: in.MeterType, Reading: in.Reading, PreviousReading: previous, Source: in.Source, SourceReference: in.SourceReference, RecordedAt: in.RecordedAt, Anomaly: anomaly, AnomalyReasons: reasons, RecordedBy: t.ActorID}
		if x := tx.QueryRow(ctx, `INSERT INTO fleet_partner_meter_readings(id,organization_id,asset_id,meter_type,reading,previous_reading,source,source_reference,recorded_at,anomaly,anomaly_reasons,recorded_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)RETURNING created_at`, out.ID, out.OrganizationID, out.AssetID, out.MeterType, out.Reading, out.PreviousReading, out.Source, out.SourceReference, out.RecordedAt, out.Anomaly, mustJSON(reasons), out.RecordedBy).Scan(&out.CreatedAt); x != nil {
			return x
		}
		column := "odometer"
		if in.MeterType == "ENGINE_HOURS" {
			column = "operating_hours"
		}
		var assetVersion int64
		if x := tx.QueryRow(ctx, `UPDATE fleet_partner_assets SET `+column+`=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, in.AssetID, in.Reading).Scan(&assetVersion); x != nil {
			return x
		}
		if x := s.assetHistory(ctx, tx, t, in.AssetID, operational, operational, lifecycle, "METER_READING", assetVersion, map[string]any{"meterReadingId": out.ID, "meterType": in.MeterType, "reading": in.Reading}); x != nil {
			return x
		}
		thresholdColumn := "next_due_odometer"
		if in.MeterType == "ENGINE_HOURS" {
			thresholdColumn = "next_due_hours"
		}
		rows, x := tx.Query(ctx, `SELECT id FROM fleet_partner_maintenance_schedules WHERE organization_id=$1 AND asset_id=$2 AND status='ACTIVE' AND trigger_type IN('MILEAGE','OPERATING_HOURS','COMBINED') AND `+thresholdColumn+` IS NOT NULL AND `+thresholdColumn+`<=$3 AND `+thresholdColumn+`>$4 ORDER BY id`, t.OrganizationID, in.AssetID, in.Reading, previous)
		if x != nil {
			return x
		}
		for rows.Next() {
			var id string
			if x = rows.Scan(&id); x != nil {
				rows.Close()
				return x
			}
			out.DueScheduleIDs = append(out.DueScheduleIDs, id)
		}
		rows.Close()
		if x = rows.Err(); x != nil {
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
	if e == nil && anomaly && s.o.Alerts != nil {
		_ = s.o.Alerts.CreateSystemAlert(ctx, t.OrganizationID, "METER_READING_ANOMALY", in.AssetID, out.ID, "MEDIUM", "Meter reading flagged for review")
	}
	return out, e
}

func (s *Service) ListMeterReadings(ctx context.Context, t tenant.Context, assetID, meterType string) ([]MeterReading, error) {
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,asset_id,meter_type,reading,previous_reading,source,source_reference,recorded_at,anomaly,anomaly_reasons,recorded_by,created_at FROM fleet_partner_meter_readings WHERE organization_id=$1 AND asset_id=$2 AND($3=''OR meter_type=$3) ORDER BY recorded_at DESC,created_at DESC LIMIT 200`, t.OrganizationID, assetID, strings.ToUpper(meterType))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []MeterReading{}
	for rows.Next() {
		var v MeterReading
		var reasons []byte
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.MeterType, &v.Reading, &v.PreviousReading, &v.Source, &v.SourceReference, &v.RecordedAt, &v.Anomaly, &reasons, &v.RecordedBy, &v.CreatedAt); e != nil {
			return nil, e
		}
		_ = json.Unmarshal(reasons, &v.AnomalyReasons)
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Asset history (spec 47.3 GET /assets/{id}/history)
// ---------------------------------------------------------------------------

type AssetHistoryEntry struct {
	ID                        string          `json:"id"`
	PreviousOperationalStatus string          `json:"previousOperationalStatus"`
	NewOperationalStatus      string          `json:"newOperationalStatus"`
	PreviousLifecycleStatus   string          `json:"previousLifecycleStatus"`
	NewLifecycleStatus        string          `json:"newLifecycleStatus"`
	Reason                    string          `json:"reason"`
	ActorID                   string          `json:"actorId"`
	Version                   int64           `json:"version"`
	Snapshot                  json.RawMessage `json:"snapshot"`
	OccurredAt                time.Time       `json:"occurredAt"`
}

func (s *Service) AssetHistory(ctx context.Context, t tenant.Context, assetID string) ([]AssetHistoryEntry, error) {
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,previous_operational_status,new_operational_status,previous_lifecycle_status,new_lifecycle_status,reason,actor_id,version,snapshot,occurred_at FROM fleet_partner_asset_history WHERE organization_id=$1 AND asset_id=$2 ORDER BY occurred_at DESC,version DESC LIMIT 200`, t.OrganizationID, assetID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []AssetHistoryEntry{}
	for rows.Next() {
		var v AssetHistoryEntry
		var snap []byte
		if e = rows.Scan(&v.ID, &v.PreviousOperationalStatus, &v.NewOperationalStatus, &v.PreviousLifecycleStatus, &v.NewLifecycleStatus, &v.Reason, &v.ActorID, &v.Version, &snap, &v.OccurredAt); e != nil {
			return nil, e
		}
		v.Snapshot = json.RawMessage(snap)
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Operating-cost history (spec 47.3 GET /assets/{id}/costs)
// ---------------------------------------------------------------------------

type CostInput struct {
	CostType        string    `json:"costType"`
	SourceType      string    `json:"sourceType"`
	SourceReference string    `json:"sourceReference"`
	AmountMinor     int64     `json:"amountMinor"`
	Currency        string    `json:"currency"`
	IncurredAt      time.Time `json:"incurredAt"`
	Description     string    `json:"description"`
}

type CostEntry struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organizationId"`
	AssetID         string    `json:"assetId"`
	CostType        string    `json:"costType"`
	SourceType      string    `json:"sourceType"`
	SourceReference string    `json:"sourceReference,omitempty"`
	AmountMinor     int64     `json:"amountMinor"`
	Currency        string    `json:"currency"`
	IncurredAt      time.Time `json:"incurredAt"`
	Description     string    `json:"description,omitempty"`
	RecordedBy      string    `json:"recordedBy"`
	CreatedAt       time.Time `json:"createdAt"`
}

type CostTotal struct {
	CostType    string `json:"costType"`
	Currency    string `json:"currency"`
	AmountMinor int64  `json:"amountMinor"`
}

type CostHistory struct {
	AssetID string      `json:"assetId"`
	Entries []CostEntry `json:"entries"`
	Totals  []CostTotal `json:"totals"`
}

// RecordCost appends an operating cost for an asset. Amounts are integer minor
// units with an ISO currency; this is organization reporting data and never a
// wallet or ledger mutation. A sourced cost must reference a record of the
// same asset in the same organization.
func (s *Service) RecordCost(ctx context.Context, t tenant.Context, assetID string, in CostInput) (CostEntry, error) {
	if e := s.gate(s.o.Flags.Assets, true); e != nil {
		return CostEntry{}, e
	}
	in.CostType = strings.ToUpper(strings.TrimSpace(in.CostType))
	in.SourceType = strings.ToUpper(strings.TrimSpace(in.SourceType))
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.SourceType == "" {
		in.SourceType = "MANUAL"
	}
	if !map[string]bool{"MAINTENANCE": true, "FUEL": true, "PARTS": true, "TYRES": true, "OTHER": true}[in.CostType] || !map[string]bool{"WORK_ORDER": true, "FUEL_RECORD": true, "STOCK_MOVEMENT": true, "TYRE": true, "MANUAL": true}[in.SourceType] {
		return CostEntry{}, apperrors.BadRequest("costType or sourceType is invalid.", nil)
	}
	if in.AmountMinor <= 0 || !currencyPattern.MatchString(in.Currency) {
		return CostEntry{}, apperrors.BadRequest("amountMinor must be a positive integer and currency an ISO 4217 code.", nil)
	}
	if in.SourceType != "MANUAL" && strings.TrimSpace(in.SourceReference) == "" {
		return CostEntry{}, apperrors.BadRequest("sourceReference is required for a sourced cost.", nil)
	}
	now := s.o.Now()
	if in.IncurredAt.IsZero() {
		in.IncurredAt = now
	}
	if in.IncurredAt.After(now.Add(5 * time.Minute)) {
		return CostEntry{}, apperrors.BadRequest("incurredAt cannot be in the future.", nil)
	}
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return CostEntry{}, e
	}
	var linked bool
	var e error
	switch in.SourceType {
	case "WORK_ORDER":
		e = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_work_orders WHERE organization_id=$1 AND id=$2 AND asset_id=$3)`, t.OrganizationID, in.SourceReference, assetID).Scan(&linked)
	case "FUEL_RECORD":
		e = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_fuel_records WHERE organization_id=$1 AND id=$2 AND asset_id=$3)`, t.OrganizationID, in.SourceReference, assetID).Scan(&linked)
	case "STOCK_MOVEMENT":
		e = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_stock_movements m JOIN fleet_partner_work_orders w ON w.organization_id=m.organization_id AND w.id=m.work_order_id WHERE m.organization_id=$1 AND m.id=$2 AND w.asset_id=$3)`, t.OrganizationID, in.SourceReference, assetID).Scan(&linked)
	case "TYRE":
		e = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_tyres WHERE organization_id=$1 AND id=$2 AND asset_id=$3) OR EXISTS(SELECT 1 FROM fleet_partner_tyre_events WHERE organization_id=$1 AND tyre_id=$2 AND asset_id=$3)`, t.OrganizationID, in.SourceReference, assetID).Scan(&linked)
	default:
		linked = true
	}
	if e != nil {
		return CostEntry{}, e
	}
	if !linked {
		return CostEntry{}, apperrors.NotFound("Cost source not found for this asset.", nil)
	}
	out := CostEntry{ID: ids.New("acs"), OrganizationID: t.OrganizationID, AssetID: assetID, CostType: in.CostType, SourceType: in.SourceType, SourceReference: strings.TrimSpace(in.SourceReference), AmountMinor: in.AmountMinor, Currency: in.Currency, IncurredAt: in.IncurredAt, Description: strings.TrimSpace(in.Description), RecordedBy: t.ActorID}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if x := tx.QueryRow(ctx, `INSERT INTO fleet_partner_asset_cost_entries(id,organization_id,asset_id,cost_type,source_type,source_reference,amount_minor,currency,incurred_at,description,recorded_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)RETURNING created_at`, out.ID, out.OrganizationID, out.AssetID, out.CostType, out.SourceType, out.SourceReference, out.AmountMinor, out.Currency, out.IncurredAt, out.Description, out.RecordedBy).Scan(&out.CreatedAt); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "asset_cost.recorded.v1", "asset_cost", out.ID, 1, map[string]any{"cost": out, "walletMutation": false, "ledgerMutation": false})
	})
	return out, e
}

func (s *Service) ListCosts(ctx context.Context, t tenant.Context, assetID string) (CostHistory, error) {
	if e := s.gate(s.o.Flags.Assets, false); e != nil {
		return CostHistory{}, e
	}
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return CostHistory{}, e
	}
	out := CostHistory{AssetID: assetID, Entries: []CostEntry{}, Totals: []CostTotal{}}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,asset_id,cost_type,source_type,source_reference,amount_minor,currency,incurred_at,description,recorded_by,created_at FROM fleet_partner_asset_cost_entries WHERE organization_id=$1 AND asset_id=$2 ORDER BY incurred_at DESC,created_at DESC LIMIT 500`, t.OrganizationID, assetID)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v CostEntry
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.CostType, &v.SourceType, &v.SourceReference, &v.AmountMinor, &v.Currency, &v.IncurredAt, &v.Description, &v.RecordedBy, &v.CreatedAt); e != nil {
			rows.Close()
			return out, e
		}
		out.Entries = append(out.Entries, v)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return out, e
	}
	totals, e := s.o.Pool.Query(ctx, `SELECT cost_type,currency,sum(amount_minor)::bigint FROM fleet_partner_asset_cost_entries WHERE organization_id=$1 AND asset_id=$2 GROUP BY cost_type,currency ORDER BY cost_type,currency`, t.OrganizationID, assetID)
	if e != nil {
		return out, e
	}
	defer totals.Close()
	for totals.Next() {
		var v CostTotal
		if e = totals.Scan(&v.CostType, &v.Currency, &v.AmountMinor); e != nil {
			return out, e
		}
		out.Totals = append(out.Totals, v)
	}
	return out, totals.Err()
}
