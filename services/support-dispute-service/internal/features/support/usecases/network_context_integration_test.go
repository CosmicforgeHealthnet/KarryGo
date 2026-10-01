package supportusecases

// Section 15 against a real PostgreSQL and a real (fake) owner over HTTP.
//
// The owner is served through the SAME supportcase contract a real owning
// service mounts, so these tests exercise the signed read, the actor scoping
// and the minimisation end to end rather than against a Go double.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/supportcase"

	"cosmicforge/logistics/services/support-dispute-service/internal/database"
	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
	"cosmicforge/logistics/shared/go/notifications"
)

func networkPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SUPPORT_DISPUTE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SUPPORT_DISPUTE_TEST_DATABASE_URL to run Section 15 integration tests")
	}
	if !strings.Contains(strings.ToLower(dsn), "test") {
		t.Fatal("refusing a database whose name does not identify it as a test database")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	previous, _ := os.Getwd()
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	if err = database.ApplyMigrations(context.Background(), pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

// recordingOwner is the owning service. It records who it was asked about.
type recordingOwner struct {
	mu        sync.Mutex
	asked     []string
	partyTo   string
	extraJSON bool
	down      bool
}

func (o *recordingOwner) server(t *testing.T, secret []byte) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if o.extraJSON {
		// A misbehaving owner that sends more than the contract holds. The
		// snapshot must still contain only contract fields.
		engine.GET(strings.Replace(strings.Replace(supportcase.ContextPath, ":contextType", ":t", 1),
			":ref", ":r", 1), func(c *gin.Context) {
			c.Data(http.StatusOK, "application/json", []byte(`{"success":true,"data":{
				"state":"IN_RECOVERY","source_version":3,"pickup_latitude":6.43,
				"customer_phone":"08011112222","route_polyline":"abc",
				"actor_refs":{"customer_ref":"x"},"milestones":{}}}`))
		})
	} else {
		supportcase.RegisterContextRoute(engine.Group(""),
			serviceauth.NewVerifier(serviceauth.Secrets{supportcase.ContextCaller: secret}, 5*time.Minute),
			supportcase.ContextProjectorFunc(func(_ context.Context,
				request supportcase.ContextRequest) (supportcase.OperationalContext, error) {
				o.mu.Lock()
				defer o.mu.Unlock()
				o.asked = append(o.asked, request.ExpectedActorRef)
				if o.down {
					return supportcase.OperationalContext{}, errors.New("owner database down")
				}
				if request.ExpectedActorRef != o.partyTo {
					return supportcase.OperationalContext{}, supportcase.ErrContextNotFound
				}
				return supportcase.OperationalContext{
					SourceService: "driver-hauling-service", State: "IN_RECOVERY", SourceVersion: 42,
					RecoveryStatus: "REPLACEMENT_SEARCH", RecoveryCaseRef: "rc_1",
					ActorRefs: map[string]string{"customer_ref": o.partyTo, "provider_ref": "prv_2"},
				}, nil
			}))
	}
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server
}

// countingDeliverer is the owner's action contract.
type countingDeliverer struct {
	mu       sync.Mutex
	calls    []supportclients.ActionDelivery
	outcomes []supportclients.ActionOutcome
}

func (d *countingDeliverer) Deliver(_ context.Context,
	delivery supportclients.ActionDelivery) (supportclients.ActionOutcome, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, delivery)
	if len(d.outcomes) == 0 {
		return supportclients.ActionOutcome{Definite: true, Accepted: true, ResultRef: "rc_new",
			Code: "RECOVERY_STARTED"}, nil
	}
	next := d.outcomes[0]
	if len(d.outcomes) > 1 {
		d.outcomes = d.outcomes[1:]
	}
	return next, nil
}

func (d *countingDeliverer) callCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

type networkHarness struct {
	pool      *pgxpool.Pool
	support   *SupportService
	network   *NetworkContextService
	owner     *recordingOwner
	deliverer *countingDeliverer
	notified  *recordingNotifier
}

// recordingNotifier stands in for notification-service.
type recordingNotifier struct {
	mu       sync.Mutex
	requests []notifications.Request
	fail     bool
}

func (n *recordingNotifier) Send(_ context.Context, request notifications.Request) (notifications.SendResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.requests = append(n.requests, request)
	if n.fail {
		return notifications.SendResponse{}, errors.New("notification-service is down")
	}
	return notifications.SendResponse{Status: "queued"}, nil
}

func newNetworkHarness(t *testing.T, owner *recordingOwner) *networkHarness {
	t.Helper()
	pool := networkPool(t)
	secret := []byte("support-owner-secret")
	server := owner.server(t, secret)
	registry := supportclients.NewContextRegistry()
	registry.Register(supportcase.ContextHaulageBooking, "driver-hauling-service",
		supportcase.NewContextClient(server.URL, secret, time.Second))
	support := NewSupportService(supportrepositories.NewPostgresSupportRepository(pool), Options{})
	deliverer := &countingDeliverer{}
	notified := &recordingNotifier{}
	network := NewNetworkContextService(NetworkContextOptions{
		Support: support, Store: supportrepositories.NewNetworkContextStore(pool),
		Contexts: registry, HaulingRecovery: deliverer, Notifier: notified,
	})
	return &networkHarness{pool: pool, support: support, network: network, owner: owner,
		deliverer: deliverer, notified: notified}
}

// complaint opens a real complaint owned by a fresh customer.
func (h *networkHarness) complaint(t *testing.T, category string) (supportmodels.Complaint, string) {
	t.Helper()
	customer := "cus_" + uuid.NewString()
	created, err := h.support.CreateComplaint(context.Background(), CreateComplaintInput{
		ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: customer,
		ServiceType: supportmodels.ServiceTypeHauling, Category: category,
		Subject: "Truck broke down", Description: "The truck stopped on the expressway.",
	})
	if err != nil {
		t.Fatalf("create complaint: %v", err)
	}
	return created, customer
}

func (h *networkHarness) attach(t *testing.T, complaintID, bookingID string) {
	t.Helper()
	if _, err := h.network.AttachContext(context.Background(), AttachContextInput{
		ComplaintID: complaintID, Admin: true,
		Refs: supportmodels.ContextRefs{ContextType: supportcase.ContextHaulageBooking,
			MissionRef: bookingID},
	}); err != nil {
		t.Fatalf("attach context: %v", err)
	}
}

func appStatus(err error) int {
	if appErr := apperrors.From(err); appErr != nil {
		return appErr.Status
	}
	return 0
}

func TestNetworkCategoriesAreAcceptedAndUnknownOnesAreNot(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	for _, category := range []string{
		supportmodels.CategoryWrongVehicle, supportmodels.CategoryCapabilityMismatch,
		supportmodels.CategoryVendorDelay, supportmodels.CategoryProviderNoShow,
		supportmodels.CategoryRecoveryFailure, supportmodels.CategoryUnsafeVehicle,
		supportmodels.CategoryCargoFitFailure, supportmodels.CategoryTemperatureIssue,
		supportmodels.CategoryCustodyHandoverFailure,
	} {
		h.complaint(t, category) // fails the test itself if the category is refused
	}
	_, err := h.support.CreateComplaint(context.Background(), CreateComplaintInput{
		ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "cus_x",
		ServiceType: supportmodels.ServiceTypeHauling, Category: "made_up_category",
		Subject: "s", Description: "d",
	})
	if err == nil {
		t.Fatal("an uncontrolled category was accepted")
	}
}

// TestContextRefsAreAdditive: a complaint with no context is exactly what it
// was before Section 15.
func TestContextRefsAreAdditive(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	created, _ := h.complaint(t, "")
	refs, err := supportrepositories.NewNetworkContextStore(h.pool).GetContextRefs(
		context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refs != (supportmodels.ContextRefs{}) {
		t.Fatalf("a new complaint carries context it never set: %+v", refs)
	}
}

// TestComplainantCannotAttachContextToAnothersCase is the cross-user rule.
func TestComplainantCannotAttachContextToAnothersCase(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	created, _ := h.complaint(t, "")
	_, err := h.network.AttachContext(context.Background(), AttachContextInput{
		ComplaintID: created.ID, RequesterType: supportmodels.ComplainantCustomer,
		RequesterID: "cus_intruder",
		Refs: supportmodels.ContextRefs{ContextType: supportcase.ContextHaulageBooking,
			MissionRef: "bk_1"},
	})
	if err == nil {
		t.Fatal("another customer attached context to this complaint")
	}
}

// TestEnrichAsksTheOwnerAboutTheComplainant: an agent enriching a case sees
// only what the COMPLAINANT is party to.
func TestEnrichAsksTheOwnerAboutTheComplainant(t *testing.T) {
	owner := &recordingOwner{}
	h := newNetworkHarness(t, owner)
	created, customer := h.complaint(t, supportmodels.CategoryRecoveryFailure)
	owner.partyTo = customer
	h.attach(t, created.ID, "bk_"+uuid.NewString())

	snapshot, err := h.network.Enrich(context.Background(), created.ID, "corr-1")
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	if snapshot.Context.RecoveryCaseRef != "rc_1" || snapshot.SourceVersion != 42 {
		t.Fatalf("snapshot lost the owner's answer: %+v", snapshot)
	}
	if snapshot.CorrelationID != "corr-1" {
		t.Fatalf("correlation not preserved: %q", snapshot.CorrelationID)
	}
	if len(owner.asked) != 1 || owner.asked[0] != customer {
		t.Fatalf("owner was asked about %v, want the complainant %s", owner.asked, customer)
	}
}

// TestEnrichForAJobTheComplainantIsNotOn is refused by the owner.
func TestEnrichForAJobTheComplainantIsNotOn(t *testing.T) {
	owner := &recordingOwner{partyTo: "cus_somebody_else"}
	h := newNetworkHarness(t, owner)
	created, _ := h.complaint(t, "")
	h.attach(t, created.ID, "bk_"+uuid.NewString())
	if _, err := h.network.Enrich(context.Background(), created.ID, ""); appStatus(err) != 404 {
		t.Fatalf("enrich for a job the complainant is not on: %v", err)
	}
}

// TestOwnerOutageIsNotNotFound keeps the distinction on the wire.
func TestOwnerOutageIsNotNotFound(t *testing.T) {
	owner := &recordingOwner{down: true}
	h := newNetworkHarness(t, owner)
	created, customer := h.complaint(t, "")
	owner.partyTo = customer
	h.attach(t, created.ID, "bk_"+uuid.NewString())
	if _, err := h.network.Enrich(context.Background(), created.ID, ""); appStatus(err) != 503 {
		t.Fatalf("owner outage answered %v, want unavailable", err)
	}
}

// TestSnapshotContainsOnlyContractFields: a misbehaving owner cannot push
// location, contact or route data into Support's storage.
func TestSnapshotContainsOnlyContractFields(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{extraJSON: true})
	created, _ := h.complaint(t, "")
	h.attach(t, created.ID, "bk_"+uuid.NewString())
	if _, err := h.network.Enrich(context.Background(), created.ID, ""); err != nil {
		t.Fatalf("enrich: %v", err)
	}
	var stored string
	if err := h.pool.QueryRow(context.Background(),
		`SELECT minimized_json::text FROM complaint_context_snapshots WHERE complaint_id=$1`,
		created.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"latitude", "phone", "polyline", "08011112222", "6.43"} {
		if strings.Contains(stored, leaked) {
			t.Fatalf("snapshot stored %q: %s", leaked, stored)
		}
	}
}

// TestSnapshotsAreAppendOnly: what Support saw cannot be edited to match what
// is true later.
func TestSnapshotsAreAppendOnly(t *testing.T) {
	owner := &recordingOwner{}
	h := newNetworkHarness(t, owner)
	created, customer := h.complaint(t, "")
	owner.partyTo = customer
	h.attach(t, created.ID, "bk_"+uuid.NewString())
	snapshot, err := h.network.Enrich(context.Background(), created.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = h.pool.Exec(ctx, `UPDATE complaint_context_snapshots SET source_version=1 WHERE id=$1`,
		snapshot.ID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("a snapshot was editable: %v", err)
	}
	if _, err = h.pool.Exec(ctx, `DELETE FROM complaint_context_snapshots WHERE id=$1`,
		snapshot.ID); err == nil {
		t.Fatal("a snapshot was deletable")
	}
}

func (h *networkHarness) request(complaintID, key, trigger string) (supportmodels.ActionRequest, error) {
	return h.network.RequestAction(context.Background(), RequestActionInput{
		ComplaintID: complaintID, ActionCode: supportmodels.ActionRequestRecoveryEscalation,
		Parameters: map[string]string{"trigger_code": trigger}, IdempotencyKey: key,
		RequestedBy: "agent_1", CorrelationID: "corr-action",
	})
}

// TestActionRequestIsIdempotent: a retried request is the same request and
// reaches the owner once.
func TestActionRequestIsIdempotent(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	created, _ := h.complaint(t, "")
	h.attach(t, created.ID, "bk_"+uuid.NewString())

	first, err := h.request(created.ID, "key-1", "PROVIDER_UNREACHABLE")
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	second, err := h.request(created.ID, "key-1", "PROVIDER_UNREACHABLE")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("a replay created a second request: %s then %s", first.ID, second.ID)
	}
	if h.deliverer.callCount() != 1 {
		t.Fatalf("the owner was asked %d times for one request", h.deliverer.callCount())
	}
	if first.Status != supportmodels.ActionAccepted || first.ResultRef != "rc_new" {
		t.Fatalf("owner's answer not recorded: %+v", first)
	}
}

// TestSameKeyDifferentRequestConflicts: one key, two different requests.
func TestSameKeyDifferentRequestConflicts(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	created, _ := h.complaint(t, "")
	h.attach(t, created.ID, "bk_"+uuid.NewString())
	if _, err := h.request(created.ID, "key-2", "PROVIDER_UNREACHABLE"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.request(created.ID, "key-2", "VEHICLE_FAILURE"); appStatus(err) != 409 {
		t.Fatalf("reused key with a different trigger answered %v, want 409", err)
	}
}

// TestActionsWithoutAnOwnerContractAreRefused names the owner and the reason.
func TestActionsWithoutAnOwnerContractAreRefused(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	created, _ := h.complaint(t, "")
	h.attach(t, created.ID, "bk_"+uuid.NewString())

	_, err := h.network.RequestAction(context.Background(), RequestActionInput{
		ComplaintID: created.ID, ActionCode: supportmodels.ActionRequestRefundReview,
		IdempotencyKey: "key-refund", RequestedBy: "agent_1",
	})
	if appStatus(err) != 422 || !strings.Contains(err.Error(), "no money authority") {
		t.Fatalf("a refund action was not refused for the right reason: %v", err)
	}

	taxi, _ := h.complaint(t, "")
	if _, err = h.network.AttachContext(context.Background(), AttachContextInput{
		ComplaintID: taxi.ID, Admin: true,
		Refs: supportmodels.ContextRefs{ContextType: supportcase.ContextTaxiGoodsMission,
			MissionRef: "tgm_1"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err = h.network.RequestAction(context.Background(), RequestActionInput{
		ComplaintID: taxi.ID, ActionCode: supportmodels.ActionRequestRecoveryEscalation,
		Parameters: map[string]string{"trigger_code": "X"}, IdempotencyKey: "key-taxi",
		RequestedBy: "agent_1",
	})
	if appStatus(err) != 422 || !strings.Contains(err.Error(), "operations caller") {
		t.Fatalf("a Taxi Goods escalation was not refused with its reason: %v", err)
	}
	if h.deliverer.callCount() != 0 {
		t.Fatal("an unsupported action still reached an owner")
	}
}

// TestAmbiguousDeliveryIsRetriedAsTheSameOperation: after an unanswered
// attempt, the retry carries the SAME operation reference, so the owner
// replays instead of acting twice.
func TestAmbiguousDeliveryIsRetriedAsTheSameOperation(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	h.deliverer.outcomes = []supportclients.ActionOutcome{
		{Definite: false, Code: "OWNER_UNREACHABLE"},
		{Definite: true, Accepted: true, ResultRef: "rc_after_retry"},
	}
	created, _ := h.complaint(t, "")
	h.attach(t, created.ID, "bk_"+uuid.NewString())

	first, err := h.request(created.ID, "key-retry", "PROVIDER_BREAKDOWN")
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != supportmodels.ActionUnknown {
		t.Fatalf("an unanswered attempt recorded as %s", first.Status)
	}
	if _, err = h.pool.Exec(context.Background(),
		`UPDATE support_action_requests SET next_attempt_at=now() WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.network.DeliverPending(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	actions, err := h.network.ListActions(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if actions[0].Status != supportmodels.ActionAccepted || actions[0].ResultRef != "rc_after_retry" {
		t.Fatalf("retry did not resolve: %+v", actions[0])
	}
	if len(h.deliverer.calls) != 2 ||
		h.deliverer.calls[0].OperationRef != h.deliverer.calls[1].OperationRef {
		t.Fatalf("the retry was a different operation: %+v", h.deliverer.calls)
	}
}

// TestRefusalIsFinal: an owner's "no" is not retried.
func TestRefusalIsFinal(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	h.deliverer.outcomes = []supportclients.ActionOutcome{
		{Definite: true, Accepted: false, Code: "NOT_RECOVERABLE"}}
	created, _ := h.complaint(t, "")
	h.attach(t, created.ID, "bk_"+uuid.NewString())
	action, err := h.request(created.ID, "key-refused", "PROVIDER_BREAKDOWN")
	if err != nil {
		t.Fatal(err)
	}
	if action.Status != supportmodels.ActionRefused || action.CompletedAt == nil {
		t.Fatalf("refusal not recorded as final: %+v", action)
	}
	if _, err = h.network.DeliverPending(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if h.deliverer.callCount() != 1 {
		t.Fatalf("a refused request was sent again (%d calls)", h.deliverer.callCount())
	}
}

// Section 14: the complainant hears the owner's definite answer, once.
func TestADefiniteOutcomeIsAnnouncedToTheComplainant(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	created, customer := h.complaint(t, "")
	h.attach(t, created.ID, "bk_"+uuid.NewString())
	if _, err := h.request(created.ID, "key-notify", "PROVIDER_BREAKDOWN"); err != nil {
		t.Fatal(err)
	}
	if len(h.notified.requests) != 1 {
		t.Fatalf("notifications=%d, want 1", len(h.notified.requests))
	}
	sent := h.notified.requests[0]
	if sent.EventType != notifications.EventNetworkCustomerSupportUpdate || sent.Recipient.ID != customer ||
		sent.Recipient.Type != notifications.RecipientCustomer {
		t.Fatalf("notification %+v", sent)
	}
	if err := sent.Validate(); err != nil {
		t.Fatalf("the outcome notification is invalid: %v", err)
	}
}

// A notification outage never changes what the owner answered.
func TestANotificationOutageLeavesTheOutcomeStanding(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	h.notified.fail = true
	created, _ := h.complaint(t, "")
	h.attach(t, created.ID, "bk_"+uuid.NewString())
	action, err := h.request(created.ID, "key-outage", "PROVIDER_BREAKDOWN")
	if err != nil {
		t.Fatalf("a notification failure surfaced as an action failure: %v", err)
	}
	if action.Status != supportmodels.ActionAccepted {
		t.Fatalf("status=%s, want ACCEPTED despite the outage", action.Status)
	}
}

// fakeMedia answers evidence validations and records what it was asked.
type fakeMedia struct {
	verdict supportclients.EvidenceVerdict
	err     error
	asked   []string
}

func (m *fakeMedia) Validate(_ context.Context, objectID, bindingType, bindingID string) (supportclients.EvidenceVerdict, error) {
	m.asked = append(m.asked, objectID+"|"+bindingType+"|"+bindingID)
	return m.verdict, m.err
}

func withMedia(h *networkHarness, media *fakeMedia) {
	h.network.media = media
}

// E2E 6: private evidence is cited only through Media's verdict, against the
// complaint's own job.
func TestPrivateEvidenceIsCitedThroughMediasVerdict(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	media := &fakeMedia{verdict: supportclients.EvidenceVerdict{Usable: true, Purpose: "DAMAGE_EVIDENCE"}}
	withMedia(h, media)
	created, customer := h.complaint(t, "")
	booking := "bk_" + uuid.NewString()
	h.attach(t, created.ID, booking)

	evidence, err := h.network.ReferenceEvidence(context.Background(), ReferenceEvidenceInput{
		ComplaintID: created.ID, MediaObjectID: "med_1", BindingType: "mission",
		RequesterType: supportmodels.ComplainantCustomer, RequesterID: customer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.MediaAssetID == nil || *evidence.MediaAssetID != "med_1" {
		t.Fatalf("evidence %+v", evidence)
	}
	if len(media.asked) != 1 || media.asked[0] != "med_1|MISSION|"+booking {
		t.Fatalf("media was asked %v; the binding must come from the complaint", media.asked)
	}
}

func TestUnusableOrUnboundEvidenceIsRefused(t *testing.T) {
	h := newNetworkHarness(t, &recordingOwner{})
	media := &fakeMedia{verdict: supportclients.EvidenceVerdict{Usable: false, Failures: []string{"WRONG_BINDING"}}}
	withMedia(h, media)
	created, customer := h.complaint(t, "")
	h.attach(t, created.ID, "bk_"+uuid.NewString())
	cite := func(bindingType, requester string) error {
		_, err := h.network.ReferenceEvidence(context.Background(), ReferenceEvidenceInput{
			ComplaintID: created.ID, MediaObjectID: "med_2", BindingType: bindingType,
			RequesterType: supportmodels.ComplainantCustomer, RequesterID: requester,
		})
		return err
	}
	if status := appStatus(cite("MISSION", customer)); status != 422 {
		t.Fatalf("an unusable file -> %d, want 422", status)
	}
	if status := appStatus(cite("RECOVERY_CASE", customer)); status != 422 {
		t.Fatalf("citing a recovery case the complaint does not reference -> %d, want 422", status)
	}
	if status := appStatus(cite("MISSION", "cus_someone_else")); status != 403 && status != 404 {
		t.Fatalf("another customer's citation -> %d", status)
	}
	media.err = supportclients.ErrMediaUnavailable
	if status := appStatus(cite("MISSION", customer)); status != 503 {
		t.Fatalf("media down -> %d, want 503", status)
	}
	evidence, _ := h.support.ListEvidence(context.Background(), created.ID, supportmodels.ComplainantCustomer, customer)
	if len(evidence) != 0 {
		t.Fatalf("a refused citation stored %d evidence rows", len(evidence))
	}
}
