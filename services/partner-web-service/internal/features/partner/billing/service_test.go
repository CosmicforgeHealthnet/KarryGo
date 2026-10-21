package billing

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/delivery"
)

// --- fakes ------------------------------------------------------------------

type fakeRepo struct {
	mu        sync.Mutex
	invoices  map[string]*Invoice
	auths     map[string]*Authorization // by partner id
	attempts  []*ChargeAttempt
	createErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{invoices: map[string]*Invoice{}, auths: map[string]*Authorization{}}
}

func (r *fakeRepo) FindDueUnpaidInvoices(_ context.Context, now time.Time, _ int) ([]Invoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Invoice
	for _, inv := range r.invoices {
		if (inv.Status == InvoiceStatusPending || inv.Status == InvoiceStatusChargeFailed) &&
			inv.DueAt != nil && !inv.DueAt.After(now) {
			out = append(out, *inv)
		}
	}
	return out, nil
}

func (r *fakeRepo) FindInvoiceByID(_ context.Context, id string) (*Invoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inv, ok := r.invoices[id]; ok {
		cp := *inv
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeRepo) FindInvoiceByPartnerAndID(_ context.Context, partnerID, id string) (*Invoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inv, ok := r.invoices[id]; ok && inv.PartnerID == partnerID {
		cp := *inv
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeRepo) Create(_ context.Context, inv *Invoice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *inv
	r.invoices[inv.ID] = &cp
	return nil
}

func (r *fakeRepo) ListByPartner(_ context.Context, partnerID string, limit, offset int) ([]Invoice, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []Invoice
	for _, inv := range r.invoices {
		if inv.PartnerID == partnerID {
			all = append(all, *inv)
		}
	}
	total := len(all)
	if offset > total {
		return []Invoice{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (r *fakeRepo) SumPaidByPartner(_ context.Context, partnerID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total int64
	for _, inv := range r.invoices {
		if inv.PartnerID == partnerID && inv.Status == InvoiceStatusPaid {
			total += inv.AmountKobo
		}
	}
	return total, nil
}

func (r *fakeRepo) ListInvoicesByPartner(_ context.Context, partnerID string) ([]Invoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Invoice
	for _, inv := range r.invoices {
		if inv.PartnerID == partnerID {
			out = append(out, *inv)
		}
	}
	return out, nil
}

func (r *fakeRepo) MarkInvoiceCharging(_ context.Context, id, ref string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inv, ok := r.invoices[id]; ok {
		inv.Status = InvoiceStatusCharging
		if ref != "" {
			inv.WalletReference = &ref
		}
	}
	return nil
}

func (r *fakeRepo) MarkInvoicePaid(_ context.Context, id, ref string, paidAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inv, ok := r.invoices[id]; ok {
		inv.Status = InvoiceStatusPaid
		if inv.PaidAt == nil {
			inv.PaidAt = &paidAt
		}
		if ref != "" {
			inv.WalletReference = &ref
		}
	}
	return nil
}

func (r *fakeRepo) MarkInvoiceChargeFailed(_ context.Context, id, ref, failureReason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inv, ok := r.invoices[id]; ok && inv.Status != InvoiceStatusPaid {
		inv.Status = InvoiceStatusChargeFailed
		if ref != "" {
			inv.WalletReference = &ref
		}
		if failureReason != "" {
			inv.FailureReason = &failureReason
		}
	}
	return nil
}

func (r *fakeRepo) FindActiveAuthorization(_ context.Context, partnerID string) (*Authorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.auths[partnerID]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeRepo) HasPendingChargeAttempt(_ context.Context, invoiceID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.attempts {
		if a.InvoiceID == invoiceID && a.Status == AttemptStatusPending {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeRepo) CreateChargeAttempt(_ context.Context, a *ChargeAttempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	for _, existing := range r.attempts {
		if existing.IdempotencyKey == a.IdempotencyKey {
			existing.Status = a.Status
			existing.AmountKobo = a.AmountKobo
			existing.WalletReference = nil
			existing.FailureReason = nil
			existing.UpdatedAt = a.UpdatedAt
			return nil
		}
	}
	cp := *a
	r.attempts = append(r.attempts, &cp)
	return nil
}

func (r *fakeRepo) SetChargeAttemptReference(_ context.Context, invoiceID, ref string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.attempts {
		if a.InvoiceID == invoiceID && a.Status == AttemptStatusPending && ref != "" {
			a.WalletReference = &ref
		}
	}
	return nil
}

func (r *fakeRepo) ResolveLatestChargeAttempt(_ context.Context, invoiceID, status, ref, failureReason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.attempts) - 1; i >= 0; i-- {
		a := r.attempts[i]
		if a.InvoiceID == invoiceID && a.Status == AttemptStatusPending {
			a.Status = status
			if ref != "" {
				a.WalletReference = &ref
			}
			if failureReason != "" {
				a.FailureReason = &failureReason
			}
			return nil
		}
	}
	return nil
}

type fakeChargeClient struct {
	mu     sync.Mutex
	calls  []ChargeAuthorizationInput
	err    error
	errFor map[string]error // per source_reference
}

func (c *fakeChargeClient) ChargeAuthorization(_ context.Context, in ChargeAuthorizationInput) (ChargeAuthorizationResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, in)
	if err, ok := c.errFor[in.SourceReference]; ok && err != nil {
		return ChargeAuthorizationResult{}, err
	}
	if c.err != nil {
		return ChargeAuthorizationResult{}, c.err
	}
	return ChargeAuthorizationResult{Reference: "karrygo_charge_ref_" + in.SourceReference, Status: "pending"}, nil
}

// --- harness ------------------------------------------------------------------

type billingHarness struct {
	svc    *Service
	repo   *fakeRepo
	wallet *fakeChargeClient
	now    time.Time
}

func newBillingHarness() *billingHarness {
	h := &billingHarness{
		repo:   newFakeRepo(),
		wallet: &fakeChargeClient{errFor: map[string]error{}},
		now:    time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC),
	}
	var n int
	h.svc = NewService(Options{
		ChargeRepo: h.repo,
		Wallet:     h.wallet,
		Invoices:   h.repo,
		Now:        func() time.Time { return h.now },
		NewID: func() string {
			n++
			return "id-" + strconv.Itoa(n)
		},
	})
	return h
}

func (h *billingHarness) seedInvoice(id, partnerID string, status InvoiceStatus, amountKobo int64, due time.Time) {
	h.repo.invoices[id] = &Invoice{
		ID: id, PartnerID: partnerID, AmountKobo: amountKobo, Currency: "NGN",
		Status: status, DueAt: &due, CreatedAt: h.now, UpdatedAt: h.now,
	}
}

func (h *billingHarness) seedAuth(partnerID, email, code string) {
	h.repo.auths[partnerID] = &Authorization{
		ID: "auth-" + partnerID, PartnerID: partnerID, Email: email,
		AuthorizationCode: code, Status: "active",
	}
}

// --- charge loop tests --------------------------------------------------------

func TestRunDueInvoiceChargesCallsWalletAndStoresPendingAttempt(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusPending, 500000, h.now.Add(-time.Hour))
	h.seedAuth("p1", "partner@company.com", "AUTH_xxxxxxxxx")

	if err := h.svc.RunDueInvoiceCharges(context.Background()); err != nil {
		t.Fatalf("RunDueInvoiceCharges() error = %v", err)
	}
	if len(h.wallet.calls) != 1 {
		t.Fatalf("wallet calls = %d, want 1", len(h.wallet.calls))
	}
	call := h.wallet.calls[0]
	if call.SourceService != "api-gateway" || call.SourceReference != "inv-1" ||
		call.Email != "partner@company.com" || call.AmountKobo != 500000 ||
		call.AuthorizationCode != "AUTH_xxxxxxxxx" {
		t.Fatalf("wallet call = %+v", call)
	}
	if call.IdempotencyKey != "charge-inv-1" {
		t.Fatalf("idempotency key = %q, want charge-inv-1", call.IdempotencyKey)
	}
	if len(h.repo.attempts) != 1 || h.repo.attempts[0].Status != AttemptStatusPending {
		t.Fatalf("attempts = %+v, want one pending", h.repo.attempts)
	}
	if h.repo.attempts[0].WalletReference == nil || *h.repo.attempts[0].WalletReference != "karrygo_charge_ref_inv-1" {
		t.Fatalf("attempt wallet reference = %v", h.repo.attempts[0].WalletReference)
	}
	if h.repo.invoices["inv-1"].Status != InvoiceStatusCharging {
		t.Fatalf("invoice status = %q, want charging", h.repo.invoices["inv-1"].Status)
	}
}

func TestChargePendingInvoicesDelegatesToDueInvoiceChargeLoop(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusPending, 500000, h.now.Add(-time.Hour))
	h.seedAuth("p1", "partner@company.com", "AUTH_x")

	if err := h.svc.ChargePendingInvoices(context.Background()); err != nil {
		t.Fatalf("ChargePendingInvoices() error = %v", err)
	}
	if len(h.wallet.calls) != 1 || h.wallet.calls[0].IdempotencyKey != "charge-inv-1" {
		t.Fatalf("wallet calls = %+v", h.wallet.calls)
	}
}
func TestRunDueInvoiceChargesSkipsPendingAttempt(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusPending, 500000, h.now.Add(-time.Hour))
	h.seedAuth("p1", "partner@company.com", "AUTH_x")
	h.repo.attempts = append(h.repo.attempts, &ChargeAttempt{
		ID: "a1", InvoiceID: "inv-1", PartnerID: "p1", IdempotencyKey: "k1", Status: AttemptStatusPending,
	})

	if err := h.svc.RunDueInvoiceCharges(context.Background()); err != nil {
		t.Fatalf("RunDueInvoiceCharges() error = %v", err)
	}
	if len(h.wallet.calls) != 0 {
		t.Fatalf("wallet calls = %d, want 0 (pending attempt in flight)", len(h.wallet.calls))
	}
}

func TestRunDueInvoiceChargesSkipsPartnersWithoutAuthorization(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusPending, 500000, h.now.Add(-time.Hour))
	// No authorization seeded.
	if err := h.svc.RunDueInvoiceCharges(context.Background()); err != nil {
		t.Fatalf("RunDueInvoiceCharges() error = %v", err)
	}
	if len(h.wallet.calls) != 0 || len(h.repo.attempts) != 0 {
		t.Fatal("must not charge without a saved authorization")
	}
}

func TestRunDueInvoiceChargesOneFailureDoesNotAbortLoop(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusPending, 100, h.now.Add(-time.Hour))
	h.seedInvoice("inv-2", "p2", InvoiceStatusPending, 200, h.now.Add(-time.Hour))
	h.seedAuth("p1", "a@x.com", "AUTH_a")
	h.seedAuth("p2", "b@x.com", "AUTH_b")
	h.wallet.errFor["inv-1"] = errors.New("wallet down")

	if err := h.svc.RunDueInvoiceCharges(context.Background()); err != nil {
		t.Fatalf("RunDueInvoiceCharges() error = %v", err)
	}
	if len(h.wallet.calls) != 2 {
		t.Fatalf("wallet calls = %d, want 2 (both invoices tried)", len(h.wallet.calls))
	}
	if h.repo.invoices["inv-1"].Status != InvoiceStatusChargeFailed {
		t.Fatalf("inv-1 status = %q, want charge_failed", h.repo.invoices["inv-1"].Status)
	}
	if h.repo.invoices["inv-2"].Status != InvoiceStatusCharging {
		t.Fatalf("inv-2 status = %q, want charging", h.repo.invoices["inv-2"].Status)
	}
}

// --- charge-result tests --------------------------------------------------------

func TestHandleChargeResultSuccessMarksPaid(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusCharging, 500000, h.now.Add(-time.Hour))
	h.repo.attempts = append(h.repo.attempts, &ChargeAttempt{
		ID: "a1", InvoiceID: "inv-1", PartnerID: "p1", IdempotencyKey: "k1", Status: AttemptStatusPending, AmountKobo: 500000,
	})

	out, err := h.svc.HandleChargeResult(context.Background(), ChargeResultInput{
		SourceReference: "inv-1", Status: "success", Reference: "paystack_ref", AmountKobo: 500000,
	})
	if err != nil {
		t.Fatalf("HandleChargeResult() error = %v", err)
	}
	if out.Status != string(InvoiceStatusPaid) {
		t.Fatalf("outcome = %+v, want paid", out)
	}
	inv := h.repo.invoices["inv-1"]
	if inv.Status != InvoiceStatusPaid || inv.PaidAt == nil || !inv.PaidAt.Equal(h.now) {
		t.Fatalf("invoice = %+v", inv)
	}
	if inv.WalletReference == nil || *inv.WalletReference != "paystack_ref" {
		t.Fatalf("wallet reference = %v", inv.WalletReference)
	}
	if h.repo.attempts[0].Status != AttemptStatusSuccess {
		t.Fatalf("attempt status = %q, want success", h.repo.attempts[0].Status)
	}
}

func TestHandleChargeResultFailedMarksChargeFailed(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusCharging, 500000, h.now.Add(-time.Hour))
	h.repo.attempts = append(h.repo.attempts, &ChargeAttempt{
		ID: "a1", InvoiceID: "inv-1", PartnerID: "p1", IdempotencyKey: "k1", Status: AttemptStatusPending, AmountKobo: 500000,
	})

	out, err := h.svc.HandleChargeResult(context.Background(), ChargeResultInput{
		SourceReference: "inv-1", Status: "failed", Reference: "paystack_ref_failed", AmountKobo: 500000,
	})
	if err != nil {
		t.Fatalf("HandleChargeResult() error = %v", err)
	}
	if out.Status != string(InvoiceStatusChargeFailed) {
		t.Fatalf("outcome = %+v, want charge_failed", out)
	}
	if h.repo.invoices["inv-1"].Status != InvoiceStatusChargeFailed {
		t.Fatalf("invoice status = %q", h.repo.invoices["inv-1"].Status)
	}
	if h.repo.attempts[0].Status != AttemptStatusFailed {
		t.Fatalf("attempt status = %q, want failed", h.repo.attempts[0].Status)
	}
}

func TestHandleChargeResultAmountMismatchRejected(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusCharging, 500000, h.now.Add(-time.Hour))
	_, err := h.svc.HandleChargeResult(context.Background(), ChargeResultInput{
		SourceReference: "inv-1", Status: "success", Reference: "r", AmountKobo: 400000,
	})
	var apiErr *delivery.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "amount_mismatch" || apiErr.Status != 400 {
		t.Fatalf("error = %v, want 400 amount_mismatch", err)
	}
	if h.repo.invoices["inv-1"].Status == InvoiceStatusPaid {
		t.Fatal("mismatched amount must not mark invoice paid")
	}
}

func TestHandleChargeResultUnknownStatusRejected(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusCharging, 500000, h.now.Add(-time.Hour))
	_, err := h.svc.HandleChargeResult(context.Background(), ChargeResultInput{
		SourceReference: "inv-1", Status: "bogus", Reference: "r", AmountKobo: 500000,
	})
	var apiErr *delivery.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 400 {
		t.Fatalf("error = %v, want 400 validation error", err)
	}
}

func TestHandleChargeResultMissingSourceReferenceRejected(t *testing.T) {
	h := newBillingHarness()
	_, err := h.svc.HandleChargeResult(context.Background(), ChargeResultInput{Status: "success"})
	var apiErr *delivery.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 400 {
		t.Fatalf("error = %v, want 400 validation error", err)
	}
}

func TestHandleChargeResultUnknownInvoiceSkipped(t *testing.T) {
	h := newBillingHarness()
	out, err := h.svc.HandleChargeResult(context.Background(), ChargeResultInput{
		SourceReference: "not-an-invoice", Status: "success", AmountKobo: 1,
	})
	if err != nil {
		t.Fatalf("HandleChargeResult() error = %v", err)
	}
	if !out.Skipped {
		t.Fatalf("outcome = %+v, want skipped", out)
	}
}

func TestHandleChargeResultDuplicateSuccessIdempotent(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusCharging, 500000, h.now.Add(-time.Hour))
	in := ChargeResultInput{SourceReference: "inv-1", Status: "success", Reference: "paystack_ref", AmountKobo: 500000}

	if _, err := h.svc.HandleChargeResult(context.Background(), in); err != nil {
		t.Fatalf("first HandleChargeResult() error = %v", err)
	}
	firstPaidAt := *h.repo.invoices["inv-1"].PaidAt

	// Move the clock; a duplicate success must not change paid_at.
	h.now = h.now.Add(2 * time.Hour)
	if _, err := h.svc.HandleChargeResult(context.Background(), in); err != nil {
		t.Fatalf("duplicate HandleChargeResult() error = %v", err)
	}
	if !h.repo.invoices["inv-1"].PaidAt.Equal(firstPaidAt) {
		t.Fatalf("paid_at changed on duplicate success: %v -> %v", firstPaidAt, h.repo.invoices["inv-1"].PaidAt)
	}
}

// --- view tests -----------------------------------------------------------------

func TestInvoiceViewsNeverContainAuthorizationCode(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusPending, 500000, h.now)
	h.seedAuth("p1", "partner@company.com", "AUTH_secret_code")

	views, _, err := h.svc.ListInvoices(context.Background(), "p1", 1, 20)
	if err != nil {
		t.Fatalf("ListInvoices() error = %v", err)
	}
	raw, err := json.Marshal(views)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "AUTH_secret_code") || strings.Contains(string(raw), "authorization_code") {
		t.Fatalf("invoice view leaked authorization data: %s", raw)
	}
}

func TestGetInvoicePartnerScoped(t *testing.T) {
	h := newBillingHarness()
	h.seedInvoice("inv-1", "p1", InvoiceStatusPending, 500000, h.now)
	if _, err := h.svc.GetInvoice(context.Background(), "p2", "inv-1"); err == nil {
		t.Fatal("partner p2 must not see p1's invoice")
	}
	view, err := h.svc.GetInvoice(context.Background(), "p1", "inv-1")
	if err != nil || view.ID != "inv-1" {
		t.Fatalf("GetInvoice() = %+v, %v", view, err)
	}
}
