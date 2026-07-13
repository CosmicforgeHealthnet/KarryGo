package billing

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

const currencyNGN = "NGN"

// Options configures a Service. RatePerDeliveryKobo is a single fixed rate
// applied to every partner (not per-partner pricing) - e.g. sourced from a
// BILLING_RATE_PER_DELIVERY_KOBO env var at wiring time, the same way
// delivery.Service already takes its fare pricing via pricing.Config.
type Options struct {
	Invoices            InvoiceRepository
	PaymentMethods      PaymentMethodRepository
	Usage               DeliveryUsageReader
	Partners            PartnerLister
	RatePerDeliveryKobo int64
	Now                 func() time.Time
}

type Service struct {
	invoices  InvoiceRepository
	payments  PaymentMethodRepository
	usage     DeliveryUsageReader
	partners  PartnerLister
	rateKobo  int64
	now       func() time.Time
}

func NewService(o Options) *Service {
	s := &Service{
		invoices: o.Invoices,
		payments: o.PaymentMethods,
		usage:    o.Usage,
		partners: o.Partners,
		rateKobo: o.RatePerDeliveryKobo,
		now:      o.Now,
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// GetOverview assembles the whole Billing & Usage screen in one call:
// running-balance figures, current plan/status, this-month usage (computed
// live from partner_bookings via Usage, never a stored counter that could
// drift), and saved payment methods.
func (s *Service) GetOverview(ctx context.Context, partnerID, plan, status string) (*Overview, error) {
	now := s.now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	monthEnd := monthStart.AddDate(0, 1, 0)

	count, err := s.usage.CountCompletedInRange(ctx, partnerID, monthStart, monthEnd)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	thisMonthKobo := int64(count) * s.rateKobo

	totalBilled, err := s.invoices.SumPaidByPartner(ctx, partnerID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	methods, err := s.payments.ListByPartner(ctx, partnerID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	methodViews := make([]PaymentMethodView, 0, len(methods))
	for _, m := range methods {
		methodViews = append(methodViews, paymentMethodViewOf(&m))
	}

	return &Overview{
		// CurrentBalanceKobo is what's owed but not yet invoiced - this month's
		// running usage, since no invoice has been cut for the current month yet
		// (invoices are only generated for a month once it has fully ended).
		CurrentBalanceKobo:   thisMonthKobo,
		ThisMonthChargesKobo: thisMonthKobo,
		TotalBilledKobo:      totalBilled,
		CurrentPlan:          planLabel(plan),
		BillingModel:         "Per deliveries",
		Status:               statusLabel(status),
		Usage: UsageSummary{
			DeliveriesThisMonth:    count,
			BillableDeliveries:     count,
			RatePerDeliveryKobo:    s.rateKobo,
			RatePerDeliveryDisplay: formatNaira(s.rateKobo),
			TotalCostKobo:          thisMonthKobo,
			TotalCostDisplay:       formatNaira(thisMonthKobo),
		},
		PaymentMethods: methodViews,
	}, nil
}

// ListInvoices returns a page of billing history.
func (s *Service) ListInvoices(ctx context.Context, partnerID string, page, limit int) ([]InvoiceView, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	items, total, err := s.invoices.ListByPartner(ctx, partnerID, limit, offset)
	if err != nil {
		return nil, 0, delivery.ErrInternal()
	}
	views := make([]InvoiceView, 0, len(items))
	for i := range items {
		views = append(views, invoiceViewOf(&items[i]))
	}
	return views, total, nil
}

// AttachPaymentMethodInput is the POST /billing/payment-methods body. See the
// package doc comment: the frontend must tokenize the card via Paystack
// Inline/Popup FIRST and send only the resulting authorization_code here.
type AttachPaymentMethodInput struct {
	AuthorizationCode string `json:"authorization_code"`
	CardBrand         string `json:"card_brand"`
	Last4             string `json:"last4"`
	ExpMonth          string `json:"exp_month"`
	ExpYear           string `json:"exp_year"`
}

// AttachPaymentMethod stores a payment method reference. It never touches raw
// card data - only a Paystack authorization_code.
func (s *Service) AttachPaymentMethod(ctx context.Context, partnerID string, in AttachPaymentMethodInput) (*PaymentMethodView, error) {
	code := strings.TrimSpace(in.AuthorizationCode)
	if code == "" {
		return nil, delivery.NewAPIError(400, "validation_error", "authorization_code is required.", nil)
	}
	pm, err := s.payments.Create(ctx, &PaymentMethod{
		PartnerID:         partnerID,
		Provider:          "paystack",
		AuthorizationCode: code,
		CardBrand:         in.CardBrand,
		Last4:             in.Last4,
		ExpMonth:          in.ExpMonth,
		ExpYear:           in.ExpYear,
		IsDefault:         true,
	})
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	view := paymentMethodViewOf(pm)
	return &view, nil
}

// GenerateInvoiceForPeriod creates one pending invoice for a partner covering
// [periodStart, periodEnd), based on their completed delivery count in that
// range. Returns (nil, nil) if the partner had zero completed deliveries (no
// invoice is created for an empty period). The unique (partner_id,
// period_start) constraint on partner_invoices prevents double-billing the
// same period if this runs more than once.
func (s *Service) GenerateInvoiceForPeriod(ctx context.Context, partnerID string, periodStart, periodEnd time.Time) (*Invoice, error) {
	count, err := s.usage.CountCompletedInRange(ctx, partnerID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	inv := &Invoice{
		PartnerID:           partnerID,
		InvoiceNumber:       fmt.Sprintf("INV-%s", periodStart.Format("200601")),
		PeriodStart:         periodStart,
		PeriodEnd:           periodEnd,
		DeliveryCount:       count,
		RatePerDeliveryKobo: s.rateKobo,
		AmountKobo:          int64(count) * s.rateKobo,
		Currency:            currencyNGN,
		Status:              InvoiceStatusPending,
		CreatedAt:           s.now(),
	}
	if err := s.invoices.Create(ctx, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

// RunMonthlyInvoiceWorker polls once per interval (e.g. every hour) until ctx
// is cancelled, and on the first day of a new month generates one invoice per
// active partner for the month that just ended. Mirrors the polling pattern
// webhooks.Service.RunRetryWorker already uses elsewhere in this codebase.
//
// NOTE: this only creates 'pending' invoice records. It does not attempt to
// charge the partner's saved Paystack authorization_code - that needs its own
// charge/retry/failure-handling design and ideally reuses
// payment-wallet-service's existing Paystack flow rather than duplicating
// payment logic in api-gateway. Wiring this worker into cmd/main.go (as a
// goroutine, same as the webhook retry worker) is also not done here.
func (s *Service) RunMonthlyInvoiceWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runMonthlyBillingPass(ctx)
		}
	}
}

func (s *Service) runMonthlyBillingPass(ctx context.Context) {
	if s.partners == nil {
		return
	}
	now := s.now()
	if now.Day() != 1 {
		return // only bill once, on the first day of a new month
	}
	periodEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodStart := periodEnd.AddDate(0, -1, 0)

	ids, err := s.partners.ListActivePartnerIDs(ctx)
	if err != nil {
		return
	}
	for _, pid := range ids {
		_, _ = s.GenerateInvoiceForPeriod(ctx, pid, periodStart, periodEnd)
	}
}

// --- view helpers --------------------------------------------------------

func invoiceViewOf(inv *Invoice) InvoiceView {
	return InvoiceView{
		ID:            inv.ID,
		InvoiceNumber: inv.InvoiceNumber,
		Period:        inv.PeriodStart.Format("Jan 2006"),
		AmountKobo:    inv.AmountKobo,
		AmountDisplay: formatNaira(inv.AmountKobo),
		Status:        capitalize(string(inv.Status)),
		CreatedAt:     inv.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func paymentMethodViewOf(pm *PaymentMethod) PaymentMethodView {
	return PaymentMethodView{
		ID:        pm.ID,
		CardBrand: pm.CardBrand,
		Last4:     pm.Last4,
		ExpMonth:  pm.ExpMonth,
		ExpYear:   pm.ExpYear,
		IsDefault: pm.IsDefault,
		CreatedAt: pm.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func planLabel(plan string) string {
	if plan == "live" {
		return "Live"
	}
	return "Standard"
}

func statusLabel(status string) string {
	if status == "" {
		return "Active"
	}
	return capitalize(status)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// formatNaira duplicates delivery package's unexported formatNaira - not
// worth exporting a 5-line utility across packages for this.
func formatNaira(kobo int64) string {
	naira := kobo / 100
	remainder := kobo % 100
	if remainder < 0 {
		remainder = -remainder
	}
	return fmt.Sprintf("\u20A6%s.%02d", formatThousands(naira), remainder)
}

func formatThousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}