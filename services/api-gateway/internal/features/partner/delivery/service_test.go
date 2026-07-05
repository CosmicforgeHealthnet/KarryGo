package delivery

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- fakes ------------------------------------------------------------------

type fakeBookingRepo struct {
	mu          sync.Mutex
	byID        map[string]*PartnerBooking
	createCalls int
	failNextRef int // number of initial CreatePendingBooking calls that report a ref collision
}

func newFakeBookingRepo() *fakeBookingRepo {
	return &fakeBookingRepo{byID: map[string]*PartnerBooking{}}
}

func (r *fakeBookingRepo) CreatePendingBooking(_ context.Context, b *PartnerBooking) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCalls++
	if r.failNextRef > 0 {
		r.failNextRef--
		return ErrDuplicateBookingRef
	}
	cp := *b
	r.byID[b.ID] = &cp
	return nil
}

func (r *fakeBookingRepo) UpdatePaymentAndDispatching(_ context.Context, id, paymentIntentID, dispatchID string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.byID[id]
	if !ok {
		return errors.New("not found")
	}
	b.Status = StatusDispatching
	b.PaymentIntentID = &paymentIntentID
	b.DispatchBookingID = &dispatchID
	b.EventPublishedAt = &at
	return nil
}

func (r *fakeBookingRepo) FindByPartnerAndIDOrRef(_ context.Context, partnerID, idOrRef string) (*PartnerBooking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range r.byID {
		if b.PartnerID != partnerID {
			continue
		}
		if b.ID == idOrRef || b.BookingRef == idOrRef {
			cp := *b
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *fakeBookingRepo) ListByPartner(_ context.Context, partnerID string, f ListFilter) ([]PartnerBooking, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []PartnerBooking
	for _, b := range r.byID {
		if b.PartnerID != partnerID {
			continue
		}
		if f.Status != "" && b.Status != f.Status {
			continue
		}
		all = append(all, *b)
	}
	return all, len(all), nil
}

func (r *fakeBookingRepo) UpdateStatus(_ context.Context, id, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.byID[id]; ok {
		b.Status = status
	}
	return nil
}

type fakeIdemStore struct {
	mu        sync.Mutex
	records   map[string]*IdempotencyRecord
	putCalled bool
}

func newFakeIdemStore() *fakeIdemStore {
	return &fakeIdemStore{records: map[string]*IdempotencyRecord{}}
}

func (s *fakeIdemStore) Get(_ context.Context, key string) (*IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.records[key]; ok {
		cp := *rec
		return &cp, nil
	}
	return nil, nil
}

func (s *fakeIdemStore) Put(_ context.Context, rec IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putCalled = true
	cp := rec
	s.records[rec.Key] = &cp
	return nil
}

type fakeWallet struct {
	createErr   error
	payErr      error
	refundErr   error
	lastCreate  PaymentIntentInput
	lastRefund  RefundInput
	createCalls int
	payCalls    int
	refundCalls int
}

func (w *fakeWallet) CreatePaymentIntent(_ context.Context, in PaymentIntentInput) (PaymentIntentResult, error) {
	w.createCalls++
	w.lastCreate = in
	if w.createErr != nil {
		return PaymentIntentResult{}, w.createErr
	}
	return PaymentIntentResult{ID: "intent-id", Reference: "PAY-REF-1"}, nil
}

func (w *fakeWallet) PayFromWallet(_ context.Context, _, _ string) error {
	w.payCalls++
	return w.payErr
}

func (w *fakeWallet) RequestRefund(_ context.Context, in RefundInput) error {
	w.refundCalls++
	w.lastRefund = in
	return w.refundErr
}

type fakePublisher struct {
	err       error
	published map[string][][]byte
}

func newFakePublisher() *fakePublisher {
	return &fakePublisher{published: map[string][][]byte{}}
}

func (p *fakePublisher) Publish(_ context.Context, topic string, payload []byte) error {
	if p.err != nil {
		return p.err
	}
	p.published[topic] = append(p.published[topic], payload)
	return nil
}

// --- helpers ----------------------------------------------------------------

func f(v float64) *float64 { return &v }

func validDeliveryReq() DeliveryRequest {
	return DeliveryRequest{
		PickupAddress:  "15 Awolowo Road, Ikoyi, Lagos",
		PickupLat:      f(6.4474),
		PickupLng:      f(3.4343),
		DestAddress:    "32 Bode Thomas, Surulere, Lagos",
		DestLat:        f(6.4969),
		DestLng:        f(3.3481),
		ReceiverName:   "Chidi Obi",
		ReceiverPhone:  "+2348011223344",
		PackageType:    "medical",
		PackageSize:    "small",
		IsFragile:      true,
		ServiceTier:    "express",
		PaymentMethod:  "wallet",
		CustomerID:     "cust-1",
		CustomerEmail:  "customer@pharmacy.com",
		IdempotencyKey: "unique-key-1",
		ExternalRef:    "ORDER-12345",
	}
}

// sequentialIDs returns a NewID generator yielding id-1, id-2, ...
func sequentialIDs() func() string {
	var n int
	var mu sync.Mutex
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		n++
		return "id-" + strconv.Itoa(n)
	}
}

type serviceHarness struct {
	svc  *Service
	repo *fakeBookingRepo
	idem *fakeIdemStore
	wal  *fakeWallet
	pub  *fakePublisher
}

func newHarness() *serviceHarness {
	h := &serviceHarness{
		repo: newFakeBookingRepo(),
		idem: newFakeIdemStore(),
		wal:  &fakeWallet{},
		pub:  newFakePublisher(),
	}
	h.svc = NewService(Deps{
		Bookings:    h.repo,
		Idempotency: h.idem,
		Wallet:      h.wal,
		Publisher:   h.pub,
		Now:         func() time.Time { return time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC) },
		NewID:       sequentialIDs(),
	})
	return h
}

func wantAPIError(t *testing.T, err error, code string, status int) {
	t.Helper()
	var e *APIError
	if !errors.As(err, &e) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if e.Code != code {
		t.Fatalf("error code = %q, want %q", e.Code, code)
	}
	if e.Status != status {
		t.Fatalf("error status = %d, want %d", e.Status, status)
	}
}

// --- quote tests ------------------------------------------------------------

func TestQuoteValidReturnsFare(t *testing.T) {
	h := newHarness()
	q := QuoteRequest{PickupLat: f(6.4474), PickupLng: f(3.4343), DestLat: f(6.4969), DestLng: f(3.3481), ServiceTier: "express"}
	resp, err := h.svc.GetQuote(context.Background(), q)
	if err != nil {
		t.Fatalf("GetQuote() error = %v", err)
	}
	if resp.Fare.Kobo <= 0 {
		t.Fatalf("fare kobo = %d, want > 0", resp.Fare.Kobo)
	}
	if resp.ServiceTier != ServiceTierExpress {
		t.Fatalf("service_tier = %q, want express", resp.ServiceTier)
	}
	if !strings.HasPrefix(resp.Fare.Display, nairaSign) {
		t.Fatalf("fare display = %q, want naira-prefixed", resp.Fare.Display)
	}
}

func TestQuoteDoesNotTouchDBPaymentOrPublisher(t *testing.T) {
	h := newHarness()
	q := QuoteRequest{PickupLat: f(6.4), PickupLng: f(3.4), DestLat: f(6.5), DestLng: f(3.3)}
	if _, err := h.svc.GetQuote(context.Background(), q); err != nil {
		t.Fatalf("GetQuote() error = %v", err)
	}
	if h.repo.createCalls != 0 || h.wal.createCalls != 0 || len(h.pub.published) != 0 {
		t.Fatalf("quote must not write DB/payment/publish: create=%d wallet=%d pub=%d",
			h.repo.createCalls, h.wal.createCalls, len(h.pub.published))
	}
}

func TestQuoteInvalidCoordsReturns400(t *testing.T) {
	h := newHarness()
	q := QuoteRequest{PickupLat: f(200), PickupLng: f(3.4), DestLat: f(6.5), DestLng: f(3.3)}
	_, err := h.svc.GetQuote(context.Background(), q)
	wantAPIError(t, err, "validation_error", 400)
}

// --- create tests -----------------------------------------------------------

func TestCreateMissingFieldsReturns400(t *testing.T) {
	h := newHarness()
	_, _, err := h.svc.CreateDelivery(context.Background(), "partner-1", DeliveryRequest{})
	wantAPIError(t, err, "validation_error", 400)
}

func TestCreateCachedIdempotentReplay(t *testing.T) {
	h := newHarness()
	req := validDeliveryReq()
	key := "partner-1:" + req.IdempotencyKey
	cached := []byte(`{"success":true,"data":{"cached":true}}`)
	_ = h.idem.Put(context.Background(), IdempotencyRecord{
		Key:         key,
		PartnerID:   "partner-1",
		RequestHash: requestHashHex(req),
		Response:    cached,
		StatusCode:  201,
		ExpiresAt:   h.svc.now().Add(time.Hour),
	})
	h.wal.createCalls = 0

	status, body, err := h.svc.CreateDelivery(context.Background(), "partner-1", req)
	if err != nil {
		t.Fatalf("CreateDelivery() error = %v", err)
	}
	if status != 201 || string(body) != string(cached) {
		t.Fatalf("replay status=%d body=%s, want cached", status, body)
	}
	if h.wal.createCalls != 0 || h.repo.createCalls != 0 {
		t.Fatal("cached replay must not create booking or payment intent")
	}
}

func TestCreateIdempotencyConflict(t *testing.T) {
	h := newHarness()
	req := validDeliveryReq()
	key := "partner-1:" + req.IdempotencyKey
	_ = h.idem.Put(context.Background(), IdempotencyRecord{
		Key:         key,
		PartnerID:   "partner-1",
		RequestHash: "a-different-hash",
		Response:    []byte(`{}`),
		StatusCode:  201,
		ExpiresAt:   h.svc.now().Add(time.Hour),
	})
	_, _, err := h.svc.CreateDelivery(context.Background(), "partner-1", req)
	wantAPIError(t, err, "idempotency_conflict", 409)
}

func TestCreatePaymentIntentFailureReturns402NoPublish(t *testing.T) {
	h := newHarness()
	h.wal.createErr = errors.New("wallet down")
	_, _, err := h.svc.CreateDelivery(context.Background(), "partner-1", validDeliveryReq())
	wantAPIError(t, err, "payment_failed", 402)
	if len(h.pub.published) != 0 {
		t.Fatal("payment failure must not publish dispatch event")
	}
}

func TestCreateWalletPaymentCallsPayFromWallet(t *testing.T) {
	h := newHarness()
	if _, _, err := h.svc.CreateDelivery(context.Background(), "partner-1", validDeliveryReq()); err != nil {
		t.Fatalf("CreateDelivery() error = %v", err)
	}
	if h.wal.payCalls != 1 {
		t.Fatalf("PayFromWallet calls = %d, want 1", h.wal.payCalls)
	}
}

func TestCreatePublishFailureReturns503NoIdempotency(t *testing.T) {
	h := newHarness()
	h.pub.err = errors.New("redis down")
	_, _, err := h.svc.CreateDelivery(context.Background(), "partner-1", validDeliveryReq())
	wantAPIError(t, err, "dispatch_unavailable", 503)
	if h.idem.putCalled {
		t.Fatal("publish failure must not store idempotency success")
	}
}

func TestCreateSuccessFullFlow(t *testing.T) {
	h := newHarness()
	status, body, err := h.svc.CreateDelivery(context.Background(), "partner-1", validDeliveryReq())
	if err != nil {
		t.Fatalf("CreateDelivery() error = %v", err)
	}
	if status != 201 {
		t.Fatalf("status = %d, want 201", status)
	}
	if h.repo.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", h.repo.createCalls)
	}
	if h.wal.createCalls != 1 || h.wal.payCalls != 1 {
		t.Fatalf("wallet create=%d pay=%d, want 1/1", h.wal.createCalls, h.wal.payCalls)
	}
	if len(h.pub.published[TopicBookingDispatchCreated]) != 1 {
		t.Fatalf("published created = %d, want 1", len(h.pub.published[TopicBookingDispatchCreated]))
	}
	if !h.idem.putCalled {
		t.Fatal("success must store idempotency")
	}
	// Booking promoted to dispatching in repo.
	// dispatchID=id-1, bookingID=id-2 (sequential).
	b, _ := h.repo.FindByPartnerAndIDOrRef(context.Background(), "partner-1", "id-2")
	if b == nil || b.Status != StatusDispatching {
		t.Fatalf("stored booking status = %v, want dispatching", b)
	}
	if !strings.Contains(string(body), `"status":"dispatching"`) {
		t.Fatalf("response body missing dispatching status: %s", body)
	}
}

func TestCreateBookingRefFormat(t *testing.T) {
	h := newHarness()
	_, body, err := h.svc.CreateDelivery(context.Background(), "partner-1", validDeliveryReq())
	if err != nil {
		t.Fatalf("CreateDelivery() error = %v", err)
	}
	re := regexp.MustCompile(`"booking_ref":"KGP-[A-Z0-9]{8}"`)
	if !re.Match(body) {
		t.Fatalf("booking_ref not in KGP-XXXXXXXX format: %s", body)
	}
}

func TestCreatePaymentSourceUsesDispatchOptionA(t *testing.T) {
	h := newHarness()
	if _, _, err := h.svc.CreateDelivery(context.Background(), "partner-1", validDeliveryReq()); err != nil {
		t.Fatalf("CreateDelivery() error = %v", err)
	}
	if h.wal.lastCreate.SourceService != "dispatch-delivery-service" {
		t.Fatalf("SourceService = %q, want dispatch-delivery-service", h.wal.lastCreate.SourceService)
	}
	// dispatchID is the first generated id (id-1).
	if h.wal.lastCreate.SourceReference != "id-1" {
		t.Fatalf("SourceReference = %q, want id-1 (dispatch_booking_id)", h.wal.lastCreate.SourceReference)
	}
}

func TestCreateRetriesOnRefCollision(t *testing.T) {
	h := newHarness()
	h.repo.failNextRef = 2 // first two ref inserts collide, third succeeds
	if _, _, err := h.svc.CreateDelivery(context.Background(), "partner-1", validDeliveryReq()); err != nil {
		t.Fatalf("CreateDelivery() error = %v", err)
	}
	if h.repo.createCalls != 3 {
		t.Fatalf("createCalls = %d, want 3 (2 collisions + 1 success)", h.repo.createCalls)
	}
}

// --- get/list tests ---------------------------------------------------------

func seedBooking(h *serviceHarness, partnerID, id, ref, status string, paymentRef string) {
	b := &PartnerBooking{
		ID: id, BookingRef: ref, PartnerID: partnerID, Status: status,
		ServiceTier: ServiceTierExpress, FareAmountKobo: 250000, Currency: currencyNGN,
		PickupAddress: "A", DestAddress: "B", ReceiverName: "R",
		DispatchBookingID: strPtr("dispatch-" + id),
		CreatedAt:         h.svc.now(), UpdatedAt: h.svc.now(),
	}
	if paymentRef != "" {
		b.PaymentIntentID = strPtr(paymentRef)
	}
	h.repo.byID[id] = b
}

func strPtr(s string) *string { return &s }

func TestGetByUUIDSamePartner(t *testing.T) {
	h := newHarness()
	seedBooking(h, "partner-1", "uuid-1", "KGP-AAAA1111", StatusInProgress, "")
	d, err := h.svc.GetDelivery(context.Background(), "partner-1", "uuid-1")
	if err != nil {
		t.Fatalf("GetDelivery() error = %v", err)
	}
	if d.ID != "uuid-1" {
		t.Fatalf("id = %q, want uuid-1", d.ID)
	}
}

func TestGetByBookingRefSamePartner(t *testing.T) {
	h := newHarness()
	seedBooking(h, "partner-1", "uuid-1", "KGP-AAAA1111", StatusInProgress, "")
	d, err := h.svc.GetDelivery(context.Background(), "partner-1", "KGP-AAAA1111")
	if err != nil {
		t.Fatalf("GetDelivery() error = %v", err)
	}
	if d.BookingRef != "KGP-AAAA1111" {
		t.Fatalf("booking_ref = %q", d.BookingRef)
	}
}

func TestGetOtherPartnerReturns404(t *testing.T) {
	h := newHarness()
	seedBooking(h, "partner-1", "uuid-1", "KGP-AAAA1111", StatusInProgress, "")
	_, err := h.svc.GetDelivery(context.Background(), "partner-2", "uuid-1")
	wantAPIError(t, err, "booking_not_found", 404)
}

func TestListFiltersByPartnerAndStatus(t *testing.T) {
	h := newHarness()
	seedBooking(h, "partner-1", "u1", "KGP-AAAA0001", StatusDispatching, "")
	seedBooking(h, "partner-1", "u2", "KGP-AAAA0002", StatusCompleted, "")
	seedBooking(h, "partner-2", "u3", "KGP-AAAA0003", StatusDispatching, "")

	resp, err := h.svc.ListDeliveries(context.Background(), "partner-1", 1, 20, "dispatching")
	if err != nil {
		t.Fatalf("ListDeliveries() error = %v", err)
	}
	if resp.Meta.Total != 1 || len(resp.Deliveries) != 1 {
		t.Fatalf("total=%d len=%d, want 1/1", resp.Meta.Total, len(resp.Deliveries))
	}
	if resp.Deliveries[0].ID != "u1" {
		t.Fatalf("id = %q, want u1", resp.Deliveries[0].ID)
	}
}

func TestListInvalidStatusReturns400(t *testing.T) {
	h := newHarness()
	_, err := h.svc.ListDeliveries(context.Background(), "partner-1", 1, 20, "bogus")
	wantAPIError(t, err, "validation_error", 400)
}

// --- cancel tests -----------------------------------------------------------

func TestCancelAllowedStatusesSucceed(t *testing.T) {
	for _, status := range []string{StatusPending, StatusDispatching, StatusAccepted} {
		h := newHarness()
		seedBooking(h, "partner-1", "u1", "KGP-AAAA0001", status, "PAY-REF-1")
		resp, err := h.svc.CancelDelivery(context.Background(), "partner-1", "u1")
		if err != nil {
			t.Fatalf("status %s: CancelDelivery() error = %v", status, err)
		}
		if !resp.Cancelled || resp.Status != StatusCancelled {
			t.Fatalf("status %s: resp = %+v", status, resp)
		}
	}
}

func TestCancelBlockedStatusesReturn409(t *testing.T) {
	for _, status := range []string{StatusInProgress, StatusCompleted, StatusCancelled, StatusFailed, StatusNoProvider} {
		h := newHarness()
		seedBooking(h, "partner-1", "u1", "KGP-AAAA0001", status, "PAY-REF-1")
		_, err := h.svc.CancelDelivery(context.Background(), "partner-1", "u1")
		wantAPIError(t, err, "cannot_cancel", 409)
	}
}

func TestCancelRefundFailurePreventsPublish(t *testing.T) {
	h := newHarness()
	h.wal.refundErr = errors.New("refund failed")
	seedBooking(h, "partner-1", "u1", "KGP-AAAA0001", StatusDispatching, "PAY-REF-1")
	_, err := h.svc.CancelDelivery(context.Background(), "partner-1", "u1")
	wantAPIError(t, err, "payment_failed", 402)
	if len(h.pub.published[TopicBookingDispatchCancelled]) != 0 {
		t.Fatal("refund failure must not publish cancellation")
	}
	b := h.repo.byID["u1"]
	if b.Status == StatusCancelled {
		t.Fatal("booking must not be cancelled when refund fails")
	}
}

func TestCancelSuccessRefundsPublishesUpdates(t *testing.T) {
	h := newHarness()
	seedBooking(h, "partner-1", "u1", "KGP-AAAA0001", StatusAccepted, "PAY-REF-1")
	resp, err := h.svc.CancelDelivery(context.Background(), "partner-1", "u1")
	if err != nil {
		t.Fatalf("CancelDelivery() error = %v", err)
	}
	if h.wal.refundCalls != 1 {
		t.Fatalf("refund calls = %d, want 1", h.wal.refundCalls)
	}
	if h.wal.lastRefund.PaymentReference != "PAY-REF-1" {
		t.Fatalf("refund PaymentReference = %q, want PAY-REF-1", h.wal.lastRefund.PaymentReference)
	}
	if len(h.pub.published[TopicBookingDispatchCancelled]) != 1 {
		t.Fatalf("published cancelled = %d, want 1", len(h.pub.published[TopicBookingDispatchCancelled]))
	}
	if h.repo.byID["u1"].Status != StatusCancelled {
		t.Fatalf("status = %q, want cancelled", h.repo.byID["u1"].Status)
	}
	if !resp.Cancelled {
		t.Fatal("resp.Cancelled = false, want true")
	}
}
