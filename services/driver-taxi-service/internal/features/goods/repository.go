package goods

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/taxi-service/internal/movement"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/handover"
)

type Repository struct {
	db       *pgxpool.Pool
	custody  *custodyops.Repository
	movement *movement.Writer
}

// NewRepository shares the database with the service's TAXI_GOODS custody
// ledger so stop handovers and custody commit in one transaction.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, custody: custodyops.NewRepository(db, "TAXI_GOODS")}
}

// WithMovementEvents publishes customer movement summaries for commerce
// goods missions in the same transaction as each lifecycle event.
func (r *Repository) WithMovementEvents(writer *movement.Writer) *Repository {
	r.movement = writer
	return r
}

func Fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (r *Repository) ClaimNonce(ctx context.Context, caller, nonce, path string, expiresAt time.Time) error {
	if r == nil || r.db == nil || strings.TrimSpace(caller) == "" || strings.TrimSpace(nonce) == "" {
		return ErrInvalid
	}
	_, _ = r.db.Exec(ctx, `DELETE FROM taxi_service_nonces WHERE expires_at < now()`)
	_, err := r.db.Exec(ctx, `INSERT INTO taxi_service_nonces(caller_service,nonce,request_path,expires_at) VALUES($1,$2,$3,$4)`, caller, nonce, path, expiresAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}

func (r *Repository) GetProviderSettings(ctx context.Context, providerID string) (ProviderSettings, error) {
	var out ProviderSettings
	err := r.db.QueryRow(ctx, `SELECT provider_id::text,capability_status,goods_available,version FROM taxi_goods_provider_settings WHERE provider_id=$1`, providerID).
		Scan(&out.ProviderID, &out.CapabilityStatus, &out.GoodsAvailable, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderSettings{ProviderID: providerID, CapabilityStatus: CapabilityNotApproved, Version: 0}, nil
	}
	return out, err
}

func (r *Repository) SetGoodsAvailability(ctx context.Context, providerID string, enabled bool, expectedVersion int64) (ProviderSettings, error) {
	var out ProviderSettings
	err := r.db.QueryRow(ctx, `
		INSERT INTO taxi_goods_provider_settings(provider_id,goods_available,version)
		VALUES($1,$2,1)
		ON CONFLICT(provider_id) DO UPDATE SET goods_available=$2,version=taxi_goods_provider_settings.version+1,updated_at=now()
		WHERE taxi_goods_provider_settings.version=$3
		RETURNING provider_id::text,capability_status,goods_available,version`, providerID, enabled, expectedVersion).
		Scan(&out.ProviderID, &out.CapabilityStatus, &out.GoodsAvailable, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderSettings{}, ErrConflict
	}
	return out, err
}

func (r *Repository) SetCapability(ctx context.Context, providerID, status, operator string, expectedVersion int64) (ProviderSettings, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != CapabilityApproved && status != CapabilityNotApproved && status != CapabilitySuspended || strings.TrimSpace(operator) == "" {
		return ProviderSettings{}, ErrInvalid
	}
	var out ProviderSettings
	err := r.db.QueryRow(ctx, `
		INSERT INTO taxi_goods_provider_settings(provider_id,capability_status,approved_at,approved_by,version)
		SELECT id,$2,CASE WHEN $2='APPROVED' THEN now() ELSE NULL END,$3,1 FROM taxi_providers WHERE id=$1
		ON CONFLICT(provider_id) DO UPDATE SET capability_status=$2,
			approved_at=CASE WHEN $2='APPROVED' THEN now() ELSE NULL END,approved_by=$3,
			goods_available=CASE WHEN $2='APPROVED' THEN taxi_goods_provider_settings.goods_available ELSE FALSE END,
			version=taxi_goods_provider_settings.version+1,updated_at=now()
		WHERE taxi_goods_provider_settings.version=$4
		RETURNING provider_id::text,capability_status,goods_available,version`, providerID, status, operator, expectedVersion).
		Scan(&out.ProviderID, &out.CapabilityStatus, &out.GoodsAvailable, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderSettings{}, ErrConflict
	}
	return out, err
}

func (r *Repository) GetCargoProfile(ctx context.Context, providerID, vehicleID string) (CargoProfile, error) {
	var out CargoProfile
	err := r.db.QueryRow(ctx, `SELECT p.vehicle_id::text,p.max_weight_grams,p.max_volume_ml,p.max_package_count,p.longest_dimension_mm,p.handling_capabilities,p.verified,p.version
		FROM taxi_vehicle_cargo_profiles p JOIN vehicles v ON v.id=p.vehicle_id WHERE p.vehicle_id=$1 AND v.provider_id=$2`, vehicleID, providerID).
		Scan(&out.VehicleID, &out.MaxWeightGrams, &out.MaxVolumeML, &out.MaxPackageCount, &out.LongestDimensionMM, &out.HandlingCapabilities, &out.Verified, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return CargoProfile{}, ErrNotFound
	}
	return out, err
}

func (r *Repository) PutCargoProfile(ctx context.Context, providerID string, input CargoProfile, expectedVersion int64) (CargoProfile, error) {
	if err := input.Validate(); err != nil {
		return CargoProfile{}, err
	}
	var out CargoProfile
	err := r.db.QueryRow(ctx, `
		INSERT INTO taxi_vehicle_cargo_profiles(vehicle_id,max_weight_grams,max_volume_ml,max_package_count,longest_dimension_mm,handling_capabilities,verified,version)
		SELECT v.id,$3,$4,$5,$6,$7,FALSE,1 FROM vehicles v WHERE v.id=$1 AND v.provider_id=$2
		ON CONFLICT(vehicle_id) DO UPDATE SET max_weight_grams=$3,max_volume_ml=$4,max_package_count=$5,longest_dimension_mm=$6,
			handling_capabilities=$7,verified=FALSE,verified_at=NULL,verified_by='',version=taxi_vehicle_cargo_profiles.version+1,updated_at=now()
		WHERE taxi_vehicle_cargo_profiles.version=$8
		RETURNING vehicle_id::text,max_weight_grams,max_volume_ml,max_package_count,longest_dimension_mm,handling_capabilities,verified,version`,
		input.VehicleID, providerID, input.MaxWeightGrams, input.MaxVolumeML, input.MaxPackageCount, input.LongestDimensionMM, canonical(input.HandlingCapabilities), expectedVersion).
		Scan(&out.VehicleID, &out.MaxWeightGrams, &out.MaxVolumeML, &out.MaxPackageCount, &out.LongestDimensionMM, &out.HandlingCapabilities, &out.Verified, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return CargoProfile{}, ErrConflict
	}
	return out, err
}

func (r *Repository) VerifyCargoProfile(ctx context.Context, providerID, vehicleID, operator string, verified bool, expectedVersion int64) (CargoProfile, error) {
	if strings.TrimSpace(operator) == "" {
		return CargoProfile{}, ErrInvalid
	}
	var out CargoProfile
	err := r.db.QueryRow(ctx, `UPDATE taxi_vehicle_cargo_profiles p SET verified=$4,
		verified_at=CASE WHEN $4 THEN now() ELSE NULL END,verified_by=$3,version=p.version+1,updated_at=now()
		FROM vehicles v WHERE p.vehicle_id=$2 AND v.id=p.vehicle_id AND v.provider_id=$1 AND p.version=$5
		RETURNING p.vehicle_id::text,p.max_weight_grams,p.max_volume_ml,p.max_package_count,p.longest_dimension_mm,p.handling_capabilities,p.verified,p.version`,
		providerID, vehicleID, operator, verified, expectedVersion).
		Scan(&out.VehicleID, &out.MaxWeightGrams, &out.MaxVolumeML, &out.MaxPackageCount, &out.LongestDimensionMM, &out.HandlingCapabilities, &out.Verified, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return CargoProfile{}, ErrConflict
	}
	return out, err
}

func (r *Repository) EligibleCandidates(ctx context.Context, ids []string, load commercequote.Load) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT p.id::text
		FROM taxi_providers p
		JOIN taxi_goods_provider_settings s ON s.provider_id=p.id
		JOIN vehicles v ON v.provider_id=p.id AND v.status='active'
		JOIN taxi_vehicle_cargo_profiles c ON c.vehicle_id=v.id AND c.verified
		JOIN provider_operational_state o ON o.provider_id=p.id AND o.state='AVAILABLE'
		WHERE p.id::text=ANY($1) AND p.account_status='active' AND p.onboarding_status='complete'
		  AND p.identity_verification_status='verified' AND p.vehicle_operational_status='active'
		  AND p.vehicle_verification_status='verified' AND p.document_compliance_status='compliant'
		  AND s.capability_status='APPROVED' AND s.goods_available AND p.accepts_parcels
		  AND c.max_weight_grams >= $2 AND c.max_volume_ml >= $3 AND c.max_package_count >= $4
		  AND c.longest_dimension_mm >= $5 AND c.handling_capabilities @> $6::text[]
		  AND taxi_admin_assignment_block(p.id::text,v.id::text,now()) IS NULL
		ORDER BY array_position($1::text[],p.id::text)`, ids, load.TotalWeightGrams, load.TotalVolumeMl, load.PackageCount, load.LongestDimensionMm, canonical(load.Capabilities))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) SaveQuote(ctx context.Context, caller string, request commercequote.Request, quote commercequote.Quote) (commercequote.Quote, error) {
	fingerprint := Fingerprint(request)
	var existing commercequote.Quote
	err := r.scanQuote(r.db.QueryRow(ctx, quoteSelect+` WHERE source_service=$1 AND quote_reference=$2 AND request_fingerprint=$3`, caller, request.QuoteReference, fingerprint), &existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return commercequote.Quote{}, err
	}
	var revision int64
	if err = r.db.QueryRow(ctx, `SELECT COALESCE(MAX(revision),0)+1 FROM taxi_goods_quotes WHERE source_service=$1 AND quote_reference=$2`, caller, request.QuoteReference).Scan(&revision); err != nil {
		return commercequote.Quote{}, err
	}
	quote.QuoteID, quote.Revision = uuid.NewString(), revision
	payload, _ := json.Marshal(request)
	_, err = r.db.Exec(ctx, `INSERT INTO taxi_goods_quotes(id,source_service,quote_reference,revision,request_fingerprint,request_payload,serviceable,reason_code,market_id,operating_zone_id,distance_km,estimated_minutes,fee_minor,currency,available_providers,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''),$11,$12,$13,$14,$15,$16)`, quote.QuoteID, caller, request.QuoteReference, revision, fingerprint, payload, quote.Serviceable, quote.ReasonCode, quote.PickupRef, quote.DropoffRef, quote.DistanceKm, quote.EstimatedMinutes, quote.FeeMinor, quote.Currency, quote.AvailableProviders, quote.ExpiresAt)
	if err != nil {
		return commercequote.Quote{}, err
	}
	quote.QuotedAt = time.Now().UTC()
	return quote, nil
}

const quoteSelect = `SELECT id::text,quote_reference,revision,serviceable,reason_code,COALESCE(market_id::text,''),COALESCE(operating_zone_id::text,''),distance_km,estimated_minutes,fee_minor,currency,available_providers,expires_at,created_at FROM taxi_goods_quotes`

func (r *Repository) scanQuote(row pgx.Row, out *commercequote.Quote) error {
	return row.Scan(&out.QuoteID, &out.QuoteReference, &out.Revision, &out.Serviceable, &out.ReasonCode, &out.PickupRef, &out.DropoffRef, &out.DistanceKm, &out.EstimatedMinutes, &out.FeeMinor, &out.Currency, &out.AvailableProviders, &out.ExpiresAt, &out.QuotedAt)
}

func (r *Repository) CreateMission(ctx context.Context, caller string, request commercemission.Request) (Mission, error) {
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

	var quoteID, quoteRef, quoteFP string
	var quotePayload []byte
	var serviceable bool
	var expires time.Time
	var fee int64
	err := r.db.QueryRow(ctx, `SELECT id::text,quote_reference,request_fingerprint,request_payload,serviceable,expires_at,fee_minor FROM taxi_goods_quotes WHERE id=$1 AND source_service=$2`, request.QuoteID, caller).
		Scan(&quoteID, &quoteRef, &quoteFP, &quotePayload, &serviceable, &expires, &fee)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrInvalid
	}
	if err != nil {
		return Mission{}, err
	}
	if !serviceable || !expires.After(time.Now()) || fee != request.AgreedFeeMinor || (request.QuoteReference != "" && quoteRef != request.QuoteReference) {
		return Mission{}, ErrConflict
	}
	var quoted commercequote.Request
	if json.Unmarshal(quotePayload, &quoted) != nil || !missionMatchesQuote(request, quoted) {
		return Mission{}, ErrConflict
	}

	pickup := request.Pickup
	if len(request.PickupStops) > 1 {
		return Mission{}, ErrInvalid
	}
	if len(request.PickupStops) == 1 {
		pickup = request.PickupStops[0].Location
	}
	pickupJSON, _ := json.Marshal(pickup)
	dropoffJSON, _ := json.Marshal(request.Dropoff)
	loadJSON, _ := json.Marshal(request.Load)
	manifestRef, manifestVersion := "", int64(0)
	if len(request.PickupStops) == 1 {
		manifestRef = request.PickupStops[0].ManifestRef
		manifestVersion = request.PickupStops[0].ManifestVersion
	}
	binding, err := ResolvePlanBinding(request)
	if err != nil {
		return Mission{}, err
	}
	missionID := uuid.NewString()
	operation := request.OperationReference()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Mission{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO taxi_goods_missions(id,source_service,mission_ref,operation_ref,request_fingerprint,parent_order_ref,vendor_order_refs,customer_ref,quote_id,plan_ref,plan_version,plan_fingerprint,mission_slot_ref,pickup,dropoff,receiver_ref,receiver_name,receiver_phone,load,handling,manifest_ref,manifest_version,agreed_fee_minor,currency,pickup_evidence_required,delivery_evidence_required,requested_at,plan_binding_mode)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)`,
		missionID, caller, request.MissionRef, operation, fp, request.ParentOrderRef, request.VendorOrderRefs, request.CustomerRef, quoteID, binding.PlanRef, binding.PlanVersion, binding.PlanFingerprint, binding.MissionSlotRef, pickupJSON, dropoffJSON, request.ReceiverCustomerRef, request.Dropoff.Contact, request.Dropoff.Phone, loadJSON, handlingCapabilities(request.Handling), manifestRef, manifestVersion, request.AgreedFeeMinor, strings.ToUpper(request.Currency), false, true, request.RequestedAt, binding.Mode)
	if err != nil {
		if strings.Contains(err.Error(), "uq_taxi_goods_plan_slot") {
			return Mission{}, ErrConflict
		}
		return Mission{}, err
	}
	seen := map[string]bool{}
	for _, parcel := range request.Parcels {
		if seen[parcel.ParcelRef] || strings.TrimSpace(parcel.ParcelRef) == "" || parcel.ManifestVersion < 1 {
			return Mission{}, ErrInvalid
		}
		seen[parcel.ParcelRef] = true
		_, err = tx.Exec(ctx, `INSERT INTO taxi_goods_parcels(mission_id,parcel_ref,vendor_ref,branch_ref,vendor_order_ref,manifest_ref,manifest_version,weight_grams,length_mm,width_mm,height_mm,package_count,handling) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, missionID, parcel.ParcelRef, parcel.VendorRef, parcel.BranchRef, parcel.VendorOrderRef, parcel.ManifestRef, parcel.ManifestVersion, parcel.WeightGrams, parcel.LengthMm, parcel.WidthMm, parcel.HeightMm, max64(parcel.PackageCount, 1), handlingCapabilities(parcel.Handling))
		if err != nil {
			return Mission{}, err
		}
	}
	if err = r.writeEvent(ctx, tx, missionID, "taxi.goods.mission.created.v1", 1, map[string]any{"mission_ref": request.MissionRef, "operation_ref": operation, "status": StatusMatching}); err != nil {
		return Mission{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Mission{}, err
	}
	return r.ReadMission(ctx, caller, request.MissionRef)
}

func (r *Repository) ReadMission(ctx context.Context, caller, missionRef string) (Mission, error) {
	var out Mission
	err := r.scanMission(r.db.QueryRow(ctx, missionSelect+` WHERE source_service=$1 AND mission_ref=$2`, caller, missionRef), &out)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrNotFound
	}
	return out, err
}

func (r *Repository) ReadMissionByOperation(ctx context.Context, caller, operationRef string) (Mission, error) {
	var out Mission
	err := r.scanMission(r.db.QueryRow(ctx, missionSelect+` WHERE source_service=$1 AND operation_ref=$2`, caller, operationRef), &out)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrNotFound
	}
	return out, err
}

func (r *Repository) ReadMissionForProvider(ctx context.Context, providerID, missionRef string) (Mission, error) {
	var out Mission
	err := r.scanMission(r.db.QueryRow(ctx, missionSelect+` WHERE provider_id=$1 AND mission_ref=$2`, providerID, missionRef), &out)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrNotFound
	}
	return out, err
}

func (r *Repository) ListProviderMissions(ctx context.Context, providerID string) ([]Mission, error) {
	rows, err := r.db.Query(ctx, missionSelect+` WHERE provider_id=$1 AND status NOT IN ('COMPLETED','CANCELLED','FAILED','UNMATCHED') ORDER BY created_at DESC`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mission
	for rows.Next() {
		var m Mission
		if err = r.scanMission(rows, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) MatchingMissions(ctx context.Context, limit int) ([]Mission, error) {
	rows, err := r.db.Query(ctx, missionSelect+` WHERE status='MATCHING' ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mission
	for rows.Next() {
		var mission Mission
		if err = r.scanMission(rows, &mission); err != nil {
			return nil, err
		}
		out = append(out, mission)
	}
	return out, rows.Err()
}

func (r *Repository) MissionLoad(ctx context.Context, missionID string) (commercequote.Load, error) {
	var raw []byte
	var missionHandling []string
	if err := r.db.QueryRow(ctx, `SELECT load,handling FROM taxi_goods_missions WHERE id=$1`, missionID).Scan(&raw, &missionHandling); err != nil {
		return commercequote.Load{}, err
	}
	var base commercemission.Load
	if err := json.Unmarshal(raw, &base); err != nil {
		return commercequote.Load{}, err
	}
	load := commercequote.Load{TotalWeightGrams: base.WeightGrams, PackageCount: base.PackageCount, Capabilities: canonical(missionHandling)}
	rows, err := r.db.Query(ctx, `SELECT weight_grams,length_mm,width_mm,height_mm,package_count,handling FROM taxi_goods_parcels WHERE mission_id=$1`, missionID)
	if err != nil {
		return commercequote.Load{}, err
	}
	defer rows.Close()
	seen := false
	for rows.Next() {
		var weight, length, width, height, packages int64
		var handling []string
		if err = rows.Scan(&weight, &length, &width, &height, &packages, &handling); err != nil {
			return commercequote.Load{}, err
		}
		if !seen {
			load.TotalWeightGrams, load.PackageCount = 0, 0
			seen = true
		}
		load.TotalWeightGrams += weight
		load.PackageCount += packages
		load.TotalVolumeMl += length * width * height / 1000
		if longest := max64(length, max64(width, height)); int(longest) > load.LongestDimensionMm {
			load.LongestDimensionMm = int(longest)
		}
		load.Capabilities = append(load.Capabilities, handling...)
	}
	load.Capabilities = canonical(load.Capabilities)
	return load, rows.Err()
}

func (r *Repository) ExpireChallenges(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `UPDATE taxi_goods_handover_challenges SET status='EXPIRED',updated_at=now() WHERE status='ACTIVE' AND expires_at<=now()`)
	return err
}

const missionSelect = `SELECT m.id::text,m.mission_ref,m.operation_ref,m.parent_order_ref,m.customer_ref,m.receiver_ref,m.vendor_order_refs,COALESCE(m.provider_id::text,''),COALESCE(m.vehicle_id::text,''),m.status,m.version,m.agreed_fee_minor,m.currency,m.pickup,m.dropoff,(SELECT count(*) FROM taxi_goods_parcels p WHERE p.mission_id=m.id),m.manifest_ref,m.manifest_version,COALESCE((SELECT status FROM taxi_goods_handover_challenges h WHERE h.mission_id=m.id AND h.stage='PICKUP' ORDER BY version DESC LIMIT 1),''),COALESCE((SELECT status FROM taxi_goods_handover_challenges h WHERE h.mission_id=m.id AND h.stage='DELIVERY' ORDER BY version DESC LIMIT 1),''),m.pickup_evidence_required,m.delivery_evidence_required,m.pickup_evidence_ref,m.delivery_evidence_ref,m.settlement_status,m.requested_at,m.created_at FROM taxi_goods_missions m`

func (r *Repository) scanMission(row pgx.Row, out *Mission) error {
	var pickup, dropoff []byte
	if err := row.Scan(&out.ID, &out.MissionRef, &out.OperationRef, &out.ParentOrderRef, &out.CustomerRef, &out.ReceiverRef, &out.VendorOrderRefs, &out.ProviderID, &out.VehicleID, &out.Status, &out.Version, &out.AgreedFeeMinor, &out.Currency, &pickup, &dropoff, &out.ParcelCount, &out.ManifestRef, &out.ManifestVersion, &out.PickupHandoverStatus, &out.DeliveryHandoverStatus, &out.PickupEvidenceRequired, &out.DeliveryEvidenceRequired, &out.PickupEvidenceRef, &out.DeliveryEvidenceRef, &out.SettlementStatus, &out.RequestedAt, &out.CreatedAt); err != nil {
		return err
	}
	_ = json.Unmarshal(pickup, &out.Pickup)
	_ = json.Unmarshal(dropoff, &out.Dropoff)
	return nil
}

func (r *Repository) ReadMissionForOperations(ctx context.Context, missionRef string) (Mission, error) {
	var out Mission
	err := r.scanMission(r.db.QueryRow(ctx, missionSelect+` WHERE mission_ref=$1`, missionRef), &out)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrNotFound
	}
	return out, err
}

func (r *Repository) Offer(ctx context.Context, missionID, providerID, vehicleID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var version int64
	err = tx.QueryRow(ctx, `UPDATE provider_operational_state SET state='OFFERED',reason='taxi_goods_offered',booking_id=NULL,goods_mission_id=$2,effective_at=now(),version=version+1 WHERE provider_id=$1 AND state='AVAILABLE' AND taxi_admin_assignment_block($1::text,$3::text,now()) IS NULL RETURNING version`, providerID, missionID, vehicleID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO provider_operational_state_history(provider_id,from_state,to_state,reason,booking_id,goods_mission_id) VALUES($1,'AVAILABLE','OFFERED','taxi_goods_offered',NULL,$2)`, providerID, missionID); err != nil {
		return err
	}
	var missionVersion int64
	err = tx.QueryRow(ctx, `UPDATE taxi_goods_missions SET provider_id=$2,vehicle_id=$3,status='OFFERED',offered_at=now(),version=version+1,updated_at=now() WHERE id=$1 AND status='MATCHING' RETURNING version`, missionID, providerID, vehicleID).Scan(&missionVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if err = r.writeEvent(ctx, tx, missionID, "taxi.goods.offer.created.v1", missionVersion, map[string]any{"mission_id": missionID, "status": StatusOffered}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) CandidateVehicle(ctx context.Context, providerID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT v.id::text FROM vehicles v JOIN taxi_vehicle_cargo_profiles c ON c.vehicle_id=v.id AND c.verified WHERE v.provider_id=$1 AND v.status='active' AND taxi_admin_assignment_block($1::text,v.id::text,now()) IS NULL ORDER BY v.created_at LIMIT 1`, providerID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

func (r *Repository) TransitionProvider(ctx context.Context, missionRef, providerID, from, to, opState, event string) (Mission, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Mission{}, err
	}
	defer tx.Rollback(ctx)
	var missionID string
	var version int64
	err = tx.QueryRow(ctx, `UPDATE taxi_goods_missions SET status=$4,version=version+1,updated_at=now(),accepted_at=CASE WHEN $4='ASSIGNED' THEN now() ELSE accepted_at END,picked_up_at=CASE WHEN $4='PICKED_UP' THEN now() ELSE picked_up_at END,delivered_at=CASE WHEN $4='DELIVERED' THEN now() ELSE delivered_at END,completed_at=CASE WHEN $4='COMPLETED' THEN now() ELSE completed_at END,manifest_frozen_at=CASE WHEN $4='PICKED_UP' THEN COALESCE(manifest_frozen_at,now()) ELSE manifest_frozen_at END,settlement_status=CASE WHEN $4='COMPLETED' THEN 'PENDING' ELSE settlement_status END,settlement_next_attempt_at=CASE WHEN $4='COMPLETED' THEN now() ELSE settlement_next_attempt_at END WHERE mission_ref=$1 AND provider_id=$2 AND status=$3 RETURNING id::text,version`, missionRef, providerID, from, to).Scan(&missionID, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrConflict
	}
	if err != nil {
		return Mission{}, err
	}
	if opState != "" {
		tag, transitionErr := tx.Exec(ctx, `UPDATE provider_operational_state SET state=$3,reason=$4,booking_id=NULL,goods_mission_id=CASE WHEN $3='AVAILABLE' THEN NULL ELSE $2::uuid END,effective_at=now(),version=version+1 WHERE provider_id=$1 AND goods_mission_id=$2::uuid`, providerID, missionID, opState, "taxi_goods_"+strings.ToLower(to))
		if transitionErr != nil {
			return Mission{}, transitionErr
		}
		if tag.RowsAffected() != 1 {
			return Mission{}, ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO provider_operational_state_history(provider_id,from_state,to_state,reason,booking_id,goods_mission_id) VALUES($1,$2,$3,$4,NULL,$5)`, providerID, operationalStateForMissionStatus(from), opState, "taxi_goods_"+strings.ToLower(to), missionID); err != nil {
			return Mission{}, err
		}
	}
	if to == StatusPickedUp {
		_, err = tx.Exec(ctx, `UPDATE taxi_goods_parcels SET custody_status='PICKED_UP',updated_at=now() WHERE mission_id=$1`, missionID)
	} else if to == StatusDelivered {
		_, err = tx.Exec(ctx, `UPDATE taxi_goods_parcels SET custody_status='DELIVERED',updated_at=now() WHERE mission_id=$1`, missionID)
	}
	if err != nil {
		return Mission{}, err
	}
	if err = r.writeEvent(ctx, tx, missionID, event, version, map[string]any{"mission_ref": missionRef, "status": to}); err != nil {
		return Mission{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Mission{}, err
	}
	return r.ReadMissionForProvider(ctx, providerID, missionRef)
}

func (r *Repository) Cancel(ctx context.Context, caller, missionRef, reason string, expectedVersion int64) (Mission, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Mission{}, err
	}
	defer tx.Rollback(ctx)
	var id, provider string
	var version int64
	err = tx.QueryRow(ctx, `UPDATE taxi_goods_missions SET status='CANCELLED',cancel_reason=$4,cancelled_at=now(),version=version+1,updated_at=now() WHERE source_service=$1 AND mission_ref=$2 AND version=$3 AND status IN ('MATCHING','OFFERED','ASSIGNED','EN_ROUTE_PICKUP','AT_PICKUP','PICKUP_HANDOVER_REQUIRED') RETURNING id::text,COALESCE(provider_id::text,''),version`, caller, missionRef, expectedVersion, reason).Scan(&id, &provider, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrConflict
	}
	if err != nil {
		return Mission{}, err
	}
	if provider != "" {
		_, err = tx.Exec(ctx, `UPDATE provider_operational_state SET state='AVAILABLE',reason='taxi_goods_cancelled',goods_mission_id=NULL,effective_at=now(),version=version+1 WHERE provider_id=$1 AND goods_mission_id=$2`, provider, id)
		if err != nil {
			return Mission{}, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE taxi_goods_handover_challenges SET status='INVALIDATED',invalidated_reason='MISSION_CANCELLED',updated_at=now() WHERE mission_id=$1 AND status='ACTIVE'`, id)
	if err != nil {
		return Mission{}, err
	}
	if err = r.writeEvent(ctx, tx, id, "taxi.goods.mission.cancelled.v1", version, map[string]any{"mission_ref": missionRef, "status": StatusCancelled, "reason_code": reason}); err != nil {
		return Mission{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Mission{}, err
	}
	return r.ReadMission(ctx, caller, missionRef)
}

func (r *Repository) ReassignProvider(ctx context.Context, missionRef, operator string, command ReassignProviderCommand, requestFingerprint string, fresh *HandoverRecord) (Mission, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Mission{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if prior, found, priorErr := bindingOperation(ctx, tx, command.OperationRef, requestFingerprint); priorErr != nil {
		return Mission{}, false, priorErr
	} else if found {
		var mission Mission
		if err = r.scanMission(tx.QueryRow(ctx, missionSelect+` WHERE m.id=$1`, prior), &mission); err != nil {
			return Mission{}, false, err
		}
		return mission, false, tx.Commit(ctx)
	}

	var missionID, oldProvider, status string
	var version int64
	err = tx.QueryRow(ctx, `SELECT id::text,COALESCE(provider_id::text,''),status,version FROM taxi_goods_missions WHERE mission_ref=$1 FOR UPDATE`, missionRef).
		Scan(&missionID, &oldProvider, &status, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, false, ErrNotFound
	}
	if err != nil {
		return Mission{}, false, err
	}
	if version != command.ExpectedVersion || oldProvider == "" || oldProvider == command.NewProviderID || !reassignableStatus(status) {
		return Mission{}, false, ErrConflict
	}
	var eligible bool
	// Legacy single-pickup missions require a verified cargo profile. Plan-bound
	// multi-pickup missions were matched by the capability engine, so the
	// replacement vehicle must instead hold an unexpired ELIGIBLE grant for the
	// mission's product.
	err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM taxi_goods_provider_settings s
		JOIN vehicles v ON v.provider_id=s.provider_id AND v.id=$2 AND v.status='active'
		WHERE s.provider_id=$1 AND s.capability_status='APPROVED' AND s.goods_available
		  AND (EXISTS(SELECT 1 FROM taxi_vehicle_cargo_profiles c WHERE c.vehicle_id=v.id AND c.verified)
		    OR EXISTS(SELECT 1 FROM taxi_goods_missions m JOIN taxi_vehicle_product_eligibility e
		              ON e.vehicle_id=v.id AND e.product_code=m.service_product_code AND e.status='ELIGIBLE'
		             AND (e.effective_at IS NULL OR e.effective_at<=now()) AND (e.expires_at IS NULL OR e.expires_at>now())
		             WHERE m.id=$3 AND m.custody_mode='MULTI_MANIFEST')))`, command.NewProviderID, command.NewVehicleID, missionID).Scan(&eligible)
	if err != nil || !eligible {
		if err != nil {
			return Mission{}, false, err
		}
		return Mission{}, false, ErrConflict
	}

	type stateBinding struct{ state, booking, goods string }
	bindings := map[string]stateBinding{}
	providerIDs := []string{oldProvider, command.NewProviderID}
	sort.Strings(providerIDs)
	for _, providerID := range providerIDs {
		var binding stateBinding
		err = tx.QueryRow(ctx, `SELECT state,COALESCE(booking_id::text,''),COALESCE(goods_mission_id::text,'') FROM provider_operational_state WHERE provider_id=$1 FOR UPDATE`, providerID).
			Scan(&binding.state, &binding.booking, &binding.goods)
		if err != nil {
			return Mission{}, false, ErrConflict
		}
		bindings[providerID] = binding
	}
	if bindings[oldProvider].goods != missionID || bindings[command.NewProviderID].state != "AVAILABLE" || bindings[command.NewProviderID].booking != "" || bindings[command.NewProviderID].goods != "" {
		return Mission{}, false, ErrConflict
	}

	if _, err = tx.Exec(ctx, `UPDATE provider_operational_state SET state='AVAILABLE',reason='taxi_goods_reassigned',goods_mission_id=NULL,booking_id=NULL,effective_at=now(),version=version+1 WHERE provider_id=$1`, oldProvider); err != nil {
		return Mission{}, false, err
	}
	newState := operationalStateForMissionStatus(status)
	if _, err = tx.Exec(ctx, `UPDATE provider_operational_state SET state=$2,reason='taxi_goods_reassigned',goods_mission_id=$3,booking_id=NULL,effective_at=now(),version=version+1 WHERE provider_id=$1`, command.NewProviderID, newState, missionID); err != nil {
		return Mission{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO provider_operational_state_history(provider_id,from_state,to_state,reason,booking_id,goods_mission_id) VALUES($1,$2,'AVAILABLE','taxi_goods_reassigned',NULL,NULL),($3,'AVAILABLE',$4,'taxi_goods_reassigned',NULL,$5)`, oldProvider, bindings[oldProvider].state, command.NewProviderID, newState, missionID); err != nil {
		return Mission{}, false, err
	}
	var resultingVersion int64
	if err = tx.QueryRow(ctx, `UPDATE taxi_goods_missions SET provider_id=$2,vehicle_id=$3,version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, missionID, command.NewProviderID, command.NewVehicleID).Scan(&resultingVersion); err != nil {
		return Mission{}, false, err
	}
	if fresh != nil {
		if _, err = tx.Exec(ctx, `INSERT INTO taxi_goods_handover_challenges(challenge_ref,mission_id,stage,provider_id,receiver_ref,otp_hash,qr_hash,version,status,attempts,resend_count,expires_at,resend_available_at,stop_ref) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'ACTIVE',0,$9,$10,$11,$12)`, fresh.ChallengeRef, missionID, fresh.Stage, command.NewProviderID, fresh.ReceiverRef, []byte(fresh.OTPHash), []byte(fresh.QRHash), fresh.Version, fresh.ResendCount, fresh.ExpiresAt, fresh.ResendAvailableAt, fresh.StopRef); err != nil {
			return Mission{}, false, err
		}
	}
	if err = r.writeEvent(ctx, tx, missionID, "taxi.goods.provider.reassigned.v1", resultingVersion, map[string]any{"mission_ref": missionRef, "status": status, "provider_reassigned": true}); err != nil {
		return Mission{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_admin_audit_events(operator_id,action,entity_type,entity_id,reason,before,after) VALUES($1,'taxi.goods.provider.reassigned','taxi_goods_mission',$2,$3,jsonb_build_object('provider_id',$4::text),jsonb_build_object('provider_id',$5::text))`, operator, missionID, command.OperationRef, oldProvider, command.NewProviderID); err != nil {
		return Mission{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_goods_binding_operations(mission_id,operation_ref,operation_type,request_fingerprint,previous_provider_id,new_provider_id,resulting_mission_version) VALUES($1,$2,'PROVIDER_REASSIGNMENT',$3,$4,$5,$6)`, missionID, command.OperationRef, requestFingerprint, oldProvider, command.NewProviderID, resultingVersion); err != nil {
		return Mission{}, false, err
	}
	var mission Mission
	if err = r.scanMission(tx.QueryRow(ctx, missionSelect+` WHERE m.id=$1`, missionID), &mission); err != nil {
		return Mission{}, false, err
	}
	return mission, true, tx.Commit(ctx)
}

func (r *Repository) ChangeReceiver(ctx context.Context, caller, missionRef, operator string, command ReceiverChangeCommand, requestFingerprint string, fresh *HandoverRecord) (Mission, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Mission{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if prior, found, priorErr := bindingOperation(ctx, tx, command.OperationRef, requestFingerprint); priorErr != nil {
		return Mission{}, false, priorErr
	} else if found {
		var mission Mission
		if err = r.scanMission(tx.QueryRow(ctx, missionSelect+` WHERE m.id=$1`, prior), &mission); err != nil {
			return Mission{}, false, err
		}
		return mission, false, tx.Commit(ctx)
	}
	var missionID, previousReceiver, status string
	var version int64
	err = tx.QueryRow(ctx, `SELECT id::text,receiver_ref,status,version FROM taxi_goods_missions WHERE mission_ref=$1 AND source_service=$2 FOR UPDATE`, missionRef, caller).Scan(&missionID, &previousReceiver, &status, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, false, ErrNotFound
	}
	if err != nil {
		return Mission{}, false, err
	}
	if version != command.ExpectedVersion || !receiverChangeableStatus(status) || previousReceiver == command.ReceiverRef {
		return Mission{}, false, ErrConflict
	}
	var resultingVersion int64
	if err = tx.QueryRow(ctx, `UPDATE taxi_goods_missions SET receiver_ref=$2,receiver_name=$3,receiver_phone=$4,version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, missionID, command.ReceiverRef, command.ReceiverName, command.ReceiverPhone).Scan(&resultingVersion); err != nil {
		return Mission{}, false, err
	}
	if fresh != nil {
		if _, err = tx.Exec(ctx, `INSERT INTO taxi_goods_handover_challenges(challenge_ref,mission_id,stage,provider_id,receiver_ref,otp_hash,qr_hash,version,status,attempts,resend_count,expires_at,resend_available_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'ACTIVE',0,$9,$10,$11)`, fresh.ChallengeRef, missionID, fresh.Stage, fresh.ProviderID, command.ReceiverRef, []byte(fresh.OTPHash), []byte(fresh.QRHash), fresh.Version, fresh.ResendCount, fresh.ExpiresAt, fresh.ResendAvailableAt); err != nil {
			return Mission{}, false, err
		}
	}
	if err = r.writeEvent(ctx, tx, missionID, "taxi.goods.receiver.changed.v1", resultingVersion, map[string]any{"mission_ref": missionRef, "status": status, "receiver_changed": true}); err != nil {
		return Mission{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_admin_audit_events(operator_id,action,entity_type,entity_id,reason,before,after) VALUES($1,'taxi.goods.receiver.changed','taxi_goods_mission',$2,$3,jsonb_build_object('receiver_ref',$4::text),jsonb_build_object('receiver_ref',$5::text))`, operator, missionID, command.OperationRef, previousReceiver, command.ReceiverRef); err != nil {
		return Mission{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_goods_binding_operations(mission_id,operation_ref,operation_type,request_fingerprint,previous_receiver_ref,new_receiver_ref,resulting_mission_version) VALUES($1,$2,'RECEIVER_CHANGE',$3,$4,$5,$6)`, missionID, command.OperationRef, requestFingerprint, previousReceiver, command.ReceiverRef, resultingVersion); err != nil {
		return Mission{}, false, err
	}
	var mission Mission
	if err = r.scanMission(tx.QueryRow(ctx, missionSelect+` WHERE m.id=$1`, missionID), &mission); err != nil {
		return Mission{}, false, err
	}
	return mission, true, tx.Commit(ctx)
}

func bindingOperation(ctx context.Context, tx pgx.Tx, operationRef, fingerprint string) (string, bool, error) {
	var missionID, stored string
	err := tx.QueryRow(ctx, `SELECT mission_id::text,request_fingerprint FROM taxi_goods_binding_operations WHERE operation_ref=$1 FOR UPDATE`, operationRef).Scan(&missionID, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if stored != fingerprint {
		return "", false, ErrConflict
	}
	return missionID, true, nil
}

func reassignableStatus(status string) bool {
	switch status {
	case StatusOffered, StatusAssigned, StatusEnRoutePickup, StatusAtPickup, StatusPickupHandoverRequired:
		return true
	default:
		return false
	}
}

func receiverChangeableStatus(status string) bool {
	switch status {
	case StatusMatching, StatusOffered, StatusAssigned, StatusEnRoutePickup, StatusAtPickup, StatusPickupHandoverRequired,
		StatusPickedUp, StatusInTransit, StatusAtDropoff, StatusDeliveryHandoverRequired:
		return true
	default:
		return false
	}
}

func (r *Repository) InsertChallenge(ctx context.Context, record HandoverRecord) error {
	_, err := r.db.Exec(ctx, `INSERT INTO taxi_goods_handover_challenges(challenge_ref,mission_id,stage,provider_id,receiver_ref,otp_hash,qr_hash,version,status,attempts,resend_count,expires_at,resend_available_at,stop_ref) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'ACTIVE',0,$9,$10,$11,$12)`, record.ChallengeRef, record.MissionID, record.Stage, record.ProviderID, record.ReceiverRef, []byte(record.OTPHash), []byte(record.QRHash), record.Version, record.ResendCount, record.ExpiresAt, record.ResendAvailableAt, record.StopRef)
	return err
}

func (r *Repository) InvalidateChallenge(ctx context.Context, challengeRef, reason string) error {
	tag, err := r.db.Exec(ctx, `UPDATE taxi_goods_handover_challenges SET status='INVALIDATED',invalidated_reason=$2,updated_at=now() WHERE challenge_ref=$1 AND status='ACTIVE'`, challengeRef, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (r *Repository) LatestChallenge(ctx context.Context, missionID, stage string, lock bool) (HandoverRecord, error) {
	query := `SELECT challenge_ref,mission_id::text,stage,provider_id::text,receiver_ref,otp_hash,qr_hash,version,status,attempts,resend_count,expires_at,resend_available_at,stop_ref FROM taxi_goods_handover_challenges WHERE mission_id=$1 AND stage=$2 ORDER BY version DESC LIMIT 1`
	if lock {
		query += ` FOR UPDATE`
	}
	var out HandoverRecord
	var otp, qr []byte
	err := r.db.QueryRow(ctx, query, missionID, stage).Scan(&out.ChallengeRef, &out.MissionID, &out.Stage, &out.ProviderID, &out.ReceiverRef, &otp, &qr, &out.Version, &out.Status, &out.Attempts, &out.ResendCount, &out.ExpiresAt, &out.ResendAvailableAt, &out.StopRef)
	out.OTPHash = string(otp)
	out.QRHash = string(qr)
	if errors.Is(err, pgx.ErrNoRows) {
		return HandoverRecord{}, ErrNotFound
	}
	return out, err
}

func (r *Repository) ReplaceChallenge(ctx context.Context, previous HandoverRecord, next HandoverRecord) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE taxi_goods_handover_challenges SET status='INVALIDATED',invalidated_reason='RESENT',updated_at=now() WHERE challenge_ref=$1 AND status='ACTIVE'`, previous.ChallengeRef)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO taxi_goods_handover_challenges(challenge_ref,mission_id,stage,provider_id,receiver_ref,otp_hash,qr_hash,version,status,attempts,resend_count,expires_at,resend_available_at,stop_ref) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'ACTIVE',0,$9,$10,$11,$12)`, next.ChallengeRef, next.MissionID, next.Stage, next.ProviderID, next.ReceiverRef, []byte(next.OTPHash), []byte(next.QRHash), next.Version, next.ResendCount, next.ExpiresAt, next.ResendAvailableAt, next.StopRef)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ConsumeChallenge(ctx context.Context, missionID, challengeRef, method, credentialHash string, now time.Time) (HandoverRecord, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return HandoverRecord{}, err
	}
	defer tx.Rollback(ctx)
	var out HandoverRecord
	var otp, qr []byte
	err = tx.QueryRow(ctx, `SELECT challenge_ref,mission_id::text,stage,provider_id::text,receiver_ref,otp_hash,qr_hash,version,status,attempts,resend_count,expires_at,resend_available_at FROM taxi_goods_handover_challenges WHERE challenge_ref=$1 AND mission_id=$2 FOR UPDATE`, challengeRef, missionID).Scan(&out.ChallengeRef, &out.MissionID, &out.Stage, &out.ProviderID, &out.ReceiverRef, &otp, &qr, &out.Version, &out.Status, &out.Attempts, &out.ResendCount, &out.ExpiresAt, &out.ResendAvailableAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return HandoverRecord{}, ErrNotFound
	}
	if err != nil {
		return HandoverRecord{}, err
	}
	out.OTPHash = string(otp)
	out.QRHash = string(qr)
	if out.Status != "ACTIVE" {
		return HandoverRecord{}, ErrCredentialInvalid
	}
	if !out.ExpiresAt.After(now) {
		_, _ = tx.Exec(ctx, `UPDATE taxi_goods_handover_challenges SET status='EXPIRED',updated_at=now() WHERE challenge_ref=$1`, challengeRef)
		_ = tx.Commit(ctx)
		return HandoverRecord{}, ErrCredentialExpired
	}
	expected := out.OTPHash
	if strings.EqualFold(method, "QR") {
		expected = out.QRHash
	}
	if expected != credentialHash {
		out.Attempts++
		status := "ACTIVE"
		if out.Attempts >= 5 {
			status = "LOCKED"
		}
		_, err = tx.Exec(ctx, `UPDATE taxi_goods_handover_challenges SET attempts=$2,status=$3,updated_at=now() WHERE challenge_ref=$1`, challengeRef, out.Attempts, status)
		if err != nil {
			return HandoverRecord{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return HandoverRecord{}, err
		}
		if status == "LOCKED" {
			return HandoverRecord{}, ErrCredentialLocked
		}
		return HandoverRecord{}, ErrCredentialInvalid
	}
	_, err = tx.Exec(ctx, `UPDATE taxi_goods_handover_challenges SET status='CONSUMED',consumed_at=$2,updated_at=$2 WHERE challenge_ref=$1`, challengeRef, now)
	if err != nil {
		return HandoverRecord{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return HandoverRecord{}, err
	}
	out.Status = "CONSUMED"
	return out, nil
}

// VerifyAndTransition serializes binding changes, challenge consumption and
// custody transition behind the mission row. Reassignment/receiver-change and
// verification therefore cannot both commit against an old binding.
func (r *Repository) VerifyAndTransition(ctx context.Context, caller, providerID, missionRef, stage string, command VerifyCredential, candidateHash string, now time.Time) (Mission, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Mission{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var missionID, source, currentProvider, receiver, status, custodyMode, tenantRef string
	var missionVersion int64
	err = tx.QueryRow(ctx, `SELECT id::text,source_service,COALESCE(provider_id::text,''),receiver_ref,status,custody_mode,parent_order_ref,version FROM taxi_goods_missions WHERE mission_ref=$1 FOR UPDATE`, missionRef).
		Scan(&missionID, &source, &currentProvider, &receiver, &status, &custodyMode, &tenantRef, &missionVersion)
	multi := custodyMode == custodyops.ModeMultiManifest
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrNotFound
	}
	if err != nil {
		return Mission{}, err
	}
	from, to, event := StatusPickupHandoverRequired, StatusPickedUp, "taxi.goods.pickup.verified.v1"
	if stage == StageDelivery {
		from, to, event = StatusDeliveryHandoverRequired, StatusDelivered, "taxi.goods.delivery.verified.v1"
	}
	if multi {
		// Stop-scoped pickups revisit the same mission status, so replay is
		// recognised by the consumed operation, not by the resulting status.
		var replay bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM taxi_goods_handover_challenges WHERE mission_id=$1 AND stage=$2 AND consumed_operation_ref=$3)`, missionID, stage, command.OperationRef).Scan(&replay); err != nil {
			return Mission{}, err
		}
		if replay {
			var mission Mission
			if err = r.scanMission(tx.QueryRow(ctx, missionSelect+` WHERE m.id=$1`, missionID), &mission); err != nil {
				return Mission{}, err
			}
			return mission, tx.Commit(ctx)
		}
	}
	if status == to && !multi {
		var replay bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM taxi_goods_handover_challenges WHERE mission_id=$1 AND stage=$2 AND consumed_operation_ref=$3)`, missionID, stage, command.OperationRef).Scan(&replay); err != nil {
			return Mission{}, err
		}
		if !replay {
			return Mission{}, ErrConflict
		}
		var mission Mission
		if err = r.scanMission(tx.QueryRow(ctx, missionSelect+` WHERE m.id=$1`, missionID), &mission); err != nil {
			return Mission{}, err
		}
		return mission, tx.Commit(ctx)
	}
	if status != from || currentProvider == "" || (stage == StagePickup && source != caller) || (stage == StageDelivery && currentProvider != providerID) {
		return Mission{}, ErrConflict
	}
	if stage == StageDelivery && strings.TrimSpace(command.ReceiverRef) != receiver {
		return Mission{}, ErrCredentialInvalid
	}
	var record HandoverRecord
	var otp, qr []byte
	err = tx.QueryRow(ctx, `SELECT challenge_ref,mission_id::text,stage,provider_id::text,receiver_ref,otp_hash,qr_hash,version,status,attempts,resend_count,expires_at,resend_available_at,stop_ref FROM taxi_goods_handover_challenges WHERE challenge_ref=$1 AND mission_id=$2 FOR UPDATE`, command.ChallengeRef, missionID).
		Scan(&record.ChallengeRef, &record.MissionID, &record.Stage, &record.ProviderID, &record.ReceiverRef, &otp, &qr, &record.Version, &record.Status, &record.Attempts, &record.ResendCount, &record.ExpiresAt, &record.ResendAvailableAt, &record.StopRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mission{}, ErrCredentialInvalid
	}
	if err != nil {
		return Mission{}, err
	}
	if record.Stage != stage || record.ProviderID != currentProvider || record.ReceiverRef != receiver || record.Status != "ACTIVE" {
		return Mission{}, ErrCredentialInvalid
	}
	if !record.ExpiresAt.After(now) {
		if _, err = tx.Exec(ctx, `UPDATE taxi_goods_handover_challenges SET status='EXPIRED',updated_at=$2 WHERE challenge_ref=$1`, record.ChallengeRef, now); err != nil {
			return Mission{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return Mission{}, err
		}
		return Mission{}, ErrCredentialExpired
	}
	expected := otp
	if strings.EqualFold(command.Method, "QR") {
		expected = qr
	}
	if !hmac.Equal(expected, []byte(candidateHash)) {
		record.Attempts++
		challengeStatus := "ACTIVE"
		if record.Attempts >= handover.MaxVerifyAttempts {
			challengeStatus = "LOCKED"
		}
		if _, err = tx.Exec(ctx, `UPDATE taxi_goods_handover_challenges SET attempts=$2,status=$3,updated_at=$4 WHERE challenge_ref=$1`, record.ChallengeRef, record.Attempts, challengeStatus, now); err != nil {
			return Mission{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return Mission{}, err
		}
		if challengeStatus == "LOCKED" {
			return Mission{}, ErrCredentialLocked
		}
		return Mission{}, ErrCredentialInvalid
	}
	if _, err = tx.Exec(ctx, `UPDATE taxi_goods_handover_challenges SET status='CONSUMED',consumed_at=$2,consumed_operation_ref=$3,updated_at=$2 WHERE challenge_ref=$1`, record.ChallengeRef, now, command.OperationRef); err != nil {
		return Mission{}, err
	}
	opState := "ON_TRIP"
	if multi {
		next, nextState, custodyErr := r.applyStopCustodyTx(ctx, tx, stopCustody{
			missionID: missionID, missionRef: missionRef, tenantRef: tenantRef, provider: currentProvider, stage: stage,
			stopRef: record.StopRef, challengeRef: record.ChallengeRef, operationRef: command.OperationRef,
			verifiedBy: caller, sourceVersion: missionVersion + 1, now: now,
		})
		if custodyErr != nil {
			return Mission{}, custodyErr
		}
		if next != "" {
			to, opState = next, nextState
		}
	}
	var version int64
	if err = tx.QueryRow(ctx, `UPDATE taxi_goods_missions SET status=$2,version=version+1,updated_at=$3,picked_up_at=CASE WHEN $2='PICKED_UP' THEN $3 ELSE picked_up_at END,delivered_at=CASE WHEN $2='DELIVERED' THEN $3 ELSE delivered_at END,manifest_frozen_at=CASE WHEN $2='PICKED_UP' THEN COALESCE(manifest_frozen_at,$3) ELSE manifest_frozen_at END WHERE id=$1 RETURNING version`, missionID, to, now).Scan(&version); err != nil {
		return Mission{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE provider_operational_state SET state=$5,reason=$3,booking_id=NULL,goods_mission_id=$2,effective_at=$4,version=version+1 WHERE provider_id=$1 AND goods_mission_id=$2`, currentProvider, missionID, "taxi_goods_"+strings.ToLower(to), now, opState); err != nil {
		return Mission{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO provider_operational_state_history(provider_id,from_state,to_state,reason,booking_id,goods_mission_id) VALUES($1,$2,$5,$3,NULL,$4)`, currentProvider, operationalStateForMissionStatus(from), "taxi_goods_"+strings.ToLower(to), missionID, opState); err != nil {
		return Mission{}, err
	}
	custody := "PICKED_UP"
	if stage == StageDelivery {
		custody = "DELIVERED"
	}
	parcelScope := `UPDATE taxi_goods_parcels SET custody_status=$2,updated_at=$3 WHERE mission_id=$1`
	parcelArgs := []any{missionID, custody, now}
	if multi && stage == StagePickup {
		// Only the parcels handed over at THIS stop enter custody.
		parcelScope += ` AND pickup_stop_ref=$4`
		parcelArgs = append(parcelArgs, record.StopRef)
	}
	if _, err = tx.Exec(ctx, parcelScope, parcelArgs...); err != nil {
		return Mission{}, err
	}
	if err = r.writeEvent(ctx, tx, missionID, event, version, map[string]any{"mission_ref": missionRef, "status": to}); err != nil {
		return Mission{}, err
	}
	var mission Mission
	if err = r.scanMission(tx.QueryRow(ctx, missionSelect+` WHERE m.id=$1`, missionID), &mission); err != nil {
		return Mission{}, err
	}
	return mission, tx.Commit(ctx)
}

func (r *Repository) AddEvidence(ctx context.Context, mission Mission, providerID string, command EvidenceCommand) (string, error) {
	if _, err := r.ReadMissionForProvider(ctx, providerID, mission.MissionRef); err != nil {
		return "", err
	}
	var count int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM taxi_goods_parcels WHERE mission_id=$1 AND (cardinality($2::text[])=0 OR parcel_ref=ANY($2))`, mission.ID, canonical(command.ParcelRefs)).Scan(&count); err != nil {
		return "", err
	}
	if len(command.ParcelRefs) > 0 && count != len(canonical(command.ParcelRefs)) {
		return "", ErrMediaScopeMismatch
	}
	ref := "taxi_goods_evidence_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err := r.db.Exec(ctx, `INSERT INTO taxi_goods_evidence_refs(evidence_ref,mission_id,stage,media_ref,parcel_refs,captured_by_provider_id,captured_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, ref, mission.ID, command.Stage, command.MediaRef, canonical(command.ParcelRefs), providerID, command.CapturedAt)
	if err != nil {
		return "", err
	}
	column := "pickup_evidence_ref"
	if command.Stage == StageDelivery {
		column = "delivery_evidence_ref"
	}
	_, err = r.db.Exec(ctx, `UPDATE taxi_goods_missions SET `+column+`=$2,updated_at=now() WHERE id=$1`, mission.ID, ref)
	return ref, err
}

func (r *Repository) CompleteSettlement(ctx context.Context, missionID string, confirmed bool) error {
	status := "FAILED"
	if confirmed {
		status = "CONFIRMED"
	}
	_, err := r.db.Exec(ctx, `UPDATE taxi_goods_missions SET settlement_status=$2,settlement_attempts=settlement_attempts+1,settlement_next_attempt_at=CASE WHEN $2='CONFIRMED' THEN NULL ELSE now()+interval '1 minute' END,updated_at=now() WHERE id=$1`, missionID, status)
	return err
}

func (r *Repository) PendingSettlements(ctx context.Context, limit int) ([]Mission, error) {
	rows, err := r.db.Query(ctx, missionSelect+` WHERE settlement_status IN ('PENDING','FAILED') AND settlement_next_attempt_at<=now() ORDER BY settlement_next_attempt_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mission
	for rows.Next() {
		var m Mission
		if err = r.scanMission(rows, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) writeEvent(ctx context.Context, tx pgx.Tx, id, event string, version int64, payload any) error {
	var missionRef, parentOrderRef string
	var vendorOrderRefs []string
	if err := tx.QueryRow(ctx, `SELECT mission_ref,parent_order_ref,vendor_order_refs FROM taxi_goods_missions WHERE id=$1`, id).
		Scan(&missionRef, &parentOrderRef, &vendorOrderRefs); err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	fields := map[string]any{}
	if err = json.Unmarshal(body, &fields); err != nil {
		return err
	}
	fields["mission_ref"] = missionRef
	fields["parent_order_id"] = parentOrderRef
	fields["vendor_order_ids"] = vendorOrderRefs
	fields["source_version"] = version
	if status, ok := fields["status"].(string); ok {
		fields["vendor_event_type"] = vendorLifecycleEvent(status)
	}
	body, err = json.Marshal(fields)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_outbox(aggregate_type,aggregate_id,event_type,payload,target) VALUES('taxi_goods_mission',$1,$2,$3,'vendor-service')`, id, event, body); err != nil {
		return err
	}
	return r.writeGoodsMovement(ctx, tx, id, event, fields)
}

func (r *Repository) writeGoodsMovement(ctx context.Context, tx pgx.Tx, id, event string, fields map[string]any) error {
	if r.movement == nil || !r.movement.Enabled {
		return nil
	}
	status, _ := fields["status"].(string)
	if status == "" {
		return nil
	}
	refs := movement.GoodsMissionRefs{MissionID: id, Status: status}
	var product *string
	var pickup, dropoff []byte
	if err := tx.QueryRow(ctx, `SELECT mission_ref,customer_ref,parent_order_ref,COALESCE(plan_ref,''),fulfilment_group_ref,service_product_code,pickup,dropoff FROM taxi_goods_missions WHERE id=$1`, id).
		Scan(&refs.MissionRef, &refs.CustomerRef, &refs.ParentOrderRef, &refs.FulfilmentPlanRef, &refs.FulfilmentGroupRef, &product, &pickup, &dropoff); err != nil {
		return err
	}
	if product != nil {
		refs.ProductCode = *product
	}
	var p, d commercemission.Point
	if json.Unmarshal(pickup, &p) == nil {
		refs.Pickup = &events.PlaceSummary{Latitude: p.Latitude, Longitude: p.Longitude}
	}
	if json.Unmarshal(dropoff, &d) == nil {
		refs.Dropoff = &events.PlaceSummary{Latitude: d.Latitude, Longitude: d.Longitude}
	}
	return r.movement.WriteGoodsMission(ctx, tx, refs, strings.TrimSuffix(event, ".v1"), refs.MissionRef)
}

func vendorLifecycleEvent(status string) string {
	switch status {
	case StatusPickedUp:
		return "PICKED_UP"
	case StatusInTransit, StatusAtDropoff, StatusDeliveryHandoverRequired:
		return "DELIVERING"
	case StatusDelivered, StatusCompleted:
		return "DELIVERED"
	case StatusFailed, StatusUnmatched:
		return "DELIVERY_FAILED"
	default:
		return status
	}
}

func canonical(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.ToUpper(strings.TrimSpace(v))
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
func handlingCapabilities(h commercemission.Handling) []string {
	var out []string
	if h.Fragile {
		out = append(out, commercequote.CapabilityFragileCarry)
	}
	if h.Upright {
		out = append(out, commercequote.CapabilityUprightCarry)
	}
	if h.SpillLiquid {
		out = append(out, commercequote.CapabilityLeakContainment)
	}
	if h.Hot {
		out = append(out, commercequote.CapabilityHotHold)
	}
	if h.Chilled {
		out = append(out, commercequote.CapabilityColdChain)
	}
	if h.Frozen {
		out = append(out, commercequote.CapabilityFrozenChain)
	}
	return canonical(out)
}

func missionMatchesQuote(mission commercemission.Request, quoted commercequote.Request) bool {
	if quoted.VehicleClass != commercequote.VehicleCar || mission.VehicleClass != commercemission.VehicleCar ||
		!strings.EqualFold(strings.TrimSpace(mission.Currency), strings.TrimSpace(quoted.Currency)) {
		return false
	}
	pickup := mission.Pickup
	if len(mission.PickupStops) == 1 {
		pickup = mission.PickupStops[0].Location
	}
	quotedPickup := quoted.Pickup
	if len(quoted.PickupStops) == 1 {
		quotedPickup = quoted.PickupStops[0].Location
	}
	if pickup.Latitude != quotedPickup.Latitude || pickup.Longitude != quotedPickup.Longitude ||
		mission.Dropoff.Latitude != quoted.Dropoff.Latitude || mission.Dropoff.Longitude != quoted.Dropoff.Longitude {
		return false
	}
	weight, packages, volume, longest := mission.Load.WeightGrams, mission.Load.PackageCount, int64(0), int64(0)
	capabilities := handlingCapabilities(mission.Handling)
	if len(mission.Parcels) > 0 {
		weight, packages = 0, 0
		for _, parcel := range mission.Parcels {
			weight += parcel.WeightGrams
			packages += max64(parcel.PackageCount, 1)
			volume += parcel.LengthMm * parcel.WidthMm * parcel.HeightMm / 1000
			longest = max64(longest, max64(parcel.LengthMm, max64(parcel.WidthMm, parcel.HeightMm)))
			capabilities = append(capabilities, handlingCapabilities(parcel.Handling)...)
		}
	}
	return weight == quoted.Load.TotalWeightGrams && packages == quoted.Load.PackageCount &&
		(quoted.Load.TotalVolumeMl == 0 || volume == quoted.Load.TotalVolumeMl) &&
		(quoted.Load.LongestDimensionMm == 0 || longest == int64(quoted.Load.LongestDimensionMm)) &&
		strings.Join(canonical(capabilities), "|") == strings.Join(canonical(quoted.Load.Capabilities), "|")
}

func operationalStateForMissionStatus(status string) string {
	switch status {
	case StatusOffered, StatusAssigned:
		return "OFFERED"
	case StatusEnRoutePickup:
		return "EN_ROUTE_PICKUP"
	case StatusAtPickup, StatusPickupHandoverRequired:
		return "AT_PICKUP"
	case StatusPickedUp, StatusInTransit, StatusDeliveryHandoverRequired, StatusDelivered:
		return "ON_TRIP"
	case StatusAtDropoff:
		return "ON_TRIP"
	default:
		return "AVAILABLE"
	}
}
func planValue(r commercemission.Request, which string) string {
	if r.Plan == nil {
		return ""
	}
	switch which {
	case "ref":
		return r.Plan.PlanRef
	case "fingerprint":
		return r.Plan.PlanFingerprint
	default:
		return r.Plan.MissionSlotRef
	}
}

// Plan binding modes. A legacy quote-bound mission has no plan identity and is
// stored with an empty plan_ref, never a generated placeholder that could
// collide with a real plan slot.
const (
	PlanBindingLegacyQuote = "LEGACY_QUOTE"
	PlanBindingPlanBound   = "PLAN_BOUND"
)

type PlanBinding struct {
	Mode            string
	PlanRef         string
	PlanVersion     int64
	PlanFingerprint string
	MissionSlotRef  string
}

// ResolvePlanBinding classifies a single-pickup mission request. A request
// without a plan is LEGACY_QUOTE. A plan binding must be complete (ref and a
// version >= 1); a partial binding is rejected rather than stored half-bound.
func ResolvePlanBinding(r commercemission.Request) (PlanBinding, error) {
	if r.Plan == nil || (strings.TrimSpace(r.Plan.PlanRef) == "" && r.Plan.PlanVersion == 0 && strings.TrimSpace(r.Plan.PlanFingerprint) == "" && strings.TrimSpace(r.Plan.MissionSlotRef) == "") {
		return PlanBinding{Mode: PlanBindingLegacyQuote}, nil
	}
	ref := strings.TrimSpace(r.Plan.PlanRef)
	if ref == "" || r.Plan.PlanVersion < 1 {
		return PlanBinding{}, ErrInvalid
	}
	return PlanBinding{Mode: PlanBindingPlanBound, PlanRef: ref, PlanVersion: r.Plan.PlanVersion,
		PlanFingerprint: strings.TrimSpace(r.Plan.PlanFingerprint), MissionSlotRef: strings.TrimSpace(r.Plan.MissionSlotRef)}, nil
}

func planVersion(r commercemission.Request) int64 {
	if r.Plan == nil {
		return 0
	}
	return r.Plan.PlanVersion
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

var _ = fmt.Sprintf
