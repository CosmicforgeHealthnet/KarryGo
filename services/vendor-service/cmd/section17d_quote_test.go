package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
)

type section17DSource struct {
	service    *checkoutquote.Service
	customerID string
	sessionID  string
	planID     string
}

func seedSection17DSource(t *testing.T, env *s17cEnv, variable, multi bool, now func() time.Time) section17DSource {
	return seedSection17DSourceWithOptions(t, env, variable, multi, false, now)
}

func seedSection19VariableMultiSource(t *testing.T, env *s17cEnv, now func() time.Time) section17DSource {
	return seedSection17DSourceWithOptions(t, env, true, true, true, now)
}

func seedSection17DSourceWithOptions(t *testing.T, env *s17cEnv, variable, multi, allVariable bool, now func() time.Time) section17DSource {
	t.Helper()
	ctx := context.Background()
	customerID, sessionID := env.seedCheckoutSession(ctx)
	cartID := "cart_" + env.suffix
	addressRef := "address_" + env.suffix
	env.exec(ctx, `INSERT INTO commerce_cart_addresses(cart_id,source_address_ref,title_snapshot,address_snapshot,latitude_snapshot,longitude_snapshot,description_snapshot) VALUES($1,$2,'Home','1 Quote Street',6.43,3.45,'Front desk')`, cartID, addressRef)

	productTotal, maxProduct := int64(1000), int64(1000)
	if variable {
		maxProduct = 1150
	}
	if multi {
		productTotal += 2000
		if variable && allVariable {
			maxProduct += 2300
		} else {
			maxProduct += 2000
		}
	}
	env.exec(ctx, `UPDATE commerce_checkout_sessions SET address_ref=$2,address_snapshotted_at=(SELECT snapshotted_at FROM commerce_cart_addresses WHERE cart_id=$1),quoted_items_total_minor=$3,max_authorized_items_total_minor=$4 WHERE id=$5`, cartID, addressRef, productTotal, maxProduct, sessionID)

	type groupSeed struct {
		id, business, branch, lineID, itemID, reservation, sourceRef string
		product, maximum, delivery, customer, vendor, platform       int64
		variable                                                     bool
	}
	groups := []groupSeed{{
		id: "cqg_src_a_" + env.suffix, business: env.businessA, branch: env.branchA,
		lineID: "cql_src_a_" + env.suffix, itemID: "cart_item_a_" + env.suffix,
		reservation: "reservation_a_" + env.suffix, sourceRef: "reservation_source_a_" + env.suffix,
		product: 1000, maximum: map[bool]int64{false: 1000, true: 1150}[variable],
		delivery: 500, customer: 200, vendor: 300, variable: variable,
	}}
	if multi {
		groups = append(groups, groupSeed{
			id: "cqg_src_b_" + env.suffix, business: env.businessB, branch: env.branchB,
			lineID: "cql_src_b_" + env.suffix, itemID: "cart_item_b_" + env.suffix,
			reservation: "reservation_b_" + env.suffix, sourceRef: "reservation_source_b_" + env.suffix,
			product: 2000, maximum: map[bool]int64{false: 2000, true: 2300}[allVariable], delivery: 700, customer: 500, platform: 200, variable: allVariable,
		})
	}

	planID := "vfp_quote_" + env.suffix
	var gross, customer, vendor, platform int64
	for _, group := range groups {
		gross += group.delivery
		customer += group.customer
		vendor += group.vendor
		platform += group.platform
	}
	env.exec(ctx, `INSERT INTO commerce_fulfilment_plans(id,checkout_session_id,customer_id,checkout_validation_revision,status,mode,plan_revision,currency,delivery_gross_minor,delivery_customer_minor,delivery_vendor_funded_minor,delivery_platform_funded_minor,expires_at,planned_at) VALUES($1,$2,$3,1,'PLANNED','SINGLE',1,'NGN',$4,$5,$6,$7,now()+interval '10 minutes',now())`, planID, sessionID, customerID, gross, customer, vendor, platform)

	for index, group := range groups {
		env.exec(ctx, `INSERT INTO commerce_checkout_groups(id,session_id,customer_id,domain,business_id,branch_id,requires_stock_reservation,reservation_status,reservation_source_ref,inventory_reservation_ref,catalogue_revision,inventory_revision,currency,quoted_total_minor,max_authorized_total_minor) VALUES($1,$2,$3,'SHOP',$4,$5,true,'RESERVED',$6,$7,11,22,'NGN',$8,$9)`, group.id, sessionID, customerID, group.business, group.branch, group.sourceRef, group.reservation, group.product, group.maximum)
		pricingBasis, measureRef, expected, overfill := "FIXED_UNIT", "", int64(0), 0
		if group.variable {
			pricingBasis, measureRef, expected, overfill = "VARIABLE_MEASURE", "kg", 1000, 1500
		}
		env.exec(ctx, `INSERT INTO commerce_checkout_lines(id,session_id,group_id,customer_id,cart_item_id,domain,business_id,branch_id,item_ref,variant_ref,listing_ref,sell_unit_ref,quantity,unit_price_minor,currency,price_reference,catalogue_revision,inventory_revision,pricing_basis,requested_measure_ref,expected_measure_quantity,measure_overfill_bps,quoted_total_minor,max_authorized_total_minor,inventory_item_ref,reservation_quantity,reservation_basis,validation_status) VALUES($1,$2,$3,$4,$5,'SHOP',$6,$7,$8,$9,$10,$11,1,$12,'NGN',$13,11,22,$14,$15,$16,$17,$18,$19,$20,1,'SELL_UNIT','VALID')`, group.lineID, sessionID, group.id, customerID, group.itemID, group.business, group.branch, "product_"+env.suffix, "variant_"+env.suffix, "listing_"+env.suffix, "unit_"+env.suffix, group.product, "online-price-v1", pricingBasis, measureRef, expected, overfill, group.product, group.maximum, "stock_"+env.suffix)

		missionID := "mission_" + string(rune('a'+index)) + "_" + env.suffix
		env.exec(ctx, `INSERT INTO commerce_fulfilment_missions(id,plan_id,customer_id,authority,service,vehicle_class,pickup_business_id,pickup_branch_id,pickup_latitude,pickup_longitude,dropoff_latitude,dropoff_longitude,authority_quote_id,authority_quote_reference,authority_quote_revision,authority_quote_expires_at,currency,gross_fee_minor,customer_fee_minor,vendor_funded_minor,platform_funded_minor,fee_beneficiary) VALUES($1,$2,$3,'PLATFORM_LOGISTICS','DISPATCH','MOTORCYCLE',$4,$5,6.455,3.41,6.43,3.45,$6,$7,1,now()+interval '10 minutes','NGN',$8,$9,$10,$11,'PLATFORM_PROVIDER')`, missionID, planID, customerID, group.business, group.branch, "delivery_quote_"+env.suffix+string(rune('a'+index)), "delivery_ref_"+env.suffix+string(rune('a'+index)), group.delivery, group.customer, group.vendor, group.platform)
		env.exec(ctx, `INSERT INTO commerce_fulfilment_plan_groups(id,plan_id,customer_id,checkout_group_id,business_id,branch_id,serviceable,reason_code,authority,mission_id,vehicle_class,gross_fee_minor,customer_fee_minor,vendor_funded_minor,platform_funded_minor) VALUES($1,$2,$3,$4,$5,$6,true,'SERVICEABLE','PLATFORM_LOGISTICS',$7,'MOTORCYCLE',$8,$9,$10,$11)`, "plan_group_"+string(rune('a'+index))+"_"+env.suffix, planID, customerID, group.id, group.business, group.branch, missionID, group.delivery, group.customer, group.vendor, group.platform)
	}

	options := checkoutquote.Options{Store: checkoutquote.NewStore(env.pool, events.NewDomainPublisher(events.NewOutboxRepository(env.pool))), Promotions: checkoutquote.NoPromotions{}, QuoteTTL: 5 * time.Minute}
	if now != nil {
		options.Now = now
	}
	service, err := checkoutquote.NewService(options)
	if err != nil {
		t.Fatal(err)
	}
	return section17DSource{service: service, customerID: customerID, sessionID: sessionID, planID: planID}
}

func TestSection17DQuote_ImmutableApprovalHistoryAndNoSilentRecompute(t *testing.T) {
	env := newSection17CEnv(t)
	fixture := seedSection17DSource(t, env, false, false, nil)
	ctx := context.Background()
	first, err := fixture.service.Create(ctx, fixture.customerID, fixture.sessionID, fixture.planID)
	if err != nil {
		t.Fatal(err)
	}
	if first.QuotedTotalMinor != 1200 || first.MaxAuthorizedTotalMinor != 1200 {
		t.Fatalf("exact total = %d/%d", first.QuotedTotalMinor, first.MaxAuthorizedTotalMinor)
	}
	approved, err := fixture.service.Approve(ctx, fixture.customerID, first.ID, first.QuoteFingerprint)
	if err != nil || approved.State != checkoutquote.StateApproved {
		t.Fatalf("approve: state=%s err=%v", approved.State, err)
	}
	var lockedEvents int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE event_type='commerce.quote.locked' AND aggregate_type='CommerceQuote' AND aggregate_id=$1`, first.ID).Scan(&lockedEvents); err != nil || lockedEvents != 1 {
		t.Fatalf("transactional quote locked events=%d err=%v", lockedEvents, err)
	}
	if _, err = fixture.service.Approve(ctx, fixture.customerID, first.ID, first.QuoteFingerprint); err != nil {
		t.Fatalf("idempotent approval: %v", err)
	}
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE event_type='commerce.quote.locked' AND aggregate_id=$1`, first.ID).Scan(&lockedEvents); err != nil || lockedEvents != 1 {
		t.Fatalf("idempotent quote locked events=%d err=%v", lockedEvents, err)
	}
	if _, err = env.pool.Exec(ctx, `UPDATE commerce_quotes SET quoted_total_minor=1 WHERE id=$1`, first.ID); err == nil {
		t.Fatal("database allowed immutable quote mutation")
	}

	// Authoritative evidence can change, but the historical quote never does.
	env.exec(ctx, `UPDATE commerce_checkout_lines SET unit_price_minor=1100,quoted_total_minor=1100,max_authorized_total_minor=1100,price_reference='online-price-v2' WHERE session_id=$1`, fixture.sessionID)
	env.exec(ctx, `UPDATE commerce_checkout_groups SET quoted_total_minor=1100,max_authorized_total_minor=1100 WHERE session_id=$1`, fixture.sessionID)
	env.exec(ctx, `UPDATE commerce_checkout_sessions SET quoted_items_total_minor=1100,max_authorized_items_total_minor=1100 WHERE id=$1`, fixture.sessionID)
	historical, err := fixture.service.Get(ctx, fixture.customerID, first.ID)
	if err != nil || historical.QuotedTotalMinor != 1200 || historical.QuoteFingerprint != first.QuoteFingerprint {
		t.Fatalf("historical quote recomputed: %+v err=%v", historical, err)
	}
	second, err := fixture.service.Create(ctx, fixture.customerID, fixture.sessionID, fixture.planID)
	if err != nil {
		t.Fatal(err)
	}
	if second.QuoteVersion != 2 || second.QuoteFingerprint == first.QuoteFingerprint || second.QuotedTotalMinor != 1300 {
		t.Fatalf("material change did not create a new agreement: first=%+v second=%+v", first, second)
	}
	if _, err = fixture.service.Get(ctx, "another_customer", first.ID); !errors.Is(err, checkoutquote.ErrQuoteNotFound) {
		t.Fatalf("customer isolation failed: %v", err)
	}
}

func TestSection17DQuote_VariableMultiVendorAndFundingBreakdown(t *testing.T) {
	env := newSection17CEnv(t)
	fixture := seedSection17DSource(t, env, true, true, nil)
	quote, err := fixture.service.Create(context.Background(), fixture.customerID, fixture.sessionID, fixture.planID)
	if err != nil {
		t.Fatal(err)
	}
	if len(quote.Groups) != 2 || len(quote.Missions) != 2 || len(quote.Lines) != 2 {
		t.Fatalf("multi-vendor evidence lost: groups=%d missions=%d lines=%d", len(quote.Groups), len(quote.Missions), len(quote.Lines))
	}
	if quote.QuotedTotalMinor != 3700 || quote.MaxAuthorizedTotalMinor != 3850 {
		t.Fatalf("bounded variable totals=%d/%d", quote.QuotedTotalMinor, quote.MaxAuthorizedTotalMinor)
	}
	if quote.FundingBreakdown.DeliveryCustomerMinor != 700 || quote.FundingBreakdown.DeliveryVendorFundedMinor != 300 || quote.FundingBreakdown.DeliveryPlatformFundedMinor != 200 {
		t.Fatalf("delivery funding misattributed: %+v", quote.FundingBreakdown)
	}
	for _, group := range quote.Groups {
		if group.BusinessID == "" || group.BranchID == "" || group.InventoryReservationRef == "" || group.LogisticsPlanGroupRef == "" || group.LogisticsMissionRef == "" {
			t.Fatalf("vendor/allocation/logistics evidence incomplete: %+v", group)
		}
	}
	for _, mission := range quote.Missions {
		if mission.AuthorityQuoteID == "" || mission.AuthorityQuoteReference == "" || mission.AuthorityQuoteRevision < 1 {
			t.Fatalf("authority quote evidence incomplete: %+v", mission)
		}
	}
	for _, line := range quote.Lines {
		if line.OnlinePriceDisclosure != "VENDOR_ONLINE_PRICE" || line.StorePriceParityClaimed {
			t.Fatalf("online disclosure missing: %+v", line)
		}
	}
}

func TestSection17DQuote_ConcurrentGenerationConverges(t *testing.T) {
	env := newSection17CEnv(t)
	fixture := seedSection17DSource(t, env, false, false, nil)
	const count = 12
	ids := make(chan string, count)
	errs := make(chan error, count)
	var wait sync.WaitGroup
	for i := 0; i < count; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			quote, err := fixture.service.Create(context.Background(), fixture.customerID, fixture.sessionID, fixture.planID)
			if err != nil {
				errs <- err
				return
			}
			ids <- quote.ID
		}()
	}
	wait.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Errorf("concurrent create: %v", err)
	}
	var expected string
	for id := range ids {
		if expected == "" {
			expected = id
		} else if id != expected {
			t.Errorf("concurrent material produced %q and %q", expected, id)
		}
	}
	var rows int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM commerce_quotes WHERE checkout_session_id=$1`, fixture.sessionID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("stored quote rows=%d err=%v", rows, err)
	}
}

func TestSection17DQuote_ExpiryRequiresNewApproval(t *testing.T) {
	env := newSection17CEnv(t)
	clock := time.Now().UTC()
	fixture := seedSection17DSource(t, env, false, false, func() time.Time { return clock })
	quote, err := fixture.service.Create(context.Background(), fixture.customerID, fixture.sessionID, fixture.planID)
	if err != nil {
		t.Fatal(err)
	}
	clock = quote.ExpiresAt.Add(time.Second)
	if _, err = fixture.service.Approve(context.Background(), fixture.customerID, quote.ID, quote.QuoteFingerprint); !errors.Is(err, checkoutquote.ErrQuoteExpired) {
		t.Fatalf("expired quote approved: %v", err)
	}
	read, err := fixture.service.Get(context.Background(), fixture.customerID, quote.ID)
	if err != nil || read.State != checkoutquote.StateExpired || !read.Expired {
		t.Fatalf("expired state=%s expired=%t err=%v", read.State, read.Expired, err)
	}
	renewed, err := fixture.service.Create(context.Background(), fixture.customerID, fixture.sessionID, fixture.planID)
	if err != nil {
		t.Fatalf("new quote after expiry: %v", err)
	}
	if renewed.ID == quote.ID || renewed.QuoteVersion != 2 || renewed.QuoteFingerprint == quote.QuoteFingerprint {
		t.Fatalf("expiry did not require a new agreement: old=%+v new=%+v", quote, renewed)
	}
}

func TestSection17DQuote_MaterialCustomerChangeInvalidatesApproval(t *testing.T) {
	env := newSection17CEnv(t)
	fixture := seedSection17DSource(t, env, false, false, nil)
	ctx := context.Background()
	quote, err := fixture.service.Create(ctx, fixture.customerID, fixture.sessionID, fixture.planID)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the address authority replacing the cart snapshot. The quote is
	// retained as history, but it may no longer be approved.
	env.exec(ctx, `UPDATE commerce_cart_addresses SET address_snapshot='2 Changed Street',updated_at=now() WHERE cart_id=(SELECT cart_id FROM commerce_checkout_sessions WHERE id=$1)`, fixture.sessionID)
	if _, err = fixture.service.Approve(ctx, fixture.customerID, quote.ID, quote.QuoteFingerprint); !errors.Is(err, checkoutquote.ErrQuoteConflict) {
		t.Fatalf("material address change left old quote approvable: %v", err)
	}
	historical, err := fixture.service.Get(ctx, fixture.customerID, quote.ID)
	if err != nil || historical.Address.Address != "1 Quote Street" {
		t.Fatalf("historical address was recomputed: %+v err=%v", historical.Address, err)
	}
}
