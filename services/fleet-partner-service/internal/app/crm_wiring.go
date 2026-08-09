package app

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	approvalusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	crmmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	crmrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	crmusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// BuildCRMService assembles the Section 14 CRM service.
//
// It is always constructible. Whether a quotation can actually be issued to a
// customer depends on Media being configured and a token secret being present;
// an unconfigured integration makes those paths fail closed, and it never makes
// the read surface vanish from the API — an absent integration must be visible
// rather than silently missing.
func BuildCRMService(cfg config.Config, pool *pgxpool.Pool, metrics *observability.Metrics,
	publisher *events.DomainPublisher,
	approvals *approvalusecases.Service) *crmusecases.Service {
	var media crmusecases.MediaPort
	mediaCfg := cfg.Clients["media-file-service"]
	if !cfg.IntegrationsKillSwitch && !cfg.CRMKillSwitch && mediaCfg.Configured() {
		media = platformclients.NewMediaFileClient(platformclients.Options{
			ServiceName: "media-file-service", BaseURL: mediaCfg.BaseURL,
			Secret: mediaCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics})
	}

	var notifier crmusecases.NotificationPort
	notificationCfg := cfg.Clients["notification-service"]
	if !cfg.IntegrationsKillSwitch && !cfg.CRMKillSwitch && notificationCfg.Configured() {
		notifier = crmNotifier{
			client: notifications.Client{
				BaseURL:     notificationCfg.BaseURL,
				Secret:      notificationCfg.Secret,
				ServiceName: config.ServiceName,
				HTTPClient:  &http.Client{Timeout: cfg.HTTPClientTimeout},
			},
			repository: crmrepos.NewRepository(pool),
		}
	}

	return crmusecases.NewService(crmusecases.Options{
		Pool:          pool,
		Repository:    crmrepos.NewRepository(pool),
		Approvals:     crmApprovalAdapter{service: approvals},
		Media:         media,
		Notifications: notifier,
		Events:        publisher,
		Metrics:       metrics,
		Flags: crmusecases.Flags{
			CRMEnabled:             cfg.CRMEnabled,
			CustomerWritesEnabled:  cfg.CRMCustomerWritesEnabled,
			ContactWritesEnabled:   cfg.CRMContactWritesEnabled,
			LocationWritesEnabled:  cfg.CRMLocationWritesEnabled,
			OpportunitiesEnabled:   cfg.CRMOpportunitiesEnabled,
			QuotationsEnabled:      cfg.QuotationsEnabled,
			QuotationIssueEnabled:  cfg.QuotationIssueEnabled,
			QuotationAcceptEnabled: cfg.QuotationAcceptanceEnabled,
			ContractsEnabled:       cfg.ContractsEnabled,
			ContractActivation:     cfg.ContractActivationEnabled,
			SLAEnabled:             cfg.SLAEnabled,
			CreditEnabled:          cfg.CustomerCreditEnabled,

			NotificationWorkerEnabled: cfg.CRMNotificationWorkerEnabled,
			QuotationExpiryWorker:     cfg.QuotationExpiryWorkerEnabled,
			ContractExpiryWorker:      cfg.ContractExpiryWorkerEnabled,
			ContractRenewalWorker:     cfg.ContractRenewalWorkerEnabled,
			SLAEvaluationWorker:       cfg.SLAEvaluationWorkerEnabled,
			SLAEscalationWorker:       cfg.SLAEscalationWorkerEnabled,

			KillSwitch: cfg.CRMKillSwitch || cfg.IntegrationsKillSwitch,
		},
		// The session secret already keys refresh and invitation tokens. A
		// customer quotation link is the same class of bearer credential, so it
		// reuses that key rather than introducing a new mandatory secret whose
		// absence would break startup for deployments that do not use CRM.
		TokenSecret:    cfg.SessionSecret,
		ContactSecret:  cfg.ContactSecret,
		TokenTTL:       cfg.QuotationTokenTTL,
		WorkerMaxRetry: cfg.CRMWorkerMaxAttempts,
	})
}

// crmApprovalAdapter binds the Section 14 port to the WP9 approval framework.
//
// It exists so no second approval framework grows inside CRM: quorum, eligible
// roles, self-approval policy, separation of duties, expiry and immutable
// decisions all stay in the one engine that already implements them.
type crmApprovalAdapter struct{ service *approvalusecases.Service }

func (a crmApprovalAdapter) Request(ctx context.Context,
	input crmusecases.ApprovalRequestInput) (string, error) {
	if a.service == nil {
		return "", nil
	}
	result, err := a.service.Request(ctx, approvalusecases.RequestInput{
		Actor: approvalusecases.Actor{OrganizationID: input.OrganizationID,
			OrganizationUserID: input.OrganizationUserID, BranchID: input.BranchID,
			ActorType: "ORGANIZATION_USER", ActorID: input.OrganizationUserID},
		ResourceType: input.ResourceType, ResourceID: input.ResourceID,
		Action: input.Action, OperationType: input.OperationType, Payload: input.Payload,
		ExpectedSubjectVersion: input.ExpectedSubjectVersion,
		IdempotencyKey:         input.IdempotencyKey, CorrelationID: input.CorrelationID,
		CausationID: input.CausationID})
	if err != nil || result == nil || result.Request == nil {
		// A nil request means the policy did not require an approval at this
		// value. The caller reads an empty ID as "no approval needed" and
		// proceeds; it never reads it as "approved".
		return "", err
	}
	return result.Request.ID, nil
}

func (a crmApprovalAdapter) RequireApproved(ctx context.Context, organizationID,
	requestID string, payload map[string]any) error {
	if a.service == nil {
		return nil
	}
	_, err := a.service.RequireApproved(ctx, organizationID, requestID, payload)
	return err
}

// crmNotifier delivers a CRM notification intent through notification-service.
//
// The recipient address is resolved here, at delivery time, from the live
// contact record — never from the queued payload. A customer who withdraws
// consent between the intent being recorded and the worker running is not
// contacted, which would not be true if the address had been frozen into the
// queue.
type crmNotifier struct {
	client     notifications.Client
	repository *crmrepos.Repository
}

func (n crmNotifier) Configured() bool { return n.client.BaseURL != "" }

func (n crmNotifier) SupportedChannels() []string {
	// Only what this platform actually delivers. SMS and WhatsApp are accepted
	// as a stated contact preference and are deliberately absent here: claiming
	// a route that does not exist produces a silent no-op dressed as a success.
	return []string{crmmodels.ChannelEmail, crmmodels.ChannelInApp}
}

func (n crmNotifier) Deliver(ctx context.Context,
	operation crmmodels.NotificationOperation) (string, crmusecases.NotificationOutcome,
	string) {
	if !n.Configured() {
		return "", crmusecases.NotificationRetryable, "notification_unconfigured"
	}
	contact, err := n.resolveRecipient(ctx, operation)
	if err != nil {
		// The recipient cannot be resolved, so retrying will not help. This is a
		// permanent failure rather than an endless retry that hides the problem.
		return "", crmusecases.NotificationPermanent, "recipient_unresolved"
	}
	if !contact.Contactable() {
		return "", crmusecases.NotificationPermanent, "recipient_not_contactable"
	}

	response, err := n.client.Send(ctx, notifications.Request{
		IDempotencyKey: operation.IdempotencyKey,
		SourceService:  config.ServiceName,
		EventType:      strings.ToLower(operation.IntentType),
		Recipient: notifications.Recipient{
			Type:  "CRM_CONTACT",
			ID:    contact.ID,
			Email: contact.NormalizedEmail,
			Phone: contact.NormalizedPhone,
		},
		Channels: []string{notifications.ChannelEmail},
		Data:     operation.Payload,
		Priority: notifications.PriorityNormal,
	})
	if err != nil {
		if isTimeout(err) {
			// The send may well have landed. Reporting it as failed invites a
			// duplicate; reporting it as delivered hides a customer who was
			// never told. UNKNOWN is the only honest answer.
			return "", crmusecases.NotificationUnknown, "notification_timeout"
		}
		return "", crmusecases.NotificationRetryable, "notification_unavailable"
	}
	return response.MessageID, crmusecases.NotificationDelivered, ""
}

func (n crmNotifier) resolveRecipient(ctx context.Context,
	operation crmmodels.NotificationOperation) (crmmodels.Contact, error) {
	// RecipientReference is an opaque contact ID. The customer ID is not in the
	// operation, so the lookup goes through the repository's tenant-scoped read
	// by ID rather than by anything the payload asserts.
	return n.repository.GetContactByID(ctx, nil, operation.OrganizationID,
		operation.RecipientReference)
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "timeout") ||
		strings.Contains(message, "deadline exceeded") ||
		strings.Contains(message, "context canceled")
}

// StartCRMWorkers starts the Section 14 workers on the supplied wait group.
//
// A deployment that runs the dedicated worker process must get the same expiry
// sweeps and notification drain the in-process option provides; otherwise
// quotations would sit past their validity and issued offers would never be
// delivered, and nothing would say so.
func StartCRMWorkers(ctx context.Context, cfg config.Config, workers *sync.WaitGroup,
	service *crmusecases.Service) {
	if service == nil {
		return
	}
	if cfg.IntegrationsKillSwitch || cfg.CRMKillSwitch {
		logging.Notice(config.ServiceName+"-worker", "CRM workers stopped by kill switch")
		return
	}
	if !cfg.CRMEnabled {
		return
	}
	// Nothing is started unless at least one worker is actually enabled: an idle
	// ticker that never does anything is a goroutine and a log line pretending
	// to be a capability.
	if !cfg.CRMNotificationWorkerEnabled && !cfg.QuotationExpiryWorkerEnabled &&
		!cfg.ContractExpiryWorkerEnabled && !cfg.ContractRenewalWorkerEnabled &&
		!cfg.SLAEvaluationWorkerEnabled && !cfg.SLAEscalationWorkerEnabled {
		return
	}

	interval := cfg.CRMWorkerInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		_ = service.RunWorkers(ctx, crmusecases.WorkerConfig{
			Owner:       config.ServiceName + "-crm",
			BatchSize:   cfg.CRMWorkerBatchSize,
			Interval:    interval,
			MaxAttempts: cfg.CRMWorkerMaxAttempts,
		})
	}()
}
