package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/money"
)

// EventJobPaymentChanged announces an authoritative job payment transition.
const EventJobPaymentChanged = "wallet.job_payment.status_changed.v1"

// JobPaymentInput records or advances the authoritative payment for one unit
// of dispatched work.
type JobPaymentInput struct {
	Meta                     Meta
	OrganizationID           string
	SourceService            string
	DispatchRequestReference string
	TripReference            string
	BookingReference         string
	PaymentReference         string
	AmountMinor              int64
	Currency                 string
	Status                   string
	PaidAt                   *time.Time
	JournalID                string
	RefundState              string
	ReversalState            string
	OperationReference       string
}

func (i JobPaymentInput) normalized() JobPaymentInput {
	i.OrganizationID = strings.TrimSpace(i.OrganizationID)
	i.SourceService = strings.TrimSpace(i.SourceService)
	i.DispatchRequestReference = strings.TrimSpace(i.DispatchRequestReference)
	i.TripReference = strings.TrimSpace(i.TripReference)
	i.BookingReference = strings.TrimSpace(i.BookingReference)
	i.PaymentReference = strings.TrimSpace(i.PaymentReference)
	i.Currency = strings.ToUpper(strings.TrimSpace(i.Currency))
	i.Status = strings.ToUpper(strings.TrimSpace(i.Status))
	return i
}

func validJobPaymentStatus(status string) bool {
	switch status {
	case models.JobPaymentPending, models.JobPaymentAuthorized, models.JobPaymentPaid,
		models.JobPaymentFailed, models.JobPaymentRefunded, models.JobPaymentPartiallyRefunded,
		models.JobPaymentReversed, models.JobPaymentUnknown:
		return true
	default:
		return false
	}
}

// RecordJobPayment writes the authoritative payment record for a job.
//
// It requires finance.payment.confirm, which the Fleet service identity does
// not hold: a service that consumes payment verification can never mint one.
func (s *Service) RecordJobPayment(ctx context.Context, input JobPaymentInput) (*models.JobPayment, error) {
	input = input.normalized()
	if err := input.Meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, input.Meta, financeauth.PermPaymentConfirm); err != nil {
		return nil, err
	}
	if input.OrganizationID == "" || input.SourceService == "" {
		return nil, fmt.Errorf("%w: organization and source service are required", ErrInvalid)
	}
	if input.AmountMinor <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive", ErrInvalid)
	}
	if _, err := money.New(input.AmountMinor, input.Currency); err != nil {
		return nil, err
	}
	if !validJobPaymentStatus(input.Status) {
		return nil, fmt.Errorf("%w: unsupported payment status %q", ErrInvalid, input.Status)
	}
	if input.DispatchRequestReference == "" && input.TripReference == "" &&
		input.BookingReference == "" && input.PaymentReference == "" {
		return nil, fmt.Errorf("%w: at least one job reference is required", ErrInvalid)
	}

	fp := fingerprint("JOB_PAYMENT", input.OrganizationID, input.SourceService,
		input.DispatchRequestReference, input.TripReference, input.BookingReference,
		input.PaymentReference, input.AmountMinor, input.Currency)
	if existing, err := s.jobPayments.FindByKey(ctx, nil, input.OrganizationID, input.Meta.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != fp {
			return nil, ErrIdempotencyReuse
		}
		if existing.Status == input.Status {
			return existing, nil
		}
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}

	candidate := models.JobPayment{
		OrganizationID:           input.OrganizationID,
		SourceService:            input.SourceService,
		DispatchRequestReference: input.DispatchRequestReference,
		TripReference:            input.TripReference,
		BookingReference:         input.BookingReference,
		PaymentReference:         input.PaymentReference,
		AmountMinor:              input.AmountMinor,
		Currency:                 input.Currency,
		Status:                   input.Status,
		PaidAt:                   input.PaidAt,
		JournalID:                input.JournalID,
		RefundState:              strings.ToUpper(strings.TrimSpace(input.RefundState)),
		ReversalState:            strings.ToUpper(strings.TrimSpace(input.ReversalState)),
		OperationReference:       input.OperationReference,
		IdempotencyKey:           input.Meta.IdempotencyKey,
		RequestFingerprint:       fp,
		CorrelationID:            input.Meta.CorrelationID,
	}

	var result *models.JobPayment
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		previousStatus := ""
		existing, err := s.jobPayments.LockExisting(ctx, tx, candidate)
		switch {
		case err == nil:
			// Amount and currency identify the payment. A different value under
			// the same job reference is a different payment, and silently
			// overwriting it would destroy the evidence of the first.
			if existing.AmountMinor != candidate.AmountMinor || existing.Currency != candidate.Currency {
				return fmt.Errorf("%w: recorded amount or currency differs from the existing payment", ErrInvalid)
			}
			previousStatus = existing.Status
			if advanceErr := s.jobPayments.Advance(ctx, tx, existing.ID, existing.Version, candidate); advanceErr != nil {
				return advanceErr
			}
			candidate.ID = existing.ID
		case errors.Is(err, repositories.ErrNotFound):
			if insertErr := s.jobPayments.Insert(ctx, tx, &candidate); insertErr != nil {
				return insertErr
			}
		default:
			return err
		}

		if err := repositories.AppendHistory(ctx, tx, input.OrganizationID, "JOB_PAYMENT",
			candidate.ID, previousStatus, candidate.Status, input.Meta.Actor.Type,
			input.Meta.Actor.ID, "recorded", map[string]any{
				"sourceService": input.SourceService, "amountMinor": candidate.AmountMinor,
				"currency": candidate.Currency}, input.Meta.CorrelationID, input.Meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID,
			ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID,
			Action: "job_payment.recorded", ResourceType: "job_payment", ResourceID: candidate.ID,
			PreviousValues: map[string]any{"status": previousStatus},
			NewValues: map[string]any{"status": candidate.Status,
				"amountMinor": candidate.AmountMinor, "currency": candidate.Currency},
			RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); err != nil {
			return err
		}
		if err := s.events.Publish(ctx, tx, EventJobPaymentChanged, "PAYMENT_VERIFICATION",
			candidate.ID, candidate.Version, input.OrganizationID, input.Meta.CorrelationID,
			input.Meta.CausationID, map[string]any{"paymentVerificationId": candidate.ID,
				"status": candidate.Status, "amountMinor": candidate.AmountMinor,
				"currency": candidate.Currency, "refundState": candidate.RefundState,
				"reversalState": candidate.ReversalState}); err != nil {
			return err
		}
		result, err = s.jobPayments.Get(ctx, tx, input.OrganizationID, candidate.ID)
		return err
	})
	return result, err
}

// JobPaymentView is the minimum a consuming service needs in order to decide
// whether a job was paid for.
//
// It deliberately carries no balance, no ledger identifier and no provider
// reference: everything a caller could misuse to reconstruct Wallet's internal
// state is left out rather than filtered later.
type JobPaymentView struct {
	PaymentVerificationID string     `json:"paymentVerificationId"`
	OrganizationID        string     `json:"organizationId"`
	RequestReference      string     `json:"requestReference,omitempty"`
	TripReference         string     `json:"tripReference,omitempty"`
	AmountMinor           int64      `json:"amountMinor"`
	Currency              string     `json:"currency"`
	PaymentStatus         string     `json:"paymentStatus"`
	SourceVersion         int64      `json:"sourceVersion"`
	PaidAt                *time.Time `json:"paidAt,omitempty"`
	RefundState           string     `json:"refundState"`
	ReversalState         string     `json:"reversalState"`
	OperationReference    string     `json:"operationReference,omitempty"`
	Settled               bool       `json:"settled"`
}

func newJobPaymentView(payment *models.JobPayment) JobPaymentView {
	return JobPaymentView{
		PaymentVerificationID: payment.ID,
		OrganizationID:        payment.OrganizationID,
		RequestReference:      payment.DispatchRequestReference,
		TripReference:         payment.TripReference,
		AmountMinor:           payment.AmountMinor,
		Currency:              payment.Currency,
		PaymentStatus:         payment.Status,
		SourceVersion:         payment.Version,
		PaidAt:                payment.PaidAt,
		RefundState:           payment.RefundState,
		ReversalState:         payment.ReversalState,
		OperationReference:    payment.OperationReference,
		Settled:               payment.Settled(),
	}
}

// LookupJobPayment is the authoritative job payment verification.
//
// Not-found is reported as not-found for every reason — unsupported key,
// unknown reference, or a record belonging to another tenant — so the response
// cannot be used to probe for the existence of another organization's payments.
func (s *Service) LookupJobPayment(ctx context.Context, meta Meta,
	organizationID, referenceType, reference string) (JobPaymentView, error) {
	if err := s.AuthorizeRead(ctx, meta, financeauth.PermJobPaymentView); err != nil {
		return JobPaymentView{}, err
	}
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return JobPaymentView{}, financeauth.ErrForbidden
	}
	// A tenant-scoped actor may only ask about its own tenant.
	if meta.Actor.OrganizationID != "" && meta.Actor.OrganizationID != organizationID {
		return JobPaymentView{}, financeauth.ErrForbidden
	}
	payment, err := s.jobPayments.Lookup(ctx, nil, organizationID,
		referenceType, strings.TrimSpace(reference))
	if err != nil {
		return JobPaymentView{}, err
	}
	return newJobPaymentView(payment), nil
}

// SetJobPaymentOperationState links a refund or reversal to the job payment it
// affects, so a consumer reading the verification sees the operation that is
// in flight against it.
func (s *Service) SetJobPaymentOperationState(ctx context.Context, meta Meta,
	organizationID, id, refundState, reversalState, operationReference, status string) error {
	if err := meta.validate(); err != nil {
		return err
	}
	if err := s.require(ctx, meta, financeauth.PermPaymentConfirm); err != nil {
		return err
	}
	if status != "" && !validJobPaymentStatus(strings.ToUpper(status)) {
		return fmt.Errorf("%w: unsupported payment status %q", ErrInvalid, status)
	}
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		previous, err := s.jobPayments.Get(ctx, tx, organizationID, id)
		if err != nil {
			return err
		}
		if err := s.jobPayments.SetOperationState(ctx, tx, organizationID, id,
			strings.ToUpper(refundState), strings.ToUpper(reversalState),
			operationReference, strings.ToUpper(status)); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, organizationID, "JOB_PAYMENT", id,
			previous.Status, defaultString(strings.ToUpper(status), previous.Status),
			meta.Actor.Type, meta.Actor.ID, "operation state changed",
			map[string]any{"refundState": refundState, "reversalState": reversalState},
			meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventJobPaymentChanged, "PAYMENT_VERIFICATION", id,
			previous.Version+1, organizationID, meta.CorrelationID, meta.CausationID,
			map[string]any{"paymentVerificationId": id, "refundState": refundState,
				"reversalState": reversalState, "operationReference": operationReference})
	})
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
