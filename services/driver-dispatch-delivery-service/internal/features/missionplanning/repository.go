package missionplanning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func isUnique(err error) bool                    { var e *pgconn.PgError; return errors.As(err, &e) && e.Code == "23505" }

func (r *Repository) Create(ctx context.Context, in PlanRequest, plan Plan, fp string) (Plan, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Plan{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var existingGroup, existingFP string
	err = tx.QueryRow(ctx, `SELECT g.id::text,p.request_fingerprint FROM dispatch_delivery_groups g JOIN dispatch_mission_plans p ON p.delivery_group_id=g.id WHERE g.source_type='VENDOR_COMMERCE' AND g.source_ref=$1 AND g.source_version=$2 ORDER BY p.version DESC LIMIT 1`, in.SourceRef, in.SourceVersion).Scan(&existingGroup, &existingFP)
	if err == nil {
		if existingFP != fp {
			return Plan{}, ErrConflict
		}
		out, readErr := readPlan(ctx, tx, existingGroup)
		if readErr == nil {
			out.Replayed = true
		}
		return out, readErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO dispatch_delivery_groups(id,customer_id,source_type,source_ref,source_version,state) VALUES($1,NULLIF($2,'')::uuid,'VENDOR_COMMERCE',$3,$4,'READY_FOR_DISPATCH') RETURNING id::text`, plan.GroupRef, in.CustomerID, in.SourceRef, in.SourceVersion).Scan(&plan.GroupRef)
	if err != nil {
		if isUnique(err) {
			_ = tx.Rollback(ctx)
			var groupID, storedFP string
			readErr := r.db.QueryRow(ctx, `SELECT g.id::text,p.request_fingerprint FROM dispatch_delivery_groups g JOIN dispatch_mission_plans p ON p.delivery_group_id=g.id WHERE g.source_type='VENDOR_COMMERCE' AND g.source_ref=$1 AND g.source_version=$2 ORDER BY p.version DESC LIMIT 1`, in.SourceRef, in.SourceVersion).Scan(&groupID, &storedFP)
			if readErr != nil || storedFP != fp {
				return Plan{}, ErrConflict
			}
			out, readErr := r.Read(ctx, groupID)
			if readErr == nil {
				out.Replayed = true
			}
			return out, readErr
		}
		return Plan{}, err
	}
	if err = insertPlan(ctx, tx, in, &plan, fp); err != nil {
		return Plan{}, err
	}
	if err = emitPlan(ctx, tx, "dispatch.delivery_plan.created.v1", plan); err != nil {
		return Plan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (r *Repository) Replan(ctx context.Context, groupRef string, in PlanRequest, plan Plan, fp string) (Plan, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Plan{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var groupID, sourceType, sourceRef string
	var sourceVersion int64
	if err = tx.QueryRow(ctx, `SELECT id::text,source_type,source_ref,source_version FROM dispatch_delivery_groups WHERE id=$1 FOR UPDATE`, groupRef).Scan(&groupID, &sourceType, &sourceRef, &sourceVersion); errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrNotFound
	} else if err != nil {
		return Plan{}, err
	}
	if sourceType != "VENDOR_COMMERCE" || sourceRef != in.SourceRef || sourceVersion != in.SourceVersion {
		return Plan{}, ErrConflict
	}
	var priorFP string
	var priorVersion *int64
	err = tx.QueryRow(ctx, `SELECT request_fingerprint,result_plan_version FROM dispatch_replan_operations WHERE operation_ref=$1`, in.OperationRef).Scan(&priorFP, &priorVersion)
	if err == nil {
		if priorFP != fp || priorVersion == nil {
			return Plan{}, ErrConflict
		}
		out, readErr := readPlan(ctx, tx, groupRef)
		if readErr == nil {
			out.Replayed = true
		}
		return out, readErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, err
	}
	var blocked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dispatch_delivery_missions WHERE delivery_group_id=$1 AND state IN ('ASSIGNED','EN_ROUTE_PICKUP','AT_PICKUP','PICKUP_VERIFICATION','PICKED_UP','IN_TRANSIT','AT_DELIVERY','DELIVERY_VERIFICATION','DELIVERED','COMPLETED','RETURNING','RETURNED'))`, groupRef).Scan(&blocked); err != nil {
		return Plan{}, err
	}
	if blocked {
		return Plan{}, ErrReplanBlocked
	}
	var oldPlanID string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT id::text,version FROM dispatch_mission_plans WHERE delivery_group_id=$1 AND status='ACTIVE' FOR UPDATE`, groupRef).Scan(&oldPlanID, &version); err != nil {
		return Plan{}, err
	}
	if in.ExpectedPlanVersion != version {
		return Plan{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dispatch_replan_operations(delivery_group_id,operation_ref,request_fingerprint,source_type,source_ref,expected_source_version,expected_plan_version) VALUES($1,$2,$3,$4,$5,$6,$7)`, groupRef, in.OperationRef, fp, sourceType, sourceRef, sourceVersion, version); err != nil {
		return Plan{}, err
	}
	plan.GroupRef = groupRef
	plan.Version = version + 1
	// Supersede execution-free missions and their active manifests; parcel rows
	// remain stable and are rebound to the new immutable manifest versions.
	if _, err = tx.Exec(ctx, `UPDATE dispatch_parcel_manifests pm SET status='SUPERSEDED' FROM dispatch_delivery_missions m WHERE pm.mission_id=m.id AND m.delivery_group_id=$1 AND pm.status='ACTIVE'`, groupRef); err != nil {
		return Plan{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_mission_plans SET status='SUPERSEDED' WHERE id=$1`, oldPlanID); err != nil {
		return Plan{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='CANCELLED',version=version+1,updated_at=now() WHERE delivery_group_id=$1 AND state IN ('PLANNED','QUEUED','OFFERING')`, groupRef); err != nil {
		return Plan{}, err
	}
	if err = insertPlan(ctx, tx, in, &plan, fp); err != nil {
		return Plan{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_replan_operations SET result_plan_id=(SELECT id FROM dispatch_mission_plans WHERE delivery_group_id=$1 AND version=$2),result_plan_version=$2 WHERE operation_ref=$3`, groupRef, plan.Version, in.OperationRef); err != nil {
		return Plan{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_mission_plans SET superseded_by=(SELECT id FROM dispatch_mission_plans WHERE delivery_group_id=$1 AND version=$2) WHERE id=$3`, groupRef, plan.Version, oldPlanID); err != nil {
		return Plan{}, err
	}
	if err = emitPlan(ctx, tx, "dispatch.delivery_plan.superseded.v1", plan); err != nil {
		return Plan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func insertPlan(ctx context.Context, tx pgx.Tx, in PlanRequest, plan *Plan, fp string) error {
	for i := range plan.Missions {
		var prior int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(pm.version),0) FROM dispatch_parcel_manifests pm JOIN dispatch_delivery_missions m ON m.id=pm.mission_id WHERE m.delivery_group_id=$1 AND pm.manifest_ref=$2`, plan.GroupRef, plan.Missions[i].ManifestRef).Scan(&prior); err != nil {
			return err
		}
		plan.Missions[i].ManifestVersion = prior + 1
	}
	snapshot, _ := json.Marshal(plan)
	reasons, _ := json.Marshal(plan.Reasons)
	var planID string
	if err := tx.QueryRow(ctx, `INSERT INTO dispatch_mission_plans(delivery_group_id,plan_ref,version,outcome,policy_version,request_fingerprint,reason_codes,plan_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, plan.GroupRef, plan.PlanRef, plan.Version, plan.Outcome, plan.PolicyVersion, fp, reasons, snapshot).Scan(&planID); err != nil {
		return err
	}
	parcelIDs := map[string]string{}
	for _, p := range in.Parcels {
		decl, _ := json.Marshal(p)
		var id string
		err := tx.QueryRow(ctx, `INSERT INTO dispatch_parcels(delivery_group_id,parcel_ref,karrygo_parcel_ref,scannable_code,declaration,package_policy_version,state) VALUES($1,$2,$3,$4,$5,$6,'DECLARED') ON CONFLICT(delivery_group_id,parcel_ref) DO UPDATE SET parcel_ref=EXCLUDED.parcel_ref RETURNING id::text`, plan.GroupRef, p.ParcelRef, p.KarryGoParcelRef, p.ScannableCode, decl, p.PackagePolicyVersion).Scan(&id)
		if err != nil {
			return err
		}
		parcelIDs[p.ParcelRef] = id
	}
	for _, m := range plan.Missions {
		capacity, _ := json.Marshal(map[string]any{"max_weight_grams": in.MaxWeightGrams, "max_volume_ml": in.MaxVolumeML})
		var missionID string
		missionSourceRef := fmt.Sprintf("%s:plan:%d:mission:%d", in.SourceRef, plan.Version, m.Index)
		err := tx.QueryRow(ctx, `INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref,source_version,state,plan_id,plan_mission_index,vehicle_class,capacity_snapshot,logical_mission_ref) VALUES($1,$2,'VENDOR_COMMERCE',$3,$4,'PLANNED',$5,$6,$7,$8,$9) RETURNING id::text`, m.MissionRef, plan.GroupRef, missionSourceRef, in.SourceVersion, planID, m.Index, in.VehicleClass, capacity, m.LogicalMissionRef).Scan(&missionID)
		if err != nil {
			return err
		}
		stopIDs := map[string]string{}
		for _, s := range m.Stops {
			location, _ := json.Marshal(s.Location)
			var stopID string
			err = tx.QueryRow(ctx, `INSERT INTO dispatch_mission_stops(mission_id,sequence,stop_type,location_snapshot,stop_ref,readiness_from,readiness_to) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, missionID, s.Sequence, s.Type, location, s.StopRef, nullTime(s.Ready.From), nullTime(s.Ready.To)).Scan(&stopID)
			if err != nil {
				return err
			}
			stopIDs[s.StopRef] = stopID
		}
		var manifestID string
		if err = tx.QueryRow(ctx, `INSERT INTO dispatch_parcel_manifests(mission_id,manifest_ref,version,status,policy_version) VALUES($1,$2,$3,'ACTIVE',$4) RETURNING id::text`, missionID, m.ManifestRef, m.ManifestVersion, PolicyVersion).Scan(&manifestID); err != nil {
			return err
		}
		for idx, ref := range m.ParcelRefs {
			id := parcelIDs[ref]
			if id == "" {
				return fmt.Errorf("parcel %s missing from declaration", ref)
			}
			if _, err = tx.Exec(ctx, `INSERT INTO dispatch_manifest_parcels(manifest_id,parcel_id,ordinal) VALUES($1,$2,$3)`, manifestID, id, idx+1); err != nil {
				return err
			}
		}
		for _, s := range m.Stops {
			for _, ref := range s.ParcelRefs {
				if _, err = tx.Exec(ctx, `INSERT INTO dispatch_stop_expected_parcels(mission_stop_id,parcel_id,manifest_id) VALUES($1,$2,$3)`, stopIDs[s.StopRef], parcelIDs[ref], manifestID); err != nil {
					return err
				}
			}
		}
		if err = insertExecutionBridge(ctx, tx, in, *plan, m, missionID, missionSourceRef, fp); err != nil {
			return err
		}
	}
	return nil
}

func insertExecutionBridge(ctx context.Context, tx pgx.Tx, in PlanRequest, plan Plan, mission Mission, missionID, missionSourceRef, fp string) error {
	if len(mission.Stops) < 2 {
		return nil
	}
	pickup, dropoff := mission.Stops[0].Location, mission.Stops[len(mission.Stops)-1].Location
	bookingID := uuid.NewString()
	fare := in.FareAmount / int64(len(plan.Missions))
	if mission.Index == 1 {
		fare += in.FareAmount % int64(len(plan.Missions))
	}
	if fare <= 0 {
		return errors.New("accepted commercial amount cannot produce a valid mission fare")
	}
	var weightGrams int64
	parcelSet := map[string]bool{}
	for _, ref := range mission.ParcelRefs {
		parcelSet[ref] = true
	}
	for _, parcel := range in.Parcels {
		if parcelSet[parcel.ParcelRef] {
			weightGrams += parcel.WeightGrams
		}
	}
	now := time.Now().UTC()
	packageDesc := "KarryGo parcels " + strings.Join(mission.ParcelRefs, ",")
	_, err := tx.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,distance_km,fare_amount,currency,payment_method,payment_status,payment_ref,receiver_name,receiver_phone,package_desc,package_weight,package_type,package_size,is_fragile,service_tier,rollout_protection_mode,rollout_policy_version,handover_required,pickup_evidence_required,delivery_evidence_required,provider_transfer_required) VALUES($1,$2,'searching',$3,$4,$5,$6,$7,$8,$9,$10,$11,'wallet','held',$12,$13,$14,$15,$16,'commerce_manifest','large',false,$17,$18,$19,$20,$21,$22,$23)`, bookingID, in.CustomerID, pickup.Address, pickup.Latitude, pickup.Longitude, dropoff.Address, dropoff.Latitude, dropoff.Longitude, routeDistance(plan.RouteEvidence), fare, strings.ToUpper(in.Currency), in.PaymentRef, in.ReceiverName, in.ReceiverPhone, packageDesc, float64(weightGrams)/1000, normalizeTier(in.ServiceClass), in.Protection.Mode, in.Protection.PolicyVersion, in.Protection.HandoverRequired, in.Protection.PickupEvidenceRequired, in.Protection.DeliveryEvidenceRequired, in.Protection.ProviderTransferRequired)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"correlation_id": in.CorrelationID, "booking_id": bookingID, "customer_id": in.CustomerID, "pickup_lat": pickup.Latitude, "pickup_lng": pickup.Longitude, "dropoff_lat": dropoff.Latitude, "dropoff_lng": dropoff.Longitude, "pickup_address": pickup.Address, "dropoff_address": dropoff.Address, "fare_amount": fare, "service_type": "dispatch", "payment_method": "wallet", "currency": strings.ToUpper(in.Currency), "receiver_name": in.ReceiverName, "receiver_phone": in.ReceiverPhone, "package_desc": packageDesc, "package_weight": float64(weightGrams) / 1000, "package_type": "commerce_manifest", "package_size": "large", "is_fragile": false, "service_tier": normalizeTier(in.ServiceClass), "occurred_at": now})
	_, err = tx.Exec(ctx, `INSERT INTO request_broadcasts(booking_id,service_type,broadcast_radius_km,status,broadcast_at,expires_at,booking_payload) VALUES($1,'dispatch',1,'cancelled',$2::timestamptz,$2::timestamptz+interval '5 minutes',$3)`, bookingID, now, payload)
	if err != nil {
		return err
	}
	operation := in.OperationRef + ":execution:" + mission.LogicalMissionRef
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_mission_execution_links(mission_id,booking_id,source_type,source_ref,source_version,operation_ref,request_fingerprint) VALUES($1,$2,'VENDOR_COMMERCE',$3,$4,$5,$6)`, missionID, bookingID, missionSourceRef, in.SourceVersion, operation, fp)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET booking_id=$2 WHERE id=$1`, missionID, bookingID)
	return err
}

func routeDistance(evidence *RouteEvidence) float64 {
	if evidence == nil {
		return 0
	}
	return evidence.DistanceKM
}
func normalizeTier(class string) string {
	if strings.EqualFold(class, "EXPRESS") {
		return "express"
	}
	return "standard"
}
func nullTime(t interface{ IsZero() bool }) any {
	if t.IsZero() {
		return nil
	}
	return t
}
func (r *Repository) Read(ctx context.Context, groupRef string) (Plan, error) {
	return readPlan(ctx, r.db, groupRef)
}

func (r *Repository) ReadBySource(ctx context.Context, sourceRef string, sourceVersion int64) (Plan, error) {
	var groupRef string
	err := r.db.QueryRow(ctx, `SELECT id::text FROM dispatch_delivery_groups WHERE source_type='VENDOR_COMMERCE' AND source_ref=$1 AND source_version=$2`, sourceRef, sourceVersion).Scan(&groupRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	return r.Read(ctx, groupRef)
}

func (r *Repository) ActivateManifestCustody(ctx context.Context, manifestRef string, command CustodyCommand, fp string) error {
	return r.changeCustody(ctx, manifestRef, command, fp, "ACTIVATE")
}
func (r *Repository) ReleaseManifestCustody(ctx context.Context, manifestRef string, command CustodyCommand, fp string) error {
	return r.changeCustody(ctx, manifestRef, command, fp, "RELEASE")
}
func (r *Repository) changeCustody(ctx context.Context, manifestRef string, command CustodyCommand, fp, action string) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var storedFP string
	err = tx.QueryRow(ctx, `SELECT request_fingerprint FROM dispatch_manifest_custody_operations WHERE operation_ref=$1`, command.OperationRef).Scan(&storedFP)
	if err == nil {
		if storedFP != fp {
			return ErrConflict
		}
		return tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var manifestID, missionID, status string
	err = tx.QueryRow(ctx, `SELECT pm.id::text,m.id::text,pm.status FROM dispatch_parcel_manifests pm JOIN dispatch_delivery_missions m ON m.id=pm.mission_id JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id WHERE pm.manifest_ref=$1 AND pm.version=$2 AND m.id=$3 AND g.source_type='VENDOR_COMMERCE' AND g.source_ref=$4 AND g.source_version=$5 FOR UPDATE`, manifestRef, command.ManifestVersion, command.MissionRef, command.SourceRef, command.SourceVersion).Scan(&manifestID, &missionID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if action == "ACTIVATE" {
		if status != "ACTIVE" && status != "CUSTODY_ACTIVE" {
			return ErrReplanBlocked
		}
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_active_custody_manifest_parcels(parcel_id,manifest_id,mission_id) SELECT parcel_id,$1,$2 FROM dispatch_manifest_parcels WHERE manifest_id=$1`, manifestID, missionID)
		if err != nil {
			return ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE dispatch_parcel_manifests SET status='CUSTODY_ACTIVE' WHERE id=$1`, manifestID); err != nil {
			return err
		}
	} else {
		if status != "CUSTODY_ACTIVE" && status != "COMPLETED" {
			return ErrConflict
		}
		if _, err = tx.Exec(ctx, `DELETE FROM dispatch_active_custody_manifest_parcels WHERE manifest_id=$1`, manifestID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE dispatch_parcel_manifests SET status='COMPLETED' WHERE id=$1`, manifestID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dispatch_manifest_custody_operations(operation_ref,request_fingerprint,manifest_id,action) VALUES($1,$2,$3,$4)`, command.OperationRef, fp, manifestID, action); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readPlan(ctx context.Context, q rowQuerier, groupRef string) (Plan, error) {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT plan_snapshot FROM dispatch_mission_plans WHERE delivery_group_id=$1 AND status='ACTIVE'`, groupRef).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	var p Plan
	err = json.Unmarshal(raw, &p)
	return p, err
}
func emitPlan(ctx context.Context, tx pgx.Tx, event string, plan Plan) error {
	raw, _ := json.Marshal(plan)
	sum := sha256.Sum256(raw)
	_, err := tx.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES($1,'delivery_group',$2,$3,$4,$5,'vendor-service') ON CONFLICT DO NOTHING`, event, plan.GroupRef, plan.Version, raw, hex.EncodeToString(sum[:]))
	return err
}
