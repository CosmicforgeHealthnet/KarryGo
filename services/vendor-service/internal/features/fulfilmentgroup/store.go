package fulfilmentgroup

// Persistence for the fulfilment aggregate.
//
// Two things this layer is deliberately NOT allowed to be:
//
//   - the only thing enforcing an invariant. Every rule that matters under
//     concurrency — one group per order, one unit per vendor order, one active
//     plan version, monotonic plan versions, one live mission per slot — is a
//     database constraint as well. This code is the readable statement of the
//     rule; the constraint is the guarantee.
//
//   - a place where tenancy is taken on trust. Every vendor-facing write
//     resolves the owning business from the row it is about and compares it to
//     the authenticated tenant. A business id that arrived in a request body
//     is never used to decide who owns something.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	vendorevents "cosmicforge/logistics/services/vendor-service/internal/events"
)

// Store persists fulfilment groups, units, parcels, plans and missions.
type Store struct {
	db     *pgxpool.Pool
	outbox *vendorevents.DomainPublisher
}

// NewStore builds the store.
func NewStore(db *pgxpool.Pool, outbox *vendorevents.DomainPublisher) *Store {
	return &Store{db: db, outbox: outbox}
}

// Ready reports whether persistence is connected.
func (s *Store) Ready() bool { return s != nil && s.db != nil }

func (s *Store) begin(ctx context.Context) (pgx.Tx, error) {
	if !s.Ready() {
		return nil, ErrDependencyMissing
	}
	return s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
}

// -----------------------------------------------------------------------------
// Group and units
// -----------------------------------------------------------------------------

const groupColumns = `id, parent_order_id, customer_id,
	COALESCE(current_plan_id, ''), current_plan_version,
	version, created_at, updated_at`

func scanGroup(row pgx.Row) (Group, error) {
	var group Group
	err := row.Scan(&group.ID, &group.ParentOrderID, &group.CustomerID,
		&group.CurrentPlanID, &group.CurrentPlanVersion,
		&group.Version, &group.CreatedAt, &group.UpdatedAt)
	return group, err
}

// EnsureGroup opens the fulfilment group for one commerce order, or returns
// the one that already exists.
//
// Two concurrent bootstraps cannot both create a group: the second loses the
// unique(parent_order_id) race and reads back the winner's row rather than
// failing. That is what makes "one enduring group per order" true under load
// and not merely true in the happy path.
func (s *Store) EnsureGroup(
	ctx context.Context, parentOrderID, customerID string, now time.Time,
) (Group, error) {
	if !s.Ready() {
		return Group{}, ErrDependencyMissing
	}
	parentOrderID = strings.TrimSpace(parentOrderID)
	customerID = strings.TrimSpace(customerID)
	if parentOrderID == "" || customerID == "" {
		return Group{}, ErrInvalid
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return Group{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	group, err := scanGroup(tx.QueryRow(ctx, `
		SELECT `+groupColumns+`
		  FROM commerce_fulfilment_groups
		 WHERE parent_order_id = $1 AND customer_id = $2`,
		parentOrderID, customerID))
	if err == nil {
		return group, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Group{}, err
	}

	id := "cfg_" + uuid.NewString()
	group, err = scanGroup(tx.QueryRow(ctx, `
		INSERT INTO commerce_fulfilment_groups (
			id, parent_order_id, customer_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, 1, $4, $4)
		ON CONFLICT (parent_order_id) DO NOTHING
		RETURNING `+groupColumns, id, parentOrderID, customerID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		// Lost the race. The winner's group is the group; there is no second
		// one to reconcile.
		group, err = scanGroup(tx.QueryRow(ctx, `
			SELECT `+groupColumns+`
			  FROM commerce_fulfilment_groups
			 WHERE parent_order_id = $1 AND customer_id = $2`,
			parentOrderID, customerID))
		if err != nil {
			return Group{}, err
		}
		return group, tx.Commit(ctx)
	}
	if err != nil {
		return Group{}, translateConstraint(err)
	}

	if err = s.publish(ctx, tx, EventGroupOpened, group.ID, group.Version, "",
		map[string]string{"parent_order_id": parentOrderID}); err != nil {
		return Group{}, err
	}

	return group, tx.Commit(ctx)
}

const unitColumns = `id, group_id, parent_order_id, vendor_order_id,
	business_id, branch_id, pickup_address, pickup_latitude, pickup_longitude,
	service_class, vehicle_class, required_weight_grams, required_package_count,
	handling_requirements, readiness, version, created_at, updated_at`

func scanUnit(row pgx.Row) (Unit, error) {
	var unit Unit
	var handling []byte
	err := row.Scan(&unit.ID, &unit.GroupID, &unit.ParentOrderID, &unit.VendorOrderID,
		&unit.BusinessID, &unit.BranchID, &unit.PickupAddress,
		&unit.PickupLatitude, &unit.PickupLongitude,
		&unit.ServiceClass, &unit.VehicleClass,
		&unit.RequiredWeightGrams, &unit.RequiredPackageCount,
		&handling, &unit.Readiness, &unit.Version, &unit.CreatedAt, &unit.UpdatedAt)
	if err != nil {
		return Unit{}, err
	}
	if len(handling) > 0 {
		_ = json.Unmarshal(handling, &unit.HandlingRequirements)
	}
	return unit, nil
}

// EnsureUnit opens the pickup obligation for one vendor order.
//
// The vendor and branch are read from the vendor order inside the same
// transaction, never from the caller. That is what stops a unit being declared
// into a tenancy it does not belong to, and it is why UnitInput has no
// business_id field to supply.
func (s *Store) EnsureUnit(
	ctx context.Context, group Group, in UnitInput, now time.Time,
) (Unit, error) {
	if !s.Ready() {
		return Unit{}, ErrDependencyMissing
	}
	if err := in.Validate(); err != nil {
		return Unit{}, err
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return Unit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	unit, err := scanUnit(tx.QueryRow(ctx, `
		SELECT `+unitColumns+`
		  FROM commerce_fulfilment_units
		 WHERE vendor_order_id = $1`, in.VendorOrderID))
	if err == nil {
		if unit.GroupID != group.ID {
			// The obligation already belongs to another group, which can only
			// mean the vendor order belongs to another order.
			return Unit{}, ErrConflict
		}
		return unit, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, err
	}

	var businessID, branchID string
	err = tx.QueryRow(ctx, `
		SELECT business_id, branch_id
		  FROM commerce_vendor_orders
		 WHERE id = $1 AND parent_order_id = $2`,
		in.VendorOrderID, group.ParentOrderID).Scan(&businessID, &branchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrNotFound
	}
	if err != nil {
		return Unit{}, err
	}

	handling, err := json.Marshal(nonNilStrings(in.HandlingRequirements))
	if err != nil {
		return Unit{}, ErrInvalid
	}

	id := "cfu_" + uuid.NewString()
	unit, err = scanUnit(tx.QueryRow(ctx, `
		INSERT INTO commerce_fulfilment_units (
			id, group_id, parent_order_id, vendor_order_id,
			business_id, branch_id, pickup_address, pickup_latitude, pickup_longitude,
			service_class, vehicle_class, required_weight_grams, required_package_count,
			handling_requirements, readiness, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'NOT_READY',1,$15,$15)
		ON CONFLICT (vendor_order_id) DO NOTHING
		RETURNING `+unitColumns,
		id, group.ID, group.ParentOrderID, in.VendorOrderID,
		businessID, branchID, in.PickupAddress, in.PickupLatitude, in.PickupLongitude,
		in.ServiceClass, in.VehicleClass, in.RequiredWeightGrams, in.RequiredPackageCount,
		handling, now))
	if errors.Is(err, pgx.ErrNoRows) {
		unit, err = scanUnit(tx.QueryRow(ctx, `
			SELECT `+unitColumns+`
			  FROM commerce_fulfilment_units
			 WHERE vendor_order_id = $1`, in.VendorOrderID))
		if err != nil {
			return Unit{}, err
		}
		return unit, tx.Commit(ctx)
	}
	if err != nil {
		return Unit{}, translateConstraint(err)
	}

	return unit, tx.Commit(ctx)
}

// SetUnitReadiness advances a pickup obligation's readiness.
//
// Scoped to the owning business, so a branch manager cannot mark another
// vendor's pickup ready. The transition itself is checked before the write
// rather than after, so an illegal move never reaches the row.
func (s *Store) SetUnitReadiness(
	ctx context.Context, businessID, unitID string,
	expectedVersion int64, to Readiness, now time.Time,
) (Unit, error) {
	if !s.Ready() {
		return Unit{}, ErrDependencyMissing
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return Unit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	unit, err := scanUnit(tx.QueryRow(ctx, `
		SELECT `+unitColumns+`
		  FROM commerce_fulfilment_units
		 WHERE id = $1 FOR UPDATE`, unitID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrNotFound
	}
	if err != nil {
		return Unit{}, err
	}
	if unit.BusinessID != businessID {
		// Same answer as a missing row: a probe must not learn that another
		// vendor's unit exists.
		return Unit{}, ErrNotFound
	}
	if unit.Version != expectedVersion {
		return Unit{}, ErrVersionConflict
	}
	if err = AdvanceReadiness(unit.Readiness, to); err != nil {
		return Unit{}, err
	}

	unit, err = scanUnit(tx.QueryRow(ctx, `
		UPDATE commerce_fulfilment_units
		   SET readiness = $1, version = version + 1, updated_at = $2
		 WHERE id = $3 AND version = $4
		RETURNING `+unitColumns, to, now, unitID, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrVersionConflict
	}
	if err != nil {
		return Unit{}, err
	}

	return unit, tx.Commit(ctx)
}

// GetUnit reads one pickup obligation.
//
// Unscoped on purpose: the caller compares the unit's own business against the
// authenticated tenant, so the tenancy decision is made in one place rather
// than half here and half there.
func (s *Store) GetUnit(ctx context.Context, unitID string) (Unit, error) {
	if !s.Ready() {
		return Unit{}, ErrDependencyMissing
	}
	unit, err := scanUnit(s.db.QueryRow(ctx, `
		SELECT `+unitColumns+`
		  FROM commerce_fulfilment_units WHERE id = $1`, unitID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrNotFound
	}
	return unit, err
}

// ListUnits reads a group's pickup obligations.
func (s *Store) ListUnits(ctx context.Context, groupID string) ([]Unit, error) {
	if !s.Ready() {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+unitColumns+`
		  FROM commerce_fulfilment_units
		 WHERE group_id = $1
		 ORDER BY business_id, branch_id, id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var units []Unit
	for rows.Next() {
		unit, scanErr := scanUnit(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		units = append(units, unit)
	}
	return units, rows.Err()
}

// -----------------------------------------------------------------------------
// Parcels
// -----------------------------------------------------------------------------

const parcelColumns = `id, group_id, unit_id, parent_order_id, vendor_order_id,
	business_id, branch_id, parcel_ref, sequence, status,
	custodian_type, custodian_ref, manifest, manifest_version,
	weight_grams, length_mm, width_mm, height_mm, fragile, temperature_control,
	version, created_at, updated_at`

func scanParcel(row pgx.Row) (Parcel, error) {
	var parcel Parcel
	var manifest []byte
	err := row.Scan(&parcel.ID, &parcel.GroupID, &parcel.UnitID,
		&parcel.ParentOrderID, &parcel.VendorOrderID,
		&parcel.BusinessID, &parcel.BranchID, &parcel.ParcelRef, &parcel.Sequence,
		&parcel.Status, &parcel.CustodianType, &parcel.CustodianRef,
		&manifest, &parcel.ManifestVersion,
		&parcel.WeightGrams, &parcel.LengthMm, &parcel.WidthMm, &parcel.HeightMm,
		&parcel.Fragile, &parcel.TemperatureControl,
		&parcel.Version, &parcel.CreatedAt, &parcel.UpdatedAt)
	if err != nil {
		return Parcel{}, err
	}
	parcel.Manifest = DecodeManifest(manifest)
	return parcel, nil
}

// CreateParcel packs one parcel into a unit.
//
// Idempotent on the caller's operation key. A packing worker that retries
// after a lost response gets the parcel it already created back, rather than a
// second physical box the picker will never find.
func (s *Store) CreateParcel(
	ctx context.Context, businessID, unitID string, in ParcelInput, now time.Time,
) (Parcel, error) {
	if !s.Ready() {
		return Parcel{}, ErrDependencyMissing
	}
	in = in.Normalized()
	if err := in.Validate(); err != nil {
		return Parcel{}, err
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return Parcel{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the unit so two concurrent packing calls cannot both compute the
	// same next sequence number.
	unit, err := scanUnit(tx.QueryRow(ctx, `
		SELECT `+unitColumns+`
		  FROM commerce_fulfilment_units
		 WHERE id = $1 FOR UPDATE`, unitID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Parcel{}, ErrNotFound
	}
	if err != nil {
		return Parcel{}, err
	}
	if unit.BusinessID != businessID {
		return Parcel{}, ErrNotFound
	}

	existing, err := scanParcel(tx.QueryRow(ctx, `
		SELECT `+parcelColumns+`
		  FROM commerce_fulfilment_parcels
		 WHERE unit_id = $1 AND idempotency_key = $2`, unitID, in.IdempotencyKey))
	if err == nil {
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Parcel{}, err
	}

	var nextSequence int
	if err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(sequence), 0) + 1
		  FROM commerce_fulfilment_parcels WHERE unit_id = $1`,
		unitID).Scan(&nextSequence); err != nil {
		return Parcel{}, err
	}

	manifest, err := in.Manifest.Encode()
	if err != nil {
		return Parcel{}, err
	}

	id := "cfp_" + uuid.NewString()
	// The reference a human reads. Derived from the unit and the ordinal so it
	// is stable and meaningful; the primary key remains the identity.
	parcelRef := fmt.Sprintf("PKG-%s-%d", strings.ToUpper(shortSuffix(unit.ID)), nextSequence)

	parcel, err := scanParcel(tx.QueryRow(ctx, `
		INSERT INTO commerce_fulfilment_parcels (
			id, group_id, unit_id, parent_order_id, vendor_order_id,
			business_id, branch_id, parcel_ref, sequence, status,
			custodian_type, custodian_ref, manifest, manifest_version,
			weight_grams, length_mm, width_mm, height_mm, fragile,
			temperature_control, idempotency_key, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'CREATED','VENDOR','',$10,1,
			$11,$12,$13,$14,$15,$16,$17,1,$18,$18)
		RETURNING `+parcelColumns,
		id, unit.GroupID, unit.ID, unit.ParentOrderID, unit.VendorOrderID,
		unit.BusinessID, unit.BranchID, parcelRef, nextSequence,
		manifest, in.WeightGrams, in.LengthMm, in.WidthMm, in.HeightMm,
		in.Fragile, in.TemperatureControl, in.IdempotencyKey, now))
	if err != nil {
		return Parcel{}, translateConstraint(err)
	}

	// A parcel packed after the plan is already in force still has to ride a
	// mission. Attaching it to the active plan's slot for its own unit means a
	// late-packed box is never orphaned, and the attachment is per plan
	// version, so a later replan re-assigns it without disturbing this record.
	if err = attachParcelToActivePlan(ctx, tx, parcel, now); err != nil {
		return Parcel{}, err
	}

	if err = recordCustodyTx(ctx, tx, CustodyEvent{
		ParcelID:          parcel.ID,
		GroupID:           parcel.GroupID,
		EventType:         CustodyCreated,
		FromCustodianType: CustodianNone,
		ToCustodianType:   CustodianVendor,
		ToCustodianRef:    parcel.BusinessID,
		Source:            SourceVendor,
		SourceEventID:     "parcel-created-" + parcel.ID,
		OccurredAt:        now,
	}, now); err != nil {
		return Parcel{}, err
	}

	if err = s.publish(ctx, tx, EventParcelCreated, parcel.ID, parcel.Version,
		parcel.BusinessID, map[string]string{
			"parcel_ref":      parcel.ParcelRef,
			"vendor_order_id": parcel.VendorOrderID,
		}); err != nil {
		return Parcel{}, err
	}

	return parcel, tx.Commit(ctx)
}

// attachParcelToActivePlan places a parcel on the mission slot that already
// collects its unit under the plan currently in force.
//
// A group with no active plan yet is normal, not an error: parcels can be
// packed before planning finishes, and the plan will assign them when it is
// written. Likewise a plan whose slots do not cover this unit is left alone
// rather than guessed at.
func attachParcelToActivePlan(
	ctx context.Context, tx pgx.Tx, parcel Parcel, now time.Time,
) error {
	var planID, planMissionID string
	err := tx.QueryRow(ctx, `
		SELECT pm.plan_id, pm.id
		  FROM commerce_fulfilment_plan_versions pv
		  JOIN commerce_fulfilment_plan_missions pm ON pm.plan_id = pv.id
		  JOIN commerce_fulfilment_plan_stops st ON st.plan_mission_id = pm.id
		 WHERE pv.group_id = $1 AND pv.status = 'ACTIVE'
		   AND st.stop_type = 'PICKUP' AND st.unit_id = $2
		 ORDER BY pm.sequence
		 LIMIT 1`, parcel.GroupID, parcel.UnitID).Scan(&planID, &planMissionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO commerce_fulfilment_plan_mission_parcels (
			plan_mission_id, parcel_id, plan_id, group_id, created_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT DO NOTHING`,
		planMissionID, parcel.ID, planID, parcel.GroupID, now)
	return err
}

// UpdateParcelManifest amends a parcel before it leaves the vendor's hands.
//
// Refused once custody has moved. What a delivered parcel contained is
// evidence somebody may later have to rely on, and a manifest that can be
// rewritten after handover is not evidence.
func (s *Store) UpdateParcelManifest(
	ctx context.Context, businessID, parcelID string,
	expectedVersion int64, manifest Manifest, now time.Time,
) (Parcel, error) {
	if !s.Ready() {
		return Parcel{}, ErrDependencyMissing
	}
	if err := manifest.Validate(); err != nil {
		return Parcel{}, err
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return Parcel{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	parcel, err := scanParcel(tx.QueryRow(ctx, `
		SELECT `+parcelColumns+`
		  FROM commerce_fulfilment_parcels WHERE id = $1 FOR UPDATE`, parcelID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Parcel{}, ErrNotFound
	}
	if err != nil {
		return Parcel{}, err
	}
	if parcel.BusinessID != businessID {
		return Parcel{}, ErrNotFound
	}
	if parcel.Version != expectedVersion {
		return Parcel{}, ErrVersionConflict
	}
	if parcel.Sealed() {
		return Parcel{}, ErrCustodySealed
	}

	// The caller's SealedAt is ignored: sealing follows custody evidence, and
	// a request body is not custody evidence.
	manifest.SealedAt = nil
	encoded, err := manifest.Encode()
	if err != nil {
		return Parcel{}, err
	}

	parcel, err = scanParcel(tx.QueryRow(ctx, `
		UPDATE commerce_fulfilment_parcels
		   SET manifest = $1, manifest_version = manifest_version + 1,
		       status = CASE WHEN status = 'CREATED' THEN 'PACKED' ELSE status END,
		       version = version + 1, updated_at = $2
		 WHERE id = $3 AND version = $4
		RETURNING `+parcelColumns, encoded, now, parcelID, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Parcel{}, ErrVersionConflict
	}
	if err != nil {
		return Parcel{}, err
	}

	return parcel, tx.Commit(ctx)
}

// ListParcels reads a group's parcels.
func (s *Store) ListParcels(ctx context.Context, groupID string) ([]Parcel, error) {
	return s.queryParcels(ctx, `
		SELECT `+parcelColumns+`
		  FROM commerce_fulfilment_parcels
		 WHERE group_id = $1 ORDER BY unit_id, sequence`, groupID)
}

// ListParcelsByUnit reads one pickup obligation's parcels.
func (s *Store) ListParcelsByUnit(ctx context.Context, unitID string) ([]Parcel, error) {
	return s.queryParcels(ctx, `
		SELECT `+parcelColumns+`
		  FROM commerce_fulfilment_parcels
		 WHERE unit_id = $1 ORDER BY sequence`, unitID)
}

func (s *Store) queryParcels(ctx context.Context, query string, args ...any) ([]Parcel, error) {
	if !s.Ready() {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var parcels []Parcel
	for rows.Next() {
		parcel, scanErr := scanParcel(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		parcels = append(parcels, parcel)
	}
	return parcels, rows.Err()
}

// GetParcel reads one parcel, scoped to the owning business.
func (s *Store) GetParcel(ctx context.Context, businessID, parcelID string) (Parcel, error) {
	if !s.Ready() {
		return Parcel{}, ErrDependencyMissing
	}
	parcel, err := scanParcel(s.db.QueryRow(ctx, `
		SELECT `+parcelColumns+`
		  FROM commerce_fulfilment_parcels WHERE id = $1`, parcelID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Parcel{}, ErrNotFound
	}
	if err != nil {
		return Parcel{}, err
	}
	if businessID != "" && parcel.BusinessID != businessID {
		return Parcel{}, ErrNotFound
	}
	return parcel, nil
}

// -----------------------------------------------------------------------------
// Plan versions
// -----------------------------------------------------------------------------

const planColumns = `id, group_id, version, status, reason_code, reason_detail,
	planner_version, policy_version, delivery_mode,
	planned_cost_minor, currency, cost_snapshot, routing_evidence,
	fingerprint, COALESCE(supersedes_version, 0), idempotency_key,
	created_at, activated_at, superseded_at`

func scanPlan(row pgx.Row) (Plan, error) {
	var plan Plan
	var cost, routing []byte
	err := row.Scan(&plan.ID, &plan.GroupID, &plan.Version, &plan.Status,
		&plan.Reason, &plan.ReasonDetail, &plan.PlannerVersion, &plan.PolicyVersion,
		&plan.DeliveryMode, &plan.PlannedCostMinor, &plan.Currency,
		&cost, &routing, &plan.Fingerprint, &plan.SupersedesVersion,
		&plan.IdempotencyKey, &plan.CreatedAt, &plan.ActivatedAt, &plan.SupersededAt)
	if err != nil {
		return Plan{}, err
	}
	plan.CostSnapshot = json.RawMessage(cost)
	plan.RoutingEvidence = json.RawMessage(routing)
	return plan, nil
}

// ActivatePlan appends the next plan version and puts it in force.
//
// This is the concurrency-critical operation of the whole aggregate, and the
// ordering matters:
//
//  1. The group row is locked, so two replans serialise rather than interleave.
//  2. The caller's idempotency key is read back first, so a retry returns its
//     own version instead of minting another one.
//  3. The caller's expected version is compared to the version actually in
//     force. A caller that planned against a superseded version is told its
//     plan is STALE — it is not silently allowed to overwrite the newer one.
//  4. The new version, its mission slots, its stops, its parcel assignments,
//     the supersession of the previous version and the group's current-plan
//     pointer all commit together. There is no instant at which the pointer
//     names a version whose missions were never durably written.
//
// Two concurrent replans against v3 therefore produce exactly one v4 and one
// stale-plan answer, never two v4 rows: unique(group_id, version) is the
// backstop if the lock is ever bypassed.
func (s *Store) ActivatePlan(
	ctx context.Context, groupID string, in PlanInput, now time.Time,
) (Plan, error) {
	if !s.Ready() {
		return Plan{}, ErrDependencyMissing
	}
	if err := in.Validate(); err != nil {
		return Plan{}, err
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return Plan{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	group, err := scanGroup(tx.QueryRow(ctx, `
		SELECT `+groupColumns+`
		  FROM commerce_fulfilment_groups WHERE id = $1 FOR UPDATE`, groupID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}

	// Read-back before write. A retried replan must not become a new version.
	existing, err := scanPlan(tx.QueryRow(ctx, `
		SELECT `+planColumns+`
		  FROM commerce_fulfilment_plan_versions
		 WHERE group_id = $1 AND idempotency_key = $2`, groupID, in.IdempotencyKey))
	if err == nil {
		existing.Missions, err = loadPlanMissions(ctx, tx, existing.ID)
		if err != nil {
			return Plan{}, err
		}
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, err
	}

	if in.ExpectedVersion != group.CurrentPlanVersion {
		return Plan{}, fmt.Errorf("%w: expected version %d, current version %d", ErrStalePlan, in.ExpectedVersion, group.CurrentPlanVersion)
	}

	if err = assertMissionUnits(ctx, tx, groupID, in.Missions); err != nil {
		return Plan{}, err
	}

	nextVersion := group.CurrentPlanVersion + 1
	var supersedes any
	if group.CurrentPlanVersion > 0 {
		supersedes = group.CurrentPlanVersion
	}

	planID := "cfpv_" + uuid.NewString()
	fingerprint := Fingerprint(in.DeliveryMode, in.Missions)

	// Retire the current row before inserting its ACTIVE replacement. The
	// partial unique index permits only one ACTIVE plan per group, and both
	// writes are in this transaction: if any later write fails, rollback makes
	// the prior plan active again, so no intermediate state is observable.
	if group.CurrentPlanVersion > 0 {
		tag, updateErr := tx.Exec(ctx, `
			UPDATE commerce_fulfilment_plan_versions
			   SET status = 'SUPERSEDED', superseded_at = $1
			 WHERE group_id = $2 AND status = 'ACTIVE'`, now, groupID)
		if updateErr != nil {
			return Plan{}, updateErr
		}
		if tag.RowsAffected() != 1 {
			return Plan{}, ErrVersionConflict
		}
	}

	plan, err := scanPlan(tx.QueryRow(ctx, `
		INSERT INTO commerce_fulfilment_plan_versions (
			id, group_id, version, status, reason_code, reason_detail,
			planner_version, policy_version, delivery_mode,
			planned_cost_minor, currency, cost_snapshot, routing_evidence,
			fingerprint, supersedes_version, idempotency_key,
			created_at, activated_at)
		VALUES ($1,$2,$3,'ACTIVE',$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)
		RETURNING `+planColumns,
		planID, groupID, nextVersion, in.Reason, in.ReasonDetail,
		PlannerVersion, in.PolicyVersion, in.DeliveryMode,
		in.PlannedCostMinor, in.Currency,
		jsonOrEmptyObject(in.CostSnapshot), jsonOrEmptyObject(in.RoutingEvidence),
		fingerprint, supersedes, in.IdempotencyKey, now))
	if err != nil {
		return Plan{}, translateConstraint(err)
	}

	// The previous version is now history. Its execution reservation is
	// released when it never progressed beyond REQUESTED.
	if group.CurrentPlanVersion > 0 {
		// A reservation the previous version made but never bound stops
		// occupying its slot. It stays readable: the mission really was
		// requested, and a plan history that loses that cannot explain itself.
		//
		// An already-LINKED mission is deliberately left alone. The authority
		// owns it, a rider may already be holding the goods, and marking it
		// superseded here would be this service asserting something about
		// another service's mission that nobody told it. Genuinely replacing
		// an executing mission is ReplacePlatformMission, which requires an
		// explicit reason and keeps both rows.
		if _, err = tx.Exec(ctx, `
			UPDATE commerce_platform_mission_refs
			   SET link_state = 'SUPERSEDED', version = version + 1, updated_at = $1
			 WHERE group_id = $2 AND plan_id <> $3
			   AND link_state = 'REQUESTED'`, now, groupID, planID); err != nil {
			return Plan{}, err
		}
	}

	plan.Missions, err = insertPlanMissions(ctx, tx, plan, in.Missions, now)
	if err != nil {
		return Plan{}, err
	}

	// The evidence commits with the plan it justifies, in this transaction.
	// Writing it afterwards would leave a window in which an active plan
	// version exists that nobody can explain, and that window is precisely
	// when a crash would be noticed.
	plan.RequirementDecisions, err = insertRequirementDecisions(
		ctx, tx, plan, in.RequirementDecisions, now,
	)
	if err != nil {
		return Plan{}, err
	}

	tag, err := tx.Exec(ctx, `
		UPDATE commerce_fulfilment_groups
		   SET current_plan_id = $1, current_plan_version = $2,
		       version = version + 1, updated_at = $3
		 WHERE id = $4 AND version = $5`,
		planID, nextVersion, now, groupID, group.Version)
	if err != nil {
		return Plan{}, err
	}
	if tag.RowsAffected() != 1 {
		return Plan{}, ErrVersionConflict
	}

	if err = s.publish(ctx, tx, EventPlanActivated, groupID, nextVersion, "",
		map[string]any{
			"plan_id":          planID,
			"version":          nextVersion,
			"reason_code":      string(in.Reason),
			"missions":         len(in.Missions),
			"deliveries":       len(in.Missions),
			"parent_order_ref": group.ParentOrderID,
			"customer_ref":     group.CustomerID,
		}); err != nil {
		return Plan{}, err
	}

	return plan, tx.Commit(ctx)
}

// assertMissionUnits confirms every planned pickup names a unit of this group.
//
// Without it, a plan could reference another order's unit through a stop and
// build a mission that collects from a branch this purchase has no claim on.
func assertMissionUnits(
	ctx context.Context, tx pgx.Tx, groupID string, missions []PlanMission,
) error {
	seen := map[string]bool{}
	for _, mission := range missions {
		for _, unitID := range mission.UnitIDs() {
			if seen[unitID] {
				continue
			}
			seen[unitID] = true

			var owner string
			err := tx.QueryRow(ctx, `
				SELECT group_id FROM commerce_fulfilment_units WHERE id = $1`,
				unitID).Scan(&owner)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if owner != groupID {
				return ErrForbidden
			}
		}
	}
	return nil
}

// insertRequirementDecisions stores the vehicle-selection evidence for one
// plan version.
//
// The group and plan identifiers come from the PLAN, never from the record: a
// caller cannot file evidence against another group's plan by putting its id
// in the payload, which is the same rule every other write in this aggregate
// follows.
func insertRequirementDecisions(
	ctx context.Context,
	tx pgx.Tx,
	plan Plan,
	decisions []RequirementDecision,
	now time.Time,
) ([]RequirementDecision, error) {
	stored := make([]RequirementDecision, 0, len(decisions))

	for _, decision := range decisions {
		decisionID := "cdrs_" + uuid.NewString()

		if _, err := tx.Exec(ctx, `
			INSERT INTO commerce_delivery_requirement_snapshots (
				id, group_id, plan_id, slot_ref, business_id, branch_id,
				minimum_physical_class, selected_vehicle_class,
				selected_service, executable,
				primary_reason, supporting_reasons, candidates,
				measurement_state, measurement_source,
				total_weight_grams, total_volume_ml, effective_volume_ml,
				longest_dimension_mm, package_count,
				route_distance_meters, pickup_count,
				risk_class, declared_value_minor, declared_value_currency,
				policy_version, evidence, fingerprint, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
			        $16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)`,
			decisionID, plan.GroupID, plan.ID, decision.SlotRef,
			decision.BusinessID, decision.BranchID,
			decision.MinimumPhysicalClass, decision.SelectedVehicleClass,
			decision.SelectedService, decision.Executable,
			decision.PrimaryReason,
			marshalStrings(decision.SupportingReasons),
			jsonOrEmptyArray(decision.Candidates),
			decision.MeasurementState, decision.MeasurementSource,
			decision.TotalWeightGrams, decision.TotalVolumeMl,
			decision.EffectiveVolumeMl, decision.LongestDimensionMm,
			decision.PackageCount,
			decision.RouteDistanceMeters, decision.PickupCount,
			decision.RiskClass, decision.DeclaredValueMinor,
			decision.DeclaredValueCurrency,
			decision.PolicyVersion, jsonOrEmptyObject(decision.Evidence),
			decision.Fingerprint, now,
		); err != nil {
			return nil, translateConstraint(err)
		}

		decision.ID = decisionID
		decision.GroupID = plan.GroupID
		decision.PlanID = plan.ID
		decision.CreatedAt = now

		stored = append(stored, decision)
	}

	return stored, nil
}

// marshalStrings renders a string slice as a JSON array, never as null. A
// null in a jsonb array column fails the array type check and would turn a
// missing reason list into a failed write.
func marshalStrings(values []string) []byte {
	payload, err := json.Marshal(nonNilStrings(values))
	if err != nil {
		return []byte(`[]`)
	}

	return payload
}

const requirementDecisionColumns = `id, group_id, plan_id, slot_ref,
	business_id, branch_id,
	minimum_physical_class, selected_vehicle_class, selected_service,
	executable, primary_reason, supporting_reasons, candidates,
	measurement_state, measurement_source,
	total_weight_grams, total_volume_ml, effective_volume_ml,
	longest_dimension_mm, package_count,
	route_distance_meters, pickup_count,
	risk_class, declared_value_minor, declared_value_currency,
	policy_version, evidence, fingerprint, created_at`

func scanRequirementDecision(row pgx.Row) (RequirementDecision, error) {
	var (
		decision   RequirementDecision
		supporting []byte
		candidates []byte
		evidence   []byte
	)

	if err := row.Scan(
		&decision.ID, &decision.GroupID, &decision.PlanID, &decision.SlotRef,
		&decision.BusinessID, &decision.BranchID,
		&decision.MinimumPhysicalClass, &decision.SelectedVehicleClass,
		&decision.SelectedService,
		&decision.Executable, &decision.PrimaryReason, &supporting, &candidates,
		&decision.MeasurementState, &decision.MeasurementSource,
		&decision.TotalWeightGrams, &decision.TotalVolumeMl,
		&decision.EffectiveVolumeMl, &decision.LongestDimensionMm,
		&decision.PackageCount,
		&decision.RouteDistanceMeters, &decision.PickupCount,
		&decision.RiskClass, &decision.DeclaredValueMinor,
		&decision.DeclaredValueCurrency,
		&decision.PolicyVersion, &evidence, &decision.Fingerprint,
		&decision.CreatedAt,
	); err != nil {
		return RequirementDecision{}, err
	}

	_ = json.Unmarshal(supporting, &decision.SupportingReasons)
	decision.Candidates = json.RawMessage(candidates)
	decision.Evidence = json.RawMessage(evidence)

	return decision, nil
}

// loadRequirementDecisions reads the evidence stored against one plan version.
func (s *Store) loadRequirementDecisions(
	ctx context.Context, planID string,
) ([]RequirementDecision, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+requirementDecisionColumns+`
		  FROM commerce_delivery_requirement_snapshots
		 WHERE plan_id = $1
		 ORDER BY slot_ref, created_at`, planID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var decisions []RequirementDecision

	for rows.Next() {
		decision, scanErr := scanRequirementDecision(rows)
		if scanErr != nil {
			return nil, scanErr
		}

		decisions = append(decisions, decision)
	}

	return decisions, rows.Err()
}

func insertPlanMissions(
	ctx context.Context, tx pgx.Tx, plan Plan, missions []PlanMission, now time.Time,
) ([]PlanMission, error) {
	stored := make([]PlanMission, 0, len(missions))

	for _, mission := range missions {
		missionID := "cfpm_" + uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO commerce_fulfilment_plan_missions (
				id, plan_id, group_id, slot_ref, sequence, owner, authority,
				service_class, vehicle_class, planned_cost_minor, currency,
				loading_duration_seconds, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			missionID, plan.ID, plan.GroupID, mission.SlotRef, mission.Sequence,
			mission.Owner, mission.Authority, mission.ServiceClass,
			mission.VehicleClass, mission.PlannedCostMinor, mission.Currency,
			mission.LoadingDurationSeconds, now); err != nil {
			return nil, translateConstraint(err)
		}
		mission.ID = missionID
		mission.PlanID = plan.ID
		mission.CreatedAt = now

		for index := range mission.Stops {
			stop := &mission.Stops[index]
			stopID := "cfps_" + uuid.NewString()
			var unitRef any
			if strings.TrimSpace(stop.UnitID) != "" {
				unitRef = stop.UnitID
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO commerce_fulfilment_plan_stops (
					id, plan_mission_id, plan_id, sequence, stop_type,
					unit_id, business_id, branch_id, address, latitude, longitude, created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
				stopID, missionID, plan.ID, stop.Sequence, stop.Type,
				unitRef, stop.BusinessID, stop.BranchID,
				stop.Address, stop.Latitude, stop.Longitude, now); err != nil {
				return nil, translateConstraint(err)
			}
			stop.ID = stopID
		}

		for _, parcelID := range mission.ParcelIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO commerce_fulfilment_plan_mission_parcels (
					plan_mission_id, parcel_id, plan_id, group_id, created_at)
				VALUES ($1,$2,$3,$4,$5)`,
				missionID, parcelID, plan.ID, plan.GroupID, now); err != nil {
				return nil, translateConstraint(err)
			}
		}

		stored = append(stored, mission)
	}

	return stored, nil
}

func loadPlanMissions(ctx context.Context, q querier, planID string) ([]PlanMission, error) {
	rows, err := q.Query(ctx, `
		SELECT id, plan_id, slot_ref, sequence, owner, authority,
		       service_class, vehicle_class, planned_cost_minor, currency,
		       loading_duration_seconds, created_at
		  FROM commerce_fulfilment_plan_missions
		 WHERE plan_id = $1 ORDER BY sequence`, planID)
	if err != nil {
		return nil, err
	}

	var missions []PlanMission
	for rows.Next() {
		var mission PlanMission
		if err = rows.Scan(&mission.ID, &mission.PlanID, &mission.SlotRef,
			&mission.Sequence, &mission.Owner, &mission.Authority,
			&mission.ServiceClass, &mission.VehicleClass,
			&mission.PlannedCostMinor, &mission.Currency,
			&mission.LoadingDurationSeconds, &mission.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		missions = append(missions, mission)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	for index := range missions {
		if missions[index].Stops, err = loadStops(ctx, q, missions[index].ID); err != nil {
			return nil, err
		}
		if missions[index].ParcelIDs, err = loadMissionParcels(ctx, q, missions[index].ID); err != nil {
			return nil, err
		}
	}
	return missions, nil
}

func loadStops(ctx context.Context, q querier, planMissionID string) ([]Stop, error) {
	rows, err := q.Query(ctx, `
		SELECT id, sequence, stop_type, COALESCE(unit_id, ''),
		       business_id, branch_id, address, latitude, longitude
		  FROM commerce_fulfilment_plan_stops
		 WHERE plan_mission_id = $1 ORDER BY sequence`, planMissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stops []Stop
	for rows.Next() {
		var stop Stop
		if err = rows.Scan(&stop.ID, &stop.Sequence, &stop.Type, &stop.UnitID,
			&stop.BusinessID, &stop.BranchID, &stop.Address,
			&stop.Latitude, &stop.Longitude); err != nil {
			return nil, err
		}
		stops = append(stops, stop)
	}
	return stops, rows.Err()
}

func loadMissionParcels(ctx context.Context, q querier, planMissionID string) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT parcel_id FROM commerce_fulfilment_plan_mission_parcels
		 WHERE plan_mission_id = $1 ORDER BY parcel_id`, planMissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var parcels []string
	for rows.Next() {
		var parcelID string
		if err = rows.Scan(&parcelID); err != nil {
			return nil, err
		}
		parcels = append(parcels, parcelID)
	}
	return parcels, rows.Err()
}

// ListPlans reads a group's full plan history, newest first.
//
// Superseded versions are returned unchanged. A plan that was in force and is
// no longer is still the plan a delivery was executed under, and hiding it
// would make a later financial question unanswerable.
func (s *Store) ListPlans(ctx context.Context, groupID string) ([]Plan, error) {
	if !s.Ready() {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+planColumns+`
		  FROM commerce_fulfilment_plan_versions
		 WHERE group_id = $1 ORDER BY version DESC`, groupID)
	if err != nil {
		return nil, err
	}

	var plans []Plan
	for rows.Next() {
		plan, scanErr := scanPlan(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		plans = append(plans, plan)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	for index := range plans {
		if plans[index].Missions, err = loadPlanMissions(ctx, s.db, plans[index].ID); err != nil {
			return nil, err
		}

		// The evidence travels with the plan history. A history that lists
		// vehicle names without saying why they were chosen answers the
		// question nobody asks and not the one everybody does.
		if plans[index].RequirementDecisions, err = s.loadRequirementDecisions(
			ctx, plans[index].ID,
		); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

// GetPlan reads one plan version with its missions.
func (s *Store) GetPlan(ctx context.Context, groupID, planID string) (Plan, error) {
	if !s.Ready() {
		return Plan{}, ErrDependencyMissing
	}
	plan, err := scanPlan(s.db.QueryRow(ctx, `
		SELECT `+planColumns+`
		  FROM commerce_fulfilment_plan_versions
		 WHERE id = $1 AND group_id = $2`, planID, groupID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	if plan.Missions, err = loadPlanMissions(ctx, s.db, plan.ID); err != nil {
		return Plan{}, err
	}

	plan.RequirementDecisions, err = s.loadRequirementDecisions(ctx, plan.ID)

	return plan, err
}

// -----------------------------------------------------------------------------
// Platform mission references
// -----------------------------------------------------------------------------

const missionRefColumns = `id, group_id, plan_id, plan_mission_id, authority,
	mission_ref, external_ref, link_state, status_projection,
	status_source_version, status_source_ref, status_updated_at, dispatch_domain_projection,
	refusal_code, failure_reason, COALESCE(replaces_id, ''), replacement_reason,
	version, created_at, updated_at, linked_at`

func scanMissionRef(row pgx.Row) (PlatformMissionRef, error) {
	var ref PlatformMissionRef
	err := row.Scan(&ref.ID, &ref.GroupID, &ref.PlanID, &ref.PlanMissionID,
		&ref.Authority, &ref.MissionRef, &ref.ExternalRef, &ref.LinkState,
		&ref.StatusProjection, &ref.StatusSourceVersion, &ref.StatusSourceRef,
		&ref.StatusUpdatedAt, &ref.DispatchDomainProjection, &ref.RefusalCode, &ref.FailureReason,
		&ref.ReplacesID, &ref.ReplacementReason,
		&ref.Version, &ref.CreatedAt, &ref.UpdatedAt, &ref.LinkedAt)
	return ref, err
}

// EnsurePlatformMissionRef reserves the mission reference for one plan slot.
//
// This is what makes "create the mission exactly once" achievable across a
// crash. The reservation is durable BEFORE any authority is called, so a
// process that dies mid-call leaves a REQUESTED row a recovery pass can find,
// read back against the authority, and link — instead of a lost request that
// nothing records and a second vehicle that nobody ordered.
//
// A plan with three missions where the third call timed out recovers by
// resolving only the third: the first two are already LINKED and are not
// re-sent.
func (s *Store) EnsurePlatformMissionRef(
	ctx context.Context, groupID, planID, planMissionID string,
	authority Authority, now time.Time,
) (PlatformMissionRef, bool, error) {
	if !s.Ready() {
		return PlatformMissionRef{}, false, ErrDependencyMissing
	}
	if !authority.Platform() {
		return PlatformMissionRef{}, false, ErrInvalid
	}
	if !authority.ActiveForCommerce() {
		// Taxi lands here. Refusing is the honest answer; creating a
		// reservation against an integration that does not exist would make
		// the plan look executable.
		return PlatformMissionRef{}, false, ErrConflict
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return PlatformMissionRef{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err := scanMissionRef(tx.QueryRow(ctx, `
		SELECT `+missionRefColumns+`
		  FROM commerce_platform_mission_refs
		 WHERE plan_mission_id = $1
		   AND link_state IN ('REQUESTED','LINKED')
		 FOR UPDATE`, planMissionID))
	if err == nil {
		return ref, false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return PlatformMissionRef{}, false, err
	}

	id := "cpmr_" + uuid.NewString()
	// The mission reference is derived from the slot, so the same slot always
	// produces the same idempotency key at the authority.
	missionRef := "cm-" + shortSuffix(planMissionID)

	ref, err = scanMissionRef(tx.QueryRow(ctx, `
		INSERT INTO commerce_platform_mission_refs (
			id, group_id, plan_id, plan_mission_id, authority, mission_ref,
			link_state, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,'REQUESTED',1,$7,$7)
		RETURNING `+missionRefColumns,
		id, groupID, planID, planMissionID, authority, missionRef, now))
	if err != nil {
		return PlatformMissionRef{}, false, translateConstraint(err)
	}

	if err = s.publish(ctx, tx, EventMissionRequested, ref.ID, ref.Version, "",
		map[string]string{
			"authority":   string(authority),
			"mission_ref": missionRef,
		}); err != nil {
		return PlatformMissionRef{}, false, err
	}

	return ref, true, tx.Commit(ctx)
}

// LinkPlatformMission binds the authority's answer to the reservation.
//
// Binding is idempotent for the same external reference, which is exactly the
// unknown-outcome case: a recovery pass that reads the canonical mission back
// from the authority links the reference it finds, and a duplicate answer
// changes nothing. A DIFFERENT external reference on an already-linked slot is
// refused, because that would mean two canonical missions believe they are
// executing the same work.
func (s *Store) LinkPlatformMission(
	ctx context.Context, missionRef string, link MissionLink, now time.Time,
) (PlatformMissionRef, error) {
	if !s.Ready() {
		return PlatformMissionRef{}, ErrDependencyMissing
	}
	if err := link.Validate(); err != nil {
		return PlatformMissionRef{}, err
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return PlatformMissionRef{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err := scanMissionRef(tx.QueryRow(ctx, `
		SELECT `+missionRefColumns+`
		  FROM commerce_platform_mission_refs
		 WHERE mission_ref = $1 FOR UPDATE`, missionRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return PlatformMissionRef{}, ErrNotFound
	}
	if err != nil {
		return PlatformMissionRef{}, err
	}

	if ref.LinkState == LinkLinked {
		if link.Accepted && ref.ExternalRef == strings.TrimSpace(link.ExternalRef) {
			return ref, tx.Commit(ctx)
		}
		return PlatformMissionRef{}, ErrConflict
	}
	if ref.LinkState != LinkRequested {
		return PlatformMissionRef{}, ErrConflict
	}

	if !link.Accepted {
		ref, err = scanMissionRef(tx.QueryRow(ctx, `
			UPDATE commerce_platform_mission_refs
			   SET link_state = 'REFUSED', refusal_code = $1,
			       version = version + 1, updated_at = $2
			 WHERE id = $3 AND version = $4
			RETURNING `+missionRefColumns,
			strings.TrimSpace(link.RefusalCode), now, ref.ID, ref.Version))
		if err != nil {
			return PlatformMissionRef{}, err
		}
		return ref, tx.Commit(ctx)
	}

	ref, err = scanMissionRef(tx.QueryRow(ctx, `
		UPDATE commerce_platform_mission_refs
		   SET link_state = 'LINKED', external_ref = $1, linked_at = $2,
		       version = version + 1, updated_at = $2
		 WHERE id = $3 AND version = $4
		RETURNING `+missionRefColumns,
		strings.TrimSpace(link.ExternalRef), now, ref.ID, ref.Version))
	if err != nil {
		return PlatformMissionRef{}, translateConstraint(err)
	}

	if err = s.publish(ctx, tx, EventMissionLinked, ref.ID, ref.Version, "",
		map[string]string{
			"authority":    string(ref.Authority),
			"external_ref": ref.ExternalRef,
		}); err != nil {
		return PlatformMissionRef{}, err
	}

	return ref, tx.Commit(ctx)
}

// ReplacePlatformMission records that the canonical mission itself was
// replaced.
//
// The old row is NOT overwritten. It named a mission that really was requested
// and possibly executed, and an operational history that quietly loses the
// mission a parcel actually travelled on cannot be reconstructed afterwards.
func (s *Store) ReplacePlatformMission(
	ctx context.Context, missionRef, reason string, now time.Time,
) (PlatformMissionRef, error) {
	if !s.Ready() {
		return PlatformMissionRef{}, ErrDependencyMissing
	}
	if strings.TrimSpace(reason) == "" {
		return PlatformMissionRef{}, ErrInvalid
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return PlatformMissionRef{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	old, err := scanMissionRef(tx.QueryRow(ctx, `
		SELECT `+missionRefColumns+`
		  FROM commerce_platform_mission_refs
		 WHERE mission_ref = $1 FOR UPDATE`, missionRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return PlatformMissionRef{}, ErrNotFound
	}
	if err != nil {
		return PlatformMissionRef{}, err
	}
	if !old.LinkState.Occupies() {
		return PlatformMissionRef{}, ErrConflict
	}

	if _, err = tx.Exec(ctx, `
		UPDATE commerce_platform_mission_refs
		   SET link_state = 'REPLACED', replacement_reason = $1,
		       version = version + 1, updated_at = $2
		 WHERE id = $3 AND version = $4`,
		reason, now, old.ID, old.Version); err != nil {
		return PlatformMissionRef{}, err
	}

	id := "cpmr_" + uuid.NewString()
	replacement, err := scanMissionRef(tx.QueryRow(ctx, `
		INSERT INTO commerce_platform_mission_refs (
			id, group_id, plan_id, plan_mission_id, authority, mission_ref,
			link_state, replaces_id, replacement_reason,
			version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,'REQUESTED',$7,$8,1,$9,$9)
		RETURNING `+missionRefColumns,
		id, old.GroupID, old.PlanID, old.PlanMissionID, old.Authority,
		"cm-"+shortSuffix(id), old.ID, reason, now))
	if err != nil {
		return PlatformMissionRef{}, translateConstraint(err)
	}

	return replacement, tx.Commit(ctx)
}

// ProjectMissionStatus applies an authority's own status report.
//
// The source version is the whole safety mechanism. An event that is not
// strictly newer than what the projection already holds is IGNORED rather than
// rejected: an at-least-once pipeline redelivers events as a matter of course,
// and treating a redelivery as an error would turn normal operation into an
// alarm. What it must never do is regress — a delayed IN_TRANSIT cannot
// un-deliver a mission the authority has already reported delivered.
//
// applied reports whether the projection actually moved.
func (s *Store) ProjectMissionStatus(
	ctx context.Context, missionRef, status, sourceRef string,
	sourceVersion int64, now time.Time,
) (ref PlatformMissionRef, applied bool, err error) {
	if !s.Ready() {
		return PlatformMissionRef{}, false, ErrDependencyMissing
	}
	status = strings.TrimSpace(status)
	if status == "" || sourceVersion < 1 {
		return PlatformMissionRef{}, false, ErrInvalid
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return PlatformMissionRef{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err = scanMissionRef(tx.QueryRow(ctx, `
		SELECT `+missionRefColumns+`
		  FROM commerce_platform_mission_refs
		 WHERE mission_ref = $1 FOR UPDATE`, missionRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return PlatformMissionRef{}, false, ErrNotFound
	}
	if err != nil {
		return PlatformMissionRef{}, false, err
	}

	if !ref.AcceptStatusProjection(sourceVersion) {
		return ref, false, tx.Commit(ctx)
	}

	ref, err = scanMissionRef(tx.QueryRow(ctx, `
		UPDATE commerce_platform_mission_refs
		   SET status_projection = $1, status_source_version = $2,
		       status_source_ref = $3, status_updated_at = $4,
		       version = version + 1, updated_at = $4
		 WHERE id = $5 AND version = $6
		RETURNING `+missionRefColumns,
		status, sourceVersion, sourceRef, now, ref.ID, ref.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return PlatformMissionRef{}, false, ErrVersionConflict
	}
	if err != nil {
		return PlatformMissionRef{}, false, err
	}

	return ref, true, tx.Commit(ctx)
}

// LiveMissionRefForSlot resolves the mission currently executing one plan slot
// of one order.
//
// The slot is named by the plan's own slot reference — the same reference a
// logistics authority echoes on its events — so correlating an inbound event to
// the projection it belongs to is an equality check on data both sides already
// agreed on, never a parse of somebody else's identifier.
//
// Superseded and replaced references are excluded: they name missions that
// really happened, but not the one executing this slot now.
func (s *Store) LiveMissionRefForSlot(
	ctx context.Context, parentOrderID, slotRef string,
) (PlatformMissionRef, bool, error) {
	if !s.Ready() {
		return PlatformMissionRef{}, false, ErrDependencyMissing
	}
	// Scoped to the plan currently in force. After a replan the same slot
	// reference exists under both the old and the new version, and answering
	// with the old one would bind an authority's answer to work that has
	// already moved.
	ref, err := scanMissionRef(s.db.QueryRow(ctx, `
		SELECT `+prefixed(missionRefColumns, "r")+`
		  FROM commerce_platform_mission_refs r
		  JOIN commerce_fulfilment_plan_missions pm ON pm.id = r.plan_mission_id
		  JOIN commerce_fulfilment_groups g ON g.id = r.group_id
		 WHERE g.parent_order_id = $1 AND pm.slot_ref = $2
		   AND r.plan_id = g.current_plan_id
		   AND r.link_state IN ('REQUESTED','LINKED')
		 ORDER BY r.created_at DESC
		 LIMIT 1`, parentOrderID, slotRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return PlatformMissionRef{}, false, nil
	}
	if err != nil {
		return PlatformMissionRef{}, false, err
	}
	return ref, true, nil
}

// ParcelsForMissionSlot reads the parcels riding one plan mission slot.
func (s *Store) ParcelsForMissionSlot(ctx context.Context, planMissionID string) ([]Parcel, error) {
	return s.queryParcels(ctx, `
		SELECT `+prefixed(parcelColumns, "p")+`
		  FROM commerce_fulfilment_parcels p
		  JOIN commerce_fulfilment_plan_mission_parcels mp ON mp.parcel_id = p.id
		 WHERE mp.plan_mission_id = $1
		 ORDER BY p.unit_id, p.sequence`, planMissionID)
}

// prefixed qualifies a bare column list with a table alias, so one column list
// serves both the simple selects and the joined ones instead of drifting into
// two lists that have to be kept in agreement by hand.
func prefixed(columns, alias string) string {
	parts := strings.Split(columns, ",")
	for index, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		// COALESCE(...) wrappers already name their column; only bare
		// identifiers need qualifying.
		if strings.Contains(trimmed, "(") {
			parts[index] = strings.Replace(trimmed, "COALESCE(", "COALESCE("+alias+".", 1)
			continue
		}
		parts[index] = alias + "." + trimmed
	}
	return strings.Join(parts, ", ")
}

// ListMissionRefs reads a group's platform mission projections, including
// superseded and replaced ones.
func (s *Store) ListMissionRefs(ctx context.Context, groupID string) ([]PlatformMissionRef, error) {
	if !s.Ready() {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+missionRefColumns+`
		  FROM commerce_platform_mission_refs
		 WHERE group_id = $1 ORDER BY created_at, id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []PlatformMissionRef
	for rows.Next() {
		ref, scanErr := scanMissionRef(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// FindMissionRefByExternal resolves a projection from the authority's own
// opaque identifier.
//
// The authority is matched explicitly alongside it, because an external
// reference is only unique within the service that issued it.
func (s *Store) FindMissionRefByExternal(
	ctx context.Context, authority Authority, externalRef string,
) (PlatformMissionRef, error) {
	if !s.Ready() {
		return PlatformMissionRef{}, ErrDependencyMissing
	}
	ref, err := scanMissionRef(s.db.QueryRow(ctx, `
		SELECT `+missionRefColumns+`
		  FROM commerce_platform_mission_refs
		 WHERE authority = $1 AND external_ref = $2`, authority, externalRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return PlatformMissionRef{}, ErrNotFound
	}
	return ref, err
}

// -----------------------------------------------------------------------------
// Vendor missions
// -----------------------------------------------------------------------------

const vendorMissionColumns = `id, group_id, plan_id, plan_mission_id,
	business_id, branch_id, COALESCE(assigned_staff_id, ''), assigned_at,
	status, handover_ref, proof_ref, failure_reason,
	version, created_at, updated_at`

func scanVendorMission(row pgx.Row) (VendorMission, error) {
	var mission VendorMission
	err := row.Scan(&mission.ID, &mission.GroupID, &mission.PlanID,
		&mission.PlanMissionID, &mission.BusinessID, &mission.BranchID,
		&mission.AssignedStaffID, &mission.AssignedAt, &mission.Status,
		&mission.HandoverRef, &mission.ProofRef, &mission.FailureReason,
		&mission.Version, &mission.CreatedAt, &mission.UpdatedAt)
	return mission, err
}

// EnsureVendorMission opens the canonical mission for a vendor-owned slot.
//
// Only for a VENDOR-owned slot. A platform slot gets a PlatformMissionRef
// instead, and refusing here is what keeps the presence of a VendorMission a
// reliable statement that the vendor is the responsible executor.
func (s *Store) EnsureVendorMission(
	ctx context.Context, groupID, planID, planMissionID string, now time.Time,
) (VendorMission, error) {
	if !s.Ready() {
		return VendorMission{}, ErrDependencyMissing
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return VendorMission{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	mission, err := scanVendorMission(tx.QueryRow(ctx, `
		SELECT `+vendorMissionColumns+`
		  FROM commerce_vendor_missions WHERE plan_mission_id = $1`, planMissionID))
	if err == nil {
		return mission, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, err
	}

	var owner, authority string
	if err = tx.QueryRow(ctx, `
		SELECT owner, authority FROM commerce_fulfilment_plan_missions
		 WHERE id = $1 AND plan_id = $2`, planMissionID, planID).
		Scan(&owner, &authority); errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, ErrNotFound
	} else if err != nil {
		return VendorMission{}, err
	}
	if Owner(owner) != OwnerVendor {
		return VendorMission{}, ErrConflict
	}

	// The executing branch is the branch of the slot's own pickup stop, read
	// here rather than accepted from a caller.
	var businessID, branchID string
	if err = tx.QueryRow(ctx, `
		SELECT u.business_id, u.branch_id
		  FROM commerce_fulfilment_plan_stops s
		  JOIN commerce_fulfilment_units u ON u.id = s.unit_id
		 WHERE s.plan_mission_id = $1 AND s.stop_type = 'PICKUP'
		 ORDER BY s.sequence LIMIT 1`, planMissionID).
		Scan(&businessID, &branchID); errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, ErrConflict
	} else if err != nil {
		return VendorMission{}, err
	}

	id := "cvm_" + uuid.NewString()
	mission, err = scanVendorMission(tx.QueryRow(ctx, `
		INSERT INTO commerce_vendor_missions (
			id, group_id, plan_id, plan_mission_id, business_id, branch_id,
			status, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,'PLANNED',1,$7,$7)
		RETURNING `+vendorMissionColumns,
		id, groupID, planID, planMissionID, businessID, branchID, now))
	if err != nil {
		return VendorMission{}, translateConstraint(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_vendor_mission_runtime
		(vendor_mission_id,stage,version,updated_at) VALUES($1,'PLANNED',1,$2)
		ON CONFLICT(vendor_mission_id) DO NOTHING`, mission.ID, now); err != nil {
		return VendorMission{}, err
	}

	return mission, tx.Commit(ctx)
}

// AssignVendorMission puts one of the vendor's own riders on the mission.
//
// The rider must be staff of the owning business. That check is what keeps a
// vendor from assigning another vendor's staff, and it is done against the
// staff row rather than against anything the request supplied.
func (s *Store) AssignVendorMission(
	ctx context.Context, businessID, missionID, staffID string,
	expectedVersion int64, now time.Time,
) (VendorMission, error) {
	if !s.Ready() {
		return VendorMission{}, ErrDependencyMissing
	}
	if strings.TrimSpace(staffID) == "" {
		return VendorMission{}, ErrInvalid
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return VendorMission{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	mission, err := scanVendorMission(tx.QueryRow(ctx, `
		SELECT `+vendorMissionColumns+`
		  FROM commerce_vendor_missions WHERE id = $1 FOR UPDATE`, missionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, ErrNotFound
	}
	if err != nil {
		return VendorMission{}, err
	}
	if mission.BusinessID != businessID {
		return VendorMission{}, ErrNotFound
	}
	if mission.Version != expectedVersion {
		return VendorMission{}, ErrVersionConflict
	}
	if mission.Status.InCustody() || mission.Status == VendorMissionDelivered ||
		mission.Status == VendorMissionReturned {
		// Reassigning a rider who is already carrying the goods is a custody
		// transfer, not an edit to a field.
		return VendorMission{}, ErrConflict
	}

	var staffVendor string
	if err = tx.QueryRow(ctx, `
		SELECT vendor_id FROM vendor_staff WHERE id = $1`, staffID).
		Scan(&staffVendor); errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, ErrNotFound
	} else if err != nil {
		return VendorMission{}, err
	}
	if staffVendor != businessID {
		return VendorMission{}, ErrForbidden
	}

	next := mission.Status
	if next == VendorMissionPlanned {
		next = VendorMissionAssigned
	}

	mission, err = scanVendorMission(tx.QueryRow(ctx, `
		UPDATE commerce_vendor_missions
		   SET assigned_staff_id = $1, assigned_at = $2, status = $3,
		       version = version + 1, updated_at = $2
		 WHERE id = $4 AND version = $5
		RETURNING `+vendorMissionColumns,
		staffID, now, next, missionID, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, ErrVersionConflict
	}
	if err != nil {
		return VendorMission{}, translateConstraint(err)
	}

	if err = s.publish(ctx, tx, EventVendorMission, mission.ID, mission.Version,
		mission.BusinessID, map[string]string{
			"status":   string(mission.Status),
			"staff_id": staffID,
		}); err != nil {
		return VendorMission{}, err
	}

	return mission, tx.Commit(ctx)
}

// TransitionVendorMissionStatus advances a vendor-owned mission.
//
// Every step is checked against the legal successor set before it is written,
// so there is no path by which a caller PATCHes a mission straight to
// delivered. Custody-moving steps also write the parcel custody chain in the
// same transaction, because a delivery whose evidence commits separately can
// be half-recorded.
func (s *Store) TransitionVendorMissionStatus(
	ctx context.Context, businessID, missionID string,
	transition VendorMissionTransition, now time.Time,
) (VendorMission, error) {
	if !s.Ready() {
		return VendorMission{}, ErrDependencyMissing
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return VendorMission{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	mission, err := scanVendorMission(tx.QueryRow(ctx, `
		SELECT `+vendorMissionColumns+`
		  FROM commerce_vendor_missions WHERE id = $1 FOR UPDATE`, missionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, ErrNotFound
	}
	if err != nil {
		return VendorMission{}, err
	}
	if mission.BusinessID != businessID {
		return VendorMission{}, ErrNotFound
	}
	if mission.Version != transition.ExpectedVersion {
		return VendorMission{}, ErrVersionConflict
	}
	if err = transition.Validate(mission); err != nil {
		return VendorMission{}, err
	}

	handover := firstNonEmpty(transition.HandoverRef, mission.HandoverRef)
	proof := firstNonEmpty(transition.ProofRef, mission.ProofRef)

	mission, err = scanVendorMission(tx.QueryRow(ctx, `
		UPDATE commerce_vendor_missions
		   SET status = $1, handover_ref = $2, proof_ref = $3,
		       failure_reason = $4, version = version + 1, updated_at = $5
		 WHERE id = $6 AND version = $7
		RETURNING `+vendorMissionColumns,
		transition.To, handover, proof, transition.FailureReason,
		now, missionID, transition.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, ErrVersionConflict
	}
	if err != nil {
		return VendorMission{}, translateConstraint(err)
	}

	if custodyEvent, moves := vendorCustodyEventFor(transition.To); moves {
		parcels, parcelErr := missionParcelIDs(ctx, tx, mission.PlanMissionID)
		if parcelErr != nil {
			return VendorMission{}, parcelErr
		}
		for _, parcelID := range parcels {
			event := CustodyEvent{
				ParcelID:          parcelID,
				GroupID:           mission.GroupID,
				EventType:         custodyEvent,
				FromCustodianType: CustodianVendor,
				FromCustodianRef:  mission.BusinessID,
				ToCustodianType:   vendorCustodianFor(custodyEvent),
				ToCustodianRef:    mission.AssignedStaffID,
				Source:            SourceVendor,
				SourceEventID: fmt.Sprintf("vendor-mission-%s-%s-v%d",
					mission.ID, custodyEvent, mission.Version),
				SourceVersion:   mission.Version,
				VendorMissionID: mission.ID,
				HandoverRef:     handover,
				ProofRef:        proof,
				ReasonCode:      transition.FailureReason,
				OccurredAt:      now,
			}
			if err = recordCustodyTx(ctx, tx, event, now); err != nil {
				return VendorMission{}, err
			}
		}
	}

	if err = s.publish(ctx, tx, EventVendorMission, mission.ID, mission.Version,
		mission.BusinessID, map[string]string{
			"status": string(mission.Status),
		}); err != nil {
		return VendorMission{}, err
	}

	return mission, tx.Commit(ctx)
}

// vendorCustodyEventFor maps a vendor mission step onto the custody chain.
//
// Only steps that actually move the goods produce a custody link. Assignment
// does not: naming a rider is not the same as handing them the box.
func vendorCustodyEventFor(status VendorMissionStatus) (CustodyEventType, bool) {
	switch status {
	case VendorMissionReadyForPickup:
		return CustodyReady, true
	case VendorMissionCustodyTransferred:
		return CustodyHandedOver, true
	case VendorMissionDelivered:
		return CustodyDelivered, true
	case VendorMissionFailed:
		return CustodyFailed, true
	case VendorMissionReturnRequired:
		return CustodyReturnInitiated, true
	case VendorMissionReturned:
		return CustodyReturned, true
	default:
		return "", false
	}
}

func vendorCustodianFor(event CustodyEventType) CustodianType {
	switch event {
	case CustodyDelivered:
		return CustodianCustomer
	case CustodyReturned:
		return CustodianVendor
	default:
		return CustodianVendor
	}
}

func missionParcelIDs(ctx context.Context, q querier, planMissionID string) ([]string, error) {
	return loadMissionParcels(ctx, q, planMissionID)
}

// ListVendorMissions reads a business's vendor-owned missions.
func (s *Store) ListVendorMissions(
	ctx context.Context, businessID string, branchIDs []string, limit int,
) ([]VendorMission, error) {
	if !s.Ready() {
		return nil, ErrDependencyMissing
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `SELECT ` + vendorMissionColumns + `
		  FROM commerce_vendor_missions
		 WHERE business_id = $1`
	args := []any{businessID}
	if len(branchIDs) > 0 {
		// Branch-scoped staff see only their own branches. Filtering in SQL
		// rather than after the read means a branch outside the scope never
		// reaches the process at all.
		query += ` AND branch_id = ANY($2)`
		args = append(args, branchIDs)
		query += ` ORDER BY created_at DESC, id LIMIT $3`
	} else {
		query += ` ORDER BY created_at DESC, id LIMIT $2`
	}
	args = append(args, limit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var missions []VendorMission
	for rows.Next() {
		mission, scanErr := scanVendorMission(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		missions = append(missions, mission)
	}
	return missions, rows.Err()
}

// GetVendorMission reads one vendor mission with its route and parcels.
func (s *Store) GetVendorMission(
	ctx context.Context, businessID, missionID string,
) (VendorMission, error) {
	if !s.Ready() {
		return VendorMission{}, ErrDependencyMissing
	}
	mission, err := scanVendorMission(s.db.QueryRow(ctx, `
		SELECT `+vendorMissionColumns+`
		  FROM commerce_vendor_missions WHERE id = $1`, missionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorMission{}, ErrNotFound
	}
	if err != nil {
		return VendorMission{}, err
	}
	if mission.BusinessID != businessID {
		return VendorMission{}, ErrNotFound
	}
	if mission.Stops, err = loadStops(ctx, s.db, mission.PlanMissionID); err != nil {
		return VendorMission{}, err
	}
	mission.ParcelIDs, err = loadMissionParcels(ctx, s.db, mission.PlanMissionID)
	return mission, err
}

// -----------------------------------------------------------------------------
// Custody and returns
// -----------------------------------------------------------------------------

// RecordCustody appends one link to a parcel's custody chain.
//
// Safe to replay. The unique key on (parcel, source, event) turns a duplicate
// delivery into a no-op rather than a second entry, and the parcel's projected
// status only moves forward, so an event that arrives out of order cannot walk
// the journey backwards.
func (s *Store) RecordCustody(
	ctx context.Context, event CustodyEvent, now time.Time,
) (applied bool, err error) {
	if !s.Ready() {
		return false, ErrDependencyMissing
	}
	if err = event.Validate(); err != nil {
		return false, err
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err = recordCustodyTx(ctx, tx, event, now); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// recordCustodyTx is the in-transaction form, so custody evidence commits with
// whatever produced it instead of in a second transaction that may not run.
func recordCustodyTx(
	ctx context.Context, tx pgx.Tx, event CustodyEvent, now time.Time,
) error {
	if err := event.Validate(); err != nil {
		return err
	}

	parcel, err := scanParcel(tx.QueryRow(ctx, `
		SELECT `+parcelColumns+`
		  FROM commerce_fulfilment_parcels WHERE id = $1 FOR UPDATE`, event.ParcelID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if event.GroupID != "" && parcel.GroupID != event.GroupID {
		return ErrForbidden
	}

	from := event.FromCustodianType
	if from == "" {
		from = parcel.CustodianType
	}

	// Custody legality is checked only for an event that would actually move
	// the parcel forward. A stale or duplicate report is already handled
	// below by being recorded as evidence without advancing the projection,
	// and turning that into an error would make an at-least-once pipeline
	// look broken. An event that WOULD advance the parcel has to describe a
	// move that could physically have happened: goods cannot reach the
	// customer without passing through whoever was carrying them, and a
	// provider holding goods is never swapped for another outside an explicit
	// transfer.
	if AdvanceParcelStatus(parcel.Status, ParcelStatusFor(event.EventType)) {
		if err := checkCustodyTransition(event, from); err != nil {
			return err
		}
	}

	id := "cpce_" + uuid.NewString()
	tag, err := tx.Exec(ctx, `
		INSERT INTO commerce_parcel_custody_events (
			id, parcel_id, group_id, event_type,
			from_custodian_type, from_custodian_ref,
			to_custodian_type, to_custodian_ref,
			source, source_event_id, source_version,
			platform_mission_ref_id, vendor_mission_id,
			handover_ref, proof_ref, reason_code, occurred_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		ON CONFLICT (parcel_id, source, source_event_id) DO NOTHING`,
		id, parcel.ID, parcel.GroupID, event.EventType,
		from, event.FromCustodianRef,
		event.ToCustodianType, event.ToCustodianRef,
		event.Source, event.SourceEventID, event.SourceVersion,
		nullableText(event.PlatformMissionRefID), nullableText(event.VendorMissionID),
		event.HandoverRef, event.ProofRef, event.ReasonCode,
		event.OccurredAt, now)
	if err != nil {
		return translateConstraint(err)
	}
	if tag.RowsAffected() == 0 {
		// Already recorded. A redelivery is normal, not an error.
		return nil
	}

	next := ParcelStatusFor(event.EventType)
	if !AdvanceParcelStatus(parcel.Status, next) {
		// The chain keeps the evidence; the projection does not regress.
		return nil
	}

	// Custody leaving the vendor is what seals the manifest. Doing it here,
	// from evidence, means no request body can seal or unseal a parcel.
	sealed := "sealed_at"
	if next.InCustody() && parcel.Manifest.SealedAt == nil {
		sealed = "$4::timestamptz"
	}

	_, err = tx.Exec(ctx, `
		UPDATE commerce_fulfilment_parcels
		   SET status = $1, custodian_type = $2, custodian_ref = $3,
		       sealed_at = `+sealed+`,
		       version = version + 1, updated_at = $4
		 WHERE id = $5`,
		next, event.ToCustodianType, event.ToCustodianRef, now, parcel.ID)
	return err
}

// ListCustodyEvents reads one parcel's custody chain in journey order.
func (s *Store) ListCustodyEvents(ctx context.Context, parcelID string) ([]CustodyEvent, error) {
	if !s.Ready() {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, parcel_id, group_id, event_type,
		       from_custodian_type, from_custodian_ref,
		       to_custodian_type, to_custodian_ref,
		       source, source_event_id, source_version,
		       COALESCE(platform_mission_ref_id, ''), COALESCE(vendor_mission_id, ''),
		       handover_ref, proof_ref, reason_code, occurred_at, created_at
		  FROM commerce_parcel_custody_events
		 WHERE parcel_id = $1 ORDER BY occurred_at, id`, parcelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []CustodyEvent
	for rows.Next() {
		var event CustodyEvent
		if err = rows.Scan(&event.ID, &event.ParcelID, &event.GroupID, &event.EventType,
			&event.FromCustodianType, &event.FromCustodianRef,
			&event.ToCustodianType, &event.ToCustodianRef,
			&event.Source, &event.SourceEventID, &event.SourceVersion,
			&event.PlatformMissionRefID, &event.VendorMissionID,
			&event.HandoverRef, &event.ProofRef, &event.ReasonCode,
			&event.OccurredAt, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

const returnColumns = `id, parcel_id, group_id, reason_code,
	COALESCE(origin_platform_mission_ref_id, ''), COALESCE(origin_vendor_mission_id, ''),
	COALESCE(return_platform_mission_ref_id, ''), COALESCE(return_vendor_mission_id, ''),
	state, version, created_at, updated_at, completed_at`

func scanReturn(row pgx.Row) (ParcelReturn, error) {
	var parcelReturn ParcelReturn
	err := row.Scan(&parcelReturn.ID, &parcelReturn.ParcelID, &parcelReturn.GroupID,
		&parcelReturn.ReasonCode,
		&parcelReturn.OriginPlatformMissionRefID, &parcelReturn.OriginVendorMissionID,
		&parcelReturn.ReturnPlatformMissionRefID, &parcelReturn.ReturnVendorMissionID,
		&parcelReturn.State, &parcelReturn.Version,
		&parcelReturn.CreatedAt, &parcelReturn.UpdatedAt, &parcelReturn.CompletedAt)
	return parcelReturn, err
}

// InitiateReturn opens a return journey for a parcel.
//
// The parcel is not replaced. There is no "return copy" identity here on
// purpose: the customer, the vendor and a support agent must all still be
// looking at the same box, and a duplicate identity would give each of them a
// different one. A return may need a brand-new logistics mission, and that
// mission reference is linked to this record — the box it carries is not new.
func (s *Store) InitiateReturn(
	ctx context.Context, parcelID, reasonCode, idempotencyKey string, now time.Time,
) (ParcelReturn, error) {
	if !s.Ready() {
		return ParcelReturn{}, ErrDependencyMissing
	}
	reasonCode = strings.TrimSpace(reasonCode)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if reasonCode == "" || idempotencyKey == "" {
		return ParcelReturn{}, ErrInvalid
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return ParcelReturn{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	parcel, err := scanParcel(tx.QueryRow(ctx, `
		SELECT `+parcelColumns+`
		  FROM commerce_fulfilment_parcels WHERE id = $1 FOR UPDATE`, parcelID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ParcelReturn{}, ErrNotFound
	}
	if err != nil {
		return ParcelReturn{}, err
	}

	existing, err := scanReturn(tx.QueryRow(ctx, `
		SELECT `+returnColumns+`
		  FROM commerce_parcel_returns
		 WHERE parcel_id = $1 AND idempotency_key = $2`, parcelID, idempotencyKey))
	if err == nil {
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ParcelReturn{}, err
	}

	id := "cpr_" + uuid.NewString()
	parcelReturn, err := scanReturn(tx.QueryRow(ctx, `
		INSERT INTO commerce_parcel_returns (
			id, parcel_id, group_id, reason_code, state,
			idempotency_key, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,'INITIATED',$5,1,$6,$6)
		RETURNING `+returnColumns,
		id, parcel.ID, parcel.GroupID, reasonCode, idempotencyKey, now))
	if err != nil {
		return ParcelReturn{}, translateConstraint(err)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE commerce_fulfilment_parcels
		   SET status = 'RETURN_REQUIRED', version = version + 1, updated_at = $1
		 WHERE id = $2 AND status IN ('IN_TRANSIT','FAILED','DELIVERED')`,
		now, parcel.ID); err != nil {
		return ParcelReturn{}, err
	}

	if err = s.publish(ctx, tx, EventReturnInitiated, parcelReturn.ID,
		parcelReturn.Version, parcel.BusinessID, map[string]string{
			"parcel_id":   parcel.ID,
			"reason_code": reasonCode,
		}); err != nil {
		return ParcelReturn{}, err
	}

	return parcelReturn, tx.Commit(ctx)
}

// LinkReturnMission attaches the mission carrying a parcel back.
func (s *Store) LinkReturnMission(
	ctx context.Context, returnID, platformMissionRefID, vendorMissionID string,
	now time.Time,
) (ParcelReturn, error) {
	if !s.Ready() {
		return ParcelReturn{}, ErrDependencyMissing
	}
	hasPlatform := strings.TrimSpace(platformMissionRefID) != ""
	hasVendor := strings.TrimSpace(vendorMissionID) != ""
	if hasPlatform == hasVendor {
		// Exactly one carrier. Neither leaves the return unexecutable; both
		// leaves it unclear who has the box.
		return ParcelReturn{}, ErrInvalid
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return ParcelReturn{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	parcelReturn, err := scanReturn(tx.QueryRow(ctx, `
		UPDATE commerce_parcel_returns
		   SET return_platform_mission_ref_id = $1, return_vendor_mission_id = $2,
		       state = 'IN_RETURN', version = version + 1, updated_at = $3
		 WHERE id = $4 AND state = 'INITIATED'
		RETURNING `+returnColumns,
		nullableText(platformMissionRefID), nullableText(vendorMissionID),
		now, returnID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ParcelReturn{}, ErrConflict
	}
	if err != nil {
		return ParcelReturn{}, translateConstraint(err)
	}

	return parcelReturn, tx.Commit(ctx)
}

// ListReturns reads a group's return journeys.
func (s *Store) ListReturns(ctx context.Context, groupID string) ([]ParcelReturn, error) {
	if !s.Ready() {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+returnColumns+`
		  FROM commerce_parcel_returns
		 WHERE group_id = $1 ORDER BY created_at DESC, id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var returns []ParcelReturn
	for rows.Next() {
		parcelReturn, scanErr := scanReturn(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		returns = append(returns, parcelReturn)
	}
	return returns, rows.Err()
}

// -----------------------------------------------------------------------------
// Group reads
// -----------------------------------------------------------------------------

// GetGroup reads one group with its units and parcels.
func (s *Store) GetGroup(ctx context.Context, groupID string) (Group, error) {
	if !s.Ready() {
		return Group{}, ErrDependencyMissing
	}
	group, err := scanGroup(s.db.QueryRow(ctx, `
		SELECT `+groupColumns+`
		  FROM commerce_fulfilment_groups WHERE id = $1`, groupID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrNotFound
	}
	if err != nil {
		return Group{}, err
	}
	if group.Units, err = s.ListUnits(ctx, group.ID); err != nil {
		return Group{}, err
	}
	group.Parcels, err = s.ListParcels(ctx, group.ID)
	return group, err
}

// GetGroupByOrder resolves the group for one commerce order.
func (s *Store) GetGroupByOrder(ctx context.Context, parentOrderID string) (Group, error) {
	if !s.Ready() {
		return Group{}, ErrDependencyMissing
	}
	group, err := scanGroup(s.db.QueryRow(ctx, `
		SELECT `+groupColumns+`
		  FROM commerce_fulfilment_groups WHERE parent_order_id = $1`, parentOrderID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrNotFound
	}
	if err != nil {
		return Group{}, err
	}
	if group.Units, err = s.ListUnits(ctx, group.ID); err != nil {
		return Group{}, err
	}
	group.Parcels, err = s.ListParcels(ctx, group.ID)
	return group, err
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// querier is the read surface shared by the pool and a transaction, so a load
// helper works inside and outside a transaction without duplication.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *Store) publish(
	ctx context.Context, tx pgx.Tx, eventType, aggregateID string,
	aggregateVersion int64, vendorID string, payload any,
) error {
	if s == nil || s.outbox == nil {
		return nil
	}
	return s.outbox.Publish(ctx, tx, eventType, AggregateType, aggregateID,
		aggregateVersion, vendorID, aggregateID, aggregateID, payload)
}

// translateConstraint turns a database rejection into the domain's own
// vocabulary.
//
// The constraints are the real enforcement; this only makes their answers
// legible to a caller. A unique violation on a plan version is a stale plan,
// not an internal error, and reporting it as one would hide a normal race
// behind a 500.
func translateConstraint(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505": // unique_violation
		switch {
		case strings.Contains(pgErr.ConstraintName, "plan_versions"):
			return fmt.Errorf("%w: constraint %s", ErrStalePlan, pgErr.ConstraintName)
		case strings.Contains(pgErr.ConstraintName, "one_live_slot"),
			strings.Contains(pgErr.ConstraintName, "mission_refs"):
			return ErrConflict
		default:
			return ErrConflict
		}
	case "23503": // foreign_key_violation
		return ErrNotFound
	case "23514": // check_violation
		return ErrInvalid
	}
	return err
}

func jsonOrEmptyObject(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte(`{}`)
	}
	return raw
}

func jsonOrEmptyArray(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte(`[]`)
	}
	return raw
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nullableText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// shortSuffix takes the trailing identifying part of an internal id, so a
// derived reference stays short without becoming ambiguous.
func shortSuffix(id string) string {
	if index := strings.LastIndex(id, "_"); index >= 0 && index+1 < len(id) {
		id = id[index+1:]
	}
	id = strings.ReplaceAll(id, "-", "")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
