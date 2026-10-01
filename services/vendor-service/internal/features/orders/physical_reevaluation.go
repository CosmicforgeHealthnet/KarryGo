package orders

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Post-pick physical re-evaluation (Karryam V4 §14.6).
//
// Checkout froze a vehicle class on the ESTIMATED load. Picking, variable
// measurement and substitution can change the real load before custody. When
// packing completes, every live platform mission carrying this VendorOrder is
// re-answered by the same requirement engine and the owning authority's real
// quote:
//
//   - the class still fits, or a smaller one would: UNCHANGED. The platform
//     never downgrades a paid mission because a smaller vehicle is available;
//   - a larger safe class is required: ESCALATED. The old authority mission is
//     cancelled before custody (reading back an unknown outcome), the plan slot
//     is re-pointed to the new authority, and one mission is created under a
//     new deterministic reference. The customer's approved delivery price is
//     unchanged; any authority fee above it is recorded as platform-funded;
//   - no safe executable class, or the load is already in custody: REFUSED.
//     Packing is refused, so the goods are never handed to an unsafe vehicle.
//
// Order, VendorOrder, parcel and customer identity never change.

var (
	// ErrPhysicalReplanRequired refuses packing completion because no safe
	// executable class exists for the packed load (or it can no longer be
	// changed). The goods must not be handed over.
	ErrPhysicalReplanRequired = errors.New("orders: packed load needs a logistics replan")
)

// PhysicalTrigger names what caused a re-evaluation.
const PhysicalTriggerPackingCompleted = "PACKING_COMPLETED"

// Physical re-evaluation outcomes.
const (
	PhysicalUnchanged = "UNCHANGED"
	PhysicalEscalated = "ESCALATED"
	PhysicalRefused   = "REFUSED"
)

// PhysicalDecision is the evaluator's answer for one mission's packed load.
type PhysicalDecision struct {
	// Selected reports that a safe executable class exists.
	Selected bool

	Service      LogisticsService
	VehicleClass string

	// ReasonCode explains a refusal.
	ReasonCode string

	PolicyVersion string

	// AuthorityFeeMinor and the quote identity are the selected authority's
	// real quote. Required for an escalation; ignored otherwise.
	AuthorityFeeMinor       int64
	AuthorityQuoteID        string
	AuthorityQuoteReference string

	ConfirmedWeightGrams  int64
	ConfirmedPackageCount int64

	// EvidenceFingerprint identifies the physical evidence (64 hex chars).
	// The same packed load evaluated twice yields one decision row.
	EvidenceFingerprint string

	// Evidence is the immutable requirement snapshot, for audit.
	Evidence json.RawMessage
}

// PhysicalLoadEvaluator re-plans one mission's final load for platform
// execution. Implemented in the composition root over the Section 17C planner.
type PhysicalLoadEvaluator interface {
	EvaluateMissionLoad(ctx context.Context, order ParentOrder, mission OrderMission) (PhysicalDecision, error)
}

// PhysicalReplanProjection re-points one plan slot of the operational
// fulfilment aggregate to another authority (append-only plan version).
type PhysicalReplanProjection interface {
	ReassignSlotAuthority(ctx context.Context, order ParentOrder, planMissionRef string,
		service LogisticsService, idempotencyKey string) error
}

// SetPhysicalLoadEvaluator wires post-pick re-evaluation. Nil leaves packing
// exactly as before (no re-evaluation), which only pre-§14 deployments use.
func (s *Service) SetPhysicalLoadEvaluator(evaluator PhysicalLoadEvaluator) {
	s.physical = evaluator
}

// PhysicalReevaluation is one persisted decision.
type PhysicalReevaluation struct {
	ID                       string           `json:"id"`
	MissionID                string           `json:"mission_id"`
	Outcome                  string           `json:"outcome"`
	ReasonCode               string           `json:"reason_code,omitempty"`
	PreviousService          LogisticsService `json:"previous_service"`
	PreviousVehicleClass     string           `json:"previous_vehicle_class"`
	SelectedService          LogisticsService `json:"selected_service,omitempty"`
	SelectedVehicleClass     string           `json:"selected_vehicle_class,omitempty"`
	MissionRef               string           `json:"mission_ref,omitempty"`
	AuthorityFeeMinor        int64            `json:"authority_fee_minor"`
	CustomerFeeMinor         int64            `json:"customer_fee_minor"`
	PlatformFundedDeltaMinor int64            `json:"platform_funded_delta_minor"`
}

// vehicleRank orders classes smallest first. Unknown ranks highest so an
// unrecognised stored class is never "downgraded" onto.
func vehicleRank(class string) int {
	switch strings.ToUpper(strings.TrimSpace(class)) {
	case "MOTORCYCLE":
		return 1
	case "CAR":
		return 2
	case "VAN":
		return 3
	case "TRUCK":
		return 4
	default:
		return 99
	}
}

func custodyStarted(status VendorStatus) bool {
	switch status {
	case VendorStatusHandedOver, VendorStatusPickedUp, VendorStatusDelivering,
		VendorStatusDelivered, VendorStatusCompleted:
		return true
	default:
		return false
	}
}

func (s *Store) lockPhysicalReplan(ctx context.Context, orderID string) (*pgxpool.Conn, error) {
	for {
		lease, err := s.db.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		var acquired bool
		err = lease.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,14125))`, orderID).Scan(&acquired)
		if err != nil {
			lease.Release()
			return nil, err
		}
		if acquired {
			return lease, nil
		}
		lease.Release()
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// ReevaluatePhysicalLoad re-answers every live platform mission carrying the
// VendorOrder from its packed/measured/substituted load.
func (s *Service) ReevaluatePhysicalLoad(ctx context.Context, parentOrderID, vendorOrderID string) ([]PhysicalReevaluation, error) {
	if s == nil || s.physical == nil || s.store == nil {
		return nil, nil
	}
	// Keep cancellation, operational-plan replacement and mission creation
	// single-winner across processes. A loser releases its connection while
	// waiting so it cannot starve the winner's normal repository calls.
	lease, err := s.store.lockPhysicalReplan(ctx, parentOrderID)
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, unlockErr := lease.Exec(cleanup, `SELECT pg_advisory_unlock(hashtextextended($1,14125))`, parentOrderID); unlockErr != nil {
			_ = lease.Conn().Close(cleanup)
		}
		lease.Release()
	}()
	order, err := s.store.GetParentByID(ctx, parentOrderID)
	if err != nil {
		return nil, err
	}
	var target *VendorOrder
	statuses := map[string]VendorStatus{}
	for index := range order.VendorOrders {
		statuses[order.VendorOrders[index].ID] = order.VendorOrders[index].Status
		if order.VendorOrders[index].ID == vendorOrderID {
			target = &order.VendorOrders[index]
		}
	}
	if target == nil {
		return nil, ErrOrderNotFound
	}
	if target.DeliveryResponsibility != DeliveryPlatform {
		// Vendor-owned delivery is the vendor's own vehicle policy.
		return nil, nil
	}
	missions, err := s.store.ListMissions(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	var outcomes []PhysicalReevaluation
	for _, mission := range missions {
		if mission.Status != MissionPending && mission.Status != MissionAccepted {
			continue
		}
		carries := false
		inCustody := false
		for _, id := range mission.VendorOrderIDs {
			if id == vendorOrderID {
				carries = true
			}
			if custodyStarted(statuses[id]) {
				inCustody = true
			}
		}
		if !carries {
			continue
		}
		decision, evalErr := s.physical.EvaluateMissionLoad(ctx, order, mission)
		if evalErr != nil {
			return outcomes, evalErr
		}
		outcome, reevalErr := s.applyPhysicalDecision(ctx, order, mission, vendorOrderID, decision, inCustody)
		if outcome.ID != "" {
			outcomes = append(outcomes, outcome)
		}
		if reevalErr != nil {
			return outcomes, reevalErr
		}
	}
	s.publishCustomerGrouping(ctx, order, nil)
	return outcomes, nil
}

func (s *Service) applyPhysicalDecision(ctx context.Context, order ParentOrder, mission OrderMission,
	vendorOrderID string, decision PhysicalDecision, inCustody bool) (PhysicalReevaluation, error) {
	row := physicalRow{
		MissionID: mission.ID, ParentOrderID: order.ID, PlanMissionRef: mission.PlanMissionRef,
		VendorOrderID: vendorOrderID, Fingerprint: decision.EvidenceFingerprint,
		PolicyVersion: decision.PolicyVersion, PreviousService: mission.Service,
		PreviousClass: mission.VehicleClass, PreviousRef: mission.MissionRef,
		PreviousFee: mission.AgreedFeeMinor, CustomerFee: mission.AgreedFeeMinor,
		WeightGrams: decision.ConfirmedWeightGrams, PackageCount: decision.ConfirmedPackageCount,
		Evidence: decision.Evidence,
	}
	if customerFee, ok := s.customerDeliveryFee(ctx, order, mission); ok {
		row.CustomerFee = customerFee
	}
	if len(row.Fingerprint) != 64 {
		return PhysicalReevaluation{}, ErrDependencyMissing
	}

	switch {
	case !decision.Selected:
		row.Outcome, row.Reason = PhysicalRefused, nonEmpty(decision.ReasonCode, "NO_SAFE_EXECUTABLE_CLASS")
	case vehicleRank(decision.VehicleClass) <= vehicleRank(mission.VehicleClass):
		// Still safe. A smaller class that would also fit is NOT taken: the
		// frozen mission is already safe, and supply never downgrades a load.
		row.Outcome = PhysicalUnchanged
		row.SelectedService, row.SelectedClass = mission.Service, mission.VehicleClass
	case inCustody:
		row.Outcome, row.Reason = PhysicalRefused, "LOAD_ALREADY_IN_CUSTODY"
	case s.logisticsPlanner != nil:
		// Just-in-time orchestration owns this slot's authority choice until it
		// commits; switching it here would race the orchestrator.
		row.Outcome, row.Reason = PhysicalRefused, "JIT_ESCALATION_REQUIRES_ORCHESTRATION"
	case decision.AuthorityQuoteID == "" || decision.AuthorityFeeMinor <= 0:
		row.Outcome, row.Reason = PhysicalRefused, "ESCALATION_QUOTE_UNAVAILABLE"
	default:
		row.Outcome = PhysicalEscalated
	}

	if row.Outcome != PhysicalEscalated {
		recorded, err := s.store.RecordPhysicalReevaluation(ctx, row, s.now().UTC())
		if err != nil {
			return PhysicalReevaluation{}, err
		}
		if recorded.Outcome == PhysicalRefused {
			return recorded, ErrPhysicalReplanRequired
		}
		return recorded, nil
	}
	return s.escalateMission(ctx, order, mission, decision, row)
}

// escalateMission moves one mission to a larger safe class.
//
// Order of operations is what prevents a duplicate vehicle:
//  1. cancel the old authority mission (idempotent, reads back unknown);
//  2. re-point the plan slot (append-only plan version);
//  3. atomically move the order mission to a NEW deterministic reference and
//     record the evidence (one winner under concurrency);
//  4. create exactly one mission under that reference (idempotent at the
//     authority, read back on an unknown outcome).
func (s *Service) escalateMission(ctx context.Context, order ParentOrder, mission OrderMission,
	decision PhysicalDecision, row physicalRow) (PhysicalReevaluation, error) {
	if existing, found, err := s.store.FindPhysicalReevaluation(ctx, mission.ID, row.Fingerprint); err != nil {
		return PhysicalReevaluation{}, err
	} else if found && existing.Outcome == PhysicalEscalated {
		// Replay after a crash: the move already happened; make sure the
		// replacement mission exists.
		return existing, s.createEscalatedMission(ctx, order, existing)
	}
	canceller, ok := s.missions.(MissionCancellationAuthority)
	if !ok || s.missions == nil {
		return PhysicalReevaluation{}, ErrDependencyMissing
	}
	if !s.missions.Available(decision.Service) {
		row.Outcome, row.Reason = PhysicalRefused, "ESCALATION_AUTHORITY_UNAVAILABLE"
		recorded, err := s.store.RecordPhysicalReevaluation(ctx, row, s.now().UTC())
		if err != nil {
			return PhysicalReevaluation{}, err
		}
		return recorded, ErrPhysicalReplanRequired
	}
	if mission.Status == MissionAccepted || mission.FailureReason != "" || mission.ExternalRef != "" {
		// A PENDING mission with a recorded failure may exist at the authority
		// (unknown outcome); cancel reads it back rather than assuming.
		if err := canceller.CancelMission(ctx, mission.MissionRef, "PHYSICAL_LOAD_ESCALATION"); err != nil {
			return PhysicalReevaluation{}, err
		}
	}
	if projection, ok := s.fulfilmentProjection.(PhysicalReplanProjection); ok {
		if err := projection.ReassignSlotAuthority(ctx, order, mission.PlanMissionRef,
			decision.Service, "physical:"+mission.ID+":"+row.Fingerprint); err != nil {
			return PhysicalReevaluation{}, err
		}
	}
	row.Outcome = PhysicalEscalated
	row.SelectedService, row.SelectedClass = decision.Service, strings.ToUpper(strings.TrimSpace(decision.VehicleClass))
	row.AuthorityFee = decision.AuthorityFeeMinor
	row.QuoteID, row.QuoteReference = decision.AuthorityQuoteID, decision.AuthorityQuoteReference
	row.NewRef = physicalMissionRef(mission, row.Fingerprint)
	if row.AuthorityFee > row.CustomerFee {
		row.PlatformDelta = row.AuthorityFee - row.CustomerFee
	}
	recorded, err := s.store.EscalateMission(ctx, mission, row, s.now().UTC())
	if err != nil {
		return PhysicalReevaluation{}, err
	}
	return recorded, s.createEscalatedMission(ctx, order, recorded)
}

func physicalMissionRef(mission OrderMission, fingerprint string) string {
	base := mission.MissionRef
	if index := strings.Index(base, "~px"); index > 0 {
		base = base[:index]
	}
	return base + "~px" + fingerprint[:16]
}

// createEscalatedMission creates the one replacement mission. Idempotent on
// its reference; an unreachable authority leaves the mission PENDING with the
// failure recorded for recovery, exactly like activation.
func (s *Service) createEscalatedMission(ctx context.Context, order ParentOrder, evidence PhysicalReevaluation) error {
	mission, err := s.store.GetMissionByRef(ctx, evidence.MissionRef)
	if err != nil {
		return err
	}
	if mission.Status != MissionPending {
		return nil
	}
	// The resolver checks the customer-approved fee against the frozen quote,
	// then applies the escalation to what the authority executes.
	plan := MissionPlan{
		MissionRef: mission.MissionRef, PlanMissionRef: mission.PlanMissionRef,
		Service: mission.Service, VehicleClass: mission.VehicleClass,
		AgreedFeeMinor: evidence.CustomerFeeMinor, VendorOrderIDs: mission.VendorOrderIDs,
	}
	if resolver, ok := s.fulfilmentProjection.(MissionPlanResolver); ok {
		if plan, err = resolver.ResolveMissionPlan(ctx, order, plan); err != nil {
			return err
		}
		plan.MissionRef = mission.MissionRef
	} else {
		plan.AgreedFeeMinor = mission.AgreedFeeMinor
	}
	var execution *MissionExecution
	if projection, ok := s.fulfilmentProjection.(MissionExecutionProjection); ok {
		snapshot, snapshotErr := projection.MissionExecution(ctx, order, plan)
		if snapshotErr != nil {
			return snapshotErr
		}
		execution = &snapshot
	}
	result, callErr := s.missions.CreateMission(ctx, MissionRequest{
		MissionRef: mission.MissionRef, ParentOrderID: order.ID, VendorOrderIDs: mission.VendorOrderIDs,
		CustomerID: order.CustomerID, Service: plan.Service, VehicleClass: plan.VehicleClass,
		Currency: order.Currency, AgreedFeeMinor: plan.AgreedFeeMinor, FundingReference: order.QuoteID,
		PlanMissionRef: plan.PlanMissionRef, PickupBusiness: plan.PickupBusiness, PickupBranch: plan.PickupBranch,
		DeliveryAddress: order.Address, Receiver: order.Receiver, WeightGrams: plan.WeightGrams,
		PackageCount: plan.PackageCount, Execution: execution,
	})
	if callErr != nil {
		_ = s.store.RecordMissionFailure(ctx, mission.ID, redactAuthorityError(callErr), s.now().UTC())
		return callErr
	}
	if err = s.store.RecordMissionResult(ctx, mission.ID, result, s.now().UTC()); err != nil {
		return err
	}
	if s.fulfilmentProjection != nil {
		return s.fulfilmentProjection.LinkMission(ctx, order, plan, result)
	}
	return nil
}

// customerDeliveryFee is what the customer approved for this mission: the
// checkout-frozen fee. Escalation never changes it.
func (s *Service) customerDeliveryFee(ctx context.Context, order ParentOrder, mission OrderMission) (int64, bool) {
	fee, err := s.store.FrozenMissionFee(ctx, order.CustomerID, mission.PlanMissionRef)
	if err != nil {
		return 0, false
	}
	return fee, true
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

type physicalRow struct {
	MissionID, ParentOrderID, PlanMissionRef, VendorOrderID string
	Fingerprint, PolicyVersion, Outcome, Reason             string
	PreviousService                                         LogisticsService
	PreviousClass, PreviousRef                              string
	PreviousFee, CustomerFee, AuthorityFee, PlatformDelta   int64
	SelectedService                                         LogisticsService
	SelectedClass, NewRef, QuoteID, QuoteReference          string
	WeightGrams, PackageCount                               int64
	Evidence                                                json.RawMessage
}

const physicalColumns = `id,mission_id,outcome,reason_code,previous_service,previous_vehicle_class,
	selected_service,selected_vehicle_class,mission_ref,authority_fee_minor,customer_fee_minor,
	platform_funded_delta_minor`

func scanPhysical(row pgx.Row) (PhysicalReevaluation, error) {
	var out PhysicalReevaluation
	var previous, selected string
	err := row.Scan(&out.ID, &out.MissionID, &out.Outcome, &out.ReasonCode, &previous,
		&out.PreviousVehicleClass, &selected, &out.SelectedVehicleClass, &out.MissionRef,
		&out.AuthorityFeeMinor, &out.CustomerFeeMinor, &out.PlatformFundedDeltaMinor)
	out.PreviousService, out.SelectedService = LogisticsService(previous), LogisticsService(selected)
	return out, err
}

func insertPhysical(ctx context.Context, tx pgx.Tx, row physicalRow, now time.Time) (PhysicalReevaluation, error) {
	evidence := row.Evidence
	if len(evidence) == 0 {
		evidence = json.RawMessage(`{}`)
	}
	return scanPhysical(tx.QueryRow(ctx, `
		INSERT INTO commerce_mission_physical_reevaluations(
			id,mission_id,parent_order_id,plan_mission_ref,vendor_order_id,trigger,evidence_fingerprint,
			outcome,reason_code,policy_version,previous_service,previous_vehicle_class,previous_mission_ref,
			previous_fee_minor,selected_service,selected_vehicle_class,mission_ref,authority_fee_minor,
			authority_quote_id,authority_quote_reference,customer_fee_minor,platform_funded_delta_minor,
			confirmed_weight_grams,confirmed_package_count,decision,created_at)
		VALUES($1,$2,$3,$4,$5,'`+PhysicalTriggerPackingCompleted+`',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
		RETURNING `+physicalColumns,
		"cmpr_"+uuid.NewString(), row.MissionID, row.ParentOrderID, row.PlanMissionRef, row.VendorOrderID,
		row.Fingerprint, row.Outcome, row.Reason, row.PolicyVersion, string(row.PreviousService),
		row.PreviousClass, row.PreviousRef, row.PreviousFee, string(row.SelectedService), row.SelectedClass,
		row.NewRef, row.AuthorityFee, row.QuoteID, row.QuoteReference, row.CustomerFee, row.PlatformDelta,
		row.WeightGrams, row.PackageCount, evidence, now))
}

// FindPhysicalReevaluation reads the decision already recorded for the same
// packed evidence, if any.
func (s *Store) FindPhysicalReevaluation(ctx context.Context, missionID, fingerprint string) (PhysicalReevaluation, bool, error) {
	if s == nil || s.db == nil {
		return PhysicalReevaluation{}, false, ErrDependencyMissing
	}
	out, err := scanPhysical(s.db.QueryRow(ctx, `SELECT `+physicalColumns+`
		FROM commerce_mission_physical_reevaluations WHERE mission_id=$1 AND evidence_fingerprint=$2`, missionID, fingerprint))
	if errors.Is(err, pgx.ErrNoRows) {
		return PhysicalReevaluation{}, false, nil
	}
	return out, err == nil, err
}

// RecordPhysicalReevaluation stores an UNCHANGED or REFUSED decision once per
// evidence fingerprint.
func (s *Store) RecordPhysicalReevaluation(ctx context.Context, row physicalRow, now time.Time) (PhysicalReevaluation, error) {
	if s == nil || s.db == nil {
		return PhysicalReevaluation{}, ErrDependencyMissing
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return PhysicalReevaluation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,29))`, row.MissionID+":"+row.Fingerprint); err != nil {
		return PhysicalReevaluation{}, err
	}
	existing, err := scanPhysical(tx.QueryRow(ctx, `SELECT `+physicalColumns+`
		FROM commerce_mission_physical_reevaluations WHERE mission_id=$1 AND evidence_fingerprint=$2`, row.MissionID, row.Fingerprint))
	if err == nil {
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return PhysicalReevaluation{}, err
	}
	recorded, err := insertPhysical(ctx, tx, row, now)
	if err != nil {
		return PhysicalReevaluation{}, err
	}
	return recorded, tx.Commit(ctx)
}

// EscalateMission atomically moves the mission to the escalated authority
// under a new reference and records the evidence. The version guard makes it
// single-winner: a concurrent escalation of the same mission conflicts.
func (s *Store) EscalateMission(ctx context.Context, mission OrderMission, row physicalRow, now time.Time) (PhysicalReevaluation, error) {
	if s == nil || s.db == nil {
		return PhysicalReevaluation{}, ErrDependencyMissing
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return PhysicalReevaluation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,29))`, row.MissionID+":"+row.Fingerprint); err != nil {
		return PhysicalReevaluation{}, err
	}
	existing, err := scanPhysical(tx.QueryRow(ctx, `SELECT `+physicalColumns+`
		FROM commerce_mission_physical_reevaluations WHERE mission_id=$1 AND evidence_fingerprint=$2`, row.MissionID, row.Fingerprint))
	if err == nil {
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return PhysicalReevaluation{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE commerce_order_missions
		   SET mission_ref=$3,service=$4,vehicle_class=$5,agreed_fee_minor=$6,status='PENDING',
		       external_ref='',refusal_code='',failure_reason='',version=version+1,updated_at=$7
		 WHERE id=$1 AND version=$2 AND status IN ('PENDING','ACCEPTED')`,
		mission.ID, mission.Version, row.NewRef, string(row.SelectedService), row.SelectedClass,
		row.AuthorityFee, now)
	if err != nil {
		return PhysicalReevaluation{}, err
	}
	if tag.RowsAffected() != 1 {
		return PhysicalReevaluation{}, ErrVersionConflict
	}
	recorded, err := insertPhysical(ctx, tx, row, now)
	if err != nil {
		return PhysicalReevaluation{}, err
	}
	return recorded, tx.Commit(ctx)
}

// FrozenMissionFee reads the checkout-frozen gross fee for a plan mission.
func (s *Store) FrozenMissionFee(ctx context.Context, customerID, planMissionRef string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrDependencyMissing
	}
	var fee int64
	err := s.db.QueryRow(ctx, `SELECT gross_fee_minor FROM commerce_fulfilment_missions WHERE id=$1 AND customer_id=$2`,
		planMissionRef, customerID).Scan(&fee)
	return fee, err
}

// LatestPhysicalEscalation returns the escalation now executing a plan
// mission slot, if any.
func (s *Store) LatestPhysicalEscalation(ctx context.Context, parentOrderID, planMissionRef string) (PhysicalEscalationEvidence, bool, error) {
	if s == nil || s.db == nil {
		return PhysicalEscalationEvidence{}, false, ErrDependencyMissing
	}
	var out PhysicalEscalationEvidence
	var service string
	err := s.db.QueryRow(ctx, `SELECT selected_service,selected_vehicle_class,authority_fee_minor,
		authority_quote_id,authority_quote_reference,confirmed_weight_grams,confirmed_package_count,mission_ref
		FROM commerce_mission_physical_reevaluations
		WHERE parent_order_id=$1 AND plan_mission_ref=$2 AND outcome='ESCALATED'
		ORDER BY created_at DESC,id DESC LIMIT 1`, parentOrderID, planMissionRef).Scan(&service,
		&out.VehicleClass, &out.AuthorityFeeMinor, &out.AuthorityQuoteID, &out.AuthorityQuoteReference,
		&out.WeightGrams, &out.PackageCount, &out.MissionRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, nil
	}
	out.Service = LogisticsService(service)
	return out, err == nil, err
}

// PhysicalEscalationEvidence is what an escalation says about execution.
type PhysicalEscalationEvidence struct {
	Service                                   LogisticsService
	VehicleClass                              string
	AuthorityFeeMinor                         int64
	AuthorityQuoteID, AuthorityQuoteReference string
	WeightGrams, PackageCount                 int64
	MissionRef                                string
}
