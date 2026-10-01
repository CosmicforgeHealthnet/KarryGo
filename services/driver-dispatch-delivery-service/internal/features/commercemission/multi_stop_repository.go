package commercemission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

const vendorServiceTarget = "vendor-service"

type TargetEventEmitter interface {
	EmitTarget(ctx context.Context, tx pgx.Tx, targetService, eventType,
		aggregateType, aggregateID string, aggregateVersion int64,
		correlationID, causationID string, payload map[string]any) error
}

type multiStopMissionRecord struct {
	ID                 string
	CallerService      string
	OperationRef       string
	RequestFingerprint string
	CanonicalRef       string
	BookingID          string
	TripID             string
	RuntimeState       string
	QueueState         string
	SourceVersion      int64
	PlanRef            string
	PlanVersion        int64
	PlanFingerprint    string
	MissionSlotRef     string
	QuoteID            string
	QuoteReference     string
	QuoteRevision      int64
	QuoteFingerprint   string
	Mode               string
	ServiceClass       string
	CapabilityVersion  int64
	ReasonCode         string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type createMultiStopInput struct {
	Caller      string
	Fingerprint string
	Request     dispatchcommerce.MissionCreateRequest
	Quote       commercequote.MultiStopRecord
}

var errConcurrentOperation = errors.New("commerce mission operation already exists")

const multiStopMissionColumns = `id::text,caller_service,operation_ref,request_fingerprint,
	canonical_ref,COALESCE(booking_id::text,''),COALESCE(trip_id::text,''),runtime_state,
	queue_state,source_version,plan_ref,plan_version,plan_fingerprint,mission_slot_ref,
	quote_id,quote_reference,quote_revision,quote_fingerprint,mode,service_class,
	capability_version,reason_code,created_at,updated_at`

func (r *Repository) CreateMultiStop(ctx context.Context, input createMultiStopInput) (dispatchcommerce.Mission, error) {
	if r == nil || r.db == nil {
		return dispatchcommerce.Mission{}, ErrMissionNotFound
	}
	req := input.Request
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	operationID := uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_operations
		(id,caller_service,operation_ref,operation_type,request_fingerprint)
		VALUES($1,$2,$3,'CREATE',$4)`, operationID, input.Caller, req.OperationRef, input.Fingerprint); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return dispatchcommerce.Mission{}, errConcurrentOperation
		}
		return dispatchcommerce.Mission{}, err
	}

	first := orderedPickupStops(req.PickupStops, input.Quote.RouteSnapshot)[0]
	var totalWeight int64
	for _, parcel := range req.Parcels {
		totalWeight += parcel.WeightGrams
	}
	protection := r.protection(req.CustomerRef, req.OperationRef, serviceTier(req.Mode))
	var bookingID string
	if err = tx.QueryRow(ctx, `INSERT INTO dispatch_bookings(
		customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,
		dropoff_lat,dropoff_lng,fare_amount,currency,payment_method,payment_status,
		payment_ref,receiver_name,receiver_phone,receiver_customer_id,package_desc,package_weight,
		package_type,package_size,is_fragile,service_tier,
		rollout_protection_mode,rollout_policy_version,handover_required,
		pickup_evidence_required,delivery_evidence_required,provider_transfer_required)
		VALUES($1::uuid,'searching',$2,$3,$4,$5,$6,$7,$8,$9,'wallet','paid',$10,
		$11,$12,NULLIF($23,'')::uuid,$13,$14,'commerce','large',$15,$16,$17,$18,$19,$20,$21,$22) RETURNING id::text`,
		req.CustomerRef, first.Location.Address, first.Location.Latitude, first.Location.Longitude,
		req.Receiver.Address, req.Receiver.Latitude, req.Receiver.Longitude, input.Quote.FeeMinor,
		input.Quote.Currency, req.FundingReference, receiverDisplay(req.Receiver), req.Receiver.Phone,
		fmt.Sprintf("Commerce multi-stop mission with %d parcels", len(req.Parcels)),
		float64(totalWeight)/1000, hasFragile(req.Parcels), serviceTier(req.Mode),
		protection.Mode, protection.PolicyVersion, protection.HandoverRequired,
		protection.PickupEvidenceRequired, protection.DeliveryEvidenceRequired,
		protection.ProviderTransferRequired, req.ReceiverCustomerRef).Scan(&bookingID); err != nil {
		return dispatchcommerce.Mission{}, err
	}

	requestSnapshot, _ := json.Marshal(req)
	vendorOrders := uniqueVendorOrders(req.Parcels)
	canonicalRef := "dcm_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	var missionID string
	if err = tx.QueryRow(ctx, `INSERT INTO commerce_missions(
		mission_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,
		agreed_fee_minor,currency,quote_id,quote_reference,status,created_at,
		caller_service,operation_ref,request_fingerprint,canonical_ref,runtime_state,
		queue_state,source_version,plan_ref,plan_version,plan_fingerprint,mission_slot_ref,
		quote_revision,quote_fingerprint,mode,service_class,capability_version,
		funding_reference,request_snapshot,promised_by,updated_at)
		VALUES($1,$2,$3,$4::uuid,$5::uuid,$6,$7,$8,$9,'accepted',now(),$10,$11,$12,
		$13,'SEARCHING','SEARCHING',1,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,now())
		RETURNING id::text`, req.OperationRef, req.ParentOrderRef, vendorOrders, req.CustomerRef,
		bookingID, input.Quote.FeeMinor, input.Quote.Currency, input.Quote.ID, input.Quote.QuoteReference,
		input.Caller, req.OperationRef, input.Fingerprint, canonicalRef, req.Plan.PlanRef,
		req.Plan.PlanVersion, req.Plan.PlanFingerprint, req.Plan.MissionSlotRef,
		input.Quote.Revision, input.Quote.RequestFingerprint, string(req.Mode), req.ServiceClass,
		req.Vehicle.CapabilityVersion, req.FundingReference, requestSnapshot, req.PromisedBy).Scan(&missionID); err != nil {
		return dispatchcommerce.Mission{}, err
	}

	ordered := orderedPickupStops(req.PickupStops, input.Quote.RouteSnapshot)
	stopIDs := map[string]string{}
	stopViews := make([]dispatchcommerce.StopView, 0, len(ordered))
	for sequence, stop := range ordered {
		physical := strings.TrimSpace(stop.PhysicalStopRef)
		if physical == "" {
			physical = stop.StopRef
		}
		location, _ := json.Marshal(stop.Location)
		handling, _ := json.Marshal(stop.Handling)
		var stopID string
		if err = tx.QueryRow(ctx, `INSERT INTO commerce_mission_stops(
			commerce_mission_id,stop_ref,physical_stop_ref,sequence,vendor_ref,vendor_display,
			branch_ref,manifest_ref,manifest_version,location_snapshot,ready_from,ready_to,handling_snapshot)
			VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text`,
			missionID, stop.StopRef, physical, sequence+1, stop.VendorRef, stop.VendorDisplay,
			stop.BranchRef, stop.ManifestRef, stop.ManifestVersion, location, stop.ReadyWindow.From, stop.ReadyWindow.To, handling).Scan(&stopID); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		stopIDs[stop.StopRef] = stopID
		stopViews = append(stopViews, dispatchcommerce.StopView{StopRef: stop.StopRef, PhysicalStopRef: physical,
			Sequence: sequence + 1, VendorRef: stop.VendorRef, BranchRef: stop.BranchRef,
			ManifestRef: stop.ManifestRef, ManifestVersion: stop.ManifestVersion, ParcelRefs: append([]string(nil), stop.ParcelRefs...),
			ReadyWindow: stop.ReadyWindow, Status: "AWAITING_PICKUP", HandoverStatus: "REQUIRED",
			Location: stop.Location, Handling: stop.Handling})
	}

	parcelViews := make([]dispatchcommerce.ParcelView, 0, len(req.Parcels))
	for _, parcel := range req.Parcels {
		manifest, _ := json.Marshal(parcel.Manifest)
		handling, _ := json.Marshal(parcel.Handling)
		manifestCreatedAt := parcel.ManifestCreatedAt
		if manifestCreatedAt.IsZero() {
			manifestCreatedAt = time.Now().UTC()
		}
		if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_parcels(
			commerce_mission_id,stop_id,parcel_ref,vendor_ref,branch_ref,fulfilment_unit_ref,
			vendor_order_ref,manifest_ref,manifest_version,manifest_created_at,manifest_snapshot,weight_grams,volume_ml,length_mm,
			width_mm,height_mm,package_count,handling_snapshot)
			VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			missionID, stopIDs[parcel.PickupStopRef], parcel.ParcelRef, parcel.VendorRef, parcel.BranchRef,
			parcel.FulfilmentUnit, parcel.VendorOrderRef, parcel.ManifestRef, parcel.ManifestVersion, manifestCreatedAt, manifest, parcel.WeightGrams,
			parcel.VolumeMl, parcel.LengthMm, parcel.WidthMm, parcel.HeightMm, parcel.PackageCount, handling); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		parcelViews = append(parcelViews, dispatchcommerce.ParcelView{ParcelRef: parcel.ParcelRef,
			VendorRef: parcel.VendorRef, BranchRef: parcel.BranchRef, FulfilmentUnit: parcel.FulfilmentUnit,
			VendorOrderRef: parcel.VendorOrderRef, PickupStopRef: parcel.PickupStopRef,
			ManifestRef: parcel.ManifestRef, ManifestVersion: parcel.ManifestVersion, PackageCount: parcel.PackageCount,
			CustodianType: "VENDOR", Status: "AWAITING_PICKUP"})
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_mission_assignment_attempts(
		commerce_mission_id,attempt_number,booking_id,state) VALUES($1::uuid,1,$2::uuid,'SEARCHING')`, missionID, bookingID); err != nil {
		return dispatchcommerce.Mission{}, err
	}

	// Shared custody registers the same topology in the same transaction: a
	// committed mission always has a custody record, and a custody record
	// never exists without its mission.
	if r.custody != nil {
		if _, err = r.custody.RegisterMissionTx(ctx, tx, custodyRegistration(
			canonicalRef, req.ParentOrderRef, req.OperationRef, req.CorrelationID, ordered, req.Parcels,
		)); err != nil {
			return dispatchcommerce.Mission{}, err
		}
	}

	now := time.Now().UTC()
	view := dispatchcommerce.Mission{MissionRef: canonicalRef, OperationRef: req.OperationRef,
		BookingRef: bookingID, State: "SEARCHING", QueueState: "SEARCHING", Version: 1,
		Plan: req.Plan, Quote: req.Quote, Stops: stopViews, Parcels: parcelViews, CreatedAt: now, UpdatedAt: now}
	result, _ := json.Marshal(view)
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_operations SET commerce_mission_id=$1::uuid,
		result_snapshot=$2,updated_at=now() WHERE id=$3::uuid`, missionID, result, operationID); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	readyFrom, readyTo := req.PickupStops[0].ReadyWindow.From, req.PickupStops[0].ReadyWindow.To
	for _, stop := range req.PickupStops[1:] {
		if stop.ReadyWindow.From.After(readyFrom) {
			readyFrom = stop.ReadyWindow.From
		}
		if stop.ReadyWindow.To.After(readyTo) {
			readyTo = stop.ReadyWindow.To
		}
	}
	if err = r.emitCommerceEvent(ctx, tx, "commerce.mission.created.v1", canonicalRef, 1, req.CorrelationID, "", view, map[string]any{
		"planVersion":          req.Plan.PlanVersion,
		"mode":                 req.Mode,
		"promisedBy":           req.PromisedBy,
		"readyFrom":            readyFrom,
		"readyTo":              readyTo,
		"routeProvider":        input.Quote.RouteProvider,
		"routeEvidenceVersion": input.Quote.RouteEvidenceVersion,
		"routeObservedAt":      input.Quote.CreatedAt,
		"estimatedSeconds":     input.Quote.EstimatedSeconds,
		"pickupStopCount":      len(req.PickupStops),
		"completedPickupStops": 0,
	}); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	return view, nil
}

func (r *Repository) emitCommerceEvent(ctx context.Context, tx pgx.Tx, eventType,
	canonicalRef string, version int64, correlationID, causationID string, view dispatchcommerce.Mission, evidence ...map[string]any) error {
	if r.events == nil {
		return nil
	}
	payload := map[string]any{
		"missionRef": view.MissionRef, "operationRef": view.OperationRef, "planRef": view.Plan.PlanRef,
		"planVersion": view.Plan.PlanVersion, "missionSlotRef": view.Plan.MissionSlotRef,
		"state": view.State, "queueState": view.QueueState, "sourceVersion": view.Version,
		"parcelRefs": dispatchcommerce.SortedParcelRefs(view.Parcels), "occurredAt": view.UpdatedAt,
	}
	if len(evidence) > 0 {
		for key, value := range evidence[0] {
			payload[key] = value
		}
	}
	return r.events.EmitTarget(ctx, tx, vendorServiceTarget, eventType, "COMMERCE_MISSION",
		canonicalRef, version, correlationID, causationID, payload)
}

func (r *Repository) emitParcelEvent(
	ctx context.Context, tx pgx.Tx, missionID, canonicalRef, stopRef string,
	status string, version int64, now time.Time,
) error {
	if r.events == nil {
		return nil
	}
	var operationRef, missionSlotRef string
	if err := tx.QueryRow(ctx, `SELECT operation_ref,mission_slot_ref FROM commerce_missions WHERE id=$1::uuid`, missionID).Scan(&operationRef, &missionSlotRef); err != nil {
		return err
	}
	query := `SELECT p.parcel_ref FROM commerce_mission_parcels p
		JOIN commerce_mission_stops s ON s.id=p.stop_id
		WHERE p.commerce_mission_id=$1::uuid AND ($2='' OR s.stop_ref=$2) AND p.status=$3
		ORDER BY p.parcel_ref`
	rows, err := tx.Query(ctx, query, missionID, stopRef, status)
	if err != nil {
		return err
	}
	defer rows.Close()
	var parcelRefs []string
	for rows.Next() {
		var parcelRef string
		if err = rows.Scan(&parcelRef); err != nil {
			return err
		}
		parcelRefs = append(parcelRefs, parcelRef)
	}
	if err = rows.Err(); err != nil || len(parcelRefs) == 0 {
		return err
	}
	return r.events.EmitTarget(ctx, tx, vendorServiceTarget,
		"commerce.parcel.changed.v1", "COMMERCE_MISSION",
		canonicalRef, version, "", "", map[string]any{
			"missionRef": canonicalRef, "operationRef": operationRef,
			"missionSlotRef": missionSlotRef, "parcelRefs": parcelRefs,
			"status": status, "sourceVersion": version, "occurredAt": now,
		})
}

func (r *Repository) FindMultiStopByOperation(ctx context.Context, caller, operation string) (dispatchcommerce.Mission, string, bool, error) {
	return r.findMultiStop(ctx, `caller_service=$1 AND operation_ref=$2`, caller, operation)
}

func (r *Repository) FindMultiStopByCanonical(ctx context.Context, caller, canonical string) (dispatchcommerce.Mission, string, bool, error) {
	return r.findMultiStop(ctx, `caller_service=$1 AND canonical_ref=$2`, caller, canonical)
}

func (r *Repository) findMultiStop(ctx context.Context, predicate string, args ...any) (dispatchcommerce.Mission, string, bool, error) {
	record, err := scanMultiStopMission(r.db.QueryRow(ctx, `SELECT `+multiStopMissionColumns+` FROM commerce_missions WHERE `+predicate, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return dispatchcommerce.Mission{}, "", false, nil
	}
	if err != nil {
		return dispatchcommerce.Mission{}, "", false, err
	}
	view, err := r.loadMissionView(ctx, record)
	return view, record.RequestFingerprint, err == nil, err
}

func scanMultiStopMission(row rowScanner) (multiStopMissionRecord, error) {
	var record multiStopMissionRecord
	err := row.Scan(&record.ID, &record.CallerService, &record.OperationRef, &record.RequestFingerprint,
		&record.CanonicalRef, &record.BookingID, &record.TripID, &record.RuntimeState, &record.QueueState,
		&record.SourceVersion, &record.PlanRef, &record.PlanVersion, &record.PlanFingerprint,
		&record.MissionSlotRef, &record.QuoteID, &record.QuoteReference, &record.QuoteRevision,
		&record.QuoteFingerprint, &record.Mode, &record.ServiceClass, &record.CapabilityVersion,
		&record.ReasonCode, &record.CreatedAt, &record.UpdatedAt)
	return record, err
}

func (r *Repository) loadMissionView(ctx context.Context, record multiStopMissionRecord) (dispatchcommerce.Mission, error) {
	view := dispatchcommerce.Mission{MissionRef: record.CanonicalRef, OperationRef: record.OperationRef,
		BookingRef: record.BookingID, TripRef: record.TripID, State: record.RuntimeState,
		QueueState: record.QueueState, Version: record.SourceVersion,
		Plan: dispatchcommerce.PlanBinding{PlanRef: record.PlanRef, PlanVersion: record.PlanVersion,
			PlanFingerprint: record.PlanFingerprint, MissionSlotRef: record.MissionSlotRef},
		Quote: dispatchcommerce.QuoteBinding{QuoteID: record.QuoteID, QuoteReference: record.QuoteReference,
			QuoteRevision: record.QuoteRevision, QuoteFingerprint: record.QuoteFingerprint},
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, ReasonCode: record.ReasonCode}
	rows, err := r.db.Query(ctx, `SELECT stop_ref,physical_stop_ref,sequence,vendor_ref,branch_ref,
		manifest_ref,manifest_version,location_snapshot,ready_from,ready_to,handling_snapshot,status,handover_status
		FROM commerce_mission_stops WHERE commerce_mission_id=$1::uuid ORDER BY sequence`, record.ID)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var stop dispatchcommerce.StopView
		var location, handling []byte
		if err = rows.Scan(&stop.StopRef, &stop.PhysicalStopRef, &stop.Sequence, &stop.VendorRef,
			&stop.BranchRef, &stop.ManifestRef, &stop.ManifestVersion, &location, &stop.ReadyWindow.From, &stop.ReadyWindow.To,
			&handling, &stop.Status, &stop.HandoverStatus); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		_ = json.Unmarshal(location, &stop.Location)
		_ = json.Unmarshal(handling, &stop.Handling)
		view.Stops = append(view.Stops, stop)
	}
	if err = rows.Err(); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	parcelRows, err := r.db.Query(ctx, `SELECT parcel_ref,vendor_ref,branch_ref,fulfilment_unit_ref,
		vendor_order_ref,manifest_ref,manifest_version,package_count,custodian_type,custodian_ref,status,(SELECT stop_ref FROM commerce_mission_stops s WHERE s.id=p.stop_id)
		FROM commerce_mission_parcels p WHERE commerce_mission_id=$1::uuid ORDER BY created_at,parcel_ref`, record.ID)
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	defer parcelRows.Close()
	for parcelRows.Next() {
		var parcel dispatchcommerce.ParcelView
		if err = parcelRows.Scan(&parcel.ParcelRef, &parcel.VendorRef, &parcel.BranchRef,
			&parcel.FulfilmentUnit, &parcel.VendorOrderRef, &parcel.ManifestRef, &parcel.ManifestVersion,
			&parcel.PackageCount, &parcel.CustodianType, &parcel.CustodianRef, &parcel.Status,
			&parcel.PickupStopRef); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		view.Parcels = append(view.Parcels, parcel)
	}
	if err = parcelRows.Err(); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	byStop := map[string][]string{}
	for _, p := range view.Parcels {
		byStop[p.PickupStopRef] = append(byStop[p.PickupStopRef], p.ParcelRef)
	}
	for i := range view.Stops {
		view.Stops[i].ParcelRefs = byStop[view.Stops[i].StopRef]
	}
	return view, nil
}

func orderedPickupStops(stops []dispatchcommerce.PickupStop, route []byte) []dispatchcommerce.PickupStop {
	byRef := map[string]dispatchcommerce.PickupStop{}
	for _, stop := range stops {
		byRef[stop.StopRef] = stop
	}
	var sequence []dispatchcommerce.RouteStop
	_ = json.Unmarshal(route, &sequence)
	ordered := make([]dispatchcommerce.PickupStop, 0, len(stops))
	for _, item := range sequence {
		if stop, ok := byRef[item.StopRef]; ok {
			ordered = append(ordered, stop)
			delete(byRef, item.StopRef)
		}
	}
	left := make([]string, 0, len(byRef))
	for ref := range byRef {
		left = append(left, ref)
	}
	sort.Strings(left)
	for _, ref := range left {
		ordered = append(ordered, byRef[ref])
	}
	return ordered
}

func uniqueVendorOrders(parcels []dispatchcommerce.Parcel) []string {
	seen := map[string]bool{}
	var result []string
	for _, p := range parcels {
		if !seen[p.VendorOrderRef] {
			seen[p.VendorOrderRef] = true
			result = append(result, p.VendorOrderRef)
		}
	}
	sort.Strings(result)
	return result
}
func hasFragile(parcels []dispatchcommerce.Parcel) bool {
	for _, p := range parcels {
		if p.Handling.Fragile {
			return true
		}
	}
	return false
}
func receiverDisplay(point dispatchcommerce.Point) string {
	if strings.TrimSpace(point.Contact) != "" {
		return strings.TrimSpace(point.Contact)
	}
	return "Commerce recipient"
}
func serviceTier(mode dispatchcommerce.Mode) string {
	if mode == dispatchcommerce.ModeFastest {
		return "express"
	}
	return "standard"
}
