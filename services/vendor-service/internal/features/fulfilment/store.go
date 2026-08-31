package fulfilment

// Plan persistence.
//
// Every write is customer-scoped and every read filters on the customer, so a
// plan can never be read or superseded by anyone but its owner. The database's
// own CHECK constraints carry the funding and no-fake-fee invariants; this
// layer must not be the only thing enforcing them.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists fulfilment plans.
type Store struct {
	db *pgxpool.Pool
}

// NewStore builds the store.
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// Save writes one plan and its missions, groups and events atomically.
//
// It supersedes any live plan for the same checkout session first. Planning is
// idempotent from the customer's point of view — asking twice must leave one
// live plan, not two competing ones — and doing it in the same transaction is
// what makes the "one live plan per session" unique index a guarantee rather
// than a race the caller has to avoid.
func (s *Store) Save(
	ctx context.Context,
	plan Plan,
	events []PlanEvent,
) (Plan, error) {
	if s == nil || s.db == nil {
		return Plan{}, ErrDependencyMissing
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Plan{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, `
		UPDATE commerce_fulfilment_plans
		   SET status = 'SUPERSEDED', updated_at = now(), version = version + 1
		 WHERE checkout_session_id = $1
		   AND customer_id = $2
		   AND status IN ('PLANNING', 'PLANNED')`,
		plan.CheckoutSessionID, plan.CustomerID,
	); err != nil {
		return Plan{}, err
	}

	plan.ID = "vfp_" + uuid.NewString()

	if err = tx.QueryRow(ctx, `
		INSERT INTO commerce_fulfilment_plans (
			id, checkout_session_id, customer_id,
			checkout_validation_revision, status, mode, plan_revision,
			currency,
			delivery_gross_minor, delivery_customer_minor,
			delivery_vendor_funded_minor, delivery_platform_funded_minor,
			failure_code, expires_at, planned_at
			, delivery_policy_snapshot, customer_delivery_price_guarantee_minor,
			delivery_price_guarantee_currency
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING created_at, updated_at, version`,
		plan.ID, plan.CheckoutSessionID, plan.CustomerID,
		plan.CheckoutValidationRevision, string(plan.Status), string(plan.Mode),
		plan.PlanRevision, plan.Currency,
		plan.Funding.GrossMinor, plan.Funding.CustomerMinor,
		plan.Funding.VendorFundedMinor, plan.Funding.PlatformFundedMinor,
		plan.FailureCode, plan.ExpiresAt, plan.PlannedAt,
		plan.DeliveryPolicySnapshot, plan.CustomerDeliveryPriceGuaranteeMinor,
		plan.DeliveryPriceGuaranteeCurrency,
	).Scan(&plan.CreatedAt, &plan.UpdatedAt, &plan.Version); err != nil {
		return Plan{}, err
	}

	for index := range plan.Missions {
		if err = insertMission(
			ctx, tx, plan, &plan.Missions[index],
		); err != nil {
			return Plan{}, err
		}
	}

	missionIDs := map[string]string{}

	for _, mission := range plan.Missions {
		for _, groupID := range mission.GroupIDs {
			missionIDs[groupID] = mission.ID
		}
	}

	for index := range plan.Groups {
		group := &plan.Groups[index]

		if group.Serviceable {
			group.MissionID = missionIDs[group.CheckoutGroupID]
		}

		if err = insertGroup(ctx, tx, plan, group); err != nil {
			return Plan{}, err
		}
	}

	// Alternatives commit with the plan. A plan that saved its winner and
	// lost its alternatives would present the selected candidate as though it
	// had been the only one - a complete-looking answer that is not one.
	plan.Candidates = prepareCandidates(plan.Candidates)

	for _, candidate := range plan.Candidates {
		if err = insertCandidate(ctx, tx, plan, candidate); err != nil {
			return Plan{}, err
		}
	}

	for _, event := range events {
		if err = insertEvent(ctx, tx, plan, event); err != nil {
			return Plan{}, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return Plan{}, err
	}

	return plan, nil
}

func insertMission(
	ctx context.Context,
	tx pgx.Tx,
	plan Plan,
	mission *Mission,
) error {
	var expiresAt *time.Time

	if mission.Quote.ExpiresAt != nil {
		expiresAt = mission.Quote.ExpiresAt
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO commerce_fulfilment_missions (
			id, plan_id, customer_id, authority, service, vehicle_class,
			pickup_business_id, pickup_branch_id,
			pickup_latitude, pickup_longitude,
			dropoff_latitude, dropoff_longitude,
			consolidated,
			authority_quote_id, authority_quote_reference,
			authority_quote_revision, authority_quote_expires_at,
			authority_pickup_ref, authority_dropoff_ref,
			distance_km, estimated_minutes, currency,
			gross_fee_minor, customer_fee_minor,
			vendor_funded_minor, platform_funded_minor,
			base_minor, distance_minor, surcharge_minor,
			fee_beneficiary, total_weight_grams, package_count,
			delivery_mode, fallback_trigger, policy_version,
			customer_price_guarantee_minor, custody_state
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,
		          $18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,
		          $33,$34,$35,$36,$37)`,
		mission.ID, plan.ID, plan.CustomerID,
		string(mission.Authority), string(mission.Service),
		string(mission.VehicleClass),
		mission.PickupBusinessID, mission.PickupBranchID,
		mission.PickupLatitude, mission.PickupLongitude,
		mission.DropoffLatitude, mission.DropoffLongitude,
		mission.Consolidated,
		mission.Quote.QuoteID, mission.Quote.QuoteReference,
		mission.Quote.Revision, expiresAt,
		mission.Quote.PickupRef, mission.Quote.DropoffRef,
		mission.DistanceKm, mission.EstimatedMinutes, mission.Currency,
		mission.Funding.GrossMinor, mission.Funding.CustomerMinor,
		mission.Funding.VendorFundedMinor, mission.Funding.PlatformFundedMinor,
		mission.Breakdown.BaseMinor, mission.Breakdown.DistanceMinor,
		mission.Breakdown.SurchargeMinor,
		string(mission.FeeBeneficiary),
		mission.TotalWeightGrams, mission.PackageCount,
		stringOrDefault(mission.DeliveryMode, "PLATFORM"), mission.FallbackTrigger,
		mission.PolicyVersion, mission.CustomerPriceGuaranteeMinor,
		stringOrDefault(mission.CustodyState, "NOT_TRANSFERRED"),
	)

	return err
}

func insertGroup(
	ctx context.Context,
	tx pgx.Tx,
	plan Plan,
	group *PlanGroup,
) error {
	group.ID = "vfg_" + uuid.NewString()

	var missionID *string

	if group.MissionID != "" {
		missionID = &group.MissionID
	}

	capabilities := make([]string, 0, len(group.Requirement.Capabilities))

	for _, capability := range group.Requirement.Capabilities {
		capabilities = append(capabilities, string(capability))
	}

	weightEvidence := string(group.Requirement.WeightEvidence)
	if weightEvidence == "" {
		weightEvidence = string(EvidenceAbsent)
	}

	urgency := string(group.Requirement.Urgency)
	if urgency == "" {
		urgency = string(UrgencyStandard)
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO commerce_fulfilment_plan_groups (
			id, plan_id, customer_id, checkout_group_id,
			business_id, branch_id,
			serviceable, reason_code, authority, mission_id, vehicle_class,
			package_count, total_weight_grams, total_volume_ml,
			longest_dimension_mm, weight_evidence, urgency, capabilities,
			gross_fee_minor, customer_fee_minor,
			vendor_funded_minor, platform_funded_minor,
			delivery_mode, fallback_trigger, customer_price_guarantee_minor
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,
		          $18,$19,$20,$21,$22,$23,$24,$25)`,
		group.ID, plan.ID, plan.CustomerID, group.CheckoutGroupID,
		group.BusinessID, group.BranchID,
		group.Serviceable, group.ReasonCode, string(group.Authority),
		missionID, string(group.VehicleClass),
		group.Requirement.PackageCount, group.Requirement.TotalWeightGrams,
		group.Requirement.TotalVolumeMl, group.Requirement.LongestDimensionMm,
		weightEvidence, urgency, capabilities,
		group.Funding.GrossMinor, group.Funding.CustomerMinor,
		group.Funding.VendorFundedMinor, group.Funding.PlatformFundedMinor,
		stringOrDefault(group.DeliveryMode, "PLATFORM"), group.FallbackTrigger,
		group.CustomerPriceGuaranteeMinor,
	)

	return err
}

func insertEvent(
	ctx context.Context,
	tx pgx.Tx,
	plan Plan,
	event PlanEvent,
) error {
	var groupID, missionID *string

	if event.GroupID != "" {
		groupID = &event.GroupID
	}

	if event.MissionID != "" {
		missionID = &event.MissionID
	}

	detail := event.Detail

	// Event detail is operator evidence, not a payload. Bounding it keeps a
	// verbose authority error from becoming an unbounded row.
	if len(detail) > 500 {
		detail = detail[:500]
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO commerce_fulfilment_plan_events (
			id, plan_id, customer_id, action, group_id, mission_id,
			detail, plan_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		"vfe_"+uuid.NewString(), plan.ID, plan.CustomerID,
		event.Action, groupID, missionID, detail, plan.Version,
	)

	return err
}

// Get reads one plan with its missions and groups.
//
// The customer predicate is not optional: a plan id alone must never be enough
// to read someone else's delivery addresses and fees.
func (s *Store) Get(
	ctx context.Context,
	customerID, planID string,
) (Plan, error) {
	if s == nil || s.db == nil {
		return Plan{}, ErrDependencyMissing
	}

	var plan Plan

	err := s.db.QueryRow(ctx, `
		SELECT id, checkout_session_id, customer_id,
		       checkout_validation_revision, status, mode, plan_revision,
		       currency,
		       delivery_gross_minor, delivery_customer_minor,
		       delivery_vendor_funded_minor, delivery_platform_funded_minor,
		       failure_code, expires_at, version,
		       delivery_policy_snapshot, customer_delivery_price_guarantee_minor,
		       delivery_price_guarantee_currency,
		       created_at, updated_at, planned_at
		  FROM commerce_fulfilment_plans
		 WHERE id = $1 AND customer_id = $2`,
		strings.TrimSpace(planID), strings.TrimSpace(customerID),
	).Scan(
		&plan.ID, &plan.CheckoutSessionID, &plan.CustomerID,
		&plan.CheckoutValidationRevision, &plan.Status, &plan.Mode,
		&plan.PlanRevision, &plan.Currency,
		&plan.Funding.GrossMinor, &plan.Funding.CustomerMinor,
		&plan.Funding.VendorFundedMinor, &plan.Funding.PlatformFundedMinor,
		&plan.FailureCode, &plan.ExpiresAt, &plan.Version,
		&plan.DeliveryPolicySnapshot, &plan.CustomerDeliveryPriceGuaranteeMinor,
		&plan.DeliveryPriceGuaranteeCurrency,
		&plan.CreatedAt, &plan.UpdatedAt, &plan.PlannedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrPlanNotFound
	}
	if err != nil {
		return Plan{}, err
	}

	if plan.Missions, err = s.missions(ctx, plan.ID); err != nil {
		return Plan{}, err
	}

	if plan.Groups, err = s.groups(ctx, plan.ID); err != nil {
		return Plan{}, err
	}

	if plan.Candidates, err = loadCandidates(
		ctx, s.db, plan.CustomerID, plan.ID,
	); err != nil {
		return Plan{}, err
	}

	return plan, nil
}

// GetCurrentForCheckout reads the current plan for a checkout session, if any.
//
// "Current" is the most recent plan that has not been superseded, which is
// deliberately wider than "live". An UNSERVICEABLE plan is a terminal answer
// the customer was just given - which vendors cannot be delivered and why -
// and it must stay readable. Returning only PLANNING/PLANNED would mean a
// customer told "these two vendors cannot deliver" gets a 404 the moment they
// reload the page.
//
// The one-live-plan-per-session uniqueness still governs PLANNING and PLANNED
// only; terminal plans accumulate as evidence and never compete.
func (s *Store) GetCurrentForCheckout(
	ctx context.Context,
	customerID, checkoutSessionID string,
) (Plan, error) {
	if s == nil || s.db == nil {
		return Plan{}, ErrDependencyMissing
	}

	var planID string

	err := s.db.QueryRow(ctx, `
		SELECT id FROM commerce_fulfilment_plans
		 WHERE checkout_session_id = $1 AND customer_id = $2
		   AND status <> 'SUPERSEDED'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		strings.TrimSpace(checkoutSessionID), strings.TrimSpace(customerID),
	).Scan(&planID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrPlanNotFound
	}
	if err != nil {
		return Plan{}, err
	}

	return s.Get(ctx, customerID, planID)
}

func (s *Store) missions(
	ctx context.Context,
	planID string,
) ([]Mission, error) {
	rows, err := s.db.Query(ctx, `
		SELECT m.id, m.authority, m.service, m.vehicle_class,
		       m.pickup_business_id, m.pickup_branch_id,
		       m.pickup_latitude, m.pickup_longitude,
		       m.dropoff_latitude, m.dropoff_longitude,
		       m.consolidated,
		       m.authority_quote_id, m.authority_quote_reference,
		       m.authority_quote_revision, m.authority_quote_expires_at,
		       m.authority_pickup_ref, m.authority_dropoff_ref,
		       m.distance_km, m.estimated_minutes, m.currency,
		       m.gross_fee_minor, m.customer_fee_minor,
		       m.vendor_funded_minor, m.platform_funded_minor,
		       m.base_minor, m.distance_minor, m.surcharge_minor,
		       m.fee_beneficiary, m.total_weight_grams, m.package_count,
		       m.delivery_mode, m.fallback_trigger, m.policy_version,
		       m.customer_price_guarantee_minor, m.custody_state,
		       COALESCE(
		           ARRAY(
		               SELECT g.checkout_group_id
		                 FROM commerce_fulfilment_plan_groups g
		                WHERE g.mission_id = m.id
		                ORDER BY g.checkout_group_id
		           ),
		           ARRAY[]::text[]
		       )
		  FROM commerce_fulfilment_missions m
		 WHERE m.plan_id = $1
		 ORDER BY m.id`,
		planID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var missions []Mission

	for rows.Next() {
		var (
			mission   Mission
			expiresAt *time.Time
		)

		if err = rows.Scan(
			&mission.ID, &mission.Authority, &mission.Service,
			&mission.VehicleClass,
			&mission.PickupBusinessID, &mission.PickupBranchID,
			&mission.PickupLatitude, &mission.PickupLongitude,
			&mission.DropoffLatitude, &mission.DropoffLongitude,
			&mission.Consolidated,
			&mission.Quote.QuoteID, &mission.Quote.QuoteReference,
			&mission.Quote.Revision, &expiresAt,
			&mission.Quote.PickupRef, &mission.Quote.DropoffRef,
			&mission.DistanceKm, &mission.EstimatedMinutes, &mission.Currency,
			&mission.Funding.GrossMinor, &mission.Funding.CustomerMinor,
			&mission.Funding.VendorFundedMinor,
			&mission.Funding.PlatformFundedMinor,
			&mission.Breakdown.BaseMinor, &mission.Breakdown.DistanceMinor,
			&mission.Breakdown.SurchargeMinor,
			&mission.FeeBeneficiary,
			&mission.TotalWeightGrams, &mission.PackageCount,
			&mission.DeliveryMode, &mission.FallbackTrigger, &mission.PolicyVersion,
			&mission.CustomerPriceGuaranteeMinor, &mission.CustodyState,
			&mission.GroupIDs,
		); err != nil {
			return nil, err
		}

		mission.Quote.ExpiresAt = expiresAt

		missions = append(missions, mission)
	}

	return missions, rows.Err()
}

func (s *Store) groups(
	ctx context.Context,
	planID string,
) ([]PlanGroup, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, checkout_group_id, business_id, branch_id,
		       serviceable, reason_code, authority,
		       COALESCE(mission_id, ''), vehicle_class,
		       package_count, total_weight_grams, total_volume_ml,
		       longest_dimension_mm, weight_evidence, urgency, capabilities,
		       gross_fee_minor, customer_fee_minor,
		       vendor_funded_minor, platform_funded_minor,
		       delivery_mode, fallback_trigger, customer_price_guarantee_minor
		  FROM commerce_fulfilment_plan_groups
		 WHERE plan_id = $1
		 ORDER BY checkout_group_id`,
		planID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var groups []PlanGroup

	for rows.Next() {
		var (
			group        PlanGroup
			capabilities []string
		)

		if err = rows.Scan(
			&group.ID, &group.CheckoutGroupID,
			&group.BusinessID, &group.BranchID,
			&group.Serviceable, &group.ReasonCode, &group.Authority,
			&group.MissionID, &group.VehicleClass,
			&group.Requirement.PackageCount,
			&group.Requirement.TotalWeightGrams,
			&group.Requirement.TotalVolumeMl,
			&group.Requirement.LongestDimensionMm,
			&group.Requirement.WeightEvidence,
			&group.Requirement.Urgency,
			&capabilities,
			&group.Funding.GrossMinor, &group.Funding.CustomerMinor,
			&group.Funding.VendorFundedMinor,
			&group.Funding.PlatformFundedMinor,
			&group.DeliveryMode, &group.FallbackTrigger,
			&group.CustomerPriceGuaranteeMinor,
		); err != nil {
			return nil, err
		}

		group.Requirement.BusinessID = group.BusinessID
		group.Requirement.BranchID = group.BranchID

		for _, capability := range capabilities {
			group.Requirement.Capabilities = append(
				group.Requirement.Capabilities,
				HandlingCapability(capability),
			)
		}

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// ExpireDue marks lapsed plans expired.
//
// A plan past its expiry is not merely stale: its quotes have lapsed, so the
// fees on it are no longer backed by any authority. Sweeping them keeps a
// later section from finding a plan that looks live and is not.
func (s *Store) ExpireDue(
	ctx context.Context,
	now time.Time,
	limit int,
) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrDependencyMissing
	}

	if limit <= 0 {
		limit = 200
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE commerce_fulfilment_plans
		   SET status = 'EXPIRED', updated_at = now(), version = version + 1
		 WHERE id IN (
			SELECT id FROM commerce_fulfilment_plans
			 WHERE status IN ('PLANNING', 'PLANNED')
			   AND expires_at <= $1
			 ORDER BY expires_at
			 LIMIT $2
		 )`,
		now, limit,
	)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}
