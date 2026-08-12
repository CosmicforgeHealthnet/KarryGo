package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	approvalusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	walletrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/repositories"
	walletusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/usecases"
	eligibilityenforcement "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/enforcement"
	financefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/finance"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// BuildCompanyWalletService assembles the Section 13 service.
//
// It is always constructible. Whether an allocation or a withdrawal can
// actually reach payment-wallet-service depends on the Wallet client being
// configured; an unconfigured client makes those paths fail closed and makes
// the workers refuse to run, and it never makes the read surface vanish from
// the API — an absent integration must be visible rather than silently missing.
func BuildCompanyWalletService(cfg config.Config, pool *pgxpool.Pool,
	metrics *observability.Metrics, publisher *events.DomainPublisher,
	approvals *approvalusecases.Service, eligibility eligibilityenforcement.Gate,
	finance *financefeature.Service) *walletusecases.Service {
	var wallet platformclients.FinanceWalletContract
	walletCfg := cfg.Clients["payment-wallet-service"]
	if !cfg.IntegrationsKillSwitch && !cfg.CompanyWalletKillSwitch && walletCfg.Configured() {
		wallet = platformclients.NewWalletClient(platformclients.Options{
			ServiceName: "payment-wallet-service", BaseURL: walletCfg.BaseURL,
			Secret: walletCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics})
	}

	return walletusecases.NewService(walletusecases.Options{
		Pool:        pool,
		Repository:  walletrepos.NewRepository(pool),
		Wallet:      wallet,
		Approvals:   approvalAdapter{service: approvals},
		Eligibility: eligibilityAdapter{gate: eligibility},
		Work:        workAdapter{finance: finance},
		Events:      publisher,
		Metrics:     metrics,
		Flags: walletusecases.Flags{
			WalletReadEnabled:        cfg.OrganizationWalletReadEnabled,
			SubWalletEnabled:         cfg.ProviderSubWalletEnabled,
			AllocationEnabled:        cfg.ProviderAllocationEnabled,
			WithdrawalEnabled:        cfg.ProviderWithdrawalEnabled,
			ProjectionRefreshEnabled: cfg.WalletProjectionRefreshEnabled,
			ReconciliationEnabled:    cfg.WalletReconciliationEnabled,
			KillSwitch:               cfg.CompanyWalletKillSwitch || cfg.IntegrationsKillSwitch,
		},
		StaleAfter: cfg.CompanyWalletStaleAfter,
	})
}

// approvalAdapter binds the Section 13 port to the WP9 approval framework.
// It exists so no second approval framework grows inside this feature.
type approvalAdapter struct{ service *approvalusecases.Service }

func (a approvalAdapter) Request(ctx context.Context,
	input walletusecases.ApprovalRequestInput) (string, error) {
	if a.service == nil {
		return "", nil
	}
	result, err := a.service.Request(ctx, approvalusecases.RequestInput{
		Actor: approvalusecases.Actor{OrganizationID: input.OrganizationID,
			OrganizationUserID: input.OrganizationUserID, BranchID: input.BranchID,
			ActorType: "ORGANIZATION_USER", ActorID: input.OrganizationUserID},
		ResourceType: input.ResourceType, ResourceID: input.ResourceID, Action: input.Action,
		OperationType: input.OperationType, Payload: input.Payload,
		ExpectedSubjectVersion: input.ExpectedSubjectVersion,
		IdempotencyKey:         input.IdempotencyKey, CorrelationID: input.CorrelationID,
		CausationID: input.CausationID})
	if err != nil || result == nil || result.Request == nil {
		return "", err
	}
	return result.Request.ID, nil
}

func (a approvalAdapter) RequireApproved(ctx context.Context, organizationID, requestID string,
	payload map[string]any) error {
	if a.service == nil {
		return nil
	}
	_, err := a.service.RequireApproved(ctx, organizationID, requestID, payload)
	return err
}

// eligibilityAdapter binds the Section 13 port to the durable eligibility gate.
type eligibilityAdapter struct{ gate eligibilityenforcement.Gate }

func (e eligibilityAdapter) Require(ctx context.Context, organizationID, branchID, operation,
	actorType, actorID, correlationID string) error {
	subject := eligibilityenforcement.Company(organizationID, operation)
	subject.BranchID = branchID
	subject.ActorType = actorType
	subject.ActorID = actorID
	subject.CorrelationID = correlationID
	_, err := eligibilityenforcement.Require(ctx, e.gate, subject)
	return err
}

// workAdapter answers "did this job actually complete?" from the existing
// finance gate, which reads the Dispatch-owned trip projection. Fleet does not
// re-derive completion; it reuses the one place that already knows.
type workAdapter struct{ finance *financefeature.Service }

func (w workAdapter) RequireCompletedDelivery(ctx context.Context, organizationID, tripID string,
	requireRecipientConfirmation bool) error {
	if w.finance == nil {
		// No finance service means no way to confirm completion. Refusing is
		// the only safe answer: a per-job allocation for unverified work is a
		// payment for work that may never have happened.
		return context.Canceled
	}
	return w.finance.RequireCompletedDelivery(ctx, organizationID, tripID,
		requireRecipientConfirmation)
}
