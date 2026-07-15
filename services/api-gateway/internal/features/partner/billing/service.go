package billing

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

const (
	currencyNGN = "NGN"

	// sourceService is the source_service api-gateway sends to payment-wallet.
	sourceService = "api-gateway"
	// dueInvoiceBatchSize bounds one charge-loop pass.
	dueInvoiceBatchSize = 50
)

// Options configures a Service. The dashboard/monthly invoice fields and the
// charge-loop fields are separate so tests and deployments can wire either side
// independently, while production wires both.
type Options struct {
	Invoices            InvoiceRepository
	PaymentMethods      PaymentMethodRepository
	Usage               DeliveryUsageReader
	Partners            PartnerLister
	RatePerDeliveryKobo int64

	ChargeRepo ChargeRepository
	Wallet     ChargeClient

	Now   func() time.Time
	NewID func() string
}

type Service struct {
	invoices InvoiceRepository
	payments PaymentMethodRepository
	usage    DeliveryUsageReader
	partners PartnerLister
	rateKobo int64

	charges ChargeRepository
	wallet  ChargeClient

	now   func() time.Time
	newID func() string
}

func NewService(o Options) *Service {
	s := &Service{
		invoices: o.Invoices,
		payments: o.PaymentMethods,
		usage:    o.Usage,
		partners: o.Partners,
		rateKobo: o.RatePerDeliveryKobo,
		charges:  o.ChargeRepo,
		wallet:   o.Wallet,
		now:      o.Now,
		newID:    o.NewID,
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.newID == nil {
		s.newID = uuid.NewString
	}
	return s
}

// GetOverview assembles the Billing & Usage screen in one call.
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


func (s *Service) GetDashboardSummary(ctx context.Context, partnerID string) (*DashboardSummary, error) {
	summary, err := s.usage.GetDashboardSummary(ctx, partnerID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	return &summary, nil
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

// AttachPaymentMethodInput is the POST /billing/payment-methods body. The
// frontend must tokenize the card first and send only the authorization_code.
type AttachPaymentMethodInput struct {
	AuthorizationCode string `json:"authorization_code"`
	CardBrand         string `json:"card_brand"`
	Last4             string `json:"last4"`
	ExpMonth          string `json:"exp_month"`
	ExpYear           string `json:"exp_year"`
}

// AttachPaymentMethod stores a payment method reference. It never touches raw
// card data, only a Paystack authorization_code.
func (s *Service) AttachPaymentMethod(ctx context.Context, partnerID string, in AttachPaymentMethodInput) (*PaymentMethodView, error) {
	code := strings.TrimSpace(in.AuthorizationCode)
	if code == "" {
		return nil, delivery.NewAPIError(http.StatusBadRequest, "validation_error", "authorization_code is required.", nil)
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
// [periodStart, periodEnd), based on completed delivery count in that range.
func (s *Service) GenerateInvoiceForPeriod(ctx context.Context, partnerID string, periodStart, periodEnd time.Time) (*Invoice, error) {
	count, err := s.usage.CountCompletedInRange(ctx, partnerID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	now := s.now()
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
		DueAt:               &now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := s.invoices.Create(ctx, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

// RunMonthlyInvoiceWorker creates prior-month invoices on the first day of each
// month, polling at the supplied interval until ctx is cancelled.
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
		return
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

// RunDueInvoiceCharges finds due unpaid invoices and initiates wallet charges.
// One invoice failing never aborts the loop.
func (s *Service) RunDueInvoiceCharges(ctx context.Context) error {
	if s.charges == nil || s.wallet == nil {
		return nil
	}
	invoices, err := s.charges.FindDueUnpaidInvoices(ctx, s.now(), dueInvoiceBatchSize)
	if err != nil {
		return err
	}
	for i := range invoices {
		if err := s.chargeInvoice(ctx, &invoices[i]); err != nil {
			log.Printf("billing: invoice %s charge failed: %v", invoices[i].ID, err)
		}
	}
	return nil
}

// ChargePendingInvoices is the named billing entrypoint used by the partner
// billing flow. It delegates to RunDueInvoiceCharges.
func (s *Service) ChargePendingInvoices(ctx context.Context) error {
	return s.RunDueInvoiceCharges(ctx)
}

func (s *Service) chargeInvoice(ctx context.Context, inv *Invoice) error {
	pending, err := s.charges.HasPendingChargeAttempt(ctx, inv.ID)
	if err != nil {
		return err
	}
	if pending {
		return nil
	}

	auth, err := s.charges.FindActiveAuthorization(ctx, inv.PartnerID)
	if err != nil {
		return err
	}
	if auth == nil {
		log.Printf("billing: invoice %s skipped: partner %s has no active billing authorization", inv.ID, inv.PartnerID)
		return nil
	}

	attempt := &ChargeAttempt{
		ID:             s.newID(),
		InvoiceID:      inv.ID,
		PartnerID:      inv.PartnerID,
		IdempotencyKey: "charge-" + inv.ID,
		Status:         AttemptStatusPending,
		AmountKobo:     inv.AmountKobo,
		CreatedAt:      s.now(),
		UpdatedAt:      s.now(),
	}
	if err := s.charges.CreateChargeAttempt(ctx, attempt); err != nil {
		return err
	}

	result, err := s.wallet.ChargeAuthorization(ctx, ChargeAuthorizationInput{
		SourceService:     sourceService,
		SourceReference:   inv.ID,
		Email:             auth.Email,
		AmountKobo:        inv.AmountKobo,
		AuthorizationCode: auth.AuthorizationCode,
		IdempotencyKey:    attempt.IdempotencyKey,
	})
	if err != nil {
		_ = s.charges.ResolveLatestChargeAttempt(ctx, inv.ID, AttemptStatusFailed, "", err.Error())
		_ = s.charges.MarkInvoiceChargeFailed(ctx, inv.ID, "", err.Error())
		return err
	}

	_ = s.charges.SetChargeAttemptReference(ctx, inv.ID, result.Reference)
	return s.charges.MarkInvoiceCharging(ctx, inv.ID, result.Reference)
}

// RunChargeWorker polls due invoices on an interval until ctx is cancelled.
func (s *Service) RunChargeWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RunDueInvoiceCharges(ctx); err != nil {
				log.Printf("billing: charge worker error: %v", err)
			}
		}
	}
}

// HandleChargeResult applies a payment-wallet charge-result callback.
func (s *Service) HandleChargeResult(ctx context.Context, in ChargeResultInput) (*ChargeResultOutcome, error) {
	if s.charges == nil {
		return nil, delivery.ErrInternal()
	}
	invoiceID := strings.TrimSpace(in.SourceReference)
	if invoiceID == "" {
		return nil, delivery.ValidationError([]delivery.FieldError{{Field: "source_reference", Message: "This field is required."}})
	}
	status := strings.ToLower(strings.TrimSpace(in.Status))
	if status != ChargeResultSuccess && status != ChargeResultFailed {
		return nil, delivery.ValidationError([]delivery.FieldError{{Field: "status", Message: "Must be one of: success, failed."}})
	}

	inv, err := s.charges.FindInvoiceByID(ctx, invoiceID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if inv == nil {
		return &ChargeResultOutcome{Skipped: true}, nil
	}

	if in.AmountKobo != 0 && in.AmountKobo != inv.AmountKobo {
		return nil, delivery.NewAPIError(http.StatusBadRequest, "amount_mismatch", "The charge amount does not match the invoice amount.", nil)
	}

	switch status {
	case ChargeResultSuccess:
		if inv.Status == InvoiceStatusPaid {
			return &ChargeResultOutcome{InvoiceID: inv.ID, Status: string(InvoiceStatusPaid)}, nil
		}
		if err := s.charges.ResolveLatestChargeAttempt(ctx, inv.ID, AttemptStatusSuccess, in.Reference, ""); err != nil {
			return nil, delivery.ErrInternal()
		}
		if err := s.charges.MarkInvoicePaid(ctx, inv.ID, in.Reference, s.now()); err != nil {
			return nil, delivery.ErrInternal()
		}
		return &ChargeResultOutcome{InvoiceID: inv.ID, Status: string(InvoiceStatusPaid)}, nil
	default:
		if inv.Status == InvoiceStatusPaid {
			return &ChargeResultOutcome{InvoiceID: inv.ID, Status: string(InvoiceStatusPaid)}, nil
		}
		if err := s.charges.ResolveLatestChargeAttempt(ctx, inv.ID, AttemptStatusFailed, in.Reference, "charge failed"); err != nil {
			return nil, delivery.ErrInternal()
		}
		if err := s.charges.MarkInvoiceChargeFailed(ctx, inv.ID, in.Reference, "charge failed"); err != nil {
			return nil, delivery.ErrInternal()
		}
		return &ChargeResultOutcome{InvoiceID: inv.ID, Status: string(InvoiceStatusChargeFailed)}, nil
	}
}

// GetInvoice returns one partner invoice, partner-scoped.
func (s *Service) GetInvoice(ctx context.Context, partnerID, invoiceID string) (*InvoiceView, error) {
	if s.charges == nil {
		return nil, delivery.ErrInternal()
	}
	inv, err := s.charges.FindInvoiceByPartnerAndID(ctx, partnerID, strings.TrimSpace(invoiceID))
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if inv == nil {
		return nil, delivery.NewAPIError(http.StatusNotFound, "invoice_not_found", "No invoice was found for this account.", nil)
	}
	v := invoiceViewOf(inv)
	return &v, nil
}

// --- view helpers --------------------------------------------------------

func invoiceViewOf(inv *Invoice) InvoiceView {
	fmtTime := func(t *time.Time) *string {
		if t == nil {
			return nil
		}
		s := t.UTC().Format(time.RFC3339)
		return &s
	}
	period := ""
	if !inv.PeriodStart.IsZero() {
		period = inv.PeriodStart.Format("Jan 2006")
	}
	return InvoiceView{
		ID:              inv.ID,
		InvoiceNumber:   inv.InvoiceNumber,
		Period:          period,
		AmountKobo:      inv.AmountKobo,
		AmountDisplay:   formatNaira(inv.AmountKobo),
		Currency:        inv.Currency,
		Status:          capitalize(string(inv.Status)),
		DueAt:           fmtTime(inv.DueAt),
		PaidAt:          fmtTime(inv.PaidAt),
		WalletReference: inv.WalletReference,
		FailureReason:   inv.FailureReason,
		CreatedAt:       inv.CreatedAt.UTC().Format(time.RFC3339),
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
