package main

// Section 17C plan persistence tests.
//
// These run against real Postgres because the guarantees under test are
// CHECK constraints. A Go-level fake would prove that the planner happens to
// build balanced plans today; only the database proves that an unbalanced or
// unquoted one cannot be stored at all.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
)

func s17cPlan(env *s17cEnv, status fulfilment.PlanStatus) fulfilment.Plan {
	return fulfilment.Plan{
		CustomerID:                 "cust_" + env.suffix,
		CheckoutSessionID:          "cs_" + env.suffix,
		CheckoutValidationRevision: 1,
		Status:                     status,
		Mode:                       fulfilment.ModeSingle,
		PlanRevision:               1,
		Currency:                   "NGN",
		Version:                    1,
		ExpiresAt:                  time.Now().Add(10 * time.Minute).UTC(),
	}
}

// seedCheckoutSession creates the Section 17B rows a plan's foreign key needs.
func (e *s17cEnv) seedCheckoutSession(ctx context.Context) (string, string) {
	e.t.Helper()

	customerID := "cust_" + e.suffix
	cartID := "cart_" + e.suffix
	sessionID := "cs_" + e.suffix

	e.exec(ctx,
		`INSERT INTO commerce_carts(id,customer_id,domain,status,expires_at)
		 VALUES($1,$2,'SHOP','CHECKOUT_IN_PROGRESS',now()+interval '1 hour')`,
		cartID, customerID)

	e.exec(ctx,
		`INSERT INTO commerce_checkout_sessions(
			id,customer_id,cart_id,cart_version,domain,status,
			validation_revision,currency,reservation_ttl_seconds,expires_at,
			validated_at)
		 VALUES($1,$2,$3,1,'SHOP','VALIDATED',1,'NGN',600,now()+interval '10 minutes',now())`,
		sessionID, customerID, cartID)

	return customerID, sessionID
}

func TestSection17CPlanStore_RoundTrip(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	customerID, sessionID := env.seedCheckoutSession(ctx)

	store := fulfilment.NewStore(env.pool)

	expiry := time.Now().Add(9 * time.Minute).UTC()

	plan := s17cPlan(env, fulfilment.StatusPlanned)
	plan.CustomerID = customerID
	plan.CheckoutSessionID = sessionID

	planned := time.Now().UTC()
	plan.PlannedAt = &planned

	missionID := "pfm_" + uuid.NewString()

	plan.Missions = []fulfilment.Mission{{
		ID:               missionID,
		Authority:        fulfilment.AuthorityPlatform,
		Service:          fulfilment.ServiceDispatch,
		VehicleClass:     fulfilment.VehicleMotorcycle,
		PickupBusinessID: env.businessA,
		PickupBranchID:   env.branchA,
		PickupLatitude:   6.4550,
		PickupLongitude:  3.4100,
		DropoffLatitude:  6.4300,
		DropoffLongitude: 3.4500,
		Quote: fulfilment.QuoteEvidence{
			QuoteID:        "dq_1",
			QuoteReference: "vfq_1",
			Revision:       1,
			PickupRef:      "dq_1:pickup",
			DropoffRef:     "dq_1:dropoff",
			ExpiresAt:      &expiry,
		},
		DistanceKm:       4.9,
		EstimatedMinutes: 26,
		Currency:         "NGN",
		Funding: fulfilment.Funding{
			GrossMinor: 150000, CustomerMinor: 150000,
		},
		FeeBeneficiary:   fulfilment.BeneficiaryPlatformProvider,
		TotalWeightGrams: 3000,
		PackageCount:     2,
		GroupIDs:         []string{"g1"},
	}}

	plan.Funding = plan.Missions[0].Funding

	plan.Groups = []fulfilment.PlanGroup{
		{
			CheckoutGroupID: "g1",
			BusinessID:      env.businessA,
			BranchID:        env.branchA,
			Serviceable:     true,
			ReasonCode:      fulfilment.ReasonServiceable,
			Authority:       fulfilment.AuthorityPlatform,
			VehicleClass:    fulfilment.VehicleMotorcycle,
			Requirement: fulfilment.Requirement{
				PackageCount: 2, TotalWeightGrams: 3000,
				WeightEvidence: fulfilment.EvidenceComplete,
				Urgency:        fulfilment.UrgencyStandard,
				Capabilities: []fulfilment.HandlingCapability{
					fulfilment.CapabilityInsulated,
				},
			},
			Funding: plan.Missions[0].Funding,
		},
		{
			// The unserviceable group is stored too. Not silently dropped.
			CheckoutGroupID: "g2",
			BusinessID:      env.businessA,
			BranchID:        env.branchA,
			Serviceable:     false,
			ReasonCode:      "NO_PROVIDER_AVAILABLE",
			Authority:       fulfilment.AuthorityNone,
		},
	}

	saved, err := store.Save(ctx, plan, []fulfilment.PlanEvent{
		{Action: fulfilment.EventPlanningStarted},
		{Action: fulfilment.EventQuoteReceived, GroupID: "g1", Detail: "dq_1"},
		{Action: fulfilment.EventPlanned},
	})
	if err != nil {
		t.Fatal(err)
	}

	if saved.ID == "" || saved.Version != 1 {
		t.Fatalf("unexpected saved plan: %+v", saved)
	}

	read, err := store.Get(ctx, customerID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(read.Missions) != 1 || len(read.Groups) != 2 {
		t.Fatalf("read back %d missions and %d groups",
			len(read.Missions), len(read.Groups))
	}

	mission := read.Missions[0]

	if mission.Quote.QuoteID != "dq_1" ||
		mission.Funding.GrossMinor != 150000 {
		t.Fatalf("mission not round-tripped: %+v", mission)
	}

	// The mission must name the groups it covers, read back from the join.
	if len(mission.GroupIDs) != 1 || mission.GroupIDs[0] != "g1" {
		t.Fatalf("mission group ids = %v", mission.GroupIDs)
	}

	if mission.Quote.ExpiresAt == nil {
		t.Fatal("quote expiry was not persisted")
	}

	// The serviceable group must be linked to its mission.
	var linked, refused fulfilment.PlanGroup

	for _, group := range read.Groups {
		if group.CheckoutGroupID == "g1" {
			linked = group
		} else {
			refused = group
		}
	}

	if linked.MissionID != missionID {
		t.Fatalf("group not linked to mission: %+v", linked)
	}

	if len(linked.Requirement.Capabilities) != 1 {
		t.Fatalf("capabilities not round-tripped: %+v", linked.Requirement)
	}

	if refused.Serviceable || refused.MissionID != "" ||
		refused.Funding.GrossMinor != 0 {
		t.Fatalf("unserviceable group is wrong: %+v", refused)
	}
}

// A plan id alone must never be enough to read someone else's plan.
func TestSection17CPlanStore_CustomerIsolation(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	customerID, sessionID := env.seedCheckoutSession(ctx)

	store := fulfilment.NewStore(env.pool)

	plan := s17cPlan(env, fulfilment.StatusUnserviceable)
	plan.CustomerID = customerID
	plan.CheckoutSessionID = sessionID
	plan.FailureCode = fulfilment.FailureGroupUnserviceable
	plan.Groups = []fulfilment.PlanGroup{{
		CheckoutGroupID: "g1",
		BusinessID:      env.businessA,
		BranchID:        env.branchA,
		ReasonCode:      "NO_PROVIDER_AVAILABLE",
		Authority:       fulfilment.AuthorityNone,
	}}

	saved, err := store.Save(ctx, plan, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = store.Get(ctx, "someone_else", saved.ID); err == nil {
		t.Fatal("another customer read the plan")
	}
}

// Planning twice must leave one LIVE plan, not two competing ones.
//
// Only PLANNING and PLANNED are live. A terminal plan — UNSERVICEABLE, FAILED,
// EXPIRED — is evidence of a refusal that already happened and is deliberately
// left readable rather than being rewritten as superseded, because an operator
// asked "why did this checkout say no" needs to find it.
func TestSection17CPlanStore_SecondPlanSupersedesTheLiveOne(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	customerID, sessionID := env.seedCheckoutSession(ctx)

	store := fulfilment.NewStore(env.pool)

	build := func() fulfilment.Plan {
		plan := s17cPlan(env, fulfilment.StatusPlanned)
		plan.CustomerID = customerID
		plan.CheckoutSessionID = sessionID

		planned := time.Now().UTC()
		plan.PlannedAt = &planned

		return plan
	}

	first, err := store.Save(ctx, build(), nil)
	if err != nil {
		t.Fatal(err)
	}

	second, err := store.Save(ctx, build(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if second.ID == first.ID {
		t.Fatal("the second plan reused the first plan id")
	}

	reread, err := store.Get(ctx, customerID, first.ID)
	if err != nil {
		t.Fatal(err)
	}

	if reread.Status != fulfilment.StatusSuperseded {
		t.Fatalf("first plan status = %q, want SUPERSEDED", reread.Status)
	}

	// The unique index only permits one live plan per session, so a third
	// save must still succeed rather than collide.
	if _, err = store.Save(ctx, build(), nil); err != nil {
		t.Fatalf("a third planning attempt collided: %v", err)
	}

	var live int

	if err = env.pool.QueryRow(ctx, `
		SELECT count(*) FROM commerce_fulfilment_plans
		 WHERE checkout_session_id = $1
		   AND status IN ('PLANNING','PLANNED')`,
		sessionID,
	).Scan(&live); err != nil {
		t.Fatal(err)
	}

	if live != 1 {
		t.Fatalf("live plans = %d, want exactly 1", live)
	}
}

// The invariants this section turns on are CHECK constraints, not conventions.
// These assertions exist so a future refactor that bypasses the planner still
// cannot store a fee nobody stands behind.
func TestSection17CPlanConstraints(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	customerID, sessionID := env.seedCheckoutSession(ctx)

	planID := "vfp_" + uuid.NewString()

	if _, err := env.pool.Exec(ctx, `
		INSERT INTO commerce_fulfilment_plans(
			id,checkout_session_id,customer_id,checkout_validation_revision,
			status,mode,plan_revision,currency,expires_at,planned_at)
		VALUES($1,$2,$3,1,'PLANNED','SINGLE',1,'NGN',now()+interval '10 minutes',now())`,
		planID, sessionID, customerID,
	); err != nil {
		t.Fatal(err)
	}

	insertMission := func(sql string, args ...any) error {
		_, err := env.pool.Exec(ctx, sql, args...)

		return err
	}

	// 1. A PLATFORM_LOGISTICS mission with a fee and no authority quote id.
	err := insertMission(`
		INSERT INTO commerce_fulfilment_missions(
			id,plan_id,customer_id,authority,service,vehicle_class,
			pickup_business_id,pickup_branch_id,pickup_latitude,pickup_longitude,
			dropoff_latitude,dropoff_longitude,currency,
			gross_fee_minor,customer_fee_minor,fee_beneficiary)
		VALUES($1,$2,$3,'PLATFORM_LOGISTICS','DISPATCH','MOTORCYCLE',
		       $4,$5,6.455,3.41,6.43,3.45,'NGN',150000,150000,'PLATFORM_PROVIDER')`,
		"pfm_"+uuid.NewString(), planID, customerID, env.businessA, env.branchA)

	if err == nil {
		t.Fatal("a platform mission was stored with a fee and no authority quote")
	}

	if !strings.Contains(err.Error(), "platform_quote_chk") {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2. Funding that does not add up.
	expiry := time.Now().Add(5 * time.Minute)

	err = insertMission(`
		INSERT INTO commerce_fulfilment_missions(
			id,plan_id,customer_id,authority,service,vehicle_class,
			pickup_business_id,pickup_branch_id,pickup_latitude,pickup_longitude,
			dropoff_latitude,dropoff_longitude,currency,
			authority_quote_id,authority_quote_expires_at,
			gross_fee_minor,customer_fee_minor,vendor_funded_minor,
			fee_beneficiary)
		VALUES($1,$2,$3,'PLATFORM_LOGISTICS','DISPATCH','MOTORCYCLE',
		       $4,$5,6.455,3.41,6.43,3.45,'NGN','dq_x',$6,150000,100000,10000,
		       'PLATFORM_PROVIDER')`,
		"pfm_"+uuid.NewString(), planID, customerID,
		env.businessA, env.branchA, expiry)

	if err == nil {
		t.Fatal("an unbalanced funding split was stored")
	}

	if !strings.Contains(err.Error(), "funding_chk") {
		t.Fatalf("unexpected error: %v", err)
	}

	// 3. Vendor-owned delivery claiming a platform authority quote.
	err = insertMission(`
		INSERT INTO commerce_fulfilment_missions(
			id,plan_id,customer_id,authority,service,
			pickup_business_id,pickup_branch_id,pickup_latitude,pickup_longitude,
			dropoff_latitude,dropoff_longitude,currency,
			authority_quote_id,gross_fee_minor,customer_fee_minor,
			fee_beneficiary)
		VALUES($1,$2,$3,'VENDOR_OWN_DELIVERY','VENDOR_OWN_DELIVERY',
		       $4,$5,6.455,3.41,6.43,3.45,'NGN','dq_stolen',50000,50000,'VENDOR')`,
		"vfm_"+uuid.NewString(), planID, customerID, env.businessA, env.branchA)

	if err == nil {
		t.Fatal("vendor own delivery claimed a platform authority quote")
	}

	// 4. A platform mission paying the vendor.
	err = insertMission(`
		INSERT INTO commerce_fulfilment_missions(
			id,plan_id,customer_id,authority,service,vehicle_class,
			pickup_business_id,pickup_branch_id,pickup_latitude,pickup_longitude,
			dropoff_latitude,dropoff_longitude,currency,
			authority_quote_id,authority_quote_expires_at,
			gross_fee_minor,customer_fee_minor,fee_beneficiary)
		VALUES($1,$2,$3,'PLATFORM_LOGISTICS','DISPATCH','MOTORCYCLE',
		       $4,$5,6.455,3.41,6.43,3.45,'NGN','dq_y',$6,150000,150000,'VENDOR')`,
		"pfm_"+uuid.NewString(), planID, customerID,
		env.businessA, env.branchA, expiry)

	if err == nil {
		t.Fatal("a platform delivery fee was assigned to the vendor")
	}

	// 5. An unserviceable group carrying a charge.
	_, err = env.pool.Exec(ctx, `
		INSERT INTO commerce_fulfilment_plan_groups(
			id,plan_id,customer_id,checkout_group_id,business_id,branch_id,
			serviceable,reason_code,authority,gross_fee_minor,customer_fee_minor)
		VALUES($1,$2,$3,'g9',$4,$5,false,'NO_PROVIDER_AVAILABLE','NONE',5000,5000)`,
		"vfg_"+uuid.NewString(), planID, customerID, env.businessA, env.branchA)

	if err == nil {
		t.Fatal("an unserviceable group was stored with a charge")
	}

	// 6. A serviceable group naming no mission.
	_, err = env.pool.Exec(ctx, `
		INSERT INTO commerce_fulfilment_plan_groups(
			id,plan_id,customer_id,checkout_group_id,business_id,branch_id,
			serviceable,reason_code,authority)
		VALUES($1,$2,$3,'g10',$4,$5,true,'SERVICEABLE','PLATFORM_LOGISTICS')`,
		"vfg_"+uuid.NewString(), planID, customerID, env.businessA, env.branchA)

	if err == nil {
		t.Fatal("a serviceable group was stored with no mission")
	}
}

func TestSection17CPlanStore_ExpirySweep(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	customerID, sessionID := env.seedCheckoutSession(ctx)

	store := fulfilment.NewStore(env.pool)

	plan := s17cPlan(env, fulfilment.StatusPlanned)
	plan.CustomerID = customerID
	plan.CheckoutSessionID = sessionID
	plan.ExpiresAt = time.Now().Add(-time.Minute).UTC()

	planned := time.Now().UTC()
	plan.PlannedAt = &planned

	saved, err := store.Save(ctx, plan, nil)
	if err != nil {
		t.Fatal(err)
	}

	swept, err := store.ExpireDue(ctx, time.Now().UTC(), 100)
	if err != nil {
		t.Fatal(err)
	}

	if swept < 1 {
		t.Fatalf("swept %d plans, want at least 1", swept)
	}

	read, err := store.Get(ctx, customerID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}

	if read.Status != fulfilment.StatusExpired {
		t.Fatalf("status = %q, want EXPIRED", read.Status)
	}
}
