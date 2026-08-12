package usecases

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// WorkerConfig tunes one CRM worker loop.
type WorkerConfig struct {
	Owner       string
	BatchSize   int
	Interval    time.Duration
	Lease       time.Duration
	MaxAttempts int
	Backoff     time.Duration
}

func (c WorkerConfig) normalized() WorkerConfig {
	if c.Owner == "" {
		c.Owner = "fleet-partner-crm"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 25
	}
	if c.Interval <= 0 {
		c.Interval = 30 * time.Second
	}
	if c.Lease <= 0 {
		c.Lease = 2 * time.Minute
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	if c.Backoff <= 0 {
		c.Backoff = time.Minute
	}
	return c
}

// RunWorkers runs every enabled CRM worker until the context is cancelled.
//
// One loop drives all of them so a deployment cannot end up with the expiry
// sweep running and the notification drain not, which is the shape of failure
// where quotations quietly expire and nobody is told.
//
// Shutdown is graceful: the loop returns on cancellation between passes, and a
// pass in flight finishes its batch. Anything it had leased and did not
// complete is recovered by the next process through the lease expiry, so a
// restart mid-batch loses nothing.
func (s *Service) RunWorkers(ctx context.Context, config WorkerConfig) error {
	config = config.normalized()
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.RunPass(ctx, config)
		}
	}
}

// RunPass performs one pass of every enabled worker.
//
// Each worker's failure is isolated: a Media outage stalling the contract sweep
// must not stop quotations from expiring, because an expired quotation that
// stays open is an offer a customer can still accept.
func (s *Service) RunPass(ctx context.Context, config WorkerConfig) {
	config = config.normalized()
	if s.opts.Flags.KillSwitch || !s.opts.Flags.CRMEnabled {
		return
	}
	if s.workerEnabled(s.opts.Flags.QuotationExpiryWorker) {
		_, _ = s.ExpireQuotations(ctx, config)
	}
	if s.workerEnabled(s.opts.Flags.ContractExpiryWorker) {
		_, _ = s.ExpireContracts(ctx, config)
	}
	if s.workerEnabled(s.opts.Flags.ContractRenewalWorker) {
		_, _ = s.SweepContractRenewals(ctx, config)
	}
	if s.workerEnabled(s.opts.Flags.SLAEvaluationWorker) {
		_, _ = s.EvaluateObservations(ctx, config)
	}
	if s.workerEnabled(s.opts.Flags.SLAEscalationWorker) {
		_, _ = s.EscalateSLABreaches(ctx, config)
	}
	if s.workerEnabled(s.opts.Flags.NotificationWorkerEnabled) {
		_, _ = s.DeliverNotifications(ctx, config)
	}
	s.RefreshMetrics(ctx)
}

// ExpireQuotations moves delivered quotations past their validity to EXPIRED.
//
// Server time is the authority throughout. The claim query already excludes
// resolved quotations, and the transition re-checks the status under a row lock
// — so an acceptance that lands between the claim and the write wins, and a
// quotation a customer has already accepted can never be expired out from under
// them.
func (s *Service) ExpireQuotations(ctx context.Context, config WorkerConfig) (int, error) {
	config = config.normalized()
	candidates, err := s.opts.Repository.ClaimExpiredQuotations(ctx, config.BatchSize)
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, candidate := range candidates {
		quotationID := candidate.ID
		organizationID := candidate.OrganizationID
		txErr := s.withTx(ctx, func(tx pgx.Tx) error {
			quotation, lockErr := s.opts.Repository.LockQuotation(ctx, tx, organizationID,
				quotationID)
			if lockErr != nil {
				return lockErr
			}
			// Re-checked under the lock, not trusted from the claim.
			if !models.DeliveredQuotation(quotation.Status) ||
				!quotation.Expired(s.now()) {
				return nil
			}
			updated, transitionErr := s.opts.Repository.TransitionQuotation(ctx, tx,
				organizationID, quotationID, quotation.Version,
				repositories.QuotationTransition{
					FromStatus: quotation.Status, ToStatus: models.QuotationExpired,
					MarkResolved: true, Actor: models.ActorSystem,
				})
			if transitionErr != nil {
				return transitionErr
			}
			if historyErr := s.opts.Repository.AppendQuotationHistory(ctx, tx,
				models.QuotationHistory{
					OrganizationID: organizationID, QuotationID: quotationID,
					PreviousStatus: quotation.Status, NewStatus: updated.Status,
					QuotationVersion: updated.CurrentVersion, ReasonCode: "validity_elapsed",
					ActorType: models.ActorSystem, ActorID: config.Owner,
				}); historyErr != nil {
				return historyErr
			}
			if auditErr := s.audit.Record(ctx, tx, auditEntry{
				OrganizationID: organizationID, ActorType: models.ActorSystem,
				ActorID: config.Owner, Action: "crm.quotation.expired",
				ResourceType: "quotation", ResourceID: quotationID,
				PreviousValues: map[string]any{"status": quotation.Status},
				NewValues:      map[string]any{"status": updated.Status},
			}); auditErr != nil {
				return auditErr
			}
			primary := s.primaryContactFor(ctx, tx, organizationID, quotation.CustomerID,
				models.PurposeSales)
			if notifyErr := s.enqueueNotification(ctx, tx, organizationID, "quotation",
				quotationID, models.IntentQuotationExpired, primary.ID,
				notificationChannelFor(primary),
				models.JSONMap{"quotationNumber": updated.QuotationNumber},
				"quotation_expired:"+quotationID, "", ""); notifyErr != nil {
				return notifyErr
			}
			expired++
			return s.publish(ctx, tx, EventQuotationExpired, "quotation", quotationID,
				updated.Version, organizationID, "",
				quotationStatusEventPayload(updated, quotation.Status, "validity_elapsed"))
		})
		if txErr != nil {
			// One quotation failing does not stop the batch: the rest are
			// independent, and this one is picked up on the next pass.
			s.count("crm_worker_errors_total", map[string]string{"worker": "quotation_expiry"})
			continue
		}
	}
	if expired > 0 {
		s.count("quotations_expired_total", nil)
	}
	return expired, nil
}

// ExpireContracts moves contracts past their end date to EXPIRED.
//
// Expiry never terminates a contract early: the claim requires the end date to
// be in the past and the transition re-checks it under the lock. A contract
// with time left on it cannot be swept up by this worker.
func (s *Service) ExpireContracts(ctx context.Context, config WorkerConfig) (int, error) {
	config = config.normalized()
	candidates, err := s.opts.Repository.ClaimExpiringContracts(ctx, config.BatchSize)
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, candidate := range candidates {
		contractID, organizationID := candidate.ID, candidate.OrganizationID
		txErr := s.withTx(ctx, func(tx pgx.Tx) error {
			contract, lockErr := s.opts.Repository.LockContract(ctx, tx, organizationID,
				contractID)
			if lockErr != nil {
				return lockErr
			}
			if contract.EffectiveTo == nil || contract.EffectiveTo.After(s.now()) {
				return nil
			}
			if !models.CanTransitionContract(contract.Status, models.ContractExpired) {
				return nil
			}
			updated, transitionErr := s.opts.Repository.TransitionContract(ctx, tx,
				organizationID, contractID, contract.Version,
				repositories.ContractTransition{
					FromStatus: contract.Status, ToStatus: models.ContractExpired,
					Actor: models.ActorSystem,
				})
			if transitionErr != nil {
				return transitionErr
			}
			if historyErr := s.opts.Repository.AppendContractHistory(ctx, tx,
				models.ContractHistory{
					OrganizationID: organizationID, ContractID: contractID,
					PreviousStatus: contract.Status, NewStatus: updated.Status,
					ReasonCode: "term_elapsed", ActorType: models.ActorSystem,
					ActorID: config.Owner,
				}); historyErr != nil {
				return historyErr
			}
			if auditErr := s.audit.Record(ctx, tx, auditEntry{
				OrganizationID: organizationID, ActorType: models.ActorSystem,
				ActorID: config.Owner, Action: "crm.contract.expired",
				ResourceType: "contract", ResourceID: contractID,
				PreviousValues: map[string]any{"status": contract.Status},
				NewValues:      map[string]any{"status": updated.Status},
			}); auditErr != nil {
				return auditErr
			}
			primary := s.primaryContactFor(ctx, tx, organizationID, contract.CustomerID,
				models.PurposeLegal)
			if notifyErr := s.enqueueNotification(ctx, tx, organizationID, "contract",
				contractID, models.IntentContractExpired, primary.ID,
				notificationChannelFor(primary),
				models.JSONMap{"contractNumber": updated.ContractNumber},
				"contract_expired:"+contractID, "", ""); notifyErr != nil {
				return notifyErr
			}
			expired++
			return s.publish(ctx, tx, EventContractExpired, "contract", contractID,
				updated.Version, organizationID, "",
				contractStatusEventPayload(updated, contract.Status, "term_elapsed"))
		})
		if txErr != nil {
			s.count("crm_worker_errors_total", map[string]string{"worker": "contract_expiry"})
			continue
		}
	}
	return expired, nil
}

// SweepContractRenewals raises renewal reminders for contracts nearing their
// end date.
//
// A reminder failure never changes contract state. The contract is untouched by
// this worker; all it produces is a durable notification intent, so a
// notification outage delays the reminder and nothing else.
func (s *Service) SweepContractRenewals(ctx context.Context, config WorkerConfig) (int, error) {
	config = config.normalized()
	candidates, err := s.opts.Repository.ClaimRenewalDueContracts(ctx, s.opts.RenewalWindow,
		config.BatchSize)
	if err != nil {
		return 0, err
	}
	reminded := 0
	for _, contract := range candidates {
		contractID, organizationID := contract.ID, contract.OrganizationID
		effectiveTo := contract.EffectiveTo
		txErr := s.withTx(ctx, func(tx pgx.Tx) error {
			primary := s.primaryContactFor(ctx, tx, organizationID, contract.CustomerID,
				models.PurposeLegal)
			key := "contract_renewal_reminder:" + contractID
			if effectiveTo != nil {
				// Keyed by the end date so a renewal that moves the date raises a
				// fresh reminder, while a repeated sweep of the same term does
				// not.
				key += ":" + effectiveTo.UTC().Format(time.RFC3339)
			}
			return s.enqueueNotification(ctx, tx, organizationID, "contract", contractID,
				models.IntentContractRenewalDue, primary.ID, notificationChannelFor(primary),
				models.JSONMap{"contractNumber": contract.ContractNumber}, key, "", "")
		})
		if txErr != nil {
			s.count("crm_worker_errors_total", map[string]string{"worker": "contract_renewal"})
			continue
		}
		reminded++
	}
	if reminded > 0 {
		s.count("contract_renewal_reminders_total", nil)
	}
	return reminded, nil
}

// EvaluateObservations resolves measurements that arrived unevaluated.
//
// An observation whose metric or unit cannot be interpreted becomes UNKNOWN and
// one with no measured value becomes INSUFFICIENT_DATA. Neither is ever turned
// into MET: an SLA that could not be measured has not been met, and a
// monitoring outage must not read as perfect service.
func (s *Service) EvaluateObservations(ctx context.Context, config WorkerConfig) (int, error) {
	config = config.normalized()
	pending, err := s.opts.Repository.ClaimUnevaluatedObservations(ctx, config.BatchSize)
	if err != nil {
		return 0, err
	}
	evaluated := 0
	for _, observation := range pending {
		sla, slaErr := s.opts.Repository.GetSLA(ctx, nil, observation.OrganizationID,
			observation.SLAID)
		if slaErr != nil {
			continue
		}
		outcome := models.EvaluateOutcome(sla.MetricCode, observation.MeasurementUnit,
			sla.TargetValue, observation.MeasuredValue)
		if outcome == models.OutcomeNotEvaluated {
			continue
		}
		// Observations are append-only: the resolved outcome is recorded as a
		// new row at the next source version rather than by editing the one that
		// arrived. The immutable record of what a source said stands.
		txErr := s.withTx(ctx, func(tx pgx.Tx) error {
			resolved, duplicate, appendErr := s.opts.Repository.AppendObservation(ctx, tx,
				models.SLAObservation{
					OrganizationID:  observation.OrganizationID,
					SLAID:           observation.SLAID,
					SourceType:      observation.SourceType,
					SourceReference: observation.SourceReference,
					MeasuredValue:   observation.MeasuredValue,
					MeasurementUnit: observation.MeasurementUnit,
					MeasuredAt:      observation.MeasuredAt,
					WindowStart:     observation.WindowStart,
					WindowEnd:       observation.WindowEnd,
					Outcome:         outcome,
					SourceVersion:   observation.SourceVersion + 1,
					EventID:         observation.EventID,
					PayloadHash:     observation.PayloadHash,
					CorrelationID:   observation.CorrelationID,
				})
			if appendErr != nil || duplicate {
				return appendErr
			}
			evaluated++
			return s.publish(ctx, tx, EventSLAOutcomeRecorded, "service_level_agreement",
				sla.ID, sla.Version, sla.OrganizationID, observation.CorrelationID,
				slaOutcomeEventPayload(sla, resolved))
		})
		if txErr != nil {
			s.count("crm_worker_errors_total", map[string]string{"worker": "sla_evaluation"})
			continue
		}
	}
	if evaluated > 0 {
		s.count("sla_outcomes_total", nil)
	}
	return evaluated, nil
}

// EscalateSLABreaches raises attention for missed service commitments.
//
// An escalation is a request for a human to look. It creates no penalty, no
// customer credit, no provider deduction, no refund and no wallet journal:
// whether a missed SLA costs anybody money is a commercial question this
// service has no authority to answer, and answering it automatically would
// move money nobody agreed to move.
func (s *Service) EscalateSLABreaches(ctx context.Context, config WorkerConfig) (int, error) {
	config = config.normalized()
	since := s.now().Add(-24 * time.Hour)
	breaches, err := s.opts.Repository.ClaimMissedObservations(ctx, since, config.BatchSize)
	if err != nil {
		return 0, err
	}
	escalated := 0
	for _, observation := range breaches {
		sla, slaErr := s.opts.Repository.GetSLA(ctx, nil, observation.OrganizationID,
			observation.SLAID)
		if slaErr != nil || sla.Status != models.SLAActive {
			continue
		}
		caseReference := ""
		if s.opts.Support != nil && s.opts.Support.Configured() && sla.ContractID != "" {
			// A support case only where a real contract exists: raising one
			// against no agreement produces noise nobody can action.
			caseReference, _ = s.opts.Support.RaiseSLAEscalation(ctx, sla.OrganizationID,
				sla.ID, sla.ContractID, observation.CorrelationID)
		}
		observationID := observation.ID
		txErr := s.withTx(ctx, func(tx pgx.Tx) error {
			primary := s.primaryContactFor(ctx, tx, sla.OrganizationID, sla.CustomerID,
				models.PurposeOperations)
			if notifyErr := s.enqueueNotification(ctx, tx, sla.OrganizationID,
				"service_level_agreement", sla.ID, models.IntentSLAEscalation, primary.ID,
				notificationChannelFor(primary),
				models.JSONMap{"metricCode": sla.MetricCode, "outcome": observation.Outcome},
				"sla_escalation:"+observationID, observation.CorrelationID,
				""); notifyErr != nil {
				return notifyErr
			}
			escalated++
			return s.publish(ctx, tx, EventSLAEscalationRequested, "service_level_agreement",
				sla.ID, sla.Version, sla.OrganizationID, observation.CorrelationID,
				slaEscalationEventPayload(sla, observationID, caseReference))
		})
		if txErr != nil {
			s.count("crm_worker_errors_total", map[string]string{"worker": "sla_escalation"})
			continue
		}
	}
	if escalated > 0 {
		s.count("sla_escalations_total", nil)
	}
	return escalated, nil
}

// DeliverNotifications drains the durable CRM notification queue.
//
// Leases are taken with an expiry, so a worker that crashes mid-delivery has
// its rows returned to the pool rather than leaving them stuck. An unknown
// outcome stays retryable rather than being written off in either direction: a
// send whose fate could not be determined may well have landed, and recording
// it as failed invites a duplicate while recording it as delivered hides a
// customer who was never told.
func (s *Service) DeliverNotifications(ctx context.Context, config WorkerConfig) (int, error) {
	config = config.normalized()
	if _, err := s.opts.Repository.RecoverExpiredNotificationLeases(ctx); err != nil {
		return 0, err
	}
	if s.opts.Notifications == nil || !s.opts.Notifications.Configured() {
		// No delivery route. The queue is left exactly as it is rather than
		// drained into a fake success: an operator must be able to see that
		// nothing was sent.
		return 0, nil
	}
	claimed, err := s.opts.Repository.ClaimNotifications(ctx, config.Owner, config.Lease,
		config.BatchSize)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, operation := range claimed {
		reference, outcome, safeCode := s.opts.Notifications.Deliver(ctx, operation)
		switch outcome {
		case NotificationDelivered:
			if markErr := s.opts.Repository.MarkNotificationDelivered(ctx, config.Owner,
				operation.ID, reference); markErr == nil {
				delivered++
			}
		case NotificationUnknown:
			_ = s.opts.Repository.MarkNotificationUnknown(ctx, config.Owner, operation.ID,
				safeCode, config.Backoff)
		case NotificationPermanent:
			_ = s.opts.Repository.MarkNotificationPermanent(ctx, config.Owner, operation.ID,
				safeCode)
		default:
			_ = s.opts.Repository.MarkNotificationRetryable(ctx, config.Owner, operation.ID,
				safeCode, config.Backoff, config.MaxAttempts)
		}
	}
	return delivered, nil
}

// ReplayNotification returns a dead-lettered notification operation to the
// queue.
//
// The identity is preserved — same row, same idempotency key, same payload — so
// an operator repairing a downstream outage replays the original intent rather
// than minting a new one that could deliver twice.
func (s *Service) ReplayNotification(ctx context.Context, t tenant.Context,
	operationID string) (models.NotificationOperation, error) {
	if err := s.writeEnabled(s.opts.Flags.CRMEnabled); err != nil {
		return models.NotificationOperation{}, err
	}
	var replayed models.NotificationOperation
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var txErr error
		replayed, txErr = s.opts.Repository.ReplayNotification(ctx, tx, t.OrganizationID,
			operationID)
		if txErr != nil {
			return translate(txErr, "Notification operation",
				errorcodes.QuotationNotFound, errorcodes.QuotationAcceptanceConflict)
		}
		return s.record(ctx, tx, t, "crm.notification.replayed", "crm_notification_operation",
			operationID, nil, map[string]any{"status": replayed.Status}, "operator_replay")
	})
	if err != nil {
		return models.NotificationOperation{}, err
	}
	return replayed, nil
}

// ListNotifications returns a tenant-scoped page of notification operations.
func (s *Service) ListNotifications(ctx context.Context, t tenant.Context, status,
	subjectID string, limit int) ([]models.NotificationOperation, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListNotifications(ctx, t.OrganizationID, status, subjectID,
		cursor.Cursor{}, false, limit)
}

// RefreshMetrics publishes the CRM gauges.
//
// Every label here is a status or a stage. No organization ID, customer ID,
// quotation ID or contract ID is ever a metric label: those are unbounded
// cardinality and, worse, a way to enumerate one tenant's records from another
// tenant's dashboard.
func (s *Service) RefreshMetrics(ctx context.Context) {
	if s.opts.Metrics == nil {
		return
	}
	if counts, err := s.opts.Repository.CountCustomersByStatus(ctx); err == nil {
		var total int64
		for status, value := range counts {
			s.gauge("crm_customers_total", map[string]string{"status": status},
				float64(value))
			total += value
		}
		s.gauge("crm_customers_total", map[string]string{"status": "all"}, float64(total))
	}
	if total, err := s.opts.Repository.CountContacts(ctx); err == nil {
		s.gauge("crm_contacts_total", nil, float64(total))
	}
	if total, err := s.opts.Repository.CountLocations(ctx); err == nil {
		s.gauge("crm_locations_total", nil, float64(total))
	}
	if counts, err := s.opts.Repository.CountOpportunitiesByStage(ctx); err == nil {
		var total int64
		for stage, value := range counts {
			s.gauge("sales_opportunities_by_stage", map[string]string{"stage": stage},
				float64(value))
			total += value
		}
		s.gauge("sales_opportunities_total", nil, float64(total))
	}
	if counts, err := s.opts.Repository.CountContractsByStatus(ctx); err == nil {
		var total int64
		for status, value := range counts {
			switch status {
			case models.ContractActive:
				s.gauge("contracts_active_total", nil, float64(value))
			case models.ContractSuspended:
				s.gauge("contracts_suspended_total", nil, float64(value))
			}
			total += value
		}
		s.gauge("contracts_total", nil, float64(total))
	}
	if total, err := s.opts.Repository.CountSLAs(ctx); err == nil {
		s.gauge("sla_definitions_total", nil, float64(total))
	}
	if counts, err := s.opts.Repository.CountObservationsByOutcome(ctx); err == nil {
		for outcome, value := range counts {
			s.gauge("sla_outcomes_total", map[string]string{"outcome": outcome},
				float64(value))
		}
	}
	if profiles, holds, err := s.opts.Repository.CountCreditHolds(ctx); err == nil {
		s.gauge("customer_credit_profiles_total", nil, float64(profiles))
		s.gauge("customer_credit_holds_total", nil, float64(holds))
	}
	if backlog, deadLetters, err := s.opts.Repository.NotificationBacklog(ctx); err == nil {
		s.gauge("crm_notification_retry_backlog", nil, float64(backlog))
		s.gauge("crm_worker_dead_letters", nil, float64(deadLetters))
	}
}
