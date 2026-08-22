package main

// Section 17C HTTP contract, driven through the real production registration
// helper rather than a parallel test-only router.
//
// The gate this file exists for: a customer must not be able to overwrite the
// logistics fee from a request payload. The strongest possible assertion is
// that the surface has no such input at all, and that a request carrying one
// anyway changes nothing about the fee — which is what these tests check
// against the real handler.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/httpx"
)

const section17CCustomerSecret = "section17c-customer-access-secret"

// fixedQuoteAuthority answers every request with the same authoritative fee.
type fixedQuoteAuthority struct {
	feeMinor int64
}

func (fixedQuoteAuthority) Available() bool { return true }

func (a fixedQuoteAuthority) RequestQuote(
	_ context.Context, request commercequote.Request,
) (commercequote.Quote, error) {
	return commercequote.Quote{
		QuoteID:            "dq_fixed",
		QuoteReference:     request.QuoteReference,
		Revision:           1,
		Serviceable:        true,
		ReasonCode:         commercequote.ReasonServiceable,
		VehicleClass:       request.VehicleClass,
		PickupRef:          "dq_fixed:pickup",
		DropoffRef:         "dq_fixed:dropoff",
		DistanceKm:         4.9,
		EstimatedMinutes:   26,
		FeeMinor:           a.feeMinor,
		Currency:           "NGN",
		AvailableProviders: 2,
		ExpiresAt:          time.Now().Add(10 * time.Minute),
	}, nil
}

// section17CHTTPFixture builds the real router over a real database, with the
// logistics authority faked and nothing else.
func (e *s17cEnv) section17CHTTPFixture(
	t *testing.T,
	feeMinor int64,
) (*gin.Engine, string, string) {
	t.Helper()

	return e.section17CHTTPFixtureWith(
		t, fixedQuoteAuthority{feeMinor: feeMinor},
	)
}

func (e *s17cEnv) section17CHTTPFixtureWith(
	t *testing.T,
	dispatch fulfilment.QuoteAuthority,
) (*gin.Engine, string, string) {
	t.Helper()

	ctx := context.Background()

	customerID, sessionID := e.seedCheckoutSession(ctx)

	e.exec(ctx,
		`INSERT INTO vendor_branch_service_areas(
			branch_id,business_id,mode,center_latitude,center_longitude,radius_meters)
		 VALUES($1,$2,'RADIUS',6.4550,3.4100,15000)`,
		e.branchA, e.businessA)

	e.exec(ctx,
		`INSERT INTO commerce_cart_addresses(
			cart_id,source_address_ref,address_snapshot,
			latitude_snapshot,longitude_snapshot)
		 VALUES($1,'saved_1','1 Test Street',6.4300,3.4500)`,
		"cart_"+e.suffix)

	groupID := "cg_" + e.suffix

	e.exec(ctx,
		`INSERT INTO commerce_checkout_groups(
			id,session_id,customer_id,domain,business_id,branch_id,
			currency,quoted_total_minor,max_authorized_total_minor)
		 VALUES($1,$2,$3,'SHOP',$4,$5,'NGN',500000,500000)`,
		groupID, sessionID, customerID, e.businessA, e.branchA)

	variantID := e.seedVariant(ctx, s17cVariantOptions{
		netWeightGrams: ptrInt64(2000),
		stackable:      true,
	})

	listingID := e.seedListing(ctx, variantID)

	e.exec(ctx,
		`INSERT INTO commerce_checkout_lines(
			id,session_id,group_id,customer_id,cart_item_id,domain,
			business_id,branch_id,item_ref,listing_ref,quantity,
			validation_status)
		 VALUES($1,$2,$3,$4,'ci_1','SHOP',$5,$6,'item_1',$7,1,'VALID')`,
		"cl_"+e.suffix, sessionID, groupID, customerID,
		e.businessA, e.branchA, listingID)

	planner, err := fulfilment.NewPlanner(fulfilment.Options{
		Logistics: newSection17CLogisticsAuthority(e.pool),
		Branches:  newSection17CBranchAuthority(e.pool),
		Dispatch:  dispatch,
	})
	if err != nil {
		t.Fatal(err)
	}

	service, err := fulfilment.NewService(fulfilment.ServiceOptions{
		Store:    fulfilment.NewStore(e.pool),
		Planner:  planner,
		Checkout: newSection17CCheckoutAuthority(e.pool),
	})
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)

	router := gin.New()

	// The same platform middleware serviceapp.Run installs. Without the error
	// handler the shared bearer middleware's abort would record an error that
	// nothing renders, and a test router would report 200 for a request
	// production refuses.
	router.Use(httpx.RequestID())
	router.Use(httpx.Recovery())
	router.Use(httpx.ErrorHandler())

	registerSection17CFulfilmentRoutes(
		router,
		config.Config{
			CustomerAccessTokenSecret: []byte(section17CCustomerSecret),
			Flags: config.FeatureFlags{
				CommerceFulfilmentPlanningEnabled: true,
			},
		},
		service,
	)

	return router, customerID, sessionID
}

// section17CHTTPFixtureNoAuthority is the same fixture with no delivery
// authority available at all, which is the shortest honest route to an
// UNSERVICEABLE plan.
func (e *s17cEnv) section17CHTTPFixtureNoAuthority(
	t *testing.T,
) (*gin.Engine, string, string) {
	t.Helper()

	return e.section17CHTTPFixtureWith(t, nil)
}

func section17CToken(t *testing.T, customerID string) string {
	t.Helper()

	signer := sharedauth.NewTokenSigner([]byte(section17CCustomerSecret))

	token, err := signer.Sign(sharedauth.Claims{
		Subject:   customerID,
		Role:      "customer",
		Service:   "customer",
		SessionID: "sess_" + customerID,
		Type:      sharedauth.TokenTypeAccess,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	return token
}

func decodePlan(t *testing.T, body []byte) fulfilment.Plan {
	t.Helper()

	var envelope struct {
		Success bool            `json:"success"`
		Data    fulfilment.Plan `json:"data"`
	}

	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, body)
	}

	if !envelope.Success {
		t.Fatalf("unsuccessful response: %s", body)
	}

	return envelope.Data
}

// The fee on the plan is the authority's, whatever the customer sent.
func TestSection17CHTTP_CustomerCannotOverwriteTheLogisticsFee(t *testing.T) {
	env := newSection17CEnv(t)

	const authorityFee = 150000

	router, customerID, sessionID := env.section17CHTTPFixture(t, authorityFee)

	token := section17CToken(t, customerID)

	// A body that tries every plausible way to name a cheaper fee.
	payload, err := json.Marshal(map[string]any{
		"fee_minor":          1,
		"gross_fee_minor":    1,
		"customer_fee_minor": 1,
		"delivery_fee":       1,
		"currency":           "USD",
		"business_id":        "someone-else",
		"branch_id":          "someone-elses-branch",
		"customer_id":        "someone-else",
		"service":            "VENDOR_OWN_DELIVERY",
		"vehicle_class":      "MOTORCYCLE",
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/commerce/checkout/"+sessionID+"/fulfilment-plan",
		bytes.NewReader(payload),
	)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	plan := decodePlan(t, recorder.Body.Bytes())

	if plan.Status != fulfilment.StatusPlanned {
		t.Fatalf("status = %q, groups = %+v", plan.Status, plan.Groups)
	}

	if len(plan.Missions) != 1 {
		t.Fatalf("missions = %d, want 1", len(plan.Missions))
	}

	mission := plan.Missions[0]

	if mission.Funding.GrossMinor != authorityFee {
		t.Fatalf("fee = %d, want the authority's %d",
			mission.Funding.GrossMinor, authorityFee)
	}

	if mission.Currency != "NGN" {
		t.Fatalf("currency = %q, want NGN — the body asked for USD",
			mission.Currency)
	}

	// The body named another vendor and another authority. Neither took.
	if mission.PickupBusinessID != env.businessA ||
		mission.PickupBranchID != env.branchA {
		t.Fatalf("pickup was influenced by the body: %+v", mission)
	}

	if mission.Authority != fulfilment.AuthorityPlatform {
		t.Fatalf("authority = %q, want PLATFORM_LOGISTICS", mission.Authority)
	}

	if mission.Quote.QuoteID != "dq_fixed" {
		t.Fatalf("quote id = %q, want the authority's", mission.Quote.QuoteID)
	}
}

// The same request with no body at all produces the identical plan, which is
// the honest statement that the body was never read.
func TestSection17CHTTP_BodyIsIrrelevant(t *testing.T) {
	env := newSection17CEnv(t)

	router, customerID, sessionID := env.section17CHTTPFixture(t, 150000)

	token := section17CToken(t, customerID)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/commerce/checkout/"+sessionID+"/fulfilment-plan",
		nil,
	)
	request.Header.Set("Authorization", "Bearer "+token)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	plan := decodePlan(t, recorder.Body.Bytes())

	if plan.Status != fulfilment.StatusPlanned ||
		plan.Missions[0].Funding.GrossMinor != 150000 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

// Another customer's token must not reach this checkout's plan.
func TestSection17CHTTP_CrossCustomerIsRefused(t *testing.T) {
	env := newSection17CEnv(t)

	router, _, sessionID := env.section17CHTTPFixture(t, 150000)

	token := section17CToken(t, "some-other-customer")

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/commerce/checkout/"+sessionID+"/fulfilment-plan",
		nil,
	)
	request.Header.Set("Authorization", "Bearer "+token)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	// The session does not resolve for this customer, so it is not plannable.
	if recorder.Code == http.StatusOK {
		t.Fatalf("another customer planned this checkout: %s",
			recorder.Body.String())
	}
}

func TestSection17CHTTP_AuthenticationIsRequired(t *testing.T) {
	env := newSection17CEnv(t)

	router, _, sessionID := env.section17CHTTPFixture(t, 150000)

	for _, method := range []string{http.MethodPost, http.MethodGet} {
		request := httptest.NewRequest(
			method,
			"/api/v1/commerce/checkout/"+sessionID+"/fulfilment-plan",
			nil,
		)

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s without a token = %d, want 401",
				method, recorder.Code)
		}
	}
}

// A vendor staff token is not a customer token. The two principals are
// separate and neither may act as the other.
func TestSection17CHTTP_VendorStaffTokenIsRefused(t *testing.T) {
	env := newSection17CEnv(t)

	router, customerID, sessionID := env.section17CHTTPFixture(t, 150000)

	signer := sharedauth.NewTokenSigner([]byte(section17CCustomerSecret))

	staffToken, err := signer.Sign(sharedauth.Claims{
		Subject:   customerID,
		Role:      "vendor_staff",
		Service:   "vendor",
		SessionID: "sess_staff",
		Type:      sharedauth.TokenTypeAccess,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/commerce/checkout/"+sessionID+"/fulfilment-plan",
		nil,
	)
	request.Header.Set("Authorization", "Bearer "+staffToken)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	// The shared bearer middleware distinguishes the two refusals: a missing
	// or unverifiable token is 401, while a token that verifies but carries
	// the wrong principal is 403. Asserting the specific code matters — a test
	// that accepted "any 4xx" would still pass if the two were ever collapsed,
	// and losing that distinction is how a role check quietly becomes a
	// signature check.
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("a vendor staff token was answered %d, want 403",
			recorder.Code)
	}
}

// Planning then reading returns the same plan, and the read carries the same
// authority fee.
func TestSection17CHTTP_PlanThenRead(t *testing.T) {
	env := newSection17CEnv(t)

	router, customerID, sessionID := env.section17CHTTPFixture(t, 150000)

	token := section17CToken(t, customerID)

	post := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/commerce/checkout/"+sessionID+"/fulfilment-plan",
		nil,
	)
	post.Header.Set("Authorization", "Bearer "+token)

	postRecorder := httptest.NewRecorder()
	router.ServeHTTP(postRecorder, post)

	if postRecorder.Code != http.StatusOK {
		t.Fatalf("plan failed: %s", postRecorder.Body.String())
	}

	created := decodePlan(t, postRecorder.Body.Bytes())

	get := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/commerce/checkout/"+sessionID+"/fulfilment-plan",
		nil,
	)
	get.Header.Set("Authorization", "Bearer "+token)

	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, get)

	if getRecorder.Code != http.StatusOK {
		t.Fatalf("read failed: %s", getRecorder.Body.String())
	}

	read := decodePlan(t, getRecorder.Body.Bytes())

	if read.ID != created.ID {
		t.Fatalf("read plan %q, created %q", read.ID, created.ID)
	}

	if read.Missions[0].Funding.GrossMinor !=
		created.Missions[0].Funding.GrossMinor {
		t.Fatal("the fee changed between planning and reading")
	}
}

// An UNSERVICEABLE plan must survive a page reload.
//
// The customer was just told which vendors cannot be delivered and why; a GET
// that answered 404 immediately afterwards would throw that answer away.
func TestSection17CHTTP_UnserviceablePlanStaysReadable(t *testing.T) {
	env := newSection17CEnv(t)

	router, customerID, sessionID := env.section17CHTTPFixtureNoAuthority(t)

	token := section17CToken(t, customerID)

	post := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/commerce/checkout/"+sessionID+"/fulfilment-plan",
		nil,
	)
	post.Header.Set("Authorization", "Bearer "+token)

	postRecorder := httptest.NewRecorder()
	router.ServeHTTP(postRecorder, post)

	if postRecorder.Code != http.StatusOK {
		t.Fatalf("plan failed: %s", postRecorder.Body.String())
	}

	created := decodePlan(t, postRecorder.Body.Bytes())

	if created.Status != fulfilment.StatusUnserviceable {
		t.Fatalf("status = %q, want UNSERVICEABLE", created.Status)
	}

	if len(created.Groups) != 1 || created.Groups[0].Serviceable {
		t.Fatalf("the refused group was dropped: %+v", created.Groups)
	}

	get := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/commerce/checkout/"+sessionID+"/fulfilment-plan",
		nil,
	)
	get.Header.Set("Authorization", "Bearer "+token)

	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, get)

	if getRecorder.Code != http.StatusOK {
		t.Fatalf("an unserviceable plan was not readable: %d %s",
			getRecorder.Code, getRecorder.Body.String())
	}

	read := decodePlan(t, getRecorder.Body.Bytes())

	if read.ID != created.ID {
		t.Fatalf("read plan %q, created %q", read.ID, created.ID)
	}

	if read.Groups[0].ReasonCode != created.Groups[0].ReasonCode {
		t.Fatal("the refusal reason did not survive the read")
	}
}
