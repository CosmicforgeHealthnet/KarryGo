package goods

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/custodyops"
)

// ErrStalePlan means a commit carried an older fulfilment plan version than
// one already committed for the same plan.
var ErrStalePlan = errors.New("taxi goods fulfilment plan version is stale")

// MultiStopCommit is everything a multi-pickup mission persists atomically.
type MultiStopCommit struct {
	Request       commercemission.Request
	Product       capability.Product
	PolicyVersion string
	Stops         []MissionStop
	Cargo         []capability.StopCargo
	Custody       custodyops.RegisterMissionCommand
}

// MissionStop is one ordered stop of a Taxi Goods mission.
type MissionStop struct {
	ID                   string     `json:"stop_id,omitempty"`
	Sequence             int        `json:"sequence"`
	StopType             string     `json:"stop_type"`
	ExternalStopRef      string     `json:"stop_ref"`
	VendorRef            string     `json:"vendor_ref,omitempty"`
	VendorBranchRef      string     `json:"vendor_branch_ref,omitempty"`
	Latitude             float64    `json:"latitude"`
	Longitude            float64    `json:"longitude"`
	ReadinessWindowStart *time.Time `json:"readiness_window_start,omitempty"`
	ReadinessWindowEnd   *time.Time `json:"readiness_window_end,omitempty"`
	SecureHandover       bool       `json:"secure_handover_required"`
	Status               string     `json:"status"`
	Version              int64      `json:"version"`
}

// CreateMultiStopMission commits a plan-bound multi-pickup mission, its stops,
// per-stop cargo and the shared custody ledger in ONE transaction. A replay
// of the same operation returns the original mission. A different body, a
// stale plan version or a second mission for the same plan slot is refused.
func (r *Repository) CreateMultiStopMission(ctx context.Context, caller string, commit MultiStopCommit) (Mission, error) {
	request := commit.Request
	fp := request.Fingerprint()
	if prior, err := r.ReadMissionByOperation(ctx, caller, request.OperationReference()); err == nil {
		var stored string
		if e := r.db.QueryRow(ctx, `SELECT request_fingerprint FROM taxi_goods_missions WHERE id=$1`, prior.ID).Scan(&stored); e != nil {
			return Mission{}, e
		}
		if stored != fp {
			return Mission{}, ErrConflict
		}
		return prior, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Mission{}, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Mission{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	planRef, version := planValue(request, "ref"), planVersion(request)
	// Serialize commits of one plan so the stale-version check is race free.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7171))`, caller+"|"+planRef); err != nil {
		return Mission{}, err
	}
	var newest int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(plan_version),0) FROM taxi_goods_missions WHERE source_service=$1 AND plan_ref=$2 AND status<>'CANCELLED'`, caller, planRef).Scan(&newest); err != nil {
		return Mission{}, err
	}
	if version < newest {
		return Mission{}, ErrStalePlan
	}
	missionID := uuid.NewString()
	first := commit.Stops[0]
	pickupJSON, _ := json.Marshal(commercemission.Point{Latitude: first.Latitude, Longitude: first.Longitude})
	dropoffJSON, _ := json.Marshal(request.Dropoff)
	loadJSON, _ := json.Marshal(request.Load)
	pickups := 0
	var windowStart, windowEnd *time.Time
	for _, stop := range commit.Stops {
		if stop.StopType != custodyops.StopPickup {
			continue
		}
		pickups++
		if stop.ReadinessWindowStart != nil && (windowStart == nil || stop.ReadinessWindowStart.Before(*windowStart)) {
			windowStart = stop.ReadinessWindowStart
		}
		if stop.ReadinessWindowEnd != nil && (windowEnd == nil || stop.ReadinessWindowEnd.After(*windowEnd)) {
			windowEnd = stop.ReadinessWindowEnd
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO taxi_goods_missions(id,source_service,mission_ref,operation_ref,request_fingerprint,parent_order_ref,vendor_order_refs,customer_ref,quote_id,plan_ref,plan_version,plan_fingerprint,mission_slot_ref,pickup,dropoff,receiver_ref,receiver_name,receiver_phone,load,handling,manifest_ref,manifest_version,agreed_fee_minor,currency,pickup_evidence_required,delivery_evidence_required,requested_at,service_product_code,service_product_version,eligibility_policy_version,pickup_count,custody_mode,not_before,pickup_window_start,pickup_window_end,plan_binding_mode)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULL,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,'',0,$20,$21,FALSE,TRUE,$22,$23,$24,$25,$26,'MULTI_MANIFEST',$27,$28,$29,'PLAN_BOUND')`,
		missionID, caller, request.MissionRef, request.OperationReference(), fp, request.ParentOrderRef, request.VendorOrderRefs, request.CustomerRef,
		planRef, version, planValue(request, "fingerprint"), planValue(request, "slot"), pickupJSON, dropoffJSON, request.CustomerRef,
		request.Dropoff.Contact, request.Dropoff.Phone, loadJSON, handlingCapabilities(request.Handling), request.AgreedFeeMinor,
		strings.ToUpper(request.Currency), request.RequestedAt, commit.Product.Code, commit.Product.Version, commit.PolicyVersion, pickups,
		windowStart, windowStart, windowEnd)
	if err != nil {
		if strings.Contains(err.Error(), "uq_taxi_goods_plan_slot") {
			return Mission{}, ErrConflict
		}
		return Mission{}, err
	}
	stopIDs := map[string]string{}
	for _, stop := range commit.Stops {
		id := uuid.NewString()
		stopIDs[stop.ExternalStopRef] = id
		if _, err = tx.Exec(ctx, `INSERT INTO taxi_goods_mission_stops(id,mission_id,sequence,stop_type,external_stop_ref,vendor_ref,vendor_branch_ref,latitude,longitude,readiness_window_start,readiness_window_end,secure_handover_required,handover_policy_version)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, id, missionID, stop.Sequence, stop.StopType, stop.ExternalStopRef, stop.VendorRef, stop.VendorBranchRef,
			stop.Latitude, stop.Longitude, stop.ReadinessWindowStart, stop.ReadinessWindowEnd, stop.SecureHandover, commit.PolicyVersion); err != nil {
			return Mission{}, err
		}
	}
	for _, cargo := range commit.Cargo {
		if _, err = tx.Exec(ctx, `INSERT INTO taxi_goods_cargo_requirements(mission_id,stop_id,weight_grams,volume_ml,max_length_mm,max_width_mm,max_height_mm,package_count,fragile,upright_required,weather_protection_required,temperature_class,temperature_min_c,temperature_max_c,secure_cargo_required)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT (mission_id, stop_id) WHERE stop_id IS NOT NULL DO UPDATE SET weight_grams=taxi_goods_cargo_requirements.weight_grams+EXCLUDED.weight_grams,
				volume_ml=taxi_goods_cargo_requirements.volume_ml+EXCLUDED.volume_ml, package_count=taxi_goods_cargo_requirements.package_count+EXCLUDED.package_count,
				max_length_mm=GREATEST(taxi_goods_cargo_requirements.max_length_mm,EXCLUDED.max_length_mm), max_width_mm=GREATEST(taxi_goods_cargo_requirements.max_width_mm,EXCLUDED.max_width_mm),
				max_height_mm=GREATEST(taxi_goods_cargo_requirements.max_height_mm,EXCLUDED.max_height_mm), fragile=taxi_goods_cargo_requirements.fragile OR EXCLUDED.fragile,
				upright_required=taxi_goods_cargo_requirements.upright_required OR EXCLUDED.upright_required, secure_cargo_required=taxi_goods_cargo_requirements.secure_cargo_required OR EXCLUDED.secure_cargo_required`,
			missionID, stopIDs[cargo.StopRef], cargo.WeightGrams, cargo.VolumeML, cargo.MaxLengthMM, cargo.MaxWidthMM, cargo.MaxHeightMM, cargo.PackageCount,
			cargo.Fragile, cargo.UprightRequired, cargo.WeatherProtectionRequired, cargoTemperatureClass(cargo), cargo.TemperatureMinC, cargo.TemperatureMaxC, cargo.SecureCargoRequired); err != nil {
			return Mission{}, err
		}
	}
	for _, parcel := range request.Parcels {
		if _, err = tx.Exec(ctx, `INSERT INTO taxi_goods_parcels(mission_id,parcel_ref,vendor_ref,branch_ref,vendor_order_ref,manifest_ref,manifest_version,weight_grams,length_mm,width_mm,height_mm,package_count,handling,pickup_stop_ref) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			missionID, parcel.ParcelRef, parcel.VendorRef, parcel.BranchRef, parcel.VendorOrderRef, parcel.ManifestRef, parcel.ManifestVersion, parcel.WeightGrams,
			parcel.LengthMm, parcel.WidthMm, parcel.HeightMm, max64(parcel.PackageCount, 1), handlingCapabilities(parcel.Handling), parcel.PickupStopRef); err != nil {
			return Mission{}, err
		}
	}
	if _, err = r.custody.RegisterMissionTx(ctx, tx, commit.Custody); err != nil {
		return Mission{}, err
	}
	if err = r.writeEvent(ctx, tx, missionID, "taxi.goods.mission.committed.v1", 1, map[string]any{"mission_ref": request.MissionRef, "status": StatusMatching, "pickup_count": pickups, "service_product_code": commit.Product.Code}); err != nil {
		return Mission{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Mission{}, err
	}
	return r.ReadMission(ctx, caller, request.MissionRef)
}

func cargoTemperatureClass(cargo capability.StopCargo) string {
	if class := strings.ToUpper(strings.TrimSpace(cargo.TemperatureClass)); class != "" {
		return class
	}
	return capability.TemperatureAmbient
}

// MissionShape loads a multi-pickup mission's stops and per-stop cargo.
func (r *Repository) MissionShape(ctx context.Context, missionID string) ([]MissionStop, []capability.StopCargo, capability.Product, error) {
	var product capability.Product
	var productCode *string
	var productVersion *int64
	if err := r.db.QueryRow(ctx, `SELECT service_product_code, service_product_version FROM taxi_goods_missions WHERE id=$1`, missionID).Scan(&productCode, &productVersion); err != nil {
		return nil, nil, product, err
	}
	if productCode != nil {
		product.Code = *productCode
	}
	if productVersion != nil {
		product.Version = *productVersion
	}
	rows, err := r.db.Query(ctx, `SELECT id::text,sequence,stop_type,external_stop_ref,vendor_ref,vendor_branch_ref,latitude,longitude,readiness_window_start,readiness_window_end,secure_handover_required,status,version FROM taxi_goods_mission_stops WHERE mission_id=$1 ORDER BY sequence`, missionID)
	if err != nil {
		return nil, nil, product, err
	}
	var stops []MissionStop
	for rows.Next() {
		var s MissionStop
		if err = rows.Scan(&s.ID, &s.Sequence, &s.StopType, &s.ExternalStopRef, &s.VendorRef, &s.VendorBranchRef, &s.Latitude, &s.Longitude, &s.ReadinessWindowStart, &s.ReadinessWindowEnd, &s.SecureHandover, &s.Status, &s.Version); err != nil {
			rows.Close()
			return nil, nil, product, err
		}
		stops = append(stops, s)
	}
	rows.Close()
	rows, err = r.db.Query(ctx, `SELECT st.external_stop_ref,c.weight_grams,c.volume_ml,c.max_length_mm,c.max_width_mm,c.max_height_mm,c.package_count,c.fragile,c.upright_required,c.weather_protection_required,c.temperature_class,c.temperature_min_c,c.temperature_max_c,c.secure_cargo_required
		FROM taxi_goods_cargo_requirements c JOIN taxi_goods_mission_stops st ON st.id=c.stop_id
		WHERE c.mission_id=$1 AND st.status NOT IN ('DELIVERED','CANCELLED_BY_REPLAN','FAILED') ORDER BY st.sequence`, missionID)
	if err != nil {
		return nil, nil, product, err
	}
	defer rows.Close()
	var cargo []capability.StopCargo
	for rows.Next() {
		var c capability.StopCargo
		if err = rows.Scan(&c.StopRef, &c.WeightGrams, &c.VolumeML, &c.MaxLengthMM, &c.MaxWidthMM, &c.MaxHeightMM, &c.PackageCount, &c.Fragile, &c.UprightRequired, &c.WeatherProtectionRequired, &c.TemperatureClass, &c.TemperatureMinC, &c.TemperatureMaxC, &c.SecureCargoRequired); err != nil {
			return nil, nil, product, err
		}
		cargo = append(cargo, c)
	}
	return stops, cargo, product, rows.Err()
}

// GoodsCapabilityCandidates loads the facts every hard gate needs for the
// given providers, preserving proximity order. Offers are never created here.
func (r *Repository) GoodsCapabilityCandidates(ctx context.Context, providerIDs []string) ([]capability.Candidate, error) {
	if len(providerIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT p.id::text, v.id::text,
		       COALESCE(o.state='AVAILABLE',FALSE) AND s.capability_status='APPROVED' AND s.goods_available,
		       p.account_status='active' AND p.onboarding_status='complete' AND p.identity_verification_status='verified'
		         AND p.vehicle_verification_status='verified' AND p.document_compliance_status='compliant',
		       v.status='active' AND p.vehicle_operational_status='active',
		       c.vehicle_id IS NOT NULL, c.goods_max_weight_grams, c.goods_max_volume_ml, c.goods_max_length_mm, c.goods_max_width_mm,
		       c.goods_max_height_mm, c.goods_max_package_count, c.upright_safe, c.fragile_safe, c.weather_protected, c.temperature_controlled,
		       c.temperature_min_c, c.temperature_max_c, c.secure_cargo_area, c.cargo_separation_supported, c.version,
		       COALESCE((SELECT jsonb_agg(jsonb_build_object('product_code',e.product_code,'status',e.status,'effective_at',e.effective_at,'expires_at',e.expires_at,'policy_version',e.policy_version))
		         FROM taxi_vehicle_product_eligibility e WHERE e.vehicle_id=v.id),'[]'::jsonb)
		FROM taxi_providers p
		JOIN taxi_goods_provider_settings s ON s.provider_id=p.id
		JOIN vehicles v ON v.provider_id=p.id
		LEFT JOIN provider_operational_state o ON o.provider_id=p.id
		LEFT JOIN taxi_vehicle_capabilities c ON c.vehicle_id=v.id
		WHERE p.id::text=ANY($1)
		ORDER BY array_position($1::text[],p.id::text), v.created_at`, providerIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []capability.Candidate
	for rows.Next() {
		var c capability.Candidate
		var hasCapability bool
		v := capability.VehicleCapability{}
		var eligibility []byte
		if err = rows.Scan(&c.ProviderID, &c.VehicleID, &c.ProviderOperational, &c.ProviderCompliant, &c.VehicleOperational, &hasCapability,
			&v.GoodsMaxWeightGrams, &v.GoodsMaxVolumeML, &v.GoodsMaxLengthMM, &v.GoodsMaxWidthMM, &v.GoodsMaxHeightMM, &v.GoodsMaxPackageCount,
			&v.UprightSafe, &v.FragileSafe, &v.WeatherProtected, &v.TemperatureControlled, &v.TemperatureMinC, &v.TemperatureMaxC,
			&v.SecureCargoArea, &v.CargoSeparationSupported, &v.Version, &eligibility); err != nil {
			return nil, err
		}
		if hasCapability {
			v.VehicleID = c.VehicleID
			c.Capability = &v
		}
		c.Eligibility = map[string]capability.Eligibility{}
		var rowsJSON []capability.Eligibility
		_ = json.Unmarshal(eligibility, &rowsJSON)
		for _, e := range rowsJSON {
			e.VehicleID = c.VehicleID
			c.Eligibility[e.ProductCode] = e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// TransitionStop moves the mission and one stop together under the mission
// row lock, so stop and mission lifecycle can never disagree.
func (r *Repository) TransitionStop(ctx context.Context, missionRef, providerID, stopRef string, missionFrom []string, missionTo, opState string, stopFrom []string, stopTo, event string) (Mission, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Mission{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var missionID, status string
	var version int64
	err = tx.QueryRow(ctx, `SELECT id::text,status,version FROM taxi_goods_missions WHERE mission_ref=$1 AND provider_id=$2 AND custody_mode='MULTI_MANIFEST' FOR UPDATE`, missionRef, providerID).Scan(&missionID, &status, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrNotFound
	}
	if err != nil {
		return Mission{}, err
	}
	if !contains(missionFrom, status) {
		return Mission{}, ErrConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE taxi_goods_mission_stops SET status=$3,version=version+1,updated_at=now(),arrived_at=CASE WHEN $3='ARRIVED' THEN now() ELSE arrived_at END WHERE mission_id=$1 AND external_stop_ref=$2 AND stop_type='PICKUP' AND status=ANY($4)`, missionID, stopRef, stopTo, stopFrom)
	if err != nil {
		return Mission{}, err
	}
	if tag.RowsAffected() != 1 {
		return Mission{}, ErrConflict
	}
	if status != missionTo {
		if err = tx.QueryRow(ctx, `UPDATE taxi_goods_missions SET status=$2,version=version+1,updated_at=now(),accepted_at=COALESCE(accepted_at,CASE WHEN $2='ASSIGNED' THEN now() END) WHERE id=$1 RETURNING version`, missionID, missionTo).Scan(&version); err != nil {
			return Mission{}, err
		}
		if opState != "" {
			if _, err = tx.Exec(ctx, `UPDATE provider_operational_state SET state=$3,reason=$4,effective_at=now(),version=version+1 WHERE provider_id=$1 AND goods_mission_id=$2::uuid`, providerID, missionID, opState, "taxi_goods_"+strings.ToLower(missionTo)); err != nil {
				return Mission{}, err
			}
		}
	}
	if err = r.writeEvent(ctx, tx, missionID, event, version, map[string]any{"mission_ref": missionRef, "status": missionTo, "stop_ref": stopRef, "stop_status": stopTo}); err != nil {
		return Mission{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Mission{}, err
	}
	return r.ReadMissionForProvider(ctx, providerID, missionRef)
}

// MarkStopDelayed records a readiness delay at a pending pickup and requests
// a replan. It never cancels or re-creates the mission.
func (r *Repository) MarkStopDelayed(ctx context.Context, caller, missionRef, stopRef string, readyAt time.Time) (Mission, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Mission{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var missionID string
	var version int64
	err = tx.QueryRow(ctx, `SELECT id::text,version FROM taxi_goods_missions WHERE mission_ref=$1 AND source_service=$2 AND custody_mode='MULTI_MANIFEST' FOR UPDATE`, missionRef, caller).Scan(&missionID, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrNotFound
	}
	if err != nil {
		return Mission{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE taxi_goods_mission_stops SET status='DELAYED',readiness_window_start=$3,version=version+1,updated_at=now() WHERE mission_id=$1 AND external_stop_ref=$2 AND stop_type='PICKUP' AND status IN ('PLANNED','EN_ROUTE','ARRIVED','DELAYED')`, missionID, stopRef, readyAt)
	if err != nil {
		return Mission{}, err
	}
	if tag.RowsAffected() != 1 {
		return Mission{}, ErrConflict
	}
	for _, event := range []string{"taxi.goods.pickup.delayed.v1", "taxi.goods.replan_requested.v1"} {
		if _, err = tx.Exec(ctx, `INSERT INTO taxi_outbox(aggregate_type,aggregate_id,event_type,payload,destination) VALUES('taxi_goods_mission',$1,$2,$3,'UNROUTED')`, missionID, event,
			mustJSON(map[string]any{"mission_ref": missionRef, "stop_ref": stopRef, "ready_at": readyAt.UTC(), "mission_version": version})); err != nil {
			return Mission{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Mission{}, err
	}
	return r.ReadMission(ctx, caller, missionRef)
}

// ReplanRoute records a new stop order as a new route version. Stop identity,
// manifests and handover history are untouched; only order changes.
func (r *Repository) ReplanRoute(ctx context.Context, caller, missionRef, operationRef, reason string, order []string, expectedRouteVersion int64) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var missionID string
	var routeVersion int64
	if err = tx.QueryRow(ctx, `SELECT id::text,route_version FROM taxi_goods_missions WHERE mission_ref=$1 AND source_service=$2 FOR UPDATE`, missionRef, caller).Scan(&missionID, &routeVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	var replayed int64
	if err = tx.QueryRow(ctx, `SELECT route_version FROM taxi_goods_route_versions WHERE mission_id=$1 AND operation_ref=$2`, missionID, operationRef).Scan(&replayed); err == nil {
		return replayed, nil
	}
	if routeVersion != expectedRouteVersion {
		return 0, ErrConflict
	}
	// Collected or delivered stops keep their position; only pending stops
	// may be re-sequenced, and the dropoff stays last.
	rows, err := tx.Query(ctx, `SELECT external_stop_ref,stop_type,status FROM taxi_goods_mission_stops WHERE mission_id=$1 ORDER BY sequence`, missionID)
	if err != nil {
		return 0, err
	}
	var current []MissionStop
	for rows.Next() {
		var s MissionStop
		if err = rows.Scan(&s.ExternalStopRef, &s.StopType, &s.Status); err != nil {
			rows.Close()
			return 0, err
		}
		current = append(current, s)
	}
	rows.Close()
	if err = validateReplan(current, order); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE taxi_goods_mission_stops SET sequence=sequence+100000 WHERE mission_id=$1`, missionID); err != nil {
		return 0, err
	}
	for i, ref := range order {
		if _, err = tx.Exec(ctx, `UPDATE taxi_goods_mission_stops SET sequence=$3,version=version+1,updated_at=now() WHERE mission_id=$1 AND external_stop_ref=$2`, missionID, ref, i+1); err != nil {
			return 0, err
		}
	}
	routeVersion++
	if _, err = tx.Exec(ctx, `UPDATE taxi_goods_missions SET route_version=$2,updated_at=now() WHERE id=$1`, missionID, routeVersion); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_goods_route_versions(mission_id,route_version,stop_order,reason_code,operation_ref) VALUES($1,$2,$3,$4,$5)`, missionID, routeVersion, order, reason, operationRef); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_outbox(aggregate_type,aggregate_id,event_type,payload,destination) VALUES('taxi_goods_mission',$1,'taxi.goods.route_replanned.v1',$2,'UNROUTED')`, missionID,
		mustJSON(map[string]any{"mission_ref": missionRef, "route_version": routeVersion, "stop_order": order, "reason_code": reason})); err != nil {
		return 0, err
	}
	return routeVersion, tx.Commit(ctx)
}

func validateReplan(current []MissionStop, order []string) error {
	if len(order) != len(current) {
		return ErrInvalid
	}
	position := map[string]int{}
	for i, ref := range order {
		if _, dup := position[ref]; dup {
			return ErrInvalid
		}
		position[ref] = i
	}
	for i, stop := range current {
		at, ok := position[stop.ExternalStopRef]
		if !ok {
			return ErrInvalid
		}
		if stop.StopType == custodyops.StopDropoff && at != len(order)-1 {
			return ErrInvalid
		}
		if (stop.Status == "COLLECTED" || stop.Status == "DELIVERED") && at != i {
			return ErrConflict
		}
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func mustJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

type stopCustody struct {
	missionID, missionRef, tenantRef, provider, stage string
	stopRef, challengeRef, operationRef, verifiedBy   string
	sourceVersion                                     int64
	now                                               time.Time
}

// applyStopCustodyTx runs inside VerifyAndTransition's transaction for a
// multi-pickup mission. A pickup verification collects exactly the manifests
// registered at that stop and returns the next mission status: back to
// EN_ROUTE_PICKUP while pickups remain, PICKED_UP after the last one. A
// delivery verification releases every manifest the provider holds.
func (r *Repository) applyStopCustodyTx(ctx context.Context, tx pgx.Tx, in stopCustody) (string, string, error) {
	if in.stage == StageDelivery {
		rows, err := tx.Query(ctx, `SELECT manifest_ref,manifest_version FROM custody_stop_manifests WHERE mission_ref=$1 AND status='COLLECTED' AND current_custodian_ref=$2`, in.missionRef, in.provider)
		if err != nil {
			return "", "", err
		}
		var keys []custodyops.ManifestKey
		for rows.Next() {
			var key custodyops.ManifestKey
			if err = rows.Scan(&key.ManifestRef, &key.ManifestVersion); err != nil {
				rows.Close()
				return "", "", err
			}
			keys = append(keys, key)
		}
		rows.Close()
		var dropoff string
		if err = tx.QueryRow(ctx, `SELECT external_stop_ref FROM taxi_goods_mission_stops WHERE mission_id=$1 AND stop_type='DROPOFF'`, in.missionID).Scan(&dropoff); err != nil {
			return "", "", err
		}
		if len(keys) == 0 {
			return "", "", ErrConflict
		}
		result, err := r.custody.DeliverManifestsTx(ctx, tx, custodyops.DeliverManifestsCommand{
			OperationRef: in.operationRef, CorrelationID: in.missionRef, MissionRef: in.missionRef, TenantRef: in.tenantRef,
			DropoffStopRef: dropoff, ProviderRef: in.provider, DeliveryChallengeRef: in.challengeRef, SourceVersion: in.sourceVersion, Manifests: keys,
		})
		if err != nil {
			return "", "", err
		}
		if result.State != "DELIVERED" {
			// Some required manifests are still unaccounted for; the
			// mission must not be marked delivered.
			return "", "", custodyops.ErrManifestsUnaccounted
		}
		if _, err = tx.Exec(ctx, `UPDATE taxi_goods_mission_stops SET status='DELIVERED',handover_verified_at=$2,completed_at=$2,handover_challenge_ref=$3,version=version+1,updated_at=$2 WHERE mission_id=$1 AND stop_type='DROPOFF'`, in.missionID, in.now, in.challengeRef); err != nil {
			return "", "", err
		}
		return "", "", nil
	}
	if strings.TrimSpace(in.stopRef) == "" {
		return "", "", ErrCredentialInvalid
	}
	tag, err := tx.Exec(ctx, `UPDATE taxi_goods_mission_stops SET status='COLLECTED',handover_verified_at=$3,completed_at=$3,handover_challenge_ref=$4,version=version+1,updated_at=$3 WHERE mission_id=$1 AND external_stop_ref=$2 AND stop_type='PICKUP' AND status='HANDOVER_PENDING'`, in.missionID, in.stopRef, in.now, in.challengeRef)
	if err != nil {
		return "", "", err
	}
	if tag.RowsAffected() != 1 {
		return "", "", ErrConflict
	}
	rows, err := tx.Query(ctx, `SELECT manifest_ref,manifest_version FROM custody_stop_manifests WHERE mission_ref=$1 AND pickup_stop_ref=$2 AND status='REGISTERED'`, in.missionRef, in.stopRef)
	if err != nil {
		return "", "", err
	}
	var keys []custodyops.ManifestKey
	for rows.Next() {
		var key custodyops.ManifestKey
		if err = rows.Scan(&key.ManifestRef, &key.ManifestVersion); err != nil {
			rows.Close()
			return "", "", err
		}
		keys = append(keys, key)
	}
	rows.Close()
	if len(keys) == 0 {
		return "", "", ErrConflict
	}
	if _, err = r.custody.CollectManifestsTx(ctx, tx, custodyops.CollectManifestsCommand{
		OperationRef: in.operationRef, CorrelationID: in.missionRef, MissionRef: in.missionRef, TenantRef: in.tenantRef,
		StopRef: in.stopRef, ProviderRef: in.provider, HandoverChallengeRef: in.challengeRef, HandoverVerifiedByRef: in.verifiedBy,
		SourceVersion: in.sourceVersion, Manifests: keys,
	}); err != nil {
		return "", "", err
	}
	var remaining int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM taxi_goods_mission_stops WHERE mission_id=$1 AND stop_type='PICKUP' AND status NOT IN ('COLLECTED','CANCELLED_BY_REPLAN','FAILED')`, in.missionID).Scan(&remaining); err != nil {
		return "", "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_outbox(aggregate_type,aggregate_id,event_type,payload,destination) VALUES('taxi_goods_mission',$1,'taxi.goods.pickup.collected.v1',$2,'UNROUTED')`, in.missionID,
		mustJSON(map[string]any{"mission_ref": in.missionRef, "stop_ref": in.stopRef, "manifests": keys, "remaining_pickups": remaining})); err != nil {
		return "", "", err
	}
	if remaining > 0 {
		return StatusEnRoutePickup, "EN_ROUTE_PICKUP", nil
	}
	return StatusPickedUp, "ON_TRIP", nil
}
