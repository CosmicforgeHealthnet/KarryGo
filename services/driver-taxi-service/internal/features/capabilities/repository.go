// Package capabilities owns Taxi vehicle capability facts, product
// eligibility, locally applied service-product policy snapshots and the
// hard-gate evaluation used by matching. It never prices anything.
package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
)

var (
	ErrNotFound = errors.New("capability resource not found")
	ErrConflict = errors.New("capability state conflict")
	ErrInvalid  = errors.New("capability request is invalid")
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// LatestPolicy returns the newest effective snapshot of a policy kind.
func (r *Repository) LatestPolicy(ctx context.Context, kind string, now time.Time) ([]byte, string, error) {
	var payload []byte
	var checksum string
	err := r.db.QueryRow(ctx, `SELECT payload_json, checksum FROM taxi_policy_snapshots WHERE policy_kind=$1 AND effective_at<=$2 ORDER BY effective_at DESC, applied_at DESC LIMIT 1`, kind, now).Scan(&payload, &checksum)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return payload, checksum, err
}

// InsertPolicy stores an immutable snapshot. Re-applying the same version
// with the same checksum is a no-op; a different body is a conflict.
func (r *Repository) InsertPolicy(ctx context.Context, kind, version, checksum string, effectiveAt time.Time, payload []byte) (bool, error) {
	tag, err := r.db.Exec(ctx, `INSERT INTO taxi_policy_snapshots(policy_kind,policy_version,payload_json,checksum,effective_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT (policy_kind, policy_version) DO NOTHING`, kind, version, payload, checksum, effectiveAt)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}
	var existing string
	if err = r.db.QueryRow(ctx, `SELECT checksum FROM taxi_policy_snapshots WHERE policy_kind=$1 AND policy_version=$2`, kind, version).Scan(&existing); err != nil {
		return false, err
	}
	if existing != checksum {
		return false, ErrConflict
	}
	return false, nil
}

const capabilityColumns = `v.id::text, v.provider_id::text, v.total_seats, v.passenger_capacity, v.capability_version,
	c.body_style, c.door_count, c.ac_status, c.luggage_class, c.luggage_volume_ml, c.wheelchair_accessible, c.child_seat_capable,
	c.pet_eligible, c.goods_max_weight_grams, c.goods_max_volume_ml, c.goods_max_length_mm, c.goods_max_width_mm, c.goods_max_height_mm,
	c.goods_max_package_count, c.upright_safe, c.fragile_safe, c.weather_protected, c.temperature_controlled, c.temperature_min_c,
	c.temperature_max_c, c.secure_cargo_area, c.cargo_separation_supported, c.observed_at, c.verified_at, COALESCE(c.version,0)`

func scanCapability(row pgx.Row) (string, capability.VehicleCapability, error) {
	var v capability.VehicleCapability
	var provider string
	var capabilityVersion int64
	var body, ac, luggage *string
	err := row.Scan(&v.VehicleID, &provider, &v.TotalSeats, &v.PassengerCapacity, &capabilityVersion, &body, &v.DoorCount, &ac, &luggage,
		&v.LuggageVolumeML, &v.WheelchairAccessible, &v.ChildSeatCapable, &v.PetEligible, &v.GoodsMaxWeightGrams, &v.GoodsMaxVolumeML,
		&v.GoodsMaxLengthMM, &v.GoodsMaxWidthMM, &v.GoodsMaxHeightMM, &v.GoodsMaxPackageCount, &v.UprightSafe, &v.FragileSafe,
		&v.WeatherProtected, &v.TemperatureControlled, &v.TemperatureMinC, &v.TemperatureMaxC, &v.SecureCargoArea,
		&v.CargoSeparationSupported, &v.ObservedAt, &v.VerifiedAt, &v.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", v, ErrNotFound
	}
	if body != nil {
		v.BodyStyle = *body
	}
	if ac != nil {
		v.ACStatus = *ac
	}
	if luggage != nil {
		v.LuggageClass = *luggage
	}
	return provider, v, err
}

// GetCapability returns a vehicle's capability. When providerID is non-empty
// the vehicle must belong to that provider.
func (r *Repository) GetCapability(ctx context.Context, vehicleID, providerID string) (capability.VehicleCapability, error) {
	owner, v, err := scanCapability(r.db.QueryRow(ctx, `SELECT `+capabilityColumns+` FROM vehicles v LEFT JOIN taxi_vehicle_capabilities c ON c.vehicle_id=v.id WHERE v.id::text=$1`, vehicleID))
	if err != nil {
		return v, err
	}
	if providerID != "" && owner != providerID {
		return capability.VehicleCapability{}, ErrNotFound
	}
	return v, nil
}

// PutProviderCapability stores provider-declared capability. Changing any
// certification-sensitive field clears verification and moves ELIGIBLE
// products back to PENDING review in the same transaction.
func (r *Repository) PutProviderCapability(ctx context.Context, providerID string, input capability.VehicleCapability, expectedVersion int64, sensitive []string) (capability.VehicleCapability, []string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return capability.VehicleCapability{}, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	owner, current, err := scanCapability(tx.QueryRow(ctx, `SELECT `+capabilityColumns+` FROM vehicles v LEFT JOIN taxi_vehicle_capabilities c ON c.vehicle_id=v.id WHERE v.id::text=$1 FOR UPDATE OF v`, input.VehicleID))
	if err != nil {
		return capability.VehicleCapability{}, nil, err
	}
	if owner != providerID {
		return capability.VehicleCapability{}, nil, ErrNotFound
	}
	if current.Version != expectedVersion {
		return capability.VehicleCapability{}, nil, ErrConflict
	}
	changed := ChangedSensitiveFields(current, input, sensitive)
	if _, err = tx.Exec(ctx, `UPDATE vehicles SET total_seats=$2, passenger_capacity=$3, capability_version=capability_version+1, capability_updated_at=now(), updated_at=now() WHERE id=$1`, input.VehicleID, input.TotalSeats, input.PassengerCapacity); err != nil {
		return capability.VehicleCapability{}, nil, err
	}
	verifiedClause := `verified_at`
	if len(changed) > 0 {
		verifiedClause = `NULL`
	}
	_, err = tx.Exec(ctx, `INSERT INTO taxi_vehicle_capabilities(vehicle_id,body_style,door_count,ac_status,luggage_class,luggage_volume_ml,wheelchair_accessible,child_seat_capable,pet_eligible,
			goods_max_weight_grams,goods_max_volume_ml,goods_max_length_mm,goods_max_width_mm,goods_max_height_mm,goods_max_package_count,upright_safe,fragile_safe,weather_protected,
			temperature_controlled,temperature_min_c,temperature_max_c,secure_cargo_area,cargo_separation_supported,observed_at)
		VALUES($1,NULLIF($2,''),$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,now())
		ON CONFLICT (vehicle_id) DO UPDATE SET body_style=EXCLUDED.body_style,door_count=EXCLUDED.door_count,ac_status=EXCLUDED.ac_status,luggage_class=EXCLUDED.luggage_class,
			luggage_volume_ml=EXCLUDED.luggage_volume_ml,wheelchair_accessible=EXCLUDED.wheelchair_accessible,child_seat_capable=EXCLUDED.child_seat_capable,pet_eligible=EXCLUDED.pet_eligible,
			goods_max_weight_grams=EXCLUDED.goods_max_weight_grams,goods_max_volume_ml=EXCLUDED.goods_max_volume_ml,goods_max_length_mm=EXCLUDED.goods_max_length_mm,
			goods_max_width_mm=EXCLUDED.goods_max_width_mm,goods_max_height_mm=EXCLUDED.goods_max_height_mm,goods_max_package_count=EXCLUDED.goods_max_package_count,
			upright_safe=EXCLUDED.upright_safe,fragile_safe=EXCLUDED.fragile_safe,weather_protected=EXCLUDED.weather_protected,temperature_controlled=EXCLUDED.temperature_controlled,
			temperature_min_c=EXCLUDED.temperature_min_c,temperature_max_c=EXCLUDED.temperature_max_c,secure_cargo_area=EXCLUDED.secure_cargo_area,
			cargo_separation_supported=EXCLUDED.cargo_separation_supported,observed_at=now(),verified_at=`+verifiedClause+`,version=taxi_vehicle_capabilities.version+1,updated_at=now()`,
		input.VehicleID, input.BodyStyle, input.DoorCount, input.ACStatus, input.LuggageClass, input.LuggageVolumeML, input.WheelchairAccessible, input.ChildSeatCapable,
		input.PetEligible, input.GoodsMaxWeightGrams, input.GoodsMaxVolumeML, input.GoodsMaxLengthMM, input.GoodsMaxWidthMM, input.GoodsMaxHeightMM, input.GoodsMaxPackageCount,
		input.UprightSafe, input.FragileSafe, input.WeatherProtected, input.TemperatureControlled, input.TemperatureMinC, input.TemperatureMaxC, input.SecureCargoArea,
		input.CargoSeparationSupported)
	if err != nil {
		return capability.VehicleCapability{}, nil, err
	}
	var suspended []string
	if len(changed) > 0 {
		rows, err := tx.Query(ctx, `UPDATE taxi_vehicle_product_eligibility SET status='PENDING',reason_code='CAPABILITY_CHANGED_PENDING_REVIEW',version=version+1,updated_at=now() WHERE vehicle_id=$1 AND status='ELIGIBLE' RETURNING product_code`, input.VehicleID)
		if err != nil {
			return capability.VehicleCapability{}, nil, err
		}
		for rows.Next() {
			var code string
			if err = rows.Scan(&code); err != nil {
				rows.Close()
				return capability.VehicleCapability{}, nil, err
			}
			suspended = append(suspended, code)
		}
		rows.Close()
	}
	event := map[string]any{"vehicle_id": input.VehicleID, "changed_certification_fields": changed, "products_pending_review": suspended}
	if err = writeEvent(ctx, tx, input.VehicleID, "taxi.vehicle.capability_changed", event); err != nil {
		return capability.VehicleCapability{}, nil, err
	}
	if len(suspended) > 0 {
		if err = writeEvent(ctx, tx, input.VehicleID, "taxi.vehicle.product_eligibility_changed", event); err != nil {
			return capability.VehicleCapability{}, nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return capability.VehicleCapability{}, nil, err
	}
	saved, err := r.GetCapability(ctx, input.VehicleID, providerID)
	return saved, suspended, err
}

// ChangedSensitiveFields lists certification-sensitive fields whose value
// differs between the stored and submitted capability.
func ChangedSensitiveFields(current, next capability.VehicleCapability, sensitive []string) []string {
	a, _ := json.Marshal(current)
	b, _ := json.Marshal(next)
	var before, after map[string]json.RawMessage
	_ = json.Unmarshal(a, &before)
	_ = json.Unmarshal(b, &after)
	var changed []string
	for _, field := range sensitive {
		if string(before[field]) != string(after[field]) {
			changed = append(changed, field)
		}
	}
	return changed
}

func (r *Repository) ListEligibility(ctx context.Context, vehicleID string) ([]capability.Eligibility, error) {
	rows, err := r.db.Query(ctx, `SELECT vehicle_id::text,product_code,status,policy_version,certification_ref,effective_at,expires_at,reason_code,version FROM taxi_vehicle_product_eligibility WHERE vehicle_id::text=$1 ORDER BY product_code`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]capability.Eligibility, 0)
	for rows.Next() {
		var e capability.Eligibility
		if err = rows.Scan(&e.VehicleID, &e.ProductCode, &e.Status, &e.PolicyVersion, &e.CertificationRef, &e.EffectiveAt, &e.ExpiresAt, &e.ReasonCode, &e.Version); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CertificationEvent is a verification-compliance decision about one
// vehicle/product. Verified capability facts may accompany a certification.
type CertificationEvent struct {
	EventID          string                        `json:"event_id"`
	VehicleID        string                        `json:"vehicle_id"`
	ProductCode      string                        `json:"product_code"`
	Decision         string                        `json:"decision"`
	SourceVersion    int64                         `json:"source_version"`
	CertificationRef string                        `json:"certification_ref"`
	PolicyVersion    string                        `json:"policy_version"`
	EffectiveAt      *time.Time                    `json:"effective_at,omitempty"`
	ExpiresAt        *time.Time                    `json:"expires_at,omitempty"`
	VerifiedFacts    *capability.VehicleCapability `json:"verified_facts,omitempty"`
	ReasonCode       string                        `json:"reason_code,omitempty"`
}

var decisionStatus = map[string]string{
	"CERTIFIED": capability.EligibilityEligible, "REVOKED": capability.EligibilityRejected,
	"SUSPENDED": capability.EligibilitySuspended, "EXPIRED": capability.EligibilityExpired, "REJECTED": capability.EligibilityRejected,
}

// ApplyCertification applies one event exactly once. Duplicate event ids
// replay their recorded outcome; an older source_version never regresses a
// newer decision.
func (r *Repository) ApplyCertification(ctx context.Context, event CertificationEvent) (string, error) {
	status, ok := decisionStatus[event.Decision]
	if !ok {
		return "", ErrInvalid
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var outcome string
	if err = tx.QueryRow(ctx, `SELECT outcome FROM taxi_certification_events WHERE event_id=$1`, event.EventID).Scan(&outcome); err == nil {
		return outcome, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7474))`, event.VehicleID+"|"+event.ProductCode); err != nil {
		return "", err
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT source_version FROM taxi_vehicle_product_eligibility WHERE vehicle_id::text=$1 AND product_code=$2 FOR UPDATE`, event.VehicleID, event.ProductCode).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	outcome = "APPLIED"
	if event.SourceVersion <= current {
		outcome = "STALE"
	} else {
		if _, err = tx.Exec(ctx, `INSERT INTO taxi_vehicle_product_eligibility(vehicle_id,product_code,status,policy_version,certification_ref,effective_at,expires_at,reason_code,source_version)
			VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (vehicle_id, product_code) DO UPDATE SET status=EXCLUDED.status,policy_version=EXCLUDED.policy_version,certification_ref=EXCLUDED.certification_ref,
				effective_at=EXCLUDED.effective_at,expires_at=EXCLUDED.expires_at,reason_code=EXCLUDED.reason_code,source_version=EXCLUDED.source_version,
				version=taxi_vehicle_product_eligibility.version+1,updated_at=now()`,
			event.VehicleID, event.ProductCode, status, event.PolicyVersion, event.CertificationRef, event.EffectiveAt, event.ExpiresAt, event.ReasonCode, event.SourceVersion); err != nil {
			return "", err
		}
		if status == capability.EligibilityEligible && event.VerifiedFacts != nil {
			f := event.VerifiedFacts
			if _, err = tx.Exec(ctx, `UPDATE vehicles SET total_seats=COALESCE($2,total_seats), passenger_capacity=COALESCE($3,passenger_capacity), capability_version=capability_version+1, capability_updated_at=now() WHERE id::text=$1`, event.VehicleID, f.TotalSeats, f.PassengerCapacity); err != nil {
				return "", err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO taxi_vehicle_capabilities(vehicle_id,wheelchair_accessible,child_seat_capable,verified_at,verified_by,observed_at) VALUES($1::uuid,$2,$3,now(),$4,now())
				ON CONFLICT (vehicle_id) DO UPDATE SET wheelchair_accessible=COALESCE(EXCLUDED.wheelchair_accessible,taxi_vehicle_capabilities.wheelchair_accessible),
					child_seat_capable=COALESCE(EXCLUDED.child_seat_capable,taxi_vehicle_capabilities.child_seat_capable),verified_at=now(),verified_by=EXCLUDED.verified_by,
					version=taxi_vehicle_capabilities.version+1,updated_at=now()`, event.VehicleID, f.WheelchairAccessible, f.ChildSeatCapable, event.CertificationRef); err != nil {
				return "", err
			}
		}
		if err = writeEvent(ctx, tx, event.VehicleID, "taxi.vehicle.product_eligibility_changed", map[string]any{"vehicle_id": event.VehicleID, "product_code": event.ProductCode, "status": status, "source_version": event.SourceVersion}); err != nil {
			return "", err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_certification_events(event_id,vehicle_id,product_code,decision,source_version,certification_ref,outcome) VALUES($1,$2::uuid,$3,$4,$5,$6,$7)`,
		event.EventID, event.VehicleID, event.ProductCode, event.Decision, event.SourceVersion, event.CertificationRef, outcome); err != nil {
		return "", err
	}
	return outcome, tx.Commit(ctx)
}

// PassengerCandidates loads capability and eligibility for each provider's
// first active vehicle.
func (r *Repository) PassengerCandidates(ctx context.Context, providerIDs []string) (map[string]capability.Candidate, error) {
	out := map[string]capability.Candidate{}
	if len(providerIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (v.provider_id) `+capabilityColumns+`,
		       COALESCE((SELECT jsonb_agg(jsonb_build_object('product_code',e.product_code,'status',e.status,'effective_at',e.effective_at,'expires_at',e.expires_at,'policy_version',e.policy_version))
		         FROM taxi_vehicle_product_eligibility e WHERE e.vehicle_id=v.id),'[]'::jsonb), c.vehicle_id IS NOT NULL
		FROM vehicles v LEFT JOIN taxi_vehicle_capabilities c ON c.vehicle_id=v.id
		WHERE v.provider_id::text=ANY($1) AND v.status='active'
		ORDER BY v.provider_id, v.created_at`, providerIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v capability.VehicleCapability
		var provider, body, ac, luggage *string
		var capabilityVersion int64
		var eligibility []byte
		var hasRow bool
		var vehicleID string
		if err = rows.Scan(&vehicleID, &provider, &v.TotalSeats, &v.PassengerCapacity, &capabilityVersion, &body, &v.DoorCount, &ac, &luggage,
			&v.LuggageVolumeML, &v.WheelchairAccessible, &v.ChildSeatCapable, &v.PetEligible, &v.GoodsMaxWeightGrams, &v.GoodsMaxVolumeML,
			&v.GoodsMaxLengthMM, &v.GoodsMaxWidthMM, &v.GoodsMaxHeightMM, &v.GoodsMaxPackageCount, &v.UprightSafe, &v.FragileSafe,
			&v.WeatherProtected, &v.TemperatureControlled, &v.TemperatureMinC, &v.TemperatureMaxC, &v.SecureCargoArea,
			&v.CargoSeparationSupported, &v.ObservedAt, &v.VerifiedAt, &v.Version, &eligibility, &hasRow); err != nil {
			return nil, err
		}
		v.VehicleID = vehicleID
		candidate := capability.Candidate{ProviderID: *provider, VehicleID: vehicleID, ProviderOperational: true, ProviderCompliant: true, VehicleOperational: true, Eligibility: map[string]capability.Eligibility{}}
		// passenger_capacity lives on vehicles, so a capability view exists
		// even without a capabilities row; unknown fields stay nil.
		candidate.Capability = &v
		var list []capability.Eligibility
		_ = json.Unmarshal(eligibility, &list)
		for _, e := range list {
			candidate.Eligibility[strings.ToUpper(e.ProductCode)] = e
		}
		out[*provider] = candidate
	}
	return out, rows.Err()
}

// RecordMatchEvidence persists one evaluation round as operational evidence.
// Competing providers are never exposed to customers through this table.
func (r *Repository) RecordMatchEvidence(ctx context.Context, bookingID, instance, mode string, rows []EvidenceRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var attemptID string
	if err = tx.QueryRow(ctx, `INSERT INTO taxi_match_attempts(booking_id,matcher_instance,outcome) VALUES($1,$2,'evaluated') RETURNING id::text`, bookingID, instance).Scan(&attemptID); err != nil {
		return err
	}
	for _, row := range rows {
		result := "REJECTED"
		if row.Decision.Eligible {
			result = "ELIGIBLE"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO taxi_match_candidates(attempt_id,provider_id,vehicle_id,eligibility_result,rejection_reason_code,evaluation_mode,legacy_decision_eligible)
			VALUES($1,$2::uuid,NULLIF($3,'')::uuid,$4,NULLIF($5,''),$6,$7)`, attemptID, row.ProviderID, row.VehicleID, result, rejection(row.Decision), mode, row.LegacyEligible); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

type EvidenceRow struct {
	ProviderID     string
	VehicleID      string
	Decision       capability.Decision
	LegacyEligible bool
}

func rejection(d capability.Decision) string {
	if d.Eligible {
		return ""
	}
	return d.Reason
}

func writeEvent(ctx context.Context, tx pgx.Tx, vehicleID, event string, payload any) error {
	body, _ := json.Marshal(payload)
	_, err := tx.Exec(ctx, `INSERT INTO taxi_outbox(aggregate_type,aggregate_id,event_type,payload,destination) VALUES('vehicle',$1::uuid,$2,$3,'UNROUTED')`, vehicleID, event, body)
	return err
}

// BookingRequirement returns a booking's frozen product code and hard
// requirement snapshot, both NULL for legacy bookings.
func (r *Repository) BookingRequirement(ctx context.Context, bookingID string) (string, []byte, error) {
	var code *string
	var snapshot []byte
	err := r.db.QueryRow(ctx, `SELECT service_product_code, requirement_snapshot_json FROM taxi_bookings WHERE id::text=$1`, bookingID).Scan(&code, &snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrNotFound
	}
	if code == nil {
		return "", snapshot, err
	}
	return *code, snapshot, err
}
