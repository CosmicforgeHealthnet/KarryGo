package app

// FLEET-13.11 wiring: Fleet's settlement instructions reach payment-wallet
// through the financeops posting machinery.

import (
	"context"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fleetpartner"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/logging"
)

// NewFinancePostingAdapter builds the production posting adapter over signed
// payment-wallet and accounting clients. It is the same adapter BuildComponents
// wires; integration tests use it so the wire contract they exercise is the
// production one.
func NewFinancePostingAdapter(wallet, accounting *platformclients.Client) financeops.PostingPort {
	return financialPostingAdapter{wallet: walletPostingAdapter{client: wallet}, accounting: accountingPostingAdapter{client: accounting}}
}

// ManagedSettlementSubmitter adapts the financeops posting service to the
// fleetpartner money-authority port.
type ManagedSettlementSubmitter struct{ Finance *financeops.Service }

// SubmitManagedSettlement implements fleetpartner.SettlementSubmitter.
func (a ManagedSettlementSubmitter) SubmitManagedSettlement(ctx context.Context, in fleetpartner.SettlementSubmission) (fleetpartner.SettlementOutcome, error) {
	if a.Finance == nil {
		return fleetpartner.SettlementOutcome{Disposition: fleetpartner.SettlementUnavailable, ErrorCode: "posting_unavailable"},
			financeops.ErrManagedSettlementUnavailable
	}
	t := tenant.Context{
		OrganizationID: in.OrganizationID, ActorType: tenant.ActorTypeSystem,
		ActorID: "managed-settlement-delivery", CorrelationID: in.CorrelationID,
	}
	result, err := a.Finance.SubmitManagedSettlement(ctx, t, financeops.ManagedSettlementSubmission{
		InstructionID: in.InstructionID, InstructionVersion: in.InstructionVersion,
		OrganizationID: in.OrganizationID, MembershipID: in.MembershipID,
		ProviderID: in.ProviderID, ProviderType: in.ProviderType,
		SourceService: in.SourceService, SourceJobID: in.SourceJobID,
		SourceAssignmentID: in.SourceAssignmentID, Currency: in.Currency,
		GrossBasisMinor: in.GrossBasisMinor, ProviderShareMinor: in.ProviderShareMinor,
		OrganizationShareMinor: in.OrganizationShareMinor,
		CompensationPlanID:     in.CompensationPlanID, CompensationPlanVersion: in.CompensationPlanVersion,
		CompensationResolutionVersion: in.CompensationResolutionVersion,
		CorrelationID:                 in.CorrelationID, IdempotencyKey: in.IdempotencyKey,
	})
	return fleetpartner.SettlementOutcome{
		OperationID: result.OperationID, Disposition: result.Disposition,
		Reference: result.Reference, ConfirmedVersion: result.ConfirmedVersion, ErrorCode: result.ErrorCode,
	}, err
}

// managedSettlementDeliveryEnabled is the single gate for both the job
// handler and the recovery sweep. The posting flag and kill switch are
// enforced again inside financeops, so nothing posts when either is off even
// if this gate were bypassed.
func managedSettlementDeliveryEnabled(cfg config.Config) bool {
	return !cfg.IntegrationsKillSwitch && cfg.FleetSettlementInstructionEnabled
}

// RegisterManagedSettlementDelivery binds the delivery job handler.
func RegisterManagedSettlementDelivery(cfg config.Config, c *Components, worker *jobs.Worker) {
	if c == nil || worker == nil || c.FleetPartner == nil || !managedSettlementDeliveryEnabled(cfg) {
		return
	}
	c.FleetPartner.RegisterWorkers(worker)
}

// StartManagedSettlementSweep runs the recovery sweep once a minute: every
// undelivered instruction without a live delivery job gets one.
func StartManagedSettlementSweep(ctx context.Context, cfg config.Config, c *Components, wg *sync.WaitGroup) {
	if c == nil || wg == nil || c.FleetPartner == nil || !managedSettlementDeliveryEnabled(cfg) || !cfg.FinancePostingEnabled {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		run := func() {
			if n, err := c.FleetPartner.ScheduleSettlementDelivery(ctx, 200); err != nil {
				logging.Notice(config.ServiceName, "managed settlement sweep failed: %v", err)
			} else if n > 0 {
				logging.Notice(config.ServiceName, "managed settlement sweep scheduled %d deliveries", n)
			}
		}
		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
