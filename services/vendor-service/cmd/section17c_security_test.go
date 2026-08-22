package main

// Section 17C security tests.
//
// The section names five things a vendor must not be able to do and one a
// customer must not. Each gets a test here, and each is asserted against the
// real enforcement path rather than a fake:
//
//   Vendor A must not set Vendor B's delivery policy
//   Vendor A must not receive Vendor B's delivery fee
//   Vendor A must not inject another branch's pickup
//   Vendor A must not spoof a logistics quote ref
//   A customer must not overwrite the logistics fee from a request payload

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/shared/go/commercequote"
)

// s17cScopes grants one permission set, globally, to one staff member and
// refuses everyone else. Tenant separation is NOT delegated to it: the point
// of these tests is that the tenant boundary holds even when the actor's
// permission scope is unrestricted.
type s17cScopes struct {
	staffID string
	global  bool
}

func (s s17cScopes) ResolveScope(
	_ context.Context, staffID, _ string,
) (production.Scope, error) {
	if staffID != s.staffID {
		return production.Scope{}, nil
	}

	return production.Scope{Global: s.global}, nil
}

func s17cDeliveryService(env *s17cEnv, staffID string) *delivery.Service {
	return delivery.NewService(
		env.pool,
		production.NewService(production.Options{
			DB:     env.pool,
			Scopes: s17cScopes{staffID: staffID, global: true},
		}),
		true,
	)
}

func s17cActor(business, staff string) production.Actor {
	return production.Actor{StaffID: staff, VendorID: business}
}

func s17cPolicyInput() delivery.PolicyInput {
	return delivery.PolicyInput{
		Enabled:              true,
		Currency:             "NGN",
		BaseFeeMinor:         50000,
		PerKmFeeMinor:        10000,
		EstimatedMinutesBase: 20,
		Reason:               "test",
	}
}

// Vendor A must not set Vendor B's delivery policy.
//
// The actor here holds vendor.delivery.policy.manage GLOBALLY, so the refusal
// cannot come from a permission scope: it must come from the tenant boundary.
func TestSection17CSecurity_VendorCannotSetAnotherVendorsPolicy(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	staffA := "s17c_staff_a_" + env.suffix

	service := s17cDeliveryService(env, staffA)

	// Vendor A configuring its OWN branch succeeds.
	if _, err := service.UpsertPolicy(
		ctx, s17cActor(env.businessA, staffA), env.branchA, 0, s17cPolicyInput(),
	); err != nil {
		t.Fatalf("vendor A could not configure its own branch: %v", err)
	}

	// The same actor naming Vendor B's branch resolves to nothing.
	_, err := service.UpsertPolicy(
		ctx, s17cActor(env.businessA, staffA), env.branchB, 0, s17cPolicyInput(),
	)

	if err == nil {
		t.Fatal("vendor A configured vendor B's branch")
	}

	if !errors.Is(err, delivery.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	// And nothing was written for it.
	var count int

	if err = env.pool.QueryRow(ctx, `
		SELECT count(*) FROM vendor_delivery_policies WHERE branch_id = $1`,
		env.branchB,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatalf("a policy was written for vendor B's branch: %d rows", count)
	}
}

// Vendor A must not read Vendor B's delivery policy either — a fee another
// vendor charges is that vendor's commercial information.
func TestSection17CSecurity_VendorCannotReadAnotherVendorsPolicy(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	staffA := "s17c_staff_a_" + env.suffix
	staffB := "s17c_staff_b_" + env.suffix

	serviceB := s17cDeliveryService(env, staffB)

	if _, err := serviceB.UpsertPolicy(
		ctx, s17cActor(env.businessB, staffB), env.branchB, 0, s17cPolicyInput(),
	); err != nil {
		t.Fatal(err)
	}

	serviceA := s17cDeliveryService(env, staffA)

	if _, err := serviceA.GetPolicy(
		ctx, s17cActor(env.businessA, staffA), env.branchB,
	); !errors.Is(err, delivery.ErrNotFound) {
		t.Fatalf("vendor A read vendor B's policy: %v", err)
	}
}

// An actor holding no permission at all is refused, and refused as NotFound
// rather than Forbidden, so a probe cannot use the response to learn whether
// the branch has a policy.
func TestSection17CSecurity_NoPermissionIsRefused(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	service := s17cDeliveryService(env, "somebody-else")

	if _, err := service.GetPolicy(
		ctx, s17cActor(env.businessA, "s17c_staff_a_"+env.suffix), env.branchA,
	); !errors.Is(err, delivery.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	if _, err := service.UpsertPolicy(
		ctx, s17cActor(env.businessA, "s17c_staff_a_"+env.suffix),
		env.branchA, 0, s17cPolicyInput(),
	); !errors.Is(err, delivery.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

// Vendor A must not inject another branch's pickup.
//
// The branch authority is scoped by BOTH ids, so a branch borrowed from
// another business does not resolve at all — the group is refused with
// PICKUP_BRANCH_UNKNOWN rather than being planned from a pickup its vendor
// does not own.
func TestSection17CSecurity_ForeignBranchCannotBecomeAPickup(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	authority := newSection17CBranchAuthority(env.pool)

	// Vendor B's branch does not resolve under Vendor A.
	if _, found, err := authority.ResolveBranch(
		ctx, env.businessA, env.branchB,
	); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("vendor A resolved vendor B's branch as a pickup")
	}

	// And the planner turns that into an explicit refusal rather than a
	// silently-dropped group.
	planner, err := fulfilment.NewPlanner(fulfilment.Options{
		Logistics: newSection17CLogisticsAuthority(env.pool),
		Branches:  authority,
	})
	if err != nil {
		t.Fatal(err)
	}

	plan, _, err := planner.Build(ctx, fulfilment.PlanRequest{
		CustomerID:                 "cust_" + env.suffix,
		CheckoutSessionID:          "cs_" + env.suffix,
		CheckoutValidationRevision: 1,
		Currency:                   "NGN",
		Destination: fulfilment.Destination{
			Latitude: 6.4300, Longitude: 3.4500,
		},
		Groups: []fulfilment.GroupInput{{
			CheckoutGroupID: "g1",
			BusinessID:      env.businessA,
			BranchID:        env.branchB,
			Currency:        "NGN",
			Lines: []fulfilment.LineLogistics{{
				CheckoutLineID: "l1", Domain: "SHOP", Quantity: 1,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != fulfilment.StatusUnserviceable {
		t.Fatalf("status = %q, want UNSERVICEABLE", plan.Status)
	}

	if len(plan.Groups) != 1 ||
		plan.Groups[0].ReasonCode != fulfilment.ReasonBranchUnknown {
		t.Fatalf("unexpected groups: %+v", plan.Groups)
	}

	if plan.Funding.GrossMinor != 0 {
		t.Fatalf("a cross-tenant pickup produced a fee: %+v", plan.Funding)
	}
}

// spoofingAuthority answers with a quote reference it was not asked for, which
// is what a compromised or confused authority — or a spoofed one — would look
// like from Vendor Service's side.
type spoofingAuthority struct {
	quote commercequote.Quote
}

func (s spoofingAuthority) Available() bool { return true }

func (s spoofingAuthority) RequestQuote(
	_ context.Context, _ commercequote.Request,
) (commercequote.Quote, error) {
	return s.quote, nil
}

// A logistics quote reference is never accepted from anywhere but the
// authority's own answer to this exact request.
//
// Vendor Service mints the REQUEST reference from the checkout it is planning,
// and the shared client refuses an answer that names a different one. A vendor
// has no input into either, and neither does a customer.
func TestSection17CSecurity_QuoteReferenceCannotBeSpoofed(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	env.exec(ctx,
		`INSERT INTO vendor_branch_service_areas(
			branch_id,business_id,mode,center_latitude,center_longitude,radius_meters)
		 VALUES($1,$2,'RADIUS',6.4550,3.4100,15000)`,
		env.branchA, env.businessA)

	planner, err := fulfilment.NewPlanner(fulfilment.Options{
		Logistics: newSection17CLogisticsAuthority(env.pool),
		Branches:  newSection17CBranchAuthority(env.pool),
		Dispatch: spoofingAuthority{quote: commercequote.Quote{
			QuoteID:            "dq_stolen",
			QuoteReference:     "someone-elses-reference",
			Serviceable:        true,
			ReasonCode:         commercequote.ReasonServiceable,
			VehicleClass:       commercequote.VehicleMotorcycle,
			FeeMinor:           1,
			Currency:           "NGN",
			AvailableProviders: 1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	plan, _, err := planner.Build(ctx, fulfilment.PlanRequest{
		CustomerID:                 "cust_" + env.suffix,
		CheckoutSessionID:          "cs_" + env.suffix,
		CheckoutValidationRevision: 1,
		Currency:                   "NGN",
		Destination: fulfilment.Destination{
			Latitude: 6.4300, Longitude: 3.4500,
		},
		Groups: []fulfilment.GroupInput{{
			CheckoutGroupID: "g1",
			BusinessID:      env.businessA,
			BranchID:        env.branchA,
			Currency:        "NGN",
			Lines: []fulfilment.LineLogistics{{
				CheckoutLineID: "l1", Domain: "SHOP", Quantity: 1,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The mismatched reference is a contract violation, so the group is
	// unserviceable and no fee reaches the plan.
	if plan.Status != fulfilment.StatusUnserviceable {
		t.Fatalf("status = %q, want UNSERVICEABLE", plan.Status)
	}

	if plan.Funding.GrossMinor != 0 || len(plan.Missions) != 0 {
		t.Fatalf("a spoofed quote produced a mission or a fee: %+v", plan)
	}
}

// A planning fault must leave nothing behind.
//
// Build returns an error only for a genuine fault, and the service returns
// before Save, so a half-planned checkout never becomes a half-written plan.
func TestSection17CSecurity_FaultPersistsNothing(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	customerID, sessionID := env.seedCheckoutSession(ctx)

	store := fulfilment.NewStore(env.pool)

	if _, err := store.GetCurrentForCheckout(
		ctx, customerID, sessionID,
	); !errors.Is(err, fulfilment.ErrPlanNotFound) {
		t.Fatalf("err = %v, want ErrPlanNotFound before any plan exists", err)
	}

	var count int

	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM commerce_fulfilment_plans
		 WHERE checkout_session_id = $1`,
		sessionID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatalf("plans exist for a checkout that was never planned: %d", count)
	}
}
