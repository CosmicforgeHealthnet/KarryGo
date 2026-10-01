// Package referralusecases is the referral programme's business workflow:
// turning relayed events into activity, referrals and rewards, and running
// the background decisions that follow.
package referralusecases

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/referral-service/internal/database"
	"cosmicforge/logistics/services/referral-service/internal/events"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// Producer service names, as they sign their requests.
const (
	ProducerWallet   = "payment-wallet-service"
	ProducerCustomer = "customer-service"
	ProducerHauling  = "driver-hauling-service"
	ProducerDispatch = "driver-dispatch-delivery-service"
	ProducerTaxi     = "driver-taxi-service"
)

// Producers lists every service allowed to send events.
func Producers() []string {
	return []string{ProducerWallet, ProducerCustomer, ProducerHauling, ProducerDispatch, ProducerTaxi}
}

// Outcome is what happened to one delivered event.
type Outcome string

const (
	OutcomeProcessed Outcome = "processed"
	// OutcomeDuplicate: this event, or another event for the same business
	// fact, was already applied.
	OutcomeDuplicate Outcome = "duplicate"
	// OutcomeIgnored: valid, but outside the programme (e.g. a vendor order).
	OutcomeIgnored Outcome = "ignored"
	// OutcomeRejected: malformed; acknowledged and never applied.
	OutcomeRejected Outcome = "rejected"
)

// Options wires the service.
type Options struct {
	Store *referralrepositories.Store
	Rules referralmodels.Rules
	// ShareLinkBase is prepended to a code to build its share link.
	ShareLinkBase string
	Now           func() time.Time
	// Random feeds code generation; nil uses crypto/rand.
	Random io.Reader
	// Wallet posts approved rewards to payment-wallet. Nil leaves them
	// queued (wallet_state=pending) until a wallet is configured.
	Wallet WalletClient
}

// Service runs the referral programme.
type Service struct {
	store         *referralrepositories.Store
	inbox         *events.Inbox
	rules         referralmodels.Rules
	shareLinkBase string
	now           func() time.Time
	random        io.Reader
	wallet        WalletClient
}

// NewService builds the service.
func NewService(opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		store:         opts.Store,
		inbox:         events.NewInbox(opts.Store.Pool()),
		rules:         opts.Rules,
		shareLinkBase: opts.ShareLinkBase,
		now:           now,
		random:        opts.Random,
		wallet:        opts.Wallet,
	}
}

// eventRejection is a permanent, payload-level problem with an event.
type eventRejection struct{ code string }

func (e eventRejection) Error() string { return "event rejected: " + e.code }

func reject(code string) error { return eventRejection{code: code} }

// HandleEvent applies one relayed event from an authenticated caller. The
// inbox claim, every business effect and the inbox completion share one
// transaction, so a crash at any point leaves the event re-deliverable and
// never half-applied.
func (s *Service) HandleEvent(ctx context.Context, caller string, raw []byte) (Outcome, error) {
	var envelope outbox.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil || strings.TrimSpace(envelope.EventID) == "" || strings.TrimSpace(envelope.EventType) == "" {
		return "", apperrors.BadRequest("Event envelope is invalid.", err)
	}
	if envelope.Producer != caller {
		return "", apperrors.Forbidden("Event producer does not match the calling service.", nil)
	}
	if !producerMayEmit(caller, envelope.EventType) {
		return "", apperrors.Forbidden("This service may not emit this event type.", nil)
	}

	outcome := OutcomeProcessed
	err := database.WithinTx(ctx, s.store.Pool(), func(tx pgx.Tx) error {
		if err := s.inbox.Begin(ctx, tx, envelope.EventID, envelope.EventType, caller, raw); err != nil {
			return err
		}
		var handled Outcome
		var handleErr error
		switch envelope.EventType {
		case referralcontract.EventJobSettled:
			handled, handleErr = s.handleJobSettled(ctx, tx, envelope.Payload)
		case referralcontract.EventJobSettlementReversed:
			handled, handleErr = s.handleJobSettlementReversed(ctx, tx, envelope.Payload)
		case referralcontract.EventCustomerRegistered, referralcontract.EventProviderRegistered:
			handled, handleErr = s.handleAccountRegistered(ctx, tx, caller, envelope.EventType, envelope.Payload)
		case referralcontract.EventProviderOnboardingCompleted:
			handled, handleErr = s.handleProviderOnboardingCompleted(ctx, tx, caller, envelope.Payload)
		default:
			// Known producer, event type this build does not act on yet:
			// acknowledge rather than make the producer retry forever.
			handled = OutcomeIgnored
		}
		var rejection eventRejection
		if errors.As(handleErr, &rejection) {
			outcome = OutcomeRejected
			return s.inbox.MarkRejected(ctx, tx, envelope.EventID, rejection.code)
		}
		if handleErr != nil {
			return handleErr
		}
		outcome = handled
		return s.inbox.MarkProcessed(ctx, tx, envelope.EventID)
	})
	switch {
	case errors.Is(err, events.ErrDuplicateEvent):
		return OutcomeDuplicate, nil
	case errors.Is(err, events.ErrPayloadMismatch):
		return "", apperrors.Conflict("This event id was already delivered with a different payload.", err)
	case err != nil:
		s.inbox.MarkFailed(ctx, envelope.EventID, envelope.EventType, caller, raw, "processing_failed")
		return "", apperrors.Unavailable("The event could not be processed; it will be retried.", err)
	}
	return outcome, nil
}

// producerMayEmit pins each event type to the one service that owns the fact
// it reports. Without this, any service holding a secret could forge a paid
// job and mint commission.
func producerMayEmit(caller, eventType string) bool {
	switch eventType {
	case referralcontract.EventJobSettled, referralcontract.EventJobSettlementReversed:
		return caller == ProducerWallet
	case referralcontract.EventCustomerRegistered:
		return caller == ProducerCustomer
	case referralcontract.EventProviderRegistered, referralcontract.EventProviderOnboardingCompleted:
		return caller == ProducerHauling || caller == ProducerDispatch || caller == ProducerTaxi
	}
	// Unknown event types from a known producer are acknowledged and ignored.
	for _, producer := range Producers() {
		if caller == producer {
			return true
		}
	}
	return false
}

// providerActorFor is the driver actor type each provider service owns.
func providerActorFor(caller string) string {
	switch caller {
	case ProducerHauling:
		return referralcontract.ActorHauling
	case ProducerDispatch:
		return referralcontract.ActorDispatch
	case ProducerTaxi:
		return referralcontract.ActorTaxi
	}
	return ""
}

// handleJobSettled records a paid-out job and the activity it proves.
func (s *Service) handleJobSettled(ctx context.Context, tx pgx.Tx, raw json.RawMessage) (Outcome, error) {
	var payload referralcontract.JobSettled
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", reject("invalid_payload")
	}
	if err := payload.Validate(); err != nil {
		return "", reject("invalid_job")
	}
	job := referralmodels.SettledJobFrom(payload)
	if err := s.store.LockJob(ctx, tx, job.SourceService, job.SourceReference); err != nil {
		return "", err
	}
	inserted, err := s.store.InsertSettledJob(ctx, tx, job)
	if err != nil {
		return "", err
	}
	if !inserted {
		// Same job, different event id (a replay or re-emit): already applied.
		return OutcomeDuplicate, nil
	}
	if !job.InProgramme {
		return OutcomeIgnored, nil
	}
	if err := s.store.RecordActivity(ctx, tx, referralcontract.ActorCustomer, job.CustomerID, job.SourceService, job.SourceReference, job.JobCompletedAt); err != nil {
		return "", err
	}
	if err := s.store.RecordActivity(ctx, tx, job.ProviderActorType, job.ProviderID, job.SourceService, job.SourceReference, job.JobCompletedAt); err != nil {
		return "", err
	}
	if err := s.applyJobToReferrals(ctx, tx, job); err != nil {
		return "", err
	}
	// A reversal of this job may have arrived before its settlement.
	if _, err := s.applyReversals(ctx, tx, job); err != nil {
		return "", err
	}
	return OutcomeProcessed, nil
}

// ActivityCount reports an account's paid jobs in the activity window ending
// at asOf.
func (s *Service) ActivityCount(ctx context.Context, actorType, actorID string, asOf time.Time) (int, error) {
	if !referralcontract.ValidActorType(actorType) || strings.TrimSpace(actorID) == "" {
		return 0, apperrors.BadRequest("A valid account type and id are required.", nil)
	}
	if asOf.IsZero() {
		asOf = s.now()
	}
	return s.store.CountActivity(ctx, s.store.Pool(), actorType, actorID, asOf, s.rules.Activity.Window, 1_000_000)
}
